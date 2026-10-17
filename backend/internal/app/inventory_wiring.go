package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/filestore"
	"balans/internal/inventory"
	"balans/internal/notification"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/sales"
	"balans/internal/warehouse"
)

// maxDisposalPhotoBytes adalah batas ukuran satu foto berita acara pemusnahan. Batas teknis upload.
const maxDisposalPhotoBytes = 10 << 20

// buildInventory menyusun modul inventory beserta dependensi dokumen persediaan (Pemakaian Internal, Repacking,
// Pemusnahan, Disposisi Retur, recost) dan mendaftarkan handler approval-nya (menimpa handler pengembangan).
func (a *App) buildInventory(mailer *notification.Module, reasons *returnReasonAdapter) {
	d := inventory.Deps{
		Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder,
		Products: a.catalog, Locations: a.warehouse, Audit: a.audit, Jobs: a.Jobs, ReturnReasons: reasons,
	}
	if a.accounting != nil {
		d.Periods, d.Poster, d.Dimensions = a.accounting, a.accounting, dimensionAdapter{acc: a.accounting}
	}
	if a.document != nil {
		d.Numberer = a.document
	}
	if a.approval != nil {
		d.Approvals = a.approval
	}
	if a.filestore != nil {
		d.Files = a.filestore
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: inventory.FileOwnerDisposal, WritePermission: inventory.PermDisposalWrite,
			ReadPermission: inventory.PermDisposalRead, ContentTypes: []string{filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP},
			MaxBytes: maxDisposalPhotoBytes,
		})
	}
	if mailer != nil {
		d.Notifier = mailer
	}
	if a.identity != nil {
		d.Members = roleMembersAdapter{a: a}
	}
	a.inventory = inventory.New(d)
	a.modules = append(a.modules, a.inventory)
	registerAuditEntities(a.audit, inventory.AuditEntities())
	if a.approval != nil {
		for _, r := range a.inventory.ApprovalRegistrations() {
			a.approval.RegisterHandler(r.Type, r.ReadPermission, r.Handler)
		}
	}
}

// dimensionAdapter mengisi inventory.DimensionDirectory dari master dimensi accounting.
type dimensionAdapter struct{ acc *accounting.Module }

func (d dimensionAdapter) DimensionOptions(ctx context.Context, kind string, includeInactive bool) ([]inventory.DimensionOption, error) {
	vals, err := d.acc.DimensionValues(ctx, accounting.DimensionKind(kind), includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]inventory.DimensionOption, len(vals))
	for i, v := range vals {
		o := inventory.DimensionOption{ID: v.ID, Code: v.Code, Name: v.Name, IsActive: v.IsActive}
		if kind == inventory.DimensionCampaign {
			from, to := v.ValidFrom, v.ValidTo
			o.ValidFrom, o.ValidTo = &from, &to
		}
		out[i] = o
	}
	return out, nil
}

// returnReasonAdapter mengisi inventory.ReturnReasonDirectory dari master Alasan Retur modul sales. Modul sales
// disusun setelah inventory, sehingga field sales diisi belakangan.
type returnReasonAdapter struct{ sales *sales.Module }

func (r *returnReasonAdapter) ReturnReason(ctx context.Context, id uuid.UUID) (inventory.ReturnReason, error) {
	if r.sales == nil {
		return inventory.ReturnReason{}, apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul sales (Alasan Retur) tidak aktif")
	}
	rr, err := r.sales.ReturnReason(ctx, id)
	if err != nil {
		return inventory.ReturnReason{}, err
	}
	out := inventory.ReturnReason{ID: rr.ID, Code: rr.Code, Name: rr.Name, DefaultDisposition: inventory.Disposition(rr.DefaultDisposition)}
	if msg := rr.UsableError(sales.ScopeSalesReturn); msg != "" && rr.UsableError(sales.ScopePurchaseReturn) != "" {
		out.Usable = errors.New(msg)
	}
	return out, nil
}

// roleMembersAdapter mengisi inventory.RoleMembers dari modul identity.
type roleMembersAdapter struct{ a *App }

func (m roleMembersAdapter) UserIDsWithRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	users, err := m.a.identity.UsersWithRole(ctx, role, nil)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, len(users))
	for i, u := range users {
		out[i] = u.ID
	}
	return out, nil
}

// devStockNamespace membuat ID dokumen seed yang tetap sehingga seed aman diulang (ErrAlreadyPosted).
var devStockNamespace = uuid.MustParse("6f1f2a8e-6d2b-4d55-9c7e-2a1c0b6a9e11")

// seedDevStock mengisi saldo awal stok contoh di Gudang Utama agar halaman Persediaan punya data (APP_ENV=local).
func (a *App) seedDevStock(ctx context.Context) error {
	ctx = actor.System(ctx)
	product := func(sku string) (uuid.UUID, error) {
		p, err := a.catalog.ProductByCode(ctx, sku)
		if err != nil {
			return uuid.Nil, fmt.Errorf("seed stok: produk %s: %w", sku, err)
		}
		return p.ID, nil
	}
	locs, err := a.warehouse.FindLocations(ctx, warehouse.LocationQuery{})
	if err != nil {
		return err
	}
	location := map[string]uuid.UUID{}
	for _, l := range locs {
		location[l.Code] = l.ID
	}
	for _, code := range []string{"UTAMA-FG", "UTAMA-ECM", "UTAMA-RM", "UTAMA-INB"} {
		if _, ok := location[code]; !ok {
			return fmt.Errorf("seed stok: lokasi %s tidak ditemukan", code)
		}
	}
	coklat, err := product("MS-COK-30")
	if err != nil {
		return err
	}
	vanila, err := product("MS-VAN-30")
	if err != nil {
		return err
	}
	tube, err := product("PKG-TUBE-30")
	if err != nil {
		return err
	}
	today := clock.Today(a.Clock)
	batch := func(number string, producedDaysAgo int) *inventory.NewBatch {
		d := today.AddDays(-producedDaysAgo)
		return &inventory.NewBatch{Number: number, ProductionDate: &d}
	}
	cost := func(s string) *decimal.Decimal {
		d := decimal.RequireFromString(s)
		return &d
	}
	at := func(code string, st inventory.StockStatus) *inventory.Place {
		return &inventory.Place{LocationID: location[code], Status: st}
	}
	receive := func(p uuid.UUID, b *inventory.NewBatch, qty int64, unitCost, code string, st inventory.StockStatus) inventory.Line {
		return inventory.Line{Kind: inventory.KindReceive, ProductID: p, Batch: b, Qty: decimal.NewFromInt(qty), UnitCost: cost(unitCost), To: at(code, st)}
	}
	opening := inventory.Posting{
		Source: inventory.Source{
			Type: "opening_balance", ID: uuid.NewSHA1(devStockNamespace, []byte("opening")), Label: "Saldo Awal Stok (demo)",
			BusinessDate: today.AddDays(-30),
		},
		Lines: []inventory.Line{
			receive(coklat, batch("MSC2510", 380), 15, "12000", "UTAMA-FG", inventory.StatusAvailable),
			receive(coklat, batch("MSC2601", 200), 120, "12500", "UTAMA-FG", inventory.StatusAvailable),
			receive(coklat, batch("MSC2604", 60), 200, "13000", "UTAMA-FG", inventory.StatusAvailable),
			receive(vanila, batch("MSV2603", 100), 150, "12800", "UTAMA-FG", inventory.StatusAvailable),
			receive(vanila, batch("MSV2603", 100), 40, "12800", "UTAMA-ECM", inventory.StatusAvailable),
			receive(vanila, batch("MSV2609", 10), 60, "12900", "UTAMA-INB", inventory.StatusQuarantine),
			receive(tube, nil, 1000, "850", "UTAMA-RM", inventory.StatusAvailable),
		},
	}
	sample := inventory.Posting{
		Source: inventory.Source{
			Type: "dev_sample_issue", ID: uuid.NewSHA1(devStockNamespace, []byte("issue")), Label: "Contoh pengeluaran FEFO (demo)",
			BusinessDate: today.AddDays(-5),
		},
		Lines: []inventory.Line{
			{Kind: inventory.KindIssue, ProductID: coklat, Qty: decimal.NewFromInt(10), From: at("UTAMA-FG", inventory.StatusAvailable)},
			{Kind: inventory.KindMove, ProductID: vanila, Qty: decimal.NewFromInt(20), From: at("UTAMA-FG", inventory.StatusAvailable),
				To: at("UTAMA-ECM", inventory.StatusAvailable), Note: "Isi ulang E-commerce Stock"},
		},
	}
	for _, p := range []inventory.Posting{opening, sample} {
		if _, err := a.inventory.Post(ctx, p); err != nil && !errors.Is(err, inventory.ErrAlreadyPosted) {
			return fmt.Errorf("seed stok %s: %w", p.Source.Type, err)
		}
	}
	return nil
}
