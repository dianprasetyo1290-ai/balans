package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres/sqlcgen"
)

func toPriceList(r sqlcgen.PriceList) domain.PriceList {
	return domain.PriceList{
		ID: r.ID, Code: r.Code, Name: r.Name, Type: domain.PriceListType(r.PriceListType),
		ChannelID: uuidPtr(r.ChannelID), SubChannelID: uuidPtr(r.SubChannelID), PartnerID: uuidPtr(r.PartnerID),
		ResellerLevel: r.ResellerLevel, Basis: domain.Basis(r.Basis), ReferenceID: uuidPtr(r.ReferencePriceListID),
		Description: r.Description, IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version), Mark: mark(r.DeletedAt, r.DeletedBy),
	}
}

func toPriceLists(rows []sqlcgen.PriceList) []domain.PriceList {
	out := make([]domain.PriceList, len(rows))
	for i, r := range rows {
		out[i] = toPriceList(r)
	}
	return out
}

func toVersion(r sqlcgen.PriceListVersion, itemCount int) domain.PriceListVersion {
	return domain.PriceListVersion{
		ID: r.ID, PriceListID: r.PriceListID, No: int(r.VersionNo),
		Period: domain.Period{ValidFrom: r.ValidFrom, ValidTo: fromPgDate(r.ValidTo)},
		Status: domain.VersionStatus(r.Status), Percent: decimalPtr(r.Percent), Notes: r.Notes,
		SubmittedAt: r.SubmittedAt, SubmittedBy: uuidPtr(r.SubmittedBy), DecidedAt: r.DecidedAt, DecidedBy: uuidPtr(r.DecidedBy),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version), ItemCount: itemCount,
	}
}

// PriceListFilter adalah filter Data Master › Daftar Harga.
type PriceListFilter struct {
	Type            domain.PriceListType
	Query           string
	IncludeInactive bool
	Deleted         bool
}

// ListPriceLists mengembalikan daftar harga; Harga Dasar & HET selalu paling atas.
func (s *Store) ListPriceLists(ctx context.Context, f PriceListFilter) ([]domain.PriceList, error) {
	rows, err := s.q(ctx).ListPriceLists(ctx, sqlcgen.ListPriceListsParams{
		Deleted: f.Deleted, IncludeInactive: f.IncludeInactive, PriceListType: text(string(f.Type)), Q: text(f.Query),
	})
	if err != nil {
		return nil, fmt.Errorf("list price lists: %w", err)
	}
	return toPriceLists(rows), nil
}

// GetPriceList membaca daftar harga (termasuk yang terhapus); forUpdate mengunci baris.
func (s *Store) GetPriceList(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PriceList, error) {
	var (
		r   sqlcgen.PriceList
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPriceListForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPriceList(ctx, id)
	}
	if err != nil {
		return domain.PriceList{}, notFound(err, "get price list")
	}
	return toPriceList(r), nil
}

// PriceListsByIDs membaca beberapa daftar harga.
func (s *Store) PriceListsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.PriceList, error) {
	out := map[uuid.UUID]domain.PriceList{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).GetPriceListsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("price lists by ids: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = toPriceList(r)
	}
	return out, nil
}

// BasePriceList membaca daftar Harga Dasar & HET.
func (s *Store) BasePriceList(ctx context.Context) (domain.PriceList, error) {
	r, err := s.q(ctx).GetBasePriceList(ctx)
	if err != nil {
		return domain.PriceList{}, notFound(err, "get base price list")
	}
	return toPriceList(r), nil
}

// InsertPriceList menyimpan daftar harga baru.
func (s *Store) InsertPriceList(ctx context.Context, p domain.PriceList, actorID uuid.UUID) (domain.PriceList, error) {
	r, err := s.q(ctx).InsertPriceList(ctx, sqlcgen.InsertPriceListParams{
		Code: p.Code, Name: p.Name, PriceListType: string(p.Type), ChannelID: nullUUID(p.ChannelID),
		SubChannelID: nullUUID(p.SubChannelID), PartnerID: nullUUID(p.PartnerID), ResellerLevel: p.ResellerLevel,
		Basis: string(p.Basis), ReferencePriceListID: nullUUID(p.ReferenceID), Description: p.Description,
		IsActive: p.IsActive, ActorID: actorID,
	})
	if err != nil {
		return domain.PriceList{}, mapWriteError(err, "insert price list")
	}
	return toPriceList(r), nil
}

// UpdatePriceList menyimpan perubahan header dengan optimistic locking.
func (s *Store) UpdatePriceList(ctx context.Context, p domain.PriceList, version int, actorID uuid.UUID) (domain.PriceList, error) {
	r, err := s.q(ctx).UpdatePriceList(ctx, sqlcgen.UpdatePriceListParams{
		Name: p.Name, ChannelID: nullUUID(p.ChannelID), SubChannelID: nullUUID(p.SubChannelID),
		PartnerID: nullUUID(p.PartnerID), ResellerLevel: p.ResellerLevel, Basis: string(p.Basis),
		ReferencePriceListID: nullUUID(p.ReferenceID), Description: p.Description, IsActive: p.IsActive,
		ActorID: actorID, ID: p.ID, Version: version32(version),
	})
	if err != nil {
		return domain.PriceList{}, versionConflict(err, "update price list")
	}
	return toPriceList(r), nil
}

// SetPriceListDeleted menyimpan tanda hapus/pulihkan.
func (s *Store) SetPriceListDeleted(ctx context.Context, p domain.PriceList, version int, actorID uuid.UUID) (domain.PriceList, error) {
	r, err := s.q(ctx).SetPriceListDeleted(ctx, sqlcgen.SetPriceListDeletedParams{
		DeletedAt: p.DeletedAt, DeletedBy: nullUUID(p.DeletedBy), ActorID: actorID, ID: p.ID, Version: version32(version),
	})
	if err != nil {
		return domain.PriceList{}, versionConflict(err, "set price list deleted")
	}
	return toPriceList(r), nil
}

// CountReferences menghitung daftar harga basis percent yang memakai id sebagai acuan.
func (s *Store) CountReferences(ctx context.Context, id uuid.UUID) (int, error) {
	n, err := s.q(ctx).CountPriceListReferences(ctx, uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("count price list references: %w", err)
	}
	return int(n), nil
}

// ScopePriceLists mengembalikan daftar harga aktif berjenis karyawan, mitra (partnerID), dan channel/sub-channel.
func (s *Store) ScopePriceLists(ctx context.Context, partnerID, channelID, subChannelID *uuid.UUID) ([]domain.PriceList, error) {
	rows, err := s.q(ctx).FindScopePriceLists(ctx, sqlcgen.FindScopePriceListsParams{
		PartnerID: nullUUID(partnerID), ChannelID: nullUUID(channelID), SubChannelID: nullUUID(subChannelID),
	})
	if err != nil {
		return nil, fmt.Errorf("find scope price lists: %w", err)
	}
	return toPriceLists(rows), nil
}

// ListVersions mengembalikan riwayat versi (terbaru dulu) tanpa item.
func (s *Store) ListVersions(ctx context.Context, priceListID uuid.UUID) ([]domain.PriceListVersion, error) {
	rows, err := s.q(ctx).ListVersions(ctx, priceListID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	out := make([]domain.PriceListVersion, len(rows))
	for i, r := range rows {
		out[i] = toVersion(sqlcgen.PriceListVersion{
			ID: r.ID, PriceListID: r.PriceListID, VersionNo: r.VersionNo, ValidFrom: r.ValidFrom, ValidTo: r.ValidTo,
			Status: r.Status, Percent: r.Percent, Notes: r.Notes, SubmittedAt: r.SubmittedAt, SubmittedBy: r.SubmittedBy,
			DecidedAt: r.DecidedAt, DecidedBy: r.DecidedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
		}, int(r.ItemCount))
	}
	return out, nil
}

// CurrentVersions mengembalikan versi aktif, draft, dan yang diajukan per daftar harga.
func (s *Store) CurrentVersions(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.PriceListVersion, error) {
	out := map[uuid.UUID][]domain.PriceListVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).ListCurrentVersions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list current versions: %w", err)
	}
	for _, r := range rows {
		out[r.PriceListID] = append(out[r.PriceListID], toVersion(sqlcgen.PriceListVersion{
			ID: r.ID, PriceListID: r.PriceListID, VersionNo: r.VersionNo, ValidFrom: r.ValidFrom, ValidTo: r.ValidTo,
			Status: r.Status, Percent: r.Percent, Notes: r.Notes, SubmittedAt: r.SubmittedAt, SubmittedBy: r.SubmittedBy,
			DecidedAt: r.DecidedAt, DecidedBy: r.DecidedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
		}, int(r.ItemCount)))
	}
	return out, nil
}

// GetVersion membaca versi beserta itemnya; forUpdate mengunci baris versi.
func (s *Store) GetVersion(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PriceListVersion, error) {
	var (
		r   sqlcgen.PriceListVersion
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetVersionForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetVersion(ctx, id)
	}
	if err != nil {
		return domain.PriceListVersion{}, notFound(err, "get price list version")
	}
	items, err := s.q(ctx).ListVersionItems(ctx, id)
	if err != nil {
		return domain.PriceListVersion{}, fmt.Errorf("list version items: %w", err)
	}
	v := toVersion(r, len(items))
	v.Items = make([]domain.PriceListItem, len(items))
	for i, it := range items {
		v.Items[i] = domain.PriceListItem{
			ProductID: it.ProductID, Price: money.FromDecimal(it.PriceAmount), MaxRetailPrice: amountPtr(it.MaxRetailPriceAmount),
		}
	}
	return v, nil
}

// NextVersionNo mengembalikan nomor versi berikutnya.
func (s *Store) NextVersionNo(ctx context.Context, priceListID uuid.UUID) (int, error) {
	n, err := s.q(ctx).NextVersionNo(ctx, priceListID)
	if err != nil {
		return 0, fmt.Errorf("next version no: %w", err)
	}
	return int(n), nil
}

// ActiveVersionsForUpdate mengunci versi aktif daftar harga.
func (s *Store) ActiveVersionsForUpdate(ctx context.Context, priceListID uuid.UUID) ([]domain.PriceListVersion, error) {
	rows, err := s.q(ctx).ActiveVersionsForUpdate(ctx, priceListID)
	if err != nil {
		return nil, fmt.Errorf("active versions: %w", err)
	}
	out := make([]domain.PriceListVersion, len(rows))
	for i, r := range rows {
		out[i] = toVersion(r, 0)
	}
	return out, nil
}

func (s *Store) insertItems(ctx context.Context, versionID uuid.UUID, items []domain.PriceListItem, actorID uuid.UUID) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	prices := make([]string, len(items))
	het := make([]string, len(items))
	for i, it := range items {
		ids[i], prices[i], het[i] = it.ProductID, it.Price.String(), amountText(it.MaxRetailPrice)
	}
	err := s.q(ctx).InsertVersionItems(ctx, sqlcgen.InsertVersionItemsParams{
		VersionID: versionID, ActorID: actorID, ProductIds: ids, Prices: prices, MaxRetailPrices: het,
	})
	if err != nil {
		return mapWriteError(err, "insert version items")
	}
	return nil
}

// InsertVersion menyimpan versi baru beserta itemnya.
func (s *Store) InsertVersion(ctx context.Context, v domain.PriceListVersion, actorID uuid.UUID) (domain.PriceListVersion, error) {
	r, err := s.q(ctx).InsertVersion(ctx, sqlcgen.InsertVersionParams{
		PriceListID: v.PriceListID, VersionNo: int32(v.No), ValidFrom: v.ValidFrom, ValidTo: toPgDate(v.ValidTo), //nolint:gosec // nomor versi kecil
		Status: string(v.Status), Percent: nullDecimal(v.Percent), Notes: v.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.PriceListVersion{}, mapWriteError(err, "insert price list version")
	}
	if err := s.insertItems(ctx, r.ID, v.Items, actorID); err != nil {
		return domain.PriceListVersion{}, err
	}
	out := toVersion(r, len(v.Items))
	out.Items = v.Items
	return out, nil
}

// UpdateVersion menyimpan perubahan versi dengan optimistic locking; replaceItems mengganti seluruh item.
func (s *Store) UpdateVersion(ctx context.Context, v domain.PriceListVersion, version int, actorID uuid.UUID, replaceItems bool) (domain.PriceListVersion, error) {
	r, err := s.q(ctx).UpdateVersion(ctx, sqlcgen.UpdateVersionParams{
		ValidFrom: v.ValidFrom, ValidTo: toPgDate(v.ValidTo), Status: string(v.Status), Percent: nullDecimal(v.Percent),
		Notes: v.Notes, SubmittedAt: v.SubmittedAt, SubmittedBy: nullUUID(v.SubmittedBy), DecidedAt: v.DecidedAt,
		DecidedBy: nullUUID(v.DecidedBy), ActorID: actorID, ID: v.ID, Version: version32(version),
	})
	if err != nil {
		return domain.PriceListVersion{}, versionConflict(err, "update price list version")
	}
	if replaceItems {
		if err := s.q(ctx).DeleteVersionItems(ctx, v.ID); err != nil {
			return domain.PriceListVersion{}, fmt.Errorf("delete version items: %w", err)
		}
		if err := s.insertItems(ctx, v.ID, v.Items, actorID); err != nil {
			return domain.PriceListVersion{}, err
		}
	}
	out := toVersion(r, len(v.Items))
	out.Items = v.Items
	return out, nil
}

// VersionPrice adalah harga produk dari versi aktif satu daftar harga pada satu tanggal.
type VersionPrice struct {
	VersionID uuid.UUID
	// Percent terisi untuk daftar basis percent.
	Percent *decimal.Decimal
	// Price nil bila produk tidak ada di versi.
	Price          *money.Amount
	MaxRetailPrice *money.Amount
}

// ListPriceOn membaca harga produk dari versi aktif daftar harga pada tanggal; ok=false bila tidak ada versi aktif.
func (s *Store) ListPriceOn(ctx context.Context, priceListID, productID uuid.UUID, on clock.BusinessDate) (VersionPrice, bool, error) {
	r, err := s.q(ctx).ListPriceOn(ctx, sqlcgen.ListPriceOnParams{ProductID: productID, PriceListID: priceListID, OnDate: on})
	if errors.Is(err, pgx.ErrNoRows) {
		return VersionPrice{}, false, nil
	}
	if err != nil {
		return VersionPrice{}, false, fmt.Errorf("list price on: %w", err)
	}
	return VersionPrice{
		VersionID: r.VersionID, Percent: decimalPtr(r.Percent), Price: amountPtr(r.PriceAmount),
		MaxRetailPrice: amountPtr(r.MaxRetailPriceAmount),
	}, true, nil
}
