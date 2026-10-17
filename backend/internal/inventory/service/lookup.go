package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/paging"
)

// maxLookupBalances membatasi saldo yang dibaca modul lain sekaligus (snapshot stock count satu lokasi).
const maxLookupBalances = 5000

// LookupBalances mengembalikan saldo > 0 sesuai filter tanpa cek permission (dipakai modul dokumen gudang).
func (s *Service) LookupBalances(ctx context.Context, f BalanceFilter) ([]postgres.BalanceRow, error) {
	rows, _, err := s.Store.ListBalances(ctx, f, paging.Page{Number: 1, Size: maxLookupBalances})
	return rows, err
}

// LookupBatches membaca banyak batch berdasarkan ID tanpa cek permission.
func (s *Service) LookupBatches(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Batch, error) {
	return s.Store.BatchesByIDs(ctx, ids)
}
