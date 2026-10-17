// Package postgres adalah repository modul pricing.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/softdelete"
	"balans/internal/pricing/postgres/sqlcgen"
)

// Store menyimpan daftar harga dan aturan harga.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

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

func toPgDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func fromPgDate(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	b := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &b
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

func amountPtr(d decimal.NullDecimal) *money.Amount {
	if !d.Valid {
		return nil
	}
	a := money.FromDecimal(d.Decimal)
	return &a
}

func amountText(a *money.Amount) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func mark(at *time.Time, by uuid.NullUUID) softdelete.Mark {
	return softdelete.Mark{DeletedAt: at, DeletedBy: uuidPtr(by)}
}

func version32(v int) int32 { return int32(v) } //nolint:gosec // versi optimistic locking kecil

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// constraintMessages memetakan pelanggaran constraint ke field dan pesan untuk user.
var constraintMessages = map[string][2]string{
	"uq_price_lists_code":             {"code", "Kode sudah dipakai (termasuk daftar harga yang sudah dihapus)"},
	"uq_price_lists_scope":            {"priceListType", "Sudah ada daftar harga untuk cakupan ini"},
	"uq_price_list_versions_open":     {"validFrom", "Masih ada versi draft atau yang sedang diajukan untuk daftar harga ini"},
	"ex_price_list_versions_period":   {"validFrom", "Periode beririsan dengan versi aktif lain"},
	"uq_promotions_code":              {"code", "Kode promo sudah dipakai (termasuk promo yang sudah dihapus)"},
	"ex_special_prices_period":        {"validFrom", "Periode beririsan dengan harga khusus lain untuk pihak dan produk ini"},
	"ex_tiered_prices_period":         {"validFrom", "Periode beririsan dengan harga bertingkat lain untuk produk dan qty ini"},
	"ex_discount_limits_period":       {"validFrom", "Periode beririsan dengan batas diskon lain untuk role ini"},
	"uq_price_list_items_product":     {"items", "Produk tidak boleh ganda dalam satu versi"},
	"uq_promotion_items_product":      {"items", "Produk tidak boleh ganda dalam satu promo"},
	"ck_price_list_items_max_retail":  {"items", "HET tidak boleh di bawah harga dasar"},
	"ck_price_lists_basis_reference":  {"referencePriceListId", "Pilih daftar harga acuan"},
	"fk_price_lists_reference":        {"referencePriceListId", "Daftar harga acuan tidak ditemukan"},
	"fk_tiered_prices_price_list":     {"priceListId", "Daftar harga tidak ditemukan"},
	"ck_price_list_versions_validity": {"validTo", "Tanggal berakhir tidak boleh sebelum tanggal mulai"},
}

// mapWriteError mengubah pelanggaran unik/EXCLUDE/check yang dikenal menjadi VALIDATION_FAILED/DUPLICATE_CODE.
func mapWriteError(err error, what string) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if m, ok := constraintMessages[pe.ConstraintName]; ok {
			code := apperr.CodeValidationFailed
			if pe.Code == "23505" && m[0] == "code" {
				code = apperr.CodeDuplicateCode
			}
			return &apperr.Error{Code: code, Message: code.DefaultMessage(), Fields: map[string]string{m[0]: m[1]}, Err: err}
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

func versionConflict(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return mapWriteError(err, what)
}

func rowsConflict(n int64, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
