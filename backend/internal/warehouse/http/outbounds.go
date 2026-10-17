package http

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/service"
)

// --- Outbound (WMS-12) ------------------------------------------------------------------------

func toOutboundSummary(o domain.Outbound, lineCount, pending, held int, refs service.Refs) OutboundSummary {
	return OutboundSummary{
		Id: o.ID, Number: o.Number, SourceType: o.SourceType, SourceTypeLabel: domain.OutboundSourceLabel(o.SourceType),
		SourceId: o.SourceID, SourceLabel: o.SourceLabel, Location: locationRef(refs, o.LocationID),
		BusinessDate: toDate(o.BusinessDate), RecipientName: o.RecipientName, Status: OutboundStatus(o.Status),
		LineCount: lineCount, PendingCount: pending, HeldCount: held, CreatedAt: o.CreatedAt,
	}
}

func qualityChecks(list []domain.QualityCheck) []InboundQualityCheck {
	out := make([]InboundQualityCheck, len(list))
	for i, c := range list {
		photos := c.PhotoFileIDs
		if photos == nil {
			photos = []uuid.UUID{}
		}
		out[i] = InboundQualityCheck{
			Id: c.ID, LineId: c.DocumentLineID, Result: InboundQualityCheckResult(c.Result), Qty: qty(c.Qty),
			Checks: InboundQcChecklist{Batch: c.Checks.Batch, Expiry: c.Checks.Expiry, Condition: c.Checks.Condition, Seal: c.Checks.Seal},
			Note:   c.Note, PhotoFileIds: photos, CheckedByName: c.CheckedByName, CheckedAt: c.CheckedAt,
		}
	}
	return out
}

func toOutbound(ctx context.Context, d service.OutboundDetail) Outbound {
	o := d.Outbound
	pending, held := 0, 0
	lines := make([]OutboundLine, len(o.Lines))
	for i, l := range o.Lines {
		switch l.Status {
		case domain.OutboundPending, domain.OutboundPicked:
			pending++
		case domain.OutboundHeld:
			held++
		case domain.OutboundPassed, domain.OutboundRejected, domain.OutboundDropped:
		}
		acts := []OutboundLineAction{}
		for _, a := range service.OutboundLineActions(ctx, o, l) {
			acts = append(acts, OutboundLineAction(a))
		}
		lines[i] = OutboundLine{
			Id: l.ID, LineNo: l.LineNo, SourceLineId: l.SourceLineID, Product: productRef(d.Refs, l.ProductID),
			Batch: batchRef(d.Refs, l.BatchID), AllocatedQty: qty(l.AllocatedQty), PickedQty: qty(l.PickedQty),
			PassedQty: qty(l.PassedQty), RejectedQty: qty(l.RejectedQty), Status: OutboundLineStatus(l.Status),
			ReplacementOfLineId: l.ReplacementOfLineID, RejectReason: l.RejectReason, ReturnLabel: l.ReturnLabel,
			CheckedByName: optStr(l.CheckedByName), CheckedAt: l.CheckedAt, AllowedActions: acts,
		}
	}
	pkgs := make([]OutboundPackage, len(o.Packages))
	for i, p := range o.Packages {
		items := make([]OutboundPackageItem, len(p.Contents))
		for j, it := range p.Contents {
			q, err := decimal.NewFromString(it.Qty)
			if err != nil {
				q = decimal.Zero
			}
			items[j] = OutboundPackageItem{LineId: it.LineID, Product: productRef(d.Refs, it.ProductID), Batch: batchRef(d.Refs, it.BatchID), Qty: qty(q)}
		}
		pkgs[i] = OutboundPackage{PackageNumber: p.Number, PackageCount: p.Count, WeightGrams: p.WeightGrams, Items: items}
	}
	acts := []OutboundAction{}
	for _, a := range service.OutboundActions(ctx, o) {
		acts = append(acts, OutboundAction(a))
	}
	s := toOutboundSummary(o, len(o.Lines), pending, held, d.Refs)
	return Outbound{
		Id: s.Id, Number: s.Number, SourceType: s.SourceType, SourceTypeLabel: s.SourceTypeLabel, SourceId: s.SourceId,
		SourceLabel: s.SourceLabel, Location: s.Location, BusinessDate: s.BusinessDate, RecipientName: s.RecipientName,
		Status: s.Status, LineCount: s.LineCount, PendingCount: s.PendingCount, HeldCount: s.HeldCount, CreatedAt: s.CreatedAt,
		RecipientPhone: o.RecipientPhone, RecipientAddress: o.RecipientAddress, Note: o.Note, PreparedById: o.PreparedBy,
		PreparedByName: o.PreparedByName, PackedAt: o.PackedAt, CheckedByName: o.CheckedByName, CompletedAt: o.CompletedAt,
		CancelReason: optStr(o.CancelReason), NeedsRepack: o.NeedsRepack(), Version: o.Version, Lines: lines, Packages: pkgs,
		Checks: qualityChecks(d.Checks), AllowedActions: acts,
	}
}

// ListOutbounds mengembalikan daftar Outbound.
func (h *WarehouseHandler) ListOutbounds(ctx context.Context, req ListOutboundsRequestObject) (ListOutboundsResponseObject, error) {
	p := req.Params
	f := service.OutboundFilter{SourceType: strOr(p.SourceType), Search: strOr(p.Q)}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, refs, err := h.svc.ListOutbounds(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]OutboundSummary, len(res.Items))
	for i, it := range res.Items {
		items[i] = toOutboundSummary(it.Outbound, it.LineCount, it.PendingCount, it.HeldCount, refs)
	}
	out := ListOutbounds200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// GetOutbound mengembalikan detail Outbound.
func (h *WarehouseHandler) GetOutbound(ctx context.Context, req GetOutboundRequestObject) (GetOutboundResponseObject, error) {
	d, err := h.svc.GetOutbound(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetOutbound200JSONResponse(toOutbound(ctx, d)), nil
}

// PickOutbound mencatat picking.
func (h *WarehouseHandler) PickOutbound(ctx context.Context, req PickOutboundRequestObject) (PickOutboundResponseObject, error) {
	d, err := h.svc.PickOutbound(ctx, req.Id, req.Body.Version, ids(req.Body.LineIds))
	if err != nil {
		return nil, err
	}
	return PickOutbound200JSONResponse(toOutbound(ctx, d)), nil
}

// PackOutbound mencatat koli.
func (h *WarehouseHandler) PackOutbound(ctx context.Context, req PackOutboundRequestObject) (PackOutboundResponseObject, error) {
	fields := map[string]string{}
	pkgs := make([]domain.PackageInput, len(req.Body.Packages))
	for i, p := range req.Body.Packages {
		pkgs[i] = domain.PackageInput{WeightGrams: p.WeightGrams}
		for j, it := range p.Items {
			pkgs[i].Items = append(pkgs[i].Items, domain.PackageItemInput{
				LineID: it.LineId, Qty: parseQty(it.Qty, "packages["+strconv.Itoa(i)+"].items["+strconv.Itoa(j)+"].qty", fields),
			})
		}
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	d, err := h.svc.PackOutbound(ctx, req.Id, req.Body.Version, pkgs)
	if err != nil {
		return nil, err
	}
	return PackOutbound200JSONResponse(toOutbound(ctx, d)), nil
}

// RecordOutboundQc mencatat QC satu baris Outbound.
func (h *WarehouseHandler) RecordOutboundQc(ctx context.Context, req RecordOutboundQcRequestObject) (RecordOutboundQcResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	in := domain.OutboundQCInput{
		PassedQty: parseQty(b.PassedQty, "passedQty", fields), RejectedQty: parseQty(b.RejectedQty, "rejectedQty", fields),
		Checks:       domain.Checklist{Batch: b.Checks.Batch, Expiry: b.Checks.Expiry, Condition: b.Checks.Condition, Seal: b.Checks.Seal},
		RejectReason: strOr(b.RejectReason), Note: strOr(b.Note), PhotoFileIDs: ids(b.PhotoFileIds),
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	d, err := h.svc.RecordOutboundQC(ctx, req.Id, req.LineId, in)
	if err != nil {
		return nil, err
	}
	return RecordOutboundQc200JSONResponse(toOutbound(ctx, d)), nil
}

// ReallocateOutboundLine mengalokasikan ulang baris ditahan.
func (h *WarehouseHandler) ReallocateOutboundLine(ctx context.Context, req ReallocateOutboundLineRequestObject) (ReallocateOutboundLineResponseObject, error) {
	d, err := h.svc.ReallocateOutboundLine(ctx, req.Id, req.LineId)
	if err != nil {
		return nil, err
	}
	return ReallocateOutboundLine200JSONResponse(toOutbound(ctx, d)), nil
}

// DropOutboundLine mencatat keputusan kirim sebagian.
func (h *WarehouseHandler) DropOutboundLine(ctx context.Context, req DropOutboundLineRequestObject) (DropOutboundLineResponseObject, error) {
	d, err := h.svc.DropOutboundLine(ctx, req.Id, req.LineId, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return DropOutboundLine200JSONResponse(toOutbound(ctx, d)), nil
}
