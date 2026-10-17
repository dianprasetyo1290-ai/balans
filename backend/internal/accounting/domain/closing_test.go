package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/money"
)

func TestCloseStepsFollowBRD(t *testing.T) {
	require.Len(t, CloseSteps, 9)
	for i, s := range CloseSteps {
		assert.Equal(t, i+1, s.Order)
		got, ok := StepByCode(s.Code)
		assert.True(t, ok)
		assert.Equal(t, s, got)
	}
	_, ok := StepByCode("unknown")
	assert.False(t, ok)
	dep, _ := StepByCode(StepDepreciation)
	assert.True(t, dep.Automatic)
}

func item(code CloseStepCode, status CloseStepStatus, checks ...CloseCheck) ChecklistItem {
	st, _ := StepByCode(code)
	return ChecklistItem{Step: st, Status: status, Checks: checks}
}

func TestChecklistApply(t *testing.T) {
	by, at := uuid.New(), time.Now()
	it := item(StepBankPettyCashReconciliation, StepPending)
	require.NoError(t, it.Apply(ChecklistUpdate{Status: StepDone, Note: " ok "}, by, at))
	assert.Equal(t, StepDone, it.Status)
	assert.Equal(t, "ok", it.Note)
	assert.Equal(t, &by, it.CompletedBy)
	require.NoError(t, it.Apply(ChecklistUpdate{Status: StepPending}, by, at))
	assert.Nil(t, it.CompletedAt)

	assert.Contains(t, fieldsOf(it.Apply(ChecklistUpdate{Status: StepSkipped}, by, at)), "note", "Tidak Berlaku wajib alasan")
	require.NoError(t, it.Apply(ChecklistUpdate{Status: StepSkipped, Note: "Belum ada payroll"}, by, at))
	assert.Contains(t, fieldsOf(it.Apply(ChecklistUpdate{Status: "x"}, by, at)), "status")

	blocked := item(StepOperationalDocuments, StepPending, CloseCheck{OK: false, Message: "Masih ada 2 Jurnal Umum draft"})
	err := blocked.Apply(ChecklistUpdate{Status: StepDone}, by, at)
	assert.Equal(t, "Masih ada 2 Jurnal Umum draft", fieldsOf(err)["status"])
	assert.False(t, blocked.ChecksPassed())

	last := item(StepFinancialReview, StepPending)
	assert.Contains(t, fieldsOf(last.Apply(ChecklistUpdate{Status: StepDone}, by, at)), "status", "langkah 9 lewat Tutup Periode")
	long := make([]byte, 1001)
	assert.Contains(t, fieldsOf(it.Apply(ChecklistUpdate{Status: StepPending, Note: string(long)}, by, at)), "note")
}

func TestCloseBlockers(t *testing.T) {
	var items []ChecklistItem
	for _, s := range CloseSteps {
		items = append(items, item(s.Code, StepDone))
	}
	items[8].Status = StepPending
	assert.Empty(t, CloseBlockers(items), "langkah 9 tidak menahan")
	items[2].Status = StepSkipped
	assert.Empty(t, CloseBlockers(items))
	items[0].Status = StepPending
	items[5].Checks = []CloseCheck{{OK: false, Message: "2 aset tetap belum disusutkan"}}
	b := CloseBlockers(items)
	require.Len(t, b, 2)
	assert.Contains(t, b[0], "Langkah 1")
	assert.Contains(t, b[1], "Langkah 6: 2 aset tetap belum disusutkan")
}

func TestPeriodCloseReopenTransitions(t *testing.T) {
	by := uuid.New()
	p := Period{Status: PeriodOpen, Year: 2027, Month: time.January}
	all := func(string) bool { return true }
	assert.Equal(t, []PeriodAction{PeriodActionEditChecklist, PeriodActionClose}, p.AllowedActions(all))
	assert.Error(t, p.RequestReopen())
	require.NoError(t, p.Close(time.Now(), by))
	assert.Error(t, p.CheckOpen(), "periode tertutup menolak transaksi")
	assert.Error(t, p.Close(time.Now(), by))
	assert.Equal(t, []PeriodAction{PeriodActionRequestReopen}, p.AllowedActions(all))
	assert.Empty(t, p.AllowedActions(func(string) bool { return false }))

	assert.Error(t, p.ApproveReopen())
	require.NoError(t, p.RequestReopen())
	assert.Equal(t, PeriodPendingApproval, p.Status)
	assert.Error(t, p.CheckOpen(), "menunggu persetujuan tetap terkunci")
	assert.Empty(t, p.AllowedActions(all))
	require.NoError(t, p.RejectReopen())
	assert.Equal(t, PeriodClosed, p.Status)
	assert.Error(t, p.RejectReopen())

	require.NoError(t, p.RequestReopen())
	require.NoError(t, p.ApproveReopen())
	assert.Equal(t, PeriodOpen, p.Status)
	assert.Equal(t, 1, p.ReopenedCount)
	assert.Nil(t, p.ClosedAt)

	_, err := ValidateReopenReason("  ")
	assert.Error(t, err)
	r, err := ValidateReopenReason(" koreksi HPP ")
	require.NoError(t, err)
	assert.Equal(t, "koreksi HPP", r)
}

func TestTrialBalanceTotals(t *testing.T) {
	rows := []TrialBalanceRow{
		{AccountCode: "1111", OpeningNet: money.FromInt(1000), PeriodDebit: money.FromInt(500), PeriodCredit: money.FromInt(200)},
		{AccountCode: "3101", OpeningNet: money.FromInt(-1000)},
		{AccountCode: "4101", PeriodCredit: money.FromInt(500), PeriodDebit: money.Zero},
		{AccountCode: "6301", PeriodDebit: money.FromInt(200), PeriodCredit: money.Zero},
	}
	assert.Equal(t, "1300.00", rows[0].ClosingNet().String())
	d, c := SplitNet(money.FromInt(-5))
	assert.Equal(t, "0.00", d.String())
	assert.Equal(t, "5.00", c.String())
	tot := TrialBalanceTotalsOf(rows)
	assert.True(t, tot.Balanced())
	assert.Equal(t, "1000.00", tot.OpeningDebit.String())
	assert.Equal(t, "1500.00", tot.ClosingDebit.String())
	rows[3].PeriodDebit = money.FromInt(201)
	assert.False(t, TrialBalanceTotalsOf(rows).Balanced())
}
