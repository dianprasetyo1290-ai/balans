// Package domain berisi aturan harga Balans (BRD 5.18): daftar harga & versinya, harga khusus, promo, harga
// bertingkat, batas diskon per role, serta perhitungan diskon manual (MD-04, MD-05, MD-11, MD-12). Tanpa DB,
// HTTP, atau waktu sistem: tanggal "hari ini" selalu dikirim pemanggil.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Period adalah rentang berlaku inklusif ValidFrom..ValidTo (Technical Guideline 5.14). ValidTo nil = tanpa
// tanggal berakhir.
type Period struct {
	ValidFrom clock.BusinessDate
	ValidTo   *clock.BusinessDate
}

// Contains memeriksa apakah tanggal on berada dalam periode.
func (p Period) Contains(on clock.BusinessDate) bool {
	return !on.Before(p.ValidFrom) && (p.ValidTo == nil || !on.After(*p.ValidTo))
}

// Overlaps memeriksa apakah dua periode beririsan.
func (p Period) Overlaps(o Period) bool {
	return (p.ValidTo == nil || !p.ValidTo.Before(o.ValidFrom)) && (o.ValidTo == nil || !o.ValidTo.Before(p.ValidFrom))
}

// PeriodStatus adalah posisi periode terhadap satu tanggal.
type PeriodStatus string

const (
	PeriodScheduled PeriodStatus = "scheduled"
	PeriodActive    PeriodStatus = "active"
	PeriodExpired   PeriodStatus = "expired"
)

// StatusOn mengembalikan status periode pada tanggal on.
func (p Period) StatusOn(on clock.BusinessDate) PeriodStatus {
	switch {
	case on.Before(p.ValidFrom):
		return PeriodScheduled
	case p.ValidTo != nil && on.After(*p.ValidTo):
		return PeriodExpired
	}
	return PeriodActive
}

// Label menampilkan periode untuk pesan, mis. "2026-11-01 s.d. 2026-11-30" atau "mulai 2026-11-01".
func (p Period) Label() string {
	if p.ValidTo == nil {
		return "mulai " + p.ValidFrom.String()
	}
	return p.ValidFrom.String() + " s.d. " + p.ValidTo.String()
}

// validateNew memeriksa periode data effective-dated baru: tidak boleh mulai sebelum hari ini agar transaksi
// lama tidak terpengaruh.
func (p Period) validateNew(f fieldErrors, today clock.BusinessDate) {
	switch {
	case p.ValidFrom.IsZero():
		f.add("validFrom", "Tanggal mulai berlaku wajib diisi")
	case p.ValidFrom.Before(today):
		f.add("validFrom", "Tanggal mulai berlaku tidak boleh sebelum hari ini")
	}
	p.validateEnd(f)
}

func (p Period) validateEnd(f fieldErrors) {
	if p.ValidTo != nil && !p.ValidFrom.IsZero() && p.ValidTo.Before(p.ValidFrom) {
		f.add("validTo", "Tanggal berakhir tidak boleh sebelum tanggal mulai")
	}
}

// EndAt menghitung periode baru saat data effective-dated diakhiri pada validTo (hari terakhir berlaku).
// Tanggal berakhir paling cepat kemarin (berhenti berlaku hari ini) dan tidak sebelum tanggal mulai; data
// yang mulai hari ini atau nanti dihapus saja.
func (p Period) EndAt(validTo, today clock.BusinessDate) (Period, error) {
	f := fieldErrors{}
	switch {
	case validTo.IsZero():
		f.add("validTo", "Tanggal berakhir wajib diisi")
	case validTo.Before(today.AddDays(-1)):
		f.add("validTo", "Tanggal berakhir paling cepat kemarin; periode yang sudah lewat tidak diubah")
	case validTo.Before(p.ValidFrom):
		f.add("validTo", "Tanggal berakhir tidak boleh sebelum tanggal mulai; hapus data yang belum berlaku")
	}
	if err := f.err(); err != nil {
		return p, err
	}
	p.ValidTo = &validTo
	return p, nil
}

// CanDelete: data effective-dated hanya boleh dihapus selama belum pernah berlaku sebelum hari ini.
func (p Period) CanDelete(today clock.BusinessDate) bool { return !p.ValidFrom.Before(today) }

// Supersede menghitung periode baru data lama saat data baru mulai newFrom: data lama tanpa tanggal berakhir
// yang mulai sebelum newFrom ditutup sehari sebelumnya. ok=false bila data lama tidak bisa ditutup otomatis.
func (p Period) Supersede(newFrom clock.BusinessDate) (Period, bool) {
	if p.ValidTo != nil || !p.ValidFrom.Before(newFrom) {
		return p, false
	}
	end := newFrom.AddDays(-1)
	p.ValidTo = &end
	return p, true
}

// fieldErrors mengumpulkan pesan validasi per field (JSON camelCase).
type fieldErrors map[string]string

func (f fieldErrors) add(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	return apperr.Validation(f)
}

func (f fieldErrors) text(field, s string, maxLen int, required bool, label string) string {
	s = strings.TrimSpace(s)
	switch {
	case required && s == "":
		f.add(field, label+" wajib diisi")
	case utf8.RuneCountInString(s) > maxLen:
		f.add(field, fmt.Sprintf("%s maksimal %d karakter", label, maxLen))
	}
	return s
}

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

func (f fieldErrors) code(field, s, label string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch {
	case s == "":
		f.add(field, label+" wajib diisi")
	case len(s) > 30:
		f.add(field, label+" maksimal 30 karakter")
	case !codePattern.MatchString(s):
		f.add(field, label+" hanya huruf kapital, angka, dan tanda minus")
	}
	return s
}

// amount membaca nominal rupiah string desimal ≥ 0 dengan paling banyak 2 angka desimal.
func (f fieldErrors) amount(field, s, label string, required bool) *money.Amount {
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			f.add(field, label+" wajib diisi")
		}
		return nil
	}
	a, err := money.Parse(s)
	switch {
	case err != nil:
		f.add(field, label+" tidak valid")
		return nil
	case a.IsNegative():
		f.add(field, label+" tidak boleh negatif")
		return nil
	case !a.Decimal().Equal(a.Decimal().Round(money.AmountScale)):
		f.add(field, label+" paling banyak 2 angka desimal")
		return nil
	}
	return &a
}

// percent membaca persen string desimal dalam rentang (minExclusive, maxInclusive], paling banyak 4 desimal.
func (f fieldErrors) percent(field, s, label string, lowInclusive bool, high decimal.Decimal) *decimal.Decimal {
	s = strings.TrimSpace(s)
	if s == "" {
		f.add(field, label+" wajib diisi")
		return nil
	}
	d, err := decimal.NewFromString(s)
	switch {
	case err != nil:
		f.add(field, label+" tidak valid")
		return nil
	case lowInclusive && d.IsNegative(), !lowInclusive && !d.IsPositive(), d.GreaterThan(high):
		low := "lebih dari 0"
		if lowInclusive {
			low = "0"
		}
		f.add(field, fmt.Sprintf("%s harus %s sampai %s", label, low, high.String()))
		return nil
	case !d.Equal(d.Round(money.RateScale)):
		f.add(field, label+" paling banyak 4 angka desimal")
		return nil
	}
	return &d
}

var hundred = decimal.NewFromInt(100)
