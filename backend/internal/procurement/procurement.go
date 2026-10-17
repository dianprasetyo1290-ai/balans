// Package procurement adalah API publik modul pengadaan R1.2: Purchase Request, Purchase Order (barang/packaging,
// jasa maklon per batch, reprocess) dengan approval berjenjang, notifikasi ETA H-1, dan uang muka pembelian A-19
// (BRD 5.2, 5.3; PRD F1-201 – F1-203). Fungsi untuk Penerimaan Barang dan Tagihan Pemasok berjalan di transaksi
// pemanggil dan tidak memeriksa permission (pemanggil memeriksa aksi user-nya).
package procurement

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/procurement/domain"
	procurementhttp "balans/internal/procurement/http"
	"balans/internal/procurement/postgres"
	"balans/internal/procurement/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "procurement"

type (
	// PurchaseOrder adalah PO beserta baris (qty dipesan/diterima/ditagih) dan bahan maklon.
	PurchaseOrder = domain.PurchaseOrder
	// OrderLine adalah baris PO.
	OrderLine = domain.OrderLine
	// OrderMaterial adalah packaging/material GM yang direncanakan dipakai maklon.
	OrderMaterial = domain.OrderMaterial
	// OrderType adalah jenis PO.
	OrderType = domain.OrderType
	// Status adalah status dokumen pengadaan.
	Status = domain.Status
	// QtyDelta adalah perubahan qty per baris PO (negatif untuk pembatalan dokumen).
	QtyDelta = domain.QtyDelta
	// Advance adalah uang muka pembelian.
	Advance = domain.Advance
	// AdvanceApplication adalah potongan satu uang muka oleh tagihan pemasok.
	AdvanceApplication = service.AdvanceApplication

	// Catalog, Suppliers, Locations, Ledger, Approvals, Numberer, Users, Notifier diisi adapter saat wiring.
	Catalog   = service.Catalog
	Suppliers = service.Suppliers
	Locations = service.Locations
	Ledger    = service.Ledger
	Approvals = service.Approvals
	Numberer  = service.Numberer
	Users     = service.Users
	Notifier  = service.Notifier
	// ProductRef, SupplierRef, LocationRef, UserRef, Option adalah data rujukan dari modul lain.
	ProductRef   = service.ProductRef
	SupplierRef  = service.SupplierRef
	LocationRef  = service.LocationRef
	UserRef      = service.UserRef
	Option       = service.Option
	DocumentType = service.DocumentType
)

// Jenis PO, status, jenis nomor dokumen, dan permission.
const (
	OrderGoods             = domain.OrderGoods
	OrderPackaging         = domain.OrderPackaging
	OrderTollManufacturing = domain.OrderTollManufacturing
	OrderReprocess         = domain.OrderReprocess
	OrderOther             = domain.OrderOther

	StatusApproved          = domain.StatusApproved
	StatusPartiallyReceived = domain.StatusPartiallyReceived
	StatusReceived          = domain.StatusReceived
	StatusCompleted         = domain.StatusCompleted
	StatusClosed            = domain.StatusClosed

	DocPurchaseRequest = service.DocPurchaseRequest
	DocPurchaseOrder   = service.DocPurchaseOrder
	DocPayment         = service.DocPayment

	PermRequestRead = service.PermRequestRead
	PermOrderRead   = service.PermOrderRead
	PermAdvanceRead = service.PermAdvanceRead

	// SourcePurchaseAdvance adalah source_type jurnal A-19 uang muka.
	SourcePurchaseAdvance = service.SourcePurchaseAdvance
)

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Catalog   Catalog
	Suppliers Suppliers
	Locations Locations
	Ledger    Ledger
	Approvals Approvals
	Numberer  Numberer
	Users     Users
	Notifier  Notifier
	BaseURL   string
}

// Module adalah modul procurement.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *procurementhttp.ProcurementHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log, Catalog: d.Catalog,
		Suppliers: d.Suppliers, Locations: d.Locations, Ledger: d.Ledger, Approvals: d.Approvals, Numberer: d.Numberer,
		Users: d.Users, Notifier: d.Notifier, BaseURL: d.BaseURL,
	})
	return &Module{svc: svc, rs: d.Responder, h: procurementhttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route PR, PO, dan uang muka.
func (m *Module) RegisterRoutes(r chi.Router) { procurementhttp.Register(r, m.h, m.rs) }

// RegisterWorkers mendaftarkan worker notifikasi ETA.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewNotifyETAWorker(m.svc))
}

// PeriodicJobs menjadwalkan procurement.notify_eta harian 07.00 WIB: PO yang tiba besok diberitahukan ke
// Logistik Leader (S-202, Desain Alur Lintas Modul 1).
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(jobqueue.DailyAt{Hour: 7},
			func() (river.JobArgs, *river.InsertOpts) { return service.NotifyETAArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.NotifyETAArgs{}.Kind()}),
	}
}

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat (audit.RegisterEntity).
func AuditEntities() map[string]string {
	return map[string]string{
		service.EntityPurchaseRequest: service.PermRequestRead,
		service.EntityPurchaseOrder:   service.PermOrderRead,
		service.EntityPurchaseAdvance: service.PermAdvanceRead,
	}
}

// ApprovalRegistrar adalah bagian modul approval yang dipakai saat wiring.
type ApprovalRegistrar interface {
	RegisterHandler(t approval.SubjectType, readPermission string, h approval.Handler)
}

// RegisterApprovalHandlers mendaftarkan handler keputusan approval `purchase_request` dan `purchase_order`.
// Dipanggil SETELAH handler dev agar menggantikannya.
func (m *Module) RegisterApprovalHandlers(r ApprovalRegistrar) {
	r.RegisterHandler(approval.SubjectPurchaseRequest, service.PermRequestRead, approval.Handler{
		OnApproved: m.svc.OnRequestApproved, OnRejected: m.svc.OnRequestRejected,
	})
	r.RegisterHandler(approval.SubjectPurchaseOrder, service.PermOrderRead, approval.Handler{
		OnApproved: m.svc.OnOrderApproved, OnRejected: m.svc.OnOrderRejected,
	})
}

// NotifyETA menjalankan notifikasi ETA H-1 satu kali (dipakai worker dan test).
func (m *Module) NotifyETA(ctx context.Context) (int, error) { return m.svc.NotifyETA(ctx) }

// --- API untuk Penerimaan Barang (S-206, S-207) -----------------------------------------------

// ReceivableOrder mengembalikan PO yang bisa diterima (Disetujui/Diterima Sebagian) beserta baris (OutstandingQty
// per baris), bahan maklon (Materials), lokasi maklon (TollLocationID), dan lokasi asal bahan. Ditolak
// INVALID_STATE_TRANSITION bila PO belum disetujui, sudah diterima penuh, ditutup, atau dibatalkan.
func (m *Module) ReceivableOrder(ctx context.Context, orderID uuid.UUID) (PurchaseOrder, error) {
	return m.svc.ReceivableOrder(ctx, orderID)
}

// RecordReceipt mencatat qty diterima per baris PO di transaksi aktif (qty negatif saat Penerimaan Barang
// dibatalkan) dan mengubah status PO menjadi Diterima Sebagian/Diterima. Toleransi qty 0%: qty diterima melebihi
// qty PO ditolak VALIDATION_FAILED. receiptLabel (nomor GR) dicatat di audit trail PO.
func (m *Module) RecordReceipt(ctx context.Context, orderID uuid.UUID, receiptLabel string, lines []QtyDelta) (PurchaseOrder, error) {
	return m.svc.RecordReceipt(ctx, orderID, receiptLabel, lines)
}

// --- API untuk Tagihan Pemasok (S-208) --------------------------------------------------------

// Order membaca PO beserta baris tanpa memeriksa permission (3-way match tagihan).
func (m *Module) Order(ctx context.Context, orderID uuid.UUID) (PurchaseOrder, error) {
	return m.svc.Order(ctx, orderID)
}

// RecordBilling mencatat qty yang ditagih per baris PO di transaksi aktif (negatif saat tagihan dibatalkan);
// PO Diterima yang tertagih penuh menjadi Selesai. Qty ditagih tidak boleh melebihi qty diterima.
func (m *Module) RecordBilling(ctx context.Context, orderID uuid.UUID, billLabel string, lines []QtyDelta) (PurchaseOrder, error) {
	return m.svc.RecordBilling(ctx, orderID, billLabel, lines)
}

// OutstandingAdvances mengembalikan uang muka Diposting PO yang masih punya sisa (Advance.Outstanding), urut
// tanggal bayar.
func (m *Module) OutstandingAdvances(ctx context.Context, orderID uuid.UUID) ([]Advance, error) {
	return m.svc.OutstandingAdvances(ctx, orderID)
}

// OutstandingAdvanceAmount menjumlah sisa uang muka PO yang akan dipotong otomatis dari tagihan.
func (m *Module) OutstandingAdvanceAmount(ctx context.Context, orderID uuid.UUID) (money.Amount, error) {
	list, err := m.svc.OutstandingAdvances(ctx, orderID)
	if err != nil {
		return money.Zero, err
	}
	total := money.Zero
	for _, a := range list {
		total = total.Add(a.Outstanding())
	}
	return total, nil
}

// ApplyAdvances memotong sisa uang muka PO sebesar maksimal amount (FIFO tanggal bayar) di transaksi aktif dan
// mengembalikan rinciannya. Tagihan pemasok memposting A-19 `advance_applied` (Dr 2101 / Cr 1401) sebesar total
// potongan dan menyimpan rincian untuk ReleaseAdvances.
func (m *Module) ApplyAdvances(ctx context.Context, orderID uuid.UUID, amount money.Amount) ([]AdvanceApplication, error) {
	return m.svc.ApplyAdvances(ctx, orderID, amount)
}

// ReleaseAdvances mengembalikan potongan uang muka saat tagihan pemasok dibatalkan.
func (m *Module) ReleaseAdvances(ctx context.Context, applications []AdvanceApplication) error {
	return m.svc.ReleaseAdvances(ctx, applications)
}
