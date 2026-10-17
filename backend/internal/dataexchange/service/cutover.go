package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/workbook"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// FileResult adalah hasil impor satu file untuk satu jenis data dari cmd/cutover (Technical Guideline 5.19).
type FileResult struct {
	Kind  string
	Label string
	// Skipped: sheet jenis data tidak ada di file, atau file yang sama sudah selesai diimpor (idempotent per file).
	Skipped    bool
	SkipReason string
	Total      int
	Valid      int
	Errors     int
	Committed  int
	// RowErrors adalah contoh baris error (maks. 20) untuk ditampilkan di terminal.
	RowErrors []domain.RowResult
	JobID     *uuid.UUID
}

// maxReportedRowErrors membatasi baris error yang dilaporkan cmd/cutover.
const maxReportedRowErrors = 20

// HasSheet memeriksa apakah file xlsx memuat sheet jenis data (cmd/cutover memakai satu workbook berisi banyak
// sheet, mis. Template Migrasi Saldo Awal).
func (s *Service) HasSheet(kind string, data []byte) (bool, error) {
	imp, ok := s.importer(kind)
	if !ok {
		return false, unknownKind()
	}
	names, err := workbook.SheetNames(bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if strings.EqualFold(strings.TrimSpace(n), imp.Spec().Sheet) {
			return true, nil
		}
	}
	return false, nil
}

// ApplyFileRows memeriksa dan menerapkan baris file di transaksi aktif tanpa membuat job impor (dry-run
// cmd/cutover): setiap baris di savepoint; baris valid tetap diterapkan agar jenis data berikutnya melihatnya,
// lalu pemanggil membatalkan transaksi luar.
func (s *Service) ApplyFileRows(ctx context.Context, kind string, data []byte) (FileResult, error) {
	imp, _, err := s.requireImporter(ctx, kind)
	if err != nil {
		return FileResult{}, err
	}
	spec := imp.Spec()
	res := FileResult{Kind: kind, Label: spec.Label}
	sheet, err := workbook.Read(bytes.NewReader(data), spec)
	if err != nil {
		return FileResult{}, err
	}
	res.Total = len(sheet.Rows)
	errs := domain.CheckRows(spec.Columns, sheet.Rows)
	pseudo := uuid.New()
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, row := range sheet.Rows {
			if len(errs[row.Number]) > 0 {
				continue
			}
			err := db.Savepoint(withRowKey(ctx, pseudo, row.Number), func(ctx context.Context) error {
				return imp.ApplyRow(ctx, row)
			})
			if err != nil {
				rowErrs, sysErr := rowErrors(err, spec)
				if sysErr != nil {
					return fmt.Errorf("baris %d: %w", row.Number, sysErr)
				}
				errs[row.Number] = rowErrs
			}
		}
		return nil
	})
	if err != nil {
		return FileResult{}, err
	}
	for _, row := range sheet.Rows {
		if e := errs[row.Number]; len(e) > 0 {
			res.Errors++
			if len(res.RowErrors) < maxReportedRowErrors {
				res.RowErrors = append(res.RowErrors, domain.RowResult{Number: row.Number, Errors: e})
			}
		}
	}
	res.Valid = res.Total - res.Errors
	return res, nil
}

// RunFile mengimpor file lewat alur job yang sama dengan ERP (file di filestore, pemeriksaan, simpan) secara
// sinkron. File yang sama (SHA-256) yang sudah selesai diimpor dilewati, sehingga cut-over aman diulang. Bila ada
// baris error, job berhenti di status Diperiksa (bisa dilihat di Integrasi › Import/Export Excel) dan tidak ada
// baris yang disimpan.
func (s *Service) RunFile(ctx context.Context, kind, name string, data []byte) (FileResult, error) {
	imp, a, err := s.requireImporter(ctx, kind)
	if err != nil {
		return FileResult{}, err
	}
	spec := imp.Spec()
	res := FileResult{Kind: kind, Label: spec.Label}
	sum := sha256.Sum256(data)
	job, exists, err := s.Store.ImportJobBySHA(ctx, kind, sum[:])
	if err != nil {
		return FileResult{}, err
	}
	if exists && job.Status == domain.ImportCompleted {
		res.Skipped, res.SkipReason = true, "file yang sama sudah diimpor"
		res.Total, res.Valid, res.Committed, res.JobID = job.TotalRows, job.ValidRows, job.CommittedRows, &job.ID
		return res, nil
	}
	if !exists {
		id, err := uuid.NewV7()
		if err != nil {
			return FileResult{}, fmt.Errorf("new import job id: %w", err)
		}
		err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
			fileID, err := s.Files.Save(ctx, UploadOwnerType(kind), id, name, workbook.ContentType, data)
			if err != nil {
				return err
			}
			job, err = s.Store.InsertImportJob(ctx, domain.ImportJob{
				ID: id, Kind: kind, FileID: fileID, FileName: name, FileSHA256: sum[:],
			}, a.UserID)
			return err
		})
		if err != nil {
			return FileResult{}, err
		}
	}
	if job.Status == domain.ImportUploaded || job.Status == domain.ImportValidating {
		if err := s.RunImport(ctx, job.ID); err != nil {
			return FileResult{}, err
		}
		if job, err = s.Store.GetImportJob(ctx, job.ID, false); err != nil {
			return FileResult{}, err
		}
	}
	res.JobID = &job.ID
	res.Total, res.Valid, res.Errors = job.TotalRows, job.ValidRows, job.ErrorRows
	if job.Status == domain.ImportFailed {
		return res, apperr.WithMessage(apperr.CodeValidationFailed, job.Message)
	}
	if job.ErrorRows > 0 {
		invalid := false
		rows, err := s.Store.ListImportRows(ctx, job.ID, &invalid, paging.New(nil, ptrInt(maxReportedRowErrors)))
		if err != nil {
			return FileResult{}, err
		}
		res.RowErrors = rows.Items
		return res, nil
	}
	if job.Status == domain.ImportValidated {
		// Tanpa enqueue job river: penyimpanan dijalankan sinkron di bawah.
		if _, err := s.transition(ctx, job.ID, func(j *domain.ImportJob) error { return j.Commit() }, false); err != nil {
			return FileResult{}, err
		}
		job.Status = domain.ImportCommitting
	}
	if job.Status == domain.ImportCommitting {
		if err := s.RunImport(ctx, job.ID); err != nil {
			return FileResult{}, err
		}
	}
	if job, err = s.Store.GetImportJob(ctx, job.ID, false); err != nil {
		return FileResult{}, err
	}
	res.Committed = job.CommittedRows
	if job.Status != domain.ImportCompleted {
		return res, apperr.WithMessage(apperr.CodeValidationFailed, job.Message)
	}
	return res, nil
}

func ptrInt(n int) *int { return &n }
