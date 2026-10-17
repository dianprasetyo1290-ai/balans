package http

import (
	"context"
	"strings"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
)

func toExpenseType(e domain.ExpenseType) ExpenseType {
	return ExpenseType{
		Id: e.ID, Code: e.Code, Name: e.Name, ExpenseAccountId: e.ExpenseAccountID, ExpenseAccountCode: e.ExpenseAccountCode,
		ExpenseAccountName: e.ExpenseAccountName, WithholdingType: WithholdingType(e.WithholdingType),
		RequiresReceipt: e.RequiresReceipt, RequiresCampaign: e.RequiresCampaign,
		ApprovalSubject: ExpenseApprovalSubject(e.ApprovalSubject), Description: e.Description, IsActive: e.IsActive,
		UpdatedAt: e.UpdatedAt, Version: e.Version,
	}
}

func expenseTypeInput(f ExpenseTypeFields) domain.ExpenseTypeInput {
	in := domain.ExpenseTypeInput{
		Name: f.Name, ExpenseAccountID: f.ExpenseAccountId, WithholdingType: domain.WithholdingType(f.WithholdingType),
		RequiresReceipt: f.RequiresReceipt, RequiresCampaign: f.RequiresCampaign, Description: str(f.Description),
		IsActive: f.IsActive,
	}
	if f.ApprovalSubject != nil {
		in.ApprovalSubject = string(*f.ApprovalSubject)
	}
	return in
}

// ListExpenseTypes mengembalikan jenis biaya.
func (h *AccountingHandler) ListExpenseTypes(ctx context.Context, req ListExpenseTypesRequestObject) (ListExpenseTypesResponseObject, error) {
	list, err := h.svc.ListExpenseTypes(ctx, postgres.ExpenseTypeFilter{
		IncludeInactive: includeInactive(req.Params.IncludeInactive), Query: strings.TrimSpace(str(req.Params.Q)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ExpenseType, len(list))
	for i, e := range list {
		out[i] = toExpenseType(e)
	}
	return ListExpenseTypes200JSONResponse(out), nil
}

// CreateExpenseType menambah jenis biaya.
func (h *AccountingHandler) CreateExpenseType(ctx context.Context, req CreateExpenseTypeRequestObject) (CreateExpenseTypeResponseObject, error) {
	b := req.Body
	in := expenseTypeInput(ExpenseTypeFields{
		Name: b.Name, ExpenseAccountId: b.ExpenseAccountId, WithholdingType: b.WithholdingType, RequiresReceipt: b.RequiresReceipt,
		RequiresCampaign: b.RequiresCampaign, ApprovalSubject: b.ApprovalSubject, Description: b.Description, IsActive: b.IsActive,
	})
	in.Code = b.Code
	e, err := h.svc.CreateExpenseType(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateExpenseType201JSONResponse(toExpenseType(e)), nil
}

// UpdateExpenseType mengubah jenis biaya.
func (h *AccountingHandler) UpdateExpenseType(ctx context.Context, req UpdateExpenseTypeRequestObject) (UpdateExpenseTypeResponseObject, error) {
	b := req.Body
	in := expenseTypeInput(ExpenseTypeFields{
		Name: b.Name, ExpenseAccountId: b.ExpenseAccountId, WithholdingType: b.WithholdingType, RequiresReceipt: b.RequiresReceipt,
		RequiresCampaign: b.RequiresCampaign, ApprovalSubject: b.ApprovalSubject, Description: b.Description, IsActive: b.IsActive,
	})
	e, err := h.svc.UpdateExpenseType(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateExpenseType200JSONResponse(toExpenseType(e)), nil
}

// DeleteExpenseType menghapus jenis biaya.
func (h *AccountingHandler) DeleteExpenseType(ctx context.Context, req DeleteExpenseTypeRequestObject) (DeleteExpenseTypeResponseObject, error) {
	if err := h.svc.DeleteExpenseType(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteExpenseType204Response{}, nil
}
