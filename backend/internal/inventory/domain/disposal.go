package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// DisposalReason adalah alasan pemusnahan (Desain Data Model `disposals.reason`).
type DisposalReason string

const (
	DisposalExpired DisposalReason = "expired"
	DisposalDamaged DisposalReason = "damaged"
	// DisposalReturn: barang dari Gudang Retur (disposisi retur), dijurnal A-26 (Dr 6403, Cr 1314).
	DisposalReturn DisposalReason = "return"
)

var disposalReasonLabels = map[DisposalReason]string{
	DisposalExpired: "Kedaluwarsa", DisposalDamaged: "Rusak", DisposalReturn: "Barang Retur",
}

// Valid memeriksa alasan.
func (r DisposalReason) Valid() bool {
	_, ok := disposalReasonLabels[r]
	return ok
}

// Label adalah label alasan.
func (r DisposalReason) Label() string { return disposalReasonLabels[r] }

// PostingRule memilih jurnal: barang retur A-26 (musnah), kedaluwarsa/rusak di gudang A-27 (Desain Finance 2.1).
func (r DisposalReason) PostingRule() string {
	if r == DisposalReturn {
		return "A-26"
	}
	return "A-27"
}

// Batas berita acara.
const (
	MaxWitnesses      = 10
	MaxPhotos         = 10
	MaxWitnessLength  = 120
	MaxMethodLength   = 500
	MinWitnessesForBA = 1
	MinPhotosForBA    = 1
)

// Disposal adalah dokumen Pemusnahan dengan Berita Acara (INV-12): draft → pending_approval → approved → posted.
type Disposal struct {
	ID                  uuid.UUID
	Number              string
	Reason              DisposalReason
	DisposalDate        clock.BusinessDate
	Method              string
	Witnesses           []string
	PhotoFileIDs        []uuid.UUID
	Note                string
	Status              DocStatus
	EstimatedCost       decimal.Decimal
	CostAmount          decimal.Decimal
	PostingRuleCode     string
	JournalID           *uuid.UUID
	ReturnDispositionID *uuid.UUID
	CreatedBy           Actor
	Submitted           *Step
	Posted              *Step
	Lines               []DisposalLine
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int
}

// DisposalLine adalah barang yang dimusnahkan.
type DisposalLine struct {
	ID          uuid.UUID
	LineNo      int
	ProductID   uuid.UUID
	BatchID     *uuid.UUID
	LocationID  uuid.UUID
	StockStatus StockStatus
	Qty         decimal.Decimal
	CostAmount  decimal.Decimal
	Note        string
}

// DisposalLineInput adalah isian baris.
type DisposalLineInput struct {
	ProductID   uuid.UUID
	BatchID     *uuid.UUID
	LocationID  uuid.UUID
	StockStatus StockStatus
	Qty         decimal.Decimal
	Note        string
}

// DisposalReport adalah isi berita acara: tanggal, metode, saksi, foto.
type DisposalReport struct {
	DisposalDate clock.BusinessDate
	Method       string
	Witnesses    []string
	PhotoFileIDs []uuid.UUID
}

// DisposalInput adalah isian form Pemusnahan.
type DisposalInput struct {
	Reason DisposalReason
	DisposalReport
	Note  string
	Lines []DisposalLineInput
}

// NewDisposal memvalidasi dan membuat draft.
func NewDisposal(in DisposalInput, by Actor) (Disposal, error) {
	d := Disposal{Status: DocDraft, CreatedBy: by, EstimatedCost: decimal.Zero, CostAmount: decimal.Zero}
	if err := d.apply(in); err != nil {
		return Disposal{}, err
	}
	return d, nil
}

// Update mengganti isian draft.
func (d *Disposal) Update(in DisposalInput) error {
	if d.Status != DocDraft {
		return errTransition(ActionEdit, d.Status)
	}
	return d.apply(in)
}

func normalizeReport(fields map[string]string, r DisposalReport) DisposalReport {
	if r.DisposalDate.IsZero() {
		fields["disposalDate"] = "Tanggal pemusnahan wajib diisi"
	}
	r.Method = strings.TrimSpace(r.Method)
	if len([]rune(r.Method)) > MaxMethodLength {
		fields["method"] = "Metode maksimal 500 karakter"
	}
	var witnesses []string
	for i, w := range r.Witnesses {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if len([]rune(w)) > MaxWitnessLength {
			fields[docLineField(i, "witness")] = "Nama saksi maksimal 120 karakter"
		}
		witnesses = append(witnesses, w)
	}
	if len(witnesses) > MaxWitnesses {
		fields["witnesses"] = "Maksimal 10 saksi"
	}
	if len(r.PhotoFileIDs) > MaxPhotos {
		fields["photoFileIds"] = "Maksimal 10 foto"
	}
	r.Witnesses = witnesses
	if r.Witnesses == nil {
		r.Witnesses = []string{}
	}
	if r.PhotoFileIDs == nil {
		r.PhotoFileIDs = []uuid.UUID{}
	}
	return r
}

func (d *Disposal) apply(in DisposalInput) error {
	fields := map[string]string{}
	if !in.Reason.Valid() {
		fields["reason"] = "Alasan pemusnahan tidak valid"
	}
	report := normalizeReport(fields, in.DisposalReport)
	note := checkNote(fields, "note", in.Note)
	checkLineCount(fields, len(in.Lines))
	lines := make([]DisposalLine, len(in.Lines))
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
		lines[i] = DisposalLine{
			LineNo: i + 1, ProductID: l.ProductID, BatchID: l.BatchID, LocationID: l.LocationID, StockStatus: l.StockStatus,
			Qty: l.Qty, CostAmount: decimal.Zero, Note: checkNote(fields, docLineField(i, "note"), l.Note),
		}
	}
	if err := validationOrNil(fields); err != nil {
		return err
	}
	d.Reason, d.DisposalDate, d.Method = in.Reason, report.DisposalDate, report.Method
	d.Witnesses, d.PhotoFileIDs, d.Note, d.Lines = report.Witnesses, report.PhotoFileIDs, note, lines
	return nil
}

// UpdateReport mengubah isi berita acara (draft atau sudah disetujui, sebelum posting).
func (d *Disposal) UpdateReport(r DisposalReport) error {
	if d.Status != DocDraft && d.Status != DocApproved {
		return errTransition(ActionReport, d.Status)
	}
	fields := map[string]string{}
	r = normalizeReport(fields, r)
	if err := validationOrNil(fields); err != nil {
		return err
	}
	d.DisposalDate, d.Method, d.Witnesses, d.PhotoFileIDs = r.DisposalDate, r.Method, r.Witnesses, r.PhotoFileIDs
	return nil
}

// ReportComplete memeriksa kelengkapan berita acara sebelum posting (metode, saksi, foto).
func (d Disposal) ReportComplete() error {
	fields := map[string]string{}
	if d.Method == "" {
		fields["method"] = "Metode pemusnahan wajib diisi sebelum posting"
	}
	if len(d.Witnesses) < MinWitnessesForBA {
		fields["witnesses"] = "Minimal satu saksi sebelum posting"
	}
	if len(d.PhotoFileIDs) < MinPhotosForBA {
		fields["photoFileIds"] = "Minimal satu foto sebelum posting"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

var disposalTransitions = []transition{
	{ActionSubmit, []DocStatus{DocDraft}, DocPendingApproval},
	{ActionApprove, []DocStatus{DocPendingApproval}, DocApproved},
	{ActionReject, []DocStatus{DocPendingApproval}, DocRejected},
	{ActionRevise, []DocStatus{DocRejected}, DocDraft},
	{ActionExecute, []DocStatus{DocApproved}, DocPosted},
	{ActionCancel, []DocStatus{DocDraft, DocApproved}, DocCancelled},
}

// Transition menjalankan aksi pemusnahan; posting mewajibkan berita acara lengkap.
func (d *Disposal) Transition(a DocAction) error {
	to, ok := findTransition(disposalTransitions, a, d.Status)
	if !ok {
		return errTransition(a, d.Status)
	}
	if a == ActionExecute {
		if err := d.ReportComplete(); err != nil {
			return err
		}
	}
	d.Status = to
	return nil
}

// Permission Pemusnahan (Desain Role & Approval 3.2). Head Logistik mengajukan (S).
const (
	PermDisposalRead   = "inventory.disposal.read"
	PermDisposalWrite  = "inventory.disposal.write"
	PermDisposalSubmit = "inventory.disposal.submit"
	PermDisposalPost   = "inventory.disposal.post"
)

// AllowedActions mengembalikan aksi yang bisa dilakukan pemegang permission.
func (d Disposal) AllowedActions(has func(string) bool) []DocAction {
	var out []DocAction
	if d.Status == DocDraft && has(PermDisposalWrite) {
		out = append(out, ActionEdit)
	}
	if (d.Status == DocDraft || d.Status == DocApproved) && has(PermDisposalWrite) {
		out = append(out, ActionReport)
	}
	perms := map[DocAction]string{
		ActionSubmit: PermDisposalSubmit, ActionRevise: PermDisposalWrite, ActionExecute: PermDisposalPost,
		ActionCancel: PermDisposalWrite,
	}
	for _, a := range []DocAction{ActionSubmit, ActionRevise, ActionExecute, ActionCancel} {
		if _, ok := findTransition(disposalTransitions, a, d.Status); ok && has(perms[a]) {
			// Pemusnahan dari Disposisi Retur sudah disetujui lewat disposisi; batal lewat disposisi tidak ada.
			if a == ActionCancel && d.ReturnDispositionID != nil {
				continue
			}
			out = append(out, a)
		}
	}
	return out
}
