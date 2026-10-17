// Package service berisi use case file: meminta URL upload, menerima isi file, menautkan file ke pemiliknya,
// dan membuat URL unduhan setelah cek permission (Technical Guideline 5.16).
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"balans/internal/filestore/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Storage adalah penyimpanan isi file (disk lokal sekarang, object storage S3-compatible nanti).
type Storage interface {
	Put(ctx context.Context, key uuid.UUID, r io.Reader) (int64, []byte, error)
	Open(ctx context.Context, key uuid.UUID) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key uuid.UUID) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	Insert(ctx context.Context, f domain.File, actorID uuid.UUID) (domain.File, error)
	Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.File, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.File, error)
	MarkUploaded(ctx context.Context, f domain.File) error
	Attach(ctx context.Context, f domain.File, actorID uuid.UUID) error
}

// Deps adalah dependensi Service.
type Deps struct {
	Store   store
	Tx      txRunner
	Storage Storage
	Clock   clock.Clock
	Log     *slog.Logger
	// Sign menghitung HMAC untuk URL bertanda tangan.
	Sign domain.Signer
}

// Service adalah use case filestore.
type Service struct {
	Deps

	mu       sync.RWMutex
	policies map[string]domain.OwnerPolicy
}

// New membuat Service.
func New(d Deps) *Service {
	return &Service{Deps: d, policies: map[string]domain.OwnerPolicy{}}
}

// RegisterOwner didaftarkan modul pemilik file saat wiring.
func (s *Service) RegisterOwner(p domain.OwnerPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[p.OwnerType] = p
}

func (s *Service) policy(ownerType string) (domain.OwnerPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[ownerType]
	return p, ok
}

// Ticket adalah URL bertanda tangan beserta masa berlakunya.
type Ticket struct {
	URL       string
	ExpiresAt time.Time
}

// CreateUpload memvalidasi permintaan upload dan mengembalikan URL PUT bertanda tangan.
func (s *Service) CreateUpload(ctx context.Context, in domain.UploadInput) (domain.File, Ticket, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return domain.File{}, Ticket{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	p, ok := s.policy(in.OwnerType)
	if !ok {
		return domain.File{}, Ticket{}, apperr.Validation(map[string]string{"ownerType": "Jenis pemilik file tidak dikenal"})
	}
	if !a.Has(p.WritePermission) {
		return domain.File{}, Ticket{}, apperr.New(apperr.CodeForbidden)
	}
	f, err := domain.NewUpload(in, p)
	if err != nil {
		return domain.File{}, Ticket{}, err
	}
	if f, err = s.Store.Insert(ctx, f, a.UserID); err != nil {
		return domain.File{}, Ticket{}, err
	}
	exp := s.Clock.Now().Add(domain.UploadURLTTL)
	return f, Ticket{URL: domain.ContentURL(s.Sign, domain.PurposeUpload, f.ID, exp), ExpiresAt: exp}, nil
}

// MaxBytes mengembalikan batas ukuran pemilik.
func (s *Service) MaxBytes(ownerType string) int64 {
	p, _ := s.policy(ownerType)
	return p.MaxBytes
}

// sniffWriter menyimpan 512 byte pertama untuk deteksi tipe isi file.
type sniffWriter struct{ buf bytes.Buffer }

func (w *sniffWriter) Write(p []byte) (int, error) {
	if room := 512 - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// Upload menerima isi file dari URL bertanda tangan. Tidak membutuhkan login: tanda tangan URL yang
// membuktikan izin (setara presigned PUT).
func (s *Service) Upload(ctx context.Context, id uuid.UUID, expires int64, sig string, body io.Reader) (domain.File, error) {
	if err := domain.Verify(s.Sign, domain.PurposeUpload, id, expires, sig, s.Clock.Now()); err != nil {
		return domain.File{}, err
	}
	f, err := s.Store.Get(ctx, id, false)
	if err != nil {
		return domain.File{}, err
	}
	if f.Status != domain.StatusPending {
		return domain.File{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "File sudah diunggah.")
	}
	sniff := &sniffWriter{}
	written, sum, err := s.Storage.Put(ctx, f.ObjectKey, io.TeeReader(io.LimitReader(body, f.SizeBytes+1), sniff))
	if err != nil {
		return domain.File{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if err := cur.MarkUploaded(written, sum, http.DetectContentType(sniff.buf.Bytes()), s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.MarkUploaded(ctx, cur); err != nil {
			return err
		}
		cur.Version++
		f = cur
		return nil
	})
	if err != nil {
		// File yang sudah diunggah oleh permintaan lain dibiarkan; selain itu isi yang tidak valid dibuang.
		if apperr.HasCode(err, apperr.CodeInvalidStateTransition) {
			return domain.File{}, err
		}
		if derr := s.Storage.Delete(context.WithoutCancel(ctx), f.ObjectKey); derr != nil && s.Log != nil {
			s.Log.WarnContext(ctx, "hapus file gagal upload", "file_id", id, "error", derr)
		}
		return domain.File{}, err
	}
	return f, nil
}

// Attach menautkan file ke pemiliknya di transaksi bisnis aktif. Dipanggil modul pemilik (mis. party untuk
// PDF PKS) setelah memeriksa permission tulis datanya sendiri.
func (s *Service) Attach(ctx context.Context, id uuid.UUID, ownerType string, ownerID uuid.UUID) (domain.File, error) {
	var f domain.File
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		f, err = s.Store.Get(ctx, id, true)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{"fileId": "File tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		changed, err := f.AttachTo(ownerType, ownerID, actor.UserID(ctx))
		if err != nil || !changed {
			return err
		}
		if err := s.Store.Attach(ctx, f, actor.UserID(ctx)); err != nil {
			return err
		}
		f.Version++
		return nil
	})
	return f, err
}

// Get mengembalikan metadata file yang boleh dibaca actor.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.File, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return domain.File{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	f, err := s.Store.Get(ctx, id, false)
	if err != nil {
		return domain.File{}, err
	}
	p, _ := s.policy(f.OwnerType)
	if !f.CanRead(a, p) {
		return domain.File{}, apperr.New(apperr.CodeForbidden)
	}
	return f, nil
}

// Names mengembalikan nama asli file untuk ditampilkan modul pemilik; file yang tidak ada dilewati.
func (s *Service) Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	files, err := s.Store.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		out[f.ID] = f.OriginalName
	}
	return out, nil
}

// DownloadURL membuat URL unduhan berumur 5 menit setelah cek permission.
func (s *Service) DownloadURL(ctx context.Context, id uuid.UUID) (Ticket, error) {
	f, err := s.Get(ctx, id)
	if err != nil {
		return Ticket{}, err
	}
	if f.Status != domain.StatusUploaded {
		return Ticket{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "File belum selesai diunggah.")
	}
	exp := s.Clock.Now().Add(domain.DownloadURLTTL)
	return Ticket{URL: domain.ContentURL(s.Sign, domain.PurposeDownload, f.ID, exp), ExpiresAt: exp}, nil
}

// Open membuka isi file dari URL unduhan bertanda tangan.
func (s *Service) Open(ctx context.Context, id uuid.UUID, expires int64, sig string) (domain.File, io.ReadSeekCloser, error) {
	if err := domain.Verify(s.Sign, domain.PurposeDownload, id, expires, sig, s.Clock.Now()); err != nil {
		return domain.File{}, nil, err
	}
	f, err := s.Store.Get(ctx, id, false)
	if err != nil {
		return domain.File{}, nil, err
	}
	if f.Status != domain.StatusUploaded {
		return domain.File{}, nil, apperr.New(apperr.CodeNotFound)
	}
	r, err := s.Storage.Open(ctx, f.ObjectKey)
	if err != nil {
		return domain.File{}, nil, fmt.Errorf("open file %s: %w", id, err)
	}
	return f, r, nil
}
