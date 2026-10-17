package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Toleransi 3-way match (BRD 5.2, Desain Role & Approval 4 #12: qty ≠ 0% atau harga > 1%).
//
// TODO(setting): baca dari modul setting (Technical Guideline 5.18) saat modul pengaturan dibangun.
var (
	QtyTolerancePercent   = decimal.Zero
	PriceTolerancePercent = decimal.NewFromInt(1)
)

// WithholdingType adalah jenis PPh yang dipotong dari tagihan (kode party.WithholdingType).
type WithholdingType string

const (
	WithholdingNone  WithholdingType = "none"
	WithholdingPPh23 WithholdingType = "pph23"
	WithholdingPPh21 WithholdingType = "pph21"
	WithholdingPPh42 WithholdingType = "pph4_2"
)

var withholdingLabels = map[WithholdingType]string{
	WithholdingNone: "Tidak dipotong", WithholdingPPh23: "PPh 23", WithholdingPPh21: "PPh 21", WithholdingPPh42: "PPh 4(2)",
}

// Valid memeriksa jenis PPh.
func (w WithholdingType) Valid() bool { _, ok := withholdingLabels[w]; return ok }

// Label mengembalikan label UI jenis PPh.
func (w WithholdingType) Label() string { return withholdingLabels[w] }

// Account mengembalikan akun utang PPh (COA Desain Finance & Accounting 1).
func (w WithholdingType) Account() string {
	switch w {
	case WithholdingPPh21:
		return "2301"
	case WithholdingPPh42:
		return "2303"
	case WithholdingNone, WithholdingPPh23:
	}
	return "2302"
}

// DefaultRate adalah tarif awal PPh (persen dari subtotal tagihan) yang bisa diubah per tagihan.
//
// TODO(S-410): ganti dengan withholding_tax_rates effective-dated setelah konfirmasi konsultan pajak (Desain Finance
// & Accounting 4.2). PPh 21 bukan pegawai: 50% × 5% bruto.
func (w WithholdingType) DefaultRate() decimal.Decimal {
	switch w {
	case WithholdingPPh23:
		return decimal.NewFromInt(2)
	case WithholdingPPh21:
		return decimal.RequireFromString("2.5")
	case WithholdingPPh42:
		return decimal.NewFromInt(10)
	case WithholdingNone:
	}
	return decimal.Zero
}

// MatchStatus adalah hasil 3-way match tagihan.
type MatchStatus string

const (
	MatchMatched           MatchStatus = "matched"
	MatchExceedsTolerance  MatchStatus = "exceeds_tolerance"
	MatchApprovedException MatchStatus = "approved_exception"
)

// billTransitions: Desain Status Dokumen 2.4. Pembayaran (R1.4) memindahkan ke Dibayar Sebagian/Lunas.
var billTransitions = transitions{
	StatusDraft:           {StatusAwaitingPayment, StatusPendingApproval, StatusCancelled, StatusPaid},
	StatusPendingApproval: {StatusAwaitingPayment, StatusRejected, StatusPaid},
	StatusRejected:        {StatusDraft, StatusCancelled},
	StatusAwaitingPayment: {StatusPartiallyPaid, StatusPaid},
	StatusPartiallyPaid:   {StatusPaid},
}

// BillLine adalah satu baris tagihan terhadap satu baris Penerimaan Barang.
type BillLine struct {
	ID            uuid.UUID
	LineNo        int
	ReceiptLineID uuid.UUID
	OrderLineID   uuid.UUID
	ProductID     uuid.UUID
	Qty           decimal.Decimal
	// ReceiptOpenQty adalah qty GR yang belum ditagih saat dicocokkan.
	ReceiptOpenQty   decimal.Decimal
	UnitPrice        money.Amount
	POUnitPrice      money.Amount
	Amount           money.Amount
	GRNIAmount       money.Amount
	PriceVariance    money.Amount
	ExceedsTolerance bool
	Note             string
}

// QtyMismatch memeriksa selisih qty terhadap GR melebihi toleransi qty.
func (l BillLine) QtyMismatch() bool {
	if l.ReceiptOpenQty.IsZero() {
		return !l.Qty.IsZero()
	}
	diff := l.ReceiptOpenQty.Sub(l.Qty).Abs().Mul(decimal.NewFromInt(100)).Div(l.ReceiptOpenQty)
	return diff.GreaterThan(QtyTolerancePercent)
}

// PriceVariancePercent adalah selisih harga tagihan terhadap harga PO dalam persen (4 desimal).
func (l BillLine) PriceVariancePercent() decimal.Decimal {
	po := l.POUnitPrice.Decimal()
	if po.IsZero() {
		if l.UnitPrice.IsZero() {
			return decimal.Zero
		}
		return decimal.NewFromInt(100)
	}
	return l.UnitPrice.Decimal().Sub(po).Mul(decimal.NewFromInt(100)).DivRound(po, 4)
}

// PriceMismatch memeriksa selisih harga melebihi toleransi harga.
func (l BillLine) PriceMismatch() bool {
	return l.PriceVariancePercent().Abs().GreaterThan(PriceTolerancePercent)
}

// BillAdvance adalah uang muka yang dipotong tagihan.
type BillAdvance struct {
	AdvanceID     uuid.UUID
	AdvanceNumber string
	Amount        money.Amount
}

// SupplierBill adalah tagihan pemasok/maklon dengan 3-way match PO–GR–tagihan (PRC-07, ACC-04).
type SupplierBill struct {
	ID                    uuid.UUID
	Number                string
	SupplierID            uuid.UUID
	SupplierName          string
	PurchaseOrderID       uuid.UUID
	OrderNumber           string
	DivisionID            uuid.UUID
	BillDate              clock.BusinessDate
	DueDate               clock.BusinessDate
	Note                  string
	Subtotal              money.Amount
	GRNIAmount            money.Amount
	PriceVariance         money.Amount
	WithholdingType       WithholdingType
	WithholdingRate       decimal.Decimal
	WithholdingAmount     money.Amount
	AdvanceApplied        money.Amount
	TotalPayable          money.Amount
	PaidAmount            money.Amount
	MatchStatus           MatchStatus
	Status                Status
	JournalEntryID        *uuid.UUID
	AdvanceJournalEntryID *uuid.UUID
	CancelReason          string
	SubmittedAt           *time.Time
	PostedAt              *time.Time
	Lines                 []BillLine
	Advances              []BillAdvance
	CreatedBy             uuid.UUID
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Version               int
}

// Label mengembalikan nomor tagihan pemasok.
func (b SupplierBill) Label() string { return b.Number }

// OpenAmount adalah sisa utang tagihan.
func (b SupplierBill) OpenAmount() money.Amount { return b.TotalPayable.Sub(b.PaidAmount) }

// ExceedsTolerance memeriksa apakah ada baris di luar toleransi 3-way match.
func (b SupplierBill) ExceedsTolerance() bool {
	for _, l := range b.Lines {
		if l.ExceedsTolerance {
			return true
		}
	}
	return false
}

// PaymentBlocked bernilai true selama tagihan belum membentuk utang (draft, menunggu persetujuan selisih, ditolak).
func (b SupplierBill) PaymentBlocked() bool {
	return b.Status != StatusAwaitingPayment && b.Status != StatusPartiallyPaid
}

// BillLineInput adalah isian satu baris tagihan.
type BillLineInput struct {
	ReceiptLineID uuid.UUID
	Qty           decimal.Decimal
	UnitPrice     money.Amount
	Note          string
}

// BillInput adalah isian form tagihan pemasok.
type BillInput struct {
	PurchaseOrderID uuid.UUID
	Number          string
	BillDate        clock.BusinessDate
	// DueDate nil = tanggal tagihan + termin PO.
	DueDate *clock.BusinessDate
	// WithholdingType nil = PPh default pemasok; WithholdingRate nil = tarif awal jenis PPh.
	WithholdingType *WithholdingType
	WithholdingRate *decimal.Decimal
	Note            string
	Lines           []BillLineInput
}

// BillReceiptLine adalah baris GR yang bisa ditagih beserta PO-nya.
type BillReceiptLine struct {
	Line            ReceiptLine
	ReceiptID       uuid.UUID
	ReceiptNumber   string
	ReceiptStatus   Status
	PurchaseOrderID uuid.UUID
	// PendingQty adalah qty baris ini di tagihan lain yang sedang menunggu persetujuan.
	PendingQty decimal.Decimal
}

// BillRefs adalah data rujukan validasi tagihan.
type BillRefs struct {
	Order              PurchaseOrder
	ReceiptLines       map[uuid.UUID]BillReceiptLine
	DefaultWithholding WithholdingType
}

// MaxBillNumberLength membatasi nomor tagihan pemasok.
const MaxBillNumberLength = 60

// BillableOrder memeriksa PO bisa ditagih (sudah ada penerimaan, belum dibatalkan).
func BillableOrder(o PurchaseOrder) error {
	switch o.Status {
	case StatusPartiallyReceived, StatusReceived, StatusCompleted, StatusClosed:
		return nil
	default:
	}
	return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
		"Purchase Order %s berstatus %s; belum ada penerimaan yang bisa ditagih.", o.Label(), o.Status.Label()))
}

// Validate memeriksa isian tagihan terhadap PO dan baris GR yang belum ditagih.
func (in *BillInput) Validate(refs BillRefs) error {
	if err := BillableOrder(refs.Order); err != nil {
		return err
	}
	fields := map[string]string{}
	in.Number, in.Note = strings.TrimSpace(in.Number), strings.TrimSpace(in.Note)
	switch {
	case in.Number == "":
		fields["number"] = "Nomor tagihan pemasok wajib diisi"
	case len([]rune(in.Number)) > MaxBillNumberLength:
		fields["number"] = "Nomor tagihan maksimal 60 karakter"
	}
	switch {
	case in.BillDate.IsZero():
		fields["billDate"] = "Tanggal tagihan wajib diisi"
	case in.DueDate != nil && in.DueDate.Before(in.BillDate):
		fields["dueDate"] = "Jatuh tempo tidak boleh sebelum tanggal tagihan"
	}
	if in.WithholdingType != nil && !in.WithholdingType.Valid() {
		fields["withholdingType"] = "Jenis PPh tidak dikenal"
	}
	if r := in.WithholdingRate; r != nil {
		switch {
		case r.IsNegative() || r.GreaterThan(decimal.NewFromInt(100)):
			fields["withholdingRate"] = "Tarif PPh 0 sampai 100%"
		case !r.Equal(r.Round(4)):
			fields["withholdingRate"] = "Tarif maksimal 4 angka di belakang koma"
		}
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	in.validateLines(refs, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (in *BillInput) validateLines(refs BillRefs, fields map[string]string) {
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris penerimaan yang ditagih"
		return
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Maksimal 200 baris"
	}
	seen := map[uuid.UUID]bool{}
	for i := range in.Lines {
		l := &in.Lines[i]
		l.Note = strings.TrimSpace(l.Note)
		p := fmt.Sprintf("lines.%d.", i)
		rl, ok := refs.ReceiptLines[l.ReceiptLineID]
		switch {
		case !ok || rl.PurchaseOrderID != refs.Order.ID:
			fields[p+"receiptLineId"] = "Baris Penerimaan Barang tidak ditemukan di PO ini"
			continue
		case rl.ReceiptStatus != StatusPosted:
			fields[p+"receiptLineId"] = "Penerimaan Barang " + rl.ReceiptNumber + " belum diposting atau sudah dibatalkan"
			continue
		case seen[l.ReceiptLineID]:
			fields[p+"receiptLineId"] = "Baris penerimaan yang sama ditagih lebih dari sekali"
			continue
		}
		seen[l.ReceiptLineID] = true
		if msg := validQty(l.Qty); msg != "" {
			fields[p+"qty"] = msg
		} else if open := rl.Line.OpenQty().Sub(rl.PendingQty); l.Qty.GreaterThan(open) {
			fields[p+"qty"] = fmt.Sprintf("Melebihi qty %s yang belum ditagih (%s); tagihan hanya atas barang yang diterima",
				rl.ReceiptNumber, open.String())
		}
		if msg := validAmount(l.UnitPrice, false); msg != "" {
			fields[p+"unitPrice"] = msg
		}
		if len([]rune(l.Note)) > 500 {
			fields[p+"note"] = "Catatan maksimal 500 karakter"
		}
	}
}

func (b *SupplierBill) apply(in BillInput, refs BillRefs) {
	o := refs.Order
	b.Number, b.SupplierID, b.SupplierName, b.PurchaseOrderID = in.Number, o.SupplierID, o.SupplierName, o.ID
	b.OrderNumber, b.DivisionID, b.BillDate, b.Note = o.Number, o.DivisionID, in.BillDate, in.Note
	b.DueDate = in.BillDate.AddDays(o.PaymentTermDays)
	if in.DueDate != nil {
		b.DueDate = *in.DueDate
	}
	b.WithholdingType = refs.DefaultWithholding
	if in.WithholdingType != nil {
		b.WithholdingType = *in.WithholdingType
	}
	if !b.WithholdingType.Valid() {
		b.WithholdingType = WithholdingNone
	}
	b.WithholdingRate = b.WithholdingType.DefaultRate()
	if in.WithholdingRate != nil {
		b.WithholdingRate = *in.WithholdingRate
	}
	if b.WithholdingType == WithholdingNone {
		b.WithholdingRate = decimal.Zero
	}
	b.Lines = make([]BillLine, len(in.Lines))
	for i, l := range in.Lines {
		rl := refs.ReceiptLines[l.ReceiptLineID]
		b.Lines[i] = BillLine{
			LineNo: i + 1, ReceiptLineID: rl.Line.ID, OrderLineID: rl.Line.OrderLineID, ProductID: rl.Line.ProductID,
			Qty: l.Qty, UnitPrice: l.UnitPrice, POUnitPrice: rl.Line.UnitPrice, Note: l.Note,
		}
	}
	b.Match(refs)
}

// Match mencocokkan ulang baris dengan GR saat ini (qty belum ditagih) dan menghitung total, selisih harga
// (ke 5103), PPh (rupiah penuh), dan status match. Uang muka dihitung saat posting.
func (b *SupplierBill) Match(refs BillRefs) {
	b.Subtotal, b.GRNIAmount, b.PriceVariance = money.Zero, money.Zero, money.Zero
	for i := range b.Lines {
		l := &b.Lines[i]
		rl, ok := refs.ReceiptLines[l.ReceiptLineID]
		if ok {
			l.ReceiptOpenQty, l.POUnitPrice = rl.Line.OpenQty(), rl.Line.UnitPrice
		}
		l.Amount = LineAmount(l.Qty, l.UnitPrice)
		l.GRNIAmount = LineAmount(l.Qty, l.POUnitPrice)
		if ok && l.Qty.Equal(rl.Line.OpenQty()) {
			// Menagih seluruh sisa GR menutup saldo 2102 baris itu tanpa sisa pembulatan.
			l.GRNIAmount = rl.Line.OpenGRNIAmount()
		}
		l.PriceVariance = l.Amount.Sub(l.GRNIAmount)
		l.ExceedsTolerance = l.QtyMismatch() || l.PriceMismatch()
		b.Subtotal = b.Subtotal.Add(l.Amount)
		b.GRNIAmount = b.GRNIAmount.Add(l.GRNIAmount)
		b.PriceVariance = b.PriceVariance.Add(l.PriceVariance)
	}
	b.WithholdingAmount = b.Subtotal.Percent(b.WithholdingRate).RoundRupiah()
	if b.WithholdingAmount.Cmp(b.Subtotal) > 0 {
		b.WithholdingAmount = b.Subtotal
	}
	if b.MatchStatus != MatchApprovedException {
		b.MatchStatus = MatchMatched
		if b.ExceedsTolerance() {
			b.MatchStatus = MatchExceedsTolerance
		}
	}
	b.recalcPayable()
}

func (b *SupplierBill) recalcPayable() {
	b.TotalPayable = b.Subtotal.Sub(b.WithholdingAmount).Sub(b.AdvanceApplied)
}

// PayableBeforeAdvance adalah utang usaha sebelum dipotong uang muka (kredit 2101 A-18).
func (b SupplierBill) PayableBeforeAdvance() money.Amount { return b.Subtotal.Sub(b.WithholdingAmount) }

// NewSupplierBill membuat draft tagihan.
func NewSupplierBill(in BillInput, refs BillRefs, createdBy uuid.UUID) (SupplierBill, error) {
	if err := in.Validate(refs); err != nil {
		return SupplierBill{}, err
	}
	b := SupplierBill{Status: StatusDraft, AdvanceApplied: money.Zero, PaidAmount: money.Zero, CreatedBy: createdBy}
	b.apply(in, refs)
	return b, nil
}

// Update mengganti isi draft tagihan. PO tidak bisa diganti.
func (b *SupplierBill) Update(in BillInput, refs BillRefs) error {
	if b.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya tagihan berstatus Draft yang bisa diubah.")
	}
	if in.PurchaseOrderID != b.PurchaseOrderID {
		return apperr.Validation(map[string]string{"purchaseOrderId": "Purchase Order tagihan tidak bisa diganti"})
	}
	if err := in.Validate(refs); err != nil {
		return err
	}
	b.apply(in, refs)
	return nil
}

// Input mengembalikan isian draft (validasi ulang saat posting).
func (b SupplierBill) Input() BillInput {
	wt, rate, due := b.WithholdingType, b.WithholdingRate, b.DueDate
	in := BillInput{
		PurchaseOrderID: b.PurchaseOrderID, Number: b.Number, BillDate: b.BillDate, DueDate: &due,
		WithholdingType: &wt, WithholdingRate: &rate, Note: b.Note,
	}
	for _, l := range b.Lines {
		in.Lines = append(in.Lines, BillLineInput{ReceiptLineID: l.ReceiptLineID, Qty: l.Qty, UnitPrice: l.UnitPrice, Note: l.Note})
	}
	return in
}

func (b *SupplierBill) transition(to Status) error {
	if !billTransitions.allowed(b.Status, to) {
		return invalidTransition("Tagihan pemasok "+b.Label(), b.Status, to)
	}
	b.Status = to
	return nil
}

// Submit menandai tagihan di luar toleransi menunggu persetujuan Head F&A; pembayaran diblokir.
func (b *SupplierBill) Submit(at time.Time) error {
	if err := b.transition(StatusPendingApproval); err != nil {
		return err
	}
	b.MatchStatus, b.SubmittedAt = MatchExceedsTolerance, &at
	return nil
}

// Finalize memposting tagihan: jurnal A-18 (dan A-19 bila ada potongan uang muka) dan utang terbentuk.
// Tagihan dengan sisa utang nol langsung Lunas.
func (b *SupplierBill) Finalize(advances []BillAdvance, journalID uuid.UUID, advanceJournalID *uuid.UUID, at time.Time) error {
	applied := money.Zero
	for _, a := range advances {
		applied = applied.Add(a.Amount)
	}
	if applied.Cmp(b.PayableBeforeAdvance()) > 0 {
		return fmt.Errorf("finalize bill: potongan uang muka melebihi utang")
	}
	if b.Status == StatusPendingApproval {
		b.MatchStatus = MatchApprovedException
	}
	b.Advances, b.AdvanceApplied = advances, applied
	b.recalcPayable()
	to := StatusAwaitingPayment
	if !b.TotalPayable.IsPositive() {
		to = StatusPaid
	}
	if err := b.transition(to); err != nil {
		return err
	}
	b.JournalEntryID, b.AdvanceJournalEntryID, b.PostedAt = &journalID, advanceJournalID, &at
	return nil
}

// Reject dipanggil handler approval saat selisih ditolak.
func (b *SupplierBill) Reject() error { return b.transition(StatusRejected) }

// Revise mengembalikan tagihan yang ditolak ke Draft.
func (b *SupplierBill) Revise() error {
	if err := b.transition(StatusDraft); err != nil {
		return err
	}
	b.MatchStatus = MatchMatched
	return nil
}

// Cancel membatalkan tagihan Draft atau Ditolak.
func (b *SupplierBill) Cancel(reason string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > MaxReasonLength {
		return apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	if err := b.transition(StatusCancelled); err != nil {
		return err
	}
	b.CancelReason = reason
	return nil
}

// ApplyPayment mencatat pembayaran (R1.4) dan mengubah status Dibayar Sebagian/Lunas.
func (b *SupplierBill) ApplyPayment(amount money.Amount) error {
	if !amount.IsPositive() || amount.Cmp(b.OpenAmount()) > 0 {
		return apperr.Validation(map[string]string{"amount": "Pembayaran harus lebih dari nol dan tidak melebihi sisa utang " + b.OpenAmount().Rupiah()})
	}
	if b.PaymentBlocked() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tagihan "+b.Number+" belum bisa dibayar (status "+b.Status.Label()+").")
	}
	b.PaidAmount = b.PaidAmount.Add(amount)
	to := StatusPartiallyPaid
	if b.OpenAmount().IsZero() {
		to = StatusPaid
	}
	if to == b.Status {
		return nil
	}
	return b.transition(to)
}

// Actions mengembalikan aksi yang dimungkinkan status tagihan (belum memperhitungkan permission).
func (b SupplierBill) Actions() []Action {
	switch b.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionPost, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise, ActionCancel}
	default:
	}
	return nil
}
