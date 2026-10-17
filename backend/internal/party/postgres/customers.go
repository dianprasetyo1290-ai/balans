package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"balans/internal/party/domain"
	"balans/internal/party/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
)

// CustomerFilter adalah filter daftar pelanggan.
type CustomerFilter struct {
	Search      string
	PhoneSearch string
	Phone       string
	Type        domain.CustomerType
	Sort        string
}

func toCustomer(r sqlcgen.Customer) domain.Customer {
	return domain.Customer{
		ID: r.ID, Code: r.Code, FullName: r.FullName, Phone: r.PhoneNormalized, Email: r.Email,
		Type: domain.CustomerType(r.CustomerType), EmployeeID: uuidPtr(r.EmployeeID),
		MarketingConsentAt: r.MarketingConsentAt, MarketingConsentVersion: r.MarketingConsentVersion, Notes: r.Notes,
		Addresses: []domain.Address{}, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toAddress(r sqlcgen.CustomerAddress) domain.Address {
	return domain.Address{
		ID: r.ID, Label: r.Label, RecipientName: r.RecipientName, Phone: r.Phone, AddressLine: r.AddressLine,
		District: r.District, City: r.City, Province: r.Province, PostalCode: r.PostalCode,
		Latitude: decimalPtr(r.Latitude), Longitude: decimalPtr(r.Longitude), IsDefault: r.IsDefault,
	}
}

func customerError(err error, what string) error {
	switch violation(err) {
	case "uq_customers_phone_normalized":
		return apperr.Validation(map[string]string{"phone": "Nomor HP sudah dipakai pelanggan lain"})
	case "uq_customers_employee_id":
		return apperr.Validation(map[string]string{"employeeId": "Karyawan sudah tertaut ke pelanggan lain"})
	}
	return fmt.Errorf("%s: %w", what, err)
}

func (s *Store) withAddresses(ctx context.Context, list []domain.Customer) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	idx := make(map[uuid.UUID]int, len(list))
	for i, c := range list {
		ids[i], idx[c.ID] = c.ID, i
	}
	rows, err := s.q(ctx).ListCustomerAddresses(ctx, ids)
	if err != nil {
		return fmt.Errorf("list customer addresses: %w", err)
	}
	for _, r := range rows {
		i := idx[r.CustomerID]
		list[i].Addresses = append(list[i].Addresses, toAddress(r))
	}
	return nil
}

func addressParams(a domain.Address) sqlcgen.UpdateCustomerAddressParams {
	return sqlcgen.UpdateCustomerAddressParams{
		Label: a.Label, RecipientName: a.RecipientName, Phone: a.Phone, AddressLine: a.AddressLine,
		District: a.District, City: a.City, Province: a.Province, PostalCode: a.PostalCode,
		Latitude: nullDecimal(a.Latitude), Longitude: nullDecimal(a.Longitude), IsDefault: a.IsDefault,
	}
}

// saveAddresses menyelaraskan alamat: yang dihapus di-soft delete, yang ada diubah, yang baru ditambah.
func (s *Store) saveAddresses(ctx context.Context, customerID uuid.UUID, before, after []domain.Address, actorID uuid.UUID) ([]domain.Address, error) {
	q := s.q(ctx)
	if removed := domain.RemovedAddresses(before, after); len(removed) > 0 {
		if err := q.SoftDeleteCustomerAddresses(ctx, sqlcgen.SoftDeleteCustomerAddressesParams{
			CustomerID: customerID, Ids: removed, ActorID: actorID,
		}); err != nil {
			return nil, fmt.Errorf("delete customer addresses: %w", err)
		}
	}
	// Alamat utama dilepas dulu agar indeks unik alamat utama tidak bentrok saat pindah.
	if err := q.ClearCustomerDefaultAddress(ctx, customerID); err != nil {
		return nil, fmt.Errorf("clear default address: %w", err)
	}
	out := make([]domain.Address, len(after))
	for i, a := range after {
		p := addressParams(a)
		p.CustomerID, p.ActorID = customerID, actorID
		if a.ID == uuid.Nil {
			id, err := q.InsertCustomerAddress(ctx, sqlcgen.InsertCustomerAddressParams{
				CustomerID: customerID, Label: p.Label, RecipientName: p.RecipientName, Phone: p.Phone,
				AddressLine: p.AddressLine, District: p.District, City: p.City, Province: p.Province,
				PostalCode: p.PostalCode, Latitude: p.Latitude, Longitude: p.Longitude, IsDefault: p.IsDefault,
				ActorID: actorID,
			})
			if err != nil {
				return nil, fmt.Errorf("insert customer address: %w", err)
			}
			a.ID = id
		} else {
			p.ID = a.ID
			if _, err := q.UpdateCustomerAddress(ctx, p); err != nil {
				return nil, fmt.Errorf("update customer address: %w", err)
			}
		}
		out[i] = a
	}
	return out, nil
}

// InsertCustomer menyimpan pelanggan baru beserta alamatnya.
func (s *Store) InsertCustomer(ctx context.Context, c domain.Customer, rawPhone string, actorID uuid.UUID) (domain.Customer, error) {
	r, err := s.q(ctx).InsertCustomer(ctx, sqlcgen.InsertCustomerParams{
		FullName: c.FullName, Phone: rawPhone, PhoneNormalized: c.Phone, Email: c.Email,
		CustomerType: string(c.Type), EmployeeID: nullUUID(c.EmployeeID), MarketingConsentAt: c.MarketingConsentAt,
		MarketingConsentVersion: c.MarketingConsentVersion, Notes: c.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.Customer{}, customerError(err, "insert customer")
	}
	out := toCustomer(r)
	if out.Addresses, err = s.saveAddresses(ctx, out.ID, nil, c.Addresses, actorID); err != nil {
		return domain.Customer{}, err
	}
	return out, nil
}

// GetCustomer membaca pelanggan beserta alamat; forUpdate mengunci baris pelanggan.
func (s *Store) GetCustomer(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Customer, error) {
	var (
		r   sqlcgen.Customer
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetCustomerForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetCustomer(ctx, id)
	}
	if err != nil {
		return domain.Customer{}, notFound(err, "get customer")
	}
	list := []domain.Customer{toCustomer(r)}
	if err := s.withAddresses(ctx, list); err != nil {
		return domain.Customer{}, err
	}
	return list[0], nil
}

// CustomerByPhone mencari pelanggan aktif dengan nomor ternormalisasi.
func (s *Store) CustomerByPhone(ctx context.Context, phone string) (domain.Customer, bool, error) {
	r, err := s.q(ctx).GetCustomerByPhone(ctx, phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Customer{}, false, nil
	}
	if err != nil {
		return domain.Customer{}, false, fmt.Errorf("customer by phone: %w", err)
	}
	return toCustomer(r), true, nil
}

// ListCustomers mengembalikan satu halaman pelanggan beserta alamatnya.
func (s *Store) ListCustomers(ctx context.Context, f CustomerFilter, page paging.Page) (paging.Result[domain.Customer], error) {
	q := s.q(ctx)
	search, phoneSearch, phone, typ := text(f.Search), text(f.PhoneSearch), text(f.Phone), text(string(f.Type))
	rows, err := q.ListCustomers(ctx, sqlcgen.ListCustomersParams{
		Search: search, PhoneSearch: phoneSearch, Phone: phone, CustomerType: typ, Sort: f.Sort,
		RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Customer]{}, fmt.Errorf("list customers: %w", err)
	}
	total, err := q.CountCustomers(ctx, sqlcgen.CountCustomersParams{
		Search: search, PhoneSearch: phoneSearch, Phone: phone, CustomerType: typ,
	})
	if err != nil {
		return paging.Result[domain.Customer]{}, fmt.Errorf("count customers: %w", err)
	}
	items := make([]domain.Customer, len(rows))
	for i, r := range rows {
		items[i] = toCustomer(r)
	}
	if err := s.withAddresses(ctx, items); err != nil {
		return paging.Result[domain.Customer]{}, err
	}
	return paging.Result[domain.Customer]{Items: items, Page: page, Total: total}, nil
}

// UpdateCustomer menyimpan perubahan pelanggan dan alamat dengan optimistic locking.
func (s *Store) UpdateCustomer(ctx context.Context, before, after domain.Customer, rawPhone string, actorID uuid.UUID) (domain.Customer, error) {
	n, err := s.q(ctx).UpdateCustomer(ctx, sqlcgen.UpdateCustomerParams{
		ID: after.ID, Version: v32(before.Version), FullName: after.FullName, Phone: rawPhone,
		PhoneNormalized: after.Phone, Email: after.Email, CustomerType: string(after.Type),
		EmployeeID: nullUUID(after.EmployeeID), MarketingConsentAt: after.MarketingConsentAt,
		MarketingConsentVersion: after.MarketingConsentVersion, Notes: after.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.Customer{}, customerError(err, "update customer")
	}
	if err := versionConflict(n, nil, "update customer"); err != nil {
		return domain.Customer{}, err
	}
	if after.Addresses, err = s.saveAddresses(ctx, after.ID, before.Addresses, after.Addresses, actorID); err != nil {
		return domain.Customer{}, err
	}
	after.Version = before.Version + 1
	return after, nil
}

// DeleteCustomer melakukan soft delete.
func (s *Store) DeleteCustomer(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteCustomer(ctx, sqlcgen.SoftDeleteCustomerParams{ID: id, Version: v32(version), ActorID: actorID})
	return versionConflict(n, err, "delete customer")
}
