package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

func accountSnapshot(a domain.Account) map[string]any {
	return map[string]any{
		"code": a.Code, "name": a.Name, "group": int(a.Group), "normalBalance": a.NormalBalance, "parentId": a.ParentID,
		"isPostable": a.IsPostable, "allowManualPosting": a.AllowManualPosting, "isActive": a.IsActive, "description": a.Description,
	}
}

// ListAccounts mengembalikan Chart of Accounts urut kode.
func (s *Service) ListAccounts(ctx context.Context, includeInactive bool) ([]domain.Account, error) {
	if _, err := actor.Require(ctx, PermAccountRead); err != nil {
		return nil, err
	}
	return s.Store.ListAccounts(ctx, includeInactive)
}

// GetAccount mengembalikan satu akun.
func (s *Service) GetAccount(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	if _, err := actor.RequireAny(ctx, PermAccountRead, PermJournalRead); err != nil {
		return domain.Account{}, err
	}
	return s.Store.GetAccount(ctx, id, false)
}

// checkParent memastikan induk ada, akun judul, satu kelompok, dan tidak membentuk siklus.
func (s *Service) checkParent(ctx context.Context, a domain.Account) error {
	if a.ParentID == nil {
		return nil
	}
	parent, err := s.Store.GetAccount(ctx, *a.ParentID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"parentId": "Akun induk tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	ancestors, err := s.Store.AccountAncestors(ctx, parent.ID)
	if err != nil {
		return err
	}
	return domain.ValidateParent(a, parent, ancestors)
}

// CreateAccount menambah akun COA (S-126).
func (s *Service) CreateAccount(ctx context.Context, in domain.AccountInput) (domain.Account, error) {
	a, err := actor.Require(ctx, PermAccountWrite)
	if err != nil {
		return domain.Account{}, err
	}
	acc, err := domain.NewAccount(in)
	if err != nil {
		return domain.Account{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkParent(ctx, acc); err != nil {
			return err
		}
		if acc, err = s.Store.InsertAccount(ctx, acc, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "account", EntityID: acc.ID, EntityLabel: acc.Code + " " + acc.Name,
			After: accountSnapshot(acc),
		})
	})
	return acc, err
}

// UpdateAccount mengubah akun. Kode tidak berubah; saldo normal dan sifat posting akun terpakai dikunci.
func (s *Service) UpdateAccount(ctx context.Context, id uuid.UUID, version int, in domain.AccountInput) (domain.Account, error) {
	a, err := actor.Require(ctx, PermAccountWrite)
	if err != nil {
		return domain.Account{}, err
	}
	var acc domain.Account
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if acc, err = s.Store.GetAccount(ctx, id, true); err != nil {
			return err
		}
		if acc.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		usage, err := s.Store.AccountUsage(ctx, id)
		if err != nil {
			return err
		}
		before := acc
		if err := acc.Update(in, usage); err != nil {
			return err
		}
		if err := s.checkParent(ctx, acc); err != nil {
			return err
		}
		if acc, err = s.Store.UpdateAccount(ctx, acc, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "account", EntityID: acc.ID, EntityLabel: acc.Code + " " + acc.Name,
			Before: accountSnapshot(before), After: accountSnapshot(acc),
		})
	})
	return acc, err
}

// DeleteAccount menghapus (soft delete) akun yang belum dipakai. Kode tetap tidak bisa dipakai ulang.
func (s *Service) DeleteAccount(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermAccountWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		acc, err := s.Store.GetAccount(ctx, id, true)
		if err != nil {
			return err
		}
		if acc.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		usage, err := s.Store.AccountUsage(ctx, id)
		if err != nil {
			return err
		}
		if err := acc.CheckDelete(usage); err != nil {
			return err
		}
		if err := s.Store.DeleteAccount(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "account", EntityID: acc.ID, EntityLabel: acc.Code + " " + acc.Name,
			Before: accountSnapshot(acc),
		})
	})
}
