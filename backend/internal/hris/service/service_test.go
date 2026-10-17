package service_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/hris/domain"
	"balans/internal/hris/postgres"
	"balans/internal/hris/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type harness struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *service.Service
	clock *clock.Fixed
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	cipher, err := pii.New("k1", map[string][]byte{"k1": bytes.Repeat([]byte{3}, 32)}, bytes.Repeat([]byte{4}, 32))
	require.NoError(t, err)
	fixed := clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC))
	svc := service.New(service.Deps{
		Store: postgres.NewStore(pool, cipher), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}), Clock: fixed,
	})
	return &harness{t: t, pool: pool, svc: svc, clock: fixed}
}

func ctx(perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: uuid.New(), Permissions: set})
}

var hrga = ctx(service.PermEmployeeRead, service.PermEmployeeWrite)

func code(err error) apperr.Code { return apperr.As(err).Code }

func nik() string { return fmt.Sprintf("3201%012d", rand.Int64N(1_000_000_000_000)) }

func ptr[T any](v T) *T { return &v }

func (h *harness) create(mutate ...func(*domain.Input)) domain.Employee {
	h.t.Helper()
	in := domain.Input{
		Number: "BN-" + strings.ToUpper(uuid.NewString()[:8]), FullName: "Rudi Hartono", PositionTitle: "Staf Gudang",
		JoinDate: clock.NewBusinessDate(2024, 3, 1), NIK: ptr(nik()),
	}
	for _, m := range mutate {
		m(&in)
	}
	e, err := h.svc.Create(hrga, in)
	require.NoError(h.t, err)
	return e
}

func TestEmployeeNIKEncryptedAndSearchable(t *testing.T) {
	h := setup(t)
	n := nik()
	e := h.create(func(in *domain.Input) { in.NIK = ptr(n[:4] + " " + n[4:]) })
	assert.Equal(t, n, e.NIK)

	var enc string
	var hash []byte
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT nik_enc, nik_hash FROM employees WHERE id = $1`, e.ID).Scan(&enc, &hash))
	assert.NotContains(t, enc, n, "NIK terenkripsi")
	assert.Len(t, hash, 32)

	res, err := h.svc.List(hrga, service.Query{NIK: n}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, e.ID, res.Items[0].ID)
	_, err = h.svc.List(hrga, service.Query{NIK: "123"}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeValidationFailed, code(err))

	// NIK dan nomor karyawan unik.
	_, err = h.svc.Create(hrga, domain.Input{Number: "X-" + uuid.NewString()[:6], FullName: "Lain", PositionTitle: "Staf",
		JoinDate: clock.NewBusinessDate(2024, 1, 1), NIK: ptr(n)})
	assert.Contains(t, apperr.As(err).Fields, "nik")
	_, err = h.svc.Create(hrga, domain.Input{Number: strings.ToLower(e.Number), FullName: "Lain", PositionTitle: "Staf",
		JoinDate: clock.NewBusinessDate(2024, 1, 1)})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))

	var changes string
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT changes::text FROM audit_logs WHERE entity_id = $1`, e.ID).Scan(&changes))
	assert.NotContains(t, changes, n, "audit trail tidak memuat NIK")
}

func TestEmployeeResignEndsActiveStatus(t *testing.T) {
	h := setup(t)
	e := h.create()
	active, err := h.svc.IsActiveEmployee(context.Background(), e.ID, h.svc.Today())
	require.NoError(t, err)
	assert.True(t, active)

	resignOn := clock.NewBusinessDate(2026, 10, 31)
	resigned, err := h.svc.Resign(hrga, e.ID, e.Version, resignOn, "Pindah kota")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusResigned, resigned.Status)
	_, err = h.svc.Resign(hrga, e.ID, resigned.Version, resignOn, "Lagi")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	_, err = h.svc.Resign(hrga, e.ID, e.Version, resignOn, "Lagi")
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	for d, want := range map[clock.BusinessDate]bool{clock.NewBusinessDate(2026, 10, 30): true, resignOn: false, clock.NewBusinessDate(2027, 1, 1): false} {
		active, err := h.svc.IsActiveEmployee(context.Background(), e.ID, d)
		require.NoError(t, err)
		assert.Equal(t, want, active, d.String())
	}
	active, err = h.svc.IsActiveEmployee(context.Background(), uuid.New(), resignOn)
	require.NoError(t, err)
	assert.False(t, active, "karyawan tidak dikenal dianggap tidak aktif")

	got, err := h.svc.Get(hrga, e.ID)
	require.NoError(t, err)
	assert.Equal(t, resignOn, *got.ResignDate)
	assert.Equal(t, "Pindah kota", got.ResignReason)

	res, err := h.svc.List(hrga, service.Query{Search: e.Number, Status: domain.StatusResigned}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, res.Items, 1)
}

func TestEmployeeUpdateOptionsAndPermissions(t *testing.T) {
	h := setup(t)
	e := h.create(func(in *domain.Input) { in.FullName = "Wulan Options " + uuid.NewString()[:4] })
	updated, err := h.svc.Update(hrga, e.ID, e.Version, domain.Input{
		FullName: e.FullName, PositionTitle: "Supervisor Gudang", JoinDate: e.JoinDate, Phone: "0812 9999 0000",
	})
	require.NoError(t, err)
	assert.Equal(t, e.Version+1, updated.Version)
	assert.Equal(t, e.NIK, updated.NIK, "NIK tidak dikirim = tetap")
	assert.Equal(t, "+6281299990000", updated.Phone)

	// CS (party.customer.write) boleh mencari karyawan untuk ditautkan, tetapi tidak membaca data karyawan.
	cs := ctx(service.PermCustomerWrite)
	opts, err := h.svc.Options(cs, e.FullName)
	require.NoError(t, err)
	require.Len(t, opts, 1)
	_, err = h.svc.Get(cs, e.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.Options(ctx(), "x")
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.Create(ctx(service.PermEmployeeRead), domain.Input{})
	assert.Equal(t, apperr.CodeForbidden, code(err))

	list, err := h.svc.ByIDs(context.Background(), []uuid.UUID{e.ID})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Supervisor Gudang", list[0].PositionTitle)
}
