package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Lookup untuk modul dokumen lain (procurement, sales, …). Tidak memeriksa permission karena modul pemanggil
// memeriksa permission dokumennya sendiri.

// DivisionOptions mengembalikan divisi aktif untuk pilihan divisi di dokumen modul lain (PR, PO).
func (s *Service) DivisionOptions(ctx context.Context) ([]domain.DimensionValue, error) {
	return s.Store.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{})
}

// DivisionNames mengembalikan nama divisi (termasuk yang nonaktif/terhapus) untuk tampilan dokumen.
func (s *Service) DivisionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	names, err := s.Store.DimensionNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(names))
	for id, n := range names {
		if n.Kind == domain.KindDivision {
			out[id] = n.Name
		}
	}
	return out, nil
}

// CheckDivision menolak divisi yang tidak ada atau nonaktif dengan VALIDATION_FAILED pada field.
func (s *Service) CheckDivision(ctx context.Context, id uuid.UUID, at clock.BusinessDate, field string) error {
	check, err := s.Store.CheckDimensions(ctx, domain.Dimensions{DivisionID: &id}, at)
	if err != nil {
		return err
	}
	if !check.DivisionOK {
		return apperr.Validation(map[string]string{field: "Divisi tidak ditemukan atau nonaktif"})
	}
	return nil
}

// PostableAccounts mengembalikan akun aktif yang bisa diposting dengan awalan kode tertentu (mis. "11" Kas &
// Bank) untuk pilihan akun di dokumen modul lain.
func (s *Service) PostableAccounts(ctx context.Context, codePrefix string) ([]domain.Account, error) {
	all, err := s.Store.ListAccounts(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Account, 0, len(all))
	for _, a := range all {
		if a.IsActive && a.IsPostable && strings.HasPrefix(a.Code, codePrefix) {
			out = append(out, a)
		}
	}
	return out, nil
}

// SalesChannels mengembalikan channel dan sub-channel aktif untuk pilihan channel dokumen penjualan. Tidak
// memeriksa permission.
func (s *Service) SalesChannels(ctx context.Context) ([]domain.DimensionValue, error) {
	channels, err := s.Store.ListDimensions(ctx, domain.KindChannel, postgres.DimensionFilter{})
	if err != nil {
		return nil, err
	}
	subs, err := s.Store.ListDimensions(ctx, domain.KindSubChannel, postgres.DimensionFilter{})
	if err != nil {
		return nil, err
	}
	return append(channels, subs...), nil
}

// DimensionLabels mengembalikan jenis, kode, dan nama dimensi (termasuk nonaktif/terhapus) untuk tampilan dokumen.
func (s *Service) DimensionLabels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.DimensionValue, error) {
	names, err := s.Store.DimensionNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.DimensionValue, len(names))
	for id, n := range names {
		out[id] = domain.DimensionValue{ID: id, Kind: n.Kind, Code: n.Code, Name: n.Name}
	}
	return out, nil
}
