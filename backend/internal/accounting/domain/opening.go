package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Saldo awal migrasi (ACC-14, A-36, U-10).
const (
	SourceOpeningBalance    = "opening_balance"
	RuleOpeningBalance      = "A-36"
	AmountOpeningDebit      = "opening_debit"
	AmountOpeningCredit     = "opening_credit"
	AmountOpeningDifference = "opening_difference"
)

// OpeningStatus adalah status saldo awal (UI: Draft, Diposting).
type OpeningStatus string

const (
	OpeningDraft  OpeningStatus = "draft"
	OpeningPosted OpeningStatus = "posted"
)

// OpeningSet adalah saldo awal per tanggal cut-over. Hanya satu per sistem.
type OpeningSet struct {
	ID             uuid.UUID
	CutoverDate    clock.BusinessDate
	Status         OpeningStatus
	JournalEntryID *uuid.UUID
	JournalNumber  string
	PostedAt       *time.Time
	PostedBy       *uuid.UUID
	UpdatedAt      time.Time
	Version        int
}

// CheckEditable menolak perubahan saldo awal yang sudah diposting.
func (s OpeningSet) CheckEditable() error {
	if s.Status != OpeningDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Saldo awal sudah diposting dan tidak bisa diubah. Koreksi lewat Jurnal Umum.")
	}
	return nil
}

// ValidateCutoverDate memastikan tanggal cut-over adalah akhir bulan (akhir periode akuntansi, U-10).
func ValidateCutoverDate(d clock.BusinessDate) error {
	if d.IsZero() {
		return apperr.Validation(map[string]string{"cutoverDate": "Tanggal cut-over wajib diisi"})
	}
	if d.AddDays(1).Day() != 1 {
		return apperr.Validation(map[string]string{"cutoverDate": "Tanggal cut-over harus tanggal terakhir bulan (akhir periode akuntansi)"})
	}
	return nil
}

// OpeningLine adalah saldo awal satu akun neraca.
type OpeningLine struct {
	ID          uuid.UUID
	SetID       uuid.UUID
	AccountID   uuid.UUID
	AccountCode string
	AccountName string
	Debit       money.Amount
	Credit      money.Amount
	Note        string
	UpdatedAt   time.Time
	Version     int
}

// OpeningLineInput adalah isian satu baris Neraca Saldo Awal.
type OpeningLineInput struct {
	AccountID uuid.UUID
	Debit     money.Amount
	Credit    money.Amount
	Note      string
}

// NewOpeningLine memvalidasi baris: akun neraca (kelompok 1–3) yang bisa diposting, debit XOR kredit.
func NewOpeningLine(in OpeningLineInput, acc Account) (OpeningLine, error) {
	fields := map[string]string{}
	switch {
	case acc.ID == uuid.Nil:
		fields["accountId"] = "Akun tidak ditemukan"
	case acc.Group.IsProfitLoss():
		fields["accountId"] = "Saldo awal hanya untuk akun neraca (1xxx–3xxx)"
	case acc.AutoPostingError() != "":
		fields["accountId"] = acc.AutoPostingError()
	}
	switch {
	case in.Debit.IsNegative() || in.Credit.IsNegative():
		fields["debit"] = "Nominal tidak boleh negatif"
	case in.Debit.IsPositive() && in.Credit.IsPositive():
		fields["debit"] = "Isi salah satu Debit atau Kredit"
	case in.Debit.IsZero() && in.Credit.IsZero():
		fields["debit"] = "Isi nominal Debit atau Kredit"
	case !isWholeCents(in.Debit) || !isWholeCents(in.Credit):
		fields["debit"] = "Nominal paling banyak 2 angka di belakang koma"
	}
	note := strings.TrimSpace(in.Note)
	if len(note) > 500 {
		fields["note"] = "Keterangan paling panjang 500 karakter"
	}
	if len(fields) > 0 {
		return OpeningLine{}, apperr.Validation(fields)
	}
	return OpeningLine{
		AccountID: acc.ID, AccountCode: acc.Code, AccountName: acc.Name, Debit: in.Debit, Credit: in.Credit, Note: note,
	}, nil
}

// OpeningTotals adalah total Neraca Saldo Awal; Difference = debit − kredit (dibukukan ke 3901 saat posting).
type OpeningTotals struct {
	Debit      money.Amount
	Credit     money.Amount
	Difference money.Amount
}

// TotalsOf menjumlahkan baris saldo awal.
func TotalsOf(lines []OpeningLine) OpeningTotals {
	t := OpeningTotals{Debit: money.Zero, Credit: money.Zero}
	for _, l := range lines {
		t.Debit, t.Credit = t.Debit.Add(l.Debit), t.Credit.Add(l.Credit)
	}
	t.Difference = t.Debit.Sub(t.Credit)
	return t
}

// OpeningPosting menyusun PostingRequest A-36: tiap akun lewat split opening_debit/opening_credit, selisih
// debit − kredit ke 3901 (opening_difference di sisi kredit; negatif dibalik ke debit oleh Plan).
func OpeningPosting(set OpeningSet, lines []OpeningLine) (PostingRequest, error) {
	if len(lines) == 0 {
		return PostingRequest{}, apperr.WithMessage(apperr.CodeValidationFailed, "Neraca Saldo Awal masih kosong.")
	}
	req := PostingRequest{
		RuleCode: RuleOpeningBalance, SourceType: SourceOpeningBalance, SourceID: set.ID,
		SourceLabel: "Saldo Awal " + set.CutoverDate.String(), BusinessDate: set.CutoverDate,
		Description: "Saldo awal migrasi per " + set.CutoverDate.String(), Amounts: map[string]money.Amount{},
	}
	for _, l := range lines {
		key, amount := AmountOpeningDebit, l.Debit
		if l.Credit.IsPositive() {
			key, amount = AmountOpeningCredit, l.Credit
		}
		req.Splits = append(req.Splits, PostingSplit{AmountKey: key, AccountCode: l.AccountCode, Amount: amount, Memo: l.Note})
	}
	if diff := TotalsOf(lines).Difference; !diff.IsZero() {
		req.Amounts[AmountOpeningDifference] = diff
	}
	return req, nil
}

// OpeningItemType adalah jenis saldo terbuka: piutang atau utang.
type OpeningItemType string

const (
	OpeningReceivable OpeningItemType = "receivable"
	OpeningPayable    OpeningItemType = "payable"
)

// Jenis pihak saldo terbuka (modul party).
const (
	PartyCustomer = "customer"
	PartyPartner  = "partner"
	PartySupplier = "supplier"
)

// OpeningItem adalah piutang/utang terbuka per dokumen lama per tanggal cut-over. OpenAmount adalah sisa per
// cut-over; Settled adalah pelunasan oleh dokumen pembayaran setelah go-live. Outstanding = OpenAmount − Settled.
type OpeningItem struct {
	ID             uuid.UUID
	SetID          uuid.UUID
	Type           OpeningItemType
	PartyType      string
	PartyID        uuid.UUID
	PartyCode      string
	DocumentNumber string
	DocumentDate   clock.BusinessDate
	DueDate        clock.BusinessDate
	Amount         money.Amount
	OpenAmount     money.Amount
	Settled        money.Amount
	AccountID      uuid.UUID
	AccountCode    string
	ChannelID      *uuid.UUID
	Note           string
	UpdatedAt      time.Time
	Version        int
}

// Outstanding adalah sisa yang belum dilunasi saat ini.
func (i OpeningItem) Outstanding() money.Amount { return i.OpenAmount.Sub(i.Settled) }

// OpeningItemInput adalah isian satu baris Piutang/Utang Terbuka.
type OpeningItemInput struct {
	Type           OpeningItemType
	PartyType      string
	PartyID        uuid.UUID
	PartyCode      string
	DocumentNumber string
	DocumentDate   clock.BusinessDate
	DueDate        clock.BusinessDate
	Amount         money.Amount
	OpenAmount     money.Amount
	ChannelID      *uuid.UUID
	Note           string
}

// NewOpeningItem memvalidasi baris saldo terbuka. acc adalah akun kontrol (piutang 12xx / utang 2101).
func NewOpeningItem(in OpeningItemInput, acc Account, cutover clock.BusinessDate) (OpeningItem, error) {
	fields := map[string]string{}
	switch in.Type {
	case OpeningReceivable:
		if in.PartyType != PartyCustomer && in.PartyType != PartyPartner {
			fields["partyCode"] = "Pihak piutang harus pelanggan atau mitra"
		}
		if acc.ID != uuid.Nil && !strings.HasPrefix(acc.Code, "12") {
			fields["accountCode"] = "Akun piutang harus akun 12xx"
		}
	case OpeningPayable:
		if in.PartyType != PartySupplier {
			fields["partyCode"] = "Pihak utang harus pemasok/maklon"
		}
		if acc.ID != uuid.Nil && acc.Group != GroupLiability {
			fields["accountCode"] = "Akun utang harus akun liabilitas"
		}
	default:
		fields["type"] = "Jenis saldo tidak dikenal"
	}
	if in.PartyID == uuid.Nil && fields["partyCode"] == "" {
		fields["partyCode"] = "Pihak tidak ditemukan"
	}
	if acc.ID == uuid.Nil {
		fields["accountCode"] = "Akun tidak ditemukan"
	} else if msg := acc.AutoPostingError(); msg != "" {
		fields["accountCode"] = msg
	}
	doc := strings.TrimSpace(in.DocumentNumber)
	if doc == "" || len(doc) > 60 {
		fields["documentNumber"] = "Nomor dokumen wajib diisi (maks. 60 karakter)"
	}
	if in.DocumentDate.IsZero() {
		fields["documentDate"] = "Tanggal dokumen wajib diisi"
	} else if !cutover.IsZero() && in.DocumentDate.After(cutover) {
		fields["documentDate"] = "Tanggal dokumen tidak boleh setelah tanggal cut-over " + cutover.String()
	}
	if in.DueDate.IsZero() {
		fields["dueDate"] = "Jatuh tempo wajib diisi"
	} else if !in.DocumentDate.IsZero() && in.DueDate.Before(in.DocumentDate) {
		fields["dueDate"] = "Jatuh tempo tidak boleh sebelum tanggal dokumen"
	}
	if !in.Amount.IsPositive() || !isWholeCents(in.Amount) {
		fields["amount"] = "Nilai dokumen harus lebih dari 0 (maks. 2 desimal)"
	}
	if !in.OpenAmount.IsPositive() || !isWholeCents(in.OpenAmount) {
		fields["openAmount"] = "Sisa belum dibayar harus lebih dari 0 (maks. 2 desimal)"
	} else if in.Amount.IsPositive() && in.OpenAmount.Cmp(in.Amount) > 0 {
		fields["openAmount"] = "Sisa belum dibayar tidak boleh melebihi nilai dokumen"
	}
	if len(in.Note) > 500 {
		fields["note"] = "Keterangan paling panjang 500 karakter"
	}
	if len(fields) > 0 {
		return OpeningItem{}, apperr.Validation(fields)
	}
	return OpeningItem{
		Type: in.Type, PartyType: in.PartyType, PartyID: in.PartyID, PartyCode: strings.TrimSpace(in.PartyCode),
		DocumentNumber: doc, DocumentDate: in.DocumentDate, DueDate: in.DueDate, Amount: in.Amount,
		OpenAmount: in.OpenAmount, Settled: money.Zero, AccountID: acc.ID, AccountCode: acc.Code, ChannelID: in.ChannelID,
		Note: strings.TrimSpace(in.Note),
	}, nil
}

// Settle mencatat pelunasan (positif) atau pembatalan pelunasan (negatif) dari dokumen pembayaran.
func (i *OpeningItem) Settle(amount money.Amount) error {
	next := i.Settled.Add(amount)
	if amount.IsZero() || !isWholeCents(amount) {
		return apperr.Validation(map[string]string{"amount": "Nilai pelunasan tidak valid"})
	}
	if next.IsNegative() || next.Cmp(i.OpenAmount) > 0 {
		return apperr.Validation(map[string]string{
			"amount": "Pelunasan melebihi sisa saldo awal " + i.DocumentNumber + " (" + i.Outstanding().String() + ")",
		})
	}
	i.Settled = next
	return nil
}

// ReconciliationStatus adalah hasil satu cek rekonsiliasi cut-over.
type ReconciliationStatus string

const (
	ReconOK      ReconciliationStatus = "ok"
	ReconFailed  ReconciliationStatus = "failed"
	ReconSkipped ReconciliationStatus = "skipped"
)

// ReconciliationCheck adalah satu baris pengecekan rekonsiliasi cut-over (Panduan Migrasi Data bagian 4):
// nilai sub-ledger dibanding saldo GL akun terkait, toleransi 0.
type ReconciliationCheck struct {
	Code        string
	Label       string
	AccountCode string
	Subledger   money.Amount
	Ledger      money.Amount
	Difference  money.Amount
	Status      ReconciliationStatus
	Note        string
}

// Compare membuat cek dengan selisih sub-ledger − GL.
func Compare(code, label, account string, subledger, ledger money.Amount) ReconciliationCheck {
	c := ReconciliationCheck{
		Code: code, Label: label, AccountCode: account, Subledger: subledger, Ledger: ledger,
		Difference: subledger.Sub(ledger), Status: ReconOK,
	}
	if !c.Difference.IsZero() {
		c.Status = ReconFailed
	}
	return c
}

// ReconciliationPassed: semua cek lolos (cek yang dilewati tidak menggagalkan).
func ReconciliationPassed(checks []ReconciliationCheck) bool {
	for _, c := range checks {
		if c.Status == ReconFailed {
			return false
		}
	}
	return true
}
