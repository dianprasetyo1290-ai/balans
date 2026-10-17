// Package service berisi use case pengadaan: Purchase Request, Purchase Order (barang/packaging, jasa maklon,
// reprocess) dengan approval berjenjang, notifikasi ETA H-1, dan uang muka pembelian A-19 (S-201 – S-205).
package service

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// Permission modul procurement (Desain Role & Approval bagian 3.2).
const (
	PermRequestRead   = "procurement.purchase_request.read"
	PermRequestWrite  = "procurement.purchase_request.write"
	PermRequestSubmit = "procurement.purchase_request.submit"
	PermOrderRead     = "procurement.purchase_order.read"
	PermOrderWrite    = "procurement.purchase_order.write"
	PermOrderSubmit   = "procurement.purchase_order.submit"
	PermOrderCancel   = "procurement.purchase_order.cancel"
	PermAdvanceRead   = "procurement.purchase_advance.read"
	PermAdvanceWrite  = "procurement.purchase_advance.write"
	PermAdvancePost   = "procurement.purchase_advance.post"
)

// Jenis entitas audit trail. Sama dengan subject_type approval agar keputusan approval ikut tampil di tab Riwayat.
const (
	EntityPurchaseRequest = "purchase_request"
	EntityPurchaseOrder   = "purchase_order"
	EntityPurchaseAdvance = "purchase_advance"
)

// RoleLogisticsLeader menerima notifikasi ETA H-1 (Desain Alur Lintas Modul 1).
const RoleLogisticsLeader = "logistics_leader"

// RuleAdvance adalah posting rule uang muka pembelian.
const RuleAdvance = "A-19"

// DocumentType adalah jenis nomor dokumen yang diminta ke modul document.
type DocumentType string

const (
	DocPurchaseRequest DocumentType = "PR"
	DocPurchaseOrder   DocumentType = "PO"
	// DocPayment: uang muka adalah pembayaran keluar sehingga memakai nomor BYR.
	DocPayment DocumentType = "BYR"
)

// ProductRef adalah produk katalog untuk validasi dan tampilan.
type ProductRef struct {
	ID       uuid.UUID
	SKU      string
	Name     string
	UOM      string
	Type     string
	IsBundle bool
	IsActive bool
	Deleted  bool
}

// SupplierRef adalah pemasok/maklon dari modul party.
type SupplierRef struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	IsTollManufacturer bool
	PaymentTermDays    int
	Address            string
	Phone              string
	Email              string
	PICName            string
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

// Jenis lokasi yang dipakai PO maklon/reprocess (kode warehouse.LocationType).
const (
	LocationRawMaterial   = "raw_material"
	LocationGoodsMaterial = "goods_material"
)

// UserRef adalah pengguna aktif dari modul identity.
type UserRef struct {
	ID    uuid.UUID
	Name  string
	Email string
}

// Option adalah pilihan kode–nama (divisi, akun).
type Option struct {
	ID   uuid.UUID
	Code string
	Name string
}

// Catalog membaca produk.
type Catalog interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error)
}

// Suppliers membaca pemasok; ok false jika tidak ada atau dihapus.
type Suppliers interface {
	Supplier(ctx context.Context, id uuid.UUID) (SupplierRef, bool, error)
}

// Locations membaca lokasi stok.
type Locations interface {
	// TollLocation mengembalikan lokasi eksternal maklon aktif milik pemasok; nil jika belum ada.
	TollLocation(ctx context.Context, supplierID uuid.UUID) (*LocationRef, error)
	Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]LocationRef, error)
}

// Ledger membaca master accounting dan membuat jurnal otomatis.
type Ledger interface {
	DivisionOptions(ctx context.Context) ([]Option, error)
	DivisionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
	CheckDivision(ctx context.Context, id uuid.UUID, at clock.BusinessDate, field string) error
	CashBankAccounts(ctx context.Context) ([]Option, error)
	Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error)
	Reverse(ctx context.Context, req accounting.ReversalRequest) (accounting.JournalEntry, error)
}

// Approvals mengajukan dokumen ke engine approval.
type Approvals interface {
	Request(ctx context.Context, s approval.Subject) (approval.Request, error)
}

// Numberer mengambil nomor dokumen dari modul document di transaksi aktif.
type Numberer interface {
	Next(ctx context.Context, t DocumentType, businessDate clock.BusinessDate) (string, error)
}

// Users membaca pengguna dari modul identity.
type Users interface {
	UsersWithRole(ctx context.Context, role string) ([]UserRef, error)
	UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Notifier mengirim notifikasi in-app dan email.
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
	QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error)
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	ListRequests(ctx context.Context, f postgres.RequestFilter, page paging.Page) (paging.Result[domain.PurchaseRequest], error)
	GetRequest(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PurchaseRequest, error)
	InsertRequest(ctx context.Context, r domain.PurchaseRequest, actorID uuid.UUID) (domain.PurchaseRequest, error)
	SaveRequest(ctx context.Context, r domain.PurchaseRequest, version int, actorID uuid.UUID, replaceLines bool) error
	SetRequestLinesFulfilled(ctx context.Context, lines []domain.RequestLine) error
	RequestLineOrders(ctx context.Context, requestID uuid.UUID) ([]postgres.RequestLineOrder, error)
	RequestIDsByLines(ctx context.Context, lineIDs []uuid.UUID) ([]uuid.UUID, error)

	ListOrders(ctx context.Context, f postgres.OrderFilter, page paging.Page) (paging.Result[domain.PurchaseOrder], error)
	GetOrder(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PurchaseOrder, error)
	InsertOrder(ctx context.Context, o domain.PurchaseOrder, actorID uuid.UUID) (domain.PurchaseOrder, error)
	SaveOrder(ctx context.Context, o domain.PurchaseOrder, version int, actorID uuid.UUID, replaceLines bool) error
	SaveOrderLineQty(ctx context.Context, lines []domain.OrderLine) error
	OrdersDueForETA(ctx context.Context, expected clock.BusinessDate) ([]uuid.UUID, error)
	MarkETANotified(ctx context.Context, id uuid.UUID, at time.Time) (bool, error)

	ListAdvances(ctx context.Context, f postgres.AdvanceFilter, page paging.Page) (paging.Result[domain.Advance], error)
	GetAdvance(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Advance, error)
	AdvancesByOrder(ctx context.Context, orderID uuid.UUID) ([]domain.Advance, error)
	InsertAdvance(ctx context.Context, a domain.Advance, actorID uuid.UUID) (uuid.UUID, error)
	SaveAdvance(ctx context.Context, a domain.Advance, version int, actorID uuid.UUID) error
}

// Deps adalah dependensi Service.
type Deps struct {
	Store     store
	Tx        txRunner
	Audit     audit.Recorder
	Clock     clock.Clock
	Log       *slog.Logger
	Catalog   Catalog
	Suppliers Suppliers
	Locations Locations
	Ledger    Ledger
	Approvals Approvals
	Numberer  Numberer
	Users     Users
	Notifier  Notifier
	// BaseURL adalah URL ERP untuk tautan di email.
	BaseURL string
}

// Service adalah use case modul procurement.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

// Path detail dokumen di ERP (dipakai approval Subject.Link dan notifikasi).
const (
	requestPathPrefix = "/procurement/purchase-requests/"
	orderPathPrefix   = "/procurement/purchase-orders/"
)

// RequestPath adalah path detail PR.
func RequestPath(id uuid.UUID) string { return requestPathPrefix + id.String() }

// OrderPath adalah path detail PO.
func OrderPath(id uuid.UUID) string { return orderPathPrefix + id.String() }

// productInfos mengubah ProductRef menjadi data validasi domain.
func productInfos(refs map[uuid.UUID]ProductRef) map[uuid.UUID]domain.ProductInfo {
	out := make(map[uuid.UUID]domain.ProductInfo, len(refs))
	for id, p := range refs {
		out[id] = domain.ProductInfo{ID: id, Type: p.Type, IsBundle: p.IsBundle, IsActive: p.IsActive, Deleted: p.Deleted}
	}
	return out
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// filterActions menyaring aksi status dokumen dengan permission actor.
func filterActions(ctx context.Context, actions []domain.Action, perms map[domain.Action]string) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	for _, act := range actions {
		if p, ok := perms[act]; ok && a.Has(p) {
			out = append(out, string(act))
		}
	}
	return out
}

// DivisionOptions mengembalikan divisi aktif untuk form PR/PO.
func (s *Service) DivisionOptions(ctx context.Context) ([]Option, error) {
	if _, err := actor.RequireAny(ctx, PermRequestWrite, PermOrderWrite, PermRequestRead, PermOrderRead); err != nil {
		return nil, err
	}
	return s.Ledger.DivisionOptions(ctx)
}

// CashBankAccounts mengembalikan akun Kas & Bank untuk form uang muka.
func (s *Service) CashBankAccounts(ctx context.Context) ([]Option, error) {
	if _, err := actor.RequireAny(ctx, PermAdvanceWrite, PermAdvancePost, PermAdvanceRead); err != nil {
		return nil, err
	}
	return s.Ledger.CashBankAccounts(ctx)
}
