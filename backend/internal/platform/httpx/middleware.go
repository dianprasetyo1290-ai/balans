package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/logger"
)

// HeaderRequestID diteruskan dari Caddy dan dikembalikan ke client (Technical Guideline 13).
const HeaderRequestID = "X-Request-ID"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID memakai X-Request-ID dari proxy jika formatnya aman, jika tidak membuat UUID v7 baru.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = uuid.Must(uuid.NewV7()).String()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(logger.WithRequestID(r.Context(), id)))
	})
}

// statusRecorder mencatat status response untuk access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog mencatat setiap request. Query string tidak dicatat karena bisa memuat data pribadi.
func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			level := slog.LevelInfo
			if rec.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			log.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// Recover mengubah panic menjadi INTERNAL_ERROR tanpa menjatuhkan proses.
func Recover(rs *Responder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // nilai sentinel dari net/http
						panic(v)
					}
					rs.Error(w, r, apperr.Wrap(apperr.CodeInternal, fmt.Errorf("panic: %v", v)))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout memberi setiap request context dengan batas waktu (Technical Guideline 7.2).
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
