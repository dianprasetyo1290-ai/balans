package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"balans/internal/platform/apperr"
	"balans/internal/platform/httpx"
	"balans/internal/sales/service"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/sales.yaml ../../../api/openapi.bundle.yaml

const maxWebhookBytes = 1 << 20

type paymentWebhookReceiver interface {
	Receive(ctx context.Context, body []byte) (bool, error)
}

type responder interface {
	JSON(w http.ResponseWriter, r *http.Request, status int, body any)
	Error(w http.ResponseWriter, r *http.Request, err error)
}

// WebhookHandler menerima webhook payment gateway. Webhook memakai handler non-strict karena signature
// diverifikasi terhadap body asli.
type WebhookHandler struct {
	webhooks paymentWebhookReceiver
	rs       responder
}

// NewWebhookHandler membuat handler.
func NewWebhookHandler(webhooks paymentWebhookReceiver, rs responder) *WebhookHandler {
	return &WebhookHandler{webhooks: webhooks, rs: rs}
}

// ReceiveMidtransWebhook menerima notifikasi pembayaran.
func (h *WebhookHandler) ReceiveMidtransWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes))
	if err != nil {
		h.rs.Error(w, r, apperr.Wrap(apperr.CodeBadRequest, err))
		return
	}
	duplicate, err := h.webhooks.Receive(r.Context(), body)
	if err != nil {
		h.rs.Error(w, r, err)
		return
	}
	h.rs.JSON(w, r, http.StatusOK, WebhookAck{Received: true, Duplicate: duplicate})
}

// SalesHandler mengimplementasikan StrictServerInterface untuk endpoint JSON biasa (master alasan retur).
type SalesHandler struct {
	reasons *service.ReturnReasonService
}

var _ StrictServerInterface = (*SalesHandler)(nil)

// NewHandler membuat handler.
func NewHandler(reasons *service.ReturnReasonService) *SalesHandler {
	return &SalesHandler{reasons: reasons}
}

// errWebhookRoute: webhook tidak pernah dirutekan lewat handler strict (lihat router).
var errWebhookRoute = errors.New("sales: webhook Midtrans ditangani WebhookHandler")

// ReceiveMidtransWebhook tidak dipakai; route webhook diarahkan ke WebhookHandler oleh router.
func (h *SalesHandler) ReceiveMidtransWebhook(context.Context, ReceiveMidtransWebhookRequestObject) (ReceiveMidtransWebhookResponseObject, error) {
	return nil, errWebhookRoute
}

// router menggabungkan handler strict dengan WebhookHandler yang membaca body asli.
type router struct {
	ServerInterface
	webhook *WebhookHandler
}

func (r router) ReceiveMidtransWebhook(w http.ResponseWriter, req *http.Request) {
	r.webhook.ReceiveMidtransWebhook(w, req)
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *SalesHandler, webhook *WebhookHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(router{ServerInterface: strict, webhook: webhook}, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}
