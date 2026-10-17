package http

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/party/domain"
	"balans/internal/party/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/party.yaml ../../../api/openapi.bundle.yaml

// PartyHandler mengimplementasikan StrictServerInterface hasil generate.
type PartyHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*PartyHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *PartyHandler { return &PartyHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *PartyHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// --- Helper ------------------------------------------------------------------------------------

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func decimalString(d *decimal.Decimal, places int32) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(places)
	return &s
}

func masked(v string) *string {
	if v == "" {
		return nil
	}
	m := pii.Mask(v)
	return &m
}

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func sortOf(s *Sort, allowed []string, fallback paging.Sort) paging.Sort {
	var raw *string
	if s != nil {
		v := *s
		raw = &v
	}
	return paging.ParseSort(raw, allowed, fallback)
}

func pageOf(p *Page, size *PageSize) paging.Page {
	var n, sz *int
	if p != nil {
		v := *p
		n = &v
	}
	if size != nil {
		v := *size
		sz = &v
	}
	return paging.New(n, sz)
}

func ids(list ...*uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(list))
	for _, id := range list {
		if id != nil {
			out = append(out, *id)
		}
	}
	return out
}

// --- Pelanggan ---------------------------------------------------------------------------------

func customerInput(b CustomerInput) domain.CustomerInput {
	in := domain.CustomerInput{
		FullName: b.FullName, Phone: b.Phone, Email: str(b.Email), Type: domain.CustomerType(b.CustomerType),
		EmployeeID: b.EmployeeId, MarketingConsent: b.MarketingConsent, MarketingConsentVersion: str(b.MarketingConsentVersion),
		Notes: str(b.Notes), Addresses: make([]domain.AddressInput, len(b.Addresses)),
	}
	for i, a := range b.Addresses {
		in.Addresses[i] = domain.AddressInput{
			Label: str(a.Label), RecipientName: a.RecipientName, Phone: str(a.Phone), AddressLine: a.AddressLine,
			District: str(a.District), City: a.City, Province: str(a.Province), PostalCode: str(a.PostalCode),
			Latitude: a.Latitude, Longitude: a.Longitude, IsDefault: a.IsDefault,
		}
		if a.Id != nil {
			in.Addresses[i].ID = *a.Id
		}
	}
	return in
}

func toCustomer(c domain.Customer, employees map[uuid.UUID]service.EmployeeRef) Customer {
	out := Customer{
		Id: c.ID, Code: c.Code, FullName: c.FullName, Phone: c.Phone, Email: c.Email,
		CustomerType: CustomerType(c.Type), EmployeeId: c.EmployeeID, MarketingConsent: c.HasMarketingConsent(),
		MarketingConsentAt: c.MarketingConsentAt, MarketingConsentVersion: c.MarketingConsentVersion, Notes: c.Notes,
		Addresses: make([]CustomerAddress, len(c.Addresses)), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Version: c.Version,
	}
	if c.EmployeeID != nil {
		if e, ok := employees[*c.EmployeeID]; ok {
			out.EmployeeName, out.EmployeeNumber, out.IsEmployeeActive = &e.Name, &e.Number, &e.IsActive
		} else {
			inactive := false
			out.IsEmployeeActive = &inactive
		}
	}
	for i, a := range c.Addresses {
		out.Addresses[i] = CustomerAddress{
			Id: a.ID, Label: a.Label, RecipientName: a.RecipientName, Phone: a.Phone, AddressLine: a.AddressLine,
			District: a.District, City: a.City, Province: a.Province, PostalCode: a.PostalCode,
			Latitude: decimalString(a.Latitude, 6), Longitude: decimalString(a.Longitude, 6), IsDefault: a.IsDefault,
		}
	}
	return out
}

func (h *PartyHandler) customers(ctx context.Context, list []domain.Customer) ([]Customer, error) {
	var refs []uuid.UUID
	for _, c := range list {
		refs = append(refs, ids(c.EmployeeID)...)
	}
	employees, err := h.svc.EmployeeRefs(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := make([]Customer, len(list))
	for i, c := range list {
		out[i] = toCustomer(c, employees)
	}
	return out, nil
}

func (h *PartyHandler) customer(ctx context.Context, c domain.Customer) (Customer, error) {
	out, err := h.customers(ctx, []domain.Customer{c})
	if err != nil {
		return Customer{}, err
	}
	return out[0], nil
}

// ListCustomers mengembalikan daftar pelanggan.
func (h *PartyHandler) ListCustomers(ctx context.Context, req ListCustomersRequestObject) (ListCustomersResponseObject, error) {
	p := req.Params
	q := service.CustomerQuery{
		Search: str(p.Q), Phone: str(p.Phone),
		Sort: sortOf(p.Sort, []string{"code", "fullName", "createdAt"}, paging.Sort{Field: "createdAt", Desc: true}),
	}
	if p.CustomerType != nil {
		q.Type = domain.CustomerType(*p.CustomerType)
	}
	res, err := h.svc.ListCustomers(ctx, q, pageOf(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items, err := h.customers(ctx, res.Items)
	if err != nil {
		return nil, err
	}
	return ListCustomers200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateCustomer membuat pelanggan.
func (h *PartyHandler) CreateCustomer(ctx context.Context, req CreateCustomerRequestObject) (CreateCustomerResponseObject, error) {
	c, err := h.svc.CreateCustomer(ctx, customerInput(*req.Body))
	if err != nil {
		return nil, err
	}
	out, err := h.customer(ctx, c)
	if err != nil {
		return nil, err
	}
	return CreateCustomer201JSONResponse(out), nil
}

// GetCustomer mengembalikan detail pelanggan.
func (h *PartyHandler) GetCustomer(ctx context.Context, req GetCustomerRequestObject) (GetCustomerResponseObject, error) {
	c, err := h.svc.GetCustomer(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.customer(ctx, c)
	if err != nil {
		return nil, err
	}
	return GetCustomer200JSONResponse(out), nil
}

// UpdateCustomer mengubah pelanggan.
func (h *PartyHandler) UpdateCustomer(ctx context.Context, req UpdateCustomerRequestObject) (UpdateCustomerResponseObject, error) {
	b := req.Body
	c, err := h.svc.UpdateCustomer(ctx, req.Id, b.Version, customerInput(CustomerInput{
		Addresses: b.Addresses, CustomerType: b.CustomerType, Email: b.Email, EmployeeId: b.EmployeeId,
		FullName: b.FullName, MarketingConsent: b.MarketingConsent, MarketingConsentVersion: b.MarketingConsentVersion,
		Notes: b.Notes, Phone: b.Phone,
	}))
	if err != nil {
		return nil, err
	}
	out, err := h.customer(ctx, c)
	if err != nil {
		return nil, err
	}
	return UpdateCustomer200JSONResponse(out), nil
}

// DeleteCustomer menghapus pelanggan.
func (h *PartyHandler) DeleteCustomer(ctx context.Context, req DeleteCustomerRequestObject) (DeleteCustomerResponseObject, error) {
	if err := h.svc.DeleteCustomer(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteCustomer204Response{}, nil
}

// --- Mitra & PKS -------------------------------------------------------------------------------

func partnerInput(b PartnerInput) domain.PartnerInput {
	return domain.PartnerInput{
		Code: b.Code, Name: b.Name, Type: domain.PartnerType(b.PartnerType), NPWP: b.Npwp, Address: str(b.Address),
		PICName: str(b.PicName), PICPhone: str(b.PicPhone), SalesUserID: b.SalesUserId,
		MerchandiserUserID: b.MerchandiserUserId, Notes: str(b.Notes),
	}
}

func toAgreement(a domain.Agreement, today clock.BusinessDate, files map[uuid.UUID]string) PartnerAgreement {
	out := PartnerAgreement{
		Id: a.ID, PartnerId: a.PartnerID, AgreementNumber: a.Number, AgreementType: AgreementType(a.Type),
		ValidFrom: toDate(a.ValidFrom), ValidTo: toDate(a.ValidTo), Status: AgreementStatus(a.StatusOn(today)),
		FileId: a.FileID, PriceListId: a.PriceListID, ConsignmentMarginPercent: decimalString(a.MarginPercent, 4),
		PaymentTermDays: a.PaymentTermDays, LossBorneBy: BorneBy(a.LossBorneBy),
		PartnerDamageBorneBy: BorneBy(a.PartnerDamageBorneBy), ProductDefectBorneBy: BorneBy(a.ProductDefectBorneBy),
		NearExpiryBorneBy: BorneBy(a.NearExpiryBorneBy), Notes: a.Notes, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		Version: a.Version,
	}
	if a.FileID != nil {
		if name, ok := files[*a.FileID]; ok {
			out.FileName = &name
		}
	}
	return out
}

func (h *PartyHandler) agreements(ctx context.Context, list []domain.Agreement) ([]PartnerAgreement, error) {
	var fileIDs []uuid.UUID
	for _, a := range list {
		fileIDs = append(fileIDs, ids(a.FileID)...)
	}
	files, err := h.svc.FileNames(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	today := h.svc.Today()
	out := make([]PartnerAgreement, len(list))
	for i, a := range list {
		out[i] = toAgreement(a, today, files)
	}
	return out, nil
}

func (h *PartyHandler) partners(ctx context.Context, list []domain.Partner) ([]Partner, error) {
	var userIDs, partnerIDs []uuid.UUID
	for _, p := range list {
		userIDs = append(userIDs, ids(p.SalesUserID, p.MerchandiserUserID)...)
		partnerIDs = append(partnerIDs, p.ID)
	}
	users, err := h.svc.UserRefs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	active, err := h.svc.ActiveAgreementsByPartner(ctx, partnerIDs)
	if err != nil {
		return nil, err
	}
	name := func(id *uuid.UUID) *string {
		if id == nil {
			return nil
		}
		if u, ok := users[*id]; ok {
			return &u.Name
		}
		n := "Pengguna tidak aktif"
		return &n
	}
	out := make([]Partner, len(list))
	for i, p := range list {
		agreements, err := h.agreements(ctx, active[p.ID])
		if err != nil {
			return nil, err
		}
		out[i] = Partner{
			Id: p.ID, Code: p.Code, Name: p.Name, PartnerType: PartnerType(p.Type), HasNpwp: p.NPWP != "",
			NpwpMasked: masked(p.NPWP), Address: p.Address, PicName: p.PICName, PicPhone: p.PICPhone,
			SalesUserId: p.SalesUserID, SalesUserName: name(p.SalesUserID), MerchandiserUserId: p.MerchandiserUserID,
			MerchandiserUserName: name(p.MerchandiserUserID), Notes: p.Notes, ActiveAgreements: &agreements,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version,
		}
	}
	return out, nil
}

func (h *PartyHandler) partner(ctx context.Context, p domain.Partner) (Partner, error) {
	out, err := h.partners(ctx, []domain.Partner{p})
	if err != nil {
		return Partner{}, err
	}
	return out[0], nil
}

// ListPartners mengembalikan daftar mitra.
func (h *PartyHandler) ListPartners(ctx context.Context, req ListPartnersRequestObject) (ListPartnersResponseObject, error) {
	p := req.Params
	q := service.PartnerQuery{
		Search: str(p.Q), Sort: sortOf(p.Sort, []string{"code", "name", "createdAt"}, paging.Sort{Field: "code"}),
	}
	if p.PartnerType != nil {
		q.Type = domain.PartnerType(*p.PartnerType)
	}
	res, err := h.svc.ListPartners(ctx, q, pageOf(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items, err := h.partners(ctx, res.Items)
	if err != nil {
		return nil, err
	}
	return ListPartners200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreatePartner membuat mitra.
func (h *PartyHandler) CreatePartner(ctx context.Context, req CreatePartnerRequestObject) (CreatePartnerResponseObject, error) {
	p, err := h.svc.CreatePartner(ctx, partnerInput(*req.Body))
	if err != nil {
		return nil, err
	}
	out, err := h.partner(ctx, p)
	if err != nil {
		return nil, err
	}
	return CreatePartner201JSONResponse(out), nil
}

// GetPartner mengembalikan detail mitra.
func (h *PartyHandler) GetPartner(ctx context.Context, req GetPartnerRequestObject) (GetPartnerResponseObject, error) {
	p, err := h.svc.GetPartner(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.partner(ctx, p)
	if err != nil {
		return nil, err
	}
	return GetPartner200JSONResponse(out), nil
}

// UpdatePartner mengubah mitra.
func (h *PartyHandler) UpdatePartner(ctx context.Context, req UpdatePartnerRequestObject) (UpdatePartnerResponseObject, error) {
	b := req.Body
	p, err := h.svc.UpdatePartner(ctx, req.Id, b.Version, partnerInput(PartnerInput{
		Address: b.Address, Code: b.Code, MerchandiserUserId: b.MerchandiserUserId, Name: b.Name, Notes: b.Notes,
		Npwp: b.Npwp, PartnerType: b.PartnerType, PicName: b.PicName, PicPhone: b.PicPhone, SalesUserId: b.SalesUserId,
	}))
	if err != nil {
		return nil, err
	}
	out, err := h.partner(ctx, p)
	if err != nil {
		return nil, err
	}
	return UpdatePartner200JSONResponse(out), nil
}

// DeletePartner menghapus mitra.
func (h *PartyHandler) DeletePartner(ctx context.Context, req DeletePartnerRequestObject) (DeletePartnerResponseObject, error) {
	if err := h.svc.DeletePartner(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeletePartner204Response{}, nil
}

func agreementInput(b PartnerAgreementInput) domain.AgreementInput {
	in := domain.AgreementInput{
		Number: b.AgreementNumber, Type: domain.AgreementType(b.AgreementType), ValidFrom: fromDate(b.ValidFrom),
		ValidTo: fromDate(b.ValidTo), FileID: b.FileId, PriceListID: b.PriceListId, MarginPercent: b.ConsignmentMarginPercent,
		PaymentTermDays: b.PaymentTermDays, Notes: str(b.Notes),
	}
	borne := func(v *BorneBy) domain.BorneBy {
		if v == nil {
			return ""
		}
		return domain.BorneBy(*v)
	}
	in.LossBorneBy, in.PartnerDamageBorneBy = borne(b.LossBorneBy), borne(b.PartnerDamageBorneBy)
	in.ProductDefectBorneBy, in.NearExpiryBorneBy = borne(b.ProductDefectBorneBy), borne(b.NearExpiryBorneBy)
	return in
}

func (h *PartyHandler) agreement(ctx context.Context, a domain.Agreement) (PartnerAgreement, error) {
	out, err := h.agreements(ctx, []domain.Agreement{a})
	if err != nil {
		return PartnerAgreement{}, err
	}
	return out[0], nil
}

// ListPartnerAgreements mengembalikan riwayat PKS.
func (h *PartyHandler) ListPartnerAgreements(ctx context.Context, req ListPartnerAgreementsRequestObject) (ListPartnerAgreementsResponseObject, error) {
	list, err := h.svc.ListAgreements(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.agreements(ctx, list)
	if err != nil {
		return nil, err
	}
	return ListPartnerAgreements200JSONResponse(out), nil
}

// CreatePartnerAgreement mencatat PKS.
func (h *PartyHandler) CreatePartnerAgreement(ctx context.Context, req CreatePartnerAgreementRequestObject) (CreatePartnerAgreementResponseObject, error) {
	a, err := h.svc.CreateAgreement(ctx, req.Id, agreementInput(*req.Body))
	if err != nil {
		return nil, err
	}
	out, err := h.agreement(ctx, a)
	if err != nil {
		return nil, err
	}
	return CreatePartnerAgreement201JSONResponse(out), nil
}

// UpdatePartnerAgreement mengoreksi PKS.
func (h *PartyHandler) UpdatePartnerAgreement(ctx context.Context, req UpdatePartnerAgreementRequestObject) (UpdatePartnerAgreementResponseObject, error) {
	b := req.Body
	a, err := h.svc.UpdateAgreement(ctx, req.Id, b.Version, agreementInput(PartnerAgreementInput{
		AgreementNumber: b.AgreementNumber, AgreementType: b.AgreementType, ConsignmentMarginPercent: b.ConsignmentMarginPercent,
		FileId: b.FileId, LossBorneBy: b.LossBorneBy, NearExpiryBorneBy: b.NearExpiryBorneBy, Notes: b.Notes,
		PartnerDamageBorneBy: b.PartnerDamageBorneBy, PaymentTermDays: b.PaymentTermDays, PriceListId: b.PriceListId,
		ProductDefectBorneBy: b.ProductDefectBorneBy, ValidFrom: b.ValidFrom, ValidTo: b.ValidTo,
	}))
	if err != nil {
		return nil, err
	}
	out, err := h.agreement(ctx, a)
	if err != nil {
		return nil, err
	}
	return UpdatePartnerAgreement200JSONResponse(out), nil
}

// DeletePartnerAgreement menghapus PKS.
func (h *PartyHandler) DeletePartnerAgreement(ctx context.Context, req DeletePartnerAgreementRequestObject) (DeletePartnerAgreementResponseObject, error) {
	if err := h.svc.DeleteAgreement(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeletePartnerAgreement204Response{}, nil
}

// ListPartyUserOptions mencari pengguna aktif.
func (h *PartyHandler) ListPartyUserOptions(ctx context.Context, req ListPartyUserOptionsRequestObject) (ListPartyUserOptionsResponseObject, error) {
	users, err := h.svc.SearchUsers(ctx, str(req.Params.Q), str(req.Params.Role))
	if err != nil {
		return nil, err
	}
	out := make([]PartyUserOption, len(users))
	for i, u := range users {
		out[i] = PartyUserOption{Id: u.ID, FullName: u.Name, Email: u.Email}
	}
	return ListPartyUserOptions200JSONResponse(out), nil
}

// --- Kode produk mitra -------------------------------------------------------------------------

func (h *PartyHandler) productCodes(ctx context.Context, list []domain.ProductCode) ([]PartnerProductCode, error) {
	partnerIDs := make([]uuid.UUID, 0, len(list))
	productIDs := make([]uuid.UUID, 0, len(list))
	for _, c := range list {
		partnerIDs, productIDs = append(partnerIDs, c.PartnerID), append(productIDs, c.ProductID)
	}
	partners, err := h.svc.PartnerRefs(ctx, partnerIDs)
	if err != nil {
		return nil, err
	}
	products, err := h.svc.ProductRefs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	out := make([]PartnerProductCode, len(list))
	for i, c := range list {
		p := partners[c.PartnerID]
		out[i] = PartnerProductCode{
			Id: c.ID, PartnerId: c.PartnerID, PartnerCode: p.Code, PartnerName: p.Name, ProductId: c.ProductID,
			ArticleCode: c.ArticleCode, Barcode: c.Barcode, Description: c.Description, CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt, Version: c.Version,
		}
		if prod, ok := products[c.ProductID]; ok {
			out[i].ProductSku, out[i].ProductName = &prod.SKU, &prod.Name
		}
	}
	return out, nil
}

func (h *PartyHandler) productCode(ctx context.Context, c domain.ProductCode) (PartnerProductCode, error) {
	out, err := h.productCodes(ctx, []domain.ProductCode{c})
	if err != nil {
		return PartnerProductCode{}, err
	}
	return out[0], nil
}

// ListPartnerProductCodes mengembalikan kode produk mitra.
func (h *PartyHandler) ListPartnerProductCodes(ctx context.Context, req ListPartnerProductCodesRequestObject) (ListPartnerProductCodesResponseObject, error) {
	p := req.Params
	res, err := h.svc.ListProductCodes(ctx, service.ProductCodeQuery{PartnerID: p.PartnerId, ProductID: p.ProductId, Search: str(p.Q)},
		pageOf(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items, err := h.productCodes(ctx, res.Items)
	if err != nil {
		return nil, err
	}
	return ListPartnerProductCodes200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreatePartnerProductCode menambah kode produk mitra.
func (h *PartyHandler) CreatePartnerProductCode(ctx context.Context, req CreatePartnerProductCodeRequestObject) (CreatePartnerProductCodeResponseObject, error) {
	b := req.Body
	c, err := h.svc.CreateProductCode(ctx, b.PartnerId, b.ProductId, domain.ProductCodeInput{
		ArticleCode: b.ArticleCode, Barcode: str(b.Barcode), Description: str(b.Description),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.productCode(ctx, c)
	if err != nil {
		return nil, err
	}
	return CreatePartnerProductCode201JSONResponse(out), nil
}

// UpdatePartnerProductCode mengubah kode produk mitra.
func (h *PartyHandler) UpdatePartnerProductCode(ctx context.Context, req UpdatePartnerProductCodeRequestObject) (UpdatePartnerProductCodeResponseObject, error) {
	b := req.Body
	c, err := h.svc.UpdateProductCode(ctx, req.Id, b.Version, domain.ProductCodeInput{
		ArticleCode: b.ArticleCode, Barcode: str(b.Barcode), Description: str(b.Description),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.productCode(ctx, c)
	if err != nil {
		return nil, err
	}
	return UpdatePartnerProductCode200JSONResponse(out), nil
}

// DeletePartnerProductCode menghapus kode produk mitra.
func (h *PartyHandler) DeletePartnerProductCode(ctx context.Context, req DeletePartnerProductCodeRequestObject) (DeletePartnerProductCodeResponseObject, error) {
	if err := h.svc.DeleteProductCode(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeletePartnerProductCode204Response{}, nil
}

// --- Pemasok & maklon --------------------------------------------------------------------------

func supplierInput(b SupplierInput) domain.SupplierInput {
	in := domain.SupplierInput{
		Code: b.Code, Name: b.Name, Type: domain.SupplierType(b.SupplierType), IsBusinessEntity: b.IsBusinessEntity,
		NPWP: b.Npwp, DefaultWithholding: domain.WithholdingType(b.DefaultWithholding), PaymentTermDays: b.PaymentTermDays,
		Address: str(b.Address), Phone: str(b.Phone), Email: str(b.Email), PICName: str(b.PicName), Notes: str(b.Notes),
		BankAccounts: make([]domain.BankAccountInput, len(b.BankAccounts)),
	}
	for i, a := range b.BankAccounts {
		in.BankAccounts[i] = domain.BankAccountInput{
			BankName: a.BankName, AccountNumber: str(a.AccountNumber), AccountName: a.AccountName, IsDefault: a.IsDefault,
		}
		if a.Id != nil {
			in.BankAccounts[i].ID = *a.Id
		}
	}
	return in
}

// toSupplier menampilkan NPWP dan nomor rekening tersamar (NFR-04).
func toSupplier(s domain.Supplier) Supplier {
	out := Supplier{
		Id: s.ID, Code: s.Code, Name: s.Name, SupplierType: SupplierType(s.Type), IsBusinessEntity: s.IsBusinessEntity,
		HasNpwp: s.NPWP != "", NpwpMasked: masked(s.NPWP), DefaultWithholding: WithholdingType(s.DefaultWithholding),
		PaymentTermDays: s.PaymentTermDays, Address: s.Address, Phone: s.Phone, Email: s.Email, PicName: s.PICName,
		Notes: s.Notes, BankAccounts: make([]SupplierBankAccount, len(s.BankAccounts)), CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt, Version: s.Version,
	}
	for i, b := range s.BankAccounts {
		out.BankAccounts[i] = SupplierBankAccount{
			Id: b.ID, BankName: b.BankName, AccountNumberMasked: pii.Mask(b.AccountNumber), AccountName: b.AccountName,
			IsDefault: b.IsDefault,
		}
	}
	return out
}

// ListSuppliers mengembalikan daftar pemasok.
func (h *PartyHandler) ListSuppliers(ctx context.Context, req ListSuppliersRequestObject) (ListSuppliersResponseObject, error) {
	p := req.Params
	q := service.SupplierQuery{
		Search: str(p.Q), Sort: sortOf(p.Sort, []string{"code", "name", "createdAt"}, paging.Sort{Field: "code"}),
	}
	if p.SupplierType != nil {
		q.Type = domain.SupplierType(*p.SupplierType)
	}
	res, err := h.svc.ListSuppliers(ctx, q, pageOf(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]Supplier, len(res.Items))
	for i, s := range res.Items {
		items[i] = toSupplier(s)
	}
	return ListSuppliers200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateSupplier membuat pemasok.
func (h *PartyHandler) CreateSupplier(ctx context.Context, req CreateSupplierRequestObject) (CreateSupplierResponseObject, error) {
	s, err := h.svc.CreateSupplier(ctx, supplierInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return CreateSupplier201JSONResponse(toSupplier(s)), nil
}

// GetSupplier mengembalikan detail pemasok.
func (h *PartyHandler) GetSupplier(ctx context.Context, req GetSupplierRequestObject) (GetSupplierResponseObject, error) {
	s, err := h.svc.GetSupplier(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetSupplier200JSONResponse(toSupplier(s)), nil
}

// UpdateSupplier mengubah pemasok.
func (h *PartyHandler) UpdateSupplier(ctx context.Context, req UpdateSupplierRequestObject) (UpdateSupplierResponseObject, error) {
	b := req.Body
	s, err := h.svc.UpdateSupplier(ctx, req.Id, b.Version, supplierInput(SupplierInput{
		Address: b.Address, BankAccounts: b.BankAccounts, Code: b.Code, DefaultWithholding: b.DefaultWithholding,
		Email: b.Email, IsBusinessEntity: b.IsBusinessEntity, Name: b.Name, Notes: b.Notes, Npwp: b.Npwp,
		PaymentTermDays: b.PaymentTermDays, Phone: b.Phone, PicName: b.PicName, SupplierType: b.SupplierType,
	}))
	if err != nil {
		return nil, err
	}
	return UpdateSupplier200JSONResponse(toSupplier(s)), nil
}

// DeleteSupplier menghapus pemasok.
func (h *PartyHandler) DeleteSupplier(ctx context.Context, req DeleteSupplierRequestObject) (DeleteSupplierResponseObject, error) {
	if err := h.svc.DeleteSupplier(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteSupplier204Response{}, nil
}
