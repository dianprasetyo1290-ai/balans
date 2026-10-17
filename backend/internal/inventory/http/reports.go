package http

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
)

// Laporan persediaan (S-414, S-415).

func reportProduct(r service.ReportRefs, id uuid.UUID) ReportProductRef {
	p := r.Product(id)
	return ReportProductRef{Id: id, Sku: p.SKU, Name: p.Name, Uom: p.UOM}
}

func reportLocation(r service.ReportRefs, id uuid.UUID) ReportLocationRef {
	l := r.Location(id)
	return ReportLocationRef{Id: id, Code: l.Code, Name: l.Name, WarehouseName: l.WarehouseName, Type: l.Type}
}

func stockFilter(locationID *uuid.UUID, locationType *string, productID, batchID *uuid.UUID) service.StockReportFilter {
	f := service.StockReportFilter{StockReportFilter: postgres.StockReportFilter{ProductID: productID, BatchID: batchID}}
	if locationID != nil {
		f.LocationIDs = []uuid.UUID{*locationID}
	}
	if locationType != nil {
		f.LocationType = *locationType
	}
	return f
}

// GetStockPositionReport mengembalikan posisi & nilai stok (RPT-01).
func (h *InventoryHandler) GetStockPositionReport(ctx context.Context, req GetStockPositionReportRequestObject) (GetStockPositionReportResponseObject, error) {
	p := req.Params
	rep, err := h.svc.StockPositionReport(ctx, stockFilter(p.LocationId, p.LocationType, p.ProductId, p.BatchId))
	if err != nil {
		return nil, err
	}
	items := make([]StockPositionRow, len(rep.Rows))
	for i, r := range rep.Rows {
		items[i] = StockPositionRow{
			Product: reportProduct(rep.Refs, r.ProductID), Location: reportLocation(rep.Refs, r.LocationID), BatchId: r.BatchID,
			BatchNumber: r.BatchNumber, ExpiryDate: optDate(r.ExpiryDate), FirstInDate: optDate(r.FirstInDate),
			Qty: qty(r.Qty), Value: amount(r.Value),
		}
	}
	types := make([]StockTypeTotal, len(rep.ByType))
	for i, t := range rep.ByType {
		types[i] = StockTypeTotal{LocationType: t.Type, Qty: qty(t.Qty), Value: amount(t.Value)}
	}
	return GetStockPositionReport200JSONResponse{
		Items: items, TotalQty: qty(rep.TotalQty), TotalValue: amount(rep.TotalValue), ByLocationType: types,
	}, nil
}

// GetStockMovementReport mengembalikan mutasi stok periode (RPT-02).
func (h *InventoryHandler) GetStockMovementReport(ctx context.Context, req GetStockMovementReportRequestObject) (GetStockMovementReportResponseObject, error) {
	p := req.Params
	from, to := fromDate(p.DateFrom), fromDate(p.DateTo)
	rep, err := h.svc.StockMovementReport(ctx, stockFilter(p.LocationId, p.LocationType, p.ProductId, nil), from, to)
	if err != nil {
		return nil, err
	}
	items := make([]StockMovementRow, len(rep.Rows))
	for i, r := range rep.Rows {
		items[i] = StockMovementRow{
			Product: reportProduct(rep.Refs, r.ProductID), Location: reportLocation(rep.Refs, r.LocationID),
			OpeningQty: qty(r.OpeningQty()), ReceiveQty: qty(r.ReceiveQty), IssueQty: qty(r.IssueQty.Neg()),
			MoveInQty: qty(r.MoveInQty), MoveOutQty: qty(r.MoveOutQty.Neg()), ClosingQty: qty(r.ClosingQty),
			OpeningValue: amount(r.OpeningValue()), InValue: amount(r.InValue), OutValue: amount(r.OutValue.Neg()),
			ClosingValue: amount(r.ClosingValue),
		}
	}
	return GetStockMovementReport200JSONResponse{DateFrom: p.DateFrom, DateTo: p.DateTo, Items: items}, nil
}

// GetStockExpiryReport mengembalikan stok mendekati kedaluwarsa & umur stok (RPT-19).
func (h *InventoryHandler) GetStockExpiryReport(ctx context.Context, req GetStockExpiryReportRequestObject) (GetStockExpiryReportResponseObject, error) {
	p := req.Params
	within := 0
	if p.WithinDays != nil {
		within = *p.WithinDays
	}
	rep, err := h.svc.StockExpiryReport(ctx, stockFilter(p.LocationId, p.LocationType, p.ProductId, nil), within)
	if err != nil {
		return nil, err
	}
	items := make([]StockExpiryRow, len(rep.Rows))
	for i, r := range rep.Rows {
		items[i] = StockExpiryRow{
			Product: reportProduct(rep.Refs, r.ProductID), Location: reportLocation(rep.Refs, r.LocationID),
			BatchNumber: r.BatchNumber, ExpiryDate: optDate(r.ExpiryDate), DaysToExpiry: r.DaysToExpiry, AgeDays: r.AgeDays,
			Qty: qty(r.Qty), Value: amount(r.Value), Bucket: StockExpiryRowBucket(r.Bucket),
		}
	}
	return GetStockExpiryReport200JSONResponse{Today: toDate(rep.Today), NearDays: rep.NearDays, Items: items}, nil
}

// GetInternalUsageReport mengembalikan pemakaian internal diposting (RPT-08).
func (h *InventoryHandler) GetInternalUsageReport(ctx context.Context, req GetInternalUsageReportRequestObject) (GetInternalUsageReportResponseObject, error) {
	p := req.Params
	f := postgres.UsageReportFilter{DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo), CampaignID: p.CampaignId, DivisionID: p.DivisionId}
	if p.Purpose != nil {
		f.Purpose = string(*p.Purpose)
	}
	rep, err := h.svc.InternalUsageReport(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]InternalUsageReportRow, len(rep.Rows))
	total := decimal.Zero
	for i, r := range rep.Rows {
		row := InternalUsageReportRow{
			UsageId: r.ID, Number: r.Number, BusinessDate: toDate(r.BusinessDate), Purpose: r.Purpose,
			DivisionName: rep.Dimensions[r.DivisionID].Name, Product: reportProduct(rep.Refs, r.ProductID),
			Qty: qty(r.Qty), ReturnedQty: qty(r.ReturnedQty), ConsumedQty: qty(r.ConsumedQty), Cost: amount(r.Cost),
		}
		if r.CampaignID != nil {
			c := rep.Dimensions[*r.CampaignID]
			row.CampaignCode, row.CampaignName = c.Code, c.Name
		}
		items[i] = row
		total = total.Add(r.Cost)
	}
	return GetInternalUsageReport200JSONResponse{Items: items, TotalCost: amount(total)}, nil
}

// GetReturnReport mengembalikan barang retur per alasan & tindak lanjut (RPT-09).
func (h *InventoryHandler) GetReturnReport(ctx context.Context, req GetReturnReportRequestObject) (GetReturnReportResponseObject, error) {
	rep, err := h.svc.ReturnDispositionReport(ctx, fromDate(req.Params.DateFrom), fromDate(req.Params.DateTo))
	if err != nil {
		return nil, err
	}
	items := make([]ReturnReportRow, len(rep.Rows))
	for i, r := range rep.Rows {
		row := ReturnReportRow{
			DispositionId: r.ID, Number: r.Number, BusinessDate: toDate(r.BusinessDate),
			Location: reportLocation(rep.Refs, r.LocationID), Product: reportProduct(rep.Refs, r.ProductID),
			Disposition: ReturnDisposition(r.Disposition), Qty: qty(r.Qty), Cost: amount(r.Cost), QcNote: r.QCNote,
		}
		if r.ReasonID != nil {
			rr := rep.Reasons[*r.ReasonID]
			row.ReasonCode, row.ReasonName = rr.Code, rr.Name
		}
		items[i] = row
	}
	return GetReturnReport200JSONResponse{Items: items}, nil
}
