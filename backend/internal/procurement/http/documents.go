package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
	"balans/internal/procurement/service"
)

// Handler Penerimaan Barang, Tagihan Pemasok, Retur Pembelian, dan laporan Barang Diterima Belum Ditagih.

func unitCost(d decimal.Decimal) PurchaseUnitCost { return d.StringFixed(domain.UnitCostScale) }

func percent(d decimal.Decimal) PurchasePercent { return d.StringFixed(4) }

func optQty(s *PurchaseQuantity, field string, fields map[string]string) decimal.Decimal {
	if s == nil || strings.TrimSpace(*s) == "" {
		return decimal.Zero
	}
	return parseQty(*s, field, fields)
}

func product(id uuid.UUID, products map[uuid.UUID]service.ProductRef) PurchaseProductRef {
	return *productRef(&id, products)
}

func location(id uuid.UUID, locs map[uuid.UUID]service.LocationRef) PurchaseLocationRef {
	return *optOption(&id, locs)
}

// --- Penerimaan Barang ------------------------------------------------------------------------

// ListProcurementInboundLocations mengembalikan lokasi Inbound aktif.
func (h *ProcurementHandler) ListProcurementInboundLocations(ctx context.Context, _ ListProcurementInboundLocationsRequestObject) (ListProcurementInboundLocationsResponseObject, error) {
	list, err := h.svc.InboundLocations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PurchaseLocationRef, len(list))
	for i, l := range list {
		out[i] = PurchaseLocationRef{Id: l.ID, Code: l.Code, Name: l.Name}
	}
	return ListProcurementInboundLocations200JSONResponse(out), nil
}

func toReceiptSummary(r domain.GoodsReceipt) GoodsReceiptSummary {
	return GoodsReceiptSummary{
		Id: r.ID, Number: optStr(r.Number), PurchaseOrderId: r.PurchaseOrderID, PurchaseOrderNumber: r.OrderNumber,
		OrderType: PurchaseOrderType(r.OrderType), SupplierId: r.SupplierID, SupplierName: r.SupplierName,
		BusinessDate: toDate(r.BusinessDate), DeliveryNote: r.DeliveryNote, InventoryAmount: r.InventoryAmount.String(),
		GrniAmount: r.GRNIAmount.String(), Status: GoodsReceiptStatus(r.Status), CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
}

func toReceipt(ctx context.Context, v service.ReceiptView) GoodsReceipt {
	s := toReceiptSummary(v.GoodsReceipt)
	out := GoodsReceipt{
		Id: s.Id, Number: s.Number, PurchaseOrderId: s.PurchaseOrderId, PurchaseOrderNumber: s.PurchaseOrderNumber,
		OrderType: s.OrderType, SupplierId: s.SupplierId, SupplierName: s.SupplierName, BusinessDate: s.BusinessDate,
		DeliveryNote: s.DeliveryNote, InventoryAmount: s.InventoryAmount, GrniAmount: s.GrniAmount, Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
		DivisionId: v.DivisionID, DivisionName: v.DivisionName, Location: location(v.LocationID, v.Locations),
		TollLocation: optOption(v.TollLocationID, v.Locations), Note: v.Note, MaterialAmount: v.MaterialAmount.String(),
		InboundId: v.InboundID, JournalEntryId: v.JournalEntryID, ReversalJournalEntryId: v.ReversalEntryID,
		CancelReason: v.CancelReason, PostedAt: v.PostedAt, CreatedByName: v.CreatedByName,
		Lines: make([]GoodsReceiptLine, len(v.Lines)), Materials: make([]GoodsReceiptMaterial, len(v.Materials)),
		AllowedActions: []GoodsReceiptAllowedActions{},
	}
	for i, l := range v.Lines {
		ol := v.OrderLines[l.OrderLineID]
		out.Lines[i] = GoodsReceiptLine{
			Id: l.ID, LineNo: l.LineNo, PurchaseOrderLineId: l.OrderLineID, Product: product(l.ProductID, v.Products),
			OrderedQty: qty(ol.OrderedQty), GoodQty: qty(l.GoodQty), RejectQty: qty(l.RejectQty),
			RejectDisposition: RejectDisposition(l.RejectDisposition), BatchNumber: l.BatchNumber,
			ProductionDate: optToDate(l.ProductionDate), ExpiryDate: optToDate(l.ExpiryDate), BatchId: l.BatchID,
			UnitPrice: l.UnitPrice.String(), GrniQty: qty(l.GRNIQty), GrniAmount: l.GRNIAmount.String(),
			MaterialCostAmount: l.MaterialCost.String(), InventoryAmount: l.InventoryAmount.String(),
			UnitCost: unitCost(l.UnitCost), InventoryAccountCode: l.InventoryAccount, BilledQty: qty(l.BilledQty),
			ReturnedQty: qty(l.ReturnedQty), OpenQty: qty(l.OpenQty()), Note: l.Note,
		}
	}
	for i, m := range v.Materials {
		out.Materials[i] = GoodsReceiptMaterial{
			Id: m.ID, LineNo: m.LineNo, Product: product(m.ProductID, v.Products), UsedGoodQty: qty(m.UsedGoodQty),
			UsedRejectQty: qty(m.UsedRejectQty), CostAmount: m.CostAmount.String(), AccountCode: m.AccountCode, Note: m.Note,
		}
	}
	for _, a := range service.ReceiptActions(ctx, v.GoodsReceipt) {
		out.AllowedActions = append(out.AllowedActions, GoodsReceiptAllowedActions(a))
	}
	return out
}

func receiptInput(orderID uuid.UUID, f GoodsReceiptFields) (domain.ReceiptInput, error) {
	fields := map[string]string{}
	in := domain.ReceiptInput{
		PurchaseOrderID: orderID, BusinessDate: fromDate(f.BusinessDate), LocationID: f.LocationId,
		DeliveryNote: str(f.DeliveryNote), Note: str(f.Note),
	}
	for i, l := range f.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		line := domain.ReceiptLineInput{
			OrderLineID: l.PurchaseOrderLineId, GoodQty: parseQty(l.GoodQty, p+"goodQty", fields),
			RejectQty: optQty(l.RejectQty, p+"rejectQty", fields), BatchNumber: str(l.BatchNumber),
			ProductionDate: optFromDate(l.ProductionDate), ExpiryDate: optFromDate(l.ExpiryDate), Note: str(l.Note),
		}
		if l.RejectDisposition != nil {
			line.RejectDisposition = domain.RejectDisposition(*l.RejectDisposition)
		}
		in.Lines = append(in.Lines, line)
	}
	if f.Materials != nil {
		for i, m := range *f.Materials {
			p := fmt.Sprintf("materials.%d.", i)
			in.Materials = append(in.Materials, domain.MaterialUsageInput{
				ProductID: m.ProductId, UsedGoodQty: parseQty(m.UsedGoodQty, p+"usedGoodQty", fields),
				UsedRejectQty: parseQty(m.UsedRejectQty, p+"usedRejectQty", fields), Note: str(m.Note),
			})
		}
	}
	return in, validation(fields)
}

// ListGoodsReceipts mengembalikan daftar Penerimaan Barang.
func (h *ProcurementHandler) ListGoodsReceipts(ctx context.Context, req ListGoodsReceiptsRequestObject) (ListGoodsReceiptsResponseObject, error) {
	p := req.Params
	f := service.ReceiptFilter{
		PurchaseOrderID: p.PurchaseOrderId, SupplierID: p.SupplierId, DateFrom: optFromDate(p.DateFrom),
		DateTo: optFromDate(p.DateTo), Search: str(p.Q),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.OrderType != nil {
		for _, t := range *p.OrderType {
			f.Types = append(f.Types, domain.OrderType(t))
		}
	}
	res, err := h.svc.ListReceipts(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]GoodsReceiptSummary, len(res.Items))
	for i, r := range res.Items {
		items[i] = toReceiptSummary(r)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListGoodsReceipts200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateGoodsReceipt menyimpan draft Penerimaan Barang.
func (h *ProcurementHandler) CreateGoodsReceipt(ctx context.Context, req CreateGoodsReceiptRequestObject) (CreateGoodsReceiptResponseObject, error) {
	b := req.Body
	in, err := receiptInput(b.PurchaseOrderId, GoodsReceiptFields{
		BusinessDate: b.BusinessDate, LocationId: b.LocationId, DeliveryNote: b.DeliveryNote, Note: b.Note,
		Lines: b.Lines, Materials: b.Materials,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateReceipt(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateGoodsReceipt201JSONResponse(toReceipt(ctx, v)), nil
}

// GetGoodsReceipt mengembalikan detail Penerimaan Barang.
func (h *ProcurementHandler) GetGoodsReceipt(ctx context.Context, req GetGoodsReceiptRequestObject) (GetGoodsReceiptResponseObject, error) {
	v, err := h.svc.GetReceipt(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetGoodsReceipt200JSONResponse(toReceipt(ctx, v)), nil
}

// UpdateGoodsReceipt mengubah draft Penerimaan Barang.
func (h *ProcurementHandler) UpdateGoodsReceipt(ctx context.Context, req UpdateGoodsReceiptRequestObject) (UpdateGoodsReceiptResponseObject, error) {
	cur, err := h.svc.GetReceipt(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in, err := receiptInput(cur.PurchaseOrderID, GoodsReceiptFields{
		BusinessDate: b.BusinessDate, LocationId: b.LocationId, DeliveryNote: b.DeliveryNote, Note: b.Note,
		Lines: b.Lines, Materials: b.Materials,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateReceipt(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateGoodsReceipt200JSONResponse(toReceipt(ctx, v)), nil
}

// PostGoodsReceipt memposting Penerimaan Barang.
func (h *ProcurementHandler) PostGoodsReceipt(ctx context.Context, req PostGoodsReceiptRequestObject) (PostGoodsReceiptResponseObject, error) {
	v, err := h.svc.PostReceipt(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostGoodsReceipt200JSONResponse(toReceipt(ctx, v)), nil
}

// CancelGoodsReceipt membatalkan Penerimaan Barang.
func (h *ProcurementHandler) CancelGoodsReceipt(ctx context.Context, req CancelGoodsReceiptRequestObject) (CancelGoodsReceiptResponseObject, error) {
	v, err := h.svc.CancelReceipt(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelGoodsReceipt200JSONResponse(toReceipt(ctx, v)), nil
}

// --- Tagihan Pemasok --------------------------------------------------------------------------

func toBillSummary(b domain.SupplierBill) SupplierBillSummary {
	return SupplierBillSummary{
		Id: b.ID, Number: b.Number, PurchaseOrderId: b.PurchaseOrderID, PurchaseOrderNumber: b.OrderNumber,
		SupplierId: b.SupplierID, SupplierName: b.SupplierName, BillDate: toDate(b.BillDate), DueDate: toDate(b.DueDate),
		SubtotalAmount: b.Subtotal.String(), TotalPayableAmount: b.TotalPayable.String(), PaidAmount: b.PaidAmount.String(),
		MatchStatus: SupplierBillMatchStatus(b.MatchStatus), Status: SupplierBillStatus(b.Status),
		PaymentBlocked: b.PaymentBlocked(), CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, Version: b.Version,
	}
}

func toBill(ctx context.Context, v service.BillView) SupplierBill {
	s := toBillSummary(v.SupplierBill)
	out := SupplierBill{
		Id: s.Id, Number: s.Number, PurchaseOrderId: s.PurchaseOrderId, PurchaseOrderNumber: s.PurchaseOrderNumber,
		SupplierId: s.SupplierId, SupplierName: s.SupplierName, BillDate: s.BillDate, DueDate: s.DueDate,
		SubtotalAmount: s.SubtotalAmount, TotalPayableAmount: s.TotalPayableAmount, PaidAmount: s.PaidAmount,
		MatchStatus: s.MatchStatus, Status: s.Status, PaymentBlocked: s.PaymentBlocked, CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt, Version: s.Version,
		DivisionId: v.DivisionID, DivisionName: v.DivisionName, Note: v.Note, GrniAmount: v.GRNIAmount.String(),
		PriceVarianceAmount: v.PriceVariance.String(), WithholdingType: WithholdingType(v.WithholdingType),
		WithholdingRate: percent(v.WithholdingRate), WithholdingAmount: v.WithholdingAmount.String(),
		AdvanceAppliedAmount: v.AdvanceApplied.String(), OpenAmount: v.OpenAmount().String(),
		JournalEntryId: v.JournalEntryID, AdvanceJournalEntryId: v.AdvanceJournalEntryID, CancelReason: v.CancelReason,
		SubmittedAt: v.SubmittedAt, PostedAt: v.PostedAt, CreatedByName: v.CreatedByName,
		QtyTolerancePercent: percent(domain.QtyTolerancePercent), PriceTolerancePercent: percent(domain.PriceTolerancePercent),
		Lines: make([]SupplierBillLine, len(v.Lines)), Advances: make([]SupplierBillAdvance, len(v.Advances)),
		AllowedActions: []SupplierBillAllowedActions{},
	}
	for i, l := range v.Lines {
		ref := v.Receipts[l.ReceiptLineID]
		out.Lines[i] = SupplierBillLine{
			Id: l.ID, LineNo: l.LineNo, GoodsReceiptLineId: l.ReceiptLineID, GoodsReceiptId: ref.ReceiptID,
			GoodsReceiptNumber: ref.ReceiptNumber, Product: product(l.ProductID, v.Products), BatchNumber: ref.Line.BatchNumber,
			Qty: qty(l.Qty), ReceiptOpenQty: qty(l.ReceiptOpenQty), UnitPrice: l.UnitPrice.String(),
			PoUnitPrice: l.POUnitPrice.String(), Amount: l.Amount.String(), GrniAmount: l.GRNIAmount.String(),
			PriceVarianceAmount: l.PriceVariance.String(), PriceVariancePercent: percent(l.PriceVariancePercent()),
			QtyMismatch: l.QtyMismatch(), PriceMismatch: l.PriceMismatch(), ExceedsTolerance: l.ExceedsTolerance, Note: l.Note,
		}
	}
	for i, a := range v.Advances {
		out.Advances[i] = SupplierBillAdvance{AdvanceId: a.AdvanceID, AdvanceNumber: a.AdvanceNumber, Amount: a.Amount.String()}
	}
	for _, a := range service.BillActions(ctx, v.SupplierBill) {
		out.AllowedActions = append(out.AllowedActions, SupplierBillAllowedActions(a))
	}
	return out
}

func billInput(orderID uuid.UUID, f SupplierBillFields) (domain.BillInput, error) {
	fields := map[string]string{}
	in := domain.BillInput{
		PurchaseOrderID: orderID, Number: f.Number, BillDate: fromDate(f.BillDate), DueDate: optFromDate(f.DueDate),
		Note: str(f.Note),
	}
	if f.WithholdingType != nil {
		w := domain.WithholdingType(*f.WithholdingType)
		in.WithholdingType = &w
	}
	if f.WithholdingRate != nil && strings.TrimSpace(*f.WithholdingRate) != "" {
		r, err := decimal.NewFromString(strings.TrimSpace(*f.WithholdingRate))
		if err != nil {
			fields["withholdingRate"] = "Tarif tidak valid"
		} else {
			in.WithholdingRate = &r
		}
	}
	for i, l := range f.Lines {
		p := fmt.Sprintf("lines.%d.", i)
		in.Lines = append(in.Lines, domain.BillLineInput{
			ReceiptLineID: l.GoodsReceiptLineId, Qty: parseQty(l.Qty, p+"qty", fields),
			UnitPrice: parseMoney(l.UnitPrice, p+"unitPrice", fields), Note: str(l.Note),
		})
	}
	return in, validation(fields)
}

// ListBillableReceipts mengembalikan baris GR PO yang masih bisa ditagih.
func (h *ProcurementHandler) ListBillableReceipts(ctx context.Context, req ListBillableReceiptsRequestObject) (ListBillableReceiptsResponseObject, error) {
	b, err := h.svc.BillableReceipts(ctx, req.Params.PurchaseOrderId)
	if err != nil {
		return nil, err
	}
	out := BillableReceipts{
		PurchaseOrderId: b.Order.ID, PurchaseOrderNumber: b.Order.Number, SupplierId: b.Order.SupplierID,
		SupplierName: b.Order.SupplierName, PaymentTermDays: b.Order.PaymentTermDays,
		DefaultWithholdingType: WithholdingType(b.DefaultWithholding), DefaultWithholdingRate: percent(b.DefaultWithholding.DefaultRate()),
		OutstandingAdvanceAmount: b.OutstandingAdvance.String(), QtyTolerancePercent: percent(domain.QtyTolerancePercent),
		PriceTolerancePercent: percent(domain.PriceTolerancePercent), Lines: make([]BillableReceiptLine, len(b.Lines)),
	}
	for i, l := range b.Lines {
		out.Lines[i] = BillableReceiptLine{
			GoodsReceiptLineId: l.Line.ID, GoodsReceiptId: l.ReceiptID, GoodsReceiptNumber: l.ReceiptNumber,
			BusinessDate: toDate(l.BusinessDate), Product: product(l.Line.ProductID, b.Products), BatchNumber: l.Line.BatchNumber,
			PoUnitPrice: l.Line.UnitPrice.String(), ReceivedQty: qty(l.Line.GRNIQty), BilledQty: qty(l.Line.BilledQty),
			OpenQty: qty(l.Line.OpenQty()), PendingQty: qty(b.PendingQty[l.Line.ID]),
		}
	}
	return ListBillableReceipts200JSONResponse(out), nil
}

// ListSupplierBills mengembalikan daftar tagihan pemasok.
func (h *ProcurementHandler) ListSupplierBills(ctx context.Context, req ListSupplierBillsRequestObject) (ListSupplierBillsResponseObject, error) {
	p := req.Params
	f := service.BillFilter{
		PurchaseOrderID: p.PurchaseOrderId, SupplierID: p.SupplierId, DateFrom: optFromDate(p.DateFrom),
		DateTo: optFromDate(p.DateTo), Search: str(p.Q),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.MatchStatus != nil {
		f.MatchStatus = domain.MatchStatus(*p.MatchStatus)
	}
	res, err := h.svc.ListBills(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]SupplierBillSummary, len(res.Items))
	for i, b := range res.Items {
		items[i] = toBillSummary(b)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListSupplierBills200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateSupplierBill menyimpan draft tagihan pemasok.
func (h *ProcurementHandler) CreateSupplierBill(ctx context.Context, req CreateSupplierBillRequestObject) (CreateSupplierBillResponseObject, error) {
	b := req.Body
	in, err := billInput(b.PurchaseOrderId, SupplierBillFields{
		Number: b.Number, BillDate: b.BillDate, DueDate: b.DueDate, WithholdingType: b.WithholdingType,
		WithholdingRate: b.WithholdingRate, Note: b.Note, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateBill(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateSupplierBill201JSONResponse(toBill(ctx, v)), nil
}

// GetSupplierBill mengembalikan detail tagihan pemasok.
func (h *ProcurementHandler) GetSupplierBill(ctx context.Context, req GetSupplierBillRequestObject) (GetSupplierBillResponseObject, error) {
	v, err := h.svc.GetBill(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetSupplierBill200JSONResponse(toBill(ctx, v)), nil
}

// UpdateSupplierBill mengubah draft tagihan pemasok.
func (h *ProcurementHandler) UpdateSupplierBill(ctx context.Context, req UpdateSupplierBillRequestObject) (UpdateSupplierBillResponseObject, error) {
	cur, err := h.svc.GetBill(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in, err := billInput(cur.PurchaseOrderID, SupplierBillFields{
		Number: b.Number, BillDate: b.BillDate, DueDate: b.DueDate, WithholdingType: b.WithholdingType,
		WithholdingRate: b.WithholdingRate, Note: b.Note, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateBill(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateSupplierBill200JSONResponse(toBill(ctx, v)), nil
}

// PostSupplierBill memposting tagihan pemasok.
func (h *ProcurementHandler) PostSupplierBill(ctx context.Context, req PostSupplierBillRequestObject) (PostSupplierBillResponseObject, error) {
	v, err := h.svc.PostBill(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostSupplierBill200JSONResponse(toBill(ctx, v)), nil
}

// ReviseSupplierBill mengembalikan tagihan yang ditolak ke Draft.
func (h *ProcurementHandler) ReviseSupplierBill(ctx context.Context, req ReviseSupplierBillRequestObject) (ReviseSupplierBillResponseObject, error) {
	v, err := h.svc.ReviseBill(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return ReviseSupplierBill200JSONResponse(toBill(ctx, v)), nil
}

// CancelSupplierBill membatalkan tagihan.
func (h *ProcurementHandler) CancelSupplierBill(ctx context.Context, req CancelSupplierBillRequestObject) (CancelSupplierBillResponseObject, error) {
	v, err := h.svc.CancelBill(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelSupplierBill200JSONResponse(toBill(ctx, v)), nil
}

// --- Retur Pembelian --------------------------------------------------------------------------

func toReturnSummary(r domain.PurchaseReturn) PurchaseReturnSummary {
	return PurchaseReturnSummary{
		Id: r.ID, Number: optStr(r.Number), DebitNoteNumber: optStr(r.DebitNoteNumber), PurchaseOrderId: r.PurchaseOrderID,
		PurchaseOrderNumber: r.OrderNumber, SupplierId: r.SupplierID, SupplierName: r.SupplierName,
		Origin: PurchaseReturnOrigin(r.Origin), BusinessDate: toDate(r.BusinessDate), Reason: r.Reason,
		TotalAmount: r.Total.String(), Status: PurchaseReturnStatus(r.Status), CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
}

func toReturn(ctx context.Context, v service.ReturnView) PurchaseReturn {
	s := toReturnSummary(v.PurchaseReturn)
	out := PurchaseReturn{
		Id: s.Id, Number: s.Number, DebitNoteNumber: s.DebitNoteNumber, PurchaseOrderId: s.PurchaseOrderId,
		PurchaseOrderNumber: s.PurchaseOrderNumber, SupplierId: s.SupplierId, SupplierName: s.SupplierName,
		Origin: s.Origin, BusinessDate: s.BusinessDate, Reason: s.Reason, TotalAmount: s.TotalAmount, Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
		DivisionId: v.DivisionID, DivisionName: v.DivisionName, SourceType: v.SourceType, SourceLabel: v.SourceLabel,
		Note: v.Note, UnbilledAmount: v.UnbilledAmount.String(), BilledAmount: v.BilledAmount.String(),
		JournalEntryId: v.JournalEntryID, CancelReason: v.CancelReason, SubmittedAt: v.SubmittedAt,
		ApprovedAt: v.ApprovedAt, PostedAt: v.PostedAt, CreatedByName: v.CreatedByName,
		Lines: make([]PurchaseReturnLine, len(v.Lines)), AllowedActions: []PurchaseReturnAllowedActions{},
	}
	for i, l := range v.Lines {
		ref := v.Receipts[l.ReceiptLineID]
		out.Lines[i] = PurchaseReturnLine{
			Id: l.ID, LineNo: l.LineNo, GoodsReceiptLineId: l.ReceiptLineID, GoodsReceiptNumber: ref.ReceiptNumber,
			Product: product(l.ProductID, v.Products), BatchNumber: ref.Line.BatchNumber, Location: location(l.LocationID, v.Locations),
			StockStatus: PurchaseStockStatus(l.StockStatus), Qty: qty(l.Qty), UnitCost: unitCost(l.UnitCost),
			Amount: l.Amount.String(), UnbilledQty: qty(l.UnbilledQty), UnbilledAmount: l.UnbilledAmount.String(),
			BilledAmount: l.BilledAmount().String(), InventoryAccountCode: l.InventoryAccount, Note: l.Note,
		}
	}
	for _, a := range service.ReturnActions(ctx, v.PurchaseReturn) {
		out.AllowedActions = append(out.AllowedActions, PurchaseReturnAllowedActions(a))
	}
	return out
}

func returnInput(orderID uuid.UUID, f PurchaseReturnFields) (domain.ReturnInput, error) {
	fields := map[string]string{}
	in := domain.ReturnInput{PurchaseOrderID: orderID, BusinessDate: fromDate(f.BusinessDate), Reason: f.Reason, Note: str(f.Note)}
	for i, l := range f.Lines {
		line := domain.ReturnLineInput{
			ReceiptLineID: l.GoodsReceiptLineId, LocationID: l.LocationId,
			Qty: parseQty(l.Qty, fmt.Sprintf("lines.%d.qty", i), fields), Note: str(l.Note),
		}
		if l.StockStatus != nil {
			line.StockStatus = string(*l.StockStatus)
		}
		in.Lines = append(in.Lines, line)
	}
	return in, validation(fields)
}

// ListReturnableReceipts mengembalikan baris GR PO yang masih bisa diretur.
func (h *ProcurementHandler) ListReturnableReceipts(ctx context.Context, req ListReturnableReceiptsRequestObject) (ListReturnableReceiptsResponseObject, error) {
	r, err := h.svc.ReturnableReceipts(ctx, req.Params.PurchaseOrderId)
	if err != nil {
		return nil, err
	}
	out := ReturnableReceipts{
		PurchaseOrderId: r.Order.ID, PurchaseOrderNumber: r.Order.Number, SupplierName: r.Order.SupplierName,
		Lines: make([]ReturnableReceiptLine, len(r.Lines)),
	}
	for i, l := range r.Lines {
		out.Lines[i] = ReturnableReceiptLine{
			GoodsReceiptLineId: l.Line.ID, GoodsReceiptId: l.ReceiptID, GoodsReceiptNumber: l.ReceiptNumber,
			Product: product(l.Line.ProductID, r.Products), BatchNumber: l.Line.BatchNumber,
			Location: location(l.LocationID, r.Locations), GoodQty: qty(l.Line.GoodQty), ReturnedQty: qty(l.Line.ReturnedQty),
			PendingQty: qty(r.PendingQty[l.Line.ID]), ReturnableQty: qty(l.Line.ReturnableQty().Sub(r.PendingQty[l.Line.ID])),
			OpenQty: qty(l.Line.OpenQty()), UnitCost: unitCost(l.Line.UnitCost),
		}
	}
	return ListReturnableReceipts200JSONResponse(out), nil
}

// ListPurchaseReturns mengembalikan daftar retur pembelian.
func (h *ProcurementHandler) ListPurchaseReturns(ctx context.Context, req ListPurchaseReturnsRequestObject) (ListPurchaseReturnsResponseObject, error) {
	p := req.Params
	f := service.ReturnFilter{
		PurchaseOrderID: p.PurchaseOrderId, SupplierID: p.SupplierId, DateFrom: optFromDate(p.DateFrom),
		DateTo: optFromDate(p.DateTo), Search: str(p.Q),
	}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.Origin != nil {
		f.Origin = domain.ReturnOrigin(*p.Origin)
	}
	res, err := h.svc.ListReturns(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]PurchaseReturnSummary, len(res.Items))
	for i, r := range res.Items {
		items[i] = toReturnSummary(r)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListPurchaseReturns200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreatePurchaseReturn menyimpan draft retur manual.
func (h *ProcurementHandler) CreatePurchaseReturn(ctx context.Context, req CreatePurchaseReturnRequestObject) (CreatePurchaseReturnResponseObject, error) {
	b := req.Body
	in, err := returnInput(b.PurchaseOrderId, PurchaseReturnFields{BusinessDate: b.BusinessDate, Reason: b.Reason, Note: b.Note, Lines: b.Lines})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateReturn(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePurchaseReturn201JSONResponse(toReturn(ctx, v)), nil
}

// GetPurchaseReturn mengembalikan detail retur.
func (h *ProcurementHandler) GetPurchaseReturn(ctx context.Context, req GetPurchaseReturnRequestObject) (GetPurchaseReturnResponseObject, error) {
	v, err := h.svc.GetReturn(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// UpdatePurchaseReturn mengubah draft retur.
func (h *ProcurementHandler) UpdatePurchaseReturn(ctx context.Context, req UpdatePurchaseReturnRequestObject) (UpdatePurchaseReturnResponseObject, error) {
	cur, err := h.svc.GetReturn(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in, err := returnInput(cur.PurchaseOrderID, PurchaseReturnFields{BusinessDate: b.BusinessDate, Reason: b.Reason, Note: b.Note, Lines: b.Lines})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateReturn(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// SubmitPurchaseReturn mengajukan retur manual.
func (h *ProcurementHandler) SubmitPurchaseReturn(ctx context.Context, req SubmitPurchaseReturnRequestObject) (SubmitPurchaseReturnResponseObject, error) {
	v, err := h.svc.SubmitReturn(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitPurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// PostPurchaseReturn memposting retur.
func (h *ProcurementHandler) PostPurchaseReturn(ctx context.Context, req PostPurchaseReturnRequestObject) (PostPurchaseReturnResponseObject, error) {
	v, err := h.svc.PostReturn(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostPurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// RevisePurchaseReturn mengembalikan retur yang ditolak ke Draft.
func (h *ProcurementHandler) RevisePurchaseReturn(ctx context.Context, req RevisePurchaseReturnRequestObject) (RevisePurchaseReturnResponseObject, error) {
	v, err := h.svc.ReviseReturn(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return RevisePurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// CancelPurchaseReturn membatalkan retur.
func (h *ProcurementHandler) CancelPurchaseReturn(ctx context.Context, req CancelPurchaseReturnRequestObject) (CancelPurchaseReturnResponseObject, error) {
	v, err := h.svc.CancelReturn(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelPurchaseReturn200JSONResponse(toReturn(ctx, v)), nil
}

// --- Laporan ----------------------------------------------------------------------------------

// GetGoodsReceivedNotInvoiced mengembalikan laporan Barang Diterima Belum Ditagih.
func (h *ProcurementHandler) GetGoodsReceivedNotInvoiced(ctx context.Context, req GetGoodsReceivedNotInvoicedRequestObject) (GetGoodsReceivedNotInvoicedResponseObject, error) {
	p := req.Params
	r, err := h.svc.GRNIReport(ctx, postgres.GRNIFilter{SupplierID: p.SupplierId, PurchaseOrderID: p.PurchaseOrderId, Search: str(p.Q)})
	if err != nil {
		return nil, err
	}
	out := GoodsReceivedNotInvoicedReport{
		AsOf: toDate(r.AsOf), AccountCode: service.AccountGRNI, Lines: make([]GoodsReceivedNotInvoicedLine, len(r.Lines)),
		FilteredTotal: r.FilteredTotal.String(), SubledgerTotal: r.SubledgerTotal.String(),
		LedgerBalance: r.LedgerBalance.String(), Difference: r.Difference().String(),
	}
	for i, l := range r.Lines {
		out.Lines[i] = GoodsReceivedNotInvoicedLine{
			GoodsReceiptLineId: l.LineID, GoodsReceiptId: l.ReceiptID, GoodsReceiptNumber: l.ReceiptNumber,
			BusinessDate: toDate(l.BusinessDate), PurchaseOrderId: l.PurchaseOrderID, PurchaseOrderNumber: l.OrderNumber,
			OrderType: PurchaseOrderType(l.OrderType), SupplierId: l.SupplierID, SupplierName: l.SupplierName,
			Product: product(l.ProductID, r.Products), UnitPrice: l.UnitPrice.String(), ReceivedQty: qty(l.ReceivedQty),
			ReceivedAmount: l.ReceivedAmount.String(), BilledQty: qty(l.BilledQty), BilledAmount: l.BilledAmount.String(),
			ReturnedQty: qty(l.ReturnedQty), ReturnedAmount: l.ReturnedAmount.String(), OpenQty: qty(l.OpenQty()),
			OpenAmount: l.OpenAmount().String(),
		}
	}
	return GetGoodsReceivedNotInvoiced200JSONResponse(out), nil
}
