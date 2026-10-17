package sales

import (
	"context"

	"balans/internal/platform/tabular"
	"balans/internal/sales/service"
)

// Laporan penjualan & margin (S-415, S-417) dan settlement marketplace (S-403).

// ReadModelSalesLines adalah nama read model penjualan & HPP.
const ReadModelSalesLines = service.ReadModelSalesLines

// SettlementFormat adalah format laporan settlement satu marketplace (jenis data impor + pemetaan kolom).
type SettlementFormat = service.SettlementFormat

// Permission, resource, dan jenis entitas settlement marketplace.
const (
	PermSettlementRead = service.PermSettlementRead
	EntitySettlement   = service.EntitySettlement
	SettlementResource = "accounting.settlement"
)

// RefreshReadModels me-refresh rm_sales_lines (didaftarkan ke job accounting.refresh_read_models).
func (m *Module) RefreshReadModels(ctx context.Context) error { return m.docs.RefreshReadModels(ctx) }

// SalesReportColumns adalah kolom ekspor Laporan Penjualan & margin.
func SalesReportColumns() []tabular.Column { return service.SalesReportColumns }

// ValidateSalesReportParams memeriksa parameter ekspor laporan penjualan.
func (m *Module) ValidateSalesReportParams(p map[string]string) error {
	_, err := m.docs.SalesReportFilterFromParams(p)
	return err
}

// ExportSalesReport mengalirkan laporan penjualan & margin (params groupBy, dateFrom, dateTo, …).
func (m *Module) ExportSalesReport(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.docs.ExportSalesReport(ctx, p, emit)
}

// SettlementFormats adalah format laporan settlement per marketplace.
func SettlementFormats() []SettlementFormat { return service.SettlementFormats }

// ImportSettlementRow mencocokkan satu baris laporan settlement dengan order marketplace dan menambahkannya ke
// settlement draft. Butuh accounting.settlement.import + write.
func (m *Module) ImportSettlementRow(ctx context.Context, f SettlementFormat, row tabular.Row, rowKey string) error {
	return m.docs.ImportSettlementRow(ctx, f, row, rowKey)
}
