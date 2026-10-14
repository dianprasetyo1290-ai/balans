// Command storefront akan melayani API publik website e-commerce (api-storefront), dipisah dari api
// agar beban back-office tidak menjatuhkan website (Technical Guideline 2.1). Endpoint katalog,
// keranjang, dan checkout dibangun menjelang rilis website; saat ini hanya health check.
package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"balans/internal/app"
	"balans/internal/platform/config"
	"balans/internal/platform/httpx"
)

func main() {
	app.Main("storefront", func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error {
		rs := httpx.NewResponder(log, report)
		r := chi.NewRouter()
		r.Use(httpx.RequestID, httpx.AccessLog(log), httpx.Recover(rs))
		r.NotFound(rs.NotFound)
		r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
			rs.JSON(w, req, http.StatusOK, map[string]string{"status": "ok"})
		})
		return app.Serve(ctx, log, cfg.HTTPAddr, r, cfg.ShutdownTimeout)
	})
}
