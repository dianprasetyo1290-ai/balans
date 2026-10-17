package app

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// consignmentPartner membuat mitra konsinyasi beserta sub-channel di channel konsinyasi & vending, lokasi
// konsinyasi & buffer vending, dan Merchandiser yang ditugaskan.
func (h *salesHarness) consignmentPartner() (partnerID, consLoc, bufferLoc string) {
	h.t.Helper()
	h.as("admin@balans.local")
	p := h.json(http.MethodPost, "/api/v1/partners", map[string]any{
		"code": "K" + h.tag, "name": "Toko Sehat " + h.tag, "partnerType": "consignment", "address": "Jl. Thamrin 9, Jakarta",
	}, http.StatusCreated)
	partnerID = p["id"].(string)
	for _, ch := range []string{"consignment", "vending"} {
		h.exec(`INSERT INTO sub_channels (channel_id, code, name, party_type, party_id, created_by, updated_by)
			SELECT id, $2, $3, 'partner', $4, $5, $5 FROM channels WHERE code = $1`,
			ch, strings.ToLower("k"+h.tag+ch[:3]), "Toko Sehat "+h.tag, partnerID, systemUser)
	}
	h.exec(`UPDATE partners SET merchandiser_user_id = (SELECT id FROM users WHERE email = 'merchandiser@balans.local') WHERE id = $1`, partnerID)
	loc := func(code, t string) string {
		l := h.json(http.MethodPost, "/api/v1/locations", map[string]any{
			"code": code + h.tag, "name": t + " " + h.tag, "locationType": t,
			"externalPartyType": "partner", "externalPartyId": partnerID, "isActive": true,
		}, http.StatusCreated)
		return l["id"].(string)
	}
	return partnerID, loc("KS", "consignment"), loc("BV", "vending_buffer")
}

// upload mengunggah file kecil untuk jenis pemilik tertentu dan mengembalikan ID file.
func (h *salesHarness) upload(ownerType, name string) string {
	h.t.Helper()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	target := h.json(http.MethodPost, "/api/v1/files", map[string]any{
		"ownerType": ownerType, "fileName": name, "contentType": "image/png", "sizeBytes": len(png),
	}, http.StatusCreated)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, h.srv.URL+target["uploadUrl"].(string), bytes.NewReader(png))
	require.NoError(h.t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	require.NoError(h.t, res.Body.Close())
	require.Equal(h.t, http.StatusNoContent, res.StatusCode)
	return target["fileId"].(string)
}

// consignmentCount: Merchandiser menghitung stok lokasi konsinyasi dari HP (foto wajib) → Selesai.
func (h *salesHarness) consignmentCount(location string, counted map[uuid.UUID]string) string {
	h.t.Helper()
	h.as("merchandiser@balans.local")
	created := h.json(http.MethodPost, "/api/v1/stock-counts", map[string]any{"countType": "consignment", "locationIds": []any{location}}, http.StatusCreated)
	id := created["items"].([]any)[0].(map[string]any)["id"].(string)
	sc := h.json(http.MethodGet, "/api/v1/stock-counts/"+id, nil, http.StatusOK)
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "start", "version": sc["version"]}, http.StatusOK)
	var entries []any
	for _, l := range items(sc, "lines") {
		pid := uuid.MustParse(l["product"].(map[string]any)["id"].(string))
		entries = append(entries, map[string]any{"lineId": l["id"], "countedQty": counted[pid]})
	}
	photo := h.upload(warehouse.FileOwnerCount, "rak.png")
	sc = h.json(http.MethodPut, "/api/v1/stock-counts/"+id+"/entries", map[string]any{"lines": entries, "photoFileIds": []any{photo}}, http.StatusOK)
	sc = h.json(http.MethodPost, "/api/v1/stock-counts/"+id+"/actions", map[string]any{"action": "submit", "version": sc["version"]}, http.StatusOK)
	require.Equal(h.t, "completed", sc["status"], "stock count konsinyasi tidak membuat penyesuaian")
	return id
}

func lineOf(t *testing.T, rec map[string]any, product uuid.UUID) map[string]any {
	t.Helper()
	for _, l := range items(rec, "lines") {
		if l["product"].(map[string]any)["id"] == product.String() {
			return l
		}
	}
	t.Fatalf("baris produk %s tidak ada", product)
	return nil
}

func (h *salesHarness) approve(subjectType, subjectID, email string) {
	h.t.Helper()
	h.as(email)
	list := h.json(http.MethodGet, "/api/v1/approval-requests?subjectType="+subjectType+"&subjectId="+subjectID, nil, http.StatusOK)
	req := items(list, "items")
	require.NotEmpty(h.t, req)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+req[0]["id"].(string)+"/approve", map[string]any{"version": req[0]["version"]}, http.StatusOK)
}

// TestConsignmentDeliveryReconciliationFlow: S-303 (A-07, A-24), S-304, S-305, S-313 konsinyasi end-to-end:
// DO konsinyasi butuh PKS → stok ke lokasi mitra → stock count Merchandiser → rekonsiliasi (laporan mitra,
// selisih per penyebab, BA) → approval Sales MT → Head F&A → invoice konsinyasi + invoice selisih + jurnal
// A-08/A-23 balance dengan dimensi channel konsinyasi; periode berikutnya memakai sisa fisik sebagai stok awal.
func TestConsignmentDeliveryReconciliationFlow(t *testing.T) {
	h := newSalesHarness(t)
	coklat := h.product("KCO", "100000")
	vanila := h.product("KVA", "50000")
	h.receive(coklat, h.fg, "KC1", 300, 100, "40000")
	h.receive(vanila, h.fg, "KV1", 300, 20, "20000")
	partner, consLoc, bufferLoc := h.consignmentPartner()
	consID, bufferID := uuid.MustParse(consLoc), uuid.MustParse(bufferLoc)

	delivery := func(deliveryType string, lines []map[string]any) (int, map[string]any) {
		h.as("sales.mt@balans.local")
		st, out, _ := h.do(http.MethodPost, "/api/v1/sales/consignment-deliveries", map[string]any{
			"deliveryType": deliveryType, "partnerId": partner, "fromLocationId": h.fg, "businessDate": "2026-11-02", "lines": lines,
		})
		return st, out
	}
	// Tanpa PKS konsinyasi berlaku → diblokir.
	st, problem := delivery("consignment", []map[string]any{{"productId": coklat, "qty": "50"}})
	require.Equal(t, http.StatusUnprocessableEntity, st)
	assert.Contains(t, problem["fields"], "partnerId")

	// PKS konsinyasi: daftar harga mitra (coklat Rp70.000), margin 20% untuk SKU tanpa harga PKS, termin 30 hari.
	priceList := h.scalar(`INSERT INTO price_lists (code, name, price_list_type, partner_id, created_by, updated_by)
		VALUES ($1, 'Harga Konsinyasi', 'partner', $2, $3, $3) RETURNING id::text`, "KSN"+h.tag, partner, systemUser)
	version := h.scalar(`INSERT INTO price_list_versions (price_list_id, version_no, valid_from, status, created_by, updated_by)
		VALUES ($1, 1, DATE '2026-01-01', 'active', $2, $2) RETURNING id::text`, priceList, systemUser)
	h.exec(`INSERT INTO price_list_items (price_list_version_id, product_id, price_amount, created_by) VALUES ($1, $2, 70000, $3)`, version, coklat, systemUser)
	h.exec(`INSERT INTO partner_agreements (partner_id, agreement_number, agreement_type, valid_from, valid_to, price_list_id,
		consignment_margin_percent, payment_term_days, created_by, updated_by)
		VALUES ($1, $2, 'consignment', DATE '2026-01-01', DATE '2027-12-31', $3, 20, 30, $4, $4)`, partner, "PKS-"+h.tag, priceList, systemUser)

	st, d := delivery("consignment", []map[string]any{{"productId": coklat, "qty": "50"}, {"productId": vanila, "qty": "10"}})
	require.Equal(t, http.StatusCreated, st, d)
	assert.Equal(t, "draft", d["status"])
	assert.Equal(t, "PKS-"+h.tag, d["agreementNumber"])
	assert.Equal(t, consLoc, d["toLocation"].(map[string]any)["id"])
	assert.ElementsMatch(t, []any{"edit", "cancel"}, d["allowedActions"], "Sales MT tidak mengirim fisik")

	// Logistik mengirim: TRF Dalam Pengiriman + A-07 (Dr 1312, Cr 1311) nilai FIFO.
	h.as("logistik.leader@balans.local")
	d = h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+d["id"].(string)+"/ship", map[string]any{"version": d["version"]}, http.StatusOK)
	require.Equal(t, "in_transit", d["status"])
	assert.Regexp(t, `^DO/2026/11/\d{5}$`, d["number"])
	assert.Regexp(t, `^TRF/`, d["transferNumber"])
	assert.Equal(t, "2200000.00", d["costAmount"])
	a07 := h.journal("A-07", "consignment_delivery", d["id"])
	dr, _ := amountOf(a07, "1312")
	_, cr := amountOf(a07, "1311")
	assert.Equal(t, "2200000.00", dr)
	assert.Equal(t, "2200000.00", cr)
	consSub := strings.ToLower("k" + h.tag + "con")
	assertDims(t, a07, "consignment", consSub)
	assert.Equal(t, "50", h.available(coklat, h.fg))
	d = h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+d["id"].(string)+"/receive", map[string]any{"version": d["version"]}, http.StatusOK)
	assert.Equal(t, "completed", d["status"])
	assert.Equal(t, "50", h.available(coklat, consID))
	assert.Equal(t, "10", h.available(vanila, consID))

	// Buffer vending (A-24, Dr 1313 Cr 1311) lewat API logistics (integrasi DO) — idempotent per DO.
	ctx := actor.System(context.Background())
	doID := uuid.New()
	in := sales.ConsignmentShipmentInput{
		Type: sales.DeliveryVendingBuffer, PartnerID: uuid.MustParse(partner), SourceType: "delivery_order", SourceID: doID,
		SourceLabel: "DO/2026/11/09999", FromLocationID: h.fg,
		Lines: []sales.DeliveryLineInput{{ProductID: coklat, Qty: dec("5")}},
	}
	buf, err := h.app.sales.ShipConsignment(ctx, in)
	require.NoError(t, err)
	again, err := h.app.sales.ShipConsignment(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, buf.ID, again.ID)
	a24 := h.journal("A-24", "consignment_delivery", buf.ID)
	dr, _ = amountOf(a24, "1313")
	assert.Equal(t, "200000.00", dr)
	assertDims(t, a24, "vending", strings.ToLower("k"+h.tag+"ven"))
	_, err = h.app.sales.ConfirmConsignmentReceived(ctx, buf.ID)
	require.NoError(t, err)
	assert.Equal(t, "5", h.available(coklat, bufferID))

	// Stock count Merchandiser: sisa fisik coklat 30, vanila 6.
	count := h.consignmentCount(consLoc, map[uuid.UUID]string{coklat: "30", vanila: "6"})

	// Rekonsiliasi: cut-off hari ini, stock count lokasi mitra.
	h.as("sales.mt@balans.local")
	rec := h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations", map[string]any{
		"partnerId": partner, "cutOffDate": "2026-11-02", "stockCountId": count,
	}, http.StatusCreated)
	assert.Equal(t, "draft", rec["status"])
	rid := rec["id"].(string)
	rec = h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations/"+rid+"/start", map[string]any{"version": rec["version"]}, http.StatusOK)
	require.Equal(t, "in_progress", rec["status"])
	assert.Regexp(t, `^KSN/2026/11/\d{5}$`, rec["number"])
	assert.Equal(t, "2026-11-01", rec["periodStart"])
	assert.Equal(t, 30, int(rec["paymentTermDays"].(float64)))
	cl := lineOf(t, rec, coklat)
	assert.Equal(t, "0.000", cl["openingQty"])
	assert.Equal(t, "50.000", cl["deliveredQty"])
	assert.Equal(t, "30.000", cl["countedQty"])
	assert.Equal(t, "20.000", cl["soldQty"], "terjual = awal + kiriman − tarikan − sisa fisik")
	assert.Equal(t, "70000.00", cl["unitPrice"], "harga konsinyasi dari daftar harga PKS")
	vl := lineOf(t, rec, vanila)
	assert.Equal(t, "4.000", vl["soldQty"])
	assert.Equal(t, "40000.00", vl["unitPrice"], "tanpa harga PKS: harga dasar − margin mitra 20%")

	// Laporan mitra: coklat 17 (3 hilang → ditagih), vanila 2 (2 cacat produk → ditanggung Balans).
	entries := func(coklatQty, coklatCause string) map[string]any {
		return map[string]any{
			"version": rec["version"], "lines": []any{
				map[string]any{"lineId": cl["id"], "reportedQty": coklatQty, "varianceCause": nilIfEmpty(coklatCause)},
				map[string]any{"lineId": vl["id"], "reportedQty": "2", "varianceCause": "product_defect"},
			},
		}
	}
	h.json(http.MethodPut, "/api/v1/sales/consignment-reconciliations/"+rid+"/entries", entries("21", ""), http.StatusUnprocessableEntity)
	rec = h.json(http.MethodPut, "/api/v1/sales/consignment-reconciliations/"+rid+"/entries", entries("17", ""), http.StatusOK)
	st, problem, _ = h.do(http.MethodPost, "/api/v1/sales/consignment-reconciliations/"+rid+"/submit", map[string]any{"version": rec["version"]})
	require.Equal(t, http.StatusUnprocessableEntity, st)
	assert.Contains(t, problem["fields"], "baFileId")
	assert.Contains(t, problem["fields"], "lines.0.varianceCause")
	ba := h.upload(sales.FileOwnerConsignmentBA, "ba-ksn.png")
	report := h.upload(sales.FileOwnerConsignmentReport, "laporan-mitra.png")
	body := entries("17", "lost")
	body["setFiles"], body["baFileId"], body["partnerReportFileId"] = true, ba, report
	rec = h.json(http.MethodPut, "/api/v1/sales/consignment-reconciliations/"+rid+"/entries", body, http.StatusOK)
	cl = lineOf(t, rec, coklat)
	assert.Equal(t, "3.000", cl["varianceQty"])
	assert.Equal(t, "partner", cl["borneBy"], "hilang ditagih ke mitra (default PKS)")
	assert.Equal(t, "balans", lineOf(t, rec, vanila)["borneBy"], "cacat produk ditanggung Balans")
	assert.Equal(t, "1270000.00", rec["salesAmount"])
	assert.Equal(t, "210000.00", rec["billedVarianceAmount"])
	assert.Equal(t, "ba-ksn.png", rec["baFileName"])

	// Level 1 Sales MT: pembuat tidak boleh menyetujui sendiri → pengganti (Head Sales) diatur di Matriks Persetujuan.
	h.exec(`UPDATE approval_policies SET substitute_user_id = (SELECT id FROM users WHERE email = 'head.sales@balans.local')
		WHERE subject_type = 'consignment_reconciliation' AND level = 1`)
	rec = h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations/"+rid+"/submit", map[string]any{"version": rec["version"]}, http.StatusOK)
	require.Equal(t, "pending_approval", rec["status"])
	h.approve("consignment_reconciliation", rid, "head.sales@balans.local")
	h.approve("consignment_reconciliation", rid, "head.finance@balans.local")

	h.as("sales.mt@balans.local")
	rec = h.json(http.MethodGet, "/api/v1/sales/consignment-reconciliations/"+rid, nil, http.StatusOK)
	require.Equal(t, "posted", rec["status"])
	assert.Equal(t, "840000.00", rec["costAmount"], "HPP terjual 17×40.000 + 2×20.000 + selisih ditagih 3×40.000")
	assert.Equal(t, "40000.00", rec["absorbedVarianceCost"])
	assert.Equal(t, "30", h.available(coklat, consID), "terjual + selisih keluar dari lokasi mitra")
	assert.Equal(t, "6", h.available(vanila, consID))

	// Invoice konsinyasi (termin PKS) dan invoice selisih ditagih ke mitra.
	h.as("finance@balans.local")
	inv := h.json(http.MethodGet, "/api/v1/sales/invoices/"+rec["invoiceId"].(string), nil, http.StatusOK)
	assert.Equal(t, "consignment", inv["invoiceType"])
	assert.Equal(t, "1270000.00", inv["totalAmount"])
	assert.Equal(t, clock.Today(h.clock).AddDays(30).String(), inv["dueDate"])
	assert.Equal(t, "awaiting_payment", inv["status"])
	vinv := h.json(http.MethodGet, "/api/v1/sales/invoices/"+rec["varianceInvoiceId"].(string), nil, http.StatusOK)
	assert.Equal(t, "variance", vinv["invoiceType"])
	assert.Equal(t, "210000.00", vinv["totalAmount"])

	// A-08: Dr 1201 / Cr 4101 1.270.000; Dr 5101 / Cr 1312 HPP 720.000.
	a08 := h.journal("A-08", "consignment_reconciliation", rid)
	dr, _ = amountOf(a08, "1201")
	_, cr = amountOf(a08, "4101")
	assert.Equal(t, "1270000.00", dr)
	assert.Equal(t, "1270000.00", cr)
	dr, _ = amountOf(a08, "5101")
	_, cr = amountOf(a08, "1312")
	assert.Equal(t, "720000.00", dr)
	assert.Equal(t, "720000.00", cr)
	assertDims(t, a08, "consignment", consSub)
	// A-23: ditagih seperti A-08 (210.000 + HPP 120.000), ditanggung Balans Dr 6404 Cr 1312 40.000.
	a23 := h.journal("A-23", "consignment_reconciliation", rid)
	dr, _ = amountOf(a23, "1201")
	assert.Equal(t, "210000.00", dr)
	dr, _ = amountOf(a23, "6404")
	assert.Equal(t, "40000.00", dr)
	_, cr = amountOf(a23, "1312")
	assert.Equal(t, "160000.00", cr)
	assertDims(t, a23, "consignment", consSub)
	assert.Equal(t, "1320000.00", h.scalar(`SELECT (sum(l.debit_amount) - sum(l.credit_amount))::numeric(18,2)::text
		FROM journal_lines l JOIN accounts a ON a.id = l.account_id JOIN journal_entries e ON e.id = l.journal_entry_id
		WHERE a.code = '1312' AND e.source_id IN ($1::uuid, $2::uuid)`, d["id"], rid),
		"saldo Persediaan FG – Konsinyasi 1312 = nilai sisa fisik 30×40.000 + 6×20.000")

	// Periode berikutnya: stok awal = sisa fisik periode sebelumnya.
	h.clock.Advance(28 * 24 * time.Hour)
	count2 := h.consignmentCount(consLoc, map[uuid.UUID]string{coklat: "25", vanila: "6"})
	h.as("sales.mt@balans.local")
	rec2 := h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations", map[string]any{
		"partnerId": partner, "cutOffDate": "2026-11-30", "stockCountId": count2,
	}, http.StatusCreated)
	rec2 = h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations/"+rec2["id"].(string)+"/start", map[string]any{"version": rec2["version"]}, http.StatusOK)
	assert.Equal(t, "2026-11-03", rec2["periodStart"])
	c2 := lineOf(t, rec2, coklat)
	assert.Equal(t, "30.000", c2["openingQty"])
	assert.Equal(t, "0.000", c2["deliveredQty"])
	assert.Equal(t, "5.000", c2["soldQty"])
	rec2 = h.json(http.MethodPost, "/api/v1/sales/consignment-reconciliations/"+rec2["id"].(string)+"/cancel",
		map[string]any{"version": rec2["version"], "reason": "Uji batal"}, http.StatusOK)
	assert.Equal(t, "cancelled", rec2["status"])

	list := h.json(http.MethodGet, "/api/v1/sales/consignment-reconciliations?partnerId="+partner, nil, http.StatusOK)
	assert.EqualValues(t, 2, list["totalItems"])
	dl := h.json(http.MethodGet, "/api/v1/sales/consignment-deliveries?partnerId="+partner, nil, http.StatusOK)
	assert.EqualValues(t, 2, dl["totalItems"])
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
