// Package domain berisi aturan impor/ekspor Excel (BRD MD-10, INT-02, Technical Guideline 5.15): status job,
// pencocokan header, pemeriksaan kolom wajib dan kunci ganda, pemetaan error per baris, dan penyamaran data
// pribadi di preview. Tanpa DB, HTTP, maupun library Excel.
package domain

import (
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
)

// ImportStatus adalah status job impor.
type ImportStatus string

const (
	// ImportUploaded: file diterima, validasi menunggu giliran di antrean bulk.
	ImportUploaded ImportStatus = "uploaded"
	// ImportValidating: job memeriksa baris (dry-run).
	ImportValidating ImportStatus = "validating"
	// ImportValidated: hasil pemeriksaan siap dipreview; menunggu konfirmasi user.
	ImportValidated ImportStatus = "validated"
	// ImportCommitting: baris valid sedang disimpan.
	ImportCommitting ImportStatus = "committing"
	ImportCompleted  ImportStatus = "completed"
	ImportFailed     ImportStatus = "failed"
	// ImportCancelled: user membatalkan setelah melihat preview; file boleh diunggah ulang.
	ImportCancelled ImportStatus = "cancelled"
)

// Running bernilai true selama job masih diproses worker (FE menampilkan progress).
func (s ImportStatus) Running() bool {
	return s == ImportUploaded || s == ImportValidating || s == ImportCommitting
}

// ImportJob adalah satu file impor beserta hasil pemeriksaannya.
type ImportJob struct {
	ID            uuid.UUID
	Kind          string
	FileID        uuid.UUID
	FileName      string
	FileSHA256    []byte
	Status        ImportStatus
	TotalRows     int
	ProcessedRows int
	ValidRows     int
	ErrorRows     int
	CommittedRows int
	ErrorFileID   *uuid.UUID
	// Message adalah alasan gagal yang ditampilkan ke user.
	Message     string
	ValidatedAt *time.Time
	CompletedAt *time.Time
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// Progress mengembalikan persentase 0–100 tahap yang sedang berjalan.
func (j ImportJob) Progress() int {
	switch j.Status {
	case ImportValidated, ImportCompleted, ImportFailed, ImportCancelled:
		return 100
	case ImportCommitting:
		return percent(j.CommittedRows, j.ValidRows)
	case ImportValidating:
		return percent(j.ProcessedRows, j.TotalRows)
	}
	return 0
}

func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return min(100, done*100/total)
}

// Commit memindahkan job ke status committing. Hanya hasil pemeriksaan dengan minimal satu baris valid.
func (j *ImportJob) Commit() error {
	if j.Status != ImportValidated {
		return apperr.InvalidStateTransition("import_job", string(j.Status), string(ImportCommitting))
	}
	if j.ValidRows == 0 {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tidak ada baris valid untuk disimpan.")
	}
	j.Status = ImportCommitting
	return nil
}

// Cancel membatalkan job yang menunggu konfirmasi.
func (j *ImportJob) Cancel() error {
	if j.Status != ImportValidated {
		return apperr.InvalidStateTransition("import_job", string(j.Status), string(ImportCancelled))
	}
	j.Status = ImportCancelled
	return nil
}

// AllowedActions untuk tombol di ERP.
func (j ImportJob) AllowedActions() []string {
	out := []string{}
	if j.Status == ImportValidated {
		if j.ValidRows > 0 {
			out = append(out, "commit")
		}
		out = append(out, "cancel")
	}
	if j.ErrorFileID != nil {
		out = append(out, "download_errors")
	}
	return out
}

// ExportStatus adalah status job ekspor.
type ExportStatus string

const (
	ExportQueued    ExportStatus = "queued"
	ExportRunning   ExportStatus = "running"
	ExportCompleted ExportStatus = "completed"
	ExportFailed    ExportStatus = "failed"
)

// ExportJob adalah permintaan ekspor yang menghasilkan file xlsx di filestore.
type ExportJob struct {
	ID          uuid.UUID
	Kind        string
	Status      ExportStatus
	TotalRows   int
	FileID      *uuid.UUID
	FileName    string
	Message     string
	CompletedAt *time.Time
	// Params adalah parameter ekspor laporan (periode, filter); kosong untuk ekspor master.
	Params    map[string]string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// Spec adalah definisi satu jenis data impor/ekspor.
type Spec struct {
	// Kind memakai nama resource permission, mis. `catalog.product`.
	Kind  string
	Label string
	// Sheet adalah nama sheet data di template (mis. `Produk`).
	Sheet   string
	Columns []tabular.Column
	// Resource opsional: resource permission bila berbeda dari Kind (mis. impor settlement per marketplace
	// `settlement.shopee` memakai `accounting.settlement`).
	Resource string
	// ExportPerm opsional: permission ekspor penuh (mis. laporan `accounting.report.read`) menggantikan
	// `<resource>.export`.
	ExportPerm string
}

func (s Spec) resource() string {
	if s.Resource != "" {
		return s.Resource
	}
	return s.Kind
}

// ImportPermission adalah `<resource>.import`.
func (s Spec) ImportPermission() string { return s.resource() + ".import" }

// ExportPermission adalah `<resource>.export` atau ExportPerm.
func (s Spec) ExportPermission() string {
	if s.ExportPerm != "" {
		return s.ExportPerm
	}
	return s.resource() + ".export"
}

// FileName adalah nama file template/ekspor, mis. `Produk.xlsx`.
func (s Spec) FileName(prefix string) string { return prefix + " " + s.Label + ".xlsx" }

// Batas teknis file impor (bukan nilai bisnis): ukuran upload dan jumlah baris per file.
const (
	MaxFileBytes = 10 << 20
	MaxRows      = 20000
)
