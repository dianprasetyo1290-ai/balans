package app

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/identity"
	"balans/internal/platform/apperr"
)

// approvalDirectory menyesuaikan API publik identity dengan kebutuhan engine approval, sehingga approval
// tidak bergantung pada bentuk data identity.
type approvalDirectory struct {
	identity *identity.Module
}

func toPrincipal(u identity.User) approval.Principal {
	p := approval.Principal{UserID: u.ID, Name: u.FullName, Email: u.Email}
	for _, r := range u.Roles {
		p.Roles = append(p.Roles, approval.RoleGrant{Code: r.Code, Division: r.ScopeDivisionID})
	}
	return p
}

// User mengembalikan user aktif; user nonaktif dianggap tidak ada.
func (d approvalDirectory) User(ctx context.Context, id uuid.UUID) (approval.Principal, error) {
	u, err := d.identity.UserByID(ctx, id)
	if err != nil {
		return approval.Principal{}, err
	}
	if !u.IsActive {
		return approval.Principal{}, apperr.New(apperr.CodeNotFound)
	}
	return toPrincipal(u), nil
}

func (d approvalDirectory) UsersWithRole(ctx context.Context, role string, division *uuid.UUID) ([]approval.Principal, error) {
	users, err := d.identity.UsersWithRole(ctx, role, division)
	if err != nil {
		return nil, err
	}
	out := make([]approval.Principal, len(users))
	for i, u := range users {
		out[i] = toPrincipal(u)
	}
	return out, nil
}

func (d approvalDirectory) RoleName(ctx context.Context, code string) (string, bool, error) {
	return d.identity.RoleName(ctx, code)
}

func (d approvalDirectory) SearchUsers(ctx context.Context, q string, limit int) ([]approval.Principal, error) {
	users, err := d.identity.SearchActiveUsers(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]approval.Principal, len(users))
	for i, u := range users {
		out[i] = toPrincipal(u)
	}
	return out, nil
}
