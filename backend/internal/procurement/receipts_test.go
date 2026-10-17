package procurement_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/inventory"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/procurement"
)

type goodsReceipt struct {
	ID              uuid.UUID  `json:"id"`
	Number          string     `json:"number"`
	Status          string     `json:"status"`
	Version         int        `json:"version"`
	InventoryAmount string     `json:"inventoryAmount"`
	GrniAmount      string     `json:"grniAmount"`
	MaterialAmount  string     `json:"materialAmount"`
	JournalEntryID  *uuid.UUID `json:"journalEntryId"`
	ReversalEntryID *uuid.UUID `json:"reversalJournalEntryId"`
	AllowedActions  []string   `json:"allowedActions"`
	Lines           []struct {
		ID         uuid.UUID  `json:"id"`
		UnitCost   string     `json:"unitCost"`
		GrniQty    string     `json:"grniQty"`
		GrniAmount string     `json:"grniAmount"`
		BatchID    *uuid.UUID `json:"batchId"`
	} `json:"lines"`
}

type supplierBill struct {
	ID                    uuid.UUID  `json:"id"`
	Status                string     `json:"status"`
	MatchStatus           string     `json:"matchStatus"`
	Version               int        `json:"version"`
	PaymentBlocked        bool       `json:"paymentBlocked"`
	SubtotalAmount        string     `json:"subtotalAmount"`
	WithholdingType       string     `json:"withholdingType"`
	WithholdingAmount     string     `json:"withholdingAmount"`
	AdvanceAppliedAmount  string     `json:"advanceAppliedAmount"`
	TotalPayableAmount    string     `json:"totalPayableAmount"`
	JournalEntryID        *uuid.UUID `json:"journalEntryId"`
	AdvanceJournalEntryID *uuid.UUID `json:"advanceJournalEntryId"`
	Lines                 []struct {
		ExceedsTolerance bool `json:"exceedsTolerance"`
		PriceMismatch    bool `json:"priceMismatch"`
		QtyMismatch      bool `json:"qtyMismatch"`
	} `json:"lines"`
}

type purchaseReturn struct {
	ID              uuid.UUID  `json:"id"`
	Number          string     `json:"number"`
	DebitNoteNumber string     `json:"debitNoteNumber"`
	Status          string     `json:"status"`
	Version         int        `json:"version"`
	TotalAmount     string     `json:"totalAmount"`
	UnbilledAmount  string     `json:"unbilledAmount"`
	BilledAmount    string     `json:"billedAmount"`
	JournalEntryID  *uuid.UUID `json:"journalEntryId"`
	AllowedActions  []string   `json:"allowedActions"`
}

type grniReport struct {
	Lines          []struct{ OpenAmount string } `json:"lines"`
	SubledgerTotal string                        `json:"subledgerTotal"`
	LedgerBalance  string                        `json:"ledgerBalance"`
	Difference     string                        `json:"difference"`
}

// journal membaca jurnal: total debit & kredit per akun, dan memastikan balance serta dimensi divisi & pihak
// pemasok di setiap baris (S-225).
func (h *harness) journal(id uuid.UUID, rule string) map[string][2]string {
	h.t.Helper()
	var gotRule string
	require.NoError(h.t, h.pool.QueryRow(context.Background(),
		`SELECT coalesce(posting_rule_code, '') FROM journal_entries WHERE id = $1`, id).Scan(&gotRule))
	if rule != "" {
		assert.Equal(h.t, rule, gotRule)
	}
	rows, err := h.pool.Query(context.Background(), `
		SELECT a.code, l.debit_amount, l.credit_amount, l.division_id, l.party_type
		FROM journal_lines l JOIN accounts a ON a.id = l.account_id WHERE l.journal_entry_id = $1`, id)
	require.NoError(h.t, err)
	defer rows.Close()
	sums := map[string][2]decimal.Decimal{}
	debit, credit := decimal.Zero, decimal.Zero
	for rows.Next() {
		var code string
		var d, c decimal.Decimal
		var division *uuid.UUID
		var partyType *string
		require.NoError(h.t, rows.Scan(&code, &d, &c, &division, &partyType))
		require.NotNil(h.t, division, "dimensi divisi wajib %s", rule)
		assert.Equal(h.t, h.division, *division)
		require.NotNil(h.t, partyType)
		assert.Equal(h.t, "supplier", *partyType)
		cur := sums[code]
		sums[code] = [2]decimal.Decimal{cur[0].Add(d), cur[1].Add(c)}
		debit, credit = debit.Add(d), credit.Add(c)
	}
	require.NoError(h.t, rows.Err())
	assert.True(h.t, debit.Equal(credit), "jurnal %s tidak balance: %s vs %s", rule, debit, credit)
	out := map[string][2]string{}
	for code, v := range sums {
		out[code] = [2]string{v[0].StringFixed(2), v[1].StringFixed(2)}
	}
	return out
}

func (h *harness) stockQty(product, location uuid.UUID, status string) string {
	h.t.Helper()
	var q decimal.Decimal
	require.NoError(h.t, h.pool.QueryRow(context.Background(), `
		SELECT coalesce(sum(qty), 0) FROM stock_balances WHERE product_id = $1 AND location_id = $2 AND stock_status = $3`,
		product, location, status).Scan(&q))
	return q.StringFixed(3)
}

// approvedOrder membuat dan menyetujui PO (≤ Rp10 jt: satu level Head F&A).
func (h *harness) approvedOrder(body map[string]any) purchaseOrder {
	h.t.Helper()
	var po purchaseOrder
	require.Equal(h.t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders", body, &po))
	require.Equal(h.t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+po.ID.String()+"/submit",
		map[string]any{"version": po.Version}, &po))
	h.approve(h.hfa, "purchase_order", po.ID)
	require.Equal(h.t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	require.Equal(h.t, "approved", po.Status)
	return po
}

func (h *harness) postReceipt(body map[string]any) goodsReceipt {
	h.t.Helper()
	var gr goodsReceipt
	require.Equal(h.t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts", body, &gr))
	require.Equal(h.t, "draft", gr.Status)
	require.Equal(h.t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts/"+gr.ID.String()+"/post",
		map[string]any{"version": gr.Version}, &gr))
	require.Equal(h.t, "posted", gr.Status)
	return gr
}

func (h *harness) grniReconciles() grniReport {
	h.t.Helper()
	var rep grniReport
	require.Equal(h.t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/procurement/reports/goods-received-not-invoiced", nil, &rep))
	assert.Equal(h.t, "0.00", rep.Difference, "laporan Barang Diterima Belum Ditagih harus cocok dengan saldo 2102")
	assert.Equal(h.t, rep.SubledgerTotal, rep.LedgerBalance)
	return rep
}

// TestGoodsReceiptBillReturnFlow: PO barang → GR (batch wajib, reject diserap, toleransi qty 0%) A-01 → batal GR →
// tagihan dalam toleransi A-18 + potong uang muka A-19 → tagihan di luar toleransi (approval Head F&A, PPh 23) →
// retur manual (approval Head Logistik) A-02 nota debit → retur QC tanpa approval → laporan 2102 cocok.
func TestGoodsReceiptBillReturnFlow(t *testing.T) {
	h, m := setup(t)
	ctx := context.Background()
	tube := h.masters.product("PKG-TB-GR", "packaging")
	fg := h.masters.batchProduct("FG-GR-1", "finished_good")
	supplier := h.masters.supplier("KEMASGR", false)
	sup := h.masters.suppliers[supplier]
	sup.DefaultWithholding = "pph23"
	h.masters.suppliers[supplier] = sup
	inbound := h.masters.location("UTAMA-INB-GR", "inbound")
	fgLoc := h.masters.location("UTAMA-FG-GR", "finished_goods")

	po := h.approvedOrder(map[string]any{
		"orderType": "goods", "supplierId": supplier, "divisionId": h.division, "businessDate": today(),
		"expectedDate": plusDays(1),
		"lines": []map[string]any{
			{"productId": tube, "qty": "100", "unitPrice": "1000"},
			{"productId": fg, "qty": "50", "unitPrice": "20000"},
		},
	})
	tubeLine, fgLine := po.Lines[0].ID, po.Lines[1].ID

	// Uang muka 300 rb.
	var adv purchaseAdvance
	require.Equal(t, http.StatusCreated, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances", map[string]any{
		"purchaseOrderId": po.ID, "amount": "300000", "paymentDate": today(), "bankAccountCode": "1111",
	}, &adv))
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances/"+adv.ID.String()+"/post",
		map[string]any{"version": adv.Version}, &adv))

	grBody := func(lines ...map[string]any) map[string]any {
		return map[string]any{"purchaseOrderId": po.ID, "businessDate": today(), "locationId": inbound, "deliveryNote": "SJ-01", "lines": lines}
	}
	// Produk berbatch wajib nomor batch + tanggal produksi & kedaluwarsa; lokasi harus Inbound; qty > sisa PO ditolak.
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts",
		grBody(map[string]any{"purchaseOrderLineId": fgLine, "goodQty": "30"}), nil))
	bad := grBody(map[string]any{"purchaseOrderLineId": tubeLine, "goodQty": "10"})
	bad["locationId"] = fgLoc
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts", bad, nil))
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts",
		grBody(map[string]any{"purchaseOrderLineId": tubeLine, "goodQty": "95", "rejectQty": "6"}), nil), "toleransi qty 0%")

	// 1. GR1: tube 60 baik + 5 reject diserap, FG 30 batch B1.
	gr1 := h.postReceipt(grBody(
		map[string]any{"purchaseOrderLineId": tubeLine, "goodQty": "60", "rejectQty": "5", "rejectDisposition": "absorbed"},
		map[string]any{"purchaseOrderLineId": fgLine, "goodQty": "30", "batchNumber": "b1", "productionDate": today(), "expiryDate": plusDays(365)},
	))
	assert.True(t, strings.HasPrefix(gr1.Number, "GR/2026/10/"), gr1.Number)
	assert.Equal(t, "665000.00", gr1.InventoryAmount)
	assert.Equal(t, "1083.3333", gr1.Lines[0].UnitCost, "reject diserap ke HPP barang baik")
	assert.Equal(t, "65.000", gr1.Lines[0].GrniQty)
	require.NotNil(t, gr1.JournalEntryID)
	assert.Equal(t, map[string][2]string{
		"1301": {"65000.00", "0.00"}, "1311": {"600000.00", "0.00"}, "2102": {"0.00", "665000.00"},
	}, h.journal(*gr1.JournalEntryID, "A-01"))
	assert.Equal(t, "60.000", h.stockQty(tube, inbound, "quarantine"), "stok masuk Inbound berstatus Karantina")
	assert.Equal(t, "30.000", h.stockQty(fg, inbound, "quarantine"))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	assert.Equal(t, "partially_received", po.Status)
	assert.Equal(t, "65.000", po.Lines[0].ReceivedQty)
	h.grniReconciles()

	// 2. GR2 lalu dibatalkan sebelum ditagih → stok & jurnal dibalik, qty PO kembali.
	gr2 := h.postReceipt(grBody(map[string]any{"purchaseOrderLineId": tubeLine, "goodQty": "10"}))
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts/"+gr2.ID.String()+"/cancel",
		map[string]any{"version": gr2.Version}, nil), "alasan wajib")
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts/"+gr2.ID.String()+"/cancel",
		map[string]any{"version": gr2.Version, "reason": "Salah input"}, &gr2))
	assert.Equal(t, "cancelled", gr2.Status)
	require.NotNil(t, gr2.ReversalEntryID)
	assert.Equal(t, map[string][2]string{"1301": {"0.00", "10000.00"}, "2102": {"10000.00", "0.00"}}, h.journal(*gr2.ReversalEntryID, ""))
	assert.Equal(t, "60.000", h.stockQty(tube, inbound, "quarantine"))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	assert.Equal(t, "65.000", po.Lines[0].ReceivedQty)

	// 3. Tagihan 1 cocok penuh dengan GR1 → A-18 + potong uang muka A-19; utang terbentuk (PPh 23 dinolkan manual).
	var billable struct {
		DefaultWithholdingType string `json:"defaultWithholdingType"`
		OutstandingAdvance     string `json:"outstandingAdvanceAmount"`
		Lines                  []struct {
			GoodsReceiptLineID uuid.UUID `json:"goodsReceiptLineId"`
			OpenQty            string    `json:"openQty"`
		} `json:"lines"`
	}
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/procurement/billable-receipts?purchaseOrderId="+po.ID.String(), nil, &billable))
	assert.Equal(t, "pph23", billable.DefaultWithholdingType)
	assert.Equal(t, "300000.00", billable.OutstandingAdvance)
	require.Len(t, billable.Lines, 2)
	billBody := func(number string, lines ...map[string]any) map[string]any {
		return map[string]any{"purchaseOrderId": po.ID, "number": number, "billDate": today(), "withholdingType": "none", "lines": lines}
	}
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills",
		billBody("INV-K-1", map[string]any{"goodsReceiptLineId": gr1.Lines[1].ID, "qty": "31", "unitPrice": "20000"}), nil),
		"tagihan melebihi qty diterima")
	var bill1 supplierBill
	require.Equal(t, http.StatusCreated, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills", billBody("INV-K-1",
		map[string]any{"goodsReceiptLineId": gr1.Lines[0].ID, "qty": "65", "unitPrice": "1000"},
		map[string]any{"goodsReceiptLineId": gr1.Lines[1].ID, "qty": "30", "unitPrice": "20000"},
	), &bill1))
	assert.Equal(t, "matched", bill1.MatchStatus)
	assert.True(t, bill1.PaymentBlocked)
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills", billBody("inv-k-1",
		map[string]any{"goodsReceiptLineId": gr1.Lines[0].ID, "qty": "1", "unitPrice": "1000"}), nil), "nomor tagihan unik per pemasok")
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills/"+bill1.ID.String()+"/post",
		map[string]any{"version": bill1.Version}, &bill1))
	assert.Equal(t, "awaiting_payment", bill1.Status)
	assert.False(t, bill1.PaymentBlocked)
	assert.Equal(t, "300000.00", bill1.AdvanceAppliedAmount)
	assert.Equal(t, "365000.00", bill1.TotalPayableAmount)
	require.NotNil(t, bill1.JournalEntryID)
	assert.Equal(t, map[string][2]string{"2102": {"665000.00", "0.00"}, "2101": {"0.00", "665000.00"}}, h.journal(*bill1.JournalEntryID, "A-18"))
	require.NotNil(t, bill1.AdvanceJournalEntryID)
	assert.Equal(t, map[string][2]string{"2101": {"300000.00", "0.00"}, "1401": {"0.00", "300000.00"}}, h.journal(*bill1.AdvanceJournalEntryID, "A-19"))
	payables, err := m.OpenPayables(ctx, supplier)
	require.NoError(t, err)
	require.Len(t, payables, 1)
	assert.Equal(t, "365000.00", payables[0].OpenAmount.String())
	// GR yang sudah ditagih tidak bisa dibatalkan.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/goods-receipts/"+gr1.ID.String(), nil, &gr1))
	assert.NotContains(t, gr1.AllowedActions, "cancel")
	assert.Equal(t, http.StatusConflict, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts/"+gr1.ID.String()+"/cancel",
		map[string]any{"version": gr1.Version, "reason": "x"}, nil))
	h.grniReconciles()

	// 4. GR3 FG 20 batch B2, tagihan harga +2,5% (> 1%) → menunggu persetujuan Head F&A, pembayaran diblokir.
	gr3 := h.postReceipt(grBody(map[string]any{
		"purchaseOrderLineId": fgLine, "goodQty": "20", "batchNumber": "B2", "productionDate": today(), "expiryDate": plusDays(300),
	}))
	var bill2 supplierBill
	b2 := billBody("INV-K-2", map[string]any{"goodsReceiptLineId": gr3.Lines[0].ID, "qty": "20", "unitPrice": "20500"})
	delete(b2, "withholdingType")
	require.Equal(t, http.StatusCreated, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills", b2, &bill2))
	assert.Equal(t, "exceeds_tolerance", bill2.MatchStatus)
	assert.True(t, bill2.Lines[0].PriceMismatch)
	assert.Equal(t, "pph23", bill2.WithholdingType, "PPh default pemasok")
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodPost, "/api/v1/supplier-bills/"+bill2.ID.String()+"/post",
		map[string]any{"version": bill2.Version}, &bill2))
	assert.Equal(t, "pending_approval", bill2.Status)
	assert.True(t, bill2.PaymentBlocked)
	assert.Nil(t, bill2.JournalEntryID)
	payables, err = m.OpenPayables(ctx, supplier)
	require.NoError(t, err)
	assert.Len(t, payables, 1, "utang belum terbentuk selama menunggu persetujuan")
	decided := h.approve(h.hfa, "supplier_bill_exception", bill2.ID)
	assert.Equal(t, "approved", decided.Status)
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/supplier-bills/"+bill2.ID.String(), nil, &bill2))
	assert.Equal(t, "awaiting_payment", bill2.Status)
	assert.Equal(t, "approved_exception", bill2.MatchStatus)
	assert.Equal(t, "8200.00", bill2.WithholdingAmount, "PPh 23 2% dari Rp410.000")
	assert.Equal(t, "401800.00", bill2.TotalPayableAmount)
	require.NotNil(t, bill2.JournalEntryID)
	assert.Equal(t, map[string][2]string{
		"2102": {"400000.00", "0.00"}, "5103": {"10000.00", "0.00"}, "2101": {"0.00", "401800.00"}, "2302": {"0.00", "8200.00"},
	}, h.journal(*bill2.JournalEntryID, "A-18"))
	h.grniReconciles()

	// 5. Retur manual tube 5 dari GR1 (sudah ditagih) → approval Head Logistik → posting: DN, A-02 Dr 2101 / Cr 1301.
	var ret purchaseReturn
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-returns", map[string]any{
		"purchaseOrderId": po.ID, "businessDate": today(), "reason": "Kemasan penyok",
		"lines": []map[string]any{{"goodsReceiptLineId": gr1.Lines[0].ID, "locationId": inbound, "stockStatus": "quarantine", "qty": "5"}},
	}, &ret))
	assert.Contains(t, ret.AllowedActions, "submit")
	assert.NotContains(t, ret.AllowedActions, "post", "retur manual wajib disetujui")
	assert.Equal(t, http.StatusConflict, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-returns/"+ret.ID.String()+"/post",
		map[string]any{"version": ret.Version}, nil))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-returns/"+ret.ID.String()+"/submit",
		map[string]any{"version": ret.Version}, &ret))
	assert.Equal(t, "pending_approval", ret.Status)
	assert.True(t, strings.HasPrefix(ret.Number, "RB/2026/10/"), ret.Number)
	h.approve(h.headLog, "purchase_return", ret.ID)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-returns/"+ret.ID.String(), nil, &ret))
	assert.Equal(t, "approved", ret.Status)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-returns/"+ret.ID.String()+"/post",
		map[string]any{"version": ret.Version}, &ret))
	assert.Equal(t, "posted", ret.Status)
	assert.True(t, strings.HasPrefix(ret.DebitNoteNumber, "DN/2026/10/"), ret.DebitNoteNumber)
	assert.Equal(t, "5416.67", ret.TotalAmount)
	assert.Equal(t, "5416.67", ret.BilledAmount)
	require.NotNil(t, ret.JournalEntryID)
	assert.Equal(t, map[string][2]string{"2101": {"5416.67", "0.00"}, "1301": {"0.00", "5416.67"}}, h.journal(*ret.JournalEntryID, "A-02"))
	assert.Equal(t, "55.000", h.stockQty(tube, inbound, "quarantine"))
	payables, err = m.OpenPayables(ctx, supplier)
	require.NoError(t, err)
	require.Len(t, payables, 3)
	total := decimal.Zero
	for _, p := range payables {
		total = total.Add(p.OpenAmount.Decimal())
	}
	assert.Equal(t, "761383.33", total.StringFixed(2), "utang berkurang sebesar nota debit")

	// 6. Retur dari QC Inbound tanpa approval (D-24) atas GR4 yang belum ditagih → A-02 Dr 2102, qty PO berkurang.
	gr4 := h.postReceipt(grBody(map[string]any{"purchaseOrderLineId": tubeLine, "goodQty": "10"}))
	sourceID := uuid.New()
	qc, err := m.CreateReturnFromQC(h.as(h.leader), procurement.QCReturnInput{
		Origin: procurement.ReturnQCInbound, SourceType: "inbound_line", SourceID: sourceID, SourceLabel: "IN/2026/10/00001",
		BusinessDate: clock.BusinessDateOf(monday09), Reason: "Ditolak QC: bocor",
		Lines: []procurement.QCReturnLine{{ReceiptLineID: &gr4.Lines[0].ID, ProductID: tube, LocationID: inbound, StockStatus: "quarantine", Qty: decimal.NewFromInt(4)}},
	})
	require.NoError(t, err)
	assert.Equal(t, procurement.Status("posted"), qc.Status)
	// Tube tidak berbatch: HPP FIFO mengambil cost layer tertua (GR1, Rp1.083,3333/pcs).
	assert.Equal(t, "4333.33", qc.UnbilledAmount.String())
	require.NotNil(t, qc.JournalEntryID)
	assert.Equal(t, map[string][2]string{"2102": {"4333.33", "0.00"}, "1301": {"0.00", "4333.33"}}, h.journal(*qc.JournalEntryID, "A-02"))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	assert.Equal(t, "71.000", po.Lines[0].ReceivedQty, "pemasok mengganti barang retur yang belum ditagih")
	_, err = m.CreateReturnFromQC(h.as(h.leader), procurement.QCReturnInput{
		Origin: procurement.ReturnQCInbound, SourceType: "inbound_line", SourceID: sourceID, BusinessDate: clock.BusinessDateOf(monday09),
		Reason: "ulang", Lines: []procurement.QCReturnLine{{ReceiptLineID: &gr4.Lines[0].ID, ProductID: tube, LocationID: inbound, StockStatus: "quarantine", Qty: decimal.NewFromInt(1)}},
	})
	assert.Error(t, err, "satu dokumen QC satu retur aktif")
	_, err = m.CreateReturnFromQC(h.as(h.leader), procurement.QCReturnInput{
		Origin: procurement.ReturnQCInbound, SourceType: "inbound_line", SourceID: uuid.New(), BusinessDate: clock.BusinessDateOf(monday09),
		Reason: "lebih", Lines: []procurement.QCReturnLine{{ReceiptLineID: &gr4.Lines[0].ID, ProductID: tube, LocationID: inbound, StockStatus: "quarantine", Qty: decimal.NewFromInt(7)}},
	})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "qty retur melebihi qty baik GR")

	rep := h.grniReconciles()
	assert.NotEmpty(t, rep.Lines)
}

// TestTollReceiptHPP (S-207): penerimaan FG maklon memakai packaging baik + reject dari lokasi maklon, HPP batch =
// (jasa + packaging terpakai) ÷ qty baik, sisa packaging tetap di lokasi maklon, jurnal A-03; batal mengembalikan
// packaging ke lokasi maklon.
func TestTollReceiptHPP(t *testing.T) {
	h, _ := setup(t)
	ctx := actorCtx(h)
	fg := h.masters.batchProduct("FG-MKL-1", "finished_good")
	tube := h.masters.product("PKG-MKL-1", "packaging")
	maklon := h.masters.supplier("MAKLONHPP", true)
	tollLoc := h.masters.location("MKL-HPP", "toll_manufacturer")
	h.masters.toll[maklon] = tollLoc
	inbound := h.masters.location("UTAMA-INB-MKL", "inbound")

	// Packaging 1.100 pcs @ Rp800 sudah di lokasi maklon (dikirim lewat Transfer Stok).
	_, err := h.stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: "opening_balance", ID: uuid.New(), Label: "Saldo awal", BusinessDate: clock.BusinessDateOf(monday09)},
		Lines: []inventory.Line{{Kind: inventory.KindReceive, ProductID: tube, Qty: decimal.NewFromInt(1100), UnitCost: decPtr("800"),
			To: &inventory.Place{LocationID: tollLoc, Status: inventory.StatusAvailable}}},
	})
	require.NoError(t, err)

	po := h.approvedOrder(map[string]any{
		"orderType": "toll_manufacturing", "supplierId": maklon, "divisionId": h.division, "businessDate": today(),
		"expectedDate": plusDays(2),
		"lines":        []map[string]any{{"productId": fg, "qty": "1000", "unitPrice": "1500"}},
		"materials":    []map[string]any{{"productId": tube, "plannedQty": "1050"}},
	})
	body := map[string]any{
		"purchaseOrderId": po.ID, "businessDate": today(), "locationId": inbound,
		"lines": []map[string]any{{
			"purchaseOrderLineId": po.Lines[0].ID, "goodQty": "980", "rejectQty": "20", "rejectDisposition": "absorbed",
			"batchNumber": "MKL2610", "productionDate": today(), "expiryDate": plusDays(540),
		}},
	}
	// Pemakaian packaging wajib untuk PO maklon.
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts", body, nil))
	body["materials"] = []map[string]any{{"productId": tube, "usedGoodQty": "1000", "usedRejectQty": "30"}}
	gr := h.postReceipt(body)

	// Packaging terpakai 1.030 × 800 = 824.000 (reject 30 ikut); jasa 1.000 × 1.500 = 1.500.000.
	assert.Equal(t, "824000.00", gr.MaterialAmount)
	assert.Equal(t, "1500000.00", gr.GrniAmount)
	assert.Equal(t, "2324000.00", gr.InventoryAmount)
	assert.Equal(t, "2371.4286", gr.Lines[0].UnitCost, "HPP = (jasa + packaging baik + reject) ÷ qty baik")
	withoutReject := decimal.RequireFromString("2300000").DivRound(decimal.NewFromInt(980), 4)
	assert.True(t, decimal.RequireFromString(gr.Lines[0].UnitCost).GreaterThan(withoutReject), "reject packaging masuk HPP")
	require.NotNil(t, gr.JournalEntryID)
	assert.Equal(t, map[string][2]string{
		"1311": {"2324000.00", "0.00"}, "1301": {"0.00", "824000.00"}, "2102": {"0.00", "1500000.00"},
	}, h.journal(*gr.JournalEntryID, "A-03"))
	assert.Equal(t, "70.000", h.stockQty(tube, tollLoc, "available"), "sisa packaging tetap di lokasi maklon")
	assert.Equal(t, "980.000", h.stockQty(fg, inbound, "quarantine"))
	var cost decimal.Decimal
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT unit_cost FROM cost_layers WHERE batch_id = $1`, gr.Lines[0].BatchID).Scan(&cost))
	assert.Equal(t, "2371.4286", cost.StringFixed(4), "cost layer batch FG memakai HPP maklon")
	h.grniReconciles()

	// Batal GR maklon: FG keluar dari Karantina, packaging kembali ke lokasi maklon, jurnal A-03 dibalik.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/goods-receipts/"+gr.ID.String()+"/cancel",
		map[string]any{"version": gr.Version, "reason": "Laporan maklon salah"}, &gr))
	assert.Equal(t, "cancelled", gr.Status)
	assert.Equal(t, "1100.000", h.stockQty(tube, tollLoc, "available"))
	assert.Equal(t, "0.000", h.stockQty(fg, inbound, "quarantine"))
	require.NotNil(t, gr.ReversalEntryID)
	h.journal(*gr.ReversalEntryID, "")
	h.grniReconciles()
}

func actorCtx(h *harness) context.Context { return h.as(h.purchasing) }

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
