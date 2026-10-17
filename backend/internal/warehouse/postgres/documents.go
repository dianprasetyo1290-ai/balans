package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres/sqlcgen"
)

func ptrToNull(id *uuid.UUID) uuid.NullUUID { return nullUUID(id) }

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

func textOf(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func int4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true} //nolint:gosec // dibatasi validasi domain (1–31)
}

func intPtr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int32)
	return &i
}

func uuids(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// --- Inbound ----------------------------------------------------------------------------------

func toInbound(r sqlcgen.Inbound) domain.Inbound {
	return domain.Inbound{
		ID: r.ID, Number: r.Number, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
		WarehouseID: r.WarehouseID, LocationID: r.LocationID, BusinessDate: r.BusinessDate,
		RejectAction: domain.RejectAction(r.RejectAction), Status: domain.Status(r.Status), Note: r.Note,
		CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toInboundLine(r sqlcgen.InboundLine) domain.InboundLine {
	return domain.InboundLine{
		ID: r.ID, LineNo: int(r.LineNo), SourceLineID: uuidPtr(r.SourceLineID), ProductID: r.ProductID,
		BatchID: uuidPtr(r.BatchID), Qty: r.Qty, PassedQty: r.PassedQty, RejectedQty: r.RejectedQty,
		DestinationLocationID: r.DestinationLocationID, Status: domain.LineStatus(r.Status), RejectReason: r.RejectReason,
		RejectLocationID: uuidPtr(r.RejectLocationID), CheckedBy: uuidPtr(r.CheckedBy), CheckedByName: r.CheckedByName,
		CheckedAt: r.CheckedAt,
	}
}

// InsertInbound menyimpan Inbound beserta barisnya. Dokumen asal yang sudah punya Inbound ditolak.
func (s *Store) InsertInbound(ctx context.Context, ib domain.Inbound, actorID uuid.UUID) (domain.Inbound, error) {
	q := s.q(ctx)
	r, err := q.InsertInbound(ctx, sqlcgen.InsertInboundParams{
		Number: ib.Number, SourceType: ib.SourceType, SourceID: ib.SourceID, SourceLabel: ib.SourceLabel,
		WarehouseID: ib.WarehouseID, LocationID: ib.LocationID, BusinessDate: ib.BusinessDate,
		RejectAction: string(ib.RejectAction), Status: string(ib.Status), Note: ib.Note, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_inbounds_source") {
		return domain.Inbound{}, apperr.Validation(map[string]string{"source.id": "Dokumen asal ini sudah punya Inbound"})
	}
	if err != nil {
		return domain.Inbound{}, fmt.Errorf("insert inbound: %w", err)
	}
	out := toInbound(r)
	for _, l := range ib.Lines {
		id, err := q.InsertInboundLine(ctx, sqlcgen.InsertInboundLineParams{
			InboundID: r.ID, LineNo: int32(l.LineNo), SourceLineID: ptrToNull(l.SourceLineID), ProductID: l.ProductID, //nolint:gosec // maks. 200 baris
			BatchID: ptrToNull(l.BatchID), Qty: l.Qty, DestinationLocationID: l.DestinationLocationID,
		})
		if err != nil {
			return domain.Inbound{}, fmt.Errorf("insert inbound line: %w", err)
		}
		l.ID = id
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// GetInbound membaca Inbound beserta barisnya; forUpdate mengunci header.
func (s *Store) GetInbound(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Inbound, error) {
	q := s.q(ctx)
	var (
		r   sqlcgen.Inbound
		err error
	)
	if forUpdate {
		r, err = q.GetInboundForUpdate(ctx, id)
	} else {
		r, err = q.GetInbound(ctx, id)
	}
	if err != nil {
		return domain.Inbound{}, notFound(err, "get inbound")
	}
	return s.withInboundLines(ctx, toInbound(r))
}

// InboundBySource membaca Inbound milik dokumen asal.
func (s *Store) InboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.Inbound, error) {
	r, err := s.q(ctx).GetInboundBySource(ctx, sqlcgen.GetInboundBySourceParams{SourceType: sourceType, SourceID: sourceID})
	if err != nil {
		return domain.Inbound{}, notFound(err, "get inbound by source")
	}
	return s.withInboundLines(ctx, toInbound(r))
}

func (s *Store) withInboundLines(ctx context.Context, ib domain.Inbound) (domain.Inbound, error) {
	rows, err := s.q(ctx).ListInboundLines(ctx, ib.ID)
	if err != nil {
		return domain.Inbound{}, fmt.Errorf("list inbound lines: %w", err)
	}
	for _, l := range rows {
		ib.Lines = append(ib.Lines, toInboundLine(l))
	}
	return ib, nil
}

// SaveInboundQC menyimpan hasil QC satu baris, catatan QC, dan status header (optimistic lock header).
func (s *Store) SaveInboundQC(ctx context.Context, ib domain.Inbound, l domain.InboundLine, checks []domain.QualityCheck, actorID uuid.UUID) (domain.Inbound, error) {
	q := s.q(ctx)
	if err := q.UpdateInboundLineQC(ctx, sqlcgen.UpdateInboundLineQCParams{
		ID: l.ID, PassedQty: l.PassedQty, RejectedQty: l.RejectedQty, DestinationLocationID: l.DestinationLocationID,
		Status: string(l.Status), RejectReason: l.RejectReason, RejectLocationID: ptrToNull(l.RejectLocationID),
		CheckedBy: ptrToNull(l.CheckedBy), CheckedByName: l.CheckedByName, CheckedAt: l.CheckedAt,
	}); err != nil {
		return domain.Inbound{}, fmt.Errorf("update inbound line: %w", err)
	}
	for _, c := range checks {
		raw, err := json.Marshal(c.Checks)
		if err != nil {
			return domain.Inbound{}, err
		}
		if err := q.InsertQualityCheck(ctx, sqlcgen.InsertQualityCheckParams{
			DocumentType: c.DocumentType, DocumentID: c.DocumentID, DocumentLineID: c.DocumentLineID, Result: c.Result,
			Qty: c.Qty, Checks: raw, Note: c.Note, PhotoFileIds: uuids(c.PhotoFileIDs), CheckedBy: c.CheckedBy,
			CheckedByName: c.CheckedByName, CheckedAt: c.CheckedAt,
		}); err != nil {
			return domain.Inbound{}, fmt.Errorf("insert quality check: %w", err)
		}
	}
	r, err := q.UpdateInbound(ctx, sqlcgen.UpdateInboundParams{
		ID: ib.ID, Version: int32(ib.Version), Status: string(ib.Status), CompletedAt: ib.CompletedAt, ActorID: actorID, //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Inbound{}, versionConflict(err, "update inbound")
	}
	return s.withInboundLines(ctx, toInbound(r))
}

// QualityChecks mengembalikan catatan QC dokumen.
func (s *Store) QualityChecks(ctx context.Context, documentType string, documentID uuid.UUID) ([]domain.QualityCheck, error) {
	rows, err := s.q(ctx).ListQualityChecks(ctx, sqlcgen.ListQualityChecksParams{DocumentType: documentType, DocumentID: documentID})
	if err != nil {
		return nil, fmt.Errorf("list quality checks: %w", err)
	}
	out := make([]domain.QualityCheck, len(rows))
	for i, r := range rows {
		var c domain.Checklist
		if len(r.Checks) > 0 {
			if err := json.Unmarshal(r.Checks, &c); err != nil {
				return nil, fmt.Errorf("decode quality check: %w", err)
			}
		}
		out[i] = domain.QualityCheck{
			ID: r.ID, DocumentType: r.DocumentType, DocumentID: r.DocumentID, DocumentLineID: r.DocumentLineID,
			Result: r.Result, Qty: r.Qty, Checks: c, Note: r.Note, PhotoFileIDs: r.PhotoFileIds, CheckedBy: r.CheckedBy,
			CheckedByName: r.CheckedByName, CheckedAt: r.CheckedAt,
		}
	}
	return out, nil
}

// InboundFilter adalah kriteria daftar Inbound.
type InboundFilter struct {
	Status      string
	SourceType  string
	WarehouseID *uuid.UUID
	Search      string
}

// InboundSummary adalah satu baris daftar Inbound.
type InboundSummary struct {
	Inbound      domain.Inbound
	LineCount    int
	PendingCount int
}

// ListInbounds mengembalikan satu halaman Inbound terbaru.
func (s *Store) ListInbounds(ctx context.Context, f InboundFilter, page paging.Page) (paging.Result[InboundSummary], error) {
	q := s.q(ctx)
	rows, err := q.ListInbounds(ctx, sqlcgen.ListInboundsParams{
		Status: nullText(f.Status), SourceType: nullText(f.SourceType), WarehouseID: ptrToNull(f.WarehouseID),
		Search: nullText(f.Search), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[InboundSummary]{}, fmt.Errorf("list inbounds: %w", err)
	}
	total, err := q.CountInbounds(ctx, sqlcgen.CountInboundsParams{
		Status: nullText(f.Status), SourceType: nullText(f.SourceType), WarehouseID: ptrToNull(f.WarehouseID), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[InboundSummary]{}, fmt.Errorf("count inbounds: %w", err)
	}
	items := make([]InboundSummary, len(rows))
	for i, r := range rows {
		items[i] = InboundSummary{
			Inbound: toInbound(sqlcgen.Inbound{
				ID: r.ID, Number: r.Number, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
				WarehouseID: r.WarehouseID, LocationID: r.LocationID, BusinessDate: r.BusinessDate, RejectAction: r.RejectAction,
				Status: r.Status, Note: r.Note, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
				UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			LineCount: int(r.LineCount), PendingCount: int(r.PendingCount),
		}
	}
	return paging.Result[InboundSummary]{Items: items, Page: page, Total: total}, nil
}

// --- Transfer Stok ----------------------------------------------------------------------------

func toTransfer(r sqlcgen.StockTransfer) domain.StockTransfer {
	return domain.StockTransfer{
		ID: r.ID, Number: textOf(r.Number), Type: domain.TransferType(r.TransferType), FromLocationID: r.FromLocationID,
		ToLocationID: r.ToLocationID, RequiresShipping: r.RequiresShipping, BusinessDate: r.BusinessDate,
		Status: domain.Status(r.Status), SourceType: r.SourceType, SourceID: uuidPtr(r.SourceID), Note: r.Note,
		RequestedBy: r.RequestedBy, RequestedByName: r.RequestedByName, SubmittedAt: r.SubmittedAt,
		ReleasedBy: uuidPtr(r.ReleasedBy), ReleasedByName: r.ReleasedByName, ReleasedAt: r.ReleasedAt,
		PreparedBy: uuidPtr(r.PreparedBy), PreparedByName: r.PreparedByName, PreparedAt: r.PreparedAt,
		ShippedBy: uuidPtr(r.ShippedBy), ShippedByName: r.ShippedByName, ShippedAt: r.ShippedAt,
		ReceivedBy: uuidPtr(r.ReceivedBy), ReceivedByName: r.ReceivedByName, ReceivedAt: r.ReceivedAt,
		CompletedAt: r.CompletedAt, CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toTransferLine(r sqlcgen.StockTransferLine) domain.TransferLine {
	return domain.TransferLine{
		ID: r.ID, LineNo: int(r.LineNo), ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), Qty: r.Qty,
		ReceivedQty: r.ReceivedQty, ShortageQty: r.ShortageQty, CostAmount: r.CostAmount, BatchReason: r.BatchReason, Note: r.Note,
	}
}

func (s *Store) insertTransferLines(ctx context.Context, transferID uuid.UUID, lines []domain.TransferLine) ([]domain.TransferLine, error) {
	out := make([]domain.TransferLine, 0, len(lines))
	for i, l := range lines {
		l.LineNo = i + 1
		id, err := s.q(ctx).InsertTransferLine(ctx, sqlcgen.InsertTransferLineParams{
			TransferID: transferID, LineNo: int32(l.LineNo), ProductID: l.ProductID, BatchID: ptrToNull(l.BatchID), //nolint:gosec // dibatasi domain
			Qty: l.Qty, ReceivedQty: l.ReceivedQty, ShortageQty: l.ShortageQty, CostAmount: l.CostAmount,
			BatchReason: l.BatchReason, Note: l.Note,
		})
		if err != nil {
			return nil, fmt.Errorf("insert transfer line: %w", err)
		}
		l.ID = id
		out = append(out, l)
	}
	return out, nil
}

// InsertTransfer menyimpan transfer draft beserta barisnya.
func (s *Store) InsertTransfer(ctx context.Context, t domain.StockTransfer, actorID uuid.UUID) (domain.StockTransfer, error) {
	r, err := s.q(ctx).InsertTransfer(ctx, sqlcgen.InsertTransferParams{
		TransferType: string(t.Type), FromLocationID: t.FromLocationID, ToLocationID: t.ToLocationID,
		RequiresShipping: t.RequiresShipping, BusinessDate: t.BusinessDate, Status: string(t.Status), SourceType: t.SourceType,
		SourceID: ptrToNull(t.SourceID), Note: t.Note, RequestedBy: t.RequestedBy, RequestedByName: t.RequestedByName, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_stock_transfers_source") {
		return domain.StockTransfer{}, apperr.Validation(map[string]string{"source": "Dokumen sumber ini sudah punya transfer"})
	}
	if err != nil {
		return domain.StockTransfer{}, fmt.Errorf("insert transfer: %w", err)
	}
	out := toTransfer(r)
	if out.Lines, err = s.insertTransferLines(ctx, r.ID, t.Lines); err != nil {
		return domain.StockTransfer{}, err
	}
	return out, nil
}

// GetTransfer membaca transfer beserta barisnya; forUpdate mengunci header.
func (s *Store) GetTransfer(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.StockTransfer, error) {
	var (
		r   sqlcgen.StockTransfer
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetTransferForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetTransfer(ctx, id)
	}
	if err != nil {
		return domain.StockTransfer{}, notFound(err, "get transfer")
	}
	return s.withTransferLines(ctx, toTransfer(r))
}

func (s *Store) withTransferLines(ctx context.Context, t domain.StockTransfer) (domain.StockTransfer, error) {
	rows, err := s.q(ctx).ListTransferLines(ctx, t.ID)
	if err != nil {
		return domain.StockTransfer{}, fmt.Errorf("list transfer lines: %w", err)
	}
	for _, r := range rows {
		t.Lines = append(t.Lines, toTransferLine(r))
	}
	return t, nil
}

// UpdateTransfer menyimpan header (optimistic lock) dan baris. replaceLines menulis ulang semua baris (draft
// atau pemecahan baris per batch saat dikirim); selain itu hanya qty diterima/kekurangan/nilai yang diubah.
func (s *Store) UpdateTransfer(ctx context.Context, t domain.StockTransfer, replaceLines bool, actorID uuid.UUID) (domain.StockTransfer, error) {
	q := s.q(ctx)
	r, err := q.UpdateTransfer(ctx, sqlcgen.UpdateTransferParams{
		ID: t.ID, Version: int32(t.Version), Number: nullText(t.Number), FromLocationID: t.FromLocationID, //nolint:gosec // versi kecil
		ToLocationID: t.ToLocationID, RequiresShipping: t.RequiresShipping, BusinessDate: t.BusinessDate,
		Status: string(t.Status), Note: t.Note, SubmittedAt: t.SubmittedAt,
		ReleasedBy: ptrToNull(t.ReleasedBy), ReleasedByName: t.ReleasedByName, ReleasedAt: t.ReleasedAt,
		PreparedBy: ptrToNull(t.PreparedBy), PreparedByName: t.PreparedByName, PreparedAt: t.PreparedAt,
		ShippedBy: ptrToNull(t.ShippedBy), ShippedByName: t.ShippedByName, ShippedAt: t.ShippedAt,
		ReceivedBy: ptrToNull(t.ReceivedBy), ReceivedByName: t.ReceivedByName, ReceivedAt: t.ReceivedAt,
		CompletedAt: t.CompletedAt, CancelledAt: t.CancelledAt, CancelReason: t.CancelReason, ActorID: actorID,
	})
	if err != nil {
		return domain.StockTransfer{}, versionConflict(err, "update transfer")
	}
	out := toTransfer(r)
	if replaceLines {
		if err := q.DeleteTransferLines(ctx, t.ID); err != nil {
			return domain.StockTransfer{}, fmt.Errorf("delete transfer lines: %w", err)
		}
		if out.Lines, err = s.insertTransferLines(ctx, t.ID, t.Lines); err != nil {
			return domain.StockTransfer{}, err
		}
		return out, nil
	}
	for _, l := range t.Lines {
		if err := q.UpdateTransferLine(ctx, sqlcgen.UpdateTransferLineParams{
			ID: l.ID, ReceivedQty: l.ReceivedQty, ShortageQty: l.ShortageQty, CostAmount: l.CostAmount,
		}); err != nil {
			return domain.StockTransfer{}, fmt.Errorf("update transfer line: %w", err)
		}
	}
	out.Lines = t.Lines
	return out, nil
}

// TransferFilter adalah kriteria daftar transfer.
type TransferFilter struct {
	Type       domain.TransferType
	Statuses   []string
	LocationID *uuid.UUID
	Search     string
}

// TransferSummary adalah satu baris daftar transfer.
type TransferSummary struct {
	Transfer  domain.StockTransfer
	LineCount int
	TotalQty  decimal.Decimal
}

// ListTransfers mengembalikan satu halaman transfer satu jenis.
func (s *Store) ListTransfers(ctx context.Context, f TransferFilter, page paging.Page) (paging.Result[TransferSummary], error) {
	q := s.q(ctx)
	statuses := f.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	rows, err := q.ListTransfers(ctx, sqlcgen.ListTransfersParams{
		TransferType: string(f.Type), Statuses: statuses, LocationID: ptrToNull(f.LocationID), Search: nullText(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[TransferSummary]{}, fmt.Errorf("list transfers: %w", err)
	}
	total, err := q.CountTransfers(ctx, sqlcgen.CountTransfersParams{
		TransferType: string(f.Type), Statuses: statuses, LocationID: ptrToNull(f.LocationID), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[TransferSummary]{}, fmt.Errorf("count transfers: %w", err)
	}
	items := make([]TransferSummary, len(rows))
	for i, r := range rows {
		items[i] = TransferSummary{
			Transfer: toTransfer(sqlcgen.StockTransfer{
				ID: r.ID, Number: r.Number, TransferType: r.TransferType, FromLocationID: r.FromLocationID,
				ToLocationID: r.ToLocationID, RequiresShipping: r.RequiresShipping, BusinessDate: r.BusinessDate,
				Status: r.Status, SourceType: r.SourceType, SourceID: r.SourceID, Note: r.Note, RequestedBy: r.RequestedBy,
				RequestedByName: r.RequestedByName, SubmittedAt: r.SubmittedAt, ReleasedBy: r.ReleasedBy,
				ReleasedByName: r.ReleasedByName, ReleasedAt: r.ReleasedAt, PreparedBy: r.PreparedBy,
				PreparedByName: r.PreparedByName, PreparedAt: r.PreparedAt, ShippedBy: r.ShippedBy, ShippedByName: r.ShippedByName,
				ShippedAt: r.ShippedAt, ReceivedBy: r.ReceivedBy, ReceivedByName: r.ReceivedByName, ReceivedAt: r.ReceivedAt,
				CompletedAt: r.CompletedAt, CancelledAt: r.CancelledAt, CancelReason: r.CancelReason, CreatedAt: r.CreatedAt,
				CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			LineCount: int(r.LineCount), TotalQty: r.TotalQty,
		}
	}
	return paging.Result[TransferSummary]{Items: items, Page: page, Total: total}, nil
}

// InTransitLine adalah satu baris transfer yang masih Dalam Pengiriman.
type InTransitLine struct {
	TransferID     uuid.UUID
	Number         string
	FromLocationID uuid.UUID
	ToLocationID   uuid.UUID
	BusinessDate   clock.BusinessDate
	ShippedAt      *time.Time
	Status         domain.Status
	LineID         uuid.UUID
	ProductID      uuid.UUID
	BatchID        *uuid.UUID
	Qty            decimal.Decimal
	ReceivedQty    decimal.Decimal
	CostAmount     decimal.Decimal
}

// ListInTransit mengembalikan baris transfer yang belum diterima penuh (Stok dalam Pengiriman).
func (s *Store) ListInTransit(ctx context.Context, locationID *uuid.UUID, page paging.Page) (paging.Result[InTransitLine], error) {
	q := s.q(ctx)
	rows, err := q.ListInTransitLines(ctx, sqlcgen.ListInTransitLinesParams{
		LocationID: ptrToNull(locationID), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[InTransitLine]{}, fmt.Errorf("list in transit: %w", err)
	}
	total, err := q.CountInTransitLines(ctx, ptrToNull(locationID))
	if err != nil {
		return paging.Result[InTransitLine]{}, fmt.Errorf("count in transit: %w", err)
	}
	items := make([]InTransitLine, len(rows))
	for i, r := range rows {
		items[i] = InTransitLine{
			TransferID: r.TransferID, Number: textOf(r.Number), FromLocationID: r.FromLocationID, ToLocationID: r.ToLocationID,
			BusinessDate: r.BusinessDate, ShippedAt: r.ShippedAt, Status: domain.Status(r.Status), LineID: r.LineID,
			ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), Qty: r.Qty, ReceivedQty: r.ReceivedQty, CostAmount: r.CostAmount,
		}
	}
	return paging.Result[InTransitLine]{Items: items, Page: page, Total: total}, nil
}

// --- Stock Count ------------------------------------------------------------------------------

func toStockCount(r sqlcgen.StockCount) domain.StockCount {
	return domain.StockCount{
		ID: r.ID, Number: textOf(r.Number), CountType: domain.CountType(r.CountType), LocationID: r.LocationID,
		ScheduleID: uuidPtr(r.ScheduleID), BusinessDate: r.BusinessDate, Status: domain.Status(r.Status),
		AssignedTo: uuidPtr(r.AssignedTo), AssignedToName: r.AssignedToName, Note: r.Note, PhotoFileIDs: r.PhotoFileIds,
		SnapshotAt: r.SnapshotAt, StartedBy: uuidPtr(r.StartedBy), StartedByName: r.StartedByName,
		SubmittedBy: uuidPtr(r.SubmittedBy), SubmittedByName: r.SubmittedByName, SubmittedAt: r.SubmittedAt,
		VarianceAmount: r.VarianceAmount, NetVariance: r.NetVariance, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
		PostedAt: r.PostedAt, JournalEntryID: uuidPtr(r.JournalEntryID), CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toCountLine(r sqlcgen.StockCountLine) domain.CountLine {
	return domain.CountLine{
		ID: r.ID, LineNo: int(r.LineNo), ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), SystemQty: r.SystemQty,
		UnitCost: r.UnitCost, CountedQty: decimalPtr(r.CountedQty), VarianceQty: r.VarianceQty, VarianceAmount: r.VarianceAmount,
		IsExtra: r.IsExtra, PhotoFileID: uuidPtr(r.PhotoFileID), Note: r.Note, CountedBy: uuidPtr(r.CountedBy), CountedAt: r.CountedAt,
	}
}

// ErrScheduleRunExists: stock count jadwal untuk lokasi & tanggal itu sudah dibuat (job diulang).
var ErrScheduleRunExists = errors.New("stock count jadwal sudah ada")

// InsertStockCount menyimpan stock count draft (tanpa baris).
func (s *Store) InsertStockCount(ctx context.Context, c domain.StockCount, actorID uuid.UUID) (domain.StockCount, error) {
	r, err := s.q(ctx).InsertStockCount(ctx, sqlcgen.InsertStockCountParams{
		CountType: string(c.CountType), LocationID: c.LocationID, ScheduleID: ptrToNull(c.ScheduleID),
		BusinessDate: c.BusinessDate, Status: string(c.Status), AssignedTo: ptrToNull(c.AssignedTo),
		AssignedToName: c.AssignedToName, Note: c.Note, ActorID: actorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StockCount{}, ErrScheduleRunExists
	}
	if err != nil {
		return domain.StockCount{}, fmt.Errorf("insert stock count: %w", err)
	}
	return toStockCount(r), nil
}

// GetStockCount membaca stock count beserta barisnya; forUpdate mengunci header.
func (s *Store) GetStockCount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.StockCount, error) {
	var (
		r   sqlcgen.StockCount
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetStockCountForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetStockCount(ctx, id)
	}
	if err != nil {
		return domain.StockCount{}, notFound(err, "get stock count")
	}
	c := toStockCount(r)
	rows, err := s.q(ctx).ListStockCountLines(ctx, id)
	if err != nil {
		return domain.StockCount{}, fmt.Errorf("list stock count lines: %w", err)
	}
	for _, l := range rows {
		c.Lines = append(c.Lines, toCountLine(l))
	}
	return c, nil
}

// LineWrite menentukan baris yang ditulis UpdateStockCount.
type LineWrite int

const (
	// LinesNone tidak mengubah baris.
	LinesNone LineWrite = iota
	// LinesReplace menulis ulang seluruh baris (snapshot saat mulai).
	LinesReplace
	// LinesChanged mengubah baris terpilih dan menyisipkan baris baru (ID kosong).
	LinesChanged
	// LinesAll mengubah semua baris (selisih saat submit).
	LinesAll
)

// UpdateStockCount menyimpan header (optimistic lock) dan baris sesuai mode.
func (s *Store) UpdateStockCount(ctx context.Context, c domain.StockCount, mode LineWrite, changed []int, actorID uuid.UUID) (domain.StockCount, error) {
	q := s.q(ctx)
	r, err := q.UpdateStockCount(ctx, sqlcgen.UpdateStockCountParams{
		ID: c.ID, Version: int32(c.Version), Number: nullText(c.Number), Status: string(c.Status), Note: c.Note, //nolint:gosec // versi kecil
		PhotoFileIds: uuids(c.PhotoFileIDs), SnapshotAt: c.SnapshotAt, StartedBy: ptrToNull(c.StartedBy),
		StartedByName: c.StartedByName, SubmittedBy: ptrToNull(c.SubmittedBy), SubmittedByName: c.SubmittedByName,
		SubmittedAt: c.SubmittedAt, VarianceAmount: c.VarianceAmount, NetVariance: c.NetVariance, DecidedAt: c.DecidedAt,
		DecisionNote: c.DecisionNote, PostedAt: c.PostedAt, JournalEntryID: ptrToNull(c.JournalEntryID), ActorID: actorID,
	})
	if err != nil {
		return domain.StockCount{}, versionConflict(err, "update stock count")
	}
	out := toStockCount(r)
	write := func(i int) error {
		l := &c.Lines[i]
		if l.ID == uuid.Nil {
			id, err := q.InsertStockCountLine(ctx, sqlcgen.InsertStockCountLineParams{
				StockCountID: c.ID, LineNo: int32(l.LineNo), ProductID: l.ProductID, BatchID: ptrToNull(l.BatchID), //nolint:gosec // dibatasi domain
				SystemQty: l.SystemQty, UnitCost: l.UnitCost, CountedQty: nullDecimal(l.CountedQty), VarianceQty: l.VarianceQty,
				VarianceAmount: l.VarianceAmount, IsExtra: l.IsExtra, PhotoFileID: ptrToNull(l.PhotoFileID), Note: l.Note,
				CountedBy: ptrToNull(l.CountedBy), CountedAt: l.CountedAt,
			})
			if isUniqueViolation(err, "uq_stock_count_lines_product_batch") {
				return apperr.Validation(map[string]string{"extraLines": "Produk dan batch ini sudah ada di lembar hitung"})
			}
			if err != nil {
				return fmt.Errorf("insert stock count line: %w", err)
			}
			l.ID = id
			return nil
		}
		if err := q.UpdateStockCountLine(ctx, sqlcgen.UpdateStockCountLineParams{
			ID: l.ID, CountedQty: nullDecimal(l.CountedQty), VarianceQty: l.VarianceQty, VarianceAmount: l.VarianceAmount,
			PhotoFileID: ptrToNull(l.PhotoFileID), Note: l.Note, CountedBy: ptrToNull(l.CountedBy), CountedAt: l.CountedAt,
		}); err != nil {
			return fmt.Errorf("update stock count line: %w", err)
		}
		return nil
	}
	switch mode {
	case LinesReplace:
		if err := q.DeleteStockCountLines(ctx, c.ID); err != nil {
			return domain.StockCount{}, fmt.Errorf("delete stock count lines: %w", err)
		}
		for i := range c.Lines {
			c.Lines[i].ID = uuid.Nil
			if err := write(i); err != nil {
				return domain.StockCount{}, err
			}
		}
	case LinesChanged:
		for _, i := range changed {
			if err := write(i); err != nil {
				return domain.StockCount{}, err
			}
		}
	case LinesAll:
		for i := range c.Lines {
			if err := write(i); err != nil {
				return domain.StockCount{}, err
			}
		}
	case LinesNone:
	}
	out.Lines = c.Lines
	return out, nil
}

// StockCountFilter adalah kriteria daftar stock count.
type StockCountFilter struct {
	Statuses   []string
	CountType  string
	LocationID *uuid.UUID
	Search     string
	// AllowedLocationIDs membatasi lokasi (scope Merchandiser) bila Scoped.
	Scoped             bool
	AllowedLocationIDs []uuid.UUID
}

// StockCountSummary adalah satu baris daftar stock count.
type StockCountSummary struct {
	Count        domain.StockCount
	LineCount    int
	CountedCount int
}

// ListStockCounts mengembalikan satu halaman stock count.
func (s *Store) ListStockCounts(ctx context.Context, f StockCountFilter, page paging.Page) (paging.Result[StockCountSummary], error) {
	q := s.q(ctx)
	statuses := f.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	rows, err := q.ListStockCounts(ctx, sqlcgen.ListStockCountsParams{
		Statuses: statuses, CountType: nullText(f.CountType), LocationID: ptrToNull(f.LocationID), Scoped: f.Scoped,
		AllowedLocationIds: uuids(f.AllowedLocationIDs), Search: nullText(f.Search), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[StockCountSummary]{}, fmt.Errorf("list stock counts: %w", err)
	}
	total, err := q.CountStockCounts(ctx, sqlcgen.CountStockCountsParams{
		Statuses: statuses, CountType: nullText(f.CountType), LocationID: ptrToNull(f.LocationID), Scoped: f.Scoped,
		AllowedLocationIds: uuids(f.AllowedLocationIDs), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[StockCountSummary]{}, fmt.Errorf("count stock counts: %w", err)
	}
	items := make([]StockCountSummary, len(rows))
	for i, r := range rows {
		items[i] = StockCountSummary{
			Count: toStockCount(sqlcgen.StockCount{
				ID: r.ID, Number: r.Number, CountType: r.CountType, LocationID: r.LocationID, ScheduleID: r.ScheduleID,
				BusinessDate: r.BusinessDate, Status: r.Status, AssignedTo: r.AssignedTo, AssignedToName: r.AssignedToName,
				Note: r.Note, PhotoFileIds: r.PhotoFileIds, SnapshotAt: r.SnapshotAt, StartedBy: r.StartedBy,
				StartedByName: r.StartedByName, SubmittedBy: r.SubmittedBy, SubmittedByName: r.SubmittedByName,
				SubmittedAt: r.SubmittedAt, VarianceAmount: r.VarianceAmount, NetVariance: r.NetVariance, DecidedAt: r.DecidedAt,
				DecisionNote: r.DecisionNote, PostedAt: r.PostedAt, JournalEntryID: r.JournalEntryID, CreatedAt: r.CreatedAt,
				CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			LineCount: int(r.LineCount), CountedCount: int(r.CountedCount),
		}
	}
	return paging.Result[StockCountSummary]{Items: items, Page: page, Total: total}, nil
}

// --- Jadwal Stock Count -----------------------------------------------------------------------

func toSchedule(r sqlcgen.StockCountSchedule) domain.CountSchedule {
	s := domain.CountSchedule{
		ID: r.ID, Name: r.Name, CountType: domain.CountType(r.CountType), Frequency: domain.Frequency(r.Frequency),
		DayOfWeek: intPtr(r.DayOfWeek), DayOfMonth: intPtr(r.DayOfMonth), MonthOfYear: intPtr(r.MonthOfYear),
		WarehouseID: uuidPtr(r.WarehouseID), LocationIDs: r.LocationIds, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		AssigneeName: r.AssigneeName, NextRunDate: r.NextRunDate, IsActive: r.IsActive, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
	if r.LastRunDate.Valid {
		d := clock.NewBusinessDate(r.LastRunDate.Time.Year(), r.LastRunDate.Time.Month(), r.LastRunDate.Time.Day())
		s.LastRunDate = &d
	}
	return s
}

func pgDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

// InsertSchedule menyimpan jadwal baru.
func (s *Store) InsertSchedule(ctx context.Context, sc domain.CountSchedule, actorID uuid.UUID) (domain.CountSchedule, error) {
	r, err := s.q(ctx).InsertCountSchedule(ctx, sqlcgen.InsertCountScheduleParams{
		Name: sc.Name, CountType: string(sc.CountType), Frequency: string(sc.Frequency), DayOfWeek: int4(sc.DayOfWeek),
		DayOfMonth: int4(sc.DayOfMonth), MonthOfYear: int4(sc.MonthOfYear), WarehouseID: ptrToNull(sc.WarehouseID),
		LocationIds: uuids(sc.LocationIDs), AssigneeUserID: ptrToNull(sc.AssigneeUserID), AssigneeName: sc.AssigneeName,
		NextRunDate: sc.NextRunDate, IsActive: sc.IsActive, ActorID: actorID,
	})
	if err != nil {
		return domain.CountSchedule{}, fmt.Errorf("insert schedule: %w", err)
	}
	return toSchedule(r), nil
}

// UpdateSchedule menyimpan jadwal (optimistic lock).
func (s *Store) UpdateSchedule(ctx context.Context, sc domain.CountSchedule, actorID uuid.UUID) (domain.CountSchedule, error) {
	r, err := s.q(ctx).UpdateCountSchedule(ctx, sqlcgen.UpdateCountScheduleParams{
		ID: sc.ID, Version: int32(sc.Version), Name: sc.Name, CountType: string(sc.CountType), Frequency: string(sc.Frequency), //nolint:gosec // versi kecil
		DayOfWeek: int4(sc.DayOfWeek), DayOfMonth: int4(sc.DayOfMonth), MonthOfYear: int4(sc.MonthOfYear),
		WarehouseID: ptrToNull(sc.WarehouseID), LocationIds: uuids(sc.LocationIDs), AssigneeUserID: ptrToNull(sc.AssigneeUserID),
		AssigneeName: sc.AssigneeName, NextRunDate: sc.NextRunDate, LastRunDate: pgDate(sc.LastRunDate), IsActive: sc.IsActive,
		ActorID: actorID,
	})
	if err != nil {
		return domain.CountSchedule{}, versionConflict(err, "update schedule")
	}
	return toSchedule(r), nil
}

// GetSchedule membaca jadwal.
func (s *Store) GetSchedule(ctx context.Context, id uuid.UUID) (domain.CountSchedule, error) {
	r, err := s.q(ctx).GetCountSchedule(ctx, id)
	if err != nil {
		return domain.CountSchedule{}, notFound(err, "get schedule")
	}
	return toSchedule(r), nil
}

// ListSchedules mengembalikan semua jadwal.
func (s *Store) ListSchedules(ctx context.Context) ([]domain.CountSchedule, error) {
	rows, err := s.q(ctx).ListCountSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	out := make([]domain.CountSchedule, len(rows))
	for i, r := range rows {
		out[i] = toSchedule(r)
	}
	return out, nil
}

// DueSchedules mengunci jadwal aktif yang jatuh tempo (SKIP LOCKED: aman bila job berjalan ganda).
func (s *Store) DueSchedules(ctx context.Context, today clock.BusinessDate) ([]domain.CountSchedule, error) {
	rows, err := s.q(ctx).DueCountSchedules(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("due schedules: %w", err)
	}
	out := make([]domain.CountSchedule, len(rows))
	for i, r := range rows {
		out[i] = toSchedule(r)
	}
	return out, nil
}
