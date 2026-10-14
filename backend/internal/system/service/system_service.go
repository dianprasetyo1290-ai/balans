// Package service berisi use case modul system.
package service

import (
	"context"
	"time"

	"balans/internal/platform/clock"
	"balans/internal/platform/db"
)

type schemaChecker interface {
	Status(ctx context.Context) (db.SchemaStatus, error)
}

type pinger interface {
	Ping(ctx context.Context) error
}

// Build adalah identitas build dan lingkungan proses.
type Build struct {
	Version        string
	Commit         string
	Environment    string
	EnabledModules []string
	MockActive     bool
}

// DatabaseInfo adalah status database.
type DatabaseInfo struct {
	OK                  bool
	SchemaVersion       int64
	LatestSchemaVersion int64
}

// Info adalah informasi sistem untuk halaman Tentang Sistem.
type Info struct {
	Build      Build
	ServerTime time.Time
	Database   DatabaseInfo
}

// Check adalah hasil satu pemeriksaan kesiapan.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Readiness adalah hasil pemeriksaan kesiapan proses.
type Readiness struct {
	Ready  bool
	Checks []Check
}

// SystemService menyusun informasi sistem.
type SystemService struct {
	build  Build
	db     pinger
	schema schemaChecker
	clock  clock.Clock
}

// NewSystemService membuat service.
func NewSystemService(build Build, db pinger, schema schemaChecker, c clock.Clock) *SystemService {
	return &SystemService{build: build, db: db, schema: schema, clock: c}
}

const checkTimeout = 2 * time.Second

func (s *SystemService) database(ctx context.Context) (DatabaseInfo, []Check) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		return DatabaseInfo{}, []Check{{Name: "database", Detail: "tidak terhubung"}}
	}
	status, err := s.schema.Status(ctx)
	if err != nil {
		return DatabaseInfo{OK: true}, []Check{{Name: "database", OK: true}, {Name: "migration", Detail: "versi skema tidak terbaca"}}
	}
	info := DatabaseInfo{OK: true, SchemaVersion: status.Current, LatestSchemaVersion: status.Latest}
	migration := Check{Name: "migration", OK: status.UpToDate()}
	if !migration.OK {
		migration.Detail = "migrasi belum dijalankan"
	}
	return info, []Check{{Name: "database", OK: true}, migration}
}

// Info mengembalikan versi, lingkungan, modul aktif, dan status database.
func (s *SystemService) Info(ctx context.Context) Info {
	dbInfo, _ := s.database(ctx)
	return Info{Build: s.build, ServerTime: s.clock.Now(), Database: dbInfo}
}

// Readiness memeriksa database terhubung dan migrasi terbaru (Technical Guideline 13).
func (s *SystemService) Readiness(ctx context.Context) Readiness {
	_, checks := s.database(ctx)
	ready := true
	for _, c := range checks {
		ready = ready && c.OK
	}
	return Readiness{Ready: ready, Checks: checks}
}
