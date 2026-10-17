package service

import (
	"context"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/pricing"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Permission dokumen penjualan dan piutang (Desain Role & Approval bagian 3.3 – 3.4).
const (
	PermOrderRead         = "sales.sales_order.read"
	PermOrderWrite        = "sales.sales_order.write"
	PermOrderSubmit       = "sales.sales_order.submit"
	PermOrderCancel       = "sales.sales_order.cancel"
	PermMarketplaceRead   = "sales.marketplace_order.read"
	PermMarketplaceWrite  = "sales.marketplace_order.write"
	PermMarketplaceImport = "sales.marketplace_order.import"
	PermProformaRead      = "sales.proforma_invoice.read"
	PermProformaWrite     = "sales.proforma_invoice.write"
	PermInvoiceRead       = "sales.invoice.read"
	PermInvoicePost       = "sales.invoice.post"
	PermInvoiceCancel     = "sales.invoice.cancel"
	PermPaymentRead       = "accounting.payment.read"
	PermPaymentWrite      = "accounting.payment.write"
	PermPaymentPost       = "accounting.payment.post"
)

// Jenis entitas audit trail dokumen penjualan.
const (
	EntitySalesOrder = "sales_order"
	// EntityMarketplaceOrder: riwayat pesanan marketplace dibaca dengan sales.marketplace_order.read.
	EntityMarketplaceOrder = "marketplace_order"
	EntityProforma         = "proforma_invoice"
	EntityInvoice          = "invoice"
	EntityCustomerPayment  = "customer_payment"
)

// Posting rule dan jenis dokumen sumber jurnal penjualan (Desain Finance & Accounting 2.1).
const (
	RuleSales            = "A-05"
	RuleCOGS             = "A-06"
	RuleMarketplaceSales = "A-10"
	RulePayment          = "A-PB"

	SourceInvoice  = "sales_invoice"
	SourceShipment = "sales_shipment"
	SourcePayment  = "customer_payment"
	// SourceSalesOrder adalah source_type reservasi dan mutasi stok Sales Order di modul inventory.
	SourceSalesOrder = "sales_order"
)

func entityOf(t domain.OrderType) string {
	if t == domain.OrderMarketplace {
		return EntityMarketplaceOrder
	}
	return EntitySalesOrder
}

// Jenis pemilik file di filestore: bukti bayar pesanan yang diunggah CS/Sales dan bukti pembayaran pelanggan yang
// dicatat F&A.
const (
	FileOwnerPaymentProof         = "sales.payment_proof"
	FileOwnerCustomerPaymentProof = "sales.customer_payment_proof"
)

// DocumentType adalah jenis nomor dokumen yang diminta ke modul document.
type DocumentType string

const (
	DocSalesOrder DocumentType = "SO"
	DocProforma   DocumentType = "PI"
	DocInvoice    DocumentType = "INV"
	DocPayment    DocumentType = "BYR"
)

// ProductRef adalah produk katalog untuk validasi, alokasi bundel, dan tampilan.
type ProductRef struct {
	ID         uuid.UUID
	SKU        string
	Name       string
	UOM        string
	Type       string
	IsBundle   bool
	IsActive   bool
	Deleted    bool
	Components []domain.ComponentQty
}

// Label mengembalikan "SKU – nama".
func (p ProductRef) Label() string {
	if p.SKU == "" {
		return p.Name
	}
	return p.SKU + " – " + p.Name
}

// LocationRef adalah lokasi stok dari modul warehouse.
type LocationRef struct {
	ID       uuid.UUID
	Code     string
	Name     string
	Type     string
	IsActive bool
	Deleted  bool
}

// ChannelRef adalah channel atau sub-channel dimensi jurnal.
type ChannelRef struct {
	ID        uuid.UUID
	Kind      string
	Code      string
	Name      string
	ChannelID *uuid.UUID
	PartyType string
	PartyID   *uuid.UUID
}

// Option adalah pilihan kode–nama (akun Kas & Bank).
type Option struct {
	ID   uuid.UUID
	Code string
	Name string
}

// Catalog membaca produk katalog.
type Catalog interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error)
	ProductBySKU(ctx context.Context, sku string) (ProductRef, bool, error)
}

// Parties membaca pelanggan dan mitra (modul party); ok false bila tidak ada atau terhapus.
type Parties interface {
	Customer(ctx context.Context, id uuid.UUID) (domain.PartyInfo, bool, error)
	Partner(ctx context.Context, id uuid.UUID) (domain.PartyInfo, bool, error)
	EmployeeEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error)
}

// Locations membaca lokasi stok.
type Locations interface {
	Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]LocationRef, error)
	SaleLocations(ctx context.Context, types []string) ([]LocationRef, error)
}

// Pricing adalah resolver harga dan pemeriksaan batas diskon (modul pricing).
type Pricing interface {
	Resolve(ctx context.Context, req pricing.PriceRequest) (pricing.PriceQuote, error)
	CheckDiscount(ctx context.Context, roleCodes []string, lines []pricing.LineDiscount, doc pricing.DocumentDiscount) (pricing.DiscountCheck, error)
}

// Stock memutasi stok (modul inventory) di transaksi aktif.
type Stock interface {
	Post(ctx context.Context, p inventory.Posting) (inventory.PostingResult, error)
	Reserve(ctx context.Context, r inventory.ReserveRequest) (inventory.PostingResult, error)
	Release(ctx context.Context, r inventory.ReleaseRequest) (inventory.PostingResult, error)
	Batch(ctx context.Context, id uuid.UUID) (inventory.Batch, error)
}

// Ledger membaca dimensi & akun dan membuat jurnal otomatis (modul accounting).
type Ledger interface {
	Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error)
	Reverse(ctx context.Context, req accounting.ReversalRequest) (accounting.JournalEntry, error)
	Channels(ctx context.Context) ([]ChannelRef, error)
	ChannelLabels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ChannelRef, error)
	CashBankAccounts(ctx context.Context) ([]Option, error)
}

// Approvals mengajukan dan membatalkan approval diskon.
type Approvals interface {
	Request(ctx context.Context, s approval.Subject) (approval.Request, error)
	Cancel(ctx context.Context, t approval.SubjectType, subjectID uuid.UUID) error
}

// Numberer mengambil nomor dokumen dari modul document di transaksi aktif.
type Numberer interface {
	Next(ctx context.Context, t DocumentType, businessDate clock.BusinessDate) (string, error)
}

// Files menautkan file bukti bayar (modul filestore).
type Files interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
	Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Users membaca nama pengguna.
type Users interface {
	UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Deps adalah dependensi DocumentService.
type Deps struct {
	Store     *postgres.Store
	Tx        txRunner
	Audit     audit.Recorder
	Clock     clock.Clock
	Log       *slog.Logger
	Catalog   Catalog
	Parties   Parties
	Locations Locations
	Pricing   Pricing
	Stock     Stock
	Ledger    Ledger
	Approvals Approvals
	Numberer  Numberer
	Files     Files
	Users     Users
	// Konsinyasi (S-303 – S-305): lokasi mitra, transfer & stock count (warehouse), PKS (party), mutasi stok
	// lokasi (inventory). Nil = fitur konsinyasi menolak dengan PROVIDER_UNAVAILABLE.
	Consignment ConsignmentStock
	Agreements  Agreements
	Flows       StockFlows
	// ProformaValidDays adalah masa berlaku Proforma Invoice; 0 = 14 hari (BRD 5.4).
	ProformaValidDays int
	// BaseURL adalah URL ERP untuk tautan invoice di teks WhatsApp.
	BaseURL string
}

// DocumentService adalah use case Sales Order, Proforma Invoice, Invoice, dan piutang (S-301 – S-315).
type DocumentService struct {
	Deps
}

// NewDocumentService membuat DocumentService.
func NewDocumentService(d Deps) *DocumentService { return &DocumentService{Deps: d} }

func (s *DocumentService) today() clock.BusinessDate { return clock.Today(s.Clock) }

// Today adalah tanggal bisnis hari ini (WIB) menurut jam service.
func (s *DocumentService) Today() clock.BusinessDate { return s.today() }

// Path detail dokumen di ERP.
const (
	orderPathPrefix    = "/sales/orders/"
	proformaPathPrefix = "/sales/proforma-invoices/"
	invoicePathPrefix  = "/sales/invoices/"
)

// OrderPath adalah path detail Sales Order.
func OrderPath(id uuid.UUID) string { return orderPathPrefix + id.String() }

// ProformaPath adalah path detail Proforma Invoice.
func ProformaPath(id uuid.UUID) string { return proformaPathPrefix + id.String() }

// InvoicePath adalah path detail Invoice.
func InvoicePath(id uuid.UUID) string { return invoicePathPrefix + id.String() }

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func filterActions(ctx context.Context, actions []domain.Action, perms map[domain.Action][]string) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	for _, act := range actions {
		for _, p := range perms[act] {
			if a.Has(p) {
				out = append(out, string(act))
				break
			}
		}
	}
	return out
}

// pageOf membungkus slice menjadi hasil paging.
func pageOf[T any](items []T, page paging.Page, total int64) paging.Result[T] {
	return paging.Result[T]{Items: items, Page: page, Total: total}
}
