// Package pricing adalah API publik modul harga (BRD 5.18; MD-04, MD-05, MD-11, MD-12): daftar harga per
// channel/mitra/karyawan/vending dengan versi berlaku yang disetujui, harga khusus, promo, harga bertingkat,
// resolver harga 5 aturan (Technical Guideline 5.6), dan batas diskon manual per role. Modul dokumen penjualan
// memanggil Resolve untuk setiap baris dan menyimpan PriceQuote.Rule serta PriceQuote.SourceID/versi di barisnya;
// CheckDiscount menentukan apakah dokumen butuh approval sales_order_discount.
package pricing

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/pricing/domain"
	pricinghttp "balans/internal/pricing/http"
	"balans/internal/pricing/postgres"
	"balans/internal/pricing/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "pricing"

type (
	// PriceRequest adalah permintaan harga satu baris transaksi (tanggal = business_date transaksi).
	PriceRequest = domain.PriceRequest
	// PriceQuote adalah harga hasil resolver beserta aturan dan versi yang dipakai.
	PriceQuote = domain.PriceQuote
	// Rule adalah aturan harga yang disimpan sebagai `price_rule`.
	Rule = domain.Rule
	// Discount adalah diskon manual persen atau nominal.
	Discount = domain.Discount
	// DiscountKind adalah bentuk diskon manual.
	DiscountKind = domain.DiscountKind
	// LineDiscount adalah diskon manual satu baris (Gross = qty × harga satuan).
	LineDiscount = domain.LineDiscount
	// DocumentDiscount adalah diskon manual per dokumen (Subtotal = total baris setelah diskon baris).
	DocumentDiscount = domain.DocumentDiscount
	// DiscountCheck adalah hasil pemeriksaan diskon terhadap batas role.
	DiscountCheck = domain.DiscountCheck
	// DiscountLimit adalah batas diskon efektif (terbesar dari role-role user).
	DiscountLimit = domain.Limit

	// ProductRef adalah ringkasan produk dari modul catalog.
	ProductRef = service.ProductRef
	// Products membaca produk dari modul catalog.
	Products = service.Products
	// PartyRef adalah ringkasan pelanggan/mitra dari modul party.
	PartyRef = service.PartyRef
	// Parties membaca pelanggan dan mitra dari modul party.
	Parties = service.Parties
	// Roles membaca nama role dari modul identity.
	Roles = service.Roles
	// Approvals mengajukan dan membatalkan approval (modul approval).
	Approvals = service.Approvals
)

// Aturan harga, urut prioritas BRD 5.18.
const (
	RuleSpecialPrice = domain.RuleSpecialPrice
	RulePromotion    = domain.RulePromotion
	RuleTieredPrice  = domain.RuleTieredPrice
	RulePriceList    = domain.RulePriceList
	RuleBasePrice    = domain.RuleBasePrice

	DiscountPercent = domain.DiscountPercent
	DiscountAmount  = domain.DiscountAmount
)

// Permission modul.
const (
	PermPriceListRead     = service.PermPriceListRead
	PermPromotionRead     = service.PermPromotionRead
	PermSpecialPriceRead  = service.PermSpecialPriceRead
	PermDiscountLimitRead = service.PermDiscountLimitRead
)

// Deps adalah dependensi modul. Products, Parties, Roles, dan Eligibility opsional (nil = tanpa validasi/nama
// dan tanpa harga karyawan/PKS); Approvals wajib untuk mengajukan versi daftar harga.
type Deps struct {
	Pool        *pgxpool.Pool
	Tx          *db.TxManager
	Clock       clock.Clock
	Responder   *httpx.Responder
	Audit       audit.Recorder
	Products    Products
	Parties     Parties
	Roles       Roles
	Eligibility Eligibility
	Approvals   Approvals
}

// Module adalah modul pricing.
type Module struct {
	svc      *service.Service
	resolver *Resolver
	rs       *httpx.Responder
	h        *pricinghttp.PricingHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Products: d.Products,
		Parties: d.Parties, Roles: d.Roles, Approvals: d.Approvals,
	})
	return newModule(svc, d.Eligibility, d.Responder)
}

func newModule(svc *service.Service, eligibility Eligibility, rs *httpx.Responder) *Module {
	m := &Module{svc: svc, resolver: NewResolver(svc, eligibility, DefaultRules(svc)...), rs: rs}
	m.h = pricinghttp.NewHandler(svc, simulator{m: m})
	return m
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route daftar harga, aturan harga, batas diskon, dan simulasi.
func (m *Module) RegisterRoutes(r chi.Router) { pricinghttp.Register(r, m.h, m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat.
func AuditEntities() map[string]string {
	return map[string]string{
		service.EntityPriceList: PermPriceListRead, service.EntityPriceListVersion: PermPriceListRead,
		service.EntitySpecialPrice: PermSpecialPriceRead, service.EntityPromotion: PermPromotionRead,
		service.EntityTieredPrice: PermPromotionRead, service.EntityDiscountLimit: PermDiscountLimitRead,
	}
}

// ApprovalHandler adalah handler approval price_list_version: versi diberlakukan saat disetujui level terakhir
// dan ditandai ditolak saat ditolak. Didaftarkan saat wiring dengan approval.Module.RegisterHandler.
func (m *Module) ApprovalHandler() approval.Handler {
	return approval.Handler{OnApproved: m.svc.OnVersionApproved, OnRejected: m.svc.OnVersionRejected}
}

// Resolve menentukan harga satu baris transaksi dengan rantai aturan BRD 5.18 (harga khusus → promo → harga
// bertingkat → daftar harga channel/mitra/karyawan → Harga Dasar & HET) pada req.Date. Tidak memeriksa
// permission; NOT_FOUND bila produk belum punya harga.
func (m *Module) Resolve(ctx context.Context, req PriceRequest) (PriceQuote, error) {
	return m.resolver.Resolve(ctx, req)
}

// EffectiveDiscountLimit mengembalikan batas diskon hari ini untuk role-role user (nilai terbesar antar role).
func (m *Module) EffectiveDiscountLimit(ctx context.Context, roleCodes []string) (DiscountLimit, error) {
	return m.svc.EffectiveLimit(ctx, roleCodes)
}

// CheckDiscount memeriksa diskon manual per baris dan per dokumen terhadap batas diskon role-role pembuat yang
// berlaku hari ini (MD-12). DiscountCheck.RequiresApproval = true berarti dokumen harus diajukan dengan approval
// sales_order_discount (Head Sales); nominalnya DiscountCheck.TotalDiscountAmount. Tidak memeriksa permission.
func (m *Module) CheckDiscount(ctx context.Context, roleCodes []string, lines []LineDiscount, doc DocumentDiscount) (DiscountCheck, error) {
	limit, err := m.svc.EffectiveLimit(ctx, roleCodes)
	if err != nil {
		return DiscountCheck{}, err
	}
	return domain.CheckDiscount(limit, lines, doc)
}

// ApprovalSubjectDiscount adalah jenis approval untuk diskon di atas batas role.
const ApprovalSubjectDiscount = approval.SubjectSalesOrderDiscount

// simulator melayani halaman Penjualan › Harga & Diskon.
type simulator struct{ m *Module }

func (s simulator) QuotePrice(ctx context.Context, req PriceRequest) (PriceQuote, error) {
	if _, err := actor.RequireAny(ctx, service.PermPriceListRead, service.PermPromotionRead, service.PermSpecialPriceRead); err != nil {
		return PriceQuote{}, err
	}
	return s.m.Resolve(ctx, req)
}

// CheckDiscountForActor memeriksa diskon terhadap role user yang login; subtotal dokumen = bruto − diskon baris.
func (s simulator) CheckDiscountForActor(ctx context.Context, lines []LineDiscount, doc Discount) (DiscountCheck, money.Amount, error) {
	a, err := actor.RequireAny(ctx, service.PermDiscountLimitRead, service.PermPriceListRead)
	if err != nil {
		return DiscountCheck{}, money.Zero, err
	}
	subtotal := money.Zero
	for _, l := range lines {
		amount, _, err := l.Discount.Apply(l.Gross)
		if err != nil {
			amount = money.Zero
		}
		subtotal = subtotal.Add(l.Gross.Sub(amount))
	}
	check, err := s.m.CheckDiscount(ctx, a.Roles, lines, DocumentDiscount{Subtotal: subtotal, Discount: doc})
	return check, subtotal, err
}
