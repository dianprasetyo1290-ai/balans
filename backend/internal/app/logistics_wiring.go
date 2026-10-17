package app

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/filestore"
	"balans/internal/identity"
	"balans/internal/integration/mockshipment"
	"balans/internal/logistics"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/webhook"
	"balans/internal/procurement"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// buildLogistics menyusun modul logistics: webhook kurir (mock Biteship) dan, bila modul gudang/persediaan aktif,
// Delivery Order, Outbound DO, kurir & resi, serta pengiriman (R1.3).
func (a *App) buildLogistics(inbox *webhook.Inbox, provider *mockshipment.Provider) {
	d := logistics.Deps{Provider: provider, Inbox: inbox, Responder: a.Responder, Log: a.Log}
	if a.warehouse != nil && a.inventory != nil && a.catalog != nil && a.document != nil {
		d.Pool, d.Tx, d.Audit, d.Clock = a.Pool, a.Tx, a.audit, a.Clock
		d.Warehouse, d.Products, d.Batches, d.Numberer = a.warehouse, a.catalog, a.inventory, a.document
		if a.sales != nil {
			d.Sales = logisticsSales{sales: a.sales}
		}
		if a.party != nil {
			d.Partners = a.party
		}
		if a.filestore != nil {
			d.Files = a.filestore
			a.filestore.RegisterOwner(filestore.OwnerPolicy{
				OwnerType: logistics.FileOwnerDeliveryProof, WritePermission: logistics.PermShipmentPost,
				ReadPermission: logistics.PermShipmentRead, ContentTypes: []string{filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP, filestore.TypePDF},
				MaxBytes: maxPhotoBytes,
			})
		}
		if a.identity != nil {
			d.Users = logisticsUsers{identity: a.identity}
		}
		if a.notification != nil {
			d.Notifier = a.notification
		}
		registerAuditEntities(a.audit, logistics.AuditEntities())
	}
	a.logistics = logistics.New(d)
	a.modules = append(a.modules, a.logistics)
	a.wireConsignmentDeliveries()
}

// logisticsSales menyesuaikan API publik sales untuk DO (sales berada di tingkat atas logistics).
type logisticsSales struct{ sales *sales.Module }

func (s logisticsSales) ShippableOrder(ctx context.Context, id uuid.UUID) (logistics.SalesOrder, error) {
	o, err := s.sales.ShippableOrder(ctx, id)
	if err != nil {
		return logistics.SalesOrder{}, err
	}
	out := logistics.SalesOrder{
		ID: o.ID, Number: o.Number, Marketplace: o.Type == sales.OrderMarketplace, PartnerID: o.PartnerID,
		CustomerID: o.CustomerID, RecipientName: o.ShipToName, RecipientPhone: o.ShipToPhone, ShipToAddress: o.ShipToAddress,
		LocationID: o.SourceLocationID,
	}
	if out.RecipientName == "" {
		out.RecipientName = o.PartyName
	}
	if out.RecipientPhone == "" {
		out.RecipientPhone = o.PartyPhone
	}
	for _, al := range o.Allocations {
		if al.Released {
			continue
		}
		la := logistics.SalesAllocation{
			ID: al.ID, LineID: al.LineID, ProductID: al.ProductID, LocationID: al.LocationID,
			Outstanding: al.OutstandingQty(), Gap: al.Gap(),
		}
		for _, b := range al.OutstandingBatches() {
			la.Batches = append(la.Batches, warehouse.ReplacementBatch{BatchID: b.BatchID, Qty: b.Qty})
		}
		out.Allocations = append(out.Allocations, la)
	}
	return out, nil
}

func (s logisticsSales) Reallocate(ctx context.Context, orderID, allocationID uuid.UUID, batchID *uuid.UUID, qty decimal.Decimal, ref, label string) (logistics.SalesReallocation, error) {
	r, err := s.sales.ReallocateBatch(ctx, sales.ReallocateInput{
		OrderID: orderID, AllocationID: allocationID, RejectBatchID: batchID, RejectQty: qty, Ref: ref, Label: label,
		Note: "Ditolak QC Outbound",
	})
	if err != nil {
		return logistics.SalesReallocation{}, err
	}
	out := logistics.SalesReallocation{QuarantineLocationID: r.QuarantineLocationID, Shortfall: r.Shortfall}
	for _, b := range r.Replacements {
		out.Replacements = append(out.Replacements, warehouse.ReplacementBatch{BatchID: b.BatchID, Qty: b.Qty})
	}
	return out, nil
}

func (s logisticsSales) Ship(ctx context.Context, orderID, doID uuid.UUID, label string, date clock.BusinessDate, lines map[uuid.UUID]decimal.Decimal) error {
	in := sales.ShipmentInput{OrderID: orderID, SourceType: logistics.EntityDeliveryOrder, SourceID: doID, SourceLabel: label, BusinessDate: date}
	o, err := s.sales.ShippableOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if o.Type == sales.OrderMarketplace {
		_, err = s.sales.MarkHandedOver(ctx, in)
		return err
	}
	for allocID, q := range lines {
		if q.IsPositive() {
			in.Lines = append(in.Lines, sales.ShipmentQty{AllocationID: allocID, Qty: q})
		}
	}
	_, err = s.sales.MarkShipped(ctx, in)
	return err
}

func (s logisticsSales) Deliver(ctx context.Context, orderID uuid.UUID) error {
	_, err := s.sales.MarkDelivered(ctx, orderID)
	// Pesanan dengan sisa barang belum dikirim tetap Diproses; DO berikutnya menyelesaikannya.
	if apperr.HasCode(err, apperr.CodeInvalidStateTransition) {
		return nil
	}
	return err
}

// logisticsUsers memeriksa role user dari identity (kurir internal harus Merchandiser).
type logisticsUsers struct{ identity *identity.Module }

func (u logisticsUsers) UserWithRole(ctx context.Context, userID uuid.UUID, role string) (string, bool, error) {
	users, err := u.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return "", false, err
	}
	for _, x := range users {
		if x.ID == userID {
			return x.FullName, true, nil
		}
	}
	return "", false, nil
}

func (u logisticsUsers) UsersWithRole(ctx context.Context, role string) ([]logistics.UserOption, error) {
	users, err := u.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	out := make([]logistics.UserOption, 0, len(users))
	for _, x := range users {
		out = append(out, logistics.UserOption{ID: x.ID, Name: x.FullName})
	}
	slices.SortFunc(out, func(a, b logistics.UserOption) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// outboundQCReturns membuat retur pembelian otomatis (nota debit, A-02) dari barang reject QC Outbound yang berasal
// dari Penerimaan Barang pemasok/maklon. Barang tanpa GR (mis. saldo awal) tetap di Karantina.
type outboundQCReturns struct {
	procurement *procurement.Module
	clock       clock.Clock
}

func (r outboundQCReturns) ReturnRejected(ctx context.Context, o warehouse.OutboundReturn) (string, error) {
	reason := "Ditolak QC Outbound " + o.OutboundNumber
	if o.Reason != "" {
		reason += ": " + o.Reason
	}
	var number string
	err := db.Savepoint(ctx, func(ctx context.Context) error {
		ret, err := r.procurement.CreateReturnFromQC(ctx, procurement.QCReturnInput{
			Origin: procurement.ReturnQCOutbound, SourceType: "outbound_line", SourceID: o.LineID, SourceLabel: o.OutboundNumber,
			BusinessDate: clock.Today(r.clock), Reason: reason,
			Lines: []procurement.QCReturnLine{{
				ProductID: o.ProductID, BatchID: o.BatchID, LocationID: o.LocationID, StockStatus: "quarantine", Qty: o.Qty,
			}},
		})
		number = ret.Number
		return err
	})
	if e := apperr.As(err); err != nil && e.Code == apperr.CodeValidationFailed && notFromReceipt(e.Fields) {
		return "", nil
	}
	return number, err
}

// notFromReceipt: batch tidak berasal dari Penerimaan Barang (retur tidak bisa dibuat).
func notFromReceipt(fields map[string]string) bool {
	for k := range fields {
		if strings.HasSuffix(k, "receiptLineId") || k == "lines" {
			return true
		}
	}
	return false
}
