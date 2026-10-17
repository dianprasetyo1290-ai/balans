package warehouse

import (
	"context"

	"balans/internal/platform/tabular"
	"balans/internal/warehouse/service"
)

// StockCountReportColumns adalah kolom ekspor Laporan Stock Count (RPT-03).
func StockCountReportColumns() []tabular.Column { return service.CountVarianceColumns }

// ExportStockCountReport mengalirkan Laporan Stock Count (params dateFrom, dateTo, locationId, varianceOnly).
func (m *Module) ExportStockCountReport(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportStockCountVariances(ctx, p, emit)
}
