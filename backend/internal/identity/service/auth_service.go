// Package service berisi use case modul identity: otentikasi, sesi, MFA, pengguna, dan role.
package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"balans/internal/audit"
	"balans/internal/identity/domain"
	"balans/internal/identity/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// mfaIssuer adalah nama yang tampil di aplikasi autentikator.
const mfaIssuer = "Balans ERP"

// rotationGrace: refresh bersamaan dari dua tab memakai token yang sama dalam jendela ini tidak dianggap pencurian.
const rotationGrace = 10 * time.Second

type store interface {
	CreateUser(ctx context.Context, in domain.UserInput, roles []domain.Role, by uuid.UUID) (uuid.UUID, error)
	ReplaceUserRoles(ctx context.Context, userID uuid.UUID, a []domain.RoleAssignment, roles []domain.Role, by uuid.UUID) error
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	FindUserByEmail(ctx context.Context, email string) (domain.User, bool, error)
	ListUsers(ctx context.Context, f postgres.UserFilter, page paging.Page) (paging.Result[domain.User], error)
	CountAllUsers(ctx context.Context) (int64, error)
	UpdateUserProfile(ctx context.Context, id uuid.UUID, version int, in domain.UserInput, by uuid.UUID) error
	SetUserActive(ctx context.Context, id uuid.UUID, version int, active bool, by uuid.UUID) error
	BumpTokenVersion(ctx context.Context, id uuid.UUID) error
	SetPassword(ctx context.Context, id uuid.UUID, hash string, at time.Time) error
	SetMFASecret(ctx context.Context, id uuid.UUID, enc string) error
	EnableMFA(ctx context.Context, id uuid.UUID) error
	ResetMFA(ctx context.Context, id, by uuid.UUID) error
	TouchLogin(ctx context.Context, id uuid.UUID, at time.Time) error
	UserPermissions(ctx context.Context, id uuid.UUID) ([]string, error)
	GetAuthState(ctx context.Context, sessionID uuid.UUID) (postgres.AuthState, error)
	UsersByRoleCode(ctx context.Context, code string, division *uuid.UUID) ([]domain.User, error)
	ListRoles(ctx context.Context) ([]domain.Role, error)
	GetRole(ctx context.Context, id uuid.UUID) (domain.Role, error)
	RolesByCodes(ctx context.Context, codes []string) ([]domain.Role, error)
	SetRolePermissions(ctx context.Context, id uuid.UUID, version int, codes []string, by uuid.UUID) error
	ListPermissions(ctx context.Context) ([]domain.Permission, error)
	SyncPermission(ctx context.Context, p domain.Permission) error
	CreateSession(ctx context.Context, n postgres.NewSession) error
	SessionByTokenHash(ctx context.Context, hash []byte) (domain.Session, bool, error)
	RevokeSession(ctx context.Context, id uuid.UUID, at time.Time, reason string, replacedBy *uuid.UUID) error
	RevokeFamily(ctx context.Context, family uuid.UUID, at time.Time, reason string) ([]uuid.UUID, error)
	RevokeUserSessions(ctx context.Context, userID uuid.UUID, at time.Time, reason string) error
	RecordLoginAttempt(ctx context.Context, emailHash []byte, userID *uuid.UUID, ip string, ok bool, at time.Time) error
	RecentFailures(ctx context.Context, emailHash []byte, since time.Time) (int, time.Time, error)
	CreatePasswordToken(ctx context.Context, userID uuid.UUID, p domain.TokenPurpose, hash []byte, expires, now time.Time, by uuid.UUID) error
	PasswordTokenByHash(ctx context.Context, hash []byte) (domain.PasswordToken, bool, error)
	MarkPasswordTokenUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

// Cipher mengenkripsi secret MFA dan menghitung hash email untuk login attempt.
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	Hash(value string) []byte
}

// Mailer mengirim email lewat modul notification (di transaksi yang sama).
type Mailer interface {
	QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error)
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Deps adalah dependensi service identity.
type Deps struct {
	Store   store
	Tx      txRunner
	Tokens  *TokenIssuer
	Cipher  Cipher
	Mailer  Mailer
	Audit   audit.Recorder
	Clock   clock.Clock
	Log     *slog.Logger
	BaseURL string // URL ERP untuk tautan di email
	// SkipMFA hanya untuk APP_ENV=local (DEV_SKIP_MFA): kode TOTP tidak diminta dan pendaftaran MFA tidak diwajibkan.
	SkipMFA bool
}

// AuthService menangani login, sesi, MFA, dan password.
type AuthService struct {
	Deps
	cache *authCache
}

// NewAuthService membuat AuthService.
func NewAuthService(d Deps) *AuthService {
	return &AuthService{Deps: d, cache: newAuthCache(d.Clock)}
}

// SessionTokens adalah hasil login atau refresh. RefreshToken kosong berarti cookie tidak perlu diganti.
type SessionTokens struct {
	AccessToken      string
	ExpiresAt        time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// Login memverifikasi email, password, dan kode TOTP.
func (s *AuthService) Login(ctx context.Context, email, password, totpCode string) (SessionTokens, error) {
	now := s.Clock.Now()
	email = domain.NormalizeEmail(email)
	emailHash := s.Cipher.Hash(email)
	client := actor.ClientFrom(ctx)

	failures, lastFailed, err := s.Store.RecentFailures(ctx, emailHash, now.Add(-domain.LockoutWindow))
	if err != nil {
		return SessionTokens{}, err
	}
	if failures >= domain.MaxFailedLogins && now.Sub(lastFailed) < domain.LockoutWindow {
		return SessionTokens{}, apperr.New(apperr.CodeAuthAccountLocked)
	}

	user, found, err := s.Store.FindUserByEmail(ctx, email)
	if err != nil {
		return SessionTokens{}, err
	}
	fail := func(code apperr.Code) (SessionTokens, error) {
		var uid *uuid.UUID
		if found {
			uid = &user.ID
		}
		if err := s.Store.RecordLoginAttempt(ctx, emailHash, uid, client.IP, false, now); err != nil {
			return SessionTokens{}, err
		}
		if found {
			actx := actor.With(ctx, actor.Actor{UserID: user.ID, FullName: user.FullName})
			if err := s.Audit.Record(actx, audit.Entry{
				Action: audit.ActionLoginFailed, EntityType: "user", EntityID: user.ID, EntityLabel: user.Email,
				Note: string(code),
			}); err != nil {
				s.Log.WarnContext(ctx, "audit login failed", "error", err)
			}
		}
		return SessionTokens{}, apperr.New(code)
	}

	if !found || !user.HasPassword() || !user.IsActive {
		domain.BurnPasswordCheck(password)
		return fail(apperr.CodeAuthInvalidCredentials)
	}
	ok, err := domain.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return SessionTokens{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return fail(apperr.CodeAuthInvalidCredentials)
	}
	if user.IsMFAEnabled && !s.SkipMFA {
		if totpCode == "" {
			// Password benar tetapi kode belum dikirim: bukan percobaan gagal.
			return SessionTokens{}, apperr.New(apperr.CodeAuthMFARequired)
		}
		valid, err := s.validateTOTP(user, totpCode, now)
		if err != nil {
			return SessionTokens{}, err
		}
		if !valid {
			return fail(apperr.CodeAuthMFAInvalid)
		}
	}

	var tokens SessionTokens
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.Store.RecordLoginAttempt(ctx, emailHash, &user.ID, client.IP, true, now); err != nil {
			return err
		}
		if err := s.Store.TouchLogin(ctx, user.ID, now); err != nil {
			return err
		}
		tokens, err = s.startSession(ctx, user, uuid.Must(uuid.NewV7()), now, now.Add(domain.SessionPolicies[domain.AudienceBackoffice].MaxAge), time.Time{})
		if err != nil {
			return err
		}
		actx := actor.With(ctx, actor.Actor{UserID: user.ID, FullName: user.FullName})
		return s.Audit.Record(actx, audit.Entry{Action: audit.ActionLogin, EntityType: "user", EntityID: user.ID, EntityLabel: user.Email})
	})
	return tokens, err
}

// startSession membuat refresh token baru dalam keluarga sesi dan menerbitkan access token.
func (s *AuthService) startSession(ctx context.Context, user domain.User, family uuid.UUID, now, expires, reauthAt time.Time) (SessionTokens, error) {
	refresh, hash, err := newOpaqueToken()
	if err != nil {
		return SessionTokens{}, err
	}
	policy := domain.SessionPolicies[domain.AudienceBackoffice]
	idle := now.Add(policy.Idle)
	if idle.After(expires) {
		idle = expires
	}
	sessionID := uuid.Must(uuid.NewV7())
	client := actor.ClientFrom(ctx)
	if err := s.Store.CreateSession(ctx, postgres.NewSession{
		ID: sessionID, UserID: user.ID, FamilyID: family, Audience: domain.AudienceBackoffice, TokenHash: hash,
		ExpiresAt: expires, IdleUntil: idle, UserAgent: truncate(client.UserAgent, 300), IP: client.IP, CreatedAt: now,
	}); err != nil {
		return SessionTokens{}, err
	}
	access, exp, err := s.Tokens.Issue(user.ID, sessionID, user.TokenVersion, reauthAt)
	if err != nil {
		return SessionTokens{}, err
	}
	return SessionTokens{AccessToken: access, ExpiresAt: exp, RefreshToken: refresh, RefreshExpiresAt: idle}, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// Refresh merotasi refresh token. Token lama yang dipakai ulang mencabut seluruh keluarga sesi.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (SessionTokens, error) {
	if refreshToken == "" {
		return SessionTokens{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	now := s.Clock.Now()
	var tokens SessionTokens
	var reuse bool
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		sess, ok, err := s.Store.SessionByTokenHash(ctx, hashOpaque(refreshToken))
		if err != nil {
			return err
		}
		if !ok {
			return apperr.New(apperr.CodeAuthSessionExpired)
		}
		user, err := s.Store.GetUser(ctx, sess.UserID)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.New(apperr.CodeAuthSessionExpired)
		}
		if err != nil {
			return err
		}
		if sess.RevokedAt != nil {
			if sess.ReplacedBy != nil && now.Sub(*sess.RevokedAt) <= rotationGrace && user.IsActive {
				// Tab lain baru saja merotasi token yang sama: terbitkan access token untuk sesi penerus,
				// cookie tidak diganti karena browser sudah menyimpan token terbaru.
				access, exp, err := s.Tokens.Issue(user.ID, *sess.ReplacedBy, user.TokenVersion, time.Time{})
				tokens = SessionTokens{AccessToken: access, ExpiresAt: exp}
				return err
			}
			if sess.ReplacedBy != nil {
				reuse = true
				ids, err := s.Store.RevokeFamily(ctx, sess.FamilyID, now, domain.RevokeReuse)
				s.cache.invalidateSessions(ids)
				if err != nil {
					return err
				}
			}
			return apperr.New(apperr.CodeAuthSessionExpired)
		}
		if !now.Before(sess.ExpiresAt) || !now.Before(sess.IdleExpiresAt) || !user.IsActive {
			if err := s.Store.RevokeSession(ctx, sess.ID, now, domain.RevokeExpired, nil); err != nil {
				return err
			}
			return apperr.New(apperr.CodeAuthSessionExpired)
		}
		tokens, err = s.startSession(ctx, user, sess.FamilyID, now, sess.ExpiresAt, time.Time{})
		if err != nil {
			return err
		}
		claims, err := s.Tokens.Parse(tokens.AccessToken)
		if err != nil {
			return err
		}
		return s.Store.RevokeSession(ctx, sess.ID, now, domain.RevokeRotated, &claims.SessionID)
	})
	if reuse && apperr.HasCode(err, apperr.CodeAuthSessionExpired) {
		s.Log.WarnContext(ctx, "refresh token reuse detected; session family revoked")
		// Pencabutan keluarga sesi harus tetap tersimpan walaupun request ini ditolak.
		return SessionTokens{}, s.revokeFamilyOutsideTx(ctx, refreshToken, now)
	}
	return tokens, err
}

// revokeFamilyOutsideTx mengulang pencabutan di transaksi terpisah karena transaksi refresh di-rollback saat error.
func (s *AuthService) revokeFamilyOutsideTx(ctx context.Context, refreshToken string, now time.Time) error {
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		sess, ok, err := s.Store.SessionByTokenHash(ctx, hashOpaque(refreshToken))
		if err != nil || !ok {
			return err
		}
		ids, err := s.Store.RevokeFamily(ctx, sess.FamilyID, now, domain.RevokeReuse)
		s.cache.invalidateSessions(ids)
		return err
	})
	if err != nil {
		return err
	}
	return apperr.New(apperr.CodeAuthSessionExpired)
}

// Logout mencabut keluarga sesi dari refresh token. Token yang tidak dikenal diabaikan.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	now := s.Clock.Now()
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		sess, ok, err := s.Store.SessionByTokenHash(ctx, hashOpaque(refreshToken))
		if err != nil || !ok {
			return err
		}
		ids, err := s.Store.RevokeFamily(ctx, sess.FamilyID, now, domain.RevokeLogout)
		if err != nil {
			return err
		}
		s.cache.invalidateSessions(ids)
		user, err := s.Store.GetUser(ctx, sess.UserID)
		if err != nil {
			return nil //nolint:nilerr // user sudah dihapus; sesi tetap dicabut
		}
		actx := actor.With(ctx, actor.Actor{UserID: user.ID, FullName: user.FullName})
		return s.Audit.Record(actx, audit.Entry{Action: audit.ActionLogout, EntityType: "user", EntityID: user.ID, EntityLabel: user.Email})
	})
}

// Reauth memeriksa password atau TOTP lalu menerbitkan access token dengan reauth_at.
func (s *AuthService) Reauth(ctx context.Context, password, totpCode string) (SessionTokens, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return SessionTokens{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	user, err := s.Store.GetUser(ctx, a.UserID)
	if err != nil {
		return SessionTokens{}, err
	}
	now := s.Clock.Now()
	switch {
	case totpCode != "" && user.IsMFAEnabled:
		valid, err := s.validateTOTP(user, totpCode, now)
		if err != nil {
			return SessionTokens{}, err
		}
		if !valid {
			return SessionTokens{}, apperr.New(apperr.CodeAuthMFAInvalid)
		}
	case password != "":
		ok, err := domain.VerifyPassword(password, user.PasswordHash)
		if err != nil {
			return SessionTokens{}, fmt.Errorf("verify password: %w", err)
		}
		if !ok {
			return SessionTokens{}, apperr.WithMessage(apperr.CodeValidationFailed, "Password salah.")
		}
	default:
		return SessionTokens{}, apperr.Validation(map[string]string{"password": "Masukkan password atau kode autentikasi"}) //nolint:gosec // pesan validasi, bukan kredensial
	}
	access, exp, err := s.Tokens.Issue(user.ID, a.SessionID, user.TokenVersion, now)
	if err != nil {
		return SessionTokens{}, err
	}
	return SessionTokens{AccessToken: access, ExpiresAt: exp}, nil
}

func (s *AuthService) validateTOTP(user domain.User, code string, now time.Time) (bool, error) {
	secret, err := s.Cipher.Decrypt(user.MFASecretEnc)
	if err != nil {
		return false, fmt.Errorf("decrypt mfa secret: %w", err)
	}
	ok, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return false, nil //nolint:nilerr // kode berformat salah diperlakukan sebagai kode salah
	}
	return ok, nil
}

// CurrentUser adalah data /me.
type CurrentUser struct {
	User        domain.User
	Permissions []string
	MFAPending  bool
}

// Me mengembalikan user login beserta permission.
func (s *AuthService) Me(ctx context.Context) (CurrentUser, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return CurrentUser{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	user, err := s.Store.GetUser(ctx, a.UserID)
	if err != nil {
		return CurrentUser{}, err
	}
	perms, err := s.Store.UserPermissions(ctx, a.UserID)
	if err != nil {
		return CurrentUser{}, err
	}
	if perms == nil {
		perms = []string{}
	}
	return CurrentUser{User: user, Permissions: perms, MFAPending: user.MFAEnrollmentPending() && !s.SkipMFA}, nil
}

// MFASetup adalah secret baru untuk didaftarkan.
type MFASetup struct {
	Secret     string
	OtpauthURL string
	QRDataURL  string
}

// SetupMFA membuat secret TOTP baru. MFA belum aktif sampai kode pertama dikonfirmasi.
func (s *AuthService) SetupMFA(ctx context.Context) (MFASetup, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return MFASetup{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	user, err := s.Store.GetUser(ctx, a.UserID)
	if err != nil {
		return MFASetup{}, err
	}
	if user.IsMFAEnabled {
		return MFASetup{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Autentikasi dua langkah sudah aktif. Minta Admin Sistem mereset jika HP hilang.")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: mfaIssuer, AccountName: user.Email})
	if err != nil {
		return MFASetup{}, fmt.Errorf("generate totp: %w", err)
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return MFASetup{}, fmt.Errorf("totp qr: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return MFASetup{}, fmt.Errorf("encode qr: %w", err)
	}
	enc, err := s.Cipher.Encrypt(key.Secret())
	if err != nil {
		return MFASetup{}, err
	}
	if err := s.Store.SetMFASecret(ctx, user.ID, enc); err != nil {
		return MFASetup{}, err
	}
	return MFASetup{
		Secret:     key.Secret(),
		OtpauthURL: key.URL(),
		QRDataURL:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// ActivateMFA mengaktifkan MFA setelah kode pertama benar.
func (s *AuthService) ActivateMFA(ctx context.Context, code string) error {
	a, ok := actor.From(ctx)
	if !ok {
		return apperr.New(apperr.CodeAuthSessionExpired)
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := s.Store.GetUser(ctx, a.UserID)
		if err != nil {
			return err
		}
		if user.MFASecretEnc == "" {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Mulai pendaftaran autentikasi dua langkah terlebih dahulu.")
		}
		valid, err := s.validateTOTP(user, code, s.Clock.Now())
		if err != nil {
			return err
		}
		if !valid {
			return apperr.New(apperr.CodeAuthMFAInvalid)
		}
		if err := s.Store.EnableMFA(ctx, user.ID); err != nil {
			return err
		}
		s.cache.invalidateUser(user.ID)
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "user", EntityID: user.ID, EntityLabel: user.Email,
			Before: map[string]any{"isMfaEnabled": false}, After: map[string]any{"isMfaEnabled": true},
		})
	})
}

// RequestPasswordReset mengirim tautan atur ulang jika email terdaftar dan aktif. Selalu sukses.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	email = domain.NormalizeEmail(email)
	user, found, err := s.Store.FindUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !found || !user.IsActive {
		return nil
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		return s.sendPasswordLink(ctx, user, domain.PurposeReset, uuid.Nil)
	})
}

// sendPasswordLink membuat tautan dan mengantrekan email di transaksi aktif.
func (s *AuthService) sendPasswordLink(ctx context.Context, user domain.User, purpose domain.TokenPurpose, by uuid.UUID) error {
	token, hash, err := newOpaqueToken()
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	ttl := domain.ResetTokenTTL
	if purpose == domain.PurposeInvite {
		ttl = domain.InviteTokenTTL
	}
	if err := s.Store.CreatePasswordToken(ctx, user.ID, purpose, hash, now.Add(ttl), now, by); err != nil {
		return err
	}
	link := s.BaseURL + "/set-password?token=" + token
	var subject, body string
	if purpose == domain.PurposeInvite {
		subject = "Undangan Balans ERP"
		body = fmt.Sprintf("Halo %s,\n\nAnda diundang memakai Balans ERP dengan email %s.\n"+
			"Atur password Anda lewat tautan berikut (berlaku 72 jam):\n\n%s\n\n"+
			"Jika Anda tidak merasa diundang, abaikan email ini.\n", user.FullName, user.Email, link)
	} else {
		subject = "Atur ulang password Balans ERP"
		body = fmt.Sprintf("Halo %s,\n\nKami menerima permintaan atur ulang password untuk akun %s.\n"+
			"Atur password baru lewat tautan berikut (berlaku 1 jam):\n\n%s\n\n"+
			"Jika Anda tidak meminta, abaikan email ini; password Anda tidak berubah.\n", user.FullName, user.Email, link)
	}
	_, err = s.Mailer.QueueEmail(ctx, user.Email, subject, body)
	return err
}

// PasswordTokenInfo memeriksa tautan untuk halaman atur password.
func (s *AuthService) PasswordTokenInfo(ctx context.Context, token string) (domain.PasswordToken, error) {
	var info domain.PasswordToken
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, ok, err := s.Store.PasswordTokenByHash(ctx, hashOpaque(token))
		if err != nil {
			return err
		}
		if !ok || !t.Usable(s.Clock.Now()) {
			return apperr.New(apperr.CodeAuthPasswordToken)
		}
		info = t
		return nil
	})
	return info, err
}

// SetupPassword menyimpan password dari tautan undangan/atur ulang dan mencabut semua sesi lama.
func (s *AuthService) SetupPassword(ctx context.Context, token, password string) error {
	now := s.Clock.Now()
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, ok, err := s.Store.PasswordTokenByHash(ctx, hashOpaque(token))
		if err != nil {
			return err
		}
		if !ok || !t.Usable(now) {
			return apperr.New(apperr.CodeAuthPasswordToken)
		}
		if err := domain.ValidatePassword(password, t.Email); err != nil {
			return err
		}
		hash, err := domain.HashPassword(password)
		if err != nil {
			return err
		}
		if err := s.Store.SetPassword(ctx, t.UserID, hash, now); err != nil {
			return err
		}
		if err := s.Store.MarkPasswordTokenUsed(ctx, t.ID, now); err != nil {
			return err
		}
		if err := s.Store.RevokeUserSessions(ctx, t.UserID, now, domain.RevokePassword); err != nil {
			return err
		}
		s.cache.invalidateUser(t.UserID)
		actx := actor.With(ctx, actor.Actor{UserID: t.UserID, FullName: t.FullName})
		return s.Audit.Record(actx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "user", EntityID: t.UserID, EntityLabel: t.Email,
			After: map[string]any{"password": "set"}, Note: "Password diatur lewat tautan " + string(t.Purpose),
		})
	})
}

// ChangePassword mengganti password sendiri; sesi lain dicabut, sesi ini tetap.
func (s *AuthService) ChangePassword(ctx context.Context, current, next string) (SessionTokens, error) {
	a, ok := actor.From(ctx)
	if !ok {
		return SessionTokens{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	now := s.Clock.Now()
	var tokens SessionTokens
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := s.Store.GetUser(ctx, a.UserID)
		if err != nil {
			return err
		}
		ok, err := domain.VerifyPassword(current, user.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify password: %w", err)
		}
		if !ok {
			return apperr.Validation(map[string]string{"currentPassword": "Password saat ini salah"})
		}
		if err := domain.ValidatePassword(next, user.Email); err != nil {
			return err
		}
		hash, err := domain.HashPassword(next)
		if err != nil {
			return err
		}
		if err := s.Store.SetPassword(ctx, user.ID, hash, now); err != nil {
			return err
		}
		s.cache.invalidateUser(user.ID)
		// token_version naik sehingga access token ini juga dicabut; terbitkan ulang untuk sesi aktif.
		user.TokenVersion++
		access, exp, err := s.Tokens.Issue(user.ID, a.SessionID, user.TokenVersion, time.Time{})
		if err != nil {
			return err
		}
		tokens = SessionTokens{AccessToken: access, ExpiresAt: exp}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "user", EntityID: user.ID, EntityLabel: user.Email,
			After: map[string]any{"password": "changed"},
		})
	})
	return tokens, err
}

// ActorFor menyusun actor dari role dan permission terbaru user untuk proses latar (mis. job impor Excel yang
// berjalan atas nama pembuatnya). User nonaktif ditolak FORBIDDEN.
func (s *AuthService) ActorFor(ctx context.Context, userID uuid.UUID) (actor.Actor, error) {
	user, err := s.Store.GetUser(ctx, userID)
	if err != nil {
		return actor.Actor{}, err
	}
	if !user.IsActive {
		return actor.Actor{}, apperr.New(apperr.CodeForbidden)
	}
	perms, err := s.Store.UserPermissions(ctx, userID)
	if err != nil {
		return actor.Actor{}, err
	}
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.Actor{
		UserID: user.ID, Email: user.Email, FullName: user.FullName, Roles: user.RoleCodes(),
		Permissions: set, DivisionIDs: user.DivisionScope(),
	}, nil
}

// Authenticate memverifikasi access token dan menyusun actor. Status user dan sesi di-cache 30 detik.
func (s *AuthService) Authenticate(ctx context.Context, raw string) (actor.Actor, bool, error) {
	claims, err := s.Tokens.Parse(raw)
	if err != nil {
		return actor.Actor{}, false, err
	}
	entry, err := s.cache.get(ctx, claims.SessionID, s.loadAuthEntry)
	if err != nil {
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return actor.Actor{}, false, apperr.New(apperr.CodeAuthSessionExpired)
		}
		return actor.Actor{}, false, err
	}
	if entry.revoked || !entry.active || entry.tokenVersion != claims.TokenVersion || entry.userID.String() != claims.Subject {
		return actor.Actor{}, false, apperr.New(apperr.CodeAuthSessionExpired)
	}
	a := entry.actor
	a.SessionID = claims.SessionID
	if claims.ReauthAt > 0 {
		a.ReauthAt = time.Unix(claims.ReauthAt, 0).UTC()
	}
	return a, entry.mfaPending, nil
}

func (s *AuthService) loadAuthEntry(ctx context.Context, sessionID uuid.UUID) (authEntry, error) {
	st, err := s.Store.GetAuthState(ctx, sessionID)
	if err != nil {
		return authEntry{}, err
	}
	e := authEntry{userID: st.UserID, revoked: st.SessionRevoked, active: st.Active, tokenVersion: st.TokenVersion}
	if !st.Active || st.SessionRevoked {
		return e, nil
	}
	user, err := s.Store.GetUser(ctx, st.UserID)
	if err != nil {
		return authEntry{}, err
	}
	perms, err := s.Store.UserPermissions(ctx, st.UserID)
	if err != nil {
		return authEntry{}, err
	}
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	e.mfaPending = user.MFAEnrollmentPending() && !s.SkipMFA
	e.actor = actor.Actor{
		UserID: user.ID, Email: user.Email, FullName: user.FullName, Roles: user.RoleCodes(),
		Permissions: set, DivisionIDs: user.DivisionScope(),
	}
	return e, nil
}
