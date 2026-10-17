package service_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/catalog"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
	"balans/internal/warehouse"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type fakeProducts struct {
	mu sync.Mutex
	m  map[uuid.UUID]catalog.Product
}

func (f *fakeProducts) Products(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]catalog.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uuid.UUID]catalog.Product{}
	for _, id := range ids {
		if p, ok := f.m[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakeLocations struct {
	mu sync.Mutex
	m  map[uuid.UUID]warehouse.Location
}

func (f *fakeLocations) Locations(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]warehouse.Location, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uuid.UUID]warehouse.Location{}
	for _, id := range ids {
		if l, ok := f.m[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

func (f *fakeLocations) FindLocations(_ context.Context, q warehouse.LocationQuery) ([]warehouse.Location, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []warehouse.Location
	for _, l := range f.m {
		if q.WarehouseID != nil && (l.WarehouseID == nil || *l.WarehouseID != *q.WarehouseID) {
			continue
		}
		if len(q.Types) > 0 && !slices.Contains(q.Types, l.Type) {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

type fakePeriods struct{ locked *clock.BusinessDate }

func (f *fakePeriods) CheckPeriodOpen(_ context.Context, d clock.BusinessDate) error {
	if f.locked != nil && !d.After(*f.locked) {
		return apperr.New(apperr.CodePeriodLocked)
	}
	return nil
}

type harness struct {
	t        *testing.T
	pool     *pgxpool.Pool
	svc      *service.Service
	clock    *clock.Fixed
	products *fakeProducts
	locs     *fakeLocations
	periods  *fakePeriods
	ctx      context.Context
	reader   context.Context
	fg, inb  uuid.UUID
	ecm      uuid.UUID
}

var today = clock.NewBusinessDate(2026, 10, 22)

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	fixed := clock.NewFixed(time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC))
	h := &harness{
		t: t, pool: pool, clock: fixed,
		products: &fakeProducts{m: map[uuid.UUID]catalog.Product{}},
		locs:     &fakeLocations{m: map[uuid.UUID]warehouse.Location{}},
		periods:  &fakePeriods{},
	}
	h.svc = service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Clock: fixed,
		Products: h.products, Locations: h.locs, Periods: h.periods,
	})
	h.ctx = actor.With(context.Background(), actor.Actor{UserID: uuid.New(), FullName: "Staf Gudang"})
	h.reader = actor.With(context.Background(), actor.Actor{
		UserID: uuid.New(), FullName: "Pembaca", Permissions: map[string]struct{}{service.PermStockRead: {}},
	})
	h.fg = h.location("FG", true, true)
	h.inb = h.location("INB", false, true)
	h.ecm = h.location("ECM", true, true)
	return h
}

func (h *harness) location(code string, sellable, active bool) uuid.UUID {
	id := uuid.New()
	h.locs.mu.Lock()
	defer h.locs.mu.Unlock()
	h.locs.m[id] = warehouse.Location{ID: id, Code: code + "-" + id.String()[:4], Name: code, IsSellable: sellable, IsActive: active}
	return id
}

func (h *harness) product(hasBatch bool) uuid.UUID {
	id := uuid.New()
	shelf := 365
	h.products.mu.Lock()
	defer h.products.mu.Unlock()
	p := catalog.Product{ID: id, SKU: "SKU-" + id.String()[:6], Name: "Produk", UOM: "pcs", HasBatch: hasBatch, IsActive: true}
	if hasBatch {
		p.ShelfLifeDays = &shelf
	}
	h.products.m[id] = p
	return id
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr[T any](v T) *T { return &v }

func src(typ string) domain.Source {
	return domain.Source{Type: typ, ID: uuid.New(), Label: "DOC/1", BusinessDate: today}
}

func at(loc uuid.UUID, s domain.StockStatus) *domain.Place {
	return &domain.Place{LocationID: loc, Status: s}
}

func (h *harness) receive(product, loc uuid.UUID, batch string, expiry *clock.BusinessDate, qty, cost string, st domain.StockStatus) service.PostingResult {
	h.t.Helper()
	l := domain.Line{Kind: domain.KindReceive, ProductID: product, Qty: dec(qty), UnitCost: ptr(dec(cost)), To: at(loc, st)}
	if batch != "" {
		l.Batch = &domain.NewBatch{Number: batch, ExpiryDate: expiry}
	}
	res, err := h.svc.Post(h.ctx, domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{l}})
	require.NoError(h.t, err)
	return res
}

func (h *harness) balance(product, loc uuid.UUID, batch *uuid.UUID, st domain.StockStatus) decimal.Decimal {
	h.t.Helper()
	var q decimal.Decimal
	err := h.pool.QueryRow(context.Background(), `SELECT coalesce(sum(qty), 0) FROM stock_balances
		WHERE product_id = $1 AND location_id = $2 AND batch_id IS NOT DISTINCT FROM $3 AND stock_status = $4`,
		product, loc, batch, string(st)).Scan(&q)
	require.NoError(h.t, err)
	return q
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func exp(days int) *clock.BusinessDate { return ptr(today.AddDays(days)) }

func TestReceiveCreatesBatchLayerAndIsIdempotent(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	prod := today.AddDays(-10)
	s := src("goods_receipt")
	posting := domain.Posting{Source: s, Lines: []domain.Line{{
		Kind: domain.KindReceive, ProductID: p, Qty: dec("10"), UnitCost: ptr(dec("12500.5")), To: at(h.inb, domain.StatusQuarantine),
		Batch: &domain.NewBatch{Number: "b-01", ProductionDate: &prod},
	}}}
	res, err := h.svc.Post(h.ctx, posting)
	require.NoError(t, err)
	require.Len(t, res.Movements, 1)
	mv := res.Movements[0]
	assert.Equal(t, domain.MovementReceive, mv.Type)
	assert.Equal(t, "125005.00", res.Lines[0].CostAmount.StringFixed(2))
	assert.Equal(t, "125005.00", res.TotalCost().StringFixed(2))
	require.NotNil(t, mv.BatchID)
	b, err := h.svc.Batch(h.ctx, *mv.BatchID)
	require.NoError(t, err)
	assert.Equal(t, "B-01", b.Number)
	assert.Equal(t, prod.AddDays(365).String(), b.ExpiryDate.String(), "kedaluwarsa dari umur simpan")
	assert.True(t, h.balance(p, h.inb, mv.BatchID, domain.StatusQuarantine).Equal(dec("10")))

	_, err = h.svc.Post(h.ctx, posting)
	assert.True(t, errors.Is(err, service.ErrAlreadyPosted))

	// Batch yang sama dengan kedaluwarsa berbeda ditolak.
	bad := domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{{
		Kind: domain.KindReceive, ProductID: p, Qty: dec("1"), UnitCost: ptr(dec("1")), To: at(h.inb, domain.StatusQuarantine),
		Batch: &domain.NewBatch{Number: "B-01", ExpiryDate: exp(10)},
	}}}
	_, err = h.svc.Post(h.ctx, bad)
	assert.Equal(t, apperr.CodeValidationFailed, code(err))

	// Penerimaan tanpa HPP memakai biaya cost layer terakhir batch.
	res, err = h.svc.Post(h.ctx, domain.Posting{Source: src("stock_count"), Lines: []domain.Line{{
		Kind: domain.KindReceive, ProductID: p, Qty: dec("2"), BatchID: mv.BatchID, To: at(h.inb, domain.StatusQuarantine),
	}}})
	require.NoError(t, err)
	assert.Equal(t, "12500.5000", res.Movements[0].UnitCost.StringFixed(4))

	eb, err := h.svc.EnsureBatch(h.ctx, service.BatchInput{ProductID: p, Number: "b-01"})
	require.NoError(t, err)
	assert.Equal(t, b.ID, eb.ID)
}

func TestValidationAgainstMasters(t *testing.T) {
	h := setup(t)
	batched := h.product(true)
	plain := h.product(false)
	bundle := uuid.New()
	h.products.m[bundle] = catalog.Product{ID: bundle, SKU: "BDL", IsBundle: true}
	inactive := h.location("OFF", true, false)
	deleted := h.location("DEL", true, true)
	l := h.locs.m[deleted]
	l.Mark = softdelete.Mark{DeletedAt: ptr(time.Now())}
	h.locs.m[deleted] = l

	cost := ptr(dec("1"))
	_, err := h.svc.Post(h.ctx, domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{
		{Kind: domain.KindReceive, ProductID: bundle, Qty: dec("1"), UnitCost: cost, To: at(h.fg, domain.StatusAvailable)},
		{Kind: domain.KindReceive, ProductID: plain, Qty: dec("1"), UnitCost: cost, To: at(h.fg, domain.StatusAvailable), Batch: &domain.NewBatch{Number: "X"}},
		{Kind: domain.KindReceive, ProductID: batched, Qty: dec("1"), UnitCost: cost, To: at(h.fg, domain.StatusAvailable)},
		{Kind: domain.KindReceive, ProductID: uuid.New(), Qty: dec("1"), UnitCost: cost, To: at(h.fg, domain.StatusAvailable)},
		{Kind: domain.KindReceive, ProductID: plain, Qty: dec("1"), UnitCost: cost, To: at(inactive, domain.StatusAvailable)},
		{Kind: domain.KindReceive, ProductID: plain, Qty: dec("1"), UnitCost: cost, To: at(deleted, domain.StatusAvailable)},
		{Kind: domain.KindIssue, ProductID: plain, Qty: dec("1"), From: at(uuid.New(), domain.StatusAvailable)},
		{Kind: domain.KindIssue, ProductID: batched, Qty: dec("1"), BatchID: ptr(uuid.New()), From: at(h.fg, domain.StatusAvailable)},
	}})
	f := apperr.As(err).Fields
	for _, k := range []string{"lines[0].productId", "lines[1].batchId", "lines[2].batchId", "lines[3].productId",
		"lines[4].to.locationId", "lines[5].to.locationId", "lines[6].from.locationId", "lines[7].batchId"} {
		assert.Contains(t, f, k)
	}

	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{
		{Kind: domain.KindReceive, ProductID: plain, Qty: dec("1"), To: at(h.fg, domain.StatusAvailable)},
	}})
	assert.Contains(t, apperr.As(err).Fields, "lines[0].unitCost")

	h.periods.locked = ptr(today)
	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{
		{Kind: domain.KindReceive, ProductID: plain, Qty: dec("1"), UnitCost: cost, To: at(h.fg, domain.StatusAvailable)},
	}})
	assert.Equal(t, apperr.CodePeriodLocked, code(err))

	_, err = h.svc.EnsureBatch(h.ctx, service.BatchInput{ProductID: plain, Number: "X"})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
}

func TestMoveQuarantineToAvailableKeepsLayers(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	r := h.receive(p, h.inb, "Q1", exp(200), "10", "1000", domain.StatusQuarantine)
	batch := r.Movements[0].BatchID
	res, err := h.svc.Post(h.ctx, domain.Posting{Source: src("inbound"), Lines: []domain.Line{{
		Kind: domain.KindMove, ProductID: p, Qty: dec("8"), BatchID: batch, From: at(h.inb, domain.StatusQuarantine), To: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)
	require.Len(t, res.Movements, 2)
	assert.Equal(t, domain.MovementMoveOut, res.Movements[0].Type)
	assert.Equal(t, "-8000.00", res.Movements[0].CostAmount.StringFixed(2))
	assert.Equal(t, "8000.00", res.Movements[1].CostAmount.StringFixed(2))
	assert.Equal(t, "8000.00", res.Lines[0].CostAmount.StringFixed(2))
	assert.True(t, h.balance(p, h.inb, batch, domain.StatusQuarantine).Equal(dec("2")))
	assert.True(t, h.balance(p, h.fg, batch, domain.StatusAvailable).Equal(dec("8")))

	layers, err := h.svc.ListCostLayers(h.reader, service.LayerFilter{ProductID: &p}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, layers.Items, 1)
	assert.True(t, layers.Items[0].RemainingQty.Equal(dec("10")), "pindah status tidak mengonsumsi cost layer")
}

func TestIssueConsumesFIFOWithinBatch(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	r1 := h.receive(p, h.fg, "F1", exp(100), "5", "100", domain.StatusAvailable)
	batch := r1.Movements[0].BatchID
	_, err := h.svc.Post(h.ctx, domain.Posting{Source: src("goods_receipt"), Lines: []domain.Line{{
		Kind: domain.KindReceive, ProductID: p, Qty: dec("10"), BatchID: batch, UnitCost: ptr(dec("120.3333")), To: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)

	res, err := h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("7"), BatchID: batch, From: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)
	mv := res.Movements[0]
	assert.Equal(t, "-740.67", mv.CostAmount.StringFixed(2))
	assert.Equal(t, "740.67", res.Lines[0].CostAmount.StringFixed(2))
	assert.Equal(t, "105.8095", mv.UnitCost.StringFixed(4))
	require.Len(t, mv.Consumptions, 2)
	assert.Equal(t, "740.67", res.IssuedCostByLocation()[h.fg].StringFixed(2))

	layers, err := h.svc.ListCostLayers(h.reader, service.LayerFilter{ProductID: &p}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, layers.Items, 2)
	assert.True(t, layers.Items[0].RemainingQty.IsZero())
	assert.True(t, layers.Items[1].RemainingQty.Equal(dec("8")))
	trace, err := h.svc.CostLayerConsumptions(h.reader, layers.Items[1].ID)
	require.NoError(t, err)
	require.Len(t, trace.Items, 1)
	assert.True(t, trace.Items[0].Qty.Equal(dec("2")))
	assert.Equal(t, "DOC/1", trace.Items[0].SourceLabel)

	open, err := h.svc.ListCostLayers(h.reader, service.LayerFilter{ProductID: &p, OpenOnly: true}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, open.Items, 1)
}

func TestFEFOAllocationAndManualReason(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	h.receive(p, h.fg, "OLD", exp(-1), "50", "90", domain.StatusAvailable)
	near := h.receive(p, h.fg, "NEAR", exp(30), "4", "100", domain.StatusAvailable).Movements[0].BatchID
	far := h.receive(p, h.fg, "FAR", exp(300), "10", "110", domain.StatusAvailable).Movements[0].BatchID
	h.receive(p, h.fg, "QRN", exp(5), "7", "100", domain.StatusQuarantine)

	sug, err := h.svc.SuggestBatchesForUser(h.reader, p, h.fg, dec("6"))
	require.NoError(t, err)
	require.Len(t, sug.Items, 2)
	assert.Equal(t, "NEAR", sug.Items[0].Candidate.BatchNumber)
	assert.True(t, sug.Items[0].SuggestedQty.Equal(dec("4")))
	assert.True(t, sug.Items[1].SuggestedQty.Equal(dec("2")))
	require.Len(t, sug.Expired, 1)
	assert.Equal(t, "OLD", sug.Expired[0].BatchNumber)
	assert.True(t, sug.Quarantine.Equal(dec("7")))
	assert.True(t, sug.Shortfall.IsZero())
	big, err := h.svc.SuggestBatches(h.ctx, p, h.fg, dec("20"))
	require.NoError(t, err)
	assert.True(t, big.Shortfall.Equal(dec("6")))

	// Tanpa batch: FEFO otomatis, batch kedaluwarsa dilewati.
	res, err := h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("6"), From: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)
	require.Len(t, res.Movements, 2)
	assert.Equal(t, *near, *res.Movements[0].BatchID)
	assert.Equal(t, *far, *res.Movements[1].BatchID)
	assert.Equal(t, "620.00", res.Lines[0].CostAmount.StringFixed(2))

	// Batch FAR sekarang saran FEFO (NEAR habis), batch OLD kedaluwarsa bukan saran → wajib alasan.
	old := sug.Expired[0].BatchID
	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("disposal"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("5"), BatchID: old, From: at(h.fg, domain.StatusAvailable),
	}}})
	assert.Contains(t, apperr.As(err).Fields, "lines[0].batchReason")
	res, err = h.svc.Post(h.ctx, domain.Posting{Source: src("disposal"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("5"), BatchID: old, From: at(h.fg, domain.StatusAvailable), BatchReason: "Pemusnahan kedaluwarsa",
	}}})
	require.NoError(t, err)
	var reason string
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT batch_override_reason FROM stock_movements WHERE id = $1`, res.Movements[0].ID).Scan(&reason))
	assert.Equal(t, "Pemusnahan kedaluwarsa", reason)

	// Tidak cukup: tidak ada yang ditulis.
	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("9"), From: at(h.fg, domain.StatusAvailable),
	}}})
	assert.Equal(t, apperr.CodeStockInsufficient, code(err))
	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{
		{Kind: domain.KindIssue, ProductID: p, Qty: dec("1"), BatchID: far, From: at(h.fg, domain.StatusAvailable)},
		{Kind: domain.KindIssue, ProductID: p, Qty: dec("100"), BatchID: far, From: at(h.fg, domain.StatusAvailable)},
	}})
	assert.Equal(t, apperr.CodeStockInsufficient, code(err))
	assert.True(t, h.balance(p, h.fg, far, domain.StatusAvailable).Equal(dec("8")), "posting gagal di-rollback")
}

func TestNonBatchProduct(t *testing.T) {
	h := setup(t)
	p := h.product(false)
	h.receive(p, h.fg, "", nil, "100", "850", domain.StatusAvailable)
	res, err := h.svc.Post(h.ctx, domain.Posting{Source: src("internal_usage"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("30"), From: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)
	assert.Nil(t, res.Movements[0].BatchID)
	assert.Equal(t, "25500.00", res.Lines[0].CostAmount.StringFixed(2))
	assert.True(t, h.balance(p, h.fg, nil, domain.StatusAvailable).Equal(dec("70")))
}

func TestReserveIssueAndRelease(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	b1 := h.receive(p, h.fg, "R1", exp(20), "5", "100", domain.StatusAvailable).Movements[0].BatchID
	b2 := h.receive(p, h.fg, "R2", exp(60), "10", "200", domain.StatusAvailable).Movements[0].BatchID
	so := src("sales_order")
	line := uuid.New()
	expires := h.clock.Now().Add(30 * time.Minute)
	res, err := h.svc.Reserve(h.ctx, service.ReserveRequest{Source: so, ExpiresAt: &expires, Lines: []service.ReserveLine{
		{SourceLineID: &line, ProductID: p, LocationID: h.fg, Qty: dec("8")},
	}})
	require.NoError(t, err)
	require.Len(t, res.Movements, 4)
	assert.True(t, h.balance(p, h.fg, b1, domain.StatusReserved).Equal(dec("5")))
	assert.True(t, h.balance(p, h.fg, b2, domain.StatusReserved).Equal(dec("3")))
	assert.True(t, h.balance(p, h.fg, b2, domain.StatusAvailable).Equal(dec("7")))
	assert.NotNil(t, res.Movements[1].ReservationID)

	// Stok reserved tidak bisa diambil tanpa referensi reservasi, dan tidak bisa melebihi reservasi.
	ref := &domain.ReservationRef{SourceType: so.Type, SourceID: so.ID}
	_, err = h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("9"), From: at(h.fg, domain.StatusReserved), Reservation: ref,
	}}})
	assert.Equal(t, apperr.CodeStockInsufficient, code(err))

	do := src("delivery_order")
	res, err = h.svc.Post(h.ctx, domain.Posting{Source: do, Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("6"), From: at(h.fg, domain.StatusReserved), Reservation: ref,
	}}})
	require.NoError(t, err)
	assert.Equal(t, "700.00", res.Lines[0].CostAmount.StringFixed(2), "5 × 100 + 1 × 200")

	rel, err := h.svc.Release(h.ctx, service.ReleaseRequest{Source: so, Reservation: *ref})
	require.NoError(t, err)
	require.Len(t, rel.Movements, 2)
	assert.True(t, h.balance(p, h.fg, b2, domain.StatusReserved).IsZero())
	assert.True(t, h.balance(p, h.fg, b2, domain.StatusAvailable).Equal(dec("9")))
	var consumed, released decimal.Decimal
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT sum(consumed_qty), sum(released_qty) FROM stock_reservations WHERE source_id = $1`, so.ID).Scan(&consumed, &released))
	assert.True(t, consumed.Equal(dec("6")))
	assert.True(t, released.Equal(dec("2")))

	empty, err := h.svc.Release(h.ctx, service.ReleaseRequest{Source: domain.Source{Type: so.Type, ID: so.ID, Action: "release-2", BusinessDate: today}, Reservation: *ref})
	require.NoError(t, err)
	assert.Empty(t, empty.Movements)
	_, err = h.svc.Release(h.ctx, service.ReleaseRequest{Source: so})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
}

func TestReleaseExpiredReservations(t *testing.T) {
	h := setup(t)
	p := h.product(false)
	h.receive(p, h.ecm, "", nil, "10", "50", domain.StatusAvailable)
	checkout := src("website_checkout")
	expires := h.clock.Now().Add(15 * time.Minute)
	_, err := h.svc.Reserve(h.ctx, service.ReserveRequest{Source: checkout, ExpiresAt: &expires, Lines: []service.ReserveLine{
		{ProductID: p, LocationID: h.ecm, Qty: dec("4")},
	}})
	require.NoError(t, err)
	n, err := h.svc.ReleaseExpired(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n)

	h.clock.Advance(20 * time.Minute)
	n, err = h.svc.ReleaseExpired(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	assert.True(t, h.balance(p, h.ecm, nil, domain.StatusAvailable).Equal(dec("10")))
	assert.True(t, h.balance(p, h.ecm, nil, domain.StatusReserved).IsZero())
}

func TestLedgerIsAppendOnly(t *testing.T) {
	h := setup(t)
	p := h.product(false)
	res := h.receive(p, h.fg, "", nil, "1", "1", domain.StatusAvailable)
	_, err := h.pool.Exec(context.Background(), `UPDATE stock_movements SET qty = 2 WHERE id = $1`, res.Movements[0].ID)
	assert.ErrorContains(t, err, "append-only")
	_, err = h.pool.Exec(context.Background(), `DELETE FROM stock_movements WHERE id = $1`, res.Movements[0].ID)
	assert.ErrorContains(t, err, "append-only")
	_, err = h.pool.Exec(context.Background(), `UPDATE stock_balances SET qty = -1 WHERE product_id = $1`, p)
	assert.ErrorContains(t, err, "ck_stock_balances_qty_non_negative")
}

// TestConcurrentIssuesNeverNegative: 30 transaksi paralel mengambil 5 dari stok 50 → tepat 10 berhasil, saldo
// dan cost layer tepat 0, tidak pernah negatif (S-213).
func TestConcurrentIssuesNeverNegative(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	batch := h.receive(p, h.fg, "CC", exp(100), "50", "1000", domain.StatusAvailable).Movements[0].BatchID

	const workers = 30
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, insufficient := 0, 0
	var other []error
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.svc.Post(h.ctx, domain.Posting{Source: src("delivery_order"), Lines: []domain.Line{{
				Kind: domain.KindIssue, ProductID: p, Qty: dec("5"), From: at(h.fg, domain.StatusAvailable),
			}}})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case code(err) == apperr.CodeStockInsufficient:
				insufficient++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Empty(t, other)
	assert.Equal(t, 10, ok)
	assert.Equal(t, workers-10, insufficient)
	assert.True(t, h.balance(p, h.fg, batch, domain.StatusAvailable).IsZero())
	var remaining, issued decimal.Decimal
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT sum(remaining_qty) FROM cost_layers WHERE product_id = $1`, p).Scan(&remaining))
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT -sum(cost_amount) FROM stock_movements WHERE product_id = $1 AND movement_type = 'issue'`, p).Scan(&issued))
	assert.True(t, remaining.IsZero())
	assert.Equal(t, "50000.00", issued.StringFixed(2))
}

// TestConcurrentCrossLinesNoDeadlock: posting dengan urutan baris berlawanan atas dua produk tidak deadlock
// karena saldo dikunci berurutan, dan total stok tetap konsisten.
func TestConcurrentCrossLinesNoDeadlock(t *testing.T) {
	h := setup(t)
	a, b := h.product(false), h.product(false)
	h.receive(a, h.fg, "", nil, "100", "10", domain.StatusAvailable)
	h.receive(b, h.fg, "", nil, "100", "10", domain.StatusAvailable)
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	start := make(chan struct{})
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			first, second := a, b
			if i%2 == 1 {
				first, second = b, a
			}
			_, err := h.svc.Post(h.ctx, domain.Posting{Source: src("stock_transfer"), Lines: []domain.Line{
				{Kind: domain.KindMove, ProductID: first, Qty: dec("1"), From: at(h.fg, domain.StatusAvailable), To: at(h.ecm, domain.StatusAvailable)},
				{Kind: domain.KindMove, ProductID: second, Qty: dec("1"), From: at(h.fg, domain.StatusAvailable), To: at(h.ecm, domain.StatusAvailable)},
			}})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for _, p := range []uuid.UUID{a, b} {
		assert.True(t, h.balance(p, h.fg, nil, domain.StatusAvailable).Equal(dec("60")))
		assert.True(t, h.balance(p, h.ecm, nil, domain.StatusAvailable).Equal(dec("40")))
	}
}

func TestReadViews(t *testing.T) {
	h := setup(t)
	p := h.product(true)
	c := h.product(false)
	bundle := uuid.New()
	h.products.m[bundle] = catalog.Product{ID: bundle, SKU: "BDL", IsBundle: true, Components: []catalog.BundleComponent{
		{ComponentID: p, Qty: dec("2")}, {ComponentID: c, Qty: dec("1")},
	}}
	h.receive(p, h.fg, "V1", exp(-3), "4", "100", domain.StatusAvailable)
	h.receive(p, h.fg, "V2", exp(40), "10", "100", domain.StatusAvailable)
	h.receive(p, h.inb, "V3", exp(400), "6", "100", domain.StatusQuarantine)
	h.receive(c, h.fg, "", nil, "3", "5", domain.StatusAvailable)
	h.receive(c, h.inb, "", nil, "50", "5", domain.StatusAvailable)

	_, err := h.svc.ListBalances(h.ctx, service.BalanceFilter{}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeForbidden, code(err))

	bal, err := h.svc.ListBalances(h.reader, service.BalanceFilter{ProductID: &p, Statuses: []domain.StockStatus{domain.StatusAvailable}}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, bal.Items, 2)
	assert.True(t, bal.Totals.Qty.Equal(dec("14")))
	assert.True(t, bal.Items[0].AvgUnitCost.Equal(dec("100")))
	assert.NotEmpty(t, bal.Refs.Product(p).SKU)
	_, err = h.svc.ListBalances(h.reader, service.BalanceFilter{Statuses: []domain.StockStatus{"x"}}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeValidationFailed, code(err))

	batches, err := h.svc.ListBatches(h.reader, service.BatchQuery{ProductID: &p, WithStock: true}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, batches.Items, 3)
	assert.Equal(t, "V1", batches.Items[0].Number)
	assert.Equal(t, domain.ExpiryExpired, batches.Items[0].State(batches.Today, batches.NearDays))
	assert.True(t, batches.Items[2].Qty[domain.StatusQuarantine].Equal(dec("6")))
	for state, n := range map[domain.ExpiryState]int{domain.ExpiryExpired: 1, domain.ExpiryNear: 1, domain.ExpiryOK: 1, domain.ExpiryNone: 0} {
		res, err := h.svc.ListBatches(h.reader, service.BatchQuery{ProductID: &p, State: state}, paging.New(nil, nil))
		require.NoError(t, err)
		assert.Len(t, res.Items, n, state)
	}
	_, err = h.svc.ListBatches(h.reader, service.BatchQuery{State: "x"}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeValidationFailed, code(err))

	_, err = h.svc.Post(h.ctx, domain.Posting{Source: domain.Source{Type: "delivery_order", ID: uuid.New(), Label: "DO/1", BusinessDate: today.AddDays(1)}, Lines: []domain.Line{{
		Kind: domain.KindIssue, ProductID: p, Qty: dec("3"), From: at(h.fg, domain.StatusAvailable),
	}}})
	require.NoError(t, err)
	card, err := h.svc.StockCard(h.reader, service.CardFilter{ProductID: p, LocationID: &h.fg, DateFrom: today.AddDays(1), DateTo: today.AddDays(1)}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.True(t, card.Summary.OpeningQty.Equal(dec("14")))
	require.Len(t, card.Lines, 1)
	assert.True(t, card.Lines[0].RunningQty.Equal(dec("11")))
	assert.Equal(t, "1100.00", card.Lines[0].RunningAmount.StringFixed(2))
	assert.Equal(t, "DO/1", card.Lines[0].SourceLabel)
	full, err := h.svc.StockCard(h.reader, service.CardFilter{ProductID: p, DateFrom: today, DateTo: today.AddDays(1)}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, full.Lines, 4)
	assert.True(t, full.Lines[3].RunningQty.Equal(dec("17")))
	_, err = h.svc.StockCard(h.reader, service.CardFilter{DateFrom: today, DateTo: today.AddDays(-1), Status: "x"}, paging.New(nil, nil))
	f := apperr.As(err).Fields
	assert.Contains(t, f, "productId")
	assert.Contains(t, f, "status")
	assert.Contains(t, f, "dateTo")

	// Stok tersedia bundel: hanya lokasi yang bisa dijual (FG); V1 kedaluwarsa tetap dihitung sebagai stok available.
	avail, err := h.svc.BundleAvailabilityForUser(h.reader, []uuid.UUID{bundle, p}, nil)
	require.NoError(t, err)
	require.Len(t, avail, 1)
	// komponen p: 11 di FG → floor(11/2) = 5; komponen c: 3 di FG (INB tidak dijual) → 3.
	assert.Equal(t, "3", avail[0].AvailableQty.String())
	atInb, err := h.svc.BundleAvailability(h.ctx, []uuid.UUID{bundle}, &h.inb)
	require.NoError(t, err)
	assert.True(t, atInb[0].AvailableQty.IsZero())
	none, err := h.svc.BundleAvailability(h.ctx, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
	_, err = h.svc.BundleAvailabilityForUser(h.ctx, []uuid.UUID{bundle}, nil)
	assert.Equal(t, apperr.CodeForbidden, code(err))
}
