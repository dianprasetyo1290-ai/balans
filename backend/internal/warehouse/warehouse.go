// Package warehouse adalah API publik modul gudang. Bagian master (R1.1): gudang fisik dengan lokasi standar
// dan lokasi eksternal per pihak (MD-09, F1-114). Fungsi lookup tidak memeriksa permission (modul pemanggil
// memeriksa permission dokumennya sendiri) dan tetap mengembalikan data terhapus agar dokumen lama bisa
// menampilkannya (Technical Guideline 5.13).
package warehouse

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/warehouse/domain"
	warehousehttp "balans/internal/warehouse/http"
	"balans/internal/warehouse/postgres"
	"balans/internal/warehouse/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "warehouse"

type (
	// Warehouse adalah gudang fisik.
	Warehouse = domain.Warehouse
	// Location adalah lokasi stok internal atau eksternal.
	Location = domain.Location
	// LocationType adalah jenis lokasi.
	LocationType = domain.LocationType
	// PartyType adalah jenis pihak pemilik lokasi eksternal.
	PartyType = domain.PartyType
)

// Jenis lokasi.
const (
	LocationInbound          = domain.LocationInbound
	LocationFinishedGoods    = domain.LocationFinishedGoods
	LocationEcommerce        = domain.LocationEcommerce
	LocationRawMaterial      = domain.LocationRawMaterial
	LocationGoodsMaterial    = domain.LocationGoodsMaterial
	LocationReturns          = domain.LocationReturns
	LocationEvent            = domain.LocationEvent
	LocationTollManufacturer = domain.LocationTollManufacturer
	LocationConsignment      = domain.LocationConsignment
	LocationVendingBuffer    = domain.LocationVendingBuffer
	LocationVendingMachine   = domain.LocationVendingMachine

	PartySupplier       = domain.PartySupplier
	PartyPartner        = domain.PartyPartner
	PartyVendingMachine = domain.PartyVendingMachine
)

// Permission dan jenis entitas audit trail.
const (
	PermWarehouseRead = service.PermWarehouseRead
	PermLocationRead  = service.PermLocationRead
	EntityWarehouse   = service.EntityWarehouse
	EntityLocation    = service.EntityLocation
)

// LocationQuery adalah kriteria pencarian lokasi aktif untuk modul lain. Field kosong tidak difilter.
type LocationQuery struct {
	WarehouseID *uuid.UUID
	Types       []LocationType
	// External true = hanya lokasi eksternal, false = hanya lokasi di dalam gudang.
	External  *bool
	PartyType PartyType
	PartyID   *uuid.UUID
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Responder *httpx.Responder
	Audit     audit.Recorder
}

// Module adalah modul warehouse.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *warehousehttp.WarehouseHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{Store: postgres.NewStore(d.Pool), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock})
	return &Module{svc: svc, rs: d.Responder, h: warehousehttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route gudang dan lokasi.
func (m *Module) RegisterRoutes(r chi.Router) { warehousehttp.Register(r, m.h, m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat (audit.RegisterEntity).
func AuditEntities() map[string]string {
	return map[string]string{EntityWarehouse: PermWarehouseRead, EntityLocation: PermLocationRead}
}

// Warehouse membaca gudang berdasarkan ID, termasuk yang terhapus.
func (m *Module) Warehouse(ctx context.Context, id uuid.UUID) (Warehouse, error) {
	return m.svc.WarehouseByID(ctx, id)
}

// Location membaca lokasi berdasarkan ID, termasuk yang terhapus. NOT_FOUND jika tidak ada.
func (m *Module) Location(ctx context.Context, id uuid.UUID) (Location, error) {
	return m.svc.LocationByID(ctx, id)
}

// Locations membaca banyak lokasi berdasarkan ID (termasuk yang terhapus); ID yang tidak ada tidak ikut di peta.
func (m *Module) Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Location, error) {
	return m.svc.LocationsByIDs(ctx, ids)
}

// FindLocations mengembalikan lokasi aktif yang belum dihapus (maks. 200) menurut gudang, jenis, atau pihak,
// mis. lokasi Inbound gudang tertentu atau lokasi konsinyasi milik satu mitra.
func (m *Module) FindLocations(ctx context.Context, q LocationQuery) ([]Location, error) {
	return m.svc.FindLocations(ctx, service.LocationFilter{
		WarehouseID: q.WarehouseID, Types: q.Types, External: q.External, PartyType: q.PartyType, PartyID: q.PartyID,
	})
}

// SeedDev membuat Gudang Utama beserta lokasi standarnya untuk demo lokal (APP_ENV=local, BRD U-08).
// Aman diulang: gudang yang sudah ada dilewati.
func (m *Module) SeedDev(ctx context.Context) error { return m.svc.SeedDev(ctx) }

// ExternalLocation mengembalikan lokasi eksternal aktif jenis t milik pihak partyID; ok false jika belum ada.
func (m *Module) ExternalLocation(ctx context.Context, t LocationType, partyID uuid.UUID) (Location, bool, error) {
	list, err := m.FindLocations(ctx, LocationQuery{Types: []LocationType{t}, PartyID: &partyID})
	if err != nil || len(list) == 0 {
		return Location{}, false, err
	}
	return list[0], true, nil
}
