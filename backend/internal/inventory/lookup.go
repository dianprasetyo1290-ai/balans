package inventory

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/service"
)

type (
	// BalanceFilter adalah kriteria saldo stok (produk, lokasi, batch, status).
	BalanceFilter = service.BalanceFilter
	// BalanceKey adalah kunci saldo produk × lokasi × batch × status.
	BalanceKey = domain.BalanceKey
)

// Balance adalah saldo > 0 satu produk × lokasi × batch × status dengan HPP rata-rata sisa cost layer batch.
type Balance struct {
	Key         BalanceKey
	Qty         decimal.Decimal
	BatchNumber string
	AvgUnitCost decimal.Decimal
}

// Balances membaca saldo > 0 (maks. 5.000 baris) tanpa cek permission, mis. snapshot Stock Count satu lokasi.
func (m *Module) Balances(ctx context.Context, f BalanceFilter) ([]Balance, error) {
	rows, err := m.svc.LookupBalances(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]Balance, len(rows))
	for i, r := range rows {
		out[i] = Balance{Key: r.Key, Qty: r.Qty, BatchNumber: r.BatchNumber, AvgUnitCost: r.AvgUnitCost}
	}
	return out, nil
}

// Batches membaca banyak batch berdasarkan ID; ID yang tidak ada tidak ikut di peta.
func (m *Module) Batches(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Batch, error) {
	return m.svc.LookupBatches(ctx, ids)
}
