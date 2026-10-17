package tabular_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/tabular"
)

var opts = []tabular.Option{{Value: "finished_good", Label: "Produk Jadi"}, {Value: "packaging", Label: "Packaging"}}

func TestParser(t *testing.T) {
	r := tabular.NewRow(2).Set("name", "Meal").Set("yes", "ya").Set("no", "Tidak").Set("badBool", "mungkin").
		Set("int", "24").Set("intFloat", "24.0").Set("badInt", "2,5").Set("dec", "1.250").Set("badDec", "1.000,5").
		Set("date", "2026-10-17").Set("serial", "46312").Set("badDate", "17/10/2026").
		Set("choice", "produk jadi").Set("value", "packaging").Set("badChoice", "Lain")
	p := tabular.Parse(r)
	assert.Equal(t, "Meal", p.Text("name"))
	assert.Nil(t, p.OptionalText("missing"))
	assert.Equal(t, "Meal", *p.OptionalText("name"))
	assert.True(t, p.Bool("yes", false))
	assert.False(t, p.Bool("no", true))
	assert.True(t, p.Bool("missing", true))
	assert.False(t, p.Bool("badBool", false))
	assert.Equal(t, 24, *p.Int("int"))
	assert.Equal(t, 24, *p.Int("intFloat"))
	assert.Nil(t, p.Int("missing"))
	assert.Nil(t, p.Int("badInt"))
	assert.True(t, decimal.RequireFromString("1.25").Equal(*p.Decimal("dec")))
	assert.Nil(t, p.Decimal("missing"))
	assert.Nil(t, p.Decimal("badDec"))
	assert.Equal(t, clock.NewBusinessDate(2026, 10, 17), *p.Date("date"))
	assert.Equal(t, clock.NewBusinessDate(2026, 10, 17), *p.Date("serial"), "nomor seri tanggal Excel")
	assert.Nil(t, p.Date("missing"))
	assert.Nil(t, p.Date("badDate"))
	assert.Equal(t, "finished_good", p.Choice("choice", opts))
	assert.Equal(t, "packaging", p.Choice("value", opts))
	assert.Empty(t, p.Choice("missing", opts))
	assert.Empty(t, p.Choice("badChoice", opts))
	p.Fail("badChoice", "pesan kedua tidak menimpa")

	err := p.Err()
	require.Error(t, err)
	fields := apperr.As(err).Fields
	assert.Equal(t, []string{"badBool", "badChoice", "badDate", "badDec", "badInt"}, keys(fields))
	assert.Contains(t, fields["badChoice"], "Produk Jadi, Packaging")

	assert.NoError(t, tabular.Parse(tabular.NewRow(3)).Err())
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, k := range []string{"badBool", "badChoice", "badDate", "badDec", "badInt"} {
		if _, ok := m[k]; ok {
			out = append(out, k)
		}
	}
	if len(out) != len(m) {
		return nil
	}
	return out
}

func TestFormatting(t *testing.T) {
	n := 5
	d := decimal.RequireFromString("2.5")
	date := clock.NewBusinessDate(2026, 1, 2)
	assert.Equal(t, "5", tabular.FormatInt(&n))
	assert.Empty(t, tabular.FormatInt(nil))
	assert.Equal(t, "2.5", tabular.FormatDecimal(&d))
	assert.Empty(t, tabular.FormatDecimal(nil))
	assert.Equal(t, "2026-01-02", tabular.FormatDate(&date))
	assert.Empty(t, tabular.FormatDate(nil))
	assert.Equal(t, tabular.Yes, tabular.BoolLabel(true))
	assert.Equal(t, tabular.No, tabular.BoolLabel(false))
	assert.Equal(t, "Packaging", tabular.Label(opts, "packaging"))
	assert.Equal(t, "lain", tabular.Label(opts, "lain"))
}

func TestColumnHints(t *testing.T) {
	tests := []struct {
		col  tabular.Column
		want string
	}{
		{tabular.Column{Kind: tabular.KindText}, "Teks"},
		{tabular.Column{Kind: tabular.KindInteger}, "Angka bulat tanpa titik ribuan"},
		{tabular.Column{Kind: tabular.KindDecimal}, "Angka; desimal memakai titik, tanpa titik ribuan"},
		{tabular.Column{Kind: tabular.KindDate}, "Tanggal YYYY-MM-DD"},
		{tabular.Column{Kind: tabular.KindBool}, "Pilih: Ya, Tidak"},
		{tabular.Column{Kind: tabular.KindChoice, Options: opts}, "Pilih: Produk Jadi, Packaging"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.col.FormatHint())
	}
	assert.Empty(t, tabular.Column{Kind: tabular.KindText}.OptionLabels())
}
