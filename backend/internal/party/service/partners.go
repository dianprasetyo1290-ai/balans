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
	"balans/internal/platform/pii"
)

func maskedOrNil(v string) any {
	if v == "" {
		return nil
	}
	return pii.Mask(v)
}

func partnerSnapshot(p domain.Partner) map[string]any {
	return map[string]any{
		"code": p.Code, "name": p.Name, "partnerType": p.Type, "npwp": maskedOrNil(p.NPWP), "address": p.Address,
		"picName": p.PICName, "picPhone": p.PICPhone, "salesUserId": p.SalesUserID,
		"merchandiserUserId": p.MerchandiserUserID, "notes": p.Notes,
	}
}

func agreementSnapshot(a domain.Agreement) map[string]any {
	m := map[string]any{
		"partnerId": a.PartnerID, "agreementNumber": a.Number, "agreementType": a.Type, "validFrom": a.ValidFrom,
		"validTo": a.ValidTo, "fileId": a.FileID, "priceListId": a.PriceListID, "paymentTermDays": a.PaymentTermDays,
		"lossBorneBy": a.LossBorneBy, "partnerDamageBorneBy": a.PartnerDamageBorneBy,
		"productDefectBorneBy": a.ProductDefectBorneBy, "nearExpiryBorneBy": a.NearExpiryBorneBy, "notes": a.Notes,
	}
	if a.MarginPercent != nil {
		m["consignmentMarginPercent"] = a.MarginPercent.StringFixed(4)
	}
	return m
}

// PartnerQuery adalah filter daftar mitra dari API.
type PartnerQuery struct {
	Search string
	Type   domain.PartnerType
	Sort   paging.Sort
}

// ListPartners mengembalikan mitra.
func (s *Service) ListPartners(ctx context.Context, q PartnerQuery, page paging.Page) (paging.Result[domain.Partner], error) {
	if _, err := actor.Require(ctx, PermPartnerRead); err != nil {
		return paging.Result[domain.Partner]{}, err
	}
	return s.Store.ListPartners(ctx, postgres.PartnerFilter{Search: strings.TrimSpace(q.Search), Type: q.Type, Sort: q.Sort.Key()}, page)
}

// GetPartner mengembalikan detail mitra.
func (s *Service) GetPartner(ctx context.Context, id uuid.UUID) (domain.Partner, error) {
	if _, err := actor.Require(ctx, PermPartnerRead); err != nil {
		return domain.Partner{}, err
	}
	return s.Store.GetPartner(ctx, id, false)
}

// ActiveAgreementsByPartner mengembalikan PKS yang berlaku hari ini per mitra (untuk daftar & detail mitra).
// Kosong jika actor tidak punya hak baca PKS.
func (s *Service) ActiveAgreementsByPartner(ctx context.Context, partnerIDs []uuid.UUID) (map[uuid.UUID][]domain.Agreement, error) {
	out := map[uuid.UUID][]domain.Agreement{}
	if a, ok := actor.From(ctx); !ok || !a.Has(PermAgreementRead) {
		return out, nil
	}
	list, err := s.Store.AgreementsActiveOn(ctx, partnerIDs, s.today())
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		out[a.PartnerID] = append(out[a.PartnerID], a)
	}
	return out, nil
}

// checkPartnerUsers memastikan Sales dan Merchandiser yang ditugaskan adalah pengguna aktif.
func (s *Service) checkPartnerUsers(ctx context.Context, p domain.Partner) error {
	var ids []uuid.UUID
	for _, id := range []*uuid.UUID{p.SalesUserID, p.MerchandiserUserID} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	users, err := s.Users.UsersByID(ctx, ids)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	if p.SalesUserID != nil {
		if _, ok := users[*p.SalesUserID]; !ok {
			fields["salesUserId"] = "Pengguna tidak ditemukan atau nonaktif"
		}
	}
	if p.MerchandiserUserID != nil {
		if _, ok := users[*p.MerchandiserUserID]; !ok {
			fields["merchandiserUserId"] = "Pengguna tidak ditemukan atau nonaktif"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CreatePartner membuat mitra.
func (s *Service) CreatePartner(ctx context.Context, in domain.PartnerInput) (domain.Partner, error) {
	a, err := actor.Require(ctx, PermPartnerWrite)
	if err != nil {
		return domain.Partner{}, err
	}
	p, err := domain.NewPartner(in)
	if err != nil {
		return domain.Partner{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkPartnerUsers(ctx, p); err != nil {
			return err
		}
		if p, err = s.Store.InsertPartner(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPartner, EntityID: p.ID, EntityLabel: p.Code + " " + p.Name,
			After: partnerSnapshot(p),
		})
	})
	return p, err
}

// UpdatePartner mengubah mitra; kode tetap.
func (s *Service) UpdatePartner(ctx context.Context, id uuid.UUID, version int, in domain.PartnerInput) (domain.Partner, error) {
	a, err := actor.Require(ctx, PermPartnerWrite)
	if err != nil {
		return domain.Partner{}, err
	}
	var next domain.Partner
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetPartner(ctx, id, true)
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
		if err := s.checkPartnerUsers(ctx, next); err != nil {
			return err
		}
		if err := s.Store.UpdatePartner(ctx, next, version, a.UserID); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPartner, EntityID: next.ID, EntityLabel: next.Code + " " + next.Name,
			Before: partnerSnapshot(cur), After: partnerSnapshot(next),
		})
	})
	return next, err
}

// DeletePartner menghapus mitra (soft delete). Kode mitra tidak bisa dipakai ulang.
func (s *Service) DeletePartner(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermPartnerWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetPartner(ctx, id, true)
		if err != nil {
			return err
		}
		if err := s.Store.DeletePartner(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityPartner, EntityID: id, EntityLabel: cur.Code + " " + cur.Name,
			Before: partnerSnapshot(cur),
		})
	})
}

// PartnerByID dipakai modul lain tanpa cek permission.
func (s *Service) PartnerByID(ctx context.Context, id uuid.UUID) (domain.Partner, error) {
	return s.Store.GetPartner(ctx, id, false)
}

// PartnersForMerchandiser mengembalikan mitra yang ditugaskan ke merchandiser (membatasi stock count, MD-07).
func (s *Service) PartnersForMerchandiser(ctx context.Context, userID uuid.UUID) ([]domain.Partner, error) {
	return s.Store.PartnersByMerchandiser(ctx, userID)
}

// --- PKS ----------------------------------------------------------------------------------------

// ListAgreements mengembalikan riwayat PKS mitra.
func (s *Service) ListAgreements(ctx context.Context, partnerID uuid.UUID) ([]domain.Agreement, error) {
	if _, err := actor.Require(ctx, PermAgreementRead); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetPartner(ctx, partnerID, false); err != nil {
		return nil, err
	}
	return s.Store.ListAgreements(ctx, partnerID)
}

func (s *Service) attachAgreementFile(ctx context.Context, a domain.Agreement, previous *uuid.UUID) error {
	if a.FileID == nil || (previous != nil && *previous == *a.FileID) {
		return nil
	}
	return s.Files.Attach(ctx, *a.FileID, FileOwnerAgreement, a.ID)
}

// CreateAgreement mencatat PKS baru. Perubahan ketentuan di tengah masa berlaku dicatat sebagai PKS baru
// setelah PKS lama dipersingkat (effective-dated, Technical Guideline 5.14).
func (s *Service) CreateAgreement(ctx context.Context, partnerID uuid.UUID, in domain.AgreementInput) (domain.Agreement, error) {
	actorInfo, err := actor.Require(ctx, PermAgreementWrite)
	if err != nil {
		return domain.Agreement{}, err
	}
	a, err := domain.NewAgreement(partnerID, in)
	if err != nil {
		return domain.Agreement{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPartner(ctx, partnerID, true)
		if err != nil {
			return err
		}
		if a, err = s.Store.InsertAgreement(ctx, a, actorInfo.UserID); err != nil {
			return err
		}
		if err := s.attachAgreementFile(ctx, a, nil); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityAgreement, EntityID: a.ID,
			EntityLabel: p.Code + " PKS " + a.Number, After: agreementSnapshot(a),
		})
	})
	return a, err
}

// UpdateAgreement mengoreksi PKS (mis. salah input atau mempersingkat masa berlaku).
func (s *Service) UpdateAgreement(ctx context.Context, id uuid.UUID, version int, in domain.AgreementInput) (domain.Agreement, error) {
	actorInfo, err := actor.Require(ctx, PermAgreementWrite)
	if err != nil {
		return domain.Agreement{}, err
	}
	var next domain.Agreement
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetAgreementForUpdate(ctx, id)
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
		if err := s.Store.UpdateAgreement(ctx, next, version, actorInfo.UserID); err != nil {
			return err
		}
		if err := s.attachAgreementFile(ctx, next, cur.FileID); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityAgreement, EntityID: next.ID, EntityLabel: "PKS " + next.Number,
			Before: agreementSnapshot(cur), After: agreementSnapshot(next),
		})
	})
	return next, err
}

// DeleteAgreement menghapus PKS yang salah input (soft delete).
func (s *Service) DeleteAgreement(ctx context.Context, id uuid.UUID, version int) error {
	actorInfo, err := actor.Require(ctx, PermAgreementWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetAgreementForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Store.DeleteAgreement(ctx, id, version, actorInfo.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityAgreement, EntityID: id, EntityLabel: "PKS " + cur.Number,
			Before: agreementSnapshot(cur),
		})
	})
}

// ActiveAgreement mengembalikan PKS jenis t yang berlaku pada tanggal transaksi; ok=false jika tidak ada.
// Dipakai modul lain tanpa cek permission.
func (s *Service) ActiveAgreement(ctx context.Context, partnerID uuid.UUID, t domain.AgreementType, on clock.BusinessDate) (domain.Agreement, bool, error) {
	return s.Store.ActiveAgreement(ctx, partnerID, t, on)
}

// RequireActiveAgreement menolak transaksi (VALIDATION_FAILED pada `partnerId`) bila mitra tidak punya PKS
// jenis t yang berlaku pada tanggal transaksi, mis. DO konsinyasi (BRD 5.5).
func (s *Service) RequireActiveAgreement(ctx context.Context, partnerID uuid.UUID, t domain.AgreementType, on clock.BusinessDate) (domain.Agreement, error) {
	p, err := s.Store.GetPartner(ctx, partnerID, false)
	if err != nil {
		return domain.Agreement{}, err
	}
	a, ok, err := s.Store.ActiveAgreement(ctx, partnerID, t, on)
	if err != nil {
		return domain.Agreement{}, err
	}
	var found *domain.Agreement
	if ok {
		found = &a
	}
	if err := domain.RequireActive(found, t, p.Name, on); err != nil {
		return domain.Agreement{}, err
	}
	return a, nil
}

// FileNames mengembalikan nama file PDF PKS.
func (s *Service) FileNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	return s.Files.Names(ctx, ids)
}

// UserRefs mengembalikan nama pengguna (Sales/Merchandiser) untuk ditampilkan.
func (s *Service) UserRefs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]UserRef, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]UserRef{}, nil
	}
	return s.Users.UsersByID(ctx, ids)
}

// SearchUsers mencari pengguna aktif untuk dipilih sebagai Sales atau Merchandiser mitra.
func (s *Service) SearchUsers(ctx context.Context, q, role string) ([]UserRef, error) {
	if _, err := actor.RequireAny(ctx, PermPartnerWrite, PermPartnerRead); err != nil {
		return nil, err
	}
	return s.Users.SearchUsers(ctx, strings.TrimSpace(q), role, 20)
}
