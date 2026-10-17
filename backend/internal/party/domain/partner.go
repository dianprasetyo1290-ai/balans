package domain

import (
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// PartnerType adalah jenis mitra (MD-07). Satu mitra bisa MT dan konsinyasi lewat dua PKS.
type PartnerType string

const (
	PartnerModernTrade PartnerType = "modern_trade"
	PartnerConsignment PartnerType = "consignment"
	PartnerB2B         PartnerType = "b2b"
)

// Valid memeriksa jenis mitra.
func (t PartnerType) Valid() bool {
	switch t {
	case PartnerModernTrade, PartnerConsignment, PartnerB2B:
		return true
	}
	return false
}

// Partner adalah master mitra MT/konsinyasi/B2B.
type Partner struct {
	ID   uuid.UUID
	Code string
	Name string
	Type PartnerType
	// NPWP adalah NPWP terdekripsi (digit saja); disimpan terenkripsi.
	NPWP               string
	Address            string
	PICName            string
	PICPhone           string
	SalesUserID        *uuid.UUID
	MerchandiserUserID *uuid.UUID
	Notes              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int
}

// PartnerInput adalah isian mitra. NPWP nil = tidak berubah (saat update).
type PartnerInput struct {
	Code               string
	Name               string
	Type               PartnerType
	NPWP               *string
	Address            string
	PICName            string
	PICPhone           string
	SalesUserID        *uuid.UUID
	MerchandiserUserID *uuid.UUID
	Notes              string
}

func applyNPWP(f fieldErrors, cur *string, in *string) {
	if in == nil {
		return
	}
	if *in == "" {
		*cur = ""
		return
	}
	n, ok := NormalizeNPWP(*in)
	if !ok {
		f.add("npwp", "NPWP harus 15 atau 16 digit")
		return
	}
	*cur = n
}

func (p *Partner) apply(f fieldErrors, in PartnerInput) error {
	p.Name = f.text("name", in.Name, 160, true, "Nama mitra")
	if !in.Type.Valid() {
		f.add("partnerType", "Jenis mitra tidak dikenal")
	}
	p.Type = in.Type
	applyNPWP(f, &p.NPWP, in.NPWP)
	p.Address = f.text("address", in.Address, 500, false, "Alamat")
	p.PICName = f.text("picName", in.PICName, 120, false, "Nama PIC")
	p.PICPhone = ""
	if in.PICPhone != "" {
		if n, ok := NormalizePhone(in.PICPhone); ok {
			p.PICPhone = n
		} else {
			f.add("picPhone", "Nomor HP tidak valid")
		}
	}
	p.SalesUserID, p.MerchandiserUserID = in.SalesUserID, in.MerchandiserUserID
	p.Notes = f.text("notes", in.Notes, 1000, false, "Catatan")
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewPartner memvalidasi mitra baru. Kode disimpan huruf besar dan tidak bisa diubah.
func NewPartner(in PartnerInput) (Partner, error) {
	f := fieldErrors{}
	p := Partner{Code: f.code("code", in.Code, "Kode mitra")}
	if err := p.apply(f, in); err != nil {
		return Partner{}, err
	}
	return p, nil
}

// Update menerapkan perubahan; kode tetap.
func (p *Partner) Update(in PartnerInput) error { return p.apply(fieldErrors{}, in) }
