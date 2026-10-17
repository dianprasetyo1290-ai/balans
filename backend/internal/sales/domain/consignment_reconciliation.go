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

// VarianceCause adalah penyebab selisih konsinyasi (Desain Data Model 8 `variance_cause`).
type VarianceCause string

const (
	CauseLost          VarianceCause = "lost"
	CausePartnerDamage VarianceCause = "partner_damage"
	CauseProductDefect VarianceCause = "product_defect"
	CauseNearExpiry    VarianceCause = "near_expiry"
)

var causeLabels = map[VarianceCause]string{
	CauseLost: "Hilang", CausePartnerDamage: "Rusak karena mitra", CauseProductDefect: "Cacat produk",
	CauseNearExpiry: "Mendekati kedaluwarsa",
}

// Valid memeriksa penyebab selisih.
func (c VarianceCause) Valid() bool { _, ok := causeLabels[c]; return ok }

// Label adalah label UI penyebab selisih.
func (c VarianceCause) Label() string {
	if l, ok := causeLabels[c]; ok {
		return l
	}
	return string(c)
}

// BorneBy adalah penanggung selisih: ditagih ke mitra atau dibebankan Balans.
type BorneBy string

const (
	BorneByPartner BorneBy = "partner"
	BorneByBalans  BorneBy = "balans"
)

// Label adalah label UI penanggung selisih.
func (b BorneBy) Label() string {
	switch b {
	case BorneByPartner:
		return "Ditagih ke mitra"
	case BorneByBalans:
		return "Ditanggung Balans"
	}
	return ""
}

// VarianceRules adalah ketentuan selisih PKS per penyebab (BRD U-02; default hilang & rusak karena mitra ditagih
// ke mitra, cacat produk & mendekati kedaluwarsa ditanggung Balans).
type VarianceRules struct {
	Lost          BorneBy
	PartnerDamage BorneBy
	ProductDefect BorneBy
	NearExpiry    BorneBy
}

// DefaultVarianceRules adalah ketentuan selisih default BRD U-02.
var DefaultVarianceRules = VarianceRules{
	Lost: BorneByPartner, PartnerDamage: BorneByPartner, ProductDefect: BorneByBalans, NearExpiry: BorneByBalans,
}

func orDefault(b, def BorneBy) BorneBy {
	if b == BorneByPartner || b == BorneByBalans {
		return b
	}
	return def
}

// Normalize mengisi penanggung kosong dengan default U-02.
func (r VarianceRules) Normalize() VarianceRules {
	d := DefaultVarianceRules
	return VarianceRules{
		Lost: orDefault(r.Lost, d.Lost), PartnerDamage: orDefault(r.PartnerDamage, d.PartnerDamage),
		ProductDefect: orDefault(r.ProductDefect, d.ProductDefect), NearExpiry: orDefault(r.NearExpiry, d.NearExpiry),
	}
}

// For mengembalikan penanggung untuk penyebab selisih.
func (r VarianceRules) For(c VarianceCause) BorneBy {
	r = r.Normalize()
	switch c {
	case CauseLost:
		return r.Lost
	case CausePartnerDamage:
		return r.PartnerDamage
	case CauseProductDefect:
		return r.ProductDefect
	case CauseNearExpiry:
		return r.NearExpiry
	}
	return ""
}

// ReconciliationLine adalah satu SKU di Berita Acara Rekonsiliasi: terjual sistem = stok awal + kiriman − tarikan −
// sisa fisik (BRD 5.5); selisih = terjual sistem − laporan mitra.
type ReconciliationLine struct {
	ID               uuid.UUID
	LineNo           int
	ProductID        uuid.UUID
	OpeningQty       decimal.Decimal
	DeliveredQty     decimal.Decimal
	ReturnedQty      decimal.Decimal
	CountedQty       decimal.Decimal
	SoldQty          decimal.Decimal
	ReportedQty      decimal.Decimal
	VarianceQty      decimal.Decimal
	Cause            VarianceCause
	BorneBy          BorneBy
	UnitPrice        money.Amount
	PriceRule        string
	PriceDescription string
	// SalesAmount = laporan mitra × harga konsinyasi (invoice A-08).
	SalesAmount money.Amount
	// VarianceAmount = selisih × harga bila ditagih ke mitra (invoice selisih A-23).
	VarianceAmount money.Amount
	// Cost dan VarianceCost adalah HPP FIFO barang terjual dan selisih, diisi saat posting.
	Cost         money.Amount
	VarianceCost money.Amount
	Note         string
}

// LineSource adalah bahan baris rekonsiliasi yang dihitung service dari ledger stok, stock count, dan pricing.
type LineSource struct {
	ProductID        uuid.UUID
	OpeningQty       decimal.Decimal
	DeliveredQty     decimal.Decimal
	ReturnedQty      decimal.Decimal
	CountedQty       decimal.Decimal
	UnitPrice        money.Amount
	PriceRule        string
	PriceDescription string
}

func (l *ReconciliationLine) recalc(rules VarianceRules) {
	l.VarianceQty = l.SoldQty.Sub(l.ReportedQty)
	if !l.VarianceQty.IsPositive() {
		l.Cause = ""
	}
	l.BorneBy = ""
	if l.Cause != "" {
		l.BorneBy = rules.For(l.Cause)
	}
	l.SalesAmount = l.UnitPrice.Mul(l.ReportedQty).Round()
	l.VarianceAmount = money.Zero
	if l.BorneBy == BorneByPartner && l.VarianceQty.IsPositive() {
		l.VarianceAmount = l.UnitPrice.Mul(l.VarianceQty).Round()
	}
}

// IssueQty adalah qty yang keluar dari lokasi konsinyasi saat posting (terjual + selisih).
func (l ReconciliationLine) IssueQty() decimal.Decimal { return l.ReportedQty.Add(l.VarianceQty) }

// Reconciliation adalah Rekonsiliasi Konsinyasi bulanan per mitra (KSN, Berita Acara Rekonsiliasi; Desain Status
// Dokumen 5.6): draft → in_progress (mulai) → pending_approval (submit) → posted (disetujui Sales MT → Head F&A;
// invoice + jurnal A-08/A-23), atau rejected → in_progress (revisi).
type Reconciliation struct {
	ID                  uuid.UUID
	Number              string
	PartnerID           uuid.UUID
	PartnerCode         string
	PartnerName         string
	PartnerAddress      string
	AgreementID         *uuid.UUID
	AgreementNumber     string
	PaymentTermDays     int
	MarginPercent       *decimal.Decimal
	Rules               VarianceRules
	LocationID          uuid.UUID
	ChannelID           uuid.UUID
	SubChannelID        uuid.UUID
	PeriodStart         *clock.BusinessDate
	CutOffDate          clock.BusinessDate
	StockCountID        uuid.UUID
	StockCountNumber    string
	PartnerReportFileID *uuid.UUID
	BAFileID            *uuid.UUID
	Status              Status
	Note                string
	SalesAmount         money.Amount
	BilledVariance      money.Amount
	AbsorbedCost        money.Amount
	Cost                money.Amount
	InvoiceID           *uuid.UUID
	VarianceInvoiceID   *uuid.UUID
	StartedAt           *time.Time
	SubmittedAt         *time.Time
	DecidedAt           *time.Time
	DecisionNote        string
	PostedAt            *time.Time
	CancelReason        string
	CreatedAt           time.Time
	CreatedBy           uuid.UUID
	UpdatedAt           time.Time
	Version             int
	Lines               []ReconciliationLine
}

// ReconciliationInput adalah isian rekonsiliasi baru: mitra, tanggal cut-off, stock count lokasi mitra.
type ReconciliationInput struct {
	PartnerID    uuid.UUID
	CutOffDate   clock.BusinessDate
	StockCountID uuid.UUID
	Note         string
}

func validateReconciliation(in ReconciliationInput) error {
	f := map[string]string{}
	if in.PartnerID == uuid.Nil {
		f["partnerId"] = "Pilih mitra konsinyasi"
	}
	if in.CutOffDate.IsZero() {
		f["cutOffDate"] = "Tanggal cut-off wajib diisi"
	}
	if in.StockCountID == uuid.Nil {
		f["stockCountId"] = "Pilih stock count lokasi mitra"
	}
	if tooLong(in.Note, MaxNoteLength) {
		f["note"] = "Catatan maksimal 1000 karakter"
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewReconciliation memvalidasi rekonsiliasi baru berstatus Draft.
func NewReconciliation(in ReconciliationInput) (Reconciliation, error) {
	if err := validateReconciliation(in); err != nil {
		return Reconciliation{}, err
	}
	return Reconciliation{
		PartnerID: in.PartnerID, CutOffDate: in.CutOffDate, StockCountID: in.StockCountID, Status: StatusDraft,
		Note: strings.TrimSpace(in.Note), Rules: DefaultVarianceRules, SalesAmount: money.Zero,
		BilledVariance: money.Zero, AbsorbedCost: money.Zero, Cost: money.Zero,
	}, nil
}

// Label adalah nomor KSN atau "Draft".
func (r Reconciliation) Label() string {
	if r.Number == "" {
		return "Draft"
	}
	return r.Number
}

func (r Reconciliation) stateError(action string) error {
	return apperr.WithMessage(apperr.CodeInvalidStateTransition,
		fmt.Sprintf("Rekonsiliasi %s berstatus %s tidak bisa %s.", r.Label(), r.Status.Label(), action))
}

// UpdateDraft mengganti cut-off, stock count, dan catatan rekonsiliasi Draft.
func (r *Reconciliation) UpdateDraft(in ReconciliationInput) error {
	if r.Status != StatusDraft {
		return r.stateError("diubah")
	}
	in.PartnerID = r.PartnerID
	if err := validateReconciliation(in); err != nil {
		return err
	}
	r.CutOffDate, r.StockCountID, r.Note = in.CutOffDate, in.StockCountID, strings.TrimSpace(in.Note)
	return nil
}

// Start memulai rekonsiliasi: nomor KSN, periode, dan baris dihitung dari ledger + stock count; laporan mitra
// awal = terjual sistem (tanpa selisih) dan diubah Sales MT.
func (r *Reconciliation) Start(number string, periodStart clock.BusinessDate, src []LineSource, now time.Time) error {
	if r.Status != StatusDraft {
		return r.stateError("dimulai")
	}
	if periodStart.After(r.CutOffDate) {
		return apperr.Validation(map[string]string{"cutOffDate": "Cut-off harus setelah cut-off rekonsiliasi sebelumnya"})
	}
	if len(src) == 0 {
		return apperr.Validation(map[string]string{"stockCountId": "Tidak ada stok maupun mutasi di lokasi konsinyasi mitra pada periode ini"})
	}
	r.Rules = r.Rules.Normalize()
	r.Number, r.PeriodStart, r.StartedAt = number, &periodStart, &now
	r.Lines = make([]ReconciliationLine, len(src))
	for i, s := range src {
		sold := s.OpeningQty.Add(s.DeliveredQty).Sub(s.ReturnedQty).Sub(s.CountedQty)
		reported := sold
		if reported.IsNegative() {
			reported = decimal.Zero
		}
		l := ReconciliationLine{
			LineNo: i + 1, ProductID: s.ProductID, OpeningQty: s.OpeningQty, DeliveredQty: s.DeliveredQty,
			ReturnedQty: s.ReturnedQty, CountedQty: s.CountedQty, SoldQty: sold, ReportedQty: reported,
			UnitPrice: s.UnitPrice, PriceRule: s.PriceRule, PriceDescription: s.PriceDescription,
			Cost: money.Zero, VarianceCost: money.Zero,
		}
		l.recalc(r.Rules)
		r.Lines[i] = l
	}
	r.Status = StatusInProgress
	r.recalcTotals()
	return nil
}

func (r *Reconciliation) recalcTotals() {
	r.SalesAmount, r.BilledVariance = money.Zero, money.Zero
	for _, l := range r.Lines {
		r.SalesAmount = r.SalesAmount.Add(l.SalesAmount)
		r.BilledVariance = r.BilledVariance.Add(l.VarianceAmount)
	}
}

// LineEntry adalah isian Sales MT per baris: qty laporan penjualan mitra, penyebab selisih, catatan.
type LineEntry struct {
	LineID      uuid.UUID
	ReportedQty decimal.Decimal
	Cause       VarianceCause
	Note        string
}

// ReconciliationUpdate adalah perubahan rekonsiliasi yang sedang berjalan.
type ReconciliationUpdate struct {
	Lines []LineEntry
	// SetFiles true: laporan mitra dan BA diganti (nil = dikosongkan).
	SetFiles            bool
	PartnerReportFileID *uuid.UUID
	BAFileID            *uuid.UUID
	SetNote             bool
	Note                string
}

// Update mencatat laporan mitra, penyebab selisih, file laporan/BA, dan catatan pada rekonsiliasi Diproses.
func (r *Reconciliation) Update(u ReconciliationUpdate) error {
	if r.Status != StatusInProgress {
		return r.stateError("diubah")
	}
	f := map[string]string{}
	idx := map[uuid.UUID]int{}
	for i, l := range r.Lines {
		idx[l.ID] = i
	}
	for i, e := range u.Lines {
		j, ok := idx[e.LineID]
		if !ok {
			f[fmt.Sprintf("lines.%d.lineId", i)] = "Baris tidak ditemukan"
			continue
		}
		l := &r.Lines[j]
		switch {
		case e.ReportedQty.IsNegative():
			f[fmt.Sprintf("lines.%d.reportedQty", i)] = "Qty laporan mitra tidak boleh negatif"
		case !e.ReportedQty.Equal(e.ReportedQty.Round(QtyScale)):
			f[fmt.Sprintf("lines.%d.reportedQty", i)] = "Qty maksimal 3 angka di belakang koma"
		case e.ReportedQty.GreaterThan(l.SoldQty) && !e.ReportedQty.IsZero():
			f[fmt.Sprintf("lines.%d.reportedQty", i)] = fmt.Sprintf(
				"Laporan mitra melebihi terjual sistem (%s); periksa stock count atau laporan mitra", l.SoldQty.String())
		}
		if e.Cause != "" && !e.Cause.Valid() {
			f[fmt.Sprintf("lines.%d.varianceCause", i)] = "Penyebab selisih tidak valid"
		}
		if tooLong(e.Note, MaxTextLength) {
			f[fmt.Sprintf("lines.%d.note", i)] = "Catatan maksimal 200 karakter"
		}
		l.ReportedQty, l.Cause, l.Note = e.ReportedQty, e.Cause, strings.TrimSpace(e.Note)
		l.recalc(r.Rules)
	}
	if u.SetNote {
		if tooLong(u.Note, MaxNoteLength) {
			f["note"] = "Catatan maksimal 1000 karakter"
		}
		r.Note = strings.TrimSpace(u.Note)
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	if u.SetFiles {
		r.PartnerReportFileID, r.BAFileID = u.PartnerReportFileID, u.BAFileID
	}
	r.recalcTotals()
	return nil
}

// Submit memeriksa kelengkapan (BA bertanda tangan, sisa fisik ≤ saldo sistem, penyebab setiap selisih) lalu
// mengajukan approval.
func (r *Reconciliation) Submit(now time.Time) error {
	if r.Status != StatusInProgress {
		return r.stateError("diajukan")
	}
	f := map[string]string{}
	if r.BAFileID == nil {
		f["baFileId"] = "Unggah Berita Acara Rekonsiliasi yang sudah ditandatangani mitra"
	}
	for i, l := range r.Lines {
		switch {
		case l.SoldQty.IsNegative():
			f[fmt.Sprintf("lines.%d.countedQty", i)] = "Sisa fisik melebihi saldo sistem; periksa stock count atau tarikan"
		case l.VarianceQty.IsNegative():
			f[fmt.Sprintf("lines.%d.reportedQty", i)] = "Laporan mitra melebihi terjual sistem"
		case l.VarianceQty.IsPositive() && l.Cause == "":
			f[fmt.Sprintf("lines.%d.varianceCause", i)] = "Pilih penyebab selisih"
		}
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	r.Status, r.SubmittedAt = StatusPendingApproval, &now
	return nil
}

// ApprovalAmount adalah nilai tagihan (penjualan + selisih ditagih) untuk approval.
func (r Reconciliation) ApprovalAmount() money.Amount { return r.SalesAmount.Add(r.BilledVariance) }

// HasVariance bernilai true bila ada baris selisih.
func (r Reconciliation) HasVariance() bool {
	for _, l := range r.Lines {
		if l.VarianceQty.IsPositive() {
			return true
		}
	}
	return false
}

// LineCost adalah HPP hasil posting satu baris.
type LineCost struct {
	LineID       uuid.UUID
	Cost         money.Amount
	VarianceCost money.Amount
}

// MarkPosted mencatat hasil approval akhir: HPP per baris, invoice konsinyasi, dan invoice selisih.
func (r *Reconciliation) MarkPosted(costs []LineCost, invoiceID, varianceInvoiceID *uuid.UUID, note string, now time.Time) error {
	if r.Status != StatusPendingApproval {
		return r.stateError("diposting")
	}
	byID := map[uuid.UUID]LineCost{}
	for _, c := range costs {
		byID[c.LineID] = c
	}
	r.Cost, r.AbsorbedCost = money.Zero, money.Zero
	for i := range r.Lines {
		l := &r.Lines[i]
		c := byID[l.ID]
		l.Cost, l.VarianceCost = c.Cost, c.VarianceCost
		r.Cost = r.Cost.Add(c.Cost)
		if l.BorneBy == BorneByBalans {
			r.AbsorbedCost = r.AbsorbedCost.Add(c.VarianceCost)
		} else {
			r.Cost = r.Cost.Add(c.VarianceCost)
		}
	}
	r.InvoiceID, r.VarianceInvoiceID = invoiceID, varianceInvoiceID
	r.Status, r.DecidedAt, r.PostedAt, r.DecisionNote = StatusPosted, &now, &now, strings.TrimSpace(note)
	return nil
}

// Reject mencatat penolakan approval.
func (r *Reconciliation) Reject(note string, now time.Time) error {
	if r.Status != StatusPendingApproval {
		return r.stateError("ditolak")
	}
	r.Status, r.DecidedAt, r.DecisionNote = StatusRejected, &now, strings.TrimSpace(note)
	return nil
}

// Revise membuka kembali rekonsiliasi yang ditolak.
func (r *Reconciliation) Revise() error {
	if r.Status != StatusRejected {
		return r.stateError("direvisi")
	}
	r.Status, r.SubmittedAt = StatusInProgress, nil
	return nil
}

// Cancel membatalkan rekonsiliasi Draft atau Diproses.
func (r *Reconciliation) Cancel(reason string) error {
	rs, err := requireReason(reason, "pembatalan")
	if err != nil {
		return err
	}
	if r.Status != StatusDraft && r.Status != StatusInProgress {
		return r.stateError("dibatalkan")
	}
	r.Status, r.CancelReason = StatusCancelled, rs
	return nil
}

// Actions adalah aksi rekonsiliasi sebelum disaring permission.
func (r Reconciliation) Actions() []Action {
	switch r.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionStart, ActionCancel}
	case StatusInProgress:
		return []Action{ActionUpdate, ActionSubmit, ActionCancel, ActionPrint}
	case StatusRejected:
		return []Action{ActionRevise, ActionPrint}
	case StatusPendingApproval, StatusPosted:
		return []Action{ActionPrint}
	}
	return nil
}

// PeriodStartAfter adalah awal periode: hari setelah cut-off rekonsiliasi terakhir, atau tanggal 1 bulan cut-off.
func PeriodStartAfter(last *clock.BusinessDate, cutOff clock.BusinessDate) clock.BusinessDate {
	if last != nil {
		return last.AddDays(1)
	}
	return clock.NewBusinessDate(cutOff.Year(), cutOff.Month(), 1)
}

// ConsignmentUnitPrice menghitung harga konsinyasi: harga dari daftar harga PKS dipakai apa adanya; bila PKS
// tanpa daftar harga, harga resolver dipotong margin mitra (%).
func ConsignmentUnitPrice(resolved money.Amount, fromAgreementList bool, margin *decimal.Decimal) money.Amount {
	if fromAgreementList || margin == nil || margin.IsZero() {
		return resolved.Round()
	}
	return resolved.Sub(resolved.Percent(*margin)).Round()
}
