package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// CreditNoteApplication adalah bagian nota kredit yang mengurangi piutang satu invoice.
type CreditNoteApplication struct {
	ID            uuid.UUID
	InvoiceID     uuid.UUID
	InvoiceNumber string
	Amount        money.Amount
	AppliedDate   clock.BusinessDate
	CreatedAt     time.Time
}

// CreditNote adalah Nota Kredit (CN) dari Retur Penjualan. Piutang Usaha sudah dikredit jurnal A-13 saat retur
// diposting; CN mencatat saldo kredit pihak itu. `credit_note`: dialokasikan ke invoice asal lalu invoice
// berikutnya (Diposting → Selesai saat habis). `refund`: Menunggu Persetujuan Head F&A → Menunggu Pembayaran →
// Lunas setelah F&A membayar (jurnal A-PB Dr Piutang, Cr Kas/Bank). Refund ditolak menjadi saldo nota kredit.
type CreditNote struct {
	ID              uuid.UUID
	Number          string
	ReturnID        uuid.UUID
	ReturnNumber    string
	InvoiceID       *uuid.UUID
	InvoiceNumber   string
	PartyType       string
	PartyID         uuid.UUID
	PartyName       string
	ChannelID       uuid.UUID
	SubChannelID    uuid.UUID
	Settlement      SettlementMethod
	IssueDate       clock.BusinessDate
	Amount          money.Amount
	Applied         money.Amount
	Refunded        money.Amount
	Status          Status
	DecisionNote    string
	RefundDate      *clock.BusinessDate
	RefundAccount   string
	RefundReference string
	RefundJournalID *uuid.UUID
	Applications    []CreditNoteApplication
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// NewCreditNote membuat nota kredit dari retur yang diposting.
func NewCreditNote(number string, r SalesReturn, issue clock.BusinessDate) (CreditNote, error) {
	if r.PartyID == nil || r.Settlement == SettlementMarketplace || !r.Amount.IsPositive() {
		return CreditNote{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Retur ini tidak membutuhkan nota kredit.")
	}
	cn := CreditNote{
		Number: number, ReturnID: r.ID, ReturnNumber: r.Number, InvoiceID: r.InvoiceID, InvoiceNumber: r.InvoiceNumber,
		PartyType: r.PartyType, PartyID: *r.PartyID, PartyName: r.PartyName, ChannelID: r.ChannelID,
		SubChannelID: r.SubChannelID, Settlement: r.Settlement, IssueDate: issue, Amount: r.Amount,
		Applied: money.Zero, Refunded: money.Zero, Status: StatusPosted,
	}
	if r.Settlement == SettlementRefund {
		cn.Status = StatusPendingApproval
	}
	return cn, nil
}

// Remaining adalah saldo nota kredit yang belum dialokasikan/dibayarkan.
func (c CreditNote) Remaining() money.Amount { return c.Amount.Sub(c.Applied).Sub(c.Refunded) }

// CanApply bernilai true bila saldo nota kredit masih bisa dialokasikan ke invoice.
func (c CreditNote) CanApply() bool {
	return c.Settlement == SettlementCreditNote && c.Status == StatusPosted && c.Remaining().IsPositive()
}

// Apply mengalokasikan saldo nota kredit ke invoice terbuka pihak yang sama (≤ sisa piutang dan ≤ saldo).
func (c *CreditNote) Apply(inv Invoice, amount money.Amount, date clock.BusinessDate) (CreditNoteApplication, error) {
	f := fieldErrors{}
	switch {
	case !c.CanApply():
		return CreditNoteApplication{}, apperr.WithMessage(apperr.CodeInvalidStateTransition,
			fmt.Sprintf("Nota kredit %s berstatus %s dan tidak punya saldo untuk dialokasikan.", c.Number, c.Status.Label()))
	case inv.PartyType != c.PartyType || inv.PartyID != c.PartyID:
		f.add("invoiceId", "Invoice "+inv.Number+" milik pihak lain")
	case !inv.IsOpen():
		f.add("invoiceId", "Invoice "+inv.Number+" tidak punya sisa piutang")
	}
	if msg := validAmount(amount, true); msg != "" {
		f.add("amount", msg)
	} else if amount.Cmp(c.Remaining()) > 0 {
		f.add("amount", "Melebihi saldo nota kredit "+c.Remaining().Rupiah())
	} else if amount.Cmp(inv.Outstanding()) > 0 {
		f.add("amount", "Melebihi sisa piutang "+inv.Outstanding().Rupiah())
	}
	if err := f.err(); err != nil {
		return CreditNoteApplication{}, err
	}
	c.Applied = c.Applied.Add(amount)
	if !c.Remaining().IsPositive() {
		c.Status = StatusCompleted
	}
	app := CreditNoteApplication{InvoiceID: inv.ID, InvoiceNumber: inv.Number, Amount: amount, AppliedDate: date}
	c.Applications = append(c.Applications, app)
	return app, nil
}

// ApproveRefund: Head F&A menyetujui refund → Menunggu Pembayaran.
func (c *CreditNote) ApproveRefund(note string) error {
	if c.Settlement != SettlementRefund || c.Status != StatusPendingApproval {
		return invalidTransition("Nota kredit "+c.Number, c.Status, StatusAwaitingPayment)
	}
	c.Status, c.DecisionNote = StatusAwaitingPayment, strings.TrimSpace(note)
	return nil
}

// RejectRefund: refund ditolak → saldo menjadi nota kredit untuk invoice berikutnya.
func (c *CreditNote) RejectRefund(note string) error {
	if c.Settlement != SettlementRefund || c.Status != StatusPendingApproval {
		return invalidTransition("Nota kredit "+c.Number, c.Status, StatusPosted)
	}
	c.Settlement, c.Status, c.DecisionNote = SettlementCreditNote, StatusPosted, strings.TrimSpace(note)
	return nil
}

// RefundInput adalah pembayaran refund oleh F&A.
type RefundInput struct {
	PaymentDate clock.BusinessDate
	AccountCode string
	Reference   string
}

// PayRefund mencatat pembayaran refund penuh dari akun Kas & Bank.
func (c *CreditNote) PayRefund(in RefundInput, cashAccounts []string) error {
	in.Reference = strings.TrimSpace(in.Reference)
	if c.Settlement != SettlementRefund || c.Status != StatusAwaitingPayment {
		return invalidTransition("Nota kredit "+c.Number, c.Status, StatusPaid)
	}
	f := fieldErrors{}
	switch {
	case in.PaymentDate.IsZero():
		f.add("paymentDate", "Tanggal bayar wajib diisi")
	case in.PaymentDate.Before(c.IssueDate):
		f.add("paymentDate", "Tanggal bayar tidak boleh sebelum tanggal nota kredit")
	}
	found := false
	for _, a := range cashAccounts {
		found = found || a == in.AccountCode
	}
	if !found {
		f.add("accountCode", "Pilih akun Kas & Bank")
	}
	if tooLong(in.Reference, MaxTextLength) {
		f.add("reference", "Referensi maksimal 200 karakter")
	}
	if err := f.err(); err != nil {
		return err
	}
	d := in.PaymentDate
	c.Refunded, c.Status = c.Remaining(), StatusPaid
	c.RefundDate, c.RefundAccount, c.RefundReference = &d, in.AccountCode, in.Reference
	return nil
}

// Actions adalah aksi nota kredit sebelum disaring permission.
func (c CreditNote) Actions() []Action {
	acts := []Action{ActionPrint}
	switch {
	case c.CanApply():
		acts = append(acts, ActionApply)
	case c.Settlement == SettlementRefund && c.Status == StatusAwaitingPayment:
		acts = append(acts, ActionPayRefund)
	}
	return acts
}
