package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
	"balans/internal/warehouse"
)

// whHarness menyusun modul lengkap (identity, approval, document, accounting, catalog, warehouse, inventory,
// filestore, party) dengan Postgres asli, gudang baru per test, dan router dengan actor tetap.
type whHarness struct {
	dx    *dxHarness
	t     *testing.T
	app   *App
	wh    uuid.UUID
	loc   map[warehouse.LocationType]uuid.UUID
	cok   uuid.UUID
	van   uuid.UUID
	batch string
}

func newWHHarness(t *testing.T) *whHarness {
	t.Helper()
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Env: config.EnvLocal, EmailProvider: config.ProviderMock, PaymentGateway: config.ProviderMock,
		ShipmentProvider: config.ProviderMock, PublicBaseURL: "http://localhost:5173", FilestoreDir: t.TempDir(),
		EnabledModules: []string{
			"identity", "audit", "notification", "approval", "document", "accounting", "catalog", "warehouse",
			"inventory", "filestore", "party", "hris",
		},
	}
	a := &App{
		Config: cfg, Log: log, Pool: pool, Clock: clock.System{}, Tx: db.NewTxManager(pool),
		Responder: httpx.NewResponder(log, nil), idempotency: idempotency.NewStore(pool),
	}
	var err error
	a.Jobs, err = jobqueue.NewInserter(pool)
	require.NoError(t, err)
	require.NoError(t, a.buildModules())
	ctx := context.Background()
	_, err = a.identity.SeedDevUsers(ctx, "Rahasia-Dev-12345")
	require.NoError(t, err)
	require.NoError(t, a.catalog.SeedDev(ctx))

	dx := &dxHarness{t: t, app: a}
	dx.as("admin@balans.local")
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), dx.actor)))
		})
	})
	a.warehouse.RegisterRoutes(r)
	a.approval.RegisterRoutes(r)
	a.filestore.RegisterRoutes(r)
	dx.srv = httptest.NewServer(r)
	t.Cleanup(dx.srv.Close)

	h := &whHarness{dx: dx, t: t, app: a, loc: map[warehouse.LocationType]uuid.UUID{}}
	code := "T" + strings.ToUpper(uuid.NewString()[:6])
	w := dx.json(http.MethodPost, "/api/v1/warehouses", map[string]any{"code": code, "name": "Gudang " + code, "isActive": true}, http.StatusCreated)
	h.wh = uuid.MustParse(w["id"].(string))
	locs, err := a.warehouse.FindLocations(ctx, warehouse.LocationQuery{WarehouseID: &h.wh})
	require.NoError(t, err)
	for _, l := range locs {
		h.loc[l.Type] = l.ID
	}
	cok, err := a.catalog.ProductByCode(ctx, "MS-COK-30")
	require.NoError(t, err)
	van, err := a.catalog.ProductByCode(ctx, "MS-VAN-30")
	require.NoError(t, err)
	h.cok, h.van, h.batch = cok.ID, van.ID, "B"+code
	return h
}

// receive memposting stok awal (simulasi Penerimaan Barang) dengan batch unik per test.
func (h *whHarness) receive(product uuid.UUID, batch string, qty int64, cost string, loc uuid.UUID, status inventory.StockStatus) {
	h.t.Helper()
	today := clock.Today(h.app.Clock)
	prod := today.AddDays(-10)
	c := decimal.RequireFromString(cost)
	_, err := h.app.inventory.Post(actor.System(context.Background()), inventory.Posting{
		Source: inventory.Source{Type: "test_receipt", ID: uuid.New(), Label: "Uji", BusinessDate: today},
		Lines: []inventory.Line{{
			Kind: inventory.KindReceive, ProductID: product, Qty: decimal.NewFromInt(qty), UnitCost: &c,
			Batch: &inventory.NewBatch{Number: batch, ProductionDate: &prod}, To: &inventory.Place{LocationID: loc, Status: status},
		}},
	})
	require.NoError(h.t, err)
}

func (h *whHarness) qty(product, loc uuid.UUID, status inventory.StockStatus) string {
	h.t.Helper()
	rows, err := h.app.inventory.Balances(context.Background(), inventory.BalanceFilter{
		ProductID: &product, LocationIDs: []uuid.UUID{loc}, Statuses: []inventory.StockStatus{status},
	})
	require.NoError(h.t, err)
	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.Qty)
	}
	return total.StringFixed(3)
}

func (h *whHarness) as(email string) { h.dx.as(email) }

func (h *whHarness) json(method, path string, body any, want int) map[string]any {
	return h.dx.json(method, path, body, want)
}

func lines(m map[string]any) []map[string]any {
	raw := m["lines"].([]any)
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i] = r.(map[string]any)
	}
	return out
}

func TestInboundQC(t *testing.T) {
	h := newWHHarness(t)
	ctx := actor.System(context.Background())
	inb := h.loc[warehouse.LocationInbound]
	h.receive(h.cok, h.batch, 100, "12500", inb, inventory.StatusQuarantine)
	h.receive(h.van, h.batch, 40, "12800", inb, inventory.StatusQuarantine)

	// Stok Karantina tidak bisa dialokasikan.
	avail, err := h.app.inventory.AvailableQty(ctx, []uuid.UUID{h.cok}, &inb)
	require.NoError(t, err)
	assert.True(t, avail[h.cok].IsZero())

	var outcomes []warehouse.InboundOutcome
	h.app.warehouse.RegisterInboundHandler("goods_receipt", func(_ context.Context, o warehouse.InboundOutcome) error {
		outcomes = append(outcomes, o)
		return nil
	})
	sourceID := uuid.New()
	var ib warehouse.Inbound
	require.NoError(t, h.app.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ib, err = h.app.warehouse.CreateInbound(ctx, warehouse.InboundRequest{
			SourceType: "goods_receipt", SourceID: sourceID, SourceLabel: "GR/UJI/1", WarehouseID: &h.wh,
			Lines: []warehouse.InboundLineInput{
				{ProductID: h.cok, Qty: decimal.NewFromInt(100)},
				{ProductID: h.van, Qty: decimal.NewFromInt(40)},
			},
		})
		return err
	}))
	require.True(t, strings.HasPrefix(ib.Number, "IN/"), ib.Number)
	require.Len(t, ib.Lines, 2)
	assert.Equal(t, h.loc[warehouse.LocationFinishedGoods], ib.Lines[0].DestinationLocationID)

	h.as("staf.gudang@balans.local")
	detail := h.json(http.MethodGet, "/api/v1/inbounds/"+ib.ID.String(), nil, http.StatusOK)
	ls := lines(detail)
	assert.Equal(t, []any{"qc"}, ls[0]["allowedActions"])

	allOK := map[string]any{"batch": true, "expiry": true, "condition": true, "seal": true}
	// Lulus seluruhnya wajib semua butir sesuai.
	bad := h.json(http.MethodPost, "/api/v1/inbounds/"+ib.ID.String()+"/lines/"+ls[0]["id"].(string)+"/qc", map[string]any{
		"passedQty": "100", "rejectedQty": "0", "checks": map[string]any{"batch": true, "expiry": true, "condition": false, "seal": true},
	}, http.StatusUnprocessableEntity)
	assert.Contains(t, bad["fields"], "checks")

	h.json(http.MethodPost, "/api/v1/inbounds/"+ib.ID.String()+"/lines/"+ls[0]["id"].(string)+"/qc", map[string]any{
		"passedQty": "100", "rejectedQty": "0", "checks": allOK,
	}, http.StatusOK)
	done := h.json(http.MethodPost, "/api/v1/inbounds/"+ib.ID.String()+"/lines/"+ls[1]["id"].(string)+"/qc", map[string]any{
		"passedQty": "30", "rejectedQty": "10", "checks": map[string]any{"batch": true, "expiry": true, "condition": false, "seal": true},
		"rejectReason": "Kemasan penyok",
	}, http.StatusOK)
	assert.Equal(t, "completed", done["status"])
	assert.Len(t, done["checks"], 3)
	// QC ulang ditolak.
	h.json(http.MethodPost, "/api/v1/inbounds/"+ib.ID.String()+"/lines/"+ls[1]["id"].(string)+"/qc", map[string]any{
		"passedQty": "40", "rejectedQty": "0", "checks": allOK,
	}, http.StatusConflict)

	fg := h.loc[warehouse.LocationFinishedGoods]
	assert.Equal(t, "100.000", h.qty(h.cok, fg, inventory.StatusAvailable))
	assert.Equal(t, "30.000", h.qty(h.van, fg, inventory.StatusAvailable))
	assert.Equal(t, "10.000", h.qty(h.van, inb, inventory.StatusQuarantine), "reject pemasok ditahan di Inbound")
	require.Len(t, outcomes, 2)
	assert.Equal(t, "10", outcomes[1].RejectedQty.String())
	assert.True(t, outcomes[1].Completed)
	assert.Equal(t, inb, *outcomes[1].RejectLocationID)

	// Retur penjualan: barang ditolak pindah ke Gudang Retur (tetap Karantina).
	h.receive(h.cok, h.batch+"R", 5, "12500", inb, inventory.StatusQuarantine)
	var ret warehouse.Inbound
	require.NoError(t, h.app.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ret, err = h.app.warehouse.CreateInbound(ctx, warehouse.InboundRequest{
			SourceType: "sales_return", SourceID: uuid.New(), LocationID: &inb,
			Lines: []warehouse.InboundLineInput{{ProductID: h.cok, Qty: decimal.NewFromInt(5)}},
		})
		return err
	}))
	h.json(http.MethodPost, "/api/v1/inbounds/"+ret.ID.String()+"/lines/"+ret.Lines[0].ID.String()+"/qc", map[string]any{
		"passedQty": "0", "rejectedQty": "5", "checks": allOK, "rejectReason": "Segel rusak",
	}, http.StatusOK)
	assert.Equal(t, "5.000", h.qty(h.cok, h.loc[warehouse.LocationReturns], inventory.StatusQuarantine))

	list := h.json(http.MethodGet, "/api/v1/inbounds?status=completed&q="+ib.Number, nil, http.StatusOK)
	assert.EqualValues(t, 1, list["totalItems"])

	// Merchandiser tidak punya hak QC.
	h.as("merchandiser@balans.local")
	h.json(http.MethodGet, "/api/v1/inbounds/"+ib.ID.String(), nil, http.StatusOK)
}

func TestStockTransfer(t *testing.T) {
	h := newWHHarness(t)
	fg, ecm := h.loc[warehouse.LocationFinishedGoods], h.loc[warehouse.LocationEcommerce]
	h.receive(h.cok, h.batch+"A", 30, "12000", fg, inventory.StatusAvailable)
	h.receive(h.cok, h.batch+"B", 50, "13000", fg, inventory.StatusAvailable)
	today := clock.Today(h.app.Clock).String()

	h.as("staf.gudang@balans.local")
	// Dalam satu gudang: langsung diposting, baris dipecah per batch FEFO.
	tr := h.json(http.MethodPost, "/api/v1/stock-transfers", map[string]any{
		"transferType": "direct", "fromLocationId": fg, "toLocationId": ecm, "businessDate": today,
		"lines": []any{map[string]any{"productId": h.cok, "qty": "40"}},
	}, http.StatusCreated)
	assert.Equal(t, "draft", tr["status"])
	assert.Equal(t, false, tr["requiresShipping"])
	assert.Contains(t, tr["allowedActions"], "post")
	assert.NotContains(t, tr["allowedActions"], "ship")
	id := tr["id"].(string)
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "post", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "completed", tr["status"])
	assert.True(t, strings.HasPrefix(tr["number"].(string), "TRF/"))
	require.Len(t, lines(tr), 2, "dipecah per batch")
	assert.Equal(t, "40.000", h.qty(h.cok, ecm, inventory.StatusAvailable))

	// Antar gudang: Dalam Pengiriman sampai diterima.
	h.as("admin@balans.local")
	code := "U" + strings.ToUpper(uuid.NewString()[:6])
	w2 := h.json(http.MethodPost, "/api/v1/warehouses", map[string]any{"code": code, "name": "Gudang " + code, "isActive": true}, http.StatusCreated)
	w2id := uuid.MustParse(w2["id"].(string))
	locs, err := h.app.warehouse.FindLocations(context.Background(), warehouse.LocationQuery{WarehouseID: &w2id, Types: []warehouse.LocationType{warehouse.LocationFinishedGoods}})
	require.NoError(t, err)
	dest := locs[0].ID
	h.as("staf.gudang@balans.local")
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers", map[string]any{
		"transferType": "direct", "fromLocationId": fg, "toLocationId": dest, "businessDate": today,
		"lines": []any{map[string]any{"productId": h.cok, "qty": "20"}},
	}, http.StatusCreated)
	assert.Equal(t, true, tr["requiresShipping"])
	id = tr["id"].(string)
	h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "post", "version": tr["version"]}, http.StatusConflict)
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "ship", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "in_transit", tr["status"])
	assert.Equal(t, "20.000", h.qty(h.cok, dest, inventory.StatusInTransit))
	transit := h.json(http.MethodGet, "/api/v1/stock-transfers/in-transit?locationId="+dest.String(), nil, http.StatusOK)
	assert.EqualValues(t, 1, transit["totalItems"])

	line := lines(tr)[0]["id"].(string)
	h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{
		"action": "receive", "version": tr["version"], "lines": []any{map[string]any{"lineId": line, "qty": "25"}},
	}, http.StatusUnprocessableEntity)
	tr = h.json(http.MethodGet, "/api/v1/stock-transfers/"+id, nil, http.StatusOK)
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{
		"action": "receive", "version": tr["version"], "lines": []any{map[string]any{"lineId": line, "qty": "18"}},
	}, http.StatusOK)
	assert.Equal(t, "partially_received", tr["status"])
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "complete", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "completed", tr["status"])
	assert.Equal(t, "2.000", lines(tr)[0]["shortageQty"])
	assert.Equal(t, "20.000", h.qty(h.cok, dest, inventory.StatusAvailable))
	assert.Equal(t, "0.000", h.qty(h.cok, dest, inventory.StatusInTransit))

	// API publik untuk DO konsinyasi: ke lokasi eksternal → Dalam Pengiriman + nilai FIFO.
	h.as("admin@balans.local")
	partner := uuid.New()
	cons := h.json(http.MethodPost, "/api/v1/locations", map[string]any{
		"code": "K" + code, "name": "Konsinyasi " + code, "locationType": "consignment",
		"externalPartyType": "partner", "externalPartyId": partner, "isActive": true,
	}, http.StatusCreated)
	consID := uuid.MustParse(cons["id"].(string))
	ctx := actor.System(context.Background())
	var res warehouse.TransferResult
	require.NoError(t, h.app.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		res, err = h.app.warehouse.TransferStock(ctx, warehouse.TransferRequest{
			SourceType: "delivery_order", SourceID: uuid.New(), FromLocationID: fg, ToLocationID: consID,
			Lines: []warehouse.TransferLineInput{{ProductID: h.cok, Qty: decimal.NewFromInt(10)}},
		})
		return err
	}))
	assert.Equal(t, "in_transit", string(res.Transfer.Status))
	assert.Equal(t, "130000", res.TotalCost.String())
	_, err = h.app.warehouse.ReceiveTransfer(ctx, res.Transfer.ID, []warehouse.ReceiveLine{{LineID: res.Transfer.Lines[0].ID, Qty: decimal.NewFromInt(10)}}, true)
	require.NoError(t, err)
	assert.Equal(t, "10.000", h.qty(h.cok, consID, inventory.StatusAvailable))
}

func TestPettyStockRequest(t *testing.T) {
	h := newWHHarness(t)
	fg, pty := h.loc[warehouse.LocationFinishedGoods], h.loc[warehouse.LocationPettyStock]
	h.receive(h.van, h.batch, 50, "12800", fg, inventory.StatusAvailable)
	today := clock.Today(h.app.Clock).String()

	h.as("fulfillment.leader@balans.local")
	h.json(http.MethodPost, "/api/v1/stock-transfers", map[string]any{
		"transferType": "request", "fromLocationId": fg, "toLocationId": h.loc[warehouse.LocationEcommerce], "businessDate": today,
		"lines": []any{map[string]any{"productId": h.van, "qty": "10"}},
	}, http.StatusUnprocessableEntity)
	tr := h.json(http.MethodPost, "/api/v1/stock-transfers", map[string]any{
		"transferType": "request", "fromLocationId": fg, "toLocationId": pty, "businessDate": today,
		"lines": []any{map[string]any{"productId": h.van, "qty": "10"}},
	}, http.StatusCreated)
	id := tr["id"].(string)
	assert.Contains(t, tr["allowedActions"], "submit")
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "submit", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "submitted", tr["status"])
	h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "release", "version": tr["version"]}, http.StatusForbidden)

	h.as("logistik.leader@balans.local")
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "release", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "in_progress", tr["status"])
	assert.Equal(t, "10.000", h.qty(h.van, fg, inventory.StatusReserved))

	h.as("fulfillment.leader@balans.local")
	h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{
		"action": "receive", "version": tr["version"], "lines": []any{map[string]any{"lineId": lines(tr)[0]["id"], "qty": "8"}},
	}, http.StatusConflict) // belum disiapkan

	h.as("staf.gudang@balans.local")
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{"action": "prepare", "version": tr["version"]}, http.StatusOK)
	assert.Equal(t, "Staf Gudang", tr["preparedByName"])

	h.as("fulfillment.leader@balans.local")
	tr = h.json(http.MethodPost, "/api/v1/stock-transfers/"+id+"/actions", map[string]any{
		"action": "receive", "version": tr["version"], "lines": []any{map[string]any{"lineId": lines(tr)[0]["id"], "qty": "8"}},
	}, http.StatusOK)
	assert.Equal(t, "completed", tr["status"])
	assert.Equal(t, "8.000", h.qty(h.van, pty, inventory.StatusAvailable))
	assert.Equal(t, "0.000", h.qty(h.van, fg, inventory.StatusReserved), "sisa alokasi dilepas")
	assert.Equal(t, "42.000", h.qty(h.van, fg, inventory.StatusAvailable))
	list := h.json(http.MethodGet, "/api/v1/stock-transfers?transferType=request&status=completed&q="+tr["number"].(string), nil, http.StatusOK)
	assert.EqualValues(t, 1, list["totalItems"])
}

func (h *whHarness) approvalRequest(subjectID string) map[string]any {
	h.t.Helper()
	list := h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=stock_count&subjectId="+subjectID, nil, http.StatusOK)
	items := list["items"].([]any)
	require.NotEmpty(h.t, items)
	return items[0].(map[string]any)
}

func TestStockCountApprovalPostsAdjustment(t *testing.T) {
	h := newWHHarness(t)
	fg := h.loc[warehouse.LocationFinishedGoods]
	h.receive(h.cok, h.batch, 300, "12500", fg, inventory.StatusAvailable)
	h.receive(h.van, h.batch, 20, "12800", fg, inventory.StatusAvailable)

	h.as("staf.gudang@balans.local")
	created := h.json(http.MethodPost, "/api/v1/stock-counts", map[string]any{"countType": "cycle", "locationIds": []any{fg}}, http.StatusCreated)
	id := created["items"].([]any)[0].(map[string]any)["id"].(string)
	sc := h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusOK)
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "start", "version": sc["version"]}, http.StatusOK)
	assert.Equal(t, "in_progress", sc["status"])
	assert.True(t, strings.HasPrefix(sc["number"].(string), "SC/"))
	assert.Equal(t, false, sc["revealed"])
	ls := lines(sc)
	require.Len(t, ls, 2)
	for _, l := range ls {
		assert.Nil(t, l["systemQty"], "hitung buta")
	}
	counted := map[string]string{}
	for _, l := range ls {
		if l["product"].(map[string]any)["id"] == h.cok.String() {
			counted[l["id"].(string)] = "100" // kurang 200 × 12.500 = 2.500.000 (> Rp2 jt → dua level)
		} else {
			counted[l["id"].(string)] = "22" // lebih 2 × 12.800
		}
	}
	h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "submit", "version": sc["version"]}, http.StatusUnprocessableEntity)
	var entries []any
	for lid, q := range counted {
		entries = append(entries, map[string]any{"lineId": lid, "countedQty": q})
	}
	sc = h.json(http.MethodPut, "/api/v1/stock-counts/"+id+"/entries", map[string]any{"lines": entries}, http.StatusOK)
	assert.EqualValues(t, 2, sc["countedCount"])
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "submit", "version": sc["version"]}, http.StatusOK)
	assert.Equal(t, "pending_approval", sc["status"])
	assert.Equal(t, "2525600.00", sc["varianceAmount"])
	assert.Equal(t, "-2474400.00", sc["netVariance"])

	req := h.approvalRequest(id)
	assert.Equal(t, "2525600.00", req["amount"])
	h.as("head.logistik@balans.local")
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req["id"].(string)+"/approve", map[string]any{"version": req["version"]}, http.StatusOK)
	sc = h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusOK)
	assert.Equal(t, "pending_approval", sc["status"], "> Rp2 jt butuh Head F&A")
	h.as("head.finance@balans.local")
	req = h.approvalRequest(id)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req["id"].(string)+"/approve", map[string]any{"version": req["version"]}, http.StatusOK)

	h.as("staf.gudang@balans.local")
	sc = h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusOK)
	assert.Equal(t, "posted", sc["status"])
	assert.Equal(t, true, sc["revealed"])
	assert.Equal(t, "100.000", h.qty(h.cok, fg, inventory.StatusAvailable))
	assert.Equal(t, "22.000", h.qty(h.van, fg, inventory.StatusAvailable))

	// Jurnal A-14 balance: kurang 2.500.000 − lebih 25.600 = beban bersih 2.474.400 di 6401 / 1311.
	journalID := sc["journalEntryId"].(string)
	var debit, credit decimal.Decimal
	require.NoError(t, h.app.Pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(debit_amount),0), coalesce(sum(credit_amount),0) FROM journal_lines WHERE journal_entry_id = $1`, journalID).
		Scan(&debit, &credit))
	assert.True(t, debit.Equal(credit), "debit %s kredit %s", debit, credit)
	assert.Equal(t, "2474400", debit.String())
	var expense decimal.Decimal
	require.NoError(t, h.app.Pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(l.debit_amount - l.credit_amount),0) FROM journal_lines l JOIN accounts a ON a.id = l.account_id
		 WHERE l.journal_entry_id = $1 AND a.code = '6401'`, journalID).Scan(&expense))
	assert.Equal(t, "2474400", expense.String())
}

func TestStockCountRejectRecountAndScope(t *testing.T) {
	h := newWHHarness(t)
	fg := h.loc[warehouse.LocationFinishedGoods]
	h.receive(h.van, h.batch, 10, "12800", fg, inventory.StatusAvailable)
	h.as("staf.gudang@balans.local")
	created := h.json(http.MethodPost, "/api/v1/stock-counts", map[string]any{"countType": "cycle", "locationIds": []any{fg}}, http.StatusCreated)
	id := created["items"].([]any)[0].(map[string]any)["id"].(string)
	sc := h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "start", "version": 1}, http.StatusOK)
	// Barang ekstra tanpa snapshot (produk lain) + qty kurang 1.
	cok := h.cok
	h.receive(cok, h.batch+"X", 1, "12500", h.loc[warehouse.LocationEcommerce], inventory.StatusAvailable)
	sc = h.json(http.MethodPut, "/api/v1/stock-counts/"+id+"/entries", map[string]any{
		"lines": []any{map[string]any{"lineId": lines(sc)[0]["id"], "countedQty": "9"}},
	}, http.StatusOK)
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "submit", "version": sc["version"]}, http.StatusOK)
	assert.Equal(t, "pending_approval", sc["status"])
	req := h.approvalRequest(id)
	h.as("head.logistik@balans.local")
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req["id"].(string)+"/reject", map[string]any{"version": req["version"], "note": "Hitung ulang rak B"}, http.StatusOK)
	h.as("staf.gudang@balans.local")
	sc = h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusOK)
	assert.Equal(t, "rejected", sc["status"])
	assert.Equal(t, false, sc["revealed"])
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "recount", "version": sc["version"]}, http.StatusOK)
	sc = h.json(http.MethodPut, "/api/v1/stock-counts/"+id+"/entries", map[string]any{
		"lines": []any{map[string]any{"lineId": lines(sc)[0]["id"], "countedQty": "10"}},
	}, http.StatusOK)
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "submit", "version": sc["version"]}, http.StatusOK)
	assert.Equal(t, "completed", sc["status"], "tanpa selisih langsung selesai")

	// Merchandiser hanya melihat lokasi konsinyasi mitra yang ditugaskan.
	h.as("merchandiser@balans.local")
	h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusForbidden)
	list := h.json(http.MethodGet, "/api/v1/stock-counts?locationId="+fg.String(), nil, http.StatusOK)
	assert.EqualValues(t, 0, list["totalItems"])
	h.json(http.MethodPost, "/api/v1/stock-counts", map[string]any{"countType": "cycle", "locationIds": []any{fg}}, http.StatusForbidden)
}

func TestStockCountSchedule(t *testing.T) {
	h := newWHHarness(t)
	fg := h.loc[warehouse.LocationFinishedGoods]
	h.as("logistik.leader@balans.local")
	today := clock.Today(h.app.Clock)
	sc := h.json(http.MethodPost, "/api/v1/stock-count-schedules", map[string]any{
		"name": "Cycle count FG " + h.batch, "countType": "cycle", "frequency": "monthly", "dayOfMonth": today.Day(),
		"locationIds": []any{fg}, "isActive": true,
	}, http.StatusCreated)
	assert.Equal(t, today.String(), sc["nextRunDate"])
	n, err := h.app.warehouse.RunStockCountSchedules(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	list := h.json(http.MethodGet, "/api/v1/stock-counts?locationId="+fg.String(), nil, http.StatusOK)
	assert.EqualValues(t, 1, list["totalItems"])
	item := list["items"].([]any)[0].(map[string]any)
	assert.Equal(t, sc["id"], item["scheduleId"])
	again, err := h.app.warehouse.RunStockCountSchedules(context.Background())
	require.NoError(t, err)
	_ = again
	list = h.json(http.MethodGet, "/api/v1/stock-counts?locationId="+fg.String(), nil, http.StatusOK)
	assert.EqualValues(t, 1, list["totalItems"], "idempotent")
	schedules := h.json(http.MethodGet, "/api/v1/stock-count-schedules", nil, http.StatusOK)
	raw, _ := json.Marshal(schedules)
	assert.Contains(t, string(raw), sc["id"].(string))
	var next string
	for _, s := range schedules["items"].([]any) {
		if s.(map[string]any)["id"] == sc["id"] {
			next = s.(map[string]any)["nextRunDate"].(string)
		}
	}
	assert.NotEqual(t, today.String(), next, "tanggal jalan berikutnya maju")
}
