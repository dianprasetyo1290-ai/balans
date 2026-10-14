package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"balans/migrations"
)

// SchemaStatus adalah versi skema di DB dibanding migrasi terbaru di binary.
type SchemaStatus struct {
	Current int64
	Latest  int64
}

// UpToDate bernilai true jika seluruh migrasi sudah dijalankan.
func (s SchemaStatus) UpToDate() bool { return s.Current >= s.Latest }

// Migrator menjalankan migrasi goose milik aplikasi dan migrasi tabel river.
type Migrator struct {
	pool  *pgxpool.Pool
	sqlDB *sql.DB
	goose *goose.Provider
}

// NewMigrator menyiapkan migrator. Panggil Close setelah selesai.
func NewMigrator(pool *pgxpool.Pool) (*Migrator, error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("create goose provider: %w", err)
	}
	return &Migrator{pool: pool, sqlDB: sqlDB, goose: provider}, nil
}

// Close menutup koneksi database/sql. Pool tetap terbuka.
func (m *Migrator) Close() error { return m.sqlDB.Close() }

// Up menjalankan migrasi river lalu migrasi aplikasi.
func (m *Migrator) Up(ctx context.Context) error {
	riverMigrator, err := rivermigrate.New(riverpgxv5.New(m.pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	if _, err := riverMigrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrate river: %w", err)
	}
	if _, err := m.goose.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// DownOne membatalkan satu migrasi aplikasi. Hanya untuk development lokal (Technical Guideline 14.4).
func (m *Migrator) DownOne(ctx context.Context) error {
	if _, err := m.goose.Down(ctx); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// Status membandingkan versi DB dengan migrasi terbaru.
func (m *Migrator) Status(ctx context.Context) (SchemaStatus, error) {
	current, err := m.goose.GetDBVersion(ctx)
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("get db version: %w", err)
	}
	sources := m.goose.ListSources()
	var latest int64
	if len(sources) > 0 {
		latest = sources[len(sources)-1].Version
	}
	return SchemaStatus{Current: current, Latest: latest}, nil
}
