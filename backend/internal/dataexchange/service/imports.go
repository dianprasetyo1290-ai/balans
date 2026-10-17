package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/workbook"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

// Jenis notifikasi in-app.
const (
	notifyImportValidated = "dataexchange.import_validated"
	notifyImportCompleted = "dataexchange.import_completed"
	notifyImportFailed    = "dataexchange.import_failed"
)

// progressEvery menentukan seberapa sering progress ditulis.
const progressEvery = 50

func importLink(j domain.ImportJob) string {
	return "/integrasi/impor-ekspor?kind=" + j.Kind + "&job=" + j.ID.String()
}

// WriteTemplate menulis template impor jenis data ke w.
func (s *Service) WriteTemplate(ctx context.Context, kind string, w io.Writer) (domain.Spec, error) {
	imp, _, err := s.requireImporter(ctx, kind)
	if err != nil {
		return domain.Spec{}, err
	}
	return imp.Spec(), workbook.WriteTemplate(w, imp.Spec())
}

// CreateImport mencatat file yang sudah diunggah sebagai job impor lalu menjadwalkan pemeriksaan di antrean
// bulk. File yang sama (SHA-256) untuk jenis data yang sama ditolak IMPORT_ALREADY_PROCESSED.
func (s *Service) CreateImport(ctx context.Context, kind string, fileID uuid.UUID) (domain.ImportJob, error) {
	_, a, err := s.requireImporter(ctx, kind)
	if err != nil {
		return domain.ImportJob{}, err
	}
	invalid := func(msg string) error { return apperr.Validation(map[string]string{"fileId": msg}) }
	f, err := s.Files.Info(ctx, fileID)
	switch {
	case apperr.HasCode(err, apperr.CodeNotFound):
		return domain.ImportJob{}, invalid("File tidak ditemukan")
	case err != nil:
		return domain.ImportJob{}, err
	case f.OwnerType != UploadOwnerType(kind):
		return domain.ImportJob{}, invalid("File tidak diunggah untuk jenis data ini")
	case !f.Uploaded:
		return domain.ImportJob{}, invalid("File belum selesai diunggah")
	case f.CreatedBy != a.UserID:
		return domain.ImportJob{}, invalid("File diunggah pengguna lain")
	}
	var job domain.ImportJob
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		job, err = s.Store.InsertImportJob(ctx, domain.ImportJob{
			Kind: kind, FileID: fileID, FileName: f.Name, FileSHA256: f.SHA256,
		}, a.UserID)
		if err != nil {
			return err
		}
		if err := s.Files.Attach(ctx, fileID, UploadOwnerType(kind), job.ID); err != nil {
			return err
		}
		return s.Jobs.Enqueue(ctx, ImportArgs{JobID: job.ID}, nil)
	})
	if apperr.HasCode(err, apperr.CodeImportAlreadyProcessed) {
		return domain.ImportJob{}, apperr.WithMessage(apperr.CodeImportAlreadyProcessed,
			"File ini sudah pernah diimpor atau sedang diproses. Buka riwayat impor untuk melihat hasilnya.")
	}
	return job, err
}

// GetImport mengembalikan job impor.
func (s *Service) GetImport(ctx context.Context, id uuid.UUID) (domain.ImportJob, error) {
	j, err := s.Store.GetImportJob(ctx, id, false)
	if err != nil {
		return domain.ImportJob{}, err
	}
	if _, _, err := s.requireImporter(ctx, j.Kind); err != nil {
		return domain.ImportJob{}, err
	}
	return j, nil
}

// ListImports mengembalikan riwayat impor jenis data yang boleh diimpor actor.
func (s *Service) ListImports(ctx context.Context, kind *string, page paging.Page) (paging.Result[domain.ImportJob], error) {
	a, ok := actor.From(ctx)
	if !ok {
		return paging.Result[domain.ImportJob]{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	kinds := s.importKinds(a)
	if len(kinds) == 0 {
		return paging.Result[domain.ImportJob]{}, apperr.New(apperr.CodeForbidden)
	}
	return s.Store.ListImportJobs(ctx, kinds, kind, page)
}

// ImportRows mengembalikan baris hasil pemeriksaan; valid nil = semua baris.
func (s *Service) ImportRows(ctx context.Context, id uuid.UUID, valid *bool, page paging.Page) (paging.Result[domain.RowResult], error) {
	if _, err := s.GetImport(ctx, id); err != nil {
		return paging.Result[domain.RowResult]{}, err
	}
	return s.Store.ListImportRows(ctx, id, valid, page)
}

// CommitImport mengonfirmasi penyimpanan baris valid; penyimpanan berjalan sebagai job.
func (s *Service) CommitImport(ctx context.Context, id uuid.UUID) (domain.ImportJob, error) {
	return s.transition(ctx, id, func(j *domain.ImportJob) error { return j.Commit() }, true)
}

// CancelImport membatalkan job yang menunggu konfirmasi; file yang sama boleh diunggah ulang.
func (s *Service) CancelImport(ctx context.Context, id uuid.UUID) (domain.ImportJob, error) {
	return s.transition(ctx, id, func(j *domain.ImportJob) error {
		if err := j.Cancel(); err != nil {
			return err
		}
		now := s.now()
		j.CompletedAt = &now
		return nil
	}, false)
}

func (s *Service) transition(ctx context.Context, id uuid.UUID, apply func(*domain.ImportJob) error, enqueue bool) (domain.ImportJob, error) {
	var out domain.ImportJob
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		j, err := s.Store.GetImportJob(ctx, id, true)
		if err != nil {
			return err
		}
		_, a, err := s.requireImporter(ctx, j.Kind)
		if err != nil {
			return err
		}
		if err := apply(&j); err != nil {
			return err
		}
		if out, err = s.Store.SaveImportJob(ctx, j, s.now(), a.UserID); err != nil {
			return err
		}
		if enqueue {
			return s.Jobs.Enqueue(ctx, ImportArgs{JobID: id}, nil)
		}
		return nil
	})
	return out, err
}

// ImportErrorFileURL membuat URL unduhan file baris error.
func (s *Service) ImportErrorFileURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	j, err := s.GetImport(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if j.ErrorFileID == nil {
		return "", time.Time{}, apperr.WithMessage(apperr.CodeNotFound, "Tidak ada baris error.")
	}
	return s.Files.DownloadURL(ctx, *j.ErrorFileID)
}

// RunImport dijalankan worker dataexchange.import: memeriksa file (status uploaded/validating) atau menyimpan
// baris valid (status committing). Status lain tidak diproses ulang (idempotent).
func (s *Service) RunImport(ctx context.Context, id uuid.UUID) error {
	j, err := s.Store.GetImportJob(ctx, id, false)
	if err != nil {
		return err
	}
	if !j.Status.Running() {
		return nil
	}
	imp, ok := s.importer(j.Kind)
	if !ok {
		return s.failImport(ctx, j, "Jenis data "+j.Kind+" tidak aktif.")
	}
	a, err := s.Actors.ActorFor(ctx, j.CreatedBy)
	if err != nil {
		if apperr.As(err).Code != apperr.CodeInternal {
			return s.failImport(ctx, j, "Pengguna pembuat impor tidak aktif.")
		}
		return err
	}
	if !a.Has(imp.Spec().ImportPermission()) {
		return s.failImport(ctx, j, "Pengguna tidak lagi memiliki akses impor "+imp.Spec().Label+".")
	}
	ctx = actor.With(ctx, a)
	if j.Status == domain.ImportCommitting {
		return s.commit(ctx, j, imp)
	}
	return s.validate(ctx, j, imp)
}

func (s *Service) failImport(ctx context.Context, j domain.ImportJob, msg string) error {
	now := s.now()
	j.Status, j.Message, j.CompletedAt = domain.ImportFailed, msg, &now
	saved, err := s.Store.SaveImportJob(db.WithoutTx(ctx), j, now, j.CreatedBy)
	if err != nil {
		return err
	}
	s.notify(ctx, j.CreatedBy, notifyImportFailed, "Impor gagal: "+j.FileName, msg, importLink(saved))
	return nil
}

func (s *Service) readFile(ctx context.Context, j domain.ImportJob, spec domain.Spec) (workbook.Sheet, error) {
	r, err := s.Files.Open(ctx, j.FileID)
	if err != nil {
		return workbook.Sheet{}, err
	}
	defer r.Close() //nolint:errcheck // hanya membaca
	return workbook.Read(r, spec)
}

// rowErrors memetakan error ApplyRow ke error baris. Pelanggaran constraint database (kelas 23) dianggap error
// data; error lain dikembalikan sebagai gangguan sistem.
func rowErrors(err error, spec domain.Spec) ([]domain.RowError, error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && len(pgErr.Code) == 5 && pgErr.Code[:2] == "23" {
		return []domain.RowError{{Message: "Data tidak memenuhi aturan penyimpanan (" + pgErr.ConstraintName + ")"}}, nil
	}
	return domain.ErrorsFrom(err, spec.Columns)
}

var errDryRun = errors.New("dry-run selesai")

func withRowKey(ctx context.Context, jobID uuid.UUID, row int) context.Context {
	return context.WithValue(ctx, rowKeyCtx{}, jobID.String()+":"+strconv.Itoa(row))
}

// checkRows menjalankan pemeriksaan semua baris di satu transaksi yang selalu dibatalkan. Tiap baris berjalan
// di savepoint sehingga baris berikutnya melihat hasil baris sebelumnya (mis. komponen bundel dari SKU baru di
// file yang sama), tetapi tidak ada data yang tersimpan.
func (s *Service) checkRows(ctx context.Context, j domain.ImportJob, imp Importer, rows []tabular.Row) (map[int][]domain.RowError, error) {
	spec := imp.Spec()
	errs := domain.CheckRows(spec.Columns, rows)
	validator, hasValidator := imp.(RowValidator)
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for i, row := range rows {
			if len(errs[row.Number]) == 0 {
				rctx := withRowKey(ctx, j.ID, row.Number)
				err := db.Savepoint(rctx, func(ctx context.Context) error {
					if hasValidator {
						return validator.ValidateRow(ctx, row)
					}
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
			if (i+1)%progressEvery == 0 {
				if err := s.Store.SetImportProgress(db.WithoutTx(ctx), j.ID, i+1, 0, s.now()); err != nil {
					return err
				}
			}
		}
		return errDryRun
	})
	if !errors.Is(err, errDryRun) {
		return nil, err
	}
	return errs, nil
}

func (s *Service) validate(ctx context.Context, j domain.ImportJob, imp Importer) error {
	spec := imp.Spec()
	j.Status, j.ProcessedRows, j.Message = domain.ImportValidating, 0, ""
	j, err := s.Store.SaveImportJob(ctx, j, s.now(), j.CreatedBy)
	if err != nil {
		return err
	}
	sheet, err := s.readFile(ctx, j, spec)
	if err != nil {
		if appErr := apperr.As(err); appErr.Code != apperr.CodeInternal {
			return s.failImport(ctx, j, appErr.Message)
		}
		return err
	}
	if len(sheet.Rows) == 0 {
		return s.failImport(ctx, j, "File tidak berisi data. Isi data mulai baris 2 template.")
	}
	j.TotalRows = len(sheet.Rows)
	if err := s.Store.SetImportProgress(ctx, j.ID, 0, 0, s.now()); err != nil {
		return err
	}
	if j, err = s.Store.SaveImportJob(ctx, j, s.now(), j.CreatedBy); err != nil {
		return err
	}
	errs, err := s.checkRows(ctx, j, imp, sheet.Rows)
	if err != nil {
		return err
	}
	results := make([]domain.RowResult, len(sheet.Rows))
	valid := 0
	for i, r := range sheet.Rows {
		results[i] = domain.RowResult{Number: r.Number, Values: domain.PreviewValues(spec.Columns, r), Errors: errs[r.Number]}
		if results[i].Valid() {
			valid++
		}
	}
	var errorFile *uuid.UUID
	if valid < len(results) {
		id, err := s.saveErrorFile(ctx, j, spec, sheet.Rows, errs)
		if err != nil {
			return err
		}
		errorFile = &id
	}
	now := s.now()
	j.Status, j.ProcessedRows, j.ValidRows, j.ErrorRows = domain.ImportValidated, len(results), valid, len(results)-valid
	j.ErrorFileID, j.ValidatedAt = errorFile, &now
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.Store.ReplaceImportRows(ctx, j.ID, results); err != nil {
			return err
		}
		j, err = s.Store.SaveImportJob(ctx, j, now, j.CreatedBy)
		return err
	})
	if err != nil {
		return err
	}
	s.notify(ctx, j.CreatedBy, notifyImportValidated, "Pemeriksaan impor selesai: "+j.FileName,
		fmt.Sprintf("%d baris valid, %d baris error. Periksa hasilnya lalu simpan.", j.ValidRows, j.ErrorRows), importLink(j))
	return nil
}

func (s *Service) saveErrorFile(ctx context.Context, j domain.ImportJob, spec domain.Spec, rows []tabular.Row, errs map[int][]domain.RowError) (uuid.UUID, error) {
	w, err := workbook.NewWriter(spec, true)
	if err != nil {
		return uuid.Nil, err
	}
	for _, r := range rows {
		if e := errs[r.Number]; len(e) > 0 {
			if err := w.Write(r, fmt.Sprintf("Baris %d: %s", r.Number, domain.FormatErrors(spec.Columns, e))); err != nil {
				return uuid.Nil, errors.Join(err, w.Discard())
			}
		}
	}
	var buf bytes.Buffer
	if err := w.Close(&buf); err != nil {
		return uuid.Nil, err
	}
	return s.Files.Save(ctx, ownerErrors+j.Kind, j.ID, "Baris Error "+spec.Label+".xlsx", workbook.ContentType, buf.Bytes())
}

// rowFailure adalah baris yang ditolak saat simpan (data berubah setelah pemeriksaan).
type rowFailure struct {
	row  int
	errs []domain.RowError
}

func (e *rowFailure) Error() string { return fmt.Sprintf("baris %d ditolak saat simpan", e.row) }

func batchSize(imp Importer) int {
	if b, ok := imp.(BatchImporter); ok && b.BatchSize() > 0 {
		return b.BatchSize()
	}
	return 0
}

func (s *Service) commit(ctx context.Context, j domain.ImportJob, imp Importer) error {
	spec := imp.Spec()
	validRows, err := s.Store.ValidRowNumbers(ctx, j.ID)
	if err != nil {
		return err
	}
	sheet, err := s.readFile(ctx, j, spec)
	if err != nil {
		if appErr := apperr.As(err); appErr.Code != apperr.CodeInternal {
			return s.failImport(ctx, j, appErr.Message)
		}
		return err
	}
	rows := make([]tabular.Row, 0, len(validRows))
	for _, r := range sheet.Rows {
		if validRows[r.Number] {
			rows = append(rows, r)
		}
	}
	size := batchSize(imp)
	if size == 0 {
		size = len(rows)
	}
	// Mode batch melanjutkan dari batch terakhir yang sudah tersimpan saat job diulang.
	done := j.CommittedRows
	if batchSize(imp) == 0 {
		done = 0
	}
	for start := done; start < len(rows); start += size {
		batch := rows[start:min(start+size, len(rows))]
		err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
			for i, row := range batch {
				if err := imp.ApplyRow(withRowKey(ctx, j.ID, row.Number), row); err != nil {
					rowErrs, sysErr := rowErrors(err, spec)
					if sysErr != nil {
						return fmt.Errorf("baris %d: %w", row.Number, sysErr)
					}
					return &rowFailure{row: row.Number, errs: rowErrs}
				}
				if n := start + i + 1; n%progressEvery == 0 {
					if err := s.Store.SetImportProgress(db.WithoutTx(ctx), j.ID, j.ProcessedRows, n, s.now()); err != nil {
						return err
					}
				}
			}
			if batchSize(imp) > 0 {
				return s.Store.SetImportProgress(ctx, j.ID, j.ProcessedRows, start+len(batch), s.now())
			}
			return nil
		})
		var rf *rowFailure
		if errors.As(err, &rf) {
			return s.failCommit(ctx, j, rf, start, spec)
		}
		if err != nil {
			return err
		}
	}
	now := s.now()
	cur, err := s.Store.GetImportJob(ctx, j.ID, false)
	if err != nil {
		return err
	}
	cur.Status, cur.CommittedRows, cur.CompletedAt = domain.ImportCompleted, len(rows), &now
	if cur, err = s.Store.SaveImportJob(ctx, cur, now, j.CreatedBy); err != nil {
		return err
	}
	s.notify(ctx, j.CreatedBy, notifyImportCompleted, "Impor selesai: "+j.FileName,
		fmt.Sprintf("%d baris %s tersimpan.", cur.CommittedRows, spec.Label), importLink(cur))
	return nil
}

func (s *Service) failCommit(ctx context.Context, j domain.ImportJob, rf *rowFailure, committed int, spec domain.Spec) error {
	if err := s.Store.SetRowErrors(ctx, j.ID, rf.row, rf.errs); err != nil {
		return err
	}
	cur, err := s.Store.GetImportJob(ctx, j.ID, false)
	if err != nil {
		return err
	}
	cur.CommittedRows = committed
	msg := fmt.Sprintf("Baris %d: %s. ", rf.row, domain.FormatErrors(spec.Columns, rf.errs))
	if committed == 0 {
		msg += "Tidak ada data yang disimpan. Perbaiki file lalu impor ulang."
	} else {
		msg += fmt.Sprintf("%d baris sebelumnya sudah tersimpan.", committed)
	}
	return s.failImport(ctx, cur, msg)
}
