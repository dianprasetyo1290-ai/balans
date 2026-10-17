package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/dataexchange/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
)

var columns = []tabular.Column{
	{Key: "sku", Header: "SKU", Required: true, Identity: true},
	{Key: "name", Header: "Nama Produk", Required: true},
	{Key: "npwp", Header: "NPWP", Sensitive: true},
	{Key: "notes", Header: "Catatan"},
}

func row(n int, kv ...string) tabular.Row {
	r := tabular.NewRow(n)
	for i := 0; i+1 < len(kv); i += 2 {
		r.Values[kv[i]] = kv[i+1]
	}
	return r
}

func TestImportJobTransitions(t *testing.T) {
	tests := []struct {
		name      string
		status    domain.ImportStatus
		valid     int
		commitErr bool
		cancelErr bool
	}{
		{"validated dengan baris valid", domain.ImportValidated, 3, false, false},
		{"validated tanpa baris valid", domain.ImportValidated, 0, true, false},
		{"masih diperiksa", domain.ImportValidating, 3, true, true},
		{"sudah selesai", domain.ImportCompleted, 3, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := domain.ImportJob{Status: tt.status, ValidRows: tt.valid}
			err := j.Commit()
			assert.Equal(t, tt.commitErr, err != nil)
			if err == nil {
				assert.Equal(t, domain.ImportCommitting, j.Status)
			}
			j2 := domain.ImportJob{Status: tt.status, ValidRows: tt.valid}
			err = j2.Cancel()
			assert.Equal(t, tt.cancelErr, err != nil)
			if err == nil {
				assert.Equal(t, domain.ImportCancelled, j2.Status)
			}
		})
	}
}

func TestImportJobProgressAndActions(t *testing.T) {
	j := domain.ImportJob{Status: domain.ImportValidating, TotalRows: 200, ProcessedRows: 50}
	assert.Equal(t, 25, j.Progress())
	assert.True(t, j.Status.Running())
	assert.Empty(t, j.AllowedActions())

	j = domain.ImportJob{Status: domain.ImportCommitting, ValidRows: 10, CommittedRows: 10}
	assert.Equal(t, 100, j.Progress())
	j = domain.ImportJob{Status: domain.ImportUploaded}
	assert.Equal(t, 0, j.Progress())
	j = domain.ImportJob{Status: domain.ImportValidating}
	assert.Equal(t, 0, j.Progress(), "total 0 tidak membagi dengan nol")

	id := j.ID
	j = domain.ImportJob{Status: domain.ImportValidated, ValidRows: 2, ErrorFileID: &id}
	assert.Equal(t, 100, j.Progress())
	assert.False(t, j.Status.Running())
	assert.Equal(t, []string{"commit", "cancel", "download_errors"}, j.AllowedActions())
	j.ValidRows = 0
	assert.Equal(t, []string{"cancel", "download_errors"}, j.AllowedActions())
}

func TestSpecNames(t *testing.T) {
	s := domain.Spec{Kind: "catalog.product", Label: "Produk"}
	assert.Equal(t, "catalog.product.import", s.ImportPermission())
	assert.Equal(t, "catalog.product.export", s.ExportPermission())
	assert.Equal(t, "Template Produk.xlsx", s.FileName("Template"))
}

func TestMapHeaders(t *testing.T) {
	m, err := domain.MapHeaders(columns, []string{"sku *", " Nama  Produk*", "Keterangan Error", "SKU"})
	require.NoError(t, err)
	assert.Equal(t, map[int]string{0: "sku", 1: "name"}, m, "kolom tak dikenal dan kolom ganda diabaikan")

	_, err = domain.MapHeaders(columns, []string{"SKU", "Catatan"})
	require.Error(t, err)
	assert.Contains(t, apperr.As(err).Message, "Nama Produk")
}

func TestBuildRowAndCleanCell(t *testing.T) {
	headers := map[int]string{0: "sku", 2: "name"}
	r, ok := domain.BuildRow(5, []string{" ab\t1 ", "abaikan", "Nama\x01"}, headers)
	require.True(t, ok)
	assert.Equal(t, 5, r.Number)
	assert.Equal(t, "ab 1", r.Get("sku"))
	assert.Equal(t, "Nama", r.Get("name"))

	_, ok = domain.BuildRow(6, []string{"", "isi kolom lain", " "}, headers)
	assert.False(t, ok, "baris tanpa nilai di kolom dikenal dilewati")
}

func TestCheckRows(t *testing.T) {
	long := make([]byte, 2001)
	for i := range long {
		long[i] = 'x'
	}
	rows := []tabular.Row{
		row(2, "sku", "A", "name", "Satu"),
		row(3, "sku", "b", "name", "Dua"),
		row(4, "sku", "a", "name", "Ganda"),
		row(5, "name", "Tanpa SKU"),
		row(6, "sku", "C", "name", "Panjang", "notes", string(long)),
	}
	errs := domain.CheckRows(columns, rows)
	assert.NotContains(t, errs, 2)
	assert.NotContains(t, errs, 3)
	require.Len(t, errs[4], 1)
	assert.Equal(t, "Data ganda dengan baris 2 di file yang sama", errs[4][0].Message)
	assert.Equal(t, []domain.RowError{{Column: "sku", Message: "SKU wajib diisi"}}, errs[5])
	assert.Equal(t, "notes", errs[6][0].Column)

	assert.Empty(t, domain.CheckRows([]tabular.Column{{Key: "x", Header: "X"}}, rows))
}

func TestErrorsFrom(t *testing.T) {
	got, err := domain.ErrorsFrom(nil, columns)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = domain.ErrorsFrom(apperr.Validation(map[string]string{"sku": "SKU dipakai", "addresses.0.city": "Kota wajib"}), columns)
	require.NoError(t, err)
	assert.Equal(t, []domain.RowError{{Message: "Kota wajib"}, {Column: "sku", Message: "SKU dipakai"}}, got)

	got, err = domain.ErrorsFrom(apperr.New(apperr.CodeDuplicateCode), columns)
	require.NoError(t, err)
	assert.Equal(t, []domain.RowError{{Message: apperr.CodeDuplicateCode.DefaultMessage()}}, got)

	sys := errors.New("koneksi putus")
	_, err = domain.ErrorsFrom(sys, columns)
	assert.ErrorIs(t, err, sys)
}

func TestFormatErrorsAndPreview(t *testing.T) {
	text := domain.FormatErrors(columns, []domain.RowError{{Column: "sku", Message: "Wajib"}, {Message: "Lainnya"}})
	assert.Equal(t, "SKU: Wajib; Lainnya", text)

	assert.Equal(t, "SKU *", domain.HeaderText(columns[0]))
	assert.Equal(t, "Catatan", domain.HeaderText(columns[3]))

	pv := domain.PreviewValues(columns, row(2, "sku", "A", "npwp", "012345678901234", "other", "x"))
	assert.Equal(t, map[string]string{"sku": "A", "npwp": "•••••••••••1234"}, pv)

	r := domain.RowResult{Number: 2}
	assert.True(t, r.Valid())
	r.Errors = []domain.RowError{{Message: "x"}}
	assert.False(t, r.Valid())
}
