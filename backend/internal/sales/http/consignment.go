package http

import (
	"context"
	"fmt"

	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/service"
)

// --- Pengiriman konsinyasi (S-303) -----------------------------------------------------------

func deliveryLinesIn(lines []ConsignmentDeliveryLineInput, fields map[string]string) []domain.DeliveryLineInput {
	out := make([]domain.DeliveryLineInput, len(lines))
	for i, l := range lines {
		out[i] = domain.DeliveryLineInput{
			ProductID: l.ProductId, Qty: parseQty(l.Qty, fmt.Sprintf("lines.%d.qty", i), fields), Note: str(l.Note),
		}
	}
	return out
}

func toDeliverySummary(d domain.ConsignmentDelivery) ConsignmentDeliverySummary {
	return ConsignmentDeliverySummary{
		Id: d.ID, Number: optStr(d.Number), DeliveryType: ConsignmentDeliveryType(d.Type), PartnerId: d.PartnerID,
		PartnerCode: d.PartnerCode, PartnerName: d.PartnerName, BusinessDate: toDate(d.BusinessDate),
		Status: ConsignmentDeliveryStatus(d.Status), TransferNumber: d.TransferNumber, CostAmount: d.Cost.String(),
		Version: d.Version,
	}
}

func toDelivery(v service.DeliveryView) ConsignmentDelivery {
	s := toDeliverySummary(v.ConsignmentDelivery)
	lines := make([]ConsignmentDeliveryLine, len(v.Lines))
	for i, l := range v.Lines {
		lines[i] = ConsignmentDeliveryLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products), BatchId: l.BatchID,
			Qty: qty(l.Qty), CostAmount: l.Cost.String(), Note: l.Note,
		}
	}
	from, to := v.FromLocation, v.ToLocation
	return ConsignmentDelivery{
		Id: s.Id, Number: s.Number, DeliveryType: s.DeliveryType, PartnerId: s.PartnerId, PartnerCode: s.PartnerCode,
		PartnerName: s.PartnerName, BusinessDate: s.BusinessDate, Status: s.Status, TransferNumber: s.TransferNumber,
		CostAmount: s.CostAmount, Version: s.Version, AgreementNumber: v.AgreementNumber,
		FromLocation: locationRef(v.FromLocationID, &from), ToLocation: locationRef(v.ToLocationID, &to),
		ChannelName: v.ChannelName, SubChannelName: v.SubChannelName, SourceType: v.SourceType, TransferId: v.TransferID,
		Note: v.Note, CancelReason: v.CancelReason, ShippedAt: v.ShippedAt, ReceivedAt: v.ReceivedAt,
		CreatedAt: v.CreatedAt, CreatedByName: v.CreatedByName, UpdatedAt: v.UpdatedAt, Lines: lines,
		AllowedActions: v.AllowedActions,
	}
}

func bodyRequired[T any](b *T) (*T, error) {
	if b == nil {
		return nil, apperr.New(apperr.CodeBadRequest)
	}
	return b, nil
}

// ListConsignmentDeliveries mengembalikan daftar pengiriman konsinyasi.
func (h *SalesHandler) ListConsignmentDeliveries(ctx context.Context, req ListConsignmentDeliveriesRequestObject) (ListConsignmentDeliveriesResponseObject, error) {
	p := req.Params
	f := service.DeliveryFilter{PartnerID: p.PartnerId, Search: str(p.Q)}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	if p.DeliveryType != nil {
		f.Type = domain.DeliveryType(*p.DeliveryType)
	}
	res, err := h.docs.ListDeliveries(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]ConsignmentDeliverySummary, len(res.Items))
	for i, d := range res.Items {
		items[i] = toDeliverySummary(d)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListConsignmentDeliveries200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateConsignmentDelivery membuat draft pengiriman.
func (h *SalesHandler) CreateConsignmentDelivery(ctx context.Context, req CreateConsignmentDeliveryRequestObject) (CreateConsignmentDeliveryResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	in := domain.DeliveryInput{
		Type: domain.DeliveryType(b.DeliveryType), PartnerID: b.PartnerId, FromLocationID: b.FromLocationId,
		BusinessDate: fromDate(b.BusinessDate), Note: str(b.Note), Lines: deliveryLinesIn(b.Lines, fields),
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	v, err := h.docs.CreateDelivery(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateConsignmentDelivery201JSONResponse(toDelivery(v)), nil
}

// GetConsignmentDelivery mengembalikan detail pengiriman.
func (h *SalesHandler) GetConsignmentDelivery(ctx context.Context, req GetConsignmentDeliveryRequestObject) (GetConsignmentDeliveryResponseObject, error) {
	v, err := h.docs.GetDelivery(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetConsignmentDelivery200JSONResponse(toDelivery(v)), nil
}

// UpdateConsignmentDelivery mengubah draft pengiriman.
func (h *SalesHandler) UpdateConsignmentDelivery(ctx context.Context, req UpdateConsignmentDeliveryRequestObject) (UpdateConsignmentDeliveryResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	in := domain.DeliveryInput{
		FromLocationID: b.FromLocationId, BusinessDate: fromDate(b.BusinessDate), Note: str(b.Note),
		Lines: deliveryLinesIn(b.Lines, fields),
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	v, err := h.docs.UpdateDelivery(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateConsignmentDelivery200JSONResponse(toDelivery(v)), nil
}

// ShipConsignmentDelivery mengirim barang titipan.
func (h *SalesHandler) ShipConsignmentDelivery(ctx context.Context, req ShipConsignmentDeliveryRequestObject) (ShipConsignmentDeliveryResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.ShipDelivery(ctx, req.Id, b.Version)
	if err != nil {
		return nil, err
	}
	return ShipConsignmentDelivery200JSONResponse(toDelivery(v)), nil
}

// ReceiveConsignmentDelivery mencatat barang diterima mitra.
func (h *SalesHandler) ReceiveConsignmentDelivery(ctx context.Context, req ReceiveConsignmentDeliveryRequestObject) (ReceiveConsignmentDeliveryResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.ReceiveDelivery(ctx, req.Id, b.Version)
	if err != nil {
		return nil, err
	}
	return ReceiveConsignmentDelivery200JSONResponse(toDelivery(v)), nil
}

// CancelConsignmentDelivery membatalkan draft pengiriman.
func (h *SalesHandler) CancelConsignmentDelivery(ctx context.Context, req CancelConsignmentDeliveryRequestObject) (CancelConsignmentDeliveryResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.CancelDelivery(ctx, req.Id, b.Version, str(b.Reason))
	if err != nil {
		return nil, err
	}
	return CancelConsignmentDelivery200JSONResponse(toDelivery(v)), nil
}

// --- Rekonsiliasi konsinyasi (S-304, S-305) --------------------------------------------------

func toReconciliationSummary(r domain.Reconciliation) ConsignmentReconciliationSummary {
	out := ConsignmentReconciliationSummary{
		Id: r.ID, Number: optStr(r.Number), PartnerId: r.PartnerID, PartnerCode: r.PartnerCode, PartnerName: r.PartnerName,
		CutOffDate: toDate(r.CutOffDate), StockCountNumber: r.StockCountNumber,
		Status: ConsignmentReconciliationStatus(r.Status), SalesAmount: r.SalesAmount.String(),
		BilledVarianceAmount: r.BilledVariance.String(), Version: r.Version,
	}
	if r.PeriodStart != nil {
		d := toDate(*r.PeriodStart)
		out.PeriodStart = &d
	}
	return out
}

func toReconciliation(v service.ReconciliationView) ConsignmentReconciliation {
	s := toReconciliationSummary(v.Reconciliation)
	lines := make([]ConsignmentReconciliationLine, len(v.Lines))
	for i, l := range v.Lines {
		line := ConsignmentReconciliationLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products), OpeningQty: qty(l.OpeningQty),
			DeliveredQty: qty(l.DeliveredQty), ReturnedQty: qty(l.ReturnedQty), CountedQty: qty(l.CountedQty),
			SoldQty: qty(l.SoldQty), ReportedQty: qty(l.ReportedQty), VarianceQty: qty(l.VarianceQty),
			UnitPrice: l.UnitPrice.String(), PriceRule: l.PriceRule, PriceDescription: l.PriceDescription,
			SalesAmount: l.SalesAmount.String(), VarianceAmount: l.VarianceAmount.String(), CostAmount: l.Cost.String(),
			VarianceCostAmount: l.VarianceCost.String(), Note: l.Note,
		}
		if l.Cause != "" {
			c := ConsignmentVarianceCause(l.Cause)
			line.VarianceCause = &c
		}
		if l.BorneBy != "" {
			b := ConsignmentBorneBy(l.BorneBy)
			line.BorneBy = &b
		}
		lines[i] = line
	}
	rules := v.Rules.Normalize()
	var margin *string
	if v.MarginPercent != nil {
		m := v.MarginPercent.String()
		margin = &m
	}
	loc := v.Location
	return ConsignmentReconciliation{
		Id: s.Id, Number: s.Number, PartnerId: s.PartnerId, PartnerCode: s.PartnerCode, PartnerName: s.PartnerName,
		PeriodStart: s.PeriodStart, CutOffDate: s.CutOffDate, StockCountNumber: s.StockCountNumber, Status: s.Status,
		SalesAmount: s.SalesAmount, BilledVarianceAmount: s.BilledVarianceAmount, Version: s.Version,
		PartnerAddress: v.PartnerAddress, AgreementNumber: v.AgreementNumber, PaymentTermDays: v.PaymentTermDays,
		MarginPercent: margin,
		VarianceRules: ConsignmentVarianceRules{
			Lost: ConsignmentBorneBy(rules.Lost), PartnerDamage: ConsignmentBorneBy(rules.PartnerDamage),
			ProductDefect: ConsignmentBorneBy(rules.ProductDefect), NearExpiry: ConsignmentBorneBy(rules.NearExpiry),
		},
		Location: locationRef(v.LocationID, &loc), ChannelName: v.ChannelName, SubChannelName: v.SubChannelName,
		StockCountId: v.StockCountID, PartnerReportFileId: v.PartnerReportFileID, PartnerReportFileName: v.PartnerReportFileName,
		BaFileId: v.BAFileID, BaFileName: v.BAFileName, Note: v.Note, AbsorbedVarianceCost: v.AbsorbedCost.String(),
		CostAmount: v.Cost.String(), InvoiceId: v.InvoiceID, InvoiceNumber: v.InvoiceNumber,
		VarianceInvoiceId: v.VarianceInvoiceID, VarianceInvoiceNumber: v.VarianceInvoiceNumber,
		SubmittedAt: v.SubmittedAt, DecidedAt: v.DecidedAt, DecisionNote: v.DecisionNote, PostedAt: v.PostedAt,
		CancelReason: v.CancelReason, CreatedAt: v.CreatedAt, CreatedByName: v.CreatedByName, UpdatedAt: v.UpdatedAt,
		Lines: lines, AllowedActions: v.AllowedActions,
	}
}

// ListConsignmentReconciliations mengembalikan daftar rekonsiliasi.
func (h *SalesHandler) ListConsignmentReconciliations(ctx context.Context, req ListConsignmentReconciliationsRequestObject) (ListConsignmentReconciliationsResponseObject, error) {
	p := req.Params
	f := service.ReconciliationFilter{PartnerID: p.PartnerId, Search: str(p.Q)}
	if p.Status != nil {
		f.Status = domain.Status(*p.Status)
	}
	res, err := h.docs.ListReconciliations(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]ConsignmentReconciliationSummary, len(res.Items))
	for i, r := range res.Items {
		items[i] = toReconciliationSummary(r)
	}
	page, size, total, pages := pageParams(res.Page, res.Total)
	return ListConsignmentReconciliations200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateConsignmentReconciliation membuat draft rekonsiliasi.
func (h *SalesHandler) CreateConsignmentReconciliation(ctx context.Context, req CreateConsignmentReconciliationRequestObject) (CreateConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.CreateReconciliation(ctx, domain.ReconciliationInput{
		PartnerID: b.PartnerId, CutOffDate: fromDate(b.CutOffDate), StockCountID: b.StockCountId, Note: str(b.Note),
	})
	if err != nil {
		return nil, err
	}
	return CreateConsignmentReconciliation201JSONResponse(toReconciliation(v)), nil
}

// GetConsignmentReconciliation mengembalikan detail rekonsiliasi.
func (h *SalesHandler) GetConsignmentReconciliation(ctx context.Context, req GetConsignmentReconciliationRequestObject) (GetConsignmentReconciliationResponseObject, error) {
	v, err := h.docs.GetReconciliation(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}

// UpdateConsignmentReconciliationDraft mengubah draft rekonsiliasi.
func (h *SalesHandler) UpdateConsignmentReconciliationDraft(ctx context.Context, req UpdateConsignmentReconciliationDraftRequestObject) (UpdateConsignmentReconciliationDraftResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.UpdateReconciliationDraft(ctx, req.Id, b.Version, domain.ReconciliationInput{
		CutOffDate: fromDate(b.CutOffDate), StockCountID: b.StockCountId, Note: str(b.Note),
	})
	if err != nil {
		return nil, err
	}
	return UpdateConsignmentReconciliationDraft200JSONResponse(toReconciliation(v)), nil
}

// StartConsignmentReconciliation memulai rekonsiliasi.
func (h *SalesHandler) StartConsignmentReconciliation(ctx context.Context, req StartConsignmentReconciliationRequestObject) (StartConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.StartReconciliation(ctx, req.Id, b.Version)
	if err != nil {
		return nil, err
	}
	return StartConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}

// UpdateConsignmentReconciliation mencatat laporan mitra, penyebab selisih, file, dan catatan.
func (h *SalesHandler) UpdateConsignmentReconciliation(ctx context.Context, req UpdateConsignmentReconciliationRequestObject) (UpdateConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	u := service.ReconciliationUpdate{
		SetFiles: b.SetFiles != nil && *b.SetFiles, PartnerReportFileID: b.PartnerReportFileId, BAFileID: b.BaFileId,
		SetNote: b.Note != nil, Note: str(b.Note),
	}
	if b.Lines != nil {
		for i, l := range *b.Lines {
			e := domain.LineEntry{
				LineID: l.LineId, ReportedQty: parseQty(l.ReportedQty, fmt.Sprintf("lines.%d.reportedQty", i), fields), Note: str(l.Note),
			}
			if l.VarianceCause != nil {
				e.Cause = domain.VarianceCause(*l.VarianceCause)
			}
			u.Lines = append(u.Lines, e)
		}
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	v, err := h.docs.UpdateReconciliation(ctx, req.Id, b.Version, u)
	if err != nil {
		return nil, err
	}
	return UpdateConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}

// SubmitConsignmentReconciliation mengajukan rekonsiliasi ke approval.
func (h *SalesHandler) SubmitConsignmentReconciliation(ctx context.Context, req SubmitConsignmentReconciliationRequestObject) (SubmitConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.SubmitReconciliation(ctx, req.Id, b.Version)
	if err != nil {
		return nil, err
	}
	return SubmitConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}

// ReviseConsignmentReconciliation membuka kembali rekonsiliasi yang ditolak.
func (h *SalesHandler) ReviseConsignmentReconciliation(ctx context.Context, req ReviseConsignmentReconciliationRequestObject) (ReviseConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.ReviseReconciliation(ctx, req.Id, b.Version)
	if err != nil {
		return nil, err
	}
	return ReviseConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}

// CancelConsignmentReconciliation membatalkan rekonsiliasi.
func (h *SalesHandler) CancelConsignmentReconciliation(ctx context.Context, req CancelConsignmentReconciliationRequestObject) (CancelConsignmentReconciliationResponseObject, error) {
	b, err := bodyRequired(req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.docs.CancelReconciliation(ctx, req.Id, b.Version, str(b.Reason))
	if err != nil {
		return nil, err
	}
	return CancelConsignmentReconciliation200JSONResponse(toReconciliation(v)), nil
}
