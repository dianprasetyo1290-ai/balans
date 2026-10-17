package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/warehouse/domain"
)

func ptr[T any](v T) *T { return &v }

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func TestWarehouse(t *testing.T) {
	w, err := domain.NewWarehouse(domain.WarehouseInput{Code: " utama ", Name: " Gudang Utama ", Address: " Jl. Raya 1 ", IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, domain.Warehouse{Code: "UTAMA", Name: "Gudang Utama", Address: "Jl. Raya 1", IsActive: true}, w)

	for _, tt := range []struct {
		in   domain.WarehouseInput
		want map[string]string
	}{
		{domain.WarehouseInput{Name: "X"}, map[string]string{"code": "Kode wajib diisi"}},
		{domain.WarehouseInput{Code: strings.Repeat("A", 21), Name: "X"}, map[string]string{"code": "Kode maksimal 20 karakter"}},
		{domain.WarehouseInput{Code: "G 1"}, map[string]string{"code": "Kode hanya boleh huruf, angka, dan tanda minus", "name": "Nama wajib diisi"}},
		{domain.WarehouseInput{Code: "G1", Name: strings.Repeat("x", 121), Address: strings.Repeat("x", 501)}, map[string]string{
			"name": "Nama maksimal 120 karakter", "address": "Alamat maksimal 500 karakter",
		}},
	} {
		_, err := domain.NewWarehouse(tt.in)
		assert.Equal(t, tt.want, fields(t, err))
	}

	require.NoError(t, w.Update(domain.WarehouseInput{Code: "utama", Name: "Gudang Pusat", IsActive: false}))
	assert.Equal(t, "Gudang Pusat", w.Name)
	assert.False(t, w.IsActive)
	assert.Equal(t, map[string]string{"code": "Kode tidak bisa diubah", "name": "Nama wajib diisi"},
		fields(t, w.Update(domain.WarehouseInput{Code: "LAIN"})))
	assert.Equal(t, "Gudang Pusat", w.Name)
	require.NoError(t, w.Delete(time.Now(), uuid.New()))
	assert.True(t, apperr.HasCode(w.Update(domain.WarehouseInput{Name: "X"}), apperr.CodeInvalidStateTransition))
}

func TestStandardLocations(t *testing.T) {
	w := domain.Warehouse{ID: uuid.New(), Code: "UTAMA"}
	locs := domain.StandardLocations(w)
	var codes []string
	var types []domain.LocationType
	for _, l := range locs {
		codes = append(codes, l.Code)
		types = append(types, l.Type)
		assert.Equal(t, w.ID, *l.WarehouseID)
		assert.True(t, l.IsStandard)
		assert.True(t, l.IsActive)
		assert.False(t, l.IsExternal())
	}
	assert.Equal(t, []string{"UTAMA-INB", "UTAMA-FG", "UTAMA-ECM", "UTAMA-PTY", "UTAMA-RM", "UTAMA-GM", "UTAMA-RET"}, codes)
	assert.Equal(t, []domain.LocationType{
		domain.LocationInbound, domain.LocationFinishedGoods, domain.LocationEcommerce, domain.LocationPettyStock, domain.LocationRawMaterial,
		domain.LocationGoodsMaterial, domain.LocationReturns,
	}, types)
	assert.True(t, locs[1].IsSellable, "FG Non-Ecommerce bisa dijual")
	assert.True(t, locs[2].IsSellable, "E-commerce Stock bisa dijual")
	assert.False(t, locs[0].IsSellable, "Inbound (karantina) tidak bisa dijual")
	assert.Equal(t, "E-commerce Stock", locs[2].Name)
	assert.Equal(t, "Petty Stock", locs[3].Name)
	assert.True(t, apperr.HasCode(locs[0].CanDelete(), apperr.CodeInvalidStateTransition))
}

func TestLocationTypes(t *testing.T) {
	assert.False(t, domain.LocationType("gudang").Valid())
	assert.Equal(t, "Konsinyasi", domain.LocationConsignment.Label())
	assert.True(t, domain.LocationConsignment.IsExternal())
	assert.False(t, domain.LocationEvent.IsExternal())
	assert.False(t, domain.LocationEvent.Sellable(), "stok event tidak bisa dijual")
	assert.True(t, domain.LocationVendingMachine.Sellable())
	assert.False(t, domain.LocationVendingBuffer.Sellable())
	assert.False(t, domain.LocationTollManufacturer.Sellable())
	assert.Equal(t, []domain.PartyType{domain.PartySupplier}, domain.AllowedPartyTypes(domain.LocationTollManufacturer))
	assert.Equal(t, []domain.PartyType{domain.PartyPartner, domain.PartyVendingMachine}, domain.AllowedPartyTypes(domain.LocationVendingBuffer))
	assert.Equal(t, []domain.PartyType{domain.PartyVendingMachine}, domain.AllowedPartyTypes(domain.LocationVendingMachine))
	assert.Nil(t, domain.AllowedPartyTypes(domain.LocationReturns))
}

func TestNewLocation(t *testing.T) {
	wh, party := uuid.New(), uuid.New()
	l, err := domain.NewLocation(domain.LocationInput{Code: "utama-event", Name: " Event Jakarta ", Type: domain.LocationEvent, WarehouseID: &wh, IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "UTAMA-EVENT", l.Code)
	assert.Equal(t, "Event Jakarta", l.Name)
	assert.False(t, l.IsSellable)
	assert.False(t, l.IsStandard)
	require.NoError(t, l.CanDelete())

	l, err = domain.NewLocation(domain.LocationInput{
		Code: "KSG-SINAR", Name: "Konsinyasi – Toko Sinar Jaya", Type: domain.LocationConsignment,
		PartyType: domain.PartyPartner, PartyID: &party, IsActive: true,
	})
	require.NoError(t, err)
	assert.True(t, l.IsExternal())
	assert.True(t, l.IsSellable)
	assert.Nil(t, l.WarehouseID)
	assert.Equal(t, party, *l.PartyID)

	tests := []struct {
		name string
		in   domain.LocationInput
		want map[string]string
	}{
		{"jenis tidak dikenal", domain.LocationInput{Code: "X", Name: "X", Type: "rak"}, map[string]string{"locationType": "Jenis lokasi tidak dikenal"}},
		{"kode & nama kosong", domain.LocationInput{Type: domain.LocationEvent, WarehouseID: &wh}, map[string]string{"code": "Kode wajib diisi", "name": "Nama wajib diisi"}},
		{"kode panjang", domain.LocationInput{Code: strings.Repeat("A", 31), Name: "X", Type: domain.LocationEvent, WarehouseID: &wh}, map[string]string{"code": "Kode maksimal 30 karakter"}},
		{"internal tanpa gudang", domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationEvent}, map[string]string{"warehouseId": "Gudang wajib dipilih"}},
		{"internal dengan pihak", domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationEvent, WarehouseID: &wh, PartyType: domain.PartyPartner, PartyID: &party}, map[string]string{"externalPartyId": "Lokasi di dalam gudang tidak punya pihak eksternal"}},
		{"eksternal di gudang", domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationTollManufacturer, WarehouseID: &wh, PartyType: domain.PartySupplier, PartyID: &party}, map[string]string{"warehouseId": "Lokasi eksternal tidak berada di gudang"}},
		{"pihak salah jenis", domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationTollManufacturer, PartyType: domain.PartyPartner, PartyID: &party}, map[string]string{"externalPartyType": "Jenis pihak tidak sesuai jenis lokasi"}},
		{"pihak kosong", domain.LocationInput{Code: "X", Name: "X", Type: domain.LocationVendingMachine, PartyType: domain.PartyVendingMachine, PartyID: ptr(uuid.Nil)}, map[string]string{"externalPartyId": "Pihak pemilik lokasi wajib diisi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewLocation(tt.in)
			assert.Equal(t, tt.want, fields(t, err))
		})
	}
}

func TestLocationUpdate(t *testing.T) {
	l := domain.Location{Name: "Lama", IsActive: true}
	require.NoError(t, l.Update(" Baru ", false))
	assert.Equal(t, "Baru", l.Name)
	assert.False(t, l.IsActive)
	assert.Equal(t, map[string]string{"name": "Nama wajib diisi"}, fields(t, l.Update("", true)))
	assert.Equal(t, "Baru", l.Name)
	require.NoError(t, l.Delete(time.Now(), uuid.New()))
	assert.True(t, apperr.HasCode(l.Update("Lagi", true), apperr.CodeInvalidStateTransition))
}
