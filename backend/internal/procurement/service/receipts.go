package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// ReceiptFilter adalah kriteria daftar Penerimaan Barang.
type ReceiptFilter = postgres.ReceiptFilter

// Posting rule dan source type jurnal penerimaan.
const (
	RuleGoodsReceipt     = "A-01"
	RuleTollReceipt      = "A-03"
	SourceGoodsReceipt   = "goods_receipt"
	stockActionConsume   = "consume"
	stockActionCancel    = "cancel"
	stockActionUnconsume = "cancel_consume"
)

var receiptActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermReceiptWrite, domain.ActionPost: PermReceiptPost,
}

// ReceiptActions mengembalikan aksi GR yang boleh dilakukan actor. Batal draft cukup hak tulis; batal GR
// Diposting (membalik stok & jurnal) butuh hak batal.
func ReceiptActions(ctx context.Context, r domain.GoodsReceipt) []string {
	perms := map[domain.Action]string{domain.ActionCancel: PermReceiptWrite}
	if r.Status == domain.StatusPosted {
		perms[domain.ActionCancel] = PermReceiptCancel
	}
	for k, v := range receiptActionPerms {
		perms[k] = v
	}
	return filterActions(ctx, r.Actions(), perms)
}

// ReceiptView adalah GR beserta data tampilan.
type ReceiptView struct {
	domain.GoodsReceipt
	DivisionName  string
	CreatedByName string
	Products      map[uuid.UUID]ProductRef
	Locations     map[uuid.UUID]LocationRef
	// OrderLines memetakan baris PO (qty dipesan & sisa) untuk tampilan.
	OrderLines map[uuid.UUID]domain.OrderLine
}

func receiptSnapshot(r domain.GoodsReceipt) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"orderLineId": l.OrderLineID, "productId": l.ProductID, "goodQty": l.GoodQty.String(),
			"rejectQty": l.RejectQty.String(), "rejectDisposition": string(l.RejectDisposition), "batchNumber": l.BatchNumber,
			"unitCost": l.UnitCost.String(),
		}
	}
	materials := make([]map[string]any, len(r.Materials))
	for i, m := range r.Materials {
		materials[i] = map[string]any{
			"productId": m.ProductID, "usedGoodQty": m.UsedGoodQty.String(), "usedRejectQty": m.UsedRejectQty.String(),
			"costAmount": m.CostAmount.String(),
		}
	}
	return map[string]any{
		"purchaseOrderId": r.PurchaseOrderID, "businessDate": r.BusinessDate.String(), "locationId": r.LocationID,
		"deliveryNote": r.DeliveryNote, "note": r.Note, "status": string(r.Status), "inventoryAmount": r.InventoryAmount.String(),
		"grniAmount": r.GRNIAmount.String(), "cancelReason": r.CancelReason, "lines": lines, "materials": materials,
	}
}

// ListReceipts mengembalikan daftar Penerimaan Barang.
func (s *Service) ListReceipts(ctx context.Context, f ReceiptFilter, page paging.Page) (paging.Result[domain.GoodsReceipt], error) {
	if _, err := actor.Require(ctx, PermReceiptRead); err != nil {
		return paging.Result[domain.GoodsReceipt]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListReceipts(ctx, f, page)
}

// GetReceipt mengembalikan detail Penerimaan Barang.
func (s *Service) GetReceipt(ctx context.Context, id uuid.UUID) (ReceiptView, error) {
	if _, err := actor.Require(ctx, PermReceiptRead); err != nil {
		return ReceiptView{}, err
	}
	r, err := s.Store.GetReceipt(ctx, id, false)
	if err != nil {
		return ReceiptView{}, err
	}
	return s.receiptView(ctx, r)
}

func (s *Service) receiptView(ctx context.Context, r domain.GoodsReceipt) (ReceiptView, error) {
	v := ReceiptView{GoodsReceipt: r, OrderLines: map[uuid.UUID]domain.OrderLine{}}
	ids := []uuid.UUID{}
	for _, l := range r.Lines {
		ids = append(ids, l.ProductID)
	}
	for _, m := range r.Materials {
		ids = append(ids, m.ProductID)
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return ReceiptView{}, err
	}
	locIDs := []uuid.UUID{r.LocationID}
	if r.TollLocationID != nil {
		locIDs = append(locIDs, *r.TollLocationID)
	}
	if v.Locations, err = s.Locations.Locations(ctx, uniqueIDs(locIDs)); err != nil {
		return ReceiptView{}, err
	}
	divisions, err := s.Ledger.DivisionNames(ctx, []uuid.UUID{r.DivisionID})
	if err != nil {
		return ReceiptView{}, err
	}
	users, err := s.Users.UserNames(ctx, []uuid.UUID{r.CreatedBy})
	if err != nil {
		return ReceiptView{}, err
	}
	v.DivisionName, v.CreatedByName = divisions[r.DivisionID], users[r.CreatedBy]
	o, err := s.Store.GetOrder(ctx, r.PurchaseOrderID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	for _, l := range o.Lines {
		v.OrderLines[l.ID] = l
	}
	return v, nil
}

// receiptRefs membaca produk (jenis & pelacakan batch) dan lokasi tujuan untuk validasi GR.
func (s *Service) receiptRefs(ctx context.Context, o domain.PurchaseOrder, in domain.ReceiptInput) (domain.ReceiptRefs, map[uuid.UUID]ProductRef, error) {
	refs := domain.ReceiptRefs{Order: o, Products: map[uuid.UUID]domain.ReceiptProduct{}}
	ids := []uuid.UUID{}
	for _, l := range o.Lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	for _, m := range in.Materials {
		ids = append(ids, m.ProductID)
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return refs, nil, err
	}
	for id, p := range products {
		refs.Products[id] = domain.ReceiptProduct{Type: p.Type, HasBatch: p.HasBatch}
	}
	if in.LocationID != uuid.Nil {
		locs, err := s.Locations.Locations(ctx, []uuid.UUID{in.LocationID})
		if err != nil {
			return refs, nil, err
		}
		if l, ok := locs[in.LocationID]; ok {
			refs.Location = &domain.ReceiptLocation{Type: l.Type, IsActive: l.IsActive, Deleted: l.Deleted}
		}
	}
	return refs, products, nil
}

// lockReceivableOrder mengunci PO tujuan penerimaan.
func (s *Service) lockOrder(ctx context.Context, id uuid.UUID, field string) (domain.PurchaseOrder, error) {
	o, err := s.Store.GetOrder(ctx, id, true)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.PurchaseOrder{}, apperr.Validation(map[string]string{field: "Purchase Order tidak ditemukan"})
	}
	return o, err
}

// InboundLocations mengembalikan lokasi Inbound aktif untuk form Penerimaan Barang.
func (s *Service) InboundLocations(ctx context.Context) ([]LocationRef, error) {
	if _, err := actor.RequireAny(ctx, PermReceiptWrite, PermReceiptRead); err != nil {
		return nil, err
	}
	return s.Locations.InboundLocations(ctx)
}

// CreateReceipt menyimpan draft Penerimaan Barang untuk PO Disetujui/Diterima Sebagian.
func (s *Service) CreateReceipt(ctx context.Context, in domain.ReceiptInput) (ReceiptView, error) {
	a, err := actor.Require(ctx, PermReceiptWrite)
	if err != nil {
		return ReceiptView{}, err
	}
	var r domain.GoodsReceipt
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.lockOrder(ctx, in.PurchaseOrderID, "purchaseOrderId")
		if err != nil {
			return err
		}
		refs, _, err := s.receiptRefs(ctx, o, in)
		if err != nil {
			return err
		}
		if r, err = domain.NewGoodsReceipt(in, refs, a.UserID); err != nil {
			return err
		}
		if r.ID, err = s.Store.InsertReceipt(ctx, r, a.UserID); err != nil {
			return err
		}
		if r, err = s.Store.GetReceipt(ctx, r.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityGoodsReceipt, EntityID: r.ID, EntityLabel: r.Label(),
			After: receiptSnapshot(r),
		})
	})
	if err != nil {
		return ReceiptView{}, err
	}
	return s.receiptView(ctx, r)
}

// mutateReceipt mengunci PO lalu GR, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit trail.
func (s *Service) mutateReceipt(ctx context.Context, id uuid.UUID, version int, action audit.Action, replaceItems bool,
	fn func(ctx context.Context, r *domain.GoodsReceipt, o domain.PurchaseOrder) (string, error),
) (ReceiptView, error) {
	a, _ := actor.From(ctx)
	var r domain.GoodsReceipt
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetReceipt(ctx, id, false)
		if err != nil {
			return err
		}
		o, err := s.Store.GetOrder(ctx, cur.PurchaseOrderID, true)
		if err != nil {
			return err
		}
		if r, err = s.Store.GetReceipt(ctx, id, true); err != nil {
			return err
		}
		if r.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := receiptSnapshot(r)
		note, err := fn(ctx, &r, o)
		if err != nil {
			return err
		}
		if err := s.Store.SaveReceipt(ctx, r, version, a.UserID, replaceItems); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityGoodsReceipt, EntityID: r.ID, EntityLabel: r.Label(),
			Before: before, After: receiptSnapshot(r), Note: note,
		})
	})
	if err != nil {
		return ReceiptView{}, err
	}
	if r, err = s.Store.GetReceipt(ctx, id, false); err != nil {
		return ReceiptView{}, err
	}
	return s.receiptView(ctx, r)
}

// UpdateReceipt mengubah draft Penerimaan Barang.
func (s *Service) UpdateReceipt(ctx context.Context, id uuid.UUID, version int, in domain.ReceiptInput) (ReceiptView, error) {
	if _, err := actor.Require(ctx, PermReceiptWrite); err != nil {
		return ReceiptView{}, err
	}
	return s.mutateReceipt(ctx, id, version, audit.ActionUpdate, true, func(ctx context.Context, r *domain.GoodsReceipt, o domain.PurchaseOrder) (string, error) {
		if r.Status != domain.StatusDraft {
			return "", r.Update(in, domain.ReceiptRefs{})
		}
		refs, _, err := s.receiptRefs(ctx, o, in)
		if err != nil {
			return "", err
		}
		return "", r.Update(in, refs)
	})
}

// PostReceipt memposting Penerimaan Barang di satu transaksi (Desain Alur Lintas Modul 1):
//
//  1. Validasi ulang terhadap PO saat ini (sisa qty, toleransi 0%) dan nomor GR/TAHUN/BULAN/NOMOR.
//  2. PO maklon/reprocess: packaging/bahan terpakai (baik + reject) keluar dari lokasi maklon dengan HPP FIFO;
//     sisanya tetap di lokasi maklon.
//  3. Nilai batch & HPP per unit (domain.ApplyCosts), lalu stok baik masuk lokasi Inbound berstatus Karantina
//     dengan cost layer per batch.
//  4. Jurnal A-01 (Dr persediaan / Cr 2102) atau A-03 (Dr 1311 / Cr 1301|1302 bahan, Cr 2102 jasa maklon) dengan
//     dimensi divisi PO dan pihak pemasok.
//  5. Qty diterima PO (Diterima Sebagian/Diterima) dan dokumen Inbound/QC bila integrasi aktif.
func (s *Service) PostReceipt(ctx context.Context, id uuid.UUID, version int) (ReceiptView, error) {
	if _, err := actor.Require(ctx, PermReceiptPost); err != nil {
		return ReceiptView{}, err
	}
	return s.mutateReceipt(ctx, id, version, audit.ActionPost, false, func(ctx context.Context, r *domain.GoodsReceipt, o domain.PurchaseOrder) (string, error) {
		if r.Status != domain.StatusDraft {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Penerimaan Barang "+r.Label()+" berstatus "+r.Status.Label()+".")
		}
		in := r.Input()
		refs, products, err := s.receiptRefs(ctx, o, in)
		if err != nil {
			return "", err
		}
		if err := in.Validate(refs); err != nil {
			return "", err
		}
		number, err := s.Numberer.Next(ctx, DocGoodsReceipt, r.BusinessDate)
		if err != nil {
			return "", err
		}
		materialCosts, materialAccounts, err := s.consumeMaterials(ctx, r, number, products)
		if err != nil {
			return "", err
		}
		lineAccounts := make([]string, len(r.Lines))
		for i, l := range r.Lines {
			lineAccounts[i] = domain.InventoryAccount(products[l.ProductID].Type, domain.LocationInbound)
		}
		if err := r.ApplyCosts(materialCosts, materialAccounts, lineAccounts); err != nil {
			return "", err
		}
		if err := s.receiveStock(ctx, r, number); err != nil {
			return "", err
		}
		entry, err := s.Ledger.Post(ctx, receiptPosting(*r, o, number))
		if err != nil {
			return "", err
		}
		if _, err := s.RecordReceipt(ctx, o.ID, number, r.OrderDeltas(1)); err != nil {
			return "", err
		}
		if err := r.Post(number, entry.ID, s.Clock.Now()); err != nil {
			return "", err
		}
		if err := s.Store.SaveMaterialCosts(ctx, r.Materials); err != nil {
			return "", err
		}
		if s.Inbounds != nil {
			inboundID, err := s.Inbounds.CreateInbound(ctx, inboundRequest(*r))
			if err != nil {
				return "", err
			}
			r.InboundID = &inboundID
		}
		return "Jurnal " + entry.Number, nil
	})
}

func inboundRequest(r domain.GoodsReceipt) InboundRequest {
	req := InboundRequest{
		SourceType: SourceGoodsReceipt, SourceID: r.ID, SourceLabel: r.Number, LocationID: r.LocationID,
		BusinessDate: r.BusinessDate, SupplierID: r.SupplierID,
	}
	for _, l := range r.Lines {
		req.Lines = append(req.Lines, InboundLine{SourceLineID: l.ID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.GoodQty})
	}
	return req
}

// consumeMaterials mengeluarkan packaging/bahan terpakai dari lokasi maklon (FIFO) dan mengembalikan HPP serta akun
// persediaan per bahan.
func (s *Service) consumeMaterials(ctx context.Context, r *domain.GoodsReceipt, number string, products map[uuid.UUID]ProductRef) ([]money.Amount, []string, error) {
	costs := make([]money.Amount, len(r.Materials))
	accounts := make([]string, len(r.Materials))
	if len(r.Materials) == 0 {
		return costs, accounts, nil
	}
	if r.TollLocationID == nil {
		return nil, nil, apperr.Validation(map[string]string{"materials": "PO maklon belum punya lokasi maklon"})
	}
	lines := make([]inventory.Line, len(r.Materials))
	for i, m := range r.Materials {
		lineID := m.ID
		lines[i] = inventory.Line{
			Kind: inventory.KindIssue, SourceLineID: &lineID, ProductID: m.ProductID, Qty: m.TotalQty(),
			From: &inventory.Place{LocationID: *r.TollLocationID, Status: inventory.StatusAvailable},
			Note: fmt.Sprintf("Pemakaian maklon baik %s + reject %s", m.UsedGoodQty.String(), m.UsedRejectQty.String()),
		}
	}
	res, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourceGoodsReceipt, ID: r.ID, Action: stockActionConsume, Label: number, BusinessDate: r.BusinessDate},
		Lines:  lines,
	})
	if err != nil {
		return nil, nil, err
	}
	for i := range r.Materials {
		costs[i] = money.FromDecimal(res.Lines[i].CostAmount)
		accounts[i] = domain.InventoryAccount(products[r.Materials[i].ProductID].Type, domain.LocationTollManufacturer)
		r.Materials[i].Consumptions = nil
		for _, mv := range res.Lines[i].Movements {
			r.Materials[i].Consumptions = append(r.Materials[i].Consumptions, domain.MaterialConsumption{
				BatchID: mv.BatchID, Qty: mv.Qty.Abs(), UnitCost: mv.UnitCost,
			})
		}
	}
	return costs, accounts, nil
}

// receiveStock memasukkan qty baik ke lokasi Inbound berstatus Karantina dengan HPP per unit batch.
func (s *Service) receiveStock(ctx context.Context, r *domain.GoodsReceipt, number string) error {
	lines := make([]inventory.Line, len(r.Lines))
	for i, l := range r.Lines {
		lineID, cost := l.ID, l.UnitCost
		line := inventory.Line{
			Kind: inventory.KindReceive, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.GoodQty, UnitCost: &cost,
			To: &inventory.Place{LocationID: r.LocationID, Status: inventory.StatusQuarantine},
		}
		if l.BatchNumber != "" {
			line.Batch = &inventory.NewBatch{Number: l.BatchNumber, ProductionDate: l.ProductionDate, ExpiryDate: l.ExpiryDate}
		}
		lines[i] = line
	}
	res, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourceGoodsReceipt, ID: r.ID, Label: number, BusinessDate: r.BusinessDate},
		Lines:  lines,
	})
	if err != nil {
		return err
	}
	for i := range r.Lines {
		for _, mv := range res.Lines[i].Movements {
			r.Lines[i].BatchID = mv.BatchID
		}
	}
	return nil
}

// receiptPosting menyusun PostingRequest A-01/A-03. Akun persediaan per baris dikirim lewat Splits.
func receiptPosting(r domain.GoodsReceipt, o domain.PurchaseOrder, number string) accounting.PostingRequest {
	division := r.DivisionID
	req := accounting.PostingRequest{
		RuleCode: RuleGoodsReceipt, SourceType: SourceGoodsReceipt, SourceID: r.ID, SourceLabel: number,
		BusinessDate: r.BusinessDate, Dimensions: accounting.Dimensions{DivisionID: &division},
		Party:       &accounting.Party{Type: PartySupplier, ID: r.SupplierID},
		Description: "Penerimaan barang " + o.Number + " · " + o.SupplierName,
	}
	if r.IsToll() {
		req.RuleCode = RuleTollReceipt
		req.Description = "Penerimaan FG maklon " + o.Number + " · " + o.SupplierName
		req.Amounts = map[string]money.Amount{"fg_value": r.InventoryAmount, "service_value": r.GRNIAmount}
		byAccount := map[string]money.Amount{}
		order := []string{}
		for _, m := range r.Materials {
			if _, ok := byAccount[m.AccountCode]; !ok {
				order = append(order, m.AccountCode)
			}
			byAccount[m.AccountCode] = byAccount[m.AccountCode].Add(m.CostAmount)
		}
		for _, code := range order {
			req.Splits = append(req.Splits, accounting.PostingSplit{AmountKey: "material_value", AccountCode: code, Amount: byAccount[code]})
		}
		return req
	}
	byAccount := map[string]money.Amount{}
	order := []string{}
	for _, l := range r.Lines {
		if _, ok := byAccount[l.InventoryAccount]; !ok {
			order = append(order, l.InventoryAccount)
		}
		byAccount[l.InventoryAccount] = byAccount[l.InventoryAccount].Add(l.InventoryAmount)
	}
	for _, code := range order {
		req.Splits = append(req.Splits, accounting.PostingSplit{AmountKey: "inventory_value", AccountCode: code, Amount: byAccount[code]})
	}
	return req
}

// CancelReceipt membatalkan Penerimaan Barang. Draft cukup dibatalkan; GR Diposting yang belum ditagih dan belum
// diretur dibalik di transaksi yang sama: Inbound dibatalkan, stok Karantina keluar lagi, packaging maklon kembali
// ke lokasi maklon dengan HPP semula, jurnal A-01/A-03 dibalik, dan qty diterima PO dikurangi (TG 5.8.3).
func (s *Service) CancelReceipt(ctx context.Context, id uuid.UUID, version int, reason string) (ReceiptView, error) {
	if _, err := actor.RequireAny(ctx, PermReceiptWrite, PermReceiptCancel); err != nil {
		return ReceiptView{}, err
	}
	return s.mutateReceipt(ctx, id, version, audit.ActionCancel, false, func(ctx context.Context, r *domain.GoodsReceipt, o domain.PurchaseOrder) (string, error) {
		posted := r.Status == domain.StatusPosted
		need := PermReceiptWrite
		if posted {
			need = PermReceiptCancel
		}
		if _, err := actor.Require(ctx, need); err != nil {
			return "", err
		}
		if err := r.Cancel(reason, s.Clock.Now()); err != nil {
			return "", err
		}
		if !posted {
			return r.CancelReason, nil
		}
		if s.Inbounds != nil && r.InboundID != nil {
			if err := s.Inbounds.CancelInbound(ctx, *r.InboundID, r.CancelReason); err != nil {
				return "", err
			}
		}
		if err := s.reverseReceiptStock(ctx, *r); err != nil {
			return "", err
		}
		rule := RuleGoodsReceipt
		if r.IsToll() {
			rule = RuleTollReceipt
		}
		entry, err := s.Ledger.Reverse(ctx, accounting.ReversalRequest{
			SourceType: SourceGoodsReceipt, SourceID: r.ID, RuleCode: rule, BusinessDate: s.today(),
			Reason: "Pembatalan penerimaan " + r.Number + ": " + r.CancelReason,
		})
		if err != nil {
			return "", err
		}
		r.ReversalEntryID = &entry.ID
		if _, err := s.RecordReceipt(ctx, o.ID, r.Number+" (batal)", r.OrderDeltas(-1)); err != nil {
			return "", err
		}
		return r.CancelReason + " (jurnal balik " + entry.Number + ")", nil
	})
}

func (s *Service) reverseReceiptStock(ctx context.Context, r domain.GoodsReceipt) error {
	date := s.today()
	lines := make([]inventory.Line, len(r.Lines))
	for i, l := range r.Lines {
		lineID := l.ID
		lines[i] = inventory.Line{
			Kind: inventory.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.GoodQty, BatchID: l.BatchID,
			From: &inventory.Place{LocationID: r.LocationID, Status: inventory.StatusQuarantine},
			Note: "Pembatalan " + r.Number,
		}
	}
	if _, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourceGoodsReceipt, ID: r.ID, Action: stockActionCancel, Label: r.Number, BusinessDate: date},
		Lines:  lines,
	}); err != nil {
		return err
	}
	if r.TollLocationID == nil || len(r.Materials) == 0 {
		return nil
	}
	back := []inventory.Line{}
	for _, m := range r.Materials {
		lineID := m.ID
		for _, c := range m.Consumptions {
			cost := c.UnitCost
			back = append(back, inventory.Line{
				Kind: inventory.KindReceive, SourceLineID: &lineID, ProductID: m.ProductID, Qty: c.Qty, BatchID: c.BatchID,
				UnitCost: &cost, To: &inventory.Place{LocationID: *r.TollLocationID, Status: inventory.StatusAvailable},
				Note: "Pembatalan pemakaian maklon " + r.Number,
			})
		}
	}
	if len(back) == 0 {
		return nil
	}
	_, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourceGoodsReceipt, ID: r.ID, Action: stockActionUnconsume, Label: r.Number, BusinessDate: date},
		Lines:  back,
	})
	return err
}

// receiptLinesOf membaca baris GR (dikunci lewat GR induknya) untuk tagihan dan retur.
func (s *Service) lockReceiptLines(ctx context.Context, lineIDs []uuid.UUID) (map[uuid.UUID]postgres.ReceiptLineRef, error) {
	receiptIDs, err := s.Store.ReceiptIDsByLines(ctx, uniqueIDs(lineIDs))
	if err != nil {
		return nil, err
	}
	for _, id := range receiptIDs {
		if _, err := s.Store.GetReceipt(ctx, id, true); err != nil {
			return nil, err
		}
	}
	return s.Store.ReceiptLinesByIDs(ctx, uniqueIDs(lineIDs))
}

// saveReceiptLineChanges menyimpan qty ditagih/diretur baris GR.
func (s *Service) saveReceiptLineChanges(ctx context.Context, refs map[uuid.UUID]postgres.ReceiptLineRef, ids []uuid.UUID) error {
	lines := make([]domain.ReceiptLine, 0, len(ids))
	for _, id := range uniqueIDs(ids) {
		lines = append(lines, refs[id].Line)
	}
	return s.Store.SaveReceiptLines(ctx, lines)
}
