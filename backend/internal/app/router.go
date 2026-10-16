package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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
		r.Use(a.identity.Middleware())
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

// isIdempotencyExempt: webhook memakai event ID, halaman simulasi bukan API.
func isIdempotencyExempt(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/v1/webhooks/") || strings.HasPrefix(p, "/_mock/") || idempotencyExemptPaths[p]
}
