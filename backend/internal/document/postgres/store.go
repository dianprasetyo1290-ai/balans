// Package postgres adalah repository modul document.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/document/domain"
	"balans/internal/document/postgres/sqlcgen"
	"balans/internal/platform/db"
)

// Store menyimpan sequence nomor dokumen.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Next menaikkan nomor urut kode dokumen pada tahun tertentu di transaksi aktif.
func (s *Store) Next(ctx context.Context, t domain.Type, year int) (int64, error) {
	n, err := s.q(ctx).NextNumber(ctx, sqlcgen.NextNumberParams{DocumentType: string(t), Year: int32(year)}) //nolint:gosec // tahun 4 digit
	if err != nil {
		return 0, fmt.Errorf("next document number %s/%d: %w", t, year, err)
	}
	return n, nil
}

// List mengembalikan sequence yang sudah dipakai pada tahun tertentu.
func (s *Store) List(ctx context.Context, year int) ([]domain.Sequence, error) {
	rows, err := s.q(ctx).ListSequences(ctx, int32(year)) //nolint:gosec // tahun 4 digit
	if err != nil {
		return nil, fmt.Errorf("list document sequences: %w", err)
	}
	out := make([]domain.Sequence, len(rows))
	for i, r := range rows {
		updated := r.UpdatedAt
		out[i] = domain.Sequence{Type: domain.Type(r.DocumentType), Year: int(r.Year), LastNumber: r.LastNumber, UpdatedAt: &updated}
	}
	return out, nil
}
