// Package service berisi use case penomoran dokumen (U-15, Technical Guideline 5.12).
package service

import (
	"context"
	"errors"

	"balans/internal/document/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
)

// PermSequenceRead adalah permission melihat Pengaturan › Nomor Dokumen.
const PermSequenceRead = "document.sequence.read"

// ErrNoTransaction dikembalikan jika Next dipanggil di luar transaksi bisnis. Nomor yang diambil di luar
// transaksi tidak ikut rollback, sehingga bisa loncat.
var ErrNoTransaction = errors.New("document: nomor dokumen harus diambil di dalam transaksi bisnis")

type store interface {
	Next(ctx context.Context, t domain.Type, year int) (int64, error)
	List(ctx context.Context, year int) ([]domain.Sequence, error)
}

// Service mengelola sequence nomor dokumen.
type Service struct {
	Store store
	Clock clock.Clock
}

// New membuat Service.
func New(st store, c clock.Clock) *Service { return &Service{Store: st, Clock: c} }

// Next mengambil nomor berikutnya untuk kode dokumen. Wajib dipanggil di dalam transaksi pemanggil
// (Tx.WithTx) agar nomor kembali jika transaksi gagal. Tahun dan bulan diambil dari tanggal bisnis.
func (s *Service) Next(ctx context.Context, t domain.Type, date clock.BusinessDate) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	if _, ok := db.Tx(ctx); !ok {
		return "", ErrNoTransaction
	}
	n, err := s.Store.Next(ctx, t, date.Year())
	if err != nil {
		return "", err
	}
	return domain.Format(t, date, n), nil
}

// Row adalah satu baris halaman Nomor Dokumen.
type Row struct {
	Info     domain.TypeInfo
	Sequence domain.Sequence
}

// List mengembalikan semua kode dokumen beserta nomor terakhir pada tahun tertentu (0 = tahun berjalan).
func (s *Service) List(ctx context.Context, year int) (int, []Row, error) {
	if _, err := actor.Require(ctx, PermSequenceRead); err != nil {
		return 0, nil, err
	}
	if year == 0 {
		year = clock.Today(s.Clock).Year()
	}
	seqs, err := s.Store.List(ctx, year)
	if err != nil {
		return 0, nil, err
	}
	byType := make(map[domain.Type]domain.Sequence, len(seqs))
	for _, sq := range seqs {
		byType[sq.Type] = sq
	}
	rows := make([]Row, 0, len(domain.Types()))
	for _, info := range domain.Types() {
		sq, ok := byType[info.Type]
		if !ok {
			sq = domain.Sequence{Type: info.Type, Year: year}
		}
		rows = append(rows, Row{Info: info, Sequence: sq})
	}
	return year, rows, nil
}
