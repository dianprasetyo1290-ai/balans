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

// CustomerIDByCode mencari pelanggan yang belum dihapus berdasarkan kode (impor Excel).
func (s *Store) CustomerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetCustomerIDByCode(ctx, code)
	return idLookup(id, err, "customer by code")
}

// PartnerIDByCode mencari mitra yang belum dihapus berdasarkan kode.
func (s *Store) PartnerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetPartnerIDByCode(ctx, code)
	return idLookup(id, err, "partner by code")
}

// SupplierIDByCode mencari pemasok yang belum dihapus berdasarkan kode.
func (s *Store) SupplierIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetSupplierIDByCode(ctx, code)
	return idLookup(id, err, "supplier by code")
}
