package http

import (
	"context"

	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
	"balans/internal/sales/service"
)

// Laporan penjualan & margin (RPT-05, RPT-15), konsinyasi (RPT-06), dan settlement marketplace (S-403).

func toSalesReportLine(l service.SalesReportLine) SalesReportLine {
	return SalesReportLine{
		Key: l.Key, Code: l.Code, Name: l.Name, Qty: l.Qty.Decimal().String(), Gross: l.Gross.String(),
		Discount: l.Discount.String(), Net: l.Net.String(), Cogs: l.COGS.String(), GrossProfit: l.GrossProfit.String(),
		MarginPercent: l.MarginPct.StringFixed(2),
	}
}

// GetSalesReport mengembalikan laporan penjualan & margin.
func (h *SalesHandler) GetSalesReport(ctx context.Context, req GetSalesReportRequestObject) (GetSalesReportResponseObject, error) {
	p := req.Params
	f := service.SalesReportFilter{
		DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo), ChannelID: p.ChannelId, SubChannelID: p.SubChannelId,
		ProductID: p.ProductId,
	}
	if p.GroupBy != nil {
		f.GroupBy = string(*p.GroupBy)
	}
	if p.OrderType != nil {
		f.OrderType = *p.OrderType
	}
	rep, err := h.docs.SalesReport(ctx, f)
	if err != nil {
		return nil, err
	}
	lines := make([]SalesReportLine, len(rep.Lines))
	for i, l := range rep.Lines {
		lines[i] = toSalesReportLine(l)
	}
	return GetSalesReport200JSONResponse{
		GroupBy: ReportGroupBy(rep.Filter.GroupBy), DateFrom: p.DateFrom, DateTo: p.DateTo, Lines: lines,
		Total: toSalesReportLine(rep.Total),
	}, nil
}

// GetConsignmentReport mengembalikan invoice konsinyasi per mitra.
func (h *SalesHandler) GetConsignmentReport(ctx context.Context, req GetConsignmentReportRequestObject) (GetConsignmentReportResponseObject, error) {
	rows, err := h.docs.ConsignmentReport(ctx, fromDate(req.Params.DateFrom), fromDate(req.Params.DateTo))
	if err != nil {
		return nil, err
	}
	items := make([]ConsignmentReportRow, len(rows))
	for i, r := range rows {
		items[i] = ConsignmentReportRow{
			PartnerId: r.PartyID, PartnerCode: r.PartyCode, PartnerName: r.PartyName,
			InvoiceType: ConsignmentReportRowInvoiceType(r.InvoiceType), InvoiceCount: int(r.InvoiceCount),
			Gross: r.Gross.String(), Discount: r.Discount.String(), Total: r.Total.String(), Paid: r.Paid.String(),
			Outstanding: r.Total.Sub(r.Paid).String(),
		}
	}
	return GetConsignmentReport200JSONResponse{DateFrom: req.Params.DateFrom, DateTo: req.Params.DateTo, Items: items}, nil
}

// ListSettlementFormats mengembalikan format laporan settlement per marketplace.
func (h *SalesHandler) ListSettlementFormats(context.Context, ListSettlementFormatsRequestObject) (ListSettlementFormatsResponseObject, error) {
	items := make([]SettlementFormat, len(service.SettlementFormats))
	for i, f := range service.SettlementFormats {
		cols := make([]SettlementFormatColumn, len(f.Columns))
		for j, c := range f.Columns {
			cols[j] = SettlementFormatColumn{Key: c.Key, Header: c.Header, Required: c.Required}
		}
		items[i] = SettlementFormat{Marketplace: f.Marketplace, Label: f.Label, Kind: f.Kind(), Columns: cols}
	}
	return ListSettlementFormats200JSONResponse{Items: items}, nil
}

func toSettlementSummary(st domain.Settlement, name string) MarketplaceSettlementSummary {
	out := MarketplaceSettlementSummary{
		Id: st.ID, MarketplaceCode: st.MarketplaceCode, MarketplaceName: name, PayoutReference: st.PayoutReference,
		SettlementDate: toDate(st.SettlementDate), BankAccountCode: st.BankAccountCode, Status: SettlementStatus(st.Status),
		StatusLabel: st.Status.Label(), LineCount: st.LineCount, Settled: st.Settled.String(), Fee: st.Fee.String(),
		Promo: st.Promo.String(), Net: st.Net.String(), JournalEntryId: st.JournalEntryID, PostedAt: st.PostedAt,
		Version: st.Version, UpdatedAt: st.UpdatedAt,
	}
	if st.CancelReason != "" {
		r := st.CancelReason
		out.CancelReason = &r
	}
	return out
}

func (h *SalesHandler) toSettlement(ctx context.Context, v service.SettlementView) MarketplaceSettlement {
	s := toSettlementSummary(v.Settlement, v.MarketplaceName)
	lines := make([]MarketplaceSettlementLine, len(v.Lines))
	for i, l := range v.Lines {
		lines[i] = MarketplaceSettlementLine{
			LineNo: l.LineNo, SalesOrderId: l.OrderID, ExternalOrderId: l.ExternalOrderID, OrderNumber: l.OrderNumber,
			Receivable: l.Receivable.String(), Fee: l.Fee.String(), Promo: l.Promo.String(), Net: l.Net.String(),
			Settled: l.Settled.String(), Outstanding: l.Outstanding().String(),
		}
	}
	return MarketplaceSettlement{
		Id: s.Id, MarketplaceCode: s.MarketplaceCode, MarketplaceName: s.MarketplaceName, PayoutReference: s.PayoutReference,
		SettlementDate: s.SettlementDate, BankAccountCode: s.BankAccountCode, Status: s.Status, StatusLabel: s.StatusLabel,
		LineCount: s.LineCount, Settled: s.Settled, Fee: s.Fee, Promo: s.Promo, Net: s.Net, JournalEntryId: s.JournalEntryId,
		PostedAt: s.PostedAt, CancelReason: s.CancelReason, Version: s.Version, UpdatedAt: s.UpdatedAt, Lines: lines,
		AllowedActions: h.docs.SettlementActions(ctx, v.Settlement),
	}
}

// ListMarketplaceSettlements mengembalikan daftar settlement.
func (h *SalesHandler) ListMarketplaceSettlements(ctx context.Context, req ListMarketplaceSettlementsRequestObject) (ListMarketplaceSettlementsResponseObject, error) {
	p := req.Params
	f := service.SettlementFilter{SubChannelID: p.SubChannelId}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, names, err := h.docs.ListSettlements(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]MarketplaceSettlementSummary, len(res.Items))
	for i, st := range res.Items {
		items[i] = toSettlementSummary(st, names[st.SubChannelID])
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListMarketplaceSettlements200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// GetMarketplaceSettlement mengembalikan detail settlement.
func (h *SalesHandler) GetMarketplaceSettlement(ctx context.Context, req GetMarketplaceSettlementRequestObject) (GetMarketplaceSettlementResponseObject, error) {
	v, err := h.docs.GetSettlement(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetMarketplaceSettlement200JSONResponse(h.toSettlement(ctx, v)), nil
}

// PostMarketplaceSettlement memposting jurnal A-10s.
func (h *SalesHandler) PostMarketplaceSettlement(ctx context.Context, req PostMarketplaceSettlementRequestObject) (PostMarketplaceSettlementResponseObject, error) {
	v, err := h.docs.PostSettlement(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostMarketplaceSettlement200JSONResponse(h.toSettlement(ctx, v)), nil
}

// CancelMarketplaceSettlement membatalkan settlement draft.
func (h *SalesHandler) CancelMarketplaceSettlement(ctx context.Context, req CancelMarketplaceSettlementRequestObject) (CancelMarketplaceSettlementResponseObject, error) {
	v, err := h.docs.CancelSettlement(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return CancelMarketplaceSettlement200JSONResponse(h.toSettlement(ctx, v)), nil
}

// ListMarketplaceReceivables mengembalikan sisa piutang marketplace per order.
func (h *SalesHandler) ListMarketplaceReceivables(ctx context.Context, req ListMarketplaceReceivablesRequestObject) (ListMarketplaceReceivablesResponseObject, error) {
	p := req.Params
	f := postgres.MarketplaceReceivableFilter{SubChannelID: p.SubChannelId, OpenOnly: p.OpenOnly == nil || *p.OpenOnly}
	if p.Q != nil {
		f.Search = *p.Q
	}
	pg := paging.New(p.Page, p.PageSize)
	rows, totals, names, err := h.docs.MarketplaceReceivables(ctx, f, pg)
	if err != nil {
		return nil, err
	}
	items := make([]MarketplaceReceivable, len(rows))
	for i, r := range rows {
		items[i] = MarketplaceReceivable{
			SalesOrderId: r.OrderID, Number: r.Number, ExternalOrderId: r.ExternalOrderID, MarketplaceName: names[r.SubChannelID],
			OrderDate: toDate(r.OrderDate), HandedOverDate: toDate(r.HandedOverDate), BuyerName: r.BuyerName,
			Receivable: r.Receivable.String(), Settled: r.Settled.String(), Draft: r.Draft.String(), Outstanding: r.Outstanding.String(),
		}
	}
	page, size, total, pages := pageParams(pg, totals.Rows)
	return ListMarketplaceReceivables200JSONResponse{
		Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages,
		TotalReceivable: totals.Receivable.String(), TotalSettled: totals.Settled.String(), TotalOutstanding: totals.Outstanding.String(),
	}, nil
}
