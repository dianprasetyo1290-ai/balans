// Command worker menjalankan background job dan jadwal berkala (Technical Guideline 7.3, Lampiran B).
package main

import (
	"context"
	"fmt"
	"log/slog"

	"balans/internal/app"
	"balans/internal/platform/config"
)

func main() {
	app.Main("worker", func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error {
		a, err := app.New(ctx, cfg, log, report)
		if err != nil {
			return err
		}
		defer a.Close()

		client, err := a.NewWorkerClient()
		if err != nil {
			return err
		}
		if err := client.Start(ctx); err != nil {
			return fmt.Errorf("start worker: %w", err)
		}
		log.Info("worker started")
		<-ctx.Done()

		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := client.Stop(stopCtx); err != nil {
			return fmt.Errorf("stop worker: %w", err)
		}
		return nil
	})
}
