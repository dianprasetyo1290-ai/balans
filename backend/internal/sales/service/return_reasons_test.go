package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
	"balans/internal/sales/service"
)

func asUser(perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: uuid.New(), FullName: "Admin", Permissions: set})
}

func newReasonService(t *testing.T) *service.ReturnReasonService {
	t.Helper()
	pool := dbtest.Pool(t)
	return service.NewReturnReasonService(postgres.NewStore(pool), db.NewTxManager(pool), audit.New(audit.Deps{Pool: pool}))
}

func TestSeededReturnReasons(t *testing.T) {
	svc := newReasonService(t)
	ctx := asUser(service.PermSalesReturnRead)
	all, err := svc.List(ctx, postgres.ReturnReasonFilter{})
	require.NoError(t, err)
	codes := map[string]domain.ReturnReason{}
	for _, r := range all {
		codes[r.Code] = r
	}
	for _, c := range []string{"DAMAGED_SHIPPING", "EXPIRED", "WRONG_ITEM", "PRODUCTION_DEFECT", "CHANGED_MIND", "QC_REJECT", "OTHER"} {
		assert.Contains(t, codes, c)
	}
	assert.Equal(t, domain.DispositionDispose, codes["EXPIRED"].DefaultDisposition)
	assert.Empty(t, codes["OTHER"].DefaultDisposition)

	purchase, err := svc.List(asUser(service.PermPurchaseReturnRead), postgres.ReturnReasonFilter{AppliesTo: domain.ScopePurchaseReturn})
	require.NoError(t, err)
	for _, r := range purchase {
		assert.True(t, r.AppliesTo.Covers(domain.ScopePurchaseReturn), r.Code)
		assert.NotEqual(t, "EXPIRED", r.Code)
	}

	got, err := svc.GetByCode(context.Background(), "qc_reject")
	require.NoError(t, err)
	assert.Equal(t, domain.ScopePurchaseReturn, got.AppliesTo)
	assert.NotEmpty(t, got.UsableError(domain.ScopeSalesReturn))

	_, err = svc.List(asUser("sales.sales_order.read"), postgres.ReturnReasonFilter{})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}

func TestReturnReasonLifecycle(t *testing.T) {
	svc := newReasonService(t)
	ctx := asUser(service.PermReturnReasonRead, service.PermReturnReasonWrite)
	code := "T_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))

	r, err := svc.Create(ctx, domain.ReturnReasonInput{
		Code: strings.ToLower(code), Name: "Segel rusak", AppliesTo: domain.ScopeSalesReturn,
		DefaultDisposition: domain.DispositionRepack, IsActive: true,
	})
	require.NoError(t, err)
	assert.Equal(t, code, r.Code)
	assert.Equal(t, 1, r.Version)

	_, err = svc.Create(ctx, domain.ReturnReasonInput{Code: code, Name: "x", AppliesTo: domain.ScopeBoth})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode))
	_, err = svc.Create(ctx, domain.ReturnReasonInput{Code: code + "Z", Name: "x"})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))

	r, err = svc.Update(ctx, r.ID, r.Version, domain.ReturnReasonInput{Name: "Segel rusak/terbuka", AppliesTo: domain.ScopeBoth, IsActive: false})
	require.NoError(t, err)
	assert.Equal(t, 2, r.Version)
	assert.Empty(t, r.DefaultDisposition)
	assert.False(t, r.IsActive)
	_, err = svc.Update(ctx, r.ID, 1, domain.ReturnReasonInput{Name: "x", AppliesTo: domain.ScopeBoth})
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))
	_, err = svc.Update(ctx, r.ID, r.Version, domain.ReturnReasonInput{Name: "", AppliesTo: domain.ScopeBoth})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))

	active, err := svc.List(ctx, postgres.ReturnReasonFilter{Query: code})
	require.NoError(t, err)
	assert.Empty(t, active, "nonaktif tidak tampil di dropdown")
	all, err := svc.List(ctx, postgres.ReturnReasonFilter{IncludeInactive: true, Query: strings.ToLower(code)})
	require.NoError(t, err)
	require.Len(t, all, 1)

	got, err := svc.Get(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "Segel rusak/terbuka", got.Name)

	assert.True(t, apperr.HasCode(svc.Delete(ctx, r.ID, 1), apperr.CodeVersionConflict))
	require.NoError(t, svc.Delete(ctx, r.ID, r.Version))
	_, err = svc.Get(ctx, r.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))
	assert.True(t, apperr.HasCode(svc.Delete(ctx, r.ID, r.Version+1), apperr.CodeNotFound))
	_, err = svc.Create(ctx, domain.ReturnReasonInput{Code: code, Name: "x", AppliesTo: domain.ScopeBoth})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode), "kode tidak bisa dipakai ulang")

	_, err = svc.Create(asUser(service.PermReturnReasonRead), domain.ReturnReasonInput{Code: "NOPE", Name: "x", AppliesTo: domain.ScopeBoth})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
	_, err = svc.Update(asUser(), r.ID, 1, domain.ReturnReasonInput{})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
	assert.True(t, apperr.HasCode(svc.Delete(asUser(), r.ID, 1), apperr.CodeForbidden))
}
