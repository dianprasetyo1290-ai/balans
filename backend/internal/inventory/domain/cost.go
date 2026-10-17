package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/money"
)

// UnitCostScale dan AmountScale mengikuti Technical Guideline 5.8.6.
const (
	UnitCostScale = money.UnitCostScale
	AmountScale   = money.AmountScale
)

// Layer adalah cost layer FIFO di dalam satu batch (INV-03).
type Layer struct {
	ID        uuid.UUID
	Remaining decimal.Decimal
	UnitCost  decimal.Decimal
}

// Consumption adalah qty yang dikonsumsi dari satu cost layer.
type Consumption struct {
	LayerID  uuid.UUID
	Qty      decimal.Decimal
	UnitCost decimal.Decimal
}

// LayerSet adalah cost layer satu batch dalam urutan FIFO (tanggal bisnis penerimaan, lalu urutan dibuat).
type LayerSet struct {
	Layers []Layer
}

// Available adalah total sisa qty seluruh layer.
func (s *LayerSet) Available() decimal.Decimal {
	total := decimal.Zero
	for _, l := range s.Layers {
		total = total.Add(l.Remaining)
	}
	return total
}

// plan menghitung konsumsi FIFO tanpa mengubah layer. ok false bila sisa layer kurang dari qty.
func (s *LayerSet) plan(qty decimal.Decimal) ([]Consumption, bool) {
	remaining := qty
	var out []Consumption
	for _, l := range s.Layers {
		if !remaining.IsPositive() {
			break
		}
		if !l.Remaining.IsPositive() {
			continue
		}
		take := decimal.Min(l.Remaining, remaining)
		out = append(out, Consumption{LayerID: l.ID, Qty: take, UnitCost: l.UnitCost})
		remaining = remaining.Sub(take)
	}
	return out, !remaining.IsPositive()
}

// Consume mengonsumsi qty FIFO dan mengurangi sisa layer. ok false (layer tidak berubah) bila tidak cukup.
func (s *LayerSet) Consume(qty decimal.Decimal) ([]Consumption, bool) {
	cons, ok := s.plan(qty)
	if !ok {
		return nil, false
	}
	for _, c := range cons {
		for i := range s.Layers {
			if s.Layers[i].ID == c.LayerID {
				s.Layers[i].Remaining = s.Layers[i].Remaining.Sub(c.Qty)
			}
		}
	}
	return cons, true
}

// Peek menghitung nilai qty FIFO tanpa mengonsumsi (dipakai pindah lokasi/status untuk nilai barang yang
// berpindah). ok false bila tidak cukup.
func (s *LayerSet) Peek(qty decimal.Decimal) ([]Consumption, bool) { return s.plan(qty) }

// Add menambah layer baru di akhir antrean FIFO.
func (s *LayerSet) Add(l Layer) { s.Layers = append(s.Layers, l) }

// Changed mengembalikan layer yang sisanya berbeda dari before (untuk disimpan).
func (s *LayerSet) Changed(before map[uuid.UUID]decimal.Decimal) []Layer {
	var out []Layer
	for _, l := range s.Layers {
		if b, ok := before[l.ID]; ok && !b.Equal(l.Remaining) {
			out = append(out, l)
		}
	}
	return out
}

// CostAmount adalah total HPP konsumsi, dibulatkan half-up ke 2 desimal (TG 5.8.6).
func CostAmount(cons []Consumption) decimal.Decimal {
	total := decimal.Zero
	for _, c := range cons {
		total = total.Add(c.Qty.Mul(c.UnitCost))
	}
	return money.FromDecimal(total).Round().Decimal()
}

// LineAmount adalah qty × HPP per unit dibulatkan ke 2 desimal.
func LineAmount(qty, unitCost decimal.Decimal) decimal.Decimal {
	return money.FromDecimal(qty.Mul(unitCost)).Round().Decimal()
}

// AverageUnitCost adalah HPP per unit konsumsi (4 desimal); satu layer = biaya layer itu.
func AverageUnitCost(cons []Consumption) decimal.Decimal {
	if len(cons) == 1 {
		return cons[0].UnitCost
	}
	qty, total := decimal.Zero, decimal.Zero
	for _, c := range cons {
		qty = qty.Add(c.Qty)
		total = total.Add(c.Qty.Mul(c.UnitCost))
	}
	if qty.IsZero() {
		return decimal.Zero
	}
	return money.FromDecimal(total.Div(qty)).RoundUnitCost().Decimal()
}
