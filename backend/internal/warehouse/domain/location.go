// Package domain berisi entity gudang fisik dan lokasi stok (internal dan eksternal) beserta validasinya
// (MD-09, WMS-01, PRD F1-114).
package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

// LocationType adalah jenis lokasi stok (Desain Data Model 6.1).
type LocationType string

const (
	LocationInbound       LocationType = "inbound"
	LocationFinishedGoods LocationType = "finished_goods"
	// LocationEcommerce adalah E-commerce Stock (Petty Stock) yang dikelola Fulfillment.
	LocationEcommerce LocationType = "ecommerce"
	// LocationRawMaterial adalah lokasi RM (packaging) di Gudang GM.
	LocationRawMaterial LocationType = "raw_material"
	// LocationGoodsMaterial adalah lokasi GM (barang jadi menunggu reprocess) di Gudang GM.
	LocationGoodsMaterial LocationType = "goods_material"
	LocationReturns       LocationType = "returns"
	// LocationEvent adalah stok yang dibawa ke event; tidak bisa dijual.
	LocationEvent LocationType = "event"

	// Lokasi eksternal per pihak.
	LocationTollManufacturer LocationType = "toll_manufacturer"
	LocationConsignment      LocationType = "consignment"
	LocationVendingBuffer    LocationType = "vending_buffer"
	LocationVendingMachine   LocationType = "vending_machine"
)

var locationLabels = map[LocationType]string{
	LocationInbound: "Inbound", LocationFinishedGoods: "FG Non-Ecommerce", LocationEcommerce: "E-commerce Stock",
	LocationRawMaterial: "RM", LocationGoodsMaterial: "GM", LocationReturns: "Retur", LocationEvent: "Event",
	LocationTollManufacturer: "Maklon", LocationConsignment: "Konsinyasi", LocationVendingBuffer: "Buffer Vending",
	LocationVendingMachine: "Mesin Vending",
}

// Valid memeriksa jenis lokasi.
func (t LocationType) Valid() bool {
	_, ok := locationLabels[t]
	return ok
}

// Label adalah nama jenis lokasi untuk tampilan.
func (t LocationType) Label() string { return locationLabels[t] }

// IsExternal memeriksa apakah lokasi berada di luar gudang Balans (milik maklon, mitra, atau mesin).
func (t LocationType) IsExternal() bool { return len(AllowedPartyTypes(t)) > 0 }

// Sellable memeriksa apakah stok di lokasi jenis ini bisa dialokasikan ke penjualan.
func (t LocationType) Sellable() bool {
	switch t {
	case LocationFinishedGoods, LocationEcommerce, LocationConsignment, LocationVendingMachine:
		return true
	}
	return false
}

// PartyType adalah jenis pihak pemilik lokasi eksternal.
type PartyType string

const (
	PartySupplier       PartyType = "supplier"
	PartyPartner        PartyType = "partner"
	PartyVendingMachine PartyType = "vending_machine"
)

// AllowedPartyTypes mengembalikan jenis pihak yang boleh memiliki lokasi eksternal jenis t (kosong = lokasi internal).
func AllowedPartyTypes(t LocationType) []PartyType {
	switch t {
	case LocationTollManufacturer:
		return []PartyType{PartySupplier}
	case LocationConsignment:
		return []PartyType{PartyPartner}
	case LocationVendingBuffer:
		return []PartyType{PartyPartner, PartyVendingMachine}
	case LocationVendingMachine:
		return []PartyType{PartyVendingMachine}
	}
	return nil
}

// Batas panjang field.
const (
	MaxWarehouseCodeLength = 20
	MaxLocationCodeLength  = 30
	MaxNameLength          = 120
	MaxAddressLength       = 500
)

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

// NormalizeCode merapikan kode gudang/lokasi: huruf besar tanpa spasi di tepi.
func NormalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func validateCode(code string, maxLen int, fields map[string]string) {
	switch {
	case code == "":
		fields["code"] = "Kode wajib diisi"
	case len(code) > maxLen:
		fields["code"] = fmt.Sprintf("Kode maksimal %d karakter", maxLen)
	case !codePattern.MatchString(code):
		fields["code"] = "Kode hanya boleh huruf, angka, dan tanda minus"
	}
}

func validateName(name string, fields map[string]string) {
	switch {
	case name == "":
		fields["name"] = "Nama wajib diisi"
	case len([]rune(name)) > MaxNameLength:
		fields["name"] = fmt.Sprintf("Nama maksimal %d karakter", MaxNameLength)
	}
}

// Location adalah lokasi stok. Kode unik permanen walau lokasi dihapus.
type Location struct {
	ID          uuid.UUID
	Code        string
	Name        string
	WarehouseID *uuid.UUID
	Type        LocationType
	PartyType   PartyType
	PartyID     *uuid.UUID
	IsSellable  bool
	IsStandard  bool
	IsActive    bool
	softdelete.Mark

	// WarehouseName diisi saat dibaca untuk tampilan.
	WarehouseName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// IsExternal memeriksa apakah lokasi milik pihak luar.
func (l Location) IsExternal() bool { return l.Type.IsExternal() }

// LocationInput adalah isian lokasi baru.
type LocationInput struct {
	Code        string
	Name        string
	Type        LocationType
	WarehouseID *uuid.UUID
	PartyType   PartyType
	PartyID     *uuid.UUID
	IsActive    bool
}

// NewLocation memvalidasi lokasi baru. Lokasi internal wajib berada di gudang; lokasi eksternal wajib menunjuk
// pihak pemiliknya dan tidak berada di gudang. Keberadaan gudang diperiksa service.
func NewLocation(in LocationInput) (Location, error) {
	l := Location{
		Code: NormalizeCode(in.Code), Name: strings.TrimSpace(in.Name), Type: in.Type, IsActive: in.IsActive,
		IsSellable: in.Type.Sellable(),
	}
	fields := map[string]string{}
	validateCode(l.Code, MaxLocationCodeLength, fields)
	validateName(l.Name, fields)
	switch {
	case !in.Type.Valid():
		fields["locationType"] = "Jenis lokasi tidak dikenal"
	case in.Type.IsExternal():
		if in.WarehouseID != nil {
			fields["warehouseId"] = "Lokasi eksternal tidak berada di gudang"
		}
		if !slices.Contains(AllowedPartyTypes(in.Type), in.PartyType) {
			fields["externalPartyType"] = "Jenis pihak tidak sesuai jenis lokasi"
		}
		if in.PartyID == nil || *in.PartyID == uuid.Nil {
			fields["externalPartyId"] = "Pihak pemilik lokasi wajib diisi"
		}
		l.PartyType, l.PartyID = in.PartyType, in.PartyID
	default:
		if in.WarehouseID == nil || *in.WarehouseID == uuid.Nil {
			fields["warehouseId"] = "Gudang wajib dipilih"
		}
		if in.PartyType != "" || in.PartyID != nil {
			fields["externalPartyId"] = "Lokasi di dalam gudang tidak punya pihak eksternal"
		}
		l.WarehouseID = in.WarehouseID
	}
	if len(fields) > 0 {
		return Location{}, apperr.Validation(fields)
	}
	return l, nil
}

// Update mengubah nama dan status aktif. Kode, jenis, gudang, dan pihak tidak bisa diubah karena dipakai mutasi stok.
func (l *Location) Update(name string, isActive bool) error {
	if err := l.EnsureActive(); err != nil {
		return err
	}
	fields := map[string]string{}
	name = strings.TrimSpace(name)
	validateName(name, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	l.Name, l.IsActive = name, isActive
	return nil
}

// CanDelete menolak penghapusan lokasi standar satu per satu; lokasi standar ikut terhapus bersama gudangnya.
func (l Location) CanDelete() error {
	if l.IsStandard {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Lokasi standar tidak bisa dihapus sendiri. Nonaktifkan lokasi atau hapus gudangnya.")
	}
	return nil
}
