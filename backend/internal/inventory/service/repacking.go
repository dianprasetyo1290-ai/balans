package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// RepackView adalah detail Repacking.
type RepackView struct {
	Repacking      domain.Repacking
	Refs           DocRefs
	Movements      []postgres.SourceMovement
	AllowedActions []domain.DocAction
}

// RepackPage adalah satu halaman daftar Repacking.
type RepackPage struct {
	paging.Result[domain.Repacking]
	Refs DocRefs
}

func repackSnapshot(r domain.Repacking) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"direction": l.Direction, "productId": l.ProductID, "locationId": l.LocationID, "batchId": l.BatchID,
			"batchNumber": l.BatchNumber, "qty": l.Qty.String(), "costAmount": l.CostAmount.StringFixed(2),
		}
	}
	return map[string]any{
		"number": r.Number, "businessDate": r.BusinessDate.String(), "status": r.Status, "note": r.Note,
		"costAmount": r.CostAmount.StringFixed(2), "lines": lines,
	}
}

func repackRefIDs(docs ...domain.Repacking) refIDs {
	var ids refIDs
	for _, r := range docs {
		for _, l := range r.Lines {
			ids.products = append(ids.products, l.ProductID)
			ids.locations = append(ids.locations, l.LocationID)
			if l.BatchID != nil {
				ids.batches = append(ids.batches, *l.BatchID)
			}
		}
	}
	return ids
}

func repackLabel(r domain.Repacking) string {
	if r.Number != "" {
		return r.Number
	}
	return "Draft Repacking"
}

// ListRepackings mengembalikan daftar Repacking.
func (s *Service) ListRepackings(ctx context.Context, f postgres.DocFilter, page paging.Page) (RepackPage, error) {
	if _, err := actor.Require(ctx, domain.PermRepackRead); err != nil {
		return RepackPage{}, err
	}
	res, err := s.Store.ListRepackings(ctx, f, page)
	if err != nil {
		return RepackPage{}, err
	}
	refs, err := s.docRefs(ctx, repackRefIDs(res.Items...))
	if err != nil {
		return RepackPage{}, err
	}
	return RepackPage{Result: res, Refs: refs}, nil
}

// GetRepacking mengembalikan detail Repacking.
func (s *Service) GetRepacking(ctx context.Context, id uuid.UUID) (RepackView, error) {
	a, err := actor.Require(ctx, domain.PermRepackRead)
	if err != nil {
		return RepackView{}, err
	}
	r, err := s.Store.GetRepacking(ctx, id, false)
	if err != nil {
		return RepackView{}, err
	}
	movs, err := s.sourceMovements(ctx, SourceRepacking, r.ID)
	if err != nil {
		return RepackView{}, err
	}
	ids := repackRefIDs(r)
	for _, m := range movs {
		if m.BatchID != nil {
			ids.batches = append(ids.batches, *m.BatchID)
		}
	}
	refs, err := s.docRefs(ctx, ids)
	if err != nil {
		return RepackView{}, err
	}
	return RepackView{Repacking: r, Refs: refs, Movements: movs, AllowedActions: r.AllowedActions(a.Has)}, nil
}

// checkRepackMasters: bahan FG dari lokasi Retur, packaging dari lokasi RM; hasil FG ke lokasi FG Non-Ecommerce.
func (s *Service) checkRepackMasters(ctx context.Context, r domain.Repacking) error {
	var pids, lids []uuid.UUID
	for _, l := range r.Lines {
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
	if err := requireProducts(prods, pids, func(i int) string { return lineField(i, "productId") }); err != nil {
		return err
	}
	for i, l := range r.Lines {
		p := prods[l.ProductID]
		field := lineField(i, "locationId")
		switch {
		case l.Direction == domain.RepackOutput:
			if p.Type != catalog.TypeFinishedGood {
				return apperr.Validation(map[string]string{lineField(i, "productId"): "Hasil repacking harus Produk Jadi"})
			}
			if _, err := requireLocation(locs, l.LocationID, field, warehouse.LocationFinishedGoods); err != nil {
				return err
			}
		case p.Type == catalog.TypeFinishedGood:
			if _, err := requireLocation(locs, l.LocationID, field, warehouse.LocationReturns); err != nil {
				return err
			}
		case p.Type == catalog.TypePackaging:
			if _, err := requireLocation(locs, l.LocationID, field, warehouse.LocationRawMaterial); err != nil {
				return err
			}
		default:
			return apperr.Validation(map[string]string{lineField(i, "productId"): "Bahan repacking hanya barang retur (Produk Jadi) atau Packaging"})
		}
	}
	return nil
}

// CreateRepacking membuat draft Repacking.
func (s *Service) CreateRepacking(ctx context.Context, in domain.RepackingInput) (domain.Repacking, error) {
	a, err := actor.Require(ctx, domain.PermRepackWrite)
	if err != nil {
		return domain.Repacking{}, err
	}
	r, err := domain.NewRepacking(in, docActor(a))
	if err != nil {
		return domain.Repacking{}, err
	}
	if err := s.checkRepackMasters(ctx, r); err != nil {
		return domain.Repacking{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if r, err = s.Store.InsertRepacking(ctx, r); err != nil {
			return err
		}
		return s.record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: SourceRepacking, EntityID: r.ID, EntityLabel: repackLabel(r), After: repackSnapshot(r),
		})
	})
	return r, err
}

func (s *Service) mutateRepacking(ctx context.Context, id uuid.UUID, version int, perm string,
	fn func(ctx context.Context, a actor.Actor, r *domain.Repacking) (audit.Action, error),
) (domain.Repacking, error) {
	a, err := actor.Require(ctx, perm)
	if err != nil {
		return domain.Repacking{}, err
	}
	var out domain.Repacking
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		r, err := s.Store.GetRepacking(ctx, id, true)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(r.Version, version); err != nil {
			return err
		}
		before := repackSnapshot(r)
		action, err := fn(ctx, a, &r)
		if err != nil {
			return err
		}
		if err := s.Store.SaveRepacking(ctx, &r, a.UserID); err != nil {
			return err
		}
		out = r
		return s.record(ctx, audit.Entry{
			Action: action, EntityType: SourceRepacking, EntityID: r.ID, EntityLabel: repackLabel(r), Before: before, After: repackSnapshot(r),
		})
	})
	return out, err
}

// UpdateRepacking mengubah draft.
func (s *Service) UpdateRepacking(ctx context.Context, id uuid.UUID, version int, in domain.RepackingInput) (domain.Repacking, error) {
	return s.mutateRepacking(ctx, id, version, domain.PermRepackWrite, func(ctx context.Context, _ actor.Actor, r *domain.Repacking) (audit.Action, error) {
		if err := r.Update(in); err != nil {
			return "", err
		}
		if err := s.checkRepackMasters(ctx, *r); err != nil {
			return "", err
		}
		return audit.ActionUpdate, s.Store.ReplaceRepackLines(ctx, r)
	})
}

func (s *Service) repackSource(r domain.Repacking, action string) domain.Source {
	return domain.Source{Type: SourceRepacking, ID: r.ID, Action: action, Label: r.Number, BusinessDate: s.today()}
}

// StartRepacking memberi nomor RPK dan mengalokasikan bahan berstatus Tersedia (reserved) selama proses.
func (s *Service) StartRepacking(ctx context.Context, id uuid.UUID, version int) (domain.Repacking, error) {
	return s.mutateRepacking(ctx, id, version, domain.PermRepackPost, func(ctx context.Context, a actor.Actor, r *domain.Repacking) (audit.Action, error) {
		if err := r.Transition(domain.ActionStart); err != nil {
			return "", err
		}
		if err := s.checkRepackMasters(ctx, *r); err != nil {
			return "", err
		}
		var err error
		if r.Number, err = s.number(ctx, document.TypeRepacking, r.BusinessDate); err != nil {
			return "", err
		}
		var lines []ReserveLine
		for _, l := range r.Inputs() {
			if l.StockStatus != domain.StatusAvailable {
				continue
			}
			lineID := l.ID
			lines = append(lines, ReserveLine{
				SourceLineID: &lineID, ProductID: l.ProductID, LocationID: l.LocationID, BatchID: l.BatchID, Qty: l.Qty,
				BatchReason: batchReasonFor(l.BatchID, "Bahan repacking "+r.Number),
			})
		}
		if len(lines) > 0 {
			if _, err := s.Reserve(ctx, ReserveRequest{Source: s.repackSource(*r, stockReserve), Lines: lines}); err != nil {
				return "", err
			}
		}
		r.Started = domain.NewStep(docActor(a), s.Clock.Now())
		return audit.ActionStatusChange, nil
	})
}

// batchReasonFor mengisi alasan batch manual bila batch ditentukan dokumen (bukan saran FEFO).
func batchReasonFor(batchID *uuid.UUID, reason string) string {
	if batchID == nil {
		return ""
	}
	return reason
}

// PostRepacking mengeluarkan bahan (HPP FIFO), menerima FG hasil dengan batch baru bernilai HPP barang retur +
// packaging, dan menjurnal A-26 (Dr 1311; Cr 1314 barang retur, Cr 1301 packaging).
func (s *Service) PostRepacking(ctx context.Context, id uuid.UUID, version int) (domain.Repacking, error) {
	return s.mutateRepacking(ctx, id, version, domain.PermRepackPost, func(ctx context.Context, a actor.Actor, r *domain.Repacking) (audit.Action, error) {
		if err := r.Transition(domain.ActionExecute); err != nil {
			return "", err
		}
		inputs := r.Inputs()
		lines := make([]domain.Line, len(inputs))
		for i, l := range inputs {
			lineID := l.ID
			from := &domain.Place{LocationID: l.LocationID, Status: l.StockStatus}
			line := domain.Line{Kind: domain.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID, From: from}
			if l.StockStatus == domain.StatusAvailable {
				from.Status = domain.StatusReserved
				line.Reservation = &domain.ReservationRef{SourceType: SourceRepacking, SourceID: r.ID, SourceLineID: &lineID}
			}
			lines[i] = line
		}
		res, err := s.Post(ctx, domain.Posting{Source: s.repackSource(*r, stockIssue), Lines: lines})
		if err != nil {
			return "", err
		}
		prods, err := s.productsOf(ctx, lineProducts(inputs)...)
		if err != nil {
			return "", err
		}
		returned, packaging := decimal.Zero, decimal.Zero
		cost := map[uuid.UUID]decimal.Decimal{}
		for i, l := range inputs {
			c := res.Lines[i].CostAmount
			cost[l.ID] = c
			if prods[l.ProductID].Type == catalog.TypePackaging {
				packaging = packaging.Add(c)
			} else {
				returned = returned.Add(c)
			}
		}
		total := returned.Add(packaging)
		out := r.Output()
		unit := domain.OutputUnitCost(total, out.Qty)
		outID := out.ID
		recv, err := s.Post(ctx, domain.Posting{Source: s.repackSource(*r, stockExecute), Lines: []domain.Line{{
			Kind: domain.KindReceive, SourceLineID: &outID, ProductID: out.ProductID, Qty: out.Qty, UnitCost: &unit,
			Batch: &domain.NewBatch{Number: out.BatchNumber, ProductionDate: out.ProductionDate, ExpiryDate: out.ExpiryDate},
			To:    &domain.Place{LocationID: out.LocationID, Status: domain.StatusAvailable}, Note: "Hasil repacking " + r.Number,
		}}})
		if err != nil {
			return "", err
		}
		for i := range r.Lines {
			l := &r.Lines[i]
			if l.Direction == domain.RepackOutput {
				l.CostAmount = total
				if len(recv.Movements) > 0 {
					l.BatchID = recv.Movements[0].BatchID
				}
				continue
			}
			l.CostAmount = cost[l.ID]
		}
		r.CostAmount = total
		r.Posted = domain.NewStep(docActor(a), s.Clock.Now())
		if total.IsPositive() {
			div, err := s.divisionByCode(ctx, DivisionLogistics)
			if err != nil {
				return "", err
			}
			r.JournalID, err = s.postJournal(ctx, accounting.PostingRequest{
				RuleCode: "A-26", SourceType: SourceRepacking, SourceID: r.ID, SourceLabel: r.Number, BusinessDate: s.today(),
				Amounts: map[string]money.Amount{
					"restock_value": amountOf(total), "returned_value": amountOf(returned), "packaging_value": amountOf(packaging),
				},
				Dimensions:  accounting.Dimensions{DivisionID: &div},
				Description: fmt.Sprintf("Repacking %s", r.Number),
			})
			if err != nil {
				return "", err
			}
		}
		return audit.ActionPost, nil
	})
}

func lineProducts(lines []domain.RepackLine) []uuid.UUID {
	out := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		out[i] = l.ProductID
	}
	return out
}

// CancelRepacking membatalkan draft atau repacking yang sedang diproses (alokasi bahan dilepas).
func (s *Service) CancelRepacking(ctx context.Context, id uuid.UUID, version int) (domain.Repacking, error) {
	return s.mutateRepacking(ctx, id, version, domain.PermRepackWrite, func(ctx context.Context, _ actor.Actor, r *domain.Repacking) (audit.Action, error) {
		started := r.Status == domain.DocInProgress
		if err := r.Transition(domain.ActionCancel); err != nil {
			return "", err
		}
		if started {
			if _, err := s.Release(ctx, ReleaseRequest{
				Source:      s.repackSource(*r, stockRelease),
				Reservation: domain.ReservationRef{SourceType: SourceRepacking, SourceID: r.ID},
			}); err != nil {
				return "", err
			}
		}
		return audit.ActionCancel, nil
	})
}
