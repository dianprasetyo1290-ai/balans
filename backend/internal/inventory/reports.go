package inventory

import (
	"context"

	"balans/internal/inventory/service"
	"balans/internal/platform/tabular"
)

// Laporan persediaan (S-414, S-415) dan read model rm_stock_values.

// ReadModelStock adalah nama read model posisi & nilai stok.
const ReadModelStock = service.ReadModelStock

// RefreshReadModels me-refresh rm_stock_values (didaftarkan ke job accounting.refresh_read_models).
func (m *Module) RefreshReadModels(ctx context.Context) error { return m.svc.RefreshReadModels(ctx) }

// StockPositionColumns adalah kolom ekspor Laporan Stok.
func StockPositionColumns() []tabular.Column { return service.StockPositionColumns }

// StockMovementColumns adalah kolom ekspor Laporan Mutasi Stok.
func StockMovementColumns() []tabular.Column { return service.MovementColumns }

// StockExpiryColumns adalah kolom ekspor Laporan Kedaluwarsa.
func StockExpiryColumns() []tabular.Column { return service.ExpiryColumns }

// ValidateStockReportParams memeriksa filter ekspor laporan stok (locationId, locationType, productId, batchId).
func (m *Module) ValidateStockReportParams(p map[string]string) error {
	_, err := service.ReportFilterFromParams(p)
	return err
}

// ValidateStockPeriodParams memeriksa filter + periode ekspor mutasi stok.
func (m *Module) ValidateStockPeriodParams(p map[string]string) error {
	if _, err := service.ReportFilterFromParams(p); err != nil {
		return err
	}
	_, _, err := m.svc.PeriodFromParams(p)
	return err
}

// ExportStockPosition mengalirkan Laporan Stok.
func (m *Module) ExportStockPosition(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportStockPosition(ctx, p, emit)
}

// ExportStockMovements mengalirkan Laporan Mutasi Stok.
func (m *Module) ExportStockMovements(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportStockMovements(ctx, p, emit)
}

// ExportStockExpiry mengalirkan Laporan Kedaluwarsa.
func (m *Module) ExportStockExpiry(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportStockExpiry(ctx, p, emit)
}
