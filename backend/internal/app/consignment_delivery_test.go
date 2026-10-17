package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/actor"
	"balans/internal/sales"
)

// submitConsignment: Sales MT mengajukan pengiriman titipan (reservasi FEFO + DO Draft), lalu Logistik Leader
// me-release DO (nomor DO + Outbound). Mengembalikan pengiriman (Diproses) dan DO hasil release.
func (h *salesHarness) submitConsignment(d map[string]any) (map[string]any, map[string]any) {
	h.t.Helper()
	h.as("sales.mt@balans.local")
	d = h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+d["id"].(string)+"/submit", map[string]any{"version": d["version"]}, http.StatusOK)
	require.Equal(h.t, "in_progress", d["status"])
	assert.Empty(h.t, d["allowedActions"], "Diproses: dikerjakan Logistik dari halaman DO")
	ref := d["deliveryOrder"].(map[string]any)
	assert.Equal(h.t, "draft", ref["status"])
	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodGet, "/api/v1/delivery-orders/"+ref["id"].(string), nil, http.StatusOK)
	assert.Equal(h.t, "consignment_delivery", do["sourceType"])
	assert.Contains(h.t, do["allowedActions"], "release")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+ref["id"].(string)+"/release", map[string]any{"version": do["version"]}, http.StatusOK)
	require.Equal(h.t, "in_progress", do["status"])
	assert.Regexp(h.t, `^DO/2026/11/\d{5}$`, do["number"])
	return d, do
}

// passOutbound: Staf Gudang picking & Head Logistik packing, lalu Fulfillment Leader meluluskan QC semua baris
// yang sudah diambil.
func (h *salesHarness) passOutbound(obID string) map[string]any {
	h.t.Helper()
	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	h.as("head.logistik@balans.local")
	out = h.packAll(out)
	h.as("fulfillment.leader@balans.local")
	for _, l := range lineByStatus(out, "picked") {
		out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+l["id"].(string)+"/qc", map[string]any{
			"passedQty": l["pickedQty"], "rejectedQty": "0", "checks": okQC,
		}, http.StatusOK)
	}
	require.Equal(h.t, "completed", out["status"])
	return out
}

// shipDO: Logistik Leader menyerahkan ke kurir JNE (resi manual) dan, bila deliver, mengonfirmasi diterima.
func (h *salesHarness) shipDO(doID string, deliver bool) map[string]any {
	h.t.Helper()
	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodGet, "/api/v1/delivery-orders/"+doID, nil, http.StatusOK)
	require.Contains(h.t, do["allowedActions"], "ship")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("jne"), "trackingNumber": "JNE" + h.tag,
	}, http.StatusOK)
	require.Equal(h.t, "in_transit", do["status"])
	if deliver {
		do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{"receivedByName": "Kepala Toko"}, http.StatusOK)
		require.Equal(h.t, "received", do["status"])
	}
	return do
}

// TestConsignmentDeliveryOrderQCFlow: pengiriman konsinyasi lewat DO (S-303, S-309 – S-312): diajukan → stok FEFO
// reserved + DO konsinyasi → batal DO melepas reservasi (Draft lagi) → ajukan ulang → picking/packing → QC tolak
// (Karantina + pengganti FEFO dari reservasi) → QC pengganti → serah ke kurir internal Merchandiser (reserved →
// lokasi mitra Dalam Pengiriman, A-07 balance dengan dimensi konsinyasi + sub-channel mitra, sekali) → diterima
// (Tersedia di lokasi mitra); jalur kirim langsung lama ditolak dan pemanggilan ulang idempotent.
func TestConsignmentDeliveryOrderQCFlow(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("KCO", "100000")
	h.receive(coklat, h.fg, "R1", 60, 10, "40000")
	h.receive(coklat, h.fg, "R2", 300, 50, "42000")
	partner, consLoc, _ := h.consignmentPartner()
	consID := uuid.MustParse(consLoc)
	h.exec(`INSERT INTO partner_agreements (partner_id, agreement_number, agreement_type, valid_from, valid_to,
		consignment_margin_percent, payment_term_days, created_by, updated_by)
		VALUES ($1, $2, 'consignment', DATE '2026-01-01', DATE '2027-12-31', 20, 30, $3, $3)`, partner, "PKS-"+h.tag, systemUser)

	h.as("sales.mt@balans.local")
	d := h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries", map[string]any{
		"deliveryType": "consignment", "partnerId": partner, "fromLocationId": h.fg, "businessDate": "2026-11-02",
		"lines": []map[string]any{{"productId": coklat, "qty": "12"}},
	}, http.StatusCreated)
	id := d["id"].(string)
	// Jalur kirim langsung dari halaman Konsinyasi sudah tidak ada.
	st, _, _ := h.do(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+id+"/ship", map[string]any{"version": d["version"]})
	assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, st)
	h.as("logistik.leader@balans.local")
	st, _, _ = h.do(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+id+"/submit", map[string]any{"version": d["version"]})
	assert.Equal(t, http.StatusForbidden, st, "pengajuan oleh Sales MT")

	// Ajukan → reservasi FEFO (R1 10 + R2 2) + DO Draft jenis konsinyasi; batal DO → reservasi dilepas, Draft lagi.
	h.as("sales.mt@balans.local")
	d = h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+id+"/submit", map[string]any{"version": d["version"]}, http.StatusOK)
	require.Equal(t, "in_progress", d["status"])
	assert.Equal(t, "12", h.reserved(coklat, h.fg))
	assert.Equal(t, "48", h.available(coklat, h.fg))
	h.as("logistik.leader@balans.local")
	do := h.json(http.MethodGet, "/api/v1/delivery-orders/"+d["deliveryOrder"].(map[string]any)["id"].(string), nil, http.StatusOK)
	assert.Equal(t, "consignment", do["deliveryType"])
	assert.Equal(t, "Toko Sehat "+h.tag, do["recipientName"])
	require.Len(t, items(do, "lines"), 1)
	assert.Len(t, items(items(do, "lines")[0], "batches"), 2, "picking list per batch reserved")
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+do["id"].(string)+"/cancel", map[string]any{"version": do["version"], "reason": "Jadwal mitra mundur"}, http.StatusOK)
	assert.Equal(t, "cancelled", do["status"])
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "60", h.available(coklat, h.fg))
	h.as("sales.mt@balans.local")
	d = h.json(http.MethodGet, "/api/v1/sales/consignment-deliveries/"+id, nil, http.StatusOK)
	assert.Equal(t, "draft", d["status"])
	assert.Nil(t, d["deliveryOrder"])
	assert.ElementsMatch(t, []any{"edit", "submit", "cancel"}, d["allowedActions"])

	// Ajukan ulang → DO baru → release (nomor DO dari satu counter DO) → Outbound.
	d, do = h.submitConsignment(d)
	doID := do["id"].(string)
	assert.Nil(t, d["deliveryOrder"].(map[string]any)["number"], "nomor DO diberikan saat release")
	obID := do["outbound"].(map[string]any)["id"].(string)
	h.as("staf.gudang@balans.local")
	out := h.json(http.MethodGet, "/api/v1/outbounds/"+obID, nil, http.StatusOK)
	require.Len(t, items(out, "lines"), 2)
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/pick", map[string]any{"version": out["version"]}, http.StatusOK)
	h.as("head.logistik@balans.local")
	out = h.packAll(out)

	// QC tolak 3 unit R1 → Karantina; pengganti FEFO dari R2 langsung direservasi untuk baris pengiriman.
	h.as("fulfillment.leader@balans.local")
	r1 := items(out, "lines")[0]
	require.Equal(t, h.tag+"R1", r1["batch"].(map[string]any)["number"])
	out = h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+r1["id"].(string)+"/qc", map[string]any{
		"passedQty": "7", "rejectedQty": "3", "checks": map[string]any{"batch": true, "expiry": true, "condition": false, "seal": true},
		"rejectReason": "Kemasan penyok",
	}, http.StatusOK)
	assert.Equal(t, "3", h.quarantine(coklat, h.fg))
	assert.Equal(t, "12", h.reserved(coklat, h.fg))
	assert.Equal(t, "45", h.available(coklat, h.fg))
	pending := lineByStatus(out, "pending")
	require.Len(t, pending, 1)
	assert.Equal(t, "3.000", pending[0]["allocatedQty"])
	assert.Equal(t, h.tag+"R2", pending[0]["batch"].(map[string]any)["number"])
	r2 := lineByStatus(out, "picked")[0]
	h.json(http.MethodPost, "/api/v1/outbounds/"+obID+"/lines/"+r2["id"].(string)+"/qc", map[string]any{
		"passedQty": "2", "rejectedQty": "0", "checks": okQC,
	}, http.StatusOK)
	h.passOutbound(obID)

	// Serah ke kurir internal: Merchandiser dipilih dari daftar role Merchandiser.
	h.as("logistik.leader@balans.local")
	users := items(h.json(http.MethodGet, "/api/v1/courier-users", nil, http.StatusOK), "items")
	require.NotEmpty(t, users)
	merch := h.userID("merchandiser@balans.local")
	for _, u := range users {
		assert.NotEqual(t, h.userID("staf.gudang@balans.local"), u["id"], "hanya role Merchandiser")
	}
	assert.Contains(t, ids(users), merch)
	do = h.json(http.MethodGet, "/api/v1/delivery-orders/"+doID, nil, http.StatusOK)
	do = h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("own"), "internalCourierUserId": merch,
	}, http.StatusOK)
	require.Equal(t, "in_transit", do["status"])
	assert.Equal(t, "12.000", items(do, "lines")[0]["shippedQty"])

	// Stok reserved keluar ke lokasi mitra (Dalam Pengiriman); A-07 = 7 × 40.000 + 5 × 42.000.
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "45", h.available(coklat, h.fg))
	assert.Equal(t, "0", h.available(coklat, consID))
	h.as("sales.mt@balans.local")
	d = h.json(http.MethodGet, "/api/v1/sales/consignment-deliveries/"+id, nil, http.StatusOK)
	require.Equal(t, "in_transit", d["status"])
	assert.Equal(t, do["number"], d["number"])
	assert.Equal(t, "490000.00", d["costAmount"])
	shipped := map[string]string{}
	for _, l := range items(d, "lines") {
		require.NotNil(t, l["batchId"], "baris per batch yang dikirim")
		shipped[l["qty"].(string)+"@"+l["costAmount"].(string)] = l["batchId"].(string)
	}
	assert.Len(t, shipped, 3, "R1 7 + R2 2 (reservasi awal) + R2 3 (pengganti QC)")
	assert.Equal(t, "in_transit", d["deliveryOrder"].(map[string]any)["status"])
	assert.NotContains(t, d["allowedActions"], "receive", "diterima lewat DO")
	a07 := h.journal("A-07", "consignment_delivery", id)
	dr, _ := amountOf(a07, "1312")
	_, cr := amountOf(a07, "1311")
	assert.Equal(t, "490000.00", dr)
	assert.Equal(t, "490000.00", cr)
	assertDims(t, a07, "consignment", strings.ToLower("k"+h.tag+"con"))

	// Idempotent: DO ship dipanggil ulang tidak memindah stok / menjurnal lagi.
	ctx := actor.System(context.Background())
	again, err := h.app.sales.ShipConsignment(ctx, sales.ConsignmentShipmentInput{
		DeliveryID: uuid.MustParse(id), SourceType: sales.SourceDeliveryOrder, SourceID: uuid.MustParse(doID),
		SourceLabel: do["number"].(string),
		Lines:       []sales.ConsignmentShippedLine{{SourceLineID: uuid.MustParse(items(do, "lines")[0]["sourceLineId"].(string)), Qty: dec("1")}},
	})
	require.NoError(t, err)
	assert.Equal(t, sales.StatusInTransit, again.Status)
	assert.Equal(t, "1", h.scalar(`SELECT count(*)::text FROM journal_entries WHERE posting_rule_code = 'A-07' AND source_id = $1`, id))
	assert.Equal(t, "1", h.scalar(`SELECT count(*)::text FROM stock_transfers WHERE source_id = $1`, doID))
	st, _, _ = h.do(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/ship", map[string]any{
		"version": do["version"], "courierId": h.courier("jne"), "trackingNumber": "X",
	})
	assert.NotEqual(t, http.StatusOK, st)
	_, err = h.app.sales.ShipConsignment(ctx, sales.ConsignmentShipmentInput{
		DeliveryID: uuid.MustParse(id), SourceType: sales.SourceDeliveryOrder, SourceID: uuid.New(), SourceLabel: "DO lain",
	})
	require.Error(t, err, "DO lain tidak bisa mengirim pengiriman ini")

	// Konfirmasi diterima dari halaman DO (Merchandiser pengantar, foto wajib) → Tersedia di lokasi mitra.
	h.as("logistik.leader@balans.local")
	st, _, _ = h.do(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+id+"/receive", map[string]any{"version": d["version"]})
	assert.Equal(t, http.StatusConflict, st, "pengiriman lewat DO tidak dikonfirmasi dari halaman Konsinyasi")
	h.as("merchandiser@balans.local")
	h.json(http.MethodPost, "/api/v1/delivery-orders/"+doID+"/deliver", map[string]any{
		"receivedByName": "Kepala Toko", "proofFileIds": []string{h.uploadPhoto("delivery_proof")},
	}, http.StatusOK)
	assert.Equal(t, "12", h.available(coklat, consID))
	assert.Equal(t, "3", h.quarantine(coklat, h.fg))
	h.as("sales.mt@balans.local")
	d = h.json(http.MethodGet, "/api/v1/sales/consignment-deliveries/"+id, nil, http.StatusOK)
	assert.Equal(t, "completed", d["status"])
	_, err = h.app.sales.ConfirmConsignmentReceived(ctx, uuid.MustParse(id))
	require.NoError(t, err, "diterima idempotent")
	assert.Equal(t, "12", h.available(coklat, consID))
}

// TestConsignmentDeliveryStockShortage: stok lokasi asal kurang saat diajukan → STOCK_INSUFFICIENT tanpa DO.
func TestConsignmentDeliveryStockShortage(t *testing.T) {
	h := newLogisticsHarness(t)
	coklat := h.product("KCO", "100000")
	h.receive(coklat, h.fg, "S1", 300, 4, "40000")
	partner, _, _ := h.consignmentPartner()
	h.exec(`INSERT INTO partner_agreements (partner_id, agreement_number, agreement_type, valid_from, valid_to,
		consignment_margin_percent, payment_term_days, created_by, updated_by)
		VALUES ($1, $2, 'consignment', DATE '2026-01-01', DATE '2027-12-31', 20, 30, $3, $3)`, partner, "PKS-"+h.tag, systemUser)
	h.as("sales.mt@balans.local")
	d := h.json(http.MethodPost, "/api/v1/sales/consignment-deliveries", map[string]any{
		"deliveryType": "vending_buffer", "partnerId": partner, "fromLocationId": h.fg, "businessDate": "2026-11-02",
		"lines": []map[string]any{{"productId": coklat, "qty": "5"}},
	}, http.StatusCreated)
	st, problem, _ := h.do(http.MethodPost, "/api/v1/sales/consignment-deliveries/"+d["id"].(string)+"/submit", map[string]any{"version": d["version"]})
	require.Equal(t, http.StatusUnprocessableEntity, st, problem)
	assert.Equal(t, "STOCK_INSUFFICIENT", problem["code"])
	assert.Equal(t, "0", h.reserved(coklat, h.fg))
	assert.Equal(t, "0", h.scalar(`SELECT count(*)::text FROM delivery_orders WHERE source_id = $1`, d["id"]))
	d = h.json(http.MethodGet, "/api/v1/sales/consignment-deliveries/"+d["id"].(string), nil, http.StatusOK)
	assert.Equal(t, "draft", d["status"])
}

func ids(list []map[string]any) []any {
	out := make([]any, len(list))
	for i, m := range list {
		out[i] = m["id"]
	}
	return out
}
