package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/sales/domain"
)

// API pengiriman untuk modul logistics (DO ship / serah ke kurir marketplace). Dipanggil di transaksi dokumen
// logistik; tidak memeriksa permission karena pemanggil memeriksa aksi user-nya sendiri.

// ShipmentInput adalah barang Sales Order yang keluar lewat satu dokumen logistik.
type ShipmentInput struct {
	OrderID uuid.UUID
	// SourceType/SourceID/SourceLabel adalah dokumen pengiriman, mis. "delivery_order" + ID DO + nomor DO. Satu
	// dokumen sumber hanya dicatat sekali per pesanan (pemanggilan ulang mengembalikan pengiriman yang sama).
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	BusinessDate clock.BusinessDate
	// Lines kosong = seluruh sisa alokasi. Marketplace wajib seluruh pesanan.
	Lines []domain.ShipmentQty
}

// ShippableOrder mengembalikan pesanan Diproses beserta alokasi (batch FEFO, qty sisa) untuk dibuatkan DO.
func (s *DocumentService) ShippableOrder(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	o, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return domain.Order{}, err
	}
	if o.Status != domain.StatusInProgress {
		return domain.Order{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Sales Order %s berstatus %s; hanya pesanan Diproses yang bisa dikirim.", o.Label(), o.Status.Label()))
	}
	return o, nil
}

// MarkShipped mencatat barang keluar: stok reserved pesanan di-issue (HPP FIFO) lalu jurnal dibuat di transaksi
// yang sama — A-06 (Dr HPP, Cr Persediaan FG) untuk MT/B2B/WA-IG/karyawan yang penjualannya sudah diakui saat
// invoice, atau A-10 (piutang marketplace, voucher penjual, penjualan, HPP) untuk marketplace yang diakui saat
// serah ke kurir (U-25). Semua alokasi terkirim → pesanan Dalam Pengiriman.
func (s *DocumentService) MarkShipped(ctx context.Context, in ShipmentInput) (domain.Shipment, error) {
	if in.SourceType == "" || in.SourceID == uuid.Nil {
		return domain.Shipment{}, apperr.Validation(map[string]string{"source": "Dokumen pengiriman wajib diisi"})
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = s.today()
	}
	var out domain.Shipment
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.GetOrder(ctx, in.OrderID, true)
		if err != nil {
			return err
		}
		if sh, ok, err := s.Store.ShipmentBySource(ctx, o.ID, in.SourceType, in.SourceID); err != nil || ok {
			out = sh
			return err
		}
		lines, err := o.PlanShipment(in.Lines)
		if err != nil {
			return err
		}
		out, err = s.ship(ctx, &o, in, lines)
		return err
	})
	return out, err
}

// MarkHandedOver adalah MarkShipped khusus pesanan marketplace (serah ke kurir = titik pengakuan penjualan A-10).
func (s *DocumentService) MarkHandedOver(ctx context.Context, in ShipmentInput) (domain.Shipment, error) {
	o, err := s.Store.GetOrder(ctx, in.OrderID, false)
	if err != nil {
		return domain.Shipment{}, err
	}
	if o.Type != domain.OrderMarketplace {
		return domain.Shipment{}, apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Serah ke kurir hanya untuk pesanan marketplace; pesanan lain memakai MarkShipped.")
	}
	in.Lines = nil
	return s.MarkShipped(ctx, in)
}

func (s *DocumentService) ship(ctx context.Context, o *domain.Order, in ShipmentInput, lines []domain.ShipmentQty) (domain.Shipment, error) {
	byID := map[uuid.UUID]domain.Allocation{}
	for _, a := range o.Allocations {
		byID[a.ID] = a
	}
	posting := make([]inventory.Line, len(lines))
	for i, l := range lines {
		a := byID[l.AllocationID]
		allocID := a.ID
		posting[i] = inventory.Line{
			Kind: inventory.KindIssue, SourceLineID: &allocID, ProductID: a.ProductID, Qty: l.Qty,
			From:        &inventory.Place{LocationID: a.LocationID, Status: inventory.StatusReserved},
			Reservation: &inventory.ReservationRef{SourceType: SourceSalesOrder, SourceID: o.ID, SourceLineID: &allocID},
		}
	}
	label := in.SourceLabel
	if label == "" {
		label = o.Number
	}
	res, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{
			Type: SourceSalesOrder, ID: o.ID, Action: "ship:" + in.SourceID.String(), Label: label, BusinessDate: in.BusinessDate,
		},
		Lines: posting,
	})
	if err != nil {
		return domain.Shipment{}, err
	}
	sh := domain.Shipment{
		OrderID: o.ID, SourceType: in.SourceType, SourceID: in.SourceID, SourceLabel: label,
		BusinessDate: in.BusinessDate, Cost: money.Zero,
	}
	for i, l := range lines {
		cost := money.Zero
		if i < len(res.Lines) {
			cost = money.FromDecimal(res.Lines[i].CostAmount)
		}
		sh.Lines = append(sh.Lines, domain.ShipmentLine{AllocationID: l.AllocationID, ProductID: byID[l.AllocationID].ProductID, Qty: l.Qty, Cost: cost})
		sh.Cost = sh.Cost.Add(cost)
	}
	if sh, err = s.Store.InsertShipment(ctx, sh, actor.UserID(ctx)); err != nil {
		return domain.Shipment{}, err
	}
	if err := s.postShipment(ctx, *o, sh); err != nil {
		return domain.Shipment{}, err
	}
	if err := s.saveOrderChange(ctx, o, audit.ActionStatusChange, false, func(ctx context.Context, o *domain.Order) (string, error) {
		if err := o.ApplyShipment(lines, s.Clock.Now()); err != nil {
			return "", err
		}
		return "Dikirim lewat " + label, s.Store.SaveAllocationState(ctx, o.Allocations)
	}); err != nil {
		return domain.Shipment{}, err
	}
	return sh, nil
}

// postShipment memposting HPP (A-06) atau penjualan marketplace (A-10) pengiriman dengan dimensi pesanan.
func (s *DocumentService) postShipment(ctx context.Context, o domain.Order, sh domain.Shipment) error {
	req := accounting.PostingRequest{
		SourceType: SourceShipment, SourceID: sh.ID, SourceLabel: sh.SourceLabel, BusinessDate: sh.BusinessDate,
		Dimensions: salesDimensions(o.ChannelID, o.SubChannelID),
	}
	if !o.Type.RecognizedAtHandover() {
		if !sh.Cost.IsPositive() {
			return nil
		}
		req.RuleCode = RuleCOGS
		req.Amounts = map[string]money.Amount{"cogs_value": sh.Cost}
		req.Accounts = map[accounting.Resolver]string{accounting.ResolverLocationInventory: AccountInventory}
		req.Description = "HPP " + o.Number
		_, err := s.Ledger.Post(ctx, req)
		return err
	}
	labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{o.SubChannelID})
	if err != nil {
		return err
	}
	req.RuleCode = RuleMarketplaceSales
	req.Amounts = map[string]money.Amount{
		"gross_sales": o.Gross, "seller_voucher": o.TotalDiscount(), "receivable_value": o.Total, "cogs_value": sh.Cost,
	}
	if acct := domain.MarketplaceReceivableAccount(labels[o.SubChannelID].Code); acct != "" {
		req.Accounts = map[accounting.Resolver]string{accounting.ResolverReceivable: acct}
	}
	req.Description = fmt.Sprintf("Penjualan marketplace %s (%s)", o.ExternalOrderID, o.Number)
	_, err = s.Ledger.Post(ctx, req)
	return err
}

// MarkDelivered menandai pesanan Dalam Pengiriman sudah diterima pelanggan (Selesai).
func (s *DocumentService) MarkDelivered(ctx context.Context, orderID uuid.UUID) (domain.Order, error) {
	var out domain.Order
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.GetOrder(ctx, orderID, true)
		if err != nil {
			return err
		}
		if err := s.saveOrderChange(ctx, &o, audit.ActionStatusChange, false, func(_ context.Context, o *domain.Order) (string, error) {
			return "Diterima pelanggan", o.Deliver(s.Clock.Now())
		}); err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}
