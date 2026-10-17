package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"balans/internal/catalog"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// dxHarness menyusun modul master data + dataexchange dengan Postgres asli dan router berisi actor tetap
// (tanpa JWT), lalu menjalankan job impor/ekspor secara sinkron.
type dxHarness struct {
	t     *testing.T
	app   *App
	srv   *httptest.Server
	ctx   context.Context
	actor actor.Actor
}

// as mengganti user yang menjalankan request berikutnya (akun seed pengembangan per role).
func (h *dxHarness) as(email string) {
	h.t.Helper()
	users, err := h.app.identity.SearchActiveUsers(context.Background(), email, 1)
	require.NoError(h.t, err)
	require.Len(h.t, users, 1)
	h.actor, err = h.app.identity.ActorFor(context.Background(), users[0].ID)
	require.NoError(h.t, err)
	h.ctx = actor.With(context.Background(), h.actor)
}

func newDXHarness(t *testing.T) *dxHarness {
	t.Helper()
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Env: config.EnvLocal, EmailProvider: config.ProviderMock, PaymentGateway: config.ProviderMock,
		ShipmentProvider: config.ProviderMock, PublicBaseURL: "http://localhost:5173", FilestoreDir: t.TempDir(),
		EnabledModules: []string{
			"identity", "audit", "notification", "document", "accounting", "catalog", "warehouse", "filestore",
			"party", "hris", "dataexchange",
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
	h := &dxHarness{t: t, app: a}
	h.as("admin@balans.local")
	require.True(t, h.actor.Has("catalog.product.import"))

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), h.actor)))
		})
	})
	a.filestore.RegisterRoutes(r)
	a.dataexchange.RegisterRoutes(r)
	h.srv = httptest.NewServer(r)
	t.Cleanup(h.srv.Close)
	return h
}

func (h *dxHarness) do(method, path string, body any) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(h.t, err)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, h.srv.URL+path, rd)
	require.NoError(h.t, err)
	if _, ok := body.([]byte); !ok && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", uuid.NewString())
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer res.Body.Close() //nolint:errcheck
	out, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	return res.StatusCode, out
}

func (h *dxHarness) json(method, path string, body any, wantStatus int) map[string]any {
	h.t.Helper()
	status, raw := h.do(method, path, body)
	require.Equal(h.t, wantStatus, status, string(raw))
	var out map[string]any
	require.NoError(h.t, json.Unmarshal(raw, &out))
	return out
}

// upload mengunggah xlsx lewat alur filestore (URL bertanda tangan) dan mengembalikan fileId.
func (h *dxHarness) upload(kind string, data []byte) string {
	h.t.Helper()
	target := h.json(http.MethodPost, "/api/v1/files", map[string]any{
		"ownerType": "import." + kind, "fileName": "data.xlsx",
		"contentType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "sizeBytes": len(data),
	}, http.StatusCreated)
	status, raw := h.do(http.MethodPut, target["uploadUrl"].(string), data)
	require.Equal(h.t, http.StatusNoContent, status, string(raw))
	return target["fileId"].(string)
}

func (h *dxHarness) fetch(url string) []byte {
	h.t.Helper()
	status, raw := h.do(http.MethodGet, url, nil)
	require.Equal(h.t, http.StatusOK, status)
	return raw
}

func (h *dxHarness) run(id string) map[string]any {
	h.t.Helper()
	require.NoError(h.t, h.app.dataexchange.RunImport(context.Background(), uuid.MustParse(id)))
	return h.json(http.MethodGet, "/api/v1/import-jobs/"+id, nil, http.StatusOK)
}

// fillTemplate mengisi template dari sistem dengan baris data berdasarkan nama header.
func fillTemplate(t *testing.T, tpl []byte, sheet string, rows []map[string]string) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(tpl))
	require.NoError(t, err)
	headers, err := f.GetRows(sheet)
	require.NoError(t, err)
	col := map[string]int{}
	for i, hd := range headers[0] {
		col[strings.TrimSuffix(hd, " *")] = i + 1
	}
	for r, values := range rows {
		for header, v := range values {
			c, ok := col[header]
			require.True(t, ok, "header %s ada di template", header)
			cell, _ := excelize.CoordinatesToCellName(c, r+2)
			require.NoError(t, f.SetCellStr(sheet, cell, v))
		}
	}
	var out bytes.Buffer
	require.NoError(t, f.Write(&out))
	require.NoError(t, f.Close())
	return out.Bytes()
}

func TestImportProductsFromGeneratedTemplateEndToEnd(t *testing.T) {
	h := newDXHarness(t)

	kinds := h.json(http.MethodGet, "/api/v1/data-exchange/kinds", nil, http.StatusOK)
	labels := map[string]bool{}
	for _, k := range kinds["items"].([]any) {
		labels[k.(map[string]any)["kind"].(string)] = true
	}
	for _, k := range []string{"catalog.product", "party.customer", "party.supplier", "party.partner_product_code", "warehouse.location"} {
		assert.True(t, labels[k], "jenis %s terdaftar untuk Admin Sistem", k)
	}
	assert.False(t, labels["hris.employee"], "Admin Sistem tidak punya izin impor karyawan")

	status, tpl := h.do(http.MethodGet, "/api/v1/data-exchange/kinds/catalog.product/template", nil)
	require.Equal(t, http.StatusOK, status)

	file := fillTemplate(t, tpl, "Produk", []map[string]string{
		{"SKU": "DX-A", "Nama Produk": "Produk A", "Jenis Produk": "Produk Jadi", "Tipe Kemasan": "Pouch", "Satuan": "pcs", "Umur Simpan (hari)": "365", "Barcode": "4006381333931"},
		{"SKU": "dx-b", "Nama Produk": "Kemasan B", "Jenis Produk": "Packaging", "Tipe Kemasan": "Tube", "Satuan": "pcs"},
		{"SKU": "DX-A", "Nama Produk": "Ganda", "Jenis Produk": "Produk Jadi", "Tipe Kemasan": "Pouch", "Satuan": "pcs", "Umur Simpan (hari)": "30"},
		{"SKU": "DX-C", "Nama Produk": "Tanpa Umur", "Jenis Produk": "Produk Jadi", "Satuan": "pcs"},
		{"SKU": "DX-BND", "Nama Produk": "Paket", "Jenis Produk": "Produk Jadi", "Satuan": "paket", "Bundel Virtual": "Ya", "Komponen Bundel": "DX-A:2"},
	})

	job := h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": "catalog.product", "fileId": h.upload("catalog.product", file)}, http.StatusAccepted)
	assert.Equal(t, "uploaded", job["status"])
	id := job["id"].(string)

	// File yang sama tidak bisa diimpor dua kali, walau diunggah ulang sebagai file baru.
	dup := h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": "catalog.product", "fileId": h.upload("catalog.product", file)}, http.StatusConflict)
	assert.Equal(t, "IMPORT_ALREADY_PROCESSED", dup["code"])

	job = h.run(id)
	require.Equal(t, "validated", job["status"], job["message"])
	assert.EqualValues(t, 5, job["totalRows"])
	assert.EqualValues(t, 3, job["validRows"])
	assert.EqualValues(t, 2, job["errorRows"])
	assert.EqualValues(t, 100, job["progress"])
	assert.Contains(t, job["allowedActions"], "commit")

	rows := h.json(http.MethodGet, "/api/v1/import-jobs/"+id+"/rows?valid=false", nil, http.StatusOK)
	items := rows["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.EqualValues(t, 4, first["rowNumber"])
	assert.Contains(t, first["errors"].([]any)[0].(map[string]any)["message"], "ganda dengan baris 2")
	second := items[1].(map[string]any)
	assert.EqualValues(t, 5, second["rowNumber"])
	var cols []string
	for _, e := range second["errors"].([]any) {
		cols = append(cols, e.(map[string]any)["column"].(string))
	}
	assert.Equal(t, []string{"packagingType", "shelfLifeDays"}, cols)

	// Pemeriksaan tidak menyimpan apa pun.
	_, err := h.app.catalog.ProductByCode(context.Background(), "DX-A")
	require.Error(t, err)

	link := h.json(http.MethodGet, "/api/v1/import-jobs/"+id+"/error-file", nil, http.StatusOK)
	errFile, err := excelize.OpenReader(bytes.NewReader(h.fetch(link["url"].(string))))
	require.NoError(t, err)
	errRows, err := errFile.GetRows("Produk")
	require.NoError(t, err)
	require.NoError(t, errFile.Close())
	require.Len(t, errRows, 3)
	assert.Equal(t, "Keterangan Error", errRows[0][len(errRows[0])-1])
	assert.Contains(t, errRows[2][len(errRows[2])-1], "Baris 5: Tipe Kemasan")
	assert.Contains(t, errRows[2][len(errRows[2])-1], "Umur Simpan (hari): Umur simpan wajib diisi")

	job = h.json(http.MethodPost, "/api/v1/import-jobs/"+id+"/commit", nil, http.StatusAccepted)
	assert.Equal(t, "committing", job["status"])
	job = h.run(id)
	require.Equal(t, "completed", job["status"], job["message"])
	assert.EqualValues(t, 3, job["committedRows"])

	bundle, err := h.app.catalog.ProductByCode(context.Background(), "DX-BND")
	require.NoError(t, err)
	require.Len(t, bundle.Components, 1)
	assert.Equal(t, "DX-A", bundle.Components[0].SKU)
	pkg, err := h.app.catalog.ProductByCode(context.Background(), "DX-B")
	require.NoError(t, err)
	assert.Equal(t, "Kemasan B", pkg.Name)

	// Job yang sudah selesai tidak diproses ulang (idempotent).
	job = h.run(id)
	assert.Equal(t, "completed", job["status"])

	// Ekspor menghasilkan file dengan kolom template yang sama, lalu diimpor ulang sebagai pembaruan.
	exp := h.json(http.MethodPost, "/api/v1/export-jobs", map[string]any{"kind": "catalog.product"}, http.StatusAccepted)
	require.NoError(t, h.app.dataexchange.RunExport(context.Background(), uuid.MustParse(exp["id"].(string))))
	exp = h.json(http.MethodGet, "/api/v1/export-jobs/"+exp["id"].(string), nil, http.StatusOK)
	require.Equal(t, "completed", exp["status"], exp["message"])
	dl := h.json(http.MethodGet, "/api/v1/export-jobs/"+exp["id"].(string)+"/download-url", nil, http.StatusOK)
	exported := h.fetch(dl["url"].(string))
	ef, err := excelize.OpenReader(bytes.NewReader(exported))
	require.NoError(t, err)
	exRows, err := ef.GetRows("Produk")
	require.NoError(t, err)
	require.NoError(t, ef.Close())
	var skus []string
	for _, r := range exRows[1:] {
		skus = append(skus, r[0])
	}
	assert.Subset(t, skus, []string{"DX-A", "DX-B", "DX-BND"})
	assert.Equal(t, "DX-BND", skus[len(skus)-1], "bundel ditulis setelah komponennya")

	reimport := h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": "catalog.product", "fileId": h.upload("catalog.product", exported)}, http.StatusAccepted)
	again := h.run(reimport["id"].(string))
	require.Equal(t, "validated", again["status"], again["message"])
	assert.EqualValues(t, 0, again["errorRows"], "file ekspor valid diimpor ulang")

	cancelled := h.json(http.MethodPost, "/api/v1/import-jobs/"+reimport["id"].(string)+"/cancel", nil, http.StatusOK)
	assert.Equal(t, "cancelled", cancelled["status"])

	history := h.json(http.MethodGet, "/api/v1/import-jobs?kind=catalog.product", nil, http.StatusOK)
	assert.GreaterOrEqual(t, len(history["items"].([]any)), 2)
	exports := h.json(http.MethodGet, "/api/v1/export-jobs", nil, http.StatusOK)
	assert.NotEmpty(t, exports["items"])
}

func TestImportPartnerProductCodesAndCustomers(t *testing.T) {
	h := newDXHarness(t)
	ctx := h.ctx
	require.NoError(t, h.app.catalog.SeedDev(ctx))
	_, err := h.app.Pool.Exec(ctx, `INSERT INTO partners (code, name, partner_type, address, pic_name, pic_phone, created_by, updated_by)
		VALUES ('MT-DX', 'Mitra DX', 'modern_trade', 'Jl. A', 'PIC', '+628123456789', $1, $1) ON CONFLICT DO NOTHING`, actor.UserID(ctx))
	require.NoError(t, err)
	products, err := h.app.catalog.ProductOptions(ctx, catalog.OptionFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, products)
	sku := products[0].SKU

	_, tpl := h.do(http.MethodGet, "/api/v1/data-exchange/kinds/party.partner_product_code/template", nil)
	file := fillTemplate(t, tpl, "Kode Produk Mitra", []map[string]string{
		{"Kode Mitra": "mt-dx", "SKU": sku, "Kode Artikel": "ART-1", "Barcode Retailer": "2000000000001"},
		{"Kode Mitra": "TIDAK-ADA", "SKU": sku, "Kode Artikel": "ART-2"},
	})
	job := h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": "party.partner_product_code", "fileId": h.upload("party.partner_product_code", file)}, http.StatusAccepted)
	res := h.run(job["id"].(string))
	require.Equal(t, "validated", res["status"], res["message"])
	assert.EqualValues(t, 1, res["validRows"])
	h.json(http.MethodPost, "/api/v1/import-jobs/"+job["id"].(string)+"/commit", nil, http.StatusAccepted)
	res = h.run(job["id"].(string))
	require.Equal(t, "completed", res["status"], res["message"])

	// Pelanggan diperiksa tanpa menulis (kode dari sequence), lalu disimpan saat konfirmasi.
	var before int64
	require.NoError(t, h.app.Pool.QueryRow(ctx, `SELECT last_value FROM customer_code_seq`).Scan(&before))
	_, tpl = h.do(http.MethodGet, "/api/v1/data-exchange/kinds/party.customer/template", nil)
	file = fillTemplate(t, tpl, "Pelanggan", []map[string]string{
		{"Nama Lengkap": "Pelanggan DX", "Nomor WA/HP": "0812-9999-0001", "Tipe Pelanggan": "Retail", "Alamat Pengiriman": "Jl. B", "Kota/Kabupaten": "Bandung"},
		{"Nama Lengkap": "Nomor Sama", "Nomor WA/HP": "0812-9999-0001", "Tipe Pelanggan": "Retail"},
		{"Nama Lengkap": "Karyawan", "Nomor WA/HP": "0812-9999-0002", "Tipe Pelanggan": "Karyawan", "Nomor Karyawan": "TIDAK-ADA"},
	})
	job = h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": "party.customer", "fileId": h.upload("party.customer", file)}, http.StatusAccepted)
	res = h.run(job["id"].(string))
	require.Equal(t, "validated", res["status"], res["message"])
	assert.EqualValues(t, 1, res["validRows"])
	var after int64
	require.NoError(t, h.app.Pool.QueryRow(ctx, `SELECT last_value FROM customer_code_seq`).Scan(&after))
	assert.Equal(t, before, after, "pemeriksaan tidak memakai nomor pelanggan")
	h.json(http.MethodPost, "/api/v1/import-jobs/"+job["id"].(string)+"/commit", nil, http.StatusAccepted)
	res = h.run(job["id"].(string))
	require.Equal(t, "completed", res["status"], res["message"])
	c, found, err := h.app.party.CustomerByPhone(ctx, "081299990001")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Pelanggan DX", c.FullName)
	require.Len(t, c.Addresses, 1)
	assert.Equal(t, "Bandung", c.Addresses[0].City)
}

// importFile mengunduh template jenis data, mengisinya, lalu menjalankan pemeriksaan dan (bila ada baris valid)
// penyimpanan. Mengembalikan hasil pemeriksaan dan hasil akhir.
func (h *dxHarness) importFile(kind, sheet string, rows []map[string]string) (validated, final map[string]any) {
	h.t.Helper()
	status, tpl := h.do(http.MethodGet, "/api/v1/data-exchange/kinds/"+kind+"/template", nil)
	require.Equal(h.t, http.StatusOK, status, string(tpl))
	file := fillTemplate(h.t, tpl, sheet, rows)
	job := h.json(http.MethodPost, "/api/v1/import-jobs", map[string]any{"kind": kind, "fileId": h.upload(kind, file)}, http.StatusAccepted)
	id := job["id"].(string)
	validated = h.run(id)
	require.Equal(h.t, "validated", validated["status"], validated["message"])
	if validated["validRows"].(float64) == 0 {
		return validated, validated
	}
	h.json(http.MethodPost, "/api/v1/import-jobs/"+id+"/commit", nil, http.StatusAccepted)
	final = h.run(id)
	return validated, final
}

func (h *dxHarness) exportRows(kind, sheet string) [][]string {
	h.t.Helper()
	exp := h.json(http.MethodPost, "/api/v1/export-jobs", map[string]any{"kind": kind}, http.StatusAccepted)
	id := exp["id"].(string)
	require.NoError(h.t, h.app.dataexchange.RunExport(context.Background(), uuid.MustParse(id)))
	dl := h.json(http.MethodGet, "/api/v1/export-jobs/"+id+"/download-url", nil, http.StatusOK)
	f, err := excelize.OpenReader(bytes.NewReader(h.fetch(dl["url"].(string))))
	require.NoError(h.t, err)
	defer f.Close() //nolint:errcheck
	rows, err := f.GetRows(sheet)
	require.NoError(h.t, err)
	return rows
}

func TestImportMastersPerRole(t *testing.T) {
	h := newDXHarness(t)

	// Pemasok & maklon (Admin Sistem): NPWP dan rekening diekspor tersamar dan tidak berubah saat diimpor ulang.
	_, res := h.importFile("party.supplier", "Pemasok & Maklon", []map[string]string{
		{"Kode Pemasok": "sup-dx", "Nama": "PT DX", "Jenis": "Maklon", "Badan Usaha": "Ya", "NPWP": "01.234.567.8-901.234",
			"PPh Default": "PPh 23", "Termin Pembayaran (hari)": "30", "Bank": "BCA", "Nomor Rekening": "1234567890", "Nama Pemilik Rekening": "PT DX"},
	})
	require.Equal(t, "completed", res["status"], res["message"])
	rows := h.exportRows("party.supplier", "Pemasok & Maklon")
	require.Len(t, rows, 2)
	assert.Equal(t, "SUP-DX", rows[1][0])
	assert.Contains(t, strings.Join(rows[1], "|"), "••••••7890")

	// Lokasi stok: lokasi maklon menunjuk pemasok lewat kode.
	_, res = h.importFile("warehouse.location", "Gudang & Lokasi", []map[string]string{
		{"Kode Lokasi": "MKL-DX", "Nama Lokasi": "Maklon DX", "Tipe Lokasi": "Maklon", "Kode Pihak Eksternal": "SUP-DX"},
		{"Kode Lokasi": "X-1", "Nama Lokasi": "Tanpa Gudang", "Tipe Lokasi": "Event"},
	})
	assert.EqualValues(t, 1, res["committedRows"])
	rows = h.exportRows("warehouse.location", "Gudang & Lokasi")
	// Basis data uji dipakai bersama: cari baris lokasi maklon (lokasi seed UTAMA-* bisa ada sesudahnya).
	maklon := ""
	for _, r := range rows {
		if j := strings.Join(r, "|"); strings.Contains(j, "MKL-DX") {
			maklon = j
		}
	}
	assert.Contains(t, maklon, "SUP-DX")

	// Chart of Accounts (Finance): akun induk dari baris sebelumnya, kode lama diperbarui.
	h.as("finance@balans.local")
	_, res = h.importFile("accounting.account", "COA", []map[string]string{
		{"Kode Akun": "1190", "Nama Akun": "Induk DX", "Akun Posting": "Tidak"},
		{"Kode Akun": "1191", "Nama Akun": "Anak DX", "Kode Akun Induk": "1190"},
	})
	require.Equal(t, "completed", res["status"], res["message"])
	rows = h.exportRows("accounting.account", "COA")
	var child []string
	for _, r := range rows {
		if r[0] == "1191" {
			child = r
		}
	}
	require.NotNil(t, child)
	assert.Equal(t, "1190", child[2])

	// Karyawan (HRGA): status Resign tercatat lewat aturan resign.
	h.as("hrga@balans.local")
	validated, res := h.importFile("hris.employee", "Karyawan", []map[string]string{
		{"Nomor Karyawan": "dx-01", "Nama Lengkap": "Andi", "Jabatan": "Staf", "Status": "Aktif", "Tanggal Masuk": "2024-01-15"},
		{"Nomor Karyawan": "DX-02", "Nama Lengkap": "Budi", "Jabatan": "Staf", "Status": "Resign", "Tanggal Masuk": "2024-01-15", "Tanggal Resign": "2025-06-30"},
		{"Nomor Karyawan": "DX-03", "Nama Lengkap": "Citra", "Jabatan": "Staf", "Status": "Resign", "Tanggal Masuk": "2024-01-15"},
	})
	assert.EqualValues(t, 1, validated["errorRows"])
	require.Equal(t, "completed", res["status"], res["message"])
	rows = h.exportRows("hris.employee", "Karyawan")
	joined := map[string]string{}
	for _, r := range rows[1:] {
		joined[r[0]] = strings.Join(r, "|")
	}
	assert.Contains(t, joined["DX-01"], "Aktif")
	assert.Contains(t, joined["DX-02"], "Resign|2024-01-15|2025-06-30")

	// HRGA tidak boleh mengimpor produk.
	status, _ := h.do(http.MethodGet, "/api/v1/data-exchange/kinds/catalog.product/template", nil)
	assert.Equal(t, http.StatusForbidden, status)
}
