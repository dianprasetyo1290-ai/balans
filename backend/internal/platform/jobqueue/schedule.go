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
