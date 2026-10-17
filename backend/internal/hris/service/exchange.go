package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/hris/domain"
	"balans/internal/hris/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
	"balans/internal/platform/tabular"
)

// Permission impor/ekspor karyawan (matriks Desain Role & Approval 3.1: HRGA RWI).
const (
	PermEmployeeImport = "hris.employee.import"
	PermEmployeeExport = "hris.employee.export"
)

// DivisionRef adalah divisi dari master dimensi accounting.
type DivisionRef struct {
	ID   uuid.UUID
	Code string
	Name string
}

// Divisions membaca master divisi (modul accounting) untuk impor/ekspor karyawan. Nil = kolom divisi
// tidak bisa diisi.
type Divisions interface {
	Divisions(ctx context.Context) ([]DivisionRef, error)
}

var statusOptions = []tabular.Option{
	{Value: string(domain.StatusActive), Label: "Aktif"},
	{Value: string(domain.StatusResigned), Label: "Resign"},
}

// defaultResignReason dipakai bila baris resign tidak mengisi alasan.
const defaultResignReason = "Resign (impor Excel)"

// EmployeeColumns adalah kolom template karyawan (HR-01), mengikuti template migrasi sheet Karyawan.
var EmployeeColumns = []tabular.Column{
	{Key: "employeeNumber", Header: "Nomor Karyawan", Kind: tabular.KindText, Required: true, Identity: true, Example: "BLN-0001"},
	{Key: "fullName", Header: "Nama Lengkap", Kind: tabular.KindText, Required: true, Example: "Andi Pratama"},
	{Key: "division", Header: "Divisi", Kind: tabular.KindText, Example: "Logistik", Note: "Kode atau nama divisi di master dimensi."},
	{Key: "positionTitle", Header: "Jabatan", Kind: tabular.KindText, Required: true, Example: "Staf Gudang"},
	{Key: "employmentStatus", Header: "Status", Kind: tabular.KindChoice, Required: true, Options: statusOptions, Example: "Aktif",
		Note: "Resign membutuhkan Tanggal Resign; karyawan yang sudah resign tidak bisa diaktifkan lewat impor."},
	{Key: "joinDate", Header: "Tanggal Masuk", Kind: tabular.KindDate, Required: true, Example: "2024-01-15"},
	{Key: "resignDate", Header: "Tanggal Resign", Kind: tabular.KindDate},
	{Key: "resignReason", Header: "Alasan Resign", Kind: tabular.KindText, Note: "Kosong = " + defaultResignReason + "."},
	{Key: "workEmail", Header: "Email Kantor", Kind: tabular.KindText, Example: "andi@balans.id"},
	{Key: "phone", Header: "Nomor HP", Kind: tabular.KindText, Example: "081234567890"},
	{Key: "nik", Header: "NIK", Kind: tabular.KindText, Sensitive: true, Example: "3273010101900001",
		Note: "16 digit. Nilai tersamar dari file ekspor (•••) berarti tidak diubah."},
}

func employeeField(k string) string {
	switch k {
	case "divisionId":
		return "division"
	case "reason":
		return "resignReason"
	}
	return k
}

func remap(err error) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		fields[employeeField(k)] = v
	}
	return apperr.Validation(fields)
}

func (s *Service) divisionByRef(ctx context.Context, ref string) (*uuid.UUID, bool, error) {
	if ref == "" {
		return nil, true, nil
	}
	if s.Divisions == nil {
		return nil, false, nil
	}
	list, err := s.Divisions.Divisions(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, d := range list {
		if strings.EqualFold(d.Code, ref) || strings.EqualFold(d.Name, ref) {
			id := d.ID
			return &id, true, nil
		}
	}
	return nil, false, nil
}

// ImportEmployeeRow membuat atau memperbarui karyawan dengan nomor karyawan yang sama; status Resign dicatat
// lewat aturan resign (HR-03: pelanggan tertaut kehilangan harga karyawan mulai tanggal resign).
func (s *Service) ImportEmployeeRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermEmployeeImport); err != nil {
		return err
	}
	p := tabular.Parse(row)
	in := domain.Input{
		Number: p.Text("employeeNumber"), FullName: p.Text("fullName"), PositionTitle: p.Text("positionTitle"),
		WorkEmail: p.Text("workEmail"), Phone: p.Text("phone"),
	}
	if d := p.Date("joinDate"); d != nil {
		in.JoinDate = *d
	}
	if v := p.Text("nik"); !strings.Contains(v, "•") {
		in.NIK = &v
	}
	status := domain.Status(p.Choice("employmentStatus", statusOptions))
	resignDate := p.Date("resignDate")
	reason := p.Text("resignReason")
	if reason == "" {
		reason = defaultResignReason
	}
	if status == domain.StatusResigned && resignDate == nil {
		p.Fail("resignDate", "Tanggal resign wajib diisi untuk status Resign")
	}
	division, ok, err := s.divisionByRef(ctx, p.Text("division"))
	if err != nil {
		return err
	}
	if !ok {
		p.Fail("division", "Divisi tidak ditemukan")
	}
	in.DivisionID = division
	if err := p.Err(); err != nil {
		return err
	}
	id, found, err := s.Store.IDByNumber(ctx, strings.ToUpper(in.Number))
	if err != nil {
		return err
	}
	var e domain.Employee
	if !found {
		if in.NIK != nil && *in.NIK == "" {
			in.NIK = nil
		}
		e, err = s.Create(ctx, in)
	} else {
		cur, gerr := s.Store.Get(ctx, id, true)
		if gerr != nil {
			return gerr
		}
		if cur.Status == domain.StatusResigned && status == domain.StatusActive {
			return apperr.Validation(map[string]string{
				"employmentStatus": "Karyawan sudah resign; status tidak bisa dikembalikan ke Aktif lewat impor",
			})
		}
		e, err = s.Update(ctx, cur.ID, cur.Version, in)
	}
	if err != nil {
		return remap(err)
	}
	if status == domain.StatusResigned && e.Status != domain.StatusResigned {
		if _, err := s.Resign(ctx, e.ID, e.Version, *resignDate, reason); err != nil {
			return remap(err)
		}
	}
	return nil
}

// ExportEmployees mengalirkan karyawan urut nomor; NIK ditulis tersamar (TG 6.6).
func (s *Service) ExportEmployees(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermEmployeeExport); err != nil {
		return err
	}
	names := map[uuid.UUID]string{}
	if s.Divisions != nil {
		list, err := s.Divisions.Divisions(ctx)
		if err != nil {
			return err
		}
		for _, d := range list {
			names[d.ID] = d.Name
		}
	}
	page := paging.Page{Number: 1, Size: paging.MaxSize}
	for {
		res, err := s.Store.List(ctx, postgres.Filter{Sort: "employeeNumber"}, page)
		if err != nil {
			return err
		}
		for _, e := range res.Items {
			r := tabular.NewRow(0)
			r.Set("employeeNumber", e.Number).Set("fullName", e.FullName).Set("positionTitle", e.PositionTitle)
			if e.DivisionID != nil {
				r.Set("division", names[*e.DivisionID])
			}
			r.Set("employmentStatus", tabular.Label(statusOptions, string(e.Status))).Set("joinDate", e.JoinDate.String())
			r.Set("resignDate", tabular.FormatDate(e.ResignDate)).Set("resignReason", e.ResignReason)
			r.Set("workEmail", e.WorkEmail).Set("phone", e.Phone)
			if e.NIK != "" {
				r.Set("nik", pii.Mask(e.NIK))
			}
			if err := emit(r); err != nil {
				return err
			}
		}
		if int64(page.Number*page.Size) >= res.Total || len(res.Items) == 0 {
			return nil
		}
		page.Number++
	}
}

// EmployeeIDByNumber dipakai modul lain (impor pelanggan) tanpa cek permission.
func (s *Service) EmployeeIDByNumber(ctx context.Context, number string) (uuid.UUID, bool, error) {
	return s.Store.IDByNumber(ctx, strings.ToUpper(strings.TrimSpace(number)))
}
