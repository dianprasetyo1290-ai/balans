// Package logistics adalah API publik modul pengiriman. Modul lain dan adapter hanya memakai package ini.
package logistics

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"

	"balans/internal/logistics/domain"
	logisticshttp "balans/internal/logistics/http"
	"balans/internal/logistics/service"
	"balans/internal/platform/httpx"
	"balans/internal/platform/webhook"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "logistics"

// Tipe publik untuk adapter kurir.
type (
	ShipmentProvider     = service.ShipmentProvider
	RateRequest          = domain.RateRequest
	Rate                 = domain.Rate
	ShipmentOrder        = domain.ShipmentOrder
	Shipment             = domain.Shipment
	ShipmentStatus       = domain.ShipmentStatus
	ShipmentNotification = domain.ShipmentNotification
)

// Status pengiriman.
const (
	ShipmentStatusConfirmed = domain.ShipmentStatusConfirmed
	ShipmentStatusPickedUp  = domain.ShipmentStatusPickedUp
	ShipmentStatusInTransit = domain.ShipmentStatusInTransit
	ShipmentStatusDelivered = domain.ShipmentStatusDelivered
	ShipmentStatusReturned  = domain.ShipmentStatusReturned
	ShipmentStatusCancelled = domain.ShipmentStatusCancelled
)

// ErrInvalidSignature dikembalikan adapter jika signature webhook tidak valid.
var ErrInvalidSignature = domain.ErrInvalidSignature

// Deps adalah dependensi modul.
type Deps struct {
	Provider  ShipmentProvider
	Inbox     *webhook.Inbox
	Responder *httpx.Responder
	Log       *slog.Logger
}

// Module adalah modul logistics.
type Module struct {
	webhooks *service.ShipmentWebhookService
	rs       *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	return &Module{
		webhooks: service.NewShipmentWebhookService(d.Provider, d.Inbox, d.Log),
		rs:       d.Responder,
	}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route modul.
func (m *Module) RegisterRoutes(r chi.Router) {
	logisticshttp.HandlerWithOptions(logisticshttp.NewWebhookHandler(m.webhooks, m.rs), logisticshttp.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: m.rs.RequestError,
	})
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewShipmentWebhookWorker(m.webhooks))
}

// PeriodicJobs mengembalikan jadwal berkala modul.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }
