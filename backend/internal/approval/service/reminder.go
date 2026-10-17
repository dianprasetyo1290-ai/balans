package service

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"balans/internal/approval/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/jobqueue"
)

// RemindArgs adalah argumen job approval.remind (Technical Guideline Lampiran B).
type RemindArgs struct{}

// Kind adalah nama job.
func (RemindArgs) Kind() string { return "approval.remind" }

// InsertOpts: maks. 3 percobaan.
func (RemindArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 3}
}

// Remind mengirim pengingat untuk level yang belum diputuskan lebih dari 1 jam, hanya di jam kerja (NFR-11).
// Mengembalikan jumlah dokumen yang diingatkan.
func (s *Service) Remind(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	if !domain.InWorkingHours(now) {
		return 0, nil
	}
	ctx = actor.System(ctx)
	due, err := s.Store.DueForReminder(ctx, now.Add(-domain.ReminderInterval))
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, r := range due {
		if !r.DueForReminder(now, domain.ReminderInterval) {
			continue
		}
		err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
			step := r.CurrentStep()
			step.LastRemindedAt = &now
			if err := s.Store.SaveSteps(ctx, []domain.Step{*step}); err != nil {
				return err
			}
			return s.notifyApprovers(ctx, r, true)
		})
		if err != nil {
			return sent, fmt.Errorf("remind approval request %s: %w", r.ID, err)
		}
		sent++
	}
	return sent, nil
}

// RemindWorker menjalankan job approval.remind.
type RemindWorker struct {
	river.WorkerDefaults[RemindArgs]
	service *Service
}

// NewRemindWorker membuat worker.
func NewRemindWorker(s *Service) *RemindWorker { return &RemindWorker{service: s} }

// Work mengirim pengingat.
func (w *RemindWorker) Work(ctx context.Context, _ *river.Job[RemindArgs]) error {
	n, err := w.service.Remind(ctx)
	if n > 0 {
		w.service.Log.InfoContext(ctx, "approval reminders sent", "count", n)
	}
	return err
}
