package domain

import (
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Kode akun persediaan (Desain Finance & Accounting bagian 1) untuk resolver location_inventory_account.
const (
	AccountPackaging   = "1301"
	AccountReprocess   = "1302"
	AccountFGWarehouse = "1311"
	AccountFGConsign   = "1312"
	AccountFGVending   = "1313"
	AccountReturns     = "1314"
)

// InventoryAccount memilih akun persediaan menurut jenis produk catalog dan jenis lokasi: Packaging → 1301,
// bahan reprocess → 1302, Produk Jadi di konsinyasi → 1312, di buffer/mesin vending → 1313, di Gudang Retur
// → 1314, Produk Jadi lain → 1311.
func InventoryAccount(productType string, location LocationType) string {
	if location == LocationReturns {
		return AccountReturns
	}
	switch productType {
	case "packaging":
		return AccountPackaging
	case "reprocess_material":
		return AccountReprocess
	}
	switch location {
	case LocationConsignment:
		return AccountFGConsign
	case LocationVendingBuffer, LocationVendingMachine:
		return AccountFGVending
	}
	return AccountFGWarehouse
}

// AdjustmentLine adalah satu mutasi penyesuaian hasil stock count: Qty negatif = kurang (keluar dari stok
// Tersedia), positif = lebih (masuk stok Tersedia dengan HPP snapshot).
type AdjustmentLine struct {
	LineID    uuid.UUID
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
	UnitCost  decimal.Decimal
}

// Adjustments mengembalikan baris yang perlu disesuaikan (selisih ≠ 0).
func (c StockCount) Adjustments() []AdjustmentLine {
	var out []AdjustmentLine
	for _, l := range c.Lines {
		if l.VarianceQty.IsZero() {
			continue
		}
		out = append(out, AdjustmentLine{LineID: l.ID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.VarianceQty, UnitCost: l.UnitCost})
	}
	return out
}

// AccountSplit adalah nilai selisih bersih per akun persediaan untuk A-14 (positif = kurang/beban).
type AccountSplit struct {
	Account string
	Amount  decimal.Decimal
}

// ShortageSplits menjumlahkan nilai per akun dan membuang akun yang bersih nol; urut kode akun.
func ShortageSplits(values map[string]decimal.Decimal) []AccountSplit {
	out := make([]AccountSplit, 0, len(values))
	for acc, v := range values {
		if v.Round(2).IsZero() {
			continue
		}
		out = append(out, AccountSplit{Account: acc, Amount: v.Round(2)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Account < out[j].Account })
	return out
}
