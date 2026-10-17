package domain

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"balans/internal/platform/apperr"
	"balans/internal/platform/pii"
	"balans/internal/platform/tabular"
)

// RowError adalah satu error pada baris impor. Column kosong = error baris (bukan kolom tertentu).
type RowError struct {
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}

// RowResult adalah hasil pemeriksaan satu baris untuk preview.
type RowResult struct {
	Number int
	Values map[string]string
	Errors []RowError
}

// Valid bernilai true jika baris tidak punya error.
func (r RowResult) Valid() bool { return len(r.Errors) == 0 }

// RequiredMarker ditambahkan pada header kolom wajib di template, mis. `SKU *`.
const RequiredMarker = " *"

// HeaderText menulis header kolom di template.
func HeaderText(c tabular.Column) string {
	if c.Required {
		return c.Header + RequiredMarker
	}
	return c.Header
}

func normalizeHeader(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "*"))
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// MapHeaders mencocokkan header baris pertama file dengan kolom spec (tanda `*` dan huruf besar/kecil
// diabaikan). Mengembalikan peta indeks kolom file → key. Kolom yang tidak dikenal diabaikan (mis. kolom
// `Keterangan Error` dari file baris error); kolom wajib yang tidak ada membuat file ditolak.
func MapHeaders(columns []tabular.Column, headers []string) (map[int]string, error) {
	byHeader := make(map[string]string, len(columns))
	for _, c := range columns {
		byHeader[normalizeHeader(c.Header)] = c.Key
	}
	out := map[int]string{}
	seen := map[string]bool{}
	for i, h := range headers {
		key, ok := byHeader[normalizeHeader(h)]
		if !ok || seen[key] {
			continue
		}
		out[i], seen[key] = key, true
	}
	var missing []string
	for _, c := range columns {
		if c.Required && !seen[c.Key] {
			missing = append(missing, c.Header)
		}
	}
	if len(missing) > 0 {
		return nil, apperr.WithMessage(apperr.CodeValidationFailed,
			"Kolom wajib tidak ditemukan: "+strings.Join(missing, ", ")+". Gunakan template yang diunduh dari sistem.")
	}
	return out, nil
}

// CleanCell merapikan isi sel: spasi di tepi dan karakter kontrol dibuang.
func CleanCell(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// BuildRow menyusun baris dari sel file. Mengembalikan ok=false jika semua sel kosong (baris dilewati).
func BuildRow(number int, cells []string, headerMap map[int]string) (tabular.Row, bool) {
	row := tabular.NewRow(number)
	for i, cell := range cells {
		key, ok := headerMap[i]
		if !ok {
			continue
		}
		if v := CleanCell(cell); v != "" {
			row.Values[key] = v
		}
	}
	return row, len(row.Values) > 0
}

// maxCellLength membatasi isi satu sel agar tabel preview tidak membengkak.
const maxCellLength = 2000

// CheckRows memeriksa kolom wajib, panjang sel, dan kunci bisnis ganda di satu file. Hasilnya error per nomor
// baris; baris tanpa error tidak ada di peta.
func CheckRows(columns []tabular.Column, rows []tabular.Row) map[int][]RowError {
	out := map[int][]RowError{}
	var identity []tabular.Column
	for _, c := range columns {
		if c.Identity {
			identity = append(identity, c)
		}
	}
	firstByKey := map[string]int{}
	for _, r := range rows {
		for _, c := range columns {
			v := r.Get(c.Key)
			switch {
			case c.Required && v == "":
				out[r.Number] = append(out[r.Number], RowError{Column: c.Key, Message: c.Header + " wajib diisi"})
			case utf8.RuneCountInString(v) > maxCellLength:
				out[r.Number] = append(out[r.Number], RowError{Column: c.Key, Message: fmt.Sprintf("Maksimal %d karakter", maxCellLength)})
			}
		}
		if len(identity) == 0 {
			continue
		}
		parts := make([]string, len(identity))
		empty := true
		for i, c := range identity {
			parts[i] = strings.ToUpper(r.Get(c.Key))
			empty = empty && parts[i] == ""
		}
		if empty {
			continue
		}
		key := strings.Join(parts, "\x00")
		if first, dup := firstByKey[key]; dup {
			out[r.Number] = append(out[r.Number], RowError{
				Column: identity[0].Key, Message: fmt.Sprintf("Data ganda dengan baris %d di file yang sama", first),
			})
			continue
		}
		firstByKey[key] = r.Number
	}
	return out
}

// ErrorsFrom memetakan error modul ke error baris. Field VALIDATION_FAILED yang sama dengan key kolom menjadi
// error kolom; field lain (mis. `addresses.0.city`) menjadi error baris dengan nama field-nya. Error selain
// apperr (gangguan sistem) dikembalikan sebagai error agar job diulang.
func ErrorsFrom(err error, columns []tabular.Column) ([]RowError, error) {
	if err == nil {
		return nil, nil
	}
	appErr := apperr.As(err)
	if appErr.Code == apperr.CodeInternal {
		return nil, err
	}
	if len(appErr.Fields) == 0 {
		return []RowError{{Message: appErr.Message}}, nil
	}
	keys := make([]string, 0, len(appErr.Fields))
	for k := range appErr.Fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	known := map[string]bool{}
	for _, c := range columns {
		known[c.Key] = true
	}
	out := make([]RowError, 0, len(keys))
	for _, k := range keys {
		if known[k] {
			out = append(out, RowError{Column: k, Message: appErr.Fields[k]})
		} else {
			out = append(out, RowError{Message: appErr.Fields[k]})
		}
	}
	return out, nil
}

// FormatErrors menggabungkan error baris untuk kolom `Keterangan Error` di file baris error.
func FormatErrors(columns []tabular.Column, errs []RowError) string {
	headers := map[string]string{}
	for _, c := range columns {
		headers[c.Key] = c.Header
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		if h, ok := headers[e.Column]; ok {
			parts[i] = h + ": " + e.Message
		} else {
			parts[i] = e.Message
		}
	}
	return strings.Join(parts, "; ")
}

// PreviewValues menyalin nilai baris untuk disimpan sebagai preview; kolom Sensitive disamarkan sehingga data
// pribadi tidak tersimpan di tabel impor (Technical Guideline 6.6).
func PreviewValues(columns []tabular.Column, r tabular.Row) map[string]string {
	out := make(map[string]string, len(r.Values))
	for _, c := range columns {
		v, ok := r.Values[c.Key]
		if !ok {
			continue
		}
		if c.Sensitive {
			v = pii.Mask(v)
		}
		out[c.Key] = v
	}
	return out
}
