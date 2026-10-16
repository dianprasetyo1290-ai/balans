package domain_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/identity/domain"
	"balans/internal/platform/apperr"
)

func find(t *testing.T, code string) domain.Permission {
	t.Helper()
	for _, p := range domain.Catalog() {
		if p.Code == code {
			return p
		}
	}
	t.Fatalf("permission %s tidak ada", code)
	return domain.Permission{}
}

func TestCatalogFollowsMatrix(t *testing.T) {
	format := regexp.MustCompile(`^[a-z_]+\.[a-z_]+\.[a-z_]+$`)
	codes := map[string]bool{}
	for _, p := range domain.Catalog() {
		assert.Regexp(t, format, p.Code)
		assert.False(t, codes[p.Code], "kode ganda %s", p.Code)
		codes[p.Code] = true
		assert.NotEmpty(t, p.Description)
		assert.NotEmpty(t, p.DefaultRoles, p.Code)
	}

	assert.Equal(t, []string{domain.RoleSystemAdmin}, find(t, "catalog.product.write").DefaultRoles)
	assert.Contains(t, find(t, "catalog.product.import").DefaultRoles, domain.RoleSystemAdmin)
	assert.ElementsMatch(t, []string{domain.RoleHeadFinance, domain.RoleDirector, domain.RolePresidentDirector},
		find(t, "procurement.purchase_order.approve").DefaultRoles)
	assert.ElementsMatch(t, []string{domain.RoleFulfillmentLeader, domain.RoleMerchandiser, domain.RoleHeadLogistics},
		find(t, "warehouse.outbound_qc.post").DefaultRoles, "QC Outbound bukan oleh Staf Gudang")
	assert.Contains(t, find(t, "sales.sales_return.verify").DefaultRoles, domain.RoleLogisticsLeader)
	assert.Contains(t, find(t, "dataexchange.transaction.import").DefaultRoles, domain.RoleLogisticsLeader)
}

func TestCatalogReauthFlags(t *testing.T) {
	for _, code := range []string{"accounting.fiscal_period.reopen", "accounting.payment.post", "identity.user_role.write", "setting.setting.write"} {
		assert.True(t, find(t, code).RequiresReauth, code)
	}
	assert.False(t, find(t, "catalog.product.write").RequiresReauth)
	assert.Equal(t, []string{domain.RoleHeadFinance}, find(t, "accounting.fiscal_period.reopen").DefaultRoles)
}

func TestCatalogIsSorted(t *testing.T) {
	c := domain.Catalog()
	assert.True(t, slices.IsSortedFunc(c, func(a, b domain.Permission) int {
		if a.Code < b.Code {
			return -1
		}
		return 1
	}))
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		password string
		ok       bool
	}{
		{"Kopi-Pagi-2026", true},
		{"pendek", false},
		{"1234567890", false},
		{"aaaaaaaaaaaa", false},
		{"sitianisa2026!", false},
	}
	for _, tt := range tests {
		t.Run(tt.password, func(t *testing.T) {
			err := domain.ValidatePassword(tt.password, "sitianisa@balans.id")
			if tt.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	h, err := domain.HashPassword("Kopi-Pagi-2026")
	require.NoError(t, err)
	assert.Contains(t, h, "$argon2id$")

	ok, err := domain.VerifyPassword("Kopi-Pagi-2026", h)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = domain.VerifyPassword("salah-password", h)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = domain.VerifyPassword("x", "$bcrypt$xx")
	assert.Error(t, err)

	other, err := domain.HashPassword("Kopi-Pagi-2026")
	require.NoError(t, err)
	assert.NotEqual(t, h, other, "salt acak")
	domain.BurnPasswordCheck("apa saja")
}

func TestNormalizeEmail(t *testing.T) {
	assert.Equal(t, "admin@balans.id", domain.NormalizeEmail("  Admin@Balans.ID "))
}
