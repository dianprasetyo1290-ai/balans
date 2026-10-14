package app

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"

	"balans/internal/platform/config"
	"balans/internal/platform/httpx"
	"balans/internal/platform/logger"
)

// InitSentry menyalakan error tracking jika SENTRY_DSN diisi. Data pribadi dibuang sebelum dikirim
// (Technical Guideline 13). Fungsi flush dipanggil saat proses berhenti.
func InitSentry(cfg config.Config, process string) (httpx.ErrorReporter, func(), error) {
	if cfg.SentryDSN == "" {
		return nil, func() {}, nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         cfg.SentryDSN,
		Environment: string(cfg.Env),
		Release:     "balans-backend@" + config.Version,
		ServerName:  process,
		BeforeSend:  scrubEvent,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("init sentry: %w", err)
	}
	report := func(ctx context.Context, err error) {
		hub := sentry.CurrentHub().Clone()
		hub.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("process", process)
			if id := logger.RequestID(ctx); id != "" {
				scope.SetTag("request_id", id)
			}
		})
		hub.CaptureException(err)
	}
	return report, func() { sentry.Flush(2 * time.Second) }, nil
}

// scrubEvent membuang header, cookie, query string, body, dan identitas user selain ID.
func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request != nil {
		event.Request.Cookies = ""
		event.Request.Headers = nil
		event.Request.QueryString = ""
		event.Request.Data = ""
		event.Request.Env = nil
	}
	event.User = sentry.User{ID: event.User.ID}
	return event
}
