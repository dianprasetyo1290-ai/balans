package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/catalog/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

// Permission impor/ekspor produk (matriks Desain Role & Approval 3.1, kode I).
const (
	PermProductImport = "catalog.product.import"
	PermProductExport = "catalog.product.export"
)

var productTypeOptions = []tabular.Option{
	{Value: string(domain.TypeFinishedGood), Label: "Produk Jadi"},
	{Value: string(domain.TypePackaging), Label: "Packaging"},
	{Value: string(domain.TypeReprocessMaterial), Label: "Material"},
}

var packagingOptions = []tabular.Option{
	{Value: string(domain.PackagingTube), Label: "Tube"},
	{Value: string(domain.PackagingPouch), Label: "Pouch"},
	{Value: string(domain.PackagingRollSachet), Label: "Roll Sachet"},
}

// ProductColumns adalah kolom template impor/ekspor produk (MD-01, MD-02, MD-03). Header mengikuti template
// migrasi Fase 0 sheet Produk dan Packaging & Material; komponen bundel ditulis di satu kolom.
var ProductColumns = []tabular.Column{
	{Key: "sku", Header: "SKU", Kind: tabular.KindText, Required: true, Identity: true, Example: "MS-COK-30",
		Note: "Huruf besar, angka, dan tanda minus. Data dengan SKU yang sudah ada diperbarui."},
	{Key: "name", Header: "Nama Produk", Kind: tabular.KindText, Required: true, Example: "Meal Shake"},
	{Key: "variant", Header: "Varian/Rasa", Kind: tabular.KindText, Example: "Coklat 30 g"},
	{Key: "productType", Header: "Jenis Produk", Kind: tabular.KindChoice, Required: true, Options: productTypeOptions,
		Example: "Produk Jadi", Note: "Tidak bisa diubah untuk SKU yang sudah ada."},
	{Key: "packagingType", Header: "Tipe Kemasan", Kind: tabular.KindChoice, Options: packagingOptions, Example: "Pouch",
		Note: "Wajib untuk Produk Jadi dan Packaging."},
	{Key: "uom", Header: "Satuan", Kind: tabular.KindText, Required: true, Example: "pcs"},
	{Key: "barcode", Header: "Barcode", Kind: tabular.KindText, Example: "8991234567895",
		Note: "Barcode utama. 13 digit = EAN-13 (digit periksa harus benar)."},
	{Key: "shelfLifeDays", Header: "Umur Simpan (hari)", Kind: tabular.KindInteger, Example: "365",
		Note: "Wajib untuk Produk Jadi non-bundel."},
	{Key: "category", Header: "Kategori", Kind: tabular.KindText, Example: "MEAL", Note: "Kode atau nama kategori yang sudah ada."},
	{Key: "isBundle", Header: "Bundel Virtual", Kind: tabular.KindBool, Example: tabular.No,
		Note: "Kosong = Tidak. Tidak bisa diubah untuk SKU yang sudah ada."},
	{Key: "bundleComponents", Header: "Komponen Bundel", Kind: tabular.KindText, Example: "",
		Note: "Untuk bundel: SKU:qty dipisah titik koma, mis. MS-COK-30:2; MS-VAN-30:1. Komponen harus sudah ada atau berada di baris sebelumnya."},
	{Key: "minStockQty", Header: "Stok Minimum", Kind: tabular.KindDecimal, Example: "100"},
	{Key: "unitsPerCarton", Header: "Isi per Karton", Kind: tabular.KindInteger, Example: "24"},
	{Key: "isActive", Header: "Aktif", Kind: tabular.KindBool, Example: tabular.Yes, Note: "Kosong = Ya."},
}

func (s *Service) categoryByRef(ctx context.Context, ref string) (*uuid.UUID, error) {
	if ref == "" {
		return nil, nil
	}
	list, err := s.Store.ListCategories(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		if strings.EqualFold(c.Code, ref) || strings.EqualFold(c.Name, ref) {
			id := c.ID
			return &id, nil
		}
	}
	return nil, apperr.Validation(map[string]string{"category": "Kategori tidak ditemukan"})
}

func parseComponents(ctx context.Context, s *Service, raw string, p *tabular.Parser) []domain.ComponentInput {
	if raw == "" {
		return nil
	}
	var out []domain.ComponentInput
	for part := range strings.SplitSeq(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		sku, qtyText, found := strings.Cut(part, ":")
		qty := decimal.NewFromInt(1)
		if found {
			q, err := decimal.NewFromString(strings.TrimSpace(qtyText))
			if err != nil {
				p.Fail("bundleComponents", "Qty komponen "+sku+" harus angka")
				continue
			}
			qty = q
		}
		c, err := s.Store.ActiveProductBySKU(ctx, domain.NormalizeCode(sku))
		if err != nil {
			p.Fail("bundleComponents", "SKU komponen "+strings.TrimSpace(sku)+" tidak ditemukan")
			continue
		}
		out = append(out, domain.ComponentInput{ComponentID: c.ID, Qty: qty})
	}
	return out
}

// mergeBarcode menjadikan barcode baris sebagai barcode utama tanpa menghapus barcode lain yang sudah ada.
func mergeBarcode(existing []domain.Barcode, code string) []domain.BarcodeInput {
	out := make([]domain.BarcodeInput, 0, len(existing)+1)
	found := false
	for _, b := range existing {
		primary := b.IsPrimary
		if code != "" {
			primary = b.Barcode == code
			found = found || primary
		}
		out = append(out, domain.BarcodeInput{Barcode: b.Barcode, IsPrimary: primary})
	}
	if code != "" && !found {
		out = append(out, domain.BarcodeInput{Barcode: code, IsPrimary: true})
	}
	return out
}

// mapProductFields memetakan field error domain ke key kolom impor.
func mapProductFields(err error) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		switch {
		case k == "categoryId":
			k = "category"
		case strings.HasPrefix(k, "barcodes"):
			k = "barcode"
		case strings.HasPrefix(k, "bundleComponents"):
			k = "bundleComponents"
		}
		if _, dup := fields[k]; !dup {
			fields[k] = v
		}
	}
	return apperr.Validation(fields)
}

// ImportProductRow membuat produk baru atau memperbarui produk dengan SKU yang sama dari satu baris Excel
// (S-125). Aturan sama dengan form produk; dipanggil job impor di transaksi aktif.
func (s *Service) ImportProductRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermProductImport); err != nil {
		return err
	}
	p := tabular.Parse(row)
	in := domain.ProductInput{
		SKU: p.Text("sku"), Name: p.Text("name"), Variant: p.Text("variant"),
		Type:          domain.ProductType(p.Choice("productType", productTypeOptions)),
		PackagingType: domain.PackagingType(p.Choice("packagingType", packagingOptions)),
		UOM:           p.Text("uom"), ShelfLifeDays: p.Int("shelfLifeDays"), IsBundle: p.Bool("isBundle", false),
		MinStockQty: p.Decimal("minStockQty"), UnitsPerCarton: p.Int("unitsPerCarton"), IsActive: p.Bool("isActive", true),
	}
	category, err := s.categoryByRef(ctx, p.Text("category"))
	if err != nil {
		if !apperr.HasCode(err, apperr.CodeValidationFailed) {
			return err
		}
		p.Fail("category", "Kategori tidak ditemukan")
	}
	in.CategoryID = category
	in.Components = parseComponents(ctx, s, p.Text("bundleComponents"), p)
	if err := p.Err(); err != nil {
		return err
	}
	cur, err := s.Store.ActiveProductBySKU(ctx, domain.NormalizeCode(in.SKU))
	switch {
	case apperr.HasCode(err, apperr.CodeNotFound):
		in.Barcodes = mergeBarcode(nil, p.Text("barcode"))
		_, err = s.CreateProduct(ctx, in)
	case err != nil:
		return err
	default:
		in.Barcodes = mergeBarcode(cur.Barcodes, p.Text("barcode"))
		_, err = s.UpdateProduct(ctx, cur.ID, cur.Version, in)
	}
	if err != nil {
		return mapProductFields(err)
	}
	return nil
}

// ExportProducts menulis semua produk yang belum dihapus dengan kolom yang sama dengan template impor: produk
// non-bundel dulu lalu bundel (urut SKU), sehingga file bisa langsung diimpor ulang.
func (s *Service) ExportProducts(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermProductExport); err != nil {
		return err
	}
	for _, bundle := range []bool{false, true} {
		page := paging.Page{Number: 1, Size: paging.MaxSize}
		for {
			res, err := s.Store.ListProducts(ctx, ProductFilter{IsBundle: &bundle, Sort: paging.Sort{Field: "sku"}}, page)
			if err != nil {
				return err
			}
			for _, prod := range res.Items {
				if err := emit(productRow(prod)); err != nil {
					return err
				}
			}
			if int64(page.Number*page.Size) >= res.Total || len(res.Items) == 0 {
				break
			}
			page.Number++
		}
	}
	return nil
}

func productRow(p domain.Product) tabular.Row {
	r := tabular.NewRow(0)
	r.Set("sku", p.SKU).Set("name", p.Name).Set("variant", p.Variant)
	r.Set("productType", tabular.Label(productTypeOptions, string(p.Type)))
	r.Set("packagingType", tabular.Label(packagingOptions, string(p.PackagingType))).Set("uom", p.UOM)
	if b, ok := p.PrimaryBarcode(); ok {
		r.Set("barcode", b.Barcode)
	}
	r.Set("shelfLifeDays", tabular.FormatInt(p.ShelfLifeDays)).Set("category", p.CategoryName)
	r.Set("isBundle", tabular.BoolLabel(p.IsBundle))
	parts := make([]string, len(p.Components))
	for i, c := range p.Components {
		parts[i] = fmt.Sprintf("%s:%s", c.SKU, c.Qty.String())
	}
	slices.Sort(parts)
	r.Set("bundleComponents", strings.Join(parts, "; "))
	r.Set("minStockQty", tabular.FormatDecimal(p.MinStockQty)).Set("unitsPerCarton", tabular.FormatInt(p.UnitsPerCarton))
	r.Set("isActive", tabular.BoolLabel(p.IsActive))
	return r
}
