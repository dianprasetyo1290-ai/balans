package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/inventory"
	"balans/internal/logistics/domain"
	"balans/internal/logistics/postgres"
	"balans/internal/party"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/warehouse"
)

// Permission logistik (Desain Role & Approval 3.3, 3.5).
const (
	PermDORead        = "logistics.delivery_order.read"
	PermDOWrite       = "logistics.delivery_order.write"
	PermDOPost        = "logistics.delivery_order.post"
	PermDOCancel      = "logistics.delivery_order.cancel"
	PermShipmentRead  = "logistics.shipment.read"
	PermShipmentWrite = "logistics.shipment.write"
	PermShipmentPost  = "logistics.shipment.post"
	PermLabelPrint    = "logistics.label.post"
)

// Jenis entitas audit, dokumen asal, dan pemilik file.
const (
	EntityDeliveryOrder = "delivery_order"
	EntityShipment      = "shipment"
	EntityCourier       = "courier"
	// SourceSalesOrder adalah dokumen asal DO dari Sales Order (handler bawaan modul logistics).
	SourceSalesOrder = "sales_order"
	// FileOwnerDeliveryProof adalah foto/tanda terima barang diterima (kurir internal dari HP).
	FileOwnerDeliveryProof = "delivery_proof"
	// RoleMerchandiser adalah role kurir internal.
	RoleMerchandiser = "merchandiser"
)

// Warehouse adalah API modul warehouse yang dipakai DO (Outbound, lokasi).
type Warehouse interface {
	CreateOutbound(ctx context.Context, r warehouse.OutboundRequest) (warehouse.Outbound, error)
	OutboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (warehouse.Outbound, error)
	CancelOutbound(ctx context.Context, sourceType string, sourceID uuid.UUID, reason string) error
	Location(ctx context.Context, id uuid.UUID) (warehouse.Location, error)
	RegisterOutboundHandler(sourceType string, h warehouse.OutboundHandler)
}

// Products membaca master produk (dipenuhi *catalog.Module).
type Products interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]catalog.Product, error)
}

// Batches membaca batch (dipenuhi *inventory.Module).
type Batches interface {
	Batches(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventory.Batch, error)
}

// Partners membaca kode produk mitra (dipenuhi *party.Module).
type Partners interface {
	ProductCodeFor(ctx context.Context, partnerID, productID uuid.UUID) (party.ProductCode, bool, error)
}

// Numberer memberi nomor DO (dipenuhi *document.Module).
type Numberer interface {
	Next(ctx context.Context, t document.Type, businessDate clock.BusinessDate) (string, error)
}

// Files menautkan foto/tanda terima ke pengiriman (dipenuhi *filestore.Module).
type Files interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
}

// Users memeriksa role user (adapter identity).
type Users interface {
	// UserWithRole mengembalikan nama user aktif bila punya role; ok false bila tidak.
	UserWithRole(ctx context.Context, userID uuid.UUID, role string) (name string, ok bool, err error)
	// UsersWithRole mengembalikan user aktif dengan role (urut nama).
	UsersWithRole(ctx context.Context, role string) ([]UserOption, error)
}

// UserOption adalah user untuk dropdown (mis. Merchandiser kurir internal).
type UserOption struct {
	ID   uuid.UUID
	Name string
}

// Notifier mengirim notifikasi in-app (dipenuhi *notification.Module).
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// SalesAllocation adalah alokasi stok Sales Order yang belum dikirim.
type SalesAllocation struct {
	ID          uuid.UUID
	LineID      uuid.UUID
	ProductID   uuid.UUID
	LocationID  uuid.UUID
	Outstanding decimal.Decimal
	// Gap: qty belum punya stok reserved (batch ditolak QC Outbound sebelumnya).
	Gap     decimal.Decimal
	Batches []warehouse.ReplacementBatch
}

// SalesOrder adalah Sales Order Diproses yang siap dibuatkan DO.
type SalesOrder struct {
	ID             uuid.UUID
	Number         string
	Marketplace    bool
	PartnerID      *uuid.UUID
	CustomerID     *uuid.UUID
	RecipientName  string
	RecipientPhone string
	ShipToAddress  string
	LocationID     uuid.UUID
	Allocations    []SalesAllocation
}

// SalesReallocation adalah hasil ganti batch alokasi Sales Order.
type SalesReallocation struct {
	QuarantineLocationID uuid.UUID
	Replacements         []warehouse.ReplacementBatch
	Shortfall            decimal.Decimal
}

// SalesOrders adalah API modul sales untuk DO (adapter di internal/app; sales berada di tingkat atas).
type SalesOrders interface {
	ShippableOrder(ctx context.Context, id uuid.UUID) (SalesOrder, error)
	// Reallocate: qty > 0 batch ditolak QC → Karantina + pengganti FEFO; qty nol = isi kekurangan alokasi.
	Reallocate(ctx context.Context, orderID, allocationID uuid.UUID, batchID *uuid.UUID, qty decimal.Decimal, ref, label string) (SalesReallocation, error)
	// Ship: stok keluar + jurnal HPP (A-06) atau penjualan marketplace saat serah ke kurir (A-10).
	Ship(ctx context.Context, orderID, deliveryOrderID uuid.UUID, label string, date clock.BusinessDate, lines map[uuid.UUID]decimal.Decimal) error
	// Deliver: pesanan diterima (Selesai) bila seluruhnya sudah dikirim.
	Deliver(ctx context.Context, orderID uuid.UUID) error
}

// Deps adalah dependensi service logistik. Sales, Partners, Files, Users, Notifier opsional.
type Deps struct {
	Store     *postgres.Store
	Tx        *db.TxManager
	Audit     audit.Recorder
	Clock     clock.Clock
	Log       *slog.Logger
	Warehouse Warehouse
	Products  Products
	Batches   Batches
	Numberer  Numberer
	Sales     SalesOrders
	Partners  Partners
	Files     Files
	Users     Users
	Notifier  Notifier
}

// Service adalah use case kurir, Delivery Order, dan pengiriman.
type Service struct {
	Deps
	mu       sync.RWMutex
	handlers map[string]DeliveryHandler
}

// New membuat service dan mendaftarkan handler Outbound DO serta handler bawaan DO Sales Order.
func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	s := &Service{Deps: d, handlers: map[string]DeliveryHandler{}}
	if d.Sales != nil {
		s.handlers[SourceSalesOrder] = s.salesHandler()
	}
	if d.Warehouse != nil {
		d.Warehouse.RegisterOutboundHandler(warehouse.OutboundSourceDeliveryOrder, warehouse.OutboundHandler{
			Replace: s.onOutboundReject, Completed: s.onOutboundCompleted,
		})
	}
	return s
}

func (s *Service) now() time.Time { return s.Clock.Now().Truncate(time.Microsecond) }

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

func (s *Service) ready() error {
	if s.Store == nil || s.Warehouse == nil || s.Products == nil || s.Batches == nil || s.Numberer == nil {
		return apperr.WithMessage(apperr.CodeProviderUnavailable,
			"Delivery Order membutuhkan modul warehouse, inventory, catalog, dan document aktif.")
	}
	return nil
}

// DeliveryShipped adalah barang DO yang diserahkan ke kurir: qty lulus QC Outbound per baris dokumen asal.
type DeliveryShipped struct {
	DeliveryOrder domain.DeliveryOrder
	// Lines: qty per SourceLineID (mis. alokasi Sales Order).
	Lines map[uuid.UUID]decimal.Decimal
	// Outbound beserta batch yang benar-benar dikirim.
	Outbound warehouse.Outbound
}

// DeliveryHandler adalah callback dokumen asal DO, dipanggil di transaksi aksi DO/Outbound.
type DeliveryHandler struct {
	// Replace wajib agar QC Outbound bisa menolak barang: pindahkan stok reserved batch ditolak ke Karantina dan
	// alokasikan stok pengganti FEFO (qty nol = isi baris ditahan).
	Replace func(ctx context.Context, do domain.DeliveryOrder, r warehouse.OutboundRejection) (warehouse.OutboundReplacement, error)
	// Shipped wajib: stok keluar dan jurnal dokumen asal (A-06/A-10 penjualan, A-07 konsinyasi, A-24 buffer).
	Shipped func(ctx context.Context, sh DeliveryShipped) error
	// Delivered opsional: barang diterima.
	Delivered func(ctx context.Context, do domain.DeliveryOrder) error
	// Cancelled opsional: DO dibatalkan sebelum dikirim (reservasi tetap milik dokumen asal).
	Cancelled func(ctx context.Context, do domain.DeliveryOrder) error
}

// RegisterHandler mendaftarkan callback dokumen asal DO (registrasi terakhir menang).
func (s *Service) RegisterHandler(sourceType string, h DeliveryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[sourceType] = h
}

func (s *Service) handler(sourceType string) (DeliveryHandler, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[sourceType]
	if !ok || h.Shipped == nil {
		return DeliveryHandler{}, apperr.WithMessage(apperr.CodeProviderUnavailable,
			"Jenis dokumen asal DO "+sourceType+" belum didukung (modul asal tidak aktif).")
	}
	return h, nil
}

func (s *Service) notify(ctx context.Context, ids []uuid.UUID, kind, title, body, link string) {
	if s.Notifier == nil || len(ids) == 0 {
		return
	}
	if err := s.Notifier.Notify(ctx, ids, kind, title, body, link); err != nil && s.Log != nil {
		s.Log.WarnContext(ctx, "logistics notify", "kind", kind, "error", err)
	}
}
