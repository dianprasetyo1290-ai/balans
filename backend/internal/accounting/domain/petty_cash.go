package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// --- Kas kecil imprest (S-405, ACC-12, Desain Status Dokumen 6.2) -----------------------------

// Batas kas kecil. TODO(setting): pindahkan ke settings `petty_cash.max_per_transaction` dan
// `petty_cash.default_ceiling` (effective-dated) saat modul setting dibangun.
var (
	PettyCashMaxPerTransaction = money.FromInt(1_000_000)
	PettyCashDefaultCeiling    = money.FromInt(5_000_000)
)

// Rule dan nilai bernama kas kecil (posting_rule_accounts A-16, A-17).
const (
	RulePettyCashReplenish = "A-16"
	RulePettyCashExpense   = "A-17"
	AmountReplenish        = "replenish_value"
	SourcePettyCash        = "petty_cash_transaction"
)

var fundCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,29}$`)

// PettyCashFund adalah dana kas kecil dengan plafon (imprest) dan saldo berjalan.
type PettyCashFund struct {
	ID           uuid.UUID
	Code         string
	Name         string
	HolderUserID uuid.UUID
	HolderName   string
	DivisionID   uuid.UUID
	DivisionName string
	Ceiling      money.Amount
	Balance      money.Amount
	IsActive     bool
	Description  string
	UpdatedAt    time.Time
	Version      int
}

// FundInput adalah isian Tambah/Ubah Dana Kas Kecil. Code hanya dipakai saat membuat.
type FundInput struct {
	Code         string
	Name         string
	HolderUserID uuid.UUID
	DivisionID   uuid.UUID
	Ceiling      money.Amount
	IsActive     bool
	Description  string
}

func (in *FundInput) normalize() {
	in.Code, in.Name = strings.ToUpper(strings.TrimSpace(in.Code)), strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Ceiling.IsZero() {
		in.Ceiling = PettyCashDefaultCeiling
	}
}

func (in FundInput) validate(withCode bool) map[string]string {
	fields := map[string]string{}
	if withCode && !fundCodePattern.MatchString(in.Code) {
		fields["code"] = "Kode huruf kapital/angka/garis bawah/strip (2–30 karakter), mis. KK-HO"
	}
	if in.Name == "" || len(in.Name) > 150 {
		fields["name"] = "Nama wajib diisi (maks. 150 karakter)"
	}
	if in.HolderUserID == uuid.Nil {
		fields["holderUserId"] = "Pemegang kas kecil wajib dipilih"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib dipilih"
	}
	if msg := validMoney(in.Ceiling); msg != "" {
		fields["ceilingAmount"] = msg
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan maks. 500 karakter"
	}
	return fields
}

// NewFund membuat dana kas kecil bersaldo nol (diisi lewat pengisian kembali).
func NewFund(in FundInput) (PettyCashFund, error) {
	in.normalize()
	if fields := in.validate(true); len(fields) > 0 {
		return PettyCashFund{}, apperr.Validation(fields)
	}
	f := PettyCashFund{Code: in.Code, Balance: money.Zero}
	f.apply(in)
	return f, nil
}

// Update mengubah dana. Plafon tidak boleh di bawah saldo saat ini.
func (f *PettyCashFund) Update(in FundInput) error {
	in.normalize()
	fields := in.validate(false)
	if _, bad := fields["ceilingAmount"]; !bad && in.Ceiling.Cmp(f.Balance) < 0 {
		fields["ceilingAmount"] = "Plafon tidak boleh lebih kecil dari saldo " + f.Balance.Rupiah()
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	f.apply(in)
	return nil
}

func (f *PettyCashFund) apply(in FundInput) {
	f.Name, f.HolderUserID, f.DivisionID, f.Ceiling = in.Name, in.HolderUserID, in.DivisionID, in.Ceiling
	f.IsActive, f.Description = in.IsActive, in.Description
}

// Shortfall adalah nilai pengisian kembali agar saldo kembali ke plafon.
func (f PettyCashFund) Shortfall() money.Amount { return f.Ceiling.Sub(f.Balance) }

// PettyCashType adalah jenis transaksi kas kecil.
type PettyCashType string

const (
	PettyCashExpense       PettyCashType = "expense"
	PettyCashReplenishment PettyCashType = "replenishment"
)

// PettyCashTxn adalah transaksi kas kecil (KK).
type PettyCashTxn struct {
	ID                 uuid.UUID
	Number             string
	FundID             uuid.UUID
	FundCode           string
	FundName           string
	Type               PettyCashType
	BusinessDate       clock.BusinessDate
	ExpenseTypeID      *uuid.UUID
	ExpenseTypeCode    string
	ExpenseTypeName    string
	ExpenseAccountCode string
	DivisionID         uuid.UUID
	DivisionName       string
	CampaignID         *uuid.UUID
	CampaignName       string
	Description        string
	Amount             money.Amount
	ReceiptFileID      *uuid.UUID
	ReceiptFileName    string
	BankAccountID      *uuid.UUID
	BankAccountName    string
	Status             DocStatus
	DecisionNote       string
	JournalEntryID     *uuid.UUID
	JournalNumber      string
	PostedAt           *time.Time
	PostedBy           *uuid.UUID
	CancelReason       string
	CreatedAt          time.Time
	CreatedBy          uuid.UUID
	UpdatedAt          time.Time
	Version            int
}

// PettyCashInput adalah isian transaksi kas kecil.
type PettyCashInput struct {
	Type          PettyCashType
	FundID        uuid.UUID
	BusinessDate  clock.BusinessDate
	ExpenseTypeID *uuid.UUID
	DivisionID    uuid.UUID
	CampaignID    *uuid.UUID
	Description   string
	Amount        money.Amount
	ReceiptFileID *uuid.UUID
}

func (in *PettyCashInput) normalize() {
	in.Description = strings.TrimSpace(in.Description)
	if in.Type == PettyCashReplenishment {
		in.ExpenseTypeID, in.CampaignID, in.ReceiptFileID = nil, nil, nil
	}
}

func (in PettyCashInput) validate() map[string]string {
	fields := map[string]string{}
	if in.Type != PettyCashExpense && in.Type != PettyCashReplenishment {
		fields["type"] = "Jenis transaksi tidak dikenal"
	}
	if in.FundID == uuid.Nil {
		fields["fundId"] = "Dana kas kecil wajib dipilih"
	}
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal wajib diisi"
	}
	if in.Type == PettyCashExpense && in.ExpenseTypeID == nil {
		fields["expenseTypeId"] = "Jenis biaya wajib dipilih"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib dipilih"
	}
	if msg := validMoney(in.Amount); msg != "" {
		fields["amount"] = msg
	}
	if len(in.Description) > 300 {
		fields["description"] = "Keterangan maks. 300 karakter"
	}
	return fields
}

// NewPettyCashTxn membuat draft transaksi kas kecil.
func NewPettyCashTxn(in PettyCashInput) (PettyCashTxn, error) {
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return PettyCashTxn{}, apperr.Validation(fields)
	}
	t := PettyCashTxn{Type: in.Type, FundID: in.FundID, Status: StatusDraft}
	t.apply(in)
	return t, nil
}

// Update mengubah draft. Jenis transaksi dan dana tidak berubah.
func (t *PettyCashTxn) Update(in PettyCashInput) error {
	if t.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya transaksi Draft yang bisa diubah.")
	}
	in.Type, in.FundID = t.Type, t.FundID
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	t.apply(in)
	return nil
}

func (t *PettyCashTxn) apply(in PettyCashInput) {
	if !equalUUIDPtr(t.ExpenseTypeID, in.ExpenseTypeID) {
		t.ExpenseTypeCode, t.ExpenseTypeName, t.ExpenseAccountCode = "", "", ""
	}
	t.BusinessDate, t.ExpenseTypeID, t.DivisionID, t.CampaignID = in.BusinessDate, in.ExpenseTypeID, in.DivisionID, in.CampaignID
	t.Description, t.Amount, t.ReceiptFileID = in.Description, in.Amount, in.ReceiptFileID
}

func equalUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// CheckExpense memeriksa pengeluaran sebelum diposting: dana aktif, bukti wajib, maksimal per transaksi,
// tidak melebihi saldo dana, jenis biaya aktif dan kampanye bila jenis biaya mewajibkan.
func (t PettyCashTxn) CheckExpense(f PettyCashFund, et ExpenseType, maxPerTx money.Amount) error {
	if t.Type != PettyCashExpense {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Bukan transaksi pengeluaran.")
	}
	fields := map[string]string{}
	if !f.IsActive {
		fields["fundId"] = "Dana kas kecil " + f.Code + " tidak aktif"
	}
	if t.ReceiptFileID == nil {
		fields["receiptFileId"] = "Bukti pengeluaran wajib dilampirkan"
	}
	if t.Amount.Cmp(maxPerTx) > 0 {
		fields["amount"] = "Maksimal " + maxPerTx.Rupiah() + " per transaksi kas kecil; gunakan Pengajuan Biaya"
	} else if t.Amount.Cmp(f.Balance) > 0 {
		fields["amount"] = "Saldo kas kecil tidak cukup (saldo " + f.Balance.Rupiah() + ")"
	}
	if msg := et.UsableError(); msg != "" {
		fields["expenseTypeId"] = msg
	}
	if et.RequiresCampaign && t.CampaignID == nil {
		fields["campaignId"] = "Kode kampanye wajib untuk " + et.Name
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CheckReplenishment memeriksa pengisian kembali: dana aktif dan saldo setelah diisi tidak melebihi plafon.
func (t PettyCashTxn) CheckReplenishment(f PettyCashFund) error {
	if t.Type != PettyCashReplenishment {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Bukan transaksi pengisian kembali.")
	}
	fields := map[string]string{}
	if !f.IsActive {
		fields["fundId"] = "Dana kas kecil " + f.Code + " tidak aktif"
	}
	if t.Amount.Cmp(f.Shortfall()) > 0 {
		fields["amount"] = fmt.Sprintf("Pengisian maksimal %s agar saldo tidak melebihi plafon %s",
			f.Shortfall().Rupiah(), f.Ceiling.Rupiah())
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (t *PettyCashTxn) transition(to DocStatus, from ...DocStatus) error {
	if !slices.Contains(from, t.Status) {
		return invalidTransition("Transaksi kas kecil", t.Status, to)
	}
	t.Status = to
	return nil
}

// Submit mengajukan pengisian kembali untuk approval Head F&A.
func (t *PettyCashTxn) Submit() error {
	if t.Type != PettyCashReplenishment {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Pengeluaran kas kecil langsung diposting tanpa approval.")
	}
	return t.transition(StatusPendingApproval, StatusDraft)
}

// Approve dipanggil handler approval.
func (t *PettyCashTxn) Approve(note string) error {
	t.DecisionNote = note
	return t.transition(StatusApproved, StatusPendingApproval)
}

// Reject dipanggil handler approval.
func (t *PettyCashTxn) Reject(note string) error {
	t.DecisionNote = note
	return t.transition(StatusRejected, StatusPendingApproval)
}

// Revise mengembalikan pengisian ditolak menjadi draft.
func (t *PettyCashTxn) Revise() error { return t.transition(StatusDraft, StatusRejected) }

// Post menandai transaksi diposting: pengeluaran dari draft, pengisian dari disetujui.
func (t *PettyCashTxn) Post(number string, journalID uuid.UUID, at time.Time, by uuid.UUID) error {
	from := StatusDraft
	if t.Type == PettyCashReplenishment {
		from = StatusApproved
	}
	if err := t.transition(StatusPosted, from); err != nil {
		return err
	}
	t.Number, t.JournalEntryID, t.PostedAt, t.PostedBy = number, &journalID, &at, &by
	return nil
}

// Cancel membatalkan transaksi yang belum diposting dan tidak sedang menunggu persetujuan.
func (t *PettyCashTxn) Cancel(reason string) error {
	r, err := requireReason(reason, "reason", "pembatalan")
	if err != nil {
		return err
	}
	if err := t.transition(StatusCancelled, StatusDraft, StatusRejected, StatusApproved); err != nil {
		return err
	}
	t.CancelReason = r
	return nil
}

// Label adalah nomor transaksi atau label draft.
func (t PettyCashTxn) Label() string {
	if t.Number != "" {
		return t.Number
	}
	if t.Type == PettyCashReplenishment {
		return "Draft pengisian kas kecil " + t.FundCode
	}
	return "Draft pengeluaran kas kecil " + t.FundCode
}

// Actions mengembalikan aksi transaksi kas kecil sesuai status.
func (t PettyCashTxn) Actions() []Action {
	switch t.Status {
	case StatusDraft:
		if t.Type == PettyCashReplenishment {
			return []Action{ActionEdit, ActionSubmit, ActionCancel}
		}
		return []Action{ActionEdit, ActionPost, ActionCancel}
	case StatusRejected:
		return []Action{ActionRevise, ActionCancel}
	case StatusApproved:
		return []Action{ActionPost, ActionCancel}
	}
	return nil
}
