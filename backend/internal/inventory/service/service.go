// Package service berisi use case ledger stok: posting mutasi (receive/issue/move), reservasi, saran FEFO,
// HPP FIFO, dan tampilan Persediaan (S-212 – S-215, S-115).
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// PermStockRead adalah permission baca stok (Desain Role & Approval 3.1).
const PermStockRead = "inventory.stock.read"

// ProductDirectory membaca master produk (dipenuhi *catalog.Module).
type ProductDirectory interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]catalog.Product, error)
}

// LocationDirectory membaca master lokasi (dipenuhi *warehouse.Module).
type LocationDirectory interface {
	Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]warehouse.Location, error)
	// FindLocations mencari lokasi aktif, mis. Inbound/FG/GM satu gudang untuk eksekusi dokumen.
	FindLocations(ctx context.Context, q warehouse.LocationQuery) ([]warehouse.Location, error)
}

// PeriodGuard menolak tanggal bisnis di periode tertutup (dipenuhi *accounting.Module). Opsional.
type PeriodGuard interface {
	CheckPeriodOpen(ctx context.Context, businessDate clock.BusinessDate) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	EnsureBatch(ctx context.Context, b domain.Batch, actorID uuid.UUID) (domain.Batch, bool, error)
	GetBatch(ctx context.Context, id uuid.UUID) (domain.Batch, error)
	BatchesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Batch, error)
	ListBatches(ctx context.Context, f postgres.BatchFilter, page paging.Page) (paging.Result[postgres.BatchSummary], error)

	InsertPosting(ctx context.Context, src domain.Source, actorID uuid.UUID) (uuid.UUID, error)
	InsertMovement(ctx context.Context, r postgres.MovementRow, actorID uuid.UUID) (uuid.UUID, error)

	BalanceCandidates(ctx context.Context, productID, locationID uuid.UUID, status domain.StockStatus) ([]domain.Candidate, error)
	LockBalance(ctx context.Context, k domain.BalanceKey) (postgres.LockedBalance, error)
	SetBalanceQty(ctx context.Context, id uuid.UUID, qty decimal.Decimal) error
	ListBalances(ctx context.Context, f postgres.BalanceFilter, page paging.Page) ([]postgres.BalanceRow, postgres.BalanceTotals, error)
	StatusQty(ctx context.Context, productIDs []uuid.UUID, status domain.StockStatus, locationID *uuid.UUID) (map[uuid.UUID]map[uuid.UUID]decimal.Decimal, error)

	LockLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (domain.LayerSet, error)
	LatestUnitCost(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (decimal.Decimal, bool, error)
	InsertLayer(ctx context.Context, l postgres.NewLayer) (uuid.UUID, error)
	SetLayerRemaining(ctx context.Context, id uuid.UUID, remaining decimal.Decimal) error
	InsertConsumption(ctx context.Context, movementID uuid.UUID, c domain.Consumption, actorID uuid.UUID) error
	ListLayers(ctx context.Context, f postgres.LayerFilter, page paging.Page) (paging.Result[postgres.LayerRow], error)
	GetLayer(ctx context.Context, id uuid.UUID) (postgres.LayerRow, error)
	LayerConsumptions(ctx context.Context, layerID uuid.UUID) ([]postgres.ConsumptionRow, error)

	StockCard(ctx context.Context, f postgres.CardFilter, page paging.Page) (postgres.CardSummary, []postgres.CardLine, error)

	OpenReservations(ctx context.Context, f postgres.ReservationQuery) ([]postgres.OpenReservation, error)
	LockReservations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]decimal.Decimal, error)
	InsertReservation(ctx context.Context, r postgres.NewReservation, actorID uuid.UUID) (uuid.UUID, error)
	AddReservationUsage(ctx context.Context, id uuid.UUID, consumed, released decimal.Decimal, actorID uuid.UUID) error
	ExpiredReservationSources(ctx context.Context, now time.Time, limit int32) ([]postgres.ReservationSource, error)
	StockValues(ctx context.Context, asOf clock.BusinessDate) ([]postgres.StockValue, error)

	documentStore
}

var _ store = (*postgres.Store)(nil)

// Deps adalah dependensi Service.
type Deps struct {
	Store     store
	Tx        txRunner
	Clock     clock.Clock
	Log       *slog.Logger
	Products  ProductDirectory
	Locations LocationDirectory
	// Periods opsional; nil = tidak memeriksa periode akuntansi.
	Periods PeriodGuard
	// Allocator strategi alokasi batch; nil = FEFO.
	Allocator domain.BatchAllocator

	// Dependensi dokumen persediaan (Pemakaian Internal, Repacking, Pemusnahan, Disposisi Retur, recost).
	// Semua opsional agar ledger tetap bisa dipakai tanpa modul lain; dokumen yang butuh dependensi yang
	// tidak ada ditolak saat aksi dijalankan.
	Audit         audit.Recorder
	Approvals     Approvals
	Poster        Poster
	Numberer      Numberer
	Dimensions    DimensionDirectory
	ReturnReasons ReturnReasonDirectory
	Files         FileAttacher
	Notifier      Notifier
	Members       RoleMembers
	Jobs          JobEnqueuer
}

// Service adalah use case modul inventory.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service {
	if d.Allocator == nil {
		d.Allocator = domain.FEFO{}
	}
	return &Service{Deps: d}
}

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

// StockValues mengembalikan qty & nilai persediaan per produk & lokasi s.d. tanggal tanpa cek permission
// (rekonsiliasi cut-over di modul accounting).
func (s *Service) StockValues(ctx context.Context, asOf clock.BusinessDate) ([]postgres.StockValue, error) {
	return s.Store.StockValues(ctx, asOf)
}
