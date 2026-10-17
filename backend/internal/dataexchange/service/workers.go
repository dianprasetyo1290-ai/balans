package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/platform/jobqueue"
)

// maxJobAttempts mengikuti Lampiran B (dataexchange.import/export: 3 percobaan).
const maxJobAttempts = 3

// ImportArgs adalah argumen job dataexchange.import: pemeriksaan atau penyimpanan satu job impor.
type ImportArgs struct {
	JobID uuid.UUID `json:"jobId"`
}

// Kind adalah nama job.
func (ImportArgs) Kind() string { return "dataexchange.import" }

// InsertOpts: antrean bulk agar impor besar tidak menahan job kritis.
func (ImportArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: maxJobAttempts}
}

// ExportArgs adalah argumen job dataexchange.export.
type ExportArgs struct {
	JobID uuid.UUID `json:"jobId"`
}

// Kind adalah nama job.
func (ExportArgs) Kind() string { return "dataexchange.export" }

// InsertOpts: antrean bulk.
func (ExportArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: maxJobAttempts}
}

// ImportWorker menjalankan dataexchange.import.
type ImportWorker struct {
	river.WorkerDefaults[ImportArgs]
	service *Service
}

// NewImportWorker membuat worker.
func NewImportWorker(s *Service) *ImportWorker { return &ImportWorker{service: s} }

// Work memeriksa atau menyimpan job impor.
func (w *ImportWorker) Work(ctx context.Context, job *river.Job[ImportArgs]) error {
	return w.service.RunImport(ctx, job.Args.JobID)
}

// ExportWorker menjalankan dataexchange.export.
type ExportWorker struct {
	river.WorkerDefaults[ExportArgs]
	service *Service
}

// NewExportWorker membuat worker.
func NewExportWorker(s *Service) *ExportWorker { return &ExportWorker{service: s} }

// Work membuat file ekspor.
func (w *ExportWorker) Work(ctx context.Context, job *river.Job[ExportArgs]) error {
	return w.service.RunExport(ctx, job.Args.JobID)
}
