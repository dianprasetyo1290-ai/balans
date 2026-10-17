package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/catalog/domain"
	"balans/internal/platform/apperr"
)

func ptr[T any](v T) *T { return &v }

func qty(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func finishedGood() domain.ProductInput {
	return domain.ProductInput{
		SKU: " ms-cok-30 ", Name: " Meal Shake ", Variant: "Coklat 30 g", Type: domain.TypeFinishedGood,
		PackagingType: domain.PackagingTube, UOM: "pcs", ShelfLifeDays: ptr(365), UnitsPerCarton: ptr(24), IsActive: true,
		Barcodes: []domain.BarcodeInput{{Barcode: "8991234567891"}},
	}
}

func TestNewProductFinishedGood(t *testing.T) {
	p, err := domain.NewProduct(finishedGood())
	require.NoError(t, err)
	assert.Equal(t, "MS-COK-30", p.SKU)
	assert.Equal(t, "Meal Shake", p.Name)
	assert.Equal(t, "Meal Shake Coklat 30 g", p.DisplayName())
	assert.True(t, p.HasBatch, "Produk Jadi selalu memakai batch")
	assert.True(t, p.HasStock())
	b, ok := p.PrimaryBarcode()
	require.True(t, ok, "satu-satunya barcode otomatis menjadi utama")
	assert.Equal(t, domain.SymbologyEAN13, b.Symbology)

	p.Variant = ""
	assert.Equal(t, "Meal Shake", p.DisplayName())
	assert.True(t, p.CanBeComponent())
}

func TestNewProductValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ProductInput)
		want   map[string]string
	}{
		{"sku kosong", func(in *domain.ProductInput) { in.SKU = " " }, map[string]string{"sku": "SKU wajib diisi"}},
		{"sku karakter salah", func(in *domain.ProductInput) { in.SKU = "MS COK" }, map[string]string{"sku": "SKU hanya boleh huruf, angka, dan tanda minus"}},
		{"sku terlalu panjang", func(in *domain.ProductInput) { in.SKU = strings.Repeat("A", 41) }, map[string]string{"sku": "SKU maksimal 40 karakter"}},
		{"jenis tidak dikenal", func(in *domain.ProductInput) { in.Type = "service" }, map[string]string{"productType": "Jenis produk tidak dikenal"}},
		{"nama kosong", func(in *domain.ProductInput) { in.Name = "" }, map[string]string{"name": "Nama produk wajib diisi"}},
		{"nama panjang", func(in *domain.ProductInput) { in.Name = strings.Repeat("a", 161) }, map[string]string{"name": "Nama maksimal 160 karakter"}},
		{"varian panjang", func(in *domain.ProductInput) { in.Variant = strings.Repeat("a", 81) }, map[string]string{"variant": "Varian maksimal 80 karakter"}},
		{"satuan kosong", func(in *domain.ProductInput) { in.UOM = "" }, map[string]string{"uom": "Satuan wajib diisi"}},
		{"satuan panjang", func(in *domain.ProductInput) { in.UOM = strings.Repeat("a", 21) }, map[string]string{"uom": "Satuan maksimal 20 karakter"}},
		{"kemasan tidak dikenal", func(in *domain.ProductInput) { in.PackagingType = "box" }, map[string]string{"packagingType": "Tipe kemasan tidak dikenal"}},
		{"kemasan wajib FG", func(in *domain.ProductInput) { in.PackagingType = "" }, map[string]string{"packagingType": "Tipe kemasan wajib diisi untuk Produk Jadi"}},
		{"umur simpan wajib", func(in *domain.ProductInput) { in.ShelfLifeDays = nil }, map[string]string{"shelfLifeDays": "Umur simpan wajib diisi untuk Produk Jadi"}},
		{"umur simpan nol", func(in *domain.ProductInput) { in.ShelfLifeDays = ptr(0) }, map[string]string{"shelfLifeDays": "Umur simpan minimal 1 hari"}},
		{"isi karton nol", func(in *domain.ProductInput) { in.UnitsPerCarton = ptr(0) }, map[string]string{"unitsPerCarton": "Isi per karton minimal 1"}},
		{"stok minimum negatif", func(in *domain.ProductInput) { in.MinStockQty = ptr(qty("-1")) }, map[string]string{"minStockQty": "Stok minimum tidak boleh negatif dan maksimal 3 desimal"}},
		{"stok minimum 4 desimal", func(in *domain.ProductInput) { in.MinStockQty = ptr(qty("1.0005")) }, map[string]string{"minStockQty": "Stok minimum tidak boleh negatif dan maksimal 3 desimal"}},
		{"komponen di non-bundel", func(in *domain.ProductInput) {
			in.Components = []domain.ComponentInput{{ComponentID: uuid.New(), Qty: qty("1")}}
		}, map[string]string{"bundleComponents": "Komponen hanya untuk bundel"}},
		{"packaging jadi bundel", func(in *domain.ProductInput) {
			in.Type, in.IsBundle, in.Components = domain.TypePackaging, true, []domain.ComponentInput{{ComponentID: uuid.New(), Qty: qty("1")}}
		}, map[string]string{"isBundle": "Hanya Produk Jadi yang bisa menjadi bundel"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := finishedGood()
			tt.mutate(&in)
			_, err := domain.NewProduct(in)
			assert.Equal(t, tt.want, fields(t, err))
		})
	}
}

func TestBarcodes(t *testing.T) {
	tests := []struct {
		name     string
		barcodes []domain.BarcodeInput
		want     map[string]string
	}{
		{"tanpa barcode boleh", nil, nil},
		{"dua barcode tanpa utama", []domain.BarcodeInput{{Barcode: "8991234567891"}, {Barcode: "MS-COK"}}, map[string]string{"barcodes": "Pilih tepat satu barcode utama"}},
		{"dua utama", []domain.BarcodeInput{{Barcode: "8991234567891", IsPrimary: true}, {Barcode: "MS-COK", IsPrimary: true}}, map[string]string{"barcodes": "Pilih tepat satu barcode utama"}},
		{"ganda", []domain.BarcodeInput{{Barcode: "MS-COK", IsPrimary: true}, {Barcode: " MS-COK "}}, map[string]string{"barcodes[1].barcode": "Barcode ganda"}},
		{"kosong", []domain.BarcodeInput{{Barcode: " ", IsPrimary: true}}, map[string]string{"barcodes[0].barcode": "Barcode wajib diisi"}},
		{"ean salah digit", []domain.BarcodeInput{{Barcode: "8991234567890", IsPrimary: true}}, map[string]string{"barcodes[0].barcode": "Digit periksa EAN-13 tidak valid"}},
		{"karakter non-ascii", []domain.BarcodeInput{{Barcode: "KOPI-É", IsPrimary: true}}, map[string]string{"barcodes[0].barcode": "Barcode hanya boleh huruf, angka, dan tanda baca tanpa aksen"}},
		{"terlalu panjang", []domain.BarcodeInput{{Barcode: strings.Repeat("1", 49), IsPrimary: true}}, map[string]string{"barcodes[0].barcode": "Barcode maksimal 48 karakter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := finishedGood()
			in.Barcodes = tt.barcodes
			_, err := domain.NewProduct(in)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tt.want, fields(t, err))
		})
	}

	in := finishedGood()
	in.Barcodes = []domain.BarcodeInput{{Barcode: "8991234567891"}, {Barcode: "MSCOK30", IsPrimary: true}}
	p, err := domain.NewProduct(in)
	require.NoError(t, err)
	b, _ := p.PrimaryBarcode()
	assert.Equal(t, domain.Barcode{Barcode: "MSCOK30", Symbology: domain.SymbologyCode128, IsPrimary: true}, b)
	_, ok := domain.Product{}.PrimaryBarcode()
	assert.False(t, ok)
}

func TestDetectSymbology(t *testing.T) {
	s, err := domain.DetectSymbology("4006381333931")
	require.NoError(t, err)
	assert.Equal(t, domain.SymbologyEAN13, s)
	s, err = domain.DetectSymbology("123456789012") // 12 digit bukan EAN-13
	require.NoError(t, err)
	assert.Equal(t, domain.SymbologyCode128, s)
	assert.Equal(t, byte('1'), domain.EAN13CheckDigit("400638133393"))
	assert.Equal(t, byte('0'), domain.EAN13CheckDigit("000000000000"))
}

func TestPackagingAndMaterial(t *testing.T) {
	in := domain.ProductInput{SKU: "PKG-T30", Name: "Tube 30 ml", Type: domain.TypePackaging, PackagingType: domain.PackagingTube, UOM: "pcs", IsActive: true}
	p, err := domain.NewProduct(in)
	require.NoError(t, err)
	assert.False(t, p.HasBatch, "packaging tanpa batch jika tidak dipilih")
	assert.Nil(t, p.ShelfLifeDays, "umur simpan opsional untuk packaging")
	assert.False(t, p.CanBeComponent())

	in.HasBatch = true
	p, err = domain.NewProduct(in)
	require.NoError(t, err)
	assert.True(t, p.HasBatch, "packaging boleh memakai batch")

	in.PackagingType = ""
	_, err = domain.NewProduct(in)
	assert.Equal(t, map[string]string{"packagingType": "Tipe kemasan wajib diisi untuk Packaging"}, fields(t, err))

	gm := domain.ProductInput{SKU: "GM-COK", Name: "Meal Shake Coklat reprocess", Type: domain.TypeReprocessMaterial, UOM: "pcs", HasBatch: true, IsActive: true}
	p, err = domain.NewProduct(gm)
	require.NoError(t, err)
	assert.True(t, p.HasBatch)
	assert.Equal(t, domain.PackagingType(""), p.PackagingType)
}

func TestBundle(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	in := domain.ProductInput{
		SKU: "BDL-01", Name: "Paket Hemat", Type: domain.TypeFinishedGood, IsBundle: true, UOM: "paket", IsActive: true,
		ShelfLifeDays: ptr(10), HasBatch: true, MinStockQty: ptr(qty("5")),
		Components: []domain.ComponentInput{{ComponentID: a, Qty: qty("2")}, {ComponentID: b, Qty: qty("1")}},
	}
	p, err := domain.NewProduct(in)
	require.NoError(t, err)
	assert.False(t, p.HasStock())
	assert.False(t, p.HasBatch)
	assert.Nil(t, p.ShelfLifeDays)
	assert.Nil(t, p.MinStockQty)
	assert.Len(t, p.Components, 2)
	assert.False(t, p.CanBeComponent(), "bundel tidak bisa menjadi komponen bundel lain")

	tests := []struct {
		name       string
		components []domain.ComponentInput
		want       map[string]string
	}{
		{"tanpa komponen", nil, map[string]string{"bundleComponents": "Bundel wajib punya minimal satu komponen"}},
		{"komponen kosong", []domain.ComponentInput{{Qty: qty("1")}}, map[string]string{"bundleComponents[0].componentProductId": "Produk komponen wajib dipilih"}},
		{"komponen ganda", []domain.ComponentInput{{ComponentID: a, Qty: qty("1")}, {ComponentID: a, Qty: qty("1")}}, map[string]string{"bundleComponents[1].componentProductId": "Komponen ganda"}},
		{"qty nol", []domain.ComponentInput{{ComponentID: a, Qty: qty("0")}}, map[string]string{"bundleComponents[0].qty": "Qty harus lebih dari 0 dan maksimal 3 desimal"}},
		{"qty 4 desimal", []domain.ComponentInput{{ComponentID: a, Qty: qty("0.0001")}}, map[string]string{"bundleComponents[0].qty": "Qty harus lebih dari 0 dan maksimal 3 desimal"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in.Components = tt.components
			_, err := domain.NewProduct(in)
			assert.Equal(t, tt.want, fields(t, err))
		})
	}
}

func TestProductUpdate(t *testing.T) {
	p, err := domain.NewProduct(finishedGood())
	require.NoError(t, err)
	p.ID = uuid.New()

	in := finishedGood()
	in.Name, in.ShelfLifeDays = "Meal Shake Baru", ptr(180)
	require.NoError(t, p.Update(in))
	assert.Equal(t, "Meal Shake Baru", p.Name)
	assert.Equal(t, 180, *p.ShelfLifeDays)

	bad := finishedGood()
	bad.SKU, bad.Type, bad.IsBundle, bad.Name = "LAIN", domain.TypePackaging, true, ""
	bad.Components = []domain.ComponentInput{{ComponentID: p.ID, Qty: qty("1")}}
	assert.Equal(t, map[string]string{
		"sku": "SKU tidak bisa diubah", "productType": "Jenis produk tidak bisa diubah", "isBundle": "Status bundel tidak bisa diubah",
		"name": "Nama produk wajib diisi", "bundleComponents": "Bundel tidak bisa menjadi komponen dirinya sendiri",
	}, fields(t, p.Update(bad)))
	assert.Equal(t, "Meal Shake Baru", p.Name, "update gagal tidak mengubah produk")

	require.NoError(t, p.Delete(time.Now(), uuid.New()))
	assert.True(t, apperr.HasCode(p.Update(in), apperr.CodeInvalidStateTransition), "produk terhapus harus dipulihkan dulu")
	assert.False(t, p.CanBeComponent())
}

func TestAvailableBundleQty(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	comps := []domain.BundleComponent{{ComponentID: a, Qty: qty("2")}, {ComponentID: b, Qty: qty("3")}}
	tests := []struct {
		name  string
		comps []domain.BundleComponent
		stock map[uuid.UUID]decimal.Decimal
		want  string
	}{
		{"minimum komponen", comps, map[uuid.UUID]decimal.Decimal{a: qty("10"), b: qty("9")}, "3"},
		{"dibulatkan ke bawah", comps, map[uuid.UUID]decimal.Decimal{a: qty("11"), b: qty("100")}, "5"},
		{"komponen tanpa stok", comps, map[uuid.UUID]decimal.Decimal{a: qty("10")}, "0"},
		{"stok negatif", comps, map[uuid.UUID]decimal.Decimal{a: qty("-4"), b: qty("9")}, "0"},
		{"stok kurang dari qty", comps, map[uuid.UUID]decimal.Decimal{a: qty("1"), b: qty("9")}, "0"},
		{"qty desimal", []domain.BundleComponent{{ComponentID: c, Qty: qty("0.5")}}, map[uuid.UUID]decimal.Decimal{c: qty("2.4")}, "4"},
		{"qty komponen tidak valid", []domain.BundleComponent{{ComponentID: c, Qty: qty("0")}}, map[uuid.UUID]decimal.Decimal{c: qty("2")}, "0"},
		{"tanpa komponen", nil, nil, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.AvailableBundleQty(tt.comps, tt.stock)
			assert.True(t, got.Equal(qty(tt.want)), "got %s", got)
		})
	}
}

func TestExplodeBundle(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := domain.ExplodeBundle([]domain.BundleComponent{{ComponentID: a, Qty: qty("2")}, {ComponentID: b, Qty: qty("0.5")}}, qty("3"))
	require.Len(t, got, 2)
	assert.Equal(t, a, got[0].ComponentID)
	assert.True(t, got[0].Qty.Equal(qty("6")))
	assert.True(t, got[1].Qty.Equal(qty("1.5")))
}

func TestCategory(t *testing.T) {
	c, err := domain.NewCategory(domain.CategoryInput{Code: " meal ", Name: " Meal Replacement "})
	require.NoError(t, err)
	assert.Equal(t, "MEAL", c.Code)
	assert.Equal(t, "Meal Replacement", c.Name)
	require.NoError(t, c.Rename("Pengganti Makan"))
	assert.Equal(t, "Pengganti Makan", c.Name)
	assert.Equal(t, map[string]string{"name": "Nama wajib diisi"}, fields(t, c.Rename(" ")))

	for _, tt := range []struct {
		in   domain.CategoryInput
		want map[string]string
	}{
		{domain.CategoryInput{Name: "X"}, map[string]string{"code": "Kode wajib diisi"}},
		{domain.CategoryInput{Code: strings.Repeat("A", 21), Name: "X"}, map[string]string{"code": "Kode maksimal 20 karakter"}},
		{domain.CategoryInput{Code: "A B", Name: strings.Repeat("x", 81)}, map[string]string{"code": "Kode hanya boleh huruf, angka, dan tanda minus", "name": "Nama maksimal 80 karakter"}},
	} {
		_, err := domain.NewCategory(tt.in)
		assert.Equal(t, tt.want, fields(t, err))
	}

	require.NoError(t, c.Delete(time.Now(), uuid.New()))
	assert.True(t, apperr.HasCode(c.Rename("Lain"), apperr.CodeInvalidStateTransition))
}
