// Package service berisi use case modul pricing: daftar harga & versinya (approval price_list_version), harga
// khusus, promo, harga bertingkat, dan batas diskon per role (BRD 5.18).
package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
)

// Permission modul pricing (Desain Role & Approval bagian 3.1).
const (
	PermPriceListRead     = "pricing.price_list.read"
	PermPriceListWrite    = "pricing.price_list.write"
	PermPriceListSubmit   = "pricing.price_list.submit"
	PermPriceListApprove  = "pricing.price_list.approve"
	PermPromotionRead     = "pricing.promotion.read"
	PermPromotionWrite    = "pricing.promotion.write"
	PermSpecialPriceRead  = "pricing.special_price.read"
	PermSpecialPriceWrite = "pricing.special_price.write"
	PermDiscountLimitRead = "pricing.discount_limit.read"
	// PermDiscountLimitWrite mengatur batas diskon per role.
	PermDiscountLimitWrite = "pricing.discount_limit.write"
)

// Jenis entitas audit trail.
const (
	EntityPriceList        = "price_list"
	EntityPriceListVersion = "price_list_version"
	EntitySpecialPrice     = "special_price"
	EntityPromotion        = "promotion"
	EntityTieredPrice      = "tiered_price"
	EntityDiscountLimit    = "discount_limit"
)

// ProductRef adalah ringkasan produk dari modul catalog.
type ProductRef struct {
	ID   uuid.UUID
	SKU  string
	Name string
	// Unusable berisi alasan produk tidak boleh diberi harga baru (dihapus, nonaktif, bukan Produk Jadi); "" = boleh.
	Unusable string
}

// Products membaca produk dari modul catalog.
type Products interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error)
}

// PartyRef adalah ringkasan pelanggan atau mitra dari modul party.
type PartyRef struct {
	ID   uuid.UUID
	Code string
	Name string
}

// Parties membaca pelanggan dan mitra dari modul party.
type Parties interface {
	Customers(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]PartyRef, error)
	Partners(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]PartyRef, error)
}

// Roles membaca nama role dari modul identity.
type Roles interface {
	RoleName(ctx context.Context, code string) (string, bool, error)
}

// Approvals mengajukan dan membatalkan permintaan approval (modul approval).
type Approvals interface {
	Request(ctx context.Context, s approval.Subject) (approval.Request, error)
	Cancel(ctx context.Context, t approval.SubjectType, subjectID uuid.UUID) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Deps adalah dependensi Service.
type Deps struct {
	Store     *postgres.Store
	Tx        txRunner
	Audit     audit.Recorder
	Clock     clock.Clock
	Products  Products
	Parties   Parties
	Roles     Roles
	Approvals Approvals
}

// Service adalah use case modul pricing.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

func (s *Service) now() time.Time { return s.Clock.Now() }

// products membaca produk; tanpa modul catalog dikembalikan peta kosong.
func (s *Service) products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error) {
	if s.Products == nil || len(ids) == 0 {
		return map[uuid.UUID]ProductRef{}, nil
	}
	return s.Products.Products(ctx, uniqueIDs(ids))
}

// checkProducts memastikan produk ada dan boleh diberi harga. field(i) adalah nama field error baris ke-i.
func (s *Service) checkProducts(ctx context.Context, ids []uuid.UUID, field func(i int) string) error {
	if s.Products == nil {
		return nil
	}
	refs, err := s.products(ctx, ids)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	for i, id := range ids {
		ref, ok := refs[id]
		switch {
		case !ok:
			fields[field(i)] = "Produk tidak ditemukan"
		case ref.Unusable != "":
			fields[field(i)] = ref.Unusable
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok || id == uuid.Nil {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func checkVersion(cur, want int) error {
	if cur != want {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

func dateText(d *clock.BusinessDate) string {
	if d == nil {
		return ""
	}
	return d.String()
}

func decimalText(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return d.String()
}

func idText(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// productLabel adalah label audit "SKU" atau ID produk bila catalog tidak tersedia.
func (s *Service) productLabel(ctx context.Context, id uuid.UUID) string {
	refs, err := s.products(ctx, []uuid.UUID{id})
	if err == nil {
		if r, ok := refs[id]; ok {
			return r.SKU
		}
	}
	return id.String()
}

// supersede menutup otomatis data effective-dated lama yang beririsan dengan data baru (period). Data lama tanpa
// tanggal berakhir yang mulai sebelum data baru ditutup sehari sebelumnya; irisan lain ditolak.
func supersede[T any](existing []T, period func(T) domain.Period, newPeriod domain.Period, what string,
	end func(old T, p domain.Period) error,
) error {
	for _, e := range existing {
		cur := period(e)
		p, ok := cur.Supersede(newPeriod.ValidFrom)
		if !ok || p.Overlaps(newPeriod) {
			return domain.OverlapError(what, cur)
		}
		if err := end(e, p); err != nil {
			return err
		}
	}
	return nil
}

func normalizeQuery(q string) string { return strings.TrimSpace(q) }

func itoa(i int) string { return strconv.Itoa(i) }
