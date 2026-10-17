package domain

import (
	"sort"
	"strings"

	"balans/internal/platform/money"
)

// Laporan keuangan (ACC-07, RPT-12): Neraca, Laba Rugi, dan Arus Kas metode tidak langsung disusun dari mutasi
// akun read model `rm_gl_daily`. Pengelompokan mengikuti kelompok COA (digit pertama kode) dan akun induk
// (Desain Finance & Accounting bagian 1). Semua perhitungan di sini murni tanpa DB.

// StatementKind adalah jenis laporan keuangan.
type StatementKind string

const (
	StatementBalanceSheet    StatementKind = "balance_sheet"
	StatementIncomeStatement StatementKind = "income_statement"
	StatementCashFlow        StatementKind = "cash_flow"
)

// Valid memeriksa jenis laporan.
func (k StatementKind) Valid() bool {
	return k == StatementBalanceSheet || k == StatementIncomeStatement || k == StatementCashFlow
}

// Label adalah judul laporan untuk UI dan Excel.
func (k StatementKind) Label() string {
	switch k {
	case StatementBalanceSheet:
		return "Neraca"
	case StatementIncomeStatement:
		return "Laba Rugi"
	case StatementCashFlow:
		return "Arus Kas"
	}
	return string(k)
}

// RowKind adalah jenis baris laporan.
type RowKind string

const (
	RowSection  RowKind = "section"
	RowGroup    RowKind = "group"
	RowAccount  RowKind = "account"
	RowSubtotal RowKind = "subtotal"
	RowTotal    RowKind = "total"
)

// FinancialRow adalah satu baris laporan. Code = kode akun untuk baris akun (drill-down Buku Besar).
type FinancialRow struct {
	Kind   RowKind
	Code   string
	Label  string
	Level  int
	Amount money.Amount
}

// Movement adalah mutasi satu akun: Before = debit − kredit sebelum awal periode, During = debit − kredit selama
// periode.
type Movement struct {
	Account Account
	Before  money.Amount
	During  money.Amount
}

// Closing adalah saldo debit − kredit akhir periode.
func (m Movement) Closing() money.Amount { return m.Before.Add(m.During) }

// ProfitLoss adalah ringkasan laba rugi; dipakai Laba Rugi, laba rugi per dimensi, dan dashboard.
type ProfitLoss struct {
	Revenue         money.Amount
	COGS            money.Amount
	GrossProfit     money.Amount
	OperatingExp    money.Amount
	OperatingProfit money.Amount
	OtherNet        money.Amount
	NetProfit       money.Amount
}

// AddAccount menambah mutasi debit − kredit satu akun laba rugi ke ringkasan.
func (p *ProfitLoss) AddAccount(g Group, debitMinusCredit money.Amount) {
	switch g {
	case GroupRevenue:
		p.Revenue = p.Revenue.Sub(debitMinusCredit)
	case GroupCOGS:
		p.COGS = p.COGS.Add(debitMinusCredit)
	case GroupExpense:
		p.OperatingExp = p.OperatingExp.Add(debitMinusCredit)
	case GroupOther:
		p.OtherNet = p.OtherNet.Sub(debitMinusCredit)
	default:
		return
	}
	p.GrossProfit = p.Revenue.Sub(p.COGS)
	p.OperatingProfit = p.GrossProfit.Sub(p.OperatingExp)
	p.NetProfit = p.OperatingProfit.Add(p.OtherNet)
}

// Statement adalah laporan keuangan tersusun.
type Statement struct {
	Kind StatementKind
	Rows []FinancialRow
	// Balanced: Neraca → Aset = Liabilitas + Ekuitas; Arus Kas → kenaikan kas = mutasi akun Kas & Bank.
	Balanced bool
	Summary  ProfitLoss
}

func sortedMovements(list []Movement) []Movement {
	out := append([]Movement(nil), list...)
	sort.Slice(out, func(i, j int) bool { return out[i].Account.Code < out[j].Account.Code })
	return out
}

// ParentLookup mengembalikan kode & nama akun induk untuk subjudul (mis. 1100 Kas & Bank); kosong = tanpa subjudul.
type ParentLookup func(a Account) (code, name string)

// section menyusun baris satu kelompok: judul, akun per akun induk dengan subtotal, dan total kelompok.
// credit = true membalik tanda (saldo kredit = K − D).
func section(title string, list []Movement, value func(Movement) money.Amount, credit bool, parent ParentLookup) ([]FinancialRow, money.Amount) {
	rows := []FinancialRow{{Kind: RowSection, Label: title}}
	total := money.Zero
	type bucket struct {
		code, name string
		rows       []FinancialRow
		sum        money.Amount
	}
	var order []string
	buckets := map[string]*bucket{}
	for _, m := range sortedMovements(list) {
		v := value(m)
		if credit {
			v = v.Neg()
		}
		if v.IsZero() {
			continue
		}
		pc, pn := parent(m.Account)
		b, ok := buckets[pc]
		if !ok {
			b = &bucket{code: pc, name: pn, sum: money.Zero}
			buckets[pc] = b
			order = append(order, pc)
		}
		b.rows = append(b.rows, FinancialRow{Kind: RowAccount, Code: m.Account.Code, Label: m.Account.Name, Level: 2, Amount: v})
		b.sum = b.sum.Add(v)
		total = total.Add(v)
	}
	for _, pc := range order {
		b := buckets[pc]
		if b.name != "" {
			rows = append(rows, FinancialRow{Kind: RowGroup, Code: b.code, Label: b.name, Level: 1})
		}
		rows = append(rows, b.rows...)
		if b.name != "" {
			rows = append(rows, FinancialRow{Kind: RowSubtotal, Code: b.code, Label: "Jumlah " + b.name, Level: 1, Amount: b.sum})
		}
	}
	return rows, total
}

func byGroup(list []Movement, g Group) []Movement {
	var out []Movement
	for _, m := range list {
		if m.Account.Group == g {
			out = append(out, m)
		}
	}
	return out
}

func during(m Movement) money.Amount { return m.During }

// BalanceSheet menyusun Neraca per akhir periode. Ekuitas ditambah laba (rugi) tahun berjalan (kelompok 4–7
// selama periode = sejak 1 Januari) dan laba (rugi) tahun lalu yang belum ditutup ke Laba Ditahan (saldo
// kelompok 4–7 sebelum awal tahun), sehingga Neraca selalu seimbang sebelum maupun sesudah tutup buku.
func BalanceSheet(list []Movement, parent ParentLookup) Statement {
	closing := func(m Movement) money.Amount { return m.Closing() }
	assets, totalAssets := section("ASET", byGroup(list, GroupAsset), closing, false, parent)
	liabs, totalLiabs := section("LIABILITAS", byGroup(list, GroupLiability), closing, true, parent)
	equity, totalEquity := section("EKUITAS", byGroup(list, GroupEquity), closing, true, parent)
	var current, prior ProfitLoss
	for _, m := range list {
		current.AddAccount(m.Account.Group, m.During)
		prior.AddAccount(m.Account.Group, m.Before)
	}
	if !prior.NetProfit.IsZero() {
		equity = append(equity, FinancialRow{Kind: RowAccount, Label: "Laba (Rugi) Tahun Lalu Belum Ditutup", Level: 2, Amount: prior.NetProfit})
		totalEquity = totalEquity.Add(prior.NetProfit)
	}
	equity = append(equity, FinancialRow{Kind: RowAccount, Code: "3202", Label: "Laba (Rugi) Tahun Berjalan (dihitung)", Level: 2, Amount: current.NetProfit})
	totalEquity = totalEquity.Add(current.NetProfit)

	rows := make([]FinancialRow, 0, len(assets)+len(liabs)+len(equity)+4)
	rows = append(rows, assets...)
	rows = append(rows, FinancialRow{Kind: RowTotal, Label: "TOTAL ASET", Amount: totalAssets})
	rows = append(rows, liabs...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Total Liabilitas", Amount: totalLiabs})
	rows = append(rows, equity...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Total Ekuitas", Amount: totalEquity})
	sumLE := totalLiabs.Add(totalEquity)
	rows = append(rows, FinancialRow{Kind: RowTotal, Label: "TOTAL LIABILITAS & EKUITAS", Amount: sumLE})
	return Statement{Kind: StatementBalanceSheet, Rows: rows, Balanced: totalAssets.Equal(sumLE), Summary: current}
}

// IncomeStatement menyusun Laba Rugi periode: Pendapatan − HPP = Laba Kotor; − Beban Operasional = Laba
// Operasional; ± Pendapatan & Beban Lain-lain = Laba Bersih.
func IncomeStatement(list []Movement, parent ParentLookup) Statement {
	var pl ProfitLoss
	for _, m := range list {
		pl.AddAccount(m.Account.Group, m.During)
	}
	rev, _ := section("PENDAPATAN", byGroup(list, GroupRevenue), during, true, parent)
	cogs, _ := section("HARGA POKOK PENJUALAN", byGroup(list, GroupCOGS), during, false, parent)
	opex, _ := section("BEBAN OPERASIONAL", byGroup(list, GroupExpense), during, false, parent)
	other, _ := section("PENDAPATAN & BEBAN LAIN-LAIN", byGroup(list, GroupOther), during, true, parent)
	rows := make([]FinancialRow, 0, len(rev)+len(cogs)+len(opex)+len(other)+8)
	rows = append(rows, rev...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Pendapatan Bersih", Amount: pl.Revenue})
	rows = append(rows, cogs...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Total HPP", Amount: pl.COGS},
		FinancialRow{Kind: RowTotal, Label: "LABA KOTOR", Amount: pl.GrossProfit})
	rows = append(rows, opex...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Total Beban Operasional", Amount: pl.OperatingExp},
		FinancialRow{Kind: RowTotal, Label: "LABA OPERASIONAL", Amount: pl.OperatingProfit})
	rows = append(rows, other...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Total Pendapatan & Beban Lain-lain", Amount: pl.OtherNet},
		FinancialRow{Kind: RowTotal, Label: "LABA (RUGI) BERSIH", Amount: pl.NetProfit})
	return Statement{Kind: StatementIncomeStatement, Rows: rows, Balanced: true, Summary: pl}
}

// Klasifikasi akun Arus Kas berdasarkan awalan kode COA.
const (
	cashPrefix         = "11"
	depreciationPrefix = "159"
)

// IsCashAccount: akun Kas & Bank (11xx).
func IsCashAccount(code string) bool { return strings.HasPrefix(code, cashPrefix) }

func cashFlowActivity(a Account) string {
	switch {
	case a.Group.IsProfitLoss(), IsCashAccount(a.Code):
		return ""
	case strings.HasPrefix(a.Code, depreciationPrefix):
		return "depreciation"
	case a.Group == GroupAsset && (strings.HasPrefix(a.Code, "12") || strings.HasPrefix(a.Code, "13") || strings.HasPrefix(a.Code, "14")):
		return "operating"
	case a.Group == GroupLiability:
		return "operating"
	case a.Group == GroupAsset:
		return "investing"
	case a.Group == GroupEquity:
		return "financing"
	}
	return ""
}

// CashFlow menyusun Arus Kas metode tidak langsung: laba bersih + penyusutan (kenaikan akumulasi penyusutan)
// ± perubahan modal kerja (piutang, persediaan, uang muka, liabilitas) = arus kas operasi; perubahan aset tetap
// = investasi; perubahan ekuitas = pendanaan. Setiap baris = −(mutasi debit − kredit) akun non-kas, sehingga
// jumlahnya selalu sama dengan kenaikan Kas & Bank (11xx).
func CashFlow(list []Movement) Statement {
	var pl ProfitLoss
	cashBefore, cashDuring := money.Zero, money.Zero
	type part struct {
		rows []FinancialRow
		sum  money.Amount
	}
	parts := map[string]*part{"operating": {sum: money.Zero}, "investing": {sum: money.Zero}, "financing": {sum: money.Zero}}
	depreciation := money.Zero
	for _, m := range sortedMovements(list) {
		pl.AddAccount(m.Account.Group, m.During)
		if IsCashAccount(m.Account.Code) {
			cashBefore, cashDuring = cashBefore.Add(m.Before), cashDuring.Add(m.During)
			continue
		}
		act := cashFlowActivity(m.Account)
		v := m.During.Neg()
		if act == "" || v.IsZero() {
			continue
		}
		if act == "depreciation" {
			depreciation = depreciation.Add(v)
			continue
		}
		label := "Perubahan " + m.Account.Name
		p := parts[act]
		p.rows = append(p.rows, FinancialRow{Kind: RowAccount, Code: m.Account.Code, Label: label, Level: 2, Amount: v})
		p.sum = p.sum.Add(v)
	}
	op := parts["operating"]
	opTotal := pl.NetProfit.Add(depreciation).Add(op.sum)
	rows := []FinancialRow{
		{Kind: RowSection, Label: "ARUS KAS DARI AKTIVITAS OPERASI"},
		{Kind: RowAccount, Label: "Laba (Rugi) Bersih", Level: 2, Amount: pl.NetProfit},
	}
	if !depreciation.IsZero() {
		rows = append(rows, FinancialRow{Kind: RowAccount, Label: "Penyusutan Aset Tetap", Level: 2, Amount: depreciation})
	}
	rows = append(rows, op.rows...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Arus Kas Bersih dari Aktivitas Operasi", Amount: opTotal})
	rows = append(rows, FinancialRow{Kind: RowSection, Label: "ARUS KAS DARI AKTIVITAS INVESTASI"})
	rows = append(rows, parts["investing"].rows...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Arus Kas Bersih dari Aktivitas Investasi", Amount: parts["investing"].sum})
	rows = append(rows, FinancialRow{Kind: RowSection, Label: "ARUS KAS DARI AKTIVITAS PENDANAAN"})
	rows = append(rows, parts["financing"].rows...)
	rows = append(rows, FinancialRow{Kind: RowSubtotal, Label: "Arus Kas Bersih dari Aktivitas Pendanaan", Amount: parts["financing"].sum})
	net := opTotal.Add(parts["investing"].sum).Add(parts["financing"].sum)
	rows = append(rows,
		FinancialRow{Kind: RowTotal, Label: "KENAIKAN (PENURUNAN) KAS & BANK", Amount: net},
		FinancialRow{Kind: RowAccount, Label: "Kas & Bank Awal Periode", Amount: cashBefore},
		FinancialRow{Kind: RowTotal, Label: "KAS & BANK AKHIR PERIODE", Amount: cashBefore.Add(net)},
	)
	return Statement{Kind: StatementCashFlow, Rows: rows, Balanced: net.Equal(cashDuring), Summary: pl}
}
