package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// PeriodStatus adalah status periode akuntansi (Naming Convention 7: Terbuka, Ditutup).
type PeriodStatus string

const (
	PeriodOpen   PeriodStatus = "open"
	PeriodClosed PeriodStatus = "closed"
)

var monthLabels = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// Period adalah periode akuntansi bulanan (ACC-08).
type Period struct {
	ID            uuid.UUID
	Year          int
	Month         time.Month
	Status        PeriodStatus
	ClosedAt      *time.Time
	ClosedBy      *uuid.UUID
	ReopenedCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// PeriodKey adalah tahun & bulan periode.
type PeriodKey struct {
	Year  int
	Month time.Month
}

// PeriodOf mengembalikan periode tanggal bisnis.
func PeriodOf(d clock.BusinessDate) PeriodKey { return PeriodKey{Year: d.Year(), Month: d.Month()} }

// Index adalah urutan bulan absolut untuk membandingkan periode.
func (k PeriodKey) Index() int { return k.Year*12 + int(k.Month) - 1 }

// Next mengembalikan periode berikutnya.
func (k PeriodKey) Next() PeriodKey {
	if k.Month == time.December {
		return PeriodKey{Year: k.Year + 1, Month: time.January}
	}
	return PeriodKey{Year: k.Year, Month: k.Month + 1}
}

// Label mengembalikan nama periode, mis. `Okt 2026`.
func (k PeriodKey) Label() string {
	if k.Month < time.January || k.Month > time.December {
		return fmt.Sprintf("%02d/%d", int(k.Month), k.Year)
	}
	return fmt.Sprintf("%s %d", monthLabels[k.Month-1], k.Year)
}

// Key mengembalikan tahun & bulan periode.
func (p Period) Key() PeriodKey { return PeriodKey{Year: p.Year, Month: p.Month} }

// Label mengembalikan nama periode.
func (p Period) Label() string { return p.Key().Label() }

// Start dan End adalah tanggal pertama dan terakhir periode.
func (p Period) Start() clock.BusinessDate { return clock.NewBusinessDate(p.Year, p.Month, 1) }
func (p Period) End() clock.BusinessDate   { return clock.NewBusinessDate(p.Year, p.Month+1, 0) }

// PeriodLocked adalah error PERIOD_LOCKED untuk periode tertutup (Technical Guideline 5.8.4).
func PeriodLocked(k PeriodKey) error {
	return apperr.WithMessage(apperr.CodePeriodLocked,
		fmt.Sprintf("Periode %s sudah ditutup. Transaksi tidak bisa dibuat atau diubah.", k.Label()))
}

// CheckOpen menolak transaksi di periode tertutup.
func (p Period) CheckOpen() error {
	if p.Status != PeriodOpen {
		return PeriodLocked(p.Key())
	}
	return nil
}
