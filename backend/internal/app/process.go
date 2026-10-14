package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"balans/internal/platform/config"
	"balans/internal/platform/logger"
)

// Main memuat konfigurasi, menyiapkan logger dan Sentry, lalu menjalankan run sampai menerima
// SIGINT/SIGTERM. Exit code 1 jika run gagal.
func Main(process string, run func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error) {
	log := logger.New(os.Stdout, os.Getenv("LOG_LEVEL")).With("process", process, "version", config.Version)
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	report, flush, err := InitSentry(cfg, process)
	if err != nil {
		log.Error("init sentry", "error", err)
		os.Exit(1)
	}
	defer flush()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log, report); err != nil {
		log.Error("process stopped with error", "error", err)
		flush()
		os.Exit(1) //nolint:gocritic // flush sudah dipanggil manual sebelum exit
	}
	log.Info("process stopped")
}

// Serve menjalankan server HTTP dan mematikannya dengan rapi saat ctx selesai.
func Serve(ctx context.Context, log *slog.Logger, addr string, handler http.Handler, shutdownTimeout time.Duration) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
