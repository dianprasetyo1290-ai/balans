package idempotency

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
)

// CleanupArgs adalah argumen job maintenance.cleanup_idempotency_keys.
type CleanupArgs struct{}

// Kind adalah nama job di Technical Guideline Lampiran B.
func (CleanupArgs) Kind() string { return "maintenance.cleanup_idempotency_keys" }

// InsertOpts: queue bulk, maks. 3 percobaan.
func (CleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: 3}
}

// CleanupWorker menghapus key yang sudah kedaluwarsa.
type CleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	store *Store
	clock clock.Clock
	log   *slog.Logger
}

// NewCleanupWorker membuat worker.
func NewCleanupWorker(store *Store, c clock.Clock, log *slog.Logger) *CleanupWorker {
	return &CleanupWorker{store: store, clock: c, log: log}
}

// Work menghapus key kedaluwarsa.
func (w *CleanupWorker) Work(ctx context.Context, _ *river.Job[CleanupArgs]) error {
	n, err := w.store.DeleteExpired(ctx, w.clock.Now())
	if err != nil {
		return err
	}
	w.log.InfoContext(ctx, "expired idempotency keys deleted", "count", n)
	return nil
}
