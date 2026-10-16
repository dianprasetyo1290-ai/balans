// Package postgres adalah repository modul identity.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/identity/domain"
	"balans/internal/identity/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// Store menyimpan user, role, permission, sesi, dan tautan password.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- User -------------------------------------------------------------------------------------

// CreateUser menyimpan user baru beserta role-nya.
func (s *Store) CreateUser(ctx context.Context, in domain.UserInput, roles []domain.Role, by uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertUser(ctx, sqlcgen.InsertUserParams{
		Email: in.Email, FullName: in.FullName, DivisionID: nullUUID(in.DivisionID),
		EmployeeID: nullUUID(in.EmployeeID), CreatedBy: by,
	})
	if isUniqueViolation(err) {
		return uuid.Nil, apperr.Validation(map[string]string{"email": "Email sudah dipakai pengguna lain"})
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user: %w", err)
	}
	return id, s.ReplaceUserRoles(ctx, id, in.Roles, roles, by)
}

// ReplaceUserRoles mengganti seluruh role user.
func (s *Store) ReplaceUserRoles(ctx context.Context, userID uuid.UUID, assignments []domain.RoleAssignment, roles []domain.Role, by uuid.UUID) error {
	byCode := map[string]uuid.UUID{}
	for _, r := range roles {
		byCode[r.Code] = r.ID
	}
	if err := s.q(ctx).DeleteUserRoles(ctx, userID); err != nil {
		return fmt.Errorf("delete user roles: %w", err)
	}
	for _, a := range assignments {
		if err := s.q(ctx).InsertUserRole(ctx, sqlcgen.InsertUserRoleParams{
			UserID: userID, RoleID: byCode[a.Code], ScopeDivisionID: nullUUID(a.ScopeDivisionID), CreatedBy: by,
		}); err != nil {
			return fmt.Errorf("insert user role: %w", err)
		}
	}
	return nil
}

func userFromRow(r sqlcgen.GetUserByIDRow) domain.User {
	return domain.User{
		ID: r.ID, Email: r.Email, FullName: r.FullName, PasswordHash: r.PasswordHash.String, IsActive: r.IsActive,
		TokenVersion: int(r.TokenVersion), MFASecretEnc: r.MfaSecretEnc.String, IsMFAEnabled: r.IsMfaEnabled,
		EmployeeID: uuidPtr(r.EmployeeID), DivisionID: uuidPtr(r.DivisionID), LastLoginAt: r.LastLoginAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// GetUser membaca user beserta role.
func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := s.q(ctx).GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, notFound(err, "get user")
	}
	u := userFromRow(row)
	return u, s.attachRoles(ctx, []*domain.User{&u})
}

// FindUserByEmail mengembalikan ok=false jika email tidak terdaftar.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (domain.User, bool, error) {
	row, err := s.q(ctx).GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, fmt.Errorf("get user by email: %w", err)
	}
	u := userFromRow(sqlcgen.GetUserByIDRow(row))
	return u, true, s.attachRoles(ctx, []*domain.User{&u})
}

// UserFilter adalah kriteria list user.
type UserFilter struct {
	Search   string
	IsActive *bool
	Sort     paging.Sort
}

// ListUsers mengembalikan satu halaman user.
func (s *Store) ListUsers(ctx context.Context, f UserFilter, page paging.Page) (paging.Result[domain.User], error) {
	search := pgtype.Text{String: f.Search, Valid: f.Search != ""}
	active := pgtype.Bool{}
	if f.IsActive != nil {
		active = pgtype.Bool{Bool: *f.IsActive, Valid: true}
	}
	rows, err := s.q(ctx).ListUsers(ctx, sqlcgen.ListUsersParams{
		Search: search, IsActive: active, Sort: f.Sort.Key(), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.User]{}, fmt.Errorf("list users: %w", err)
	}
	total, err := s.q(ctx).CountUsers(ctx, sqlcgen.CountUsersParams{Search: search, IsActive: active})
	if err != nil {
		return paging.Result[domain.User]{}, fmt.Errorf("count users: %w", err)
	}
	users := make([]domain.User, len(rows))
	ptrs := make([]*domain.User, len(rows))
	for i, r := range rows {
		users[i] = domain.User{
			ID: r.ID, Email: r.Email, FullName: r.FullName, IsActive: r.IsActive, IsMFAEnabled: r.IsMfaEnabled,
			EmployeeID: uuidPtr(r.EmployeeID), DivisionID: uuidPtr(r.DivisionID), LastLoginAt: r.LastLoginAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
		}
		if r.HasPassword {
			users[i].PasswordHash = "set"
		}
		ptrs[i] = &users[i]
	}
	if err := s.attachRoles(ctx, ptrs); err != nil {
		return paging.Result[domain.User]{}, err
	}
	return paging.Result[domain.User]{Items: users, Page: page, Total: total}, nil
}

func (s *Store) attachRoles(ctx context.Context, users []*domain.User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(users))
	index := map[uuid.UUID]*domain.User{}
	for i, u := range users {
		ids[i] = u.ID
		index[u.ID] = u
		u.Roles = []domain.UserRole{}
	}
	rows, err := s.q(ctx).ListUserRoles(ctx, ids)
	if err != nil {
		return fmt.Errorf("list user roles: %w", err)
	}
	for _, r := range rows {
		u := index[r.UserID]
		u.Roles = append(u.Roles, domain.UserRole{
			RoleID: r.RoleID, Code: r.Code, Name: r.Name, RequiresMFA: r.RequiresMfa,
			HasDivisionScope: r.HasDivisionScope, ScopeDivisionID: uuidPtr(r.ScopeDivisionID),
		})
	}
	return nil
}

// CountAllUsers termasuk user yang dihapus; dipakai untuk bootstrap admin pertama.
func (s *Store) CountAllUsers(ctx context.Context) (int64, error) {
	n, err := s.q(ctx).CountAllUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func versionConflict(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeVersionConflict, fmt.Errorf("%s: version mismatch", what))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// UpdateUserProfile mengubah nama, divisi, dan karyawan dengan optimistic locking.
func (s *Store) UpdateUserProfile(ctx context.Context, id uuid.UUID, version int, in domain.UserInput, by uuid.UUID) error {
	_, err := s.q(ctx).UpdateUserProfile(ctx, sqlcgen.UpdateUserProfileParams{
		ID: id, Version: int32(version), FullName: in.FullName, DivisionID: nullUUID(in.DivisionID), //nolint:gosec // versi kecil
		EmployeeID: nullUUID(in.EmployeeID), UpdatedBy: by,
	})
	return versionConflict(err, "update user")
}

// SetUserActive mengaktifkan/menonaktifkan user dan menaikkan token_version.
func (s *Store) SetUserActive(ctx context.Context, id uuid.UUID, version int, active bool, by uuid.UUID) error {
	_, err := s.q(ctx).SetUserActive(ctx, sqlcgen.SetUserActiveParams{
		ID: id, Version: int32(version), IsActive: active, UpdatedBy: by, //nolint:gosec // versi kecil
	})
	return versionConflict(err, "set user active")
}

// BumpTokenVersion mencabut semua access token user.
func (s *Store) BumpTokenVersion(ctx context.Context, id uuid.UUID) error {
	if err := s.q(ctx).BumpUserTokenVersion(ctx, id); err != nil {
		return fmt.Errorf("bump token version: %w", err)
	}
	return nil
}

// SetPassword menyimpan hash password baru dan menaikkan token_version.
func (s *Store) SetPassword(ctx context.Context, id uuid.UUID, hash string, at time.Time) error {
	if err := s.q(ctx).SetUserPassword(ctx, sqlcgen.SetUserPasswordParams{
		UpdatedBy: id, PasswordHash: pgtype.Text{String: hash, Valid: true}, PasswordChangedAt: &at,
	}); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

// SetMFASecret menyimpan secret TOTP yang belum aktif.
func (s *Store) SetMFASecret(ctx context.Context, id uuid.UUID, enc string) error {
	if err := s.q(ctx).SetUserMFASecret(ctx, sqlcgen.SetUserMFASecretParams{ID: id, MfaSecretEnc: pgtype.Text{String: enc, Valid: true}}); err != nil {
		return fmt.Errorf("set mfa secret: %w", err)
	}
	return nil
}

// EnableMFA mengaktifkan MFA.
func (s *Store) EnableMFA(ctx context.Context, id uuid.UUID) error {
	if err := s.q(ctx).EnableUserMFA(ctx, id); err != nil {
		return fmt.Errorf("enable mfa: %w", err)
	}
	return nil
}

// ResetMFA menghapus secret MFA dan mencabut sesi.
func (s *Store) ResetMFA(ctx context.Context, id, by uuid.UUID) error {
	if err := s.q(ctx).ResetUserMFA(ctx, sqlcgen.ResetUserMFAParams{ID: id, UpdatedBy: by}); err != nil {
		return fmt.Errorf("reset mfa: %w", err)
	}
	return nil
}

// TouchLogin mencatat waktu login terakhir.
func (s *Store) TouchLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := s.q(ctx).TouchUserLogin(ctx, sqlcgen.TouchUserLoginParams{ID: id, LastLoginAt: &at}); err != nil {
		return fmt.Errorf("touch login: %w", err)
	}
	return nil
}

// UserPermissions mengembalikan kode permission user dari seluruh role-nya.
func (s *Store) UserPermissions(ctx context.Context, id uuid.UUID) ([]string, error) {
	codes, err := s.q(ctx).ListUserPermissionCodes(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list user permissions: %w", err)
	}
	return codes, nil
}

// AuthState adalah status sesi dan user untuk validasi access token.
type AuthState struct {
	SessionRevoked bool
	SessionExpires time.Time
	UserID         uuid.UUID
	Email          string
	FullName       string
	Active         bool
	TokenVersion   int
}

// GetAuthState membaca status sesi. Sesi yang dicabut karena rotasi tetap sah.
func (s *Store) GetAuthState(ctx context.Context, sessionID uuid.UUID) (AuthState, error) {
	r, err := s.q(ctx).GetAuthState(ctx, sessionID)
	if err != nil {
		return AuthState{}, notFound(err, "get auth state")
	}
	revoked := r.RevokedAt != nil && r.RevokeReason.String != domain.RevokeRotated
	return AuthState{
		SessionRevoked: revoked, SessionExpires: r.ExpiresAt, UserID: r.UserID, Email: r.Email, FullName: r.FullName,
		Active: r.IsActive && r.DeletedAt == nil, TokenVersion: int(r.TokenVersion),
	}, nil
}

// UsersByRoleCode mengembalikan user aktif dengan role tertentu (opsional dibatasi divisi).
func (s *Store) UsersByRoleCode(ctx context.Context, code string, division *uuid.UUID) ([]domain.User, error) {
	rows, err := s.q(ctx).ListActiveUsersByRoleCode(ctx, sqlcgen.ListActiveUsersByRoleCodeParams{Code: code, DivisionID: nullUUID(division)})
	if err != nil {
		return nil, fmt.Errorf("list users by role: %w", err)
	}
	out := make([]domain.User, len(rows))
	for i, r := range rows {
		out[i] = domain.User{ID: r.ID, Email: r.Email, FullName: r.FullName, IsActive: true}
	}
	return out, nil
}

// --- Role & permission ------------------------------------------------------------------------

// ListRoles mengembalikan semua role beserta permission.
func (s *Store) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := s.q(ctx).ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	pairs, err := s.q(ctx).ListRolePermissionPairs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	perms := map[uuid.UUID][]string{}
	for _, p := range pairs {
		perms[p.RoleID] = append(perms[p.RoleID], p.Code)
	}
	out := make([]domain.Role, len(rows))
	for i, r := range rows {
		out[i] = domain.Role{
			ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, RequiresMFA: r.RequiresMfa,
			HasDivisionScope: r.HasDivisionScope, Version: int(r.Version), Permissions: perms[r.ID],
		}
		if out[i].Permissions == nil {
			out[i].Permissions = []string{}
		}
	}
	return out, nil
}

// GetRole membaca role beserta permission.
func (s *Store) GetRole(ctx context.Context, id uuid.UUID) (domain.Role, error) {
	r, err := s.q(ctx).GetRoleByID(ctx, id)
	if err != nil {
		return domain.Role{}, notFound(err, "get role")
	}
	codes, err := s.q(ctx).ListRolePermissionCodes(ctx, id)
	if err != nil {
		return domain.Role{}, fmt.Errorf("list role permissions: %w", err)
	}
	if codes == nil {
		codes = []string{}
	}
	return domain.Role{
		ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, RequiresMFA: r.RequiresMfa,
		HasDivisionScope: r.HasDivisionScope, Version: int(r.Version), Permissions: codes,
	}, nil
}

// RolesByCodes membaca role dari kodenya.
func (s *Store) RolesByCodes(ctx context.Context, codes []string) ([]domain.Role, error) {
	rows, err := s.q(ctx).ListRolesByCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("list roles by codes: %w", err)
	}
	out := make([]domain.Role, len(rows))
	for i, r := range rows {
		out[i] = domain.Role{ID: r.ID, Code: r.Code, Name: r.Name, RequiresMFA: r.RequiresMfa, HasDivisionScope: r.HasDivisionScope}
	}
	return out, nil
}

// SetRolePermissions mengganti permission role dengan optimistic locking.
func (s *Store) SetRolePermissions(ctx context.Context, id uuid.UUID, version int, codes []string, by uuid.UUID) error {
	if _, err := s.q(ctx).BumpRoleVersion(ctx, sqlcgen.BumpRoleVersionParams{ID: id, Version: int32(version), UpdatedBy: by}); err != nil { //nolint:gosec // versi kecil
		return versionConflict(err, "set role permissions")
	}
	if err := s.q(ctx).DeleteRolePermissions(ctx, id); err != nil {
		return fmt.Errorf("delete role permissions: %w", err)
	}
	if err := s.q(ctx).InsertRolePermissions(ctx, sqlcgen.InsertRolePermissionsParams{RoleID: id, CreatedBy: by, Codes: codes}); err != nil {
		return fmt.Errorf("insert role permissions: %w", err)
	}
	return nil
}

// ListPermissions mengembalikan katalog permission tersimpan.
func (s *Store) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := s.q(ctx).ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	out := make([]domain.Permission, len(rows))
	for i, r := range rows {
		out[i] = domain.Permission{Code: r.Code, Module: r.Module, Description: r.Description, RequiresReauth: r.RequiresReauth}
	}
	return out, nil
}

// SyncPermission menyimpan permission dari katalog kode. Role default hanya diberikan saat permission
// pertama kali dibuat, sehingga perubahan hak akses oleh Admin tidak ditimpa saat deploy berikutnya.
func (s *Store) SyncPermission(ctx context.Context, p domain.Permission) error {
	row, err := s.q(ctx).UpsertPermission(ctx, sqlcgen.UpsertPermissionParams{
		Code: p.Code, Module: p.Module, Description: p.Description, RequiresReauth: p.RequiresReauth,
	})
	if err != nil {
		return fmt.Errorf("upsert permission %s: %w", p.Code, err)
	}
	if !row.Inserted {
		return nil
	}
	for _, role := range p.DefaultRoles {
		if err := s.q(ctx).GrantPermissionByRoleCode(ctx, sqlcgen.GrantPermissionByRoleCodeParams{PermissionID: row.ID, RoleCode: role}); err != nil {
			return fmt.Errorf("grant %s to %s: %w", p.Code, role, err)
		}
	}
	return nil
}

// --- Sesi ------------------------------------------------------------------------------------

// NewSession adalah sesi baru yang akan disimpan.
type NewSession struct {
	ID, UserID, FamilyID uuid.UUID
	Audience             domain.Audience
	TokenHash            []byte
	ExpiresAt, IdleUntil time.Time
	UserAgent, IP        string
	CreatedAt            time.Time
}

// CreateSession menyimpan sesi.
func (s *Store) CreateSession(ctx context.Context, n NewSession) error {
	if err := s.q(ctx).InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID: n.ID, UserID: n.UserID, Audience: string(n.Audience), FamilyID: n.FamilyID, RefreshTokenHash: n.TokenHash,
		ExpiresAt: n.ExpiresAt, IdleExpiresAt: n.IdleUntil, UserAgent: n.UserAgent, Ip: n.IP, CreatedAt: n.CreatedAt,
	}); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// SessionByTokenHash mengunci sesi dari hash refresh token; ok=false jika tidak ada.
func (s *Store) SessionByTokenHash(ctx context.Context, hash []byte) (domain.Session, bool, error) {
	r, err := s.q(ctx).GetSessionByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, false, nil
	}
	if err != nil {
		return domain.Session{}, false, fmt.Errorf("get session: %w", err)
	}
	return domain.Session{
		ID: r.ID, UserID: r.UserID, Audience: domain.Audience(r.Audience), FamilyID: r.FamilyID,
		ExpiresAt: r.ExpiresAt, IdleExpiresAt: r.IdleExpiresAt, RevokedAt: r.RevokedAt, ReplacedBy: uuidPtr(r.ReplacedBy),
	}, true, nil
}

// RevokeSession mencabut satu sesi.
func (s *Store) RevokeSession(ctx context.Context, id uuid.UUID, at time.Time, reason string, replacedBy *uuid.UUID) error {
	if err := s.q(ctx).RevokeSession(ctx, sqlcgen.RevokeSessionParams{
		ID: id, RevokedAt: &at, RevokeReason: pgtype.Text{String: reason, Valid: true}, ReplacedBy: nullUUID(replacedBy),
	}); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeFamily mencabut seluruh sesi dalam satu keluarga dan mengembalikan ID sesi yang dicabut.
func (s *Store) RevokeFamily(ctx context.Context, family uuid.UUID, at time.Time, reason string) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).RevokeSessionFamily(ctx, sqlcgen.RevokeSessionFamilyParams{
		FamilyID: family, RevokedAt: &at, RevokeReason: pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("revoke session family: %w", err)
	}
	return ids, nil
}

// RevokeUserSessions mencabut seluruh sesi user.
func (s *Store) RevokeUserSessions(ctx context.Context, userID uuid.UUID, at time.Time, reason string) error {
	if _, err := s.q(ctx).RevokeUserSessions(ctx, sqlcgen.RevokeUserSessionsParams{
		UserID: userID, RevokedAt: &at, RevokeReason: pgtype.Text{String: reason, Valid: true},
	}); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// --- Login attempt & tautan password ----------------------------------------------------------

// RecordLoginAttempt mencatat percobaan login.
func (s *Store) RecordLoginAttempt(ctx context.Context, emailHash []byte, userID *uuid.UUID, ip string, ok bool, at time.Time) error {
	if err := s.q(ctx).InsertLoginAttempt(ctx, sqlcgen.InsertLoginAttemptParams{
		EmailHash: emailHash, UserID: nullUUID(userID), Ip: ip, Succeeded: ok, CreatedAt: at,
	}); err != nil {
		return fmt.Errorf("insert login attempt: %w", err)
	}
	return nil
}

// RecentFailures menghitung gagal login sejak `since` setelah login sukses terakhir.
func (s *Store) RecentFailures(ctx context.Context, emailHash []byte, since time.Time) (int, time.Time, error) {
	r, err := s.q(ctx).RecentFailedLogins(ctx, sqlcgen.RecentFailedLoginsParams{EmailHash: emailHash, CreatedAt: since})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("recent failed logins: %w", err)
	}
	return int(r.Failures), r.LastFailedAt, nil
}

// CreatePasswordToken menyimpan hash tautan; tautan lama user dibatalkan.
func (s *Store) CreatePasswordToken(ctx context.Context, userID uuid.UUID, purpose domain.TokenPurpose, hash []byte, expires, now time.Time, by uuid.UUID) error {
	if err := s.q(ctx).InvalidateUserPasswordTokens(ctx, sqlcgen.InvalidateUserPasswordTokensParams{UserID: userID, UsedAt: &now}); err != nil {
		return fmt.Errorf("invalidate password tokens: %w", err)
	}
	if err := s.q(ctx).InsertPasswordToken(ctx, sqlcgen.InsertPasswordTokenParams{
		UserID: userID, Purpose: string(purpose), TokenHash: hash, ExpiresAt: expires, CreatedBy: by,
	}); err != nil {
		return fmt.Errorf("insert password token: %w", err)
	}
	return nil
}

// PasswordTokenByHash mengunci tautan; ok=false jika tidak ada.
func (s *Store) PasswordTokenByHash(ctx context.Context, hash []byte) (domain.PasswordToken, bool, error) {
	r, err := s.q(ctx).GetPasswordTokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PasswordToken{}, false, nil
	}
	if err != nil {
		return domain.PasswordToken{}, false, fmt.Errorf("get password token: %w", err)
	}
	return domain.PasswordToken{
		ID: r.ID, UserID: r.UserID, Purpose: domain.TokenPurpose(r.Purpose), Expires: r.ExpiresAt, UsedAt: r.UsedAt,
		Email: r.Email, FullName: r.FullName, IsActive: r.IsActive,
	}, true, nil
}

// MarkPasswordTokenUsed menandai tautan terpakai.
func (s *Store) MarkPasswordTokenUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := s.q(ctx).MarkPasswordTokenUsed(ctx, sqlcgen.MarkPasswordTokenUsedParams{ID: id, UsedAt: &at}); err != nil {
		return fmt.Errorf("mark password token used: %w", err)
	}
	return nil
}
