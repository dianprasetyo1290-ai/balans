package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/party"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Permission dokumen gudang (Desain Role & Approval 3.2 dan 3.5).
const (
	PermInboundRead    = "warehouse.inbound.read"
	PermInboundPost    = "warehouse.inbound.post"
	PermTransferRead   = "warehouse.stock_transfer.read"
	PermTransferWrite  = "warehouse.stock_transfer.write"
	PermTransferSubmit = "warehouse.stock_transfer.submit"
	PermTransferPost   = "warehouse.stock_transfer.post"
	PermTransferCancel = "warehouse.stock_transfer.cancel"
	PermCountRead      = "warehouse.stock_count.read"
	PermCountWrite     = "warehouse.stock_count.write"
	PermCountSubmit    = "warehouse.stock_count.submit"
	PermCountApprove   = "warehouse.stock_count.approve"
)

// Jenis entitas audit trail dokumen gudang (juga source_type ledger stok).
const (
	EntityInbound       = "inbound"
	EntityStockTransfer = "stock_transfer"
	EntityStockCount    = "stock_count"
	EntityCountSchedule = "stock_count_schedule"
)

// Jenis pemilik file foto (filestore.RegisterOwner).
const (
	FileOwnerQualityCheck = "quality_check"
	FileOwnerStockCount   = "stock_count"
)

// Role yang datanya dibatasi (Desain Role & Approval 3.5 scoping).
const (
	RoleMerchandiser      = "merchandiser"
	RoleFulfillmentLeader = "fulfillment_leader"
	RoleLogisticsLeader   = "logistics_leader"
)

// Status stok ledger (inventory).
const (
	StockAvailable  = "available"
	StockQuarantine = "quarantine"
	StockInTransit  = "in_transit"
	StockReserved   = "reserved"
)

// Jenis baris posting stok (inventory).
const (
	StockReceive = "receive"
	StockIssue   = "issue"
	StockMove    = "move"
)

// StockPlace adalah lokasi + status stok.
type StockPlace struct {
	LocationID uuid.UUID
	Status     string
}

// StockReservationRef menunjuk reservasi dokumen (baris yang mengambil stok reserved).
type StockReservationRef struct {
	SourceType   string
	SourceID     uuid.UUID
	SourceLineID *uuid.UUID
}

// StockLine adalah satu baris posting ke ledger stok.
type StockLine struct {
	Kind         string
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	BatchID      *uuid.UUID
	From         *StockPlace
	To           *StockPlace
	// UnitCost khusus receive; nil = HPP cost layer terakhir batch.
	UnitCost    *decimal.Decimal
	BatchReason string
	Reservation *StockReservationRef
	Note        string
}

// StockSource adalah aksi dokumen sumber mutasi stok.
type StockSource struct {
	Type         string
	ID           uuid.UUID
	Action       string
	Label        string
	BusinessDate clock.BusinessDate
}

// StockMovement adalah satu baris ledger hasil posting (Qty dan CostAmount bertanda).
type StockMovement struct {
	LineIndex  int
	LocationID uuid.UUID
	BatchID    *uuid.UUID
	Status     string
	Qty        decimal.Decimal
	CostAmount decimal.Decimal
}

// StockLineResult adalah hasil satu baris posting.
type StockLineResult struct {
	CostAmount decimal.Decimal
	Movements  []StockMovement
}

// StockResult adalah hasil posting.
type StockResult struct {
	Lines []StockLineResult
}

// StockReserveLine adalah satu baris reservasi available → reserved.
type StockReserveLine struct {
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	LocationID   uuid.UUID
	BatchID      *uuid.UUID
	Qty          decimal.Decimal
	BatchReason  string
}

// StockBalanceQuery adalah kriteria saldo stok.
type StockBalanceQuery struct {
	LocationIDs []uuid.UUID
	ProductID   *uuid.UUID
	BatchID     *uuid.UUID
	Statuses    []string
}

// StockBalance adalah saldo > 0 per produk × lokasi × batch × status dengan HPP rata-rata batch.
type StockBalance struct {
	ProductID  uuid.UUID
	LocationID uuid.UUID
	BatchID    *uuid.UUID
	Status     string
	Qty        decimal.Decimal
	UnitCost   decimal.Decimal
}

// BatchInfo adalah ringkasan batch untuk tampilan.
type BatchInfo struct {
	ID         uuid.UUID
	Number     string
	ExpiryDate *clock.BusinessDate
}

// StockLedger adalah API ledger stok modul inventory (diisi adapter di internal/app agar tidak terjadi siklus
// import inventory ↔ warehouse).
type StockLedger interface {
	Post(ctx context.Context, src StockSource, lines []StockLine) (StockResult, error)
	Reserve(ctx context.Context, src StockSource, lines []StockReserveLine) (StockResult, error)
	Release(ctx context.Context, src StockSource, ref StockReservationRef) error
	Balances(ctx context.Context, q StockBalanceQuery) ([]StockBalance, error)
	Batches(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]BatchInfo, error)
}

// Numberer memberi nomor dokumen (dipenuhi *document.Module).
type Numberer interface {
	Next(ctx context.Context, t document.Type, businessDate clock.BusinessDate) (string, error)
}

// Approvals mengajukan dokumen ke engine approval (dipenuhi *approval.Module).
type Approvals interface {
	Request(ctx context.Context, s approval.Subject) (approval.Request, error)
}

// Poster memposting jurnal otomatis (dipenuhi *accounting.Module).
type Poster interface {
	Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error)
}

// Divisions mencari ID divisi berdasarkan kode (dipenuhi *accounting.Module).
type Divisions interface {
	DivisionIDByCode(ctx context.Context, code string) (uuid.UUID, error)
}

// Files menautkan foto yang sudah diunggah ke dokumen (dipenuhi *filestore.Module).
type Files interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
}

// Products membaca master produk (dipenuhi *catalog.Module).
type Products interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]catalog.Product, error)
}

// Partners membaca mitra dan penugasan Merchandiser (dipenuhi *party.Module).
type Partners interface {
	PartnerByID(ctx context.Context, id uuid.UUID) (party.Partner, error)
	PartnersForMerchandiser(ctx context.Context, userID uuid.UUID) ([]party.Partner, error)
}

// Notifier mengirim notifikasi in-app (dipenuhi *notification.Module).
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// Users mencari user aktif per role (diisi adapter identity).
type Users interface {
	UserIDsWithRole(ctx context.Context, role string) ([]uuid.UUID, error)
	UserName(ctx context.Context, id uuid.UUID) (string, error)
}

// assigneeName mengisi nama petugas dari modul identity bila tersedia.
func (s *Service) assigneeName(ctx context.Context, d DocumentDeps, id *uuid.UUID) string {
	if id == nil || d.Users == nil {
		return ""
	}
	name, err := d.Users.UserName(ctx, *id)
	if err != nil {
		return ""
	}
	return name
}

// InboundOutcome adalah hasil QC satu baris Inbound yang diteruskan ke modul asal.
type InboundOutcome struct {
	InboundID     uuid.UUID
	InboundNumber string
	SourceType    string
	SourceID      uuid.UUID
	LineID        uuid.UUID
	SourceLineID  *uuid.UUID
	ProductID     uuid.UUID
	BatchID       *uuid.UUID
	PassedQty     decimal.Decimal
	RejectedQty   decimal.Decimal
	RejectReason  string
	// DestinationLocationID: lokasi stok Tersedia qty lulus.
	DestinationLocationID uuid.UUID
	// RejectLocationID: lokasi stok Karantina qty ditolak (Inbound bila ditahan, Gudang Retur bila dipindah).
	RejectLocationID *uuid.UUID
	// Completed true bila baris ini menutup Inbound (semua baris sudah QC).
	Completed bool
}

// InboundHandler dipanggil di transaksi QC agar modul asal mencatat keputusan (mis. reject penerimaan PRC-05).
type InboundHandler func(ctx context.Context, o InboundOutcome) error

// DocumentDeps adalah dependensi dokumen gudang (R1.2). Approvals, Poster, Divisions, Files, Partners,
// Notifier, dan Users opsional; fitur yang memakainya ditolak/dilewati bila kosong.
type DocumentDeps struct {
	Ledger    StockLedger
	Numbers   Numberer
	Products  Products
	Approvals Approvals
	Poster    Poster
	Divisions Divisions
	Files     Files
	Partners  Partners
	Notifier  Notifier
	Users     Users
}

type documentStore interface {
	InsertInbound(ctx context.Context, ib domain.Inbound, actorID uuid.UUID) (domain.Inbound, error)
	GetInbound(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Inbound, error)
	InboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.Inbound, error)
	SaveInboundQC(ctx context.Context, ib domain.Inbound, l domain.InboundLine, checks []domain.QualityCheck, actorID uuid.UUID) (domain.Inbound, error)
	QualityChecks(ctx context.Context, documentType string, documentID uuid.UUID) ([]domain.QualityCheck, error)
	ListInbounds(ctx context.Context, f postgres.InboundFilter, page paging.Page) (paging.Result[postgres.InboundSummary], error)

	InsertTransfer(ctx context.Context, t domain.StockTransfer, actorID uuid.UUID) (domain.StockTransfer, error)
	GetTransfer(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.StockTransfer, error)
	UpdateTransfer(ctx context.Context, t domain.StockTransfer, replaceLines bool, actorID uuid.UUID) (domain.StockTransfer, error)
	ListTransfers(ctx context.Context, f postgres.TransferFilter, page paging.Page) (paging.Result[postgres.TransferSummary], error)
	ListInTransit(ctx context.Context, locationID *uuid.UUID, page paging.Page) (paging.Result[postgres.InTransitLine], error)

	InsertStockCount(ctx context.Context, c domain.StockCount, actorID uuid.UUID) (domain.StockCount, error)
	GetStockCount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.StockCount, error)
	UpdateStockCount(ctx context.Context, c domain.StockCount, mode postgres.LineWrite, changed []int, actorID uuid.UUID) (domain.StockCount, error)
	ListStockCounts(ctx context.Context, f postgres.StockCountFilter, page paging.Page) (paging.Result[postgres.StockCountSummary], error)

	InsertSchedule(ctx context.Context, sc domain.CountSchedule, actorID uuid.UUID) (domain.CountSchedule, error)
	UpdateSchedule(ctx context.Context, sc domain.CountSchedule, actorID uuid.UUID) (domain.CountSchedule, error)
	GetSchedule(ctx context.Context, id uuid.UUID) (domain.CountSchedule, error)
	ListSchedules(ctx context.Context) ([]domain.CountSchedule, error)
	DueSchedules(ctx context.Context, today clock.BusinessDate) ([]domain.CountSchedule, error)
}

var _ documentStore = (*postgres.Store)(nil)

// documents menyimpan dependensi dokumen yang dipasang setelah modul lain disusun.
type documents struct {
	mu       sync.RWMutex
	deps     DocumentDeps
	handlers map[string]InboundHandler
}

// SetDocumentDeps memasang dependensi dokumen gudang (dipanggil saat wiring setelah inventory dibuat).
func (s *Service) SetDocumentDeps(d DocumentDeps) {
	s.docs.mu.Lock()
	defer s.docs.mu.Unlock()
	s.docs.deps = d
}

// RegisterInboundHandler mendaftarkan callback hasil QC untuk jenis dokumen asal (registrasi terakhir menang).
func (s *Service) RegisterInboundHandler(sourceType string, h InboundHandler) {
	s.docs.mu.Lock()
	defer s.docs.mu.Unlock()
	if s.docs.handlers == nil {
		s.docs.handlers = map[string]InboundHandler{}
	}
	s.docs.handlers[sourceType] = h
}

func (s *Service) inboundHandler(sourceType string) InboundHandler {
	s.docs.mu.RLock()
	defer s.docs.mu.RUnlock()
	return s.docs.handlers[sourceType]
}

// deps mengembalikan dependensi dokumen; PROVIDER_UNAVAILABLE bila modul inventory/document belum dipasang.
func (s *Service) deps() (DocumentDeps, error) {
	s.docs.mu.RLock()
	defer s.docs.mu.RUnlock()
	d := s.docs.deps
	if d.Ledger == nil || d.Numbers == nil || d.Products == nil || s.Docs == nil {
		return d, apperr.WithMessage(apperr.CodeProviderUnavailable,
			"Dokumen gudang membutuhkan modul inventory, catalog, dan document aktif.")
	}
	return d, nil
}

func actorName(a actor.Actor) string {
	if a.FullName != "" {
		return a.FullName
	}
	return a.Email
}

func nowPtr(t time.Time) *time.Time { return &t }

func clockToday(s *Service) clock.BusinessDate { return clock.Today(s.Clock) }

func itoaInt(i int) string { return strconv.Itoa(i) }

// Refs adalah data tampilan produk, batch, dan lokasi untuk dokumen gudang.
type Refs struct {
	Products  map[uuid.UUID]catalog.Product
	Batches   map[uuid.UUID]BatchInfo
	Locations map[uuid.UUID]domain.Location
}

// refs membaca produk, batch, dan lokasi sekaligus.
func (s *Service) refs(ctx context.Context, d DocumentDeps, products, batches, locations []uuid.UUID) (Refs, error) {
	out := Refs{}
	var err error
	if out.Products, err = d.Products.Products(ctx, uniq(products)); err != nil {
		return Refs{}, err
	}
	if b := uniq(batches); len(b) > 0 {
		if out.Batches, err = d.Ledger.Batches(ctx, b); err != nil {
			return Refs{}, err
		}
	}
	if out.Locations, err = s.LocationsByIDs(ctx, uniq(locations)); err != nil {
		return Refs{}, err
	}
	return out, nil
}

func uniq(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func derefIDs(ids ...*uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != nil {
			out = append(out, *id)
		}
	}
	return out
}

// activeLocation membaca lokasi yang aktif dan belum dihapus; selain itu error field.
func (s *Service) activeLocation(ctx context.Context, id uuid.UUID, field string) (domain.Location, error) {
	l, err := s.Store.GetLocation(ctx, id, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.Location{}, apperr.Validation(map[string]string{field: "Lokasi tidak ditemukan"})
	}
	if err != nil {
		return domain.Location{}, err
	}
	if !l.IsActive || l.Deleted() {
		return domain.Location{}, apperr.Validation(map[string]string{field: "Lokasi " + l.Code + " tidak aktif"})
	}
	return l, nil
}

// standardLocation mencari lokasi standar jenis t di gudang.
func (s *Service) standardLocation(ctx context.Context, warehouseID uuid.UUID, t domain.LocationType) (domain.Location, bool, error) {
	list, err := s.FindLocations(ctx, LocationFilter{WarehouseID: &warehouseID, Types: []domain.LocationType{t}})
	if err != nil || len(list) == 0 {
		return domain.Location{}, false, err
	}
	for _, l := range list {
		if l.IsStandard {
			return l, true, nil
		}
	}
	return list[0], true, nil
}

// attachPhotos menautkan foto ke pemilik bila modul filestore aktif; foto tanpa filestore ditolak.
func attachPhotos(ctx context.Context, d DocumentDeps, ids []uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if d.Files == nil {
		return apperr.Validation(map[string]string{"photoFileIds": "Penyimpanan file belum aktif"})
	}
	for _, id := range ids {
		if err := d.Files.Attach(ctx, id, ownerType, ownerID); err != nil {
			return err
		}
	}
	return nil
}

// notify mengirim notifikasi in-app bila Notifier dipasang; kegagalan dicatat di log dan tidak menggagalkan dokumen.
func (s *Service) notify(ctx context.Context, d DocumentDeps, userIDs []uuid.UUID, kind, title, body, link string) {
	ids := uniq(userIDs)
	if d.Notifier == nil || len(ids) == 0 {
		return
	}
	if err := d.Notifier.Notify(ctx, ids, kind, title, body, link); err != nil && s.Log != nil {
		s.Log.WarnContext(ctx, "warehouse notify", "kind", kind, "error", err)
	}
}

// roleUsers membaca user aktif suatu role bila Users dipasang.
func (s *Service) roleUsers(ctx context.Context, d DocumentDeps, role string) []uuid.UUID {
	if d.Users == nil {
		return nil
	}
	ids, err := d.Users.UserIDsWithRole(ctx, role)
	if err != nil {
		if s.Log != nil {
			s.Log.WarnContext(ctx, "warehouse role users", "role", role, "error", err)
		}
		return nil
	}
	return ids
}
