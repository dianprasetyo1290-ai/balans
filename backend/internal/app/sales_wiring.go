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
	"balans/internal/document"
	"balans/internal/filestore"
	"balans/internal/integration/mockpayment"
	"balans/internal/inventory"
	"balans/internal/party"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/tabular"
	"balans/internal/platform/webhook"
	"balans/internal/pricing"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// maxPaymentProofBytes adalah batas ukuran file bukti bayar (foto/scan transfer). Batas teknis upload.
const maxPaymentProofBytes = 10 << 20

// buildSales menyusun modul sales (R1.3): webhook Midtrans, alasan retur, Sales Order semua channel, Proforma,
// Invoice, piutang & pembayaran pelanggan. Handler approval `sales_order_discount` didaftarkan setelah handler dev.
func (a *App) buildSales(inbox *webhook.Inbox, reasons *returnReasonAdapter) (*mockpayment.Gateway, error) {
	cfg := a.Config
	if !cfg.IsEnabled(sales.Name) {
		return nil, nil
	}
	if cfg.PaymentGateway != config.ProviderMock {
		return nil, providerNotAvailable("PAYMENT_GATEWAY", cfg.PaymentGateway)
	}
	if a.approval == nil || a.accounting == nil || a.document == nil || a.catalog == nil || a.party == nil ||
		a.warehouse == nil || a.inventory == nil || a.pricing == nil || a.filestore == nil || a.identity == nil {
		return nil, errors.New("modul sales membutuhkan modul approval, accounting, document, catalog, party, warehouse, inventory, pricing, filestore, dan identity")
	}
	gateway := mockpayment.New(a.Pool, mockpayment.Config{
		ServerKey:     cfg.MidtransServerKey,
		PublicBaseURL: cfg.PublicBaseURL,
		WebhookURL:    cfg.InternalAPIURL + "/api/v1/webhooks/midtrans",
	})
	m := sales.New(sales.Deps{
		Pool: a.Pool, Tx: a.Tx, Audit: a.audit, Gateway: gateway, Inbox: inbox, Responder: a.Responder, Log: a.Log,
		Clock: a.Clock, Catalog: salesCatalog{a.catalog}, Parties: salesParties{a.party},
		Locations: salesLocations{a.warehouse}, Pricing: a.pricing, Stock: a.inventory, Ledger: salesLedger{a.accounting},
		Approvals: a.approval, Numberer: salesNumberer{a.document}, Files: a.filestore,
		Users: procurementUsers{a.identity}, BaseURL: cfg.PublicBaseURL,
	})
	reasons.sales = m
	m.RegisterApprovalHandlers(a.approval)
	registerAuditEntities(a.audit, sales.AuditEntities())
	images := []string{filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP, filestore.TypePDF}
	a.filestore.RegisterOwner(filestore.OwnerPolicy{
		OwnerType: sales.FileOwnerPaymentProof, WritePermission: "sales.sales_order.write",
		ReadPermission: sales.PermOrderRead, ContentTypes: images, MaxBytes: maxPaymentProofBytes,
	})
	a.filestore.RegisterOwner(filestore.OwnerPolicy{
		OwnerType: sales.FileOwnerCustomerPaymentProof, WritePermission: "sales.invoice.post",
		ReadPermission: sales.PermInvoiceRead, ContentTypes: images, MaxBytes: maxPaymentProofBytes,
	})
	if a.dataexchange != nil {
		s := spec(sales.MarketplaceImportKind, "Penjualan Marketplace", "Penjualan Marketplace", sales.MarketplaceColumns())
		a.dataexchange.RegisterImporter(tableImporter{spec: s, apply: func(ctx context.Context, row tabular.Row) error {
			return m.ImportMarketplaceRow(ctx, row, dataexchange.RowKey(ctx))
		}})
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: dataexchange.UploadOwnerType(s.Kind), WritePermission: s.Kind + ".import",
			ReadPermission: s.Kind + ".import", ContentTypes: []string{filestore.TypeXLSX}, MaxBytes: dataexchange.MaxFileBytes,
		})
	}
	a.sales = m
	a.modules = append(a.modules, m)
	return gateway, nil
}

// salesCatalog membaca produk katalog (komponen bundel ikut dibaca untuk alokasi stok).
type salesCatalog struct{ catalog *catalog.Module }

func toSalesProduct(p catalog.Product) sales.ProductRef {
	ref := sales.ProductRef{
		ID: p.ID, SKU: p.SKU, Name: p.DisplayName(), UOM: p.UOM, Type: string(p.Type), IsBundle: p.IsBundle,
		IsActive: p.IsActive, Deleted: p.Deleted(),
	}
	for _, c := range p.Components {
		ref.Components = append(ref.Components, sales.ComponentQty{ProductID: c.ComponentID, Qty: c.Qty})
	}
	return ref
}

func (c salesCatalog) Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]sales.ProductRef, error) {
	out := make(map[uuid.UUID]sales.ProductRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	list, err := c.catalog.Products(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, p := range list {
		if p.IsBundle && len(p.Components) == 0 {
			if p.Components, err = c.catalog.BundleComponents(ctx, id); err != nil {
				return nil, err
			}
		}
		out[id] = toSalesProduct(p)
	}
	return out, nil
}

func (c salesCatalog) ProductBySKU(ctx context.Context, sku string) (sales.ProductRef, bool, error) {
	p, err := c.catalog.ProductByCode(ctx, sku)
	if isNotFound(err) {
		return sales.ProductRef{}, false, nil
	}
	if err != nil {
		return sales.ProductRef{}, false, err
	}
	if !strings.EqualFold(p.SKU, sku) {
		return sales.ProductRef{}, false, nil
	}
	if p.IsBundle && len(p.Components) == 0 {
		if p.Components, err = c.catalog.BundleComponents(ctx, p.ID); err != nil {
			return sales.ProductRef{}, false, err
		}
	}
	return toSalesProduct(p), true, nil
}

// salesParties membaca pelanggan & mitra serta status harga karyawan.
type salesParties struct{ party *party.Module }

func customerAddress(c party.Customer) string {
	for _, a := range c.Addresses {
		if a.IsDefault {
			parts := []string{a.AddressLine, a.District, a.City, a.Province, a.PostalCode}
			out := []string{}
			for _, p := range parts {
				if strings.TrimSpace(p) != "" {
					out = append(out, p)
				}
			}
			return strings.Join(out, ", ")
		}
	}
	return ""
}

func (p salesParties) Customer(ctx context.Context, id uuid.UUID) (sales.PartyInfo, bool, error) {
	c, err := p.party.CustomerByID(ctx, id)
	if isNotFound(err) {
		return sales.PartyInfo{}, false, nil
	}
	if err != nil {
		return sales.PartyInfo{}, false, err
	}
	return sales.PartyInfo{
		ID: c.ID, Kind: sales.PartyCustomer, Code: c.Code, Name: c.FullName, Phone: c.Phone, Type: string(c.Type),
		Address: customerAddress(c),
	}, true, nil
}

func (p salesParties) Partner(ctx context.Context, id uuid.UUID) (sales.PartyInfo, bool, error) {
	m, err := p.party.PartnerByID(ctx, id)
	if isNotFound(err) {
		return sales.PartyInfo{}, false, nil
	}
	if err != nil {
		return sales.PartyInfo{}, false, err
	}
	return sales.PartyInfo{
		ID: m.ID, Kind: sales.PartyPartner, Code: m.Code, Name: m.Name, Phone: m.PICPhone, Type: string(m.Type), Address: m.Address,
	}, true, nil
}

func (p salesParties) EmployeeEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error) {
	ok, err := p.party.EmployeePriceEligible(ctx, customerID, on)
	if isNotFound(err) {
		return false, nil
	}
	return ok, err
}

// salesLocations membaca lokasi stok gudang.
type salesLocations struct{ warehouse *warehouse.Module }

func toSalesLocation(l warehouse.Location) sales.LocationRef {
	return sales.LocationRef{ID: l.ID, Code: l.Code, Name: l.Name, Type: string(l.Type), IsActive: l.IsActive, Deleted: l.Deleted()}
}

func (l salesLocations) Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]sales.LocationRef, error) {
	out := make(map[uuid.UUID]sales.LocationRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	locs, err := l.warehouse.Locations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, loc := range locs {
		out[id] = toSalesLocation(loc)
	}
	return out, nil
}

func (l salesLocations) SaleLocations(ctx context.Context, types []string) ([]sales.LocationRef, error) {
	internal := false
	q := warehouse.LocationQuery{External: &internal}
	for _, t := range types {
		q.Types = append(q.Types, warehouse.LocationType(t))
	}
	list, err := l.warehouse.FindLocations(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]sales.LocationRef, len(list))
	for i, loc := range list {
		out[i] = toSalesLocation(loc)
	}
	return out, nil
}

// salesLedger menyesuaikan API publik accounting: channel, akun Kas & Bank, dan Poster.
type salesLedger struct{ accounting *accounting.Module }

func toSalesChannel(d accounting.DimensionValue) sales.ChannelRef {
	return sales.ChannelRef{
		ID: d.ID, Kind: string(d.Kind), Code: d.Code, Name: d.Name, ChannelID: d.ChannelID, PartyType: d.PartyType, PartyID: d.PartyID,
	}
}

func (l salesLedger) Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error) {
	return l.accounting.Post(ctx, req)
}

func (l salesLedger) Reverse(ctx context.Context, req accounting.ReversalRequest) (accounting.JournalEntry, error) {
	return l.accounting.Reverse(ctx, req)
}

func (l salesLedger) Channels(ctx context.Context) ([]sales.ChannelRef, error) {
	list, err := l.accounting.SalesChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sales.ChannelRef, len(list))
	for i, d := range list {
		out[i] = toSalesChannel(d)
	}
	return out, nil
}

func (l salesLedger) ChannelLabels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]sales.ChannelRef, error) {
	labels, err := l.accounting.DimensionLabels(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]sales.ChannelRef, len(labels))
	for id, d := range labels {
		out[id] = toSalesChannel(d)
	}
	return out, nil
}

func (l salesLedger) CashBankAccounts(ctx context.Context) ([]sales.Option, error) {
	list, err := l.accounting.PostableAccounts(ctx, cashBankPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]sales.Option, len(list))
	for i, a := range list {
		out[i] = sales.Option{ID: a.ID, Code: a.Code, Name: a.Name}
	}
	return out, nil
}

// salesNumberer mengambil nomor SO, PI, INV, dan BYR dari modul document.
type salesNumberer struct{ documents *document.Module }

func (n salesNumberer) Next(ctx context.Context, t sales.DocumentType, businessDate clock.BusinessDate) (string, error) {
	var dt document.Type
	switch t {
	case sales.DocSalesOrder:
		dt = document.TypeSalesOrder
	case sales.DocProforma:
		dt = document.TypeProformaInvoice
	case sales.DocInvoice:
		dt = document.TypeInvoice
	case sales.DocPayment:
		dt = document.TypePayment
	case sales.DocSalesReturn:
		dt = document.TypeSalesReturn
	case sales.DocCreditNote:
		dt = document.TypeCreditNote
	default:
		return "", fmt.Errorf("jenis nomor dokumen sales %q tidak dikenal", t)
	}
	return n.documents.Next(ctx, dt, businessDate)
}

var (
	_ sales.Pricing   = (*pricing.Module)(nil)
	_ sales.Stock     = (*inventory.Module)(nil)
	_ sales.Approvals = (*approval.Module)(nil)
	_ sales.Files     = (*filestore.Module)(nil)
)
