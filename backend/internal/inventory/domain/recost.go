package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
)

// ReplayLayer adalah cost layer satu batch dalam urutan FIFO (tanggal bisnis penerimaan, lalu urutan dibuat).
type ReplayLayer struct {
	ID           uuid.UUID
	BusinessDate clock.BusinessDate
	ReceivedQty  decimal.Decimal
	UnitCost     decimal.Decimal
}

// ReplayIssue adalah movement keluar (issue) satu batch dalam urutan tanggal bisnis, lalu urutan dibuat.
// Qty positif. CurrentCost adalah HPP efektif saat ini (HPP posting + selisih recost sebelumnya).
type ReplayIssue struct {
	MovementID   uuid.UUID
	BusinessDate clock.BusinessDate
	Qty          decimal.Decimal
	CurrentCost  decimal.Decimal
}

// ReplayResult adalah hasil hitung ulang FIFO satu batch.
type ReplayResult struct {
	// Costs: HPP hasil hitung ulang per movement (dibulatkan 2 desimal seperti posting).
	Costs map[uuid.UUID]decimal.Decimal
	// Remaining: sisa qty per cost layer setelah hitung ulang.
	Remaining map[uuid.UUID]decimal.Decimal
	// Short: true bila layer tidak cukup untuk semua issue (ledger tidak konsisten; recost dihentikan).
	Short bool
}

// Replay menghitung ulang konsumsi FIFO seluruh issue satu batch terhadap seluruh cost layer-nya (S-224).
// Setiap issue mengambil lebih dulu layer yang tanggal bisnisnya ≤ tanggal issue (FIFO), baru layer
// sesudahnya bila belum cukup. Tanpa backdate hasilnya sama dengan konsumsi saat posting; dengan penerimaan
// bertanggal mundur, issue sesudah tanggal itu mengonsumsi layer mundur tersebut lebih dulu.
func Replay(layers []ReplayLayer, issues []ReplayIssue) ReplayResult {
	res := ReplayResult{Costs: map[uuid.UUID]decimal.Decimal{}, Remaining: map[uuid.UUID]decimal.Decimal{}}
	remaining := make([]decimal.Decimal, len(layers))
	for i, l := range layers {
		remaining[i] = l.ReceivedQty
	}
	for _, is := range issues {
		need := is.Qty
		var cons []Consumption
		take := func(eligible func(ReplayLayer) bool) {
			for i, l := range layers {
				if !need.IsPositive() {
					return
				}
				if !eligible(l) || !remaining[i].IsPositive() {
					continue
				}
				q := decimal.Min(remaining[i], need)
				cons = append(cons, Consumption{LayerID: l.ID, Qty: q, UnitCost: l.UnitCost})
				remaining[i] = remaining[i].Sub(q)
				need = need.Sub(q)
			}
		}
		take(func(l ReplayLayer) bool { return !l.BusinessDate.After(is.BusinessDate) })
		take(func(l ReplayLayer) bool { return l.BusinessDate.After(is.BusinessDate) })
		if need.IsPositive() {
			res.Short = true
		}
		res.Costs[is.MovementID] = CostAmount(cons)
	}
	for i, l := range layers {
		res.Remaining[l.ID] = remaining[i]
	}
	return res
}

// RecostDiff adalah selisih HPP satu movement.
type RecostDiff struct {
	MovementID uuid.UUID
	Previous   decimal.Decimal
	Recomputed decimal.Decimal
}

// Difference = Recomputed − Previous (positif = HPP naik).
func (d RecostDiff) Difference() decimal.Decimal { return d.Recomputed.Sub(d.Previous) }

// Diffs mengembalikan movement yang HPP-nya berubah, sesuai urutan issue.
func (r ReplayResult) Diffs(issues []ReplayIssue) []RecostDiff {
	var out []RecostDiff
	for _, is := range issues {
		if c, ok := r.Costs[is.MovementID]; ok && !c.Equal(is.CurrentCost) {
			out = append(out, RecostDiff{MovementID: is.MovementID, Previous: is.CurrentCost, Recomputed: c})
		}
	}
	return out
}
