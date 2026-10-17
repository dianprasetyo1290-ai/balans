package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// DisposalView adalah detail Pemusnahan (juga isi Berita Acara cetak).
type DisposalView struct {
	Disposal       domain.Disposal
	Refs           DocRefs
	Movements      []postgres.SourceMovement
	AllowedActions []domain.DocAction
}

// DisposalPage adalah satu halaman daftar Pemusnahan.
type DisposalPage struct {
	paging.Result[domain.Disposal]
	Refs DocRefs
}

func disposalSnapshot(d domain.Disposal) map[string]any {
	lines := make([]map[string]any, len(d.Lines))
	for i, l := range d.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "batchId": l.BatchID, "locationId": l.LocationID, "stockStatus": l.StockStatus,
			"qty": l.Qty.String(), "costAmount": l.CostAmount.StringFixed(2),
		}
	}
	return map[string]any{
		"number": d.Number, "reason": d.Reason, "disposalDate": d.DisposalDate.String(), "method": d.Method,
		"witnesses": d.Witnesses, "photoFileIds": d.PhotoFileIDs, "status": d.Status, "note": d.Note,
		"estimatedCost": d.EstimatedCost.StringFixed(2), "costAmount": d.CostAmount.StringFixed(2), "lines": lines,
	}
}

func disposalRefIDs(docs ...domain.Disposal) refIDs {
	var ids refIDs
	for _, d := range docs {
		ids.files = append(ids.files, d.PhotoFileIDs...)
		for _, l := range d.Lines {
			ids.products = append(ids.products, l.ProductID)
			ids.locations = append(ids.locations, l.LocationID)
			if l.BatchID != nil {
				ids.batches = append(ids.batches, *l.BatchID)
			}
		}
	}
	return ids
}

func disposalLabel(d domain.Disposal) string {
	if d.Number != "" {
		return d.Number
	}
	return "Draft Pemusnahan"
}

// ListDisposals mengembalikan daftar Pemusnahan.
func (s *Service) ListDisposals(ctx context.Context, f postgres.DocFilter, page paging.Page) (DisposalPage, error) {
	if _, err := actor.Require(ctx, domain.PermDisposalRead); err != nil {
		return DisposalPage{}, err
	}
	res, err := s.Store.ListDisposals(ctx, f, page)
	if err != nil {
		return DisposalPage{}, err
	}
	refs, err := s.docRefs(ctx, disposalRefIDs(res.Items...))
	if err != nil {
		return DisposalPage{}, err
	}
	return DisposalPage{Result: res, Refs: refs}, nil
}

// GetDisposal mengembalikan detail Pemusnahan.
func (s *Service) GetDisposal(ctx context.Context, id uuid.UUID) (DisposalView, error) {
	a, err := actor.Require(ctx, domain.PermDisposalRead)
	if err != nil {
		return DisposalView{}, err
	}
	d, err := s.Store.GetDisposal(ctx, id, false)
	if err != nil {
		return DisposalView{}, err
	}
	movs, err := s.sourceMovements(ctx, SourceDisposal, d.ID)
	if err != nil {
		return DisposalView{}, err
	}
	ids := disposalRefIDs(d)
	for _, m := range movs {
		if m.BatchID != nil {
			ids.batches = append(ids.batches, *m.BatchID)
		}
	}
	refs, err := s.docRefs(ctx, ids)
	if err != nil {
		return DisposalView{}, err
	}
	return DisposalView{Disposal: d, Refs: refs, Movements: movs, AllowedActions: d.AllowedActions(a.Has)}, nil
}

// disposalLocationTypes: barang retur hanya dari Gudang Retur (A-26 kredit 1314); kedaluwarsa/rusak dari lokasi
// persediaan FG gudang (A-27 kredit 1311).
func disposalLocationTypes(r domain.DisposalReason) []warehouse.LocationType {
	if r == domain.DisposalReturn {
		return []warehouse.LocationType{warehouse.LocationReturns}
	}
	return []warehouse.LocationType{
		warehouse.LocationFinishedGoods, warehouse.LocationEcommerce, warehouse.LocationType("petty_stock"),
		warehouse.LocationInbound, warehouse.LocationEvent,
	}
}

func (s *Service) checkDisposalMasters(ctx context.Context, d domain.Disposal) error {
	var pids, lids []uuid.UUID
	for _, l := range d.Lines {
		pids, lids = append(pids, l.ProductID), append(lids, l.LocationID)
	}
	prods, err := s.productsOf(ctx, pids...)
	if err != nil {
		return err
	}
	if err := requireProducts(prods, pids, func(i int) string { return lineField(i, "productId") }); err != nil {
		return err
	}
	locs, err := s.locationsOf(ctx, lids...)
	if err != nil {
		return err
	}
	for i, l := range d.Lines {
		if _, err := requireLocation(locs, l.LocationID, lineField(i, "locationId"), disposalLocationTypes(d.Reason)...); err != nil {
			return err
		}
	}
	return nil
}

// attachPhotos menautkan foto berita acara baru ke dokumen (filestore). Foto lama tetap tertaut.
func (s *Service) attachPhotos(ctx context.Context, d domain.Disposal, previous []uuid.UUID) error {
	for _, id := range d.PhotoFileIDs {
		if slices.Contains(previous, id) {
			continue
		}
		if err := requireDep(s.Files != nil, "filestore"); err != nil {
			return err
		}
		if err := s.Files.Attach(ctx, id, FileOwnerDisposal, d.ID); err != nil {
			return err
		}
	}
	return nil
}

// CreateDisposal membuat draft Pemusnahan.
func (s *Service) CreateDisposal(ctx context.Context, in domain.DisposalInput) (domain.Disposal, error) {
	a, err := actor.Require(ctx, domain.PermDisposalWrite)
	if err != nil {
		return domain.Disposal{}, err
	}
	d, err := domain.NewDisposal(in, docActor(a))
	if err != nil {
		return domain.Disposal{}, err
	}
	if err := s.checkDisposalMasters(ctx, d); err != nil {
		return domain.Disposal{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if d, err = s.Store.InsertDisposal(ctx, d); err != nil {
			return err
		}
		if err := s.attachPhotos(ctx, d, nil); err != nil {
			return err
		}
		return s.record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: SourceDisposal, EntityID: d.ID, EntityLabel: disposalLabel(d), After: disposalSnapshot(d),
		})
	})
	return d, err
}

func (s *Service) mutateDisposal(ctx context.Context, id uuid.UUID, version int, perm string,
	fn func(ctx context.Context, a actor.Actor, d *domain.Disposal) (audit.Action, error),
) (domain.Disposal, error) {
	a, err := actor.Require(ctx, perm)
	if err != nil {
		return domain.Disposal{}, err
	}
	var out domain.Disposal
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDisposal(ctx, id, true)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(d.Version, version); err != nil {
			return err
		}
		before := disposalSnapshot(d)
		previous := slices.Clone(d.PhotoFileIDs)
		action, err := fn(ctx, a, &d)
		if err != nil {
			return err
		}
		if err := s.attachPhotos(ctx, d, previous); err != nil {
			return err
		}
		if err := s.Store.SaveDisposal(ctx, &d, a.UserID); err != nil {
			return err
		}
		out = d
		return s.record(ctx, audit.Entry{
			Action: action, EntityType: SourceDisposal, EntityID: d.ID, EntityLabel: disposalLabel(d), Before: before, After: disposalSnapshot(d),
		})
	})
	return out, err
}

// UpdateDisposal mengubah draft.
func (s *Service) UpdateDisposal(ctx context.Context, id uuid.UUID, version int, in domain.DisposalInput) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalWrite, func(ctx context.Context, _ actor.Actor, d *domain.Disposal) (audit.Action, error) {
		if err := d.Update(in); err != nil {
			return "", err
		}
		if err := s.checkDisposalMasters(ctx, *d); err != nil {
			return "", err
		}
		return audit.ActionUpdate, s.Store.ReplaceDisposalLines(ctx, d)
	})
}

// UpdateDisposalReport mengisi berita acara (tanggal, metode, saksi, foto) sebelum posting.
func (s *Service) UpdateDisposalReport(ctx context.Context, id uuid.UUID, version int, r domain.DisposalReport) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalWrite, func(_ context.Context, _ actor.Actor, d *domain.Disposal) (audit.Action, error) {
		return audit.ActionUpdate, d.UpdateReport(r)
	})
}

func (s *Service) estimateDisposal(ctx context.Context, d *domain.Disposal) error {
	var pids, lids []uuid.UUID
	for _, l := range d.Lines {
		pids, lids = append(pids, l.ProductID), append(lids, l.LocationID)
	}
	prods, err := s.productsOf(ctx, pids...)
	if err != nil {
		return err
	}
	locs, err := s.locationsOf(ctx, lids...)
	if err != nil {
		return err
	}
	total := decimal.Zero
	for _, l := range d.Lines {
		from := domain.Place{LocationID: l.LocationID, Status: l.StockStatus}
		c, err := s.estimateIssue(ctx, l.ProductID, from, l.BatchID, l.Qty, prods[l.ProductID], locs[l.LocationID])
		if err != nil {
			return err
		}
		total = total.Add(c)
	}
	d.EstimatedCost = total
	return nil
}

// SubmitDisposal memberi nomor MUS, menaksir nilai HPP, dan mengajukan approval `disposal` (Head Logistik → Head
// F&A → Direksi bila > Rp10 jt).
func (s *Service) SubmitDisposal(ctx context.Context, id uuid.UUID, version int) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalSubmit, func(ctx context.Context, a actor.Actor, d *domain.Disposal) (audit.Action, error) {
		if err := d.Transition(domain.ActionSubmit); err != nil {
			return "", err
		}
		if err := s.checkDisposalMasters(ctx, *d); err != nil {
			return "", err
		}
		if err := s.estimateDisposal(ctx, d); err != nil {
			return "", err
		}
		var err error
		if d.Number == "" {
			if d.Number, err = s.number(ctx, document.TypeDisposal, d.DisposalDate); err != nil {
				return "", err
			}
		}
		d.Submitted = domain.NewStep(docActor(a), s.Clock.Now())
		return audit.ActionSubmit, s.requestApproval(ctx, approval.Subject{
			Type: approval.SubjectDisposal, ID: d.ID, Label: d.Number, Link: "/inventory/disposals/" + d.ID.String(),
			Summary: fmt.Sprintf("%s · %d baris", d.Reason.Label(), len(d.Lines)), Amount: amountOf(d.EstimatedCost),
		})
	})
}

// ReviseDisposal mengembalikan dokumen yang ditolak ke draft.
func (s *Service) ReviseDisposal(ctx context.Context, id uuid.UUID, version int) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalWrite, func(_ context.Context, _ actor.Actor, d *domain.Disposal) (audit.Action, error) {
		return audit.ActionStatusChange, d.Transition(domain.ActionRevise)
	})
}

// CancelDisposal membatalkan draft atau dokumen yang sudah disetujui sebelum posting.
func (s *Service) CancelDisposal(ctx context.Context, id uuid.UUID, version int) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalWrite, func(_ context.Context, _ actor.Actor, d *domain.Disposal) (audit.Action, error) {
		if d.ReturnDispositionID != nil {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Pemusnahan dari Disposisi Retur tidak bisa dibatalkan")
		}
		return audit.ActionCancel, d.Transition(domain.ActionCancel)
	})
}

// PostDisposal mengeluarkan barang (HPP FIFO) dan menjurnal A-26 (barang retur: Dr 6403, Cr 1314) atau A-27
// (kedaluwarsa/rusak: Dr 6402, Cr 1311). Berita acara wajib lengkap.
func (s *Service) PostDisposal(ctx context.Context, id uuid.UUID, version int) (domain.Disposal, error) {
	return s.mutateDisposal(ctx, id, version, domain.PermDisposalPost, func(ctx context.Context, a actor.Actor, d *domain.Disposal) (audit.Action, error) {
		if err := d.Transition(domain.ActionExecute); err != nil {
			return "", err
		}
		lines := make([]domain.Line, len(d.Lines))
		for i, l := range d.Lines {
			lineID := l.ID
			lines[i] = domain.Line{
				Kind: domain.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID,
				From:        &domain.Place{LocationID: l.LocationID, Status: l.StockStatus},
				BatchReason: batchReasonFor(l.BatchID, "Pemusnahan "+d.Number+" ("+d.Reason.Label()+")"),
			}
		}
		res, err := s.Post(ctx, domain.Posting{
			Source: domain.Source{Type: SourceDisposal, ID: d.ID, Action: stockExecute, Label: d.Number, BusinessDate: s.today()},
			Lines:  lines,
		})
		if err != nil {
			return "", err
		}
		total := decimal.Zero
		for i := range d.Lines {
			d.Lines[i].CostAmount = res.Lines[i].CostAmount
			total = total.Add(res.Lines[i].CostAmount)
		}
		d.CostAmount = total
		d.PostingRuleCode = d.Reason.PostingRule()
		d.Posted = domain.NewStep(docActor(a), s.Clock.Now())
		if !total.IsPositive() {
			return audit.ActionPost, nil
		}
		div, err := s.divisionByCode(ctx, DivisionLogistics)
		if err != nil {
			return "", err
		}
		amounts := map[string]money.Amount{"expired_value": amountOf(total)}
		if d.PostingRuleCode == "A-26" {
			amounts = map[string]money.Amount{"destroy_value": amountOf(total), "returned_value": amountOf(total)}
		}
		d.JournalID, err = s.postJournal(ctx, accounting.PostingRequest{
			RuleCode: d.PostingRuleCode, SourceType: SourceDisposal, SourceID: d.ID, SourceLabel: d.Number,
			BusinessDate: s.today(), Amounts: amounts, Dimensions: accounting.Dimensions{DivisionID: &div},
			Description: fmt.Sprintf("Pemusnahan %s (%s)", d.Number, d.Reason.Label()),
		})
		return audit.ActionPost, err
	})
}

func (s *Service) decideDisposal(ctx context.Context, dec approval.Decided, a domain.DocAction) error {
	d, err := s.Store.GetDisposal(ctx, dec.Subject.ID, true)
	if err != nil {
		return err
	}
	before := disposalSnapshot(d)
	if err := d.Transition(a); err != nil {
		return err
	}
	if err := s.Store.SaveDisposal(ctx, &d, dec.DecidedBy); err != nil {
		return err
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionStatusChange, EntityType: SourceDisposal, EntityID: d.ID, EntityLabel: d.Number,
		Before: before, After: disposalSnapshot(d), Note: dec.Note,
	})
}
