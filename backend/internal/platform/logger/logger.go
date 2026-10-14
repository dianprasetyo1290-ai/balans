// Package logger menyiapkan slog JSON dengan field dari context (Technical Guideline 13).
package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	userIDKey
	moduleKey
)

// New membuat logger JSON yang otomatis menambahkan request_id, user_id, dan module dari context.
func New(w io.Writer, level string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(contextHandler{Handler: handler})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithRequestID menyimpan request ID di context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID membaca request ID dari context.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithUserID menyimpan ID user yang sedang login di context.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// WithModule menandai modul yang sedang berjalan.
func WithModule(ctx context.Context, module string) context.Context {
	return context.WithValue(ctx, moduleKey, module)
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v, ok := ctx.Value(requestIDKey).(string); ok && v != "" {
		r.AddAttrs(slog.String("request_id", v))
	}
	if v, ok := ctx.Value(userIDKey).(string); ok && v != "" {
		r.AddAttrs(slog.String("user_id", v))
	}
	if v, ok := ctx.Value(moduleKey).(string); ok && v != "" {
		r.AddAttrs(slog.String("module", v))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
