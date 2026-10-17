package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/sales/domain"
)

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	out := make([]string, 0, len(ae.Fields))
	for k := range ae.Fields {
		out = append(out, k)
	}
	return out
}

func TestNewReturnReason(t *testing.T) {
	tests := []struct {
		name   string
		in     domain.ReturnReasonInput
		fields []string
	}{
		{"valid", domain.ReturnReasonInput{Code: " damaged_shipping ", Name: " Rusak ", AppliesTo: domain.ScopeBoth, DefaultDisposition: domain.DispositionRepack, IsActive: true}, nil},
		{"tanpa disposisi", domain.ReturnReasonInput{Code: "OTHER", Name: "Lainnya", AppliesTo: domain.ScopeSalesReturn}, nil},
		{"kode salah", domain.ReturnReasonInput{Code: "x", Name: "x", AppliesTo: domain.ScopeBoth}, []string{"code"}},
		{"nama kosong", domain.ReturnReasonInput{Code: "ABC", AppliesTo: domain.ScopeBoth}, []string{"name"}},
		{"nama panjang", domain.ReturnReasonInput{Code: "ABC", Name: strings.Repeat("a", 151), AppliesTo: domain.ScopeBoth}, []string{"name"}},
		{"cakupan kosong", domain.ReturnReasonInput{Code: "ABC", Name: "x"}, []string{"appliesTo"}},
		{"disposisi salah", domain.ReturnReasonInput{Code: "ABC", Name: "x", AppliesTo: domain.ScopeBoth, DefaultDisposition: "sell"}, []string{"defaultDisposition"}},
		{"keterangan panjang", domain.ReturnReasonInput{Code: "ABC", Name: "x", AppliesTo: domain.ScopeBoth, Description: strings.Repeat("a", 501)}, []string{"description"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := domain.NewReturnReason(tt.in)
			if tt.fields == nil {
				require.NoError(t, err)
				assert.Equal(t, strings.ToUpper(strings.TrimSpace(tt.in.Code)), r.Code)
				assert.Equal(t, strings.TrimSpace(tt.in.Name), r.Name)
				return
			}
			assert.ElementsMatch(t, tt.fields, fieldsOf(t, err))
		})
	}
}

func TestReturnReasonUpdateAndUsable(t *testing.T) {
	r, err := domain.NewReturnReason(domain.ReturnReasonInput{Code: "EXPIRED", Name: "Kedaluwarsa", AppliesTo: domain.ScopeSalesReturn, IsActive: true})
	require.NoError(t, err)
	assert.Empty(t, r.UsableError(domain.ScopeSalesReturn))
	assert.Contains(t, r.UsableError(domain.ScopePurchaseReturn), "tidak berlaku untuk retur pembelian")

	require.NoError(t, r.Update(domain.ReturnReasonInput{
		Code: "IGNORED", Name: "Kedaluwarsa/rusak", AppliesTo: domain.ScopeBoth, DefaultDisposition: domain.DispositionDispose,
		Description: " musnah ", IsActive: true,
	}))
	assert.Equal(t, "EXPIRED", r.Code)
	assert.Equal(t, domain.DispositionDispose, r.DefaultDisposition)
	assert.Equal(t, "musnah", r.Description)
	assert.Empty(t, r.UsableError(domain.ScopePurchaseReturn))

	require.NoError(t, r.Update(domain.ReturnReasonInput{Name: "x", AppliesTo: domain.ScopeBoth}))
	assert.Contains(t, r.UsableError(domain.ScopeSalesReturn), "tidak aktif")

	err = r.Update(domain.ReturnReasonInput{Name: "x", AppliesTo: "all"})
	assert.ElementsMatch(t, []string{"appliesTo"}, fieldsOf(t, err))
}

func TestReturnLabels(t *testing.T) {
	assert.Equal(t, "Retur penjualan", domain.ScopeSalesReturn.Label())
	assert.True(t, domain.ScopeBoth.Covers(domain.ScopePurchaseReturn))
	assert.False(t, domain.ScopeSalesReturn.Covers(domain.ScopePurchaseReturn))
	assert.Equal(t, "Pemusnahan", domain.DispositionDispose.Label())
	assert.True(t, domain.DispositionMarketingSample.Valid())
	assert.False(t, domain.Disposition("x").Valid())
}
