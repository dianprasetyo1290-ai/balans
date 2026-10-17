package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// SupplierType adalah jenis pemasok (MD-08).
type SupplierType string

const (
	SupplierRegular SupplierType = "supplier"
	// SupplierTollManufacturer adalah maklon.
	SupplierTollManufacturer SupplierType = "toll_manufacturer"
)

// Valid memeriksa jenis pemasok.
func (t SupplierType) Valid() bool { return t == SupplierRegular || t == SupplierTollManufacturer }

// WithholdingType adalah PPh yang dipotong default saat membayar tagihan pemasok.
type WithholdingType string

const (
	WithholdingNone  WithholdingType = "none"
	WithholdingPPh23 WithholdingType = "pph23"
	WithholdingPPh21 WithholdingType = "pph21"
	WithholdingPPh42 WithholdingType = "pph4_2"
)

// Valid memeriksa jenis PPh.
func (t WithholdingType) Valid() bool {
	switch t {
	case WithholdingNone, WithholdingPPh23, WithholdingPPh21, WithholdingPPh42:
		return true
	}
	return false
}

// MaxBankAccounts membatasi rekening per pemasok.
const MaxBankAccounts = 5

// BankAccount adalah rekening pemasok. AccountNumber terdekripsi (digit saja); disimpan terenkripsi.
type BankAccount struct {
	ID            uuid.UUID
	BankName      string
	AccountNumber string
	AccountName   string
	IsDefault     bool
}

// Supplier adalah master pemasok atau maklon.
type Supplier struct {
	ID               uuid.UUID
	Code             string
	Name             string
	Type             SupplierType
	IsBusinessEntity bool
	// NPWP terdekripsi (digit saja).
	NPWP               string
	DefaultWithholding WithholdingType
	PaymentTermDays    int
	Address            string
	Phone              string
	Email              string
	PICName            string
	Notes              string
	BankAccounts       []BankAccount
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int
}

// BankAccountInput adalah isian rekening. AccountNumber kosong pada rekening lama = nomor tidak berubah.
type BankAccountInput struct {
	ID            uuid.UUID
	BankName      string
	AccountNumber string
	AccountName   string
	IsDefault     bool
}

// SupplierInput adalah isian pemasok. NPWP nil = tidak berubah.
type SupplierInput struct {
	Code               string
	Name               string
	Type               SupplierType
	IsBusinessEntity   bool
	NPWP               *string
	DefaultWithholding WithholdingType
	PaymentTermDays    int
	Address            string
	Phone              string
	Email              string
	PICName            string
	Notes              string
	BankAccounts       []BankAccountInput
}

func (s *Supplier) applyBankAccounts(f fieldErrors, in []BankAccountInput) {
	if len(in) > MaxBankAccounts {
		f.add("bankAccounts", fmt.Sprintf("Maksimal %d rekening", MaxBankAccounts))
		return
	}
	existing := map[uuid.UUID]BankAccount{}
	for _, b := range s.BankAccounts {
		existing[b.ID] = b
	}
	out := make([]BankAccount, len(in))
	defaults := 0
	for i, b := range in {
		p := fmt.Sprintf("bankAccounts.%d.", i)
		acc := BankAccount{
			ID:          b.ID,
			BankName:    f.text(p+"bankName", b.BankName, 80, true, "Nama bank"),
			AccountName: f.text(p+"accountName", b.AccountName, 120, true, "Nama pemilik rekening"),
			IsDefault:   b.IsDefault,
		}
		old, isOld := existing[b.ID]
		switch {
		case b.ID != uuid.Nil && !isOld:
			f.add(p+"id", "Rekening tidak ditemukan")
		case b.AccountNumber == "" && isOld:
			acc.AccountNumber = old.AccountNumber
		case b.AccountNumber == "":
			f.add(p+"accountNumber", "Nomor rekening wajib diisi")
		default:
			if n, ok := NormalizeAccountNumber(b.AccountNumber); ok {
				acc.AccountNumber = n
			} else {
				f.add(p+"accountNumber", "Nomor rekening 5 sampai 20 digit")
			}
		}
		if b.IsDefault {
			defaults++
		}
		out[i] = acc
	}
	switch {
	case defaults > 1:
		f.add("bankAccounts", "Hanya satu rekening utama")
	case defaults == 0 && len(out) > 0:
		out[0].IsDefault = true
	}
	s.BankAccounts = out
}

func (s *Supplier) apply(f fieldErrors, in SupplierInput) error {
	s.Name = f.text("name", in.Name, 160, true, "Nama pemasok")
	if !in.Type.Valid() {
		f.add("supplierType", "Jenis pemasok tidak dikenal")
	}
	s.Type, s.IsBusinessEntity = in.Type, in.IsBusinessEntity
	applyNPWP(f, &s.NPWP, in.NPWP)
	if !in.DefaultWithholding.Valid() {
		f.add("defaultWithholding", "Jenis PPh tidak dikenal")
	}
	s.DefaultWithholding = in.DefaultWithholding
	if in.PaymentTermDays < 0 || in.PaymentTermDays > MaxPaymentTermDays {
		f.add("paymentTermDays", "Termin pembayaran 0 sampai 365 hari")
	}
	s.PaymentTermDays = in.PaymentTermDays
	s.Address = f.text("address", in.Address, 500, false, "Alamat")
	s.Phone = ""
	if in.Phone != "" {
		if n, ok := NormalizePhone(in.Phone); ok {
			s.Phone = n
		} else {
			f.add("phone", "Nomor telepon tidak valid")
		}
	}
	s.Email = f.email("email", in.Email)
	s.PICName = f.text("picName", in.PICName, 120, false, "Nama PIC")
	s.Notes = f.text("notes", in.Notes, 1000, false, "Catatan")
	s.applyBankAccounts(f, in.BankAccounts)
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewSupplier memvalidasi pemasok baru. Kode disimpan huruf besar dan tidak bisa diubah.
func NewSupplier(in SupplierInput) (Supplier, error) {
	f := fieldErrors{}
	s := Supplier{Code: f.code("code", in.Code, "Kode pemasok")}
	if err := s.apply(f, in); err != nil {
		return Supplier{}, err
	}
	return s, nil
}

// Update menerapkan perubahan; kode tetap.
func (s *Supplier) Update(in SupplierInput) error { return s.apply(fieldErrors{}, in) }

// IsTollManufacturer bernilai true untuk maklon.
func (s Supplier) IsTollManufacturer() bool { return s.Type == SupplierTollManufacturer }

// DefaultBankAccount mengembalikan rekening utama, jika ada.
func (s Supplier) DefaultBankAccount() (BankAccount, bool) {
	for _, b := range s.BankAccounts {
		if b.IsDefault {
			return b, true
		}
	}
	return BankAccount{}, false
}

// RemovedBankAccounts mengembalikan ID rekening lama yang tidak ada di daftar baru.
func RemovedBankAccounts(before, after []BankAccount) []uuid.UUID {
	keep := map[uuid.UUID]bool{}
	for _, b := range after {
		keep[b.ID] = true
	}
	var out []uuid.UUID
	for _, b := range before {
		if !keep[b.ID] {
			out = append(out, b.ID)
		}
	}
	return out
}
