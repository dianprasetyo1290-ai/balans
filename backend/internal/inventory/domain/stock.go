// Package domain berisi aturan ledger stok: status stok, posting mutasi, kunci saldo berurutan, alokasi
// FEFO, dan konsumsi HPP FIFO per batch (INV-01, INV-03, INV-09; Technical Guideline 5.7, 7.2). Tanpa DB/HTTP.
package domain

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
)

// StockStatus adalah status stok (Technical Guideline 5.7.2). Perubahan status adalah movement.
type StockStatus string

const (
	// StatusAvailable bisa dialokasikan/dijual.
	StatusAvailable StockStatus = "available"
	// StatusQuarantine menunggu QC; tidak bisa dialokasikan.
	StatusQuarantine StockStatus = "quarantine"
	// StatusInTransit dalam pengiriman antar gudang/ke pihak luar.
	StatusInTransit StockStatus = "in_transit"
	// StatusReserved sudah dialokasikan ke dokumen (reservasi).
	StatusReserved StockStatus = "reserved"
)

// Statuses adalah semua status stok dengan urutan tampilan.
var Statuses = []StockStatus{StatusAvailable, StatusQuarantine, StatusInTransit, StatusReserved}

var statusLabels = map[StockStatus]string{
	StatusAvailable: "Tersedia", StatusQuarantine: "Karantina", StatusInTransit: "Dalam Pengiriman", StatusReserved: "Dialokasikan",
}

// Valid memeriksa status.
func (s StockStatus) Valid() bool {
	_, ok := statusLabels[s]
	return ok
}

// Label adalah nama status untuk tampilan.
func (s StockStatus) Label() string { return statusLabels[s] }

// MovementType adalah jenis baris ledger.
type MovementType string

const (
	// MovementReceive menambah stok dan membuat cost layer.
	MovementReceive MovementType = "receive"
	// MovementIssue mengurangi stok dan mengonsumsi cost layer FIFO (HPP).
	MovementIssue MovementType = "issue"
	// MovementMoveOut dan MovementMoveIn adalah pasangan pindah lokasi/status; cost layer tidak dikonsumsi.
	MovementMoveOut MovementType = "move_out"
	MovementMoveIn  MovementType = "move_in"
)

var movementLabels = map[MovementType]string{
	MovementReceive: "Masuk", MovementIssue: "Keluar", MovementMoveOut: "Pindah keluar", MovementMoveIn: "Pindah masuk",
}

// Label adalah nama jenis movement untuk tampilan.
func (t MovementType) Label() string { return movementLabels[t] }

// QtyScale adalah jumlah desimal qty (numeric(18,3)).
const QtyScale = 3

// maxQty adalah batas numeric(18,3).
var maxQty = decimal.New(1, 15)

// ValidateQty memeriksa qty positif dengan maks. 3 desimal (value object inventory.Quantity, TG 5.3).
func ValidateQty(q decimal.Decimal) string {
	switch {
	case !q.IsPositive():
		return "Qty harus lebih dari 0"
	case q.Exponent() < -QtyScale && !q.Equal(q.Round(QtyScale)):
		return "Qty maksimal 3 angka di belakang koma"
	case q.GreaterThanOrEqual(maxQty):
		return "Qty terlalu besar"
	}
	return ""
}

// BalanceKey adalah kunci saldo stok. BatchID nil = produk tanpa batch.
type BalanceKey struct {
	ProductID  uuid.UUID
	LocationID uuid.UUID
	BatchID    *uuid.UUID
	Status     StockStatus
}

func batchOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// SameBatch membandingkan dua batch (nil = tanpa batch).
func SameBatch(a, b *uuid.UUID) bool { return batchOrNil(a) == batchOrNil(b) }

// Compare mengurutkan kunci (product_id, location_id, batch_id, stock_status); urutan ini WAJIB dipakai saat
// mengunci saldo agar tidak deadlock (Technical Guideline 7.2). Batch nil diurutkan paling awal.
func (k BalanceKey) Compare(o BalanceKey) int {
	if c := bytes.Compare(k.ProductID[:], o.ProductID[:]); c != 0 {
		return c
	}
	if c := bytes.Compare(k.LocationID[:], o.LocationID[:]); c != 0 {
		return c
	}
	a, b := batchOrNil(k.BatchID), batchOrNil(o.BatchID)
	if c := bytes.Compare(a[:], b[:]); c != 0 {
		return c
	}
	return cmp.Compare(k.Status, o.Status)
}

// String untuk log dan kunci peta.
func (k BalanceKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", k.ProductID, k.LocationID, batchOrNil(k.BatchID), k.Status)
}

// BalanceDelta adalah perubahan bersih satu kunci saldo di satu posting.
type BalanceDelta struct {
	Key   BalanceKey
	Delta decimal.Decimal
}

// NetDeltas menjumlahkan movement per kunci dan mengurutkannya untuk penguncian berurutan.
func NetDeltas(movements []Movement) []BalanceDelta {
	idx := map[string]int{}
	var out []BalanceDelta
	for _, m := range movements {
		k := m.Key()
		s := k.String()
		if i, ok := idx[s]; ok {
			out[i].Delta = out[i].Delta.Add(m.Qty)
			continue
		}
		idx[s] = len(out)
		out = append(out, BalanceDelta{Key: k, Delta: m.Qty})
	}
	slices.SortFunc(out, func(a, b BalanceDelta) int { return a.Key.Compare(b.Key) })
	return out
}

// InsufficientStock membuat STOCK_INSUFFICIENT dengan pesan Lampiran A: "Stok {produk} di {lokasi} tidak cukup.
// Tersedia {qty}."
func InsufficientStock(product, location string, status StockStatus, available decimal.Decimal) error {
	where := location
	if status != StatusAvailable {
		where += " (" + status.Label() + ")"
	}
	return apperr.WithMessage(apperr.CodeStockInsufficient,
		fmt.Sprintf("Stok %s di %s tidak cukup. Tersedia %s.", product, where, FormatQty(available)))
}

// FormatQty menampilkan qty tanpa nol berlebih, mis. 12 atau 2,5.
func FormatQty(q decimal.Decimal) string {
	s := q.Round(QtyScale).String()
	out := []byte(s)
	for i, c := range out {
		if c == '.' {
			out[i] = ','
		}
	}
	return string(out)
}
