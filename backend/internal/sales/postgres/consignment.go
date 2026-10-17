package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

func numberText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func fromPgDate(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	b := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &b
}

func nullDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}

func decimalPtr(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	v := d.Decimal
	return &v
}

func validation(err error, fields map[string]string) error {
	e := apperr.Validation(fields)
	e.Err = err
	return e
}

func mapConsignmentWriteError(err error, what string) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		switch pe.ConstraintName {
		case "uq_consignment_reconciliations_open":
			return validation(err, map[string]string{
				"partnerId": "Mitra ini masih punya rekonsiliasi yang belum selesai"})
		case "uq_consignment_reconciliations_cut_off":
			return validation(err, map[string]string{
				"cutOffDate": "Rekonsiliasi mitra untuk cut-off ini sudah ada"})
		case "uq_consignment_reconciliations_stock_count":
			return validation(err, map[string]string{
				"stockCountId": "Stock count ini sudah dipakai rekonsiliasi lain"})
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// --- Pengiriman konsinyasi -----------------------------------------------------------------

// DeliveryFilter adalah kriteria daftar pengiriman konsinyasi.
type DeliveryFilter struct {
	Status    domain.Status
	Type      domain.DeliveryType
	PartnerID *uuid.UUID
	Search    string
}

func toDelivery(r sqlcgen.ConsignmentDelivery) domain.ConsignmentDelivery {
	return domain.ConsignmentDelivery{
		ID: r.ID, Number: r.Number.String, Type: domain.DeliveryType(r.DeliveryType), PartnerID: r.PartnerID,
		PartnerCode: r.PartnerCode, PartnerName: r.PartnerName, AgreementID: uuidPtr(r.PartnerAgreementID),
		AgreementNumber: r.AgreementNumber, FromLocationID: r.FromLocationID, ToLocationID: r.ToLocationID,
		ChannelID: r.ChannelID, SubChannelID: r.SubChannelID, BusinessDate: r.BusinessDate,
		Status: domain.Status(r.Status), SourceType: r.SourceType, SourceID: uuidPtr(r.SourceID),
		TransferID: uuidPtr(r.TransferID), TransferNumber: r.TransferNumber, Cost: amount(r.CostAmount), Note: r.Note,
		CancelReason: r.CancelReason, ShippedAt: r.ShippedAt, ReceivedAt: r.ReceivedAt, CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) deliveryLines(ctx context.Context, d *domain.ConsignmentDelivery) error {
	rows, err := s.q(ctx).ListConsignmentDeliveryLines(ctx, d.ID)
	if err != nil {
		return fmt.Errorf("list consignment delivery lines: %w", err)
	}
	d.Lines = make([]domain.DeliveryLine, len(rows))
	for i, l := range rows {
		d.Lines[i] = domain.DeliveryLine{
			ID: l.ID, LineNo: int(l.LineNo), ProductID: l.ProductID, BatchID: uuidPtr(l.BatchID), Qty: l.Qty,
			Cost: amount(l.CostAmount), Note: l.Note,
		}
	}
	return nil
}

func (s *Store) insertDeliveryLines(ctx context.Context, d *domain.ConsignmentDelivery) error {
	if err := s.q(ctx).DeleteConsignmentDeliveryLines(ctx, d.ID); err != nil {
		return fmt.Errorf("delete consignment delivery lines: %w", err)
	}
	for i, l := range d.Lines {
		if err := s.q(ctx).InsertConsignmentDeliveryLine(ctx, sqlcgen.InsertConsignmentDeliveryLineParams{
			DeliveryID: d.ID, LineNo: int32(i + 1), ProductID: l.ProductID, BatchID: nullUUID(l.BatchID), Qty: l.Qty, //nolint:gosec // ≤ 200 baris
			CostAmount: l.Cost.Decimal(), Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert consignment delivery line: %w", err)
		}
	}
	return s.deliveryLines(ctx, d)
}

// InsertDelivery menyimpan pengiriman baru beserta baris.
func (s *Store) InsertDelivery(ctx context.Context, d domain.ConsignmentDelivery, actorID uuid.UUID) (domain.ConsignmentDelivery, error) {
	r, err := s.q(ctx).InsertConsignmentDelivery(ctx, sqlcgen.InsertConsignmentDeliveryParams{
		Number: numberText(d.Number), DeliveryType: string(d.Type), PartnerID: d.PartnerID, PartnerCode: d.PartnerCode,
		PartnerName: d.PartnerName, PartnerAgreementID: nullUUID(d.AgreementID), AgreementNumber: d.AgreementNumber,
		FromLocationID: d.FromLocationID, ToLocationID: d.ToLocationID, ChannelID: d.ChannelID,
		SubChannelID: d.SubChannelID, BusinessDate: d.BusinessDate, Status: string(d.Status), SourceType: d.SourceType,
		SourceID: nullUUID(d.SourceID), Note: d.Note, ActorID: actorID,
	})
	if err != nil {
		return domain.ConsignmentDelivery{}, fmt.Errorf("insert consignment delivery: %w", err)
	}
	d.ID, d.CreatedAt, d.UpdatedAt, d.Version, d.CreatedBy = r.ID, r.CreatedAt, r.UpdatedAt, int(r.Version), actorID
	return d, s.insertDeliveryLines(ctx, &d)
}

// SaveDelivery menyimpan perubahan pengiriman dengan optimistic locking; replaceLines menulis ulang baris.
func (s *Store) SaveDelivery(ctx context.Context, d *domain.ConsignmentDelivery, ver int, actorID uuid.UUID, replaceLines bool) error {
	r, err := s.q(ctx).UpdateConsignmentDelivery(ctx, sqlcgen.UpdateConsignmentDeliveryParams{
		Number: numberText(d.Number), PartnerAgreementID: nullUUID(d.AgreementID), AgreementNumber: d.AgreementNumber,
		FromLocationID: d.FromLocationID, ToLocationID: d.ToLocationID, ChannelID: d.ChannelID,
		SubChannelID: d.SubChannelID, BusinessDate: d.BusinessDate, Status: string(d.Status),
		SourceType: d.SourceType, SourceID: nullUUID(d.SourceID), TransferID: nullUUID(d.TransferID),
		TransferNumber: d.TransferNumber,
		CostAmount:     d.Cost.Decimal(), Note: d.Note, CancelReason: d.CancelReason, ShippedAt: d.ShippedAt,
		ReceivedAt: d.ReceivedAt, ActorID: actorID, ID: d.ID, Version: version(ver),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if err != nil {
		return fmt.Errorf("update consignment delivery: %w", err)
	}
	d.UpdatedAt, d.Version = r.UpdatedAt, int(r.Version)
	if replaceLines {
		return s.insertDeliveryLines(ctx, d)
	}
	return nil
}

// GetDelivery membaca pengiriman beserta baris; forUpdate mengunci baris dokumen.
func (s *Store) GetDelivery(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ConsignmentDelivery, error) {
	var (
		r   sqlcgen.ConsignmentDelivery
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetConsignmentDeliveryForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetConsignmentDelivery(ctx, id)
	}
	if err != nil {
		return domain.ConsignmentDelivery{}, notFound(err, "get consignment delivery")
	}
	d := toDelivery(r)
	return d, s.deliveryLines(ctx, &d)
}

// DeliveryBySource membaca pengiriman yang dicatat dokumen sumber (DO); ok false bila belum ada.
func (s *Store) DeliveryBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.ConsignmentDelivery, bool, error) {
	r, err := s.q(ctx).ConsignmentDeliveryBySource(ctx, sqlcgen.ConsignmentDeliveryBySourceParams{
		SourceType: sourceType, SourceID: uuid.NullUUID{UUID: sourceID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ConsignmentDelivery{}, false, nil
	}
	if err != nil {
		return domain.ConsignmentDelivery{}, false, fmt.Errorf("consignment delivery by source: %w", err)
	}
	d := toDelivery(r)
	return d, true, s.deliveryLines(ctx, &d)
}

// ListDeliveries mengembalikan daftar pengiriman tanpa baris.
func (s *Store) ListDeliveries(ctx context.Context, f DeliveryFilter, page paging.Page) (paging.Result[domain.ConsignmentDelivery], error) {
	st, tp, search := text(string(f.Status)), text(string(f.Type)), text(f.Search)
	rows, err := s.q(ctx).ListConsignmentDeliveries(ctx, sqlcgen.ListConsignmentDeliveriesParams{
		Status: st, DeliveryType: tp, PartnerID: nullUUID(f.PartnerID), Search: search,
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.ConsignmentDelivery]{}, fmt.Errorf("list consignment deliveries: %w", err)
	}
	total, err := s.q(ctx).CountConsignmentDeliveries(ctx, sqlcgen.CountConsignmentDeliveriesParams{
		Status: st, DeliveryType: tp, PartnerID: nullUUID(f.PartnerID), Search: search,
	})
	if err != nil {
		return paging.Result[domain.ConsignmentDelivery]{}, fmt.Errorf("count consignment deliveries: %w", err)
	}
	items := make([]domain.ConsignmentDelivery, len(rows))
	for i, r := range rows {
		items[i] = toDelivery(r)
	}
	return paging.Result[domain.ConsignmentDelivery]{Items: items, Page: page, Total: total}, nil
}

// --- Rekonsiliasi konsinyasi ---------------------------------------------------------------

// ReconciliationFilter adalah kriteria daftar rekonsiliasi.
type ReconciliationFilter struct {
	Status    domain.Status
	PartnerID *uuid.UUID
	Search    string
}

func toReconciliation(r sqlcgen.ConsignmentReconciliation) domain.Reconciliation {
	return domain.Reconciliation{
		ID: r.ID, Number: r.Number.String, PartnerID: r.PartnerID, PartnerCode: r.PartnerCode, PartnerName: r.PartnerName,
		PartnerAddress: r.PartnerAddress, AgreementID: uuidPtr(r.PartnerAgreementID), AgreementNumber: r.AgreementNumber,
		PaymentTermDays: int(r.PaymentTermDays), MarginPercent: decimalPtr(r.MarginPercent),
		Rules: domain.VarianceRules{
			Lost: domain.BorneBy(r.LossBorneBy), PartnerDamage: domain.BorneBy(r.PartnerDamageBorneBy),
			ProductDefect: domain.BorneBy(r.ProductDefectBorneBy), NearExpiry: domain.BorneBy(r.NearExpiryBorneBy),
		},
		LocationID: r.LocationID, ChannelID: r.ChannelID, SubChannelID: r.SubChannelID,
		PeriodStart: fromPgDate(r.PeriodStart), CutOffDate: r.CutOffDate, StockCountID: r.StockCountID,
		StockCountNumber: r.StockCountNumber, PartnerReportFileID: uuidPtr(r.PartnerReportFileID),
		BAFileID: uuidPtr(r.BaFileID), Status: domain.Status(r.Status), Note: r.Note, SalesAmount: amount(r.SalesAmount),
		BilledVariance: amount(r.BilledVarianceAmount), AbsorbedCost: amount(r.AbsorbedVarianceCost),
		Cost: amount(r.CostAmount), InvoiceID: uuidPtr(r.InvoiceID), VarianceInvoiceID: uuidPtr(r.VarianceInvoiceID),
		StartedAt: r.StartedAt, SubmittedAt: r.SubmittedAt, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
		PostedAt: r.PostedAt, CancelReason: r.CancelReason, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) reconciliationLines(ctx context.Context, rc *domain.Reconciliation) error {
	rows, err := s.q(ctx).ListConsignmentReconciliationLines(ctx, rc.ID)
	if err != nil {
		return fmt.Errorf("list consignment reconciliation lines: %w", err)
	}
	rc.Lines = make([]domain.ReconciliationLine, len(rows))
	for i, l := range rows {
		rc.Lines[i] = domain.ReconciliationLine{
			ID: l.ID, LineNo: int(l.LineNo), ProductID: l.ProductID, OpeningQty: l.OpeningQty, DeliveredQty: l.DeliveredQty,
			ReturnedQty: l.ReturnedQty, CountedQty: l.CountedQty, SoldQty: l.SoldQty, ReportedQty: l.ReportedQty,
			VarianceQty: l.VarianceQty, Cause: domain.VarianceCause(l.VarianceCause), BorneBy: domain.BorneBy(l.VarianceBorneBy),
			UnitPrice: amount(l.UnitPriceAmount), PriceRule: l.PriceRule, PriceDescription: l.PriceDescription,
			SalesAmount: amount(l.SalesAmount), VarianceAmount: amount(l.VarianceAmount), Cost: amount(l.CostAmount),
			VarianceCost: amount(l.VarianceCostAmount), Note: l.Note,
		}
	}
	return nil
}

// InsertReconciliation menyimpan rekonsiliasi Draft.
func (s *Store) InsertReconciliation(ctx context.Context, rc domain.Reconciliation, actorID uuid.UUID) (domain.Reconciliation, error) {
	r, err := s.q(ctx).InsertConsignmentReconciliation(ctx, sqlcgen.InsertConsignmentReconciliationParams{
		PartnerID: rc.PartnerID, PartnerCode: rc.PartnerCode, PartnerName: rc.PartnerName, PartnerAddress: rc.PartnerAddress,
		LocationID: rc.LocationID, ChannelID: rc.ChannelID, SubChannelID: rc.SubChannelID, CutOffDate: rc.CutOffDate,
		StockCountID: rc.StockCountID, StockCountNumber: rc.StockCountNumber, Note: rc.Note, ActorID: actorID,
	})
	if err != nil {
		return domain.Reconciliation{}, mapConsignmentWriteError(err, "insert consignment reconciliation")
	}
	rc.ID, rc.CreatedAt, rc.UpdatedAt, rc.Version, rc.CreatedBy = r.ID, r.CreatedAt, r.UpdatedAt, int(r.Version), actorID
	return rc, nil
}

// LineMode menentukan cara baris rekonsiliasi disimpan.
type LineMode int

const (
	// LinesKeep tidak menyentuh baris.
	LinesKeep LineMode = iota
	// LinesReplace menulis ulang semua baris (mulai rekonsiliasi).
	LinesReplace
	// LinesUpdate memperbarui isian laporan mitra, selisih, dan HPP per baris.
	LinesUpdate
)

// SaveReconciliation menyimpan rekonsiliasi dengan optimistic locking.
func (s *Store) SaveReconciliation(ctx context.Context, rc *domain.Reconciliation, ver int, actorID uuid.UUID, mode LineMode) error {
	rules := rc.Rules.Normalize()
	r, err := s.q(ctx).UpdateConsignmentReconciliation(ctx, sqlcgen.UpdateConsignmentReconciliationParams{
		Number: numberText(rc.Number), PartnerAgreementID: nullUUID(rc.AgreementID), AgreementNumber: rc.AgreementNumber,
		PaymentTermDays: int32(rc.PaymentTermDays), MarginPercent: nullDecimal(rc.MarginPercent), //nolint:gosec // ≤ 365
		LossBorneBy: string(rules.Lost), PartnerDamageBorneBy: string(rules.PartnerDamage),
		ProductDefectBorneBy: string(rules.ProductDefect), NearExpiryBorneBy: string(rules.NearExpiry),
		LocationID: rc.LocationID, ChannelID: rc.ChannelID, SubChannelID: rc.SubChannelID,
		PeriodStart: pgDate(rc.PeriodStart), CutOffDate: rc.CutOffDate, StockCountID: rc.StockCountID,
		StockCountNumber: rc.StockCountNumber, PartnerReportFileID: nullUUID(rc.PartnerReportFileID),
		BaFileID: nullUUID(rc.BAFileID), Status: string(rc.Status), Note: rc.Note, SalesAmount: rc.SalesAmount.Decimal(),
		BilledVarianceAmount: rc.BilledVariance.Decimal(), AbsorbedVarianceCost: rc.AbsorbedCost.Decimal(),
		CostAmount: rc.Cost.Decimal(), InvoiceID: nullUUID(rc.InvoiceID), VarianceInvoiceID: nullUUID(rc.VarianceInvoiceID),
		StartedAt: rc.StartedAt, SubmittedAt: rc.SubmittedAt, DecidedAt: rc.DecidedAt, DecisionNote: rc.DecisionNote,
		PostedAt: rc.PostedAt, CancelReason: rc.CancelReason, ActorID: actorID, ID: rc.ID, Version: version(ver),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if err != nil {
		return mapConsignmentWriteError(err, "update consignment reconciliation")
	}
	rc.UpdatedAt, rc.Version = r.UpdatedAt, int(r.Version)
	switch mode {
	case LinesReplace:
		if err := s.q(ctx).DeleteConsignmentReconciliationLines(ctx, rc.ID); err != nil {
			return fmt.Errorf("delete consignment reconciliation lines: %w", err)
		}
		for i := range rc.Lines {
			l := &rc.Lines[i]
			id, err := s.q(ctx).InsertConsignmentReconciliationLine(ctx, sqlcgen.InsertConsignmentReconciliationLineParams{
				ReconciliationID: rc.ID, LineNo: int32(i + 1), ProductID: l.ProductID, OpeningQty: l.OpeningQty, //nolint:gosec // baris sedikit
				DeliveredQty: l.DeliveredQty, ReturnedQty: l.ReturnedQty, CountedQty: l.CountedQty, SoldQty: l.SoldQty,
				ReportedQty: l.ReportedQty, VarianceQty: l.VarianceQty, VarianceCause: string(l.Cause),
				VarianceBorneBy: string(l.BorneBy), UnitPriceAmount: l.UnitPrice.Decimal(), PriceRule: l.PriceRule,
				PriceDescription: l.PriceDescription, SalesAmount: l.SalesAmount.Decimal(),
				VarianceAmount: l.VarianceAmount.Decimal(), CostAmount: l.Cost.Decimal(),
				VarianceCostAmount: l.VarianceCost.Decimal(), Note: l.Note,
			})
			if err != nil {
				return fmt.Errorf("insert consignment reconciliation line: %w", err)
			}
			l.ID, l.LineNo = id, i+1
		}
	case LinesUpdate:
		for _, l := range rc.Lines {
			if err := s.q(ctx).UpdateConsignmentReconciliationLine(ctx, sqlcgen.UpdateConsignmentReconciliationLineParams{
				ReportedQty: l.ReportedQty, VarianceQty: l.VarianceQty, VarianceCause: string(l.Cause),
				VarianceBorneBy: string(l.BorneBy), SalesAmount: l.SalesAmount.Decimal(), VarianceAmount: l.VarianceAmount.Decimal(),
				CostAmount: l.Cost.Decimal(), VarianceCostAmount: l.VarianceCost.Decimal(), Note: l.Note, ID: l.ID,
			}); err != nil {
				return fmt.Errorf("update consignment reconciliation line: %w", err)
			}
		}
	case LinesKeep:
	}
	return nil
}

// GetReconciliation membaca rekonsiliasi beserta baris; forUpdate mengunci dokumen.
func (s *Store) GetReconciliation(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Reconciliation, error) {
	var (
		r   sqlcgen.ConsignmentReconciliation
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetConsignmentReconciliationForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetConsignmentReconciliation(ctx, id)
	}
	if err != nil {
		return domain.Reconciliation{}, notFound(err, "get consignment reconciliation")
	}
	rc := toReconciliation(r)
	return rc, s.reconciliationLines(ctx, &rc)
}

// LastPostedCutOff mengembalikan cut-off rekonsiliasi Diposting terakhir mitra (nil bila belum ada).
func (s *Store) LastPostedCutOff(ctx context.Context, partnerID uuid.UUID) (*clock.BusinessDate, error) {
	d, err := s.q(ctx).LastPostedCutOff(ctx, partnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("last posted cut-off: %w", err)
	}
	return &d, nil
}

// ListReconciliations mengembalikan daftar rekonsiliasi tanpa baris.
func (s *Store) ListReconciliations(ctx context.Context, f ReconciliationFilter, page paging.Page) (paging.Result[domain.Reconciliation], error) {
	st, search := text(string(f.Status)), text(f.Search)
	rows, err := s.q(ctx).ListConsignmentReconciliations(ctx, sqlcgen.ListConsignmentReconciliationsParams{
		Status: st, PartnerID: nullUUID(f.PartnerID), Search: search, RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Reconciliation]{}, fmt.Errorf("list consignment reconciliations: %w", err)
	}
	total, err := s.q(ctx).CountConsignmentReconciliations(ctx, sqlcgen.CountConsignmentReconciliationsParams{
		Status: st, PartnerID: nullUUID(f.PartnerID), Search: search,
	})
	if err != nil {
		return paging.Result[domain.Reconciliation]{}, fmt.Errorf("count consignment reconciliations: %w", err)
	}
	items := make([]domain.Reconciliation, len(rows))
	for i, r := range rows {
		items[i] = toReconciliation(r)
	}
	return paging.Result[domain.Reconciliation]{Items: items, Page: page, Total: total}, nil
}
