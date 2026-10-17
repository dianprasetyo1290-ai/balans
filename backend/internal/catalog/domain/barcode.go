package domain

import (
	"fmt"
	"strings"
)

// Symbology adalah jenis simbol barcode yang dicetak di label (Desain Dokumen Cetak 2.3).
type Symbology string

const (
	SymbologyEAN13   Symbology = "ean13"
	SymbologyCode128 Symbology = "code128"
)

// MaxBarcodeLength adalah panjang maksimum isi barcode.
const MaxBarcodeLength = 48

// Barcode adalah satu barcode produk. Tepat satu barcode utama per produk yang punya barcode.
type Barcode struct {
	Barcode   string
	Symbology Symbology
	IsPrimary bool
}

// BarcodeInput adalah isian barcode dari form produk.
type BarcodeInput struct {
	Barcode   string
	IsPrimary bool
}

// DetectSymbology menentukan simbol barcode: 13 digit = EAN-13 (digit periksa wajib benar), selain itu Code 128.
// Code 128 hanya menerima karakter ASCII tercetak (spasi sampai ~).
func DetectSymbology(code string) (Symbology, error) {
	if code == "" {
		return "", fmt.Errorf("barcode wajib diisi")
	}
	if len(code) > MaxBarcodeLength {
		return "", fmt.Errorf("barcode maksimal %d karakter", MaxBarcodeLength)
	}
	if len(code) == 13 && isDigits(code) {
		if EAN13CheckDigit(code[:12]) != code[12] {
			return "", fmt.Errorf("digit periksa EAN-13 tidak valid")
		}
		return SymbologyEAN13, nil
	}
	for _, r := range code {
		if r < ' ' || r > '~' {
			return "", fmt.Errorf("barcode hanya boleh huruf, angka, dan tanda baca tanpa aksen")
		}
	}
	return SymbologyCode128, nil
}

// EAN13CheckDigit menghitung digit periksa dari 12 digit pertama EAN-13.
func EAN13CheckDigit(first12 string) byte {
	sum := 0
	for i := range 12 {
		d := int(first12[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// normalizeBarcodes memvalidasi barcode dan memastikan tepat satu barcode utama. Jika hanya ada satu barcode
// tanpa tanda utama, barcode itu otomatis menjadi utama.
func normalizeBarcodes(in []BarcodeInput, fields map[string]string) []Barcode {
	out := make([]Barcode, 0, len(in))
	seen := map[string]bool{}
	primaries := 0
	for i, b := range in {
		code := strings.TrimSpace(b.Barcode)
		key := fmt.Sprintf("barcodes[%d].barcode", i)
		sym, err := DetectSymbology(code)
		switch {
		case err != nil:
			fields[key] = upperFirst(err.Error())
		case seen[code]:
			fields[key] = "Barcode ganda"
		}
		seen[code] = true
		if b.IsPrimary {
			primaries++
		}
		out = append(out, Barcode{Barcode: code, Symbology: sym, IsPrimary: b.IsPrimary})
	}
	switch {
	case len(out) == 1 && primaries == 0:
		out[0].IsPrimary = true
	case len(out) > 0 && primaries != 1:
		fields["barcodes"] = "Pilih tepat satu barcode utama"
	}
	return out
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
