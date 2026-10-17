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
	"balans/internal/platform/money"
	"balans/internal/sales/domain"
)

type returnFixture struct {
	order    domain.Order
	invoice  domain.Invoice
	line     uuid.UUID
	bundle   uuid.UUID
	product  uuid.UUID
	compA    uuid.UUID
	compB    uuid.UUID
	batch    uuid.UUID
	reason   domain.ReturnReason
	inactive domain.ReturnReason
	purchase domain.ReturnReason
	costs    map[uuid.UUID]domain.ShipmentCost
}

// fixture: SO MT tempo, baris 1 produk 10 pcs @100.000 (diskon dokumen 10%), baris 2 bundel 2 paket @300.000
// (komponen A 2 pcs + B 1 pcs). Semua terkirim; HPP A 40.000, B 20.000, produk 40.000.
func newReturnFixture() returnFixture {
	f := returnFixture{
		line: uuid.New(), bundle: uuid.New(), product: uuid.New(), compA: uuid.New(), compB: uuid.New(), batch: uuid.New(),
	}
	partner := uuid.New()
	bundleProduct := uuid.New()
	alloc1, alloc2, alloc3 := uuid.New(), uuid.New(), uuid.New()
	f.order = domain.Order{
		ID: uuid.New(), Number: "SO/2026/11/00001", Type: domain.OrderModernTrade, ChannelID: uuid.New(), SubChannelID: uuid.New(),
		PartnerID: &partner, PartyName: "Mitra", BusinessDate: clock.NewBusinessDate(2026, 11, 2), SourceLocationID: uuid.New(),
		Status: domain.StatusInTransit, Gross: money.FromInt(1600000), LineDiscount: money.Zero,
		DocumentDiscountAmount: money.FromInt(160000), Total: money.FromInt(1440000),
		Lines: []domain.Line{
			{ID: f.line, LineNo: 1, ProductID: f.product, Qty: dec("10"), Total: money.FromInt(1000000)},
			{ID: f.bundle, LineNo: 2, ProductID: bundleProduct, IsBundle: true, Qty: dec("2"), Total: money.FromInt(600000)},
		},
		Allocations: []domain.Allocation{
			{ID: alloc1, LineID: f.line, ProductID: f.product, Qty: dec("10"), ShippedQty: dec("10"),
				Batches: []domain.AllocationBatch{{BatchID: &f.batch, Qty: dec("10")}}},
			{ID: alloc2, LineID: f.bundle, ProductID: f.compA, Qty: dec("4"), ShippedQty: dec("4")},
			{ID: alloc3, LineID: f.bundle, ProductID: f.compB, Qty: dec("2"), ShippedQty: dec("2")},
		},
	}
	f.costs = domain.ShipmentCosts([]domain.Shipment{{Lines: []domain.ShipmentLine{
		{AllocationID: alloc1, Qty: dec("6"), Cost: money.FromInt(240000)},
		{AllocationID: alloc2, Qty: dec("4"), Cost: money.FromInt(160000)},
		{AllocationID: alloc3, Qty: dec("2"), Cost: money.FromInt(40000)},
	}}, {Lines: []domain.ShipmentLine{{AllocationID: alloc1, Qty: dec("4"), Cost: money.FromInt(160000)}}}})
	f.invoice = domain.Invoice{
		ID: uuid.New(), Number: "INV/1", PartyType: domain.PartyPartner, PartyID: partner, Total: money.FromInt(1440000),
		Paid: money.Zero, Status: domain.StatusAwaitingPayment,
	}
	f.reason = domain.ReturnReason{ID: uuid.New(), Code: "RUSAK", Name: "Rusak", AppliesTo: domain.ScopeBoth, IsActive: true}
	f.inactive = domain.ReturnReason{ID: uuid.New(), Code: "LAMA", Name: "Lama", AppliesTo: domain.ScopeSalesReturn}
	f.purchase = domain.ReturnReason{ID: uuid.New(), Code: "PO", Name: "Beli", AppliesTo: domain.ScopePurchaseReturn, IsActive: true}
	return f
}

func (f returnFixture) refs(returned map[uuid.UUID]decimal.Decimal) domain.ReturnRefs {
	inv := f.invoice
	return domain.ReturnRefs{
		Order: f.order, Invoice: &inv, Returnable: domain.ReturnableLines(f.order, f.costs, returned),
		Reasons: map[uuid.UUID]domain.ReturnReason{f.reason.ID: f.reason, f.inactive.ID: f.inactive, f.purchase.ID: f.purchase},
		Today:   clock.NewBusinessDate(2026, 11, 3),
	}
}

func (f returnFixture) input(lines ...domain.ReturnLineInput) domain.ReturnInput {
	return domain.ReturnInput{
		OrderID: f.order.ID, ReturnDate: clock.NewBusinessDate(2026, 11, 3), Settlement: domain.SettlementCreditNote, Lines: lines,
	}
}

func TestReturnableLines(t *testing.T) {
	f := newReturnFixture()
	lines := domain.ReturnableLines(f.order, f.costs, map[uuid.UUID]decimal.Decimal{f.line: dec("3")})
	require.Len(t, lines, 2)
	assert.Equal(t, "10", lines[0].ShippedQty.String())
	assert.Equal(t, "7", lines[0].AvailableQty().String())
	assert.Equal(t, "90000.00", lines[0].UnitPrice.String(), "harga bersih setelah diskon dokumen 10%")
	assert.Equal(t, "40000", lines[0].UnitCost.String())
	assert.Equal(t, []uuid.UUID{f.batch}, lines[0].Batches)
	assert.Equal(t, "100000", lines[1].UnitCost.String(), "bundel: 2×40.000 + 1×20.000")
	assert.Equal(t, "270000.00", lines[1].UnitPrice.String())

	over := domain.ReturnableLine{ShippedQty: dec("1"), ReturnedQty: dec("2")}
	assert.True(t, over.AvailableQty().IsZero())
}

func TestNewSalesReturnValidation(t *testing.T) {
	f := newReturnFixture()
	ok := domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("2"), ReasonID: f.reason.ID}
	tests := []struct {
		name  string
		in    func() domain.ReturnInput
		field string
	}{
		{"tanpa baris", func() domain.ReturnInput { return f.input() }, "lines"},
		{"qty melebihi terkirim", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("11"), ReasonID: f.reason.ID})
		}, "lines.0.qty"},
		{"qty total dua baris melebihi", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("6"), ReasonID: f.reason.ID},
				domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("5"), ReasonID: f.reason.ID})
		}, "lines.1.qty"},
		{"qty nol", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("0"), ReasonID: f.reason.ID})
		}, "lines.0.qty"},
		{"baris lain", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: uuid.New(), Qty: dec("1"), ReasonID: f.reason.ID})
		}, "lines.0.salesOrderLineId"},
		{"alasan kosong", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1")})
		}, "lines.0.returnReasonId"},
		{"alasan nonaktif", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.inactive.ID})
		}, "lines.0.returnReasonId"},
		{"alasan khusus retur pembelian", func() domain.ReturnInput {
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.purchase.ID})
		}, "lines.0.returnReasonId"},
		{"batch bukan yang dikirim", func() domain.ReturnInput {
			b := uuid.New()
			return f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.reason.ID, BatchID: &b})
		}, "lines.0.batchId"},
		{"tanggal kosong", func() domain.ReturnInput {
			in := f.input(ok)
			in.ReturnDate = clock.BusinessDate{}
			return in
		}, "returnDate"},
		{"tanggal sebelum SO", func() domain.ReturnInput {
			in := f.input(ok)
			in.ReturnDate = clock.NewBusinessDate(2026, 11, 1)
			return in
		}, "returnDate"},
		{"tanggal masa depan", func() domain.ReturnInput {
			in := f.input(ok)
			in.ReturnDate = clock.NewBusinessDate(2026, 12, 1)
			return in
		}, "returnDate"},
		{"cara penyelesaian", func() domain.ReturnInput {
			in := f.input(ok)
			in.Settlement = domain.SettlementMarketplace
			return in
		}, "settlementMethod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewSalesReturn(tt.in(), f.refs(nil), uuid.New())
			require.Error(t, err)
			assert.Contains(t, apperr.As(err).Fields, tt.field)
		})
	}

	// Tanpa invoice (non-marketplace) ditolak.
	refs := f.refs(nil)
	refs.Invoice = nil
	_, err := domain.NewSalesReturn(f.input(ok), refs, uuid.New())
	require.Error(t, err)
	assert.Contains(t, apperr.As(err).Fields, "salesOrderId")

	// Pesanan belum dikirim ditolak.
	o := f.order
	o.Status = domain.StatusDraft
	refs = f.refs(nil)
	refs.Order = o
	_, err = domain.NewSalesReturn(f.input(ok), refs, uuid.New())
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	o = f.order
	o.Allocations = nil
	refs.Order = o
	_, err = domain.NewSalesReturn(f.input(ok), refs, uuid.New())
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
}

func TestNewSalesReturnAmountsAndItems(t *testing.T) {
	f := newReturnFixture()
	r, err := domain.NewSalesReturn(f.input(
		domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("3"), ReasonID: f.reason.ID, Note: " penyok "},
		domain.ReturnLineInput{OrderLineID: f.bundle, Qty: dec("1"), ReasonID: f.reason.ID},
	), f.refs(nil), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, r.Status)
	assert.True(t, r.RequiresVerification, "retur mitra diverifikasi Logistik Leader")
	assert.Equal(t, "Draft Retur Penjualan", r.Label())
	assert.Equal(t, "540000.00", r.Amount.String(), "3×90.000 + 1×270.000")
	assert.Equal(t, "220000.00", r.Cost.String(), "3×40.000 + 1×100.000")
	require.Len(t, r.Lines, 2)
	assert.Equal(t, &f.batch, r.Lines[0].BatchID, "batch tunggal otomatis")
	assert.Equal(t, "penyok", r.Lines[0].Note)
	require.Len(t, r.Items, 3, "bundel dipecah ke komponen")
	assert.Equal(t, "2", r.Items[1].Qty.String())
	assert.Equal(t, "80000.00", r.Items[1].Cost.String())
	assert.Equal(t, 2, r.Items[2].LineNo)

	// Update draft mengganti baris; Sales Order tidak bisa diganti.
	require.NoError(t, r.Update(f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.reason.ID}), f.refs(nil)))
	assert.Len(t, r.Lines, 1)
	other := f.input()
	other.OrderID = uuid.New()
	assert.Contains(t, apperr.As(r.Update(other, f.refs(nil))).Fields, "salesOrderId")

	// Marketplace: penyelesaian dipaksa marketplace, tanpa verifikasi, tanpa invoice.
	mp := f.order
	mp.Type, mp.PartnerID = domain.OrderMarketplace, nil
	refs := f.refs(nil)
	refs.Order, refs.Invoice = mp, nil
	in := f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.reason.ID})
	in.Settlement = ""
	m, err := domain.NewSalesReturn(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.SettlementMarketplace, m.Settlement)
	assert.False(t, m.RequiresVerification)
	assert.Equal(t, "Potongan settlement marketplace", m.Settlement.Label())
	assert.Equal(t, "x", domain.SettlementMethod("x").Label())
	assert.True(t, domain.SettlementRefund.Valid())
}

func withIDs(r *domain.SalesReturn) {
	for i := range r.Lines {
		r.Lines[i].ID = uuid.New()
		for j := range r.Items {
			if r.Items[j].LineNo == r.Lines[i].LineNo {
				r.Items[j].LineID = r.Lines[i].ID
			}
		}
	}
	for j := range r.Items {
		r.Items[j].ID = uuid.New()
	}
}

func TestSalesReturnVerificationFlow(t *testing.T) {
	f := newReturnFixture()
	r, err := domain.NewSalesReturn(f.input(
		domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("4"), ReasonID: f.reason.ID},
		domain.ReturnLineInput{OrderLineID: f.bundle, Qty: dec("2"), ReasonID: f.reason.ID},
	), f.refs(nil), uuid.New())
	require.NoError(t, err)
	withIDs(&r)
	now := time.Date(2026, 11, 3, 3, 0, 0, 0, time.UTC)
	sales, ll, merch := uuid.New(), uuid.New(), uuid.New()
	ctx := domain.ReturnContext{ActorID: merch}
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, r.Actions(ctx))

	require.NoError(t, r.Submit("RJ/1", sales, now))
	assert.Equal(t, domain.StatusSubmitted, r.Status)
	assert.Equal(t, "Diajukan", r.Status.Label())
	assert.Equal(t, []domain.Action{domain.ActionVerify, domain.ActionSendBack, domain.ActionCancel}, r.Actions(ctx))
	assert.Error(t, r.SendBack(" "))
	require.NoError(t, r.SendBack("Foto kurang"))
	assert.Equal(t, domain.StatusDraft, r.Status)
	assert.Error(t, r.SendBack("lagi"), "hanya dari Diajukan")
	require.NoError(t, r.Submit("RJ/2", sales, now))
	assert.Equal(t, "RJ/1", r.Number, "nomor tetap")
	assert.Empty(t, r.SendBackNote)

	assert.Error(t, r.AssignPickup(merch, now), "belum terverifikasi")
	require.NoError(t, r.Verify(ll, true, " ok ", now))
	assert.Equal(t, "Terverifikasi", r.Status.Label())
	assert.Equal(t, "ok", r.VerifyNote)
	assert.Equal(t, []domain.Action{domain.ActionAssignPickup, domain.ActionCancel}, r.Actions(ctx))
	assert.Error(t, r.Receive(sales, "", []uuid.UUID{uuid.New()}, nil, now), "belum diambil")
	assert.Error(t, r.RecordPickup(merch, "", []uuid.UUID{uuid.New()}, now), "belum ditugaskan")
	assert.Error(t, r.AssignPickup(uuid.Nil, now))
	require.NoError(t, r.AssignPickup(merch, now))
	assert.Contains(t, r.Actions(ctx), domain.ActionPickup)
	assert.True(t, apperr.HasCode(r.RecordPickup(uuid.New(), "", []uuid.UUID{uuid.New()}, now), apperr.CodeForbidden))
	assert.Contains(t, apperr.As(r.RecordPickup(merch, "", nil, now)).Fields, "photoFileIds")
	photo := uuid.New()
	require.NoError(t, r.RecordPickup(merch, "dicek", []uuid.UUID{photo, photo}, now))
	assert.Len(t, r.PickupFileIDs, 1)
	assert.Error(t, r.RecordPickup(merch, "", []uuid.UUID{photo}, now), "sudah dicatat")
	assert.Error(t, r.AssignPickup(merch, now), "sudah diambil")
	assert.Equal(t, []domain.Action{domain.ActionReceive, domain.ActionCancel}, r.Actions(ctx))

	assert.Contains(t, apperr.As(r.Receive(sales, "", nil, nil, now)).Fields, "documentationFileIds")
	bad := []domain.ReceivedQty{{LineID: r.Lines[0].ID, Qty: dec("5")}, {LineID: uuid.New(), Qty: dec("1")}, {LineID: r.Lines[1].ID, Qty: dec("0")}}
	fields := apperr.As(r.Receive(sales, "", []uuid.UUID{photo}, bad, now)).Fields
	assert.Contains(t, fields, "lines.0.qty")
	assert.Contains(t, fields, "lines.1.lineId")
	assert.Contains(t, fields, "lines.2.qty")
	require.NoError(t, r.Receive(sales, "cek ulang", []uuid.UUID{photo}, []domain.ReceivedQty{
		{LineID: r.Lines[0].ID, Qty: dec("2")}, {LineID: r.Lines[1].ID, Qty: dec("1")},
	}, now))
	assert.Equal(t, domain.StatusInProgress, r.Status)
	assert.Equal(t, "2", r.Lines[0].Qty.String())
	assert.Equal(t, "180000.00", r.Lines[0].Amount.String())
	assert.Equal(t, "450000.00", r.Amount.String(), "2×90.000 + 1×270.000")
	assert.Equal(t, "180000.00", r.Cost.String(), "2×40.000 + 1×100.000")
	assert.Equal(t, "2", r.Items[1].Qty.String(), "komponen A bundel ikut turun")
	assert.Empty(t, r.Actions(ctx))
	assert.Error(t, r.Cancel("batal"), "sudah masuk Inbound")

	// QC per item; baris bundel dihitung dari komponen terkecil.
	assert.Error(t, r.Post(uuid.New(), now), "QC belum selesai")
	assert.True(t, apperr.HasCode(r.ApplyQC(uuid.New(), dec("1"), decimal.Zero), apperr.CodeNotFound))
	require.NoError(t, r.ApplyQC(r.Items[0].ID, dec("1"), dec("1")))
	p, rej, done := r.LineQC(r.Lines[0].ID)
	assert.True(t, done)
	assert.Equal(t, "1", p.String())
	assert.Equal(t, "1", rej.String())
	require.NoError(t, r.ApplyQC(r.Items[1].ID, dec("2"), decimal.Zero))
	_, _, done = r.LineQC(r.Lines[1].ID)
	assert.False(t, done)
	assert.False(t, r.QCComplete())
	require.NoError(t, r.ApplyQC(r.Items[2].ID, decimal.Zero, dec("1")))
	p, rej, done = r.LineQC(r.Lines[1].ID)
	assert.True(t, done)
	assert.Equal(t, "0", p.String())
	assert.Equal(t, "1", rej.String())
	_, _, done = r.LineQC(uuid.New())
	assert.False(t, done)
	require.NoError(t, r.Post(uuid.New(), now))
	assert.Equal(t, domain.StatusPosted, r.Status)
	assert.NotNil(t, r.JournalEntryID)
	assert.Error(t, r.ApplyQC(r.Items[0].ID, dec("1"), decimal.Zero))
}

func TestSalesReturnDirectAndCancel(t *testing.T) {
	f := newReturnFixture()
	cust := uuid.New()
	o := f.order
	o.PartnerID, o.CustomerID, o.Type = nil, &cust, domain.OrderSocialCommerce
	refs := f.refs(nil)
	refs.Order = o
	r, err := domain.NewSalesReturn(f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.reason.ID}), refs, uuid.New())
	require.NoError(t, err)
	assert.False(t, r.RequiresVerification)
	by := uuid.New()
	require.NoError(t, r.Submit("RJ/9", by, time.Now()))
	assert.Equal(t, domain.StatusInProgress, r.Status, "retur pelanggan langsung Diproses")
	assert.Equal(t, &by, r.ReceivedBy)
	assert.Equal(t, "RJ/9", r.Label())

	c, err := domain.NewSalesReturn(f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("1"), ReasonID: f.reason.ID}), f.refs(nil), uuid.New())
	require.NoError(t, err)
	assert.Error(t, c.Cancel(""))
	require.NoError(t, c.Cancel("Salah input"))
	assert.Equal(t, domain.StatusCancelled, c.Status)
	assert.Error(t, c.Verify(by, false, "", time.Now()))
	assert.Error(t, c.Update(f.input(), f.refs(nil)))
	assert.False(t, c.QCComplete())
}

func TestCreditNote(t *testing.T) {
	f := newReturnFixture()
	r, err := domain.NewSalesReturn(f.input(domain.ReturnLineInput{OrderLineID: f.line, Qty: dec("2"), ReasonID: f.reason.ID}), f.refs(nil), uuid.New())
	require.NoError(t, err)
	r.ID, r.Number = uuid.New(), "RJ/1"
	date := clock.NewBusinessDate(2026, 11, 3)

	cn, err := domain.NewCreditNote("CN/1", r, date)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPosted, cn.Status)
	assert.Equal(t, "180000.00", cn.Remaining().String())
	assert.Equal(t, []domain.Action{domain.ActionPrint, domain.ActionApply}, cn.Actions())

	inv := f.invoice
	inv.Total, inv.Paid = money.FromInt(100000), money.Zero
	other := inv
	other.PartyID = uuid.New()
	_, err = cn.Apply(other, money.FromInt(1000), date)
	assert.Contains(t, apperr.As(err).Fields, "invoiceId")
	_, err = cn.Apply(inv, money.FromInt(150000), date)
	assert.Contains(t, apperr.As(err).Fields, "amount", "melebihi sisa piutang")
	_, err = cn.Apply(inv, money.FromInt(-1), date)
	assert.Contains(t, apperr.As(err).Fields, "amount")
	app, err := cn.Apply(inv, money.FromInt(100000), date)
	require.NoError(t, err)
	assert.Equal(t, "INV/1", app.InvoiceNumber)
	_, err = cn.Apply(inv, money.FromInt(90000), date)
	assert.Contains(t, apperr.As(err).Fields, "amount", "melebihi saldo")
	inv2 := f.invoice
	app, err = cn.Apply(inv2, money.FromInt(80000), date)
	require.NoError(t, err)
	assert.Equal(t, money.FromInt(80000), app.Amount)
	assert.Equal(t, domain.StatusCompleted, cn.Status)
	_, err = cn.Apply(inv2, money.FromInt(1), date)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	assert.Error(t, cn.ApproveRefund(""))
	assert.Error(t, cn.RejectRefund(""))

	// Refund: persetujuan → pembayaran.
	r.Settlement = domain.SettlementRefund
	rf, err := domain.NewCreditNote("CN/2", r, date)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPendingApproval, rf.Status)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, rf.Actions())
	assert.Error(t, rf.PayRefund(domain.RefundInput{PaymentDate: date, AccountCode: "1111"}, []string{"1111"}))
	require.NoError(t, rf.ApproveRefund(" ok "))
	assert.Equal(t, domain.StatusAwaitingPayment, rf.Status)
	assert.Contains(t, rf.Actions(), domain.ActionPayRefund)
	fields := apperr.As(rf.PayRefund(domain.RefundInput{PaymentDate: clock.NewBusinessDate(2026, 11, 1), AccountCode: "9999"}, []string{"1111"})).Fields
	assert.Contains(t, fields, "paymentDate")
	assert.Contains(t, fields, "accountCode")
	assert.Contains(t, apperr.As(rf.PayRefund(domain.RefundInput{AccountCode: "1111"}, []string{"1111"})).Fields, "paymentDate")
	require.NoError(t, rf.PayRefund(domain.RefundInput{PaymentDate: date, AccountCode: "1111", Reference: " TRF "}, []string{"1111"}))
	assert.Equal(t, domain.StatusPaid, rf.Status)
	assert.Equal(t, "180000.00", rf.Refunded.String())
	assert.Equal(t, "TRF", rf.RefundReference)
	assert.True(t, rf.Remaining().IsZero())

	// Refund ditolak → nota kredit.
	rj, err := domain.NewCreditNote("CN/3", r, date)
	require.NoError(t, err)
	require.NoError(t, rj.RejectRefund("potong tagihan"))
	assert.Equal(t, domain.SettlementCreditNote, rj.Settlement)
	assert.True(t, rj.CanApply())

	// Marketplace / nilai nol tidak membuat nota kredit.
	r.Settlement = domain.SettlementMarketplace
	_, err = domain.NewCreditNote("CN/4", r, date)
	assert.Error(t, err)
}
