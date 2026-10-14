package clock

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	// Data zona waktu di-embed agar Asia/Jakarta tersedia di image distroless.
	_ "time/tzdata"
)

const businessDateLayout = "2006-01-02"

// Jakarta adalah zona waktu bisnis Balans (WIB).
var Jakarta = mustLoadJakarta()

func mustLoadJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// WIB tidak mengenal daylight saving, sehingga offset tetap aman dipakai jika tzdata tidak tersedia.
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}

// BusinessDate adalah tanggal transaksi menurut user (WIB), tanpa jam.
type BusinessDate struct {
	year  int
	month time.Month
	day   int
}

// NewBusinessDate membuat tanggal bisnis. Tanggal yang tidak valid dinormalisasi seperti time.Date.
func NewBusinessDate(year int, month time.Month, day int) BusinessDate {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return BusinessDate{year: t.Year(), month: t.Month(), day: t.Day()}
}

// Today mengembalikan tanggal bisnis hari ini menurut WIB.
func Today(c Clock) BusinessDate {
	return BusinessDateOf(c.Now())
}

// BusinessDateOf mengubah waktu menjadi tanggal bisnis WIB.
func BusinessDateOf(t time.Time) BusinessDate {
	w := t.In(Jakarta)
	return BusinessDate{year: w.Year(), month: w.Month(), day: w.Day()}
}

// ParseBusinessDate membaca format YYYY-MM-DD.
func ParseBusinessDate(s string) (BusinessDate, error) {
	t, err := time.Parse(businessDateLayout, s)
	if err != nil {
		return BusinessDate{}, fmt.Errorf("parse business date %q: %w", s, err)
	}
	return BusinessDate{year: t.Year(), month: t.Month(), day: t.Day()}, nil
}

func (d BusinessDate) Year() int         { return d.year }
func (d BusinessDate) Month() time.Month { return d.month }
func (d BusinessDate) Day() int          { return d.day }
func (d BusinessDate) IsZero() bool      { return d.year == 0 && d.month == 0 && d.day == 0 }
func (d BusinessDate) String() string    { return d.Time().Format(businessDateLayout) }
func (d BusinessDate) AddDays(n int) BusinessDate {
	return NewBusinessDate(d.year, d.month, d.day+n)
}

// Time mengembalikan tengah malam UTC pada tanggal tersebut, untuk disimpan sebagai kolom date.
func (d BusinessDate) Time() time.Time {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC)
}

func (d BusinessDate) Before(o BusinessDate) bool { return d.Time().Before(o.Time()) }
func (d BusinessDate) After(o BusinessDate) bool  { return d.Time().After(o.Time()) }
func (d BusinessDate) Equal(o BusinessDate) bool  { return d == o }

func (d BusinessDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *BusinessDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("business date must be a string: %w", err)
	}
	parsed, err := ParseBusinessDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Value menyimpan tanggal bisnis sebagai kolom date.
func (d BusinessDate) Value() (driver.Value, error) {
	return d.Time(), nil
}

// Scan membaca kolom date.
func (d *BusinessDate) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		*d = BusinessDate{year: v.Year(), month: v.Month(), day: v.Day()}
		return nil
	case string:
		parsed, err := ParseBusinessDate(v)
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	default:
		return fmt.Errorf("scan business date: unsupported type %T", src)
	}
}
