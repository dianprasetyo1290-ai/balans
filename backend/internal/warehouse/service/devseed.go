package service

import (
	"context"
	"fmt"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/warehouse/domain"
)

// SeedDev membuat Gudang Utama beserta lokasi standarnya (hanya dipanggil devseed di APP_ENV=local, BRD U-08).
func (s *Service) SeedDev(ctx context.Context) error {
	ctx = actor.With(ctx, actor.Actor{FullName: "Sistem", Permissions: map[string]struct{}{PermWarehouseWrite: {}}})
	_, _, err := s.CreateWarehouse(ctx, domain.WarehouseInput{
		Code: "UTAMA", Name: "Gudang Utama", Address: "Gudang fisik pertama Balans", IsActive: true,
	})
	if err != nil && !apperr.HasCode(err, apperr.CodeDuplicateCode) {
		return fmt.Errorf("seed warehouse: %w", err)
	}
	return nil
}
