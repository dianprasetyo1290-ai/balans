package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Frequency adalah frekuensi jadwal stock count (WMS-11).
type Frequency string

const (
	FrequencyWeekly  Frequency = "weekly"
	FrequencyMonthly Frequency = "monthly"
	FrequencyYearly  Frequency = "yearly"
)

// CountSchedule adalah jadwal stock count: cycle count berkala, konsinyasi/vending bulanan, penuh akhir tahun.
type CountSchedule struct {
	ID             uuid.UUID
	Name           string
	CountType      CountType
	Frequency      Frequency
	DayOfWeek      *int
	DayOfMonth     *int
	MonthOfYear    *int
	WarehouseID    *uuid.UUID
	LocationIDs    []uuid.UUID
	AssigneeUserID *uuid.UUID
	AssigneeName   string
	NextRunDate    clock.BusinessDate
	LastRunDate    *clock.BusinessDate
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// ScheduleInput adalah isian jadwal.
type ScheduleInput struct {
	Name           string
	CountType      CountType
	Frequency      Frequency
	DayOfWeek      *int
	DayOfMonth     *int
	MonthOfYear    *int
	WarehouseID    *uuid.UUID
	LocationIDs    []uuid.UUID
	AssigneeUserID *uuid.UUID
	AssigneeName   string
	IsActive       bool
}

// NewSchedule memvalidasi jadwal dan menghitung tanggal jalan pertama (hari ini atau sesudahnya).
func NewSchedule(in ScheduleInput, today clock.BusinessDate) (CountSchedule, error) {
	s := CountSchedule{}
	if err := s.apply(in, today); err != nil {
		return CountSchedule{}, err
	}
	return s, nil
}

// Update mengganti isian jadwal; tanggal jalan berikutnya dihitung ulang dari hari ini.
func (s *CountSchedule) Update(in ScheduleInput, today clock.BusinessDate) error {
	return s.apply(in, today)
}

func (s *CountSchedule) apply(in ScheduleInput, today clock.BusinessDate) error {
	fields := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > MaxNameLength {
		fields["name"] = "Nama jadwal wajib diisi (maks. 120 karakter)"
	}
	if !in.CountType.Valid() {
		fields["countType"] = "Jenis stock count tidak dikenal"
	}
	n := CountSchedule{
		Name: name, CountType: in.CountType, Frequency: in.Frequency, WarehouseID: in.WarehouseID,
		LocationIDs: in.LocationIDs, AssigneeUserID: in.AssigneeUserID, AssigneeName: in.AssigneeName, IsActive: in.IsActive,
	}
	inRange := func(v *int, lo, hi int) bool { return v != nil && *v >= lo && *v <= hi }
	switch in.Frequency {
	case FrequencyWeekly:
		if !inRange(in.DayOfWeek, 1, 7) {
			fields["dayOfWeek"] = "Hari wajib dipilih (Senin – Minggu)"
		}
		n.DayOfWeek = in.DayOfWeek
	case FrequencyMonthly:
		if !inRange(in.DayOfMonth, 1, 31) {
			fields["dayOfMonth"] = "Tanggal 1 – 31 (31 = akhir bulan)"
		}
		n.DayOfMonth = in.DayOfMonth
	case FrequencyYearly:
		if !inRange(in.DayOfMonth, 1, 31) {
			fields["dayOfMonth"] = "Tanggal 1 – 31 (31 = akhir bulan)"
		}
		if !inRange(in.MonthOfYear, 1, 12) {
			fields["monthOfYear"] = "Bulan wajib dipilih"
		}
		n.DayOfMonth, n.MonthOfYear = in.DayOfMonth, in.MonthOfYear
	default:
		fields["frequency"] = "Frekuensi tidak dikenal"
	}
	if in.CountType.Adjusts() && in.WarehouseID == nil && len(in.LocationIDs) == 0 {
		fields["warehouseId"] = "Pilih gudang atau lokasi yang dihitung"
	}
	if len(in.LocationIDs) > 100 {
		fields["locationIds"] = "Maksimal 100 lokasi"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	n.ID, n.CreatedAt, n.UpdatedAt, n.Version, n.LastRunDate = s.ID, s.CreatedAt, s.UpdatedAt, s.Version, s.LastRunDate
	n.NextRunDate = n.Occurrence(today)
	*s = n
	return nil
}

// Occurrence mengembalikan tanggal jalan pertama yang sama dengan atau sesudah from.
func (s CountSchedule) Occurrence(from clock.BusinessDate) clock.BusinessDate {
	switch s.Frequency {
	case FrequencyWeekly:
		// time.Weekday: Minggu = 0; jadwal: Senin = 1 … Minggu = 7.
		want := *s.DayOfWeek % 7
		d := from
		for int(d.Time().Weekday()) != want {
			d = d.AddDays(1)
		}
		return d
	case FrequencyMonthly:
		d := dayInMonth(from.Year(), from.Month(), *s.DayOfMonth)
		if d.Before(from) {
			next := from.Time().AddDate(0, 1, 1-from.Day())
			d = dayInMonth(next.Year(), next.Month(), *s.DayOfMonth)
		}
		return d
	case FrequencyYearly:
		m := time.Month(*s.MonthOfYear)
		d := dayInMonth(from.Year(), m, *s.DayOfMonth)
		if d.Before(from) {
			d = dayInMonth(from.Year()+1, m, *s.DayOfMonth)
		}
		return d
	}
	return from
}

// Advance mencatat jalan pada tanggal run dan menghitung tanggal berikutnya (sesudah run).
func (s *CountSchedule) Advance(run clock.BusinessDate) {
	r := run
	s.LastRunDate = &r
	s.NextRunDate = s.Occurrence(run.AddDays(1))
}

// dayInMonth memotong tanggal ke hari terakhir bulan (mis. 31 → 30 November, 28/29 Februari).
func dayInMonth(year int, month time.Month, day int) clock.BusinessDate {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return clock.NewBusinessDate(year, month, day)
}
