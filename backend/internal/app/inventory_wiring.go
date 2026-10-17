package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/warehouse"
)

// devStockNamespace membuat ID dokumen seed yang tetap sehingga seed aman diulang (ErrAlreadyPosted).
var devStockNamespace = uuid.MustParse("6f1f2a8e-6d2b-4d55-9c7e-2a1c0b6a9e11")

// seedDevStock mengisi saldo awal stok contoh di Gudang Utama agar halaman Persediaan punya data (APP_ENV=local).
func (a *App) seedDevStock(ctx context.Context) error {
	ctx = actor.System(ctx)
	product := func(sku string) (uuid.UUID, error) {
		p, err := a.catalog.ProductByCode(ctx, sku)
		if err != nil {
			return uuid.Nil, fmt.Errorf("seed stok: produk %s: %w", sku, err)
		}
		return p.ID, nil
	}
	locs, err := a.warehouse.FindLocations(ctx, warehouse.LocationQuery{})
	if err != nil {
		return err
	}
	location := map[string]uuid.UUID{}
	for _, l := range locs {
		location[l.Code] = l.ID
	}
	for _, code := range []string{"UTAMA-FG", "UTAMA-ECM", "UTAMA-RM", "UTAMA-INB"} {
		if _, ok := location[code]; !ok {
			return fmt.Errorf("seed stok: lokasi %s tidak ditemukan", code)
		}
	}
	coklat, err := product("MS-COK-30")
	if err != nil {
		return err
	}
	vanila, err := product("MS-VAN-30")
	if err != nil {
		return err
	}
	tube, err := product("PKG-TUBE-30")
	if err != nil {
		return err
	}
	today := clock.Today(a.Clock)
	batch := func(number string, producedDaysAgo int) *inventory.NewBatch {
		d := today.AddDays(-producedDaysAgo)
		return &inventory.NewBatch{Number: number, ProductionDate: &d}
	}
	cost := func(s string) *decimal.Decimal {
		d := decimal.RequireFromString(s)
		return &d
	}
	at := func(code string, st inventory.StockStatus) *inventory.Place {
		return &inventory.Place{LocationID: location[code], Status: st}
	}
	receive := func(p uuid.UUID, b *inventory.NewBatch, qty int64, unitCost, code string, st inventory.StockStatus) inventory.Line {
		return inventory.Line{Kind: inventory.KindReceive, ProductID: p, Batch: b, Qty: decimal.NewFromInt(qty), UnitCost: cost(unitCost), To: at(code, st)}
	}
	opening := inventory.Posting{
		Source: inventory.Source{
			Type: "opening_balance", ID: uuid.NewSHA1(devStockNamespace, []byte("opening")), Label: "Saldo Awal Stok (demo)",
			BusinessDate: today.AddDays(-30),
		},
		Lines: []inventory.Line{
			receive(coklat, batch("MSC2510", 380), 15, "12000", "UTAMA-FG", inventory.StatusAvailable),
			receive(coklat, batch("MSC2601", 200), 120, "12500", "UTAMA-FG", inventory.StatusAvailable),
			receive(coklat, batch("MSC2604", 60), 200, "13000", "UTAMA-FG", inventory.StatusAvailable),
			receive(vanila, batch("MSV2603", 100), 150, "12800", "UTAMA-FG", inventory.StatusAvailable),
			receive(vanila, batch("MSV2603", 100), 40, "12800", "UTAMA-ECM", inventory.StatusAvailable),
			receive(vanila, batch("MSV2609", 10), 60, "12900", "UTAMA-INB", inventory.StatusQuarantine),
			receive(tube, nil, 1000, "850", "UTAMA-RM", inventory.StatusAvailable),
		},
	}
	sample := inventory.Posting{
		Source: inventory.Source{
			Type: "dev_sample_issue", ID: uuid.NewSHA1(devStockNamespace, []byte("issue")), Label: "Contoh pengeluaran FEFO (demo)",
			BusinessDate: today.AddDays(-5),
		},
		Lines: []inventory.Line{
			{Kind: inventory.KindIssue, ProductID: coklat, Qty: decimal.NewFromInt(10), From: at("UTAMA-FG", inventory.StatusAvailable)},
			{Kind: inventory.KindMove, ProductID: vanila, Qty: decimal.NewFromInt(20), From: at("UTAMA-FG", inventory.StatusAvailable),
				To: at("UTAMA-ECM", inventory.StatusAvailable), Note: "Isi ulang E-commerce Stock"},
		},
	}
	for _, p := range []inventory.Posting{opening, sample} {
		if _, err := a.inventory.Post(ctx, p); err != nil && !errors.Is(err, inventory.ErrAlreadyPosted) {
			return fmt.Errorf("seed stok %s: %w", p.Source.Type, err)
		}
	}
	return nil
}
