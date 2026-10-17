package workbook_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/workbook"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
)

var spec = domain.Spec{
	Kind: "test.item", Label: "Barang", Sheet: "Barang",
	Columns: []tabular.Column{
		{Key: "code", Header: "Kode", Kind: tabular.KindText, Required: true, Identity: true, Example: "007"},
		{Key: "type", Header: "Jenis", Kind: tabular.KindChoice, Options: []tabular.Option{{Value: "a", Label: "Satu"}, {Value: "b", Label: "Dua"}}},
		{Key: "qty", Header: "Qty", Kind: tabular.KindDecimal, Example: "1.5"},
		{Key: "active", Header: "Aktif", Kind: tabular.KindBool},
	},
}

func TestTemplateHasHeadersDropdownsAndGuide(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, workbook.WriteTemplate(&buf, spec))
	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	assert.Equal(t, []string{"Barang", workbook.GuideSheet}, f.GetSheetList())
	rows, err := f.GetRows("Barang")
	require.NoError(t, err)
	assert.Equal(t, []string{"Kode *", "Jenis", "Qty", "Aktif"}, rows[0])
	assert.Len(t, rows, 1, "template tanpa baris data")

	dvs, err := f.GetDataValidations("Barang")
	require.NoError(t, err)
	require.Len(t, dvs, 2)
	assert.Contains(t, dvs[0].Formula1, "Satu,Dua")

	guide, err := f.GetRows(workbook.GuideSheet)
	require.NoError(t, err)
	var flat []string
	for _, r := range guide {
		flat = append(flat, r...)
	}
	assert.Contains(t, flat, "Template Impor Barang")
	assert.Contains(t, flat, "Pilih: Satu, Dua")
	assert.Contains(t, flat, "007", "contoh baris")
}

func fill(t *testing.T, rows [][]any) *bytes.Buffer {
	t.Helper()
	var tpl bytes.Buffer
	require.NoError(t, workbook.WriteTemplate(&tpl, spec))
	f, err := excelize.OpenReader(&tpl)
	require.NoError(t, err)
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		require.NoError(t, f.SetSheetRow("Barang", cell, &r))
	}
	var out bytes.Buffer
	require.NoError(t, f.Write(&out))
	require.NoError(t, f.Close())
	return &out
}

func TestReadFilledTemplate(t *testing.T) {
	file := fill(t, [][]any{
		{"007", "Satu", 1.5, "Ya"},
		{nil, nil, nil, nil},
		{"A-2", "Dua", 12000, "Tidak"},
	})
	sheet, err := workbook.Read(file, spec)
	require.NoError(t, err)
	require.Len(t, sheet.Rows, 2, "baris kosong dilewati")
	assert.Equal(t, 2, sheet.Rows[0].Number)
	assert.Equal(t, map[string]string{"code": "007", "type": "Satu", "qty": "1.5", "active": "Ya"}, sheet.Rows[0].Values)
	assert.Equal(t, 4, sheet.Rows[1].Number)
	assert.Equal(t, "12000", sheet.Rows[1].Get("qty"), "angka tanpa format ribuan")
}

func TestReadRejectsBadFiles(t *testing.T) {
	_, err := workbook.Read(bytes.NewReader([]byte("bukan xlsx")), spec)
	assert.True(t, apperr.HasCode(err, apperr.CodeFileTypeNotAllowed))

	f := excelize.NewFile()
	require.NoError(t, f.SetSheetRow("Sheet1", "A1", &[]any{"Jenis"}))
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	_, err = workbook.Read(&buf, spec)
	require.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
	assert.Contains(t, apperr.As(err).Message, "Kode")

	empty := excelize.NewFile()
	buf.Reset()
	require.NoError(t, empty.Write(&buf))
	_, err = workbook.Read(&buf, spec)
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
}

func TestWriterWithErrorColumnCanBeReimported(t *testing.T) {
	w, err := workbook.NewWriter(spec, true)
	require.NoError(t, err)
	require.NoError(t, w.Write(tabular.NewRow(3).Set("code", "007").Set("qty", "x"), "Baris 3: Qty: Harus angka"))
	assert.Equal(t, 1, w.Rows())
	var buf bytes.Buffer
	require.NoError(t, w.Close(&buf))

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	rows, err := f.GetRows("Barang")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	assert.Equal(t, []string{"Kode *", "Jenis", "Qty", "Aktif", workbook.ErrorColumnHeader}, rows[0])
	assert.Equal(t, "Baris 3: Qty: Harus angka", rows[1][4])

	sheet, err := workbook.Read(bytes.NewReader(buf.Bytes()), spec)
	require.NoError(t, err)
	require.Len(t, sheet.Rows, 1)
	assert.Equal(t, "007", sheet.Rows[0].Get("code"), "kolom error diabaikan dan nol di depan tetap")

	w2, err := workbook.NewWriter(spec, false)
	require.NoError(t, err)
	require.NoError(t, w2.Discard())
}
