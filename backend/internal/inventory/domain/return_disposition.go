package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
)

// Disposition adalah keputusan barang retur (BRD 5.11, U-09). Kode sama dengan usulan disposisi master
// Alasan Retur (modul sales).
type Disposition string

const (
	DispositionRestock   Disposition = "restock"
	DispositionRepack    Disposition = "repack"
	DispositionReprocess Disposition = "reprocess"
	DispositionSample    Disposition = "marketing_sample"
	DispositionDispose   Disposition = "dispose"
)

var dispositionLabels = map[Disposition]string{
	DispositionRestock: "Kembali ke stok", DispositionRepack: "Repacking", DispositionReprocess: "Reprocess maklon",
	DispositionSample: "Sampel/marketing", DispositionDispose: "Pemusnahan",
}

// Valid memeriksa disposisi.
func (d Disposition) Valid() bool {
	_, ok := dispositionLabels[d]
	return ok
}

// Label adalah label disposisi.
func (d Disposition) Label() string { return dispositionLabels[d] }

// Jenis dokumen lanjutan eksekusi disposisi.
const (
	TargetRepacking     = "repacking"
	TargetDisposal      = "disposal"
	TargetInternalUsage = "internal_usage"
)

// ReturnDisposition adalah dokumen Disposisi Retur (Desain Status Dokumen 3.5).
type ReturnDisposition struct {
	ID            uuid.UUID
	Number        string
	LocationID    uuid.UUID
	InboundID     *uuid.UUID
	BusinessDate  clock.BusinessDate
	Note          string
	Status        DocStatus
	DisposalValue decimal.Decimal
	CostAmount    decimal.Decimal
	JournalID     *uuid.UUID
	CreatedBy     Actor
	Submitted     *Step
	Posted        *Step
	Lines         []DispositionLine
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// DispositionLine adalah satu barang retur beserta hasil QC dan disposisinya.
type DispositionLine struct {
	ID            uuid.UUID
	LineNo        int
	ProductID     uuid.UUID
	BatchID       *uuid.UUID
	StockStatus   StockStatus
	Qty           decimal.Decimal
	ReasonID      *uuid.UUID
	Disposition   Disposition
	QCNote        string
	EstimatedCost decimal.Decimal
	CostAmount    decimal.Decimal
	TargetType    string
	TargetID      *uuid.UUID
}

// DispositionLineInput adalah isian baris. Disposition kosong = usulan default alasan retur (diisi service).
type DispositionLineInput struct {
	ProductID   uuid.UUID
	BatchID     *uuid.UUID
	StockStatus StockStatus
	Qty         decimal.Decimal
	ReasonID    *uuid.UUID
	Disposition Disposition
	QCNote      string
}

// ReturnDispositionInput adalah isian form Disposisi Retur.
type ReturnDispositionInput struct {
	LocationID   uuid.UUID
	InboundID    *uuid.UUID
	BusinessDate clock.BusinessDate
	Note         string
	Lines        []DispositionLineInput
}

// NewReturnDisposition memvalidasi dan membuat draft.
func NewReturnDisposition(in ReturnDispositionInput, by Actor) (ReturnDisposition, error) {
	r := ReturnDisposition{Status: DocDraft, CreatedBy: by, DisposalValue: decimal.Zero, CostAmount: decimal.Zero}
	if err := r.apply(in); err != nil {
		return ReturnDisposition{}, err
	}
	return r, nil
}

// Update mengganti isian draft.
func (r *ReturnDisposition) Update(in ReturnDispositionInput) error {
	if r.Status != DocDraft {
		return errTransition(ActionEdit, r.Status)
	}
	return r.apply(in)
}

func (r *ReturnDisposition) apply(in ReturnDispositionInput) error {
	fields := map[string]string{}
	if in.LocationID == uuid.Nil {
		fields["locationId"] = "Lokasi Gudang Retur wajib diisi"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	note := checkNote(fields, "note", in.Note)
	checkLineCount(fields, len(in.Lines))
	lines := make([]DispositionLine, len(in.Lines))
	for i, l := range in.Lines {
		if l.StockStatus == "" {
			l.StockStatus = StatusAvailable
		}
		if l.ProductID == uuid.Nil {
			fields[docLineField(i, "productId")] = "Produk wajib diisi"
		}
		if l.StockStatus != StatusAvailable && l.StockStatus != StatusQuarantine {
			fields[docLineField(i, "stockStatus")] = "Status stok harus Tersedia atau Karantina"
		}
		if !l.Disposition.Valid() {
			fields[docLineField(i, "disposition")] = "Pilih disposisi atau alasan retur yang punya usulan disposisi"
		}
		checkQty(fields, docLineField(i, "qty"), l.Qty)
		lines[i] = DispositionLine{
			LineNo: i + 1, ProductID: l.ProductID, BatchID: l.BatchID, StockStatus: l.StockStatus, Qty: l.Qty,
			ReasonID: l.ReasonID, Disposition: l.Disposition, QCNote: checkNote(fields, docLineField(i, "qcNote"), l.QCNote),
			EstimatedCost: decimal.Zero, CostAmount: decimal.Zero,
		}
	}
	if err := validationOrNil(fields); err != nil {
		return err
	}
	r.LocationID, r.InboundID, r.BusinessDate, r.Note, r.Lines = in.LocationID, in.InboundID, in.BusinessDate, note, lines
	return nil
}

// HasDisposal memeriksa baris pemusnahan (syarat level 2–3 approval `has_disposal`).
func (r ReturnDisposition) HasDisposal() bool {
	for _, l := range r.Lines {
		if l.Disposition == DispositionDispose {
			return true
		}
	}
	return false
}

// ComputeDisposalValue menjumlahkan estimasi HPP baris pemusnahan (nominal approval).
func (r *ReturnDisposition) ComputeDisposalValue() decimal.Decimal {
	total := decimal.Zero
	for _, l := range r.Lines {
		if l.Disposition == DispositionDispose {
			total = total.Add(l.EstimatedCost)
		}
	}
	r.DisposalValue = total
	return total
}

var dispositionTransitions = []transition{
	{ActionSubmit, []DocStatus{DocDraft}, DocPendingApproval},
	{ActionApprove, []DocStatus{DocPendingApproval}, DocApproved},
	{ActionReject, []DocStatus{DocPendingApproval}, DocRejected},
	{ActionRevise, []DocStatus{DocRejected}, DocDraft},
	{ActionExecute, []DocStatus{DocApproved}, DocPosted},
	{ActionCancel, []DocStatus{DocDraft}, DocCancelled},
}

// Transition menjalankan aksi disposisi.
func (r *ReturnDisposition) Transition(a DocAction) error {
	to, ok := findTransition(dispositionTransitions, a, r.Status)
	if !ok {
		return errTransition(a, r.Status)
	}
	r.Status = to
	return nil
}

// Permission Disposisi Retur (Desain Role & Approval 3.2): Staf Gudang/Logistik Leader RWS; eksekusi memakai W
// karena matriks tidak memberi P.
const (
	PermDispositionRead   = "warehouse.return_disposition.read"
	PermDispositionWrite  = "warehouse.return_disposition.write"
	PermDispositionSubmit = "warehouse.return_disposition.submit"
)

// AllowedActions mengembalikan aksi yang bisa dilakukan pemegang permission.
func (r ReturnDisposition) AllowedActions(has func(string) bool) []DocAction {
	var out []DocAction
	if r.Status == DocDraft && has(PermDispositionWrite) {
		out = append(out, ActionEdit)
	}
	perms := map[DocAction]string{
		ActionSubmit: PermDispositionSubmit, ActionRevise: PermDispositionWrite, ActionExecute: PermDispositionWrite,
		ActionCancel: PermDispositionWrite,
	}
	for _, a := range []DocAction{ActionSubmit, ActionRevise, ActionExecute, ActionCancel} {
		if _, ok := findTransition(dispositionTransitions, a, r.Status); ok && has(perms[a]) {
			out = append(out, a)
		}
	}
	return out
}

// DispositionAmounts adalah nilai A-26 dari eksekusi restock/reprocess langsung di dokumen disposisi.
type DispositionAmounts struct {
	Restock   decimal.Decimal
	Reprocess decimal.Decimal
}

// Returned adalah nilai kredit Persediaan Retur (1314).
func (a DispositionAmounts) Returned() decimal.Decimal { return a.Restock.Add(a.Reprocess) }
