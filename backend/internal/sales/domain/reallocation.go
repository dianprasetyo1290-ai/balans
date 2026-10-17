package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
)

func sameBatch(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ReservedQty adalah total qty batch yang tercatat untuk alokasi.
func (a Allocation) ReservedQty() decimal.Decimal {
	total := decimal.Zero
	for _, b := range a.Batches {
		total = total.Add(b.Qty)
	}
	return total
}

// Gap adalah qty alokasi yang belum punya stok reserved (batch ditolak QC Outbound dan pengganti belum cukup).
func (a Allocation) Gap() decimal.Decimal {
	g := a.Qty.Sub(a.ReservedQty())
	if g.IsNegative() {
		return decimal.Zero
	}
	return g
}

// RemoveBatchQty mengurangi qty batch alokasi yang ditolak QC Outbound (WMS-12). Qty melebihi batch ditolak.
func (a *Allocation) RemoveBatchQty(batchID *uuid.UUID, qty decimal.Decimal) error {
	if msg := ValidQty(qty); msg != "" {
		return apperr.Validation(map[string]string{"rejectedQty": msg})
	}
	for i := range a.Batches {
		if !sameBatch(a.Batches[i].BatchID, batchID) {
			continue
		}
		if qty.GreaterThan(a.Batches[i].Qty) {
			return apperr.Validation(map[string]string{
				"rejectedQty": "Qty ditolak melebihi qty batch yang dialokasikan " + a.Batches[i].Qty.String(),
			})
		}
		a.Batches[i].Qty = a.Batches[i].Qty.Sub(qty)
		if a.Batches[i].Qty.IsZero() {
			a.Batches = append(a.Batches[:i], a.Batches[i+1:]...)
		}
		return nil
	}
	return apperr.Validation(map[string]string{"batchId": "Batch tidak termasuk alokasi Sales Order"})
}

// AddBatch menambah batch pengganti (digabung bila batch sama sudah ada).
func (a *Allocation) AddBatch(batchID *uuid.UUID, qty decimal.Decimal) {
	for i := range a.Batches {
		if sameBatch(a.Batches[i].BatchID, batchID) {
			a.Batches[i].Qty = a.Batches[i].Qty.Add(qty)
			return
		}
	}
	a.Batches = append(a.Batches, AllocationBatch{BatchID: batchID, Qty: qty})
}

// Allocation mengembalikan alokasi aktif (belum dilepas) berdasarkan ID.
func (o *Order) Allocation(id uuid.UUID) (*Allocation, bool) {
	for i := range o.Allocations {
		if o.Allocations[i].ID == id && !o.Allocations[i].Released {
			return &o.Allocations[i], true
		}
	}
	return nil, false
}

// OutstandingBatches membagi qty belum dikirim alokasi ke batch-nya. Qty terkirim dianggap mengambil batch
// terdepan lebih dulu (urutan FEFO alokasi), sama seperti ledger yang mengeluarkan stok reserved secara FEFO.
func (a Allocation) OutstandingBatches() []AllocationBatch {
	skip := a.ShippedQty
	var out []AllocationBatch
	for _, b := range a.Batches {
		q := b.Qty
		if skip.IsPositive() {
			take := decimal.Min(skip, q)
			q, skip = q.Sub(take), skip.Sub(take)
		}
		if q.IsPositive() {
			out = append(out, AllocationBatch{BatchID: b.BatchID, Qty: q})
		}
	}
	return out
}
