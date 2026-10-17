package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
)

const msgStartedUndeletable = "Data yang sudah pernah berlaku tidak bisa dihapus; isi tanggal berakhir untuk menghentikannya."

// --- Harga khusus ----------------------------------------------------------------------------

// SpecialPriceView adalah harga khusus beserta nama pihak dan produk.
type SpecialPriceView struct {
	domain.SpecialPrice
	PartyCode string
	PartyName string
	Product   ProductRef
	Status    domain.PeriodStatus
}

func specialPriceSnapshot(sp domain.SpecialPrice) map[string]any {
	return map[string]any{
		"partyType": sp.PartyType, "partyId": sp.PartyID.String(), "productId": sp.ProductID.String(),
		"priceAmount": sp.Price.String(), "validFrom": sp.ValidFrom.String(), "validTo": dateText(sp.ValidTo), "notes": sp.Notes,
	}
}

func (s *Service) parties(ctx context.Context, t domain.PartyType, ids []uuid.UUID) (map[uuid.UUID]PartyRef, error) {
	if s.Parties == nil || len(ids) == 0 {
		return map[uuid.UUID]PartyRef{}, nil
	}
	if t == domain.PartyCustomer {
		return s.Parties.Customers(ctx, uniqueIDs(ids))
	}
	return s.Parties.Partners(ctx, uniqueIDs(ids))
}

// ListSpecialPrices mengembalikan harga khusus (Data Master › Aturan Harga).
func (s *Service) ListSpecialPrices(ctx context.Context, f postgres.SpecialPriceFilter) ([]SpecialPriceView, error) {
	if _, err := actor.Require(ctx, PermSpecialPriceRead); err != nil {
		return nil, err
	}
	f.Today = s.today()
	list, err := s.Store.ListSpecialPrices(ctx, f)
	if err != nil {
		return nil, err
	}
	var customers, partners, products []uuid.UUID
	for _, sp := range list {
		products = append(products, sp.ProductID)
		if sp.PartyType == domain.PartyCustomer {
			customers = append(customers, sp.PartyID)
		} else {
			partners = append(partners, sp.PartyID)
		}
	}
	cs, err := s.parties(ctx, domain.PartyCustomer, customers)
	if err != nil {
		return nil, err
	}
	ps, err := s.parties(ctx, domain.PartyPartner, partners)
	if err != nil {
		return nil, err
	}
	prods, err := s.products(ctx, products)
	if err != nil {
		return nil, err
	}
	out := make([]SpecialPriceView, len(list))
	for i, sp := range list {
		ref := cs[sp.PartyID]
		if sp.PartyType == domain.PartyPartner {
			ref = ps[sp.PartyID]
		}
		out[i] = SpecialPriceView{
			SpecialPrice: sp, PartyCode: ref.Code, PartyName: ref.Name, Product: prods[sp.ProductID], Status: sp.StatusOn(f.Today),
		}
	}
	return out, nil
}

// CreateSpecialPrice menambah harga khusus. Harga khusus lama tanpa tanggal berakhir untuk pihak dan produk yang
// sama ditutup otomatis sehari sebelum harga baru berlaku.
func (s *Service) CreateSpecialPrice(ctx context.Context, in domain.SpecialPriceInput) (domain.SpecialPrice, error) {
	a, err := actor.Require(ctx, PermSpecialPriceWrite)
	if err != nil {
		return domain.SpecialPrice{}, err
	}
	sp, err := domain.NewSpecialPrice(in, s.today())
	if err != nil {
		return domain.SpecialPrice{}, err
	}
	if s.Parties != nil {
		refs, err := s.parties(ctx, sp.PartyType, []uuid.UUID{sp.PartyID})
		if err != nil {
			return domain.SpecialPrice{}, err
		}
		if _, ok := refs[sp.PartyID]; !ok {
			return domain.SpecialPrice{}, apperr.Validation(map[string]string{"partyId": "Pelanggan atau mitra tidak ditemukan"})
		}
	}
	if err := s.checkProducts(ctx, []uuid.UUID{sp.ProductID}, func(int) string { return "productId" }); err != nil {
		return domain.SpecialPrice{}, err
	}
	label := s.productLabel(ctx, sp.ProductID)
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		existing, err := s.Store.OverlappingSpecialPrices(ctx, sp)
		if err != nil {
			return err
		}
		err = supersede(existing, func(o domain.SpecialPrice) domain.Period { return o.Period }, sp.Period, "harga khusus lain",
			func(o domain.SpecialPrice, p domain.Period) error {
				before := o
				o.Period = p
				if _, err := s.Store.UpdateSpecialPriceEnd(ctx, o, o.Version, a.UserID); err != nil {
					return err
				}
				return s.Audit.Record(ctx, audit.Entry{
					Action: audit.ActionUpdate, EntityType: EntitySpecialPrice, EntityID: o.ID, EntityLabel: label,
					Before: specialPriceSnapshot(before), After: specialPriceSnapshot(o),
				})
			})
		if err != nil {
			return err
		}
		if sp, err = s.Store.InsertSpecialPrice(ctx, sp, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntitySpecialPrice, EntityID: sp.ID, EntityLabel: label, After: specialPriceSnapshot(sp),
		})
	})
	return sp, err
}

// EndSpecialPrice mengisi tanggal berakhir harga khusus.
func (s *Service) EndSpecialPrice(ctx context.Context, id uuid.UUID, version int, validTo clock.BusinessDate) (domain.SpecialPrice, error) {
	a, err := actor.Require(ctx, PermSpecialPriceWrite)
	if err != nil {
		return domain.SpecialPrice{}, err
	}
	var sp domain.SpecialPrice
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if sp, err = s.Store.GetSpecialPrice(ctx, id, true); err != nil {
			return err
		}
		if err := sp.EnsureActive(); err != nil {
			return err
		}
		if err := checkVersion(sp.Version, version); err != nil {
			return err
		}
		before := sp
		if sp.Period, err = sp.EndAt(validTo, s.today()); err != nil {
			return err
		}
		if sp, err = s.Store.UpdateSpecialPriceEnd(ctx, sp, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntitySpecialPrice, EntityID: sp.ID, EntityLabel: s.productLabel(ctx, sp.ProductID),
			Before: specialPriceSnapshot(before), After: specialPriceSnapshot(sp),
		})
	})
	return sp, err
}

// DeleteSpecialPrice menghapus harga khusus yang belum pernah berlaku.
func (s *Service) DeleteSpecialPrice(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermSpecialPriceWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		sp, err := s.Store.GetSpecialPrice(ctx, id, true)
		if err != nil {
			return err
		}
		if err := checkVersion(sp.Version, version); err != nil {
			return err
		}
		if !sp.CanDelete(s.today()) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, msgStartedUndeletable)
		}
		if err := s.Store.DeleteSpecialPrice(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntitySpecialPrice, EntityID: sp.ID, EntityLabel: s.productLabel(ctx, sp.ProductID),
			Before: specialPriceSnapshot(sp),
		})
	})
}

// --- Promo -----------------------------------------------------------------------------------

// PromotionView adalah promo beserta statusnya hari ini dan produk itemnya.
type PromotionView struct {
	domain.Promotion
	Status         domain.PeriodStatus
	Products       map[uuid.UUID]ProductRef
	AllowedActions []string
}

func promotionSnapshot(p domain.Promotion) map[string]any {
	items := make([]map[string]any, len(p.Items))
	for i, it := range p.Items {
		m := map[string]any{"productId": it.ProductID.String()}
		if it.Price != nil {
			m["priceAmount"] = it.Price.String()
		}
		if it.DiscountPercent != nil {
			m["discountPercent"] = it.DiscountPercent.String()
		}
		items[i] = m
	}
	return map[string]any{
		"code": p.Code, "name": p.Name, "channelId": idText(p.ChannelID), "subChannelId": idText(p.SubChannelID),
		"validFrom": p.ValidFrom.String(), "validTo": p.ValidTo.String(), "isActive": p.IsActive, "description": p.Description,
		"items": items,
	}
}

func promotionActions(a actor.Actor, p domain.Promotion, today clock.BusinessDate) []string {
	actions := []string{}
	if p.Deleted() || !a.Has(PermPromotionWrite) {
		return actions
	}
	actions = append(actions, "update")
	if !p.Started(today) {
		actions = append(actions, "delete")
	}
	return actions
}

// ListPromotions mengembalikan promo (tanpa item).
func (s *Service) ListPromotions(ctx context.Context, f postgres.PromotionFilter) ([]PromotionView, error) {
	a, err := actor.Require(ctx, PermPromotionRead)
	if err != nil {
		return nil, err
	}
	f.Today, f.Query = s.today(), normalizeQuery(f.Query)
	list, err := s.Store.ListPromotions(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]PromotionView, len(list))
	for i, p := range list {
		out[i] = PromotionView{Promotion: p, Status: p.Period().StatusOn(f.Today), AllowedActions: promotionActions(a, p, f.Today)}
	}
	return out, nil
}

// GetPromotion mengembalikan detail promo beserta item.
func (s *Service) GetPromotion(ctx context.Context, id uuid.UUID) (PromotionView, error) {
	a, err := actor.Require(ctx, PermPromotionRead)
	if err != nil {
		return PromotionView{}, err
	}
	p, err := s.Store.GetPromotion(ctx, id, false)
	if err != nil {
		return PromotionView{}, err
	}
	ids := make([]uuid.UUID, len(p.Items))
	for i, it := range p.Items {
		ids[i] = it.ProductID
	}
	products, err := s.products(ctx, ids)
	if err != nil {
		return PromotionView{}, err
	}
	today := s.today()
	return PromotionView{Promotion: p, Status: p.Period().StatusOn(today), Products: products, AllowedActions: promotionActions(a, p, today)}, nil
}

func (s *Service) checkPromotionProducts(ctx context.Context, p domain.Promotion) error {
	ids := make([]uuid.UUID, len(p.Items))
	for i, it := range p.Items {
		ids[i] = it.ProductID
	}
	return s.checkProducts(ctx, ids, itemField)
}

// CreatePromotion menambah promo periode.
func (s *Service) CreatePromotion(ctx context.Context, in domain.PromotionInput) (domain.Promotion, error) {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return domain.Promotion{}, err
	}
	p, err := domain.NewPromotion(in, s.today())
	if err != nil {
		return domain.Promotion{}, err
	}
	if err := s.checkPromotionProducts(ctx, p); err != nil {
		return domain.Promotion{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.InsertPromotion(ctx, p, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPromotion, EntityID: p.ID, EntityLabel: p.Code, After: promotionSnapshot(p),
		})
	})
	return p, err
}

// UpdatePromotion mengubah promo. Promo yang sudah berjalan hanya boleh diubah nama, keterangan, status, dan
// tanggal selesai.
func (s *Service) UpdatePromotion(ctx context.Context, id uuid.UUID, version int, in domain.PromotionInput) (domain.Promotion, error) {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return domain.Promotion{}, err
	}
	var p domain.Promotion
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetPromotion(ctx, id, true); err != nil {
			return err
		}
		if err := checkVersion(p.Version, version); err != nil {
			return err
		}
		before := p
		started := p.Started(s.today())
		if err := p.Update(in, s.today()); err != nil {
			return err
		}
		if !started {
			if err := s.checkPromotionProducts(ctx, p); err != nil {
				return err
			}
		}
		if p, err = s.Store.UpdatePromotion(ctx, p, version, a.UserID, !started); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPromotion, EntityID: p.ID, EntityLabel: p.Code,
			Before: promotionSnapshot(before), After: promotionSnapshot(p),
		})
	})
	return p, err
}

// DeletePromotion menghapus promo yang belum berjalan.
func (s *Service) DeletePromotion(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPromotion(ctx, id, true)
		if err != nil {
			return err
		}
		if err := checkVersion(p.Version, version); err != nil {
			return err
		}
		if p.Started(s.today()) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Promo yang sudah berjalan tidak bisa dihapus; nonaktifkan atau majukan tanggal selesainya.")
		}
		if err := s.Store.DeletePromotion(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityPromotion, EntityID: p.ID, EntityLabel: p.Code, Before: promotionSnapshot(p),
		})
	})
}

// --- Harga bertingkat ------------------------------------------------------------------------

// TieredPriceView adalah harga bertingkat beserta produk dan daftar harganya.
type TieredPriceView struct {
	domain.TieredPrice
	Product       ProductRef
	PriceListCode string
	PriceListName string
	Status        domain.PeriodStatus
}

func tieredPriceSnapshot(t domain.TieredPrice) map[string]any {
	return map[string]any{
		"priceListId": idText(t.PriceListID), "productId": t.ProductID.String(), "minQty": t.MinQty.String(),
		"priceAmount": t.Price.String(), "validFrom": t.ValidFrom.String(), "validTo": dateText(t.ValidTo), "notes": t.Notes,
	}
}

// ListTieredPrices mengembalikan harga bertingkat.
func (s *Service) ListTieredPrices(ctx context.Context, f postgres.TieredPriceFilter) ([]TieredPriceView, error) {
	if _, err := actor.Require(ctx, PermPromotionRead); err != nil {
		return nil, err
	}
	f.Today = s.today()
	list, err := s.Store.ListTieredPrices(ctx, f)
	if err != nil {
		return nil, err
	}
	var products, lists []uuid.UUID
	for _, t := range list {
		products = append(products, t.ProductID)
		if t.PriceListID != nil {
			lists = append(lists, *t.PriceListID)
		}
	}
	prods, err := s.products(ctx, products)
	if err != nil {
		return nil, err
	}
	pls, err := s.Store.PriceListsByIDs(ctx, uniqueIDs(lists))
	if err != nil {
		return nil, err
	}
	out := make([]TieredPriceView, len(list))
	for i, t := range list {
		v := TieredPriceView{TieredPrice: t, Product: prods[t.ProductID], Status: t.StatusOn(f.Today)}
		if t.PriceListID != nil {
			v.PriceListCode, v.PriceListName = pls[*t.PriceListID].Code, pls[*t.PriceListID].Name
		}
		out[i] = v
	}
	return out, nil
}

// CreateTieredPrice menambah harga bertingkat. Harga lama tanpa tanggal berakhir untuk daftar harga, produk,
// dan qty minimum yang sama ditutup otomatis.
func (s *Service) CreateTieredPrice(ctx context.Context, in domain.TieredPriceInput) (domain.TieredPrice, error) {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return domain.TieredPrice{}, err
	}
	t, err := domain.NewTieredPrice(in, s.today())
	if err != nil {
		return domain.TieredPrice{}, err
	}
	if t.PriceListID != nil {
		l, err := s.Store.GetPriceList(ctx, *t.PriceListID, false)
		if apperr.HasCode(err, apperr.CodeNotFound) || (err == nil && l.Deleted()) {
			return domain.TieredPrice{}, apperr.Validation(map[string]string{"priceListId": "Daftar harga tidak ditemukan"})
		}
		if err != nil {
			return domain.TieredPrice{}, err
		}
	}
	if err := s.checkProducts(ctx, []uuid.UUID{t.ProductID}, func(int) string { return "productId" }); err != nil {
		return domain.TieredPrice{}, err
	}
	label := s.productLabel(ctx, t.ProductID)
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		existing, err := s.Store.OverlappingTieredPrices(ctx, t)
		if err != nil {
			return err
		}
		err = supersede(existing, func(o domain.TieredPrice) domain.Period { return o.Period }, t.Period, "harga bertingkat lain",
			func(o domain.TieredPrice, p domain.Period) error {
				before := o
				o.Period = p
				if _, err := s.Store.UpdateTieredPriceEnd(ctx, o, o.Version, a.UserID); err != nil {
					return err
				}
				return s.Audit.Record(ctx, audit.Entry{
					Action: audit.ActionUpdate, EntityType: EntityTieredPrice, EntityID: o.ID, EntityLabel: label,
					Before: tieredPriceSnapshot(before), After: tieredPriceSnapshot(o),
				})
			})
		if err != nil {
			return err
		}
		if t, err = s.Store.InsertTieredPrice(ctx, t, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityTieredPrice, EntityID: t.ID, EntityLabel: label, After: tieredPriceSnapshot(t),
		})
	})
	return t, err
}

// EndTieredPrice mengisi tanggal berakhir harga bertingkat.
func (s *Service) EndTieredPrice(ctx context.Context, id uuid.UUID, version int, validTo clock.BusinessDate) (domain.TieredPrice, error) {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return domain.TieredPrice{}, err
	}
	var t domain.TieredPrice
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if t, err = s.Store.GetTieredPrice(ctx, id, true); err != nil {
			return err
		}
		if err := t.EnsureActive(); err != nil {
			return err
		}
		if err := checkVersion(t.Version, version); err != nil {
			return err
		}
		before := t
		if t.Period, err = t.EndAt(validTo, s.today()); err != nil {
			return err
		}
		if t, err = s.Store.UpdateTieredPriceEnd(ctx, t, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityTieredPrice, EntityID: t.ID, EntityLabel: s.productLabel(ctx, t.ProductID),
			Before: tieredPriceSnapshot(before), After: tieredPriceSnapshot(t),
		})
	})
	return t, err
}

// DeleteTieredPrice menghapus harga bertingkat yang belum pernah berlaku.
func (s *Service) DeleteTieredPrice(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermPromotionWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		t, err := s.Store.GetTieredPrice(ctx, id, true)
		if err != nil {
			return err
		}
		if err := checkVersion(t.Version, version); err != nil {
			return err
		}
		if !t.CanDelete(s.today()) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, msgStartedUndeletable)
		}
		if err := s.Store.DeleteTieredPrice(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityTieredPrice, EntityID: t.ID, EntityLabel: s.productLabel(ctx, t.ProductID),
			Before: tieredPriceSnapshot(t),
		})
	})
}

// TieredPricesOn membaca harga bertingkat yang berlaku untuk resolver.
func (s *Service) TieredPricesOn(ctx context.Context, productID uuid.UUID, priceListIDs []uuid.UUID, qty decimal.Decimal, on clock.BusinessDate) ([]domain.TieredPrice, error) {
	return s.Store.TieredPricesOn(ctx, productID, priceListIDs, qty, on)
}
