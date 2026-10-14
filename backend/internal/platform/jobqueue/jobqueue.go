// Package jobqueue membungkus river agar job selalu di-enqueue di dalam transaksi bisnis
// (transactional outbox, Technical Guideline 5.11 dan 7.3).
package jobqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"balans/internal/platform/db"
)

// Queue dipisah per prioritas supaya import besar tidak menahan konfirmasi pembayaran.
const (
	QueueCritical = "critical"
	QueueDefault  = river.QueueDefault
	QueueBulk     = "bulk"
)

// ErrNoTransaction dikembalikan jika Enqueue dipanggil di luar transaksi.
var ErrNoTransaction = errors.New("job must be enqueued inside a business transaction")

// Inserter memasukkan job ke antrean. Tidak menjalankan job.
type Inserter struct {
	client *river.Client[pgx.Tx]
}

// NewInserter membuat client river khusus insert.
func NewInserter(pool *pgxpool.Pool) (*Inserter, error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("create river insert client: %w", err)
	}
	return &Inserter{client: client}, nil
}

// Enqueue memasukkan job di transaksi yang aktif di context. Job hanya ada jika transaksi commit.
func (i *Inserter) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	tx, ok := db.Tx(ctx)
	if !ok {
		return fmt.Errorf("enqueue %s: %w", args.Kind(), ErrNoTransaction)
	}
	if _, err := i.client.InsertTx(ctx, tx, args, opts); err != nil {
		return fmt.Errorf("enqueue %s: %w", args.Kind(), err)
	}
	return nil
}

// QueueConfig mengatur konkurensi per queue untuk proses worker. Queue bulk dibuat rendah
// karena berbagi CPU dengan api di satu VPS (Technical Guideline 14.8).
func QueueConfig() map[string]river.QueueConfig {
	return map[string]river.QueueConfig{
		QueueCritical: {MaxWorkers: 10},
		QueueDefault:  {MaxWorkers: 10},
		QueueBulk:     {MaxWorkers: 2},
	}
}
