package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

// OpeningView adalah halaman Saldo Awal: set, Neraca Saldo Awal, total, dan aksi.
type OpeningView struct {
	Set            *domain.OpeningSet
	Lines          []domain.OpeningLine
	Totals         domain.OpeningTotals
	AllowedActions []string
}

// Aksi halaman Saldo Awal.
const (
	OpeningActionEdit = "edit"
	OpeningActionPost = "post"
)

func (s *Service) requireOpeningRead(ctx context.Context) (actor.Actor, error) {
	return actor.RequireAny(ctx, PermJournalRead, PermReportRead)
}

// GetOpening mengembalikan saldo awal (Set nil bila tanggal cut-over belum diisi).
func (s *Service) GetOpening(ctx context.Context) (OpeningView, error) {
	a, err := s.requireOpeningRead(ctx)
	if err != nil {
		return OpeningView{}, err
	}
	return s.openingView(ctx, a)
}

func (s *Service) openingView(ctx context.Context, a actor.Actor) (OpeningView, error) {
	v := OpeningView{AllowedActions: []string{}, Totals: domain.TotalsOf(nil)}
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil {
		return OpeningView{}, err
	}
	if !ok {
		if a.Has(PermJournalWrite) {
			v.AllowedActions = append(v.AllowedActions, OpeningActionEdit)
		}
		return v, nil
	}
	v.Set = &set
	if v.Lines, err = s.Store.ListOpeningLines(ctx, set.ID); err != nil {
		return OpeningView{}, err
	}
	v.Totals = domain.TotalsOf(v.Lines)
	if set.Status == domain.OpeningDraft {
		if a.Has(PermJournalWrite) {
			v.AllowedActions = append(v.AllowedActions, OpeningActionEdit)
		}
		if a.Has(PermJournalPost) && len(v.Lines) > 0 {
			v.AllowedActions = append(v.AllowedActions, OpeningActionPost)
		}
	}
	return v, nil
}

// draftOpeningSet mengunci set saldo awal yang masih Draft.
func (s *Service) draftOpeningSet(ctx context.Context) (domain.OpeningSet, error) {
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil {
		return domain.OpeningSet{}, err
	}
	if !ok {
		return domain.OpeningSet{}, apperr.WithMessage(apperr.CodeValidationFailed,
			"Isi tanggal cut-over di Keuangan & Akuntansi › Saldo Awal sebelum mengisi atau mengimpor saldo awal.")
	}
	if _, err := s.Store.LockOpeningSet(ctx, set.ID); err != nil {
		return domain.OpeningSet{}, err
	}
	return set, set.CheckEditable()
}

// CurrentOpeningSet membaca set saldo awal tanpa cek permission (impor saldo stok, cmd/cutover).
func (s *Service) CurrentOpeningSet(ctx context.Context) (domain.OpeningSet, bool, error) {
	return s.Store.GetOpeningSet(ctx)
}

// SaveCutoverDate membuat set saldo awal atau mengubah tanggal cut-over (selama Draft).
func (s *Service) SaveCutoverDate(ctx context.Context, cutover clock.BusinessDate, version int) (OpeningView, error) {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return OpeningView{}, err
	}
	if err := domain.ValidateCutoverDate(cutover); err != nil {
		return OpeningView{}, err
	}
	var out OpeningView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		set, ok, err := s.Store.GetOpeningSet(ctx)
		if err != nil {
			return err
		}
		if !ok {
			id, err := s.Store.InsertOpeningSet(ctx, cutover, a.UserID)
			if err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionCreate, EntityType: "opening_balance", EntityID: id, EntityLabel: "Saldo Awal",
				After: map[string]any{"cutoverDate": cutover.String()},
			}); err != nil {
				return err
			}
		} else {
			if _, err := s.Store.LockOpeningSet(ctx, set.ID); err != nil {
				return err
			}
			if err := set.CheckEditable(); err != nil {
				return err
			}
			before := set.CutoverDate
			set.CutoverDate = cutover
			if err := s.Store.SaveOpeningSet(ctx, set, version, a.UserID); err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionUpdate, EntityType: "opening_balance", EntityID: set.ID, EntityLabel: "Saldo Awal",
				Before: map[string]any{"cutoverDate": before.String()}, After: map[string]any{"cutoverDate": cutover.String()},
			}); err != nil {
				return err
			}
		}
		out, err = s.openingView(ctx, a)
		return err
	})
	return out, err
}

// EnsureCutoverDate membuat set saldo awal dengan tanggal cut-over bila belum ada (cmd/cutover). Set yang sudah
// ada dengan tanggal lain ditolak.
func (s *Service) EnsureCutoverDate(ctx context.Context, cutover clock.BusinessDate) error {
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil {
		return err
	}
	if ok {
		if !set.CutoverDate.Equal(cutover) {
			return apperr.WithMessage(apperr.CodeValidationFailed,
				fmt.Sprintf("Tanggal cut-over sudah diisi %s, berbeda dengan %s.", set.CutoverDate, cutover))
		}
		return nil
	}
	_, err = s.SaveCutoverDate(ctx, cutover, 0)
	return err
}

// SaveOpeningLine membuat atau mengganti saldo awal satu akun (entri manual dan impor).
func (s *Service) SaveOpeningLine(ctx context.Context, in domain.OpeningLineInput) (OpeningView, error) {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return OpeningView{}, err
	}
	var out OpeningView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.saveOpeningLine(ctx, in, a.UserID); err != nil {
			return err
		}
		out, err = s.openingView(ctx, a)
		return err
	})
	return out, err
}

func (s *Service) saveOpeningLine(ctx context.Context, in domain.OpeningLineInput, by uuid.UUID) error {
	set, err := s.draftOpeningSet(ctx)
	if err != nil {
		return err
	}
	accounts, err := s.Store.AccountsByIDs(ctx, []uuid.UUID{in.AccountID})
	if err != nil {
		return err
	}
	l, err := domain.NewOpeningLine(in, accounts[in.AccountID])
	if err != nil {
		return err
	}
	id, err := s.Store.UpsertOpeningLine(ctx, set.ID, l, by)
	if err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionUpdate, EntityType: "opening_balance", EntityID: set.ID, EntityLabel: "Saldo Awal",
		After: map[string]any{"lineId": id, "account": l.AccountCode, "debit": l.Debit.String(), "credit": l.Credit.String()},
		Note:  "Saldo awal akun " + l.AccountCode,
	})
}

// DeleteOpeningLine menghapus saldo awal satu akun.
func (s *Service) DeleteOpeningLine(ctx context.Context, id uuid.UUID) (OpeningView, error) {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return OpeningView{}, err
	}
	var out OpeningView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		set, err := s.draftOpeningSet(ctx)
		if err != nil {
			return err
		}
		l, err := s.Store.GetOpeningLine(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Store.DeleteOpeningLine(ctx, set.ID, id); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "opening_balance", EntityID: set.ID, EntityLabel: "Saldo Awal",
			Before: map[string]any{"account": l.AccountCode, "debit": l.Debit.String(), "credit": l.Credit.String()},
			Note:   "Hapus saldo awal akun " + l.AccountCode,
		}); err != nil {
			return err
		}
		out, err = s.openingView(ctx, a)
		return err
	})
	return out, err
}

// PostOpening memposting Neraca Saldo Awal sebagai satu jurnal A-36 pada tanggal cut-over; selisih debit −
// kredit dibukukan ke 3901 Saldo Awal Ekuitas. Setelah diposting saldo awal terkunci.
func (s *Service) PostOpening(ctx context.Context, version int) (OpeningView, error) {
	a, err := actor.Require(ctx, PermJournalPost)
	if err != nil {
		return OpeningView{}, err
	}
	var out OpeningView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		set, err := s.draftOpeningSet(ctx)
		if err != nil {
			return err
		}
		if set.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		lines, err := s.Store.ListOpeningLines(ctx, set.ID)
		if err != nil {
			return err
		}
		req, err := domain.OpeningPosting(set, lines)
		if err != nil {
			return err
		}
		e, err := s.Post(ctx, req)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		set.Status, set.JournalEntryID, set.PostedAt, set.PostedBy = domain.OpeningPosted, &e.ID, &now, &a.UserID
		if err := s.Store.SaveOpeningSet(ctx, set, version, a.UserID); err != nil {
			return err
		}
		t := domain.TotalsOf(lines)
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: "opening_balance", EntityID: set.ID, EntityLabel: "Saldo Awal",
			After: map[string]any{"journal": e.Number, "debit": t.Debit.String(), "credit": t.Credit.String(), "difference": t.Difference.String()},
			Note:  "Posting saldo awal A-36 " + e.Number,
		}); err != nil {
			return err
		}
		out, err = s.openingView(ctx, a)
		return err
	})
	return out, err
}

// --- Impor saldo awal -------------------------------------------------------------------------------

// OpeningBalanceColumns adalah kolom sheet Neraca Saldo Awal (Template Migrasi Saldo Awal).
var OpeningBalanceColumns = []tabular.Column{
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText, Required: true, Identity: true, Example: "1111",
		Note: "Hanya akun neraca (1xxx–3xxx). Akun yang sama di impor ulang diganti."},
	{Key: "accountName", Header: "Nama Akun", Kind: tabular.KindText, Example: "Bank — rekening operasional 1", Note: "Untuk pengecekan."},
	{Key: "debit", Header: "Debit", Kind: tabular.KindDecimal, Example: "250000000", Note: "Isi salah satu Debit/Kredit."},
	{Key: "credit", Header: "Kredit", Kind: tabular.KindDecimal},
}

func parseAmount(p *tabular.Parser, key string) money.Amount {
	d := p.Decimal(key)
	if d == nil {
		return money.Zero
	}
	return money.FromDecimal(*d)
}

func remapFields(err error, mapping map[string]string) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		if m, ok := mapping[k]; ok {
			k = m
		}
		fields[k] = v
	}
	return apperr.Validation(fields)
}

func (s *Service) accountByCode(ctx context.Context, code string) (domain.Account, bool, error) {
	found, err := s.Store.AccountsByCodes(ctx, []string{code})
	if err != nil {
		return domain.Account{}, false, err
	}
	acc, ok := found[code]
	return acc, ok, nil
}

// ImportOpeningBalanceRow menyimpan satu baris Neraca Saldo Awal dari Excel (kunci: kode akun).
func (s *Service) ImportOpeningBalanceRow(ctx context.Context, row tabular.Row) error {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return err
	}
	p := tabular.Parse(row)
	code := strings.TrimSpace(p.Text("accountCode"))
	in := domain.OpeningLineInput{Debit: parseAmount(p, "debit"), Credit: parseAmount(p, "credit")}
	acc, ok, err := s.accountByCode(ctx, code)
	if err != nil {
		return err
	}
	if !ok {
		p.Fail("accountCode", "Akun tidak ditemukan di Chart of Accounts")
	}
	if err := p.Err(); err != nil {
		return err
	}
	in.AccountID = acc.ID
	return remapFields(s.saveOpeningLine(ctx, in, a.UserID), map[string]string{"accountId": "accountCode"})
}

// ExportOpeningBalances mengalirkan Neraca Saldo Awal.
func (s *Service) ExportOpeningBalances(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := s.requireOpeningRead(ctx); err != nil {
		return err
	}
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil || !ok {
		return err
	}
	lines, err := s.Store.ListOpeningLines(ctx, set.ID)
	if err != nil {
		return err
	}
	for _, l := range lines {
		r := tabular.NewRow(0).Set("accountCode", l.AccountCode).Set("accountName", l.AccountName)
		if l.Debit.IsPositive() {
			r.Set("debit", l.Debit.String())
		}
		if l.Credit.IsPositive() {
			r.Set("credit", l.Credit.String())
		}
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// receivableChannelOptions adalah pilihan kolom Channel sheet Piutang Terbuka → kode channel.
var receivableChannelOptions = []tabular.Option{
	{Value: "modern_trade", Label: "Modern Trade"}, {Value: "consignment", Label: "Konsinyasi"},
	{Value: "marketplace", Label: "Marketplace"}, {Value: "social_commerce", Label: "WA/IG"},
	{Value: "b2b", Label: "B2B"}, {Value: "employee", Label: "Karyawan"},
}

// OpeningReceivableColumns adalah kolom sheet Piutang Terbuka.
var OpeningReceivableColumns = []tabular.Column{
	{Key: "documentNumber", Header: "Nomor Invoice Lama", Kind: tabular.KindText, Required: true, Identity: true, Example: "INV-LAMA-0931"},
	{Key: "documentDate", Header: "Tanggal Invoice", Kind: tabular.KindDate, Required: true, Example: "2026-12-15"},
	{Key: "dueDate", Header: "Jatuh Tempo", Kind: tabular.KindDate, Required: true, Example: "2027-01-14"},
	{Key: "partyCode", Header: "Kode Pelanggan/Mitra", Kind: tabular.KindText, Required: true, Identity: true, Example: "MT-SINARJAYA"},
	{Key: "channel", Header: "Channel", Kind: tabular.KindChoice, Required: true, Options: receivableChannelOptions, Example: "Konsinyasi"},
	{Key: "amount", Header: "Nilai Invoice", Kind: tabular.KindDecimal, Required: true, Example: "4500000"},
	{Key: "openAmount", Header: "Sisa Belum Dibayar", Kind: tabular.KindDecimal, Required: true, Example: "4500000", Note: "<= Nilai Invoice"},
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText, Example: "1201", Note: "Kosong = 1201 Piutang Usaha."},
}

// OpeningPayableColumns adalah kolom sheet Utang Terbuka.
var OpeningPayableColumns = []tabular.Column{
	{Key: "documentNumber", Header: "Nomor Tagihan Pemasok", Kind: tabular.KindText, Required: true, Identity: true, Example: "KJ/INV/2026/551"},
	{Key: "partyCode", Header: "Kode Pemasok", Kind: tabular.KindText, Required: true, Identity: true, Example: "SUP-KEMASJAYA"},
	{Key: "documentDate", Header: "Tanggal Tagihan", Kind: tabular.KindDate, Required: true, Example: "2026-12-20"},
	{Key: "dueDate", Header: "Jatuh Tempo", Kind: tabular.KindDate, Required: true, Example: "2027-01-19"},
	{Key: "amount", Header: "Nilai Tagihan", Kind: tabular.KindDecimal, Required: true, Example: "48500000"},
	{Key: "openAmount", Header: "Sisa Belum Dibayar", Kind: tabular.KindDecimal, Required: true, Example: "48500000"},
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText, Example: "2101", Note: "Kosong = 2101 Utang Usaha."},
}

// Akun kontrol default saldo terbuka.
const (
	defaultReceivableAccount = "1201"
	defaultPayableAccount    = "2101"
)

// ImportOpeningReceivableRow menyimpan satu piutang terbuka per invoice lama.
func (s *Service) ImportOpeningReceivableRow(ctx context.Context, row tabular.Row) error {
	return s.importOpeningItem(ctx, row, domain.OpeningReceivable)
}

// ImportOpeningPayableRow menyimpan satu utang terbuka per tagihan pemasok lama.
func (s *Service) ImportOpeningPayableRow(ctx context.Context, row tabular.Row) error {
	return s.importOpeningItem(ctx, row, domain.OpeningPayable)
}

func (s *Service) importOpeningItem(ctx context.Context, row tabular.Row, t domain.OpeningItemType) error {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return err
	}
	s.ext.mu.RLock()
	parties := s.ext.parties
	s.ext.mu.RUnlock()
	if parties == nil {
		return apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul party tidak aktif.")
	}
	p := tabular.Parse(row)
	in := domain.OpeningItemInput{
		Type: t, PartyCode: strings.TrimSpace(p.Text("partyCode")), DocumentNumber: p.Text("documentNumber"),
		Amount: parseAmount(p, "amount"), OpenAmount: parseAmount(p, "openAmount"),
	}
	if d := p.Date("documentDate"); d != nil {
		in.DocumentDate = *d
	}
	if d := p.Date("dueDate"); d != nil {
		in.DueDate = *d
	}
	accountCode := strings.TrimSpace(p.Text("accountCode"))
	if accountCode == "" {
		accountCode = defaultPayableAccount
		if t == domain.OpeningReceivable {
			accountCode = defaultReceivableAccount
		}
	}
	if t == domain.OpeningReceivable {
		if code := p.Choice("channel", receivableChannelOptions); code != "" {
			list, err := s.Store.ListDimensions(ctx, domain.KindChannel, postgres.DimensionFilter{IncludeInactive: true})
			if err != nil {
				return err
			}
			for _, ch := range list {
				if ch.Code == code {
					id := ch.ID
					in.ChannelID = &id
				}
			}
			if in.ChannelID == nil {
				p.Fail("channel", "Channel "+code+" belum ada di master channel")
			}
		}
	}
	if in.PartyCode != "" {
		partyType, id, ok, err := parties.PartyByCode(ctx, t, in.PartyCode)
		if err != nil {
			return err
		}
		if !ok {
			p.Fail("partyCode", "Kode pihak tidak ditemukan di data master")
		}
		in.PartyType, in.PartyID = partyType, id
	}
	acc, _, err := s.accountByCode(ctx, accountCode)
	if err != nil {
		return err
	}
	if err := p.Err(); err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		set, err := s.draftOpeningSet(ctx)
		if err != nil {
			return err
		}
		it, err := domain.NewOpeningItem(in, acc, set.CutoverDate)
		if err != nil {
			return err
		}
		id, err := s.Store.UpsertOpeningItem(ctx, set.ID, it, a.UserID)
		if err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionImport, EntityType: "opening_balance", EntityID: set.ID, EntityLabel: "Saldo Awal",
			After: map[string]any{"itemId": id, "type": t, "party": it.PartyCode, "document": it.DocumentNumber, "open": it.OpenAmount.String()},
		})
	})
}

// OpeningItemList adalah satu halaman saldo terbuka.
type OpeningItemList struct {
	Items       []domain.OpeningItem
	Page        paging.Page
	Total       int64
	Outstanding money.Amount
}

// ListOpeningItems mengembalikan piutang/utang terbuka saldo awal (halaman Saldo Awal).
func (s *Service) ListOpeningItems(ctx context.Context, f postgres.OpeningItemFilter, page paging.Page) (OpeningItemList, error) {
	if _, err := s.requireOpeningRead(ctx); err != nil {
		return OpeningItemList{}, err
	}
	return s.OpeningItems(ctx, f, page)
}

// OpeningItems mengembalikan saldo terbuka tanpa cek permission (API publik untuk modul penjualan/pengadaan).
func (s *Service) OpeningItems(ctx context.Context, f postgres.OpeningItemFilter, page paging.Page) (OpeningItemList, error) {
	r, err := s.Store.ListOpeningItems(ctx, f, page)
	if err != nil {
		return OpeningItemList{}, err
	}
	return OpeningItemList{Items: r.Items, Page: r.Page, Total: r.Total, Outstanding: r.Outstanding}, nil
}

// OpeningSettlement adalah pelunasan saldo terbuka oleh dokumen pembayaran modul lain.
type OpeningSettlement struct {
	ItemID       uuid.UUID
	SourceType   string
	SourceID     uuid.UUID
	BusinessDate clock.BusinessDate
	// Amount positif = pelunasan; negatif = pembatalan pelunasan (dokumen pembayaran dibatalkan, source berbeda).
	Amount money.Amount
}

// SettleOpeningItem mencatat pelunasan saldo terbuka di transaksi aktif. Idempotent per dokumen sumber: pelunasan
// kedua dengan source yang sama diabaikan. Tidak memposting jurnal (jurnal pembayaran A-PB oleh modul pembayar).
func (s *Service) SettleOpeningItem(ctx context.Context, in OpeningSettlement) (domain.OpeningItem, error) {
	var it domain.OpeningItem
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if it, err = s.Store.GetOpeningItem(ctx, in.ItemID); err != nil {
			return err
		}
		if strings.TrimSpace(in.SourceType) == "" || in.SourceID == uuid.Nil || in.BusinessDate.IsZero() {
			return apperr.Validation(map[string]string{"source": "Dokumen pelunasan wajib diisi"})
		}
		exists, err := s.Store.SettlementExists(ctx, it.ID, in.SourceType, in.SourceID)
		if err != nil || exists {
			return err
		}
		if err := it.Settle(in.Amount); err != nil {
			return err
		}
		return s.Store.RecordSettlement(ctx, it, in.SourceType, in.SourceID, in.BusinessDate, in.Amount, actor.UserID(ctx))
	})
	return it, err
}

// --- Rekonsiliasi cut-over -------------------------------------------------------------------------

// Reconciliation adalah hasil pengecekan rekonsiliasi cut-over (Panduan Migrasi Data bagian 4).
type Reconciliation struct {
	CutoverDate *clock.BusinessDate
	// LedgerSource: "posted" = saldo GL dari jurnal terposting s.d. cut-over; "draft" = dari Neraca Saldo Awal
	// yang belum diposting.
	LedgerSource string
	Checks       []domain.ReconciliationCheck
	Passed       bool
}

// inventoryAccounts adalah akun persediaan yang dicek (Panduan Migrasi Data bagian 4).
var inventoryAccounts = []string{"1301", "1302", "1311", "1312", "1313", "1314"}

// CutoverReconciliation membandingkan sub-ledger saldo awal dengan saldo GL per akun: neraca saldo awal balance,
// persediaan (Σ qty × HPP per akun), piutang & utang terbuka, harga perolehan & akumulasi aset tetap.
func (s *Service) CutoverReconciliation(ctx context.Context) (Reconciliation, error) {
	if _, err := s.requireOpeningRead(ctx); err != nil {
		return Reconciliation{}, err
	}
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	if !ok {
		return Reconciliation{Checks: []domain.ReconciliationCheck{}, Passed: false}, nil
	}
	cutover := set.CutoverDate
	out := Reconciliation{CutoverDate: &cutover}
	var gl map[string]money.Amount
	lines, err := s.Store.ListOpeningLines(ctx, set.ID)
	if err != nil {
		return Reconciliation{}, err
	}
	if set.Status == domain.OpeningPosted {
		out.LedgerSource = "posted"
		if gl, err = s.Store.AccountNets(ctx, cutover); err != nil {
			return Reconciliation{}, err
		}
	} else {
		out.LedgerSource = "draft"
		if gl, err = s.Store.OpeningLineNets(ctx, set.ID); err != nil {
			return Reconciliation{}, err
		}
	}
	accounts, err := s.Store.ListAccounts(ctx, true)
	if err != nil {
		return Reconciliation{}, err
	}
	normal := map[string]domain.NormalBalance{}
	for _, a := range accounts {
		normal[a.Code] = a.NormalBalance
	}
	balance := func(code string) money.Amount {
		net := gl[code]
		if normal[code] == domain.NormalCredit {
			return net.Neg()
		}
		return net
	}

	t := domain.TotalsOf(lines)
	balanced := domain.Compare("trial_balance", "Neraca saldo awal balance (Σ debit = Σ kredit)", "", t.Debit, t.Credit)
	if len(lines) == 0 {
		balanced.Status, balanced.Note = domain.ReconFailed, "Neraca Saldo Awal belum diisi"
	}
	out.Checks = append(out.Checks, balanced)

	s.ext.mu.RLock()
	stock := s.ext.stock
	s.ext.mu.RUnlock()
	if stock != nil {
		values, err := stock.InventoryValueByAccount(ctx, cutover)
		if err != nil {
			return Reconciliation{}, err
		}
		for _, code := range inventoryAccounts {
			v, hasStock := values[code]
			if !hasStock && balance(code).IsZero() {
				continue
			}
			if !hasStock {
				v = money.Zero
			}
			out.Checks = append(out.Checks, domain.Compare("inventory", "Nilai persediaan = saldo akun persediaan", code, v, balance(code)))
		}
	} else {
		out.Checks = append(out.Checks, domain.ReconciliationCheck{
			Code: "inventory", Label: "Nilai persediaan = saldo akun persediaan", Status: domain.ReconSkipped,
			Subledger: money.Zero, Ledger: money.Zero, Difference: money.Zero, Note: "Modul inventory tidak aktif",
		})
	}

	items, err := s.Store.OpeningItemTotals(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	itemTotals := map[string]money.Amount{}
	for _, it := range items {
		itemTotals[it.AccountCode] = itemTotals[it.AccountCode].Add(it.Open)
	}
	controlAccounts := map[string]string{defaultReceivableAccount: "receivable", defaultPayableAccount: "payable"}
	for _, a := range accounts {
		if strings.HasPrefix(a.Code, "121") {
			controlAccounts[a.Code] = "receivable"
		}
	}
	for code := range itemTotals {
		if _, ok := controlAccounts[code]; !ok {
			controlAccounts[code] = "payable"
			if strings.HasPrefix(code, "12") {
				controlAccounts[code] = "receivable"
			}
		}
	}
	codes := make([]string, 0, len(controlAccounts))
	for c := range controlAccounts {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, code := range codes {
		sub := itemTotals[code]
		if sub.IsZero() && balance(code).IsZero() && code != defaultReceivableAccount && code != defaultPayableAccount {
			continue
		}
		label := "Piutang terbuka = saldo akun piutang"
		if controlAccounts[code] == "payable" {
			label = "Utang terbuka = saldo akun utang"
		}
		out.Checks = append(out.Checks, domain.Compare(controlAccounts[code], label, code, sub, balance(code)))
	}

	assets, err := s.Store.AssetClassTotals(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	for _, a := range assets {
		out.Checks = append(out.Checks,
			domain.Compare("fixed_asset_cost", "Harga perolehan aset tetap = saldo akun aset", a.AssetAccountCode, a.Cost, balance(a.AssetAccountCode)),
			domain.Compare("fixed_asset_accumulated", "Akumulasi penyusutan = saldo akun akumulasi", a.AccumulatedAccountCode, a.Accumulated, balance(a.AccumulatedAccountCode)))
	}
	out.Passed = domain.ReconciliationPassed(out.Checks)
	return out, nil
}

// --- Impor aset saldo awal --------------------------------------------------------------------------

// OpeningAssetColumns adalah kolom sheet Aset Tetap (Template Migrasi Saldo Awal) + Kode Divisi opsional.
var OpeningAssetColumns = []tabular.Column{
	{Key: "code", Header: "Kode Aset", Kind: tabular.KindText, Required: true, Identity: true, Example: "AST-VM-001"},
	{Key: "name", Header: "Nama Aset", Kind: tabular.KindText, Required: true, Example: "Mesin Vending Lokasi A"},
	{Key: "className", Header: "Kelas Aset", Kind: tabular.KindText, Required: true, Example: "Mesin Vending",
		Note: "Nama kelas aset: Mesin Vending, Peralatan Kantor & Komputer, Kendaraan."},
	{Key: "acquisitionDate", Header: "Tanggal Perolehan", Kind: tabular.KindDate, Required: true, Example: "2025-07-01"},
	{Key: "cost", Header: "Harga Perolehan", Kind: tabular.KindDecimal, Required: true, Example: "45000000"},
	{Key: "usefulLife", Header: "Umur Manfaat (bulan)", Kind: tabular.KindInteger, Required: true, Example: "48"},
	{Key: "salvage", Header: "Nilai Sisa", Kind: tabular.KindDecimal, Example: "0"},
	{Key: "openingAccumulated", Header: "Akumulasi Penyusutan per Cut-over", Kind: tabular.KindDecimal, Required: true, Example: "16875000"},
	{Key: "locationLabel", Header: "Lokasi / Mitra", Kind: tabular.KindText, Example: "MT-SINARJAYA", Note: "Untuk mesin vending: kode mitra."},
	{Key: "divisionCode", Header: "Kode Divisi", Kind: tabular.KindText, Example: "SLS", Note: "Dimensi jurnal penyusutan. Kosong = FAT."},
}

// defaultAssetDivision adalah divisi penyusutan aset saldo awal bila kolom Kode Divisi kosong.
const defaultAssetDivision = "FAT"

// ImportOpeningAssetRow membuat aset tetap saldo awal (langsung Aktif, tanpa jurnal perolehan). Aset dengan
// kode yang sudah ada dan masih Draft diganti; aset yang sudah aktif ditolak.
func (s *Service) ImportOpeningAssetRow(ctx context.Context, row tabular.Row) error {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return err
	}
	p := tabular.Parse(row)
	in := domain.FixedAssetInput{
		Code: p.Text("code"), Name: p.Text("name"), Source: domain.AssetOpening, Cost: parseAmount(p, "cost"),
		Salvage: parseAmount(p, "salvage"), OpeningAccumulated: parseAmount(p, "openingAccumulated"),
		LocationLabel: p.Text("locationLabel"),
	}
	if d := p.Date("acquisitionDate"); d != nil {
		in.AcquisitionDate = *d
	}
	if n := p.Int("usefulLife"); n != nil {
		in.UsefulLife = *n
	}
	classes, err := s.Store.ListAssetClasses(ctx, false)
	if err != nil {
		return err
	}
	var class domain.AssetClass
	for _, c := range classes {
		if strings.EqualFold(c.Name, p.Text("className")) || strings.EqualFold(c.Code, p.Text("className")) {
			class = c
		}
	}
	if class.ID == uuid.Nil {
		p.Fail("className", "Kelas aset tidak dikenal")
	}
	in.ClassID = class.ID
	divCode := strings.TrimSpace(p.Text("divisionCode"))
	if divCode == "" {
		divCode = defaultAssetDivision
	}
	divs, err := s.Store.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{})
	if err != nil {
		return err
	}
	for _, d := range divs {
		if strings.EqualFold(d.Code, divCode) {
			in.DivisionID = d.ID
		}
	}
	if in.DivisionID == uuid.Nil {
		p.Fail("divisionCode", "Divisi "+divCode+" tidak ditemukan atau nonaktif")
	}
	if err := p.Err(); err != nil {
		return err
	}
	mapping := map[string]string{"classId": "className", "divisionId": "divisionCode", "acquisitionDate": "acquisitionDate"}
	return remapFields(s.Tx.WithTx(ctx, func(ctx context.Context) error {
		set, err := s.draftOpeningSet(ctx)
		if err != nil {
			return err
		}
		cutover := set.CutoverDate
		asset, err := domain.NewFixedAsset(in, class, &cutover)
		if err != nil {
			return err
		}
		if err := s.checkAssetRefs(ctx, asset); err != nil {
			return err
		}
		if cur, err := s.Store.GetFixedAssetByCode(ctx, asset.Code); err == nil {
			if cur.Status != domain.AssetDraft || cur.DeletedAt != nil {
				return apperr.Validation(map[string]string{"code": "Aset dengan kode ini sudah aktif atau dihapus"})
			}
			if err := cur.Update(in, class, &cutover); err != nil {
				return err
			}
			if err := s.Store.UpdateFixedAsset(ctx, cur, cur.Version, a.UserID); err != nil {
				return err
			}
			asset = cur
			asset.Version++
		} else if !apperr.HasCode(err, apperr.CodeNotFound) {
			return err
		} else if asset.ID, err = s.Store.InsertFixedAsset(ctx, asset, a.UserID); err != nil {
			return err
		} else {
			asset.Version = 1
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionImport, EntityType: "fixed_asset", EntityID: asset.ID, EntityLabel: asset.Code, After: assetSnapshot(asset),
		}); err != nil {
			return err
		}
		return s.activate(ctx, asset, a.UserID)
	}), mapping)
}
