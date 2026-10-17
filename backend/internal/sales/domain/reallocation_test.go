package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/sales/domain"
)

func TestAllocationReallocation(t *testing.T) {
	b1, b2, b3 := uuid.New(), uuid.New(), uuid.New()
	a := domain.Allocation{ID: uuid.New(), Qty: dec("10"), Batches: []domain.AllocationBatch{{BatchID: &b1, Qty: dec("6")}, {BatchID: &b2, Qty: dec("4")}}}
	assert.True(t, a.Gap().IsZero())
	assert.True(t, a.ReservedQty().Equal(dec("10")))

	require.NoError(t, a.RemoveBatchQty(&b1, dec("2")))
	assert.True(t, a.Gap().Equal(dec("2")))
	require.NoError(t, a.RemoveBatchQty(&b2, dec("4")))
	require.Len(t, a.Batches, 1, "batch habis dihapus")
	a.AddBatch(&b3, dec("5"))
	a.AddBatch(&b3, dec("1"))
	require.Len(t, a.Batches, 2)
	assert.True(t, a.Gap().IsZero())

	err := a.RemoveBatchQty(&b2, dec("1"))
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "batch bukan milik alokasi")
	err = a.RemoveBatchQty(&b1, dec("9"))
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "melebihi qty batch")
	err = a.RemoveBatchQty(&b1, dec("0"))
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "qty nol")

	over := domain.Allocation{Qty: dec("1"), Batches: []domain.AllocationBatch{{Qty: dec("3")}}}
	assert.True(t, over.Gap().IsZero())
	require.NoError(t, over.RemoveBatchQty(nil, dec("1")), "produk tanpa batch")
}

func TestAllocationOutstandingBatches(t *testing.T) {
	b1, b2 := uuid.New(), uuid.New()
	a := domain.Allocation{ID: uuid.New(), Qty: dec("10"), ShippedQty: dec("7"), Batches: []domain.AllocationBatch{{BatchID: &b1, Qty: dec("6")}, {BatchID: &b2, Qty: dec("4")}}}
	out := a.OutstandingBatches()
	require.Len(t, out, 1)
	assert.Equal(t, b2, *out[0].BatchID)
	assert.True(t, out[0].Qty.Equal(dec("3")))

	o := domain.Order{Allocations: []domain.Allocation{a, {ID: uuid.New(), Released: true}}}
	_, ok := o.Allocation(uuid.New())
	assert.False(t, ok)
	_, ok = o.Allocation(o.Allocations[1].ID)
	assert.False(t, ok, "alokasi dilepas tidak dikembalikan")
	got, ok := o.Allocation(a.ID)
	require.True(t, ok)
	assert.Equal(t, a.ID, got.ID)
}
