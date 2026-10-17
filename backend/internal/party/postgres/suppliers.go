package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/party/domain"
	"balans/internal/party/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
)

// partyTypeSupplier adalah party_type rekening pemasok.
const partyTypeSupplier = "supplier"

// SupplierFilter adalah filter daftar pemasok.
type SupplierFilter struct {
	Search string
	Type   domain.SupplierType
	Sort   string
}

func (s *Store) toSupplier(r sqlcgen.Supplier) (domain.Supplier, error) {
	npwp, err := s.decrypt(r.NpwpEnc)
	if err != nil {
		return domain.Supplier{}, fmt.Errorf("supplier %s npwp: %w", r.ID, err)
	}
	return domain.Supplier{
		ID: r.ID, Code: r.Code, Name: r.Name, Type: domain.SupplierType(r.SupplierType),
		IsBusinessEntity: r.IsBusinessEntity, NPWP: npwp, DefaultWithholding: domain.WithholdingType(r.DefaultWithholding),
		PaymentTermDays: int(r.PaymentTermDays), Address: r.Address, Phone: r.Phone, Email: r.Email, PICName: r.PicName,
		Notes: r.Notes, BankAccounts: []domain.BankAccount{}, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}, nil
}

func (s *Store) withBankAccounts(ctx context.Context, list []domain.Supplier) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	idx := make(map[uuid.UUID]int, len(list))
	for i, sup := range list {
		ids[i], idx[sup.ID] = sup.ID, i
	}
	rows, err := s.q(ctx).ListBankAccounts(ctx, sqlcgen.ListBankAccountsParams{PartyType: partyTypeSupplier, PartyIds: ids})
	if err != nil {
		return fmt.Errorf("list bank accounts: %w", err)
	}
	for _, r := range rows {
		number, err := s.decrypt(r.AccountNumberEnc)
		if err != nil {
			return fmt.Errorf("bank account %s: %w", r.ID, err)
		}
		i := idx[r.PartyID]
		list[i].BankAccounts = append(list[i].BankAccounts, domain.BankAccount{
			ID: r.ID, BankName: r.BankName, AccountNumber: number, AccountName: r.AccountName, IsDefault: r.IsDefault,
		})
	}
	return nil
}

func (s *Store) saveBankAccounts(ctx context.Context, supplierID uuid.UUID, before, after []domain.BankAccount, actorID uuid.UUID) ([]domain.BankAccount, error) {
	q := s.q(ctx)
	if removed := domain.RemovedBankAccounts(before, after); len(removed) > 0 {
		if err := q.SoftDeleteBankAccounts(ctx, sqlcgen.SoftDeleteBankAccountsParams{
			PartyType: partyTypeSupplier, PartyID: supplierID, Ids: removed, ActorID: actorID,
		}); err != nil {
			return nil, fmt.Errorf("delete bank accounts: %w", err)
		}
	}
	if err := q.ClearDefaultBankAccount(ctx, sqlcgen.ClearDefaultBankAccountParams{
		PartyType: partyTypeSupplier, PartyID: supplierID,
	}); err != nil {
		return nil, fmt.Errorf("clear default bank account: %w", err)
	}
	out := make([]domain.BankAccount, len(after))
	for i, b := range after {
		enc, err := s.encrypt(b.AccountNumber)
		if err != nil {
			return nil, err
		}
		if b.ID == uuid.Nil {
			if b.ID, err = q.InsertBankAccount(ctx, sqlcgen.InsertBankAccountParams{
				PartyType: partyTypeSupplier, PartyID: supplierID, BankName: b.BankName, AccountNumberEnc: enc,
				AccountName: b.AccountName, IsDefault: b.IsDefault, ActorID: actorID,
			}); err != nil {
				return nil, fmt.Errorf("insert bank account: %w", err)
			}
		} else if _, err := q.UpdateBankAccount(ctx, sqlcgen.UpdateBankAccountParams{
			ID: b.ID, PartyType: partyTypeSupplier, PartyID: supplierID, BankName: b.BankName, AccountNumberEnc: enc,
			AccountName: b.AccountName, IsDefault: b.IsDefault, ActorID: actorID,
		}); err != nil {
			return nil, fmt.Errorf("update bank account: %w", err)
		}
		out[i] = b
	}
	return out, nil
}

// InsertSupplier menyimpan pemasok baru beserta rekeningnya. Kode yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) InsertSupplier(ctx context.Context, sup domain.Supplier, actorID uuid.UUID) (domain.Supplier, error) {
	npwp, err := s.encrypt(sup.NPWP)
	if err != nil {
		return domain.Supplier{}, err
	}
	r, err := s.q(ctx).InsertSupplier(ctx, sqlcgen.InsertSupplierParams{
		Code: sup.Code, Name: sup.Name, SupplierType: string(sup.Type), IsBusinessEntity: sup.IsBusinessEntity,
		NpwpEnc: npwp, DefaultWithholding: string(sup.DefaultWithholding), PaymentTermDays: v32(sup.PaymentTermDays),
		Address: sup.Address, Phone: sup.Phone, Email: sup.Email, PicName: sup.PICName, Notes: sup.Notes, ActorID: actorID,
	})
	if violation(err) == "uq_suppliers_code" {
		return domain.Supplier{}, apperr.New(apperr.CodeDuplicateCode)
	}
	if err != nil {
		return domain.Supplier{}, fmt.Errorf("insert supplier: %w", err)
	}
	out, err := s.toSupplier(r)
	if err != nil {
		return domain.Supplier{}, err
	}
	if out.BankAccounts, err = s.saveBankAccounts(ctx, out.ID, nil, sup.BankAccounts, actorID); err != nil {
		return domain.Supplier{}, err
	}
	return out, nil
}

// GetSupplier membaca pemasok beserta rekening; forUpdate mengunci baris.
func (s *Store) GetSupplier(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Supplier, error) {
	var (
		r   sqlcgen.Supplier
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetSupplierForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetSupplier(ctx, id)
	}
	if err != nil {
		return domain.Supplier{}, notFound(err, "get supplier")
	}
	sup, err := s.toSupplier(r)
	if err != nil {
		return domain.Supplier{}, err
	}
	list := []domain.Supplier{sup}
	if err := s.withBankAccounts(ctx, list); err != nil {
		return domain.Supplier{}, err
	}
	return list[0], nil
}

// ListSuppliers mengembalikan satu halaman pemasok beserta rekeningnya.
func (s *Store) ListSuppliers(ctx context.Context, f SupplierFilter, page paging.Page) (paging.Result[domain.Supplier], error) {
	q := s.q(ctx)
	search, typ := text(f.Search), text(string(f.Type))
	rows, err := q.ListSuppliers(ctx, sqlcgen.ListSuppliersParams{
		Search: search, SupplierType: typ, Sort: f.Sort, RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Supplier]{}, fmt.Errorf("list suppliers: %w", err)
	}
	total, err := q.CountSuppliers(ctx, sqlcgen.CountSuppliersParams{Search: search, SupplierType: typ})
	if err != nil {
		return paging.Result[domain.Supplier]{}, fmt.Errorf("count suppliers: %w", err)
	}
	items := make([]domain.Supplier, len(rows))
	for i, r := range rows {
		if items[i], err = s.toSupplier(r); err != nil {
			return paging.Result[domain.Supplier]{}, err
		}
	}
	if err := s.withBankAccounts(ctx, items); err != nil {
		return paging.Result[domain.Supplier]{}, err
	}
	return paging.Result[domain.Supplier]{Items: items, Page: page, Total: total}, nil
}

// UpdateSupplier menyimpan perubahan pemasok dan rekening dengan optimistic locking.
func (s *Store) UpdateSupplier(ctx context.Context, before, after domain.Supplier, actorID uuid.UUID) (domain.Supplier, error) {
	npwp, err := s.encrypt(after.NPWP)
	if err != nil {
		return domain.Supplier{}, err
	}
	n, err := s.q(ctx).UpdateSupplier(ctx, sqlcgen.UpdateSupplierParams{
		ID: after.ID, Version: v32(before.Version), Name: after.Name, SupplierType: string(after.Type),
		IsBusinessEntity: after.IsBusinessEntity, NpwpEnc: npwp, DefaultWithholding: string(after.DefaultWithholding),
		PaymentTermDays: v32(after.PaymentTermDays), Address: after.Address, Phone: after.Phone, Email: after.Email,
		PicName: after.PICName, Notes: after.Notes, ActorID: actorID,
	})
	if err := versionConflict(n, err, "update supplier"); err != nil {
		return domain.Supplier{}, err
	}
	if after.BankAccounts, err = s.saveBankAccounts(ctx, after.ID, before.BankAccounts, after.BankAccounts, actorID); err != nil {
		return domain.Supplier{}, err
	}
	after.Version = before.Version + 1
	return after, nil
}

// DeleteSupplier melakukan soft delete.
func (s *Store) DeleteSupplier(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteSupplier(ctx, sqlcgen.SoftDeleteSupplierParams{ID: id, Version: v32(version), ActorID: actorID})
	return versionConflict(n, err, "delete supplier")
}
