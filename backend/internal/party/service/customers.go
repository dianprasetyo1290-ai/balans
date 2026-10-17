package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/party/domain"
	"balans/internal/party/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

func customerSnapshot(c domain.Customer) map[string]any {
	addresses := make([]map[string]any, len(c.Addresses))
	for i, a := range c.Addresses {
		addresses[i] = map[string]any{
			"label": a.Label, "recipientName": a.RecipientName, "phone": a.Phone, "addressLine": a.AddressLine,
			"district": a.District, "city": a.City, "province": a.Province, "postalCode": a.PostalCode,
			"isDefault": a.IsDefault,
		}
	}
	return map[string]any{
		"fullName": c.FullName, "phone": c.Phone, "email": c.Email, "customerType": c.Type, "employeeId": c.EmployeeID,
		"marketingConsentAt": c.MarketingConsentAt, "marketingConsentVersion": c.MarketingConsentVersion,
		"notes": c.Notes, "addresses": addresses,
	}
}

// CustomerQuery adalah filter daftar pelanggan dari API.
type CustomerQuery struct {
	Search string
	Phone  string
	Type   domain.CustomerType
	Sort   paging.Sort
}

// ListCustomers mencari pelanggan berdasarkan nama, kode, atau nomor HP.
func (s *Service) ListCustomers(ctx context.Context, q CustomerQuery, page paging.Page) (paging.Result[domain.Customer], error) {
	if _, err := actor.Require(ctx, PermCustomerRead); err != nil {
		return paging.Result[domain.Customer]{}, err
	}
	f := postgres.CustomerFilter{Search: strings.TrimSpace(q.Search), Type: q.Type, Sort: q.Sort.Key()}
	if d, ok := domain.PhoneSearchDigits(f.Search); ok {
		f.PhoneSearch = d
	}
	if q.Phone != "" {
		phone, ok := domain.NormalizePhone(q.Phone)
		if !ok {
			return paging.Result[domain.Customer]{Page: page, Items: []domain.Customer{}}, nil
		}
		f.Phone = phone
	}
	return s.Store.ListCustomers(ctx, f, page)
}

// GetCustomer mengembalikan detail pelanggan.
func (s *Service) GetCustomer(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	if _, err := actor.Require(ctx, PermCustomerRead); err != nil {
		return domain.Customer{}, err
	}
	return s.Store.GetCustomer(ctx, id, false)
}

// checkCustomer memastikan nomor HP belum dipakai pelanggan lain dan karyawan yang ditautkan aktif.
func (s *Service) checkCustomer(ctx context.Context, c domain.Customer, previousEmployee *uuid.UUID) error {
	if other, ok, err := s.Store.CustomerByPhone(ctx, c.Phone); err != nil {
		return err
	} else if ok && other.ID != c.ID {
		return apperr.Validation(map[string]string{
			"phone": "Nomor HP sudah dipakai pelanggan " + other.Code + " (" + other.FullName + ")",
		})
	}
	if c.EmployeeID == nil || (previousEmployee != nil && *previousEmployee == *c.EmployeeID) {
		return nil
	}
	if s.Employees == nil {
		return apperr.Validation(map[string]string{"employeeId": "Modul karyawan belum aktif"})
	}
	active, err := s.Employees.IsActiveEmployee(ctx, *c.EmployeeID, s.today())
	if err != nil {
		return err
	}
	if !active {
		return apperr.Validation(map[string]string{"employeeId": "Karyawan tidak ditemukan atau sudah resign"})
	}
	return nil
}

// CreateCustomer membuat pelanggan. Nomor HP dinormalisasi dan harus unik (MD-06).
func (s *Service) CreateCustomer(ctx context.Context, in domain.CustomerInput) (domain.Customer, error) {
	a, err := actor.Require(ctx, PermCustomerWrite)
	if err != nil {
		return domain.Customer{}, err
	}
	c, err := domain.NewCustomer(in, s.Clock.Now())
	if err != nil {
		return domain.Customer{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkCustomer(ctx, c, nil); err != nil {
			return err
		}
		saved, err := s.Store.InsertCustomer(ctx, c, strings.TrimSpace(in.Phone), a.UserID)
		if err != nil {
			return err
		}
		c = saved
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityCustomer, EntityID: c.ID, EntityLabel: c.Code + " " + c.FullName,
			After: customerSnapshot(c),
		})
	})
	return c, err
}

// UpdateCustomer mengubah pelanggan dan alamatnya.
func (s *Service) UpdateCustomer(ctx context.Context, id uuid.UUID, version int, in domain.CustomerInput) (domain.Customer, error) {
	a, err := actor.Require(ctx, PermCustomerWrite)
	if err != nil {
		return domain.Customer{}, err
	}
	var next domain.Customer
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetCustomer(ctx, id, true)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		next = cur
		next.Addresses = append([]domain.Address(nil), cur.Addresses...)
		if err := next.Update(in, s.Clock.Now()); err != nil {
			return err
		}
		if err := s.checkCustomer(ctx, next, cur.EmployeeID); err != nil {
			return err
		}
		if next, err = s.Store.UpdateCustomer(ctx, cur, next, strings.TrimSpace(in.Phone), a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityCustomer, EntityID: next.ID,
			EntityLabel: next.Code + " " + next.FullName, Before: customerSnapshot(cur), After: customerSnapshot(next),
		})
	})
	return next, err
}

// DeleteCustomer menghapus pelanggan (soft delete). Nomor HP-nya boleh dipakai pelanggan baru.
func (s *Service) DeleteCustomer(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermCustomerWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetCustomer(ctx, id, true)
		if err != nil {
			return err
		}
		if err := s.Store.DeleteCustomer(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityCustomer, EntityID: id, EntityLabel: cur.Code + " " + cur.FullName,
			Before: customerSnapshot(cur),
		})
	})
}

// CustomerByID dipakai modul lain (sales, pricing) tanpa cek permission; pemanggil sudah memeriksa hak aksesnya.
func (s *Service) CustomerByID(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	return s.Store.GetCustomer(ctx, id, false)
}

// CustomerByPhone mencari pelanggan aktif dari nomor HP dalam format apa pun; ok=false jika tidak ada.
func (s *Service) CustomerByPhone(ctx context.Context, phone string) (domain.Customer, bool, error) {
	n, ok := domain.NormalizePhone(phone)
	if !ok {
		return domain.Customer{}, false, nil
	}
	c, found, err := s.Store.CustomerByPhone(ctx, n)
	if err != nil || !found {
		return domain.Customer{}, false, err
	}
	full, err := s.Store.GetCustomer(ctx, c.ID, false)
	return full, err == nil, err
}

// EmployeePriceEligible memeriksa hak harga karyawan pelanggan pada tanggal transaksi: pelanggan tertaut ke
// karyawan yang masih aktif. Karyawan yang resign otomatis kehilangan hak ini (HR-03).
func (s *Service) EmployeePriceEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error) {
	c, err := s.Store.GetCustomer(ctx, customerID, false)
	if err != nil {
		return false, err
	}
	if c.EmployeeID == nil || s.Employees == nil {
		return false, nil
	}
	return s.Employees.IsActiveEmployee(ctx, *c.EmployeeID, on)
}

// EmployeeRefs mengembalikan karyawan yang tertaut ke pelanggan beserta status aktif hari ini.
func (s *Service) EmployeeRefs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]EmployeeRef, error) {
	if s.Employees == nil || len(ids) == 0 {
		return map[uuid.UUID]EmployeeRef{}, nil
	}
	return s.Employees.Employees(ctx, ids, s.today())
}
