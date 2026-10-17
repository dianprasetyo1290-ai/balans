package service_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
	"balans/internal/pricing/service"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// approvals adalah pengganti modul approval yang mencatat pengajuan.
type approvals struct {
	mu        sync.Mutex
	requested []approval.Subject
	cancelled []uuid.UUID
}

func (a *approvals) Request(_ context.Context, s approval.Subject) (approval.Request, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requested = append(a.requested, s)
	return approval.Request{}, nil
}

func (a *approvals) Cancel(_ context.Context, _ approval.SubjectType, id uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancelled = append(a.cancelled, id)
	return nil
}

type products struct{ unusable map[uuid.UUID]string }

func (p products) Products(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]service.ProductRef, error) {
	out := map[uuid.UUID]service.ProductRef{}
	for _, id := range ids {
		out[id] = service.ProductRef{ID: id, SKU: "SKU-" + id.String()[:4], Name: "Produk", Unusable: p.unusable[id]}
	}
	return out, nil
}

type parties struct {
	known map[uuid.UUID]service.PartyRef
}

func (p parties) Customers(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]service.PartyRef, error) {
	return p.pick(ids), nil
}

func (p parties) Partners(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]service.PartyRef, error) {
	return p.pick(ids), nil
}

func (p parties) pick(ids []uuid.UUID) map[uuid.UUID]service.PartyRef {
	out := map[uuid.UUID]service.PartyRef{}
	for _, id := range ids {
		if r, ok := p.known[id]; ok {
			out[id] = r
		}
	}
	return out
}

type roles struct{}

func (roles) RoleName(_ context.Context, code string) (string, bool, error) {
	names := map[string]string{"sales_mt": "Sales MT", "head_sales": "Head Sales", "sales_ecommerce": "Sales E-commerce", "customer_service": "CS"}
	n, ok := names[code]
	return n, ok, nil
}

type harness struct {
	t         *testing.T
	pool      *pgxpool.Pool
	svc       *service.Service
	approvals *approvals
	products  products
	parties   parties
	today     clock.BusinessDate
	clock     *clock.Fixed
	actorID   uuid.UUID
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	// Jumat 23 Okt 2026 09.00 WIB.
	fixed := clock.NewFixed(time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC))
	h := &harness{
		t: t, pool: pool, approvals: &approvals{}, products: products{unusable: map[uuid.UUID]string{}},
		parties: parties{known: map[uuid.UUID]service.PartyRef{}}, today: clock.Today(fixed), clock: fixed, actorID: uuid.New(),
	}
	h.svc = service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}), Clock: fixed,
		Products: h.products, Parties: h.parties, Roles: roles{}, Approvals: h.approvals,
	})
	return h
}

var allPerms = []string{
	service.PermPriceListRead, service.PermPriceListWrite, service.PermPriceListSubmit, service.PermPriceListApprove,
	service.PermPromotionRead, service.PermPromotionWrite, service.PermSpecialPriceRead, service.PermSpecialPriceWrite,
	service.PermDiscountLimitRead, service.PermDiscountLimitWrite,
}

func (h *harness) ctx(perms ...string) context.Context {
	if perms == nil {
		perms = allPerms
	}
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: h.actorID, FullName: "Penguji", Permissions: set})
}

func (h *harness) adminCtx() context.Context {
	set := map[string]struct{}{}
	for _, p := range allPerms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: h.actorID, Roles: []string{"system_admin"}, Permissions: set})
}

func (h *harness) day(n int) clock.BusinessDate { return h.today.AddDays(n) }

func code(err error) apperr.Code { return apperr.As(err).Code }

func uniqueCode(prefix string) string { return strings.ToUpper(prefix + "-" + uuid.NewString()[:8]) }

func (h *harness) channelList(name string) domain.PriceList {
	h.t.Helper()
	ch := uuid.New()
	l, err := h.svc.CreatePriceList(h.ctx(), domain.PriceListInput{
		Code: uniqueCode("CH"), Name: name, Type: domain.TypeChannel, ChannelID: &ch, IsActive: true,
	})
	require.NoError(h.t, err)
	return l
}

// activate mengajukan dan menyetujui versi seperti engine approval.
func (h *harness) activate(v domain.PriceListVersion) domain.PriceListVersion {
	h.t.Helper()
	v, err := h.svc.SubmitVersion(h.ctx(), v.ID, v.Version)
	require.NoError(h.t, err)
	err = db.NewTxManager(h.pool).WithTx(h.ctx(), func(ctx context.Context) error {
		return h.svc.OnVersionApproved(ctx, approval.Decided{Subject: approval.Subject{ID: v.ID}, DecidedBy: uuid.New()})
	})
	require.NoError(h.t, err)
	d, err := h.svc.GetVersion(h.ctx(), v.ID)
	require.NoError(h.t, err)
	return d.PriceListVersion
}

func (h *harness) auditActions(entityID uuid.UUID) []string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `SELECT action FROM audit_logs WHERE entity_id = $1 ORDER BY created_at, id`, entityID)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(h.t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(h.t, rows.Err())
	return out
}

func TestPriceListCRUD(t *testing.T) {
	h := setup(t)
	ch := uuid.New()
	in := domain.PriceListInput{Code: uniqueCode("mt"), Name: "Modern Trade", Type: domain.TypeChannel, ChannelID: &ch, IsActive: true}
	l, err := h.svc.CreatePriceList(h.ctx(), in)
	require.NoError(t, err)

	_, err = h.svc.CreatePriceList(h.ctx(), in)
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))
	dup := in
	dup.Code = uniqueCode("mt")
	_, err = h.svc.CreatePriceList(h.ctx(), dup)
	require.Equal(t, apperr.CodeValidationFailed, code(err))
	assert.Contains(t, apperr.As(err).Fields, "priceListType", "satu daftar harga per cakupan")

	_, err = h.svc.CreatePriceList(h.ctx(service.PermPriceListRead), dup)
	assert.Equal(t, apperr.CodeForbidden, code(err))

	partnerIn := domain.PriceListInput{Code: uniqueCode("p"), Name: "Mitra", Type: domain.TypePartner, PartnerID: ptr(uuid.New()), IsActive: true}
	_, err = h.svc.CreatePriceList(h.ctx(), partnerIn)
	assert.Contains(t, apperr.As(err).Fields, "partnerId", "mitra harus dikenal modul party")

	ref := uuid.New()
	_, err = h.svc.CreatePriceList(h.ctx(), domain.PriceListInput{
		Code: uniqueCode("pc"), Name: "Persen", Type: domain.TypeVending, ChannelID: ptr(uuid.New()), Basis: domain.BasisPercent, ReferenceID: &ref, IsActive: true,
	})
	assert.Contains(t, apperr.As(err).Fields, "referencePriceListId")

	upd := in
	upd.Name = "Modern Trade Beli Putus"
	l, err = h.svc.UpdatePriceList(h.ctx(), l.ID, l.Version, upd)
	require.NoError(t, err)
	assert.Equal(t, "Modern Trade Beli Putus", l.Name)
	_, err = h.svc.UpdatePriceList(h.ctx(), l.ID, l.Version-1, upd)
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	list, err := h.svc.ListPriceLists(h.ctx(service.PermPriceListApprove), postgres.PriceListFilter{Query: l.Code})
	require.NoError(t, err)
	require.Len(t, list, 1)

	base, err := h.svc.ListPriceLists(h.ctx(), postgres.PriceListFilter{Type: domain.TypeBase})
	require.NoError(t, err)
	require.Len(t, base, 1, "Harga Dasar & HET disemai migrasi")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeletePriceList(h.ctx(), base[0].ID, base[0].Version)))

	d, err := h.svc.GetPriceList(h.ctx(), l.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"update", "create_version", "delete"}, d.AllowedActions)

	require.NoError(t, h.svc.DeletePriceList(h.ctx(), l.ID, l.Version))
	deleted, err := h.svc.ListPriceLists(h.ctx(), postgres.PriceListFilter{Query: l.Code, Deleted: true})
	require.NoError(t, err)
	require.Len(t, deleted, 1)
	d, err = h.svc.GetPriceList(h.adminCtx(), l.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"restore"}, d.AllowedActions)

	_, err = h.svc.RestorePriceList(h.ctx(), l.ID, deleted[0].Version)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	l, err = h.svc.RestorePriceList(h.adminCtx(), l.ID, deleted[0].Version)
	require.NoError(t, err)
	assert.False(t, l.Deleted())
	assert.Equal(t, []string{"create", "update", "delete", "restore"}, h.auditActions(l.ID))
}

func TestPriceListVersionApprovalFlow(t *testing.T) {
	h := setup(t)
	l := h.channelList("Website")
	p1, p2 := uuid.New(), uuid.New()

	v1, err := h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{
		ValidFrom: h.day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "48000"}, {ProductID: p2, Price: "30000"}},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, v1.No)

	_, err = h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{ValidFrom: h.day(0)}, nil)
	require.Equal(t, apperr.CodeValidationFailed, code(err), "hanya satu versi draft per daftar harga")

	h.products.unusable[p2] = "Produk nonaktif"
	_, err = h.svc.SubmitVersion(h.ctx(), v1.ID, v1.Version)
	assert.Equal(t, "Produk nonaktif", apperr.As(err).Fields["items.1.productId"])
	delete(h.products.unusable, p2)

	_, err = h.svc.SubmitVersion(h.ctx(service.PermPriceListWrite), v1.ID, v1.Version)
	assert.Equal(t, apperr.CodeForbidden, code(err))

	v1 = h.activate(v1)
	assert.Equal(t, domain.VersionActive, v1.Status)
	require.NotEmpty(t, h.approvals.requested)
	s := h.approvals.requested[len(h.approvals.requested)-1]
	assert.Equal(t, approval.SubjectPriceListVersion, s.Type)
	assert.Equal(t, l.Code+" v1", s.Label)
	assert.Equal(t, "/master/price-lists/"+l.ID.String(), s.Link)

	lp, ok, err := h.svc.ListPriceOn(context.Background(), l, p1, h.day(0))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "48000.00", lp.Price.String())
	assert.Equal(t, v1.ID, lp.VersionID)
	_, ok, err = h.svc.ListPriceOn(context.Background(), l, uuid.New(), h.day(0))
	require.NoError(t, err)
	assert.False(t, ok, "produk yang tidak ada di versi")

	// Versi 2 (salin dari v1, ubah satu harga) berlaku 1 Nov; v1 ditutup 31 Okt.
	v2, err := h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{ValidFrom: h.day(9), Notes: "Kenaikan"}, &v1.ID)
	require.NoError(t, err)
	assert.Len(t, v2.Items, 2)
	v2, err = h.svc.UpdateVersion(h.ctx(), v2.ID, v2.Version, domain.VersionInput{
		ValidFrom: h.day(9), Items: []domain.ItemInput{{ProductID: p1, Price: "50000"}, {ProductID: p2, Price: "30000"}},
	})
	require.NoError(t, err)

	views, err := h.svc.ListPriceLists(h.ctx(), postgres.PriceListFilter{Query: l.Code})
	require.NoError(t, err)
	require.NotNil(t, views[0].Current)
	require.NotNil(t, views[0].Open)
	assert.Equal(t, 2, views[0].Open.No)

	v2 = h.activate(v2)
	v1d, err := h.svc.GetVersion(h.ctx(), v1.ID)
	require.NoError(t, err)
	require.NotNil(t, v1d.ValidTo)
	assert.Equal(t, h.day(8), *v1d.ValidTo, "versi lama ditutup sehari sebelum versi baru")

	// Transaksi lama (tanggal sebelum versi 2) tetap memakai harga versi 1.
	old, _, err := h.svc.ListPriceOn(context.Background(), l, p1, h.day(8))
	require.NoError(t, err)
	assert.Equal(t, "48000.00", old.Price.String())
	assert.Equal(t, v1.ID, old.VersionID)
	cur, _, err := h.svc.ListPriceOn(context.Background(), l, p1, h.day(9))
	require.NoError(t, err)
	assert.Equal(t, "50000.00", cur.Price.String())
	assert.Equal(t, v2.ID, cur.VersionID)

	views, err = h.svc.ListPriceLists(h.ctx(), postgres.PriceListFilter{Query: l.Code})
	require.NoError(t, err)
	assert.Equal(t, 1, views[0].Current.No)
	require.NotNil(t, views[0].Next)
	assert.Equal(t, 2, views[0].Next.No)

	// Versi 3 diajukan lalu ditarik, diajukan lagi lalu ditolak.
	v3, err := h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{ValidFrom: h.day(20), Items: []domain.ItemInput{{ProductID: p1, Price: "1"}}}, nil)
	require.NoError(t, err)
	v3, err = h.svc.SubmitVersion(h.ctx(), v3.ID, v3.Version)
	require.NoError(t, err)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeletePriceList(h.ctx(), l.ID, views[0].Version)),
		"daftar dengan versi menunggu persetujuan tidak bisa dihapus")
	d, err := h.svc.GetVersion(h.ctx(), v3.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"withdraw"}, d.AllowedActions)
	v3, err = h.svc.WithdrawVersion(h.ctx(), v3.ID, v3.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.VersionDraft, v3.Status)
	assert.Contains(t, h.approvals.cancelled, v3.ID)
	v3, err = h.svc.SubmitVersion(h.ctx(), v3.ID, v3.Version)
	require.NoError(t, err)
	err = db.NewTxManager(h.pool).WithTx(h.ctx(), func(ctx context.Context) error {
		return h.svc.OnVersionRejected(ctx, approval.Decided{Subject: approval.Subject{ID: v3.ID}, DecidedBy: uuid.New(), Note: "Terlalu murah"})
	})
	require.NoError(t, err)
	d, err = h.svc.GetVersion(h.ctx(), v3.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.VersionRejected, d.Status)

	v4, err := h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{ValidFrom: h.day(1)}, nil)
	require.NoError(t, err)
	v4, err = h.svc.CancelVersion(h.ctx(), v4.ID, v4.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.VersionCancelled, v4.Status)

	detail, err := h.svc.GetPriceList(h.ctx(), l.ID)
	require.NoError(t, err)
	assert.Len(t, detail.Versions, 4)
	assert.Equal(t, []string{"create", "submit", "approve"}, h.auditActions(v1.ID)[:3])

	// Versi baru yang mulai sebelum versi aktif terjadwal ditolak saat disetujui.
	v5, err := h.svc.CreateVersion(h.ctx(), l.ID, domain.VersionInput{ValidFrom: h.day(5), Items: []domain.ItemInput{{ProductID: p1, Price: "2"}}}, nil)
	require.NoError(t, err)
	v5, err = h.svc.SubmitVersion(h.ctx(), v5.ID, v5.Version)
	require.NoError(t, err)
	err = db.NewTxManager(h.pool).WithTx(h.ctx(), func(ctx context.Context) error {
		return h.svc.OnVersionApproved(ctx, approval.Decided{Subject: approval.Subject{ID: v5.ID}, DecidedBy: uuid.New()})
	})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
}

func TestPercentPriceList(t *testing.T) {
	h := setup(t)
	web := h.channelList("Website")
	p := uuid.New()
	h.activate(must(h.svc.CreateVersion(h.ctx(), web.ID, domain.VersionInput{ValidFrom: h.day(0), Items: []domain.ItemInput{{ProductID: p, Price: "45500"}}}, nil)))

	pct, err := h.svc.CreatePriceList(h.ctx(), domain.PriceListInput{
		Code: uniqueCode("v"), Name: "Vending 90%", Type: domain.TypeVending, ChannelID: ptr(uuid.New()), Basis: domain.BasisPercent,
		ReferenceID: &web.ID, IsActive: true,
	})
	require.NoError(t, err)
	_, err = h.svc.CreatePriceList(h.ctx(), domain.PriceListInput{
		Code: uniqueCode("v"), Name: "Rantai", Type: domain.TypeVending, ChannelID: ptr(uuid.New()), Basis: domain.BasisPercent,
		ReferenceID: &pct.ID, IsActive: true,
	})
	assert.Contains(t, apperr.As(err).Fields, "referencePriceListId", "acuan harus berbasis harga per produk")

	_, ok, err := h.svc.ListPriceOn(context.Background(), pct, p, h.day(0))
	require.NoError(t, err)
	assert.False(t, ok, "belum ada versi aktif")
	v := h.activate(must(h.svc.CreateVersion(h.ctx(), pct.ID, domain.VersionInput{ValidFrom: h.day(0), Percent: "90"}, nil)))
	lp, ok, err := h.svc.ListPriceOn(context.Background(), pct, p, h.day(0))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "40950.00", lp.Price.String())
	assert.Equal(t, v.ID, lp.VersionID)
	_, ok, err = h.svc.ListPriceOn(context.Background(), pct, uuid.New(), h.day(0))
	require.NoError(t, err)
	assert.False(t, ok)

	upd := domain.PriceListInput{Name: "Vending", ChannelID: pct.ChannelID, IsActive: true}
	_, err = h.svc.UpdatePriceList(h.ctx(), pct.ID, pct.Version, upd)
	assert.Contains(t, apperr.As(err).Fields, "basis", "basis tetap setelah ada versi aktif")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeletePriceList(h.ctx(), web.ID, web.Version)),
		"acuan daftar persen tidak bisa dihapus")
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }

func TestSpecialPrices(t *testing.T) {
	h := setup(t)
	customer, product := uuid.New(), uuid.New()
	h.parties.known[customer] = service.PartyRef{ID: customer, Code: "PLG-000001", Name: "Siti"}
	in := domain.SpecialPriceInput{PartyType: domain.PartyCustomer, PartyID: customer, ProductID: product, Price: "41000", ValidFrom: h.day(0)}
	first, err := h.svc.CreateSpecialPrice(h.ctx(), in)
	require.NoError(t, err)

	_, err = h.svc.CreateSpecialPrice(h.ctx(), domain.SpecialPriceInput{PartyType: domain.PartyPartner, PartyID: uuid.New(), ProductID: product, Price: "1", ValidFrom: h.day(0)})
	assert.Contains(t, apperr.As(err).Fields, "partyId")

	_, err = h.svc.CreateSpecialPrice(h.ctx(), in)
	assert.Contains(t, apperr.As(err).Fields, "validFrom", "mulai bersamaan dengan harga lama tidak bisa ditutup otomatis")

	next := in
	next.Price, next.ValidFrom = "40000", h.day(10)
	second, err := h.svc.CreateSpecialPrice(h.ctx(), next)
	require.NoError(t, err)

	list, err := h.svc.ListSpecialPrices(h.ctx(), postgres.SpecialPriceFilter{PartyID: &customer})
	require.NoError(t, err)
	require.Len(t, list, 2)
	byID := map[uuid.UUID]service.SpecialPriceView{}
	for _, v := range list {
		byID[v.ID] = v
	}
	assert.Equal(t, h.day(9), *byID[first.ID].ValidTo, "harga lama ditutup otomatis")
	assert.Equal(t, "Siti", byID[first.ID].PartyName)
	assert.Equal(t, domain.PeriodActive, byID[first.ID].Status)
	assert.Equal(t, domain.PeriodScheduled, byID[second.ID].Status)

	sp, ok, err := h.svc.SpecialPriceOn(context.Background(), domain.PartyCustomer, customer, product, h.day(10))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, second.ID, sp.ID)

	h.clock.Advance(48 * time.Hour)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteSpecialPrice(h.ctx(), first.ID, byID[first.ID].Version)),
		"harga yang sudah berlaku tidak dihapus")
	ended, err := h.svc.EndSpecialPrice(h.ctx(), first.ID, byID[first.ID].Version, h.day(3))
	require.NoError(t, err)
	assert.Equal(t, h.day(3), *ended.ValidTo)
	require.NoError(t, h.svc.DeleteSpecialPrice(h.ctx(), second.ID, second.Version))
	_, err = h.svc.EndSpecialPrice(h.ctx(), second.ID, second.Version+1, h.day(30))
	assert.Error(t, err, "data terhapus tidak bisa diakhiri")

	_, err = h.svc.ListSpecialPrices(h.ctx(service.PermPriceListRead), postgres.SpecialPriceFilter{})
	assert.Equal(t, apperr.CodeForbidden, code(err))
}

func TestPromotions(t *testing.T) {
	h := setup(t)
	ch, p1, p2 := uuid.New(), uuid.New(), uuid.New()
	in := domain.PromotionInput{
		Code: uniqueCode("promo"), Name: "11.11", ChannelID: &ch, ValidFrom: h.day(0), ValidTo: h.day(5), IsActive: true,
		Items: []domain.PromotionItemInput{{ProductID: p1, Price: "39000"}, {ProductID: p2, DiscountPercent: "10"}},
	}
	p, err := h.svc.CreatePromotion(h.ctx(), in)
	require.NoError(t, err)
	_, err = h.svc.CreatePromotion(h.ctx(), in)
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))

	offers, err := h.svc.PromotionOffersOn(context.Background(), p2, &ch, nil, h.day(1))
	require.NoError(t, err)
	require.Len(t, offers, 1)
	assert.True(t, offers[0].Item.DiscountPercent.Equal(decimal.NewFromInt(10)))
	none, err := h.svc.PromotionOffersOn(context.Background(), p2, ptr(uuid.New()), nil, h.day(1))
	require.NoError(t, err)
	assert.Empty(t, none, "promo per channel tidak berlaku di channel lain")

	v, err := h.svc.GetPromotion(h.ctx(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodActive, v.Status)
	assert.Equal(t, []string{"update"}, v.AllowedActions, "promo berjalan tidak bisa dihapus")
	assert.Len(t, v.Items, 2)

	upd := in
	upd.Name, upd.ValidTo, upd.Items = "11.11 diperpanjang", h.day(7), nil
	p, err = h.svc.UpdatePromotion(h.ctx(), p.ID, p.Version, upd)
	require.NoError(t, err)
	assert.Equal(t, h.day(7), p.ValidTo)
	assert.Len(t, p.Items, 2, "item promo berjalan tidak berubah")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeletePromotion(h.ctx(), p.ID, p.Version)))

	future := in
	future.Code, future.ValidFrom, future.ValidTo = uniqueCode("promo"), h.day(30), h.day(31)
	f, err := h.svc.CreatePromotion(h.ctx(), future)
	require.NoError(t, err)
	future.Items = []domain.PromotionItemInput{{ProductID: p1, DiscountPercent: "5"}}
	f, err = h.svc.UpdatePromotion(h.ctx(), f.ID, f.Version, future)
	require.NoError(t, err)
	assert.Len(t, f.Items, 1)

	list, err := h.svc.ListPromotions(h.ctx(), postgres.PromotionFilter{ChannelID: &ch})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, 1, list[0].ItemCount)
	require.NoError(t, h.svc.DeletePromotion(h.ctx(), f.ID, f.Version))
	list, err = h.svc.ListPromotions(h.ctx(), postgres.PromotionFilter{ChannelID: &ch})
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestTieredPrices(t *testing.T) {
	h := setup(t)
	l := h.channelList("B2B")
	product := uuid.New()
	in := domain.TieredPriceInput{PriceListID: &l.ID, ProductID: product, MinQty: "12", Price: "45000", ValidFrom: h.day(0)}
	first, err := h.svc.CreateTieredPrice(h.ctx(), in)
	require.NoError(t, err)
	bigger := in
	bigger.MinQty, bigger.Price = "50", "43000"
	_, err = h.svc.CreateTieredPrice(h.ctx(), bigger)
	require.NoError(t, err)
	global := in
	global.PriceListID, global.Price = nil, "46000"
	_, err = h.svc.CreateTieredPrice(h.ctx(), global)
	require.NoError(t, err)

	_, err = h.svc.CreateTieredPrice(h.ctx(), domain.TieredPriceInput{PriceListID: ptr(uuid.New()), ProductID: product, MinQty: "1", Price: "1", ValidFrom: h.day(0)})
	assert.Contains(t, apperr.As(err).Fields, "priceListId")

	tiers, err := h.svc.TieredPricesOn(context.Background(), product, []uuid.UUID{l.ID}, decimal.NewFromInt(60), h.day(0))
	require.NoError(t, err)
	assert.Len(t, tiers, 3)
	tiers, err = h.svc.TieredPricesOn(context.Background(), product, nil, decimal.NewFromInt(20), h.day(0))
	require.NoError(t, err)
	require.Len(t, tiers, 1, "tanpa daftar kandidat hanya tingkat umum")

	change := in
	change.Price, change.ValidFrom = "44000", h.day(14)
	_, err = h.svc.CreateTieredPrice(h.ctx(), change)
	require.NoError(t, err)

	list, err := h.svc.ListTieredPrices(h.ctx(), postgres.TieredPriceFilter{ProductID: &product})
	require.NoError(t, err)
	require.Len(t, list, 4)
	h.clock.Advance(48 * time.Hour)
	for _, v := range list {
		if v.ID == first.ID {
			assert.Equal(t, h.day(13), *v.ValidTo)
			assert.Equal(t, l.Code, v.PriceListCode)
			_, err := h.svc.EndTieredPrice(h.ctx(), v.ID, v.Version, h.day(2))
			require.NoError(t, err)
			assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteTieredPrice(h.ctx(), v.ID, v.Version+1)))
		}
		if v.Status == domain.PeriodScheduled {
			require.NoError(t, h.svc.DeleteTieredPrice(h.ctx(), v.ID, v.Version))
		}
	}
}

func TestDiscountLimits(t *testing.T) {
	h := setup(t)
	// Role uji unik supaya tidak bertabrakan dengan data test lain di database bersama.
	role := "sales_mt"
	_, err := h.pool.Exec(context.Background(), `DELETE FROM discount_limits WHERE role_code = $1`, role)
	require.NoError(t, err)

	first, err := h.svc.CreateDiscountLimit(h.ctx(), domain.DiscountLimitInput{RoleCode: role, MaxLinePercent: "5", MaxDocumentPercent: "2", ValidFrom: h.day(0)})
	require.NoError(t, err)
	_, err = h.svc.CreateDiscountLimit(h.ctx(), domain.DiscountLimitInput{RoleCode: "nope", MaxLinePercent: "5", MaxDocumentPercent: "2", ValidFrom: h.day(0)})
	assert.Contains(t, apperr.As(err).Fields, "roleCode")
	_, err = h.svc.CreateDiscountLimit(h.ctx(service.PermDiscountLimitRead), domain.DiscountLimitInput{RoleCode: role})
	assert.Equal(t, apperr.CodeForbidden, code(err))

	second, err := h.svc.CreateDiscountLimit(h.ctx(), domain.DiscountLimitInput{RoleCode: role, MaxLinePercent: "10", MaxDocumentPercent: "3", ValidFrom: h.day(7)})
	require.NoError(t, err)

	limit, err := h.svc.EffectiveLimit(context.Background(), []string{role, role, "customer_service"})
	require.NoError(t, err)
	assert.True(t, limit.MaxLinePercent.Equal(decimal.NewFromInt(5)), "batas hari ini masih batas pertama")

	views, err := h.svc.ListDiscountLimits(h.ctx(service.PermPriceListRead), false)
	require.NoError(t, err)
	var mine []service.DiscountLimitView
	for _, v := range views {
		if v.RoleCode == role {
			mine = append(mine, v)
		}
	}
	require.Len(t, mine, 2)
	assert.Equal(t, "Sales MT", mine[0].RoleName)
	assert.Equal(t, h.day(6), *mine[1].ValidTo, "batas lama ditutup sehari sebelum batas baru")

	roleOpts, err := h.svc.DiscountRoles(h.ctx())
	require.NoError(t, err)
	assert.Len(t, roleOpts, 4)

	h.clock.Advance(48 * time.Hour)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(h.svc.DeleteDiscountLimit(h.ctx(), first.ID, mine[1].Version)))
	_, err = h.svc.EndDiscountLimit(h.ctx(), first.ID, mine[1].Version, h.day(1))
	require.NoError(t, err)
	require.NoError(t, h.svc.DeleteDiscountLimit(h.ctx(), second.ID, second.Version))
	views, err = h.svc.ListDiscountLimits(h.ctx(), true)
	require.NoError(t, err)
	count := 0
	for _, v := range views {
		if v.RoleCode == role {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
