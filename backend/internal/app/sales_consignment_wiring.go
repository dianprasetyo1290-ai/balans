package app

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/inventory"
	"balans/internal/party"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// salesConsignment mengisi sales.ConsignmentStock dari API publik warehouse: lokasi konsinyasi/buffer vending
// mitra, Transfer Stok ke lokasi mitra, penerimaan transfer, dan hasil stock count konsinyasi.
type salesConsignment struct{ warehouse *warehouse.Module }

func (c salesConsignment) PartnerLocation(ctx context.Context, locationType string, partnerID uuid.UUID) (sales.LocationRef, bool, error) {
	l, ok, err := c.warehouse.ExternalLocation(ctx, warehouse.LocationType(locationType), partnerID)
	if err != nil || !ok {
		return sales.LocationRef{}, ok, err
	}
	return toSalesLocation(l), true, nil
}

func (c salesConsignment) Transfer(ctx context.Context, in sales.StockTransferInput) (sales.StockTransferResult, error) {
	lines := make([]warehouse.TransferLineInput, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = warehouse.TransferLineInput{ProductID: l.ProductID, Qty: l.Qty, Note: l.Note}
	}
	res, err := c.warehouse.TransferStock(ctx, warehouse.TransferRequest{
		SourceType: in.SourceType, SourceID: in.SourceID, FromLocationID: in.FromLocationID, ToLocationID: in.ToLocationID,
		BusinessDate: in.BusinessDate, Note: in.Note, Lines: lines,
	})
	if err != nil {
		return sales.StockTransferResult{}, err
	}
	out := sales.StockTransferResult{ID: res.Transfer.ID, Number: res.Transfer.Number}
	for _, l := range res.Transfer.Lines {
		out.Lines = append(out.Lines, sales.TransferredLine{
			ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty, Cost: money.FromDecimal(l.CostAmount),
		})
	}
	return out, nil
}

// ReceiveAll menerima seluruh sisa transfer Dalam Pengiriman; transfer dalam satu gudang yang sudah Selesai
// dilewati.
func (c salesConsignment) ReceiveAll(ctx context.Context, transferID uuid.UUID) error {
	t, err := c.warehouse.Transfer(ctx, transferID)
	if err != nil {
		return err
	}
	var lines []warehouse.ReceiveLine
	for _, l := range t.Lines {
		if q := l.Outstanding(); q.IsPositive() {
			lines = append(lines, warehouse.ReceiveLine{LineID: l.ID, Qty: q})
		}
	}
	if len(lines) == 0 {
		return nil
	}
	_, err = c.warehouse.ReceiveTransfer(ctx, transferID, lines, true)
	return err
}

func (c salesConsignment) StockCount(ctx context.Context, id uuid.UUID) (sales.StockCountRef, error) {
	sc, err := c.warehouse.StockCountResult(ctx, id)
	if err != nil {
		return sales.StockCountRef{}, err
	}
	out := sales.StockCountRef{
		ID: sc.ID, Number: sc.Number, CountType: string(sc.CountType), LocationID: sc.LocationID,
		Status: string(sc.Status), BusinessDate: sc.BusinessDate,
	}
	for _, l := range sc.Lines {
		if l.CountedQty == nil {
			continue
		}
		out.Lines = append(out.Lines, sales.CountedLine{ProductID: l.ProductID, BatchID: l.BatchID, CountedQty: *l.CountedQty})
	}
	return out, nil
}

// salesAgreements membaca PKS konsinyasi berlaku dari modul party.
type salesAgreements struct{ party *party.Module }

func (a salesAgreements) RequireConsignment(ctx context.Context, partnerID uuid.UUID, on clock.BusinessDate) (sales.AgreementRef, error) {
	ag, err := a.party.RequireActiveAgreement(ctx, partnerID, party.AgreementConsignment, on)
	if err != nil {
		return sales.AgreementRef{}, err
	}
	return sales.AgreementRef{
		ID: ag.ID, Number: ag.Number, PaymentTermDays: ag.PaymentTermDays, MarginPercent: ag.MarginPercent,
		PriceListID: ag.PriceListID,
		Rules: sales.VarianceRules{
			Lost: sales.BorneBy(ag.LossBorneBy), PartnerDamage: sales.BorneBy(ag.PartnerDamageBorneBy),
			ProductDefect: sales.BorneBy(ag.ProductDefectBorneBy), NearExpiry: sales.BorneBy(ag.NearExpiryBorneBy),
		},
	}, nil
}

var _ sales.StockFlows = (*inventory.Module)(nil)
