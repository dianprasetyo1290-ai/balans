package service_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/platform/apperr"
)

// --- Jenis biaya (S-124) ----------------------------------------------------------------------

func TestSeededExpenseTypes(t *testing.T) {
	h := setup(t)
	list, err := h.svc.ListExpenseTypes(h.ctx(), postgres.ExpenseTypeFilter{IncludeInactive: true})
	require.NoError(t, err)
	byCode := map[string]domain.ExpenseType{}
	for _, e := range list {
		byCode[e.Code] = e
	}
	for _, code := range []string{"ENDORSE_PERSON", "ENDORSE_AGENCY", "EVENT", "ADS", "RENT", "UTILITY", "OFFICE_SUPPLY",
		"SOFTWARE", "SHIPPING", "BANK_FEE", "REIMBURSE", "OTHER"} {
		assert.Contains(t, byCode, code)
	}
	rent := byCode["RENT"]
	assert.Equal(t, "6301", rent.ExpenseAccountCode)
	assert.Equal(t, domain.WithholdingPPh42, rent.WithholdingType)
	assert.Equal(t, domain.ApprovalSubjectExpenseClaim, rent.ApprovalSubject)
	assert.True(t, byCode["ENDORSE_PERSON"].RequiresCampaign)
	assert.Equal(t, domain.WithholdingPPh21, byCode["ENDORSE_PERSON"].WithholdingType)
	assert.Equal(t, "7202", byCode["BANK_FEE"].ExpenseAccountCode)

	got, err := h.svc.ExpenseTypeByCode(as(), " office_supply ")
	require.NoError(t, err)
	assert.Equal(t, "6303", got.ExpenseAccountCode)
	got, err = h.svc.ExpenseType(as(), got.ID)
	require.NoError(t, err)
	assert.Equal(t, "OFFICE_SUPPLY", got.Code)
	assert.Empty(t, got.UsableError())

	// Pengaju biaya (expense_claim.read) boleh melihat daftar untuk dropdown.
	_, err = h.svc.ListExpenseTypes(as(service.PermExpenseClaimRead), postgres.ExpenseTypeFilter{})
	require.NoError(t, err)
	_, err = h.svc.ListExpenseTypes(as(service.PermAccountRead), postgres.ExpenseTypeFilter{})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}

func TestExpenseTypeLifecycle(t *testing.T) {
	h := setup(t)
	ctx := as(service.PermExpenseTypeRead, service.PermExpenseTypeWrite, service.PermAccountRead, service.PermAccountWrite)
	code := "T_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	supplies := h.account("6303")

	e, err := h.svc.CreateExpenseType(ctx, domain.ExpenseTypeInput{
		Code: strings.ToLower(code), Name: "Biaya test", ExpenseAccountID: supplies.ID, RequiresReceipt: true, IsActive: true,
	})
	require.NoError(t, err)
	assert.Equal(t, code, e.Code)
	assert.Equal(t, "6303", e.ExpenseAccountCode)
	assert.Equal(t, domain.WithholdingNone, e.WithholdingType)
	assert.Equal(t, domain.ApprovalSubjectExpenseClaim, e.ApprovalSubject)

	_, err = h.svc.CreateExpenseType(ctx, domain.ExpenseTypeInput{Code: code, Name: "x", ExpenseAccountID: supplies.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode))

	// Akun bukan beban, akun induk, dan akun tidak dikenal ditolak.
	for _, id := range []uuid.UUID{h.account("1101").ID, h.account("6000").ID, uuid.New()} {
		_, err = h.svc.CreateExpenseType(ctx, domain.ExpenseTypeInput{Code: code + "X", Name: "x", ExpenseAccountID: id})
		var ae *apperr.Error
		require.ErrorAs(t, err, &ae)
		assert.Contains(t, ae.Fields, "expenseAccountId")
	}

	// Ubah: akun beban pindah ke 7202, PPh 23, approval kas kecil.
	e, err = h.svc.UpdateExpenseType(ctx, e.ID, e.Version, domain.ExpenseTypeInput{
		Name: "Biaya test 2", ExpenseAccountID: h.account("7202").ID, WithholdingType: domain.WithholdingPPh23,
		ApprovalSubject: domain.ApprovalSubjectPettyCashReplenishment, RequiresCampaign: true, IsActive: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "7202", e.ExpenseAccountCode)
	assert.Equal(t, "Beban Bank", e.ExpenseAccountName)
	assert.False(t, e.IsActive)
	assert.NotEmpty(t, e.UsableError())
	_, err = h.svc.UpdateExpenseType(ctx, e.ID, e.Version-1, domain.ExpenseTypeInput{Name: "x", ExpenseAccountID: supplies.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))

	active, err := h.svc.ListExpenseTypes(ctx, postgres.ExpenseTypeFilter{Query: code})
	require.NoError(t, err)
	assert.Empty(t, active, "nonaktif tidak tampil di dropdown")
	all, err := h.svc.ListExpenseTypes(ctx, postgres.ExpenseTypeFilter{IncludeInactive: true, Query: code})
	require.NoError(t, err)
	require.Len(t, all, 1)

	// Akun yang dipakai jenis biaya tidak bisa dihapus atau dijadikan akun induk.
	acc, err := h.svc.CreateAccount(ctx, domain.AccountInput{
		Code: "6397", Name: "Beban test jenis biaya", NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true,
	})
	require.NoError(t, err)
	e2, err := h.svc.CreateExpenseType(ctx, domain.ExpenseTypeInput{Code: code + "Y", Name: "y", ExpenseAccountID: acc.ID, IsActive: true})
	require.NoError(t, err)
	assert.True(t, apperr.HasCode(h.svc.DeleteAccount(ctx, acc.ID, acc.Version), apperr.CodeInvalidStateTransition))
	_, err = h.svc.UpdateAccount(ctx, acc.ID, acc.Version, domain.AccountInput{Name: acc.Name, NormalBalance: domain.NormalDebit, IsActive: true})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
	require.NoError(t, h.svc.DeleteExpenseType(ctx, e2.ID, e2.Version))
	require.NoError(t, h.svc.DeleteAccount(ctx, acc.ID, acc.Version))

	// Hapus: tidak tampil lagi, tidak bisa dibaca modul lain, kode tidak bisa dipakai ulang.
	assert.True(t, apperr.HasCode(h.svc.DeleteExpenseType(ctx, e.ID, e.Version-1), apperr.CodeVersionConflict))
	require.NoError(t, h.svc.DeleteExpenseType(ctx, e.ID, e.Version))
	_, err = h.svc.ExpenseType(ctx, e.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))
	_, err = h.svc.CreateExpenseType(ctx, domain.ExpenseTypeInput{Code: code, Name: "x", ExpenseAccountID: supplies.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode))

	_, err = h.svc.CreateExpenseType(as(service.PermExpenseTypeRead), domain.ExpenseTypeInput{Code: "NOPE", Name: "x", ExpenseAccountID: supplies.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}
