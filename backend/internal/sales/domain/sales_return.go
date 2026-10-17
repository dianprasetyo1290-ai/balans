package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Status khusus Retur Penjualan MT/reseller (Desain Status Dokumen 1 dan 5.5).
const (
	StatusSubmitted Status = "submitted"
	StatusVerified  Status = "verified"
)

// Aksi Retur Penjualan dan Nota Kredit (allowedActions).
const (
	ActionSubmit       Action = "submit"
	ActionSendBack     Action = "send_back"
	ActionVerify       Action = "verify"
	ActionAssignPickup Action = "assign_pickup"
	ActionPickup       Action = "record_pickup"
	ActionReceive      Action = "receive"
	ActionApply        Action = "apply"
	ActionPayRefund    Action = "pay_refund"
)

// SettlementMethod adalah cara penyelesaian nilai retur (Desain Data Model sales_returns.settlement_method).
type SettlementMethod string

const (
	// SettlementCreditNote: nota kredit mengurangi piutang invoice asal dan sisanya dipakai invoice berikutnya.
	SettlementCreditNote SettlementMethod = "credit_note"
	// SettlementRefund: nilai retur dibayarkan kembali ke pelanggan/mitra (pembayaran keluar).
	SettlementRefund SettlementMethod = "refund"
	// SettlementMarketplace: retur marketplace mengurangi Piutang Marketplace 121x (dipotong di settlement).
	SettlementMarketplace SettlementMethod = "marketplace"
)

var settlementLabels = map[SettlementMethod]string{
	SettlementCreditNote: "Nota kredit", SettlementRefund: "Refund", SettlementMarketplace: "Potongan settlement marketplace",
}

// Valid memeriksa cara penyelesaian.
func (m SettlementMethod) Valid() bool { _, ok := settlementLabels[m]; return ok }

// Label mengembalikan label UI.
func (m SettlementMethod) Label() string {
	if l, ok := settlementLabels[m]; ok {
		return l
	}
	return string(m)
}

// ReturnLine adalah baris Retur Penjualan: baris Sales Order asal, qty, alasan dari master, nilai jual bersih yang
// dibalik (4103) dan HPP asal (1314) untuk jurnal A-13.
type ReturnLine struct {
	ID          uuid.UUID
	LineNo      int
	OrderLineID uuid.UUID
	ProductID   uuid.UUID
	BatchID     *uuid.UUID
	Qty         decimal.Decimal
	ReasonID    uuid.UUID
	// UnitPrice adalah harga jual bersih per unit (setelah diskon baris & dokumen); Amount = nilai retur baris.
	UnitPrice money.Amount
	Amount    money.Amount
	// UnitCost adalah HPP asal per unit (4 desimal) dari pengiriman; Cost = HPP baris.
	UnitCost decimal.Decimal
	Cost     money.Amount
	Note     string
}

// ReturnItem adalah barang fisik satu baris retur (bundel dipecah ke komponen) yang masuk Inbound sebagai satu
// baris dengan HPP asal; hasil QC Inbound dicatat per item.
type ReturnItem struct {
	ID     uuid.UUID
	LineID uuid.UUID
	// LineNo menautkan item ke baris retur sebelum baris disimpan (ID baris belum ada).
	LineNo       int
	AllocationID uuid.UUID
	ProductID    uuid.UUID
	BatchID      *uuid.UUID
	Qty          decimal.Decimal
	UnitCost     decimal.Decimal
	Cost         money.Amount
	PassedQty    decimal.Decimal
	RejectedQty  decimal.Decimal
	QCDone       bool
}

// SalesReturn adalah Retur Penjualan (RJ, SLS-08).
type SalesReturn struct {
	ID               uuid.UUID
	Number           string
	OrderID          uuid.UUID
	OrderNumber      string
	OrderType        OrderType
	InvoiceID        *uuid.UUID
	InvoiceNumber    string
	ChannelID        uuid.UUID
	SubChannelID     uuid.UUID
	PartyType        string
	PartyID          *uuid.UUID
	PartyName        string
	SourceLocationID uuid.UUID
	ReturnDate       clock.BusinessDate
	Settlement       SettlementMethod
	// RequiresVerification: retur MT/reseller (pihak mitra) diverifikasi Logistik Leader sebelum barang diambil.
	RequiresVerification bool
	Note                 string
	SendBackNote         string
	VerifiedBy           *uuid.UUID
	VerifiedAt           *time.Time
	VerifyNote           string
	PickupRequired       bool
	PickupUserID         *uuid.UUID
	PickupAssignedAt     *time.Time
	PickedUpAt           *time.Time
	PickupNote           string
	PickupFileIDs        []uuid.UUID
	ReceivedBy           *uuid.UUID
	ReceivedAt           *time.Time
	ReceiveNote          string
	DocumentationFileIDs []uuid.UUID
	InboundID            *uuid.UUID
	InboundNumber        string
	Amount               money.Amount
	Cost                 money.Amount
	Status               Status
	CancelReason         string
	PostedAt             *time.Time
	JournalEntryID       *uuid.UUID
	Lines                []ReturnLine
	Items                []ReturnItem
	CreatedBy            uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Version              int
}

// Label mengembalikan nomor RJ atau "Draft Retur Penjualan".
func (r SalesReturn) Label() string {
	if r.Number != "" {
		return r.Number
	}
	return "Draft Retur Penjualan"
}

// ReturnableLine adalah baris Sales Order yang bisa diretur: qty terkirim dikurangi qty retur lain yang belum
// dibatalkan, harga jual bersih per unit, HPP asal per unit, dan batch yang dikirim.
type ReturnableLine struct {
	OrderLineID uuid.UUID
	LineNo      int
	ProductID   uuid.UUID
	IsBundle    bool
	OrderedQty  decimal.Decimal
	ShippedQty  decimal.Decimal
	ReturnedQty decimal.Decimal
	UnitPrice   money.Amount
	UnitCost    decimal.Decimal
	Batches     []uuid.UUID
	// components adalah alokasi baris (komponen bundel) beserta HPP asal per unit dan rasio per unit baris.
	components []returnComponent
	netRatio   decimal.Decimal
	lineTotal  money.Amount
}

type returnComponent struct {
	allocationID uuid.UUID
	productID    uuid.UUID
	perUnit      decimal.Decimal
	unitCost     decimal.Decimal
	batchID      *uuid.UUID
	batches      []uuid.UUID
}

// AvailableQty adalah qty yang masih bisa diretur.
func (l ReturnableLine) AvailableQty() decimal.Decimal {
	q := l.ShippedQty.Sub(l.ReturnedQty)
	if q.IsNegative() {
		return decimal.Zero
	}
	return q
}

// CostScale adalah jumlah desimal HPP per unit (numeric(18,4)).
const CostScale = 4

// ShipmentCost adalah qty dan HPP total yang sudah keluar untuk satu alokasi.
type ShipmentCost struct {
	Qty  decimal.Decimal
	Cost money.Amount
}

// ShipmentCosts menjumlahkan qty dan HPP pengiriman per alokasi.
func ShipmentCosts(shipments []Shipment) map[uuid.UUID]ShipmentCost {
	out := map[uuid.UUID]ShipmentCost{}
	for _, sh := range shipments {
		for _, l := range sh.Lines {
			c, ok := out[l.AllocationID]
			if !ok {
				c = ShipmentCost{Qty: decimal.Zero, Cost: money.Zero}
			}
			c.Qty, c.Cost = c.Qty.Add(l.Qty), c.Cost.Add(l.Cost)
			out[l.AllocationID] = c
		}
	}
	return out
}

// CanReturn menolak pesanan yang belum ada barang terkirim.
func (o Order) CanReturn() error {
	if o.Status != StatusInProgress && o.Status != StatusInTransit && o.Status != StatusCompleted {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Sales Order %s berstatus %s; retur hanya untuk pesanan yang barangnya sudah dikirim.", o.Label(), o.Status.Label()))
	}
	if !o.HasShipment() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Sales Order "+o.Label()+" belum punya barang yang dikirim.")
	}
	return nil
}

// netRatio adalah porsi nilai setelah diskon dokumen (diskon dokumen dibagi rata ke baris sesuai nilai baris).
func (o Order) netRatio() decimal.Decimal {
	sub := o.Subtotal()
	if !sub.IsPositive() {
		return decimal.NewFromInt(1)
	}
	return sub.Sub(o.DocumentDiscountAmount).Decimal().Div(sub.Decimal())
}

// ReturnableLines menghitung baris yang bisa diretur dari pesanan, HPP pengiriman per alokasi, dan qty retur
// lain per baris pesanan.
func ReturnableLines(o Order, costs map[uuid.UUID]ShipmentCost, returned map[uuid.UUID]decimal.Decimal) []ReturnableLine {
	ratio := o.netRatio()
	out := make([]ReturnableLine, 0, len(o.Lines))
	for _, l := range o.Lines {
		rl := ReturnableLine{
			OrderLineID: l.ID, LineNo: l.LineNo, ProductID: l.ProductID, IsBundle: l.IsBundle, OrderedQty: l.Qty,
			ReturnedQty: returned[l.ID], UnitCost: decimal.Zero, netRatio: ratio, lineTotal: l.Total,
		}
		allocQty, shipped := decimal.Zero, decimal.Zero
		for _, a := range o.Allocations {
			if a.LineID != l.ID {
				continue
			}
			allocQty, shipped = allocQty.Add(a.Qty), shipped.Add(a.ShippedQty)
			c := returnComponent{allocationID: a.ID, productID: a.ProductID, unitCost: decimal.Zero}
			if l.Qty.IsPositive() {
				c.perUnit = a.Qty.Div(l.Qty)
			}
			if sc, ok := costs[a.ID]; ok && sc.Qty.IsPositive() {
				c.unitCost = sc.Cost.Decimal().Div(sc.Qty).Round(CostScale)
			}
			for _, b := range a.Batches {
				if b.BatchID != nil {
					c.batches = append(c.batches, *b.BatchID)
					if c.batchID == nil {
						id := *b.BatchID
						c.batchID = &id
					}
				}
			}
			if !a.ShippedQty.IsPositive() {
				continue
			}
			rl.components = append(rl.components, c)
			rl.UnitCost = rl.UnitCost.Add(c.unitCost.Mul(c.perUnit))
			if a.ProductID == l.ProductID {
				for _, b := range c.batches {
					if !slices.Contains(rl.Batches, b) {
						rl.Batches = append(rl.Batches, b)
					}
				}
			}
		}
		if allocQty.IsPositive() {
			rl.ShippedQty = l.Qty.Mul(shipped).Div(allocQty).RoundDown(QtyScale)
		}
		rl.UnitCost = rl.UnitCost.Round(CostScale)
		if l.Qty.IsPositive() {
			rl.UnitPrice = money.FromDecimal(l.Total.Decimal().Mul(ratio).Div(l.Qty)).Round()
		} else {
			rl.UnitPrice = money.Zero
		}
		out = append(out, rl)
	}
	return out
}

// amountFor menghitung nilai jual bersih qty retur (dibulatkan 2 desimal dari nilai baris, bukan harga satuan).
func (l ReturnableLine) amountFor(qty decimal.Decimal) money.Amount {
	if !l.OrderedQty.IsPositive() {
		return money.Zero
	}
	return money.FromDecimal(l.lineTotal.Decimal().Mul(l.netRatio).Mul(qty).Div(l.OrderedQty)).Round()
}

// ReturnLineInput adalah isian baris retur.
type ReturnLineInput struct {
	OrderLineID uuid.UUID
	BatchID     *uuid.UUID
	Qty         decimal.Decimal
	ReasonID    uuid.UUID
	Note        string
}

// ReturnInput adalah isian Retur Penjualan.
type ReturnInput struct {
	OrderID    uuid.UUID
	ReturnDate clock.BusinessDate
	Settlement SettlementMethod
	Note       string
	Lines      []ReturnLineInput
}

// ReturnRefs adalah data rujukan validasi retur yang dibaca service.
type ReturnRefs struct {
	Order      Order
	Invoice    *Invoice
	Returnable []ReturnableLine
	Reasons    map[uuid.UUID]ReturnReason
	Today      clock.BusinessDate
}

// RequiresVerification: retur dari mitra (MT/reseller/B2B partner) melewati verifikasi Logistik Leader;
// retur pelanggan langsung (WA/IG, website, karyawan) dan marketplace langsung masuk Inbound saat diajukan.
func RequiresVerification(o Order) bool { return o.PartnerID != nil }

// NewSalesReturn memvalidasi draft retur terhadap pesanan: barang sudah dikirim, qty ≤ qty terkirim yang belum
// diretur, alasan aktif dengan cakupan retur penjualan, batch sesuai batch yang dikirim, dan cara penyelesaian.
func NewSalesReturn(in ReturnInput, refs ReturnRefs, createdBy uuid.UUID) (SalesReturn, error) {
	r := SalesReturn{Status: StatusDraft, CreatedBy: createdBy}
	if err := r.apply(in, refs); err != nil {
		return SalesReturn{}, err
	}
	return r, nil
}

// Update mengubah draft retur.
func (r *SalesReturn) Update(in ReturnInput, refs ReturnRefs) error {
	if r.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya retur Draft yang bisa diubah.")
	}
	if in.OrderID != r.OrderID {
		return apperr.Validation(map[string]string{"salesOrderId": "Sales Order retur tidak bisa diganti; buat retur baru"})
	}
	return r.apply(in, refs)
}

func (r *SalesReturn) apply(in ReturnInput, refs ReturnRefs) error {
	in.Note = strings.TrimSpace(in.Note)
	o := refs.Order
	f := fieldErrors{}
	if err := o.CanReturn(); err != nil {
		return err
	}
	if o.Type == OrderMarketplace {
		in.Settlement = SettlementMarketplace
	} else if in.Settlement != SettlementCreditNote && in.Settlement != SettlementRefund {
		f.add("settlementMethod", "Pilih nota kredit atau refund")
	}
	partyType, partyID := o.PartyType()
	if o.Type != OrderMarketplace && (partyID == nil || refs.Invoice == nil) {
		f.add("salesOrderId", "Retur membutuhkan invoice pesanan")
	}
	switch {
	case in.ReturnDate.IsZero():
		f.add("returnDate", "Tanggal retur wajib diisi")
	case in.ReturnDate.Before(o.BusinessDate):
		f.add("returnDate", "Tanggal retur tidak boleh sebelum tanggal Sales Order")
	case !refs.Today.IsZero() && in.ReturnDate.After(refs.Today):
		f.add("returnDate", "Tanggal retur tidak boleh di masa depan")
	}
	if tooLong(in.Note, MaxNoteLength) {
		f.add("note", "Catatan maksimal 1000 karakter")
	}
	switch {
	case len(in.Lines) == 0:
		f.add("lines", "Isi minimal satu barang yang diretur")
	case len(in.Lines) > MaxLines:
		f.add("lines", "Maksimal 200 baris")
	}
	byLine := map[uuid.UUID]ReturnableLine{}
	for _, l := range refs.Returnable {
		byLine[l.OrderLineID] = l
	}
	used := map[uuid.UUID]decimal.Decimal{}
	lines := make([]ReturnLine, 0, len(in.Lines))
	amount, cost := money.Zero, money.Zero
	for i, li := range in.Lines {
		key := fmt.Sprintf("lines.%d.", i)
		li.Note = strings.TrimSpace(li.Note)
		rl, ok := byLine[li.OrderLineID]
		if !ok {
			f.add(key+"salesOrderLineId", "Baris tidak ada di Sales Order ini")
			continue
		}
		if msg := ValidQty(li.Qty); msg != "" {
			f.add(key+"qty", msg)
		} else {
			used[rl.OrderLineID] = used[rl.OrderLineID].Add(li.Qty)
			if used[rl.OrderLineID].GreaterThan(rl.AvailableQty()) {
				f.add(key+"qty", "Melebihi qty terkirim yang belum diretur ("+rl.AvailableQty().String()+")")
			}
		}
		reason, ok := refs.Reasons[li.ReasonID]
		switch {
		case li.ReasonID == uuid.Nil || !ok:
			f.add(key+"returnReasonId", "Pilih alasan retur")
		case reason.UsableError(ScopeSalesReturn) != "":
			f.add(key+"returnReasonId", reason.UsableError(ScopeSalesReturn))
		}
		if li.BatchID != nil && !slices.Contains(rl.Batches, *li.BatchID) {
			f.add(key+"batchId", "Batch tidak termasuk batch yang dikirim untuk baris ini")
		}
		if tooLong(li.Note, MaxTextLength) {
			f.add(key+"note", "Catatan maksimal 200 karakter")
		}
		batch := li.BatchID
		if batch == nil && len(rl.Batches) == 1 {
			b := rl.Batches[0]
			batch = &b
		}
		line := ReturnLine{
			LineNo: i + 1, OrderLineID: rl.OrderLineID, ProductID: rl.ProductID, BatchID: batch, Qty: li.Qty,
			ReasonID: li.ReasonID, UnitPrice: rl.UnitPrice, Amount: rl.amountFor(li.Qty), UnitCost: rl.UnitCost,
			Cost: money.FromDecimal(rl.UnitCost.Mul(li.Qty)).Round(), Note: li.Note,
		}
		amount, cost = amount.Add(line.Amount), cost.Add(line.Cost)
		lines = append(lines, line)
	}
	if err := f.err(); err != nil {
		return err
	}
	r.OrderID, r.OrderNumber, r.OrderType = o.ID, o.Number, o.Type
	r.ChannelID, r.SubChannelID, r.SourceLocationID = o.ChannelID, o.SubChannelID, o.SourceLocationID
	r.PartyType, r.PartyID, r.PartyName = partyType, partyID, o.PartyName
	r.InvoiceID, r.InvoiceNumber = nil, ""
	if refs.Invoice != nil {
		id := refs.Invoice.ID
		r.InvoiceID, r.InvoiceNumber = &id, refs.Invoice.Number
	}
	r.ReturnDate, r.Settlement, r.Note = in.ReturnDate, in.Settlement, in.Note
	r.RequiresVerification = RequiresVerification(o)
	r.Lines, r.Amount, r.Cost = lines, amount, cost
	r.Items = planItems(lines, byLine)
	return nil
}

// planItems memecah baris retur ke barang fisik per alokasi (komponen bundel) dengan HPP asal.
func planItems(lines []ReturnLine, byLine map[uuid.UUID]ReturnableLine) []ReturnItem {
	var items []ReturnItem
	for li, l := range lines {
		rl := byLine[l.OrderLineID]
		for _, c := range rl.components {
			qty := l.Qty.Mul(c.perUnit).Round(QtyScale)
			if !qty.IsPositive() {
				continue
			}
			batch := c.batchID
			if c.productID == l.ProductID && l.BatchID != nil {
				batch = l.BatchID
			}
			items = append(items, ReturnItem{
				LineID: lines[li].ID, LineNo: l.LineNo, AllocationID: c.allocationID, ProductID: c.productID, BatchID: batch, Qty: qty,
				UnitCost: c.unitCost, Cost: money.FromDecimal(c.unitCost.Mul(qty)).Round(),
				PassedQty: decimal.Zero, RejectedQty: decimal.Zero,
			})
		}
	}
	return items
}

var returnTransitions = transitions{
	StatusDraft:      {StatusSubmitted, StatusInProgress, StatusCancelled},
	StatusSubmitted:  {StatusDraft, StatusVerified, StatusCancelled},
	StatusVerified:   {StatusInProgress, StatusCancelled},
	StatusInProgress: {StatusPosted},
}

func (r *SalesReturn) transition(to Status) error {
	if !returnTransitions.allowed(r.Status, to) {
		return invalidTransition("Retur Penjualan "+r.Label(), r.Status, to)
	}
	r.Status = to
	return nil
}

// Submit mengajukan retur. Retur MT/reseller → Diajukan (menunggu verifikasi Logistik Leader); lainnya langsung
// Diproses (Inbound dibuat service). Nomor RJ diberikan saat submit.
func (r *SalesReturn) Submit(number string, by uuid.UUID, now time.Time) error {
	to := StatusInProgress
	if r.RequiresVerification {
		to = StatusSubmitted
	}
	if err := r.transition(to); err != nil {
		return err
	}
	if r.Number == "" {
		r.Number = number
	}
	r.SendBackNote = ""
	if to == StatusInProgress {
		r.ReceivedBy, r.ReceivedAt = &by, &now
	}
	return nil
}

// SendBack mengembalikan retur yang tidak sesuai SOP retur ke Draft dengan catatan wajib.
func (r *SalesReturn) SendBack(note string) error {
	n, err := requireNote(note, "Catatan pengembalian")
	if err != nil {
		return err
	}
	if r.Status != StatusSubmitted {
		return invalidTransition("Retur Penjualan "+r.Label(), r.Status, StatusDraft)
	}
	r.Status, r.SendBackNote = StatusDraft, n
	return nil
}

// Verify mencatat crosscheck Logistik Leader dan menentukan barang diambil Merchandiser atau dikirim partner.
func (r *SalesReturn) Verify(by uuid.UUID, pickupRequired bool, note string, now time.Time) error {
	note = strings.TrimSpace(note)
	if tooLong(note, MaxNoteLength) {
		return apperr.Validation(map[string]string{"note": "Catatan maksimal 1000 karakter"})
	}
	if err := r.transition(StatusVerified); err != nil {
		return err
	}
	r.VerifiedBy, r.VerifiedAt, r.VerifyNote, r.PickupRequired = &by, &now, note, pickupRequired
	return nil
}

// AssignPickup menugaskan Merchandiser untuk mengambil barang (retur Terverifikasi yang perlu pickup).
func (r *SalesReturn) AssignPickup(userID uuid.UUID, now time.Time) error {
	switch {
	case r.Status != StatusVerified || !r.PickupRequired:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Penugasan pickup hanya untuk retur Terverifikasi yang perlu diambil Merchandiser.")
	case r.PickedUpAt != nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Barang retur sudah diambil.")
	case userID == uuid.Nil:
		return apperr.Validation(map[string]string{"pickupUserId": "Pilih Merchandiser"})
	}
	r.PickupUserID, r.PickupAssignedAt = &userID, &now
	return nil
}

// RecordPickup mencatat Merchandiser sudah mengecek dan mengambil barang di lokasi partner (foto wajib).
func (r *SalesReturn) RecordPickup(by uuid.UUID, note string, files []uuid.UUID, now time.Time) error {
	note = strings.TrimSpace(note)
	switch {
	case r.Status != StatusVerified || !r.PickupRequired || r.PickupUserID == nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Retur belum ditugaskan untuk pickup.")
	case *r.PickupUserID != by:
		return apperr.New(apperr.CodeForbidden)
	case r.PickedUpAt != nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Pickup sudah dicatat.")
	}
	f := fieldErrors{}
	if len(files) == 0 {
		f.add("photoFileIds", "Unggah minimal satu foto barang di lokasi partner")
	}
	if tooLong(note, MaxNoteLength) {
		f.add("note", "Catatan maksimal 1000 karakter")
	}
	if err := f.err(); err != nil {
		return err
	}
	r.PickedUpAt, r.PickupNote, r.PickupFileIDs = &now, note, uniqueUUIDs(files)
	return nil
}

// ReceivedQty adalah qty hasil cek ulang saat barang tiba (boleh kurang dari qty retur, tidak boleh nol).
type ReceivedQty struct {
	LineID uuid.UUID
	Qty    decimal.Decimal
}

// Receive mencatat barang tiba: cek ulang qty, foto & form retur wajib; retur → Diproses (Inbound dibuat service).
func (r *SalesReturn) Receive(by uuid.UUID, note string, files []uuid.UUID, qtys []ReceivedQty, now time.Time) error {
	note = strings.TrimSpace(note)
	if r.Status != StatusVerified {
		return invalidTransition("Retur Penjualan "+r.Label(), r.Status, StatusInProgress)
	}
	if r.PickupRequired && r.PickedUpAt == nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Barang retur belum diambil Merchandiser.")
	}
	f := fieldErrors{}
	if len(files) == 0 {
		f.add("documentationFileIds", "Unggah foto barang dan form retur")
	}
	if tooLong(note, MaxNoteLength) {
		f.add("note", "Catatan maksimal 1000 karakter")
	}
	for i, q := range qtys {
		key := fmt.Sprintf("lines.%d.qty", i)
		idx := slices.IndexFunc(r.Lines, func(l ReturnLine) bool { return l.ID == q.LineID })
		switch {
		case idx < 0:
			f.add(fmt.Sprintf("lines.%d.lineId", i), "Baris retur tidak ditemukan")
		case ValidQty(q.Qty) != "":
			f.add(key, ValidQty(q.Qty))
		case q.Qty.GreaterThan(r.Lines[idx].Qty):
			f.add(key, "Qty diterima melebihi qty retur "+r.Lines[idx].Qty.String())
		}
	}
	if err := f.err(); err != nil {
		return err
	}
	for _, q := range qtys {
		r.adjustLine(q.LineID, q.Qty)
	}
	if err := r.transition(StatusInProgress); err != nil {
		return err
	}
	r.ReceivedBy, r.ReceivedAt, r.ReceiveNote, r.DocumentationFileIDs = &by, &now, note, uniqueUUIDs(files)
	return nil
}

// adjustLine menurunkan qty baris (cek ulang) beserta nilai, HPP, dan qty item secara proporsional.
func (r *SalesReturn) adjustLine(lineID uuid.UUID, qty decimal.Decimal) {
	r.Amount, r.Cost = money.Zero, money.Zero
	for i := range r.Lines {
		l := &r.Lines[i]
		if l.ID == lineID && !qty.Equal(l.Qty) {
			ratio := qty.Div(l.Qty)
			l.Amount = l.Amount.Mul(ratio).Round()
			l.Cost = money.FromDecimal(l.UnitCost.Mul(qty)).Round()
			l.Qty = qty
			for j := range r.Items {
				it := &r.Items[j]
				if it.LineID == lineID {
					it.Qty = it.Qty.Mul(ratio).Round(QtyScale)
					it.Cost = money.FromDecimal(it.UnitCost.Mul(it.Qty)).Round()
				}
			}
		}
		r.Amount, r.Cost = r.Amount.Add(l.Amount), r.Cost.Add(l.Cost)
	}
	kept := r.Items[:0]
	for _, it := range r.Items {
		if it.Qty.IsPositive() {
			kept = append(kept, it)
		}
	}
	r.Items = kept
}

// Cancel membatalkan retur sebelum barang masuk Inbound.
func (r *SalesReturn) Cancel(reason string) error {
	rs, err := requireReason(reason, "pembatalan")
	if err != nil {
		return err
	}
	if err := r.transition(StatusCancelled); err != nil {
		return err
	}
	r.CancelReason = rs
	return nil
}

// ApplyQC mencatat hasil QC Inbound satu item retur.
func (r *SalesReturn) ApplyQC(itemID uuid.UUID, passed, rejected decimal.Decimal) error {
	if r.Status != StatusInProgress {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Retur Penjualan "+r.Label()+" tidak sedang menunggu QC.")
	}
	for i := range r.Items {
		if r.Items[i].ID == itemID {
			r.Items[i].PassedQty, r.Items[i].RejectedQty, r.Items[i].QCDone = passed, rejected, true
			return nil
		}
	}
	return apperr.New(apperr.CodeNotFound)
}

// QCComplete bernilai true bila semua item retur sudah di-QC.
func (r SalesReturn) QCComplete() bool {
	for _, it := range r.Items {
		if !it.QCDone {
			return false
		}
	}
	return len(r.Items) > 0
}

// Post menandai retur Diposting setelah QC selesai (jurnal A-13 dan nota kredit/refund dibuat service).
func (r *SalesReturn) Post(journalID uuid.UUID, now time.Time) error {
	if !r.QCComplete() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "QC Inbound retur belum selesai.")
	}
	if err := r.transition(StatusPosted); err != nil {
		return err
	}
	r.PostedAt = &now
	if journalID != uuid.Nil {
		r.JournalEntryID = &journalID
	}
	return nil
}

// LineQC menjumlahkan hasil QC item per baris retur. Komponen bundel dikonversi ke qty baris (hasil lulus
// terkecil antarkomponen); done bernilai true bila semua item baris sudah di-QC.
func (r SalesReturn) LineQC(lineID uuid.UUID) (passed, rejected decimal.Decimal, done bool) {
	idx := slices.IndexFunc(r.Lines, func(l ReturnLine) bool { return l.ID == lineID })
	if idx < 0 {
		return decimal.Zero, decimal.Zero, false
	}
	line := r.Lines[idx]
	found, first := false, true
	done = true
	passed = decimal.Zero
	for _, it := range r.Items {
		if it.LineID != lineID || !it.Qty.IsPositive() {
			continue
		}
		found = true
		done = done && it.QCDone
		p := line.Qty.Mul(it.PassedQty).Div(it.Qty).RoundDown(QtyScale)
		if first || p.LessThan(passed) {
			passed, first = p, false
		}
	}
	if !found {
		return decimal.Zero, decimal.Zero, false
	}
	if !done {
		return passed, decimal.Zero, false
	}
	return passed, line.Qty.Sub(passed), true
}

// ReturnContext adalah informasi actor untuk aksi retur (pickup hanya oleh Merchandiser yang ditugaskan).
type ReturnContext struct {
	ActorID uuid.UUID
}

// Actions adalah aksi retur sebelum disaring permission.
func (r SalesReturn) Actions(c ReturnContext) []Action {
	switch r.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusSubmitted:
		return []Action{ActionVerify, ActionSendBack, ActionCancel}
	case StatusVerified:
		acts := []Action{}
		if r.PickupRequired && r.PickedUpAt == nil {
			acts = append(acts, ActionAssignPickup)
			if r.PickupUserID != nil && *r.PickupUserID == c.ActorID {
				acts = append(acts, ActionPickup)
			}
		}
		if !r.PickupRequired || r.PickedUpAt != nil {
			acts = append(acts, ActionReceive)
		}
		return append(acts, ActionCancel)
	case StatusInProgress, StatusPosted, StatusCancelled:
	}
	return []Action{}
}

func requireNote(note, what string) (string, error) {
	note = strings.TrimSpace(note)
	switch {
	case note == "":
		return "", apperr.Validation(map[string]string{"note": what + " wajib diisi"})
	case tooLong(note, MaxNoteLength):
		return "", apperr.Validation(map[string]string{"note": "Catatan maksimal 1000 karakter"})
	}
	return note, nil
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
