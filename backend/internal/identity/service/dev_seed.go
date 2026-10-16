package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/identity/domain"
	"balans/internal/platform/actor"
)

// DevUser adalah user hasil seed pengembangan.
type DevUser struct {
	Email    string
	FullName string
	Role     string
}

// devUsers: satu akun per role awal untuk pengembangan dan demo lokal.
var devUsers = []DevUser{
	{"admin@balans.local", "Admin Sistem", domain.RoleSystemAdmin},
	{"purchasing@balans.local", "Purchasing", domain.RolePurchasing},
	{"staf.gudang@balans.local", "Staf Gudang", domain.RoleWarehouseStaff},
	{"logistik.leader@balans.local", "Logistik Leader", domain.RoleLogisticsLeader},
	{"fulfillment.leader@balans.local", "Fulfillment Leader", domain.RoleFulfillmentLeader},
	{"merchandiser@balans.local", "Merchandiser", domain.RoleMerchandiser},
	{"head.logistik@balans.local", "Head Logistik", domain.RoleHeadLogistics},
	{"finance@balans.local", "Finance & Accounting", domain.RoleFinance},
	{"head.finance@balans.local", "Head Finance & Accounting", domain.RoleHeadFinance},
	{"sales.mt@balans.local", "Sales MT", domain.RoleSalesMT},
	{"sales.ecommerce@balans.local", "Sales E-commerce", domain.RoleSalesEcommerce},
	{"cs@balans.local", "Customer Service", domain.RoleCustomerService},
	{"head.sales@balans.local", "Head Sales", domain.RoleHeadSales},
	{"marketing@balans.local", "Marketing", domain.RoleMarketing},
	{"hrga@balans.local", "HRGA", domain.RoleHRGA},
	{"head.divisi@balans.local", "Head Divisi", domain.RoleDivisionHead},
	{"direksi@balans.local", "Direksi", domain.RoleDirector},
	{"dirut@balans.local", "Direktur Utama", domain.RolePresidentDirector},
}

// SeedDevUsers membuat akun pengembangan yang belum ada; password semua akun diatur ulang ke password seed.
func (s *AdminService) SeedDevUsers(ctx context.Context, password string) ([]DevUser, error) {
	if err := s.SyncCatalog(ctx); err != nil {
		return nil, err
	}
	hash, err := domain.HashPassword(password)
	if err != nil {
		return nil, err
	}
	ctx = actor.System(ctx)
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, d := range devUsers {
			user, found, err := s.Store.FindUserByEmail(ctx, d.Email)
			if err != nil {
				return err
			}
			if !found {
				in := domain.UserInput{Email: d.Email, FullName: d.FullName, Roles: []domain.RoleAssignment{{Code: d.Role}}}
				roles, err := s.resolveRoles(ctx, in.Roles)
				if err != nil {
					return err
				}
				if user.ID, err = s.Store.CreateUser(ctx, in, roles, uuid.Nil); err != nil {
					return err
				}
			}
			if err := s.Store.SetPassword(ctx, user.ID, hash, s.Clock.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	return devUsers, err
}
