package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LocationFlow adalah mutasi satu produk di satu lokasi dalam periode: saldo awal sebelum periode, masuk, keluar,
// dan keluar dari dokumen yang dikecualikan. Saldo akhir = OpeningQty + InQty − OutQty − ExcludedOutQty.
type LocationFlow struct {
	ProductID      uuid.UUID
	OpeningQty     decimal.Decimal
	InQty          decimal.Decimal
	OutQty         decimal.Decimal
	ExcludedOutQty decimal.Decimal
}

// ClosingQty adalah saldo akhir periode.
func (f LocationFlow) ClosingQty() decimal.Decimal {
	return f.OpeningQty.Add(f.InQty).Sub(f.OutQty).Sub(f.ExcludedOutQty)
}
