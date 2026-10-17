package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

func expenseTypeSnapshot(e domain.ExpenseType) map[string]any {
	return map[string]any{
		"code": e.Code, "name": e.Name, "expenseAccountId": e.ExpenseAccountID, "withholdingType": e.WithholdingType,
		"requiresReceipt": e.RequiresReceipt, "requiresCampaign": e.RequiresCampaign, "approvalSubject": e.ApprovalSubject,
		"description": e.Description, "isActive": e.IsActive,
	}
}

// ListExpenseTypes mengembalikan jenis biaya (Data Master › Jenis Biaya, dropdown pengajuan biaya).
func (s *Service) ListExpenseTypes(ctx context.Context, f postgres.ExpenseTypeFilter) ([]domain.ExpenseType, error) {
	if _, err := actor.RequireAny(ctx, PermExpenseTypeRead, PermExpenseClaimRead); err != nil {
		return nil, err
	}
	return s.Store.ListExpenseTypes(ctx, f)
}

// ExpenseType membaca jenis biaya untuk modul/use case lain (pengajuan biaya, kas kecil) tanpa cek permission;
// pemanggil memeriksa permission dokumennya sendiri. Jenis biaya terhapus → NOT_FOUND.
func (s *Service) ExpenseType(ctx context.Context, id uuid.UUID) (domain.ExpenseType, error) {
	return s.Store.GetExpenseType(ctx, id, false)
}

// ExpenseTypeByCode membaca jenis biaya berdasarkan kode (impor, seed) tanpa cek permission.
func (s *Service) ExpenseTypeByCode(ctx context.Context, code string) (domain.ExpenseType, error) {
	return s.Store.GetExpenseTypeByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
}

// checkExpenseAccount memastikan akun beban ada dan memenuhi syarat.
func (s *Service) checkExpenseAccount(ctx context.Context, id uuid.UUID) error {
	acc, err := s.Store.GetAccount(ctx, id, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"expenseAccountId": "Akun beban tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	return domain.ValidateExpenseAccount(acc)
}

// CreateExpenseType menambah jenis biaya (S-124).
func (s *Service) CreateExpenseType(ctx context.Context, in domain.ExpenseTypeInput) (domain.ExpenseType, error) {
	a, err := actor.Require(ctx, PermExpenseTypeWrite)
	if err != nil {
		return domain.ExpenseType{}, err
	}
	e, err := domain.NewExpenseType(in)
	if err != nil {
		return domain.ExpenseType{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkExpenseAccount(ctx, e.ExpenseAccountID); err != nil {
			return err
		}
		if e, err = s.Store.InsertExpenseType(ctx, e, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "expense_type", EntityID: e.ID, EntityLabel: e.Code, After: expenseTypeSnapshot(e),
		})
	})
	return e, err
}

// UpdateExpenseType mengubah jenis biaya. Kode tidak berubah; akun beban baru diperiksa ulang.
func (s *Service) UpdateExpenseType(ctx context.Context, id uuid.UUID, version int, in domain.ExpenseTypeInput) (domain.ExpenseType, error) {
	a, err := actor.Require(ctx, PermExpenseTypeWrite)
	if err != nil {
		return domain.ExpenseType{}, err
	}
	var e domain.ExpenseType
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if e, err = s.Store.GetExpenseType(ctx, id, true); err != nil {
			return err
		}
		if e.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := e
		if err := e.Update(in); err != nil {
			return err
		}
		if e.ExpenseAccountID != before.ExpenseAccountID {
			if err := s.checkExpenseAccount(ctx, e.ExpenseAccountID); err != nil {
				return err
			}
		}
		if e, err = s.Store.UpdateExpenseType(ctx, e, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "expense_type", EntityID: e.ID, EntityLabel: e.Code,
			Before: expenseTypeSnapshot(before), After: expenseTypeSnapshot(e),
		})
	})
	return e, err
}

// DeleteExpenseType menghapus (soft delete) jenis biaya. Kode tetap tidak bisa dipakai ulang. Pemeriksaan
// pemakaian oleh pengajuan biaya/kas kecil ditambahkan bersama dokumennya (S-404, S-405).
func (s *Service) DeleteExpenseType(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermExpenseTypeWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		e, err := s.Store.GetExpenseType(ctx, id, true)
		if err != nil {
			return err
		}
		if e.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if err := s.Store.DeleteExpenseType(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "expense_type", EntityID: e.ID, EntityLabel: e.Code, Before: expenseTypeSnapshot(e),
		})
	})
}
