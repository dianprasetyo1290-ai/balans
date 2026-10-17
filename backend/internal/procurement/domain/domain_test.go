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
	"balans/internal/procurement/domain"
)

var (
	now   = time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	today = clock.NewBusinessDate(2026, 10, 20)
)

func qty(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr[T any](v T) *T { return &v }

type catalogFixture struct {
	fg, pkg, gm, bundle, inactive, deleted uuid.UUID
	products                               map[uuid.UUID]domain.ProductInfo
}

func newCatalog() catalogFixture {
	c := catalogFixture{
		fg: uuid.New(), pkg: uuid.New(), gm: uuid.New(), bundle: uuid.New(), inactive: uuid.New(), deleted: uuid.New(),
	}
	c.products = map[uuid.UUID]domain.ProductInfo{
		c.fg:       {ID: c.fg, Type: domain.ProductFinishedGood, IsActive: true},
		c.pkg:      {ID: c.pkg, Type: domain.ProductPackaging, IsActive: true},
		c.gm:       {ID: c.gm, Type: domain.ProductReprocessMaterial, IsActive: true},
		c.bundle:   {ID: c.bundle, Type: domain.ProductFinishedGood, IsActive: true, IsBundle: true},
		c.inactive: {ID: c.inactive, Type: domain.ProductPackaging},
		c.deleted:  {ID: c.deleted, Type: domain.ProductPackaging, IsActive: true, Deleted: true},
	}
	return c
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var e *apperr.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, apperr.CodeValidationFailed, e.Code)
	return e.Fields
}

func requireCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	require.Error(t, err)
	assert.True(t, apperr.HasCode(err, code), "want %s, got %v", code, err)
}

// --- Purchase Request ------------------------------------------------------------------------

func validRequest(c catalogFixture) domain.RequestInput {
	return domain.RequestInput{
		DivisionID: uuid.New(), BusinessDate: today, NeededDate: ptr(today.AddDays(7)), Note: " Stok tube ",
		Lines: []domain.RequestLineInput{
			{ProductID: &c.pkg, Qty: qty("1000"), EstimatedPrice: money.MustParse("1250")},
			{Description: "Lakban coklat", Qty: qty("12.5"), EstimatedPrice: money.MustParse("10000.10")},
		},
	}
}

func TestRequestValidate(t *testing.T) {
	c := newCatalog()
	cases := []struct {
		name  string
		edit  func(*domain.RequestInput)
		field string
	}{
		{"divisi kosong", func(in *domain.RequestInput) { in.DivisionID = uuid.Nil }, "divisionId"},
		{"tanggal kosong", func(in *domain.RequestInput) { in.BusinessDate = clock.BusinessDate{} }, "businessDate"},
		{"dibutuhkan sebelum tanggal", func(in *domain.RequestInput) { in.NeededDate = ptr(today.AddDays(-1)) }, "neededDate"},
		{"catatan panjang", func(in *domain.RequestInput) { in.Note = string(make([]rune, 1001)) }, "note"},
		{"tanpa baris", func(in *domain.RequestInput) { in.Lines = nil }, "lines"},
		{"terlalu banyak baris", func(in *domain.RequestInput) {
			in.Lines = make([]domain.RequestLineInput, domain.MaxLines+1)
		}, "lines"},
		{"produk dan uraian kosong", func(in *domain.RequestInput) { in.Lines[1].Description = " " }, "lines.1.productId"},
		{"produk bundel", func(in *domain.RequestInput) { in.Lines[0].ProductID = &c.bundle }, "lines.0.productId"},
		{"produk nonaktif", func(in *domain.RequestInput) { in.Lines[0].ProductID = &c.inactive }, "lines.0.productId"},
		{"produk dihapus", func(in *domain.RequestInput) { in.Lines[0].ProductID = &c.deleted }, "lines.0.productId"},
		{"produk tidak ada", func(in *domain.RequestInput) { in.Lines[0].ProductID = ptr(uuid.New()) }, "lines.0.productId"},
		{"qty nol", func(in *domain.RequestInput) { in.Lines[0].Qty = decimal.Zero }, "lines.0.qty"},
		{"qty 4 desimal", func(in *domain.RequestInput) { in.Lines[0].Qty = qty("1.0001") }, "lines.0.qty"},
		{"qty terlalu besar", func(in *domain.RequestInput) { in.Lines[0].Qty = qty("1234567890123456") }, "lines.0.qty"},
		{"harga negatif", func(in *domain.RequestInput) { in.Lines[0].EstimatedPrice = money.MustParse("-1") }, "lines.0.estimatedPrice"},
		{"harga 3 desimal", func(in *domain.RequestInput) { in.Lines[0].EstimatedPrice = money.MustParse("1.005") }, "lines.0.estimatedPrice"},
		{"uraian panjang", func(in *domain.RequestInput) { in.Lines[1].Description = string(make([]rune, 501)) }, "lines.1.description"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validRequest(c)
			tc.edit(&in)
			assert.Contains(t, fieldsOf(t, in.Validate(c.products)), tc.field)
		})
	}
	require.NoError(t, validRequest(c).Validate(c.products))
}

func TestRequestLifecycle(t *testing.T) {
	c := newCatalog()
	requester := uuid.New()
	r, err := domain.NewPurchaseRequest(validRequest(c), c.products, requester)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, r.Status)
	assert.Equal(t, "Stok tube", r.Note)
	// 1000 × 1.250 + 12,5 × 10.000,10 = 1.250.000 + 125.001,25
	assert.Equal(t, "1375001.25", r.EstimatedTotal.String())
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionSubmit, domain.ActionCancel}, r.Actions())

	in := validRequest(c)
	in.Lines = in.Lines[:1]
	require.NoError(t, r.Update(in, c.products))
	assert.Len(t, r.Lines, 1)
	assert.True(t, r.NeedsNumber())

	require.NoError(t, r.Submit("PR/2026/10/00001", now))
	assert.Equal(t, "PR/2026/10/00001", r.Number)
	assert.Empty(t, r.Actions())
	requireCode(t, r.Update(in, c.products), apperr.CodeInvalidStateTransition)
	requireCode(t, r.Cancel(), apperr.CodeInvalidStateTransition)

	require.NoError(t, r.Reject(now))
	assert.Equal(t, []domain.Action{domain.ActionRevise}, r.Actions())
	require.NoError(t, r.Revise())
	require.NoError(t, r.Submit("PR/2026/10/00099", now))
	assert.Equal(t, "PR/2026/10/00001", r.Number, "nomor PR yang direvisi dipertahankan")
	require.NoError(t, r.Approve(now))
	assert.Equal(t, []domain.Action{domain.ActionCreateOrder, domain.ActionClose}, r.Actions())
	require.NoError(t, r.CanOrder())

	// Fulfillment: sebagian → tetap Disetujui; penuh → Selesai; PO batal → kembali Disetujui.
	r.Lines[0].ID = uuid.New()
	line := r.Lines[0].ID
	l, ok := r.Line(line)
	require.True(t, ok)
	assert.Equal(t, "1000", l.RemainingQty().String())
	_, ok = r.Line(uuid.New())
	assert.False(t, ok)
	require.NoError(t, r.ApplyFulfillment(map[uuid.UUID]decimal.Decimal{line: qty("400")}))
	assert.Equal(t, domain.StatusApproved, r.Status)
	require.NoError(t, r.ApplyFulfillment(map[uuid.UUID]decimal.Decimal{line: qty("1000")}))
	assert.Equal(t, domain.StatusCompleted, r.Status)
	requireCode(t, r.CanOrder(), apperr.CodeInvalidStateTransition)
	require.NoError(t, r.ApplyFulfillment(map[uuid.UUID]decimal.Decimal{}))
	assert.Equal(t, domain.StatusApproved, r.Status)
	requireCode(t, r.ApplyFulfillment(map[uuid.UUID]decimal.Decimal{line: qty("1001")}), apperr.CodeInvalidStateTransition)

	// Tutup wajib alasan.
	assert.Contains(t, fieldsOf(t, r.Close(" ")), "reason")
	assert.Contains(t, fieldsOf(t, r.Close(string(make([]rune, 401)))), "reason")
	require.NoError(t, r.Close("Anggaran dipotong"))
	assert.Equal(t, domain.StatusClosed, r.Status)
	assert.Equal(t, "Ditutup", r.Status.Label())
	assert.Empty(t, r.Actions())
	// PR ditutup tetap ditutup walau PO terkait dihitung ulang.
	require.NoError(t, r.ApplyFulfillment(map[uuid.UUID]decimal.Decimal{line: qty("1000")}))
	assert.Equal(t, domain.StatusClosed, r.Status)
}

func TestRequestCancelDraft(t *testing.T) {
	c := newCatalog()
	r, err := domain.NewPurchaseRequest(validRequest(c), c.products, uuid.New())
	require.NoError(t, err)
	require.NoError(t, r.Cancel())
	assert.Equal(t, domain.StatusCancelled, r.Status)
	requireCode(t, r.Submit("PR/1", now), apperr.CodeInvalidStateTransition)
	_, err = domain.NewPurchaseRequest(domain.RequestInput{}, c.products, uuid.New())
	require.Error(t, err)
	assert.Equal(t, "status_x", domain.Status("status_x").Label())
}

// --- Purchase Order --------------------------------------------------------------------------

func supplier(toll bool) *domain.SupplierInfo {
	return &domain.SupplierInfo{ID: uuid.New(), Code: "KEMAS", Name: "PT Kemas Jaya", IsTollManufacturer: toll, PaymentTermDays: 30}
}

func packagingOrder(c catalogFixture) (domain.OrderInput, domain.OrderRefs) {
	s := supplier(false)
	in := domain.OrderInput{
		Type: domain.OrderPackaging, SupplierID: s.ID, DivisionID: uuid.New(), BusinessDate: today,
		ExpectedDate: today.AddDays(1), Note: "Kirim pagi",
		Lines: []domain.OrderLineInput{
			{ProductID: &c.pkg, Qty: qty("10000"), UnitPrice: money.MustParse("1250")},
			{ProductID: &c.pkg, Qty: qty("3"), UnitPrice: money.MustParse("0.33")},
		},
	}
	return in, domain.OrderRefs{Supplier: s, Products: c.products}
}

func tollOrder(c catalogFixture, t domain.OrderType) (domain.OrderInput, domain.OrderRefs) {
	s := supplier(true)
	material := c.pkg
	var source *uuid.UUID
	if t == domain.OrderReprocess {
		material, source = c.gm, ptr(uuid.New())
	}
	in := domain.OrderInput{
		Type: t, SupplierID: s.ID, DivisionID: uuid.New(), BusinessDate: today, ExpectedDate: today.AddDays(14),
		MaterialSourceLocationID: source,
		Lines:                    []domain.OrderLineInput{{ProductID: &c.fg, Qty: qty("5000"), UnitPrice: money.MustParse("3500")}},
		Materials:                []domain.OrderMaterialInput{{ProductID: material, PlannedQty: qty("5100")}},
	}
	return in, domain.OrderRefs{Supplier: s, TollLocationID: ptr(uuid.New()), Products: c.products}
}

func TestOrderTypes(t *testing.T) {
	assert.True(t, domain.OrderTollManufacturing.IsToll())
	assert.True(t, domain.OrderReprocess.IsToll())
	assert.False(t, domain.OrderPackaging.IsToll())
	assert.Equal(t, "Jasa Maklon", domain.OrderTollManufacturing.Label())
	assert.Equal(t, "x", domain.OrderType("x").Label())
	assert.False(t, domain.OrderType("x").Valid())
	assert.True(t, domain.OrderOther.Valid())
}

func TestOrderValidate(t *testing.T) {
	c := newCatalog()
	cases := []struct {
		name  string
		build func() (domain.OrderInput, domain.OrderRefs)
		edit  func(*domain.OrderInput, *domain.OrderRefs)
		field string
	}{
		{"jenis tidak dikenal", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Type = "x" }, "orderType"},
		{"pemasok kosong", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.SupplierID = uuid.Nil }, "supplierId"},
		{"pemasok tidak ada", nil, func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Supplier = nil }, "supplierId"},
		{"divisi kosong", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.DivisionID = uuid.Nil }, "divisionId"},
		{"tanggal kosong", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.BusinessDate = clock.BusinessDate{} }, "businessDate"},
		{"tanggal kirim kosong", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.ExpectedDate = clock.BusinessDate{} }, "expectedDate"},
		{"tanggal kirim mundur", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.ExpectedDate = today.AddDays(-1) }, "expectedDate"},
		{"termin negatif", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.PaymentTermDays = ptr(-1) }, "paymentTermDays"},
		{"catatan panjang", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Note = string(make([]rune, 1001)) }, "note"},
		{"lokasi bahan di PO packaging", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.MaterialSourceLocationID = ptr(uuid.New())
		}, "materialSourceLocationId"},
		{"bahan di PO packaging", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Materials = []domain.OrderMaterialInput{{ProductID: c.pkg, PlannedQty: qty("1")}}
		}, "materials"},
		{"tanpa baris", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines = nil }, "lines"},
		{"terlalu banyak baris", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Lines = make([]domain.OrderLineInput, domain.MaxLines+1)
		}, "lines"},
		{"produk wajib", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].ProductID = nil }, "lines.0.productId"},
		{"jenis produk salah", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].ProductID = &c.fg }, "lines.0.productId"},
		{"qty negatif", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].Qty = qty("-1") }, "lines.0.qty"},
		{"harga negatif", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].UnitPrice = money.MustParse("-5") }, "lines.0.unitPrice"},
		{"uraian panjang", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].Note = string(make([]rune, 501)) }, "lines.0.description"},
		{"total nol", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Lines[0].UnitPrice, in.Lines[1].UnitPrice = money.Zero, money.Zero
		}, "lines"},
		{"baris PR ganda", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			id := uuid.New()
			in.Lines[0].PurchaseRequestLineID, in.Lines[1].PurchaseRequestLineID = &id, &id
		}, "lines.1.purchaseRequestLineId"},
		{"lainnya tanpa uraian", nil, func(in *domain.OrderInput, _ *domain.OrderRefs) {
			in.Type, in.Lines[0].ProductID = domain.OrderOther, nil
		}, "lines.0.productId"},
		{"maklon ke pemasok biasa", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(_ *domain.OrderInput, r *domain.OrderRefs) { r.Supplier.IsTollManufacturer = false }, "supplierId"},
		{"maklon tanpa lokasi", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(_ *domain.OrderInput, r *domain.OrderRefs) { r.TollLocationID = nil }, "supplierId"},
		{"maklon baris bukan FG", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Lines[0].ProductID = &c.pkg }, "lines.0.productId"},
		{"maklon tanpa packaging", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials = nil }, "materials"},
		{"maklon bahan GM", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials[0].ProductID = c.gm }, "materials.0.productId"},
		{"maklon bahan kosong", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials[0].ProductID = uuid.Nil }, "materials.0.productId"},
		{"maklon bahan ganda", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials = append(in.Materials, in.Materials[0]) }, "materials.1.productId"},
		{"maklon qty bahan nol", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials[0].PlannedQty = decimal.Zero }, "materials.0.plannedQty"},
		{"maklon catatan bahan panjang", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials[0].Note = string(make([]rune, 501)) }, "materials.0.note"},
		{"maklon terlalu banyak bahan", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderTollManufacturing) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) {
				in.Materials = make([]domain.OrderMaterialInput, domain.MaxLines+1)
			}, "materials"},
		{"reprocess tanpa lokasi GM", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderReprocess) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.MaterialSourceLocationID = nil }, "materialSourceLocationId"},
		{"reprocess tanpa bahan", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderReprocess) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials = nil }, "materials"},
		{"reprocess bahan packaging", func() (domain.OrderInput, domain.OrderRefs) { return tollOrder(c, domain.OrderReprocess) },
			func(in *domain.OrderInput, _ *domain.OrderRefs) { in.Materials[0].ProductID = c.pkg }, "materials.0.productId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := tc.build
			if build == nil {
				build = func() (domain.OrderInput, domain.OrderRefs) { return packagingOrder(c) }
			}
			in, refs := build()
			tc.edit(&in, &refs)
			assert.Contains(t, fieldsOf(t, in.Validate(refs)), tc.field)
		})
	}
	other, refs := packagingOrder(c)
	other.Type, other.Lines[0].ProductID, other.Lines[0].Description = domain.OrderOther, nil, "Jasa desain label"
	require.NoError(t, other.Validate(refs))
	goods, refs := packagingOrder(c)
	goods.Type, goods.Lines[0].ProductID = domain.OrderGoods, &c.fg
	require.NoError(t, goods.Validate(refs))
}

func TestOrderLifecycle(t *testing.T) {
	c := newCatalog()
	in, refs := packagingOrder(c)
	o, err := domain.NewPurchaseOrder(in, refs, uuid.New())
	require.NoError(t, err)
	// NON-PKP: total = subtotal = 10.000 × 1.250 + 3 × 0,33.
	assert.Equal(t, "12500000.99", o.Total.String())
	assert.Equal(t, o.Subtotal, o.Total)
	assert.Equal(t, 30, o.PaymentTermDays, "termin default pemasok")
	assert.Equal(t, "PT Kemas Jaya", o.SupplierName)
	assert.Nil(t, o.TollLocationID)
	assert.Empty(t, o.Materials)

	in.PaymentTermDays = ptr(14)
	require.NoError(t, o.Update(in, refs))
	assert.Equal(t, 14, o.PaymentTermDays)
	bad := in
	bad.Type = domain.OrderGoods
	assert.Contains(t, fieldsOf(t, o.Update(bad, refs)), "orderType")

	require.NoError(t, o.Submit("PO/2026/10/00001", now))
	requireCode(t, o.Update(in, refs), apperr.CodeInvalidStateTransition)
	assert.Empty(t, o.Actions())
	require.NoError(t, o.Reject())
	assert.Equal(t, []domain.Action{domain.ActionRevise}, o.Actions())
	require.NoError(t, o.Revise())
	require.NoError(t, o.Submit("PO/2026/10/00002", now))
	assert.Equal(t, "PO/2026/10/00001", o.Number)
	require.NoError(t, o.Approve(now))
	assert.True(t, o.Receivable())
	assert.True(t, o.AcceptsAdvance())
	assert.Equal(t, []domain.Action{domain.ActionPrint, domain.ActionCreateAdvance, domain.ActionCancel}, o.Actions())

	// Penerimaan sebagian → Diterima Sebagian; toleransi 0%.
	o.Lines[0].ID, o.Lines[1].ID = uuid.New(), uuid.New()
	l0, l1 := o.Lines[0].ID, o.Lines[1].ID
	assert.Contains(t, fieldsOf(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("10001")}})), "lines.0.qty")
	assert.Contains(t, fieldsOf(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: uuid.New(), Qty: qty("1")}})), "lines.0.lineId")
	assert.Contains(t, fieldsOf(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("-1")}})), "lines.0.qty")
	assert.Contains(t, fieldsOf(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("0.0001")}})), "lines.0.qty")
	require.NoError(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("4000")}}))
	assert.Equal(t, domain.StatusPartiallyReceived, o.Status)
	assert.True(t, o.HasReceipt())
	assert.Equal(t, []domain.Action{domain.ActionPrint, domain.ActionCreateAdvance, domain.ActionClose}, o.Actions())
	requireCode(t, o.Cancel("batal"), apperr.CodeInvalidStateTransition)
	// Tagihan sebagian.
	assert.Contains(t, fieldsOf(t, o.ApplyBilling([]domain.QtyDelta{{LineID: l0, Qty: qty("4001")}})), "lines.0.qty")
	require.NoError(t, o.ApplyBilling([]domain.QtyDelta{{LineID: l0, Qty: qty("4000")}}))
	// Pembatalan penerimaan yang sudah ditagih ditolak.
	assert.Contains(t, fieldsOf(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("-1")}})), "lines.0.qty")
	require.NoError(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("6000")}, {LineID: l1, Qty: qty("3")}}))
	assert.Equal(t, domain.StatusReceived, o.Status)
	assert.False(t, o.Receivable())
	assert.Equal(t, []domain.Action{domain.ActionPrint, domain.ActionCreateAdvance}, o.Actions())
	require.NoError(t, o.ApplyBilling([]domain.QtyDelta{{LineID: l0, Qty: qty("6000")}, {LineID: l1, Qty: qty("3")}}))
	assert.Equal(t, domain.StatusCompleted, o.Status)
	assert.False(t, o.AcceptsAdvance())
	assert.Equal(t, []domain.Action{domain.ActionPrint}, o.Actions())
	requireCode(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: l0, Qty: qty("1")}}), apperr.CodeInvalidStateTransition)
	// Tagihan dibatalkan → kembali Diterima.
	require.NoError(t, o.ApplyBilling([]domain.QtyDelta{{LineID: l1, Qty: qty("-3")}}))
	assert.Equal(t, domain.StatusReceived, o.Status)
	require.NoError(t, o.ApplyBilling([]domain.QtyDelta{{LineID: l1, Qty: qty("3")}}))
	assert.Equal(t, domain.StatusCompleted, o.Status)

	line, ok := o.Line(l0)
	require.True(t, ok)
	assert.Equal(t, "0", line.OutstandingQty().String())
	assert.True(t, line.FulfilledQty(domain.StatusCancelled).IsZero())
	assert.Equal(t, "10000", line.FulfilledQty(domain.StatusClosed).String())
	assert.Equal(t, "10000", line.FulfilledQty(domain.StatusDraft).String())
	_, ok = o.Line(uuid.New())
	assert.False(t, ok)
}

func TestOrderCloseCancel(t *testing.T) {
	c := newCatalog()
	in, refs := tollOrder(c, domain.OrderReprocess)
	o, err := domain.NewPurchaseOrder(in, refs, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, refs.TollLocationID, o.TollLocationID)
	assert.Equal(t, in.MaterialSourceLocationID, o.MaterialSourceLocationID)
	require.Len(t, o.Materials, 1)
	assert.Equal(t, "17500000.00", o.Total.String())

	// Draft: batal tanpa alasan.
	draft := o
	require.NoError(t, draft.Cancel(""))
	assert.Equal(t, domain.StatusCancelled, draft.Status)
	assert.Contains(t, fieldsOf(t, o.Cancel(string(make([]rune, 401)))), "reason")

	require.NoError(t, o.Submit("PO/1", now))
	requireCode(t, o.ApplyReceipt(nil), apperr.CodeInvalidStateTransition)
	requireCode(t, o.ApplyBilling(nil), apperr.CodeInvalidStateTransition)
	require.NoError(t, o.Approve(now))
	approved := o
	approved.Lines = append([]domain.OrderLine(nil), o.Lines...)
	assert.Contains(t, fieldsOf(t, approved.Cancel("")), "reason")
	require.NoError(t, approved.Cancel("Maklon tidak sanggup"))
	assert.Equal(t, "Maklon tidak sanggup", approved.CancelReason)

	o.Lines[0].ID = uuid.New()
	requireCode(t, o.Close("x"), apperr.CodeInvalidStateTransition)
	require.NoError(t, o.ApplyReceipt([]domain.QtyDelta{{LineID: o.Lines[0].ID, Qty: qty("4900")}}))
	assert.Contains(t, fieldsOf(t, o.Close("")), "reason")
	require.NoError(t, o.Close("Sisa 100 pcs tidak diproduksi"))
	assert.Equal(t, domain.StatusClosed, o.Status)
	assert.Equal(t, []domain.Action{domain.ActionPrint}, o.Actions())
	require.NoError(t, o.ApplyBilling([]domain.QtyDelta{{LineID: o.Lines[0].ID, Qty: qty("4900")}}))
	assert.Equal(t, domain.StatusClosed, o.Status)
	requireCode(t, o.Revise(), apperr.CodeInvalidStateTransition)

	// Pembatalan seluruh penerimaan mengembalikan PO ke Disetujui.
	p, refs2 := packagingOrder(c)
	po, err := domain.NewPurchaseOrder(p, refs2, uuid.New())
	require.NoError(t, err)
	require.NoError(t, po.Submit("PO/2", now))
	require.NoError(t, po.Approve(now))
	po.Lines[0].ID = uuid.New()
	require.NoError(t, po.ApplyReceipt([]domain.QtyDelta{{LineID: po.Lines[0].ID, Qty: qty("1")}}))
	require.NoError(t, po.ApplyReceipt([]domain.QtyDelta{{LineID: po.Lines[0].ID, Qty: qty("-1")}}))
	assert.Equal(t, domain.StatusApproved, po.Status)
	_, err = domain.NewPurchaseOrder(domain.OrderInput{}, domain.OrderRefs{}, uuid.New())
	require.Error(t, err)
}

func TestLineAmountRounding(t *testing.T) {
	assert.Equal(t, "0.01", domain.LineAmount(qty("1"), money.MustParse("0.005")).String())
	assert.Equal(t, "3333.33", domain.LineAmount(qty("3.333"), money.MustParse("1000.10")).String())
}

// --- Uang muka -------------------------------------------------------------------------------

func approvedOrder(t *testing.T) domain.PurchaseOrder {
	t.Helper()
	c := newCatalog()
	in, refs := packagingOrder(c)
	o, err := domain.NewPurchaseOrder(in, refs, uuid.New())
	require.NoError(t, err)
	o.ID = uuid.New()
	require.NoError(t, o.Submit("PO/2026/10/00001", now))
	require.NoError(t, o.Approve(now))
	return o
}

func TestAdvanceValidate(t *testing.T) {
	o := approvedOrder(t)
	valid := domain.AdvanceInput{PurchaseOrderID: o.ID, Amount: money.MustParse("5000000"), PaymentDate: today, BankAccountCode: "1111"}
	cases := []struct {
		name   string
		edit   func(*domain.AdvanceInput)
		others string
		field  string
	}{
		{"nol", func(in *domain.AdvanceInput) { in.Amount = money.Zero }, "0", "amount"},
		{"melebihi total PO", func(in *domain.AdvanceInput) { in.Amount = money.MustParse("12500001.00") }, "0", "amount"},
		{"melebihi sisa", func(in *domain.AdvanceInput) {}, "8000000", "amount"},
		{"tanggal kosong", func(in *domain.AdvanceInput) { in.PaymentDate = clock.BusinessDate{} }, "0", "paymentDate"},
		{"bukan akun bank", func(in *domain.AdvanceInput) { in.BankAccountCode = "1401" }, "0", "bankAccountCode"},
		{"catatan panjang", func(in *domain.AdvanceInput) { in.Note = string(make([]rune, 1001)) }, "0", "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.edit(&in)
			assert.Contains(t, fieldsOf(t, in.Validate(o, money.MustParse(tc.others))), tc.field)
		})
	}
	require.NoError(t, valid.Validate(o, money.MustParse("7500000.99")))
	draft := o
	draft.Status = domain.StatusDraft
	requireCode(t, valid.Validate(draft, money.Zero), apperr.CodeInvalidStateTransition)
}

func TestAdvanceLifecycle(t *testing.T) {
	o := approvedOrder(t)
	in := domain.AdvanceInput{PurchaseOrderID: o.ID, Amount: money.MustParse("5000000"), PaymentDate: today, BankAccountCode: " 1112 ", Note: " DP 40% "}
	a, err := domain.NewAdvance(in, o, money.Zero, uuid.New())
	require.NoError(t, err)
	a.ID = uuid.New()
	assert.Equal(t, "1112", a.BankAccountCode)
	assert.Equal(t, "DP 40%", a.Note)
	assert.Equal(t, o.Number, a.OrderNumber)
	assert.True(t, a.Counts())
	assert.True(t, a.Outstanding().IsZero(), "draft belum punya sisa")
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionPost, domain.ActionCancel}, a.Actions())
	requireCode(t, a.Apply(money.FromInt(1)), apperr.CodeInvalidStateTransition)

	in.Amount = money.MustParse("6000000")
	require.NoError(t, a.Update(in, o, money.Zero))
	assert.Equal(t, "6000000.00", a.Amount.String())
	moved := in
	moved.PurchaseOrderID = uuid.New()
	assert.Contains(t, fieldsOf(t, a.Update(moved, o, money.Zero)), "purchaseOrderId")
	require.NoError(t, a.CanPost(o, money.Zero))
	assert.Contains(t, fieldsOf(t, a.CanPost(o, money.MustParse("7000000"))), "amount")

	journal := uuid.New()
	require.NoError(t, a.Post("BYR/2026/10/00001", journal, now))
	assert.Equal(t, domain.StatusPosted, a.Status)
	assert.Equal(t, &journal, a.JournalEntryID)
	assert.Equal(t, "6000000.00", a.Outstanding().String())
	assert.Equal(t, []domain.Action{domain.ActionCancel}, a.Actions())
	requireCode(t, a.Update(in, o, money.Zero), apperr.CodeInvalidStateTransition)
	requireCode(t, a.CanPost(o, money.Zero), apperr.CodeInvalidStateTransition)
	requireCode(t, a.Post("BYR/2", journal, now), apperr.CodeInvalidStateTransition)

	// Potong tagihan sebagian, lalu batal ditolak; potongan bisa dikembalikan.
	require.NoError(t, a.Apply(money.MustParse("2500000")))
	assert.Equal(t, "3500000.00", a.Outstanding().String())
	assert.Empty(t, a.Actions())
	requireCode(t, a.Apply(money.MustParse("3500000.01")), apperr.CodeInvalidStateTransition)
	requireCode(t, a.Apply(money.MustParse("-2500000.01")), apperr.CodeInvalidStateTransition)
	requireCode(t, a.Cancel("salah input"), apperr.CodeInvalidStateTransition)
	require.NoError(t, a.Apply(money.MustParse("-2500000")))
	assert.Contains(t, fieldsOf(t, a.Cancel(" ")), "reason")
	require.NoError(t, a.Cancel("Salah nominal"))
	assert.Equal(t, domain.StatusCancelled, a.Status)
	assert.False(t, a.Counts())
	assert.Empty(t, a.Actions())
	requireCode(t, a.Cancel("lagi"), apperr.CodeInvalidStateTransition)

	d, err := domain.NewAdvance(in, o, money.Zero, uuid.New())
	require.NoError(t, err)
	assert.Contains(t, fieldsOf(t, d.Cancel(string(make([]rune, 401)))), "reason")
	require.NoError(t, d.Cancel(""))
	_, err = domain.NewAdvance(domain.AdvanceInput{}, o, money.Zero, uuid.New())
	require.Error(t, err)
}

func TestAllocateAdvances(t *testing.T) {
	posted := func(amount, applied string) domain.Advance {
		return domain.Advance{ID: uuid.New(), Status: domain.StatusPosted, Amount: money.MustParse(amount), Applied: money.MustParse(applied)}
	}
	a, b, c := posted("1000", "1000"), posted("3000", "500"), posted("2000", "0")
	draft := domain.Advance{ID: uuid.New(), Status: domain.StatusDraft, Amount: money.MustParse("9999")}
	alloc := domain.AllocateAdvances([]domain.Advance{a, draft, b, c}, money.MustParse("3000"))
	assert.Equal(t, map[uuid.UUID]money.Amount{b.ID: money.MustParse("2500"), c.ID: money.MustParse("500")}, alloc)
	assert.Empty(t, domain.AllocateAdvances([]domain.Advance{b}, money.Zero))
	all := domain.AllocateAdvances([]domain.Advance{b, c}, money.MustParse("99999"))
	assert.Equal(t, "2500.00", all[b.ID].String())
	assert.Equal(t, "2000.00", all[c.ID].String())
}
