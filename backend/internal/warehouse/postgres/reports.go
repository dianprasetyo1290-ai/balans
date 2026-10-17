package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/warehouse/postgres/sqlcgen"
)

// CountVarianceFilter adalah filter laporan stock count (RPT-03).
type CountVarianceFilter struct {
	DateFrom     clock.BusinessDate
	DateTo       clock.BusinessDate
	LocationID   *uuid.UUID
	ProductID    *uuid.UUID
	VarianceOnly bool
}

// CountVarianceRow adalah satu baris hasil stock count.
type CountVarianceRow struct {
	CountID        uuid.UUID
	Number         string
	CountType      string
	LocationID     uuid.UUID
	BusinessDate   clock.BusinessDate
	Status         string
	ProductID      uuid.UUID
	BatchID        *uuid.UUID
	SystemQty      decimal.Decimal
	CountedQty     *decimal.Decimal
	VarianceQty    decimal.Decimal
	VarianceAmount decimal.Decimal
	UnitCost       decimal.Decimal
	IsExtra        bool
}

func idOrNil(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

// StockCountVariances membaca hasil stock count & selisih.
func (s *Store) StockCountVariances(ctx context.Context, f CountVarianceFilter) ([]CountVarianceRow, error) {
	rows, err := s.q(ctx).StockCountVarianceReport(ctx, sqlcgen.StockCountVarianceReportParams{
		DateFrom: f.DateFrom, DateTo: f.DateTo, LocationID: idOrNil(f.LocationID), ProductID: idOrNil(f.ProductID),
		VarianceOnly: f.VarianceOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("stock count variance report: %w", err)
	}
	out := make([]CountVarianceRow, len(rows))
	for i, r := range rows {
		out[i] = CountVarianceRow{
			CountID: r.ID, Number: r.Number, CountType: r.CountType, LocationID: r.LocationID, BusinessDate: r.BusinessDate,
			Status: r.Status, ProductID: r.ProductID, SystemQty: r.SystemQty, VarianceQty: r.VarianceQty,
			VarianceAmount: r.VarianceAmount, UnitCost: r.UnitCost, IsExtra: r.IsExtra,
		}
		if r.BatchID.Valid {
			id := r.BatchID.UUID
			out[i].BatchID = &id
		}
		if r.CountedQty.Valid {
			q := r.CountedQty.Decimal
			out[i].CountedQty = &q
		}
	}
	return out, nil
}
