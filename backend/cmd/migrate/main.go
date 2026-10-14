// Command migrate menjalankan migrasi skema sebelum container aplikasi diganti (Technical Guideline 14.4).
//
//	migrate up      menjalankan semua migrasi
//	migrate status  menampilkan versi skema
//	migrate down    membatalkan satu migrasi (hanya local)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"balans/internal/app"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
)

func main() {
	app.Main("migrate", func(ctx context.Context, cfg config.Config, log *slog.Logger, _ func(context.Context, error)) error {
		command := "up"
		if len(os.Args) > 1 {
			command = os.Args[1]
		}
		pool, err := db.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		m, err := db.NewMigrator(pool)
		if err != nil {
			return err
		}
		defer m.Close() //nolint:errcheck // hanya menutup pembungkus database/sql

		switch command {
		case "up":
			if err := m.Up(ctx); err != nil {
				return err
			}
		case "down":
			if cfg.Env != config.EnvLocal {
				return errors.New("migrate down hanya untuk local; perbaikan di staging/production lewat migrasi baru")
			}
			if err := m.DownOne(ctx); err != nil {
				return err
			}
		case "status":
		default:
			return fmt.Errorf("perintah %q tidak dikenal (up, status, down)", command)
		}
		status, err := m.Status(ctx)
		if err != nil {
			return err
		}
		log.Info("schema version", "current", status.Current, "latest", status.Latest)
		return nil
	})
}
