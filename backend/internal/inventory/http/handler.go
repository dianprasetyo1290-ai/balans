// Package http adalah handler REST modul inventory (halaman Persediaan).
package http

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/inventory.yaml ../../../api/openapi.bundle.yaml

// InventoryHandler mengimplementasikan StrictServerInterface hasil generate.
type InventoryHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*InventoryHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *InventoryHandler { return &InventoryHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *InventoryHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func qty(d decimal.Decimal) StockQuantity { return d.StringFixed(domain.QtyScale) }

func unitCost(d decimal.Decimal) UnitCost { return d.StringFixed(domain.UnitCostScale) }

func amount(d decimal.Decimal) Money { return money.FromDecimal(d).Round().String() }

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func optDate(d *clock.BusinessDate) *BusinessDate {
	if d == nil {
		return nil
	}
	v := toDate(*d)
	return &v
}

func fromDate(d BusinessDate) clock.BusinessDate {
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func productRef(p service.ProductRef) StockProductRef {
	return StockProductRef{Id: p.ID, Sku: p.SKU, Name: p.Name, Uom: p.UOM}
}

func locationRef(l service.LocationRef) StockLocationRef {
	return StockLocationRef{Id: l.ID, Code: l.Code, Name: l.Name, WarehouseName: optStr(l.WarehouseName)}
}

func expiryState(b domain.Batch, today clock.BusinessDate, near int) ExpiryState {
	return ExpiryState(b.State(today, near))
}

// ListStockBalances mengembalikan Stok per Lokasi.
func (h *InventoryHandler) ListStockBalances(ctx context.Context, req ListStockBalancesRequestObject) (ListStockBalancesResponseObject, error) {
	p := req.Params
	f := service.BalanceFilter{ProductID: p.ProductId, BatchID: p.BatchId}
	if p.LocationId != nil {
		f.LocationIDs = *p.LocationId
	}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, domain.StockStatus(s))
		}
	}
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListBalances(ctx, f, page)
	if err != nil {
		return nil, err
	}
	items := make([]StockBalance, len(res.Items))
	for i, r := range res.Items {
		b := domain.Batch{ExpiryDate: r.ExpiryDate}
		item := StockBalance{
			Product: productRef(res.Refs.Product(r.Key.ProductID)), Location: locationRef(res.Refs.Location(r.Key.LocationID)),
			BatchId: r.Key.BatchID, BatchNumber: optStr(r.BatchNumber), ProductionDate: optDate(r.ProductionDate),
			ExpiryDate: optDate(r.ExpiryDate), Status: StockStatus(r.Key.Status), StatusLabel: r.Key.Status.Label(),
			Qty: qty(r.Qty), UnitCost: unitCost(r.AvgUnitCost.Round(domain.UnitCostScale)),
			Value: amount(domain.LineAmount(r.Qty, r.AvgUnitCost)), UpdatedAt: r.UpdatedAt,
		}
		if r.Key.BatchID != nil {
			st := expiryState(b, res.Today, res.NearDays)
			item.ExpiryState = &st
		}
		items[i] = item
	}
	return ListStockBalances200JSONResponse{
		Items: items, TotalQty: qty(res.Totals.Qty), Page: page.Number, PageSize: page.Size,
		TotalItems: res.Totals.Rows, TotalPages: page.TotalPages(res.Totals.Rows),
	}, nil
}

// GetStockCard mengembalikan Kartu Stok.
func (h *InventoryHandler) GetStockCard(ctx context.Context, req GetStockCardRequestObject) (GetStockCardResponseObject, error) {
	p := req.Params
	f := service.CardFilter{
		ProductID: p.ProductId, LocationID: p.LocationId, BatchID: p.BatchId,
		DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo),
	}
	if p.Status != nil {
		f.Status = domain.StockStatus(*p.Status)
	}
	page := paging.New(p.Page, p.PageSize)
	card, err := h.svc.StockCard(ctx, f, page)
	if err != nil {
		return nil, err
	}
	items := make([]StockCardLine, len(card.Lines))
	for i, l := range card.Lines {
		in, out := decimal.Zero, decimal.Zero
		if l.Qty.IsPositive() {
			in = l.Qty
		} else {
			out = l.Qty.Neg()
		}
		items[i] = StockCardLine{
			MovementId: l.MovementID, BusinessDate: toDate(l.BusinessDate), CreatedAt: l.CreatedAt, SourceType: l.SourceType,
			SourceId: l.SourceID, SourceLabel: l.SourceLabel, MovementType: StockMovementType(l.Type),
			MovementTypeLabel: l.Type.Label(), Location: locationRef(card.Refs.Location(l.LocationID)), BatchId: l.BatchID,
			BatchNumber: optStr(l.BatchNumber), Status: StockStatus(l.Status), StatusLabel: l.Status.Label(),
			QtyIn: qty(in), QtyOut: qty(out), UnitCost: unitCost(l.UnitCost), CostAmount: amount(l.CostAmount),
			RunningQty: qty(l.RunningQty), RunningValue: amount(l.RunningAmount), BatchReason: optStr(l.BatchReason), Note: l.Note,
		}
	}
	s := card.Summary
	return GetStockCard200JSONResponse{
		Product: productRef(card.Product), OpeningQty: qty(s.OpeningQty), OpeningValue: amount(s.OpeningAmount),
		QtyIn: qty(s.QtyIn), QtyOut: qty(s.QtyOut), ClosingQty: qty(s.OpeningQty.Add(s.NetQty)),
		ClosingValue: amount(s.OpeningAmount.Add(s.NetAmount)), Items: items, Page: page.Number, PageSize: page.Size,
		TotalItems: s.Lines, TotalPages: page.TotalPages(s.Lines),
	}, nil
}

// ListBatches mengembalikan Batch & Kedaluwarsa.
func (h *InventoryHandler) ListBatches(ctx context.Context, req ListBatchesRequestObject) (ListBatchesResponseObject, error) {
	p := req.Params
	q := service.BatchQuery{ProductID: p.ProductId, WithStock: p.WithStock == nil || *p.WithStock}
	if p.Q != nil {
		q.Search = *p.Q
	}
	if p.ExpiryState != nil {
		q.State = domain.ExpiryState(*p.ExpiryState)
	}
	if p.NearExpiryDays != nil {
		q.NearDays = *p.NearExpiryDays
	}
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListBatches(ctx, q, page)
	if err != nil {
		return nil, err
	}
	items := make([]BatchSummary, len(res.Items))
	for i, b := range res.Items {
		item := BatchSummary{
			Id: b.ID, Product: productRef(res.Refs.Product(b.ProductID)), BatchNumber: b.Number,
			ProductionDate: optDate(b.ProductionDate), ExpiryDate: optDate(b.ExpiryDate),
			ExpiryState:  expiryState(b.Batch, res.Today, res.NearDays),
			QtyAvailable: qty(b.Qty[domain.StatusAvailable]), QtyQuarantine: qty(b.Qty[domain.StatusQuarantine]),
			QtyInTransit: qty(b.Qty[domain.StatusInTransit]), QtyReserved: qty(b.Qty[domain.StatusReserved]),
			QtyTotal: qty(b.Total), CreatedAt: b.CreatedAt,
		}
		if days, ok := b.DaysToExpiry(res.Today); ok {
			item.DaysToExpiry = &days
		}
		items[i] = item
	}
	return ListBatches200JSONResponse{
		Items: items, NearExpiryDays: res.NearDays, Page: page.Number, PageSize: page.Size,
		TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func suggested(c domain.Candidate, take decimal.Decimal) SuggestedBatch {
	return SuggestedBatch{
		BatchId: c.BatchID, BatchNumber: c.BatchNumber, ProductionDate: optDate(c.ProductionDate),
		ExpiryDate: optDate(c.ExpiryDate), AvailableQty: qty(c.Qty), SuggestedQty: qty(take),
	}
}

// SuggestBatches mengembalikan saran batch FEFO.
func (h *InventoryHandler) SuggestBatches(ctx context.Context, req SuggestBatchesRequestObject) (SuggestBatchesResponseObject, error) {
	p := req.Params
	q, err := decimal.NewFromString(p.Qty)
	if err != nil {
		return nil, apperr.Validation(map[string]string{"qty": "Qty tidak valid"})
	}
	s, err := h.svc.SuggestBatchesForUser(ctx, p.ProductId, p.LocationId, q)
	if err != nil {
		return nil, err
	}
	out := BatchSuggestion{
		Product: productRef(s.Product), Location: locationRef(s.Location), RequestedQty: qty(s.RequestedQty),
		AllocatedQty: qty(s.AllocatedQty), ShortfallQty: qty(s.Shortfall), QuarantineQty: qty(s.Quarantine),
		Items: make([]SuggestedBatch, len(s.Items)), Expired: make([]SuggestedBatch, len(s.Expired)),
	}
	for i, it := range s.Items {
		out.Items[i] = suggested(it.Candidate, it.SuggestedQty)
	}
	for i, c := range s.Expired {
		out.Expired[i] = suggested(c, decimal.Zero)
	}
	return SuggestBatches200JSONResponse(out), nil
}

func toLayer(l postgres.LayerRow, refs service.Refs) CostLayer {
	return CostLayer{
		Id: l.ID, Product: productRef(refs.Product(l.ProductID)), BatchId: l.BatchID, BatchNumber: optStr(l.BatchNumber),
		ExpiryDate: optDate(l.ExpiryDate), SourceType: l.SourceType, SourceId: l.SourceID, SourceLabel: l.SourceLabel,
		BusinessDate: toDate(l.BusinessDate), ReceivedQty: qty(l.ReceivedQty), RemainingQty: qty(l.RemainingQty),
		ConsumedQty: qty(l.ReceivedQty.Sub(l.RemainingQty)), UnitCost: unitCost(l.UnitCost),
		RemainingValue: amount(domain.LineAmount(l.RemainingQty, l.UnitCost)), CreatedAt: l.CreatedAt,
	}
}

// ListCostLayers mengembalikan HPP per Batch.
func (h *InventoryHandler) ListCostLayers(ctx context.Context, req ListCostLayersRequestObject) (ListCostLayersResponseObject, error) {
	p := req.Params
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListCostLayers(ctx, service.LayerFilter{ProductID: p.ProductId, BatchID: p.BatchId, OpenOnly: p.OpenOnly != nil && *p.OpenOnly}, page)
	if err != nil {
		return nil, err
	}
	items := make([]CostLayer, len(res.Items))
	for i, l := range res.Items {
		items[i] = toLayer(l, res.Refs)
	}
	return ListCostLayers200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

// ListCostLayerConsumptions mengembalikan jejak konsumsi satu cost layer.
func (h *InventoryHandler) ListCostLayerConsumptions(ctx context.Context, req ListCostLayerConsumptionsRequestObject) (ListCostLayerConsumptionsResponseObject, error) {
	res, err := h.svc.CostLayerConsumptions(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	items := make([]CostLayerConsumption, len(res.Items))
	for i, c := range res.Items {
		items[i] = CostLayerConsumption{
			Id: c.ID, MovementId: c.MovementID, BusinessDate: toDate(c.BusinessDate), SourceType: c.SourceType,
			SourceId: c.SourceID, SourceLabel: c.SourceLabel, Location: locationRef(res.Refs.Location(c.LocationID)),
			Qty: qty(c.Qty), UnitCost: unitCost(c.UnitCost), Amount: amount(domain.LineAmount(c.Qty, c.UnitCost)),
		}
	}
	return ListCostLayerConsumptions200JSONResponse{Layer: toLayer(res.Layer, res.Refs), Items: items}, nil
}

// ListBundleAvailability mengembalikan stok tersedia bundel virtual.
func (h *InventoryHandler) ListBundleAvailability(ctx context.Context, req ListBundleAvailabilityRequestObject) (ListBundleAvailabilityResponseObject, error) {
	ids := make([]uuid.UUID, len(req.Params.BundleId))
	copy(ids, req.Params.BundleId)
	res, err := h.svc.BundleAvailabilityForUser(ctx, ids, req.Params.LocationId)
	if err != nil {
		return nil, err
	}
	items := make([]BundleAvailability, len(res))
	for i, b := range res {
		comps := make([]BundleComponentAvailability, len(b.Components))
		for j, c := range b.Components {
			comps[j] = BundleComponentAvailability{ProductId: c.ProductID, QtyPerBundle: qty(c.QtyPerBundle), AvailableQty: qty(c.AvailableQty)}
		}
		items[i] = BundleAvailability{BundleId: b.BundleID, AvailableQty: qty(b.AvailableQty), Components: comps}
	}
	return ListBundleAvailability200JSONResponse{Items: items}, nil
}
