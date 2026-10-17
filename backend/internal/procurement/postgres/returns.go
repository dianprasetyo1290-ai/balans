package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres/sqlcgen"
)

// ReturnFilter adalah kriteria daftar retur pembelian.
type ReturnFilter struct {
	Status          domain.Status
	Origin          domain.ReturnOrigin
	PurchaseOrderID *uuid.UUID
	SupplierID      *uuid.UUID
	DateFrom        *clock.BusinessDate
	DateTo          *clock.BusinessDate
	Search          string
}

func toReturn(r sqlcgen.ListPurchaseReturnsRow) domain.PurchaseReturn {
	return domain.PurchaseReturn{
		ID: r.ID, Number: r.Number.String, DebitNoteNumber: r.DebitNoteNumber.String, SupplierID: r.SupplierID,
		SupplierName: r.SupplierName, PurchaseOrderID: r.PurchaseOrderID, OrderNumber: r.OrderNumber.String,
		DivisionID: r.DivisionID, Origin: domain.ReturnOrigin(r.Origin), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), SourceLabel: r.SourceLabel, BusinessDate: r.BusinessDate, Reason: r.Reason,
		Note: r.Note, Total: amount(r.TotalAmount), UnbilledAmount: amount(r.UnbilledAmount),
		BilledAmount: amount(r.BilledAmount), Status: domain.Status(r.Status), JournalEntryID: uuidPtr(r.JournalEntryID),
		CancelReason: r.CancelReason, SubmittedAt: r.SubmittedAt, ApprovedAt: r.ApprovedAt, PostedAt: r.PostedAt,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListReturns mengembalikan satu halaman retur (tanpa baris).
func (s *Store) ListReturns(ctx context.Context, f ReturnFilter, page paging.Page) (paging.Result[domain.PurchaseReturn], error) {
	p := sqlcgen.ListPurchaseReturnsParams{
		Status: text(string(f.Status)), Origin: text(string(f.Origin)), PurchaseOrderID: nullUUID(f.PurchaseOrderID),
		SupplierID: nullUUID(f.SupplierID), DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: text(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	}
	rows, err := s.q(ctx).ListPurchaseReturns(ctx, p)
	if err != nil {
		return paging.Result[domain.PurchaseReturn]{}, fmt.Errorf("list purchase returns: %w", err)
	}
	total, err := s.q(ctx).CountPurchaseReturns(ctx, sqlcgen.CountPurchaseReturnsParams{
		Status: p.Status, Origin: p.Origin, PurchaseOrderID: p.PurchaseOrderID, SupplierID: p.SupplierID,
		DateFrom: p.DateFrom, DateTo: p.DateTo, Search: p.Search,
	})
	if err != nil {
		return paging.Result[domain.PurchaseReturn]{}, fmt.Errorf("count purchase returns: %w", err)
	}
	items := make([]domain.PurchaseReturn, len(rows))
	for i, r := range rows {
		items[i] = toReturn(r)
	}
	return paging.Result[domain.PurchaseReturn]{Items: items, Page: page, Total: total}, nil
}

// GetReturn membaca retur beserta baris; forUpdate mengunci retur.
func (s *Store) GetReturn(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PurchaseReturn, error) {
	q := s.q(ctx)
	if forUpdate {
		if _, err := q.LockPurchaseReturn(ctx, id); err != nil {
			return domain.PurchaseReturn{}, notFound(err, "lock purchase return")
		}
	}
	row, err := q.GetPurchaseReturn(ctx, id)
	if err != nil {
		return domain.PurchaseReturn{}, notFound(err, "get purchase return")
	}
	r := toReturn(sqlcgen.ListPurchaseReturnsRow(row))
	lines, err := q.ListPurchaseReturnLines(ctx, id)
	if err != nil {
		return domain.PurchaseReturn{}, fmt.Errorf("list purchase return lines: %w", err)
	}
	for _, l := range lines {
		r.Lines = append(r.Lines, domain.ReturnLine{
			ID: l.ID, LineNo: int(l.LineNo), ReceiptLineID: l.GoodsReceiptLineID, ProductID: l.ProductID,
			BatchID: uuidPtr(l.BatchID), LocationID: l.LocationID, StockStatus: l.StockStatus, Qty: l.Qty,
			UnitCost: l.UnitCost, Amount: amount(l.Amount), UnbilledQty: l.UnbilledQty, UnbilledAmount: amount(l.UnbilledAmount),
			InventoryAccount: l.InventoryAccountCode, Note: l.Note,
		})
	}
	return r, nil
}

func (s *Store) insertReturnLines(ctx context.Context, r domain.PurchaseReturn) error {
	for _, l := range r.Lines {
		if err := s.q(ctx).InsertPurchaseReturnLine(ctx, sqlcgen.InsertPurchaseReturnLineParams{
			PurchaseReturnID: r.ID, LineNo: int32Of(l.LineNo), GoodsReceiptLineID: l.ReceiptLineID, ProductID: l.ProductID,
			BatchID: nullUUID(l.BatchID), LocationID: l.LocationID, StockStatus: l.StockStatus, Qty: l.Qty,
			UnitCost: l.UnitCost, Amount: l.Amount.Decimal(), UnbilledQty: l.UnbilledQty,
			UnbilledAmount: l.UnbilledAmount.Decimal(), InventoryAccountCode: l.InventoryAccount, Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert purchase return line: %w", err)
		}
	}
	return nil
}

// InsertReturn menyimpan draft retur beserta baris.
func (s *Store) InsertReturn(ctx context.Context, r domain.PurchaseReturn, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPurchaseReturn(ctx, sqlcgen.InsertPurchaseReturnParams{
		SupplierID: r.SupplierID, SupplierName: r.SupplierName, PurchaseOrderID: r.PurchaseOrderID, DivisionID: r.DivisionID,
		Origin: string(r.Origin), SourceType: r.SourceType, SourceID: nullUUID(r.SourceID), SourceLabel: r.SourceLabel,
		BusinessDate: r.BusinessDate, Reason: r.Reason, Note: r.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert purchase return: %w", err)
	}
	r.ID = id
	return id, s.insertReturnLines(ctx, r)
}

// SaveReturn menyimpan header retur (cek versi) dan mengganti baris (hasil hitung HPP ikut tersimpan).
func (s *Store) SaveReturn(ctx context.Context, r domain.PurchaseReturn, version int, actorID uuid.UUID) error {
	q := s.q(ctx)
	n, err := q.UpdatePurchaseReturn(ctx, sqlcgen.UpdatePurchaseReturnParams{
		Number: text(r.Number), DebitNoteNumber: text(r.DebitNoteNumber), BusinessDate: r.BusinessDate, Reason: r.Reason,
		Note: r.Note, TotalAmount: r.Total.Decimal(), UnbilledAmount: r.UnbilledAmount.Decimal(),
		BilledAmount: r.BilledAmount.Decimal(), Status: string(r.Status), JournalEntryID: nullUUID(r.JournalEntryID),
		CancelReason: r.CancelReason, SubmittedAt: r.SubmittedAt, ApprovedAt: r.ApprovedAt, PostedAt: r.PostedAt,
		ActorID: actorID, ID: r.ID, Version: int32Of(version),
	})
	if err != nil {
		return fmt.Errorf("update purchase return: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if err := q.DeletePurchaseReturnLines(ctx, r.ID); err != nil {
		return fmt.Errorf("delete purchase return lines: %w", err)
	}
	return s.insertReturnLines(ctx, r)
}

// PendingReturnQty menjumlah qty baris GR di retur lain yang belum diposting.
func (s *Store) PendingReturnQty(ctx context.Context, lineIDs []uuid.UUID, except uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	out := map[uuid.UUID]decimal.Decimal{}
	if len(lineIDs) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).PendingReturnQtyByReceiptLines(ctx, sqlcgen.PendingReturnQtyByReceiptLinesParams{ExceptID: except, Ids: lineIDs})
	if err != nil {
		return nil, fmt.Errorf("pending return qty: %w", err)
	}
	for _, r := range rows {
		out[r.GoodsReceiptLineID] = r.Qty
	}
	return out, nil
}
