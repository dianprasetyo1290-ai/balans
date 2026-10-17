package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

func claimInput(types ...uuid.UUID) domain.ClaimInput {
	in := domain.ClaimInput{
		DivisionID: uuid.New(), PayeeName: " Agensi Kreatif ", PayeeTaxID: "01.234.567.8-901.000", ClaimDate: oct20,
		Description: "Event", Lines: []domain.ClaimLineInput{},
	}
	for i, t := range types {
		in.Lines = append(in.Lines, domain.ClaimLineInput{ExpenseTypeID: t, Amount: rp(int64(1_000_000 * (i + 1))), Description: " baris "})
	}
	return in
}

func TestNewExpenseClaim(t *testing.T) {
	typeA := uuid.New()
	c, err := domain.NewExpenseClaim(claimInput(typeA, typeA), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.ClaimExpense, c.ClaimType)
	assert.Equal(t, domain.PayeeOther, c.PayeeType)
	assert.Equal(t, "Agensi Kreatif", c.PayeeName)
	assert.Equal(t, "012345678901000", c.PayeeTaxID)
	assert.Equal(t, "3000000.00", c.Total.String())
	assert.Equal(t, c.Total, c.Payable)
	assert.Equal(t, 2, c.Lines[1].LineNo)
	assert.Equal(t, "baris", c.Lines[0].Description)
	assert.Equal(t, "Draft pengajuan biaya", c.Label())
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, c.Actions())

	tests := []struct {
		name  string
		mod   func(in *domain.ClaimInput)
		field string
	}{
		{"jenis", func(in *domain.ClaimInput) { in.ClaimType = "x" }, "claimType"},
		{"divisi", func(in *domain.ClaimInput) { in.DivisionID = uuid.Nil }, "divisionId"},
		{"penerima karyawan", func(in *domain.ClaimInput) { in.PayeeType = domain.PayeeEmployee }, "payeeId"},
		{"jenis penerima", func(in *domain.ClaimInput) { in.PayeeType = "bank" }, "payeeType"},
		{"nama penerima", func(in *domain.ClaimInput) { in.PayeeName = "" }, "payeeName"},
		{"npwp", func(in *domain.ClaimInput) { in.PayeeTaxID = "123" }, "payeeTaxId"},
		{"tanggal", func(in *domain.ClaimInput) { in.ClaimDate = clock.BusinessDate{} }, "claimDate"},
		{"keterangan", func(in *domain.ClaimInput) { in.Description = string(make([]byte, 501)) }, "description"},
		{"tanpa baris", func(in *domain.ClaimInput) { in.Lines = nil }, "lines"},
		{"terlalu banyak baris", func(in *domain.ClaimInput) { in.Lines = make([]domain.ClaimLineInput, domain.MaxClaimLines+1) }, "lines"},
		{"jenis biaya baris", func(in *domain.ClaimInput) { in.Lines[0].ExpenseTypeID = uuid.Nil }, "lines.0.expenseTypeId"},
		{"nominal baris", func(in *domain.ClaimInput) { in.Lines[0].Amount = money.Zero }, "lines.0.amount"},
		{"uraian baris", func(in *domain.ClaimInput) { in.Lines[0].Description = string(make([]byte, 301)) }, "lines.0.description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := claimInput(typeA)
			tt.mod(&in)
			_, err := domain.NewExpenseClaim(in, uuid.New())
			assert.Contains(t, fieldsOf(t, err), tt.field)
		})
	}

	// Penerima lain tidak menyimpan ID penerima.
	in := claimInput(typeA)
	id := uuid.New()
	in.PayeeID = &id
	c2, err := domain.NewExpenseClaim(in, uuid.New())
	require.NoError(t, err)
	assert.Nil(t, c2.PayeeID)

	upd := claimInput(typeA)
	upd.Description = "Revisi"
	require.NoError(t, c.Update(upd))
	assert.Equal(t, "Revisi", c.Description)
	upd.Lines = nil
	assert.Error(t, c.Update(upd))
}

func TestExpenseClaimCheckSubmit(t *testing.T) {
	rent, ads := uuid.New(), uuid.New()
	c, err := domain.NewExpenseClaim(claimInput(rent, ads), uuid.New())
	require.NoError(t, err)
	types := map[uuid.UUID]domain.ExpenseType{
		rent: {ID: rent, Name: "Sewa", IsActive: true, RequiresReceipt: true},
		ads:  {ID: ads, Name: "Iklan", IsActive: false, RequiresCampaign: true},
	}
	fields := fieldsOf(t, c.CheckSubmit(types))
	assert.ElementsMatch(t, []string{"lines.0.receiptFileId", "lines.1.expenseTypeId", "campaignId"}, fields)

	file, camp := uuid.New(), uuid.New()
	c.Lines[0].ReceiptFileID, c.CampaignID = &file, &camp
	types[ads] = domain.ExpenseType{ID: ads, Name: "Iklan", IsActive: true, RequiresCampaign: true}
	require.NoError(t, c.CheckSubmit(types))

	delete(types, ads)
	assert.Contains(t, fieldsOf(t, c.CheckSubmit(types)), "lines.1.expenseTypeId")
}

func approvedClaim(t *testing.T, types ...uuid.UUID) domain.ExpenseClaim {
	t.Helper()
	c, err := domain.NewExpenseClaim(claimInput(types...), uuid.New())
	require.NoError(t, err)
	for i := range c.Lines {
		c.Lines[i].ID = uuid.New()
	}
	require.NoError(t, c.Submit("EXP/2026/10/00001", time.Now()))
	require.NoError(t, c.Approve(time.Now(), "ok"))
	return c
}

func TestExpenseClaimWorkflow(t *testing.T) {
	c, err := domain.NewExpenseClaim(claimInput(uuid.New()), uuid.New())
	require.NoError(t, err)
	require.NoError(t, c.Submit("EXP/1", time.Now()))
	assert.Equal(t, "EXP/1", c.Label())
	assert.Empty(t, c.Actions())
	assert.True(t, apperr.HasCode(c.Update(claimInput(uuid.New())), apperr.CodeInvalidStateTransition))
	assert.Error(t, c.Submit("EXP/2", time.Now()))
	require.NoError(t, c.Reject(time.Now(), "kurang bukti"))
	assert.Equal(t, "kurang bukti", c.DecisionNote)
	assert.Equal(t, []domain.Action{domain.ActionRevise, domain.ActionCancel}, c.Actions())
	assert.Error(t, c.Approve(time.Now(), ""))
	require.NoError(t, c.Revise())
	assert.Equal(t, domain.StatusDraft, c.Status)
	assert.Contains(t, fieldsOf(t, c.Cancel("")), "reason")
	require.NoError(t, c.Cancel("tidak jadi"))
	assert.Equal(t, domain.StatusCancelled, c.Status)
	assert.Error(t, c.Cancel("lagi"))
	assert.Error(t, c.Revise())
	assert.Error(t, c.Reject(time.Now(), ""))
}

func TestExpenseClaimVerifyAndPost(t *testing.T) {
	event, office, rent := uuid.New(), uuid.New(), uuid.New()
	c := approvedClaim(t, event, event, office, rent)
	assert.Equal(t, []domain.Action{domain.ActionVerify}, c.Actions())
	c.Lines[0].ExpenseAccountCode, c.Lines[1].ExpenseAccountCode = "6102", "6102"
	c.Lines[2].ExpenseAccountCode, c.Lines[3].ExpenseAccountCode = "6303", "6301"
	c.Lines[0].ExpenseTypeName = "Biaya event"

	defaults := map[uuid.UUID]domain.WithholdingType{event: domain.WithholdingPPh23, office: domain.WithholdingNone, rent: domain.WithholdingPPh42}
	groups, err := c.TaxGroups([]domain.LineTaxChoice{{LineID: c.Lines[3].ID, Type: domain.WithholdingPPh42, ObjectCode: "28-403-01"}}, defaults)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, domain.WithholdingPPh23, groups[0].Type)
	assert.Equal(t, "3000000.00", groups[0].Gross.String(), "baris 1 (1 jt) + baris 2 (2 jt)")
	assert.Len(t, groups[0].LineIDs, 2)
	assert.Equal(t, "28-403-01", groups[1].ObjectCode)
	assert.Equal(t, "4000000.00", groups[1].Gross.String())

	_, err = c.TaxGroups([]domain.LineTaxChoice{{LineID: uuid.New()}}, defaults)
	assert.Error(t, err)
	_, err = c.TaxGroups([]domain.LineTaxChoice{{LineID: c.Lines[0].ID, Type: "ppn"}}, defaults)
	assert.Error(t, err)
	none, err := c.TaxGroups([]domain.LineTaxChoice{
		{LineID: c.Lines[0].ID, Type: domain.WithholdingNone}, {LineID: c.Lines[1].ID, Type: domain.WithholdingNone},
		{LineID: c.Lines[3].ID, Type: domain.WithholdingNone},
	}, defaults)
	require.NoError(t, err)
	assert.Empty(t, none)

	results := []domain.TaxResult{
		{Group: groups[0], ObjectCode: "24-104-14", RatePercent: decimal.NewFromInt(2), Amount: rp(60_001)},
		{Group: groups[1], ObjectCode: "28-403-01", RatePercent: decimal.NewFromInt(10), Amount: rp(400_000)},
	}
	require.NoError(t, c.Verify(results, oct20, time.Now(), uuid.New()))
	assert.Equal(t, domain.StatusAwaitingPayment, c.Status)
	assert.Equal(t, "460001.00", c.Withholding.String())
	assert.Equal(t, "9539999.00", c.Payable.String())
	assert.Equal(t, money.Sum(c.Lines[0].WithholdingAmount, c.Lines[1].WithholdingAmount).String(), "60001.00", "alokasi tanpa selisih")
	assert.Equal(t, "24-104-14", c.Lines[0].ObjectCode)
	assert.Equal(t, domain.WithholdingNone, c.Lines[2].WithholdingType)
	assert.Equal(t, []domain.Action{domain.ActionPay}, c.Actions())

	splits, amounts := c.PostingSplits()
	assert.Equal(t, "9539999.00", amounts[domain.AmountPayable].String())
	sum := map[string]string{}
	for _, s := range splits {
		sum[s.AmountKey+":"+s.AccountCode] = s.Amount.String()
	}
	assert.Equal(t, map[string]string{
		"expense_value:6102": "3000000.00", "expense_value:6303": "3000000.00", "expense_value:6301": "4000000.00",
		"withholding_value:2302": "60001.00", "withholding_value:2303": "400000.00",
	}, sum)

	assert.Error(t, c.Verify(results, oct20, time.Now(), uuid.New()), "verifikasi hanya dari Disetujui")
	require.NoError(t, c.MarkPaid(uuid.New(), "BYR/1", time.Now()))
	assert.Equal(t, domain.StatusPaid, c.Status)
	assert.Error(t, c.MarkPaid(uuid.New(), "BYR/2", time.Now()))
}

func TestExpenseClaimVerifyRejects(t *testing.T) {
	typ := uuid.New()
	c := approvedClaim(t, typ)
	g := domain.TaxGroup{Type: domain.WithholdingPPh23, LineIDs: []uuid.UUID{c.Lines[0].ID}, Gross: c.Total}
	assert.Error(t, c.Verify([]domain.TaxResult{{Group: g, Amount: c.Total}}, oct20, time.Now(), uuid.New()), "potongan ≥ total")
	bad := domain.TaxGroup{Type: domain.WithholdingPPh23, LineIDs: []uuid.UUID{uuid.New()}}
	assert.Error(t, c.Verify([]domain.TaxResult{{Group: bad, Amount: rp(1)}}, oct20, time.Now(), uuid.New()))
	zero := domain.TaxGroup{Type: domain.WithholdingPPh23}
	assert.Error(t, c.Verify([]domain.TaxResult{{Group: zero, Amount: rp(1)}}, oct20, time.Now(), uuid.New()), "kelompok tanpa baris")
}

func TestWithholdingAccountCode(t *testing.T) {
	assert.Equal(t, "2301", domain.WithholdingAccountCode(domain.WithholdingPPh21))
	assert.Equal(t, "2302", domain.WithholdingAccountCode(domain.WithholdingPPh23))
	assert.Equal(t, "2303", domain.WithholdingAccountCode(domain.WithholdingPPh42))
	assert.Empty(t, domain.WithholdingAccountCode(domain.WithholdingNone))
}
