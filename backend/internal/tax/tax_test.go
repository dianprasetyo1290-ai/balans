package tax_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/tax"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func rp(n int64) money.Amount { return money.FromInt(n) }

var jan2026 = clock.NewBusinessDate(2026, time.January, 1)

// article17 adalah lapisan tarif Pasal 17 UU HPP (berlaku 2022).
var article17 = []tax.Bracket{
	{UpTo: rp(60_000_000), RatePercent: d("5")},
	{UpTo: rp(250_000_000), RatePercent: d("15")},
	{UpTo: rp(500_000_000), RatePercent: d("25")},
	{UpTo: rp(5_000_000_000), RatePercent: d("30")},
	{RatePercent: d("35")},
}

var (
	pph23Service = tax.Rate{Type: tax.PPh23, ObjectCode: "24-104-14", Name: "Jasa lain", RatePercent: d("2"),
		NoTaxIDSurchargePercent: d("100"), ValidFrom: jan2026}
	pph21NonEmployee = tax.Rate{Type: tax.PPh21, ObjectCode: "21-100-08", Name: "Bukan pegawai", Brackets: article17,
		BasePercent: d("50"), NoTaxIDSurchargePercent: d("20"), ValidFrom: jan2026}
	pph42Rent = tax.Rate{Type: tax.PPh42, ObjectCode: "28-403-01", Name: "Sewa tanah/bangunan", RatePercent: d("10"), ValidFrom: jan2026}
	finalUMKM = tax.Rate{Type: tax.FinalUMKM, ObjectCode: "UMKM", Name: "PPh Final UMKM", RatePercent: d("0.5"), ValidFrom: jan2026}
)

func TestWithhold(t *testing.T) {
	tests := []struct {
		name       string
		rate       tax.Rate
		gross      int64
		hasTaxID   bool
		wantBase   string
		wantAmount string
		wantRate   string
		surcharged bool
	}{
		// PPh 23 jasa 2% × bruto; tanpa NPWP 100% lebih tinggi (UU PPh Pasal 23 ayat 1a).
		{"pph23 dengan NPWP", pph23Service, 10_000_000, true, "10000000.00", "200000.00", "2", false},
		{"pph23 tanpa NPWP", pph23Service, 10_000_000, false, "10000000.00", "400000.00", "4", true},
		{"pph23 dibulatkan ke bawah", pph23Service, 1_234_567, true, "1234567.00", "24691.00", "2", false},
		// PPh 21 bukan pegawai tidak berkesinambungan: Pasal 17 × 50% bruto (PMK 168/2023). Contoh resmi:
		// bruto Rp10.000.000 → DPP Rp5.000.000 × 5% = Rp250.000.
		{"pph21 bukan pegawai lapis 1", pph21NonEmployee, 10_000_000, true, "5000000.00", "250000.00", "5", false},
		// Bruto Rp200.000.000 → DPP Rp100.000.000: 60 jt × 5% + 40 jt × 15% = Rp9.000.000.
		{"pph21 bukan pegawai progresif", pph21NonEmployee, 200_000_000, true, "100000000.00", "9000000.00", "9", false},
		{"pph21 tanpa NPWP 20% lebih tinggi", pph21NonEmployee, 10_000_000, false, "5000000.00", "300000.00", "6", true},
		// Lapisan teratas tanpa batas: DPP Rp6 M.
		{"pph21 lapisan teratas", pph21NonEmployee, 12_000_000_000, true, "6000000000.00", "1794000000.00", "29.9", false},
		// PPh 4(2) sewa bangunan 10% final.
		{"pph4(2) sewa", pph42Rent, 25_000_000, false, "25000000.00", "2500000.00", "10", false},
		{"bruto nol", pph23Service, 0, true, "0.00", "0.00", "2", false},
		{"bruto nol tanpa NPWP", pph23Service, 0, false, "0.00", "0.00", "4", true},
		{"bruto nol progresif", pph21NonEmployee, 0, true, "0.00", "0.00", "5", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.rate.Withhold(tax.WithholdingInput{Gross: rp(tt.gross), HasTaxID: tt.hasTaxID})
			require.NoError(t, err)
			assert.Equal(t, tt.wantBase, got.TaxableBase.String())
			assert.Equal(t, tt.wantAmount, got.Amount.String())
			assert.True(t, d(tt.wantRate).Equal(got.RatePercent), "rate %s", got.RatePercent)
			assert.Equal(t, tt.surcharged, got.Surcharged)
			assert.Equal(t, tt.rate.Type, got.Type)
			assert.Equal(t, tt.rate.ObjectCode, got.ObjectCode)
		})
	}
}

func TestWithholdRejects(t *testing.T) {
	_, err := pph23Service.Withhold(tax.WithholdingInput{Gross: rp(-1)})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
	_, err = finalUMKM.Withhold(tax.WithholdingInput{Gross: rp(1)})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
}

func TestFinalTax(t *testing.T) {
	tests := []struct {
		turnover int64
		want     string
	}{
		// PP 55/2022: 0,5% × omzet bruto. Rp100.000.000 → Rp500.000.
		{100_000_000, "500000.00"},
		{123_456_789, "617283.00"},
		{0, "0.00"},
		{-5_000_000, "0.00"},
	}
	for _, tt := range tests {
		got, err := tax.FinalTax(rp(tt.turnover), finalUMKM)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got.String(), "omzet %d", tt.turnover)
	}
	_, err := tax.FinalTax(rp(1), pph23Service)
	assert.Error(t, err)
}

func TestCheckThreshold(t *testing.T) {
	limit := tax.UMKMAnnualThreshold
	tests := []struct {
		cumulative int64
		status     tax.ThresholdStatus
		remaining  string
		used       string
	}{
		{1_000_000_000, tax.ThresholdBelow, "3800000000.00", "20.83"},
		{3_840_000_000, tax.ThresholdApproaching, "960000000.00", "80"},
		{4_800_000_000, tax.ThresholdApproaching, "0.00", "100"},
		{5_000_000_000, tax.ThresholdExceeded, "0.00", "104.17"},
	}
	for _, tt := range tests {
		got := tax.CheckThreshold(rp(tt.cumulative), limit, tax.UMKMWarningPercent)
		assert.Equal(t, tt.status, got.Status, "omzet %d", tt.cumulative)
		assert.Equal(t, tt.remaining, got.Remaining.String())
		assert.True(t, d(tt.used).Equal(got.UsedPercent), "used %s", got.UsedPercent)
	}
	zero := tax.CheckThreshold(rp(0), money.Zero, tax.UMKMWarningPercent)
	assert.Equal(t, tax.ThresholdBelow, zero.Status)
}

func TestSelectRate(t *testing.T) {
	jul := clock.NewBusinessDate(2026, time.July, 1)
	endJun := clock.NewBusinessDate(2026, time.June, 30)
	old := pph23Service
	old.ValidTo = &endJun
	newer := pph23Service
	newer.RatePercent, newer.ValidFrom = d("2.5"), jul
	other := pph42Rent
	rates := []tax.Rate{old, newer, other}

	got, err := tax.SelectRate(rates, tax.PPh23, "24-104-14", clock.NewBusinessDate(2026, time.March, 5))
	require.NoError(t, err)
	assert.True(t, d("2").Equal(got.RatePercent))

	got, err = tax.SelectRate(rates, tax.PPh23, "24-104-14", clock.NewBusinessDate(2026, time.August, 1))
	require.NoError(t, err)
	assert.True(t, d("2.5").Equal(got.RatePercent))

	// Versi tumpang tindih: ValidFrom terakhir menang.
	overlap := pph23Service
	overlap.RatePercent, overlap.ValidFrom = d("3"), clock.NewBusinessDate(2026, time.March, 1)
	got, err = tax.SelectRate([]tax.Rate{pph23Service, overlap}, tax.PPh23, "24-104-14", clock.NewBusinessDate(2026, time.April, 1))
	require.NoError(t, err)
	assert.True(t, d("3").Equal(got.RatePercent))

	_, err = tax.SelectRate(rates, tax.PPh23, "24-104-14", clock.NewBusinessDate(2025, time.December, 31))
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
	_, err = tax.SelectRate(rates, tax.PPh21, "24-104-14", jul)
	assert.Error(t, err)
}

func TestRateValidate(t *testing.T) {
	require.NoError(t, pph23Service.Validate())
	require.NoError(t, pph21NonEmployee.Validate())
	require.NoError(t, finalUMKM.Validate())

	before := clock.NewBusinessDate(2025, time.December, 1)
	tests := []struct {
		name  string
		mod   func(r *tax.Rate)
		field string
	}{
		{"jenis", func(r *tax.Rate) { r.Type = "ppn" }, "taxType"},
		{"objek kosong", func(r *tax.Rate) { r.ObjectCode = " " }, "objectCode"},
		{"objek panjang", func(r *tax.Rate) { r.ObjectCode = "1234567890123456789012345678901" }, "objectCode"},
		{"nama", func(r *tax.Rate) { r.Name = "" }, "name"},
		{"tarif nol", func(r *tax.Rate) { r.RatePercent = decimal.Zero }, "ratePercent"},
		{"tarif > 100", func(r *tax.Rate) { r.RatePercent = d("101") }, "ratePercent"},
		{"dpp", func(r *tax.Rate) { r.BasePercent = d("-1") }, "basePercent"},
		{"kenaikan", func(r *tax.Rate) { r.NoTaxIDSurchargePercent = d("150") }, "noTaxIdSurchargePercent"},
		{"tanggal kosong", func(r *tax.Rate) { r.ValidFrom = clock.BusinessDate{} }, "validFrom"},
		{"tanggal akhir", func(r *tax.Rate) { r.ValidTo = &before }, "validTo"},
		{"lapisan tarif", func(r *tax.Rate) { r.Brackets = []tax.Bracket{{RatePercent: decimal.Zero}} }, "brackets"},
		{"lapisan tanpa batas di tengah", func(r *tax.Rate) {
			r.Brackets = []tax.Bracket{{RatePercent: d("5")}, {UpTo: rp(10), RatePercent: d("10")}}
		}, "brackets"},
		{"lapisan menurun", func(r *tax.Rate) {
			r.Brackets = []tax.Bracket{{UpTo: rp(10), RatePercent: d("5")}, {UpTo: rp(5), RatePercent: d("10")}}
		}, "brackets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := pph23Service
			tt.mod(&r)
			err := r.Validate()
			var e *apperr.Error
			require.ErrorAs(t, err, &e)
			assert.Contains(t, e.Fields, tt.field)
		})
	}
}

func TestTypesAndPeriod(t *testing.T) {
	assert.Len(t, tax.Types(), 4)
	for _, ty := range tax.Types() {
		assert.True(t, ty.Valid())
		assert.NotEmpty(t, ty.Label())
	}
	assert.False(t, tax.Type("x").Valid())
	assert.True(t, tax.PPh23.IsWithholding())
	assert.False(t, tax.FinalUMKM.IsWithholding())

	p := tax.PeriodOf(clock.NewBusinessDate(2026, time.February, 14))
	require.NoError(t, p.Validate())
	assert.Equal(t, "2026-02", p.String())
	assert.Equal(t, "2026-02-01", p.FirstDay().String())
	assert.Equal(t, "2026-02-28", p.LastDay().String())
	assert.Equal(t, "2026-12-31", tax.Period{Year: 2026, Month: time.December}.LastDay().String())
	assert.True(t, p.Before(tax.Period{Year: 2026, Month: time.March}))
	assert.True(t, p.Before(tax.Period{Year: 2027, Month: time.January}))
	assert.False(t, p.Before(p))
	assert.Error(t, tax.Period{Year: 2026, Month: 13}.Validate())
}
