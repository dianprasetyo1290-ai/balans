package http

import (
	"context"

	"github.com/go-chi/chi/v5"

	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/service"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/warehouse.yaml ../../../api/openapi.bundle.yaml

// WarehouseHandler mengimplementasikan StrictServerInterface hasil generate.
type WarehouseHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*WarehouseHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *WarehouseHandler { return &WarehouseHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *WarehouseHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func actions(list []string) []RecordAction {
	out := make([]RecordAction, len(list))
	for i, a := range list {
		out[i] = RecordAction(a)
	}
	return out
}

func toWarehouse(ctx context.Context, w domain.Warehouse) Warehouse {
	return Warehouse{
		Id: w.ID, Code: w.Code, Name: w.Name, Address: w.Address, IsActive: w.IsActive, DeletedAt: w.DeletedAt,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, Version: w.Version,
		AllowedActions: actions(service.RecordActions(ctx, w.Mark, service.PermWarehouseWrite)),
	}
}

func toLocation(ctx context.Context, l domain.Location) Location {
	out := Location{
		Id: l.ID, Code: l.Code, Name: l.Name, LocationType: LocationType(l.Type), LocationTypeLabel: l.Type.Label(),
		WarehouseId: l.WarehouseID, ExternalPartyId: l.PartyID, IsExternal: l.IsExternal(), IsSellable: l.IsSellable,
		IsStandard: l.IsStandard, IsActive: l.IsActive, DeletedAt: l.DeletedAt, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
		Version: l.Version, AllowedActions: actions(service.LocationActions(ctx, l)),
	}
	if l.WarehouseName != "" {
		out.WarehouseName = &l.WarehouseName
	}
	if l.PartyType != "" {
		t := ExternalPartyType(l.PartyType)
		out.ExternalPartyType = &t
	}
	return out
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- Gudang -----------------------------------------------------------------------------------

// ListWarehouses mengembalikan daftar gudang.
func (h *WarehouseHandler) ListWarehouses(ctx context.Context, req ListWarehousesRequestObject) (ListWarehousesResponseObject, error) {
	p := req.Params
	res, err := h.svc.ListWarehouses(ctx, service.WarehouseFilter{
		Search: str(p.Q), IsActive: p.IsActive, Deleted: p.Deleted != nil && *p.Deleted,
		Sort: paging.ParseSort(p.Sort, service.SortFields, paging.Sort{Field: "code"}),
	}, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]Warehouse, len(res.Items))
	for i, w := range res.Items {
		items[i] = toWarehouse(ctx, w)
	}
	return ListWarehouses200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateWarehouse membuat gudang beserta lokasi standarnya.
func (h *WarehouseHandler) CreateWarehouse(ctx context.Context, req CreateWarehouseRequestObject) (CreateWarehouseResponseObject, error) {
	b := req.Body
	w, _, err := h.svc.CreateWarehouse(ctx, domain.WarehouseInput{Code: b.Code, Name: b.Name, Address: str(b.Address), IsActive: b.IsActive})
	if err != nil {
		return nil, err
	}
	return CreateWarehouse201JSONResponse(toWarehouse(ctx, w)), nil
}

// GetWarehouse mengembalikan detail gudang.
func (h *WarehouseHandler) GetWarehouse(ctx context.Context, req GetWarehouseRequestObject) (GetWarehouseResponseObject, error) {
	w, err := h.svc.GetWarehouse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetWarehouse200JSONResponse(toWarehouse(ctx, w)), nil
}

// UpdateWarehouse mengubah gudang.
func (h *WarehouseHandler) UpdateWarehouse(ctx context.Context, req UpdateWarehouseRequestObject) (UpdateWarehouseResponseObject, error) {
	b := req.Body
	w, err := h.svc.UpdateWarehouse(ctx, req.Id, b.Version, domain.WarehouseInput{Name: b.Name, Address: str(b.Address), IsActive: b.IsActive})
	if err != nil {
		return nil, err
	}
	return UpdateWarehouse200JSONResponse(toWarehouse(ctx, w)), nil
}

// DeleteWarehouse menghapus gudang.
func (h *WarehouseHandler) DeleteWarehouse(ctx context.Context, req DeleteWarehouseRequestObject) (DeleteWarehouseResponseObject, error) {
	if err := h.svc.DeleteWarehouse(ctx, req.Id); err != nil {
		return nil, err
	}
	return DeleteWarehouse204Response{}, nil
}

// RestoreWarehouse memulihkan gudang.
func (h *WarehouseHandler) RestoreWarehouse(ctx context.Context, req RestoreWarehouseRequestObject) (RestoreWarehouseResponseObject, error) {
	w, err := h.svc.RestoreWarehouse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return RestoreWarehouse200JSONResponse(toWarehouse(ctx, w)), nil
}

// --- Lokasi -----------------------------------------------------------------------------------

// ListLocations mengembalikan daftar lokasi.
func (h *WarehouseHandler) ListLocations(ctx context.Context, req ListLocationsRequestObject) (ListLocationsResponseObject, error) {
	p := req.Params
	f := service.LocationFilter{
		Search: str(p.Q), WarehouseID: p.WarehouseId, External: p.External, PartyID: p.ExternalPartyId, IsActive: p.IsActive,
		Deleted: p.Deleted != nil && *p.Deleted, Sort: paging.ParseSort(p.Sort, service.SortFields, paging.Sort{}),
	}
	if p.ExternalPartyType != nil {
		f.PartyType = domain.PartyType(*p.ExternalPartyType)
	}
	if p.LocationType != nil {
		for _, t := range *p.LocationType {
			f.Types = append(f.Types, domain.LocationType(t))
		}
	}
	res, err := h.svc.ListLocations(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]Location, len(res.Items))
	for i, l := range res.Items {
		items[i] = toLocation(ctx, l)
	}
	return ListLocations200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateLocation membuat lokasi.
func (h *WarehouseHandler) CreateLocation(ctx context.Context, req CreateLocationRequestObject) (CreateLocationResponseObject, error) {
	b := req.Body
	in := domain.LocationInput{
		Code: b.Code, Name: b.Name, Type: domain.LocationType(b.LocationType), WarehouseID: b.WarehouseId,
		PartyID: b.ExternalPartyId, IsActive: b.IsActive,
	}
	if b.ExternalPartyType != nil {
		in.PartyType = domain.PartyType(*b.ExternalPartyType)
	}
	l, err := h.svc.CreateLocation(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateLocation201JSONResponse(toLocation(ctx, l)), nil
}

// GetLocation mengembalikan detail lokasi.
func (h *WarehouseHandler) GetLocation(ctx context.Context, req GetLocationRequestObject) (GetLocationResponseObject, error) {
	l, err := h.svc.GetLocation(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetLocation200JSONResponse(toLocation(ctx, l)), nil
}

// UpdateLocation mengubah lokasi.
func (h *WarehouseHandler) UpdateLocation(ctx context.Context, req UpdateLocationRequestObject) (UpdateLocationResponseObject, error) {
	l, err := h.svc.UpdateLocation(ctx, req.Id, req.Body.Version, req.Body.Name, req.Body.IsActive)
	if err != nil {
		return nil, err
	}
	return UpdateLocation200JSONResponse(toLocation(ctx, l)), nil
}

// DeleteLocation menghapus lokasi.
func (h *WarehouseHandler) DeleteLocation(ctx context.Context, req DeleteLocationRequestObject) (DeleteLocationResponseObject, error) {
	if err := h.svc.DeleteLocation(ctx, req.Id); err != nil {
		return nil, err
	}
	return DeleteLocation204Response{}, nil
}

// RestoreLocation memulihkan lokasi.
func (h *WarehouseHandler) RestoreLocation(ctx context.Context, req RestoreLocationRequestObject) (RestoreLocationResponseObject, error) {
	l, err := h.svc.RestoreLocation(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return RestoreLocation200JSONResponse(toLocation(ctx, l)), nil
}
