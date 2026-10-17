package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
	"balans/internal/warehouse/service"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type harness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	svc    *service.Service
	clock  *clock.Fixed
	admin  context.Context
	head   context.Context
	reader context.Context
}

var allPerms = []string{service.PermWarehouseRead, service.PermWarehouseWrite, service.PermLocationRead, service.PermLocationWrite}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	// Audit trail append-only; ID entitas selalu baru sehingga riwayat antar test tidak tercampur.
	for _, q := range []string{`DELETE FROM locations`, `DELETE FROM warehouses`} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	fixed := clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 123456789, time.UTC))
	svc := service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}), Clock: fixed,
	})
	as := func(name string, roles []string, perms ...string) context.Context {
		set := map[string]struct{}{}
		for _, p := range perms {
			set[p] = struct{}{}
		}
		return actor.With(ctx, actor.Actor{UserID: uuid.New(), FullName: name, Roles: roles, Permissions: set})
	}
	return &harness{
		t: t, pool: pool, svc: svc, clock: fixed,
		admin:  as("Ani Admin", []string{"system_admin"}, allPerms...),
		head:   as("Hadi Head Logistik", []string{"head_logistics"}, allPerms...),
		reader: as("Sari Sales", []string{"sales_mt"}, service.PermWarehouseRead, service.PermLocationRead),
	}
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func (h *harness) auditActions(entityID uuid.UUID) []string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `SELECT action FROM audit_logs WHERE entity_id = $1 ORDER BY created_at, id`, entityID)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(h.t, rows.Scan(&s))
		out = append(out, s)
	}
	return out
}

func (h *harness) locations(f service.LocationFilter) []domain.Location {
	h.t.Helper()
	res, err := h.svc.ListLocations(h.reader, f, paging.New(nil, nil))
	require.NoError(h.t, err)
	return res.Items
}

func codes(locs []domain.Location) []string {
	out := make([]string, len(locs))
	for i, l := range locs {
		out[i] = l.Code
	}
	return out
}

func TestCreateWarehouseCreatesStandardLocations(t *testing.T) {
	h := setup(t)
	w, locs, err := h.svc.CreateWarehouse(h.head, domain.WarehouseInput{Code: "utama", Name: "Gudang Utama", Address: "Jl. Raya 1", IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "UTAMA", w.Code)
	require.Len(t, locs, 7)

	list := h.locations(service.LocationFilter{WarehouseID: &w.ID})
	assert.Equal(t, []string{"UTAMA-INB", "UTAMA-FG", "UTAMA-ECM", "UTAMA-PTY", "UTAMA-RM", "UTAMA-GM", "UTAMA-RET"}, codes(list))
	for _, l := range list {
		assert.True(t, l.IsStandard)
		assert.Equal(t, "Gudang Utama", l.WarehouseName)
		assert.Equal(t, []string{"create"}, h.auditActions(l.ID))
	}
	assert.Equal(t, []string{"create"}, h.auditActions(w.ID))
	assert.Equal(t, []string{"UTAMA-FG", "UTAMA-ECM"}, codes(h.locations(service.LocationFilter{
		Types: []domain.LocationType{domain.LocationFinishedGoods, domain.LocationEcommerce},
	})))

	_, _, err = h.svc.CreateWarehouse(h.head, domain.WarehouseInput{Code: "UTAMA", Name: "Lain", IsActive: true})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))
	_, _, err = h.svc.CreateWarehouse(h.reader, domain.WarehouseInput{Code: "G2", Name: "Lain", IsActive: true})
	assert.Equal(t, apperr.CodeForbidden, code(err))

	// Kode lokasi standar bentrok dengan lokasi tambahan yang sudah ada: seluruh pembuatan gudang dibatalkan.
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{Code: "G2-INB", Name: "Event", Type: domain.LocationEvent, WarehouseID: &w.ID, IsActive: true})
	require.NoError(t, err)
	_, _, err = h.svc.CreateWarehouse(h.head, domain.WarehouseInput{Code: "G2", Name: "Gudang 2", IsActive: true})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
	res, err := h.svc.ListWarehouses(h.reader, service.WarehouseFilter{}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, res.Items, 1, "gudang G2 tidak tersimpan")
}

func TestUpdateWarehouseAndLocation(t *testing.T) {
	h := setup(t)
	w, locs, err := h.svc.CreateWarehouse(h.head, domain.WarehouseInput{Code: "UTAMA", Name: "Gudang Utama", IsActive: true})
	require.NoError(t, err)

	w, err = h.svc.UpdateWarehouse(h.head, w.ID, w.Version, domain.WarehouseInput{Name: "Gudang Pusat", Address: "Bekasi", IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "Gudang Pusat", w.Name)
	assert.Equal(t, 2, w.Version)
	_, err = h.svc.UpdateWarehouse(h.head, w.ID, 1, domain.WarehouseInput{Name: "X", IsActive: true})
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	l, err := h.svc.UpdateLocation(h.head, locs[2].ID, locs[2].Version, "Petty Stock", false)
	require.NoError(t, err)
	assert.Equal(t, "Petty Stock", l.Name)
	assert.False(t, l.IsActive)
	_, err = h.svc.UpdateLocation(h.head, locs[2].ID, locs[2].Version, "Lagi", true)
	assert.Equal(t, apperr.CodeVersionConflict, code(err))
	assert.Equal(t, []string{"create", "update"}, h.auditActions(l.ID))

	got, err := h.svc.GetWarehouse(h.reader, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "Gudang Pusat", got.Name)
	gotLoc, err := h.svc.GetLocation(h.reader, l.ID)
	require.NoError(t, err)
	assert.Equal(t, "Gudang Pusat", gotLoc.WarehouseName)

	// Lokasi standar bisa diubah tetapi tidak bisa dihapus sendiri.
	assert.Equal(t, []string{"update"}, service.LocationActions(h.head, l))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteLocation(h.head, l.ID)))
}

func TestExternalLocations(t *testing.T) {
	h := setup(t)
	partner, supplier := uuid.New(), uuid.New()
	ksg, err := h.svc.CreateLocation(h.head, domain.LocationInput{
		Code: "KSG-SINAR", Name: "Konsinyasi – Toko Sinar Jaya", Type: domain.LocationConsignment,
		PartyType: domain.PartyPartner, PartyID: &partner, IsActive: true,
	})
	require.NoError(t, err)
	assert.True(t, ksg.IsSellable)
	assert.Nil(t, ksg.WarehouseID)
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{
		Code: "MKL-NUTRI", Name: "Maklon Nutri Sehat", Type: domain.LocationTollManufacturer,
		PartyType: domain.PartySupplier, PartyID: &supplier, IsActive: true,
	})
	require.NoError(t, err)

	// Satu lokasi per jenis per pihak.
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{
		Code: "KSG-SINAR-2", Name: "Duplikat", Type: domain.LocationConsignment, PartyType: domain.PartyPartner, PartyID: &partner, IsActive: true,
	})
	assert.Equal(t, map[string]string{"externalPartyId": "Pihak ini sudah punya lokasi dengan jenis yang sama"}, apperr.As(err).Fields)
	// Kode unik.
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{
		Code: "KSG-SINAR", Name: "Duplikat", Type: domain.LocationVendingBuffer, PartyType: domain.PartyPartner, PartyID: &partner, IsActive: true,
	})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))
	// Lokasi internal di gudang yang tidak ada.
	missing := uuid.New()
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationEvent, WarehouseID: &missing, IsActive: true})
	assert.Equal(t, map[string]string{"warehouseId": "Gudang tidak ditemukan atau sudah dihapus"}, apperr.As(err).Fields)
	_, err = h.svc.CreateLocation(h.reader, domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationEvent, WarehouseID: &missing})
	assert.Equal(t, apperr.CodeForbidden, code(err))

	ext, yes := true, true
	assert.Equal(t, []string{"KSG-SINAR", "MKL-NUTRI"}, codes(h.locations(service.LocationFilter{External: &ext, Sort: paging.Sort{Field: "code"}})))
	assert.Equal(t, []string{"KSG-SINAR"}, codes(h.locations(service.LocationFilter{PartyType: domain.PartyPartner, PartyID: &partner})))
	assert.Equal(t, []string{"MKL-NUTRI"}, codes(h.locations(service.LocationFilter{Search: "nutri", IsActive: &yes})))

	found, err := h.svc.FindLocations(context.Background(), service.LocationFilter{Types: []domain.LocationType{domain.LocationConsignment}, PartyID: &partner})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, ksg.ID, found[0].ID)

	// Hapus lalu pulihkan lokasi eksternal; setelah dihapus pihak boleh mendapat lokasi baru, sehingga pemulihan
	// lokasi lama ditolak.
	require.NoError(t, h.svc.DeleteLocation(h.head, ksg.ID))
	assert.Empty(t, h.locations(service.LocationFilter{PartyID: &partner}))
	old, err := h.svc.LocationByID(context.Background(), ksg.ID)
	require.NoError(t, err)
	assert.True(t, old.Deleted(), "lokasi terhapus tetap terbaca lewat ID")
	_, err = h.svc.RestoreLocation(h.head, ksg.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	restored, err := h.svc.RestoreLocation(h.admin, ksg.ID)
	require.NoError(t, err)
	assert.False(t, restored.Deleted())
	assert.Equal(t, []string{"create", "delete", "restore"}, h.auditActions(ksg.ID))

	require.NoError(t, h.svc.DeleteLocation(h.head, ksg.ID))
	_, err = h.svc.CreateLocation(h.head, domain.LocationInput{
		Code: "KSG-SINAR-B", Name: "Konsinyasi baru", Type: domain.LocationConsignment, PartyType: domain.PartyPartner, PartyID: &partner, IsActive: true,
	})
	require.NoError(t, err)
	_, err = h.svc.RestoreLocation(h.admin, ksg.ID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
}

func TestDeleteAndRestoreWarehouseCascades(t *testing.T) {
	h := setup(t)
	w, locs, err := h.svc.CreateWarehouse(h.head, domain.WarehouseInput{Code: "UTAMA", Name: "Gudang Utama", IsActive: true})
	require.NoError(t, err)
	event, err := h.svc.CreateLocation(h.head, domain.LocationInput{Code: "UTAMA-EVT", Name: "Event", Type: domain.LocationEvent, WarehouseID: &w.ID, IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"update", "delete"}, service.LocationActions(h.head, event))

	// Lokasi tambahan yang dihapus lebih dulu tetap terhapus saat gudang dipulihkan.
	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.DeleteLocation(h.head, event.ID))
	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.DeleteWarehouse(h.head, w.ID))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteWarehouse(h.head, w.ID)))

	res, err := h.svc.ListWarehouses(h.reader, service.WarehouseFilter{}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Empty(t, res.Items)
	assert.Empty(t, h.locations(service.LocationFilter{WarehouseID: &w.ID}))
	deleted, err := h.svc.ListWarehouses(h.reader, service.WarehouseFilter{Deleted: true}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, deleted.Items, 1)
	assert.Equal(t, []string{"restore"}, service.RecordActions(h.admin, deleted.Items[0].Mark, service.PermWarehouseWrite))
	assert.Empty(t, service.RecordActions(h.head, deleted.Items[0].Mark, service.PermWarehouseWrite))
	assert.Equal(t, []string{"create", "delete"}, h.auditActions(locs[0].ID))

	// Lokasi di gudang terhapus harus dipulihkan lewat gudangnya.
	_, err = h.svc.RestoreLocation(h.admin, locs[0].ID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	_, err = h.svc.UpdateWarehouse(h.head, w.ID, deleted.Items[0].Version, domain.WarehouseInput{Name: "X", IsActive: true})
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))

	_, err = h.svc.RestoreWarehouse(h.head, w.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	w, err = h.svc.RestoreWarehouse(h.admin, w.ID)
	require.NoError(t, err)
	assert.False(t, w.Deleted())
	assert.Equal(t, []string{"UTAMA-INB", "UTAMA-FG", "UTAMA-ECM", "UTAMA-PTY", "UTAMA-RM", "UTAMA-GM", "UTAMA-RET"},
		codes(h.locations(service.LocationFilter{WarehouseID: &w.ID})))
	assert.Equal(t, []string{"create", "delete", "restore"}, h.auditActions(locs[0].ID))
	assert.Equal(t, []string{"create", "delete", "restore"}, h.auditActions(w.ID))

	byID, err := h.svc.LocationsByIDs(context.Background(), []uuid.UUID{event.ID, locs[0].ID})
	require.NoError(t, err)
	assert.True(t, byID[event.ID].Deleted())
	assert.False(t, byID[locs[0].ID].Deleted())
	got, err := h.svc.WarehouseByID(context.Background(), w.ID)
	require.NoError(t, err)
	assert.Equal(t, "UTAMA", got.Code)

	_, err = h.svc.RestoreLocation(h.admin, event.ID)
	require.NoError(t, err)
}

func TestSeedDevIsIdempotent(t *testing.T) {
	h := setup(t)
	require.NoError(t, h.svc.SeedDev(context.Background()))
	require.NoError(t, h.svc.SeedDev(context.Background()))
	assert.Len(t, h.locations(service.LocationFilter{}), 7)
}
