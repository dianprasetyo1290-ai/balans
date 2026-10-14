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
	"balans/internal/notification/postgres"
	"balans/internal/notification/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
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
}

// Module adalah modul notification.
type Module struct {
	emails *service.EmailService
}

// New menyusun modul.
func New(d Deps) *Module {
	store := postgres.NewEmailStore(d.Pool, d.Clock)
	return &Module{emails: service.NewEmailService(store, d.Sender, d.Tx, d.Jobs, d.Log)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes tidak mendaftarkan route; notifikasi in-app menyusul bersama login di R1.1.
func (m *Module) RegisterRoutes(chi.Router) {}

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
