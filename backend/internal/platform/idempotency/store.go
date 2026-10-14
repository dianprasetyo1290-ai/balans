// Package idempotency menjalankan header Idempotency-Key untuk request yang mengubah data (Technical Guideline 7.1).
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/platform/db"
	"balans/internal/platform/idempotency/sqlcgen"
)

// Status penyimpanan key.
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// Record adalah satu Idempotency-Key yang tersimpan.
type Record struct {
	UserID       uuid.UUID
	Key          uuid.UUID
	Method       string
	Path         string
	RequestHash  []byte
	Status       string
	ResponseCode int
	ResponseBody []byte
	ExpiresAt    time.Time
}

// Store menyimpan key di tabel idempotency_keys.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Claim menyimpan key berstatus processing. Jika key sudah ada, record yang tersimpan dikembalikan
// dengan claimed bernilai false.
func (s *Store) Claim(ctx context.Context, rec Record) (existing Record, claimed bool, err error) {
	inserted, err := s.q(ctx).InsertIdempotencyKey(ctx, sqlcgen.InsertIdempotencyKeyParams{
		UserID:        rec.UserID,
		Key:           rec.Key,
		RequestMethod: rec.Method,
		RequestPath:   rec.Path,
		RequestHash:   rec.RequestHash,
		ExpiresAt:     rec.ExpiresAt,
	})
	if err != nil {
		return Record{}, false, fmt.Errorf("insert idempotency key: %w", err)
	}
	if inserted == 1 {
		return Record{}, true, nil
	}

	row, err := s.q(ctx).GetIdempotencyKey(ctx, sqlcgen.GetIdempotencyKeyParams{UserID: rec.UserID, Key: rec.Key})
	if errors.Is(err, pgx.ErrNoRows) {
		// Key dihapus di antara insert dan select (request sebelumnya gagal 5xx); anggap masih diproses.
		return Record{UserID: rec.UserID, Key: rec.Key, Status: StatusProcessing, RequestHash: rec.RequestHash}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("get idempotency key: %w", err)
	}
	return Record{
		UserID:       row.UserID,
		Key:          row.Key,
		Method:       row.RequestMethod,
		Path:         row.RequestPath,
		RequestHash:  row.RequestHash,
		Status:       row.Status,
		ResponseCode: int(row.ResponseCode.Int32),
		ResponseBody: row.ResponseBody,
		ExpiresAt:    row.ExpiresAt,
	}, false, nil
}

// Complete menyimpan response final.
func (s *Store) Complete(ctx context.Context, userID, key uuid.UUID, code int, body []byte) error {
	if err := s.q(ctx).CompleteIdempotencyKey(ctx, sqlcgen.CompleteIdempotencyKeyParams{
		UserID:       userID,
		Key:          key,
		ResponseCode: pgtype.Int4{Int32: int32(code), Valid: true}, //nolint:gosec // status HTTP selalu < 600
		ResponseBody: body,
	}); err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

// Release menghapus key agar request bisa diulang setelah error sementara.
func (s *Store) Release(ctx context.Context, userID, key uuid.UUID) error {
	if err := s.q(ctx).DeleteIdempotencyKey(ctx, sqlcgen.DeleteIdempotencyKeyParams{UserID: userID, Key: key}); err != nil {
		return fmt.Errorf("release idempotency key: %w", err)
	}
	return nil
}

// DeleteExpired menghapus key yang sudah lewat masa berlaku, dipanggil job harian.
func (s *Store) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	n, err := s.q(ctx).DeleteExpiredIdempotencyKeys(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency keys: %w", err)
	}
	return n, nil
}
