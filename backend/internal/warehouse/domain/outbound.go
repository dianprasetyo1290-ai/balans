package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Jenis dokumen asal Outbound (Desain Data Model 6.1).
const (
	OutboundSourceDeliveryOrder = "delivery_order"
	OutboundSourceStockTransfer = "stock_transfer"
	OutboundSourceInternalUsage = "internal_usage"
	OutboundSourcePurchaseRet   = "purchase_return"
)

var outboundSourceLabels = map[string]string{
	OutboundSourceDeliveryOrder: "Delivery Order", OutboundSourceStockTransfer: "Transfer Stok",
	OutboundSourceInternalUsage: "Pemakaian Internal", OutboundSourcePurchaseRet: "Retur Pembelian",
}

// OutboundSourceLabel adalah label jenis dokumen asal Outbound.
func OutboundSourceLabel(t string) string {
	if l, ok := outboundSourceLabels[t]; ok {
		return l
	}
	return t
}

// Aksi Outbound (allowedActions).
const (
	ActionPick       = "pick"
	ActionPack       = "pack"
	ActionReallocate = "reallocate"
	ActionDrop       = "drop"
)

// OutboundLineStatus adalah status baris Outbound.
type OutboundLineStatus string

const (
	// OutboundPending: menunggu picking.
	OutboundPending OutboundLineStatus = "pending"
	// OutboundPicked: sudah diambil Staf Gudang, menunggu QC.
	OutboundPicked OutboundLineStatus = "picked"
	// OutboundPassed: lulus QC (qty lulus siap diserahkan).
	OutboundPassed OutboundLineStatus = "passed"
	// OutboundRejected: seluruh qty ditolak QC.
	OutboundRejected OutboundLineStatus = "qc_rejected"
	// OutboundHeld: stok pengganti belum cukup; baris ditahan menunggu stok.
	OutboundHeld OutboundLineStatus = "held"
	// OutboundDropped: qty ditahan tidak ikut dikirim atas keputusan user (pengiriman sebagian).
	OutboundDropped OutboundLineStatus = "dropped"
)

// MaxPackages membatasi jumlah koli per Outbound.
const MaxPackages = 500

// OutboundLine adalah satu baris picking (satu batch).
type OutboundLine struct {
	ID                  uuid.UUID
	LineNo              int
	SourceLineID        *uuid.UUID
	ProductID           uuid.UUID
	BatchID             *uuid.UUID
	AllocatedQty        decimal.Decimal
	PickedQty           decimal.Decimal
	PassedQty           decimal.Decimal
	RejectedQty         decimal.Decimal
	Status              OutboundLineStatus
	ReplacementOfLineID *uuid.UUID
	RejectReason        string
	ReturnLabel         string
	CheckedBy           *uuid.UUID
	CheckedByName       string
	CheckedAt           *time.Time
}

// PackageItem adalah isi satu koli.
type PackageItem struct {
	LineID    uuid.UUID  `json:"lineId"`
	ProductID uuid.UUID  `json:"productId"`
	BatchID   *uuid.UUID `json:"batchId,omitempty"`
	Qty       string     `json:"qty"`
}

// OutboundPackage adalah satu koli (label koli n/total, S-316).
type OutboundPackage struct {
	ID          uuid.UUID
	Number      int
	Count       int
	WeightGrams *int
	Contents    []PackageItem
}

// Outbound adalah dokumen picking, packing, dan QC sebelum barang diserahkan ke pengiriman (WMS-12).
type Outbound struct {
	ID               uuid.UUID
	Number           string
	SourceType       string
	SourceID         uuid.UUID
	SourceLabel      string
	WarehouseID      uuid.UUID
	LocationID       uuid.UUID
	BusinessDate     clock.BusinessDate
	RecipientName    string
	RecipientPhone   string
	RecipientAddress string
	Status           Status
	Note             string
	PreparedBy       *uuid.UUID
	PreparedByName   string
	PackedAt         *time.Time
	CheckedBy        *uuid.UUID
	CheckedByName    string
	CompletedAt      *time.Time
	CancelledAt      *time.Time
	CancelReason     string
	CreatedAt        time.Time
	CreatedBy        uuid.UUID
	UpdatedAt        time.Time
	Version          int
	Lines            []OutboundLine
	Packages         []OutboundPackage
}

// OutboundLineInput adalah baris picking dari dokumen asal (stok sudah reserved oleh dokumen asal).
type OutboundLineInput struct {
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	BatchID      *uuid.UUID
	Qty          decimal.Decimal
}

// OutboundInput adalah permintaan pembuatan Outbound.
type OutboundInput struct {
	SourceType       string
	SourceID         uuid.UUID
	SourceLabel      string
	LocationID       uuid.UUID
	BusinessDate     clock.BusinessDate
	RecipientName    string
	RecipientPhone   string
	RecipientAddress string
	Note             string
	Lines            []OutboundLineInput
}

// NewOutbound memvalidasi Outbound baru berstatus in_progress dengan semua baris menunggu picking.
func NewOutbound(in OutboundInput) (Outbound, error) {
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
		fields["locationId"] = "Lokasi asal wajib diisi"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	name := strings.TrimSpace(in.RecipientName)
	phone := strings.TrimSpace(in.RecipientPhone)
	addr := strings.TrimSpace(in.RecipientAddress)
	if len([]rune(name)) > 200 || len([]rune(phone)) > 40 || len([]rune(addr)) > 1000 {
		fields["recipient"] = "Data penerima terlalu panjang"
	}
	note := checkNote(in.Note, "note", fields)
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
	case len(in.Lines) > MaxDocumentLines:
		fields["lines"] = "Terlalu banyak baris"
	}
	o := Outbound{
		SourceType: in.SourceType, SourceID: in.SourceID, SourceLabel: in.SourceLabel, LocationID: in.LocationID,
		BusinessDate: in.BusinessDate, RecipientName: name, RecipientPhone: phone, RecipientAddress: addr,
		Status: StatusInProgress, Note: note,
	}
	for i, l := range in.Lines {
		if l.ProductID == uuid.Nil {
			fields[lineField(i, "productId")] = "Produk wajib diisi"
		}
		if !validQty(l.Qty) {
			fields[lineField(i, "qty")] = "Qty harus lebih dari 0 (maks. 3 desimal)"
		}
		o.addLine(OutboundLine{SourceLineID: l.SourceLineID, ProductID: l.ProductID, BatchID: l.BatchID, AllocatedQty: l.Qty, Status: OutboundPending})
	}
	if len(fields) > 0 {
		return Outbound{}, apperr.Validation(fields)
	}
	return o, nil
}

func (o *Outbound) addLine(l OutboundLine) *OutboundLine {
	l.LineNo = len(o.Lines) + 1
	if l.PickedQty.IsZero() {
		l.PickedQty = decimal.Zero
	}
	l.PassedQty, l.RejectedQty = decimal.Zero, decimal.Zero
	o.Lines = append(o.Lines, l)
	return &o.Lines[len(o.Lines)-1]
}

// Line mengembalikan baris berdasarkan ID.
func (o *Outbound) Line(id uuid.UUID) (*OutboundLine, bool) {
	for i := range o.Lines {
		if o.Lines[i].ID == id {
			return &o.Lines[i], true
		}
	}
	return nil, false
}

func (o Outbound) has(st ...OutboundLineStatus) bool {
	for _, l := range o.Lines {
		for _, s := range st {
			if l.Status == s {
				return true
			}
		}
	}
	return false
}

func (o Outbound) requireOpen(action string) error {
	if o.Status != StatusInProgress {
		return apperr.InvalidStateTransition("outbound", string(o.Status), action)
	}
	return nil
}

// Pick mencatat picking per batch oleh Staf Gudang (prepared_by). lineIDs kosong = semua baris menunggu
// picking. Qty diambil sama dengan qty alokasi; barang kurang/rusak ditolak saat QC.
func (o *Outbound) Pick(lineIDs []uuid.UUID, by uuid.UUID, byName string) error {
	if err := o.requireOpen(ActionPick); err != nil {
		return err
	}
	want := map[uuid.UUID]bool{}
	for _, id := range lineIDs {
		want[id] = true
	}
	n := 0
	for i := range o.Lines {
		l := &o.Lines[i]
		if len(want) > 0 && !want[l.ID] {
			continue
		}
		if l.Status != OutboundPending {
			if len(want) > 0 {
				return apperr.InvalidStateTransition("outbound_line", string(l.Status), ActionPick)
			}
			continue
		}
		l.Status, l.PickedQty = OutboundPicked, l.AllocatedQty
		n++
	}
	if n == 0 {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tidak ada baris yang menunggu picking.")
	}
	o.PreparedBy, o.PreparedByName, o.PackedAt = &by, byName, nil
	return nil
}

// PackageInput adalah isian satu koli saat packing.
type PackageInput struct {
	WeightGrams *int
	Items       []PackageItemInput
}

// PackageItemInput adalah qty satu baris di dalam koli.
type PackageItemInput struct {
	LineID uuid.UUID
	Qty    decimal.Decimal
}

// packQty adalah qty baris yang harus masuk koli: qty diambil (belum QC) atau qty lulus.
func (l OutboundLine) packQty() decimal.Decimal {
	switch l.Status {
	case OutboundPicked:
		return l.PickedQty
	case OutboundPassed:
		return l.PassedQty
	case OutboundPending, OutboundRejected, OutboundHeld, OutboundDropped:
	}
	return decimal.Zero
}

// Pack mencatat koli (S-316) oleh Staf Gudang. Semua baris harus sudah dipicking; setiap baris yang diambil
// atau lulus QC harus masuk koli seluruhnya. Packing ulang (setelah stok pengganti) menggantikan koli lama.
func (o *Outbound) Pack(pkgs []PackageInput, by uuid.UUID, byName string, at time.Time) error {
	if err := o.requireOpen(ActionPack); err != nil {
		return err
	}
	if o.has(OutboundPending) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Selesaikan picking semua baris sebelum packing.")
	}
	fields := map[string]string{}
	switch {
	case len(pkgs) == 0:
		fields["packages"] = "Minimal satu koli"
	case len(pkgs) > MaxPackages:
		fields["packages"] = fmt.Sprintf("Maksimal %d koli", MaxPackages)
	}
	packed := map[uuid.UUID]decimal.Decimal{}
	out := make([]OutboundPackage, len(pkgs))
	for i, p := range pkgs {
		key := fmt.Sprintf("packages[%d]", i)
		if p.WeightGrams != nil && *p.WeightGrams <= 0 {
			fields[key+".weightGrams"] = "Berat harus lebih dari 0 gram"
		}
		if len(p.Items) == 0 {
			fields[key+".items"] = "Isi koli wajib diisi"
		}
		out[i] = OutboundPackage{Number: i + 1, Count: len(pkgs), WeightGrams: p.WeightGrams}
		for j, it := range p.Items {
			l, ok := o.Line(it.LineID)
			if !ok || !l.packQty().IsPositive() {
				fields[fmt.Sprintf("%s.items[%d].lineId", key, j)] = "Baris tidak bisa dimasukkan ke koli"
				continue
			}
			if !validQty(it.Qty) {
				fields[fmt.Sprintf("%s.items[%d].qty", key, j)] = "Qty harus lebih dari 0"
				continue
			}
			packed[l.ID] = packed[l.ID].Add(it.Qty)
			out[i].Contents = append(out[i].Contents, PackageItem{LineID: l.ID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: it.Qty.StringFixed(QtyScale)})
		}
	}
	if len(fields) == 0 {
		for _, l := range o.Lines {
			if q := l.packQty(); !packed[l.ID].Equal(q) {
				fields["packages"] = fmt.Sprintf("Qty baris %d di koli (%s) harus sama dengan qty disiapkan (%s)",
					l.LineNo, packed[l.ID].StringFixed(QtyScale), q.StringFixed(QtyScale))
				break
			}
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	o.Packages = out
	o.PreparedBy, o.PreparedByName, o.PackedAt = &by, byName, &at
	return nil
}

// OutboundQCInput adalah hasil QC satu baris Outbound.
type OutboundQCInput struct {
	PassedQty    decimal.Decimal
	RejectedQty  decimal.Decimal
	Checks       Checklist
	RejectReason string
	Note         string
	PhotoFileIDs []uuid.UUID
}

// RequireChecker menolak QC oleh user yang sama dengan penyiap barang (D-23, D-24) dan QC sebelum packing.
func (o Outbound) RequireChecker(by uuid.UUID) error {
	if err := o.requireOpen(ActionQC); err != nil {
		return err
	}
	if len(o.Packages) == 0 {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Barang belum dipacking; QC dilakukan setelah packing.")
	}
	if o.PreparedBy != nil && *o.PreparedBy == by {
		return apperr.WithMessage(apperr.CodeForbidden,
			"QC Outbound harus dilakukan user yang berbeda dengan penyiap barang ("+o.PreparedByName+").")
	}
	return nil
}

// ApplyQC mencatat QC baris yang sudah dipicking: lulus + ditolak = qty diambil; ditolak wajib alasan.
func (l *OutboundLine) ApplyQC(in OutboundQCInput, by uuid.UUID, byName string, at time.Time) (OutboundQCInput, error) {
	if l.Status != OutboundPicked {
		return in, apperr.InvalidStateTransition("outbound_line", string(l.Status), ActionQC)
	}
	fields := map[string]string{}
	if !validQtyOrZero(in.PassedQty) {
		fields["passedQty"] = "Qty lulus tidak valid"
	}
	if !validQtyOrZero(in.RejectedQty) {
		fields["rejectedQty"] = "Qty ditolak tidak valid"
	}
	if len(fields) == 0 && !in.PassedQty.Add(in.RejectedQty).Equal(l.PickedQty) {
		fields["passedQty"] = "Qty lulus + ditolak harus sama dengan qty diambil (" + l.PickedQty.String() + ")"
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
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	l.PassedQty, l.RejectedQty, l.RejectReason = in.PassedQty, in.RejectedQty, in.RejectReason
	l.Status = OutboundPassed
	if l.PassedQty.IsZero() {
		l.Status = OutboundRejected
	}
	l.CheckedBy, l.CheckedByName, l.CheckedAt = &by, byName, &at
	return in, nil
}

// ReplacementBatch adalah stok pengganti FEFO yang dialokasikan dokumen asal.
type ReplacementBatch struct {
	BatchID *uuid.UUID
	Qty     decimal.Decimal
}

// AddReplacement menambah baris pengganti (menunggu picking ulang) untuk qty yang ditolak QC dan, bila stok
// pengganti kurang, satu baris ditahan sebesar kekurangannya. Packing harus diulang.
func (o *Outbound) AddReplacement(of OutboundLine, batches []ReplacementBatch, shortfall decimal.Decimal) {
	ofID := of.ID
	for _, b := range batches {
		if b.Qty.IsPositive() {
			o.addLine(OutboundLine{
				SourceLineID: of.SourceLineID, ProductID: of.ProductID, BatchID: b.BatchID, AllocatedQty: b.Qty,
				Status: OutboundPending, ReplacementOfLineID: &ofID,
			})
		}
	}
	if shortfall.IsPositive() {
		o.addLine(OutboundLine{
			SourceLineID: of.SourceLineID, ProductID: of.ProductID, AllocatedQty: shortfall, Status: OutboundHeld,
			ReplacementOfLineID: &ofID,
		})
	}
	o.PackedAt = nil
}

// Refill mengisi baris ditahan dengan stok yang baru dialokasikan: batch pertama menjadi isi baris itu
// (menunggu picking), batch berikutnya baris baru, sisa kekurangan tetap ditahan di baris baru.
func (o *Outbound) Refill(lineID uuid.UUID, batches []ReplacementBatch) error {
	l, ok := o.Line(lineID)
	if !ok {
		return apperr.New(apperr.CodeNotFound)
	}
	if l.Status != OutboundHeld {
		return apperr.InvalidStateTransition("outbound_line", string(l.Status), ActionReallocate)
	}
	total := decimal.Zero
	for _, b := range batches {
		total = total.Add(b.Qty)
	}
	if !total.IsPositive() {
		return nil
	}
	held := *l
	remaining := held.AllocatedQty.Sub(total)
	first := true
	for _, b := range batches {
		if !b.Qty.IsPositive() {
			continue
		}
		if first {
			l, _ = o.Line(lineID)
			l.BatchID, l.AllocatedQty, l.Status = b.BatchID, b.Qty, OutboundPending
			first = false
			continue
		}
		o.addLine(OutboundLine{
			SourceLineID: held.SourceLineID, ProductID: held.ProductID, BatchID: b.BatchID, AllocatedQty: b.Qty,
			Status: OutboundPending, ReplacementOfLineID: held.ReplacementOfLineID,
		})
	}
	if remaining.IsPositive() {
		o.addLine(OutboundLine{
			SourceLineID: held.SourceLineID, ProductID: held.ProductID, AllocatedQty: remaining, Status: OutboundHeld,
			ReplacementOfLineID: held.ReplacementOfLineID,
		})
	}
	o.PackedAt = nil
	return nil
}

// Drop: qty baris ditahan tidak ikut dikirim (pengiriman sebagian atas keputusan user, TG 5.7.2).
func (o *Outbound) Drop(lineID uuid.UUID, reason string) error {
	if err := o.requireOpen(ActionDrop); err != nil {
		return err
	}
	l, ok := o.Line(lineID)
	if !ok {
		return apperr.New(apperr.CodeNotFound)
	}
	if l.Status != OutboundHeld {
		return apperr.InvalidStateTransition("outbound_line", string(l.Status), ActionDrop)
	}
	fields := map[string]string{}
	reason = checkNote(reason, "reason", fields)
	if reason == "" {
		fields["reason"] = "Alasan pengiriman sebagian wajib diisi"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	l.Status, l.RejectReason = OutboundDropped, reason
	return nil
}

// TryComplete menutup Outbound bila semua baris sudah QC (tidak ada yang menunggu picking, QC, atau stok),
// minimal satu baris lulus, dan koli sudah dicatat untuk isi terakhir.
func (o *Outbound) TryComplete(by uuid.UUID, byName string, at time.Time) bool {
	if o.Status != StatusInProgress || o.has(OutboundPending, OutboundPicked, OutboundHeld) || !o.has(OutboundPassed) {
		return false
	}
	if o.PackedAt == nil {
		return false
	}
	o.Status, o.CompletedAt, o.CheckedBy, o.CheckedByName = StatusCompleted, &at, &by, byName
	return true
}

// NeedsRepack: semua baris sudah QC tetapi koli harus dicatat ulang (isi berubah karena QC gagal/pengganti).
func (o Outbound) NeedsRepack() bool {
	return o.Status == StatusInProgress && o.PackedAt == nil && !o.has(OutboundPending) && o.has(OutboundPicked, OutboundPassed)
}

// Cancel membatalkan Outbound (DO dibatalkan); reservasi tetap milik dokumen asal.
func (o *Outbound) Cancel(reason string, at time.Time) error {
	if err := o.requireOpen(ActionCancel); err != nil {
		return err
	}
	fields := map[string]string{}
	o.CancelReason = checkNote(reason, "reason", fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	o.Status, o.CancelledAt = StatusCancelled, &at
	return nil
}

// Actions adalah aksi header Outbound sebelum disaring permission.
func (o Outbound) Actions() []string {
	if o.Status != StatusInProgress {
		return []string{}
	}
	out := []string{}
	if o.has(OutboundPending) {
		out = append(out, ActionPick)
	} else if o.has(OutboundPicked, OutboundPassed) {
		out = append(out, ActionPack)
	}
	return out
}

// LineActions adalah aksi baris sebelum disaring permission.
func (o Outbound) LineActions(l OutboundLine) []string {
	if o.Status != StatusInProgress {
		return []string{}
	}
	switch l.Status {
	case OutboundPicked:
		if len(o.Packages) > 0 {
			return []string{ActionQC}
		}
	case OutboundHeld:
		return []string{ActionReallocate, ActionDrop}
	case OutboundPending, OutboundPassed, OutboundRejected, OutboundDropped:
	}
	return []string{}
}

// OutboundQualityChecks menyusun catatan QC dari hasil satu baris Outbound.
func OutboundQualityChecks(o Outbound, l OutboundLine, in OutboundQCInput) []QualityCheck {
	base := QualityCheck{
		DocumentType: "outbound", DocumentID: o.ID, DocumentLineID: l.ID, Checks: in.Checks, Note: in.Note,
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
