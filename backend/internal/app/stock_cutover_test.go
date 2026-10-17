package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"balans/internal/accounting"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// writeWorkbook membuat file xlsx dengan beberapa sheet (baris pertama = header).
func writeWorkbook(t *testing.T, path string, sheets map[string][][]string) {
	t.Helper()
	f := excelize.NewFile()
	first := true
	for name, rows := range sheets {
		if first {
			require.NoError(t, f.SetSheetName("Sheet1", name))
			first = false
		} else {
			_, err := f.NewSheet(name)
			require.NoError(t, err)
		}
		for r, cells := range rows {
			for c, v := range cells {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
				require.NoError(t, f.SetCellStr(name, cell, v))
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
}

func newCutoverApp(t *testing.T) *App {
	t.Helper()
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Env: config.EnvLocal, EmailProvider: config.ProviderMock, PaymentGateway: config.ProviderMock,
		ShipmentProvider: config.ProviderMock, PublicBaseURL: "http://localhost:5173", FilestoreDir: t.TempDir(),
		EnabledModules: []string{
			"identity", "audit", "notification", "approval", "document", "accounting", "catalog", "warehouse", "inventory",
			"filestore", "party", "hris", "dataexchange",
		},
	}
	a := &App{
		Config: cfg, Log: log, Pool: pool, Clock: clock.System{}, Tx: db.NewTxManager(pool),
		Responder: httpx.NewResponder(log, nil), idempotency: idempotency.NewStore(pool),
	}
	var err error
	a.Jobs, err = jobqueue.NewInserter(pool)
	require.NoError(t, err)
	require.NoError(t, a.buildModules())
	ctx := context.Background()
	_, err = a.identity.SeedDevUsers(ctx, "Rahasia-Dev-12345")
	require.NoError(t, err)
	// Master gudang & produk tanpa stok demo agar nilai persediaan hanya dari saldo stok cut-over.
	require.NoError(t, a.warehouse.SeedDev(ctx))
	require.NoError(t, a.catalog.SeedDev(ctx))
	for _, q := range []string{
		`INSERT INTO suppliers (code, name, supplier_type, is_business_entity, created_by, updated_by)
		 VALUES ('SUP-CUT', 'Pemasok Cut-over', 'supplier', true, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO customers (code, full_name, phone, phone_normalized, customer_type, created_by, updated_by)
		 VALUES ('PLG-CUT', 'Pelanggan Cut-over', '081234567890', '+6281234567890', 'b2b', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000')
		 ON CONFLICT DO NOTHING`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	// Test lain di package ini mengekspor seluruh pemasok/pelanggan; hapus data cut-over setelah selesai.
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM suppliers WHERE code = 'SUP-CUT'`, `DELETE FROM customers WHERE code = 'PLG-CUT'`} {
			_, err := pool.Exec(context.Background(), q)
			assert.NoError(t, err)
		}
	})
	return a
}

func TestCutoverDryRunThenCommitIdempotent(t *testing.T) {
	a := newCutoverApp(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "Template-Migrasi-Saldo-Awal.xlsx")
	writeWorkbook(t, path, map[string][][]string{
		"Neraca Saldo Awal": {
			{"Kode Akun *", "Nama Akun", "Debit", "Kredit"},
			{"1111", "Bank", "1000000", ""},
			{"1201", "Piutang Usaha", "1000000", ""},
			{"1311", "Persediaan FG", "10000", ""},
			{"1502", "Peralatan", "4800000", ""},
			{"1592", "Akumulasi", "", "1200000"},
			{"2101", "Utang Usaha", "", "500000"},
			{"3101", "Modal", "", "5110000"},
		},
		"Piutang Terbuka": {
			{"Nomor Invoice Lama *", "Tanggal Invoice *", "Jatuh Tempo *", "Kode Pelanggan/Mitra *", "Channel *", "Nilai Invoice *", "Sisa Belum Dibayar *"},
			{"INV-LAMA-1", "2026-01-15", "2026-02-14", "PLG-CUT", "B2B", "1500000", "1000000"},
		},
		"Utang Terbuka": {
			{"Nomor Tagihan Pemasok *", "Kode Pemasok *", "Tanggal Tagihan *", "Jatuh Tempo *", "Nilai Tagihan *", "Sisa Belum Dibayar *"},
			{"BILL-LAMA-1", "SUP-CUT", "2026-01-20", "2026-02-19", "500000", "500000"},
		},
		"Saldo Stok": {
			{"Kode Lokasi *", "SKU/Kode *", "Nomor Batch *", "Tanggal Produksi", "Tanggal Kedaluwarsa *", "Qty *", "HPP per Unit *"},
			{"UTAMA-FG", "MS-COK-30", "CUT2612", "2025-06-01", "2027-12-12", "10", "1000"},
		},
		"Aset Tetap": {
			{"Kode Aset *", "Nama Aset *", "Kelas Aset *", "Tanggal Perolehan *", "Harga Perolehan *", "Umur Manfaat (bulan) *", "Nilai Sisa", "Akumulasi Penyusutan per Cut-over *", "Lokasi / Mitra"},
			{"AST-CUT-1", "Laptop lama", "Peralatan Kantor & Komputer", "2025-01-01", "4800000", "48", "0", "1200000", "Kantor"},
		},
	})
	// Cut-over Jan 2026 (posting rule berlaku mulai 2026) agar jurnal test lain (bertanggal Okt 2026) tidak ikut GL per cut-over; stok yang sudah ada
	// per tanggal itu tetap dikurangkan dari nilai persediaan.
	cutover := clock.NewBusinessDate(2026, time.January, 31)
	base := stockValuer{inventory: a.inventory, catalog: a.catalog, warehouse: a.warehouse}
	baseline, err := base.InventoryValueByAccount(ctx, cutover)
	require.NoError(t, err)
	a.accounting.SetStockValuer(baselineValuer{inner: base, baseline: baseline})

	var out bytes.Buffer
	opts := CutoverOptions{
		Files: []string{path}, UserEmail: "finance@balans.local", CutoverDate: clock.NewBusinessDate(2026, time.January, 31),
		Post: true, Out: &out,
	}

	require.NoError(t, a.Cutover(ctx, opts), out.String())
	assert.Contains(t, out.String(), "DRY-RUN")
	assert.Contains(t, out.String(), "inventory.opening_stock")
	_, _, ok, err := a.accounting.CutoverDate(ctx)
	require.NoError(t, err)
	assert.False(t, ok, "dry-run tidak menyimpan apa pun")
	_ = cutover

	out.Reset()
	opts.Commit = true
	require.NoError(t, a.Cutover(ctx, opts), out.String())
	assert.Contains(t, out.String(), "Saldo awal diposting")
	date, posted, ok, err := a.accounting.CutoverDate(ctx)
	require.NoError(t, err)
	assert.True(t, ok && posted)
	assert.Equal(t, "2026-01-31", date.String())

	items, err := a.accounting.OpeningItems(ctx, accounting.OpeningItemQuery{}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, items.Items, 2)
	values, err := a.inventory.StockValues(ctx, date)
	require.NoError(t, err)
	require.NotEmpty(t, values)

	out.Reset()
	require.NoError(t, a.Cutover(ctx, opts), out.String())
	assert.Contains(t, out.String(), "sudah diimpor", "impor ulang file yang sama dilewati")

	out.Reset()
	opts.UserEmail = "nobody@balans.local"
	assert.Error(t, a.Cutover(ctx, opts))
}

// baselineValuer mengurangi nilai persediaan yang sudah ada sebelum test.
type baselineValuer struct {
	inner    stockValuer
	baseline map[string]money.Amount
}

func (b baselineValuer) InventoryValueByAccount(ctx context.Context, asOf clock.BusinessDate) (map[string]money.Amount, error) {
	cur, err := b.inner.InventoryValueByAccount(ctx, asOf)
	if err != nil {
		return nil, err
	}
	out := map[string]money.Amount{}
	for k, v := range cur {
		if d := v.Sub(b.baseline[k]); !d.IsZero() {
			out[k] = d
		}
	}
	return out, nil
}
