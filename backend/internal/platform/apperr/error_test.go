package apperr_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
)

func TestCodeSpecs(t *testing.T) {
	tests := []struct {
		code   apperr.Code
		status int
	}{
		{apperr.CodeValidationFailed, http.StatusUnprocessableEntity},
		{apperr.CodeAuthInvalidCredentials, http.StatusUnauthorized},
		{apperr.CodeAuthAccountLocked, http.StatusLocked},
		{apperr.CodeForbidden, http.StatusForbidden},
		{apperr.CodeNotFound, http.StatusNotFound},
		{apperr.CodeVersionConflict, http.StatusConflict},
		{apperr.CodeIdempotencyKeyRequired, http.StatusBadRequest},
		{apperr.CodeIdempotencyKeyReused, http.StatusUnprocessableEntity},
		{apperr.CodeRateLimited, http.StatusTooManyRequests},
		{apperr.CodeProviderUnavailable, http.StatusServiceUnavailable},
		{apperr.CodeInternal, http.StatusInternalServerError},
		{apperr.Code("UNKNOWN"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			assert.Equal(t, tt.status, tt.code.HTTPStatus())
			assert.NotEmpty(t, tt.code.Title())
			assert.NotEmpty(t, tt.code.DefaultMessage())
		})
	}
}

func TestEveryCodeHasIndonesianText(t *testing.T) {
	for _, code := range apperr.Codes() {
		assert.NotEmpty(t, code.Title(), code)
		assert.NotEmpty(t, code.DefaultMessage(), code)
	}
}

func TestAsKeepsAppErrorThroughWrapping(t *testing.T) {
	original := apperr.New(apperr.CodePeriodLocked)
	wrapped := fmt.Errorf("post journal: %w", original)

	got := apperr.As(wrapped)

	assert.Same(t, original, got)
	assert.True(t, apperr.HasCode(wrapped, apperr.CodePeriodLocked))
}

func TestAsTreatsUnknownErrorAsInternal(t *testing.T) {
	cause := errors.New("connection reset")

	got := apperr.As(cause)

	assert.Equal(t, apperr.CodeInternal, got.Code)
	require.ErrorIs(t, got, cause)
	assert.False(t, apperr.HasCode(cause, apperr.CodeInternal))
}

func TestConstructors(t *testing.T) {
	v := apperr.Validation(map[string]string{"supplierId": "Supplier wajib dipilih"})
	assert.Equal(t, apperr.CodeValidationFailed, v.Code)
	assert.Equal(t, "Supplier wajib dipilih", v.Fields["supplierId"])
	assert.Contains(t, v.Error(), "VALIDATION_FAILED")

	m := apperr.WithMessage(apperr.CodeStockInsufficient, "Stok A di Gudang Utama tidak cukup. Tersedia 3.")
	assert.Equal(t, "Stok A di Gudang Utama tidak cukup. Tersedia 3.", m.Message)

	s := apperr.InvalidStateTransition("purchase_order", "draft", "received")
	assert.Equal(t, apperr.CodeInvalidStateTransition, s.Code)
	assert.Contains(t, s.Error(), "draft -> received")
}
