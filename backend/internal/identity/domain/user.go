package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// Audience memisahkan token per jenis pengguna (Technical Guideline 9.1).
type Audience string

const (
	AudienceBackoffice Audience = "backoffice"
	AudiencePartner    Audience = "partner"
	AudienceReseller   Audience = "reseller"
	AudienceCustomer   Audience = "customer"
)

// SessionPolicy adalah idle timeout dan umur maksimum sesi per audiens.
type SessionPolicy struct {
	Idle   time.Duration
	MaxAge time.Duration
}

// SessionPolicies mengikuti tabel Technical Guideline 9.1.
var SessionPolicies = map[Audience]SessionPolicy{
	AudienceBackoffice: {Idle: 60 * time.Minute, MaxAge: 12 * time.Hour},
	AudiencePartner:    {Idle: 60 * time.Minute, MaxAge: 12 * time.Hour},
	AudienceReseller:   {Idle: 7 * 24 * time.Hour, MaxAge: 30 * 24 * time.Hour},
	AudienceCustomer:   {Idle: 30 * 24 * time.Hour, MaxAge: 90 * 24 * time.Hour},
}

// Aturan kunci akun (Technical Guideline 9.1).
const (
	MaxFailedLogins = 5
	LockoutWindow   = 15 * time.Minute
)

// Masa berlaku tautan atur password.
const (
	InviteTokenTTL = 72 * time.Hour
	ResetTokenTTL  = time.Hour
)

// TokenPurpose adalah jenis tautan atur password.
type TokenPurpose string

const (
	PurposeInvite TokenPurpose = "invite"
	PurposeReset  TokenPurpose = "reset"
)

// UserRole adalah role yang dipegang user beserta scope-nya.
type UserRole struct {
	RoleID           uuid.UUID
	Code             string
	Name             string
	RequiresMFA      bool
	HasDivisionScope bool
	ScopeDivisionID  *uuid.UUID
}

// User adalah pengguna ERP.
type User struct {
	ID           uuid.UUID
	Email        string
	FullName     string
	PasswordHash string
	IsActive     bool
	TokenVersion int
	MFASecretEnc string
	IsMFAEnabled bool
	EmployeeID   *uuid.UUID
	DivisionID   *uuid.UUID
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
	Roles        []UserRole
}

// HasPassword bernilai false selama undangan belum diterima.
func (u User) HasPassword() bool { return u.PasswordHash != "" }

// RequiresMFA bernilai true jika salah satu role mewajibkan MFA.
func (u User) RequiresMFA() bool {
	for _, r := range u.Roles {
		if r.RequiresMFA {
			return true
		}
	}
	return false
}

// MFAEnrollmentPending: role mewajibkan MFA tetapi MFA belum aktif.
func (u User) MFAEnrollmentPending() bool { return u.RequiresMFA() && !u.IsMFAEnabled }

// RoleCodes mengembalikan kode role user.
func (u User) RoleCodes() []string {
	out := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		out = append(out, r.Code)
	}
	return out
}

// DivisionScope mengembalikan divisi yang menjadi scope role ber-scope divisi. Role tanpa scope eksplisit
// memakai divisi user.
func (u User) DivisionScope() []uuid.UUID {
	var out []uuid.UUID
	for _, r := range u.Roles {
		if !r.HasDivisionScope {
			continue
		}
		switch {
		case r.ScopeDivisionID != nil:
			out = append(out, *r.ScopeDivisionID)
		case u.DivisionID != nil:
			out = append(out, *u.DivisionID)
		}
	}
	return out
}

// RoleAssignment adalah role yang diberikan admin.
type RoleAssignment struct {
	Code            string
	ScopeDivisionID *uuid.UUID
}

// UserInput adalah isian pembuatan atau perubahan user.
type UserInput struct {
	Email      string
	FullName   string
	DivisionID *uuid.UUID
	EmployeeID *uuid.UUID
	Roles      []RoleAssignment
}

// Validate memeriksa isian user. requireEmail=false untuk perubahan (email tidak bisa diganti).
func (in *UserInput) Validate(requireEmail bool) error {
	fields := map[string]string{}
	in.FullName = strings.TrimSpace(in.FullName)
	if requireEmail {
		in.Email = NormalizeEmail(in.Email)
		if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email || utf8.RuneCountInString(in.Email) > 254 {
			fields["email"] = "Alamat email tidak valid"
		}
	}
	switch n := utf8.RuneCountInString(in.FullName); {
	case n == 0:
		fields["fullName"] = "Nama wajib diisi"
	case n > 120:
		fields["fullName"] = "Nama maksimal 120 karakter"
	}
	if len(in.Roles) == 0 {
		fields["roles"] = "Pilih minimal satu role"
	}
	seen := map[string]bool{}
	for _, r := range in.Roles {
		if seen[r.Code] {
			fields["roles"] = "Role tidak boleh dipilih dua kali"
		}
		seen[r.Code] = true
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Role adalah kumpulan permission.
type Role struct {
	ID               uuid.UUID
	Code             string
	Name             string
	Description      string
	RequiresMFA      bool
	HasDivisionScope bool
	Version          int
	Permissions      []string
}

// Session adalah satu refresh token dalam keluarga sesi.
type Session struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Audience      Audience
	FamilyID      uuid.UUID
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
	RevokedAt     *time.Time
	ReplacedBy    *uuid.UUID
}

// Alasan pencabutan sesi. Sesi yang dicabut karena rotasi tetap sah untuk access token yang sudah terbit.
const (
	RevokeRotated     = "rotated"
	RevokeLogout      = "logout"
	RevokeReuse       = "reuse_detected"
	RevokeExpired     = "expired"
	RevokeUserChanged = "user_changed"
	RevokePassword    = "password_changed"
)

// PasswordToken adalah tautan undangan/atur ulang yang ditemukan dari hash.
type PasswordToken struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	Purpose  TokenPurpose
	Expires  time.Time
	UsedAt   *time.Time
	Email    string
	FullName string
	IsActive bool
}

// Usable memeriksa tautan belum dipakai, belum kedaluwarsa, dan user masih aktif.
func (t PasswordToken) Usable(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.Expires) && t.IsActive
}
