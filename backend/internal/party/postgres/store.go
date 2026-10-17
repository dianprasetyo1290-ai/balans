// Package postgres adalah repository modul party. Kolom terenkripsi (NPWP, nomor rekening) dienkripsi dan
// didekripsi di sini sehingga domain dan service hanya melihat nilai asli (Technical Guideline 6.7).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/party/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
)

// Cipher mengenkripsi data pribadi (platform/pii).
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Store menyimpan pelanggan, mitra, PKS, kode produk mitra, dan pemasok.
type Store struct {
	pool   *pgxpool.Pool
	cipher Cipher
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool, cipher Cipher) *Store { return &Store{pool: pool, cipher: cipher} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func (s *Store) encrypt(v string) (string, error) {
	enc, err := s.cipher.Encrypt(v)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}
	return enc, nil
}

func (s *Store) decrypt(v string) (string, error) {
	plain, err := s.cipher.Decrypt(v)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}

func v32(n int) int32 { return int32(n) } //nolint:gosec // versi, termin, dan offset kecil

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}

func decimalPtr(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	v := d.Decimal
	return &v
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// violation mengembalikan nama constraint untuk pelanggaran unik (23505) atau EXCLUDE (23P01).
func violation(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23P01") {
		return pgErr.ConstraintName
	}
	return ""
}

func versionConflict(n int64, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
