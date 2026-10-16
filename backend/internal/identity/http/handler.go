package http

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"balans/internal/identity/domain"
	"balans/internal/identity/postgres"
	"balans/internal/identity/service"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/identity.yaml ../../../api/openapi.bundle.yaml

// RefreshCookieName adalah nama cookie refresh token (Path=/api/v1/auth).
const RefreshCookieName = "balans_refresh"

const refreshCookiePath = "/api/v1/auth"

// IdentityHandler mengimplementasikan StrictServerInterface hasil generate.
type IdentityHandler struct {
	auth         *service.AuthService
	admin        *service.AdminService
	secureCookie bool
}

var _ StrictServerInterface = (*IdentityHandler)(nil)

// NewHandler membuat handler. secureCookie=false hanya untuk APP_ENV=local tanpa HTTPS.
func NewHandler(auth *service.AuthService, admin *service.AdminService, secureCookie bool) *IdentityHandler {
	return &IdentityHandler{auth: auth, admin: admin, secureCookie: secureCookie}
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *IdentityHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func (h *IdentityHandler) refreshCookie(token string, expires time.Time) *string {
	c := &http.Cookie{ //nolint:gosec // Secure=false hanya untuk APP_ENV=local tanpa HTTPS; HttpOnly & SameSite selalu aktif
		Name: RefreshCookieName, Value: token, Path: refreshCookiePath, HttpOnly: true,
		Secure: h.secureCookie, SameSite: http.SameSiteStrictMode,
	}
	if token == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	s := c.String()
	return &s
}

func cookieValue(c *RefreshCookie) string {
	if c == nil {
		return ""
	}
	return *c
}

// --- Sesi ------------------------------------------------------------------------------------

// Login masuk dan memasang cookie refresh token.
func (h *IdentityHandler) Login(ctx context.Context, req LoginRequestObject) (LoginResponseObject, error) {
	totp := ""
	if req.Body.TotpCode != nil {
		totp = *req.Body.TotpCode
	}
	t, err := h.auth.Login(ctx, string(req.Body.Email), req.Body.Password, totp)
	if err != nil {
		return nil, err
	}
	return Login200JSONResponse{
		Body:    AccessToken{AccessToken: t.AccessToken, ExpiresAt: t.ExpiresAt},
		Headers: Login200ResponseHeaders{SetCookie: h.refreshCookie(t.RefreshToken, t.RefreshExpiresAt)},
	}, nil
}

// RefreshSession merotasi refresh token.
func (h *IdentityHandler) RefreshSession(ctx context.Context, req RefreshSessionRequestObject) (RefreshSessionResponseObject, error) {
	t, err := h.auth.Refresh(ctx, cookieValue(req.Params.BalansRefresh))
	if err != nil {
		return nil, err
	}
	resp := RefreshSession200JSONResponse{Body: AccessToken{AccessToken: t.AccessToken, ExpiresAt: t.ExpiresAt}}
	if t.RefreshToken != "" {
		resp.Headers.SetCookie = h.refreshCookie(t.RefreshToken, t.RefreshExpiresAt)
	}
	return resp, nil
}

// Logout mencabut sesi dan menghapus cookie.
func (h *IdentityHandler) Logout(ctx context.Context, req LogoutRequestObject) (LogoutResponseObject, error) {
	if err := h.auth.Logout(ctx, cookieValue(req.Params.BalansRefresh)); err != nil {
		return nil, err
	}
	return Logout204Response{Headers: Logout204ResponseHeaders{SetCookie: h.refreshCookie("", time.Time{})}}, nil
}

// Reauthenticate menerbitkan access token dengan reauth_at.
func (h *IdentityHandler) Reauthenticate(ctx context.Context, req ReauthenticateRequestObject) (ReauthenticateResponseObject, error) {
	var password, code string
	if req.Body.Password != nil {
		password = *req.Body.Password
	}
	if req.Body.TotpCode != nil {
		code = *req.Body.TotpCode
	}
	t, err := h.auth.Reauth(ctx, password, code)
	if err != nil {
		return nil, err
	}
	return Reauthenticate200JSONResponse{AccessToken: t.AccessToken, ExpiresAt: t.ExpiresAt}, nil
}

// --- Password & MFA --------------------------------------------------------------------------

// RequestPasswordReset selalu 204.
func (h *IdentityHandler) RequestPasswordReset(ctx context.Context, req RequestPasswordResetRequestObject) (RequestPasswordResetResponseObject, error) {
	if err := h.auth.RequestPasswordReset(ctx, string(req.Body.Email)); err != nil {
		return nil, err
	}
	return RequestPasswordReset204Response{}, nil
}

// GetPasswordToken memeriksa tautan.
func (h *IdentityHandler) GetPasswordToken(ctx context.Context, req GetPasswordTokenRequestObject) (GetPasswordTokenResponseObject, error) {
	t, err := h.auth.PasswordTokenInfo(ctx, req.Params.Token)
	if err != nil {
		return nil, err
	}
	return GetPasswordToken200JSONResponse{Email: t.Email, FullName: t.FullName, Purpose: PasswordTokenInfoPurpose(t.Purpose)}, nil
}

// SetupPassword menyimpan password dari tautan.
func (h *IdentityHandler) SetupPassword(ctx context.Context, req SetupPasswordRequestObject) (SetupPasswordResponseObject, error) {
	if err := h.auth.SetupPassword(ctx, req.Body.Token, req.Body.Password); err != nil {
		return nil, err
	}
	return SetupPassword204Response{}, nil
}

// ChangePassword mengganti password sendiri.
func (h *IdentityHandler) ChangePassword(ctx context.Context, req ChangePasswordRequestObject) (ChangePasswordResponseObject, error) {
	if _, err := h.auth.ChangePassword(ctx, req.Body.CurrentPassword, req.Body.NewPassword); err != nil {
		return nil, err
	}
	return ChangePassword204Response{}, nil
}

// SetupMfa membuat secret TOTP.
func (h *IdentityHandler) SetupMfa(ctx context.Context, _ SetupMfaRequestObject) (SetupMfaResponseObject, error) {
	m, err := h.auth.SetupMFA(ctx)
	if err != nil {
		return nil, err
	}
	return SetupMfa200JSONResponse{Secret: m.Secret, OtpauthUrl: m.OtpauthURL, QrCodeDataUrl: m.QRDataURL}, nil
}

// ActivateMfa mengaktifkan MFA.
func (h *IdentityHandler) ActivateMfa(ctx context.Context, req ActivateMfaRequestObject) (ActivateMfaResponseObject, error) {
	if err := h.auth.ActivateMFA(ctx, req.Body.Code); err != nil {
		return nil, err
	}
	return ActivateMfa204Response{}, nil
}

// GetCurrentUser mengembalikan user login.
func (h *IdentityHandler) GetCurrentUser(ctx context.Context, _ GetCurrentUserRequestObject) (GetCurrentUserResponseObject, error) {
	me, err := h.auth.Me(ctx)
	if err != nil {
		return nil, err
	}
	return GetCurrentUser200JSONResponse{
		Id: me.User.ID, Email: me.User.Email, FullName: me.User.FullName, Roles: me.User.RoleCodes(),
		Permissions: me.Permissions, IsMfaEnabled: me.User.IsMFAEnabled, MfaEnrollmentRequired: me.MFAPending,
	}, nil
}

// --- Pengguna --------------------------------------------------------------------------------

func toUser(u domain.User) User {
	roles := make([]UserRoleAssignment, 0, len(u.Roles))
	for _, r := range u.Roles {
		name := r.Name
		roles = append(roles, UserRoleAssignment{Code: r.Code, Name: &name, ScopeDivisionId: r.ScopeDivisionID})
	}
	return User{
		Id: u.ID, Email: u.Email, FullName: u.FullName, Roles: roles, DivisionId: u.DivisionID, EmployeeId: u.EmployeeID,
		IsActive: u.IsActive, IsMfaEnabled: u.IsMFAEnabled, HasPassword: u.HasPassword(), LastLoginAt: u.LastLoginAt,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Version: u.Version,
	}
}

func toAssignments(in []UserRoleAssignment) []domain.RoleAssignment {
	out := make([]domain.RoleAssignment, len(in))
	for i, r := range in {
		out[i] = domain.RoleAssignment{Code: r.Code, ScopeDivisionID: r.ScopeDivisionId}
	}
	return out
}

var userSorts = []string{"fullName", "email", "lastLoginAt", "createdAt"}

// ListUsers mengembalikan daftar pengguna.
func (h *IdentityHandler) ListUsers(ctx context.Context, req ListUsersRequestObject) (ListUsersResponseObject, error) {
	p := req.Params
	f := postgres.UserFilter{
		IsActive: p.IsActive,
		Sort:     paging.ParseSort(p.Sort, userSorts, paging.Sort{Field: "fullName"}),
	}
	if p.Q != nil {
		f.Search = *p.Q
	}
	res, err := h.admin.ListUsers(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]User, len(res.Items))
	for i, u := range res.Items {
		items[i] = toUser(u)
	}
	return ListUsers200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateUser membuat pengguna dan mengirim undangan.
func (h *IdentityHandler) CreateUser(ctx context.Context, req CreateUserRequestObject) (CreateUserResponseObject, error) {
	b := req.Body
	u, err := h.admin.CreateUser(ctx, domain.UserInput{
		Email: string(b.Email), FullName: b.FullName, DivisionID: b.DivisionId, EmployeeID: b.EmployeeId, Roles: toAssignments(b.Roles),
	})
	if err != nil {
		return nil, err
	}
	return CreateUser201JSONResponse(toUser(u)), nil
}

// GetUser mengembalikan detail pengguna.
func (h *IdentityHandler) GetUser(ctx context.Context, req GetUserRequestObject) (GetUserResponseObject, error) {
	u, err := h.admin.GetUser(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetUser200JSONResponse(toUser(u)), nil
}

// UpdateUser mengubah pengguna.
func (h *IdentityHandler) UpdateUser(ctx context.Context, req UpdateUserRequestObject) (UpdateUserResponseObject, error) {
	b := req.Body
	u, err := h.admin.UpdateUser(ctx, req.Id, b.Version, domain.UserInput{
		FullName: b.FullName, DivisionID: b.DivisionId, EmployeeID: b.EmployeeId, Roles: toAssignments(b.Roles),
	})
	if err != nil {
		return nil, err
	}
	return UpdateUser200JSONResponse(toUser(u)), nil
}

// ActivateUser mengaktifkan pengguna.
func (h *IdentityHandler) ActivateUser(ctx context.Context, req ActivateUserRequestObject) (ActivateUserResponseObject, error) {
	u, err := h.admin.SetUserActive(ctx, req.Id, req.Body.Version, true)
	if err != nil {
		return nil, err
	}
	return ActivateUser200JSONResponse(toUser(u)), nil
}

// DeactivateUser menonaktifkan pengguna.
func (h *IdentityHandler) DeactivateUser(ctx context.Context, req DeactivateUserRequestObject) (DeactivateUserResponseObject, error) {
	u, err := h.admin.SetUserActive(ctx, req.Id, req.Body.Version, false)
	if err != nil {
		return nil, err
	}
	return DeactivateUser200JSONResponse(toUser(u)), nil
}

// ResendUserInvitation mengirim ulang undangan.
func (h *IdentityHandler) ResendUserInvitation(ctx context.Context, req ResendUserInvitationRequestObject) (ResendUserInvitationResponseObject, error) {
	if err := h.admin.ResendInvitation(ctx, req.Id); err != nil {
		return nil, err
	}
	return ResendUserInvitation204Response{}, nil
}

// ResetUserMfa mereset MFA.
func (h *IdentityHandler) ResetUserMfa(ctx context.Context, req ResetUserMfaRequestObject) (ResetUserMfaResponseObject, error) {
	if err := h.admin.ResetMFA(ctx, req.Id); err != nil {
		return nil, err
	}
	return ResetUserMfa204Response{}, nil
}

// --- Role & permission -----------------------------------------------------------------------

func toRole(r domain.Role) Role {
	return Role{
		Id: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, RequiresMfa: r.RequiresMFA,
		HasDivisionScope: r.HasDivisionScope, Permissions: r.Permissions, Version: r.Version,
	}
}

// ListRoles mengembalikan role.
func (h *IdentityHandler) ListRoles(ctx context.Context, _ ListRolesRequestObject) (ListRolesResponseObject, error) {
	roles, err := h.admin.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make(ListRoles200JSONResponse, len(roles))
	for i, r := range roles {
		out[i] = toRole(r)
	}
	return out, nil
}

// SetRolePermissions mengganti permission role.
func (h *IdentityHandler) SetRolePermissions(ctx context.Context, req SetRolePermissionsRequestObject) (SetRolePermissionsResponseObject, error) {
	r, err := h.admin.SetRolePermissions(ctx, req.Id, req.Body.Version, req.Body.Permissions)
	if err != nil {
		return nil, err
	}
	return SetRolePermissions200JSONResponse(toRole(r)), nil
}

// ListPermissions mengembalikan katalog permission.
func (h *IdentityHandler) ListPermissions(ctx context.Context, _ ListPermissionsRequestObject) (ListPermissionsResponseObject, error) {
	perms, err := h.admin.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(ListPermissions200JSONResponse, len(perms))
	for i, p := range perms {
		out[i] = Permission{Code: p.Code, Module: p.Module, Description: p.Description, RequiresReauth: p.RequiresReauth}
	}
	return out, nil
}
