package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/procurement/postgres/sqlcgen"
)

// Laporan pengadaan: Outstanding PO (RPT-10), umur utang (RPT-11), komposisi HPP batch (RPT-04).

// OutstandingPOLine adalah baris PO yang belum diterima/ditagih penuh.
type OutstandingPOLine struct {
	OrderID      uuid.UUID
	Number       string
	OrderType    string
	SupplierID   uuid.UUID
	SupplierCode string
	SupplierName string
	OrderDate    clock.BusinessDate
	ExpectedDate clock.BusinessDate
	Status       string
	LineNo       int
	ProductID    *uuid.UUID
	Description  string
	OrderedQty   decimal.Decimal
	ReceivedQty  decimal.Decimal
	BilledQty    decimal.Decimal
	UnitPrice    money.Amount
}

// OutstandingPurchaseOrders membaca baris PO terbuka.
func (s *Store) OutstandingPurchaseOrders(ctx context.Context, supplierID *uuid.UUID) ([]OutstandingPOLine, error) {
	rows, err := s.q(ctx).OutstandingPurchaseOrderLines(ctx, nullUUID(supplierID))
	if err != nil {
		return nil, fmt.Errorf("outstanding purchase orders: %w", err)
	}
	out := make([]OutstandingPOLine, len(rows))
	for i, r := range rows {
		out[i] = OutstandingPOLine{
			OrderID: r.ID, Number: r.Number, OrderType: r.OrderType, SupplierID: r.SupplierID, SupplierCode: r.SupplierCode,
			SupplierName: r.SupplierName, OrderDate: r.BusinessDate, ExpectedDate: r.ExpectedDate, Status: r.Status,
			LineNo: int(r.LineNo), Description: r.Description, OrderedQty: r.OrderedQty, ReceivedQty: r.ReceivedQty,
			BilledQty: r.BilledQty, UnitPrice: money.FromDecimal(r.UnitPriceAmount),
		}
		if r.ProductID.Valid {
			id := r.ProductID.UUID
			out[i].ProductID = &id
		}
	}
	return out, nil
}

// OpenPayableRow adalah utang pemasok terbuka satu dokumen.
type OpenPayableRow struct {
	ID             uuid.UUID
	DocumentType   string
	DocumentNumber string
	SupplierID     uuid.UUID
	SupplierCode   string
	SupplierName   string
	BusinessDate   clock.BusinessDate
	DueDate        clock.BusinessDate
	Amount         money.Amount
	Open           money.Amount
}

// OpenPayablesAsOf membaca utang terbuka per tanggal dokumen ≤ asOf.
func (s *Store) OpenPayablesAsOf(ctx context.Context, asOf clock.BusinessDate) ([]OpenPayableRow, error) {
	rows, err := s.q(ctx).PayableAgingReport(ctx, asOf)
	if err != nil {
		return nil, fmt.Errorf("payable aging: %w", err)
	}
	out := make([]OpenPayableRow, len(rows))
	for i, r := range rows {
		out[i] = OpenPayableRow{
			ID: r.ID, DocumentType: r.DocumentType, DocumentNumber: r.DocumentNumber, SupplierID: r.SupplierID,
			SupplierCode: r.SupplierCode, SupplierName: r.SupplierName, BusinessDate: r.BusinessDate, DueDate: r.DueDate,
			Amount: money.FromDecimal(r.Amount), Open: money.FromDecimal(r.OpenAmount),
		}
	}
	return out, nil
}

// BatchCostRow adalah komposisi HPP satu baris Penerimaan Barang.
type BatchCostRow struct {
	ReceiptID         uuid.UUID
	ReceiptNumber     string
	BusinessDate      clock.BusinessDate
	SupplierName      string
	OrderType         string
	ProductID         uuid.UUID
	BatchNumber       string
	GoodQty           decimal.Decimal
	RejectQty         decimal.Decimal
	RejectDisposition string
	UnitPrice         money.Amount
	GRNIAmount        money.Amount
	MaterialCost      money.Amount
	InventoryAmount   money.Amount
	UnitCost          decimal.Decimal
}

// BatchCosts membaca komposisi HPP batch dari Penerimaan Barang diposting.
func (s *Store) BatchCosts(ctx context.Context, from, to clock.BusinessDate, productID *uuid.UUID) ([]BatchCostRow, error) {
	rows, err := s.q(ctx).BatchCostReport(ctx, sqlcgen.BatchCostReportParams{DateFrom: from, DateTo: to, ProductID: nullUUID(productID)})
	if err != nil {
		return nil, fmt.Errorf("batch cost report: %w", err)
	}
	out := make([]BatchCostRow, len(rows))
	for i, r := range rows {
		out[i] = BatchCostRow{
			ReceiptID: r.ReceiptID, ReceiptNumber: r.ReceiptNumber, BusinessDate: r.BusinessDate, SupplierName: r.SupplierName,
			OrderType: r.OrderType, ProductID: r.ProductID, BatchNumber: r.BatchNumber, GoodQty: r.GoodQty,
			RejectQty: r.RejectQty, RejectDisposition: r.RejectDisposition, UnitPrice: money.FromDecimal(r.UnitPriceAmount),
			GRNIAmount: money.FromDecimal(r.GrniAmount), MaterialCost: money.FromDecimal(r.MaterialCostAmount),
			InventoryAmount: money.FromDecimal(r.InventoryAmount), UnitCost: r.UnitCost,
		}
	}
	return out, nil
}
