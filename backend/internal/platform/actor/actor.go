// Package actor membawa identitas user yang sedang login di context. Diisi middleware otentikasi
// modul identity dan dibaca layer service untuk cek permission (Technical Guideline 9.2) serta audit trail.
package actor

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// ReauthWindow adalah masa berlaku re-autentikasi untuk aksi sensitif (Technical Guideline 9.1).
const ReauthWindow = 5 * time.Minute

// Actor adalah user yang menjalankan request.
type Actor struct {
	UserID      uuid.UUID
	SessionID   uuid.UUID
	Email       string
	FullName    string
	Roles       []string
	Permissions map[string]struct{}
	// DivisionIDs adalah scope divisi dari role ber-scope (mis. Head Divisi).
	DivisionIDs []uuid.UUID
	ReauthAt    time.Time
}

// Client adalah informasi koneksi untuk audit trail dan login attempt.
type Client struct {
	IP        string
	UserAgent string
}

type contextKey int

const (
	actorKey contextKey = iota
	clientKey
)

// With menyimpan actor di context.
func With(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey, a)
}

// From membaca actor dari context. ok bernilai false untuk request anonim.
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey).(Actor)
	return a, ok
}

// UserID membaca ID user, atau uuid.Nil untuk request anonim dan proses sistem.
func UserID(ctx context.Context) uuid.UUID {
	if a, ok := From(ctx); ok {
		return a.UserID
	}
	return uuid.Nil
}

// WithClient menyimpan IP dan user agent.
func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

// ClientFrom membaca IP dan user agent.
func ClientFrom(ctx context.Context) Client {
	c, _ := ctx.Value(clientKey).(Client)
	return c
}

// Has memeriksa permission `<modul>.<resource>.<aksi>`.
func (a Actor) Has(permission string) bool {
	_, ok := a.Permissions[permission]
	return ok
}

// HasRole memeriksa kode role.
func (a Actor) HasRole(code string) bool {
	return slices.Contains(a.Roles, code)
}

// Require menolak request tanpa permission dengan FORBIDDEN. Request anonim ditolak AUTH_SESSION_EXPIRED.
func Require(ctx context.Context, permission string) (Actor, error) {
	a, ok := From(ctx)
	if !ok {
		return Actor{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	if !a.Has(permission) {
		return Actor{}, apperr.New(apperr.CodeForbidden)
	}
	return a, nil
}

// RequireAny lolos jika actor punya salah satu permission.
func RequireAny(ctx context.Context, permissions ...string) (Actor, error) {
	a, ok := From(ctx)
	if !ok {
		return Actor{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	for _, p := range permissions {
		if a.Has(p) {
			return a, nil
		}
	}
	return Actor{}, apperr.New(apperr.CodeForbidden)
}

// RequireReauth menolak aksi sensitif jika re-autentikasi terakhir lebih dari 5 menit lalu.
func RequireReauth(ctx context.Context, now time.Time, permission string) (Actor, error) {
	a, err := Require(ctx, permission)
	if err != nil {
		return Actor{}, err
	}
	if a.ReauthAt.IsZero() || now.Sub(a.ReauthAt) > ReauthWindow {
		return Actor{}, apperr.New(apperr.CodeAuthReauthRequired)
	}
	return a, nil
}

// System adalah actor untuk job dan proses sistem (UUID nil tercatat sebagai "Sistem" di audit).
func System(ctx context.Context) context.Context {
	return With(ctx, Actor{UserID: uuid.Nil, FullName: "Sistem"})
}
