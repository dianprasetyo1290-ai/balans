// Package catalog adalah API publik modul master produk: Produk Jadi, Packaging (RM), Material (GM), bundel
// virtual, barcode, dan kategori (MD-01, MD-02, MD-03). Modul lain membaca produk lewat fungsi di sini; fungsi
// lookup tidak memeriksa permission (modul pemanggil memeriksa permission dokumennya sendiri) dan tetap
// mengembalikan produk terhapus agar dokumen lama bisa menampilkannya (Technical Guideline 5.13).
package catalog

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/catalog/domain"
	cataloghttp "balans/internal/catalog/http"
	"balans/internal/catalog/postgres"
	"balans/internal/catalog/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "catalog"

type (
	// Product adalah satu SKU beserta barcode dan komponen bundelnya.
	Product = domain.Product
	// ProductType adalah jenis produk.
	ProductType = domain.ProductType
	// PackagingType adalah tipe kemasan.
	PackagingType = domain.PackagingType
	// Barcode adalah barcode produk.
	Barcode = domain.Barcode
	// BundleComponent adalah komponen bundel virtual beserta qty per bundel.
	BundleComponent = domain.BundleComponent
	// ComponentQty adalah kebutuhan qty satu komponen.
	ComponentQty = domain.ComponentQty
	// OptionFilter adalah kriteria dropdown produk untuk transaksi baru.
	OptionFilter = service.OptionFilter
)

// Jenis produk.
const (
	TypeFinishedGood      = domain.TypeFinishedGood
	TypePackaging         = domain.TypePackaging
	TypeReprocessMaterial = domain.TypeReprocessMaterial
)

// Permission dan jenis entitas audit trail.
const (
	PermProductRead  = service.PermProductRead
	PermProductWrite = service.PermProductWrite
	EntityProduct    = service.EntityProduct
	EntityCategory   = service.EntityCategory
)

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Responder *httpx.Responder
	Audit     audit.Recorder
}

// Module adalah modul catalog.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *cataloghttp.CatalogHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{Store: postgres.NewStore(d.Pool), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock})
	return &Module{svc: svc, rs: d.Responder, h: cataloghttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route produk dan kategori.
func (m *Module) RegisterRoutes(r chi.Router) { cataloghttp.Register(r, m.h, m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat (audit.RegisterEntity).
func AuditEntities() map[string]string {
	return map[string]string{EntityProduct: PermProductRead, EntityCategory: PermProductRead}
}

// Product membaca satu produk berdasarkan ID, termasuk produk terhapus. NOT_FOUND jika tidak ada.
func (m *Module) Product(ctx context.Context, id uuid.UUID) (Product, error) {
	return m.svc.ProductByID(ctx, id)
}

// Products membaca banyak produk berdasarkan ID (termasuk yang terhapus); ID yang tidak ada tidak ikut di peta.
func (m *Module) Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Product, error) {
	return m.svc.ProductsByIDs(ctx, ids)
}

// ProductByCode membaca produk yang belum dihapus berdasarkan SKU atau barcode persis (hasil pindai).
func (m *Module) ProductByCode(ctx context.Context, code string) (Product, error) {
	return m.svc.FindByCode(ctx, code)
}

// ProductOptions mengembalikan produk aktif yang belum dihapus untuk dropdown transaksi baru.
func (m *Module) ProductOptions(ctx context.Context, f OptionFilter) ([]Product, error) {
	return m.svc.ProductOptions(ctx, f)
}

// BundleComponents mengembalikan komponen bundel virtual. Produk bukan bundel mengembalikan slice kosong.
func (m *Module) BundleComponents(ctx context.Context, bundleID uuid.UUID) ([]BundleComponent, error) {
	p, err := m.svc.ProductByID(ctx, bundleID)
	if err != nil {
		return nil, err
	}
	return p.Components, nil
}

// SeedDev membuat kategori dan produk contoh untuk demo lokal (APP_ENV=local). Aman diulang: kode yang sudah
// ada dilewati.
func (m *Module) SeedDev(ctx context.Context) error { return m.svc.SeedDev(ctx) }

// AvailableBundleQty menghitung stok tersedia bundel = min(floor(stok komponen ÷ qty)) dari stok per komponen.
func AvailableBundleQty(components []BundleComponent, componentStock map[uuid.UUID]decimal.Decimal) decimal.Decimal {
	return domain.AvailableBundleQty(components, componentStock)
}

// ExplodeBundle memecah qty bundel menjadi qty per komponen sebelum memanggil inventory (TG 5.7.4).
func ExplodeBundle(components []BundleComponent, qty decimal.Decimal) []ComponentQty {
	return domain.ExplodeBundle(components, qty)
}
