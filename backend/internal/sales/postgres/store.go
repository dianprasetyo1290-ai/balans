// Package postgres adalah repository modul sales.
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

	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

// Store menyimpan master dan dokumen penjualan.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func mapWriteError(err error, what string) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "uq_return_reasons_code" {
		return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: apperr.CodeDuplicateCode.DefaultMessage(),
			Fields: map[string]string{"code": "Kode sudah dipakai (termasuk data yang sudah dihapus)"}, Err: err}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// --- Alasan retur -----------------------------------------------------------------------------

func toReturnReason(r sqlcgen.ReturnReason) domain.ReturnReason {
	return domain.ReturnReason{
		ID: r.ID, Code: r.Code, Name: r.Name, AppliesTo: domain.ReturnScope(r.AppliesTo),
		DefaultDisposition: domain.Disposition(r.DefaultDisposition.String), Description: r.Description,
		IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ReturnReasonFilter adalah filter list alasan retur. AppliesTo kosong = semua.
type ReturnReasonFilter struct {
	IncludeInactive bool
	AppliesTo       domain.ReturnScope
	Query           string
}

// ListReturnReasons mengembalikan alasan retur urut kode.
func (s *Store) ListReturnReasons(ctx context.Context, f ReturnReasonFilter) ([]domain.ReturnReason, error) {
	rows, err := s.q(ctx).ListReturnReasons(ctx, sqlcgen.ListReturnReasonsParams{
		IncludeInactive: f.IncludeInactive, AppliesTo: text(string(f.AppliesTo)), Q: text(f.Query),
	})
	if err != nil {
		return nil, fmt.Errorf("list return reasons: %w", err)
	}
	out := make([]domain.ReturnReason, len(rows))
	for i, r := range rows {
		out[i] = toReturnReason(r)
	}
	return out, nil
}

// GetReturnReason membaca satu alasan retur yang belum dihapus; forUpdate mengunci baris.
func (s *Store) GetReturnReason(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ReturnReason, error) {
	var (
		r   sqlcgen.ReturnReason
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetReturnReasonForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetReturnReason(ctx, id)
	}
	if err != nil {
		return domain.ReturnReason{}, notFound(err, "get return reason")
	}
	return toReturnReason(r), nil
}

// GetReturnReasonByCode membaca alasan retur berdasarkan kode.
func (s *Store) GetReturnReasonByCode(ctx context.Context, code string) (domain.ReturnReason, error) {
	r, err := s.q(ctx).GetReturnReasonByCode(ctx, code)
	if err != nil {
		return domain.ReturnReason{}, notFound(err, "get return reason by code")
	}
	return toReturnReason(r), nil
}

// InsertReturnReason menyimpan alasan retur baru.
func (s *Store) InsertReturnReason(ctx context.Context, rr domain.ReturnReason, actorID uuid.UUID) (domain.ReturnReason, error) {
	r, err := s.q(ctx).InsertReturnReason(ctx, sqlcgen.InsertReturnReasonParams{
		Code: rr.Code, Name: rr.Name, AppliesTo: string(rr.AppliesTo), DefaultDisposition: text(string(rr.DefaultDisposition)),
		Description: rr.Description, IsActive: rr.IsActive, ActorID: actorID,
	})
	if err != nil {
		return domain.ReturnReason{}, mapWriteError(err, "insert return reason")
	}
	return toReturnReason(r), nil
}

// UpdateReturnReason menyimpan perubahan alasan retur dengan optimistic locking.
func (s *Store) UpdateReturnReason(ctx context.Context, rr domain.ReturnReason, version int, actorID uuid.UUID) (domain.ReturnReason, error) {
	r, err := s.q(ctx).UpdateReturnReason(ctx, sqlcgen.UpdateReturnReasonParams{
		Name: rr.Name, AppliesTo: string(rr.AppliesTo), DefaultDisposition: text(string(rr.DefaultDisposition)),
		Description: rr.Description, IsActive: rr.IsActive, ActorID: actorID,
		ID: rr.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReturnReason{}, apperr.New(apperr.CodeVersionConflict)
	}
	if err != nil {
		return domain.ReturnReason{}, mapWriteError(err, "update return reason")
	}
	return toReturnReason(r), nil
}

// DeleteReturnReason melakukan soft delete.
func (s *Store) DeleteReturnReason(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteReturnReason(ctx, sqlcgen.SoftDeleteReturnReasonParams{ActorID: actorID, ID: id, Version: int32(version)}) //nolint:gosec // versi kecil
	if err != nil {
		return fmt.Errorf("delete return reason: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
