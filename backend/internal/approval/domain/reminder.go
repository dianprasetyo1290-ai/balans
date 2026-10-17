package domain

import (
	"time"

	"balans/internal/platform/clock"
)

// ReminderInterval adalah jeda pengingat approval yang belum diputuskan (NFR-11).
const ReminderInterval = time.Hour

// Jam kerja WIB untuk pengingat: Senin–Jumat 08.00–17.00 (asumsi Status Fase 1 #6).
const (
	workdayStartHour = 8
	workdayEndHour   = 17
)

// InWorkingHours memeriksa apakah t berada di jam kerja WIB.
func InWorkingHours(t time.Time) bool {
	local := t.In(clock.Jakarta)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	return local.Hour() >= workdayStartHour && local.Hour() < workdayEndHour
}
