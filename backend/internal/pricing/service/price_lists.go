package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/softdelete"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
)

// PriceListView adalah daftar harga beserta ringkasan versinya.
type PriceListView struct {
	domain.PriceList
	// Current adalah versi yang berlaku hari ini.
	Current *domain.PriceListVersion
	// Next adalah versi aktif terdekat yang mulai berlaku setelah hari ini.
	Next *domain.PriceListVersion
	// Open adalah versi draft atau yang sedang diajukan.
	Open *domain.PriceListVersion
	// PartnerName dan ReferenceName untuk tampilan.
	PartnerName   string
	ReferenceName string
}

// PriceListDetail adalah detail daftar harga: header, riwayat versi, dan aksi yang boleh dilakukan user.
type PriceListDetail struct {
	PriceListView
	Versions       []domain.PriceListVersion
	AllowedActions []string
}

func priceListSnapshot(p domain.PriceList) map[string]any {
	return map[string]any{
		"code": p.Code, "name": p.Name, "priceListType": p.Type, "channelId": idText(p.ChannelID),
		"subChannelId": idText(p.SubChannelID), "partnerId": idText(p.PartnerID), "resellerLevel": p.ResellerLevel,
		"basis": p.Basis, "referencePriceListId": idText(p.ReferenceID), "description": p.Description, "isActive": p.IsActive,
	}
}

func readPriceList(ctx context.Context) (actor.Actor, error) {
	return actor.RequireAny(ctx, PermPriceListRead, PermPriceListApprove)
}

// ListPriceLists mengembalikan Data Master › Daftar Harga.
func (s *Service) ListPriceLists(ctx context.Context, f postgres.PriceListFilter) ([]PriceListView, error) {
	if _, err := readPriceList(ctx); err != nil {
		return nil, err
	}
	f.Query = normalizeQuery(f.Query)
	lists, err := s.Store.ListPriceLists(ctx, f)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, lists)
}

// PriceListOptions mengembalikan daftar harga aktif untuk dropdown (acuan basis persen, harga bertingkat).
func (s *Service) PriceListOptions(ctx context.Context) ([]domain.PriceList, error) {
	if _, err := actor.RequireAny(ctx, PermPriceListRead, PermPriceListApprove, PermPromotionRead, PermSpecialPriceRead); err != nil {
		return nil, err
	}
	return s.Store.ListPriceLists(ctx, postgres.PriceListFilter{})
}

func (s *Service) views(ctx context.Context, lists []domain.PriceList) ([]PriceListView, error) {
	ids := make([]uuid.UUID, len(lists))
	var partnerIDs, refIDs []uuid.UUID
	for i, l := range lists {
		ids[i] = l.ID
		if l.PartnerID != nil {
			partnerIDs = append(partnerIDs, *l.PartnerID)
		}
		if l.ReferenceID != nil {
			refIDs = append(refIDs, *l.ReferenceID)
		}
	}
	versions, err := s.Store.CurrentVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	partners := map[uuid.UUID]PartyRef{}
	if s.Parties != nil && len(partnerIDs) > 0 {
		if partners, err = s.Parties.Partners(ctx, uniqueIDs(partnerIDs)); err != nil {
			return nil, err
		}
	}
	refs, err := s.Store.PriceListsByIDs(ctx, uniqueIDs(refIDs))
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]PriceListView, len(lists))
	for i, l := range lists {
		v := PriceListView{PriceList: l}
		for _, ver := range versions[l.ID] {
			ver := ver
			switch {
			case ver.Status == domain.VersionDraft || ver.Status == domain.VersionPendingApproval:
				v.Open = &ver
			case ver.Status == domain.VersionActive && ver.Contains(today):
				v.Current = &ver
			case ver.Status == domain.VersionActive && ver.ValidFrom.After(today) && (v.Next == nil || ver.ValidFrom.Before(v.Next.ValidFrom)):
				v.Next = &ver
			}
		}
		if l.PartnerID != nil {
			v.PartnerName = partners[*l.PartnerID].Name
		}
		if l.ReferenceID != nil {
			v.ReferenceName = refs[*l.ReferenceID].Name
		}
		out[i] = v
	}
	return out, nil
}

// GetPriceList mengembalikan detail daftar harga beserta riwayat versinya.
func (s *Service) GetPriceList(ctx context.Context, id uuid.UUID) (PriceListDetail, error) {
	a, err := readPriceList(ctx)
	if err != nil {
		return PriceListDetail{}, err
	}
	l, err := s.Store.GetPriceList(ctx, id, false)
	if err != nil {
		return PriceListDetail{}, err
	}
	views, err := s.views(ctx, []domain.PriceList{l})
	if err != nil {
		return PriceListDetail{}, err
	}
	versions, err := s.Store.ListVersions(ctx, id)
	if err != nil {
		return PriceListDetail{}, err
	}
	d := PriceListDetail{PriceListView: views[0], Versions: versions}
	d.AllowedActions = priceListActions(a, d)
	return d, nil
}

func priceListActions(a actor.Actor, d PriceListDetail) []string {
	actions := []string{}
	if d.Deleted() {
		if softdelete.CanRestore(a, PermPriceListWrite) {
			actions = append(actions, "restore")
		}
		return actions
	}
	if a.Has(PermPriceListWrite) {
		actions = append(actions, "update")
		if d.Open == nil {
			actions = append(actions, "create_version")
		}
		if d.Type != domain.TypeBase {
			actions = append(actions, "delete")
		}
	}
	return actions
}

// VersionActions mengembalikan aksi yang boleh dilakukan user atas satu versi.
func VersionActions(a actor.Actor, v domain.PriceListVersion) []string {
	actions := []string{}
	switch v.Status {
	case domain.VersionDraft:
		if a.Has(PermPriceListWrite) {
			actions = append(actions, "update", "cancel")
		}
		if a.Has(PermPriceListSubmit) {
			actions = append(actions, "submit")
		}
	case domain.VersionPendingApproval:
		if a.Has(PermPriceListSubmit) {
			actions = append(actions, "withdraw")
		}
	}
	return actions
}

func (s *Service) checkReference(ctx context.Context, p domain.PriceList) error {
	if p.ReferenceID == nil {
		return nil
	}
	ref, err := s.Store.GetPriceList(ctx, *p.ReferenceID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"referencePriceListId": "Daftar harga acuan tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	return p.ValidateReference(ref)
}

func (s *Service) checkPartner(ctx context.Context, p domain.PriceList) error {
	if p.PartnerID == nil || s.Parties == nil {
		return nil
	}
	refs, err := s.Parties.Partners(ctx, []uuid.UUID{*p.PartnerID})
	if err != nil {
		return err
	}
	if _, ok := refs[*p.PartnerID]; !ok {
		return apperr.Validation(map[string]string{"partnerId": "Mitra tidak ditemukan"})
	}
	return nil
}

// CreatePriceList menambah daftar harga. Harga diisi lewat versi yang diajukan untuk approval.
func (s *Service) CreatePriceList(ctx context.Context, in domain.PriceListInput) (domain.PriceList, error) {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceList{}, err
	}
	p, err := domain.NewPriceList(in)
	if err != nil {
		return domain.PriceList{}, err
	}
	if err := s.checkReference(ctx, p); err != nil {
		return domain.PriceList{}, err
	}
	if err := s.checkPartner(ctx, p); err != nil {
		return domain.PriceList{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.InsertPriceList(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPriceList, EntityID: p.ID, EntityLabel: p.Code, After: priceListSnapshot(p),
		})
	})
	return p, err
}

// UpdatePriceList mengubah header daftar harga. Basis harga tidak bisa diubah setelah ada versi yang diajukan
// atau aktif.
func (s *Service) UpdatePriceList(ctx context.Context, id uuid.UUID, version int, in domain.PriceListInput) (domain.PriceList, error) {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceList{}, err
	}
	var p domain.PriceList
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetPriceList(ctx, id, true); err != nil {
			return err
		}
		if err := checkVersion(p.Version, version); err != nil {
			return err
		}
		before := p
		if err := p.Update(in); err != nil {
			return err
		}
		if p.Basis != before.Basis || !domain.SameID(p.ReferenceID, before.ReferenceID) {
			versions, err := s.Store.ListVersions(ctx, id)
			if err != nil {
				return err
			}
			if slices.ContainsFunc(versions, func(v domain.PriceListVersion) bool {
				return v.Status == domain.VersionActive || v.Status == domain.VersionPendingApproval
			}) {
				return apperr.Validation(map[string]string{"basis": "Basis harga dan acuan tidak bisa diubah setelah ada versi yang diajukan atau berlaku; buat daftar harga baru"})
			}
		}
		if err := s.checkReference(ctx, p); err != nil {
			return err
		}
		if err := s.checkPartner(ctx, p); err != nil {
			return err
		}
		if p, err = s.Store.UpdatePriceList(ctx, p, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPriceList, EntityID: p.ID, EntityLabel: p.Code,
			Before: priceListSnapshot(before), After: priceListSnapshot(p),
		})
	})
	return p, err
}

// DeletePriceList menghapus (soft delete) daftar harga. Transaksi lama tetap menyimpan harga dan versinya.
func (s *Service) DeletePriceList(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPriceList(ctx, id, true)
		if err != nil {
			return err
		}
		if err := checkVersion(p.Version, version); err != nil {
			return err
		}
		if msg := p.DeleteError(); msg != "" {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, msg)
		}
		if n, err := s.Store.CountReferences(ctx, id); err != nil {
			return err
		} else if n > 0 {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Daftar harga ini dipakai sebagai acuan daftar harga persentase; ubah acuannya dulu.")
		}
		versions, err := s.Store.ListVersions(ctx, id)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(versions, func(v domain.PriceListVersion) bool { return v.Status == domain.VersionPendingApproval }) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Masih ada versi yang menunggu persetujuan; tarik kembali pengajuannya dulu.")
		}
		if err := p.Delete(s.now(), a.UserID); err != nil {
			return err
		}
		if _, err := s.Store.SetPriceListDeleted(ctx, p, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityPriceList, EntityID: p.ID, EntityLabel: p.Code,
			After: map[string]any{"deletedAt": p.DeletedAt},
		})
	})
}

// RestorePriceList memulihkan daftar harga terhapus (Admin Sistem).
func (s *Service) RestorePriceList(ctx context.Context, id uuid.UUID, version int) (domain.PriceList, error) {
	a, err := softdelete.RequireRestore(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceList{}, err
	}
	var p domain.PriceList
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetPriceList(ctx, id, true); err != nil {
			return err
		}
		if err := checkVersion(p.Version, version); err != nil {
			return err
		}
		before := p.DeletedAt
		if err := p.Restore(); err != nil {
			return err
		}
		if p, err = s.Store.SetPriceListDeleted(ctx, p, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionRestore, EntityType: EntityPriceList, EntityID: p.ID, EntityLabel: p.Code,
			Before: map[string]any{"deletedAt": before}, After: map[string]any{"deletedAt": nil},
		})
	})
	return p, err
}

// --- Versi -----------------------------------------------------------------------------------

// VersionDetail adalah satu versi beserta item, produk, dan daftar harganya.
type VersionDetail struct {
	domain.PriceListVersion
	PriceList      domain.PriceList
	Products       map[uuid.UUID]ProductRef
	AllowedActions []string
}

func versionSnapshot(v domain.PriceListVersion) map[string]any {
	items := make([]map[string]any, len(v.Items))
	for i, it := range v.Items {
		het := ""
		if it.MaxRetailPrice != nil {
			het = it.MaxRetailPrice.String()
		}
		items[i] = map[string]any{"productId": it.ProductID.String(), "priceAmount": it.Price.String(), "maxRetailPriceAmount": het}
	}
	return map[string]any{
		"versionNo": v.No, "validFrom": v.ValidFrom.String(), "validTo": dateText(v.ValidTo), "status": v.Status,
		"percent": decimalText(v.Percent), "notes": v.Notes, "items": items,
	}
}

// GetVersion mengembalikan detail versi.
func (s *Service) GetVersion(ctx context.Context, id uuid.UUID) (VersionDetail, error) {
	a, err := readPriceList(ctx)
	if err != nil {
		return VersionDetail{}, err
	}
	v, err := s.Store.GetVersion(ctx, id, false)
	if err != nil {
		return VersionDetail{}, err
	}
	l, err := s.Store.GetPriceList(ctx, v.PriceListID, false)
	if err != nil {
		return VersionDetail{}, err
	}
	ids := make([]uuid.UUID, len(v.Items))
	for i, it := range v.Items {
		ids[i] = it.ProductID
	}
	products, err := s.products(ctx, ids)
	if err != nil {
		return VersionDetail{}, err
	}
	return VersionDetail{PriceListVersion: v, PriceList: l, Products: products, AllowedActions: VersionActions(a, v)}, nil
}

func itemField(i int) string { return "items." + itoa(i) + ".productId" }

func (s *Service) checkVersionProducts(ctx context.Context, v domain.PriceListVersion) error {
	ids := make([]uuid.UUID, len(v.Items))
	for i, it := range v.Items {
		ids[i] = it.ProductID
	}
	return s.checkProducts(ctx, ids, itemField)
}

// CreateVersion menyiapkan versi draft baru. copyFrom (opsional) menyalin harga dari versi lain bila in.Items
// kosong, sehingga perubahan harga cukup mengubah baris yang berbeda.
func (s *Service) CreateVersion(ctx context.Context, priceListID uuid.UUID, in domain.VersionInput, copyFrom *uuid.UUID) (domain.PriceListVersion, error) {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceListVersion{}, err
	}
	var v domain.PriceListVersion
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.Store.GetPriceList(ctx, priceListID, true)
		if err != nil {
			return err
		}
		if copyFrom != nil && len(in.Items) == 0 {
			src, err := s.Store.GetVersion(ctx, *copyFrom, false)
			if err != nil {
				return err
			}
			if src.PriceListID != l.ID {
				return apperr.Validation(map[string]string{"copyFromVersionId": "Versi sumber bukan milik daftar harga ini"})
			}
			for _, it := range src.Items {
				item := domain.ItemInput{ProductID: it.ProductID, Price: it.Price.String()}
				if it.MaxRetailPrice != nil {
					item.MaxRetailPrice = it.MaxRetailPrice.String()
				}
				in.Items = append(in.Items, item)
			}
			if in.Percent == "" {
				in.Percent = decimalText(src.Percent)
			}
		}
		no, err := s.Store.NextVersionNo(ctx, l.ID)
		if err != nil {
			return err
		}
		if v, err = domain.NewVersion(l, no, in, s.today()); err != nil {
			return err
		}
		if err := s.checkVersionProducts(ctx, v); err != nil {
			return err
		}
		if v, err = s.Store.InsertVersion(ctx, v, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPriceListVersion, EntityID: v.ID, EntityLabel: v.Label(l),
			After: versionSnapshot(v),
		})
	})
	return v, err
}

// UpdateVersion mengubah versi draft (tanggal berlaku, persen, catatan, dan seluruh item).
func (s *Service) UpdateVersion(ctx context.Context, id uuid.UUID, version int, in domain.VersionInput) (domain.PriceListVersion, error) {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceListVersion{}, err
	}
	var v domain.PriceListVersion
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if v, err = s.Store.GetVersion(ctx, id, true); err != nil {
			return err
		}
		if err := checkVersion(v.Version, version); err != nil {
			return err
		}
		l, err := s.Store.GetPriceList(ctx, v.PriceListID, false)
		if err != nil {
			return err
		}
		before := v
		if err := v.Update(l, in, s.today()); err != nil {
			return err
		}
		if err := s.checkVersionProducts(ctx, v); err != nil {
			return err
		}
		if v, err = s.Store.UpdateVersion(ctx, v, version, a.UserID, true); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPriceListVersion, EntityID: v.ID, EntityLabel: v.Label(l),
			Before: versionSnapshot(before), After: versionSnapshot(v),
		})
	})
	return v, err
}

// CancelVersion membatalkan versi draft.
func (s *Service) CancelVersion(ctx context.Context, id uuid.UUID, version int) (domain.PriceListVersion, error) {
	a, err := actor.Require(ctx, PermPriceListWrite)
	if err != nil {
		return domain.PriceListVersion{}, err
	}
	return s.transition(ctx, id, version, a, audit.ActionCancel, func(v *domain.PriceListVersion, _ domain.PriceList) error {
		return v.Cancel()
	})
}

// SubmitVersion mengajukan versi draft untuk approval price_list_version (Head Sales → Head Finance &
// Accounting). Versi berlaku setelah disetujui level terakhir.
func (s *Service) SubmitVersion(ctx context.Context, id uuid.UUID, version int) (domain.PriceListVersion, error) {
	a, err := actor.Require(ctx, PermPriceListSubmit)
	if err != nil {
		return domain.PriceListVersion{}, err
	}
	if s.Approvals == nil {
		return domain.PriceListVersion{}, apperr.WithMessage(apperr.CodeApprovalPolicyMissing, "Modul approval belum aktif.")
	}
	return s.transition(ctx, id, version, a, audit.ActionSubmit, func(v *domain.PriceListVersion, l domain.PriceList) error {
		if err := v.Submit(l, s.today(), s.now(), a.UserID); err != nil {
			return err
		}
		if err := s.checkVersionProducts(ctx, *v); err != nil {
			return err
		}
		_, err := s.Approvals.Request(ctx, approval.Subject{
			Type: approval.SubjectPriceListVersion, ID: v.ID, Label: v.Label(l), Link: "/master/price-lists/" + l.ID.String(),
			Summary: v.Summary(l),
		})
		return err
	})
}

// WithdrawVersion menarik kembali pengajuan menjadi draft dan membatalkan permintaan approval.
func (s *Service) WithdrawVersion(ctx context.Context, id uuid.UUID, version int) (domain.PriceListVersion, error) {
	a, err := actor.Require(ctx, PermPriceListSubmit)
	if err != nil {
		return domain.PriceListVersion{}, err
	}
	return s.transition(ctx, id, version, a, audit.ActionStatusChange, func(v *domain.PriceListVersion, _ domain.PriceList) error {
		if err := v.Withdraw(); err != nil {
			return err
		}
		if s.Approvals == nil {
			return nil
		}
		return s.Approvals.Cancel(ctx, approval.SubjectPriceListVersion, v.ID)
	})
}

func (s *Service) transition(ctx context.Context, id uuid.UUID, version int, a actor.Actor, action audit.Action,
	apply func(v *domain.PriceListVersion, l domain.PriceList) error,
) (domain.PriceListVersion, error) {
	var v domain.PriceListVersion
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if v, err = s.Store.GetVersion(ctx, id, true); err != nil {
			return err
		}
		if err := checkVersion(v.Version, version); err != nil {
			return err
		}
		l, err := s.Store.GetPriceList(ctx, v.PriceListID, false)
		if err != nil {
			return err
		}
		before := v
		if err := apply(&v, l); err != nil {
			return err
		}
		if v, err = s.Store.UpdateVersion(ctx, v, version, a.UserID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPriceListVersion, EntityID: v.ID, EntityLabel: v.Label(l),
			Before: map[string]any{"status": before.Status}, After: map[string]any{"status": v.Status},
		})
	})
	return v, err
}

// OnVersionApproved dipanggil engine approval (di transaksi keputusan) saat level terakhir menyetujui: versi
// diberlakukan dan versi aktif sebelumnya yang tanpa tanggal berakhir ditutup sehari sebelum versi baru mulai.
func (s *Service) OnVersionApproved(ctx context.Context, d approval.Decided) error {
	v, err := s.Store.GetVersion(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	l, err := s.Store.GetPriceList(ctx, v.PriceListID, true)
	if err != nil {
		return err
	}
	active, err := s.Store.ActiveVersionsForUpdate(ctx, l.ID)
	if err != nil {
		return err
	}
	cur := v.Version
	if err := v.Activate(s.today(), s.now(), d.DecidedBy); err != nil {
		return err
	}
	closed, err := v.SupersedeActive(active)
	if err != nil {
		return err
	}
	for _, c := range closed {
		if _, err := s.Store.UpdateVersion(ctx, c, c.Version, d.DecidedBy, false); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPriceListVersion, EntityID: c.ID, EntityLabel: c.Label(l),
			After: map[string]any{"validTo": dateText(c.ValidTo)}, Note: "Ditutup oleh versi v" + itoa(v.No),
		}); err != nil {
			return err
		}
	}
	if v, err = s.Store.UpdateVersion(ctx, v, cur, d.DecidedBy, false); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionApprove, EntityType: EntityPriceListVersion, EntityID: v.ID, EntityLabel: v.Label(l),
		After: map[string]any{"status": v.Status, "validFrom": v.ValidFrom.String()}, Note: d.Note,
	})
}

// OnVersionRejected dipanggil engine approval saat pengajuan ditolak.
func (s *Service) OnVersionRejected(ctx context.Context, d approval.Decided) error {
	v, err := s.Store.GetVersion(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	l, err := s.Store.GetPriceList(ctx, v.PriceListID, false)
	if err != nil {
		return err
	}
	cur := v.Version
	if err := v.Reject(s.now(), d.DecidedBy); err != nil {
		return err
	}
	if v, err = s.Store.UpdateVersion(ctx, v, cur, d.DecidedBy, false); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionReject, EntityType: EntityPriceListVersion, EntityID: v.ID, EntityLabel: v.Label(l),
		After: map[string]any{"status": v.Status}, Note: d.Note,
	})
}

// --- Sumber data resolver --------------------------------------------------------------------

// PriceListByID membaca daftar harga untuk resolver (tanpa cek permission).
func (s *Service) PriceListByID(ctx context.Context, id uuid.UUID) (domain.PriceList, error) {
	return s.Store.GetPriceList(ctx, id, false)
}

// BasePriceList membaca daftar Harga Dasar & HET.
func (s *Service) BasePriceList(ctx context.Context) (domain.PriceList, error) {
	return s.Store.BasePriceList(ctx)
}

// ScopePriceLists membaca daftar harga aktif berjenis karyawan, mitra, dan channel/sub-channel.
func (s *Service) ScopePriceLists(ctx context.Context, partnerID, channelID, subChannelID *uuid.UUID) ([]domain.PriceList, error) {
	return s.Store.ScopePriceLists(ctx, partnerID, channelID, subChannelID)
}

// ListPriceOn membaca harga produk dari versi daftar harga yang berlaku pada tanggal. Daftar basis percent
// dihitung dari harga daftar acuan pada tanggal yang sama. ok=false bila produk tidak punya harga di daftar itu.
func (s *Service) ListPriceOn(ctx context.Context, l domain.PriceList, productID uuid.UUID, on clock.BusinessDate) (domain.ListPrice, bool, error) {
	vp, ok, err := s.Store.ListPriceOn(ctx, l.ID, productID, on)
	if err != nil || !ok {
		return domain.ListPrice{}, false, err
	}
	out := domain.ListPrice{PriceListID: l.ID, VersionID: vp.VersionID, Code: l.Code, Name: l.Name, MaxRetailPrice: vp.MaxRetailPrice}
	if l.Basis != domain.BasisPercent {
		if vp.Price == nil {
			return domain.ListPrice{}, false, nil
		}
		out.Price = *vp.Price
		return out, true, nil
	}
	if vp.Percent == nil || l.ReferenceID == nil {
		return domain.ListPrice{}, false, nil
	}
	ref, err := s.Store.GetPriceList(ctx, *l.ReferenceID, false)
	if err != nil {
		return domain.ListPrice{}, false, err
	}
	if ref.Deleted() || ref.Basis != domain.BasisFixed {
		return domain.ListPrice{}, false, nil
	}
	rp, ok, err := s.Store.ListPriceOn(ctx, ref.ID, productID, on)
	if err != nil || !ok || rp.Price == nil {
		return domain.ListPrice{}, false, err
	}
	out.Price = domain.PercentOf(*rp.Price, *vp.Percent)
	out.MaxRetailPrice = nil
	return out, true, nil
}

// SpecialPriceOn membaca harga khusus yang berlaku.
func (s *Service) SpecialPriceOn(ctx context.Context, t domain.PartyType, partyID, productID uuid.UUID, on clock.BusinessDate) (domain.SpecialPrice, bool, error) {
	return s.Store.SpecialPriceOn(ctx, t, partyID, productID, on)
}

// PromotionOffersOn membaca item promo yang berlaku.
func (s *Service) PromotionOffersOn(ctx context.Context, productID uuid.UUID, channelID, subChannelID *uuid.UUID, on clock.BusinessDate) ([]domain.PromotionOffer, error) {
	return s.Store.PromotionOffersOn(ctx, productID, channelID, subChannelID, on)
}
