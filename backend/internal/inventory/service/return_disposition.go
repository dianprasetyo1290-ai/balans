package service

import (
	"context"
	"fmt"

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

// DispositionView adalah detail Disposisi Retur.
type DispositionView struct {
	Disposition    domain.ReturnDisposition
	Refs           DocRefs
	Movements      []postgres.SourceMovement
	AllowedActions []domain.DocAction
}

// DispositionPage adalah satu halaman daftar Disposisi Retur.
type DispositionPage struct {
	paging.Result[domain.ReturnDisposition]
	Refs DocRefs
}

func dispositionSnapshot(d domain.ReturnDisposition) map[string]any {
	lines := make([]map[string]any, len(d.Lines))
	for i, l := range d.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "batchId": l.BatchID, "qty": l.Qty.String(), "reasonId": l.ReasonID,
			"disposition": l.Disposition, "qcNote": l.QCNote, "estimatedCost": l.EstimatedCost.StringFixed(2),
			"targetType": l.TargetType, "targetId": l.TargetID,
		}
	}
	return map[string]any{
		"number": d.Number, "locationId": d.LocationID, "businessDate": d.BusinessDate.String(), "status": d.Status,
		"note": d.Note, "disposalValue": d.DisposalValue.StringFixed(2), "costAmount": d.CostAmount.StringFixed(2),
		"lines": lines,
	}
}

func dispositionRefIDs(docs ...domain.ReturnDisposition) refIDs {
	var ids refIDs
	for _, d := range docs {
		ids.locations = append(ids.locations, d.LocationID)
		for _, l := range d.Lines {
			ids.products = append(ids.products, l.ProductID)
			if l.BatchID != nil {
				ids.batches = append(ids.batches, *l.BatchID)
			}
			if l.ReasonID != nil {
				ids.reasons = append(ids.reasons, *l.ReasonID)
			}
		}
	}
	return ids
}

func dispositionLabel(d domain.ReturnDisposition) string {
	if d.Number != "" {
		return d.Number
	}
	return "Draft Disposisi Retur"
}

// ListDispositions mengembalikan daftar Disposisi Retur.
func (s *Service) ListDispositions(ctx context.Context, f postgres.DocFilter, page paging.Page) (DispositionPage, error) {
	if _, err := actor.Require(ctx, domain.PermDispositionRead); err != nil {
		return DispositionPage{}, err
	}
	res, err := s.Store.ListDispositions(ctx, f, page)
	if err != nil {
		return DispositionPage{}, err
	}
	refs, err := s.docRefs(ctx, dispositionRefIDs(res.Items...))
	if err != nil {
		return DispositionPage{}, err
	}
	return DispositionPage{Result: res, Refs: refs}, nil
}

// GetDisposition mengembalikan detail Disposisi Retur.
func (s *Service) GetDisposition(ctx context.Context, id uuid.UUID) (DispositionView, error) {
	a, err := actor.Require(ctx, domain.PermDispositionRead)
	if err != nil {
		return DispositionView{}, err
	}
	d, err := s.Store.GetDisposition(ctx, id, false)
	if err != nil {
		return DispositionView{}, err
	}
	movs, err := s.sourceMovements(ctx, SourceReturnDisposition, d.ID)
	if err != nil {
		return DispositionView{}, err
	}
	ids := dispositionRefIDs(d)
	for _, m := range movs {
		ids.locations = append(ids.locations, m.LocationID)
	}
	refs, err := s.docRefs(ctx, ids)
	if err != nil {
		return DispositionView{}, err
	}
	return DispositionView{Disposition: d, Refs: refs, Movements: movs, AllowedActions: d.AllowedActions(a.Has)}, nil
}

// applyReasonDefaults mengisi disposisi kosong dengan usulan disposisi alasan retur (master sales) dan menolak
// alasan yang tidak bisa dipakai.
func (s *Service) applyReasonDefaults(ctx context.Context, in domain.ReturnDispositionInput) (domain.ReturnDispositionInput, error) {
	fields := map[string]string{}
	for i := range in.Lines {
		l := &in.Lines[i]
		if l.ReasonID == nil {
			continue
		}
		if err := requireDep(s.ReturnReasons != nil, "sales"); err != nil {
			return in, err
		}
		r, err := s.ReturnReasons.ReturnReason(ctx, *l.ReasonID)
		switch {
		case apperr.HasCode(err, apperr.CodeNotFound):
			fields[lineField(i, "reasonId")] = "Alasan retur tidak ditemukan"
			continue
		case err != nil:
			return in, err
		case r.Usable != nil:
			fields[lineField(i, "reasonId")] = "Alasan retur " + r.Code + " tidak bisa dipakai"
			continue
		}
		if l.Disposition == "" {
			l.Disposition = r.DefaultDisposition
		}
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

func (s *Service) checkDispositionMasters(ctx context.Context, d domain.ReturnDisposition) error {
	locs, err := s.locationsOf(ctx, d.LocationID)
	if err != nil {
		return err
	}
	if _, err := requireLocation(locs, d.LocationID, "locationId", warehouse.LocationReturns); err != nil {
		return err
	}
	var pids, bids []uuid.UUID
	for _, l := range d.Lines {
		pids = append(pids, l.ProductID)
		if l.BatchID != nil {
			bids = append(bids, *l.BatchID)
		}
	}
	prods, err := s.productsOf(ctx, pids...)
	if err != nil {
		return err
	}
	if err := requireProducts(prods, pids, func(i int) string { return lineField(i, "productId") }); err != nil {
		return err
	}
	batches, err := s.Store.BatchesByIDs(ctx, uniq(bids))
	if err != nil {
		return err
	}
	fields := map[string]string{}
	for i, l := range d.Lines {
		p := prods[l.ProductID]
		switch {
		case l.BatchID == nil && p.HasBatch:
			fields[lineField(i, "batchId")] = "Batch wajib diisi untuk " + p.SKU
		case l.BatchID != nil && (batches[*l.BatchID].ProductID != l.ProductID):
			fields[lineField(i, "batchId")] = "Batch tidak ditemukan untuk produk " + p.SKU
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CreateDisposition membuat draft Disposisi Retur dari barang di Gudang Retur: hasil QC dan usulan disposisi per baris.
func (s *Service) CreateDisposition(ctx context.Context, in domain.ReturnDispositionInput) (domain.ReturnDisposition, error) {
	a, err := actor.Require(ctx, domain.PermDispositionWrite)
	if err != nil {
		return domain.ReturnDisposition{}, err
	}
	if in, err = s.applyReasonDefaults(ctx, in); err != nil {
		return domain.ReturnDisposition{}, err
	}
	d, err := domain.NewReturnDisposition(in, docActor(a))
	if err != nil {
		return domain.ReturnDisposition{}, err
	}
	if err := s.checkDispositionMasters(ctx, d); err != nil {
		return domain.ReturnDisposition{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if d, err = s.Store.InsertDisposition(ctx, d); err != nil {
			return err
		}
		return s.record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: SourceReturnDisposition, EntityID: d.ID, EntityLabel: dispositionLabel(d),
			After: dispositionSnapshot(d),
		})
	})
	return d, err
}

func (s *Service) mutateDisposition(ctx context.Context, id uuid.UUID, version int, perm string,
	fn func(ctx context.Context, a actor.Actor, d *domain.ReturnDisposition) (audit.Action, error),
) (domain.ReturnDisposition, error) {
	a, err := actor.Require(ctx, perm)
	if err != nil {
		return domain.ReturnDisposition{}, err
	}
	var out domain.ReturnDisposition
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDisposition(ctx, id, true)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(d.Version, version); err != nil {
			return err
		}
		before := dispositionSnapshot(d)
		action, err := fn(ctx, a, &d)
		if err != nil {
			return err
		}
		if err := s.Store.SaveDisposition(ctx, &d, a.UserID); err != nil {
			return err
		}
		out = d
		return s.record(ctx, audit.Entry{
			Action: action, EntityType: SourceReturnDisposition, EntityID: d.ID, EntityLabel: dispositionLabel(d),
			Before: before, After: dispositionSnapshot(d),
		})
	})
	return out, err
}

// UpdateDisposition mengubah draft.
func (s *Service) UpdateDisposition(ctx context.Context, id uuid.UUID, version int, in domain.ReturnDispositionInput) (domain.ReturnDisposition, error) {
	in, err := s.applyReasonDefaults(ctx, in)
	if err != nil {
		return domain.ReturnDisposition{}, err
	}
	return s.mutateDisposition(ctx, id, version, domain.PermDispositionWrite, func(ctx context.Context, _ actor.Actor, d *domain.ReturnDisposition) (audit.Action, error) {
		if err := d.Update(in); err != nil {
			return "", err
		}
		if err := s.checkDispositionMasters(ctx, *d); err != nil {
			return "", err
		}
		return audit.ActionUpdate, s.Store.ReplaceDispositionLines(ctx, d)
	})
}

// SubmitDisposition menaksir HPP per baris, memberi nomor DRT, dan mengajukan approval `return_disposition`
// (Head Logistik; ada pemusnahan + Head F&A; nilai pemusnahan > Rp10 jt + Direksi).
func (s *Service) SubmitDisposition(ctx context.Context, id uuid.UUID, version int) (domain.ReturnDisposition, error) {
	return s.mutateDisposition(ctx, id, version, domain.PermDispositionSubmit, func(ctx context.Context, a actor.Actor, d *domain.ReturnDisposition) (audit.Action, error) {
		if err := d.Transition(domain.ActionSubmit); err != nil {
			return "", err
		}
		if err := s.checkDispositionMasters(ctx, *d); err != nil {
			return "", err
		}
		pids := make([]uuid.UUID, len(d.Lines))
		for i, l := range d.Lines {
			pids[i] = l.ProductID
		}
		prods, err := s.productsOf(ctx, pids...)
		if err != nil {
			return "", err
		}
		locs, err := s.locationsOf(ctx, d.LocationID)
		if err != nil {
			return "", err
		}
		for i := range d.Lines {
			l := &d.Lines[i]
			from := domain.Place{LocationID: d.LocationID, Status: l.StockStatus}
			if l.EstimatedCost, err = s.estimateIssue(ctx, l.ProductID, from, l.BatchID, l.Qty, prods[l.ProductID], locs[d.LocationID]); err != nil {
				return "", err
			}
		}
		value := d.ComputeDisposalValue()
		if d.Number == "" {
			if d.Number, err = s.number(ctx, document.TypeReturnDisposition, d.BusinessDate); err != nil {
				return "", err
			}
		}
		d.Submitted = domain.NewStep(docActor(a), s.Clock.Now())
		subj := approval.Subject{
			Type: approval.SubjectReturnDisposition, ID: d.ID, Label: d.Number, Link: "/warehouse/return-dispositions/" + d.ID.String(),
			Summary: fmt.Sprintf("Gudang Retur %s · %d baris", locs[d.LocationID].Code, len(d.Lines)), Amount: amountOf(value),
		}
		if d.HasDisposal() {
			subj.Flags = []string{approval.ConditionHasDisposal}
		}
		return audit.ActionSubmit, s.requestApproval(ctx, subj)
	})
}

// ReviseDisposition mengembalikan dokumen yang ditolak ke draft.
func (s *Service) ReviseDisposition(ctx context.Context, id uuid.UUID, version int) (domain.ReturnDisposition, error) {
	return s.mutateDisposition(ctx, id, version, domain.PermDispositionWrite, func(_ context.Context, _ actor.Actor, d *domain.ReturnDisposition) (audit.Action, error) {
		return audit.ActionStatusChange, d.Transition(domain.ActionRevise)
	})
}

// CancelDisposition membatalkan draft.
func (s *Service) CancelDisposition(ctx context.Context, id uuid.UUID, version int) (domain.ReturnDisposition, error) {
	return s.mutateDisposition(ctx, id, version, domain.PermDispositionWrite, func(_ context.Context, _ actor.Actor, d *domain.ReturnDisposition) (audit.Action, error) {
		return audit.ActionCancel, d.Transition(domain.ActionCancel)
	})
}

// PostDisposition mengeksekusi disposisi per baris: restock → pindah ke FG Non-Ecommerce, reprocess → pindah ke GM
// (jurnal A-26 Dr 1311/1302, Cr 1314); repack → draft Repacking; sampel → Pemakaian Internal (Disetujui) yang
// dijurnal A-26 saat diserahkan; musnah → Pemusnahan (Disetujui) yang dijurnal A-26 setelah berita acara.
func (s *Service) PostDisposition(ctx context.Context, id uuid.UUID, version int) (domain.ReturnDisposition, error) {
	return s.mutateDisposition(ctx, id, version, domain.PermDispositionWrite, func(ctx context.Context, a actor.Actor, d *domain.ReturnDisposition) (audit.Action, error) {
		if err := d.Transition(domain.ActionExecute); err != nil {
			return "", err
		}
		locs, err := s.locationsOf(ctx, d.LocationID)
		if err != nil {
			return "", err
		}
		retur := locs[d.LocationID]
		by := docActor(a)
		amounts, err := s.executeMoves(ctx, d, retur)
		if err != nil {
			return "", err
		}
		if err := s.executeRepack(ctx, d, by); err != nil {
			return "", err
		}
		if err := s.executeSample(ctx, d, by); err != nil {
			return "", err
		}
		if err := s.executeDispose(ctx, d, by); err != nil {
			return "", err
		}
		d.CostAmount = amounts.Returned()
		d.Posted = domain.NewStep(by, s.Clock.Now())
		if !amounts.Returned().IsPositive() {
			return audit.ActionPost, nil
		}
		div, err := s.divisionByCode(ctx, DivisionLogistics)
		if err != nil {
			return "", err
		}
		d.JournalID, err = s.postJournal(ctx, accounting.PostingRequest{
			RuleCode: "A-26", SourceType: SourceReturnDisposition, SourceID: d.ID, SourceLabel: d.Number, BusinessDate: s.today(),
			Amounts: map[string]money.Amount{
				"restock_value": amountOf(amounts.Restock), "reprocess_value": amountOf(amounts.Reprocess),
				"returned_value": amountOf(amounts.Returned()),
			},
			Dimensions:  accounting.Dimensions{DivisionID: &div},
			Description: fmt.Sprintf("Disposisi Retur %s", d.Number),
		})
		return audit.ActionPost, err
	})
}

func (s *Service) executeMoves(ctx context.Context, d *domain.ReturnDisposition, retur warehouse.Location) (domain.DispositionAmounts, error) {
	amounts := domain.DispositionAmounts{Restock: decimal.Zero, Reprocess: decimal.Zero}
	var lines []domain.Line
	var idx []int
	var fg, gm *warehouse.Location
	for i, l := range d.Lines {
		var to *warehouse.Location
		switch l.Disposition {
		case domain.DispositionRestock:
			if fg == nil {
				loc, err := s.siblingLocation(ctx, retur, warehouse.LocationFinishedGoods, "locationId")
				if err != nil {
					return amounts, err
				}
				fg = &loc
			}
			to = fg
		case domain.DispositionReprocess:
			if gm == nil {
				loc, err := s.siblingLocation(ctx, retur, warehouse.LocationGoodsMaterial, "locationId")
				if err != nil {
					return amounts, err
				}
				gm = &loc
			}
			to = gm
		default:
			continue
		}
		lineID := l.ID
		lines = append(lines, domain.Line{
			Kind: domain.KindMove, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID,
			From:        &domain.Place{LocationID: d.LocationID, Status: l.StockStatus},
			To:          &domain.Place{LocationID: to.ID, Status: domain.StatusAvailable},
			BatchReason: batchReasonFor(l.BatchID, "Disposisi retur "+d.Number), Note: l.Disposition.Label(),
		})
		idx = append(idx, i)
	}
	if len(lines) == 0 {
		return amounts, nil
	}
	res, err := s.Post(ctx, domain.Posting{
		Source: domain.Source{Type: SourceReturnDisposition, ID: d.ID, Action: stockExecute, Label: d.Number, BusinessDate: s.today()},
		Lines:  lines,
	})
	if err != nil {
		return amounts, err
	}
	for k, i := range idx {
		l := &d.Lines[i]
		l.CostAmount = res.Lines[k].CostAmount
		if l.Disposition == domain.DispositionRestock {
			amounts.Restock = amounts.Restock.Add(l.CostAmount)
		} else {
			amounts.Reprocess = amounts.Reprocess.Add(l.CostAmount)
		}
	}
	return amounts, nil
}

func (s *Service) linesWith(d *domain.ReturnDisposition, disp domain.Disposition) []*domain.DispositionLine {
	var out []*domain.DispositionLine
	for i := range d.Lines {
		if d.Lines[i].Disposition == disp {
			out = append(out, &d.Lines[i])
		}
	}
	return out
}

func (s *Service) executeRepack(ctx context.Context, d *domain.ReturnDisposition, by domain.Actor) error {
	lines := s.linesWith(d, domain.DispositionRepack)
	if len(lines) == 0 {
		return nil
	}
	dispID := d.ID
	r := domain.Repacking{
		BusinessDate: s.today(), Note: "Dari Disposisi Retur " + d.Number, Status: domain.DocDraft, CostAmount: decimal.Zero,
		ReturnDispositionID: &dispID, CreatedBy: by,
	}
	for i, l := range lines {
		r.Lines = append(r.Lines, domain.RepackLine{
			LineNo: i + 1, Direction: domain.RepackInput, ProductID: l.ProductID, LocationID: d.LocationID,
			StockStatus: l.StockStatus, BatchID: l.BatchID, Qty: l.Qty, CostAmount: decimal.Zero, Note: l.QCNote,
		})
	}
	saved, err := s.Store.InsertRepacking(ctx, r)
	if err != nil {
		return err
	}
	for _, l := range lines {
		l.TargetType, l.TargetID = domain.TargetRepacking, &saved.ID
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: SourceRepacking, EntityID: saved.ID, EntityLabel: repackLabel(saved),
		After: repackSnapshot(saved), Note: "Dibuat dari Disposisi Retur " + d.Number,
	})
}

func (s *Service) executeSample(ctx context.Context, d *domain.ReturnDisposition, by domain.Actor) error {
	lines := s.linesWith(d, domain.DispositionSample)
	if len(lines) == 0 {
		return nil
	}
	div, err := s.divisionByCode(ctx, DivisionMarketing)
	if err != nil {
		return err
	}
	dispID := d.ID
	u := domain.InternalUsage{
		Purpose: domain.PurposeMarketing, DivisionID: div, SourceLocationID: d.LocationID, BusinessDate: s.today(),
		Note: "Sampel/marketing dari Disposisi Retur " + d.Number, Status: domain.DocApproved, ReturnDispositionID: &dispID,
		RequestedBy: by, EstimatedCost: decimal.Zero, CostAmount: decimal.Zero,
	}
	byProduct := map[uuid.UUID]int{}
	for _, l := range lines {
		u.EstimatedCost = u.EstimatedCost.Add(l.EstimatedCost)
		if i, ok := byProduct[l.ProductID]; ok {
			u.Lines[i].Qty = u.Lines[i].Qty.Add(l.Qty)
			continue
		}
		byProduct[l.ProductID] = len(u.Lines)
		u.Lines = append(u.Lines, domain.UsageLine{
			LineNo: len(u.Lines) + 1, ProductID: l.ProductID, Qty: l.Qty, ReturnedQty: decimal.Zero, ConsumedQty: decimal.Zero,
			CostAmount: decimal.Zero, Note: l.QCNote,
		})
	}
	if u.Number, err = s.number(ctx, document.TypeInternalUsage, u.BusinessDate); err != nil {
		return err
	}
	saved, err := s.Store.InsertUsage(ctx, u)
	if err != nil {
		return err
	}
	if err := s.Store.SaveUsage(ctx, &saved, by.ID); err != nil {
		return err
	}
	for _, l := range lines {
		l.TargetType, l.TargetID = domain.TargetInternalUsage, &saved.ID
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: SourceInternalUsage, EntityID: saved.ID, EntityLabel: saved.Number,
		After: usageSnapshot(saved), Note: "Dibuat dari Disposisi Retur " + d.Number + " (sudah disetujui)",
	})
}

func (s *Service) executeDispose(ctx context.Context, d *domain.ReturnDisposition, by domain.Actor) error {
	lines := s.linesWith(d, domain.DispositionDispose)
	if len(lines) == 0 {
		return nil
	}
	dispID := d.ID
	ds := domain.Disposal{
		Reason: domain.DisposalReturn, DisposalDate: s.today(), Witnesses: []string{}, PhotoFileIDs: []uuid.UUID{},
		Note: "Dari Disposisi Retur " + d.Number, Status: domain.DocApproved, EstimatedCost: decimal.Zero, CostAmount: decimal.Zero,
		ReturnDispositionID: &dispID, CreatedBy: by,
	}
	for i, l := range lines {
		ds.EstimatedCost = ds.EstimatedCost.Add(l.EstimatedCost)
		ds.Lines = append(ds.Lines, domain.DisposalLine{
			LineNo: i + 1, ProductID: l.ProductID, BatchID: l.BatchID, LocationID: d.LocationID, StockStatus: l.StockStatus,
			Qty: l.Qty, CostAmount: decimal.Zero, Note: l.QCNote,
		})
	}
	var err error
	if ds.Number, err = s.number(ctx, document.TypeDisposal, ds.DisposalDate); err != nil {
		return err
	}
	saved, err := s.Store.InsertDisposal(ctx, ds)
	if err != nil {
		return err
	}
	saved.Number, saved.EstimatedCost = ds.Number, ds.EstimatedCost
	if err := s.Store.SaveDisposal(ctx, &saved, by.ID); err != nil {
		return err
	}
	for _, l := range lines {
		l.TargetType, l.TargetID = domain.TargetDisposal, &saved.ID
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: SourceDisposal, EntityID: saved.ID, EntityLabel: saved.Number,
		After: disposalSnapshot(saved), Note: "Dibuat dari Disposisi Retur " + d.Number + " (sudah disetujui)",
	})
}

func (s *Service) decideDisposition(ctx context.Context, dec approval.Decided, a domain.DocAction) error {
	d, err := s.Store.GetDisposition(ctx, dec.Subject.ID, true)
	if err != nil {
		return err
	}
	before := dispositionSnapshot(d)
	if err := d.Transition(a); err != nil {
		return err
	}
	if err := s.Store.SaveDisposition(ctx, &d, dec.DecidedBy); err != nil {
		return err
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionStatusChange, EntityType: SourceReturnDisposition, EntityID: d.ID, EntityLabel: d.Number,
		Before: before, After: dispositionSnapshot(d), Note: dec.Note,
	})
}
