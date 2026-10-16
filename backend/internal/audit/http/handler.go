package http

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"balans/internal/audit/domain"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/audit.yaml ../../../api/openapi.bundle.yaml

type auditReader interface {
	List(ctx context.Context, f domain.Filter, page paging.Page) (paging.Result[domain.Log], error)
}

// AuditHandler mengimplementasikan StrictServerInterface hasil generate.
type AuditHandler struct {
	svc auditReader
}

var _ StrictServerInterface = (*AuditHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc auditReader) *AuditHandler { return &AuditHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *AuditHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// ListAuditLogs mengembalikan riwayat perubahan.
func (h *AuditHandler) ListAuditLogs(ctx context.Context, req ListAuditLogsRequestObject) (ListAuditLogsResponseObject, error) {
	p := req.Params
	f := domain.Filter{From: p.CreatedFrom, To: p.CreatedTo}
	if p.EntityType != nil {
		f.EntityType = *p.EntityType
	}
	if p.EntityId != nil {
		f.EntityID = *p.EntityId
	}
	if p.ActorId != nil {
		f.ActorID = *p.ActorId
	}
	if p.Action != nil {
		f.Action = domain.Action(*p.Action)
	}
	res, err := h.svc.List(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]AuditLog, 0, len(res.Items))
	for _, l := range res.Items {
		changes := make(map[string]AuditChange, len(l.Changes))
		for k, c := range l.Changes {
			changes[k] = AuditChange{From: c.From, To: c.To}
		}
		item := AuditLog{
			Id: l.ID, ActorId: l.ActorID, ActorName: l.ActorName, Action: AuditAction(l.Action),
			EntityType: l.EntityType, EntityId: l.EntityID, EntityLabel: l.EntityLabel, Changes: changes,
			Note: l.Note, CreatedAt: l.CreatedAt,
		}
		if l.IP != "" {
			item.Ip = &l.IP
		}
		if item.ActorId == uuid.Nil {
			item.ActorName = "Sistem"
		}
		items = append(items, item)
	}
	return ListAuditLogs200JSONResponse{
		Items:      items,
		Page:       res.Page.Number,
		PageSize:   res.Page.Size,
		TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}
