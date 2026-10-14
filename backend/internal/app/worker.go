package app

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
)

const maxJobAttempts = 5

// NewWorkerClient membuat client river yang menjalankan job dan jadwal berkala modul aktif.
// Job modul nonaktif tidak didaftarkan dan tidak dijadwalkan (Technical Guideline 14.5).
func (a *App) NewWorkerClient() (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, idempotency.NewCleanupWorker(a.idempotency, a.Clock, a.Log))

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(jobqueue.DailyAt{Hour: 2},
			func() (river.JobArgs, *river.InsertOpts) { return idempotency.CleanupArgs{}, nil },
			&river.PeriodicJobOpts{ID: idempotency.CleanupArgs{}.Kind()}),
	}
	for _, m := range a.modules {
		m.RegisterWorkers(workers)
		periodic = append(periodic, m.PeriodicJobs()...)
	}

	client, err := river.NewClient(riverpgxv5.New(a.Pool), &river.Config{
		Queues:       jobqueue.QueueConfig(),
		Workers:      workers,
		PeriodicJobs: periodic,
		MaxAttempts:  maxJobAttempts,
		Logger:       a.Log,
	})
	if err != nil {
		return nil, fmt.Errorf("create river worker client: %w", err)
	}
	return client, nil
}
