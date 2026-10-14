// Package httpx berisi middleware dan penulisan response standar (Technical Guideline 8).
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"balans/internal/platform/apperr"
	"balans/internal/platform/logger"
)

const problemTypeBase = "https://docs.balans.local/errors/"

// Problem adalah body error RFC 7807 dengan field tambahan (Technical Guideline 8.3).
type Problem struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Code      apperr.Code       `json:"code"`
	Detail    string            `json:"detail"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"requestId,omitempty"`
}

// ErrorReporter mengirim error 5xx ke error tracking (Sentry).
type ErrorReporter func(ctx context.Context, err error)

// Responder menulis response sukses dan error dengan format standar.
type Responder struct {
	log    *slog.Logger
	report ErrorReporter
}

// NewResponder membuat Responder. report boleh nil.
func NewResponder(log *slog.Logger, report ErrorReporter) *Responder {
	return &Responder{log: log, report: report}
}

// JSON menulis body JSON dengan status tertentu.
func (rs *Responder) JSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		rs.log.WarnContext(r.Context(), "write response body", "error", err)
	}
}

// Error memetakan error ke problem+json. Error 5xx dicatat di log satu kali di sini dan
// tidak membocorkan detail internal ke client.
func (rs *Responder) Error(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	appErr := apperr.As(err)
	status := appErr.Code.HTTPStatus()

	problem := Problem{
		Type:      problemTypeBase + strings.ReplaceAll(strings.ToLower(string(appErr.Code)), "_", "-"),
		Title:     appErr.Code.Title(),
		Status:    status,
		Code:      appErr.Code,
		Detail:    appErr.Message,
		Fields:    appErr.Fields,
		RequestID: logger.RequestID(ctx),
	}
	if status >= http.StatusInternalServerError {
		problem.Detail = appErr.Code.DefaultMessage()
		problem.Fields = nil
		rs.log.ErrorContext(ctx, "request failed", "error", err, "code", appErr.Code)
		if rs.report != nil {
			rs.report(ctx, err)
		}
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		rs.log.WarnContext(ctx, "write problem body", "error", encodeErr)
	}
}

// NotFound dipakai router untuk route yang tidak dikenal.
func (rs *Responder) NotFound(w http.ResponseWriter, r *http.Request) {
	rs.Error(w, r, apperr.New(apperr.CodeNotFound))
}

// MethodNotAllowed dipakai router untuk method yang tidak didukung.
func (rs *Responder) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	rs.Error(w, r, apperr.WithMessage(apperr.CodeBadRequest, "Metode permintaan tidak didukung."))
}

// RequestError dipakai handler hasil oapi-codegen saat request tidak bisa dibaca (JSON rusak, parameter salah).
func (rs *Responder) RequestError(w http.ResponseWriter, r *http.Request, err error) {
	rs.Error(w, r, &apperr.Error{Code: apperr.CodeBadRequest, Message: apperr.CodeBadRequest.DefaultMessage(), Err: err})
}
