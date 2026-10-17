// Package domain berisi data dasar karyawan Fase 1 (BRD HR-01, HR-03): nomor karyawan permanen, status aktif,
// resign, dan validasi NIK. Tanpa DB, HTTP, atau waktu sistem.
package domain

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Status adalah status kepegawaian.
type Status string

const (
	StatusActive Status = "active"
	// StatusResigned juga dipakai untuk resign terjadwal (tanggal resign di masa depan).
	StatusResigned Status = "resigned"
)

// Employee adalah data dasar karyawan.
type Employee struct {
	ID            uuid.UUID
	Number        string
	FullName      string
	DivisionID    *uuid.UUID
	PositionTitle string
	Status        Status
	JoinDate      clock.BusinessDate
	// ResignDate adalah tanggal efektif berhenti: mulai tanggal ini karyawan tidak lagi aktif.
	ResignDate   *clock.BusinessDate
	ResignReason string
	WorkEmail    string
	Phone        string
	// NIK terdekripsi (16 digit); disimpan terenkripsi dengan hash HMAC untuk pencarian.
	NIK       string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// Input adalah isian karyawan. NIK nil = tidak berubah (saat update), string kosong = dihapus.
type Input struct {
	Number        string
	FullName      string
	DivisionID    *uuid.UUID
	PositionTitle string
	JoinDate      clock.BusinessDate
	WorkEmail     string
	Phone         string
	NIK           *string
}

var numberPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9./-]*$`)

// NormalizeNIK menerima NIK KTP 16 digit dengan atau tanpa spasi/titik.
func NormalizeNIK(s string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '-':
		default:
			return "", false
		}
	}
	if b.Len() != 16 {
		return "", false
	}
	return b.String(), true
}

// NormalizePhone menyimpan nomor HP dengan format +62… (sama dengan master pelanggan).
func NormalizePhone(s string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return "", false
		}
	}
	d := b.String()
	switch {
	case strings.HasPrefix(d, "62"):
		d = d[2:]
	case strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	if len(d) < 8 || len(d) > 12 || d[0] == '0' {
		return "", false
	}
	return "+62" + d, true
}

type fields map[string]string

func (f fields) text(field, s string, maxLen int, required bool, label string) string {
	s = strings.TrimSpace(s)
	switch {
	case required && s == "":
		f[field] = label + " wajib diisi"
	case utf8.RuneCountInString(s) > maxLen:
		f[field] = fmt.Sprintf("%s maksimal %d karakter", label, maxLen)
	}
	return s
}

func (e *Employee) apply(f fields, in Input) error {
	e.FullName = f.text("fullName", in.FullName, 120, true, "Nama lengkap")
	e.PositionTitle = f.text("positionTitle", in.PositionTitle, 80, true, "Jabatan")
	e.DivisionID = in.DivisionID
	if in.JoinDate.IsZero() {
		f["joinDate"] = "Tanggal bergabung wajib diisi"
	} else if e.ResignDate != nil && e.ResignDate.Before(in.JoinDate) {
		f["joinDate"] = "Tanggal bergabung tidak boleh setelah tanggal resign"
	}
	e.JoinDate = in.JoinDate
	e.WorkEmail = strings.ToLower(strings.TrimSpace(in.WorkEmail))
	if e.WorkEmail != "" {
		if addr, err := mail.ParseAddress(e.WorkEmail); err != nil || addr.Address != e.WorkEmail || len(e.WorkEmail) > 120 {
			f["workEmail"] = "Alamat email tidak valid"
		}
	}
	e.Phone = ""
	if in.Phone != "" {
		if n, ok := NormalizePhone(in.Phone); ok {
			e.Phone = n
		} else {
			f["phone"] = "Nomor HP tidak valid"
		}
	}
	if in.NIK != nil {
		if nik := strings.TrimSpace(*in.NIK); nik == "" {
			e.NIK = ""
		} else if n, ok := NormalizeNIK(nik); ok {
			e.NIK = n
		} else {
			f["nik"] = "NIK harus 16 digit"
		}
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// New memvalidasi karyawan baru. Nomor karyawan disimpan huruf besar dan permanen.
func New(in Input) (Employee, error) {
	f := fields{}
	e := Employee{Number: strings.ToUpper(strings.TrimSpace(in.Number)), Status: StatusActive}
	switch {
	case e.Number == "":
		f["employeeNumber"] = "Nomor karyawan wajib diisi"
	case len(e.Number) > 30:
		f["employeeNumber"] = "Nomor karyawan maksimal 30 karakter"
	case !numberPattern.MatchString(e.Number):
		f["employeeNumber"] = "Nomor karyawan hanya huruf, angka, titik, garis miring, dan minus"
	}
	if err := e.apply(f, in); err != nil {
		return Employee{}, err
	}
	return e, nil
}

// Update menerapkan perubahan; nomor karyawan tetap.
func (e *Employee) Update(in Input) error { return e.apply(fields{}, in) }

// Resign mencatat resign mulai tanggal efektif. Tanggal boleh di masa depan (resign terjadwal).
func (e *Employee) Resign(date clock.BusinessDate, reason string) error {
	if e.Status == StatusResigned {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Resign karyawan ini sudah tercatat.")
	}
	f := fields{}
	switch {
	case date.IsZero():
		f["resignDate"] = "Tanggal resign wajib diisi"
	case date.Before(e.JoinDate):
		f["resignDate"] = "Tanggal resign tidak boleh sebelum tanggal bergabung"
	}
	reason = f.text("reason", reason, 500, true, "Alasan resign")
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	e.Status, e.ResignDate, e.ResignReason = StatusResigned, &date, reason
	return nil
}

// ActiveOn memeriksa apakah karyawan aktif pada tanggal: sudah bergabung dan belum mencapai tanggal resign.
func (e Employee) ActiveOn(d clock.BusinessDate) bool {
	if d.Before(e.JoinDate) {
		return false
	}
	return e.ResignDate == nil || d.Before(*e.ResignDate)
}
