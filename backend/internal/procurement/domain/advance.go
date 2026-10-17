package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// advanceTransitions: Draft → Diposting (jurnal A-19) → Dibatalkan (jurnal dibalik, hanya bila belum dipotong
// tagihan); Draft → Dibatalkan.
var advanceTransitions = transitions{
	StatusDraft:  {StatusPosted, StatusCancelled},
	StatusPosted: {StatusCancelled},
}

// cashBankAccount: akun pembayaran uang muka harus akun Kas & Bank (11xx, COA Desain Finance & Accounting 1).
var cashBankAccount = regexp.MustCompile(`^11[0-9]{2}$`)

// Advance adalah uang muka pembelian per PO (PRC-09, A-19).
type Advance struct {
	ID              uuid.UUID
	Number          string
	PurchaseOrderID uuid.UUID
	OrderNumber     string
	SupplierID      uuid.UUID
	SupplierName    string
	DivisionID      uuid.UUID
	Amount          money.Amount
	PaymentDate     clock.BusinessDate
	BankAccountCode string
	Note            string
	// Applied adalah bagian uang muka yang sudah dipotong dari tagihan pemasok.
	Applied                money.Amount
	Status                 Status
	JournalEntryID         *uuid.UUID
	ReversalJournalEntryID *uuid.UUID
	CancelReason           string
	PostedAt               *time.Time
	CreatedBy              uuid.UUID
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Version                int
}

// Outstanding adalah sisa uang muka yang belum dipotong tagihan. Hanya uang muka Diposting yang punya sisa.
func (a Advance) Outstanding() money.Amount {
	if a.Status != StatusPosted {
		return money.Zero
	}
	return a.Amount.Sub(a.Applied)
}

// Counts memeriksa apakah uang muka dihitung terhadap batas total PO (belum dibatalkan).
func (a Advance) Counts() bool { return a.Status == StatusDraft || a.Status == StatusPosted }

// AdvanceInput adalah isian form uang muka.
type AdvanceInput struct {
	PurchaseOrderID uuid.UUID
	Amount          money.Amount
	PaymentDate     clock.BusinessDate
	BankAccountCode string
	Note            string
}

// Validate memeriksa isian uang muka terhadap PO dan total uang muka lain di PO yang sama.
func (in AdvanceInput) Validate(o PurchaseOrder, otherAdvances money.Amount) error {
	fields := map[string]string{}
	if !o.AcceptsAdvance() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; uang muka hanya untuk PO yang sudah disetujui dan belum selesai.",
			o.Label(), o.Status.Label()))
	}
	if msg := validAmount(in.Amount, true); msg != "" {
		fields["amount"] = msg
	} else if limit := o.Total.Sub(otherAdvances); in.Amount.Cmp(limit) > 0 {
		fields["amount"] = "Total uang muka melebihi total PO; sisa yang bisa dibayar " + limit.Rupiah()
	}
	if in.PaymentDate.IsZero() {
		fields["paymentDate"] = "Tanggal bayar wajib diisi"
	}
	if !cashBankAccount.MatchString(strings.TrimSpace(in.BankAccountCode)) {
		fields["bankAccountCode"] = "Pilih akun Kas & Bank (11xx)"
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (a *Advance) apply(in AdvanceInput, o PurchaseOrder) {
	a.PurchaseOrderID, a.OrderNumber, a.SupplierID, a.SupplierName, a.DivisionID = o.ID, o.Number, o.SupplierID, o.SupplierName, o.DivisionID
	a.Amount, a.PaymentDate, a.BankAccountCode, a.Note = in.Amount, in.PaymentDate, strings.TrimSpace(in.BankAccountCode), strings.TrimSpace(in.Note)
}

// NewAdvance membuat draft uang muka.
func NewAdvance(in AdvanceInput, o PurchaseOrder, otherAdvances money.Amount, createdBy uuid.UUID) (Advance, error) {
	if err := in.Validate(o, otherAdvances); err != nil {
		return Advance{}, err
	}
	a := Advance{Status: StatusDraft, Applied: money.Zero, CreatedBy: createdBy}
	a.apply(in, o)
	return a, nil
}

// Update mengubah draft uang muka. PO tidak bisa diganti.
func (a *Advance) Update(in AdvanceInput, o PurchaseOrder, otherAdvances money.Amount) error {
	if a.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya uang muka berstatus Draft yang bisa diubah.")
	}
	if in.PurchaseOrderID != a.PurchaseOrderID {
		return apperr.Validation(map[string]string{"purchaseOrderId": "Purchase Order uang muka tidak bisa diganti"})
	}
	if err := in.Validate(o, otherAdvances); err != nil {
		return err
	}
	a.apply(in, o)
	return nil
}

func (a *Advance) transition(to Status) error {
	if !advanceTransitions.allowed(a.Status, to) {
		return invalidTransition("Uang muka", a.Status, to)
	}
	a.Status = to
	return nil
}

// CanPost memeriksa apakah draft boleh diposting terhadap PO saat ini.
func (a Advance) CanPost(o PurchaseOrder, otherAdvances money.Amount) error {
	if a.Status != StatusDraft {
		return invalidTransition("Uang muka", a.Status, StatusPosted)
	}
	return AdvanceInput{
		PurchaseOrderID: a.PurchaseOrderID, Amount: a.Amount, PaymentDate: a.PaymentDate,
		BankAccountCode: a.BankAccountCode, Note: a.Note,
	}.Validate(o, otherAdvances)
}

// Post menandai uang muka diposting dengan nomor BYR dan jurnal A-19.
func (a *Advance) Post(number string, journalID uuid.UUID, at time.Time) error {
	if err := a.transition(StatusPosted); err != nil {
		return err
	}
	a.Number, a.JournalEntryID, a.PostedAt = number, &journalID, &at
	return nil
}

// Cancel membatalkan uang muka. Uang muka Diposting wajib alasan dan belum dipotong tagihan; jurnalnya dibalik
// oleh service di transaksi yang sama.
func (a *Advance) Cancel(reason string) error {
	reason = strings.TrimSpace(reason)
	if a.Status == StatusPosted {
		var err error
		if reason, err = requireReason(reason, "reason", "pembatalan"); err != nil {
			return err
		}
		if a.Applied.IsPositive() {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Uang muka sudah dipotong tagihan pemasok sebesar "+a.Applied.Rupiah()+" sehingga tidak bisa dibatalkan.")
		}
	}
	if len([]rune(reason)) > MaxReasonLength {
		return apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	if err := a.transition(StatusCancelled); err != nil {
		return err
	}
	a.CancelReason = reason
	return nil
}

// Apply memotong uang muka (tagihan pemasok, S-208). Nilai negatif mengembalikan potongan (tagihan dibatalkan).
func (a *Advance) Apply(amount money.Amount) error {
	if a.Status != StatusPosted {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya uang muka Diposting yang bisa dipotong.")
	}
	next := a.Applied.Add(amount)
	if next.IsNegative() || next.Cmp(a.Amount) > 0 || !amount.Equal(amount.Round()) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Potongan uang muka %s tidak valid; sisa uang muka %s.", amount.Rupiah(), a.Outstanding().Rupiah()))
	}
	a.Applied = next
	return nil
}

// Actions mengembalikan aksi yang dimungkinkan status uang muka (belum memperhitungkan permission).
func (a Advance) Actions() []Action {
	switch a.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionPost, ActionCancel}
	case StatusPosted:
		if a.Applied.IsZero() {
			return []Action{ActionCancel}
		}
	case StatusPendingApproval, StatusApproved, StatusRejected, StatusPartiallyReceived, StatusReceived,
		StatusCompleted, StatusClosed, StatusCancelled:
	}
	return nil
}

// AllocateAdvances membagi potongan total ke uang muka berurutan (FIFO tanggal bayar sesuai urutan input).
// Mengembalikan potongan per uang muka; total dibatasi sisa uang muka.
func AllocateAdvances(advances []Advance, total money.Amount) map[uuid.UUID]money.Amount {
	out := map[uuid.UUID]money.Amount{}
	left := total
	for _, a := range advances {
		if !left.IsPositive() {
			break
		}
		o := a.Outstanding()
		if !o.IsPositive() {
			continue
		}
		take := o
		if left.Cmp(o) < 0 {
			take = left
		}
		out[a.ID] = take
		left = left.Sub(take)
	}
	return out
}
