// Package dbtest menyediakan Postgres asli lewat testcontainers untuk test integrasi (Technical Guideline 12).
package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"balans/internal/platform/db"
)

// PostgresImage harus sama dengan versi di deploy/compose.yaml.
const PostgresImage = "postgres:18-alpine"

var (
	once      sync.Once
	sharedURL string
	setupErr  error
	container *tcpostgres.PostgresContainer
)

// Main dipanggil dari TestMain agar container dihentikan setelah semua test di package selesai,
// termasuk saat Ryuk dinonaktifkan (TESTCONTAINERS_RYUK_DISABLED=true di Docker Desktop Windows).
func Main(m *testing.M) int {
	code := m.Run()
	if container != nil {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", err)
		}
	}
	return code
}

// Pool mengembalikan pool ke database yang sudah dimigrasi. Satu container dipakai bersama
// per proses test; setiap test membersihkan datanya sendiri atau memakai data unik.
// Test dilewati jika Docker tidak tersedia dan BALANS_REQUIRE_DB tidak diset.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	once.Do(func() { sharedURL, setupErr = start() })
	if setupErr != nil {
		if os.Getenv("BALANS_REQUIRE_DB") != "" {
			t.Fatalf("start postgres: %v", setupErr)
		}
		t.Skipf("postgres tidak tersedia: %v", setupErr)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, sharedURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func start() (string, error) {
	if url := os.Getenv("BALANS_TEST_DATABASE_URL"); url != "" {
		return url, migrate(url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var err error
	container, err = tcpostgres.Run(ctx, PostgresImage,
		tcpostgres.WithDatabase("balans_test"),
		tcpostgres.WithUsername("balans"),
		tcpostgres.WithPassword("balans"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return "", fmt.Errorf("run container: %w", err)
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("connection string: %w", err)
	}
	return url, migrate(url)
}

func migrate(url string) error {
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	m, err := db.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck // hanya menutup *sql.DB pembungkus pool
	return m.Up(ctx)
}
