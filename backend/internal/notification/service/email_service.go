// Package service berisi use case notifikasi.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/notification/domain"
	"balans/internal/platform/jobqueue"
)

// EmailSender mengirim email lewat provider. Implementasinya adapter di integration/ (asli atau Mailpit untuk mock).
type EmailSender interface {
	Send(ctx context.Context, email domain.Email) error
}

type emailStore interface {
	Create(ctx context.Context, email domain.Email) (uuid.UUID, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Email, bool, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkAttemptFailed(ctx context.Context, id uuid.UUID, reason string) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type jobEnqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// SendEmailArgs adalah argumen job notification.send_email. Hanya berisi ID (Technical Guideline 7.3).
type SendEmailArgs struct {
	EmailID uuid.UUID `json:"emailId"`
}

// Kind adalah nama job di Technical Guideline Lampiran B.
func (SendEmailArgs) Kind() string { return "notification.send_email" }

// InsertOpts: maks. 5 percobaan (± 30 menit).
func (SendEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 5}
}

// EmailService menyimpan email dan mengirimkannya lewat job.
type EmailService struct {
	store  emailStore
	sender EmailSender
	tx     txRunner
	jobs   jobEnqueuer
	log    *slog.Logger
}

// NewEmailService membuat EmailService.
func NewEmailService(store emailStore, sender EmailSender, tx txRunner, jobs jobEnqueuer, log *slog.Logger) *EmailService {
	return &EmailService{store: store, sender: sender, tx: tx, jobs: jobs, log: log}
}

// QueueEmail menyimpan email dan meng-enqueue pengirimannya di transaksi yang sama. Jika dipanggil
// di dalam transaksi bisnis, email hanya terkirim bila transaksi tersebut commit.
func (s *EmailService) QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error) {
	email, err := domain.NewEmail(to, subject, text)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if id, err = s.store.Create(ctx, email); err != nil {
			return err
		}
		return s.jobs.Enqueue(ctx, SendEmailArgs{EmailID: id}, nil)
	})
	return id, err
}

// Send mengirim email yang tersimpan. Email yang sudah terkirim tidak dikirim ulang.
func (s *EmailService) Send(ctx context.Context, id uuid.UUID) error {
	email, sent, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}
	if err := s.sender.Send(ctx, email); err != nil {
		if markErr := s.store.MarkAttemptFailed(ctx, id, err.Error()); markErr != nil {
			s.log.WarnContext(ctx, "record email failure", "error", markErr)
		}
		return fmt.Errorf("send email %s: %w", id, err)
	}
	return s.store.MarkSent(ctx, id)
}

// SendEmailWorker menjalankan job notification.send_email.
type SendEmailWorker struct {
	river.WorkerDefaults[SendEmailArgs]
	service *EmailService
}

// NewSendEmailWorker membuat worker.
func NewSendEmailWorker(service *EmailService) *SendEmailWorker {
	return &SendEmailWorker{service: service}
}

// Work mengirim satu email.
func (w *SendEmailWorker) Work(ctx context.Context, job *river.Job[SendEmailArgs]) error {
	return w.service.Send(ctx, job.Args.EmailID)
}
