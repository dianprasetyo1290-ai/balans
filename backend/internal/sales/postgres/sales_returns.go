package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

// --- Retur Penjualan --------------------------------------------------------------------------

// ReturnFilter adalah kriteria daftar Retur Penjualan.
type ReturnFilter struct {
	Statuses     []domain.Status
	OrderID      *uuid.UUID
	PartyID      *uuid.UUID
	PickupUserID *uuid.UUID
	Search       string
}

func toReturn(r sqlcgen.SalesReturn, orderNumber, invoiceNumber string) domain.SalesReturn {
	return domain.SalesReturn{
		ID: r.ID, Number: r.Number.String, OrderID: r.SalesOrderID, OrderNumber: orderNumber,
		OrderType: domain.OrderType(r.OrderType), InvoiceID: uuidPtr(r.InvoiceID), InvoiceNumber: invoiceNumber,
		ChannelID: r.ChannelID, SubChannelID: r.SubChannelID, PartyType: r.PartyType, PartyID: uuidPtr(r.PartyID),
		PartyName: r.PartyName, SourceLocationID: r.SourceLocationID, ReturnDate: r.ReturnDate,
		Settlement: domain.SettlementMethod(r.SettlementMethod), RequiresVerification: r.RequiresVerification,
		Note: r.Note, SendBackNote: r.SendBackNote, VerifiedBy: uuidPtr(r.VerifiedBy), VerifiedAt: r.VerifiedAt,
		VerifyNote: r.VerifyNote, PickupRequired: r.PickupRequired, PickupUserID: uuidPtr(r.PickupUserID),
		PickupAssignedAt: r.PickupAssignedAt, PickedUpAt: r.PickedUpAt, PickupNote: r.PickupNote,
		PickupFileIDs: r.PickupFileIds, ReceivedBy: uuidPtr(r.ReceivedBy), ReceivedAt: r.ReceivedAt,
		ReceiveNote: r.ReceiveNote, DocumentationFileIDs: r.DocumentationFileIds, InboundID: uuidPtr(r.InboundID),
		InboundNumber: r.InboundNumber, Amount: amount(r.ReturnAmount), Cost: amount(r.CostAmount),
		Status: domain.Status(r.Status), CancelReason: r.CancelReason, PostedAt: r.PostedAt,
		JournalEntryID: uuidPtr(r.JournalEntryID), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (f ReturnFilter) statuses() []string {
	return statusesOf(f.Statuses)
}

// ListReturns mengembalikan daftar retur tanpa baris, terbaru lebih dulu.
func (s *Store) ListReturns(ctx context.Context, f ReturnFilter, page paging.Page) (paging.Result[domain.SalesReturn], error) {
	rows, err := s.q(ctx).ListSalesReturns(ctx, sqlcgen.ListSalesReturnsParams{
		Statuses: f.statuses(), SalesOrderID: nullUUID(f.OrderID), PartyID: nullUUID(f.PartyID),
		PickupUserID: nullUUID(f.PickupUserID), Search: text(f.Search), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.SalesReturn]{}, fmt.Errorf("list sales returns: %w", err)
	}
	total, err := s.q(ctx).CountSalesReturns(ctx, sqlcgen.CountSalesReturnsParams{
		Statuses: f.statuses(), SalesOrderID: nullUUID(f.OrderID), PartyID: nullUUID(f.PartyID),
		PickupUserID: nullUUID(f.PickupUserID), Search: text(f.Search),
	})
	if err != nil {
		return paging.Result[domain.SalesReturn]{}, fmt.Errorf("count sales returns: %w", err)
	}
	items := make([]domain.SalesReturn, len(rows))
	for i, r := range rows {
		items[i] = toReturn(r.SalesReturn, r.OrderNumber, r.InvoiceNumber)
	}
	return paging.Result[domain.SalesReturn]{Items: items, Page: page, Total: total}, nil
}

// GetReturn membaca retur beserta baris dan item; forUpdate mengunci baris retur.
func (s *Store) GetReturn(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.SalesReturn, error) {
	var r domain.SalesReturn
	if forUpdate {
		row, err := s.q(ctx).GetSalesReturnForUpdate(ctx, id)
		if err != nil {
			return domain.SalesReturn{}, notFound(err, "lock sales return")
		}
		r = toReturn(row.SalesReturn, row.OrderNumber, row.InvoiceNumber)
	} else {
		row, err := s.q(ctx).GetSalesReturn(ctx, id)
		if err != nil {
			return domain.SalesReturn{}, notFound(err, "get sales return")
		}
		r = toReturn(row.SalesReturn, row.OrderNumber, row.InvoiceNumber)
	}
	lines, err := s.q(ctx).ListSalesReturnLines(ctx, id)
	if err != nil {
		return domain.SalesReturn{}, fmt.Errorf("list sales return lines: %w", err)
	}
	lineNo := map[uuid.UUID]int{}
	for _, l := range lines {
		lineNo[l.ID] = int(l.LineNo)
		r.Lines = append(r.Lines, domain.ReturnLine{
			ID: l.ID, LineNo: int(l.LineNo), OrderLineID: l.SalesOrderLineID, ProductID: l.ProductID,
			BatchID: uuidPtr(l.BatchID), Qty: l.Qty, ReasonID: l.ReturnReasonID, UnitPrice: amount(l.UnitPriceAmount),
			Amount: amount(l.LineAmount), UnitCost: l.OriginalUnitCost, Cost: amount(l.CostAmount), Note: l.Note,
		})
	}
	items, err := s.q(ctx).ListSalesReturnItems(ctx, id)
	if err != nil {
		return domain.SalesReturn{}, fmt.Errorf("list sales return items: %w", err)
	}
	for _, it := range items {
		r.Items = append(r.Items, domain.ReturnItem{
			ID: it.ID, LineID: it.SalesReturnLineID, LineNo: lineNo[it.SalesReturnLineID], AllocationID: it.AllocationID,
			ProductID: it.ProductID, BatchID: uuidPtr(it.BatchID), Qty: it.Qty, UnitCost: it.UnitCost,
			Cost: amount(it.CostAmount), PassedQty: it.PassedQty, RejectedQty: it.RejectedQty, QCDone: it.QcDone,
		})
	}
	return r, nil
}

// InsertReturn menyimpan draft retur beserta baris & item.
func (s *Store) InsertReturn(ctx context.Context, r domain.SalesReturn, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertSalesReturn(ctx, sqlcgen.InsertSalesReturnParams{
		SalesOrderID: r.OrderID, InvoiceID: nullUUID(r.InvoiceID), OrderType: string(r.OrderType), ChannelID: r.ChannelID,
		SubChannelID: r.SubChannelID, PartyType: r.PartyType, PartyID: nullUUID(r.PartyID), PartyName: r.PartyName,
		SourceLocationID: r.SourceLocationID, ReturnDate: r.ReturnDate, SettlementMethod: string(r.Settlement),
		RequiresVerification: r.RequiresVerification, Note: r.Note, ReturnAmount: r.Amount.Decimal(),
		CostAmount: r.Cost.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert sales return: %w", err)
	}
	r.ID = id
	return id, s.insertReturnLines(ctx, r)
}

func (s *Store) insertReturnLines(ctx context.Context, r domain.SalesReturn) error {
	ids := map[int]uuid.UUID{}
	for _, l := range r.Lines {
		id, err := s.q(ctx).InsertSalesReturnLine(ctx, sqlcgen.InsertSalesReturnLineParams{
			SalesReturnID: r.ID, LineNo: int32(l.LineNo), SalesOrderLineID: l.OrderLineID, ProductID: l.ProductID, //nolint:gosec // ≤ 200
			BatchID: nullUUID(l.BatchID), Qty: l.Qty, ReturnReasonID: l.ReasonID, UnitPriceAmount: l.UnitPrice.Decimal(),
			LineAmount: l.Amount.Decimal(), OriginalUnitCost: l.UnitCost, CostAmount: l.Cost.Decimal(), Note: l.Note,
		})
		if err != nil {
			return fmt.Errorf("insert sales return line: %w", err)
		}
		ids[l.LineNo] = id
	}
	for _, it := range r.Items {
		if _, err := s.q(ctx).InsertSalesReturnItem(ctx, sqlcgen.InsertSalesReturnItemParams{
			SalesReturnID: r.ID, SalesReturnLineID: ids[it.LineNo], AllocationID: it.AllocationID, ProductID: it.ProductID,
			BatchID: nullUUID(it.BatchID), Qty: it.Qty, UnitCost: it.UnitCost, CostAmount: it.Cost.Decimal(),
		}); err != nil {
			return fmt.Errorf("insert sales return item: %w", err)
		}
	}
	return nil
}

func ts(d *clock.BusinessDate) pgtype.Date { return pgDate(d) }

// SaveReturn menyimpan header retur dengan optimistic locking. replaceLines menulis ulang baris & item (draft);
// selain itu qty baris dan item (cek ulang, hasil QC) diperbarui.
func (s *Store) SaveReturn(ctx context.Context, r domain.SalesReturn, ver int, actorID uuid.UUID, replaceLines bool) error {
	number := pgtype.Text{String: r.Number, Valid: r.Number != ""}
	n, err := s.q(ctx).UpdateSalesReturn(ctx, sqlcgen.UpdateSalesReturnParams{
		Number: number, InvoiceID: nullUUID(r.InvoiceID), ReturnDate: r.ReturnDate, SettlementMethod: string(r.Settlement),
		Note: r.Note, SendBackNote: r.SendBackNote, VerifiedBy: nullUUID(r.VerifiedBy), VerifiedAt: r.VerifiedAt,
		VerifyNote: r.VerifyNote, PickupRequired: r.PickupRequired, PickupUserID: nullUUID(r.PickupUserID),
		PickupAssignedAt: r.PickupAssignedAt, PickedUpAt: r.PickedUpAt, PickupNote: r.PickupNote,
		PickupFileIds: nonNilIDs(r.PickupFileIDs), ReceivedBy: nullUUID(r.ReceivedBy), ReceivedAt: r.ReceivedAt,
		ReceiveNote: r.ReceiveNote, DocumentationFileIds: nonNilIDs(r.DocumentationFileIDs),
		InboundID: nullUUID(r.InboundID), InboundNumber: r.InboundNumber, ReturnAmount: r.Amount.Decimal(),
		CostAmount: r.Cost.Decimal(), Status: string(r.Status), CancelReason: r.CancelReason, PostedAt: r.PostedAt,
		JournalEntryID: nullUUID(r.JournalEntryID), ActorID: actorID, ID: r.ID, Version: version(ver),
	})
	if err != nil {
		return fmt.Errorf("update sales return: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if replaceLines {
		if err := s.q(ctx).DeleteSalesReturnItems(ctx, r.ID); err != nil {
			return fmt.Errorf("delete sales return items: %w", err)
		}
		if err := s.q(ctx).DeleteSalesReturnLines(ctx, r.ID); err != nil {
			return fmt.Errorf("delete sales return lines: %w", err)
		}
		return s.insertReturnLines(ctx, r)
	}
	for _, l := range r.Lines {
		if err := s.q(ctx).UpdateSalesReturnLineQty(ctx, sqlcgen.UpdateSalesReturnLineQtyParams{
			Qty: l.Qty, LineAmount: l.Amount.Decimal(), CostAmount: l.Cost.Decimal(), ID: l.ID,
		}); err != nil {
			return fmt.Errorf("update sales return line: %w", err)
		}
	}
	for _, it := range r.Items {
		if err := s.q(ctx).UpdateSalesReturnItem(ctx, sqlcgen.UpdateSalesReturnItemParams{
			Qty: it.Qty, CostAmount: it.Cost.Decimal(), PassedQty: it.PassedQty, RejectedQty: it.RejectedQty,
			QcDone: it.QCDone, ID: it.ID,
		}); err != nil {
			return fmt.Errorf("update sales return item: %w", err)
		}
	}
	return nil
}

func nonNilIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// ReturnedQty menjumlahkan qty retur (belum dibatalkan) per baris Sales Order, selain retur excludeID.
func (s *Store) ReturnedQty(ctx context.Context, orderID, excludeID uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	rows, err := s.q(ctx).ReturnedQtyByOrderLine(ctx, sqlcgen.ReturnedQtyByOrderLineParams{SalesOrderID: orderID, ExcludeID: excludeID})
	if err != nil {
		return nil, fmt.Errorf("returned qty: %w", err)
	}
	out := make(map[uuid.UUID]decimal.Decimal, len(rows))
	for _, r := range rows {
		out[r.SalesOrderLineID] = r.Qty
	}
	return out, nil
}

// ReturnSource adalah Sales Order yang bisa diretur (sudah ada barang terkirim).
type ReturnSource struct {
	ID           uuid.UUID
	Number       string
	Type         domain.OrderType
	PartyName    string
	BusinessDate clock.BusinessDate
	Status       domain.Status
	Total        money.Amount
}

// ReturnSources mencari Sales Order sumber retur (maks. 20).
func (s *Store) ReturnSources(ctx context.Context, search string) ([]ReturnSource, error) {
	rows, err := s.q(ctx).ReturnSourceOrders(ctx, text(search))
	if err != nil {
		return nil, fmt.Errorf("return sources: %w", err)
	}
	out := make([]ReturnSource, len(rows))
	for i, r := range rows {
		out[i] = ReturnSource{
			ID: r.ID, Number: r.Number, Type: domain.OrderType(r.OrderType), PartyName: r.PartyName,
			BusinessDate: r.BusinessDate, Status: domain.Status(r.Status), Total: amount(r.TotalAmount),
		}
	}
	return out, nil
}

// --- Nota Kredit -----------------------------------------------------------------------------

// CreditNoteFilter adalah kriteria daftar nota kredit.
type CreditNoteFilter struct {
	Statuses   []domain.Status
	PartyType  string
	PartyID    *uuid.UUID
	Settlement domain.SettlementMethod
	Search     string
}

func toCreditNote(c sqlcgen.CreditNote, returnNumber, invoiceNumber string) domain.CreditNote {
	out := domain.CreditNote{
		ID: c.ID, Number: c.Number, ReturnID: c.SalesReturnID, ReturnNumber: returnNumber, InvoiceID: uuidPtr(c.InvoiceID),
		InvoiceNumber: invoiceNumber, PartyType: c.PartyType, PartyID: c.PartyID, PartyName: c.PartyName,
		ChannelID: c.ChannelID, SubChannelID: c.SubChannelID, Settlement: domain.SettlementMethod(c.SettlementMethod),
		IssueDate: c.IssueDate, Amount: amount(c.Amount), Applied: amount(c.AppliedAmount),
		Refunded: amount(c.RefundedAmount), Status: domain.Status(c.Status), DecisionNote: c.DecisionNote,
		RefundAccount: c.RefundAccountCode, RefundReference: c.RefundReference,
		RefundJournalID: uuidPtr(c.RefundJournalEntryID), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Version: int(c.Version),
	}
	if c.RefundDate.Valid {
		d := clock.NewBusinessDate(c.RefundDate.Time.Year(), c.RefundDate.Time.Month(), c.RefundDate.Time.Day())
		out.RefundDate = &d
	}
	return out
}

func (f CreditNoteFilter) settlement() pgtype.Text { return text(string(f.Settlement)) }

// ListCreditNotes mengembalikan daftar nota kredit terbaru lebih dulu.
func (s *Store) ListCreditNotes(ctx context.Context, f CreditNoteFilter, page paging.Page) (paging.Result[domain.CreditNote], error) {
	rows, err := s.q(ctx).ListCreditNotes(ctx, sqlcgen.ListCreditNotesParams{
		Statuses: statusesOf(f.Statuses), PartyType: text(f.PartyType), PartyID: nullUUID(f.PartyID),
		Settlement: f.settlement(), Search: text(f.Search), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.CreditNote]{}, fmt.Errorf("list credit notes: %w", err)
	}
	total, err := s.q(ctx).CountCreditNotes(ctx, sqlcgen.CountCreditNotesParams{
		Statuses: statusesOf(f.Statuses), PartyType: text(f.PartyType), PartyID: nullUUID(f.PartyID),
		Settlement: f.settlement(), Search: text(f.Search),
	})
	if err != nil {
		return paging.Result[domain.CreditNote]{}, fmt.Errorf("count credit notes: %w", err)
	}
	items := make([]domain.CreditNote, len(rows))
	for i, r := range rows {
		items[i] = toCreditNote(r.CreditNote, r.ReturnNumber, r.InvoiceNumber)
	}
	return paging.Result[domain.CreditNote]{Items: items, Page: page, Total: total}, nil
}

// GetCreditNote membaca nota kredit beserta alokasinya; forUpdate mengunci baris.
func (s *Store) GetCreditNote(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.CreditNote, error) {
	var c domain.CreditNote
	if forUpdate {
		row, err := s.q(ctx).GetCreditNoteForUpdate(ctx, id)
		if err != nil {
			return domain.CreditNote{}, notFound(err, "lock credit note")
		}
		c = toCreditNote(row.CreditNote, row.ReturnNumber, row.InvoiceNumber)
	} else {
		row, err := s.q(ctx).GetCreditNote(ctx, id)
		if err != nil {
			return domain.CreditNote{}, notFound(err, "get credit note")
		}
		c = toCreditNote(row.CreditNote, row.ReturnNumber, row.InvoiceNumber)
	}
	apps, err := s.q(ctx).ListCreditNoteApplications(ctx, id)
	if err != nil {
		return domain.CreditNote{}, fmt.Errorf("list credit note applications: %w", err)
	}
	for _, a := range apps {
		c.Applications = append(c.Applications, domain.CreditNoteApplication{
			ID: a.ID, InvoiceID: a.InvoiceID, InvoiceNumber: a.InvoiceNumber, Amount: amount(a.Amount),
			AppliedDate: a.AppliedDate, CreatedAt: a.CreatedAt,
		})
	}
	return c, nil
}

// CreditNoteIDByReturn mengembalikan ID nota kredit retur; ok false bila belum ada.
func (s *Store) CreditNoteIDByReturn(ctx context.Context, returnID uuid.UUID) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).CreditNoteIDByReturn(ctx, returnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("credit note by return: %w", err)
	}
	return id, true, nil
}

// InsertCreditNote menyimpan nota kredit baru.
func (s *Store) InsertCreditNote(ctx context.Context, c domain.CreditNote, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertCreditNote(ctx, sqlcgen.InsertCreditNoteParams{
		Number: c.Number, SalesReturnID: c.ReturnID, InvoiceID: nullUUID(c.InvoiceID), PartyType: c.PartyType,
		PartyID: c.PartyID, PartyName: c.PartyName, ChannelID: c.ChannelID, SubChannelID: c.SubChannelID,
		SettlementMethod: string(c.Settlement), IssueDate: c.IssueDate, Amount: c.Amount.Decimal(),
		Status: string(c.Status), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert credit note: %w", err)
	}
	return id, nil
}

// SaveCreditNote menyimpan status, saldo, dan refund nota kredit dengan optimistic locking.
func (s *Store) SaveCreditNote(ctx context.Context, c domain.CreditNote, ver int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateCreditNote(ctx, sqlcgen.UpdateCreditNoteParams{
		SettlementMethod: string(c.Settlement), AppliedAmount: c.Applied.Decimal(), RefundedAmount: c.Refunded.Decimal(),
		Status: string(c.Status), DecisionNote: c.DecisionNote, RefundDate: ts(c.RefundDate),
		RefundAccountCode: c.RefundAccount, RefundReference: c.RefundReference,
		RefundJournalEntryID: nullUUID(c.RefundJournalID), ActorID: actorID, ID: c.ID, Version: version(ver),
	})
	if err != nil {
		return fmt.Errorf("update credit note: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// InsertCreditNoteApplication mencatat alokasi nota kredit ke invoice.
func (s *Store) InsertCreditNoteApplication(ctx context.Context, creditNoteID uuid.UUID, a domain.CreditNoteApplication, actorID uuid.UUID) error {
	if err := s.q(ctx).InsertCreditNoteApplication(ctx, sqlcgen.InsertCreditNoteApplicationParams{
		CreditNoteID: creditNoteID, InvoiceID: a.InvoiceID, Amount: a.Amount.Decimal(), AppliedDate: a.AppliedDate, ActorID: actorID,
	}); err != nil {
		return fmt.Errorf("insert credit note application: %w", err)
	}
	return nil
}
