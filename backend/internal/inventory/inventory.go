// Package inventory adalah API publik modul ledger stok (INV-01, INV-03, INV-09, WMS-01; Technical Guideline 5.7).
//
// Modul dokumen (Penerimaan Barang, Inbound/QC, Transfer Stok, Outbound, Stock Count, Pemakaian Internal,
// Pemusnahan, DO) memutasi stok HANYA lewat Post/Reserve/Release di transaksi dokumennya sendiri. Satu
// posting = satu aksi dokumen sumber (Source.Type, Source.ID, Source.Action) dan hanya bisa terjadi sekali
// (ErrAlreadyPosted). Baris posting:
//
//   - KindReceive: + qty di To (lokasi + status), membuat cost layer dengan UnitCost (HPP per unit) dan batch
//     (BatchID atau Batch baru dari nomornya).
//   - KindIssue: − qty dari From; batch kosong = dialokasikan FEFO; HPP dihitung FIFO dari cost layer batch
//     yang keluar dan dikembalikan di LineResult.CostAmount / Movement.CostAmount.
//   - KindMove: pindah lokasi dan/atau status dalam batch yang sama (quarantine → available, available →
//     reserved, → in_transit, antar lokasi). Cost layer tidak dikonsumsi; nilai barang yang pindah (FIFO)
//     dikembalikan untuk jurnal antar akun persediaan (mis. A-07, A-24).
//
// Modul inventory tidak memposting jurnal: semua jurnal A-xx dipicu dokumen sumber, sehingga pemanggil
// memakai biaya hasil posting untuk PostingRequest ke accounting.Poster di transaksi yang sama.
package inventory

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	inventoryhttp "balans/internal/inventory/http"
	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "inventory"

type (
	// Posting adalah satu aksi dokumen sumber terhadap stok.
	Posting = domain.Posting
	// Source adalah dokumen sumber mutasi.
	Source = domain.Source
	// Line adalah satu baris posting.
	Line = domain.Line
	// LineKind adalah jenis baris posting.
	LineKind = domain.LineKind
	// Place adalah lokasi + status stok.
	Place = domain.Place
	// NewBatch adalah batch yang dicari/dibuat dari nomornya saat penerimaan.
	NewBatch = domain.NewBatch
	// ReservationRef menunjuk reservasi dokumen yang diambil baris dari stok reserved.
	ReservationRef = domain.ReservationRef
	// StockStatus adalah status stok.
	StockStatus = domain.StockStatus
	// Movement adalah satu baris ledger hasil posting (Qty dan CostAmount bertanda).
	Movement = domain.Movement
	// MovementType adalah jenis baris ledger.
	MovementType = domain.MovementType
	// Consumption adalah konsumsi satu cost layer oleh movement keluar.
	Consumption = domain.Consumption
	// Batch adalah batch produk.
	Batch = domain.Batch
	// BatchInput adalah batch yang dicari/dibuat sebelum posting.
	BatchInput = service.BatchInput
	// PostingResult adalah hasil posting beserta biaya per baris.
	PostingResult = service.PostingResult
	// LineResult adalah hasil satu baris posting.
	LineResult = service.LineResult
	// ReserveRequest adalah permintaan reservasi stok.
	ReserveRequest = service.ReserveRequest
	// ReserveLine adalah satu baris reservasi.
	ReserveLine = service.ReserveLine
	// ReleaseRequest melepas reservasi terbuka.
	ReleaseRequest = service.ReleaseRequest
	// Suggestion adalah saran batch FEFO.
	Suggestion = service.Suggestion
	// BundleAvailability adalah stok tersedia bundel virtual.
	BundleAvailability = service.BundleAvailability
	// ProductDirectory, LocationDirectory, PeriodGuard adalah dependensi ke modul catalog, warehouse, accounting.
	ProductDirectory  = service.ProductDirectory
	LocationDirectory = service.LocationDirectory
	PeriodGuard       = service.PeriodGuard
)

// Jenis baris, status stok, dan jenis movement.
const (
	KindReceive = domain.KindReceive
	KindIssue   = domain.KindIssue
	KindMove    = domain.KindMove

	StatusAvailable  = domain.StatusAvailable
	StatusQuarantine = domain.StatusQuarantine
	StatusInTransit  = domain.StatusInTransit
	StatusReserved   = domain.StatusReserved

	MovementReceive = domain.MovementReceive
	MovementIssue   = domain.MovementIssue
	MovementMoveOut = domain.MovementMoveOut
	MovementMoveIn  = domain.MovementMoveIn

	ActionPost    = domain.ActionPost
	ActionReserve = service.ActionReserve
	ActionRelease = service.ActionRelease
)

// PermStockRead adalah permission halaman Persediaan.
const PermStockRead = service.PermStockRead

// ErrAlreadyPosted: aksi dokumen sumber sudah memutasi stok (errors.Is).
var ErrAlreadyPosted = service.ErrAlreadyPosted

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Products  ProductDirectory
	Locations LocationDirectory
	// Periods opsional (accounting): tanggal bisnis di periode tertutup ditolak PERIOD_LOCKED.
	Periods PeriodGuard
}

// Module adalah modul inventory.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *inventoryhttp.InventoryHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Clock: d.Clock, Log: d.Log,
		Products: d.Products, Locations: d.Locations, Periods: d.Periods,
	})
	return &Module{svc: svc, rs: d.Responder, h: inventoryhttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route halaman Persediaan.
func (m *Module) RegisterRoutes(r chi.Router) { inventoryhttp.Register(r, m.h, m.rs) }

// RegisterWorkers mendaftarkan worker pelepasan reservasi kedaluwarsa.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewExpireReservationsWorker(m.svc))
}

// PeriodicJobs menjadwalkan stock_reservation.expire tiap 1 menit (Lampiran B).
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return service.ExpireReservationsArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.ExpireReservationsArgs{}.Kind()}),
	}
}

// Post memutasi stok di transaksi aktif (atau transaksi baru): validasi, alokasi FEFO untuk baris tanpa
// batch, kunci saldo berurutan (product, location, batch, status), tolak saldo negatif (STOCK_INSUFFICIENT),
// konsumsi cost layer FIFO, tulis ledger append-only. Batch manual dari stok available yang bukan saran FEFO
// wajib Line.BatchReason. Tidak memeriksa permission.
func (m *Module) Post(ctx context.Context, p Posting) (PostingResult, error) {
	return m.svc.Post(ctx, p)
}

// Reserve memindah stok available → reserved (FEFO bila batch kosong) dan mencatat reservasi per baris dokumen.
func (m *Module) Reserve(ctx context.Context, r ReserveRequest) (PostingResult, error) {
	return m.svc.Reserve(ctx, r)
}

// Release melepas seluruh reservasi terbuka dokumen (opsional satu baris) kembali ke available.
func (m *Module) Release(ctx context.Context, r ReleaseRequest) (PostingResult, error) {
	return m.svc.Release(ctx, r)
}

// EnsureBatch mencari atau membuat batch (produk + nomor); kedaluwarsa dihitung dari umur simpan bila kosong.
func (m *Module) EnsureBatch(ctx context.Context, in BatchInput) (Batch, error) {
	return m.svc.EnsureBatch(ctx, in)
}

// Batch membaca batch berdasarkan ID.
func (m *Module) Batch(ctx context.Context, id uuid.UUID) (Batch, error) { return m.svc.Batch(ctx, id) }

// SuggestBatches mengusulkan batch FEFO stok available (tanpa batch kedaluwarsa dan stok Karantina).
func (m *Module) SuggestBatches(ctx context.Context, productID, locationID uuid.UUID, qty decimal.Decimal) (Suggestion, error) {
	return m.svc.SuggestBatches(ctx, productID, locationID, qty)
}

// AvailableQty menjumlahkan stok available per produk di satu lokasi, atau di semua lokasi yang bisa dijual
// bila locationID nil.
func (m *Module) AvailableQty(ctx context.Context, productIDs []uuid.UUID, locationID *uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	return m.svc.AvailableQty(ctx, productIDs, locationID)
}

// BundleAvailability menghitung stok tersedia bundel virtual dari stok komponennya (catalog.AvailableBundleQty).
func (m *Module) BundleAvailability(ctx context.Context, bundleIDs []uuid.UUID, locationID *uuid.UUID) ([]BundleAvailability, error) {
	return m.svc.BundleAvailability(ctx, bundleIDs, locationID)
}

// StockValue adalah qty & nilai persediaan satu produk di satu lokasi.
type StockValue = postgres.StockValue

// StockValues mengembalikan qty & nilai persediaan (Σ HPP ledger) per produk & lokasi s.d. tanggal, tanpa cek
// permission (rekonsiliasi cut-over).
func (m *Module) StockValues(ctx context.Context, asOf clock.BusinessDate) ([]StockValue, error) {
	return m.svc.StockValues(ctx, asOf)
}

// ReleaseExpired melepas reservasi yang melewati expires_at (dipakai job dan test).
func (m *Module) ReleaseExpired(ctx context.Context) (int, error) { return m.svc.ReleaseExpired(ctx) }
