package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
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

// UsageView adalah detail Pemakaian Internal beserta master rujukan, batch yang keluar, dan aksi.
type UsageView struct {
	Usage          domain.InternalUsage
	Refs           DocRefs
	Movements      []postgres.SourceMovement
	AllowedActions []domain.DocAction
}

// UsagePage adalah satu halaman daftar.
type UsagePage struct {
	paging.Result[domain.InternalUsage]
	Refs DocRefs
}

func usageSnapshot(u domain.InternalUsage) map[string]any {
	lines := make([]map[string]any, len(u.Lines))
	for i, l := range u.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "qty": l.Qty.String(), "returnedQty": l.ReturnedQty.String(),
			"consumedQty": l.ConsumedQty.String(), "costAmount": l.CostAmount.StringFixed(2), "note": l.Note,
		}
	}
	m := map[string]any{
		"number": u.Number, "purpose": u.Purpose, "divisionId": u.DivisionID, "campaignId": u.CampaignID,
		"sourceLocationId": u.SourceLocationID, "eventLocationId": u.EventLocationID, "businessDate": u.BusinessDate.String(),
		"status": u.Status, "estimatedCost": u.EstimatedCost.StringFixed(2), "costAmount": u.CostAmount.StringFixed(2),
		"note": u.Note, "lines": lines,
	}
	if u.EventEndDate != nil {
		m["eventEndDate"] = u.EventEndDate.String()
	}
	return m
}

func usageRefIDs(docs ...domain.InternalUsage) refIDs {
	var ids refIDs
	for _, u := range docs {
		ids.locations = append(ids.locations, u.SourceLocationID)
		if u.EventLocationID != nil {
			ids.locations = append(ids.locations, *u.EventLocationID)
		}
		ids.dimensions = append(ids.dimensions, u.DivisionID)
		if u.CampaignID != nil {
			ids.dimensions = append(ids.dimensions, *u.CampaignID)
		}
		for _, l := range u.Lines {
			ids.products = append(ids.products, l.ProductID)
		}
	}
	return ids
}

// ListUsages mengembalikan daftar Pemakaian Internal.
func (s *Service) ListUsages(ctx context.Context, f postgres.DocFilter, page paging.Page) (UsagePage, error) {
	if _, err := actor.Require(ctx, domain.PermUsageRead); err != nil {
		return UsagePage{}, err
	}
	res, err := s.Store.ListUsages(ctx, f, page)
	if err != nil {
		return UsagePage{}, err
	}
	refs, err := s.docRefs(ctx, usageRefIDs(res.Items...))
	if err != nil {
		return UsagePage{}, err
	}
	return UsagePage{Result: res, Refs: refs}, nil
}

// GetUsage mengembalikan detail Pemakaian Internal.
func (s *Service) GetUsage(ctx context.Context, id uuid.UUID) (UsageView, error) {
	a, err := actor.Require(ctx, domain.PermUsageRead)
	if err != nil {
		return UsageView{}, err
	}
	u, err := s.Store.GetUsage(ctx, id, false)
	if err != nil {
		return UsageView{}, err
	}
	return s.usageView(ctx, a, u)
}

func (s *Service) usageView(ctx context.Context, a actor.Actor, u domain.InternalUsage) (UsageView, error) {
	movs, err := s.sourceMovements(ctx, SourceInternalUsage, u.ID)
	if err != nil {
		return UsageView{}, err
	}
	ids := usageRefIDs(u)
	for _, m := range movs {
		ids.locations = append(ids.locations, m.LocationID)
		if m.BatchID != nil {
			ids.batches = append(ids.batches, *m.BatchID)
		}
	}
	refs, err := s.docRefs(ctx, ids)
	if err != nil {
		return UsageView{}, err
	}
	return UsageView{Usage: u, Refs: refs, Movements: movs, AllowedActions: u.AllowedActions(a.Has)}, nil
}

// checkUsageMasters memeriksa lokasi, produk, divisi, dan kampanye dokumen.
func (s *Service) checkUsageMasters(ctx context.Context, u domain.InternalUsage) error {
	ids := []uuid.UUID{u.SourceLocationID}
	if u.EventLocationID != nil {
		ids = append(ids, *u.EventLocationID)
	}
	locs, err := s.locationsOf(ctx, ids...)
	if err != nil {
		return err
	}
	sourceTypes := []warehouse.LocationType{warehouse.LocationFinishedGoods, warehouse.LocationEcommerce, warehouse.LocationType("petty_stock")}
	if u.ReturnDispositionID != nil {
		sourceTypes = []warehouse.LocationType{warehouse.LocationReturns}
	}
	if _, err := requireLocation(locs, u.SourceLocationID, "sourceLocationId", sourceTypes...); err != nil {
		return err
	}
	if u.EventLocationID != nil {
		if _, err := requireLocation(locs, *u.EventLocationID, "eventLocationId", warehouse.LocationEvent); err != nil {
			return err
		}
	}
	pids := make([]uuid.UUID, len(u.Lines))
	for i, l := range u.Lines {
		pids[i] = l.ProductID
	}
	prods, err := s.productsOf(ctx, pids...)
	if err != nil {
		return err
	}
	if err := requireProducts(prods, pids, func(i int) string { return lineField(i, "productId") }, catalog.TypeFinishedGood); err != nil {
		return err
	}
	return s.checkDimensions(ctx, u)
}

func (s *Service) checkDimensions(ctx context.Context, u domain.InternalUsage) error {
	if err := requireDep(s.Dimensions != nil, "accounting"); err != nil {
		return err
	}
	divs, err := s.Dimensions.DimensionOptions(ctx, DimensionDivision, false)
	if err != nil {
		return err
	}
	if !containsOption(divs, u.DivisionID) {
		return apperr.Validation(map[string]string{"divisionId": "Divisi tidak ditemukan atau nonaktif"})
	}
	if u.CampaignID == nil {
		return nil
	}
	camps, err := s.Dimensions.DimensionOptions(ctx, DimensionCampaign, false)
	if err != nil {
		return err
	}
	for _, c := range camps {
		if c.ID != *u.CampaignID {
			continue
		}
		if (c.ValidFrom != nil && u.BusinessDate.Before(*c.ValidFrom)) || (c.ValidTo != nil && u.BusinessDate.After(*c.ValidTo)) {
			return apperr.Validation(map[string]string{"campaignId": "Kampanye " + c.Code + " tidak berlaku pada tanggal pemakaian"})
		}
		return nil
	}
	return apperr.Validation(map[string]string{"campaignId": "Kampanye tidak ditemukan atau nonaktif"})
}

func containsOption(opts []DimensionOption, id uuid.UUID) bool {
	for _, o := range opts {
		if o.ID == id {
			return true
		}
	}
	return false
}

// CreateUsage membuat draft Pemakaian Internal (peminta: Marketing, Sales, HRGA).
func (s *Service) CreateUsage(ctx context.Context, in domain.InternalUsageInput) (domain.InternalUsage, error) {
	a, err := actor.Require(ctx, domain.PermUsageWrite)
	if err != nil {
		return domain.InternalUsage{}, err
	}
	u, err := domain.NewInternalUsage(in, docActor(a))
	if err != nil {
		return domain.InternalUsage{}, err
	}
	if err := s.checkUsageMasters(ctx, u); err != nil {
		return domain.InternalUsage{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if u, err = s.Store.InsertUsage(ctx, u); err != nil {
			return err
		}
		return s.record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: SourceInternalUsage, EntityID: u.ID, EntityLabel: "Draft Pemakaian Internal",
			After: usageSnapshot(u),
		})
	})
	return u, err
}

// mutateUsage mengunci dokumen, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit trail.
func (s *Service) mutateUsage(ctx context.Context, id uuid.UUID, version int, perm string,
	fn func(ctx context.Context, a actor.Actor, u *domain.InternalUsage) (audit.Action, error),
) (domain.InternalUsage, error) {
	a, err := actor.Require(ctx, perm)
	if err != nil {
		return domain.InternalUsage{}, err
	}
	var out domain.InternalUsage
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		u, err := s.Store.GetUsage(ctx, id, true)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(u.Version, version); err != nil {
			return err
		}
		before := usageSnapshot(u)
		action, err := fn(ctx, a, &u)
		if err != nil {
			return err
		}
		if err := s.Store.SaveUsage(ctx, &u, a.UserID); err != nil {
			return err
		}
		out = u
		return s.record(ctx, audit.Entry{
			Action: action, EntityType: SourceInternalUsage, EntityID: u.ID, EntityLabel: usageLabel(u),
			Before: before, After: usageSnapshot(u),
		})
	})
	return out, err
}

func usageLabel(u domain.InternalUsage) string {
	if u.Number != "" {
		return u.Number
	}
	return "Draft Pemakaian Internal"
}

// UpdateUsage mengubah draft.
func (s *Service) UpdateUsage(ctx context.Context, id uuid.UUID, version int, in domain.InternalUsageInput) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageWrite, func(ctx context.Context, _ actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Update(in); err != nil {
			return "", err
		}
		if err := s.checkUsageMasters(ctx, *u); err != nil {
			return "", err
		}
		return audit.ActionUpdate, s.Store.ReplaceUsageLines(ctx, u)
	})
}

// SubmitUsage memberi nomor PU, menaksir nilai HPP (FEFO), dan mengajukan approval `internal_usage`.
func (s *Service) SubmitUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageSubmit, func(ctx context.Context, _ actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Transition(domain.ActionSubmit); err != nil {
			return "", err
		}
		if err := s.checkUsageMasters(ctx, *u); err != nil {
			return "", err
		}
		estimate, err := s.estimateUsage(ctx, *u)
		if err != nil {
			return "", err
		}
		u.EstimatedCost = estimate
		if u.Number == "" {
			if u.Number, err = s.number(ctx, document.TypeInternalUsage, u.BusinessDate); err != nil {
				return "", err
			}
		}
		div := u.DivisionID
		return audit.ActionSubmit, s.requestApproval(ctx, approval.Subject{
			Type: approval.SubjectInternalUsage, ID: u.ID, Label: u.Number, Link: "/inventory/internal-usages/" + u.ID.String(),
			Summary: fmt.Sprintf("%s · %d baris", u.Purpose.Label(), len(u.Lines)), Amount: amountOf(estimate), Division: &div,
		})
	})
}

func (s *Service) estimateUsage(ctx context.Context, u domain.InternalUsage) (decimal.Decimal, error) {
	pids := make([]uuid.UUID, len(u.Lines))
	for i, l := range u.Lines {
		pids[i] = l.ProductID
	}
	prods, err := s.productsOf(ctx, pids...)
	if err != nil {
		return decimal.Zero, err
	}
	locs, err := s.locationsOf(ctx, u.SourceLocationID)
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	from := domain.Place{LocationID: u.SourceLocationID, Status: domain.StatusAvailable}
	for _, l := range u.Lines {
		c, err := s.estimateIssue(ctx, l.ProductID, from, nil, l.Qty, prods[l.ProductID], locs[u.SourceLocationID])
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(c)
	}
	return total, nil
}

// ReviseUsage mengembalikan dokumen yang ditolak ke draft.
func (s *Service) ReviseUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageWrite, func(_ context.Context, _ actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		return audit.ActionStatusChange, u.Transition(domain.ActionRevise)
	})
}

// CancelUsage membatalkan draft atau dokumen yang sudah disetujui (belum ada mutasi stok).
func (s *Service) CancelUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageWrite, func(_ context.Context, _ actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		return audit.ActionCancel, u.Transition(domain.ActionCancel)
	})
}

func (s *Service) usageSource(u domain.InternalUsage, action string) domain.Source {
	return domain.Source{Type: SourceInternalUsage, ID: u.ID, Action: action, Label: u.Number, BusinessDate: s.today()}
}

// ReleaseUsage: Logistik Leader meneruskan dokumen yang disetujui; stok asal dialokasikan FEFO (reserved).
func (s *Service) ReleaseUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsagePost, func(ctx context.Context, a actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Transition(domain.ActionRelease); err != nil {
			return "", err
		}
		lines := make([]ReserveLine, len(u.Lines))
		for i, l := range u.Lines {
			lineID := l.ID
			lines[i] = ReserveLine{SourceLineID: &lineID, ProductID: l.ProductID, LocationID: u.SourceLocationID, Qty: l.Qty}
		}
		if _, err := s.Reserve(ctx, ReserveRequest{Source: s.usageSource(*u, stockReserve), Lines: lines}); err != nil {
			return "", err
		}
		u.Released = domain.NewStep(docActor(a), s.Clock.Now())
		return audit.ActionStatusChange, nil
	})
}

// HandoverUsage: divisi peminta mengecek ulang dan mengonfirmasi serah terima. Non-event: stok keluar dari
// alokasi dan jurnal A-11/A-12 (atau A-26 untuk sampel dari Gudang Retur). Event: stok pindah ke lokasi Event
// tanpa jurnal, status Menunggu Penyelesaian.
func (s *Service) HandoverUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageWrite, func(ctx context.Context, a actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Transition(domain.ActionHandover); err != nil {
			return "", err
		}
		now := s.Clock.Now()
		u.HandedOver = domain.NewStep(docActor(a), now)
		lines := make([]domain.Line, len(u.Lines))
		for i, l := range u.Lines {
			lineID := l.ID
			lines[i] = domain.Line{
				Kind: domain.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty,
				From:        &domain.Place{LocationID: u.SourceLocationID, Status: domain.StatusReserved},
				Reservation: &domain.ReservationRef{SourceType: SourceInternalUsage, SourceID: u.ID, SourceLineID: &lineID},
			}
			if u.IsEvent() {
				lines[i].Kind = domain.KindMove
				lines[i].To = &domain.Place{LocationID: *u.EventLocationID, Status: domain.StatusAvailable}
			}
		}
		if u.IsEvent() {
			_, err := s.Post(ctx, domain.Posting{Source: s.usageSource(*u, stockDispatch), Lines: lines})
			return audit.ActionStatusChange, err
		}
		res, err := s.Post(ctx, domain.Posting{Source: s.usageSource(*u, stockIssue), Lines: lines})
		if err != nil {
			return "", err
		}
		for i := range u.Lines {
			u.Lines[i].ConsumedQty = u.Lines[i].Qty
			u.Lines[i].CostAmount = res.Lines[i].CostAmount
		}
		u.Posted = domain.NewStep(docActor(a), now)
		return audit.ActionPost, s.journalUsage(ctx, u)
	})
}

// journalUsage memposting biaya pemakaian sebesar HPP qty terpakai.
func (s *Service) journalUsage(ctx context.Context, u *domain.InternalUsage) error {
	total := decimal.Zero
	for _, l := range u.Lines {
		total = total.Add(l.CostAmount)
	}
	u.CostAmount = total
	if !total.IsPositive() {
		return nil
	}
	locs, err := s.locationsOf(ctx, u.SourceLocationID)
	if err != nil {
		return err
	}
	div := u.DivisionID
	req := accounting.PostingRequest{
		RuleCode: u.Purpose.PostingRule(), SourceType: SourceInternalUsage, SourceID: u.ID, SourceLabel: u.Number,
		BusinessDate: s.today(), Amounts: map[string]money.Amount{"usage_value": amountOf(total)},
		Dimensions:  accounting.Dimensions{DivisionID: &div, CampaignID: u.CampaignID},
		Description: fmt.Sprintf("Pemakaian Internal %s (%s)", u.Number, u.Purpose.Label()),
	}
	if locs[u.SourceLocationID].Type == warehouse.LocationReturns {
		// Sampel dari Gudang Retur: A-26 Dr Beban Marketing — Sampel (6103), Cr Persediaan Retur (1314).
		req.RuleCode = "A-26"
		req.Amounts = map[string]money.Amount{"sample_value": amountOf(total), "returned_value": amountOf(total)}
	}
	u.JournalID, err = s.postJournal(ctx, req)
	return err
}

// SettleUsage mencatat qty sisa event (maks. H+1): sisa dipindah dari lokasi Event ke Inbound berstatus Karantina
// untuk dicek ulang. Batch dengan kedaluwarsa paling akhir dianggap sisa.
func (s *Service) SettleUsage(ctx context.Context, id uuid.UUID, version int, lines []domain.SettleLine) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsageWrite, func(ctx context.Context, a actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Transition(domain.ActionSettle); err != nil {
			return "", err
		}
		if err := u.ApplySettlement(lines); err != nil {
			return "", err
		}
		u.Settled = domain.NewStep(docActor(a), s.Clock.Now())
		sent, err := s.dispatchedBatches(ctx, *u)
		if err != nil {
			return "", err
		}
		locs, err := s.locationsOf(ctx, *u.EventLocationID, u.SourceLocationID)
		if err != nil {
			return "", err
		}
		inbound, err := s.siblingLocation(ctx, locs[u.SourceLocationID], warehouse.LocationInbound, "sourceLocationId")
		if err != nil {
			return "", err
		}
		var moves []domain.Line
		for _, l := range u.Lines {
			lineID := l.ID
			for _, al := range domain.DistributeReturn(sent[l.ID], l.ReturnedQty) {
				moves = append(moves, domain.Line{
					Kind: domain.KindMove, SourceLineID: &lineID, ProductID: l.ProductID, Qty: al.Qty, BatchID: al.Candidate.BatchID,
					From:        &domain.Place{LocationID: *u.EventLocationID, Status: domain.StatusAvailable},
					To:          &domain.Place{LocationID: inbound.ID, Status: domain.StatusQuarantine},
					BatchReason: "Sisa event " + u.Number, Note: "Sisa event dikembalikan ke Inbound untuk dicek",
				})
			}
		}
		if len(moves) > 0 {
			if _, err := s.Post(ctx, domain.Posting{Source: s.usageSource(*u, stockSettle), Lines: moves}); err != nil {
				return "", err
			}
		}
		return audit.ActionUpdate, nil
	})
}

// dispatchedBatches mengembalikan batch yang dikirim ke lokasi Event per baris, urut FEFO.
func (s *Service) dispatchedBatches(ctx context.Context, u domain.InternalUsage) (map[uuid.UUID][]domain.Candidate, error) {
	movs, err := s.Store.SourceMovements(ctx, SourceInternalUsage, u.ID, stockDispatch)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]domain.Candidate{}
	for _, m := range movs {
		if m.Type != domain.MovementMoveIn || m.SourceLineID == nil {
			continue
		}
		out[*m.SourceLineID] = addCandidate(out[*m.SourceLineID], m, m.Qty)
	}
	for k := range out {
		domain.SortFEFO(out[k])
	}
	return out, nil
}

func addCandidate(cands []domain.Candidate, m postgres.SourceMovement, qty decimal.Decimal) []domain.Candidate {
	for i := range cands {
		if domain.SameBatch(cands[i].BatchID, m.BatchID) {
			cands[i].Qty = cands[i].Qty.Add(qty)
			return cands
		}
	}
	return append(cands, domain.Candidate{BatchID: m.BatchID, BatchNumber: m.BatchNumber, ExpiryDate: m.ExpiryDate, Qty: qty})
}

// PostUsage memposting pemakaian event setelah sisa dicatat dan dicek: qty terpakai (kirim − sisa) keluar dari
// lokasi Event dan dijurnal A-11 dengan kode kampanye.
func (s *Service) PostUsage(ctx context.Context, id uuid.UUID, version int) (domain.InternalUsage, error) {
	return s.mutateUsage(ctx, id, version, domain.PermUsagePost, func(ctx context.Context, a actor.Actor, u *domain.InternalUsage) (audit.Action, error) {
		if err := u.Transition(domain.ActionExecute); err != nil {
			return "", err
		}
		sent, err := s.dispatchedBatches(ctx, *u)
		if err != nil {
			return "", err
		}
		settled, err := s.Store.SourceMovements(ctx, SourceInternalUsage, u.ID, stockSettle)
		if err != nil {
			return "", err
		}
		returned := map[uuid.UUID][]domain.Candidate{}
		for _, m := range settled {
			if m.Type == domain.MovementMoveOut && m.SourceLineID != nil {
				returned[*m.SourceLineID] = addCandidate(returned[*m.SourceLineID], m, m.Qty.Neg())
			}
		}
		var issues []domain.Line
		lineOf := map[int]int{}
		for li, l := range u.Lines {
			lineID := l.ID
			for _, c := range sent[l.ID] {
				consumed := c.Qty
				for _, r := range returned[l.ID] {
					if domain.SameBatch(r.BatchID, c.BatchID) {
						consumed = consumed.Sub(r.Qty)
					}
				}
				if !consumed.IsPositive() {
					continue
				}
				lineOf[len(issues)] = li
				issues = append(issues, domain.Line{
					Kind: domain.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: consumed, BatchID: c.BatchID,
					From:        &domain.Place{LocationID: *u.EventLocationID, Status: domain.StatusAvailable},
					BatchReason: "Pemakaian event " + u.Number,
				})
			}
			u.Lines[li].CostAmount = decimal.Zero
		}
		if len(issues) > 0 {
			res, err := s.Post(ctx, domain.Posting{Source: s.usageSource(*u, stockExecute), Lines: issues})
			if err != nil {
				return "", err
			}
			for i, lr := range res.Lines {
				li := lineOf[i]
				u.Lines[li].CostAmount = u.Lines[li].CostAmount.Add(lr.CostAmount)
			}
		}
		u.Posted = domain.NewStep(docActor(a), s.Clock.Now())
		return audit.ActionPost, s.journalUsage(ctx, u)
	})
}

// decideUsage dipanggil engine approval di transaksi keputusan.
func (s *Service) decideUsage(ctx context.Context, d approval.Decided, a domain.DocAction) error {
	u, err := s.Store.GetUsage(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	before := usageSnapshot(u)
	if err := u.Transition(a); err != nil {
		return err
	}
	if err := s.Store.SaveUsage(ctx, &u, d.DecidedBy); err != nil {
		return err
	}
	return s.record(ctx, audit.Entry{
		Action: audit.ActionStatusChange, EntityType: SourceInternalUsage, EntityID: u.ID, EntityLabel: u.Number,
		Before: before, After: usageSnapshot(u), Note: d.Note,
	})
}

// RemindSettlements mengirim pengingat pencatatan sisa event yang lewat batas H+1 ke peminta dan Logistik Leader
// (job harian internal_usage.settle_reminder). Satu pengingat per dokumen per hari.
func (s *Service) RemindSettlements(ctx context.Context) (int, error) {
	today := s.today()
	ids, err := s.Store.SettlementDueUsages(ctx, today)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	ctx = actor.System(ctx)
	var leaders []uuid.UUID
	if s.Members != nil {
		if leaders, err = s.Members.UserIDsWithRole(ctx, "logistics_leader"); err != nil {
			return 0, err
		}
	}
	n := 0
	for _, id := range ids {
		err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
			u, err := s.Store.GetUsage(ctx, id, true)
			if err != nil {
				return err
			}
			if s.Notifier != nil {
				to := append([]uuid.UUID{u.RequestedBy.ID}, leaders...)
				body := fmt.Sprintf("Sisa barang event %s wajib dicatat paling lambat %s (H+1 setelah event).", u.Number, u.SettleDueDate.String())
				if err := s.Notifier.Notify(ctx, uniq(to), "internal_usage_settle_due", "Catat sisa barang event "+u.Number,
					body, "/inventory/internal-usages/"+u.ID.String()); err != nil {
					return err
				}
			}
			d := today
			u.RemindedOn = &d
			return s.Store.SaveUsage(ctx, &u, uuid.Nil)
		})
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
