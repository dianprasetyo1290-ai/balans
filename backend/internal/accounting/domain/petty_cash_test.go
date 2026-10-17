package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

func fundInput() domain.FundInput {
	return domain.FundInput{Code: " kk-ho ", Name: "Kas Kecil HO", HolderUserID: uuid.New(), DivisionID: uuid.New(), IsActive: true}
}

func TestPettyCashFund(t *testing.T) {
	f, err := domain.NewFund(fundInput())
	require.NoError(t, err)
	assert.Equal(t, "KK-HO", f.Code)
	assert.Equal(t, domain.PettyCashDefaultCeiling, f.Ceiling, "plafon kosong = default")
	assert.True(t, f.Balance.IsZero())
	assert.Equal(t, f.Ceiling, f.Shortfall())

	tests := []struct {
		name  string
		mod   func(in *domain.FundInput)
		field string
	}{
		{"kode", func(in *domain.FundInput) { in.Code = "!" }, "code"},
		{"nama", func(in *domain.FundInput) { in.Name = "" }, "name"},
		{"pemegang", func(in *domain.FundInput) { in.HolderUserID = uuid.Nil }, "holderUserId"},
		{"divisi", func(in *domain.FundInput) { in.DivisionID = uuid.Nil }, "divisionId"},
		{"plafon", func(in *domain.FundInput) { in.Ceiling = rp(-5) }, "ceilingAmount"},
		{"keterangan", func(in *domain.FundInput) { in.Description = string(make([]byte, 501)) }, "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fundInput()
			tt.mod(&in)
			_, err := domain.NewFund(in)
			assert.Contains(t, fieldsOf(t, err), tt.field)
		})
	}

	f.Balance = rp(3_000_000)
	in := fundInput()
	in.Ceiling = rp(2_000_000)
	assert.Contains(t, fieldsOf(t, f.Update(in)), "ceilingAmount")
	in.Ceiling = rp(4_000_000)
	require.NoError(t, f.Update(in))
	assert.Equal(t, "1000000.00", f.Shortfall().String())
}

func pettyInput(t domain.PettyCashType) domain.PettyCashInput {
	et := uuid.New()
	file := uuid.New()
	return domain.PettyCashInput{
		Type: t, FundID: uuid.New(), BusinessDate: oct20, ExpenseTypeID: &et, DivisionID: uuid.New(), Description: " ATK ",
		Amount: rp(150_000), ReceiptFileID: &file,
	}
}

func TestPettyCashExpense(t *testing.T) {
	tx, err := domain.NewPettyCashTxn(pettyInput(domain.PettyCashExpense))
	require.NoError(t, err)
	assert.Equal(t, "ATK", tx.Description)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionPost, domain.ActionCancel}, tx.Actions())
	tx.FundCode = "KK-HO"
	assert.Equal(t, "Draft pengeluaran kas kecil KK-HO", tx.Label())

	fund := domain.PettyCashFund{Code: "KK-HO", IsActive: true, Ceiling: rp(5_000_000), Balance: rp(1_000_000)}
	et := domain.ExpenseType{Name: "ATK", IsActive: true}
	require.NoError(t, tx.CheckExpense(fund, et, domain.PettyCashMaxPerTransaction))

	big := tx
	big.Amount = rp(1_500_000)
	assert.Contains(t, fieldsOf(t, big.CheckExpense(fund, et, domain.PettyCashMaxPerTransaction)), "amount")
	low := tx
	low.Amount = rp(900_000)
	poor := fund
	poor.Balance = rp(100_000)
	assert.Contains(t, fieldsOf(t, low.CheckExpense(poor, et, domain.PettyCashMaxPerTransaction)), "amount")
	noReceipt := tx
	noReceipt.ReceiptFileID = nil
	inactive := fund
	inactive.IsActive = false
	campaignType := domain.ExpenseType{Name: "Event", IsActive: false, RequiresCampaign: true}
	assert.ElementsMatch(t, []string{"receiptFileId", "fundId", "expenseTypeId", "campaignId"},
		fieldsOf(t, noReceipt.CheckExpense(inactive, campaignType, domain.PettyCashMaxPerTransaction)))
	assert.True(t, apperr.HasCode(tx.CheckReplenishment(fund), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(tx.Submit(), apperr.CodeInvalidStateTransition))

	upd := pettyInput(domain.PettyCashExpense)
	upd.Amount = rp(200_000)
	tx.ExpenseTypeCode = "OFFICE_SUPPLY"
	require.NoError(t, tx.Update(upd))
	assert.Equal(t, rp(200_000), tx.Amount)
	assert.Empty(t, tx.ExpenseTypeCode, "jenis biaya berganti → kode tampilan dikosongkan")
	upd.Amount = money.Zero
	assert.Error(t, tx.Update(upd))

	require.NoError(t, tx.Post("KK/2026/10/00001", uuid.New(), time.Now(), uuid.New()))
	assert.Equal(t, domain.StatusPosted, tx.Status)
	assert.Equal(t, "KK/2026/10/00001", tx.Label())
	assert.Empty(t, tx.Actions())
	assert.True(t, apperr.HasCode(tx.Update(upd), apperr.CodeInvalidStateTransition))
	assert.Error(t, tx.Cancel("x"))
}

func TestPettyCashReplenishment(t *testing.T) {
	in := pettyInput(domain.PettyCashReplenishment)
	tx, err := domain.NewPettyCashTxn(in)
	require.NoError(t, err)
	assert.Nil(t, tx.ExpenseTypeID, "pengisian tanpa jenis biaya")
	assert.Nil(t, tx.ReceiptFileID)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, tx.Actions())
	tx.FundCode = "KK-HO"
	assert.Equal(t, "Draft pengisian kas kecil KK-HO", tx.Label())

	fund := domain.PettyCashFund{IsActive: true, Ceiling: rp(5_000_000), Balance: rp(4_900_000)}
	assert.Contains(t, fieldsOf(t, tx.CheckReplenishment(fund)), "amount")
	fund.Balance = rp(1_000_000)
	require.NoError(t, tx.CheckReplenishment(fund))
	fund.IsActive = false
	assert.Contains(t, fieldsOf(t, tx.CheckReplenishment(fund)), "fundId")
	assert.True(t, apperr.HasCode(tx.CheckExpense(fund, domain.ExpenseType{}, rp(1)), apperr.CodeInvalidStateTransition))

	assert.Error(t, tx.Post("KK/1", uuid.New(), time.Now(), uuid.New()), "pengisian harus disetujui dulu")
	require.NoError(t, tx.Submit())
	assert.Empty(t, tx.Actions())
	require.NoError(t, tx.Reject("plafon terlalu besar"))
	assert.Equal(t, []domain.Action{domain.ActionRevise, domain.ActionCancel}, tx.Actions())
	require.NoError(t, tx.Revise())
	require.NoError(t, tx.Submit())
	require.NoError(t, tx.Approve("ok"))
	assert.Equal(t, []domain.Action{domain.ActionPost, domain.ActionCancel}, tx.Actions())
	require.NoError(t, tx.Post("KK/2", uuid.New(), time.Now(), uuid.New()))
	assert.Error(t, tx.Approve(""))

	other, err := domain.NewPettyCashTxn(in)
	require.NoError(t, err)
	assert.Contains(t, fieldsOf(t, other.Cancel(" ")), "reason")
	require.NoError(t, other.Cancel("batal"))
	assert.Equal(t, domain.StatusCancelled, other.Status)
}

func TestNewPettyCashValidation(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(in *domain.PettyCashInput)
		field string
	}{
		{"jenis", func(in *domain.PettyCashInput) { in.Type = "x" }, "type"},
		{"dana", func(in *domain.PettyCashInput) { in.FundID = uuid.Nil }, "fundId"},
		{"tanggal", func(in *domain.PettyCashInput) { in.BusinessDate = clock.BusinessDate{} }, "businessDate"},
		{"jenis biaya", func(in *domain.PettyCashInput) { in.ExpenseTypeID = nil }, "expenseTypeId"},
		{"divisi", func(in *domain.PettyCashInput) { in.DivisionID = uuid.Nil }, "divisionId"},
		{"nominal", func(in *domain.PettyCashInput) { in.Amount = money.Zero }, "amount"},
		{"keterangan", func(in *domain.PettyCashInput) { in.Description = string(make([]byte, 301)) }, "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := pettyInput(domain.PettyCashExpense)
			tt.mod(&in)
			_, err := domain.NewPettyCashTxn(in)
			assert.Contains(t, fieldsOf(t, err), tt.field)
		})
	}
}
