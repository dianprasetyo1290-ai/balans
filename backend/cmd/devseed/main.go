// Command devseed mengisi data pengembangan lokal: satu akun per role dengan password yang sama.
// Hanya berjalan di APP_ENV=local.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"balans/internal/app"
	"balans/internal/platform/config"
)

// defaultPassword dipakai semua akun seed; bisa diganti lewat DEV_SEED_PASSWORD.
const defaultPassword = "Balans-Lokal-2026" //nolint:gosec // password khusus APP_ENV=local, bukan kredensial production

func main() {
	app.Main("devseed", func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error {
		a, err := app.New(ctx, cfg, log, report)
		if err != nil {
			return err
		}
		defer a.Close()
		password := os.Getenv("DEV_SEED_PASSWORD")
		if password == "" {
			password = defaultPassword
		}
		users, err := a.SeedDev(ctx, password)
		if err != nil {
			return err
		}
		fmt.Printf("Akun pengembangan (password semua: %s)\n", password)
		for _, u := range users {
			fmt.Printf("  %-34s %s\n", u.Email, u.FullName)
		}
		return nil
	})
}
