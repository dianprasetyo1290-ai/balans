package http

import (
	"context"
	"strings"

	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

func toReturnReason(r domain.ReturnReason) ReturnReason {
	out := ReturnReason{
		Id: r.ID, Code: r.Code, Name: r.Name, AppliesTo: ReturnReasonAppliesTo(r.AppliesTo), Description: r.Description,
		IsActive: r.IsActive, UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
	if r.DefaultDisposition != "" {
		d := ReturnDisposition(r.DefaultDisposition)
		out.DefaultDisposition = &d
	}
	return out
}

func returnReasonInput(f ReturnReasonFields) domain.ReturnReasonInput {
	in := domain.ReturnReasonInput{Name: f.Name, AppliesTo: domain.ReturnScope(f.AppliesTo), IsActive: f.IsActive}
	if f.DefaultDisposition != nil {
		in.DefaultDisposition = domain.Disposition(*f.DefaultDisposition)
	}
	if f.Description != nil {
		in.Description = *f.Description
	}
	return in
}

// ListReturnReasons mengembalikan alasan retur.
func (h *SalesHandler) ListReturnReasons(ctx context.Context, req ListReturnReasonsRequestObject) (ListReturnReasonsResponseObject, error) {
	f := postgres.ReturnReasonFilter{IncludeInactive: req.Params.IncludeInactive != nil && *req.Params.IncludeInactive}
	if req.Params.AppliesTo != nil {
		f.AppliesTo = domain.ReturnScope(*req.Params.AppliesTo)
	}
	if req.Params.Q != nil {
		f.Query = strings.TrimSpace(*req.Params.Q)
	}
	list, err := h.reasons.List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]ReturnReason, len(list))
	for i, r := range list {
		out[i] = toReturnReason(r)
	}
	return ListReturnReasons200JSONResponse(out), nil
}

// CreateReturnReason menambah alasan retur.
func (h *SalesHandler) CreateReturnReason(ctx context.Context, req CreateReturnReasonRequestObject) (CreateReturnReasonResponseObject, error) {
	b := req.Body
	in := returnReasonInput(ReturnReasonFields{
		Name: b.Name, AppliesTo: b.AppliesTo, DefaultDisposition: b.DefaultDisposition, Description: b.Description, IsActive: b.IsActive,
	})
	in.Code = b.Code
	r, err := h.reasons.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateReturnReason201JSONResponse(toReturnReason(r)), nil
}

// UpdateReturnReason mengubah alasan retur.
func (h *SalesHandler) UpdateReturnReason(ctx context.Context, req UpdateReturnReasonRequestObject) (UpdateReturnReasonResponseObject, error) {
	b := req.Body
	in := returnReasonInput(ReturnReasonFields{
		Name: b.Name, AppliesTo: b.AppliesTo, DefaultDisposition: b.DefaultDisposition, Description: b.Description, IsActive: b.IsActive,
	})
	r, err := h.reasons.Update(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateReturnReason200JSONResponse(toReturnReason(r)), nil
}

// DeleteReturnReason menghapus alasan retur.
func (h *SalesHandler) DeleteReturnReason(ctx context.Context, req DeleteReturnReasonRequestObject) (DeleteReturnReasonResponseObject, error) {
	if err := h.reasons.Delete(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteReturnReason204Response{}, nil
}
