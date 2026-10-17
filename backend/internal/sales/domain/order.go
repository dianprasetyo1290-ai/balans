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

// OrderType adalah jenis pesanan; satu model Sales Order untuk semua channel (Desain Data Model 8).
type OrderType string

const (
	OrderModernTrade    OrderType = "modern_trade"
	OrderB2B            OrderType = "b2b"
	OrderSocialCommerce OrderType = "social_commerce"
	OrderMarketplace    OrderType = "marketplace"
	OrderEmployee       OrderType = "employee"
)

var orderTypeLabels = map[OrderType]string{
	OrderModernTrade: "Modern Trade", OrderB2B: "B2B / Corporate", OrderSocialCommerce: "WhatsApp/Instagram",
	OrderMarketplace: "Marketplace", OrderEmployee: "Penjualan Karyawan",
}

// OrderTypes adalah semua jenis pesanan urut tampilan.
var OrderTypes = []OrderType{OrderModernTrade, OrderB2B, OrderSocialCommerce, OrderMarketplace, OrderEmployee}

// Valid memeriksa jenis pesanan.
func (t OrderType) Valid() bool { _, ok := orderTypeLabels[t]; return ok }

// Label mengembalikan label UI jenis pesanan.
func (t OrderType) Label() string {
	if l, ok := orderTypeLabels[t]; ok {
		return l
	}
	return string(t)
}

// ChannelCode adalah kode channel dimensi jurnal yang wajib dipakai jenis pesanan (Desain Finance & Accounting 3.1).
func (t OrderType) ChannelCode() string { return string(t) }

// LocationTypes adalah jenis lokasi asal stok yang boleh dialokasikan: MT/B2B dari FG Non-Ecommerce, marketplace
// dari E-commerce Stock, WA/IG dan karyawan dari FG Non-Ecommerce, Petty Stock, atau E-commerce Stock.
func (t OrderType) LocationTypes() []string {
	switch t {
	case OrderModernTrade, OrderB2B:
		return []string{LocationFinishedGoods}
	case OrderMarketplace:
		return []string{LocationEcommerce}
	case OrderSocialCommerce, OrderEmployee:
	}
	return []string{LocationFinishedGoods, LocationPettyStock, LocationEcommerce}
}

// RecognizedAtHandover: penjualan diakui saat serah ke kurir (marketplace, U-25, A-10); channel lain diakui saat
// invoice (A-05) dan HPP saat barang dikirim (A-06).
func (t OrderType) RecognizedAtHandover() bool { return t == OrderMarketplace }

// AllowsCredit: hanya MT beli putus dan B2B yang boleh tempo (invoice terbit saat konfirmasi).
func (t OrderType) AllowsCredit() bool { return t == OrderModernTrade || t == OrderB2B }

// Jenis lokasi (kode warehouse.LocationType) yang dipakai alokasi penjualan.
const (
	LocationFinishedGoods = "finished_goods"
	LocationEcommerce     = "ecommerce"
	LocationPettyStock    = "petty_stock"
)

// PaymentMode adalah cara bayar pesanan: `prepaid` = Proforma Invoice lalu invoice lunas saat pembayaran
// dikonfirmasi (BRD 5.4, K-10); `credit` = tempo, invoice terbit saat konfirmasi dan menjadi piutang.
type PaymentMode string

const (
	PaymentPrepaid PaymentMode = "prepaid"
	PaymentCredit  PaymentMode = "credit"
)

// Valid memeriksa cara bayar.
func (m PaymentMode) Valid() bool { return m == PaymentPrepaid || m == PaymentCredit }

// Jenis pihak pesanan/invoice.
const (
	PartyCustomer = "customer"
	PartyPartner  = "partner"
)

// Tipe pelanggan dan mitra (kode modul party) yang menentukan jenis pesanan yang boleh dipakai.
const (
	CustomerTypeB2B        = "b2b"
	CustomerTypeEmployee   = "employee"
	PartnerTypeModernTrade = "modern_trade"
	PartnerTypeConsignment = "consignment"
	PartnerTypeB2B         = "b2b"
)

// DiscountKind adalah bentuk diskon manual (MD-05).
type DiscountKind string

const (
	DiscountPercent DiscountKind = "percent"
	DiscountAmount  DiscountKind = "amount"
)

var hundred = decimal.NewFromInt(100)

// Discount adalah diskon manual persen atau nominal. Nilai nol = tanpa diskon.
type Discount struct {
	Kind  DiscountKind
	Value decimal.Decimal
}

// Apply menghitung nominal diskon dari dasar (bruto baris atau subtotal dokumen), dibulatkan 2 desimal.
func (d Discount) Apply(base money.Amount) (money.Amount, string) {
	if d.Value.IsZero() {
		return money.Zero, ""
	}
	if d.Value.IsNegative() {
		return money.Zero, "Diskon tidak boleh negatif"
	}
	switch d.Kind {
	case DiscountPercent:
		if d.Value.GreaterThan(hundred) {
			return money.Zero, "Diskon paling besar 100%"
		}
		if !d.Value.Equal(d.Value.Round(4)) {
			return money.Zero, "Diskon persen maksimal 4 angka di belakang koma"
		}
		return base.Percent(d.Value).Round(), ""
	case DiscountAmount:
		amount := money.FromDecimal(d.Value)
		if !amount.Equal(amount.Round()) {
			return money.Zero, "Nominal diskon maksimal 2 angka di belakang koma"
		}
		if amount.Cmp(base) > 0 {
			return money.Zero, "Diskon melebihi nilai sebelum diskon"
		}
		return amount, ""
	}
	return money.Zero, "Jenis diskon tidak dikenal"
}

// IsZero bernilai true bila tanpa diskon.
func (d Discount) IsZero() bool { return d.Value.IsZero() }

func (d Discount) normalized() Discount {
	if d.Kind == "" {
		d.Kind = DiscountAmount
	}
	return d
}

// PriceRuleMarketplaceInput: harga baris marketplace diambil dari order marketplace (input/impor), bukan resolver.
const PriceRuleMarketplaceInput = "marketplace_input"

// LinePrice adalah harga satuan baris hasil resolver harga (atau harga order marketplace).
type LinePrice struct {
	UnitPrice          money.Amount
	Rule               string
	SourceID           *uuid.UUID
	PriceListID        *uuid.UUID
	PriceListVersionID *uuid.UUID
	Description        string
}

// Line adalah baris pesanan: produk (boleh bundel), qty, harga otomatis beserta aturan & versinya, diskon manual.
type Line struct {
	ID        uuid.UUID
	LineNo    int
	ProductID uuid.UUID
	IsBundle  bool
	Qty       decimal.Decimal
	Price     LinePrice
	Discount  Discount
	// Gross = qty × harga; DiscountAmount = diskon baris; Total = Gross − DiscountAmount.
	Gross          money.Amount
	DiscountAmount money.Amount
	Total          money.Amount
	Note           string
}

// AllocationBatch adalah batch hasil alokasi FEFO satu alokasi.
type AllocationBatch struct {
	BatchID *uuid.UUID
	Qty     decimal.Decimal
}

// Allocation adalah stok yang dialokasikan (reserved) untuk satu komponen baris; bundel dipecah ke komponen.
type Allocation struct {
	ID         uuid.UUID
	LineID     uuid.UUID
	ProductID  uuid.UUID
	LocationID uuid.UUID
	Qty        decimal.Decimal
	ShippedQty decimal.Decimal
	Released   bool
	Batches    []AllocationBatch
}

// OutstandingQty adalah qty alokasi yang belum dikirim.
func (a Allocation) OutstandingQty() decimal.Decimal { return a.Qty.Sub(a.ShippedQty) }

// Order adalah Sales Order (SLS-01, SLS-04, SLS-05, SLS-07).
type Order struct {
	ID               uuid.UUID
	Number           string
	Type             OrderType
	ChannelID        uuid.UUID
	SubChannelID     uuid.UUID
	CustomerID       *uuid.UUID
	PartnerID        *uuid.UUID
	PartyCode        string
	PartyName        string
	PartyPhone       string
	ExternalOrderID  string
	ClientPONumber   string
	BusinessDate     clock.BusinessDate
	SourceLocationID uuid.UUID
	PaymentMode      PaymentMode
	PaymentTermDays  int
	ShipToName       string
	ShipToPhone      string
	ShipToAddress    string
	Lines            []Line
	DocumentDiscount Discount
	// Gross = Σ bruto baris; LineDiscount = Σ diskon baris; DocumentDiscountAmount dari subtotal setelah diskon
	// baris; Total = Gross − LineDiscount − DocumentDiscountAmount + Shipping. NON-PKP: tanpa PPN.
	Gross                    money.Amount
	LineDiscount             money.Amount
	DocumentDiscountAmount   money.Amount
	Shipping                 money.Amount
	Total                    money.Amount
	DiscountApprovalRequired bool
	DiscountApprovedAt       *time.Time
	Status                   Status
	Note                     string
	CancelReason             string
	ConfirmedAt              *time.Time
	ShippedAt                *time.Time
	DeliveredAt              *time.Time
	PaymentProofFileID       *uuid.UUID
	PaymentProofUploadedAt   *time.Time
	ImportRowKey             string
	// AllocationRound bertambah setiap stok dialokasikan ulang (aksi reservasi inventory unik per putaran).
	AllocationRound int
	Allocations     []Allocation
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// TotalDiscount adalah diskon baris + diskon dokumen.
func (o Order) TotalDiscount() money.Amount { return o.LineDiscount.Add(o.DocumentDiscountAmount) }

// Subtotal adalah total baris setelah diskon baris.
func (o Order) Subtotal() money.Amount { return o.Gross.Sub(o.LineDiscount) }

// PartyType mengembalikan jenis pihak pesanan ("" untuk marketplace tanpa pelanggan).
func (o Order) PartyType() (string, *uuid.UUID) {
	switch {
	case o.PartnerID != nil:
		return PartyPartner, o.PartnerID
	case o.CustomerID != nil:
		return PartyCustomer, o.CustomerID
	}
	return "", nil
}

// Label mengembalikan nomor SO atau "Draft Sales Order".
func (o Order) Label() string {
	if o.Number != "" {
		return o.Number
	}
	return "Draft Sales Order"
}

// HasShipment bernilai true bila ada barang yang sudah dikirim.
func (o Order) HasShipment() bool {
	for _, a := range o.Allocations {
		if a.ShippedQty.IsPositive() {
			return true
		}
	}
	return false
}

// HasOutstanding bernilai true bila pesanan Diproses masih punya alokasi yang belum dikirim (bisa dibuatkan DO).
func (o Order) HasOutstanding() bool {
	if o.Status != StatusInProgress {
		return false
	}
	for _, a := range o.Allocations {
		if !a.Released && a.OutstandingQty().IsPositive() {
			return true
		}
	}
	return false
}

// FullyShipped bernilai true bila semua alokasi sudah dikirim.
func (o Order) FullyShipped() bool {
	if len(o.Allocations) == 0 {
		return false
	}
	for _, a := range o.Allocations {
		if !a.Released && a.OutstandingQty().IsPositive() {
			return false
		}
	}
	return true
}

// PartyInfo adalah pelanggan atau mitra pesanan dari modul party.
type PartyInfo struct {
	ID      uuid.UUID
	Kind    string
	Code    string
	Name    string
	Phone   string
	Type    string
	Address string
	Deleted bool
}

// ProductInfo adalah produk katalog untuk validasi baris.
type ProductInfo struct {
	ID       uuid.UUID
	SKU      string
	Type     string
	IsBundle bool
	IsActive bool
	Deleted  bool
}

// ProductFinishedGood adalah jenis produk katalog yang boleh dijual.
const ProductFinishedGood = "finished_good"

// LocationInfo adalah lokasi asal stok.
type LocationInfo struct {
	ID       uuid.UUID
	Type     string
	IsActive bool
	Deleted  bool
}

// OrderRefs adalah data rujukan dari modul lain untuk validasi pesanan.
type OrderRefs struct {
	// ChannelCode adalah kode channel ChannelID (kosong bila tidak ditemukan).
	ChannelCode string
	// SubChannelValid: sub-channel aktif dan milik ChannelID.
	SubChannelValid bool
	Party           *PartyInfo
	// EmployeeEligible: pelanggan karyawan masih aktif pada tanggal pesanan (party.EmployeePriceEligible).
	EmployeeEligible bool
	Products         map[uuid.UUID]ProductInfo
	Location         *LocationInfo
	// Prices adalah harga per baris input (urutan sama); nil = harga belum bisa ditentukan (error dari resolver
	// dikirim service).
	Prices []*LinePrice
}

// LineInput adalah isian satu baris pesanan. UnitPrice hanya untuk marketplace (harga di marketplace).
type LineInput struct {
	ProductID uuid.UUID
	Qty       decimal.Decimal
	Discount  Discount
	UnitPrice *money.Amount
	Note      string
}

// OrderInput adalah isian form pesanan.
type OrderInput struct {
	Type             OrderType
	ChannelID        uuid.UUID
	SubChannelID     uuid.UUID
	CustomerID       *uuid.UUID
	PartnerID        *uuid.UUID
	BuyerName        string
	ExternalOrderID  string
	ClientPONumber   string
	BusinessDate     clock.BusinessDate
	SourceLocationID uuid.UUID
	PaymentMode      PaymentMode
	PaymentTermDays  int
	ShipToName       string
	ShipToPhone      string
	ShipToAddress    string
	DocumentDiscount Discount
	Shipping         money.Amount
	Note             string
	Lines            []LineInput
}

func (in *OrderInput) normalize() {
	in.BuyerName = strings.TrimSpace(in.BuyerName)
	in.ExternalOrderID = strings.TrimSpace(in.ExternalOrderID)
	in.ClientPONumber = strings.TrimSpace(in.ClientPONumber)
	in.ShipToName = strings.TrimSpace(in.ShipToName)
	in.ShipToPhone = strings.TrimSpace(in.ShipToPhone)
	in.ShipToAddress = strings.TrimSpace(in.ShipToAddress)
	in.Note = strings.TrimSpace(in.Note)
	in.DocumentDiscount = in.DocumentDiscount.normalized()
	if in.PaymentMode == "" {
		in.PaymentMode = PaymentPrepaid
	}
	for i := range in.Lines {
		in.Lines[i].Note = strings.TrimSpace(in.Lines[i].Note)
		in.Lines[i].Discount = in.Lines[i].Discount.normalized()
	}
}

type fieldErrors map[string]string

func (f fieldErrors) add(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	return apperr.Validation(f)
}

// Validate memeriksa isian pesanan terhadap data rujukan. Error per field memakai nama field API.
func (in *OrderInput) Validate(refs OrderRefs) error {
	in.normalize()
	f := fieldErrors{}
	if !in.Type.Valid() {
		f.add("orderType", "Jenis pesanan tidak dikenal")
		return f.err()
	}
	in.validateHeader(refs, f)
	in.validateParty(refs, f)
	in.validateLines(refs, f)
	return f.err()
}

func (in *OrderInput) validateHeader(refs OrderRefs, f fieldErrors) {
	switch {
	case in.ChannelID == uuid.Nil:
		f.add("channelId", "Pilih channel")
	case refs.ChannelCode != in.Type.ChannelCode():
		f.add("channelId", fmt.Sprintf("Channel harus %s untuk pesanan %s", in.Type.ChannelCode(), in.Type.Label()))
	}
	switch {
	case in.SubChannelID == uuid.Nil:
		f.add("subChannelId", "Pilih sub-channel")
	case !refs.SubChannelValid:
		f.add("subChannelId", "Sub-channel tidak ditemukan, nonaktif, atau bukan bagian dari channel")
	}
	if in.BusinessDate.IsZero() {
		f.add("businessDate", "Tanggal pesanan wajib diisi")
	}
	loc := refs.Location
	switch {
	case in.SourceLocationID == uuid.Nil:
		f.add("sourceLocationId", "Pilih lokasi asal stok")
	case loc == nil || loc.Deleted:
		f.add("sourceLocationId", "Lokasi tidak ditemukan")
	case !loc.IsActive:
		f.add("sourceLocationId", "Lokasi nonaktif")
	case !slices.Contains(in.Type.LocationTypes(), loc.Type):
		f.add("sourceLocationId", "Lokasi asal stok tidak boleh dipakai pesanan "+in.Type.Label())
	}
	switch {
	case !in.PaymentMode.Valid():
		f.add("paymentMode", "Cara bayar tidak dikenal")
	case in.PaymentMode == PaymentCredit && !in.Type.AllowsCredit():
		f.add("paymentMode", "Tempo hanya untuk Modern Trade dan B2B")
	}
	if in.PaymentTermDays < 0 || in.PaymentTermDays > 365 {
		f.add("paymentTermDays", "Termin 0–365 hari")
	}
	if in.Type == OrderMarketplace {
		switch {
		case in.ExternalOrderID == "":
			f.add("externalOrderId", "Nomor order marketplace wajib diisi")
		case tooLong(in.ExternalOrderID, 80):
			f.add("externalOrderId", "Nomor order maksimal 80 karakter")
		}
		if in.Shipping.IsPositive() {
			f.add("shippingAmount", "Ongkos kirim marketplace tidak dicatat di pesanan")
		}
	} else if in.ExternalOrderID != "" {
		f.add("externalOrderId", "Nomor order eksternal hanya untuk marketplace")
	}
	if msg := validAmount(in.Shipping, false); msg != "" {
		f.add("shippingAmount", msg)
	}
	for field, v := range map[string]string{
		"clientPoNumber": in.ClientPONumber, "shipToName": in.ShipToName, "shipToPhone": in.ShipToPhone,
		"buyerName": in.BuyerName,
	} {
		if tooLong(v, MaxTextLength) {
			f.add(field, "Maksimal 200 karakter")
		}
	}
	if tooLong(in.ShipToAddress, MaxNoteLength) || tooLong(in.Note, MaxNoteLength) {
		f.add("note", "Alamat/catatan maksimal 1000 karakter")
	}
}

func (in *OrderInput) validateParty(refs OrderRefs, f fieldErrors) {
	p := refs.Party
	if in.CustomerID != nil && in.PartnerID != nil {
		f.add("customerId", "Pilih pelanggan atau mitra, bukan keduanya")
		return
	}
	needParty := func(field, what string) bool {
		if p == nil || p.Deleted {
			f.add(field, what+" tidak ditemukan")
			return false
		}
		return true
	}
	switch in.Type {
	case OrderModernTrade:
		if in.PartnerID == nil {
			f.add("partnerId", "Pilih mitra Modern Trade")
		} else if needParty("partnerId", "Mitra") && p.Type != PartnerTypeModernTrade && p.Type != PartnerTypeConsignment {
			f.add("partnerId", "Mitra bukan mitra Modern Trade")
		}
	case OrderB2B:
		switch {
		case in.PartnerID != nil:
			if needParty("partnerId", "Mitra") && p.Type != PartnerTypeB2B {
				f.add("partnerId", "Mitra bukan mitra B2B/corporate")
			}
		case in.CustomerID != nil:
			if needParty("customerId", "Pelanggan") && p.Type != CustomerTypeB2B {
				f.add("customerId", "Pelanggan bukan pelanggan B2B/corporate")
			}
		default:
			f.add("partnerId", "Pilih mitra atau pelanggan B2B")
		}
	case OrderSocialCommerce:
		if in.CustomerID == nil {
			f.add("customerId", "Pilih pelanggan atau buat pelanggan baru")
		} else {
			needParty("customerId", "Pelanggan")
		}
	case OrderEmployee:
		switch {
		case in.CustomerID == nil:
			f.add("customerId", "Pilih pelanggan karyawan")
		case !needParty("customerId", "Pelanggan"):
		case p.Type != CustomerTypeEmployee:
			f.add("customerId", "Pelanggan bukan pelanggan bertipe Karyawan")
		case !refs.EmployeeEligible:
			f.add("customerId", "Karyawan sudah tidak aktif; harga karyawan tidak berlaku")
		}
	case OrderMarketplace:
		if in.CustomerID != nil || in.PartnerID != nil {
			f.add("customerId", "Pesanan marketplace tidak memakai master pelanggan")
		}
	}
}

func (in *OrderInput) validateLines(refs OrderRefs, f fieldErrors) {
	switch {
	case len(in.Lines) == 0:
		f.add("lines", "Minimal satu baris")
	case len(in.Lines) > MaxLines:
		f.add("lines", fmt.Sprintf("Maksimal %d baris", MaxLines))
	}
	seen := map[uuid.UUID]int{}
	for i, l := range in.Lines {
		key := fmt.Sprintf("lines.%d.", i)
		p, ok := refs.Products[l.ProductID]
		switch {
		case l.ProductID == uuid.Nil:
			f.add(key+"productId", "Pilih produk")
		case !ok || p.Deleted:
			f.add(key+"productId", "Produk tidak ditemukan")
		case !p.IsActive:
			f.add(key+"productId", "Produk "+p.SKU+" nonaktif")
		case p.Type != ProductFinishedGood:
			f.add(key+"productId", "Hanya Produk Jadi atau bundel yang bisa dijual")
		}
		if j, dup := seen[l.ProductID]; dup && l.ProductID != uuid.Nil {
			f.add(key+"productId", fmt.Sprintf("Produk sama dengan baris %d", j+1))
		}
		seen[l.ProductID] = i
		if msg := ValidQty(l.Qty); msg != "" {
			f.add(key+"qty", msg)
		}
		if l.UnitPrice != nil {
			if in.Type != OrderMarketplace {
				f.add(key+"unitPrice", "Harga diambil otomatis dari daftar harga")
			} else if msg := validAmount(*l.UnitPrice, false); msg != "" {
				f.add(key+"unitPrice", msg)
			}
		}
		if tooLong(l.Note, MaxTextLength) {
			f.add(key+"note", "Catatan baris maksimal 200 karakter")
		}
		if i >= len(refs.Prices) || refs.Prices[i] == nil {
			f.add(key+"productId", "Harga produk belum tersedia")
		}
	}
}

// NewOrder membuat draft pesanan dari isian yang sudah divalidasi (Validate) dengan harga per baris.
func NewOrder(in OrderInput, refs OrderRefs, createdBy uuid.UUID) (Order, error) {
	if err := in.Validate(refs); err != nil {
		return Order{}, err
	}
	o := Order{Type: in.Type, Status: StatusDraft, CreatedBy: createdBy}
	if err := o.apply(in, refs); err != nil {
		return Order{}, err
	}
	return o, nil
}

// Update mengubah draft pesanan. Jenis pesanan tetap.
func (o *Order) Update(in OrderInput, refs OrderRefs) error {
	if o.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			fmt.Sprintf("Sales Order berstatus %s tidak bisa diubah.", o.Status.Label()))
	}
	in.Type = o.Type
	if err := in.Validate(refs); err != nil {
		return err
	}
	return o.apply(in, refs)
}

func (o *Order) apply(in OrderInput, refs OrderRefs) error {
	o.ChannelID, o.SubChannelID = in.ChannelID, in.SubChannelID
	o.CustomerID, o.PartnerID = in.CustomerID, in.PartnerID
	o.PartyCode, o.PartyName, o.PartyPhone = "", in.BuyerName, ""
	if refs.Party != nil && in.Type != OrderMarketplace {
		o.PartyCode, o.PartyName, o.PartyPhone = refs.Party.Code, refs.Party.Name, refs.Party.Phone
	}
	o.ExternalOrderID, o.ClientPONumber = in.ExternalOrderID, in.ClientPONumber
	o.BusinessDate, o.SourceLocationID = in.BusinessDate, in.SourceLocationID
	o.PaymentMode, o.PaymentTermDays = in.PaymentMode, in.PaymentTermDays
	if o.PaymentMode == PaymentPrepaid {
		o.PaymentTermDays = 0
	}
	o.ShipToName, o.ShipToPhone, o.ShipToAddress = in.ShipToName, in.ShipToPhone, in.ShipToAddress
	if o.ShipToAddress == "" && refs.Party != nil {
		o.ShipToAddress = refs.Party.Address
	}
	o.Note, o.DocumentDiscount, o.Shipping = in.Note, in.DocumentDiscount, in.Shipping
	o.Lines = make([]Line, len(in.Lines))
	for i, l := range in.Lines {
		o.Lines[i] = Line{
			LineNo: i + 1, ProductID: l.ProductID, IsBundle: refs.Products[l.ProductID].IsBundle, Qty: l.Qty,
			Price: *refs.Prices[i], Discount: l.Discount, Note: l.Note,
		}
	}
	return o.Recalculate()
}

// Recalculate menghitung bruto, diskon, dan total baris serta dokumen.
func (o *Order) Recalculate() error {
	f := fieldErrors{}
	o.Gross, o.LineDiscount = money.Zero, money.Zero
	for i := range o.Lines {
		l := &o.Lines[i]
		l.Gross = l.Price.UnitPrice.Mul(l.Qty).Round()
		disc, msg := l.Discount.Apply(l.Gross)
		if msg != "" {
			f.add(fmt.Sprintf("lines.%d.discount", i), msg)
		}
		l.DiscountAmount, l.Total = disc, l.Gross.Sub(disc)
		o.Gross, o.LineDiscount = o.Gross.Add(l.Gross), o.LineDiscount.Add(disc)
	}
	disc, msg := o.DocumentDiscount.Apply(o.Subtotal())
	if msg != "" {
		f.add("documentDiscount", msg)
	}
	o.DocumentDiscountAmount = disc
	o.Total = o.Subtotal().Sub(disc).Add(o.Shipping)
	return f.err()
}

var orderTransitions = transitions{
	StatusDraft:           {StatusPendingApproval, StatusAwaitingPayment, StatusInProgress, StatusCancelled},
	StatusPendingApproval: {StatusDraft, StatusAwaitingPayment, StatusInProgress, StatusCancelled},
	StatusAwaitingPayment: {StatusInProgress, StatusExpired, StatusCancelled},
	StatusInProgress:      {StatusInTransit, StatusCancelled},
	StatusInTransit:       {StatusCompleted},
}

func (o *Order) transition(to Status) error {
	if !orderTransitions.allowed(o.Status, to) {
		return invalidTransition("Sales Order "+o.Label(), o.Status, to)
	}
	o.Status = to
	return nil
}

// statusAfterConfirm: prepaid → Menunggu Pembayaran (Proforma terbit); tempo dan marketplace → Diproses.
func (o Order) statusAfterConfirm() Status {
	if o.Type == OrderMarketplace || o.PaymentMode == PaymentCredit {
		return StatusInProgress
	}
	return StatusAwaitingPayment
}

// Confirm mengonfirmasi draft: nomor SO diberikan (bila belum), stok dialokasikan oleh service, lalu status
// Menunggu Persetujuan bila diskon melebihi batas role, selain itu Menunggu Pembayaran/Diproses.
func (o *Order) Confirm(number string, requiresApproval bool, now time.Time) error {
	if o.Status != StatusDraft {
		return invalidTransition("Sales Order "+o.Label(), o.Status, StatusPendingApproval)
	}
	if !o.Total.IsPositive() && o.Type != OrderMarketplace {
		return apperr.Validation(map[string]string{"lines": "Total pesanan harus lebih dari nol"})
	}
	if o.Number == "" {
		o.Number = number
	}
	o.ConfirmedAt = &now
	o.DiscountApprovalRequired = requiresApproval
	o.DiscountApprovedAt = nil
	o.AllocationRound++
	if requiresApproval {
		return o.transition(StatusPendingApproval)
	}
	return o.transition(o.statusAfterConfirm())
}

// ApproveDiscount melanjutkan pesanan setelah diskon disetujui Head Sales.
func (o *Order) ApproveDiscount(now time.Time) error {
	if o.Status != StatusPendingApproval {
		return invalidTransition("Sales Order "+o.Label(), o.Status, o.statusAfterConfirm())
	}
	o.DiscountApprovedAt = &now
	return o.transition(o.statusAfterConfirm())
}

// RejectDiscount mengembalikan pesanan ke Draft; alokasi stok dilepas service.
func (o *Order) RejectDiscount() error {
	if o.Status != StatusPendingApproval {
		return invalidTransition("Sales Order "+o.Label(), o.Status, StatusDraft)
	}
	return o.transition(StatusDraft)
}

// MarkPaid: pembayaran Proforma dikonfirmasi → invoice lunas, pesanan siap dikirim.
func (o *Order) MarkPaid() error {
	if o.Status != StatusAwaitingPayment {
		return invalidTransition("Sales Order "+o.Label(), o.Status, StatusInProgress)
	}
	return o.transition(StatusInProgress)
}

// Expire: Proforma lewat masa berlaku → pesanan kedaluwarsa, alokasi dilepas service.
func (o *Order) Expire() error { return o.transition(StatusExpired) }

// Cancel membatalkan pesanan yang belum ada barang dikirim (alasan wajib selain draft).
func (o *Order) Cancel(reason string) error {
	if o.Status != StatusDraft {
		r, err := requireReason(reason, "pembatalan")
		if err != nil {
			return err
		}
		reason = r
	}
	if o.HasShipment() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Sales Order "+o.Label()+" sudah ada barang yang dikirim sehingga tidak bisa dibatalkan.")
	}
	if err := o.transition(StatusCancelled); err != nil {
		return err
	}
	o.CancelReason = strings.TrimSpace(reason)
	return nil
}

// AttachPaymentProof mencatat bukti bayar dari pelanggan (WA/IG, karyawan, MT prepaid).
func (o *Order) AttachPaymentProof(fileID uuid.UUID, now time.Time) error {
	if o.Status != StatusAwaitingPayment {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Bukti bayar hanya untuk pesanan yang Menunggu Pembayaran.")
	}
	o.PaymentProofFileID, o.PaymentProofUploadedAt = &fileID, &now
	return nil
}

// ShipmentQty adalah qty yang dikirim untuk satu alokasi.
type ShipmentQty struct {
	AllocationID uuid.UUID
	Qty          decimal.Decimal
}

// PlanShipment menentukan qty kirim per alokasi: kosong = semua sisa alokasi. Qty melebihi sisa ditolak.
// Marketplace wajib serah ke kurir seluruh pesanan sekaligus (penjualan diakui per order).
func (o Order) PlanShipment(lines []ShipmentQty) ([]ShipmentQty, error) {
	if o.Status != StatusInProgress {
		return nil, apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Sales Order %s berstatus %s; hanya pesanan Diproses yang bisa dikirim.", o.Label(), o.Status.Label()))
	}
	byID := map[uuid.UUID]Allocation{}
	for _, a := range o.Allocations {
		if !a.Released {
			byID[a.ID] = a
		}
	}
	if len(lines) == 0 {
		for _, a := range o.Allocations {
			if !a.Released && a.OutstandingQty().IsPositive() {
				lines = append(lines, ShipmentQty{AllocationID: a.ID, Qty: a.OutstandingQty()})
			}
		}
	}
	f := fieldErrors{}
	total := map[uuid.UUID]decimal.Decimal{}
	for i, l := range lines {
		a, ok := byID[l.AllocationID]
		key := fmt.Sprintf("lines.%d.", i)
		if !ok {
			f.add(key+"allocationId", "Alokasi tidak ditemukan di Sales Order ini")
			continue
		}
		if msg := ValidQty(l.Qty); msg != "" {
			f.add(key+"qty", msg)
			continue
		}
		total[a.ID] = total[a.ID].Add(l.Qty)
		if total[a.ID].GreaterThan(a.OutstandingQty()) {
			f.add(key+"qty", "Qty melebihi sisa alokasi "+a.OutstandingQty().String())
		}
	}
	if len(lines) == 0 {
		f.add("lines", "Tidak ada barang yang belum dikirim")
	}
	if err := f.err(); err != nil {
		return nil, err
	}
	if o.Type.RecognizedAtHandover() {
		for _, a := range byID {
			if !total[a.ID].Equal(a.OutstandingQty()) {
				return nil, apperr.Validation(map[string]string{"lines": "Pesanan marketplace diserahkan ke kurir sekaligus"})
			}
		}
	}
	return lines, nil
}

// ApplyShipment menambah qty dikirim; semua terkirim → Dalam Pengiriman.
func (o *Order) ApplyShipment(lines []ShipmentQty, now time.Time) error {
	for _, l := range lines {
		for i := range o.Allocations {
			if o.Allocations[i].ID == l.AllocationID {
				o.Allocations[i].ShippedQty = o.Allocations[i].ShippedQty.Add(l.Qty)
			}
		}
	}
	if o.FullyShipped() {
		o.ShippedAt = &now
		return o.transition(StatusInTransit)
	}
	return nil
}

// Deliver menandai pesanan diterima pelanggan.
func (o *Order) Deliver(now time.Time) error {
	if err := o.transition(StatusCompleted); err != nil {
		return err
	}
	o.DeliveredAt = &now
	return nil
}

// Actions adalah aksi status pesanan (sebelum disaring permission).
func (o Order) Actions() []Action {
	switch o.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionConfirm, ActionCancel}
	case StatusPendingApproval:
		return []Action{ActionCancel}
	case StatusAwaitingPayment:
		return []Action{ActionUploadProof, ActionCancel}
	case StatusInProgress:
		if !o.HasShipment() {
			return []Action{ActionCancel}
		}
	case StatusPaid, StatusPartiallyPaid, StatusInTransit, StatusCompleted, StatusCancelled, StatusExpired, StatusPosted:
	}
	return nil
}

// ComponentQty adalah qty satu komponen per unit produk baris (produk biasa = dirinya sendiri qty 1).
type ComponentQty struct {
	ProductID uuid.UUID
	Qty       decimal.Decimal
}

// PlanAllocations memecah baris (bundel → komponen, TG 5.7.4) menjadi alokasi stok di lokasi asal.
// components memuat komponen per produk bundel.
func (o Order) PlanAllocations(components map[uuid.UUID][]ComponentQty, newID func() uuid.UUID) ([]Allocation, error) {
	var out []Allocation
	f := fieldErrors{}
	for i, l := range o.Lines {
		parts := []ComponentQty{{ProductID: l.ProductID, Qty: decimal.NewFromInt(1)}}
		if l.IsBundle {
			parts = components[l.ProductID]
			if len(parts) == 0 {
				f.add(fmt.Sprintf("lines.%d.productId", i), "Bundel belum punya komponen")
				continue
			}
		}
		for _, c := range parts {
			out = append(out, Allocation{
				ID: newID(), LineID: l.ID, ProductID: c.ProductID, LocationID: o.SourceLocationID,
				Qty: c.Qty.Mul(l.Qty).Round(QtyScale),
			})
		}
	}
	if err := f.err(); err != nil {
		return nil, err
	}
	return out, nil
}
