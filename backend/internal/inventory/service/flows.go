package service

import (
	"context"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
)

// LocationFlowQuery adalah kriteria mutasi satu lokasi.
type LocationFlowQuery = postgres.LocationFlowQuery

// LocationFlows mengembalikan mutasi per produk satu lokasi dalam periode tanpa cek permission (rekonsiliasi
// konsinyasi). Status kosong = Tersedia + Reserved (stok fisik di lokasi).
func (s *Service) LocationFlows(ctx context.Context, q LocationFlowQuery) ([]domain.LocationFlow, error) {
	if len(q.Statuses) == 0 {
		q.Statuses = []domain.StockStatus{domain.StatusAvailable, domain.StatusReserved}
	}
	return s.Store.LocationFlows(ctx, q)
}
