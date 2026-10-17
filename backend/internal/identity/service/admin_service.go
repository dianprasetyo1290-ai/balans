package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/identity/domain"
	"balans/internal/identity/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
)

// Permission modul identity.
const (
	PermUserRead      = "identity.user.read"
	PermUserWrite     = "identity.user.write"
	PermUserRoleWrite = "identity.user_role.write"
	PermRoleRead      = "identity.role.read"
	PermRoleWrite     = "identity.role.write"
)

// AdminService mengelola pengguna, role, dan permission (S-102, S-103).
type AdminService struct {
	*AuthService
}

// NewAdminService membuat AdminService yang berbagi cache sesi dengan AuthService.
func NewAdminService(auth *AuthService) *AdminService { return &AdminService{AuthService: auth} }

// userSnapshot adalah bentuk user untuk audit trail.
func userSnapshot(u domain.User) map[string]any {
	roles := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		roles = append(roles, r.Code)
	}
	slices.Sort(roles)
	return map[string]any{
		"fullName": u.FullName, "email": u.Email, "isActive": u.IsActive, "roles": roles,
		"divisionId": u.DivisionID, "employeeId": u.EmployeeID, "isMfaEnabled": u.IsMFAEnabled,
	}
}

// ListUsers mengembalikan daftar user.
func (s *AdminService) ListUsers(ctx context.Context, f postgres.UserFilter, page paging.Page) (paging.Result[domain.User], error) {
	if _, err := actor.Require(ctx, PermUserRead); err != nil {
		return paging.Result[domain.User]{}, err
	}
	return s.Store.ListUsers(ctx, f, page)
}

// GetUser mengembalikan detail user.
func (s *AdminService) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	if _, err := actor.Require(ctx, PermUserRead); err != nil {
		return domain.User{}, err
	}
	return s.Store.GetUser(ctx, id)
}

// resolveRoles memastikan semua kode role ada.
func (s *AdminService) resolveRoles(ctx context.Context, in []domain.RoleAssignment) ([]domain.Role, error) {
	codes := make([]string, len(in))
	for i, r := range in {
		codes[i] = r.Code
	}
	roles, err := s.Store.RolesByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	if len(roles) != len(codes) {
		return nil, apperr.Validation(map[string]string{"roles": "Role tidak dikenal"})
	}
	return roles, nil
}

// CreateUser membuat user dan mengirim undangan atur password. Pemberian role butuh re-autentikasi.
func (s *AdminService) CreateUser(ctx context.Context, in domain.UserInput) (domain.User, error) {
	a, err := actor.Require(ctx, PermUserWrite)
	if err != nil {
		return domain.User{}, err
	}
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermUserRoleWrite); err != nil {
		return domain.User{}, err
	}
	if err := in.Validate(true); err != nil {
		return domain.User{}, err
	}
	var user domain.User
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		roles, err := s.resolveRoles(ctx, in.Roles)
		if err != nil {
			return err
		}
		id, err := s.Store.CreateUser(ctx, in, roles, a.UserID)
		if err != nil {
			return err
		}
		if user, err = s.Store.GetUser(ctx, id); err != nil {
			return err
		}
		if err := s.sendPasswordLink(ctx, user, domain.PurposeInvite, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "user", EntityID: id, EntityLabel: user.Email, After: userSnapshot(user),
		})
	})
	return user, err
}

// UpdateUser mengubah nama, divisi, karyawan, dan role. Perubahan role butuh re-autentikasi dan memutus sesi.
func (s *AdminService) UpdateUser(ctx context.Context, id uuid.UUID, version int, in domain.UserInput) (domain.User, error) {
	a, err := actor.Require(ctx, PermUserWrite)
	if err != nil {
		return domain.User{}, err
	}
	if err := in.Validate(false); err != nil {
		return domain.User{}, err
	}
	var after domain.User
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		before, err := s.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		rolesChanged := !sameRoles(before.Roles, in.Roles)
		if rolesChanged {
			if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermUserRoleWrite); err != nil {
				return err
			}
			if id == a.UserID && !slices.ContainsFunc(in.Roles, func(r domain.RoleAssignment) bool { return r.Code == domain.RoleSystemAdmin }) &&
				slices.ContainsFunc(before.Roles, func(r domain.UserRole) bool { return r.Code == domain.RoleSystemAdmin }) {
				return apperr.Validation(map[string]string{"roles": "Anda tidak bisa mencabut role Admin Sistem dari akun sendiri"})
			}
		}
		if err := s.Store.UpdateUserProfile(ctx, id, version, in, a.UserID); err != nil {
			return err
		}
		if rolesChanged {
			roles, err := s.resolveRoles(ctx, in.Roles)
			if err != nil {
				return err
			}
			if err := s.Store.ReplaceUserRoles(ctx, id, in.Roles, roles, a.UserID); err != nil {
				return err
			}
			// Role berubah → token_version naik dan semua sesi dicabut (Technical Guideline 9.1).
			if err := s.Store.BumpTokenVersion(ctx, id); err != nil {
				return err
			}
			if err := s.Store.RevokeUserSessions(ctx, id, s.Clock.Now(), domain.RevokeUserChanged); err != nil {
				return err
			}
		}
		s.cache.invalidateUser(id)
		if after, err = s.Store.GetUser(ctx, id); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "user", EntityID: id, EntityLabel: after.Email,
			Before: userSnapshot(before), After: userSnapshot(after),
		})
	})
	return after, err
}

func sameRoles(current []domain.UserRole, next []domain.RoleAssignment) bool {
	if len(current) != len(next) {
		return false
	}
	for _, n := range next {
		if !slices.ContainsFunc(current, func(c domain.UserRole) bool {
			return c.Code == n.Code && equalUUIDPtr(c.ScopeDivisionID, n.ScopeDivisionID)
		}) {
			return false
		}
	}
	return true
}

func equalUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// SetUserActive mengaktifkan atau menonaktifkan user. Nonaktif memutus semua sesi dalam < 30 detik.
func (s *AdminService) SetUserActive(ctx context.Context, id uuid.UUID, version int, active bool) (domain.User, error) {
	a, err := actor.Require(ctx, PermUserWrite)
	if err != nil {
		return domain.User{}, err
	}
	if id == a.UserID && !active {
		return domain.User{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Anda tidak bisa menonaktifkan akun sendiri.")
	}
	var after domain.User
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		before, err := s.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Store.SetUserActive(ctx, id, version, active, a.UserID); err != nil {
			return err
		}
		if !active {
			if err := s.Store.RevokeUserSessions(ctx, id, s.Clock.Now(), domain.RevokeUserChanged); err != nil {
				return err
			}
		}
		s.cache.invalidateUser(id)
		if after, err = s.Store.GetUser(ctx, id); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: "user", EntityID: id, EntityLabel: after.Email,
			Before: map[string]any{"isActive": before.IsActive}, After: map[string]any{"isActive": after.IsActive},
		})
	})
	return after, err
}

// ResendInvitation mengirim ulang tautan undangan untuk user yang belum mengatur password.
func (s *AdminService) ResendInvitation(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermUserWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := s.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if !user.IsActive {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Pengguna nonaktif tidak bisa diundang.")
		}
		purpose := domain.PurposeInvite
		if user.HasPassword() {
			purpose = domain.PurposeReset
		}
		return s.sendPasswordLink(ctx, user, purpose, a.UserID)
	})
}

// ResetMFA menghapus MFA user (HP hilang). User mendaftar ulang saat login berikutnya.
func (s *AdminService) ResetMFA(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermUserWrite)
	if err != nil {
		return err
	}
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermUserRoleWrite); err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := s.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Store.ResetMFA(ctx, id, a.UserID); err != nil {
			return err
		}
		if err := s.Store.RevokeUserSessions(ctx, id, s.Clock.Now(), domain.RevokeUserChanged); err != nil {
			return err
		}
		s.cache.invalidateUser(id)
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "user", EntityID: id, EntityLabel: user.Email,
			Before: map[string]any{"isMfaEnabled": user.IsMFAEnabled}, After: map[string]any{"isMfaEnabled": false},
			Note: "MFA direset Admin Sistem",
		})
	})
}

// ListRoles mengembalikan role beserta permission.
func (s *AdminService) ListRoles(ctx context.Context) ([]domain.Role, error) {
	if _, err := actor.RequireAny(ctx, PermRoleRead, PermUserRead); err != nil {
		return nil, err
	}
	return s.Store.ListRoles(ctx)
}

// ListPermissions mengembalikan katalog permission.
func (s *AdminService) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	if _, err := actor.RequireAny(ctx, PermRoleRead, PermUserRead); err != nil {
		return nil, err
	}
	return s.Store.ListPermissions(ctx)
}

// SetRolePermissions mengganti permission role; berlaku langsung untuk semua pemegang role.
func (s *AdminService) SetRolePermissions(ctx context.Context, id uuid.UUID, version int, codes []string) (domain.Role, error) {
	a, err := actor.RequireReauth(ctx, s.Clock.Now(), PermRoleWrite)
	if err != nil {
		return domain.Role{}, err
	}
	var after domain.Role
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		known, err := s.Store.ListPermissions(ctx)
		if err != nil {
			return err
		}
		valid := map[string]bool{}
		for _, p := range known {
			valid[p.Code] = true
		}
		codes = slices.Compact(slices.Sorted(slices.Values(codes)))
		for _, c := range codes {
			if !valid[c] {
				return apperr.Validation(map[string]string{"permissions": fmt.Sprintf("Permission %s tidak dikenal", c)})
			}
		}
		before, err := s.Store.GetRole(ctx, id)
		if err != nil {
			return err
		}
		// Admin Sistem tidak boleh kehilangan akses mengatur role, agar sistem tidak terkunci.
		if before.Code == domain.RoleSystemAdmin && (!slices.Contains(codes, PermRoleWrite) || !slices.Contains(codes, PermUserRoleWrite)) {
			return apperr.Validation(map[string]string{"permissions": "Role Admin Sistem wajib memegang hak mengatur role dan pengguna"})
		}
		if err := s.Store.SetRolePermissions(ctx, id, version, codes, a.UserID); err != nil {
			return err
		}
		if after, err = s.Store.GetRole(ctx, id); err != nil {
			return err
		}
		s.cache.invalidateAll()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "role", EntityID: id, EntityLabel: after.Name,
			Before: map[string]any{"permissions": before.Permissions}, After: map[string]any{"permissions": after.Permissions},
		})
	})
	return after, err
}

// SyncCatalog menyemai permission dari kode saat proses api start.
func (s *AdminService) SyncCatalog(ctx context.Context) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, p := range domain.Catalog() {
			if err := s.Store.SyncPermission(ctx, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// BootstrapAdmin mengundang Admin Sistem pertama jika database belum punya user sama sekali.
func (s *AdminService) BootstrapAdmin(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		n, err := s.Store.CountAllUsers(ctx)
		if err != nil || n > 0 {
			return err
		}
		in := domain.UserInput{Email: email, FullName: "Admin Sistem", Roles: []domain.RoleAssignment{{Code: domain.RoleSystemAdmin}}}
		if err := in.Validate(true); err != nil {
			return fmt.Errorf("INITIAL_ADMIN_EMAIL: %w", err)
		}
		roles, err := s.resolveRoles(ctx, in.Roles)
		if err != nil {
			return err
		}
		id, err := s.Store.CreateUser(ctx, in, roles, uuid.Nil)
		if err != nil {
			return err
		}
		user, err := s.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		s.Log.InfoContext(ctx, "initial admin invited", "user_id", id)
		ctx = actor.System(ctx)
		if err := s.sendPasswordLink(ctx, user, domain.PurposeInvite, uuid.Nil); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "user", EntityID: id, EntityLabel: user.Email, After: userSnapshot(user),
			Note: "Admin Sistem pertama dari INITIAL_ADMIN_EMAIL",
		})
	})
}

// UsersWithRole dipakai modul approval untuk mencari approver per jabatan.
func (s *AdminService) UsersWithRole(ctx context.Context, code string, division *uuid.UUID) ([]domain.User, error) {
	return s.Store.UsersByRoleCode(ctx, code, division)
}

// UserByID dipakai modul lain (approval, notification) untuk nama dan email user.
func (s *AdminService) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return s.Store.GetUser(ctx, id)
}

// SearchActiveUsers dipakai modul lain (approval) untuk memilih user aktif; tanpa cek permission karena
// hanya nama dan email yang dipakai, dan pemanggil sudah memastikan user login.
func (s *AdminService) SearchActiveUsers(ctx context.Context, q string, limit int) ([]domain.User, error) {
	active := true
	size := limit
	res, err := s.Store.ListUsers(ctx, postgres.UserFilter{
		Search: q, IsActive: &active, Sort: paging.Sort{Field: "fullName"},
	}, paging.New(nil, &size))
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// RoleName mengembalikan nama role; ok false jika kode tidak dikenal.
func (s *AdminService) RoleName(ctx context.Context, code string) (string, bool, error) {
	roles, err := s.Store.RolesByCodes(ctx, []string{code})
	if err != nil || len(roles) == 0 {
		return "", false, err
	}
	return roles[0].Name, true, nil
}
