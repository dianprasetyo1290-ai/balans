package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Dimension adalah dimensi jurnal (Technical Guideline 5.8.2): C channel, S sub-channel, D divisi, K kampanye.
type Dimension string

const (
	DimensionChannel    Dimension = "channel"
	DimensionSubChannel Dimension = "sub_channel"
	DimensionDivision   Dimension = "division"
	DimensionCampaign   Dimension = "campaign"
)

var dimensionLabels = map[Dimension]string{
	DimensionChannel: "Channel", DimensionSubChannel: "Sub-channel", DimensionDivision: "Divisi", DimensionCampaign: "Kampanye",
}

// Label mengembalikan nama dimensi untuk UI.
func (d Dimension) Label() string { return dimensionLabels[d] }

// Field mengembalikan nama field API (camelCase + Id).
func (d Dimension) Field() string {
	switch d {
	case DimensionChannel:
		return "channelId"
	case DimensionSubChannel:
		return "subChannelId"
	case DimensionDivision:
		return "divisionId"
	case DimensionCampaign:
		return "campaignId"
	}
	return string(d)
}

// ParseDimension membaca kode dimensi.
func ParseDimension(s string) (Dimension, bool) {
	d := Dimension(s)
	_, ok := dimensionLabels[d]
	return d, ok
}

// Dimensions adalah dimensi yang dibawa setiap baris jurnal (ACC-17). Nil = tidak diisi.
type Dimensions struct {
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	DivisionID   *uuid.UUID
	CampaignID   *uuid.UUID
}

// Has memeriksa apakah dimensi diisi.
func (d Dimensions) Has(dim Dimension) bool {
	switch dim {
	case DimensionChannel:
		return d.ChannelID != nil
	case DimensionSubChannel:
		return d.SubChannelID != nil
	case DimensionDivision:
		return d.DivisionID != nil
	case DimensionCampaign:
		return d.CampaignID != nil
	}
	return false
}

// Missing mengembalikan error per field untuk dimensi wajib yang kosong, dengan awalan field (mis. `lines.0.`).
// Sub-channel tanpa channel juga ditolak.
func (d Dimensions) Missing(required []Dimension, prefix, context string) map[string]string {
	fields := map[string]string{}
	for _, dim := range required {
		if !d.Has(dim) {
			fields[prefix+dim.Field()] = fmt.Sprintf("%s wajib diisi untuk %s", dim.Label(), context)
		}
	}
	if d.SubChannelID != nil && d.ChannelID == nil {
		fields[prefix+DimensionChannel.Field()] = "Channel wajib diisi bila sub-channel diisi"
	}
	return fields
}

// DimensionCheck adalah hasil pemeriksaan dimensi di DB: ada, aktif, sub-channel milik channel, dan
// kampanye berlaku pada tanggal transaksi.
type DimensionCheck struct {
	ChannelOK    bool
	SubChannelOK bool
	DivisionOK   bool
	CampaignOK   bool
}

// Errors mengubah hasil pemeriksaan menjadi error per field.
func (c DimensionCheck) Errors(prefix string) map[string]string {
	fields := map[string]string{}
	if !c.ChannelOK {
		fields[prefix+"channelId"] = "Channel tidak ditemukan atau nonaktif"
	}
	if !c.SubChannelOK {
		fields[prefix+"subChannelId"] = "Sub-channel tidak ditemukan, nonaktif, atau bukan bagian dari channel"
	}
	if !c.DivisionOK {
		fields[prefix+"divisionId"] = "Divisi tidak ditemukan atau nonaktif"
	}
	if !c.CampaignOK {
		fields[prefix+"campaignId"] = "Kampanye tidak ditemukan, nonaktif, atau tidak berlaku pada tanggal transaksi"
	}
	return fields
}

// DimensionKind adalah jenis master dimensi.
type DimensionKind string

const (
	KindChannel    DimensionKind = "channel"
	KindSubChannel DimensionKind = "sub_channel"
	KindDivision   DimensionKind = "division"
	KindCampaign   DimensionKind = "campaign"
)

// Label mengembalikan nama jenis master untuk audit trail dan pesan.
func (k DimensionKind) Label() string {
	switch k {
	case KindChannel:
		return "Channel"
	case KindSubChannel:
		return "Sub-channel"
	case KindDivision:
		return "Divisi"
	case KindCampaign:
		return "Kampanye"
	}
	return string(k)
}

var (
	channelCodePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	subChannelCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,49}$`)
	divisionCodePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
	campaignCodePattern   = regexp.MustCompile(`^KMP-([0-9]{4})(0[1-9]|1[0-2])-[A-Z0-9]{2,30}$`)
)

// DimensionValue adalah satu baris master dimensi: channel, sub-channel, divisi, atau kampanye.
type DimensionValue struct {
	ID   uuid.UUID
	Kind DimensionKind
	Code string
	Name string
	// ChannelID: induk sub-channel.
	ChannelID *uuid.UUID
	// PartyType/PartyID: partner atau customer pemilik sub-channel (opsional).
	PartyType string
	PartyID   *uuid.UUID
	// ValidFrom/ValidTo/Budget/Description: hanya kampanye.
	ValidFrom   clock.BusinessDate
	ValidTo     clock.BusinessDate
	Budget      *money.Amount
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// DimensionInput adalah isian master dimensi.
type DimensionInput struct {
	Code        string
	Name        string
	ChannelID   *uuid.UUID
	PartyType   string
	PartyID     *uuid.UUID
	ValidFrom   clock.BusinessDate
	ValidTo     clock.BusinessDate
	Budget      *money.Amount
	Description string
	IsActive    bool
}

func (in *DimensionInput) normalize(kind DimensionKind) {
	in.Code = strings.TrimSpace(in.Code)
	switch kind {
	case KindChannel, KindSubChannel:
		in.Code = strings.ToLower(in.Code)
	case KindDivision, KindCampaign:
		in.Code = strings.ToUpper(in.Code)
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PartyType = strings.TrimSpace(in.PartyType)
	in.Description = strings.TrimSpace(in.Description)
}

func (in DimensionInput) validate(kind DimensionKind, withCode bool) map[string]string {
	fields := map[string]string{}
	if withCode {
		switch kind {
		case KindChannel:
			if !channelCodePattern.MatchString(in.Code) {
				fields["code"] = "Kode huruf kecil, angka, atau garis bawah (2–50 karakter), mis. modern_trade"
			}
		case KindSubChannel:
			if !subChannelCodePattern.MatchString(in.Code) {
				fields["code"] = "Kode huruf kecil, angka, garis bawah, atau tanda hubung (2–50 karakter), mis. shopee"
			}
		case KindDivision:
			if !divisionCodePattern.MatchString(in.Code) {
				fields["code"] = "Kode huruf kapital 2–10 karakter, mis. MKT"
			}
		case KindCampaign:
			if !campaignCodePattern.MatchString(in.Code) {
				fields["code"] = "Format KMP-YYYYMM-SINGKATAN, mis. KMP-202701-RAMADAN"
			}
		}
	}
	if in.Name == "" {
		fields["name"] = "Nama wajib diisi"
	} else if len(in.Name) > 150 {
		fields["name"] = "Nama paling panjang 150 karakter"
	}
	switch kind {
	case KindSubChannel:
		if in.ChannelID == nil {
			fields["channelId"] = "Channel wajib dipilih"
		}
		if (in.PartyType == "") != (in.PartyID == nil) {
			fields["partyId"] = "Jenis dan ID pihak diisi bersamaan"
		}
	case KindCampaign:
		if in.ValidFrom.IsZero() {
			fields["validFrom"] = "Tanggal mulai wajib diisi"
		}
		if in.ValidTo.IsZero() {
			fields["validTo"] = "Tanggal selesai wajib diisi"
		} else if !in.ValidFrom.IsZero() && in.ValidTo.Before(in.ValidFrom) {
			fields["validTo"] = "Tanggal selesai tidak boleh sebelum tanggal mulai"
		}
		if in.Budget != nil && in.Budget.IsNegative() {
			fields["budgetAmount"] = "Anggaran tidak boleh negatif"
		}
		if len(in.Description) > 500 {
			fields["description"] = "Keterangan paling panjang 500 karakter"
		}
	case KindChannel, KindDivision:
	}
	return fields
}

// NewDimension membuat master dimensi baru.
func NewDimension(kind DimensionKind, in DimensionInput) (DimensionValue, error) {
	in.normalize(kind)
	if fields := in.validate(kind, true); len(fields) > 0 {
		return DimensionValue{}, apperr.Validation(fields)
	}
	v := DimensionValue{Kind: kind, Code: in.Code}
	v.apply(kind, in)
	return v, nil
}

// Update mengubah master dimensi. Kode tidak berubah (dipakai laporan); channel sub-channel tidak dipindah.
func (v *DimensionValue) Update(in DimensionInput) error {
	in.normalize(v.Kind)
	if v.Kind == KindSubChannel {
		in.ChannelID = v.ChannelID
	}
	if fields := in.validate(v.Kind, false); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	v.apply(v.Kind, in)
	return nil
}

func (v *DimensionValue) apply(kind DimensionKind, in DimensionInput) {
	v.Name, v.IsActive = in.Name, in.IsActive
	switch kind {
	case KindSubChannel:
		v.ChannelID, v.PartyType, v.PartyID = in.ChannelID, in.PartyType, in.PartyID
	case KindCampaign:
		v.ValidFrom, v.ValidTo, v.Budget, v.Description = in.ValidFrom, in.ValidTo, in.Budget, in.Description
	case KindChannel, KindDivision:
	}
}

// CampaignActiveOn memeriksa periode kampanye pada tanggal transaksi.
func (v DimensionValue) CampaignActiveOn(d clock.BusinessDate) bool {
	return v.IsActive && !d.Before(v.ValidFrom) && !d.After(v.ValidTo)
}

// CheckDelete menolak penghapusan master yang sudah dipakai jurnal atau masih punya sub-channel.
func (v DimensionValue) CheckDelete(usedByJournal bool, children int64) error {
	if usedByJournal {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			fmt.Sprintf("%s %s sudah dipakai jurnal dan tidak bisa dihapus. Nonaktifkan bila tidak dipakai lagi.", v.Kind.Label(), v.Code))
	}
	if children > 0 {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			fmt.Sprintf("%s %s masih punya sub-channel.", v.Kind.Label(), v.Code))
	}
	return nil
}
