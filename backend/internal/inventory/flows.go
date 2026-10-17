package inventory

import (
	"context"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/service"
)

type (
	// LocationFlowQuery adalah kriteria mutasi satu lokasi dalam periode (DateFrom s.d. DateTo).
	LocationFlowQuery = service.LocationFlowQuery
	// LocationFlow adalah saldo awal, masuk, dan keluar satu produk di lokasi.
	LocationFlow = domain.LocationFlow
)

// LocationFlows membaca mutasi per produk satu lokasi tanpa cek permission, mis. Rekonsiliasi Konsinyasi: saldo
// awal (sebelum DateFrom), kiriman masuk, dan tarikan/keluar dalam periode. Pindah status di lokasi yang sama
// tidak dihitung; status kosong = Tersedia + Reserved.
func (m *Module) LocationFlows(ctx context.Context, q LocationFlowQuery) ([]LocationFlow, error) {
	return m.svc.LocationFlows(ctx, q)
}
