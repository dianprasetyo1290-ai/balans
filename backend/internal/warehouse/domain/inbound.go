package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Jenis dokumen asal Inbound yang dikenal (Desain Data Model 6.1). Modul lain boleh memakai jenis lain
// selama snake_case; labelnya tampil apa adanya.
const (
	SourceGoodsReceipt        = "goods_receipt"
	SourceSalesReturn         = "sales_return"
	SourceConsignmentPullback = "consignment_pullback"
	SourceStockTransfer       = "stock_transfer"
	SourceInternalUsageReturn = "internal_usage_return"
)

var sourceLabels = map[string]string{
	SourceGoodsReceipt: "Penerimaan Barang", SourceSalesReturn: "Retur Penjualan",
	SourceConsignmentPullback: "Tarikan Konsinyasi", SourceStockTransfer: "Transfer Stok",
	SourceInternalUsageReturn: "Sisa Pemakaian Internal",
}

// SourceTypeLabel adalah label jenis dokumen asal Inbound.
func SourceTypeLabel(t string) string {
	if l, ok := sourceLabels[t]; ok {
		return l
	}
	return t
}

// RejectAction adalah perlakuan qty yang ditolak QC.
type RejectAction string

const (
	// RejectHold: qty ditolak tetap Karantina di Inbound; modul asal memutuskan (reject penerimaan PRC-05,
	// retur ke pemasok/maklon).
	RejectHold RejectAction = "hold"
	// RejectToReturns: qty ditolak pindah ke Gudang Retur (tetap Karantina) untuk disposisi (WMS-07).
	RejectToReturns RejectAction = "returns"
)

// DefaultRejectAction: barang dari pemasok/maklon ditahan di Inbound, selain itu ke Gudang Retur.
func DefaultRejectAction(sourceType string) RejectAction {
	if sourceType == SourceGoodsReceipt {
		return RejectHold
	}
	return RejectToReturns
}

// LineStatus adalah status baris Inbound.
type LineStatus string

const (
	LineQuarantine LineStatus = "quarantine"
	LinePosted     LineStatus = "posted"
	LineQCRejected LineStatus = "qc_rejected"
)

// Inbound adalah antrian barang Karantina dari satu dokumen asal (WMS-02, WMS-09).
type Inbound struct {
	ID           uuid.UUID
	Number       string
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	WarehouseID  uuid.UUID
	LocationID   uuid.UUID
	BusinessDate clock.BusinessDate
	RejectAction RejectAction
	Status       Status
	Note         string
	CompletedAt  *time.Time
	CreatedAt    time.Time
	CreatedBy    uuid.UUID
	UpdatedAt    time.Time
	Version      int
	Lines        []InboundLine
}

// InboundLine adalah satu baris barang Karantina.
type InboundLine struct {
	ID                    uuid.UUID
	LineNo                int
	SourceLineID          *uuid.UUID
	ProductID             uuid.UUID
	BatchID               *uuid.UUID
	Qty                   decimal.Decimal
	PassedQty             decimal.Decimal
	RejectedQty           decimal.Decimal
	DestinationLocationID uuid.UUID
	Status                LineStatus
	RejectReason          string
	RejectLocationID      *uuid.UUID
	CheckedBy             *uuid.UUID
	CheckedByName         string
	CheckedAt             *time.Time
}

// InboundLineInput adalah baris dari modul asal.
type InboundLineInput struct {
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	BatchID      *uuid.UUID
	Qty          decimal.Decimal
	// DestinationLocationID opsional; kosong = lokasi standar sesuai jenis produk (diisi service).
	DestinationLocationID *uuid.UUID
}

// InboundInput adalah permintaan pembuatan Inbound.
type InboundInput struct {
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	LocationID   uuid.UUID
	BusinessDate clock.BusinessDate
	RejectAction RejectAction
	Note         string
	Lines        []InboundLineInput
}

// NewInbound memvalidasi Inbound baru berstatus in_progress dengan semua baris Karantina. Lokasi tujuan
// setiap baris harus sudah terisi (service mengisi default sebelum memanggil).
func NewInbound(in InboundInput) (Inbound, error) {
	fields := map[string]string{}
	if !ValidSourceType(in.SourceType) {
		fields["source.type"] = "Jenis dokumen asal tidak valid"
	}
	if in.SourceID == uuid.Nil {
		fields["source.id"] = "Dokumen asal wajib diisi"
	}
	if len([]rune(in.SourceLabel)) > 120 {
		fields["source.label"] = "Maksimal 120 karakter"
	}
	if in.LocationID == uuid.Nil {
		fields["locationId"] = "Lokasi Inbound wajib diisi"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	if in.RejectAction == "" {
		in.RejectAction = DefaultRejectAction(in.SourceType)
	}
	if in.RejectAction != RejectHold && in.RejectAction != RejectToReturns {
		fields["rejectAction"] = "Perlakuan barang ditolak tidak dikenal"
	}
	note := checkNote(in.Note, "note", fields)
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
	case len(in.Lines) > MaxDocumentLines:
		fields["lines"] = "Terlalu banyak baris"
	}
	ib := Inbound{
		SourceType: in.SourceType, SourceID: in.SourceID, SourceLabel: in.SourceLabel, LocationID: in.LocationID,
		BusinessDate: in.BusinessDate, RejectAction: in.RejectAction, Status: StatusInProgress, Note: note,
	}
	for i, l := range in.Lines {
		if l.ProductID == uuid.Nil {
			fields[lineField(i, "productId")] = "Produk wajib diisi"
		}
		if !validQty(l.Qty) {
			fields[lineField(i, "qty")] = "Qty harus lebih dari 0 (maks. 3 desimal)"
		}
		if l.DestinationLocationID == nil || *l.DestinationLocationID == uuid.Nil {
			fields[lineField(i, "destinationLocationId")] = "Lokasi tujuan wajib diisi"
		}
		line := InboundLine{
			LineNo: i + 1, SourceLineID: l.SourceLineID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty,
			Status: LineQuarantine, PassedQty: decimal.Zero, RejectedQty: decimal.Zero,
		}
		if l.DestinationLocationID != nil {
			line.DestinationLocationID = *l.DestinationLocationID
		}
		ib.Lines = append(ib.Lines, line)
	}
	if len(fields) > 0 {
		return Inbound{}, apperr.Validation(fields)
	}
	return ib, nil
}

// Line mengembalikan baris berdasarkan ID.
func (ib *Inbound) Line(id uuid.UUID) (*InboundLine, bool) {
	for i := range ib.Lines {
		if ib.Lines[i].ID == id {
			return &ib.Lines[i], true
		}
	}
	return nil, false
}

// AllChecked memeriksa apakah semua baris sudah QC.
func (ib Inbound) AllChecked() bool {
	for _, l := range ib.Lines {
		if l.Status == LineQuarantine {
			return false
		}
	}
	return true
}

// Checklist adalah butir QC per baris (Desain UI/UX 5: batch, expired, kondisi, segel). true = sesuai.
type Checklist struct {
	Batch     bool `json:"batch"`
	Expiry    bool `json:"expiry"`
	Condition bool `json:"condition"`
	Seal      bool `json:"seal"`
}

// AllPassed memeriksa apakah semua butir sesuai.
func (c Checklist) AllPassed() bool { return c.Batch && c.Expiry && c.Condition && c.Seal }

// QCInput adalah hasil QC satu baris.
type QCInput struct {
	PassedQty   decimal.Decimal
	RejectedQty decimal.Decimal
	Checks      Checklist
	// RejectReason wajib bila ada qty ditolak.
	RejectReason string
	// DestinationLocationID opsional: mengganti lokasi tujuan qty lulus.
	DestinationLocationID *uuid.UUID
	Note                  string
	PhotoFileIDs          []uuid.UUID
}

// MaxQCPhotos membatasi jumlah foto QC per baris.
const MaxQCPhotos = 5

// ApplyQC memvalidasi dan mencatat hasil QC baris: qty lulus + ditolak = qty baris; ditolak wajib alasan; baris
// yang seluruhnya lulus wajib semua butir checklist sesuai. Baris yang sudah QC ditolak INVALID_STATE_TRANSITION.
func (l *InboundLine) ApplyQC(in QCInput, by uuid.UUID, byName string, at time.Time) (QCInput, error) {
	if l.Status != LineQuarantine {
		return in, apperr.InvalidStateTransition("inbound_line", string(l.Status), ActionQC)
	}
	fields := map[string]string{}
	if !validQtyOrZero(in.PassedQty) {
		fields["passedQty"] = "Qty lulus tidak valid"
	}
	if !validQtyOrZero(in.RejectedQty) {
		fields["rejectedQty"] = "Qty ditolak tidak valid"
	}
	if len(fields) == 0 && !in.PassedQty.Add(in.RejectedQty).Equal(l.Qty) {
		fields["passedQty"] = "Qty lulus + ditolak harus sama dengan qty baris (" + l.Qty.String() + ")"
	}
	in.RejectReason = checkNote(in.RejectReason, "rejectReason", fields)
	in.Note = checkNote(in.Note, "note", fields)
	if in.RejectedQty.IsPositive() && in.RejectReason == "" {
		fields["rejectReason"] = "Alasan penolakan wajib diisi"
	}
	if in.RejectedQty.IsZero() && !in.Checks.AllPassed() {
		fields["checks"] = "Semua butir QC harus sesuai untuk meluluskan seluruh qty; tolak qty yang tidak sesuai"
	}
	if len(in.PhotoFileIDs) > MaxQCPhotos {
		fields["photoFileIds"] = "Maksimal 5 foto"
	}
	if in.DestinationLocationID != nil && *in.DestinationLocationID == uuid.Nil {
		in.DestinationLocationID = nil
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	l.PassedQty, l.RejectedQty, l.RejectReason = in.PassedQty, in.RejectedQty, in.RejectReason
	if in.DestinationLocationID != nil {
		l.DestinationLocationID = *in.DestinationLocationID
	}
	l.Status = LinePosted
	if l.PassedQty.IsZero() {
		l.Status = LineQCRejected
	}
	l.CheckedBy, l.CheckedByName, l.CheckedAt = &by, byName, &at
	return in, nil
}

// QualityCheck adalah catatan QC (TG 5.7.2); satu baris QC bisa menghasilkan catatan lulus dan ditolak.
type QualityCheck struct {
	ID             uuid.UUID
	DocumentType   string
	DocumentID     uuid.UUID
	DocumentLineID uuid.UUID
	Result         string
	Qty            decimal.Decimal
	Checks         Checklist
	Note           string
	PhotoFileIDs   []uuid.UUID
	CheckedBy      uuid.UUID
	CheckedByName  string
	CheckedAt      time.Time
}

// Hasil QC.
const (
	QCPassed   = "passed"
	QCRejected = "rejected"
)

// QualityChecks menyusun catatan QC dari hasil satu baris Inbound.
func QualityChecks(ib Inbound, l InboundLine, in QCInput) []QualityCheck {
	base := QualityCheck{
		DocumentType: "inbound", DocumentID: ib.ID, DocumentLineID: l.ID, Checks: in.Checks, Note: in.Note,
		PhotoFileIDs: in.PhotoFileIDs, CheckedByName: l.CheckedByName,
	}
	if l.CheckedBy != nil {
		base.CheckedBy = *l.CheckedBy
	}
	if l.CheckedAt != nil {
		base.CheckedAt = *l.CheckedAt
	}
	var out []QualityCheck
	if l.PassedQty.IsPositive() {
		q := base
		q.Result, q.Qty = QCPassed, l.PassedQty
		out = append(out, q)
	}
	if l.RejectedQty.IsPositive() {
		q := base
		q.Result, q.Qty, q.Note = QCRejected, l.RejectedQty, joinNote(l.RejectReason, in.Note)
		out = append(out, q)
	}
	return out
}

func joinNote(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " — " + b
}

// DefaultDestinationType adalah lokasi tujuan standar barang lulus QC menurut jenis produk catalog:
// Produk Jadi → FG Non-Ecommerce, Packaging → RM, bahan reprocess → GM.
func DefaultDestinationType(productType string) LocationType {
	switch productType {
	case "packaging":
		return LocationRawMaterial
	case "reprocess_material":
		return LocationGoodsMaterial
	}
	return LocationFinishedGoods
}

// Complete menutup Inbound bila semua baris sudah QC.
func (ib *Inbound) Complete(at time.Time) bool {
	if ib.Status != StatusInProgress || !ib.AllChecked() {
		return false
	}
	ib.Status, ib.CompletedAt = StatusCompleted, &at
	return true
}
