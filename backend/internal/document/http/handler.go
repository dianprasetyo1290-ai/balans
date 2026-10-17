package http

import (
	"context"

	"github.com/go-chi/chi/v5"

	"balans/internal/document/domain"
	"balans/internal/document/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/document.yaml ../../../api/openapi.bundle.yaml

// DocumentHandler mengimplementasikan StrictServerInterface hasil generate.
type DocumentHandler struct {
	svc   *service.Service
	clock clock.Clock
}

var _ StrictServerInterface = (*DocumentHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service, c clock.Clock) *DocumentHandler {
	return &DocumentHandler{svc: svc, clock: c}
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *DocumentHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// ListDocumentSequences mengembalikan nomor terakhir per kode dokumen.
func (h *DocumentHandler) ListDocumentSequences(ctx context.Context, req ListDocumentSequencesRequestObject) (ListDocumentSequencesResponseObject, error) {
	year := 0
	if req.Params.Year != nil {
		year = *req.Params.Year
	}
	year, rows, err := h.svc.List(ctx, year)
	if err != nil {
		return nil, err
	}
	today := clock.Today(h.clock)
	exampleDate := clock.NewBusinessDate(year, today.Month(), 1)
	items := make([]DocumentSequence, len(rows))
	for i, r := range rows {
		items[i] = DocumentSequence{
			DocumentType: string(r.Info.Type), Label: r.Info.Label, AssignedAt: r.Info.AssignedAt, Year: year,
			LastNumber: r.Sequence.LastNumber, LastIssuedAt: r.Sequence.UpdatedAt,
			Example: domain.Format(r.Info.Type, exampleDate, r.Sequence.LastNumber+1),
		}
	}
	return ListDocumentSequences200JSONResponse{Year: year, Items: items}, nil
}
