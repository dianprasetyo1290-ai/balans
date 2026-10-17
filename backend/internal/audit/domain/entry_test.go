package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit/domain"
)

type product struct {
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	IsActive bool   `json:"isActive"`
	Version  int    `json:"version"`
}

type employee struct {
	Name       string `json:"name"`
	NIK        string `json:"nik"`
	BankEnc    string `json:"bankAccountEnc"`
	Department string `json:"department"`
}

func TestDiffUpdateOnlyChangedFields(t *testing.T) {
	e := domain.Entry{
		Before: product{SKU: "BLN-001", Name: "Lama", IsActive: true, Version: 1},
		After:  product{SKU: "BLN-001", Name: "Baru", IsActive: false, Version: 2},
	}
	changes, err := e.Diff()
	require.NoError(t, err)

	assert.Equal(t, map[string]domain.Change{
		"name":     {From: "Lama", To: "Baru"},
		"isActive": {From: true, To: false},
	}, changes)
}

func TestDiffCreateAndDelete(t *testing.T) {
	created, err := domain.Entry{After: product{SKU: "BLN-002", Name: "Baru"}}.Diff()
	require.NoError(t, err)
	assert.Equal(t, domain.Change{To: "BLN-002"}, created["sku"])

	deleted, err := domain.Entry{Before: product{SKU: "BLN-002"}}.Diff()
	require.NoError(t, err)
	assert.Equal(t, domain.Change{From: "BLN-002"}, deleted["sku"])

	// Field null di sisi yang ada dan tidak ada di sisi lain bukan perubahan.
	empty, err := domain.Entry{After: map[string]any{"sku": "BLN-003", "shelfLifeDays": nil}}.Diff()
	require.NoError(t, err)
	assert.NotContains(t, empty, "shelfLifeDays")
	cleared, err := domain.Entry{Before: map[string]any{"minStockQty": "5"}, After: map[string]any{"minStockQty": nil}}.Diff()
	require.NoError(t, err)
	assert.Equal(t, domain.Change{From: "5"}, cleared["minStockQty"])
}

func TestDiffMasksSensitiveFields(t *testing.T) {
	e := domain.Entry{
		Before:     employee{Name: "A", NIK: "3201", BankEnc: "k1:xxx", Department: "Sales"},
		After:      employee{Name: "A", NIK: "3202", BankEnc: "k1:yyy", Department: "Gudang"},
		MaskFields: []string{"department"},
	}
	changes, err := e.Diff()
	require.NoError(t, err)

	assert.Equal(t, domain.Change{From: domain.MaskedValue, To: domain.MaskedValue}, changes["nik"])
	assert.Equal(t, domain.Change{From: domain.MaskedValue, To: domain.MaskedValue}, changes["bankAccountEnc"])
	assert.Equal(t, domain.Change{From: domain.MaskedValue, To: domain.MaskedValue}, changes["department"])
	assert.NotContains(t, changes, "name")
}

func TestDiffAcceptsMapsAndNil(t *testing.T) {
	changes, err := domain.Entry{Before: nil, After: map[string]any{"status": "approved"}}.Diff()
	require.NoError(t, err)
	assert.Equal(t, domain.Change{To: "approved"}, changes["status"])

	_, err = domain.Entry{After: func() {}}.Diff()
	assert.Error(t, err)
}
