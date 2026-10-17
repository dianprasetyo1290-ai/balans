package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
)

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	out := make([]string, 0, len(ae.Fields))
	for k := range ae.Fields {
		out = append(out, k)
	}
	return out
}

func TestNewExpenseType(t *testing.T) {
	acc := uuid.New()
	tests := []struct {
		name   string
		in     domain.ExpenseTypeInput
		fields []string
	}{
		{"valid + default", domain.ExpenseTypeInput{Code: " office_supply ", Name: " Perlengkapan ", ExpenseAccountID: acc, IsActive: true}, nil},
		{"kode pendek", domain.ExpenseTypeInput{Code: "A", Name: "x", ExpenseAccountID: acc}, []string{"code"}},
		{"kode diawali angka", domain.ExpenseTypeInput{Code: "1ABC", Name: "x", ExpenseAccountID: acc}, []string{"code"}},
		{"nama kosong", domain.ExpenseTypeInput{Code: "ABC", Name: " ", ExpenseAccountID: acc}, []string{"name"}},
		{"nama panjang", domain.ExpenseTypeInput{Code: "ABC", Name: strings.Repeat("a", 151), ExpenseAccountID: acc}, []string{"name"}},
		{"akun kosong", domain.ExpenseTypeInput{Code: "ABC", Name: "x"}, []string{"expenseAccountId"}},
		{"PPh salah", domain.ExpenseTypeInput{Code: "ABC", Name: "x", ExpenseAccountID: acc, WithholdingType: "pph22"}, []string{"withholdingType"}},
		{"approval salah", domain.ExpenseTypeInput{Code: "ABC", Name: "x", ExpenseAccountID: acc, ApprovalSubject: "purchase_order"}, []string{"approvalSubject"}},
		{"keterangan panjang", domain.ExpenseTypeInput{Code: "ABC", Name: "x", ExpenseAccountID: acc, Description: strings.Repeat("a", 501)}, []string{"description"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := domain.NewExpenseType(tt.in)
			if tt.fields == nil {
				require.NoError(t, err)
				assert.Equal(t, "OFFICE_SUPPLY", e.Code)
				assert.Equal(t, "Perlengkapan", e.Name)
				assert.Equal(t, domain.WithholdingNone, e.WithholdingType)
				assert.Equal(t, domain.ApprovalSubjectExpenseClaim, e.ApprovalSubject)
				assert.Empty(t, e.UsableError())
				return
			}
			assert.ElementsMatch(t, tt.fields, fieldsOf(t, err))
		})
	}
}

func TestExpenseTypeUpdate(t *testing.T) {
	acc, other := uuid.New(), uuid.New()
	e, err := domain.NewExpenseType(domain.ExpenseTypeInput{Code: "RENT", Name: "Sewa", ExpenseAccountID: acc, IsActive: true})
	require.NoError(t, err)
	e.ExpenseAccountCode, e.ExpenseAccountName = "6301", "Beban Sewa"

	require.NoError(t, e.Update(domain.ExpenseTypeInput{
		Code: "IGNORED", Name: "Sewa gudang", ExpenseAccountID: acc, WithholdingType: domain.WithholdingPPh42,
		RequiresReceipt: true, ApprovalSubject: domain.ApprovalSubjectPettyCashReplenishment, IsActive: false,
	}))
	assert.Equal(t, "RENT", e.Code, "kode tidak berubah")
	assert.Equal(t, "6301", e.ExpenseAccountCode, "akun sama, label tetap")
	assert.Equal(t, domain.WithholdingPPh42, e.WithholdingType)
	assert.True(t, e.RequiresReceipt)
	assert.Contains(t, e.UsableError(), "tidak aktif")

	require.NoError(t, e.Update(domain.ExpenseTypeInput{Name: "Sewa", ExpenseAccountID: other, RequiresCampaign: true, IsActive: true}))
	assert.Empty(t, e.ExpenseAccountCode, "akun berubah, label dibaca ulang store")
	assert.True(t, e.RequiresCampaign)

	err = e.Update(domain.ExpenseTypeInput{Name: "", ExpenseAccountID: other})
	assert.ElementsMatch(t, []string{"name"}, fieldsOf(t, err))
}

func TestValidateExpenseAccount(t *testing.T) {
	ok := domain.Account{Code: "6303", Group: domain.GroupExpense, NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true}
	require.NoError(t, domain.ValidateExpenseAccount(ok))
	other := domain.Account{Code: "7202", Group: domain.GroupOther, NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true}
	require.NoError(t, domain.ValidateExpenseAccount(other))

	for name, a := range map[string]domain.Account{
		"aset":                 {Code: "1101", Group: domain.GroupAsset, NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true},
		"pendapatan lain-lain": {Code: "7101", Group: domain.GroupOther, NormalBalance: domain.NormalCredit, IsPostable: true, IsActive: true},
		"induk":                {Code: "6000", Group: domain.GroupExpense, NormalBalance: domain.NormalDebit, IsActive: true},
		"nonaktif":             {Code: "6399", Group: domain.GroupExpense, NormalBalance: domain.NormalDebit, IsPostable: true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.ElementsMatch(t, []string{"expenseAccountId"}, fieldsOf(t, domain.ValidateExpenseAccount(a)))
		})
	}
}

func TestExpenseTypeLabels(t *testing.T) {
	assert.Equal(t, "PPh 4(2)", domain.WithholdingPPh42.Label())
	assert.Equal(t, "Tidak dipotong", domain.WithholdingNone.Label())
	assert.False(t, domain.WithholdingType("x").Valid())
	assert.Equal(t, "Pengajuan biaya / reimbursement", domain.ApprovalSubjectLabel(domain.ApprovalSubjectExpenseClaim))
	assert.Empty(t, domain.ApprovalSubjectLabel("x"))
}

func TestAccountUsageExpenseTypes(t *testing.T) {
	a := domain.Account{Code: "6303", Group: domain.GroupExpense, NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true}
	usage := domain.AccountUsage{ExpenseTypes: 1}
	assert.True(t, usage.Used())
	assert.True(t, apperr.HasCode(a.CheckDelete(usage), apperr.CodeInvalidStateTransition))
	err := a.Update(domain.AccountInput{Name: "x", NormalBalance: domain.NormalDebit, IsActive: true}, usage)
	assert.ElementsMatch(t, []string{"isPostable"}, fieldsOf(t, err))
}
