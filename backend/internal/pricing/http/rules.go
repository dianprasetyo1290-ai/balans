package http

import (
	"context"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
	"balans/internal/pricing/service"
)

// --- Harga khusus ----------------------------------------------------------------------------

func toSpecialPrice(v service.SpecialPriceView) SpecialPrice {
	return SpecialPrice{
		Id: v.ID, PartyType: SpecialPricePartyType(v.PartyType), PartyId: v.PartyID, PartyCode: v.PartyCode,
		PartyName: v.PartyName, ProductId: v.ProductID, ProductSku: v.Product.SKU, ProductName: v.Product.Name,
		PriceAmount: v.Price.String(), ValidFrom: toDate(v.ValidFrom), ValidTo: optDate(v.ValidTo),
		Status: PricingPeriodStatus(v.Status), Notes: v.Notes, UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

func (h *PricingHandler) specialPrice(ctx context.Context, sp domain.SpecialPrice) (SpecialPrice, error) {
	pid, ptype := sp.PartyID, sp.PartyType
	list, err := h.svc.ListSpecialPrices(ctx, postgres.SpecialPriceFilter{
		PartyType: ptype, PartyID: &pid, ProductID: &sp.ProductID, IncludeEnded: true,
	})
	if err != nil {
		return SpecialPrice{}, err
	}
	for _, v := range list {
		if v.ID == sp.ID {
			return toSpecialPrice(v), nil
		}
	}
	return toSpecialPrice(service.SpecialPriceView{SpecialPrice: sp}), nil
}

// ListSpecialPrices mengembalikan harga khusus.
func (h *PricingHandler) ListSpecialPrices(ctx context.Context, req ListSpecialPricesRequestObject) (ListSpecialPricesResponseObject, error) {
	f := postgres.SpecialPriceFilter{PartyID: req.Params.PartyId, ProductID: req.Params.ProductId, IncludeEnded: boolValue(req.Params.IncludeEnded)}
	if req.Params.PartyType != nil {
		f.PartyType = domain.PartyType(*req.Params.PartyType)
	}
	list, err := h.svc.ListSpecialPrices(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]SpecialPrice, len(list))
	for i, v := range list {
		out[i] = toSpecialPrice(v)
	}
	return ListSpecialPrices200JSONResponse(out), nil
}

// CreateSpecialPrice menambah harga khusus.
func (h *PricingHandler) CreateSpecialPrice(ctx context.Context, req CreateSpecialPriceRequestObject) (CreateSpecialPriceResponseObject, error) {
	b := req.Body
	sp, err := h.svc.CreateSpecialPrice(ctx, domain.SpecialPriceInput{
		PartyType: domain.PartyType(b.PartyType), PartyID: b.PartyId, ProductID: b.ProductId, Price: b.PriceAmount,
		ValidFrom: fromDate(b.ValidFrom), ValidTo: fromOptDate(b.ValidTo), Notes: str(b.Notes),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.specialPrice(ctx, sp)
	if err != nil {
		return nil, err
	}
	return CreateSpecialPrice201JSONResponse(out), nil
}

// EndSpecialPrice mengakhiri harga khusus.
func (h *PricingHandler) EndSpecialPrice(ctx context.Context, req EndSpecialPriceRequestObject) (EndSpecialPriceResponseObject, error) {
	sp, err := h.svc.EndSpecialPrice(ctx, req.Id, req.Body.Version, fromDate(req.Body.ValidTo))
	if err != nil {
		return nil, err
	}
	out, err := h.specialPrice(ctx, sp)
	if err != nil {
		return nil, err
	}
	return EndSpecialPrice200JSONResponse(out), nil
}

// DeleteSpecialPrice menghapus harga khusus.
func (h *PricingHandler) DeleteSpecialPrice(ctx context.Context, req DeleteSpecialPriceRequestObject) (DeleteSpecialPriceResponseObject, error) {
	if err := h.svc.DeleteSpecialPrice(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteSpecialPrice204Response{}, nil
}

// --- Promo -----------------------------------------------------------------------------------

func toPromotion(v service.PromotionView, withItems bool) Promotion {
	out := Promotion{
		Id: v.ID, Code: v.Code, Name: v.Name, ChannelId: v.ChannelID, SubChannelId: v.SubChannelID,
		ValidFrom: toDate(v.ValidFrom), ValidTo: toDate(v.ValidTo), IsActive: v.IsActive, Description: v.Description,
		Status: PricingPeriodStatus(v.Status), ItemCount: v.ItemCount, AllowedActions: v.AllowedActions,
		UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
	if withItems {
		items := make([]PromotionItem, len(v.Items))
		for i, it := range v.Items {
			sku, name := productFields(v.Products, it.ProductID)
			items[i] = PromotionItem{
				ProductId: it.ProductID, ProductSku: sku, ProductName: name, PriceAmount: optMoney(it.Price),
				DiscountPercent: optPercent(it.DiscountPercent),
			}
		}
		out.Items = &items
	}
	return out
}

func promotionInput(f PromotionFields) domain.PromotionInput {
	in := domain.PromotionInput{
		Name: f.Name, ChannelID: f.ChannelId, SubChannelID: f.SubChannelId, ValidFrom: fromDate(f.ValidFrom),
		ValidTo: fromDate(f.ValidTo), IsActive: f.IsActive, Description: str(f.Description),
		Items: make([]domain.PromotionItemInput, len(f.Items)),
	}
	for i, it := range f.Items {
		in.Items[i] = domain.PromotionItemInput{ProductID: it.ProductId, Price: str(it.PriceAmount), DiscountPercent: str(it.DiscountPercent)}
	}
	return in
}

// ListPromotions mengembalikan promo.
func (h *PricingHandler) ListPromotions(ctx context.Context, req ListPromotionsRequestObject) (ListPromotionsResponseObject, error) {
	list, err := h.svc.ListPromotions(ctx, postgres.PromotionFilter{
		ChannelID: req.Params.ChannelId, Query: str(req.Params.Q), IncludeEnded: boolValue(req.Params.IncludeEnded),
		Deleted: boolValue(req.Params.Deleted),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Promotion, len(list))
	for i, v := range list {
		out[i] = toPromotion(v, false)
	}
	return ListPromotions200JSONResponse(out), nil
}

// GetPromotion mengembalikan detail promo.
func (h *PricingHandler) GetPromotion(ctx context.Context, req GetPromotionRequestObject) (GetPromotionResponseObject, error) {
	v, err := h.svc.GetPromotion(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPromotion200JSONResponse(toPromotion(v, true)), nil
}

// CreatePromotion menambah promo.
func (h *PricingHandler) CreatePromotion(ctx context.Context, req CreatePromotionRequestObject) (CreatePromotionResponseObject, error) {
	b := req.Body
	in := promotionInput(PromotionFields{
		Name: b.Name, ChannelId: b.ChannelId, SubChannelId: b.SubChannelId, ValidFrom: b.ValidFrom, ValidTo: b.ValidTo,
		IsActive: b.IsActive, Description: b.Description, Items: b.Items,
	})
	in.Code = b.Code
	p, err := h.svc.CreatePromotion(ctx, in)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.GetPromotion(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return CreatePromotion201JSONResponse(toPromotion(v, true)), nil
}

// UpdatePromotion mengubah promo.
func (h *PricingHandler) UpdatePromotion(ctx context.Context, req UpdatePromotionRequestObject) (UpdatePromotionResponseObject, error) {
	b := req.Body
	in := promotionInput(PromotionFields{
		Name: b.Name, ChannelId: b.ChannelId, SubChannelId: b.SubChannelId, ValidFrom: b.ValidFrom, ValidTo: b.ValidTo,
		IsActive: b.IsActive, Description: b.Description, Items: b.Items,
	})
	if len(b.Items) == 0 {
		in.Items = nil
	}
	if _, err := h.svc.UpdatePromotion(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	v, err := h.svc.GetPromotion(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return UpdatePromotion200JSONResponse(toPromotion(v, true)), nil
}

// DeletePromotion menghapus promo.
func (h *PricingHandler) DeletePromotion(ctx context.Context, req DeletePromotionRequestObject) (DeletePromotionResponseObject, error) {
	if err := h.svc.DeletePromotion(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeletePromotion204Response{}, nil
}

// --- Harga bertingkat ------------------------------------------------------------------------

func toTieredPrice(v service.TieredPriceView) TieredPrice {
	return TieredPrice{
		Id: v.ID, PriceListId: v.PriceListID, PriceListCode: optString(v.PriceListCode), PriceListName: optString(v.PriceListName),
		ProductId: v.ProductID, ProductSku: v.Product.SKU, ProductName: v.Product.Name, MinQty: v.MinQty.StringFixed(3),
		PriceAmount: v.Price.String(), ValidFrom: toDate(v.ValidFrom), ValidTo: optDate(v.ValidTo),
		Status: PricingPeriodStatus(v.Status), Notes: v.Notes, UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

func (h *PricingHandler) tieredPrice(ctx context.Context, t domain.TieredPrice) (TieredPrice, error) {
	list, err := h.svc.ListTieredPrices(ctx, postgres.TieredPriceFilter{ProductID: &t.ProductID, PriceListID: t.PriceListID, IncludeEnded: true})
	if err != nil {
		return TieredPrice{}, err
	}
	for _, v := range list {
		if v.ID == t.ID {
			return toTieredPrice(v), nil
		}
	}
	return toTieredPrice(service.TieredPriceView{TieredPrice: t}), nil
}

// ListTieredPrices mengembalikan harga bertingkat.
func (h *PricingHandler) ListTieredPrices(ctx context.Context, req ListTieredPricesRequestObject) (ListTieredPricesResponseObject, error) {
	list, err := h.svc.ListTieredPrices(ctx, postgres.TieredPriceFilter{
		ProductID: req.Params.ProductId, PriceListID: req.Params.PriceListId, IncludeEnded: boolValue(req.Params.IncludeEnded),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TieredPrice, len(list))
	for i, v := range list {
		out[i] = toTieredPrice(v)
	}
	return ListTieredPrices200JSONResponse(out), nil
}

// CreateTieredPrice menambah harga bertingkat.
func (h *PricingHandler) CreateTieredPrice(ctx context.Context, req CreateTieredPriceRequestObject) (CreateTieredPriceResponseObject, error) {
	b := req.Body
	t, err := h.svc.CreateTieredPrice(ctx, domain.TieredPriceInput{
		PriceListID: b.PriceListId, ProductID: b.ProductId, MinQty: b.MinQty, Price: b.PriceAmount,
		ValidFrom: fromDate(b.ValidFrom), ValidTo: fromOptDate(b.ValidTo), Notes: str(b.Notes),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.tieredPrice(ctx, t)
	if err != nil {
		return nil, err
	}
	return CreateTieredPrice201JSONResponse(out), nil
}

// EndTieredPrice mengakhiri harga bertingkat.
func (h *PricingHandler) EndTieredPrice(ctx context.Context, req EndTieredPriceRequestObject) (EndTieredPriceResponseObject, error) {
	t, err := h.svc.EndTieredPrice(ctx, req.Id, req.Body.Version, fromDate(req.Body.ValidTo))
	if err != nil {
		return nil, err
	}
	out, err := h.tieredPrice(ctx, t)
	if err != nil {
		return nil, err
	}
	return EndTieredPrice200JSONResponse(out), nil
}

// DeleteTieredPrice menghapus harga bertingkat.
func (h *PricingHandler) DeleteTieredPrice(ctx context.Context, req DeleteTieredPriceRequestObject) (DeleteTieredPriceResponseObject, error) {
	if err := h.svc.DeleteTieredPrice(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteTieredPrice204Response{}, nil
}

// --- Batas diskon ----------------------------------------------------------------------------

func toDiscountLimit(v service.DiscountLimitView) DiscountLimit {
	return DiscountLimit{
		Id: v.ID, RoleCode: v.RoleCode, RoleName: v.RoleName, MaxLineDiscountPercent: percentText(v.MaxLinePercent),
		MaxDocumentDiscountPercent: percentText(v.MaxDocumentPercent), ValidFrom: toDate(v.ValidFrom), ValidTo: optDate(v.ValidTo),
		Status: PricingPeriodStatus(v.Status), Notes: v.Notes, UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

func (h *PricingHandler) discountLimit(ctx context.Context, l domain.DiscountLimit) (DiscountLimit, error) {
	list, err := h.svc.ListDiscountLimits(ctx, true)
	if err != nil {
		return DiscountLimit{}, err
	}
	for _, v := range list {
		if v.ID == l.ID {
			return toDiscountLimit(v), nil
		}
	}
	return toDiscountLimit(service.DiscountLimitView{DiscountLimit: l, RoleName: l.RoleCode}), nil
}

// ListDiscountLimits mengembalikan batas diskon.
func (h *PricingHandler) ListDiscountLimits(ctx context.Context, req ListDiscountLimitsRequestObject) (ListDiscountLimitsResponseObject, error) {
	list, err := h.svc.ListDiscountLimits(ctx, boolValue(req.Params.IncludeEnded))
	if err != nil {
		return nil, err
	}
	out := make([]DiscountLimit, len(list))
	for i, v := range list {
		out[i] = toDiscountLimit(v)
	}
	return ListDiscountLimits200JSONResponse(out), nil
}

// ListDiscountLimitRoles mengembalikan pilihan role.
func (h *PricingHandler) ListDiscountLimitRoles(ctx context.Context, _ ListDiscountLimitRolesRequestObject) (ListDiscountLimitRolesResponseObject, error) {
	roles, err := h.svc.DiscountRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DiscountLimitRole, len(roles))
	for i, r := range roles {
		out[i] = DiscountLimitRole{Code: r.Code, Name: r.Name}
	}
	return ListDiscountLimitRoles200JSONResponse(out), nil
}

// CreateDiscountLimit menetapkan batas diskon.
func (h *PricingHandler) CreateDiscountLimit(ctx context.Context, req CreateDiscountLimitRequestObject) (CreateDiscountLimitResponseObject, error) {
	b := req.Body
	l, err := h.svc.CreateDiscountLimit(ctx, domain.DiscountLimitInput{
		RoleCode: b.RoleCode, MaxLinePercent: b.MaxLineDiscountPercent, MaxDocumentPercent: b.MaxDocumentDiscountPercent,
		ValidFrom: fromDate(b.ValidFrom), ValidTo: fromOptDate(b.ValidTo), Notes: str(b.Notes),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.discountLimit(ctx, l)
	if err != nil {
		return nil, err
	}
	return CreateDiscountLimit201JSONResponse(out), nil
}

// EndDiscountLimit mengakhiri batas diskon.
func (h *PricingHandler) EndDiscountLimit(ctx context.Context, req EndDiscountLimitRequestObject) (EndDiscountLimitResponseObject, error) {
	l, err := h.svc.EndDiscountLimit(ctx, req.Id, req.Body.Version, fromDate(req.Body.ValidTo))
	if err != nil {
		return nil, err
	}
	out, err := h.discountLimit(ctx, l)
	if err != nil {
		return nil, err
	}
	return EndDiscountLimit200JSONResponse(out), nil
}

// DeleteDiscountLimit menghapus batas diskon.
func (h *PricingHandler) DeleteDiscountLimit(ctx context.Context, req DeleteDiscountLimitRequestObject) (DeleteDiscountLimitResponseObject, error) {
	if err := h.svc.DeleteDiscountLimit(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteDiscountLimit204Response{}, nil
}

// --- Simulasi --------------------------------------------------------------------------------

func toQuote(q domain.PriceQuote) PriceQuote {
	return PriceQuote{
		UnitPrice: q.UnitPrice.String(), Rule: PriceRule(q.Rule), SourceId: q.SourceID, PriceListId: q.PriceListID,
		PriceListVersionId: q.PriceListVersionID, PromotionId: q.PromotionID, SpecialPriceId: q.SpecialPriceID,
		TieredPriceId: q.TieredPriceID, RegularPrice: optMoney(q.RegularPrice), MaxRetailPrice: optMoney(q.MaxRetailPrice),
		Description: q.Description,
	}
}

// QuotePrice menjalankan resolver harga.
func (h *PricingHandler) QuotePrice(ctx context.Context, req QuotePriceRequestObject) (QuotePriceResponseObject, error) {
	b := req.Body
	qty, err := parseDecimal("qty", b.Qty)
	if err != nil {
		return nil, err
	}
	q, err := h.sim.QuotePrice(ctx, domain.PriceRequest{
		ProductID: b.ProductId, Qty: qty, ChannelID: b.ChannelId, SubChannelID: b.SubChannelId, CustomerID: b.CustomerId,
		PartnerID: b.PartnerId, Date: fromDate(b.Date),
	})
	if err != nil {
		return nil, err
	}
	return QuotePrice200JSONResponse(toQuote(q)), nil
}

func discountValue(field string, v *DiscountValue) (domain.Discount, error) {
	if v == nil {
		return domain.Discount{}, nil
	}
	d, err := parseDecimal(field, v.Value)
	if err != nil {
		return domain.Discount{}, err
	}
	return domain.Discount{Kind: domain.DiscountKind(v.Kind), Value: d}, nil
}

func toOutcome(r domain.DiscountResult) DiscountCheckOutcome {
	return DiscountCheckOutcome{DiscountAmount: r.Amount.String(), DiscountPercent: percentText(r.Percent), ExceedsLimit: r.ExceedsLimit}
}

// CheckDiscount memeriksa diskon manual terhadap batas role user yang login.
func (h *PricingHandler) CheckDiscount(ctx context.Context, req CheckDiscountRequestObject) (CheckDiscountResponseObject, error) {
	b := req.Body
	lines := make([]domain.LineDiscount, len(b.Lines))
	for i, l := range b.Lines {
		gross, err := money.Parse(l.GrossAmount)
		if err != nil {
			return nil, apperr.Validation(map[string]string{"lines." + itoa(i) + ".grossAmount": "Nominal tidak valid"})
		}
		d, err := discountValue("lines."+itoa(i)+".discount", l.Discount)
		if err != nil {
			return nil, err
		}
		lines[i] = domain.LineDiscount{Gross: gross, Discount: d}
	}
	doc, err := discountValue("documentDiscount", b.DocumentDiscount)
	if err != nil {
		return nil, err
	}
	check, subtotal, err := h.sim.CheckDiscountForActor(ctx, lines, doc)
	if err != nil {
		return nil, err
	}
	out := DiscountCheckResult{
		MaxLineDiscountPercent: percentText(check.Limit.MaxLinePercent), MaxDocumentDiscountPercent: percentText(check.Limit.MaxDocumentPercent),
		Lines: make([]DiscountCheckOutcome, len(check.Lines)), Document: toOutcome(check.Document), SubtotalAmount: subtotal.String(),
		TotalDiscountAmount: check.TotalDiscountAmount.String(), RequiresApproval: check.RequiresApproval,
	}
	for i, l := range check.Lines {
		out.Lines[i] = toOutcome(l)
	}
	return CheckDiscount200JSONResponse(out), nil
}
