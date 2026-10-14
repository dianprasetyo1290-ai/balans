// Package system adalah modul kesehatan proses dan informasi sistem. Modul ini selalu aktif.
package system

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	systemhttp "balans/internal/system/http"
	"balans/internal/system/service"
)

// Build adalah identitas build dan lingkungan proses.
type Build = service.Build

type schemaChecker interface {
	Status(ctx context.Context) (db.SchemaStatus, error)
}

// Deps adalah dependensi modul.
type Deps struct {
	Build     Build
	Pool      *pgxpool.Pool
	Schema    schemaChecker
	Clock     clock.Clock
	Responder *httpx.Responder
}

// Module adalah modul system.
type Module struct {
	handler *systemhttp.SystemHandler
	rs      *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.NewSystemService(d.Build, d.Pool, d.Schema, d.Clock)
	return &Module{handler: systemhttp.NewSystemHandler(svc), rs: d.Responder}
}

// RegisterRoutes mendaftarkan /healthz, /readyz, dan /api/v1/system/info.
func (m *Module) RegisterRoutes(r chi.Router) {
	strict := systemhttp.NewStrictHandlerWithOptions(m.handler, nil, systemhttp.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  m.rs.RequestError,
		ResponseErrorHandlerFunc: m.rs.Error,
	})
	systemhttp.HandlerWithOptions(strict, systemhttp.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: m.rs.RequestError,
	})
}
