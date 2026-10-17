package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// TransferType adalah jenis Transfer Stok.
type TransferType string

const (
	// TransferDirect adalah transfer oleh Logistik antar lokasi/gudang/eksternal.
	TransferDirect TransferType = "direct"
	// TransferRequest adalah Permintaan Petty Stock oleh Fulfillment Leader (FG Non-Ecommerce → Petty Stock).
	TransferRequest TransferType = "request"
)

// Valid memeriksa jenis transfer.
func (t TransferType) Valid() bool { return t == TransferDirect || t == TransferRequest }

// directTransitions: Desain Status Dokumen 3.2 (transfer langsung).
var directTransitions = transitionTable{
	{StatusDraft, ActionEdit, StatusDraft},
	{StatusDraft, ActionPost, StatusCompleted},
	{StatusDraft, ActionShip, StatusInTransit},
	{StatusInTransit, ActionReceive, StatusReceived},
	{StatusPartiallyReceived, ActionReceive, StatusReceived},
	{StatusPartiallyReceived, ActionComplete, StatusCompleted},
	{StatusReceived, ActionComplete, StatusCompleted},
	{StatusDraft, ActionCancel, StatusCancelled},
}

// requestTransitions: Permintaan Petty Stock (Desain Status Dokumen 3.2).
var requestTransitions = transitionTable{
	{StatusDraft, ActionEdit, StatusDraft},
	{StatusDraft, ActionSubmit, StatusSubmitted},
	{StatusSubmitted, ActionRelease, StatusInProgress},
	{StatusInProgress, ActionPrepare, StatusInProgress},
	{StatusInProgress, ActionReceive, StatusCompleted},
	{StatusDraft, ActionCancel, StatusCancelled},
	{StatusSubmitted, ActionCancel, StatusCancelled},
}

// StockTransfer adalah dokumen Transfer Stok (TRF) atau Permintaan Petty Stock.
type StockTransfer struct {
	ID               uuid.UUID
	Number           string
	Type             TransferType
	FromLocationID   uuid.UUID
	ToLocationID     uuid.UUID
	RequiresShipping bool
	BusinessDate     clock.BusinessDate
	Status           Status
	SourceType       string
	SourceID         *uuid.UUID
	Note             string
	RequestedBy      uuid.UUID
	RequestedByName  string
	SubmittedAt      *time.Time
	ReleasedBy       *uuid.UUID
	ReleasedByName   string
	ReleasedAt       *time.Time
	PreparedBy       *uuid.UUID
	PreparedByName   string
	PreparedAt       *time.Time
	ShippedBy        *uuid.UUID
	ShippedByName    string
	ShippedAt        *time.Time
	ReceivedBy       *uuid.UUID
	ReceivedByName   string
	ReceivedAt       *time.Time
	CompletedAt      *time.Time
	CancelledAt      *time.Time
	CancelReason     string
	CreatedAt        time.Time
	CreatedBy        uuid.UUID
	UpdatedAt        time.Time
	Version          int
	Lines            []TransferLine
}

// TransferLine adalah satu baris transfer. Setelah dikirim, satu baris = satu batch.
type TransferLine struct {
	ID          uuid.UUID
	LineNo      int
	ProductID   uuid.UUID
	BatchID     *uuid.UUID
	Qty         decimal.Decimal
	ReceivedQty decimal.Decimal
	ShortageQty decimal.Decimal
	// CostAmount adalah nilai FIFO barang yang dipindah (untuk jurnal A-07/A-24 oleh modul pemanggil).
	CostAmount  decimal.Decimal
	BatchReason string
	Note        string
}

// Outstanding adalah qty yang belum diterima dan belum dicatat sebagai kekurangan.
func (l TransferLine) Outstanding() decimal.Decimal {
	return l.Qty.Sub(l.ReceivedQty).Sub(l.ShortageQty)
}

// TransferLineInput adalah isian baris.
type TransferLineInput struct {
	ProductID uuid.UUID
	// BatchID nil = dialokasikan FEFO saat posting/kirim/release.
	BatchID     *uuid.UUID
	Qty         decimal.Decimal
	BatchReason string
	Note        string
}

// TransferInput adalah isian transfer.
type TransferInput struct {
	Type           TransferType
	FromLocationID uuid.UUID
	ToLocationID   uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	SourceType     string
	SourceID       *uuid.UUID
	Lines          []TransferLineInput
}

func (in TransferInput) validate() (StockTransfer, error) {
	fields := map[string]string{}
	if !in.Type.Valid() {
		fields["transferType"] = "Jenis transfer tidak dikenal"
	}
	if in.FromLocationID == uuid.Nil {
		fields["fromLocationId"] = "Lokasi asal wajib diisi"
	}
	if in.ToLocationID == uuid.Nil {
		fields["toLocationId"] = "Lokasi tujuan wajib diisi"
	}
	if in.FromLocationID != uuid.Nil && in.FromLocationID == in.ToLocationID {
		fields["toLocationId"] = "Lokasi tujuan harus berbeda dari lokasi asal"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	if in.SourceType != "" && (!ValidSourceType(in.SourceType) || in.SourceID == nil || *in.SourceID == uuid.Nil) {
		fields["source"] = "Dokumen sumber tidak valid"
	}
	note := checkNote(in.Note, "note", fields)
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
	case len(in.Lines) > MaxDocumentLines:
		fields["lines"] = "Terlalu banyak baris"
	}
	t := StockTransfer{
		Type: in.Type, FromLocationID: in.FromLocationID, ToLocationID: in.ToLocationID, BusinessDate: in.BusinessDate,
		Note: note, SourceType: in.SourceType, SourceID: in.SourceID,
	}
	for i, l := range in.Lines {
		if l.ProductID == uuid.Nil {
			fields[lineField(i, "productId")] = "Produk wajib diisi"
		}
		if !validQty(l.Qty) {
			fields[lineField(i, "qty")] = "Qty harus lebih dari 0 (maks. 3 desimal)"
		}
		reason := checkNote(l.BatchReason, lineField(i, "batchReason"), fields)
		lnote := checkNote(l.Note, lineField(i, "note"), fields)
		t.Lines = append(t.Lines, TransferLine{
			LineNo: i + 1, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty, BatchReason: reason, Note: lnote,
			ReceivedQty: decimal.Zero, ShortageQty: decimal.Zero, CostAmount: decimal.Zero,
		})
	}
	if len(fields) > 0 {
		return StockTransfer{}, apperr.Validation(fields)
	}
	return t, nil
}

// NewTransfer memvalidasi transfer baru berstatus draft. Jenis lokasi (Petty Stock, aktif) diperiksa service.
func NewTransfer(in TransferInput, by uuid.UUID, byName string) (StockTransfer, error) {
	t, err := in.validate()
	if err != nil {
		return StockTransfer{}, err
	}
	t.Status, t.RequestedBy, t.RequestedByName = StatusDraft, by, byName
	return t, nil
}

// UpdateDraft mengganti isian transfer draft. Jenis dan dokumen sumber tidak berubah.
func (t *StockTransfer) UpdateDraft(in TransferInput) error {
	if _, err := t.next(ActionEdit); err != nil {
		return err
	}
	in.Type, in.SourceType, in.SourceID = t.Type, t.SourceType, t.SourceID
	n, err := in.validate()
	if err != nil {
		return err
	}
	t.FromLocationID, t.ToLocationID, t.BusinessDate, t.Note, t.Lines = n.FromLocationID, n.ToLocationID, n.BusinessDate, n.Note, n.Lines
	return nil
}

func (t StockTransfer) table() transitionTable {
	if t.Type == TransferRequest {
		return requestTransitions
	}
	return directTransitions
}

func (t StockTransfer) next(action string) (Status, error) {
	return t.table().next("stock_transfer", t.Status, action)
}

// Transition menjalankan aksi status (tanpa efek stok). Aksi post/ship disesuaikan dengan RequiresShipping.
func (t *StockTransfer) Transition(action string) error {
	if t.Type == TransferDirect {
		if action == ActionPost && t.RequiresShipping {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Transfer antar gudang atau ke lokasi eksternal harus dikirim (Dalam Pengiriman), bukan langsung diposting")
		}
		if action == ActionShip && !t.RequiresShipping {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Transfer dalam satu gudang langsung diposting tanpa status Dalam Pengiriman")
		}
	}
	if t.Type == TransferRequest && action == ActionReceive && t.PreparedAt == nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Barang belum disiapkan Staf Gudang")
	}
	if action == ActionPrepare && t.PreparedAt != nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Barang sudah disiapkan")
	}
	to, err := t.next(action)
	if err != nil {
		return err
	}
	t.Status = to
	return nil
}

// Can memeriksa apakah aksi status diizinkan (tanpa cek permission).
func (t StockTransfer) Can(action string) bool {
	probe := t
	probe.Lines = nil
	return probe.Transition(action) == nil
}

// ReceiveLine adalah qty yang diterima untuk satu baris.
type ReceiveLine struct {
	LineID uuid.UUID
	Qty    decimal.Decimal
}

// ApplyReceive mencatat qty diterima (transfer langsung yang Dalam Pengiriman). Qty tidak boleh melebihi
// sisa; baris yang tidak disebut dianggap 0. Status menjadi received bila semua baris lengkap, selain itu
// partially_received. Mengembalikan qty per baris yang dipindah ke Tersedia.
func (t *StockTransfer) ApplyReceive(lines []ReceiveLine) (map[uuid.UUID]decimal.Decimal, error) {
	if t.Type != TransferDirect || (t.Status != StatusInTransit && t.Status != StatusPartiallyReceived) {
		return nil, apperr.InvalidStateTransition("stock_transfer", string(t.Status), ActionReceive)
	}
	fields := map[string]string{}
	got := map[uuid.UUID]decimal.Decimal{}
	for i, r := range lines {
		idx := t.lineIndex(r.LineID)
		switch {
		case idx < 0:
			fields[lineField(i, "lineId")] = "Baris tidak ditemukan"
		case !validQtyOrZero(r.Qty):
			fields[lineField(i, "qty")] = "Qty tidak valid"
		case r.Qty.Add(got[r.LineID]).GreaterThan(t.Lines[idx].Outstanding()):
			fields[lineField(i, "qty")] = "Qty diterima melebihi sisa kiriman (" + t.Lines[idx].Outstanding().String() + ")"
		default:
			got[r.LineID] = got[r.LineID].Add(r.Qty)
		}
	}
	total := decimal.Zero
	for _, q := range got {
		total = total.Add(q)
	}
	if len(fields) == 0 && !total.IsPositive() {
		fields["lines"] = "Isi qty diterima minimal satu baris"
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	complete := true
	for i := range t.Lines {
		t.Lines[i].ReceivedQty = t.Lines[i].ReceivedQty.Add(got[t.Lines[i].ID])
		if t.Lines[i].Outstanding().IsPositive() {
			complete = false
		}
	}
	t.Status = StatusReceived
	if !complete {
		t.Status = StatusPartiallyReceived
	}
	return got, nil
}

// CloseShortage menyelesaikan transfer yang diterima sebagian: sisa yang tidak datang dicatat sebagai
// kekurangan per baris (diproses lewat Stock Count lokasi tujuan). Mengembalikan kekurangan per baris.
func (t *StockTransfer) CloseShortage() (map[uuid.UUID]decimal.Decimal, error) {
	if err := t.Transition(ActionComplete); err != nil {
		return nil, err
	}
	out := map[uuid.UUID]decimal.Decimal{}
	for i := range t.Lines {
		if o := t.Lines[i].Outstanding(); o.IsPositive() {
			t.Lines[i].ShortageQty = t.Lines[i].ShortageQty.Add(o)
			out[t.Lines[i].ID] = o
		}
	}
	return out, nil
}

// ConfirmRequest mencatat qty yang diterima Fulfillment Leader untuk Permintaan Petty Stock. Qty tidak boleh
// melebihi qty permintaan; selisih tetap di lokasi asal (reservasinya dilepas).
func (t *StockTransfer) ConfirmRequest(lines []ReceiveLine) (map[uuid.UUID]decimal.Decimal, error) {
	if t.Type != TransferRequest {
		return nil, apperr.InvalidStateTransition("stock_transfer", string(t.Status), ActionReceive)
	}
	if err := t.Transition(ActionReceive); err != nil {
		return nil, err
	}
	fields := map[string]string{}
	got := map[uuid.UUID]decimal.Decimal{}
	for i, r := range lines {
		idx := t.lineIndex(r.LineID)
		switch {
		case idx < 0:
			fields[lineField(i, "lineId")] = "Baris tidak ditemukan"
		case !validQtyOrZero(r.Qty):
			fields[lineField(i, "qty")] = "Qty tidak valid"
		case r.Qty.Add(got[r.LineID]).GreaterThan(t.Lines[idx].Qty):
			fields[lineField(i, "qty")] = "Qty diterima melebihi qty permintaan (" + t.Lines[idx].Qty.String() + ")"
		default:
			got[r.LineID] = got[r.LineID].Add(r.Qty)
		}
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	for i := range t.Lines {
		t.Lines[i].ReceivedQty = got[t.Lines[i].ID]
		t.Lines[i].ShortageQty = t.Lines[i].Qty.Sub(t.Lines[i].ReceivedQty)
	}
	return got, nil
}

func (t StockTransfer) lineIndex(id uuid.UUID) int {
	for i, l := range t.Lines {
		if l.ID == id {
			return i
		}
	}
	return -1
}

// Cancel membatalkan transfer draft atau permintaan yang belum di-release.
func (t *StockTransfer) Cancel(reason string, at time.Time) error {
	fields := map[string]string{}
	reason = checkNote(reason, "reason", fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	if err := t.Transition(ActionCancel); err != nil {
		return err
	}
	t.CancelReason, t.CancelledAt = reason, &at
	return nil
}
