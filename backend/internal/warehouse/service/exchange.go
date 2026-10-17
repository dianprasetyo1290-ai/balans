package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Permission impor/ekspor lokasi (matriks Desain Role & Approval 3.1).
const (
	PermLocationImport = "warehouse.location.import"
	PermLocationExport = "warehouse.location.export"
)

// Parties mencari pihak pemilik lokasi eksternal (pemasok/maklon, mitra) dari modul party.
type Parties interface {
	PartyIDByCode(ctx context.Context, t domain.PartyType, code string) (uuid.UUID, bool, error)
	PartyCode(ctx context.Context, t domain.PartyType, id uuid.UUID) (string, error)
}

func locationTypeOptions() []tabular.Option {
	types := []domain.LocationType{
		domain.LocationInbound, domain.LocationFinishedGoods, domain.LocationEcommerce, domain.LocationPettyStock,
		domain.LocationRawMaterial, domain.LocationGoodsMaterial, domain.LocationReturns, domain.LocationEvent,
		domain.LocationTollManufacturer, domain.LocationConsignment, domain.LocationVendingBuffer, domain.LocationVendingMachine,
	}
	out := make([]tabular.Option, len(types))
	for i, t := range types {
		out[i] = tabular.Option{Value: string(t), Label: t.Label()}
	}
	return out
}

// LocationColumns adalah kolom template lokasi stok (MD-09), mengikuti template migrasi sheet Gudang & Lokasi.
// Gudang dibuat lewat layar Gudang (lokasi standarnya otomatis); impor menambah lokasi tambahan dan lokasi
// eksternal, atau memperbarui nama/status lokasi yang ada.
var LocationColumns = []tabular.Column{
	{Key: "warehouseCode", Header: "Kode Gudang", Kind: tabular.KindText, Example: "GDG-01",
		Note: "Wajib untuk lokasi internal; gudang harus sudah dibuat. Kosong untuk lokasi eksternal."},
	{Key: "code", Header: "Kode Lokasi", Kind: tabular.KindText, Required: true, Identity: true, Example: "GDG-01-EVT",
		Note: "Lokasi dengan kode yang sudah ada diperbarui nama dan status aktifnya."},
	{Key: "name", Header: "Nama Lokasi", Kind: tabular.KindText, Required: true, Example: "Stok Event"},
	{Key: "locationType", Header: "Tipe Lokasi", Kind: tabular.KindChoice, Required: true, Options: locationTypeOptions(), Example: "Event"},
	{Key: "externalPartyCode", Header: "Kode Pihak Eksternal", Kind: tabular.KindText,
		Note: "Untuk Maklon: kode pemasok; Konsinyasi/Buffer Vending: kode mitra."},
	{Key: "isActive", Header: "Aktif", Kind: tabular.KindBool, Example: tabular.Yes, Note: "Kosong = Ya."},
}

func locationField(k string) string {
	switch k {
	case "warehouseId":
		return "warehouseCode"
	case "externalPartyId", "externalPartyType":
		return "externalPartyCode"
	}
	return k
}

func remapLocation(err error) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		fields[locationField(k)] = v
	}
	return apperr.Validation(fields)
}

// ImportLocationRow membuat lokasi baru atau memperbarui nama/status lokasi dengan kode yang sama (S-125).
// Kode, jenis, gudang, dan pihak lokasi yang sudah ada tidak bisa diubah karena dipakai mutasi stok.
func (s *Service) ImportLocationRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermLocationImport); err != nil {
		return err
	}
	p := tabular.Parse(row)
	code := domain.NormalizeCode(p.Text("code"))
	typ := domain.LocationType(p.Choice("locationType", locationTypeOptions()))
	in := domain.LocationInput{Code: code, Name: p.Text("name"), Type: typ, IsActive: p.Bool("isActive", true)}
	if wc := domain.NormalizeCode(p.Text("warehouseCode")); wc != "" {
		id, found, err := s.Store.WarehouseIDByCode(ctx, wc)
		if err != nil {
			return err
		}
		if found {
			in.WarehouseID = &id
		} else {
			p.Fail("warehouseCode", "Kode gudang tidak ditemukan")
		}
	}
	if pc := strings.TrimSpace(p.Text("externalPartyCode")); pc != "" && typ.IsExternal() {
		in.PartyType = domain.AllowedPartyTypes(typ)[0]
		switch {
		case s.Parties == nil || in.PartyType == domain.PartyVendingMachine:
			p.Fail("externalPartyCode", "Pihak eksternal jenis ini belum bisa diimpor; buat lokasi lewat layar Lokasi")
		default:
			id, found, err := s.Parties.PartyIDByCode(ctx, in.PartyType, strings.ToUpper(pc))
			if err != nil {
				return err
			}
			if found {
				in.PartyID = &id
			} else {
				p.Fail("externalPartyCode", "Kode pihak eksternal tidak ditemukan")
			}
		}
	}
	if err := p.Err(); err != nil {
		return err
	}
	id, found, err := s.Store.LocationIDByCode(ctx, code)
	if err != nil {
		return err
	}
	if !found {
		_, err = s.CreateLocation(ctx, in)
		return remapLocation(err)
	}
	cur, err := s.Store.GetLocation(ctx, id, true)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	if cur.Type != typ {
		fields["locationType"] = "Tipe lokasi yang sudah ada tidak bisa diubah"
	}
	if in.WarehouseID != nil && (cur.WarehouseID == nil || *cur.WarehouseID != *in.WarehouseID) {
		fields["warehouseCode"] = "Gudang lokasi yang sudah ada tidak bisa diubah"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	_, err = s.UpdateLocation(ctx, cur.ID, cur.Version, in.Name, in.IsActive)
	return remapLocation(err)
}

// ExportLocations mengalirkan lokasi yang belum dihapus urut kode.
func (s *Service) ExportLocations(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermLocationExport); err != nil {
		return err
	}
	warehouses := map[uuid.UUID]string{}
	page := paging.Page{Number: 1, Size: paging.MaxSize}
	for {
		res, err := s.Store.ListLocations(ctx, postgres.LocationFilter{Sort: paging.Sort{Field: "code"}}, page)
		if err != nil {
			return err
		}
		for _, l := range res.Items {
			r := tabular.NewRow(0)
			if l.WarehouseID != nil {
				wc, ok := warehouses[*l.WarehouseID]
				if !ok {
					w, err := s.Store.GetWarehouse(ctx, *l.WarehouseID, false)
					if err != nil {
						return err
					}
					wc, warehouses[*l.WarehouseID] = w.Code, w.Code
				}
				r.Set("warehouseCode", wc)
			}
			r.Set("code", l.Code).Set("name", l.Name).Set("locationType", l.Type.Label())
			if l.PartyID != nil && s.Parties != nil {
				pc, err := s.Parties.PartyCode(ctx, l.PartyType, *l.PartyID)
				if err != nil {
					return err
				}
				r.Set("externalPartyCode", pc)
			}
			r.Set("isActive", tabular.BoolLabel(l.IsActive))
			if err := emit(r); err != nil {
				return err
			}
		}
		if int64(page.Number*page.Size) >= res.Total || len(res.Items) == 0 {
			return nil
		}
		page.Number++
	}
}
