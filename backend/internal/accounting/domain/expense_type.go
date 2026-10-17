package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// WithholdingType adalah jenis PPh yang dipotong saat biaya dibayar (Desain Finance & Accounting 4.2).
// Tarif tidak disimpan di sini; nilainya di withholding_tax_rates (effective-dated, S-410).
type WithholdingType string

const (
	WithholdingNone  WithholdingType = "none"
	WithholdingPPh21 WithholdingType = "pph21"
	WithholdingPPh23 WithholdingType = "pph23"
	WithholdingPPh42 WithholdingType = "pph4_2"
)

var withholdingLabels = map[WithholdingType]string{
	WithholdingNone: "Tidak dipotong", WithholdingPPh21: "PPh 21", WithholdingPPh23: "PPh 23", WithholdingPPh42: "PPh 4(2)",
}

// Label mengembalikan nama jenis PPh untuk UI.
func (w WithholdingType) Label() string { return withholdingLabels[w] }

// Valid memeriksa nilai jenis PPh.
func (w WithholdingType) Valid() bool {
	_, ok := withholdingLabels[w]
	return ok
}

// Jenis dokumen approval yang boleh dipakai jenis biaya (Desain Role & Approval bagian 5). Nilainya sama
// dengan approval.SubjectType; accounting tidak mengimpor approval (tier lebih tinggi).
const (
	ApprovalSubjectExpenseClaim           = "expense_claim"
	ApprovalSubjectPettyCashReplenishment = "petty_cash_replenishment"
)

var approvalSubjectLabels = map[string]string{
	ApprovalSubjectExpenseClaim:           "Pengajuan biaya / reimbursement",
	ApprovalSubjectPettyCashReplenishment: "Pengisian kembali kas kecil",
}

// ApprovalSubjectLabel mengembalikan nama jenis dokumen approval untuk UI.
func ApprovalSubjectLabel(s string) string { return approvalSubjectLabels[s] }

var expenseTypeCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,29}$`)

// ExpenseType adalah master jenis biaya (MD-13, ACC-11).
type ExpenseType struct {
	ID   uuid.UUID
	Code string
	Name string
	// ExpenseAccountID adalah akun beban yang didebit (resolver expense_type_account, A-15/A-17).
	ExpenseAccountID uuid.UUID
	// ExpenseAccountCode/Name: hanya untuk tampilan, diisi store.
	ExpenseAccountCode string
	ExpenseAccountName string
	WithholdingType    WithholdingType
	// RequiresReceipt: baris biaya wajib melampirkan bukti.
	RequiresReceipt bool
	// RequiresCampaign: jurnal biaya wajib dimensi kampanye (Desain Finance & Accounting 3.4).
	RequiresCampaign bool
	// ApprovalSubject adalah subject_type approval yang dipakai dokumen biaya dengan jenis ini.
	ApprovalSubject string
	Description     string
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// ExpenseTypeInput adalah isian Tambah/Ubah Jenis Biaya. Code hanya dipakai saat membuat (kode tidak berubah).
// ApprovalSubject kosong = expense_claim; WithholdingType kosong = none.
type ExpenseTypeInput struct {
	Code             string
	Name             string
	ExpenseAccountID uuid.UUID
	WithholdingType  WithholdingType
	RequiresReceipt  bool
	RequiresCampaign bool
	ApprovalSubject  string
	Description      string
	IsActive         bool
}

func (in *ExpenseTypeInput) normalize() {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.ApprovalSubject = strings.TrimSpace(in.ApprovalSubject)
	if in.ApprovalSubject == "" {
		in.ApprovalSubject = ApprovalSubjectExpenseClaim
	}
	if in.WithholdingType == "" {
		in.WithholdingType = WithholdingNone
	}
}

func (in ExpenseTypeInput) validate(withCode bool) map[string]string {
	fields := map[string]string{}
	if withCode && !expenseTypeCodePattern.MatchString(in.Code) {
		fields["code"] = "Kode huruf kapital, angka, atau garis bawah (2–30 karakter), mis. OFFICE_SUPPLY"
	}
	if in.Name == "" {
		fields["name"] = "Nama wajib diisi"
	} else if len(in.Name) > 150 {
		fields["name"] = "Nama paling panjang 150 karakter"
	}
	if in.ExpenseAccountID == uuid.Nil {
		fields["expenseAccountId"] = "Akun beban wajib dipilih"
	}
	if !in.WithholdingType.Valid() {
		fields["withholdingType"] = "Jenis PPh tidak dikenal"
	}
	if _, ok := approvalSubjectLabels[in.ApprovalSubject]; !ok {
		fields["approvalSubject"] = "Jenis dokumen approval tidak dikenal"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan paling panjang 500 karakter"
	}
	return fields
}

// NewExpenseType membuat jenis biaya baru. Akun beban diperiksa terpisah dengan ValidateExpenseAccount.
func NewExpenseType(in ExpenseTypeInput) (ExpenseType, error) {
	in.normalize()
	if fields := in.validate(true); len(fields) > 0 {
		return ExpenseType{}, apperr.Validation(fields)
	}
	e := ExpenseType{Code: in.Code}
	e.apply(in)
	return e, nil
}

// Update mengubah jenis biaya. Kode tidak berubah (dipakai dokumen dan laporan).
func (e *ExpenseType) Update(in ExpenseTypeInput) error {
	in.normalize()
	if fields := in.validate(false); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	e.apply(in)
	return nil
}

func (e *ExpenseType) apply(in ExpenseTypeInput) {
	if e.ExpenseAccountID != in.ExpenseAccountID {
		e.ExpenseAccountCode, e.ExpenseAccountName = "", ""
	}
	e.Name, e.ExpenseAccountID, e.WithholdingType = in.Name, in.ExpenseAccountID, in.WithholdingType
	e.RequiresReceipt, e.RequiresCampaign, e.ApprovalSubject = in.RequiresReceipt, in.RequiresCampaign, in.ApprovalSubject
	e.Description, e.IsActive = in.Description, in.IsActive
}

// UsableError mengembalikan alasan jenis biaya tidak boleh dipakai dokumen baru, atau "".
func (e ExpenseType) UsableError() string {
	if !e.IsActive {
		return "Jenis biaya " + e.Code + " tidak aktif"
	}
	return ""
}

// ValidateExpenseAccount memeriksa akun beban jenis biaya: akun beban operasional (6xxx) atau beban lain-lain
// (7xxx bersaldo debit) yang aktif dan bisa diposting.
func ValidateExpenseAccount(a Account) error {
	var msg string
	switch {
	case a.Group != GroupExpense && a.Group != GroupOther, a.NormalBalance != NormalDebit:
		msg = "Akun " + a.Code + " bukan akun beban (6xxx atau beban lain-lain 7xxx)"
	case !a.IsPostable:
		msg = "Akun induk " + a.Code + " tidak bisa diposting"
	case !a.IsActive:
		msg = "Akun " + a.Code + " tidak aktif"
	default:
		return nil
	}
	return apperr.Validation(map[string]string{"expenseAccountId": msg})
}
