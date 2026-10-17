package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/sales/domain"
)

func TestDeliveryType(t *testing.T) {
	for _, tc := range []struct {
		t                         domain.DeliveryType
		label, loc, channel, rule string
	}{
		{domain.DeliveryConsignment, "Konsinyasi", "consignment", "consignment", "A-07"},
		{domain.DeliveryVendingBuffer, "Buffer Vending", "vending_buffer", "vending", "A-24"},
	} {
		assert.True(t, tc.t.Valid())
		assert.Equal(t, tc.label, tc.t.Label())
		assert.Equal(t, tc.loc, tc.t.LocationType())
		assert.Equal(t, tc.channel, tc.t.ChannelCode())
		assert.Equal(t, tc.rule, tc.t.RuleCode())
	}
	assert.False(t, domain.DeliveryType("x").Valid())
}

func deliveryInput() domain.DeliveryInput {
	return domain.DeliveryInput{
		Type: domain.DeliveryConsignment, PartnerID: uuid.New(), FromLocationID: location, BusinessDate: today,
		Lines: []domain.DeliveryLineInput{{ProductID: product1, Qty: dec("10")}},
	}
}

func TestNewConsignmentDeliveryValidation(t *testing.T) {
	f := fields(t, func() error { _, err := domain.NewConsignmentDelivery(domain.DeliveryInput{}); return err }())
	for _, k := range []string{"deliveryType", "partnerId", "fromLocationId", "businessDate", "lines"} {
		assert.Contains(t, f, k)
	}
	in := deliveryInput()
	in.Note = strings.Repeat("x", 1001)
	in.Lines = append(in.Lines, domain.DeliveryLineInput{ProductID: product1, Qty: dec("0")}, domain.DeliveryLineInput{Qty: dec("1")})
	f = fields(t, func() error { _, err := domain.NewConsignmentDelivery(in); return err }())
	assert.Contains(t, f, "note")
	assert.Equal(t, "Produk sudah ada di baris lain", f["lines.1.productId"])
	assert.Contains(t, f, "lines.1.qty")
	assert.Equal(t, "Pilih produk", f["lines.2.productId"])
	in = deliveryInput()
	in.Lines = make([]domain.DeliveryLineInput, domain.MaxLines+1)
	assert.Contains(t, fields(t, func() error { _, err := domain.NewConsignmentDelivery(in); return err }()), "lines")
}

func TestConsignmentDeliveryLifecycle(t *testing.T) {
	d, err := domain.NewConsignmentDelivery(deliveryInput())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, d.Status)
	assert.Equal(t, "Draft", d.Label())
	assert.Equal(t, domain.SourceConsignmentDelivery, d.SourceType)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, d.Actions())
	assert.False(t, d.ViaDeliveryOrder())

	upd := deliveryInput()
	upd.Lines[0].Qty = dec("12")
	require.NoError(t, d.Update(upd))
	assert.Equal(t, "12", d.Lines[0].Qty.String())

	// Kirim langsung tanpa DO tidak ada lagi: Draft harus diajukan dulu (reservasi + DO).
	b1, b2 := uuid.New(), uuid.New()
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.MarkShipped("DO/2026/11/00001", uuid.New(), "TRF", nil, now)))
	assert.Contains(t, fields(t, d.Submit(uuid.Nil)), "deliveryOrderId")
	doID := uuid.New()
	require.NoError(t, d.Submit(doID))
	assert.Equal(t, domain.StatusInProgress, d.Status)
	assert.True(t, d.ViaDeliveryOrder())
	assert.Equal(t, domain.SourceDeliveryOrder, d.SourceType)
	assert.Empty(t, d.Actions(), "Diproses: dikerjakan Logistik dari halaman DO")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.Update(upd)))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.Submit(doID)))
	// DO dibatalkan → Draft lagi; diajukan ulang dengan DO baru.
	require.NoError(t, d.Reopen())
	assert.Equal(t, domain.StatusDraft, d.Status)
	assert.Nil(t, d.SourceID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.Reopen()))
	require.NoError(t, d.Submit(doID))
	require.NoError(t, d.MarkShipped("DO/2026/11/00001", uuid.New(), "TRF/2026/11/00001", []domain.DeliveryLine{
		{ProductID: product1, BatchID: &b1, Qty: dec("7"), Cost: money.MustParse("70000")},
		{ProductID: product1, BatchID: &b2, Qty: dec("5"), Cost: money.MustParse("55000")},
	}, now))
	assert.Equal(t, domain.StatusInTransit, d.Status)
	assert.Equal(t, "DO/2026/11/00001", d.Label())
	assert.Equal(t, "125000.00", d.Cost.String())
	assert.Equal(t, 2, d.Lines[1].LineNo)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, d.Actions(), "diterima lewat DO")
	legacy := d
	legacy.SourceType, legacy.SourceID = domain.SourceConsignmentDelivery, nil
	assert.Equal(t, []domain.Action{domain.ActionReceive, domain.ActionPrint}, legacy.Actions(), "data lama tanpa DO")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.Update(upd)))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.Cancel("batal")))

	require.NoError(t, d.MarkReceived(now))
	assert.Equal(t, domain.StatusCompleted, d.Status)
	assert.NotNil(t, d.ReceivedAt)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, d.Actions())
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(d.MarkReceived(now)))

	c, err := domain.NewConsignmentDelivery(deliveryInput())
	require.NoError(t, err)
	assert.Contains(t, fields(t, c.Cancel(" ")), "reason")
	require.NoError(t, c.Cancel("Mitra tutup"))
	assert.Equal(t, domain.StatusCancelled, c.Status)
	assert.Nil(t, c.Actions())
	upd.Lines = nil
	d2, _ := domain.NewConsignmentDelivery(deliveryInput())
	assert.Contains(t, fields(t, d2.Update(upd)), "lines")
}

func TestVarianceRules(t *testing.T) {
	r := domain.VarianceRules{}.Normalize()
	assert.Equal(t, domain.DefaultVarianceRules, r)
	assert.Equal(t, domain.BorneByPartner, r.For(domain.CauseLost))
	assert.Equal(t, domain.BorneByPartner, r.For(domain.CausePartnerDamage))
	assert.Equal(t, domain.BorneByBalans, r.For(domain.CauseProductDefect))
	assert.Equal(t, domain.BorneByBalans, r.For(domain.CauseNearExpiry))
	assert.Equal(t, domain.BorneBy(""), r.For("x"))
	custom := domain.VarianceRules{Lost: domain.BorneByBalans, NearExpiry: domain.BorneByPartner}
	assert.Equal(t, domain.BorneByBalans, custom.For(domain.CauseLost), "per PKS")
	assert.Equal(t, domain.BorneByPartner, custom.For(domain.CauseNearExpiry))
	assert.Equal(t, "Ditagih ke mitra", domain.BorneByPartner.Label())
	assert.Equal(t, "Ditanggung Balans", domain.BorneByBalans.Label())
	assert.Empty(t, domain.BorneBy("").Label())
	assert.Equal(t, "Hilang", domain.CauseLost.Label())
	assert.Equal(t, "x", domain.VarianceCause("x").Label())
	assert.False(t, domain.VarianceCause("x").Valid())
}

func TestConsignmentUnitPrice(t *testing.T) {
	m := dec("20")
	assert.Equal(t, "70000.00", domain.ConsignmentUnitPrice(money.MustParse("70000"), true, &m).String(), "harga PKS apa adanya")
	assert.Equal(t, "40000.00", domain.ConsignmentUnitPrice(money.MustParse("50000"), false, &m).String(), "dipotong margin")
	assert.Equal(t, "50000.00", domain.ConsignmentUnitPrice(money.MustParse("50000"), false, nil).String())
}

func TestPeriodStartAfter(t *testing.T) {
	cut := clock.NewBusinessDate(2026, 11, 30)
	assert.Equal(t, "2026-11-01", domain.PeriodStartAfter(nil, cut).String())
	last := clock.NewBusinessDate(2026, 10, 31)
	assert.Equal(t, "2026-11-01", domain.PeriodStartAfter(&last, cut).String())
}

func newReconciliation(t *testing.T) domain.Reconciliation {
	t.Helper()
	r, err := domain.NewReconciliation(domain.ReconciliationInput{PartnerID: uuid.New(), CutOffDate: today, StockCountID: uuid.New()})
	require.NoError(t, err)
	return r
}

func startedReconciliation(t *testing.T) domain.Reconciliation {
	t.Helper()
	r := newReconciliation(t)
	require.NoError(t, r.Start("KSN/2026/11/00001", clock.NewBusinessDate(2026, 11, 1), []domain.LineSource{
		{ProductID: product1, OpeningQty: dec("5"), DeliveredQty: dec("50"), ReturnedQty: dec("5"), CountedQty: dec("30"), UnitPrice: money.MustParse("70000")},
		{ProductID: product2, DeliveredQty: dec("10"), CountedQty: dec("6"), UnitPrice: money.MustParse("40000")},
	}, now))
	for i := range r.Lines {
		r.Lines[i].ID = uuid.New()
	}
	return r
}

func TestNewReconciliationValidation(t *testing.T) {
	f := fields(t, func() error {
		_, err := domain.NewReconciliation(domain.ReconciliationInput{Note: strings.Repeat("x", 1001)})
		return err
	}())
	for _, k := range []string{"partnerId", "cutOffDate", "stockCountId", "note"} {
		assert.Contains(t, f, k)
	}
	r := newReconciliation(t)
	assert.Equal(t, "Draft", r.Label())
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionStart, domain.ActionCancel}, r.Actions())
	sc := uuid.New()
	require.NoError(t, r.UpdateDraft(domain.ReconciliationInput{CutOffDate: today.AddDays(1), StockCountID: sc, Note: " catatan "}))
	assert.Equal(t, sc, r.StockCountID)
	assert.Equal(t, "catatan", r.Note)
	assert.Contains(t, fields(t, r.UpdateDraft(domain.ReconciliationInput{})), "cutOffDate")
	assert.Contains(t, fields(t, r.Start("KSN/1", today.AddDays(5), []domain.LineSource{{ProductID: product1}}, now)), "cutOffDate")
	assert.Contains(t, fields(t, r.Start("KSN/1", today, nil, now)), "stockCountId")
}

func TestReconciliationLifecycle(t *testing.T) {
	r := startedReconciliation(t)
	assert.Equal(t, domain.StatusInProgress, r.Status)
	assert.Equal(t, "KSN/2026/11/00001", r.Label())
	assert.Equal(t, "20", r.Lines[0].SoldQty.String(), "5 + 50 − 5 − 30")
	assert.Equal(t, "20", r.Lines[0].ReportedQty.String(), "laporan mitra awal = terjual sistem")
	assert.True(t, r.Lines[0].VarianceQty.IsZero())
	assert.Equal(t, "1560000.00", r.SalesAmount.String())
	assert.False(t, r.HasVariance())
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.UpdateDraft(domain.ReconciliationInput{})))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Start("x", today, nil, now)))

	// Laporan mitra lebih kecil → selisih wajib penyebab; BA wajib.
	require.NoError(t, r.Update(domain.ReconciliationUpdate{Lines: []domain.LineEntry{
		{LineID: r.Lines[0].ID, ReportedQty: dec("17")},
		{LineID: r.Lines[1].ID, ReportedQty: dec("2"), Cause: domain.CauseProductDefect, Note: " cacat "},
	}}))
	assert.True(t, r.HasVariance())
	f := fields(t, r.Submit(now))
	assert.Contains(t, f, "baFileId")
	assert.Equal(t, "Pilih penyebab selisih", f["lines.0.varianceCause"])

	ba := uuid.New()
	require.NoError(t, r.Update(domain.ReconciliationUpdate{
		Lines:    []domain.LineEntry{{LineID: r.Lines[0].ID, ReportedQty: dec("17"), Cause: domain.CauseLost}},
		SetFiles: true, BAFileID: &ba, SetNote: true, Note: "Cut-off Nov",
	}))
	assert.Equal(t, "Cut-off Nov", r.Note)
	assert.Equal(t, domain.BorneByPartner, r.Lines[0].BorneBy)
	assert.Equal(t, "210000.00", r.Lines[0].VarianceAmount.String())
	assert.Equal(t, domain.BorneByBalans, r.Lines[1].BorneBy)
	assert.True(t, r.Lines[1].VarianceAmount.IsZero(), "ditanggung Balans tidak ditagih")
	assert.Equal(t, "cacat", r.Lines[1].Note)
	assert.Equal(t, "1270000.00", r.SalesAmount.String())
	assert.Equal(t, "210000.00", r.BilledVariance.String())
	assert.Equal(t, "1480000.00", r.ApprovalAmount().String())
	assert.Equal(t, "20", r.Lines[0].IssueQty().String())

	// Laporan sama dengan terjual → penyebab dikosongkan.
	r2 := startedReconciliation(t)
	require.NoError(t, r2.Update(domain.ReconciliationUpdate{Lines: []domain.LineEntry{{LineID: r2.Lines[0].ID, ReportedQty: dec("20"), Cause: domain.CauseLost}}}))
	assert.Empty(t, r2.Lines[0].Cause)

	require.NoError(t, r.Submit(now))
	assert.Equal(t, domain.StatusPendingApproval, r.Status)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, r.Actions())
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Update(domain.ReconciliationUpdate{})))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Submit(now)))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Cancel("x")))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Revise()))

	// Ditolak → revisi → diajukan ulang → diposting.
	require.NoError(t, r.Reject(" salah hitung ", now))
	assert.Equal(t, domain.StatusRejected, r.Status)
	assert.Equal(t, "salah hitung", r.DecisionNote)
	assert.Equal(t, []domain.Action{domain.ActionRevise, domain.ActionPrint}, r.Actions())
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.Reject("", now)))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(r.MarkPosted(nil, nil, nil, "", now)))
	require.NoError(t, r.Revise())
	assert.Equal(t, domain.StatusInProgress, r.Status)
	require.NoError(t, r.Submit(now))
	inv, vinv := uuid.New(), uuid.New()
	require.NoError(t, r.MarkPosted([]domain.LineCost{
		{LineID: r.Lines[0].ID, Cost: money.MustParse("680000"), VarianceCost: money.MustParse("120000")},
		{LineID: r.Lines[1].ID, Cost: money.MustParse("40000"), VarianceCost: money.MustParse("40000")},
	}, &inv, &vinv, "ok", now))
	assert.Equal(t, domain.StatusPosted, r.Status)
	assert.Equal(t, "840000.00", r.Cost.String())
	assert.Equal(t, "40000.00", r.AbsorbedCost.String())
	assert.Equal(t, &inv, r.InvoiceID)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, r.Actions())
}

func TestReconciliationUpdateValidation(t *testing.T) {
	r := startedReconciliation(t)
	f := fields(t, r.Update(domain.ReconciliationUpdate{
		Lines: []domain.LineEntry{
			{LineID: uuid.New(), ReportedQty: dec("1")},
			{LineID: r.Lines[0].ID, ReportedQty: dec("-1")},
			{LineID: r.Lines[0].ID, ReportedQty: dec("1.0001")},
			{LineID: r.Lines[0].ID, ReportedQty: dec("21")},
			{LineID: r.Lines[1].ID, ReportedQty: dec("1"), Cause: "x", Note: strings.Repeat("n", 201)},
		},
		SetNote: true, Note: strings.Repeat("x", 1001),
	}))
	assert.Contains(t, f, "lines.0.lineId")
	assert.Contains(t, f, "lines.1.reportedQty")
	assert.Contains(t, f, "lines.2.reportedQty")
	assert.Contains(t, f["lines.3.reportedQty"], "melebihi terjual sistem")
	assert.Contains(t, f, "lines.4.varianceCause")
	assert.Contains(t, f, "lines.4.note")
	assert.Contains(t, f, "note")

	// Sisa fisik melebihi saldo sistem → terjual negatif, tidak bisa diajukan.
	neg := newReconciliation(t)
	require.NoError(t, neg.Start("KSN/2", today, []domain.LineSource{{ProductID: product1, DeliveredQty: dec("5"), CountedQty: dec("7")}}, now))
	assert.Equal(t, "-2", neg.Lines[0].SoldQty.String())
	assert.True(t, neg.Lines[0].ReportedQty.IsZero())
	ba := uuid.New()
	neg.BAFileID = &ba
	assert.Contains(t, fields(t, neg.Submit(now)), "lines.0.countedQty")
	neg.Lines[0].SoldQty, neg.Lines[0].VarianceQty = decimal.NewFromInt(1), decimal.NewFromInt(-1)
	assert.Contains(t, fields(t, neg.Submit(now)), "lines.0.reportedQty")

	c := newReconciliation(t)
	assert.Contains(t, fields(t, c.Cancel("")), "reason")
	require.NoError(t, c.Cancel("Salah mitra"))
	assert.Equal(t, domain.StatusCancelled, c.Status)
	assert.Nil(t, c.Actions())
}
