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

// --- Pengajuan biaya / reimbursement (S-404, ACC-11, Desain Status Dokumen 6.1) ---------------

// ClaimType adalah jenis pengajuan.
type ClaimType string

const (
	ClaimExpense       ClaimType = "expense"
	ClaimReimbursement ClaimType = "reimbursement"
)

// PayeeType adalah jenis penerima pembayaran pengajuan biaya.
type PayeeType string

const (
	PayeeEmployee PayeeType = "employee"
	PayeeSupplier PayeeType = "supplier"
	PayeeOther    PayeeType = "other"
)

// Rule dan nilai bernama jurnal biaya operasional (posting_rule_accounts A-15).
const (
	RuleExpenseClaim    = "A-15"
	AmountExpense       = "expense_value"
	AmountPayable       = "payable_value"
	AmountWithholding   = "withholding_value"
	SourceExpenseClaim  = "expense_claim"
	ExpensePayableCode  = "2103"
	MaxClaimDescription = 500
)

// WithholdingAccountCode mengembalikan akun utang PPh per jenis potongan (Desain Finance & Accounting 1).
func WithholdingAccountCode(t WithholdingType) string {
	switch t {
	case WithholdingPPh21:
		return "2301"
	case WithholdingPPh23:
		return "2302"
	case WithholdingPPh42:
		return "2303"
	}
	return ""
}

// FinalTaxPayableCode adalah akun Utang PPh Final.
const FinalTaxPayableCode = "2304"

// ClaimLine adalah satu baris biaya.
type ClaimLine struct {
	ID                 uuid.UUID
	LineNo             int
	ExpenseTypeID      uuid.UUID
	ExpenseTypeCode    string
	ExpenseTypeName    string
	ExpenseAccountCode string
	Description        string
	Amount             money.Amount
	ReceiptFileID      *uuid.UUID
	ReceiptFileName    string
	WithholdingType    WithholdingType
	ObjectCode         string
	WithholdingRate    decimal.Decimal
	WithholdingAmount  money.Amount
}

// ExpenseClaim adalah pengajuan biaya (EXP).
type ExpenseClaim struct {
	ID            uuid.UUID
	Number        string
	ClaimType     ClaimType
	RequesterID   uuid.UUID
	RequesterName string
	DivisionID    uuid.UUID
	DivisionName  string
	CampaignID    *uuid.UUID
	CampaignName  string
	PayeeType     PayeeType
	PayeeID       *uuid.UUID
	PayeeName     string
	// PayeeTaxID adalah NPWP/NIK penerima (didekripsi store) untuk bukti potong.
	PayeeTaxID     string
	ClaimDate      clock.BusinessDate
	Description    string
	Lines          []ClaimLine
	Total          money.Amount
	Withholding    money.Amount
	Payable        money.Amount
	Status         DocStatus
	SubmittedAt    *time.Time
	DecidedAt      *time.Time
	DecisionNote   string
	VerifiedAt     *time.Time
	VerifiedBy     *uuid.UUID
	VerifyDate     *clock.BusinessDate
	JournalEntryID *uuid.UUID
	JournalNumber  string
	PaymentID      *uuid.UUID
	PaymentNumber  string
	PaidAt         *time.Time
	CancelReason   string
	CreatedAt      time.Time
	CreatedBy      uuid.UUID
	UpdatedAt      time.Time
	Version        int
}

// ClaimLineInput adalah isian satu baris biaya.
type ClaimLineInput struct {
	ExpenseTypeID uuid.UUID
	Description   string
	Amount        money.Amount
	ReceiptFileID *uuid.UUID
}

// ClaimInput adalah isian Tambah/Ubah Pengajuan Biaya.
type ClaimInput struct {
	ClaimType   ClaimType
	DivisionID  uuid.UUID
	CampaignID  *uuid.UUID
	PayeeType   PayeeType
	PayeeID     *uuid.UUID
	PayeeName   string
	PayeeTaxID  string
	ClaimDate   clock.BusinessDate
	Description string
	Lines       []ClaimLineInput
}

// MaxClaimLines membatasi baris per pengajuan.
const MaxClaimLines = 100

func (in *ClaimInput) normalize() {
	if in.ClaimType == "" {
		in.ClaimType = ClaimExpense
	}
	if in.PayeeType == "" {
		in.PayeeType = PayeeOther
	}
	if in.PayeeType == PayeeOther {
		in.PayeeID = nil
	}
	in.PayeeName, in.Description = strings.TrimSpace(in.PayeeName), strings.TrimSpace(in.Description)
	in.PayeeTaxID = digitsOnly(in.PayeeTaxID)
	for i := range in.Lines {
		in.Lines[i].Description = strings.TrimSpace(in.Lines[i].Description)
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (in ClaimInput) validate() map[string]string {
	fields := map[string]string{}
	if in.ClaimType != ClaimExpense && in.ClaimType != ClaimReimbursement {
		fields["claimType"] = "Jenis pengajuan tidak dikenal"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib dipilih"
	}
	switch in.PayeeType {
	case PayeeEmployee, PayeeSupplier:
		if in.PayeeID == nil {
			fields["payeeId"] = "Penerima wajib dipilih"
		}
	case PayeeOther:
	default:
		fields["payeeType"] = "Jenis penerima tidak dikenal"
	}
	if in.PayeeName == "" || len(in.PayeeName) > 150 {
		fields["payeeName"] = "Nama penerima wajib diisi (maks. 150 karakter)"
	}
	if in.PayeeTaxID != "" && len(in.PayeeTaxID) != 15 && len(in.PayeeTaxID) != 16 {
		fields["payeeTaxId"] = "NPWP/NIK 15 atau 16 angka"
	}
	if in.ClaimDate.IsZero() {
		fields["claimDate"] = "Tanggal biaya wajib diisi"
	}
	if len(in.Description) > MaxClaimDescription {
		fields["description"] = "Keterangan maks. 500 karakter"
	}
	switch {
	case len(in.Lines) == 0:
		fields["lines"] = "Minimal satu baris biaya"
	case len(in.Lines) > MaxClaimLines:
		fields["lines"] = fmt.Sprintf("Maksimal %d baris biaya", MaxClaimLines)
	}
	for i, l := range in.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		if l.ExpenseTypeID == uuid.Nil {
			fields[p+"expenseTypeId"] = "Jenis biaya wajib dipilih"
		}
		if msg := validMoney(l.Amount); msg != "" {
			fields[p+"amount"] = msg
		}
		if len(l.Description) > 300 {
			fields[p+"description"] = "Uraian maks. 300 karakter"
		}
	}
	return fields
}

// NewExpenseClaim membuat draft pengajuan biaya milik requester.
func NewExpenseClaim(in ClaimInput, requesterID uuid.UUID) (ExpenseClaim, error) {
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return ExpenseClaim{}, apperr.Validation(fields)
	}
	c := ExpenseClaim{RequesterID: requesterID, Status: StatusDraft}
	c.apply(in)
	return c, nil
}

// Update mengubah draft pengajuan.
func (c *ExpenseClaim) Update(in ClaimInput) error {
	if c.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya pengajuan Draft yang bisa diubah.")
	}
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	c.apply(in)
	return nil
}

func (c *ExpenseClaim) apply(in ClaimInput) {
	c.ClaimType, c.DivisionID, c.CampaignID = in.ClaimType, in.DivisionID, in.CampaignID
	c.PayeeType, c.PayeeID, c.PayeeName, c.PayeeTaxID = in.PayeeType, in.PayeeID, in.PayeeName, in.PayeeTaxID
	c.ClaimDate, c.Description = in.ClaimDate, in.Description
	c.Lines = make([]ClaimLine, len(in.Lines))
	total := money.Zero
	for i, l := range in.Lines {
		c.Lines[i] = ClaimLine{
			LineNo: i + 1, ExpenseTypeID: l.ExpenseTypeID, Description: l.Description, Amount: l.Amount,
			ReceiptFileID: l.ReceiptFileID, WithholdingType: WithholdingNone, WithholdingAmount: money.Zero,
		}
		total = total.Add(l.Amount)
	}
	c.Total, c.Withholding, c.Payable = total, money.Zero, total
}

// CheckSubmit memeriksa kelengkapan sebelum diajukan: jenis biaya aktif, bukti wajib dilampirkan untuk jenis
// biaya yang mewajibkan bukti, dan kampanye wajib untuk jenis biaya bertanda kampanye (Desain Finance &
// Accounting 3.4).
func (c ExpenseClaim) CheckSubmit(types map[uuid.UUID]ExpenseType) error {
	fields := map[string]string{}
	for i, l := range c.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		t, ok := types[l.ExpenseTypeID]
		if !ok {
			fields[p+"expenseTypeId"] = "Jenis biaya tidak ditemukan"
			continue
		}
		if msg := t.UsableError(); msg != "" {
			fields[p+"expenseTypeId"] = msg
		}
		if t.RequiresReceipt && l.ReceiptFileID == nil {
			fields[p+"receiptFileId"] = "Bukti wajib dilampirkan untuk " + t.Name
		}
		if t.RequiresCampaign && c.CampaignID == nil {
			fields["campaignId"] = "Kode kampanye wajib untuk " + t.Name
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (c *ExpenseClaim) transition(to DocStatus, from ...DocStatus) error {
	if !slices.Contains(from, c.Status) {
		return invalidTransition("Pengajuan biaya", c.Status, to)
	}
	c.Status = to
	return nil
}

// Submit memberi nomor EXP dan mengajukan approval.
func (c *ExpenseClaim) Submit(number string, at time.Time) error {
	if err := c.transition(StatusPendingApproval, StatusDraft); err != nil {
		return err
	}
	c.Number, c.SubmittedAt = number, &at
	return nil
}

// Approve dipanggil handler approval saat level terakhir menyetujui.
func (c *ExpenseClaim) Approve(at time.Time, note string) error {
	if err := c.transition(StatusApproved, StatusPendingApproval); err != nil {
		return err
	}
	c.DecidedAt, c.DecisionNote = &at, note
	return nil
}

// Reject dipanggil handler approval saat ditolak.
func (c *ExpenseClaim) Reject(at time.Time, note string) error {
	if err := c.transition(StatusRejected, StatusPendingApproval); err != nil {
		return err
	}
	c.DecidedAt, c.DecisionNote = &at, note
	return nil
}

// Revise mengembalikan pengajuan ditolak menjadi draft.
func (c *ExpenseClaim) Revise() error { return c.transition(StatusDraft, StatusRejected) }

// Cancel membatalkan draft atau pengajuan ditolak.
func (c *ExpenseClaim) Cancel(reason string) error {
	r, err := requireReason(reason, "reason", "pembatalan")
	if err != nil {
		return err
	}
	if err := c.transition(StatusCancelled, StatusDraft, StatusRejected); err != nil {
		return err
	}
	c.CancelReason = r
	return nil
}

// LineTaxChoice adalah pilihan PPh per baris saat verifikasi F&A. Type kosong = jenis PPh jenis biaya.
type LineTaxChoice struct {
	LineID     uuid.UUID
	Type       WithholdingType
	ObjectCode string
}

// TaxGroup adalah gabungan baris dengan jenis PPh dan kode objek yang sama: PPh dihitung atas total bruto
// kelompok (satu bukti potong per objek pajak per pengajuan).
type TaxGroup struct {
	Type       WithholdingType
	ObjectCode string
	LineIDs    []uuid.UUID
	Gross      money.Amount
}

// TaxGroups mengelompokkan baris berdasarkan pilihan PPh. Baris tanpa pilihan memakai jenis PPh jenis biaya
// (defaults) dan kode objek kosong (diisi tarif default oleh service).
func (c ExpenseClaim) TaxGroups(choices []LineTaxChoice, defaults map[uuid.UUID]WithholdingType) ([]TaxGroup, error) {
	byLine := map[uuid.UUID]LineTaxChoice{}
	for _, ch := range choices {
		if !slices.ContainsFunc(c.Lines, func(l ClaimLine) bool { return l.ID == ch.LineID }) {
			return nil, apperr.Validation(map[string]string{"lines": "Baris pengajuan tidak ditemukan"})
		}
		if ch.Type != "" && !ch.Type.Valid() {
			return nil, apperr.Validation(map[string]string{"lines": "Jenis PPh tidak dikenal"})
		}
		byLine[ch.LineID] = ch
	}
	var groups []TaxGroup
	for _, l := range c.Lines {
		ch := byLine[l.ID]
		t := ch.Type
		if t == "" {
			t = defaults[l.ExpenseTypeID]
		}
		if t == "" || t == WithholdingNone {
			continue
		}
		obj := strings.TrimSpace(ch.ObjectCode)
		i := slices.IndexFunc(groups, func(g TaxGroup) bool { return g.Type == t && g.ObjectCode == obj })
		if i < 0 {
			groups = append(groups, TaxGroup{Type: t, ObjectCode: obj, Gross: money.Zero})
			i = len(groups) - 1
		}
		groups[i].LineIDs = append(groups[i].LineIDs, l.ID)
		groups[i].Gross = groups[i].Gross.Add(l.Amount)
	}
	return groups, nil
}

// TaxResult adalah hasil kalkulator tax untuk satu kelompok.
type TaxResult struct {
	Group       TaxGroup
	ObjectCode  string
	RatePercent decimal.Decimal
	Amount      money.Amount
}

// Verify mencatat verifikasi F&A (bukti + PPh): potongan kelompok dialokasikan ke baris sesuai bruto, total
// potongan dan nilai yang dibayar dihitung ulang. Jurnal A-15 diposting service di transaksi yang sama.
func (c *ExpenseClaim) Verify(results []TaxResult, date clock.BusinessDate, at time.Time, by uuid.UUID) error {
	if c.Status != StatusApproved {
		return invalidTransition("Pengajuan biaya", c.Status, StatusAwaitingPayment)
	}
	for i := range c.Lines {
		c.Lines[i].WithholdingType, c.Lines[i].ObjectCode = WithholdingNone, ""
		c.Lines[i].WithholdingRate, c.Lines[i].WithholdingAmount = decimal.Zero, money.Zero
	}
	total := money.Zero
	for _, r := range results {
		idx := make([]int, 0, len(r.Group.LineIDs))
		weights := make([]decimal.Decimal, 0, len(r.Group.LineIDs))
		for _, id := range r.Group.LineIDs {
			i := slices.IndexFunc(c.Lines, func(l ClaimLine) bool { return l.ID == id })
			if i < 0 {
				return apperr.Validation(map[string]string{"lines": "Baris pengajuan tidak ditemukan"})
			}
			idx = append(idx, i)
			weights = append(weights, c.Lines[i].Amount.Decimal())
		}
		parts, err := money.Allocate(r.Amount, weights)
		if err != nil {
			return fmt.Errorf("alokasi PPh: %w", err)
		}
		for k, i := range idx {
			c.Lines[i].WithholdingType, c.Lines[i].ObjectCode = r.Group.Type, r.ObjectCode
			c.Lines[i].WithholdingRate, c.Lines[i].WithholdingAmount = r.RatePercent, parts[k]
		}
		total = total.Add(r.Amount)
	}
	if total.Cmp(c.Total) >= 0 {
		return apperr.Validation(map[string]string{"lines": "Potongan PPh tidak boleh sama atau melebihi total biaya"})
	}
	c.Withholding, c.Payable = total, c.Total.Sub(total)
	c.Status, c.VerifiedAt, c.VerifiedBy, c.VerifyDate = StatusAwaitingPayment, &at, &by, &date
	return nil
}

// PostingSplits menyusun nilai bernama A-15: beban per akun jenis biaya, utang biaya (2103) sebesar nilai
// dibayar, dan utang PPh per jenis potongan.
func (c ExpenseClaim) PostingSplits() ([]PostingSplit, map[string]money.Amount) {
	var splits []PostingSplit
	addSplit := func(key, account string, amount money.Amount, memo string) {
		if amount.IsZero() {
			return
		}
		for i := range splits {
			if splits[i].AmountKey == key && splits[i].AccountCode == account {
				splits[i].Amount = splits[i].Amount.Add(amount)
				return
			}
		}
		splits = append(splits, PostingSplit{AmountKey: key, AccountCode: account, Amount: amount, Memo: memo})
	}
	for _, l := range c.Lines {
		addSplit(AmountExpense, l.ExpenseAccountCode, l.Amount, l.ExpenseTypeName)
	}
	for _, l := range c.Lines {
		addSplit(AmountWithholding, WithholdingAccountCode(l.WithholdingType), l.WithholdingAmount, l.WithholdingType.Label())
	}
	return splits, map[string]money.Amount{AmountPayable: c.Payable}
}

// MarkPaid menandai pengajuan lunas oleh pembayaran.
func (c *ExpenseClaim) MarkPaid(paymentID uuid.UUID, number string, at time.Time) error {
	if err := c.transition(StatusPaid, StatusAwaitingPayment); err != nil {
		return err
	}
	c.PaymentID, c.PaymentNumber, c.PaidAt = &paymentID, number, &at
	return nil
}

// Label adalah nomor pengajuan atau label draft.
func (c ExpenseClaim) Label() string {
	if c.Number != "" {
		return c.Number
	}
	return "Draft pengajuan biaya"
}

// Actions mengembalikan aksi pengajuan biaya sesuai status (sebelum filter permission).
func (c ExpenseClaim) Actions() []Action {
	switch c.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionSubmit, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise, ActionCancel}
	case StatusApproved:
		return []Action{ActionVerify}
	case StatusAwaitingPayment:
		return []Action{ActionPay}
	}
	return nil
}
