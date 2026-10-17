package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/catalog"
	"balans/internal/dataexchange"
	"balans/internal/filestore"
	"balans/internal/hris"
	"balans/internal/identity"
	"balans/internal/party"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
	"balans/internal/warehouse"
)

// tableImporter mengisi dataexchange.Importer dengan fungsi publik modul pemilik data; validasi bisnis tetap
// di modul tersebut.
type tableImporter struct {
	spec  dataexchange.Spec
	apply func(ctx context.Context, row tabular.Row) error
}

func (t tableImporter) Spec() dataexchange.Spec { return t.spec }

func (t tableImporter) ApplyRow(ctx context.Context, row tabular.Row) error { return t.apply(ctx, row) }

// validatingImporter memeriksa baris tanpa menulis (dataexchange.RowValidator).
type validatingImporter struct {
	tableImporter
	validate func(ctx context.Context, row tabular.Row) error
}

func (v validatingImporter) ValidateRow(ctx context.Context, row tabular.Row) error {
	return v.validate(ctx, row)
}

type tableExporter struct {
	spec   dataexchange.Spec
	export func(ctx context.Context, emit func(tabular.Row) error) error
}

func (t tableExporter) Spec() dataexchange.Spec { return t.spec }

func (t tableExporter) Export(ctx context.Context, emit func(tabular.Row) error) error {
	return t.export(ctx, emit)
}

// dxFiles menyesuaikan API publik filestore untuk dataexchange.
type dxFiles struct {
	files *filestore.Module
}

func (f dxFiles) Info(ctx context.Context, id uuid.UUID) (dataexchange.FileInfo, error) {
	file, err := f.files.Info(ctx, id)
	if err != nil {
		return dataexchange.FileInfo{}, err
	}
	return dataexchange.FileInfo{
		ID: file.ID, Name: file.OriginalName, OwnerType: file.OwnerType, SHA256: file.SHA256,
		Uploaded: file.Status == "uploaded", CreatedBy: file.CreatedBy,
	}, nil
}

func (f dxFiles) Attach(ctx context.Context, id uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	return f.files.Attach(ctx, id, ownerType, ownerID)
}

func (f dxFiles) Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	return f.files.OpenContent(ctx, id)
}

func (f dxFiles) Save(ctx context.Context, ownerType string, ownerID uuid.UUID, name, contentType string, data []byte) (uuid.UUID, error) {
	file, err := f.files.Save(ctx, ownerType, ownerID, name, contentType, data)
	return file.ID, err
}

func (f dxFiles) DownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	return f.files.SignedDownloadURL(ctx, id)
}

// dxActors menyusun actor pembuat job dari modul identity.
type dxActors struct {
	identity *identity.Module
}

func (a dxActors) ActorFor(ctx context.Context, userID uuid.UUID) (actor.Actor, error) {
	return a.identity.ActorFor(ctx, userID)
}

// partyProducts menyesuaikan API publik catalog untuk modul party (nama produk, SKU impor kode produk mitra).
type partyProducts struct {
	catalog *catalog.Module
}

func (p partyProducts) Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]party.ProductRef, error) {
	list, err := p.catalog.Products(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]party.ProductRef, len(list))
	for id, prod := range list {
		out[id] = party.ProductRef{ID: prod.ID, SKU: prod.SKU, Name: prod.DisplayName()}
	}
	return out, nil
}

func (p partyProducts) ProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, bool, error) {
	prod, err := p.catalog.ProductByCode(ctx, sku)
	if isNotFound(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	if prod.SKU != sku {
		// ProductByCode juga mencocokkan barcode; impor kode produk mitra memakai SKU persis.
		return uuid.Nil, false, nil
	}
	return prod.ID, true, nil
}

// warehouseParties mencari pemilik lokasi eksternal di modul party.
type warehouseParties struct {
	party *party.Module
}

var errUnsupportedParty = errors.New("jenis pihak lokasi eksternal belum didukung")

func (w warehouseParties) PartyIDByCode(ctx context.Context, t warehouse.PartyType, code string) (uuid.UUID, bool, error) {
	switch t {
	case warehouse.PartySupplier:
		return w.party.SupplierIDByCode(ctx, code)
	case warehouse.PartyPartner:
		return w.party.PartnerIDByCode(ctx, code)
	}
	return uuid.Nil, false, errUnsupportedParty
}

func (w warehouseParties) PartyCode(ctx context.Context, t warehouse.PartyType, id uuid.UUID) (string, error) {
	switch t {
	case warehouse.PartySupplier:
		s, err := w.party.SupplierByID(ctx, id)
		if isNotFound(err) {
			return "", nil
		}
		return s.Code, err
	case warehouse.PartyPartner:
		p, err := w.party.PartnerByID(ctx, id)
		if isNotFound(err) {
			return "", nil
		}
		return p.Code, err
	}
	return "", nil
}

// hrisDivisions membaca master divisi accounting untuk modul hris.
type hrisDivisions struct {
	accounting *accounting.Module
}

func (h hrisDivisions) Divisions(ctx context.Context) ([]hris.DivisionRef, error) {
	list, err := h.accounting.Divisions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]hris.DivisionRef, len(list))
	for i, d := range list {
		out[i] = hris.DivisionRef{ID: d.ID, Code: d.Code, Name: d.Name}
	}
	return out, nil
}

func isNotFound(err error) bool { return apperr.HasCode(err, apperr.CodeNotFound) }

// spec menyusun definisi jenis data dataexchange.
func spec(kind, label, sheet string, columns []tabular.Column) dataexchange.Spec {
	return dataexchange.Spec{Kind: kind, Label: label, Sheet: sheet, Columns: columns}
}

// buildDataExchange menyusun modul impor/ekspor Excel dan mendaftarkan jenis data dari modul yang aktif
// (S-125, S-134). Dipanggil setelah semua modul master disusun.
func (a *App) buildDataExchange() error {
	if !a.Config.IsEnabled(dataexchange.Name) {
		return nil
	}
	if a.identity == nil || a.filestore == nil {
		return errors.New("modul dataexchange membutuhkan modul identity dan filestore")
	}
	d := dataexchange.Deps{
		Pool: a.Pool, Tx: a.Tx, Jobs: a.Jobs, Clock: a.Clock, Log: a.Log, Responder: a.Responder,
		Files: dxFiles{files: a.filestore}, Actors: dxActors{identity: a.identity},
	}
	if a.notification != nil {
		d.Notifier = a.notification
	}
	dx := dataexchange.New(d)
	a.dataexchange = dx
	a.modules = append(a.modules, dx)

	register := func(s dataexchange.Spec, imp dataexchange.Importer, exp func(ctx context.Context, emit func(tabular.Row) error) error) {
		dx.RegisterImporter(imp)
		dx.RegisterExporter(tableExporter{spec: s, export: exp})
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: dataexchange.UploadOwnerType(s.Kind), WritePermission: s.Kind + ".import",
			ReadPermission: s.Kind + ".import", ContentTypes: []string{filestore.TypeXLSX}, MaxBytes: dataexchange.MaxFileBytes,
		})
	}
	if a.catalog != nil {
		s := spec("catalog.product", "Produk", "Produk", catalog.ProductColumns())
		register(s, tableImporter{spec: s, apply: a.catalog.ImportProductRow}, a.catalog.ExportProducts)
	}
	if a.party != nil {
		s := spec("party.customer", "Pelanggan", "Pelanggan", party.CustomerColumns())
		register(s, validatingImporter{
			tableImporter: tableImporter{spec: s, apply: a.party.ImportCustomerRow}, validate: a.party.ValidateCustomerRow,
		}, a.party.ExportCustomers)
		s = spec("party.supplier", "Pemasok & Maklon", "Pemasok & Maklon", party.SupplierColumns())
		register(s, tableImporter{spec: s, apply: a.party.ImportSupplierRow}, a.party.ExportSuppliers)
		s = spec("party.partner_product_code", "Kode Produk Mitra", "Kode Produk Mitra", party.ProductCodeColumns())
		register(s, tableImporter{spec: s, apply: a.party.ImportProductCodeRow}, a.party.ExportProductCodes)
	}
	if a.hris != nil {
		s := spec("hris.employee", "Karyawan", "Karyawan", hris.EmployeeColumns())
		register(s, tableImporter{spec: s, apply: a.hris.ImportEmployeeRow}, a.hris.ExportEmployees)
	}
	if a.accounting != nil {
		s := spec("accounting.account", "Chart of Accounts", "COA", accounting.AccountColumns())
		register(s, tableImporter{spec: s, apply: a.accounting.ImportAccountRow}, a.accounting.ExportAccounts)
	}
	if a.warehouse != nil {
		s := spec("warehouse.location", "Lokasi Stok", "Gudang & Lokasi", warehouse.LocationColumns())
		register(s, tableImporter{spec: s, apply: a.warehouse.ImportLocationRow}, a.warehouse.ExportLocations)
	}
	return nil
}

// connectMasterData memasang adapter lintas modul master data setelah semuanya disusun.
func (a *App) connectMasterData() {
	if a.party != nil && a.catalog != nil {
		a.party.SetProducts(partyProducts{catalog: a.catalog})
	}
	if a.hris != nil && a.accounting != nil {
		a.hris.SetDivisions(hrisDivisions{accounting: a.accounting})
	}
	if a.warehouse != nil && a.party != nil {
		a.warehouse.SetParties(warehouseParties{party: a.party})
	}
}
