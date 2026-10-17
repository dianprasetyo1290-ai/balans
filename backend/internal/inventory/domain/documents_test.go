package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/inventory/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

var (
	day   = clock.NewBusinessDate(2026, 10, 22)
	actor = domain.Actor{ID: uuid.New(), Name: "Uji"}
)

func all(string) bool { return true }

func none(string) bool { return false }

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var e *apperr.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, apperr.CodeValidationFailed, e.Code)
	return e.Fields
}

func TestDocStatusAndHelpers(t *testing.T) {
	for _, s := range []domain.DocStatus{domain.DocDraft, domain.DocPendingApproval, domain.DocApproved, domain.DocRejected,
		domain.DocInProgress, domain.DocAwaitingSettlement, domain.DocPosted, domain.DocCancelled} {
		assert.True(t, s.Valid())
		assert.NotEmpty(t, s.Label())
	}
	assert.False(t, domain.DocStatus("x").Valid())
	assert.NoError(t, domain.CheckVersion(2, 2))
	assert.True(t, apperr.HasCode(domain.CheckVersion(2, 1), apperr.CodeVersionConflict))
	s := domain.NewStep(actor, time.Unix(0, 0))
	assert.Equal(t, actor.ID, s.By)
	assert.True(t, domain.SumDecimal(d("1"), d("2.5")).Equal(d("3.5")))
	assert.Equal(t, "Marketing", domain.PurposeMarketing.Label())
	assert.False(t, domain.UsagePurpose("x").Valid())
	assert.Equal(t, "A-12", domain.PurposeEmployeeSample.PostingRule())
	assert.Equal(t, "A-11", domain.PurposeSalesSampling.PostingRule())
	assert.False(t, domain.PurposeEmployeeSample.RequiresCampaign())
	assert.Equal(t, "A-26", domain.DisposalReturn.PostingRule())
	assert.Equal(t, "A-27", domain.DisposalDamaged.PostingRule())
	assert.Equal(t, "Rusak", domain.DisposalDamaged.Label())
	assert.False(t, domain.DisposalReason("x").Valid())
	assert.Equal(t, "Pemusnahan", domain.DispositionDispose.Label())
	assert.False(t, domain.Disposition("x").Valid())
	assert.True(t, domain.OutputUnitCost(d("10500"), d("10")).Equal(d("1050")))
	assert.True(t, domain.OutputUnitCost(d("1"), decimal.Zero).IsZero())
}

func usageInput() domain.InternalUsageInput {
	camp := uuid.New()
	return domain.InternalUsageInput{
		Purpose: domain.PurposeMarketing, DivisionID: uuid.New(), CampaignID: &camp, SourceLocationID: uuid.New(),
		BusinessDate: day, Lines: []domain.UsageLineInput{{ProductID: uuid.New(), Qty: d("2")}},
	}
}

func TestNewInternalUsageValidation(t *testing.T) {
	u, err := domain.NewInternalUsage(usageInput(), actor)
	require.NoError(t, err)
	assert.Equal(t, domain.DocDraft, u.Status)
	assert.Nil(t, u.SettleDueDate)

	cases := map[string]func(in *domain.InternalUsageInput){
		"purpose":          func(in *domain.InternalUsageInput) { in.Purpose = "x" },
		"divisionId":       func(in *domain.InternalUsageInput) { in.DivisionID = uuid.Nil },
		"campaignId":       func(in *domain.InternalUsageInput) { in.CampaignID = nil },
		"sourceLocationId": func(in *domain.InternalUsageInput) { in.SourceLocationID = uuid.Nil },
		"businessDate":     func(in *domain.InternalUsageInput) { in.BusinessDate = clock.BusinessDate{} },
		"lines":            func(in *domain.InternalUsageInput) { in.Lines = nil },
		"lines[0].qty":     func(in *domain.InternalUsageInput) { in.Lines[0].Qty = d("0") },
		"lines[0].productId": func(in *domain.InternalUsageInput) {
			in.Lines[0].ProductID = uuid.Nil
		},
		"lines[1].productId": func(in *domain.InternalUsageInput) { in.Lines = append(in.Lines, in.Lines[0]) },
		"note":               func(in *domain.InternalUsageInput) { in.Note = strings.Repeat("x", 501) },
		"eventLocationId":    func(in *domain.InternalUsageInput) { in.Purpose = domain.PurposeEvent },
		"eventEndDate": func(in *domain.InternalUsageInput) {
			in.Purpose = domain.PurposeEvent
			loc := uuid.New()
			before := day.AddDays(-1)
			in.EventLocationID, in.EventEndDate = &loc, &before
		},
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			in := usageInput()
			mutate(&in)
			_, err := domain.NewInternalUsage(in, actor)
			assert.Contains(t, fieldsOf(t, err), field)
		})
	}

	in := usageInput()
	in.Purpose = domain.PurposeEvent
	loc, end := uuid.New(), day.AddDays(2)
	in.EventLocationID, in.EventEndDate = &loc, &end
	ev, err := domain.NewInternalUsage(in, actor)
	require.NoError(t, err)
	assert.True(t, ev.IsEvent())
	assert.Equal(t, day.AddDays(3), *ev.SettleDueDate)

	in = usageInput()
	in.Purpose = domain.PurposeEmployeeSample
	in.CampaignID = nil
	_, err = domain.NewInternalUsage(in, actor)
	assert.NoError(t, err)
}

func TestInternalUsageTransitions(t *testing.T) {
	u, err := domain.NewInternalUsage(usageInput(), actor)
	require.NoError(t, err)
	assert.Equal(t, []domain.DocAction{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, u.AllowedActions(all))
	assert.Empty(t, u.AllowedActions(none))
	require.NoError(t, u.Update(usageInput()))
	steps := []struct {
		a  domain.DocAction
		to domain.DocStatus
	}{
		{domain.ActionSubmit, domain.DocPendingApproval},
		{domain.ActionReject, domain.DocRejected},
		{domain.ActionRevise, domain.DocDraft},
		{domain.ActionSubmit, domain.DocPendingApproval},
		{domain.ActionApprove, domain.DocApproved},
		{domain.ActionRelease, domain.DocInProgress},
		{domain.ActionHandover, domain.DocPosted},
	}
	for _, s := range steps {
		require.NoError(t, u.Transition(s.a), s.a)
		assert.Equal(t, s.to, u.Status)
	}
	assert.True(t, apperr.HasCode(u.Transition(domain.ActionCancel), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(u.Update(usageInput()), apperr.CodeInvalidStateTransition))

	in := usageInput()
	in.Purpose = domain.PurposeEvent
	loc, end := uuid.New(), day
	in.EventLocationID, in.EventEndDate = &loc, &end
	ev, err := domain.NewInternalUsage(in, actor)
	require.NoError(t, err)
	ev.Status = domain.DocInProgress
	assert.Contains(t, ev.AllowedActions(all), domain.ActionHandover)
	require.NoError(t, ev.Transition(domain.ActionHandover))
	assert.Equal(t, domain.DocAwaitingSettlement, ev.Status)
	assert.Contains(t, ev.AllowedActions(all), domain.ActionSettle)
	assert.True(t, apperr.HasCode(ev.Transition(domain.ActionExecute), apperr.CodeInvalidStateTransition))
	require.NoError(t, ev.Transition(domain.ActionSettle))
	ev.Settled = domain.NewStep(actor, time.Now())
	assert.Contains(t, ev.AllowedActions(all), domain.ActionExecute)
	assert.True(t, apperr.HasCode(ev.Transition(domain.ActionSettle), apperr.CodeInvalidStateTransition))
	require.NoError(t, ev.Transition(domain.ActionExecute))
	assert.Equal(t, domain.DocPosted, ev.Status)
}

func TestApplySettlementAndDistribute(t *testing.T) {
	u, err := domain.NewInternalUsage(usageInput(), actor)
	require.NoError(t, err)
	u.Lines[0].ID = uuid.New()
	u.Lines[0].Qty = d("10")
	require.NoError(t, u.ApplySettlement([]domain.SettleLine{{LineID: u.Lines[0].ID, ReturnedQty: d("3")}}))
	assert.True(t, u.Lines[0].ConsumedQty.Equal(d("7")))
	assert.True(t, u.Lines[0].Qty.Equal(u.Lines[0].ConsumedQty.Add(u.Lines[0].ReturnedQty)))
	require.NoError(t, u.ApplySettlement(nil))
	assert.True(t, u.Lines[0].ConsumedQty.Equal(d("10")))

	f := fieldsOf(t, u.ApplySettlement([]domain.SettleLine{{LineID: u.Lines[0].ID, ReturnedQty: d("11")}}))
	assert.Contains(t, f, "lines[0].returnedQty")
	f = fieldsOf(t, u.ApplySettlement([]domain.SettleLine{{LineID: uuid.New(), ReturnedQty: d("-1")}}))
	assert.Contains(t, f, "lines[0].lineId")
	f = fieldsOf(t, u.ApplySettlement([]domain.SettleLine{
		{LineID: u.Lines[0].ID, ReturnedQty: d("1")}, {LineID: u.Lines[0].ID, ReturnedQty: d("1")},
	}))
	assert.Contains(t, f, "lines[1].lineId")

	b1, b2 := uuid.New(), uuid.New()
	sent := []domain.Candidate{{BatchID: &b1, Qty: d("6")}, {BatchID: &b2, Qty: d("4")}}
	al := domain.DistributeReturn(sent, d("5"))
	require.Len(t, al, 2)
	assert.Equal(t, b2, *al[0].Candidate.BatchID)
	assert.True(t, al[0].Qty.Equal(d("4")))
	assert.True(t, al[1].Qty.Equal(d("1")))
	assert.Empty(t, domain.DistributeReturn(sent, decimal.Zero))
}

func repackInput() domain.RepackingInput {
	exp := day.AddDays(100)
	return domain.RepackingInput{BusinessDate: day, Lines: []domain.RepackLineInput{
		{Direction: domain.RepackInput, ProductID: uuid.New(), LocationID: uuid.New(), Qty: d("5")},
		{Direction: domain.RepackOutput, ProductID: uuid.New(), LocationID: uuid.New(), BatchNumber: " rp1 ", ExpiryDate: &exp, Qty: d("5")},
	}}
}

func TestRepacking(t *testing.T) {
	r, err := domain.NewRepacking(repackInput(), actor)
	require.NoError(t, err)
	assert.Equal(t, "RP1", r.Output().BatchNumber)
	assert.Len(t, r.Inputs(), 1)
	assert.Equal(t, []domain.DocAction{domain.ActionEdit, domain.ActionStart, domain.ActionCancel}, r.AllowedActions(all))
	require.NoError(t, r.Update(repackInput()))

	cases := map[string]func(in *domain.RepackingInput){
		"businessDate":         func(in *domain.RepackingInput) { in.BusinessDate = clock.BusinessDate{} },
		"lines[0].productId":   func(in *domain.RepackingInput) { in.Lines[0].ProductID = uuid.Nil },
		"lines[0].locationId":  func(in *domain.RepackingInput) { in.Lines[0].LocationID = uuid.Nil },
		"lines[0].stockStatus": func(in *domain.RepackingInput) { in.Lines[0].StockStatus = domain.StatusReserved },
		"lines[0].direction":   func(in *domain.RepackingInput) { in.Lines[0].Direction = "x" },
		"lines[1].batchNumber": func(in *domain.RepackingInput) { in.Lines[1].BatchNumber = "" },
		"lines[1].stockStatus": func(in *domain.RepackingInput) { in.Lines[1].StockStatus = domain.StatusQuarantine },
		"lines":                func(in *domain.RepackingInput) { in.Lines = in.Lines[:1] },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			in := repackInput()
			mutate(&in)
			_, err := domain.NewRepacking(in, actor)
			assert.Contains(t, fieldsOf(t, err), field)
		})
	}
	in := repackInput()
	in.Lines = in.Lines[1:]
	_, err = domain.NewRepacking(in, actor)
	assert.Contains(t, fieldsOf(t, err), "lines")

	require.NoError(t, r.Transition(domain.ActionStart))
	assert.Equal(t, []domain.DocAction{domain.ActionExecute, domain.ActionCancel}, r.AllowedActions(all))
	assert.True(t, apperr.HasCode(r.Update(repackInput()), apperr.CodeInvalidStateTransition))
	require.NoError(t, r.Transition(domain.ActionExecute))
	assert.True(t, apperr.HasCode(r.Transition(domain.ActionCancel), apperr.CodeInvalidStateTransition))
	none := domain.Repacking{}
	assert.Nil(t, none.Output())
}

func disposalInput() domain.DisposalInput {
	return domain.DisposalInput{
		Reason: domain.DisposalExpired, DisposalReport: domain.DisposalReport{DisposalDate: day},
		Lines: []domain.DisposalLineInput{{ProductID: uuid.New(), LocationID: uuid.New(), Qty: d("1")}},
	}
}

func TestDisposal(t *testing.T) {
	ds, err := domain.NewDisposal(disposalInput(), actor)
	require.NoError(t, err)
	assert.Equal(t, []string{}, ds.Witnesses)
	assert.Contains(t, ds.AllowedActions(all), domain.ActionReport)
	require.NoError(t, ds.Update(disposalInput()))

	cases := map[string]func(in *domain.DisposalInput){
		"reason":               func(in *domain.DisposalInput) { in.Reason = "x" },
		"disposalDate":         func(in *domain.DisposalInput) { in.DisposalDate = clock.BusinessDate{} },
		"method":               func(in *domain.DisposalInput) { in.Method = strings.Repeat("x", 501) },
		"witnesses":            func(in *domain.DisposalInput) { in.Witnesses = make([]string, 11); fill(in.Witnesses) },
		"lines[0].witness":     func(in *domain.DisposalInput) { in.Witnesses = []string{strings.Repeat("x", 121)} },
		"photoFileIds":         func(in *domain.DisposalInput) { in.PhotoFileIDs = make([]uuid.UUID, 11) },
		"lines[0].productId":   func(in *domain.DisposalInput) { in.Lines[0].ProductID = uuid.Nil },
		"lines[0].locationId":  func(in *domain.DisposalInput) { in.Lines[0].LocationID = uuid.Nil },
		"lines[0].stockStatus": func(in *domain.DisposalInput) { in.Lines[0].StockStatus = domain.StatusInTransit },
		"lines[0].qty":         func(in *domain.DisposalInput) { in.Lines[0].Qty = d("-1") },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			in := disposalInput()
			mutate(&in)
			_, err := domain.NewDisposal(in, actor)
			assert.Contains(t, fieldsOf(t, err), field)
		})
	}

	require.NoError(t, ds.Transition(domain.ActionSubmit))
	require.NoError(t, ds.Transition(domain.ActionApprove))
	f := fieldsOf(t, ds.Transition(domain.ActionExecute))
	assert.Contains(t, f, "method")
	assert.Contains(t, f, "witnesses")
	assert.Contains(t, f, "photoFileIds")
	require.Error(t, ds.UpdateReport(domain.DisposalReport{}))
	require.NoError(t, ds.UpdateReport(domain.DisposalReport{
		DisposalDate: day, Method: " Dibakar ", Witnesses: []string{" Budi ", ""}, PhotoFileIDs: []uuid.UUID{uuid.New()},
	}))
	assert.Equal(t, "Dibakar", ds.Method)
	assert.Equal(t, []string{"Budi"}, ds.Witnesses)
	require.NoError(t, ds.Transition(domain.ActionExecute))
	assert.Equal(t, domain.DocPosted, ds.Status)
	assert.True(t, apperr.HasCode(ds.UpdateReport(domain.DisposalReport{DisposalDate: day}), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(ds.Update(disposalInput()), apperr.CodeInvalidStateTransition))

	fromDisposition := domain.Disposal{Status: domain.DocApproved, ReturnDispositionID: &actor.ID}
	assert.NotContains(t, fromDisposition.AllowedActions(all), domain.ActionCancel)
	assert.Contains(t, fromDisposition.AllowedActions(all), domain.ActionExecute)
}

func fill(s []string) {
	for i := range s {
		s[i] = "Saksi"
	}
}

func dispositionInput() domain.ReturnDispositionInput {
	return domain.ReturnDispositionInput{LocationID: uuid.New(), BusinessDate: day, Lines: []domain.DispositionLineInput{
		{ProductID: uuid.New(), Qty: d("2"), Disposition: domain.DispositionRestock},
		{ProductID: uuid.New(), Qty: d("3"), Disposition: domain.DispositionDispose},
	}}
}

func TestReturnDisposition(t *testing.T) {
	r, err := domain.NewReturnDisposition(dispositionInput(), actor)
	require.NoError(t, err)
	assert.True(t, r.HasDisposal())
	r.Lines[0].EstimatedCost, r.Lines[1].EstimatedCost = d("100"), d("300")
	assert.True(t, r.ComputeDisposalValue().Equal(d("300")))
	assert.Equal(t, []domain.DocAction{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, r.AllowedActions(all))
	require.NoError(t, r.Update(dispositionInput()))

	cases := map[string]func(in *domain.ReturnDispositionInput){
		"locationId":           func(in *domain.ReturnDispositionInput) { in.LocationID = uuid.Nil },
		"businessDate":         func(in *domain.ReturnDispositionInput) { in.BusinessDate = clock.BusinessDate{} },
		"lines[0].productId":   func(in *domain.ReturnDispositionInput) { in.Lines[0].ProductID = uuid.Nil },
		"lines[0].stockStatus": func(in *domain.ReturnDispositionInput) { in.Lines[0].StockStatus = domain.StatusReserved },
		"lines[0].disposition": func(in *domain.ReturnDispositionInput) { in.Lines[0].Disposition = "" },
		"lines[0].qty":         func(in *domain.ReturnDispositionInput) { in.Lines[0].Qty = d("0") },
		"lines[0].qcNote":      func(in *domain.ReturnDispositionInput) { in.Lines[0].QCNote = strings.Repeat("x", 501) },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			in := dispositionInput()
			mutate(&in)
			_, err := domain.NewReturnDisposition(in, actor)
			assert.Contains(t, fieldsOf(t, err), field)
		})
	}
	for _, a := range []domain.DocAction{domain.ActionSubmit, domain.ActionReject, domain.ActionRevise, domain.ActionSubmit, domain.ActionApprove} {
		require.NoError(t, r.Transition(a), a)
	}
	assert.Equal(t, []domain.DocAction{domain.ActionExecute}, r.AllowedActions(all))
	require.NoError(t, r.Transition(domain.ActionExecute))
	assert.True(t, apperr.HasCode(r.Transition(domain.ActionExecute), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(r.Update(dispositionInput()), apperr.CodeInvalidStateTransition))
	amounts := domain.DispositionAmounts{Restock: d("1"), Reprocess: d("2")}
	assert.True(t, amounts.Returned().Equal(d("3")))
}

func TestReplay(t *testing.T) {
	l1, l0, l2 := uuid.New(), uuid.New(), uuid.New()
	layers := []domain.ReplayLayer{
		{ID: l0, BusinessDate: day.AddDays(-12), ReceivedQty: d("10"), UnitCost: d("80")},
		{ID: l1, BusinessDate: day.AddDays(-10), ReceivedQty: d("10"), UnitCost: d("100")},
		{ID: l2, BusinessDate: day.AddDays(5), ReceivedQty: d("10"), UnitCost: d("200")},
	}
	early, i1, i2 := uuid.New(), uuid.New(), uuid.New()
	issues := []domain.ReplayIssue{
		// Issue sebelum layer mundur: hanya layer ≤ tanggalnya tidak ada → ambil layer sesudahnya.
		{MovementID: early, BusinessDate: day.AddDays(-20), Qty: d("1"), CurrentCost: d("80")},
		{MovementID: i1, BusinessDate: day.AddDays(-5), Qty: d("12"), CurrentCost: d("1200")},
		{MovementID: i2, BusinessDate: day, Qty: d("5"), CurrentCost: d("500")},
	}
	res := domain.Replay(layers, issues)
	require.False(t, res.Short)
	assert.Equal(t, "80.00", res.Costs[early].StringFixed(2))
	assert.Equal(t, "1020.00", res.Costs[i1].StringFixed(2), "9×80 + 3×100")
	assert.Equal(t, "500.00", res.Costs[i2].StringFixed(2))
	assert.True(t, res.Remaining[l0].IsZero())
	assert.True(t, res.Remaining[l1].Equal(d("2")))
	assert.True(t, res.Remaining[l2].Equal(d("10")))
	diffs := res.Diffs(issues)
	require.Len(t, diffs, 1)
	assert.Equal(t, i1, diffs[0].MovementID)
	assert.Equal(t, "-180.00", diffs[0].Difference().StringFixed(2))

	short := domain.Replay(layers[:1], []domain.ReplayIssue{{MovementID: i1, BusinessDate: day, Qty: d("11")}})
	assert.True(t, short.Short)
}
