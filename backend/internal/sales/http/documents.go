package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/service"
)

// --- konversi --------------------------------------------------------------------------------

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func optFromDate(d *BusinessDate) *clock.BusinessDate {
	if d == nil || d.IsZero() {
		return nil
	}
	out := fromDate(*d)
	return &out
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseMoney(s string, field string, fields map[string]string) money.Amount {
	if strings.TrimSpace(s) == "" {
		return money.Zero
	}
	a, err := money.Parse(strings.TrimSpace(s))
	if err != nil {
		fields[field] = "Nominal tidak valid"
	}
	return a
}

func parseQty(s string, field string, fields map[string]string) decimal.Decimal {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		fields[field] = "Qty tidak valid"
		return decimal.Zero
	}
	return d
}

func qty(d decimal.Decimal) SalesQuantity { return d.StringFixed(domain.QtyScale) }

func validation(fields map[string]string) error {
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func pageParams(res paging.Page, total int64) (int, int, int64, int) {
	return res.Number, res.Size, total, res.TotalPages(total)
}

func discountIn(d *SalesDiscount, field string, fields map[string]string) domain.Discount {
	if d == nil {
		return domain.Discount{Kind: domain.DiscountAmount}
	}
	out := domain.Discount{Kind: domain.DiscountKind(d.Kind)}
	if strings.TrimSpace(d.Value) == "" {
		return out
	}
	v, err := decimal.NewFromString(strings.TrimSpace(d.Value))
	if err != nil {
		fields[field] = "Diskon tidak valid"
		return out
	}
	out.Value = v
	return out
}

func discountOut(d domain.Discount) SalesDiscount {
	kind := d.Kind
	if kind == "" {
		kind = domain.DiscountAmount
	}
	return SalesDiscount{Kind: DiscountKind(kind), Value: d.Value.String()}
}

// orderInput menyusun isian domain dari body create/update.
func orderInput(t domain.OrderType, b SalesOrderUpdate) (domain.OrderInput, error) {
	fields := map[string]string{}
	in := domain.OrderInput{
		Type: t, ChannelID: b.ChannelId, SubChannelID: b.SubChannelId, CustomerID: b.CustomerId, PartnerID: b.PartnerId,
		BuyerName: str(b.BuyerName), ExternalOrderID: str(b.ExternalOrderId), ClientPONumber: str(b.ClientPoNumber),
		BusinessDate: fromDate(b.BusinessDate), ShipToName: str(b.ShipToName), ShipToPhone: str(b.ShipToPhone),
		ShipToAddress: str(b.ShipToAddress), Note: str(b.Note),
		DocumentDiscount: discountIn(b.DocumentDiscount, "documentDiscount", fields),
	}
	if b.SourceLocationId != nil {
		in.SourceLocationID = *b.SourceLocationId
	}
	if b.PaymentMode != nil {
		in.PaymentMode = domain.PaymentMode(*b.PaymentMode)
	}
	if b.PaymentTermDays != nil {
		in.PaymentTermDays = *b.PaymentTermDays
	}
	if b.ShippingAmount != nil {
		in.Shipping = parseMoney(*b.ShippingAmount, "shippingAmount", fields)
	}
	for i, l := range b.Lines {
		key := fmt.Sprintf("lines.%d.", i)
		li := domain.LineInput{
			ProductID: l.ProductId, Qty: parseQty(l.Qty, key+"qty", fields), Note: str(l.Note),
			Discount: discountIn(l.Discount, key+"discount", fields),
		}
		if l.UnitPrice != nil {
			p := parseMoney(*l.UnitPrice, key+"unitPrice", fields)
			li.UnitPrice = &p
		}
		in.Lines = append(in.Lines, li)
	}
	return in, validation(fields)
}

func productRef(id uuid.UUID, products map[uuid.UUID]service.ProductRef) SalesProductRef {
	p, ok := products[id]
	if !ok {
		return SalesProductRef{Id: id, Name: "(produk tidak ditemukan)"}
	}
	return SalesProductRef{Id: p.ID, Sku: p.SKU, Name: p.Name, Uom: p.UOM}
}

func locationRef(id uuid.UUID, l *service.LocationRef) SalesLocationRef {
	if l == nil {
		return SalesLocationRef{Id: id}
	}
	return SalesLocationRef{Id: l.ID, Code: l.Code, Name: l.Name, LocationType: l.Type}
}

func toOrderSummary(o domain.Order, channel, subChannel string) SalesOrderSummary {
	return SalesOrderSummary{
		Id: o.ID, Number: optStr(o.Number), OrderType: SalesOrderType(o.Type), ChannelId: o.ChannelID, ChannelName: channel,
		SubChannelId: o.SubChannelID, SubChannelName: subChannel, CustomerId: o.CustomerID, PartnerId: o.PartnerID,
		PartyName: o.PartyName, PartyCode: o.PartyCode, PartyPhone: o.PartyPhone, ExternalOrderId: o.ExternalOrderID,
		ClientPoNumber: o.ClientPONumber, BusinessDate: toDate(o.BusinessDate), PaymentMode: SalesPaymentMode(o.PaymentMode),
		TotalAmount: o.Total.String(), Status: SalesDocumentStatus(o.Status), CreatedAt: o.CreatedAt, Version: o.Version,
	}
}

func toOrder(ctx context.Context, v service.OrderView) SalesOrder {
	s := toOrderSummary(v.Order, v.Channel.Name, v.SubChannel.Name)
	out := SalesOrder{
		Id: s.Id, Number: s.Number, OrderType: s.OrderType, ChannelId: s.ChannelId, ChannelName: s.ChannelName,
		SubChannelId: s.SubChannelId, SubChannelName: s.SubChannelName, CustomerId: s.CustomerId, PartnerId: s.PartnerId,
		PartyName: s.PartyName, PartyCode: s.PartyCode, PartyPhone: s.PartyPhone, ExternalOrderId: s.ExternalOrderId,
		ClientPoNumber: s.ClientPoNumber, BusinessDate: s.BusinessDate, PaymentMode: s.PaymentMode,
		TotalAmount: s.TotalAmount, Status: s.Status, CreatedAt: s.CreatedAt, Version: s.Version,
		SourceLocation: locationRef(v.SourceLocationID, v.Location), PaymentTermDays: v.PaymentTermDays,
		ShipToName: v.ShipToName, ShipToPhone: v.ShipToPhone, ShipToAddress: v.ShipToAddress,
		GrossAmount: v.Gross.String(), LineDiscountAmount: v.LineDiscount.String(),
		DocumentDiscount: discountOut(v.DocumentDiscount), DocumentDiscountAmount: v.DocumentDiscountAmount.String(),
		ShippingAmount: v.Shipping.String(), DiscountApprovalRequired: v.DiscountApprovalRequired,
		DiscountApprovedAt: v.DiscountApprovedAt, Note: v.Note, CancelReason: v.CancelReason, ConfirmedAt: v.ConfirmedAt,
		ShippedAt: v.ShippedAt, DeliveredAt: v.DeliveredAt, PaymentProofFileId: v.PaymentProofFileID,
		PaymentProofFileName: optStr(v.PaymentProofName), CreatedByName: v.CreatedByName,
		Lines: []SalesOrderLine{}, Allocations: []SalesOrderAllocation{}, Proformas: []SalesDocumentRef{},
		Invoices: []SalesDocumentRef{}, Shipments: []SalesShipment{}, AllowedActions: []SalesOrderAllowedActions{},
	}
	shipped := lineShipped(v.Order)
	for _, l := range v.Lines {
		out.Lines = append(out.Lines, SalesOrderLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products), IsBundle: l.IsBundle, Qty: qty(l.Qty),
			UnitPrice: l.Price.UnitPrice.String(), PriceRule: l.Price.Rule, PriceSourceId: l.Price.SourceID,
			PriceListId: l.Price.PriceListID, PriceListVersionId: l.Price.PriceListVersionID,
			PriceDescription: l.Price.Description, Discount: discountOut(l.Discount), GrossAmount: l.Gross.String(),
			DiscountAmount: l.DiscountAmount.String(), LineTotalAmount: l.Total.String(), ShippedQty: qty(shipped[l.ID]),
			Note: l.Note,
		})
	}
	for _, a := range v.Allocations {
		batches := make([]SalesAllocationBatch, len(a.Batches))
		for i, b := range a.Batches {
			batches[i] = SalesAllocationBatch{BatchId: b.BatchID, Qty: qty(b.Qty)}
			if b.BatchID != nil {
				if info, ok := v.Batches[*b.BatchID]; ok {
					batches[i].BatchNumber = optStr(info.Number)
					if info.ExpiryDate != nil {
						d := toDate(*info.ExpiryDate)
						batches[i].ExpiryDate = &d
					}
				}
			}
		}
		out.Allocations = append(out.Allocations, SalesOrderAllocation{
			Id: a.ID, LineId: a.LineID, Product: productRef(a.ProductID, v.Products), Location: locationRef(a.LocationID, v.Location),
			Qty: qty(a.Qty), ShippedQty: qty(a.ShippedQty), Released: a.Released, Batches: batches,
		})
	}
	for _, p := range v.Proformas {
		valid := toDate(p.ValidUntil)
		out.Proformas = append(out.Proformas, SalesDocumentRef{
			Id: p.ID, Number: p.Number, Status: SalesDocumentStatus(p.Status), Amount: p.Amount.String(), ValidUntil: &valid,
		})
	}
	for _, inv := range v.Invoices {
		paid := inv.Paid.String()
		out.Invoices = append(out.Invoices, SalesDocumentRef{
			Id: inv.ID, Number: inv.Number, Status: SalesDocumentStatus(inv.Status), Amount: inv.Total.String(), PaidAmount: &paid,
		})
	}
	for _, sh := range v.Shipments {
		out.Shipments = append(out.Shipments, SalesShipment{
			Id: sh.ID, SourceType: sh.SourceType, SourceId: sh.SourceID, SourceLabel: sh.SourceLabel,
			BusinessDate: toDate(sh.BusinessDate), CostAmount: sh.Cost.String(), CreatedAt: sh.CreatedAt,
		})
	}
	if v.DeliveryOrdersEnabled {
		dos := make([]SalesDeliveryOrderRef, len(v.DeliveryOrders))
		for i, d := range v.DeliveryOrders {
			dos[i] = SalesDeliveryOrderRef{Id: d.ID, Number: optStr(d.Number), Status: d.Status}
		}
		out.DeliveryOrders = &dos
	}
	for _, a := range service.OrderViewActions(ctx, v) {
		out.AllowedActions = append(out.AllowedActions, SalesOrderAllowedActions(a))
	}
	return out
}

// lineShipped menghitung qty baris terkirim: produk biasa = qty alokasinya, bundel = minimum rasio komponen.
func lineShipped(o domain.Order) map[uuid.UUID]decimal.Decimal {
	out := map[uuid.UUID]decimal.Decimal{}
	lines := map[uuid.UUID]domain.Line{}
	for _, l := range o.Lines {
		lines[l.ID] = l
	}
	seen := map[uuid.UUID]bool{}
	for _, a := range o.Allocations {
		l, ok := lines[a.LineID]
		if !ok || !a.Qty.IsPositive() {
			continue
		}
		ratio := a.ShippedQty.Div(a.Qty).Mul(l.Qty).Round(domain.QtyScale)
		if !seen[a.LineID] || ratio.LessThan(out[a.LineID]) {
			out[a.LineID] = ratio
		}
		seen[a.LineID] = true
	}
	return out
}

// --- Sales Order -----------------------------------------------------------------------------

// ListSalesOrders mengembalikan daftar pesanan.
func (h *SalesHandler) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequestObject) (ListSalesOrdersResponseObject, error) {
	p := req.Params
	f := service.OrderFilter{
		ChannelID: p.ChannelId, SubChannelID: p.SubChannelId, DateFrom: optFromDate(p.DateFrom), DateTo: optFromDate(p.DateTo),
		Search: str(p.Q), Sort: paging.ParseSort(p.Sort, service.OrderSortFields, paging.Sort{}),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.OrderType != nil {
		for _, t := range *p.OrderType {
			f.Types = append(f.Types, domain.OrderType(t))
		}
	}
	res, err := h.docs.ListOrders(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]SalesOrderSummary, len(res.Items))
	for i, o := range res.Items {
		items[i] = toOrderSummary(o.Order, o.ChannelName, o.SubChannelName)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListSalesOrders200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateSalesOrder menyimpan draft pesanan atau mencatat penjualan marketplace.
func (h *SalesHandler) CreateSalesOrder(ctx context.Context, req CreateSalesOrderRequestObject) (CreateSalesOrderResponseObject, error) {
	b := req.Body
	in, err := orderInput(domain.OrderType(b.OrderType), SalesOrderUpdate{
		ChannelId: b.ChannelId, SubChannelId: b.SubChannelId, CustomerId: b.CustomerId, PartnerId: b.PartnerId,
		BuyerName: b.BuyerName, ExternalOrderId: b.ExternalOrderId, ClientPoNumber: b.ClientPoNumber,
		BusinessDate: b.BusinessDate, SourceLocationId: b.SourceLocationId, PaymentMode: b.PaymentMode,
		PaymentTermDays: b.PaymentTermDays, ShipToName: b.ShipToName, ShipToPhone: b.ShipToPhone,
		ShipToAddress: b.ShipToAddress, DocumentDiscount: b.DocumentDiscount, ShippingAmount: b.ShippingAmount,
		Note: b.Note, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.docs.CreateOrder(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateSalesOrder201JSONResponse(toOrder(ctx, v)), nil
}

// GetSalesOrder mengembalikan detail pesanan.
func (h *SalesHandler) GetSalesOrder(ctx context.Context, req GetSalesOrderRequestObject) (GetSalesOrderResponseObject, error) {
	v, err := h.docs.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetSalesOrder200JSONResponse(toOrder(ctx, v)), nil
}

// UpdateSalesOrder mengubah draft pesanan.
func (h *SalesHandler) UpdateSalesOrder(ctx context.Context, req UpdateSalesOrderRequestObject) (UpdateSalesOrderResponseObject, error) {
	cur, err := h.docs.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	in, err := orderInput(cur.Type, *req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.UpdateOrder(ctx, req.Id, req.Body.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateSalesOrder200JSONResponse(toOrder(ctx, v)), nil
}

// ConfirmSalesOrder mengonfirmasi pesanan.
func (h *SalesHandler) ConfirmSalesOrder(ctx context.Context, req ConfirmSalesOrderRequestObject) (ConfirmSalesOrderResponseObject, error) {
	v, err := h.docs.ConfirmOrder(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return ConfirmSalesOrder200JSONResponse(toOrder(ctx, v)), nil
}

// CancelSalesOrder membatalkan pesanan.
func (h *SalesHandler) CancelSalesOrder(ctx context.Context, req CancelSalesOrderRequestObject) (CancelSalesOrderResponseObject, error) {
	v, err := h.docs.CancelOrder(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelSalesOrder200JSONResponse(toOrder(ctx, v)), nil
}

// UploadSalesOrderPaymentProof menautkan bukti bayar.
func (h *SalesHandler) UploadSalesOrderPaymentProof(ctx context.Context, req UploadSalesOrderPaymentProofRequestObject) (UploadSalesOrderPaymentProofResponseObject, error) {
	v, err := h.docs.UploadPaymentProof(ctx, req.Id, req.Body.Version, req.Body.FileId)
	if err != nil {
		return nil, err
	}
	return UploadSalesOrderPaymentProof200JSONResponse(toOrder(ctx, v)), nil
}

// ListSalesChannelOptions mengembalikan channel & sub-channel aktif.
func (h *SalesHandler) ListSalesChannelOptions(ctx context.Context, _ ListSalesChannelOptionsRequestObject) (ListSalesChannelOptionsResponseObject, error) {
	list, err := h.docs.ChannelOptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(ListSalesChannelOptions200JSONResponse, len(list))
	for i, c := range list {
		out[i] = SalesChannelOption{
			Id: c.ID, Kind: SalesChannelOptionKind(c.Kind), Code: c.Code, Name: c.Name, ChannelId: c.ChannelID,
			PartyType: optStr(c.PartyType), PartyId: c.PartyID,
		}
	}
	return out, nil
}

// ListSalesLocationOptions mengembalikan lokasi asal stok per jenis pesanan.
func (h *SalesHandler) ListSalesLocationOptions(ctx context.Context, req ListSalesLocationOptionsRequestObject) (ListSalesLocationOptionsResponseObject, error) {
	list, err := h.docs.SaleLocations(ctx, domain.OrderType(req.Params.OrderType))
	if err != nil {
		return nil, err
	}
	out := make(ListSalesLocationOptions200JSONResponse, len(list))
	for i, l := range list {
		out[i] = locationRef(l.ID, &l)
	}
	return out, nil
}

// ListSalesCashBankAccounts mengembalikan akun Kas & Bank.
func (h *SalesHandler) ListSalesCashBankAccounts(ctx context.Context, _ ListSalesCashBankAccountsRequestObject) (ListSalesCashBankAccountsResponseObject, error) {
	list, err := h.docs.CashBankAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(ListSalesCashBankAccounts200JSONResponse, len(list))
	for i, a := range list {
		out[i] = SalesAccountOption{Id: a.ID, Code: a.Code, Name: a.Name}
	}
	return out, nil
}

// --- Proforma Invoice ------------------------------------------------------------------------

func (h *SalesHandler) toProforma(ctx context.Context, p service.ProformaRowView) ProformaInvoice {
	out := ProformaInvoice{
		Id: p.ID, Number: p.Number, SalesOrderId: p.OrderID, SalesOrderNumber: p.OrderNumber, PartyName: p.PartyName,
		OrderType: SalesOrderType(p.OrderType), Amount: p.Amount.String(), IssueDate: toDate(p.IssueDate),
		ValidUntil: toDate(p.ValidUntil), Status: SalesDocumentStatus(p.Status), PaidAt: p.PaidAt, InvoiceId: p.InvoiceID,
		CancelReason: p.CancelReason, Version: p.Version, AllowedActions: []ProformaInvoiceAllowedActions{},
	}
	for _, a := range h.docs.ProformaActions(ctx, p.Proforma) {
		out.AllowedActions = append(out.AllowedActions, ProformaInvoiceAllowedActions(a))
	}
	return out
}

func (h *SalesHandler) toProformaDetail(ctx context.Context, v service.ProformaView) ProformaInvoiceDetail {
	p := h.toProforma(ctx, v.ProformaRow)
	out := ProformaInvoiceDetail{
		Id: p.Id, Number: p.Number, SalesOrderId: p.SalesOrderId, SalesOrderNumber: p.SalesOrderNumber,
		PartyName: p.PartyName, OrderType: p.OrderType, Amount: p.Amount, IssueDate: p.IssueDate, ValidUntil: p.ValidUntil,
		Status: p.Status, PaidAt: p.PaidAt, InvoiceId: p.InvoiceId, CancelReason: p.CancelReason, Version: p.Version,
		SalesOrder: toOrder(ctx, v.Order), AllowedActions: []ProformaInvoiceDetailAllowedActions{},
	}
	for _, a := range p.AllowedActions {
		out.AllowedActions = append(out.AllowedActions, ProformaInvoiceDetailAllowedActions(a))
	}
	return out
}

// ListProformaInvoices mengembalikan daftar PI.
func (h *SalesHandler) ListProformaInvoices(ctx context.Context, req ListProformaInvoicesRequestObject) (ListProformaInvoicesResponseObject, error) {
	p := req.Params
	f := service.ProformaFilter{Search: str(p.Q)}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	res, err := h.docs.ListProformas(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]ProformaInvoice, len(res.Items))
	for i, r := range res.Items {
		items[i] = h.toProforma(ctx, r)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListProformaInvoices200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// GetProformaInvoice mengembalikan detail PI.
func (h *SalesHandler) GetProformaInvoice(ctx context.Context, req GetProformaInvoiceRequestObject) (GetProformaInvoiceResponseObject, error) {
	v, err := h.docs.GetProforma(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetProformaInvoice200JSONResponse(h.toProformaDetail(ctx, v)), nil
}

// ConfirmProformaPayment mengonfirmasi pembayaran PI.
func (h *SalesHandler) ConfirmProformaPayment(ctx context.Context, req ConfirmProformaPaymentRequestObject) (ConfirmProformaPaymentResponseObject, error) {
	b := req.Body
	in := service.ProformaPaymentInput{
		Method: domain.PaymentMethod(b.Method), AccountCode: b.AccountCode, Reference: str(b.Reference), ProofFileID: b.ProofFileId,
	}
	if d := optFromDate(b.PaymentDate); d != nil {
		in.PaymentDate = *d
	}
	v, err := h.docs.ConfirmProformaPayment(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return ConfirmProformaPayment200JSONResponse(h.toProformaDetail(ctx, v)), nil
}

// --- Invoice ---------------------------------------------------------------------------------

func (h *SalesHandler) toInvoiceSummary(inv domain.Invoice) SalesInvoiceSummary {
	return SalesInvoiceSummary{
		Id: inv.ID, Number: inv.Number, InvoiceType: inv.Type, SalesOrderId: inv.OrderID, SalesOrderNumber: inv.OrderNumber,
		PartyType: SalesPartyType(inv.PartyType), PartyId: inv.PartyID, PartyCode: inv.PartyCode, PartyName: inv.PartyName,
		InvoiceDate: toDate(inv.InvoiceDate), DueDate: toDate(inv.DueDate), TotalAmount: inv.Total.String(),
		PaidAmount: inv.Paid.String(), OutstandingAmount: inv.Outstanding().String(),
		DaysOverdue: overdue(inv, h.docs.Today()), Status: SalesDocumentStatus(inv.Status), Version: inv.Version,
	}
}

func overdue(inv domain.Invoice, today clock.BusinessDate) int {
	if !inv.IsOpen() {
		return 0
	}
	return inv.DaysOverdue(today)
}

// ListSalesInvoices mengembalikan daftar invoice.
func (h *SalesHandler) ListSalesInvoices(ctx context.Context, req ListSalesInvoicesRequestObject) (ListSalesInvoicesResponseObject, error) {
	p := req.Params
	f := service.InvoiceFilter{
		PartyID: p.PartyId, DateFrom: optFromDate(p.DateFrom), DateTo: optFromDate(p.DateTo), Search: str(p.Q),
		Sort: paging.ParseSort(p.Sort, service.InvoiceSortFields, paging.Sort{}),
	}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, domain.Status(s))
		}
	}
	if p.PartyType != nil {
		f.PartyType = string(*p.PartyType)
	}
	if p.Overdue != nil && *p.Overdue {
		today := h.docs.Today()
		f.OverdueOn = &today
	}
	res, err := h.docs.ListInvoices(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]SalesInvoiceSummary, len(res.Items))
	for i, inv := range res.Items {
		items[i] = h.toInvoiceSummary(inv)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListSalesInvoices200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// GetSalesInvoice mengembalikan detail invoice.
func (h *SalesHandler) GetSalesInvoice(ctx context.Context, req GetSalesInvoiceRequestObject) (GetSalesInvoiceResponseObject, error) {
	v, err := h.docs.GetInvoice(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	s := h.toInvoiceSummary(v.Invoice)
	out := SalesInvoice{
		Id: s.Id, Number: s.Number, InvoiceType: s.InvoiceType, SalesOrderId: s.SalesOrderId, SalesOrderNumber: s.SalesOrderNumber,
		PartyType: s.PartyType, PartyId: s.PartyId, PartyCode: s.PartyCode, PartyName: s.PartyName, InvoiceDate: s.InvoiceDate,
		DueDate: s.DueDate, TotalAmount: s.TotalAmount, PaidAmount: s.PaidAmount, OutstandingAmount: s.OutstandingAmount,
		DaysOverdue: s.DaysOverdue, Status: s.Status, Version: s.Version, ChannelName: v.ChannelName,
		SubChannelName: v.SubChannelName, GrossAmount: v.Gross.String(), DiscountAmount: v.Discount.String(),
		ShippingAmount: v.Shipping.String(), CancelReason: v.CancelReason, ShipToAddress: v.ShipToAddress,
		Lines: []SalesInvoiceLine{}, Payments: []SalesInvoicePayment{}, AllowedActions: []SalesInvoiceAllowedActions{},
	}
	if v.OrderType != "" {
		t := SalesOrderType(v.OrderType)
		out.OrderType = &t
	}
	for _, l := range v.Lines {
		out.Lines = append(out.Lines, SalesInvoiceLine{
			Id: l.ID, LineNo: l.LineNo, ProductId: l.ProductID, Description: l.Description, Qty: qty(l.Qty),
			UnitPrice: l.UnitPrice.String(), DiscountAmount: l.Discount.String(), LineTotalAmount: l.Total.String(),
		})
	}
	for _, p := range v.Payments {
		out.Payments = append(out.Payments, SalesInvoicePayment{
			PaymentId: p.PaymentID, PaymentNumber: p.PaymentNumber, PaymentDate: toDate(p.PaymentDate),
			Status: SalesDocumentStatus(p.Status), Amount: p.Amount.String(),
		})
	}
	for _, a := range service.InvoiceActions(ctx, v.Invoice) {
		out.AllowedActions = append(out.AllowedActions, SalesInvoiceAllowedActions(a))
	}
	return GetSalesInvoice200JSONResponse(out), nil
}

// GetSalesInvoiceShareText mengembalikan teks invoice untuk WhatsApp.
func (h *SalesHandler) GetSalesInvoiceShareText(ctx context.Context, req GetSalesInvoiceShareTextRequestObject) (GetSalesInvoiceShareTextResponseObject, error) {
	text, err := h.docs.InvoiceShareText(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetSalesInvoiceShareText200JSONResponse{Text: text}, nil
}

// --- Pembayaran pelanggan --------------------------------------------------------------------

func toPayment(ctx context.Context, p domain.CustomerPayment) CustomerPayment {
	out := CustomerPayment{
		Id: p.ID, Number: p.Number, PartyType: SalesPartyType(p.PartyType), PartyId: p.PartyID, PartyName: p.PartyName,
		PaymentDate: toDate(p.PaymentDate), Amount: p.Amount.String(), Method: CustomerPaymentMethod(p.Method),
		AccountCode: p.AccountCode, Reference: p.Reference, ProofFileId: p.ProofFileID, Note: p.Note,
		Status: SalesDocumentStatus(p.Status), CancelReason: p.CancelReason, JournalEntryId: p.JournalEntryID,
		CreatedAt: p.CreatedAt, Version: p.Version, Allocations: []CustomerPaymentAllocation{},
		AllowedActions: []CustomerPaymentAllowedActions{},
	}
	for _, a := range p.Allocations {
		number := a.InvoiceNumber
		out.Allocations = append(out.Allocations, CustomerPaymentAllocation{InvoiceId: a.InvoiceID, InvoiceNumber: &number, Amount: a.Amount.String()})
	}
	for _, a := range service.PaymentActions(ctx, p) {
		out.AllowedActions = append(out.AllowedActions, CustomerPaymentAllowedActions(a))
	}
	return out
}

// ListCustomerPayments mengembalikan daftar pembayaran.
func (h *SalesHandler) ListCustomerPayments(ctx context.Context, req ListCustomerPaymentsRequestObject) (ListCustomerPaymentsResponseObject, error) {
	p := req.Params
	f := service.PaymentFilter{PartyID: p.PartyId, DateFrom: optFromDate(p.DateFrom), DateTo: optFromDate(p.DateTo), Search: str(p.Q)}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.PartyType != nil {
		f.PartyType = string(*p.PartyType)
	}
	res, err := h.docs.ListPayments(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]CustomerPayment, len(res.Items))
	for i, pay := range res.Items {
		items[i] = toPayment(ctx, pay)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListCustomerPayments200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateCustomerPayment mencatat pembayaran pelanggan.
func (h *SalesHandler) CreateCustomerPayment(ctx context.Context, req CreateCustomerPaymentRequestObject) (CreateCustomerPaymentResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	in := domain.PaymentInput{
		PartyType: string(b.PartyType), PartyID: b.PartyId, PaymentDate: fromDate(b.PaymentDate),
		Amount: parseMoney(b.Amount, "amount", fields), Method: domain.PaymentMethod(b.Method), AccountCode: b.AccountCode,
		Reference: str(b.Reference), ProofFileID: b.ProofFileId, Note: str(b.Note),
	}
	for i, a := range b.Allocations {
		in.Allocations = append(in.Allocations, domain.PaymentAllocation{
			InvoiceID: a.InvoiceId, Amount: parseMoney(a.Amount, fmt.Sprintf("allocations.%d.amount", i), fields),
		})
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	p, err := h.docs.CreatePayment(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateCustomerPayment201JSONResponse(toPayment(ctx, p)), nil
}

// GetCustomerPayment mengembalikan detail pembayaran.
func (h *SalesHandler) GetCustomerPayment(ctx context.Context, req GetCustomerPaymentRequestObject) (GetCustomerPaymentResponseObject, error) {
	p, err := h.docs.GetPayment(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetCustomerPayment200JSONResponse(toPayment(ctx, p)), nil
}

// CancelCustomerPayment membatalkan pembayaran.
func (h *SalesHandler) CancelCustomerPayment(ctx context.Context, req CancelCustomerPaymentRequestObject) (CancelCustomerPaymentResponseObject, error) {
	p, err := h.docs.CancelPayment(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelCustomerPayment200JSONResponse(toPayment(ctx, p)), nil
}

// GetReceivableAging mengembalikan laporan umur piutang.
func (h *SalesHandler) GetReceivableAging(ctx context.Context, req GetReceivableAgingRequestObject) (GetReceivableAgingResponseObject, error) {
	asOf := h.docs.Today()
	if d := optFromDate(req.Params.AsOf); d != nil {
		asOf = *d
	}
	partyType := ""
	if req.Params.PartyType != nil {
		partyType = string(*req.Params.PartyType)
	}
	rows, err := h.docs.AgingReport(ctx, asOf, partyType)
	if err != nil {
		return nil, err
	}
	out := GetReceivableAging200JSONResponse{AsOf: toDate(asOf), Rows: make([]ReceivableAgingRow, len(rows))}
	totals := domain.AgingRow{Buckets: map[domain.AgingBucket]money.Amount{}, Total: money.Zero}
	for i, r := range rows {
		out.Rows[i] = agingRow(r)
		for b, v := range r.Buckets {
			totals.Buckets[b] = totals.Buckets[b].Add(v)
		}
		totals.Total, totals.Invoices = totals.Total.Add(r.Total), totals.Invoices+r.Invoices
	}
	totals.PartyName, totals.PartyType = "Total", domain.PartyCustomer
	out.Totals = agingRow(totals)
	return out, nil
}

func agingRow(r domain.AgingRow) ReceivableAgingRow {
	b := func(k domain.AgingBucket) Money { return r.Buckets[k].String() }
	return ReceivableAgingRow{
		PartyType: SalesPartyType(r.PartyType), PartyId: r.PartyID, PartyName: r.PartyName, Invoices: r.Invoices,
		Current: b(domain.BucketCurrent), Days1To30: b(domain.Bucket1To30), Days31To60: b(domain.Bucket31To60),
		Days61To90: b(domain.Bucket61To90), Over90: b(domain.BucketOver90), Total: r.Total.String(),
	}
}
