package domain

import (
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Batch adalah batch produk (INV-04): nomor unik per produk, tanggal produksi, dan tanggal kedaluwarsa.
type Batch struct {
	ID             uuid.UUID
	ProductID      uuid.UUID
	Number         string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
	CreatedAt      time.Time
}

// ExpiryState adalah kondisi kedaluwarsa batch pada satu hari bisnis.
type ExpiryState string

const (
	ExpiryNone    ExpiryState = "no_expiry"
	ExpiryOK      ExpiryState = "ok"
	ExpiryNear    ExpiryState = "near_expiry"
	ExpiryExpired ExpiryState = "expired"
)

// DefaultNearExpiryDays adalah batas "segera kedaluwarsa" di halaman Batch & Kedaluwarsa.
const DefaultNearExpiryDays = 90

// DaysToExpiry mengembalikan sisa hari sampai kedaluwarsa (negatif = lewat); ok false bila tanpa kedaluwarsa.
func (b Batch) DaysToExpiry(today clock.BusinessDate) (int, bool) {
	if b.ExpiryDate == nil {
		return 0, false
	}
	return int(b.ExpiryDate.Time().Sub(today.Time()).Hours() / 24), true
}

// State mengembalikan kondisi kedaluwarsa batch.
func (b Batch) State(today clock.BusinessDate, nearDays int) ExpiryState {
	days, ok := b.DaysToExpiry(today)
	switch {
	case !ok:
		return ExpiryNone
	case days < 0:
		return ExpiryExpired
	case days <= nearDays:
		return ExpiryNear
	}
	return ExpiryOK
}

// ResolveNewBatch melengkapi batch baru dari master produk: kedaluwarsa dihitung dari tanggal produksi + umur
// simpan bila kosong (lihat RequireExpiry untuk batch yang benar-benar baru).
func ResolveNewBatch(productID uuid.UUID, in NewBatch, shelfLifeDays *int, field string) (Batch, error) {
	in, msg := normalizeNewBatch(in)
	if msg != "" {
		return Batch{}, apperr.Validation(map[string]string{field + ".number": msg})
	}
	b := Batch{ProductID: productID, Number: in.Number, ProductionDate: in.ProductionDate, ExpiryDate: in.ExpiryDate}
	if b.ExpiryDate == nil && b.ProductionDate != nil && shelfLifeDays != nil && *shelfLifeDays > 0 {
		exp := b.ProductionDate.AddDays(*shelfLifeDays)
		b.ExpiryDate = &exp
	}
	return b, nil
}

// RequireExpiry menolak batch BARU tanpa tanggal kedaluwarsa untuk produk yang punya umur simpan. Batch yang
// sudah terdaftar boleh dirujuk dengan nomornya saja.
func RequireExpiry(b Batch, shelfLifeDays *int, field string) error {
	if b.ExpiryDate == nil && shelfLifeDays != nil && *shelfLifeDays > 0 {
		return apperr.Validation(map[string]string{field + ".expiryDate": "Tanggal kedaluwarsa atau tanggal produksi wajib diisi"})
	}
	return nil
}

func sameDate(a, b *clock.BusinessDate) bool {
	return a == nil || b == nil || a.Equal(*b)
}

// Matches memeriksa apakah batch baru dengan nomor yang sudah terdaftar tidak bertentangan tanggalnya.
func (b Batch) Matches(in Batch, field string) error {
	fields := map[string]string{}
	if !sameDate(b.ExpiryDate, in.ExpiryDate) {
		fields[field+".expiryDate"] = "Batch " + b.Number + " sudah terdaftar dengan tanggal kedaluwarsa " + b.ExpiryDate.String()
	}
	if !sameDate(b.ProductionDate, in.ProductionDate) {
		fields[field+".productionDate"] = "Batch " + b.Number + " sudah terdaftar dengan tanggal produksi " + b.ProductionDate.String()
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Candidate mengubah batch + qty menjadi kandidat alokasi.
func (b Batch) Candidate() Candidate {
	id := b.ID
	return Candidate{BatchID: &id, BatchNumber: b.Number, ProductionDate: b.ProductionDate, ExpiryDate: b.ExpiryDate}
}
