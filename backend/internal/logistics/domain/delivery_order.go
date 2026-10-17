package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// DeliveryType adalah jenis Delivery Order (Desain Data Model: delivery_orders.delivery_type).
type DeliveryType string

const (
	DeliverySales         DeliveryType = "sales"
	DeliveryMarketplace   DeliveryType = "marketplace"
	DeliveryConsignment   DeliveryType = "consignment"
	DeliveryVendingBuffer DeliveryType = "vending_buffer"
	DeliveryTransfer      DeliveryType = "transfer"
)

// Valid memeriksa jenis DO.
func (t DeliveryType) Valid() bool {
	switch t {
	case DeliverySales, DeliveryMarketplace, DeliveryConsignment, DeliveryVendingBuffer, DeliveryTransfer:
		return true
	}
	return false
}

// Status adalah status Delivery Order (Desain Status Dokumen 5.4).
type Status string

const (
	StatusDraft      Status = "draft"
	StatusInProgress Status = "in_progress"
	StatusInTransit  Status = "in_transit"
	StatusReceived   Status = "received"
	StatusCancelled  Status = "cancelled"
)

// Label adalah label status untuk pesan error.
func (s Status) Label() string {
	switch s {
	case StatusDraft:
		return "Draft"
	case StatusInProgress:
		return "Diproses"
	case StatusInTransit:
		return "Dalam Pengiriman"
	case StatusReceived:
		return "Diterima"
	case StatusCancelled:
		return "Dibatalkan"
	}
	return string(s)
}

// Aksi Delivery Order.
const (
	ActionRelease = "release"
	ActionShip    = "ship"
	ActionDeliver = "deliver"
	ActionCancel  = "cancel"
)

// Batas field.
const (
	MaxLines    = 200
	MaxNote     = 500
	QtyScale    = 3
	MaxProofs   = 5
	MaxTracking = 60
)

var sourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// BatchQty adalah qty satu batch.
type BatchQty struct {
	BatchID *uuid.UUID `json:"batchId,omitempty"`
	Qty     string     `json:"qty"`
}

// Quantity mengembalikan qty sebagai desimal (nol bila tidak valid).
func (b BatchQty) Quantity() decimal.Decimal {
	q, err := decimal.NewFromString(b.Qty)
	if err != nil {
		return decimal.Zero
	}
	return q
}

// Line adalah satu baris DO (satu komponen produk).
type Line struct {
	ID               uuid.UUID
	LineNo           int
	SourceLineID     *uuid.UUID
	SalesOrderLineID *uuid.UUID
	ProductID        uuid.UUID
	Qty              decimal.Decimal
	ShippedQty       decimal.Decimal
	Batches          []BatchQty
}

// DeliveryOrder adalah Delivery Order / surat jalan (LOG-01).
type DeliveryOrder struct {
	ID                  uuid.UUID
	Number              string
	Type                DeliveryType
	SourceType          string
	SourceID            uuid.UUID
	SourceLabel         string
	SalesOrderID        *uuid.UUID
	PartnerID           *uuid.UUID
	CustomerID          *uuid.UUID
	RecipientName       string
	RecipientPhone      string
	ShipToAddress       string
	LocationID          uuid.UUID
	BusinessDate        clock.BusinessDate
	Status              Status
	Note                string
	OutboundID          *uuid.UUID
	OutboundCompletedAt *time.Time
	ReleasedBy          *uuid.UUID
	ReleasedByName      string
	ReleasedAt          *time.Time
	ShippedAt           *time.Time
	DeliveredAt         *time.Time
	CancelledAt         *time.Time
	CancelReason        string
	CreatedAt           time.Time
	CreatedBy           uuid.UUID
	UpdatedAt           time.Time
	Version             int
	Lines               []Line
}

// Label adalah nomor DO atau penanda draft.
func (d DeliveryOrder) Label() string {
	if d.Number != "" {
		return d.Number
	}
	return "Draft DO " + d.SourceLabel
}

// LineInput adalah baris DO baru.
type LineInput struct {
	SourceLineID     *uuid.UUID
	SalesOrderLineID *uuid.UUID
	ProductID        uuid.UUID
	Qty              decimal.Decimal
	// Batches rencana batch (stok sudah reserved oleh dokumen asal). Jumlah qty = Qty.
	Batches []BatchQty
}

// Input adalah isian DO baru.
type Input struct {
	Type           DeliveryType
	SourceType     string
	SourceID       uuid.UUID
	SourceLabel    string
	SalesOrderID   *uuid.UUID
	PartnerID      *uuid.UUID
	CustomerID     *uuid.UUID
	RecipientName  string
	RecipientPhone string
	ShipToAddress  string
	LocationID     uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	Lines          []LineInput
}

func validQty(q decimal.Decimal) bool { return q.IsPositive() && q.Equal(q.Round(QtyScale)) }

func trimMax(s string, n int, field string, fields map[string]string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > n {
		fields[field] = fmt.Sprintf("Maksimal %d karakter", n)
	}
	return s
}

// New memvalidasi Delivery Order baru berstatus Draft.
func New(in Input) (DeliveryOrder, error) {
	fields := map[string]string{}
	if !in.Type.Valid() {
		fields["deliveryType"] = "Jenis DO tidak dikenal"
	}
	if !sourceTypePattern.MatchString(in.SourceType) || len(in.SourceType) > 60 {
		fields["source.type"] = "Jenis dokumen asal tidak valid"
	}
	if in.SourceID == uuid.Nil {
		fields["source.id"] = "Dokumen asal wajib diisi"
	}
	if in.LocationID == uuid.Nil {
		fields["locationId"] = "Lokasi asal wajib diisi"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	d := DeliveryOrder{
		Type: in.Type, SourceType: in.SourceType, SourceID: in.SourceID, SalesOrderID: in.SalesOrderID,
		PartnerID: in.PartnerID, CustomerID: in.CustomerID, LocationID: in.LocationID, BusinessDate: in.BusinessDate,
		Status: StatusDraft,
	}
	d.SourceLabel = trimMax(in.SourceLabel, 120, "source.label", fields)
	d.RecipientName = trimMax(in.RecipientName, 200, "recipientName", fields)
	d.RecipientPhone = trimMax(in.RecipientPhone, 40, "recipientPhone", fields)
	d.ShipToAddress = trimMax(in.ShipToAddress, 1000, "shipToAddress", fields)
	d.Note = trimMax(in.Note, MaxNote, "note", fields)
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Tidak ada barang yang perlu dikirim"
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Terlalu banyak baris"
	}
	for i, l := range in.Lines {
		key := fmt.Sprintf("lines[%d].", i)
		if l.ProductID == uuid.Nil {
			fields[key+"productId"] = "Produk wajib diisi"
		}
		if !validQty(l.Qty) {
			fields[key+"qty"] = "Qty harus lebih dari 0 (maks. 3 desimal)"
		}
		total := decimal.Zero
		for _, b := range l.Batches {
			total = total.Add(b.Quantity())
		}
		if len(l.Batches) > 0 && !total.Equal(l.Qty) {
			fields[key+"batches"] = "Jumlah qty batch harus sama dengan qty baris"
		}
		d.Lines = append(d.Lines, Line{
			LineNo: i + 1, SourceLineID: l.SourceLineID, SalesOrderLineID: l.SalesOrderLineID, ProductID: l.ProductID,
			Qty: l.Qty, ShippedQty: decimal.Zero, Batches: l.Batches,
		})
	}
	if len(fields) > 0 {
		return DeliveryOrder{}, apperr.Validation(fields)
	}
	return d, nil
}

func (d *DeliveryOrder) transition(action string, to Status, from ...Status) error {
	for _, f := range from {
		if d.Status == f {
			d.Status = to
			return nil
		}
	}
	return apperr.InvalidStateTransition("delivery_order", string(d.Status), action)
}

// Release: Logistik Leader meneruskan DO ke gudang (nomor DO, Outbound dibuat).
func (d *DeliveryOrder) Release(number string, by uuid.UUID, byName string, at time.Time) error {
	if err := d.transition(ActionRelease, StatusInProgress, StatusDraft); err != nil {
		return err
	}
	d.Number, d.ReleasedBy, d.ReleasedByName, d.ReleasedAt = number, &by, byName, &at
	return nil
}

// OutboundDone mencatat Outbound selesai (semua barang lulus QC dan siap diserahkan).
func (d *DeliveryOrder) OutboundDone(at time.Time) { d.OutboundCompletedAt = &at }

// ApplyShipment mencatat qty dikirim per baris dokumen asal dan status Dalam Pengiriman.
func (d *DeliveryOrder) ApplyShipment(shipped map[uuid.UUID]decimal.Decimal, at time.Time) error {
	if d.Status != StatusInProgress {
		return apperr.InvalidStateTransition("delivery_order", string(d.Status), ActionShip)
	}
	if d.OutboundCompletedAt == nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Outbound DO ini belum selesai; barang dikirim setelah picking, packing, dan QC selesai.")
	}
	total := decimal.Zero
	for i := range d.Lines {
		l := &d.Lines[i]
		if l.SourceLineID == nil {
			continue
		}
		q := decimal.Min(shipped[*l.SourceLineID], l.Qty)
		shipped[*l.SourceLineID] = shipped[*l.SourceLineID].Sub(q)
		l.ShippedQty = q
		total = total.Add(q)
	}
	if !total.IsPositive() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tidak ada barang lulus QC untuk dikirim.")
	}
	d.Status, d.ShippedAt = StatusInTransit, &at
	return nil
}

// Deliver menandai DO diterima.
func (d *DeliveryOrder) Deliver(at time.Time) error {
	if err := d.transition(ActionDeliver, StatusReceived, StatusInTransit); err != nil {
		return err
	}
	d.DeliveredAt = &at
	return nil
}

// Cancel membatalkan DO sebelum dikirim.
func (d *DeliveryOrder) Cancel(reason string, at time.Time) error {
	fields := map[string]string{}
	reason = trimMax(reason, MaxNote, "reason", fields)
	if reason == "" {
		fields["reason"] = "Alasan pembatalan wajib diisi"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	if err := d.transition(ActionCancel, StatusCancelled, StatusDraft, StatusInProgress); err != nil {
		return err
	}
	d.CancelReason, d.CancelledAt = reason, &at
	return nil
}

// Actions adalah aksi DO sebelum disaring permission.
func (d DeliveryOrder) Actions() []string {
	switch d.Status {
	case StatusDraft:
		return []string{ActionRelease, ActionCancel}
	case StatusInProgress:
		if d.OutboundCompletedAt != nil {
			return []string{ActionShip, ActionCancel}
		}
		return []string{ActionCancel}
	case StatusInTransit:
		return []string{ActionDeliver}
	case StatusReceived, StatusCancelled:
	}
	return []string{}
}

// ShipmentStatus manual yang boleh dicatat sebagai riwayat pelacakan.
func (s ShipmentStatus) Trackable() bool {
	return s == ShipmentStatusPickedUp || s == ShipmentStatusInTransit || s == ShipmentStatusReturned
}

// ShipmentRecord adalah pengiriman satu DO (LOG-02 – LOG-04): kurir, resi, AWB marketplace, kurir internal.
type ShipmentRecord struct {
	ID                    uuid.UUID
	DeliveryOrderID       uuid.UUID
	CourierID             uuid.UUID
	InternalCourierUserID *uuid.UUID
	InternalCourierName   string
	ServiceCode           string
	TrackingNumber        string
	MarketplaceAWB        string
	ShippingCost          money.Amount
	PackageCount          int
	Status                ShipmentStatus
	ShippedAt             time.Time
	DeliveredAt           *time.Time
	ReceivedByName        string
	DeliveryNote          string
	ProofFileIDs          []uuid.UUID
	ConfirmedBy           *uuid.UUID
	ConfirmedByName       string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Version               int
}

// ShipInput adalah isian serah barang ke kurir.
type ShipInput struct {
	CourierID             uuid.UUID
	InternalCourierUserID *uuid.UUID
	ServiceCode           string
	TrackingNumber        string
	MarketplaceAWB        string
	ShippingCost          money.Amount
}

// NewShipment memvalidasi pengiriman: kurir aktif dari master; kurir internal wajib Merchandiser pengantar;
// pesanan marketplace wajib AWB marketplace.
func NewShipment(d DeliveryOrder, c Courier, in ShipInput, courierName string, packages int, at time.Time) (ShipmentRecord, error) {
	fields := map[string]string{}
	if !c.IsActive {
		fields["courierId"] = "Kurir tidak aktif"
	}
	s := ShipmentRecord{
		DeliveryOrderID: d.ID, CourierID: c.ID, ShippingCost: in.ShippingCost, PackageCount: packages,
		Status: ShipmentStatusPickedUp, ShippedAt: at,
	}
	s.ServiceCode = trimMax(in.ServiceCode, 40, "serviceCode", fields)
	s.TrackingNumber = trimMax(in.TrackingNumber, MaxTracking, "trackingNumber", fields)
	s.MarketplaceAWB = trimMax(in.MarketplaceAWB, MaxTracking, "marketplaceAwb", fields)
	if c.IsInternal {
		if in.InternalCourierUserID == nil {
			fields["internalCourierUserId"] = "Kurir internal wajib memilih Merchandiser pengantar"
		}
		s.InternalCourierUserID, s.InternalCourierName = in.InternalCourierUserID, courierName
	}
	if d.Type == DeliveryMarketplace && s.MarketplaceAWB == "" {
		fields["marketplaceAwb"] = "Pesanan marketplace wajib mencatat AWB/barcode marketplace"
	}
	if in.ShippingCost.IsNegative() {
		fields["shippingCost"] = "Ongkos kirim tidak boleh negatif"
	}
	if len(fields) > 0 {
		return ShipmentRecord{}, apperr.Validation(fields)
	}
	return s, nil
}

// DeliverInput adalah konfirmasi barang diterima.
type DeliverInput struct {
	ReceivedByName string
	Note           string
	ProofFileIDs   []uuid.UUID
}

// Confirm mencatat barang diterima. Kurir internal: hanya Merchandiser yang ditugaskan, wajib foto/tanda terima.
func (s *ShipmentRecord) Confirm(in DeliverInput, by uuid.UUID, byName string, at time.Time) error {
	if s.Status == ShipmentStatusDelivered {
		return apperr.InvalidStateTransition("shipment", string(s.Status), ActionDeliver)
	}
	fields := map[string]string{}
	s.ReceivedByName = trimMax(in.ReceivedByName, 200, "receivedByName", fields)
	s.DeliveryNote = trimMax(in.Note, MaxNote, "note", fields)
	if len(in.ProofFileIDs) > MaxProofs {
		fields["proofFileIds"] = "Maksimal 5 foto"
	}
	if s.InternalCourierUserID != nil {
		if *s.InternalCourierUserID != by {
			return apperr.WithMessage(apperr.CodeForbidden,
				"Pengiriman kurir internal dikonfirmasi oleh Merchandiser yang mengantar ("+s.InternalCourierName+").")
		}
		if len(in.ProofFileIDs) == 0 {
			fields["proofFileIds"] = "Unggah foto barang diterima atau tanda terima"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	s.ProofFileIDs = in.ProofFileIDs
	s.Status, s.DeliveredAt, s.ConfirmedBy, s.ConfirmedByName = ShipmentStatusDelivered, &at, &by, byName
	return nil
}

// Track mencatat status pelacakan manual (diserahkan, dalam perjalanan, kembali).
func (s *ShipmentRecord) Track(status ShipmentStatus) error {
	if !status.Trackable() {
		return apperr.Validation(map[string]string{"status": "Status pelacakan tidak valid; status Diterima lewat konfirmasi diterima"})
	}
	if s.Status == ShipmentStatusDelivered {
		return apperr.InvalidStateTransition("shipment", string(s.Status), "track")
	}
	s.Status = status
	return nil
}
