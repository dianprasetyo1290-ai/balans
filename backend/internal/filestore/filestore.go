// Package filestore adalah API publik penyimpanan file (Technical Guideline 5.16): lampiran PKS, foto QC,
// bukti bayar, bukti biaya. Modul pemilik mendaftarkan jenis pemilik (permission, tipe, ukuran) lewat
// RegisterOwner, lalu menautkan file yang sudah diunggah ke datanya lewat Attach di transaksi bisnis.
package filestore

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/filestore/domain"
	filehttp "balans/internal/filestore/http"
	"balans/internal/filestore/localdisk"
	"balans/internal/filestore/postgres"
	"balans/internal/filestore/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "filestore"

type (
	// File adalah metadata file.
	File = domain.File
	// OwnerPolicy adalah aturan file untuk satu jenis pemilik.
	OwnerPolicy = domain.OwnerPolicy
)

// Tipe file yang umum dipakai pemilik.
const (
	TypePDF  = domain.TypePDF
	TypeJPEG = domain.TypeJPEG
	TypePNG  = domain.TypePNG
	TypeWebP = domain.TypeWebP
)

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	// Dir adalah folder penyimpanan lokal (FILESTORE_DIR).
	Dir string
	// Sign menghitung HMAC untuk URL bertanda tangan.
	Sign func(msg string) []byte
}

// Module adalah modul filestore.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
}

// New menyusun modul dengan penyimpanan disk lokal.
func New(d Deps) (*Module, error) {
	disk, err := localdisk.New(d.Dir)
	if err != nil {
		return nil, err
	}
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Storage: disk, Clock: d.Clock, Log: d.Log, Sign: d.Sign,
	})
	return &Module{svc: svc, rs: d.Responder}, nil
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route upload, metadata, dan unduhan.
func (m *Module) RegisterRoutes(r chi.Router) { filehttp.Register(r, filehttp.NewHandler(m.svc), m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// RegisterOwner didaftarkan modul pemilik file saat wiring.
func (m *Module) RegisterOwner(p OwnerPolicy) { m.svc.RegisterOwner(p) }

// Attach menautkan file yang sudah diunggah actor ke pemiliknya di transaksi aktif. File dengan jenis pemilik
// lain, belum selesai diunggah, atau sudah dipakai data lain ditolak VALIDATION_FAILED pada field `fileId`.
func (m *Module) Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	_, err := m.svc.Attach(ctx, fileID, ownerType, ownerID)
	return err
}

// Names mengembalikan nama asli file untuk ditampilkan.
func (m *Module) Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return m.svc.Names(ctx, ids)
}

// IsContentPath menandai URL isi file bertanda tangan: tidak memakai access token maupun Idempotency-Key.
func IsContentPath(path string) bool { return strings.HasPrefix(path, filehttp.ContentPathPrefix) }
