// Package workbook membaca dan menulis file xlsx impor/ekspor (Technical Guideline 5.15) dengan excelize.
// Template dihasilkan dari definisi kolom: sheet data dengan header berwarna (oranye = wajib, abu-abu =
// opsional), dropdown untuk kolom pilihan, format teks agar nol di depan tidak hilang, dan sheet Petunjuk
// berisi aturan per kolom serta contoh baris (sama dengan template migrasi Fase 0).
package workbook

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"balans/internal/dataexchange/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
)

// GuideSheet adalah nama sheet petunjuk.
const GuideSheet = "Petunjuk"

// ErrorColumnHeader adalah kolom tambahan di file baris error.
const ErrorColumnHeader = "Keterangan Error"

// ContentType adalah tipe MIME xlsx.
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

const (
	colorRequired = "F4B183"
	colorOptional = "D9D9D9"
	colorError    = "F8CBAD"
)

type styles struct {
	required, optional, errHeader, text, example, title int
}

func newStyles(f *excelize.File) (styles, error) {
	var s styles
	header := func(color string) (int, error) {
		return f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}},
			Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
			Border: []excelize.Border{
				{Type: "bottom", Color: "808080", Style: 1},
			},
		})
	}
	var err error
	if s.required, err = header(colorRequired); err != nil {
		return s, err
	}
	if s.optional, err = header(colorOptional); err != nil {
		return s, err
	}
	if s.errHeader, err = header(colorError); err != nil {
		return s, err
	}
	textFmt := "@"
	if s.text, err = f.NewStyle(&excelize.Style{CustomNumFmt: &textFmt}); err != nil {
		return s, err
	}
	if s.example, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Color: "595959"}, CustomNumFmt: &textFmt}); err != nil {
		return s, err
	}
	s.title, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	return s, err
}

func columnWidth(c tabular.Column) float64 {
	w := float64(len([]rune(domain.HeaderText(c)))) + 4
	return max(14, min(w, 40))
}

// writeHeader menulis baris header sheet data dan mengatur lebar kolom.
func writeHeader(f *excelize.File, sheet string, st styles, columns []tabular.Column, extra string) error {
	for i, c := range columns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, cell, domain.HeaderText(c)); err != nil {
			return err
		}
		style := st.optional
		if c.Required {
			style = st.required
		}
		if err := f.SetCellStyle(sheet, cell, cell, style); err != nil {
			return err
		}
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if err := f.SetColWidth(sheet, col, col, columnWidth(c)); err != nil {
			return err
		}
	}
	if extra != "" {
		cell, err := excelize.CoordinatesToCellName(len(columns)+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, cell, extra); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, st.errHeader); err != nil {
			return err
		}
		col, _ := excelize.ColumnNumberToName(len(columns) + 1)
		if err := f.SetColWidth(sheet, col, col, 60); err != nil {
			return err
		}
	}
	return f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

// prepareDataColumns memberi format teks dan dropdown untuk baris data template.
func prepareDataColumns(f *excelize.File, sheet string, st styles, columns []tabular.Column) error {
	last := domain.MaxRows + 1
	for i, c := range columns {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if c.Kind == tabular.KindText || c.Kind == "" || c.Kind == tabular.KindChoice || c.Kind == tabular.KindBool {
			if err := f.SetColStyle(sheet, col, st.text); err != nil {
				return err
			}
		}
		labels := c.OptionLabels()
		if len(labels) == 0 || len(strings.Join(labels, ",")) > 250 {
			continue
		}
		dv := excelize.NewDataValidation(true)
		dv.Sqref = fmt.Sprintf("%s2:%s%d", col, col, last)
		if err := dv.SetDropList(labels); err != nil {
			return err
		}
		dv.SetError(excelize.DataValidationErrorStyleStop, "Pilihan tidak dikenal", "Pilih salah satu nilai di daftar.")
		if err := f.AddDataValidation(sheet, dv); err != nil {
			return err
		}
	}
	return nil
}

func writeGuide(f *excelize.File, st styles, spec domain.Spec) error {
	sh := GuideSheet
	set := func(cell string, v any) error { return f.SetCellValue(sh, cell, v) }
	if err := set("A1", "Template Impor "+spec.Label); err != nil {
		return err
	}
	if err := f.SetCellStyle(sh, "A1", "A1", st.title); err != nil {
		return err
	}
	rules := []string{
		"Isi data di sheet \"" + spec.Sheet + "\" mulai baris 2. Jangan mengubah nama sheet maupun nama kolom.",
		"Header oranye bertanda * wajib diisi; header abu-abu boleh dikosongkan.",
		"Tanggal YYYY-MM-DD. Angka tanpa titik ribuan dan tanpa Rp; desimal memakai titik.",
		"Kolom pilihan memakai dropdown; nilai di luar daftar ditolak saat impor.",
		"Data dengan kode yang sudah ada akan diperbarui; kode baru akan ditambahkan.",
		"Baris error bisa diunduh setelah pemeriksaan, diperbaiki, lalu diimpor ulang.",
	}
	for i, r := range rules {
		if err := set(fmt.Sprintf("A%d", i+3), fmt.Sprintf("%d. %s", i+1, r)); err != nil {
			return err
		}
	}
	start := len(rules) + 4
	heads := []string{"Kolom", "Wajib", "Format", "Keterangan", "Contoh"}
	for i, h := range heads {
		cell, _ := excelize.CoordinatesToCellName(i+1, start)
		if err := set(cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sh, cell, cell, st.optional); err != nil {
			return err
		}
	}
	for i, c := range spec.Columns {
		r := start + 1 + i
		wajib := tabular.No
		if c.Required {
			wajib = tabular.Yes
		}
		vals := []string{c.Header, wajib, c.FormatHint(), c.Note, c.Example}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, r)
			if err := f.SetCellStr(sh, cell, v); err != nil {
				return err
			}
		}
	}
	// Contoh baris dengan susunan kolom yang sama dengan sheet data.
	exRow := start + len(spec.Columns) + 3
	if err := set(fmt.Sprintf("A%d", exRow-1), "Contoh baris (salin ke sheet "+spec.Sheet+" jika perlu):"); err != nil {
		return err
	}
	for i, c := range spec.Columns {
		head, _ := excelize.CoordinatesToCellName(i+1, exRow)
		if err := f.SetCellStr(sh, head, domain.HeaderText(c)); err != nil {
			return err
		}
		style := st.optional
		if c.Required {
			style = st.required
		}
		if err := f.SetCellStyle(sh, head, head, style); err != nil {
			return err
		}
		val, _ := excelize.CoordinatesToCellName(i+1, exRow+1)
		if err := f.SetCellStr(sh, val, c.Example); err != nil {
			return err
		}
		if err := f.SetCellStyle(sh, val, val, st.example); err != nil {
			return err
		}
	}
	widths := map[string]float64{"A": 28, "B": 10, "C": 40, "D": 50, "E": 24}
	for col, w := range widths {
		if err := f.SetColWidth(sh, col, col, w); err != nil {
			return err
		}
	}
	return nil
}

// WriteTemplate menulis template impor kosong ke w.
func WriteTemplate(w io.Writer, spec domain.Spec) (err error) {
	f := excelize.NewFile()
	defer func() { err = errors.Join(err, f.Close()) }()
	st, err := newStyles(f)
	if err != nil {
		return err
	}
	if err := f.SetSheetName("Sheet1", spec.Sheet); err != nil {
		return err
	}
	if err := writeHeader(f, spec.Sheet, st, spec.Columns, ""); err != nil {
		return err
	}
	if err := prepareDataColumns(f, spec.Sheet, st, spec.Columns); err != nil {
		return err
	}
	if _, err := f.NewSheet(GuideSheet); err != nil {
		return err
	}
	if err := writeGuide(f, st, spec); err != nil {
		return err
	}
	return f.Write(w)
}

// Writer menulis baris data (ekspor atau baris error) secara streaming agar file besar tidak ditahan di memori.
type Writer struct {
	f       *excelize.File
	sw      *excelize.StreamWriter
	columns []tabular.Column
	extra   bool
	next    int
}

// NewWriter membuat file dengan header kolom spec; withErrors menambah kolom Keterangan Error.
func NewWriter(spec domain.Spec, withErrors bool) (*Writer, error) {
	f := excelize.NewFile()
	st, err := newStyles(f)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err := f.SetSheetName("Sheet1", spec.Sheet); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	sw, err := f.NewStreamWriter(spec.Sheet)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	header := make([]any, 0, len(spec.Columns)+1)
	for i, c := range spec.Columns {
		style := st.optional
		if c.Required {
			style = st.required
		}
		header = append(header, excelize.Cell{StyleID: style, Value: domain.HeaderText(c)})
		if err := sw.SetColWidth(i+1, i+1, columnWidth(c)); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	if withErrors {
		header = append(header, excelize.Cell{StyleID: st.errHeader, Value: ErrorColumnHeader})
		if err := sw.SetColWidth(len(spec.Columns)+1, len(spec.Columns)+1, 60); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	if err := sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err := sw.SetRow("A1", header); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return &Writer{f: f, sw: sw, columns: spec.Columns, extra: withErrors, next: 2}, nil
}

// Write menambah satu baris. errText dipakai jika Writer dibuat dengan kolom error.
func (w *Writer) Write(r tabular.Row, errText string) error {
	vals := make([]any, 0, len(w.columns)+1)
	for _, c := range w.columns {
		// Semua nilai ditulis sebagai teks agar kode bernol depan dan nomor panjang tidak berubah.
		vals = append(vals, r.Get(c.Key))
	}
	if w.extra {
		vals = append(vals, errText)
	}
	cell, err := excelize.CoordinatesToCellName(1, w.next)
	if err != nil {
		return err
	}
	w.next++
	return w.sw.SetRow(cell, vals)
}

// Rows mengembalikan jumlah baris data yang sudah ditulis.
func (w *Writer) Rows() int { return w.next - 2 }

// Close menyelesaikan file dan menulisnya ke out.
func (w *Writer) Close(out io.Writer) (err error) {
	defer func() { err = errors.Join(err, w.f.Close()) }()
	if err := w.sw.Flush(); err != nil {
		return err
	}
	return w.f.Write(out)
}

// Discard menutup file tanpa menulis.
func (w *Writer) Discard() error { return w.f.Close() }

// Sheet adalah isi sheet data yang dibaca dari file impor (nilai asli, belum disamarkan).
type Sheet struct {
	Rows []tabular.Row
}

func pickSheet(f *excelize.File, name string) (string, error) {
	sheets := f.GetSheetList()
	for _, s := range sheets {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return s, nil
		}
	}
	for _, s := range sheets {
		if !strings.EqualFold(s, GuideSheet) {
			return s, nil
		}
	}
	return "", apperr.WithMessage(apperr.CodeValidationFailed, "Sheet "+name+" tidak ditemukan.")
}

func invalidFile() error {
	return apperr.WithMessage(apperr.CodeFileTypeNotAllowed, "File tidak bisa dibaca. Gunakan file .xlsx dari template sistem.")
}

// Read membaca sheet data file impor: baris 1 header, baris kosong dilewati. Header yang tidak cocok
// ditolak VALIDATION_FAILED, file bukan xlsx FILE_TYPE_NOT_ALLOWED, lebih dari MaxRows baris ditolak.
func Read(r io.Reader, spec domain.Spec) (out Sheet, err error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return Sheet{}, invalidFile()
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	name, err := pickSheet(f, spec.Sheet)
	if err != nil {
		return Sheet{}, err
	}
	rows, err := f.Rows(name)
	if err != nil {
		return Sheet{}, invalidFile()
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	var headerMap map[int]string
	number := 0
	for rows.Next() {
		number++
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return Sheet{}, invalidFile()
		}
		if headerMap == nil {
			if headerMap, err = domain.MapHeaders(spec.Columns, cells); err != nil {
				return Sheet{}, err
			}
			continue
		}
		row, ok := domain.BuildRow(number, cells, headerMap)
		if !ok {
			continue
		}
		if len(out.Rows) >= domain.MaxRows {
			return Sheet{}, apperr.WithMessage(apperr.CodeValidationFailed,
				fmt.Sprintf("File berisi lebih dari %d baris. Pecah menjadi beberapa file.", domain.MaxRows))
		}
		out.Rows = append(out.Rows, row)
	}
	if headerMap == nil {
		return Sheet{}, apperr.WithMessage(apperr.CodeValidationFailed, "File kosong. Isi data mulai baris 2 template.")
	}
	return out, nil
}
