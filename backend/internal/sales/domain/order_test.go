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

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func amt(s string) money.Amount { return money.MustParse(s) }

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func code(err error) apperr.Code { return apperr.As(err).Code }

var (
	today    = clock.NewBusinessDate(2026, 11, 2)
	now      = time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	product1 = uuid.New()
	product2 = uuid.New()
	bundle   = uuid.New()
	location = uuid.New()
	channel  = uuid.New()
	sub      = uuid.New()
	partner  = uuid.New()
	customer = uuid.New()
)

func price(p string) *domain.LinePrice {
	return &domain.LinePrice{UnitPrice: amt(p), Rule: "base_price"}
}

func refs(t domain.OrderType) domain.OrderRefs {
	partyType := domain.PartnerTypeModernTrade
	switch t {
	case domain.OrderB2B:
		partyType = domain.PartnerTypeB2B
	case domain.OrderEmployee:
		partyType = domain.CustomerTypeEmployee
	case domain.OrderSocialCommerce:
		partyType = "retail"
	case domain.OrderModernTrade, domain.OrderMarketplace:
	}
	locType := domain.LocationFinishedGoods
	if t == domain.OrderMarketplace {
		locType = domain.LocationEcommerce
	}
	return domain.OrderRefs{
		ChannelCode: t.ChannelCode(), SubChannelValid: true, EmployeeEligible: true,
		Party: &domain.PartyInfo{ID: partner, Name: "Mitra A", Code: "MA", Type: partyType, Address: "Jl. A"},
		Products: map[uuid.UUID]domain.ProductInfo{
			product1: {ID: product1, SKU: "P1", Type: domain.ProductFinishedGood, IsActive: true},
			product2: {ID: product2, SKU: "P2", Type: domain.ProductFinishedGood, IsActive: true},
			bundle:   {ID: bundle, SKU: "B1", Type: domain.ProductFinishedGood, IsActive: true, IsBundle: true},
		},
		Location: &domain.LocationInfo{ID: location, Type: locType, IsActive: true},
		Prices:   []*domain.LinePrice{price("100000"), price("220000")},
	}
}

func input(t domain.OrderType) domain.OrderInput {
	in := domain.OrderInput{
		Type: t, ChannelID: channel, SubChannelID: sub, BusinessDate: today, SourceLocationID: location,
		Lines: []domain.LineInput{
			{ProductID: product1, Qty: dec("10"), Discount: domain.Discount{Kind: domain.DiscountPercent, Value: dec("10")}},
			{ProductID: bundle, Qty: dec("2")},
		},
		DocumentDiscount: domain.Discount{Kind: domain.DiscountAmount, Value: dec("40000")},
		Shipping:         amt("15000"),
	}
	switch t {
	case domain.OrderModernTrade, domain.OrderB2B:
		in.PartnerID = &partner
	case domain.OrderSocialCommerce, domain.OrderEmployee:
		in.CustomerID = &customer
	case domain.OrderMarketplace:
		in.ExternalOrderID = "SHP-1"
		in.Shipping = money.Zero
	}
	return in
}

func TestNewOrderComputesTotals(t *testing.T) {
	o, err := domain.NewOrder(input(domain.OrderModernTrade), refs(domain.OrderModernTrade), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, o.Status)
	assert.Equal(t, "1440000.00", o.Gross.String())
	assert.Equal(t, "100000.00", o.LineDiscount.String())
	assert.Equal(t, "1340000.00", o.Subtotal().String())
	assert.Equal(t, "40000.00", o.DocumentDiscountAmount.String())
	assert.Equal(t, "140000.00", o.TotalDiscount().String())
	assert.Equal(t, "1315000.00", o.Total.String(), "NON-PKP: total = bruto − diskon + ongkir, tanpa PPN")
	assert.Equal(t, "Mitra A", o.PartyName)
	assert.Equal(t, "Jl. A", o.ShipToAddress)
	assert.True(t, o.Lines[1].IsBundle)
	assert.Equal(t, 0, o.PaymentTermDays)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionConfirm, domain.ActionCancel}, o.Actions())
	pt, id := o.PartyType()
	assert.Equal(t, domain.PartyPartner, pt)
	assert.Equal(t, partner, *id)
	assert.Equal(t, "Draft Sales Order", o.Label())
}

func TestOrderValidation(t *testing.T) {
	tests := []struct {
		name  string
		typ   domain.OrderType
		mut   func(in *domain.OrderInput, r *domain.OrderRefs)
		field string
	}{
		{"jenis tidak dikenal", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Type = "x" }, "orderType"},
		{"channel salah", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.ChannelCode = "b2b" }, "channelId"},
		{"tanpa channel", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.ChannelID = uuid.Nil }, "channelId"},
		{"sub-channel bukan milik channel", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.SubChannelValid = false }, "subChannelId"},
		{"tanpa sub-channel", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.SubChannelID = uuid.Nil }, "subChannelId"},
		{"tanpa tanggal", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.BusinessDate = clock.BusinessDate{} }, "businessDate"},
		{"tanpa lokasi", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.SourceLocationID = uuid.Nil }, "sourceLocationId"},
		{"lokasi tidak ada", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Location = nil }, "sourceLocationId"},
		{"lokasi nonaktif", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Location.IsActive = false }, "sourceLocationId"},
		{"MT dari E-commerce Stock", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Location.Type = domain.LocationEcommerce }, "sourceLocationId"},
		{"marketplace dari FG", domain.OrderMarketplace, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Location.Type = domain.LocationFinishedGoods }, "sourceLocationId"},
		{"cara bayar tidak dikenal", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PaymentMode = "x" }, "paymentMode"},
		{"tempo WA", domain.OrderSocialCommerce, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PaymentMode = domain.PaymentCredit }, "paymentMode"},
		{"termin terlalu panjang", domain.OrderB2B, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PaymentTermDays = 400 }, "paymentTermDays"},
		{"marketplace tanpa nomor order", domain.OrderMarketplace, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.ExternalOrderID = "" }, "externalOrderId"},
		{"nomor order terlalu panjang", domain.OrderMarketplace, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.ExternalOrderID = string(make([]byte, 81))
		}, "externalOrderId"},
		{"marketplace dengan ongkir", domain.OrderMarketplace, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Shipping = amt("1000") }, "shippingAmount"},
		{"nomor eksternal bukan marketplace", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.ExternalOrderID = "X" }, "externalOrderId"},
		{"ongkir negatif", domain.OrderSocialCommerce, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Shipping = amt("-1") }, "shippingAmount"},
		{"PO klien terlalu panjang", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.ClientPONumber = string(make([]rune, 201))
		}, "clientPoNumber"},
		{"catatan terlalu panjang", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Note = string(make([]rune, 1001))
		}, "note"},
		{"pelanggan dan mitra", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.CustomerID = &customer }, "customerId"},
		{"MT tanpa mitra", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PartnerID = nil }, "partnerId"},
		{"MT mitra tidak ada", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party = nil }, "partnerId"},
		{"MT mitra B2B", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party.Type = domain.PartnerTypeB2B }, "partnerId"},
		{"B2B mitra MT", domain.OrderB2B, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party.Type = domain.PartnerTypeModernTrade }, "partnerId"},
		{"B2B pelanggan retail", domain.OrderB2B, func(in *domain.OrderInput, r *domain.OrderRefs) {
			in.PartnerID, in.CustomerID, r.Party.Type = nil, &customer, "retail"
		}, "customerId"},
		{"B2B tanpa pihak", domain.OrderB2B, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PartnerID = nil }, "partnerId"},
		{"WA tanpa pelanggan", domain.OrderSocialCommerce, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.CustomerID = nil }, "customerId"},
		{"WA pelanggan terhapus", domain.OrderSocialCommerce, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party.Deleted = true }, "customerId"},
		{"karyawan tanpa pelanggan", domain.OrderEmployee, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.CustomerID = nil }, "customerId"},
		{"karyawan pelanggan retail", domain.OrderEmployee, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party.Type = "retail" }, "customerId"},
		{"karyawan resign", domain.OrderEmployee, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.EmployeeEligible = false }, "customerId"},
		{"karyawan tidak ada", domain.OrderEmployee, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Party = nil }, "customerId"},
		{"marketplace dengan pelanggan", domain.OrderMarketplace, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.CustomerID = &customer }, "customerId"},
		{"tanpa baris", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines = nil }, "lines"},
		{"terlalu banyak baris", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Lines = make([]domain.LineInput, domain.MaxLines+1)
		}, "lines"},
		{"produk kosong", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].ProductID = uuid.Nil }, "lines.0.productId"},
		{"produk tidak ada", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].ProductID = uuid.New() }, "lines.0.productId"},
		{"produk nonaktif", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) {
			p := r.Products[product1]
			p.IsActive = false
			r.Products[product1] = p
		}, "lines.0.productId"},
		{"bukan produk jadi", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) {
			p := r.Products[product1]
			p.Type = "packaging"
			r.Products[product1] = p
		}, "lines.0.productId"},
		{"produk ganda", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[1].ProductID = product1 }, "lines.1.productId"},
		{"qty nol", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].Qty = decimal.Zero }, "lines.0.qty"},
		{"qty 4 desimal", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].Qty = dec("1.0001") }, "lines.0.qty"},
		{"harga diketik MT", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			p := amt("1")
			in.Lines[0].UnitPrice = &p
		}, "lines.0.unitPrice"},
		{"harga marketplace negatif", domain.OrderMarketplace, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			p := amt("-1")
			in.Lines[0].UnitPrice = &p
		}, "lines.0.unitPrice"},
		{"catatan baris panjang", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Lines[0].Note = string(make([]rune, 201))
		}, "lines.0.note"},
		{"tanpa harga", domain.OrderModernTrade, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Prices[1] = nil }, "lines.1.productId"},
		{"diskon baris > 100%", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Lines[0].Discount = domain.Discount{Kind: domain.DiscountPercent, Value: dec("101")}
		}, "lines.0.discount"},
		{"diskon dokumen melebihi subtotal", domain.OrderModernTrade, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.DocumentDiscount = domain.Discount{Kind: domain.DiscountAmount, Value: dec("9999999")}
		}, "documentDiscount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, r := input(tt.typ), refs(tt.typ)
			tt.mut(&in, &r)
			_, err := domain.NewOrder(in, r, uuid.New())
			assert.Contains(t, fields(t, err), tt.field)
		})
	}
}

func TestDiscountApply(t *testing.T) {
	base := amt("200000")
	tests := []struct {
		d    domain.Discount
		want string
		msg  bool
	}{
		{domain.Discount{}, "0.00", false},
		{domain.Discount{Kind: domain.DiscountPercent, Value: dec("12.5")}, "25000.00", false},
		{domain.Discount{Kind: domain.DiscountAmount, Value: dec("1500.5")}, "1500.50", false},
		{domain.Discount{Kind: domain.DiscountAmount, Value: dec("-1")}, "0.00", true},
		{domain.Discount{Kind: domain.DiscountPercent, Value: dec("1.00001")}, "0.00", true},
		{domain.Discount{Kind: domain.DiscountAmount, Value: dec("1.001")}, "0.00", true},
		{domain.Discount{Kind: domain.DiscountAmount, Value: dec("300000")}, "0.00", true},
		{domain.Discount{Kind: "x", Value: dec("1")}, "0.00", true},
	}
	for _, tt := range tests {
		got, msg := tt.d.Apply(base)
		assert.Equal(t, tt.want, got.String())
		assert.Equal(t, tt.msg, msg != "", msg)
	}
	assert.True(t, domain.Discount{}.IsZero())
}

func TestOrderTypeRules(t *testing.T) {
	for _, ot := range domain.OrderTypes {
		assert.True(t, ot.Valid())
		assert.NotEmpty(t, ot.Label())
		assert.NotEmpty(t, ot.LocationTypes())
	}
	assert.Equal(t, "x", domain.OrderType("x").Label())
	assert.True(t, domain.OrderMarketplace.RecognizedAtHandover())
	assert.False(t, domain.OrderModernTrade.RecognizedAtHandover())
	assert.True(t, domain.OrderB2B.AllowsCredit())
	assert.False(t, domain.OrderEmployee.AllowsCredit())
	assert.Equal(t, "Lunas", domain.StatusPaid.Label())
	assert.Equal(t, "x", domain.Status("x").Label())
	assert.Empty(t, domain.ValidQty(dec("1.5")))
	assert.NotEmpty(t, domain.ValidQty(dec("1234567890123456")))
}

func confirmed(t *testing.T, typ domain.OrderType, requiresApproval bool) domain.Order {
	t.Helper()
	o, err := domain.NewOrder(input(typ), refs(typ), uuid.New())
	require.NoError(t, err)
	o.ID = uuid.New()
	for i := range o.Lines {
		o.Lines[i].ID = uuid.New()
	}
	require.NoError(t, o.Confirm("SO/2026/11/00001", requiresApproval, now))
	return o
}

func TestOrderLifecycle(t *testing.T) {
	o := confirmed(t, domain.OrderModernTrade, false)
	assert.Equal(t, domain.StatusAwaitingPayment, o.Status)
	assert.Equal(t, 1, o.AllocationRound)
	assert.Equal(t, "SO/2026/11/00001", o.Label())
	assert.Equal(t, []domain.Action{domain.ActionUploadProof, domain.ActionCancel}, o.Actions())
	require.NoError(t, o.AttachPaymentProof(uuid.New(), now))
	require.NoError(t, o.MarkPaid())
	assert.Equal(t, domain.StatusInProgress, o.Status)
	assert.Equal(t, code(o.AttachPaymentProof(uuid.New(), now)), apperr.CodeInvalidStateTransition)
	require.Error(t, o.Confirm("X", false, now), "konfirmasi hanya dari draft")

	// Edit hanya draft.
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(o.Update(input(domain.OrderModernTrade), refs(domain.OrderModernTrade))))

	credit := input(domain.OrderB2B)
	credit.PaymentMode, credit.PaymentTermDays = domain.PaymentCredit, 30
	c, err := domain.NewOrder(credit, refs(domain.OrderB2B), uuid.New())
	require.NoError(t, err)
	require.NoError(t, c.Confirm("SO/2", false, now))
	assert.Equal(t, domain.StatusInProgress, c.Status, "tempo langsung Diproses")
	assert.Equal(t, 30, c.PaymentTermDays)

	mp := confirmed(t, domain.OrderMarketplace, false)
	assert.Equal(t, domain.StatusInProgress, mp.Status)

	p := confirmed(t, domain.OrderSocialCommerce, true)
	assert.Equal(t, domain.StatusPendingApproval, p.Status)
	assert.Equal(t, []domain.Action{domain.ActionCancel}, p.Actions())
	require.Error(t, p.MarkPaid())
	require.NoError(t, p.ApproveDiscount(now))
	assert.Equal(t, domain.StatusAwaitingPayment, p.Status)
	require.Error(t, p.ApproveDiscount(now))
	require.Error(t, p.RejectDiscount())
	require.NoError(t, p.Expire())
	assert.Equal(t, domain.StatusExpired, p.Status)
	assert.Empty(t, p.Actions())

	r := confirmed(t, domain.OrderSocialCommerce, true)
	require.NoError(t, r.RejectDiscount())
	assert.Equal(t, domain.StatusDraft, r.Status)
	require.NoError(t, r.Update(input(domain.OrderSocialCommerce), refs(domain.OrderSocialCommerce)))

	// Total nol tidak bisa dikonfirmasi (kecuali marketplace).
	zero := input(domain.OrderSocialCommerce)
	zr := refs(domain.OrderSocialCommerce)
	zr.Prices = []*domain.LinePrice{price("0"), price("0")}
	zero.Lines[0].Discount, zero.DocumentDiscount, zero.Shipping = domain.Discount{}, domain.Discount{}, money.Zero
	z, err := domain.NewOrder(zero, zr, uuid.New())
	require.NoError(t, err)
	assert.Contains(t, fields(t, z.Confirm("SO", false, now)), "lines")
}

func TestOrderCancel(t *testing.T) {
	d, err := domain.NewOrder(input(domain.OrderModernTrade), refs(domain.OrderModernTrade), uuid.New())
	require.NoError(t, err)
	require.NoError(t, d.Cancel(""), "draft batal tanpa alasan")
	assert.Equal(t, domain.StatusCancelled, d.Status)

	o := confirmed(t, domain.OrderModernTrade, false)
	assert.Contains(t, fields(t, o.Cancel(" ")), "reason")
	assert.Contains(t, fields(t, o.Cancel(string(make([]rune, 401)))), "reason")
	require.NoError(t, o.Cancel("Klien batal"))
	assert.Equal(t, "Klien batal", o.CancelReason)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(o.Cancel("lagi")))

	s := confirmed(t, domain.OrderMarketplace, false)
	s.Allocations = []domain.Allocation{{ID: uuid.New(), Qty: dec("2"), ShippedQty: dec("1")}}
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(s.Cancel("x")), "sudah ada pengiriman")
}

func TestPlanAllocationsExplodesBundle(t *testing.T) {
	o := confirmed(t, domain.OrderModernTrade, false)
	n := 0
	allocs, err := o.PlanAllocations(map[uuid.UUID][]domain.ComponentQty{
		bundle: {{ProductID: product1, Qty: dec("2")}, {ProductID: product2, Qty: dec("1")}},
	}, func() uuid.UUID { n++; return uuid.New() })
	require.NoError(t, err)
	require.Len(t, allocs, 3)
	assert.Equal(t, 3, n)
	assert.Equal(t, product1, allocs[0].ProductID)
	assert.Equal(t, "10", allocs[0].Qty.String())
	assert.Equal(t, product1, allocs[1].ProductID)
	assert.Equal(t, "4", allocs[1].Qty.String(), "2 bundel × 2 komponen")
	assert.Equal(t, "2", allocs[2].Qty.String())
	assert.Equal(t, location, allocs[2].LocationID)

	_, err = o.PlanAllocations(map[uuid.UUID][]domain.ComponentQty{}, uuid.New)
	assert.Contains(t, fields(t, err), "lines.1.productId", "bundel tanpa komponen")
}

func TestShipmentPlanning(t *testing.T) {
	o := confirmed(t, domain.OrderModernTrade, false)
	_, err := o.PlanShipment(nil)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err), "belum dibayar")
	require.NoError(t, o.MarkPaid())
	a1, a2 := uuid.New(), uuid.New()
	o.Allocations = []domain.Allocation{
		{ID: a1, Qty: dec("10"), ShippedQty: decimal.Zero},
		{ID: a2, Qty: dec("4"), ShippedQty: decimal.Zero},
	}
	assert.False(t, o.FullyShipped())
	_, err = o.PlanShipment([]domain.ShipmentQty{{AllocationID: uuid.New(), Qty: dec("1")}})
	assert.Contains(t, fields(t, err), "lines.0.allocationId")
	_, err = o.PlanShipment([]domain.ShipmentQty{{AllocationID: a1, Qty: dec("0")}})
	assert.Contains(t, fields(t, err), "lines.0.qty")
	_, err = o.PlanShipment([]domain.ShipmentQty{{AllocationID: a1, Qty: dec("6")}, {AllocationID: a1, Qty: dec("6")}})
	assert.Contains(t, fields(t, err), "lines.1.qty", "melebihi sisa")

	lines, err := o.PlanShipment([]domain.ShipmentQty{{AllocationID: a1, Qty: dec("6")}})
	require.NoError(t, err)
	require.NoError(t, o.ApplyShipment(lines, now))
	assert.Equal(t, domain.StatusInProgress, o.Status, "kirim sebagian")
	assert.True(t, o.HasShipment())
	assert.Nil(t, o.Actions(), "sudah dikirim tidak bisa dibatalkan")

	rest, err := o.PlanShipment(nil)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	assert.Equal(t, "4", rest[0].Qty.String())
	require.NoError(t, o.ApplyShipment(rest, now))
	assert.Equal(t, domain.StatusInTransit, o.Status)
	assert.True(t, o.FullyShipped())
	require.NoError(t, o.Deliver(now))
	assert.Equal(t, domain.StatusCompleted, o.Status)
	require.Error(t, o.Deliver(now))

	mp := confirmed(t, domain.OrderMarketplace, false)
	mp.Allocations = []domain.Allocation{{ID: a1, Qty: dec("2")}, {ID: a2, Qty: dec("1")}}
	_, err = mp.PlanShipment([]domain.ShipmentQty{{AllocationID: a1, Qty: dec("2")}})
	assert.Contains(t, fields(t, err), "lines", "marketplace diserahkan sekaligus")
	mp.Allocations = []domain.Allocation{{ID: a1, Qty: dec("2"), ShippedQty: dec("2")}}
	_, err = mp.PlanShipment(nil)
	assert.Contains(t, fields(t, err), "lines", "tidak ada sisa")
}
