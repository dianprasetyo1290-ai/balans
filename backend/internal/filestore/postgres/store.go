// Package postgres adalah repository metadata file.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/filestore/domain"
	"balans/internal/filestore/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
)

// Store menyimpan metadata file di file_objects.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func toFile(r sqlcgen.FileObject) domain.File {
	f := domain.File{
		ID: r.ID, ObjectKey: r.ObjectKey, OriginalName: r.OriginalName, ContentType: r.ContentType,
		SizeBytes: r.SizeBytes, SHA256: r.Sha256, OwnerType: r.OwnerType, Status: domain.Status(r.Status),
		UploadedAt: r.UploadedAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, Version: int(r.Version),
	}
	if r.OwnerID.Valid {
		id := r.OwnerID.UUID
		f.OwnerID = &id
	}
	return f
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// Insert mencatat file pending.
func (s *Store) Insert(ctx context.Context, f domain.File, actorID uuid.UUID) (domain.File, error) {
	r, err := s.q(ctx).InsertFile(ctx, sqlcgen.InsertFileParams{
		OriginalName: f.OriginalName, ContentType: f.ContentType, SizeBytes: f.SizeBytes, OwnerType: f.OwnerType,
		ActorID: actorID,
	})
	if err != nil {
		return domain.File{}, fmt.Errorf("insert file: %w", err)
	}
	return toFile(r), nil
}

// Get membaca metadata file; forUpdate mengunci baris.
func (s *Store) Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.File, error) {
	var (
		r   sqlcgen.FileObject
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetFileForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetFile(ctx, id)
	}
	if err != nil {
		return domain.File{}, notFound(err, "get file")
	}
	return toFile(r), nil
}

// ListByIDs membaca metadata beberapa file.
func (s *Store) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.File, error) {
	rows, err := s.q(ctx).ListFilesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	out := make([]domain.File, len(rows))
	for i, r := range rows {
		out[i] = toFile(r)
	}
	return out, nil
}

// MarkUploaded menyimpan hash dan waktu upload.
func (s *Store) MarkUploaded(ctx context.Context, f domain.File) error {
	n, err := s.q(ctx).MarkFileUploaded(ctx, sqlcgen.MarkFileUploadedParams{
		ID: f.ID, Sha256: f.SHA256, UploadedAt: *f.UploadedAt, Version: int32(f.Version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return fmt.Errorf("mark file uploaded: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// Attach menyimpan pemilik file.
func (s *Store) Attach(ctx context.Context, f domain.File, actorID uuid.UUID) error {
	n, err := s.q(ctx).AttachFile(ctx, sqlcgen.AttachFileParams{
		ID: f.ID, OwnerID: uuid.NullUUID{UUID: *f.OwnerID, Valid: true}, ActorID: actorID,
		Version: int32(f.Version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return fmt.Errorf("attach file: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
