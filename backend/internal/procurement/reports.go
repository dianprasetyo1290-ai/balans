package procurement

import (
	"context"

	"balans/internal/platform/tabular"
	"balans/internal/procurement/service"
)

// OutstandingPOColumns adalah kolom ekspor Outstanding PO (RPT-10).
func OutstandingPOColumns() []tabular.Column { return service.OutstandingPOColumns }

// PayableAgingColumns adalah kolom ekspor umur utang (RPT-11).
func PayableAgingColumns() []tabular.Column { return service.PayableAgingColumns }

// ExportOutstandingPO mengalirkan Outstanding PO.
func (m *Module) ExportOutstandingPO(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportOutstandingPO(ctx, p, emit)
}

// ExportPayableAging mengalirkan umur utang per dokumen (params asOf).
func (m *Module) ExportPayableAging(ctx context.Context, p map[string]string, emit func(tabular.Row) error) error {
	return m.svc.ExportPayableAging(ctx, p, emit)
}
