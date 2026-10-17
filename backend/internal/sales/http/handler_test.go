package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/httpx"
	saleshttp "balans/internal/sales/http"
	"balans/internal/sales/service"
)

type receiver struct{ body string }

func (r *receiver) Receive(_ context.Context, body []byte) (bool, error) {
	r.body = string(body)
	return false, nil
}

// Webhook Midtrans tetap ditangani WebhookHandler (body asli) walau modul memakai strict server.
func TestRegisterRoutesWebhookToRawHandler(t *testing.T) {
	rs := httpx.NewResponder(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	rec := &receiver{}
	r := chi.NewRouter()
	saleshttp.Register(r, saleshttp.NewHandler(service.NewReturnReasonService(nil, nil, nil), nil), saleshttp.NewWebhookHandler(rec, rs), rs)

	body := `{"order_id":"SO-1",  "status_code":"200"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/midtrans", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"received":true,"duplicate":false}`, w.Body.String())
	assert.Equal(t, body, rec.body, "body tidak di-decode ulang")

	// Endpoint master tanpa actor ditolak oleh service (bukan 404).
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/return-reasons", nil))
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}
