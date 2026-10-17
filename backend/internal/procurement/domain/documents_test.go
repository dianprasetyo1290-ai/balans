package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/procurement/domain"
)

// orderWithIDs membuat PO Disetujui (barang atau maklon) dengan ID baris.
func orderWithIDs(t *testing.T, toll bool) (domain.PurchaseOrder, catalogFixture) {
	t.Helper()
	c := newCatalog()
	var in domain.OrderInput
	var refs domain.OrderRefs
	if toll {
		in, refs = tollOrder(c, domain.OrderTollManufacturing)
	} else {
		in, refs = packagingOrder(c)
	}
	o, err := domain.NewPurchaseOrder(in, refs, uuid.New())
	require.NoError(t, err)
	o.ID = uuid.New()
	for i := range o.Lines {
		o.Lines[i].ID = uuid.New()
	}
	require.NoError(t, o.Submit("PO/2026/10/00009", now))
	require.NoError(t, o.Approve(now))
	return o, c
}

func receiptRefs(o domain.PurchaseOrder, c catalogFixture, hasBatch bool) domain.ReceiptRefs {
	return domain.ReceiptRefs{
		Order: o,
		Products: map[uuid.UUID]domain.ReceiptProduct{
			c.pkg: {Type: domain.ProductPackaging}, c.fg: {Type: domain.ProductFinishedGood, HasBatch: hasBatch},
			c.gm: {Type: domain.ProductReprocessMaterial},
		},
		Location: &domain.ReceiptLocation{Type: domain.LocationInbound, IsActive: true},
	}
}

func TestInventoryAccount(t *testing.T) {
	cases := []struct{ productType, location, want string }{
		{domain.ProductPackaging, domain.LocationInbound, "1301"},
		{domain.ProductReprocessMaterial, domain.LocationTollManufacturer, "1302"},
		{domain.ProductFinishedGood, domain.LocationInbound, "1311"},
		{domain.ProductFinishedGood, domain.LocationReturns, "1314"},
		{domain.ProductFinishedGood, domain.LocationConsignment, "1312"},
		{domain.ProductFinishedGood, domain.LocationVendingMachine, "1313"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, domain.InventoryAccount(tc.productType, tc.location))
	}
}

func TestReceiptValidate(t *testing.T) {
	o, c := orderWithIDs(t, false)
	line := o.Lines[0].ID
	base := func() domain.ReceiptInput {
		return domain.ReceiptInput{
			PurchaseOrderID: o.ID, BusinessDate: today, LocationID: uuid.New(),
			Lines: []domain.ReceiptLineInput{{OrderLineID: line, GoodQty: qty("100"), RejectQty: qty("5")}},
		}
	}
	refs := receiptRefs(o, c, false)
	in := base()
	require.NoError(t, in.Validate(refs))
	assert.Equal(t, domain.RejectAbsorbed, in.Lines[0].RejectDisposition, "default reject diserap")

	cases := []struct {
		name  string
		mod   func(*domain.ReceiptInput, *domain.ReceiptRefs)
		field string
	}{
		{"tanpa tanggal", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) {
			in.BusinessDate = domain.ReceiptInput{}.BusinessDate
		}, ""},
		{"sebelum tanggal PO", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.BusinessDate = today.AddDays(-1) }, "businessDate"},
		{"lokasi bukan inbound", func(_ *domain.ReceiptInput, r *domain.ReceiptRefs) {
			r.Location = &domain.ReceiptLocation{Type: "finished_goods", IsActive: true}
		}, "locationId"},
		{"lokasi nonaktif", func(_ *domain.ReceiptInput, r *domain.ReceiptRefs) {
			r.Location = &domain.ReceiptLocation{Type: "inbound"}
		}, "locationId"},
		{"lokasi tidak ada", func(_ *domain.ReceiptInput, r *domain.ReceiptRefs) { r.Location = nil }, "locationId"},
		{"tanpa lokasi", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.LocationID = uuid.Nil }, "locationId"},
		{"tanpa baris", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines = nil }, "lines"},
		{"baris PO salah", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].OrderLineID = uuid.New() }, "lines.0.orderLineId"},
		{"qty baik nol", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].GoodQty = decimal.Zero }, "lines.0.goodQty"},
		{"reject negatif", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].RejectQty = qty("-1") }, "lines.0.rejectQty"},
		{"reject 4 desimal", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].RejectQty = qty("0.0001") }, "lines.0.rejectQty"},
		{"perlakuan salah", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].RejectDisposition = "x" }, "lines.0.rejectDisposition"},
		{"melebihi sisa PO (toleransi 0%)", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].GoodQty = qty("9996") }, "lines.0.goodQty"},
		{"batch untuk produk tanpa batch", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Lines[0].BatchNumber = "B1" }, "lines.0.batchNumber"},
		{"bahan di PO barang", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) {
			in.Materials = []domain.MaterialUsageInput{{ProductID: c.pkg, UsedGoodQty: qty("1")}}
		}, "materials"},
		{"catatan panjang", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.Note = string(make([]rune, 1001)) }, "note"},
		{"surat jalan panjang", func(in *domain.ReceiptInput, _ *domain.ReceiptRefs) { in.DeliveryNote = string(make([]rune, 101)) }, "deliveryNote"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, r := base(), receiptRefs(o, c, false)
			tc.mod(&in, &r)
			err := in.Validate(r)
			if tc.field == "" {
				require.Error(t, err)
				return
			}
			assert.Contains(t, fieldsOf(t, err), tc.field)
		})
	}

	// Reject diganti pemasok tidak dihitung diterima.
	in = base()
	in.Lines[0].GoodQty, in.Lines[0].RejectQty, in.Lines[0].RejectDisposition = qty("9995"), qty("50"), domain.RejectReturnToSupplier
	require.NoError(t, in.Validate(refs))

	// PO belum disetujui ditolak.
	draft := o
	draft.Status = domain.StatusDraft
	in = base()
	requireCode(t, in.Validate(domain.ReceiptRefs{Order: draft}), apperr.CodeInvalidStateTransition)
}

func TestReceiptBatchRequired(t *testing.T) {
	o, c := orderWithIDs(t, true)
	refs := receiptRefs(o, c, true)
	in := domain.ReceiptInput{
		PurchaseOrderID: o.ID, BusinessDate: today, LocationID: uuid.New(),
		Lines:     []domain.ReceiptLineInput{{OrderLineID: o.Lines[0].ID, GoodQty: qty("4900"), RejectQty: qty("100")}},
		Materials: []domain.MaterialUsageInput{{ProductID: c.pkg, UsedGoodQty: qty("5000"), UsedRejectQty: qty("80")}},
	}
	f := fieldsOf(t, in.Validate(refs))
	assert.Contains(t, f, "lines.0.batchNumber")
	assert.Contains(t, f, "lines.0.productionDate")
	assert.Contains(t, f, "lines.0.expiryDate")
	in.Lines[0].BatchNumber, in.Lines[0].ProductionDate, in.Lines[0].ExpiryDate = " mk01 ", ptr(today), ptr(today.AddDays(-1))
	assert.Contains(t, fieldsOf(t, in.Validate(refs)), "lines.0.expiryDate")
	in.Lines[0].ExpiryDate = ptr(today.AddDays(365))
	require.NoError(t, in.Validate(refs))
	assert.Equal(t, "MK01", in.Lines[0].BatchNumber)

	// Pemakaian bahan maklon.
	bad := in
	bad.Materials = nil
	assert.Contains(t, fieldsOf(t, bad.Validate(refs)), "materials")
	bad.Materials = []domain.MaterialUsageInput{
		{ProductID: c.fg, UsedGoodQty: qty("1")}, {ProductID: c.fg, UsedGoodQty: qty("0")}, {ProductID: uuid.Nil},
		{ProductID: uuid.New(), UsedGoodQty: qty("-1")},
	}
	f = fieldsOf(t, bad.Validate(refs))
	assert.Contains(t, f, "materials.0.productId")
	assert.Contains(t, f, "materials.1.productId")
	assert.Contains(t, f, "materials.1.usedGoodQty")
	assert.Contains(t, f, "materials.2.productId")
	assert.Contains(t, f, "materials.3.usedGoodQty")

	// Maklon: nilai batch = jasa + packaging (baik + reject), HPP = nilai ÷ qty baik.
	r, err := domain.NewGoodsReceipt(in, refs, uuid.New())
	require.NoError(t, err)
	assert.True(t, r.IsToll())
	assert.Equal(t, "5000.000", r.Lines[0].GRNIQty.StringFixed(3))
	assert.Equal(t, "17500000.00", r.GRNIAmount.String())
	assert.Equal(t, qty("5080"), r.Materials[0].TotalQty())
	require.Error(t, r.ApplyCosts(nil, nil, nil))
	require.NoError(t, r.ApplyCosts([]money.Amount{money.MustParse("4064000")}, []string{"1301"}, []string{"1311"}))
	assert.Equal(t, "21564000.00", r.InventoryAmount.String())
	assert.Equal(t, "4064000.00", r.MaterialAmount.String())
	assert.Equal(t, "4400.8163", r.Lines[0].UnitCost.String())
	deltas := r.OrderDeltas(-1)
	require.Len(t, deltas, 1)
	assert.Equal(t, "-5000", deltas[0].Qty.String())
}

func TestReceiptLifecycle(t *testing.T) {
	o, c := orderWithIDs(t, false)
	refs := receiptRefs(o, c, false)
	in := domain.ReceiptInput{
		PurchaseOrderID: o.ID, BusinessDate: today, LocationID: uuid.New(), DeliveryNote: " SJ-1 ",
		Lines: []domain.ReceiptLineInput{
			{OrderLineID: o.Lines[0].ID, GoodQty: qty("60"), RejectQty: qty("5")},
			{OrderLineID: o.Lines[0].ID, GoodQty: qty("10")},
		},
	}
	r, err := domain.NewGoodsReceipt(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "Draft penerimaan PO/2026/10/00009", r.Label())
	assert.Equal(t, "SJ-1", r.DeliveryNote)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionPost, domain.ActionCancel}, r.Actions())
	deltas := r.OrderDeltas(1)
	require.Len(t, deltas, 1, "baris PO yang sama digabung")
	assert.Equal(t, "75", deltas[0].Qty.String())
	require.NoError(t, r.ApplyCosts(nil, nil, []string{"1301", "1301"}))
	assert.Equal(t, "1250.0000", r.Lines[1].UnitCost.StringFixed(4))
	assert.Equal(t, "1354.1667", r.Lines[0].UnitCost.String(), "reject diserap: 65 × 1.250 ÷ 60")

	// Update: PO tidak bisa diganti.
	in2 := r.Input()
	in2.PurchaseOrderID = uuid.New()
	assert.Contains(t, fieldsOf(t, r.Update(in2, refs)), "purchaseOrderId")
	require.NoError(t, r.Update(r.Input(), refs))

	r.ID = uuid.New()
	r.Lines[0].ID = uuid.New()
	require.NoError(t, r.Post("GR/2026/10/00001", uuid.New(), now))
	assert.Equal(t, "GR/2026/10/00001", r.Label())
	requireCode(t, r.Update(r.Input(), refs), apperr.CodeInvalidStateTransition)
	requireCode(t, r.Post("x", uuid.New(), now), apperr.CodeInvalidStateTransition)
	assert.Equal(t, []domain.Action{domain.ActionCancel}, r.Actions())
	l, ok := r.Line(r.Lines[0].ID)
	require.True(t, ok)
	assert.Equal(t, "65", l.OpenQty().String())
	_, ok = r.Line(uuid.New())
	assert.False(t, ok)

	// Sudah ditagih → tidak bisa dibatalkan.
	billed := r
	billed.Lines = append([]domain.ReceiptLine(nil), r.Lines...)
	billed.Lines[0].BilledQty = qty("1")
	assert.Empty(t, billed.Actions())
	requireCode(t, billed.Cancel("salah", now), apperr.CodeInvalidStateTransition)
	assert.Contains(t, fieldsOf(t, r.Cancel("", now)), "reason")
	require.NoError(t, r.Cancel("Salah input", now))
	assert.Equal(t, domain.StatusCancelled, r.Status)
	assert.Empty(t, r.Actions())

	draft, err := domain.NewGoodsReceipt(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Contains(t, fieldsOf(t, draft.Cancel(string(make([]rune, 401)), now)), "reason")
	require.NoError(t, draft.Cancel("", now))
}

func billRefs(o domain.PurchaseOrder, line domain.ReceiptLine) domain.BillRefs {
	return domain.BillRefs{
		Order: o, DefaultWithholding: domain.WithholdingPPh23,
		ReceiptLines: map[uuid.UUID]domain.BillReceiptLine{line.ID: {
			Line: line, ReceiptID: uuid.New(), ReceiptNumber: "GR/2026/10/00001", ReceiptStatus: domain.StatusPosted,
			PurchaseOrderID: o.ID,
		}},
	}
}

func receivedOrder(t *testing.T) (domain.PurchaseOrder, domain.ReceiptLine) {
	t.Helper()
	o, _ := orderWithIDs(t, false)
	require.NoError(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: o.Lines[0].ID, Qty: qty("100")}}))
	line := domain.ReceiptLine{
		ID: uuid.New(), OrderLineID: o.Lines[0].ID, ProductID: uuid.New(), GoodQty: qty("100"), GRNIQty: qty("100"),
		UnitPrice: money.MustParse("1250"), GRNIAmount: money.MustParse("125000"), BilledQty: decimal.Zero,
		BilledGRNIAmount: money.Zero, ReturnedQty: decimal.Zero, ReturnedUnbilledQty: decimal.Zero, ReturnedGRNIAmount: money.Zero,
	}
	return o, line
}

func TestBillMatchAndLifecycle(t *testing.T) {
	o, line := receivedOrder(t)
	refs := billRefs(o, line)
	in := domain.BillInput{
		PurchaseOrderID: o.ID, Number: " INV-1 ", BillDate: today,
		Lines: []domain.BillLineInput{{ReceiptLineID: line.ID, Qty: qty("100"), UnitPrice: money.MustParse("1262.5")}},
	}
	b, err := domain.NewSupplierBill(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "INV-1", b.Label())
	assert.Equal(t, today.AddDays(30), b.DueDate, "jatuh tempo = tanggal tagihan + termin PO")
	assert.Equal(t, domain.WithholdingPPh23, b.WithholdingType)
	assert.Equal(t, "2", b.WithholdingRate.String())
	assert.Equal(t, domain.MatchMatched, b.MatchStatus, "selisih harga tepat 1% masih dalam toleransi")
	assert.Equal(t, "126250.00", b.Subtotal.String())
	assert.Equal(t, "125000.00", b.GRNIAmount.String())
	assert.Equal(t, "1250.00", b.PriceVariance.String())
	assert.Equal(t, "2525.00", b.WithholdingAmount.String())
	assert.Equal(t, "123725.00", b.TotalPayable.String())
	assert.Equal(t, "1.0000", b.Lines[0].PriceVariancePercent().StringFixed(4))
	assert.True(t, b.PaymentBlocked())

	// Harga +1,01% atau qty kurang dari GR → di luar toleransi.
	in.Lines[0].UnitPrice = money.MustParse("1262.63")
	require.NoError(t, b.Update(in, refs))
	assert.Equal(t, domain.MatchExceedsTolerance, b.MatchStatus)
	assert.True(t, b.Lines[0].PriceMismatch())
	in.Lines[0].UnitPrice, in.Lines[0].Qty = money.MustParse("1250"), qty("99")
	require.NoError(t, b.Update(in, refs))
	assert.True(t, b.Lines[0].QtyMismatch())
	assert.True(t, b.ExceedsTolerance())

	// Di luar toleransi → Menunggu Persetujuan → disetujui → Menunggu Pembayaran dengan potongan uang muka.
	require.NoError(t, b.Submit(now))
	assert.Equal(t, domain.StatusPendingApproval, b.Status)
	assert.Empty(t, b.Actions())
	require.Error(t, b.Finalize([]domain.BillAdvance{{Amount: money.MustParse("1000000")}}, uuid.New(), nil, now))
	advance := uuid.New()
	require.NoError(t, b.Finalize([]domain.BillAdvance{{AdvanceID: advance, Amount: money.MustParse("50000")}}, uuid.New(), &advance, now))
	assert.Equal(t, domain.StatusAwaitingPayment, b.Status)
	assert.Equal(t, domain.MatchApprovedException, b.MatchStatus)
	assert.False(t, b.PaymentBlocked())
	open := b.OpenAmount()
	assert.Equal(t, b.TotalPayable, open)

	// Pembayaran (R1.4).
	assert.Contains(t, fieldsOf(t, b.ApplyPayment(money.Zero)), "amount")
	assert.Contains(t, fieldsOf(t, b.ApplyPayment(open.Add(money.MustParse("1")))), "amount")
	require.NoError(t, b.ApplyPayment(money.MustParse("1000")))
	assert.Equal(t, domain.StatusPartiallyPaid, b.Status)
	require.NoError(t, b.ApplyPayment(b.OpenAmount()))
	assert.Equal(t, domain.StatusPaid, b.Status)
	requireCode(t, b.Update(in, refs), apperr.CodeInvalidStateTransition)

	// Ditolak → revisi / batal.
	b2, err := domain.NewSupplierBill(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionPost, domain.ActionCancel}, b2.Actions())
	requireCode(t, b2.ApplyPayment(money.MustParse("1")), apperr.CodeInvalidStateTransition)
	require.NoError(t, b2.Submit(now))
	require.NoError(t, b2.Reject())
	assert.Equal(t, []domain.Action{domain.ActionRevise, domain.ActionCancel}, b2.Actions())
	require.NoError(t, b2.Revise())
	assert.Equal(t, domain.MatchMatched, b2.MatchStatus)
	assert.Contains(t, fieldsOf(t, b2.Cancel(string(make([]rune, 401)))), "reason")
	require.NoError(t, b2.Cancel("Dobel"))
	assert.Equal(t, domain.StatusCancelled, b2.Status)

	// Tagihan dalam toleransi dengan sisa utang nol langsung Lunas.
	in.Lines[0].Qty = qty("100")
	none := domain.WithholdingNone
	in.WithholdingType = &none
	b3, err := domain.NewSupplierBill(in, refs, uuid.New())
	require.NoError(t, err)
	assert.True(t, b3.WithholdingRate.IsZero())
	require.NoError(t, b3.Finalize([]domain.BillAdvance{{Amount: b3.PayableBeforeAdvance()}}, uuid.New(), nil, now))
	assert.Equal(t, domain.StatusPaid, b3.Status)
	assert.Equal(t, b3.Input().Number, b3.Number)
}

func TestBillValidate(t *testing.T) {
	o, line := receivedOrder(t)
	base := func() domain.BillInput {
		return domain.BillInput{
			PurchaseOrderID: o.ID, Number: "INV-9", BillDate: today,
			Lines: []domain.BillLineInput{{ReceiptLineID: line.ID, Qty: qty("10"), UnitPrice: money.MustParse("1250")}},
		}
	}
	bad := domain.WithholdingType("pph99")
	cases := []struct {
		name  string
		mod   func(*domain.BillInput, *domain.BillRefs)
		field string
	}{
		{"tanpa nomor", func(in *domain.BillInput, _ *domain.BillRefs) { in.Number = " " }, "number"},
		{"nomor panjang", func(in *domain.BillInput, _ *domain.BillRefs) { in.Number = string(make([]rune, 61)) }, "number"},
		{"tanpa tanggal", func(in *domain.BillInput, _ *domain.BillRefs) { in.BillDate = domain.BillInput{}.BillDate }, ""},
		{"jatuh tempo mundur", func(in *domain.BillInput, _ *domain.BillRefs) { in.DueDate = ptr(today.AddDays(-1)) }, "dueDate"},
		{"PPh tidak dikenal", func(in *domain.BillInput, _ *domain.BillRefs) { in.WithholdingType = &bad }, "withholdingType"},
		{"tarif > 100", func(in *domain.BillInput, _ *domain.BillRefs) { in.WithholdingRate = ptr(qty("101")) }, "withholdingRate"},
		{"tarif 5 desimal", func(in *domain.BillInput, _ *domain.BillRefs) { in.WithholdingRate = ptr(qty("2.00001")) }, "withholdingRate"},
		{"catatan panjang", func(in *domain.BillInput, _ *domain.BillRefs) { in.Note = string(make([]rune, 1001)) }, "note"},
		{"tanpa baris", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines = nil }, "lines"},
		{"baris GR lain", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines[0].ReceiptLineID = uuid.New() }, "lines.0.receiptLineId"},
		{"GR batal", func(_ *domain.BillInput, r *domain.BillRefs) {
			x := r.ReceiptLines[line.ID]
			x.ReceiptStatus = domain.StatusCancelled
			r.ReceiptLines[line.ID] = x
		}, "lines.0.receiptLineId"},
		{"duplikat", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines = append(in.Lines, in.Lines[0]) }, "lines.1.receiptLineId"},
		{"qty melebihi GR", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines[0].Qty = qty("101") }, "lines.0.qty"},
		{"qty tertahan tagihan lain", func(_ *domain.BillInput, r *domain.BillRefs) {
			x := r.ReceiptLines[line.ID]
			x.PendingQty = qty("95")
			r.ReceiptLines[line.ID] = x
		}, "lines.0.qty"},
		{"harga negatif", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines[0].UnitPrice = money.MustParse("-1") }, "lines.0.unitPrice"},
		{"catatan baris", func(in *domain.BillInput, _ *domain.BillRefs) { in.Lines[0].Note = string(make([]rune, 501)) }, "lines.0.note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, r := base(), billRefs(o, line)
			tc.mod(&in, &r)
			err := in.Validate(r)
			if tc.field == "" {
				require.Error(t, err)
				return
			}
			assert.Contains(t, fieldsOf(t, err), tc.field)
		})
	}
	in := base()
	approved, _ := orderWithIDs(t, false)
	requireCode(t, in.Validate(domain.BillRefs{Order: approved}), apperr.CodeInvalidStateTransition)
	b, err := domain.NewSupplierBill(in, billRefs(o, line), uuid.New())
	require.NoError(t, err)
	in.PurchaseOrderID = uuid.New()
	assert.Contains(t, fieldsOf(t, b.Update(in, billRefs(o, line))), "purchaseOrderId")
}

func TestWithholding(t *testing.T) {
	assert.Equal(t, "2301", domain.WithholdingPPh21.Account())
	assert.Equal(t, "2302", domain.WithholdingPPh23.Account())
	assert.Equal(t, "2303", domain.WithholdingPPh42.Account())
	assert.Equal(t, "2.5", domain.WithholdingPPh21.DefaultRate().String())
	assert.Equal(t, "10", domain.WithholdingPPh42.DefaultRate().String())
	assert.True(t, domain.WithholdingNone.DefaultRate().IsZero())
	assert.Equal(t, "PPh 23", domain.WithholdingPPh23.Label())
	assert.False(t, domain.WithholdingType("x").Valid())
	// Harga PO nol: tagihan berharga dianggap selisih 100%.
	l := domain.BillLine{POUnitPrice: money.Zero, UnitPrice: money.MustParse("1"), Qty: qty("1")}
	assert.True(t, l.PriceMismatch())
	l.UnitPrice = money.Zero
	assert.False(t, l.PriceMismatch())
	assert.True(t, domain.BillLine{Qty: qty("1")}.QtyMismatch())
}

func returnRefs(o domain.PurchaseOrder, line domain.ReceiptLine, loc uuid.UUID) domain.ReturnRefs {
	return domain.ReturnRefs{
		Order: o, Locations: map[uuid.UUID]bool{loc: true},
		ReceiptLines: map[uuid.UUID]domain.ReturnReceiptLine{line.ID: {
			Line: line, ReceiptNumber: "GR/2026/10/00001", ReceiptStatus: domain.StatusPosted, PurchaseOrderID: o.ID,
		}},
	}
}

func TestReturnLifecycle(t *testing.T) {
	o, line := receivedOrder(t)
	loc := uuid.New()
	refs := returnRefs(o, line, loc)
	in := domain.ReturnInput{
		PurchaseOrderID: o.ID, BusinessDate: today, Reason: "Penyok",
		Lines: []domain.ReturnLineInput{{ReceiptLineID: line.ID, LocationID: loc, Qty: qty("10")}},
	}
	r, err := domain.NewPurchaseReturn(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.ReturnManual, r.Origin)
	assert.Equal(t, domain.StockQuarantine, r.Lines[0].StockStatus)
	assert.Equal(t, "Draft retur PO/2026/10/00009", r.Label())
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, r.Actions())
	requireCode(t, r.CanPost(), apperr.CodeInvalidStateTransition)
	require.NoError(t, r.Update(r.Input(), refs))

	require.NoError(t, r.Submit("RB/2026/10/00001", now))
	requireCode(t, r.Update(in, refs), apperr.CodeInvalidStateTransition)
	require.NoError(t, r.Reject())
	assert.Equal(t, []domain.Action{domain.ActionRevise}, r.Actions())
	require.NoError(t, r.Revise())
	require.NoError(t, r.Submit("RB/x", now))
	assert.Equal(t, "RB/2026/10/00001", r.Number, "nomor RB tetap saat diajukan ulang")
	require.NoError(t, r.Approve(now))
	assert.Equal(t, []domain.Action{domain.ActionPost, domain.ActionCancel}, r.Actions())

	// 6 dari 10 belum ditagih → 2102; sisanya 2101 (nota debit).
	r.ApplyCost(0, money.MustParse("12500"), qty("6"), "1301")
	assert.Equal(t, "7500.00", r.Lines[0].UnbilledAmount.String())
	assert.Equal(t, "5000.00", r.Lines[0].BilledAmount().String())
	assert.Equal(t, "1250.0000", r.Lines[0].UnitCost.StringFixed(4))
	r.ApplyCost(0, money.MustParse("12500"), qty("0"), "1301")
	assert.True(t, r.UnbilledAmount.IsZero())
	r.ApplyCost(0, money.MustParse("12500"), qty("50"), "1301")
	assert.Equal(t, "12500.00", r.UnbilledAmount.String())
	require.NoError(t, r.Post("", "DN/2026/10/00001", uuid.New(), now))
	assert.Equal(t, domain.StatusPosted, r.Status)
	assert.Equal(t, "RB/2026/10/00001", r.Label())
	assert.Empty(t, r.Actions())
	requireCode(t, r.Cancel(""), apperr.CodeInvalidStateTransition)

	// Asal QC: langsung diposting tanpa approval, tidak bisa diajukan.
	qc := in
	qc.Origin, qc.SourceType, qc.SourceID = domain.ReturnQCOutbound, "outbound_line", ptr(uuid.New())
	q, err := domain.NewPurchaseReturn(qc, refs, uuid.New())
	require.NoError(t, err)
	assert.True(t, q.Origin.IsQC())
	assert.Equal(t, "Reject QC Outbound", q.Origin.Label())
	assert.Equal(t, []domain.Action{domain.ActionPost, domain.ActionCancel}, q.Actions())
	requireCode(t, q.Submit("RB", now), apperr.CodeInvalidStateTransition)
	require.NoError(t, q.CanPost())
	require.NoError(t, q.Post("RB/2026/10/00002", "DN/2026/10/00002", uuid.New(), now))
	assert.Equal(t, "RB/2026/10/00002", q.Number)

	d, err := domain.NewPurchaseReturn(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Contains(t, fieldsOf(t, d.Cancel(string(make([]rune, 401)))), "reason")
	require.NoError(t, d.Cancel("batal"))
	in2 := in
	in2.PurchaseOrderID = uuid.New()
	d2, err := domain.NewPurchaseReturn(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Contains(t, fieldsOf(t, d2.Update(in2, refs)), "purchaseOrderId")
}

func TestReturnValidate(t *testing.T) {
	o, line := receivedOrder(t)
	loc := uuid.New()
	base := func() domain.ReturnInput {
		return domain.ReturnInput{
			PurchaseOrderID: o.ID, BusinessDate: today, Reason: "Rusak",
			Lines: []domain.ReturnLineInput{{ReceiptLineID: line.ID, LocationID: loc, Qty: qty("10")}},
		}
	}
	cases := []struct {
		name  string
		mod   func(*domain.ReturnInput, *domain.ReturnRefs)
		field string
	}{
		{"asal salah", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Origin = "x" }, "origin"},
		{"tanpa alasan", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Reason = "" }, "reason"},
		{"alasan panjang", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Reason = string(make([]rune, 401)) }, "reason"},
		{"catatan panjang", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Note = string(make([]rune, 1001)) }, "note"},
		{"QC tanpa dokumen", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Origin = domain.ReturnQCInbound }, "sourceId"},
		{"tanpa baris", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines = nil }, "lines"},
		{"baris GR lain", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].ReceiptLineID = uuid.New() }, "lines.0.receiptLineId"},
		{"GR batal", func(_ *domain.ReturnInput, r *domain.ReturnRefs) {
			x := r.ReceiptLines[line.ID]
			x.ReceiptStatus = domain.StatusCancelled
			r.ReceiptLines[line.ID] = x
		}, "lines.0.receiptLineId"},
		{"tanpa lokasi", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].LocationID = uuid.Nil }, "lines.0.locationId"},
		{"lokasi tidak ada", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].LocationID = uuid.New() }, "lines.0.locationId"},
		{"status stok salah", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].StockStatus = "reserved" }, "lines.0.stockStatus"},
		{"qty nol", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].Qty = decimal.Zero }, "lines.0.qty"},
		{"melebihi qty baik", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].Qty = qty("101") }, "lines.0.qty"},
		{"tertahan retur lain", func(_ *domain.ReturnInput, r *domain.ReturnRefs) {
			x := r.ReceiptLines[line.ID]
			x.PendingQty = qty("95")
			r.ReceiptLines[line.ID] = x
		}, "lines.0.qty"},
		{"catatan baris", func(in *domain.ReturnInput, _ *domain.ReturnRefs) { in.Lines[0].Note = string(make([]rune, 501)) }, "lines.0.note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, r := base(), returnRefs(o, line, loc)
			tc.mod(&in, &r)
			assert.Contains(t, fieldsOf(t, in.Validate(r)), tc.field)
		})
	}
	in := base()
	in.BusinessDate = today.AddDays(0)
	draft := o
	draft.Status = domain.StatusDraft
	requireCode(t, in.Validate(domain.ReturnRefs{Order: draft}), apperr.CodeInvalidStateTransition)
}

func TestPayableSettle(t *testing.T) {
	p := domain.Payable{OpenAmount: money.MustParse("1000")}
	assert.Contains(t, fieldsOf(t, p.Settle(money.Zero)), "amount")
	assert.Contains(t, fieldsOf(t, p.Settle(money.MustParse("1001"))), "amount")
	assert.Contains(t, fieldsOf(t, p.Settle(money.MustParse("-1"))), "amount")
	require.NoError(t, p.Settle(money.MustParse("400")))
	assert.Equal(t, "600.00", p.OpenAmount.String())
	dn := domain.Payable{OpenAmount: money.MustParse("-500")}
	assert.Contains(t, fieldsOf(t, dn.Settle(money.MustParse("1"))), "amount")
	require.NoError(t, dn.Settle(money.MustParse("-500")))
	assert.True(t, dn.OpenAmount.IsZero())
	assert.Contains(t, fieldsOf(t, dn.Settle(money.MustParse("-1"))), "amount")
}

func TestStatusLabelsDocuments(t *testing.T) {
	assert.Equal(t, "Menunggu Pembayaran", domain.StatusAwaitingPayment.Label())
	assert.Equal(t, "Lunas", domain.StatusPaid.Label())
	assert.True(t, domain.RejectAbsorbed.Valid())
	assert.False(t, domain.ReturnOrigin("x").Valid())
	requireCode(t, domain.BillableOrder(domain.PurchaseOrder{Status: domain.StatusApproved}), apperr.CodeInvalidStateTransition)
}
