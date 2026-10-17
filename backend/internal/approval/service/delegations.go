package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/approval/domain"
	"balans/internal/audit"
	"balans/internal/platform/apperr"
)

func delegationSnapshot(d domain.Delegation) map[string]any {
	return map[string]any{
		"fromUserId": d.From, "toUserId": d.To, "validFrom": d.ValidFrom, "validTo": d.ValidTo,
		"reason": d.Reason, "cancelledAt": d.CancelledAt,
	}
}

// ListDelegations mengembalikan delegasi milik user (sebagai pemberi atau penerima). all=true menampilkan
// delegasi semua user dan hanya untuk pengelola kebijakan.
func (s *Service) ListDelegations(ctx context.Context, all, includePast bool) ([]domain.Delegation, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return nil, err
	}
	var user *uuid.UUID
	if all {
		if !a.Has(PermPolicyRead) {
			return nil, apperr.New(apperr.CodeForbidden)
		}
	} else {
		user = &a.UserID
	}
	return s.Store.ListDelegations(ctx, user, includePast, s.today())
}

// CreateDelegation mencatat delegasi approval saat cuti. User mendelegasikan miliknya sendiri; pengelola
// kebijakan boleh mencatat delegasi untuk user lain.
func (s *Service) CreateDelegation(ctx context.Context, in domain.DelegationInput) (domain.Delegation, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return domain.Delegation{}, err
	}
	if in.From == uuid.Nil {
		in.From = a.UserID
	}
	if in.From != a.UserID && !a.Has(PermPolicyWrite) {
		return domain.Delegation{}, apperr.New(apperr.CodeForbidden)
	}
	d, err := domain.NewDelegation(in, s.today())
	if err != nil {
		return domain.Delegation{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		fields := map[string]string{}
		if _, err := s.Directory.User(ctx, d.From); apperr.HasCode(err, apperr.CodeNotFound) {
			fields["fromUserId"] = "Pengguna tidak ditemukan atau nonaktif"
		} else if err != nil {
			return err
		}
		if _, err := s.Directory.User(ctx, d.To); apperr.HasCode(err, apperr.CodeNotFound) {
			fields["toUserId"] = "Pengguna tidak ditemukan atau nonaktif"
		} else if err != nil {
			return err
		}
		if len(fields) > 0 {
			return apperr.Validation(fields)
		}
		if d.ID, err = s.Store.InsertDelegation(ctx, d, a.UserID); err != nil {
			return err
		}
		d.CreatedBy, d.Version = a.UserID, 1
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "approval_delegation", EntityID: d.ID, After: delegationSnapshot(d),
		})
	})
	return d, err
}

// CancelDelegation menghentikan delegasi. Boleh oleh pemberi delegasi, pencatatnya, atau pengelola kebijakan.
func (s *Service) CancelDelegation(ctx context.Context, id uuid.UUID, version int) (domain.Delegation, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return domain.Delegation{}, err
	}
	var d domain.Delegation
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if d, err = s.Store.GetDelegationForUpdate(ctx, id); err != nil {
			return err
		}
		if d.From != a.UserID && d.CreatedBy != a.UserID && !a.Has(PermPolicyWrite) {
			return apperr.New(apperr.CodeForbidden)
		}
		if d.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := d
		if err := d.Cancel(s.Clock.Now(), s.today()); err != nil {
			return err
		}
		if err := s.Store.CancelDelegation(ctx, d, version, a.UserID); err != nil {
			return err
		}
		d.Version++
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCancel, EntityType: "approval_delegation", EntityID: d.ID,
			Before: delegationSnapshot(before), After: delegationSnapshot(d),
		})
	})
	return d, err
}

// SearchUsers mencari user aktif untuk dipilih sebagai penerima delegasi, approver, atau pengganti.
func (s *Service) SearchUsers(ctx context.Context, q string) ([]domain.Principal, error) {
	if _, err := loggedIn(ctx); err != nil {
		return nil, err
	}
	return s.Directory.SearchUsers(ctx, q, 20)
}

// UsersByID mengembalikan nama user untuk ditampilkan; user yang tidak ditemukan dilewati.
func (s *Service) UsersByID(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Principal, error) {
	out := make(map[uuid.UUID]domain.Principal, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok || id == uuid.Nil {
			continue
		}
		p, err := s.Directory.User(ctx, id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, nil
}

// RoleName mengembalikan nama jabatan untuk ditampilkan.
func (s *Service) RoleName(ctx context.Context, code string) string { return s.roleLabel(ctx, code) }
