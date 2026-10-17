package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IDByNumber mencari karyawan yang belum dihapus berdasarkan nomor karyawan (impor Excel).
func (s *Store) IDByNumber(ctx context.Context, number string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetEmployeeIDByNumber(ctx, number)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("employee by number: %w", err)
	}
	return id, true, nil
}
