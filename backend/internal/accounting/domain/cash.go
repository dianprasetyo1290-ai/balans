package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// DocStatus adalah status dokumen keuangan R1.4 (pembayaran, pengajuan biaya, kas kecil). Kode mengikuti
// Desain Status Dokumen bagian 1.
type DocStatus string

const (
	StatusDraft           DocStatus = "draft"
	StatusPendingApproval DocStatus = "pending_approval"
	StatusApproved        DocStatus = "approved"
	StatusRejected        DocStatus = "rejected"
	StatusAwaitingPayment DocStatus = "awaiting_payment"
	StatusPaid            DocStatus = "paid"
	StatusPosted          DocStatus = "posted"
	StatusCancelled       DocStatus = "cancelled"
)

var docStatusLabels = map[DocStatus]string{
	StatusDraft: "Draft", StatusPendingApproval: "Menunggu Persetujuan", StatusApproved: "Disetujui",
	StatusRejected: "Ditolak", StatusAwaitingPayment: "Menunggu Pembayaran", StatusPaid: "Lunas",
	StatusPosted: "Diposting", StatusCancelled: "Dibatalkan",
}

// Label mengembalikan label UI status.
func (s DocStatus) Label() string {
	if l, ok := docStatusLabels[s]; ok {
		return l
	}
	return string(s)
}

// Aksi dokumen keuangan selain aksi jurnal (allowedActions).
const (
	ActionSubmit Action = "submit"
	ActionCancel Action = "cancel"
	ActionRevise Action = "revise"
	ActionVerify Action = "verify"
	ActionPay    Action = "pay"
)

func invalidTransition(doc string, from, to DocStatus) error {
	return apperr.WithMessage(apperr.CodeInvalidStateTransition,
		fmt.Sprintf("%s berstatus %s tidak bisa diubah menjadi %s.", doc, from.Label(), to.Label()))
}

// validMoney memeriksa nominal positif dengan maks. 2 desimal.
func validMoney(a money.Amount) string {
	switch {
	case !a.IsPositive():
		return "Nominal harus lebih dari nol"
	case !a.Equal(a.Round()):
		return "Nominal maksimal 2 angka di belakang koma"
	case len(a.Decimal().Truncate(0).String()) > 16:
		return "Nominal terlalu besar"
	}
	return ""
}

// MaxReasonLength membatasi alasan batal/keputusan.
const MaxReasonLength = 400

func requireReason(reason, field, what string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", apperr.Validation(map[string]string{field: "Alasan " + what + " wajib diisi"})
	case len([]rune(reason)) > MaxReasonLength:
		return "", apperr.Validation(map[string]string{field: "Alasan maksimal 400 karakter"})
	}
	return reason, nil
}

// --- Rekening kas & bank (S-401) ---------------------------------------------------------------

// BankAccountKind adalah jenis rekening: bank atau kas tunai.
type BankAccountKind string

const (
	BankKindBank BankAccountKind = "bank"
	BankKindCash BankAccountKind = "cash"
)

// Kode akun Kas & Bank (Desain Finance & Accounting 1).
const (
	CashBankPrefix   = "11"
	PettyCashAccount = "1102"
)

var bankCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,29}$`)

// BankAccount adalah rekening kas/bank Balans yang terhubung ke satu akun Kas & Bank di COA.
type BankAccount struct {
	ID       uuid.UUID
	Code     string
	Name     string
	Kind     BankAccountKind
	BankName string
	// AccountNumber adalah nomor rekening (didekripsi store); kosong untuk kas.
	AccountNumber string
	AccountHolder string
	AccountID     uuid.UUID
	// AccountCode/AccountName: akun GL untuk tampilan dan posting, diisi store.
	AccountCode string
	AccountName string
	IsActive    bool
	Description string
	DeletedAt   *time.Time
	UpdatedAt   time.Time
	Version     int
}

// BankAccountInput adalah isian Tambah/Ubah Rekening. Code hanya dipakai saat membuat.
type BankAccountInput struct {
	Code          string
	Name          string
	Kind          BankAccountKind
	BankName      string
	AccountNumber string
	AccountHolder string
	AccountID     uuid.UUID
	IsActive      bool
	Description   string
}

func (in *BankAccountInput) normalize() {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.BankName = strings.TrimSpace(in.BankName)
	in.AccountNumber = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' || r == '.' {
			return -1
		}
		return r
	}, in.AccountNumber)
	in.AccountHolder = strings.TrimSpace(in.AccountHolder)
	in.Description = strings.TrimSpace(in.Description)
	if in.Kind == "" {
		in.Kind = BankKindBank
	}
	if in.Kind == BankKindCash {
		in.BankName, in.AccountNumber, in.AccountHolder = "", "", ""
	}
}

func (in BankAccountInput) validate(withCode bool) map[string]string {
	fields := map[string]string{}
	if withCode && !bankCodePattern.MatchString(in.Code) {
		fields["code"] = "Kode huruf kapital/angka/garis bawah/strip (2–30 karakter), mis. BCA-OPS"
	}
	if in.Name == "" || len(in.Name) > 150 {
		fields["name"] = "Nama wajib diisi (maks. 150 karakter)"
	}
	if in.Kind != BankKindBank && in.Kind != BankKindCash {
		fields["kind"] = "Jenis rekening tidak dikenal"
	}
	if in.Kind == BankKindBank {
		if in.BankName == "" || len(in.BankName) > 100 {
			fields["bankName"] = "Nama bank wajib diisi"
		}
		if !accountNumberPattern.MatchString(in.AccountNumber) {
			fields["accountNumber"] = "Nomor rekening 5–30 angka"
		}
		if len(in.AccountHolder) > 150 {
			fields["accountHolder"] = "Nama pemilik rekening maks. 150 karakter"
		}
	}
	if in.AccountID == uuid.Nil {
		fields["accountId"] = "Akun Kas & Bank wajib dipilih"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan paling panjang 500 karakter"
	}
	return fields
}

var accountNumberPattern = regexp.MustCompile(`^[0-9]{5,30}$`)

// NewBankAccount membuat rekening baru. Akun GL diperiksa terpisah dengan ValidateCashBankAccount.
func NewBankAccount(in BankAccountInput) (BankAccount, error) {
	in.normalize()
	if fields := in.validate(true); len(fields) > 0 {
		return BankAccount{}, apperr.Validation(fields)
	}
	b := BankAccount{Code: in.Code}
	b.apply(in)
	return b, nil
}

// Update mengubah rekening. Kode tidak berubah.
func (b *BankAccount) Update(in BankAccountInput) error {
	if b.DeletedAt != nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Rekening sudah dihapus.")
	}
	in.normalize()
	if fields := in.validate(false); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	b.apply(in)
	return nil
}

func (b *BankAccount) apply(in BankAccountInput) {
	if b.AccountID != in.AccountID {
		b.AccountCode, b.AccountName = "", ""
	}
	b.Name, b.Kind, b.BankName, b.AccountNumber, b.AccountHolder = in.Name, in.Kind, in.BankName, in.AccountNumber, in.AccountHolder
	b.AccountID, b.IsActive, b.Description = in.AccountID, in.IsActive, in.Description
}

// MaskedNumber mengembalikan nomor rekening tersamar (4 digit terakhir).
func (b BankAccount) MaskedNumber() string { return MaskAccountNumber(b.AccountNumber) }

// MaskAccountNumber menyamarkan nomor rekening menjadi ••••1234.
func MaskAccountNumber(n string) string {
	if n == "" {
		return ""
	}
	if len(n) <= 4 {
		return "••••"
	}
	return "••••" + n[len(n)-4:]
}

// Last4 adalah 4 digit terakhir nomor rekening untuk disimpan tanpa enkripsi.
func (b BankAccount) Last4() string {
	if len(b.AccountNumber) <= 4 {
		return b.AccountNumber
	}
	return b.AccountNumber[len(b.AccountNumber)-4:]
}

// UsableError mengembalikan alasan rekening tidak boleh dipakai transaksi baru, atau "".
func (b BankAccount) UsableError() string {
	switch {
	case b.DeletedAt != nil:
		return "Rekening " + b.Code + " sudah dihapus"
	case !b.IsActive:
		return "Rekening " + b.Code + " tidak aktif"
	}
	return ""
}

// Label adalah nama rekening untuk tampilan dokumen.
func (b BankAccount) Label() string {
	if b.Kind == BankKindCash || b.AccountNumber == "" {
		return b.Name
	}
	return b.Name + " " + b.MaskedNumber()
}

// ValidateCashBankAccount memeriksa akun GL rekening: akun Kas & Bank (11xx) yang bisa diposting dan aktif,
// bukan Kas Kecil (dikelola dana kas kecil).
func ValidateCashBankAccount(a Account) error {
	var msg string
	switch {
	case !strings.HasPrefix(a.Code, CashBankPrefix):
		msg = "Akun " + a.Code + " bukan akun Kas & Bank (11xx)"
	case a.Code == PettyCashAccount:
		msg = "Kas Kecil dikelola lewat dana kas kecil"
	case !a.IsPostable:
		msg = "Akun induk " + a.Code + " tidak bisa diposting"
	case !a.IsActive:
		msg = "Akun " + a.Code + " tidak aktif"
	default:
		return nil
	}
	return apperr.Validation(map[string]string{"accountId": msg})
}

// --- Pembayaran (S-401) -----------------------------------------------------------------------

// PaymentDirection adalah arah pembayaran.
type PaymentDirection string

const (
	PaymentIncoming PaymentDirection = "incoming"
	PaymentOutgoing PaymentDirection = "outgoing"
)

// Valid memeriksa arah pembayaran.
func (d PaymentDirection) Valid() bool { return d == PaymentIncoming || d == PaymentOutgoing }

// Label mengembalikan label UI arah pembayaran.
func (d PaymentDirection) Label() string {
	if d == PaymentIncoming {
		return "Masuk"
	}
	return "Keluar"
}

// Rule dan nilai bernama jurnal pembayaran (posting_rule_accounts A-PB).
const (
	RulePayment        = "A-PB"
	AmountIncoming     = "incoming_value"
	AmountOutgoing     = "outgoing_value"
	SourcePayment      = "payment"
	maxReferenceLength = 100
)

// Payment adalah pembayaran masuk/keluar (BYR).
type Payment struct {
	ID               uuid.UUID
	Number           string
	Direction        PaymentDirection
	BankAccountID    uuid.UUID
	BankAccountCode  string
	BankAccountName  string
	BankGLCode       string
	PaymentDate      clock.BusinessDate
	Amount           money.Amount
	CounterAccountID uuid.UUID
	CounterCode      string
	CounterName      string
	PartyType        string
	PartyID          *uuid.UUID
	PartyName        string
	Reference        string
	Description      string
	ProofFileID      *uuid.UUID
	SourceType       string
	SourceID         *uuid.UUID
	SourceLabel      string
	Status           DocStatus
	JournalEntryID   *uuid.UUID
	JournalNumber    string
	PostedAt         *time.Time
	PostedBy         *uuid.UUID
	CancelReason     string
	// Reconciled: baris jurnal rekening sudah dicocokkan dengan mutasi bank (pembatalan ditolak).
	Reconciled bool
	CreatedAt  time.Time
	CreatedBy  uuid.UUID
	UpdatedAt  time.Time
	Version    int
}

// PaymentInput adalah isian pembayaran manual atau dari modul lain.
type PaymentInput struct {
	Direction        PaymentDirection
	BankAccountID    uuid.UUID
	PaymentDate      clock.BusinessDate
	Amount           money.Amount
	CounterAccountID uuid.UUID
	PartyType        string
	PartyID          *uuid.UUID
	PartyName        string
	Reference        string
	Description      string
	ProofFileID      *uuid.UUID
	SourceType       string
	SourceID         *uuid.UUID
	SourceLabel      string
}

func (in *PaymentInput) normalize() {
	in.PartyType, in.PartyName = strings.TrimSpace(in.PartyType), strings.TrimSpace(in.PartyName)
	in.Reference, in.Description = strings.TrimSpace(in.Reference), strings.TrimSpace(in.Description)
	in.SourceType, in.SourceLabel = strings.TrimSpace(in.SourceType), strings.TrimSpace(in.SourceLabel)
	if in.PartyID == nil {
		in.PartyType = ""
	}
	if in.SourceID == nil {
		in.SourceType, in.SourceLabel = "", ""
	}
}

func (in PaymentInput) validate() map[string]string {
	fields := map[string]string{}
	if !in.Direction.Valid() {
		fields["direction"] = "Arah pembayaran tidak dikenal"
	}
	if in.BankAccountID == uuid.Nil {
		fields["bankAccountId"] = "Rekening wajib dipilih"
	}
	if in.PaymentDate.IsZero() {
		fields["paymentDate"] = "Tanggal bayar wajib diisi"
	}
	if msg := validMoney(in.Amount); msg != "" {
		fields["amount"] = msg
	}
	if in.CounterAccountID == uuid.Nil {
		fields["counterAccountId"] = "Akun lawan wajib dipilih"
	}
	if in.PartyID != nil && in.PartyType == "" {
		fields["partyType"] = "Jenis pihak wajib diisi"
	}
	if len(in.PartyName) > 150 {
		fields["partyName"] = "Nama pihak maks. 150 karakter"
	}
	if len(in.Reference) > maxReferenceLength {
		fields["reference"] = "Referensi maks. 100 karakter"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan maks. 500 karakter"
	}
	return fields
}

// NewPayment membuat draft pembayaran.
func NewPayment(in PaymentInput) (Payment, error) {
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return Payment{}, apperr.Validation(fields)
	}
	p := Payment{Status: StatusDraft}
	p.apply(in)
	return p, nil
}

// Update mengubah draft pembayaran.
func (p *Payment) Update(in PaymentInput) error {
	if p.Status != StatusDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya pembayaran Draft yang bisa diubah.")
	}
	in.normalize()
	if fields := in.validate(); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	p.apply(in)
	return nil
}

func (p *Payment) apply(in PaymentInput) {
	p.Direction, p.BankAccountID, p.PaymentDate, p.Amount = in.Direction, in.BankAccountID, in.PaymentDate, in.Amount
	p.CounterAccountID, p.PartyType, p.PartyID, p.PartyName = in.CounterAccountID, in.PartyType, in.PartyID, in.PartyName
	p.Reference, p.Description, p.ProofFileID = in.Reference, in.Description, in.ProofFileID
	p.SourceType, p.SourceID, p.SourceLabel = in.SourceType, in.SourceID, in.SourceLabel
}

// Post menandai pembayaran diposting dengan nomor BYR dan jurnalnya.
func (p *Payment) Post(number string, journalID uuid.UUID, at time.Time, by uuid.UUID) error {
	if p.Status != StatusDraft {
		return invalidTransition("Pembayaran", p.Status, StatusPosted)
	}
	p.Number, p.JournalEntryID, p.Status, p.PostedAt, p.PostedBy = number, &journalID, StatusPosted, &at, &by
	return nil
}

// Cancel membatalkan draft, atau pembayaran Diposting yang belum direkonsiliasi (jurnal dibalik pemanggil).
func (p *Payment) Cancel(reason string) error {
	if p.Status != StatusDraft && p.Status != StatusPosted {
		return invalidTransition("Pembayaran", p.Status, StatusCancelled)
	}
	if p.Status == StatusPosted && p.Reconciled {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Pembayaran sudah direkonsiliasi dengan mutasi bank; lepaskan pencocokan terlebih dahulu.")
	}
	if p.Status == StatusPosted && p.SourceID != nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Pembayaran dokumen "+p.SourceLabel+" dibatalkan dari dokumen sumbernya.")
	}
	r, err := requireReason(reason, "reason", "pembatalan")
	if err != nil {
		return err
	}
	p.Status, p.CancelReason = StatusCancelled, r
	return nil
}

// Amounts mengembalikan nilai bernama A-PB sesuai arah pembayaran.
func (p Payment) Amounts() map[string]money.Amount {
	if p.Direction == PaymentIncoming {
		return map[string]money.Amount{AmountIncoming: p.Amount}
	}
	return map[string]money.Amount{AmountOutgoing: p.Amount}
}

// Accounts mengembalikan akun resolver A-PB: rekening (bank_account) dan akun lawan (piutang/utang).
func (p Payment) Accounts() map[Resolver]string {
	if p.Direction == PaymentIncoming {
		return map[Resolver]string{ResolverBank: p.BankGLCode, ResolverReceivable: p.CounterCode}
	}
	return map[Resolver]string{ResolverBank: p.BankGLCode, ResolverPayable: p.CounterCode}
}

// Signed mengembalikan nominal bertanda dari sisi rekening: masuk positif, keluar negatif.
func (p Payment) Signed() money.Amount {
	if p.Direction == PaymentOutgoing {
		return p.Amount.Neg()
	}
	return p.Amount
}

// Actions mengembalikan aksi pembayaran sesuai status.
func (p Payment) Actions() []Action {
	switch p.Status {
	case StatusDraft:
		return []Action{ActionEdit, ActionPost, ActionCancel}
	case StatusPosted:
		if p.Reconciled || p.SourceID != nil {
			return nil
		}
		return []Action{ActionCancel}
	}
	return nil
}

// ValidateCounterAccount memeriksa akun lawan pembayaran manual: akun bisa diposting, aktif, boleh dipakai
// jurnal manual (bukan akun kontrol sub-ledger), dan bukan akun rekening itu sendiri.
func ValidateCounterAccount(a Account, bankGL uuid.UUID) error {
	var msg string
	switch {
	case a.ID == bankGL:
		msg = "Akun lawan tidak boleh sama dengan akun rekening"
	case !a.IsPostable:
		msg = "Akun induk " + a.Code + " tidak bisa diposting"
	case !a.IsActive:
		msg = "Akun " + a.Code + " tidak aktif"
	case !a.AllowManualPosting:
		msg = "Akun " + a.Code + " adalah akun kontrol; pembayaran dicatat dari dokumennya (piutang/utang/biaya)"
	default:
		return nil
	}
	return apperr.Validation(map[string]string{"counterAccountId": msg})
}

// --- Mutasi rekening --------------------------------------------------------------------------

// BankMutation adalah satu baris mutasi rekening (baris jurnal akun GL rekening) dengan saldo berjalan.
type BankMutation struct {
	JournalLineID  uuid.UUID
	JournalEntryID uuid.UUID
	JournalNumber  string
	BusinessDate   clock.BusinessDate
	Description    string
	SourceType     string
	SourceLabel    string
	Memo           string
	// Amount bertanda: debit akun rekening (masuk) positif, kredit (keluar) negatif.
	Amount  money.Amount
	Balance money.Amount
	// Reconciled: sudah dicocokkan dengan baris mutasi bank.
	Reconciled bool
}
