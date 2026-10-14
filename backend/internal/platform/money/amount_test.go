package money_test

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/money"
)

func TestRounding(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		round  string
		rupiah string
		cost   string
	}{
		{"half up positif", "1250.125", "1250.13", "1250.00", "1250.1250"},
		{"setengah rupiah naik", "999.50", "999.50", "1000.00", "999.5000"},
		{"di bawah setengah", "999.49", "999.49", "999.00", "999.4900"},
		{"negatif simetris", "-1250.125", "-1250.13", "-1250.00", "-1250.1250"},
		{"negatif setengah rupiah", "-0.5", "-0.50", "-1.00", "-0.5000"},
		{"presisi cost layer", "1234.56785", "1234.57", "1235.00", "1234.5679"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := money.MustParse(tt.in)
			assert.Equal(t, tt.round, a.Round().String())
			assert.Equal(t, tt.rupiah, a.RoundRupiah().String())
			assert.Equal(t, tt.cost, a.RoundUnitCost().Decimal().StringFixed(money.UnitCostScale))
		})
	}
}

func TestArithmetic(t *testing.T) {
	a := money.FromInt(1000)
	b := money.MustParse("250.50")

	assert.Equal(t, "1250.50", a.Add(b).String())
	assert.Equal(t, "749.50", a.Sub(b).String())
	assert.Equal(t, "-1000.00", a.Neg().String())
	assert.Equal(t, "3000.00", a.Mul(decimal.NewFromInt(3)).String())
	assert.Equal(t, "110.00", a.Percent(decimal.NewFromInt(11)).String())
	assert.Equal(t, "2501.00", money.Sum(a, a, b, b).String())
	assert.True(t, money.Zero.IsZero())
	assert.True(t, a.IsPositive())
	assert.True(t, a.Neg().IsNegative())
	assert.Equal(t, 1, a.Cmp(b))
	assert.True(t, b.Equal(money.MustParse("250.5")))
	assert.True(t, money.FromDecimal(decimal.NewFromInt(5)).Equal(money.FromInt(5)))
}

func TestParseRejectsInvalid(t *testing.T) {
	_, err := money.Parse("1.250.000")
	assert.Error(t, err)
	assert.Panics(t, func() { money.MustParse("abc") })
}

func TestJSONUsesDecimalString(t *testing.T) {
	type payload struct {
		TotalAmount money.Amount `json:"totalAmount"`
	}
	b, err := json.Marshal(payload{money.FromInt(1250000)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"totalAmount":"1250000.00"}`, string(b))

	var got payload
	require.NoError(t, json.Unmarshal([]byte(`{"totalAmount":"99.9"}`), &got))
	assert.Equal(t, "99.90", got.TotalAmount.String())

	assert.Error(t, json.Unmarshal([]byte(`{"totalAmount":99.9}`), &got), "angka JSON ditolak agar tidak lewat float")
	assert.Error(t, json.Unmarshal([]byte(`{"totalAmount":"x"}`), &got))
}

func TestSQLRoundTrip(t *testing.T) {
	a := money.MustParse("1250000.25")
	v, err := a.Value()
	require.NoError(t, err)

	var scanned money.Amount
	require.NoError(t, scanned.Scan(v))
	assert.True(t, a.Equal(scanned))
	assert.Error(t, scanned.Scan(struct{}{}))
}

func TestAllocateKeepsTotal(t *testing.T) {
	tests := []struct {
		name    string
		total   string
		weights []int64
		want    []string
	}{
		{"bagi tiga", "100.00", []int64{1, 1, 1}, []string{"33.34", "33.33", "33.33"}},
		{"bobot qty", "1000.00", []int64{3, 7}, []string{"300.00", "700.00"}},
		{"sisa ke bobot terbesar", "10.00", []int64{1, 2, 3}, []string{"1.67", "3.33", "5.00"}},
		{"bobot nol", "50.00", []int64{0, 5}, []string{"0.00", "50.00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weights := make([]decimal.Decimal, len(tt.weights))
			for i, w := range tt.weights {
				weights[i] = decimal.NewFromInt(w)
			}
			parts, err := money.Allocate(money.MustParse(tt.total), weights)
			require.NoError(t, err)

			got := make([]string, len(parts))
			for i, p := range parts {
				got[i] = p.String()
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, money.MustParse(tt.total).String(), money.Sum(parts...).String())
		})
	}
}

func TestAllocateRejectsInvalidWeights(t *testing.T) {
	_, err := money.Allocate(money.FromInt(10), []decimal.Decimal{decimal.Zero})
	assert.ErrorIs(t, err, money.ErrNoWeights)

	_, err = money.Allocate(money.FromInt(10), []decimal.Decimal{decimal.NewFromInt(-1), decimal.NewFromInt(2)})
	assert.Error(t, err)
}
