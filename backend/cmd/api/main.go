// Command api melayani API back-office, form partner, self-service karyawan, dan webhook.
package main

import (
	"context"
	"log/slog"

	"balans/internal/app"
	"balans/internal/platform/config"
)

func main() {
	app.Main("api", func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error {
		a, err := app.New(ctx, cfg, log, report)
		if err != nil {
			return err
		}
		defer a.Close()
		return app.Serve(ctx, log, cfg.HTTPAddr, a.Router(), cfg.ShutdownTimeout)
	})
}
