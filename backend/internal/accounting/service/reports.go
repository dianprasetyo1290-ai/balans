package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/shopspring/decimal"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
)

// ReadModelGL adalah nama read model jurnal di read_model_refreshes.
const ReadModelGL = "accounting.gl"

type reportStore interface {
	ListAccounts(ctx context.Context, includeInactive bool) ([]domain.Account, error)
	ReportMovements(ctx context.Context, from, to clock.BusinessDate, d domain.Dimensions) ([]postgres.MovementRow, error)
	ReportDimensionMovements(ctx context.Context, kind domain.DimensionKind, from, to clock.BusinessDate, channelID *uuid.UUID) ([]postgres.DimensionMovementRow, error)
	ReportMonthlyAccounts(ctx context.Context, codes []string, from, to clock.BusinessDate) ([]domain.MonthlyAccount, error)
	ReportMonthlyGroups(ctx context.Context, from, to clock.BusinessDate) ([]postgres.MonthlyGroupRow, error)
	ReportTaxLines(ctx context.Context, codes []string, from, to clock.BusinessDate, limit int32) ([]postgres.TaxLine, error)
	TryReadModelLock(ctx context.Context) (bool, error)
	RefreshGLReadModel(ctx context.Context) error
	SaveReadModelRefresh(ctx context.Context, r postgres.ReadModelRefresh) error
	ListReadModelRefreshes(ctx context.Context) ([]postgres.ReadModelRefresh, error)
}

// ReadModelRefresher me-refresh read model milik modul lain (didaftarkan saat wiring, TG 2.3 butir 3).
type ReadModelRefresher func(ctx context.Context) error

type namedRefresher struct {
	name string
	fn   ReadModelRefresher
}

// ReportService menyusun laporan dari read model dan menjalankan job accounting.refresh_read_models.
type ReportService struct {
	Store  reportStore
	Tx     txRunner
	Clock  clock.Clock
	Labels func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.DimensionValue, error)

	mu         sync.RWMutex
	refreshers []namedRefresher
}

// NewReportService membuat ReportService.
func NewReportService(store reportStore, tx txRunner, c clock.Clock, labels func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.DimensionValue, error)) *ReportService {
	return &ReportService{Store: store, Tx: tx, Clock: c, Labels: labels}
}

// RegisterReadModel mendaftarkan refresher read model modul lain; nama tampil di status read model.
func (s *ReportService) RegisterReadModel(name string, fn ReadModelRefresher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.refreshers {
		if r.name == name {
			s.refreshers[i].fn = fn
			return
		}
	}
	s.refreshers = append(s.refreshers, namedRefresher{name: name, fn: fn})
}

// RefreshReadModels me-refresh seluruh read model (CONCURRENTLY, pembaca tidak terkunci) dalam satu transaksi
// dengan advisory lock: bila refresh lain sedang berjalan, langsung kembali (false). Idempotent.
func (s *ReportService) RefreshReadModels(ctx context.Context) (bool, error) {
	s.mu.RLock()
	list := append([]namedRefresher{{name: ReadModelGL, fn: s.Store.RefreshGLReadModel}}, s.refreshers...)
	s.mu.RUnlock()
	ran := false
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		ok, err := s.Store.TryReadModelLock(ctx)
		if err != nil || !ok {
			return err
		}
		ran = true
		for _, r := range list {
			start := s.Clock.Now()
			if err := r.fn(ctx); err != nil {
				return fmt.Errorf("refresh %s: %w", r.name, err)
			}
			end := s.Clock.Now()
			if err := s.Store.SaveReadModelRefresh(ctx, postgres.ReadModelRefresh{
				Name: r.name, RefreshedAt: end, DurationMs: int(end.Sub(start).Milliseconds()),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return ran, err
}

// RefreshNow me-refresh read model atas permintaan user laporan (tombol Perbarui Data).
func (s *ReportService) RefreshNow(ctx context.Context) ([]postgres.ReadModelRefresh, error) {
	if _, err := actor.RequireAny(ctx, PermReportRead, "inventory.stock.read", "sales.sales_order.read", "sales.invoice.read"); err != nil {
		return nil, err
	}
	if _, err := s.RefreshReadModels(ctx); err != nil {
		return nil, err
	}
	return s.Store.ListReadModelRefreshes(ctx)
}

// ReadModelStatus mengembalikan waktu refresh terakhir semua read model (tanpa cek permission).
func (s *ReportService) ReadModelStatus(ctx context.Context) ([]postgres.ReadModelRefresh, error) {
	return s.Store.ListReadModelRefreshes(ctx)
}

// RefreshedAt mengembalikan waktu refresh terakhir satu read model (nil = belum pernah).
func (s *ReportService) RefreshedAt(ctx context.Context, name string) (*time.Time, error) {
	list, err := s.Store.ListReadModelRefreshes(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.Name == name {
			t := r.RefreshedAt
			return &t, nil
		}
	}
	return nil, nil
}

// --- Laporan keuangan ------------------------------------------------------------------------

// StatementFilter adalah kriteria laporan keuangan. Neraca memakai DateTo sebagai tanggal posisi.
type StatementFilter struct {
	Kind       domain.StatementKind
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	Dimensions domain.Dimensions
}

// StatementView adalah laporan keuangan beserta periode dan waktu data.
type StatementView struct {
	domain.Statement
	DateFrom    clock.BusinessDate
	DateTo      clock.BusinessDate
	RefreshedAt *time.Time
}

func yearStart(d clock.BusinessDate) clock.BusinessDate { return clock.NewBusinessDate(d.Year(), time.January, 1) }

func (s *ReportService) movements(ctx context.Context, from, to clock.BusinessDate, d domain.Dimensions) ([]domain.Movement, map[uuid.UUID]domain.Account, error) {
	accs, err := s.Store.ListAccounts(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[uuid.UUID]domain.Account, len(accs))
	for _, a := range accs {
		byID[a.ID] = a
	}
	rows, err := s.Store.ReportMovements(ctx, from, to, d)
	if err != nil {
		return nil, nil, err
	}
	out := make([]domain.Movement, 0, len(rows))
	for _, r := range rows {
		a, ok := byID[r.AccountID]
		if !ok {
			continue
		}
		out = append(out, domain.Movement{Account: a, Before: r.Before, During: r.During})
	}
	return out, byID, nil
}

func parentOf(byID map[uuid.UUID]domain.Account) domain.ParentLookup {
	return func(a domain.Account) (string, string) {
		if a.ParentID == nil {
			return "", ""
		}
		p, ok := byID[*a.ParentID]
		if !ok || p.ParentID == nil {
			// Induk tingkat kelompok (1000 ASET, …) sudah menjadi judul bagian.
			return "", ""
		}
		return p.Code, p.Name
	}
}

// Statement menyusun Neraca (per DateTo, sejak awal tahun), Laba Rugi, atau Arus Kas periode.
func (s *ReportService) Statement(ctx context.Context, f StatementFilter) (StatementView, error) {
	if _, err := actor.Require(ctx, PermReportRead); err != nil {
		return StatementView{}, err
	}
	if !f.Kind.Valid() {
		return StatementView{}, apperr.Validation(map[string]string{"kind": "Jenis laporan tidak valid"})
	}
	if f.Kind == domain.StatementBalanceSheet {
		if f.DateTo.IsZero() {
			return StatementView{}, apperr.Validation(map[string]string{"dateTo": "Tanggal posisi wajib diisi"})
		}
		f.DateFrom = yearStart(f.DateTo)
		f.Dimensions = domain.Dimensions{}
	} else if err := domain.CheckRange(f.DateFrom, f.DateTo); err != nil {
		return StatementView{}, err
	}
	if f.Kind == domain.StatementCashFlow {
		f.Dimensions = domain.Dimensions{}
	}
	list, byID, err := s.movements(ctx, f.DateFrom, f.DateTo, f.Dimensions)
	if err != nil {
		return StatementView{}, err
	}
	var st domain.Statement
	switch f.Kind {
	case domain.StatementBalanceSheet:
		st = domain.BalanceSheet(list, parentOf(byID))
	case domain.StatementIncomeStatement:
		st = domain.IncomeStatement(list, parentOf(byID))
	default:
		st = domain.CashFlow(list)
	}
	at, err := s.RefreshedAt(ctx, ReadModelGL)
	if err != nil {
		return StatementView{}, err
	}
	return StatementView{Statement: st, DateFrom: f.DateFrom, DateTo: f.DateTo, RefreshedAt: at}, nil
}

// --- Laba rugi per dimensi -------------------------------------------------------------------

// DimensionReportFilter adalah kriteria laba rugi per dimensi.
type DimensionReportFilter struct {
	Kind      domain.DimensionKind
	DateFrom  clock.BusinessDate
	DateTo    clock.BusinessDate
	ChannelID *uuid.UUID
}

// DimensionReport adalah laba rugi per dimensi beserta label kolom.
type DimensionReport struct {
	domain.DimensionProfitLoss
	Kind        domain.DimensionKind
	DateFrom    clock.BusinessDate
	DateTo      clock.BusinessDate
	Labels      map[uuid.UUID]domain.DimensionValue
	RefreshedAt *time.Time
}

// DimensionProfitLoss menyusun laba rugi per channel/sub-channel/divisi/kampanye (RPT-15). Jumlah kolom = laba
// rugi total periode.
func (s *ReportService) DimensionProfitLoss(ctx context.Context, f DimensionReportFilter) (DimensionReport, error) {
	if _, err := actor.Require(ctx, PermReportRead); err != nil {
		return DimensionReport{}, err
	}
	return s.dimensionProfitLoss(ctx, f)
}

func (s *ReportService) dimensionProfitLoss(ctx context.Context, f DimensionReportFilter) (DimensionReport, error) {
	if !f.Kind.Valid() {
		return DimensionReport{}, apperr.Validation(map[string]string{"dimension": "Dimensi tidak valid"})
	}
	if err := domain.CheckRange(f.DateFrom, f.DateTo); err != nil {
		return DimensionReport{}, err
	}
	accs, err := s.Store.ListAccounts(ctx, true)
	if err != nil {
		return DimensionReport{}, err
	}
	byID := make(map[uuid.UUID]domain.Account, len(accs))
	for _, a := range accs {
		byID[a.ID] = a
	}
	rows, err := s.Store.ReportDimensionMovements(ctx, f.Kind, f.DateFrom, f.DateTo, f.ChannelID)
	if err != nil {
		return DimensionReport{}, err
	}
	in := make([]domain.DimensionAmount, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a, ok := byID[r.AccountID]
		if !ok {
			continue
		}
		in = append(in, domain.DimensionAmount{Account: a, DimensionID: r.DimensionID, Net: r.Net})
		if r.DimensionID != uuid.Nil {
			ids = append(ids, r.DimensionID)
		}
	}
	labels := map[uuid.UUID]domain.DimensionValue{}
	if len(ids) > 0 && s.Labels != nil {
		if labels, err = s.Labels(ctx, ids); err != nil {
			return DimensionReport{}, err
		}
	}
	order := sortedByCode(ids, labels)
	at, err := s.RefreshedAt(ctx, ReadModelGL)
	if err != nil {
		return DimensionReport{}, err
	}
	return DimensionReport{
		DimensionProfitLoss: domain.BuildDimensionProfitLoss(in, order), Kind: f.Kind, DateFrom: f.DateFrom,
		DateTo: f.DateTo, Labels: labels, RefreshedAt: at,
	}, nil
}

func sortedByCode(ids []uuid.UUID, labels map[uuid.UUID]domain.DimensionValue) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && labels[out[j]].Code < labels[out[j-1]].Code; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// --- Rekap pajak -----------------------------------------------------------------------------

// TaxReport adalah rekap pajak tahun berjalan beserta rincian jurnal masa terpilih.
type TaxReport struct {
	domain.TaxSummary
	Lines       []postgres.TaxLine
	LineMonth   *clock.BusinessDate
	RefreshedAt *time.Time
}

// maxTaxLines membatasi rincian jurnal pajak satu masa (teknis, bukan nilai bisnis).
const maxTaxLines = 1000

// TaxReport menyusun rekap PPh dipotong, PPh Final, dan pemantauan omzet bruto terhadap Rp4,8 M (RPT-18).
// month (1–12, 0 = tanpa rincian) memuat rincian jurnal akun PPh masa itu.
func (s *ReportService) TaxReport(ctx context.Context, year, month int) (TaxReport, error) {
	if _, err := actor.RequireAny(ctx, PermReportRead, "accounting.withholding.read"); err != nil {
		return TaxReport{}, err
	}
	if year < 2000 || year > 2100 || month < 0 || month > 12 {
		return TaxReport{}, apperr.Validation(map[string]string{"year": "Tahun atau masa tidak valid"})
	}
	from := clock.NewBusinessDate(year, time.January, 1)
	to := clock.NewBusinessDate(year, time.December, 31)
	rows, err := s.Store.ReportMonthlyAccounts(ctx, domain.TaxAccounts, from, to)
	if err != nil {
		return TaxReport{}, err
	}
	out := TaxReport{TaxSummary: domain.BuildTaxSummary(year, rows)}
	if month > 0 {
		mf := clock.NewBusinessDate(year, time.Month(month), 1)
		mt := clock.NewBusinessDate(year, time.Month(month)+1, 1).AddDays(-1)
		codes := []string{domain.AccountPPh21, domain.AccountPPh23, domain.AccountPPh42, domain.AccountPPhFinal}
		if out.Lines, err = s.Store.ReportTaxLines(ctx, codes, mf, mt, maxTaxLines); err != nil {
			return TaxReport{}, err
		}
		out.LineMonth = &mf
	}
	if out.RefreshedAt, err = s.RefreshedAt(ctx, ReadModelGL); err != nil {
		return TaxReport{}, err
	}
	return out, nil
}

// --- Dashboard eksekutif ---------------------------------------------------------------------

// TrendPoint adalah ringkasan laba rugi satu bulan.
type TrendPoint struct {
	Month       clock.BusinessDate
	Revenue     money.Amount
	GrossProfit money.Amount
	NetProfit   money.Amount
}

// ChannelPerformance adalah pendapatan & laba kotor satu channel.
type ChannelPerformance struct {
	ChannelID   uuid.UUID
	Code, Name  string
	Revenue     money.Amount
	GrossProfit money.Amount
	MarginPct   decimal.Decimal
}

// Dashboard adalah dashboard eksekutif dasar (RPT-14): penjualan, margin, stok, kas.
type Dashboard struct {
	DateFrom, DateTo clock.BusinessDate
	Period           domain.ProfitLoss
	Previous         domain.ProfitLoss
	GrossMarginPct   decimal.Decimal
	NetMarginPct     decimal.Decimal
	Cash             money.Amount
	Receivables      money.Amount
	Inventory        money.Amount
	Payables         money.Amount
	Trend            []TrendPoint
	Channels         []ChannelPerformance
	RefreshedAt      *time.Time
}

// Awalan kode akun posisi dashboard (Desain Finance & Accounting bagian 1).
const (
	prefixCash        = "11"
	prefixReceivables = "12"
	prefixInventory   = "13"
	prefixPayables    = "21"
)

// dashboardTrendMonths adalah jumlah bulan tren dashboard (tampilan, bukan nilai bisnis).
const dashboardTrendMonths = 12

// ExecutiveDashboard menyusun dashboard eksekutif dari read model GL (materialized view, di-refresh job).
func (s *ReportService) ExecutiveDashboard(ctx context.Context, from, to clock.BusinessDate) (Dashboard, error) {
	if _, err := actor.Require(ctx, PermReportRead); err != nil {
		return Dashboard{}, err
	}
	if err := domain.CheckRange(from, to); err != nil {
		return Dashboard{}, err
	}
	out := Dashboard{DateFrom: from, DateTo: to}
	list, _, err := s.movements(ctx, from, to, domain.Dimensions{})
	if err != nil {
		return Dashboard{}, err
	}
	out.Cash, out.Receivables, out.Inventory, out.Payables = money.Zero, money.Zero, money.Zero, money.Zero
	for _, m := range list {
		out.Period.AddAccount(m.Account.Group, m.During)
		c := m.Closing()
		switch code := m.Account.Code; {
		case len(code) < 2:
		case code[:2] == prefixCash:
			out.Cash = out.Cash.Add(c)
		case code[:2] == prefixReceivables:
			out.Receivables = out.Receivables.Add(c)
		case code[:2] == prefixInventory:
			out.Inventory = out.Inventory.Add(c)
		case code[:2] == prefixPayables:
			out.Payables = out.Payables.Add(c.Neg())
		}
	}
	days := int(to.Time().Sub(from.Time()).Hours()/24) + 1
	prevTo := from.AddDays(-1)
	prev, _, err := s.movements(ctx, prevTo.AddDays(-(days - 1)), prevTo, domain.Dimensions{})
	if err != nil {
		return Dashboard{}, err
	}
	for _, m := range prev {
		out.Previous.AddAccount(m.Account.Group, m.During)
	}
	out.GrossMarginPct = domain.MarginPercent(out.Period.GrossProfit, out.Period.Revenue)
	out.NetMarginPct = domain.MarginPercent(out.Period.NetProfit, out.Period.Revenue)

	trendFrom := clock.NewBusinessDate(to.Year(), to.Month()-dashboardTrendMonths+1, 1)
	groups, err := s.Store.ReportMonthlyGroups(ctx, trendFrom, to)
	if err != nil {
		return Dashboard{}, err
	}
	byMonth := map[clock.BusinessDate]*domain.ProfitLoss{}
	for _, g := range groups {
		pl, ok := byMonth[g.Month]
		if !ok {
			pl = &domain.ProfitLoss{}
			byMonth[g.Month] = pl
		}
		pl.AddAccount(g.Group, g.Net)
	}
	for i := range dashboardTrendMonths {
		m := clock.NewBusinessDate(trendFrom.Year(), trendFrom.Month()+time.Month(i), 1)
		p := TrendPoint{Month: m, Revenue: money.Zero, GrossProfit: money.Zero, NetProfit: money.Zero}
		if pl, ok := byMonth[m]; ok {
			p.Revenue, p.GrossProfit, p.NetProfit = pl.Revenue, pl.GrossProfit, pl.NetProfit
		}
		out.Trend = append(out.Trend, p)
	}

	dim, err := s.dimensionProfitLoss(ctx, DimensionReportFilter{Kind: domain.KindChannel, DateFrom: from, DateTo: to})
	if err != nil {
		return Dashboard{}, err
	}
	for _, c := range dim.Columns {
		if c.Summary.Revenue.IsZero() && c.Summary.GrossProfit.IsZero() {
			continue
		}
		l := dim.Labels[c.ID]
		name := l.Name
		if c.ID == uuid.Nil {
			name = "Tanpa channel"
		}
		out.Channels = append(out.Channels, ChannelPerformance{
			ChannelID: c.ID, Code: l.Code, Name: name, Revenue: c.Summary.Revenue, GrossProfit: c.Summary.GrossProfit,
			MarginPct: domain.MarginPercent(c.Summary.GrossProfit, c.Summary.Revenue),
		})
	}
	out.RefreshedAt = dim.RefreshedAt
	return out, nil
}

// --- Job -------------------------------------------------------------------------------------

// RefreshReadModelsArgs adalah argumen job accounting.refresh_read_models (Lampiran B).
type RefreshReadModelsArgs struct{}

// Kind adalah nama job.
func (RefreshReadModelsArgs) Kind() string { return "accounting.refresh_read_models" }

// InsertOpts: antrean bulk, maks. 3 percobaan.
func (RefreshReadModelsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: 3}
}

// RefreshReadModelsWorker menjalankan refresh read model.
type RefreshReadModelsWorker struct {
	river.WorkerDefaults[RefreshReadModelsArgs]
	reports *ReportService
}

// NewRefreshReadModelsWorker membuat worker.
func NewRefreshReadModelsWorker(r *ReportService) *RefreshReadModelsWorker {
	return &RefreshReadModelsWorker{reports: r}
}

// Work me-refresh semua read model; idempotent.
func (w *RefreshReadModelsWorker) Work(ctx context.Context, _ *river.Job[RefreshReadModelsArgs]) error {
	if _, err := w.reports.RefreshReadModels(ctx); err != nil {
		return fmt.Errorf("refresh read models: %w", err)
	}
	return nil
}
