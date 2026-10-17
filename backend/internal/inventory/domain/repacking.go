package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
)

// RepackDirection adalah arah baris repacking.
type RepackDirection string

const (
	// RepackInput: barang retur (dari Gudang Retur) atau packaging baru (dari RM) yang dipakai.
	RepackInput RepackDirection = "input"
	// RepackOutput: FG hasil repacking dengan batch baru.
	RepackOutput RepackDirection = "output"
)

// Repacking adalah dokumen Repacking (INV-06; Desain Status Dokumen 4.2): draft → in_progress → posted.
type Repacking struct {
	ID                  uuid.UUID
	Number              string
	BusinessDate        clock.BusinessDate
	Note                string
	Status              DocStatus
	CostAmount          decimal.Decimal
	JournalID           *uuid.UUID
	ReturnDispositionID *uuid.UUID
	CreatedBy           Actor
	Started             *Step
	Posted              *Step
	Lines               []RepackLine
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int
}

// RepackLine adalah satu baris input/output.
type RepackLine struct {
	ID          uuid.UUID
	LineNo      int
	Direction   RepackDirection
	ProductID   uuid.UUID
	LocationID  uuid.UUID
	StockStatus StockStatus
	// BatchID: input = batch yang diambil (nil = FEFO); output = batch baru setelah posting.
	BatchID *uuid.UUID
	// Batch baru output.
	BatchNumber    string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
	Qty            decimal.Decimal
	CostAmount     decimal.Decimal
	Note           string
}

// RepackLineInput adalah isian baris.
type RepackLineInput struct {
	Direction      RepackDirection
	ProductID      uuid.UUID
	LocationID     uuid.UUID
	StockStatus    StockStatus
	BatchID        *uuid.UUID
	BatchNumber    string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
	Qty            decimal.Decimal
	Note           string
}

// RepackingInput adalah isian form Repacking.
type RepackingInput struct {
	BusinessDate clock.BusinessDate
	Note         string
	Lines        []RepackLineInput
}

// NewRepacking memvalidasi dan membuat draft repacking.
func NewRepacking(in RepackingInput, by Actor) (Repacking, error) {
	r := Repacking{Status: DocDraft, CreatedBy: by, CostAmount: decimal.Zero}
	if err := r.apply(in); err != nil {
		return Repacking{}, err
	}
	return r, nil
}

// Update mengganti isian draft.
func (r *Repacking) Update(in RepackingInput) error {
	if r.Status != DocDraft {
		return errTransition(ActionEdit, r.Status)
	}
	return r.apply(in)
}

func (r *Repacking) apply(in RepackingInput) error {
	fields := map[string]string{}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	note := checkNote(fields, "note", in.Note)
	checkLineCount(fields, len(in.Lines))
	inputs, outputs := 0, 0
	lines := make([]RepackLine, len(in.Lines))
	for i, l := range in.Lines {
		if l.StockStatus == "" {
			l.StockStatus = StatusAvailable
		}
		if l.ProductID == uuid.Nil {
			fields[docLineField(i, "productId")] = "Produk wajib diisi"
		}
		if l.LocationID == uuid.Nil {
			fields[docLineField(i, "locationId")] = "Lokasi wajib diisi"
		}
		if l.StockStatus != StatusAvailable && l.StockStatus != StatusQuarantine {
			fields[docLineField(i, "stockStatus")] = "Status stok harus Tersedia atau Karantina"
		}
		checkQty(fields, docLineField(i, "qty"), l.Qty)
		line := RepackLine{
			LineNo: i + 1, Direction: l.Direction, ProductID: l.ProductID, LocationID: l.LocationID,
			StockStatus: l.StockStatus, Qty: l.Qty, CostAmount: decimal.Zero, Note: checkNote(fields, docLineField(i, "note"), l.Note),
		}
		switch l.Direction {
		case RepackInput:
			inputs++
			line.BatchID = l.BatchID
		case RepackOutput:
			outputs++
			nb, msg := normalizeNewBatch(NewBatch{Number: l.BatchNumber, ProductionDate: l.ProductionDate, ExpiryDate: l.ExpiryDate})
			if msg != "" {
				fields[docLineField(i, "batchNumber")] = msg
			}
			if l.StockStatus != StatusAvailable {
				fields[docLineField(i, "stockStatus")] = "Hasil repacking masuk sebagai stok Tersedia"
			}
			line.BatchNumber, line.ProductionDate, line.ExpiryDate = nb.Number, nb.ProductionDate, nb.ExpiryDate
		default:
			fields[docLineField(i, "direction")] = "Arah baris tidak valid"
		}
		lines[i] = line
	}
	if len(in.Lines) > 0 {
		if inputs == 0 {
			fields["lines"] = "Minimal satu baris bahan (barang retur atau packaging)"
		}
		if outputs != 1 {
			fields["lines"] = "Repacking menghasilkan tepat satu baris FG hasil"
		}
	}
	if err := validationOrNil(fields); err != nil {
		return err
	}
	r.BusinessDate, r.Note, r.Lines = in.BusinessDate, note, lines
	return nil
}

var repackTransitions = []transition{
	{ActionStart, []DocStatus{DocDraft}, DocInProgress},
	{ActionExecute, []DocStatus{DocInProgress}, DocPosted},
	{ActionCancel, []DocStatus{DocDraft, DocInProgress}, DocCancelled},
}

// Transition menjalankan aksi repacking.
func (r *Repacking) Transition(a DocAction) error {
	to, ok := findTransition(repackTransitions, a, r.Status)
	if !ok {
		return errTransition(a, r.Status)
	}
	r.Status = to
	return nil
}

// Permission Repacking (Desain Role & Approval 3.2).
const (
	PermRepackRead  = "inventory.repacking.read"
	PermRepackWrite = "inventory.repacking.write"
	PermRepackPost  = "inventory.repacking.post"
)

// AllowedActions mengembalikan aksi yang bisa dilakukan pemegang permission.
func (r Repacking) AllowedActions(has func(string) bool) []DocAction {
	var out []DocAction
	if r.Status == DocDraft && has(PermRepackWrite) {
		out = append(out, ActionEdit)
	}
	for _, a := range []DocAction{ActionStart, ActionExecute, ActionCancel} {
		perm := PermRepackPost
		if a == ActionCancel {
			perm = PermRepackWrite
		}
		if _, ok := findTransition(repackTransitions, a, r.Status); ok && has(perm) {
			out = append(out, a)
		}
	}
	return out
}

// Inputs mengembalikan baris bahan.
func (r Repacking) Inputs() []RepackLine {
	var out []RepackLine
	for _, l := range r.Lines {
		if l.Direction == RepackInput {
			out = append(out, l)
		}
	}
	return out
}

// Output mengembalikan baris hasil (tepat satu setelah validasi).
func (r Repacking) Output() *RepackLine {
	for i := range r.Lines {
		if r.Lines[i].Direction == RepackOutput {
			return &r.Lines[i]
		}
	}
	return nil
}

// OutputUnitCost = (HPP barang retur + packaging) ÷ qty hasil, 4 desimal (INV-06).
func OutputUnitCost(inputCost, outputQty decimal.Decimal) decimal.Decimal {
	if !outputQty.IsPositive() {
		return decimal.Zero
	}
	return inputCost.Div(outputQty).Round(UnitCostScale)
}
