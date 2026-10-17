package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/filestore"
	"balans/internal/identity"
	"balans/internal/notification"
	"balans/internal/platform/apperr"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// wireSalesReturns memasang Retur Penjualan (S-314, S-317): Inbound/QC modul warehouse untuk barang retur,
// lokasi Inbound & Gudang Retur, Merchandiser untuk pickup, notifikasi, handler approval refund nota kredit, file
// foto/form retur, dan Riwayat.
func (a *App) wireSalesReturns(mailer *notification.Module) {
	if a.sales == nil || a.warehouse == nil || a.identity == nil {
		return
	}
	m := a.sales
	d := sales.ReturnDeps{
		Inbounds: salesReturnInbounds{a.warehouse}, Locations: salesReturnLocations{a.warehouse},
		Staff: salesReturnStaff{a.identity},
	}
	if mailer != nil {
		d.Notifier = mailer
	}
	m.SetReturnDeps(d)
	a.warehouse.RegisterInboundHandler(warehouse.SourceSalesReturn, func(ctx context.Context, o warehouse.InboundOutcome) error {
		if o.SourceLineID == nil {
			return nil
		}
		return m.OnReturnInboundQC(ctx, sales.ReturnQCOutcome{
			ReturnID: o.SourceID, ItemID: *o.SourceLineID, PassedQty: o.PassedQty, RejectedQty: o.RejectedQty,
		})
	})
	if a.approval != nil {
		m.RegisterReturnApprovalHandlers(a.approval)
	}
	registerAuditEntities(a.audit, sales.ReturnAuditEntities())
	if a.filestore != nil {
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: sales.FileOwnerSalesReturnPhoto, WritePermission: sales.PermReturnWrite,
			ReadPermission: sales.PermReturnRead,
			ContentTypes:   []string{filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP, filestore.TypePDF},
			MaxBytes:       maxPaymentProofBytes,
		})
	}
}

// salesReturnInbounds membuat Inbound barang retur: lulus QC → Gudang Retur (Tersedia, menunggu disposisi);
// ditolak → Gudang Retur (Karantina).
type salesReturnInbounds struct{ warehouse *warehouse.Module }

func (i salesReturnInbounds) CreateInbound(ctx context.Context, r sales.ReturnInboundRequest) (sales.InboundRef, error) {
	loc, dest := r.InboundLocationID, r.ReturnsLocationID
	req := warehouse.InboundRequest{
		SourceType: warehouse.SourceSalesReturn, SourceID: r.ReturnID, SourceLabel: r.ReturnNumber, LocationID: &loc,
		BusinessDate: r.BusinessDate, RejectAction: warehouse.RejectToReturns, Note: r.Note,
	}
	for _, l := range r.Lines {
		itemID := l.ItemID
		req.Lines = append(req.Lines, warehouse.InboundLineInput{
			SourceLineID: &itemID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty, DestinationLocationID: &dest,
		})
	}
	ib, err := i.warehouse.CreateInbound(ctx, req)
	if err != nil {
		return sales.InboundRef{}, err
	}
	return sales.InboundRef{ID: ib.ID, Number: ib.Number}, nil
}

// salesReturnLocations menentukan lokasi Inbound dan Gudang Retur di gudang lokasi asal stok pesanan.
type salesReturnLocations struct{ warehouse *warehouse.Module }

func (l salesReturnLocations) ReturnLocations(ctx context.Context, sourceLocationID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	locs, err := l.warehouse.Locations(ctx, []uuid.UUID{sourceLocationID})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	src, ok := locs[sourceLocationID]
	if !ok || src.WarehouseID == nil {
		return uuid.Nil, uuid.Nil, apperr.Validation(map[string]string{"salesOrderId": "Lokasi asal stok pesanan bukan lokasi gudang"})
	}
	pick := func(t warehouse.LocationType, label string) (uuid.UUID, error) {
		list, err := l.warehouse.FindLocations(ctx, warehouse.LocationQuery{WarehouseID: src.WarehouseID, Types: []warehouse.LocationType{t}})
		if err != nil {
			return uuid.Nil, err
		}
		var found uuid.UUID
		for _, loc := range list {
			if found == uuid.Nil || loc.IsStandard {
				found = loc.ID
			}
		}
		if found == uuid.Nil {
			return uuid.Nil, apperr.Validation(map[string]string{"salesOrderId": fmt.Sprintf("Gudang asal tidak punya lokasi %s", label)})
		}
		return found, nil
	}
	inbound, err := pick(warehouse.LocationInbound, "Inbound")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	returns, err := pick(warehouse.LocationReturns, "Retur")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return inbound, returns, nil
}

// salesReturnStaff mencari pengguna aktif per role (Merchandiser, Logistik Leader, Fulfillment Leader).
type salesReturnStaff struct{ identity *identity.Module }

func (s salesReturnStaff) UsersWithRole(ctx context.Context, role string) ([]sales.StaffRef, error) {
	users, err := s.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	out := make([]sales.StaffRef, 0, len(users))
	for _, u := range users {
		if u.IsActive {
			out = append(out, sales.StaffRef{ID: u.ID, Name: u.FullName})
		}
	}
	return out, nil
}
