package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BundleComponent adalah satu SKU komponen bundel virtual beserta qty per satu bundel (MD-03).
type BundleComponent struct {
	ComponentID uuid.UUID
	Qty         decimal.Decimal
	// SKU, Name, Variant, UOM, dan Deleted diisi saat dibaca untuk tampilan.
	SKU     string
	Name    string
	Variant string
	UOM     string
	Deleted bool
}

// ComponentInput adalah isian komponen dari form bundel.
type ComponentInput struct {
	ComponentID uuid.UUID
	Qty         decimal.Decimal
}

// ComponentQty adalah kebutuhan qty satu komponen untuk sejumlah bundel.
type ComponentQty struct {
	ComponentID uuid.UUID
	Qty         decimal.Decimal
}

// AvailableBundleQty menghitung stok tersedia bundel virtual = min(floor(stok komponen ÷ qty per bundel))
// (BRD 5.7, MD-03). Komponen yang tidak ada di peta stok dianggap 0; stok negatif dianggap 0. Bundel tanpa
// komponen tidak tersedia.
func AvailableBundleQty(components []BundleComponent, stock map[uuid.UUID]decimal.Decimal) decimal.Decimal {
	if len(components) == 0 {
		return decimal.Zero
	}
	var available decimal.Decimal
	for i, c := range components {
		if !c.Qty.IsPositive() {
			return decimal.Zero
		}
		s := stock[c.ComponentID]
		if !s.IsPositive() {
			return decimal.Zero
		}
		n := s.Div(c.Qty).Floor()
		if i == 0 || n.LessThan(available) {
			available = n
		}
	}
	return available
}

// ExplodeBundle memecah qty bundel menjadi qty per komponen. Dipakai modul sales/storefront sebelum memanggil
// inventory, karena inventory tidak pernah menerima SKU bundel (Technical Guideline 5.7.4).
func ExplodeBundle(components []BundleComponent, qty decimal.Decimal) []ComponentQty {
	out := make([]ComponentQty, len(components))
	for i, c := range components {
		out[i] = ComponentQty{ComponentID: c.ComponentID, Qty: c.Qty.Mul(qty)}
	}
	return out
}
