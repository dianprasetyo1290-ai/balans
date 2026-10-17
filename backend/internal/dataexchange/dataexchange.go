// Package dataexchange adalah API publik impor/ekspor Excel (BRD MD-10, ACC-10, INT-02, Technical Guideline
// 5.15). Modul ini generik dan tidak mengimpor modul bisnis: setiap jenis data didaftarkan saat wiring lewat
// RegisterImporter/RegisterExporter oleh adapter di internal/app yang memanggil fungsi publik modul pemilik data.
//
// Alur impor: unduh template → unggah xlsx (filestore, ownerType UploadOwnerType(kind)) → CreateImport →
// job dataexchange.import memeriksa per baris (dry-run di transaksi yang dibatalkan) → preview valid/error dan
// file baris error → user konfirmasi → job yang sama menyimpan baris valid. File yang sama (SHA-256) tidak bisa
// diimpor dua kali per jenis data. Ekspor: job dataexchange.export menulis xlsx ke filestore lalu memberi
// notifikasi berisi tautan unduhan.
package dataexchange

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/dataexchange/domain"
	dxhttp "balans/internal/dataexchange/http"
	"balans/internal/dataexchange/postgres"
	"balans/internal/dataexchange/service"
	"balans/internal/dataexchange/workbook"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "dataexchange"

type (
	// Spec adalah definisi jenis data: kind (= resource permission), label, nama sheet, dan kolom.
	Spec = domain.Spec
	// Importer adalah jenis data yang bisa diimpor.
	Importer = service.Importer
	// RowValidator opsional: memeriksa baris tanpa menulis.
	RowValidator = service.RowValidator
	// BatchImporter opsional: menyimpan per batch (impor transaksi).
	BatchImporter = service.BatchImporter
	// Exporter adalah jenis data yang bisa diekspor.
	Exporter = service.Exporter
	// Files adalah akses ke modul filestore.
	Files = service.Files
	// FileInfo adalah metadata file.
	FileInfo = service.FileInfo
	// Actors menyusun actor pembuat job saat worker berjalan.
	Actors = service.Actors
	// Notifier mengirim notifikasi in-app.
	Notifier = service.Notifier
	// Enqueuer memasukkan job river di transaksi aktif.
	Enqueuer = service.Enqueuer
)

// Batas teknis file impor.
const (
	MaxFileBytes = domain.MaxFileBytes
	MaxRows      = domain.MaxRows
	// ContentTypeXLSX adalah tipe MIME file xlsx.
	ContentTypeXLSX = workbook.ContentType
)

// RowKey adalah kunci idempotensi baris (ID job + nomor baris) untuk importer batch.
var RowKey = service.RowKey

// UploadOwnerType adalah jenis pemilik file upload impor di filestore, mis. `import.catalog.product`.
func UploadOwnerType(kind string) string { return service.UploadOwnerType(kind) }

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Jobs      Enqueuer
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Files     Files
	Actors    Actors
	// Notifier nil = tanpa notifikasi in-app.
	Notifier Notifier
}

// Module adalah modul dataexchange.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Jobs: d.Jobs, Files: d.Files, Actors: d.Actors,
		Notifier: d.Notifier, Clock: d.Clock, Log: d.Log,
	})
	return &Module{svc: svc, rs: d.Responder}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route jenis data, template, job impor, dan job ekspor.
func (m *Module) RegisterRoutes(r chi.Router) { dxhttp.Register(r, dxhttp.NewHandler(m.svc), m.rs) }

// RegisterWorkers mendaftarkan worker dataexchange.import dan dataexchange.export (antrean bulk).
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewImportWorker(m.svc))
	river.AddWorker(w, service.NewExportWorker(m.svc))
}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// RegisterImporter mendaftarkan jenis data impor (saat wiring).
func (m *Module) RegisterImporter(imp Importer) { m.svc.RegisterImporter(imp) }

// RegisterExporter mendaftarkan jenis data ekspor (saat wiring).
func (m *Module) RegisterExporter(exp Exporter) { m.svc.RegisterExporter(exp) }

// RunImport menjalankan satu langkah job impor (pemeriksaan atau penyimpanan) secara sinkron; dipakai worker
// dan test.
func (m *Module) RunImport(ctx context.Context, jobID uuid.UUID) error {
	return m.svc.RunImport(ctx, jobID)
}

// RunExport menjalankan job ekspor secara sinkron; dipakai worker dan test.
func (m *Module) RunExport(ctx context.Context, jobID uuid.UUID) error {
	return m.svc.RunExport(ctx, jobID)
}
