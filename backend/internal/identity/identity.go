// Package identity adalah API publik modul identitas: login, sesi, MFA, pengguna, role, dan permission
// (NFR-02, Technical Guideline 9). Modul lain hanya memakai package ini.
package identity

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/identity/domain"
	identityhttp "balans/internal/identity/http"
	"balans/internal/identity/postgres"
	"balans/internal/identity/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "identity"

// KeyConfig berisi kunci JWT dari env.
type KeyConfig = service.KeyConfig

// Cipher mengenkripsi data pribadi.
type Cipher = service.Cipher

// Mailer mengirim email transaksional.
type Mailer = service.Mailer

// User adalah pengguna ERP.
type User = domain.User

// Kode role yang dipakai modul lain (mis. approver per jabatan).
const (
	RoleHeadFinance       = domain.RoleHeadFinance
	RoleHeadLogistics     = domain.RoleHeadLogistics
	RoleHeadSales         = domain.RoleHeadSales
	RoleSalesMT           = domain.RoleSalesMT
	RoleDivisionHead      = domain.RoleDivisionHead
	RoleDirector          = domain.RoleDirector
	RolePresidentDirector = domain.RolePresidentDirector
)

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Mailer    Mailer
	Cipher    Cipher
	// Keys kosong hanya untuk APP_ENV=local; memakai kunci pengembangan deterministik.
	Keys         KeyConfig
	BaseURL      string
	SecureCookie bool
	SkipMFA      bool
}

// Module adalah modul identity.
type Module struct {
	auth  *service.AuthService
	admin *service.AdminService
	rs    *httpx.Responder
	h     *identityhttp.IdentityHandler
}

// New menyusun modul.
func New(d Deps) (*Module, error) {
	var tokens *service.TokenIssuer
	if d.Keys.SigningKey == "" {
		tokens = service.NewDevTokenIssuer(d.Clock)
	} else {
		var err error
		if tokens, err = service.NewTokenIssuer(d.Keys, d.Clock); err != nil {
			return nil, err
		}
	}
	auth := service.NewAuthService(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Tokens: tokens, Cipher: d.Cipher, Mailer: d.Mailer,
		Audit: d.Audit, Clock: d.Clock, Log: d.Log, BaseURL: d.BaseURL, SkipMFA: d.SkipMFA,
	})
	admin := service.NewAdminService(auth)
	return &Module{auth: auth, admin: admin, rs: d.Responder, h: identityhttp.NewHandler(auth, admin, d.SecureCookie)}, nil
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route sesi, pengguna, dan role.
func (m *Module) RegisterRoutes(r chi.Router) { identityhttp.Register(r, m.h, m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// Middleware memasang actor di context untuk semua request /api/v1 yang terlindungi.
func (m *Module) Middleware() func(http.Handler) http.Handler {
	return identityhttp.Middleware(m.auth, m.rs)
}

// Start menyemai katalog permission dan mengundang Admin Sistem pertama jika belum ada user.
func (m *Module) Start(ctx context.Context, initialAdminEmail string) error {
	if err := m.admin.SyncCatalog(ctx); err != nil {
		return fmt.Errorf("sync permission catalog: %w", err)
	}
	if err := m.admin.BootstrapAdmin(ctx, initialAdminEmail); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	return nil
}

// SeedDevUsers membuat satu user per role dengan password yang sama. Hanya dipanggil cmd/devseed di APP_ENV=local.
func (m *Module) SeedDevUsers(ctx context.Context, password string) ([]DevUser, error) {
	return m.admin.SeedDevUsers(ctx, password)
}

// DevUser adalah user hasil seed pengembangan.
type DevUser = service.DevUser

// UsersWithRole mengembalikan user aktif dengan role tertentu; division membatasi role ber-scope divisi.
func (m *Module) UsersWithRole(ctx context.Context, roleCode string, division *uuid.UUID) ([]User, error) {
	return m.admin.UsersWithRole(ctx, roleCode, division)
}

// UserByID mengembalikan user.
func (m *Module) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	return m.admin.UserByID(ctx, id)
}
