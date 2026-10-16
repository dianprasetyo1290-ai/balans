// Package audit adalah API publik modul audit trail (NFR-03). Modul lain mencatat perubahan lewat Record.
package audit

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit/domain"
	audithttp "balans/internal/audit/http"
	"balans/internal/audit/postgres"
	"balans/internal/audit/service"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "audit"

// PermissionRead adalah permission melihat seluruh audit trail.
const PermissionRead = service.PermissionRead

type (
	// Entry adalah perubahan yang dicatat.
	Entry = domain.Entry
	// Action adalah jenis aksi.
	Action = domain.Action
)

// Aksi audit trail.
const (
	ActionCreate       = domain.ActionCreate
	ActionUpdate       = domain.ActionUpdate
	ActionDelete       = domain.ActionDelete
	ActionRestore      = domain.ActionRestore
	ActionStatusChange = domain.ActionStatusChange
	ActionApprove      = domain.ActionApprove
	ActionReject       = domain.ActionReject
	ActionSubmit       = domain.ActionSubmit
	ActionPost         = domain.ActionPost
	ActionCancel       = domain.ActionCancel
	ActionLogin        = domain.ActionLogin
	ActionLoginFailed  = domain.ActionLoginFailed
	ActionLogout       = domain.ActionLogout
	ActionImport       = domain.ActionImport
	ActionExport       = domain.ActionExport
)

// Recorder dipakai modul lain untuk mencatat perubahan.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Responder *httpx.Responder
}

// Module adalah modul audit. Selalu dibuat karena dipakai semua modul; route hanya didaftarkan jika aktif.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	return &Module{svc: service.New(postgres.NewLogStore(d.Pool)), rs: d.Responder}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan GET /api/v1/audit-logs.
func (m *Module) RegisterRoutes(r chi.Router) {
	audithttp.Register(r, audithttp.NewHandler(m.svc), m.rs)
}

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// Record mencatat perubahan di transaksi aktif.
func (m *Module) Record(ctx context.Context, e Entry) error { return m.svc.Record(ctx, e) }

// RegisterEntity mendaftarkan permission baca entitas untuk tab Riwayat.
func (m *Module) RegisterEntity(entityType, readPermission string) {
	m.svc.RegisterEntity(entityType, readPermission)
}
