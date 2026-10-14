package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/notification/domain"
	"balans/internal/platform/apperr"
)

func TestNewEmail(t *testing.T) {
	e, err := domain.NewEmail(" finance@findyourbalans.com ", " Persetujuan PO ", "PO menunggu persetujuan Anda.")
	require.NoError(t, err)
	assert.Equal(t, "finance@findyourbalans.com", e.To)
	assert.Equal(t, "Persetujuan PO", e.Subject)
}

func TestNewEmailValidation(t *testing.T) {
	tests := []struct {
		name, to, subject, text, field string
	}{
		{"alamat kosong", "", "S", "T", "to"},
		{"alamat tanpa domain", "finance", "S", "T", "to"},
		{"subjek kosong", "a@b.co", " ", "T", "subject"},
		{"isi kosong", "a@b.co", "S", "  ", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewEmail(tt.to, tt.subject, tt.text)
			require.Error(t, err)
			appErr := apperr.As(err)
			assert.Equal(t, apperr.CodeValidationFailed, appErr.Code)
			assert.Contains(t, appErr.Fields, tt.field)
		})
	}
}
