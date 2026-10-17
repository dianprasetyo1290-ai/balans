package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// UsagePurpose adalah tujuan Pemakaian Internal FG (INV-05, HR-02).
type UsagePurpose string

const (
	PurposeMarketing      UsagePurpose = "marketing"
	PurposeEvent          UsagePurpose = "event"
	PurposeEmployeeSample UsagePurpose = "employee_sample"
	PurposeSalesSampling  UsagePurpose = "sales_sampling"
)

var purposeLabels = map[UsagePurpose]string{
	PurposeMarketing: "Marketing", PurposeEvent: "Event", PurposeEmployeeSample: "Sampel Karyawan",
	PurposeSalesSampling: "Sampling Sales",
}

// Valid memeriksa tujuan.
func (p UsagePurpose) Valid() bool {
	_, ok := purposeLabels[p]
	return ok
}

// Label adalah label tujuan.
func (p UsagePurpose) Label() string { return purposeLabels[p] }

// RequiresCampaign: semua tujuan kecuali sampel karyawan dibebankan ke Biaya Marketing (A-11) yang wajib kode
// kampanye (Desain Finance & Accounting 3.3; sampling sales U-19).
func (p UsagePurpose) RequiresCampaign() bool { return p != PurposeEmployeeSample }

// PostingRule adalah rule jurnal pemakaian: A-12 sampel karyawan, selain itu A-11.
func (p UsagePurpose) PostingRule() string {
	if p == PurposeEmployeeSample {
		return "A-12"
	}
	return "A-11"
}

// InternalUsage adalah dokumen Pemakaian Internal (Desain Status Dokumen 4.1).
type InternalUsage struct {
	ID               uuid.UUID
	Number           string
	Purpose          UsagePurpose
	DivisionID       uuid.UUID
	CampaignID       *uuid.UUID
	SourceLocationID uuid.UUID
	EventLocationID  *uuid.UUID
	EventEndDate     *clock.BusinessDate
	// SettleDueDate = tanggal selesai event + 1 (sisa wajib dicatat maks. H+1).
	SettleDueDate *clock.BusinessDate
	BusinessDate  clock.BusinessDate
	Note          string
	Status        DocStatus
	EstimatedCost decimal.Decimal
	CostAmount    decimal.Decimal
	JournalID     *uuid.UUID
	// ReturnDispositionID: dibuat dari Disposisi Retur (sampel) dan langsung berstatus Disetujui.
	ReturnDispositionID *uuid.UUID
	RequestedBy         Actor
	Released            *Step
	HandedOver          *Step
	Settled             *Step
	Posted              *Step
	RemindedOn          *clock.BusinessDate
	Lines               []UsageLine
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int
}

// UsageLine adalah satu baris pemakaian. ConsumedQty = Qty − ReturnedQty; CostAmount atas ConsumedQty.
type UsageLine struct {
	ID          uuid.UUID
	LineNo      int
	ProductID   uuid.UUID
	Qty         decimal.Decimal
	ReturnedQty decimal.Decimal
	ConsumedQty decimal.Decimal
	CostAmount  decimal.Decimal
	Note        string
}

// IsEvent memeriksa apakah pemakaian untuk event (stok dipindah ke lokasi Event dulu).
func (u InternalUsage) IsEvent() bool { return u.Purpose == PurposeEvent }

// UsageLineInput adalah isian baris.
type UsageLineInput struct {
	ProductID uuid.UUID
	Qty       decimal.Decimal
	Note      string
}

// InternalUsageInput adalah isian form Pemakaian Internal.
type InternalUsageInput struct {
	Purpose          UsagePurpose
	DivisionID       uuid.UUID
	CampaignID       *uuid.UUID
	SourceLocationID uuid.UUID
	EventLocationID  *uuid.UUID
	EventEndDate     *clock.BusinessDate
	BusinessDate     clock.BusinessDate
	Note             string
	Lines            []UsageLineInput
}

// NewInternalUsage memvalidasi isian dan membuat draft. Jenis lokasi, produk, dan dimensi diperiksa service.
func NewInternalUsage(in InternalUsageInput, by Actor) (InternalUsage, error) {
	u := InternalUsage{Status: DocDraft, RequestedBy: by, EstimatedCost: decimal.Zero, CostAmount: decimal.Zero}
	if err := u.apply(in, false); err != nil {
		return InternalUsage{}, err
	}
	return u, nil
}

// Update mengganti isian draft.
func (u *InternalUsage) Update(in InternalUsageInput) error {
	if u.Status != DocDraft {
		return errTransition(ActionEdit, u.Status)
	}
	return u.apply(in, u.ReturnDispositionID != nil)
}

func (u *InternalUsage) apply(in InternalUsageInput, fromDisposition bool) error {
	fields := map[string]string{}
	if !in.Purpose.Valid() {
		fields["purpose"] = "Tujuan pemakaian tidak valid"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib diisi"
	}
	if in.Purpose.Valid() && in.Purpose.RequiresCampaign() && in.CampaignID == nil && !fromDisposition {
		fields["campaignId"] = "Kode kampanye wajib diisi untuk pemakaian marketing, event, dan sampling sales"
	}
	if in.SourceLocationID == uuid.Nil {
		fields["sourceLocationId"] = "Lokasi asal wajib diisi"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	var due *clock.BusinessDate
	if in.Purpose == PurposeEvent {
		if in.EventLocationID == nil || *in.EventLocationID == uuid.Nil {
			fields["eventLocationId"] = "Lokasi Event wajib diisi"
		}
		switch {
		case in.EventEndDate == nil:
			fields["eventEndDate"] = "Tanggal selesai event wajib diisi"
		case !in.BusinessDate.IsZero() && in.EventEndDate.Before(in.BusinessDate):
			fields["eventEndDate"] = "Tanggal selesai event tidak boleh sebelum tanggal pemakaian"
		default:
			d := in.EventEndDate.AddDays(1)
			due = &d
		}
	} else {
		in.EventLocationID, in.EventEndDate = nil, nil
	}
	note := checkNote(fields, "note", in.Note)
	checkLineCount(fields, len(in.Lines))
	seen := map[uuid.UUID]bool{}
	lines := make([]UsageLine, len(in.Lines))
	for i, l := range in.Lines {
		if l.ProductID == uuid.Nil {
			fields[docLineField(i, "productId")] = "Produk wajib diisi"
		} else if seen[l.ProductID] {
			fields[docLineField(i, "productId")] = "Produk sudah ada di baris lain"
		}
		seen[l.ProductID] = true
		checkQty(fields, docLineField(i, "qty"), l.Qty)
		lines[i] = UsageLine{
			LineNo: i + 1, ProductID: l.ProductID, Qty: l.Qty, ReturnedQty: decimal.Zero, ConsumedQty: decimal.Zero,
			CostAmount: decimal.Zero, Note: checkNote(fields, docLineField(i, "note"), l.Note),
		}
	}
	if err := validationOrNil(fields); err != nil {
		return err
	}
	u.Purpose, u.DivisionID, u.CampaignID, u.SourceLocationID = in.Purpose, in.DivisionID, in.CampaignID, in.SourceLocationID
	u.EventLocationID, u.EventEndDate, u.SettleDueDate = in.EventLocationID, in.EventEndDate, due
	u.BusinessDate, u.Note, u.Lines = in.BusinessDate, note, lines
	return nil
}

var usageTransitions = []transition{
	{ActionSubmit, []DocStatus{DocDraft}, DocPendingApproval},
	{ActionApprove, []DocStatus{DocPendingApproval}, DocApproved},
	{ActionReject, []DocStatus{DocPendingApproval}, DocRejected},
	{ActionRevise, []DocStatus{DocRejected}, DocDraft},
	{ActionRelease, []DocStatus{DocApproved}, DocInProgress},
	{ActionHandover, []DocStatus{DocInProgress}, DocPosted},
	{ActionSettle, []DocStatus{DocAwaitingSettlement}, DocAwaitingSettlement},
	{ActionExecute, []DocStatus{DocAwaitingSettlement}, DocPosted},
	{ActionCancel, []DocStatus{DocDraft, DocApproved}, DocCancelled},
}

// Transition menjalankan aksi: event diserahkan ke lokasi Event (Menunggu Penyelesaian), sisa dicatat sekali
// (settle), lalu diposting atas qty terpakai.
func (u *InternalUsage) Transition(a DocAction) error {
	to, ok := findTransition(usageTransitions, a, u.Status)
	switch {
	case !ok:
		return errTransition(a, u.Status)
	case a == ActionSettle && u.Settled != nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Sisa barang event sudah dicatat")
	case a == ActionExecute && u.Settled == nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Catat sisa barang event terlebih dahulu")
	case a == ActionHandover && u.IsEvent():
		to = DocAwaitingSettlement
	}
	u.Status = to
	return nil
}

// Permission Pemakaian Internal (Desain Role & Approval 3.2): peminta W/S, Logistik Leader/Staf Gudang P.
const (
	PermUsageRead   = "inventory.internal_usage.read"
	PermUsageWrite  = "inventory.internal_usage.write"
	PermUsageSubmit = "inventory.internal_usage.submit"
	PermUsagePost   = "inventory.internal_usage.post"
)

// AllowedActions mengembalikan aksi yang bisa dilakukan pemegang permission.
func (u InternalUsage) AllowedActions(has func(string) bool) []DocAction {
	var out []DocAction
	add := func(a DocAction, perm string) {
		if _, ok := findTransition(usageTransitions, a, u.Status); ok && has(perm) {
			out = append(out, a)
		}
	}
	if u.Status == DocDraft && has(PermUsageWrite) {
		out = append(out, ActionEdit)
	}
	add(ActionSubmit, PermUsageSubmit)
	add(ActionRevise, PermUsageWrite)
	add(ActionRelease, PermUsagePost)
	add(ActionHandover, PermUsageWrite)
	if u.Settled == nil {
		add(ActionSettle, PermUsageWrite)
	} else {
		add(ActionExecute, PermUsagePost)
	}
	add(ActionCancel, PermUsageWrite)
	return out
}

// SettleLine adalah qty sisa event satu baris.
type SettleLine struct {
	LineID      uuid.UUID
	ReturnedQty decimal.Decimal
}

// ApplySettlement mencatat qty sisa per baris; baris yang tidak dikirim berarti tanpa sisa.
func (u *InternalUsage) ApplySettlement(lines []SettleLine) error {
	fields := map[string]string{}
	byID := map[uuid.UUID]decimal.Decimal{}
	for i, l := range lines {
		if _, dup := byID[l.LineID]; dup {
			fields[docLineField(i, "lineId")] = "Baris dikirim lebih dari sekali"
		}
		if l.ReturnedQty.IsNegative() || !l.ReturnedQty.Equal(l.ReturnedQty.Round(QtyScale)) {
			fields[docLineField(i, "returnedQty")] = "Qty sisa tidak boleh negatif dan maksimal 3 angka di belakang koma"
		}
		byID[l.LineID] = l.ReturnedQty
	}
	known := map[uuid.UUID]bool{}
	for i := range u.Lines {
		known[u.Lines[i].ID] = true
	}
	for i, l := range lines {
		if !known[l.LineID] {
			fields[docLineField(i, "lineId")] = "Baris tidak ditemukan"
		}
	}
	for i := range u.Lines {
		l := &u.Lines[i]
		ret := byID[l.ID]
		if ret.GreaterThan(l.Qty) {
			fields[docLineField(i, "returnedQty")] = "Qty sisa melebihi qty yang dikirim"
		}
		l.ReturnedQty = ret
		l.ConsumedQty = l.Qty.Sub(ret)
	}
	return validationOrNil(fields)
}

// DistributeReturn membagi qty sisa ke batch yang dikirim: batch dengan kedaluwarsa paling akhir dianggap sisa
// (barang dengan kedaluwarsa terdekat dipakai lebih dulu, FEFO). sent harus sudah urut FEFO.
func DistributeReturn(sent []Candidate, returned decimal.Decimal) []Allocation {
	var out []Allocation
	remaining := returned
	for i := len(sent) - 1; i >= 0 && remaining.IsPositive(); i-- {
		take := decimal.Min(sent[i].Qty, remaining)
		if take.IsPositive() {
			out = append(out, Allocation{Candidate: sent[i], Qty: take})
			remaining = remaining.Sub(take)
		}
	}
	return out
}
