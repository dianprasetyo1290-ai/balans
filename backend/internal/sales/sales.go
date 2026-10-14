// Package sales adalah API publik modul penjualan. Modul lain dan adapter hanya memakai package ini.
package sales

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"

	"balans/internal/platform/httpx"
	"balans/internal/platform/webhook"
	"balans/internal/sales/domain"
	saleshttp "balans/internal/sales/http"
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
	Gateway   PaymentGateway
	Inbox     *webhook.Inbox
	Responder *httpx.Responder
	Log       *slog.Logger
}

// Module adalah modul sales.
type Module struct {
	webhooks *service.PaymentWebhookService
	rs       *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	return &Module{
		webhooks: service.NewPaymentWebhookService(d.Gateway, d.Inbox, d.Log),
		rs:       d.Responder,
	}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route modul.
func (m *Module) RegisterRoutes(r chi.Router) {
	saleshttp.HandlerWithOptions(saleshttp.NewWebhookHandler(m.webhooks, m.rs), saleshttp.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: m.rs.RequestError,
	})
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewPaymentWebhookWorker(m.webhooks))
}

// PeriodicJobs mengembalikan jadwal berkala modul.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }
