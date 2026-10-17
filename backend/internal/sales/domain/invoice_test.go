package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/sales/domain"
)

func TestProformaLifecycle(t *testing.T) {
	o := confirmed(t, domain.OrderModernTrade, false)
	p := domain.NewProforma("PI/2026/11/00001", o, today, 0)
	assert.Equal(t, "2026-11-16", p.ValidUntil.String(), "default 14 hari")
	assert.Equal(t, o.Total, p.Amount)
	assert.Equal(t, []domain.Action{domain.ActionConfirmPay, domain.ActionPrint}, p.Actions(today))
	assert.False(t, p.IsExpired(today.AddDays(14)))
	assert.True(t, p.IsExpired(today.AddDays(15)))
	assert.Equal(t, []domain.Action{domain.ActionPrint}, p.Actions(today.AddDays(15)))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(p.MarkPaid(uuid.New(), today.AddDays(15), now)))

	inv := uuid.New()
	require.NoError(t, p.MarkPaid(inv, today, now))
	assert.Equal(t, domain.StatusPaid, p.Status)
	assert.Equal(t, inv, *p.InvoiceID)
	require.Error(t, p.Expire())
	require.Error(t, p.Cancel("x"))

	q := domain.NewProforma("PI/2", o, today, 7)
	assert.Equal(t, "2026-11-09", q.ValidUntil.String())
	require.NoError(t, q.Expire())
	r := domain.NewProforma("PI/3", o, today, 14)
	require.NoError(t, r.Cancel(" batal "))
	assert.Equal(t, "batal", r.CancelReason)
}

func newInvoice(t *testing.T) domain.Invoice {
	t.Helper()
	o := confirmed(t, domain.OrderModernTrade, false)
	o.PaymentTermDays = 30
	inv, err := domain.NewInvoiceFromOrder("INV/2026/11/00001", o, today, func(id uuid.UUID) string {
		if id == product1 {
			return "P1 – Produk 1"
		}
		return "B1 – Paket"
	})
	require.NoError(t, err)
	return inv
}

func TestInvoiceFromOrderAndPayments(t *testing.T) {
	inv := newInvoice(t)
	assert.Equal(t, domain.PartyPartner, inv.PartyType)
	assert.Equal(t, "2026-12-02", inv.DueDate.String())
	assert.Equal(t, "1440000.00", inv.Gross.String())
	assert.Equal(t, "140000.00", inv.Discount.String())
	assert.Equal(t, "15000.00", inv.Shipping.String())
	assert.Equal(t, "1315000.00", inv.Total.String())
	require.Len(t, inv.Lines, 2)
	assert.Equal(t, "P1 – Produk 1", inv.Lines[0].Description)
	assert.Equal(t, "900000.00", inv.Lines[0].Total.String())
	assert.True(t, inv.IsOpen())
	assert.Contains(t, inv.Actions(), domain.ActionRecordPayment)

	require.NoError(t, inv.ApplyPayment(amt("315000")))
	assert.Equal(t, domain.StatusPartiallyPaid, inv.Status)
	assert.Equal(t, "1000000.00", inv.Outstanding().String())
	assert.Contains(t, fields(t, inv.ApplyPayment(amt("1000000.01"))), "amount")
	assert.Contains(t, fields(t, inv.ApplyPayment(amt("-315000.01"))), "amount")
	require.NoError(t, inv.ApplyPayment(amt("1000000")))
	assert.Equal(t, domain.StatusPaid, inv.Status)
	assert.False(t, inv.IsOpen())
	assert.NotContains(t, inv.Actions(), domain.ActionRecordPayment)
	require.Error(t, inv.Cancel("x", now), "invoice dibayar tidak bisa dibatalkan")
	require.NoError(t, inv.ApplyPayment(amt("-1315000")))
	assert.Equal(t, domain.StatusAwaitingPayment, inv.Status)

	require.NoError(t, inv.Cancel(" batal ", now))
	assert.Equal(t, domain.StatusCancelled, inv.Status)
	assert.True(t, inv.Outstanding().IsZero())
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(inv.ApplyPayment(amt("1"))))

	mp := confirmed(t, domain.OrderMarketplace, false)
	_, err := domain.NewInvoiceFromOrder("INV/X", mp, today, func(uuid.UUID) string { return "" })
	assert.Contains(t, fields(t, err), "customerId", "marketplace tanpa pelanggan tidak berinvoice")
}

func TestInvoiceAging(t *testing.T) {
	inv := newInvoice(t)
	assert.Equal(t, 0, inv.DaysOverdue(today.AddDays(30)))
	assert.Equal(t, 5, inv.DaysOverdue(today.AddDays(35)))
	for days, want := range map[int]domain.AgingBucket{
		0: domain.BucketCurrent, -3: domain.BucketCurrent, 1: domain.Bucket1To30, 30: domain.Bucket1To30, 31: domain.Bucket31To60,
		60: domain.Bucket31To60, 61: domain.Bucket61To90, 90: domain.Bucket61To90, 91: domain.BucketOver90,
	} {
		assert.Equal(t, want, domain.BucketOf(days), "hari %d", days)
	}

	other := newInvoice(t)
	other.PartyID, other.PartyName = uuid.New(), "Abadi"
	paid := newInvoice(t)
	require.NoError(t, paid.ApplyPayment(paid.Total))
	future := newInvoice(t)
	future.InvoiceDate = today.AddDays(100)
	partial := newInvoice(t)
	require.NoError(t, partial.ApplyPayment(amt("15000")))

	rows := domain.AgeReceivables([]domain.Invoice{inv, other, paid, future, partial}, today.AddDays(45))
	require.Len(t, rows, 2)
	assert.Equal(t, "Abadi", rows[0].PartyName, "urut nama pihak")
	assert.Equal(t, "1315000.00", rows[0].Buckets[domain.Bucket1To30].String())
	assert.Equal(t, "Mitra A", rows[1].PartyName)
	assert.Equal(t, 2, rows[1].Invoices)
	assert.Equal(t, "2615000.00", rows[1].Total.String())
	assert.Equal(t, "0.00", rows[1].Buckets[domain.BucketCurrent].String())
}

func TestNewPaymentAllocations(t *testing.T) {
	inv1, inv2 := newInvoice(t), newInvoice(t)
	inv1.ID, inv2.ID, inv2.Number = uuid.New(), uuid.New(), "INV/2"
	invoices := map[uuid.UUID]domain.Invoice{inv1.ID: inv1, inv2.ID: inv2}
	base := domain.PaymentInput{
		PartyType: domain.PartyPartner, PartyID: partner, PaymentDate: today, Amount: amt("1500000"),
		Method: domain.MethodBankTransfer, AccountCode: "1111", Reference: " TRF-1 ",
		Allocations: []domain.PaymentAllocation{{InvoiceID: inv1.ID, Amount: amt("1315000")}, {InvoiceID: inv2.ID, Amount: amt("185000")}},
	}
	p, err := domain.NewPayment(base, invoices, []string{"1101", "1111"})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPosted, p.Status)
	assert.Equal(t, "TRF-1", p.Reference)
	assert.Equal(t, "INV/2", p.Allocations[1].InvoiceNumber)

	tests := []struct {
		name  string
		mut   func(in *domain.PaymentInput)
		field string
	}{
		{"pihak tidak dikenal", func(in *domain.PaymentInput) { in.PartyType = "x" }, "partyType"},
		{"tanpa pihak", func(in *domain.PaymentInput) { in.PartyID = uuid.Nil }, "partyId"},
		{"tanpa tanggal", func(in *domain.PaymentInput) { in.PaymentDate = clock.BusinessDate{} }, "paymentDate"},
		{"nominal nol", func(in *domain.PaymentInput) { in.Amount = amt("0") }, "amount"},
		{"cara bayar", func(in *domain.PaymentInput) { in.Method = "giro" }, "method"},
		{"akun bukan kas", func(in *domain.PaymentInput) { in.AccountCode = "1201" }, "accountCode"},
		{"referensi panjang", func(in *domain.PaymentInput) { in.Reference = strings.Repeat("x", 201) }, "reference"},
		{"catatan panjang", func(in *domain.PaymentInput) { in.Note = strings.Repeat("x", 1001) }, "note"},
		{"tanpa alokasi", func(in *domain.PaymentInput) { in.Allocations = nil }, "allocations"},
		{"invoice tidak ada", func(in *domain.PaymentInput) { in.Allocations[0].InvoiceID = uuid.New() }, "allocations.0.invoiceId"},
		{"invoice ganda", func(in *domain.PaymentInput) { in.Allocations[1].InvoiceID = inv1.ID }, "allocations.1.invoiceId"},
		{"pihak lain", func(in *domain.PaymentInput) { in.PartyID = uuid.New() }, "allocations.0.invoiceId"},
		{"alokasi nol", func(in *domain.PaymentInput) { in.Allocations[0].Amount = amt("0") }, "allocations.0.amount"},
		{"alokasi melebihi sisa", func(in *domain.PaymentInput) { in.Allocations[0].Amount = amt("1315000.01") }, "allocations.0.amount"},
		{"jumlah alokasi beda", func(in *domain.PaymentInput) { in.Amount = amt("1600000") }, "amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			in.Allocations = append([]domain.PaymentAllocation(nil), base.Allocations...)
			tt.mut(&in)
			_, err := domain.NewPayment(in, invoices, []string{"1111"})
			assert.Contains(t, fields(t, err), tt.field)
		})
	}

	closed := inv2
	require.NoError(t, closed.ApplyPayment(closed.Total))
	_, err = domain.NewPayment(base, map[uuid.UUID]domain.Invoice{inv1.ID: inv1, inv2.ID: closed}, []string{"1111"})
	assert.Contains(t, fields(t, err), "allocations.1.invoiceId", "invoice lunas")

	assert.Contains(t, fields(t, p.Cancel("")), "reason")
	require.NoError(t, p.Cancel("Salah input"))
	assert.Equal(t, domain.StatusCancelled, p.Status)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(p.Cancel("lagi")))
	assert.True(t, domain.MethodCash.Valid())
}

func TestMarketplaceHelpers(t *testing.T) {
	assert.Equal(t, "1211", domain.MarketplaceReceivableAccount("shopee"))
	assert.Equal(t, "1212", domain.MarketplaceReceivableAccount("tiktok_tokopedia"))
	assert.Equal(t, "1213", domain.MarketplaceReceivableAccount("lazada"))
	assert.Equal(t, "1214", domain.MarketplaceReceivableAccount("blibli"))
	assert.Empty(t, domain.MarketplaceReceivableAccount("lain"))

	items, msg := domain.ParseImportItems(" fg-a:2:185000 ; BDL:1 ;")
	require.Empty(t, msg)
	require.Len(t, items, 2)
	assert.Equal(t, "FG-A", items[0].SKU)
	assert.Equal(t, "185000.00", items[0].UnitPrice.String())
	assert.Nil(t, items[1].UnitPrice)
	for _, bad := range []string{"", "A", "A:1:2:3", ":1", "A:x", "A:0", "A:1:abc", "A:1:-5"} {
		_, msg := domain.ParseImportItems(bad)
		assert.NotEmpty(t, msg, bad)
	}

	inv := newInvoice(t)
	inv.OrderNumber = "SO/2026/11/00001"
	text := domain.ShareText(inv, "https://erp/sales/invoices/1/print")
	for _, want := range []string{"*Invoice INV/2026/11/00001*", "2 Nov 2026", "2 Des 2026", "SO/2026/11/00001", "P1 – Produk 1 × 10",
		"Diskon: -Rp140.000", "Ongkos kirim: Rp15.000", "*Total: Rp1.315.000*", "Menunggu Pembayaran", "https://erp", domain.NonPKPNote} {
		assert.Contains(t, text, want)
	}
	require.NoError(t, inv.ApplyPayment(amt("15000")))
	assert.Contains(t, domain.ShareText(inv, ""), "Dibayar sebagian, sisa Rp1.300.000")
	require.NoError(t, inv.ApplyPayment(inv.Outstanding()))
	assert.Contains(t, domain.ShareText(inv, ""), "LUNAS")
}
