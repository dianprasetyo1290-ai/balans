package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

// Warehouse adalah gudang fisik. Kode unik permanen walau gudang dihapus.
type Warehouse struct {
	ID       uuid.UUID
	Code     string
	Name     string
	Address  string
	IsActive bool
	softdelete.Mark
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// WarehouseInput adalah isian gudang.
type WarehouseInput struct {
	Code     string
	Name     string
	Address  string
	IsActive bool
}

// NewWarehouse memvalidasi gudang baru.
func NewWarehouse(in WarehouseInput) (Warehouse, error) {
	w := Warehouse{Code: NormalizeCode(in.Code)}
	fields := map[string]string{}
	validateCode(w.Code, MaxWarehouseCodeLength, fields)
	w.set(in, fields)
	if len(fields) > 0 {
		return Warehouse{}, apperr.Validation(fields)
	}
	return w, nil
}

// Update mengubah nama, alamat, dan status aktif; kode tidak bisa diubah.
func (w *Warehouse) Update(in WarehouseInput) error {
	if err := w.EnsureActive(); err != nil {
		return err
	}
	fields := map[string]string{}
	if code := NormalizeCode(in.Code); code != "" && code != w.Code {
		fields["code"] = "Kode tidak bisa diubah"
	}
	next := *w
	next.set(in, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	*w = next
	return nil
}

func (w *Warehouse) set(in WarehouseInput, fields map[string]string) {
	w.Name, w.Address, w.IsActive = strings.TrimSpace(in.Name), strings.TrimSpace(in.Address), in.IsActive
	validateName(w.Name, fields)
	if len([]rune(w.Address)) > MaxAddressLength {
		fields["address"] = fmt.Sprintf("Alamat maksimal %d karakter", MaxAddressLength)
	}
}

// standardLocations adalah lokasi yang dibuat otomatis untuk setiap gudang baru (PRD F1-114, BRD U-08).
// E-commerce Stock dan Petty Stock adalah lokasi terpisah (keputusan PO, PRD F1-114); RM dan GM adalah lokasi di Gudang GM.
var standardLocations = []struct {
	suffix string
	name   string
	typ    LocationType
}{
	{"INB", "Inbound", LocationInbound},
	{"FG", "FG Non-Ecommerce", LocationFinishedGoods},
	{"ECM", "E-commerce Stock", LocationEcommerce},
	{"PTY", "Petty Stock", LocationPettyStock},
	{"RM", "Gudang GM – RM", LocationRawMaterial},
	{"GM", "Gudang GM – GM", LocationGoodsMaterial},
	{"RET", "Gudang Retur", LocationReturns},
}

// StandardLocations menyusun lokasi standar gudang. Kode lokasi = kode gudang + akhiran, mis. `UTAMA-INB`.
func StandardLocations(w Warehouse) []Location {
	out := make([]Location, len(standardLocations))
	for i, s := range standardLocations {
		id := w.ID
		out[i] = Location{
			Code: w.Code + "-" + s.suffix, Name: s.name, WarehouseID: &id, Type: s.typ, IsSellable: s.typ.Sellable(),
			IsStandard: true, IsActive: true,
		}
	}
	return out
}
