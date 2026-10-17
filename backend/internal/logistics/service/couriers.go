package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/logistics/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

func courierSnapshot(c domain.Courier) map[string]any {
	return map[string]any{"code": c.Code, "name": c.Name, "isInternal": c.IsInternal, "isActive": c.IsActive}
}

// ListCouriers mengembalikan master kurir; activeOnly untuk dropdown pengiriman.
func (s *Service) ListCouriers(ctx context.Context, activeOnly bool) ([]domain.Courier, error) {
	if _, err := actor.RequireAny(ctx, PermShipmentRead, PermDORead); err != nil {
		return nil, err
	}
	var active *bool
	if activeOnly {
		t := true
		active = &t
	}
	return s.Store.ListCouriers(ctx, active)
}

// CourierUsers mengembalikan Merchandiser aktif untuk dipilih sebagai kurir internal (S-312).
func (s *Service) CourierUsers(ctx context.Context) ([]UserOption, error) {
	if _, err := actor.RequireAny(ctx, PermShipmentWrite, PermShipmentRead); err != nil {
		return nil, err
	}
	if s.Users == nil {
		return nil, apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul identity tidak aktif.")
	}
	return s.Users.UsersWithRole(ctx, RoleMerchandiser)
}

// CreateCourier menambah kurir (Kurir & Resi). Permission `logistics.shipment.write`.
func (s *Service) CreateCourier(ctx context.Context, in domain.CourierInput) (domain.Courier, error) {
	a, err := actor.Require(ctx, PermShipmentWrite)
	if err != nil {
		return domain.Courier{}, err
	}
	c, err := domain.NewCourier(in)
	if err != nil {
		return domain.Courier{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if c, err = s.Store.InsertCourier(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityCourier, EntityID: c.ID, EntityLabel: c.Name, After: courierSnapshot(c),
		})
	})
	return c, err
}

// UpdateCourier mengubah nama/jenis/status kurir.
func (s *Service) UpdateCourier(ctx context.Context, id uuid.UUID, version int, in domain.CourierInput) (domain.Courier, error) {
	a, err := actor.Require(ctx, PermShipmentWrite)
	if err != nil {
		return domain.Courier{}, err
	}
	var out domain.Courier
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetCourier(ctx, id)
		if err != nil {
			return err
		}
		if c.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := courierSnapshot(c)
		if err := c.Update(in); err != nil {
			return err
		}
		if out, err = s.Store.UpdateCourier(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityCourier, EntityID: c.ID, EntityLabel: c.Name,
			Before: before, After: courierSnapshot(out),
		})
	})
	return out, err
}
