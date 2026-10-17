package http

import (
	"context"
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"balans/internal/catalog/domain"
	"balans/internal/catalog/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/catalog.yaml ../../../api/openapi.bundle.yaml

// CatalogHandler mengimplementasikan StrictServerInterface hasil generate.
type CatalogHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*CatalogHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *CatalogHandler { return &CatalogHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *CatalogHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// --- Pemetaan ---------------------------------------------------------------------------------

func actions(list []string) []ProductAction {
	out := make([]ProductAction, len(list))
	for i, a := range list {
		out[i] = ProductAction(a)
	}
	return out
}

func toProduct(ctx context.Context, p domain.Product) Product {
	barcodes := make([]ProductBarcode, len(p.Barcodes))
	for i, b := range p.Barcodes {
		barcodes[i] = ProductBarcode{Barcode: b.Barcode, Symbology: BarcodeSymbology(b.Symbology), IsPrimary: b.IsPrimary}
	}
	components := make([]BundleComponent, len(p.Components))
	for i, c := range p.Components {
		components[i] = BundleComponent{
			ComponentProductId: c.ComponentID, Qty: c.Qty.StringFixed(3), ComponentSku: c.SKU, ComponentName: c.Name,
			ComponentVariant: c.Variant, ComponentUom: c.UOM, ComponentDeleted: c.Deleted,
		}
	}
	out := Product{
		Id: p.ID, Sku: p.SKU, Name: p.Name, Variant: p.Variant, ProductType: ProductType(p.Type), Uom: p.UOM,
		ShelfLifeDays: p.ShelfLifeDays, HasBatch: p.HasBatch, IsBundle: p.IsBundle, CategoryId: p.CategoryID,
		UnitsPerCarton: p.UnitsPerCarton, IsActive: p.IsActive, Barcodes: barcodes, BundleComponents: components,
		DeletedAt: p.DeletedAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version,
		AllowedActions: actions(service.RecordActions(ctx, p.Mark, service.PermProductWrite)),
	}
	if p.PackagingType != "" {
		t := PackagingType(p.PackagingType)
		out.PackagingType = &t
	}
	if p.CategoryName != "" {
		out.CategoryName = &p.CategoryName
	}
	if p.MinStockQty != nil {
		q := p.MinStockQty.StringFixed(3)
		out.MinStockQty = &q
	}
	if b, ok := p.PrimaryBarcode(); ok {
		out.PrimaryBarcode = &b.Barcode
	}
	return out
}

func toCategory(ctx context.Context, c domain.Category) ProductCategory {
	return ProductCategory{
		Id: c.ID, Code: c.Code, Name: c.Name, DeletedAt: c.DeletedAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Version: c.Version, AllowedActions: actions(service.RecordActions(ctx, c.Mark, service.PermProductWrite)),
	}
}

func parseQty(s *string, field string, fields map[string]string) *decimal.Decimal {
	if s == nil || *s == "" {
		return nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		fields[field] = "Qty tidak valid"
		return nil
	}
	return &d
}

// productFields mengubah isian bersama form produk menjadi input domain.
func productFields(f ProductFields) (domain.ProductInput, error) {
	fields := map[string]string{}
	in := domain.ProductInput{
		Name: f.Name, UOM: f.Uom, ShelfLifeDays: f.ShelfLifeDays, HasBatch: f.HasBatch, CategoryID: f.CategoryId,
		MinStockQty: parseQty(f.MinStockQty, "minStockQty", fields), UnitsPerCarton: f.UnitsPerCarton, IsActive: f.IsActive,
	}
	if f.Variant != nil {
		in.Variant = *f.Variant
	}
	if f.PackagingType != nil {
		in.PackagingType = domain.PackagingType(*f.PackagingType)
	}
	if f.Barcodes != nil {
		for _, b := range *f.Barcodes {
			in.Barcodes = append(in.Barcodes, domain.BarcodeInput{Barcode: b.Barcode, IsPrimary: b.IsPrimary})
		}
	}
	if f.BundleComponents != nil {
		for i, c := range *f.BundleComponents {
			q := parseQty(&c.Qty, fmt.Sprintf("bundleComponents[%d].qty", i), fields)
			ci := domain.ComponentInput{ComponentID: c.ComponentProductId}
			if q != nil {
				ci.Qty = *q
			}
			in.Components = append(in.Components, ci)
		}
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

// --- Produk -----------------------------------------------------------------------------------

// ListProducts mengembalikan daftar produk.
func (h *CatalogHandler) ListProducts(ctx context.Context, req ListProductsRequestObject) (ListProductsResponseObject, error) {
	p := req.Params
	f := service.ProductFilter{
		IsBundle: p.IsBundle, CategoryID: p.CategoryId, IsActive: p.IsActive, Deleted: p.Deleted != nil && *p.Deleted,
		Sort: paging.ParseSort(p.Sort, service.ProductSortFields, paging.Sort{Field: "sku"}),
	}
	if p.Q != nil {
		f.Search = *p.Q
	}
	if p.ProductType != nil {
		for _, t := range *p.ProductType {
			f.Types = append(f.Types, domain.ProductType(t))
		}
	}
	res, err := h.svc.ListProducts(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]Product, len(res.Items))
	for i, x := range res.Items {
		items[i] = toProduct(ctx, x)
	}
	return ListProducts200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateProduct membuat produk.
func (h *CatalogHandler) CreateProduct(ctx context.Context, req CreateProductRequestObject) (CreateProductResponseObject, error) {
	b := req.Body
	in, err := productFields(ProductFields{
		Name: b.Name, Variant: b.Variant, PackagingType: b.PackagingType, Uom: b.Uom, ShelfLifeDays: b.ShelfLifeDays,
		HasBatch: b.HasBatch, CategoryId: b.CategoryId, MinStockQty: b.MinStockQty, UnitsPerCarton: b.UnitsPerCarton,
		IsActive: b.IsActive, Barcodes: b.Barcodes, BundleComponents: b.BundleComponents,
	})
	if err != nil {
		return nil, err
	}
	in.SKU, in.Type, in.IsBundle = b.Sku, domain.ProductType(b.ProductType), b.IsBundle
	p, err := h.svc.CreateProduct(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateProduct201JSONResponse(toProduct(ctx, p)), nil
}

// LookupProduct mencari produk aktif berdasarkan SKU atau barcode.
func (h *CatalogHandler) LookupProduct(ctx context.Context, req LookupProductRequestObject) (LookupProductResponseObject, error) {
	p, err := h.svc.LookupProduct(ctx, req.Params.Code)
	if err != nil {
		return nil, err
	}
	return LookupProduct200JSONResponse(toProduct(ctx, p)), nil
}

// GetProduct mengembalikan detail produk.
func (h *CatalogHandler) GetProduct(ctx context.Context, req GetProductRequestObject) (GetProductResponseObject, error) {
	p, err := h.svc.GetProduct(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetProduct200JSONResponse(toProduct(ctx, p)), nil
}

// UpdateProduct mengubah produk.
func (h *CatalogHandler) UpdateProduct(ctx context.Context, req UpdateProductRequestObject) (UpdateProductResponseObject, error) {
	b := req.Body
	in, err := productFields(ProductFields{
		Name: b.Name, Variant: b.Variant, PackagingType: b.PackagingType, Uom: b.Uom, ShelfLifeDays: b.ShelfLifeDays,
		HasBatch: b.HasBatch, CategoryId: b.CategoryId, MinStockQty: b.MinStockQty, UnitsPerCarton: b.UnitsPerCarton,
		IsActive: b.IsActive, Barcodes: b.Barcodes, BundleComponents: b.BundleComponents,
	})
	if err != nil {
		return nil, err
	}
	cur, err := h.svc.GetProduct(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	// SKU, jenis, dan status bundel tidak bisa diubah lewat API; nilai lama dipertahankan.
	in.SKU, in.Type, in.IsBundle = cur.SKU, cur.Type, cur.IsBundle
	p, err := h.svc.UpdateProduct(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateProduct200JSONResponse(toProduct(ctx, p)), nil
}

// DeleteProduct menghapus produk (soft delete).
func (h *CatalogHandler) DeleteProduct(ctx context.Context, req DeleteProductRequestObject) (DeleteProductResponseObject, error) {
	if err := h.svc.DeleteProduct(ctx, req.Id); err != nil {
		return nil, err
	}
	return DeleteProduct204Response{}, nil
}

// RestoreProduct memulihkan produk.
func (h *CatalogHandler) RestoreProduct(ctx context.Context, req RestoreProductRequestObject) (RestoreProductResponseObject, error) {
	p, err := h.svc.RestoreProduct(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return RestoreProduct200JSONResponse(toProduct(ctx, p)), nil
}

// --- Kategori ---------------------------------------------------------------------------------

// ListProductCategories mengembalikan kategori.
func (h *CatalogHandler) ListProductCategories(ctx context.Context, req ListProductCategoriesRequestObject) (ListProductCategoriesResponseObject, error) {
	list, err := h.svc.ListCategories(ctx, req.Params.Deleted != nil && *req.Params.Deleted)
	if err != nil {
		return nil, err
	}
	out := make([]ProductCategory, len(list))
	for i, c := range list {
		out[i] = toCategory(ctx, c)
	}
	return ListProductCategories200JSONResponse(out), nil
}

// CreateProductCategory membuat kategori.
func (h *CatalogHandler) CreateProductCategory(ctx context.Context, req CreateProductCategoryRequestObject) (CreateProductCategoryResponseObject, error) {
	c, err := h.svc.CreateCategory(ctx, domain.CategoryInput{Code: req.Body.Code, Name: req.Body.Name})
	if err != nil {
		return nil, err
	}
	return CreateProductCategory201JSONResponse(toCategory(ctx, c)), nil
}

// UpdateProductCategory mengubah nama kategori.
func (h *CatalogHandler) UpdateProductCategory(ctx context.Context, req UpdateProductCategoryRequestObject) (UpdateProductCategoryResponseObject, error) {
	c, err := h.svc.UpdateCategory(ctx, req.Id, req.Body.Version, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return UpdateProductCategory200JSONResponse(toCategory(ctx, c)), nil
}

// DeleteProductCategory menghapus kategori.
func (h *CatalogHandler) DeleteProductCategory(ctx context.Context, req DeleteProductCategoryRequestObject) (DeleteProductCategoryResponseObject, error) {
	if err := h.svc.DeleteCategory(ctx, req.Id); err != nil {
		return nil, err
	}
	return DeleteProductCategory204Response{}, nil
}

// RestoreProductCategory memulihkan kategori.
func (h *CatalogHandler) RestoreProductCategory(ctx context.Context, req RestoreProductCategoryRequestObject) (RestoreProductCategoryResponseObject, error) {
	c, err := h.svc.RestoreCategory(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return RestoreProductCategory200JSONResponse(toCategory(ctx, c)), nil
}
