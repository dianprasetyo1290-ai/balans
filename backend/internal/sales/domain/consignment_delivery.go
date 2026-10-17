package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// DeliveryType adalah jenis pengiriman titipan ke mitra (Desain Data Model 9 `delivery_orders.delivery_type`).
type DeliveryType string

const (
	// DeliveryConsignment: kirim ke lokasi konsinyasi mitra (A-07).
	DeliveryConsignment DeliveryType = "consignment"
	// DeliveryVendingBuffer: kirim ke buffer vending di lokasi mitra (A-24).
	DeliveryVendingBuffer DeliveryType = "vending_buffer"
)

// Valid memeriksa jenis pengiriman.
func (t DeliveryType) Valid() bool { return t == DeliveryConsignment || t == DeliveryVendingBuffer }

// Label adalah label UI.
func (t DeliveryType) Label() string {
	if t == DeliveryVendingBuffer {
		return "Buffer Vending"
	}
	return "Konsinyasi"
}

// LocationType adalah jenis lokasi tujuan (kode warehouse.LocationType).
func (t DeliveryType) LocationType() string { return string(t) }

// ChannelCode adalah kode channel dimensi jurnal (Desain Finance & Accounting 3.1).
func (t DeliveryType) ChannelCode() string {
	if t == DeliveryVendingBuffer {
		return "vending"
	}
	return "consignment"
}

// RuleCode adalah posting rule pengiriman: A-07 konsinyasi, A-24 buffer vending.
func (t DeliveryType) RuleCode() string {
	if t == DeliveryVendingBuffer {
		return "A-24"
	}
	return "A-07"
}

// Aksi dokumen konsinyasi (ActionSubmit dan ActionReceive dipakai bersama retur penjualan).
const (
	ActionStart  Action = "start"
	ActionUpdate Action = "update_lines"
	ActionRevise Action = "revise"
)

// DeliveryLine adalah baris pengiriman. Setelah dikirim satu baris = satu batch FEFO dengan nilai FIFO-nya.
type DeliveryLine struct {
	ID        uuid.UUID
	LineNo    int
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
	Cost      money.Amount
	Note      string
}

// ConsignmentDelivery adalah pengiriman barang titipan ke lokasi konsinyasi atau buffer vending mitra. Stok tetap
// milik Balans sampai terjual (PRD 7 aturan 13).
type ConsignmentDelivery struct {
	ID              uuid.UUID
	Number          string
	Type            DeliveryType
	PartnerID       uuid.UUID
	PartnerCode     string
	PartnerName     string
	AgreementID     *uuid.UUID
	AgreementNumber string
	FromLocationID  uuid.UUID
	ToLocationID    uuid.UUID
	ChannelID       uuid.UUID
	SubChannelID    uuid.UUID
	BusinessDate    clock.BusinessDate
	Status          Status
	SourceType      string
	SourceID        *uuid.UUID
	TransferID      *uuid.UUID
	TransferNumber  string
	Cost            money.Amount
	Note            string
	CancelReason    string
	ShippedAt       *time.Time
	ReceivedAt      *time.Time
	CreatedAt       time.Time
	CreatedBy       uuid.UUID
	UpdatedAt       time.Time
	Version         int
	Lines           []DeliveryLine
}

// DeliveryLineInput adalah isian baris pengiriman.
type DeliveryLineInput struct {
	ProductID uuid.UUID
	Qty       decimal.Decimal
	Note      string
}

// DeliveryInput adalah isian pengiriman konsinyasi/buffer vending.
type DeliveryInput struct {
	Type           DeliveryType
	PartnerID      uuid.UUID
	FromLocationID uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	Lines          []DeliveryLineInput
}

// Dokumen pengiriman (source_type) pengiriman titipan. SourceConsignmentDelivery: draft / data lama yang dikirim
// langsung dari halaman Konsinyasi (sebelum lewat DO); SourceDeliveryOrder: diproses lewat Delivery Order logistik
// (source_id = ID DO).
const (
	SourceConsignmentDelivery = "consignment_delivery"
	SourceDeliveryOrder       = "delivery_order"
)

func validateDelivery(in DeliveryInput) ([]DeliveryLine, error) {
	f := map[string]string{}
	if !in.Type.Valid() {
		f["deliveryType"] = "Pilih jenis pengiriman"
	}
	if in.PartnerID == uuid.Nil {
		f["partnerId"] = "Pilih mitra"
	}
	if in.FromLocationID == uuid.Nil {
		f["fromLocationId"] = "Pilih lokasi asal"
	}
	if in.BusinessDate.IsZero() {
		f["businessDate"] = "Tanggal kirim wajib diisi"
	}
	if tooLong(in.Note, MaxNoteLength) {
		f["note"] = "Catatan maksimal 1000 karakter"
	}
	switch {
	case len(in.Lines) == 0:
		f["lines"] = "Tambahkan minimal satu produk"
	case len(in.Lines) > MaxLines:
		f["lines"] = fmt.Sprintf("Maksimal %d baris", MaxLines)
	}
	seen := map[uuid.UUID]bool{}
	lines := make([]DeliveryLine, 0, len(in.Lines))
	for i, l := range in.Lines {
		switch {
		case l.ProductID == uuid.Nil:
			f[fmt.Sprintf("lines.%d.productId", i)] = "Pilih produk"
		case seen[l.ProductID]:
			f[fmt.Sprintf("lines.%d.productId", i)] = "Produk sudah ada di baris lain"
		}
		seen[l.ProductID] = true
		if msg := ValidQty(l.Qty); msg != "" {
			f[fmt.Sprintf("lines.%d.qty", i)] = msg
		}
		lines = append(lines, DeliveryLine{LineNo: i + 1, ProductID: l.ProductID, Qty: l.Qty, Cost: money.Zero, Note: strings.TrimSpace(l.Note)})
	}
	if len(f) > 0 {
		return nil, apperr.Validation(f)
	}
	return lines, nil
}

// NewConsignmentDelivery memvalidasi pengiriman baru berstatus Draft.
func NewConsignmentDelivery(in DeliveryInput) (ConsignmentDelivery, error) {
	lines, err := validateDelivery(in)
	if err != nil {
		return ConsignmentDelivery{}, err
	}
	return ConsignmentDelivery{
		Type: in.Type, PartnerID: in.PartnerID, FromLocationID: in.FromLocationID, BusinessDate: in.BusinessDate,
		Status: StatusDraft, SourceType: SourceConsignmentDelivery, Cost: money.Zero, Note: strings.TrimSpace(in.Note),
		Lines: lines,
	}, nil
}

// Update mengganti isian pengiriman Draft (jenis dan mitra tidak berubah).
func (d *ConsignmentDelivery) Update(in DeliveryInput) error {
	if d.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			fmt.Sprintf("Pengiriman %s berstatus %s; hanya Draft yang bisa diubah.", d.Label(), d.Status.Label()))
	}
	in.Type, in.PartnerID = d.Type, d.PartnerID
	lines, err := validateDelivery(in)
	if err != nil {
		return err
	}
	d.FromLocationID, d.BusinessDate, d.Note, d.Lines = in.FromLocationID, in.BusinessDate, strings.TrimSpace(in.Note), lines
	return nil
}

// deliveryTransitions: Draft → Diproses (diajukan: stok direservasi + DO) → Dalam Pengiriman (DO dikirim) →
// Selesai (DO diterima). DO dibatalkan sebelum dikirim → kembali Draft.
var deliveryTransitions = transitions{
	StatusDraft:      {StatusInProgress, StatusCancelled},
	StatusInProgress: {StatusInTransit, StatusDraft},
	StatusInTransit:  {StatusCompleted},
}

func (d *ConsignmentDelivery) transition(to Status) error {
	if !deliveryTransitions.allowed(d.Status, to) {
		return invalidTransition("Pengiriman "+d.Label(), d.Status, to)
	}
	d.Status = to
	return nil
}

// Label adalah nomor pengiriman atau "Draft".
func (d ConsignmentDelivery) Label() string {
	if d.Number == "" {
		return "Draft"
	}
	return d.Number
}

// ViaDeliveryOrder: pengiriman diproses lewat Delivery Order logistik (bukan data lama kirim langsung).
func (d ConsignmentDelivery) ViaDeliveryOrder() bool {
	return d.SourceType == SourceDeliveryOrder && d.SourceID != nil
}

// Submit mencatat pengiriman diajukan ke Logistik: stok sudah direservasi dan DO Draft dibuat (Diproses).
func (d *ConsignmentDelivery) Submit(deliveryOrderID uuid.UUID) error {
	if deliveryOrderID == uuid.Nil {
		return apperr.Validation(map[string]string{"deliveryOrderId": "Delivery Order wajib ada"})
	}
	if err := d.transition(StatusInProgress); err != nil {
		return err
	}
	d.SourceType, d.SourceID = SourceDeliveryOrder, &deliveryOrderID
	return nil
}

// Reopen mengembalikan pengiriman Diproses ke Draft karena DO-nya dibatalkan sebelum dikirim.
func (d *ConsignmentDelivery) Reopen() error {
	if err := d.transition(StatusDraft); err != nil {
		return err
	}
	d.SourceType, d.SourceID = SourceConsignmentDelivery, nil
	return nil
}

// MarkShipped mencatat barang keluar lewat DO (TRF Dalam Pengiriman) beserta baris per batch dan nilai FIFO-nya.
func (d *ConsignmentDelivery) MarkShipped(number string, transferID uuid.UUID, transferNumber string, lines []DeliveryLine, now time.Time) error {
	if err := d.transition(StatusInTransit); err != nil {
		return err
	}
	d.Number, d.TransferID, d.TransferNumber, d.ShippedAt = number, &transferID, transferNumber, &now
	d.Lines = lines
	d.Cost = money.Zero
	for i := range d.Lines {
		d.Lines[i].LineNo = i + 1
		d.Cost = d.Cost.Add(d.Lines[i].Cost)
	}
	return nil
}

// MarkReceived mencatat barang diterima mitra (Selesai).
func (d *ConsignmentDelivery) MarkReceived(now time.Time) error {
	if err := d.transition(StatusCompleted); err != nil {
		return err
	}
	d.ReceivedAt = &now
	return nil
}

// Cancel membatalkan pengiriman Draft.
func (d *ConsignmentDelivery) Cancel(reason string) error {
	r, err := requireReason(reason, "pembatalan")
	if err != nil {
		return err
	}
	if err := d.transition(StatusCancelled); err != nil {
		return err
	}
	d.CancelReason = r
	return nil
}

// Actions adalah aksi pengiriman sebelum disaring permission.
func (d ConsignmentDelivery) Actions() []Action {
	switch d.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusInTransit:
		// Diterima lewat DO (Logistik); konfirmasi dari halaman Konsinyasi hanya untuk data lama tanpa DO.
		if d.ViaDeliveryOrder() {
			return []Action{ActionPrint}
		}
		return []Action{ActionReceive, ActionPrint}
	case StatusCompleted:
		return []Action{ActionPrint}
	}
	return nil
}
