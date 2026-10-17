// Package tabular adalah format baris tabel netral untuk impor/ekspor Excel (Technical Guideline 5.15). Modul
// bisnis mendefinisikan kolom dan membaca/menulis baris dengan tipe di sini tanpa bergantung pada modul
// dataexchange maupun library Excel; dataexchange yang membaca dan menulis file xlsx-nya.
package tabular

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Kind menentukan format sel di template dan petunjuk pengisian.
type Kind string

const (
	KindText    Kind = "text"
	KindInteger Kind = "integer"
	KindDecimal Kind = "decimal"
	KindDate    Kind = "date"
	// KindChoice memakai daftar Options (dropdown di template).
	KindChoice Kind = "choice"
	// KindBool adalah pilihan Ya/Tidak.
	KindBool Kind = "bool"
)

// Label Ya/Tidak untuk kolom KindBool.
const (
	Yes = "Ya"
	No  = "Tidak"
)

// Option adalah satu pilihan kolom KindChoice: Value disimpan sistem, Label ditulis di Excel.
type Option struct {
	Value string
	Label string
}

// Column adalah definisi satu kolom template. Key sama dengan nama field API (mis. `sku`) agar error validasi
// modul (apperr.Validation) bisa dipetakan ke kolom.
type Column struct {
	Key    string
	Header string
	Kind   Kind
	// Required: sel kosong ditolak sebelum baris diteruskan ke modul.
	Required bool
	// Identity: kolom kunci bisnis baris (mis. SKU); kombinasi nilainya harus unik di satu file.
	Identity bool
	// Sensitive: data pribadi (NPWP, nomor rekening) disamarkan di preview dan tidak disimpan di tabel impor.
	Sensitive bool
	Options   []Option
	Example   string
	// Note adalah aturan tambahan di sheet Petunjuk.
	Note string
}

// OptionLabels mengembalikan label pilihan kolom (untuk dropdown); KindBool = Ya/Tidak.
func (c Column) OptionLabels() []string {
	if c.Kind == KindBool {
		return []string{Yes, No}
	}
	out := make([]string, len(c.Options))
	for i, o := range c.Options {
		out[i] = o.Label
	}
	return out
}

// FormatHint menjelaskan format isian untuk sheet Petunjuk.
func (c Column) FormatHint() string {
	switch c.Kind {
	case KindInteger:
		return "Angka bulat tanpa titik ribuan"
	case KindDecimal:
		return "Angka; desimal memakai titik, tanpa titik ribuan"
	case KindDate:
		return "Tanggal YYYY-MM-DD"
	case KindChoice, KindBool:
		return "Pilih: " + strings.Join(c.OptionLabels(), ", ")
	}
	return "Teks"
}

// Row adalah satu baris data. Number adalah nomor baris di sheet (header = baris 1). Values berisi teks sel
// yang sudah dirapikan per Key kolom; kolom kosong boleh tidak ada.
type Row struct {
	Number int
	Values map[string]string
}

// NewRow membuat baris kosong.
func NewRow(number int) Row { return Row{Number: number, Values: map[string]string{}} }

// Get membaca nilai kolom.
func (r Row) Get(key string) string { return r.Values[key] }

// Set mengisi nilai kolom (dipakai exporter).
func (r Row) Set(key, value string) Row {
	r.Values[key] = value
	return r
}

// Label mengembalikan label pilihan dari nilai sistem; nilai tak dikenal dikembalikan apa adanya.
func Label(options []Option, value string) string {
	for _, o := range options {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

// BoolLabel menulis Ya/Tidak.
func BoolLabel(b bool) string {
	if b {
		return Yes
	}
	return No
}

// FormatInt menulis angka bulat opsional.
func FormatInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// FormatDecimal menulis angka desimal opsional.
func FormatDecimal(v *decimal.Decimal) string {
	if v == nil {
		return ""
	}
	return v.String()
}

// FormatDate menulis tanggal bisnis opsional.
func FormatDate(v *clock.BusinessDate) string {
	if v == nil || v.IsZero() {
		return ""
	}
	return v.String()
}

// Parser membaca nilai bertipe dari baris dan mengumpulkan error per kolom.
type Parser struct {
	row    Row
	fields map[string]string
}

// Parse membuat Parser untuk satu baris.
func Parse(r Row) *Parser { return &Parser{row: r, fields: map[string]string{}} }

// Fail mencatat error kolom (error pertama per kolom yang dipakai).
func (p *Parser) Fail(key, msg string) {
	if _, ok := p.fields[key]; !ok {
		p.fields[key] = msg
	}
}

// Err mengembalikan VALIDATION_FAILED berisi error per kolom, atau nil.
func (p *Parser) Err() error {
	if len(p.fields) == 0 {
		return nil
	}
	return apperr.Validation(p.fields)
}

// Text membaca teks apa adanya (sudah dirapikan).
func (p *Parser) Text(key string) string { return p.row.Get(key) }

// OptionalText membaca teks; kosong = nil.
func (p *Parser) OptionalText(key string) *string {
	v := p.row.Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// Bool membaca Ya/Tidak (juga menerima Y/N, true/false, 1/0); kosong = def.
func (p *Parser) Bool(key string, def bool) bool {
	switch strings.ToLower(p.row.Get(key)) {
	case "":
		return def
	case "ya", "y", "true", "1":
		return true
	case "tidak", "t", "n", "false", "0":
		return false
	}
	p.Fail(key, "Isi Ya atau Tidak")
	return def
}

// Int membaca angka bulat; kosong = nil.
func (p *Parser) Int(key string) *int {
	v := p.row.Get(key)
	if v == "" {
		return nil
	}
	d, err := decimal.NewFromString(v)
	if err != nil || !d.IsInteger() || d.Abs().GreaterThan(decimal.NewFromInt(1_000_000_000)) {
		p.Fail(key, "Harus angka bulat")
		return nil
	}
	n := int(d.IntPart())
	return &n
}

// Decimal membaca angka desimal (titik sebagai pemisah desimal); kosong = nil.
func (p *Parser) Decimal(key string) *decimal.Decimal {
	v := p.row.Get(key)
	if v == "" {
		return nil
	}
	d, err := decimal.NewFromString(v)
	if err != nil {
		p.Fail(key, "Harus angka; desimal memakai titik tanpa titik ribuan")
		return nil
	}
	return &d
}

// excelEpoch adalah tanggal 0 sistem tanggal Excel 1900 (dengan koreksi tahun kabisat 1900).
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// Date membaca tanggal YYYY-MM-DD atau nomor seri tanggal Excel; kosong = nil.
func (p *Parser) Date(key string) *clock.BusinessDate {
	v := p.row.Get(key)
	if v == "" {
		return nil
	}
	if d, err := clock.ParseBusinessDate(v); err == nil {
		return &d
	}
	if serial, err := strconv.ParseFloat(v, 64); err == nil && serial > 0 && serial < 2_958_466 {
		t := excelEpoch.AddDate(0, 0, int(serial))
		d := clock.NewBusinessDate(t.Year(), t.Month(), t.Day())
		return &d
	}
	p.Fail(key, "Format tanggal YYYY-MM-DD")
	return nil
}

// Choice mencocokkan label (tanpa membedakan huruf besar/kecil) atau nilai sistem; kosong = "".
func (p *Parser) Choice(key string, options []Option) string {
	v := p.row.Get(key)
	if v == "" {
		return ""
	}
	for _, o := range options {
		if strings.EqualFold(o.Label, v) || strings.EqualFold(o.Value, v) {
			return o.Value
		}
	}
	labels := make([]string, len(options))
	for i, o := range options {
		labels[i] = o.Label
	}
	p.Fail(key, fmt.Sprintf("Pilihan tidak dikenal. Gunakan: %s", strings.Join(labels, ", ")))
	return ""
}
