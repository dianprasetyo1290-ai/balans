// Package domain berisi entity master produk (Produk Jadi, Packaging, Material), barcode, bundel virtual,
// dan kategori beserta validasinya (MD-01, MD-02, MD-03).
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

// ProductType adalah jenis produk. Label UI mengikuti Naming Convention bagian 4.
type ProductType string

const (
	// TypeFinishedGood adalah Produk Jadi (FG).
	TypeFinishedGood ProductType = "finished_good"
	// TypePackaging adalah Packaging (RM, raw material kemasan).
	TypePackaging ProductType = "packaging"
	// TypeReprocessMaterial adalah Material (GM, barang jadi tidak layak jual untuk diproses ulang maklon).
	TypeReprocessMaterial ProductType = "reprocess_material"
)

// Valid memeriksa jenis produk.
func (t ProductType) Valid() bool {
	switch t {
	case TypeFinishedGood, TypePackaging, TypeReprocessMaterial:
		return true
	}
	return false
}

// PackagingType adalah tipe kemasan (MD-01, MD-02). Kosong = tidak diisi.
type PackagingType string

const (
	PackagingTube       PackagingType = "tube"
	PackagingPouch      PackagingType = "pouch"
	PackagingRollSachet PackagingType = "roll_sachet"
)

// Valid memeriksa tipe kemasan (kosong dianggap valid).
func (t PackagingType) Valid() bool {
	switch t {
	case "", PackagingTube, PackagingPouch, PackagingRollSachet:
		return true
	}
	return false
}

// Batas panjang field.
const (
	MaxSKULength     = 40
	MaxNameLength    = 160
	MaxVariantLength = 80
	MaxUOMLength     = 20
	maxQtyScale      = 3
)

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

// NormalizeCode merapikan kode bisnis (SKU, kode kategori): huruf besar tanpa spasi di tepi.
func NormalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// Product adalah satu SKU. SKU unik permanen walau produk dihapus (Technical Guideline 5.13).
type Product struct {
	ID             uuid.UUID
	SKU            string
	Name           string
	Variant        string
	Type           ProductType
	PackagingType  PackagingType
	UOM            string
	ShelfLifeDays  *int
	HasBatch       bool
	IsBundle       bool
	CategoryID     *uuid.UUID
	MinStockQty    *decimal.Decimal
	UnitsPerCarton *int
	IsActive       bool
	Barcodes       []Barcode
	Components     []BundleComponent
	softdelete.Mark

	// CategoryName diisi saat dibaca untuk tampilan.
	CategoryName string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// ProductInput adalah isian form produk.
type ProductInput struct {
	SKU            string
	Name           string
	Variant        string
	Type           ProductType
	PackagingType  PackagingType
	UOM            string
	ShelfLifeDays  *int
	HasBatch       bool
	IsBundle       bool
	CategoryID     *uuid.UUID
	MinStockQty    *decimal.Decimal
	UnitsPerCarton *int
	IsActive       bool
	Barcodes       []BarcodeInput
	Components     []ComponentInput
}

// DisplayName menggabungkan nama dan varian, mis. "Meal Shake Coklat 30 g".
func (p Product) DisplayName() string {
	if p.Variant == "" {
		return p.Name
	}
	return p.Name + " " + p.Variant
}

// PrimaryBarcode mengembalikan barcode utama yang dicetak di label produk.
func (p Product) PrimaryBarcode() (Barcode, bool) {
	for _, b := range p.Barcodes {
		if b.IsPrimary {
			return b, true
		}
	}
	return Barcode{}, false
}

// HasStock memeriksa apakah produk punya stok sendiri (bundel virtual tidak).
func (p Product) HasStock() bool { return !p.IsBundle }

// CanBeComponent memeriksa apakah produk boleh menjadi komponen bundel: Produk Jadi non-bundel yang aktif.
func (p Product) CanBeComponent() bool {
	return p.Type == TypeFinishedGood && !p.IsBundle && p.IsActive && !p.Deleted()
}

// NewProduct memvalidasi dan merapikan isian produk baru.
func NewProduct(in ProductInput) (Product, error) {
	p := Product{SKU: NormalizeCode(in.SKU), Type: in.Type, IsBundle: in.IsBundle}
	fields := map[string]string{}
	switch {
	case p.SKU == "":
		fields["sku"] = "SKU wajib diisi"
	case len(p.SKU) > MaxSKULength:
		fields["sku"] = fmt.Sprintf("SKU maksimal %d karakter", MaxSKULength)
	case !codePattern.MatchString(p.SKU):
		fields["sku"] = "SKU hanya boleh huruf, angka, dan tanda minus"
	}
	if !p.Type.Valid() {
		fields["productType"] = "Jenis produk tidak dikenal"
	}
	if p.IsBundle && p.Type != TypeFinishedGood {
		fields["isBundle"] = "Hanya Produk Jadi yang bisa menjadi bundel"
	}
	p.apply(in, fields)
	if len(fields) > 0 {
		return Product{}, apperr.Validation(fields)
	}
	return p, nil
}

// Update mengubah produk. SKU, jenis produk, dan status bundel tidak bisa diubah karena dipakai dokumen dan stok.
func (p *Product) Update(in ProductInput) error {
	if err := p.EnsureActive(); err != nil {
		return err
	}
	fields := map[string]string{}
	if sku := NormalizeCode(in.SKU); sku != "" && sku != p.SKU {
		fields["sku"] = "SKU tidak bisa diubah"
	}
	if in.Type != "" && in.Type != p.Type {
		fields["productType"] = "Jenis produk tidak bisa diubah"
	}
	if in.IsBundle != p.IsBundle {
		fields["isBundle"] = "Status bundel tidak bisa diubah"
	}
	next := *p
	next.apply(in, fields)
	for _, c := range in.Components {
		if c.ComponentID == p.ID {
			fields["bundleComponents"] = "Bundel tidak bisa menjadi komponen dirinya sendiri"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	*p = next
	return nil
}

// apply memvalidasi field yang boleh diubah dan menurunkan field yang ditentukan aturan jenis produk.
func (p *Product) apply(in ProductInput, fields map[string]string) {
	p.Name, p.Variant, p.UOM = strings.TrimSpace(in.Name), strings.TrimSpace(in.Variant), strings.TrimSpace(in.UOM)
	p.PackagingType, p.CategoryID, p.IsActive = in.PackagingType, in.CategoryID, in.IsActive
	p.ShelfLifeDays, p.MinStockQty, p.UnitsPerCarton, p.HasBatch = in.ShelfLifeDays, in.MinStockQty, in.UnitsPerCarton, in.HasBatch

	switch {
	case p.Name == "":
		fields["name"] = "Nama produk wajib diisi"
	case len([]rune(p.Name)) > MaxNameLength:
		fields["name"] = fmt.Sprintf("Nama maksimal %d karakter", MaxNameLength)
	}
	if len([]rune(p.Variant)) > MaxVariantLength {
		fields["variant"] = fmt.Sprintf("Varian maksimal %d karakter", MaxVariantLength)
	}
	switch {
	case p.UOM == "":
		fields["uom"] = "Satuan wajib diisi"
	case len([]rune(p.UOM)) > MaxUOMLength:
		fields["uom"] = fmt.Sprintf("Satuan maksimal %d karakter", MaxUOMLength)
	}
	if !p.PackagingType.Valid() {
		fields["packagingType"] = "Tipe kemasan tidak dikenal"
	}
	if p.ShelfLifeDays != nil && *p.ShelfLifeDays < 1 {
		fields["shelfLifeDays"] = "Umur simpan minimal 1 hari"
	}
	if p.UnitsPerCarton != nil && *p.UnitsPerCarton < 1 {
		fields["unitsPerCarton"] = "Isi per karton minimal 1"
	}
	if q := p.MinStockQty; q != nil && (q.IsNegative() || !q.Equal(q.Round(maxQtyScale))) {
		fields["minStockQty"] = "Stok minimum tidak boleh negatif dan maksimal 3 desimal"
	}

	switch {
	case p.IsBundle:
		// Bundel virtual tidak punya stok, batch, umur simpan, maupun stok minimum sendiri.
		p.HasBatch, p.ShelfLifeDays, p.MinStockQty = false, nil, nil
	case p.Type == TypeFinishedGood:
		// Produk Jadi selalu dilacak per batch untuk FEFO dan kedaluwarsa (INV-04).
		p.HasBatch = true
		if p.ShelfLifeDays == nil {
			fields["shelfLifeDays"] = "Umur simpan wajib diisi untuk Produk Jadi"
		}
		if p.PackagingType == "" {
			fields["packagingType"] = "Tipe kemasan wajib diisi untuk Produk Jadi"
		}
	case p.Type == TypePackaging:
		if p.PackagingType == "" {
			fields["packagingType"] = "Tipe kemasan wajib diisi untuk Packaging"
		}
	}

	p.Barcodes = normalizeBarcodes(in.Barcodes, fields)
	p.Components = normalizeComponents(p.IsBundle, in.Components, fields)
}

func normalizeComponents(isBundle bool, in []ComponentInput, fields map[string]string) []BundleComponent {
	if !isBundle {
		if len(in) > 0 {
			fields["bundleComponents"] = "Komponen hanya untuk bundel"
		}
		return nil
	}
	if len(in) == 0 {
		fields["bundleComponents"] = "Bundel wajib punya minimal satu komponen"
		return nil
	}
	out := make([]BundleComponent, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for i, c := range in {
		switch {
		case c.ComponentID == uuid.Nil:
			fields[fmt.Sprintf("bundleComponents[%d].componentProductId", i)] = "Produk komponen wajib dipilih"
		case seen[c.ComponentID]:
			fields[fmt.Sprintf("bundleComponents[%d].componentProductId", i)] = "Komponen ganda"
		}
		if !c.Qty.IsPositive() || !c.Qty.Equal(c.Qty.Round(maxQtyScale)) {
			fields[fmt.Sprintf("bundleComponents[%d].qty", i)] = "Qty harus lebih dari 0 dan maksimal 3 desimal"
		}
		seen[c.ComponentID] = true
		out = append(out, BundleComponent{ComponentID: c.ComponentID, Qty: c.Qty})
	}
	return out
}
