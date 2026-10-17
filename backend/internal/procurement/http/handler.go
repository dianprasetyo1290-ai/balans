package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/service"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/procurement.yaml ../../../api/openapi.bundle.yaml

// ProcurementHandler mengimplementasikan StrictServerInterface hasil generate.
type ProcurementHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*ProcurementHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *ProcurementHandler { return &ProcurementHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *ProcurementHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// --- konversi --------------------------------------------------------------------------------

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func optToDate(d *clock.BusinessDate) *BusinessDate {
	if d == nil {
		return nil
	}
	out := toDate(*d)
	return &out
}

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func optFromDate(d *BusinessDate) *clock.BusinessDate {
	if d == nil || d.IsZero() {
		return nil
	}
	out := fromDate(*d)
	return &out
}

func str(s *string) string {
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

func parseMoney(s Money, field string, fields map[string]string) money.Amount {
	if strings.TrimSpace(s) == "" {
		return money.Zero
	}
	a, err := money.Parse(s)
	if err != nil {
		fields[field] = "Nominal tidak valid"
	}
	return a
}

func parseQty(s PurchaseQuantity, field string, fields map[string]string) decimal.Decimal {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		fields[field] = "Qty tidak valid"
		return decimal.Zero
	}
	return d
}

func qty(d decimal.Decimal) PurchaseQuantity { return d.StringFixed(domain.QtyScale) }

func validation(fields map[string]string) error {
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func productRef(id *uuid.UUID, products map[uuid.UUID]service.ProductRef) *PurchaseProductRef {
	if id == nil {
		return nil
	}
	p, ok := products[*id]
	if !ok {
		return &PurchaseProductRef{Id: *id, Sku: "", Name: "(produk tidak ditemukan)", Uom: ""}
	}
	return &PurchaseProductRef{Id: p.ID, Sku: p.SKU, Name: p.Name, Uom: p.UOM}
}

func optOption(id *uuid.UUID, locs map[uuid.UUID]service.LocationRef) *PurchaseLocationRef {
	if id == nil {
		return nil
	}
	l, ok := locs[*id]
	if !ok {
		return &PurchaseLocationRef{Id: *id}
	}
	return &PurchaseLocationRef{Id: l.ID, Code: l.Code, Name: l.Name}
}

func options(list []service.Option) []PurchaseOption {
	out := make([]PurchaseOption, len(list))
	for i, o := range list {
		out[i] = PurchaseOption{Id: o.ID, Code: o.Code, Name: o.Name}
	}
	return out
}

func pageParams(res paging.Page, total int64) (int, int, int64, int) {
	return res.Number, res.Size, total, res.TotalPages(total)
}

// --- pilihan ---------------------------------------------------------------------------------

// ListProcurementDivisionOptions mengembalikan divisi aktif.
func (h *ProcurementHandler) ListProcurementDivisionOptions(ctx context.Context, _ ListProcurementDivisionOptionsRequestObject) (ListProcurementDivisionOptionsResponseObject, error) {
	list, err := h.svc.DivisionOptions(ctx)
	if err != nil {
		return nil, err
	}
	return ListProcurementDivisionOptions200JSONResponse(options(list)), nil
}

// ListProcurementCashBankAccounts mengembalikan akun Kas & Bank.
func (h *ProcurementHandler) ListProcurementCashBankAccounts(ctx context.Context, _ ListProcurementCashBankAccountsRequestObject) (ListProcurementCashBankAccountsResponseObject, error) {
	list, err := h.svc.CashBankAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return ListProcurementCashBankAccounts200JSONResponse(options(list)), nil
}

// --- Purchase Request ------------------------------------------------------------------------

func toRequestSummary(s service.RequestSummary) PurchaseRequestSummary {
	r := s.PurchaseRequest
	return PurchaseRequestSummary{
		Id: r.ID, Number: optStr(r.Number), DivisionId: r.DivisionID, DivisionName: s.DivisionName,
		RequestedById: r.RequestedBy, RequestedByName: s.RequesterName, BusinessDate: toDate(r.BusinessDate),
		NeededDate: optToDate(r.NeededDate), Note: r.Note, EstimatedTotal: r.EstimatedTotal.String(),
		Status: PurchaseRequestStatus(r.Status), CloseReason: r.CloseReason, SubmittedAt: r.SubmittedAt,
		DecidedAt: r.DecidedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
}

func toRequest(ctx context.Context, v service.RequestView) PurchaseRequest {
	s := toRequestSummary(service.RequestSummary{PurchaseRequest: v.PurchaseRequest, DivisionName: v.DivisionName, RequesterName: v.RequesterName})
	out := PurchaseRequest{
		Id: s.Id, Number: s.Number, DivisionId: s.DivisionId, DivisionName: s.DivisionName, RequestedById: s.RequestedById,
		RequestedByName: s.RequestedByName, BusinessDate: s.BusinessDate, NeededDate: s.NeededDate, Note: s.Note,
		EstimatedTotal: s.EstimatedTotal, Status: s.Status, CloseReason: s.CloseReason, SubmittedAt: s.SubmittedAt,
		DecidedAt: s.DecidedAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
		Lines: make([]PurchaseRequestLine, len(v.Lines)), Orders: make([]PurchaseRequestOrderRef, len(v.Orders)),
	}
	for i, l := range v.Lines {
		out.Lines[i] = PurchaseRequestLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products), Description: l.Description,
			Qty: qty(l.Qty), EstimatedPrice: l.EstimatedPrice.String(), EstimatedAmount: l.EstimatedAmount().String(),
			FulfilledQty: qty(l.FulfilledQty), RemainingQty: qty(l.RemainingQty()), Note: l.Note,
		}
	}
	for i, o := range v.Orders {
		out.Orders[i] = PurchaseRequestOrderRef{
			PurchaseOrderId: o.OrderID, PurchaseOrderNumber: optStr(o.OrderNumber), Status: PurchaseOrderStatus(o.OrderStatus),
			PurchaseRequestLineId: o.RequestLineID, OrderedQty: qty(o.OrderedQty), ReceivedQty: qty(o.ReceivedQty),
		}
	}
	for _, a := range service.RequestActions(ctx, v.PurchaseRequest) {
		out.AllowedActions = append(out.AllowedActions, PurchaseRequestAllowedActions(a))
	}
	if out.AllowedActions == nil {
		out.AllowedActions = []PurchaseRequestAllowedActions{}
	}
	return out
}

func requestInput(f PurchaseRequestFields) (domain.RequestInput, error) {
	fields := map[string]string{}
	in := domain.RequestInput{
		DivisionID: f.DivisionId, BusinessDate: fromDate(f.BusinessDate), NeededDate: optFromDate(f.NeededDate), Note: str(f.Note),
	}
	for i, l := range f.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		in.Lines = append(in.Lines, domain.RequestLineInput{
			ProductID: l.ProductId, Description: str(l.Description), Qty: parseQty(l.Qty, p+"qty", fields),
			EstimatedPrice: parseMoney(l.EstimatedPrice, p+"estimatedPrice", fields), Note: str(l.Note),
		})
	}
	return in, validation(fields)
}

// ListPurchaseRequests mengembalikan daftar PR.
func (h *ProcurementHandler) ListPurchaseRequests(ctx context.Context, req ListPurchaseRequestsRequestObject) (ListPurchaseRequestsResponseObject, error) {
	p := req.Params
	f := service.RequestFilter{
		DivisionID: p.DivisionId, DateFrom: optFromDate(p.DateFrom), DateTo: optFromDate(p.DateTo), Search: str(p.Q),
		Sort: paging.ParseSort(p.Sort, service.RequestSortFields, paging.Sort{}),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.Mine != nil && *p.Mine {
		id := actor.UserID(ctx)
		f.RequestedBy = &id
	}
	res, err := h.svc.ListRequests(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]PurchaseRequestSummary, len(res.Items))
	for i, r := range res.Items {
		items[i] = toRequestSummary(r)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListPurchaseRequests200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreatePurchaseRequest menyimpan draft PR.
func (h *ProcurementHandler) CreatePurchaseRequest(ctx context.Context, req CreatePurchaseRequestRequestObject) (CreatePurchaseRequestResponseObject, error) {
	in, err := requestInput(*req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateRequest(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePurchaseRequest201JSONResponse(toRequest(ctx, v)), nil
}

// GetPurchaseRequest mengembalikan detail PR.
func (h *ProcurementHandler) GetPurchaseRequest(ctx context.Context, req GetPurchaseRequestRequestObject) (GetPurchaseRequestResponseObject, error) {
	v, err := h.svc.GetRequest(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// UpdatePurchaseRequest mengubah draft PR.
func (h *ProcurementHandler) UpdatePurchaseRequest(ctx context.Context, req UpdatePurchaseRequestRequestObject) (UpdatePurchaseRequestResponseObject, error) {
	b := req.Body
	in, err := requestInput(PurchaseRequestFields{
		DivisionId: b.DivisionId, BusinessDate: b.BusinessDate, NeededDate: b.NeededDate, Note: b.Note, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateRequest(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// SubmitPurchaseRequest mengajukan PR.
func (h *ProcurementHandler) SubmitPurchaseRequest(ctx context.Context, req SubmitPurchaseRequestRequestObject) (SubmitPurchaseRequestResponseObject, error) {
	v, err := h.svc.SubmitRequest(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitPurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// RevisePurchaseRequest mengembalikan PR ditolak ke Draft.
func (h *ProcurementHandler) RevisePurchaseRequest(ctx context.Context, req RevisePurchaseRequestRequestObject) (RevisePurchaseRequestResponseObject, error) {
	v, err := h.svc.ReviseRequest(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return RevisePurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// CancelPurchaseRequest membatalkan draft PR.
func (h *ProcurementHandler) CancelPurchaseRequest(ctx context.Context, req CancelPurchaseRequestRequestObject) (CancelPurchaseRequestResponseObject, error) {
	v, err := h.svc.CancelRequest(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return CancelPurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// ClosePurchaseRequest menutup PR.
func (h *ProcurementHandler) ClosePurchaseRequest(ctx context.Context, req ClosePurchaseRequestRequestObject) (ClosePurchaseRequestResponseObject, error) {
	v, err := h.svc.CloseRequest(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return ClosePurchaseRequest200JSONResponse(toRequest(ctx, v)), nil
}

// --- Purchase Order --------------------------------------------------------------------------

func toOrderSummary(o domain.PurchaseOrder, divisionName string) PurchaseOrderSummary {
	return PurchaseOrderSummary{
		Id: o.ID, Number: optStr(o.Number), OrderType: PurchaseOrderType(o.Type), OrderTypeLabel: o.Type.Label(),
		SupplierId: o.SupplierID, SupplierCode: o.SupplierCode, SupplierName: o.SupplierName, DivisionId: o.DivisionID,
		DivisionName: divisionName, BusinessDate: toDate(o.BusinessDate), ExpectedDate: toDate(o.ExpectedDate),
		PaymentTermDays: o.PaymentTermDays, TotalAmount: o.Total.String(), Status: PurchaseOrderStatus(o.Status),
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt, Version: o.Version,
	}
}

func toOrder(ctx context.Context, v service.OrderView) PurchaseOrder {
	s := toOrderSummary(v.PurchaseOrder, v.DivisionName)
	paid, outstanding := v.AdvanceTotals()
	out := PurchaseOrder{
		Id: s.Id, Number: s.Number, OrderType: s.OrderType, OrderTypeLabel: s.OrderTypeLabel, SupplierId: s.SupplierId,
		SupplierCode: s.SupplierCode, SupplierName: s.SupplierName, DivisionId: s.DivisionId, DivisionName: s.DivisionName,
		BusinessDate: s.BusinessDate, ExpectedDate: s.ExpectedDate, PaymentTermDays: s.PaymentTermDays,
		TotalAmount: s.TotalAmount, Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
		Note: v.Note, SubtotalAmount: v.Subtotal.String(), CloseReason: v.CloseReason, CancelReason: v.CancelReason,
		SubmittedAt: v.SubmittedAt, ApprovedAt: v.ApprovedAt, CreatedByName: v.CreatedByName,
		TollLocation: optOption(v.TollLocationID, v.Locations), MaterialSourceLocation: optOption(v.MaterialSourceLocationID, v.Locations),
		Lines: make([]PurchaseOrderLine, len(v.Lines)), Materials: make([]PurchaseOrderMaterial, len(v.Materials)),
		Advances: make([]PurchaseAdvance, len(v.Advances)), AdvancePaidAmount: paid.String(),
		AdvanceOutstandingAmount: outstanding.String(), AllowedActions: []PurchaseOrderAllowedActions{},
	}
	if v.Supplier != nil {
		sup := v.Supplier
		out.Supplier = &PurchaseOrderSupplier{
			Id: sup.ID, Code: sup.Code, Name: sup.Name, Address: sup.Address, Phone: sup.Phone, Email: sup.Email, PicName: sup.PICName,
		}
	}
	for i, l := range v.Lines {
		line := PurchaseOrderLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products), Description: l.Description,
			OrderedQty: qty(l.OrderedQty), UnitPrice: l.UnitPrice.String(), Amount: l.Amount.String(),
			ReceivedQty: qty(l.ReceivedQty), BilledQty: qty(l.BilledQty), OutstandingQty: qty(l.OutstandingQty()),
			PurchaseRequestLineId: l.PurchaseRequestLineID, Note: l.Note,
		}
		if l.PurchaseRequestLineID != nil {
			if ref, ok := v.Requests[*l.PurchaseRequestLineID]; ok {
				id := ref.RequestID
				line.PurchaseRequestId, line.PurchaseRequestNumber = &id, optStr(ref.RequestNumber)
			}
		}
		out.Lines[i] = line
	}
	for i, m := range v.Materials {
		id := m.ProductID
		out.Materials[i] = PurchaseOrderMaterial{
			Id: m.ID, LineNo: m.LineNo, Product: *productRef(&id, v.Products), PlannedQty: qty(m.PlannedQty), Note: m.Note,
		}
	}
	for i, a := range v.Advances {
		out.Advances[i] = toAdvance(ctx, a)
	}
	for _, a := range service.OrderActions(ctx, v.PurchaseOrder) {
		out.AllowedActions = append(out.AllowedActions, PurchaseOrderAllowedActions(a))
	}
	return out
}

func orderInput(t domain.OrderType, f PurchaseOrderFields) (domain.OrderInput, error) {
	fields := map[string]string{}
	in := domain.OrderInput{
		Type: t, SupplierID: f.SupplierId, DivisionID: f.DivisionId, BusinessDate: fromDate(f.BusinessDate),
		ExpectedDate: fromDate(f.ExpectedDate), PaymentTermDays: f.PaymentTermDays, Note: str(f.Note),
		MaterialSourceLocationID: f.MaterialSourceLocationId,
	}
	for i, l := range f.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		in.Lines = append(in.Lines, domain.OrderLineInput{
			ProductID: l.ProductId, Description: str(l.Description), Qty: parseQty(l.Qty, p+"qty", fields),
			UnitPrice: parseMoney(l.UnitPrice, p+"unitPrice", fields), PurchaseRequestLineID: l.PurchaseRequestLineId,
			Note: str(l.Note),
		})
	}
	if f.Materials != nil {
		for i, m := range *f.Materials {
			in.Materials = append(in.Materials, domain.OrderMaterialInput{
				ProductID: m.ProductId, PlannedQty: parseQty(m.PlannedQty, fmt.Sprintf("materials.%d.plannedQty", i), fields),
				Note: str(m.Note),
			})
		}
	}
	return in, validation(fields)
}

// ListPurchaseOrders mengembalikan daftar PO.
func (h *ProcurementHandler) ListPurchaseOrders(ctx context.Context, req ListPurchaseOrdersRequestObject) (ListPurchaseOrdersResponseObject, error) {
	p := req.Params
	f := service.OrderFilter{
		SupplierID: p.SupplierId, DivisionID: p.DivisionId, DateFrom: optFromDate(p.DateFrom), DateTo: optFromDate(p.DateTo),
		Search: str(p.Q), Sort: paging.ParseSort(p.Sort, service.OrderSortFields, paging.Sort{}),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.OrderType != nil {
		for _, t := range *p.OrderType {
			f.Types = append(f.Types, domain.OrderType(t))
		}
	}
	res, err := h.svc.ListOrders(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]PurchaseOrderSummary, len(res.Items))
	for i, o := range res.Items {
		items[i] = toOrderSummary(o.PurchaseOrder, o.DivisionName)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListPurchaseOrders200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreatePurchaseOrder menyimpan draft PO.
func (h *ProcurementHandler) CreatePurchaseOrder(ctx context.Context, req CreatePurchaseOrderRequestObject) (CreatePurchaseOrderResponseObject, error) {
	b := req.Body
	in, err := orderInput(domain.OrderType(b.OrderType), PurchaseOrderFields{
		SupplierId: b.SupplierId, DivisionId: b.DivisionId, BusinessDate: b.BusinessDate, ExpectedDate: b.ExpectedDate,
		PaymentTermDays: b.PaymentTermDays, Note: b.Note, MaterialSourceLocationId: b.MaterialSourceLocationId,
		Lines: b.Lines, Materials: b.Materials,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateOrder(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePurchaseOrder201JSONResponse(toOrder(ctx, v)), nil
}

// GetPurchaseOrder mengembalikan detail PO.
func (h *ProcurementHandler) GetPurchaseOrder(ctx context.Context, req GetPurchaseOrderRequestObject) (GetPurchaseOrderResponseObject, error) {
	v, err := h.svc.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// UpdatePurchaseOrder mengubah draft PO. Jenis PO mengikuti PO yang ada.
func (h *ProcurementHandler) UpdatePurchaseOrder(ctx context.Context, req UpdatePurchaseOrderRequestObject) (UpdatePurchaseOrderResponseObject, error) {
	cur, err := h.svc.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in, err := orderInput(cur.Type, PurchaseOrderFields{
		SupplierId: b.SupplierId, DivisionId: b.DivisionId, BusinessDate: b.BusinessDate, ExpectedDate: b.ExpectedDate,
		PaymentTermDays: b.PaymentTermDays, Note: b.Note, MaterialSourceLocationId: b.MaterialSourceLocationId,
		Lines: b.Lines, Materials: b.Materials,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateOrder(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// SubmitPurchaseOrder mengajukan PO.
func (h *ProcurementHandler) SubmitPurchaseOrder(ctx context.Context, req SubmitPurchaseOrderRequestObject) (SubmitPurchaseOrderResponseObject, error) {
	v, err := h.svc.SubmitOrder(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitPurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// RevisePurchaseOrder mengembalikan PO ditolak ke Draft.
func (h *ProcurementHandler) RevisePurchaseOrder(ctx context.Context, req RevisePurchaseOrderRequestObject) (RevisePurchaseOrderResponseObject, error) {
	v, err := h.svc.ReviseOrder(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return RevisePurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// CancelPurchaseOrder membatalkan PO.
func (h *ProcurementHandler) CancelPurchaseOrder(ctx context.Context, req CancelPurchaseOrderRequestObject) (CancelPurchaseOrderResponseObject, error) {
	v, err := h.svc.CancelOrder(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelPurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// ClosePurchaseOrder menutup PO.
func (h *ProcurementHandler) ClosePurchaseOrder(ctx context.Context, req ClosePurchaseOrderRequestObject) (ClosePurchaseOrderResponseObject, error) {
	v, err := h.svc.CloseOrder(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return ClosePurchaseOrder200JSONResponse(toOrder(ctx, v)), nil
}

// --- Uang muka -------------------------------------------------------------------------------

func toAdvance(ctx context.Context, a domain.Advance) PurchaseAdvance {
	out := PurchaseAdvance{
		Id: a.ID, Number: optStr(a.Number), PurchaseOrderId: a.PurchaseOrderID, PurchaseOrderNumber: a.OrderNumber,
		SupplierId: a.SupplierID, SupplierName: a.SupplierName, DivisionId: a.DivisionID, Amount: a.Amount.String(),
		PaymentDate: toDate(a.PaymentDate), BankAccountCode: a.BankAccountCode, Note: a.Note,
		AppliedAmount: a.Applied.String(), OutstandingAmount: a.Outstanding().String(), Status: PurchaseAdvanceStatus(a.Status),
		JournalEntryId: a.JournalEntryID, ReversalJournalEntryId: a.ReversalJournalEntryID, CancelReason: a.CancelReason,
		PostedAt: a.PostedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, Version: a.Version,
		AllowedActions: []PurchaseAdvanceAllowedActions{},
	}
	for _, act := range service.AdvanceActions(ctx, a) {
		out.AllowedActions = append(out.AllowedActions, PurchaseAdvanceAllowedActions(act))
	}
	return out
}

func advanceInput(orderID uuid.UUID, f PurchaseAdvanceFields) (domain.AdvanceInput, error) {
	fields := map[string]string{}
	in := domain.AdvanceInput{
		PurchaseOrderID: orderID, Amount: parseMoney(f.Amount, "amount", fields), PaymentDate: fromDate(f.PaymentDate),
		BankAccountCode: f.BankAccountCode, Note: str(f.Note),
	}
	return in, validation(fields)
}

// ListPurchaseAdvances mengembalikan daftar uang muka.
func (h *ProcurementHandler) ListPurchaseAdvances(ctx context.Context, req ListPurchaseAdvancesRequestObject) (ListPurchaseAdvancesResponseObject, error) {
	p := req.Params
	f := service.AdvanceFilter{
		PurchaseOrderID: p.PurchaseOrderId, SupplierID: p.SupplierId, DateFrom: optFromDate(p.DateFrom),
		DateTo: optFromDate(p.DateTo), Search: str(p.Q),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	res, err := h.svc.ListAdvances(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]PurchaseAdvance, len(res.Items))
	for i, a := range res.Items {
		items[i] = toAdvance(ctx, a)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListPurchaseAdvances200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreatePurchaseAdvance menyimpan draft uang muka.
func (h *ProcurementHandler) CreatePurchaseAdvance(ctx context.Context, req CreatePurchaseAdvanceRequestObject) (CreatePurchaseAdvanceResponseObject, error) {
	b := req.Body
	in, err := advanceInput(b.PurchaseOrderId, PurchaseAdvanceFields{
		Amount: b.Amount, PaymentDate: b.PaymentDate, BankAccountCode: b.BankAccountCode, Note: b.Note,
	})
	if err != nil {
		return nil, err
	}
	a, err := h.svc.CreateAdvance(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePurchaseAdvance201JSONResponse(toAdvance(ctx, a)), nil
}

// GetPurchaseAdvance mengembalikan detail uang muka.
func (h *ProcurementHandler) GetPurchaseAdvance(ctx context.Context, req GetPurchaseAdvanceRequestObject) (GetPurchaseAdvanceResponseObject, error) {
	a, err := h.svc.GetAdvance(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPurchaseAdvance200JSONResponse(toAdvance(ctx, a)), nil
}

// UpdatePurchaseAdvance mengubah draft uang muka.
func (h *ProcurementHandler) UpdatePurchaseAdvance(ctx context.Context, req UpdatePurchaseAdvanceRequestObject) (UpdatePurchaseAdvanceResponseObject, error) {
	cur, err := h.svc.GetAdvance(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in, err := advanceInput(cur.PurchaseOrderID, PurchaseAdvanceFields{
		Amount: b.Amount, PaymentDate: b.PaymentDate, BankAccountCode: b.BankAccountCode, Note: b.Note,
	})
	if err != nil {
		return nil, err
	}
	a, err := h.svc.UpdateAdvance(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePurchaseAdvance200JSONResponse(toAdvance(ctx, a)), nil
}

// PostPurchaseAdvance memposting uang muka.
func (h *ProcurementHandler) PostPurchaseAdvance(ctx context.Context, req PostPurchaseAdvanceRequestObject) (PostPurchaseAdvanceResponseObject, error) {
	a, err := h.svc.PostAdvance(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostPurchaseAdvance200JSONResponse(toAdvance(ctx, a)), nil
}

// CancelPurchaseAdvance membatalkan uang muka.
func (h *ProcurementHandler) CancelPurchaseAdvance(ctx context.Context, req CancelPurchaseAdvanceRequestObject) (CancelPurchaseAdvanceResponseObject, error) {
	a, err := h.svc.CancelAdvance(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelPurchaseAdvance200JSONResponse(toAdvance(ctx, a)), nil
}
