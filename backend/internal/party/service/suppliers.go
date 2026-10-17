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
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

// supplierSnapshot memuat NPWP dan nomor rekening dalam bentuk tersamar (NFR-04, TG 6.6).
func supplierSnapshot(s domain.Supplier) map[string]any {
	accounts := make([]map[string]any, len(s.BankAccounts))
	for i, b := range s.BankAccounts {
		accounts[i] = map[string]any{
			"bankName": b.BankName, "accountNumber": pii.Mask(b.AccountNumber), "accountName": b.AccountName,
			"isDefault": b.IsDefault,
		}
	}
	return map[string]any{
		"code": s.Code, "name": s.Name, "supplierType": s.Type, "isBusinessEntity": s.IsBusinessEntity,
		"npwp": maskedOrNil(s.NPWP), "defaultWithholding": s.DefaultWithholding, "paymentTermDays": s.PaymentTermDays,
		"address": s.Address, "phone": s.Phone, "email": s.Email, "picName": s.PICName, "notes": s.Notes,
		"bankAccounts": accounts,
	}
}

// SupplierQuery adalah filter daftar pemasok dari API.
type SupplierQuery struct {
	Search string
	Type   domain.SupplierType
	Sort   paging.Sort
}

// ListSuppliers mengembalikan pemasok & maklon.
func (s *Service) ListSuppliers(ctx context.Context, q SupplierQuery, page paging.Page) (paging.Result[domain.Supplier], error) {
	if _, err := actor.Require(ctx, PermSupplierRead); err != nil {
		return paging.Result[domain.Supplier]{}, err
	}
	return s.Store.ListSuppliers(ctx, postgres.SupplierFilter{Search: strings.TrimSpace(q.Search), Type: q.Type, Sort: q.Sort.Key()}, page)
}

// GetSupplier mengembalikan detail pemasok.
func (s *Service) GetSupplier(ctx context.Context, id uuid.UUID) (domain.Supplier, error) {
	if _, err := actor.Require(ctx, PermSupplierRead); err != nil {
		return domain.Supplier{}, err
	}
	return s.Store.GetSupplier(ctx, id, false)
}

// CreateSupplier membuat pemasok atau maklon. NPWP dan rekening dienkripsi saat disimpan.
func (s *Service) CreateSupplier(ctx context.Context, in domain.SupplierInput) (domain.Supplier, error) {
	a, err := actor.Require(ctx, PermSupplierWrite)
	if err != nil {
		return domain.Supplier{}, err
	}
	sup, err := domain.NewSupplier(in)
	if err != nil {
		return domain.Supplier{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if sup, err = s.Store.InsertSupplier(ctx, sup, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntitySupplier, EntityID: sup.ID, EntityLabel: sup.Code + " " + sup.Name,
			After: supplierSnapshot(sup),
		})
	})
	return sup, err
}

// UpdateSupplier mengubah pemasok; kode tetap.
func (s *Service) UpdateSupplier(ctx context.Context, id uuid.UUID, version int, in domain.SupplierInput) (domain.Supplier, error) {
	a, err := actor.Require(ctx, PermSupplierWrite)
	if err != nil {
		return domain.Supplier{}, err
	}
	var next domain.Supplier
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetSupplier(ctx, id, true)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		next = cur
		next.BankAccounts = append([]domain.BankAccount(nil), cur.BankAccounts...)
		if err := next.Update(in); err != nil {
			return err
		}
		if next, err = s.Store.UpdateSupplier(ctx, cur, next, a.UserID); err != nil {
			return err
		}
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntitySupplier, EntityID: next.ID, EntityLabel: next.Code + " " + next.Name,
			Before: supplierSnapshot(cur), After: supplierSnapshot(next),
		})
	})
	return next, err
}

// DeleteSupplier menghapus pemasok (soft delete). Kode tidak bisa dipakai ulang.
func (s *Service) DeleteSupplier(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermSupplierWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetSupplier(ctx, id, true)
		if err != nil {
			return err
		}
		if err := s.Store.DeleteSupplier(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntitySupplier, EntityID: id, EntityLabel: cur.Code + " " + cur.Name,
			Before: supplierSnapshot(cur),
		})
	})
}

// SupplierByID dipakai modul lain (procurement, accounting) tanpa cek permission. Nomor rekening dan NPWP
// sudah terdekripsi; pemanggil wajib tidak menampilkannya tanpa masking.
func (s *Service) SupplierByID(ctx context.Context, id uuid.UUID) (domain.Supplier, error) {
	return s.Store.GetSupplier(ctx, id, false)
}
