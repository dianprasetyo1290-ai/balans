package app

import (
	"context"

	"balans/internal/accounting"
	"balans/internal/dataexchange"
	"balans/internal/filestore"
	"balans/internal/inventory"
	"balans/internal/platform/tabular"
	"balans/internal/procurement"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// reportExporter mengisi dataexchange.Exporter + ParamsValidator dengan fungsi ekspor laporan modul pemilik data
// (TG 5.17: ekspor laporan berjalan sebagai job dataexchange.export).
type reportExporter struct {
	spec     dataexchange.Spec
	validate func(params map[string]string) error
	export   func(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error
}

func (r reportExporter) Spec() dataexchange.Spec { return r.spec }

func (r reportExporter) Export(ctx context.Context, emit func(tabular.Row) error) error {
	return r.export(ctx, dataexchange.ExportParams(ctx), emit)
}

func (r reportExporter) ValidateParams(_ context.Context, params map[string]string) error {
	if r.validate == nil {
		return nil
	}
	return r.validate(params)
}

// reportSpec menyusun jenis data ekspor laporan; perm adalah permission baca laporan (bukan `<kind>.export`).
func reportSpec(kind, label, perm string, columns []tabular.Column) dataexchange.Spec {
	return dataexchange.Spec{Kind: "report." + kind, Label: label, Sheet: label, Columns: columns, ExportPerm: perm}
}

// connectReports mendaftarkan read model laporan ke job accounting.refresh_read_models (tiap 15 menit), ekspor
// laporan sebagai job, dan impor laporan settlement marketplace per marketplace (S-403, S-409, S-414 – S-418).
func (a *App) connectReports() {
	if a.accounting != nil {
		if a.inventory != nil {
			a.accounting.RegisterReadModel(inventory.ReadModelStock, a.inventory.RefreshReadModels)
		}
		if a.sales != nil {
			a.accounting.RegisterReadModel(sales.ReadModelSalesLines, a.sales.RefreshReadModels)
		}
	}
	if a.sales != nil {
		registerAuditEntities(a.audit, map[string]string{sales.EntitySettlement: sales.PermSettlementRead})
	}
	dx := a.dataexchange
	if dx == nil {
		return
	}
	reg := func(s dataexchange.Spec, validate func(map[string]string) error,
		export func(context.Context, map[string]string, func(tabular.Row) error) error) {
		dx.RegisterExporter(reportExporter{spec: s, validate: validate, export: export})
	}
	if a.accounting != nil {
		reg(reportSpec("financial_statement", "Laporan Keuangan", "accounting.report.read", accounting.StatementColumns()),
			a.accounting.ValidateStatementParams, a.accounting.ExportStatement)
		reg(reportSpec("profit_loss_dimension", "Laporan Laba Rugi per Dimensi", "accounting.report.read", accounting.DimensionReportColumns()),
			a.accounting.ValidateDimensionReportParams, a.accounting.ExportDimensionReport)
		reg(reportSpec("tax", "Laporan Pajak", "accounting.report.read", accounting.TaxReportColumns()),
			a.accounting.ValidateTaxReportParams, a.accounting.ExportTaxReport)
	}
	if a.inventory != nil {
		reg(reportSpec("stock_position", "Laporan Stok", "inventory.stock.read", inventory.StockPositionColumns()),
			a.inventory.ValidateStockReportParams, a.inventory.ExportStockPosition)
		reg(reportSpec("stock_movement", "Laporan Mutasi Stok", "inventory.stock.read", inventory.StockMovementColumns()),
			a.inventory.ValidateStockPeriodParams, a.inventory.ExportStockMovements)
		reg(reportSpec("stock_expiry", "Laporan Kedaluwarsa", "inventory.stock.read", inventory.StockExpiryColumns()),
			a.inventory.ValidateStockReportParams, a.inventory.ExportStockExpiry)
	}
	if a.warehouse != nil {
		reg(reportSpec("stock_count", "Laporan Stock Count", "warehouse.stock_count.read", warehouse.StockCountReportColumns()),
			nil, a.warehouse.ExportStockCountReport)
	}
	if a.procurement != nil {
		reg(reportSpec("outstanding_po", "Laporan Outstanding PO", "procurement.purchase_order.read", procurement.OutstandingPOColumns()),
			nil, a.procurement.ExportOutstandingPO)
		reg(reportSpec("payable_aging", "Laporan Utang", "procurement.supplier_bill.read", procurement.PayableAgingColumns()),
			nil, a.procurement.ExportPayableAging)
	}
	if a.sales != nil {
		reg(reportSpec("sales", "Laporan Penjualan", "sales.sales_order.read", sales.SalesReportColumns()),
			a.sales.ValidateSalesReportParams, a.sales.ExportSalesReport)
		m := a.sales
		for _, f := range sales.SettlementFormats() {
			s := dataexchange.Spec{Kind: f.Kind(), Label: f.Label, Sheet: "Settlement", Columns: f.Columns, Resource: sales.SettlementResource}
			dx.RegisterImporter(tableImporter{spec: s, apply: func(ctx context.Context, row tabular.Row) error {
				return m.ImportSettlementRow(ctx, f, row, dataexchange.RowKey(ctx))
			}})
			a.filestore.RegisterOwner(filestore.OwnerPolicy{
				OwnerType: dataexchange.UploadOwnerType(s.Kind), WritePermission: s.ImportPermission(),
				ReadPermission: s.ImportPermission(), ContentTypes: []string{filestore.TypeXLSX}, MaxBytes: dataexchange.MaxFileBytes,
			})
		}
	}
}
