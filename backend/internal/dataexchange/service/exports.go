package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/workbook"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

const (
	notifyExportReady  = "dataexchange.export_ready"
	notifyExportFailed = "dataexchange.export_failed"
)

func exportLink(j domain.ExportJob) string {
	return "/integrations/data-exchange?tab=export&kind=" + j.Kind
}

// CreateExport menjadwalkan ekspor jenis data; file xlsx dibuat job dataexchange.export di antrean bulk.
func (s *Service) CreateExport(ctx context.Context, kind string) (domain.ExportJob, error) {
	_, a, err := s.requireExporter(ctx, kind)
	if err != nil {
		return domain.ExportJob{}, err
	}
	var job domain.ExportJob
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if job, err = s.Store.InsertExportJob(ctx, kind, a.UserID); err != nil {
			return err
		}
		return s.Jobs.Enqueue(ctx, ExportArgs{JobID: job.ID}, nil)
	})
	return job, err
}

// GetExport mengembalikan job ekspor.
func (s *Service) GetExport(ctx context.Context, id uuid.UUID) (domain.ExportJob, error) {
	j, err := s.Store.GetExportJob(ctx, id, false)
	if err != nil {
		return domain.ExportJob{}, err
	}
	if _, _, err := s.requireExporter(ctx, j.Kind); err != nil {
		return domain.ExportJob{}, err
	}
	return j, nil
}

// ListExports mengembalikan riwayat ekspor jenis data yang boleh diekspor actor.
func (s *Service) ListExports(ctx context.Context, kind *string, page paging.Page) (paging.Result[domain.ExportJob], error) {
	a, ok := actor.From(ctx)
	if !ok {
		return paging.Result[domain.ExportJob]{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	kinds := s.exportKinds(a)
	if len(kinds) == 0 {
		return paging.Result[domain.ExportJob]{}, apperr.New(apperr.CodeForbidden)
	}
	return s.Store.ListExportJobs(ctx, kinds, kind, page)
}

// ExportDownloadURL membuat URL unduhan file ekspor.
func (s *Service) ExportDownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	j, err := s.GetExport(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if j.FileID == nil {
		return "", time.Time{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "File ekspor belum selesai dibuat.")
	}
	return s.Files.DownloadURL(ctx, *j.FileID)
}

// RunExport dijalankan worker dataexchange.export. Job yang sudah selesai atau gagal tidak diproses ulang.
func (s *Service) RunExport(ctx context.Context, id uuid.UUID) error {
	j, err := s.Store.GetExportJob(ctx, id, false)
	if err != nil {
		return err
	}
	if j.Status == domain.ExportCompleted || j.Status == domain.ExportFailed {
		return nil
	}
	exp, ok := s.exporter(j.Kind)
	if !ok {
		return s.failExport(ctx, j, "Jenis data "+j.Kind+" tidak aktif.")
	}
	a, err := s.Actors.ActorFor(ctx, j.CreatedBy)
	if err != nil {
		if apperr.As(err).Code != apperr.CodeInternal {
			return s.failExport(ctx, j, "Pengguna pembuat ekspor tidak aktif.")
		}
		return err
	}
	spec := exp.Spec()
	if !a.Has(spec.ExportPermission()) {
		return s.failExport(ctx, j, "Pengguna tidak lagi memiliki akses ekspor "+spec.Label+".")
	}
	ctx = actor.With(ctx, a)
	j.Status = domain.ExportRunning
	if j, err = s.Store.SaveExportJob(ctx, j, s.now(), j.CreatedBy); err != nil {
		return err
	}
	w, err := workbook.NewWriter(spec, false)
	if err != nil {
		return err
	}
	err = exp.Export(ctx, func(r tabular.Row) error { return w.Write(r, "") })
	if err != nil {
		err = errors.Join(err, w.Discard())
		if appErr := apperr.As(err); appErr.Code != apperr.CodeInternal {
			return s.failExport(ctx, j, appErr.Message)
		}
		return err
	}
	rows := w.Rows()
	var buf bytes.Buffer
	if err := w.Close(&buf); err != nil {
		return err
	}
	name := fmt.Sprintf("Ekspor %s %s.xlsx", spec.Label, clock.Today(s.Clock))
	fileID, err := s.Files.Save(ctx, ownerExport+j.Kind, j.ID, name, workbook.ContentType, buf.Bytes())
	if err != nil {
		return err
	}
	now := s.now()
	j.Status, j.TotalRows, j.FileID, j.FileName, j.CompletedAt = domain.ExportCompleted, rows, &fileID, name, &now
	if j, err = s.Store.SaveExportJob(ctx, j, now, j.CreatedBy); err != nil {
		return err
	}
	s.notify(ctx, j.CreatedBy, notifyExportReady, "Ekspor siap diunduh: "+spec.Label,
		fmt.Sprintf("%d baris %s.", rows, spec.Label), exportLink(j))
	return nil
}

func (s *Service) failExport(ctx context.Context, j domain.ExportJob, msg string) error {
	now := s.now()
	j.Status, j.Message, j.CompletedAt = domain.ExportFailed, msg, &now
	saved, err := s.Store.SaveExportJob(ctx, j, now, j.CreatedBy)
	if err != nil {
		return err
	}
	s.notify(ctx, j.CreatedBy, notifyExportFailed, "Ekspor gagal", msg, exportLink(saved))
	return nil
}
