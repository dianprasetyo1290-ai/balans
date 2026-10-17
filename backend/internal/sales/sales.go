// Package sales adalah API publik modul penjualan. Modul lain dan adapter hanya memakai package ini.
package sales

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/webhook"
	"balans/internal/sales/domain"
	saleshttp "balans/internal/sales/http"
	"balans/internal/sales/postgres"
	"balans/internal/sales/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "sales"

// Tipe publik untuk adapter payment gateway.
type (
	PaymentGateway      = service.PaymentGateway
	PaymentRequest      = domain.PaymentRequest
	PaymentTransaction  = domain.PaymentTransaction
	PaymentNotification = domain.PaymentNotification
	PaymentStatus       = domain.PaymentStatus
)

// Master alasan retur (MD-13).
type (
	// ReturnReason adalah alasan retur.
	ReturnReason = domain.ReturnReason
	// ReturnScope adalah dokumen yang memakai alasan retur.
	ReturnScope = domain.ReturnScope
	// Disposition adalah disposisi barang retur.
	Disposition = domain.Disposition
)

// Cakupan alasan retur dan disposisi barang retur.
const (
	ScopeSalesReturn    = domain.ScopeSalesReturn
	ScopePurchaseReturn = domain.ScopePurchaseReturn
	ScopeBoth           = domain.ScopeBoth

	DispositionRestock         = domain.DispositionRestock
	DispositionRepack          = domain.DispositionRepack
	DispositionReprocess       = domain.DispositionReprocess
	DispositionMarketingSample = domain.DispositionMarketingSample
	DispositionDispose         = domain.DispositionDispose
)

// Status pembayaran.
const (
	PaymentStatusPending = domain.PaymentStatusPending
	PaymentStatusPaid    = domain.PaymentStatusPaid
	PaymentStatusFailed  = domain.PaymentStatusFailed
	PaymentStatusExpired = domain.PaymentStatusExpired
)

// ErrInvalidSignature dikembalikan adapter jika signature webhook tidak valid.
var ErrInvalidSignature = domain.ErrInvalidSignature

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Audit     audit.Recorder
	Gateway   PaymentGateway
	Inbox     *webhook.Inbox
	Responder *httpx.Responder
	Log       *slog.Logger
}

// Module adalah modul sales.
type Module struct {
	webhooks *service.PaymentWebhookService
	reasons  *service.ReturnReasonService
	rs       *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	return &Module{
		webhooks: service.NewPaymentWebhookService(d.Gateway, d.Inbox, d.Log),
		reasons:  service.NewReturnReasonService(postgres.NewStore(d.Pool), d.Tx, d.Audit),
		rs:       d.Responder,
	}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route webhook Midtrans dan master alasan retur.
func (m *Module) RegisterRoutes(r chi.Router) {
	saleshttp.Register(r, saleshttp.NewHandler(m.reasons), saleshttp.NewWebhookHandler(m.webhooks, m.rs), m.rs)
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewPaymentWebhookWorker(m.webhooks))
}

// PeriodicJobs mengembalikan jadwal berkala modul.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// ReturnReason membaca alasan retur (belum dihapus) untuk dokumen retur penjualan/pembelian; NOT_FOUND bila
// tidak ada. Tidak memeriksa permission. Pakai ReturnReason.UsableError(scope) sebelum dipakai dokumen baru.
func (m *Module) ReturnReason(ctx context.Context, id uuid.UUID) (ReturnReason, error) {
	return m.reasons.Get(ctx, id)
}

// ReturnReasonByCode membaca alasan retur berdasarkan kode (impor data).
func (m *Module) ReturnReasonByCode(ctx context.Context, code string) (ReturnReason, error) {
	return m.reasons.GetByCode(ctx, code)
}
