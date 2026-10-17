package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/logistics"
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

// newLogisticsHarness sama dengan newSalesHarness ditambah modul procurement dan logistics (DO, Outbound/QC).
func newLogisticsHarness(t *testing.T) *salesHarness {
	t.Helper()
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fixed := clock.NewFixed(salesNow)
	cfg := config.Config{
		Env: config.EnvLocal, EmailProvider: config.ProviderMock, PaymentGateway: config.ProviderMock,
		ShipmentProvider: config.ProviderMock, PublicBaseURL: "http://localhost:5173", FilestoreDir: t.TempDir(),
		EnabledModules: []string{
			"identity", "audit", "notification", "approval", "document", "accounting", "catalog", "warehouse",
			"inventory", "filestore", "party", "hris", "pricing", "dataexchange", "procurement", "sales", "logistics",
		},
	}
	a := &App{
		Config: cfg, Log: log, Pool: pool, Clock: fixed, Tx: db.NewTxManager(pool),
		Responder: httpx.NewResponder(log, nil), idempotency: idempotency.NewStore(pool),
	}
	var err error
	a.Jobs, err = jobqueue.NewInserter(pool)
	require.NoError(t, err)
	require.NoError(t, a.buildModules())
	require.NotNil(t, a.logistics)
	ctx := context.Background()
	_, err = a.identity.SeedDevUsers(ctx, "Rahasia-Dev-12345")
	require.NoError(t, err)
	require.NoError(t, a.warehouse.SeedDev(ctx))

	h := &salesHarness{t: t, app: a, clock: fixed, tag: strings.ToUpper(uuid.NewString()[:6])}
	locs, err := a.warehouse.FindLocations(ctx, warehouse.LocationQuery{})
	require.NoError(t, err)
	for _, l := range locs {
		switch l.Code {
		case "UTAMA-FG":
			h.fg = l.ID
		case "UTAMA-ECM":
			h.ecm = l.ID
		}
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), h.actor)))
		})
	})
	for _, m := range a.modules {
		m.RegisterRoutes(r)
	}
	h.srv = httptest.NewServer(r)
	t.Cleanup(h.srv.Close)
	h.as("admin@balans.local")
	return h
}

func (h *salesHarness) userID(email string) string {
	return h.scalar(`SELECT id::text FROM users WHERE email = $1`, email)
}

func (h *salesHarness) courier(code string) string {
	return h.scalar(`SELECT id::text FROM couriers WHERE code = $1`, code)
}

func (h *salesHarness) quarantine(product, location uuid.UUID) string {
	return h.scalar(`SELECT coalesce(sum(qty), 0)::int::text FROM stock_balances
		WHERE product_id = $1 AND location_id = $2 AND stock_status = 'quarantine'`, product, location)
}

// uploadPhoto mengunggah foto kecil untuk owner tertentu dan mengembalikan ID file.
func (h *salesHarness) uploadPhoto(owner string) string {
	h.t.Helper()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	target := h.json(http.MethodPost, "/api/v1/files", map[string]any{
		"ownerType": owner, "fileName": "terima.png", "contentType": "image/png", "sizeBytes": len(png),
	}, http.StatusCreated)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, h.srv.URL+target["uploadUrl"].(string), bytes.NewReader(png))
	require.NoError(h.t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	require.NoError(h.t, res.Body.Close())
	require.Equal(h.t, http.StatusNoContent, res.StatusCode)
	return target["fileId"].(string)
}

func lineByStatus(ob map[string]any, status string) []map[string]any {
	var out []map[string]any
	for _, l := range items(ob, "lines") {
		if l["status"] == status {
			out = append(out, l)
		}
	}
	return out
}

// packAll mencatat satu koli berisi semua baris yang diambil/lulus QC.
func (h *salesHarness) packAll(ob map[string]any) map[string]any {
	h.t.Helper()
	var contents []map[string]any
	for _, l := range items(ob, "lines") {
		switch l["status"] {
		case "picked":
			contents = append(contents, map[string]any{"lineId": l["id"], "qty": l["pickedQty"]})
		case "passed":
			contents = append(contents, map[string]any{"lineId": l["id"], "qty": l["passedQty"]})
		}
	}
	return h.json(http.MethodPost, "/api/v1/outbounds/"+ob["id"].(string)+"/pack", map[string]any{
		"version": ob["version"], "packages": []map[string]any{{"weightGrams": 1500, "items": contents}},
	}, http.StatusOK)
}

var okQC = map[string]any{"batch": true, "expiry": true, "condition": true, "seal": true}

// TestDeliveryOrderOutboundQCFlow: SO MT → DO (Logistik Leader release) → Outbound picking per batch & packing koli
// → QC oleh penyiap ditolak → QC gagal (Karantina + pengganti FEFO otomatis) → QC lulus → serah ke kurir internal
// (Merchandiser) → stok keluar + HPP A-06 → konfirmasi diterima dengan foto → SO Selesai (S-309, S-310, S-312, S-316).
func TestDeliveryOrderOutboundQCFlow(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 60, 10, "40000")
	h.receive(coklat, h.fg, "C2", 300, 50, "42000")
	partner, sub := h.partnerWithSubChannel("modern_trade", "modern_trade")

	h.as("sales.mt@balans.local")
	so := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "modern_trade", "channelId": h.channel("modern_trade"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "prepaid",
		"lines": []map[string]any{{"productId": coklat, "qty": "12"}},
	}, http.StatusCreated)
	so = h.json(http.MethodPost, "/api/v1/sales/orders/"+so["id"].(string)+"/confirm", map[string]any{"version": version(so)}, http.StatusOK)
	so = h.confirmPayment(so)["salesOrder"].(map[string]any)
	require.Equal(t, "in_progress", so["status"])
	soID := so["id"].(string)

	// S-309: DO dari SO oleh Logistik Leader; batch FEFO dari alokasi (C1 10 + C2 2).
	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID}, http.StatusCreated)
	assert.Equal(t, "draft", do["status"])
	assert.Nil(t, do["number"], "nomor DO diberikan saat release")
	assert.Equal(t, "sales", do["deliveryType"])
	require.Len(t, items(do, "lines"), 1)
	assert.Len(t, items(items(do, "lines")[0], "batches"), 2)
	assert.ElementsMatch(t, []any{"release", "cancel"}, do["allowedActions"])
	status, _, _ := h.do(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID})
	assert.Equal(t, http.StatusConflict, status, "satu DO terbuka per Sales Order")
	doID := do["id"].(string)

	h.as("sales.mt@balans.local")
	status, _, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/release", map[string]any{"version": do["version"]})
	assert.Equal(t, http.StatusForbidden, status, "release butuh logistics.delivery_order.post")
	h.as("logistik.leader@balans.local")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	assert.Equal(t, "in_progress", do["status"])
	assert.Regexp(t, `^DO/2026/11/\d{5}$`, do["number"])
	ob := do["outbound"].(map[string]any)
	obID := ob["id"].(string)
	// Cetak DO tanpa harga dengan batch & ED.
	pl := items(do, "printLines")
	require.Len(t, pl, 2)
	assert.Equal(t, h.tag+"C1", pl[0]["batch"].(map[string]any)["number"])
	assert.NotNil(t, pl[0]["batch"].(map[string]any)["expiryDate"])
	assert.NotContains(t, string(must(h.do(http.MethodGet, "/api/v1/delivery-orders/"+doID, nil))), "unitPrice")
	// Label produk mitra: belum ada kode produk mitra → peringatan.
	assert.Equal(t, true, items(do, "productLabels")[0]["partnerCodeMissing"])

	// S-310: picking per batch (Staf Gudang), packing (Head Logistik = penyiap terakhir).
	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	require.Len(t, items(out, "lines"), 2)
	assert.Equal(t, []any{"pick"}, out["allowedActions"])
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	assert.Len(t, lineByStatus(out, "picked"), 2)
	h.as("head.logistik@balans.local")
	out = h.packAll(out)
	require.Len(t, items(out, "packages"), 1)
	assert.Equal(t, "Head Logistik", out["preparedByName"])

	// QC oleh penyiap barang ditolak (D-23/D-24).
	c1 := items(out, "lines")[0]
	status, problem, _ := h.do(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+c1["id"].(string)+"/qc", map[string]any{
		"passedQty": "10", "rejectedQty": "0", "checks": okQC,
	})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Contains(t, problem["detail"], "berbeda dengan penyiap")

	// QC gagal 3 unit C1 → Karantina; pengganti FEFO otomatis dari C2 (C1 habis teralokasi).
	h.as("fulfillment.leader@balans.local")
	view := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	assert.Equal(t, []any{"qc"}, items(view, "lines")[0]["allowedActions"], "QC ditawarkan ke pemeriksa, bukan penyiap")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+c1["id"].(string)+"/qc", map[string]any{
		"passedQty": "7", "rejectedQty": "3", "checks": map[string]any{"batch": true, "expiry": true, "condition": false, "seal": true},
		"rejectReason": "Kemasan penyok",
	}, http.StatusOK)
	assert.Equal(t, "3", h.quarantine(coklat, h.fg), "barang ditolak masuk Karantina")
	assert.Equal(t, "12", h.reserved(coklat, h.fg), "stok pengganti langsung direservasi")
	assert.Equal(t, "45", h.available(coklat, h.fg))
	pending := lineByStatus(out, "pending")
	require.Len(t, pending, 1)
	assert.Equal(t, "3.000", pending[0]["allocatedQty"])
	assert.Equal(t, h.tag+"C2", pending[0]["batch"].(map[string]any)["number"])
	assert.Equal(t, c1["id"], pending[0]["replacementOfLineId"])
	assert.Empty(t, lineByStatus(out, "passed")[0]["returnLabel"], "saldo awal bukan dari GR: tetap di Karantina")
	assert.True(t, out["needsRepack"] == false, "masih ada baris menunggu picking")
	// Alokasi SO ikut berganti batch.
	h.as("sales.mt@balans.local")
	batches := items(items(h.order(soID), "allocations")[0], "batches")
	total := 0
	for _, b := range batches {
		if b["batchNumber"] == h.tag+"C2" {
			assert.Equal(t, "5.000", b["qty"])
		}
		total++
	}
	assert.Equal(t, 2, total)

	h.as("fulfillment.leader@balans.local")
	c2 := lineByStatus(out, "picked")[0]
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+c2["id"].(string)+"/qc", map[string]any{
		"passedQty": "2", "rejectedQty": "0", "checks": okQC,
	}, http.StatusOK)
	assert.Equal(t, "in_progress", out["status"])

	// Picking ulang pengganti + packing ulang, lalu QC oleh Merchandiser → Outbound selesai.
	h.as("staf.gudang@balans.local")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	out = h.packAll(out)
	h.as("merchandiser@balans.local")
	rep := lineByStatus(out, "picked")[0]
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+rep["id"].(string)+"/qc", map[string]any{
		"passedQty": "3", "rejectedQty": "0", "checks": okQC, "photoFileIds": []string{h.uploadPhoto(warehouse.FileOwnerOutboundQC)},
	}, http.StatusOK)
	assert.Equal(t, "completed", out["status"])
	assert.Len(t, items(out, "checks"), 4)

	// S-312: serah ke kurir internal wajib Merchandiser.
	h.as("logistik.leader@balans.local")
	do = h.json(http.MethodGet, "/api/v1/delivery-orders/"+doID, nil, http.StatusOK)
	assert.Contains(t, do["allowedActions"], "ship")
	pk := items(do, "packages")
	require.Len(t, pk, 1)
	assert.Equal(t, float64(1), pk[0]["packageCount"])
	assert.Len(t, items(pk[0], "items"), 3)
	status, problem, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("own"),
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "internalCourierUserId")
	status, problem, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("own"), "internalCourierUserId": h.userID("staf.gudang@balans.local"),
	})
	require.Equal(t, http.StatusUnprocessableEntity, status, "kurir internal harus Merchandiser")
	merch := h.userID("merchandiser@balans.local")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("own"), "internalCourierUserId": merch,
	}, http.StatusOK)
	assert.Equal(t, "in_transit", do["status"])
	assert.Equal(t, "12.000", items(do, "lines")[0]["shippedQty"])
	shipment := do["shipment"].(map[string]any)
	assert.Equal(t, "Merchandiser", shipment["internalCourierName"])
	assert.Equal(t, float64(1), shipment["packageCount"])

	// Stok keluar dari reserved; HPP FIFO batch yang dikirim: 7 × 40.000 + 5 × 42.000.
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "45", h.available(coklat, h.fg))
	assert.Equal(t, "3", h.quarantine(coklat, h.fg))
	shID := h.scalar(`SELECT id::text FROM sales_shipments WHERE source_id = $1`, doID)
	cogs := h.journal("A-06", "sales_shipment", shID)
	d, _ := amountOf(cogs, "5101")
	assert.Equal(t, "490000.00", d)
	h.as("sales.mt@balans.local")
	assert.Equal(t, "in_transit", h.order(soID)["status"])

	// Pelacakan manual.
	h.as("logistik.leader@balans.local")
	h.json(http.MethodPost, "/api/v1/shipments/"+shipment["id"].(string)+"/events", map[string]any{"status": "in_transit", "note": "Berangkat"}, http.StatusOK)
	list := h.json(http.MethodGet, "/api/v1/shipments?q="+do["number"].(string), nil, http.StatusOK)
	require.Len(t, items(list, "items"), 1)
	assert.Equal(t, "in_transit", items(list, "items")[0]["status"])

	// Hanya Merchandiser pengantar yang mengonfirmasi, wajib foto/tanda terima.
	assert.NotContains(t, do["allowedActions"], "deliver")
	status, _, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{"receivedByName": "Budi"})
	assert.Equal(t, http.StatusForbidden, status)
	h.as("merchandiser@balans.local")
	mine := h.json(http.MethodGet, "/api/v1/shipments?mine=true", nil, http.StatusOK)
	require.Len(t, items(mine, "items"), 1)
	status, problem, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{"receivedByName": "Budi"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "proofFileIds")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{
		"receivedByName": "Budi (gudang toko)", "proofFileIds": []string{h.uploadPhoto(logistics.FileOwnerDeliveryProof)},
	}, http.StatusOK)
	assert.Equal(t, "received", do["status"])
	assert.Len(t, items(do, "events"), 3)
	h.as("sales.mt@balans.local")
	assert.Equal(t, "completed", h.order(soID)["status"])
}

// TestDeliveryOrderHeldLineAndMarketplace: stok pengganti kurang → baris ditahan + notifikasi; alokasi ulang saat
// stok masuk; kirim sebagian; marketplace wajib AWB dan diakui saat serah ke kurir (A-10); batal DO.
func TestDeliveryOrderHeldLineAndMarketplace(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.ecm, "E1", 300, 4, "40000")
	shopee := h.subChannel("marketplace", "shopee")

	h.as("sales.ecommerce@balans.local")
	mp := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "marketplace", "channelId": h.channel("marketplace"), "subChannelId": shopee, "externalOrderId": "SHP-" + h.tag,
		"buyerName": "Budi", "businessDate": "2026-11-02", "sourceLocationId": h.ecm,
		"lines": []map[string]any{{"productId": coklat, "qty": "4"}},
	}, http.StatusCreated)
	require.Equal(t, "in_progress", mp["status"])

	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": mp["id"]}, http.StatusCreated)
	assert.Equal(t, "marketplace", do["deliveryType"])
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	doID := do["id"].(string)
	obID := do["outbound"].(map[string]any)["id"].(string)

	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	out = h.packAll(out)

	// Semua stok E-commerce sudah teralokasi: 1 unit ditolak, tidak ada pengganti → baris ditahan + notifikasi CS.
	h.as("fulfillment.leader@balans.local")
	line := items(out, "lines")[0]
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+line["id"].(string)+"/qc", map[string]any{
		"passedQty": "3", "rejectedQty": "1", "checks": okQC, "rejectReason": "Segel rusak",
	}, http.StatusOK)
	held := lineByStatus(out, "held")
	require.Len(t, held, 1)
	assert.Equal(t, "1.000", held[0]["allocatedQty"])
	assert.Equal(t, float64(1), out["heldCount"])
	n := h.scalar(`SELECT count(*)::text FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE u.email = 'cs@balans.local' AND n.kind = 'outbound.held'`)
	assert.Equal(t, "1", n, "Sales/CS mendapat notifikasi")

	h.as("staf.gudang@balans.local")
	status, _, _ := h.do(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+held[0]["id"].(string)+"/reallocate", nil)
	assert.Equal(t, http.StatusUnprocessableEntity, status, "stok belum ada")

	// Stok masuk → alokasi ulang mengisi baris ditahan, picking/packing/QC lagi.
	h.receive(coklat, h.ecm, "E2", 200, 5, "41000")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+held[0]["id"].(string)+"/reallocate", nil, http.StatusOK)
	assert.Empty(t, lineByStatus(out, "held"))
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	out = h.packAll(out)
	// QC ulang gagal lagi dan stok habis → baris ditahan; diputuskan kirim sebagian.
	h.as("fulfillment.leader@balans.local")
	rep := lineByStatus(out, "picked")[0]
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+rep["id"].(string)+"/qc", map[string]any{
		"passedQty": "1", "rejectedQty": "0", "checks": okQC,
	}, http.StatusOK)
	assert.Equal(t, "completed", out["status"])

	// Marketplace wajib AWB; serah ke kurir = pengakuan penjualan A-10.
	h.as("logistik.leader@balans.local")
	do = h.json(http.MethodGet, "/api/v1/delivery-orders/"+doID, nil, http.StatusOK)
	status, problem, _ := h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("jne"), "trackingNumber": "JNE123",
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "marketplaceAwb")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("jne"), "trackingNumber": "JNE123", "marketplaceAwb": "SPXID" + h.tag,
		"shippingCost": "15000",
	}, http.StatusOK)
	assert.Equal(t, "in_transit", do["status"])
	shID := h.scalar(`SELECT id::text FROM sales_shipments WHERE source_id = $1`, doID)
	h.journal("A-10", "sales_shipment", shID)
	assert.Equal(t, "1", h.quarantine(coklat, h.ecm))
	assert.Equal(t, "4", h.available(coklat, h.ecm))
	// Pelacakan dengan AWB.
	list := h.json(http.MethodGet, "/api/v1/shipments?q=SPXID"+h.tag, nil, http.StatusOK)
	require.Len(t, items(list, "items"), 1)
	// Kurir eksternal: konfirmasi diterima tanpa foto.
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{"receivedByName": "Budi"}, http.StatusOK)
	assert.Equal(t, "received", do["status"])
}

// TestDeliveryOrderPartialAndCancel: kirim sebagian atas keputusan user, DO berikutnya untuk sisa; batal DO.
func TestDeliveryOrderPartialAndCancel(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "P1", 300, 5, "40000")
	partner, sub := h.partnerWithSubChannel("b2b", "b2b")
	h.as("sales.mt@balans.local")
	so := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "b2b", "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "prepaid",
		"lines": []map[string]any{{"productId": coklat, "qty": "5"}},
	}, http.StatusCreated)
	so = h.json(http.MethodPost, "/api/v1/sales/orders/"+so["id"].(string)+"/confirm", map[string]any{"version": version(so)}, http.StatusOK)
	so = h.confirmPayment(so)["salesOrder"].(map[string]any)
	soID := so["id"].(string)

	// Batal DO: Outbound ikut batal, reservasi SO tetap.
	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID}, http.StatusCreated)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	status, _, _ := h.do(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/cancel", map[string]any{"version": do["version"], "reason": ""})
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/cancel", map[string]any{"version": do["version"], "reason": "Salah alamat"}, http.StatusOK)
	assert.Equal(t, "cancelled", do["status"])
	assert.Equal(t, "cancelled", do["outbound"].(map[string]any)["status"])
	assert.Equal(t, "5", h.reserved(coklat, h.fg))

	// DO baru: 2 ditolak QC tanpa pengganti → kirim sebagian 3.
	do = h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID}, http.StatusCreated)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	obID := do["outbound"].(map[string]any)["id"].(string)
	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	out = h.packAll(out)
	h.as("fulfillment.leader@balans.local")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+items(out, "lines")[0]["id"].(string)+"/qc", map[string]any{
		"passedQty": "3", "rejectedQty": "2", "checks": okQC, "rejectReason": "Bocor",
	}, http.StatusOK)
	held := lineByStatus(out, "held")[0]
	h.as("staf.gudang@balans.local")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+held["id"].(string)+"/drop", map[string]any{"reason": "Pelanggan setuju kirim sebagian"}, http.StatusOK)
	assert.True(t, out["needsRepack"].(bool))
	out = h.packAll(out)
	assert.Equal(t, "completed", out["status"])

	h.as("logistik.leader@balans.local")
	do = h.json(http.MethodGet, "/api/v1/delivery-orders/"+do["id"].(string), nil, http.StatusOK)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("gosend"),
	}, http.StatusOK)
	assert.Equal(t, "3.000", items(do, "lines")[0]["shippedQty"])
	h.as("sales.mt@balans.local")
	assert.Equal(t, "in_progress", h.order(soID)["status"], "sisa 2 belum dikirim")
	h.as("logistik.leader@balans.local")
	h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/deliver", map[string]any{}, http.StatusOK)
	h.as("sales.mt@balans.local")
	assert.Equal(t, "in_progress", h.order(soID)["status"])

	// DO berikutnya untuk sisa: stok belum ada → ditolak; setelah stok masuk, kekurangan dialokasi ulang.
	h.as("logistik.leader@balans.local")
	status, _, _ = h.do(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID})
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	h.receive(coklat, h.fg, "P2", 300, 2, "41000")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": soID}, http.StatusCreated)
	assert.Equal(t, "2.000", items(do, "lines")[0]["qty"])
	assert.Equal(t, h.tag+"P2", items(items(do, "lines")[0], "batches")[0]["batch"].(map[string]any)["number"])
}

func must(_ int, _ map[string]any, raw []byte) []byte { return raw }
