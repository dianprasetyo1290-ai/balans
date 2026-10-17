package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/catalog/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

func devActor(ctx context.Context) context.Context {
	return actor.With(ctx, actor.Actor{
		FullName: "Sistem", Roles: []string{softdelete.RestoreRole},
		Permissions: map[string]struct{}{PermProductRead: {}, PermProductWrite: {}},
	})
}

// SeedDev membuat kategori dan produk contoh (hanya dipanggil devseed di APP_ENV=local). Kode yang sudah ada dilewati.
func (s *Service) SeedDev(ctx context.Context) error {
	ctx = devActor(ctx)
	var category *uuid.UUID
	c, err := s.CreateCategory(ctx, domain.CategoryInput{Code: "MEAL", Name: "Meal Replacement"})
	switch {
	case err == nil:
		category = &c.ID
	case !apperr.HasCode(err, apperr.CodeDuplicateCode):
		return fmt.Errorf("seed category: %w", err)
	}
	days, carton := 365, 24
	samples := []domain.ProductInput{
		{SKU: "MS-COK-30", Name: "Meal Shake", Variant: "Coklat 30 g", Type: domain.TypeFinishedGood, PackagingType: domain.PackagingTube,
			UOM: "pcs", ShelfLifeDays: &days, UnitsPerCarton: &carton, CategoryID: category, IsActive: true,
			Barcodes: []domain.BarcodeInput{{Barcode: "8991234567891", IsPrimary: true}}},
		{SKU: "MS-VAN-30", Name: "Meal Shake", Variant: "Vanila 30 g", Type: domain.TypeFinishedGood, PackagingType: domain.PackagingPouch,
			UOM: "pcs", ShelfLifeDays: &days, UnitsPerCarton: &carton, CategoryID: category, IsActive: true,
			Barcodes: []domain.BarcodeInput{{Barcode: "MSVAN30", IsPrimary: true}}},
		{SKU: "PKG-TUBE-30", Name: "Tube 30 ml", Type: domain.TypePackaging, PackagingType: domain.PackagingTube, UOM: "pcs", IsActive: true},
		{SKU: "GM-MS-COK", Name: "Meal Shake Coklat (reprocess)", Type: domain.TypeReprocessMaterial, UOM: "pcs", HasBatch: true, IsActive: true},
	}
	ids := map[string]uuid.UUID{}
	for _, in := range samples {
		id, err := s.seedProduct(ctx, in)
		if err != nil {
			return err
		}
		ids[in.SKU] = id
	}
	_, err = s.seedProduct(ctx, domain.ProductInput{
		SKU: "BDL-HEMAT", Name: "Paket Hemat Meal Shake", Variant: "2 Coklat + 1 Vanila", Type: domain.TypeFinishedGood,
		IsBundle: true, UOM: "paket", IsActive: true,
		Components: []domain.ComponentInput{
			{ComponentID: ids["MS-COK-30"], Qty: decimal.NewFromInt(2)},
			{ComponentID: ids["MS-VAN-30"], Qty: decimal.NewFromInt(1)},
		},
	})
	return err
}

func (s *Service) seedProduct(ctx context.Context, in domain.ProductInput) (uuid.UUID, error) {
	p, err := s.Store.ActiveProductBySKU(ctx, in.SKU)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		p, err = s.CreateProduct(ctx, in)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed product %s: %w", in.SKU, err)
	}
	return p.ID, nil
}
