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
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// PartnerFilter adalah filter daftar mitra.
type PartnerFilter struct {
	Search string
	Type   domain.PartnerType
	Sort   string
}

func (s *Store) toPartner(r sqlcgen.Partner) (domain.Partner, error) {
	npwp, err := s.decrypt(r.NpwpEnc)
	if err != nil {
		return domain.Partner{}, fmt.Errorf("partner %s npwp: %w", r.ID, err)
	}
	return domain.Partner{
		ID: r.ID, Code: r.Code, Name: r.Name, Type: domain.PartnerType(r.PartnerType), NPWP: npwp, Address: r.Address,
		PICName: r.PicName, PICPhone: r.PicPhone, SalesUserID: uuidPtr(r.SalesUserID),
		MerchandiserUserID: uuidPtr(r.MerchandiserUserID), Notes: r.Notes, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}, nil
}

func (s *Store) toPartners(rows []sqlcgen.Partner) ([]domain.Partner, error) {
	out := make([]domain.Partner, len(rows))
	for i, r := range rows {
		p, err := s.toPartner(r)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

// InsertPartner menyimpan mitra baru. Kode yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) InsertPartner(ctx context.Context, p domain.Partner, actorID uuid.UUID) (domain.Partner, error) {
	npwp, err := s.encrypt(p.NPWP)
	if err != nil {
		return domain.Partner{}, err
	}
	r, err := s.q(ctx).InsertPartner(ctx, sqlcgen.InsertPartnerParams{
		Code: p.Code, Name: p.Name, PartnerType: string(p.Type), NpwpEnc: npwp, Address: p.Address,
		PicName: p.PICName, PicPhone: p.PICPhone, SalesUserID: nullUUID(p.SalesUserID),
		MerchandiserUserID: nullUUID(p.MerchandiserUserID), Notes: p.Notes, ActorID: actorID,
	})
	if violation(err) == "uq_partners_code" {
		return domain.Partner{}, apperr.New(apperr.CodeDuplicateCode)
	}
	if err != nil {
		return domain.Partner{}, fmt.Errorf("insert partner: %w", err)
	}
	return s.toPartner(r)
}

// GetPartner membaca mitra; forUpdate mengunci baris.
func (s *Store) GetPartner(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Partner, error) {
	var (
		r   sqlcgen.Partner
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPartnerForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPartner(ctx, id)
	}
	if err != nil {
		return domain.Partner{}, notFound(err, "get partner")
	}
	return s.toPartner(r)
}

// PartnersByIDs membaca mitra (termasuk yang terhapus) untuk ditampilkan.
func (s *Store) PartnersByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Partner, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListPartnersByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list partners by id: %w", err)
	}
	return s.toPartners(rows)
}

// PartnersByMerchandiser mengembalikan mitra yang ditugaskan ke merchandiser.
func (s *Store) PartnersByMerchandiser(ctx context.Context, userID uuid.UUID) ([]domain.Partner, error) {
	rows, err := s.q(ctx).ListPartnersByMerchandiser(ctx, nullUUID(&userID))
	if err != nil {
		return nil, fmt.Errorf("list partners by merchandiser: %w", err)
	}
	return s.toPartners(rows)
}

// ListPartners mengembalikan satu halaman mitra.
func (s *Store) ListPartners(ctx context.Context, f PartnerFilter, page paging.Page) (paging.Result[domain.Partner], error) {
	q := s.q(ctx)
	search, typ := text(f.Search), text(string(f.Type))
	rows, err := q.ListPartners(ctx, sqlcgen.ListPartnersParams{
		Search: search, PartnerType: typ, Sort: f.Sort, RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Partner]{}, fmt.Errorf("list partners: %w", err)
	}
	total, err := q.CountPartners(ctx, sqlcgen.CountPartnersParams{Search: search, PartnerType: typ})
	if err != nil {
		return paging.Result[domain.Partner]{}, fmt.Errorf("count partners: %w", err)
	}
	items, err := s.toPartners(rows)
	if err != nil {
		return paging.Result[domain.Partner]{}, err
	}
	return paging.Result[domain.Partner]{Items: items, Page: page, Total: total}, nil
}

// UpdatePartner menyimpan perubahan mitra dengan optimistic locking.
func (s *Store) UpdatePartner(ctx context.Context, p domain.Partner, version int, actorID uuid.UUID) error {
	npwp, err := s.encrypt(p.NPWP)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).UpdatePartner(ctx, sqlcgen.UpdatePartnerParams{
		ID: p.ID, Version: v32(version), Name: p.Name, PartnerType: string(p.Type), NpwpEnc: npwp, Address: p.Address,
		PicName: p.PICName, PicPhone: p.PICPhone, SalesUserID: nullUUID(p.SalesUserID),
		MerchandiserUserID: nullUUID(p.MerchandiserUserID), Notes: p.Notes, ActorID: actorID,
	})
	return versionConflict(n, err, "update partner")
}

// DeletePartner melakukan soft delete.
func (s *Store) DeletePartner(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeletePartner(ctx, sqlcgen.SoftDeletePartnerParams{ID: id, Version: v32(version), ActorID: actorID})
	return versionConflict(n, err, "delete partner")
}

// --- PKS ----------------------------------------------------------------------------------------

func toAgreement(r sqlcgen.PartnerAgreement) domain.Agreement {
	return domain.Agreement{
		ID: r.ID, PartnerID: r.PartnerID, Number: r.AgreementNumber, Type: domain.AgreementType(r.AgreementType),
		ValidFrom: r.ValidFrom, ValidTo: r.ValidTo, FileID: uuidPtr(r.FileObjectID), PriceListID: uuidPtr(r.PriceListID),
		MarginPercent: decimalPtr(r.ConsignmentMarginPercent), PaymentTermDays: int(r.PaymentTermDays),
		LossBorneBy: domain.BorneBy(r.LossBorneBy), PartnerDamageBorneBy: domain.BorneBy(r.PartnerDamageBorneBy),
		ProductDefectBorneBy: domain.BorneBy(r.ProductDefectBorneBy), NearExpiryBorneBy: domain.BorneBy(r.NearExpiryBorneBy),
		Notes: r.Notes, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toAgreements(rows []sqlcgen.PartnerAgreement) []domain.Agreement {
	out := make([]domain.Agreement, len(rows))
	for i, r := range rows {
		out[i] = toAgreement(r)
	}
	return out
}

func agreementError(err error, what string) error {
	switch violation(err) {
	case "ex_partner_agreements_period":
		return apperr.Validation(map[string]string{"validFrom": "Periode bertumpang tindih dengan PKS lain yang sejenis. Persingkat PKS lama terlebih dahulu."})
	case "uq_partner_agreements_number":
		return apperr.Validation(map[string]string{"agreementNumber": "Nomor PKS sudah dipakai untuk mitra ini"})
	}
	return fmt.Errorf("%s: %w", what, err)
}

// InsertAgreement menyimpan PKS baru. Periode tumpang tindih ditolak EXCLUDE constraint.
func (s *Store) InsertAgreement(ctx context.Context, a domain.Agreement, actorID uuid.UUID) (domain.Agreement, error) {
	r, err := s.q(ctx).InsertPartnerAgreement(ctx, sqlcgen.InsertPartnerAgreementParams{
		PartnerID: a.PartnerID, AgreementNumber: a.Number, AgreementType: string(a.Type), ValidFrom: a.ValidFrom,
		ValidTo: a.ValidTo, FileObjectID: nullUUID(a.FileID), PriceListID: nullUUID(a.PriceListID),
		ConsignmentMarginPercent: nullDecimal(a.MarginPercent), PaymentTermDays: v32(a.PaymentTermDays),
		LossBorneBy: string(a.LossBorneBy), PartnerDamageBorneBy: string(a.PartnerDamageBorneBy),
		ProductDefectBorneBy: string(a.ProductDefectBorneBy), NearExpiryBorneBy: string(a.NearExpiryBorneBy),
		Notes: a.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.Agreement{}, agreementError(err, "insert partner agreement")
	}
	return toAgreement(r), nil
}

// GetAgreementForUpdate mengunci satu PKS.
func (s *Store) GetAgreementForUpdate(ctx context.Context, id uuid.UUID) (domain.Agreement, error) {
	r, err := s.q(ctx).GetPartnerAgreementForUpdate(ctx, id)
	if err != nil {
		return domain.Agreement{}, notFound(err, "get partner agreement")
	}
	return toAgreement(r), nil
}

// ListAgreements mengembalikan riwayat PKS mitra.
func (s *Store) ListAgreements(ctx context.Context, partnerID uuid.UUID) ([]domain.Agreement, error) {
	rows, err := s.q(ctx).ListPartnerAgreements(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("list partner agreements: %w", err)
	}
	return toAgreements(rows), nil
}

// AgreementsActiveOn mengembalikan PKS yang berlaku pada tanggal untuk beberapa mitra.
func (s *Store) AgreementsActiveOn(ctx context.Context, partnerIDs []uuid.UUID, on clock.BusinessDate) ([]domain.Agreement, error) {
	if len(partnerIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListAgreementsActiveOn(ctx, sqlcgen.ListAgreementsActiveOnParams{PartnerIds: partnerIDs, OnDate: on})
	if err != nil {
		return nil, fmt.Errorf("list active agreements: %w", err)
	}
	return toAgreements(rows), nil
}

// ActiveAgreement mengembalikan PKS jenis t yang berlaku pada tanggal; ok=false jika tidak ada.
func (s *Store) ActiveAgreement(ctx context.Context, partnerID uuid.UUID, t domain.AgreementType, on clock.BusinessDate) (domain.Agreement, bool, error) {
	r, err := s.q(ctx).GetActiveAgreement(ctx, sqlcgen.GetActiveAgreementParams{
		PartnerID: partnerID, AgreementType: string(t), OnDate: on,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Agreement{}, false, nil
	}
	if err != nil {
		return domain.Agreement{}, false, fmt.Errorf("get active agreement: %w", err)
	}
	return toAgreement(r), true, nil
}

// UpdateAgreement menyimpan koreksi PKS.
func (s *Store) UpdateAgreement(ctx context.Context, a domain.Agreement, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdatePartnerAgreement(ctx, sqlcgen.UpdatePartnerAgreementParams{
		ID: a.ID, Version: v32(version), AgreementNumber: a.Number, AgreementType: string(a.Type),
		ValidFrom: a.ValidFrom, ValidTo: a.ValidTo, FileObjectID: nullUUID(a.FileID), PriceListID: nullUUID(a.PriceListID),
		ConsignmentMarginPercent: nullDecimal(a.MarginPercent), PaymentTermDays: v32(a.PaymentTermDays),
		LossBorneBy: string(a.LossBorneBy), PartnerDamageBorneBy: string(a.PartnerDamageBorneBy),
		ProductDefectBorneBy: string(a.ProductDefectBorneBy), NearExpiryBorneBy: string(a.NearExpiryBorneBy),
		Notes: a.Notes, ActorID: actorID,
	})
	if err != nil {
		return agreementError(err, "update partner agreement")
	}
	return versionConflict(n, nil, "update partner agreement")
}

// DeleteAgreement melakukan soft delete PKS.
func (s *Store) DeleteAgreement(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeletePartnerAgreement(ctx, sqlcgen.SoftDeletePartnerAgreementParams{
		ID: id, Version: v32(version), ActorID: actorID,
	})
	return versionConflict(n, err, "delete partner agreement")
}

// --- Kode produk mitra --------------------------------------------------------------------------

// ProductCodeFilter adalah filter daftar kode produk mitra.
type ProductCodeFilter struct {
	PartnerID *uuid.UUID
	ProductID *uuid.UUID
	Search    string
}

func toProductCode(r sqlcgen.PartnerProductCode) domain.ProductCode {
	return domain.ProductCode{
		ID: r.ID, PartnerID: r.PartnerID, ProductID: r.ProductID, ArticleCode: r.ArticleCode, Barcode: r.Barcode,
		Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func productCodeError(err error, what string) error {
	switch violation(err) {
	case "uq_partner_product_codes_partner_product":
		return apperr.Validation(map[string]string{"productId": "Produk ini sudah punya kode di mitra yang sama"})
	case "uq_partner_product_codes_partner_article":
		return apperr.Validation(map[string]string{"articleCode": "Kode artikel sudah dipakai produk lain di mitra yang sama"})
	}
	return fmt.Errorf("%s: %w", what, err)
}

// InsertProductCode menyimpan kode produk mitra.
func (s *Store) InsertProductCode(ctx context.Context, c domain.ProductCode, actorID uuid.UUID) (domain.ProductCode, error) {
	r, err := s.q(ctx).InsertPartnerProductCode(ctx, sqlcgen.InsertPartnerProductCodeParams{
		PartnerID: c.PartnerID, ProductID: c.ProductID, ArticleCode: c.ArticleCode, Barcode: c.Barcode,
		Description: c.Description, ActorID: actorID,
	})
	if err != nil {
		return domain.ProductCode{}, productCodeError(err, "insert partner product code")
	}
	return toProductCode(r), nil
}

// GetProductCodeForUpdate mengunci satu kode produk mitra.
func (s *Store) GetProductCodeForUpdate(ctx context.Context, id uuid.UUID) (domain.ProductCode, error) {
	r, err := s.q(ctx).GetPartnerProductCodeForUpdate(ctx, id)
	if err != nil {
		return domain.ProductCode{}, notFound(err, "get partner product code")
	}
	return toProductCode(r), nil
}

// ProductCodeFor mengembalikan kode produk satu SKU di satu mitra; ok=false jika belum ada.
func (s *Store) ProductCodeFor(ctx context.Context, partnerID, productID uuid.UUID) (domain.ProductCode, bool, error) {
	r, err := s.q(ctx).GetPartnerProductCodeByProduct(ctx, sqlcgen.GetPartnerProductCodeByProductParams{
		PartnerID: partnerID, ProductID: productID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProductCode{}, false, nil
	}
	if err != nil {
		return domain.ProductCode{}, false, fmt.Errorf("get partner product code: %w", err)
	}
	return toProductCode(r), true, nil
}

// ListProductCodes mengembalikan satu halaman kode produk mitra.
func (s *Store) ListProductCodes(ctx context.Context, f ProductCodeFilter, page paging.Page) (paging.Result[domain.ProductCode], error) {
	q := s.q(ctx)
	partner, product, search := nullUUID(f.PartnerID), nullUUID(f.ProductID), text(f.Search)
	rows, err := q.ListPartnerProductCodes(ctx, sqlcgen.ListPartnerProductCodesParams{
		PartnerID: partner, ProductID: product, Search: search, RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.ProductCode]{}, fmt.Errorf("list partner product codes: %w", err)
	}
	total, err := q.CountPartnerProductCodes(ctx, sqlcgen.CountPartnerProductCodesParams{
		PartnerID: partner, ProductID: product, Search: search,
	})
	if err != nil {
		return paging.Result[domain.ProductCode]{}, fmt.Errorf("count partner product codes: %w", err)
	}
	items := make([]domain.ProductCode, len(rows))
	for i, r := range rows {
		items[i] = toProductCode(r)
	}
	return paging.Result[domain.ProductCode]{Items: items, Page: page, Total: total}, nil
}

// UpdateProductCode menyimpan perubahan kode produk mitra.
func (s *Store) UpdateProductCode(ctx context.Context, c domain.ProductCode, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdatePartnerProductCode(ctx, sqlcgen.UpdatePartnerProductCodeParams{
		ID: c.ID, Version: v32(version), ArticleCode: c.ArticleCode, Barcode: c.Barcode, Description: c.Description,
		ActorID: actorID,
	})
	if err != nil {
		return productCodeError(err, "update partner product code")
	}
	return versionConflict(n, nil, "update partner product code")
}

// DeleteProductCode melakukan soft delete.
func (s *Store) DeleteProductCode(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeletePartnerProductCode(ctx, sqlcgen.SoftDeletePartnerProductCodeParams{
		ID: id, Version: v32(version), ActorID: actorID,
	})
	return versionConflict(n, err, "delete partner product code")
}
