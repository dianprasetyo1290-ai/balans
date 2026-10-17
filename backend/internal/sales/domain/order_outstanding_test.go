package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"balans/internal/sales/domain"
)

func TestOrderHasOutstanding(t *testing.T) {
	five, two := dec("5"), dec("2")
	cases := []struct {
		name   string
		status domain.Status
		allocs []domain.Allocation
		want   bool
	}{
		{"diproses sisa", domain.StatusInProgress, []domain.Allocation{{Qty: five, ShippedQty: two}}, true},
		{"diproses terkirim", domain.StatusInProgress, []domain.Allocation{{Qty: five, ShippedQty: five}}, false},
		{"dilepas", domain.StatusInProgress, []domain.Allocation{{Qty: five, ShippedQty: two, Released: true}}, false},
		{"bukan diproses", domain.StatusInTransit, []domain.Allocation{{Qty: five, ShippedQty: two}}, false},
		{"tanpa alokasi", domain.StatusInProgress, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, domain.Order{Status: c.status, Allocations: c.allocs}.HasOutstanding())
		})
	}
}
