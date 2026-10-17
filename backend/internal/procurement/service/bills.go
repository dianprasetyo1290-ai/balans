package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// BillFilter adalah kriteria daftar tagihan pemasok.
type BillFilter = postgres.BillFilter

// Posting rule dan source type jurnal tagihan.
const (
	RuleSupplierBill   = "A-18"
	SourceSupplierBill = "supplier_bill"
)

var billActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermBillWrite, domain.ActionPost: PermBillPost, domain.ActionRevise: PermBillWrite,
	domain.ActionCancel: PermBillCancel,
}

// BillActions mengembalikan aksi tagihan yang boleh dilakukan actor.
func BillActions(ctx context.Context, b domain.SupplierBill) []string {
	return filterActions(ctx, b.Actions(), billActionPerms)
}

// BillView adalah tagihan beserta data tampilan.
type BillView struct {
	domain.SupplierBill
	DivisionName  string
	CreatedByName string
	Products      map[uuid.UUID]ProductRef
	// Receipts memetakan baris GR ke nomor GR & batch untuk tampilan 3-way match.
	Receipts map[uuid.UUID]postgres.ReceiptLineRef
}

func billSnapshot(b domain.SupplierBill) map[string]any {
	lines := make([]map[string]any, len(b.Lines))
	for i, l := range b.Lines {
		lines[i] = map[string]any{
			"receiptLineId": l.ReceiptLineID, "qty": l.Qty.String(), "unitPrice": l.UnitPrice.String(),
			"exceedsTolerance": l.ExceedsTolerance,
		}
	}
	return map[string]any{
		"number": b.Number, "purchaseOrderId": b.PurchaseOrderID, "billDate": b.BillDate.String(),
		"dueDate": b.DueDate.String(), "subtotalAmount": b.Subtotal.String(), "withholdingType": string(b.WithholdingType),
		"withholdingAmount": b.WithholdingAmount.String(), "advanceAppliedAmount": b.AdvanceApplied.String(),
		"totalPayableAmount": b.TotalPayable.String(), "matchStatus": string(b.MatchStatus), "status": string(b.Status),
		"note": b.Note, "lines": lines,
	}
}

// ListBills mengembalikan daftar tagihan pemasok.
func (s *Service) ListBills(ctx context.Context, f BillFilter, page paging.Page) (paging.Result[domain.SupplierBill], error) {
	if _, err := actor.Require(ctx, PermBillRead); err != nil {
		return paging.Result[domain.SupplierBill]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListBills(ctx, f, page)
}

// GetBill mengembalikan detail tagihan pemasok.
func (s *Service) GetBill(ctx context.Context, id uuid.UUID) (BillView, error) {
	if _, err := actor.Require(ctx, PermBillRead); err != nil {
		return BillView{}, err
	}
	b, err := s.Store.GetBill(ctx, id, false)
	if err != nil {
		return BillView{}, err
	}
	return s.billView(ctx, b)
}

func (s *Service) billView(ctx context.Context, b domain.SupplierBill) (BillView, error) {
	v := BillView{SupplierBill: b}
	ids, lineIDs := []uuid.UUID{}, []uuid.UUID{}
	for _, l := range b.Lines {
		ids, lineIDs = append(ids, l.ProductID), append(lineIDs, l.ReceiptLineID)
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return BillView{}, err
	}
	if v.Receipts, err = s.Store.ReceiptLinesByIDs(ctx, uniqueIDs(lineIDs)); err != nil {
		return BillView{}, err
	}
	divisions, err := s.Ledger.DivisionNames(ctx, []uuid.UUID{b.DivisionID})
	if err != nil {
		return BillView{}, err
	}
	users, err := s.Users.UserNames(ctx, []uuid.UUID{b.CreatedBy})
	if err != nil {
		return BillView{}, err
	}
	v.DivisionName, v.CreatedByName = divisions[b.DivisionID], users[b.CreatedBy]
	return v, nil
}

// BillableReceipts adalah data form tagihan: baris GR PO yang belum ditagih, PPh default pemasok, termin, sisa
// uang muka, dan toleransi 3-way match.
type BillableReceipts struct {
	Order              domain.PurchaseOrder
	Lines              []postgres.ReceiptLineRef
	PendingQty         map[uuid.UUID]decimal.Decimal
	Products           map[uuid.UUID]ProductRef
	DefaultWithholding domain.WithholdingType
	OutstandingAdvance money.Amount
}

// BillableReceipts mengembalikan baris Penerimaan Barang PO yang masih bisa ditagih.
func (s *Service) BillableReceipts(ctx context.Context, orderID uuid.UUID) (BillableReceipts, error) {
	if _, err := actor.RequireAny(ctx, PermBillWrite, PermBillRead); err != nil {
		return BillableReceipts{}, err
	}
	o, err := s.Store.GetOrder(ctx, orderID, false)
	if err != nil {
		return BillableReceipts{}, err
	}
	out := BillableReceipts{Order: o, OutstandingAdvance: money.Zero}
	all, err := s.Store.ReceiptLinesByOrder(ctx, orderID)
	if err != nil {
		return BillableReceipts{}, err
	}
	ids, productIDs := []uuid.UUID{}, []uuid.UUID{}
	for _, l := range all {
		if l.Line.OpenQty().IsPositive() {
			out.Lines = append(out.Lines, l)
			ids, productIDs = append(ids, l.Line.ID), append(productIDs, l.Line.ProductID)
		}
	}
	if out.PendingQty, err = s.Store.PendingBillQty(ctx, ids, uuid.Nil); err != nil {
		return BillableReceipts{}, err
	}
	if out.Products, err = s.Catalog.Products(ctx, uniqueIDs(productIDs)); err != nil {
		return BillableReceipts{}, err
	}
	if out.DefaultWithholding, err = s.defaultWithholding(ctx, o.SupplierID); err != nil {
		return BillableReceipts{}, err
	}
	advances, err := s.OutstandingAdvances(ctx, orderID)
	if err != nil {
		return BillableReceipts{}, err
	}
	for _, a := range advances {
		out.OutstandingAdvance = out.OutstandingAdvance.Add(a.Outstanding())
	}
	return out, nil
}

func (s *Service) defaultWithholding(ctx context.Context, supplierID uuid.UUID) (domain.WithholdingType, error) {
	sup, ok, err := s.Suppliers.Supplier(ctx, supplierID)
	if err != nil || !ok {
		return domain.WithholdingNone, err
	}
	w := domain.WithholdingType(sup.DefaultWithholding)
	if !w.Valid() {
		w = domain.WithholdingNone
	}
	return w, nil
}

// billRefs membaca baris GR (terkunci lewat GR), qty di tagihan lain yang menunggu persetujuan, dan PPh default.
func (s *Service) billRefs(ctx context.Context, o domain.PurchaseOrder, lineIDs []uuid.UUID, billID uuid.UUID) (domain.BillRefs, map[uuid.UUID]postgres.ReceiptLineRef, error) {
	refs := domain.BillRefs{Order: o, ReceiptLines: map[uuid.UUID]domain.BillReceiptLine{}}
	lines, err := s.lockReceiptLines(ctx, lineIDs)
	if err != nil {
		return refs, nil, err
	}
	pending, err := s.Store.PendingBillQty(ctx, uniqueIDs(lineIDs), billID)
	if err != nil {
		return refs, nil, err
	}
	for id, l := range lines {
		refs.ReceiptLines[id] = domain.BillReceiptLine{
			Line: l.Line, ReceiptID: l.ReceiptID, ReceiptNumber: l.ReceiptNumber, ReceiptStatus: l.ReceiptStatus,
			PurchaseOrderID: l.PurchaseOrderID, PendingQty: pending[id],
		}
	}
	if refs.DefaultWithholding, err = s.defaultWithholding(ctx, o.SupplierID); err != nil {
		return refs, nil, err
	}
	return refs, lines, nil
}

func billLineIDs(in domain.BillInput) []uuid.UUID {
	ids := make([]uuid.UUID, len(in.Lines))
	for i, l := range in.Lines {
		ids[i] = l.ReceiptLineID
	}
	return ids
}

func (s *Service) checkBillNumber(ctx context.Context, supplierID uuid.UUID, number string, except uuid.UUID) error {
	taken, err := s.Store.BillNumberTaken(ctx, supplierID, strings.TrimSpace(number), except)
	if err != nil {
		return err
	}
	if taken {
		return apperr.Validation(map[string]string{"number": "Nomor tagihan sudah dipakai tagihan lain dari pemasok ini"})
	}
	return nil
}

// CreateBill menyimpan draft tagihan pemasok dan menampilkan hasil 3-way match.
func (s *Service) CreateBill(ctx context.Context, in domain.BillInput) (BillView, error) {
	a, err := actor.Require(ctx, PermBillWrite)
	if err != nil {
		return BillView{}, err
	}
	var b domain.SupplierBill
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.lockOrder(ctx, in.PurchaseOrderID, "purchaseOrderId")
		if err != nil {
			return err
		}
		refs, _, err := s.billRefs(ctx, o, billLineIDs(in), uuid.Nil)
		if err != nil {
			return err
		}
		if b, err = domain.NewSupplierBill(in, refs, a.UserID); err != nil {
			return err
		}
		if err := s.checkBillNumber(ctx, b.SupplierID, b.Number, uuid.Nil); err != nil {
			return err
		}
		if b.ID, err = s.Store.InsertBill(ctx, b, a.UserID); err != nil {
			return err
		}
		if b, err = s.Store.GetBill(ctx, b.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntitySupplierBill, EntityID: b.ID, EntityLabel: b.Label(),
			After: billSnapshot(b),
		})
	})
	if err != nil {
		return BillView{}, err
	}
	return s.billView(ctx, b)
}

// mutateBill mengunci PO lalu tagihan, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit.
func (s *Service) mutateBill(ctx context.Context, id uuid.UUID, version *int, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, b *domain.SupplierBill, o domain.PurchaseOrder) (string, error),
) (domain.SupplierBill, error) {
	a, _ := actor.From(ctx)
	var b domain.SupplierBill
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetBill(ctx, id, false)
		if err != nil {
			return err
		}
		o, err := s.Store.GetOrder(ctx, cur.PurchaseOrderID, true)
		if err != nil {
			return err
		}
		if b, err = s.Store.GetBill(ctx, id, true); err != nil {
			return err
		}
		if version != nil && b.Version != *version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		v := b.Version
		before := billSnapshot(b)
		note, err := fn(ctx, &b, o)
		if err != nil {
			return err
		}
		if err := s.Store.SaveBill(ctx, b, v, a.UserID, replaceLines); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntitySupplierBill, EntityID: b.ID, EntityLabel: b.Label(),
			Before: before, After: billSnapshot(b), Note: note,
		})
	})
	return b, err
}

func (s *Service) billResult(ctx context.Context, id uuid.UUID, err error) (BillView, error) {
	if err != nil {
		return BillView{}, err
	}
	b, err := s.Store.GetBill(ctx, id, false)
	if err != nil {
		return BillView{}, err
	}
	return s.billView(ctx, b)
}

// UpdateBill mengubah draft tagihan.
func (s *Service) UpdateBill(ctx context.Context, id uuid.UUID, version int, in domain.BillInput) (BillView, error) {
	if _, err := actor.Require(ctx, PermBillWrite); err != nil {
		return BillView{}, err
	}
	_, err := s.mutateBill(ctx, id, &version, audit.ActionUpdate, true, func(ctx context.Context, b *domain.SupplierBill, o domain.PurchaseOrder) (string, error) {
		if b.Status != domain.StatusDraft {
			return "", b.Update(in, domain.BillRefs{})
		}
		refs, _, err := s.billRefs(ctx, o, billLineIDs(in), b.ID)
		if err != nil {
			return "", err
		}
		if err := b.Update(in, refs); err != nil {
			return "", err
		}
		return "", s.checkBillNumber(ctx, b.SupplierID, b.Number, b.ID)
	})
	return s.billResult(ctx, id, err)
}

// PostBill memposting tagihan pemasok setelah 3-way match ulang terhadap GR saat ini. Dalam toleransi (qty 0%,
// harga 1%): jurnal A-18, potongan uang muka A-19, qty ditagih PO/GR, dan utang terbentuk di transaksi yang sama.
// Di luar toleransi: tagihan Menunggu Persetujuan `supplier_bill_exception` (Head F&A) dan pembayaran diblokir —
// jurnal dan utang baru dibentuk saat disetujui.
func (s *Service) PostBill(ctx context.Context, id uuid.UUID, version int) (BillView, error) {
	if _, err := actor.Require(ctx, PermBillPost); err != nil {
		return BillView{}, err
	}
	_, err := s.mutateBill(ctx, id, &version, audit.ActionPost, true, func(ctx context.Context, b *domain.SupplierBill, o domain.PurchaseOrder) (string, error) {
		if b.Status != domain.StatusDraft {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tagihan "+b.Number+" berstatus "+b.Status.Label()+".")
		}
		in := b.Input()
		refs, lines, err := s.billRefs(ctx, o, billLineIDs(in), b.ID)
		if err != nil {
			return "", err
		}
		if err := in.Validate(refs); err != nil {
			return "", err
		}
		b.Match(refs)
		if b.ExceedsTolerance() {
			if err := b.Submit(s.Clock.Now()); err != nil {
				return "", err
			}
			division := b.DivisionID
			_, err := s.Approvals.Request(ctx, approval.Subject{
				Type: approval.SubjectSupplierBillException, ID: b.ID, Label: b.Number, Link: BillPath(b.ID),
				Summary: billExceptionSummary(*b), Amount: b.Subtotal, Division: &division,
			})
			return "Di luar toleransi 3-way match; menunggu persetujuan Head Finance & Accounting", err
		}
		return s.finalizeBill(ctx, b, o, lines)
	})
	return s.billResult(ctx, id, err)
}

func billExceptionSummary(b domain.SupplierBill) string {
	n := 0
	for _, l := range b.Lines {
		if l.ExceedsTolerance {
			n++
		}
	}
	return b.SupplierName + " · " + b.OrderNumber + " · " + itoa(n) + " baris di luar toleransi · selisih harga " + b.PriceVariance.Rupiah()
}

func itoa(n int) string { return decimal.NewFromInt(int64(n)).String() }

// finalizeBill membentuk jurnal A-18 dan A-19 (potongan uang muka), menambah qty ditagih GR & PO, dan utang.
func (s *Service) finalizeBill(ctx context.Context, b *domain.SupplierBill, o domain.PurchaseOrder, lines map[uuid.UUID]postgres.ReceiptLineRef) (string, error) {
	outstanding, err := s.OutstandingAdvances(ctx, o.ID)
	if err != nil {
		return "", err
	}
	available := money.Zero
	for _, a := range outstanding {
		available = available.Add(a.Outstanding())
	}
	take := b.PayableBeforeAdvance()
	if available.Cmp(take) < 0 {
		take = available
	}
	var advances []domain.BillAdvance
	if take.IsPositive() {
		applied, err := s.ApplyAdvances(ctx, o.ID, take)
		if err != nil {
			return "", err
		}
		for _, a := range applied {
			advances = append(advances, domain.BillAdvance{AdvanceID: a.AdvanceID, AdvanceNumber: a.AdvanceNumber, Amount: a.Amount})
		}
	}
	division := b.DivisionID
	party := &accounting.Party{Type: PartySupplier, ID: b.SupplierID}
	entry, err := s.Ledger.Post(ctx, accounting.PostingRequest{
		RuleCode: RuleSupplierBill, SourceType: SourceSupplierBill, SourceID: b.ID, SourceLabel: b.Number,
		BusinessDate: b.BillDate, Dimensions: accounting.Dimensions{DivisionID: &division}, Party: party,
		Amounts: map[string]money.Amount{
			"grni_value": b.GRNIAmount, "price_variance": b.PriceVariance, "payable_value": b.PayableBeforeAdvance(),
			"withholding_value": b.WithholdingAmount,
		},
		Accounts:    map[accounting.Resolver]string{accounting.ResolverWithholding: b.WithholdingType.Account()},
		Description: "Tagihan pemasok " + b.Number + " · " + o.Number + " · " + o.SupplierName,
	})
	if err != nil {
		return "", err
	}
	var advanceEntryID *uuid.UUID
	note := "Jurnal " + entry.Number
	appliedTotal := money.Zero
	for _, a := range advances {
		appliedTotal = appliedTotal.Add(a.Amount)
	}
	if appliedTotal.IsPositive() {
		adv, err := s.Ledger.Post(ctx, accounting.PostingRequest{
			RuleCode: RuleAdvance, SourceType: SourceSupplierBill, SourceID: b.ID, SourceLabel: b.Number,
			BusinessDate: b.BillDate, Dimensions: accounting.Dimensions{DivisionID: &division}, Party: party,
			Amounts:     map[string]money.Amount{"advance_applied": appliedTotal},
			Description: "Potong uang muka " + o.Number + " pada tagihan " + b.Number,
		})
		if err != nil {
			return "", err
		}
		advanceEntryID = &adv.ID
		note += ", potong uang muka " + adv.Number
	}
	if err := b.Finalize(advances, entry.ID, advanceEntryID, s.Clock.Now()); err != nil {
		return "", err
	}
	if err := s.Store.InsertBillAdvances(ctx, b.ID, advances); err != nil {
		return "", err
	}
	changed := []uuid.UUID{}
	deltas := map[uuid.UUID]decimal.Decimal{}
	for _, l := range b.Lines {
		ref := lines[l.ReceiptLineID]
		ref.Line.BilledQty = ref.Line.BilledQty.Add(l.Qty)
		ref.Line.BilledGRNIAmount = ref.Line.BilledGRNIAmount.Add(l.GRNIAmount)
		lines[l.ReceiptLineID] = ref
		changed = append(changed, l.ReceiptLineID)
		deltas[l.OrderLineID] = deltas[l.OrderLineID].Add(l.Qty)
	}
	if err := s.saveReceiptLineChanges(ctx, lines, changed); err != nil {
		return "", err
	}
	orderDeltas := make([]domain.QtyDelta, 0, len(deltas))
	for _, l := range o.Lines {
		if q, ok := deltas[l.ID]; ok {
			orderDeltas = append(orderDeltas, domain.QtyDelta{LineID: l.ID, Qty: q})
		}
	}
	if _, err := s.RecordBilling(ctx, o.ID, b.Number, orderDeltas); err != nil {
		return "", err
	}
	if err := s.Store.InsertPayable(ctx, domain.Payable{
		DocumentType: domain.PayableSupplierBill, DocumentID: b.ID, DocumentNumber: b.Number, SupplierID: b.SupplierID,
		PurchaseOrderID: b.PurchaseOrderID, DivisionID: b.DivisionID, BusinessDate: b.BillDate, DueDate: b.DueDate,
		Amount: b.TotalPayable, OpenAmount: b.TotalPayable,
	}, actor.UserID(ctx)); err != nil {
		return "", err
	}
	return note, nil
}

// ReviseBill mengembalikan tagihan yang ditolak ke Draft.
func (s *Service) ReviseBill(ctx context.Context, id uuid.UUID, version int) (BillView, error) {
	if _, err := actor.Require(ctx, PermBillWrite); err != nil {
		return BillView{}, err
	}
	_, err := s.mutateBill(ctx, id, &version, audit.ActionStatusChange, false, func(_ context.Context, b *domain.SupplierBill, _ domain.PurchaseOrder) (string, error) {
		return "Direvisi", b.Revise()
	})
	return s.billResult(ctx, id, err)
}

// CancelBill membatalkan tagihan Draft atau Ditolak.
func (s *Service) CancelBill(ctx context.Context, id uuid.UUID, version int, reason string) (BillView, error) {
	if _, err := actor.Require(ctx, PermBillCancel); err != nil {
		return BillView{}, err
	}
	_, err := s.mutateBill(ctx, id, &version, audit.ActionCancel, false, func(_ context.Context, b *domain.SupplierBill, _ domain.PurchaseOrder) (string, error) {
		return strings.TrimSpace(reason), b.Cancel(reason)
	})
	return s.billResult(ctx, id, err)
}

// OnBillExceptionApproved memposting tagihan yang selisihnya disetujui Head F&A (transaksi keputusan approval).
// GR dicocokkan ulang; qty yang sudah tidak tersedia ditolak sehingga keputusan gagal dan bisa diulang.
func (s *Service) OnBillExceptionApproved(ctx context.Context, d approval.Decided) error {
	_, err := s.mutateBill(ctx, d.Subject.ID, nil, audit.ActionStatusChange, true, func(ctx context.Context, b *domain.SupplierBill, o domain.PurchaseOrder) (string, error) {
		if b.Status != domain.StatusPendingApproval {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tagihan "+b.Number+" tidak sedang menunggu persetujuan.")
		}
		in := b.Input()
		refs, lines, err := s.billRefs(ctx, o, billLineIDs(in), b.ID)
		if err != nil {
			return "", err
		}
		if err := in.Validate(refs); err != nil {
			return "", err
		}
		b.Match(refs)
		note, err := s.finalizeBill(ctx, b, o, lines)
		return "Selisih 3-way match disetujui. " + note, err
	})
	return err
}

// OnBillExceptionRejected menandai tagihan Ditolak; pembuat bisa merevisi atau membatalkan.
func (s *Service) OnBillExceptionRejected(ctx context.Context, d approval.Decided) error {
	_, err := s.mutateBill(ctx, d.Subject.ID, nil, audit.ActionStatusChange, false, func(_ context.Context, b *domain.SupplierBill, _ domain.PurchaseOrder) (string, error) {
		return "Selisih 3-way match ditolak", b.Reject()
	})
	return err
}

// --- utang pemasok (dipakai pembayaran R1.4) ---------------------------------------------------

// OpenPayables mengembalikan utang terbuka pemasok (tagihan positif, nota debit negatif) urut jatuh tempo, tanpa
// cek permission.
func (s *Service) OpenPayables(ctx context.Context, supplierID uuid.UUID) ([]domain.Payable, error) {
	return s.Store.OpenPayables(ctx, supplierID)
}

// SettlePayable mengurangi sisa satu item utang di transaksi aktif (pembayaran atau kompensasi nota debit, R1.4).
// Untuk tagihan pemasok, status tagihan ikut menjadi Dibayar Sebagian/Lunas. Tidak memeriksa permission.
func (s *Service) SettlePayable(ctx context.Context, payableID uuid.UUID, amount money.Amount) (domain.Payable, error) {
	var p domain.Payable
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if p, err = s.Store.GetPayableForUpdate(ctx, payableID); err != nil {
			return err
		}
		if p.DocumentType == domain.PayableSupplierBill {
			b, err := s.Store.GetBill(ctx, p.DocumentID, true)
			if err != nil {
				return err
			}
			version := b.Version
			if err := b.ApplyPayment(amount); err != nil {
				return err
			}
			if err := s.Store.SaveBill(ctx, b, version, actor.UserID(ctx), false); err != nil {
				return err
			}
		}
		if err := p.Settle(amount); err != nil {
			return err
		}
		if err := s.Store.SavePayableOpen(ctx, p, actor.UserID(ctx)); err != nil {
			return err
		}
		p.Version++
		return nil
	})
	return p, err
}
