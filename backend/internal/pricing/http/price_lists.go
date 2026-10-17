package http

import (
	"context"

	"balans/internal/pricing/domain"
	"balans/internal/pricing/postgres"
	"balans/internal/pricing/service"
)

func toVersionSummary(v domain.PriceListVersion) PriceListVersionSummary {
	return PriceListVersionSummary{
		Id: v.ID, VersionNo: v.No, Status: PriceListVersionStatus(v.Status), ValidFrom: toDate(v.ValidFrom),
		ValidTo: optDate(v.ValidTo), Percent: optPercent(v.Percent), ItemCount: v.ItemCount, Notes: v.Notes,
		SubmittedAt: v.SubmittedAt, DecidedAt: v.DecidedAt, UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

func optSummary(v *domain.PriceListVersion) *PriceListVersionSummary {
	if v == nil {
		return nil
	}
	s := toVersionSummary(*v)
	return &s
}

func toPriceList(v service.PriceListView) PriceList {
	return PriceList{
		Id: v.ID, Code: v.Code, Name: v.Name, PriceListType: PriceListType(v.Type), ChannelId: v.ChannelID,
		SubChannelId: v.SubChannelID, PartnerId: v.PartnerID, PartnerName: optString(v.PartnerName),
		ResellerLevel: v.ResellerLevel, Basis: PriceListBasis(v.Basis), ReferencePriceListId: v.ReferenceID,
		ReferencePriceListName: optString(v.ReferenceName), Description: v.Description, IsActive: v.IsActive,
		DeletedAt: v.DeletedAt, CurrentVersion: optSummary(v.Current), NextVersion: optSummary(v.Next),
		OpenVersion: optSummary(v.Open), UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

func toPriceListDetail(d service.PriceListDetail) PriceListDetail {
	p := toPriceList(d.PriceListView)
	versions := make([]PriceListVersionSummary, len(d.Versions))
	for i, v := range d.Versions {
		versions[i] = toVersionSummary(v)
	}
	return PriceListDetail{
		Id: p.Id, Code: p.Code, Name: p.Name, PriceListType: p.PriceListType, ChannelId: p.ChannelId,
		SubChannelId: p.SubChannelId, PartnerId: p.PartnerId, PartnerName: p.PartnerName, ResellerLevel: p.ResellerLevel,
		Basis: p.Basis, ReferencePriceListId: p.ReferencePriceListId, ReferencePriceListName: p.ReferencePriceListName,
		Description: p.Description, IsActive: p.IsActive, DeletedAt: p.DeletedAt, CurrentVersion: p.CurrentVersion,
		NextVersion: p.NextVersion, OpenVersion: p.OpenVersion, UpdatedAt: p.UpdatedAt, Version: p.Version,
		Versions: versions, AllowedActions: d.AllowedActions,
	}
}

func priceListInput(f PriceListFields) domain.PriceListInput {
	in := domain.PriceListInput{
		Name: f.Name, ChannelID: f.ChannelId, SubChannelID: f.SubChannelId, PartnerID: f.PartnerId,
		ResellerLevel: str(f.ResellerLevel), ReferenceID: f.ReferencePriceListId, Description: str(f.Description),
		IsActive: f.IsActive,
	}
	if f.Basis != nil {
		in.Basis = domain.Basis(*f.Basis)
	}
	return in
}

// ListPriceLists mengembalikan daftar harga.
func (h *PricingHandler) ListPriceLists(ctx context.Context, req ListPriceListsRequestObject) (ListPriceListsResponseObject, error) {
	f := postgres.PriceListFilter{
		Query: str(req.Params.Q), IncludeInactive: boolValue(req.Params.IncludeInactive), Deleted: boolValue(req.Params.Deleted),
	}
	if req.Params.PriceListType != nil {
		f.Type = domain.PriceListType(*req.Params.PriceListType)
	}
	list, err := h.svc.ListPriceLists(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]PriceList, len(list))
	for i, v := range list {
		out[i] = toPriceList(v)
	}
	return ListPriceLists200JSONResponse(out), nil
}

// ListPriceListOptions mengembalikan pilihan daftar harga aktif.
func (h *PricingHandler) ListPriceListOptions(ctx context.Context, _ ListPriceListOptionsRequestObject) (ListPriceListOptionsResponseObject, error) {
	list, err := h.svc.PriceListOptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PriceListOption, len(list))
	for i, l := range list {
		out[i] = PriceListOption{Id: l.ID, Code: l.Code, Name: l.Name, PriceListType: PriceListType(l.Type), Basis: PriceListBasis(l.Basis)}
	}
	return ListPriceListOptions200JSONResponse(out), nil
}

// CreatePriceList menambah daftar harga.
func (h *PricingHandler) CreatePriceList(ctx context.Context, req CreatePriceListRequestObject) (CreatePriceListResponseObject, error) {
	b := req.Body
	in := priceListInput(PriceListFields{
		Name: b.Name, ChannelId: b.ChannelId, SubChannelId: b.SubChannelId, PartnerId: b.PartnerId, ResellerLevel: b.ResellerLevel,
		Basis: b.Basis, ReferencePriceListId: b.ReferencePriceListId, Description: b.Description, IsActive: b.IsActive,
	})
	in.Code, in.Type = b.Code, domain.PriceListType(b.PriceListType)
	p, err := h.svc.CreatePriceList(ctx, in)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.GetPriceList(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return CreatePriceList201JSONResponse(toPriceListDetail(d)), nil
}

// GetPriceList mengembalikan detail daftar harga.
func (h *PricingHandler) GetPriceList(ctx context.Context, req GetPriceListRequestObject) (GetPriceListResponseObject, error) {
	d, err := h.svc.GetPriceList(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPriceList200JSONResponse(toPriceListDetail(d)), nil
}

// UpdatePriceList mengubah header daftar harga.
func (h *PricingHandler) UpdatePriceList(ctx context.Context, req UpdatePriceListRequestObject) (UpdatePriceListResponseObject, error) {
	b := req.Body
	in := priceListInput(PriceListFields{
		Name: b.Name, ChannelId: b.ChannelId, SubChannelId: b.SubChannelId, PartnerId: b.PartnerId, ResellerLevel: b.ResellerLevel,
		Basis: b.Basis, ReferencePriceListId: b.ReferencePriceListId, Description: b.Description, IsActive: b.IsActive,
	})
	if _, err := h.svc.UpdatePriceList(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	d, err := h.svc.GetPriceList(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return UpdatePriceList200JSONResponse(toPriceListDetail(d)), nil
}

// DeletePriceList menghapus daftar harga.
func (h *PricingHandler) DeletePriceList(ctx context.Context, req DeletePriceListRequestObject) (DeletePriceListResponseObject, error) {
	if err := h.svc.DeletePriceList(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeletePriceList204Response{}, nil
}

// RestorePriceList memulihkan daftar harga.
func (h *PricingHandler) RestorePriceList(ctx context.Context, req RestorePriceListRequestObject) (RestorePriceListResponseObject, error) {
	if _, err := h.svc.RestorePriceList(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	d, err := h.svc.GetPriceList(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return RestorePriceList200JSONResponse(toPriceListDetail(d)), nil
}

func versionInput(f PriceListVersionFields) domain.VersionInput {
	in := domain.VersionInput{
		ValidFrom: fromDate(f.ValidFrom), ValidTo: fromOptDate(f.ValidTo), Percent: str(f.Percent), Notes: str(f.Notes),
		Items: make([]domain.ItemInput, len(f.Items)),
	}
	for i, it := range f.Items {
		in.Items[i] = domain.ItemInput{ProductID: it.ProductId, Price: it.PriceAmount, MaxRetailPrice: str(it.MaxRetailPriceAmount)}
	}
	return in
}

func (h *PricingHandler) versionResponse(ctx context.Context, id Id) (PriceListVersion, error) {
	d, err := h.svc.GetVersion(ctx, id)
	if err != nil {
		return PriceListVersion{}, err
	}
	s := toVersionSummary(d.PriceListVersion)
	items := make([]PriceListItem, len(d.Items))
	for i, it := range d.Items {
		sku, name := productFields(d.Products, it.ProductID)
		items[i] = PriceListItem{
			ProductId: it.ProductID, ProductSku: sku, ProductName: name, PriceAmount: it.Price.String(),
			MaxRetailPriceAmount: optMoney(it.MaxRetailPrice),
		}
	}
	return PriceListVersion{
		Id: s.Id, VersionNo: s.VersionNo, Status: s.Status, ValidFrom: s.ValidFrom, ValidTo: s.ValidTo, Percent: s.Percent,
		ItemCount: s.ItemCount, Notes: s.Notes, SubmittedAt: s.SubmittedAt, DecidedAt: s.DecidedAt, UpdatedAt: s.UpdatedAt,
		Version: s.Version, PriceListId: d.PriceList.ID, PriceListCode: d.PriceList.Code, PriceListName: d.PriceList.Name,
		PriceListType: PriceListType(d.PriceList.Type), Basis: PriceListBasis(d.PriceList.Basis), Items: items,
		AllowedActions: d.AllowedActions,
	}, nil
}

// CreatePriceListVersion menyiapkan versi draft.
func (h *PricingHandler) CreatePriceListVersion(ctx context.Context, req CreatePriceListVersionRequestObject) (CreatePriceListVersionResponseObject, error) {
	b := req.Body
	in := versionInput(PriceListVersionFields{ValidFrom: b.ValidFrom, ValidTo: b.ValidTo, Percent: b.Percent, Notes: b.Notes, Items: b.Items})
	v, err := h.svc.CreateVersion(ctx, req.Id, in, b.CopyFromVersionId)
	if err != nil {
		return nil, err
	}
	out, err := h.versionResponse(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	return CreatePriceListVersion201JSONResponse(out), nil
}

// GetPriceListVersion mengembalikan detail versi.
func (h *PricingHandler) GetPriceListVersion(ctx context.Context, req GetPriceListVersionRequestObject) (GetPriceListVersionResponseObject, error) {
	out, err := h.versionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPriceListVersion200JSONResponse(out), nil
}

// UpdatePriceListVersion mengubah versi draft.
func (h *PricingHandler) UpdatePriceListVersion(ctx context.Context, req UpdatePriceListVersionRequestObject) (UpdatePriceListVersionResponseObject, error) {
	b := req.Body
	in := versionInput(PriceListVersionFields{ValidFrom: b.ValidFrom, ValidTo: b.ValidTo, Percent: b.Percent, Notes: b.Notes, Items: b.Items})
	if _, err := h.svc.UpdateVersion(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.versionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return UpdatePriceListVersion200JSONResponse(out), nil
}

// SubmitPriceListVersion mengajukan versi.
func (h *PricingHandler) SubmitPriceListVersion(ctx context.Context, req SubmitPriceListVersionRequestObject) (SubmitPriceListVersionResponseObject, error) {
	if _, err := h.svc.SubmitVersion(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	out, err := h.versionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return SubmitPriceListVersion200JSONResponse(out), nil
}

// WithdrawPriceListVersion menarik kembali pengajuan.
func (h *PricingHandler) WithdrawPriceListVersion(ctx context.Context, req WithdrawPriceListVersionRequestObject) (WithdrawPriceListVersionResponseObject, error) {
	if _, err := h.svc.WithdrawVersion(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	out, err := h.versionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return WithdrawPriceListVersion200JSONResponse(out), nil
}

// CancelPriceListVersion membatalkan versi draft.
func (h *PricingHandler) CancelPriceListVersion(ctx context.Context, req CancelPriceListVersionRequestObject) (CancelPriceListVersionResponseObject, error) {
	if _, err := h.svc.CancelVersion(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	out, err := h.versionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return CancelPriceListVersion200JSONResponse(out), nil
}
