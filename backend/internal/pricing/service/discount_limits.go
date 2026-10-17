package service

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/pricing/domain"
)

// DiscountRoleCodes adalah role pembuat dokumen penjualan yang lazim diberi batas diskon (Desain Role &
// Approval 3.2: sales.sales_order W). Role lain tetap bisa diberi batas bila kodenya dikenal modul identity.
var DiscountRoleCodes = []string{"sales_mt", "sales_ecommerce", "customer_service", "head_sales"}

// RoleRef adalah role untuk pilihan batas diskon.
type RoleRef struct {
	Code string
	Name string
}

// DiscountLimitView adalah batas diskon beserta nama role dan statusnya hari ini.
type DiscountLimitView struct {
	domain.DiscountLimit
	RoleName string
	Status   domain.PeriodStatus
}

func discountLimitSnapshot(l domain.DiscountLimit) map[string]any {
	return map[string]any{
		"roleCode": l.RoleCode, "maxLineDiscountPercent": l.MaxLinePercent.String(),
		"maxDocumentDiscountPercent": l.MaxDocumentPercent.String(), "validFrom": l.ValidFrom.String(),
		"validTo": dateText(l.ValidTo), "notes": l.Notes,
	}
}

func readDiscountLimit(ctx context.Context) (actor.Actor, error) {
	return actor.RequireAny(ctx, PermDiscountLimitRead, PermPriceListRead)
}

func (s *Service) roleName(ctx context.Context, code string) (string, bool, error) {
	if s.Roles == nil {
		return code, true, nil
	}
	return s.Roles.RoleName(ctx, code)
}

// ListDiscountLimits mengembalikan batas diskon per role; includeEnded menampilkan riwayat.
func (s *Service) ListDiscountLimits(ctx context.Context, includeEnded bool) ([]DiscountLimitView, error) {
	if _, err := readDiscountLimit(ctx); err != nil {
		return nil, err
	}
	today := s.today()
	list, err := s.Store.ListDiscountLimits(ctx, includeEnded, today)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	out := make([]DiscountLimitView, len(list))
	for i, l := range list {
		name, ok := names[l.RoleCode]
		if !ok {
			if name, _, err = s.roleName(ctx, l.RoleCode); err != nil {
				return nil, err
			}
			names[l.RoleCode] = name
		}
		out[i] = DiscountLimitView{DiscountLimit: l, RoleName: name, Status: l.StatusOn(today)}
	}
	return out, nil
}

// DiscountRoles mengembalikan pilihan role untuk batas diskon.
func (s *Service) DiscountRoles(ctx context.Context) ([]RoleRef, error) {
	if _, err := readDiscountLimit(ctx); err != nil {
		return nil, err
	}
	out := make([]RoleRef, 0, len(DiscountRoleCodes))
	for _, code := range DiscountRoleCodes {
		name, ok, err := s.roleName(ctx, code)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, RoleRef{Code: code, Name: name})
		}
	}
	return out, nil
}

// CreateDiscountLimit menetapkan batas diskon role mulai tanggal tertentu. Batas lama tanpa tanggal berakhir
// ditutup otomatis sehari sebelumnya.
func (s *Service) CreateDiscountLimit(ctx context.Context, in domain.DiscountLimitInput) (domain.DiscountLimit, error) {
	a, err := actor.Require(ctx, PermDiscountLimitWrite)
	if err != nil {
		return domain.DiscountLimit{}, err
	}
	in.RoleCode = strings.TrimSpace(in.RoleCode)
	l, err := domain.NewDiscountLimit(in, s.today())
	if err != nil {
		return domain.DiscountLimit{}, err
	}
	if _, ok, err := s.roleName(ctx, l.RoleCode); err != nil {
		return domain.DiscountLimit{}, err
	} else if !ok {
		return domain.DiscountLimit{}, apperr.Validation(map[string]string{"roleCode": "Role tidak dikenal"})
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		existing, err := s.Store.OverlappingDiscountLimits(ctx, l)
		if err != nil {
			return err
		}
		err = supersede(existing, func(o domain.DiscountLimit) domain.Period { return o.Period }, l.Period, "batas diskon lain",
			func(o domain.DiscountLimit, p domain.Period) error {
				before := o
				o.Period = p
				if _, err := s.Store.UpdateDiscountLimitEnd(ctx, o, o.Version, a.UserID); err != nil {
					return err
				}
				return s.Audit.Record(ctx, audit.Entry{
					Action: audit.ActionUpdate, EntityType: EntityDiscountLimit, EntityID: o.ID, EntityLabel: o.RoleCode,
					Before: discountLimitSnapshot(before), After: discountLimitSnapshot(o),
				})
			})
		if err != nil {
			return err
		}
		if l, err = s.Store.InsertDiscountLimit(ctx, l, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityDiscountLimit, EntityID: l.ID, EntityLabel: l.RoleCode,
			After: discountLimitSnapshot(l),
		})
	})
	return l, err
}

// EndDiscountLimit mengisi tanggal berakhir batas diskon.
func (s *Service) EndDiscountLimit(ctx context.Context, id uuid.UUID, version int, validTo clock.BusinessDate) (domain.DiscountLimit, error) {
	a, err := actor.Require(ctx, PermDiscountLimitWrite)
	if err != nil {
		return domain.DiscountLimit{}, err
	}
	var l domain.DiscountLimit
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if l, err = s.Store.GetDiscountLimitForUpdate(ctx, id); err != nil {
			return err
		}
		if err := l.EnsureActive(); err != nil {
			return err
		}
		if err := checkVersion(l.Version, version); err != nil {
			return err
		}
		before := l
		if l.Period, err = l.EndAt(validTo, s.today()); err != nil {
			return err
		}
		if l, err = s.Store.UpdateDiscountLimitEnd(ctx, l, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityDiscountLimit, EntityID: l.ID, EntityLabel: l.RoleCode,
			Before: discountLimitSnapshot(before), After: discountLimitSnapshot(l),
		})
	})
	return l, err
}

// DeleteDiscountLimit menghapus batas diskon yang belum pernah berlaku.
func (s *Service) DeleteDiscountLimit(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermDiscountLimitWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.Store.GetDiscountLimitForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(l.Version, version); err != nil {
			return err
		}
		if l.Deleted() || !l.CanDelete(s.today()) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, msgStartedUndeletable)
		}
		if err := s.Store.DeleteDiscountLimit(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityDiscountLimit, EntityID: l.ID, EntityLabel: l.RoleCode,
			Before: discountLimitSnapshot(l),
		})
	})
}

// EffectiveLimit menghitung batas diskon role-role user hari ini (tanpa cek permission; dipakai SO).
func (s *Service) EffectiveLimit(ctx context.Context, roleCodes []string) (domain.Limit, error) {
	today := s.today()
	codes := slices.Compact(slices.Sorted(slices.Values(roleCodes)))
	limits, err := s.Store.DiscountLimitsOn(ctx, codes, today)
	if err != nil {
		return domain.Limit{}, err
	}
	return domain.EffectiveLimit(limits, codes, today), nil
}
