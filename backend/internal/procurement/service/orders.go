package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// OrderFilter adalah kriteria daftar PO.
type OrderFilter = postgres.OrderFilter

// OrderSortFields adalah field sort daftar PO.
var OrderSortFields = []string{"number", "businessDate", "expectedDate", "totalAmount"}

// OrderSummary adalah baris daftar PO.
type OrderSummary struct {
	domain.PurchaseOrder
	DivisionName string
}

// RequestRef adalah PR asal baris PO.
type RequestRef struct {
	RequestID     uuid.UUID
	RequestNumber string
}

// OrderView adalah PO beserta data tampilan.
type OrderView struct {
	domain.PurchaseOrder
	DivisionName  string
	CreatedByName string
	Supplier      *SupplierRef
	Products      map[uuid.UUID]ProductRef
	Locations     map[uuid.UUID]LocationRef
	// Requests memetakan baris PR ke PR asal.
	Requests map[uuid.UUID]RequestRef
	Advances []domain.Advance
}

// AdvanceTotals adalah ringkasan uang muka satu PO.
func (v OrderView) AdvanceTotals() (paid, outstanding money.Amount) {
	paid, outstanding = money.Zero, money.Zero
	for _, a := range v.Advances {
		if a.Status == domain.StatusPosted {
			paid = paid.Add(a.Amount)
			outstanding = outstanding.Add(a.Outstanding())
		}
	}
	return paid, outstanding
}

var orderActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermOrderWrite, domain.ActionSubmit: PermOrderSubmit, domain.ActionCancel: PermOrderCancel,
	domain.ActionRevise: PermOrderWrite, domain.ActionClose: PermOrderWrite, domain.ActionPrint: PermOrderRead,
	domain.ActionCreateAdvance: PermAdvanceWrite,
}

// OrderActions mengembalikan aksi PO yang boleh dilakukan actor (allowedActions).
func OrderActions(ctx context.Context, o domain.PurchaseOrder) []string {
	return filterActions(ctx, o.Actions(), orderActionPerms)
}

func orderSnapshot(o domain.PurchaseOrder) map[string]any {
	lines := make([]map[string]any, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "description": l.Description, "qty": l.OrderedQty.String(),
			"unitPrice": l.UnitPrice.String(), "purchaseRequestLineId": l.PurchaseRequestLineID,
		}
	}
	materials := make([]map[string]any, len(o.Materials))
	for i, m := range o.Materials {
		materials[i] = map[string]any{"productId": m.ProductID, "plannedQty": m.PlannedQty.String()}
	}
	return map[string]any{
		"orderType": string(o.Type), "supplierId": o.SupplierID, "supplierName": o.SupplierName,
		"divisionId": o.DivisionID, "businessDate": o.BusinessDate.String(), "expectedDate": o.ExpectedDate.String(),
		"paymentTermDays": o.PaymentTermDays, "note": o.Note, "materialSourceLocationId": o.MaterialSourceLocationID,
		"totalAmount": o.Total.String(), "status": string(o.Status), "closeReason": o.CloseReason,
		"cancelReason": o.CancelReason, "lines": lines, "materials": materials,
	}
}

func orderLabel(o domain.PurchaseOrder) string {
	if o.Number != "" {
		return o.Number
	}
	return "Draft Purchase Order"
}

// orderRefs membaca pemasok, lokasi maklon, dan produk untuk validasi PO.
func (s *Service) orderRefs(ctx context.Context, in domain.OrderInput) (domain.OrderRefs, error) {
	refs := domain.OrderRefs{}
	if in.SupplierID != uuid.Nil {
		sup, ok, err := s.Suppliers.Supplier(ctx, in.SupplierID)
		if err != nil {
			return refs, err
		}
		if ok {
			refs.Supplier = &domain.SupplierInfo{
				ID: sup.ID, Code: sup.Code, Name: sup.Name, IsTollManufacturer: sup.IsTollManufacturer,
				PaymentTermDays: sup.PaymentTermDays,
			}
			if in.Type.IsToll() && sup.IsTollManufacturer {
				loc, err := s.Locations.TollLocation(ctx, sup.ID)
				if err != nil {
					return refs, err
				}
				if loc != nil {
					refs.TollLocationID = &loc.ID
				}
			}
		}
	}
	ids := make([]uuid.UUID, 0, len(in.Lines)+len(in.Materials))
	for _, l := range in.Lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	for _, m := range in.Materials {
		ids = append(ids, m.ProductID)
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return refs, err
	}
	refs.Products = productInfos(products)
	return refs, nil
}

// validateOrder memeriksa isian PO termasuk divisi, lokasi asal bahan, dan baris PR asal.
func (s *Service) validateOrder(ctx context.Context, in domain.OrderInput, orderID uuid.UUID) (domain.OrderRefs, error) {
	refs, err := s.orderRefs(ctx, in)
	if err != nil {
		return refs, err
	}
	if err := in.Validate(refs); err != nil {
		return refs, err
	}
	if err := s.Ledger.CheckDivision(ctx, in.DivisionID, in.BusinessDate, "divisionId"); err != nil {
		return refs, err
	}
	if err := s.checkSourceLocation(ctx, in); err != nil {
		return refs, err
	}
	return refs, s.checkRequestLines(ctx, in, orderID)
}

func (s *Service) checkSourceLocation(ctx context.Context, in domain.OrderInput) error {
	if in.MaterialSourceLocationID == nil {
		return nil
	}
	want, label := LocationRawMaterial, "Gudang GM – RM"
	if in.Type == domain.OrderReprocess {
		want, label = LocationGoodsMaterial, "Gudang GM – GM"
	}
	locs, err := s.Locations.Locations(ctx, []uuid.UUID{*in.MaterialSourceLocationID})
	if err != nil {
		return err
	}
	loc, ok := locs[*in.MaterialSourceLocationID]
	switch {
	case !ok || loc.Deleted:
		return apperr.Validation(map[string]string{"materialSourceLocationId": "Lokasi tidak ditemukan"})
	case !loc.IsActive:
		return apperr.Validation(map[string]string{"materialSourceLocationId": "Lokasi nonaktif"})
	case loc.Type != want:
		return apperr.Validation(map[string]string{"materialSourceLocationId": "Lokasi asal bahan harus lokasi " + label})
	}
	return nil
}

// checkRequestLines memeriksa baris PR asal: PR Disetujui (atau Selesai karena PO ini sendiri), produk sama, dan qty
// tidak melebihi sisa PR di luar PO ini. Baris PR dikunci lewat PR induknya.
func (s *Service) checkRequestLines(ctx context.Context, in domain.OrderInput, orderID uuid.UUID) error {
	lineIDs := []uuid.UUID{}
	for _, l := range in.Lines {
		if l.PurchaseRequestLineID != nil {
			lineIDs = append(lineIDs, *l.PurchaseRequestLineID)
		}
	}
	if len(lineIDs) == 0 {
		return nil
	}
	requestIDs, err := s.Store.RequestIDsByLines(ctx, lineIDs)
	if err != nil {
		return err
	}
	type prLine struct {
		request domain.PurchaseRequest
		line    domain.RequestLine
		used    decimal.Decimal
	}
	byLine := map[uuid.UUID]*prLine{}
	for _, id := range requestIDs {
		r, err := s.Store.GetRequest(ctx, id, true)
		if err != nil {
			return err
		}
		rows, err := s.Store.RequestLineOrders(ctx, id)
		if err != nil {
			return err
		}
		used := map[uuid.UUID]decimal.Decimal{}
		for _, row := range rows {
			if row.OrderID == orderID {
				continue
			}
			l := domain.OrderLine{OrderedQty: row.OrderedQty, ReceivedQty: row.ReceivedQty}
			used[row.RequestLineID] = used[row.RequestLineID].Add(l.FulfilledQty(row.OrderStatus))
		}
		// PR Selesai masih boleh dipakai saat mengubah PO yang ikut menyelesaikannya; sisa qty diperiksa di bawah.
		if r.Status != domain.StatusApproved && (r.Status != domain.StatusCompleted || orderID == uuid.Nil) {
			return r.CanOrder()
		}
		for _, l := range r.Lines {
			byLine[l.ID] = &prLine{request: r, line: l, used: used[l.ID]}
		}
	}
	fields := map[string]string{}
	for i, l := range in.Lines {
		if l.PurchaseRequestLineID == nil {
			continue
		}
		key := fmt.Sprintf("lines.%d.", i)
		pl, ok := byLine[*l.PurchaseRequestLineID]
		switch {
		case !ok:
			fields[key+"purchaseRequestLineId"] = "Baris Purchase Request tidak ditemukan"
		case pl.line.ProductID != nil && (l.ProductID == nil || *l.ProductID != *pl.line.ProductID):
			fields[key+"productId"] = "Produk harus sama dengan baris Purchase Request " + pl.request.Number
		case l.Qty.GreaterThan(pl.line.Qty.Sub(pl.used)):
			fields[key+"qty"] = fmt.Sprintf("Melebihi sisa Purchase Request %s (%s)", pl.request.Number, pl.line.Qty.Sub(pl.used).String())
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func requestLineIDs(o domain.PurchaseOrder) []uuid.UUID {
	ids := []uuid.UUID{}
	for _, l := range o.Lines {
		if l.PurchaseRequestLineID != nil {
			ids = append(ids, *l.PurchaseRequestLineID)
		}
	}
	return ids
}

// recalcOrderRequests menghitung ulang PR yang ditautkan PO (sebelum dan sesudah perubahan baris).
func (s *Service) recalcOrderRequests(ctx context.Context, lineIDs []uuid.UUID) error {
	requestIDs, err := s.Store.RequestIDsByLines(ctx, uniqueIDs(lineIDs))
	if err != nil {
		return err
	}
	return s.recalcRequests(ctx, requestIDs)
}

// ListOrders mengembalikan daftar PO.
func (s *Service) ListOrders(ctx context.Context, f OrderFilter, page paging.Page) (paging.Result[OrderSummary], error) {
	if _, err := actor.Require(ctx, PermOrderRead); err != nil {
		return paging.Result[OrderSummary]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Store.ListOrders(ctx, f, page)
	if err != nil {
		return paging.Result[OrderSummary]{}, err
	}
	ids := make([]uuid.UUID, 0, len(res.Items))
	for _, o := range res.Items {
		ids = append(ids, o.DivisionID)
	}
	divisions, err := s.Ledger.DivisionNames(ctx, uniqueIDs(ids))
	if err != nil {
		return paging.Result[OrderSummary]{}, err
	}
	out := paging.Result[OrderSummary]{Page: res.Page, Total: res.Total, Items: make([]OrderSummary, len(res.Items))}
	for i, o := range res.Items {
		out.Items[i] = OrderSummary{PurchaseOrder: o, DivisionName: divisions[o.DivisionID]}
	}
	return out, nil
}

// GetOrder mengembalikan detail PO beserta uang mukanya.
func (s *Service) GetOrder(ctx context.Context, id uuid.UUID) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderRead); err != nil {
		return OrderView{}, err
	}
	o, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return OrderView{}, err
	}
	return s.orderView(ctx, o)
}

func (s *Service) orderView(ctx context.Context, o domain.PurchaseOrder) (OrderView, error) {
	v := OrderView{PurchaseOrder: o, Requests: map[uuid.UUID]RequestRef{}}
	ids := []uuid.UUID{}
	for _, l := range o.Lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	for _, m := range o.Materials {
		ids = append(ids, m.ProductID)
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return OrderView{}, err
	}
	locIDs := []uuid.UUID{}
	for _, l := range []*uuid.UUID{o.TollLocationID, o.MaterialSourceLocationID} {
		if l != nil {
			locIDs = append(locIDs, *l)
		}
	}
	if v.Locations, err = s.Locations.Locations(ctx, locIDs); err != nil {
		return OrderView{}, err
	}
	divisions, err := s.Ledger.DivisionNames(ctx, []uuid.UUID{o.DivisionID})
	if err != nil {
		return OrderView{}, err
	}
	users, err := s.Users.UserNames(ctx, []uuid.UUID{o.CreatedBy})
	if err != nil {
		return OrderView{}, err
	}
	v.DivisionName, v.CreatedByName = divisions[o.DivisionID], users[o.CreatedBy]
	if sup, ok, err := s.Suppliers.Supplier(ctx, o.SupplierID); err != nil {
		return OrderView{}, err
	} else if ok {
		v.Supplier = &sup
	}
	requestIDs, err := s.Store.RequestIDsByLines(ctx, requestLineIDs(o))
	if err != nil {
		return OrderView{}, err
	}
	for _, rid := range requestIDs {
		r, err := s.Store.GetRequest(ctx, rid, false)
		if err != nil {
			return OrderView{}, err
		}
		for _, l := range r.Lines {
			v.Requests[l.ID] = RequestRef{RequestID: r.ID, RequestNumber: r.Number}
		}
	}
	if o.ID != uuid.Nil {
		if v.Advances, err = s.Store.AdvancesByOrder(ctx, o.ID); err != nil {
			return OrderView{}, err
		}
	}
	return v, nil
}

// CreateOrder menyimpan draft PO. Baris yang berasal dari PR menambah qty PR yang sudah dijadikan PO.
func (s *Service) CreateOrder(ctx context.Context, in domain.OrderInput) (OrderView, error) {
	a, err := actor.Require(ctx, PermOrderWrite)
	if err != nil {
		return OrderView{}, err
	}
	var o domain.PurchaseOrder
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		refs, err := s.validateOrder(ctx, in, uuid.Nil)
		if err != nil {
			return err
		}
		if o, err = domain.NewPurchaseOrder(in, refs, a.UserID); err != nil {
			return err
		}
		if o, err = s.Store.InsertOrder(ctx, o, a.UserID); err != nil {
			return err
		}
		if err := s.recalcOrderRequests(ctx, requestLineIDs(o)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPurchaseOrder, EntityID: o.ID, EntityLabel: orderLabel(o),
			After: orderSnapshot(o),
		})
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.orderView(ctx, o)
}

// mutateOrder mengunci PO, memeriksa versi, menjalankan fn, lalu menyimpan, menghitung ulang PR, dan audit.
func (s *Service) mutateOrder(ctx context.Context, id uuid.UUID, version int, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, o *domain.PurchaseOrder) (string, error),
) (OrderView, error) {
	a, _ := actor.From(ctx)
	var o domain.PurchaseOrder
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if o, err = s.Store.GetOrder(ctx, id, true); err != nil {
			return err
		}
		if o.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before, beforeLines, beforeStatus := orderSnapshot(o), requestLineIDs(o), o.Status
		note, err := fn(ctx, &o)
		if err != nil {
			return err
		}
		if err := s.Store.SaveOrder(ctx, o, version, a.UserID, replaceLines); err != nil {
			return err
		}
		o.Version = version + 1
		if replaceLines || o.Status != beforeStatus {
			if err := s.recalcOrderRequests(ctx, append(beforeLines, requestLineIDs(o)...)); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPurchaseOrder, EntityID: o.ID, EntityLabel: orderLabel(o),
			Before: before, After: orderSnapshot(o), Note: note,
		})
	})
	if err != nil {
		return OrderView{}, err
	}
	if o, err = s.Store.GetOrder(ctx, id, false); err != nil {
		return OrderView{}, err
	}
	return s.orderView(ctx, o)
}

// UpdateOrder mengubah draft PO.
func (s *Service) UpdateOrder(ctx context.Context, id uuid.UUID, version int, in domain.OrderInput) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderWrite); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, version, audit.ActionUpdate, true, func(ctx context.Context, o *domain.PurchaseOrder) (string, error) {
		if o.Status != domain.StatusDraft {
			return "", o.Update(in, domain.OrderRefs{})
		}
		refs, err := s.validateOrder(ctx, in, o.ID)
		if err != nil {
			return "", err
		}
		return "", o.Update(in, refs)
	})
}

func orderInputOf(o domain.PurchaseOrder) domain.OrderInput {
	term := o.PaymentTermDays
	in := domain.OrderInput{
		Type: o.Type, SupplierID: o.SupplierID, DivisionID: o.DivisionID, BusinessDate: o.BusinessDate,
		ExpectedDate: o.ExpectedDate, PaymentTermDays: &term, Note: o.Note, MaterialSourceLocationID: o.MaterialSourceLocationID,
	}
	for _, l := range o.Lines {
		in.Lines = append(in.Lines, domain.OrderLineInput{
			ProductID: l.ProductID, Description: l.Description, Qty: l.OrderedQty, UnitPrice: l.UnitPrice,
			PurchaseRequestLineID: l.PurchaseRequestLineID, Note: l.Note,
		})
	}
	for _, m := range o.Materials {
		in.Materials = append(in.Materials, domain.OrderMaterialInput{ProductID: m.ProductID, PlannedQty: m.PlannedQty, Note: m.Note})
	}
	return in
}

// SubmitOrder mengajukan PO: nomor PO diberikan dan approval berjenjang sesuai total diminta di transaksi yang sama.
func (s *Service) SubmitOrder(ctx context.Context, id uuid.UUID, version int) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderSubmit); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, version, audit.ActionSubmit, false, func(ctx context.Context, o *domain.PurchaseOrder) (string, error) {
		if o.Status == domain.StatusDraft {
			// Pemasok, produk, lokasi, dan divisi diperiksa ulang: master bisa berubah sejak draft disimpan.
			if _, err := s.validateOrder(ctx, orderInputOf(*o), o.ID); err != nil {
				return "", err
			}
		}
		number := ""
		if o.Number == "" {
			var err error
			if number, err = s.Numberer.Next(ctx, DocPurchaseOrder, o.BusinessDate); err != nil {
				return "", err
			}
		}
		if err := o.Submit(number, s.Clock.Now()); err != nil {
			return "", err
		}
		division := o.DivisionID
		_, err := s.Approvals.Request(ctx, approval.Subject{
			Type: approval.SubjectPurchaseOrder, ID: o.ID, Label: o.Number, Link: OrderPath(o.ID),
			Summary: orderSummary(*o), Amount: o.Total, Division: &division,
		})
		return "", err
	})
}

func orderSummary(o domain.PurchaseOrder) string {
	return fmt.Sprintf("%s · %s · %d baris", o.Type.Label(), o.SupplierName, len(o.Lines))
}

// ReviseOrder mengembalikan PO yang ditolak ke Draft.
func (s *Service) ReviseOrder(ctx context.Context, id uuid.UUID, version int) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderWrite); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, version, audit.ActionStatusChange, false, func(_ context.Context, o *domain.PurchaseOrder) (string, error) {
		return "Direvisi", o.Revise()
	})
}

// CancelOrder membatalkan PO Draft atau Disetujui (belum ada penerimaan dan uang muka aktif).
func (s *Service) CancelOrder(ctx context.Context, id uuid.UUID, version int, reason string) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderCancel); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, version, audit.ActionCancel, false, func(ctx context.Context, o *domain.PurchaseOrder) (string, error) {
		advances, err := s.Store.AdvancesByOrder(ctx, o.ID)
		if err != nil {
			return "", err
		}
		for _, a := range advances {
			if a.Counts() {
				return "", apperr.WithMessage(apperr.CodeInvalidStateTransition,
					"PO masih punya uang muka yang belum dibatalkan. Batalkan uang muka lebih dulu.")
			}
		}
		return strings.TrimSpace(reason), o.Cancel(reason)
	})
}

// CloseOrder menutup PO yang diterima sebagian; sisa qty tidak ditunggu lagi dan PR terkait dihitung ulang.
func (s *Service) CloseOrder(ctx context.Context, id uuid.UUID, version int, reason string) (OrderView, error) {
	if _, err := actor.Require(ctx, PermOrderWrite); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, version, audit.ActionStatusChange, false, func(_ context.Context, o *domain.PurchaseOrder) (string, error) {
		return "Ditutup: " + strings.TrimSpace(reason), o.Close(reason)
	})
}

func (s *Service) decideOrder(ctx context.Context, id uuid.UUID, approve bool) error {
	o, err := s.Store.GetOrder(ctx, id, true)
	if err != nil {
		return err
	}
	version := o.Version
	if approve {
		err = o.Approve(s.Clock.Now())
	} else {
		err = o.Reject()
	}
	if err != nil {
		return err
	}
	if err := s.Store.SaveOrder(ctx, o, version, actor.UserID(ctx), false); err != nil {
		return err
	}
	if approve {
		return s.notifyLateETA(ctx, o)
	}
	return nil
}

// notifyLateETA mengirim notifikasi ETA langsung bila PO disetujui setelah job harian H-1 lewat, yaitu tanggal
// kirim diharapkan hari ini atau besok.
func (s *Service) notifyLateETA(ctx context.Context, o domain.PurchaseOrder) error {
	if o.ExpectedDate.After(s.today().AddDays(1)) {
		return nil
	}
	leaders, err := s.Users.UsersWithRole(ctx, RoleLogisticsLeader)
	if err != nil || len(leaders) == 0 {
		return err
	}
	ok, err := s.Store.MarkETANotified(ctx, o.ID, s.Clock.Now())
	if err != nil || !ok {
		return err
	}
	v, err := s.orderView(ctx, o)
	if err != nil {
		return err
	}
	return s.sendETA(ctx, v, leaders)
}

// OnOrderApproved menandai PO Disetujui setelah level terakhir menyetujui; PO bisa diterima dan dibayar uang muka.
func (s *Service) OnOrderApproved(ctx context.Context, d approval.Decided) error {
	return s.decideOrder(ctx, d.Subject.ID, true)
}

// OnOrderRejected menandai PO Ditolak.
func (s *Service) OnOrderRejected(ctx context.Context, d approval.Decided) error {
	return s.decideOrder(ctx, d.Subject.ID, false)
}
