package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/identity"
	"balans/internal/notification"
	"balans/internal/party"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/procurement"
	"balans/internal/warehouse"
)

// buildProcurement menyusun modul procurement (R1.2) dan mendaftarkan handler approval PR/PO setelah handler dev
// agar menggantikannya.
func (a *App) buildProcurement(mailer *notification.Module) error {
	if !a.Config.IsEnabled(procurement.Name) {
		return nil
	}
	if a.approval == nil || a.accounting == nil || a.document == nil || a.catalog == nil || a.party == nil ||
		a.warehouse == nil || a.identity == nil || mailer == nil {
		return errors.New("modul procurement membutuhkan modul approval, accounting, document, catalog, party, warehouse, identity, dan notification")
	}
	m := procurement.New(procurement.Deps{
		Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit,
		Catalog: procurementCatalog{a.catalog}, Suppliers: procurementSuppliers{a.party},
		Locations: procurementLocations{a.warehouse}, Ledger: procurementLedger{a.accounting}, Approvals: a.approval,
		Numberer: procurementNumberer{a.document}, Users: procurementUsers{a.identity}, Notifier: mailer,
		BaseURL: a.Config.PublicBaseURL,
	})
	m.RegisterApprovalHandlers(a.approval)
	registerAuditEntities(a.audit, procurement.AuditEntities())
	a.modules = append(a.modules, m)
	return nil
}

// procurementCatalog membaca produk katalog untuk PR/PO.
type procurementCatalog struct{ catalog *catalog.Module }

func (c procurementCatalog) Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]procurement.ProductRef, error) {
	out := make(map[uuid.UUID]procurement.ProductRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	products, err := c.catalog.Products(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, p := range products {
		out[id] = procurement.ProductRef{
			ID: p.ID, SKU: p.SKU, Name: p.DisplayName(), UOM: p.UOM, Type: string(p.Type), IsBundle: p.IsBundle,
			IsActive: p.IsActive, Deleted: p.Deleted(),
		}
	}
	return out, nil
}

// procurementSuppliers membaca pemasok/maklon.
type procurementSuppliers struct{ party *party.Module }

func (s procurementSuppliers) Supplier(ctx context.Context, id uuid.UUID) (procurement.SupplierRef, bool, error) {
	sup, err := s.party.SupplierByID(ctx, id)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return procurement.SupplierRef{}, false, nil
	}
	if err != nil {
		return procurement.SupplierRef{}, false, err
	}
	return procurement.SupplierRef{
		ID: sup.ID, Code: sup.Code, Name: sup.Name, IsTollManufacturer: sup.Type == party.SupplierTollManufacturer,
		PaymentTermDays: sup.PaymentTermDays, Address: sup.Address, Phone: sup.Phone, Email: sup.Email, PICName: sup.PICName,
	}, true, nil
}

// procurementLocations membaca lokasi gudang dan lokasi eksternal maklon.
type procurementLocations struct{ warehouse *warehouse.Module }

func toProcurementLocation(l warehouse.Location) procurement.LocationRef {
	return procurement.LocationRef{
		ID: l.ID, Code: l.Code, Name: l.Name, Type: string(l.Type), IsActive: l.IsActive, Deleted: l.Deleted(),
	}
}

func (l procurementLocations) TollLocation(ctx context.Context, supplierID uuid.UUID) (*procurement.LocationRef, error) {
	loc, ok, err := l.warehouse.ExternalLocation(ctx, warehouse.LocationTollManufacturer, supplierID)
	if err != nil || !ok {
		return nil, err
	}
	ref := toProcurementLocation(loc)
	return &ref, nil
}

func (l procurementLocations) Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]procurement.LocationRef, error) {
	out := make(map[uuid.UUID]procurement.LocationRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	locs, err := l.warehouse.Locations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, loc := range locs {
		out[id] = toProcurementLocation(loc)
	}
	return out, nil
}

// procurementLedger menyesuaikan API publik accounting: divisi, akun Kas & Bank, dan Poster.
type procurementLedger struct{ accounting *accounting.Module }

func (l procurementLedger) DivisionOptions(ctx context.Context) ([]procurement.Option, error) {
	list, err := l.accounting.DivisionOptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]procurement.Option, len(list))
	for i, d := range list {
		out[i] = procurement.Option{ID: d.ID, Code: d.Code, Name: d.Name}
	}
	return out, nil
}

func (l procurementLedger) DivisionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return l.accounting.DivisionNames(ctx, ids)
}

func (l procurementLedger) CheckDivision(ctx context.Context, id uuid.UUID, at clock.BusinessDate, field string) error {
	return l.accounting.CheckDivision(ctx, id, at, field)
}

// cashBankPrefix adalah awalan kode akun Kas & Bank (COA Desain Finance & Accounting 1).
const cashBankPrefix = "11"

func (l procurementLedger) CashBankAccounts(ctx context.Context) ([]procurement.Option, error) {
	list, err := l.accounting.PostableAccounts(ctx, cashBankPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]procurement.Option, len(list))
	for i, a := range list {
		out[i] = procurement.Option{ID: a.ID, Code: a.Code, Name: a.Name}
	}
	return out, nil
}

func (l procurementLedger) Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error) {
	return l.accounting.Post(ctx, req)
}

func (l procurementLedger) Reverse(ctx context.Context, req accounting.ReversalRequest) (accounting.JournalEntry, error) {
	return l.accounting.Reverse(ctx, req)
}

// procurementNumberer mengambil nomor PR, PO, dan BYR dari modul document.
type procurementNumberer struct{ documents *document.Module }

func (n procurementNumberer) Next(ctx context.Context, t procurement.DocumentType, businessDate clock.BusinessDate) (string, error) {
	var dt document.Type
	switch t {
	case procurement.DocPurchaseRequest:
		dt = document.TypePurchaseRequest
	case procurement.DocPurchaseOrder:
		dt = document.TypePurchaseOrder
	case procurement.DocPayment:
		dt = document.TypePayment
	default:
		return "", fmt.Errorf("jenis nomor dokumen procurement %q tidak dikenal", t)
	}
	return n.documents.Next(ctx, dt, businessDate)
}

// procurementUsers membaca pengguna untuk nama peminta/pembuat dan penerima notifikasi ETA.
type procurementUsers struct{ identity *identity.Module }

func (u procurementUsers) UsersWithRole(ctx context.Context, role string) ([]procurement.UserRef, error) {
	users, err := u.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	out := make([]procurement.UserRef, 0, len(users))
	for _, user := range users {
		if user.IsActive {
			out = append(out, procurement.UserRef{ID: user.ID, Name: user.FullName, Email: user.Email})
		}
	}
	return out, nil
}

func (u procurementUsers) UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok || id == uuid.Nil {
			continue
		}
		user, err := u.identity.UserByID(ctx, id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = user.FullName
	}
	return out, nil
}
