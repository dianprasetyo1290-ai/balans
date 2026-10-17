package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/hris/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func ptr[T any](v T) *T { return &v }

func date(y int, m time.Month, d int) clock.BusinessDate { return clock.NewBusinessDate(y, m, d) }

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func input() domain.Input {
	return domain.Input{
		Number: " bn-001 ", FullName: "Rudi Hartono", DivisionID: ptr(uuid.New()), PositionTitle: "Staf Gudang",
		JoinDate: date(2024, 3, 1), WorkEmail: "Rudi@Balans.id", Phone: "0812 3333 4444", NIK: ptr("3201.0101.0101.0001"),
	}
}

func TestNormalizeNIK(t *testing.T) {
	n, ok := domain.NormalizeNIK("3201 0101-0101.0001")
	assert.True(t, ok)
	assert.Equal(t, "3201010101010001", n)
	for _, bad := range []string{"123", "320101010101000A", "32010101010100011"} {
		_, ok := domain.NormalizeNIK(bad)
		assert.False(t, ok, bad)
	}
}

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{"0812-3333-4444": "+6281233334444", "+62 812 3333 4444": "+6281233334444", "62(21)5551234": "+62215551234"} {
		got, ok := domain.NormalizePhone(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"12", "0812x", "00812345678"} {
		_, ok := domain.NormalizePhone(bad)
		assert.False(t, ok, bad)
	}
}

func TestNewEmployee(t *testing.T) {
	e, err := domain.New(input())
	require.NoError(t, err)
	assert.Equal(t, "BN-001", e.Number)
	assert.Equal(t, domain.StatusActive, e.Status)
	assert.Equal(t, "rudi@balans.id", e.WorkEmail)
	assert.Equal(t, "+6281233334444", e.Phone)
	assert.Equal(t, "3201010101010001", e.NIK)

	in := input()
	in.Number, in.NIK, in.FullName = "LAIN", nil, "Rudi H."
	require.NoError(t, e.Update(in))
	assert.Equal(t, "BN-001", e.Number, "nomor karyawan permanen")
	assert.Equal(t, "3201010101010001", e.NIK, "NIK nil = tetap")
	in.NIK = ptr("")
	require.NoError(t, e.Update(in))
	assert.Empty(t, e.NIK)
}

func TestNewEmployeeValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.Input)
		field  string
	}{
		{"nomor kosong", func(in *domain.Input) { in.Number = "" }, "employeeNumber"},
		{"nomor simbol", func(in *domain.Input) { in.Number = "BN 01" }, "employeeNumber"},
		{"nomor panjang", func(in *domain.Input) { in.Number = "A234567890123456789012345678901" }, "employeeNumber"},
		{"nama kosong", func(in *domain.Input) { in.FullName = "" }, "fullName"},
		{"jabatan kosong", func(in *domain.Input) { in.PositionTitle = "" }, "positionTitle"},
		{"tanggal kosong", func(in *domain.Input) { in.JoinDate = clock.BusinessDate{} }, "joinDate"},
		{"email salah", func(in *domain.Input) { in.WorkEmail = "rudi" }, "workEmail"},
		{"hp salah", func(in *domain.Input) { in.Phone = "abc" }, "phone"},
		{"nik salah", func(in *domain.Input) { in.NIK = ptr("123") }, "nik"},
		{"nama panjang", func(in *domain.Input) { in.FullName = string(make([]rune, 121)) }, "fullName"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input()
			c.mutate(&in)
			_, err := domain.New(in)
			assert.Contains(t, fields(t, err), c.field)
		})
	}
}

func TestResignAndActiveOn(t *testing.T) {
	e, err := domain.New(input())
	require.NoError(t, err)
	assert.False(t, e.ActiveOn(date(2024, 2, 29)), "belum bergabung")
	assert.True(t, e.ActiveOn(date(2024, 3, 1)))

	assert.Contains(t, fields(t, e.Resign(date(2024, 1, 1), "Pindah kota")), "resignDate")
	assert.Contains(t, fields(t, e.Resign(clock.BusinessDate{}, "Pindah kota")), "resignDate")
	assert.Contains(t, fields(t, e.Resign(date(2026, 11, 1), " ")), "reason")

	require.NoError(t, e.Resign(date(2026, 11, 1), "Pindah kota"))
	assert.Equal(t, domain.StatusResigned, e.Status)
	assert.True(t, e.ActiveOn(date(2026, 10, 31)), "masih aktif sebelum tanggal resign")
	assert.False(t, e.ActiveOn(date(2026, 11, 1)), "tidak aktif mulai tanggal resign")

	err = e.Resign(date(2026, 12, 1), "Lagi")
	assert.Equal(t, apperr.CodeInvalidStateTransition, apperr.As(err).Code)

	in := input()
	in.JoinDate = date(2027, 1, 1)
	assert.Contains(t, fields(t, e.Update(in)), "joinDate")
}
