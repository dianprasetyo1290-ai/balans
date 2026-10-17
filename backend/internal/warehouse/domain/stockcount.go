package domain

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// CountType adalah jenis Stock Count.
type CountType string

const (
	CountFull        CountType = "full"
	CountCycle       CountType = "cycle"
	CountConsignment CountType = "consignment"
	CountVending     CountType = "vending"
)

// Valid memeriksa jenis stock count.
func (t CountType) Valid() bool {
	switch t {
	case CountFull, CountCycle, CountConsignment, CountVending:
		return true
	}
	return false
}

// Adjusts: stock count gudang membentuk penyesuaian stok (A-14); konsinyasi/vending hanya dipakai rekonsiliasi.
func (t CountType) Adjusts() bool { return t == CountFull || t == CountCycle }

// LocationTypes adalah jenis lokasi yang boleh dihitung dengan jenis stock count ini.
func (t CountType) LocationTypes() []LocationType {
	switch t {
	case CountConsignment:
		return []LocationType{LocationConsignment}
	case CountVending:
		return []LocationType{LocationVendingBuffer, LocationVendingMachine}
	}
	return []LocationType{
		LocationInbound, LocationFinishedGoods, LocationEcommerce, LocationPettyStock, LocationRawMaterial,
		LocationGoodsMaterial, LocationReturns, LocationEvent, LocationTollManufacturer,
	}
}

// stockCountTransitions: Desain Status Dokumen 3.3. approved → posted dijalankan langsung oleh handler approval
// di transaksi keputusan, sehingga status akhir dokumen yang disetujui adalah posted.
var stockCountTransitions = transitionTable{
	{StatusDraft, ActionStart, StatusInProgress},
	{StatusInProgress, ActionCount, StatusInProgress},
	{StatusInProgress, ActionSubmit, StatusPendingApproval},
	{StatusPendingApproval, ActionApprove, StatusPosted},
	{StatusPendingApproval, ActionReject, StatusRejected},
	{StatusRejected, ActionRecount, StatusInProgress},
	{StatusDraft, ActionCancel, StatusCancelled},
	{StatusInProgress, ActionCancel, StatusCancelled},
}

// StockCount adalah dokumen Stock Count (SC) satu lokasi.
type StockCount struct {
	ID              uuid.UUID
	Number          string
	CountType       CountType
	LocationID      uuid.UUID
	ScheduleID      *uuid.UUID
	BusinessDate    clock.BusinessDate
	Status          Status
	AssignedTo      *uuid.UUID
	AssignedToName  string
	Note            string
	PhotoFileIDs    []uuid.UUID
	SnapshotAt      *time.Time
	StartedBy       *uuid.UUID
	StartedByName   string
	SubmittedBy     *uuid.UUID
	SubmittedByName string
	SubmittedAt     *time.Time
	VarianceAmount  decimal.Decimal
	NetVariance     decimal.Decimal
	DecidedAt       *time.Time
	DecisionNote    string
	PostedAt        *time.Time
	JournalEntryID  *uuid.UUID
	CreatedAt       time.Time
	CreatedBy       uuid.UUID
	UpdatedAt       time.Time
	Version         int
	Lines           []CountLine
}

// CountLine adalah satu baris hitung per produk × batch.
type CountLine struct {
	ID             uuid.UUID
	LineNo         int
	ProductID      uuid.UUID
	BatchID        *uuid.UUID
	SystemQty      decimal.Decimal
	UnitCost       decimal.Decimal
	CountedQty     *decimal.Decimal
	VarianceQty    decimal.Decimal
	VarianceAmount decimal.Decimal
	IsExtra        bool
	PhotoFileID    *uuid.UUID
	Note           string
	CountedBy      *uuid.UUID
	CountedAt      *time.Time
}

// StockCountInput adalah isian stock count baru.
type StockCountInput struct {
	CountType      CountType
	LocationID     uuid.UUID
	BusinessDate   clock.BusinessDate
	ScheduleID     *uuid.UUID
	AssignedTo     *uuid.UUID
	AssignedToName string
	Note           string
}

// NewStockCount memvalidasi stock count draft. Kecocokan jenis lokasi diperiksa service (CountType.LocationTypes).
func NewStockCount(in StockCountInput) (StockCount, error) {
	fields := map[string]string{}
	if !in.CountType.Valid() {
		fields["countType"] = "Jenis stock count tidak dikenal"
	}
	if in.LocationID == uuid.Nil {
		fields["locationIds"] = "Lokasi wajib dipilih"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	note := checkNote(in.Note, "note", fields)
	if len(fields) > 0 {
		return StockCount{}, apperr.Validation(fields)
	}
	return StockCount{
		CountType: in.CountType, LocationID: in.LocationID, BusinessDate: in.BusinessDate, ScheduleID: in.ScheduleID,
		AssignedTo: in.AssignedTo, AssignedToName: in.AssignedToName, Note: note, Status: StatusDraft,
		VarianceAmount: decimal.Zero, NetVariance: decimal.Zero,
	}, nil
}

// Transition menjalankan aksi status.
func (c *StockCount) Transition(action string) error {
	to, err := stockCountTransitions.next("stock_count", c.Status, action)
	if err != nil {
		return err
	}
	c.Status = to
	return nil
}

// Can memeriksa apakah aksi status diizinkan.
func (c StockCount) Can(action string) bool { return stockCountTransitions.allows(c.Status, action) }

// RevealsSystemQty: hitung buta — qty sistem dan selisih hanya terlihat setelah hasil dikirim.
func (c StockCount) RevealsSystemQty() bool {
	switch c.Status {
	case StatusPendingApproval, StatusPosted, StatusCompleted:
		return true
	}
	return false
}

// SnapshotRow adalah saldo sistem satu produk × batch di lokasi saat mulai.
type SnapshotRow struct {
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
	UnitCost  decimal.Decimal
}

// Start mencatat snapshot saldo sistem dan mengubah status menjadi in_progress.
func (c *StockCount) Start(rows []SnapshotRow, by uuid.UUID, byName string, at time.Time) error {
	if err := c.Transition(ActionStart); err != nil {
		return err
	}
	c.Lines = c.Lines[:0]
	for i, r := range rows {
		c.Lines = append(c.Lines, CountLine{
			LineNo: i + 1, ProductID: r.ProductID, BatchID: r.BatchID, SystemQty: r.Qty, UnitCost: r.UnitCost,
			VarianceQty: decimal.Zero, VarianceAmount: decimal.Zero,
		})
	}
	c.SnapshotAt, c.StartedBy, c.StartedByName = &at, &by, byName
	return nil
}

// CountEntry adalah isian hitung satu baris yang sudah ada.
type CountEntry struct {
	LineID      uuid.UUID
	CountedQty  *decimal.Decimal
	Note        *string
	PhotoFileID *uuid.UUID
}

// ExtraEntry adalah barang yang ditemukan tetapi tidak ada di snapshot.
type ExtraEntry struct {
	ProductID  uuid.UUID
	BatchID    *uuid.UUID
	CountedQty decimal.Decimal
	UnitCost   decimal.Decimal
	Note       string
}

// MaxCountPhotos membatasi foto per stock count.
const MaxCountPhotos = 10

// RecordCounts mencatat isian hitung (autosave form HP). Baris ekstra untuk produk × batch yang sudah ada
// digabung ke baris itu. Mengembalikan indeks baris yang berubah.
func (c *StockCount) RecordCounts(entries []CountEntry, extras []ExtraEntry, by uuid.UUID, at time.Time) ([]int, error) {
	if !c.Can(ActionCount) {
		return nil, apperr.InvalidStateTransition("stock_count", string(c.Status), ActionCount)
	}
	fields := map[string]string{}
	changed := map[int]bool{}
	for i, e := range entries {
		idx := c.lineIndex(e.LineID)
		if idx < 0 {
			fields[lineField(i, "lineId")] = "Baris tidak ditemukan"
			continue
		}
		l := &c.Lines[idx]
		if e.CountedQty != nil {
			if !validQtyOrZero(*e.CountedQty) {
				fields[lineField(i, "countedQty")] = "Qty hitung tidak valid"
				continue
			}
			q := *e.CountedQty
			l.CountedQty = &q
		}
		if e.Note != nil {
			l.Note = checkNote(*e.Note, lineField(i, "note"), fields)
		}
		if e.PhotoFileID != nil {
			id := *e.PhotoFileID
			l.PhotoFileID = &id
			if id == uuid.Nil {
				l.PhotoFileID = nil
			}
		}
		l.CountedBy, l.CountedAt = &by, &at
		changed[idx] = true
	}
	for i, x := range extras {
		f := func(name string) string { return "extraLines[" + strconv.Itoa(i) + "]." + name }
		if x.ProductID == uuid.Nil {
			fields[f("productId")] = "Produk wajib diisi"
			continue
		}
		if !validQtyOrZero(x.CountedQty) {
			fields[f("countedQty")] = "Qty hitung tidak valid"
			continue
		}
		q := x.CountedQty
		note := checkNote(x.Note, f("note"), fields)
		if idx := c.lineFor(x.ProductID, x.BatchID); idx >= 0 {
			c.Lines[idx].CountedQty, c.Lines[idx].CountedBy, c.Lines[idx].CountedAt = &q, &by, &at
			changed[idx] = true
			continue
		}
		if len(c.Lines) >= MaxDocumentLines*5 {
			fields[f("productId")] = "Terlalu banyak baris"
			continue
		}
		c.Lines = append(c.Lines, CountLine{
			LineNo: len(c.Lines) + 1, ProductID: x.ProductID, BatchID: x.BatchID, SystemQty: decimal.Zero,
			UnitCost: x.UnitCost, CountedQty: &q, IsExtra: true, Note: note, CountedBy: &by, CountedAt: &at,
			VarianceQty: decimal.Zero, VarianceAmount: decimal.Zero,
		})
		changed[len(c.Lines)-1] = true
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	out := make([]int, 0, len(changed))
	for i := range c.Lines {
		if changed[i] {
			out = append(out, i)
		}
	}
	return out, nil
}

// SetPhotos mengganti foto stock count (mis. foto rak).
func (c *StockCount) SetPhotos(ids []uuid.UUID) error {
	if !c.Can(ActionCount) {
		return apperr.InvalidStateTransition("stock_count", string(c.Status), ActionCount)
	}
	if len(ids) > MaxCountPhotos {
		return apperr.Validation(map[string]string{"photoFileIds": "Maksimal 10 foto"})
	}
	c.PhotoFileIDs = ids
	return nil
}

func (c StockCount) lineIndex(id uuid.UUID) int {
	for i, l := range c.Lines {
		if l.ID == id && id != uuid.Nil {
			return i
		}
	}
	return -1
}

func (c StockCount) lineFor(productID uuid.UUID, batchID *uuid.UUID) int {
	for i, l := range c.Lines {
		if l.ProductID == productID && sameBatch(l.BatchID, batchID) {
			return i
		}
	}
	return -1
}

func sameBatch(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Submit memeriksa semua baris sudah dihitung, menghitung selisih (qty dan nilai HPP snapshot), lalu
// menentukan status: ada selisih pada stock count gudang → pending_approval; selain itu completed. Stock count
// lokasi mitra/mesin wajib foto.
func (c *StockCount) Submit(by uuid.UUID, byName string, at time.Time) error {
	if !c.Can(ActionSubmit) {
		return apperr.InvalidStateTransition("stock_count", string(c.Status), ActionSubmit)
	}
	fields := map[string]string{}
	for i, l := range c.Lines {
		if l.CountedQty == nil {
			fields[lineField(i, "countedQty")] = "Qty hitung belum diisi"
		}
	}
	if len(c.Lines) == 0 {
		fields["lines"] = "Belum ada baris yang dihitung"
	}
	if !c.CountType.Adjusts() && len(c.PhotoFileIDs) == 0 {
		fields["photoFileIds"] = "Foto lokasi wajib dilampirkan untuk stock count mitra/mesin"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	abs, net := decimal.Zero, decimal.Zero
	hasVariance := false
	for i := range c.Lines {
		l := &c.Lines[i]
		l.VarianceQty = l.CountedQty.Sub(l.SystemQty)
		l.VarianceAmount = l.VarianceQty.Mul(l.UnitCost).Round(2)
		if !l.VarianceQty.IsZero() {
			hasVariance = true
		}
		abs = abs.Add(l.VarianceAmount.Abs())
		net = net.Add(l.VarianceAmount)
	}
	c.VarianceAmount, c.NetVariance = abs, net
	c.SubmittedBy, c.SubmittedByName, c.SubmittedAt = &by, byName, &at
	if hasVariance && c.CountType.Adjusts() {
		c.Status = StatusPendingApproval
		return nil
	}
	c.Status = StatusCompleted
	return nil
}

// HasVariance memeriksa apakah ada baris dengan selisih qty.
func (c StockCount) HasVariance() bool {
	for _, l := range c.Lines {
		if !l.VarianceQty.IsZero() {
			return true
		}
	}
	return false
}

// Decide mencatat keputusan approval.
func (c *StockCount) Decide(approved bool, note string, at time.Time) error {
	action := ActionReject
	if approved {
		action = ActionApprove
	}
	if err := c.Transition(action); err != nil {
		return err
	}
	c.DecidedAt, c.DecisionNote = &at, note
	if approved {
		c.PostedAt = &at
	}
	return nil
}

// Recount membuka kembali stock count yang ditolak untuk dihitung ulang (snapshot tetap).
func (c *StockCount) Recount() error {
	if err := c.Transition(ActionRecount); err != nil {
		return err
	}
	c.SubmittedBy, c.SubmittedByName, c.SubmittedAt = nil, "", nil
	c.VarianceAmount, c.NetVariance = decimal.Zero, decimal.Zero
	for i := range c.Lines {
		c.Lines[i].VarianceQty, c.Lines[i].VarianceAmount = decimal.Zero, decimal.Zero
	}
	return nil
}
