package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Laporan turunan read model GL: laba rugi per dimensi (RPT-15), rekap pajak (RPT-18), dan dashboard (RPT-14).

// MaxReportDays membatasi rentang laporan periode (2 tahun, sama dengan Buku Besar).
const MaxReportDays = 732

// CheckRange memvalidasi rentang tanggal laporan.
func CheckRange(from, to clock.BusinessDate) error {
	fields := map[string]string{}
	if from.IsZero() {
		fields["dateFrom"] = "Tanggal awal wajib diisi"
	}
	if to.IsZero() {
		fields["dateTo"] = "Tanggal akhir wajib diisi"
	}
	if len(fields) == 0 {
		if to.Before(from) {
			fields["dateTo"] = "Tanggal akhir tidak boleh sebelum tanggal awal"
		} else if to.Time().Sub(from.Time()) > MaxReportDays*24*time.Hour {
			fields["dateTo"] = "Rentang laporan paling panjang 2 tahun"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Valid memeriksa jenis dimensi laporan.
func (k DimensionKind) Valid() bool {
	return k == KindChannel || k == KindSubChannel || k == KindDivision || k == KindCampaign
}

// DimensionAmount adalah mutasi debit − kredit satu akun laba rugi pada satu nilai dimensi. DimensionID uuid.Nil
// = jurnal tanpa dimensi tersebut (mis. biaya bersama, U-14).
type DimensionAmount struct {
	Account     Account
	DimensionID uuid.UUID
	Net         money.Amount
}

// DimensionColumn adalah satu kolom laba rugi per dimensi.
type DimensionColumn struct {
	ID      uuid.UUID
	Summary ProfitLoss
}

// DimensionAccountRow adalah satu akun laba rugi dengan nilai per kolom (urutan = kolom) dan total.
type DimensionAccountRow struct {
	Account Account
	Amounts []money.Amount
	Total   money.Amount
}

// DimensionProfitLoss adalah laba rugi per dimensi; jumlah kolom selalu = Total (laba rugi seluruh perusahaan).
type DimensionProfitLoss struct {
	Columns  []DimensionColumn
	Accounts []DimensionAccountRow
	Total    ProfitLoss
}

// BuildDimensionProfitLoss menyusun kolom per nilai dimensi (urutan order; nilai yang tidak ada di order
// ditambahkan di akhir, uuid.Nil = "Tanpa dimensi" paling akhir). Nilai akun pendapatan/lain-lain ditampilkan
// K − D, HPP/beban D − K.
func BuildDimensionProfitLoss(rows []DimensionAmount, order []uuid.UUID) DimensionProfitLoss {
	idx := map[uuid.UUID]int{}
	var cols []DimensionColumn
	add := func(id uuid.UUID) int {
		if i, ok := idx[id]; ok {
			return i
		}
		idx[id] = len(cols)
		cols = append(cols, DimensionColumn{ID: id})
		return idx[id]
	}
	present := map[uuid.UUID]bool{}
	for _, r := range rows {
		if !r.Net.IsZero() {
			present[r.DimensionID] = true
		}
	}
	for _, id := range order {
		if present[id] && id != uuid.Nil {
			add(id)
		}
	}
	for _, r := range rows {
		if present[r.DimensionID] && r.DimensionID != uuid.Nil {
			add(r.DimensionID)
		}
	}
	if present[uuid.Nil] {
		add(uuid.Nil)
	}
	out := DimensionProfitLoss{}
	accRows := map[uuid.UUID]*DimensionAccountRow{}
	var accOrder []Account
	for _, r := range rows {
		if r.Net.IsZero() {
			continue
		}
		i := idx[r.DimensionID]
		cols[i].Summary.AddAccount(r.Account.Group, r.Net)
		out.Total.AddAccount(r.Account.Group, r.Net)
		ar, ok := accRows[r.Account.ID]
		if !ok {
			ar = &DimensionAccountRow{Account: r.Account, Total: money.Zero}
			accRows[r.Account.ID] = ar
			accOrder = append(accOrder, r.Account)
		}
		for len(ar.Amounts) < len(cols) {
			ar.Amounts = append(ar.Amounts, money.Zero)
		}
		v := r.Net
		if r.Account.Group == GroupRevenue || r.Account.Group == GroupOther {
			v = v.Neg()
		}
		ar.Amounts[i] = ar.Amounts[i].Add(v)
		ar.Total = ar.Total.Add(v)
	}
	sortAccounts(accOrder)
	for _, a := range accOrder {
		ar := accRows[a.ID]
		for len(ar.Amounts) < len(cols) {
			ar.Amounts = append(ar.Amounts, money.Zero)
		}
		out.Accounts = append(out.Accounts, *ar)
	}
	out.Columns = cols
	return out
}

func sortAccounts(list []Account) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Code < list[j-1].Code; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// MarginPercent adalah laba ÷ pendapatan × 100 (2 desimal); nol bila pendapatan nol.
func MarginPercent(profit, revenue money.Amount) decimal.Decimal {
	if revenue.IsZero() {
		return decimal.Zero
	}
	return profit.Decimal().Div(revenue.Decimal()).Mul(decimal.NewFromInt(100)).Round(2)
}

// Akun pajak & omzet untuk rekap pajak (Desain Finance & Accounting bagian 1 dan 4).
const (
	AccountPPh21     = "2301"
	AccountPPh23     = "2302"
	AccountPPh42     = "2303"
	AccountPPhFinal  = "2304"
	AccountSales     = "4101"
	AccountShipping  = "4201"
	AccountFinalTaxE = "7203"
)

// TaxAccounts adalah akun yang dibaca rekap pajak.
var TaxAccounts = []string{AccountPPh21, AccountPPh23, AccountPPh42, AccountPPhFinal, AccountSales, AccountShipping, AccountFinalTaxE}

// SmallBusinessTurnoverLimit adalah batas omzet bruto setahun pengusaha kecil / PPh Final UMKM (Rp4,8 M, BRD
// asumsi 1, RPT-18). TODO(setting): pindahkan ke modul setting bersama tarif PPh Final (S-407).
var SmallBusinessTurnoverLimit = money.FromInt(4_800_000_000)

// TurnoverWarningPercent: persentase batas omzet yang memicu peringatan dini. TODO(setting).
var TurnoverWarningPercent = decimal.NewFromInt(80)

// MonthlyAccount adalah mutasi debit/kredit satu akun pada satu bulan.
type MonthlyAccount struct {
	Month  clock.BusinessDate
	Code   string
	Debit  money.Amount
	Credit money.Amount
}

// TaxMonth adalah rekap pajak satu masa (bulan).
type TaxMonth struct {
	Month clock.BusinessDate
	// Withheld per jenis PPh (2301/2302/2303): dipotong (kredit) dan disetor (debit) di masa itu.
	PPh21Withheld, PPh23Withheld, PPh42Withheld money.Amount
	WithholdingPaid                             money.Amount
	FinalTax                                    money.Amount
	FinalTaxPaid                                money.Amount
	// Turnover adalah omzet bruto (penjualan 4101 + ongkos kirim 4201, sebelum potongan) masa itu; Cumulative
	// sejak 1 Januari.
	Turnover   money.Amount
	Cumulative money.Amount
}

// TaxSummary adalah rekap pajak tahun berjalan (RPT-18).
type TaxSummary struct {
	Year            int
	Months          []TaxMonth
	Turnover        money.Amount
	Limit           money.Amount
	UsedPercent     decimal.Decimal
	WarningExceeded bool
	LimitExceeded   bool
}

// BuildTaxSummary menyusun rekap 12 masa dari mutasi bulanan akun pajak & omzet.
func BuildTaxSummary(year int, rows []MonthlyAccount) TaxSummary {
	months := make([]TaxMonth, 12)
	for i := range months {
		months[i] = TaxMonth{
			Month: clock.NewBusinessDate(year, time.Month(i+1), 1), PPh21Withheld: money.Zero, PPh23Withheld: money.Zero,
			PPh42Withheld: money.Zero, WithholdingPaid: money.Zero, FinalTax: money.Zero, FinalTaxPaid: money.Zero,
			Turnover: money.Zero, Cumulative: money.Zero,
		}
	}
	for _, r := range rows {
		if r.Month.Year() != year {
			continue
		}
		m := &months[int(r.Month.Month())-1]
		switch r.Code {
		case AccountPPh21:
			m.PPh21Withheld = m.PPh21Withheld.Add(r.Credit)
			m.WithholdingPaid = m.WithholdingPaid.Add(r.Debit)
		case AccountPPh23:
			m.PPh23Withheld = m.PPh23Withheld.Add(r.Credit)
			m.WithholdingPaid = m.WithholdingPaid.Add(r.Debit)
		case AccountPPh42:
			m.PPh42Withheld = m.PPh42Withheld.Add(r.Credit)
			m.WithholdingPaid = m.WithholdingPaid.Add(r.Debit)
		case AccountPPhFinal:
			m.FinalTax = m.FinalTax.Add(r.Credit)
			m.FinalTaxPaid = m.FinalTaxPaid.Add(r.Debit)
		case AccountSales, AccountShipping:
			m.Turnover = m.Turnover.Add(r.Credit.Sub(r.Debit))
		}
	}
	cum := money.Zero
	for i := range months {
		cum = cum.Add(months[i].Turnover)
		months[i].Cumulative = cum
	}
	out := TaxSummary{Year: year, Months: months, Turnover: cum, Limit: SmallBusinessTurnoverLimit}
	out.UsedPercent = MarginPercent(cum, out.Limit)
	out.WarningExceeded = out.UsedPercent.GreaterThanOrEqual(TurnoverWarningPercent)
	out.LimitExceeded = cum.Cmp(out.Limit) > 0
	return out
}
