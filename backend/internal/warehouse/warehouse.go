// Package warehouse adalah API publik modul gudang. Bagian master (R1.1): gudang fisik dengan lokasi standar
// dan lokasi eksternal per pihak (MD-09, F1-114). Fungsi lookup tidak memeriksa permission (modul pemanggil
// memeriksa permission dokumennya sendiri) dan tetap mengembalikan data terhapus agar dokumen lama bisa
// menampilkannya (Technical Guideline 5.13).
//
// Bagian dokumen (R1.2, WMS-02/03/04/08/09/10/11): Inbound/QC, Transfer Stok (termasuk Permintaan Petty
// Stock), dan Stock Count beserta jadwalnya. Dokumen memutasi stok lewat ledger modul inventory (StockLedger,
// diisi adapter saat wiring) di transaksi dokumennya. Modul lain memakai:
//
//   - CreateInbound: modul asal (Penerimaan Barang, Retur Penjualan, tarikan konsinyasi, sisa event) mendaftarkan
//     barang yang sudah diterimanya ke lokasi Inbound berstatus Karantina; RegisterInboundHandler menerima hasil
//     QC per baris (lulus/ditolak) di transaksi QC.
//   - TransferStock / ReceiveTransfer: memindah stok antar lokasi (DO konsinyasi A-07, buffer vending A-24, kirim
//     ke maklon) dan mengembalikan nilai FIFO barang yang dipindah untuk jurnal modul pemanggil.
//   - StockCountResult: hasil stock count konsinyasi/vending untuk rekonsiliasi.
package warehouse

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
	"balans/internal/warehouse/domain"
	warehousehttp "balans/internal/warehouse/http"
	"balans/internal/warehouse/postgres"
	"balans/internal/warehouse/service"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "warehouse"

type (
	// Warehouse adalah gudang fisik.
	Warehouse = domain.Warehouse
	// Location adalah lokasi stok internal atau eksternal.
	Location = domain.Location
	// LocationType adalah jenis lokasi.
	LocationType = domain.LocationType
	// PartyType adalah jenis pihak pemilik lokasi eksternal.
	PartyType = domain.PartyType

	// Inbound adalah antrian barang Karantina satu dokumen asal.
	Inbound = domain.Inbound
	// InboundLine adalah satu baris Inbound.
	InboundLine = domain.InboundLine
	// InboundLineInput adalah baris yang didaftarkan modul asal.
	InboundLineInput = domain.InboundLineInput
	// RejectAction adalah perlakuan qty ditolak QC.
	RejectAction = domain.RejectAction
	// InboundOutcome adalah hasil QC satu baris untuk modul asal.
	InboundOutcome = service.InboundOutcome
	// InboundHandler menerima hasil QC di transaksi QC.
	InboundHandler = service.InboundHandler
	// StockTransfer adalah dokumen Transfer Stok.
	StockTransfer = domain.StockTransfer
	// TransferLine adalah satu baris transfer (setelah dikirim: satu batch per baris).
	TransferLine = domain.TransferLine
	// TransferLineInput adalah isian baris transfer.
	TransferLineInput = domain.TransferLineInput
	// ReceiveLine adalah qty diterima per baris transfer.
	ReceiveLine = domain.ReceiveLine
	// TransferResult adalah dokumen transfer beserta total nilai FIFO barang yang dipindah.
	TransferResult = service.TransferResult
	// StockCount adalah dokumen Stock Count.
	StockCount = domain.StockCount
	// CountLine adalah satu baris Stock Count.
	CountLine = domain.CountLine

	// DocumentDeps adalah dependensi dokumen gudang yang dipasang saat wiring.
	DocumentDeps = service.DocumentDeps
	// StockLedger adalah API ledger stok (adapter modul inventory).
	StockLedger = service.StockLedger
	// StockSource adalah aksi dokumen sumber mutasi stok (adapter StockLedger).
	StockSource = service.StockSource
	// StockLine adalah satu baris posting stok.
	StockLine = service.StockLine
	// StockPlace adalah lokasi + status stok.
	StockPlace = service.StockPlace
	// StockReservationRef menunjuk reservasi dokumen.
	StockReservationRef = service.StockReservationRef
	// StockResult adalah hasil posting stok.
	StockResult = service.StockResult
	// StockLineResult adalah hasil satu baris posting stok.
	StockLineResult = service.StockLineResult
	// StockMovement adalah satu baris ledger hasil posting.
	StockMovement = service.StockMovement
	// StockReserveLine adalah satu baris reservasi.
	StockReserveLine = service.StockReserveLine
	// StockBalanceQuery adalah kriteria saldo stok.
	StockBalanceQuery = service.StockBalanceQuery
	// StockBalance adalah saldo stok dengan HPP rata-rata batch.
	StockBalance = service.StockBalance
	// BatchInfo adalah ringkasan batch.
	BatchInfo = service.BatchInfo
	// Users mencari user per role (adapter identity).
	Users = service.Users
)

// Jenis lokasi, jenis pihak, perlakuan reject, jenis asal Inbound, status & jenis baris stok.
const (
	LocationInbound          = domain.LocationInbound
	LocationFinishedGoods    = domain.LocationFinishedGoods
	LocationEcommerce        = domain.LocationEcommerce
	LocationPettyStock       = domain.LocationPettyStock
	LocationRawMaterial      = domain.LocationRawMaterial
	LocationGoodsMaterial    = domain.LocationGoodsMaterial
	LocationReturns          = domain.LocationReturns
	LocationEvent            = domain.LocationEvent
	LocationTollManufacturer = domain.LocationTollManufacturer
	LocationConsignment      = domain.LocationConsignment
	LocationVendingBuffer    = domain.LocationVendingBuffer
	LocationVendingMachine   = domain.LocationVendingMachine

	PartySupplier       = domain.PartySupplier
	PartyPartner        = domain.PartyPartner
	PartyVendingMachine = domain.PartyVendingMachine

	RejectHold      = domain.RejectHold
	RejectToReturns = domain.RejectToReturns

	SourceGoodsReceipt        = domain.SourceGoodsReceipt
	SourceSalesReturn         = domain.SourceSalesReturn
	SourceConsignmentPullback = domain.SourceConsignmentPullback
	SourceStockTransfer       = domain.SourceStockTransfer
	SourceInternalUsageReturn = domain.SourceInternalUsageReturn

	StatusAvailable  = service.StockAvailable
	StatusQuarantine = service.StockQuarantine
	StatusInTransit  = service.StockInTransit
	StatusReserved   = service.StockReserved
	KindReceive      = service.StockReceive
	KindIssue        = service.StockIssue
	KindMove         = service.StockMove
)

// Permission, jenis entitas audit trail, dan jenis pemilik file.
const (
	PermWarehouseRead = service.PermWarehouseRead
	PermLocationRead  = service.PermLocationRead
	EntityWarehouse   = service.EntityWarehouse
	EntityLocation    = service.EntityLocation

	PermInboundRead  = service.PermInboundRead
	PermInboundPost  = service.PermInboundPost
	PermTransferRead = service.PermTransferRead
	PermCountRead    = service.PermCountRead
	PermCountWrite   = service.PermCountWrite
	EntityInbound    = service.EntityInbound
	EntityTransfer   = service.EntityStockTransfer
	EntityStockCount = service.EntityStockCount
	EntitySchedule   = service.EntityCountSchedule
	FileOwnerQC      = service.FileOwnerQualityCheck
	FileOwnerCount   = service.FileOwnerStockCount
)

// LocationQuery adalah kriteria pencarian lokasi aktif untuk modul lain. Field kosong tidak difilter.
type LocationQuery struct {
	WarehouseID *uuid.UUID
	Types       []LocationType
	// External true = hanya lokasi eksternal, false = hanya lokasi di dalam gudang.
	External  *bool
	PartyType PartyType
	PartyID   *uuid.UUID
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Responder *httpx.Responder
	Audit     audit.Recorder
	Log       *slog.Logger
}

// Module adalah modul warehouse.
type Module struct {
	clock clock.Clock
	svc   *service.Service
	rs    *httpx.Responder
	h     *warehousehttp.WarehouseHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	store := postgres.NewStore(d.Pool)
	svc := service.New(service.Deps{Store: store, Docs: store, Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log})
	return &Module{clock: d.Clock, svc: svc, rs: d.Responder, h: warehousehttp.NewHandler(svc)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route gudang, lokasi, dan dokumen gudang.
func (m *Module) RegisterRoutes(r chi.Router) { warehousehttp.Register(r, m.h, m.rs) }

// RegisterWorkers mendaftarkan worker jadwal stock count.
func (m *Module) RegisterWorkers(w *river.Workers) {
	river.AddWorker(w, service.NewRunSchedulesWorker(m.svc))
}

// PeriodicJobs menjadwalkan warehouse.stock_count_schedule harian 00.15 WIB, juga saat worker mulai.
func (m *Module) PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(jobqueue.DailyAt{Hour: 0, Minute: 15},
			func() (river.JobArgs, *river.InsertOpts) { return service.RunSchedulesArgs{}, nil },
			&river.PeriodicJobOpts{ID: service.RunSchedulesArgs{}.Kind(), RunOnStart: true}),
	}
}

// AuditEntities memetakan jenis entitas audit trail ke permission baca untuk tab Riwayat (audit.RegisterEntity).
func AuditEntities() map[string]string {
	return map[string]string{
		EntityWarehouse: PermWarehouseRead, EntityLocation: PermLocationRead, EntityInbound: PermInboundRead,
		EntityTransfer: PermTransferRead, EntityStockCount: PermCountRead, EntitySchedule: PermCountRead,
	}
}

// Warehouse membaca gudang berdasarkan ID, termasuk yang terhapus.
func (m *Module) Warehouse(ctx context.Context, id uuid.UUID) (Warehouse, error) {
	return m.svc.WarehouseByID(ctx, id)
}

// Location membaca lokasi berdasarkan ID, termasuk yang terhapus. NOT_FOUND jika tidak ada.
func (m *Module) Location(ctx context.Context, id uuid.UUID) (Location, error) {
	return m.svc.LocationByID(ctx, id)
}

// Locations membaca banyak lokasi berdasarkan ID (termasuk yang terhapus); ID yang tidak ada tidak ikut di peta.
func (m *Module) Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Location, error) {
	return m.svc.LocationsByIDs(ctx, ids)
}

// FindLocations mengembalikan lokasi aktif yang belum dihapus (maks. 200) menurut gudang, jenis, atau pihak,
// mis. lokasi Inbound gudang tertentu atau lokasi konsinyasi milik satu mitra.
func (m *Module) FindLocations(ctx context.Context, q LocationQuery) ([]Location, error) {
	return m.svc.FindLocations(ctx, service.LocationFilter{
		WarehouseID: q.WarehouseID, Types: q.Types, External: q.External, PartyType: q.PartyType, PartyID: q.PartyID,
	})
}

// SeedDev membuat Gudang Utama beserta lokasi standarnya untuk demo lokal (APP_ENV=local, BRD U-08).
// Aman diulang: gudang yang sudah ada dilewati.
func (m *Module) SeedDev(ctx context.Context) error { return m.svc.SeedDev(ctx) }

// ExternalLocation mengembalikan lokasi eksternal aktif jenis t milik pihak partyID; ok false jika belum ada.
func (m *Module) ExternalLocation(ctx context.Context, t LocationType, partyID uuid.UUID) (Location, bool, error) {
	list, err := m.FindLocations(ctx, LocationQuery{Types: []LocationType{t}, PartyID: &partyID})
	if err != nil || len(list) == 0 {
		return Location{}, false, err
	}
	return list[0], true, nil
}

// --- Dokumen gudang (R1.2) --------------------------------------------------------------------

// SetDocumentDeps memasang dependensi dokumen gudang (ledger stok, penomoran, katalog, approval, jurnal, file,
// mitra, notifikasi) setelah modul-modul itu disusun.
func (m *Module) SetDocumentDeps(d DocumentDeps) { m.svc.SetDocumentDeps(d) }

// ApprovalHandler adalah handler approval `stock_count`: disetujui → penyesuaian stok + jurnal A-14 → Diposting;
// ditolak → bisa dihitung ulang. Didaftarkan ke approval.Module.RegisterHandler saat wiring.
func (m *Module) ApprovalHandler() approval.Handler {
	return approval.Handler{OnApproved: m.svc.OnStockCountApproved, OnRejected: m.svc.OnStockCountRejected}
}

// InboundRequest adalah permintaan Inbound dari modul asal. Stok sudah diterima modul asal ke lokasi Inbound
// berstatus Karantina di transaksi yang sama.
type InboundRequest struct {
	// SourceType snake_case, mis. SourceGoodsReceipt; SourceID unik per jenis (satu Inbound per dokumen asal).
	SourceType  string
	SourceID    uuid.UUID
	SourceLabel string
	// LocationID lokasi Inbound; kosong = lokasi Inbound standar WarehouseID.
	LocationID  *uuid.UUID
	WarehouseID *uuid.UUID
	// BusinessDate kosong = hari ini.
	BusinessDate clock.BusinessDate
	// RejectAction kosong = goods_receipt ditahan di Inbound (RejectHold), lainnya ke Gudang Retur (RejectToReturns).
	RejectAction RejectAction
	Note         string
	// Lines: DestinationLocationID kosong = lokasi standar sesuai jenis produk (FG/RM/GM).
	Lines []InboundLineInput
}

// CreateInbound membuat Inbound (nomor IN) di transaksi aktif tanpa memeriksa permission. QC tiap baris
// dikerjakan Staf Gudang di Gudang › Inbound; stok Karantina tidak bisa dialokasikan sampai lulus QC.
func (m *Module) CreateInbound(ctx context.Context, r InboundRequest) (Inbound, error) {
	in := domain.InboundInput{
		SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel, BusinessDate: r.BusinessDate,
		RejectAction: r.RejectAction, Note: r.Note, Lines: r.Lines,
	}
	if r.LocationID != nil {
		in.LocationID = *r.LocationID
	}
	return m.svc.CreateInbound(ctx, in, r.WarehouseID)
}

// RegisterInboundHandler mendaftarkan callback hasil QC untuk jenis dokumen asal (registrasi terakhir menang).
// Handler dipanggil di transaksi QC; error membatalkan QC.
func (m *Module) RegisterInboundHandler(sourceType string, h InboundHandler) {
	m.svc.RegisterInboundHandler(sourceType, h)
}

// InboundBySource membaca Inbound milik dokumen asal (NOT_FOUND bila belum ada).
func (m *Module) InboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (Inbound, error) {
	return m.svc.InboundBySource(ctx, sourceType, sourceID)
}

// TransferRequest adalah transfer stok yang dibuat modul lain (mis. DO konsinyasi, buffer vending, kirim ke maklon).
type TransferRequest struct {
	// SourceType + SourceID: dokumen pemanggil, wajib dan unik (satu transfer per dokumen).
	SourceType     string
	SourceID       uuid.UUID
	FromLocationID uuid.UUID
	ToLocationID   uuid.UUID
	// BusinessDate kosong = hari ini.
	BusinessDate clock.BusinessDate
	Note         string
	// Lines: BatchID kosong = FEFO; batch manual bukan saran FEFO wajib BatchReason.
	Lines []TransferLineInput
}

// TransferStock membuat Transfer Stok (nomor TRF) dan langsung menjalankannya di transaksi aktif tanpa cek
// permission: dalam satu gudang stok pindah ke Tersedia di tujuan; antar gudang atau ke/dari lokasi eksternal
// stok berstatus Dalam Pengiriman di lokasi tujuan sampai ReceiveTransfer. Baris hasil dipecah per batch FEFO
// dengan CostAmount (nilai FIFO) untuk jurnal pemanggil (A-07, A-24). Transfer tidak membuat jurnal sendiri.
func (m *Module) TransferStock(ctx context.Context, r TransferRequest) (TransferResult, error) {
	sid := r.SourceID
	if r.BusinessDate.IsZero() {
		r.BusinessDate = clock.Today(m.clock)
	}
	return m.svc.TransferStock(ctx, domain.TransferInput{
		Type: domain.TransferDirect, FromLocationID: r.FromLocationID, ToLocationID: r.ToLocationID,
		BusinessDate: r.BusinessDate, Note: r.Note, SourceType: r.SourceType, SourceID: &sid, Lines: r.Lines,
	})
}

// ReceiveTransfer mencatat qty diterima transfer Dalam Pengiriman tanpa cek permission (Dalam Pengiriman →
// Tersedia di tujuan). complete true menutup transfer; sisa dicatat sebagai kekurangan untuk Stock Count tujuan.
func (m *Module) ReceiveTransfer(ctx context.Context, transferID uuid.UUID, lines []ReceiveLine, complete bool) (TransferResult, error) {
	return m.svc.ReceiveTransfer(ctx, transferID, lines, complete)
}

// Transfer membaca Transfer Stok berdasarkan ID tanpa cek permission.
func (m *Module) Transfer(ctx context.Context, id uuid.UUID) (StockTransfer, error) {
	return m.svc.TransferByID(ctx, id)
}

// StockCountResult membaca hasil Stock Count (baris, qty sistem, qty hitung, selisih) tanpa cek permission,
// mis. untuk Rekonsiliasi Konsinyasi.
func (m *Module) StockCountResult(ctx context.Context, id uuid.UUID) (StockCount, error) {
	return m.svc.StockCountResult(ctx, id)
}

// RunStockCountSchedules menjalankan jadwal stock count yang jatuh tempo (dipakai job dan test).
func (m *Module) RunStockCountSchedules(ctx context.Context) (int, error) {
	return m.svc.RunSchedules(ctx)
}
