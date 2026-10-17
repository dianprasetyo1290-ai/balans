package app

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/logistics"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// wireConsignmentDeliveries menghubungkan pengiriman titipan (konsinyasi / buffer vending, S-303, S-219) dengan
// Delivery Order logistik: sales membuat DO saat Sales MT mengajukan, dan DO memanggil balik sales untuk QC
// Outbound (stok pengganti), serah ke kurir (Transfer Stok + A-07/A-24), diterima, dan batal.
func (a *App) wireConsignmentDeliveries() {
	if a.sales == nil || a.logistics == nil || !a.logistics.DocumentsEnabled() {
		return
	}
	s, l := a.sales, a.logistics
	s.SetDeliveryOrders(salesDeliveryOrders{logistics: l})
	l.RegisterDeliveryHandler(sales.SourceConsignmentDelivery, logistics.DeliveryHandler{
		Replace: func(ctx context.Context, do logistics.DeliveryOrder, r warehouse.OutboundRejection) (warehouse.OutboundReplacement, error) {
			if r.SourceLineID == nil {
				return warehouse.OutboundReplacement{}, apperr.Validation(map[string]string{"lineId": "Baris Outbound tanpa baris pengiriman titipan"})
			}
			// Ref unik per aksi ledger: baris ditolak QC sekali; alokasi ulang baris ditahan bisa berulang.
			ref := r.LineID.String()
			if !r.Qty.IsPositive() {
				ref = "refill-" + uuid.NewString()
			}
			res, err := s.ReplaceConsignmentBatch(ctx, sales.ConsignmentReplaceInput{
				DeliveryID: do.SourceID, LineID: *r.SourceLineID, RejectBatchID: r.BatchID, RejectQty: r.Qty, Need: r.Need,
				Ref: ref, Label: do.Number,
			})
			if err != nil {
				return warehouse.OutboundReplacement{}, err
			}
			out := warehouse.OutboundReplacement{QuarantineLocationID: res.QuarantineLocationID, Shortfall: res.Shortfall}
			for _, b := range res.Replacements {
				out.Batches = append(out.Batches, warehouse.ReplacementBatch{BatchID: b.BatchID, Qty: b.Qty})
			}
			return out, nil
		},
		Shipped: func(ctx context.Context, sh logistics.DeliveryShipped) error {
			do := sh.DeliveryOrder
			in := sales.ConsignmentShipmentInput{
				DeliveryID: do.SourceID, SourceType: sales.SourceDeliveryOrder, SourceID: do.ID, SourceLabel: do.Number,
				BusinessDate: clock.Today(a.Clock),
			}
			type key struct {
				line  uuid.UUID
				batch uuid.UUID
			}
			idx := map[key]int{}
			for _, ol := range sh.Outbound.Lines {
				if ol.Status != warehouse.OutboundPassed || ol.SourceLineID == nil || !ol.PassedQty.IsPositive() {
					continue
				}
				k := key{line: *ol.SourceLineID}
				if ol.BatchID != nil {
					k.batch = *ol.BatchID
				}
				if i, ok := idx[k]; ok {
					in.Lines[i].Qty = in.Lines[i].Qty.Add(ol.PassedQty)
					continue
				}
				idx[k] = len(in.Lines)
				in.Lines = append(in.Lines, sales.ConsignmentShippedLine{SourceLineID: k.line, BatchID: ol.BatchID, Qty: ol.PassedQty})
			}
			_, err := s.ShipConsignment(ctx, in)
			return err
		},
		Delivered: func(ctx context.Context, do logistics.DeliveryOrder) error {
			_, err := s.ConfirmConsignmentReceived(ctx, do.SourceID)
			return err
		},
		Cancelled: func(ctx context.Context, do logistics.DeliveryOrder) error {
			return s.ReopenConsignmentDelivery(ctx, do.SourceID, do.ID, do.CancelReason)
		},
	})
}

// salesDeliveryOrders menyesuaikan API Delivery Order logistics untuk sales (DO pengiriman titipan, DO Sales Order).
type salesDeliveryOrders struct{ logistics *logistics.Module }

func (d salesDeliveryOrders) CreateConsignmentDeliveryOrder(ctx context.Context, in sales.DeliveryOrderRequest) (sales.DeliveryOrderRef, error) {
	t := logistics.DeliveryConsignment
	if in.Type == sales.DeliveryVendingBuffer {
		t = logistics.DeliveryVendingBuffer
	}
	partner := in.PartnerID
	req := logistics.DeliveryRequest{
		Type: t, SourceType: sales.SourceConsignmentDelivery, SourceID: in.DeliveryID, SourceLabel: in.SourceLabel,
		PartnerID: &partner, RecipientName: in.RecipientName, RecipientPhone: in.RecipientPhone,
		ShipToAddress: in.ShipToAddress, LocationID: in.LocationID, BusinessDate: in.BusinessDate, Note: in.Note,
	}
	for _, l := range in.Lines {
		lineID := l.SourceLineID
		li := logistics.DeliveryLineInput{SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.Qty}
		for _, b := range l.Batches {
			li.Batches = append(li.Batches, logistics.BatchQty{BatchID: b.BatchID, Qty: b.Qty.StringFixed(3)})
		}
		req.Lines = append(req.Lines, li)
	}
	do, err := d.logistics.CreateDeliveryOrder(ctx, req)
	if err != nil {
		return sales.DeliveryOrderRef{}, err
	}
	return sales.DeliveryOrderRef{ID: do.ID, Number: do.Number, Status: string(do.Status)}, nil
}

func (d salesDeliveryOrders) DeliveryOrdersBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]sales.DeliveryOrderRef, error) {
	list, err := d.logistics.DeliveryOrdersBySource(ctx, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]sales.DeliveryOrderRef, len(list))
	for i, do := range list {
		out[i] = sales.DeliveryOrderRef{ID: do.ID, Number: do.Number, Status: string(do.Status)}
	}
	return out, nil
}
