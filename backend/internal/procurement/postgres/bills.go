package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres/sqlcgen"
)

// BillFilter adalah kriteria daftar tagihan pemasok.
type BillFilter struct {
	Status          domain.Status
	MatchStatus     domain.MatchStatus
	PurchaseOrderID *uuid.UUID
	SupplierID      *uuid.UUID
	DateFrom        *clock.BusinessDate
	DateTo          *clock.BusinessDate
	Search          string
}

func toBill(r sqlcgen.ListSupplierBillsRow) domain.SupplierBill {
	return domain.SupplierBill{
		ID: r.ID, Number: r.Number, SupplierID: r.SupplierID, SupplierName: r.SupplierName,
		PurchaseOrderID: r.PurchaseOrderID, OrderNumber: r.OrderNumber.String, DivisionID: r.DivisionID,
		BillDate: r.BillDate, DueDate: r.DueDate, Note: r.Note, Subtotal: amount(r.SubtotalAmount),
		GRNIAmount: amount(r.GrniAmount), PriceVariance: amount(r.PriceVarianceAmount),
		WithholdingType: domain.WithholdingType(r.WithholdingType), WithholdingRate: r.WithholdingRate,
		WithholdingAmount: amount(r.WithholdingAmount), AdvanceApplied: amount(r.AdvanceAppliedAmount),
		TotalPayable: amount(r.TotalPayableAmount), PaidAmount: amount(r.PaidAmount),
		MatchStatus: domain.MatchStatus(r.MatchStatus), Status: domain.Status(r.Status),
		JournalEntryID: uuidPtr(r.JournalEntryID), AdvanceJournalEntryID: uuidPtr(r.AdvanceJournalEntryID),
		CancelReason: r.CancelReason, SubmittedAt: r.SubmittedAt, PostedAt: r.PostedAt, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListBills mengembalikan satu halaman tagihan (tanpa baris).
func (s *Store) ListBills(ctx context.Context, f BillFilter, page paging.Page) (paging.Result[domain.SupplierBill], error) {
	p := sqlcgen.ListSupplierBillsParams{
		Status: text(string(f.Status)), MatchStatus: text(string(f.MatchStatus)), PurchaseOrderID: nullUUID(f.PurchaseOrderID),
		SupplierID: nullUUID(f.SupplierID), DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: text(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	}
	rows, err := s.q(ctx).ListSupplierBills(ctx, p)
	if err != nil {
		return paging.Result[domain.SupplierBill]{}, fmt.Errorf("list supplier bills: %w", err)
	}
	total, err := s.q(ctx).CountSupplierBills(ctx, sqlcgen.CountSupplierBillsParams{
		Status: p.Status, MatchStatus: p.MatchStatus, PurchaseOrderID: p.PurchaseOrderID, SupplierID: p.SupplierID,
		DateFrom: p.DateFrom, DateTo: p.DateTo, Search: p.Search,
	})
	if err != nil {
		return paging.Result[domain.SupplierBill]{}, fmt.Errorf("count supplier bills: %w", err)
	}
	items := make([]domain.SupplierBill, len(rows))
	for i, r := range rows {
		items[i] = toBill(r)
	}
	return paging.Result[domain.SupplierBill]{Items: items, Page: page, Total: total}, nil
}

// GetBill membaca tagihan beserta baris dan potongan uang muka; forUpdate mengunci tagihan.
func (s *Store) GetBill(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.SupplierBill, error) {
	q := s.q(ctx)
	if forUpdate {
		if _, err := q.LockSupplierBill(ctx, id); err != nil {
			return domain.SupplierBill{}, notFound(err, "lock supplier bill")
		}
	}
	row, err := q.GetSupplierBill(ctx, id)
	if err != nil {
		return domain.SupplierBill{}, notFound(err, "get supplier bill")
	}
	b := toBill(sqlcgen.ListSupplierBillsRow(row))
	lines, err := q.ListSupplierBillLines(ctx, id)
	if err != nil {
		return domain.SupplierBill{}, fmt.Errorf("list supplier bill lines: %w", err)
	}
	for _, l := range lines {
		b.Lines = append(b.Lines, domain.BillLine{
			ID: l.ID, LineNo: int(l.LineNo), ReceiptLineID: l.GoodsReceiptLineID, OrderLineID: l.PurchaseOrderLineID,
			ProductID: l.ProductID, Qty: l.Qty, ReceiptOpenQty: l.ReceiptOpenQty, UnitPrice: amount(l.UnitPriceAmount),
			POUnitPrice: amount(l.PoUnitPriceAmount), Amount: amount(l.LineAmount), GRNIAmount: amount(l.GrniAmount),
			PriceVariance: amount(l.PriceVarianceAmount), ExceedsTolerance: l.ExceedsTolerance, Note: l.Note,
		})
	}
	advances, err := q.ListSupplierBillAdvances(ctx, id)
	if err != nil {
		return domain.SupplierBill{}, fmt.Errorf("list supplier bill advances: %w", err)
	}
	for _, a := range advances {
		b.Advances = append(b.Advances, domain.BillAdvance{AdvanceID: a.PurchaseAdvanceID, AdvanceNumber: a.AdvanceNumber.String, Amount: amount(a.Amount)})
	}
	return b, nil
}

func (s *Store) insertBillLines(ctx context.Context, b domain.SupplierBill) error {
	for _, l := range b.Lines {
		if err := s.q(ctx).InsertSupplierBillLine(ctx, sqlcgen.InsertSupplierBillLineParams{
			SupplierBillID: b.ID, LineNo: int32Of(l.LineNo), GoodsReceiptLineID: l.ReceiptLineID,
			PurchaseOrderLineID: l.OrderLineID, ProductID: l.ProductID, Qty: l.Qty, ReceiptOpenQty: l.ReceiptOpenQty,
			UnitPriceAmount: l.UnitPrice.Decimal(), PoUnitPriceAmount: l.POUnitPrice.Decimal(), LineAmount: l.Amount.Decimal(),
			GrniAmount: l.GRNIAmount.Decimal(), PriceVarianceAmount: l.PriceVariance.Decimal(),
			ExceedsTolerance: l.ExceedsTolerance, Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert supplier bill line: %w", err)
		}
	}
	return nil
}

// InsertBill menyimpan draft tagihan beserta baris.
func (s *Store) InsertBill(ctx context.Context, b domain.SupplierBill, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertSupplierBill(ctx, sqlcgen.InsertSupplierBillParams{
		Number: b.Number, SupplierID: b.SupplierID, PurchaseOrderID: b.PurchaseOrderID, DivisionID: b.DivisionID,
		BillDate: b.BillDate, DueDate: b.DueDate, Note: b.Note, SubtotalAmount: b.Subtotal.Decimal(),
		GrniAmount: b.GRNIAmount.Decimal(), PriceVarianceAmount: b.PriceVariance.Decimal(),
		WithholdingType: string(b.WithholdingType), WithholdingRate: b.WithholdingRate,
		WithholdingAmount: b.WithholdingAmount.Decimal(), AdvanceAppliedAmount: b.AdvanceApplied.Decimal(),
		TotalPayableAmount: b.TotalPayable.Decimal(), MatchStatus: string(b.MatchStatus), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert supplier bill: %w", err)
	}
	b.ID = id
	return id, s.insertBillLines(ctx, b)
}

// SaveBill menyimpan header tagihan (cek versi) dan mengganti baris bila replaceLines.
func (s *Store) SaveBill(ctx context.Context, b domain.SupplierBill, version int, actorID uuid.UUID, replaceLines bool) error {
	q := s.q(ctx)
	n, err := q.UpdateSupplierBill(ctx, sqlcgen.UpdateSupplierBillParams{
		Number: b.Number, BillDate: b.BillDate, DueDate: b.DueDate, Note: b.Note, SubtotalAmount: b.Subtotal.Decimal(),
		GrniAmount: b.GRNIAmount.Decimal(), PriceVarianceAmount: b.PriceVariance.Decimal(),
		WithholdingType: string(b.WithholdingType), WithholdingRate: b.WithholdingRate,
		WithholdingAmount: b.WithholdingAmount.Decimal(), AdvanceAppliedAmount: b.AdvanceApplied.Decimal(),
		TotalPayableAmount: b.TotalPayable.Decimal(), PaidAmount: b.PaidAmount.Decimal(), MatchStatus: string(b.MatchStatus),
		Status: string(b.Status), JournalEntryID: nullUUID(b.JournalEntryID), AdvanceJournalEntryID: nullUUID(b.AdvanceJournalEntryID),
		CancelReason: b.CancelReason, SubmittedAt: b.SubmittedAt, PostedAt: b.PostedAt, ActorID: actorID, ID: b.ID,
		Version: int32Of(version),
	})
	if err != nil {
		return fmt.Errorf("update supplier bill: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if !replaceLines {
		return nil
	}
	if err := q.DeleteSupplierBillLines(ctx, b.ID); err != nil {
		return fmt.Errorf("delete supplier bill lines: %w", err)
	}
	return s.insertBillLines(ctx, b)
}

// InsertBillAdvances menyimpan potongan uang muka tagihan.
func (s *Store) InsertBillAdvances(ctx context.Context, billID uuid.UUID, list []domain.BillAdvance) error {
	for _, a := range list {
		if err := s.q(ctx).InsertSupplierBillAdvance(ctx, sqlcgen.InsertSupplierBillAdvanceParams{
			SupplierBillID: billID, PurchaseAdvanceID: a.AdvanceID, Amount: a.Amount.Decimal(),
		}); err != nil {
			return fmt.Errorf("insert supplier bill advance: %w", err)
		}
	}
	return nil
}

// BillNumberTaken memeriksa nomor tagihan sudah dipakai tagihan aktif lain pemasok yang sama.
func (s *Store) BillNumberTaken(ctx context.Context, supplierID uuid.UUID, number string, except uuid.UUID) (bool, error) {
	taken, err := s.q(ctx).SupplierBillNumberTaken(ctx, sqlcgen.SupplierBillNumberTakenParams{
		SupplierID: supplierID, Number: number, ExceptID: except,
	})
	if err != nil {
		return false, fmt.Errorf("supplier bill number taken: %w", err)
	}
	return taken, nil
}

// PendingBillQty menjumlah qty baris GR di tagihan lain yang menunggu persetujuan.
func (s *Store) PendingBillQty(ctx context.Context, lineIDs []uuid.UUID, except uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	out := map[uuid.UUID]decimal.Decimal{}
	if len(lineIDs) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).PendingBillQtyByReceiptLines(ctx, sqlcgen.PendingBillQtyByReceiptLinesParams{ExceptID: except, Ids: lineIDs})
	if err != nil {
		return nil, fmt.Errorf("pending bill qty: %w", err)
	}
	for _, r := range rows {
		out[r.GoodsReceiptLineID] = r.Qty
	}
	return out, nil
}

// --- utang pemasok ---------------------------------------------------------------------------

func toPayable(r sqlcgen.SupplierPayable) domain.Payable {
	return domain.Payable{
		ID: r.ID, DocumentType: domain.PayableDocument(r.DocumentType), DocumentID: r.DocumentID,
		DocumentNumber: r.DocumentNumber, SupplierID: r.SupplierID, PurchaseOrderID: r.PurchaseOrderID,
		DivisionID: r.DivisionID, BusinessDate: r.BusinessDate, DueDate: r.DueDate, Amount: amount(r.Amount),
		OpenAmount: amount(r.OpenAmount), Version: int(r.Version),
	}
}

// InsertPayable mencatat item utang pemasok.
func (s *Store) InsertPayable(ctx context.Context, p domain.Payable, actorID uuid.UUID) error {
	if err := s.q(ctx).InsertSupplierPayable(ctx, sqlcgen.InsertSupplierPayableParams{
		DocumentType: string(p.DocumentType), DocumentID: p.DocumentID, DocumentNumber: p.DocumentNumber,
		SupplierID: p.SupplierID, PurchaseOrderID: p.PurchaseOrderID, DivisionID: p.DivisionID,
		BusinessDate: p.BusinessDate, DueDate: p.DueDate, Amount: p.Amount.Decimal(), OpenAmount: p.OpenAmount.Decimal(),
		ActorID: actorID,
	}); err != nil {
		return fmt.Errorf("insert supplier payable: %w", err)
	}
	return nil
}

// OpenPayables mengembalikan utang terbuka pemasok urut jatuh tempo.
func (s *Store) OpenPayables(ctx context.Context, supplierID uuid.UUID) ([]domain.Payable, error) {
	rows, err := s.q(ctx).OpenSupplierPayables(ctx, supplierID)
	if err != nil {
		return nil, fmt.Errorf("open supplier payables: %w", err)
	}
	out := make([]domain.Payable, len(rows))
	for i, r := range rows {
		out[i] = toPayable(r)
	}
	return out, nil
}

// GetPayableForUpdate mengunci satu item utang.
func (s *Store) GetPayableForUpdate(ctx context.Context, id uuid.UUID) (domain.Payable, error) {
	r, err := s.q(ctx).GetSupplierPayableForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payable{}, apperr.Wrap(apperr.CodeNotFound, err)
	}
	if err != nil {
		return domain.Payable{}, fmt.Errorf("get supplier payable: %w", err)
	}
	return toPayable(r), nil
}

// SavePayableOpen menyimpan sisa utang (cek versi).
func (s *Store) SavePayableOpen(ctx context.Context, p domain.Payable, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateSupplierPayableOpen(ctx, sqlcgen.UpdateSupplierPayableOpenParams{
		OpenAmount: p.OpenAmount.Decimal(), ActorID: actorID, ID: p.ID, Version: int32Of(p.Version),
	})
	if err != nil {
		return fmt.Errorf("update supplier payable: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
