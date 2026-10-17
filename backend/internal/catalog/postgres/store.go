// Package postgres adalah repository modul catalog.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/catalog/domain"
	"balans/internal/catalog/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
)

// Store menyimpan produk, barcode, komponen bundel, dan kategori.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullInt(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true} //nolint:gosec // umur simpan & isi karton kecil
}

func intPtr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func nullDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}

func decimalPtr(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	v := d.Decimal
	return &v
}

func mark(at *time.Time, by uuid.NullUUID) softdelete.Mark {
	return softdelete.Mark{DeletedAt: at, DeletedBy: uuidPtr(by)}
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// versionConflict: UPDATE ... WHERE version = $n tanpa baris berarti data sudah diubah pengguna lain.
func versionConflict(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeVersionConflict, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// --- Kategori ---------------------------------------------------------------------------------

func toCategory(r sqlcgen.ProductCategory) domain.Category {
	return domain.Category{
		ID: r.ID, Code: r.Code, Name: r.Name, Mark: mark(r.DeletedAt, r.DeletedBy), CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListCategories mengembalikan kategori aktif, atau hanya yang terhapus jika deleted.
func (s *Store) ListCategories(ctx context.Context, deleted bool) ([]domain.Category, error) {
	rows, err := s.q(ctx).ListCategories(ctx, deleted)
	if err != nil {
		return nil, fmt.Errorf("list product categories: %w", err)
	}
	out := make([]domain.Category, len(rows))
	for i, r := range rows {
		out[i] = toCategory(r)
	}
	return out, nil
}

// GetCategory membaca kategori termasuk yang terhapus; forUpdate mengunci baris.
func (s *Store) GetCategory(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Category, error) {
	var (
		r   sqlcgen.ProductCategory
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetCategoryForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetCategory(ctx, id)
	}
	if err != nil {
		return domain.Category{}, notFound(err, "get product category")
	}
	return toCategory(r), nil
}

// InsertCategory menyimpan kategori baru. Kode yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) InsertCategory(ctx context.Context, c domain.Category, actorID uuid.UUID) (domain.Category, error) {
	r, err := s.q(ctx).InsertCategory(ctx, sqlcgen.InsertCategoryParams{Code: c.Code, Name: c.Name, ActorID: actorID})
	if isUniqueViolation(err, "uq_product_categories_code") {
		return domain.Category{}, &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "Kode kategori sudah dipakai.",
			Fields: map[string]string{"code": "Kode sudah dipakai, termasuk oleh kategori yang sudah dihapus"}}
	}
	if err != nil {
		return domain.Category{}, fmt.Errorf("insert product category: %w", err)
	}
	return toCategory(r), nil
}

// UpdateCategory menyimpan nama kategori dengan optimistic locking.
func (s *Store) UpdateCategory(ctx context.Context, c domain.Category, version int, actorID uuid.UUID) (domain.Category, error) {
	r, err := s.q(ctx).UpdateCategory(ctx, sqlcgen.UpdateCategoryParams{
		ID: c.ID, Name: c.Name, Version: int32(version), ActorID: actorID, //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Category{}, versionConflict(err, "update product category")
	}
	return toCategory(r), nil
}

// SetCategoryDeleted menyimpan penanda hapus kategori.
func (s *Store) SetCategoryDeleted(ctx context.Context, c domain.Category, actorID uuid.UUID) (domain.Category, error) {
	r, err := s.q(ctx).SetCategoryDeleted(ctx, sqlcgen.SetCategoryDeletedParams{
		ID: c.ID, DeletedAt: c.DeletedAt, DeletedBy: nullUUID(c.DeletedBy), ActorID: actorID,
	})
	if err != nil {
		return domain.Category{}, notFound(err, "set product category deleted")
	}
	return toCategory(r), nil
}

// --- Produk -----------------------------------------------------------------------------------

func toProduct(r sqlcgen.Product) domain.Product {
	return domain.Product{
		ID: r.ID, SKU: r.Sku, Name: r.Name, Variant: r.Variant, Type: domain.ProductType(r.ProductType),
		PackagingType: domain.PackagingType(r.PackagingType.String), UOM: r.Uom, ShelfLifeDays: intPtr(r.ShelfLifeDays),
		HasBatch: r.HasBatch, IsBundle: r.IsBundle, CategoryID: uuidPtr(r.CategoryID), MinStockQty: decimalPtr(r.MinStockQty),
		UnitsPerCarton: intPtr(r.UnitsPerCarton), IsActive: r.IsActive, Mark: mark(r.DeletedAt, r.DeletedBy),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// hydrate memuat barcode, komponen bundel, dan nama kategori untuk sekumpulan produk dalam tiga query.
func (s *Store) hydrate(ctx context.Context, rows []sqlcgen.Product) ([]domain.Product, error) {
	out := make([]domain.Product, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	var bundleIDs, categoryIDs []uuid.UUID
	for i, r := range rows {
		out[i] = toProduct(r)
		ids[i] = r.ID
		if r.IsBundle {
			bundleIDs = append(bundleIDs, r.ID)
		}
		if r.CategoryID.Valid {
			categoryIDs = append(categoryIDs, r.CategoryID.UUID)
		}
	}
	barcodes, err := s.q(ctx).ListBarcodesByProducts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list product barcodes: %w", err)
	}
	byProduct := map[uuid.UUID][]domain.Barcode{}
	for _, b := range barcodes {
		byProduct[b.ProductID] = append(byProduct[b.ProductID], domain.Barcode{
			Barcode: b.Barcode, Symbology: domain.Symbology(b.Symbology), IsPrimary: b.IsPrimary,
		})
	}
	components := map[uuid.UUID][]domain.BundleComponent{}
	if len(bundleIDs) > 0 {
		rows, err := s.q(ctx).ListComponentsByBundles(ctx, bundleIDs)
		if err != nil {
			return nil, fmt.Errorf("list bundle components: %w", err)
		}
		for _, c := range rows {
			components[c.BundleProductID] = append(components[c.BundleProductID], domain.BundleComponent{
				ComponentID: c.ComponentProductID, Qty: c.Qty, SKU: c.Sku, Name: c.Name, Variant: c.Variant, UOM: c.Uom,
				Deleted: c.Deleted,
			})
		}
	}
	categories := map[uuid.UUID]string{}
	if len(categoryIDs) > 0 {
		cats, err := s.q(ctx).ListCategoriesByIDs(ctx, categoryIDs)
		if err != nil {
			return nil, fmt.Errorf("list product categories by id: %w", err)
		}
		for _, c := range cats {
			categories[c.ID] = c.Name
		}
	}
	for i := range out {
		out[i].Barcodes = byProduct[out[i].ID]
		out[i].Components = components[out[i].ID]
		if out[i].CategoryID != nil {
			out[i].CategoryName = categories[*out[i].CategoryID]
		}
	}
	return out, nil
}

func (s *Store) one(ctx context.Context, r sqlcgen.Product) (domain.Product, error) {
	list, err := s.hydrate(ctx, []sqlcgen.Product{r})
	if err != nil {
		return domain.Product{}, err
	}
	return list[0], nil
}

// ProductFilter adalah kriteria daftar produk.
type ProductFilter struct {
	Search     string
	Types      []domain.ProductType
	IsBundle   *bool
	CategoryID *uuid.UUID
	IsActive   *bool
	// Deleted true = hanya produk terhapus; false = hanya produk yang belum dihapus.
	Deleted bool
	Sort    paging.Sort
}

func optBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// ListProducts mengembalikan satu halaman produk.
func (s *Store) ListProducts(ctx context.Context, f ProductFilter, page paging.Page) (paging.Result[domain.Product], error) {
	types := make([]string, len(f.Types))
	for i, t := range f.Types {
		types[i] = string(t)
	}
	search := nullText(f.Search)
	rows, err := s.q(ctx).ListProducts(ctx, sqlcgen.ListProductsParams{
		Deleted: f.Deleted, Search: search, ProductTypes: types, IsBundle: optBool(f.IsBundle),
		CategoryID: nullUUID(f.CategoryID), IsActive: optBool(f.IsActive), Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Product]{}, fmt.Errorf("list products: %w", err)
	}
	total, err := s.q(ctx).CountProducts(ctx, sqlcgen.CountProductsParams{
		Deleted: f.Deleted, Search: search, ProductTypes: types, IsBundle: optBool(f.IsBundle),
		CategoryID: nullUUID(f.CategoryID), IsActive: optBool(f.IsActive),
	})
	if err != nil {
		return paging.Result[domain.Product]{}, fmt.Errorf("count products: %w", err)
	}
	items, err := s.hydrate(ctx, rows)
	if err != nil {
		return paging.Result[domain.Product]{}, err
	}
	return paging.Result[domain.Product]{Items: items, Page: page, Total: total}, nil
}

// GetProduct membaca produk termasuk yang terhapus; forUpdate mengunci baris produk.
func (s *Store) GetProduct(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Product, error) {
	var (
		r   sqlcgen.Product
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetProductForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetProduct(ctx, id)
	}
	if err != nil {
		return domain.Product{}, notFound(err, "get product")
	}
	return s.one(ctx, r)
}

// ProductsByIDs membaca produk termasuk yang terhapus. ID yang tidak ada diabaikan.
func (s *Store) ProductsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListProductsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list products by id: %w", err)
	}
	return s.hydrate(ctx, rows)
}

// ActiveProductBySKU membaca produk yang belum dihapus berdasarkan SKU.
func (s *Store) ActiveProductBySKU(ctx context.Context, sku string) (domain.Product, error) {
	r, err := s.q(ctx).GetActiveProductBySKU(ctx, sku)
	if err != nil {
		return domain.Product{}, notFound(err, "get product by sku")
	}
	return s.one(ctx, r)
}

// ActiveProductByBarcode membaca produk yang belum dihapus berdasarkan barcode persis.
func (s *Store) ActiveProductByBarcode(ctx context.Context, barcode string) (domain.Product, error) {
	r, err := s.q(ctx).GetActiveProductByBarcode(ctx, barcode)
	if err != nil {
		return domain.Product{}, notFound(err, "get product by barcode")
	}
	return s.one(ctx, r)
}

// BarcodeOwners mengembalikan pemilik barcode yang sudah terdaftar.
func (s *Store) BarcodeOwners(ctx context.Context, barcodes []string) (map[string]uuid.UUID, error) {
	out := map[string]uuid.UUID{}
	if len(barcodes) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).ListBarcodeOwners(ctx, barcodes)
	if err != nil {
		return nil, fmt.Errorf("list barcode owners: %w", err)
	}
	for _, r := range rows {
		out[r.Barcode] = r.ProductID
	}
	return out, nil
}

// ActiveBundlesUsing mengembalikan SKU bundel aktif yang memakai produk sebagai komponen.
func (s *Store) ActiveBundlesUsing(ctx context.Context, componentID uuid.UUID) ([]string, error) {
	skus, err := s.q(ctx).ListActiveBundlesUsing(ctx, componentID)
	if err != nil {
		return nil, fmt.Errorf("list bundles using component: %w", err)
	}
	return skus, nil
}

func duplicateSKU() error {
	return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "SKU sudah dipakai.",
		Fields: map[string]string{"sku": "SKU sudah dipakai, termasuk oleh produk yang sudah dihapus"}}
}

// InsertProduct menyimpan produk baru beserta barcode dan komponennya. SKU yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) InsertProduct(ctx context.Context, p domain.Product, actorID uuid.UUID) (domain.Product, error) {
	r, err := s.q(ctx).InsertProduct(ctx, sqlcgen.InsertProductParams{
		Sku: p.SKU, Name: p.Name, Variant: p.Variant, ProductType: string(p.Type), PackagingType: nullText(string(p.PackagingType)),
		Uom: p.UOM, ShelfLifeDays: nullInt(p.ShelfLifeDays), HasBatch: p.HasBatch, IsBundle: p.IsBundle,
		CategoryID: nullUUID(p.CategoryID), MinStockQty: nullDecimal(p.MinStockQty), UnitsPerCarton: nullInt(p.UnitsPerCarton),
		IsActive: p.IsActive, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_products_sku") {
		return domain.Product{}, duplicateSKU()
	}
	if err != nil {
		return domain.Product{}, fmt.Errorf("insert product: %w", err)
	}
	if err := s.replaceChildren(ctx, r.ID, p, actorID); err != nil {
		return domain.Product{}, err
	}
	return s.one(ctx, r)
}

// UpdateProduct menyimpan perubahan produk dengan optimistic locking dan mengganti barcode serta komponennya.
func (s *Store) UpdateProduct(ctx context.Context, p domain.Product, version int, actorID uuid.UUID) (domain.Product, error) {
	r, err := s.q(ctx).UpdateProduct(ctx, sqlcgen.UpdateProductParams{
		ID: p.ID, Name: p.Name, Variant: p.Variant, PackagingType: nullText(string(p.PackagingType)), Uom: p.UOM,
		ShelfLifeDays: nullInt(p.ShelfLifeDays), HasBatch: p.HasBatch, CategoryID: nullUUID(p.CategoryID),
		MinStockQty: nullDecimal(p.MinStockQty), UnitsPerCarton: nullInt(p.UnitsPerCarton), IsActive: p.IsActive,
		ActorID: actorID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Product{}, versionConflict(err, "update product")
	}
	if err := s.replaceChildren(ctx, p.ID, p, actorID); err != nil {
		return domain.Product{}, err
	}
	return s.one(ctx, r)
}

func (s *Store) replaceChildren(ctx context.Context, id uuid.UUID, p domain.Product, actorID uuid.UUID) error {
	q := s.q(ctx)
	if err := q.DeleteBarcodesByProduct(ctx, id); err != nil {
		return fmt.Errorf("delete product barcodes: %w", err)
	}
	for i, b := range p.Barcodes {
		err := q.InsertBarcode(ctx, sqlcgen.InsertBarcodeParams{
			ProductID: id, Barcode: b.Barcode, Symbology: string(b.Symbology), IsPrimary: b.IsPrimary, ActorID: actorID,
		})
		if isUniqueViolation(err, "uq_product_barcodes_barcode") {
			return apperr.Validation(map[string]string{fmt.Sprintf("barcodes[%d].barcode", i): "Barcode sudah dipakai produk lain"})
		}
		if err != nil {
			return fmt.Errorf("insert product barcode: %w", err)
		}
	}
	if err := q.DeleteComponentsByBundle(ctx, id); err != nil {
		return fmt.Errorf("delete bundle components: %w", err)
	}
	for _, c := range p.Components {
		if err := q.InsertComponent(ctx, sqlcgen.InsertComponentParams{
			BundleProductID: id, ComponentProductID: c.ComponentID, Qty: c.Qty, ActorID: actorID,
		}); err != nil {
			return fmt.Errorf("insert bundle component: %w", err)
		}
	}
	return nil
}

// SetProductDeleted menyimpan penanda hapus produk.
func (s *Store) SetProductDeleted(ctx context.Context, p domain.Product, actorID uuid.UUID) (domain.Product, error) {
	r, err := s.q(ctx).SetProductDeleted(ctx, sqlcgen.SetProductDeletedParams{
		ID: p.ID, DeletedAt: p.DeletedAt, DeletedBy: nullUUID(p.DeletedBy), ActorID: actorID,
	})
	if err != nil {
		return domain.Product{}, notFound(err, "set product deleted")
	}
	return s.one(ctx, r)
}
