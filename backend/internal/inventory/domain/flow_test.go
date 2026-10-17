package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"balans/internal/inventory/domain"
)

func TestLocationFlowClosingQty(t *testing.T) {
	f := domain.LocationFlow{OpeningQty: d("10"), InQty: d("25"), OutQty: d("3"), ExcludedOutQty: d("12")}
	assert.Equal(t, "20", f.ClosingQty().String())
}
