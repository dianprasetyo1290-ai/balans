package http

import (
	"context"

	"github.com/shopspring/decimal"

	"balans/internal/warehouse/service"
)

// GetStockCountVarianceReport mengembalikan hasil stock count & selisih (RPT-03).
func (h *WarehouseHandler) GetStockCountVarianceReport(ctx context.Context, req GetStockCountVarianceReportRequestObject) (GetStockCountVarianceReportResponseObject, error) {
	p := req.Params
	f := service.CountVarianceFilter{
		DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo), LocationID: p.LocationId, ProductID: p.ProductId,
		VarianceOnly: p.VarianceOnly != nil && *p.VarianceOnly,
	}
	rep, err := h.svc.StockCountVarianceReport(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]StockCountVarianceRow, len(rep.Rows))
	total, short, over := decimal.Zero, decimal.Zero, decimal.Zero
	for i, r := range rep.Rows {
		loc := rep.Locations[r.LocationID]
		row := StockCountVarianceRow{
			StockCountId: r.CountID, Number: r.Number, CountType: r.CountType, BusinessDate: toDate(r.BusinessDate),
			Status: r.Status, LocationCode: loc.Code, LocationName: loc.Name, Sku: rep.Products[r.ProductID][0],
			ProductName: rep.Products[r.ProductID][1], SystemQty: r.SystemQty.String(), VarianceQty: r.VarianceQty.String(),
			VarianceAmount: money(r.VarianceAmount), IsExtra: r.IsExtra,
		}
		if r.BatchID != nil {
			row.BatchNumber = rep.Batches[*r.BatchID]
		}
		if r.CountedQty != nil {
			v := r.CountedQty.String()
			row.CountedQty = &v
		}
		items[i] = row
		total = total.Add(r.VarianceAmount)
		if r.VarianceAmount.IsNegative() {
			short = short.Add(r.VarianceAmount)
		} else {
			over = over.Add(r.VarianceAmount)
		}
	}
	return GetStockCountVarianceReport200JSONResponse{
		Items: items, TotalVarianceAmount: money(total),
		TotalShortageAmount: money(short), TotalOverageAmount: money(over),
	}, nil
}
