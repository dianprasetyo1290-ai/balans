package pricing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/pricing"
	"balans/internal/pricing/domain"
)

var (
	today     = clock.NewBusinessDate(2026, 10, 23)
	errSource = errors.New("db mati")
)

// fakeSource adalah sumber data harga di memori untuk menguji rantai aturan tanpa database.
type fakeSource struct {
	special map[domain.PartyType]map[uuid.UUID]domain.SpecialPrice
	offers  []domain.PromotionOffer
	tiers   []domain.TieredPrice
	lists   []domain.PriceList
	prices  map[uuid.UUID]money.Amount // per daftar harga untuk produk yang diuji
	het     map[uuid.UUID]money.Amount
	base    *domain.PriceList
	fail    string
}

func newSource() *fakeSource {
	return &fakeSource{
		special: map[domain.PartyType]map[uuid.UUID]domain.SpecialPrice{},
		prices:  map[uuid.UUID]money.Amount{}, het: map[uuid.UUID]money.Amount{},
	}
}

func (f *fakeSource) err(op string) error {
	if f.fail == op {
		return errSource
	}
	return nil
}

func (f *fakeSource) SpecialPriceOn(_ context.Context, t domain.PartyType, partyID, _ uuid.UUID, _ clock.BusinessDate) (domain.SpecialPrice, bool, error) {
	if err := f.err("special"); err != nil {
		return domain.SpecialPrice{}, false, err
	}
	sp, ok := f.special[t][partyID]
	return sp, ok, nil
}

func (f *fakeSource) PromotionOffersOn(context.Context, uuid.UUID, *uuid.UUID, *uuid.UUID, clock.BusinessDate) ([]domain.PromotionOffer, error) {
	return f.offers, f.err("promo")
}

func (f *fakeSource) TieredPricesOn(_ context.Context, _ uuid.UUID, ids []uuid.UUID, _ decimal.Decimal, _ clock.BusinessDate) ([]domain.TieredPrice, error) {
	if err := f.err("tier"); err != nil {
		return nil, err
	}
	var out []domain.TieredPrice
	for _, t := range f.tiers {
		if t.PriceListID == nil {
			out = append(out, t)
			continue
		}
		for _, id := range ids {
			if id == *t.PriceListID {
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func (f *fakeSource) ScopePriceLists(context.Context, *uuid.UUID, *uuid.UUID, *uuid.UUID) ([]domain.PriceList, error) {
	return f.lists, f.err("scope")
}

func (f *fakeSource) PriceListByID(_ context.Context, id uuid.UUID) (domain.PriceList, error) {
	if err := f.err("byid"); err != nil {
		return domain.PriceList{}, err
	}
	for _, l := range f.lists {
		if l.ID == id {
			return l, nil
		}
	}
	return domain.PriceList{}, apperr.New(apperr.CodeNotFound)
}

func (f *fakeSource) BasePriceList(context.Context) (domain.PriceList, error) {
	if err := f.err("base"); err != nil {
		return domain.PriceList{}, err
	}
	if f.base == nil {
		return domain.PriceList{}, apperr.New(apperr.CodeNotFound)
	}
	return *f.base, nil
}

func (f *fakeSource) ListPriceOn(_ context.Context, l domain.PriceList, _ uuid.UUID, _ clock.BusinessDate) (domain.ListPrice, bool, error) {
	if err := f.err("list"); err != nil {
		return domain.ListPrice{}, false, err
	}
	p, ok := f.prices[l.ID]
	if !ok {
		return domain.ListPrice{}, false, nil
	}
	lp := domain.ListPrice{PriceListID: l.ID, VersionID: versionOf(l.ID), Code: l.Code, Name: l.Name, Price: p}
	if het, ok := f.het[l.ID]; ok {
		lp.MaxRetailPrice = &het
	}
	return lp, true, nil
}

// versionOf membuat ID versi deterministik per daftar harga.
func versionOf(listID uuid.UUID) uuid.UUID { return uuid.NewSHA1(listID, []byte("v1")) }

type fakeEligibility struct {
	employee   map[uuid.UUID]bool
	agreements map[uuid.UUID]uuid.UUID
	fail       string
}

func (e fakeEligibility) EmployeePriceEligible(_ context.Context, id uuid.UUID, _ clock.BusinessDate) (bool, error) {
	if e.fail == "employee" {
		return false, errSource
	}
	return e.employee[id], nil
}

func (e fakeEligibility) PartnerAgreementPriceList(_ context.Context, id uuid.UUID, _ clock.BusinessDate) (*uuid.UUID, error) {
	if e.fail == "agreement" {
		return nil, errSource
	}
	if l, ok := e.agreements[id]; ok {
		return &l, nil
	}
	return nil, nil
}

func list(t domain.PriceListType, code string) domain.PriceList {
	return domain.PriceList{ID: uuid.New(), Code: code, Name: code, Type: t, Basis: domain.BasisFixed, IsActive: true}
}

func amount(v int64) money.Amount { return money.FromInt(v) }

func ptr[T any](v T) *T { return &v }

// fixture adalah data harga satu produk di semua aturan.
type fixture struct {
	src                                       *fakeSource
	elig                                      fakeEligibility
	channel, sub, customer, employee, partner uuid.UUID
	base, web, webSub, emp, partnerList, pks  domain.PriceList
}

func newFixture() *fixture {
	f := &fixture{
		src: newSource(), channel: uuid.New(), sub: uuid.New(), customer: uuid.New(), employee: uuid.New(), partner: uuid.New(),
	}
	f.base = list(domain.TypeBase, "HARGA-DASAR")
	f.web = list(domain.TypeChannel, "WEBSITE")
	f.web.ChannelID = &f.channel
	f.webSub = list(domain.TypeChannel, "WEBSITE-FYB")
	f.webSub.ChannelID, f.webSub.SubChannelID = &f.channel, &f.sub
	f.emp = list(domain.TypeEmployee, "KARYAWAN")
	f.partnerList = list(domain.TypePartner, "MITRA-A")
	f.partnerList.PartnerID = &f.partner
	f.pks = list(domain.TypePartner, "PKS-A")
	other := uuid.New()
	f.pks.PartnerID = &other
	f.src.base = &f.base
	f.src.lists = []domain.PriceList{f.web, f.webSub, f.emp, f.partnerList, f.pks}
	f.src.prices[f.base.ID] = amount(50000)
	f.src.het[f.base.ID] = amount(55000)
	f.src.prices[f.web.ID] = amount(48000)
	f.src.prices[f.webSub.ID] = amount(47000)
	f.src.prices[f.emp.ID] = amount(40000)
	f.src.prices[f.partnerList.ID] = amount(42000)
	f.src.prices[f.pks.ID] = amount(41000)
	f.elig = fakeEligibility{employee: map[uuid.UUID]bool{f.employee: true}, agreements: map[uuid.UUID]uuid.UUID{f.partner: f.pks.ID}}
	return f
}

func (f *fixture) resolver() *pricing.Resolver {
	return pricing.NewResolver(f.src, f.elig, pricing.DefaultRules(f.src)...)
}

func (f *fixture) req(mod ...func(*domain.PriceRequest)) domain.PriceRequest {
	r := domain.PriceRequest{ProductID: uuid.New(), Qty: decimal.NewFromInt(1), Date: today}
	for _, m := range mod {
		m(&r)
	}
	return r
}

func TestResolverRules(t *testing.T) {
	type want struct {
		rule  domain.Rule
		price int64
	}
	tests := []struct {
		name  string
		setup func(f *fixture)
		req   func(f *fixture) func(*domain.PriceRequest)
		want  want
		check func(t *testing.T, f *fixture, q domain.PriceQuote)
	}{
		{
			name: "5 harga dasar bila tidak ada channel",
			want: want{domain.RuleBasePrice, 50000},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, f.base.ID, *q.PriceListID)
				assert.Equal(t, versionOf(f.base.ID), *q.PriceListVersionID)
				assert.Equal(t, versionOf(f.base.ID), q.SourceID)
				assert.Equal(t, "55000.00", q.MaxRetailPrice.String())
			},
		},
		{
			name: "4 daftar harga channel",
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID = &f.channel }
			},
			want: want{domain.RulePriceList, 48000},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, f.web.ID, *q.PriceListID)
				assert.Equal(t, versionOf(f.web.ID), *q.PriceListVersionID)
			},
		},
		{
			name: "4 sub-channel didahulukan dari channel",
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.SubChannelID = &f.channel, &f.sub }
			},
			want: want{domain.RulePriceList, 47000},
		},
		{
			name:  "4 produk tidak ada di sub-channel → channel",
			setup: func(f *fixture) { delete(f.src.prices, f.webSub.ID) },
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.SubChannelID = &f.channel, &f.sub }
			},
			want: want{domain.RulePriceList, 48000},
		},
		{
			name:  "4 daftar nonaktif dilewati → harga dasar",
			setup: func(f *fixture) { f.src.lists[0].IsActive = false },
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID = &f.channel }
			},
			want: want{domain.RuleBasePrice, 50000},
		},
		{
			name: "4 pelanggan karyawan aktif memakai daftar karyawan",
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.CustomerID = &f.channel, &f.employee }
			},
			want: want{domain.RulePriceList, 40000},
		},
		{
			name: "4 pelanggan bukan karyawan tidak dapat harga karyawan",
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.CustomerID = &f.channel, &f.customer }
			},
			want: want{domain.RulePriceList, 48000},
		},
		{
			name: "4 daftar harga di PKS mitra didahulukan",
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.PartnerID = &f.channel, &f.partner }
			},
			want: want{domain.RulePriceList, 41000},
		},
		{
			name:  "4 tanpa PKS → daftar per mitra",
			setup: func(f *fixture) { f.elig.agreements = nil },
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.PartnerID = &f.channel, &f.partner }
			},
			want: want{domain.RulePriceList, 42000},
		},
		{
			name:  "4 PKS menunjuk daftar yang tidak ada → daftar per mitra",
			setup: func(f *fixture) { f.elig.agreements[f.partner] = uuid.New() },
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.PartnerID = &f.partner }
			},
			want: want{domain.RulePriceList, 42000},
		},
		{
			name: "3 harga bertingkat qty minimum terbesar yang terpenuhi",
			setup: func(f *fixture) {
				f.src.tiers = []domain.TieredPrice{
					{ID: uuid.New(), PriceListID: &f.web.ID, MinQty: decimal.NewFromInt(10), Price: amount(45000), Period: domain.Period{ValidFrom: today}},
					{ID: uuid.New(), PriceListID: &f.web.ID, MinQty: decimal.NewFromInt(50), Price: amount(43000), Period: domain.Period{ValidFrom: today}},
					{ID: uuid.New(), PriceListID: &f.web.ID, MinQty: decimal.NewFromInt(100), Price: amount(40000), Period: domain.Period{ValidFrom: today}},
				}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.Qty = &f.channel, decimal.NewFromInt(60) }
			},
			want: want{domain.RuleTieredPrice, 43000},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, f.src.tiers[1].ID, *q.TieredPriceID)
				assert.Equal(t, f.src.tiers[1].ID, q.SourceID)
				assert.Equal(t, f.web.ID, *q.PriceListID)
			},
		},
		{
			name: "3 qty di bawah semua tingkat → daftar harga",
			setup: func(f *fixture) {
				f.src.tiers = []domain.TieredPrice{{ID: uuid.New(), PriceListID: &f.web.ID, MinQty: decimal.NewFromInt(10), Price: amount(45000), Period: domain.Period{ValidFrom: today}}}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.Qty = &f.channel, decimal.NewFromInt(9) }
			},
			want: want{domain.RulePriceList, 48000},
		},
		{
			name: "3 tingkat daftar kandidat didahulukan dari tingkat umum",
			setup: func(f *fixture) {
				f.src.tiers = []domain.TieredPrice{
					{ID: uuid.New(), MinQty: decimal.NewFromInt(100), Price: amount(30000), Period: domain.Period{ValidFrom: today}},
					{ID: uuid.New(), PriceListID: &f.web.ID, MinQty: decimal.NewFromInt(10), Price: amount(45000), Period: domain.Period{ValidFrom: today}},
				}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.Qty = &f.channel, decimal.NewFromInt(100) }
			},
			want: want{domain.RuleTieredPrice, 45000},
		},
		{
			name: "3 tingkat umum berlaku tanpa daftar harga",
			setup: func(f *fixture) {
				f.src.tiers = []domain.TieredPrice{{ID: uuid.New(), MinQty: decimal.NewFromInt(12), Price: amount(46000), Period: domain.Period{ValidFrom: today}}}
			},
			req: func(*fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.Qty = decimal.NewFromInt(12) }
			},
			want: want{domain.RuleTieredPrice, 46000},
		},
		{
			name: "3 tingkat yang sudah berakhir dilewati",
			setup: func(f *fixture) {
				end := today.AddDays(-1)
				f.src.tiers = []domain.TieredPrice{{ID: uuid.New(), MinQty: decimal.NewFromInt(1), Price: amount(1), Period: domain.Period{ValidFrom: today.AddDays(-9), ValidTo: &end}}}
			},
			want: want{domain.RuleBasePrice, 50000},
		},
		{
			name: "2 promo harga tetap mengalahkan harga bertingkat",
			setup: func(f *fixture) {
				f.src.tiers = []domain.TieredPrice{{ID: uuid.New(), MinQty: decimal.NewFromInt(1), Price: amount(30000), Period: domain.Period{ValidFrom: today}}}
				f.src.offers = []domain.PromotionOffer{{PromotionID: uuid.New(), Code: "PROMO-1111", Item: domain.PromotionItem{Price: ptr(amount(39000))}}}
			},
			want: want{domain.RulePromotion, 39000},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, f.src.offers[0].PromotionID, *q.PromotionID)
				assert.Nil(t, q.RegularPrice)
			},
		},
		{
			name: "2 promo persen dihitung dari harga tanpa promo",
			setup: func(f *fixture) {
				f.src.offers = []domain.PromotionOffer{{PromotionID: uuid.New(), Code: "PROMO-10", Item: domain.PromotionItem{DiscountPercent: ptr(decimal.NewFromInt(10))}}}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID = &f.channel }
			},
			want: want{domain.RulePromotion, 43200},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, "48000.00", q.RegularPrice.String())
				assert.Equal(t, versionOf(f.web.ID), *q.PriceListVersionID, "versi harga normal tetap tercatat")
				assert.Contains(t, q.Description, "10%")
			},
		},
		{
			name: "2 beberapa promo → harga terendah",
			setup: func(f *fixture) {
				f.src.offers = []domain.PromotionOffer{
					{PromotionID: uuid.New(), Code: "A", Item: domain.PromotionItem{Price: ptr(amount(45000))}},
					{PromotionID: uuid.New(), Code: "B", Item: domain.PromotionItem{DiscountPercent: ptr(decimal.NewFromInt(20))}},
					{PromotionID: uuid.New(), Code: "C", Item: domain.PromotionItem{Price: ptr(amount(44000))}},
				}
			},
			want: want{domain.RulePromotion, 40000},
		},
		{
			name: "2 promo persen tanpa harga normal dilewati",
			setup: func(f *fixture) {
				f.src.base = nil
				f.src.offers = []domain.PromotionOffer{
					{PromotionID: uuid.New(), Code: "B", Item: domain.PromotionItem{DiscountPercent: ptr(decimal.NewFromInt(20))}},
					{PromotionID: uuid.New(), Code: "C", Item: domain.PromotionItem{Price: ptr(amount(44000))}},
				}
			},
			want: want{domain.RulePromotion, 44000},
		},
		{
			name: "1 harga khusus pelanggan mengalahkan semua aturan",
			setup: func(f *fixture) {
				f.src.special[domain.PartyCustomer] = map[uuid.UUID]domain.SpecialPrice{f.employee: {ID: uuid.New(), Price: amount(35000)}}
				f.src.offers = []domain.PromotionOffer{{PromotionID: uuid.New(), Code: "A", Item: domain.PromotionItem{Price: ptr(amount(30000))}}}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.ChannelID, r.CustomerID = &f.channel, &f.employee }
			},
			want: want{domain.RuleSpecialPrice, 35000},
			check: func(t *testing.T, f *fixture, q domain.PriceQuote) {
				assert.Equal(t, q.SourceID, *q.SpecialPriceID)
				assert.Equal(t, "Harga khusus pelanggan", q.Description)
			},
		},
		{
			name: "1 harga khusus mitra",
			setup: func(f *fixture) {
				f.src.special[domain.PartyPartner] = map[uuid.UUID]domain.SpecialPrice{f.partner: {ID: uuid.New(), Price: amount(38000)}}
			},
			req: func(f *fixture) func(*domain.PriceRequest) {
				return func(r *domain.PriceRequest) { r.CustomerID, r.PartnerID = &f.customer, &f.partner }
			},
			want: want{domain.RuleSpecialPrice, 38000},
			check: func(t *testing.T, _ *fixture, q domain.PriceQuote) {
				assert.Equal(t, "Harga khusus mitra", q.Description)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if tc.setup != nil {
				tc.setup(f)
			}
			var mods []func(*domain.PriceRequest)
			if tc.req != nil {
				mods = append(mods, tc.req(f))
			}
			q, err := f.resolver().Resolve(context.Background(), f.req(mods...))
			require.NoError(t, err)
			assert.Equal(t, tc.want.rule, q.Rule)
			assert.Equal(t, money.FromInt(tc.want.price).String(), q.UnitPrice.String())
			if tc.check != nil {
				tc.check(t, f, q)
			}
		})
	}
}

func TestResolverNoPriceAndValidation(t *testing.T) {
	f := newFixture()
	f.src.base = nil
	_, err := f.resolver().Resolve(context.Background(), f.req())
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))

	_, err = f.resolver().Resolve(context.Background(), domain.PriceRequest{})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))

	// Tanpa pembaca hak harga (modul party tidak aktif) resolver tetap berjalan tanpa harga karyawan/PKS.
	r := pricing.NewResolver(f.src, nil, pricing.DefaultRules(f.src)...)
	f.src.base = &f.base
	q, err := r.Resolve(context.Background(), f.req(func(pr *domain.PriceRequest) { pr.CustomerID, pr.PartnerID = &f.employee, &f.partner }))
	require.NoError(t, err)
	assert.Equal(t, "42000.00", q.UnitPrice.String())
}

func TestResolverErrors(t *testing.T) {
	ops := []string{"special", "promo", "tier", "scope", "byid", "base", "list"}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			f := newFixture()
			f.src.fail = op
			f.src.offers = []domain.PromotionOffer{{PromotionID: uuid.New(), Item: domain.PromotionItem{DiscountPercent: ptr(decimal.NewFromInt(5))}}}
			_, err := f.resolver().Resolve(context.Background(), f.req(func(r *domain.PriceRequest) {
				if op != "base" {
					r.CustomerID, r.PartnerID, r.ChannelID = &f.customer, &f.partner, &f.channel
				}
			}))
			assert.ErrorIs(t, err, errSource)
		})
	}
	for _, op := range []string{"employee", "agreement"} {
		t.Run(op, func(t *testing.T) {
			f := newFixture()
			f.elig.fail = op
			_, err := f.resolver().Resolve(context.Background(), f.req(func(r *domain.PriceRequest) {
				r.CustomerID, r.PartnerID = &f.customer, &f.partner
			}))
			assert.ErrorIs(t, err, errSource)
		})
	}
}

func TestDefaultRulesOrder(t *testing.T) {
	rules := pricing.DefaultRules(newSource())
	names := make([]domain.Rule, len(rules))
	for i, r := range rules {
		names[i] = r.Name()
	}
	assert.Equal(t, []domain.Rule{
		domain.RuleSpecialPrice, domain.RulePromotion, domain.RuleTieredPrice, domain.RulePriceList, domain.RuleBasePrice,
	}, names, "urutan wajib sama dengan BRD 5.18")

	var q pricing.PriceQuery
	_, ok, err := q.Next(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
}
