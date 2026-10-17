// Package domain berisi aturan akuntansi Fase 1: Chart of Accounts (ACC-01), dimensi jurnal (ACC-17),
// periode (ACC-08), jurnal & Balik Jurnal (ACC-13), dan posting rule (ACC-02). Tanpa DB, HTTP, atau jam.
package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
)

// Group adalah kelompok akun = digit pertama kode akun (Desain Finance & Accounting bagian 1).
type Group int

const (
	GroupAsset Group = iota + 1
	GroupLiability
	GroupEquity
	GroupRevenue
	GroupCOGS
	GroupExpense
	GroupOther
)

var groupLabels = map[Group]string{
	GroupAsset: "Aset", GroupLiability: "Liabilitas", GroupEquity: "Ekuitas", GroupRevenue: "Pendapatan",
	GroupCOGS: "Harga Pokok Penjualan", GroupExpense: "Beban Operasional", GroupOther: "Pendapatan & Beban Lain-lain",
}

// Label mengembalikan nama kelompok untuk UI.
func (g Group) Label() string { return groupLabels[g] }

// IsProfitLoss: kelompok 4–7 masuk laba rugi.
func (g Group) IsProfitLoss() bool { return g >= GroupRevenue && g <= GroupOther }

// DefaultNormalBalance adalah saldo normal umum kelompok; akun kontra (mis. akumulasi penyusutan) diubah manual.
func (g Group) DefaultNormalBalance() NormalBalance {
	switch g {
	case GroupLiability, GroupEquity, GroupRevenue:
		return NormalCredit
	default:
		return NormalDebit
	}
}

// NormalBalance adalah saldo normal akun.
type NormalBalance string

const (
	NormalDebit  NormalBalance = "debit"
	NormalCredit NormalBalance = "credit"
)

// Valid memeriksa nilai saldo normal.
func (n NormalBalance) Valid() bool { return n == NormalDebit || n == NormalCredit }

var accountCodePattern = regexp.MustCompile(`^[1-7][0-9]{3}$`)

// Account adalah satu akun COA.
type Account struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	Group              Group
	NormalBalance      NormalBalance
	ParentID           *uuid.UUID
	IsPostable         bool
	AllowManualPosting bool
	IsActive           bool
	Description        string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int
}

// AccountInput adalah isian Tambah/Ubah Akun. Code hanya dipakai saat membuat akun (kode tidak berubah).
type AccountInput struct {
	Code               string
	Name               string
	NormalBalance      NormalBalance
	ParentID           *uuid.UUID
	IsPostable         bool
	AllowManualPosting bool
	IsActive           bool
	Description        string
}

// GroupOfCode mengembalikan kelompok dari digit pertama kode.
func GroupOfCode(code string) Group {
	if code == "" {
		return 0
	}
	return Group(code[0] - '0')
}

func (in *AccountInput) normalize() {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
}

func (in AccountInput) validate(fields map[string]string) {
	if in.Name == "" {
		fields["name"] = "Nama akun wajib diisi"
	} else if len(in.Name) > 150 {
		fields["name"] = "Nama akun paling panjang 150 karakter"
	}
	if !in.NormalBalance.Valid() {
		fields["normalBalance"] = "Saldo normal harus debit atau kredit"
	}
	if in.AllowManualPosting && !in.IsPostable {
		fields["allowManualPosting"] = "Akun induk tidak bisa dipakai di Jurnal Umum"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan paling panjang 500 karakter"
	}
}

// NewAccount membuat akun baru. Kode 4 digit dengan digit pertama = kelompok 1–7.
func NewAccount(in AccountInput) (Account, error) {
	in.normalize()
	fields := map[string]string{}
	if !accountCodePattern.MatchString(in.Code) {
		fields["code"] = "Kode akun 4 digit, diawali kelompok 1–7"
	}
	in.validate(fields)
	if len(fields) > 0 {
		return Account{}, apperr.Validation(fields)
	}
	return Account{
		Code: in.Code, Name: in.Name, Group: GroupOfCode(in.Code), NormalBalance: in.NormalBalance, ParentID: in.ParentID,
		IsPostable: in.IsPostable, AllowManualPosting: in.AllowManualPosting, IsActive: in.IsActive, Description: in.Description,
	}, nil
}

// AccountUsage adalah pemakaian akun yang membatasi perubahan dan penghapusan (S-126).
type AccountUsage struct {
	JournalLines int64
	Children     int64
	RuleMappings int64
}

// Used: akun sudah dipakai jurnal atau posting rule.
func (u AccountUsage) Used() bool { return u.JournalLines > 0 || u.RuleMappings > 0 }

// Update menerapkan isian Ubah Akun. Saldo normal dan sifat posting akun yang sudah dipakai tidak bisa
// diubah karena memengaruhi saldo dan jurnal yang ada; akun yang punya sub-akun harus tetap akun induk.
func (a *Account) Update(in AccountInput, usage AccountUsage) error {
	in.normalize()
	fields := map[string]string{}
	in.validate(fields)
	if usage.JournalLines > 0 && in.NormalBalance != a.NormalBalance {
		fields["normalBalance"] = "Saldo normal akun yang sudah dipakai jurnal tidak bisa diubah"
	}
	if in.IsPostable != a.IsPostable {
		switch {
		case in.IsPostable && usage.Children > 0:
			fields["isPostable"] = "Akun yang punya sub-akun harus tetap akun induk"
		case !in.IsPostable && usage.Used():
			fields["isPostable"] = "Akun yang sudah dipakai jurnal atau posting rule tidak bisa dijadikan akun induk"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	a.Name, a.NormalBalance, a.ParentID, a.IsPostable = in.Name, in.NormalBalance, in.ParentID, in.IsPostable
	a.AllowManualPosting, a.IsActive, a.Description = in.AllowManualPosting, in.IsActive, in.Description
	return nil
}

// CheckDelete menolak penghapusan akun yang sudah dipakai atau masih punya sub-akun.
func (a Account) CheckDelete(usage AccountUsage) error {
	switch {
	case usage.JournalLines > 0:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Akun "+a.Code+" sudah dipakai jurnal dan tidak bisa dihapus. Nonaktifkan akun bila tidak dipakai lagi.")
	case usage.RuleMappings > 0:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Akun "+a.Code+" dipakai posting rule dan tidak bisa dihapus.")
	case usage.Children > 0:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Akun "+a.Code+" masih punya sub-akun.")
	}
	return nil
}

// ValidateParent memeriksa akun induk: harus akun induk (tidak bisa diposting), satu kelompok, dan bukan
// akun itu sendiri atau turunannya. ancestors adalah rantai induk dari parent ke atas.
func ValidateParent(child Account, parent Account, ancestors []uuid.UUID) error {
	fields := map[string]string{}
	switch {
	case parent.ID == child.ID && child.ID != uuid.Nil:
		fields["parentId"] = "Akun tidak bisa menjadi induk dirinya sendiri"
	case parent.IsPostable:
		fields["parentId"] = "Induk harus akun judul yang tidak bisa diposting"
	case parent.Group != child.Group:
		fields["parentId"] = "Induk harus satu kelompok dengan akun"
	}
	for _, id := range ancestors {
		if child.ID != uuid.Nil && id == child.ID {
			fields["parentId"] = "Induk tidak boleh sub-akun dari akun ini"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Balance menghitung saldo sesuai saldo normal: debit − kredit untuk akun bersaldo debit, sebaliknya kredit − debit.
func (n NormalBalance) Balance(debit, credit money.Amount) money.Amount {
	if n == NormalCredit {
		return credit.Sub(debit)
	}
	return debit.Sub(credit)
}

// ManualPostingError mengembalikan alasan akun tidak boleh dipakai di Jurnal Umum manual, atau "".
// Akun kontrol (piutang, utang, persediaan) hanya diisi jurnal otomatis (Desain Finance & Accounting bagian 1).
func (a Account) ManualPostingError() string {
	switch {
	case !a.IsActive:
		return "Akun " + a.Code + " tidak aktif"
	case !a.IsPostable:
		return "Akun induk " + a.Code + " tidak bisa diposting"
	case !a.AllowManualPosting:
		return "Akun " + a.Code + " hanya diisi jurnal otomatis"
	}
	return ""
}

// AutoPostingError mengembalikan alasan akun tidak boleh dipakai jurnal otomatis, atau "".
func (a Account) AutoPostingError() string {
	switch {
	case !a.IsActive:
		return "Akun " + a.Code + " tidak aktif"
	case !a.IsPostable:
		return "Akun induk " + a.Code + " tidak bisa diposting"
	}
	return ""
}

// ManualRequiredDimensions adalah dimensi wajib baris Jurnal Umum manual per kelompok akun
// (Desain Finance & Accounting 3.4): pendapatan wajib channel + sub-channel, HPP/beban/lain-lain wajib divisi.
func (a Account) ManualRequiredDimensions() []Dimension {
	switch a.Group {
	case GroupRevenue:
		return []Dimension{DimensionChannel, DimensionSubChannel}
	case GroupCOGS, GroupExpense, GroupOther:
		return []Dimension{DimensionDivision}
	}
	return nil
}
