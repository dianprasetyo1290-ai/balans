package domain

import (
	"strings"
	"unicode"
)

// NormalizePhone menormalkan nomor HP/telepon Indonesia ke format +62…, sehingga `0812-3456-7890`,
// `62 812 3456 7890`, `+62812…`, dan `812…` dianggap nomor yang sama (BRD 5.8, MD-06).
// Spasi, titik, minus, dan tanda kurung diabaikan. Nomor nasional (tanpa 0) harus 8 – 12 digit.
func NormalizePhone(s string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
		case unicode.IsSpace(r) || r == '-' || r == '.' || r == '(' || r == ')':
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
	case strings.HasPrefix(strings.TrimSpace(s), "+"):
		// Kode negara selain +62 tidak didukung.
		return "", false
	}
	if len(d) < 8 || len(d) > 12 || d[0] == '0' {
		return "", false
	}
	return "+62" + d, true
}

// PhoneSearchDigits mengubah kata kunci pencarian menjadi potongan nomor nasional (tanpa 0/62 di depan)
// bila kata kunci tampak seperti nomor telepon; selain itu ok=false.
func PhoneSearchDigits(q string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(q) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '.':
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
	if len(d) < 4 {
		return "", false
	}
	return d, true
}
