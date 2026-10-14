// Package app menyusun seluruh modul (dependency injection manual, Technical Guideline 5.1).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/integration/mockconsole"
	"balans/internal/integration/mockpayment"
	"balans/internal/integration/mockshipment"
	"balans/internal/integration/smtpmail"
	"balans/internal/logistics"
	"balans/internal/notification"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/webhook"
	"balans/internal/sales"
	"balans/internal/system"
)

// App berisi dependensi bersama dan modul yang aktif.
type App struct {
	Config    config.Config
	Log       *slog.Logger
	Pool      *pgxpool.Pool
	Clock     clock.Clock
	Tx        *db.TxManager
	Jobs      *jobqueue.Inserter
	Responder *httpx.Responder
	Migrator  *db.Migrator

	idempotency *idempotency.Store
	system      *system.Module
	modules     []module
	console     *mockconsole.Console
}

// New membuka koneksi database dan menyusun modul sesuai ENABLED_MODULES.
func New(ctx context.Context, cfg config.Config, log *slog.Logger, report httpx.ErrorReporter) (*App, error) {
	if err := validateModules(cfg.EnabledModules); err != nil {
		return nil, err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{
		Config:      cfg,
		Log:         log,
		Pool:        pool,
		Clock:       clock.System{},
		Tx:          db.NewTxManager(pool),
		Responder:   httpx.NewResponder(log, report),
		idempotency: idempotency.NewStore(pool),
	}
	if a.Jobs, err = jobqueue.NewInserter(pool); err != nil {
		pool.Close()
		return nil, err
	}
	if a.Migrator, err = db.NewMigrator(pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := a.buildModules(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// Close menutup koneksi.
func (a *App) Close() {
	if a.Migrator != nil {
		if err := a.Migrator.Close(); err != nil {
			a.Log.Warn("close migrator", "error", err)
		}
	}
	a.Pool.Close()
}

func (a *App) buildModules() error {
	cfg := a.Config
	a.system = system.New(system.Deps{
		Build: system.Build{
			Version:        config.Version,
			Commit:         config.Commit,
			Environment:    string(cfg.Env),
			EnabledModules: cfg.EnabledModules,
			MockActive:     cfg.MockActive(),
		},
		Pool:      a.Pool,
		Schema:    a.Migrator,
		Clock:     a.Clock,
		Responder: a.Responder,
	})

	inbox := webhook.NewInbox(a.Pool, a.Tx, a.Jobs, a.Clock)
	console := mockconsole.Deps{Clock: a.Clock, Log: a.Log}

	if cfg.IsEnabled(notification.Name) {
		if cfg.EmailProvider != config.ProviderMock {
			return providerNotAvailable("EMAIL_PROVIDER", cfg.EmailProvider)
		}
		m := notification.New(notification.Deps{
			Pool: a.Pool, Tx: a.Tx, Jobs: a.Jobs, Clock: a.Clock, Log: a.Log,
			Sender: smtpmail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.MailFrom),
		})
		a.modules = append(a.modules, m)
		console.Email = m
	}
	if cfg.IsEnabled(sales.Name) {
		if cfg.PaymentGateway != config.ProviderMock {
			return providerNotAvailable("PAYMENT_GATEWAY", cfg.PaymentGateway)
		}
		gateway := mockpayment.New(a.Pool, mockpayment.Config{
			ServerKey:     cfg.MidtransServerKey,
			PublicBaseURL: cfg.PublicBaseURL,
			WebhookURL:    cfg.InternalAPIURL + "/api/v1/webhooks/midtrans",
		})
		a.modules = append(a.modules, sales.New(sales.Deps{Gateway: gateway, Inbox: inbox, Responder: a.Responder, Log: a.Log}))
		console.Payments = gateway
	}
	if cfg.IsEnabled(logistics.Name) {
		if cfg.ShipmentProvider != config.ProviderMock {
			return providerNotAvailable("SHIPMENT_PROVIDER", cfg.ShipmentProvider)
		}
		provider := mockshipment.New(a.Pool, mockshipment.Config{
			WebhookSecret: cfg.BiteshipWebhookSecret,
			WebhookURL:    cfg.InternalAPIURL + "/api/v1/webhooks/biteship",
		})
		a.modules = append(a.modules, logistics.New(logistics.Deps{Provider: provider, Inbox: inbox, Responder: a.Responder, Log: a.Log}))
		console.Shipments = provider
	}

	if cfg.MockActive() && cfg.Env != config.EnvProduction && (console.Email != nil || console.Payments != nil || console.Shipments != nil) {
		a.console = mockconsole.New(console)
	}
	return nil
}

// errProviderNotAvailable dipakai sampai adapter provider asli dibangun.
var errProviderNotAvailable = errors.New("adapter provider asli belum tersedia")

func providerNotAvailable(env, value string) error {
	return fmt.Errorf("%s=%q: %w; gunakan %q", env, value, errProviderNotAvailable, config.ProviderMock)
}
