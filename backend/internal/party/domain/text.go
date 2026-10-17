// Package domain berisi master pelanggan, mitra & PKS, kode produk mitra, dan pemasok & maklon beserta
// validasinya (BRD MD-06, MD-07, MD-08). Tanpa DB, HTTP, atau waktu sistem.
package domain

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fieldErrors mengumpulkan pesan validasi per field (JSON camelCase).
type fieldErrors map[string]string

func (f fieldErrors) add(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

// text merapikan spasi dan memeriksa panjang maksimum.
func (f fieldErrors) text(field, s string, maxLen int, required bool, label string) string {
	s = strings.TrimSpace(s)
	switch {
	case required && s == "":
		f.add(field, label+" wajib diisi")
	case utf8.RuneCountInString(s) > maxLen:
		f.add(field, fmt.Sprintf("%s maksimal %d karakter", label, maxLen))
	}
	return s
}

func (f fieldErrors) email(field, s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || utf8.RuneCountInString(s) > 120 {
		f.add(field, "Alamat email tidak valid")
	}
	return s
}

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

// code menormalkan kode bisnis (kode mitra, kode pemasok) ke huruf besar.
func (f fieldErrors) code(field, s string, label string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch {
	case s == "":
		f.add(field, label+" wajib diisi")
	case len(s) > 30:
		f.add(field, label+" maksimal 30 karakter")
	case !codePattern.MatchString(s):
		f.add(field, label+" hanya huruf, angka, dan tanda minus")
	}
	return s
}

// digitsOnly membuang spasi, titik, minus, dan garis miring yang lazim di NPWP/rekening.
func digitsOnly(s string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsDigit(r) && r < unicode.MaxASCII:
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '-' || r == '/':
		default:
			return "", false
		}
	}
	return b.String(), true
}

// NormalizeNPWP menerima NPWP 15 digit (format lama) atau 16 digit (NIK/NPWP baru) dengan atau tanpa tanda baca.
func NormalizeNPWP(s string) (string, bool) {
	d, ok := digitsOnly(s)
	if !ok || (len(d) != 15 && len(d) != 16) {
		return "", false
	}
	return d, true
}

// NormalizeAccountNumber menerima nomor rekening 5 – 20 digit.
func NormalizeAccountNumber(s string) (string, bool) {
	d, ok := digitsOnly(s)
	if !ok || len(d) < 5 || len(d) > 20 {
		return "", false
	}
	return d, true
}
