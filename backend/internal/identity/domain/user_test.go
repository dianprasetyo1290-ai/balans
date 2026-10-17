package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/identity/domain"
	"balans/internal/platform/apperr"
)

func TestUserRoleHelpers(t *testing.T) {
	scope, own := uuid.New(), uuid.New()
	u := domain.User{
		DivisionID: &own,
		Roles: []domain.UserRole{
			{Code: "purchasing"},
			{Code: "division_head", HasDivisionScope: true, ScopeDivisionID: &scope},
			{Code: "head_finance", RequiresMFA: true},
		},
	}
	assert.False(t, u.HasPassword(), "undangan belum diterima")
	u.PasswordHash = "hash"
	assert.True(t, u.HasPassword())
	assert.Equal(t, []string{"purchasing", "division_head", "head_finance"}, u.RoleCodes())
	assert.True(t, u.RequiresMFA())
	assert.True(t, u.MFAEnrollmentPending())
	u.IsMFAEnabled = true
	assert.False(t, u.MFAEnrollmentPending())
	assert.Equal(t, []uuid.UUID{scope}, u.DivisionScope())

	u.Roles[1].ScopeDivisionID = nil
	assert.Equal(t, []uuid.UUID{own}, u.DivisionScope(), "tanpa scope eksplisit memakai divisi user")
	u.DivisionID = nil
	assert.Empty(t, u.DivisionScope())
	assert.False(t, domain.User{}.RequiresMFA())
}

func TestUserInputValidate(t *testing.T) {
	ok := domain.UserInput{Email: " Rina@Balans.Local ", FullName: " Rina ", Roles: []domain.RoleAssignment{{Code: "purchasing"}}}
	require.NoError(t, ok.Validate(true))
	assert.Equal(t, "rina@balans.local", ok.Email)
	assert.Equal(t, "Rina", ok.FullName)

	tests := []struct {
		name  string
		in    domain.UserInput
		email bool
		field string
	}{
		{"email tidak valid", domain.UserInput{Email: "rina", FullName: "R", Roles: []domain.RoleAssignment{{Code: "a"}}}, true, "email"},
		{"nama kosong", domain.UserInput{FullName: " ", Roles: []domain.RoleAssignment{{Code: "a"}}}, false, "fullName"},
		{"nama panjang", domain.UserInput{FullName: strings.Repeat("a", 121), Roles: []domain.RoleAssignment{{Code: "a"}}}, false, "fullName"},
		{"tanpa role", domain.UserInput{FullName: "R"}, false, "roles"},
		{"role ganda", domain.UserInput{FullName: "R", Roles: []domain.RoleAssignment{{Code: "a"}, {Code: "a"}}}, false, "roles"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate(tt.email)
			require.Error(t, err)
			assert.Contains(t, apperr.As(err).Fields, tt.field)
		})
	}
}

func TestPasswordTokenUsable(t *testing.T) {
	now := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	tok := domain.PasswordToken{Expires: now.Add(time.Hour), IsActive: true}
	assert.True(t, tok.Usable(now))
	assert.False(t, tok.Usable(now.Add(time.Hour)), "kedaluwarsa")
	used := now
	tok.UsedAt = &used
	assert.False(t, tok.Usable(now), "sudah dipakai")
	assert.False(t, domain.PasswordToken{Expires: now.Add(time.Hour)}.Usable(now), "user nonaktif")
}

func TestPresidentDirectorCanOpenPurchaseOrders(t *testing.T) {
	for _, code := range []string{"procurement.purchase_order.read", "procurement.purchase_order.approve"} {
		p := find(t, code)
		assert.Contains(t, p.DefaultRoles, domain.RolePresidentDirector, "Direktur Utama menyetujui PO > Rp50 jt dan harus bisa membuka detailnya")
	}
}
