package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/logistics/domain"
	"balans/internal/logistics/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

func doSnapshot(d domain.DeliveryOrder) map[string]any {
	return map[string]any{
		"number": d.Number, "status": d.Status, "deliveryType": d.Type, "source": d.SourceLabel, "lines": len(d.Lines),
	}
}

func actorName(a actor.Actor) string {
	if a.FullName != "" {
		return a.FullName
	}
	return a.Email
}

func batchQty(bs []warehouse.ReplacementBatch) []domain.BatchQty {
	out := make([]domain.BatchQty, 0, len(bs))
	for _, b := range bs {
		if b.Qty.IsPositive() {
			out = append(out, domain.BatchQty{BatchID: b.BatchID, Qty: b.Qty.StringFixed(domain.QtyScale)})
		}
	}
	return out
}

// DeliveryRequest adalah DO yang dibuat modul lain lewat API publik (mis. DO konsinyasi, buffer vending). Stok
// harus sudah reserved oleh dokumen asal; batch per baris dipakai sebagai picking list Outbound.
type DeliveryRequest = domain.Input

// CreateDeliveryOrder membuat DO berstatus Draft di transaksi aktif tanpa cek permission (API publik). Dokumen
// asal wajib mendaftarkan DeliveryHandler untuk SourceType-nya.
func (s *Service) CreateDeliveryOrder(ctx context.Context, in DeliveryRequest) (domain.DeliveryOrder, error) {
	if err := s.ready(); err != nil {
		return domain.DeliveryOrder{}, err
	}
	if _, err := s.handler(in.SourceType); err != nil {
		return domain.DeliveryOrder{}, err
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = s.today()
	}
	d, err := domain.New(in)
	if err != nil {
		return domain.DeliveryOrder{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if d, err = s.Store.InsertDeliveryOrder(ctx, d, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityDeliveryOrder, EntityID: d.ID, EntityLabel: d.Label(),
			After: doSnapshot(d), Note: "Dibuat dari " + d.SourceLabel,
		})
	})
	return d, err
}

// CreateFromSalesOrder membuat DO Draft dari Sales Order Diproses (S-309): satu baris per alokasi yang belum
// dikirim dengan batch FEFO hasil alokasi. Kekurangan stok karena QC Outbound sebelumnya diisi ulang dulu;
// bila stok tetap kurang ditolak STOCK_INSUFFICIENT.
func (s *Service) CreateFromSalesOrder(ctx context.Context, orderID uuid.UUID, note string) (domain.DeliveryOrder, error) {
	if _, err := actor.Require(ctx, PermDOWrite); err != nil {
		return domain.DeliveryOrder{}, err
	}
	if s.Sales == nil {
		return domain.DeliveryOrder{}, apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul sales tidak aktif.")
	}
	var out domain.DeliveryOrder
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Sales.ShippableOrder(ctx, orderID)
		if err != nil {
			return err
		}
		refilled := false
		for _, a := range o.Allocations {
			if !a.Outstanding.IsPositive() || !a.Gap.IsPositive() {
				continue
			}
			r, err := s.Sales.Reallocate(ctx, o.ID, a.ID, nil, decimal.Zero, "do-refill:"+uuid.NewString(), "")
			if err != nil {
				return err
			}
			if r.Shortfall.IsPositive() {
				return apperr.WithMessage(apperr.CodeStockInsufficient, fmt.Sprintf(
					"Stok tersedia kurang %s untuk sisa pesanan %s; tunggu stok masuk.", r.Shortfall.String(), o.Number))
			}
			refilled = true
		}
		if refilled {
			if o, err = s.Sales.ShippableOrder(ctx, orderID); err != nil {
				return err
			}
		}
		in := domain.Input{
			Type: domain.DeliverySales, SourceType: SourceSalesOrder, SourceID: o.ID, SourceLabel: o.Number,
			SalesOrderID: &o.ID, PartnerID: o.PartnerID, CustomerID: o.CustomerID, RecipientName: o.RecipientName,
			RecipientPhone: o.RecipientPhone, ShipToAddress: o.ShipToAddress, LocationID: o.LocationID, Note: note,
		}
		if o.Marketplace {
			in.Type = domain.DeliveryMarketplace
		}
		for _, a := range o.Allocations {
			if !a.Outstanding.IsPositive() {
				continue
			}
			allocID, lineID := a.ID, a.LineID
			in.Lines = append(in.Lines, domain.LineInput{
				SourceLineID: &allocID, SalesOrderLineID: &lineID, ProductID: a.ProductID, Qty: a.Outstanding,
				Batches: batchQty(a.Batches),
			})
		}
		out, err = s.CreateDeliveryOrder(ctx, in)
		return err
	})
	return out, err
}

type doMutation func(ctx context.Context, d *domain.DeliveryOrder) (string, error)

func (s *Service) mutate(ctx context.Context, id uuid.UUID, version *int, action audit.Action, fn doMutation) (domain.DeliveryOrder, error) {
	var out domain.DeliveryOrder
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDeliveryOrder(ctx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != d.Version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := doSnapshot(d)
		note, err := fn(ctx, &d)
		if err != nil {
			return err
		}
		if out, err = s.Store.SaveDeliveryOrder(ctx, d, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityDeliveryOrder, EntityID: out.ID, EntityLabel: out.Label(),
			Before: before, After: doSnapshot(out), Note: note,
		})
	})
	return out, err
}

// Release: Logistik Leader meneruskan DO ke gudang — nomor DO diberikan dan Outbound (picking list per batch)
// dibuat. Permission `logistics.delivery_order.post`.
func (s *Service) Release(ctx context.Context, id uuid.UUID, version int) (domain.DeliveryOrder, error) {
	a, err := actor.Require(ctx, PermDOPost)
	if err != nil {
		return domain.DeliveryOrder{}, err
	}
	if err := s.ready(); err != nil {
		return domain.DeliveryOrder{}, err
	}
	return s.mutate(ctx, id, &version, audit.ActionStatusChange, func(ctx context.Context, d *domain.DeliveryOrder) (string, error) {
		number, err := s.Numberer.Next(ctx, document.TypeDeliveryOrder, s.today())
		if err != nil {
			return "", err
		}
		if err := d.Release(number, a.UserID, actorName(a), s.now()); err != nil {
			return "", err
		}
		req := warehouse.OutboundRequest{
			SourceType: warehouse.OutboundSourceDeliveryOrder, SourceID: d.ID, SourceLabel: d.Number, LocationID: d.LocationID,
			RecipientName: d.RecipientName, RecipientPhone: d.RecipientPhone, RecipientAddress: d.ShipToAddress, Note: d.Note,
		}
		for _, l := range d.Lines {
			if len(l.Batches) == 0 {
				req.Lines = append(req.Lines, warehouse.OutboundLineInput{SourceLineID: l.SourceLineID, ProductID: l.ProductID, Qty: l.Qty})
				continue
			}
			for _, b := range l.Batches {
				req.Lines = append(req.Lines, warehouse.OutboundLineInput{
					SourceLineID: l.SourceLineID, ProductID: l.ProductID, BatchID: b.BatchID, Qty: b.Quantity(),
				})
			}
		}
		ob, err := s.Warehouse.CreateOutbound(ctx, req)
		if err != nil {
			return "", err
		}
		d.OutboundID = &ob.ID
		return "Release oleh " + actorName(a) + "; Outbound " + ob.Number, nil
	})
}

// onOutboundCompleted: Outbound DO selesai (semua baris lulus QC) → DO siap dikirim.
func (s *Service) onOutboundCompleted(ctx context.Context, r warehouse.OutboundResult) error {
	_, err := s.mutate(ctx, r.Outbound.SourceID, nil, audit.ActionUpdate, func(_ context.Context, d *domain.DeliveryOrder) (string, error) {
		d.OutboundDone(s.now())
		return "Outbound " + r.Outbound.Number + " selesai QC; siap diserahkan ke kurir", nil
	})
	return err
}

// onOutboundReject meneruskan QC gagal Outbound DO ke dokumen asal DO.
func (s *Service) onOutboundReject(ctx context.Context, r warehouse.OutboundRejection) (warehouse.OutboundReplacement, error) {
	d, err := s.Store.GetDeliveryOrder(ctx, r.SourceID, false)
	if err != nil {
		return warehouse.OutboundReplacement{}, err
	}
	h, err := s.handler(d.SourceType)
	if err != nil {
		return warehouse.OutboundReplacement{}, err
	}
	if h.Replace == nil {
		return warehouse.OutboundReplacement{}, apperr.WithMessage(apperr.CodeProviderUnavailable,
			"Dokumen asal DO belum mendukung stok pengganti QC Outbound.")
	}
	return h.Replace(ctx, d, r)
}

// ShipInput adalah isian serah barang ke kurir.
type ShipInput = domain.ShipInput

// Ship mencatat barang diserahkan ke kurir (LOG-02, LOG-03): kurir dari master, resi manual, AWB marketplace,
// kurir internal wajib Merchandiser. Hanya qty lulus QC Outbound yang dikirim; dokumen asal mencatat stok keluar
// dan jurnalnya (Sales Order: A-06, marketplace A-10 saat serah ke kurir). Permission `logistics.shipment.write`.
func (s *Service) Ship(ctx context.Context, id uuid.UUID, version int, in ShipInput) (domain.DeliveryOrder, error) {
	a, err := actor.Require(ctx, PermShipmentWrite)
	if err != nil {
		return domain.DeliveryOrder{}, err
	}
	if err := s.ready(); err != nil {
		return domain.DeliveryOrder{}, err
	}
	var courierUser *uuid.UUID
	out, err := s.mutate(ctx, id, &version, audit.ActionStatusChange, func(ctx context.Context, d *domain.DeliveryOrder) (string, error) {
		h, err := s.handler(d.SourceType)
		if err != nil {
			return "", err
		}
		c, err := s.Store.GetCourier(ctx, in.CourierID)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return "", apperr.Validation(map[string]string{"courierId": "Kurir tidak ditemukan"})
		}
		if err != nil {
			return "", err
		}
		name := ""
		if c.IsInternal && in.InternalCourierUserID != nil {
			if s.Users == nil {
				return "", apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul identity tidak aktif.")
			}
			n, ok, err := s.Users.UserWithRole(ctx, *in.InternalCourierUserID, RoleMerchandiser)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", apperr.Validation(map[string]string{"internalCourierUserId": "Kurir internal harus user aktif dengan role Merchandiser"})
			}
			name = n
		}
		ob, err := s.Warehouse.OutboundBySource(ctx, warehouse.OutboundSourceDeliveryOrder, d.ID)
		if err != nil {
			return "", err
		}
		shipped := map[uuid.UUID]decimal.Decimal{}
		for _, l := range ob.Lines {
			if l.Status == warehouse.OutboundPassed && l.SourceLineID != nil {
				shipped[*l.SourceLineID] = shipped[*l.SourceLineID].Add(l.PassedQty)
			}
		}
		sh, err := domain.NewShipment(*d, c, in, name, len(ob.Packages), s.now())
		if err != nil {
			return "", err
		}
		lines := make(map[uuid.UUID]decimal.Decimal, len(shipped))
		for k, v := range shipped {
			lines[k] = v
		}
		if err := d.ApplyShipment(shipped, s.now()); err != nil {
			return "", err
		}
		if err := h.Shipped(ctx, DeliveryShipped{DeliveryOrder: *d, Lines: lines, Outbound: ob}); err != nil {
			return "", err
		}
		if sh, err = s.Store.InsertShipment(ctx, sh, a.UserID); err != nil {
			return "", err
		}
		if err := s.Store.InsertShipmentEvent(ctx, sh.ID, postgres.ShipmentEvent{
			Status: sh.Status, OccurredAt: sh.ShippedAt, Source: "manual", Note: "Diserahkan ke " + c.Name, CreatedByName: actorName(a),
		}, a.UserID); err != nil {
			return "", err
		}
		courierUser = sh.InternalCourierUserID
		note := "Diserahkan ke " + c.Name
		if sh.TrackingNumber != "" {
			note += ", resi " + sh.TrackingNumber
		}
		if sh.MarketplaceAWB != "" {
			note += ", AWB " + sh.MarketplaceAWB
		}
		if name != "" {
			note += ", diantar " + name
		}
		return note, nil
	})
	if err != nil {
		return domain.DeliveryOrder{}, err
	}
	if courierUser != nil {
		s.notify(ctx, []uuid.UUID{*courierUser}, "delivery.assigned", "Antar "+out.Number,
			"Antar ke "+out.RecipientName+" dan konfirmasi diterima dengan foto/tanda terima.", "/logistics/shipments?q="+out.Number)
	}
	return out, nil
}

// DeliverInput adalah konfirmasi barang diterima.
type DeliverInput = domain.DeliverInput

// Deliver mengonfirmasi barang diterima (LOG-04). Kurir internal: hanya Merchandiser pengantar dengan
// foto/tanda terima dari HP. Permission `logistics.shipment.post`.
func (s *Service) Deliver(ctx context.Context, id uuid.UUID, in DeliverInput) (domain.DeliveryOrder, error) {
	a, err := actor.Require(ctx, PermShipmentPost)
	if err != nil {
		return domain.DeliveryOrder{}, err
	}
	return s.mutate(ctx, id, nil, audit.ActionStatusChange, func(ctx context.Context, d *domain.DeliveryOrder) (string, error) {
		h, err := s.handler(d.SourceType)
		if err != nil {
			return "", err
		}
		sh, ok, err := s.Store.ShipmentByDeliveryOrder(ctx, d.ID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", apperr.InvalidStateTransition("delivery_order", string(d.Status), domain.ActionDeliver)
		}
		now := s.now()
		if err := sh.Confirm(in, a.UserID, actorName(a), now); err != nil {
			return "", err
		}
		if err := d.Deliver(now); err != nil {
			return "", err
		}
		if len(in.ProofFileIDs) > 0 {
			if s.Files == nil {
				return "", apperr.Validation(map[string]string{"proofFileIds": "Penyimpanan file belum aktif"})
			}
			for _, f := range in.ProofFileIDs {
				if err := s.Files.Attach(ctx, f, FileOwnerDeliveryProof, sh.ID); err != nil {
					return "", err
				}
			}
		}
		if _, err := s.Store.SaveShipment(ctx, sh, a.UserID); err != nil {
			return "", err
		}
		note := "Diterima"
		if sh.ReceivedByName != "" {
			note += " oleh " + sh.ReceivedByName
		}
		if err := s.Store.InsertShipmentEvent(ctx, sh.ID, postgres.ShipmentEvent{
			Status: sh.Status, OccurredAt: now, Source: "manual", Note: note, CreatedByName: actorName(a),
		}, a.UserID); err != nil {
			return "", err
		}
		if h.Delivered != nil {
			if err := h.Delivered(ctx, *d); err != nil {
				return "", err
			}
		}
		return note, nil
	})
}

// Cancel membatalkan DO sebelum dikirim; Outbound terbuka ikut dibatalkan, reservasi tetap milik dokumen asal.
// Permission `logistics.delivery_order.cancel`.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, version int, reason string) (domain.DeliveryOrder, error) {
	if _, err := actor.Require(ctx, PermDOCancel); err != nil {
		return domain.DeliveryOrder{}, err
	}
	return s.mutate(ctx, id, &version, audit.ActionCancel, func(ctx context.Context, d *domain.DeliveryOrder) (string, error) {
		if err := d.Cancel(reason, s.now()); err != nil {
			return "", err
		}
		if d.OutboundID != nil {
			if err := s.Warehouse.CancelOutbound(ctx, warehouse.OutboundSourceDeliveryOrder, d.ID, reason); err != nil {
				return "", err
			}
		}
		if h, err := s.handler(d.SourceType); err == nil && h.Cancelled != nil {
			if err := h.Cancelled(ctx, *d); err != nil {
				return "", err
			}
		}
		return "Dibatalkan: " + strings.TrimSpace(reason), nil
	})
}

// TrackShipment mencatat status pelacakan manual (Pelacakan Pengiriman). Permission `logistics.shipment.write`.
func (s *Service) TrackShipment(ctx context.Context, shipmentID uuid.UUID, status domain.ShipmentStatus, note string) (domain.ShipmentRecord, error) {
	a, err := actor.Require(ctx, PermShipmentWrite)
	if err != nil {
		return domain.ShipmentRecord{}, err
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > domain.MaxNote {
		return domain.ShipmentRecord{}, apperr.Validation(map[string]string{"note": "Maksimal 500 karakter"})
	}
	var out domain.ShipmentRecord
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		sh, err := s.Store.GetShipment(ctx, shipmentID)
		if err != nil {
			return err
		}
		before := sh.Status
		if err := sh.Track(status); err != nil {
			return err
		}
		if out, err = s.Store.SaveShipment(ctx, sh, a.UserID); err != nil {
			return err
		}
		if err := s.Store.InsertShipmentEvent(ctx, sh.ID, postgres.ShipmentEvent{
			Status: status, OccurredAt: s.now(), Source: "manual", Note: note, CreatedByName: actorName(a),
		}, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityDeliveryOrder, EntityID: sh.DeliveryOrderID,
			EntityLabel: "Pengiriman", Before: map[string]any{"shipmentStatus": before},
			After: map[string]any{"shipmentStatus": status}, Note: note,
		})
	})
	return out, err
}

// --- Query ------------------------------------------------------------------------------------

// DeliveryOrderFilter adalah kriteria daftar DO.
type DeliveryOrderFilter = postgres.DeliveryOrderFilter

// ListDeliveryOrders mengembalikan daftar DO.
func (s *Service) ListDeliveryOrders(ctx context.Context, f DeliveryOrderFilter, page paging.Page) (paging.Result[postgres.DeliveryOrderSummary], error) {
	if _, err := actor.Require(ctx, PermDORead); err != nil {
		return paging.Result[postgres.DeliveryOrderSummary]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListDeliveryOrders(ctx, f, page)
}

// ShipmentFilter adalah kriteria daftar pengiriman.
type ShipmentFilter = postgres.ShipmentFilter

// ListShipments mengembalikan daftar pengiriman; mine = hanya kurir internal user ini (Merchandiser di HP).
func (s *Service) ListShipments(ctx context.Context, f ShipmentFilter, mine bool, page paging.Page) (paging.Result[postgres.ShipmentSummary], error) {
	a, err := actor.Require(ctx, PermShipmentRead)
	if err != nil {
		return paging.Result[postgres.ShipmentSummary]{}, err
	}
	if mine {
		f.CourierUserID = &a.UserID
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListShipments(ctx, f, page)
}

// Refs adalah data tampilan produk, batch, lokasi, dan kurir.
type Refs struct {
	Products map[uuid.UUID]ProductRef
	Batches  map[uuid.UUID]BatchRef
	Location warehouse.Location
	Couriers map[uuid.UUID]domain.Courier
	Partner  map[uuid.UUID]PartnerCode
}

// ProductRef adalah produk untuk tampilan dan label.
type ProductRef struct {
	ID      uuid.UUID
	SKU     string
	Name    string
	UOM     string
	Barcode string
}

// BatchRef adalah batch untuk tampilan dan cetak DO.
type BatchRef struct {
	ID         uuid.UUID
	Number     string
	ExpiryDate string
}

// PartnerCode adalah kode artikel & barcode retailer per produk.
type PartnerCode struct {
	ArticleCode string
	Barcode     string
}

// DeliveryDetail adalah detail DO beserta Outbound, pengiriman, dan riwayat pelacakan.
type DeliveryDetail struct {
	DeliveryOrder domain.DeliveryOrder
	Outbound      *warehouse.Outbound
	Shipment      *domain.ShipmentRecord
	Events        []postgres.ShipmentEvent
	Refs          Refs
}

// GetDeliveryOrder mengembalikan detail DO (permission baca DO atau cetak label).
func (s *Service) GetDeliveryOrder(ctx context.Context, id uuid.UUID) (DeliveryDetail, error) {
	if _, err := actor.RequireAny(ctx, PermDORead, PermShipmentRead, PermLabelPrint); err != nil {
		return DeliveryDetail{}, err
	}
	if err := s.ready(); err != nil {
		return DeliveryDetail{}, err
	}
	d, err := s.Store.GetDeliveryOrder(ctx, id, false)
	if err != nil {
		return DeliveryDetail{}, err
	}
	out := DeliveryDetail{DeliveryOrder: d}
	if d.OutboundID != nil {
		ob, err := s.Warehouse.OutboundBySource(ctx, warehouse.OutboundSourceDeliveryOrder, d.ID)
		if err != nil {
			return DeliveryDetail{}, err
		}
		out.Outbound = &ob
	}
	sh, ok, err := s.Store.ShipmentByDeliveryOrder(ctx, d.ID)
	if err != nil {
		return DeliveryDetail{}, err
	}
	if ok {
		out.Shipment = &sh
		if out.Events, err = s.Store.ShipmentEvents(ctx, sh.ID); err != nil {
			return DeliveryDetail{}, err
		}
	}
	out.Refs, err = s.refs(ctx, out)
	return out, err
}

func (s *Service) refs(ctx context.Context, d DeliveryDetail) (Refs, error) {
	var products, batches []uuid.UUID
	for _, l := range d.DeliveryOrder.Lines {
		products = append(products, l.ProductID)
		for _, b := range l.Batches {
			if b.BatchID != nil {
				batches = append(batches, *b.BatchID)
			}
		}
	}
	if d.Outbound != nil {
		for _, l := range d.Outbound.Lines {
			products = append(products, l.ProductID)
			if l.BatchID != nil {
				batches = append(batches, *l.BatchID)
			}
		}
	}
	r := Refs{Products: map[uuid.UUID]ProductRef{}, Batches: map[uuid.UUID]BatchRef{}, Couriers: map[uuid.UUID]domain.Courier{}, Partner: map[uuid.UUID]PartnerCode{}}
	ps, err := s.Products.Products(ctx, uniq(products))
	if err != nil {
		return Refs{}, err
	}
	for id, p := range ps {
		ref := ProductRef{ID: id, SKU: p.SKU, Name: p.DisplayName(), UOM: p.UOM}
		if b, ok := p.PrimaryBarcode(); ok {
			ref.Barcode = b.Barcode
		}
		r.Products[id] = ref
	}
	if ids := uniq(batches); len(ids) > 0 {
		bs, err := s.Batches.Batches(ctx, ids)
		if err != nil {
			return Refs{}, err
		}
		for id, b := range bs {
			ref := BatchRef{ID: id, Number: b.Number}
			if b.ExpiryDate != nil {
				ref.ExpiryDate = b.ExpiryDate.String()
			}
			r.Batches[id] = ref
		}
	}
	if r.Location, err = s.Warehouse.Location(ctx, d.DeliveryOrder.LocationID); err != nil {
		return Refs{}, err
	}
	if d.Shipment != nil {
		c, err := s.Store.GetCourier(ctx, d.Shipment.CourierID)
		if err != nil {
			return Refs{}, err
		}
		r.Couriers[c.ID] = c
	}
	if d.DeliveryOrder.PartnerID != nil && s.Partners != nil {
		for id := range r.Products {
			pc, ok, err := s.Partners.ProductCodeFor(ctx, *d.DeliveryOrder.PartnerID, id)
			if err != nil {
				return Refs{}, err
			}
			if ok {
				r.Partner[id] = PartnerCode{ArticleCode: pc.ArticleCode, Barcode: pc.Barcode}
			}
		}
	}
	return r, nil
}

func uniq(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// DeliveryActions mengembalikan aksi DO untuk actor.
func DeliveryActions(ctx context.Context, d domain.DeliveryOrder, sh *domain.ShipmentRecord) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	for _, act := range d.Actions() {
		switch act {
		case domain.ActionRelease:
			if a.Has(PermDOPost) {
				out = append(out, act)
			}
		case domain.ActionShip:
			if a.Has(PermShipmentWrite) {
				out = append(out, act)
			}
		case domain.ActionDeliver:
			internal := sh != nil && sh.InternalCourierUserID != nil
			if a.Has(PermShipmentPost) && (!internal || *sh.InternalCourierUserID == a.UserID) {
				out = append(out, act)
			}
		case domain.ActionCancel:
			if a.Has(PermDOCancel) {
				out = append(out, act)
			}
		}
	}
	return out
}
