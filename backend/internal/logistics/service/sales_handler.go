package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/logistics/domain"
	"balans/internal/platform/apperr"
	"balans/internal/warehouse"
)

// salesHandler adalah handler bawaan DO dari Sales Order: QC gagal → ganti batch alokasi (Karantina + FEFO
// pengganti); serah ke kurir → sales.MarkShipped (A-06) atau MarkHandedOver (marketplace, A-10); diterima →
// sales.MarkDelivered.
func (s *Service) salesHandler() DeliveryHandler {
	return DeliveryHandler{
		Replace: func(ctx context.Context, d domain.DeliveryOrder, r warehouse.OutboundRejection) (warehouse.OutboundReplacement, error) {
			if r.SourceLineID == nil {
				return warehouse.OutboundReplacement{}, apperr.Validation(map[string]string{"lineId": "Baris Outbound tanpa alokasi Sales Order"})
			}
			// Ref unik per aksi ledger: baris ditolak QC sekali; alokasi ulang baris ditahan bisa berulang.
			ref := r.LineID.String()
			if !r.Qty.IsPositive() {
				ref = "refill-" + uuid.NewString()
			}
			res, err := s.Sales.Reallocate(ctx, d.SourceID, *r.SourceLineID, r.BatchID, r.Qty, ref, d.Number)
			if err != nil {
				return warehouse.OutboundReplacement{}, err
			}
			return warehouse.OutboundReplacement{
				QuarantineLocationID: res.QuarantineLocationID, Batches: res.Replacements, Shortfall: res.Shortfall,
			}, nil
		},
		Shipped: func(ctx context.Context, sh DeliveryShipped) error {
			d := sh.DeliveryOrder
			return s.Sales.Ship(ctx, d.SourceID, d.ID, d.Number, s.today(), sh.Lines)
		},
		Delivered: func(ctx context.Context, d domain.DeliveryOrder) error {
			return s.Sales.Deliver(ctx, d.SourceID)
		},
	}
}
