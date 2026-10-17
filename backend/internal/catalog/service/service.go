// Package service berisi use case master produk, packaging & material, bundel virtual, dan kategori
// (MD-01, MD-02, MD-03, S-106 soft delete).
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/catalog/domain"
	"balans/internal/catalog/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
)

// Permission modul catalog (Desain Role & Approval bagian 3.1). Kategori memakai permission produk.
const (
	PermProductRead  = "catalog.product.read"
	PermProductWrite = "catalog.product.write"
)

// Jenis entitas di audit trail.
const (
	EntityProduct  = "product"
	EntityCategory = "product_category"
)

// Aksi yang dikembalikan di allowedActions.
const (
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionRestore = "restore"
)

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	ListCategories(ctx context.Context, deleted bool) ([]domain.Category, error)
	GetCategory(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Category, error)
	InsertCategory(ctx context.Context, c domain.Category, actorID uuid.UUID) (domain.Category, error)
	UpdateCategory(ctx context.Context, c domain.Category, version int, actorID uuid.UUID) (domain.Category, error)
	SetCategoryDeleted(ctx context.Context, c domain.Category, actorID uuid.UUID) (domain.Category, error)

	ListProducts(ctx context.Context, f postgres.ProductFilter, page paging.Page) (paging.Result[domain.Product], error)
	GetProduct(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Product, error)
	ProductsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error)
	ActiveProductBySKU(ctx context.Context, sku string) (domain.Product, error)
	ActiveProductByBarcode(ctx context.Context, barcode string) (domain.Product, error)
	BarcodeOwners(ctx context.Context, barcodes []string) (map[string]uuid.UUID, error)
	ActiveBundlesUsing(ctx context.Context, componentID uuid.UUID) ([]string, error)
	InsertProduct(ctx context.Context, p domain.Product, actorID uuid.UUID) (domain.Product, error)
	UpdateProduct(ctx context.Context, p domain.Product, version int, actorID uuid.UUID) (domain.Product, error)
	SetProductDeleted(ctx context.Context, p domain.Product, actorID uuid.UUID) (domain.Product, error)
}

// Deps adalah dependensi Service.
type Deps struct {
	Store store
	Tx    txRunner
	Audit audit.Recorder
	Clock clock.Clock
}

// Service adalah use case modul catalog.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

// RecordActions mengembalikan aksi yang boleh dilakukan actor atas data master (allowedActions).
func RecordActions(ctx context.Context, m softdelete.Mark, writePermission string) []string {
	a, ok := actor.From(ctx)
	if !ok {
		return []string{}
	}
	if m.Deleted() {
		if softdelete.CanRestore(a, writePermission) {
			return []string{ActionRestore}
		}
		return []string{}
	}
	if a.Has(writePermission) {
		return []string{ActionUpdate, ActionDelete}
	}
	return []string{}
}

func deletedSnapshot(m softdelete.Mark) map[string]any {
	return map[string]any{"deletedAt": m.DeletedAt}
}

// --- Kategori ---------------------------------------------------------------------------------

func categorySnapshot(c domain.Category) map[string]any {
	return map[string]any{"code": c.Code, "name": c.Name}
}

// ListCategories mengembalikan kategori; deleted = hanya kategori terhapus.
func (s *Service) ListCategories(ctx context.Context, deleted bool) ([]domain.Category, error) {
	if _, err := actor.Require(ctx, PermProductRead); err != nil {
		return nil, err
	}
	return s.Store.ListCategories(ctx, deleted)
}

// CreateCategory membuat kategori.
func (s *Service) CreateCategory(ctx context.Context, in domain.CategoryInput) (domain.Category, error) {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return domain.Category{}, err
	}
	c, err := domain.NewCategory(in)
	if err != nil {
		return domain.Category{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if c, err = s.Store.InsertCategory(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityCategory, EntityID: c.ID, EntityLabel: c.Code, After: categorySnapshot(c),
		})
	})
	return c, err
}

// UpdateCategory mengubah nama kategori.
func (s *Service) UpdateCategory(ctx context.Context, id uuid.UUID, version int, name string) (domain.Category, error) {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return domain.Category{}, err
	}
	var c domain.Category
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if c, err = s.Store.GetCategory(ctx, id, true); err != nil {
			return err
		}
		if c.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := c
		if err := c.Rename(name); err != nil {
			return err
		}
		if c, err = s.Store.UpdateCategory(ctx, c, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityCategory, EntityID: c.ID, EntityLabel: c.Code,
			Before: categorySnapshot(before), After: categorySnapshot(c),
		})
	})
	return c, err
}

// DeleteCategory menghapus kategori (soft delete). Produk lama tetap menampilkan nama kategorinya.
func (s *Service) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetCategory(ctx, id, true)
		if err != nil {
			return err
		}
		before := c.Mark
		if err := c.Delete(s.Clock.Now(), a.UserID); err != nil {
			return err
		}
		if c, err = s.Store.SetCategoryDeleted(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityCategory, EntityID: c.ID, EntityLabel: c.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(c.Mark),
		})
	})
}

// RestoreCategory memulihkan kategori (hanya Admin Sistem).
func (s *Service) RestoreCategory(ctx context.Context, id uuid.UUID) (domain.Category, error) {
	a, err := softdelete.RequireRestore(ctx, PermProductWrite)
	if err != nil {
		return domain.Category{}, err
	}
	var c domain.Category
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if c, err = s.Store.GetCategory(ctx, id, true); err != nil {
			return err
		}
		before := c.Mark
		if err := c.Restore(); err != nil {
			return err
		}
		if c, err = s.Store.SetCategoryDeleted(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionRestore, EntityType: EntityCategory, EntityID: c.ID, EntityLabel: c.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(c.Mark),
		})
	})
	return c, err
}

// --- Produk -----------------------------------------------------------------------------------

// productSnapshot adalah bentuk produk untuk audit trail (nama field mengikuti JSON API).
func productSnapshot(p domain.Product) map[string]any {
	barcodes := make([]string, len(p.Barcodes))
	for i, b := range p.Barcodes {
		barcodes[i] = b.Barcode
		if b.IsPrimary {
			barcodes[i] += " (utama)"
		}
	}
	components := make([]string, len(p.Components))
	for i, c := range p.Components {
		label := c.SKU
		if label == "" {
			label = c.ComponentID.String()
		}
		components[i] = fmt.Sprintf("%s × %s", label, c.Qty.String())
	}
	m := map[string]any{
		"sku": p.SKU, "name": p.Name, "variant": p.Variant, "productType": p.Type, "packagingType": p.PackagingType,
		"uom": p.UOM, "shelfLifeDays": p.ShelfLifeDays, "hasBatch": p.HasBatch, "isBundle": p.IsBundle,
		"categoryId": p.CategoryID, "unitsPerCarton": p.UnitsPerCarton, "isActive": p.IsActive, "barcodes": barcodes,
		"minStockQty": nil,
	}
	if p.MinStockQty != nil {
		m["minStockQty"] = p.MinStockQty.String()
	}
	if p.IsBundle {
		m["bundleComponents"] = components
	}
	return m
}

// ProductFilter adalah kriteria daftar produk.
type ProductFilter = postgres.ProductFilter

// ProductSortFields adalah field sort yang diizinkan di daftar produk.
var ProductSortFields = []string{"sku", "name", "createdAt"}

// ListProducts mengembalikan daftar produk.
func (s *Service) ListProducts(ctx context.Context, f ProductFilter, page paging.Page) (paging.Result[domain.Product], error) {
	if _, err := actor.Require(ctx, PermProductRead); err != nil {
		return paging.Result[domain.Product]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListProducts(ctx, f, page)
}

// GetProduct mengembalikan detail produk, termasuk produk terhapus (untuk dokumen lama).
func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	if _, err := actor.Require(ctx, PermProductRead); err != nil {
		return domain.Product{}, err
	}
	return s.Store.GetProduct(ctx, id, false)
}

// LookupProduct mencari produk aktif berdasarkan SKU, lalu barcode persis (hasil pindai).
func (s *Service) LookupProduct(ctx context.Context, code string) (domain.Product, error) {
	if _, err := actor.Require(ctx, PermProductRead); err != nil {
		return domain.Product{}, err
	}
	return s.FindByCode(ctx, code)
}

// FindByCode mencari produk aktif berdasarkan SKU atau barcode tanpa cek permission (dipakai modul lain).
func (s *Service) FindByCode(ctx context.Context, code string) (domain.Product, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return domain.Product{}, apperr.New(apperr.CodeNotFound)
	}
	p, err := s.Store.ActiveProductBySKU(ctx, domain.NormalizeCode(code))
	if !apperr.HasCode(err, apperr.CodeNotFound) {
		return p, err
	}
	return s.Store.ActiveProductByBarcode(ctx, code)
}

// validateReferences memeriksa kategori, keunikan barcode, dan komponen bundel di transaksi aktif.
// Nama, SKU, dan satuan komponen diisi untuk audit trail.
func (s *Service) validateReferences(ctx context.Context, p *domain.Product) error {
	fields := map[string]string{}
	if p.CategoryID != nil {
		c, err := s.Store.GetCategory(ctx, *p.CategoryID, false)
		switch {
		case apperr.HasCode(err, apperr.CodeNotFound) || (err == nil && c.Deleted()):
			fields["categoryId"] = "Kategori tidak ditemukan atau sudah dihapus"
		case err != nil:
			return err
		}
	}
	codes := make([]string, len(p.Barcodes))
	for i, b := range p.Barcodes {
		codes[i] = b.Barcode
	}
	owners, err := s.Store.BarcodeOwners(ctx, codes)
	if err != nil {
		return err
	}
	for i, b := range p.Barcodes {
		if owner, ok := owners[b.Barcode]; ok && owner != p.ID {
			fields[fmt.Sprintf("barcodes[%d].barcode", i)] = "Barcode sudah dipakai produk lain"
		}
	}
	if len(p.Components) > 0 {
		ids := make([]uuid.UUID, len(p.Components))
		for i, c := range p.Components {
			ids[i] = c.ComponentID
		}
		list, err := s.Store.ProductsByIDs(ctx, ids)
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]domain.Product{}
		for _, c := range list {
			byID[c.ID] = c
		}
		for i := range p.Components {
			c, ok := byID[p.Components[i].ComponentID]
			switch {
			case !ok || c.Deleted():
				fields[fmt.Sprintf("bundleComponents[%d].componentProductId", i)] = "Produk komponen tidak ditemukan atau sudah dihapus"
			case !c.CanBeComponent():
				fields[fmt.Sprintf("bundleComponents[%d].componentProductId", i)] = "Komponen harus Produk Jadi aktif yang bukan bundel"
			default:
				p.Components[i].SKU, p.Components[i].Name, p.Components[i].Variant, p.Components[i].UOM = c.SKU, c.Name, c.Variant, c.UOM
			}
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CreateProduct membuat produk.
func (s *Service) CreateProduct(ctx context.Context, in domain.ProductInput) (domain.Product, error) {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return domain.Product{}, err
	}
	p, err := domain.NewProduct(in)
	if err != nil {
		return domain.Product{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.validateReferences(ctx, &p); err != nil {
			return err
		}
		snapshot := productSnapshot(p)
		if p, err = s.Store.InsertProduct(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityProduct, EntityID: p.ID, EntityLabel: p.SKU, After: snapshot,
		})
	})
	return p, err
}

// UpdateProduct mengubah produk dengan optimistic locking. SKU, jenis, dan status bundel tetap.
func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, version int, in domain.ProductInput) (domain.Product, error) {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return domain.Product{}, err
	}
	var p domain.Product
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetProduct(ctx, id, true); err != nil {
			return err
		}
		if p.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := productSnapshot(p)
		if err := p.Update(in); err != nil {
			return err
		}
		if err := s.validateReferences(ctx, &p); err != nil {
			return err
		}
		after := productSnapshot(p)
		if p, err = s.Store.UpdateProduct(ctx, p, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityProduct, EntityID: p.ID, EntityLabel: p.SKU, Before: before, After: after,
		})
	})
	return p, err
}

// DeleteProduct menghapus produk (soft delete). Produk yang menjadi komponen bundel aktif ditolak.
func (s *Service) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermProductWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetProduct(ctx, id, true)
		if err != nil {
			return err
		}
		if !p.Deleted() {
			bundles, err := s.Store.ActiveBundlesUsing(ctx, p.ID)
			if err != nil {
				return err
			}
			if len(bundles) > 0 {
				return apperr.WithMessage(apperr.CodeInvalidStateTransition,
					"Produk masih menjadi komponen bundel "+strings.Join(bundles, ", ")+". Hapus atau ubah bundel tersebut dulu.")
			}
		}
		before := p.Mark
		if err := p.Delete(s.Clock.Now(), a.UserID); err != nil {
			return err
		}
		if p, err = s.Store.SetProductDeleted(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityProduct, EntityID: p.ID, EntityLabel: p.SKU,
			Before: deletedSnapshot(before), After: deletedSnapshot(p.Mark),
		})
	})
}

// RestoreProduct memulihkan produk (hanya Admin Sistem). Bundel hanya bisa dipulihkan jika komponennya masih ada.
func (s *Service) RestoreProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	a, err := softdelete.RequireRestore(ctx, PermProductWrite)
	if err != nil {
		return domain.Product{}, err
	}
	var p domain.Product
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetProduct(ctx, id, true); err != nil {
			return err
		}
		for _, c := range p.Components {
			if c.Deleted {
				return apperr.WithMessage(apperr.CodeInvalidStateTransition,
					"Komponen "+c.SKU+" sudah dihapus. Pulihkan komponen itu dulu.")
			}
		}
		before := p.Mark
		if err := p.Restore(); err != nil {
			return err
		}
		if p, err = s.Store.SetProductDeleted(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionRestore, EntityType: EntityProduct, EntityID: p.ID, EntityLabel: p.SKU,
			Before: deletedSnapshot(before), After: deletedSnapshot(p.Mark),
		})
	})
	return p, err
}

// --- Lookup untuk modul lain (tanpa cek permission; modul pemanggil memeriksa permission-nya sendiri) ---

// ProductByID membaca produk termasuk yang terhapus.
func (s *Service) ProductByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return s.Store.GetProduct(ctx, id, false)
}

// ProductsByIDs membaca banyak produk termasuk yang terhapus, dipetakan per ID.
func (s *Service) ProductsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Product, error) {
	list, err := s.Store.ProductsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.Product, len(list))
	for _, p := range list {
		out[p.ID] = p
	}
	return out, nil
}

// OptionFilter adalah kriteria dropdown produk untuk transaksi baru.
type OptionFilter struct {
	Search   string
	Types    []domain.ProductType
	IsBundle *bool
	// Limit default 20, maksimum 200.
	Limit int
}

// ProductOptions mengembalikan produk aktif yang belum dihapus untuk dropdown transaksi baru.
func (s *Service) ProductOptions(ctx context.Context, f OptionFilter) ([]domain.Product, error) {
	size := f.Limit
	if size <= 0 {
		size = 20
	}
	active := true
	res, err := s.Store.ListProducts(ctx, ProductFilter{
		Search: strings.TrimSpace(f.Search), Types: f.Types, IsBundle: f.IsBundle, IsActive: &active, Sort: paging.Sort{Field: "sku"},
	}, paging.New(nil, &size))
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}
