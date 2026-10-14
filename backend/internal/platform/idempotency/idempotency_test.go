package idempotency_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/logger"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type harness struct {
	handler http.Handler
	calls   *atomic.Int32
	status  *atomic.Int32
	store   *idempotency.Store
	clock   *clock.Fixed
}

func newHarness(t *testing.T) harness {
	t.Helper()
	pool := dbtest.Pool(t)
	store := idempotency.NewStore(pool)
	log := logger.New(&bytes.Buffer{}, "error")
	c := clock.NewFixed(time.Date(2026, 10, 14, 1, 0, 0, 0, time.UTC))
	calls := &atomic.Int32{}
	status := &atomic.Int32{}
	status.Store(http.StatusCreated)

	business := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(status.Load()))
		_ = json.NewEncoder(w).Encode(map[string]any{"call": n})
	})
	mw := idempotency.Middleware(idempotency.Options{
		Store:     store,
		Clock:     c,
		Responder: httpx.NewResponder(log, nil),
		Log:       log,
		Exempt:    func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/api/v1/webhooks/") },
	})
	return harness{handler: mw(business), calls: calls, status: status, store: store, clock: c}
}

func (h harness) do(method, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set(idempotency.HeaderKey, key)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) apperr.Code {
	t.Helper()
	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p.Code
}

func TestMutationWithoutKeyIsRejected(t *testing.T) {
	h := newHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/products", "", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, apperr.CodeIdempotencyKeyRequired, problemCode(t, rec))
	assert.Zero(t, h.calls.Load())
}

func TestReadAndExemptRequestsPassThrough(t *testing.T) {
	h := newHarness(t)

	assert.Equal(t, http.StatusCreated, h.do(http.MethodGet, "/api/v1/products", "", "").Code)
	assert.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/webhooks/midtrans", "", `{}`).Code)
	assert.Equal(t, int32(2), h.calls.Load())
}

func TestSameKeyReplaysStoredResponse(t *testing.T) {
	h := newHarness(t)
	key := uuid.NewString()

	first := h.do(http.MethodPost, "/api/v1/products", key, `{"name":"A"}`)
	second := h.do(http.MethodPost, "/api/v1/products", key, `{"name":"A"}`)

	assert.Equal(t, http.StatusCreated, first.Code)
	assert.Equal(t, http.StatusCreated, second.Code)
	assert.JSONEq(t, first.Body.String(), second.Body.String())
	assert.Equal(t, "true", second.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, int32(1), h.calls.Load(), "proses bisnis hanya sekali")
}

func TestSameKeyDifferentRequestIsRejected(t *testing.T) {
	h := newHarness(t)
	key := uuid.NewString()

	h.do(http.MethodPost, "/api/v1/products", key, `{"name":"A"}`)
	rec := h.do(http.MethodPost, "/api/v1/products", key, `{"name":"B"}`)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, apperr.CodeIdempotencyKeyReused, problemCode(t, rec))
}

func TestBusinessErrorIsFinal(t *testing.T) {
	h := newHarness(t)
	h.status.Store(http.StatusUnprocessableEntity)
	key := uuid.NewString()

	h.do(http.MethodPost, "/api/v1/products", key, `{}`)
	h.status.Store(http.StatusCreated)
	rec := h.do(http.MethodPost, "/api/v1/products", key, `{}`)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Equal(t, int32(1), h.calls.Load())
}

func TestServerErrorReleasesKey(t *testing.T) {
	h := newHarness(t)
	h.status.Store(http.StatusInternalServerError)
	key := uuid.NewString()

	h.do(http.MethodPost, "/api/v1/products", key, `{}`)
	h.status.Store(http.StatusCreated)
	rec := h.do(http.MethodPost, "/api/v1/products", key, `{}`)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, int32(2), h.calls.Load(), "request boleh diulang setelah 5xx")
}

func TestConcurrentSameKeyRunsBusinessOnce(t *testing.T) {
	h := newHarness(t)
	key := uuid.NewString()
	var wg sync.WaitGroup
	codes := make([]int, 10)

	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = h.do(http.MethodPost, "/api/v1/products", key, `{"name":"A"}`).Code
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), h.calls.Load())
	for _, code := range codes {
		assert.Contains(t, []int{http.StatusCreated, http.StatusConflict}, code)
	}
}

func TestDeleteExpired(t *testing.T) {
	h := newHarness(t)
	key := uuid.NewString()
	h.do(http.MethodPost, "/api/v1/products", key, `{}`)

	ctx := context.Background()
	deleted, err := h.store.DeleteExpired(ctx, h.clock.Now().Add(idempotency.DefaultTTL+time.Minute))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(1))

	rec := h.do(http.MethodPost, "/api/v1/products", key, `{}`)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Empty(t, rec.Header().Get(idempotency.HeaderReplayed))
}
