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

// --- Harga khusus ----------------------------------------------------------------------------

func toSpecialPrice(r sqlcgen.SpecialPrice) domain.SpecialPrice {
	return domain.SpecialPrice{
		ID: r.ID, PartyType: domain.PartyType(r.PartyType), PartyID: r.PartyID, ProductID: r.ProductID,
		Price: money.FromDecimal(r.PriceAmount), Period: domain.Period{ValidFrom: r.ValidFrom, ValidTo: fromPgDate(r.ValidTo)},
		Notes: r.Notes, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version), Mark: mark(r.DeletedAt, r.DeletedBy),
	}
}

func toSpecialPrices(rows []sqlcgen.SpecialPrice) []domain.SpecialPrice {
	out := make([]domain.SpecialPrice, len(rows))
	for i, r := range rows {
		out[i] = toSpecialPrice(r)
	}
	return out
}

// SpecialPriceFilter adalah filter daftar harga khusus.
type SpecialPriceFilter struct {
	PartyType    domain.PartyType
	PartyID      *uuid.UUID
	ProductID    *uuid.UUID
	IncludeEnded bool
	Today        clock.BusinessDate
}

// ListSpecialPrices mengembalikan harga khusus yang belum dihapus.
func (s *Store) ListSpecialPrices(ctx context.Context, f SpecialPriceFilter) ([]domain.SpecialPrice, error) {
	rows, err := s.q(ctx).ListSpecialPrices(ctx, sqlcgen.ListSpecialPricesParams{
		PartyType: text(string(f.PartyType)), PartyID: nullUUID(f.PartyID), ProductID: nullUUID(f.ProductID),
		IncludeEnded: f.IncludeEnded, Today: f.Today,
	})
	if err != nil {
		return nil, fmt.Errorf("list special prices: %w", err)
	}
	return toSpecialPrices(rows), nil
}

// GetSpecialPrice membaca harga khusus; forUpdate mengunci baris.
func (s *Store) GetSpecialPrice(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.SpecialPrice, error) {
	var (
		r   sqlcgen.SpecialPrice
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetSpecialPriceForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetSpecialPrice(ctx, id)
	}
	if err != nil {
		return domain.SpecialPrice{}, notFound(err, "get special price")
	}
	return toSpecialPrice(r), nil
}

// OverlappingSpecialPrices mengunci harga khusus pihak & produk yang sama yang beririsan dengan periode.
func (s *Store) OverlappingSpecialPrices(ctx context.Context, sp domain.SpecialPrice) ([]domain.SpecialPrice, error) {
	rows, err := s.q(ctx).OverlappingSpecialPrices(ctx, sqlcgen.OverlappingSpecialPricesParams{
		PartyType: string(sp.PartyType), PartyID: sp.PartyID, ProductID: sp.ProductID, ValidFrom: sp.ValidFrom,
		ValidTo: toPgDate(sp.ValidTo),
	})
	if err != nil {
		return nil, fmt.Errorf("overlapping special prices: %w", err)
	}
	return toSpecialPrices(rows), nil
}

// InsertSpecialPrice menyimpan harga khusus.
func (s *Store) InsertSpecialPrice(ctx context.Context, sp domain.SpecialPrice, actorID uuid.UUID) (domain.SpecialPrice, error) {
	r, err := s.q(ctx).InsertSpecialPrice(ctx, sqlcgen.InsertSpecialPriceParams{
		PartyType: string(sp.PartyType), PartyID: sp.PartyID, ProductID: sp.ProductID, PriceAmount: sp.Price.Decimal(),
		ValidFrom: sp.ValidFrom, ValidTo: toPgDate(sp.ValidTo), Notes: sp.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.SpecialPrice{}, mapWriteError(err, "insert special price")
	}
	return toSpecialPrice(r), nil
}

// UpdateSpecialPriceEnd menyimpan tanggal berakhir baru.
func (s *Store) UpdateSpecialPriceEnd(ctx context.Context, sp domain.SpecialPrice, version int, actorID uuid.UUID) (domain.SpecialPrice, error) {
	r, err := s.q(ctx).UpdateSpecialPriceEnd(ctx, sqlcgen.UpdateSpecialPriceEndParams{
		ValidTo: toPgDate(sp.ValidTo), ActorID: actorID, ID: sp.ID, Version: version32(version),
	})
	if err != nil {
		return domain.SpecialPrice{}, versionConflict(err, "end special price")
	}
	return toSpecialPrice(r), nil
}

// DeleteSpecialPrice melakukan soft delete.
func (s *Store) DeleteSpecialPrice(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).DeleteSpecialPrice(ctx, sqlcgen.DeleteSpecialPriceParams{ActorID: actorID, ID: id, Version: version32(version)})
	return rowsConflict(n, err, "delete special price")
}

// SpecialPriceOn membaca harga khusus yang berlaku; ok=false bila tidak ada.
func (s *Store) SpecialPriceOn(ctx context.Context, partyType domain.PartyType, partyID, productID uuid.UUID, on clock.BusinessDate) (domain.SpecialPrice, bool, error) {
	r, err := s.q(ctx).SpecialPriceOn(ctx, sqlcgen.SpecialPriceOnParams{
		PartyType: string(partyType), PartyID: partyID, ProductID: productID, OnDate: on,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SpecialPrice{}, false, nil
	}
	if err != nil {
		return domain.SpecialPrice{}, false, fmt.Errorf("special price on: %w", err)
	}
	return toSpecialPrice(r), true, nil
}

// --- Promo -----------------------------------------------------------------------------------

func toPromotion(r sqlcgen.Promotion) domain.Promotion {
	return domain.Promotion{
		ID: r.ID, Code: r.Code, Name: r.Name, ChannelID: uuidPtr(r.ChannelID), SubChannelID: uuidPtr(r.SubChannelID),
		ValidFrom: r.ValidFrom, ValidTo: r.ValidTo, IsActive: r.IsActive, Description: r.Description,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version), Mark: mark(r.DeletedAt, r.DeletedBy),
	}
}

// PromotionFilter adalah filter daftar promo.
type PromotionFilter struct {
	ChannelID    *uuid.UUID
	Query        string
	IncludeEnded bool
	Deleted      bool
	Today        clock.BusinessDate
}

// ListPromotions mengembalikan promo beserta jumlah produknya (tanpa item).
func (s *Store) ListPromotions(ctx context.Context, f PromotionFilter) ([]domain.Promotion, error) {
	rows, err := s.q(ctx).ListPromotions(ctx, sqlcgen.ListPromotionsParams{
		Deleted: f.Deleted, IncludeEnded: f.IncludeEnded, Today: f.Today, ChannelID: nullUUID(f.ChannelID), Q: text(f.Query),
	})
	if err != nil {
		return nil, fmt.Errorf("list promotions: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	counts := map[uuid.UUID]int{}
	if len(ids) > 0 {
		cs, err := s.q(ctx).CountPromotionItems(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("count promotion items: %w", err)
		}
		for _, c := range cs {
			counts[c.PromotionID] = int(c.ItemCount)
		}
	}
	out := make([]domain.Promotion, len(rows))
	for i, r := range rows {
		out[i] = toPromotion(r)
		out[i].ItemCount = counts[r.ID]
	}
	return out, nil
}

// GetPromotion membaca promo beserta itemnya; forUpdate mengunci baris.
func (s *Store) GetPromotion(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Promotion, error) {
	var (
		r   sqlcgen.Promotion
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPromotionForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPromotion(ctx, id)
	}
	if err != nil {
		return domain.Promotion{}, notFound(err, "get promotion")
	}
	items, err := s.q(ctx).ListPromotionItems(ctx, id)
	if err != nil {
		return domain.Promotion{}, fmt.Errorf("list promotion items: %w", err)
	}
	p := toPromotion(r)
	p.Items = make([]domain.PromotionItem, len(items))
	for i, it := range items {
		p.Items[i] = domain.PromotionItem{
			ProductID: it.ProductID, Price: amountPtr(it.PriceAmount), DiscountPercent: decimalPtr(it.DiscountPercent),
		}
	}
	p.ItemCount = len(items)
	return p, nil
}

func (s *Store) insertPromotionItems(ctx context.Context, promotionID uuid.UUID, items []domain.PromotionItem, actorID uuid.UUID) error {
	ids := make([]uuid.UUID, len(items))
	prices := make([]string, len(items))
	percents := make([]string, len(items))
	for i, it := range items {
		ids[i], prices[i] = it.ProductID, amountText(it.Price)
		if it.DiscountPercent != nil {
			percents[i] = it.DiscountPercent.String()
		}
	}
	err := s.q(ctx).InsertPromotionItems(ctx, sqlcgen.InsertPromotionItemsParams{
		PromotionID: promotionID, ActorID: actorID, ProductIds: ids, Prices: prices, DiscountPercents: percents,
	})
	if err != nil {
		return mapWriteError(err, "insert promotion items")
	}
	return nil
}

// InsertPromotion menyimpan promo beserta itemnya.
func (s *Store) InsertPromotion(ctx context.Context, p domain.Promotion, actorID uuid.UUID) (domain.Promotion, error) {
	r, err := s.q(ctx).InsertPromotion(ctx, sqlcgen.InsertPromotionParams{
		Code: p.Code, Name: p.Name, ChannelID: nullUUID(p.ChannelID), SubChannelID: nullUUID(p.SubChannelID),
		ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, IsActive: p.IsActive, Description: p.Description, ActorID: actorID,
	})
	if err != nil {
		return domain.Promotion{}, mapWriteError(err, "insert promotion")
	}
	if err := s.insertPromotionItems(ctx, r.ID, p.Items, actorID); err != nil {
		return domain.Promotion{}, err
	}
	out := toPromotion(r)
	out.Items, out.ItemCount = p.Items, len(p.Items)
	return out, nil
}

// UpdatePromotion menyimpan perubahan promo; replaceItems mengganti seluruh item.
func (s *Store) UpdatePromotion(ctx context.Context, p domain.Promotion, version int, actorID uuid.UUID, replaceItems bool) (domain.Promotion, error) {
	r, err := s.q(ctx).UpdatePromotion(ctx, sqlcgen.UpdatePromotionParams{
		Name: p.Name, ChannelID: nullUUID(p.ChannelID), SubChannelID: nullUUID(p.SubChannelID), ValidFrom: p.ValidFrom,
		ValidTo: p.ValidTo, IsActive: p.IsActive, Description: p.Description, ActorID: actorID, ID: p.ID,
		Version: version32(version),
	})
	if err != nil {
		return domain.Promotion{}, versionConflict(err, "update promotion")
	}
	if replaceItems {
		if err := s.q(ctx).DeletePromotionItems(ctx, p.ID); err != nil {
			return domain.Promotion{}, fmt.Errorf("delete promotion items: %w", err)
		}
		if err := s.insertPromotionItems(ctx, p.ID, p.Items, actorID); err != nil {
			return domain.Promotion{}, err
		}
	}
	out := toPromotion(r)
	out.Items, out.ItemCount = p.Items, len(p.Items)
	return out, nil
}

// DeletePromotion melakukan soft delete.
func (s *Store) DeletePromotion(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).DeletePromotion(ctx, sqlcgen.DeletePromotionParams{ActorID: actorID, ID: id, Version: version32(version)})
	return rowsConflict(n, err, "delete promotion")
}

// PromotionOffersOn mengembalikan item promo aktif untuk produk, channel, dan tanggal.
func (s *Store) PromotionOffersOn(ctx context.Context, productID uuid.UUID, channelID, subChannelID *uuid.UUID, on clock.BusinessDate) ([]domain.PromotionOffer, error) {
	rows, err := s.q(ctx).PromotionOffersOn(ctx, sqlcgen.PromotionOffersOnParams{
		ProductID: productID, OnDate: on, ChannelID: nullUUID(channelID), SubChannelID: nullUUID(subChannelID),
	})
	if err != nil {
		return nil, fmt.Errorf("promotion offers on: %w", err)
	}
	out := make([]domain.PromotionOffer, len(rows))
	for i, r := range rows {
		out[i] = domain.PromotionOffer{PromotionID: r.PromotionID, Code: r.Code, Name: r.Name, Item: domain.PromotionItem{
			ProductID: productID, Price: amountPtr(r.PriceAmount), DiscountPercent: decimalPtr(r.DiscountPercent),
		}}
	}
	return out, nil
}

// --- Harga bertingkat ------------------------------------------------------------------------

func toTieredPrice(r sqlcgen.TieredPrice) domain.TieredPrice {
	return domain.TieredPrice{
		ID: r.ID, PriceListID: uuidPtr(r.PriceListID), ProductID: r.ProductID, MinQty: r.MinQty,
		Price: money.FromDecimal(r.PriceAmount), Period: domain.Period{ValidFrom: r.ValidFrom, ValidTo: fromPgDate(r.ValidTo)},
		Notes: r.Notes, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version), Mark: mark(r.DeletedAt, r.DeletedBy),
	}
}

func toTieredPrices(rows []sqlcgen.TieredPrice) []domain.TieredPrice {
	out := make([]domain.TieredPrice, len(rows))
	for i, r := range rows {
		out[i] = toTieredPrice(r)
	}
	return out
}

// TieredPriceFilter adalah filter daftar harga bertingkat.
type TieredPriceFilter struct {
	ProductID    *uuid.UUID
	PriceListID  *uuid.UUID
	IncludeEnded bool
	Today        clock.BusinessDate
}

// ListTieredPrices mengembalikan harga bertingkat yang belum dihapus.
func (s *Store) ListTieredPrices(ctx context.Context, f TieredPriceFilter) ([]domain.TieredPrice, error) {
	rows, err := s.q(ctx).ListTieredPrices(ctx, sqlcgen.ListTieredPricesParams{
		ProductID: nullUUID(f.ProductID), PriceListID: nullUUID(f.PriceListID), IncludeEnded: f.IncludeEnded, Today: f.Today,
	})
	if err != nil {
		return nil, fmt.Errorf("list tiered prices: %w", err)
	}
	return toTieredPrices(rows), nil
}

// GetTieredPrice membaca harga bertingkat; forUpdate mengunci baris.
func (s *Store) GetTieredPrice(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.TieredPrice, error) {
	var (
		r   sqlcgen.TieredPrice
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetTieredPriceForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetTieredPrice(ctx, id)
	}
	if err != nil {
		return domain.TieredPrice{}, notFound(err, "get tiered price")
	}
	return toTieredPrice(r), nil
}

// OverlappingTieredPrices mengunci harga bertingkat dengan daftar harga, produk, dan qty sama yang beririsan.
func (s *Store) OverlappingTieredPrices(ctx context.Context, t domain.TieredPrice) ([]domain.TieredPrice, error) {
	rows, err := s.q(ctx).OverlappingTieredPrices(ctx, sqlcgen.OverlappingTieredPricesParams{
		PriceListID: nullUUID(t.PriceListID), ProductID: t.ProductID, MinQty: t.MinQty, ValidFrom: t.ValidFrom,
		ValidTo: toPgDate(t.ValidTo),
	})
	if err != nil {
		return nil, fmt.Errorf("overlapping tiered prices: %w", err)
	}
	return toTieredPrices(rows), nil
}

// InsertTieredPrice menyimpan harga bertingkat.
func (s *Store) InsertTieredPrice(ctx context.Context, t domain.TieredPrice, actorID uuid.UUID) (domain.TieredPrice, error) {
	r, err := s.q(ctx).InsertTieredPrice(ctx, sqlcgen.InsertTieredPriceParams{
		PriceListID: nullUUID(t.PriceListID), ProductID: t.ProductID, MinQty: t.MinQty, PriceAmount: t.Price.Decimal(),
		ValidFrom: t.ValidFrom, ValidTo: toPgDate(t.ValidTo), Notes: t.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.TieredPrice{}, mapWriteError(err, "insert tiered price")
	}
	return toTieredPrice(r), nil
}

// UpdateTieredPriceEnd menyimpan tanggal berakhir baru.
func (s *Store) UpdateTieredPriceEnd(ctx context.Context, t domain.TieredPrice, version int, actorID uuid.UUID) (domain.TieredPrice, error) {
	r, err := s.q(ctx).UpdateTieredPriceEnd(ctx, sqlcgen.UpdateTieredPriceEndParams{
		ValidTo: toPgDate(t.ValidTo), ActorID: actorID, ID: t.ID, Version: version32(version),
	})
	if err != nil {
		return domain.TieredPrice{}, versionConflict(err, "end tiered price")
	}
	return toTieredPrice(r), nil
}

// DeleteTieredPrice melakukan soft delete.
func (s *Store) DeleteTieredPrice(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).DeleteTieredPrice(ctx, sqlcgen.DeleteTieredPriceParams{ActorID: actorID, ID: id, Version: version32(version)})
	return rowsConflict(n, err, "delete tiered price")
}

// TieredPricesOn mengembalikan harga bertingkat berlaku (qty minimum ≤ qty) untuk daftar harga kandidat dan
// harga bertingkat umum, urut qty minimum terbesar.
func (s *Store) TieredPricesOn(ctx context.Context, productID uuid.UUID, priceListIDs []uuid.UUID, qty decimal.Decimal, on clock.BusinessDate) ([]domain.TieredPrice, error) {
	if priceListIDs == nil {
		priceListIDs = []uuid.UUID{}
	}
	rows, err := s.q(ctx).TieredPricesOn(ctx, sqlcgen.TieredPricesOnParams{
		ProductID: productID, Qty: qty, PriceListIds: priceListIDs, OnDate: on,
	})
	if err != nil {
		return nil, fmt.Errorf("tiered prices on: %w", err)
	}
	return toTieredPrices(rows), nil
}

// --- Batas diskon ----------------------------------------------------------------------------

func toDiscountLimit(r sqlcgen.DiscountLimit) domain.DiscountLimit {
	return domain.DiscountLimit{
		ID: r.ID, RoleCode: r.RoleCode, MaxLinePercent: r.MaxLineDiscountPercent, MaxDocumentPercent: r.MaxDocumentDiscountPercent,
		Period: domain.Period{ValidFrom: r.ValidFrom, ValidTo: fromPgDate(r.ValidTo)}, Notes: r.Notes,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version), Mark: mark(r.DeletedAt, r.DeletedBy),
	}
}

func toDiscountLimits(rows []sqlcgen.DiscountLimit) []domain.DiscountLimit {
	out := make([]domain.DiscountLimit, len(rows))
	for i, r := range rows {
		out[i] = toDiscountLimit(r)
	}
	return out
}

// ListDiscountLimits mengembalikan batas diskon; includeEnded menampilkan riwayat yang sudah berakhir.
func (s *Store) ListDiscountLimits(ctx context.Context, includeEnded bool, today clock.BusinessDate) ([]domain.DiscountLimit, error) {
	rows, err := s.q(ctx).ListDiscountLimits(ctx, sqlcgen.ListDiscountLimitsParams{IncludeEnded: includeEnded, Today: today})
	if err != nil {
		return nil, fmt.Errorf("list discount limits: %w", err)
	}
	return toDiscountLimits(rows), nil
}

// GetDiscountLimitForUpdate membaca dan mengunci batas diskon.
func (s *Store) GetDiscountLimitForUpdate(ctx context.Context, id uuid.UUID) (domain.DiscountLimit, error) {
	r, err := s.q(ctx).GetDiscountLimitForUpdate(ctx, id)
	if err != nil {
		return domain.DiscountLimit{}, notFound(err, "get discount limit")
	}
	return toDiscountLimit(r), nil
}

// OverlappingDiscountLimits mengunci batas diskon role yang beririsan dengan periode.
func (s *Store) OverlappingDiscountLimits(ctx context.Context, l domain.DiscountLimit) ([]domain.DiscountLimit, error) {
	rows, err := s.q(ctx).OverlappingDiscountLimits(ctx, sqlcgen.OverlappingDiscountLimitsParams{
		RoleCode: l.RoleCode, ValidFrom: l.ValidFrom, ValidTo: toPgDate(l.ValidTo),
	})
	if err != nil {
		return nil, fmt.Errorf("overlapping discount limits: %w", err)
	}
	return toDiscountLimits(rows), nil
}

// InsertDiscountLimit menyimpan batas diskon.
func (s *Store) InsertDiscountLimit(ctx context.Context, l domain.DiscountLimit, actorID uuid.UUID) (domain.DiscountLimit, error) {
	r, err := s.q(ctx).InsertDiscountLimit(ctx, sqlcgen.InsertDiscountLimitParams{
		RoleCode: l.RoleCode, MaxLineDiscountPercent: l.MaxLinePercent, MaxDocumentDiscountPercent: l.MaxDocumentPercent,
		ValidFrom: l.ValidFrom, ValidTo: toPgDate(l.ValidTo), Notes: l.Notes, ActorID: actorID,
	})
	if err != nil {
		return domain.DiscountLimit{}, mapWriteError(err, "insert discount limit")
	}
	return toDiscountLimit(r), nil
}

// UpdateDiscountLimitEnd menyimpan tanggal berakhir baru.
func (s *Store) UpdateDiscountLimitEnd(ctx context.Context, l domain.DiscountLimit, version int, actorID uuid.UUID) (domain.DiscountLimit, error) {
	r, err := s.q(ctx).UpdateDiscountLimitEnd(ctx, sqlcgen.UpdateDiscountLimitEndParams{
		ValidTo: toPgDate(l.ValidTo), ActorID: actorID, ID: l.ID, Version: version32(version),
	})
	if err != nil {
		return domain.DiscountLimit{}, versionConflict(err, "end discount limit")
	}
	return toDiscountLimit(r), nil
}

// DeleteDiscountLimit melakukan soft delete.
func (s *Store) DeleteDiscountLimit(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).DeleteDiscountLimit(ctx, sqlcgen.DeleteDiscountLimitParams{ActorID: actorID, ID: id, Version: version32(version)})
	return rowsConflict(n, err, "delete discount limit")
}

// DiscountLimitsOn mengembalikan batas diskon role-role yang berlaku pada tanggal.
func (s *Store) DiscountLimitsOn(ctx context.Context, roleCodes []string, on clock.BusinessDate) ([]domain.DiscountLimit, error) {
	if roleCodes == nil {
		roleCodes = []string{}
	}
	rows, err := s.q(ctx).DiscountLimitsOn(ctx, sqlcgen.DiscountLimitsOnParams{RoleCodes: roleCodes, OnDate: on})
	if err != nil {
		return nil, fmt.Errorf("discount limits on: %w", err)
	}
	return toDiscountLimits(rows), nil
}
