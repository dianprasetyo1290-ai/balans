package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/warehouse"
)

// TestOutboundQCRejectReturnsToSupplier: barang dari Penerimaan Barang maklon yang ditolak QC Outbound langsung
// diretur dari Karantina tanpa approval — retur pembelian Diposting, nota debit, jurnal A-02 (S-311, D-24).
func TestOutboundQCRejectReturnsToSupplier(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("COK", "100000")
	inbound := h.scalar(`SELECT id::text FROM locations WHERE code = 'UTAMA-INB'`)
	division := h.scalar(`SELECT id::text FROM divisions WHERE code = 'LOG'`)

	h.as("admin@balans.local")
	sup := h.json(http.MethodPost, "/api/v1/suppliers", map[string]any{
		"code": "MK" + h.tag, "name": "Maklon " + h.tag, "supplierType": "toll_manufacturer", "isBusinessEntity": true,
		"defaultWithholding": "none", "paymentTermDays": 30, "bankAccounts": []any{},
	}, http.StatusCreated)
	h.as("purchasing@balans.local")
	po := h.json(http.MethodPost, "/api/v1/purchase-orders", map[string]any{
		"orderType": "goods", "supplierId": sup["id"], "divisionId": division, "businessDate": "2026-11-02", "expectedDate": "2026-11-03",
		"lines": []map[string]any{{"productId": coklat, "qty": "10", "unitPrice": "40000"}},
	}, http.StatusCreated)
	po = h.json(http.MethodPost, "/api/v1/purchase-orders/"+po["id"].(string)+"/submit", map[string]any{"version": po["version"]}, http.StatusOK)
	h.as("head.finance@balans.local")
	reqs := items(h.json(http.MethodGet, "/api/v1/approval-requests?subjectType=purchase_order&subjectId="+po["id"].(string), nil, http.StatusOK), "items")
	require.NotEmpty(t, reqs)
	h.json(http.MethodPost, "/api/v1/approval-requests/"+reqs[0]["id"].(string)+"/approve", map[string]any{"version": reqs[0]["version"]}, http.StatusOK)

	h.as("purchasing@balans.local")
	po = h.json(http.MethodGet, "/api/v1/purchase-orders/"+po["id"].(string), nil, http.StatusOK)
	gr := h.json(http.MethodPost, "/api/v1/goods-receipts", map[string]any{
		"purchaseOrderId": po["id"], "businessDate": "2026-11-02", "locationId": inbound, "deliveryNote": "SJ-" + h.tag,
		"lines": []map[string]any{{
			"purchaseOrderLineId": items(po, "lines")[0]["id"], "goodQty": "10", "batchNumber": h.tag + "MK1",
			"productionDate": "2026-10-01", "expiryDate": "2027-10-01",
		}},
	}, http.StatusCreated)
	gr = h.json(http.MethodPost, "/api/v1/goods-receipts/"+gr["id"].(string)+"/post", map[string]any{"version": gr["version"]}, http.StatusOK)
	ib, err := h.app.warehouse.InboundBySource(context.Background(), warehouse.SourceGoodsReceipt, uuid.MustParse(gr["id"].(string)))
	require.NoError(t, err)
	h.as("staf.gudang@balans.local")
	h.json(http.MethodPost, "/api/v1/inbounds/"+ib.ID.String()+"/lines/"+ib.Lines[0].ID.String()+"/qc", map[string]any{
		"passedQty": "10", "rejectedQty": "0", "checks": okQC,
	}, http.StatusOK)
	require.Equal(t, "10", h.available(coklat, h.fg))

	partner, sub := h.partnerWithSubChannel("b2b", "b2b")
	h.as("sales.mt@balans.local")
	so := h.json(http.MethodPost, "/api/v1/sales/orders", map[string]any{
		"orderType": "b2b", "channelId": h.channel("b2b"), "subChannelId": sub, "partnerId": partner,
		"businessDate": "2026-11-02", "sourceLocationId": h.fg, "paymentMode": "prepaid",
		"lines": []map[string]any{{"productId": coklat, "qty": "4"}},
	}, http.StatusCreated)
	so = h.json(http.MethodPost, "/api/v1/sales/orders/"+so["id"].(string)+"/confirm", map[string]any{"version": version(so)}, http.StatusOK)
	so = h.confirmPayment(so)["salesOrder"].(map[string]any)

	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodPost, "/api/v1/delivery-orders", map[string]any{"salesOrderId": so["id"]}, http.StatusCreated)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	obID := do["outbound"].(map[string]any)["id"].(string)
	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	out = h.packAll(out)
	h.as("fulfillment.leader@balans.local")
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+items(out, "lines")[0]["id"].(string)+"/qc", map[string]any{
		"passedQty": "2", "rejectedQty": "2", "checks": okQC, "rejectReason": "Isi kurang",
	}, http.StatusOK)
	rejected := items(out, "lines")[0]
	assert.Regexp(t, `^RB/2026/11/\d{5}$`, rejected["returnLabel"], "retur pembelian otomatis")
	assert.Equal(t, "0", h.quarantine(coklat, h.fg), "barang reject keluar dari Karantina ke maklon")
	assert.Equal(t, "4", h.reserved(coklat, h.fg), "stok pengganti FEFO direservasi")
	ret := h.scalar(`SELECT status || '|' || coalesce(debit_note_number, '') || '|' || origin FROM purchase_returns WHERE number = $1`, rejected["returnLabel"])
	parts := strings.Split(ret, "|")
	assert.Equal(t, "posted", parts[0], "tanpa approval (D-24)")
	assert.Regexp(t, `^DN/`, parts[1])
	assert.Equal(t, "qc_outbound", parts[2])
	retID := h.scalar(`SELECT id::text FROM purchase_returns WHERE number = $1`, rejected["returnLabel"])
	h.journal("A-02", "purchase_return", retID)
}
