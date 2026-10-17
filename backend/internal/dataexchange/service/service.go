// Package service berisi use case impor/ekspor Excel generik (BRD MD-10, INT-02, Technical Guideline 5.15).
// Modul bisnis tidak diimpor: jenis data didaftarkan lewat Importer/Exporter yang diisi adapter di internal/app,
// sehingga validasi bisnis tetap di modul pemilik data.
package service

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/dataexchange/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

// Importer adalah satu jenis data yang bisa diimpor. ApplyRow membuat atau memperbarui data satu baris di
// transaksi aktif dengan aturan bisnis modul pemilik (termasuk cek permission tulis actor). ApplyRow dipanggil
// dua kali: saat pemeriksaan (di savepoint yang selalu dibatalkan, sehingga hasilnya sama dengan simpan
// sebenarnya) dan saat simpan. Error apperr (VALIDATION_FAILED dengan field = Key kolom, DUPLICATE_CODE, dst.)
// menjadi error baris; error lain dianggap gangguan sistem dan job diulang.
type Importer interface {
	Spec() domain.Spec
	ApplyRow(ctx context.Context, row tabular.Row) error
}

// RowValidator opsional: importer yang tidak boleh menulis saat pemeriksaan (mis. kode dari sequence yang
// tidak ikut dibatalkan) memeriksa baris tanpa menyimpan. Kolom Identity tetap dicek ganda oleh framework.
type RowValidator interface {
	ValidateRow(ctx context.Context, row tabular.Row) error
}

// BatchImporter opsional: BatchSize > 0 menyimpan per batch dengan transaksi terpisah (impor transaksi besar,
// TG 5.15 butir 4). ApplyRow wajib idempotent per baris; gunakan RowKey sebagai kunci idempotensi. Default
// (tanpa interface ini): satu impor = satu transaksi (master data).
type BatchImporter interface {
	BatchSize() int
}

// Exporter adalah satu jenis data yang bisa diekspor. Export memanggil emit per baris secara berurutan (stream)
// dengan Key kolom yang sama dengan template impor, sehingga file ekspor bisa diimpor ulang.
type Exporter interface {
	Spec() domain.Spec
	Export(ctx context.Context, emit func(tabular.Row) error) error
}

type rowKeyCtx struct{}

// RowKey adalah kunci idempotensi baris: ID job + nomor baris. Kosong di luar job impor.
func RowKey(ctx context.Context) string {
	s, _ := ctx.Value(rowKeyCtx{}).(string)
	return s
}

// FileInfo adalah metadata file dari modul filestore.
type FileInfo struct {
	ID        uuid.UUID
	Name      string
	OwnerType string
	SHA256    []byte
	Uploaded  bool
	CreatedBy uuid.UUID
}

// Files adalah akses ke modul filestore.
type Files interface {
	Info(ctx context.Context, id uuid.UUID) (FileInfo, error)
	Attach(ctx context.Context, id uuid.UUID, ownerType string, ownerID uuid.UUID) error
	Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)
	// Save menyimpan file buatan server (hasil ekspor, baris error) yang langsung tertaut ke pemiliknya.
	Save(ctx context.Context, ownerType string, ownerID uuid.UUID, name, contentType string, data []byte) (uuid.UUID, error)
	// DownloadURL membuat URL unduhan bertanda tangan tanpa cek permission; dataexchange sudah memeriksanya.
	DownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error)
}

// Actors menyusun actor pembuat job saat worker berjalan (permission terbaru, untuk cek hak dan audit trail).
type Actors interface {
	ActorFor(ctx context.Context, userID uuid.UUID) (actor.Actor, error)
}

// Notifier mengirim notifikasi in-app (opsional).
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// Enqueuer memasukkan job river di transaksi aktif.
type Enqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	InsertImportJob(ctx context.Context, j domain.ImportJob, actorID uuid.UUID) (domain.ImportJob, error)
	GetImportJob(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ImportJob, error)
	ListImportJobs(ctx context.Context, kinds []string, kind *string, page paging.Page) (paging.Result[domain.ImportJob], error)
	SaveImportJob(ctx context.Context, j domain.ImportJob, now time.Time, actorID uuid.UUID) (domain.ImportJob, error)
	SetImportProgress(ctx context.Context, id uuid.UUID, processed, committed int, now time.Time) error
	ReplaceImportRows(ctx context.Context, jobID uuid.UUID, rows []domain.RowResult) error
	ListImportRows(ctx context.Context, jobID uuid.UUID, valid *bool, page paging.Page) (paging.Result[domain.RowResult], error)
	ValidRowNumbers(ctx context.Context, jobID uuid.UUID) (map[int]bool, error)
	SetRowErrors(ctx context.Context, jobID uuid.UUID, row int, errs []domain.RowError) error

	InsertExportJob(ctx context.Context, kind string, actorID uuid.UUID) (domain.ExportJob, error)
	GetExportJob(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ExportJob, error)
	ListExportJobs(ctx context.Context, kinds []string, kind *string, page paging.Page) (paging.Result[domain.ExportJob], error)
	SaveExportJob(ctx context.Context, j domain.ExportJob, now time.Time, actorID uuid.UUID) (domain.ExportJob, error)
}

// Deps adalah dependensi Service.
type Deps struct {
	Store    store
	Tx       txRunner
	Jobs     Enqueuer
	Files    Files
	Actors   Actors
	Notifier Notifier
	Clock    clock.Clock
	Log      *slog.Logger
}

// Service adalah use case dataexchange.
type Service struct {
	Deps

	mu        sync.RWMutex
	importers map[string]Importer
	exporters map[string]Exporter
}

// New membuat Service.
func New(d Deps) *Service {
	return &Service{Deps: d, importers: map[string]Importer{}, exporters: map[string]Exporter{}}
}

// Jenis pemilik file di filestore per jenis data.
const (
	ownerUpload = "import."
	ownerErrors = "import_errors."
	ownerExport = "export."
)

// UploadOwnerType adalah jenis pemilik file upload impor, mis. `import.catalog.product`.
func UploadOwnerType(kind string) string { return ownerUpload + kind }

// RegisterImporter didaftarkan saat wiring; pendaftaran terakhir untuk kind yang sama menang.
func (s *Service) RegisterImporter(imp Importer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.importers[imp.Spec().Kind] = imp
}

// RegisterExporter didaftarkan saat wiring.
func (s *Service) RegisterExporter(exp Exporter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exporters[exp.Spec().Kind] = exp
}

func (s *Service) importer(kind string) (Importer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	imp, ok := s.importers[kind]
	return imp, ok
}

func (s *Service) exporter(kind string) (Exporter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp, ok := s.exporters[kind]
	return exp, ok
}

// KindInfo adalah jenis data yang tersedia untuk actor.
type KindInfo struct {
	Spec      domain.Spec
	CanImport bool
	CanExport bool
}

// Kinds mengembalikan jenis data yang boleh diimpor atau diekspor actor, urut label.
func (s *Service) Kinds(ctx context.Context) ([]KindInfo, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return nil, apperr.New(apperr.CodeAuthSessionExpired)
	}
	s.mu.RLock()
	byKind := map[string]*KindInfo{}
	for k, imp := range s.importers {
		if a.Has(imp.Spec().ImportPermission()) {
			byKind[k] = &KindInfo{Spec: imp.Spec(), CanImport: true}
		}
	}
	for k, exp := range s.exporters {
		if !a.Has(exp.Spec().ExportPermission()) {
			continue
		}
		if info, ok := byKind[k]; ok {
			info.CanExport = true
		} else {
			byKind[k] = &KindInfo{Spec: exp.Spec(), CanExport: true}
		}
	}
	s.mu.RUnlock()
	out := make([]KindInfo, 0, len(byKind))
	for _, info := range byKind {
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Label < out[j].Spec.Label })
	return out, nil
}

// importKinds mengembalikan jenis data yang boleh diimpor actor (filter riwayat).
func (s *Service) importKinds(a actor.Actor) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for k, imp := range s.importers {
		if a.Has(imp.Spec().ImportPermission()) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func (s *Service) exportKinds(a actor.Actor) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for k, exp := range s.exporters {
		if a.Has(exp.Spec().ExportPermission()) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// KindLabel mengembalikan label jenis data untuk tampilan; jenis yang tidak terdaftar memakai kind-nya.
func (s *Service) KindLabel(kind string) string {
	if imp, ok := s.importer(kind); ok {
		return imp.Spec().Label
	}
	if exp, ok := s.exporter(kind); ok {
		return exp.Spec().Label
	}
	return kind
}

func unknownKind() error {
	return apperr.Validation(map[string]string{"kind": "Jenis data tidak dikenal"})
}

// requireImporter memeriksa jenis data dan permission `<kind>.import`.
func (s *Service) requireImporter(ctx context.Context, kind string) (Importer, actor.Actor, error) {
	imp, ok := s.importer(kind)
	if !ok {
		return nil, actor.Actor{}, unknownKind()
	}
	a, err := actor.Require(ctx, imp.Spec().ImportPermission())
	return imp, a, err
}

func (s *Service) requireExporter(ctx context.Context, kind string) (Exporter, actor.Actor, error) {
	exp, ok := s.exporter(kind)
	if !ok {
		return nil, actor.Actor{}, unknownKind()
	}
	a, err := actor.Require(ctx, exp.Spec().ExportPermission())
	return exp, a, err
}

func (s *Service) notify(ctx context.Context, userID uuid.UUID, kind, title, body, link string) {
	if s.Notifier == nil || userID == uuid.Nil {
		return
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		return s.Notifier.Notify(ctx, []uuid.UUID{userID}, kind, title, body, link)
	})
	if err != nil && s.Log != nil {
		s.Log.WarnContext(ctx, "dataexchange notify failed", "kind", kind, "error", err)
	}
}

func (s *Service) now() time.Time { return s.Clock.Now() }
