package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"balans/internal/filestore"
	"balans/internal/platform/actor"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
)

const requestTimeout = 30 * time.Second

// Router menyusun handler HTTP proses api.
func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.NotFound(a.Responder.NotFound)
	r.MethodNotAllowed(a.Responder.MethodNotAllowed)
	r.Use(
		httpx.RequestID,
		httpx.AccessLog(a.Log),
		httpx.Recover(a.Responder),
		httpx.Timeout(requestTimeout),
	)
	if a.identity != nil {
		r.Use(unless(filestore.IsContentPath, a.identity.Middleware()))
	}
	r.Use(idempotency.Middleware(idempotency.Options{
		Store:     a.idempotency,
		Clock:     a.Clock,
		Responder: a.Responder,
		Log:       a.Log,
		UserID:    actor.UserID,
		Exempt:    isIdempotencyExempt,
	}))

	a.system.RegisterRoutes(r)
	for _, m := range a.modules {
		m.RegisterRoutes(r)
	}
	if a.console != nil {
		r.Route("/_mock", a.console.Routes)
	}
	return r
}

// idempotencyExemptPaths: endpoint sesi dan password tidak memakai Idempotency-Key (Technical Guideline 7.1).
var idempotencyExemptPaths = map[string]bool{
	"/api/v1/auth/login":           true,
	"/api/v1/auth/refresh":         true,
	"/api/v1/auth/logout":          true,
	"/api/v1/auth/reauth":          true,
	"/api/v1/auth/password/forgot": true,
	"/api/v1/auth/password/setup":  true,
}

// isIdempotencyExempt: webhook memakai event ID, halaman simulasi bukan API, dan URL isi file bertanda tangan
// setara presigned URL object storage (Technical Guideline 5.16).
func isIdempotencyExempt(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/v1/webhooks/") || strings.HasPrefix(p, "/_mock/") || idempotencyExemptPaths[p] ||
		filestore.IsContentPath(p)
}

// unless melewati middleware untuk path tertentu, mis. URL isi file bertanda tangan yang dibuka browser tanpa
// access token.
func unless(skip func(path string) bool, mw func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}
