package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Permission master alasan retur (Desain Role & Approval bagian 3).
const (
	PermReturnReasonRead  = "sales.return_reason.read"
	PermReturnReasonWrite = "sales.return_reason.write"
	// PermSalesReturnRead dan PermPurchaseReturnRead: pembuat dokumen retur memilih alasan tanpa akses master.
	PermSalesReturnRead    = "sales.sales_return.read"
	PermPurchaseReturnRead = "procurement.purchase_return.read"
)

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type returnReasonStore interface {
	ListReturnReasons(ctx context.Context, f postgres.ReturnReasonFilter) ([]domain.ReturnReason, error)
	GetReturnReason(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.ReturnReason, error)
	GetReturnReasonByCode(ctx context.Context, code string) (domain.ReturnReason, error)
	InsertReturnReason(ctx context.Context, r domain.ReturnReason, actorID uuid.UUID) (domain.ReturnReason, error)
	UpdateReturnReason(ctx context.Context, r domain.ReturnReason, version int, actorID uuid.UUID) (domain.ReturnReason, error)
	DeleteReturnReason(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error
}

// ReturnReasonService mengelola master alasan retur (MD-13, S-124).
type ReturnReasonService struct {
	store returnReasonStore
	tx    txRunner
	audit audit.Recorder
}

// NewReturnReasonService membuat ReturnReasonService.
func NewReturnReasonService(store returnReasonStore, tx txRunner, rec audit.Recorder) *ReturnReasonService {
	return &ReturnReasonService{store: store, tx: tx, audit: rec}
}

func returnReasonSnapshot(r domain.ReturnReason) map[string]any {
	return map[string]any{
		"code": r.Code, "name": r.Name, "appliesTo": r.AppliesTo, "defaultDisposition": r.DefaultDisposition,
		"description": r.Description, "isActive": r.IsActive,
	}
}

// List mengembalikan alasan retur (Data Master › Alasan Retur, dropdown dokumen retur).
func (s *ReturnReasonService) List(ctx context.Context, f postgres.ReturnReasonFilter) ([]domain.ReturnReason, error) {
	if _, err := actor.RequireAny(ctx, PermReturnReasonRead, PermSalesReturnRead, PermPurchaseReturnRead); err != nil {
		return nil, err
	}
	return s.store.ListReturnReasons(ctx, f)
}

// Get membaca alasan retur untuk use case lain tanpa cek permission; pemanggil memeriksa permission dokumennya.
func (s *ReturnReasonService) Get(ctx context.Context, id uuid.UUID) (domain.ReturnReason, error) {
	return s.store.GetReturnReason(ctx, id, false)
}

// GetByCode membaca alasan retur berdasarkan kode (impor) tanpa cek permission.
func (s *ReturnReasonService) GetByCode(ctx context.Context, code string) (domain.ReturnReason, error) {
	return s.store.GetReturnReasonByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
}

// Create menambah alasan retur.
func (s *ReturnReasonService) Create(ctx context.Context, in domain.ReturnReasonInput) (domain.ReturnReason, error) {
	a, err := actor.Require(ctx, PermReturnReasonWrite)
	if err != nil {
		return domain.ReturnReason{}, err
	}
	r, err := domain.NewReturnReason(in)
	if err != nil {
		return domain.ReturnReason{}, err
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if r, err = s.store.InsertReturnReason(ctx, r, a.UserID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "return_reason", EntityID: r.ID, EntityLabel: r.Code, After: returnReasonSnapshot(r),
		})
	})
	return r, err
}

// Update mengubah alasan retur. Kode tidak berubah.
func (s *ReturnReasonService) Update(ctx context.Context, id uuid.UUID, version int, in domain.ReturnReasonInput) (domain.ReturnReason, error) {
	a, err := actor.Require(ctx, PermReturnReasonWrite)
	if err != nil {
		return domain.ReturnReason{}, err
	}
	var r domain.ReturnReason
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if r, err = s.store.GetReturnReason(ctx, id, true); err != nil {
			return err
		}
		if r.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := r
		if err := r.Update(in); err != nil {
			return err
		}
		if r, err = s.store.UpdateReturnReason(ctx, r, version, a.UserID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "return_reason", EntityID: r.ID, EntityLabel: r.Code,
			Before: returnReasonSnapshot(before), After: returnReasonSnapshot(r),
		})
	})
	return r, err
}

// Delete menghapus (soft delete) alasan retur. Kode tetap tidak bisa dipakai ulang. Pemeriksaan pemakaian
// oleh dokumen retur ditambahkan bersama dokumennya (S-314, S-210).
func (s *ReturnReasonService) Delete(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermReturnReasonWrite)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		r, err := s.store.GetReturnReason(ctx, id, true)
		if err != nil {
			return err
		}
		if r.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if err := s.store.DeleteReturnReason(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "return_reason", EntityID: r.ID, EntityLabel: r.Code, Before: returnReasonSnapshot(r),
		})
	})
}
