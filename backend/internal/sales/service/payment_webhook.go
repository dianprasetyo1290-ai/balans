// Package service berisi use case modul sales.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/platform/apperr"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/webhook"
	"balans/internal/sales/domain"
)

// PaymentGateway adalah kontrak payment gateway (Midtrans). Adapter asli dan mock mengimplementasikan
// interface yang sama (Technical Guideline 5.10).
type PaymentGateway interface {
	Provider() string
	CreateTransaction(ctx context.Context, req domain.PaymentRequest) (domain.PaymentTransaction, error)
	Status(ctx context.Context, orderID string) (domain.PaymentStatus, error)
	// ParseNotification memverifikasi signature lalu menerjemahkan payload provider.
	ParseNotification(ctx context.Context, body []byte) (domain.PaymentNotification, error)
	// DecodeNotification menerjemahkan payload tersimpan yang sudah diverifikasi saat diterima.
	DecodeNotification(body []byte) (domain.PaymentNotification, error)
}

type webhookInbox interface {
	Receive(ctx context.Context, e webhook.Event, job webhook.JobFactory) (bool, error)
	Get(ctx context.Context, id uuid.UUID) (webhook.StoredEvent, error)
	MarkProcessed(ctx context.Context, id uuid.UUID) error
}

// PaymentWebhookArgs adalah argumen job payment_webhook.process.
type PaymentWebhookArgs struct {
	EventID uuid.UUID `json:"eventId"`
}

// Kind adalah nama job di Technical Guideline Lampiran B.
func (PaymentWebhookArgs) Kind() string { return "payment_webhook.process" }

// InsertOpts: queue critical, maks. 15 percobaan (± 24 jam).
func (PaymentWebhookArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueCritical, MaxAttempts: 15}
}

// PaymentWebhookService menerima dan memproses webhook pembayaran.
type PaymentWebhookService struct {
	gateway PaymentGateway
	inbox   webhookInbox
	log     *slog.Logger
}

// NewPaymentWebhookService membuat service.
func NewPaymentWebhookService(gateway PaymentGateway, inbox webhookInbox, log *slog.Logger) *PaymentWebhookService {
	return &PaymentWebhookService{gateway: gateway, inbox: inbox, log: log}
}

// Receive memverifikasi webhook, menyimpannya, dan meng-enqueue pemrosesan dalam satu transaksi.
// Respons 200 hanya dikirim setelah langkah ini berhasil (Technical Guideline 7.4).
func (s *PaymentWebhookService) Receive(ctx context.Context, body []byte) (duplicate bool, err error) {
	n, err := s.gateway.ParseNotification(ctx, body)
	if errors.Is(err, domain.ErrInvalidSignature) {
		s.log.WarnContext(ctx, "payment webhook rejected", "provider", s.gateway.Provider(), "reason", "invalid signature")
		return false, apperr.Wrap(apperr.CodeForbidden, err)
	}
	if err != nil {
		return false, apperr.Wrap(apperr.CodeBadRequest, err)
	}
	return s.inbox.Receive(ctx, webhook.Event{
		Provider:  s.gateway.Provider(),
		EventID:   n.EventID,
		EventType: string(n.Status),
		Payload:   body,
	}, func(id uuid.UUID) (river.JobArgs, *river.InsertOpts) {
		return PaymentWebhookArgs{EventID: id}, nil
	})
}

// Process mengonfirmasi ulang status ke provider sebelum dipercaya (Technical Guideline 9.3).
// Pembaruan status order dan jurnal pembayaran ditambahkan saat checkout dibangun.
func (s *PaymentWebhookService) Process(ctx context.Context, eventID uuid.UUID) error {
	event, err := s.inbox.Get(ctx, eventID)
	if err != nil {
		return err
	}
	if event.Processed {
		return nil
	}
	n, err := s.gateway.DecodeNotification(event.Event.Payload)
	if err != nil {
		return fmt.Errorf("parse stored payment notification: %w", err)
	}
	confirmed, err := s.gateway.Status(ctx, n.OrderID)
	if err != nil {
		return fmt.Errorf("confirm payment status %s: %w", n.OrderID, err)
	}
	if confirmed != n.Status {
		s.log.WarnContext(ctx, "payment status differs from webhook",
			"order_id", n.OrderID, "webhook_status", n.Status, "confirmed_status", confirmed)
	}
	s.log.InfoContext(ctx, "payment status confirmed", "order_id", n.OrderID, "status", confirmed)
	return s.inbox.MarkProcessed(ctx, eventID)
}

// PaymentWebhookWorker menjalankan job payment_webhook.process.
type PaymentWebhookWorker struct {
	river.WorkerDefaults[PaymentWebhookArgs]
	service *PaymentWebhookService
}

// NewPaymentWebhookWorker membuat worker.
func NewPaymentWebhookWorker(s *PaymentWebhookService) *PaymentWebhookWorker {
	return &PaymentWebhookWorker{service: s}
}

// Work memproses satu event.
func (w *PaymentWebhookWorker) Work(ctx context.Context, job *river.Job[PaymentWebhookArgs]) error {
	return w.service.Process(ctx, job.Args.EventID)
}
