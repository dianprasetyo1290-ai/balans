package domain

import (
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/softdelete"
)

// PriceListType adalah cakupan daftar harga (BRD 5.18).
type PriceListType string

const (
	// TypeBase adalah Harga Dasar & HET: acuan semua channel, satu-satunya (prioritas 5).
	TypeBase PriceListType = "base"
	// TypeChannel berlaku per channel atau sub-channel (MT beli putus, marketplace, website, WA/IG, B2B).
	TypeChannel PriceListType = "channel"
	// TypePartner berlaku untuk satu mitra MT/konsinyasi (atau ditautkan ke PKS mitra).
	TypePartner PriceListType = "partner"
	// TypeReseller berlaku per level reseller; dipakai mulai Fase 2 (modul reseller).
	TypeReseller PriceListType = "reseller"
	// TypeEmployee berlaku untuk pelanggan yang tertaut ke karyawan aktif (HR-03, S-132).
	TypeEmployee PriceListType = "employee"
	// TypeVending berlaku untuk channel vending, seluruh mesin atau per mesin (sub-channel).
	TypeVending PriceListType = "vending"
)

var typeLabels = map[PriceListType]string{
	TypeBase: "Harga Dasar & HET", TypeChannel: "Channel", TypePartner: "Mitra", TypeReseller: "Level reseller",
	TypeEmployee: "Karyawan", TypeVending: "Vending",
}

// Valid memeriksa jenis daftar harga.
func (t PriceListType) Valid() bool {
	_, ok := typeLabels[t]
	return ok
}

// Label adalah nama jenis untuk pesan.
func (t PriceListType) Label() string { return typeLabels[t] }

// Basis adalah cara harga daftar ditentukan.
type Basis string

const (
	// BasisFixed: harga per produk diisi di item versi.
	BasisFixed Basis = "fixed"
	// BasisPercent: harga = harga daftar acuan × persen versi (mis. karyawan 90% dari harga website).
	BasisPercent Basis = "percent"
)

// PriceList adalah header daftar harga. Harga ada di versi (PriceListVersion).
type PriceList struct {
	ID           uuid.UUID
	Code         string
	Name         string
	Type         PriceListType
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	PartnerID    *uuid.UUID
	// ResellerLevel adalah kode level reseller (Fase 2).
	ResellerLevel string
	Basis         Basis
	// ReferenceID adalah daftar harga acuan untuk basis percent (harus berbasis fixed).
	ReferenceID *uuid.UUID
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
	softdelete.Mark
}

// PriceListInput adalah isian Tambah/Ubah Daftar Harga. Code dan Type hanya dipakai saat membuat.
type PriceListInput struct {
	Code          string
	Name          string
	Type          PriceListType
	ChannelID     *uuid.UUID
	SubChannelID  *uuid.UUID
	PartnerID     *uuid.UUID
	ResellerLevel string
	Basis         Basis
	ReferenceID   *uuid.UUID
	Description   string
	IsActive      bool
}

// NewPriceList membuat daftar harga baru. Daftar Harga Dasar & HET sudah disemai migrasi dan tidak dibuat ulang.
func NewPriceList(in PriceListInput) (PriceList, error) {
	f := fieldErrors{}
	p := PriceList{Code: f.code("code", in.Code, "Kode daftar harga"), Type: in.Type}
	switch {
	case !in.Type.Valid():
		f.add("priceListType", "Jenis daftar harga tidak dikenal")
	case in.Type == TypeBase:
		f.add("priceListType", "Daftar Harga Dasar & HET sudah tersedia; ubah harganya lewat versi baru")
	case in.Type == TypeReseller:
		f.add("priceListType", "Daftar harga per level reseller tersedia mulai Fase 2 (modul reseller)")
	}
	p.apply(f, in)
	if err := f.err(); err != nil {
		return PriceList{}, err
	}
	return p, nil
}

// Update mengubah nama, cakupan, basis, dan status. Kode dan jenis tetap.
func (p *PriceList) Update(in PriceListInput) error {
	if err := p.EnsureActive(); err != nil {
		return err
	}
	f := fieldErrors{}
	next := *p
	next.apply(f, in)
	if err := f.err(); err != nil {
		return err
	}
	*p = next
	return nil
}

func (p *PriceList) apply(f fieldErrors, in PriceListInput) {
	p.Name = f.text("name", in.Name, 120, true, "Nama daftar harga")
	p.Description = f.text("description", in.Description, 500, false, "Keterangan")
	p.IsActive = in.IsActive
	p.ChannelID, p.SubChannelID, p.PartnerID, p.ResellerLevel = nil, nil, nil, ""
	switch p.Type {
	case TypeChannel, TypeVending:
		if in.ChannelID == nil {
			f.add("channelId", "Pilih channel")
		}
		p.ChannelID, p.SubChannelID = in.ChannelID, in.SubChannelID
	case TypePartner:
		if in.PartnerID == nil {
			f.add("partnerId", "Pilih mitra")
		}
		p.PartnerID = in.PartnerID
	case TypeReseller:
		p.ResellerLevel = f.text("resellerLevel", in.ResellerLevel, 30, true, "Level reseller")
	}
	if p.Type == TypeBase && !p.IsActive {
		f.add("isActive", "Daftar Harga Dasar & HET tidak bisa dinonaktifkan")
	}
	p.Basis, p.ReferenceID = in.Basis, nil
	switch {
	case in.Basis == "" || in.Basis == BasisFixed:
		p.Basis = BasisFixed
	case in.Basis != BasisPercent:
		f.add("basis", "Basis harga tidak dikenal")
	case p.Type == TypeBase:
		f.add("basis", "Harga Dasar & HET selalu diisi per produk")
	case in.ReferenceID == nil:
		f.add("referencePriceListId", "Pilih daftar harga acuan")
	case p.ID != uuid.Nil && *in.ReferenceID == p.ID:
		f.add("referencePriceListId", "Daftar harga acuan tidak boleh dirinya sendiri")
	default:
		p.ReferenceID = in.ReferenceID
	}
}

// ValidateReference memeriksa daftar harga acuan basis percent: belum dihapus dan berbasis fixed (tidak
// berantai).
func (p PriceList) ValidateReference(ref PriceList) error {
	f := fieldErrors{}
	switch {
	case ref.Deleted():
		f.add("referencePriceListId", "Daftar harga acuan sudah dihapus")
	case ref.Basis != BasisFixed:
		f.add("referencePriceListId", "Daftar harga acuan harus berisi harga per produk (bukan persentase)")
	}
	return f.err()
}

// Usable: daftar harga dipakai resolver bila aktif dan belum dihapus.
func (p PriceList) Usable() bool { return p.IsActive && !p.Deleted() }

// msgBaseUndeletable: daftar Harga Dasar & HET selalu ada.
const msgBaseUndeletable = "Daftar Harga Dasar & HET tidak bisa dihapus."

// DeleteError mengembalikan pesan bila daftar harga tidak boleh dihapus, atau "".
func (p PriceList) DeleteError() string {
	if p.Type == TypeBase {
		return msgBaseUndeletable
	}
	return ""
}
