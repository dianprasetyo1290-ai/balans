// Package sales adalah API publik modul penjualan. Modul lain dan adapter hanya memakai package ini.
//
// R1.3: Sales Order satu model untuk Modern Trade beli putus, B2B, WhatsApp/Instagram, marketplace, dan karyawan
// (harga otomatis dari pricing, alokasi stok FEFO lewat inventory, approval diskon), Proforma Invoice berlaku
// 14 hari, invoice NON-PKP, piutang & pembayaran pelanggan dengan alokasi ke banyak invoice, dan pengakuan
// penjualan/HPP per channel (A-05, A-06, A-10, A-PB). Logistik (DO, Outbound, kurir) memanggil ShippableOrder,
// MarkShipped/MarkHandedOver, dan MarkDelivered di transaksi dokumennya; rekonsiliasi bank memakai
// IncomingPayments/Payment.
//
// Konsinyasi (S-303 – S-305, S-313): pengiriman titipan ke lokasi konsinyasi / buffer vending mitra hanya dengan PKS
// konsinyasi berlaku (Transfer Stok + jurnal A-07/A-24), dan Rekonsiliasi Konsinyasi bulanan (KSN) yang disetujui
// Sales MT → Head F&A lalu menerbitkan invoice konsinyasi (A-08) dan memproses selisih (A-23). Modul logistics (DO
// konsinyasi) memanggil ShipConsignment/ConfirmConsignmentReceived di transaksi DO-nya.
package sales

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/tabular"
	"balans/internal/platform/webhook"
	"balans/internal/sales/domain"
	saleshttp "balans/internal/sales/http"
	"balans/internal/sales/postgres"
	"balans/internal/sales/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "sales"

// Tipe publik untuk adapter payment gateway.
type (
	PaymentGateway      = service.PaymentGateway
	PaymentRequest      = domain.PaymentRequest
	PaymentTransaction  = domain.PaymentTransaction
	PaymentNotification = domain.PaymentNotification
	PaymentStatus       = domain.PaymentStatus
)

// Master alasan retur (MD-13).
type (
	// ReturnReason adalah alasan retur.
	ReturnReason = domain.ReturnReason
	// ReturnScope adalah dokumen yang memakai alasan retur.
	ReturnScope = domain.ReturnScope
	// Disposition adalah disposisi barang retur.
	Disposition = domain.Disposition
)

// Dokumen penjualan dan dependensi modul lain (diisi adapter di internal/app).
type (
	// Order adalah Sales Order beserta baris dan alokasi stok (Allocations: komponen, lokasi, batch FEFO, qty sisa).
	Order = domain.Order
	// OrderLine adalah baris Sales Order.
	OrderLine = domain.Line
	// OrderType adalah jenis pesanan (channel).
	OrderType = domain.OrderType
	// Allocation adalah stok yang dialokasikan untuk satu komponen baris.
	Allocation = domain.Allocation
	// AllocationBatch adalah batch hasil alokasi FEFO.
	AllocationBatch = domain.AllocationBatch
	// Shipment adalah barang Sales Order yang keluar lewat satu dokumen logistik beserta HPP-nya.
	Shipment = domain.Shipment
	// ShipmentInput adalah input MarkShipped/MarkHandedOver.
	ShipmentInput = service.ShipmentInput
	// ShipmentQty adalah qty kirim per alokasi.
	ShipmentQty = domain.ShipmentQty
	// Status adalah status dokumen penjualan.
	Status = domain.Status
	// CustomerPayment adalah pembayaran pelanggan beserta alokasi invoice.
	CustomerPayment = domain.CustomerPayment
	// PaymentAllocation adalah bagian pembayaran untuk satu invoice.
	PaymentAllocation = domain.PaymentAllocation
	// PaymentQuery adalah kriteria pembayaran masuk untuk rekonsiliasi bank.
	PaymentQuery = service.PaymentQuery
	// Invoice adalah invoice NON-PKP / piutang per invoice.
	Invoice = domain.Invoice

	// ConsignmentDelivery adalah pengiriman titipan ke lokasi konsinyasi / buffer vending mitra.
	ConsignmentDelivery = domain.ConsignmentDelivery
	// ConsignmentShipmentInput adalah input ShipConsignment (DO konsinyasi dari logistics).
	ConsignmentShipmentInput = service.ConsignmentShipmentInput
	// DeliveryLineInput adalah baris pengiriman titipan.
	DeliveryLineInput = domain.DeliveryLineInput
	// DeliveryType adalah jenis pengiriman titipan.
	DeliveryType = domain.DeliveryType
	// Reconciliation adalah Rekonsiliasi Konsinyasi (KSN).
	Reconciliation = domain.Reconciliation
	// VarianceRules adalah ketentuan selisih PKS per penyebab.
	VarianceRules = domain.VarianceRules
	// BorneBy adalah penanggung selisih.
	BorneBy = domain.BorneBy

	ConsignmentStock    = service.ConsignmentStock
	Agreements          = service.Agreements
	StockFlows          = service.StockFlows
	AgreementRef        = service.AgreementRef
	StockTransferInput  = service.StockTransferInput
	StockTransferResult = service.StockTransferResult
	TransferredLine     = service.TransferredLine
	StockCountRef       = service.StockCountRef
	CountedLine         = service.CountedLine

	Catalog   = service.Catalog
	Parties   = service.Parties
	Locations = service.Locations
	Pricing   = service.Pricing
	Stock     = service.Stock
	Ledger    = service.Ledger
	Approvals = service.Approvals
	Numberer  = service.Numberer
	Files     = service.Files
	Users     = service.Users

	ProductRef   = service.ProductRef
	ComponentQty = domain.ComponentQty
	PartyInfo    = domain.PartyInfo
	LocationRef  = service.LocationRef
	ChannelRef   = service.ChannelRef
	Option       = service.Option
	DocumentType = service.DocumentType
)

// Cakupan alasan retur dan disposisi barang retur.
const (
	ScopeSalesReturn    = domain.ScopeSalesReturn
	ScopePurchaseReturn = domain.ScopePurchaseReturn
	ScopeBoth           = domain.ScopeBoth

	DispositionRestock         = domain.DispositionRestock
	DispositionRepack          = domain.DispositionRepack
	DispositionReprocess       = domain.DispositionReprocess
	DispositionMarketingSample = domain.DispositionMarketingSample
	DispositionDispose         = domain.DispositionDispose
)

// Status pembayaran.
const (
	PaymentStatusPending = domain.PaymentStatusPending
	PaymentStatusPaid    = domain.PaymentStatusPaid
	PaymentStatusFailed  = domain.PaymentStatusFailed
	PaymentStatusExpired = domain.PaymentStatusExpired
)

// Jenis pesanan, status, jenis nomor dokumen, jenis pihak, permission, dan sumber dokumen.
const (
	OrderModernTrade    = domain.OrderModernTrade
	OrderB2B            = domain.OrderB2B
	OrderSocialCommerce = domain.OrderSocialCommerce
	OrderMarketplace    = domain.OrderMarketplace
	OrderEmployee       = domain.OrderEmployee

	StatusInProgress = domain.StatusInProgress
	StatusInTransit  = domain.StatusInTransit
	StatusCompleted  = domain.StatusCompleted

	DocSalesOrder = service.DocSalesOrder
	DocProforma   = service.DocProforma
	DocInvoice    = service.DocInvoice
	DocPayment    = service.DocPayment

	DocDeliveryOrder  = service.DocDeliveryOrder
	DocReconciliation = service.DocReconciliation

	DeliveryConsignment   = domain.DeliveryConsignment
	DeliveryVendingBuffer = domain.DeliveryVendingBuffer
	BorneByPartner        = domain.BorneByPartner
	BorneByBalans         = domain.BorneByBalans

	PermReconciliationRead = service.PermReconciliationRead
	// SourceReconciliation adalah source_type issue stok & jurnal A-08/A-23 Rekonsiliasi Konsinyasi.
	SourceReconciliation = service.SourceReconciliation
	// FileOwnerConsignmentBA dan FileOwnerConsignmentReport: BA bertanda tangan dan laporan penjualan mitra.
	FileOwnerConsignmentBA     = service.FileOwnerConsignmentBA
	FileOwnerConsignmentReport = service.FileOwnerConsignmentReport

	PartyCustomer = domain.PartyCustomer
	PartyPartner  = domain.PartyPartner

	PermOrderRead       = service.PermOrderRead
	PermMarketplaceRead = service.PermMarketplaceRead
	PermProformaRead    = service.PermProformaRead
	PermInvoiceRead     = service.PermInvoiceRead
	PermPaymentRead     = service.PermPaymentRead
	PermPaymentPost     = service.PermPaymentPost

	// SourceSalesOrder adalah source_type reservasi/mutasi stok Sales Order di inventory.
	SourceSalesOrder = service.SourceSalesOrder
	// SourceCustomerPayment adalah source_type jurnal A-PB pembayaran pelanggan (rekonsiliasi bank).
	SourceCustomerPayment = service.SourcePayment
	// FileOwnerPaymentProof dan FileOwnerCustomerPaymentProof adalah jenis pemilik file bukti bayar di filestore.
	FileOwnerPaymentProof         = service.FileOwnerPaymentProof
	FileOwnerCustomerPaymentProof = service.FileOwnerCustomerPaymentProof
	// MarketplaceImportKind adalah jenis data impor penjualan marketplace (dataexchange).
	MarketplaceImportKind = service.MarketplaceImportKind
)

// ErrInvalidSignature dikembalikan adapter jika signature webhook tidak valid.
var ErrInvalidSignature = domain.ErrInvalidSignature

// Deps adalah dependensi modul. Dependensi dokumen penjualan (Catalog … Users) wajib diisi agar route dokumen
// penjualan berfungsi.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Audit     audit.Recorder
	Gateway   PaymentGateway
	Inbox     *webhook.Inbox
	Responder *httpx.Responder
	Log       *slog.Logger
	Clock     clock.Clock
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
	// Consignment, Agreements, Flows: dependensi konsinyasi (warehouse, party, inventory).
	Consignment ConsignmentStock
	Agreements  Agreements
	Flows       StockFlows
	// ProformaValidDays: masa berlaku Proforma Invoice (0 = 14 hari).
	ProformaValidDays int
	// BaseURL adalah URL ERP untuk tautan invoice di teks WhatsApp.
	BaseURL string
}

// Module adalah modul sales.
type Module struct {
	webhooks *service.PaymentWebhookService
	reasons  *service.ReturnReasonService
	docs     *service.DocumentService
	returns  *service.ReturnService
	rs       *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	store := postgres.NewStore(d.Pool)
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	m := &Module{
		webhooks: service.NewPaymentWebhookService(d.Gateway, d.Inbox, d.Log),
		reasons:  service.NewReturnReasonService(store, d.Tx, d.Audit),
		docs: service.NewDocumentService(service.Deps{
			Store: store, Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log, Catalog: d.Catalog, Parties: d.Parties,
			Locations: d.Locations, Pricing: d.Pricing, Stock: d.Stock, Ledger: d.Ledger, Approvals: d.Approvals,
			Numberer: d.Numberer, Files: d.Files, Users: d.Users, Consignment: d.Consignment, Agreements: d.Agreements,
			Flows: d.Flows, ProformaValidDays: d.ProformaValidDays, BaseURL: d.BaseURL,
		}),
		rs: d.Responder,
	}
	m.returns = service.NewReturnService(m.docs)
	return m
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route webhook Midtrans, master alasan retur, dokumen penjualan, dan piutang.
func (m *Module) RegisterRoutes(r chi.Router) {
	saleshttp.Register(r, saleshttp.NewHandler(m.reasons, m.docs).WithReturns(m.returns), saleshttp.NewWebhookHandler(m.webhooks, m.rs), m.rs)
}

// RegisterWorkers mendaftarkan worker job modul.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewPaymentWebhookWorker(m.webhooks))
	river.AddWorker(w, service.NewExpireProformasWorker(m.docs))
}

// PeriodicJobs menjadwalkan sales.expire_proformas tiap jam: Proforma lewat masa berlaku → Kedaluwarsa dan alokasi
// stok dilepas (BRD 5.4).
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return service.ExpireProformasArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.ExpireProformasArgs{}.Kind(), RunOnStart: true}),
	}
}

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat.
func AuditEntities() map[string]string {
	return map[string]string{
		"return_reason":                "sales.return_reason.read",
		service.EntitySalesOrder:       service.PermOrderRead,
		service.EntityMarketplaceOrder: service.PermMarketplaceRead,
		service.EntityProforma:         service.PermProformaRead,
		service.EntityInvoice:          service.PermInvoiceRead,
		service.EntityCustomerPayment:  service.PermPaymentRead,
		// Pengiriman konsinyasi dibaca dengan sales.sales_order.read (Logistik membuka lewat dokumen DO).
		service.EntityConsignmentDelivery: service.PermOrderRead,
		service.EntityReconciliation:      service.PermReconciliationRead,
	}
}

// ApprovalRegistrar adalah bagian modul approval yang dipakai saat wiring.
type ApprovalRegistrar interface {
	RegisterHandler(t approval.SubjectType, readPermission string, h approval.Handler)
}

// RegisterApprovalHandlers mendaftarkan handler approval `sales_order_discount` (diskon di atas batas role) dan
// `consignment_reconciliation` (disetujui → invoice + jurnal A-08/A-23; ditolak → bisa direvisi). Dipanggil
// SETELAH handler dev agar menggantikannya.
func (m *Module) RegisterApprovalHandlers(r ApprovalRegistrar) {
	r.RegisterHandler(approval.SubjectSalesOrderDiscount, service.PermOrderRead, approval.Handler{
		OnApproved: m.docs.OnDiscountApproved, OnRejected: m.docs.OnDiscountRejected,
	})
	r.RegisterHandler(approval.SubjectConsignmentReconciliation, service.PermReconciliationRead, approval.Handler{
		OnApproved: m.docs.OnReconciliationApproved, OnRejected: m.docs.OnReconciliationRejected,
	})
}

// --- API untuk logistics: DO konsinyasi / buffer vending (integrasi S-303 ↔ S-309) ----------

// ShipConsignment mencatat barang titipan keluar lewat dokumen logistik (DO konsinyasi/buffer vending ship) di
// transaksi pemanggil: PKS konsinyasi wajib berlaku pada tanggal kirim, stok FEFO dipindah ke lokasi konsinyasi /
// buffer vending mitra lewat warehouse.TransferStock (Dalam Pengiriman), dan jurnal A-07 (Dr 1312 Cr 1311) atau
// A-24 (Dr 1313 Cr 1311) sebesar nilai FIFO dibuat dengan dimensi channel konsinyasi/vending + sub-channel mitra.
// Idempotent per (SourceType, SourceID). Logistik TIDAK memindah stok atau memposting jurnal sendiri.
func (m *Module) ShipConsignment(ctx context.Context, in ConsignmentShipmentInput) (ConsignmentDelivery, error) {
	return m.docs.ShipConsignment(ctx, in)
}

// ConfirmConsignmentReceived menandai pengiriman titipan diterima mitra (DO deliver): transfer Dalam Pengiriman →
// Tersedia di lokasi mitra.
func (m *Module) ConfirmConsignmentReceived(ctx context.Context, deliveryID uuid.UUID) (ConsignmentDelivery, error) {
	return m.docs.ConfirmConsignmentReceived(ctx, deliveryID)
}

// ReturnReason membaca alasan retur (belum dihapus) untuk dokumen retur penjualan/pembelian; NOT_FOUND bila
// tidak ada. Tidak memeriksa permission. Pakai ReturnReason.UsableError(scope) sebelum dipakai dokumen baru.
func (m *Module) ReturnReason(ctx context.Context, id uuid.UUID) (ReturnReason, error) {
	return m.reasons.Get(ctx, id)
}

// ReturnReasonByCode membaca alasan retur berdasarkan kode (impor data).
func (m *Module) ReturnReasonByCode(ctx context.Context, code string) (ReturnReason, error) {
	return m.reasons.GetByCode(ctx, code)
}

// --- API untuk logistics (DO, Outbound, serah kurir) ----------------------------------------

// ShippableOrder mengembalikan Sales Order berstatus Diproses beserta baris dan alokasi stok (komponen bundel,
// lokasi, batch FEFO, qty belum dikirim = Allocation.OutstandingQty) untuk disusun menjadi DO. Ditolak
// INVALID_STATE_TRANSITION bila pesanan belum dibayar/dikonfirmasi atau sudah dikirim. Tanpa cek permission.
func (m *Module) ShippableOrder(ctx context.Context, orderID uuid.UUID) (Order, error) {
	return m.docs.ShippableOrder(ctx, orderID)
}

// MarkShipped mencatat barang Sales Order keluar lewat dokumen logistik (DO ship) di transaksi pemanggil: stok
// reserved di-issue (HPP FIFO) dan jurnal HPP A-06 (MT/B2B/WA-IG/karyawan) atau penjualan marketplace A-10 dibuat
// dengan dimensi channel/sub-channel pesanan. Lines kosong = seluruh sisa alokasi. Idempotent per
// (pesanan, SourceType, SourceID). Semua terkirim → Dalam Pengiriman. Logistik TIDAK memposting stok/jurnal sendiri.
func (m *Module) MarkShipped(ctx context.Context, in ShipmentInput) (Shipment, error) {
	return m.docs.MarkShipped(ctx, in)
}

// MarkHandedOver mencatat serah ke kurir pesanan marketplace (titik pengakuan penjualan, U-25): seluruh pesanan
// keluar dari E-commerce Stock dan jurnal A-10 (Piutang Marketplace 121x sesuai marketplace, voucher penjual,
// Penjualan, HPP) diposting.
func (m *Module) MarkHandedOver(ctx context.Context, in ShipmentInput) (Shipment, error) {
	return m.docs.MarkHandedOver(ctx, in)
}

// MarkDelivered menandai pesanan Dalam Pengiriman sudah diterima (Selesai).
func (m *Module) MarkDelivered(ctx context.Context, orderID uuid.UUID) (Order, error) {
	return m.docs.MarkDelivered(ctx, orderID)
}

// --- API untuk rekonsiliasi bank (R1.4) ------------------------------------------------------

// IncomingPayments mengembalikan pembayaran pelanggan Diposting (BYR) per akun Kas & Bank dan rentang tanggal
// untuk dicocokkan dengan mutasi bank (referensi → nominal + tanggal). CustomerPayment.JournalEntryID adalah jurnal
// A-PB-nya. Tanpa cek permission.
func (m *Module) IncomingPayments(ctx context.Context, q PaymentQuery) ([]CustomerPayment, error) {
	return m.docs.IncomingPayments(ctx, q)
}

// Payment membaca satu pembayaran pelanggan beserta alokasi invoice. Tanpa cek permission.
func (m *Module) Payment(ctx context.Context, id uuid.UUID) (CustomerPayment, error) {
	return m.docs.Payment(ctx, id)
}

// --- Job & impor ------------------------------------------------------------------------------

// ExpireProformas menjalankan pelepasan Proforma kedaluwarsa satu kali (worker dan test).
func (m *Module) ExpireProformas(ctx context.Context) (int, error) {
	return m.docs.ExpireProformas(ctx)
}

// MarketplaceColumns adalah kolom template impor penjualan marketplace.
func MarketplaceColumns() []tabular.Column { return service.MarketplaceColumns }

// ImportMarketplaceRow mencatat satu order marketplace dari baris Excel di transaksi aktif; rowKey
// (dataexchange.RowKey) membuat penyimpanan idempotent per baris. Butuh sales.marketplace_order.import + write.
func (m *Module) ImportMarketplaceRow(ctx context.Context, row tabular.Row, rowKey string) error {
	return m.docs.ImportMarketplaceRow(ctx, row, rowKey)
}
