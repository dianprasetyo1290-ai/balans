// Package approval adalah API publik engine approval generik (BRD 5.17, Technical Guideline 5.5).
// Modul dokumen mengajukan dokumennya lewat Request di dalam transaksi bisnis dan mendaftarkan Handler
// yang dipanggil di transaksi yang sama saat dokumen disetujui atau ditolak.
package approval

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/approval/domain"
	approvalhttp "balans/internal/approval/http"
	"balans/internal/approval/postgres"
	"balans/internal/approval/service"
	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "approval"

type (
	// Subject adalah dokumen yang diajukan.
	Subject = domain.Subject
	// SubjectType adalah jenis dokumen.
	SubjectType = domain.SubjectType
	// Request adalah permintaan approval beserta langkahnya.
	Request = domain.Request
	// Principal adalah user beserta role-nya.
	Principal = domain.Principal
	// RoleGrant adalah role yang dipegang user beserta scope divisinya.
	RoleGrant = domain.RoleGrant
	// Handler dipanggil saat dokumen disetujui (level terakhir) atau ditolak.
	Handler = service.Handler
	// Decided adalah keputusan akhir yang diteruskan ke Handler.
	Decided = service.Decided
	// Directory membaca user dan role dari modul identity.
	Directory = service.Directory
	// Notifier mengirim notifikasi in-app dan email.
	Notifier = service.Notifier
)

// Jenis dokumen di matriks approval Fase 1.
const (
	SubjectPurchaseRequest           = domain.SubjectPurchaseRequest
	SubjectPurchaseOrder             = domain.SubjectPurchaseOrder
	SubjectExpenseClaim              = domain.SubjectExpenseClaim
	SubjectInternalUsage             = domain.SubjectInternalUsage
	SubjectStockCount                = domain.SubjectStockCount
	SubjectReturnDisposition         = domain.SubjectReturnDisposition
	SubjectDisposal                  = domain.SubjectDisposal
	SubjectConsignmentReconciliation = domain.SubjectConsignmentReconciliation
	SubjectPriceListVersion          = domain.SubjectPriceListVersion
	SubjectSalesOrderDiscount        = domain.SubjectSalesOrderDiscount
	SubjectSupplierBillException     = domain.SubjectSupplierBillException
	SubjectPurchaseReturn            = domain.SubjectPurchaseReturn
	SubjectPettyCashReplenishment    = domain.SubjectPettyCashReplenishment
	SubjectFiscalPeriodReopen        = domain.SubjectFiscalPeriodReopen

	// ConditionHasDisposal: dokumen memuat baris pemusnahan.
	ConditionHasDisposal = domain.ConditionHasDisposal
)

// Status permintaan.
const (
	StatusPendingApproval = domain.StatusPendingApproval
	StatusApproved        = domain.StatusApproved
	StatusRejected        = domain.StatusRejected
	StatusCancelled       = domain.StatusCancelled
)

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Directory Directory
	Notifier  Notifier
	BaseURL   string
}

// Module adalah modul approval.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *approvalhttp.ApprovalHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Directory: d.Directory, Notifier: d.Notifier, Audit: d.Audit,
		Clock: d.Clock, Log: d.Log, BaseURL: d.BaseURL,
	})
	return &Module{svc: svc, rs: d.Responder, h: approvalhttp.NewHandler(svc, d.Clock)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route inbox, permintaan, kebijakan, dan delegasi.
func (m *Module) RegisterRoutes(r chi.Router) { approvalhttp.Register(r, m.h, m.rs) }

// RegisterWorkers mendaftarkan worker pengingat.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewRemindWorker(m.svc))
}

// PeriodicJobs menjadwalkan approval.remind tiap jam; job sendiri membatasi ke jam kerja (Lampiran B).
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return service.RemindArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.RemindArgs{}.Kind()}),
	}
}

// RegisterHandler didaftarkan modul pemilik dokumen saat wiring. readPermission (mis.
// `procurement.purchase_order.read`) memberi pemegangnya hak melihat riwayat persetujuan dokumen.
func (m *Module) RegisterHandler(t SubjectType, readPermission string, h Handler) {
	m.svc.RegisterHandler(t, readPermission, h)
}

// Request mengajukan dokumen untuk approval di transaksi aktif.
func (m *Module) Request(ctx context.Context, s Subject) (Request, error) {
	return m.svc.Request(ctx, s)
}

// Cancel membatalkan permintaan yang masih berjalan untuk dokumen.
func (m *Module) Cancel(ctx context.Context, t SubjectType, subjectID uuid.UUID) error {
	return m.svc.Cancel(ctx, t, subjectID)
}

// Remind menjalankan pengingat satu kali (dipakai worker dan test).
func (m *Module) Remind(ctx context.Context) (int, error) { return m.svc.Remind(ctx) }

// SubjectTypes mengembalikan jenis dokumen yang dikenal engine.
func SubjectTypes() []SubjectType {
	infos := domain.SubjectTypes()
	out := make([]SubjectType, len(infos))
	for i, t := range infos {
		out[i] = t.Type
	}
	return out
}
