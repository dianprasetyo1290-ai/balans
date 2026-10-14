// Package webhook menyimpan webhook masuk dengan dedup per event lalu meneruskannya ke job
// (Technical Guideline 7.1 lapis webhook dan 7.4).
package webhook

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/webhook/sqlcgen"
)

// Event adalah webhook yang sudah diverifikasi signature-nya oleh adapter.
type Event struct {
	Provider  string
	EventID   string
	EventType string
	Payload   []byte
}

// StoredEvent adalah event yang tersimpan di webhook_events.
type StoredEvent struct {
	ID        uuid.UUID
	Event     Event
	Processed bool
}

// ErrEventNotFound dikembalikan jika ID event tidak ada.
var ErrEventNotFound = errors.New("webhook event not found")

type jobEnqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// JobFactory membuat job pemroses untuk event yang baru disimpan.
type JobFactory func(eventID uuid.UUID) (river.JobArgs, *river.InsertOpts)

// Inbox menerima dan menandai webhook.
type Inbox struct {
	pool  *pgxpool.Pool
	tx    *db.TxManager
	jobs  jobEnqueuer
	clock clock.Clock
}

// NewInbox membuat Inbox.
func NewInbox(pool *pgxpool.Pool, tx *db.TxManager, jobs jobEnqueuer, c clock.Clock) *Inbox {
	return &Inbox{pool: pool, tx: tx, jobs: jobs, clock: c}
}

func (in *Inbox) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, in.pool))
}

// Receive menyimpan event dan meng-enqueue job dalam satu transaksi. Event yang sama dari provider
// yang sama hanya diproses sekali; duplicate bernilai true untuk pengiriman ulang.
func (in *Inbox) Receive(ctx context.Context, e Event, job JobFactory) (duplicate bool, err error) {
	err = in.tx.WithTx(ctx, func(ctx context.Context) error {
		id, err := in.q(ctx).InsertWebhookEvent(ctx, sqlcgen.InsertWebhookEventParams{
			Provider:  e.Provider,
			EventID:   e.EventID,
			EventType: e.EventType,
			Payload:   e.Payload,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			duplicate = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("insert webhook event: %w", err)
		}
		args, opts := job(id)
		return in.jobs.Enqueue(ctx, args, opts)
	})
	return duplicate, err
}

// Get membaca event tersimpan.
func (in *Inbox) Get(ctx context.Context, id uuid.UUID) (StoredEvent, error) {
	row, err := in.q(ctx).GetWebhookEvent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredEvent{}, ErrEventNotFound
	}
	if err != nil {
		return StoredEvent{}, fmt.Errorf("get webhook event: %w", err)
	}
	return StoredEvent{
		ID: row.ID,
		Event: Event{
			Provider:  row.Provider,
			EventID:   row.EventID,
			EventType: row.EventType,
			Payload:   row.Payload,
		},
		Processed: row.ProcessedAt != nil,
	}, nil
}

// MarkProcessed menandai event selesai diproses. Aman dipanggil berulang.
func (in *Inbox) MarkProcessed(ctx context.Context, id uuid.UUID) error {
	if err := in.q(ctx).MarkWebhookEventProcessed(ctx, sqlcgen.MarkWebhookEventProcessedParams{
		ID:          id,
		ProcessedAt: new(in.clock.Now()),
	}); err != nil {
		return fmt.Errorf("mark webhook event processed: %w", err)
	}
	return nil
}
