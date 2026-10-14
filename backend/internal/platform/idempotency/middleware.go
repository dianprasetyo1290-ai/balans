package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// HeaderKey adalah header yang dikirim FE per percobaan submit.
const HeaderKey = "Idempotency-Key"

// HeaderReplayed menandai response yang diambil dari key yang sudah selesai.
const HeaderReplayed = "Idempotency-Replayed"

// DefaultTTL adalah masa simpan key sebelum dihapus job harian.
const DefaultTTL = 24 * time.Hour

const maxBodyBytes = 10 << 20

type keyStore interface {
	Claim(ctx context.Context, rec Record) (Record, bool, error)
	Complete(ctx context.Context, userID, key uuid.UUID, code int, body []byte) error
	Release(ctx context.Context, userID, key uuid.UUID) error
}

type errorWriter interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
}

// Options mengatur middleware.
type Options struct {
	Store     keyStore
	Clock     clock.Clock
	Responder errorWriter
	Log       *slog.Logger
	TTL       time.Duration
	// UserID membaca user yang login dari context. Request anonim memakai uuid.Nil.
	UserID func(ctx context.Context) uuid.UUID
	// Exempt menandai request tanpa Idempotency-Key: login, refresh token, dan webhook.
	Exempt func(r *http.Request) bool
}

// Middleware menolak request yang mengubah data tanpa Idempotency-Key, mengirim ulang response
// yang tersimpan untuk key yang sama, dan menolak key yang dipakai untuk request berbeda.
func Middleware(opts Options) func(http.Handler) http.Handler {
	if opts.TTL == 0 {
		opts.TTL = DefaultTTL
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutation(r.Method) || (opts.Exempt != nil && opts.Exempt(r)) {
				next.ServeHTTP(w, r)
				return
			}
			key, err := uuid.Parse(r.Header.Get(HeaderKey))
			if err != nil {
				opts.Responder.Error(w, r, apperr.Wrap(apperr.CodeIdempotencyKeyRequired, fmt.Errorf("parse %s: %w", HeaderKey, err)))
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
			if err != nil {
				opts.Responder.Error(w, r, apperr.Wrap(apperr.CodeBadRequest, err))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			ctx := r.Context()
			userID := uuid.Nil
			if opts.UserID != nil {
				userID = opts.UserID(ctx)
			}
			hash := requestHash(r.Method, r.URL.Path, body)
			existing, claimed, err := opts.Store.Claim(ctx, Record{
				UserID:      userID,
				Key:         key,
				Method:      r.Method,
				Path:        r.URL.Path,
				RequestHash: hash,
				ExpiresAt:   opts.Clock.Now().Add(opts.TTL),
			})
			if err != nil {
				opts.Responder.Error(w, r, err)
				return
			}
			if !claimed {
				replay(w, r, opts.Responder, existing, hash)
				return
			}

			rec := &bufferedResponse{header: http.Header{}}
			next.ServeHTTP(rec, r)
			finish(ctx, opts, userID, key, rec)
			rec.flushTo(w)
		})
	}
}

func finish(ctx context.Context, opts Options, userID, key uuid.UUID, rec *bufferedResponse) {
	// Simpan hasil walaupun client sudah memutus koneksi, supaya percobaan berikutnya mendapat jawaban yang sama.
	ctx = context.WithoutCancel(ctx)
	status := rec.statusCode()
	if status >= http.StatusInternalServerError {
		if err := opts.Store.Release(ctx, userID, key); err != nil && opts.Log != nil {
			opts.Log.ErrorContext(ctx, "release idempotency key", "error", err)
		}
		return
	}
	body := rec.body.Bytes()
	if !json.Valid(body) {
		body = nil
	}
	if err := opts.Store.Complete(ctx, userID, key, status, body); err != nil && opts.Log != nil {
		opts.Log.ErrorContext(ctx, "complete idempotency key", "error", err)
	}
}

func replay(w http.ResponseWriter, r *http.Request, rs errorWriter, existing Record, hash []byte) {
	switch {
	case subtle.ConstantTimeCompare(existing.RequestHash, hash) != 1:
		rs.Error(w, r, apperr.New(apperr.CodeIdempotencyKeyReused))
	case existing.Status != StatusCompleted:
		rs.Error(w, r, apperr.New(apperr.CodeIdempotencyInProgress))
	default:
		contentType := "application/json"
		if existing.ResponseCode >= http.StatusBadRequest {
			contentType = "application/problem+json"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set(HeaderReplayed, "true")
		w.WriteHeader(existing.ResponseCode)
		_, _ = w.Write(existing.ResponseBody) //nolint:errcheck // client yang terputus tidak perlu ditangani
	}
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestHash(method, path string, body []byte) []byte {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(body)
	return h.Sum(nil)
}

// bufferedResponse menahan response sampai hasilnya tersimpan.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

func (b *bufferedResponse) statusCode() int {
	if b.status == 0 {
		return http.StatusOK
	}
	return b.status
}

func (b *bufferedResponse) flushTo(w http.ResponseWriter) {
	for k, v := range b.header {
		w.Header()[k] = v
	}
	w.WriteHeader(b.statusCode())
	_, _ = w.Write(b.body.Bytes()) //nolint:errcheck // client yang terputus tidak perlu ditangani
}
