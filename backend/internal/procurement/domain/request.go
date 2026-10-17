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

// requestTransitions: Desain Status Dokumen 2.1. completed → approved terjadi otomatis bila PO dari PR dibatalkan
// sehingga sebagian qty belum menjadi PO lagi.
var requestTransitions = transitions{
	StatusDraft:           {StatusPendingApproval, StatusCancelled},
	StatusPendingApproval: {StatusApproved, StatusRejected},
	StatusRejected:        {StatusDraft},
	StatusApproved:        {StatusCompleted, StatusClosed},
	StatusCompleted:       {StatusApproved},
}

// MaxNoteLength membatasi catatan dokumen.
const MaxNoteLength = 1000

// RequestLine adalah satu baris Purchase Request: produk katalog atau uraian bebas.
type RequestLine struct {
	ID             uuid.UUID
	LineNo         int
	ProductID      *uuid.UUID
	Description    string
	Qty            decimal.Decimal
	EstimatedPrice money.Amount
	// FulfilledQty adalah qty yang sudah dijadikan PO.
	FulfilledQty decimal.Decimal
	Note         string
}

// RemainingQty adalah qty yang belum dijadikan PO.
func (l RequestLine) RemainingQty() decimal.Decimal { return l.Qty.Sub(l.FulfilledQty) }

// EstimatedAmount adalah qty × estimasi harga.
func (l RequestLine) EstimatedAmount() money.Amount { return LineAmount(l.Qty, l.EstimatedPrice) }

// PurchaseRequest adalah permintaan pembelian dari divisi (PRC-08).
type PurchaseRequest struct {
	ID             uuid.UUID
	Number         string
	DivisionID     uuid.UUID
	RequestedBy    uuid.UUID
	BusinessDate   clock.BusinessDate
	NeededDate     *clock.BusinessDate
	Note           string
	EstimatedTotal money.Amount
	Status         Status
	CloseReason    string
	SubmittedAt    *time.Time
	DecidedAt      *time.Time
	Lines          []RequestLine
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// RequestLineInput adalah isian satu baris PR.
type RequestLineInput struct {
	ProductID      *uuid.UUID
	Description    string
	Qty            decimal.Decimal
	EstimatedPrice money.Amount
	Note           string
}

// RequestInput adalah isian form PR.
type RequestInput struct {
	DivisionID   uuid.UUID
	BusinessDate clock.BusinessDate
	NeededDate   *clock.BusinessDate
	Note         string
	Lines        []RequestLineInput
}

// Validate memeriksa isian PR terhadap data produk katalog.
func (in RequestInput) Validate(products map[uuid.UUID]ProductInfo) error {
	fields := map[string]string{}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi peminta wajib dipilih"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	if in.NeededDate != nil && !in.BusinessDate.IsZero() && in.NeededDate.Before(in.BusinessDate) {
		fields["neededDate"] = "Tanggal dibutuhkan tidak boleh sebelum tanggal PR"
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris barang"
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Maksimal 200 baris"
	}
	for i, l := range in.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		if l.ProductID != nil {
			if msg := checkProduct(*l.ProductID, products, nil); msg != "" {
				fields[p+"productId"] = msg
			}
		} else if strings.TrimSpace(l.Description) == "" {
			fields[p+"productId"] = "Pilih produk atau isi uraian barang"
		}
		if msg := validQty(l.Qty); msg != "" {
			fields[p+"qty"] = msg
		}
		if msg := validAmount(l.EstimatedPrice, false); msg != "" {
			fields[p+"estimatedPrice"] = msg
		}
		if len([]rune(l.Description)) > 500 || len([]rune(l.Note)) > 500 {
			fields[p+"description"] = "Uraian dan catatan maksimal 500 karakter"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (r *PurchaseRequest) apply(in RequestInput) {
	r.DivisionID, r.BusinessDate, r.NeededDate, r.Note = in.DivisionID, in.BusinessDate, in.NeededDate, strings.TrimSpace(in.Note)
	r.Lines = make([]RequestLine, len(in.Lines))
	total := money.Zero
	for i, l := range in.Lines {
		r.Lines[i] = RequestLine{
			LineNo: i + 1, ProductID: l.ProductID, Description: strings.TrimSpace(l.Description), Qty: l.Qty,
			EstimatedPrice: l.EstimatedPrice, FulfilledQty: decimal.Zero, Note: strings.TrimSpace(l.Note),
		}
		total = total.Add(r.Lines[i].EstimatedAmount())
	}
	r.EstimatedTotal = total
}

// NewPurchaseRequest membuat draft PR.
func NewPurchaseRequest(in RequestInput, products map[uuid.UUID]ProductInfo, requestedBy uuid.UUID) (PurchaseRequest, error) {
	if err := in.Validate(products); err != nil {
		return PurchaseRequest{}, err
	}
	r := PurchaseRequest{RequestedBy: requestedBy, Status: StatusDraft}
	r.apply(in)
	return r, nil
}

// Update mengganti isi draft PR.
func (r *PurchaseRequest) Update(in RequestInput, products map[uuid.UUID]ProductInfo) error {
	if r.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya Purchase Request berstatus Draft yang bisa diubah.")
	}
	if err := in.Validate(products); err != nil {
		return err
	}
	r.apply(in)
	return nil
}

func (r *PurchaseRequest) transition(to Status) error {
	if !requestTransitions.allowed(r.Status, to) {
		return invalidTransition("Purchase Request", r.Status, to)
	}
	r.Status = to
	return nil
}

// Submit mengajukan PR. Nomor yang sudah ada (PR yang direvisi) dipertahankan.
func (r *PurchaseRequest) Submit(number string, at time.Time) error {
	if err := r.transition(StatusPendingApproval); err != nil {
		return err
	}
	if r.Number == "" {
		r.Number = number
	}
	r.SubmittedAt, r.DecidedAt = &at, nil
	return nil
}

// NeedsNumber memeriksa apakah PR belum bernomor saat diajukan.
func (r PurchaseRequest) NeedsNumber() bool { return r.Number == "" }

// Approve dipanggil handler approval setelah Head divisi menyetujui.
func (r *PurchaseRequest) Approve(at time.Time) error {
	if err := r.transition(StatusApproved); err != nil {
		return err
	}
	r.DecidedAt = &at
	return nil
}

// Reject dipanggil handler approval saat ditolak.
func (r *PurchaseRequest) Reject(at time.Time) error {
	if err := r.transition(StatusRejected); err != nil {
		return err
	}
	r.DecidedAt = &at
	return nil
}

// Revise mengembalikan PR yang ditolak ke Draft agar bisa diperbaiki dan diajukan ulang.
func (r *PurchaseRequest) Revise() error { return r.transition(StatusDraft) }

// Cancel membatalkan draft PR.
func (r *PurchaseRequest) Cancel() error { return r.transition(StatusCancelled) }

// Close menutup PR disetujui yang sisa qty-nya tidak akan dijadikan PO.
func (r *PurchaseRequest) Close(reason string) error {
	reason, err := requireReason(reason, "reason", "penutupan")
	if err != nil {
		return err
	}
	if err := r.transition(StatusClosed); err != nil {
		return err
	}
	r.CloseReason = reason
	return nil
}

// Line mengembalikan baris berdasarkan ID.
func (r *PurchaseRequest) Line(id uuid.UUID) (*RequestLine, bool) {
	for i := range r.Lines {
		if r.Lines[i].ID == id {
			return &r.Lines[i], true
		}
	}
	return nil, false
}

// FullyFulfilled memeriksa apakah semua baris sudah dijadikan PO.
func (r PurchaseRequest) FullyFulfilled() bool {
	for _, l := range r.Lines {
		if l.RemainingQty().IsPositive() {
			return false
		}
	}
	return true
}

// ApplyFulfillment menetapkan qty yang sudah dijadikan PO per baris dan menyesuaikan status: Selesai bila
// semua baris terpenuhi, kembali Disetujui bila PO terkait dibatalkan. PR Ditutup tetap Ditutup.
func (r *PurchaseRequest) ApplyFulfillment(fulfilled map[uuid.UUID]decimal.Decimal) error {
	for i := range r.Lines {
		q := fulfilled[r.Lines[i].ID]
		if q.IsNegative() || q.GreaterThan(r.Lines[i].Qty) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Qty PO untuk baris %d Purchase Request %s melebihi qty yang diminta.", r.Lines[i].LineNo, r.Number))
		}
		r.Lines[i].FulfilledQty = q
	}
	switch {
	case r.Status == StatusApproved && r.FullyFulfilled():
		return r.transition(StatusCompleted)
	case r.Status == StatusCompleted && !r.FullyFulfilled():
		return r.transition(StatusApproved)
	}
	return nil
}

// CanOrder memeriksa apakah baris PR boleh dijadikan PO.
func (r PurchaseRequest) CanOrder() error {
	if r.Status != StatusApproved {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Request %s berstatus %s; hanya PR Disetujui yang bisa dijadikan PO.", r.Number, r.Status.Label()))
	}
	return nil
}

// Actions mengembalikan aksi yang dimungkinkan status PR (belum memperhitungkan permission).
func (r PurchaseRequest) Actions() []Action {
	switch r.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise}
	case StatusApproved:
		return []Action{ActionCreateOrder, ActionClose}
	case StatusPendingApproval, StatusPartiallyReceived, StatusReceived, StatusCompleted, StatusClosed,
		StatusCancelled, StatusPosted:
	}
	return nil
}
