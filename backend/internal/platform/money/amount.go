// Package money adalah satu-satunya tempat aturan nilai uang dan pembulatan (Technical Guideline 5.8.6).
package money

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// Presisi penyimpanan per jenis nilai.
const (
	AmountScale   int32 = 2 // harga, subtotal, invoice, pembayaran, jurnal
	UnitCostScale int32 = 4 // HPP per unit, cost layer
	RateScale     int32 = 4 // persentase diskon dan tarif
)

// Amount adalah nilai rupiah. Nilai nol siap dipakai.
type Amount struct {
	d decimal.Decimal
}

// Zero adalah Rp0.
var Zero = Amount{}

// FromDecimal membuat Amount dari decimal.
func FromDecimal(d decimal.Decimal) Amount { return Amount{d: d} }

// FromInt membuat Amount dari rupiah penuh.
func FromInt(rupiah int64) Amount { return Amount{d: decimal.NewFromInt(rupiah)} }

// Parse membaca string desimal seperti "1250000.00".
func Parse(s string) (Amount, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Amount{}, fmt.Errorf("parse amount %q: %w", s, err)
	}
	return Amount{d: d}, nil
}

// MustParse dipakai untuk konstanta di test.
func MustParse(s string) Amount {
	a, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return a
}

func (a Amount) Decimal() decimal.Decimal { return a.d }
func (a Amount) Add(b Amount) Amount      { return Amount{d: a.d.Add(b.d)} }
func (a Amount) Sub(b Amount) Amount      { return Amount{d: a.d.Sub(b.d)} }
func (a Amount) Neg() Amount              { return Amount{d: a.d.Neg()} }
func (a Amount) IsZero() bool             { return a.d.IsZero() }
func (a Amount) IsNegative() bool         { return a.d.IsNegative() }
func (a Amount) IsPositive() bool         { return a.d.IsPositive() }
func (a Amount) Cmp(b Amount) int         { return a.d.Cmp(b.d) }
func (a Amount) Equal(b Amount) bool      { return a.d.Equal(b.d) }

// Mul mengalikan dengan kuantitas atau faktor tanpa membulatkan.
func (a Amount) Mul(factor decimal.Decimal) Amount { return Amount{d: a.d.Mul(factor)} }

// Percent menghitung persentase dari nilai (rate dalam persen, mis. 11 untuk 11%) tanpa membulatkan.
func (a Amount) Percent(rate decimal.Decimal) Amount {
	return Amount{d: a.d.Mul(rate).Div(decimal.NewFromInt(100))}
}

// Round membulatkan half-up ke 2 desimal untuk disimpan.
func (a Amount) Round() Amount { return Amount{d: roundHalfUp(a.d, AmountScale)} }

// RoundRupiah membulatkan half-up ke rupiah penuh, dipakai di total dokumen dan pajak.
func (a Amount) RoundRupiah() Amount { return Amount{d: roundHalfUp(a.d, 0)} }

// RoundUnitCost membulatkan half-up ke 4 desimal untuk HPP per unit.
func (a Amount) RoundUnitCost() Amount { return Amount{d: roundHalfUp(a.d, UnitCostScale)} }

// String mengembalikan string desimal 2 digit, format kontrak API.
func (a Amount) String() string { return a.d.StringFixed(AmountScale) }

func (a Amount) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

func (a *Amount) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("amount must be a decimal string: %w", err)
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

func (a Amount) Value() (driver.Value, error) { return a.d.String(), nil }

func (a *Amount) Scan(src any) error {
	var d decimal.Decimal
	if err := d.Scan(src); err != nil {
		return fmt.Errorf("scan amount: %w", err)
	}
	a.d = d
	return nil
}

// Sum menjumlahkan beberapa nilai.
func Sum(amounts ...Amount) Amount {
	total := Zero
	for _, a := range amounts {
		total = total.Add(a)
	}
	return total
}

// roundHalfUp membulatkan menjauhi nol pada 0,5 untuk nilai positif dan negatif secara simetris.
func roundHalfUp(d decimal.Decimal, places int32) decimal.Decimal {
	if d.IsNegative() {
		return d.Neg().Round(places).Neg()
	}
	return d.Round(places)
}
