package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// AgreementType adalah jenis PKS.
type AgreementType string

const (
	AgreementConsignment AgreementType = "consignment"
	AgreementModernTrade AgreementType = "modern_trade"
)

// Valid memeriksa jenis PKS.
func (t AgreementType) Valid() bool {
	return t == AgreementConsignment || t == AgreementModernTrade
}

// Label adalah nama jenis PKS untuk pesan.
func (t AgreementType) Label() string {
	if t == AgreementModernTrade {
		return "Modern Trade"
	}
	return "konsinyasi"
}

// BorneBy adalah pihak yang menanggung selisih rekonsiliasi konsinyasi (BRD 5.5, U-02).
type BorneBy string

const (
	BorneByPartner BorneBy = "partner"
	BorneByBalans  BorneBy = "balans"
)

// Valid memeriksa nilai penanggung.
func (b BorneBy) Valid() bool { return b == BorneByPartner || b == BorneByBalans }

// AgreementStatus adalah status PKS pada satu tanggal.
type AgreementStatus string

const (
	AgreementScheduled AgreementStatus = "scheduled"
	AgreementActive    AgreementStatus = "active"
	AgreementExpired   AgreementStatus = "expired"
)

// MaxPaymentTermDays membatasi termin pembayaran.
const MaxPaymentTermDays = 365

// Agreement adalah PKS mitra yang berlaku pada rentang tanggal inklusif (effective-dated, TG 5.14).
type Agreement struct {
	ID        uuid.UUID
	PartnerID uuid.UUID
	Number    string
	Type      AgreementType
	ValidFrom clock.BusinessDate
	ValidTo   clock.BusinessDate
	// FileID adalah PDF PKS di modul filestore.
	FileID *uuid.UUID
	// PriceListID adalah daftar harga konsinyasi/MT di modul pricing.
	PriceListID *uuid.UUID
	// MarginPercent adalah margin/komisi mitra dari harga jual (persen).
	MarginPercent        *decimal.Decimal
	PaymentTermDays      int
	LossBorneBy          BorneBy
	PartnerDamageBorneBy BorneBy
	ProductDefectBorneBy BorneBy
	NearExpiryBorneBy    BorneBy
	Notes                string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Version              int
}

// AgreementInput adalah isian PKS. Penanggung kosong memakai default BRD U-02: hilang dan rusak karena
// partner ditanggung partner; cacat produk dan mendekati kedaluwarsa ditanggung Balans.
type AgreementInput struct {
	Number               string
	Type                 AgreementType
	ValidFrom            clock.BusinessDate
	ValidTo              clock.BusinessDate
	FileID               *uuid.UUID
	PriceListID          *uuid.UUID
	MarginPercent        *string
	PaymentTermDays      int
	LossBorneBy          BorneBy
	PartnerDamageBorneBy BorneBy
	ProductDefectBorneBy BorneBy
	NearExpiryBorneBy    BorneBy
	Notes                string
}

func borneBy(f fieldErrors, field string, v, fallback BorneBy) BorneBy {
	if v == "" {
		return fallback
	}
	if !v.Valid() {
		f.add(field, "Pilih partner atau Balans")
	}
	return v
}

func (a *Agreement) apply(in AgreementInput) error {
	f := fieldErrors{}
	a.Number = f.text("agreementNumber", in.Number, 60, true, "Nomor PKS")
	if !in.Type.Valid() {
		f.add("agreementType", "Jenis PKS tidak dikenal")
	}
	a.Type = in.Type
	switch {
	case in.ValidFrom.IsZero():
		f.add("validFrom", "Tanggal mulai wajib diisi")
	case in.ValidTo.IsZero():
		f.add("validTo", "Tanggal berakhir wajib diisi")
	case in.ValidTo.Before(in.ValidFrom):
		f.add("validTo", "Tanggal berakhir tidak boleh sebelum tanggal mulai")
	}
	a.ValidFrom, a.ValidTo = in.ValidFrom, in.ValidTo
	a.FileID, a.PriceListID = in.FileID, in.PriceListID
	a.MarginPercent = nil
	if in.MarginPercent != nil && *in.MarginPercent != "" {
		d, err := decimal.NewFromString(*in.MarginPercent)
		switch {
		case err != nil:
			f.add("consignmentMarginPercent", "Persentase tidak valid")
		case d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)):
			f.add("consignmentMarginPercent", "Persentase harus 0 sampai 100")
		default:
			d = d.Round(4)
			a.MarginPercent = &d
		}
	}
	if in.PaymentTermDays < 0 || in.PaymentTermDays > MaxPaymentTermDays {
		f.add("paymentTermDays", "Termin pembayaran 0 sampai 365 hari")
	}
	a.PaymentTermDays = in.PaymentTermDays
	a.LossBorneBy = borneBy(f, "lossBorneBy", in.LossBorneBy, BorneByPartner)
	a.PartnerDamageBorneBy = borneBy(f, "partnerDamageBorneBy", in.PartnerDamageBorneBy, BorneByPartner)
	a.ProductDefectBorneBy = borneBy(f, "productDefectBorneBy", in.ProductDefectBorneBy, BorneByBalans)
	a.NearExpiryBorneBy = borneBy(f, "nearExpiryBorneBy", in.NearExpiryBorneBy, BorneByBalans)
	a.Notes = f.text("notes", in.Notes, 1000, false, "Catatan")
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewAgreement memvalidasi PKS baru untuk mitra.
func NewAgreement(partnerID uuid.UUID, in AgreementInput) (Agreement, error) {
	a := Agreement{PartnerID: partnerID}
	if err := a.apply(in); err != nil {
		return Agreement{}, err
	}
	return a, nil
}

// Update mengoreksi PKS; mitra tetap.
func (a *Agreement) Update(in AgreementInput) error { return a.apply(in) }

// ActiveOn memeriksa apakah PKS berlaku pada tanggal bisnis.
func (a Agreement) ActiveOn(d clock.BusinessDate) bool {
	return !d.Before(a.ValidFrom) && !d.After(a.ValidTo)
}

// StatusOn adalah status PKS pada tanggal bisnis.
func (a Agreement) StatusOn(d clock.BusinessDate) AgreementStatus {
	switch {
	case d.Before(a.ValidFrom):
		return AgreementScheduled
	case d.After(a.ValidTo):
		return AgreementExpired
	}
	return AgreementActive
}

// RequireActive menolak transaksi bila tidak ada PKS jenis t yang berlaku pada tanggal d, mis. DO konsinyasi
// untuk mitra yang PKS-nya kedaluwarsa (BRD 5.5).
func RequireActive(a *Agreement, t AgreementType, partnerName string, d clock.BusinessDate) error {
	if a != nil && a.Type == t && a.ActiveOn(d) {
		return nil
	}
	return apperr.Validation(map[string]string{
		"partnerId": "PKS " + t.Label() + " " + partnerName + " tidak berlaku pada " + d.String() +
			". Perbarui PKS di Data Master › Mitra Konsinyasi.",
	})
}
