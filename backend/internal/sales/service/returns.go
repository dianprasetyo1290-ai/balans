package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Permission Retur Penjualan dan Nota Kredit (Desain Role & Approval 3.3 dan 3.5).
const (
	PermReturnRead     = "sales.sales_return.read"
	PermReturnWrite    = "sales.sales_return.write"
	PermReturnSubmit   = "sales.sales_return.submit"
	PermReturnVerify   = "sales.sales_return.verify"
	PermCreditNoteRead = "sales.credit_note.read"
	PermCreditNotePost = "sales.credit_note.post"
)

// Entitas audit, posting rule, sumber dokumen, nomor dokumen, dan file Retur Penjualan & Nota Kredit.
const (
	EntitySalesReturn = "sales_return"
	EntityCreditNote  = "credit_note"

	// RuleSalesReturn: Dr Retur Penjualan 4103 (nilai jual bersih) & Persediaan Retur 1314 (HPP asal),
	// Cr Piutang Usaha 1201 / Piutang Marketplace 121x & HPP 5101.
	RuleSalesReturn = "A-13"

	// SourceSalesReturn adalah source_type stok, Inbound, dan jurnal A-13 retur penjualan.
	SourceSalesReturn = "sales_return"
	// SourceSalesRefund adalah source_type jurnal A-PB pembayaran refund nota kredit.
	SourceSalesRefund = "sales_refund"

	DocSalesReturn DocumentType = "RJ"
	DocCreditNote  DocumentType = "CN"

	// FileOwnerSalesReturnPhoto adalah jenis pemilik file foto pickup, foto barang, dan form retur.
	FileOwnerSalesReturnPhoto = "sales.sales_return_photo"

	// RoleFulfillmentLeader menugaskan Merchandiser untuk pickup retur MT (Desain Role & Approval 3.5).
	RoleFulfillmentLeader = "fulfillment_leader"
	// RoleMerchandiser mengambil barang retur di lokasi partner.
	RoleMerchandiser = "merchandiser"
	// RoleLogisticsLeader memverifikasi retur MT.
	RoleLogisticsLeader = "logistics_leader"

	returnPathPrefix     = "/sales/returns/"
	creditNotePathPrefix = "/sales/credit-notes/"
	notificationKind     = "sales_return"
)

// ReturnPath adalah path detail Retur Penjualan di ERP.
func ReturnPath(id uuid.UUID) string { return returnPathPrefix + id.String() }

// CreditNotePath adalah path detail Nota Kredit di ERP.
func CreditNotePath(id uuid.UUID) string { return creditNotePathPrefix + id.String() }

// ReturnInboundLine adalah satu barang retur yang masuk Inbound (ItemID menjadi source_line_id baris Inbound).
type ReturnInboundLine struct {
	ItemID    uuid.UUID
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
}

// ReturnInboundRequest adalah Inbound barang retur: stok sudah diterima ke lokasi Inbound berstatus Karantina;
// lulus QC → lokasi Gudang Retur (ReturnsLocationID) untuk disposisi.
type ReturnInboundRequest struct {
	ReturnID          uuid.UUID
	ReturnNumber      string
	InboundLocationID uuid.UUID
	ReturnsLocationID uuid.UUID
	BusinessDate      clock.BusinessDate
	Note              string
	Lines             []ReturnInboundLine
}

// InboundRef adalah dokumen Inbound yang dibuat modul warehouse.
type InboundRef struct {
	ID     uuid.UUID
	Number string
}

// ReturnInbounds membuat Inbound barang retur (modul warehouse) di transaksi aktif.
type ReturnInbounds interface {
	CreateInbound(ctx context.Context, r ReturnInboundRequest) (InboundRef, error)
}

// ReturnLocations menentukan lokasi Inbound dan Gudang Retur gudang asal stok pesanan.
type ReturnLocations interface {
	ReturnLocations(ctx context.Context, sourceLocationID uuid.UUID) (inbound, returns uuid.UUID, err error)
}

// StaffRef adalah pengguna aktif satu role.
type StaffRef struct {
	ID   uuid.UUID
	Name string
}

// ReturnStaff mencari pengguna aktif per role (Merchandiser untuk pickup, penerima notifikasi).
type ReturnStaff interface {
	UsersWithRole(ctx context.Context, role string) ([]StaffRef, error)
}

// Notifier mengirim notifikasi in-app.
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// ReturnDeps adalah dependensi tambahan Retur Penjualan; Notifier opsional.
type ReturnDeps struct {
	Inbounds  ReturnInbounds
	Locations ReturnLocations
	Staff     ReturnStaff
	Notifier  Notifier
}

// ReturnService adalah use case Retur Penjualan (S-314, S-317) dan Nota Kredit.
type ReturnService struct {
	*DocumentService
	rd ReturnDeps
}

// NewReturnService membuat ReturnService; dependensi tambahan dipasang lewat SetDeps saat wiring.
func NewReturnService(docs *DocumentService) *ReturnService {
	return &ReturnService{DocumentService: docs}
}

// SetDeps memasang dependensi Inbound, lokasi, staf, dan notifikasi.
func (s *ReturnService) SetDeps(d ReturnDeps) { s.rd = d }

func (s *ReturnService) ready() error {
	if s.rd.Inbounds == nil || s.rd.Locations == nil || s.rd.Staff == nil {
		return apperr.WithMessage(apperr.CodeProviderUnavailable, "Retur penjualan membutuhkan modul warehouse dan identity.")
	}
	return nil
}

// --- rujukan ---------------------------------------------------------------------------------

// ReturnSourceView adalah Sales Order sumber retur beserta baris yang bisa diretur.
type ReturnSourceView struct {
	Order      domain.Order
	Invoice    *domain.Invoice
	Lines      []domain.ReturnableLine
	Products   map[uuid.UUID]ProductRef
	BatchNames map[uuid.UUID]string
}

func (s *ReturnService) returnRefs(ctx context.Context, orderID, excludeID uuid.UUID, reasonIDs []uuid.UUID) (domain.ReturnRefs, error) {
	o, err := s.Store.GetOrder(ctx, orderID, false)
	if err != nil {
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return domain.ReturnRefs{}, apperr.Validation(map[string]string{"salesOrderId": "Sales Order tidak ditemukan"})
		}
		return domain.ReturnRefs{}, err
	}
	refs := domain.ReturnRefs{Order: o, Today: s.today(), Reasons: map[uuid.UUID]domain.ReturnReason{}}
	invs, err := s.Store.InvoicesByOrder(ctx, o.ID)
	if err != nil {
		return domain.ReturnRefs{}, err
	}
	for i := range invs {
		if invs[i].Status != domain.StatusCancelled {
			inv := invs[i]
			refs.Invoice = &inv
			break
		}
	}
	shipments, err := s.Store.ShipmentsByOrder(ctx, o.ID)
	if err != nil {
		return domain.ReturnRefs{}, err
	}
	returned, err := s.Store.ReturnedQty(ctx, o.ID, excludeID)
	if err != nil {
		return domain.ReturnRefs{}, err
	}
	refs.Returnable = domain.ReturnableLines(o, domain.ShipmentCosts(shipments), returned)
	for _, id := range uniqueIDs(reasonIDs) {
		r, err := s.Store.GetReturnReason(ctx, id, false)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return domain.ReturnRefs{}, err
		}
		refs.Reasons[id] = r
	}
	return refs, nil
}

// ReturnSources mencari Sales Order yang barangnya sudah dikirim untuk diretur.
func (s *ReturnService) ReturnSources(ctx context.Context, search string) ([]postgres.ReturnSource, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return nil, err
	}
	return s.Store.ReturnSources(ctx, strings.TrimSpace(search))
}

// ReturnSource mengembalikan baris Sales Order yang bisa diretur (qty terkirim − qty retur lain).
func (s *ReturnService) ReturnSource(ctx context.Context, orderID uuid.UUID) (ReturnSourceView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnSourceView{}, err
	}
	refs, err := s.returnRefs(ctx, orderID, uuid.Nil, nil)
	if err != nil {
		return ReturnSourceView{}, err
	}
	if err := refs.Order.CanReturn(); err != nil {
		return ReturnSourceView{}, err
	}
	v := ReturnSourceView{Order: refs.Order, Invoice: refs.Invoice, Lines: refs.Returnable, BatchNames: map[uuid.UUID]string{}}
	ids := []uuid.UUID{}
	for _, l := range refs.Returnable {
		ids = append(ids, l.ProductID)
		for _, b := range l.Batches {
			if _, ok := v.BatchNames[b]; !ok {
				v.BatchNames[b] = s.batchName(ctx, b)
			}
		}
	}
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return ReturnSourceView{}, err
	}
	return v, nil
}

func (s *ReturnService) batchName(ctx context.Context, id uuid.UUID) string {
	b, err := s.Stock.Batch(ctx, id)
	if err != nil {
		return ""
	}
	return b.Number
}

// --- tampilan --------------------------------------------------------------------------------

// ReturnView adalah detail retur beserta nama rujukan dan aksi.
type ReturnView struct {
	domain.SalesReturn
	Products   map[uuid.UUID]ProductRef
	Reasons    map[uuid.UUID]domain.ReturnReason
	BatchNames map[uuid.UUID]string
	UserNames  map[uuid.UUID]string
	FileNames  map[uuid.UUID]string
	CreditNote *domain.CreditNote
	Actions    []string
}

var returnActionPerms = map[domain.Action][]string{
	domain.ActionEdit: {PermReturnWrite}, domain.ActionSubmit: {PermReturnSubmit}, domain.ActionCancel: {PermReturnWrite},
	domain.ActionVerify: {PermReturnVerify}, domain.ActionSendBack: {PermReturnVerify},
	domain.ActionAssignPickup: {PermReturnWrite}, domain.ActionPickup: {PermReturnWrite},
	domain.ActionReceive: {PermReturnWrite},
}

// ReturnActions mengembalikan aksi retur yang boleh dilakukan actor. Penugasan pickup hanya Fulfillment Leader.
func (s *ReturnService) ReturnActions(ctx context.Context, r domain.SalesReturn) []string {
	a, _ := actor.From(ctx)
	acts := filterActions(ctx, r.Actions(domain.ReturnContext{ActorID: a.UserID}), returnActionPerms)
	out := acts[:0]
	for _, act := range acts {
		if act == string(domain.ActionAssignPickup) && !a.HasRole(RoleFulfillmentLeader) {
			continue
		}
		out = append(out, act)
	}
	return out
}

// ListReturns mengembalikan daftar Retur Penjualan.
func (s *ReturnService) ListReturns(ctx context.Context, f postgres.ReturnFilter, page paging.Page) (paging.Result[domain.SalesReturn], error) {
	if _, err := actor.Require(ctx, PermReturnRead); err != nil {
		return paging.Result[domain.SalesReturn]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListReturns(ctx, f, page)
}

// GetReturn mengembalikan detail Retur Penjualan.
func (s *ReturnService) GetReturn(ctx context.Context, id uuid.UUID) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnRead); err != nil {
		return ReturnView{}, err
	}
	r, err := s.Store.GetReturn(ctx, id, false)
	if err != nil {
		return ReturnView{}, err
	}
	return s.returnView(ctx, r)
}

func (s *ReturnService) returnView(ctx context.Context, r domain.SalesReturn) (ReturnView, error) {
	v := ReturnView{SalesReturn: r, Reasons: map[uuid.UUID]domain.ReturnReason{}, BatchNames: map[uuid.UUID]string{}}
	products, users, files := []uuid.UUID{}, []uuid.UUID{r.CreatedBy}, []uuid.UUID{}
	for _, l := range r.Lines {
		products = append(products, l.ProductID)
		if _, ok := v.Reasons[l.ReasonID]; !ok {
			if rr, err := s.Store.GetReturnReason(ctx, l.ReasonID, false); err == nil {
				v.Reasons[l.ReasonID] = rr
			}
		}
	}
	for _, it := range r.Items {
		products = append(products, it.ProductID)
		if it.BatchID != nil {
			if _, ok := v.BatchNames[*it.BatchID]; !ok {
				v.BatchNames[*it.BatchID] = s.batchName(ctx, *it.BatchID)
			}
		}
	}
	for _, l := range r.Lines {
		if l.BatchID != nil {
			if _, ok := v.BatchNames[*l.BatchID]; !ok {
				v.BatchNames[*l.BatchID] = s.batchName(ctx, *l.BatchID)
			}
		}
	}
	for _, id := range []*uuid.UUID{r.VerifiedBy, r.PickupUserID, r.ReceivedBy} {
		if id != nil {
			users = append(users, *id)
		}
	}
	files = append(append(files, r.PickupFileIDs...), r.DocumentationFileIDs...)
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(products)); err != nil {
		return ReturnView{}, err
	}
	if v.UserNames, err = s.Users.UserNames(ctx, uniqueIDs(users)); err != nil {
		return ReturnView{}, err
	}
	if v.FileNames, err = s.Files.Names(ctx, uniqueIDs(files)); err != nil {
		return ReturnView{}, err
	}
	if id, ok, err := s.Store.CreditNoteIDByReturn(ctx, r.ID); err != nil {
		return ReturnView{}, err
	} else if ok {
		cn, err := s.Store.GetCreditNote(ctx, id, false)
		if err != nil {
			return ReturnView{}, err
		}
		v.CreditNote = &cn
	}
	v.Actions = s.ReturnActions(ctx, r)
	return v, nil
}

func returnSnapshot(r domain.SalesReturn) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{"qty": l.Qty.String(), "amount": l.Amount.String(), "cost": l.Cost.String()}
	}
	return map[string]any{
		"status": string(r.Status), "salesOrder": r.OrderNumber, "settlementMethod": string(r.Settlement),
		"returnDate": r.ReturnDate.String(), "returnAmount": r.Amount.String(), "costAmount": r.Cost.String(),
		"pickupRequired": r.PickupRequired, "inbound": r.InboundNumber, "lines": lines,
	}
}

func (s *ReturnService) record(ctx context.Context, action audit.Action, before map[string]any, r domain.SalesReturn, note string) error {
	return s.Audit.Record(ctx, audit.Entry{
		Action: action, EntityType: EntitySalesReturn, EntityID: r.ID, EntityLabel: r.Label(),
		Before: before, After: returnSnapshot(r), Note: note,
	})
}

func (s *ReturnService) notifyRole(ctx context.Context, role, title, body string, id uuid.UUID) {
	if s.rd.Notifier == nil || s.rd.Staff == nil {
		return
	}
	users, err := s.rd.Staff.UsersWithRole(ctx, role)
	if err != nil || len(users) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	s.notify(ctx, ids, title, body, id)
}

func (s *ReturnService) notify(ctx context.Context, ids []uuid.UUID, title, body string, id uuid.UUID) {
	if s.rd.Notifier == nil || len(ids) == 0 {
		return
	}
	if err := s.rd.Notifier.Notify(ctx, ids, notificationKind, title, body, ReturnPath(id)); err != nil && s.Log != nil {
		s.Log.WarnContext(ctx, "notifikasi retur gagal", "error", err)
	}
}

// --- draft -----------------------------------------------------------------------------------

func reasonIDs(in domain.ReturnInput) []uuid.UUID {
	out := make([]uuid.UUID, len(in.Lines))
	for i, l := range in.Lines {
		out[i] = l.ReasonID
	}
	return out
}

// CreateReturn menyimpan draft Retur Penjualan dari baris Sales Order yang sudah dikirim.
func (s *ReturnService) CreateReturn(ctx context.Context, in domain.ReturnInput) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnWrite)
	if err != nil {
		return ReturnView{}, err
	}
	var id uuid.UUID
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		refs, err := s.returnRefs(ctx, in.OrderID, uuid.Nil, reasonIDs(in))
		if err != nil {
			return err
		}
		r, err := domain.NewSalesReturn(in, refs, a.UserID)
		if err != nil {
			return err
		}
		if id, err = s.Store.InsertReturn(ctx, r, a.UserID); err != nil {
			return err
		}
		r.ID = id
		return s.record(ctx, audit.ActionCreate, nil, r, "")
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.GetReturn(ctx, id)
}

// UpdateReturn mengubah draft retur.
func (s *ReturnService) UpdateReturn(ctx context.Context, id uuid.UUID, ver int, in domain.ReturnInput) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	err := s.change(ctx, id, ver, audit.ActionUpdate, true, func(ctx context.Context, r *domain.SalesReturn) (string, error) {
		in.OrderID = r.OrderID
		refs, err := s.returnRefs(ctx, r.OrderID, r.ID, reasonIDs(in))
		if err != nil {
			return "", err
		}
		return "", r.Update(in, refs)
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.GetReturn(ctx, id)
}

// change mengunci retur, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit di satu transaksi.
func (s *ReturnService) change(ctx context.Context, id uuid.UUID, ver int, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, r *domain.SalesReturn) (string, error)) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		r, err := s.Store.GetReturn(ctx, id, true)
		if err != nil {
			return err
		}
		if ver != 0 && r.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		return s.save(ctx, &r, action, replaceLines, fn)
	})
}

func (s *ReturnService) save(ctx context.Context, r *domain.SalesReturn, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, r *domain.SalesReturn) (string, error)) error {
	before := returnSnapshot(*r)
	ver := r.Version
	note, err := fn(ctx, r)
	if err != nil {
		return err
	}
	if err := s.Store.SaveReturn(ctx, *r, ver, actor.UserID(ctx), replaceLines); err != nil {
		return err
	}
	r.Version = ver + 1
	return s.record(ctx, action, before, *r, note)
}

// --- alur status -----------------------------------------------------------------------------

// SubmitReturn mengajukan retur (nomor RJ). Retur pelanggan langsung/marketplace langsung masuk Inbound
// (Karantina); retur MT/reseller menunggu verifikasi Logistik Leader.
func (s *ReturnService) SubmitReturn(ctx context.Context, id uuid.UUID, ver int) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnSubmit)
	if err != nil {
		return ReturnView{}, err
	}
	if err := s.ready(); err != nil {
		return ReturnView{}, err
	}
	var notifyLL bool
	var label string
	err = s.change(ctx, id, ver, audit.ActionSubmit, false, func(ctx context.Context, r *domain.SalesReturn) (string, error) {
		// Qty terkirim bisa berubah sejak draft (retur lain); validasi ulang.
		in := draftInput(*r)
		refs, err := s.returnRefs(ctx, r.OrderID, r.ID, reasonIDs(in))
		if err != nil {
			return "", err
		}
		check := *r
		if err := check.Update(in, refs); err != nil {
			return "", err
		}
		number := r.Number
		if number == "" {
			if number, err = s.Numberer.Next(ctx, DocSalesReturn, r.ReturnDate); err != nil {
				return "", err
			}
		}
		if err := r.Submit(number, a.UserID, s.Clock.Now()); err != nil {
			return "", err
		}
		label = r.Number
		if r.Status == domain.StatusSubmitted {
			notifyLL = true
			return "Diajukan untuk verifikasi Logistik Leader", nil
		}
		return "Barang masuk Inbound", s.startInbound(ctx, r)
	})
	if err != nil {
		return ReturnView{}, err
	}
	if notifyLL {
		s.notifyRole(ctx, RoleLogisticsLeader, "Retur MT menunggu verifikasi", "Retur Penjualan "+label+" perlu diverifikasi.", id)
	}
	return s.GetReturn(ctx, id)
}

func draftInput(r domain.SalesReturn) domain.ReturnInput {
	in := domain.ReturnInput{OrderID: r.OrderID, ReturnDate: r.ReturnDate, Settlement: r.Settlement, Note: r.Note}
	for _, l := range r.Lines {
		in.Lines = append(in.Lines, domain.ReturnLineInput{OrderLineID: l.OrderLineID, BatchID: l.BatchID, Qty: l.Qty, ReasonID: l.ReasonID, Note: l.Note})
	}
	return in
}

// startInbound menerima barang retur ke lokasi Inbound berstatus Karantina dengan HPP asal lalu membuat Inbound
// (QC Staf Gudang; lulus → Gudang Retur).
func (s *ReturnService) startInbound(ctx context.Context, r *domain.SalesReturn) error {
	inboundLoc, returnsLoc, err := s.rd.Locations.ReturnLocations(ctx, r.SourceLocationID)
	if err != nil {
		return err
	}
	date := s.today()
	lines := make([]inventory.Line, 0, len(r.Items))
	inLines := make([]ReturnInboundLine, 0, len(r.Items))
	for _, it := range r.Items {
		itemID, cost := it.ID, it.UnitCost
		lines = append(lines, inventory.Line{
			Kind: inventory.KindReceive, SourceLineID: &itemID, ProductID: it.ProductID, BatchID: it.BatchID, Qty: it.Qty,
			UnitCost: &cost, To: &inventory.Place{LocationID: inboundLoc, Status: inventory.StatusQuarantine},
			Note: "Retur penjualan " + r.Number,
		})
		inLines = append(inLines, ReturnInboundLine{ItemID: it.ID, ProductID: it.ProductID, BatchID: it.BatchID, Qty: it.Qty})
	}
	if len(lines) == 0 {
		return apperr.Validation(map[string]string{"lines": "Tidak ada barang yang diterima"})
	}
	if _, err := s.Stock.Post(ctx, inventory.Posting{
		Source: inventory.Source{Type: SourceSalesReturn, ID: r.ID, Action: "receive", Label: r.Number, BusinessDate: date},
		Lines:  lines,
	}); err != nil {
		return err
	}
	ib, err := s.rd.Inbounds.CreateInbound(ctx, ReturnInboundRequest{
		ReturnID: r.ID, ReturnNumber: r.Number, InboundLocationID: inboundLoc, ReturnsLocationID: returnsLoc,
		BusinessDate: date, Note: r.PartyName, Lines: inLines,
	})
	if err != nil {
		return err
	}
	r.InboundID, r.InboundNumber = &ib.ID, ib.Number
	return nil
}

// SendBackReturn mengembalikan retur Diajukan ke Draft dengan catatan (tidak sesuai SOP retur).
func (s *ReturnService) SendBackReturn(ctx context.Context, id uuid.UUID, ver int, note string) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnVerify); err != nil {
		return ReturnView{}, err
	}
	var creator uuid.UUID
	var label string
	err := s.change(ctx, id, ver, audit.ActionStatusChange, false, func(_ context.Context, r *domain.SalesReturn) (string, error) {
		creator, label = r.CreatedBy, r.Label()
		if err := r.SendBack(note); err != nil {
			return "", err
		}
		return "Dikembalikan: " + r.SendBackNote, nil
	})
	if err != nil {
		return ReturnView{}, err
	}
	s.notify(ctx, []uuid.UUID{creator}, "Retur dikembalikan", "Retur Penjualan "+label+" dikembalikan Logistik Leader: "+strings.TrimSpace(note), id)
	return s.GetReturn(ctx, id)
}

// VerifyReturn mencatat verifikasi Logistik Leader (crosscheck SOP) dan perlu/tidaknya pickup Merchandiser.
func (s *ReturnService) VerifyReturn(ctx context.Context, id uuid.UUID, ver int, pickupRequired bool, note string) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnVerify)
	if err != nil {
		return ReturnView{}, err
	}
	var label string
	err = s.change(ctx, id, ver, audit.ActionStatusChange, false, func(_ context.Context, r *domain.SalesReturn) (string, error) {
		label = r.Label()
		if err := r.Verify(a.UserID, pickupRequired, note, s.Clock.Now()); err != nil {
			return "", err
		}
		if pickupRequired {
			return "Terverifikasi; perlu pickup Merchandiser", nil
		}
		return "Terverifikasi; barang dikirim partner", nil
	})
	if err != nil {
		return ReturnView{}, err
	}
	if pickupRequired {
		s.notifyRole(ctx, RoleFulfillmentLeader, "Tugaskan pickup retur", "Retur Penjualan "+label+" perlu diambil Merchandiser.", id)
	}
	return s.GetReturn(ctx, id)
}

// PickupUsers mengembalikan Merchandiser aktif untuk penugasan pickup.
func (s *ReturnService) PickupUsers(ctx context.Context) ([]StaffRef, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.rd.Staff.UsersWithRole(ctx, RoleMerchandiser)
}

// AssignPickup menugaskan Merchandiser untuk pickup (Fulfillment Leader).
func (s *ReturnService) AssignPickup(ctx context.Context, id uuid.UUID, ver int, userID uuid.UUID) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnWrite)
	if err != nil {
		return ReturnView{}, err
	}
	if !a.HasRole(RoleFulfillmentLeader) {
		return ReturnView{}, apperr.New(apperr.CodeForbidden)
	}
	if err := s.ready(); err != nil {
		return ReturnView{}, err
	}
	staff, err := s.rd.Staff.UsersWithRole(ctx, RoleMerchandiser)
	if err != nil {
		return ReturnView{}, err
	}
	name := ""
	for _, u := range staff {
		if u.ID == userID {
			name = u.Name
		}
	}
	if name == "" {
		return ReturnView{}, apperr.Validation(map[string]string{"pickupUserId": "Pilih Merchandiser aktif"})
	}
	var label string
	err = s.change(ctx, id, ver, audit.ActionUpdate, false, func(_ context.Context, r *domain.SalesReturn) (string, error) {
		label = r.Label()
		return "Pickup ditugaskan ke " + name, r.AssignPickup(userID, s.Clock.Now())
	})
	if err != nil {
		return ReturnView{}, err
	}
	s.notify(ctx, []uuid.UUID{userID}, "Tugas pickup retur", "Ambil barang Retur Penjualan "+label+" di lokasi partner.", id)
	return s.GetReturn(ctx, id)
}

func (s *ReturnService) attachFiles(ctx context.Context, ids []uuid.UUID, ownerID uuid.UUID) error {
	for _, f := range ids {
		if err := s.Files.Attach(ctx, f, FileOwnerSalesReturnPhoto, ownerID); err != nil {
			return err
		}
	}
	return nil
}

// RecordPickup mencatat Merchandiser yang ditugaskan sudah mengecek & mengambil barang (foto wajib).
func (s *ReturnService) RecordPickup(ctx context.Context, id uuid.UUID, ver int, note string, files []uuid.UUID) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnWrite)
	if err != nil {
		return ReturnView{}, err
	}
	err = s.change(ctx, id, ver, audit.ActionUpdate, false, func(ctx context.Context, r *domain.SalesReturn) (string, error) {
		if err := r.RecordPickup(a.UserID, note, files, s.Clock.Now()); err != nil {
			return "", err
		}
		return "Barang diambil Merchandiser", s.attachFiles(ctx, r.PickupFileIDs, r.ID)
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.GetReturn(ctx, id)
}

// ReceiveReturn mencatat barang retur MT tiba: cek ulang qty, foto & form retur; barang masuk Inbound.
func (s *ReturnService) ReceiveReturn(ctx context.Context, id uuid.UUID, ver int, note string, files []uuid.UUID, qtys []domain.ReceivedQty) (ReturnView, error) {
	a, err := actor.Require(ctx, PermReturnWrite)
	if err != nil {
		return ReturnView{}, err
	}
	if err := s.ready(); err != nil {
		return ReturnView{}, err
	}
	err = s.change(ctx, id, ver, audit.ActionStatusChange, false, func(ctx context.Context, r *domain.SalesReturn) (string, error) {
		if err := r.Receive(a.UserID, note, files, qtys, s.Clock.Now()); err != nil {
			return "", err
		}
		if err := s.attachFiles(ctx, r.DocumentationFileIDs, r.ID); err != nil {
			return "", err
		}
		return "Barang diterima dan masuk Inbound", s.startInbound(ctx, r)
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.GetReturn(ctx, id)
}

// CancelReturn membatalkan retur sebelum barang masuk Inbound.
func (s *ReturnService) CancelReturn(ctx context.Context, id uuid.UUID, ver int, reason string) (ReturnView, error) {
	if _, err := actor.Require(ctx, PermReturnWrite); err != nil {
		return ReturnView{}, err
	}
	err := s.change(ctx, id, ver, audit.ActionCancel, false, func(_ context.Context, r *domain.SalesReturn) (string, error) {
		return strings.TrimSpace(reason), r.Cancel(reason)
	})
	if err != nil {
		return ReturnView{}, err
	}
	return s.GetReturn(ctx, id)
}

// --- QC Inbound & posting ----------------------------------------------------------------------

// ReturnQCOutcome adalah hasil QC satu baris Inbound barang retur.
type ReturnQCOutcome struct {
	ReturnID    uuid.UUID
	ItemID      uuid.UUID
	PassedQty   decimal.Decimal
	RejectedQty decimal.Decimal
}

// OnInboundQC mencatat hasil QC item retur di transaksi QC; QC semua item selesai → retur Diposting: jurnal A-13
// dan nota kredit (atau refund menunggu persetujuan Head F&A). Tanpa cek permission (dipanggil modul warehouse).
func (s *ReturnService) OnInboundQC(ctx context.Context, o ReturnQCOutcome) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		r, err := s.Store.GetReturn(ctx, o.ReturnID, true)
		if err != nil {
			return err
		}
		return s.save(ctx, &r, audit.ActionStatusChange, false, func(ctx context.Context, r *domain.SalesReturn) (string, error) {
			if err := r.ApplyQC(o.ItemID, o.PassedQty, o.RejectedQty); err != nil {
				return "", err
			}
			if !r.QCComplete() {
				return fmt.Sprintf("QC Inbound: lulus %s, ditolak %s", o.PassedQty, o.RejectedQty), nil
			}
			return "QC selesai; retur diposting", s.post(ctx, r)
		})
	})
}

func (s *ReturnService) post(ctx context.Context, r *domain.SalesReturn) error {
	journalID := uuid.Nil
	if r.Amount.IsPositive() || r.Cost.IsPositive() {
		receivable := AccountReceivable
		if r.Settlement == domain.SettlementMarketplace {
			labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{r.SubChannelID})
			if err != nil {
				return err
			}
			if acct := domain.MarketplaceReceivableAccount(labels[r.SubChannelID].Code); acct != "" {
				receivable = acct
			}
		}
		req := accounting.PostingRequest{
			RuleCode: RuleSalesReturn, SourceType: SourceSalesReturn, SourceID: r.ID, SourceLabel: r.Number,
			BusinessDate: s.today(),
			Amounts:      map[string]money.Amount{"sales_return": r.Amount, "cogs_value": r.Cost},
			Accounts:     map[accounting.Resolver]string{accounting.ResolverReceivable: receivable},
			Dimensions:   salesDimensions(r.ChannelID, r.SubChannelID),
			Description:  fmt.Sprintf("Retur penjualan %s (%s)", r.Number, r.OrderNumber),
		}
		if r.PartyID != nil {
			req.Party = &accounting.Party{Type: r.PartyType, ID: *r.PartyID}
		}
		entry, err := s.Ledger.Post(ctx, req)
		if err != nil {
			return err
		}
		journalID = entry.ID
	}
	if err := r.Post(journalID, s.Clock.Now()); err != nil {
		return err
	}
	if r.Settlement == domain.SettlementMarketplace || r.PartyID == nil || !r.Amount.IsPositive() {
		return nil
	}
	return s.issueCreditNote(ctx, *r)
}

// issueCreditNote membuat nota kredit (CN) retur: nota kredit langsung dialokasikan ke sisa piutang invoice asal;
// refund diajukan ke Head Finance & Accounting.
func (s *ReturnService) issueCreditNote(ctx context.Context, r domain.SalesReturn) error {
	date := s.today()
	number, err := s.Numberer.Next(ctx, DocCreditNote, date)
	if err != nil {
		return err
	}
	cn, err := domain.NewCreditNote(number, r, date)
	if err != nil {
		return err
	}
	if cn.ID, err = s.Store.InsertCreditNote(ctx, cn, actor.UserID(ctx)); err != nil {
		return err
	}
	cn.Version = 1
	if err := s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: EntityCreditNote, EntityID: cn.ID, EntityLabel: cn.Number,
		After: creditNoteSnapshot(cn), Note: "Dari Retur Penjualan " + r.Number,
	}); err != nil {
		return err
	}
	if cn.Settlement == domain.SettlementRefund {
		_, err := s.Approvals.Request(ctx, approval.Subject{
			Type: approval.SubjectSalesRefund, ID: cn.ID, Label: cn.Number, Link: CreditNotePath(cn.ID),
			Summary: fmt.Sprintf("Refund retur %s ke %s", r.Number, cn.PartyName), Amount: cn.Amount,
		})
		return err
	}
	return s.autoApply(ctx, &cn)
}

// autoApply mengalokasikan saldo nota kredit ke sisa piutang invoice asal (bila masih terbuka).
func (s *ReturnService) autoApply(ctx context.Context, cn *domain.CreditNote) error {
	if cn.InvoiceID == nil || !cn.CanApply() {
		return nil
	}
	inv, err := s.Store.LockInvoice(ctx, *cn.InvoiceID)
	if err != nil {
		return err
	}
	if !inv.IsOpen() {
		return nil
	}
	amt := cn.Remaining()
	if inv.Outstanding().Cmp(amt) < 0 {
		amt = inv.Outstanding()
	}
	ver := cn.Version
	before := creditNoteSnapshot(*cn)
	if err := s.applyToInvoice(ctx, cn, inv, amt); err != nil {
		return err
	}
	if err := s.Store.SaveCreditNote(ctx, *cn, ver, actor.UserID(ctx)); err != nil {
		return err
	}
	cn.Version = ver + 1
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionUpdate, EntityType: EntityCreditNote, EntityID: cn.ID, EntityLabel: cn.Number,
		Before: before, After: creditNoteSnapshot(*cn), Note: "Dialokasikan ke invoice asal " + inv.Number,
	})
}

func (s *ReturnService) applyToInvoice(ctx context.Context, cn *domain.CreditNote, inv domain.Invoice, amount money.Amount) error {
	app, err := cn.Apply(inv, amount, s.today())
	if err != nil {
		return err
	}
	iv := inv.Version
	if err := inv.ApplyPayment(amount); err != nil {
		return err
	}
	if err := s.Store.SaveInvoice(ctx, inv, iv, actor.UserID(ctx)); err != nil {
		return err
	}
	return s.Store.InsertCreditNoteApplication(ctx, cn.ID, app, actor.UserID(ctx))
}

// --- Nota Kredit -----------------------------------------------------------------------------

func creditNoteSnapshot(c domain.CreditNote) map[string]any {
	return map[string]any{
		"number": c.Number, "settlementMethod": string(c.Settlement), "status": string(c.Status),
		"amount": c.Amount.String(), "appliedAmount": c.Applied.String(), "refundedAmount": c.Refunded.String(),
		"refundAccountCode": c.RefundAccount, "refundReference": c.RefundReference,
	}
}

var creditNoteActionPerms = map[domain.Action][]string{
	domain.ActionPrint: {PermCreditNoteRead}, domain.ActionApply: {PermCreditNotePost},
	domain.ActionPayRefund: {PermPaymentPost},
}

// CreditNoteActions mengembalikan aksi nota kredit yang boleh dilakukan actor.
func (s *ReturnService) CreditNoteActions(ctx context.Context, c domain.CreditNote) []string {
	return filterActions(ctx, c.Actions(), creditNoteActionPerms)
}

// CreditNoteView adalah detail nota kredit beserta invoice terbuka pihak yang sama (pilihan alokasi).
type CreditNoteView struct {
	domain.CreditNote
	OpenInvoices []domain.Invoice
	Actions      []string
}

// ListCreditNotes mengembalikan daftar nota kredit.
func (s *ReturnService) ListCreditNotes(ctx context.Context, f postgres.CreditNoteFilter, page paging.Page) (paging.Result[domain.CreditNote], error) {
	if _, err := actor.RequireAny(ctx, PermCreditNoteRead, PermReturnRead); err != nil {
		return paging.Result[domain.CreditNote]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListCreditNotes(ctx, f, page)
}

// GetCreditNote mengembalikan detail nota kredit.
func (s *ReturnService) GetCreditNote(ctx context.Context, id uuid.UUID) (CreditNoteView, error) {
	if _, err := actor.RequireAny(ctx, PermCreditNoteRead, PermReturnRead); err != nil {
		return CreditNoteView{}, err
	}
	c, err := s.Store.GetCreditNote(ctx, id, false)
	if err != nil {
		return CreditNoteView{}, err
	}
	v := CreditNoteView{CreditNote: c, OpenInvoices: []domain.Invoice{}, Actions: s.CreditNoteActions(ctx, c)}
	if c.CanApply() {
		pid := c.PartyID
		size, n := 100, 1
		list, err := s.Store.ListInvoices(ctx, postgres.InvoiceFilter{
			Statuses: []domain.Status{domain.StatusAwaitingPayment, domain.StatusPartiallyPaid}, PartyType: c.PartyType, PartyID: &pid,
		}, paging.New(&n, &size))
		if err != nil {
			return CreditNoteView{}, err
		}
		v.OpenInvoices = list.Items
	}
	return v, nil
}

// CreditNoteAllocation adalah alokasi saldo nota kredit ke satu invoice.
type CreditNoteAllocation struct {
	InvoiceID uuid.UUID
	Amount    money.Amount
}

// ApplyCreditNote mengalokasikan saldo nota kredit ke invoice terbuka pihak yang sama (F&A).
func (s *ReturnService) ApplyCreditNote(ctx context.Context, id uuid.UUID, ver int, allocs []CreditNoteAllocation) (CreditNoteView, error) {
	if _, err := actor.Require(ctx, PermCreditNotePost); err != nil {
		return CreditNoteView{}, err
	}
	if len(allocs) == 0 {
		return CreditNoteView{}, apperr.Validation(map[string]string{"allocations": "Pilih minimal satu invoice"})
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetCreditNote(ctx, id, true)
		if err != nil {
			return err
		}
		if c.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := creditNoteSnapshot(c)
		numbers := []string{}
		for i, a := range allocs {
			inv, err := s.Store.LockInvoice(ctx, a.InvoiceID)
			if apperr.HasCode(err, apperr.CodeNotFound) {
				return apperr.Validation(map[string]string{fmt.Sprintf("allocations.%d.invoiceId", i): "Invoice tidak ditemukan"})
			}
			if err != nil {
				return err
			}
			if err := s.applyToInvoice(ctx, &c, inv, a.Amount); err != nil {
				return prefixFields(err, fmt.Sprintf("allocations.%d.", i))
			}
			numbers = append(numbers, inv.Number)
		}
		if err := s.Store.SaveCreditNote(ctx, c, ver, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityCreditNote, EntityID: c.ID, EntityLabel: c.Number,
			Before: before, After: creditNoteSnapshot(c), Note: "Dialokasikan ke " + strings.Join(numbers, ", "),
		})
	})
	if err != nil {
		return CreditNoteView{}, err
	}
	return s.GetCreditNote(ctx, id)
}

func prefixFields(err error, prefix string) error {
	e := apperr.As(err)
	if e == nil || e.Code != apperr.CodeValidationFailed || len(e.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(e.Fields))
	for k, v := range e.Fields {
		fields[prefix+k] = v
	}
	return apperr.Validation(fields)
}

// PayRefund mencatat pembayaran refund nota kredit yang sudah disetujui (aksi ★ re-autentikasi): jurnal A-PB
// Dr Piutang Usaha, Cr Kas/Bank.
func (s *ReturnService) PayRefund(ctx context.Context, id uuid.UUID, ver int, in domain.RefundInput) (CreditNoteView, error) {
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPaymentPost); err != nil {
		return CreditNoteView{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetCreditNote(ctx, id, true)
		if err != nil {
			return err
		}
		if c.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := creditNoteSnapshot(c)
		accounts, err := s.cashAccountCodes(ctx)
		if err != nil {
			return err
		}
		amount := c.Remaining()
		if err := c.PayRefund(in, accounts); err != nil {
			return err
		}
		entry, err := s.Ledger.Post(ctx, accounting.PostingRequest{
			RuleCode: RulePayment, SourceType: SourceSalesRefund, SourceID: c.ID, SourceLabel: c.Number,
			BusinessDate: in.PaymentDate,
			Amounts:      map[string]money.Amount{"incoming_value": amount.Neg()},
			Accounts: map[accounting.Resolver]string{
				accounting.ResolverBank: c.RefundAccount, accounting.ResolverReceivable: AccountReceivable,
			},
			Party:       &accounting.Party{Type: c.PartyType, ID: c.PartyID},
			Description: "Refund nota kredit " + c.Number + " ke " + c.PartyName,
		})
		if err != nil {
			return err
		}
		c.RefundJournalID = &entry.ID
		if err := s.Store.SaveCreditNote(ctx, c, ver, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntityCreditNote, EntityID: c.ID, EntityLabel: c.Number,
			Before: before, After: creditNoteSnapshot(c), Note: "Refund dibayar",
		})
	})
	if err != nil {
		return CreditNoteView{}, err
	}
	return s.GetCreditNote(ctx, id)
}

func (s *ReturnService) decideRefund(ctx context.Context, d approval.Decided, approved bool) error {
	c, err := s.Store.GetCreditNote(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	before, ver := creditNoteSnapshot(c), c.Version
	note := "Refund disetujui"
	if approved {
		err = c.ApproveRefund(d.Note)
	} else {
		err = c.RejectRefund(d.Note)
		note = "Refund ditolak; saldo menjadi nota kredit"
	}
	if err != nil {
		return err
	}
	if err := s.Store.SaveCreditNote(ctx, c, ver, actor.UserID(ctx)); err != nil {
		return err
	}
	c.Version = ver + 1
	if err := s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionStatusChange, EntityType: EntityCreditNote, EntityID: c.ID, EntityLabel: c.Number,
		Before: before, After: creditNoteSnapshot(c), Note: note,
	}); err != nil {
		return err
	}
	if !approved {
		return s.autoApply(ctx, &c)
	}
	return nil
}

// OnRefundApproved: Head F&A menyetujui refund → Menunggu Pembayaran oleh F&A.
func (s *ReturnService) OnRefundApproved(ctx context.Context, d approval.Decided) error {
	return s.decideRefund(ctx, d, true)
}

// OnRefundRejected: refund ditolak → saldo menjadi nota kredit dan dialokasikan ke invoice asal bila terbuka.
func (s *ReturnService) OnRefundRejected(ctx context.Context, d approval.Decided) error {
	return s.decideRefund(ctx, d, false)
}
