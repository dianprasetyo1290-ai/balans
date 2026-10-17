package domain

import (
	"cmp"
	"slices"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
)

// Candidate adalah stok satu batch (atau satu reservasi) yang bisa diambil.
type Candidate struct {
	BatchID        *uuid.UUID
	BatchNumber    string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
	Qty            decimal.Decimal
	// ReservationID diisi bila kandidat adalah reservasi dokumen.
	ReservationID *uuid.UUID
}

// Expired memeriksa apakah batch sudah lewat tanggal kedaluwarsa pada hari bisnis today. Batch masih boleh
// dialokasikan pada tanggal kedaluwarsanya.
func (c Candidate) Expired(today clock.BusinessDate) bool {
	return c.ExpiryDate != nil && c.ExpiryDate.Before(today)
}

// Allocation adalah qty yang diambil dari satu kandidat.
type Allocation struct {
	Candidate Candidate
	Qty       decimal.Decimal
}

// BatchAllocator adalah strategi alokasi batch (Technical Guideline 5.7.3); default FEFO.
type BatchAllocator interface {
	// Allocate mengambil qty dari kandidat; shortfall > 0 bila kandidat tidak cukup.
	Allocate(candidates []Candidate, qty decimal.Decimal, today clock.BusinessDate) (allocs []Allocation, shortfall decimal.Decimal)
}

// FEFO adalah First Expired First Out (INV-09): kedaluwarsa terdekat lebih dulu, batch kedaluwarsa dan stok nol
// dilewati, batch tanpa tanggal kedaluwarsa paling akhir.
type FEFO struct{}

var _ BatchAllocator = FEFO{}

func compareDate(a, b *clock.BusinessDate) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case a.Before(*b):
		return -1
	case b.Before(*a):
		return 1
	}
	return 0
}

// SortFEFO mengurutkan kandidat: kedaluwarsa (null terakhir), tanggal produksi, nomor batch.
func SortFEFO(cands []Candidate) {
	slices.SortStableFunc(cands, func(a, b Candidate) int {
		if c := compareDate(a.ExpiryDate, b.ExpiryDate); c != 0 {
			return c
		}
		if c := compareDate(a.ProductionDate, b.ProductionDate); c != 0 {
			return c
		}
		return cmp.Compare(a.BatchNumber, b.BatchNumber)
	})
}

// Eligible mengembalikan kandidat yang boleh diusulkan FEFO (stok > 0, belum kedaluwarsa) dalam urutan FEFO.
func Eligible(cands []Candidate, today clock.BusinessDate) []Candidate {
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Qty.IsPositive() && !c.Expired(today) {
			out = append(out, c)
		}
	}
	SortFEFO(out)
	return out
}

// Allocate mengalokasikan qty secara FEFO.
func (FEFO) Allocate(cands []Candidate, qty decimal.Decimal, today clock.BusinessDate) ([]Allocation, decimal.Decimal) {
	remaining := qty
	var out []Allocation
	for _, c := range Eligible(cands, today) {
		if !remaining.IsPositive() {
			break
		}
		take := decimal.Min(c.Qty, remaining)
		out = append(out, Allocation{Candidate: c, Qty: take})
		remaining = remaining.Sub(take)
	}
	return out, remaining
}

// IsFEFOChoice memeriksa apakah batch yang dipilih manual termasuk batch yang akan diambil FEFO untuk qty itu.
// Bila tidak, pemilihan manual wajib alasan (S-215).
func IsFEFOChoice(alloc BatchAllocator, cands []Candidate, batchID *uuid.UUID, qty decimal.Decimal, today clock.BusinessDate) bool {
	allocs, _ := alloc.Allocate(cands, qty, today)
	for _, a := range allocs {
		if SameBatch(a.Candidate.BatchID, batchID) {
			return true
		}
	}
	return false
}
