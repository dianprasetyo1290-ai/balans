package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres/sqlcgen"
)

func toOutbound(r sqlcgen.Outbound) domain.Outbound {
	return domain.Outbound{
		ID: r.ID, Number: r.Number, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
		WarehouseID: r.WarehouseID, LocationID: r.LocationID, BusinessDate: r.BusinessDate, RecipientName: r.RecipientName,
		RecipientPhone: r.RecipientPhone, RecipientAddress: r.RecipientAddress, Status: domain.Status(r.Status), Note: r.Note,
		PreparedBy: uuidPtr(r.PreparedBy), PreparedByName: r.PreparedByName, PackedAt: r.PackedAt,
		CheckedBy: uuidPtr(r.CheckedBy), CheckedByName: r.CheckedByName, CompletedAt: r.CompletedAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toOutboundLine(r sqlcgen.OutboundLine) domain.OutboundLine {
	return domain.OutboundLine{
		ID: r.ID, LineNo: int(r.LineNo), SourceLineID: uuidPtr(r.SourceLineID), ProductID: r.ProductID,
		BatchID: uuidPtr(r.BatchID), AllocatedQty: r.AllocatedQty, PickedQty: r.PickedQty, PassedQty: r.PassedQty,
		RejectedQty: r.RejectedQty, Status: domain.OutboundLineStatus(r.Status), ReplacementOfLineID: uuidPtr(r.ReplacementOfLineID),
		RejectReason: r.RejectReason, ReturnLabel: r.ReturnLabel, CheckedBy: uuidPtr(r.CheckedBy), CheckedByName: r.CheckedByName,
		CheckedAt: r.CheckedAt,
	}
}

// InsertOutbound menyimpan Outbound beserta barisnya. Dokumen asal yang sudah punya Outbound ditolak.
func (s *Store) InsertOutbound(ctx context.Context, o domain.Outbound, actorID uuid.UUID) (domain.Outbound, error) {
	r, err := s.q(ctx).InsertOutbound(ctx, sqlcgen.InsertOutboundParams{
		Number: o.Number, SourceType: o.SourceType, SourceID: o.SourceID, SourceLabel: o.SourceLabel,
		WarehouseID: o.WarehouseID, LocationID: o.LocationID, BusinessDate: o.BusinessDate, RecipientName: o.RecipientName,
		RecipientPhone: o.RecipientPhone, RecipientAddress: o.RecipientAddress, Status: string(o.Status), Note: o.Note,
		ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_outbounds_source") {
		return domain.Outbound{}, apperr.Validation(map[string]string{"source.id": "Dokumen asal ini sudah punya Outbound"})
	}
	if err != nil {
		return domain.Outbound{}, fmt.Errorf("insert outbound: %w", err)
	}
	out := toOutbound(r)
	out.Lines = o.Lines
	if err := s.saveOutboundLines(ctx, &out); err != nil {
		return domain.Outbound{}, err
	}
	return out, nil
}

func (s *Store) saveOutboundLines(ctx context.Context, o *domain.Outbound) error {
	q := s.q(ctx)
	for i := range o.Lines {
		l := &o.Lines[i]
		if l.ID == uuid.Nil {
			id, err := q.InsertOutboundLine(ctx, sqlcgen.InsertOutboundLineParams{
				OutboundID: o.ID, LineNo: int32(l.LineNo), SourceLineID: nullUUID(l.SourceLineID), ProductID: l.ProductID, //nolint:gosec // dibatasi jumlah baris
				BatchID: nullUUID(l.BatchID), AllocatedQty: l.AllocatedQty, PickedQty: l.PickedQty, Status: string(l.Status),
				ReplacementOfLineID: nullUUID(l.ReplacementOfLineID),
			})
			if err != nil {
				return fmt.Errorf("insert outbound line: %w", err)
			}
			l.ID = id
			continue
		}
		if err := q.UpdateOutboundLine(ctx, sqlcgen.UpdateOutboundLineParams{
			ID: l.ID, BatchID: nullUUID(l.BatchID), AllocatedQty: l.AllocatedQty, PickedQty: l.PickedQty, PassedQty: l.PassedQty,
			RejectedQty: l.RejectedQty, Status: string(l.Status), RejectReason: l.RejectReason, ReturnLabel: l.ReturnLabel,
			CheckedBy: nullUUID(l.CheckedBy), CheckedByName: l.CheckedByName, CheckedAt: l.CheckedAt,
		}); err != nil {
			return fmt.Errorf("update outbound line: %w", err)
		}
	}
	return nil
}

// GetOutbound membaca Outbound beserta baris dan koli; forUpdate mengunci header.
func (s *Store) GetOutbound(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Outbound, error) {
	var (
		r   sqlcgen.Outbound
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetOutboundForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetOutbound(ctx, id)
	}
	if err != nil {
		return domain.Outbound{}, notFound(err, "get outbound")
	}
	return s.withOutboundDetail(ctx, toOutbound(r))
}

// OutboundBySource membaca Outbound milik dokumen asal.
func (s *Store) OutboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.Outbound, error) {
	r, err := s.q(ctx).GetOutboundBySource(ctx, sqlcgen.GetOutboundBySourceParams{SourceType: sourceType, SourceID: sourceID})
	if err != nil {
		return domain.Outbound{}, notFound(err, "get outbound by source")
	}
	return s.withOutboundDetail(ctx, toOutbound(r))
}

func (s *Store) withOutboundDetail(ctx context.Context, o domain.Outbound) (domain.Outbound, error) {
	rows, err := s.q(ctx).ListOutboundLines(ctx, o.ID)
	if err != nil {
		return domain.Outbound{}, fmt.Errorf("list outbound lines: %w", err)
	}
	for _, l := range rows {
		o.Lines = append(o.Lines, toOutboundLine(l))
	}
	pkgs, err := s.q(ctx).ListOutboundPackages(ctx, o.ID)
	if err != nil {
		return domain.Outbound{}, fmt.Errorf("list outbound packages: %w", err)
	}
	for _, p := range pkgs {
		pkg := domain.OutboundPackage{ID: p.ID, Number: int(p.PackageNumber), Count: int(p.PackageCount), WeightGrams: intPtr(p.WeightGrams)}
		if err := json.Unmarshal(p.Contents, &pkg.Contents); err != nil {
			return domain.Outbound{}, fmt.Errorf("decode package contents: %w", err)
		}
		o.Packages = append(o.Packages, pkg)
	}
	return o, nil
}

// OutboundWrite menentukan bagian Outbound yang ditulis ulang.
type OutboundWrite struct {
	Packages bool
	Checks   []domain.QualityCheck
}

// SaveOutbound menyimpan header (optimistic lock), baris baru/berubah, catatan QC, dan koli bila diminta.
func (s *Store) SaveOutbound(ctx context.Context, o domain.Outbound, w OutboundWrite, actorID uuid.UUID) (domain.Outbound, error) {
	q := s.q(ctx)
	if err := s.saveOutboundLines(ctx, &o); err != nil {
		return domain.Outbound{}, err
	}
	if err := s.insertChecks(ctx, w.Checks); err != nil {
		return domain.Outbound{}, err
	}
	if w.Packages {
		if err := q.DeleteOutboundPackages(ctx, o.ID); err != nil {
			return domain.Outbound{}, fmt.Errorf("delete outbound packages: %w", err)
		}
		for _, p := range o.Packages {
			raw, err := json.Marshal(p.Contents)
			if err != nil {
				return domain.Outbound{}, err
			}
			if err := q.InsertOutboundPackage(ctx, sqlcgen.InsertOutboundPackageParams{
				OutboundID: o.ID, PackageNumber: int32(p.Number), PackageCount: int32(p.Count), //nolint:gosec // maks. 500 koli
				WeightGrams: int4(p.WeightGrams), Contents: raw,
			}); err != nil {
				return domain.Outbound{}, fmt.Errorf("insert outbound package: %w", err)
			}
		}
	}
	r, err := q.UpdateOutbound(ctx, sqlcgen.UpdateOutboundParams{
		ID: o.ID, Version: int32(o.Version), Status: string(o.Status), PreparedBy: nullUUID(o.PreparedBy), //nolint:gosec // versi kecil
		PreparedByName: o.PreparedByName, PackedAt: o.PackedAt, CheckedBy: nullUUID(o.CheckedBy), CheckedByName: o.CheckedByName,
		CompletedAt: o.CompletedAt, CancelledAt: o.CancelledAt, CancelReason: o.CancelReason, ActorID: actorID,
	})
	if err != nil {
		return domain.Outbound{}, versionConflict(err, "update outbound")
	}
	return s.withOutboundDetail(ctx, toOutbound(r))
}

func (s *Store) insertChecks(ctx context.Context, checks []domain.QualityCheck) error {
	for _, c := range checks {
		raw, err := json.Marshal(c.Checks)
		if err != nil {
			return err
		}
		if err := s.q(ctx).InsertQualityCheck(ctx, sqlcgen.InsertQualityCheckParams{
			DocumentType: c.DocumentType, DocumentID: c.DocumentID, DocumentLineID: c.DocumentLineID, Result: c.Result,
			Qty: c.Qty, Checks: raw, Note: c.Note, PhotoFileIds: uuids(c.PhotoFileIDs), CheckedBy: c.CheckedBy,
			CheckedByName: c.CheckedByName, CheckedAt: c.CheckedAt,
		}); err != nil {
			return fmt.Errorf("insert quality check: %w", err)
		}
	}
	return nil
}

// OutboundFilter adalah kriteria daftar Outbound.
type OutboundFilter struct {
	Status     string
	SourceType string
	Search     string
}

// OutboundSummary adalah satu baris daftar Outbound.
type OutboundSummary struct {
	Outbound     domain.Outbound
	LineCount    int
	PendingCount int
	HeldCount    int
}

// ListOutbounds mengembalikan satu halaman Outbound terbaru.
func (s *Store) ListOutbounds(ctx context.Context, f OutboundFilter, page paging.Page) (paging.Result[OutboundSummary], error) {
	q := s.q(ctx)
	rows, err := q.ListOutbounds(ctx, sqlcgen.ListOutboundsParams{
		Status: nullText(f.Status), SourceType: nullText(f.SourceType), Search: nullText(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[OutboundSummary]{}, fmt.Errorf("list outbounds: %w", err)
	}
	total, err := q.CountOutbounds(ctx, sqlcgen.CountOutboundsParams{
		Status: nullText(f.Status), SourceType: nullText(f.SourceType), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[OutboundSummary]{}, fmt.Errorf("count outbounds: %w", err)
	}
	items := make([]OutboundSummary, len(rows))
	for i, r := range rows {
		items[i] = OutboundSummary{
			Outbound: toOutbound(sqlcgen.Outbound{
				ID: r.ID, Number: r.Number, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
				WarehouseID: r.WarehouseID, LocationID: r.LocationID, BusinessDate: r.BusinessDate, RecipientName: r.RecipientName,
				RecipientPhone: r.RecipientPhone, RecipientAddress: r.RecipientAddress, Status: r.Status, Note: r.Note,
				PreparedBy: r.PreparedBy, PreparedByName: r.PreparedByName, PackedAt: r.PackedAt, CheckedBy: r.CheckedBy,
				CheckedByName: r.CheckedByName, CompletedAt: r.CompletedAt, CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
				CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			LineCount: int(r.LineCount), PendingCount: int(r.PendingCount), HeldCount: int(r.HeldCount),
		}
	}
	return paging.Result[OutboundSummary]{Items: items, Page: page, Total: total}, nil
}
