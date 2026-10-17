package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/catalog/domain"
	"balans/internal/catalog/postgres"
	"balans/internal/catalog/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type harness struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *service.Service
	admin context.Context
	staff context.Context
	// reader hanya boleh melihat produk.
	reader context.Context
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	// Audit trail append-only; ID entitas selalu baru sehingga riwayat antar test tidak tercampur.
	for _, q := range []string{
		`DELETE FROM product_bundle_components`, `DELETE FROM product_barcodes`, `DELETE FROM products`,
		`DELETE FROM product_categories`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	svc := service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}),
		Clock: clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)),
	})
	as := func(name string, roles []string, perms ...string) context.Context {
		set := map[string]struct{}{}
		for _, p := range perms {
			set[p] = struct{}{}
		}
		return actor.With(ctx, actor.Actor{UserID: uuid.New(), FullName: name, Roles: roles, Permissions: set})
	}
	return &harness{
		t: t, pool: pool, svc: svc,
		admin:  as("Ani Admin", []string{"system_admin"}, service.PermProductRead, service.PermProductWrite),
		staff:  as("Hadi Head Logistik", []string{"head_logistics"}, service.PermProductRead, service.PermProductWrite),
		reader: as("Sari Sales", []string{"sales_mt"}, service.PermProductRead),
	}
}

func ptr[T any](v T) *T { return &v }

func qty(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func fg(sku string, barcodes ...domain.BarcodeInput) domain.ProductInput {
	return domain.ProductInput{
		SKU: sku, Name: "Meal Shake", Variant: sku, Type: domain.TypeFinishedGood, PackagingType: domain.PackagingTube,
		UOM: "pcs", ShelfLifeDays: ptr(365), UnitsPerCarton: ptr(24), IsActive: true, Barcodes: barcodes,
	}
}

func (h *harness) create(in domain.ProductInput) domain.Product {
	h.t.Helper()
	p, err := h.svc.CreateProduct(h.admin, in)
	require.NoError(h.t, err)
	return p
}

func (h *harness) auditActions(entityID uuid.UUID) []string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `SELECT action FROM audit_logs WHERE entity_id = $1 ORDER BY created_at, id`, entityID)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(h.t, rows.Scan(&s))
		out = append(out, s)
	}
	return out
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func TestCreateProductWithBarcodesAndCategory(t *testing.T) {
	h := setup(t)
	cat, err := h.svc.CreateCategory(h.admin, domain.CategoryInput{Code: "meal", Name: "Meal Replacement"})
	require.NoError(t, err)

	in := fg("ms-cok-30", domain.BarcodeInput{Barcode: "8991234567891"}, domain.BarcodeInput{Barcode: "MSCOK30", IsPrimary: true})
	in.Barcodes[0].IsPrimary, in.Barcodes[1].IsPrimary = true, false
	in.CategoryID, in.MinStockQty = &cat.ID, ptr(qty("100"))
	p := h.create(in)
	assert.Equal(t, "MS-COK-30", p.SKU)
	assert.Equal(t, "Meal Replacement", p.CategoryName)
	assert.True(t, p.HasBatch)
	require.Len(t, p.Barcodes, 2)
	primary, _ := p.PrimaryBarcode()
	assert.Equal(t, "8991234567891", primary.Barcode)
	assert.Equal(t, domain.SymbologyEAN13, primary.Symbology)
	assert.Equal(t, 1, p.Version)
	assert.Equal(t, []string{"create"}, h.auditActions(p.ID))

	// Lookup: SKU (huruf kecil dinormalkan) dan barcode persis.
	got, err := h.svc.LookupProduct(h.reader, "ms-cok-30")
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	got, err = h.svc.LookupProduct(h.reader, "MSCOK30")
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	_, err = h.svc.LookupProduct(h.reader, "TIDAK-ADA")
	assert.Equal(t, apperr.CodeNotFound, code(err))
	_, err = h.svc.LookupProduct(h.reader, " ")
	assert.Equal(t, apperr.CodeNotFound, code(err))

	// Barcode unik di semua produk.
	_, err = h.svc.CreateProduct(h.admin, fg("MS-VAN-30", domain.BarcodeInput{Barcode: "MSCOK30"}))
	assert.Equal(t, map[string]string{"barcodes[0].barcode": "Barcode sudah dipakai produk lain"}, apperr.As(err).Fields)

	// Kategori terhapus tidak bisa dipilih.
	require.NoError(t, h.svc.DeleteCategory(h.admin, cat.ID))
	in2 := fg("MS-VAN-30")
	in2.CategoryID = &cat.ID
	_, err = h.svc.CreateProduct(h.admin, in2)
	assert.Equal(t, map[string]string{"categoryId": "Kategori tidak ditemukan atau sudah dihapus"}, apperr.As(err).Fields)
	got, err = h.svc.GetProduct(h.reader, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Meal Replacement", got.CategoryName, "produk lama tetap menampilkan kategori terhapus")
}

func TestPermissions(t *testing.T) {
	h := setup(t)
	_, err := h.svc.CreateProduct(h.reader, fg("MS-COK-30"))
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.ListProducts(context.Background(), service.ProductFilter{}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeAuthSessionExpired, code(err))
	_, err = h.svc.CreateCategory(h.reader, domain.CategoryInput{Code: "X", Name: "X"})
	assert.Equal(t, apperr.CodeForbidden, code(err))

	p := h.create(fg("MS-COK-30"))
	assert.Equal(t, []string{"update", "delete"}, service.RecordActions(h.staff, p.Mark, service.PermProductWrite))
	assert.Empty(t, service.RecordActions(h.reader, p.Mark, service.PermProductWrite))
	assert.Empty(t, service.RecordActions(context.Background(), p.Mark, service.PermProductWrite))
	require.NoError(t, h.svc.DeleteProduct(h.staff, p.ID))
	p, err = h.svc.GetProduct(h.reader, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"restore"}, service.RecordActions(h.admin, p.Mark, service.PermProductWrite))
	assert.Empty(t, service.RecordActions(h.staff, p.Mark, service.PermProductWrite), "hanya Admin Sistem yang bisa memulihkan")
}

func TestSKUPermanentAfterSoftDelete(t *testing.T) {
	h := setup(t)
	p := h.create(fg("MS-COK-30", domain.BarcodeInput{Barcode: "MSCOK30"}))

	_, err := h.svc.CreateProduct(h.admin, fg("ms-cok-30"))
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))

	require.NoError(t, h.svc.DeleteProduct(h.staff, p.ID))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteProduct(h.staff, p.ID)))

	// Tidak tampil di list, pencarian, dropdown, maupun lookup...
	list, err := h.svc.ListProducts(h.reader, service.ProductFilter{Search: "MS-COK"}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Empty(t, list.Items)
	opts, err := h.svc.ProductOptions(context.Background(), service.OptionFilter{})
	require.NoError(t, err)
	assert.Empty(t, opts)
	_, err = h.svc.FindByCode(context.Background(), "MSCOK30")
	assert.Equal(t, apperr.CodeNotFound, code(err))

	// ...tetapi tetap terbaca lewat ID untuk dokumen lama, dan tampil di daftar Terhapus.
	old, err := h.svc.ProductByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.True(t, old.Deleted())
	deleted, err := h.svc.ListProducts(h.reader, service.ProductFilter{Deleted: true}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, deleted.Items, 1)

	// SKU tetap tidak bisa dipakai ulang.
	_, err = h.svc.CreateProduct(h.admin, fg("MS-COK-30"))
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))
	assert.Equal(t, map[string]string{"sku": "SKU sudah dipakai, termasuk oleh produk yang sudah dihapus"}, apperr.As(err).Fields)

	// Produk terhapus tidak bisa diubah; pemulihan hanya oleh Admin Sistem dan tercatat di audit trail.
	_, err = h.svc.UpdateProduct(h.staff, p.ID, old.Version, fg("MS-COK-30"))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	_, err = h.svc.RestoreProduct(h.staff, p.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	restored, err := h.svc.RestoreProduct(h.admin, p.ID)
	require.NoError(t, err)
	assert.False(t, restored.Deleted())
	_, err = h.svc.RestoreProduct(h.admin, p.ID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	assert.Equal(t, []string{"create", "delete", "restore"}, h.auditActions(p.ID))

	var changes string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT changes::text FROM audit_logs WHERE entity_id = $1 AND action = 'restore'`, p.ID).Scan(&changes))
	assert.Contains(t, changes, "deletedAt")
}

func TestUpdateProduct(t *testing.T) {
	h := setup(t)
	p := h.create(fg("MS-COK-30", domain.BarcodeInput{Barcode: "MSCOK30"}))

	in := fg("IGNORED", domain.BarcodeInput{Barcode: "MSCOK30-B", IsPrimary: true}, domain.BarcodeInput{Barcode: "MSCOK30"})
	in.SKU, in.Name, in.ShelfLifeDays = p.SKU, "Meal Shake Baru", ptr(180)
	updated, err := h.svc.UpdateProduct(h.staff, p.ID, p.Version, in)
	require.NoError(t, err)
	assert.Equal(t, "Meal Shake Baru", updated.Name)
	assert.Equal(t, 2, updated.Version)
	primary, _ := updated.PrimaryBarcode()
	assert.Equal(t, "MSCOK30-B", primary.Barcode)
	assert.Len(t, updated.Barcodes, 2)

	_, err = h.svc.UpdateProduct(h.staff, p.ID, p.Version, in)
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	in.SKU = "LAIN"
	_, err = h.svc.UpdateProduct(h.staff, p.ID, updated.Version, in)
	assert.Equal(t, map[string]string{"sku": "SKU tidak bisa diubah"}, apperr.As(err).Fields)

	_, err = h.svc.UpdateProduct(h.staff, uuid.New(), 1, in)
	assert.Equal(t, apperr.CodeNotFound, code(err))

	var changes string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT changes::text FROM audit_logs WHERE entity_id = $1 AND action = 'update'`, p.ID).Scan(&changes))
	assert.Contains(t, changes, "shelfLifeDays")
	assert.Contains(t, changes, "MSCOK30-B (utama)")
}

func TestBundle(t *testing.T) {
	h := setup(t)
	cok := h.create(fg("MS-COK-30"))
	van := h.create(fg("MS-VAN-30"))
	pkg := h.create(domain.ProductInput{SKU: "PKG-T30", Name: "Tube 30 ml", Type: domain.TypePackaging, PackagingType: domain.PackagingTube, UOM: "pcs", IsActive: true})

	bundle := domain.ProductInput{
		SKU: "BDL-HEMAT", Name: "Paket Hemat", Type: domain.TypeFinishedGood, IsBundle: true, UOM: "paket", IsActive: true,
		Components: []domain.ComponentInput{{ComponentID: cok.ID, Qty: qty("2")}, {ComponentID: van.ID, Qty: qty("1")}},
	}
	b := h.create(bundle)
	require.Len(t, b.Components, 2)
	assert.Equal(t, "MS-COK-30", b.Components[0].SKU)
	assert.True(t, b.Components[0].Qty.Equal(qty("2")))
	assert.False(t, b.HasBatch)

	got, err := h.svc.ProductByID(context.Background(), b.ID)
	require.NoError(t, err)
	stock := map[uuid.UUID]decimal.Decimal{cok.ID: qty("9"), van.ID: qty("10")}
	assert.True(t, domain.AvailableBundleQty(got.Components, stock).Equal(qty("4")))

	// Komponen harus Produk Jadi aktif non-bundel.
	bad := bundle
	bad.SKU = "BDL-2"
	bad.Components = []domain.ComponentInput{{ComponentID: pkg.ID, Qty: qty("1")}, {ComponentID: b.ID, Qty: qty("1")}, {ComponentID: uuid.New(), Qty: qty("1")}}
	_, err = h.svc.CreateProduct(h.admin, bad)
	assert.Equal(t, map[string]string{
		"bundleComponents[0].componentProductId": "Komponen harus Produk Jadi aktif yang bukan bundel",
		"bundleComponents[1].componentProductId": "Komponen harus Produk Jadi aktif yang bukan bundel",
		"bundleComponents[2].componentProductId": "Produk komponen tidak ditemukan atau sudah dihapus",
	}, apperr.As(err).Fields)

	// Komponen bundel aktif tidak bisa dihapus.
	err = h.svc.DeleteProduct(h.staff, cok.ID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	assert.Contains(t, apperr.As(err).Message, "BDL-HEMAT")

	// Ubah komponen bundel.
	bundle.Components = []domain.ComponentInput{{ComponentID: van.ID, Qty: qty("3")}}
	b, err = h.svc.UpdateProduct(h.staff, b.ID, b.Version, bundle)
	require.NoError(t, err)
	require.Len(t, b.Components, 1)
	assert.Equal(t, "MS-VAN-30", b.Components[0].SKU)

	// Setelah bundel dihapus, komponen bisa dihapus; bundel tidak bisa dipulihkan sebelum komponennya pulih.
	require.NoError(t, h.svc.DeleteProduct(h.staff, b.ID))
	require.NoError(t, h.svc.DeleteProduct(h.staff, van.ID))
	_, err = h.svc.RestoreProduct(h.admin, b.ID)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
	_, err = h.svc.RestoreProduct(h.admin, van.ID)
	require.NoError(t, err)
	_, err = h.svc.RestoreProduct(h.admin, b.ID)
	require.NoError(t, err)
}

func TestListFilterSearchAndOptions(t *testing.T) {
	h := setup(t)
	cok := h.create(fg("MS-COK-30", domain.BarcodeInput{Barcode: "8991234567891"}))
	inactive := fg("MS-OLD")
	inactive.IsActive = false
	h.create(inactive)
	h.create(domain.ProductInput{SKU: "PKG-T30", Name: "Tube 30 ml", Type: domain.TypePackaging, PackagingType: domain.PackagingTube, UOM: "pcs", IsActive: true})
	h.create(domain.ProductInput{SKU: "GM-COK", Name: "Coklat reprocess", Type: domain.TypeReprocessMaterial, UOM: "pcs", IsActive: true})
	h.create(domain.ProductInput{
		SKU: "BDL-1", Name: "Paket", Type: domain.TypeFinishedGood, IsBundle: true, UOM: "paket", IsActive: true,
		Components: []domain.ComponentInput{{ComponentID: cok.ID, Qty: qty("2")}},
	})

	skus := func(f service.ProductFilter) []string {
		t.Helper()
		res, err := h.svc.ListProducts(h.reader, f, paging.New(nil, nil))
		require.NoError(t, err)
		out := make([]string, len(res.Items))
		for i, p := range res.Items {
			out[i] = p.SKU
		}
		assert.Equal(t, int64(len(out)), res.Total)
		return out
	}
	no, yes := false, true
	assert.Equal(t, []string{"BDL-1", "GM-COK", "MS-COK-30", "MS-OLD", "PKG-T30"}, skus(service.ProductFilter{Sort: paging.Sort{Field: "sku"}}))
	assert.Equal(t, []string{"MS-COK-30", "MS-OLD"}, skus(service.ProductFilter{Types: []domain.ProductType{domain.TypeFinishedGood}, IsBundle: &no}))
	assert.Equal(t, []string{"BDL-1"}, skus(service.ProductFilter{IsBundle: &yes}))
	assert.Equal(t, []string{"GM-COK", "PKG-T30"}, skus(service.ProductFilter{Types: []domain.ProductType{domain.TypePackaging, domain.TypeReprocessMaterial}}))
	assert.Equal(t, []string{"MS-COK-30"}, skus(service.ProductFilter{Search: "8991234567891"}), "cari barcode persis")
	assert.Equal(t, []string{"GM-COK", "MS-COK-30"}, skus(service.ProductFilter{Search: "cok"}))
	assert.Equal(t, []string{"MS-OLD"}, skus(service.ProductFilter{IsActive: &no}))
	assert.Equal(t, []string{"PKG-T30", "MS-OLD", "MS-COK-30", "GM-COK", "BDL-1"}, skus(service.ProductFilter{Sort: paging.Sort{Field: "sku", Desc: true}}))

	opts, err := h.svc.ProductOptions(context.Background(), service.OptionFilter{Types: []domain.ProductType{domain.TypeFinishedGood}, IsBundle: &no})
	require.NoError(t, err)
	require.Len(t, opts, 1, "produk nonaktif tidak tampil di dropdown")
	assert.Equal(t, "MS-COK-30", opts[0].SKU)

	byID, err := h.svc.ProductsByIDs(context.Background(), []uuid.UUID{cok.ID, uuid.New()})
	require.NoError(t, err)
	assert.Len(t, byID, 1)
	assert.Equal(t, "MS-COK-30", byID[cok.ID].SKU)
}

func TestCategories(t *testing.T) {
	h := setup(t)
	c, err := h.svc.CreateCategory(h.staff, domain.CategoryInput{Code: "meal", Name: "Meal"})
	require.NoError(t, err)
	_, err = h.svc.CreateCategory(h.staff, domain.CategoryInput{Code: "MEAL", Name: "Lain"})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))

	c, err = h.svc.UpdateCategory(h.staff, c.ID, c.Version, "Meal Replacement")
	require.NoError(t, err)
	assert.Equal(t, "Meal Replacement", c.Name)
	_, err = h.svc.UpdateCategory(h.staff, c.ID, 1, "Lagi")
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	list, err := h.svc.ListCategories(h.reader, false)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, h.svc.DeleteCategory(h.staff, c.ID))
	list, err = h.svc.ListCategories(h.reader, false)
	require.NoError(t, err)
	assert.Empty(t, list)
	list, err = h.svc.ListCategories(h.reader, true)
	require.NoError(t, err)
	require.Len(t, list, 1)

	_, err = h.svc.RestoreCategory(h.staff, c.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	c, err = h.svc.RestoreCategory(h.admin, c.ID)
	require.NoError(t, err)
	assert.False(t, c.Deleted())
	assert.Equal(t, []string{"create", "update", "delete", "restore"}, h.auditActions(c.ID))
}

func TestSeedDevIsIdempotent(t *testing.T) {
	h := setup(t)
	require.NoError(t, h.svc.SeedDev(context.Background()))
	require.NoError(t, h.svc.SeedDev(context.Background()))
	res, err := h.svc.ListProducts(h.reader, service.ProductFilter{}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Equal(t, int64(5), res.Total)
}
