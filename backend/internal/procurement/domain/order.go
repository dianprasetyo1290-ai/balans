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

// OrderType adalah jenis PO (Desain Data Model 5).
type OrderType string

const (
	OrderGoods             OrderType = "goods"
	OrderPackaging         OrderType = "packaging"
	OrderTollManufacturing OrderType = "toll_manufacturing"
	OrderReprocess         OrderType = "reprocess"
	OrderOther             OrderType = "other"
)

var orderTypeLabels = map[OrderType]string{
	OrderGoods: "Barang", OrderPackaging: "Packaging", OrderTollManufacturing: "Jasa Maklon",
	OrderReprocess: "Reprocess", OrderOther: "Lainnya",
}

// Valid memeriksa jenis PO.
func (t OrderType) Valid() bool { _, ok := orderTypeLabels[t]; return ok }

// Label mengembalikan label UI jenis PO.
func (t OrderType) Label() string {
	if l, ok := orderTypeLabels[t]; ok {
		return l
	}
	return string(t)
}

// IsToll bernilai true untuk PO ke maklon (jasa maklon dan reprocess): baris = FG hasil + harga jasa, ditambah
// daftar bahan yang dikirim ke lokasi maklon.
func (t OrderType) IsToll() bool { return t == OrderTollManufacturing || t == OrderReprocess }

// lineProductTypes adalah jenis produk baris per jenis PO (kosong = semua jenis non-bundel).
func (t OrderType) lineProductTypes() []string {
	switch t {
	case OrderPackaging:
		return []string{ProductPackaging}
	case OrderTollManufacturing, OrderReprocess:
		return []string{ProductFinishedGood}
	case OrderGoods, OrderOther:
	}
	return nil
}

// materialProductTypes adalah jenis bahan yang dikirim ke maklon: packaging (maklon) atau material GM (reprocess).
func (t OrderType) materialProductTypes() []string {
	if t == OrderReprocess {
		return []string{ProductReprocessMaterial}
	}
	return []string{ProductPackaging}
}

// orderTransitions: Desain Status Dokumen 2.2. Transisi mundur dari received/partially_received/completed
// terjadi saat penerimaan atau tagihan dibatalkan.
var orderTransitions = transitions{
	StatusDraft:             {StatusPendingApproval, StatusCancelled},
	StatusPendingApproval:   {StatusApproved, StatusRejected},
	StatusRejected:          {StatusDraft},
	StatusApproved:          {StatusCancelled, StatusPartiallyReceived, StatusReceived},
	StatusPartiallyReceived: {StatusReceived, StatusClosed, StatusApproved},
	StatusReceived:          {StatusCompleted, StatusPartiallyReceived, StatusApproved},
	StatusCompleted:         {StatusReceived},
}

// OrderLine adalah baris PO: barang/packaging yang dibeli atau FG hasil maklon/reprocess dengan harga jasa.
type OrderLine struct {
	ID          uuid.UUID
	LineNo      int
	ProductID   *uuid.UUID
	Description string
	OrderedQty  decimal.Decimal
	UnitPrice   money.Amount
	Amount      money.Amount
	ReceivedQty decimal.Decimal
	BilledQty   decimal.Decimal
	// PurchaseRequestLineID menautkan baris ke baris PR asal.
	PurchaseRequestLineID *uuid.UUID
	Note                  string
}

// OutstandingQty adalah qty yang belum diterima.
func (l OrderLine) OutstandingQty() decimal.Decimal { return l.OrderedQty.Sub(l.ReceivedQty) }

// FulfilledQty adalah qty yang dihitung memenuhi baris PR: PO batal 0, PO ditutup sebesar qty diterima.
func (l OrderLine) FulfilledQty(status Status) decimal.Decimal {
	switch status {
	case StatusCancelled:
		return decimal.Zero
	case StatusClosed:
		return l.ReceivedQty
	case StatusDraft, StatusPendingApproval, StatusApproved, StatusRejected, StatusPartiallyReceived,
		StatusReceived, StatusCompleted, StatusPosted:
	}
	return l.OrderedQty
}

// OrderMaterial adalah packaging/bahan GM yang direncanakan dipakai maklon.
type OrderMaterial struct {
	ID         uuid.UUID
	LineNo     int
	ProductID  uuid.UUID
	PlannedQty decimal.Decimal
	Note       string
}

// PurchaseOrder adalah PO barang/packaging, jasa maklon, atau reprocess (PRC-01 – PRC-03).
type PurchaseOrder struct {
	ID              uuid.UUID
	Number          string
	Type            OrderType
	SupplierID      uuid.UUID
	SupplierCode    string
	SupplierName    string
	DivisionID      uuid.UUID
	BusinessDate    clock.BusinessDate
	ExpectedDate    clock.BusinessDate
	PaymentTermDays int
	Note            string
	// TollLocationID adalah lokasi eksternal maklon (tujuan bahan, asal FG).
	TollLocationID *uuid.UUID
	// MaterialSourceLocationID adalah lokasi asal bahan: Gudang GM – GM (reprocess) atau Gudang GM – RM (maklon).
	MaterialSourceLocationID *uuid.UUID
	Subtotal                 money.Amount
	Total                    money.Amount
	Status                   Status
	CloseReason              string
	CancelReason             string
	SubmittedAt              *time.Time
	ApprovedAt               *time.Time
	ETANotifiedAt            *time.Time
	Lines                    []OrderLine
	Materials                []OrderMaterial
	CreatedBy                uuid.UUID
	CreatedAt                time.Time
	UpdatedAt                time.Time
	Version                  int
}

// Label mengembalikan nomor PO, atau "Draft" untuk PO yang belum diajukan.
func (o PurchaseOrder) Label() string {
	if o.Number != "" {
		return o.Number
	}
	return "Draft"
}

// SupplierInfo adalah data pemasok yang dibutuhkan PO.
type SupplierInfo struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	IsTollManufacturer bool
	PaymentTermDays    int
}

// OrderLineInput adalah isian satu baris PO.
type OrderLineInput struct {
	ProductID             *uuid.UUID
	Description           string
	Qty                   decimal.Decimal
	UnitPrice             money.Amount
	PurchaseRequestLineID *uuid.UUID
	Note                  string
}

// OrderMaterialInput adalah isian satu bahan yang dikirim ke maklon.
type OrderMaterialInput struct {
	ProductID  uuid.UUID
	PlannedQty decimal.Decimal
	Note       string
}

// OrderInput adalah isian form PO.
type OrderInput struct {
	Type                     OrderType
	SupplierID               uuid.UUID
	DivisionID               uuid.UUID
	BusinessDate             clock.BusinessDate
	ExpectedDate             clock.BusinessDate
	PaymentTermDays          *int
	Note                     string
	MaterialSourceLocationID *uuid.UUID
	Lines                    []OrderLineInput
	Materials                []OrderMaterialInput
}

// OrderRefs adalah data rujukan yang dibaca service untuk validasi PO.
type OrderRefs struct {
	// Supplier nil = pemasok tidak ditemukan.
	Supplier *SupplierInfo
	// TollLocationID adalah lokasi eksternal maklon milik pemasok (nil = belum dibuat).
	TollLocationID *uuid.UUID
	Products       map[uuid.UUID]ProductInfo
}

// MaxPaymentTermDays adalah termin pembayaran maksimum.
const MaxPaymentTermDays = 365

// Validate memeriksa isian PO terhadap pemasok, lokasi maklon, dan produk katalog.
func (in OrderInput) Validate(refs OrderRefs) error {
	fields := map[string]string{}
	if !in.Type.Valid() {
		fields["orderType"] = "Jenis PO tidak dikenal"
	}
	switch {
	case in.SupplierID == uuid.Nil:
		fields["supplierId"] = "Pemasok wajib dipilih"
	case refs.Supplier == nil:
		fields["supplierId"] = "Pemasok tidak ditemukan"
	case in.Type.IsToll() && !refs.Supplier.IsTollManufacturer:
		fields["supplierId"] = "PO " + in.Type.Label() + " harus ke pemasok berjenis maklon"
	case in.Type.IsToll() && refs.TollLocationID == nil:
		fields["supplierId"] = "Maklon belum punya lokasi eksternal maklon. Buat di Data Master › Gudang & Lokasi"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib dipilih"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal PO wajib diisi"
	}
	switch {
	case in.ExpectedDate.IsZero():
		fields["expectedDate"] = "Tanggal kirim diharapkan wajib diisi"
	case !in.BusinessDate.IsZero() && in.ExpectedDate.Before(in.BusinessDate):
		fields["expectedDate"] = "Tanggal kirim diharapkan tidak boleh sebelum tanggal PO"
	}
	if in.PaymentTermDays != nil && (*in.PaymentTermDays < 0 || *in.PaymentTermDays > MaxPaymentTermDays) {
		fields["paymentTermDays"] = "Termin pembayaran 0 sampai 365 hari"
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	if in.Type == OrderReprocess && in.MaterialSourceLocationID == nil {
		fields["materialSourceLocationId"] = "Lokasi asal bahan reprocess (Gudang GM) wajib dipilih"
	}
	if !in.Type.IsToll() && in.MaterialSourceLocationID != nil {
		fields["materialSourceLocationId"] = "Lokasi asal bahan hanya untuk PO maklon atau reprocess"
	}
	in.validateLines(refs.Products, fields)
	in.validateMaterials(refs.Products, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (in OrderInput) validateLines(products map[uuid.UUID]ProductInfo, fields map[string]string) {
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
		return
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Maksimal 200 baris"
	}
	total := money.Zero
	prLines := map[uuid.UUID]bool{}
	for i, l := range in.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		switch {
		case l.ProductID != nil:
			if msg := checkProduct(*l.ProductID, products, in.Type.lineProductTypes()); msg != "" {
				fields[p+"productId"] = msg
			}
		case in.Type != OrderOther:
			fields[p+"productId"] = "Produk wajib dipilih"
		case strings.TrimSpace(l.Description) == "":
			fields[p+"productId"] = "Pilih produk atau isi uraian"
		}
		if msg := validQty(l.Qty); msg != "" {
			fields[p+"qty"] = msg
		}
		if msg := validAmount(l.UnitPrice, false); msg != "" {
			fields[p+"unitPrice"] = msg
		}
		if len([]rune(l.Description)) > 500 || len([]rune(l.Note)) > 500 {
			fields[p+"description"] = "Uraian dan catatan maksimal 500 karakter"
		}
		if l.PurchaseRequestLineID != nil {
			if prLines[*l.PurchaseRequestLineID] {
				fields[p+"purchaseRequestLineId"] = "Baris Purchase Request yang sama dipakai lebih dari sekali"
			}
			prLines[*l.PurchaseRequestLineID] = true
		}
		total = total.Add(LineAmount(l.Qty, l.UnitPrice))
	}
	if _, bad := fields["lines"]; !bad && !total.IsPositive() {
		fields["lines"] = "Total PO harus lebih dari nol"
	}
}

func (in OrderInput) validateMaterials(products map[uuid.UUID]ProductInfo, fields map[string]string) {
	if !in.Type.IsToll() {
		if len(in.Materials) > 0 {
			fields["materials"] = "Daftar bahan hanya untuk PO maklon atau reprocess"
		}
		return
	}
	switch {
	case len(in.Materials) == 0 && in.Type == OrderReprocess:
		fields["materials"] = "Minimal satu bahan GM yang dikirim ke maklon"
	case len(in.Materials) == 0:
		fields["materials"] = "Minimal satu packaging yang akan dipakai maklon"
	case len(in.Materials) > MaxLines:
		fields["materials"] = "Maksimal 200 bahan"
	}
	seen := map[uuid.UUID]bool{}
	for i, m := range in.Materials {
		p := fmt.Sprintf("materials.%d.", i)
		switch {
		case m.ProductID == uuid.Nil:
			fields[p+"productId"] = "Bahan wajib dipilih"
		case seen[m.ProductID]:
			fields[p+"productId"] = "Bahan yang sama sudah ada di daftar"
		default:
			if msg := checkProduct(m.ProductID, products, in.Type.materialProductTypes()); msg != "" {
				fields[p+"productId"] = msg
			}
		}
		seen[m.ProductID] = true
		if msg := validQty(m.PlannedQty); msg != "" {
			fields[p+"plannedQty"] = msg
		}
		if len([]rune(m.Note)) > 500 {
			fields[p+"note"] = "Catatan maksimal 500 karakter"
		}
	}
}

func (o *PurchaseOrder) apply(in OrderInput, refs OrderRefs) {
	s := refs.Supplier
	o.Type, o.SupplierID, o.SupplierCode, o.SupplierName = in.Type, s.ID, s.Code, s.Name
	o.DivisionID, o.BusinessDate, o.ExpectedDate, o.Note = in.DivisionID, in.BusinessDate, in.ExpectedDate, strings.TrimSpace(in.Note)
	o.PaymentTermDays = s.PaymentTermDays
	if in.PaymentTermDays != nil {
		o.PaymentTermDays = *in.PaymentTermDays
	}
	o.TollLocationID, o.MaterialSourceLocationID = nil, nil
	if in.Type.IsToll() {
		o.TollLocationID, o.MaterialSourceLocationID = refs.TollLocationID, in.MaterialSourceLocationID
	}
	o.Lines = make([]OrderLine, len(in.Lines))
	subtotal := money.Zero
	for i, l := range in.Lines {
		o.Lines[i] = OrderLine{
			LineNo: i + 1, ProductID: l.ProductID, Description: strings.TrimSpace(l.Description), OrderedQty: l.Qty,
			UnitPrice: l.UnitPrice, Amount: LineAmount(l.Qty, l.UnitPrice), ReceivedQty: decimal.Zero,
			BilledQty: decimal.Zero, PurchaseRequestLineID: l.PurchaseRequestLineID, Note: strings.TrimSpace(l.Note),
		}
		subtotal = subtotal.Add(o.Lines[i].Amount)
	}
	o.Materials = make([]OrderMaterial, 0, len(in.Materials))
	if in.Type.IsToll() {
		for i, m := range in.Materials {
			o.Materials = append(o.Materials, OrderMaterial{LineNo: i + 1, ProductID: m.ProductID, PlannedQty: m.PlannedQty, Note: strings.TrimSpace(m.Note)})
		}
	}
	// NON-PKP: tanpa PPN (D-04).
	o.Subtotal, o.Total = subtotal, subtotal
}

// NewPurchaseOrder membuat draft PO.
func NewPurchaseOrder(in OrderInput, refs OrderRefs, createdBy uuid.UUID) (PurchaseOrder, error) {
	if err := in.Validate(refs); err != nil {
		return PurchaseOrder{}, err
	}
	o := PurchaseOrder{Status: StatusDraft, CreatedBy: createdBy}
	o.apply(in, refs)
	return o, nil
}

// Update mengganti isi draft PO. Jenis PO tidak bisa diubah.
func (o *PurchaseOrder) Update(in OrderInput, refs OrderRefs) error {
	if o.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya Purchase Order berstatus Draft yang bisa diubah.")
	}
	if in.Type != o.Type {
		return apperr.Validation(map[string]string{"orderType": "Jenis PO tidak bisa diubah"})
	}
	if err := in.Validate(refs); err != nil {
		return err
	}
	o.apply(in, refs)
	return nil
}

func (o *PurchaseOrder) transition(to Status) error {
	if !orderTransitions.allowed(o.Status, to) {
		return invalidTransition("Purchase Order "+o.Label(), o.Status, to)
	}
	o.Status = to
	return nil
}

// Submit mengajukan PO. Nomor yang sudah ada (PO yang direvisi) dipertahankan.
func (o *PurchaseOrder) Submit(number string, at time.Time) error {
	if err := o.transition(StatusPendingApproval); err != nil {
		return err
	}
	if o.Number == "" {
		o.Number = number
	}
	o.SubmittedAt = &at
	return nil
}

// Approve dipanggil handler approval setelah level terakhir menyetujui.
func (o *PurchaseOrder) Approve(at time.Time) error {
	if err := o.transition(StatusApproved); err != nil {
		return err
	}
	o.ApprovedAt = &at
	return nil
}

// Reject dipanggil handler approval saat ditolak.
func (o *PurchaseOrder) Reject() error { return o.transition(StatusRejected) }

// Revise mengembalikan PO yang ditolak ke Draft.
func (o *PurchaseOrder) Revise() error { return o.transition(StatusDraft) }

// HasReceipt memeriksa apakah sudah ada barang yang diterima.
func (o PurchaseOrder) HasReceipt() bool {
	for _, l := range o.Lines {
		if l.ReceivedQty.IsPositive() {
			return true
		}
	}
	return false
}

// Cancel membatalkan PO Draft atau PO Disetujui yang belum ada penerimaan. PO Disetujui wajib alasan.
func (o *PurchaseOrder) Cancel(reason string) error {
	reason = strings.TrimSpace(reason)
	if o.Status == StatusApproved {
		var err error
		if reason, err = requireReason(reason, "reason", "pembatalan"); err != nil {
			return err
		}
		if o.HasReceipt() {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "PO yang sudah ada penerimaan barang tidak bisa dibatalkan; tutup PO.")
		}
	}
	if len([]rune(reason)) > MaxReasonLength {
		return apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	if err := o.transition(StatusCancelled); err != nil {
		return err
	}
	o.CancelReason = reason
	return nil
}

// Close menutup PO yang diterima sebagian; sisa qty tidak ditunggu lagi.
func (o *PurchaseOrder) Close(reason string) error {
	reason, err := requireReason(reason, "reason", "penutupan")
	if err != nil {
		return err
	}
	if err := o.transition(StatusClosed); err != nil {
		return err
	}
	o.CloseReason = reason
	return nil
}

// Line mengembalikan baris berdasarkan ID.
func (o *PurchaseOrder) Line(id uuid.UUID) (*OrderLine, bool) {
	for i := range o.Lines {
		if o.Lines[i].ID == id {
			return &o.Lines[i], true
		}
	}
	return nil, false
}

// QtyDelta adalah perubahan qty per baris PO (positif = bertambah, negatif = pembatalan dokumen).
type QtyDelta struct {
	LineID uuid.UUID
	Qty    decimal.Decimal
}

// applyDeltas menerapkan perubahan qty bila semua baris tetap di antara lower dan upper; tanpa perubahan bila ada
// yang gagal.
func (o *PurchaseOrder) applyDeltas(deltas []QtyDelta, field string, get func(*OrderLine) *decimal.Decimal,
	lower, upper func(OrderLine) decimal.Decimal,
) error {
	fields := map[string]string{}
	next := map[uuid.UUID]decimal.Decimal{}
	for i, d := range deltas {
		l, ok := o.Line(d.LineID)
		if !ok {
			fields[fmt.Sprintf("lines.%d.lineId", i)] = "Baris PO tidak ditemukan"
			continue
		}
		cur, seen := next[d.LineID]
		if !seen {
			cur = *get(l)
		}
		cur = cur.Add(d.Qty)
		switch {
		case !d.Qty.Equal(d.Qty.Round(QtyScale)):
			fields[fmt.Sprintf("lines.%d.qty", i)] = "Qty maksimal 3 angka di belakang koma"
		case cur.IsNegative():
			fields[fmt.Sprintf("lines.%d.qty", i)] = fmt.Sprintf("Qty %s baris %d menjadi negatif", field, l.LineNo)
		case cur.LessThan(lower(*l)):
			fields[fmt.Sprintf("lines.%d.qty", i)] = fmt.Sprintf(
				"Qty %s baris %d kurang dari qty yang sudah ditagih (%s); batalkan tagihan pemasok lebih dulu", field, l.LineNo, lower(*l).String())
		case cur.GreaterThan(upper(*l)):
			fields[fmt.Sprintf("lines.%d.qty", i)] = fmt.Sprintf("Qty %s baris %d melebihi %s", field, l.LineNo, upper(*l).String())
		}
		next[d.LineID] = cur
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	for id, q := range next {
		l, _ := o.Line(id)
		*get(l) = q
	}
	return nil
}

// ApplyReceipt mencatat qty diterima dari Penerimaan Barang (atau pembatalannya) dan menyesuaikan status:
// Diterima bila semua qty terpenuhi, Diterima Sebagian bila ada yang diterima, Disetujui bila kembali nol.
// Toleransi qty 0%: qty diterima tidak boleh melebihi qty PO.
func (o *PurchaseOrder) ApplyReceipt(deltas []QtyDelta) error {
	switch o.Status {
	case StatusApproved, StatusPartiallyReceived, StatusReceived:
	case StatusDraft, StatusPendingApproval, StatusRejected, StatusCompleted, StatusClosed, StatusCancelled, StatusPosted:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; penerimaan hanya untuk PO Disetujui atau Diterima Sebagian.", o.Label(), o.Status.Label()))
	}
	err := o.applyDeltas(deltas, "diterima",
		func(l *OrderLine) *decimal.Decimal { return &l.ReceivedQty },
		func(l OrderLine) decimal.Decimal { return l.BilledQty },
		func(l OrderLine) decimal.Decimal { return l.OrderedQty })
	if err != nil {
		return err
	}
	next := StatusApproved
	switch {
	case o.fullyReceived():
		next = StatusReceived
	case o.HasReceipt():
		next = StatusPartiallyReceived
	}
	if next != o.Status {
		return o.transition(next)
	}
	return nil
}

func (o PurchaseOrder) fullyReceived() bool {
	for _, l := range o.Lines {
		if l.OutstandingQty().IsPositive() {
			return false
		}
	}
	return true
}

func (o PurchaseOrder) fullyBilled() bool {
	for _, l := range o.Lines {
		if l.BilledQty.LessThan(l.OrderedQty) {
			return false
		}
	}
	return true
}

// ApplyBilling mencatat qty yang ditagih pemasok (atau pembatalannya). PO Diterima yang seluruh qty-nya ditagih
// menjadi Selesai. Qty ditagih tidak boleh melebihi qty diterima (3-way match).
func (o *PurchaseOrder) ApplyBilling(deltas []QtyDelta) error {
	switch o.Status {
	case StatusPartiallyReceived, StatusReceived, StatusCompleted, StatusClosed:
	case StatusDraft, StatusPendingApproval, StatusApproved, StatusRejected, StatusCancelled, StatusPosted:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; belum ada penerimaan yang bisa ditagih.", o.Label(), o.Status.Label()))
	}
	err := o.applyDeltas(deltas, "ditagih",
		func(l *OrderLine) *decimal.Decimal { return &l.BilledQty },
		func(OrderLine) decimal.Decimal { return decimal.Zero },
		func(l OrderLine) decimal.Decimal { return l.ReceivedQty })
	if err != nil {
		return err
	}
	switch {
	case o.Status == StatusReceived && o.fullyBilled():
		return o.transition(StatusCompleted)
	case o.Status == StatusCompleted && !o.fullyBilled():
		return o.transition(StatusReceived)
	}
	return nil
}

// AcceptsAdvance memeriksa apakah PO boleh dibayar uang muka: sudah disetujui dan belum selesai/ditutup/batal.
func (o PurchaseOrder) AcceptsAdvance() bool {
	return o.Status == StatusApproved || o.Status == StatusPartiallyReceived || o.Status == StatusReceived
}

// Receivable memeriksa apakah PO bisa diterima barangnya.
func (o PurchaseOrder) Receivable() bool {
	return o.Status == StatusApproved || o.Status == StatusPartiallyReceived
}

// Actions mengembalikan aksi yang dimungkinkan status PO (belum memperhitungkan permission).
func (o PurchaseOrder) Actions() []Action {
	switch o.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise}
	case StatusApproved:
		return []Action{ActionPrint, ActionCreateAdvance, ActionCancel}
	case StatusPartiallyReceived:
		return []Action{ActionPrint, ActionCreateAdvance, ActionClose}
	case StatusReceived:
		return []Action{ActionPrint, ActionCreateAdvance}
	case StatusCompleted, StatusClosed:
		return []Action{ActionPrint}
	case StatusPendingApproval, StatusCancelled, StatusPosted:
	}
	return nil
}
