package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"balans/internal/document/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name   string
		t      domain.Type
		date   clock.BusinessDate
		number int64
		want   string
	}{
		{"contoh desain jurnal", domain.TypeJournal, clock.NewBusinessDate(2027, time.January, 15), 102, "JV/2027/01/00102"},
		{"invoice bulan dua digit", domain.TypeInvoice, clock.NewBusinessDate(2027, time.May, 1), 12, "INV/2027/05/00012"},
		{"nomor pertama", domain.TypePurchaseOrder, clock.NewBusinessDate(2026, time.October, 31), 1, "PO/2026/10/00001"},
		{"lebih dari lima digit tidak dipotong", domain.TypeSalesOrder, clock.NewBusinessDate(2026, time.December, 31), 123456, "SO/2026/12/123456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, domain.Format(tt.t, tt.date, tt.number))
		})
	}
}

func TestTypes(t *testing.T) {
	seen := map[domain.Type]bool{}
	for _, info := range domain.Types() {
		assert.False(t, seen[info.Type], "kode ganda %s", info.Type)
		seen[info.Type] = true
		assert.NotEmpty(t, info.Label)
		assert.NoError(t, info.Type.Validate())
	}
	// Kode wajib Technical Guideline 5.12.
	for _, code := range []domain.Type{"PR", "PO", "GR", "DO", "PI", "INV", "CN", "DN", "RJ", "RB", "TRF", "SC", "KSN", "EXP", "KK", "JV"} {
		info, ok := domain.Lookup(code)
		assert.True(t, ok, code)
		assert.Equal(t, code, info.Type)
	}
	_, ok := domain.Lookup("XX")
	assert.False(t, ok)
	assert.True(t, apperr.HasCode(domain.Type("XX").Validate(), apperr.CodeValidationFailed))
}
