package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/catalog"
	"balans/internal/dataexchange"
	"balans/internal/filestore"
	"balans/internal/identity"
	"balans/internal/inventory"
	"balans/internal/party"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/tabular"
	"balans/internal/warehouse"
)

// Wiring R1.4: tutup buku (approval buka kembali), saldo awal (pihak, nilai persediaan), dan impor/ekspor
// transaksi (S-408, S-410 – S-413).

// connectAccountingClose memasang adapter modul lain ke accounting setelah semua modul disusun. Handler approval
// `fiscal_period_reopen` didaftarkan setelah handler dev sehingga menggantikannya.
func (a *App) connectAccountingClose() {
	if a.accounting == nil {
		return
	}
	if a.approval != nil {
		a.accounting.SetApprovals(accountingApprovals{approval: a.approval})
		decide := func(approved bool) func(ctx context.Context, d approval.Decided) error {
			return func(ctx context.Context, d approval.Decided) error {
				return a.accounting.DecideReopen(ctx, d.Subject.ID, approved, d.DecidedBy, d.Note)
			}
		}
		a.approval.RegisterHandler(approval.SubjectFiscalPeriodReopen, accounting.PermFiscalPeriodRead,
			approval.Handler{OnApproved: decide(true), OnRejected: decide(false)})
	}
	if a.identity != nil {
		a.accounting.SetUserDirectory(accountingUsers{identity: a.identity})
	}
	if a.party != nil {
		a.accounting.SetParties(accountingParties{party: a.party})
	}
	if a.inventory != nil && a.catalog != nil && a.warehouse != nil {
		a.accounting.SetStockValuer(stockValuer{inventory: a.inventory, catalog: a.catalog, warehouse: a.warehouse})
	}
}

// accountingApprovals mengajukan buka kembali periode ke engine approval.
type accountingApprovals struct{ approval *approval.Module }

func (p accountingApprovals) RequestApproval(ctx context.Context, s accounting.ApprovalSubject) error {
	_, err := p.approval.Request(ctx, approval.Subject{
		Type: approval.SubjectType(s.Type), ID: s.ID, Label: s.Label, Link: s.Link, Summary: s.Summary, Amount: money.Zero,
	})
	return err
}

// accountingUsers membaca nama user dari identity.
type accountingUsers struct{ identity *identity.Module }

func (u accountingUsers) UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok || id == uuid.Nil {
			continue
		}
		user, err := u.identity.UserByID(ctx, id)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = user.FullName
	}
	return out, nil
}

// accountingParties mencari pihak saldo terbuka: piutang → pelanggan lalu mitra; utang → pemasok.
type accountingParties struct{ party *party.Module }

func (p accountingParties) PartyByCode(ctx context.Context, t accounting.OpeningItemType, code string) (string, uuid.UUID, bool, error) {
	if t == accounting.OpeningPayable {
		id, ok, err := p.party.SupplierIDByCode(ctx, code)
		return accounting.PartySupplier, id, ok, err
	}
	if id, ok, err := p.party.CustomerIDByCode(ctx, code); err != nil || ok {
		return accounting.PartyCustomer, id, ok, err
	}
	id, ok, err := p.party.PartnerIDByCode(ctx, code)
	return accounting.PartyPartner, id, ok, err
}

// stockValuer menjumlahkan nilai ledger persediaan per akun persediaan (jenis produk × jenis lokasi).
type stockValuer struct {
	inventory *inventory.Module
	catalog   *catalog.Module
	warehouse *warehouse.Module
}

func (v stockValuer) InventoryValueByAccount(ctx context.Context, asOf clock.BusinessDate) (map[string]money.Amount, error) {
	values, err := v.inventory.StockValues(ctx, asOf)
	if err != nil {
		return nil, err
	}
	productIDs := make([]uuid.UUID, 0, len(values))
	locationIDs := make([]uuid.UUID, 0, len(values))
	for _, sv := range values {
		productIDs = append(productIDs, sv.ProductID)
		locationIDs = append(locationIDs, sv.LocationID)
	}
	products, err := v.catalog.Products(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	locations, err := v.warehouse.Locations(ctx, locationIDs)
	if err != nil {
		return nil, err
	}
	out := map[string]money.Amount{}
	for _, sv := range values {
		acc := warehouse.InventoryAccount(string(products[sv.ProductID].Type), locations[sv.LocationID].Type)
		out[acc] = out[acc].Add(money.FromDecimal(sv.Value))
	}
	return out, nil
}

// transactionResource adalah resource permission impor/ekspor transaksi (Desain Role & Approval 3).
const transactionResource = "dataexchange.transaction"

// registerAccountingExchange mendaftarkan impor/ekspor transaksi: Jurnal Umum, saldo awal (neraca, piutang,
// utang, aset tetap, stok), dan ekspor Neraca Saldo / Buku Besar dengan parameter.
func (a *App) registerAccountingExchange(dx *dataexchange.Module) {
	if a.accounting == nil {
		return
	}
	owner := func(s dataexchange.Spec) {
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: dataexchange.UploadOwnerType(s.Kind), WritePermission: s.ImportPermission(),
			ReadPermission: s.ImportPermission(), ContentTypes: []string{filestore.TypeXLSX}, MaxBytes: dataexchange.MaxFileBytes,
		})
	}
	txSpec := func(kind, label, sheet string, columns []tabular.Column) dataexchange.Spec {
		s := spec(kind, label, sheet, columns)
		s.Resource = transactionResource
		return s
	}
	both := func(s dataexchange.Spec, apply func(ctx context.Context, row tabular.Row) error, exp func(ctx context.Context, emit func(tabular.Row) error) error) {
		dx.RegisterImporter(tableImporter{spec: s, apply: apply})
		if exp != nil {
			dx.RegisterExporter(tableExporter{spec: s, export: exp})
		}
		owner(s)
	}
	m := a.accounting
	both(txSpec("accounting.journal_entry", "Jurnal Umum", "Jurnal Umum", accounting.JournalImportColumns()),
		func(ctx context.Context, row tabular.Row) error {
			return m.ImportJournalRow(ctx, dataexchange.RowKey(ctx), row)
		}, nil)
	both(txSpec("accounting.opening_balance", "Neraca Saldo Awal", "Neraca Saldo Awal", accounting.OpeningBalanceColumns()),
		m.ImportOpeningBalanceRow, m.ExportOpeningBalances)
	both(txSpec("accounting.opening_receivable", "Piutang Terbuka", "Piutang Terbuka", accounting.OpeningReceivableColumns()),
		m.ImportOpeningReceivableRow, nil)
	both(txSpec("accounting.opening_payable", "Utang Terbuka", "Utang Terbuka", accounting.OpeningPayableColumns()),
		m.ImportOpeningPayableRow, nil)
	both(txSpec("accounting.opening_fixed_asset", "Saldo Awal Aset Tetap", "Aset Tetap", accounting.OpeningAssetColumns()),
		m.ImportOpeningAssetRow, nil)
	if a.inventory != nil && a.catalog != nil && a.warehouse != nil {
		imp := openingStockImporter{app: a}
		dx.RegisterImporter(imp)
		owner(imp.Spec())
	}
	dx.RegisterExporter(tableExporter{
		spec: txSpec("accounting.trial_balance", "Neraca Saldo", "Neraca Saldo", accounting.TrialBalanceColumns()),
		export: func(ctx context.Context, emit func(tabular.Row) error) error {
			return m.ExportTrialBalance(ctx, dataexchange.ExportParams(ctx), emit)
		},
	})
	dx.RegisterExporter(tableExporter{
		spec: txSpec("accounting.general_ledger", "Buku Besar", "Buku Besar", accounting.LedgerColumns()),
		export: func(ctx context.Context, emit func(tabular.Row) error) error {
			return m.ExportLedger(ctx, dataexchange.ExportParams(ctx), emit)
		},
	})
}

// openingStockColumns adalah kolom sheet Saldo Stok (Template Migrasi Saldo Awal).
var openingStockColumns = []tabular.Column{
	{Key: "locationCode", Header: "Kode Lokasi", Kind: tabular.KindText, Required: true, Identity: true, Example: "UTAMA-FG",
		Note: "Ada di Gudang & Lokasi, termasuk maklon & konsinyasi."},
	{Key: "sku", Header: "SKU/Kode", Kind: tabular.KindText, Required: true, Identity: true, Example: "MS-COK-30"},
	{Key: "batchNumber", Header: "Nomor Batch", Kind: tabular.KindText, Identity: true, Example: "B2312",
		Note: "Wajib untuk produk ber-batch."},
	{Key: "productionDate", Header: "Tanggal Produksi", Kind: tabular.KindDate, Example: "2026-06-01"},
	{Key: "expiryDate", Header: "Tanggal Kedaluwarsa", Kind: tabular.KindDate, Example: "2027-12-12", Note: "Wajib untuk Produk Jadi."},
	{Key: "qty", Header: "Qty", Kind: tabular.KindDecimal, Required: true, Example: "1200", Note: "> 0, maks. 3 desimal."},
	{Key: "unitCost", Header: "HPP per Unit", Kind: tabular.KindDecimal, Required: true, Example: "14250.5", Note: "> 0, maks. 4 desimal."},
}

// openingStockNamespace membuat ID dokumen sumber saldo stok per baris impor (idempotent per baris).
var openingStockNamespace = uuid.MustParse("1c9f1f8e-3b7a-4c1e-9a52-6f0d3b8e2a71")

// openingStockImporter mencatat saldo stok per batch per lokasi sebagai penerimaan dengan HPP (cost layer) pada
// tanggal cut-over, tanpa jurnal (nilai GL masuk lewat Neraca Saldo Awal A-36).
type openingStockImporter struct{ app *App }

func (openingStockImporter) Spec() dataexchange.Spec {
	s := spec("inventory.opening_stock", "Saldo Stok", "Saldo Stok", openingStockColumns)
	s.Resource = transactionResource
	return s
}

// BatchSize menyimpan per 200 baris (idempotent per baris lewat RowKey).
func (openingStockImporter) BatchSize() int { return 200 }

func (i openingStockImporter) ApplyRow(ctx context.Context, row tabular.Row) error {
	a := i.app
	cutover, posted, ok, err := a.accounting.CutoverDate(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.WithMessage(apperr.CodeValidationFailed, "Isi tanggal cut-over di Keuangan & Akuntansi › Saldo Awal sebelum impor saldo stok.")
	}
	if posted {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Saldo awal sudah diposting; saldo stok tidak bisa diimpor lagi.")
	}
	p := tabular.Parse(row)
	sku := strings.TrimSpace(p.Text("sku"))
	prod, err := a.catalog.ProductByCode(ctx, sku)
	switch {
	case isNotFound(err) || (err == nil && prod.SKU != sku):
		p.Fail("sku", "SKU tidak ditemukan di Produk atau Packaging & Material")
	case err != nil:
		return err
	}
	locCode := strings.TrimSpace(p.Text("locationCode"))
	locs, err := a.warehouse.FindLocations(ctx, warehouse.LocationQuery{})
	if err != nil {
		return err
	}
	var locID uuid.UUID
	for _, l := range locs {
		if strings.EqualFold(l.Code, locCode) {
			locID = l.ID
		}
	}
	if locID == uuid.Nil {
		p.Fail("locationCode", "Kode lokasi tidak ditemukan")
	}
	qty, cost := p.Decimal("qty"), p.Decimal("unitCost")
	if qty != nil && (!qty.IsPositive() || qty.Exponent() < -3) {
		p.Fail("qty", "Qty harus lebih dari 0 (maks. 3 desimal)")
	}
	if cost != nil && (cost.IsNegative() || cost.Exponent() < -4) {
		p.Fail("unitCost", "HPP per unit 0 atau lebih (maks. 4 desimal)")
	}
	batchNo := strings.TrimSpace(p.Text("batchNumber"))
	expiry, production := p.Date("expiryDate"), p.Date("productionDate")
	if prod.HasBatch && batchNo == "" {
		p.Fail("batchNumber", "Nomor batch wajib untuk produk ber-batch")
	}
	if prod.Type == catalog.TypeFinishedGood && batchNo != "" && expiry == nil {
		p.Fail("expiryDate", "Tanggal kedaluwarsa wajib untuk Produk Jadi")
	}
	if err := p.Err(); err != nil {
		return err
	}
	line := inventory.Line{
		Kind: inventory.KindReceive, ProductID: prod.ID, Qty: *qty, UnitCost: cost,
		To: &inventory.Place{LocationID: locID, Status: inventory.StatusAvailable}, Note: "Saldo awal cut-over",
	}
	if batchNo != "" {
		line.Batch = &inventory.NewBatch{Number: batchNo, ProductionDate: production, ExpiryDate: expiry}
	}
	key := dataexchange.RowKey(ctx)
	if key == "" {
		key = fmt.Sprintf("%s|%s|%s", locCode, sku, batchNo)
	}
	_, err = a.inventory.Post(ctx, inventory.Posting{
		Source: inventory.Source{
			Type: accounting.SourceOpeningBalance, ID: uuid.NewSHA1(openingStockNamespace, []byte(key)),
			Label: fmt.Sprintf("Saldo Awal Stok baris %d", row.Number), BusinessDate: cutover,
		},
		Lines: []inventory.Line{line},
	})
	if errors.Is(err, inventory.ErrAlreadyPosted) {
		return nil
	}
	return err
}
