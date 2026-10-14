package money

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrNoWeights dikembalikan jika alokasi tidak punya bobot positif.
var ErrNoWeights = errors.New("allocation needs at least one positive weight")

// Allocate membagi total (2 desimal) ke beberapa bagian sesuai bobot, sehingga jumlah bagian
// selalu sama persis dengan total. Sisa pembulatan diberikan ke bagian dengan bobot terbesar,
// supaya selisih pembulatan tidak hilang (Technical Guideline 5.8.6).
func Allocate(total Amount, weights []decimal.Decimal) ([]Amount, error) {
	weightSum := decimal.Zero
	largest := -1
	for i, w := range weights {
		if w.IsNegative() {
			return nil, errors.New("allocation weight must not be negative")
		}
		weightSum = weightSum.Add(w)
		if largest == -1 || w.GreaterThan(weights[largest]) {
			largest = i
		}
	}
	if !weightSum.IsPositive() {
		return nil, ErrNoWeights
	}

	rounded := total.Round()
	parts := make([]Amount, len(weights))
	allocated := Zero
	for i, w := range weights {
		parts[i] = Amount{d: rounded.d.Mul(w).Div(weightSum)}.Round()
		allocated = allocated.Add(parts[i])
	}
	parts[largest] = parts[largest].Add(rounded.Sub(allocated))
	return parts, nil
}
