package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

func acc(code, name string, parent *uuid.UUID) Account {
	g := Group(code[0] - '0')
	return Account{ID: uuid.New(), Code: code, Name: name, Group: g, NormalBalance: g.DefaultNormalBalance(), ParentID: parent, IsPostable: true}
}

func mv(a Account, before, during int64) Movement {
	return Movement{Account: a, Before: money.FromInt(before), During: money.FromInt(during)}
}

// ledger kecil: modal 10 jt masuk bank tahun lalu (laba tahun lalu 1 jt belum ditutup), tahun ini penjualan kredit
// 5 jt (HPP 3 jt), beban 0,5 jt dari bank, piutang dibayar 2 jt, beli aset 1 jt, penyusutan 0,1 jt.
func sampleLedger() ([]Movement, ParentLookup) {
	root := uuid.New()
	cashParent := Account{ID: uuid.New(), Code: "1100", Name: "Kas & Bank", Group: GroupAsset, ParentID: &root}
	bank := acc("1111", "Bank", &cashParent.ID)
	ar := acc("1201", "Piutang Usaha", nil)
	inv := acc("1311", "Persediaan FG", nil)
	fa := acc("1502", "Peralatan", nil)
	dep := acc("1592", "Akumulasi Penyusutan", nil)
	dep.NormalBalance = NormalCredit
	capital := acc("3101", "Modal", nil)
	sales := acc("4101", "Penjualan", nil)
	cogs := acc("5101", "HPP", nil)
	exp := acc("6306", "Beban Umum", nil)
	depExp := acc("6305", "Beban Penyusutan", nil)
	ap := acc("2101", "Utang Usaha", nil)
	parents := map[uuid.UUID]Account{cashParent.ID: cashParent}
	lookup := func(a Account) (string, string) {
		if a.ParentID == nil {
			return "", ""
		}
		p := parents[*a.ParentID]
		return p.Code, p.Name
	}
	return []Movement{
		mv(bank, 10_000_000+1_000_000, -500_000+2_000_000-1_000_000),
		mv(ar, 0, 5_000_000-2_000_000),
		mv(inv, 4_000_000, -3_000_000),
		mv(fa, 0, 1_000_000),
		mv(dep, 0, -100_000),
		mv(ap, -4_000_000, 0),
		mv(capital, -10_000_000, 0),
		mv(sales, -1_000_000, -5_000_000),
		mv(cogs, 0, 3_000_000),
		mv(exp, 0, 500_000),
		mv(depExp, 0, 100_000),
	}, lookup
}

func rowAmount(t *testing.T, st Statement, label string) string {
	t.Helper()
	for _, r := range st.Rows {
		if r.Label == label {
			return r.Amount.String()
		}
	}
	require.Failf(t, "baris tidak ada", label)
	return ""
}

func TestBalanceSheetBalancesWithUnclosedProfit(t *testing.T) {
	list, parents := sampleLedger()
	st := BalanceSheet(list, parents)
	assert.True(t, st.Balanced)
	assert.Equal(t, "16400000.00", rowAmount(t, st, "TOTAL ASET"))
	assert.Equal(t, "16400000.00", rowAmount(t, st, "TOTAL LIABILITAS & EKUITAS"))
	assert.Equal(t, "1400000.00", rowAmount(t, st, "Laba (Rugi) Tahun Berjalan (dihitung)"))
	assert.Equal(t, "1000000.00", rowAmount(t, st, "Laba (Rugi) Tahun Lalu Belum Ditutup"))
	assert.Equal(t, "11500000.00", rowAmount(t, st, "Jumlah Kas & Bank"), "subtotal per akun induk")
	assert.Equal(t, "-100000.00", rowAmount(t, st, "Akumulasi Penyusutan"), "akun kontra aset bernilai negatif")
	assert.Equal(t, StatementBalanceSheet, st.Kind)
	assert.Equal(t, "Neraca", st.Kind.Label())
}

func TestIncomeStatementSummary(t *testing.T) {
	list, parents := sampleLedger()
	st := IncomeStatement(list, parents)
	assert.Equal(t, "5000000.00", st.Summary.Revenue.String())
	assert.Equal(t, "2000000.00", st.Summary.GrossProfit.String())
	assert.Equal(t, "1400000.00", st.Summary.NetProfit.String())
	assert.Equal(t, "1400000.00", rowAmount(t, st, "LABA (RUGI) BERSIH"))
	assert.Equal(t, "40", MarginPercent(st.Summary.GrossProfit, st.Summary.Revenue).String())
	assert.True(t, MarginPercent(money.Zero, money.Zero).IsZero())
}

func TestCashFlowIndirectReconcilesToCash(t *testing.T) {
	list, _ := sampleLedger()
	st := CashFlow(list)
	assert.True(t, st.Balanced)
	assert.Equal(t, "500000.00", rowAmount(t, st, "KENAIKAN (PENURUNAN) KAS & BANK"))
	assert.Equal(t, "11000000.00", rowAmount(t, st, "Kas & Bank Awal Periode"))
	assert.Equal(t, "11500000.00", rowAmount(t, st, "KAS & BANK AKHIR PERIODE"))
	assert.Equal(t, "100000.00", rowAmount(t, st, "Penyusutan Aset Tetap"))
	// Laba 1,4 jt + penyusutan 0,1 jt − kenaikan piutang 3 jt + penurunan persediaan 3 jt = 1,5 jt.
	assert.Equal(t, "1500000.00", rowAmount(t, st, "Arus Kas Bersih dari Aktivitas Operasi"))
	assert.Equal(t, "-1000000.00", rowAmount(t, st, "Arus Kas Bersih dari Aktivitas Investasi"))
	assert.Equal(t, "0.00", rowAmount(t, st, "Arus Kas Bersih dari Aktivitas Pendanaan"))
}

func TestStatementKindAndRange(t *testing.T) {
	assert.True(t, StatementCashFlow.Valid())
	assert.False(t, StatementKind("x").Valid())
	assert.Equal(t, "Laba Rugi", StatementIncomeStatement.Label())
	assert.Equal(t, "Arus Kas", StatementCashFlow.Label())
	assert.Equal(t, "x", StatementKind("x").Label())
	d := clock.NewBusinessDate
	require.NoError(t, CheckRange(d(2026, 1, 1), d(2026, 12, 31)))
	require.Error(t, CheckRange(clock.BusinessDate{}, d(2026, 1, 1)))
	require.Error(t, CheckRange(d(2026, 2, 1), d(2026, 1, 1)))
	require.Error(t, CheckRange(d(2020, 1, 1), d(2026, 1, 1)))
	assert.True(t, IsCashAccount("1112"))
	assert.False(t, IsCashAccount("1201"))
}

func TestDimensionProfitLossColumnsSumToTotal(t *testing.T) {
	sales, cogs, exp := acc("4101", "Penjualan", nil), acc("5101", "HPP", nil), acc("6201", "Gaji", nil)
	mp, mt := uuid.New(), uuid.New()
	rows := []DimensionAmount{
		{Account: sales, DimensionID: mp, Net: money.FromInt(-1000)},
		{Account: cogs, DimensionID: mp, Net: money.FromInt(600)},
		{Account: sales, DimensionID: mt, Net: money.FromInt(-500)},
		{Account: exp, DimensionID: uuid.Nil, Net: money.FromInt(200)},
		{Account: cogs, DimensionID: mt, Net: money.Zero},
	}
	r := BuildDimensionProfitLoss(rows, []uuid.UUID{mt, mp})
	require.Len(t, r.Columns, 3)
	assert.Equal(t, mt, r.Columns[0].ID, "urutan mengikuti order")
	assert.Equal(t, uuid.Nil, r.Columns[2].ID, "tanpa dimensi paling akhir")
	total := money.Zero
	for _, c := range r.Columns {
		total = total.Add(c.Summary.NetProfit)
	}
	assert.Equal(t, r.Total.NetProfit.String(), total.String())
	assert.Equal(t, "700.00", r.Total.NetProfit.String())
	require.Len(t, r.Accounts, 3)
	assert.Equal(t, "4101", r.Accounts[0].Account.Code)
	assert.Equal(t, []string{"500.00", "1000.00", "0.00"}, []string{r.Accounts[0].Amounts[0].String(), r.Accounts[0].Amounts[1].String(), r.Accounts[0].Amounts[2].String()})
	assert.True(t, KindCampaign.Valid())
	assert.False(t, DimensionKind("x").Valid())
}

func TestTaxSummaryMonitorsTurnover(t *testing.T) {
	m := func(month time.Month) clock.BusinessDate { return clock.NewBusinessDate(2026, month, 1) }
	rows := []MonthlyAccount{
		{Month: m(1), Code: AccountSales, Credit: money.FromInt(2_000_000_000), Debit: money.Zero},
		{Month: m(2), Code: AccountSales, Credit: money.FromInt(2_000_000_000), Debit: money.FromInt(100_000)},
		{Month: m(2), Code: AccountShipping, Credit: money.FromInt(100_000), Debit: money.Zero},
		{Month: m(2), Code: AccountPPh23, Credit: money.FromInt(20_000), Debit: money.Zero},
		{Month: m(3), Code: AccountPPh23, Credit: money.Zero, Debit: money.FromInt(20_000)},
		{Month: m(3), Code: AccountPPh21, Credit: money.FromInt(5_000), Debit: money.Zero},
		{Month: m(3), Code: AccountPPh42, Credit: money.FromInt(7_000), Debit: money.Zero},
		{Month: m(3), Code: AccountPPhFinal, Credit: money.FromInt(9_000), Debit: money.FromInt(1_000)},
		{Month: clock.NewBusinessDate(2025, 12, 1), Code: AccountSales, Credit: money.FromInt(1)},
	}
	s := BuildTaxSummary(2026, rows)
	require.Len(t, s.Months, 12)
	assert.Equal(t, "4000000000.00", s.Turnover.String())
	assert.Equal(t, "4000000000.00", s.Months[11].Cumulative.String())
	assert.Equal(t, "20000.00", s.Months[1].PPh23Withheld.String())
	assert.Equal(t, "20000.00", s.Months[2].WithholdingPaid.String())
	assert.Equal(t, "5000.00", s.Months[2].PPh21Withheld.String())
	assert.Equal(t, "7000.00", s.Months[2].PPh42Withheld.String())
	assert.Equal(t, "9000.00", s.Months[2].FinalTax.String())
	assert.Equal(t, "1000.00", s.Months[2].FinalTaxPaid.String())
	assert.True(t, s.WarningExceeded, "≥ 80% batas Rp4,8 M")
	assert.False(t, s.LimitExceeded)
}
