// Package postgres adalah repository job impor/ekspor.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// Store menyimpan import_jobs, import_job_rows, dan export_jobs.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	if tx, ok := db.Tx(ctx); ok {
		return sqlcgen.New(tx)
	}
	return sqlcgen.New(s.pool)
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func ptrUUID(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	id := n.UUID
	return &id
}

func text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toImportJob(r sqlcgen.ImportJob) domain.ImportJob {
	return domain.ImportJob{
		ID: r.ID, Kind: r.Kind, FileID: r.FileID, FileName: r.FileName, FileSHA256: r.FileSha256,
		Status: domain.ImportStatus(r.Status), TotalRows: int(r.TotalRows), ProcessedRows: int(r.ProcessedRows),
		ValidRows: int(r.ValidRows), ErrorRows: int(r.ErrorRows), CommittedRows: int(r.CommittedRows),
		ErrorFileID: ptrUUID(r.ErrorFileID), Message: r.Message, ValidatedAt: r.ValidatedAt, CompletedAt: r.CompletedAt,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toExportJob(r sqlcgen.ExportJob) domain.ExportJob {
	params := map[string]string{}
	if len(r.Params) > 0 {
		_ = json.Unmarshal(r.Params, &params) // kolom jsonb selalu objek string (ditulis InsertExportJob)
	}
	return domain.ExportJob{
		ID: r.ID, Kind: r.Kind, Status: domain.ExportStatus(r.Status), TotalRows: int(r.TotalRows),
		FileID: ptrUUID(r.FileID), FileName: r.FileName, Message: r.Message, CompletedAt: r.CompletedAt,
		Params: params, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func i32(n int) int32 { return int32(min(n, 1<<31-1)) } //nolint:gosec // dibatasi

// InsertImportJob mencatat job impor. File yang sama (per jenis data) atau file yang sudah dipakai job lain
// ditolak IMPORT_ALREADY_PROCESSED.
func (s *Store) InsertImportJob(ctx context.Context, j domain.ImportJob, actorID uuid.UUID) (domain.ImportJob, error) {
	r, err := s.q(ctx).InsertImportJob(ctx, sqlcgen.InsertImportJobParams{
		Kind: j.Kind, FileID: j.FileID, FileName: j.FileName, FileSha256: j.FileSHA256, ActorID: actorID,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ImportJob{}, apperr.Wrap(apperr.CodeImportAlreadyProcessed, err)
	}
	if err != nil {
		return domain.ImportJob{}, fmt.Errorf("insert import job: %w", err)
	}
	return toImportJob(r), nil
}

// GetImportJob membaca job; forUpdate mengunci baris.
func (s *Store) GetImportJob(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ImportJob, error) {
	var (
		r   sqlcgen.ImportJob
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetImportJobForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetImportJob(ctx, id)
	}
	if err != nil {
		return domain.ImportJob{}, notFound(err, "get import job")
	}
	return toImportJob(r), nil
}

// ListImportJobs mengembalikan riwayat impor untuk jenis data yang boleh dilihat actor.
func (s *Store) ListImportJobs(ctx context.Context, kinds []string, kind *string, page paging.Page) (paging.Result[domain.ImportJob], error) {
	rows, err := s.q(ctx).ListImportJobs(ctx, sqlcgen.ListImportJobsParams{
		Kinds: kinds, Kind: text(kind), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.ImportJob]{}, fmt.Errorf("list import jobs: %w", err)
	}
	total, err := s.q(ctx).CountImportJobs(ctx, sqlcgen.CountImportJobsParams{Kinds: kinds, Kind: text(kind)})
	if err != nil {
		return paging.Result[domain.ImportJob]{}, fmt.Errorf("count import jobs: %w", err)
	}
	out := make([]domain.ImportJob, len(rows))
	for i, r := range rows {
		out[i] = toImportJob(r)
	}
	return paging.Result[domain.ImportJob]{Items: out, Page: page, Total: total}, nil
}

// SaveImportJob menyimpan status dan hitungan job dengan optimistic locking.
func (s *Store) SaveImportJob(ctx context.Context, j domain.ImportJob, now time.Time, actorID uuid.UUID) (domain.ImportJob, error) {
	n, err := s.q(ctx).UpdateImportJob(ctx, sqlcgen.UpdateImportJobParams{
		Status: string(j.Status), TotalRows: i32(j.TotalRows), ProcessedRows: i32(j.ProcessedRows),
		ValidRows: i32(j.ValidRows), ErrorRows: i32(j.ErrorRows), CommittedRows: i32(j.CommittedRows),
		ErrorFileID: nullUUID(j.ErrorFileID), Message: j.Message, ValidatedAt: j.ValidatedAt, CompletedAt: j.CompletedAt,
		Now: now, ActorID: actorID, ID: j.ID, Version: i32(j.Version),
	})
	if err != nil {
		return domain.ImportJob{}, fmt.Errorf("update import job: %w", err)
	}
	if n == 0 {
		return domain.ImportJob{}, apperr.New(apperr.CodeVersionConflict)
	}
	j.Version++
	j.UpdatedAt = now
	return j, nil
}

// SetImportProgress menulis progress tanpa menaikkan versi (dipanggil di luar transaksi dry-run).
func (s *Store) SetImportProgress(ctx context.Context, id uuid.UUID, processed, committed int, now time.Time) error {
	if err := s.q(ctx).SetImportProgress(ctx, sqlcgen.SetImportProgressParams{
		ProcessedRows: i32(processed), CommittedRows: i32(committed), Now: now, ID: id,
	}); err != nil {
		return fmt.Errorf("set import progress: %w", err)
	}
	return nil
}

// ReplaceImportRows mengganti hasil pemeriksaan baris job.
func (s *Store) ReplaceImportRows(ctx context.Context, jobID uuid.UUID, rows []domain.RowResult) error {
	q := s.q(ctx)
	if err := q.DeleteImportRows(ctx, jobID); err != nil {
		return fmt.Errorf("delete import rows: %w", err)
	}
	params := make([]sqlcgen.InsertImportRowsParams, len(rows))
	for i, r := range rows {
		values, err := json.Marshal(r.Values)
		if err != nil {
			return err
		}
		errs := r.Errors
		if errs == nil {
			errs = []domain.RowError{}
		}
		encoded, err := json.Marshal(errs)
		if err != nil {
			return err
		}
		params[i] = sqlcgen.InsertImportRowsParams{
			JobID: jobID, RowNumber: i32(r.Number), IsValid: r.Valid(), CellValues: values, Errors: encoded,
		}
	}
	if len(params) == 0 {
		return nil
	}
	if _, err := q.InsertImportRows(ctx, params); err != nil {
		return fmt.Errorf("insert import rows: %w", err)
	}
	return nil
}

func toRowResult(r sqlcgen.ImportJobRow) (domain.RowResult, error) {
	out := domain.RowResult{Number: int(r.RowNumber)}
	if err := json.Unmarshal(r.CellValues, &out.Values); err != nil {
		return out, fmt.Errorf("decode import row values: %w", err)
	}
	if err := json.Unmarshal(r.Errors, &out.Errors); err != nil {
		return out, fmt.Errorf("decode import row errors: %w", err)
	}
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out, nil
}

// ListImportRows mengembalikan baris preview; valid nil = semua.
func (s *Store) ListImportRows(ctx context.Context, jobID uuid.UUID, valid *bool, page paging.Page) (paging.Result[domain.RowResult], error) {
	isValid := pgtype.Bool{}
	if valid != nil {
		isValid = pgtype.Bool{Bool: *valid, Valid: true}
	}
	rows, err := s.q(ctx).ListImportRows(ctx, sqlcgen.ListImportRowsParams{
		JobID: jobID, IsValid: isValid, Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.RowResult]{}, fmt.Errorf("list import rows: %w", err)
	}
	total, err := s.q(ctx).CountImportRows(ctx, sqlcgen.CountImportRowsParams{JobID: jobID, IsValid: isValid})
	if err != nil {
		return paging.Result[domain.RowResult]{}, fmt.Errorf("count import rows: %w", err)
	}
	out := make([]domain.RowResult, len(rows))
	for i, r := range rows {
		if out[i], err = toRowResult(r); err != nil {
			return paging.Result[domain.RowResult]{}, err
		}
	}
	return paging.Result[domain.RowResult]{Items: out, Page: page, Total: total}, nil
}

// ValidRowNumbers mengembalikan nomor baris valid hasil pemeriksaan.
func (s *Store) ValidRowNumbers(ctx context.Context, jobID uuid.UUID) (map[int]bool, error) {
	nums, err := s.q(ctx).ValidImportRowNumbers(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("valid import rows: %w", err)
	}
	out := make(map[int]bool, len(nums))
	for _, n := range nums {
		out[int(n)] = true
	}
	return out, nil
}

// SetRowErrors menandai baris gagal saat penyimpanan.
func (s *Store) SetRowErrors(ctx context.Context, jobID uuid.UUID, row int, errs []domain.RowError) error {
	encoded, err := json.Marshal(errs)
	if err != nil {
		return err
	}
	if err := s.q(ctx).SetImportRowError(ctx, sqlcgen.SetImportRowErrorParams{Errors: encoded, JobID: jobID, RowNumber: i32(row)}); err != nil {
		return fmt.Errorf("set import row error: %w", err)
	}
	return nil
}

// InsertExportJob mencatat permintaan ekspor.
func (s *Store) InsertExportJob(ctx context.Context, kind string, params map[string]string, actorID uuid.UUID) (domain.ExportJob, error) {
	if params == nil {
		params = map[string]string{}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return domain.ExportJob{}, fmt.Errorf("marshal export params: %w", err)
	}
	r, err := s.q(ctx).InsertExportJob(ctx, sqlcgen.InsertExportJobParams{Kind: kind, Params: raw, ActorID: actorID})
	if err != nil {
		return domain.ExportJob{}, fmt.Errorf("insert export job: %w", err)
	}
	return toExportJob(r), nil
}

// GetExportJob membaca job ekspor.
func (s *Store) GetExportJob(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ExportJob, error) {
	var (
		r   sqlcgen.ExportJob
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetExportJobForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetExportJob(ctx, id)
	}
	if err != nil {
		return domain.ExportJob{}, notFound(err, "get export job")
	}
	return toExportJob(r), nil
}

// ListExportJobs mengembalikan riwayat ekspor.
func (s *Store) ListExportJobs(ctx context.Context, kinds []string, kind *string, page paging.Page) (paging.Result[domain.ExportJob], error) {
	rows, err := s.q(ctx).ListExportJobs(ctx, sqlcgen.ListExportJobsParams{
		Kinds: kinds, Kind: text(kind), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.ExportJob]{}, fmt.Errorf("list export jobs: %w", err)
	}
	total, err := s.q(ctx).CountExportJobs(ctx, sqlcgen.CountExportJobsParams{Kinds: kinds, Kind: text(kind)})
	if err != nil {
		return paging.Result[domain.ExportJob]{}, fmt.Errorf("count export jobs: %w", err)
	}
	out := make([]domain.ExportJob, len(rows))
	for i, r := range rows {
		out[i] = toExportJob(r)
	}
	return paging.Result[domain.ExportJob]{Items: out, Page: page, Total: total}, nil
}

// SaveExportJob menyimpan status job ekspor.
func (s *Store) SaveExportJob(ctx context.Context, j domain.ExportJob, now time.Time, actorID uuid.UUID) (domain.ExportJob, error) {
	n, err := s.q(ctx).UpdateExportJob(ctx, sqlcgen.UpdateExportJobParams{
		Status: string(j.Status), TotalRows: i32(j.TotalRows), FileID: nullUUID(j.FileID), FileName: j.FileName,
		Message: j.Message, CompletedAt: j.CompletedAt, Now: now, ActorID: actorID, ID: j.ID, Version: i32(j.Version),
	})
	if err != nil {
		return domain.ExportJob{}, fmt.Errorf("update export job: %w", err)
	}
	if n == 0 {
		return domain.ExportJob{}, apperr.New(apperr.CodeVersionConflict)
	}
	j.Version++
	j.UpdatedAt = now
	return j, nil
}
