// Package postgres adalah repository ledger stok: batch, posting, movement, saldo, cost layer, dan reservasi.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// ErrAlreadyPosted: aksi dokumen sumber (source_type, source_id, action) sudah pernah memutasi stok.
var ErrAlreadyPosted = errors.New("inventory: dokumen sumber sudah diposting untuk aksi ini")

// Store menyimpan ledger stok.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func nullDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func datePtr(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	v := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &v
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// --- Batch ------------------------------------------------------------------------------------

func toBatch(r sqlcgen.Batch) domain.Batch {
	return domain.Batch{
		ID: r.ID, ProductID: r.ProductID, Number: r.BatchNumber, ProductionDate: datePtr(r.ProductionDate),
		ExpiryDate: datePtr(r.ExpiryDate), CreatedAt: r.CreatedAt,
	}
}

// EnsureBatch membuat batch atau mengembalikan batch yang sudah ada (created false) dengan nomor yang sama.
func (s *Store) EnsureBatch(ctx context.Context, b domain.Batch, actorID uuid.UUID) (domain.Batch, bool, error) {
	r, err := s.q(ctx).InsertBatch(ctx, sqlcgen.InsertBatchParams{
		ProductID: b.ProductID, BatchNumber: b.Number, ProductionDate: nullDate(b.ProductionDate),
		ExpiryDate: nullDate(b.ExpiryDate), ActorID: actorID,
	})
	if err == nil {
		return toBatch(r), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Batch{}, false, fmt.Errorf("insert batch: %w", err)
	}
	r, err = s.q(ctx).GetBatchByNumber(ctx, sqlcgen.GetBatchByNumberParams{ProductID: b.ProductID, BatchNumber: b.Number})
	if err != nil {
		return domain.Batch{}, false, fmt.Errorf("get batch by number: %w", err)
	}
	return toBatch(r), false, nil
}

// GetBatch membaca batch.
func (s *Store) GetBatch(ctx context.Context, id uuid.UUID) (domain.Batch, error) {
	r, err := s.q(ctx).GetBatch(ctx, id)
	if err != nil {
		return domain.Batch{}, notFound(err, "get batch")
	}
	return toBatch(r), nil
}

// BatchesByIDs membaca banyak batch.
func (s *Store) BatchesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Batch, error) {
	out := map[uuid.UUID]domain.Batch{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).ListBatchesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list batches by ids: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = toBatch(r)
	}
	return out, nil
}

// BatchFilter adalah kriteria daftar batch.
type BatchFilter struct {
	ProductID  *uuid.UUID
	Search     string
	ExpiryFrom *clock.BusinessDate
	ExpiryTo   *clock.BusinessDate
	ExpiryOnly bool
	// NoExpiryOnly hanya batch tanpa tanggal kedaluwarsa.
	NoExpiryOnly bool
	WithStock    bool
}

// BatchSummary adalah batch beserta qty per status di semua lokasi.
type BatchSummary struct {
	domain.Batch
	Qty   map[domain.StockStatus]decimal.Decimal
	Total decimal.Decimal
}

// ListBatches mengembalikan satu halaman batch berurutan FEFO.
func (s *Store) ListBatches(ctx context.Context, f BatchFilter, page paging.Page) (paging.Result[BatchSummary], error) {
	p := sqlcgen.ListBatchSummariesParams{
		ProductID: nullUUID(f.ProductID), Search: nullText(f.Search), ExpiryFrom: nullDate(f.ExpiryFrom),
		ExpiryTo: nullDate(f.ExpiryTo), ExpiryOnly: f.ExpiryOnly, NoExpiryOnly: f.NoExpiryOnly, WithStock: f.WithStock,
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	}
	rows, err := s.q(ctx).ListBatchSummaries(ctx, p)
	if err != nil {
		return paging.Result[BatchSummary]{}, fmt.Errorf("list batches: %w", err)
	}
	total, err := s.q(ctx).CountBatchSummaries(ctx, sqlcgen.CountBatchSummariesParams{
		ProductID: p.ProductID, Search: p.Search, ExpiryFrom: p.ExpiryFrom, ExpiryTo: p.ExpiryTo,
		ExpiryOnly: p.ExpiryOnly, NoExpiryOnly: p.NoExpiryOnly, WithStock: p.WithStock,
	})
	if err != nil {
		return paging.Result[BatchSummary]{}, fmt.Errorf("count batches: %w", err)
	}
	items := make([]BatchSummary, len(rows))
	for i, r := range rows {
		items[i] = BatchSummary{
			Batch: domain.Batch{
				ID: r.ID, ProductID: r.ProductID, Number: r.BatchNumber, ProductionDate: datePtr(r.ProductionDate),
				ExpiryDate: datePtr(r.ExpiryDate), CreatedAt: r.CreatedAt,
			},
			Qty: map[domain.StockStatus]decimal.Decimal{
				domain.StatusAvailable: r.QtyAvailable, domain.StatusQuarantine: r.QtyQuarantine,
				domain.StatusInTransit: r.QtyInTransit, domain.StatusReserved: r.QtyReserved,
			},
			Total: r.QtyTotal,
		}
	}
	return paging.Result[BatchSummary]{Items: items, Page: page, Total: total}, nil
}

// --- Posting & movement -----------------------------------------------------------------------

// InsertPosting mencatat header posting; ErrAlreadyPosted bila aksi dokumen sumber sudah ada.
func (s *Store) InsertPosting(ctx context.Context, src domain.Source, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPosting(ctx, sqlcgen.InsertPostingParams{
		SourceType: src.Type, SourceID: src.ID, Action: src.Action, SourceLabel: src.Label,
		BusinessDate: src.BusinessDate, ActorID: actorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("%s %s %s: %w", src.Type, src.ID, src.Action, ErrAlreadyPosted)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert stock posting: %w", err)
	}
	return id, nil
}

// MovementRow adalah movement yang ditulis beserta konteks posting.
type MovementRow struct {
	PostingID    uuid.UUID
	LineNo       int
	Source       domain.Source
	SourceLineID *uuid.UUID
	Movement     domain.Movement
}

// InsertMovement menulis satu baris ledger.
func (s *Store) InsertMovement(ctx context.Context, r MovementRow, actorID uuid.UUID) (uuid.UUID, error) {
	m := r.Movement
	reason := pgtype.Text{}
	if m.BatchReason != "" {
		reason = pgtype.Text{String: m.BatchReason, Valid: true}
	}
	id, err := s.q(ctx).InsertMovement(ctx, sqlcgen.InsertMovementParams{
		PostingID: r.PostingID, LineNo: int32(r.LineNo), SourceType: r.Source.Type, SourceID: r.Source.ID, //nolint:gosec // dibatasi MaxLines
		SourceLineID: nullUUID(r.SourceLineID), BusinessDate: r.Source.BusinessDate, MovementType: string(m.Type),
		ProductID: m.ProductID, LocationID: m.LocationID, BatchID: nullUUID(m.BatchID), StockStatus: string(m.Status),
		Qty: m.Qty, UnitCost: m.UnitCost, CostAmount: m.CostAmount, ReservationID: nullUUID(m.ReservationID),
		BatchOverrideReason: reason, Note: m.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert stock movement: %w", err)
	}
	return id, nil
}

// --- Saldo ------------------------------------------------------------------------------------

// BalanceCandidates mengembalikan stok per batch di satu lokasi + status (tanpa lock) untuk alokasi.
func (s *Store) BalanceCandidates(ctx context.Context, productID, locationID uuid.UUID, status domain.StockStatus) ([]domain.Candidate, error) {
	rows, err := s.q(ctx).BalanceCandidates(ctx, sqlcgen.BalanceCandidatesParams{
		ProductID: productID, LocationID: locationID, StockStatus: string(status),
	})
	if err != nil {
		return nil, fmt.Errorf("balance candidates: %w", err)
	}
	out := make([]domain.Candidate, len(rows))
	for i, r := range rows {
		out[i] = domain.Candidate{
			BatchID: uuidPtr(r.BatchID), BatchNumber: r.BatchNumber.String, ProductionDate: datePtr(r.ProductionDate),
			ExpiryDate: datePtr(r.ExpiryDate), Qty: r.Qty,
		}
	}
	return out, nil
}

// LockedBalance adalah saldo yang sudah dikunci FOR UPDATE.
type LockedBalance struct {
	ID  uuid.UUID
	Qty decimal.Decimal
}

// LockBalance memastikan baris saldo ada lalu menguncinya. Pemanggil WAJIB memanggilnya dalam urutan
// BalanceKey.Compare (domain.NetDeltas) agar tidak deadlock.
func (s *Store) LockBalance(ctx context.Context, k domain.BalanceKey) (LockedBalance, error) {
	q := s.q(ctx)
	if err := q.EnsureBalance(ctx, sqlcgen.EnsureBalanceParams{
		ProductID: k.ProductID, LocationID: k.LocationID, BatchID: nullUUID(k.BatchID), StockStatus: string(k.Status),
	}); err != nil {
		return LockedBalance{}, fmt.Errorf("ensure stock balance: %w", err)
	}
	r, err := q.LockBalance(ctx, sqlcgen.LockBalanceParams{
		ProductID: k.ProductID, LocationID: k.LocationID, BatchID: nullUUID(k.BatchID), StockStatus: string(k.Status),
	})
	if err != nil {
		return LockedBalance{}, fmt.Errorf("lock stock balance: %w", err)
	}
	return LockedBalance{ID: r.ID, Qty: r.Qty}, nil
}

// SetBalanceQty menyimpan qty saldo yang sudah dikunci.
func (s *Store) SetBalanceQty(ctx context.Context, id uuid.UUID, qty decimal.Decimal) error {
	if err := s.q(ctx).SetBalanceQty(ctx, sqlcgen.SetBalanceQtyParams{ID: id, Qty: qty}); err != nil {
		return fmt.Errorf("set stock balance: %w", err)
	}
	return nil
}

// BalanceFilter adalah kriteria Stok per Lokasi.
type BalanceFilter struct {
	ProductID   *uuid.UUID
	LocationIDs []uuid.UUID
	BatchID     *uuid.UUID
	Statuses    []domain.StockStatus
}

// BalanceRow adalah satu saldo > 0 beserta batch dan HPP rata-rata sisa cost layer batch.
type BalanceRow struct {
	Key            domain.BalanceKey
	Qty            decimal.Decimal
	BatchNumber    string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
	AvgUnitCost    decimal.Decimal
	UpdatedAt      time.Time
}

// BalanceTotals adalah ringkasan filter Stok per Lokasi.
type BalanceTotals struct {
	Rows int64
	Qty  decimal.Decimal
}

func statusStrings(list []domain.StockStatus) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = string(s)
	}
	return out
}

// ListBalances mengembalikan satu halaman saldo > 0.
func (s *Store) ListBalances(ctx context.Context, f BalanceFilter, page paging.Page) ([]BalanceRow, BalanceTotals, error) {
	locs := f.LocationIDs
	if locs == nil {
		locs = []uuid.UUID{}
	}
	rows, err := s.q(ctx).ListBalances(ctx, sqlcgen.ListBalancesParams{
		ProductID: nullUUID(f.ProductID), LocationIds: locs, BatchID: nullUUID(f.BatchID),
		Statuses: statusStrings(f.Statuses), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return nil, BalanceTotals{}, fmt.Errorf("list stock balances: %w", err)
	}
	sum, err := s.q(ctx).SummarizeBalances(ctx, sqlcgen.SummarizeBalancesParams{
		ProductID: nullUUID(f.ProductID), LocationIds: locs, BatchID: nullUUID(f.BatchID), Statuses: statusStrings(f.Statuses),
	})
	if err != nil {
		return nil, BalanceTotals{}, fmt.Errorf("summarize stock balances: %w", err)
	}
	out := make([]BalanceRow, len(rows))
	for i, r := range rows {
		out[i] = BalanceRow{
			Key: domain.BalanceKey{
				ProductID: r.ProductID, LocationID: r.LocationID, BatchID: uuidPtr(r.BatchID), Status: domain.StockStatus(r.StockStatus),
			},
			Qty: r.Qty, BatchNumber: r.BatchNumber.String, ProductionDate: datePtr(r.ProductionDate),
			ExpiryDate: datePtr(r.ExpiryDate), AvgUnitCost: r.AvgUnitCost, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, BalanceTotals{Rows: sum.Total, Qty: sum.TotalQty}, nil
}

// StatusQty menjumlahkan qty berstatus tertentu per produk per lokasi.
func (s *Store) StatusQty(ctx context.Context, productIDs []uuid.UUID, status domain.StockStatus, locationID *uuid.UUID) (map[uuid.UUID]map[uuid.UUID]decimal.Decimal, error) {
	out := map[uuid.UUID]map[uuid.UUID]decimal.Decimal{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).StatusQtyByProductLocation(ctx, sqlcgen.StatusQtyByProductLocationParams{
		ProductIds: productIDs, StockStatus: string(status), LocationID: nullUUID(locationID),
	})
	if err != nil {
		return nil, fmt.Errorf("status qty: %w", err)
	}
	for _, r := range rows {
		if out[r.ProductID] == nil {
			out[r.ProductID] = map[uuid.UUID]decimal.Decimal{}
		}
		out[r.ProductID][r.LocationID] = r.Qty
	}
	return out, nil
}

// --- Cost layer -------------------------------------------------------------------------------

// LockLayers mengunci cost layer batch yang masih bersisa dalam urutan FIFO.
func (s *Store) LockLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (domain.LayerSet, error) {
	rows, err := s.q(ctx).LockOpenLayers(ctx, sqlcgen.LockOpenLayersParams{ProductID: productID, BatchID: nullUUID(batchID)})
	if err != nil {
		return domain.LayerSet{}, fmt.Errorf("lock cost layers: %w", err)
	}
	set := domain.LayerSet{Layers: make([]domain.Layer, len(rows))}
	for i, r := range rows {
		set.Layers[i] = domain.Layer{ID: r.ID, Remaining: r.RemainingQty, UnitCost: r.UnitCost}
	}
	return set, nil
}

// LatestUnitCost mengembalikan HPP per unit cost layer terakhir batch; ok false bila belum ada.
func (s *Store) LatestUnitCost(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (decimal.Decimal, bool, error) {
	c, err := s.q(ctx).LatestLayerCost(ctx, sqlcgen.LatestLayerCostParams{ProductID: productID, BatchID: nullUUID(batchID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, false, nil
	}
	if err != nil {
		return decimal.Zero, false, fmt.Errorf("latest layer cost: %w", err)
	}
	return c, true, nil
}

// NewLayer adalah cost layer dari satu movement receive.
type NewLayer struct {
	ProductID  uuid.UUID
	BatchID    *uuid.UUID
	Source     domain.Source
	MovementID uuid.UUID
	Qty        decimal.Decimal
	UnitCost   decimal.Decimal
}

// InsertLayer membuat cost layer.
func (s *Store) InsertLayer(ctx context.Context, l NewLayer) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertLayer(ctx, sqlcgen.InsertLayerParams{
		ProductID: l.ProductID, BatchID: nullUUID(l.BatchID), SourceType: l.Source.Type, SourceID: l.Source.ID,
		SourceLabel: l.Source.Label, MovementID: l.MovementID, Qty: l.Qty, UnitCost: l.UnitCost,
		BusinessDate: l.Source.BusinessDate,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert cost layer: %w", err)
	}
	return id, nil
}

// SetLayerRemaining menyimpan sisa qty cost layer yang sudah dikunci.
func (s *Store) SetLayerRemaining(ctx context.Context, id uuid.UUID, remaining decimal.Decimal) error {
	if err := s.q(ctx).SetLayerRemaining(ctx, sqlcgen.SetLayerRemainingParams{ID: id, RemainingQty: remaining}); err != nil {
		return fmt.Errorf("set cost layer remaining: %w", err)
	}
	return nil
}

// InsertConsumption mencatat jejak konsumsi cost layer.
func (s *Store) InsertConsumption(ctx context.Context, movementID uuid.UUID, c domain.Consumption, actorID uuid.UUID) error {
	if err := s.q(ctx).InsertConsumption(ctx, sqlcgen.InsertConsumptionParams{
		CostLayerID: c.LayerID, StockMovementID: movementID, Qty: c.Qty, UnitCost: c.UnitCost, ActorID: actorID,
	}); err != nil {
		return fmt.Errorf("insert cost layer consumption: %w", err)
	}
	return nil
}

// LayerFilter adalah kriteria HPP per Batch.
type LayerFilter struct {
	ProductID *uuid.UUID
	BatchID   *uuid.UUID
	OpenOnly  bool
}

// LayerRow adalah cost layer untuk tampilan.
type LayerRow struct {
	ID           uuid.UUID
	ProductID    uuid.UUID
	BatchID      *uuid.UUID
	BatchNumber  string
	ExpiryDate   *clock.BusinessDate
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	ReceivedQty  decimal.Decimal
	RemainingQty decimal.Decimal
	UnitCost     decimal.Decimal
	BusinessDate clock.BusinessDate
	CreatedAt    time.Time
}

// ListLayers mengembalikan satu halaman cost layer.
func (s *Store) ListLayers(ctx context.Context, f LayerFilter, page paging.Page) (paging.Result[LayerRow], error) {
	rows, err := s.q(ctx).ListLayers(ctx, sqlcgen.ListLayersParams{
		ProductID: nullUUID(f.ProductID), BatchID: nullUUID(f.BatchID), OpenOnly: f.OpenOnly,
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[LayerRow]{}, fmt.Errorf("list cost layers: %w", err)
	}
	total, err := s.q(ctx).CountLayers(ctx, sqlcgen.CountLayersParams{ProductID: nullUUID(f.ProductID), BatchID: nullUUID(f.BatchID), OpenOnly: f.OpenOnly})
	if err != nil {
		return paging.Result[LayerRow]{}, fmt.Errorf("count cost layers: %w", err)
	}
	items := make([]LayerRow, len(rows))
	for i, r := range rows {
		items[i] = LayerRow{
			ID: r.ID, ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), BatchNumber: r.BatchNumber.String,
			ExpiryDate: datePtr(r.ExpiryDate), SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
			ReceivedQty: r.ReceivedQty, RemainingQty: r.RemainingQty, UnitCost: r.UnitCost, BusinessDate: r.BusinessDate,
			CreatedAt: r.CreatedAt,
		}
	}
	return paging.Result[LayerRow]{Items: items, Page: page, Total: total}, nil
}

// GetLayer membaca satu cost layer.
func (s *Store) GetLayer(ctx context.Context, id uuid.UUID) (LayerRow, error) {
	r, err := s.q(ctx).GetLayer(ctx, id)
	if err != nil {
		return LayerRow{}, notFound(err, "get cost layer")
	}
	return LayerRow{
		ID: r.ID, ProductID: r.ProductID, BatchID: uuidPtr(r.BatchID), SourceType: r.SourceType, SourceID: r.SourceID,
		SourceLabel: r.SourceLabel, ReceivedQty: r.ReceivedQty, RemainingQty: r.RemainingQty, UnitCost: r.UnitCost,
		BusinessDate: r.BusinessDate, CreatedAt: r.CreatedAt,
	}, nil
}

// ConsumptionRow adalah jejak konsumsi satu cost layer oleh movement keluar.
type ConsumptionRow struct {
	ID           uuid.UUID
	MovementID   uuid.UUID
	BusinessDate clock.BusinessDate
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	LocationID   uuid.UUID
	Qty          decimal.Decimal
	UnitCost     decimal.Decimal
	CreatedAt    time.Time
}

// LayerConsumptions mengembalikan jejak konsumsi cost layer.
func (s *Store) LayerConsumptions(ctx context.Context, layerID uuid.UUID) ([]ConsumptionRow, error) {
	rows, err := s.q(ctx).ListLayerConsumptions(ctx, layerID)
	if err != nil {
		return nil, fmt.Errorf("list cost layer consumptions: %w", err)
	}
	out := make([]ConsumptionRow, len(rows))
	for i, r := range rows {
		out[i] = ConsumptionRow{
			ID: r.ID, MovementID: r.MovementID, BusinessDate: r.BusinessDate, SourceType: r.SourceType, SourceID: r.SourceID,
			SourceLabel: r.SourceLabel, LocationID: r.LocationID, Qty: r.Qty, UnitCost: r.UnitCost, CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

// --- Kartu stok -------------------------------------------------------------------------------

// CardFilter adalah kriteria Kartu Stok.
type CardFilter struct {
	ProductID  uuid.UUID
	LocationID *uuid.UUID
	BatchID    *uuid.UUID
	Status     domain.StockStatus
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
}

// CardLine adalah satu baris kartu stok dengan saldo berjalan sejak tanggal awal filter.
type CardLine struct {
	MovementID    uuid.UUID
	BusinessDate  clock.BusinessDate
	CreatedAt     time.Time
	SourceType    string
	SourceID      uuid.UUID
	SourceLabel   string
	Type          domain.MovementType
	LocationID    uuid.UUID
	BatchID       *uuid.UUID
	BatchNumber   string
	Status        domain.StockStatus
	Qty           decimal.Decimal
	UnitCost      decimal.Decimal
	CostAmount    decimal.Decimal
	BatchReason   string
	Note          string
	RunningQty    decimal.Decimal
	RunningAmount decimal.Decimal
}

// CardSummary adalah saldo awal dan mutasi periode kartu stok.
type CardSummary struct {
	Lines         int64
	OpeningQty    decimal.Decimal
	OpeningAmount decimal.Decimal
	QtyIn         decimal.Decimal
	QtyOut        decimal.Decimal
	NetQty        decimal.Decimal
	NetAmount     decimal.Decimal
}

func (f CardFilter) status() pgtype.Text { return nullText(string(f.Status)) }

// StockCard mengembalikan ringkasan dan satu halaman kartu stok.
func (s *Store) StockCard(ctx context.Context, f CardFilter, page paging.Page) (CardSummary, []CardLine, error) {
	q := s.q(ctx)
	open, err := q.StockCardOpening(ctx, sqlcgen.StockCardOpeningParams{
		ProductID: f.ProductID, LocationID: nullUUID(f.LocationID), BatchID: nullUUID(f.BatchID), StockStatus: f.status(),
		DateFrom: f.DateFrom,
	})
	if err != nil {
		return CardSummary{}, nil, fmt.Errorf("stock card opening: %w", err)
	}
	sum, err := q.StockCardSummary(ctx, sqlcgen.StockCardSummaryParams{
		ProductID: f.ProductID, LocationID: nullUUID(f.LocationID), BatchID: nullUUID(f.BatchID), StockStatus: f.status(),
		DateFrom: f.DateFrom, DateTo: f.DateTo,
	})
	if err != nil {
		return CardSummary{}, nil, fmt.Errorf("stock card summary: %w", err)
	}
	rows, err := q.StockCardLines(ctx, sqlcgen.StockCardLinesParams{
		ProductID: f.ProductID, LocationID: nullUUID(f.LocationID), BatchID: nullUUID(f.BatchID), StockStatus: f.status(),
		DateFrom: f.DateFrom, DateTo: f.DateTo, RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return CardSummary{}, nil, fmt.Errorf("stock card lines: %w", err)
	}
	lines := make([]CardLine, len(rows))
	for i, r := range rows {
		lines[i] = CardLine{
			MovementID: r.ID, BusinessDate: r.BusinessDate, CreatedAt: r.CreatedAt, SourceType: r.SourceType,
			SourceID: r.SourceID, SourceLabel: r.SourceLabel, Type: domain.MovementType(r.MovementType),
			LocationID: r.LocationID, BatchID: uuidPtr(r.BatchID), BatchNumber: r.BatchNumber.String,
			Status: domain.StockStatus(r.StockStatus), Qty: r.Qty, UnitCost: r.UnitCost, CostAmount: r.CostAmount,
			BatchReason: r.BatchOverrideReason.String, Note: r.Note,
			RunningQty: open.Qty.Add(r.RunningQty), RunningAmount: open.Amount.Add(r.RunningAmount),
		}
	}
	return CardSummary{
		Lines: sum.Total, OpeningQty: open.Qty, OpeningAmount: open.Amount, QtyIn: sum.QtyIn, QtyOut: sum.QtyOut,
		NetQty: sum.QtyNet, NetAmount: sum.AmountNet,
	}, lines, nil
}

// --- Reservasi --------------------------------------------------------------------------------

// ReservationQuery menyaring reservasi terbuka satu dokumen sumber.
type ReservationQuery struct {
	SourceType   string
	SourceID     uuid.UUID
	SourceLineID *uuid.UUID
	ProductID    *uuid.UUID
	LocationID   *uuid.UUID
}

// OpenReservation adalah reservasi yang masih punya qty terbuka.
type OpenReservation struct {
	ID           uuid.UUID
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	LocationID   uuid.UUID
	ExpiresAt    *time.Time
	Candidate    domain.Candidate
}

// OpenReservations membaca reservasi terbuka (tanpa lock).
func (s *Store) OpenReservations(ctx context.Context, f ReservationQuery) ([]OpenReservation, error) {
	rows, err := s.q(ctx).OpenReservations(ctx, sqlcgen.OpenReservationsParams{
		SourceType: f.SourceType, SourceID: f.SourceID, SourceLineID: nullUUID(f.SourceLineID),
		ProductID: nullUUID(f.ProductID), LocationID: nullUUID(f.LocationID),
	})
	if err != nil {
		return nil, fmt.Errorf("open reservations: %w", err)
	}
	out := make([]OpenReservation, len(rows))
	for i, r := range rows {
		id := r.ID
		out[i] = OpenReservation{
			ID: r.ID, SourceLineID: uuidPtr(r.SourceLineID), ProductID: r.ProductID, LocationID: r.LocationID,
			ExpiresAt: r.ExpiresAt,
			Candidate: domain.Candidate{
				BatchID: uuidPtr(r.BatchID), BatchNumber: r.BatchNumber.String, ProductionDate: datePtr(r.ProductionDate),
				ExpiryDate: datePtr(r.ExpiryDate), Qty: r.OpenQty, ReservationID: &id,
			},
		}
	}
	return out, nil
}

// LockReservations mengunci reservasi (urut id) dan mengembalikan qty terbukanya.
func (s *Store) LockReservations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	out := map[uuid.UUID]decimal.Decimal{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).LockReservations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("lock reservations: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.OpenQty
	}
	return out, nil
}

// NewReservation adalah reservasi yang dibuat movement → reserved.
type NewReservation struct {
	Source       domain.Source
	SourceLineID *uuid.UUID
	Key          domain.BalanceKey
	Qty          decimal.Decimal
	ExpiresAt    *time.Time
}

// InsertReservation membuat reservasi.
func (s *Store) InsertReservation(ctx context.Context, r NewReservation, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertReservation(ctx, sqlcgen.InsertReservationParams{
		SourceType: r.Source.Type, SourceID: r.Source.ID, SourceLineID: nullUUID(r.SourceLineID), ProductID: r.Key.ProductID,
		LocationID: r.Key.LocationID, BatchID: nullUUID(r.Key.BatchID), Qty: r.Qty, ExpiresAt: r.ExpiresAt, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert reservation: %w", err)
	}
	return id, nil
}

// AddReservationUsage menambah qty terpakai/dilepas reservasi yang sudah dikunci.
func (s *Store) AddReservationUsage(ctx context.Context, id uuid.UUID, consumed, released decimal.Decimal, actorID uuid.UUID) error {
	if err := s.q(ctx).AddReservationUsage(ctx, sqlcgen.AddReservationUsageParams{
		ID: id, ConsumedQty: consumed, ReleasedQty: released, ActorID: actorID,
	}); err != nil {
		return fmt.Errorf("update reservation: %w", err)
	}
	return nil
}

// ReservationSource adalah dokumen pemilik reservasi.
type ReservationSource struct {
	SourceType string
	SourceID   uuid.UUID
}

// ExpiredReservationSources mengembalikan dokumen yang punya reservasi terbuka melewati expires_at.
func (s *Store) ExpiredReservationSources(ctx context.Context, now time.Time, limit int32) ([]ReservationSource, error) {
	rows, err := s.q(ctx).ExpiredReservationSources(ctx, sqlcgen.ExpiredReservationSourcesParams{Now: now, RowLimit: limit})
	if err != nil {
		return nil, fmt.Errorf("expired reservations: %w", err)
	}
	out := make([]ReservationSource, len(rows))
	for i, r := range rows {
		out[i] = ReservationSource{SourceType: r.SourceType, SourceID: r.SourceID}
	}
	return out, nil
}

// StockValue adalah qty dan nilai persediaan satu produk di satu lokasi.
type StockValue struct {
	ProductID  uuid.UUID
	LocationID uuid.UUID
	Qty        decimal.Decimal
	Value      decimal.Decimal
}

// StockValues menjumlahkan qty & nilai ledger per produk & lokasi s.d. tanggal (semua status stok).
func (s *Store) StockValues(ctx context.Context, asOf clock.BusinessDate) ([]StockValue, error) {
	rows, err := s.q(ctx).StockValuesAsOf(ctx, asOf)
	if err != nil {
		return nil, fmt.Errorf("stock values: %w", err)
	}
	out := make([]StockValue, len(rows))
	for i, r := range rows {
		out[i] = StockValue{ProductID: r.ProductID, LocationID: r.LocationID, Qty: r.Qty, Value: r.ValueAmount}
	}
	return out, nil
}
