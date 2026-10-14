package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/httpx"
	"balans/internal/platform/logger"
)

func newResponder(buf *bytes.Buffer, reported *[]error) *httpx.Responder {
	return httpx.NewResponder(logger.New(buf, "info"), func(_ context.Context, err error) {
		*reported = append(*reported, err)
	})
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) httpx.Problem {
	t.Helper()
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p
}

func TestErrorWritesBusinessProblem(t *testing.T) {
	var logs bytes.Buffer
	var reported []error
	rs := newResponder(&logs, &reported)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/purchase-orders", nil)
	req = req.WithContext(logger.WithRequestID(req.Context(), "req-42"))
	rec := httptest.NewRecorder()
	rs.Error(rec, req, apperr.Validation(map[string]string{"supplierId": "Supplier wajib dipilih"}))

	p := decodeProblem(t, rec)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, apperr.CodeValidationFailed, p.Code)
	assert.Equal(t, "https://docs.balans.local/errors/validation-failed", p.Type)
	assert.Equal(t, "Supplier wajib dipilih", p.Fields["supplierId"])
	assert.Equal(t, "req-42", p.RequestID)
	assert.Empty(t, reported, "error 4xx tidak dikirim ke Sentry")
	assert.Empty(t, logs.String(), "error 4xx tidak dicatat sebagai error")
}

func TestErrorHidesInternalDetail(t *testing.T) {
	var logs bytes.Buffer
	var reported []error
	rs := newResponder(&logs, &reported)

	rec := httptest.NewRecorder()
	rs.Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("pq: relation \"users\" does not exist"))

	p := decodeProblem(t, rec)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, apperr.CodeInternal, p.Code)
	assert.NotContains(t, rec.Body.String(), "relation")
	assert.Len(t, reported, 1)
	assert.Contains(t, logs.String(), "relation", "detail internal tetap ada di log server")
}

func TestRequestIDMiddleware(t *testing.T) {
	var seen string
	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = logger.RequestID(r.Context())
	}))

	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"dari Caddy dipakai", "abc-123", true},
		{"kosong dibuat baru", "", false},
		{"tidak aman diganti", "abc\nInjected: 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(httpx.HeaderRequestID, tt.incoming)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.NotEmpty(t, seen)
			assert.Equal(t, seen, rec.Header().Get(httpx.HeaderRequestID))
			assert.Equal(t, tt.keep, seen == tt.incoming)
		})
	}
}

func TestRecoverTurnsPanicIntoInternalError(t *testing.T) {
	var logs bytes.Buffer
	var reported []error
	rs := newResponder(&logs, &reported)
	h := httpx.Recover(rs)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, apperr.CodeInternal, decodeProblem(t, rec).Code)
	assert.Len(t, reported, 1)
}

func TestAccessLogRecordsStatusWithoutQuery(t *testing.T) {
	var logs bytes.Buffer
	h := httpx.AccessLog(logger.New(&logs, "info"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/items?phone=08123", nil))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	assert.InDelta(t, 201, entry["status"], 0)
	assert.Equal(t, "/api/v1/items", entry["path"])
	assert.NotContains(t, logs.String(), "08123")
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	var logs bytes.Buffer
	var reported []error
	rs := newResponder(&logs, &reported)

	rec := httptest.NewRecorder()
	rs.NotFound(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = httptest.NewRecorder()
	rs.MethodNotAllowed(rec, httptest.NewRequest(http.MethodPatch, "/x", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	rs.RequestError(rec, httptest.NewRequest(http.MethodPost, "/x", nil), errors.New("invalid character"))
	assert.Equal(t, apperr.CodeBadRequest, decodeProblem(t, rec).Code)
}

func TestJSON(t *testing.T) {
	var logs bytes.Buffer
	var reported []error
	rs := newResponder(&logs, &reported)

	rec := httptest.NewRecorder()
	rs.JSON(rec, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusOK, map[string]string{"status": "ok"})

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}
