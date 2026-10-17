package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/postgres/sqlcgen"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
)

// Read model & laporan persediaan (TG 5.17).

// RefreshStockReadModel me-refresh rm_stock_values tanpa mengunci pembaca (dipanggil job accounting).
func (s *Store) RefreshStockReadModel(ctx context.Context) error {
	if _, err := db.Conn(ctx, s.pool).Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY rm_stock_values"); err != nil {
		return fmt.Errorf("refresh rm_stock_values: %w", err)
	}
	return nil
}

func optUUID(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid || id.UUID == uuid.Nil {
		return nil
	}
	v := id.UUID
	return &v
}

func locs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// unknownDate menandai tanggal masuk pertama yang tidak diketahui di rm_stock_values.
var unknownDate = clock.NewBusinessDate(1900, 1, 1)

// StockValueRow adalah posisi & nilai stok satu produk × lokasi × batch.
type StockValueRow struct {
	ProductID   uuid.UUID
	LocationID  uuid.UUID
	BatchID     *uuid.UUID
	BatchNumber string
	ExpiryDate  *clock.BusinessDate
	FirstInDate *clock.BusinessDate
	Qty         decimal.Decimal
	Value       decimal.Decimal
}

// StockReportFilter adalah filter laporan stok.
type StockReportFilter struct {
	LocationIDs []uuid.UUID
	ProductID   *uuid.UUID
	BatchID     *uuid.UUID
}

func toDatePtr(d pgtype.Date) *clock.BusinessDate { return datePtr(d) }

// StockValueRows membaca posisi & nilai stok dari rm_stock_values.
func (s *Store) StockValueRows(ctx context.Context, f StockReportFilter) ([]StockValueRow, error) {
	rows, err := s.q(ctx).StockPositionReport(ctx, sqlcgen.StockPositionReportParams{
		LocationIds: locs(f.LocationIDs), ProductID: nullUUID(f.ProductID), BatchID: nullUUID(f.BatchID),
	})
	if err != nil {
		return nil, fmt.Errorf("stock position report: %w", err)
	}
	out := make([]StockValueRow, len(rows))
	for i, r := range rows {
		out[i] = StockValueRow{
			ProductID: r.ProductID, LocationID: r.LocationID, BatchID: optUUID(r.BatchID), BatchNumber: r.BatchNumber,
			ExpiryDate: toDatePtr(r.ExpiryDate), Qty: r.Qty, Value: r.ValueAmount,
		}
		if r.FirstInDate != unknownDate {
			d := r.FirstInDate
			out[i].FirstInDate = &d
		}
	}
	return out, nil
}

// MovementSummaryRow adalah mutasi satu produk × lokasi pada periode.
type MovementSummaryRow struct {
	ProductID    uuid.UUID
	LocationID   uuid.UUID
	ReceiveQty   decimal.Decimal
	IssueQty     decimal.Decimal
	MoveInQty    decimal.Decimal
	MoveOutQty   decimal.Decimal
	InValue      decimal.Decimal
	OutValue     decimal.Decimal
	ClosingQty   decimal.Decimal
	ClosingValue decimal.Decimal
}

// OpeningQty = saldo akhir − mutasi periode.
func (r MovementSummaryRow) OpeningQty() decimal.Decimal {
	return r.ClosingQty.Sub(r.ReceiveQty).Sub(r.IssueQty).Sub(r.MoveInQty).Sub(r.MoveOutQty)
}

// OpeningValue = nilai akhir − nilai masuk − nilai keluar (bertanda).
func (r MovementSummaryRow) OpeningValue() decimal.Decimal {
	return r.ClosingValue.Sub(r.InValue).Sub(r.OutValue)
}

// MovementSummary membaca mutasi per produk × lokasi pada periode.
func (s *Store) MovementSummary(ctx context.Context, f StockReportFilter, from, to clock.BusinessDate) ([]MovementSummaryRow, error) {
	rows, err := s.q(ctx).StockMovementSummary(ctx, sqlcgen.StockMovementSummaryParams{
		LocationIds: locs(f.LocationIDs), ProductID: nullUUID(f.ProductID), DateFrom: from, DateTo: to,
	})
	if err != nil {
		return nil, fmt.Errorf("stock movement summary: %w", err)
	}
	out := make([]MovementSummaryRow, len(rows))
	for i, r := range rows {
		out[i] = MovementSummaryRow{
			ProductID: r.ProductID, LocationID: r.LocationID, ReceiveQty: r.ReceiveQty, IssueQty: r.IssueQty,
			MoveInQty: r.MoveInQty, MoveOutQty: r.MoveOutQty, InValue: r.InValue, OutValue: r.OutValue,
			ClosingQty: r.ClosingQty, ClosingValue: r.ClosingValue,
		}
	}
	return out, nil
}

// UsageReportFilter adalah filter laporan pemakaian internal.
type UsageReportFilter struct {
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	Purpose    string
	CampaignID *uuid.UUID
	DivisionID *uuid.UUID
}

// UsageReportRow adalah satu baris pemakaian internal diposting.
type UsageReportRow struct {
	ID           uuid.UUID
	Number       string
	BusinessDate clock.BusinessDate
	Purpose      string
	DivisionID   uuid.UUID
	CampaignID   *uuid.UUID
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	ReturnedQty  decimal.Decimal
	ConsumedQty  decimal.Decimal
	Cost         decimal.Decimal
}

// UsageReport membaca pemakaian internal diposting.
func (s *Store) UsageReport(ctx context.Context, f UsageReportFilter) ([]UsageReportRow, error) {
	rows, err := s.q(ctx).InternalUsageReport(ctx, sqlcgen.InternalUsageReportParams{
		DateFrom: f.DateFrom, DateTo: f.DateTo, Purpose: pgtype.Text{String: f.Purpose, Valid: f.Purpose != ""},
		CampaignID: nullUUID(f.CampaignID), DivisionID: nullUUID(f.DivisionID),
	})
	if err != nil {
		return nil, fmt.Errorf("internal usage report: %w", err)
	}
	out := make([]UsageReportRow, len(rows))
	for i, r := range rows {
		out[i] = UsageReportRow{
			ID: r.ID, Number: r.Number, BusinessDate: r.BusinessDate, Purpose: r.Purpose, DivisionID: r.DivisionID,
			CampaignID: optUUID(r.CampaignID), ProductID: r.ProductID, Qty: r.Qty, ReturnedQty: r.ReturnedQty,
			ConsumedQty: r.ConsumedQty, Cost: r.CostAmount,
		}
	}
	return out, nil
}

// ReturnReportRow adalah satu baris Disposisi Retur diposting.
type ReturnReportRow struct {
	ID           uuid.UUID
	Number       string
	BusinessDate clock.BusinessDate
	LocationID   uuid.UUID
	ProductID    uuid.UUID
	ReasonID     *uuid.UUID
	Disposition  string
	Qty          decimal.Decimal
	Cost         decimal.Decimal
	TargetType   string
	QCNote       string
}

// ReturnReport membaca baris Disposisi Retur diposting.
func (s *Store) ReturnReport(ctx context.Context, from, to clock.BusinessDate) ([]ReturnReportRow, error) {
	rows, err := s.q(ctx).ReturnDispositionReport(ctx, sqlcgen.ReturnDispositionReportParams{DateFrom: from, DateTo: to})
	if err != nil {
		return nil, fmt.Errorf("return disposition report: %w", err)
	}
	out := make([]ReturnReportRow, len(rows))
	for i, r := range rows {
		out[i] = ReturnReportRow{
			ID: r.ID, Number: r.Number, BusinessDate: r.BusinessDate, LocationID: r.LocationID, ProductID: r.ProductID,
			ReasonID: optUUID(r.ReasonID), Disposition: r.Disposition, Qty: r.Qty, Cost: r.CostAmount,
			TargetType: r.TargetType, QCNote: r.QcNote,
		}
	}
	return out, nil
}
