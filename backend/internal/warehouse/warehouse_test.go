package warehouse_test

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/warehouse"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type server struct {
	t   *testing.T
	srv *httptest.Server
	mod *warehouse.Module
}

func setup(t *testing.T) *server {
	t.Helper()
	pool := dbtest.Pool(t)
	for _, q := range []string{`DELETE FROM locations`, `DELETE FROM warehouses`} {
		_, err := pool.Exec(context.Background(), q)
		require.NoError(t, err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mod := warehouse.New(warehouse.Deps{
		Pool: pool, Tx: db.NewTxManager(pool), Clock: clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)),
		Responder: httpx.NewResponder(log, nil), Audit: audit.New(audit.Deps{Pool: pool}),
	})
	// Peran diatur per request lewat header X-Test-Role: admin (Admin Sistem) atau pembaca.
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			perms := map[string]struct{}{warehouse.PermWarehouseRead: {}, warehouse.PermLocationRead: {}}
			roles := []string{"sales_mt"}
			if req.Header.Get("X-Test-Role") == "admin" {
				perms["warehouse.warehouse.write"], perms["warehouse.location.write"] = struct{}{}, struct{}{}
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
	if len(raw) > 0 {
		require.NoError(s.t, json.Unmarshal(raw, &out))
	}
	return res.StatusCode, out
}

func TestWarehouseAPI(t *testing.T) {
	s := setup(t)
	status, w := s.do(http.MethodPost, "/api/v1/warehouses", "admin", map[string]any{"code": "utama", "name": "Gudang Utama", "address": "Bekasi", "isActive": true})
	require.Equal(t, http.StatusCreated, status, w)
	id := w["id"].(string)
	assert.Equal(t, "UTAMA", w["code"])

	status, list := s.do(http.MethodGet, "/api/v1/locations?warehouseId="+id+"&locationType=inbound&locationType=returns", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	items := list["items"].([]any)
	require.Len(t, items, 2)
	inb := items[0].(map[string]any)
	assert.Equal(t, "UTAMA-INB", inb["code"])
	assert.Equal(t, "Inbound", inb["locationTypeLabel"])
	assert.Equal(t, "Gudang Utama", inb["warehouseName"])
	assert.Equal(t, true, inb["isStandard"])
	assert.Empty(t, inb["allowedActions"])

	partner := uuid.NewString()
	status, ext := s.do(http.MethodPost, "/api/v1/locations", "admin", map[string]any{
		"code": "KSG-SINAR", "name": "Konsinyasi – Toko Sinar Jaya", "locationType": "consignment",
		"externalPartyType": "partner", "externalPartyId": partner, "isActive": true,
	})
	require.Equal(t, http.StatusCreated, status, ext)
	assert.Equal(t, true, ext["isExternal"])
	assert.Equal(t, true, ext["isSellable"])
	assert.Equal(t, []any{"update", "delete"}, ext["allowedActions"])

	status, list = s.do(http.MethodGet, "/api/v1/locations?external=true&externalPartyType=partner&externalPartyId="+partner, "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, float64(1), list["totalItems"])

	status, upd := s.do(http.MethodPut, "/api/v1/locations/"+ext["id"].(string), "admin", map[string]any{"name": "Konsinyasi Sinar", "isActive": true, "version": 1})
	require.Equal(t, http.StatusOK, status, upd)
	status, got := s.do(http.MethodGet, "/api/v1/locations/"+ext["id"].(string), "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "Konsinyasi Sinar", got["name"])
	status, _ = s.do(http.MethodDelete, "/api/v1/locations/"+ext["id"].(string), "admin", nil)
	require.Equal(t, http.StatusNoContent, status)
	status, got = s.do(http.MethodPost, "/api/v1/locations/"+ext["id"].(string)+"/restore", "admin", nil)
	require.Equal(t, http.StatusOK, status, got)

	status, upd = s.do(http.MethodPut, "/api/v1/warehouses/"+id, "admin", map[string]any{"name": "Gudang Pusat", "isActive": true, "version": 1})
	require.Equal(t, http.StatusOK, status, upd)
	status, prob := s.do(http.MethodPut, "/api/v1/warehouses/"+id, "admin", map[string]any{"name": "Lagi", "isActive": true, "version": 1})
	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, "VERSION_CONFLICT", prob["code"])
	status, _ = s.do(http.MethodPost, "/api/v1/warehouses", "reader", map[string]any{"code": "G2", "name": "G2", "isActive": true})
	assert.Equal(t, http.StatusForbidden, status)

	status, _ = s.do(http.MethodDelete, "/api/v1/warehouses/"+id, "admin", nil)
	require.Equal(t, http.StatusNoContent, status)
	status, list = s.do(http.MethodGet, "/api/v1/warehouses?deleted=true&sort=-name&q=pusat", "reader", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, float64(1), list["totalItems"])
	status, got = s.do(http.MethodGet, "/api/v1/warehouses/"+id, "admin", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, []any{"restore"}, got["allowedActions"])
	status, got = s.do(http.MethodPost, "/api/v1/warehouses/"+id+"/restore", "admin", nil)
	require.Equal(t, http.StatusOK, status, got)
}

func TestPublicAPI(t *testing.T) {
	s := setup(t)
	status, w := s.do(http.MethodPost, "/api/v1/warehouses", "admin", map[string]any{"code": "UTAMA", "name": "Gudang Utama", "isActive": true})
	require.Equal(t, http.StatusCreated, status, w)
	whID := uuid.MustParse(w["id"].(string))
	supplier := uuid.New()
	status, ext := s.do(http.MethodPost, "/api/v1/locations", "admin", map[string]any{
		"code": "MKL-NUTRI", "name": "Maklon Nutri Sehat", "locationType": "toll_manufacturer",
		"externalPartyType": "supplier", "externalPartyId": supplier, "isActive": true,
	})
	require.Equal(t, http.StatusCreated, status, ext)

	ctx := context.Background()
	got, err := s.mod.Warehouse(ctx, whID)
	require.NoError(t, err)
	assert.Equal(t, "UTAMA", got.Code)

	inbound, err := s.mod.FindLocations(ctx, warehouse.LocationQuery{WarehouseID: &whID, Types: []warehouse.LocationType{warehouse.LocationInbound}})
	require.NoError(t, err)
	require.Len(t, inbound, 1)
	assert.Equal(t, "UTAMA-INB", inbound[0].Code)

	loc, err := s.mod.Location(ctx, inbound[0].ID)
	require.NoError(t, err)
	assert.Equal(t, warehouse.LocationInbound, loc.Type)
	byID, err := s.mod.Locations(ctx, []uuid.UUID{loc.ID})
	require.NoError(t, err)
	assert.Len(t, byID, 1)

	l, ok, err := s.mod.ExternalLocation(ctx, warehouse.LocationTollManufacturer, supplier)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "MKL-NUTRI", l.Code)
	_, ok, err = s.mod.ExternalLocation(ctx, warehouse.LocationConsignment, supplier)
	require.NoError(t, err)
	assert.False(t, ok)

	assert.Equal(t, map[string]string{"warehouse": "warehouse.warehouse.read", "location": "warehouse.location.read"}, warehouse.AuditEntities())
	assert.Equal(t, "warehouse", s.mod.Name())
	assert.Nil(t, s.mod.PeriodicJobs())
}
