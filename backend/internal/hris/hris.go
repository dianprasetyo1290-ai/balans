// Package hris adalah API publik data karyawan dasar Fase 1 (BRD HR-01, HR-03). Modul lain memeriksa status
// karyawan lewat IsActiveEmployee, mis. pricing untuk harga karyawan pelanggan yang tertaut.
package hris

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/hris/domain"
	hrishttp "balans/internal/hris/http"
	"balans/internal/hris/postgres"
	"balans/internal/hris/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/tabular"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "hris"

type (
	// Employee adalah data dasar karyawan (NIK terdekripsi; tampilkan tersamar).
	Employee = domain.Employee
	// Cipher mengenkripsi NIK dan menghitung hash pencarian.
	Cipher = postgres.Cipher
)

// AuditRegistrar mendaftarkan permission baca entitas untuk tab Riwayat (modul audit).
type AuditRegistrar interface {
	RegisterEntity(entityType, readPermission string)
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Cipher    Cipher
}

// Module adalah modul hris.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool, d.Cipher), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log,
	})
	return &Module{svc: svc, rs: d.Responder}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route karyawan.
func (m *Module) RegisterRoutes(r chi.Router) { hrishttp.Register(r, hrishttp.NewHandler(m.svc), m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// RegisterAuditEntities mendaftarkan permission baca karyawan untuk tab Riwayat.
func (m *Module) RegisterAuditEntities(a AuditRegistrar) {
	a.RegisterEntity(service.EntityEmployee, service.PermEmployeeRead)
}

// IsActiveEmployee memeriksa apakah karyawan aktif pada tanggal (sudah bergabung dan belum mencapai tanggal
// resign). Karyawan yang tidak ada dianggap tidak aktif. Tanpa cek permission.
func (m *Module) IsActiveEmployee(ctx context.Context, id uuid.UUID, on clock.BusinessDate) (bool, error) {
	return m.svc.IsActiveEmployee(ctx, id, on)
}

// EmployeesByID mengembalikan karyawan (termasuk yang terhapus) untuk ditampilkan modul lain.
func (m *Module) EmployeesByID(ctx context.Context, ids []uuid.UUID) ([]Employee, error) {
	return m.svc.ByIDs(ctx, ids)
}

// DivisionRef adalah divisi dari master dimensi accounting.
type DivisionRef = service.DivisionRef

// Divisions membaca master divisi untuk impor/ekspor karyawan.
type Divisions = service.Divisions

// SetDivisions memasang pembaca divisi (modul accounting) saat wiring.
func (m *Module) SetDivisions(d Divisions) { m.svc.Divisions = d }

// EmployeeColumns adalah kolom template impor/ekspor karyawan (S-125).
func EmployeeColumns() []tabular.Column { return service.EmployeeColumns }

// ImportEmployeeRow membuat atau memperbarui karyawan (kunci nomor karyawan) dari satu baris Excel.
func (m *Module) ImportEmployeeRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportEmployeeRow(ctx, row)
}

// ExportEmployees mengalirkan karyawan sebagai baris Excel (NIK tersamar).
func (m *Module) ExportEmployees(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportEmployees(ctx, emit)
}

// EmployeeIDByNumber mencari karyawan dari nomor karyawan tanpa cek permission (impor pelanggan).
func (m *Module) EmployeeIDByNumber(ctx context.Context, number string) (uuid.UUID, bool, error) {
	return m.svc.EmployeeIDByNumber(ctx, number)
}
