package app

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/filestore"
	"balans/internal/identity"
	"balans/internal/inventory"
	"balans/internal/warehouse"
)

// maxPhotoBytes adalah batas ukuran foto QC dan stock count setelah dikompres di browser (± 1 MB, TG 10.10).
const maxPhotoBytes = 5 << 20

// stockLedger menyesuaikan API publik inventory untuk dokumen gudang (warehouse tidak mengimpor inventory
// karena inventory membaca master lokasi dari warehouse).
type stockLedger struct {
	inv *inventory.Module
}

func toInventorySource(s warehouse.StockSource) inventory.Source {
	return inventory.Source{Type: s.Type, ID: s.ID, Action: s.Action, Label: s.Label, BusinessDate: s.BusinessDate}
}

func toPlace(p *warehouse.StockPlace) *inventory.Place {
	if p == nil {
		return nil
	}
	return &inventory.Place{LocationID: p.LocationID, Status: inventory.StockStatus(p.Status)}
}

func toWarehouseResult(r inventory.PostingResult, n int) warehouse.StockResult {
	out := warehouse.StockResult{Lines: make([]warehouse.StockLineResult, n)}
	for _, l := range r.Lines {
		if l.Index < 0 || l.Index >= n {
			continue
		}
		res := warehouse.StockLineResult{CostAmount: l.CostAmount}
		for _, m := range l.Movements {
			res.Movements = append(res.Movements, warehouse.StockMovement{
				LineIndex: m.LineIndex, LocationID: m.LocationID, BatchID: m.BatchID, Status: string(m.Status),
				Qty: m.Qty, CostAmount: m.CostAmount,
			})
		}
		out.Lines[l.Index] = res
	}
	return out
}

func (s stockLedger) Post(ctx context.Context, src warehouse.StockSource, lines []warehouse.StockLine) (warehouse.StockResult, error) {
	p := inventory.Posting{Source: toInventorySource(src)}
	for _, l := range lines {
		il := inventory.Line{
			Kind: inventory.LineKind(l.Kind), SourceLineID: l.SourceLineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID,
			From: toPlace(l.From), To: toPlace(l.To), UnitCost: l.UnitCost, BatchReason: l.BatchReason, Note: l.Note,
		}
		if l.Reservation != nil {
			il.Reservation = &inventory.ReservationRef{
				SourceType: l.Reservation.SourceType, SourceID: l.Reservation.SourceID, SourceLineID: l.Reservation.SourceLineID,
			}
		}
		p.Lines = append(p.Lines, il)
	}
	res, err := s.inv.Post(ctx, p)
	if err != nil {
		return warehouse.StockResult{}, err
	}
	return toWarehouseResult(res, len(lines)), nil
}

func (s stockLedger) Reserve(ctx context.Context, src warehouse.StockSource, lines []warehouse.StockReserveLine) (warehouse.StockResult, error) {
	req := inventory.ReserveRequest{Source: toInventorySource(src)}
	for _, l := range lines {
		req.Lines = append(req.Lines, inventory.ReserveLine{
			SourceLineID: l.SourceLineID, ProductID: l.ProductID, LocationID: l.LocationID, BatchID: l.BatchID, Qty: l.Qty,
			BatchReason: l.BatchReason,
		})
	}
	res, err := s.inv.Reserve(ctx, req)
	if err != nil {
		return warehouse.StockResult{}, err
	}
	return toWarehouseResult(res, len(lines)), nil
}

func (s stockLedger) Release(ctx context.Context, src warehouse.StockSource, ref warehouse.StockReservationRef) error {
	_, err := s.inv.Release(ctx, inventory.ReleaseRequest{
		Source:      toInventorySource(src),
		Reservation: inventory.ReservationRef{SourceType: ref.SourceType, SourceID: ref.SourceID, SourceLineID: ref.SourceLineID},
	})
	return err
}

func (s stockLedger) Balances(ctx context.Context, q warehouse.StockBalanceQuery) ([]warehouse.StockBalance, error) {
	f := inventory.BalanceFilter{ProductID: q.ProductID, LocationIDs: q.LocationIDs, BatchID: q.BatchID}
	for _, st := range q.Statuses {
		f.Statuses = append(f.Statuses, inventory.StockStatus(st))
	}
	rows, err := s.inv.Balances(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]warehouse.StockBalance, len(rows))
	for i, r := range rows {
		out[i] = warehouse.StockBalance{
			ProductID: r.Key.ProductID, LocationID: r.Key.LocationID, BatchID: r.Key.BatchID, Status: string(r.Key.Status),
			Qty: r.Qty, UnitCost: r.AvgUnitCost,
		}
	}
	return out, nil
}

func (s stockLedger) Batches(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]warehouse.BatchInfo, error) {
	m, err := s.inv.Batches(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]warehouse.BatchInfo, len(m))
	for id, b := range m {
		out[id] = warehouse.BatchInfo{ID: b.ID, Number: b.Number, ExpiryDate: b.ExpiryDate}
	}
	return out, nil
}

// warehouseUsers mencari user aktif per role dan nama user dari identity.
type warehouseUsers struct {
	identity *identity.Module
}

func (u warehouseUsers) UserIDsWithRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	users, err := u.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(users))
	for _, x := range users {
		out = append(out, x.ID)
	}
	return out, nil
}

func (u warehouseUsers) UserName(ctx context.Context, id uuid.UUID) (string, error) {
	x, err := u.identity.UserByID(ctx, id)
	if err != nil {
		return "", err
	}
	return x.FullName, nil
}

// connectWarehouseDocuments memasang dependensi dokumen gudang (R1.2) setelah inventory disusun: ledger stok,
// penomoran, katalog, approval `stock_count` (menggantikan handler dev), jurnal A-14, foto, mitra, notifikasi.
func (a *App) connectWarehouseDocuments() {
	if a.warehouse == nil || a.inventory == nil || a.document == nil || a.catalog == nil {
		return
	}
	d := warehouse.DocumentDeps{Ledger: stockLedger{inv: a.inventory}, Numbers: a.document, Products: a.catalog}
	if a.approval != nil {
		d.Approvals = a.approval
		a.approval.RegisterHandler(approval.SubjectStockCount, warehouse.PermCountRead, a.warehouse.ApprovalHandler())
	}
	if a.accounting != nil {
		d.Poster, d.Divisions = a.accounting, a.accounting
	}
	if a.filestore != nil {
		d.Files = a.filestore
		photos := []string{filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP}
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: warehouse.FileOwnerQC, WritePermission: warehouse.PermInboundPost, ReadPermission: warehouse.PermInboundRead,
			ContentTypes: photos, MaxBytes: maxPhotoBytes,
		})
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: warehouse.FileOwnerCount, WritePermission: warehouse.PermCountWrite, ReadPermission: warehouse.PermCountRead,
			ContentTypes: photos, MaxBytes: maxPhotoBytes,
		})
	}
	if a.party != nil {
		d.Partners = a.party
	}
	if a.notification != nil {
		d.Notifier = a.notification
	}
	if a.identity != nil {
		d.Users = warehouseUsers{identity: a.identity}
	}
	a.warehouse.SetDocumentDeps(d)
}
