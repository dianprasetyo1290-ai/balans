package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres/sqlcgen"
	"balans/internal/platform/clock"
)

// LocationFlowQuery adalah kriteria mutasi satu lokasi dalam periode DateFrom s.d. DateTo.
type LocationFlowQuery struct {
	LocationID uuid.UUID
	Statuses   []domain.StockStatus
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	// ExcludeSources: keluar dari jenis dokumen ini dicatat di ExcludedOutQty, bukan OutQty.
	ExcludeSources []string
}

// LocationFlows membaca saldo awal, masuk, dan keluar per produk di satu lokasi.
func (s *Store) LocationFlows(ctx context.Context, q LocationFlowQuery) ([]domain.LocationFlow, error) {
	statuses := make([]string, len(q.Statuses))
	for i, st := range q.Statuses {
		statuses[i] = string(st)
	}
	exclude := q.ExcludeSources
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := s.q(ctx).LocationFlows(ctx, sqlcgen.LocationFlowsParams{
		LocationID: q.LocationID, Statuses: statuses, DateTo: q.DateTo, DateFrom: q.DateFrom, ExcludeSources: exclude,
	})
	if err != nil {
		return nil, fmt.Errorf("location flows: %w", err)
	}
	out := make([]domain.LocationFlow, len(rows))
	for i, r := range rows {
		out[i] = domain.LocationFlow{
			ProductID: r.ProductID, OpeningQty: r.OpeningQty, InQty: r.InQty, OutQty: r.OutQty, ExcludedOutQty: r.ExcludedOutQty,
		}
	}
	return out, nil
}
