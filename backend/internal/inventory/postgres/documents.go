package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres/sqlcgen"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// DocFilter adalah filter list dokumen persediaan.
type DocFilter struct {
	Status string
	// Kind: tujuan pemakaian atau alasan pemusnahan.
	Kind  string
	Query string
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func textString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func toStep(by uuid.NullUUID, name string, at *time.Time) *domain.Step {
	if !by.Valid || at == nil {
		return nil
	}
	return &domain.Step{By: by.UUID, Name: name, At: *at}
}

type stepCols struct {
	by   uuid.NullUUID
	name string
	at   *time.Time
}

func fromStep(s *domain.Step) stepCols {
	if s == nil {
		return stepCols{}
	}
	at := s.At
	return stepCols{by: uuid.NullUUID{UUID: s.By, Valid: true}, name: s.Name, at: &at}
}

func pageArgs(page paging.Page) (offset, limit int32) { return page.Offset(), page.Limit() }

// --- Pemakaian Internal ---------------------------------------------------------------------

func toUsage(r sqlcgen.InternalUsage) domain.InternalUsage {
	return domain.InternalUsage{
		ID: r.ID, Number: textString(r.Number), Purpose: domain.UsagePurpose(r.Purpose), DivisionID: r.DivisionID,
		CampaignID: uuidPtr(r.CampaignID), SourceLocationID: r.SourceLocationID, EventLocationID: uuidPtr(r.EventLocationID),
		EventEndDate: datePtr(r.EventEndDate), SettleDueDate: datePtr(r.SettleDueDate), BusinessDate: r.BusinessDate,
		Note: r.Note, Status: domain.DocStatus(r.Status), EstimatedCost: r.EstimatedCost, CostAmount: r.CostAmount,
		JournalID: uuidPtr(r.JournalID), ReturnDispositionID: uuidPtr(r.ReturnDispositionID),
		RequestedBy: domain.Actor{ID: r.RequestedBy, Name: r.RequestedByName},
		Released:    toStep(r.ReleasedBy, r.ReleasedByName, r.ReleasedAt),
		HandedOver:  toStep(r.HandedOverBy, r.HandedOverByName, r.HandedOverAt),
		Settled:     toStep(r.SettledBy, r.SettledByName, r.SettledAt),
		Posted:      toStep(r.PostedBy, r.PostedByName, r.PostedAt),
		RemindedOn:  datePtr(r.RemindedOn), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) attachUsageLines(ctx context.Context, docs []domain.InternalUsage) error {
	if len(docs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(docs))
	idx := map[uuid.UUID]int{}
	for i, d := range docs {
		ids[i], idx[d.ID] = d.ID, i
	}
	rows, err := s.q(ctx).ListInternalUsageLines(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		i := idx[r.InternalUsageID]
		docs[i].Lines = append(docs[i].Lines, domain.UsageLine{
			ID: r.ID, LineNo: int(r.LineNo), ProductID: r.ProductID, Qty: r.Qty, ReturnedQty: r.ReturnedQty,
			ConsumedQty: r.ConsumedQty, CostAmount: r.CostAmount, Note: r.Note,
		})
	}
	return nil
}

// InsertUsage menyimpan draft beserta barisnya.
func (s *Store) InsertUsage(ctx context.Context, u domain.InternalUsage) (domain.InternalUsage, error) {
	row, err := s.q(ctx).InsertInternalUsage(ctx, sqlcgen.InsertInternalUsageParams{
		Purpose: string(u.Purpose), DivisionID: u.DivisionID, CampaignID: nullUUID(u.CampaignID),
		SourceLocationID: u.SourceLocationID, EventLocationID: nullUUID(u.EventLocationID), EventEndDate: nullDate(u.EventEndDate),
		SettleDueDate: nullDate(u.SettleDueDate), BusinessDate: u.BusinessDate, Note: u.Note, Status: string(u.Status),
		ReturnDispositionID: nullUUID(u.ReturnDispositionID), RequestedBy: u.RequestedBy.ID, RequestedByName: u.RequestedBy.Name,
	})
	if err != nil {
		return domain.InternalUsage{}, err
	}
	u.ID, u.CreatedAt, u.UpdatedAt, u.Version = row.ID, row.CreatedAt, row.UpdatedAt, int(row.Version)
	if err := s.ReplaceUsageLines(ctx, &u); err != nil {
		return domain.InternalUsage{}, err
	}
	return u, nil
}

// ReplaceUsageLines mengganti seluruh baris draft.
func (s *Store) ReplaceUsageLines(ctx context.Context, u *domain.InternalUsage) error {
	q := s.q(ctx)
	if err := q.DeleteInternalUsageLines(ctx, u.ID); err != nil {
		return err
	}
	for i := range u.Lines {
		l := &u.Lines[i]
		id, err := q.InsertInternalUsageLine(ctx, sqlcgen.InsertInternalUsageLineParams{
			InternalUsageID: u.ID, LineNo: int32(l.LineNo), ProductID: l.ProductID, Qty: l.Qty, Note: l.Note, //nolint:gosec // maks. 100 baris
		})
		if err != nil {
			return err
		}
		l.ID = id
	}
	return nil
}

// GetUsage membaca dokumen beserta barisnya; lock = SELECT … FOR UPDATE.
func (s *Store) GetUsage(ctx context.Context, id uuid.UUID, lock bool) (domain.InternalUsage, error) {
	q := s.q(ctx)
	var (
		r   sqlcgen.InternalUsage
		err error
	)
	if lock {
		r, err = q.LockInternalUsage(ctx, id)
	} else {
		r, err = q.GetInternalUsage(ctx, id)
	}
	if err != nil {
		return domain.InternalUsage{}, notFound(err, "get internal usage")
	}
	docs := []domain.InternalUsage{toUsage(r)}
	if err := s.attachUsageLines(ctx, docs); err != nil {
		return domain.InternalUsage{}, err
	}
	return docs[0], nil
}

// SaveUsage menyimpan header dan hasil per baris (returned/consumed/cost); versi naik satu.
func (s *Store) SaveUsage(ctx context.Context, u *domain.InternalUsage, actorID uuid.UUID) error {
	q := s.q(ctx)
	rel, ho, st, po := fromStep(u.Released), fromStep(u.HandedOver), fromStep(u.Settled), fromStep(u.Posted)
	row, err := q.UpdateInternalUsage(ctx, sqlcgen.UpdateInternalUsageParams{
		Number: text(u.Number), Purpose: string(u.Purpose), DivisionID: u.DivisionID, CampaignID: nullUUID(u.CampaignID),
		SourceLocationID: u.SourceLocationID, EventLocationID: nullUUID(u.EventLocationID), EventEndDate: nullDate(u.EventEndDate),
		SettleDueDate: nullDate(u.SettleDueDate), BusinessDate: u.BusinessDate, Note: u.Note, Status: string(u.Status),
		EstimatedCost: u.EstimatedCost, CostAmount: u.CostAmount, JournalID: nullUUID(u.JournalID),
		ReleasedBy: rel.by, ReleasedByName: rel.name, ReleasedAt: rel.at,
		HandedOverBy: ho.by, HandedOverByName: ho.name, HandedOverAt: ho.at,
		SettledBy: st.by, SettledByName: st.name, SettledAt: st.at,
		PostedBy: po.by, PostedByName: po.name, PostedAt: po.at,
		RemindedOn: nullDate(u.RemindedOn), ActorID: actorID, ID: u.ID,
	})
	if err != nil {
		return err
	}
	u.UpdatedAt, u.Version = row.UpdatedAt, int(row.Version)
	for _, l := range u.Lines {
		if err := q.UpdateInternalUsageLine(ctx, sqlcgen.UpdateInternalUsageLineParams{
			ReturnedQty: l.ReturnedQty, ConsumedQty: l.ConsumedQty, CostAmount: l.CostAmount, ID: l.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListUsages mengembalikan daftar Pemakaian Internal.
func (s *Store) ListUsages(ctx context.Context, f DocFilter, page paging.Page) (paging.Result[domain.InternalUsage], error) {
	q := s.q(ctx)
	off, lim := pageArgs(page)
	rows, err := q.ListInternalUsages(ctx, sqlcgen.ListInternalUsagesParams{
		Status: text(f.Status), Purpose: text(f.Kind), Q: text(f.Query), RowOffset: off, RowLimit: lim,
	})
	if err != nil {
		return paging.Result[domain.InternalUsage]{}, err
	}
	total, err := q.CountInternalUsages(ctx, sqlcgen.CountInternalUsagesParams{Status: text(f.Status), Purpose: text(f.Kind), Q: text(f.Query)})
	if err != nil {
		return paging.Result[domain.InternalUsage]{}, err
	}
	docs := make([]domain.InternalUsage, len(rows))
	for i, r := range rows {
		docs[i] = toUsage(r)
	}
	if err := s.attachUsageLines(ctx, docs); err != nil {
		return paging.Result[domain.InternalUsage]{}, err
	}
	return paging.Result[domain.InternalUsage]{Items: docs, Total: total}, nil
}

// SettlementDueUsages mengembalikan pemakaian event yang lewat batas pencatatan sisa dan belum diingatkan hari ini.
func (s *Store) SettlementDueUsages(ctx context.Context, today clock.BusinessDate) ([]uuid.UUID, error) {
	return s.q(ctx).SettlementDueUsages(ctx, today)
}

// --- Repacking --------------------------------------------------------------------------------

func toRepacking(r sqlcgen.Repacking) domain.Repacking {
	return domain.Repacking{
		ID: r.ID, Number: textString(r.Number), BusinessDate: r.BusinessDate, Note: r.Note, Status: domain.DocStatus(r.Status),
		CostAmount: r.CostAmount, JournalID: uuidPtr(r.JournalID), ReturnDispositionID: uuidPtr(r.ReturnDispositionID),
		CreatedBy: domain.Actor{ID: r.CreatedBy, Name: r.CreatedByName},
		Started:   toStep(r.StartedBy, r.StartedByName, r.StartedAt), Posted: toStep(r.PostedBy, r.PostedByName, r.PostedAt),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) attachRepackLines(ctx context.Context, docs []domain.Repacking) error {
	if len(docs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(docs))
	idx := map[uuid.UUID]int{}
	for i, d := range docs {
		ids[i], idx[d.ID] = d.ID, i
	}
	rows, err := s.q(ctx).ListRepackingLines(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		i := idx[r.RepackingID]
		docs[i].Lines = append(docs[i].Lines, domain.RepackLine{
			ID: r.ID, LineNo: int(r.LineNo), Direction: domain.RepackDirection(r.Direction), ProductID: r.ProductID,
			LocationID: r.LocationID, StockStatus: domain.StockStatus(r.StockStatus), BatchID: uuidPtr(r.BatchID),
			BatchNumber: r.BatchNumber, ProductionDate: datePtr(r.ProductionDate), ExpiryDate: datePtr(r.ExpiryDate),
			Qty: r.Qty, CostAmount: r.CostAmount, Note: r.Note,
		})
	}
	return nil
}

// InsertRepacking menyimpan draft beserta barisnya.
func (s *Store) InsertRepacking(ctx context.Context, r domain.Repacking) (domain.Repacking, error) {
	row, err := s.q(ctx).InsertRepacking(ctx, sqlcgen.InsertRepackingParams{
		BusinessDate: r.BusinessDate, Note: r.Note, Status: string(r.Status), ReturnDispositionID: nullUUID(r.ReturnDispositionID),
		CreatedBy: r.CreatedBy.ID, CreatedByName: r.CreatedBy.Name,
	})
	if err != nil {
		return domain.Repacking{}, err
	}
	r.ID, r.CreatedAt, r.UpdatedAt, r.Version = row.ID, row.CreatedAt, row.UpdatedAt, int(row.Version)
	if err := s.ReplaceRepackLines(ctx, &r); err != nil {
		return domain.Repacking{}, err
	}
	return r, nil
}

// ReplaceRepackLines mengganti seluruh baris draft.
func (s *Store) ReplaceRepackLines(ctx context.Context, r *domain.Repacking) error {
	q := s.q(ctx)
	if err := q.DeleteRepackingLines(ctx, r.ID); err != nil {
		return err
	}
	for i := range r.Lines {
		l := &r.Lines[i]
		id, err := q.InsertRepackingLine(ctx, sqlcgen.InsertRepackingLineParams{
			RepackingID: r.ID, LineNo: int32(l.LineNo), Direction: string(l.Direction), ProductID: l.ProductID, //nolint:gosec // maks. 100 baris
			LocationID: l.LocationID, StockStatus: string(l.StockStatus), BatchID: nullUUID(l.BatchID),
			BatchNumber: l.BatchNumber, ProductionDate: nullDate(l.ProductionDate), ExpiryDate: nullDate(l.ExpiryDate),
			Qty: l.Qty, Note: l.Note,
		})
		if err != nil {
			return err
		}
		l.ID = id
	}
	return nil
}

// GetRepacking membaca dokumen beserta barisnya.
func (s *Store) GetRepacking(ctx context.Context, id uuid.UUID, lock bool) (domain.Repacking, error) {
	q := s.q(ctx)
	var (
		r   sqlcgen.Repacking
		err error
	)
	if lock {
		r, err = q.LockRepacking(ctx, id)
	} else {
		r, err = q.GetRepacking(ctx, id)
	}
	if err != nil {
		return domain.Repacking{}, notFound(err, "get repacking")
	}
	docs := []domain.Repacking{toRepacking(r)}
	if err := s.attachRepackLines(ctx, docs); err != nil {
		return domain.Repacking{}, err
	}
	return docs[0], nil
}

// SaveRepacking menyimpan header dan hasil per baris (batch hasil, HPP).
func (s *Store) SaveRepacking(ctx context.Context, r *domain.Repacking, actorID uuid.UUID) error {
	q := s.q(ctx)
	st, po := fromStep(r.Started), fromStep(r.Posted)
	row, err := q.UpdateRepacking(ctx, sqlcgen.UpdateRepackingParams{
		Number: text(r.Number), BusinessDate: r.BusinessDate, Note: r.Note, Status: string(r.Status), CostAmount: r.CostAmount,
		JournalID: nullUUID(r.JournalID), StartedBy: st.by, StartedByName: st.name, StartedAt: st.at,
		PostedBy: po.by, PostedByName: po.name, PostedAt: po.at, ActorID: actorID, ID: r.ID,
	})
	if err != nil {
		return err
	}
	r.UpdatedAt, r.Version = row.UpdatedAt, int(row.Version)
	for _, l := range r.Lines {
		if err := q.UpdateRepackingLine(ctx, sqlcgen.UpdateRepackingLineParams{
			BatchID: nullUUID(l.BatchID), CostAmount: l.CostAmount, ID: l.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListRepackings mengembalikan daftar Repacking.
func (s *Store) ListRepackings(ctx context.Context, f DocFilter, page paging.Page) (paging.Result[domain.Repacking], error) {
	q := s.q(ctx)
	off, lim := pageArgs(page)
	rows, err := q.ListRepackings(ctx, sqlcgen.ListRepackingsParams{Status: text(f.Status), Q: text(f.Query), RowOffset: off, RowLimit: lim})
	if err != nil {
		return paging.Result[domain.Repacking]{}, err
	}
	total, err := q.CountRepackings(ctx, sqlcgen.CountRepackingsParams{Status: text(f.Status), Q: text(f.Query)})
	if err != nil {
		return paging.Result[domain.Repacking]{}, err
	}
	docs := make([]domain.Repacking, len(rows))
	for i, r := range rows {
		docs[i] = toRepacking(r)
	}
	if err := s.attachRepackLines(ctx, docs); err != nil {
		return paging.Result[domain.Repacking]{}, err
	}
	return paging.Result[domain.Repacking]{Items: docs, Total: total}, nil
}

// --- Pemusnahan -------------------------------------------------------------------------------

func toDisposal(r sqlcgen.Disposal) domain.Disposal {
	witnesses, photos := r.Witnesses, r.PhotoFileIds
	if witnesses == nil {
		witnesses = []string{}
	}
	if photos == nil {
		photos = []uuid.UUID{}
	}
	return domain.Disposal{
		ID: r.ID, Number: textString(r.Number), Reason: domain.DisposalReason(r.Reason), DisposalDate: r.DisposalDate,
		Method: r.Method, Witnesses: witnesses, PhotoFileIDs: photos, Note: r.Note, Status: domain.DocStatus(r.Status),
		EstimatedCost: r.EstimatedCost, CostAmount: r.CostAmount, PostingRuleCode: r.PostingRuleCode,
		JournalID: uuidPtr(r.JournalID), ReturnDispositionID: uuidPtr(r.ReturnDispositionID),
		CreatedBy: domain.Actor{ID: r.CreatedBy, Name: r.CreatedByName},
		Submitted: toStep(r.SubmittedBy, r.SubmittedByName, r.SubmittedAt), Posted: toStep(r.PostedBy, r.PostedByName, r.PostedAt),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) attachDisposalLines(ctx context.Context, docs []domain.Disposal) error {
	if len(docs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(docs))
	idx := map[uuid.UUID]int{}
	for i, d := range docs {
		ids[i], idx[d.ID] = d.ID, i
	}
	rows, err := s.q(ctx).ListDisposalLines(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		i := idx[r.DisposalID]
		docs[i].Lines = append(docs[i].Lines, domain.DisposalLine{
			ID: r.ID, LineNo: int(r.LineNo), ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), LocationID: r.LocationID,
			StockStatus: domain.StockStatus(r.StockStatus), Qty: r.Qty, CostAmount: r.CostAmount, Note: r.Note,
		})
	}
	return nil
}

// InsertDisposal menyimpan dokumen beserta barisnya.
func (s *Store) InsertDisposal(ctx context.Context, d domain.Disposal) (domain.Disposal, error) {
	row, err := s.q(ctx).InsertDisposal(ctx, sqlcgen.InsertDisposalParams{
		Reason: string(d.Reason), DisposalDate: d.DisposalDate, Method: d.Method, Witnesses: d.Witnesses,
		PhotoFileIds: d.PhotoFileIDs, Note: d.Note, Status: string(d.Status), ReturnDispositionID: nullUUID(d.ReturnDispositionID),
		CreatedBy: d.CreatedBy.ID, CreatedByName: d.CreatedBy.Name,
	})
	if err != nil {
		return domain.Disposal{}, err
	}
	d.ID, d.CreatedAt, d.UpdatedAt, d.Version = row.ID, row.CreatedAt, row.UpdatedAt, int(row.Version)
	if err := s.ReplaceDisposalLines(ctx, &d); err != nil {
		return domain.Disposal{}, err
	}
	return d, nil
}

// ReplaceDisposalLines mengganti seluruh baris draft.
func (s *Store) ReplaceDisposalLines(ctx context.Context, d *domain.Disposal) error {
	q := s.q(ctx)
	if err := q.DeleteDisposalLines(ctx, d.ID); err != nil {
		return err
	}
	for i := range d.Lines {
		l := &d.Lines[i]
		id, err := q.InsertDisposalLine(ctx, sqlcgen.InsertDisposalLineParams{
			DisposalID: d.ID, LineNo: int32(l.LineNo), ProductID: l.ProductID, BatchID: nullUUID(l.BatchID), //nolint:gosec // maks. 100 baris
			LocationID: l.LocationID, StockStatus: string(l.StockStatus), Qty: l.Qty, Note: l.Note,
		})
		if err != nil {
			return err
		}
		l.ID = id
	}
	return nil
}

// GetDisposal membaca dokumen beserta barisnya.
func (s *Store) GetDisposal(ctx context.Context, id uuid.UUID, lock bool) (domain.Disposal, error) {
	q := s.q(ctx)
	var (
		r   sqlcgen.Disposal
		err error
	)
	if lock {
		r, err = q.LockDisposal(ctx, id)
	} else {
		r, err = q.GetDisposal(ctx, id)
	}
	if err != nil {
		return domain.Disposal{}, notFound(err, "get disposal")
	}
	docs := []domain.Disposal{toDisposal(r)}
	if err := s.attachDisposalLines(ctx, docs); err != nil {
		return domain.Disposal{}, err
	}
	return docs[0], nil
}

// SaveDisposal menyimpan header dan HPP per baris.
func (s *Store) SaveDisposal(ctx context.Context, d *domain.Disposal, actorID uuid.UUID) error {
	q := s.q(ctx)
	sub, po := fromStep(d.Submitted), fromStep(d.Posted)
	row, err := q.UpdateDisposal(ctx, sqlcgen.UpdateDisposalParams{
		Number: text(d.Number), Reason: string(d.Reason), DisposalDate: d.DisposalDate, Method: d.Method, Witnesses: d.Witnesses,
		PhotoFileIds: d.PhotoFileIDs, Note: d.Note, Status: string(d.Status), EstimatedCost: d.EstimatedCost,
		CostAmount: d.CostAmount, PostingRuleCode: d.PostingRuleCode, JournalID: nullUUID(d.JournalID),
		SubmittedBy: sub.by, SubmittedByName: sub.name, SubmittedAt: sub.at, PostedBy: po.by, PostedByName: po.name,
		PostedAt: po.at, ActorID: actorID, ID: d.ID,
	})
	if err != nil {
		return err
	}
	d.UpdatedAt, d.Version = row.UpdatedAt, int(row.Version)
	for _, l := range d.Lines {
		if err := q.UpdateDisposalLine(ctx, sqlcgen.UpdateDisposalLineParams{CostAmount: l.CostAmount, ID: l.ID}); err != nil {
			return err
		}
	}
	return nil
}

// ListDisposals mengembalikan daftar Pemusnahan.
func (s *Store) ListDisposals(ctx context.Context, f DocFilter, page paging.Page) (paging.Result[domain.Disposal], error) {
	q := s.q(ctx)
	off, lim := pageArgs(page)
	rows, err := q.ListDisposals(ctx, sqlcgen.ListDisposalsParams{
		Status: text(f.Status), Reason: text(f.Kind), Q: text(f.Query), RowOffset: off, RowLimit: lim,
	})
	if err != nil {
		return paging.Result[domain.Disposal]{}, err
	}
	total, err := q.CountDisposals(ctx, sqlcgen.CountDisposalsParams{Status: text(f.Status), Reason: text(f.Kind), Q: text(f.Query)})
	if err != nil {
		return paging.Result[domain.Disposal]{}, err
	}
	docs := make([]domain.Disposal, len(rows))
	for i, r := range rows {
		docs[i] = toDisposal(r)
	}
	if err := s.attachDisposalLines(ctx, docs); err != nil {
		return paging.Result[domain.Disposal]{}, err
	}
	return paging.Result[domain.Disposal]{Items: docs, Total: total}, nil
}

// --- Disposisi Retur --------------------------------------------------------------------------

func toDisposition(r sqlcgen.ReturnDisposition) domain.ReturnDisposition {
	return domain.ReturnDisposition{
		ID: r.ID, Number: textString(r.Number), LocationID: r.LocationID, InboundID: uuidPtr(r.InboundID),
		BusinessDate: r.BusinessDate, Note: r.Note, Status: domain.DocStatus(r.Status), DisposalValue: r.DisposalValue,
		CostAmount: r.CostAmount, JournalID: uuidPtr(r.JournalID), CreatedBy: domain.Actor{ID: r.CreatedBy, Name: r.CreatedByName},
		Submitted: toStep(r.SubmittedBy, r.SubmittedByName, r.SubmittedAt), Posted: toStep(r.PostedBy, r.PostedByName, r.PostedAt),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) attachDispositionLines(ctx context.Context, docs []domain.ReturnDisposition) error {
	if len(docs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(docs))
	idx := map[uuid.UUID]int{}
	for i, d := range docs {
		ids[i], idx[d.ID] = d.ID, i
	}
	rows, err := s.q(ctx).ListReturnDispositionLines(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		i := idx[r.ReturnDispositionID]
		docs[i].Lines = append(docs[i].Lines, domain.DispositionLine{
			ID: r.ID, LineNo: int(r.LineNo), ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID),
			StockStatus: domain.StockStatus(r.StockStatus), Qty: r.Qty, ReasonID: uuidPtr(r.ReasonID),
			Disposition: domain.Disposition(r.Disposition), QCNote: r.QcNote, EstimatedCost: r.EstimatedCost,
			CostAmount: r.CostAmount, TargetType: r.TargetType, TargetID: uuidPtr(r.TargetID),
		})
	}
	return nil
}

// InsertDisposition menyimpan draft beserta barisnya.
func (s *Store) InsertDisposition(ctx context.Context, d domain.ReturnDisposition) (domain.ReturnDisposition, error) {
	row, err := s.q(ctx).InsertReturnDisposition(ctx, sqlcgen.InsertReturnDispositionParams{
		LocationID: d.LocationID, InboundID: nullUUID(d.InboundID), BusinessDate: d.BusinessDate, Note: d.Note,
		Status: string(d.Status), CreatedBy: d.CreatedBy.ID, CreatedByName: d.CreatedBy.Name,
	})
	if err != nil {
		return domain.ReturnDisposition{}, err
	}
	d.ID, d.CreatedAt, d.UpdatedAt, d.Version = row.ID, row.CreatedAt, row.UpdatedAt, int(row.Version)
	if err := s.ReplaceDispositionLines(ctx, &d); err != nil {
		return domain.ReturnDisposition{}, err
	}
	return d, nil
}

// ReplaceDispositionLines mengganti seluruh baris draft.
func (s *Store) ReplaceDispositionLines(ctx context.Context, d *domain.ReturnDisposition) error {
	q := s.q(ctx)
	if err := q.DeleteReturnDispositionLines(ctx, d.ID); err != nil {
		return err
	}
	for i := range d.Lines {
		l := &d.Lines[i]
		id, err := q.InsertReturnDispositionLine(ctx, sqlcgen.InsertReturnDispositionLineParams{
			ReturnDispositionID: d.ID, LineNo: int32(l.LineNo), ProductID: l.ProductID, BatchID: nullUUID(l.BatchID), //nolint:gosec // maks. 100 baris
			StockStatus: string(l.StockStatus), Qty: l.Qty, ReasonID: nullUUID(l.ReasonID), Disposition: string(l.Disposition),
			QcNote: l.QCNote,
		})
		if err != nil {
			return err
		}
		l.ID = id
	}
	return nil
}

// GetDisposition membaca dokumen beserta barisnya.
func (s *Store) GetDisposition(ctx context.Context, id uuid.UUID, lock bool) (domain.ReturnDisposition, error) {
	q := s.q(ctx)
	var (
		r   sqlcgen.ReturnDisposition
		err error
	)
	if lock {
		r, err = q.LockReturnDisposition(ctx, id)
	} else {
		r, err = q.GetReturnDisposition(ctx, id)
	}
	if err != nil {
		return domain.ReturnDisposition{}, notFound(err, "get return disposition")
	}
	docs := []domain.ReturnDisposition{toDisposition(r)}
	if err := s.attachDispositionLines(ctx, docs); err != nil {
		return domain.ReturnDisposition{}, err
	}
	return docs[0], nil
}

// SaveDisposition menyimpan header dan hasil per baris.
func (s *Store) SaveDisposition(ctx context.Context, d *domain.ReturnDisposition, actorID uuid.UUID) error {
	q := s.q(ctx)
	sub, po := fromStep(d.Submitted), fromStep(d.Posted)
	row, err := q.UpdateReturnDisposition(ctx, sqlcgen.UpdateReturnDispositionParams{
		Number: text(d.Number), LocationID: d.LocationID, InboundID: nullUUID(d.InboundID), BusinessDate: d.BusinessDate,
		Note: d.Note, Status: string(d.Status), DisposalValue: d.DisposalValue, CostAmount: d.CostAmount,
		JournalID: nullUUID(d.JournalID), SubmittedBy: sub.by, SubmittedByName: sub.name, SubmittedAt: sub.at,
		PostedBy: po.by, PostedByName: po.name, PostedAt: po.at, ActorID: actorID, ID: d.ID,
	})
	if err != nil {
		return err
	}
	d.UpdatedAt, d.Version = row.UpdatedAt, int(row.Version)
	for _, l := range d.Lines {
		if err := q.UpdateReturnDispositionLine(ctx, sqlcgen.UpdateReturnDispositionLineParams{
			EstimatedCost: l.EstimatedCost, CostAmount: l.CostAmount, TargetType: l.TargetType, TargetID: nullUUID(l.TargetID),
			ID: l.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListDispositions mengembalikan daftar Disposisi Retur.
func (s *Store) ListDispositions(ctx context.Context, f DocFilter, page paging.Page) (paging.Result[domain.ReturnDisposition], error) {
	q := s.q(ctx)
	off, lim := pageArgs(page)
	rows, err := q.ListReturnDispositions(ctx, sqlcgen.ListReturnDispositionsParams{
		Status: text(f.Status), Q: text(f.Query), RowOffset: off, RowLimit: lim,
	})
	if err != nil {
		return paging.Result[domain.ReturnDisposition]{}, err
	}
	total, err := q.CountReturnDispositions(ctx, sqlcgen.CountReturnDispositionsParams{Status: text(f.Status), Q: text(f.Query)})
	if err != nil {
		return paging.Result[domain.ReturnDisposition]{}, err
	}
	docs := make([]domain.ReturnDisposition, len(rows))
	for i, r := range rows {
		docs[i] = toDisposition(r)
	}
	if err := s.attachDispositionLines(ctx, docs); err != nil {
		return paging.Result[domain.ReturnDisposition]{}, err
	}
	return paging.Result[domain.ReturnDisposition]{Items: docs, Total: total}, nil
}

// --- Movement per dokumen sumber & recost -----------------------------------------------------

// SourceMovement adalah movement satu dokumen sumber (batch yang dikirim/keluar per baris).
type SourceMovement struct {
	ID           uuid.UUID
	SourceLineID *uuid.UUID
	Type         domain.MovementType
	ProductID    uuid.UUID
	LocationID   uuid.UUID
	BatchID      *uuid.UUID
	BatchNumber  string
	ExpiryDate   *clock.BusinessDate
	Status       domain.StockStatus
	Qty          decimal.Decimal
	CostAmount   decimal.Decimal
}

// SourceMovements membaca movement dokumen sumber; action kosong = semua aksi.
func (s *Store) SourceMovements(ctx context.Context, sourceType string, sourceID uuid.UUID, action string) ([]SourceMovement, error) {
	rows, err := s.q(ctx).SourceMovements(ctx, sqlcgen.SourceMovementsParams{SourceType: sourceType, SourceID: sourceID, Action: text(action)})
	if err != nil {
		return nil, err
	}
	out := make([]SourceMovement, len(rows))
	for i, r := range rows {
		out[i] = SourceMovement{
			ID: r.ID, SourceLineID: uuidPtr(r.SourceLineID), Type: domain.MovementType(r.MovementType), ProductID: r.ProductID,
			LocationID: r.LocationID, BatchID: uuidPtr(r.BatchID), BatchNumber: textString(r.BatchNumber),
			ExpiryDate: datePtr(r.ExpiryDate), Status: domain.StockStatus(r.StockStatus), Qty: r.Qty, CostAmount: r.CostAmount,
		}
	}
	return out, nil
}

// RecostRun adalah satu proses recost.
type RecostRun struct {
	ID                 uuid.UUID
	ProductID          uuid.UUID
	BatchID            *uuid.UUID
	FromBusinessDate   clock.BusinessDate
	TriggerSourceType  string
	TriggerSourceID    uuid.UUID
	TriggerSourceLabel string
	Status             string
	AdjustmentAmount   decimal.Decimal
	JournalID          *uuid.UUID
	Error              string
	CreatedAt          time.Time
	CompletedAt        *time.Time
}

func toRun(r sqlcgen.RecostRun) RecostRun {
	return RecostRun{
		ID: r.ID, ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), FromBusinessDate: r.FromBusinessDate,
		TriggerSourceType: r.TriggerSourceType, TriggerSourceID: r.TriggerSourceID, TriggerSourceLabel: r.TriggerSourceLabel,
		Status: r.Status, AdjustmentAmount: r.AdjustmentAmount, JournalID: uuidPtr(r.AdjustmentJournalID), Error: r.Error,
		CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt,
	}
}

// HasIssuesSince memeriksa apakah batch sudah punya barang keluar pada/sesudah tanggal.
func (s *Store) HasIssuesSince(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID, from clock.BusinessDate) (bool, error) {
	return s.q(ctx).HasIssuesSince(ctx, sqlcgen.HasIssuesSinceParams{ProductID: productID, BatchID: nullUUID(batchID), FromDate: from})
}

// InsertRecostRun mencatat permintaan recost.
func (s *Store) InsertRecostRun(ctx context.Context, r RecostRun, actorID uuid.UUID) (uuid.UUID, error) {
	return s.q(ctx).InsertRecostRun(ctx, sqlcgen.InsertRecostRunParams{
		ProductID: r.ProductID, BatchID: nullUUID(r.BatchID), FromBusinessDate: r.FromBusinessDate,
		TriggerSourceType: r.TriggerSourceType, TriggerSourceID: r.TriggerSourceID, TriggerSourceLabel: r.TriggerSourceLabel,
		ActorID: actorID,
	})
}

// GetRecostRun membaca run; lock = FOR UPDATE.
func (s *Store) GetRecostRun(ctx context.Context, id uuid.UUID, lock bool) (RecostRun, error) {
	var (
		r   sqlcgen.RecostRun
		err error
	)
	if lock {
		r, err = s.q(ctx).LockRecostRun(ctx, id)
	} else {
		r, err = s.q(ctx).GetRecostRun(ctx, id)
	}
	if err != nil {
		return RecostRun{}, notFound(err, "get recost run")
	}
	return toRun(r), nil
}

// FinishRecostRun menyimpan hasil run.
func (s *Store) FinishRecostRun(ctx context.Context, id uuid.UUID, status string, amount decimal.Decimal, journalID *uuid.UUID, errText string) error {
	return s.q(ctx).FinishRecostRun(ctx, sqlcgen.FinishRecostRunParams{
		Status: status, AdjustmentAmount: amount, AdjustmentJournalID: nullUUID(journalID), Error: errText, ID: id,
	})
}

// ListRecostRuns mengembalikan riwayat recost.
func (s *Store) ListRecostRuns(ctx context.Context, productID *uuid.UUID, page paging.Page) (paging.Result[RecostRun], error) {
	off, lim := pageArgs(page)
	rows, err := s.q(ctx).ListRecostRuns(ctx, sqlcgen.ListRecostRunsParams{ProductID: nullUUID(productID), RowOffset: off, RowLimit: lim})
	if err != nil {
		return paging.Result[RecostRun]{}, err
	}
	total, err := s.q(ctx).CountRecostRuns(ctx, nullUUID(productID))
	if err != nil {
		return paging.Result[RecostRun]{}, err
	}
	out := make([]RecostRun, len(rows))
	for i, r := range rows {
		out[i] = toRun(r)
	}
	return paging.Result[RecostRun]{Items: out, Total: total}, nil
}

// LockAllLayers mengunci seluruh cost layer batch dalam urutan FIFO.
func (s *Store) LockAllLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) ([]domain.ReplayLayer, error) {
	rows, err := s.q(ctx).LockAllLayers(ctx, sqlcgen.LockAllLayersParams{ProductID: productID, BatchID: nullUUID(batchID)})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ReplayLayer, len(rows))
	for i, r := range rows {
		out[i] = domain.ReplayLayer{ID: r.ID, BusinessDate: r.BusinessDate, ReceivedQty: r.ReceivedQty, UnitCost: r.UnitCost}
	}
	return out, nil
}

// BatchIssue adalah movement keluar satu batch beserta HPP efektifnya.
type BatchIssue struct {
	domain.ReplayIssue
	LocationID uuid.UUID
	SourceType string
	SourceID   uuid.UUID
}

// BatchIssues membaca seluruh movement keluar batch dalam urutan tanggal bisnis.
func (s *Store) BatchIssues(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) ([]BatchIssue, error) {
	rows, err := s.q(ctx).BatchIssues(ctx, sqlcgen.BatchIssuesParams{ProductID: productID, BatchID: nullUUID(batchID)})
	if err != nil {
		return nil, err
	}
	out := make([]BatchIssue, len(rows))
	for i, r := range rows {
		out[i] = BatchIssue{
			ReplayIssue: domain.ReplayIssue{MovementID: r.ID, BusinessDate: r.BusinessDate, Qty: r.Qty, CurrentCost: r.CurrentCost},
			LocationID:  r.LocationID, SourceType: r.SourceType, SourceID: r.SourceID,
		}
	}
	return out, nil
}

// RecostLine adalah selisih HPP satu movement.
type RecostLine struct {
	RunID      uuid.UUID
	MovementID uuid.UUID
	LocationID uuid.UUID
	SourceType string
	SourceID   uuid.UUID
	Previous   decimal.Decimal
	Recomputed decimal.Decimal
}

// InsertRecostLine mencatat selisih HPP (append-only).
func (s *Store) InsertRecostLine(ctx context.Context, l RecostLine) error {
	return s.q(ctx).InsertRecostLine(ctx, sqlcgen.InsertRecostLineParams{
		RecostRunID: l.RunID, StockMovementID: l.MovementID, LocationID: l.LocationID, SourceType: l.SourceType,
		SourceID: l.SourceID, PreviousCost: l.Previous, RecomputedCost: l.Recomputed, Difference: l.Recomputed.Sub(l.Previous),
	})
}

// OpenLayers membaca cost layer batch yang masih bersisa tanpa mengunci (estimasi nilai sebelum posting).
func (s *Store) OpenLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (domain.LayerSet, error) {
	rows, err := s.q(ctx).OpenLayers(ctx, sqlcgen.OpenLayersParams{ProductID: productID, BatchID: nullUUID(batchID)})
	if err != nil {
		return domain.LayerSet{}, err
	}
	set := domain.LayerSet{Layers: make([]domain.Layer, len(rows))}
	for i, r := range rows {
		set.Layers[i] = domain.Layer{ID: r.ID, Remaining: r.RemainingQty, UnitCost: r.UnitCost}
	}
	return set, nil
}

// RecostLines membaca selisih per movement satu run.
func (s *Store) RecostLines(ctx context.Context, runID uuid.UUID) ([]RecostLine, error) {
	rows, err := s.q(ctx).ListRecostLines(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]RecostLine, len(rows))
	for i, r := range rows {
		out[i] = RecostLine{
			RunID: r.RecostRunID, MovementID: r.StockMovementID, LocationID: r.LocationID, SourceType: r.SourceType,
			SourceID: r.SourceID, Previous: r.PreviousCost, Recomputed: r.RecomputedCost,
		}
	}
	return out, nil
}
