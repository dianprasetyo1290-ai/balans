package http

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
	"balans/internal/sales/service"
)

// WithReturns memasang service Retur Penjualan & Nota Kredit.
func (h *SalesHandler) WithReturns(r *service.ReturnService) *SalesHandler {
	h.returns = r
	return h
}

func optUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func nameOf(names map[uuid.UUID]string, id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	n := names[*id]
	return &n
}

func returnSummary(r domain.SalesReturn) SalesReturnSummary {
	return SalesReturnSummary{
		Id: r.ID, Number: optStr(r.Number), Status: SalesReturnStatus(r.Status), SalesOrderId: r.OrderID,
		SalesOrderNumber: r.OrderNumber, InvoiceId: optUUID(r.InvoiceID), InvoiceNumber: optStr(r.InvoiceNumber),
		OrderType: SalesOrderType(r.OrderType), PartyName: r.PartyName, ReturnDate: toDate(r.ReturnDate),
		SettlementMethod: SalesReturnSettlement(r.Settlement), RequiresVerification: r.RequiresVerification,
		PickupRequired: r.PickupRequired, PickupUserId: optUUID(r.PickupUserID), InboundNumber: optStr(r.InboundNumber),
		ReturnAmount: r.Amount.String(), CostAmount: r.Cost.String(), CreatedAt: r.CreatedAt, Version: r.Version,
	}
}

func files(ids []uuid.UUID, names map[uuid.UUID]string) []SalesReturnFile {
	out := make([]SalesReturnFile, len(ids))
	for i, id := range ids {
		out[i] = SalesReturnFile{Id: id, Name: names[id]}
	}
	return out
}

func toReturn(v service.ReturnView) SalesReturn {
	s := returnSummary(v.SalesReturn)
	out := SalesReturn{
		Id: s.Id, Number: s.Number, Status: s.Status, SalesOrderId: s.SalesOrderId, SalesOrderNumber: s.SalesOrderNumber,
		InvoiceId: s.InvoiceId, InvoiceNumber: s.InvoiceNumber, OrderType: s.OrderType, PartyName: s.PartyName,
		ReturnDate: s.ReturnDate, SettlementMethod: s.SettlementMethod, RequiresVerification: s.RequiresVerification,
		PickupRequired: s.PickupRequired, PickupUserId: s.PickupUserId, InboundNumber: s.InboundNumber,
		ReturnAmount: s.ReturnAmount, CostAmount: s.CostAmount, CreatedAt: s.CreatedAt, Version: s.Version,
		Note: v.Note, SendBackNote: v.SendBackNote, VerifiedByName: nameOf(v.UserNames, v.VerifiedBy),
		VerifiedAt: v.VerifiedAt, VerifyNote: v.VerifyNote, PickupUserName: nameOf(v.UserNames, v.PickupUserID),
		PickupAssignedAt: v.PickupAssignedAt, PickedUpAt: v.PickedUpAt, PickupNote: v.PickupNote,
		PickupFiles: files(v.PickupFileIDs, v.FileNames), ReceivedByName: nameOf(v.UserNames, v.ReceivedBy),
		ReceivedAt: v.ReceivedAt, ReceiveNote: v.ReceiveNote, DocumentationFiles: files(v.DocumentationFileIDs, v.FileNames),
		InboundId: optUUID(v.InboundID), CancelReason: v.CancelReason, PostedAt: v.PostedAt,
		JournalEntryId: optUUID(v.JournalEntryID), CreatedByName: v.UserNames[v.CreatedBy],
		Lines: make([]SalesReturnLine, len(v.Lines)), AllowedActions: make([]SalesReturnAllowedActions, len(v.Actions)),
	}
	for i, l := range v.Lines {
		passed, rejected, done := v.LineQC(l.ID)
		reason := v.Reasons[l.ReasonID]
		ref := SalesReturnReasonRef{Id: l.ReasonID, Code: reason.Code, Name: reason.Name}
		if reason.DefaultDisposition != "" {
			d := ReturnDisposition(reason.DefaultDisposition)
			ref.DefaultDisposition = &d
		}
		out.Lines[i] = SalesReturnLine{
			Id: l.ID, LineNo: l.LineNo, SalesOrderLineId: l.OrderLineID, Product: productRef(l.ProductID, v.Products),
			BatchId: optUUID(l.BatchID), BatchNumber: nameOf(v.BatchNames, l.BatchID), Qty: qty(l.Qty), ReturnReason: ref,
			UnitPrice: l.UnitPrice.String(), LineAmount: l.Amount.String(), UnitCost: l.UnitCost.StringFixed(domain.CostScale),
			CostAmount: l.Cost.String(), Note: l.Note, PassedQty: qty(passed), RejectedQty: qty(rejected), QcDone: done,
		}
	}
	for i, a := range v.Actions {
		out.AllowedActions[i] = SalesReturnAllowedActions(a)
	}
	if c := v.CreditNote; c != nil {
		out.CreditNote = &SalesReturnCreditNoteRef{
			Id: c.ID, Number: c.Number, SettlementMethod: CreditNoteSettlement(c.Settlement),
			Status: CreditNoteStatus(c.Status), Amount: c.Amount.String(), RemainingAmount: c.Remaining().String(),
		}
	}
	return out
}

func returnInput(orderID uuid.UUID, b SalesReturnFields) (domain.ReturnInput, error) {
	fields := map[string]string{}
	in := domain.ReturnInput{OrderID: orderID, ReturnDate: fromDate(b.ReturnDate), Note: str(b.Note)}
	if b.SettlementMethod != nil {
		in.Settlement = domain.SettlementMethod(*b.SettlementMethod)
	}
	for i, l := range b.Lines {
		in.Lines = append(in.Lines, domain.ReturnLineInput{
			OrderLineID: l.SalesOrderLineId, BatchID: l.BatchId, ReasonID: l.ReturnReasonId, Note: str(l.Note),
			Qty: parseQty(l.Qty, fmt.Sprintf("lines.%d.qty", i), fields),
		})
	}
	return in, validation(fields)
}

// ListSalesReturnSources mencari Sales Order sumber retur.
func (h *SalesHandler) ListSalesReturnSources(ctx context.Context, req ListSalesReturnSourcesRequestObject) (ListSalesReturnSourcesResponseObject, error) {
	list, err := h.returns.ReturnSources(ctx, str(req.Params.Q))
	if err != nil {
		return nil, err
	}
	out := SalesReturnSourceList{Items: make([]SalesReturnSource, len(list))}
	for i, s := range list {
		out.Items[i] = SalesReturnSource{
			Id: s.ID, Number: s.Number, OrderType: SalesOrderType(s.Type), PartyName: s.PartyName,
			BusinessDate: toDate(s.BusinessDate), Status: SalesDocumentStatus(s.Status), TotalAmount: s.Total.String(),
		}
	}
	return ListSalesReturnSources200JSONResponse(out), nil
}

// GetSalesReturnSource mengembalikan baris Sales Order yang bisa diretur.
func (h *SalesHandler) GetSalesReturnSource(ctx context.Context, req GetSalesReturnSourceRequestObject) (GetSalesReturnSourceResponseObject, error) {
	v, err := h.returns.ReturnSource(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	o := v.Order
	out := SalesReturnSourceDetail{
		Order: SalesReturnSource{
			Id: o.ID, Number: o.Number, OrderType: SalesOrderType(o.Type), PartyName: o.PartyName,
			BusinessDate: toDate(o.BusinessDate), Status: SalesDocumentStatus(o.Status), TotalAmount: o.Total.String(),
		},
		RequiresVerification: domain.RequiresVerification(o), Lines: make([]SalesReturnableLine, len(v.Lines)),
	}
	if v.Invoice != nil {
		out.InvoiceNumber = &v.Invoice.Number
	}
	for i, l := range v.Lines {
		batches := make([]SalesReturnBatchOption, len(l.Batches))
		for j, b := range l.Batches {
			batches[j] = SalesReturnBatchOption{Id: b, Number: v.BatchNames[b]}
		}
		out.Lines[i] = SalesReturnableLine{
			SalesOrderLineId: l.OrderLineID, LineNo: l.LineNo, Product: productRef(l.ProductID, v.Products),
			IsBundle: l.IsBundle, OrderedQty: qty(l.OrderedQty), ShippedQty: qty(l.ShippedQty),
			ReturnedQty: qty(l.ReturnedQty), AvailableQty: qty(l.AvailableQty()), UnitPrice: l.UnitPrice.String(),
			UnitCost: l.UnitCost.StringFixed(domain.CostScale), Batches: batches,
		}
	}
	return GetSalesReturnSource200JSONResponse(out), nil
}

// ListSalesReturnPickupUsers mengembalikan Merchandiser aktif.
func (h *SalesHandler) ListSalesReturnPickupUsers(ctx context.Context, _ ListSalesReturnPickupUsersRequestObject) (ListSalesReturnPickupUsersResponseObject, error) {
	list, err := h.returns.PickupUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := SalesReturnPickupUserList{Items: make([]SalesReturnPickupUser, len(list))}
	for i, u := range list {
		out.Items[i] = SalesReturnPickupUser{Id: u.ID, Name: u.Name}
	}
	return ListSalesReturnPickupUsers200JSONResponse(out), nil
}

// ListSalesReturns mengembalikan daftar retur.
func (h *SalesHandler) ListSalesReturns(ctx context.Context, req ListSalesReturnsRequestObject) (ListSalesReturnsResponseObject, error) {
	p := req.Params
	f := postgres.ReturnFilter{OrderID: p.SalesOrderId, PartyID: p.PartyId, Search: str(p.Q)}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, domain.Status(s))
		}
	}
	if p.AssignedToMe != nil && *p.AssignedToMe {
		me := actor.UserID(ctx)
		f.PickupUserID = &me
	}
	res, err := h.returns.ListReturns(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	out := SalesReturnList{Items: make([]SalesReturnSummary, len(res.Items))}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageParams(res.Page, res.Total)
	for i, r := range res.Items {
		out.Items[i] = returnSummary(r)
	}
	return ListSalesReturns200JSONResponse(out), nil
}

// CreateSalesReturn menyimpan draft retur.
func (h *SalesHandler) CreateSalesReturn(ctx context.Context, req CreateSalesReturnRequestObject) (CreateSalesReturnResponseObject, error) {
	b := req.Body
	in, err := returnInput(b.SalesOrderId, SalesReturnFields{Lines: b.Lines, Note: b.Note, ReturnDate: b.ReturnDate, SettlementMethod: b.SettlementMethod})
	if err != nil {
		return nil, err
	}
	v, err := h.returns.CreateReturn(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateSalesReturn201JSONResponse(toReturn(v)), nil
}

// GetSalesReturn mengembalikan detail retur.
func (h *SalesHandler) GetSalesReturn(ctx context.Context, req GetSalesReturnRequestObject) (GetSalesReturnResponseObject, error) {
	v, err := h.returns.GetReturn(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetSalesReturn200JSONResponse(toReturn(v)), nil
}

// UpdateSalesReturn mengubah draft retur.
func (h *SalesHandler) UpdateSalesReturn(ctx context.Context, req UpdateSalesReturnRequestObject) (UpdateSalesReturnResponseObject, error) {
	b := req.Body
	in, err := returnInput(uuid.Nil, SalesReturnFields{Lines: b.Lines, Note: b.Note, ReturnDate: b.ReturnDate, SettlementMethod: b.SettlementMethod})
	if err != nil {
		return nil, err
	}
	v, err := h.returns.UpdateReturn(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateSalesReturn200JSONResponse(toReturn(v)), nil
}

// SubmitSalesReturn mengajukan retur.
func (h *SalesHandler) SubmitSalesReturn(ctx context.Context, req SubmitSalesReturnRequestObject) (SubmitSalesReturnResponseObject, error) {
	v, err := h.returns.SubmitReturn(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitSalesReturn200JSONResponse(toReturn(v)), nil
}

// SendBackSalesReturn mengembalikan retur ke Draft.
func (h *SalesHandler) SendBackSalesReturn(ctx context.Context, req SendBackSalesReturnRequestObject) (SendBackSalesReturnResponseObject, error) {
	v, err := h.returns.SendBackReturn(ctx, req.Id, req.Body.Version, req.Body.Note)
	if err != nil {
		return nil, err
	}
	return SendBackSalesReturn200JSONResponse(toReturn(v)), nil
}

// VerifySalesReturn memverifikasi retur MT.
func (h *SalesHandler) VerifySalesReturn(ctx context.Context, req VerifySalesReturnRequestObject) (VerifySalesReturnResponseObject, error) {
	v, err := h.returns.VerifyReturn(ctx, req.Id, req.Body.Version, req.Body.PickupRequired, str(req.Body.Note))
	if err != nil {
		return nil, err
	}
	return VerifySalesReturn200JSONResponse(toReturn(v)), nil
}

// AssignSalesReturnPickup menugaskan Merchandiser.
func (h *SalesHandler) AssignSalesReturnPickup(ctx context.Context, req AssignSalesReturnPickupRequestObject) (AssignSalesReturnPickupResponseObject, error) {
	v, err := h.returns.AssignPickup(ctx, req.Id, req.Body.Version, req.Body.PickupUserId)
	if err != nil {
		return nil, err
	}
	return AssignSalesReturnPickup200JSONResponse(toReturn(v)), nil
}

// RecordSalesReturnPickup mencatat pickup Merchandiser.
func (h *SalesHandler) RecordSalesReturnPickup(ctx context.Context, req RecordSalesReturnPickupRequestObject) (RecordSalesReturnPickupResponseObject, error) {
	v, err := h.returns.RecordPickup(ctx, req.Id, req.Body.Version, str(req.Body.Note), req.Body.PhotoFileIds)
	if err != nil {
		return nil, err
	}
	return RecordSalesReturnPickup200JSONResponse(toReturn(v)), nil
}

// ReceiveSalesReturn menerima barang retur MT.
func (h *SalesHandler) ReceiveSalesReturn(ctx context.Context, req ReceiveSalesReturnRequestObject) (ReceiveSalesReturnResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	var qtys []domain.ReceivedQty
	if b.Lines != nil {
		for i, l := range *b.Lines {
			qtys = append(qtys, domain.ReceivedQty{LineID: l.LineId, Qty: parseQty(l.Qty, fmt.Sprintf("lines.%d.qty", i), fields)})
		}
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	v, err := h.returns.ReceiveReturn(ctx, req.Id, b.Version, str(b.Note), b.DocumentationFileIds, qtys)
	if err != nil {
		return nil, err
	}
	return ReceiveSalesReturn200JSONResponse(toReturn(v)), nil
}

// CancelSalesReturn membatalkan retur.
func (h *SalesHandler) CancelSalesReturn(ctx context.Context, req CancelSalesReturnRequestObject) (CancelSalesReturnResponseObject, error) {
	v, err := h.returns.CancelReturn(ctx, req.Id, req.Body.Version, str(req.Body.Reason))
	if err != nil {
		return nil, err
	}
	return CancelSalesReturn200JSONResponse(toReturn(v)), nil
}

// --- Nota Kredit -----------------------------------------------------------------------------

func creditNoteSummary(c domain.CreditNote) CreditNoteSummary {
	return CreditNoteSummary{
		Id: c.ID, Number: c.Number, SalesReturnId: c.ReturnID, SalesReturnNumber: c.ReturnNumber,
		InvoiceId: optUUID(c.InvoiceID), InvoiceNumber: optStr(c.InvoiceNumber), PartyType: SalesPartyType(c.PartyType),
		PartyId: c.PartyID, PartyName: c.PartyName, SettlementMethod: CreditNoteSettlement(c.Settlement),
		IssueDate: toDate(c.IssueDate), Amount: c.Amount.String(), AppliedAmount: c.Applied.String(),
		RefundedAmount: c.Refunded.String(), RemainingAmount: c.Remaining().String(), Status: CreditNoteStatus(c.Status),
		CreatedAt: c.CreatedAt, Version: c.Version,
	}
}

func toCreditNote(v service.CreditNoteView) CreditNote {
	s := creditNoteSummary(v.CreditNote)
	out := CreditNote{
		Id: s.Id, Number: s.Number, SalesReturnId: s.SalesReturnId, SalesReturnNumber: s.SalesReturnNumber,
		InvoiceId: s.InvoiceId, InvoiceNumber: s.InvoiceNumber, PartyType: s.PartyType, PartyId: s.PartyId,
		PartyName: s.PartyName, SettlementMethod: s.SettlementMethod, IssueDate: s.IssueDate, Amount: s.Amount,
		AppliedAmount: s.AppliedAmount, RefundedAmount: s.RefundedAmount, RemainingAmount: s.RemainingAmount,
		Status: s.Status, CreatedAt: s.CreatedAt, Version: s.Version, DecisionNote: v.DecisionNote,
		RefundAccountCode: v.RefundAccount, RefundReference: v.RefundReference, RefundJournalEntryId: optUUID(v.RefundJournalID),
		Applications:   make([]CreditNoteApplication, len(v.Applications)),
		OpenInvoices:   make([]CreditNoteOpenInvoice, len(v.OpenInvoices)),
		AllowedActions: make([]CreditNoteAllowedActions, len(v.Actions)),
	}
	if v.RefundDate != nil {
		d := toDate(*v.RefundDate)
		out.RefundDate = &d
	}
	for i, a := range v.Applications {
		out.Applications[i] = CreditNoteApplication{
			Id: a.ID, InvoiceId: a.InvoiceID, InvoiceNumber: a.InvoiceNumber, Amount: a.Amount.String(), AppliedDate: toDate(a.AppliedDate),
		}
	}
	for i, inv := range v.OpenInvoices {
		out.OpenInvoices[i] = CreditNoteOpenInvoice{
			Id: inv.ID, Number: inv.Number, InvoiceDate: toDate(inv.InvoiceDate), DueDate: toDate(inv.DueDate),
			TotalAmount: inv.Total.String(), OutstandingAmount: inv.Outstanding().String(),
		}
	}
	for i, a := range v.Actions {
		out.AllowedActions[i] = CreditNoteAllowedActions(a)
	}
	return out
}

// ListCreditNotes mengembalikan daftar nota kredit.
func (h *SalesHandler) ListCreditNotes(ctx context.Context, req ListCreditNotesRequestObject) (ListCreditNotesResponseObject, error) {
	p := req.Params
	f := postgres.CreditNoteFilter{PartyID: p.PartyId, Search: str(p.Q)}
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, domain.Status(s))
		}
	}
	if p.PartyType != nil {
		f.PartyType = string(*p.PartyType)
	}
	if p.SettlementMethod != nil {
		f.Settlement = domain.SettlementMethod(*p.SettlementMethod)
	}
	res, err := h.returns.ListCreditNotes(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	out := CreditNoteList{Items: make([]CreditNoteSummary, len(res.Items))}
	out.Page, out.PageSize, out.TotalItems, out.TotalPages = pageParams(res.Page, res.Total)
	for i, c := range res.Items {
		out.Items[i] = creditNoteSummary(c)
	}
	return ListCreditNotes200JSONResponse(out), nil
}

// GetCreditNote mengembalikan detail nota kredit.
func (h *SalesHandler) GetCreditNote(ctx context.Context, req GetCreditNoteRequestObject) (GetCreditNoteResponseObject, error) {
	v, err := h.returns.GetCreditNote(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetCreditNote200JSONResponse(toCreditNote(v)), nil
}

// ApplyCreditNote mengalokasikan saldo nota kredit ke invoice.
func (h *SalesHandler) ApplyCreditNote(ctx context.Context, req ApplyCreditNoteRequestObject) (ApplyCreditNoteResponseObject, error) {
	fields := map[string]string{}
	allocs := make([]service.CreditNoteAllocation, len(req.Body.Allocations))
	for i, a := range req.Body.Allocations {
		allocs[i] = service.CreditNoteAllocation{InvoiceID: a.InvoiceId, Amount: parseMoney(a.Amount, fmt.Sprintf("allocations.%d.amount", i), fields)}
	}
	if err := validation(fields); err != nil {
		return nil, err
	}
	v, err := h.returns.ApplyCreditNote(ctx, req.Id, req.Body.Version, allocs)
	if err != nil {
		return nil, err
	}
	return ApplyCreditNote200JSONResponse(toCreditNote(v)), nil
}

// PayCreditNoteRefund mencatat pembayaran refund.
func (h *SalesHandler) PayCreditNoteRefund(ctx context.Context, req PayCreditNoteRefundRequestObject) (PayCreditNoteRefundResponseObject, error) {
	b := req.Body
	v, err := h.returns.PayRefund(ctx, req.Id, b.Version, domain.RefundInput{
		PaymentDate: fromDate(b.PaymentDate), AccountCode: b.AccountCode, Reference: str(b.Reference),
	})
	if err != nil {
		return nil, err
	}
	return PayCreditNoteRefund200JSONResponse(toCreditNote(v)), nil
}
