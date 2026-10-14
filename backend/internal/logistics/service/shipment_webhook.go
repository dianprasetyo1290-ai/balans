// Package service berisi use case modul logistics.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/logistics/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/webhook"
)

// ShipmentProvider adalah kontrak penyedia kurir (Biteship). Adapter asli dan mock mengimplementasikan
// interface yang sama (Technical Guideline 5.10).
type ShipmentProvider interface {
	Provider() string
	Rates(ctx context.Context, req domain.RateRequest) ([]domain.Rate, error)
	CreateOrder(ctx context.Context, order domain.ShipmentOrder) (domain.Shipment, error)
	Status(ctx context.Context, referenceID string) (domain.ShipmentStatus, error)
	// ParseNotification memverifikasi signature lalu menerjemahkan payload provider.
	ParseNotification(ctx context.Context, signature string, body []byte) (domain.ShipmentNotification, error)
	// DecodeNotification menerjemahkan payload tersimpan yang sudah diverifikasi saat diterima.
	DecodeNotification(body []byte) (domain.ShipmentNotification, error)
}

type webhookInbox interface {
	Receive(ctx context.Context, e webhook.Event, job webhook.JobFactory) (bool, error)
	Get(ctx context.Context, id uuid.UUID) (webhook.StoredEvent, error)
	MarkProcessed(ctx context.Context, id uuid.UUID) error
}

// ShipmentWebhookArgs adalah argumen job shipment_webhook.process.
type ShipmentWebhookArgs struct {
	EventID uuid.UUID `json:"eventId"`
}

// Kind adalah nama job di Technical Guideline Lampiran B.
func (ShipmentWebhookArgs) Kind() string { return "shipment_webhook.process" }

// InsertOpts: queue default, maks. 15 percobaan.
func (ShipmentWebhookArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 15}
}

// ShipmentWebhookService menerima dan memproses webhook pengiriman.
type ShipmentWebhookService struct {
	provider ShipmentProvider
	inbox    webhookInbox
	log      *slog.Logger
}

// NewShipmentWebhookService membuat service.
func NewShipmentWebhookService(provider ShipmentProvider, inbox webhookInbox, log *slog.Logger) *ShipmentWebhookService {
	return &ShipmentWebhookService{provider: provider, inbox: inbox, log: log}
}

// Receive memverifikasi webhook, menyimpannya, dan meng-enqueue pemrosesan dalam satu transaksi.
func (s *ShipmentWebhookService) Receive(ctx context.Context, signature string, body []byte) (duplicate bool, err error) {
	n, err := s.provider.ParseNotification(ctx, signature, body)
	if errors.Is(err, domain.ErrInvalidSignature) {
		s.log.WarnContext(ctx, "shipment webhook rejected", "provider", s.provider.Provider(), "reason", "invalid signature")
		return false, apperr.Wrap(apperr.CodeForbidden, err)
	}
	if err != nil {
		return false, apperr.Wrap(apperr.CodeBadRequest, err)
	}
	return s.inbox.Receive(ctx, webhook.Event{
		Provider:  s.provider.Provider(),
		EventID:   n.EventID,
		EventType: string(n.Status),
		Payload:   body,
	}, func(id uuid.UUID) (river.JobArgs, *river.InsertOpts) {
		return ShipmentWebhookArgs{EventID: id}, nil
	})
}

// Process mengonfirmasi ulang status ke kurir. Pembaruan status Delivery Order ditambahkan saat
// modul pengiriman dibangun.
func (s *ShipmentWebhookService) Process(ctx context.Context, eventID uuid.UUID) error {
	event, err := s.inbox.Get(ctx, eventID)
	if err != nil {
		return err
	}
	if event.Processed {
		return nil
	}
	n, err := s.provider.DecodeNotification(event.Event.Payload)
	if err != nil {
		return fmt.Errorf("parse stored shipment notification: %w", err)
	}
	confirmed, err := s.provider.Status(ctx, n.ReferenceID)
	if err != nil {
		return fmt.Errorf("confirm shipment status %s: %w", n.ReferenceID, err)
	}
	if confirmed != n.Status {
		s.log.WarnContext(ctx, "shipment status differs from webhook",
			"reference_id", n.ReferenceID, "webhook_status", n.Status, "confirmed_status", confirmed)
	}
	s.log.InfoContext(ctx, "shipment status confirmed", "reference_id", n.ReferenceID, "status", confirmed)
	return s.inbox.MarkProcessed(ctx, eventID)
}

// ShipmentWebhookWorker menjalankan job shipment_webhook.process.
type ShipmentWebhookWorker struct {
	river.WorkerDefaults[ShipmentWebhookArgs]
	service *ShipmentWebhookService
}

// NewShipmentWebhookWorker membuat worker.
func NewShipmentWebhookWorker(s *ShipmentWebhookService) *ShipmentWebhookWorker {
	return &ShipmentWebhookWorker{service: s}
}

// Work memproses satu event.
func (w *ShipmentWebhookWorker) Work(ctx context.Context, job *river.Job[ShipmentWebhookArgs]) error {
	return w.service.Process(ctx, job.Args.EventID)
}
