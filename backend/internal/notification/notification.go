// Package notification adalah API publik modul notifikasi (NFR-11). Modul lain hanya memakai package ini.
package notification

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/notification/domain"
	notificationhttp "balans/internal/notification/http"
	"balans/internal/notification/postgres"
	"balans/internal/notification/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/jobqueue"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "notification"

// Email adalah email transaksional.
type Email = domain.Email

// EmailSender diimplementasikan adapter email (Mailpit untuk mock, provider asli di production).
type EmailSender = service.EmailSender

// Deps adalah dependensi modul.
type Deps struct {
	Pool   *pgxpool.Pool
	Tx     *db.TxManager
	Jobs   *jobqueue.Inserter
	Clock  clock.Clock
	Log    *slog.Logger
	Sender EmailSender
	// Responder kosong = route notifikasi in-app tidak didaftarkan.
	Responder *httpx.Responder
}

// Module adalah modul notification.
type Module struct {
	emails *service.EmailService
	inapp  *service.NotificationService
	rs     *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	store := postgres.NewEmailStore(d.Pool, d.Clock)
	return &Module{
		emails: service.NewEmailService(store, d.Sender, d.Tx, d.Jobs, d.Log),
		inapp:  service.NewNotificationService(postgres.NewNotificationStore(d.Pool), d.Clock),
		rs:     d.Responder,
	}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route notifikasi in-app milik user yang login.
func (m *Module) RegisterRoutes(r chi.Router) {
	if m.rs != nil {
		notificationhttp.Register(r, notificationhttp.NewHandler(m.inapp), m.rs)
	}
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewSendEmailWorker(m.emails))
}

// PeriodicJobs mengembalikan jadwal berkala modul.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// QueueEmail menyimpan email dan mengirimkannya lewat job setelah transaksi commit.
func (m *Module) QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error) {
	return m.emails.QueueEmail(ctx, to, subject, text)
}

// Notify menyimpan notifikasi in-app untuk para penerima di transaksi aktif (NFR-11).
func (m *Module) Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error {
	return m.inapp.Notify(ctx, userIDs, domain.Message{Kind: kind, Title: title, Body: body, Link: link})
}
