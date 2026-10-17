package http

import (
	"context"

	"github.com/go-chi/chi/v5"

	"balans/internal/notification/service"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/notification.yaml ../../../api/openapi.bundle.yaml

// NotificationHandler mengimplementasikan StrictServerInterface hasil generate.
type NotificationHandler struct {
	svc *service.NotificationService
}

var _ StrictServerInterface = (*NotificationHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *NotificationHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// ListNotifications mengembalikan notifikasi user.
func (h *NotificationHandler) ListNotifications(ctx context.Context, req ListNotificationsRequestObject) (ListNotificationsResponseObject, error) {
	p := req.Params
	res, err := h.svc.List(ctx, p.UnreadOnly != nil && *p.UnreadOnly, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	unread, err := h.svc.CountUnread(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Notification, len(res.Items))
	for i, n := range res.Items {
		items[i] = Notification{
			Id: n.ID, Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link, ReadAt: n.ReadAt, CreatedAt: n.CreatedAt,
		}
	}
	return ListNotifications200JSONResponse{
		Items: items, UnreadCount: unread, Page: res.Page.Number, PageSize: res.Page.Size,
		TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// GetNotificationUnreadCount mengembalikan jumlah belum dibaca.
func (h *NotificationHandler) GetNotificationUnreadCount(ctx context.Context, _ GetNotificationUnreadCountRequestObject) (GetNotificationUnreadCountResponseObject, error) {
	n, err := h.svc.CountUnread(ctx)
	if err != nil {
		return nil, err
	}
	return GetNotificationUnreadCount200JSONResponse{UnreadCount: n}, nil
}

// MarkNotificationRead menandai satu notifikasi sudah dibaca.
func (h *NotificationHandler) MarkNotificationRead(ctx context.Context, req MarkNotificationReadRequestObject) (MarkNotificationReadResponseObject, error) {
	if err := h.svc.MarkRead(ctx, req.Id); err != nil {
		return nil, err
	}
	return MarkNotificationRead204Response{}, nil
}

// MarkAllNotificationsRead menandai semua notifikasi sudah dibaca.
func (h *NotificationHandler) MarkAllNotificationsRead(ctx context.Context, _ MarkAllNotificationsReadRequestObject) (MarkAllNotificationsReadResponseObject, error) {
	if err := h.svc.MarkAllRead(ctx); err != nil {
		return nil, err
	}
	return MarkAllNotificationsRead204Response{}, nil
}
