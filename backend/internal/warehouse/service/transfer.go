package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

func transferSnapshot(t domain.StockTransfer) map[string]any {
	lines := make([]map[string]any, len(t.Lines))
	for i, l := range t.Lines {
		lines[i] = map[string]any{"productId": l.ProductID, "batchId": l.BatchID, "qty": l.Qty.String(), "receivedQty": l.ReceivedQty.String()}
	}
	return map[string]any{
		"number": t.Number, "status": t.Status, "fromLocationId": t.FromLocationID, "toLocationId": t.ToLocationID,
		"businessDate": t.BusinessDate.String(), "note": t.Note, "lines": lines,
	}
}

func transferLabel(t domain.StockTransfer) string {
	if t.Number != "" {
		return t.Number
	}
	if t.Type == domain.TransferRequest {
		return "Draft Permintaan Petty Stock"
	}
	return "Draft Transfer Stok"
}

// requiresShipping: antar gudang fisik atau dari/ke lokasi eksternal → Dalam Pengiriman (WMS-10).
func requiresShipping(from, to domain.Location) bool {
	if from.IsExternal() || to.IsExternal() || from.WarehouseID == nil || to.WarehouseID == nil {
		return true
	}
	return *from.WarehouseID != *to.WarehouseID
}

// checkTransfer memvalidasi lokasi dan produk transfer, lalu mengisi RequiresShipping.
func (s *Service) checkTransfer(ctx context.Context, d DocumentDeps, t *domain.StockTransfer) error {
	from, err := s.activeLocation(ctx, t.FromLocationID, "fromLocationId")
	if err != nil {
		return err
	}
	to, err := s.activeLocation(ctx, t.ToLocationID, "toLocationId")
	if err != nil {
		return err
	}
	fields := map[string]string{}
	if t.Type == domain.TransferRequest {
		if from.Type != domain.LocationFinishedGoods {
			fields["fromLocationId"] = "Permintaan Petty Stock diambil dari lokasi FG Non-Ecommerce"
		}
		if to.Type != domain.LocationPettyStock {
			fields["toLocationId"] = "Tujuan Permintaan Petty Stock harus lokasi Petty Stock"
		}
		if from.WarehouseID == nil || to.WarehouseID == nil || *from.WarehouseID != *to.WarehouseID {
			fields["toLocationId"] = "Petty Stock harus di gudang yang sama dengan lokasi asal"
		}
	}
	if from.Type == domain.LocationInbound {
		fields["fromLocationId"] = "Stok Inbound dipindah lewat QC Inbound"
	}
	ids := make([]uuid.UUID, len(t.Lines))
	for i, l := range t.Lines {
		ids[i] = l.ProductID
	}
	products, err := d.Products.Products(ctx, uniq(ids))
	if err != nil {
		return err
	}
	for i, l := range t.Lines {
		p, ok := products[l.ProductID]
		switch {
		case !ok || p.Deleted():
			fields[lineFieldName(i, "productId")] = "Produk tidak ditemukan"
		case p.IsBundle:
			fields[lineFieldName(i, "productId")] = "Bundel tidak punya stok; transfer komponennya"
		case t.Type == domain.TransferRequest && p.Type != catalog.TypeFinishedGood:
			fields[lineFieldName(i, "productId")] = "Petty Stock hanya untuk Produk Jadi"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	t.RequiresShipping = requiresShipping(from, to)
	return nil
}

// CreateTransfer membuat Transfer Stok atau Permintaan Petty Stock berstatus draft.
func (s *Service) CreateTransfer(ctx context.Context, in domain.TransferInput) (TransferDetail, error) {
	a, err := actor.Require(ctx, PermTransferWrite)
	if err != nil {
		return TransferDetail{}, err
	}
	in.SourceType, in.SourceID = "", nil
	var t domain.StockTransfer
	if err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, err = s.createTransfer(ctx, in, a)
		return err
	}); err != nil {
		return TransferDetail{}, err
	}
	return s.transferDetail(ctx, t.ID)
}

func (s *Service) createTransfer(ctx context.Context, in domain.TransferInput, a actor.Actor) (domain.StockTransfer, error) {
	d, err := s.deps()
	if err != nil {
		return domain.StockTransfer{}, err
	}
	t, err := domain.NewTransfer(in, a.UserID, actorName(a))
	if err != nil {
		return domain.StockTransfer{}, err
	}
	if err := s.checkTransfer(ctx, d, &t); err != nil {
		return domain.StockTransfer{}, err
	}
	if t, err = s.Docs.InsertTransfer(ctx, t, a.UserID); err != nil {
		return domain.StockTransfer{}, err
	}
	return t, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: EntityStockTransfer, EntityID: t.ID, EntityLabel: transferLabel(t),
		After: transferSnapshot(t),
	})
}

// UpdateTransfer mengubah transfer draft.
func (s *Service) UpdateTransfer(ctx context.Context, id uuid.UUID, version int, in domain.TransferInput) (TransferDetail, error) {
	a, err := actor.Require(ctx, PermTransferWrite)
	if err != nil {
		return TransferDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return TransferDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, err := s.lockTransfer(ctx, id, version)
		if err != nil {
			return err
		}
		before := transferSnapshot(t)
		if err := t.UpdateDraft(in); err != nil {
			return err
		}
		if err := s.checkTransfer(ctx, d, &t); err != nil {
			return err
		}
		if t, err = s.Docs.UpdateTransfer(ctx, t, true, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityStockTransfer, EntityID: t.ID, EntityLabel: transferLabel(t),
			Before: before, After: transferSnapshot(t),
		})
	})
	if err != nil {
		return TransferDetail{}, err
	}
	return s.transferDetail(ctx, id)
}

func (s *Service) lockTransfer(ctx context.Context, id uuid.UUID, version int) (domain.StockTransfer, error) {
	t, err := s.Docs.GetTransfer(ctx, id, true)
	if err != nil {
		return t, err
	}
	if version != 0 && t.Version != version {
		return t, apperr.New(apperr.CodeVersionConflict)
	}
	return t, nil
}

// TransferDetail adalah detail transfer beserta data tampilan.
type TransferDetail struct {
	Transfer domain.StockTransfer
	Refs     Refs
}

// GetTransfer mengembalikan detail transfer.
func (s *Service) GetTransfer(ctx context.Context, id uuid.UUID) (TransferDetail, error) {
	if _, err := actor.Require(ctx, PermTransferRead); err != nil {
		return TransferDetail{}, err
	}
	return s.transferDetail(ctx, id)
}

func (s *Service) transferDetail(ctx context.Context, id uuid.UUID) (TransferDetail, error) {
	d, err := s.deps()
	if err != nil {
		return TransferDetail{}, err
	}
	t, err := s.Docs.GetTransfer(ctx, id, false)
	if err != nil {
		return TransferDetail{}, err
	}
	var products, batches []uuid.UUID
	for _, l := range t.Lines {
		products = append(products, l.ProductID)
		batches = append(batches, derefIDs(l.BatchID)...)
	}
	refs, err := s.refs(ctx, d, products, batches, []uuid.UUID{t.FromLocationID, t.ToLocationID})
	return TransferDetail{Transfer: t, Refs: refs}, err
}

// TransferFilter adalah kriteria daftar transfer.
type TransferFilter = postgres.TransferFilter

// ListTransfers mengembalikan daftar transfer satu jenis.
func (s *Service) ListTransfers(ctx context.Context, f TransferFilter, page paging.Page) (paging.Result[postgres.TransferSummary], Refs, error) {
	if _, err := actor.Require(ctx, PermTransferRead); err != nil {
		return paging.Result[postgres.TransferSummary]{}, Refs{}, err
	}
	d, err := s.deps()
	if err != nil {
		return paging.Result[postgres.TransferSummary]{}, Refs{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Docs.ListTransfers(ctx, f, page)
	if err != nil {
		return res, Refs{}, err
	}
	var locs []uuid.UUID
	for _, it := range res.Items {
		locs = append(locs, it.Transfer.FromLocationID, it.Transfer.ToLocationID)
	}
	refs, err := s.refs(ctx, d, nil, nil, locs)
	return res, refs, err
}

// InTransitPage adalah Stok dalam Pengiriman.
type InTransitPage struct {
	Result paging.Result[postgres.InTransitLine]
	Refs   Refs
}

// ListInTransit mengembalikan baris transfer yang belum diterima penuh.
func (s *Service) ListInTransit(ctx context.Context, locationID *uuid.UUID, page paging.Page) (InTransitPage, error) {
	if _, err := actor.Require(ctx, PermTransferRead); err != nil {
		return InTransitPage{}, err
	}
	d, err := s.deps()
	if err != nil {
		return InTransitPage{}, err
	}
	res, err := s.Docs.ListInTransit(ctx, locationID, page)
	if err != nil {
		return InTransitPage{}, err
	}
	var products, batches, locs []uuid.UUID
	for _, l := range res.Items {
		products = append(products, l.ProductID)
		batches = append(batches, derefIDs(l.BatchID)...)
		locs = append(locs, l.FromLocationID, l.ToLocationID)
	}
	refs, err := s.refs(ctx, d, products, batches, locs)
	return InTransitPage{Result: res, Refs: refs}, err
}

// TransferActions mengembalikan aksi yang boleh dilakukan actor atas transfer (allowedActions).
func TransferActions(ctx context.Context, t domain.StockTransfer) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	add := func(action, perm string) {
		if t.Can(action) && a.Has(perm) {
			out = append(out, action)
		}
	}
	add(domain.ActionEdit, PermTransferWrite)
	if t.Type == domain.TransferRequest {
		add(domain.ActionSubmit, PermTransferSubmit)
		add(domain.ActionRelease, PermTransferPost)
		add(domain.ActionPrepare, PermTransferPost)
		add(domain.ActionReceive, PermTransferSubmit)
	} else {
		add(domain.ActionPost, PermTransferPost)
		add(domain.ActionShip, PermTransferPost)
		add(domain.ActionReceive, PermTransferPost)
		add(domain.ActionComplete, PermTransferPost)
	}
	if t.Can(domain.ActionCancel) && (a.Has(PermTransferCancel) || (a.Has(PermTransferWrite) && t.RequestedBy == a.UserID)) {
		out = append(out, domain.ActionCancel)
	}
	return out
}

// TransferAction adalah aksi status transfer dari ERP.
type TransferAction struct {
	Action  string
	Version int
	// Lines: qty diterima (aksi receive).
	Lines  []domain.ReceiveLine
	Reason string
}

var transferActionPerm = map[string]string{
	domain.ActionSubmit: PermTransferSubmit, domain.ActionRelease: PermTransferPost, domain.ActionPrepare: PermTransferPost,
	domain.ActionPost: PermTransferPost, domain.ActionShip: PermTransferPost, domain.ActionComplete: PermTransferPost,
}

// ActOnTransfer menjalankan aksi status transfer: submit, release, prepare, post, ship, receive, complete, cancel.
func (s *Service) ActOnTransfer(ctx context.Context, id uuid.UUID, req TransferAction) (TransferDetail, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return TransferDetail{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, err := s.lockTransfer(ctx, id, req.Version)
		if err != nil {
			return err
		}
		perm := transferActionPerm[req.Action]
		switch req.Action {
		case domain.ActionReceive:
			perm = PermTransferPost
			if t.Type == domain.TransferRequest {
				perm = PermTransferSubmit
			}
		case domain.ActionCancel:
			perm = PermTransferCancel
			if !a.Has(perm) && a.Has(PermTransferWrite) && t.RequestedBy == a.UserID {
				perm = PermTransferWrite
			}
		}
		if perm == "" {
			return apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
		}
		if !a.Has(perm) {
			return apperr.New(apperr.CodeForbidden)
		}
		_, err = s.applyTransferAction(ctx, &t, req, a)
		return err
	})
	if err != nil {
		return TransferDetail{}, err
	}
	return s.transferDetail(ctx, id)
}

// applyTransferAction menjalankan aksi dan efek stoknya di transaksi aktif tanpa cek permission.
func (s *Service) applyTransferAction(ctx context.Context, t *domain.StockTransfer, req TransferAction, a actor.Actor) (StockResult, error) {
	d, err := s.deps()
	if err != nil {
		return StockResult{}, err
	}
	before := transferSnapshot(*t)
	now := s.now()
	by, name := a.UserID, actorName(a)
	replace := false
	var result StockResult
	src := func(action string) StockSource {
		return StockSource{Type: EntityStockTransfer, ID: t.ID, Action: action, Label: t.Number, BusinessDate: t.BusinessDate}
	}
	ensureNumber := func() error {
		if t.Number != "" {
			return nil
		}
		n, err := d.Numbers.Next(ctx, document.TypeStockTransfer, t.BusinessDate)
		t.Number = n
		return err
	}
	auditAction := audit.ActionStatusChange
	note := ""
	switch req.Action {
	case domain.ActionSubmit:
		if err := t.Transition(req.Action); err != nil {
			return result, err
		}
		if err := ensureNumber(); err != nil {
			return result, err
		}
		t.SubmittedAt, auditAction = nowPtr(now), audit.ActionSubmit
		s.notify(ctx, d, s.roleUsers(ctx, d, RoleLogisticsLeader), "warehouse.petty_stock_request",
			"Permintaan Petty Stock "+t.Number, name+" meminta Petty Stock; mohon di-release.", transferLink(*t))
	case domain.ActionRelease:
		if err := t.Transition(req.Action); err != nil {
			return result, err
		}
		lines := make([]StockReserveLine, len(t.Lines))
		for i, l := range t.Lines {
			lineID := l.ID
			lines[i] = StockReserveLine{SourceLineID: &lineID, ProductID: l.ProductID, LocationID: t.FromLocationID, BatchID: l.BatchID, Qty: l.Qty, BatchReason: l.BatchReason}
		}
		if result, err = d.Ledger.Reserve(ctx, src("reserve"), lines); err != nil {
			return result, err
		}
		t.ReleasedBy, t.ReleasedByName, t.ReleasedAt = &by, name, nowPtr(now)
		note = "Stok asal dialokasikan (FEFO)"
	case domain.ActionPrepare:
		if err := t.Transition(req.Action); err != nil {
			return result, err
		}
		t.PreparedBy, t.PreparedByName, t.PreparedAt = &by, name, nowPtr(now)
		s.notify(ctx, d, []uuid.UUID{t.RequestedBy}, "warehouse.petty_stock_prepared",
			"Petty Stock "+t.Number+" siap diserahkan", "Barang sudah disiapkan "+name+"; cek ulang dan konfirmasi qty diterima.", transferLink(*t))
	case domain.ActionPost, domain.ActionShip:
		if err := t.Transition(req.Action); err != nil {
			return result, err
		}
		if err := ensureNumber(); err != nil {
			return result, err
		}
		status := StockAvailable
		if req.Action == domain.ActionShip {
			status = StockInTransit
			t.ShippedBy, t.ShippedByName, t.ShippedAt = &by, name, nowPtr(now)
		} else {
			t.CompletedAt = nowPtr(now)
			t.ReceivedBy, t.ReceivedByName, t.ReceivedAt = &by, name, nowPtr(now)
		}
		lines := make([]StockLine, len(t.Lines))
		for i, l := range t.Lines {
			lineID := l.ID
			lines[i] = StockLine{
				Kind: StockMove, SourceLineID: &lineID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty,
				From: &StockPlace{LocationID: t.FromLocationID, Status: StockAvailable},
				To:   &StockPlace{LocationID: t.ToLocationID, Status: status}, BatchReason: l.BatchReason, Note: l.Note,
			}
		}
		if result, err = d.Ledger.Post(ctx, src(req.Action), lines); err != nil {
			return result, err
		}
		t.Lines = splitByBatch(t.Lines, result, t.ToLocationID, req.Action == domain.ActionPost)
		replace, auditAction = true, audit.ActionPost
	case domain.ActionReceive:
		if t.Type == domain.TransferRequest {
			result, err = s.confirmRequest(ctx, d, t, req.Lines, src)
		} else {
			result, err = s.receiveDirect(ctx, d, t, req.Lines, src, now)
		}
		if err != nil {
			return result, err
		}
		t.ReceivedBy, t.ReceivedByName, t.ReceivedAt = &by, name, nowPtr(now)
		if t.Status == domain.StatusCompleted {
			t.CompletedAt = nowPtr(now)
		}
		auditAction = audit.ActionPost
	case domain.ActionComplete:
		shortage, err := t.CloseShortage()
		if err != nil {
			return result, err
		}
		var lines []StockLine
		for _, l := range t.Lines {
			q, ok := shortage[l.ID]
			if !ok {
				continue
			}
			lineID := l.ID
			lines = append(lines, StockLine{
				Kind: StockMove, SourceLineID: &lineID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: q,
				From: &StockPlace{LocationID: t.ToLocationID, Status: StockInTransit},
				To:   &StockPlace{LocationID: t.ToLocationID, Status: StockAvailable},
				Note: "Kekurangan transfer; diselesaikan lewat Stock Count lokasi tujuan",
			})
		}
		if len(lines) > 0 {
			if result, err = d.Ledger.Post(ctx, src("complete"), lines); err != nil {
				return result, err
			}
			note = "Kekurangan dicatat; selisih diproses lewat Stock Count lokasi tujuan"
		}
		t.CompletedAt = nowPtr(now)
	case domain.ActionCancel:
		if err := t.Cancel(req.Reason, now); err != nil {
			return result, err
		}
		auditAction = audit.ActionCancel
	default:
		return result, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	saved, err := s.Docs.UpdateTransfer(ctx, *t, replace, by)
	if err != nil {
		return result, err
	}
	*t = saved
	return result, s.Audit.Record(ctx, audit.Entry{
		Action: auditAction, EntityType: EntityStockTransfer, EntityID: t.ID, EntityLabel: transferLabel(*t),
		Before: before, After: transferSnapshot(*t), Note: joinNotes(req.Action, note, req.Reason),
	})
}

func joinNotes(action string, parts ...string) string {
	out := "Aksi " + action
	for _, p := range parts {
		if p != "" {
			out += ". " + p
		}
	}
	return out
}

func transferLink(t domain.StockTransfer) string {
	if t.Type == domain.TransferRequest {
		return "/warehouse/petty-stock-requests/" + t.ID.String()
	}
	return "/warehouse/stock-transfers/" + t.ID.String()
}

// splitByBatch memecah baris menjadi satu baris per batch hasil alokasi (movement masuk di lokasi tujuan)
// beserta nilai FIFO-nya; received true = qty langsung diterima (transfer dalam gudang).
func splitByBatch(lines []domain.TransferLine, res StockResult, to uuid.UUID, received bool) []domain.TransferLine {
	var out []domain.TransferLine
	for i, l := range lines {
		if i >= len(res.Lines) {
			out = append(out, l)
			continue
		}
		var parts []StockMovement
		for _, m := range res.Lines[i].Movements {
			if m.LocationID == to && m.Qty.IsPositive() {
				parts = append(parts, m)
			}
		}
		if len(parts) == 0 {
			out = append(out, l)
			continue
		}
		for _, m := range parts {
			n := l
			n.ID, n.BatchID, n.Qty, n.CostAmount = uuid.Nil, m.BatchID, m.Qty, m.CostAmount
			n.ReceivedQty, n.ShortageQty = decimal.Zero, decimal.Zero
			if received {
				n.ReceivedQty = m.Qty
			}
			out = append(out, n)
		}
	}
	return out
}

func (s *Service) receiveDirect(ctx context.Context, d DocumentDeps, t *domain.StockTransfer, lines []domain.ReceiveLine, src func(string) StockSource, now time.Time) (StockResult, error) {
	got, err := t.ApplyReceive(lines)
	if err != nil {
		return StockResult{}, err
	}
	var post []StockLine
	for _, l := range t.Lines {
		q, ok := got[l.ID]
		if !ok || !q.IsPositive() {
			continue
		}
		lineID := l.ID
		post = append(post, StockLine{
			Kind: StockMove, SourceLineID: &lineID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: q,
			From: &StockPlace{LocationID: t.ToLocationID, Status: StockInTransit},
			To:   &StockPlace{LocationID: t.ToLocationID, Status: StockAvailable}, Note: "Diterima",
		})
	}
	return d.Ledger.Post(ctx, src("receive:"+now.UTC().Format("20060102T150405.000000")), post)
}

func (s *Service) confirmRequest(ctx context.Context, d DocumentDeps, t *domain.StockTransfer, lines []domain.ReceiveLine, src func(string) StockSource) (StockResult, error) {
	got, err := t.ConfirmRequest(lines)
	if err != nil {
		return StockResult{}, err
	}
	var post []StockLine
	for _, l := range t.Lines {
		q := got[l.ID]
		if !q.IsPositive() {
			continue
		}
		lineID := l.ID
		post = append(post, StockLine{
			Kind: StockMove, SourceLineID: &lineID, ProductID: l.ProductID, Qty: q,
			From:        &StockPlace{LocationID: t.FromLocationID, Status: StockReserved},
			To:          &StockPlace{LocationID: t.ToLocationID, Status: StockAvailable},
			Reservation: &StockReservationRef{SourceType: EntityStockTransfer, SourceID: t.ID, SourceLineID: &lineID},
			Note:        "Diterima Fulfillment",
		})
	}
	var res StockResult
	if len(post) > 0 {
		if res, err = d.Ledger.Post(ctx, src("receive"), post); err != nil {
			return res, err
		}
		for i, l := range post {
			for j := range t.Lines {
				if t.Lines[j].ID == *l.SourceLineID && i < len(res.Lines) {
					t.Lines[j].CostAmount = res.Lines[i].CostAmount
				}
			}
		}
	}
	// Sisa yang tidak diterima tetap di lokasi asal: reservasinya dilepas kembali ke Tersedia.
	return res, d.Ledger.Release(ctx, src("release"), StockReservationRef{SourceType: EntityStockTransfer, SourceID: t.ID})
}

// --- API publik (modul lain) ------------------------------------------------------------------

// TransferResult adalah hasil transfer lewat API: dokumen beserta nilai FIFO barang yang dipindah per baris
// (jurnal A-07/A-24 dibuat modul pemanggil).
type TransferResult struct {
	Transfer  domain.StockTransfer
	TotalCost decimal.Decimal
}

func totalCost(t domain.StockTransfer) decimal.Decimal {
	total := decimal.Zero
	for _, l := range t.Lines {
		total = total.Add(l.CostAmount)
	}
	return total
}

// TransferStock membuat dan langsung menjalankan transfer langsung di transaksi aktif tanpa cek permission:
// dalam satu gudang diposting (Tersedia di tujuan), antar gudang/eksternal dikirim (Dalam Pengiriman di tujuan).
// in.SourceType/SourceID wajib (mis. delivery_order) dan unik.
func (s *Service) TransferStock(ctx context.Context, in domain.TransferInput) (TransferResult, error) {
	if in.SourceType == "" {
		return TransferResult{}, apperr.Validation(map[string]string{"source": "Dokumen sumber wajib diisi"})
	}
	in.Type = domain.TransferDirect
	a := systemActor(ctx)
	var t domain.StockTransfer
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if t, err = s.createTransfer(ctx, in, a); err != nil {
			return err
		}
		action := domain.ActionPost
		if t.RequiresShipping {
			action = domain.ActionShip
		}
		_, err = s.applyTransferAction(ctx, &t, TransferAction{Action: action}, a)
		return err
	})
	return TransferResult{Transfer: t, TotalCost: totalCost(t)}, err
}

// ReceiveTransfer mencatat penerimaan transfer Dalam Pengiriman tanpa cek permission (mis. mitra konfirmasi DO).
// complete true = sisa yang tidak diterima dicatat sebagai kekurangan dan transfer ditutup.
func (s *Service) ReceiveTransfer(ctx context.Context, transferID uuid.UUID, lines []domain.ReceiveLine, complete bool) (TransferResult, error) {
	a := systemActor(ctx)
	var t domain.StockTransfer
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if t, err = s.Docs.GetTransfer(ctx, transferID, true); err != nil {
			return err
		}
		if len(lines) > 0 {
			if _, err := s.applyTransferAction(ctx, &t, TransferAction{Action: domain.ActionReceive, Lines: lines}, a); err != nil {
				return err
			}
		}
		if complete && t.Can(domain.ActionComplete) {
			_, err = s.applyTransferAction(ctx, &t, TransferAction{Action: domain.ActionComplete}, a)
		}
		return err
	})
	return TransferResult{Transfer: t, TotalCost: totalCost(t)}, err
}

// TransferByID membaca transfer tanpa cek permission (API publik).
func (s *Service) TransferByID(ctx context.Context, id uuid.UUID) (domain.StockTransfer, error) {
	if _, err := s.deps(); err != nil {
		return domain.StockTransfer{}, err
	}
	return s.Docs.GetTransfer(ctx, id, false)
}

// systemActor memakai actor dari context bila ada; selain itu actor sistem (job/API modul lain).
func systemActor(ctx context.Context) actor.Actor {
	if a, ok := actor.From(ctx); ok {
		return a
	}
	return actor.Actor{UserID: uuid.Nil, FullName: "Sistem"}
}
