package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func dimensionSnapshot(v domain.DimensionValue) map[string]any {
	m := map[string]any{"code": v.Code, "name": v.Name, "isActive": v.IsActive}
	switch v.Kind {
	case domain.KindSubChannel:
		m["channelId"], m["partyType"], m["partyId"] = v.ChannelID, v.PartyType, v.PartyID
	case domain.KindCampaign:
		m["validFrom"], m["validTo"], m["description"] = v.ValidFrom.String(), v.ValidTo.String(), v.Description
		if v.Budget != nil {
			m["budgetAmount"] = v.Budget.String()
		}
	case domain.KindChannel, domain.KindDivision:
	}
	return m
}

// ListDimensions mengembalikan master dimensi satu jenis.
func (s *Service) ListDimensions(ctx context.Context, kind domain.DimensionKind, f postgres.DimensionFilter) ([]domain.DimensionValue, error) {
	if _, err := actor.RequireAny(ctx, PermDimensionRead, PermJournalRead); err != nil {
		return nil, err
	}
	return s.Store.ListDimensions(ctx, kind, f)
}

// CreateDimension menambah channel, sub-channel, divisi, atau kampanye (S-129).
func (s *Service) CreateDimension(ctx context.Context, kind domain.DimensionKind, in domain.DimensionInput) (domain.DimensionValue, error) {
	a, err := actor.Require(ctx, PermDimensionWrite)
	if err != nil {
		return domain.DimensionValue{}, err
	}
	v, err := domain.NewDimension(kind, in)
	if err != nil {
		return domain.DimensionValue{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if v, err = s.Store.InsertDimension(ctx, v, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: string(kind), EntityID: v.ID, EntityLabel: v.Code, After: dimensionSnapshot(v),
		})
	})
	return v, err
}

// UpdateDimension mengubah master dimensi. Kode tidak berubah.
func (s *Service) UpdateDimension(ctx context.Context, kind domain.DimensionKind, id uuid.UUID, version int, in domain.DimensionInput) (domain.DimensionValue, error) {
	a, err := actor.Require(ctx, PermDimensionWrite)
	if err != nil {
		return domain.DimensionValue{}, err
	}
	var v domain.DimensionValue
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if v, err = s.Store.GetDimensionForUpdate(ctx, kind, id); err != nil {
			return err
		}
		if v.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := v
		if err := v.Update(in); err != nil {
			return err
		}
		if v, err = s.Store.UpdateDimension(ctx, v, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: string(kind), EntityID: v.ID, EntityLabel: v.Code,
			Before: dimensionSnapshot(before), After: dimensionSnapshot(v),
		})
	})
	return v, err
}

// DeleteDimension menghapus (soft delete) master yang belum dipakai jurnal.
func (s *Service) DeleteDimension(ctx context.Context, kind domain.DimensionKind, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermDimensionWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		v, err := s.Store.GetDimensionForUpdate(ctx, kind, id)
		if err != nil {
			return err
		}
		if v.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		used, children, err := s.Store.DimensionUsage(ctx, kind, id)
		if err != nil {
			return err
		}
		if err := v.CheckDelete(used, children); err != nil {
			return err
		}
		if err := s.Store.DeleteDimension(ctx, kind, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: string(kind), EntityID: v.ID, EntityLabel: v.Code, Before: dimensionSnapshot(v),
		})
	})
}

// checkDimensions memeriksa keberadaan, status aktif, dan periode kampanye; prefix untuk nama field error.
func (s *Service) checkDimensions(ctx context.Context, d domain.Dimensions, at clock.BusinessDate, prefix string) (map[string]string, error) {
	if d == (domain.Dimensions{}) {
		return nil, nil
	}
	check, err := s.Store.CheckDimensions(ctx, d, at)
	if err != nil {
		return nil, err
	}
	return check.Errors(prefix), nil
}
