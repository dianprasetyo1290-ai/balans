package domain

import (
	"github.com/google/uuid"

	"balans/internal/platform/money"
)

// TrialBalanceRow adalah satu akun Neraca Saldo (ACC-18): saldo awal sebelum rentang, mutasi rentang, dan saldo
// akhir. Saldo awal/akhir ditampilkan di kolom debit atau kredit sesuai tanda (debit − kredit).
type TrialBalanceRow struct {
	AccountID     uuid.UUID
	AccountCode   string
	AccountName   string
	Group         Group
	NormalBalance NormalBalance
	OpeningNet    money.Amount
	PeriodDebit   money.Amount
	PeriodCredit  money.Amount
}

// ClosingNet adalah saldo akhir debit − kredit.
func (r TrialBalanceRow) ClosingNet() money.Amount {
	return r.OpeningNet.Add(r.PeriodDebit).Sub(r.PeriodCredit)
}

// SplitNet memecah saldo bertanda ke kolom debit/kredit.
func SplitNet(net money.Amount) (debit, credit money.Amount) {
	if net.IsNegative() {
		return money.Zero, net.Neg()
	}
	return net, money.Zero
}

// TrialBalanceTotals adalah total kolom Neraca Saldo; seimbang bila debit = kredit di tiap pasangan kolom.
type TrialBalanceTotals struct {
	OpeningDebit, OpeningCredit money.Amount
	PeriodDebit, PeriodCredit   money.Amount
	ClosingDebit, ClosingCredit money.Amount
}

// Balanced: total debit = total kredit untuk saldo awal, mutasi, dan saldo akhir.
func (t TrialBalanceTotals) Balanced() bool {
	return t.OpeningDebit.Equal(t.OpeningCredit) && t.PeriodDebit.Equal(t.PeriodCredit) && t.ClosingDebit.Equal(t.ClosingCredit)
}

// TrialBalanceTotalsOf menjumlahkan baris Neraca Saldo.
func TrialBalanceTotalsOf(rows []TrialBalanceRow) TrialBalanceTotals {
	t := TrialBalanceTotals{
		OpeningDebit: money.Zero, OpeningCredit: money.Zero, PeriodDebit: money.Zero, PeriodCredit: money.Zero,
		ClosingDebit: money.Zero, ClosingCredit: money.Zero,
	}
	for _, r := range rows {
		od, oc := SplitNet(r.OpeningNet)
		cd, cc := SplitNet(r.ClosingNet())
		t.OpeningDebit, t.OpeningCredit = t.OpeningDebit.Add(od), t.OpeningCredit.Add(oc)
		t.PeriodDebit, t.PeriodCredit = t.PeriodDebit.Add(r.PeriodDebit), t.PeriodCredit.Add(r.PeriodCredit)
		t.ClosingDebit, t.ClosingCredit = t.ClosingDebit.Add(cd), t.ClosingCredit.Add(cc)
	}
	return t
}
