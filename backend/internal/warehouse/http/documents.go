package http

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
	"balans/internal/warehouse/service"
)

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func optDate(d *clock.BusinessDate) *BusinessDate {
	if d == nil {
		return nil
	}
	v := toDate(*d)
	return &v
}

func fromDate(d BusinessDate) clock.BusinessDate {
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func qty(d decimal.Decimal) WhQuantity { return d.StringFixed(domain.QtyScale) }

func money(d decimal.Decimal) Money { return d.StringFixed(2) }

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ids(p *[]uuid.UUID) []uuid.UUID {
	if p == nil {
		return nil
	}
	return *p
}

// parseQty membaca qty string; error menjadi VALIDATION_FAILED pada field.
func parseQty(s string, field string, fields map[string]string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		fields[field] = "Qty tidak valid"
		return decimal.Zero
	}
	return d
}

func productRef(r service.Refs, id uuid.UUID) WhProductRef {
	p, ok := r.Products[id]
	if !ok {
		return WhProductRef{Id: id, Sku: "?", Name: "Produk tidak ditemukan"}
	}
	name := p.Name
	if p.Variant != "" {
		name += " " + p.Variant
	}
	return WhProductRef{Id: id, Sku: p.SKU, Name: name, Uom: p.UOM}
}

func batchRef(r service.Refs, id *uuid.UUID) *WhBatchRef {
	if id == nil {
		return nil
	}
	b, ok := r.Batches[*id]
	if !ok {
		return &WhBatchRef{Id: *id, Number: "?"}
	}
	return &WhBatchRef{Id: b.ID, Number: b.Number, ExpiryDate: optDate(b.ExpiryDate)}
}

func locationRef(r service.Refs, id uuid.UUID) WhLocationRef {
	l, ok := r.Locations[id]
	if !ok {
		return WhLocationRef{Id: id, Code: "?", Name: "Lokasi tidak ditemukan"}
	}
	return WhLocationRef{
		Id: l.ID, Code: l.Code, Name: l.Name, LocationType: LocationType(l.Type), LocationTypeLabel: l.Type.Label(),
		IsExternal: l.IsExternal(), WarehouseName: optStr(l.WarehouseName),
	}
}

func optLocationRef(r service.Refs, id *uuid.UUID) *WhLocationRef {
	if id == nil {
		return nil
	}
	v := locationRef(r, *id)
	return &v
}

func pageOf[T any](res paging.Result[T]) (int, int, int64, int) {
	return res.Page.Number, res.Page.Size, res.Total, res.Page.TotalPages(res.Total)
}

// --- Inbound ----------------------------------------------------------------------------------

func toInboundSummary(ib domain.Inbound, lineCount, pending int, refs service.Refs) InboundSummary {
	return InboundSummary{
		Id: ib.ID, Number: ib.Number, SourceType: ib.SourceType, SourceTypeLabel: domain.SourceTypeLabel(ib.SourceType),
		SourceLabel: ib.SourceLabel, Location: locationRef(refs, ib.LocationID), BusinessDate: toDate(ib.BusinessDate),
		Status: InboundStatus(ib.Status), LineCount: lineCount, PendingCount: pending, CreatedAt: ib.CreatedAt,
	}
}

func toInbound(ctx context.Context, d service.InboundDetail) Inbound {
	ib := d.Inbound
	pending := 0
	lines := make([]InboundLine, len(ib.Lines))
	for i, l := range ib.Lines {
		if l.Status == domain.LineQuarantine {
			pending++
		}
		acts := []InboundLineAllowedActions{}
		for _, a := range service.InboundLineActions(ctx, l) {
			acts = append(acts, InboundLineAllowedActions(a))
		}
		lines[i] = InboundLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(d.Refs, l.ProductID), Batch: batchRef(d.Refs, l.BatchID),
			Qty: qty(l.Qty), PassedQty: qty(l.PassedQty), RejectedQty: qty(l.RejectedQty),
			Destination: locationRef(d.Refs, l.DestinationLocationID), RejectLocation: optLocationRef(d.Refs, l.RejectLocationID),
			Status: InboundLineStatus(l.Status), RejectReason: l.RejectReason, CheckedByName: optStr(l.CheckedByName),
			CheckedAt: l.CheckedAt, AllowedActions: acts,
		}
	}
	checks := make([]InboundQualityCheck, len(d.Checks))
	for i, c := range d.Checks {
		photos := c.PhotoFileIDs
		if photos == nil {
			photos = []uuid.UUID{}
		}
		checks[i] = InboundQualityCheck{
			Id: c.ID, LineId: c.DocumentLineID, Result: InboundQualityCheckResult(c.Result), Qty: qty(c.Qty),
			Checks: InboundQcChecklist{Batch: c.Checks.Batch, Expiry: c.Checks.Expiry, Condition: c.Checks.Condition, Seal: c.Checks.Seal},
			Note:   c.Note, PhotoFileIds: photos, CheckedByName: c.CheckedByName, CheckedAt: c.CheckedAt,
		}
	}
	s := toInboundSummary(ib, len(ib.Lines), pending, d.Refs)
	return Inbound{
		Id: s.Id, Number: s.Number, SourceType: s.SourceType, SourceTypeLabel: s.SourceTypeLabel, SourceLabel: s.SourceLabel,
		Location: s.Location, BusinessDate: s.BusinessDate, Status: s.Status, LineCount: s.LineCount, PendingCount: s.PendingCount,
		CreatedAt: s.CreatedAt, SourceId: ib.SourceID, RejectAction: InboundRejectAction(ib.RejectAction), Note: ib.Note,
		CompletedAt: ib.CompletedAt, Version: ib.Version, Lines: lines, Checks: checks,
	}
}

// ListInbounds mengembalikan antrian Inbound.
func (h *WarehouseHandler) ListInbounds(ctx context.Context, req ListInboundsRequestObject) (ListInboundsResponseObject, error) {
	p := req.Params
	f := service.InboundFilter{SourceType: strOr(p.SourceType), WarehouseID: p.WarehouseId, Search: strOr(p.Q)}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, refs, err := h.svc.ListInbounds(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]InboundSummary, len(res.Items))
	for i, it := range res.Items {
		items[i] = toInboundSummary(it.Inbound, it.LineCount, it.PendingCount, refs)
	}
	out := ListInbounds200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// GetInbound mengembalikan detail Inbound.
func (h *WarehouseHandler) GetInbound(ctx context.Context, req GetInboundRequestObject) (GetInboundResponseObject, error) {
	d, err := h.svc.GetInbound(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetInbound200JSONResponse(toInbound(ctx, d)), nil
}

// RecordInboundQc mencatat QC satu baris.
func (h *WarehouseHandler) RecordInboundQc(ctx context.Context, req RecordInboundQcRequestObject) (RecordInboundQcResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	in := domain.QCInput{
		PassedQty: parseQty(b.PassedQty, "passedQty", fields), RejectedQty: parseQty(b.RejectedQty, "rejectedQty", fields),
		Checks:       domain.Checklist{Batch: b.Checks.Batch, Expiry: b.Checks.Expiry, Condition: b.Checks.Condition, Seal: b.Checks.Seal},
		RejectReason: strOr(b.RejectReason), DestinationLocationID: b.DestinationLocationId, Note: strOr(b.Note),
		PhotoFileIDs: ids(b.PhotoFileIds),
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	d, err := h.svc.RecordQC(ctx, req.Id, req.LineId, in)
	if err != nil {
		return nil, err
	}
	return RecordInboundQc200JSONResponse(toInbound(ctx, d)), nil
}

// --- Transfer Stok ----------------------------------------------------------------------------

func toTransferSummary(t domain.StockTransfer, lineCount int, total decimal.Decimal, refs service.Refs) StockTransferSummary {
	return StockTransferSummary{
		Id: t.ID, Number: optStr(t.Number), TransferType: StockTransferType(t.Type), From: locationRef(refs, t.FromLocationID),
		To: locationRef(refs, t.ToLocationID), RequiresShipping: t.RequiresShipping, BusinessDate: toDate(t.BusinessDate),
		Status: StockTransferStatus(t.Status), RequestedByName: t.RequestedByName, LineCount: lineCount, TotalQty: qty(total),
		CreatedAt: t.CreatedAt,
	}
}

func toTransfer(ctx context.Context, d service.TransferDetail) StockTransfer {
	t := d.Transfer
	total := decimal.Zero
	lines := make([]StockTransferLine, len(t.Lines))
	for i, l := range t.Lines {
		total = total.Add(l.Qty)
		lines[i] = StockTransferLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(d.Refs, l.ProductID), Batch: batchRef(d.Refs, l.BatchID),
			Qty: qty(l.Qty), ReceivedQty: qty(l.ReceivedQty), ShortageQty: qty(l.ShortageQty), CostAmount: money(l.CostAmount),
			BatchReason: l.BatchReason, Note: l.Note,
		}
	}
	acts := []StockTransferAction{}
	for _, a := range service.TransferActions(ctx, t) {
		acts = append(acts, StockTransferAction(a))
	}
	s := toTransferSummary(t, len(t.Lines), total, d.Refs)
	return StockTransfer{
		Id: s.Id, Number: s.Number, TransferType: s.TransferType, From: s.From, To: s.To, RequiresShipping: s.RequiresShipping,
		BusinessDate: s.BusinessDate, Status: s.Status, RequestedByName: s.RequestedByName, LineCount: s.LineCount,
		TotalQty: s.TotalQty, CreatedAt: s.CreatedAt, Note: t.Note, SourceType: optStr(t.SourceType), SourceId: t.SourceID,
		SubmittedAt: t.SubmittedAt, ReleasedByName: optStr(t.ReleasedByName), ReleasedAt: t.ReleasedAt,
		PreparedByName: optStr(t.PreparedByName), PreparedAt: t.PreparedAt, ShippedByName: optStr(t.ShippedByName),
		ShippedAt: t.ShippedAt, ReceivedByName: optStr(t.ReceivedByName), ReceivedAt: t.ReceivedAt, CompletedAt: t.CompletedAt,
		CancelledAt: t.CancelledAt, CancelReason: t.CancelReason, Version: t.Version, Lines: lines, AllowedActions: acts,
	}
}

func transferInput(t StockTransferType, from, to uuid.UUID, date BusinessDate, note *string, lines []StockTransferLineInput) (domain.TransferInput, error) {
	fields := map[string]string{}
	in := domain.TransferInput{
		Type: domain.TransferType(t), FromLocationID: from, ToLocationID: to, BusinessDate: fromDate(date), Note: strOr(note),
	}
	for i, l := range lines {
		in.Lines = append(in.Lines, domain.TransferLineInput{
			ProductID: l.ProductId, BatchID: l.BatchId, Qty: parseQty(l.Qty, "lines["+itoa(i)+"].qty", fields),
			BatchReason: strOr(l.BatchReason), Note: strOr(l.Note),
		})
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

// ListStockTransfers mengembalikan daftar transfer.
func (h *WarehouseHandler) ListStockTransfers(ctx context.Context, req ListStockTransfersRequestObject) (ListStockTransfersResponseObject, error) {
	p := req.Params
	f := service.TransferFilter{Type: domain.TransferDirect, LocationID: p.LocationId, Search: strOr(p.Q)}
	if p.TransferType != nil {
		f.Type = domain.TransferType(*p.TransferType)
	}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, string(s))
		}
	}
	res, refs, err := h.svc.ListTransfers(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]StockTransferSummary, len(res.Items))
	for i, it := range res.Items {
		items[i] = toTransferSummary(it.Transfer, it.LineCount, it.TotalQty, refs)
	}
	out := ListStockTransfers200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// CreateStockTransfer membuat transfer draft.
func (h *WarehouseHandler) CreateStockTransfer(ctx context.Context, req CreateStockTransferRequestObject) (CreateStockTransferResponseObject, error) {
	b := req.Body
	in, err := transferInput(b.TransferType, b.FromLocationId, b.ToLocationId, b.BusinessDate, b.Note, b.Lines)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.CreateTransfer(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateStockTransfer201JSONResponse(toTransfer(ctx, d)), nil
}

// GetStockTransfer mengembalikan detail transfer.
func (h *WarehouseHandler) GetStockTransfer(ctx context.Context, req GetStockTransferRequestObject) (GetStockTransferResponseObject, error) {
	d, err := h.svc.GetTransfer(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetStockTransfer200JSONResponse(toTransfer(ctx, d)), nil
}

// UpdateStockTransfer mengubah transfer draft.
func (h *WarehouseHandler) UpdateStockTransfer(ctx context.Context, req UpdateStockTransferRequestObject) (UpdateStockTransferResponseObject, error) {
	b := req.Body
	in, err := transferInput(b.TransferType, b.FromLocationId, b.ToLocationId, b.BusinessDate, b.Note, b.Lines)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.UpdateTransfer(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateStockTransfer200JSONResponse(toTransfer(ctx, d)), nil
}

// ActOnStockTransfer menjalankan aksi status transfer.
func (h *WarehouseHandler) ActOnStockTransfer(ctx context.Context, req ActOnStockTransferRequestObject) (ActOnStockTransferResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	act := service.TransferAction{Action: string(b.Action), Version: b.Version, Reason: strOr(b.Reason)}
	if b.Lines != nil {
		for i, l := range *b.Lines {
			act.Lines = append(act.Lines, domain.ReceiveLine{LineID: l.LineId, Qty: parseQty(l.Qty, "lines["+itoa(i)+"].qty", fields)})
		}
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	d, err := h.svc.ActOnTransfer(ctx, req.Id, act)
	if err != nil {
		return nil, err
	}
	return ActOnStockTransfer200JSONResponse(toTransfer(ctx, d)), nil
}

// ListStockInTransit mengembalikan Stok dalam Pengiriman.
func (h *WarehouseHandler) ListStockInTransit(ctx context.Context, req ListStockInTransitRequestObject) (ListStockInTransitResponseObject, error) {
	p := req.Params
	res, err := h.svc.ListInTransit(ctx, p.LocationId, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]StockInTransitLine, len(res.Result.Items))
	for i, l := range res.Result.Items {
		items[i] = toInTransit(l, res.Refs)
	}
	out := ListStockInTransit200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res.Result)
	return out, nil
}

func toInTransit(l postgres.InTransitLine, refs service.Refs) StockInTransitLine {
	return StockInTransitLine{
		TransferId: l.TransferID, Number: optStr(l.Number), LineId: l.LineID, From: locationRef(refs, l.FromLocationID),
		To: locationRef(refs, l.ToLocationID), BusinessDate: toDate(l.BusinessDate), ShippedAt: l.ShippedAt,
		Status: StockTransferStatus(l.Status), Product: productRef(refs, l.ProductID), Batch: batchRef(refs, l.BatchID),
		Qty: qty(l.Qty), ReceivedQty: qty(l.ReceivedQty), OutstandingQty: qty(l.Qty.Sub(l.ReceivedQty)), CostAmount: money(l.CostAmount),
	}
}

// --- Stock Count ------------------------------------------------------------------------------

func toCountSummary(c domain.StockCount, lineCount, counted int, refs service.Refs) StockCountSummary {
	s := StockCountSummary{
		Id: c.ID, Number: optStr(c.Number), CountType: StockCountType(c.CountType), Location: locationRef(refs, c.LocationID),
		BusinessDate: toDate(c.BusinessDate), Status: StockCountStatus(c.Status), ScheduleId: c.ScheduleID,
		AssignedToName: c.AssignedToName, LineCount: lineCount, CountedCount: counted, CreatedAt: c.CreatedAt,
	}
	if c.RevealsSystemQty() {
		v := money(c.VarianceAmount)
		s.VarianceAmount = &v
	}
	return s
}

func toCount(ctx context.Context, d service.StockCountDetail) StockCount {
	c := d.Count
	reveal := c.RevealsSystemQty()
	counted := 0
	lines := make([]StockCountLine, len(c.Lines))
	for i, l := range c.Lines {
		line := StockCountLine{
			Id: l.ID, LineNo: l.LineNo, Product: productRef(d.Refs, l.ProductID), Batch: batchRef(d.Refs, l.BatchID),
			IsExtra: l.IsExtra, PhotoFileId: l.PhotoFileID, Note: l.Note, CountedAt: l.CountedAt,
		}
		if l.CountedQty != nil {
			counted++
			q := qty(*l.CountedQty)
			line.CountedQty = &q
		}
		if reveal {
			sys, vq, va, uc := qty(l.SystemQty), qty(l.VarianceQty), money(l.VarianceAmount), l.UnitCost.StringFixed(4)
			line.SystemQty, line.VarianceQty, line.VarianceAmount, line.UnitCost = &sys, &vq, &va, &uc
		}
		lines[i] = line
	}
	acts := []StockCountAction{}
	for _, a := range service.StockCountActions(ctx, c) {
		acts = append(acts, StockCountAction(a))
	}
	photos := c.PhotoFileIDs
	if photos == nil {
		photos = []uuid.UUID{}
	}
	s := toCountSummary(c, len(c.Lines), counted, d.Refs)
	out := StockCount{
		Id: s.Id, Number: s.Number, CountType: s.CountType, Location: s.Location, BusinessDate: s.BusinessDate,
		Status: s.Status, ScheduleId: s.ScheduleId, AssignedToName: s.AssignedToName, LineCount: s.LineCount,
		CountedCount: s.CountedCount, VarianceAmount: s.VarianceAmount, CreatedAt: s.CreatedAt, Note: c.Note,
		PhotoFileIds: photos, Revealed: reveal, SnapshotAt: c.SnapshotAt, StartedByName: optStr(c.StartedByName),
		SubmittedByName: optStr(c.SubmittedByName), SubmittedAt: c.SubmittedAt, DecidedAt: c.DecidedAt,
		DecisionNote: c.DecisionNote, PostedAt: c.PostedAt, JournalEntryId: c.JournalEntryID, Version: c.Version,
		Lines: lines, AllowedActions: acts,
	}
	if reveal {
		n := money(c.NetVariance)
		out.NetVariance = &n
	}
	return out
}

// ListStockCounts mengembalikan daftar stock count.
func (h *WarehouseHandler) ListStockCounts(ctx context.Context, req ListStockCountsRequestObject) (ListStockCountsResponseObject, error) {
	p := req.Params
	f := service.StockCountFilter{LocationID: p.LocationId, Search: strOr(p.Q)}
	if p.CountType != nil {
		f.CountType = string(*p.CountType)
	}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, string(s))
		}
	}
	res, refs, err := h.svc.ListStockCounts(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]StockCountSummary, len(res.Items))
	for i, it := range res.Items {
		items[i] = toCountSummary(it.Count, it.LineCount, it.CountedCount, refs)
	}
	out := ListStockCounts200JSONResponse{Items: items}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageOf(res)
	return out, nil
}

// CreateStockCounts membuat stock count draft per lokasi.
func (h *WarehouseHandler) CreateStockCounts(ctx context.Context, req CreateStockCountsRequestObject) (CreateStockCountsResponseObject, error) {
	b := req.Body
	in := service.StockCountCreate{CountType: domain.CountType(b.CountType), LocationIDs: b.LocationIds, Note: strOr(b.Note)}
	if b.BusinessDate != nil {
		in.BusinessDate = fromDate(*b.BusinessDate)
	}
	list, err := h.svc.CreateStockCounts(ctx, in)
	if err != nil {
		return nil, err
	}
	locs := make([]uuid.UUID, len(list))
	for i, c := range list {
		locs[i] = c.LocationID
	}
	refs := service.Refs{}
	if refs.Locations, err = h.svc.LocationsByIDs(ctx, locs); err != nil {
		return nil, err
	}
	items := make([]StockCountSummary, len(list))
	for i, c := range list {
		items[i] = toCountSummary(c, 0, 0, refs)
	}
	return CreateStockCounts201JSONResponse{Items: items}, nil
}

// GetStockCount mengembalikan detail stock count.
func (h *WarehouseHandler) GetStockCount(ctx context.Context, req GetStockCountRequestObject) (GetStockCountResponseObject, error) {
	d, err := h.svc.GetStockCount(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetStockCount200JSONResponse(toCount(ctx, d)), nil
}

// SaveStockCountEntries menyimpan isian hitung (autosave).
func (h *WarehouseHandler) SaveStockCountEntries(ctx context.Context, req SaveStockCountEntriesRequestObject) (SaveStockCountEntriesResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	in := service.CountEntries{}
	if b.Lines != nil {
		for i, l := range *b.Lines {
			e := domain.CountEntry{LineID: l.LineId, Note: l.Note, PhotoFileID: l.PhotoFileId}
			if l.CountedQty != nil {
				q := parseQty(*l.CountedQty, "lines["+itoa(i)+"].countedQty", fields)
				e.CountedQty = &q
			}
			in.Lines = append(in.Lines, e)
		}
	}
	if b.ExtraLines != nil {
		for i, x := range *b.ExtraLines {
			in.Extras = append(in.Extras, domain.ExtraEntry{
				ProductID: x.ProductId, BatchID: x.BatchId, Note: strOr(x.Note),
				CountedQty: parseQty(x.CountedQty, "extraLines["+itoa(i)+"].countedQty", fields),
			})
		}
	}
	if b.PhotoFileIds != nil {
		in.SetPhotos, in.PhotoFileIDs = true, *b.PhotoFileIds
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	d, err := h.svc.SaveCounts(ctx, req.Id, in)
	if err != nil {
		return nil, err
	}
	return SaveStockCountEntries200JSONResponse(toCount(ctx, d)), nil
}

// ActOnStockCount menjalankan aksi status stock count.
func (h *WarehouseHandler) ActOnStockCount(ctx context.Context, req ActOnStockCountRequestObject) (ActOnStockCountResponseObject, error) {
	b := req.Body
	var (
		d   service.StockCountDetail
		err error
	)
	switch string(b.Action) {
	case domain.ActionStart:
		d, err = h.svc.StartCount(ctx, req.Id, b.Version)
	case domain.ActionSubmit:
		d, err = h.svc.SubmitCount(ctx, req.Id, b.Version)
	case domain.ActionRecount:
		d, err = h.svc.RecountCount(ctx, req.Id, b.Version)
	case domain.ActionCancel:
		d, err = h.svc.CancelCount(ctx, req.Id, b.Version, strOr(b.Reason))
	default:
		return nil, apperr.Validation(map[string]string{"action": "Aksi tidak dikenal"})
	}
	if err != nil {
		return nil, err
	}
	return ActOnStockCount200JSONResponse(toCount(ctx, d)), nil
}

// --- Jadwal -----------------------------------------------------------------------------------

func toSchedule(s domain.CountSchedule) StockCountSchedule {
	locs := s.LocationIDs
	if locs == nil {
		locs = []uuid.UUID{}
	}
	return StockCountSchedule{
		Id: s.ID, Name: s.Name, CountType: StockCountType(s.CountType), Frequency: StockCountFrequency(s.Frequency),
		DayOfWeek: s.DayOfWeek, DayOfMonth: s.DayOfMonth, MonthOfYear: s.MonthOfYear, WarehouseId: s.WarehouseID,
		LocationIds: locs, AssigneeUserId: s.AssigneeUserID, AssigneeName: s.AssigneeName, IsActive: s.IsActive,
		NextRunDate: toDate(s.NextRunDate), LastRunDate: optDate(s.LastRunDate), Version: s.Version,
	}
}

func scheduleInput(name string, t StockCountType, f StockCountFrequency, dow, dom, moy *int, wh *uuid.UUID, locs *[]uuid.UUID, assignee *uuid.UUID, active bool) domain.ScheduleInput {
	return domain.ScheduleInput{
		Name: name, CountType: domain.CountType(t), Frequency: domain.Frequency(f), DayOfWeek: dow, DayOfMonth: dom,
		MonthOfYear: moy, WarehouseID: wh, LocationIDs: ids(locs), AssigneeUserID: assignee, IsActive: active,
	}
}

// ListStockCountSchedules mengembalikan jadwal stock count.
func (h *WarehouseHandler) ListStockCountSchedules(ctx context.Context, _ ListStockCountSchedulesRequestObject) (ListStockCountSchedulesResponseObject, error) {
	list, err := h.svc.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]StockCountSchedule, len(list))
	for i, s := range list {
		items[i] = toSchedule(s)
	}
	return ListStockCountSchedules200JSONResponse{Items: items}, nil
}

// CreateStockCountSchedule membuat jadwal.
func (h *WarehouseHandler) CreateStockCountSchedule(ctx context.Context, req CreateStockCountScheduleRequestObject) (CreateStockCountScheduleResponseObject, error) {
	b := req.Body
	s, err := h.svc.CreateSchedule(ctx, scheduleInput(b.Name, b.CountType, b.Frequency, b.DayOfWeek, b.DayOfMonth, b.MonthOfYear,
		b.WarehouseId, b.LocationIds, b.AssigneeUserId, b.IsActive))
	if err != nil {
		return nil, err
	}
	return CreateStockCountSchedule201JSONResponse(toSchedule(s)), nil
}

// UpdateStockCountSchedule mengubah jadwal.
func (h *WarehouseHandler) UpdateStockCountSchedule(ctx context.Context, req UpdateStockCountScheduleRequestObject) (UpdateStockCountScheduleResponseObject, error) {
	b := req.Body
	s, err := h.svc.UpdateSchedule(ctx, req.Id, b.Version, scheduleInput(b.Name, b.CountType, b.Frequency, b.DayOfWeek, b.DayOfMonth,
		b.MonthOfYear, b.WarehouseId, b.LocationIds, b.AssigneeUserId, b.IsActive))
	if err != nil {
		return nil, err
	}
	return UpdateStockCountSchedule200JSONResponse(toSchedule(s)), nil
}

func itoa(i int) string { return strconv.Itoa(i) }
