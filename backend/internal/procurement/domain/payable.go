package domain

import (
	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// PayableDocument adalah jenis dokumen sumber utang pemasok.
type PayableDocument string

const (
	// PayableSupplierBill: tagihan pemasok yang sudah diposting (nilai positif).
	PayableSupplierBill PayableDocument = "supplier_bill"
	// PayableDebitNote: nota debit retur atas barang yang sudah ditagih (nilai negatif, mengurangi utang).
	PayableDebitNote PayableDocument = "debit_note"
)

// Payable adalah satu item utang pemasok di sub-ledger 2101 (Desain Data Model payable_items).
type Payable struct {
	ID              uuid.UUID
	DocumentType    PayableDocument
	DocumentID      uuid.UUID
	DocumentNumber  string
	SupplierID      uuid.UUID
	PurchaseOrderID uuid.UUID
	DivisionID      uuid.UUID
	BusinessDate    clock.BusinessDate
	DueDate         clock.BusinessDate
	Amount          money.Amount
	OpenAmount      money.Amount
	Version         int
}

// Settle mengurangi sisa utang sebesar amount (bertanda sama dengan sisa; tagihan positif, nota debit negatif).
func (p *Payable) Settle(amount money.Amount) error {
	next := p.OpenAmount.Sub(amount)
	switch {
	case amount.IsZero():
		return apperr.Validation(map[string]string{"amount": "Nominal pelunasan tidak boleh nol"})
	case p.OpenAmount.IsPositive() && (amount.IsNegative() || next.IsNegative()),
		p.OpenAmount.IsNegative() && (amount.IsPositive() || next.IsPositive()),
		p.OpenAmount.IsZero():
		return apperr.Validation(map[string]string{"amount": "Nominal melebihi sisa utang " + p.OpenAmount.Rupiah()})
	}
	p.OpenAmount = next
	return nil
}
