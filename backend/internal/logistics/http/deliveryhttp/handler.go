// Package deliveryhttp adalah handler HTTP master kurir, Delivery Order, dan pengiriman (R1.3).
package deliveryhttp

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/logistics/domain"
	"balans/internal/logistics/postgres"
	"balans/internal/logistics/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

//go:generate go tool oapi-codegen -config ../../../../api/codegen/delivery.yaml ../../../../api/openapi.bundle.yaml

// DeliveryHandler mengimplementasikan StrictServerInterface hasil generate.
type DeliveryHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*DeliveryHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *DeliveryHandler { return &DeliveryHandler{svc: svc} }

// Register mendaftarkan route.
func Register(r chi.Router, h *DeliveryHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func strOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func qty(d decimal.Decimal) DeliveryQuantity { return d.StringFixed(domain.QtyScale) }

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func pageOf[T any](res paging.Result[T]) (int, int, int64, int) {
	return res.Page.Number, res.Page.Size, res.Total, res.Page.TotalPages(res.Total)
}

func toCourier(c domain.Courier) Courier {
	return Courier{Id: c.ID, Code: c.Code, Name: c.Name, IsInternal: c.IsInternal, IsActive: c.IsActive, Version: c.Version}
}

// --- Kurir ------------------------------------------------------------------------------------

// ListCouriers mengembalikan master kurir.
func (h *DeliveryHandler) ListCouriers(ctx context.Context, req ListCouriersRequestObject) (ListCouriersResponseObject, error) {
	list, err := h.svc.ListCouriers(ctx, req.Params.Active != nil && *req.Params.Active)
	if err != nil {
		return nil, err
	}
	items := make([]Courier, len(list))
	for i, c := range list {
		items[i] = toCourier(c)
	}
	return ListCouriers200JSONResponse{Items: items}, nil
}

// ListCourierUsers mengembalikan Merchandiser aktif untuk kurir internal.
func (h *DeliveryHandler) ListCourierUsers(ctx context.Context, _ ListCourierUsersRequestObject) (ListCourierUsersResponseObject, error) {
	list, err := h.svc.CourierUsers(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]CourierUser, len(list))
	for i, u := range list {
		items[i] = CourierUser{Id: u.ID, Name: u.Name}
	}
	return ListCourierUsers200JSONResponse{Items: items}, nil
}

// CreateCourier menambah kurir.
func (h *DeliveryHandler) CreateCourier(ctx context.Context, req CreateCourierRequestObject) (CreateCourierResponseObject, error) {
	b := req.Body
	c, err := h.svc.CreateCourier(ctx, domain.CourierInput{Code: b.Code, Name: b.Name, IsInternal: b.IsInternal, IsActive: b.IsActive})
	if err != nil {
		return nil, err
	}
	return CreateCourier201JSONResponse(toCourier(c)), nil
}

// UpdateCourier mengubah kurir.
func (h *DeliveryHandler) UpdateCourier(ctx context.Context, req UpdateCourierRequestObject) (UpdateCourierResponseObject, error) {
	b := req.Body
	c, err := h.svc.UpdateCourier(ctx, req.Id, b.Version, domain.CourierInput{Name: b.Name, IsInternal: b.IsInternal, IsActive: b.IsActive})
	if err != nil {
		return nil, err
	}
	return UpdateCourier200JSONResponse(toCourier(c)), nil
}

// --- Delivery Order ---------------------------------------------------------------------------

func toSummary(s postgres.DeliveryOrderSummary) DeliveryOrderSummary {
	d := s.DeliveryOrder
	return DeliveryOrderSummary{
		Id: d.ID, Number: optStr(d.Number), DeliveryType: DeliveryOrderType(d.Type), SourceType: d.SourceType,
		SourceId: d.SourceID, SourceLabel: d.SourceLabel, RecipientName: d.RecipientName, BusinessDate: toDate(d.BusinessDate),
		Status: DeliveryOrderStatus(d.Status), LineCount: s.LineCount, CourierName: optStr(s.CourierName),
		TrackingNumber: optStr(s.TrackingNumber), MarketplaceAwb: optStr(s.MarketplaceAWB), CreatedAt: d.CreatedAt,
	}
}

type view struct {
	refs service.Refs
}

func (v view) product(id uuid.UUID) DeliveryProductRef {
	p, ok := v.refs.Products[id]
	if !ok {
		return DeliveryProductRef{Id: id, Sku: "?", Name: "Produk tidak ditemukan"}
	}
	return DeliveryProductRef{Id: id, Sku: p.SKU, Name: p.Name, Uom: p.UOM, Barcode: optStr(p.Barcode)}
}

func (v view) batch(id *uuid.UUID) *DeliveryBatchRef {
	if id == nil {
		return nil
	}
	b, ok := v.refs.Batches[*id]
	if !ok {
		return &DeliveryBatchRef{Id: *id, Number: "?"}
	}
	out := &DeliveryBatchRef{Id: b.ID, Number: b.Number}
	if b.ExpiryDate != "" {
		if d, err := clock.ParseBusinessDate(b.ExpiryDate); err == nil {
			v := toDate(d)
			out.ExpiryDate = &v
		}
	}
	return out
}

func toShipment(sh domain.ShipmentRecord, c domain.Courier) Shipment {
	proofs := sh.ProofFileIDs
	if proofs == nil {
		proofs = []uuid.UUID{}
	}
	return Shipment{
		Id: sh.ID, DeliveryOrderId: sh.DeliveryOrderID, Courier: toCourier(c), InternalCourierUserId: sh.InternalCourierUserID,
		InternalCourierName: sh.InternalCourierName, ServiceCode: sh.ServiceCode, TrackingNumber: sh.TrackingNumber,
		MarketplaceAwb: sh.MarketplaceAWB, ShippingCost: sh.ShippingCost.String(), PackageCount: sh.PackageCount,
		Status: ShipmentStatus(sh.Status), ShippedAt: sh.ShippedAt, DeliveredAt: sh.DeliveredAt, ReceivedByName: sh.ReceivedByName,
		DeliveryNote: sh.DeliveryNote, ProofFileIds: proofs, ConfirmedByName: sh.ConfirmedByName, Version: sh.Version,
	}
}

func toDetail(ctx context.Context, dd service.DeliveryDetail) DeliveryOrder {
	d := dd.DeliveryOrder
	v := view{refs: dd.Refs}
	lines := make([]DeliveryOrderLine, len(d.Lines))
	for i, l := range d.Lines {
		batches := make([]DeliveryOrderBatch, len(l.Batches))
		for j, b := range l.Batches {
			batches[j] = DeliveryOrderBatch{Batch: v.batch(b.BatchID), Qty: qty(b.Quantity())}
		}
		lines[i] = DeliveryOrderLine{
			Id: l.ID, LineNo: l.LineNo, SourceLineId: l.SourceLineID, Product: v.product(l.ProductID), Qty: qty(l.Qty),
			ShippedQty: qty(l.ShippedQty), Batches: batches,
		}
	}
	// Baris cetak DO: batch dari Outbound (lulus QC atau masih disiapkan), selain itu rencana batch DO.
	printLines := []DeliveryPrintLine{}
	labels := []DeliveryProductLabel{}
	addPrint := func(product uuid.UUID, batch *uuid.UUID, q decimal.Decimal) {
		if !q.IsPositive() {
			return
		}
		printLines = append(printLines, DeliveryPrintLine{Product: v.product(product), Batch: v.batch(batch), Qty: qty(q)})
		lbl := DeliveryProductLabel{Product: v.product(product), Batch: v.batch(batch), Qty: qty(q)}
		if pc, ok := dd.Refs.Partner[product]; ok {
			lbl.PartnerArticleCode, lbl.PartnerBarcode = optStr(pc.ArticleCode), optStr(pc.Barcode)
		} else {
			lbl.PartnerCodeMissing = d.PartnerID != nil
		}
		labels = append(labels, lbl)
	}
	packages := []DeliveryPackage{}
	var outbound *DeliveryOutboundRef
	if ob := dd.Outbound; ob != nil {
		held := false
		for _, l := range ob.Lines {
			switch l.Status {
			case warehouse.OutboundPassed:
				addPrint(l.ProductID, l.BatchID, l.PassedQty)
			case warehouse.OutboundPending, warehouse.OutboundPicked:
				addPrint(l.ProductID, l.BatchID, l.AllocatedQty)
			case warehouse.OutboundHeld:
				held = true
			case warehouse.OutboundRejected, warehouse.OutboundDropped:
			}
		}
		for _, p := range ob.Packages {
			items := make([]DeliveryPackageItem, len(p.Contents))
			for j, it := range p.Contents {
				q, err := decimal.NewFromString(it.Qty)
				if err != nil {
					q = decimal.Zero
				}
				items[j] = DeliveryPackageItem{Product: v.product(it.ProductID), Batch: v.batch(it.BatchID), Qty: qty(q)}
			}
			packages = append(packages, DeliveryPackage{PackageNumber: p.Number, PackageCount: p.Count, WeightGrams: p.WeightGrams, Items: items})
		}
		outbound = &DeliveryOutboundRef{Id: ob.ID, Number: ob.Number, Status: DeliveryOutboundRefStatus(ob.Status), NeedsAttention: held}
	} else {
		for _, l := range d.Lines {
			if len(l.Batches) == 0 {
				addPrint(l.ProductID, nil, l.Qty)
			}
			for _, b := range l.Batches {
				addPrint(l.ProductID, b.BatchID, b.Quantity())
			}
		}
	}
	loc := dd.Refs.Location
	out := DeliveryOrder{
		Id: d.ID, Number: optStr(d.Number), DeliveryType: DeliveryOrderType(d.Type), SourceType: d.SourceType,
		SourceId: d.SourceID, SourceLabel: d.SourceLabel, RecipientName: d.RecipientName, BusinessDate: toDate(d.BusinessDate),
		Status: DeliveryOrderStatus(d.Status), LineCount: len(d.Lines), CreatedAt: d.CreatedAt, SalesOrderId: d.SalesOrderID,
		PartnerId: d.PartnerID, CustomerId: d.CustomerID, RecipientPhone: d.RecipientPhone, ShipToAddress: d.ShipToAddress,
		Location: WhLocationRef{
			Id: loc.ID, Code: loc.Code, Name: loc.Name, LocationType: LocationType(loc.Type), LocationTypeLabel: loc.Type.Label(),
			IsExternal: loc.IsExternal(), WarehouseName: optStr(loc.WarehouseName),
		},
		Note: d.Note, ReleasedByName: d.ReleasedByName, ReleasedAt: d.ReleasedAt, OutboundCompletedAt: d.OutboundCompletedAt,
		ShippedAt: d.ShippedAt, DeliveredAt: d.DeliveredAt, CancelReason: d.CancelReason, Outbound: outbound, Lines: lines,
		PrintLines: printLines, Packages: packages, ProductLabels: labels, Events: []ShipmentEvent{}, Version: d.Version,
	}
	if sh := dd.Shipment; sh != nil {
		c := dd.Refs.Couriers[sh.CourierID]
		s := toShipment(*sh, c)
		out.Shipment = &s
		out.CourierName, out.TrackingNumber, out.MarketplaceAwb = optStr(c.Name), optStr(sh.TrackingNumber), optStr(sh.MarketplaceAWB)
	}
	for _, e := range dd.Events {
		out.Events = append(out.Events, ShipmentEvent{
			Id: e.ID, Status: ShipmentStatus(e.Status), OccurredAt: e.OccurredAt, Source: ShipmentEventSource(e.Source),
			Note: e.Note, CreatedByName: e.CreatedByName,
		})
	}
	acts := []DeliveryOrderAllowedActions{}
	for _, a := range service.DeliveryActions(ctx, d, dd.Shipment) {
		acts = append(acts, DeliveryOrderAllowedActions(a))
	}
	out.AllowedActions = acts
	return out
}

func (h *DeliveryHandler) detail(ctx context.Context, id uuid.UUID) (DeliveryOrder, error) {
	dd, err := h.svc.GetDeliveryOrder(ctx, id)
	if err != nil {
		return DeliveryOrder{}, err
	}
	return toDetail(ctx, dd), nil
}

// ListDeliveryOrders mengembalikan daftar DO.
func (h *DeliveryHandler) ListDeliveryOrders(ctx context.Context, req ListDeliveryOrdersRequestObject) (ListDeliveryOrdersResponseObject, error) {
	p := req.Params
	f := service.DeliveryOrderFilter{Search: strOr(p.Q), SourceID: p.SourceId}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	if p.DeliveryType != nil {
		f.Type = string(*p.DeliveryType)
	}
	res, err := h.svc.ListDeliveryOrders(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]DeliveryOrderSummary, len(res.Items))
	for i, it := range res.Items {
		items[i] = toSummary(it)
	}
	out := ListDeliveryOrders200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// CreateDeliveryOrder membuat DO dari Sales Order.
func (h *DeliveryHandler) CreateDeliveryOrder(ctx context.Context, req CreateDeliveryOrderRequestObject) (CreateDeliveryOrderResponseObject, error) {
	d, err := h.svc.CreateFromSalesOrder(ctx, req.Body.SalesOrderId, strOr(req.Body.Note))
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	return CreateDeliveryOrder201JSONResponse(out), nil
}

// GetDeliveryOrder mengembalikan detail DO.
func (h *DeliveryHandler) GetDeliveryOrder(ctx context.Context, req GetDeliveryOrderRequestObject) (GetDeliveryOrderResponseObject, error) {
	out, err := h.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetDeliveryOrder200JSONResponse(out), nil
}

// ReleaseDeliveryOrder me-release DO.
func (h *DeliveryHandler) ReleaseDeliveryOrder(ctx context.Context, req ReleaseDeliveryOrderRequestObject) (ReleaseDeliveryOrderResponseObject, error) {
	if _, err := h.svc.Release(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return ReleaseDeliveryOrder200JSONResponse(out), nil
}

// ShipDeliveryOrder mencatat serah ke kurir.
func (h *DeliveryHandler) ShipDeliveryOrder(ctx context.Context, req ShipDeliveryOrderRequestObject) (ShipDeliveryOrderResponseObject, error) {
	b := req.Body
	in := service.ShipInput{
		CourierID: b.CourierId, InternalCourierUserID: b.InternalCourierUserId, ServiceCode: strOr(b.ServiceCode),
		TrackingNumber: strOr(b.TrackingNumber), MarketplaceAWB: strOr(b.MarketplaceAwb), ShippingCost: money.Zero,
	}
	if b.ShippingCost != nil && *b.ShippingCost != "" {
		c, err := money.Parse(*b.ShippingCost)
		if err != nil {
			return nil, apperr.Validation(map[string]string{"shippingCost": "Ongkos kirim tidak valid"})
		}
		in.ShippingCost = c
	}
	if _, err := h.svc.Ship(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return ShipDeliveryOrder200JSONResponse(out), nil
}

// DeliverDeliveryOrder mengonfirmasi barang diterima.
func (h *DeliveryHandler) DeliverDeliveryOrder(ctx context.Context, req DeliverDeliveryOrderRequestObject) (DeliverDeliveryOrderResponseObject, error) {
	b := req.Body
	in := service.DeliverInput{ReceivedByName: strOr(b.ReceivedByName), Note: strOr(b.Note)}
	if b.ProofFileIds != nil {
		in.ProofFileIDs = *b.ProofFileIds
	}
	if _, err := h.svc.Deliver(ctx, req.Id, in); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return DeliverDeliveryOrder200JSONResponse(out), nil
}

// CancelDeliveryOrder membatalkan DO.
func (h *DeliveryHandler) CancelDeliveryOrder(ctx context.Context, req CancelDeliveryOrderRequestObject) (CancelDeliveryOrderResponseObject, error) {
	if _, err := h.svc.Cancel(ctx, req.Id, req.Body.Version, req.Body.Reason); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return CancelDeliveryOrder200JSONResponse(out), nil
}

// --- Pengiriman -------------------------------------------------------------------------------

// ListShipments mengembalikan daftar pengiriman.
func (h *DeliveryHandler) ListShipments(ctx context.Context, req ListShipmentsRequestObject) (ListShipmentsResponseObject, error) {
	p := req.Params
	f := service.ShipmentFilter{Search: strOr(p.Q)}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, err := h.svc.ListShipments(ctx, f, p.Mine != nil && *p.Mine, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	couriers, err := h.svc.ListCouriers(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]domain.Courier{}
	for _, c := range couriers {
		byID[c.ID] = c
	}
	items := make([]ShipmentSummary, len(res.Items))
	for i, it := range res.Items {
		c, ok := byID[it.Shipment.CourierID]
		if !ok {
			c = domain.Courier{ID: it.Shipment.CourierID, Name: it.CourierName}
		}
		s := toShipment(it.Shipment, c)
		items[i] = ShipmentSummary{
			Id: s.Id, DeliveryOrderId: s.DeliveryOrderId, Courier: s.Courier, InternalCourierUserId: s.InternalCourierUserId,
			InternalCourierName: s.InternalCourierName, ServiceCode: s.ServiceCode, TrackingNumber: s.TrackingNumber,
			MarketplaceAwb: s.MarketplaceAwb, ShippingCost: s.ShippingCost, PackageCount: s.PackageCount, Status: s.Status,
			ShippedAt: s.ShippedAt, DeliveredAt: s.DeliveredAt, ReceivedByName: s.ReceivedByName, DeliveryNote: s.DeliveryNote,
			ProofFileIds: s.ProofFileIds, ConfirmedByName: s.ConfirmedByName, Version: s.Version,
			DeliveryOrderNumber: it.DeliveryOrderNumber, DeliveryType: DeliveryOrderType(it.DeliveryType),
			RecipientName: it.RecipientName, SourceLabel: it.SourceLabel,
		}
	}
	out := ListShipments200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// RecordShipmentEvent mencatat status pelacakan manual.
func (h *DeliveryHandler) RecordShipmentEvent(ctx context.Context, req RecordShipmentEventRequestObject) (RecordShipmentEventResponseObject, error) {
	sh, err := h.svc.TrackShipment(ctx, req.Id, domain.ShipmentStatus(req.Body.Status), strOr(req.Body.Note))
	if err != nil {
		return nil, err
	}
	couriers, err := h.svc.ListCouriers(ctx, false)
	if err != nil {
		return nil, err
	}
	c := domain.Courier{ID: sh.CourierID}
	for _, x := range couriers {
		if x.ID == sh.CourierID {
			c = x
		}
	}
	return RecordShipmentEvent200JSONResponse(toShipment(sh, c)), nil
}
