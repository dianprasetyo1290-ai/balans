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

// RejectDisposition adalah perlakuan qty reject saat penerimaan (BRD 5.2, 5.3; Desain Data Model 5).
type RejectDisposition string

const (
	// RejectReturnToSupplier: reject ditolak di tempat dan diganti pemasok; tidak masuk stok, 2102, maupun tagihan.
	RejectReturnToSupplier RejectDisposition = "return_to_supplier"
	// RejectAbsorbed: reject ditanggung Balans; ikut dicatat diterima & ditagih, nilainya diserap ke HPP barang baik.
	RejectAbsorbed RejectDisposition = "absorbed"
)

// Valid memeriksa perlakuan reject.
func (d RejectDisposition) Valid() bool { return d == RejectReturnToSupplier || d == RejectAbsorbed }

// receiptTransitions: Desain Status Dokumen 2.3.
var receiptTransitions = transitions{
	StatusDraft:  {StatusPosted, StatusCancelled},
	StatusPosted: {StatusCancelled},
}

// Jenis lokasi yang dipakai penerimaan dan retur (kode warehouse.LocationType).
const (
	LocationInbound          = "inbound"
	LocationReturns          = "returns"
	LocationConsignment      = "consignment"
	LocationVendingBuffer    = "vending_buffer"
	LocationVendingMachine   = "vending_machine"
	LocationTollManufacturer = "toll_manufacturer"
)

// Akun persediaan COA (Desain Finance & Accounting 1).
const (
	AccountPackagingInventory = "1301"
	AccountReprocessInventory = "1302"
	AccountFGWarehouse        = "1311"
	AccountFGConsignment      = "1312"
	AccountFGVending          = "1313"
	AccountReturnsInventory   = "1314"
)

// InventoryAccount memilih akun persediaan dari jenis produk dan jenis lokasi: packaging 1301, material GM 1302,
// FG sesuai lokasi (gudang 1311, konsinyasi 1312, vending 1313, retur 1314).
func InventoryAccount(productType, locationType string) string {
	switch productType {
	case ProductPackaging:
		return AccountPackagingInventory
	case ProductReprocessMaterial:
		return AccountReprocessInventory
	}
	switch locationType {
	case LocationReturns:
		return AccountReturnsInventory
	case LocationConsignment:
		return AccountFGConsignment
	case LocationVendingBuffer, LocationVendingMachine:
		return AccountFGVending
	}
	return AccountFGWarehouse
}

// UnitCostScale adalah presisi HPP per unit (Technical Guideline 5.8.6).
const UnitCostScale = 4

// MaxBatchNumberLength sama dengan batas nomor batch modul inventory.
const MaxBatchNumberLength = 40

// ReceiptLine adalah satu baris Penerimaan Barang terhadap baris PO.
type ReceiptLine struct {
	ID                uuid.UUID
	LineNo            int
	OrderLineID       uuid.UUID
	ProductID         uuid.UUID
	GoodQty           decimal.Decimal
	RejectQty         decimal.Decimal
	RejectDisposition RejectDisposition
	BatchNumber       string
	ProductionDate    *clock.BusinessDate
	ExpiryDate        *clock.BusinessDate
	BatchID           *uuid.UUID
	// UnitPrice adalah harga PO (harga jasa untuk maklon/reprocess).
	UnitPrice money.Amount
	// GRNIQty = qty baik + reject diserap; GRNIAmount = GRNIQty × harga PO (masuk 2102).
	GRNIQty    decimal.Decimal
	GRNIAmount money.Amount
	// MaterialCost: bagian packaging/bahan terpakai maklon yang dialokasikan ke baris.
	MaterialCost money.Amount
	// InventoryAmount adalah nilai batch; UnitCost = nilai batch ÷ qty baik (4 desimal).
	InventoryAmount     money.Amount
	UnitCost            decimal.Decimal
	InventoryAccount    string
	BilledQty           decimal.Decimal
	BilledGRNIAmount    money.Amount
	ReturnedQty         decimal.Decimal
	ReturnedUnbilledQty decimal.Decimal
	ReturnedGRNIAmount  money.Amount
	Note                string
}

// OpenQty adalah qty yang belum ditagih dan belum diretur sebelum ditagih (3-way match).
func (l ReceiptLine) OpenQty() decimal.Decimal {
	return l.GRNIQty.Sub(l.BilledQty).Sub(l.ReturnedUnbilledQty)
}

// OpenGRNIAmount adalah saldo Barang Diterima Belum Ditagih baris ini.
func (l ReceiptLine) OpenGRNIAmount() money.Amount {
	return l.GRNIAmount.Sub(l.BilledGRNIAmount).Sub(l.ReturnedGRNIAmount)
}

// ReturnableQty adalah qty baik yang masih bisa diretur ke pemasok.
func (l ReceiptLine) ReturnableQty() decimal.Decimal { return l.GoodQty.Sub(l.ReturnedQty) }

// MaterialConsumption adalah satu batch packaging yang keluar dari lokasi maklon beserta HPP-nya.
type MaterialConsumption struct {
	BatchID  *uuid.UUID
	Qty      decimal.Decimal
	UnitCost decimal.Decimal
}

// MaterialUsage adalah pemakaian packaging/bahan GM di maklon (baik + reject) untuk satu Penerimaan Barang.
type MaterialUsage struct {
	ID             uuid.UUID
	LineNo         int
	ProductID      uuid.UUID
	FromLocationID uuid.UUID
	UsedGoodQty    decimal.Decimal
	UsedRejectQty  decimal.Decimal
	CostAmount     money.Amount
	AccountCode    string
	Note           string
	Consumptions   []MaterialConsumption
}

// TotalQty adalah qty bahan yang keluar dari lokasi maklon (baik + reject).
func (m MaterialUsage) TotalQty() decimal.Decimal { return m.UsedGoodQty.Add(m.UsedRejectQty) }

// GoodsReceipt adalah Penerimaan Barang per PO (PRC-04, PRC-05, PRC-10).
type GoodsReceipt struct {
	ID              uuid.UUID
	Number          string
	PurchaseOrderID uuid.UUID
	OrderNumber     string
	SupplierID      uuid.UUID
	SupplierName    string
	DivisionID      uuid.UUID
	OrderType       OrderType
	BusinessDate    clock.BusinessDate
	LocationID      uuid.UUID
	TollLocationID  *uuid.UUID
	DeliveryNote    string
	Note            string
	InventoryAmount money.Amount
	MaterialAmount  money.Amount
	GRNIAmount      money.Amount
	Status          Status
	InboundID       *uuid.UUID
	JournalEntryID  *uuid.UUID
	ReversalEntryID *uuid.UUID
	CancelReason    string
	PostedAt        *time.Time
	CancelledAt     *time.Time
	Lines           []ReceiptLine
	Materials       []MaterialUsage
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// Label mengembalikan nomor GR atau keterangan draft.
func (r GoodsReceipt) Label() string {
	if r.Number != "" {
		return r.Number
	}
	return "Draft penerimaan " + r.OrderNumber
}

// IsToll bernilai true untuk penerimaan FG dari maklon/reprocess (A-03).
func (r GoodsReceipt) IsToll() bool { return r.OrderType.IsToll() }

// Line mengembalikan baris berdasarkan ID.
func (r *GoodsReceipt) Line(id uuid.UUID) (*ReceiptLine, bool) {
	for i := range r.Lines {
		if r.Lines[i].ID == id {
			return &r.Lines[i], true
		}
	}
	return nil, false
}

// ReceiptLineInput adalah isian satu baris penerimaan.
type ReceiptLineInput struct {
	OrderLineID       uuid.UUID
	GoodQty           decimal.Decimal
	RejectQty         decimal.Decimal
	RejectDisposition RejectDisposition
	BatchNumber       string
	ProductionDate    *clock.BusinessDate
	ExpiryDate        *clock.BusinessDate
	Note              string
}

// MaterialUsageInput adalah isian pemakaian satu packaging/bahan di maklon.
type MaterialUsageInput struct {
	ProductID     uuid.UUID
	UsedGoodQty   decimal.Decimal
	UsedRejectQty decimal.Decimal
	Note          string
}

// ReceiptInput adalah isian form Penerimaan Barang.
type ReceiptInput struct {
	PurchaseOrderID uuid.UUID
	BusinessDate    clock.BusinessDate
	LocationID      uuid.UUID
	DeliveryNote    string
	Note            string
	Lines           []ReceiptLineInput
	Materials       []MaterialUsageInput
}

// ReceiptProduct adalah data produk untuk validasi penerimaan.
type ReceiptProduct struct {
	Type     string
	HasBatch bool
}

// ReceiptLocation adalah lokasi tujuan penerimaan.
type ReceiptLocation struct {
	Type     string
	IsActive bool
	Deleted  bool
}

// ReceiptRefs adalah data rujukan validasi penerimaan.
type ReceiptRefs struct {
	Order    PurchaseOrder
	Products map[uuid.UUID]ReceiptProduct
	// Location nil = lokasi tidak ditemukan.
	Location *ReceiptLocation
}

// grniQty adalah qty yang dicatat diterima di PO, 2102, dan tagihan: baik + reject diserap.
func (in ReceiptLineInput) grniQty() decimal.Decimal {
	if in.RejectDisposition == RejectAbsorbed {
		return in.GoodQty.Add(in.RejectQty)
	}
	return in.GoodQty
}

func (in *ReceiptLineInput) normalize() {
	in.BatchNumber = strings.ToUpper(strings.TrimSpace(in.BatchNumber))
	in.Note = strings.TrimSpace(in.Note)
	if in.RejectDisposition == "" {
		in.RejectDisposition = RejectAbsorbed
	}
}

// Validate memeriksa isian penerimaan terhadap PO (sisa qty, toleransi 0%), produk (batch wajib untuk produk
// berbatch), lokasi Inbound, dan pemakaian bahan maklon.
func (in *ReceiptInput) Validate(refs ReceiptRefs) error {
	o := refs.Order
	if !o.Receivable() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; hanya PO Disetujui atau Diterima Sebagian yang bisa diterima.",
			o.Label(), o.Status.Label()))
	}
	fields := map[string]string{}
	switch {
	case in.BusinessDate.IsZero():
		fields["businessDate"] = "Tanggal terima wajib diisi"
	case in.BusinessDate.Before(o.BusinessDate):
		fields["businessDate"] = "Tanggal terima tidak boleh sebelum tanggal PO"
	}
	switch l := refs.Location; {
	case in.LocationID == uuid.Nil:
		fields["locationId"] = "Lokasi Inbound wajib dipilih"
	case l == nil || l.Deleted:
		fields["locationId"] = "Lokasi tidak ditemukan"
	case !l.IsActive:
		fields["locationId"] = "Lokasi nonaktif"
	case l.Type != LocationInbound:
		fields["locationId"] = "Barang diterima ke lokasi Inbound (Karantina sampai QC)"
	}
	in.DeliveryNote, in.Note = strings.TrimSpace(in.DeliveryNote), strings.TrimSpace(in.Note)
	if len([]rune(in.DeliveryNote)) > 100 {
		fields["deliveryNote"] = "Nomor surat jalan maksimal 100 karakter"
	}
	if len([]rune(in.Note)) > MaxNoteLength {
		fields["note"] = "Catatan maksimal 1.000 karakter"
	}
	in.validateLines(refs, fields)
	in.validateMaterials(refs, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (in *ReceiptInput) validateLines(refs ReceiptRefs, fields map[string]string) {
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
		return
	case len(in.Lines) > MaxLines:
		fields["lines"] = "Maksimal 200 baris"
	}
	o := refs.Order
	perLine := map[uuid.UUID]decimal.Decimal{}
	for i := range in.Lines {
		l := &in.Lines[i]
		l.normalize()
		p := fmt.Sprintf("lines.%d.", i)
		ol, ok := o.Line(l.OrderLineID)
		switch {
		case !ok:
			fields[p+"orderLineId"] = "Baris PO tidak ditemukan"
			continue
		case ol.ProductID == nil:
			fields[p+"orderLineId"] = "Baris PO tanpa produk katalog tidak melalui Penerimaan Barang"
			continue
		}
		if msg := validQty(l.GoodQty); msg != "" {
			fields[p+"goodQty"] = msg
		}
		switch {
		case l.RejectQty.IsNegative():
			fields[p+"rejectQty"] = "Qty reject tidak boleh negatif"
		case !l.RejectQty.Equal(l.RejectQty.Round(QtyScale)):
			fields[p+"rejectQty"] = "Qty maksimal 3 angka di belakang koma"
		}
		if !l.RejectDisposition.Valid() {
			fields[p+"rejectDisposition"] = "Perlakuan reject tidak dikenal"
		}
		if len([]rune(l.Note)) > 500 {
			fields[p+"note"] = "Catatan maksimal 500 karakter"
		}
		validateBatch(*l, refs.Products[*ol.ProductID], p, fields)
		perLine[ol.ID] = perLine[ol.ID].Add(l.grniQty())
		if next := perLine[ol.ID]; next.GreaterThan(ol.OutstandingQty()) {
			fields[p+"goodQty"] = fmt.Sprintf("Melebihi sisa PO baris %d (%s); toleransi qty 0%%", ol.LineNo, ol.OutstandingQty().String())
		}
	}
}

func validateBatch(l ReceiptLineInput, prod ReceiptProduct, p string, fields map[string]string) {
	if !prod.HasBatch {
		if l.BatchNumber != "" || l.ProductionDate != nil || l.ExpiryDate != nil {
			fields[p+"batchNumber"] = "Produk ini tidak dilacak per batch"
		}
		return
	}
	switch {
	case l.BatchNumber == "":
		fields[p+"batchNumber"] = "Nomor batch wajib diisi"
	case len([]rune(l.BatchNumber)) > MaxBatchNumberLength:
		fields[p+"batchNumber"] = "Nomor batch maksimal 40 karakter"
	}
	if l.ProductionDate == nil {
		fields[p+"productionDate"] = "Tanggal produksi wajib diisi"
	}
	switch {
	case l.ExpiryDate == nil:
		fields[p+"expiryDate"] = "Tanggal kedaluwarsa wajib diisi"
	case l.ProductionDate != nil && l.ExpiryDate.Before(*l.ProductionDate):
		fields[p+"expiryDate"] = "Tanggal kedaluwarsa tidak boleh sebelum tanggal produksi"
	}
}

func (in *ReceiptInput) validateMaterials(refs ReceiptRefs, fields map[string]string) {
	t := refs.Order.Type
	if !t.IsToll() {
		if len(in.Materials) > 0 {
			fields["materials"] = "Pemakaian bahan hanya untuk penerimaan PO maklon atau reprocess"
		}
		return
	}
	switch {
	case len(in.Materials) == 0 && t == OrderReprocess:
		fields["materials"] = "Isi pemakaian bahan GM (baik + reject) sesuai laporan maklon"
	case len(in.Materials) == 0:
		fields["materials"] = "Isi pemakaian packaging (baik + reject) sesuai laporan maklon"
	case len(in.Materials) > MaxLines:
		fields["materials"] = "Maksimal 200 bahan"
	}
	seen := map[uuid.UUID]bool{}
	allowed := t.materialProductTypes()
	for i := range in.Materials {
		m := &in.Materials[i]
		m.Note = strings.TrimSpace(m.Note)
		p := fmt.Sprintf("materials.%d.", i)
		prod, ok := refs.Products[m.ProductID]
		switch {
		case m.ProductID == uuid.Nil:
			fields[p+"productId"] = "Bahan wajib dipilih"
		case seen[m.ProductID]:
			fields[p+"productId"] = "Bahan yang sama sudah ada di daftar"
		case !ok:
			fields[p+"productId"] = "Produk tidak ditemukan"
		case prod.Type != allowed[0]:
			fields[p+"productId"] = "Produk harus berjenis " + productTypeLabels[allowed[0]]
		}
		seen[m.ProductID] = true
		for _, q := range []struct {
			v     decimal.Decimal
			field string
		}{{m.UsedGoodQty, "usedGoodQty"}, {m.UsedRejectQty, "usedRejectQty"}} {
			switch {
			case q.v.IsNegative():
				fields[p+q.field] = "Qty tidak boleh negatif"
			case !q.v.Equal(q.v.Round(QtyScale)):
				fields[p+q.field] = "Qty maksimal 3 angka di belakang koma"
			}
		}
		if !m.UsedGoodQty.Add(m.UsedRejectQty).IsPositive() {
			fields[p+"usedGoodQty"] = "Qty terpakai (baik + reject) harus lebih dari nol"
		}
		if len([]rune(m.Note)) > 500 {
			fields[p+"note"] = "Catatan maksimal 500 karakter"
		}
	}
}

func (r *GoodsReceipt) apply(in ReceiptInput, refs ReceiptRefs) {
	o := refs.Order
	r.PurchaseOrderID, r.OrderNumber, r.SupplierID, r.SupplierName = o.ID, o.Number, o.SupplierID, o.SupplierName
	r.DivisionID, r.OrderType, r.TollLocationID = o.DivisionID, o.Type, o.TollLocationID
	r.BusinessDate, r.LocationID, r.DeliveryNote, r.Note = in.BusinessDate, in.LocationID, in.DeliveryNote, in.Note
	r.Lines = make([]ReceiptLine, len(in.Lines))
	for i, l := range in.Lines {
		ol, _ := o.Line(l.OrderLineID)
		grni := l.grniQty()
		r.Lines[i] = ReceiptLine{
			LineNo: i + 1, OrderLineID: ol.ID, ProductID: *ol.ProductID, GoodQty: l.GoodQty, RejectQty: l.RejectQty,
			RejectDisposition: l.RejectDisposition, BatchNumber: l.BatchNumber, ProductionDate: l.ProductionDate,
			ExpiryDate: l.ExpiryDate, UnitPrice: ol.UnitPrice, GRNIQty: grni, GRNIAmount: LineAmount(grni, ol.UnitPrice),
			MaterialCost: money.Zero, InventoryAmount: money.Zero, UnitCost: decimal.Zero, BilledQty: decimal.Zero,
			BilledGRNIAmount: money.Zero, ReturnedQty: decimal.Zero, ReturnedUnbilledQty: decimal.Zero,
			ReturnedGRNIAmount: money.Zero, Note: l.Note,
		}
	}
	r.Materials = make([]MaterialUsage, 0, len(in.Materials))
	for i, m := range in.Materials {
		from := uuid.Nil
		if o.TollLocationID != nil {
			from = *o.TollLocationID
		}
		r.Materials = append(r.Materials, MaterialUsage{
			LineNo: i + 1, ProductID: m.ProductID, FromLocationID: from, UsedGoodQty: m.UsedGoodQty,
			UsedRejectQty: m.UsedRejectQty, CostAmount: money.Zero, Note: m.Note,
		})
	}
	r.recalcTotals()
}

func (r *GoodsReceipt) recalcTotals() {
	r.InventoryAmount, r.MaterialAmount, r.GRNIAmount = money.Zero, money.Zero, money.Zero
	for _, l := range r.Lines {
		r.InventoryAmount = r.InventoryAmount.Add(l.InventoryAmount)
		r.GRNIAmount = r.GRNIAmount.Add(l.GRNIAmount)
	}
	for _, m := range r.Materials {
		r.MaterialAmount = r.MaterialAmount.Add(m.CostAmount)
	}
}

// NewGoodsReceipt membuat draft Penerimaan Barang.
func NewGoodsReceipt(in ReceiptInput, refs ReceiptRefs, createdBy uuid.UUID) (GoodsReceipt, error) {
	if err := in.Validate(refs); err != nil {
		return GoodsReceipt{}, err
	}
	r := GoodsReceipt{Status: StatusDraft, CreatedBy: createdBy}
	r.apply(in, refs)
	return r, nil
}

// Update mengganti isi draft. PO tidak bisa diganti.
func (r *GoodsReceipt) Update(in ReceiptInput, refs ReceiptRefs) error {
	if r.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya Penerimaan Barang berstatus Draft yang bisa diubah.")
	}
	if in.PurchaseOrderID != r.PurchaseOrderID {
		return apperr.Validation(map[string]string{"purchaseOrderId": "Purchase Order penerimaan tidak bisa diganti"})
	}
	if err := in.Validate(refs); err != nil {
		return err
	}
	r.apply(in, refs)
	return nil
}

// Input mengembalikan isian draft (validasi ulang saat posting).
func (r GoodsReceipt) Input() ReceiptInput {
	in := ReceiptInput{
		PurchaseOrderID: r.PurchaseOrderID, BusinessDate: r.BusinessDate, LocationID: r.LocationID,
		DeliveryNote: r.DeliveryNote, Note: r.Note,
	}
	for _, l := range r.Lines {
		in.Lines = append(in.Lines, ReceiptLineInput{
			OrderLineID: l.OrderLineID, GoodQty: l.GoodQty, RejectQty: l.RejectQty, RejectDisposition: l.RejectDisposition,
			BatchNumber: l.BatchNumber, ProductionDate: l.ProductionDate, ExpiryDate: l.ExpiryDate, Note: l.Note,
		})
	}
	for _, m := range r.Materials {
		in.Materials = append(in.Materials, MaterialUsageInput{
			ProductID: m.ProductID, UsedGoodQty: m.UsedGoodQty, UsedRejectQty: m.UsedRejectQty, Note: m.Note,
		})
	}
	return in
}

// OrderDeltas adalah perubahan qty diterima per baris PO (qty baik + reject diserap); sign -1 untuk pembatalan.
func (r GoodsReceipt) OrderDeltas(sign int64) []QtyDelta {
	out := []QtyDelta{}
	idx := map[uuid.UUID]int{}
	for _, l := range r.Lines {
		q := l.GRNIQty.Mul(decimal.NewFromInt(sign))
		if i, ok := idx[l.OrderLineID]; ok {
			out[i].Qty = out[i].Qty.Add(q)
			continue
		}
		idx[l.OrderLineID] = len(out)
		out = append(out, QtyDelta{LineID: l.OrderLineID, Qty: q})
	}
	return out
}

// ApplyCosts menghitung nilai batch dan HPP per unit setelah HPP packaging terpakai diketahui.
//
//   - Barang/packaging (A-01): nilai batch = qty diterima (baik + reject diserap) × harga PO.
//   - Maklon/reprocess (A-03): nilai batch = jasa maklon baris + bagian packaging/bahan terpakai (baik + reject),
//     dialokasikan ke baris FG proporsional qty baik.
//
// HPP per unit = nilai batch ÷ qty baik (4 desimal), sehingga reject ikut menjadi HPP (BRD 5.3 #11). accounts
// adalah akun persediaan per baris (InventoryAccount).
func (r *GoodsReceipt) ApplyCosts(materialCosts []money.Amount, materialAccounts, lineAccounts []string) error {
	if len(materialCosts) != len(r.Materials) || len(materialAccounts) != len(r.Materials) || len(lineAccounts) != len(r.Lines) {
		return fmt.Errorf("apply receipt costs: jumlah biaya tidak sesuai baris")
	}
	totalMaterial := money.Zero
	for i := range r.Materials {
		r.Materials[i].CostAmount, r.Materials[i].AccountCode = materialCosts[i], materialAccounts[i]
		totalMaterial = totalMaterial.Add(materialCosts[i])
	}
	shares := make([]money.Amount, len(r.Lines))
	for i := range shares {
		shares[i] = money.Zero
	}
	if r.IsToll() && !totalMaterial.IsZero() {
		weights := make([]decimal.Decimal, len(r.Lines))
		for i, l := range r.Lines {
			weights[i] = l.GoodQty
		}
		var err error
		if shares, err = money.Allocate(totalMaterial, weights); err != nil {
			return fmt.Errorf("allocate material cost: %w", err)
		}
	}
	for i := range r.Lines {
		l := &r.Lines[i]
		l.MaterialCost = shares[i]
		l.InventoryAmount = l.GRNIAmount.Add(shares[i])
		l.UnitCost = l.InventoryAmount.Decimal().DivRound(l.GoodQty, UnitCostScale)
		l.InventoryAccount = lineAccounts[i]
	}
	r.recalcTotals()
	return nil
}

func (r *GoodsReceipt) transition(to Status) error {
	if !receiptTransitions.allowed(r.Status, to) {
		return invalidTransition("Penerimaan Barang "+r.Label(), r.Status, to)
	}
	r.Status = to
	return nil
}

// Post menandai GR diposting dengan nomor GR dan jurnal A-01/A-03.
func (r *GoodsReceipt) Post(number string, journalID uuid.UUID, at time.Time) error {
	if err := r.transition(StatusPosted); err != nil {
		return err
	}
	r.Number, r.JournalEntryID, r.PostedAt = number, &journalID, &at
	return nil
}

// HasBillingOrReturn memeriksa apakah ada baris yang sudah ditagih atau diretur.
func (r GoodsReceipt) HasBillingOrReturn() bool {
	for _, l := range r.Lines {
		if l.BilledQty.IsPositive() || l.ReturnedQty.IsPositive() {
			return true
		}
	}
	return false
}

// Cancel membatalkan draft, atau GR Diposting yang belum ditagih dan belum diretur (alasan wajib). Mutasi stok
// dan jurnal dibalik service di transaksi yang sama.
func (r *GoodsReceipt) Cancel(reason string, at time.Time) error {
	reason = strings.TrimSpace(reason)
	if r.Status == StatusPosted {
		var err error
		if reason, err = requireReason(reason, "reason", "pembatalan"); err != nil {
			return err
		}
		if r.HasBillingOrReturn() {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Penerimaan Barang sudah ditagih pemasok atau diretur sehingga tidak bisa dibatalkan.")
		}
	}
	if len([]rune(reason)) > MaxReasonLength {
		return apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	if err := r.transition(StatusCancelled); err != nil {
		return err
	}
	r.CancelReason, r.CancelledAt = reason, &at
	return nil
}

// Actions mengembalikan aksi yang dimungkinkan status GR (belum memperhitungkan permission).
func (r GoodsReceipt) Actions() []Action {
	switch r.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionPost, ActionCancel}
	case StatusPosted:
		if !r.HasBillingOrReturn() {
			return []Action{ActionCancel}
		}
	default:
	}
	return nil
}
