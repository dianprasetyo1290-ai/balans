package accounting

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

// API publik R1.4: aset tetap & penyusutan (S-408), Neraca Saldo (S-410), tutup buku (S-411), saldo awal &
// cut-over (S-412), dan impor/ekspor transaksi (S-413).

type (
	// ApprovalSubject adalah dokumen yang diajukan accounting ke engine approval (buka kembali periode).
	ApprovalSubject = service.ApprovalSubject
	// ApprovalRequester dipasang saat wiring dengan adapter modul approval.
	ApprovalRequester = service.ApprovalRequester
	// PartyLookup mencari pelanggan/mitra/pemasok berdasarkan kode untuk impor piutang/utang terbuka.
	PartyLookup = service.PartyLookup
	// UserDirectory membaca nama user untuk halaman Tutup Buku.
	UserDirectory = service.UserDirectory
	// StockValuer menghitung nilai persediaan per akun untuk rekonsiliasi cut-over.
	StockValuer = service.StockValuer
	// CloseCheckFunc adalah hook pemeriksaan otomatis satu langkah checklist tutup buku.
	CloseCheckFunc = service.CloseCheckFunc
	// CloseCheck adalah hasil satu pemeriksaan otomatis.
	CloseCheck = domain.CloseCheck
	// CloseStepCode adalah kode langkah checklist tutup buku (BRD 5.15).
	CloseStepCode = domain.CloseStepCode
	// OpeningItem adalah piutang/utang terbuka saldo awal per dokumen lama.
	OpeningItem = domain.OpeningItem
	// OpeningItemType adalah jenis saldo terbuka (receivable/payable).
	OpeningItemType = domain.OpeningItemType
	// OpeningItemQuery adalah filter saldo terbuka.
	OpeningItemQuery = postgres.OpeningItemFilter
	// OpeningItemList adalah satu halaman saldo terbuka beserta total sisa.
	OpeningItemList = service.OpeningItemList
	// OpeningSettlement adalah pelunasan saldo terbuka oleh dokumen pembayaran.
	OpeningSettlement = service.OpeningSettlement
	// Reconciliation adalah hasil pengecekan rekonsiliasi cut-over.
	Reconciliation = service.Reconciliation
	// ReconciliationCheck adalah satu baris pengecekan rekonsiliasi.
	ReconciliationCheck = domain.ReconciliationCheck
)

// Langkah checklist tutup buku untuk RegisterCloseCheck.
const (
	StepOperationalDocuments        = domain.StepOperationalDocuments
	StepConsignmentVendingRecon     = domain.StepConsignmentVendingRecon
	StepSettlementReconciliation    = domain.StepSettlementReconciliation
	StepBankPettyCashReconciliation = domain.StepBankPettyCashReconciliation
	StepPayroll                     = domain.StepPayroll
	StepDepreciation                = domain.StepDepreciation
	StepTaxRecap                    = domain.StepTaxRecap
	StepInventoryReview             = domain.StepInventoryReview
	StepFinancialReview             = domain.StepFinancialReview
)

// Jenis saldo terbuka, jenis pihak, dan jenis dokumen approval buka kembali periode.
const (
	OpeningReceivable = domain.OpeningReceivable
	OpeningPayable    = domain.OpeningPayable
	PartyCustomer     = domain.PartyCustomer
	PartyPartner      = domain.PartyPartner
	PartySupplier     = domain.PartySupplier

	ApprovalSubjectFiscalPeriodReopen = domain.ApprovalSubjectFiscalPeriodReopen
	// PermFiscalPeriodRead memberi hak melihat riwayat persetujuan buka kembali periode.
	PermFiscalPeriodRead = service.PermFiscalPeriodRead
	// SourceOpeningBalance adalah source type stok & jurnal saldo awal migrasi.
	SourceOpeningBalance = domain.SourceOpeningBalance
)

// SetApprovals memasang pengaju approval buka kembali periode (dipanggil saat wiring).
func (m *Module) SetApprovals(a ApprovalRequester) { m.svc.SetApprovals(a) }

// SetUserDirectory memasang pembaca nama user (PIC checklist).
func (m *Module) SetUserDirectory(u UserDirectory) { m.svc.SetUserDirectory(u) }

// SetParties memasang pencari pihak untuk impor piutang/utang terbuka.
func (m *Module) SetParties(p PartyLookup) { m.svc.SetParties(p) }

// SetStockValuer memasang penghitung nilai persediaan rekonsiliasi cut-over.
func (m *Module) SetStockValuer(v StockValuer) { m.svc.SetStockValuer(v) }

// RegisterCloseCheck menambah pemeriksaan otomatis ke langkah checklist tutup buku; hasil yang tidak OK
// menahan langkah ditandai Selesai dan menahan Tutup Periode. Modul dokumen (pengadaan, penjualan, gudang)
// mendaftarkan cek "dokumen operasional belum diposting" pada StepOperationalDocuments.
func (m *Module) RegisterCloseCheck(step CloseStepCode, fn CloseCheckFunc) {
	m.svc.RegisterCloseCheck(step, fn)
}

// DecideReopen dipanggil handler approval `fiscal_period_reopen` di transaksi keputusan.
func (m *Module) DecideReopen(ctx context.Context, requestID uuid.UUID, approved bool, decidedBy uuid.UUID, note string) error {
	return m.svc.DecideReopen(ctx, requestID, approved, decidedBy, note)
}

// OpeningItems mengembalikan piutang/utang terbuka saldo awal (tanpa cek permission) agar umur piutang/utang
// dan pelunasan di modul penjualan/pengadaan mencakup saldo migrasi.
func (m *Module) OpeningItems(ctx context.Context, q OpeningItemQuery, page paging.Page) (OpeningItemList, error) {
	return m.svc.OpeningItems(ctx, q, page)
}

// SettleOpeningItem mencatat pelunasan saldo terbuka oleh dokumen pembayaran di transaksi aktif (idempotent per
// dokumen sumber). Jurnal pembayaran tetap diposting modul pembayar lewat Post (A-PB).
func (m *Module) SettleOpeningItem(ctx context.Context, s OpeningSettlement) (OpeningItem, error) {
	return m.svc.SettleOpeningItem(ctx, s)
}

// CutoverReconciliation menjalankan pengecekan rekonsiliasi cut-over (sub-ledger vs GL).
func (m *Module) CutoverReconciliation(ctx context.Context) (Reconciliation, error) {
	return m.svc.CutoverReconciliation(ctx)
}

// EnsureCutoverDate membuat saldo awal dengan tanggal cut-over bila belum ada (cmd/cutover).
func (m *Module) EnsureCutoverDate(ctx context.Context, cutover clock.BusinessDate) error {
	return m.svc.EnsureCutoverDate(ctx, cutover)
}

// CutoverDate mengembalikan tanggal cut-over saldo awal dan apakah sudah diposting; ok false bila belum diisi.
// Tidak memeriksa permission.
func (m *Module) CutoverDate(ctx context.Context) (date clock.BusinessDate, posted bool, ok bool, err error) {
	set, ok, err := m.svc.CurrentOpeningSet(ctx)
	if err != nil || !ok {
		return clock.BusinessDate{}, false, false, err
	}
	return set.CutoverDate, set.Status == domain.OpeningPosted, true, nil
}

// PostOpeningBalances memposting Neraca Saldo Awal (A-36) versi terkini (cmd/cutover -post).
func (m *Module) PostOpeningBalances(ctx context.Context) (journalNumber string, err error) {
	v, err := m.svc.GetOpening(ctx)
	if err != nil {
		return "", err
	}
	if v.Set == nil {
		return "", apperr.WithMessage(apperr.CodeValidationFailed, "Tanggal cut-over saldo awal belum diisi.")
	}
	if v.Set.Status == domain.OpeningPosted {
		return v.Set.JournalNumber, nil
	}
	if v, err = m.svc.PostOpening(ctx, v.Set.Version); err != nil {
		return "", err
	}
	return v.Set.JournalNumber, nil
}

// Depreciate menjalankan penyusutan periode (job, seed, test).
func (m *Module) Depreciate(ctx context.Context, year int, month int) (assets int, total money.Amount, err error) {
	r, err := m.svc.Depreciate(ctx, domain.PeriodKey{Year: year, Month: time.Month(month)})
	return r.Run.AssetCount, r.Run.Total, err
}

// depreciationPeriodicJob menjadwalkan accounting.depreciate tanggal 1 pukul 01.00 WIB untuk bulan sebelumnya.
func depreciationPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(jobqueue.MonthlyAt{Day: 1, Hour: 1, Minute: 0},
		func() (river.JobArgs, *river.InsertOpts) { return service.DepreciateArgs{}, nil },
		&river.PeriodicJobOpts{ID: service.DepreciateArgs{}.Kind()})
}

// --- Impor/ekspor Excel (S-412, S-413) ---------------------------------------------------------------

// Kolom template impor/ekspor transaksi akuntansi.
func OpeningBalanceColumns() []tabular.Column    { return service.OpeningBalanceColumns }
func OpeningReceivableColumns() []tabular.Column { return service.OpeningReceivableColumns }
func OpeningPayableColumns() []tabular.Column    { return service.OpeningPayableColumns }
func OpeningAssetColumns() []tabular.Column      { return service.OpeningAssetColumns }
func JournalImportColumns() []tabular.Column     { return service.JournalImportColumns }
func TrialBalanceColumns() []tabular.Column      { return service.TrialBalanceColumns }
func LedgerColumns() []tabular.Column            { return service.LedgerColumns }

// ImportOpeningBalanceRow menyimpan satu baris Neraca Saldo Awal.
func (m *Module) ImportOpeningBalanceRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportOpeningBalanceRow(ctx, row)
}

// ExportOpeningBalances mengalirkan Neraca Saldo Awal.
func (m *Module) ExportOpeningBalances(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportOpeningBalances(ctx, emit)
}

// ImportOpeningReceivableRow menyimpan satu piutang terbuka.
func (m *Module) ImportOpeningReceivableRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportOpeningReceivableRow(ctx, row)
}

// ImportOpeningPayableRow menyimpan satu utang terbuka.
func (m *Module) ImportOpeningPayableRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportOpeningPayableRow(ctx, row)
}

// ImportOpeningAssetRow membuat aset tetap saldo awal.
func (m *Module) ImportOpeningAssetRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportOpeningAssetRow(ctx, row)
}

// ImportJournalRow menambah satu baris ke draft Jurnal Umum hasil impor; rowKey = dataexchange.RowKey.
func (m *Module) ImportJournalRow(ctx context.Context, rowKey string, row tabular.Row) error {
	return m.svc.ImportJournalRow(ctx, rowKey, row)
}

// ExportTrialBalance mengalirkan Neraca Saldo sesuai parameter ekspor (year+month atau dateFrom+dateTo, dimensi).
func (m *Module) ExportTrialBalance(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportTrialBalance(ctx, params, emit)
}

// ExportLedger mengalirkan Buku Besar detail sesuai parameter ekspor (rentang, accountId opsional, dimensi).
func (m *Module) ExportLedger(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportLedger(ctx, params, emit)
}
