package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
)

// --- Jenis biaya ------------------------------------------------------------------------------

func toExpenseType(r sqlcgen.GetExpenseTypeRow) domain.ExpenseType {
	return domain.ExpenseType{
		ID: r.ID, Code: r.Code, Name: r.Name, ExpenseAccountID: r.ExpenseAccountID, ExpenseAccountCode: r.AccountCode,
		ExpenseAccountName: r.AccountName, WithholdingType: domain.WithholdingType(r.WithholdingType),
		RequiresReceipt: r.RequiresReceipt, RequiresCampaign: r.RequiresCampaign, ApprovalSubject: r.ApprovalSubject,
		Description: r.Description, IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ExpenseTypeFilter adalah filter list jenis biaya.
type ExpenseTypeFilter struct {
	IncludeInactive bool
	Query           string
}

// ListExpenseTypes mengembalikan jenis biaya urut kode.
func (s *Store) ListExpenseTypes(ctx context.Context, f ExpenseTypeFilter) ([]domain.ExpenseType, error) {
	rows, err := s.q(ctx).ListExpenseTypes(ctx, sqlcgen.ListExpenseTypesParams{IncludeInactive: f.IncludeInactive, Q: text(f.Query)})
	if err != nil {
		return nil, fmt.Errorf("list expense types: %w", err)
	}
	out := make([]domain.ExpenseType, len(rows))
	for i, r := range rows {
		out[i] = toExpenseType(sqlcgen.GetExpenseTypeRow(r))
	}
	return out, nil
}

// GetExpenseType membaca satu jenis biaya yang belum dihapus; forUpdate mengunci baris.
func (s *Store) GetExpenseType(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ExpenseType, error) {
	var (
		r   sqlcgen.GetExpenseTypeRow
		err error
	)
	if forUpdate {
		var row sqlcgen.GetExpenseTypeForUpdateRow
		row, err = s.q(ctx).GetExpenseTypeForUpdate(ctx, id)
		r = sqlcgen.GetExpenseTypeRow(row)
	} else {
		r, err = s.q(ctx).GetExpenseType(ctx, id)
	}
	if err != nil {
		return domain.ExpenseType{}, notFound(err, "get expense type")
	}
	return toExpenseType(r), nil
}

// GetExpenseTypeByCode membaca jenis biaya berdasarkan kode.
func (s *Store) GetExpenseTypeByCode(ctx context.Context, code string) (domain.ExpenseType, error) {
	r, err := s.q(ctx).GetExpenseTypeByCode(ctx, code)
	if err != nil {
		return domain.ExpenseType{}, notFound(err, "get expense type by code")
	}
	return toExpenseType(sqlcgen.GetExpenseTypeRow(r)), nil
}

// InsertExpenseType menyimpan jenis biaya baru dan membacanya kembali beserta akun beban.
func (s *Store) InsertExpenseType(ctx context.Context, e domain.ExpenseType, actorID uuid.UUID) (domain.ExpenseType, error) {
	id, err := s.q(ctx).InsertExpenseType(ctx, sqlcgen.InsertExpenseTypeParams{
		Code: e.Code, Name: e.Name, ExpenseAccountID: e.ExpenseAccountID, WithholdingType: string(e.WithholdingType),
		RequiresReceipt: e.RequiresReceipt, RequiresCampaign: e.RequiresCampaign, ApprovalSubject: e.ApprovalSubject,
		Description: e.Description, IsActive: e.IsActive, ActorID: actorID,
	})
	if err != nil {
		return domain.ExpenseType{}, mapWriteError(err, "insert expense type")
	}
	return s.GetExpenseType(ctx, id, false)
}

// UpdateExpenseType menyimpan perubahan jenis biaya dengan optimistic locking.
func (s *Store) UpdateExpenseType(ctx context.Context, e domain.ExpenseType, version int, actorID uuid.UUID) (domain.ExpenseType, error) {
	n, err := s.q(ctx).UpdateExpenseType(ctx, sqlcgen.UpdateExpenseTypeParams{
		Name: e.Name, ExpenseAccountID: e.ExpenseAccountID, WithholdingType: string(e.WithholdingType),
		RequiresReceipt: e.RequiresReceipt, RequiresCampaign: e.RequiresCampaign, ApprovalSubject: e.ApprovalSubject,
		Description: e.Description, IsActive: e.IsActive, ActorID: actorID,
		ID: e.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.ExpenseType{}, mapWriteError(err, "update expense type")
	}
	if n == 0 {
		return domain.ExpenseType{}, apperr.New(apperr.CodeVersionConflict)
	}
	return s.GetExpenseType(ctx, e.ID, false)
}

// DeleteExpenseType melakukan soft delete.
func (s *Store) DeleteExpenseType(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteExpenseType(ctx, sqlcgen.SoftDeleteExpenseTypeParams{ActorID: actorID, ID: id, Version: int32(version)}) //nolint:gosec // versi kecil
	if err != nil {
		return fmt.Errorf("delete expense type: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
