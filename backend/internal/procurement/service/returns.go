package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// ReturnFilter adalah kriteria daftar retur pembelian.
type ReturnFilter = postgres.ReturnFilter

// Posting rule dan source type jurnal retur pembelian.
const (
	RulePurchaseReturn   = "A-02"
	SourcePurchaseReturn = "purchase_return"
)

var returnActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermReturnWrite, domain.ActionSubmit: PermReturnSubmit, domain.ActionPost: PermReturnPost,
	domain.ActionCancel: PermReturnWrite, domain.ActionRevise: PermReturnWrite,
}

// ReturnActions mengembalikan aksi retur yang boleh dilakukan actor.
func ReturnActions(ctx context.Context, r domain.PurchaseReturn) []string {
	return filterActions(ctx, r.Actions(), returnActionPerms)
}

// ReturnView adalah retur beserta data tampilan.
type ReturnView struct {
	domain.PurchaseReturn
	DivisionName  string
	CreatedByName string
	Products      map[uuid.UUID]ProductRef
	Locations     map[uuid.UUID]LocationRef
	Receipts      map[uuid.UUID]postgres.ReceiptLineRef
}

func returnSnapshot(r domain.PurchaseReturn) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"receiptLineId": l.ReceiptLineID, "locationId": l.LocationID, "stockStatus": l.StockStatus, "qty": l.Qty.String(),
			"amount": l.Amount.String(),
		}
	}
	return map[string]any{
		"purchaseOrderId": r.PurchaseOrderID, "origin": string(r.Origin), "businessDate": r.BusinessDate.String(),
		"reason": r.Reason, "note": r.Note, "status": string(r.Status), "totalAmount": r.Total.String(),
		"debitNoteNumber": r.DebitNoteNumber, "lines": lines,
	}
}

// ListReturns mengembalikan daftar retur pembelian.
func (s *Service) ListReturns(ctx context.Context, f ReturnFilter, page paging.Page) (paging.Result[domain.PurchaseReturn], error) {
	if _, err := actor.Require(ctx, PermReturnRead); err != nil {
		return paging.Result[domain.PurchaseReturn]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListReturns(ctx, f, page)
}

// GetReturn mengembalikan detail retur pembelian.
func (s *Service) GetReturn(ctx context.Context, id uuid.UUID) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnRead); err != nil {
		return ReturnView{}, err
	}
	r, err := s.Store.GetReturn(ctx, id, false)
	if err != nil {
		return ReturnView{}, err
	}
	return s.returnView(ctx, r)
}

func (s *Service) returnView(ctx context.Context, r domain.PurchaseReturn) (ReturnView, error) {
	v := ReturnView{PurchaseReturn: r}
	ids, locIDs, lineIDs := []uuid.UUID{}, []uuid.UUID{}, []uuid.UUID{}
	for _, l := range r.Lines {
		ids, locIDs, lineIDs = append(ids, l.ProductID), append(locIDs, l.LocationID), append(lineIDs, l.ReceiptLineID)
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return ReturnView{}, err
	}
	if v.Locations, err = s.Locations.Locations(ctx, uniqueIDs(locIDs)); err != nil {
		return ReturnView{}, err
	}
	if v.Receipts, err = s.Store.ReceiptLinesByIDs(ctx, uniqueIDs(lineIDs)); err != nil {
		return ReturnView{}, err
	}
	divisions, err := s.Ledger.DivisionNames(ctx, []uuid.UUID{r.DivisionID})
	if err != nil {
		return ReturnView{}, err
	}
	users, err := s.Users.UserNames(ctx, []uuid.UUID{r.CreatedBy})
	if err != nil {
		return ReturnView{}, err
	}
	v.DivisionName, v.CreatedByName = divisions[r.DivisionID], users[r.CreatedBy]
	return v, nil
}

// ReturnableReceipts adalah baris GR PO yang masih bisa diretur beserta qty di retur lain yang belum diposting.
type ReturnableReceipts struct {
	Order      domain.PurchaseOrder
	Lines      []postgres.ReceiptLineRef
	PendingQty map[uuid.UUID]decimal.Decimal
	Products   map[uuid.UUID]ProductRef
	Locations  map[uuid.UUID]LocationRef
}

// ReturnableReceipts mengembalikan baris Penerimaan Barang PO yang masih bisa diretur (form retur manual).
func (s *Service) ReturnableReceipts(ctx context.Context, orderID uuid.UUID) (ReturnableReceipts, error) {
	if _, err := actor.RequireAny(ctx, PermReturnWrite, PermReturnRead); err != nil {
		return ReturnableReceipts{}, err
	}
	o, err := s.Store.GetOrder(ctx, orderID, false)
	if err != nil {
		return ReturnableReceipts{}, err
	}
	out := ReturnableReceipts{Order: o}
	all, err := s.Store.ReceiptLinesByOrder(ctx, orderID)
	if err != nil {
		return ReturnableReceipts{}, err
	}
	ids, productIDs, locIDs := []uuid.UUID{}, []uuid.UUID{}, []uuid.UUID{}
	for _, l := range all {
		if l.Line.ReturnableQty().IsPositive() {
			out.Lines = append(out.Lines, l)
			ids, productIDs, locIDs = append(ids, l.Line.ID), append(productIDs, l.Line.ProductID), append(locIDs, l.LocationID)
		}
	}
	if out.PendingQty, err = s.Store.PendingReturnQty(ctx, ids, uuid.Nil); err != nil {
		return ReturnableReceipts{}, err
	}
	if out.Products, err = s.Catalog.Products(ctx, uniqueIDs(productIDs)); err != nil {
		return ReturnableReceipts{}, err
	}
	if out.Locations, err = s.Locations.Locations(ctx, uniqueIDs(locIDs)); err != nil {
		return ReturnableReceipts{}, err
	}
	return out, nil
}

func returnLineIDs(in domain.ReturnInput) []uuid.UUID {
	ids := make([]uuid.UUID, len(in.Lines))
	for i, l := range in.Lines {
		ids[i] = l.ReceiptLineID
	}
	return ids
}

// returnRefs membaca baris GR (terkunci lewat GR), qty di retur lain yang belum diposting, dan lokasi asal.
func (s *Service) returnRefs(ctx context.Context, o domain.PurchaseOrder, in domain.ReturnInput, returnID uuid.UUID) (domain.ReturnRefs, map[uuid.UUID]postgres.ReceiptLineRef, map[uuid.UUID]LocationRef, error) {
	refs := domain.ReturnRefs{Order: o, ReceiptLines: map[uuid.UUID]domain.ReturnReceiptLine{}, Locations: map[uuid.UUID]bool{}}
	lineIDs := returnLineIDs(in)
	lines, err := s.lockReceiptLines(ctx, lineIDs)
	if err != nil {
		return refs, nil, nil, err
	}
	pending, err := s.Store.PendingReturnQty(ctx, uniqueIDs(lineIDs), returnID)
	if err != nil {
		return refs, nil, nil, err
	}
	for id, l := range lines {
		refs.ReceiptLines[id] = domain.ReturnReceiptLine{
			Line: l.Line, ReceiptNumber: l.ReceiptNumber, ReceiptStatus: l.ReceiptStatus,
			PurchaseOrderID: l.PurchaseOrderID, PendingQty: pending[id],
		}
	}
	locIDs := make([]uuid.UUID, 0, len(in.Lines))
	for _, l := range in.Lines {
		locIDs = append(locIDs, l.LocationID)
	}
	locs, err := s.Locations.Locations(ctx, uniqueIDs(locIDs))
	if err != nil {
		return refs, nil, nil, err
	}
	for id, l := range locs {
		refs.Locations[id] = l.IsActive && !l.Deleted
	}
	return refs, lines, locs, nil
}

func (s *Service) insertReturn(ctx context.Context, in domain.ReturnInput) (domain.PurchaseReturn, error) {
	a, _ := actor.From(ctx)
	o, err := s.lockOrder(ctx, in.PurchaseOrderID, "purchaseOrderId")
	if err != nil {
		return domain.PurchaseReturn{}, err
	}
	refs, _, _, err := s.returnRefs(ctx, o, in, uuid.Nil)
	if err != nil {
		return domain.PurchaseReturn{}, err
	}
	r, err := domain.NewPurchaseReturn(in, refs, a.UserID)
	if err != nil {
		return domain.PurchaseReturn{}, err
	}
	if r.ID, err = s.Store.InsertReturn(ctx, r, a.UserID); err != nil {
		return domain.PurchaseReturn{}, err
	}
	if r, err = s.Store.GetReturn(ctx, r.ID, false); err != nil {
		return domain.PurchaseReturn{}, err
	}
	return r, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: EntityPurchaseReturn, EntityID: r.ID, EntityLabel: r.Label(),
		After: returnSnapshot(r),
	})
}

// CreateReturn menyimpan draft retur manual (butuh persetujuan Head Logistik sebelum diposting).
func (s *Service) CreateReturn(ctx context.Context, in domain.ReturnInput) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	in.Origin, in.SourceType, in.SourceID, in.SourceLabel = domain.ReturnManual, "", nil, ""
	var r domain.PurchaseReturn
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		r, err = s.insertReturn(ctx, in)
		return err
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.returnView(ctx, r)
}

// mutateReturn mengunci PO lalu retur, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit.
func (s *Service) mutateReturn(ctx context.Context, id uuid.UUID, version *int, action audit.Action,
	fn func(ctx context.Context, r *domain.PurchaseReturn, o domain.PurchaseOrder) (string, error),
) (domain.PurchaseReturn, error) {
	a, _ := actor.From(ctx)
	var r domain.PurchaseReturn
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetReturn(ctx, id, false)
		if err != nil {
			return err
		}
		o, err := s.Store.GetOrder(ctx, cur.PurchaseOrderID, true)
		if err != nil {
			return err
		}
		if r, err = s.Store.GetReturn(ctx, id, true); err != nil {
			return err
		}
		if version != nil && r.Version != *version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		v := r.Version
		before := returnSnapshot(r)
		note, err := fn(ctx, &r, o)
		if err != nil {
			return err
		}
		if err := s.Store.SaveReturn(ctx, r, v, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPurchaseReturn, EntityID: r.ID, EntityLabel: r.Label(),
			Before: before, After: returnSnapshot(r), Note: note,
		})
	})
	return r, err
}

func (s *Service) returnResult(ctx context.Context, id uuid.UUID, err error) (ReturnView, error) {
	if err != nil {
		return ReturnView{}, err
	}
	r, err := s.Store.GetReturn(ctx, id, false)
	if err != nil {
		return ReturnView{}, err
	}
	return s.returnView(ctx, r)
}

// UpdateReturn mengubah draft retur manual.
func (s *Service) UpdateReturn(ctx context.Context, id uuid.UUID, version int, in domain.ReturnInput) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	_, err := s.mutateReturn(ctx, id, &version, audit.ActionUpdate, func(ctx context.Context, r *domain.PurchaseReturn, o domain.PurchaseOrder) (string, error) {
		if r.Status != domain.StatusDraft {
			return "", r.Update(in, domain.ReturnRefs{})
		}
		refs, _, _, err := s.returnRefs(ctx, o, in, r.ID)
		if err != nil {
			return "", err
		}
		return "", r.Update(in, refs)
	})
	return s.returnResult(ctx, id, err)
}

// SubmitReturn mengajukan retur manual: nomor RB dan approval `purchase_return` (Head Logistik) di transaksi yang
// sama.
func (s *Service) SubmitReturn(ctx context.Context, id uuid.UUID, version int) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnSubmit); err != nil {
		return ReturnView{}, err
	}
	_, err := s.mutateReturn(ctx, id, &version, audit.ActionSubmit, func(ctx context.Context, r *domain.PurchaseReturn, o domain.PurchaseOrder) (string, error) {
		in := r.Input()
		refs, _, _, err := s.returnRefs(ctx, o, in, r.ID)
		if err != nil {
			return "", err
		}
		if err := in.Validate(refs); err != nil {
			return "", err
		}
		number := ""
		if r.Number == "" {
			if number, err = s.Numberer.Next(ctx, DocPurchaseReturn, r.BusinessDate); err != nil {
				return "", err
			}
		}
		if err := r.Submit(number, s.Clock.Now()); err != nil {
			return "", err
		}
		value := money.Zero
		for _, l := range r.Lines {
			value = value.Add(LineValue(refs.ReceiptLines[l.ReceiptLineID].Line, l.Qty))
		}
		division := r.DivisionID
		_, err = s.Approvals.Request(ctx, approval.Subject{
			Type: approval.SubjectPurchaseReturn, ID: r.ID, Label: r.Number, Link: ReturnPath(r.ID),
			Summary: r.SupplierName + " · " + r.OrderNumber + " · " + r.Reason, Amount: value, Division: &division,
		})
		return "", err
	})
	return s.returnResult(ctx, id, err)
}

// LineValue memperkirakan nilai retur dari HPP per unit baris GR (nilai pasti dihitung FIFO saat posting).
func LineValue(l domain.ReceiptLine, qty decimal.Decimal) money.Amount {
	return money.FromDecimal(l.UnitCost.Mul(qty)).Round()
}

// ReviseReturn mengembalikan retur yang ditolak ke Draft.
func (s *Service) ReviseReturn(ctx context.Context, id uuid.UUID, version int) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	_, err := s.mutateReturn(ctx, id, &version, audit.ActionStatusChange, func(_ context.Context, r *domain.PurchaseReturn, _ domain.PurchaseOrder) (string, error) {
		return "Direvisi", r.Revise()
	})
	return s.returnResult(ctx, id, err)
}

// CancelReturn membatalkan retur Draft atau Disetujui yang belum diposting.
func (s *Service) CancelReturn(ctx context.Context, id uuid.UUID, version int, reason string) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	_, err := s.mutateReturn(ctx, id, &version, audit.ActionCancel, func(_ context.Context, r *domain.PurchaseReturn, _ domain.PurchaseOrder) (string, error) {
		return strings.TrimSpace(reason), r.Cancel(reason)
	})
	return s.returnResult(ctx, id, err)
}

// PostReturn memposting retur yang disetujui (manual) atau draft asal QC.
func (s *Service) PostReturn(ctx context.Context, id uuid.UUID, version int) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnPost); err != nil {
		return ReturnView{}, err
	}
	_, err := s.mutateReturn(ctx, id, &version, audit.ActionPost, s.postReturn)
	return s.returnResult(ctx, id, err)
}

// postReturn: barang keluar dari lokasi/status asal (HPP FIFO), nomor RB & nota debit DN, nilai dibagi ke 2102
// (bagian GR yang belum ditagih — qty diterima PO ikut dikurangi agar pemasok bisa mengganti) dan 2101 (sudah
// ditagih — nota debit menjadi utang negatif), jurnal A-02, semuanya di transaksi yang sama.
func (s *Service) postReturn(ctx context.Context, r *domain.PurchaseReturn, o domain.PurchaseOrder) (string, error) {
	if err := r.CanPost(); err != nil {
		return "", err
	}
	in := r.Input()
	refs, lines, locs, err := s.returnRefs(ctx, o, in, r.ID)
	if err != nil {
		return "", err
	}
	if err := in.Validate(refs); err != nil {
		return "", err
	}
	productIDs := make([]uuid.UUID, len(r.Lines))
	for i, l := range r.Lines {
		productIDs[i] = l.ProductID
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(productIDs))
	if err != nil {
		return "", err
	}
	number := r.Number
	if number == "" {
		if number, err = s.Numberer.Next(ctx, DocPurchaseReturn, r.BusinessDate); err != nil {
			return "", err
		}
	}
	debitNote, err := s.Numberer.Next(ctx, DocDebitNote, r.BusinessDate)
	if err != nil {
		return "", err
	}
	stockLines := make([]inventory.Line, len(r.Lines))
	for i, l := range r.Lines {
		lineID := l.ID
		line := inventory.Line{
			Kind: inventory.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID,
			From: &inventory.Place{LocationID: l.LocationID, Status: inventory.StockStatus(l.StockStatus)},
			Note: "Retur ke " + r.SupplierName,
		}
		if l.StockStatus == domain.StockAvailable && l.BatchID != nil {
			line.BatchReason = "Retur pembelian batch penerimaan " + refs.ReceiptLines[l.ReceiptLineID].ReceiptNumber
		}
		stockLines[i] = line
	}
	res, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourcePurchaseReturn, ID: r.ID, Label: number, BusinessDate: r.BusinessDate},
		Lines:  stockLines,
	})
	if err != nil {
		return "", err
	}
	changed := []uuid.UUID{}
	unbilledPerOrderLine := map[uuid.UUID]decimal.Decimal{}
	for i, l := range r.Lines {
		ref := lines[l.ReceiptLineID]
		account := domain.InventoryAccount(products[l.ProductID].Type, locs[l.LocationID].Type)
		r.ApplyCost(i, money.FromDecimal(res.Lines[i].CostAmount), ref.Line.OpenQty(), account)
		applied := r.Lines[i]
		ref.Line.ReturnedQty = ref.Line.ReturnedQty.Add(applied.Qty)
		ref.Line.ReturnedUnbilledQty = ref.Line.ReturnedUnbilledQty.Add(applied.UnbilledQty)
		ref.Line.ReturnedGRNIAmount = ref.Line.ReturnedGRNIAmount.Add(applied.UnbilledAmount)
		lines[l.ReceiptLineID] = ref
		changed = append(changed, l.ReceiptLineID)
		if applied.UnbilledQty.IsPositive() {
			unbilledPerOrderLine[ref.Line.OrderLineID] = unbilledPerOrderLine[ref.Line.OrderLineID].Add(applied.UnbilledQty)
		}
	}
	if err := s.saveReceiptLineChanges(ctx, lines, changed); err != nil {
		return "", err
	}
	entry, err := s.Ledger.Post(ctx, returnPosting(*r, number))
	if err != nil {
		return "", err
	}
	if err := s.reduceReceivedQty(ctx, o, number, unbilledPerOrderLine); err != nil {
		return "", err
	}
	if r.BilledAmount.IsPositive() {
		if err := s.Store.InsertPayable(ctx, domain.Payable{
			DocumentType: domain.PayableDebitNote, DocumentID: r.ID, DocumentNumber: debitNote, SupplierID: r.SupplierID,
			PurchaseOrderID: r.PurchaseOrderID, DivisionID: r.DivisionID, BusinessDate: r.BusinessDate, DueDate: r.BusinessDate,
			Amount: r.BilledAmount.Neg(), OpenAmount: r.BilledAmount.Neg(),
		}, actor.UserID(ctx)); err != nil {
			return "", err
		}
	}
	if err := r.Post(number, debitNote, entry.ID, s.Clock.Now()); err != nil {
		return "", err
	}
	return "Nota debit " + debitNote + ", jurnal " + entry.Number, nil
}

// reduceReceivedQty mengurangi qty diterima PO sebesar bagian retur yang belum ditagih (pemasok mengganti barang),
// selama PO masih bisa menerima/terbuka.
func (s *Service) reduceReceivedQty(ctx context.Context, o domain.PurchaseOrder, label string, perLine map[uuid.UUID]decimal.Decimal) error {
	if len(perLine) == 0 {
		return nil
	}
	switch o.Status {
	case domain.StatusApproved, domain.StatusPartiallyReceived, domain.StatusReceived:
	default:
		return nil
	}
	deltas := []domain.QtyDelta{}
	for _, l := range o.Lines {
		if q, ok := perLine[l.ID]; ok {
			deltas = append(deltas, domain.QtyDelta{LineID: l.ID, Qty: q.Neg()})
		}
	}
	_, err := s.RecordReceipt(ctx, o.ID, label+" (retur)", deltas)
	return err
}

// returnPosting menyusun A-02: return_value (Dr 2101) untuk bagian yang sudah ditagih dan unbilled_return_value
// (Dr 2102) untuk yang belum, keduanya Cr akun persediaan per baris (Splits).
func returnPosting(r domain.PurchaseReturn, number string) accounting.PostingRequest {
	division := r.DivisionID
	req := accounting.PostingRequest{
		RuleCode: RulePurchaseReturn, SourceType: SourcePurchaseReturn, SourceID: r.ID, SourceLabel: number,
		BusinessDate: r.BusinessDate, Dimensions: accounting.Dimensions{DivisionID: &division},
		Party:       &accounting.Party{Type: PartySupplier, ID: r.SupplierID},
		Description: "Retur pembelian " + number + " · " + r.OrderNumber + " · " + r.SupplierName,
	}
	type key struct{ amountKey, account string }
	sums := map[key]money.Amount{}
	order := []key{}
	add := func(k key, v money.Amount) {
		if v.IsZero() {
			return
		}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
		}
		sums[k] = sums[k].Add(v)
	}
	for _, l := range r.Lines {
		add(key{"unbilled_return_value", l.InventoryAccount}, l.UnbilledAmount)
		add(key{"return_value", l.InventoryAccount}, l.BilledAmount())
	}
	for _, k := range order {
		req.Splits = append(req.Splits, accounting.PostingSplit{AmountKey: k.amountKey, AccountCode: k.account, Amount: sums[k]})
	}
	return req
}

// OnReturnApproved menandai retur Disetujui; Purchasing/Gudang lalu memposting.
func (s *Service) OnReturnApproved(ctx context.Context, d approval.Decided) error {
	_, err := s.mutateReturn(ctx, d.Subject.ID, nil, audit.ActionStatusChange, func(_ context.Context, r *domain.PurchaseReturn, _ domain.PurchaseOrder) (string, error) {
		return "Disetujui", r.Approve(s.Clock.Now())
	})
	return err
}

// OnReturnRejected menandai retur Ditolak.
func (s *Service) OnReturnRejected(ctx context.Context, d approval.Decided) error {
	_, err := s.mutateReturn(ctx, d.Subject.ID, nil, audit.ActionStatusChange, func(_ context.Context, r *domain.PurchaseReturn, _ domain.PurchaseOrder) (string, error) {
		return "Ditolak", r.Reject()
	})
	return err
}

// QCReturnLine adalah satu barang reject QC yang dikembalikan ke pemasok/maklon.
type QCReturnLine struct {
	// ReceiptLineID baris Penerimaan Barang asal; kosong = dicari dari batch (GR diposting terakhir untuk batch itu).
	ReceiptLineID *uuid.UUID
	ProductID     uuid.UUID
	BatchID       *uuid.UUID
	LocationID    uuid.UUID
	// StockStatus asal: quarantine (QC Inbound) atau available (QC Outbound).
	StockStatus string
	Qty         decimal.Decimal
	Note        string
}

// QCReturnInput adalah permintaan retur dari hasil QC (D-24).
type QCReturnInput struct {
	Origin       domain.ReturnOrigin
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	BusinessDate clock.BusinessDate
	Reason       string
	Lines        []QCReturnLine
}

// CreateReturnFromQC membuat dan langsung memposting retur pembelian dari barang yang gagal QC Inbound/Outbound,
// tanpa approval (D-24, Desain Status Dokumen 2.5), di transaksi pemanggil. Satu dokumen QC hanya membuat satu
// retur aktif (uq_purchase_returns_source). Tidak memeriksa permission; pemanggil memeriksa aksi QC user-nya.
func (s *Service) CreateReturnFromQC(ctx context.Context, q QCReturnInput) (ReturnView, error) {
	if !q.Origin.IsQC() {
		return ReturnView{}, apperr.Validation(map[string]string{"origin": "Asal retur QC harus qc_inbound atau qc_outbound"})
	}
	var id uuid.UUID
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		in := domain.ReturnInput{
			Origin: q.Origin, SourceType: q.SourceType, SourceID: &q.SourceID, SourceLabel: q.SourceLabel,
			BusinessDate: q.BusinessDate, Reason: q.Reason,
		}
		for i, l := range q.Lines {
			lineID, orderID, err := s.resolveQCLine(ctx, l)
			if err != nil {
				return apperr.Validation(map[string]string{"lines." + itoa(i) + ".receiptLineId": err.Error()})
			}
			if in.PurchaseOrderID == uuid.Nil {
				in.PurchaseOrderID = orderID
			} else if in.PurchaseOrderID != orderID {
				return apperr.Validation(map[string]string{"lines." + itoa(i) + ".receiptLineId": "Satu retur hanya untuk satu Purchase Order"})
			}
			in.Lines = append(in.Lines, domain.ReturnLineInput{
				ReceiptLineID: lineID, LocationID: l.LocationID, StockStatus: l.StockStatus, Qty: l.Qty, Note: l.Note,
			})
		}
		if in.PurchaseOrderID == uuid.Nil {
			return apperr.Validation(map[string]string{"lines": "Minimal satu baris"})
		}
		r, err := s.insertReturn(ctx, in)
		if err != nil {
			return err
		}
		id = r.ID
		_, err = s.mutateReturn(ctx, r.ID, nil, audit.ActionPost, s.postReturn)
		return err
	})
	if err != nil {
		return ReturnView{}, err
	}
	r, err := s.Store.GetReturn(ctx, id, false)
	if err != nil {
		return ReturnView{}, err
	}
	return s.returnView(ctx, r)
}

type qcLineError string

func (e qcLineError) Error() string { return string(e) }

func (s *Service) resolveQCLine(ctx context.Context, l QCReturnLine) (uuid.UUID, uuid.UUID, error) {
	if l.ReceiptLineID != nil {
		refs, err := s.Store.ReceiptLinesByIDs(ctx, []uuid.UUID{*l.ReceiptLineID})
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		ref, ok := refs[*l.ReceiptLineID]
		if !ok {
			return uuid.Nil, uuid.Nil, qcLineError("Baris Penerimaan Barang tidak ditemukan")
		}
		return ref.Line.ID, ref.PurchaseOrderID, nil
	}
	if l.BatchID == nil {
		return uuid.Nil, uuid.Nil, qcLineError("Isi baris Penerimaan Barang atau batch asal")
	}
	lineID, orderID, ok, err := s.Store.ReceiptLineByBatch(ctx, l.ProductID, *l.BatchID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if !ok {
		return uuid.Nil, uuid.Nil, qcLineError("Batch tidak berasal dari Penerimaan Barang pemasok")
	}
	return lineID, orderID, nil
}
