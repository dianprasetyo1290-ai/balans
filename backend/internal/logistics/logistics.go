// Package logistics adalah API publik modul pengiriman. Modul lain dan adapter hanya memakai package ini.
//
// R1.3 (LOG-01 – LOG-04, WMS-12): master kurir, Delivery Order (DO) dari Sales Order atau dokumen modul lain,
// release oleh Logistik Leader (nomor DO + Outbound gudang untuk picking/packing/QC), serah ke kurir (resi manual,
// AWB marketplace, kurir internal Merchandiser), konfirmasi diterima dengan foto, pelacakan manual, dan data cetak
// DO/label koli/label produk mitra. DO tidak memutasi stok/jurnal sendiri: dokumen asal mendaftarkan
// DeliveryHandler (Sales Order sudah bawaan: sales.MarkShipped/MarkHandedOver/MarkDelivered, ganti batch QC
// lewat sales.ReallocateBatch). Modul lain (mis. konsinyasi) memakai CreateDeliveryOrder + RegisterDeliveryHandler.
package logistics

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/logistics/domain"
	logisticshttp "balans/internal/logistics/http"
	"balans/internal/logistics/http/deliveryhttp"
	"balans/internal/logistics/postgres"
	"balans/internal/logistics/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/webhook"
)

// Dokumen pengiriman dan dependensi modul lain (adapter di internal/app).
type (
	// DeliveryOrder adalah Delivery Order beserta baris.
	DeliveryOrder = domain.DeliveryOrder
	// DeliveryLine adalah baris DO.
	DeliveryLine = domain.Line
	// DeliveryType adalah jenis DO.
	DeliveryType = domain.DeliveryType
	// DeliveryRequest adalah DO yang dibuat modul lain (Input: jenis, dokumen asal, penerima, lokasi, baris + batch).
	DeliveryRequest = service.DeliveryRequest
	// DeliveryLineInput adalah baris DO baru (SourceLineID = baris dokumen asal yang memegang reservasi).
	DeliveryLineInput = domain.LineInput
	// BatchQty adalah qty satu batch rencana picking.
	BatchQty = domain.BatchQty
	// DeliveryHandler adalah callback dokumen asal DO (stok pengganti QC, kirim, diterima, batal).
	DeliveryHandler = service.DeliveryHandler
	// DeliveryShipped adalah barang DO yang diserahkan ke kurir (qty lulus QC per baris dokumen asal).
	DeliveryShipped = service.DeliveryShipped
	// ShipmentRecord adalah pengiriman satu DO.
	ShipmentRecord = domain.ShipmentRecord

	Warehouse         = service.Warehouse
	Products          = service.Products
	Batches           = service.Batches
	Partners          = service.Partners
	Numberer          = service.Numberer
	Files             = service.Files
	Users             = service.Users
	UserOption        = service.UserOption
	Notifier          = service.Notifier
	SalesOrders       = service.SalesOrders
	SalesOrder        = service.SalesOrder
	SalesAllocation   = service.SalesAllocation
	SalesReallocation = service.SalesReallocation
)

// Jenis DO, permission, entitas audit, dan pemilik file.
const (
	DeliverySales         = domain.DeliverySales
	DeliveryMarketplace   = domain.DeliveryMarketplace
	DeliveryConsignment   = domain.DeliveryConsignment
	DeliveryVendingBuffer = domain.DeliveryVendingBuffer
	DeliveryTransfer      = domain.DeliveryTransfer

	PermDORead             = service.PermDORead
	PermShipmentRead       = service.PermShipmentRead
	PermShipmentPost       = service.PermShipmentPost
	EntityDeliveryOrder    = service.EntityDeliveryOrder
	FileOwnerDeliveryProof = service.FileOwnerDeliveryProof
	// SourceSalesOrder adalah dokumen asal DO dari Sales Order (handler bawaan).
	SourceSalesOrder = service.SourceSalesOrder
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

// Deps adalah dependensi modul. Bagian dokumen (Pool … Notifier) aktif bila Pool diisi; Sales, Partners, Files,
// Users, Notifier opsional.
type Deps struct {
	Provider  ShipmentProvider
	Inbox     *webhook.Inbox
	Responder *httpx.Responder
	Log       *slog.Logger

	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Audit     audit.Recorder
	Clock     clock.Clock
	Warehouse Warehouse
	Products  Products
	Batches   Batches
	Numberer  Numberer
	Sales     SalesOrders
	Partners  Partners
	Files     Files
	Users     Users
	Notifier  Notifier
}

// Module adalah modul logistics.
type Module struct {
	webhooks *service.ShipmentWebhookService
	docs     *service.Service
	rs       *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	m := &Module{
		webhooks: service.NewShipmentWebhookService(d.Provider, d.Inbox, d.Log),
		rs:       d.Responder,
	}
	if d.Pool != nil {
		m.docs = service.New(service.Deps{
			Store: postgres.NewStore(d.Pool), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log, Warehouse: d.Warehouse,
			Products: d.Products, Batches: d.Batches, Numberer: d.Numberer, Sales: d.Sales, Partners: d.Partners,
			Files: d.Files, Users: d.Users, Notifier: d.Notifier,
		})
	}
	return m
}

// DocumentsEnabled: bagian Delivery Order aktif (modul warehouse, inventory, catalog, document aktif).
func (m *Module) DocumentsEnabled() bool { return m.docs != nil }

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route modul.
func (m *Module) RegisterRoutes(r chi.Router) {
	logisticshttp.HandlerWithOptions(logisticshttp.NewWebhookHandler(m.webhooks, m.rs), logisticshttp.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: m.rs.RequestError,
	})
	if m.docs != nil {
		deliveryhttp.Register(r, deliveryhttp.NewHandler(m.docs), m.rs)
	}
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewShipmentWebhookWorker(m.webhooks))
}

// PeriodicJobs mengembalikan jadwal berkala modul.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat.
func AuditEntities() map[string]string {
	return map[string]string{service.EntityDeliveryOrder: service.PermDORead, service.EntityCourier: service.PermShipmentRead}
}

// --- API Delivery Order untuk modul lain (mis. konsinyasi S-303) -----------------------------

// CreateDeliveryOrder membuat DO berstatus Draft di transaksi pemanggil tanpa cek permission. Stok harus sudah
// reserved oleh dokumen asal; Lines[].Batches menjadi picking list Outbound saat Logistik Leader me-release DO.
// SourceType wajib sudah didaftarkan dengan RegisterDeliveryHandler. Satu DO terbuka per dokumen asal.
func (m *Module) CreateDeliveryOrder(ctx context.Context, in DeliveryRequest) (DeliveryOrder, error) {
	return m.docs.CreateDeliveryOrder(ctx, in)
}

// RegisterDeliveryHandler mendaftarkan callback dokumen asal DO (registrasi terakhir menang): Replace (QC Outbound
// gagal → Karantina + stok pengganti), Shipped (wajib; stok keluar & jurnal mis. A-07/A-24), Delivered, Cancelled.
// Dipanggil saat wiring.
func (m *Module) RegisterDeliveryHandler(sourceType string, h DeliveryHandler) {
	m.docs.RegisterHandler(sourceType, h)
}

// DeliveryOrdersBySource membaca DO milik dokumen asal tanpa cek permission.
func (m *Module) DeliveryOrdersBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]DeliveryOrder, error) {
	return m.docs.Store.DeliveryOrdersBySource(ctx, sourceType, sourceID)
}
