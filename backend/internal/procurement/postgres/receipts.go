package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres/sqlcgen"
)

// ReceiptFilter adalah kriteria daftar Penerimaan Barang.
type ReceiptFilter struct {
	Status          domain.Status
	Types           []domain.OrderType
	PurchaseOrderID *uuid.UUID
	SupplierID      *uuid.UUID
	DateFrom        *clock.BusinessDate
	DateTo          *clock.BusinessDate
	Search          string
}

func toReceipt(r sqlcgen.ListGoodsReceiptsRow) domain.GoodsReceipt {
	return domain.GoodsReceipt{
		ID: r.ID, Number: r.Number.String, PurchaseOrderID: r.PurchaseOrderID, OrderNumber: r.OrderNumber.String,
		SupplierID: r.SupplierID, SupplierName: r.SupplierName, DivisionID: r.DivisionID,
		OrderType: domain.OrderType(r.OrderType), BusinessDate: r.BusinessDate, LocationID: r.LocationID,
		TollLocationID: uuidPtr(r.TollLocationID), DeliveryNote: r.DeliveryNoteNumber, Note: r.Note,
		InventoryAmount: amount(r.InventoryAmount), MaterialAmount: amount(r.MaterialAmount), GRNIAmount: amount(r.GrniAmount),
		Status: domain.Status(r.Status), InboundID: uuidPtr(r.InboundID), JournalEntryID: uuidPtr(r.JournalEntryID),
		ReversalEntryID: uuidPtr(r.ReversalJournalEntryID), CancelReason: r.CancelReason, PostedAt: r.PostedAt,
		CancelledAt: r.CancelledAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

func toReceiptLine(l sqlcgen.GoodsReceiptLine) domain.ReceiptLine {
	return domain.ReceiptLine{
		ID: l.ID, LineNo: int(l.LineNo), OrderLineID: l.PurchaseOrderLineID, ProductID: l.ProductID, GoodQty: l.GoodQty,
		RejectQty: l.RejectQty, RejectDisposition: domain.RejectDisposition(l.RejectDisposition), BatchNumber: l.BatchNumber,
		ProductionDate: fromPgDate(l.ProductionDate), ExpiryDate: fromPgDate(l.ExpiryDate), BatchID: uuidPtr(l.BatchID),
		UnitPrice: amount(l.UnitPriceAmount), GRNIQty: l.GrniQty, GRNIAmount: amount(l.GrniAmount),
		MaterialCost: amount(l.MaterialCostAmount), InventoryAmount: amount(l.InventoryAmount), UnitCost: l.UnitCost,
		InventoryAccount: l.InventoryAccountCode, BilledQty: l.BilledQty, BilledGRNIAmount: amount(l.BilledGrniAmount),
		ReturnedQty: l.ReturnedQty, ReturnedUnbilledQty: l.ReturnedUnbilledQty, ReturnedGRNIAmount: amount(l.ReturnedGrniAmount),
		Note: l.Note,
	}
}

func receiptListParams(f ReceiptFilter) sqlcgen.ListGoodsReceiptsParams {
	p := sqlcgen.ListGoodsReceiptsParams{
		Status: text(string(f.Status)), OrderTypes: typesOf(f.Types), PurchaseOrderID: nullUUID(f.PurchaseOrderID),
		SupplierID: nullUUID(f.SupplierID), DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: text(f.Search),
	}
	return p
}

// ListReceipts mengembalikan satu halaman GR (tanpa baris).
func (s *Store) ListReceipts(ctx context.Context, f ReceiptFilter, page paging.Page) (paging.Result[domain.GoodsReceipt], error) {
	p := receiptListParams(f)
	p.RowLimit, p.RowOffset = page.Limit(), page.Offset()
	rows, err := s.q(ctx).ListGoodsReceipts(ctx, p)
	if err != nil {
		return paging.Result[domain.GoodsReceipt]{}, fmt.Errorf("list goods receipts: %w", err)
	}
	total, err := s.q(ctx).CountGoodsReceipts(ctx, sqlcgen.CountGoodsReceiptsParams{
		Status: p.Status, OrderTypes: p.OrderTypes, PurchaseOrderID: p.PurchaseOrderID, SupplierID: p.SupplierID,
		DateFrom: p.DateFrom, DateTo: p.DateTo, Search: p.Search,
	})
	if err != nil {
		return paging.Result[domain.GoodsReceipt]{}, fmt.Errorf("count goods receipts: %w", err)
	}
	items := make([]domain.GoodsReceipt, len(rows))
	for i, r := range rows {
		items[i] = toReceipt(r)
	}
	return paging.Result[domain.GoodsReceipt]{Items: items, Page: page, Total: total}, nil
}

// GetReceipt membaca GR beserta baris, pemakaian bahan, dan rincian batch bahan; forUpdate mengunci GR.
func (s *Store) GetReceipt(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.GoodsReceipt, error) {
	q := s.q(ctx)
	if forUpdate {
		if _, err := q.LockGoodsReceipt(ctx, id); err != nil {
			return domain.GoodsReceipt{}, notFound(err, "lock goods receipt")
		}
	}
	row, err := q.GetGoodsReceipt(ctx, id)
	if err != nil {
		return domain.GoodsReceipt{}, notFound(err, "get goods receipt")
	}
	r := toReceipt(sqlcgen.ListGoodsReceiptsRow(row))
	lines, err := q.ListGoodsReceiptLines(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("list goods receipt lines: %w", err)
	}
	for _, l := range lines {
		r.Lines = append(r.Lines, toReceiptLine(l))
	}
	usages, err := q.ListGoodsReceiptMaterialUsages(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("list goods receipt material usages: %w", err)
	}
	cons, err := q.ListGoodsReceiptMaterialConsumptions(ctx, id)
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("list goods receipt material consumptions: %w", err)
	}
	for _, u := range usages {
		m := domain.MaterialUsage{
			ID: u.ID, LineNo: int(u.LineNo), ProductID: u.MaterialProductID, FromLocationID: u.FromLocationID,
			UsedGoodQty: u.UsedGoodQty, UsedRejectQty: u.UsedRejectQty, CostAmount: amount(u.CostAmount),
			AccountCode: u.AccountCode, Note: u.Note,
		}
		for _, c := range cons {
			if c.UsageID == u.ID {
				m.Consumptions = append(m.Consumptions, domain.MaterialConsumption{BatchID: uuidPtr(c.BatchID), Qty: c.Qty, UnitCost: c.UnitCost})
			}
		}
		r.Materials = append(r.Materials, m)
	}
	return r, nil
}

func (s *Store) insertReceiptItems(ctx context.Context, r domain.GoodsReceipt) error {
	q := s.q(ctx)
	for _, l := range r.Lines {
		if err := q.InsertGoodsReceiptLine(ctx, sqlcgen.InsertGoodsReceiptLineParams{
			GoodsReceiptID: r.ID, LineNo: int32Of(l.LineNo), PurchaseOrderLineID: l.OrderLineID, ProductID: l.ProductID,
			GoodQty: l.GoodQty, RejectQty: l.RejectQty, RejectDisposition: string(l.RejectDisposition),
			BatchNumber: l.BatchNumber, ProductionDate: pgDate(l.ProductionDate), ExpiryDate: pgDate(l.ExpiryDate),
			UnitPriceAmount: l.UnitPrice.Decimal(), GrniQty: l.GRNIQty, GrniAmount: l.GRNIAmount.Decimal(), Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert goods receipt line: %w", err)
		}
	}
	for _, m := range r.Materials {
		if err := q.InsertGoodsReceiptMaterialUsage(ctx, sqlcgen.InsertGoodsReceiptMaterialUsageParams{
			GoodsReceiptID: r.ID, LineNo: int32Of(m.LineNo), MaterialProductID: m.ProductID, FromLocationID: m.FromLocationID,
			UsedGoodQty: m.UsedGoodQty, UsedRejectQty: m.UsedRejectQty, Note: m.Note,
		}); err != nil {
			return fmt.Errorf("insert goods receipt material usage: %w", err)
		}
	}
	return nil
}

// InsertReceipt menyimpan draft GR beserta baris dan pemakaian bahan.
func (s *Store) InsertReceipt(ctx context.Context, r domain.GoodsReceipt, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertGoodsReceipt(ctx, sqlcgen.InsertGoodsReceiptParams{
		PurchaseOrderID: r.PurchaseOrderID, SupplierID: r.SupplierID, DivisionID: r.DivisionID, OrderType: string(r.OrderType),
		BusinessDate: r.BusinessDate, LocationID: r.LocationID, TollLocationID: nullUUID(r.TollLocationID),
		DeliveryNoteNumber: r.DeliveryNote, Note: r.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert goods receipt: %w", err)
	}
	r.ID = id
	return id, s.insertReceiptItems(ctx, r)
}

// SaveReceipt menyimpan header GR (cek versi); replaceItems mengganti baris & bahan draft, selain itu menyimpan
// hasil hitung baris (batch, HPP, qty ditagih/diretur) dan HPP bahan beserta rincian batch-nya.
func (s *Store) SaveReceipt(ctx context.Context, r domain.GoodsReceipt, version int, actorID uuid.UUID, replaceItems bool) error {
	q := s.q(ctx)
	n, err := q.UpdateGoodsReceipt(ctx, sqlcgen.UpdateGoodsReceiptParams{
		Number: text(r.Number), BusinessDate: r.BusinessDate, LocationID: r.LocationID, TollLocationID: nullUUID(r.TollLocationID),
		DeliveryNoteNumber: r.DeliveryNote, Note: r.Note, InventoryAmount: r.InventoryAmount.Decimal(),
		MaterialAmount: r.MaterialAmount.Decimal(), GrniAmount: r.GRNIAmount.Decimal(), Status: string(r.Status),
		InboundID: nullUUID(r.InboundID), JournalEntryID: nullUUID(r.JournalEntryID),
		ReversalJournalEntryID: nullUUID(r.ReversalEntryID), CancelReason: r.CancelReason, PostedAt: r.PostedAt,
		CancelledAt: r.CancelledAt, ActorID: actorID, ID: r.ID, Version: int32Of(version),
	})
	if err != nil {
		return fmt.Errorf("update goods receipt: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if replaceItems {
		if err := q.DeleteGoodsReceiptLines(ctx, r.ID); err != nil {
			return fmt.Errorf("delete goods receipt lines: %w", err)
		}
		if err := q.DeleteGoodsReceiptMaterialUsages(ctx, r.ID); err != nil {
			return fmt.Errorf("delete goods receipt material usages: %w", err)
		}
		return s.insertReceiptItems(ctx, r)
	}
	return s.SaveReceiptLines(ctx, r.Lines)
}

// SaveReceiptLines menyimpan hasil hitung dan qty ditagih/diretur baris GR.
func (s *Store) SaveReceiptLines(ctx context.Context, lines []domain.ReceiptLine) error {
	for _, l := range lines {
		if err := s.q(ctx).UpdateGoodsReceiptLine(ctx, sqlcgen.UpdateGoodsReceiptLineParams{
			BatchID: nullUUID(l.BatchID), MaterialCostAmount: l.MaterialCost.Decimal(), InventoryAmount: l.InventoryAmount.Decimal(),
			UnitCost: l.UnitCost, InventoryAccountCode: l.InventoryAccount, BilledQty: l.BilledQty,
			BilledGrniAmount: l.BilledGRNIAmount.Decimal(), ReturnedQty: l.ReturnedQty,
			ReturnedUnbilledQty: l.ReturnedUnbilledQty, ReturnedGrniAmount: l.ReturnedGRNIAmount.Decimal(), ID: l.ID,
		}); err != nil {
			return fmt.Errorf("update goods receipt line: %w", err)
		}
	}
	return nil
}

// SaveMaterialCosts menyimpan HPP bahan terpakai beserta rincian batch-nya.
func (s *Store) SaveMaterialCosts(ctx context.Context, materials []domain.MaterialUsage) error {
	q := s.q(ctx)
	for _, m := range materials {
		if err := q.UpdateGoodsReceiptMaterialUsage(ctx, sqlcgen.UpdateGoodsReceiptMaterialUsageParams{
			CostAmount: m.CostAmount.Decimal(), AccountCode: m.AccountCode, ID: m.ID,
		}); err != nil {
			return fmt.Errorf("update goods receipt material usage: %w", err)
		}
		for _, c := range m.Consumptions {
			if err := q.InsertGoodsReceiptMaterialConsumption(ctx, sqlcgen.InsertGoodsReceiptMaterialConsumptionParams{
				UsageID: m.ID, BatchID: nullUUID(c.BatchID), Qty: c.Qty, UnitCost: c.UnitCost,
			}); err != nil {
				return fmt.Errorf("insert goods receipt material consumption: %w", err)
			}
		}
	}
	return nil
}

// ReceiptLineRef adalah baris GR beserta GR & PO-nya.
type ReceiptLineRef struct {
	Line            domain.ReceiptLine
	ReceiptID       uuid.UUID
	ReceiptNumber   string
	ReceiptStatus   domain.Status
	PurchaseOrderID uuid.UUID
	LocationID      uuid.UUID
	BusinessDate    clock.BusinessDate
}

func toReceiptLineRef(r sqlcgen.ReceiptLinesByIDsRow) ReceiptLineRef {
	l := sqlcgen.GoodsReceiptLine{
		ID: r.ID, GoodsReceiptID: r.GoodsReceiptID, LineNo: r.LineNo, PurchaseOrderLineID: r.PurchaseOrderLineID,
		ProductID: r.ProductID, GoodQty: r.GoodQty, RejectQty: r.RejectQty, RejectDisposition: r.RejectDisposition,
		BatchNumber: r.BatchNumber, ProductionDate: r.ProductionDate, ExpiryDate: r.ExpiryDate, BatchID: r.BatchID,
		UnitPriceAmount: r.UnitPriceAmount, GrniQty: r.GrniQty, GrniAmount: r.GrniAmount,
		MaterialCostAmount: r.MaterialCostAmount, InventoryAmount: r.InventoryAmount, UnitCost: r.UnitCost,
		InventoryAccountCode: r.InventoryAccountCode, BilledQty: r.BilledQty, BilledGrniAmount: r.BilledGrniAmount,
		ReturnedQty: r.ReturnedQty, ReturnedUnbilledQty: r.ReturnedUnbilledQty, ReturnedGrniAmount: r.ReturnedGrniAmount,
		Note: r.Note,
	}
	return ReceiptLineRef{
		Line: toReceiptLine(l), ReceiptID: r.GoodsReceiptID, ReceiptNumber: r.ReceiptNumber.String,
		ReceiptStatus: domain.Status(r.ReceiptStatus), PurchaseOrderID: r.PurchaseOrderID, LocationID: r.LocationID,
		BusinessDate: r.BusinessDate,
	}
}

// ReceiptLinesByIDs membaca baris GR beserta GR-nya.
func (s *Store) ReceiptLinesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ReceiptLineRef, error) {
	out := map[uuid.UUID]ReceiptLineRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).ReceiptLinesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("receipt lines by ids: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = toReceiptLineRef(r)
	}
	return out, nil
}

// ReceiptLinesByOrder membaca baris GR diposting satu PO.
func (s *Store) ReceiptLinesByOrder(ctx context.Context, orderID uuid.UUID) ([]ReceiptLineRef, error) {
	rows, err := s.q(ctx).ReceiptLinesByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("receipt lines by order: %w", err)
	}
	out := make([]ReceiptLineRef, len(rows))
	for i, r := range rows {
		out[i] = toReceiptLineRef(sqlcgen.ReceiptLinesByIDsRow(r))
	}
	return out, nil
}

// ReceiptLineByBatch mencari baris GR diposting terakhir yang menerima batch produk; ok false bila tidak ada.
func (s *Store) ReceiptLineByBatch(ctx context.Context, productID, batchID uuid.UUID) (uuid.UUID, uuid.UUID, bool, error) {
	rows, err := s.q(ctx).ReceiptLineByBatch(ctx, sqlcgen.ReceiptLineByBatchParams{
		ProductID: productID, BatchID: uuid.NullUUID{UUID: batchID, Valid: true},
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, false, fmt.Errorf("receipt line by batch: %w", err)
	}
	if len(rows) == 0 {
		return uuid.Nil, uuid.Nil, false, nil
	}
	return rows[0].ID, rows[0].PurchaseOrderID, true, nil
}

// ReceiptIDsByLines mengembalikan GR pemilik baris.
func (s *Store) ReceiptIDsByLines(ctx context.Context, lineIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(lineIDs) == 0 {
		return nil, nil
	}
	ids, err := s.q(ctx).ReceiptIDsByLines(ctx, lineIDs)
	if err != nil {
		return nil, fmt.Errorf("receipt ids by lines: %w", err)
	}
	return ids, nil
}

// OpenReceiptLine adalah satu baris laporan Barang Diterima Belum Ditagih.
type OpenReceiptLine struct {
	LineID          uuid.UUID
	ReceiptID       uuid.UUID
	ReceiptNumber   string
	LineNo          int
	BusinessDate    clock.BusinessDate
	PurchaseOrderID uuid.UUID
	OrderNumber     string
	OrderType       domain.OrderType
	SupplierID      uuid.UUID
	SupplierName    string
	DivisionID      uuid.UUID
	ProductID       uuid.UUID
	UnitPrice       money.Amount
	ReceivedQty     decimal.Decimal
	ReceivedAmount  money.Amount
	BilledQty       decimal.Decimal
	BilledAmount    money.Amount
	ReturnedQty     decimal.Decimal
	ReturnedAmount  money.Amount
}

// OpenAmount adalah saldo 2102 baris.
func (l OpenReceiptLine) OpenAmount() money.Amount {
	return l.ReceivedAmount.Sub(l.BilledAmount).Sub(l.ReturnedAmount)
}

// OpenQty adalah qty yang belum ditagih.
func (l OpenReceiptLine) OpenQty() decimal.Decimal {
	return l.ReceivedQty.Sub(l.BilledQty).Sub(l.ReturnedQty)
}

// GRNIFilter adalah filter laporan Barang Diterima Belum Ditagih.
type GRNIFilter struct {
	SupplierID      *uuid.UUID
	PurchaseOrderID *uuid.UUID
	Search          string
}

// OpenReceiptLines mengembalikan baris GR yang saldo 2102-nya belum nol.
func (s *Store) OpenReceiptLines(ctx context.Context, f GRNIFilter) ([]OpenReceiptLine, error) {
	rows, err := s.q(ctx).OpenReceiptLines(ctx, sqlcgen.OpenReceiptLinesParams{
		SupplierID: nullUUID(f.SupplierID), PurchaseOrderID: nullUUID(f.PurchaseOrderID), Search: text(f.Search),
	})
	if err != nil {
		return nil, fmt.Errorf("open receipt lines: %w", err)
	}
	out := make([]OpenReceiptLine, len(rows))
	for i, r := range rows {
		out[i] = OpenReceiptLine{
			LineID: r.ID, ReceiptID: r.GoodsReceiptID, ReceiptNumber: r.ReceiptNumber.String, LineNo: int(r.LineNo),
			BusinessDate: r.BusinessDate, PurchaseOrderID: r.PurchaseOrderID, OrderNumber: r.OrderNumber.String,
			OrderType: domain.OrderType(r.OrderType), SupplierID: r.SupplierID, SupplierName: r.SupplierName,
			DivisionID: r.DivisionID, ProductID: r.ProductID, UnitPrice: amount(r.UnitPriceAmount),
			ReceivedQty: r.GrniQty, ReceivedAmount: amount(r.GrniAmount), BilledQty: r.BilledQty,
			BilledAmount: amount(r.BilledGrniAmount), ReturnedQty: r.ReturnedUnbilledQty, ReturnedAmount: amount(r.ReturnedGrniAmount),
		}
	}
	return out, nil
}

// TotalOpenGRNI menjumlah saldo Barang Diterima Belum Ditagih semua GR diposting.
func (s *Store) TotalOpenGRNI(ctx context.Context) (money.Amount, error) {
	total, err := s.q(ctx).TotalOpenGRNI(ctx)
	if err != nil {
		return money.Zero, fmt.Errorf("total open grni: %w", err)
	}
	return amount(total), nil
}
