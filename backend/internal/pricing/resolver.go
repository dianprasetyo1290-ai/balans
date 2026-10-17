package pricing

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/pricing/domain"
)

// Source membaca data harga yang berlaku pada tanggal transaksi (dipenuhi service modul).
type Source interface {
	SpecialPriceOn(ctx context.Context, t domain.PartyType, partyID, productID uuid.UUID, on clock.BusinessDate) (domain.SpecialPrice, bool, error)
	PromotionOffersOn(ctx context.Context, productID uuid.UUID, channelID, subChannelID *uuid.UUID, on clock.BusinessDate) ([]domain.PromotionOffer, error)
	TieredPricesOn(ctx context.Context, productID uuid.UUID, priceListIDs []uuid.UUID, qty decimal.Decimal, on clock.BusinessDate) ([]domain.TieredPrice, error)
	ScopePriceLists(ctx context.Context, partnerID, channelID, subChannelID *uuid.UUID) ([]domain.PriceList, error)
	PriceListByID(ctx context.Context, id uuid.UUID) (domain.PriceList, error)
	BasePriceList(ctx context.Context) (domain.PriceList, error)
	ListPriceOn(ctx context.Context, l domain.PriceList, productID uuid.UUID, on clock.BusinessDate) (domain.ListPrice, bool, error)
}

// Eligibility membaca hak harga dari modul party: harga karyawan (pelanggan tertaut karyawan aktif, HR-03) dan
// daftar harga di PKS mitra yang berlaku.
type Eligibility interface {
	EmployeePriceEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error)
	PartnerAgreementPriceList(ctx context.Context, partnerID uuid.UUID, on clock.BusinessDate) (*uuid.UUID, error)
}

// PriceQuery adalah permintaan harga beserta daftar harga kandidat (urut prioritas) yang dihitung sekali oleh
// Resolver. Next menghitung harga dari aturan berikutnya di rantai (dipakai promo potongan persen).
type PriceQuery struct {
	domain.PriceRequest
	// Lists adalah daftar harga yang berlaku untuk transaksi, urut prioritas: karyawan, PKS mitra, mitra,
	// sub-channel, channel.
	Lists []domain.PriceList
	next  func(ctx context.Context) (domain.PriceQuote, bool, error)
}

// Next menghitung harga dari aturan dengan prioritas lebih rendah (harga tanpa aturan saat ini).
func (q PriceQuery) Next(ctx context.Context) (domain.PriceQuote, bool, error) {
	if q.next == nil {
		return domain.PriceQuote{}, false, nil
	}
	return q.next(ctx)
}

// ListIDs mengembalikan ID daftar harga kandidat.
func (q PriceQuery) ListIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(q.Lists))
	for i, l := range q.Lists {
		ids[i] = l.ID
	}
	return ids
}

// PriceRule adalah satu mata rantai penentuan harga (Technical Guideline 5.6). ok=false meneruskan ke aturan
// berikutnya.
type PriceRule interface {
	Name() domain.Rule
	Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error)
}

// Resolver menjalankan rantai aturan harga berurutan dan mengembalikan hasil aturan pertama yang cocok.
type Resolver struct {
	rules       []PriceRule
	source      Source
	eligibility Eligibility
}

// NewResolver menyusun rantai aturan. Urutan WAJIB sama dengan BRD 5.18 (lihat DefaultRules).
func NewResolver(source Source, eligibility Eligibility, rules ...PriceRule) *Resolver {
	return &Resolver{rules: rules, source: source, eligibility: eligibility}
}

// DefaultRules adalah rantai 5 aturan BRD 5.18: harga khusus → promo → bertingkat → daftar harga channel →
// harga dasar.
func DefaultRules(s Source) []PriceRule {
	return []PriceRule{
		SpecialPriceRule{Source: s},
		PromotionRule{Source: s},
		TieredPriceRule{Source: s},
		ChannelPriceListRule{Source: s},
		BasePriceRule{Source: s},
	}
}

// errNoPrice: tidak ada aturan yang menghasilkan harga.
func errNoPrice() error {
	return apperr.WithMessage(apperr.CodeNotFound,
		"Harga produk belum diatur: tidak ada harga khusus, promo, daftar harga, atau Harga Dasar & HET yang berlaku.")
}

// Resolve menentukan harga satu baris transaksi.
func (r *Resolver) Resolve(ctx context.Context, req domain.PriceRequest) (domain.PriceQuote, error) {
	if err := req.Validate(); err != nil {
		return domain.PriceQuote{}, err
	}
	lists, err := r.candidateLists(ctx, req)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	q := PriceQuery{PriceRequest: req, Lists: lists}
	quote, ok, err := r.run(ctx, q, 0)
	if err != nil {
		return domain.PriceQuote{}, err
	}
	if !ok {
		return domain.PriceQuote{}, errNoPrice()
	}
	return quote, nil
}

func (r *Resolver) run(ctx context.Context, q PriceQuery, from int) (domain.PriceQuote, bool, error) {
	for i := from; i < len(r.rules); i++ {
		next := i + 1
		q.next = func(ctx context.Context) (domain.PriceQuote, bool, error) { return r.run(ctx, q, next) }
		quote, ok, err := r.rules[i].Resolve(ctx, q)
		if err != nil {
			return domain.PriceQuote{}, false, fmt.Errorf("aturan harga %s: %w", r.rules[i].Name(), err)
		}
		if ok {
			quote.Rule = r.rules[i].Name()
			return quote, true, nil
		}
	}
	return domain.PriceQuote{}, false, nil
}

// candidateLists menentukan daftar harga yang berlaku untuk transaksi, urut prioritas: daftar karyawan (pelanggan
// tertaut karyawan aktif), daftar harga di PKS mitra yang berlaku, daftar per mitra, daftar sub-channel, lalu
// daftar channel. Daftar nonaktif atau terhapus dilewati.
func (r *Resolver) candidateLists(ctx context.Context, req domain.PriceRequest) ([]domain.PriceList, error) {
	scope, err := r.source.ScopePriceLists(ctx, req.PartnerID, req.ChannelID, req.SubChannelID)
	if err != nil {
		return nil, err
	}
	var out []domain.PriceList
	seen := map[uuid.UUID]bool{}
	add := func(l domain.PriceList) {
		if !seen[l.ID] && l.Usable() {
			seen[l.ID] = true
			out = append(out, l)
		}
	}
	pick := func(match func(domain.PriceList) bool) {
		for _, l := range scope {
			if match(l) {
				add(l)
			}
		}
	}
	if req.CustomerID != nil && r.eligibility != nil {
		ok, err := r.eligibility.EmployeePriceEligible(ctx, *req.CustomerID, req.Date)
		if err != nil {
			return nil, err
		}
		if ok {
			pick(func(l domain.PriceList) bool { return l.Type == domain.TypeEmployee })
		}
	}
	if req.PartnerID != nil {
		if r.eligibility != nil {
			id, err := r.eligibility.PartnerAgreementPriceList(ctx, *req.PartnerID, req.Date)
			if err != nil {
				return nil, err
			}
			if id != nil {
				l, err := r.source.PriceListByID(ctx, *id)
				switch {
				case apperr.HasCode(err, apperr.CodeNotFound):
				case err != nil:
					return nil, err
				default:
					add(l)
				}
			}
		}
		pick(func(l domain.PriceList) bool {
			return l.Type == domain.TypePartner && l.PartnerID != nil && *l.PartnerID == *req.PartnerID
		})
	}
	if req.ChannelID != nil {
		isChannel := func(l domain.PriceList) bool {
			return (l.Type == domain.TypeChannel || l.Type == domain.TypeVending) && l.ChannelID != nil && *l.ChannelID == *req.ChannelID
		}
		if req.SubChannelID != nil {
			pick(func(l domain.PriceList) bool {
				return isChannel(l) && l.SubChannelID != nil && *l.SubChannelID == *req.SubChannelID
			})
		}
		pick(func(l domain.PriceList) bool { return isChannel(l) && l.SubChannelID == nil })
	}
	return out, nil
}

func ptr[T any](v T) *T { return &v }

// SpecialPriceRule (prioritas 1): harga khusus pelanggan, lalu harga khusus mitra.
type SpecialPriceRule struct{ Source Source }

// Name mengembalikan nama aturan.
func (SpecialPriceRule) Name() domain.Rule { return domain.RuleSpecialPrice }

// Resolve mencari harga khusus yang berlaku.
func (r SpecialPriceRule) Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error) {
	parties := []struct {
		t  domain.PartyType
		id *uuid.UUID
	}{{domain.PartyCustomer, q.CustomerID}, {domain.PartyPartner, q.PartnerID}}
	for _, p := range parties {
		if p.id == nil {
			continue
		}
		sp, ok, err := r.Source.SpecialPriceOn(ctx, p.t, *p.id, q.ProductID, q.Date)
		if err != nil || !ok {
			if err != nil {
				return domain.PriceQuote{}, false, err
			}
			continue
		}
		who := "pelanggan"
		if p.t == domain.PartyPartner {
			who = "mitra"
		}
		return domain.PriceQuote{
			UnitPrice: sp.Price, SourceID: sp.ID, SpecialPriceID: ptr(sp.ID), Description: "Harga khusus " + who,
		}, true, nil
	}
	return domain.PriceQuote{}, false, nil
}

// PromotionRule (prioritas 2): promo periode aktif untuk channel transaksi. Bila beberapa promo berlaku,
// dipilih harga terendah. Potongan persen dihitung dari harga aturan berikutnya (harga tanpa promo).
type PromotionRule struct{ Source Source }

// Name mengembalikan nama aturan.
func (PromotionRule) Name() domain.Rule { return domain.RulePromotion }

// Resolve mencari promo yang berlaku.
func (r PromotionRule) Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error) {
	offers, err := r.Source.PromotionOffersOn(ctx, q.ProductID, q.ChannelID, q.SubChannelID, q.Date)
	if err != nil || len(offers) == 0 {
		return domain.PriceQuote{}, false, err
	}
	var (
		regular    domain.PriceQuote
		hasRegular bool
		loaded     bool
		best       *domain.PriceQuote
	)
	for _, o := range offers {
		var price = o.Item.Price
		desc := "Promo " + o.Code
		var base *domain.PriceQuote
		if o.Item.NeedsRegular() {
			if !loaded {
				if regular, hasRegular, err = q.Next(ctx); err != nil {
					return domain.PriceQuote{}, false, err
				}
				loaded = true
			}
			if !hasRegular {
				continue
			}
			p := o.Item.Offer(regular.UnitPrice)
			price, base = &p, &regular
			desc = fmt.Sprintf("Promo %s (potongan %s%%)", o.Code, o.Item.DiscountPercent.String())
		}
		if best != nil && price.Cmp(best.UnitPrice) >= 0 {
			continue
		}
		quote := domain.PriceQuote{UnitPrice: *price, SourceID: o.PromotionID, PromotionID: ptr(o.PromotionID), Description: desc}
		if base != nil {
			quote.RegularPrice = ptr(base.UnitPrice)
			quote.PriceListID, quote.PriceListVersionID = base.PriceListID, base.PriceListVersionID
		}
		best = &quote
	}
	if best == nil {
		return domain.PriceQuote{}, false, nil
	}
	return *best, true, nil
}

// TieredPriceRule (prioritas 3): harga bertingkat qty minimum. Harga bertingkat milik daftar harga kandidat
// pertama didahulukan, lalu harga bertingkat umum; dalam satu daftar dipilih qty minimum terbesar yang terpenuhi.
type TieredPriceRule struct{ Source Source }

// Name mengembalikan nama aturan.
func (TieredPriceRule) Name() domain.Rule { return domain.RuleTieredPrice }

// Resolve mencari harga bertingkat yang berlaku.
func (r TieredPriceRule) Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error) {
	tiers, err := r.Source.TieredPricesOn(ctx, q.ProductID, q.ListIDs(), q.Qty, q.Date)
	if err != nil || len(tiers) == 0 {
		return domain.PriceQuote{}, false, err
	}
	rank := func(t domain.TieredPrice) int {
		if t.PriceListID == nil {
			return len(q.Lists)
		}
		for i, l := range q.Lists {
			if l.ID == *t.PriceListID {
				return i
			}
		}
		return -1
	}
	var best *domain.TieredPrice
	bestRank := 0
	for _, t := range tiers {
		rk := rank(t)
		if rk < 0 || t.MinQty.GreaterThan(q.Qty) || !t.Contains(q.Date) || t.Deleted() {
			continue
		}
		if best == nil || rk < bestRank || (rk == bestRank && t.MinQty.GreaterThan(best.MinQty)) {
			best, bestRank = &t, rk
		}
	}
	if best == nil {
		return domain.PriceQuote{}, false, nil
	}
	quote := domain.PriceQuote{
		UnitPrice: best.Price, SourceID: best.ID, TieredPriceID: ptr(best.ID), PriceListID: best.PriceListID,
		Description: fmt.Sprintf("Harga bertingkat qty ≥ %s", best.MinQty.String()),
	}
	return quote, true, nil
}

// ChannelPriceListRule (prioritas 4): daftar harga kandidat pertama yang memuat produk (karyawan, PKS mitra, mitra,
// sub-channel, channel).
type ChannelPriceListRule struct{ Source Source }

// Name mengembalikan nama aturan.
func (ChannelPriceListRule) Name() domain.Rule { return domain.RulePriceList }

// Resolve mencari harga di daftar harga kandidat.
func (r ChannelPriceListRule) Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error) {
	for _, l := range q.Lists {
		lp, ok, err := r.Source.ListPriceOn(ctx, l, q.ProductID, q.Date)
		if err != nil {
			return domain.PriceQuote{}, false, err
		}
		if ok {
			return listQuote(lp, "Daftar harga "+lp.Name), true, nil
		}
	}
	return domain.PriceQuote{}, false, nil
}

func listQuote(lp domain.ListPrice, desc string) domain.PriceQuote {
	return domain.PriceQuote{
		UnitPrice: lp.Price, SourceID: lp.VersionID, PriceListID: ptr(lp.PriceListID), PriceListVersionID: ptr(lp.VersionID),
		MaxRetailPrice: lp.MaxRetailPrice, Description: desc,
	}
}

// BasePriceRule (prioritas 5): Harga Dasar & HET.
type BasePriceRule struct{ Source Source }

// Name mengembalikan nama aturan.
func (BasePriceRule) Name() domain.Rule { return domain.RuleBasePrice }

// Resolve membaca harga dasar produk.
func (r BasePriceRule) Resolve(ctx context.Context, q PriceQuery) (domain.PriceQuote, bool, error) {
	base, err := r.Source.BasePriceList(ctx)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.PriceQuote{}, false, nil
	}
	if err != nil {
		return domain.PriceQuote{}, false, err
	}
	lp, ok, err := r.Source.ListPriceOn(ctx, base, q.ProductID, q.Date)
	if err != nil || !ok {
		return domain.PriceQuote{}, false, err
	}
	return listQuote(lp, "Harga dasar"), true, nil
}
