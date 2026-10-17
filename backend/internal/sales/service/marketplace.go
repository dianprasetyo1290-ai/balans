package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/tabular"
	"balans/internal/sales/domain"
)

// CreateMarketplaceOrder mencatat penjualan marketplace (input manual/impor sebelum integrasi, SLS-05): nomor order
// marketplace unik per marketplace, harga dari order marketplace (atau resolver bila kosong), bundel dipecah ke
// komponen, dan E-commerce Stock langsung dialokasikan FEFO (pesanan Diproses). Penjualan & HPP diakui saat serah
// ke kurir (MarkHandedOver, A-10).
func (s *DocumentService) CreateMarketplaceOrder(ctx context.Context, in domain.OrderInput) (OrderView, error) {
	a, err := actor.Require(ctx, PermMarketplaceWrite)
	if err != nil {
		return OrderView{}, err
	}
	var id uuid.UUID
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.recordMarketplaceOrder(ctx, a, in, "")
		id = o.ID
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.reload(ctx, id)
}

func (s *DocumentService) recordMarketplaceOrder(ctx context.Context, a actor.Actor, in domain.OrderInput, rowKey string) (domain.Order, error) {
	in.Type = domain.OrderMarketplace
	if in.ChannelID == uuid.Nil {
		c, ok, err := s.channelByCode(ctx, domain.OrderMarketplace.ChannelCode())
		if err != nil {
			return domain.Order{}, err
		}
		if !ok {
			return domain.Order{}, errNoMarketplaceChannel
		}
		in.ChannelID = c.ID
	}
	if in.SourceLocationID == uuid.Nil {
		locs, err := s.Locations.SaleLocations(ctx, domain.OrderMarketplace.LocationTypes())
		if err != nil {
			return domain.Order{}, err
		}
		if len(locs) == 1 {
			in.SourceLocationID = locs[0].ID
		}
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = s.today()
	}
	in.ExternalOrderID = strings.TrimSpace(in.ExternalOrderID)
	if in.SubChannelID != uuid.Nil && in.ExternalOrderID != "" {
		dup, ok, err := s.Store.OrderIDByExternal(ctx, in.SubChannelID, in.ExternalOrderID)
		if err != nil {
			return domain.Order{}, err
		}
		if ok {
			existing, err := s.Store.GetOrder(ctx, dup, false)
			if err != nil {
				return domain.Order{}, err
			}
			return domain.Order{}, &apperr.Error{
				Code: apperr.CodeDuplicateCode, Message: "Nomor order marketplace sudah tercatat.",
				Fields: map[string]string{"externalOrderId": fmt.Sprintf("Order %s sudah tercatat sebagai %s", in.ExternalOrderID, existing.Label())},
			}
		}
	}
	refs, products, err := s.orderRefs(ctx, in)
	if err != nil {
		return domain.Order{}, err
	}
	o, err := domain.NewOrder(in, refs, a.UserID)
	if err != nil {
		return domain.Order{}, err
	}
	o.ImportRowKey = rowKey
	if o, err = s.Store.InsertOrder(ctx, o, a.UserID); err != nil {
		return domain.Order{}, err
	}
	number, err := s.Numberer.Next(ctx, DocSalesOrder, o.BusinessDate)
	if err != nil {
		return domain.Order{}, err
	}
	if err := o.Confirm(number, false, s.Clock.Now()); err != nil {
		return domain.Order{}, err
	}
	if err := s.allocate(ctx, &o, products); err != nil {
		return domain.Order{}, err
	}
	if err := s.Store.SaveOrder(ctx, &o, o.Version, a.UserID, false); err != nil {
		return domain.Order{}, err
	}
	return o, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: EntityMarketplaceOrder, EntityID: o.ID, EntityLabel: o.Number, After: orderSnapshot(o),
	})
}

// --- Impor penjualan marketplace (dataexchange `sales.marketplace_order`) ---------------------

// MarketplaceImportKind adalah jenis data impor penjualan marketplace (= resource permission).
const MarketplaceImportKind = "sales.marketplace_order"

// MarketplaceColumns adalah kolom template impor penjualan marketplace: satu baris = satu order marketplace.
var MarketplaceColumns = []tabular.Column{
	{Key: "marketplace", Header: "Marketplace", Kind: tabular.KindText, Required: true, Identity: true, Example: "shopee",
		Note: "Kode sub-channel marketplace: shopee, tiktok_tokopedia, lazada, blibli."},
	{Key: "orderNumber", Header: "Nomor Order", Kind: tabular.KindText, Required: true, Identity: true, Example: "240101ABCDEF",
		Note: "Nomor order dari marketplace; tidak boleh sudah tercatat."},
	{Key: "orderDate", Header: "Tanggal Order", Kind: tabular.KindDate, Required: true, Example: "2026-10-19"},
	{Key: "items", Header: "Item", Kind: tabular.KindText, Required: true, Example: "FG-COK-500:2:185000; BDL-HEMAT:1",
		Note: "SKU:qty:harga satuan dipisah titik koma; harga kosong = harga daftar harga marketplace. SKU bundel dipecah ke komponen."},
	{Key: "sellerVoucher", Header: "Voucher Penjual", Kind: tabular.KindDecimal, Example: "10000",
		Note: "Potongan yang ditanggung penjual (rupiah) untuk seluruh order."},
	{Key: "buyerName", Header: "Nama Pembeli", Kind: tabular.KindText, Example: "Budi"},
	{Key: "note", Header: "Catatan", Kind: tabular.KindText},
}

// importField memetakan field error domain ke kolom template.
func importField(k string) string {
	switch {
	case strings.HasPrefix(k, "lines"):
		return "items"
	case k == "externalOrderId":
		return "orderNumber"
	case k == "subChannelId" || k == "channelId":
		return "marketplace"
	case k == "businessDate":
		return "orderDate"
	case k == "documentDiscount":
		return "sellerVoucher"
	case k == "buyerName" || k == "note":
		return k
	}
	return "items"
}

func remapImportError(err error) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed && appErr.Code != apperr.CodeDuplicateCode || len(appErr.Fields) == 0 {
		return err
	}
	fields := map[string]string{}
	for k, v := range appErr.Fields {
		key := importField(k)
		if cur, ok := fields[key]; ok {
			v = cur + "; " + v
		}
		fields[key] = v
	}
	return apperr.Validation(fields)
}

// ImportMarketplaceRow mencatat satu order marketplace dari baris Excel di transaksi aktif. Idempotent per baris
// impor (dataexchange.RowKey).
func (s *DocumentService) ImportMarketplaceRow(ctx context.Context, row tabular.Row, rowKey string) error {
	if _, err := actor.Require(ctx, PermMarketplaceImport); err != nil {
		return err
	}
	a, err := actor.Require(ctx, PermMarketplaceWrite)
	if err != nil {
		return err
	}
	if rowKey != "" {
		if _, ok, err := s.Store.OrderIDByImportRow(ctx, rowKey); err != nil || ok {
			return err
		}
	}
	p := tabular.Parse(row)
	in := domain.OrderInput{
		Type: domain.OrderMarketplace, ExternalOrderID: p.Text("orderNumber"), BuyerName: p.Text("buyerName"),
		Note: p.Text("note"), DocumentDiscount: domain.Discount{Kind: domain.DiscountAmount},
	}
	if d := p.Date("orderDate"); d != nil {
		in.BusinessDate = *d
	}
	if v := p.Decimal("sellerVoucher"); v != nil {
		in.DocumentDiscount.Value = *v
	}
	market := strings.ToLower(strings.TrimSpace(p.Text("marketplace")))
	channels, err := s.Ledger.Channels(ctx)
	if err != nil {
		return err
	}
	for _, c := range channels {
		if c.Kind == "channel" && c.Code == domain.OrderMarketplace.ChannelCode() {
			in.ChannelID = c.ID
		}
	}
	for _, c := range channels {
		if c.Kind == "sub_channel" && c.Code == market && c.ChannelID != nil && *c.ChannelID == in.ChannelID {
			in.SubChannelID = c.ID
		}
	}
	if in.SubChannelID == uuid.Nil {
		p.Fail("marketplace", "Marketplace "+market+" tidak dikenal")
	}
	items, msg := domain.ParseImportItems(p.Text("items"))
	if msg != "" {
		p.Fail("items", msg)
	}
	for _, it := range items {
		prod, ok, err := s.Catalog.ProductBySKU(ctx, it.SKU)
		if err != nil {
			return err
		}
		if !ok {
			p.Fail("items", "SKU "+it.SKU+" tidak ditemukan")
			continue
		}
		in.Lines = append(in.Lines, domain.LineInput{ProductID: prod.ID, Qty: it.Qty, UnitPrice: it.UnitPrice})
	}
	if err := p.Err(); err != nil {
		return err
	}
	if _, err := s.recordMarketplaceOrder(ctx, a, in, rowKey); err != nil {
		return remapImportError(err)
	}
	return nil
}
