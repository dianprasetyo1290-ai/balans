package http

import (
	"context"
	"io"
	"net/http"

	"balans/internal/platform/apperr"
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

// WebhookHandler mengimplementasikan ServerInterface hasil generate. Webhook memakai handler non-strict
// karena signature diverifikasi terhadap body asli.
type WebhookHandler struct {
	webhooks paymentWebhookReceiver
	rs       responder
}

var _ ServerInterface = (*WebhookHandler)(nil)

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
