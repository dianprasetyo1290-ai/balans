package http

import (
	"context"
	"net"
	"net/http"
	"strings"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/logger"
)

type authenticator interface {
	Authenticate(ctx context.Context, raw string) (actor.Actor, bool, error)
}

type errorWriter interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
}

// publicPaths tidak membutuhkan access token.
var publicPaths = map[string]bool{
	"/api/v1/system/info":          true,
	"/api/v1/auth/login":           true,
	"/api/v1/auth/refresh":         true,
	"/api/v1/auth/logout":          true,
	"/api/v1/auth/password/forgot": true,
	"/api/v1/auth/password/token":  true,
	"/api/v1/auth/password/setup":  true,
}

// mfaEnrollmentPaths tetap terbuka selama user wajib MFA tetapi belum mendaftar.
var mfaEnrollmentPaths = map[string]bool{
	"/api/v1/me":                true,
	"/api/v1/auth/mfa/setup":    true,
	"/api/v1/auth/mfa/activate": true,
}

// IsPublic menandai path yang tidak membutuhkan login. Hanya /api/v1/* yang dilindungi; webhook
// memverifikasi signature sendiri (Technical Guideline 9.3).
func IsPublic(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	return publicPaths[path] || strings.HasPrefix(path, "/api/v1/webhooks/")
}

// Middleware memasang info klien dan actor di context. Request ke path terlindungi tanpa access token
// sah ditolak AUTH_TOKEN_EXPIRED (FE refresh otomatis) atau AUTH_SESSION_EXPIRED.
func Middleware(auth authenticator, rs errorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := actor.WithClient(r.Context(), actor.Client{IP: clientIP(r), UserAgent: r.UserAgent()})
			if IsPublic(r.URL.Path) {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				rs.Error(w, r, apperr.New(apperr.CodeAuthSessionExpired))
				return
			}
			a, mfaPending, err := auth.Authenticate(ctx, strings.TrimSpace(raw))
			if err != nil {
				rs.Error(w, r, err)
				return
			}
			if mfaPending && !mfaEnrollmentPaths[r.URL.Path] && r.URL.Path != "/api/v1/auth/reauth" {
				rs.Error(w, r, apperr.New(apperr.CodeAuthMFAEnrollment))
				return
			}
			ctx = actor.With(ctx, a)
			ctx = logger.WithUserID(ctx, a.UserID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIP memakai X-Forwarded-For yang dipasang Caddy (proxy tepercaya satu-satunya di depan api).
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
