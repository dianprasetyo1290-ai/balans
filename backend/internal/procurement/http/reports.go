package http

import (
	"context"

	"balans/internal/platform/money"
	"balans/internal/procurement/service"
)

// Laporan pengadaan: Outstanding PO (RPT-10), umur utang (RPT-11), komposisi HPP batch (RPT-04).

// GetOutstandingPurchaseOrders mengembalikan baris PO terbuka.
func (h *ProcurementHandler) GetOutstandingPurchaseOrders(ctx context.Context, req GetOutstandingPurchaseOrdersRequestObject) (GetOutstandingPurchaseOrdersResponseObject, error) {
	rep, err := h.svc.OutstandingPOReport(ctx, req.Params.SupplierId)
	if err != nil {
		return nil, err
	}
	items := make([]OutstandingPurchaseOrderLine, len(rep.Lines))
	total := money.Zero
	for i, l := range rep.Lines {
		item := l.Description
		if l.ProductID != nil {
			if p, ok := rep.Products[*l.ProductID]; ok {
				item = p.SKU + " – " + p.Name
			}
		}
		open := l.OrderedQty.Sub(l.ReceivedQty)
		value := l.UnitPrice.Mul(open).Round()
		total = total.Add(value)
		items[i] = OutstandingPurchaseOrderLine{
			PurchaseOrderId: l.OrderID, Number: l.Number, OrderType: PurchaseOrderType(l.OrderType), SupplierName: l.SupplierName,
			OrderDate: toDate(l.OrderDate), ExpectedDate: toDate(l.ExpectedDate), Status: l.Status, LineNo: l.LineNo, Item: item,
			OrderedQty: l.OrderedQty.String(), ReceivedQty: l.ReceivedQty.String(), BilledQty: l.BilledQty.String(),
			OpenQty: open.String(), UnbilledQty: l.ReceivedQty.Sub(l.BilledQty).String(), UnitPrice: l.UnitPrice.String(),
			OpenValue: value.String(),
		}
	}
	return GetOutstandingPurchaseOrders200JSONResponse{AsOf: toDate(rep.AsOf), Items: items, TotalOpenValue: total.String()}, nil
}

func agingAmounts(b []service.PayableAgingBucket, total money.Amount) PayableAgingAmounts {
	return PayableAgingAmounts{
		Current: b[0].Amount.String(), D1To30: b[1].Amount.String(), D31To60: b[2].Amount.String(),
		D61To90: b[3].Amount.String(), Over90: b[4].Amount.String(), Total: total.String(),
	}
}

// GetPayableAging mengembalikan umur utang per pemasok.
func (h *ProcurementHandler) GetPayableAging(ctx context.Context, req GetPayableAgingRequestObject) (GetPayableAgingResponseObject, error) {
	var asOf = h.svc.Today()
	if req.Params.AsOf != nil {
		asOf = fromDate(*req.Params.AsOf)
	}
	rep, err := h.svc.PayableAgingReport(ctx, asOf)
	if err != nil {
		return nil, err
	}
	items := make([]PayableAgingSupplier, len(rep.Rows))
	for i, r := range rep.Rows {
		docs := make([]PayableAgingDocument, len(r.Documents))
		for j, d := range r.Documents {
			docs[j] = PayableAgingDocument{
				DocumentType: PayableAgingDocumentDocumentType(d.DocumentType), DocumentNumber: d.DocumentNumber,
				BusinessDate: toDate(d.BusinessDate), DueDate: toDate(d.DueDate), Amount: d.Amount.String(), Open: d.Open.String(),
			}
		}
		items[i] = PayableAgingSupplier{
			SupplierId: r.SupplierID, SupplierCode: r.SupplierCode, SupplierName: r.SupplierName,
			Amounts: agingAmounts(r.Buckets, r.Total), Documents: docs,
		}
	}
	return GetPayableAging200JSONResponse{
		AsOf: toDate(rep.AsOf), Items: items, Totals: agingAmounts(rep.Totals, rep.Total),
		LedgerBalance: rep.LedgerBalance.String(), Difference: rep.LedgerBalance.Sub(rep.Total).String(),
	}, nil
}

// GetBatchCostReport mengembalikan komposisi HPP per batch.
func (h *ProcurementHandler) GetBatchCostReport(ctx context.Context, req GetBatchCostReportRequestObject) (GetBatchCostReportResponseObject, error) {
	p := req.Params
	rep, err := h.svc.BatchCostReport(ctx, fromDate(p.DateFrom), fromDate(p.DateTo), p.ProductId)
	if err != nil {
		return nil, err
	}
	items := make([]BatchCostRow, len(rep.Rows))
	for i, r := range rep.Rows {
		prod := rep.Products[r.ProductID]
		items[i] = BatchCostRow{
			ReceiptId: r.ReceiptID, ReceiptNumber: r.ReceiptNumber, BusinessDate: toDate(r.BusinessDate), SupplierName: r.SupplierName,
			OrderType: PurchaseOrderType(r.OrderType), Sku: prod.SKU, ProductName: prod.Name, BatchNumber: r.BatchNumber,
			GoodQty: r.GoodQty.String(), RejectQty: r.RejectQty.String(),
			RejectDisposition: BatchCostRowRejectDisposition(r.RejectDisposition), UnitPrice: r.UnitPrice.String(),
			ServiceAmount: r.GRNIAmount.String(), MaterialCost: r.MaterialCost.String(), InventoryAmount: r.InventoryAmount.String(),
			UnitCost: r.UnitCost.StringFixed(4),
		}
	}
	return GetBatchCostReport200JSONResponse{Items: items}, nil
}
