package service

import (
	"context"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// PermReportRead memberi akses laporan keuangan, termasuk Buku Besar.
const PermReportRead = "accounting.report.read"

// maxLedgerDays membatasi rentang Buku Besar agar query tetap ringan di 1 VPS.
const maxLedgerDays = 366 * 2

// LedgerEntry adalah satu baris Buku Besar dengan saldo berjalan sesuai saldo normal akun.
type LedgerEntry struct {
	postgres.LedgerLine
	Balance money.Amount
}

// Ledger adalah Buku Besar satu akun untuk satu rentang tanggal.
type Ledger struct {
	Account        domain.Account
	Filter         postgres.LedgerFilter
	OpeningBalance money.Amount
	PeriodDebit    money.Amount
	PeriodCredit   money.Amount
	ClosingBalance money.Amount
	Lines          []LedgerEntry
	Page           paging.Page
	Total          int64
}

// GeneralLedger menghitung Buku Besar per akun dengan saldo awal dan saldo berjalan (S-127). Hanya jurnal yang
// sudah diposting. Saldo mengikuti saldo normal akun (debit − kredit atau kredit − debit).
func (s *Service) GeneralLedger(ctx context.Context, f postgres.LedgerFilter, page paging.Page) (Ledger, error) {
	if _, err := actor.RequireAny(ctx, PermJournalRead, PermReportRead); err != nil {
		return Ledger{}, err
	}
	fields := map[string]string{}
	if f.DateFrom.IsZero() {
		fields["dateFrom"] = "Tanggal mulai wajib diisi"
	}
	if f.DateTo.IsZero() {
		fields["dateTo"] = "Tanggal akhir wajib diisi"
	}
	if !f.DateFrom.IsZero() && !f.DateTo.IsZero() {
		if f.DateTo.Before(f.DateFrom) {
			fields["dateTo"] = "Tanggal akhir tidak boleh sebelum tanggal mulai"
		} else if f.DateFrom.AddDays(maxLedgerDays).Before(f.DateTo) {
			fields["dateTo"] = "Rentang Buku Besar paling panjang 2 tahun"
		}
	}
	if len(fields) > 0 {
		return Ledger{}, apperr.Validation(fields)
	}
	acc, err := s.Store.GetAccount(ctx, f.AccountID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return Ledger{}, apperr.Validation(map[string]string{"accountId": "Akun tidak ditemukan"})
	}
	if err != nil {
		return Ledger{}, err
	}
	sum, err := s.Store.LedgerSummary(ctx, f)
	if err != nil {
		return Ledger{}, err
	}
	lines, err := s.Store.LedgerLines(ctx, f, page)
	if err != nil {
		return Ledger{}, err
	}
	nb := acc.NormalBalance
	opening := nb.Balance(sum.OpeningDebit, sum.OpeningCredit)
	out := Ledger{
		Account: acc, Filter: f, OpeningBalance: opening, PeriodDebit: sum.PeriodDebit, PeriodCredit: sum.PeriodCredit,
		ClosingBalance: opening.Add(nb.Balance(sum.PeriodDebit, sum.PeriodCredit)), Page: page, Total: sum.LineCount,
		Lines: make([]LedgerEntry, len(lines)),
	}
	for i, l := range lines {
		// CumulativeNet = debit − kredit sejak date_from; untuk akun bersaldo kredit tandanya dibalik.
		movement := l.CumulativeNet
		if nb == domain.NormalCredit {
			movement = movement.Neg()
		}
		out.Lines[i] = LedgerEntry{LedgerLine: l, Balance: opening.Add(movement)}
	}
	return out, nil
}

// AccountBalance menghitung saldo satu akun (sesuai saldo normal) dari semua jurnal diposting sampai tanggal asOf,
// tanpa cek permission (laporan modul lain, mis. rekonsiliasi Barang Diterima Belum Ditagih 2102).
func (s *Service) AccountBalance(ctx context.Context, code string, asOf clock.BusinessDate) (money.Amount, error) {
	accs, err := s.Store.AccountsByCodes(ctx, []string{code})
	if err != nil {
		return money.Zero, err
	}
	acc, ok := accs[code]
	if !ok {
		return money.Zero, apperr.WithMessage(apperr.CodeNotFound, "Akun "+code+" tidak ditemukan.")
	}
	next := asOf.AddDays(1)
	sum, err := s.Store.LedgerSummary(ctx, postgres.LedgerFilter{AccountID: acc.ID, DateFrom: next, DateTo: next})
	if err != nil {
		return money.Zero, err
	}
	return acc.NormalBalance.Balance(sum.OpeningDebit, sum.OpeningCredit), nil
}
