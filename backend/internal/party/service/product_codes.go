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
)

func productCodeSnapshot(c domain.ProductCode) map[string]any {
	return map[string]any{
		"partnerId": c.PartnerID, "productId": c.ProductID, "articleCode": c.ArticleCode, "barcode": c.Barcode,
		"description": c.Description,
	}
}

// ProductCodeQuery adalah filter daftar kode produk mitra.
type ProductCodeQuery struct {
	PartnerID *uuid.UUID
	ProductID *uuid.UUID
	Search    string
}

// ListProductCodes mengembalikan kode produk mitra.
func (s *Service) ListProductCodes(ctx context.Context, q ProductCodeQuery, page paging.Page) (paging.Result[domain.ProductCode], error) {
	if _, err := actor.Require(ctx, PermProductCodeRead); err != nil {
		return paging.Result[domain.ProductCode]{}, err
	}
	return s.Store.ListProductCodes(ctx, postgres.ProductCodeFilter{
		PartnerID: q.PartnerID, ProductID: q.ProductID, Search: strings.TrimSpace(q.Search),
	}, page)
}

// CreateProductCode menambah kode artikel & barcode retailer untuk satu SKU di satu mitra.
func (s *Service) CreateProductCode(ctx context.Context, partnerID, productID uuid.UUID, in domain.ProductCodeInput) (domain.ProductCode, error) {
	a, err := actor.Require(ctx, PermProductCodeWrite)
	if err != nil {
		return domain.ProductCode{}, err
	}
	c, err := domain.NewProductCode(partnerID, productID, in)
	if err != nil {
		return domain.ProductCode{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPartner(ctx, partnerID, false)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{"partnerId": "Mitra tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		if s.Products != nil {
			products, err := s.Products.Products(ctx, []uuid.UUID{productID})
			if err != nil {
				return err
			}
			if _, ok := products[productID]; !ok {
				return apperr.Validation(map[string]string{"productId": "Produk tidak ditemukan"})
			}
		}
		if c, err = s.Store.InsertProductCode(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityProductCode, EntityID: c.ID,
			EntityLabel: p.Code + " " + c.ArticleCode, After: productCodeSnapshot(c),
		})
	})
	return c, err
}

// UpdateProductCode mengubah kode artikel, barcode, atau keterangan.
func (s *Service) UpdateProductCode(ctx context.Context, id uuid.UUID, version int, in domain.ProductCodeInput) (domain.ProductCode, error) {
	a, err := actor.Require(ctx, PermProductCodeWrite)
	if err != nil {
		return domain.ProductCode{}, err
	}
	var next domain.ProductCode
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetProductCodeForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		next = cur
		if err := next.Update(in); err != nil {
			return err
		}
		if err := s.Store.UpdateProductCode(ctx, next, version, a.UserID); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityProductCode, EntityID: next.ID, EntityLabel: next.ArticleCode,
			Before: productCodeSnapshot(cur), After: productCodeSnapshot(next),
		})
	})
	return next, err
}

// DeleteProductCode menghapus kode produk mitra (soft delete).
func (s *Service) DeleteProductCode(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermProductCodeWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetProductCodeForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Store.DeleteProductCode(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityProductCode, EntityID: id, EntityLabel: cur.ArticleCode,
			Before: productCodeSnapshot(cur),
		})
	})
}

// ProductCodeFor dipakai modul lain (label produk, DO) tanpa cek permission; ok=false jika SKU belum punya
// kode di mitra tersebut.
func (s *Service) ProductCodeFor(ctx context.Context, partnerID, productID uuid.UUID) (domain.ProductCode, bool, error) {
	return s.Store.ProductCodeFor(ctx, partnerID, productID)
}

// PartnerRefs mengembalikan mitra (termasuk terhapus) untuk ditampilkan di daftar kode produk.
func (s *Service) PartnerRefs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Partner, error) {
	list, err := s.Store.PartnersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.Partner, len(list))
	for _, p := range list {
		out[p.ID] = p
	}
	return out, nil
}

// ProductRefs mengembalikan SKU dan nama produk bila modul catalog aktif.
func (s *Service) ProductRefs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error) {
	if s.Products == nil || len(ids) == 0 {
		return map[uuid.UUID]ProductRef{}, nil
	}
	return s.Products.Products(ctx, ids)
}
