package app

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"balans/internal/hris"
	"balans/internal/identity"
	"balans/internal/party"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// partyUsers menyesuaikan API publik identity untuk pemilihan Sales dan Merchandiser mitra.
type partyUsers struct {
	identity *identity.Module
}

func toUserRef(u identity.User) party.UserRef {
	ref := party.UserRef{ID: u.ID, Name: u.FullName, Email: u.Email}
	for _, r := range u.Roles {
		ref.Roles = append(ref.Roles, r.Code)
	}
	return ref
}

// UsersByID mengembalikan pengguna aktif; pengguna nonaktif atau tidak ada dilewati.
func (p partyUsers) UsersByID(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]party.UserRef, error) {
	out := make(map[uuid.UUID]party.UserRef, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok {
			continue
		}
		u, err := p.identity.UserByID(ctx, id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if u.IsActive {
			out[id] = toUserRef(u)
		}
	}
	return out, nil
}

// SearchUsers mencari pengguna aktif berdasarkan nama/email, dibatasi role bila diisi.
func (p partyUsers) SearchUsers(ctx context.Context, q, role string, limit int) ([]party.UserRef, error) {
	if role == "" {
		users, err := p.identity.SearchActiveUsers(ctx, q, limit)
		if err != nil {
			return nil, err
		}
		out := make([]party.UserRef, len(users))
		for i, u := range users {
			out[i] = toUserRef(u)
		}
		return out, nil
	}
	users, err := p.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	out := make([]party.UserRef, 0, limit)
	for _, u := range users {
		if needle != "" && !strings.Contains(strings.ToLower(u.FullName), needle) && !strings.Contains(strings.ToLower(u.Email), needle) {
			continue
		}
		out = append(out, toUserRef(u))
	}
	slices.SortFunc(out, func(a, b party.UserRef) int { return strings.Compare(a.Name, b.Name) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// partyEmployees menyesuaikan API publik hris (tier lebih tinggi) untuk modul party.
type partyEmployees struct {
	hris *hris.Module
}

func (p partyEmployees) Employees(ctx context.Context, ids []uuid.UUID, on clock.BusinessDate) (map[uuid.UUID]party.EmployeeRef, error) {
	list, err := p.hris.EmployeesByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]party.EmployeeRef, len(list))
	for _, e := range list {
		out[e.ID] = party.EmployeeRef{ID: e.ID, Number: e.Number, Name: e.FullName, IsActive: e.ActiveOn(on)}
	}
	return out, nil
}

func (p partyEmployees) IsActiveEmployee(ctx context.Context, id uuid.UUID, on clock.BusinessDate) (bool, error) {
	return p.hris.IsActiveEmployee(ctx, id, on)
}

// EmployeeIDByNumber dipakai impor pelanggan untuk menautkan pelanggan ke karyawan dari nomor karyawan.
func (p partyEmployees) EmployeeIDByNumber(ctx context.Context, number string) (uuid.UUID, bool, error) {
	return p.hris.EmployeeIDByNumber(ctx, number)
}
