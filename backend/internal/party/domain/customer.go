package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
)

// CustomerType adalah tipe pelanggan (MD-06).
type CustomerType string

const (
	CustomerRetail   CustomerType = "retail"
	CustomerB2B      CustomerType = "b2b"
	CustomerReseller CustomerType = "reseller"
	// CustomerEmployee: pelanggan yang tertaut ke karyawan; harga karyawan berlaku selama karyawan aktif (HR-03).
	CustomerEmployee CustomerType = "employee"
)

// Valid memeriksa tipe pelanggan.
func (t CustomerType) Valid() bool {
	switch t {
	case CustomerRetail, CustomerB2B, CustomerReseller, CustomerEmployee:
		return true
	}
	return false
}

// MaxAddresses membatasi jumlah alamat per pelanggan.
const MaxAddresses = 20

// Address adalah alamat pengiriman pelanggan. ID uuid.Nil = alamat baru.
type Address struct {
	ID            uuid.UUID
	Label         string
	RecipientName string
	Phone         string
	AddressLine   string
	District      string
	City          string
	Province      string
	PostalCode    string
	Latitude      *decimal.Decimal
	Longitude     *decimal.Decimal
	IsDefault     bool
}

// Customer adalah master pelanggan.
type Customer struct {
	ID       uuid.UUID
	Code     string
	FullName string
	// Phone adalah nomor ternormalisasi +62…, dipakai untuk mencegah duplikat.
	Phone      string
	Email      string
	Type       CustomerType
	EmployeeID *uuid.UUID
	// MarketingConsentAt adalah waktu pelanggan menyetujui pesan pemasaran (NFR-04, TG 6.7); nil = belum/ditarik.
	MarketingConsentAt      *time.Time
	MarketingConsentVersion string
	Notes                   string
	Addresses               []Address
	CreatedAt               time.Time
	UpdatedAt               time.Time
	Version                 int
}

// AddressInput adalah isian alamat. Latitude/Longitude berupa string desimal.
type AddressInput struct {
	ID            uuid.UUID
	Label         string
	RecipientName string
	Phone         string
	AddressLine   string
	District      string
	City          string
	Province      string
	PostalCode    string
	Latitude      *string
	Longitude     *string
	IsDefault     bool
}

// CustomerInput adalah isian pelanggan.
type CustomerInput struct {
	FullName                string
	Phone                   string
	Email                   string
	Type                    CustomerType
	EmployeeID              *uuid.UUID
	MarketingConsent        bool
	MarketingConsentVersion string
	Notes                   string
	Addresses               []AddressInput
}

func parseCoordinate(f fieldErrors, field string, s *string, limit int64) *decimal.Decimal {
	if s == nil || *s == "" {
		return nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil || d.Abs().GreaterThan(decimal.NewFromInt(limit)) {
		f.add(field, "Koordinat tidak valid")
		return nil
	}
	d = d.Round(6)
	return &d
}

func validateAddresses(f fieldErrors, in []AddressInput) []Address {
	if len(in) > MaxAddresses {
		f.add("addresses", fmt.Sprintf("Maksimal %d alamat", MaxAddresses))
		return nil
	}
	out := make([]Address, len(in))
	defaults := 0
	seen := map[uuid.UUID]bool{}
	for i, a := range in {
		p := fmt.Sprintf("addresses.%d.", i)
		if a.ID != uuid.Nil {
			if seen[a.ID] {
				f.add(p+"id", "Alamat ganda")
			}
			seen[a.ID] = true
		}
		addr := Address{
			ID:            a.ID,
			Label:         f.text(p+"label", a.Label, 40, false, "Label"),
			RecipientName: f.text(p+"recipientName", a.RecipientName, 120, true, "Nama penerima"),
			AddressLine:   f.text(p+"addressLine", a.AddressLine, 500, true, "Alamat"),
			District:      f.text(p+"district", a.District, 80, false, "Kecamatan"),
			City:          f.text(p+"city", a.City, 80, true, "Kota/Kabupaten"),
			Province:      f.text(p+"province", a.Province, 80, false, "Provinsi"),
			PostalCode:    f.text(p+"postalCode", a.PostalCode, 10, false, "Kode pos"),
			Latitude:      parseCoordinate(f, p+"latitude", a.Latitude, 90),
			Longitude:     parseCoordinate(f, p+"longitude", a.Longitude, 180),
			IsDefault:     a.IsDefault,
		}
		if a.Phone != "" {
			if n, ok := NormalizePhone(a.Phone); ok {
				addr.Phone = n
			} else {
				f.add(p+"phone", "Nomor HP tidak valid")
			}
		}
		if (addr.Latitude == nil) != (addr.Longitude == nil) {
			f.add(p+"latitude", "Isi latitude dan longitude bersamaan")
		}
		if a.IsDefault {
			defaults++
		}
		out[i] = addr
	}
	switch {
	case defaults > 1:
		f.add("addresses", "Hanya satu alamat utama")
	case defaults == 0 && len(out) > 0:
		// Alamat pertama menjadi alamat utama jika tidak ada yang ditandai.
		out[0].IsDefault = true
	}
	return out
}

func (c *Customer) apply(in CustomerInput, now time.Time) error {
	f := fieldErrors{}
	c.FullName = f.text("fullName", in.FullName, 120, true, "Nama lengkap")
	if phone, ok := NormalizePhone(in.Phone); ok {
		c.Phone = phone
	} else if in.Phone == "" {
		f.add("phone", "Nomor HP wajib diisi")
	} else {
		f.add("phone", "Nomor HP tidak valid. Contoh: 0812-3456-7890")
	}
	c.Email = f.email("email", in.Email)
	if !in.Type.Valid() {
		f.add("customerType", "Tipe pelanggan tidak dikenal")
	}
	c.Type = in.Type
	switch {
	case in.Type == CustomerEmployee && in.EmployeeID == nil:
		f.add("employeeId", "Pilih karyawan untuk pelanggan bertipe karyawan")
	case in.Type != CustomerEmployee && in.EmployeeID != nil:
		f.add("customerType", "Pelanggan yang tertaut ke karyawan harus bertipe karyawan")
	}
	c.EmployeeID = in.EmployeeID
	version := f.text("marketingConsentVersion", in.MarketingConsentVersion, 40, false, "Versi persetujuan")
	switch {
	case !in.MarketingConsent:
		c.MarketingConsentAt, c.MarketingConsentVersion = nil, ""
	case c.MarketingConsentAt == nil || c.MarketingConsentVersion != version:
		// Persetujuan baru atau versi teks berubah: waktu persetujuan dicatat ulang.
		at := now
		c.MarketingConsentAt, c.MarketingConsentVersion = &at, version
	}
	c.Notes = f.text("notes", in.Notes, 1000, false, "Catatan")
	c.Addresses = validateAddresses(f, in.Addresses)
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewCustomer memvalidasi pelanggan baru. now adalah waktu persetujuan pemasaran bila diberikan.
func NewCustomer(in CustomerInput, now time.Time) (Customer, error) {
	var c Customer
	if err := c.apply(in, now); err != nil {
		return Customer{}, err
	}
	return c, nil
}

// Update menerapkan perubahan. Alamat dengan ID yang bukan milik pelanggan ditolak.
func (c *Customer) Update(in CustomerInput, now time.Time) error {
	owned := map[uuid.UUID]bool{}
	for _, a := range c.Addresses {
		owned[a.ID] = true
	}
	for i, a := range in.Addresses {
		if a.ID != uuid.Nil && !owned[a.ID] {
			return apperr.Validation(map[string]string{fmt.Sprintf("addresses.%d.id", i): "Alamat tidak ditemukan"})
		}
	}
	return c.apply(in, now)
}

// HasMarketingConsent bernilai true jika pelanggan menyetujui pesan pemasaran.
func (c Customer) HasMarketingConsent() bool { return c.MarketingConsentAt != nil }

// RemovedAddresses mengembalikan ID alamat lama yang tidak ada di daftar baru.
func RemovedAddresses(before, after []Address) []uuid.UUID {
	keep := map[uuid.UUID]bool{}
	for _, a := range after {
		keep[a.ID] = true
	}
	var out []uuid.UUID
	for _, a := range before {
		if !keep[a.ID] {
			out = append(out, a.ID)
		}
	}
	return out
}
