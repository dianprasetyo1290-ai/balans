package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
)

// Permission impor/ekspor COA (Finance dan Head F&A).
const (
	PermAccountImport = "accounting.account.import"
	PermAccountExport = "accounting.account.export"
)

var normalBalanceOptions = []tabular.Option{
	{Value: string(domain.NormalDebit), Label: "Debit"},
	{Value: string(domain.NormalCredit), Label: "Kredit"},
}

// AccountColumns adalah kolom template Chart of Accounts (ACC-01).
var AccountColumns = []tabular.Column{
	{Key: "code", Header: "Kode Akun", Kind: tabular.KindText, Required: true, Identity: true, Example: "1101",
		Note: "4 digit, digit pertama = kelompok 1–7. Akun dengan kode yang sudah ada diperbarui."},
	{Key: "name", Header: "Nama Akun", Kind: tabular.KindText, Required: true, Example: "Kas Kecil"},
	{Key: "parentCode", Header: "Kode Akun Induk", Kind: tabular.KindText, Example: "1100",
		Note: "Akun induk harus sudah ada atau berada di baris sebelumnya."},
	{Key: "normalBalance", Header: "Saldo Normal", Kind: tabular.KindChoice, Options: normalBalanceOptions, Example: "Debit",
		Note: "Kosong = saldo normal umum kelompok akun."},
	{Key: "isPostable", Header: "Akun Posting", Kind: tabular.KindBool, Example: tabular.Yes,
		Note: "Tidak = akun induk/judul. Kosong = Ya."},
	{Key: "allowManualPosting", Header: "Boleh Jurnal Umum", Kind: tabular.KindBool, Example: tabular.Yes,
		Note: "Kosong = sama dengan Akun Posting."},
	{Key: "isActive", Header: "Aktif", Kind: tabular.KindBool, Example: tabular.Yes, Note: "Kosong = Ya."},
	{Key: "description", Header: "Keterangan", Kind: tabular.KindText},
}

func accountField(k string) string {
	if k == "parentId" {
		return "parentCode"
	}
	return k
}

func remapAccountFields(err error) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		fields[accountField(k)] = v
	}
	return apperr.Validation(fields)
}

// ImportAccountRow membuat atau memperbarui akun COA dengan kode yang sama (S-125). Aturan sama dengan form
// Tambah/Ubah Akun (S-126): saldo normal dan sifat posting akun yang sudah dipakai dikunci.
func (s *Service) ImportAccountRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermAccountImport); err != nil {
		return err
	}
	p := tabular.Parse(row)
	code := strings.TrimSpace(p.Text("code"))
	postable := p.Bool("isPostable", true)
	in := domain.AccountInput{
		Code: code, Name: p.Text("name"), NormalBalance: domain.NormalBalance(p.Choice("normalBalance", normalBalanceOptions)),
		IsPostable: postable, AllowManualPosting: p.Bool("allowManualPosting", postable), IsActive: p.Bool("isActive", true),
		Description: p.Text("description"),
	}
	if in.NormalBalance == "" {
		in.NormalBalance = domain.GroupOfCode(code).DefaultNormalBalance()
	}
	codes := []string{code}
	parentCode := strings.TrimSpace(p.Text("parentCode"))
	if parentCode != "" {
		codes = append(codes, parentCode)
	}
	found, err := s.Store.AccountsByCodes(ctx, codes)
	if err != nil {
		return err
	}
	if parentCode != "" {
		parent, ok := found[parentCode]
		if !ok {
			p.Fail("parentCode", "Akun induk tidak ditemukan")
		} else {
			in.ParentID = &parent.ID
		}
	}
	if err := p.Err(); err != nil {
		return err
	}
	if cur, ok := found[code]; ok {
		_, err = s.UpdateAccount(ctx, cur.ID, cur.Version, in)
	} else {
		_, err = s.CreateAccount(ctx, in)
	}
	if err != nil {
		return remapAccountFields(err)
	}
	return nil
}

// ExportAccounts mengalirkan seluruh akun (termasuk nonaktif) urut kode.
func (s *Service) ExportAccounts(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermAccountExport); err != nil {
		return err
	}
	list, err := s.Store.ListAccounts(ctx, true)
	if err != nil {
		return err
	}
	codes := make(map[uuid.UUID]string, len(list))
	for _, a := range list {
		codes[a.ID] = a.Code
	}
	for _, a := range list {
		r := tabular.NewRow(0)
		r.Set("code", a.Code).Set("name", a.Name).Set("normalBalance", tabular.Label(normalBalanceOptions, string(a.NormalBalance)))
		if a.ParentID != nil {
			r.Set("parentCode", codes[*a.ParentID])
		}
		r.Set("isPostable", tabular.BoolLabel(a.IsPostable)).Set("allowManualPosting", tabular.BoolLabel(a.AllowManualPosting))
		r.Set("isActive", tabular.BoolLabel(a.IsActive)).Set("description", a.Description)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// Divisions mengembalikan seluruh divisi (termasuk nonaktif) tanpa cek permission; dipakai modul lain untuk
// mencocokkan kode/nama divisi (impor karyawan).
func (s *Service) Divisions(ctx context.Context) ([]domain.DimensionValue, error) {
	return s.Store.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{IncludeInactive: true})
}
