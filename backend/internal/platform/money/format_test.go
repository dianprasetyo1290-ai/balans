package money_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"balans/internal/platform/money"
)

func TestRupiah(t *testing.T) {
	tests := map[string]string{
		"0":           "Rp0",
		"999":         "Rp999",
		"1000":        "Rp1.000",
		"1250000":     "Rp1.250.000",
		"12500000.50": "Rp12.500.001",
		"-75000":      "-Rp75.000",
	}
	for in, want := range tests {
		assert.Equal(t, want, money.MustParse(in).Rupiah(), in)
	}
}
