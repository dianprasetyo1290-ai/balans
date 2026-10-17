package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

func testClass() AssetClass {
	return AssetClass{ID: uuid.New(), Code: "office_equipment", Name: "Peralatan Kantor & Komputer", UsefulLifeMonths: 48, IsActive: true}
}

func fieldsOf(err error) map[string]string { return apperr.As(err).Fields }

func TestNewAssetClass(t *testing.T) {
	asset, accum := uuid.New(), uuid.New()
	c, err := NewAssetClass(AssetClassInput{Code: "vehicle", Name: " Kendaraan ", UsefulLifeMonths: 96, AssetAccountID: asset, AccumulatedAccountID: accum, IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "Kendaraan", c.Name)

	_, err = NewAssetClass(AssetClassInput{Code: "X", Name: "", UsefulLifeMonths: 0, AssetAccountID: asset, AccumulatedAccountID: asset})
	f := fieldsOf(err)
	for _, k := range []string{"code", "name", "usefulLifeMonths", "accumulatedAccountId"} {
		assert.Contains(t, f, k)
	}
	_, err = NewAssetClass(AssetClassInput{Code: "ok_code", Name: "A", UsefulLifeMonths: 12})
	assert.Contains(t, fieldsOf(err), "assetAccountId")
}

func purchaseInput(class AssetClass) FixedAssetInput {
	bank := uuid.New()
	return FixedAssetInput{
		Code: "ast-lp-001", Name: "Laptop", ClassID: class.ID, DivisionID: uuid.New(), Source: AssetPurchase,
		AcquisitionDate: clock.NewBusinessDate(2027, time.January, 20), Cost: money.FromInt(12_000_000), Salvage: money.Zero,
		PaymentAccountID: &bank,
	}
}

func TestNewFixedAssetPurchase(t *testing.T) {
	class := testClass()
	a, err := NewFixedAsset(purchaseInput(class), class, nil)
	require.NoError(t, err)
	assert.Equal(t, "AST-LP-001", a.Code)
	assert.Equal(t, AssetDraft, a.Status)
	assert.Equal(t, 48, a.UsefulLife, "umur kelas dipakai bila kosong")
	assert.Equal(t, clock.NewBusinessDate(2027, time.January, 1), a.DepreciationStart, "mulai bulan perolehan")
	assert.Equal(t, "250000.00", a.MonthlyDepreciation().String())

	in := purchaseInput(class)
	in.PaymentAccountID, in.OpeningAccumulated = nil, money.FromInt(1)
	in.Salvage = money.FromInt(13_000_000)
	_, err = NewFixedAsset(in, class, nil)
	f := fieldsOf(err)
	assert.Contains(t, f, "paymentAccountId")
	assert.Contains(t, f, "openingAccumulated")
	assert.Contains(t, f, "salvage")

	bad := FixedAssetInput{Code: "!", Source: "x", UsefulLife: 700}
	_, err = NewFixedAsset(bad, AssetClass{}, nil)
	for _, k := range []string{"code", "name", "classId", "divisionId", "acquisitionDate", "cost", "usefulLife", "source"} {
		assert.Contains(t, fieldsOf(err), k)
	}
	inactive := class
	inactive.IsActive = false
	_, err = NewFixedAsset(purchaseInput(inactive), inactive, nil)
	assert.Contains(t, fieldsOf(err), "classId")
}

func TestNewFixedAssetOpening(t *testing.T) {
	class := testClass()
	cutover := clock.NewBusinessDate(2026, time.December, 31)
	in := FixedAssetInput{
		Code: "AST-VM-001", Name: "Mesin Vending A", ClassID: class.ID, DivisionID: uuid.New(), Source: AssetOpening,
		AcquisitionDate: clock.NewBusinessDate(2025, time.July, 1), Cost: money.FromInt(45_000_000), UsefulLife: 48,
		OpeningAccumulated: money.FromInt(16_875_000),
	}
	_, err := NewFixedAsset(in, class, nil)
	assert.Contains(t, fieldsOf(err), "source", "aset saldo awal butuh tanggal cut-over")

	a, err := NewFixedAsset(in, class, &cutover)
	require.NoError(t, err)
	assert.Equal(t, clock.NewBusinessDate(2027, time.January, 1), a.DepreciationStart)
	assert.Equal(t, 18, a.OpeningMonths, "Jul 2025 s.d. Des 2026")
	assert.Equal(t, "16875000.00", a.Accumulated.String())
	assert.Nil(t, a.PaymentAccountID)

	in.AcquisitionDate = clock.NewBusinessDate(2027, time.February, 1)
	in.OpeningAccumulated = money.FromInt(50_000_000)
	_, err = NewFixedAsset(in, class, &cutover)
	assert.Contains(t, fieldsOf(err), "acquisitionDate")
	assert.Contains(t, fieldsOf(err), "openingAccumulated")
}

func TestDepreciationStraightLine(t *testing.T) {
	class := testClass()
	in := purchaseInput(class)
	in.Cost, in.UsefulLife = money.MustParse("1000.00"), 3
	a, err := NewFixedAsset(in, class, nil)
	require.NoError(t, err)
	jan := PeriodKey{Year: 2027, Month: time.January}
	assert.True(t, a.DepreciationFor(jan).IsZero(), "draft belum disusutkan")
	require.NoError(t, a.Activate())
	assert.Error(t, a.Activate())

	assert.True(t, a.DepreciationFor(PeriodKey{Year: 2026, Month: time.December}).IsZero(), "sebelum mulai")
	var total money.Amount
	for i, want := range []string{"333.33", "333.34", "333.33"} {
		k := PeriodKey{Year: 2027, Month: time.Month(i + 1)}
		got := a.DepreciationFor(k)
		assert.Equal(t, want, got.String(), k.Label())
		a.RecordDepreciation(got)
		total = total.Add(got)
	}
	assert.Equal(t, "1000.00", total.String(), "akumulasi tepat sama dengan nilai yang disusutkan")
	assert.True(t, a.DepreciationFor(PeriodKey{Year: 2027, Month: time.April}).IsZero(), "habis")
	assert.Equal(t, "0.00", a.BookValue().String())

	// Job terlambat: sisa umur habis → seluruh sisa disusutkan.
	b, err := NewFixedAsset(in, class, nil)
	require.NoError(t, err)
	require.NoError(t, b.Activate())
	assert.Equal(t, "1000.00", b.DepreciationFor(PeriodKey{Year: 2027, Month: time.June}).String())
}

func TestDepreciationOpeningAsset(t *testing.T) {
	class := testClass()
	cutover := clock.NewBusinessDate(2026, time.December, 31)
	a, err := NewFixedAsset(FixedAssetInput{
		Code: "AST-VM-002", Name: "Mesin", ClassID: class.ID, DivisionID: uuid.New(), Source: AssetOpening,
		AcquisitionDate: clock.NewBusinessDate(2025, time.July, 1), Cost: money.FromInt(48_000), UsefulLife: 48,
		OpeningAccumulated: money.FromInt(18_000),
	}, class, &cutover)
	require.NoError(t, err)
	require.NoError(t, a.Activate())
	// Sisa 30.000 dibagi sisa umur 30 bulan.
	assert.Equal(t, "1000.00", a.DepreciationFor(PeriodKey{Year: 2027, Month: time.January}).String())
}

func TestDisposeAndActions(t *testing.T) {
	class := testClass()
	a, err := NewFixedAsset(purchaseInput(class), class, nil)
	require.NoError(t, err)
	has := func(string) bool { return true }
	assert.ElementsMatch(t, []AssetAction{AssetActionEdit, AssetActionDelete, AssetActionActivate}, a.AllowedActions(has))
	_, err = a.Dispose(clock.NewBusinessDate(2027, time.June, 1), money.Zero, nil, "rusak")
	assert.Error(t, err, "draft tidak bisa dilepas")

	require.NoError(t, a.Activate())
	assert.Equal(t, []AssetAction{AssetActionDispose}, a.AllowedActions(has))
	a.RecordDepreciation(money.FromInt(1_250_000))
	assert.Error(t, a.Update(purchaseInput(class), class, nil), "aset aktif tidak bisa diubah")

	_, err = a.Dispose(clock.BusinessDate{}, money.FromInt(-1), nil, "")
	f := fieldsOf(err)
	for _, k := range []string{"disposalDate", "proceeds", "reason"} {
		assert.Contains(t, f, k)
	}
	_, err = a.Dispose(clock.NewBusinessDate(2027, time.June, 1), money.FromInt(1), nil, "dijual")
	assert.Contains(t, fieldsOf(err), "disposalAccountId")

	bank := uuid.New()
	b := a
	d, err := b.Dispose(clock.NewBusinessDate(2027, time.June, 1), money.FromInt(11_000_000), &bank, "Dijual")
	require.NoError(t, err)
	assert.Equal(t, "0.00", d.Loss.String())
	assert.Equal(t, "250000.00", d.Gain.String(), "hasil 11 jt − nilai buku 10,75 jt")
	assert.Equal(t, AssetDisposed, b.Status)
	assert.Empty(t, b.AllowedActions(has))

	c := a
	d, err = c.Dispose(clock.NewBusinessDate(2027, time.June, 1), money.Zero, &bank, "Hilang")
	require.NoError(t, err)
	assert.Equal(t, "10750000.00", d.Loss.String())
	assert.Nil(t, c.DisposalAccountID, "tanpa hasil tidak perlu akun penerimaan")
	amounts := d.Amounts()
	assert.Equal(t, "1250000.00", amounts[AmountDisposalAccumulated].String())
	assert.Equal(t, "12000000.00", amounts[AmountDisposalCost].String())

	deleted := a
	now := time.Now()
	deleted.DeletedAt = &now
	assert.Empty(t, deleted.AllowedActions(has))
}
