package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
	"balans/internal/sales"
)

// Laporan R1.4 (S-403, S-409, S-414 – S-418): angka laporan dari read model harus sama dengan buku besar.

func (h *salesHarness) refreshReports() {
	h.t.Helper()
	ran, err := h.app.accounting.RefreshReadModels(context.Background())
	require.NoError(h.t, err)
	require.True(h.t, ran)
}

func dec(t *testing.T, v any) decimal.Decimal {
	t.Helper()
	s, ok := v.(string)
	require.True(t, ok, "nilai %v bukan string", v)
	return decimal.RequireFromString(s)
}

// glNet adalah Σ(debit − kredit) jurnal diposting langsung dari journal_lines (pembanding independen laporan).
func (h *salesHarness) glNet(codePattern, from, to string, extra string, args ...any) decimal.Decimal {
	h.t.Helper()
	q := `SELECT coalesce(sum(l.debit_amount - l.credit_amount), 0)::text
		FROM journal_lines l JOIN journal_entries e ON e.id = l.journal_entry_id JOIN accounts a ON a.id = l.account_id
		LEFT JOIN channels c ON c.id = l.channel_id
		WHERE e.status = 'posted' AND a.code LIKE $1 AND e.business_date >= $2::date AND e.business_date <= $3::date ` + extra
	return decimal.RequireFromString(h.scalar(q, append([]any{codePattern, from, to}, args...)...))
}

// marketplaceSale mencatat order Shopee qty × harga dengan voucher lalu menyerahkannya ke kurir (A-10).
func (h *salesHarness) marketplaceSale(product uuid.UUID, ext string, qty int, price, voucher string) map[string]any {
	h.t.Helper()
	h.as("sales.ecommerce@balans.local")
	o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "marketplace", "channelId": h.channel("marketplace"), "subChannelId": h.subChannel("marketplace", "shopee"),
		"externalOrderId": ext, "buyerName": "Budi", "businessDate": "2026-11-02", "sourceLocationId": h.ecm,
		"documentDiscount": map[string]any{"kind": "amount", "value": voucher},
		"lines":            []map[string]any{{"productId": product, "qty": fmt.Sprint(qty), "unitPrice": price}},
	}, http.StatusCreated)
	_, err := h.app.sales.MarkHandedOver(actor.System(context.Background()), sales.ShipmentInput{
		OrderID: uuid.MustParse(o["id"].(string)), SourceType: "delivery_order", SourceID: uuid.New(), SourceLabel: "DO/" + ext,
	})
	require.NoError(h.t, err)
	return o
}

type reportSnapshot struct {
	salesNet, salesCOGS, stockValue, glInventory, glRevenue decimal.Decimal
}

func (h *salesHarness) reportSnapshot() reportSnapshot {
	h.t.Helper()
	h.refreshReports()
	h.as("finance@balans.local")
	rep := h.json(http.MethodGet, "/api/v1/sales/reports/sales?groupBy=channel&dateFrom=2026-11-02&dateTo=2026-11-02&channelId="+
		h.channel("marketplace"), nil, http.StatusOK)
	total := rep["total"].(map[string]any)
	h.as("admin@balans.local")
	stock := h.json(http.MethodGet, "/api/v1/inventory/reports/stock-position", nil, http.StatusOK)
	return reportSnapshot{
		salesNet: dec(h.t, total["net"]), salesCOGS: dec(h.t, total["cogs"]), stockValue: dec(h.t, stock["totalValue"]),
		glInventory: h.glNet("13%", "2000-01-01", "2026-11-02", ""),
		glRevenue:   h.glNet("410%", "2026-11-02", "2026-11-02", "AND c.code = 'marketplace'").Neg(),
	}
}

func TestReportsMarketplaceSettlementReconciles(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("RCK", "100000")
	h.receive(coklat, h.ecm, "R1", 300, 20, "40000")
	before := h.reportSnapshot()

	ext := "SHP-ST-" + h.tag
	order := h.marketplaceSale(coklat, ext, 3, "100000", "10000")
	require.Equal(t, "290000.00", order["totalAmount"])
	after := h.reportSnapshot()

	// Laporan penjualan (read model) = jurnal 4101 − 4102 channel marketplace; HPP = 3 × 40.000.
	assert.Equal(t, "290000", after.salesNet.Sub(before.salesNet).String())
	assert.Equal(t, after.glRevenue.Sub(before.glRevenue).String(), after.salesNet.Sub(before.salesNet).String())
	assert.Equal(t, "120000", after.salesCOGS.Sub(before.salesCOGS).String())
	// Nilai stok laporan berubah sama dengan saldo akun persediaan 13xx.
	assert.Equal(t, "-120000", after.stockValue.Sub(before.stockValue).String())
	assert.Equal(t, after.glInventory.Sub(before.glInventory).String(), after.stockValue.Sub(before.stockValue).String())

	// --- S-403: impor laporan settlement Shopee, cocokkan per order, posting A-10s ---------------------
	shopee, ok := sales.SettlementFormats()[0], sales.SettlementFormats()[0].Marketplace == "shopee"
	require.True(t, ok)
	h.as("finance@balans.local")
	ref := "PAY-" + h.tag
	row := func(n int, values map[string]string) tabular.Row {
		r := tabular.NewRow(n)
		for k, v := range values {
			r.Values[k] = v
		}
		return r
	}
	importRow := func(r tabular.Row) error {
		return h.app.Tx.WithTx(h.ctx(), func(ctx context.Context) error {
			return h.app.sales.ImportSettlementRow(ctx, shopee, r, "")
		})
	}
	require.NoError(t, importRow(row(2, map[string]string{
		"order_no": ext, "payout_date": "2026-11-09", "payout_ref": ref, "net": "270000",
		"fee_admin": "-9000", "fee_service": "-3000", "promo_voucher": "-8000",
	})))
	err := importRow(row(3, map[string]string{"order_no": ext, "payout_date": "2026-11-09", "payout_ref": ref + "B", "net": "1000"}))
	require.Error(t, err, "settlement melebihi sisa piutang order ditolak")
	assert.Contains(t, apperr.As(err).Fields["net"], "melebihi")
	err = importRow(row(4, map[string]string{"order_no": "TIDAK-ADA-" + h.tag, "payout_date": "2026-11-09", "net": "1000"}))
	require.Error(t, err, "order tidak dikenal ditolak")

	list := h.json(http.MethodGet, "/api/v1/sales/marketplace-settlements?status=draft", nil, http.StatusOK)
	var id string
	for _, it := range items(list, "items") {
		if it["payoutReference"] == ref {
			id = it["id"].(string)
		}
	}
	require.NotEmpty(t, id)
	st := h.json(http.MethodGet, "/api/v1/sales/marketplace-settlements/"+id, nil, http.StatusOK)
	assert.Equal(t, "290000.00", st["settled"])
	assert.Equal(t, "12000.00", st["fee"])
	assert.Equal(t, "8000.00", st["promo"])
	require.Len(t, items(st, "lines"), 1)
	assert.ElementsMatch(t, []any{"post", "cancel"}, st["allowedActions"])

	posted := h.json(http.MethodPost, "/api/v1/sales/marketplace-settlements/"+id+"/post", map[string]any{"version": st["version"]}, http.StatusOK)
	assert.Equal(t, "posted", posted["status"])
	assert.Equal(t, "0.00", items(posted, "lines")[0]["outstanding"], "piutang order lunas")
	a10s := h.journal("A-10s", "marketplace_settlement", id)
	d, _ := amountOf(a10s, "1111")
	assert.Equal(t, "270000.00", d)
	d, _ = amountOf(a10s, "6105")
	assert.Equal(t, "12000.00", d)
	d, _ = amountOf(a10s, "6107")
	assert.Equal(t, "8000.00", d)
	_, c := amountOf(a10s, "1211")
	assert.Equal(t, "290000.00", c)
	assertDims(t, a10s, "marketplace", "shopee")

	recv := h.json(http.MethodGet, "/api/v1/sales/marketplace-receivables?openOnly=false&q="+url.QueryEscape(ext), nil, http.StatusOK)
	require.Len(t, items(recv, "items"), 1)
	assert.Equal(t, "0.00", items(recv, "items")[0]["outstanding"])
	open := h.json(http.MethodGet, "/api/v1/sales/marketplace-receivables?q="+url.QueryEscape(ext), nil, http.StatusOK)
	assert.Empty(t, items(open, "items"), "order lunas tidak tampil di piutang terbuka")

	err = importRow(row(5, map[string]string{"order_no": ext, "payout_date": "2026-11-09", "payout_ref": ref, "net": "1"}))
	require.Error(t, err, "settlement yang sudah diposting tidak bisa ditambah")

	// Sales Ecommerce tidak boleh memposting settlement.
	h.as("sales.ecommerce@balans.local")
	status, _, _ := h.do(http.MethodGet, "/api/v1/sales/marketplace-settlements", nil)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestReportsFinancialStatementsReconcileWithLedger(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("RFS", "100000")
	h.receive(coklat, h.ecm, "F1", 300, 10, "30000")
	h.marketplaceSale(coklat, "SHP-FS-"+h.tag, 2, "100000", "0")
	h.refreshReports()
	h.as("finance@balans.local")

	from, to := "2026-11-01", "2026-11-30"
	q := "&dateFrom=" + from + "&dateTo=" + to
	is := h.json(http.MethodGet, "/api/v1/reports/financial-statements?kind=income_statement"+q, nil, http.StatusOK)
	sum := is["summary"].(map[string]any)
	// Laba Rugi = jurnal kelompok 4 – 7 (dibaca langsung dari journal_lines).
	assert.Equal(t, h.glNet("4%", from, to, "").Neg().StringFixed(2), dec(t, sum["revenue"]).StringFixed(2))
	assert.Equal(t, h.glNet("5%", from, to, "").StringFixed(2), dec(t, sum["cogs"]).StringFixed(2))
	net := h.glNet("4%", from, to, "").Add(h.glNet("5%", from, to, "")).Add(h.glNet("6%", from, to, "")).Add(h.glNet("7%", from, to, "")).Neg()
	assert.Equal(t, net.StringFixed(2), dec(t, sum["netProfit"]).StringFixed(2))
	assert.NotNil(t, is["refreshedAt"])

	bs := h.json(http.MethodGet, "/api/v1/reports/financial-statements?kind=balance_sheet&dateTo="+to, nil, http.StatusOK)
	assert.Equal(t, true, bs["balanced"], "Neraca seimbang")
	var totalAssets string
	for _, r := range items(bs, "rows") {
		if r["label"] == "TOTAL ASET" {
			totalAssets = r["amount"].(string)
		}
	}
	assert.Equal(t, h.glNet("1%", "2000-01-01", to, "").StringFixed(2), totalAssets, "Total Aset = saldo kelompok 1")

	cf := h.json(http.MethodGet, "/api/v1/reports/financial-statements?kind=cash_flow&dateFrom=2026-01-01&dateTo="+to, nil, http.StatusOK)
	assert.Equal(t, true, cf["balanced"], "kenaikan kas = mutasi Kas & Bank")

	// Laba rugi per dimensi: jumlah kolom = laba rugi total untuk setiap dimensi.
	for _, dim := range []string{"channel", "sub_channel", "division", "campaign"} {
		r := h.json(http.MethodGet, "/api/v1/reports/profit-loss-by-dimension?dimension="+dim+q, nil, http.StatusOK)
		total := decimal.Zero
		for _, c := range items(r, "columns") {
			total = total.Add(dec(t, c["summary"].(map[string]any)["netProfit"]))
		}
		assert.Equal(t, dec(t, sum["netProfit"]).StringFixed(2), total.StringFixed(2), "Σ kolom %s = laba bersih", dim)
		assert.Equal(t, sum["netProfit"], r["total"].(map[string]any)["netProfit"])
	}

	// Margin per produk: penjualan neto produk ini 200.000, HPP 60.000.
	margin := h.json(http.MethodGet, "/api/v1/sales/reports/sales?groupBy=product&productId="+coklat.String()+q, nil, http.StatusOK)
	require.Len(t, items(margin, "lines"), 1)
	assert.Equal(t, "200000.00", items(margin, "lines")[0]["net"])
	assert.Equal(t, "60000.00", items(margin, "lines")[0]["cogs"])
	assert.Equal(t, "70.00", items(margin, "lines")[0]["marginPercent"])
	cat := h.json(http.MethodGet, "/api/v1/sales/reports/sales?groupBy=category"+q, nil, http.StatusOK)
	assert.NotEmpty(t, items(cat, "lines"))

	dash := h.json(http.MethodGet, "/api/v1/reports/executive-dashboard?dateFrom="+from+"&dateTo="+to, nil, http.StatusOK)
	assert.Equal(t, sum["revenue"], dash["period"].(map[string]any)["revenue"])
	assert.Len(t, dash["trend"], 12)
	assert.Equal(t, h.glNet("13%", "2000-01-01", to, "").StringFixed(2), dash["inventory"], "nilai stok dashboard = saldo 13xx")

	tax := h.json(http.MethodGet, "/api/v1/reports/tax-summary?year=2026&month=11", nil, http.StatusOK)
	months := items(tax, "months")
	require.Len(t, months, 12)
	turnover := h.glNet("4101", from, to, "").Add(h.glNet("4201", from, to, "")).Neg()
	assert.Equal(t, turnover.StringFixed(2), months[10]["turnover"])

	// Ekspor laporan sebagai job dataexchange.export dengan parameter periode.
	exp := h.json(http.MethodPost, "/api/v1/export-jobs", map[string]any{
		"kind": "report.financial_statement", "params": map[string]string{"kind": "income_statement", "dateFrom": from, "dateTo": to},
	}, http.StatusAccepted)
	require.NoError(t, h.app.dataexchange.RunExport(context.Background(), uuid.MustParse(exp["id"].(string))))
	exp = h.json(http.MethodGet, "/api/v1/export-jobs/"+exp["id"].(string), nil, http.StatusOK)
	require.Equal(t, "completed", exp["status"], exp["message"])
	assert.Greater(t, exp["totalRows"], float64(5))
	status, problem, _ := h.do(http.MethodPost, "/api/v1/export-jobs", map[string]any{
		"kind": "report.financial_statement", "params": map[string]string{"kind": "salah"},
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status, problem)

	// Hanya pemegang accounting.report.read.
	h.as("sales.ecommerce@balans.local")
	status, _, _ = h.do(http.MethodGet, "/api/v1/reports/financial-statements?kind=income_statement"+q, nil)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestReportsOperationalEndpoints(t *testing.T) {
	h := newSalesHarness(t, "procurement")
	coklat := h.product("ROP", "100000")
	h.receive(coklat, h.ecm, "O1", 10, 5, "30000")
	h.refreshReports()
	h.as("admin@balans.local")
	q := "?dateFrom=2026-11-01&dateTo=2026-11-30"

	pos := h.json(http.MethodGet, "/api/v1/inventory/reports/stock-position?productId="+coklat.String(), nil, http.StatusOK)
	require.Len(t, items(pos, "items"), 1)
	assert.Equal(t, "150000.00", pos["totalValue"])
	assert.Equal(t, "ecommerce", items(pos, "items")[0]["location"].(map[string]any)["type"])

	mv := h.json(http.MethodGet, "/api/v1/inventory/reports/stock-movements"+q+"&productId="+coklat.String(), nil, http.StatusOK)
	require.Len(t, items(mv, "items"), 1)
	assert.Equal(t, "0.000", items(mv, "items")[0]["openingQty"])
	assert.Equal(t, "5.000", items(mv, "items")[0]["receiveQty"])
	assert.Equal(t, "5.000", items(mv, "items")[0]["closingQty"])

	exp := h.json(http.MethodGet, "/api/v1/inventory/reports/stock-expiry?withinDays=30&productId="+coklat.String(), nil, http.StatusOK)
	require.Len(t, items(exp, "items"), 1)
	assert.Equal(t, "near", items(exp, "items")[0]["bucket"])
	assert.EqualValues(t, 10, items(exp, "items")[0]["daysToExpiry"])

	for _, path := range []string{
		"/api/v1/inventory/reports/internal-usage" + q,
		"/api/v1/inventory/reports/returns" + q,
		"/api/v1/warehouse/reports/stock-count-variances" + q,
		"/api/v1/procurement/reports/outstanding-purchase-orders",
		"/api/v1/procurement/reports/payable-aging",
		"/api/v1/procurement/reports/batch-costs" + q,
		"/api/v1/sales/reports/consignment" + q,
		"/api/v1/sales/settlement-formats",
		"/api/v1/reports/read-models",
	} {
		status, _, raw := h.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, status, "%s: %s", path, raw)
	}
	aging := h.json(http.MethodGet, "/api/v1/procurement/reports/payable-aging", nil, http.StatusOK)
	assert.Equal(t, "0.00", aging["difference"], "total utang terbuka = saldo akun 2101")

	// Ekspor laporan stok sebagai job.
	job := h.json(http.MethodPost, "/api/v1/export-jobs", map[string]any{
		"kind": "report.stock_position", "params": map[string]string{"productId": coklat.String()},
	}, http.StatusAccepted)
	require.NoError(t, h.app.dataexchange.RunExport(context.Background(), uuid.MustParse(job["id"].(string))))
	job = h.json(http.MethodGet, "/api/v1/export-jobs/"+job["id"].(string), nil, http.StatusOK)
	require.Equal(t, "completed", job["status"], job["message"])
	assert.EqualValues(t, 1, job["totalRows"])
}
