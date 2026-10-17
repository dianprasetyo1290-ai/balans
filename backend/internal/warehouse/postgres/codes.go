package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func idLookup(id uuid.UUID, err error, what string) (uuid.UUID, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("%s: %w", what, err)
	}
	return id, true, nil
}

// LocationIDByCode mencari lokasi berdasarkan kode (termasuk yang terhapus, karena kode unik permanen).
func (s *Store) LocationIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetLocationIDByCode(ctx, code)
	return idLookup(id, err, "location by code")
}

// WarehouseIDByCode mencari gudang berdasarkan kode (termasuk yang terhapus).
func (s *Store) WarehouseIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetWarehouseIDByCode(ctx, code)
	return idLookup(id, err, "warehouse by code")
}
