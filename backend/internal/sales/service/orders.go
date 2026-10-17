package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/pricing"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// OrderFilter adalah kriteria daftar pesanan.
type OrderFilter = postgres.OrderFilter

// OrderSortFields adalah field sort daftar pesanan.
var OrderSortFields = []string{"number", "businessDate", "totalAmount"}

// OrderView adalah pesanan beserta data tampilan dan dokumen terkait.
type OrderView struct {
	domain.Order
	Products         map[uuid.UUID]ProductRef
	Location         *LocationRef
	Channel          ChannelRef
	SubChannel       ChannelRef
	Proformas        []domain.Proforma
	Invoices         []domain.Invoice
	Shipments        []domain.Shipment
	CreatedByName    string
	PaymentProofName string
}

// OrderSummary adalah baris daftar pesanan.
type OrderSummary struct {
	domain.Order
	ChannelName    string
	SubChannelName string
}

func readPermission(t domain.OrderType) string {
	if t == domain.OrderMarketplace {
		return PermMarketplaceRead
	}
	return PermOrderRead
}

func writePermission(t domain.OrderType) string {
	if t == domain.OrderMarketplace {
		return PermMarketplaceWrite
	}
	return PermOrderWrite
}

func cancelPermission(t domain.OrderType) string {
	if t == domain.OrderMarketplace {
		return PermMarketplaceWrite
	}
	return PermOrderCancel
}

// readableTypes adalah jenis pesanan yang boleh dilihat actor.
func readableTypes(a actor.Actor) []domain.OrderType {
	var out []domain.OrderType
	for _, t := range domain.OrderTypes {
		if a.Has(readPermission(t)) {
			out = append(out, t)
		}
	}
	return out
}

func orderActionPerms(t domain.OrderType) map[domain.Action][]string {
	return map[domain.Action][]string{
		domain.ActionEdit: {writePermission(t)}, domain.ActionConfirm: {PermOrderSubmit},
		domain.ActionCancel: {cancelPermission(t)}, domain.ActionUploadProof: {PermOrderWrite, PermProformaWrite},
	}
}

// OrderActions mengembalikan aksi pesanan yang boleh dilakukan actor (allowedActions).
func OrderActions(ctx context.Context, o domain.Order) []string {
	return filterActions(ctx, o.Actions(), orderActionPerms(o.Type))
}

func orderSnapshot(o domain.Order) map[string]any {
	lines := make([]map[string]any, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "qty": l.Qty.String(), "unitPrice": l.Price.UnitPrice.String(),
			"priceRule": l.Price.Rule, "priceListVersionId": l.Price.PriceListVersionID,
			"discount": l.DiscountAmount.String(), "total": l.Total.String(),
		}
	}
	return map[string]any{
		"orderType": string(o.Type), "channelId": o.ChannelID, "subChannelId": o.SubChannelID,
		"customerId": o.CustomerID, "partnerId": o.PartnerID, "externalOrderId": o.ExternalOrderID,
		"clientPoNumber": o.ClientPONumber, "businessDate": o.BusinessDate.String(),
		"sourceLocationId": o.SourceLocationID, "paymentMode": string(o.PaymentMode),
		"paymentTermDays": o.PaymentTermDays, "shippingAmount": o.Shipping.String(),
		"discountAmount": o.TotalDiscount().String(), "totalAmount": o.Total.String(), "status": string(o.Status),
		"cancelReason": o.CancelReason, "note": o.Note, "lines": lines,
	}
}

// --- rujukan & validasi ----------------------------------------------------------------------

// channelIndex membaca channel & sub-channel aktif.
func (s *DocumentService) channelIndex(ctx context.Context) (map[uuid.UUID]ChannelRef, error) {
	list, err := s.Ledger.Channels(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]ChannelRef, len(list))
	for _, c := range list {
		out[c.ID] = c
	}
	return out, nil
}

// channelByCode mencari channel aktif berdasarkan kode.
func (s *DocumentService) channelByCode(ctx context.Context, code string) (ChannelRef, bool, error) {
	list, err := s.Ledger.Channels(ctx)
	if err != nil {
		return ChannelRef{}, false, err
	}
	for _, c := range list {
		if c.Kind == "channel" && c.Code == code {
			return c, true, nil
		}
	}
	return ChannelRef{}, false, nil
}

func (s *DocumentService) party(ctx context.Context, in domain.OrderInput) (*domain.PartyInfo, error) {
	var (
		p   domain.PartyInfo
		ok  bool
		err error
	)
	switch {
	case in.PartnerID != nil:
		p, ok, err = s.Parties.Partner(ctx, *in.PartnerID)
	case in.CustomerID != nil:
		p, ok, err = s.Parties.Customer(ctx, *in.CustomerID)
	default:
		return nil, nil
	}
	if err != nil || !ok {
		return nil, err
	}
	return &p, nil
}

// orderRefs membaca channel, pihak, produk, lokasi, dan harga per baris (resolver; marketplace boleh harga input).
func (s *DocumentService) orderRefs(ctx context.Context, in domain.OrderInput) (domain.OrderRefs, map[uuid.UUID]ProductRef, error) {
	refs := domain.OrderRefs{Products: map[uuid.UUID]domain.ProductInfo{}}
	channels, err := s.channelIndex(ctx)
	if err != nil {
		return refs, nil, err
	}
	if c, ok := channels[in.ChannelID]; ok && c.Kind == "channel" {
		refs.ChannelCode = c.Code
	}
	if sc, ok := channels[in.SubChannelID]; ok && sc.Kind == "sub_channel" && sc.ChannelID != nil && *sc.ChannelID == in.ChannelID {
		refs.SubChannelValid = true
	}
	if refs.Party, err = s.party(ctx, in); err != nil {
		return refs, nil, err
	}
	if refs.Party != nil && in.Type == domain.OrderEmployee && in.CustomerID != nil && !in.BusinessDate.IsZero() {
		if refs.EmployeeEligible, err = s.Parties.EmployeeEligible(ctx, *in.CustomerID, in.BusinessDate); err != nil {
			return refs, nil, err
		}
	}
	if in.SourceLocationID != uuid.Nil {
		locs, err := s.Locations.Locations(ctx, []uuid.UUID{in.SourceLocationID})
		if err != nil {
			return refs, nil, err
		}
		if l, ok := locs[in.SourceLocationID]; ok {
			refs.Location = &domain.LocationInfo{ID: l.ID, Type: l.Type, IsActive: l.IsActive, Deleted: l.Deleted}
		}
	}
	ids := make([]uuid.UUID, 0, len(in.Lines))
	for _, l := range in.Lines {
		ids = append(ids, l.ProductID)
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return refs, nil, err
	}
	for id, p := range products {
		refs.Products[id] = domain.ProductInfo{ID: id, SKU: p.SKU, Type: p.Type, IsBundle: p.IsBundle, IsActive: p.IsActive, Deleted: p.Deleted}
	}
	refs.Prices = make([]*domain.LinePrice, len(in.Lines))
	for i, l := range in.Lines {
		if _, ok := products[l.ProductID]; !ok || !l.Qty.IsPositive() || in.BusinessDate.IsZero() {
			continue
		}
		if in.Type == domain.OrderMarketplace && l.UnitPrice != nil {
			refs.Prices[i] = &domain.LinePrice{UnitPrice: *l.UnitPrice, Rule: domain.PriceRuleMarketplaceInput, Description: "Harga di marketplace"}
			continue
		}
		price, err := s.resolvePrice(ctx, in, l)
		if err != nil {
			return refs, nil, err
		}
		refs.Prices[i] = price
	}
	return refs, products, nil
}

// resolvePrice memanggil resolver harga 5 aturan (BRD 5.18); produk tanpa harga mengembalikan nil.
func (s *DocumentService) resolvePrice(ctx context.Context, in domain.OrderInput, l domain.LineInput) (*domain.LinePrice, error) {
	ch, sub := in.ChannelID, in.SubChannelID
	req := pricing.PriceRequest{ProductID: l.ProductID, Qty: l.Qty, CustomerID: in.CustomerID, PartnerID: in.PartnerID, Date: in.BusinessDate}
	if ch != uuid.Nil {
		req.ChannelID = &ch
		if sub != uuid.Nil {
			req.SubChannelID = &sub
		}
	}
	q, err := s.Pricing.Resolve(ctx, req)
	if code := apperr.As(err).Code; err != nil && (code == apperr.CodeNotFound || code == apperr.CodeValidationFailed) {
		return nil, nil //nolint:nilerr // produk tanpa harga dilaporkan sebagai error field baris oleh domain
	}
	if err != nil {
		return nil, err
	}
	src := q.SourceID
	out := &domain.LinePrice{
		UnitPrice: q.UnitPrice, Rule: string(q.Rule), PriceListID: q.PriceListID, PriceListVersionID: q.PriceListVersionID,
		Description: q.Description,
	}
	if src != uuid.Nil {
		out.SourceID = &src
	}
	return out, nil
}

func componentsOf(products map[uuid.UUID]ProductRef) map[uuid.UUID][]domain.ComponentQty {
	out := map[uuid.UUID][]domain.ComponentQty{}
	for id, p := range products {
		if p.IsBundle {
			out[id] = p.Components
		}
	}
	return out
}

// --- baca -----------------------------------------------------------------------------------

// ListOrders mengembalikan daftar pesanan sesuai jenis yang boleh dilihat actor.
func (s *DocumentService) ListOrders(ctx context.Context, f OrderFilter, page paging.Page) (paging.Result[OrderSummary], error) {
	a, ok := actor.From(ctx)
	if !ok {
		return paging.Result[OrderSummary]{}, apperr.New(apperr.CodeForbidden)
	}
	allowed := readableTypes(a)
	if len(allowed) == 0 {
		return paging.Result[OrderSummary]{}, apperr.New(apperr.CodeForbidden)
	}
	if len(f.Types) == 0 {
		f.Types = allowed
	} else {
		var types []domain.OrderType
		for _, t := range f.Types {
			if slices.Contains(allowed, t) {
				types = append(types, t)
			}
		}
		if len(types) == 0 {
			return paging.Result[OrderSummary]{}, apperr.New(apperr.CodeForbidden)
		}
		f.Types = types
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Store.ListOrders(ctx, f, page)
	if err != nil {
		return paging.Result[OrderSummary]{}, err
	}
	ids := []uuid.UUID{}
	for _, o := range res.Items {
		ids = append(ids, o.ChannelID, o.SubChannelID)
	}
	labels, err := s.Ledger.ChannelLabels(ctx, uniqueIDs(ids))
	if err != nil {
		return paging.Result[OrderSummary]{}, err
	}
	items := make([]OrderSummary, len(res.Items))
	for i, o := range res.Items {
		items[i] = OrderSummary{Order: o, ChannelName: labels[o.ChannelID].Name, SubChannelName: labels[o.SubChannelID].Name}
	}
	return pageOf(items, res.Page, res.Total), nil
}

// GetOrder mengembalikan detail pesanan.
func (s *DocumentService) GetOrder(ctx context.Context, id uuid.UUID) (OrderView, error) {
	o, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return OrderView{}, err
	}
	if _, err := actor.Require(ctx, readPermission(o.Type)); err != nil {
		return OrderView{}, err
	}
	return s.orderView(ctx, o)
}

func (s *DocumentService) orderView(ctx context.Context, o domain.Order) (OrderView, error) {
	v := OrderView{Order: o}
	ids := []uuid.UUID{}
	for _, l := range o.Lines {
		ids = append(ids, l.ProductID)
	}
	for _, a := range o.Allocations {
		ids = append(ids, a.ProductID)
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return OrderView{}, err
	}
	locs, err := s.Locations.Locations(ctx, []uuid.UUID{o.SourceLocationID})
	if err != nil {
		return OrderView{}, err
	}
	if l, ok := locs[o.SourceLocationID]; ok {
		v.Location = &l
	}
	labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{o.ChannelID, o.SubChannelID})
	if err != nil {
		return OrderView{}, err
	}
	v.Channel, v.SubChannel = labels[o.ChannelID], labels[o.SubChannelID]
	if v.Proformas, err = s.Store.ProformasByOrder(ctx, o.ID); err != nil {
		return OrderView{}, err
	}
	if v.Invoices, err = s.Store.InvoicesByOrder(ctx, o.ID); err != nil {
		return OrderView{}, err
	}
	if v.Shipments, err = s.Store.ShipmentsByOrder(ctx, o.ID); err != nil {
		return OrderView{}, err
	}
	names, err := s.Users.UserNames(ctx, []uuid.UUID{o.CreatedBy})
	if err != nil {
		return OrderView{}, err
	}
	v.CreatedByName = names[o.CreatedBy]
	if o.PaymentProofFileID != nil {
		files, err := s.Files.Names(ctx, []uuid.UUID{*o.PaymentProofFileID})
		if err != nil {
			return OrderView{}, err
		}
		v.PaymentProofName = files[*o.PaymentProofFileID]
	}
	return v, nil
}

// --- tulis ----------------------------------------------------------------------------------

// CreateOrder menyimpan draft pesanan MT/B2B/WA-IG/karyawan dengan harga otomatis per baris.
func (s *DocumentService) CreateOrder(ctx context.Context, in domain.OrderInput) (OrderView, error) {
	if in.Type == domain.OrderMarketplace {
		return s.CreateMarketplaceOrder(ctx, in)
	}
	a, err := actor.Require(ctx, PermOrderWrite)
	if err != nil {
		return OrderView{}, err
	}
	var o domain.Order
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		refs, _, err := s.orderRefs(ctx, in)
		if err != nil {
			return err
		}
		if o, err = domain.NewOrder(in, refs, a.UserID); err != nil {
			return err
		}
		if o, err = s.Store.InsertOrder(ctx, o, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntitySalesOrder, EntityID: o.ID, EntityLabel: o.Label(), After: orderSnapshot(o),
		})
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.reload(ctx, o.ID)
}

func (s *DocumentService) reload(ctx context.Context, id uuid.UUID) (OrderView, error) {
	o, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return OrderView{}, err
	}
	return s.orderView(ctx, o)
}

// mutateOrder mengunci pesanan, memeriksa versi, menjalankan fn, menyimpan, dan mencatat audit.
func (s *DocumentService) mutateOrder(ctx context.Context, id uuid.UUID, ver int, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, o *domain.Order) (string, error),
) (OrderView, error) {
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.GetOrder(ctx, id, true)
		if err != nil {
			return err
		}
		if o.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		return s.saveOrderChange(ctx, &o, action, replaceLines, fn)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.reload(ctx, id)
}

// saveOrderChange menjalankan fn pada pesanan terkunci lalu menyimpan dan mencatat audit.
func (s *DocumentService) saveOrderChange(ctx context.Context, o *domain.Order, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, o *domain.Order) (string, error),
) error {
	before := orderSnapshot(*o)
	note, err := fn(ctx, o)
	if err != nil {
		return err
	}
	if err := s.Store.SaveOrder(ctx, o, o.Version, actor.UserID(ctx), replaceLines); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: action, EntityType: entityOf(o.Type), EntityID: o.ID, EntityLabel: o.Label(),
		Before: before, After: orderSnapshot(*o), Note: note,
	})
}

// UpdateOrder mengubah draft pesanan (harga dihitung ulang).
func (s *DocumentService) UpdateOrder(ctx context.Context, id uuid.UUID, ver int, in domain.OrderInput) (OrderView, error) {
	cur, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return OrderView{}, err
	}
	if _, err := actor.Require(ctx, writePermission(cur.Type)); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, ver, audit.ActionUpdate, true, func(ctx context.Context, o *domain.Order) (string, error) {
		in.Type = o.Type
		refs, _, err := s.orderRefs(ctx, in)
		if err != nil {
			return "", err
		}
		return "", o.Update(in, refs)
	})
}

// OrderInputOf menyusun ulang isian dari pesanan (validasi ulang saat konfirmasi).
func OrderInputOf(o domain.Order) domain.OrderInput {
	in := domain.OrderInput{
		Type: o.Type, ChannelID: o.ChannelID, SubChannelID: o.SubChannelID, CustomerID: o.CustomerID, PartnerID: o.PartnerID,
		ExternalOrderID: o.ExternalOrderID, ClientPONumber: o.ClientPONumber, BusinessDate: o.BusinessDate,
		SourceLocationID: o.SourceLocationID, PaymentMode: o.PaymentMode, PaymentTermDays: o.PaymentTermDays,
		ShipToName: o.ShipToName, ShipToPhone: o.ShipToPhone, ShipToAddress: o.ShipToAddress,
		DocumentDiscount: o.DocumentDiscount, Shipping: o.Shipping, Note: o.Note,
	}
	if o.Type == domain.OrderMarketplace {
		in.BuyerName = o.PartyName
	}
	for _, l := range o.Lines {
		li := domain.LineInput{ProductID: l.ProductID, Qty: l.Qty, Discount: l.Discount, Note: l.Note}
		if l.Price.Rule == domain.PriceRuleMarketplaceInput {
			p := l.Price.UnitPrice
			li.UnitPrice = &p
		}
		in.Lines = append(in.Lines, li)
	}
	return in
}

// discountCheck memeriksa diskon manual pesanan terhadap batas role pembuat (MD-12).
func (s *DocumentService) discountCheck(ctx context.Context, a actor.Actor, o domain.Order) (pricing.DiscountCheck, error) {
	lines := make([]pricing.LineDiscount, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = pricing.LineDiscount{Gross: l.Gross, Discount: pricing.Discount{Kind: pricing.DiscountKind(l.Discount.Kind), Value: l.Discount.Value}}
	}
	doc := pricing.DocumentDiscount{
		Subtotal: o.Subtotal(),
		Discount: pricing.Discount{Kind: pricing.DiscountKind(o.DocumentDiscount.Kind), Value: o.DocumentDiscount.Value},
	}
	return s.Pricing.CheckDiscount(ctx, a.Roles, lines, doc)
}

// ConfirmOrder mengonfirmasi draft: harga dihitung ulang pada tanggal pesanan, nomor SO diberikan, stok
// dialokasikan FEFO dari lokasi asal (bundel dipecah), lalu Proforma Invoice terbit (prepaid) atau invoice tempo
// terbit dengan jurnal A-05 (credit). Diskon di atas batas role → Menunggu Persetujuan `sales_order_discount`.
func (s *DocumentService) ConfirmOrder(ctx context.Context, id uuid.UUID, ver int) (OrderView, error) {
	a, err := actor.Require(ctx, PermOrderSubmit)
	if err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, ver, audit.ActionSubmit, false, func(ctx context.Context, o *domain.Order) (string, error) {
		in := OrderInputOf(*o)
		refs, products, err := s.orderRefs(ctx, in)
		if err != nil {
			return "", err
		}
		if err := o.Update(in, refs); err != nil {
			return "", err
		}
		check, err := s.discountCheck(ctx, a, *o)
		if err != nil {
			return "", err
		}
		number := ""
		if o.Number == "" {
			if number, err = s.Numberer.Next(ctx, DocSalesOrder, o.BusinessDate); err != nil {
				return "", err
			}
		}
		if err := o.Confirm(number, check.RequiresApproval, s.Clock.Now()); err != nil {
			return "", err
		}
		// Baris disimpan ulang sebelum alokasi agar alokasi merujuk ID baris yang baru.
		if err := s.Store.SaveOrder(ctx, o, o.Version, a.UserID, true); err != nil {
			return "", err
		}
		if err := s.allocate(ctx, o, products); err != nil {
			return "", err
		}
		if check.RequiresApproval {
			_, err := s.Approvals.Request(ctx, approval.Subject{
				Type: approval.SubjectSalesOrderDiscount, ID: o.ID, Label: o.Number, Link: OrderPath(o.ID),
				Summary: discountSummary(*o, check), Amount: check.TotalDiscountAmount,
			})
			return "Diskon di atas batas role, menunggu persetujuan Head Sales", err
		}
		return "", s.afterConfirm(ctx, o)
	})
}

func discountSummary(o domain.Order, check pricing.DiscountCheck) string {
	return fmt.Sprintf("%s · %s · diskon %s dari %s (batas baris %s%%, dokumen %s%%)", o.Type.Label(), o.PartyName,
		check.TotalDiscountAmount.Rupiah(), o.Gross.Rupiah(), check.Limit.MaxLinePercent.String(),
		check.Limit.MaxDocumentPercent.String())
}

// afterConfirm menerbitkan Proforma Invoice (prepaid) atau invoice tempo + A-05 (credit) setelah pesanan
// dikonfirmasi dan diskonnya tidak perlu/sudah disetujui. Marketplace tidak berdokumen tagihan.
func (s *DocumentService) afterConfirm(ctx context.Context, o *domain.Order) error {
	switch {
	case o.Type == domain.OrderMarketplace:
		return nil
	case o.PaymentMode == domain.PaymentCredit:
		_, err := s.issueInvoice(ctx, o, s.today())
		return err
	}
	number, err := s.Numberer.Next(ctx, DocProforma, s.today())
	if err != nil {
		return err
	}
	pi := domain.NewProforma(number, *o, s.today(), s.ProformaValidDays)
	if pi.ID, err = s.Store.InsertProforma(ctx, pi, actor.UserID(ctx)); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: EntityProforma, EntityID: pi.ID, EntityLabel: pi.Number,
		After: map[string]any{"salesOrder": o.Number, "amount": pi.Amount.String(), "validUntil": pi.ValidUntil.String()},
	})
}

// allocate memecah baris ke komponen dan mereservasi stok FEFO dari lokasi asal (available → reserved).
func (s *DocumentService) allocate(ctx context.Context, o *domain.Order, products map[uuid.UUID]ProductRef) error {
	allocs, err := o.PlanAllocations(componentsOf(products), func() uuid.UUID { return uuid.Must(uuid.NewV7()) })
	if err != nil {
		return err
	}
	lines := make([]inventory.ReserveLine, len(allocs))
	for i, a := range allocs {
		lineID := a.ID
		lines[i] = inventory.ReserveLine{SourceLineID: &lineID, ProductID: a.ProductID, LocationID: a.LocationID, Qty: a.Qty}
	}
	res, err := s.Stock.Reserve(ctx, inventory.ReserveRequest{
		Source: inventory.Source{
			Type: SourceSalesOrder, ID: o.ID, Action: fmt.Sprintf("reserve-%d", o.AllocationRound), Label: o.Number,
			BusinessDate: s.today(),
		},
		Lines: lines,
	})
	if err != nil {
		return err
	}
	for _, m := range res.Movements {
		if m.Type == inventory.MovementMoveIn && m.Status == inventory.StatusReserved && m.LineIndex < len(allocs) {
			allocs[m.LineIndex].Batches = append(allocs[m.LineIndex].Batches, domain.AllocationBatch{BatchID: m.BatchID, Qty: m.Qty})
		}
	}
	if err := s.Store.ReplaceAllocations(ctx, o.ID, allocs); err != nil {
		return err
	}
	o.Allocations = allocs
	return nil
}

// release melepas reservasi terbuka pesanan (batal, ditolak, kedaluwarsa) kembali ke available.
func (s *DocumentService) release(ctx context.Context, o *domain.Order, reason string) error {
	open := false
	for _, a := range o.Allocations {
		open = open || (!a.Released && a.OutstandingQty().IsPositive())
	}
	if !open {
		return nil
	}
	if _, err := s.Stock.Release(ctx, inventory.ReleaseRequest{
		Source: inventory.Source{
			Type: SourceSalesOrder, ID: o.ID, Action: fmt.Sprintf("%s-%d", reason, o.AllocationRound), Label: o.Number,
			BusinessDate: s.today(),
		},
		Reservation: inventory.ReservationRef{SourceType: SourceSalesOrder, SourceID: o.ID},
	}); err != nil {
		return err
	}
	for i := range o.Allocations {
		if o.Allocations[i].OutstandingQty().IsPositive() {
			o.Allocations[i].Released = true
		}
	}
	return s.Store.SaveAllocationState(ctx, o.Allocations)
}

// CancelOrder membatalkan pesanan yang belum dikirim: approval dibatalkan, alokasi dilepas, Proforma dibatalkan,
// dan invoice tempo tanpa pembayaran dibatalkan dengan jurnal balik A-05.
func (s *DocumentService) CancelOrder(ctx context.Context, id uuid.UUID, ver int, reason string) (OrderView, error) {
	cur, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return OrderView{}, err
	}
	if _, err := actor.Require(ctx, cancelPermission(cur.Type)); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, ver, audit.ActionCancel, false, func(ctx context.Context, o *domain.Order) (string, error) {
		status := o.Status
		if err := o.Cancel(reason); err != nil {
			return "", err
		}
		if status == domain.StatusPendingApproval {
			if err := s.Approvals.Cancel(ctx, approval.SubjectSalesOrderDiscount, o.ID); err != nil {
				return "", err
			}
		}
		if err := s.release(ctx, o, "cancel"); err != nil {
			return "", err
		}
		if err := s.cancelBilling(ctx, o, o.CancelReason); err != nil {
			return "", err
		}
		return o.CancelReason, nil
	})
}

// cancelBilling membatalkan Proforma terbuka dan invoice tempo tanpa pembayaran milik pesanan.
func (s *DocumentService) cancelBilling(ctx context.Context, o *domain.Order, reason string) error {
	pis, err := s.Store.ProformasByOrder(ctx, o.ID)
	if err != nil {
		return err
	}
	for _, p := range pis {
		if p.Status != domain.StatusAwaitingPayment {
			continue
		}
		ver := p.Version
		if err := p.Cancel(reason); err != nil {
			return err
		}
		if err := s.Store.SaveProforma(ctx, p, ver, actor.UserID(ctx)); err != nil {
			return err
		}
	}
	invoices, err := s.Store.InvoicesByOrder(ctx, o.ID)
	if err != nil {
		return err
	}
	for _, inv := range invoices {
		if inv.Status == domain.StatusCancelled {
			continue
		}
		ver := inv.Version
		if err := inv.Cancel(reason, s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.SaveInvoice(ctx, inv, ver, actor.UserID(ctx)); err != nil {
			return err
		}
		if _, err := s.Ledger.Reverse(ctx, accountingReversal(SourceInvoice, inv.ID, RuleSales, s.today(), "Batal "+o.Number)); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCancel, EntityType: EntityInvoice, EntityID: inv.ID, EntityLabel: inv.Number, Note: reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UploadPaymentProof mencatat bukti bayar pelanggan pada pesanan yang Menunggu Pembayaran (WA/IG, karyawan).
func (s *DocumentService) UploadPaymentProof(ctx context.Context, id uuid.UUID, ver int, fileID uuid.UUID) (OrderView, error) {
	if _, err := actor.RequireAny(ctx, PermOrderWrite, PermProformaWrite); err != nil {
		return OrderView{}, err
	}
	return s.mutateOrder(ctx, id, ver, audit.ActionUpdate, false, func(ctx context.Context, o *domain.Order) (string, error) {
		if err := o.AttachPaymentProof(fileID, s.Clock.Now()); err != nil {
			return "", err
		}
		return "Bukti bayar diunggah", s.Files.Attach(ctx, fileID, FileOwnerPaymentProof, o.ID)
	})
}

// --- approval diskon ------------------------------------------------------------------------

// OnDiscountApproved melanjutkan pesanan setelah diskon disetujui: Proforma/invoice terbit.
func (s *DocumentService) OnDiscountApproved(ctx context.Context, d approval.Decided) error {
	o, err := s.Store.GetOrder(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	return s.saveOrderChange(ctx, &o, audit.ActionStatusChange, false, func(ctx context.Context, o *domain.Order) (string, error) {
		if err := o.ApproveDiscount(s.Clock.Now()); err != nil {
			return "", err
		}
		return "Diskon disetujui", s.afterConfirm(ctx, o)
	})
}

// OnDiscountRejected mengembalikan pesanan ke Draft dan melepas alokasi stok.
func (s *DocumentService) OnDiscountRejected(ctx context.Context, d approval.Decided) error {
	o, err := s.Store.GetOrder(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	return s.saveOrderChange(ctx, &o, audit.ActionStatusChange, false, func(ctx context.Context, o *domain.Order) (string, error) {
		if err := o.RejectDiscount(); err != nil {
			return "", err
		}
		return "Diskon ditolak, kembali ke Draft", s.release(ctx, o, "reject")
	})
}

// SaleLocations mengembalikan lokasi asal stok yang boleh dipakai jenis pesanan (form pesanan).
func (s *DocumentService) SaleLocations(ctx context.Context, t domain.OrderType) ([]LocationRef, error) {
	if !t.Valid() {
		return nil, apperr.Validation(map[string]string{"orderType": "Jenis pesanan tidak dikenal"})
	}
	if _, err := actor.RequireAny(ctx, readPermission(t), writePermission(t)); err != nil {
		return nil, err
	}
	return s.Locations.SaleLocations(ctx, t.LocationTypes())
}

// ChannelOptions mengembalikan channel & sub-channel aktif untuk form pesanan.
func (s *DocumentService) ChannelOptions(ctx context.Context) ([]ChannelRef, error) {
	if _, err := actor.RequireAny(ctx, PermOrderRead, PermMarketplaceRead, PermInvoiceRead, PermPaymentRead); err != nil {
		return nil, err
	}
	return s.Ledger.Channels(ctx)
}

var errNoMarketplaceChannel = errors.New("channel marketplace belum ada di master dimensi")
