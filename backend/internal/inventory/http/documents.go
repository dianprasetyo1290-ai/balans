package http

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// --- Konversi umum ----------------------------------------------------------------------------

func parseQty(s StockQuantity, field string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, apperr.Validation(map[string]string{field: "Qty tidak valid"})
	}
	return d, nil
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func optDatePtr(d *BusinessDate) *clock.BusinessDate {
	if d == nil {
		return nil
	}
	v := fromDate(*d)
	return &v
}

func numberPtr(s string) *string { return optStr(s) }

func actorRef(a domain.Actor) InventoryActorRef { return InventoryActorRef{UserId: a.ID, Name: a.Name} }

func step(s *domain.Step) *InventoryDocStep {
	if s == nil {
		return nil
	}
	return &InventoryDocStep{UserId: s.By, Name: s.Name, At: s.At}
}

func batchRef(b *service.BatchRef) *InventoryBatchRef {
	if b == nil {
		return nil
	}
	return &InventoryBatchRef{Id: b.ID, Number: b.Number, ExpiryDate: optDate(b.ExpiryDate)}
}

func dimRef(d *service.DimensionOption) *InventoryDimensionRef {
	if d == nil {
		return nil
	}
	return &InventoryDimensionRef{Id: d.ID, Code: d.Code, Name: d.Name}
}

func actions(as []domain.DocAction) []InventoryDocAction {
	out := make([]InventoryDocAction, len(as))
	for i, a := range as {
		out[i] = InventoryDocAction(a)
	}
	return out
}

func movements(ms []postgres.SourceMovement, refs service.DocRefs) []InventoryDocMovement {
	out := make([]InventoryDocMovement, len(ms))
	for i, m := range ms {
		out[i] = InventoryDocMovement{
			Id: m.ID, SourceLineId: m.SourceLineID, MovementType: StockMovementType(m.Type), MovementTypeLabel: m.Type.Label(),
			Product: productRef(refs.Product(m.ProductID)), Location: locationRef(refs.Location(m.LocationID)),
			Batch: batchRef(refs.Batch(m.BatchID)), Status: StockStatus(m.Status), StatusLabel: m.Status.Label(),
			Qty: qty(m.Qty), CostAmount: amount(m.CostAmount),
		}
	}
	return out
}

func docFilter(status *InventoryDocStatus, kind *string, q *Search) postgres.DocFilter {
	f := postgres.DocFilter{Kind: str(kind)}
	if status != nil {
		f.Status = string(*status)
	}
	if q != nil {
		f.Query = *q
	}
	return f
}

func stockStatus(s *InventoryDocStockStatus) domain.StockStatus {
	if s == nil {
		return ""
	}
	return domain.StockStatus(*s)
}

func versionOf(b *VersionInput) int {
	if b == nil {
		return 0
	}
	return b.Version
}

// --- Pemakaian Internal -----------------------------------------------------------------------

func toInternalUsage(v service.UsageView) InternalUsage {
	u := v.Usage
	lines := make([]InternalUsageLine, len(u.Lines))
	for i, l := range u.Lines {
		lines[i] = InternalUsageLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(v.Refs.Product(l.ProductID)), Qty: qty(l.Qty),
			ReturnedQty: qty(l.ReturnedQty), ConsumedQty: qty(l.ConsumedQty), CostAmount: amount(l.CostAmount), Note: l.Note,
		}
	}
	out := InternalUsage{
		Id: u.ID, Number: numberPtr(u.Number), Purpose: InternalUsagePurpose(u.Purpose), PurposeLabel: u.Purpose.Label(),
		Division: *dimRef(v.Refs.Dimension(&u.DivisionID)), Campaign: dimRef(v.Refs.Dimension(u.CampaignID)),
		SourceLocation: locationRef(v.Refs.Location(u.SourceLocationID)), EventEndDate: optDate(u.EventEndDate),
		SettleDueDate: optDate(u.SettleDueDate), BusinessDate: toDate(u.BusinessDate), Note: u.Note,
		Status: InventoryDocStatus(u.Status), StatusLabel: u.Status.Label(), EstimatedCost: amount(u.EstimatedCost),
		CostAmount: amount(u.CostAmount), JournalId: u.JournalID, ReturnDispositionId: u.ReturnDispositionID,
		RequestedBy: actorRef(u.RequestedBy), Released: step(u.Released), HandedOver: step(u.HandedOver),
		Settled: step(u.Settled), Posted: step(u.Posted), Lines: lines, Movements: movements(v.Movements, v.Refs),
		AllowedActions: actions(v.AllowedActions), CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Version: u.Version,
	}
	if u.EventLocationID != nil {
		loc := locationRef(v.Refs.Location(*u.EventLocationID))
		out.EventLocation = &loc
	}
	return out
}

func usageInput(b InternalUsageInput) (domain.InternalUsageInput, error) {
	in := domain.InternalUsageInput{
		Purpose: domain.UsagePurpose(b.Purpose), DivisionID: b.DivisionId, CampaignID: b.CampaignId,
		SourceLocationID: b.SourceLocationId, EventLocationID: b.EventLocationId, EventEndDate: optDatePtr(b.EventEndDate),
		BusinessDate: fromDate(b.BusinessDate), Note: str(b.Note),
	}
	for i, l := range b.Lines {
		q, err := parseQty(l.Qty, lineFieldName(i, "qty"))
		if err != nil {
			return in, err
		}
		in.Lines = append(in.Lines, domain.UsageLineInput{ProductID: l.ProductId, Qty: q, Note: str(l.Note)})
	}
	return in, nil
}

// ListInternalUsages mengembalikan daftar Pemakaian Internal.
func (h *InventoryHandler) ListInternalUsages(ctx context.Context, req ListInternalUsagesRequestObject) (ListInternalUsagesResponseObject, error) {
	p := req.Params
	var purpose *string
	if p.Purpose != nil {
		s := string(*p.Purpose)
		purpose = &s
	}
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListUsages(ctx, docFilter(p.Status, purpose, p.Q), page)
	if err != nil {
		return nil, err
	}
	items := make([]InternalUsage, len(res.Items))
	for i, u := range res.Items {
		items[i] = toInternalUsage(service.UsageView{Usage: u, Refs: res.Refs})
	}
	return ListInternalUsages200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func (h *InventoryHandler) usageView(ctx context.Context, id uuid.UUID) (InternalUsage, error) {
	v, err := h.svc.GetUsage(ctx, id)
	if err != nil {
		return InternalUsage{}, err
	}
	return toInternalUsage(v), nil
}

// CreateInternalUsage membuat draft.
func (h *InventoryHandler) CreateInternalUsage(ctx context.Context, req CreateInternalUsageRequestObject) (CreateInternalUsageResponseObject, error) {
	in, err := usageInput(*req.Body)
	if err != nil {
		return nil, err
	}
	u, err := h.svc.CreateUsage(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.usageView(ctx, u.ID)
	return CreateInternalUsage201JSONResponse(out), err
}

// GetInternalUsage mengembalikan detail.
func (h *InventoryHandler) GetInternalUsage(ctx context.Context, req GetInternalUsageRequestObject) (GetInternalUsageResponseObject, error) {
	out, err := h.usageView(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetInternalUsage200JSONResponse(out), nil
}

// UpdateInternalUsage mengubah draft.
func (h *InventoryHandler) UpdateInternalUsage(ctx context.Context, req UpdateInternalUsageRequestObject) (UpdateInternalUsageResponseObject, error) {
	b := req.Body
	in, err := usageInput(InternalUsageInput{
		Purpose: b.Purpose, DivisionId: b.DivisionId, CampaignId: b.CampaignId, SourceLocationId: b.SourceLocationId,
		EventLocationId: b.EventLocationId, EventEndDate: b.EventEndDate, BusinessDate: b.BusinessDate, Note: b.Note, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.UpdateUsage(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.usageView(ctx, req.Id)
	return UpdateInternalUsage200JSONResponse(out), err
}

// RunInternalUsageAction menjalankan aksi dokumen.
func (h *InventoryHandler) RunInternalUsageAction(ctx context.Context, req RunInternalUsageActionRequestObject) (RunInternalUsageActionResponseObject, error) {
	fns := map[InternalUsageAction]func(context.Context, uuid.UUID, int) (domain.InternalUsage, error){
		"submit": h.svc.SubmitUsage, "revise": h.svc.ReviseUsage, "release": h.svc.ReleaseUsage,
		"handover": h.svc.HandoverUsage, "post": h.svc.PostUsage, "cancel": h.svc.CancelUsage,
	}
	fn, ok := fns[req.Action]
	if !ok {
		return nil, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	if _, err := fn(ctx, req.Id, versionOf(req.Body)); err != nil {
		return nil, err
	}
	out, err := h.usageView(ctx, req.Id)
	return RunInternalUsageAction200JSONResponse(out), err
}

// SettleInternalUsage mencatat sisa barang event.
func (h *InventoryHandler) SettleInternalUsage(ctx context.Context, req SettleInternalUsageRequestObject) (SettleInternalUsageResponseObject, error) {
	lines := make([]domain.SettleLine, len(req.Body.Lines))
	for i, l := range req.Body.Lines {
		q, err := parseQty(l.ReturnedQty, lineFieldName(i, "returnedQty"))
		if err != nil {
			return nil, err
		}
		lines[i] = domain.SettleLine{LineID: l.LineId, ReturnedQty: q}
	}
	if _, err := h.svc.SettleUsage(ctx, req.Id, req.Body.Version, lines); err != nil {
		return nil, err
	}
	out, err := h.usageView(ctx, req.Id)
	return SettleInternalUsage200JSONResponse(out), err
}

// ListInternalUsageDimensionOptions mengembalikan divisi atau kampanye aktif.
func (h *InventoryHandler) ListInternalUsageDimensionOptions(ctx context.Context, req ListInternalUsageDimensionOptionsRequestObject) (ListInternalUsageDimensionOptionsResponseObject, error) {
	opts, err := h.svc.DimensionOptions(ctx, string(req.Kind))
	if err != nil {
		return nil, err
	}
	items := make([]InventoryDimensionOption, len(opts))
	for i, o := range opts {
		items[i] = InventoryDimensionOption{Id: o.ID, Code: o.Code, Name: o.Name, ValidFrom: optDate(o.ValidFrom), ValidTo: optDate(o.ValidTo)}
	}
	return ListInternalUsageDimensionOptions200JSONResponse{Items: items}, nil
}

// --- Repacking --------------------------------------------------------------------------------

func toRepacking(v service.RepackView) Repacking {
	r := v.Repacking
	lines := make([]RepackingLine, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = RepackingLine{
			Id: l.ID, LineNo: l.LineNo, Direction: RepackDirection(l.Direction), Product: productRef(v.Refs.Product(l.ProductID)),
			Location: locationRef(v.Refs.Location(l.LocationID)), StockStatus: InventoryDocStockStatus(l.StockStatus),
			Batch: batchRef(v.Refs.Batch(l.BatchID)), BatchNumber: l.BatchNumber, ProductionDate: optDate(l.ProductionDate),
			ExpiryDate: optDate(l.ExpiryDate), Qty: qty(l.Qty), CostAmount: amount(l.CostAmount), Note: l.Note,
		}
	}
	return Repacking{
		Id: r.ID, Number: numberPtr(r.Number), BusinessDate: toDate(r.BusinessDate), Note: r.Note,
		Status: InventoryDocStatus(r.Status), StatusLabel: r.Status.Label(), CostAmount: amount(r.CostAmount),
		JournalId: r.JournalID, ReturnDispositionId: r.ReturnDispositionID, CreatedBy: actorRef(r.CreatedBy),
		Started: step(r.Started), Posted: step(r.Posted), Lines: lines, Movements: movements(v.Movements, v.Refs),
		AllowedActions: actions(v.AllowedActions), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
}

func repackInput(b RepackingInput) (domain.RepackingInput, error) {
	in := domain.RepackingInput{BusinessDate: fromDate(b.BusinessDate), Note: str(b.Note)}
	for i, l := range b.Lines {
		q, err := parseQty(l.Qty, lineFieldName(i, "qty"))
		if err != nil {
			return in, err
		}
		in.Lines = append(in.Lines, domain.RepackLineInput{
			Direction: domain.RepackDirection(l.Direction), ProductID: l.ProductId, LocationID: l.LocationId,
			StockStatus: stockStatus(l.StockStatus), BatchID: l.BatchId, BatchNumber: str(l.BatchNumber),
			ProductionDate: optDatePtr(l.ProductionDate), ExpiryDate: optDatePtr(l.ExpiryDate), Qty: q, Note: str(l.Note),
		})
	}
	return in, nil
}

// ListRepackings mengembalikan daftar Repacking.
func (h *InventoryHandler) ListRepackings(ctx context.Context, req ListRepackingsRequestObject) (ListRepackingsResponseObject, error) {
	p := req.Params
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListRepackings(ctx, docFilter(p.Status, nil, p.Q), page)
	if err != nil {
		return nil, err
	}
	items := make([]Repacking, len(res.Items))
	for i, r := range res.Items {
		items[i] = toRepacking(service.RepackView{Repacking: r, Refs: res.Refs})
	}
	return ListRepackings200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func (h *InventoryHandler) repackView(ctx context.Context, id uuid.UUID) (Repacking, error) {
	v, err := h.svc.GetRepacking(ctx, id)
	if err != nil {
		return Repacking{}, err
	}
	return toRepacking(v), nil
}

// CreateRepacking membuat draft.
func (h *InventoryHandler) CreateRepacking(ctx context.Context, req CreateRepackingRequestObject) (CreateRepackingResponseObject, error) {
	in, err := repackInput(*req.Body)
	if err != nil {
		return nil, err
	}
	r, err := h.svc.CreateRepacking(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.repackView(ctx, r.ID)
	return CreateRepacking201JSONResponse(out), err
}

// GetRepacking mengembalikan detail.
func (h *InventoryHandler) GetRepacking(ctx context.Context, req GetRepackingRequestObject) (GetRepackingResponseObject, error) {
	out, err := h.repackView(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetRepacking200JSONResponse(out), nil
}

// UpdateRepacking mengubah draft.
func (h *InventoryHandler) UpdateRepacking(ctx context.Context, req UpdateRepackingRequestObject) (UpdateRepackingResponseObject, error) {
	b := req.Body
	in, err := repackInput(RepackingInput{BusinessDate: b.BusinessDate, Note: b.Note, Lines: b.Lines})
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.UpdateRepacking(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.repackView(ctx, req.Id)
	return UpdateRepacking200JSONResponse(out), err
}

// RunRepackingAction menjalankan aksi.
func (h *InventoryHandler) RunRepackingAction(ctx context.Context, req RunRepackingActionRequestObject) (RunRepackingActionResponseObject, error) {
	fns := map[RepackingAction]func(context.Context, uuid.UUID, int) (domain.Repacking, error){
		"start": h.svc.StartRepacking, "post": h.svc.PostRepacking, "cancel": h.svc.CancelRepacking,
	}
	fn, ok := fns[req.Action]
	if !ok {
		return nil, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	if _, err := fn(ctx, req.Id, versionOf(req.Body)); err != nil {
		return nil, err
	}
	out, err := h.repackView(ctx, req.Id)
	return RunRepackingAction200JSONResponse(out), err
}

// --- Pemusnahan -------------------------------------------------------------------------------

func toDisposal(v service.DisposalView) Disposal {
	d := v.Disposal
	lines := make([]DisposalLine, len(d.Lines))
	for i, l := range d.Lines {
		lines[i] = DisposalLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(v.Refs.Product(l.ProductID)), Batch: batchRef(v.Refs.Batch(l.BatchID)),
			Location: locationRef(v.Refs.Location(l.LocationID)), StockStatus: InventoryDocStockStatus(l.StockStatus),
			Qty: qty(l.Qty), CostAmount: amount(l.CostAmount), Note: l.Note,
		}
	}
	photos := make([]DisposalPhoto, len(d.PhotoFileIDs))
	for i, id := range d.PhotoFileIDs {
		photos[i] = DisposalPhoto{Id: id, Name: v.Refs.Files[id]}
	}
	return Disposal{
		Id: d.ID, Number: numberPtr(d.Number), Reason: DisposalReason(d.Reason), ReasonLabel: d.Reason.Label(),
		DisposalDate: toDate(d.DisposalDate), Method: d.Method, Witnesses: d.Witnesses, Photos: photos, Note: d.Note,
		Status: InventoryDocStatus(d.Status), StatusLabel: d.Status.Label(), EstimatedCost: amount(d.EstimatedCost),
		CostAmount: amount(d.CostAmount), PostingRuleCode: d.PostingRuleCode, JournalId: d.JournalID,
		ReturnDispositionId: d.ReturnDispositionID, CreatedBy: actorRef(d.CreatedBy), Submitted: step(d.Submitted),
		Posted: step(d.Posted), Lines: lines, Movements: movements(v.Movements, v.Refs), AllowedActions: actions(v.AllowedActions),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Version: d.Version,
	}
}

func reportOf(date BusinessDate, method *string, witnesses *[]string, photos *[]uuid.UUID) domain.DisposalReport {
	r := domain.DisposalReport{DisposalDate: fromDate(date), Method: str(method)}
	if witnesses != nil {
		r.Witnesses = *witnesses
	}
	if photos != nil {
		r.PhotoFileIDs = *photos
	}
	return r
}

func disposalInput(b DisposalInput) (domain.DisposalInput, error) {
	in := domain.DisposalInput{
		Reason: domain.DisposalReason(b.Reason), Note: str(b.Note),
		DisposalReport: reportOf(b.DisposalDate, b.Method, b.Witnesses, b.PhotoFileIds),
	}
	for i, l := range b.Lines {
		q, err := parseQty(l.Qty, lineFieldName(i, "qty"))
		if err != nil {
			return in, err
		}
		in.Lines = append(in.Lines, domain.DisposalLineInput{
			ProductID: l.ProductId, BatchID: l.BatchId, LocationID: l.LocationId, StockStatus: stockStatus(l.StockStatus),
			Qty: q, Note: str(l.Note),
		})
	}
	return in, nil
}

// ListDisposals mengembalikan daftar Pemusnahan.
func (h *InventoryHandler) ListDisposals(ctx context.Context, req ListDisposalsRequestObject) (ListDisposalsResponseObject, error) {
	p := req.Params
	var reason *string
	if p.Reason != nil {
		s := string(*p.Reason)
		reason = &s
	}
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListDisposals(ctx, docFilter(p.Status, reason, p.Q), page)
	if err != nil {
		return nil, err
	}
	items := make([]Disposal, len(res.Items))
	for i, d := range res.Items {
		items[i] = toDisposal(service.DisposalView{Disposal: d, Refs: res.Refs})
	}
	return ListDisposals200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func (h *InventoryHandler) disposalView(ctx context.Context, id uuid.UUID) (Disposal, error) {
	v, err := h.svc.GetDisposal(ctx, id)
	if err != nil {
		return Disposal{}, err
	}
	return toDisposal(v), nil
}

// CreateDisposal membuat draft.
func (h *InventoryHandler) CreateDisposal(ctx context.Context, req CreateDisposalRequestObject) (CreateDisposalResponseObject, error) {
	in, err := disposalInput(*req.Body)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.CreateDisposal(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.disposalView(ctx, d.ID)
	return CreateDisposal201JSONResponse(out), err
}

// GetDisposal mengembalikan detail.
func (h *InventoryHandler) GetDisposal(ctx context.Context, req GetDisposalRequestObject) (GetDisposalResponseObject, error) {
	out, err := h.disposalView(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetDisposal200JSONResponse(out), nil
}

// UpdateDisposal mengubah draft.
func (h *InventoryHandler) UpdateDisposal(ctx context.Context, req UpdateDisposalRequestObject) (UpdateDisposalResponseObject, error) {
	b := req.Body
	in, err := disposalInput(DisposalInput{
		Reason: b.Reason, Note: b.Note, DisposalDate: b.DisposalDate, Method: b.Method, Witnesses: b.Witnesses,
		PhotoFileIds: b.PhotoFileIds, Lines: b.Lines,
	})
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.UpdateDisposal(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.disposalView(ctx, req.Id)
	return UpdateDisposal200JSONResponse(out), err
}

// UpdateDisposalReport mengisi berita acara.
func (h *InventoryHandler) UpdateDisposalReport(ctx context.Context, req UpdateDisposalReportRequestObject) (UpdateDisposalReportResponseObject, error) {
	b := req.Body
	if _, err := h.svc.UpdateDisposalReport(ctx, req.Id, b.Version, reportOf(b.DisposalDate, b.Method, b.Witnesses, b.PhotoFileIds)); err != nil {
		return nil, err
	}
	out, err := h.disposalView(ctx, req.Id)
	return UpdateDisposalReport200JSONResponse(out), err
}

// RunDisposalAction menjalankan aksi.
func (h *InventoryHandler) RunDisposalAction(ctx context.Context, req RunDisposalActionRequestObject) (RunDisposalActionResponseObject, error) {
	fns := map[DisposalAction]func(context.Context, uuid.UUID, int) (domain.Disposal, error){
		"submit": h.svc.SubmitDisposal, "revise": h.svc.ReviseDisposal, "post": h.svc.PostDisposal, "cancel": h.svc.CancelDisposal,
	}
	fn, ok := fns[req.Action]
	if !ok {
		return nil, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	if _, err := fn(ctx, req.Id, versionOf(req.Body)); err != nil {
		return nil, err
	}
	out, err := h.disposalView(ctx, req.Id)
	return RunDisposalAction200JSONResponse(out), err
}

// --- Disposisi Retur --------------------------------------------------------------------------

func toDisposition(v service.DispositionView) ReturnDispositionDocument {
	d := v.Disposition
	lines := make([]ReturnDispositionDocumentLine, len(d.Lines))
	for i, l := range d.Lines {
		line := ReturnDispositionDocumentLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(v.Refs.Product(l.ProductID)), Batch: batchRef(v.Refs.Batch(l.BatchID)),
			StockStatus: InventoryDocStockStatus(l.StockStatus), Qty: qty(l.Qty), Disposition: ReturnDisposition(l.Disposition),
			DispositionLabel: l.Disposition.Label(), QcNote: l.QCNote, EstimatedCost: amount(l.EstimatedCost),
			CostAmount: amount(l.CostAmount), TargetType: optStr(l.TargetType), TargetId: l.TargetID,
		}
		if l.ReasonID != nil {
			r, ok := v.Refs.Reasons[*l.ReasonID]
			if !ok {
				r = service.ReturnReason{ID: *l.ReasonID, Code: l.ReasonID.String()}
			}
			line.Reason = &InventoryDimensionRef{Id: r.ID, Code: r.Code, Name: r.Name}
		}
		lines[i] = line
	}
	return ReturnDispositionDocument{
		Id: d.ID, Number: numberPtr(d.Number), Location: locationRef(v.Refs.Location(d.LocationID)),
		BusinessDate: toDate(d.BusinessDate), Note: d.Note, Status: InventoryDocStatus(d.Status), StatusLabel: d.Status.Label(),
		DisposalValue: amount(d.DisposalValue), CostAmount: amount(d.CostAmount), JournalId: d.JournalID,
		CreatedBy: actorRef(d.CreatedBy), Submitted: step(d.Submitted), Posted: step(d.Posted), Lines: lines,
		Movements: movements(v.Movements, v.Refs), AllowedActions: actions(v.AllowedActions), CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt, Version: d.Version,
	}
}

func dispositionInput(b ReturnDispositionDocumentInput) (domain.ReturnDispositionInput, error) {
	in := domain.ReturnDispositionInput{LocationID: b.LocationId, BusinessDate: fromDate(b.BusinessDate), Note: str(b.Note)}
	for i, l := range b.Lines {
		q, err := parseQty(l.Qty, lineFieldName(i, "qty"))
		if err != nil {
			return in, err
		}
		line := domain.DispositionLineInput{
			ProductID: l.ProductId, BatchID: l.BatchId, StockStatus: stockStatus(l.StockStatus), Qty: q, ReasonID: l.ReasonId,
			QCNote: str(l.QcNote),
		}
		if l.Disposition != nil {
			line.Disposition = domain.Disposition(*l.Disposition)
		}
		in.Lines = append(in.Lines, line)
	}
	return in, nil
}

// ListReturnDispositions mengembalikan daftar Disposisi Retur.
func (h *InventoryHandler) ListReturnDispositions(ctx context.Context, req ListReturnDispositionsRequestObject) (ListReturnDispositionsResponseObject, error) {
	p := req.Params
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListDispositions(ctx, docFilter(p.Status, nil, p.Q), page)
	if err != nil {
		return nil, err
	}
	items := make([]ReturnDispositionDocument, len(res.Items))
	for i, d := range res.Items {
		items[i] = toDisposition(service.DispositionView{Disposition: d, Refs: res.Refs})
	}
	return ListReturnDispositions200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func (h *InventoryHandler) dispositionView(ctx context.Context, id uuid.UUID) (ReturnDispositionDocument, error) {
	v, err := h.svc.GetDisposition(ctx, id)
	if err != nil {
		return ReturnDispositionDocument{}, err
	}
	return toDisposition(v), nil
}

// CreateReturnDisposition membuat draft.
func (h *InventoryHandler) CreateReturnDisposition(ctx context.Context, req CreateReturnDispositionRequestObject) (CreateReturnDispositionResponseObject, error) {
	in, err := dispositionInput(*req.Body)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.CreateDisposition(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.dispositionView(ctx, d.ID)
	return CreateReturnDisposition201JSONResponse(out), err
}

// GetReturnDisposition mengembalikan detail.
func (h *InventoryHandler) GetReturnDisposition(ctx context.Context, req GetReturnDispositionRequestObject) (GetReturnDispositionResponseObject, error) {
	out, err := h.dispositionView(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetReturnDisposition200JSONResponse(out), nil
}

// UpdateReturnDisposition mengubah draft.
func (h *InventoryHandler) UpdateReturnDisposition(ctx context.Context, req UpdateReturnDispositionRequestObject) (UpdateReturnDispositionResponseObject, error) {
	b := req.Body
	in, err := dispositionInput(ReturnDispositionDocumentInput{LocationId: b.LocationId, BusinessDate: b.BusinessDate, Note: b.Note, Lines: b.Lines})
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.UpdateDisposition(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.dispositionView(ctx, req.Id)
	return UpdateReturnDisposition200JSONResponse(out), err
}

// RunReturnDispositionAction menjalankan aksi.
func (h *InventoryHandler) RunReturnDispositionAction(ctx context.Context, req RunReturnDispositionActionRequestObject) (RunReturnDispositionActionResponseObject, error) {
	fns := map[ReturnDispositionAction]func(context.Context, uuid.UUID, int) (domain.ReturnDisposition, error){
		"submit": h.svc.SubmitDisposition, "revise": h.svc.ReviseDisposition, "post": h.svc.PostDisposition,
		"cancel": h.svc.CancelDisposition,
	}
	fn, ok := fns[req.Action]
	if !ok {
		return nil, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	if _, err := fn(ctx, req.Id, versionOf(req.Body)); err != nil {
		return nil, err
	}
	out, err := h.dispositionView(ctx, req.Id)
	return RunReturnDispositionAction200JSONResponse(out), err
}

// --- Recost -----------------------------------------------------------------------------------

// ListRecostRuns mengembalikan riwayat recost.
func (h *InventoryHandler) ListRecostRuns(ctx context.Context, req ListRecostRunsRequestObject) (ListRecostRunsResponseObject, error) {
	p := req.Params
	page := paging.New(p.Page, p.PageSize)
	res, err := h.svc.ListRecostRuns(ctx, p.ProductId, page)
	if err != nil {
		return nil, err
	}
	items := make([]RecostRun, len(res.Items))
	for i, r := range res.Items {
		items[i] = RecostRun{
			Id: r.ID, Product: productRef(res.Refs.Product(r.ProductID)), BatchId: r.BatchID, FromBusinessDate: toDate(r.FromBusinessDate),
			TriggerSourceType: r.TriggerSourceType, TriggerSourceLabel: r.TriggerSourceLabel, Status: RecostRunStatus(r.Status),
			AdjustmentAmount: amount(r.AdjustmentAmount), JournalId: r.JournalID, Error: r.Error, CreatedAt: r.CreatedAt,
			CompletedAt: r.CompletedAt,
		}
	}
	return ListRecostRuns200JSONResponse{
		Items: items, Page: page.Number, PageSize: page.Size, TotalItems: res.Total, TotalPages: page.TotalPages(res.Total),
	}, nil
}

func lineFieldName(i int, f string) string { return "lines[" + strconv.Itoa(i) + "]." + f }
