package service

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"balans/internal/platform/jobqueue"
)

// ExpireReservationsArgs adalah argumen job stock_reservation.expire (Technical Guideline Lampiran B).
type ExpireReservationsArgs struct{}

// Kind adalah nama job.
func (ExpireReservationsArgs) Kind() string { return "stock_reservation.expire" }

// InsertOpts: queue critical, maks. 5 percobaan.
func (ExpireReservationsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueCritical, MaxAttempts: 5}
}

// ExpireReservationsWorker melepas reservasi yang melewati batas waktunya.
type ExpireReservationsWorker struct {
	river.WorkerDefaults[ExpireReservationsArgs]
	service *Service
}

// NewExpireReservationsWorker membuat worker.
func NewExpireReservationsWorker(s *Service) *ExpireReservationsWorker {
	return &ExpireReservationsWorker{service: s}
}

// Work melepas reservasi kedaluwarsa. Idempotent: reservasi yang sudah dilepas tidak terbuka lagi.
func (w *ExpireReservationsWorker) Work(ctx context.Context, _ *river.Job[ExpireReservationsArgs]) error {
	n, err := w.service.ReleaseExpired(ctx)
	if n > 0 && w.service.Log != nil {
		w.service.Log.InfoContext(ctx, "expired reservations released", "count", n)
	}
	if err != nil {
		return fmt.Errorf("release expired reservations: %w", err)
	}
	return nil
}
