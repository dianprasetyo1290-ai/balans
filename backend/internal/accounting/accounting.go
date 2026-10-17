// Package accounting adalah API publik modul accounting (BRD 5.14, Technical Guideline 5.8). Modul sumber
// (procurement, inventory, sales, …) membuat jurnal otomatis HANYA lewat Post dengan PostingRequest berisi kode
// rule, dokumen sumber, nilai bernama, dan dimensi; modul lain dilarang menyusun debit/kredit sendiri.
package accounting

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/accounting/domain"
	accountinghttp "balans/internal/accounting/http"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/tabular"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "accounting"

type (
	// PostingRequest adalah permintaan jurnal otomatis (Technical Guideline 5.8.1).
	PostingRequest = domain.PostingRequest
	// PostingSplit memecah satu nilai bernama ke beberapa akun (mis. saldo awal A-36).
	PostingSplit = domain.PostingSplit
	// ReversalRequest membalik jurnal otomatis dokumen sumber saat dokumen dibatalkan.
	ReversalRequest = service.ReversalRequest
	// Dimensions adalah dimensi jurnal: channel, sub-channel, divisi, kampanye (ACC-17).
	Dimensions = domain.Dimensions
	// Party adalah pihak sub-ledger (pelanggan/pemasok) pada baris jurnal.
	Party = domain.Party
	// Resolver adalah sumber akun baris posting rule yang dipilih dokumen sumber.
	Resolver = domain.Resolver
	// JournalEntry adalah jurnal yang sudah diposting.
	JournalEntry = domain.Entry
	// JournalLine adalah baris jurnal.
	JournalLine = domain.Line
	// Numberer mengambil nomor JV dari modul document.
	Numberer = service.Numberer
	// ExpenseType adalah master jenis biaya (MD-13): akun beban, PPh, wajib bukti/kampanye, approval.
	ExpenseType = domain.ExpenseType
	// WithholdingType adalah jenis PPh yang dipotong pada biaya.
	WithholdingType = domain.WithholdingType
	// DimensionKind adalah jenis dimensi.
	DimensionKind = domain.DimensionKind
)

// Jenis dimensi untuk DimensionValues.
const (
	KindChannel    = domain.KindChannel
	KindSubChannel = domain.KindSubChannel
	KindDivision   = domain.KindDivision
	KindCampaign   = domain.KindCampaign
)

// Jenis PPh jenis biaya dan jenis dokumen approval yang dipakai jenis biaya.
const (
	WithholdingNone  = domain.WithholdingNone
	WithholdingPPh21 = domain.WithholdingPPh21
	WithholdingPPh23 = domain.WithholdingPPh23
	WithholdingPPh42 = domain.WithholdingPPh42

	ApprovalSubjectExpenseClaim           = domain.ApprovalSubjectExpenseClaim
	ApprovalSubjectPettyCashReplenishment = domain.ApprovalSubjectPettyCashReplenishment
)

// Resolver akun yang bisa dikirim di PostingRequest.Accounts (lihat posting_rule_accounts).
const (
	ResolverLocationInventory       = domain.ResolverLocationInventory
	ResolverMaterial                = domain.ResolverMaterial
	ResolverReceivable              = domain.ResolverReceivable
	ResolverPayable                 = domain.ResolverPayable
	ResolverBank                    = domain.ResolverBank
	ResolverExpenseType             = domain.ResolverExpenseType
	ResolverWithholding             = domain.ResolverWithholding
	ResolverPayment                 = domain.ResolverPayment
	ResolverFixedAsset              = domain.ResolverFixedAsset
	ResolverAccumulatedDepreciation = domain.ResolverAccumulatedDepreciation
	ResolverCounter                 = domain.ResolverCounter
	ResolverOpeningBalance          = domain.ResolverOpeningBalance
)

// ErrAlreadyPosted: dokumen sumber sudah dijurnal untuk rule yang sama (errors.Is).
var ErrAlreadyPosted = postgres.ErrAlreadyPosted

// Poster adalah kontrak yang dipakai modul sumber; dipenuhi *Module. Modul pemakai sebaiknya mendefinisikan
// interface sendiri yang lebih sempit dan diisi saat wiring.
type Poster interface {
	Post(ctx context.Context, req PostingRequest) (JournalEntry, error)
	Reverse(ctx context.Context, req ReversalRequest) (JournalEntry, error)
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Numberer  Numberer
}

// Module adalah modul accounting.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *accountinghttp.AccountingHandler
}

var _ Poster = (*Module)(nil)

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Numberer: d.Numberer, Audit: d.Audit, Clock: d.Clock, Log: d.Log,
	})
	return &Module{svc: svc, rs: d.Responder, h: accountinghttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route COA, dimensi, periode, posting rule, jurnal, dan Buku Besar.
func (m *Module) RegisterRoutes(r chi.Router) { accountinghttp.Register(r, m.h, m.rs) }

// RegisterWorkers mendaftarkan worker pembuatan periode.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewEnsurePeriodsWorker(m.svc))
}

// PeriodicJobs menjadwalkan accounting.ensure_fiscal_periods harian 00.05 WIB, juga saat worker mulai.
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(jobqueue.DailyAt{Hour: 0, Minute: 5},
			func() (river.JobArgs, *river.InsertOpts) { return service.EnsurePeriodsArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.EnsurePeriodsArgs{}.Kind(), RunOnStart: true}),
	}
}

// Post membuat dan memposting jurnal otomatis di transaksi aktif. Ditolak bila tidak balance, periode tertutup
// (PERIOD_LOCKED), dimensi wajib kosong/tidak valid, akun induk/nonaktif, atau dokumen sumber sudah dijurnal
// untuk rule yang sama (ErrAlreadyPosted).
func (m *Module) Post(ctx context.Context, req PostingRequest) (JournalEntry, error) {
	return m.svc.Post(ctx, req)
}

// Reverse membalik jurnal otomatis dokumen sumber (pembatalan dokumen) di transaksi aktif.
func (m *Module) Reverse(ctx context.Context, req ReversalRequest) (JournalEntry, error) {
	return m.svc.Reverse(ctx, req)
}

// CheckPeriodOpen menolak tanggal bisnis di periode tertutup dengan PERIOD_LOCKED (Technical Guideline 5.8.4).
func (m *Module) CheckPeriodOpen(ctx context.Context, businessDate clock.BusinessDate) error {
	return m.svc.CheckPeriodOpen(ctx, businessDate)
}

// EnsureUpcomingPeriods membuat periode bulan berjalan dan berikutnya (dipakai job dan seed).
func (m *Module) EnsureUpcomingPeriods(ctx context.Context) (int, error) {
	return m.svc.EnsureUpcomingPeriods(ctx)
}

// ExpenseType membaca jenis biaya (belum dihapus) beserta akun bebannya untuk pengajuan biaya (S-404) dan kas
// kecil (S-405); NOT_FOUND bila tidak ada. Tidak memeriksa permission. Pakai ExpenseType.UsableError sebelum
// dipakai dokumen baru; akun beban dikirim ke Post lewat ResolverExpenseType.
func (m *Module) ExpenseType(ctx context.Context, id uuid.UUID) (ExpenseType, error) {
	return m.svc.ExpenseType(ctx, id)
}

// DimensionValues membaca master dimensi satu jenis yang belum dihapus untuk dropdown dan tampilan dokumen modul
// lain (mis. divisi & kampanye Pemakaian Internal). Tidak memeriksa permission; includeInactive juga
// mengembalikan nilai nonaktif agar dokumen lama tetap bisa menampilkan namanya.
func (m *Module) DimensionValues(ctx context.Context, kind DimensionKind, includeInactive bool) ([]DimensionValue, error) {
	return m.svc.Store.ListDimensions(ctx, kind, postgres.DimensionFilter{IncludeInactive: includeInactive})
}

// ExpenseTypeByCode membaca jenis biaya berdasarkan kode (impor data, seed).
func (m *Module) ExpenseTypeByCode(ctx context.Context, code string) (ExpenseType, error) {
	return m.svc.ExpenseTypeByCode(ctx, code)
}

// DimensionValue adalah satu nilai master dimensi (channel, sub-channel, divisi, kampanye).
type DimensionValue = domain.DimensionValue

// AccountColumns adalah kolom template impor/ekspor Chart of Accounts (S-125).
func AccountColumns() []tabular.Column { return service.AccountColumns }

// ImportAccountRow membuat atau memperbarui akun COA (kunci kode akun) dari satu baris Excel.
func (m *Module) ImportAccountRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportAccountRow(ctx, row)
}

// ExportAccounts mengalirkan Chart of Accounts sebagai baris Excel.
func (m *Module) ExportAccounts(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportAccounts(ctx, emit)
}

// Divisions mengembalikan seluruh divisi (termasuk nonaktif) tanpa cek permission (impor karyawan).
func (m *Module) Divisions(ctx context.Context) ([]DimensionValue, error) {
	return m.svc.Divisions(ctx)
}
