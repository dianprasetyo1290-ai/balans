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

// ProformaValidityDays adalah masa berlaku Proforma Invoice bawaan (BRD 5.4 usulan 14 hari; dapat diatur lewat
// Deps modul sampai modul setting dibangun).
const ProformaValidityDays = 14

// Proforma adalah Proforma Invoice (PI): permintaan pembayaran pesanan prepaid (BRD 5.4, K-10).
type Proforma struct {
	ID           uuid.UUID
	Number       string
	OrderID      uuid.UUID
	OrderNumber  string
	Amount       money.Amount
	IssueDate    clock.BusinessDate
	ValidUntil   clock.BusinessDate
	Status       Status
	PaidAt       *time.Time
	InvoiceID    *uuid.UUID
	CancelReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// NewProforma menerbitkan PI untuk pesanan yang dikonfirmasi; berlaku validDays hari sejak tanggal terbit.
func NewProforma(number string, o Order, issue clock.BusinessDate, validDays int) Proforma {
	if validDays <= 0 {
		validDays = ProformaValidityDays
	}
	return Proforma{
		Number: number, OrderID: o.ID, OrderNumber: o.Number, Amount: o.Total, IssueDate: issue,
		ValidUntil: issue.AddDays(validDays), Status: StatusAwaitingPayment,
	}
}

var proformaTransitions = transitions{
	StatusAwaitingPayment: {StatusPaid, StatusExpired, StatusCancelled},
}

func (p *Proforma) transition(to Status) error {
	if !proformaTransitions.allowed(p.Status, to) {
		return invalidTransition("Proforma Invoice "+p.Number, p.Status, to)
	}
	p.Status = to
	return nil
}

// IsExpired bernilai true bila PI belum dibayar dan masa berlakunya sudah lewat pada tanggal today.
func (p Proforma) IsExpired(today clock.BusinessDate) bool {
	return p.Status == StatusAwaitingPayment && today.After(p.ValidUntil)
}

// MarkPaid mencatat pembayaran PI dan invoice final yang terbit.
func (p *Proforma) MarkPaid(invoiceID uuid.UUID, today clock.BusinessDate, now time.Time) error {
	if p.IsExpired(today) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Proforma Invoice %s sudah lewat masa berlaku (%s). Buat Sales Order baru.", p.Number, p.ValidUntil.String()))
	}
	if err := p.transition(StatusPaid); err != nil {
		return err
	}
	p.InvoiceID, p.PaidAt = &invoiceID, &now
	return nil
}

// Expire menandai PI kedaluwarsa.
func (p *Proforma) Expire() error { return p.transition(StatusExpired) }

// Cancel membatalkan PI bersama pesanannya.
func (p *Proforma) Cancel(reason string) error {
	if err := p.transition(StatusCancelled); err != nil {
		return err
	}
	p.CancelReason = strings.TrimSpace(reason)
	return nil
}

// Actions adalah aksi PI sebelum disaring permission.
func (p Proforma) Actions(today clock.BusinessDate) []Action {
	if p.Status == StatusAwaitingPayment && !p.IsExpired(today) {
		return []Action{ActionConfirmPay, ActionPrint}
	}
	return []Action{ActionPrint}
}

// InvoiceLine adalah baris invoice.
type InvoiceLine struct {
	ID          uuid.UUID
	LineNo      int
	ProductID   *uuid.UUID
	Description string
	Qty         decimal.Decimal
	UnitPrice   money.Amount
	Discount    money.Amount
	Total       money.Amount
}

// Invoice adalah invoice NON-PKP (tanpa PPN, SLS-02) dan piutang per invoice (ACC-03).
type Invoice struct {
	ID           uuid.UUID
	Number       string
	Type         string
	OrderID      *uuid.UUID
	OrderNumber  string
	PartyType    string
	PartyID      uuid.UUID
	PartyCode    string
	PartyName    string
	ChannelID    uuid.UUID
	SubChannelID uuid.UUID
	InvoiceDate  clock.BusinessDate
	DueDate      clock.BusinessDate
	// Gross = harga sebelum diskon; Discount = diskon baris + dokumen; Total = Gross − Discount + Shipping.
	Gross        money.Amount
	Discount     money.Amount
	Shipping     money.Amount
	Total        money.Amount
	Paid         money.Amount
	Status       Status
	CancelReason string
	CancelledAt  *time.Time
	Lines        []InvoiceLine
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// InvoiceTypeSales adalah invoice penjualan dari Sales Order.
const InvoiceTypeSales = "sales"

// NewInvoiceFromOrder menyusun invoice dari pesanan: tanggal invoice = tanggal terbit, jatuh tempo = tanggal +
// termin pesanan. productLabel memberi uraian baris (SKU – nama).
func NewInvoiceFromOrder(number string, o Order, invoiceDate clock.BusinessDate, productLabel func(uuid.UUID) string) (Invoice, error) {
	partyType, partyID := o.PartyType()
	if partyID == nil {
		return Invoice{}, apperr.Validation(map[string]string{"customerId": "Invoice membutuhkan pelanggan atau mitra"})
	}
	inv := Invoice{
		Number: number, Type: InvoiceTypeSales, OrderID: &o.ID, OrderNumber: o.Number, PartyType: partyType,
		PartyID: *partyID, PartyCode: o.PartyCode, PartyName: o.PartyName, ChannelID: o.ChannelID,
		SubChannelID: o.SubChannelID, InvoiceDate: invoiceDate, DueDate: invoiceDate.AddDays(o.PaymentTermDays),
		Gross: o.Gross, Discount: o.TotalDiscount(), Shipping: o.Shipping, Total: o.Total, Paid: money.Zero,
		Status: StatusAwaitingPayment,
	}
	for i, l := range o.Lines {
		pid := l.ProductID
		inv.Lines = append(inv.Lines, InvoiceLine{
			LineNo: i + 1, ProductID: &pid, Description: productLabel(l.ProductID), Qty: l.Qty,
			UnitPrice: l.Price.UnitPrice, Discount: l.DiscountAmount, Total: l.Total,
		})
	}
	return inv, nil
}

// Outstanding adalah sisa piutang invoice.
func (i Invoice) Outstanding() money.Amount {
	if i.Status == StatusCancelled {
		return money.Zero
	}
	return i.Total.Sub(i.Paid)
}

// IsOpen bernilai true bila invoice masih punya sisa piutang.
func (i Invoice) IsOpen() bool {
	return (i.Status == StatusAwaitingPayment || i.Status == StatusPartiallyPaid) && i.Outstanding().IsPositive()
}

// ApplyPayment menambah (atau, saat pembayaran dibatalkan, mengurangi dengan nilai negatif) nominal terbayar dan
// menyesuaikan status Menunggu Pembayaran/Dibayar Sebagian/Lunas.
func (i *Invoice) ApplyPayment(amount money.Amount) error {
	if i.Status == StatusCancelled {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Invoice "+i.Number+" sudah dibatalkan.")
	}
	paid := i.Paid.Add(amount)
	switch {
	case paid.IsNegative():
		return apperr.Validation(map[string]string{"amount": "Nominal pembatalan melebihi pembayaran invoice " + i.Number})
	case paid.Cmp(i.Total) > 0:
		return apperr.Validation(map[string]string{"amount": fmt.Sprintf(
			"Pembayaran melebihi sisa piutang invoice %s (%s)", i.Number, i.Outstanding().Rupiah())})
	}
	i.Paid = paid
	switch {
	case paid.Equal(i.Total):
		i.Status = StatusPaid
	case paid.IsPositive():
		i.Status = StatusPartiallyPaid
	default:
		i.Status = StatusAwaitingPayment
	}
	return nil
}

// Cancel membatalkan invoice yang belum dibayar (jurnal A-05 dibalik service).
func (i *Invoice) Cancel(reason string, now time.Time) error {
	if i.Status != StatusAwaitingPayment || i.Paid.IsPositive() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Invoice %s berstatus %s; hanya invoice tanpa pembayaran yang bisa dibatalkan.", i.Number, i.Status.Label()))
	}
	i.Status, i.CancelReason, i.CancelledAt = StatusCancelled, strings.TrimSpace(reason), &now
	return nil
}

// DaysOverdue adalah jumlah hari lewat jatuh tempo pada tanggal asOf (0 bila belum jatuh tempo).
func (i Invoice) DaysOverdue(asOf clock.BusinessDate) int {
	if !asOf.After(i.DueDate) {
		return 0
	}
	return int(asOf.Time().Sub(i.DueDate.Time()).Hours() / 24)
}

// Actions adalah aksi invoice sebelum disaring permission.
func (i Invoice) Actions() []Action {
	acts := []Action{ActionPrint, ActionShareWhatsApp}
	if i.IsOpen() {
		acts = append(acts, ActionRecordPayment)
	}
	return acts
}
