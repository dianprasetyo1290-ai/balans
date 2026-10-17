package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type server struct {
	t   *testing.T
	srv *httptest.Server
	mod *catalog.Module
}

func setup(t *testing.T) *server {
	t.Helper()
	pool := dbtest.Pool(t)
	for _, q := range []string{`DELETE FROM product_bundle_components`, `DELETE FROM product_barcodes`, `DELETE FROM products`, `DELETE FROM product_categories`} {
		_, err := pool.Exec(context.Background(), q)
		require.NoError(t, err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mod := catalog.New(catalog.Deps{
		Pool: pool, Tx: db.NewTxManager(pool), Clock: clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)),
		Responder: httpx.NewResponder(log, nil), Audit: audit.New(audit.Deps{Pool: pool}),
	})
	// Peran diatur per request lewat header X-Test-Role: admin (Admin Sistem) atau pembaca.
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			perms := map[string]struct{}{catalog.PermProductRead: {}}
			roles := []string{"sales_mt"}
			if req.Header.Get("X-Test-Role") == "admin" {
				perms[catalog.PermProductWrite] = struct{}{}
				roles = []string{"system_admin"}
			}
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), actor.Actor{UserID: uuid.New(), FullName: "Uji", Roles: roles, Permissions: perms})))
		})
	})
	mod.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &server{t: t, srv: srv, mod: mod}
}

func (s *server) do(method, path, role string, body any) (int, map[string]any) {
	s.t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(s.t, err)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.srv.URL+path, buf)
	require.NoError(s.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("X-Test-Role", role)
	res, err := http.DefaultClient.Do(req)
	require.NoError(s.t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(s.t, err)
	out := map[string]any{}
	if len(raw) > 0 && raw[0] == '{' {
		require.NoError(s.t, json.Unmarshal(raw, &out))
	} else if len(raw) > 0 {
		var list []any
		require.NoError(s.t, json.Unmarshal(raw, &list))
		out["list"] = list
	}
	return res.StatusCode, out
}

func TestProductAPI(t *testing.T) {
	s := setup(t)
	status, cat := s.do(http.MethodPost, "/api/v1/product-categories", "admin", map[string]any{"code": "meal", "name": "Meal"})
	require.Equal(t, http.StatusCreated, status, cat)

	status, p := s.do(http.MethodPost, "/api/v1/products", "admin", map[string]any{
		"sku": "ms-cok-30", "name": "Meal Shake", "variant": "Coklat 30 g", "productType": "finished_good", "isBundle": false,
		"packagingType": "tube", "uom": "pcs", "shelfLifeDays": 365, "hasBatch": false, "isActive": true,
		"categoryId": cat["id"], "unitsPerCarton": 24, "minStockQty": "10",
		"barcodes": []map[string]any{{"barcode": "8991234567891", "isPrimary": true}},
	})
	require.Equal(t, http.StatusCreated, status, p)
	assert.Equal(t, "MS-COK-30", p["sku"])
	assert.Equal(t, "8991234567891", p["primaryBarcode"])
	assert.Equal(t, "Meal", p["categoryName"])
	assert.Equal(t, "10.000", p["minStockQty"])
	assert.Equal(t, true, p["hasBatch"])
	assert.Equal(t, []any{"update", "delete"}, p["allowedActions"])
	id := p["id"].(string)

	status, pkg := s.do(http.MethodPost, "/api/v1/products", "admin", map[string]any{
		"sku": "PKG-T30", "name": "Tube 30 ml", "productType": "packaging", "isBundle": false, "packagingType": "tube",
		"uom": "pcs", "hasBatch": true, "isActive": true,
	})
	require.Equal(t, http.StatusCreated, status, pkg)

	status, bundle := s.do(http.MethodPost, "/api/v1/products", "admin", map[string]any{
		"sku": "BDL-1", "name": "Paket", "productType": "finished_good", "isBundle": true, "uom": "paket", "hasBatch": false,
		"isActive": true, "bundleComponents": []map[string]any{{"componentProductId": id, "qty": "2"}},
	})
	require.Equal(t, http.StatusCreated, status, bundle)
	comps := bundle["bundleComponents"].([]any)
	require.Len(t, comps, 1)
	assert.Equal(t, "2.000", comps[0].(map[string]any)["qty"])
	assert.Equal(t, "MS-COK-30", comps[0].(map[string]any)["componentSku"])

	status, list := s.do(http.MethodGet, "/api/v1/products?productType=packaging&productType=reprocess_material", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, float64(1), list["totalItems"])
	status, list = s.do(http.MethodGet, "/api/v1/products?isBundle=false&productType=finished_good&sort=-sku", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, float64(1), list["totalItems"])
	items := list["items"].([]any)
	assert.Empty(t, items[0].(map[string]any)["allowedActions"], "pembaca tidak punya aksi")

	status, got := s.do(http.MethodGet, "/api/v1/products/lookup?code=8991234567891", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, id, got["id"])

	status, upd := s.do(http.MethodPut, "/api/v1/products/"+id, "admin", map[string]any{
		"name": "Meal Shake Baru", "variant": "Coklat 30 g", "packagingType": "tube", "uom": "pcs", "shelfLifeDays": 180,
		"hasBatch": true, "isActive": true, "version": 1, "barcodes": []map[string]any{{"barcode": "8991234567891", "isPrimary": true}},
	})
	require.Equal(t, http.StatusOK, status, upd)
	assert.Equal(t, "Meal Shake Baru", upd["name"])
	assert.Equal(t, "MS-COK-30", upd["sku"])

	status, prob := s.do(http.MethodPut, "/api/v1/products/"+id, "admin", map[string]any{
		"name": "X", "uom": "pcs", "hasBatch": true, "isActive": true, "version": 1, "minStockQty": "abc",
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Equal(t, "VALIDATION_FAILED", prob["code"])

	status, prob = s.do(http.MethodPost, "/api/v1/products", "reader", map[string]any{"sku": "X", "name": "X", "productType": "packaging", "isBundle": false, "uom": "pcs", "hasBatch": false, "isActive": true})
	assert.Equal(t, http.StatusForbidden, status, prob)

	// Hapus komponen bundel aktif ditolak; hapus bundel lalu komponen, lalu pulihkan.
	status, _ = s.do(http.MethodDelete, "/api/v1/products/"+id, "admin", nil)
	assert.Equal(t, http.StatusConflict, status)
	status, _ = s.do(http.MethodDelete, "/api/v1/products/"+bundle["id"].(string), "admin", nil)
	require.Equal(t, http.StatusNoContent, status)
	status, _ = s.do(http.MethodDelete, "/api/v1/products/"+id, "admin", nil)
	require.Equal(t, http.StatusNoContent, status)
	status, got = s.do(http.MethodGet, "/api/v1/products/"+id, "admin", nil)
	require.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, got["deletedAt"])
	assert.Equal(t, []any{"restore"}, got["allowedActions"])
	status, list = s.do(http.MethodGet, "/api/v1/products?deleted=true", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, float64(2), list["totalItems"])
	status, got = s.do(http.MethodPost, "/api/v1/products/"+id+"/restore", "admin", nil)
	require.Equal(t, http.StatusOK, status, got)
	assert.Nil(t, got["deletedAt"])

	// Kategori.
	catID := cat["id"].(string)
	status, c := s.do(http.MethodPut, "/api/v1/product-categories/"+catID, "admin", map[string]any{"name": "Meal Replacement", "version": 1})
	require.Equal(t, http.StatusOK, status, c)
	status, _ = s.do(http.MethodDelete, "/api/v1/product-categories/"+catID, "admin", nil)
	require.Equal(t, http.StatusNoContent, status)
	status, cats := s.do(http.MethodGet, "/api/v1/product-categories?deleted=true", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Len(t, cats["list"], 1)
	status, c = s.do(http.MethodPost, "/api/v1/product-categories/"+catID+"/restore", "admin", nil)
	require.Equal(t, http.StatusOK, status, c)
}

func TestPublicAPI(t *testing.T) {
	s := setup(t)
	status, p := s.do(http.MethodPost, "/api/v1/products", "admin", map[string]any{
		"sku": "MS-COK-30", "name": "Meal Shake", "productType": "finished_good", "isBundle": false, "packagingType": "tube",
		"uom": "pcs", "shelfLifeDays": 365, "hasBatch": true, "isActive": true,
		"barcodes": []map[string]any{{"barcode": "MSCOK30", "isPrimary": true}},
	})
	require.Equal(t, http.StatusCreated, status, p)
	id := uuid.MustParse(p["id"].(string))
	status, b := s.do(http.MethodPost, "/api/v1/products", "admin", map[string]any{
		"sku": "BDL-1", "name": "Paket", "productType": "finished_good", "isBundle": true, "uom": "paket", "hasBatch": false,
		"isActive": true, "bundleComponents": []map[string]any{{"componentProductId": id, "qty": "3"}},
	})
	require.Equal(t, http.StatusCreated, status, b)
	bundleID := uuid.MustParse(b["id"].(string))

	ctx := context.Background()
	got, err := s.mod.Product(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "MS-COK-30", got.SKU)
	got, err = s.mod.ProductByCode(ctx, "MSCOK30")
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	all, err := s.mod.Products(ctx, []uuid.UUID{id, bundleID})
	require.NoError(t, err)
	assert.Len(t, all, 2)
	no := false
	opts, err := s.mod.ProductOptions(ctx, catalog.OptionFilter{Types: []catalog.ProductType{catalog.TypeFinishedGood}, IsBundle: &no})
	require.NoError(t, err)
	require.Len(t, opts, 1)

	comps, err := s.mod.BundleComponents(ctx, bundleID)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	avail := catalog.AvailableBundleQty(comps, map[uuid.UUID]decimal.Decimal{id: decimal.NewFromInt(10)})
	assert.True(t, avail.Equal(decimal.NewFromInt(3)))
	need := catalog.ExplodeBundle(comps, decimal.NewFromInt(2))
	assert.True(t, need[0].Qty.Equal(decimal.NewFromInt(6)))
	_, err = s.mod.BundleComponents(ctx, uuid.New())
	require.Error(t, err)

	assert.Equal(t, map[string]string{"product": "catalog.product.read", "product_category": "catalog.product.read"}, catalog.AuditEntities())
	assert.Equal(t, "catalog", s.mod.Name())
	assert.Nil(t, s.mod.PeriodicJobs())
}
