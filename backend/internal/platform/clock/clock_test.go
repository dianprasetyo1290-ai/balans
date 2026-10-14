package clock_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/clock"
)

func TestTodayUsesJakartaDate(t *testing.T) {
	tests := []struct {
		name string
		utc  time.Time
		want string
	}{
		{"siang UTC masih hari yang sama", time.Date(2026, 10, 14, 5, 0, 0, 0, time.UTC), "2026-10-14"},
		{"17.00 UTC sudah berganti hari di WIB", time.Date(2026, 10, 14, 17, 0, 0, 0, time.UTC), "2026-10-15"},
		{"16.59 UTC belum berganti hari", time.Date(2026, 10, 14, 16, 59, 59, 0, time.UTC), "2026-10-14"},
		{"akhir tahun", time.Date(2026, 12, 31, 18, 0, 0, 0, time.UTC), "2027-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clock.Today(clock.NewFixed(tt.utc)).String())
		})
	}
}

func TestFixedAdvance(t *testing.T) {
	c := clock.NewFixed(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))
	c.Advance(90 * time.Minute)
	assert.Equal(t, time.Date(2026, 10, 14, 1, 30, 0, 0, time.UTC), c.Now())
}

func TestSystemClockIsUTC(t *testing.T) {
	assert.Equal(t, time.UTC, clock.System{}.Now().Location())
}

func TestBusinessDateParseAndCompare(t *testing.T) {
	d, err := clock.ParseBusinessDate("2026-10-31")
	require.NoError(t, err)

	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.October, d.Month())
	assert.Equal(t, 31, d.Day())
	assert.Equal(t, "2026-11-01", d.AddDays(1).String())
	assert.True(t, d.Before(d.AddDays(1)))
	assert.True(t, d.AddDays(1).After(d))
	assert.True(t, d.Equal(clock.NewBusinessDate(2026, time.October, 31)))
	assert.False(t, d.IsZero())
	assert.True(t, clock.BusinessDate{}.IsZero())

	_, err = clock.ParseBusinessDate("31-10-2026")
	assert.Error(t, err)
}

func TestBusinessDateJSON(t *testing.T) {
	type payload struct {
		BusinessDate clock.BusinessDate `json:"businessDate"`
	}
	b, err := json.Marshal(payload{clock.NewBusinessDate(2026, time.October, 14)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"businessDate":"2026-10-14"}`, string(b))

	var got payload
	require.NoError(t, json.Unmarshal([]byte(`{"businessDate":"2027-01-05"}`), &got))
	assert.Equal(t, "2027-01-05", got.BusinessDate.String())

	assert.Error(t, json.Unmarshal([]byte(`{"businessDate":20270105}`), &got))
	assert.Error(t, json.Unmarshal([]byte(`{"businessDate":"05/01/2027"}`), &got))
}

func TestBusinessDateSQL(t *testing.T) {
	d := clock.NewBusinessDate(2026, time.October, 14)
	v, err := d.Value()
	require.NoError(t, err)

	var scanned clock.BusinessDate
	require.NoError(t, scanned.Scan(v))
	assert.Equal(t, d, scanned)

	require.NoError(t, scanned.Scan("2027-02-01"))
	assert.Equal(t, "2027-02-01", scanned.String())
	assert.Error(t, scanned.Scan(42))
	assert.Error(t, scanned.Scan("bukan tanggal"))
}
