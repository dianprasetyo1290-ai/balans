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

// ReturnOrigin adalah asal retur pembelian (Desain Data Model 5).
type ReturnOrigin string

const (
	// ReturnManual dibuat Purchasing/Gudang dan butuh persetujuan Head Logistik.
	ReturnManual ReturnOrigin = "manual"
	// ReturnQCInbound dan ReturnQCOutbound dibuat modul gudang dari hasil QC dan diposting tanpa approval (D-24).
	ReturnQCInbound  ReturnOrigin = "qc_inbound"
	ReturnQCOutbound ReturnOrigin = "qc_outbound"
)

var returnOriginLabels = map[ReturnOrigin]string{
	ReturnManual: "Manual", ReturnQCInbound: "Reject QC Inbound", ReturnQCOutbound: "Reject QC Outbound",
}

// Valid memeriksa asal retur.
func (o ReturnOrigin) Valid() bool { _, ok := returnOriginLabels[o]; return ok }

// Label mengembalikan label UI asal retur.
func (o ReturnOrigin) Label() string { return returnOriginLabels[o] }

// IsQC bernilai true untuk retur dari hasil QC (tanpa approval).
func (o ReturnOrigin) IsQC() bool { return o == ReturnQCInbound || o == ReturnQCOutbound }

// Status stok asal retur (kode inventory.StockStatus).
const (
	StockQuarantine = "quarantine"
	StockAvailable  = "available"
)

// returnTransitions: Desain Status Dokumen 2.5.
var returnTransitions = transitions{
	StatusDraft:           {StatusPendingApproval, StatusPosted, StatusCancelled},
	StatusPendingApproval: {StatusApproved, StatusRejected},
	StatusApproved:        {StatusPosted, StatusCancelled},
	StatusRejected:        {StatusDraft},
}

// ReturnLine adalah satu baris retur terhadap baris Penerimaan Barang.
type ReturnLine struct {
	ID            uuid.UUID
	LineNo        int
	ReceiptLineID uuid.UUID
	ProductID     uuid.UUID
	BatchID       *uuid.UUID
	LocationID    uuid.UUID
	StockStatus   string
	Qty           decimal.Decimal
	// UnitCost dan Amount adalah HPP FIFO barang yang keluar (diisi saat diposting).
	UnitCost decimal.Decimal
	Amount   money.Amount
	// UnbilledQty/UnbilledAmount: bagian yang belum ditagih (mengurangi 2102); sisanya mengurangi utang 2101.
	UnbilledQty      decimal.Decimal
	UnbilledAmount   money.Amount
	InventoryAccount string
	Note             string
}

// BilledAmount adalah bagian baris yang mengurangi utang usaha (nota debit).
func (l ReturnLine) BilledAmount() money.Amount { return l.Amount.Sub(l.UnbilledAmount) }

// PurchaseReturn adalah retur pembelian (RB) dengan nota debit (DN) (ACC-16, PRC-05).
type PurchaseReturn struct {
	ID              uuid.UUID
	Number          string
	DebitNoteNumber string
	SupplierID      uuid.UUID
	SupplierName    string
	PurchaseOrderID uuid.UUID
	OrderNumber     string
	DivisionID      uuid.UUID
	Origin          ReturnOrigin
	SourceType      string
	SourceID        *uuid.UUID
	SourceLabel     string
	BusinessDate    clock.BusinessDate
	Reason          string
	Note            string
	Total           money.Amount
	UnbilledAmount  money.Amount
	BilledAmount    money.Amount
	Status          Status
	JournalEntryID  *uuid.UUID
	CancelReason    string
	SubmittedAt     *time.Time
	ApprovedAt      *time.Time
	PostedAt        *time.Time
	Lines           []ReturnLine
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// Label mengembalikan nomor RB atau keterangan draft.
func (r PurchaseReturn) Label() string {
	if r.Number != "" {
		return r.Number
	}
	return "Draft retur " + r.OrderNumber
}

// ReturnLineInput adalah isian satu baris retur.
type ReturnLineInput struct {
	ReceiptLineID uuid.UUID
	LocationID    uuid.UUID
	StockStatus   string
	Qty           decimal.Decimal
	Note          string
}

// ReturnInput adalah isian retur pembelian.
type ReturnInput struct {
	PurchaseOrderID uuid.UUID
	Origin          ReturnOrigin
	SourceType      string
	SourceID        *uuid.UUID
	SourceLabel     string
	BusinessDate    clock.BusinessDate
	Reason          string
	Note            string
	Lines           []ReturnLineInput
}

// ReturnReceiptLine adalah baris GR asal retur.
type ReturnReceiptLine struct {
	Line            ReceiptLine
	ReceiptNumber   string
	ReceiptStatus   Status
	PurchaseOrderID uuid.UUID
	// PendingQty adalah qty baris ini di retur lain yang belum diposting (Draft/Menunggu/Disetujui).
	PendingQty decimal.Decimal
}

// ReturnRefs adalah data rujukan validasi retur.
type ReturnRefs struct {
	Order        PurchaseOrder
	ReceiptLines map[uuid.UUID]ReturnReceiptLine
	// Locations berisi lokasi yang ditemukan dan aktif.
	Locations map[uuid.UUID]bool
}

// Validate memeriksa isian retur: baris GR yang sudah diposting di PO yang sama, qty ≤ sisa yang bisa diretur,
// lokasi & status stok asal.
func (in *ReturnInput) Validate(refs ReturnRefs) error {
	o := refs.Order
	if o.Status == StatusDraft || o.Status == StatusPendingApproval || o.Status == StatusRejected || o.Status == StatusCancelled {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; retur hanya untuk PO yang sudah ada penerimaannya.", o.Label(), o.Status.Label()))
	}
	fields := map[string]string{}
	if in.Origin == "" {
		in.Origin = ReturnManual
	}
	if !in.Origin.Valid() {
		fields["origin"] = "Asal retur tidak dikenal"
	}
	in.Reason, in.Note = strings.TrimSpace(in.Reason), strings.TrimSpace(in.Note)
	in.SourceType, in.SourceLabel = strings.TrimSpace(in.SourceType), strings.TrimSpace(in.SourceLabel)
	switch {
	case in.Reason == "":
		fields["reason"] = "Alasan retur wajib diisi"
	case len([]rune(in.Reason)) > MaxReasonLength:
		fields["reason"] = "Alasan maksimal 400 karakter"
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal retur wajib diisi"
	}
	if in.Origin.IsQC() && (in.SourceType == "" || in.SourceID == nil) {
		fields["sourceId"] = "Retur asal QC wajib menyebut dokumen QC"
	}
	in.validateLines(refs, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (in *ReturnInput) validateLines(refs ReturnRefs, fields map[string]string) {
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
		return
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Maksimal 200 baris"
	}
	used := map[uuid.UUID]decimal.Decimal{}
	for i := range in.Lines {
		l := &in.Lines[i]
		l.Note = strings.TrimSpace(l.Note)
		if l.StockStatus == "" {
			l.StockStatus = StockQuarantine
		}
		p := fmt.Sprintf("lines.%d.", i)
		rl, ok := refs.ReceiptLines[l.ReceiptLineID]
		switch {
		case !ok || rl.PurchaseOrderID != refs.Order.ID:
			fields[p+"receiptLineId"] = "Baris Penerimaan Barang tidak ditemukan di PO ini"
			continue
		case rl.ReceiptStatus != StatusPosted:
			fields[p+"receiptLineId"] = "Penerimaan Barang " + rl.ReceiptNumber + " belum diposting atau sudah dibatalkan"
			continue
		}
		switch {
		case l.LocationID == uuid.Nil:
			fields[p+"locationId"] = "Lokasi asal barang wajib dipilih"
		case !refs.Locations[l.LocationID]:
			fields[p+"locationId"] = "Lokasi tidak ditemukan atau nonaktif"
		}
		if l.StockStatus != StockQuarantine && l.StockStatus != StockAvailable {
			fields[p+"stockStatus"] = "Status stok asal harus Karantina atau Tersedia"
		}
		if msg := validQty(l.Qty); msg != "" {
			fields[p+"qty"] = msg
		} else {
			used[l.ReceiptLineID] = used[l.ReceiptLineID].Add(l.Qty)
			if left := rl.Line.ReturnableQty().Sub(rl.PendingQty); used[l.ReceiptLineID].GreaterThan(left) {
				fields[p+"qty"] = fmt.Sprintf("Melebihi qty %s yang bisa diretur (%s)", rl.ReceiptNumber, left.String())
			}
		}
		if len([]rune(l.Note)) > 500 {
			fields[p+"note"] = "Catatan maksimal 500 karakter"
		}
	}
}

func (r *PurchaseReturn) apply(in ReturnInput, refs ReturnRefs) {
	o := refs.Order
	r.SupplierID, r.SupplierName, r.PurchaseOrderID, r.OrderNumber, r.DivisionID = o.SupplierID, o.SupplierName, o.ID, o.Number, o.DivisionID
	r.Origin, r.SourceType, r.SourceID, r.SourceLabel = in.Origin, in.SourceType, in.SourceID, in.SourceLabel
	r.BusinessDate, r.Reason, r.Note = in.BusinessDate, in.Reason, in.Note
	r.Lines = make([]ReturnLine, len(in.Lines))
	for i, l := range in.Lines {
		rl := refs.ReceiptLines[l.ReceiptLineID]
		r.Lines[i] = ReturnLine{
			LineNo: i + 1, ReceiptLineID: rl.Line.ID, ProductID: rl.Line.ProductID, BatchID: rl.Line.BatchID,
			LocationID: l.LocationID, StockStatus: l.StockStatus, Qty: l.Qty, UnitCost: decimal.Zero, Amount: money.Zero,
			UnbilledQty: decimal.Zero, UnbilledAmount: money.Zero, Note: l.Note,
		}
	}
	r.Total, r.UnbilledAmount, r.BilledAmount = money.Zero, money.Zero, money.Zero
}

// NewPurchaseReturn membuat draft retur.
func NewPurchaseReturn(in ReturnInput, refs ReturnRefs, createdBy uuid.UUID) (PurchaseReturn, error) {
	if err := in.Validate(refs); err != nil {
		return PurchaseReturn{}, err
	}
	r := PurchaseReturn{Status: StatusDraft, CreatedBy: createdBy}
	r.apply(in, refs)
	return r, nil
}

// Update mengganti isi draft retur manual. PO dan asal tidak bisa diganti.
func (r *PurchaseReturn) Update(in ReturnInput, refs ReturnRefs) error {
	if r.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya retur berstatus Draft yang bisa diubah.")
	}
	if in.PurchaseOrderID != r.PurchaseOrderID {
		return apperr.Validation(map[string]string{"purchaseOrderId": "Purchase Order retur tidak bisa diganti"})
	}
	in.Origin, in.SourceType, in.SourceID, in.SourceLabel = r.Origin, r.SourceType, r.SourceID, r.SourceLabel
	if err := in.Validate(refs); err != nil {
		return err
	}
	r.apply(in, refs)
	return nil
}

// Input mengembalikan isian retur (validasi ulang saat posting).
func (r PurchaseReturn) Input() ReturnInput {
	in := ReturnInput{
		PurchaseOrderID: r.PurchaseOrderID, Origin: r.Origin, SourceType: r.SourceType, SourceID: r.SourceID,
		SourceLabel: r.SourceLabel, BusinessDate: r.BusinessDate, Reason: r.Reason, Note: r.Note,
	}
	for _, l := range r.Lines {
		in.Lines = append(in.Lines, ReturnLineInput{
			ReceiptLineID: l.ReceiptLineID, LocationID: l.LocationID, StockStatus: l.StockStatus, Qty: l.Qty, Note: l.Note,
		})
	}
	return in
}

// ApplyCost mengisi HPP baris dari mutasi stok keluar dan membagi nilainya: bagian sebesar sisa qty GR yang
// belum ditagih mengurangi 2102, sisanya mengurangi utang 2101 (nota debit).
func (r *PurchaseReturn) ApplyCost(i int, cost money.Amount, openQty decimal.Decimal, account string) {
	l := &r.Lines[i]
	l.Amount, l.InventoryAccount = cost, account
	l.UnitCost = cost.Decimal().DivRound(l.Qty, UnitCostScale)
	l.UnbilledQty = decimal.Min(l.Qty, decimal.Max(openQty, decimal.Zero))
	switch {
	case l.UnbilledQty.Equal(l.Qty):
		l.UnbilledAmount = cost
	case l.UnbilledQty.IsZero():
		l.UnbilledAmount = money.Zero
	default:
		l.UnbilledAmount = money.FromDecimal(cost.Decimal().Mul(l.UnbilledQty).Div(l.Qty)).Round()
	}
	r.Total, r.UnbilledAmount, r.BilledAmount = money.Zero, money.Zero, money.Zero
	for _, x := range r.Lines {
		r.Total = r.Total.Add(x.Amount)
		r.UnbilledAmount = r.UnbilledAmount.Add(x.UnbilledAmount)
		r.BilledAmount = r.BilledAmount.Add(x.BilledAmount())
	}
}

func (r *PurchaseReturn) transition(to Status) error {
	if !returnTransitions.allowed(r.Status, to) {
		return invalidTransition("Retur pembelian "+r.Label(), r.Status, to)
	}
	r.Status = to
	return nil
}

// Submit mengajukan retur manual ke Head Logistik; nomor RB diberikan.
func (r *PurchaseReturn) Submit(number string, at time.Time) error {
	if r.Origin.IsQC() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Retur asal QC diposting tanpa persetujuan.")
	}
	if err := r.transition(StatusPendingApproval); err != nil {
		return err
	}
	if r.Number == "" {
		r.Number = number
	}
	r.SubmittedAt = &at
	return nil
}

// Approve dipanggil handler approval.
func (r *PurchaseReturn) Approve(at time.Time) error {
	if err := r.transition(StatusApproved); err != nil {
		return err
	}
	r.ApprovedAt = &at
	return nil
}

// Reject dipanggil handler approval saat ditolak.
func (r *PurchaseReturn) Reject() error { return r.transition(StatusRejected) }

// Revise mengembalikan retur yang ditolak ke Draft.
func (r *PurchaseReturn) Revise() error { return r.transition(StatusDraft) }

// CanPost memeriksa retur boleh diposting: manual harus sudah disetujui, asal QC langsung dari Draft.
func (r PurchaseReturn) CanPost() error {
	switch {
	case r.Origin.IsQC() && r.Status == StatusDraft, !r.Origin.IsQC() && r.Status == StatusApproved:
		return nil
	case !r.Origin.IsQC() && r.Status == StatusDraft:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Retur manual harus disetujui Head Logistik sebelum diposting.")
	}
	return invalidTransition("Retur pembelian "+r.Label(), r.Status, StatusPosted)
}

// Post menandai retur diposting dengan nomor RB, nomor nota debit, dan jurnal A-02.
func (r *PurchaseReturn) Post(number, debitNote string, journalID uuid.UUID, at time.Time) error {
	if err := r.CanPost(); err != nil {
		return err
	}
	if err := r.transition(StatusPosted); err != nil {
		return err
	}
	if r.Number == "" {
		r.Number = number
	}
	r.DebitNoteNumber, r.JournalEntryID, r.PostedAt = debitNote, &journalID, &at
	return nil
}

// Cancel membatalkan retur Draft atau Disetujui yang belum diposting.
func (r *PurchaseReturn) Cancel(reason string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > MaxReasonLength {
		return apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	if err := r.transition(StatusCancelled); err != nil {
		return err
	}
	r.CancelReason = reason
	return nil
}

// Actions mengembalikan aksi yang dimungkinkan status retur (belum memperhitungkan permission).
func (r PurchaseReturn) Actions() []Action {
	switch r.Status {
	case StatusDraft:
		if r.Origin.IsQC() {
			return []Action{ActionPost, ActionCancel}
		}
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusApproved:
		return []Action{ActionPost, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise}
	default:
	}
	return nil
}
