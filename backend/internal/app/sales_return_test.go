package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/actor"
	"balans/internal/sales"
)

// shipAll mengirim seluruh alokasi pesanan lewat DO tiruan (modul logistics belum dipasang di harness).
func (h *salesHarness) shipAll(orderID string) {
	h.t.Helper()
	_, err := h.app.sales.MarkShipped(actor.System(context.Background()), sales.ShipmentInput{
		OrderID: uuid.MustParse(orderID), SourceType: "delivery_order", SourceID: uuid.New(), SourceLabel: "DO/TEST",
	})
	require.NoError(h.t, err)
}

// photo mengunggah foto retur dan mengembalikan fileId.
func (h *salesHarness) photo(name string) string {
	h.t.Helper()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 64)...)
	target := h.json(http.MethodPost, "/api/v1/files", map[string]any{
		"ownerType": sales.FileOwnerSalesReturnPhoto, "fileName": name, "contentType": "image/png", "sizeBytes": len(png),
	}, http.StatusCreated)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, h.srv.URL+target["uploadUrl"].(string), bytes.NewReader(png))
	require.NoError(h.t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	require.NoError(h.t, res.Body.Close())
	require.Equal(h.t, http.StatusNoContent, res.StatusCode)
	return target["fileId"].(string)
}

func (h *salesHarness) salesReturnReason() string {
	return h.scalar(`SELECT id::text FROM return_reasons WHERE applies_to IN ('sales_return', 'both') AND deleted_at IS NULL
		AND is_active ORDER BY code LIMIT 1`)
}

// qcInbound: Staf Gudang meluluskan/menolak seluruh baris Inbound barang retur.
func (h *salesHarness) qcInbound(inboundID string, rejectFirst string) {
	h.t.Helper()
	h.as("staf.gudang@balans.local")
	ib := h.json(http.MethodGet, "/api/v1/inbounds/"+inboundID, nil, http.StatusOK)
	allOK := map[string]any{"batch": true, "expiry": true, "condition": true, "seal": true}
	for i, l := range items(ib, "lines") {
		body := map[string]any{"passedQty": l["qty"], "rejectedQty": "0", "checks": allOK}
		if i == 0 && rejectFirst != "" {
			total := l["qty"].(string)
			var q, r float64
			_, _ = fmt.Sscan(total, &q)
			_, _ = fmt.Sscan(rejectFirst, &r)
			body = map[string]any{
				"passedQty": fmt.Sprintf("%.3f", q-r), "rejectedQty": rejectFirst, "rejectReason": "Kemasan bocor",
				"checks": map[string]any{"batch": true, "expiry": true, "condition": false, "seal": true},
			}
		}
		h.json(http.MethodPost, "/api/v1/inbounds/"+inboundID+"/lines/"+l["id"].(string)+"/qc", body, http.StatusOK)
	}
}

func (h *salesHarness) returnsLocation() string {
	return h.scalar(`SELECT id::text FROM locations WHERE code = 'UTAMA-RET'`)
}

// --- S-317 retur MT: ajukan → kembalikan → verifikasi → pickup → terima → Inbound → nota kredit ----------

func TestSalesReturnModernTradeVerificationCreditNote(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 300, 50, "40000")
	partner, sub := h.partnerWithSubChannel("modern_trade", "modern_trade")
	subCode := h.scalar(`SELECT code FROM sub_channels WHERE id = $1`, sub)
	reason := h.salesReturnReason()

	h.as("sales.mt@balans.local")
	o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "modern_trade", "channelId": h.channel("modern_trade"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "credit", "paymentTermDays": 30,
		"lines": []map[string]any{{"productId": coklat, "qty": "10"}},
	}, http.StatusCreated)
	so := h.json(http.MethodPost, "/api/v1/sales/orders/"+o["id"].(string)+"/confirm", map[string]any{"version": version(o)}, http.StatusOK)
	invoiceID := items(so, "invoices")[0]["id"].(string)
	soLine := items(so, "lines")[0]["id"].(string)

	// Belum dikirim → tidak bisa diretur.
	status, _, _ := h.do(http.MethodGet, "/api/v1/sales/return-sources/"+so["id"].(string), nil)
	require.Equal(t, http.StatusConflict, status)
	h.shipAll(so["id"].(string))

	src := h.json(http.MethodGet, "/api/v1/sales/return-sources/"+so["id"].(string), nil, http.StatusOK)
	assert.Equal(t, true, src["requiresVerification"])
	srcLine := items(src, "lines")[0]
	assert.Equal(t, "10.000", srcLine["availableQty"])
	assert.Equal(t, "40000.0000", srcLine["unitCost"])
	assert.Len(t, srcLine["batches"], 1)

	// Qty melebihi qty terkirim ditolak.
	bad := h.json(http.MethodPost, "/api/v1/sales/returns", map[string]any{
		"salesOrderId": so["id"], "returnDate": "2026-11-02", "settlementMethod": "credit_note",
		"lines": []map[string]any{{"salesOrderLineId": soLine, "qty": "11", "returnReasonId": reason}},
	}, http.StatusUnprocessableEntity)
	assert.Contains(t, bad["fields"], "lines.0.qty")

	r := h.json(http.MethodPost, "/api/v1/sales/returns", map[string]any{
		"salesOrderId": so["id"], "returnDate": "2026-11-02", "settlementMethod": "credit_note", "note": "Retur outlet",
		"lines": []map[string]any{{"salesOrderLineId": soLine, "qty": "4", "returnReasonId": reason}},
	}, http.StatusCreated)
	assert.Equal(t, "draft", r["status"])
	assert.Equal(t, "400000.00", r["returnAmount"])
	assert.Equal(t, "160000.00", r["costAmount"])
	assert.ElementsMatch(t, []any{"edit", "submit", "cancel"}, r["allowedActions"])
	id := r["id"].(string)
	base := "/api/v1/sales/returns/" + id

	r = h.json(http.MethodPost, base+"/submit", map[string]any{"version": version(r)}, http.StatusOK)
	require.Equal(t, "submitted", r["status"])
	assert.Regexp(t, `^RJ/2026/11/\d{5}$`, r["number"])
	number := r["number"]

	// Logistik Leader mengembalikan (catatan wajib), Sales mengajukan ulang dengan nomor yang sama.
	h.as("logistik.leader@balans.local")
	assert.ElementsMatch(t, []any{"verify", "send_back"}, h.json(http.MethodGet, base, nil, http.StatusOK)["allowedActions"])
	h.json(http.MethodPost, base+"/send-back", map[string]any{"version": version(r), "note": ""}, http.StatusUnprocessableEntity)
	r = h.json(http.MethodPost, base+"/send-back", map[string]any{"version": version(r), "note": "Foto kemasan belum ada"}, http.StatusOK)
	assert.Equal(t, "draft", r["status"])
	assert.Equal(t, "Foto kemasan belum ada", r["sendBackNote"])
	h.as("sales.mt@balans.local")
	r = h.json(http.MethodPost, base+"/submit", map[string]any{"version": version(r)}, http.StatusOK)
	assert.Equal(t, number, r["number"])

	h.as("logistik.leader@balans.local")
	r = h.json(http.MethodPost, base+"/verify", map[string]any{"version": version(r), "pickupRequired": true, "note": "Sesuai SOP"}, http.StatusOK)
	require.Equal(t, "verified", r["status"])

	// Belum diambil → belum bisa diterima.
	h.as("sales.mt@balans.local")
	assert.NotContains(t, r["allowedActions"], "receive")
	h.json(http.MethodPost, base+"/receive", map[string]any{"version": version(r), "documentationFileIds": []string{h.photo("form.png")}}, http.StatusConflict)

	// Fulfillment Leader menugaskan Merchandiser.
	h.as("fulfillment.leader@balans.local")
	users := items(h.json(http.MethodGet, "/api/v1/sales/return-pickup-users", nil, http.StatusOK), "items")
	require.NotEmpty(t, users)
	merch := ""
	for _, u := range users {
		if u["name"] == "Merchandiser" {
			merch = u["id"].(string)
		}
	}
	require.NotEmpty(t, merch)
	r = h.json(http.MethodGet, base, nil, http.StatusOK)
	assert.Contains(t, r["allowedActions"], "assign_pickup")
	r = h.json(http.MethodPost, base+"/assign-pickup", map[string]any{"version": version(r), "pickupUserId": merch}, http.StatusOK)
	assert.Equal(t, "Merchandiser", r["pickupUserName"])

	// Merchandiser mencatat pickup (foto wajib) dari HP.
	h.as("merchandiser@balans.local")
	mine := h.json(http.MethodGet, "/api/v1/sales/returns?assignedToMe=true", nil, http.StatusOK)
	require.Len(t, items(mine, "items"), 1)
	r = h.json(http.MethodGet, base, nil, http.StatusOK)
	assert.Contains(t, r["allowedActions"], "record_pickup")
	h.json(http.MethodPost, base+"/pickup", map[string]any{"version": version(r), "photoFileIds": []string{}}, http.StatusUnprocessableEntity)
	r = h.json(http.MethodPost, base+"/pickup", map[string]any{
		"version": version(r), "note": "Barang dicek di outlet", "photoFileIds": []string{h.photo("pickup.png")},
	}, http.StatusOK)
	assert.NotNil(t, r["pickedUpAt"])
	assert.Len(t, r["pickupFiles"], 1)

	// Sales cek ulang saat barang tiba: hanya 3 dari 4 yang diterima; foto & form retur wajib.
	h.as("sales.mt@balans.local")
	h.json(http.MethodPost, base+"/receive", map[string]any{"version": version(r), "documentationFileIds": []string{}}, http.StatusUnprocessableEntity)
	r = h.json(http.MethodPost, base+"/receive", map[string]any{
		"version": version(r), "note": "1 pcs tidak ada", "documentationFileIds": []string{h.photo("barang.png"), h.photo("form.png")},
		"lines": []map[string]any{{"lineId": items(r, "lines")[0]["id"], "qty": "3"}},
	}, http.StatusOK)
	require.Equal(t, "in_progress", r["status"])
	assert.Equal(t, "300000.00", r["returnAmount"])
	assert.Equal(t, "120000.00", r["costAmount"])
	require.NotNil(t, r["inboundId"])
	assert.Regexp(t, `^IN/`, r["inboundNumber"])
	assert.Equal(t, "3", h.scalar(`SELECT coalesce(sum(qty), 0)::int::text FROM stock_balances b JOIN locations l ON l.id = b.location_id
		WHERE b.product_id = $1 AND l.code = 'UTAMA-INB' AND b.stock_status = 'quarantine'`, coklat), "barang retur Karantina di Inbound")

	// QC Inbound: 1 ditolak; semua barang ke Gudang Retur, retur Diposting, A-13 dan nota kredit.
	h.qcInbound(r["inboundId"].(string), "1")
	h.as("sales.mt@balans.local")
	r = h.json(http.MethodGet, base, nil, http.StatusOK)
	require.Equal(t, "posted", r["status"])
	line := items(r, "lines")[0]
	assert.Equal(t, "2.000", line["passedQty"])
	assert.Equal(t, "1.000", line["rejectedQty"])
	assert.Equal(t, "3", h.scalar(`SELECT coalesce(sum(qty), 0)::int::text FROM stock_balances b
		WHERE b.product_id = $1 AND b.location_id = $2`, coklat, h.returnsLocation()), "lulus & ditolak di Gudang Retur")

	a13 := h.journal("A-13", "sales_return", id)
	d, _ := amountOf(a13, "4103")
	_, c := amountOf(a13, "1201")
	assert.Equal(t, "300000.00", d)
	assert.Equal(t, "300000.00", c)
	d, _ = amountOf(a13, "1314")
	_, c = amountOf(a13, "5101")
	assert.Equal(t, "120000.00", d, "HPP asal ke Persediaan Retur")
	assert.Equal(t, "120000.00", c)
	assertDims(t, a13, "modern_trade", subCode)

	// Nota kredit langsung mengurangi piutang invoice asal (1.000.000 − 300.000).
	cnRef := r["creditNote"].(map[string]any)
	assert.Regexp(t, `^CN/2026/11/\d{5}$`, cnRef["number"])
	assert.Equal(t, "completed", cnRef["status"])
	h.as("finance@balans.local")
	inv := h.json(http.MethodGet, "/api/v1/sales/invoices/"+invoiceID, nil, http.StatusOK)
	assert.Equal(t, "300000.00", inv["paidAmount"])
	assert.Equal(t, "partially_paid", inv["status"])
	cn := h.json(http.MethodGet, "/api/v1/sales/credit-notes/"+cnRef["id"].(string), nil, http.StatusOK)
	require.Len(t, items(cn, "applications"), 1)
	assert.Equal(t, "0.00", cn["remainingAmount"])

	// Sisa qty yang bisa diretur memperhitungkan retur ini.
	h.as("sales.mt@balans.local")
	src = h.json(http.MethodGet, "/api/v1/sales/return-sources/"+so["id"].(string), nil, http.StatusOK)
	assert.Equal(t, "7.000", items(src, "lines")[0]["availableQty"])
	n := h.scalar(`SELECT count(*)::text FROM audit_logs WHERE entity_type = 'sales_return' AND entity_id = $1`, id)
	assert.NotEqual(t, "0", n)
}

// --- S-314 retur pelanggan WA langsung Inbound, refund disetujui Head F&A dan dibayar F&A ---------------

func TestSalesReturnCustomerRefund(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 300, 20, "40000")
	reason := h.salesReturnReason()

	h.as("cs@balans.local")
	phone := fmt.Sprintf("0812%08d", time.Now().UnixNano()%100000000)
	cust := h.customer("Pelanggan "+h.tag, phone, "retail", "")
	wa := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "social_commerce", "channelId": h.channel("social_commerce"), "subChannelId": h.subChannel("social_commerce", "whatsapp"),
		"customerId": cust, "businessDate": "2026-11-02", "sourceLocationId": h.fg,
		"lines": []map[string]any{{"productId": coklat, "qty": "2"}},
	}, http.StatusCreated)
	wa = h.json(http.MethodPost, "/api/v1/sales/orders/"+wa["id"].(string)+"/confirm", map[string]any{"version": version(wa)}, http.StatusOK)
	paid := h.confirmPayment(wa)["salesOrder"].(map[string]any)
	h.shipAll(paid["id"].(string))

	h.as("cs@balans.local")
	r := h.json(http.MethodPost, "/api/v1/sales/returns", map[string]any{
		"salesOrderId": paid["id"], "returnDate": "2026-11-02", "settlementMethod": "refund",
		"lines": []map[string]any{{"salesOrderLineId": items(paid, "lines")[0]["id"], "qty": "1", "returnReasonId": reason}},
	}, http.StatusCreated)
	assert.Equal(t, false, r["requiresVerification"])
	r = h.json(http.MethodPost, "/api/v1/sales/returns/"+r["id"].(string)+"/submit", map[string]any{"version": version(r)}, http.StatusOK)
	require.Equal(t, "in_progress", r["status"], "retur pelanggan langsung masuk Inbound")
	h.qcInbound(r["inboundId"].(string), "")

	h.as("cs@balans.local")
	r = h.json(http.MethodGet, "/api/v1/sales/returns/"+r["id"].(string), nil, http.StatusOK)
	require.Equal(t, "posted", r["status"])
	a13 := h.journal("A-13", "sales_return", r["id"])
	d, _ := amountOf(a13, "4103")
	assert.Equal(t, "100000.00", d)
	assertDims(t, a13, "social_commerce", "whatsapp")
	cnRef := r["creditNote"].(map[string]any)
	assert.Equal(t, "pending_approval", cnRef["status"])
	cnID := cnRef["id"].(string)

	// Head F&A menyetujui refund.
	h.as("head.finance@balans.local")
	list := h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=sales_refund&subjectId="+cnID, nil, http.StatusOK)
	req := items(list, "items")
	require.Len(t, req, 1)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req[0]["id"].(string)+"/approve", map[string]any{"version": req[0]["version"]}, http.StatusOK)

	// F&A membayar refund (re-autentikasi wajib).
	h.as("finance@balans.local")
	cn := h.json(http.MethodGet, "/api/v1/sales/credit-notes/"+cnID, nil, http.StatusOK)
	require.Equal(t, "awaiting_payment", cn["status"])
	assert.Contains(t, cn["allowedActions"], "pay_refund")
	h.actor.ReauthAt = time.Time{}
	h.json(http.MethodPost, "/api/v1/sales/credit-notes/"+cnID+"/pay-refund", map[string]any{
		"version": cn["version"], "paymentDate": "2026-11-02", "accountCode": "1111",
	}, http.StatusUnauthorized)
	h.as("finance@balans.local")
	cn = h.json(http.MethodPost, "/api/v1/sales/credit-notes/"+cnID+"/pay-refund", map[string]any{
		"version": cn["version"], "paymentDate": "2026-11-02", "accountCode": "1111", "reference": "TRF-REFUND",
	}, http.StatusOK)
	assert.Equal(t, "paid", cn["status"])
	assert.Equal(t, "100000.00", cn["refundedAmount"])
	pb := h.journal("A-PB", "sales_refund", cnID)
	d, _ = amountOf(pb, "1201")
	_, c := amountOf(pb, "1111")
	assert.Equal(t, "100000.00", d, "refund: Dr Piutang Usaha (saldo kredit pelanggan ditutup)")
	assert.Equal(t, "100000.00", c, "Cr Kas/Bank")
}

// --- Refund ditolak → saldo nota kredit dialokasikan ke invoice berikutnya --------------------------------

func TestSalesReturnRefundRejectedBecomesCredit(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("COK", "100000")
	h.receive(coklat, h.fg, "C1", 300, 20, "40000")
	partner, sub := h.partnerWithSubChannel("b2b", "b2b")
	reason := h.salesReturnReason()
	order := func(qty string) map[string]any {
		h.as("sales.mt@balans.local")
		o := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
			"orderType": "b2b", "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
			"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "credit", "paymentTermDays": 30,
			"lines": []map[string]any{{"productId": coklat, "qty": qty}},
		}, http.StatusCreated)
		return h.json(http.MethodPost, "/api/v1/sales/orders/"+o["id"].(string)+"/confirm", map[string]any{"version": version(o)}, http.StatusOK)
	}
	first := order("2")
	h.shipAll(first["id"].(string))
	// Invoice pertama lunas.
	h.as("finance@balans.local")
	h.json(http.MethodPost, "/api/v1/sales/customer-payments", map[string]any{
		"partyType": "partner", "partyId": partner, "paymentDate": "2026-11-02", "amount": "200000", "method": "bank_transfer",
		"accountCode": "1111", "allocations": []map[string]any{{"invoiceId": items(first, "invoices")[0]["id"], "amount": "200000"}},
	}, http.StatusCreated)

	h.as("sales.mt@balans.local")
	r := h.json(http.MethodPost, "/api/v1/sales/returns", map[string]any{
		"salesOrderId": first["id"], "returnDate": "2026-11-02", "settlementMethod": "refund",
		"lines": []map[string]any{{"salesOrderLineId": items(first, "lines")[0]["id"], "qty": "1", "returnReasonId": reason}},
	}, http.StatusCreated)
	base := "/api/v1/sales/returns/" + r["id"].(string)
	r = h.json(http.MethodPost, base+"/submit", map[string]any{"version": version(r)}, http.StatusOK)
	h.as("logistik.leader@balans.local")
	r = h.json(http.MethodPost, base+"/verify", map[string]any{"version": version(r), "pickupRequired": false}, http.StatusOK)
	h.as("sales.mt@balans.local")
	r = h.json(http.MethodPost, base+"/receive", map[string]any{"version": version(r), "documentationFileIds": []string{h.photo("form.png")}}, http.StatusOK)
	h.qcInbound(r["inboundId"].(string), "")
	h.as("sales.mt@balans.local")
	r = h.json(http.MethodGet, base, nil, http.StatusOK)
	cnID := r["creditNote"].(map[string]any)["id"].(string)

	h.as("head.finance@balans.local")
	req := items(h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=sales_refund&subjectId="+cnID, nil, http.StatusOK), "items")
	require.Len(t, req, 1)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req[0]["id"].(string)+"/reject",
		map[string]any{"version": req[0]["version"], "note": "Potong tagihan berikutnya"}, http.StatusOK)

	h.as("finance@balans.local")
	cn := h.json(http.MethodGet, "/api/v1/sales/credit-notes/"+cnID, nil, http.StatusOK)
	assert.Equal(t, "credit_note", cn["settlementMethod"])
	assert.Equal(t, "posted", cn["status"], "invoice asal sudah lunas → saldo tetap")
	assert.Equal(t, "100000.00", cn["remainingAmount"])

	second := order("3")
	inv2 := items(second, "invoices")[0]["id"].(string)
	h.as("finance@balans.local")
	cn = h.json(http.MethodGet, "/api/v1/sales/credit-notes/"+cnID, nil, http.StatusOK)
	require.Contains(t, cn["allowedActions"], "apply")
	require.Len(t, items(cn, "openInvoices"), 1)
	h.json(http.MethodPost, "/api/v1/sales/credit-notes/"+cnID+"/apply", map[string]any{
		"version": cn["version"], "allocations": []map[string]any{{"invoiceId": inv2, "amount": "150000"}},
	}, http.StatusUnprocessableEntity)
	cn = h.json(http.MethodPost, "/api/v1/sales/credit-notes/"+cnID+"/apply", map[string]any{
		"version": cn["version"], "allocations": []map[string]any{{"invoiceId": inv2, "amount": "100000"}},
	}, http.StatusOK)
	assert.Equal(t, "completed", cn["status"])
	inv := h.json(http.MethodGet, "/api/v1/sales/invoices/"+inv2, nil, http.StatusOK)
	assert.Equal(t, "100000.00", inv["paidAmount"])
}
