package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	"balans/internal/platform/tabular"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// salesHarness menyusun aplikasi lengkap (modul R1.1 – R1.3) dengan Postgres asli, router berisi actor tetap
// (tanpa JWT), dan jam tetap sehingga masa berlaku Proforma dan umur piutang bisa diuji.
type salesHarness struct {
	t     *testing.T
	app   *App
	clock *clock.Fixed
	srv   *httptest.Server
	actor actor.Actor
	tag   string
	fg    uuid.UUID
	ecm   uuid.UUID
}

// Senin 2 Nov 2026 09.00 WIB.
var salesNow = time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)

func newSalesHarness(t *testing.T, extraModules ...string) *salesHarness {
	t.Helper()
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fixed := clock.NewFixed(salesNow)
	cfg := config.Config{
		Env: config.EnvLocal, EmailProvider: config.ProviderMock, PaymentGateway: config.ProviderMock,
		ShipmentProvider: config.ProviderMock, PublicBaseURL: "http://localhost:5173", FilestoreDir: t.TempDir(),
		EnabledModules: []string{
			"identity", "audit", "notification", "approval", "document", "accounting", "catalog", "warehouse",
			"inventory", "filestore", "party", "hris", "pricing", "dataexchange", "sales",
		},
	}
	cfg.EnabledModules = append(cfg.EnabledModules, extraModules...)
	a := &App{
		Config: cfg, Log: log, Pool: pool, Clock: fixed, Tx: db.NewTxManager(pool),
		Responder: httpx.NewResponder(log, nil), idempotency: idempotency.NewStore(pool),
	}
	var err error
	a.Jobs, err = jobqueue.NewInserter(pool)
	require.NoError(t, err)
	require.NoError(t, a.buildModules())
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
	require.NotEqual(t, uuid.Nil, h.fg)
	require.NotEqual(t, uuid.Nil, h.ecm)

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

func (h *salesHarness) as(email string) {
	h.t.Helper()
	users, err := h.app.identity.SearchActiveUsers(context.Background(), email, 1)
	require.NoError(h.t, err)
	require.Len(h.t, users, 1)
	h.actor, err = h.app.identity.ActorFor(context.Background(), users[0].ID)
	require.NoError(h.t, err)
	h.actor.ReauthAt = h.clock.Now()
}

func (h *salesHarness) ctx() context.Context { return actor.With(context.Background(), h.actor) }

func (h *salesHarness) do(method, path string, body any) (int, map[string]any, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, h.srv.URL+path, rd)
	require.NoError(h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer res.Body.Close() //nolint:errcheck // body respons test
	raw, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	var out map[string]any
	if len(raw) > 0 && raw[0] == '{' {
		require.NoError(h.t, json.Unmarshal(raw, &out))
	}
	return res.StatusCode, out, raw
}

func (h *salesHarness) json(method, path string, body any, want int) map[string]any {
	h.t.Helper()
	status, out, raw := h.do(method, path, body)
	require.Equal(h.t, want, status, string(raw))
	return out
}

func (h *salesHarness) exec(sql string, args ...any) {
	h.t.Helper()
	_, err := h.app.Pool.Exec(context.Background(), sql, args...)
	require.NoError(h.t, err)
}

func (h *salesHarness) scalar(sql string, args ...any) string {
	h.t.Helper()
	var s string
	require.NoError(h.t, h.app.Pool.QueryRow(context.Background(), sql, args...).Scan(&s))
	return s
}

// --- master data -----------------------------------------------------------------------------

// product membuat Produk Jadi dengan Harga Dasar.
func (h *salesHarness) product(name, basePrice string) uuid.UUID {
	h.t.Helper()
	h.as("admin@balans.local")
	p := h.json(http.MethodPost, "/api/v1/products", map[string]any{
		"sku": fmt.Sprintf("T%s-%s", h.tag, name), "name": "Produk " + name, "productType": "finished_good", "isBundle": false,
		"packagingType": "pouch", "uom": "pcs", "shelfLifeDays": 365, "hasBatch": true, "isActive": true,
	}, http.StatusCreated)
	id := uuid.MustParse(p["id"].(string))
	h.basePrice(id, basePrice)
	return id
}

func (h *salesHarness) bundle(name, basePrice string, components map[uuid.UUID]string) uuid.UUID {
	h.t.Helper()
	h.as("admin@balans.local")
	comps := []map[string]any{}
	for id, q := range components {
		comps = append(comps, map[string]any{"componentProductId": id, "qty": q})
	}
	p := h.json(http.MethodPost, "/api/v1/products", map[string]any{
		"sku": fmt.Sprintf("T%s-%s", h.tag, name), "name": "Paket " + name, "productType": "finished_good", "isBundle": true,
		"uom": "paket", "hasBatch": false, "isActive": true, "bundleComponents": comps,
	}, http.StatusCreated)
	id := uuid.MustParse(p["id"].(string))
	h.basePrice(id, basePrice)
	return id
}

const systemUser = "00000000-0000-0000-0000-000000000000"

// priceVersion mengembalikan versi aktif daftar harga (dibuat bila belum ada).
func (h *salesHarness) priceVersion(code string) string {
	h.t.Helper()
	var id string
	err := h.app.Pool.QueryRow(context.Background(), `
		SELECT v.id::text FROM price_list_versions v JOIN price_lists p ON p.id = v.price_list_id
		WHERE p.code = $1 AND v.status = 'active'`, code).Scan(&id)
	if err == nil {
		return id
	}
	require.NoError(h.t, h.app.Pool.QueryRow(context.Background(), `
		INSERT INTO price_list_versions (price_list_id, version_no, valid_from, status, created_by, updated_by)
		SELECT id, 1, DATE '2026-01-01', 'active', $2, $2 FROM price_lists WHERE code = $1
		RETURNING id::text`, code, systemUser).Scan(&id))
	return id
}

func (h *salesHarness) basePrice(product uuid.UUID, price string) {
	h.exec(`INSERT INTO price_list_items (price_list_version_id, product_id, price_amount, created_by) VALUES ($1, $2, $3, $4)`,
		h.priceVersion("HARGA-DASAR"), product, price, systemUser)
}

// receive memposting stok Tersedia satu batch di lokasi.
func (h *salesHarness) receive(product, location uuid.UUID, batch string, expiryDays int, qty int64, unitCost string) {
	h.t.Helper()
	exp := clock.Today(h.clock).AddDays(expiryDays)
	cost := decimal.RequireFromString(unitCost)
	_, err := h.app.inventory.Post(actor.System(context.Background()), inventory.Posting{
		Source: inventory.Source{Type: "opening_balance", ID: uuid.New(), Label: "Saldo awal test", BusinessDate: clock.Today(h.clock)},
		Lines: []inventory.Line{{
			Kind: inventory.KindReceive, ProductID: product, Qty: decimal.NewFromInt(qty), UnitCost: &cost,
			Batch: &inventory.NewBatch{Number: h.tag + batch, ExpiryDate: &exp},
			To:    &inventory.Place{LocationID: location, Status: inventory.StatusAvailable},
		}},
	})
	require.NoError(h.t, err)
}

func (h *salesHarness) available(product, location uuid.UUID) string {
	h.t.Helper()
	q, err := h.app.inventory.AvailableQty(context.Background(), []uuid.UUID{product}, &location)
	require.NoError(h.t, err)
	return q[product].StringFixed(0)
}

func (h *salesHarness) reserved(product, location uuid.UUID) string {
	return h.scalar(`SELECT coalesce(sum(qty), 0)::int::text FROM stock_balances
		WHERE product_id = $1 AND location_id = $2 AND stock_status = 'reserved'`, product, location)
}

func (h *salesHarness) channel(code string) string {
	return h.scalar(`SELECT id::text FROM channels WHERE code = $1`, code)
}

func (h *salesHarness) subChannel(channel, code string) string {
	return h.scalar(`SELECT s.id::text FROM sub_channels s JOIN channels c ON c.id = s.channel_id WHERE c.code = $1 AND s.code = $2`, channel, code)
}

// partnerWithSubChannel membuat mitra dan sub-channel miliknya pada channel tertentu.
func (h *salesHarness) partnerWithSubChannel(partnerType, channel string) (partnerID, subChannelID string) {
	h.t.Helper()
	h.as("admin@balans.local")
	p := h.json(http.MethodPost, "/api/v1/partners", map[string]any{
		"code": "M" + h.tag + strings.ToUpper(partnerType[:2]), "name": "Mitra " + partnerType + " " + h.tag, "partnerType": partnerType,
		"address": "Jl. Sudirman 1, Jakarta",
	}, http.StatusCreated)
	partnerID = p["id"].(string)
	subChannelID = h.scalar(`INSERT INTO sub_channels (channel_id, code, name, party_type, party_id, created_by, updated_by)
		SELECT id, $2, $3, 'partner', $4, $5, $5 FROM channels WHERE code = $1 RETURNING id::text`,
		channel, strings.ToLower("m"+h.tag+partnerType[:2]), "Mitra "+h.tag, partnerID, systemUser)
	return partnerID, subChannelID
}

func (h *salesHarness) customer(name, phone, customerType string, employeeID string) string {
	h.t.Helper()
	body := map[string]any{
		"fullName": name, "phone": phone, "customerType": customerType, "marketingConsent": false,
		"addresses": []map[string]any{{"recipientName": name, "addressLine": "Jl. Melati 5", "city": "Bandung", "isDefault": true}},
	}
	if employeeID != "" {
		body["employeeId"] = employeeID
	}
	c := h.json(http.MethodPost, "/api/v1/customers", body, http.StatusCreated)
	return c["id"].(string)
}

func (h *salesHarness) discountLimit(role, pct string) {
	h.exec(`INSERT INTO discount_limits (role_code, max_line_discount_percent, max_document_discount_percent, valid_from, created_by, updated_by)
		VALUES ($1, $2, $2, DATE '2026-01-01', $3, $3) ON CONFLICT DO NOTHING`, role, pct, systemUser)
}

// --- jurnal ----------------------------------------------------------------------------------

type journalLine struct {
	account, debit, credit, channel, subChannel string
}

// journal membaca baris jurnal satu rule untuk dokumen sumber dan memastikan balance.
func (h *salesHarness) journal(rule, sourceType string, sourceID any) []journalLine {
	h.t.Helper()
	rows, err := h.app.Pool.Query(context.Background(), `
		SELECT a.code, l.debit_amount::text, l.credit_amount::text, coalesce(c.code, ''), coalesce(s.code, '')
		FROM journal_entries e JOIN journal_lines l ON l.journal_entry_id = e.id JOIN accounts a ON a.id = l.account_id
		LEFT JOIN channels c ON c.id = l.channel_id LEFT JOIN sub_channels s ON s.id = l.sub_channel_id
		WHERE e.posting_rule_code = $1 AND e.source_type = $2 AND e.source_id = $3 AND e.reversal_of_id IS NULL
		ORDER BY a.code`, rule, sourceType, sourceID)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []journalLine
	debit, credit := decimal.Zero, decimal.Zero
	for rows.Next() {
		var l journalLine
		require.NoError(h.t, rows.Scan(&l.account, &l.debit, &l.credit, &l.channel, &l.subChannel))
		debit = debit.Add(decimal.RequireFromString(l.debit))
		credit = credit.Add(decimal.RequireFromString(l.credit))
		out = append(out, l)
	}
	require.NoError(h.t, rows.Err())
	require.NotEmpty(h.t, out, "jurnal %s %s ada", rule, sourceType)
	require.True(h.t, debit.Equal(credit), "jurnal %s balance: %s vs %s", rule, debit, credit)
	return out
}

func amountOf(lines []journalLine, account string) (debit, credit string) {
	d, c := decimal.Zero, decimal.Zero
	for _, l := range lines {
		if l.account == account {
			d = d.Add(decimal.RequireFromString(l.debit))
			c = c.Add(decimal.RequireFromString(l.credit))
		}
	}
	return d.StringFixed(2), c.StringFixed(2)
}

func assertDims(t *testing.T, lines []journalLine, channel, subChannel string) {
	t.Helper()
	for _, l := range lines {
		assert.Equal(t, channel, l.channel, "channel baris %s", l.account)
		assert.Equal(t, subChannel, l.subChannel, "sub-channel baris %s", l.account)
	}
}

func items(m map[string]any, key string) []map[string]any {
	raw, _ := m[key].([]any)
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i] = r.(map[string]any)
	}
	return out
}

func (h *salesHarness) order(id string) map[string]any {
	return h.json(http.MethodGet, "/api/v1/sales/orders/"+id, nil, http.StatusOK)
}

func version(m map[string]any) any { return m["version"] }

// confirmPayment: Finance mengonfirmasi pembayaran Proforma pertama pesanan.
func (h *salesHarness) confirmPayment(order map[string]any) map[string]any {
	h.t.Helper()
	pis := items(order, "proformas")
	require.Len(h.t, pis, 1)
	h.as("finance@balans.local")
	pi := h.json(http.MethodGet, "/api/v1/sales/proforma-invoices/"+pis[0]["id"].(string), nil, http.StatusOK)
	assert.Contains(h.t, pi["allowedActions"], "confirm_payment")
	return h.json(http.MethodPost, "/api/v1/sales/proforma-invoices/"+pis[0]["id"].(string)+"/confirm-payment", map[string]any{
		"version": version(pi), "method": "bank_transfer", "accountCode": "1111", "reference": "TRF-" + h.tag,
	}, http.StatusOK)
}

// --- S-301, S-302, S-313 (MT) ----------------------------------------------------------------

func TestSalesModernTradePrepaidFlow(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	vanila := h.product("VAN", "50000")
	paket := h.bundle("PKT", "220000", map[uuid.UUID]string{coklat: "2", vanila: "1"})
	h.receive(coklat, h.fg, "C1", 60, 10, "40000")
	h.receive(coklat, h.fg, "C2", 300, 50, "42000")
	h.receive(vanila, h.fg, "V1", 200, 30, "20000")
	partner, sub := h.partnerWithSubChannel("modern_trade", "modern_trade")

	h.as("sales.mt@balans.local")
	draft := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "modern_trade", "channelId": h.channel("modern_trade"), "subChannelId": sub, "partnerId": partner,
		"clientPoNumber": "PO-KLIEN-1", "businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "prepaid",
		"lines": []map[string]any{{"productId": coklat, "qty": "12"}, {"productId": paket, "qty": "2"}},
	}, http.StatusCreated)
	assert.Equal(t, "draft", draft["status"])
	assert.Nil(t, draft["number"], "nomor diberikan saat konfirmasi")
	lines := items(draft, "lines")
	assert.Equal(t, "100000.00", lines[0]["unitPrice"])
	assert.Equal(t, "base_price", lines[0]["priceRule"], "harga otomatis dari resolver")
	assert.NotNil(t, lines[0]["priceListVersionId"], "versi daftar harga disimpan per baris")
	assert.Equal(t, "1640000.00", draft["totalAmount"])
	assert.ElementsMatch(t, []any{"edit", "confirm", "cancel"}, draft["allowedActions"])

	// Harga tidak boleh diketik untuk MT.
	status, problem, _ := h.do(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "modern_trade", "channelId": h.channel("modern_trade"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "1", "unitPrice": "1000"}},
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "lines.0.unitPrice")

	so := h.json(http.MethodPost, "/api/v1/sales/orders/"+draft["id"].(string)+"/confirm", map[string]any{"version": version(draft)}, http.StatusOK)
	require.Equal(t, "awaiting_payment", so["status"])
	assert.Regexp(t, `^SO/2026/11/\d{5}$`, so["number"])
	pis := items(so, "proformas")
	require.Len(t, pis, 1)
	assert.Regexp(t, `^PI/2026/11/\d{5}$`, pis[0]["number"])
	assert.Equal(t, "2026-11-16", pis[0]["validUntil"], "Proforma berlaku 14 hari")
	assert.Equal(t, "1640000.00", pis[0]["amount"])

	// Alokasi FEFO: 12 coklat + 4 coklat dari bundel = 16 → batch C1 (10) lalu C2 (6); vanila 2 dari bundel.
	allocs := items(so, "allocations")
	require.Len(t, allocs, 3)
	assert.Equal(t, "16", h.reserved(coklat, h.fg))
	assert.Equal(t, "2", h.reserved(vanila, h.fg))
	assert.Equal(t, "44", h.available(coklat, h.fg))
	first := items(allocs[0], "batches")
	require.Len(t, first, 2, "12 coklat terpecah ke dua batch FEFO")
	assert.Equal(t, "10.000", first[0]["qty"])
	assert.Equal(t, h.tag+"C1", first[0]["batchNumber"], "batch kedaluwarsa terdekat lebih dulu (FEFO)")
	assert.NotNil(t, first[0]["expiryDate"])

	so = h.confirmPayment(so)["salesOrder"].(map[string]any)
	require.Equal(t, "in_progress", so["status"])
	invs := items(so, "invoices")
	require.Len(t, invs, 1)
	assert.Equal(t, "paid", invs[0]["status"])
	assert.Equal(t, "1640000.00", invs[0]["paidAmount"])

	// A-05: Dr Piutang Usaha 1.640.000, Cr Penjualan 1.640.000, dimensi channel MT + sub-channel mitra.
	invoiceID := invs[0]["id"]
	sales05 := h.journal("A-05", "sales_invoice", invoiceID)
	d, _ := amountOf(sales05, "1201")
	_, c := amountOf(sales05, "4101")
	assert.Equal(t, "1640000.00", d)
	assert.Equal(t, "1640000.00", c)
	subCode := h.scalar(`SELECT code FROM sub_channels WHERE id = $1`, sub)
	assertDims(t, sales05, "modern_trade", subCode)
	// Pembayaran A-PB: Dr Bank, Cr Piutang Usaha.
	h.as("finance@balans.local")
	pays := h.json(http.MethodGet, "/api/v1/sales/customer-payments?partyId="+partner, nil, http.StatusOK)
	payItems := items(pays, "items")
	require.Len(t, payItems, 1)
	pb := h.journal("A-PB", "customer_payment", payItems[0]["id"])
	d, _ = amountOf(pb, "1111")
	assert.Equal(t, "1640000.00", d)

	// Logistik: barang dikirim → stok reserved keluar, HPP A-06 (FIFO batch: 10×40.000 + 6×42.000 + 2×20.000).
	ctx := actor.System(context.Background())
	orderID := uuid.MustParse(so["id"].(string))
	shippable, err := h.app.sales.ShippableOrder(ctx, orderID)
	require.NoError(t, err)
	require.Len(t, shippable.Allocations, 3)
	doID := uuid.New()
	sh, err := h.app.sales.MarkShipped(ctx, sales.ShipmentInput{OrderID: orderID, SourceType: "delivery_order", SourceID: doID, SourceLabel: "DO/2026/11/00001"})
	require.NoError(t, err)
	assert.Equal(t, "692000.00", sh.Cost.String())
	again, err := h.app.sales.MarkShipped(ctx, sales.ShipmentInput{OrderID: orderID, SourceType: "delivery_order", SourceID: doID})
	require.NoError(t, err)
	assert.Equal(t, sh.ID, again.ID, "pengiriman idempotent per DO")
	cogs := h.journal("A-06", "sales_shipment", sh.ID)
	d, _ = amountOf(cogs, "5101")
	_, c = amountOf(cogs, "1311")
	assert.Equal(t, "692000.00", d)
	assert.Equal(t, "692000.00", c)
	assertDims(t, cogs, "modern_trade", subCode)
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "44", h.available(coklat, h.fg))

	h.as("sales.mt@balans.local")
	so = h.order(so["id"].(string))
	assert.Equal(t, "in_transit", so["status"])
	assert.Equal(t, "2.000", items(so, "lines")[1]["shippedQty"])
	_, err = h.app.sales.MarkDelivered(ctx, orderID)
	require.NoError(t, err)
	assert.Equal(t, "completed", h.order(so["id"].(string))["status"])

	// Invoice cetak & teks WhatsApp memuat keterangan NON-PKP.
	text := h.json(http.MethodGet, fmt.Sprintf("/api/v1/sales/invoices/%s/share-text", invoiceID), nil, http.StatusOK)
	assert.Contains(t, text["text"], "Balans bukan Pengusaha Kena Pajak")
	assert.Contains(t, text["text"], "LUNAS")

	// Riwayat dokumen tercatat.
	n := h.scalar(`SELECT count(*)::text FROM audit_logs WHERE entity_type = 'sales_order' AND entity_id = $1`, so["id"])
	assert.NotEqual(t, "0", n)
}

// --- S-301 diskon di atas batas role → approval sales_order_discount -----------------------

func TestSalesDiscountApprovalAndRejection(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 200, 40, "40000")
	partner, sub := h.partnerWithSubChannel("b2b", "b2b")
	h.discountLimit("sales_mt", "5")

	create := func(discount string) map[string]any {
		h.as("sales.mt@balans.local")
		o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
			"orderType": "b2b", "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
			"businessDate": "2026-11-02", "sourceLocationId": h.fg,
			"lines": []map[string]any{{"productId": coklat, "qty": "10", "discount": map[string]any{"kind": "percent", "value": discount}}},
		}, http.StatusCreated)
		return h.json(http.MethodPost, "/api/v1/sales/orders/"+o["id"].(string)+"/confirm", map[string]any{"version": version(o)}, http.StatusOK)
	}

	// Diskon 3% (≤ 5%) langsung Menunggu Pembayaran.
	ok := create("3")
	assert.Equal(t, "awaiting_payment", ok["status"])
	assert.Equal(t, "970000.00", ok["totalAmount"])

	// Diskon 10% > batas → Menunggu Persetujuan, stok tetap dialokasikan, belum ada Proforma.
	so := create("10")
	require.Equal(t, "pending_approval", so["status"])
	assert.Empty(t, items(so, "proformas"))
	assert.Equal(t, "20", h.reserved(coklat, h.fg))

	h.as("head.sales@balans.local")
	list := h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=sales_order_discount&subjectId="+so["id"].(string), nil, http.StatusOK)
	req := items(list, "items")
	require.Len(t, req, 1)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req[0]["id"].(string)+"/approve", map[string]any{"version": req[0]["version"]}, http.StatusOK)
	h.as("sales.mt@balans.local")
	so = h.order(so["id"].(string))
	assert.Equal(t, "awaiting_payment", so["status"])
	assert.Len(t, items(so, "proformas"), 1, "Proforma terbit setelah diskon disetujui")

	// Ditolak → kembali Draft dan alokasi dilepas.
	rej := create("15")
	require.Equal(t, "pending_approval", rej["status"])
	assert.Equal(t, "30", h.reserved(coklat, h.fg))
	h.as("head.sales@balans.local")
	list = h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=sales_order_discount&subjectId="+rej["id"].(string), nil, http.StatusOK)
	req = items(list, "items")
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req[0]["id"].(string)+"/reject",
		map[string]any{"version": req[0]["version"], "note": "Diskon terlalu besar"}, http.StatusOK)
	h.as("sales.mt@balans.local")
	rej = h.order(rej["id"].(string))
	assert.Equal(t, "draft", rej["status"])
	assert.Equal(t, "20", h.reserved(coklat, h.fg))

	// Revisi diskon lalu konfirmasi ulang: alokasi baru dengan nomor SO yang sama.
	number := rej["number"]
	upd := h.json(http.MethodPut, "/api/v1/sales/orders/"+rej["id"].(string), map[string]any{
		"version": version(rej), "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "10", "discount": map[string]any{"kind": "percent", "value": "5"}}},
	}, http.StatusOK)
	again := h.json(http.MethodPost, "/api/v1/sales/orders/"+rej["id"].(string)+"/confirm", map[string]any{"version": version(upd)}, http.StatusOK)
	assert.Equal(t, "awaiting_payment", again["status"])
	assert.Equal(t, number, again["number"])
	assert.Equal(t, "30", h.reserved(coklat, h.fg))

	// Batal melepas alokasi dan membatalkan Proforma.
	cancel := h.json(http.MethodPost, "/api/v1/sales/orders/"+again["id"].(string)+"/cancel",
		map[string]any{"version": version(again), "reason": "Klien membatalkan PO"}, http.StatusOK)
	assert.Equal(t, "cancelled", cancel["status"])
	assert.Equal(t, "cancelled", items(cancel, "proformas")[0]["status"])
	assert.Equal(t, "20", h.reserved(coklat, h.fg))
}

// --- S-302 Proforma kedaluwarsa melepas alokasi --------------------------------------------------

func TestSalesProformaExpiryReleasesAllocation(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 200, 10, "40000")
	partner, sub := h.partnerWithSubChannel("modern_trade", "modern_trade")
	h.as("sales.mt@balans.local")
	o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "modern_trade", "channelId": h.channel("modern_trade"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg, "lines": []map[string]any{{"productId": coklat, "qty": "4"}},
	}, http.StatusCreated)
	so := h.json(http.MethodPost, "/api/v1/sales/orders/"+o["id"].(string)+"/confirm", map[string]any{"version": version(o)}, http.StatusOK)
	assert.Equal(t, "4", h.reserved(coklat, h.fg))

	// Hari ke-14 masih berlaku.
	h.clock.Advance(14 * 24 * time.Hour)
	n, err := h.app.sales.ExpireProformas(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	h.clock.Advance(24 * time.Hour)
	n, err = h.app.sales.ExpireProformas(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	so = h.order(so["id"].(string))
	assert.Equal(t, "expired", so["status"])
	assert.Equal(t, "expired", items(so, "proformas")[0]["status"])
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "10", h.available(coklat, h.fg))

	// Proforma kedaluwarsa tidak bisa dibayar.
	h.as("finance@balans.local")
	pi := h.json(http.MethodGet, "/api/v1/sales/proforma-invoices/"+items(so, "proformas")[0]["id"].(string), nil, http.StatusOK)
	assert.NotContains(t, pi["allowedActions"], "confirm_payment")
	status, _, _ := h.do(http.MethodPost, "/api/v1/sales/proforma-invoices/"+pi["id"].(string)+"/confirm-payment",
		map[string]any{"version": version(pi), "method": "bank_transfer", "accountCode": "1111"})
	assert.Equal(t, http.StatusConflict, status)
}

// --- S-315 piutang tempo, pembayaran sebagian, satu pembayaran ke banyak invoice, umur piutang ----

func TestSalesReceivablesPartialAndMultiInvoicePayments(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 300, 50, "40000")
	partner, sub := h.partnerWithSubChannel("b2b", "b2b")
	subCode := h.scalar(`SELECT code FROM sub_channels WHERE id = $1`, sub)

	credit := func(qty string) map[string]any {
		h.as("sales.mt@balans.local")
		o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
			"orderType": "b2b", "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
			"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "credit", "paymentTermDays": 30,
			"lines": []map[string]any{{"productId": coklat, "qty": qty}},
		}, http.StatusCreated)
		so := h.json(http.MethodPost, "/api/v1/sales/orders/"+o["id"].(string)+"/confirm", map[string]any{"version": version(o)}, http.StatusOK)
		require.Equal(t, "in_progress", so["status"], "tempo: invoice terbit saat konfirmasi")
		require.Len(t, items(so, "invoices"), 1)
		return items(so, "invoices")[0]
	}
	inv1, inv2 := credit("3"), credit("2")
	assert.Equal(t, "awaiting_payment", inv1["status"])
	a05 := h.journal("A-05", "sales_invoice", inv1["id"])
	d, _ := amountOf(a05, "1201")
	assert.Equal(t, "300000.00", d)
	assertDims(t, a05, "b2b", subCode)

	h.as("finance@balans.local")
	detail := h.json(http.MethodGet, "/api/v1/sales/invoices/"+inv1["id"].(string), nil, http.StatusOK)
	assert.Equal(t, "2026-12-02", detail["dueDate"])
	assert.Contains(t, detail["allowedActions"], "record_payment")

	// Pembayaran sebagian invoice 1.
	pay := func(amount string, allocs ...map[string]any) map[string]any {
		return h.json(http.MethodPost, "/api/v1/sales/customer-payments", map[string]any{
			"partyType": "partner", "partyId": partner, "paymentDate": "2026-11-05", "amount": amount, "method": "bank_transfer",
			"accountCode": "1111", "reference": "TRF-" + amount, "allocations": allocs,
		}, http.StatusCreated)
	}
	p1 := pay("100000", map[string]any{"invoiceId": inv1["id"], "amount": "100000"})
	assert.Regexp(t, `^BYR/2026/11/\d{5}$`, p1["number"])
	inv := h.json(http.MethodGet, "/api/v1/sales/invoices/"+inv1["id"].(string), nil, http.StatusOK)
	assert.Equal(t, "partially_paid", inv["status"])
	assert.Equal(t, "200000.00", inv["outstandingAmount"])
	pb := h.journal("A-PB", "customer_payment", p1["id"])
	d, _ = amountOf(pb, "1111")
	_, c := amountOf(pb, "1201")
	assert.Equal(t, "100000.00", d)
	assert.Equal(t, "100000.00", c)

	// Alokasi melebihi sisa dan jumlah alokasi ≠ nominal ditolak.
	status, problem, _ := h.do(http.MethodPost, "/api/v1/sales/customer-payments", map[string]any{
		"partyType": "partner", "partyId": partner, "paymentDate": "2026-11-05", "amount": "250000", "method": "bank_transfer",
		"accountCode": "1111", "allocations": []map[string]any{{"invoiceId": inv1["id"], "amount": "250000"}},
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "allocations.0.amount")

	// Umur piutang 40 hari setelah invoice: 500.000 − 100.000 lewat jatuh tempo 1–30 hari.
	h.clock.Advance(40 * 24 * time.Hour)
	aging := h.json(http.MethodGet, "/api/v1/sales/receivable-aging", nil, http.StatusOK)
	var row map[string]any
	for _, r := range items(aging, "rows") {
		if r["partyId"] == partner {
			row = r
		}
	}
	require.NotNil(t, row)
	assert.Equal(t, "400000.00", row["days1To30"])
	assert.Equal(t, "400000.00", row["total"])
	assert.EqualValues(t, 2, row["invoices"])
	overdue := h.json(http.MethodGet, "/api/v1/sales/invoices?overdue=true&partyId="+partner, nil, http.StatusOK)
	assert.Len(t, items(overdue, "items"), 2)

	// Satu pembayaran dialokasikan ke dua invoice → keduanya Lunas.
	h.as("finance@balans.local")
	p2 := pay("400000", map[string]any{"invoiceId": inv1["id"], "amount": "200000"}, map[string]any{"invoiceId": inv2["id"], "amount": "200000"})
	assert.Len(t, items(p2, "allocations"), 2)
	for _, id := range []any{inv1["id"], inv2["id"]} {
		assert.Equal(t, "paid", h.json(http.MethodGet, fmt.Sprintf("/api/v1/sales/invoices/%s", id), nil, http.StatusOK)["status"])
	}

	// API rekonsiliasi bank melihat kedua pembayaran di rekening 1111.
	found, err := h.app.sales.IncomingPayments(context.Background(), sales.PaymentQuery{
		AccountCode: "1111", DateFrom: clock.NewBusinessDate(2026, 11, 5), DateTo: clock.NewBusinessDate(2026, 11, 5),
	})
	require.NoError(t, err)
	numbers := map[string]bool{}
	for _, p := range found {
		numbers[p.Number] = true
		assert.NotNil(t, p.JournalEntryID)
	}
	assert.True(t, numbers[p1["number"].(string)] && numbers[p2["number"].(string)])

	// Batal pembayaran: jurnal dibalik dan piutang invoice kembali.
	cancelled := h.json(http.MethodPost, "/api/v1/sales/customer-payments/"+p2["id"].(string)+"/cancel",
		map[string]any{"version": version(p2), "reason": "Salah rekening"}, http.StatusOK)
	assert.Equal(t, "cancelled", cancelled["status"])
	assert.Equal(t, "partially_paid", h.json(http.MethodGet, fmt.Sprintf("/api/v1/sales/invoices/%s", inv1["id"]), nil, http.StatusOK)["status"])
	assert.Equal(t, "1", h.scalar(`SELECT count(*)::text FROM journal_entries r JOIN journal_entries o ON o.id = r.reversal_of_id
		WHERE o.source_type = 'customer_payment' AND o.source_id = $1`, p2["id"]))

	// Tanpa re-autentikasi ditolak (aksi ★).
	h.actor.ReauthAt = time.Time{}
	status, problem, _ = h.do(http.MethodPost, "/api/v1/sales/customer-payments/"+p1["id"].(string)+"/cancel",
		map[string]any{"version": version(p1), "reason": "x"})
	assert.Equal(t, http.StatusUnauthorized, status, problem)
}

// --- S-306, S-308 WA/IG dan penjualan karyawan ---------------------------------------------------

func TestSalesSocialCommerceAndEmployeePricing(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 300, 20, "40000")

	// Daftar harga karyawan (S-116) berisi coklat Rp80.000.
	listID := h.scalar(`INSERT INTO price_lists (code, name, price_list_type, created_by, updated_by)
		VALUES ($1, 'Harga Karyawan', 'employee', $2, $2) RETURNING id::text`, "KRY-"+h.tag, systemUser)
	versionID := h.scalar(`INSERT INTO price_list_versions (price_list_id, version_no, valid_from, status, created_by, updated_by)
		VALUES ($1, 1, DATE '2026-01-01', 'active', $2, $2) RETURNING id::text`, listID, systemUser)
	h.exec(`INSERT INTO price_list_items (price_list_version_id, product_id, price_amount, created_by) VALUES ($1, $2, 80000, $3)`,
		versionID, coklat, systemUser)

	h.as("hrga@balans.local")
	emp := h.json(http.MethodPost, "/api/v1/employees", map[string]any{
		"employeeNumber": "K" + h.tag, "fullName": "Karyawan " + h.tag, "positionTitle": "Staf", "joinDate": "2025-01-01",
	}, http.StatusCreated)

	h.as("cs@balans.local")
	phone := fmt.Sprintf("0812%08d", time.Now().UnixNano()%100000000)
	retail := h.customer("Pelanggan "+h.tag, phone, "retail", "")
	employee := h.customer("Karyawan "+h.tag, fmt.Sprintf("0813%08d", time.Now().UnixNano()%100000000), "employee", emp["id"].(string))

	// Pesanan WA: harga dasar + ongkir.
	wa := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "social_commerce", "channelId": h.channel("social_commerce"), "subChannelId": h.subChannel("social_commerce", "whatsapp"),
		"customerId": retail, "businessDate": "2026-11-02", "sourceLocationId": h.fg, "shippingAmount": "15000",
		"lines": []map[string]any{{"productId": coklat, "qty": "2"}},
	}, http.StatusCreated)
	assert.Equal(t, "100000.00", items(wa, "lines")[0]["unitPrice"])
	assert.Equal(t, "215000.00", wa["totalAmount"])
	assert.Equal(t, "Jl. Melati 5, Bandung", wa["shipToAddress"], "alamat utama pelanggan dipakai bila kosong")
	wa = h.json(http.MethodPost, "/api/v1/sales/orders/"+wa["id"].(string)+"/confirm", map[string]any{"version": version(wa)}, http.StatusOK)
	require.Equal(t, "awaiting_payment", wa["status"])

	// CS mengunggah bukti bayar.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	target := h.json(http.MethodPost, "/api/v1/files", map[string]any{
		"ownerType": sales.FileOwnerPaymentProof, "fileName": "bukti.png", "contentType": "image/png", "sizeBytes": len(png),
	}, http.StatusCreated)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, h.srv.URL+target["uploadUrl"].(string), bytes.NewReader(png))
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	wa = h.json(http.MethodPost, "/api/v1/sales/orders/"+wa["id"].(string)+"/payment-proof",
		map[string]any{"version": version(wa), "fileId": target["fileId"]}, http.StatusOK)
	assert.Equal(t, "bukti.png", wa["paymentProofFileName"])

	paid := h.confirmPayment(wa)["salesOrder"].(map[string]any)
	invoiceID := items(paid, "invoices")[0]["id"]
	a05 := h.journal("A-05", "sales_invoice", invoiceID)
	_, ship := amountOf(a05, "4201")
	_, gross := amountOf(a05, "4101")
	assert.Equal(t, "15000.00", ship, "ongkir ke Pendapatan Ongkos Kirim")
	assert.Equal(t, "200000.00", gross)
	assertDims(t, a05, "social_commerce", "whatsapp")
	h.as("cs@balans.local")
	text := h.json(http.MethodGet, fmt.Sprintf("/api/v1/sales/invoices/%s/share-text", invoiceID), nil, http.StatusOK)
	assert.Contains(t, text["text"], "Ongkos kirim")

	// Penjualan karyawan otomatis memakai daftar harga karyawan.
	emo := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "employee", "channelId": h.channel("employee"), "subChannelId": h.subChannel("employee", "karyawan"),
		"customerId": employee, "businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "1"}},
	}, http.StatusCreated)
	line := items(emo, "lines")[0]
	assert.Equal(t, "80000.00", line["unitPrice"])
	assert.Equal(t, "price_list", line["priceRule"])
	assert.Equal(t, versionID, line["priceListVersionId"])

	// Pelanggan retail tidak bisa dipakai di Penjualan Karyawan.
	status, problem, _ := h.do(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "employee", "channelId": h.channel("employee"), "subChannelId": h.subChannel("employee", "karyawan"),
		"customerId": retail, "businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "1"}},
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "customerId")

	// Karyawan resign: harga karyawan hilang mulai tanggal resign.
	h.as("hrga@balans.local")
	h.json(http.MethodPost, "/api/v1/employees/"+emp["id"].(string)+"/resign",
		map[string]any{"version": emp["version"], "resignDate": "2026-11-02", "reason": "Mengundurkan diri"}, http.StatusOK)
	h.as("cs@balans.local")
	status, problem, _ = h.do(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "employee", "channelId": h.channel("employee"), "subChannelId": h.subChannel("employee", "karyawan"),
		"customerId": employee, "businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "1"}},
	})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"].(map[string]any)["customerId"], "tidak aktif")
}

// --- S-307, S-313 marketplace ---------------------------------------------------------------------

func TestSalesMarketplaceManualImportAndHandover(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	vanila := h.product("VAN", "50000")
	paket := h.bundle("PKT", "220000", map[uuid.UUID]string{coklat: "2", vanila: "1"})
	h.receive(coklat, h.ecm, "C1", 300, 20, "40000")
	h.receive(vanila, h.ecm, "V1", 300, 20, "20000")
	h.receive(coklat, h.fg, "C9", 300, 20, "40000")
	shopee := h.subChannel("marketplace", "shopee")

	h.as("sales.ecommerce@balans.local")
	ext := "SHP-" + h.tag
	mp := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "marketplace", "channelId": h.channel("marketplace"), "subChannelId": shopee, "externalOrderId": ext,
		"buyerName": "Budi", "businessDate": "2026-11-02", "sourceLocationId": h.ecm,
		"documentDiscount": map[string]any{"kind": "amount", "value": "20000"},
		"lines":            []map[string]any{{"productId": paket, "qty": "2", "unitPrice": "210000"}, {"productId": coklat, "qty": "1"}},
	}, http.StatusCreated)
	require.Equal(t, "in_progress", mp["status"], "order marketplace langsung Diproses")
	assert.Equal(t, "marketplace_input", items(mp, "lines")[0]["priceRule"])
	assert.Equal(t, "base_price", items(mp, "lines")[1]["priceRule"])
	assert.Equal(t, "500000.00", mp["totalAmount"])
	// Bundel dipecah: 4 coklat + 2 vanila + 1 coklat dari E-commerce Stock.
	assert.Equal(t, "5", h.reserved(coklat, h.ecm))
	assert.Equal(t, "2", h.reserved(vanila, h.ecm))
	assert.Equal(t, "0", h.reserved(coklat, h.fg), "FG Non-Ecommerce tidak dipakai marketplace")

	// Nomor order ganda ditolak.
	status, problem, _ := h.do(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "marketplace", "channelId": h.channel("marketplace"), "subChannelId": shopee, "externalOrderId": ext,
		"businessDate": "2026-11-02", "sourceLocationId": h.ecm, "lines": []map[string]any{{"productId": coklat, "qty": "1"}},
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, problem["fields"], "externalOrderId")

	// Sales MT tidak melihat order marketplace.
	h.as("sales.mt@balans.local")
	status, _, _ = h.do(http.MethodGet, "/api/v1/sales/orders/"+mp["id"].(string), nil)
	assert.Equal(t, http.StatusForbidden, status)

	// Serah ke kurir (logistik): A-10 Piutang Shopee 1211, voucher 4102, Penjualan 4101, HPP.
	ctx := actor.System(context.Background())
	sh, err := h.app.sales.MarkHandedOver(ctx, sales.ShipmentInput{
		OrderID: uuid.MustParse(mp["id"].(string)), SourceType: "delivery_order", SourceID: uuid.New(), SourceLabel: "DO/MP/1",
	})
	require.NoError(t, err)
	assert.Equal(t, "240000.00", sh.Cost.String())
	a10 := h.journal("A-10", "sales_shipment", sh.ID)
	d, _ := amountOf(a10, "1211")
	assert.Equal(t, "500000.00", d)
	d, _ = amountOf(a10, "4102")
	assert.Equal(t, "20000.00", d)
	_, c := amountOf(a10, "4101")
	assert.Equal(t, "520000.00", c)
	d, _ = amountOf(a10, "5101")
	_, c = amountOf(a10, "1311")
	assert.Equal(t, "240000.00", d)
	assert.Equal(t, "240000.00", c)
	assertDims(t, a10, "marketplace", "shopee")
	assert.Equal(t, "0", h.scalar(`SELECT count(*)::text FROM journal_entries WHERE posting_rule_code = 'A-06' AND source_id = $1`, sh.ID),
		"HPP marketplace ada di jurnal A-10")
	assert.Equal(t, "0", h.reserved(coklat, h.ecm))

	// Impor Excel (dataexchange `sales.marketplace_order`): satu baris = satu order, SKU bundel dipecah.
	sku := func(id uuid.UUID) string {
		return h.scalar(`SELECT sku FROM products WHERE id = $1`, id)
	}
	h.as("sales.ecommerce@balans.local")
	row := func(n int, values map[string]string) tabular.Row {
		r := tabular.NewRow(n)
		for k, v := range values {
			r.Values[k] = v
		}
		return r
	}
	imp := row(2, map[string]string{
		"marketplace": "lazada", "orderNumber": "LZD-" + h.tag, "orderDate": "2026-11-02",
		"items": sku(paket) + ":1:200000; " + sku(vanila) + ":2", "sellerVoucher": "5000", "buyerName": "Sari",
	})
	importRow := func(r tabular.Row, key string) error {
		return h.app.Tx.WithTx(h.ctx(), func(ctx context.Context) error { return h.app.sales.ImportMarketplaceRow(ctx, r, key) })
	}
	require.NoError(t, importRow(imp, "job-"+h.tag+":2"))
	require.NoError(t, importRow(imp, "job-"+h.tag+":2"), "baris yang sama idempotent")
	assert.Equal(t, "2", h.reserved(coklat, h.ecm))
	assert.Equal(t, "3", h.reserved(vanila, h.ecm))
	list := h.json(http.MethodGet, "/api/v1/sales/orders?orderType=marketplace&q=LZD-"+h.tag, nil, http.StatusOK)
	require.Len(t, items(list, "items"), 1)
	assert.Equal(t, "295000.00", items(list, "items")[0]["totalAmount"])

	// Baris berikutnya dengan nomor order sama ditolak dengan kolom Nomor Order.
	err = importRow(row(3, map[string]string{
		"marketplace": "lazada", "orderNumber": "LZD-" + h.tag, "orderDate": "2026-11-02", "items": sku(vanila) + ":1",
	}), "job-"+h.tag+":3")
	require.Error(t, err)
	err = importRow(row(4, map[string]string{
		"marketplace": "tokoabc", "orderNumber": "X-" + h.tag, "orderDate": "2026-11-02", "items": "TIDAKADA:1",
	}), "job-"+h.tag+":4")
	require.Error(t, err)

	// Batal order marketplace sebelum serah kurir hanya melepas alokasi.
	imported := h.order(items(list, "items")[0]["id"].(string))
	h.json(http.MethodPost, "/api/v1/sales/orders/"+imported["id"].(string)+"/cancel",
		map[string]any{"version": version(imported), "reason": "Dibatalkan pembeli"}, http.StatusOK)
	assert.Equal(t, "0", h.reserved(vanila, h.ecm))
}
