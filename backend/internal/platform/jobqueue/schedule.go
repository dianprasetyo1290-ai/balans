package jobqueue

import (
	"time"

	"balans/internal/platform/clock"
)

// DailyAt adalah jadwal harian pada jam WIB tertentu (jadwal di Technical Guideline Lampiran B memakai WIB).
type DailyAt struct {
	Hour   int
	Minute int
}

// Next mengembalikan waktu jalan berikutnya setelah t.
func (d DailyAt) Next(t time.Time) time.Time {
	local := t.In(clock.Jakarta)
	next := time.Date(local.Year(), local.Month(), local.Day(), d.Hour, d.Minute, 0, 0, clock.Jakarta)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC()
}

// MonthlyAt adalah jadwal bulanan pada tanggal dan jam WIB tertentu (mis. penyusutan tanggal 1 pukul 01.00).
// Day 1–28 agar selalu ada di setiap bulan.
type MonthlyAt struct {
	Day    int
	Hour   int
	Minute int
}

// Next mengembalikan waktu jalan berikutnya setelah t.
func (m MonthlyAt) Next(t time.Time) time.Time {
	local := t.In(clock.Jakarta)
	next := time.Date(local.Year(), local.Month(), m.Day, m.Hour, m.Minute, 0, 0, clock.Jakarta)
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month()+1, m.Day, m.Hour, m.Minute, 0, 0, clock.Jakarta)
	}
	return next.UTC()
}
