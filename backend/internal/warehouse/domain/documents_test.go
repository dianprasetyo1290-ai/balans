package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/warehouse/domain"
)

var (
	day = clock.NewBusinessDate(2026, 10, 26)
	now = time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal { v := d(s); return &v }

func isTransition(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, apperr.CodeInvalidStateTransition, apperr.As(err).Code)
}

func TestNewInbound(t *testing.T) {
	dest := uuid.New()
	ok := domain.InboundInput{
		SourceType: domain.SourceGoodsReceipt, SourceID: uuid.New(), LocationID: uuid.New(), BusinessDate: day,
		Lines: []domain.InboundLineInput{{ProductID: uuid.New(), Qty: d("10"), DestinationLocationID: &dest}},
	}
	ib, err := domain.NewInbound(ok)
	require.NoError(t, err)
	assert.Equal(t, domain.RejectHold, ib.RejectAction)
	assert.Equal(t, domain.StatusInProgress, ib.Status)
	assert.Equal(t, domain.LineQuarantine, ib.Lines[0].Status)

	ok.SourceType = domain.SourceSalesReturn
	ib, err = domain.NewInbound(ok)
	require.NoError(t, err)
	assert.Equal(t, domain.RejectToReturns, ib.RejectAction)

	_, err = domain.NewInbound(domain.InboundInput{SourceType: "Bad Type", RejectAction: "x"})
	f := fields(t, err)
	for _, k := range []string{"source.type", "source.id", "locationId", "businessDate", "rejectAction", "lines"} {
		assert.Contains(t, f, k)
	}
	_, err = domain.NewInbound(domain.InboundInput{
		SourceType: "x", SourceID: uuid.New(), LocationID: uuid.New(), BusinessDate: day, SourceLabel: string(make([]rune, 121)),
		Lines: []domain.InboundLineInput{{Qty: d("1.0001")}},
	})
	f = fields(t, err)
	assert.Contains(t, f, "lines[0].productId")
	assert.Contains(t, f, "lines[0].qty")
	assert.Contains(t, f, "lines[0].destinationLocationId")
	assert.Contains(t, f, "source.label")
	_, err = domain.NewInbound(domain.InboundInput{Lines: make([]domain.InboundLineInput, domain.MaxDocumentLines+1)})
	assert.Contains(t, fields(t, err), "lines")

	assert.Equal(t, "Penerimaan Barang", domain.SourceTypeLabel(domain.SourceGoodsReceipt))
	assert.Equal(t, "lainnya", domain.SourceTypeLabel("lainnya"))
	assert.Equal(t, domain.LocationRawMaterial, domain.DefaultDestinationType("packaging"))
	assert.Equal(t, domain.LocationGoodsMaterial, domain.DefaultDestinationType("reprocess_material"))
	assert.Equal(t, domain.LocationFinishedGoods, domain.DefaultDestinationType("finished_good"))
}

func TestInboundQC(t *testing.T) {
	ib := domain.Inbound{ID: uuid.New(), Status: domain.StatusInProgress, Lines: []domain.InboundLine{
		{ID: uuid.New(), LineNo: 1, Qty: d("10"), Status: domain.LineQuarantine},
		{ID: uuid.New(), LineNo: 2, Qty: d("5"), Status: domain.LineQuarantine},
	}}
	all := domain.Checklist{Batch: true, Expiry: true, Condition: true, Seal: true}
	l, ok := ib.Line(ib.Lines[0].ID)
	require.True(t, ok)
	_, missing := ib.Line(uuid.New())
	assert.False(t, missing)

	cases := []struct {
		name  string
		in    domain.QCInput
		field string
	}{
		{"jumlah tidak sama", domain.QCInput{PassedQty: d("5"), RejectedQty: d("1"), Checks: all, RejectReason: "x"}, "passedQty"},
		{"qty negatif", domain.QCInput{PassedQty: d("-1"), RejectedQty: d("11"), Checks: all}, "passedQty"},
		{"reject tanpa alasan", domain.QCInput{PassedQty: d("5"), RejectedQty: d("5"), Checks: all}, "rejectReason"},
		{"lulus penuh checklist gagal", domain.QCInput{PassedQty: d("10"), RejectedQty: d("0"), Checks: domain.Checklist{Batch: true}}, "checks"},
		{"terlalu banyak foto", domain.QCInput{PassedQty: d("10"), RejectedQty: d("0"), Checks: all, PhotoFileIDs: make([]uuid.UUID, 6)}, "photoFileIds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			probe := *l
			_, err := probe.ApplyQC(c.in, uuid.New(), "QC", now)
			assert.Contains(t, fields(t, err), c.field)
		})
	}

	dest, by := uuid.New(), uuid.New()
	in, err := l.ApplyQC(domain.QCInput{
		PassedQty: d("7"), RejectedQty: d("3"), Checks: domain.Checklist{Batch: true}, RejectReason: " penyok ",
		DestinationLocationID: &dest, Note: "cek",
	}, by, "Staf", now)
	require.NoError(t, err)
	assert.Equal(t, domain.LinePosted, l.Status)
	assert.Equal(t, dest, l.DestinationLocationID)
	assert.Equal(t, "penyok", in.RejectReason)
	checks := domain.QualityChecks(ib, *l, in)
	require.Len(t, checks, 2)
	assert.Equal(t, domain.QCPassed, checks[0].Result)
	assert.Equal(t, "penyok — cek", checks[1].Note)
	_, err = l.ApplyQC(in, by, "Staf", now)
	isTransition(t, err)
	assert.False(t, ib.Complete(now))

	l2 := &ib.Lines[1]
	nilDest := uuid.Nil
	_, err = l2.ApplyQC(domain.QCInput{PassedQty: d("0"), RejectedQty: d("5"), Checks: all, RejectReason: "rusak", DestinationLocationID: &nilDest}, by, "Staf", now)
	require.NoError(t, err)
	assert.Equal(t, domain.LineQCRejected, l2.Status)
	assert.Len(t, domain.QualityChecks(ib, *l2, domain.QCInput{Checks: all}), 1)
	assert.True(t, ib.AllChecked())
	assert.True(t, ib.Complete(now))
	assert.Equal(t, domain.StatusCompleted, ib.Status)
	assert.True(t, all.AllPassed())
}

func transferInput(t domain.TransferType) domain.TransferInput {
	return domain.TransferInput{
		Type: t, FromLocationID: uuid.New(), ToLocationID: uuid.New(), BusinessDate: day,
		Lines: []domain.TransferLineInput{{ProductID: uuid.New(), Qty: d("10")}, {ProductID: uuid.New(), Qty: d("4")}},
	}
}

func withIDs(tr *domain.StockTransfer) {
	for i := range tr.Lines {
		tr.Lines[i].ID = uuid.New()
	}
}

func TestTransferValidation(t *testing.T) {
	same := uuid.New()
	_, err := domain.NewTransfer(domain.TransferInput{Type: "x", FromLocationID: same, ToLocationID: same, SourceType: "do"}, uuid.New(), "A")
	f := fields(t, err)
	for _, k := range []string{"transferType", "toLocationId", "businessDate", "source", "lines"} {
		assert.Contains(t, f, k)
	}
	_, err = domain.NewTransfer(domain.TransferInput{Type: domain.TransferDirect, Lines: []domain.TransferLineInput{{Qty: d("0")}}}, uuid.New(), "A")
	f = fields(t, err)
	assert.Contains(t, f, "fromLocationId")
	assert.Contains(t, f, "lines[0].productId")
	assert.Contains(t, f, "lines[0].qty")
	_, err = domain.NewTransfer(domain.TransferInput{Lines: make([]domain.TransferLineInput, domain.MaxDocumentLines+1)}, uuid.New(), "A")
	assert.Contains(t, fields(t, err), "lines")
	assert.True(t, domain.TransferRequest.Valid())
}

func TestDirectTransferInternal(t *testing.T) {
	tr, err := domain.NewTransfer(transferInput(domain.TransferDirect), uuid.New(), "Staf")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, tr.Status)
	assert.True(t, tr.Can(domain.ActionPost))
	assert.False(t, tr.Can(domain.ActionShip))
	assert.False(t, tr.Can(domain.ActionSubmit))
	in := transferInput(domain.TransferDirect)
	in.Note = "ubah"
	require.NoError(t, tr.UpdateDraft(in))
	assert.Equal(t, "ubah", tr.Note)
	require.NoError(t, tr.Transition(domain.ActionPost))
	assert.Equal(t, domain.StatusCompleted, tr.Status)
	isTransition(t, tr.UpdateDraft(in))
	isTransition(t, tr.Cancel("", now))
}

func TestDirectTransferShipping(t *testing.T) {
	tr, err := domain.NewTransfer(transferInput(domain.TransferDirect), uuid.New(), "Staf")
	require.NoError(t, err)
	tr.RequiresShipping = true
	withIDs(&tr)
	isTransition(t, tr.Transition(domain.ActionPost))
	_, err = tr.ApplyReceive(nil)
	isTransition(t, err)
	require.NoError(t, tr.Transition(domain.ActionShip))
	assert.Equal(t, domain.StatusInTransit, tr.Status)

	_, err = tr.ApplyReceive([]domain.ReceiveLine{{LineID: uuid.New(), Qty: d("1")}, {LineID: tr.Lines[0].ID, Qty: d("11")}, {LineID: tr.Lines[1].ID, Qty: d("-1")}})
	f := fields(t, err)
	assert.Len(t, f, 3)
	_, err = tr.ApplyReceive([]domain.ReceiveLine{{LineID: tr.Lines[0].ID, Qty: d("0")}})
	assert.Contains(t, fields(t, err), "lines")

	got, err := tr.ApplyReceive([]domain.ReceiveLine{{LineID: tr.Lines[0].ID, Qty: d("6")}})
	require.NoError(t, err)
	assert.Equal(t, "6", got[tr.Lines[0].ID].String())
	assert.Equal(t, domain.StatusPartiallyReceived, tr.Status)
	short, err := tr.CloseShortage()
	require.NoError(t, err)
	assert.Equal(t, "4", short[tr.Lines[0].ID].String())
	assert.Equal(t, "4", short[tr.Lines[1].ID].String())
	assert.Equal(t, domain.StatusCompleted, tr.Status)
	assert.True(t, tr.Lines[0].Outstanding().IsZero())

	tr2, _ := domain.NewTransfer(transferInput(domain.TransferDirect), uuid.New(), "Staf")
	tr2.RequiresShipping = true
	withIDs(&tr2)
	require.NoError(t, tr2.Transition(domain.ActionShip))
	_, err = tr2.ApplyReceive([]domain.ReceiveLine{{LineID: tr2.Lines[0].ID, Qty: d("10")}, {LineID: tr2.Lines[1].ID, Qty: d("4")}})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusReceived, tr2.Status)
	require.NoError(t, tr2.Transition(domain.ActionComplete))

	tr3, _ := domain.NewTransfer(transferInput(domain.TransferDirect), uuid.New(), "Staf")
	isTransition(t, tr3.Transition(domain.ActionShip))
	require.Error(t, tr3.Cancel(string(make([]rune, 600)), now))
	require.NoError(t, tr3.Cancel("salah input", now))
	assert.Equal(t, domain.StatusCancelled, tr3.Status)
}

func TestPettyStockRequestFlow(t *testing.T) {
	tr, err := domain.NewTransfer(transferInput(domain.TransferRequest), uuid.New(), "Fulfillment")
	require.NoError(t, err)
	withIDs(&tr)
	assert.False(t, tr.Can(domain.ActionPost))
	_, err = tr.ConfirmRequest(nil)
	isTransition(t, err)
	require.NoError(t, tr.Transition(domain.ActionSubmit))
	require.NoError(t, tr.Transition(domain.ActionRelease))
	assert.Equal(t, domain.StatusInProgress, tr.Status)
	_, err = tr.ConfirmRequest(nil)
	require.Error(t, err, "belum disiapkan")
	at := now
	tr.PreparedAt = &at
	assert.False(t, tr.Can(domain.ActionPrepare))
	probe := tr
	_, err = probe.ConfirmRequest([]domain.ReceiveLine{{LineID: uuid.New(), Qty: d("1")}, {LineID: tr.Lines[0].ID, Qty: d("11")}, {LineID: tr.Lines[1].ID, Qty: d("1.0001")}})
	assert.Len(t, fields(t, err), 3)
	tr.Status = domain.StatusInProgress
	got, err := tr.ConfirmRequest([]domain.ReceiveLine{{LineID: tr.Lines[0].ID, Qty: d("8")}})
	require.NoError(t, err)
	assert.Equal(t, "8", got[tr.Lines[0].ID].String())
	assert.Equal(t, domain.StatusCompleted, tr.Status)
	assert.Equal(t, "2", tr.Lines[0].ShortageQty.String())
	assert.Equal(t, "4", tr.Lines[1].ShortageQty.String())

	dt, _ := domain.NewTransfer(transferInput(domain.TransferDirect), uuid.New(), "A")
	_, err = dt.ConfirmRequest(nil)
	isTransition(t, err)
}

func TestStockCountFlow(t *testing.T) {
	_, err := domain.NewStockCount(domain.StockCountInput{CountType: "x"})
	f := fields(t, err)
	assert.Contains(t, f, "countType")
	assert.Contains(t, f, "locationIds")
	assert.Contains(t, f, "businessDate")

	c, err := domain.NewStockCount(domain.StockCountInput{CountType: domain.CountCycle, LocationID: uuid.New(), BusinessDate: day})
	require.NoError(t, err)
	assert.True(t, c.Can(domain.ActionStart))
	_, err = c.RecordCounts(nil, nil, uuid.New(), now)
	isTransition(t, err)
	isTransition(t, c.SetPhotos(nil))
	isTransition(t, c.Submit(uuid.New(), "A", now))
	p1, p2, b1 := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, c.Start([]domain.SnapshotRow{
		{ProductID: p1, BatchID: &b1, Qty: d("10"), UnitCost: d("1000")},
		{ProductID: p2, Qty: d("5"), UnitCost: d("200")},
	}, uuid.New(), "Staf", now))
	assert.Equal(t, domain.StatusInProgress, c.Status)
	assert.False(t, c.RevealsSystemQty())
	for i := range c.Lines {
		c.Lines[i].ID = uuid.New()
	}
	err = c.Submit(uuid.New(), "A", now)
	assert.Contains(t, fields(t, err), "lines[0].countedQty")

	note, photo, zero := "rak A", uuid.New(), uuid.Nil
	_, err = c.RecordCounts([]domain.CountEntry{{LineID: uuid.New()}, {LineID: c.Lines[0].ID, CountedQty: dp("-1")}}, []domain.ExtraEntry{
		{CountedQty: d("1")}, {ProductID: p1, CountedQty: d("-2")},
	}, uuid.New(), now)
	assert.Len(t, fields(t, err), 4)
	changed, err := c.RecordCounts([]domain.CountEntry{
		{LineID: c.Lines[0].ID, CountedQty: dp("8"), Note: &note, PhotoFileID: &photo},
		{LineID: c.Lines[1].ID, CountedQty: dp("5"), PhotoFileID: &zero},
	}, []domain.ExtraEntry{
		{ProductID: p2, CountedQty: d("6")},
		{ProductID: uuid.New(), CountedQty: d("3"), UnitCost: d("50"), Note: "temuan"},
	}, uuid.New(), now)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, changed)
	assert.Equal(t, photo, *c.Lines[0].PhotoFileID)
	assert.Nil(t, c.Lines[1].PhotoFileID)
	assert.True(t, c.Lines[2].IsExtra)
	assert.Error(t, c.SetPhotos(make([]uuid.UUID, 11)))
	require.NoError(t, c.SetPhotos([]uuid.UUID{uuid.New()}))

	require.NoError(t, c.Submit(uuid.New(), "Staf", now))
	assert.Equal(t, domain.StatusPendingApproval, c.Status)
	assert.True(t, c.HasVariance())
	// −2×1000, +1×200, +3×50 → absolut 2350, bersih −1650.
	assert.Equal(t, "2350", c.VarianceAmount.String())
	assert.Equal(t, "-1650", c.NetVariance.String())
	assert.True(t, c.RevealsSystemQty())
	assert.Len(t, c.Adjustments(), 3)

	rej := c
	require.NoError(t, rej.Decide(false, "ulang", now))
	assert.Equal(t, domain.StatusRejected, rej.Status)
	require.NoError(t, rej.Recount())
	assert.Equal(t, domain.StatusInProgress, rej.Status)
	assert.True(t, rej.VarianceAmount.IsZero())
	isTransition(t, rej.Recount())

	require.NoError(t, c.Decide(true, "", now))
	assert.Equal(t, domain.StatusPosted, c.Status)
	assert.NotNil(t, c.PostedAt)
	isTransition(t, c.Decide(true, "", now))
}

func TestStockCountNoVarianceAndConsignment(t *testing.T) {
	c, _ := domain.NewStockCount(domain.StockCountInput{CountType: domain.CountFull, LocationID: uuid.New(), BusinessDate: day})
	require.NoError(t, c.Start([]domain.SnapshotRow{{ProductID: uuid.New(), Qty: d("3")}}, uuid.New(), "A", now))
	c.Lines[0].ID = uuid.New()
	_, err := c.RecordCounts([]domain.CountEntry{{LineID: c.Lines[0].ID, CountedQty: dp("3")}}, nil, uuid.New(), now)
	require.NoError(t, err)
	require.NoError(t, c.Submit(uuid.New(), "A", now))
	assert.Equal(t, domain.StatusCompleted, c.Status)
	assert.False(t, c.HasVariance())

	k, _ := domain.NewStockCount(domain.StockCountInput{CountType: domain.CountConsignment, LocationID: uuid.New(), BusinessDate: day})
	require.NoError(t, k.Start(nil, uuid.New(), "M", now))
	err = k.Submit(uuid.New(), "M", now)
	f := fields(t, err)
	assert.Contains(t, f, "lines")
	assert.Contains(t, f, "photoFileIds")
	_, err = k.RecordCounts(nil, []domain.ExtraEntry{{ProductID: uuid.New(), CountedQty: d("2")}}, uuid.New(), now)
	require.NoError(t, err)
	require.NoError(t, k.SetPhotos([]uuid.UUID{uuid.New()}))
	require.NoError(t, k.Submit(uuid.New(), "M", now))
	assert.Equal(t, domain.StatusCompleted, k.Status, "konsinyasi tanpa approval")

	cancel, _ := domain.NewStockCount(domain.StockCountInput{CountType: domain.CountVending, LocationID: uuid.New(), BusinessDate: day})
	require.NoError(t, cancel.Transition(domain.ActionCancel))
	assert.Contains(t, domain.CountVending.LocationTypes(), domain.LocationVendingMachine)
	assert.Contains(t, domain.CountConsignment.LocationTypes(), domain.LocationConsignment)
	assert.Contains(t, domain.CountCycle.LocationTypes(), domain.LocationPettyStock)
	assert.False(t, domain.CountVending.Adjusts())
}

func ip(i int) *int { return &i }

func TestScheduleOccurrence(t *testing.T) {
	_, err := domain.NewSchedule(domain.ScheduleInput{CountType: "x", Frequency: "daily"}, day)
	f := fields(t, err)
	assert.Contains(t, f, "name")
	assert.Contains(t, f, "countType")
	assert.Contains(t, f, "frequency")
	for _, in := range []domain.ScheduleInput{
		{Name: "a", CountType: domain.CountCycle, Frequency: domain.FrequencyWeekly},
		{Name: "a", CountType: domain.CountConsignment, Frequency: domain.FrequencyMonthly, DayOfMonth: ip(0)},
		{Name: "a", CountType: domain.CountConsignment, Frequency: domain.FrequencyYearly, DayOfMonth: ip(31)},
		{Name: "a", CountType: domain.CountConsignment, Frequency: domain.FrequencyMonthly, DayOfMonth: ip(1), LocationIDs: make([]uuid.UUID, 101)},
	} {
		_, err := domain.NewSchedule(in, day)
		require.Error(t, err)
	}
	wh := uuid.New()
	// 26 Okt 2026 = Senin.
	w, err := domain.NewSchedule(domain.ScheduleInput{Name: "Mingguan", CountType: domain.CountCycle, Frequency: domain.FrequencyWeekly, DayOfWeek: ip(5), WarehouseID: &wh, IsActive: true}, day)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-30", w.NextRunDate.String())
	w.Advance(w.NextRunDate)
	assert.Equal(t, "2026-11-06", w.NextRunDate.String())
	assert.Equal(t, "2026-10-30", w.LastRunDate.String())

	m, err := domain.NewSchedule(domain.ScheduleInput{Name: "Konsinyasi", CountType: domain.CountConsignment, Frequency: domain.FrequencyMonthly, DayOfMonth: ip(31)}, day)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-31", m.NextRunDate.String())
	m.Advance(m.NextRunDate)
	assert.Equal(t, "2026-11-30", m.NextRunDate.String(), "akhir bulan")
	early, _ := domain.NewSchedule(domain.ScheduleInput{Name: "Awal", CountType: domain.CountVending, Frequency: domain.FrequencyMonthly, DayOfMonth: ip(5)}, day)
	assert.Equal(t, "2026-11-05", early.NextRunDate.String())

	y, err := domain.NewSchedule(domain.ScheduleInput{Name: "Akhir tahun", CountType: domain.CountFull, Frequency: domain.FrequencyYearly, DayOfMonth: ip(31), MonthOfYear: ip(12), WarehouseID: &wh}, day)
	require.NoError(t, err)
	assert.Equal(t, "2026-12-31", y.NextRunDate.String())
	y.Advance(y.NextRunDate)
	assert.Equal(t, "2027-12-31", y.NextRunDate.String())
	require.NoError(t, y.Update(domain.ScheduleInput{Name: "Feb", CountType: domain.CountFull, Frequency: domain.FrequencyYearly, DayOfMonth: ip(30), MonthOfYear: ip(2), WarehouseID: &wh}, day))
	assert.Equal(t, "2027-02-28", y.NextRunDate.String())
	assert.Equal(t, "Feb", y.Name)
}

func TestInventoryAccountAndSplits(t *testing.T) {
	assert.Equal(t, domain.AccountReturns, domain.InventoryAccount("finished_good", domain.LocationReturns))
	assert.Equal(t, domain.AccountPackaging, domain.InventoryAccount("packaging", domain.LocationRawMaterial))
	assert.Equal(t, domain.AccountReprocess, domain.InventoryAccount("reprocess_material", domain.LocationGoodsMaterial))
	assert.Equal(t, domain.AccountFGConsign, domain.InventoryAccount("finished_good", domain.LocationConsignment))
	assert.Equal(t, domain.AccountFGVending, domain.InventoryAccount("finished_good", domain.LocationVendingBuffer))
	assert.Equal(t, domain.AccountFGWarehouse, domain.InventoryAccount("finished_good", domain.LocationFinishedGoods))
	s := domain.ShortageSplits(map[string]decimal.Decimal{"1311": d("100.004"), "1301": d("-5"), "1312": d("0.001")})
	require.Len(t, s, 2)
	assert.Equal(t, "1301", s[0].Account)
	assert.Equal(t, "100", s[1].Amount.String())
	assert.True(t, domain.ValidSourceType("goods_receipt"))
	assert.False(t, domain.ValidSourceType("Goods"))
}
