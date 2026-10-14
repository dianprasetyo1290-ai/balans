package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
		idempotency.Middleware(idempotency.Options{
			Store:     a.idempotency,
			Clock:     a.Clock,
			Responder: a.Responder,
			Log:       a.Log,
			// User diisi middleware otentikasi di R1.1; sebelum itu semua request anonim.
			UserID: func(context.Context) uuid.UUID { return uuid.Nil },
			Exempt: isIdempotencyExempt,
		}),
	)

	a.system.RegisterRoutes(r)
	for _, m := range a.modules {
		m.RegisterRoutes(r)
	}
	if a.console != nil {
		r.Route("/_mock", a.console.Routes)
	}
	return r
}

// isIdempotencyExempt: webhook memakai event ID, halaman simulasi bukan API, login dan refresh
// token dikecualikan saat modul identity dibangun (Technical Guideline 7.1).
func isIdempotencyExempt(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/v1/webhooks/") ||
		strings.HasPrefix(p, "/_mock/") ||
		p == "/api/v1/auth/login" ||
		p == "/api/v1/auth/refresh"
}
