package http

import (
	"context"

	"github.com/go-chi/chi/v5"

	"balans/internal/hris/domain"
	"balans/internal/hris/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/hris.yaml ../../../api/openapi.bundle.yaml

// HRISHandler mengimplementasikan StrictServerInterface hasil generate.
type HRISHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*HRISHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *HRISHandler { return &HRISHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *HRISHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func input(b EmployeeInput) domain.Input {
	return domain.Input{
		Number: b.EmployeeNumber, FullName: b.FullName, DivisionID: b.DivisionId, PositionTitle: b.PositionTitle,
		JoinDate: fromDate(b.JoinDate), WorkEmail: str(b.WorkEmail), Phone: str(b.Phone), NIK: b.Nik,
	}
}

// toEmployee menampilkan NIK tersamar (NFR-04).
func toEmployee(e domain.Employee, today clock.BusinessDate) Employee {
	out := Employee{
		Id: e.ID, EmployeeNumber: e.Number, FullName: e.FullName, DivisionId: e.DivisionID, PositionTitle: e.PositionTitle,
		EmploymentStatus: EmploymentStatus(e.Status), IsActive: e.ActiveOn(today), JoinDate: toDate(e.JoinDate),
		ResignReason: e.ResignReason, WorkEmail: e.WorkEmail, Phone: e.Phone, HasNik: e.NIK != "",
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Version: e.Version,
	}
	if e.ResignDate != nil {
		d := toDate(*e.ResignDate)
		out.ResignDate = &d
	}
	if e.NIK != "" {
		m := pii.Mask(e.NIK)
		out.NikMasked = &m
	}
	return out
}

// ListEmployees mengembalikan daftar karyawan.
func (h *HRISHandler) ListEmployees(ctx context.Context, req ListEmployeesRequestObject) (ListEmployeesResponseObject, error) {
	p := req.Params
	var sort *string
	if p.Sort != nil {
		v := *p.Sort
		sort = &v
	}
	q := service.Query{
		Search: str(p.Q), NIK: str(p.Nik),
		Sort: paging.ParseSort(sort, []string{"employeeNumber", "fullName", "joinDate"}, paging.Sort{Field: "employeeNumber"}),
	}
	if p.Status != nil {
		q.Status = domain.Status(*p.Status)
	}
	var page, size *int
	if p.Page != nil {
		v := *p.Page
		page = &v
	}
	if p.PageSize != nil {
		v := *p.PageSize
		size = &v
	}
	res, err := h.svc.List(ctx, q, paging.New(page, size))
	if err != nil {
		return nil, err
	}
	today := h.svc.Today()
	items := make([]Employee, len(res.Items))
	for i, e := range res.Items {
		items[i] = toEmployee(e, today)
	}
	return ListEmployees200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateEmployee menambah karyawan.
func (h *HRISHandler) CreateEmployee(ctx context.Context, req CreateEmployeeRequestObject) (CreateEmployeeResponseObject, error) {
	e, err := h.svc.Create(ctx, input(*req.Body))
	if err != nil {
		return nil, err
	}
	return CreateEmployee201JSONResponse(toEmployee(e, h.svc.Today())), nil
}

// GetEmployee mengembalikan detail karyawan.
func (h *HRISHandler) GetEmployee(ctx context.Context, req GetEmployeeRequestObject) (GetEmployeeResponseObject, error) {
	e, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetEmployee200JSONResponse(toEmployee(e, h.svc.Today())), nil
}

// UpdateEmployee mengubah data karyawan.
func (h *HRISHandler) UpdateEmployee(ctx context.Context, req UpdateEmployeeRequestObject) (UpdateEmployeeResponseObject, error) {
	b := req.Body
	e, err := h.svc.Update(ctx, req.Id, b.Version, input(EmployeeInput{
		DivisionId: b.DivisionId, EmployeeNumber: b.EmployeeNumber, FullName: b.FullName, JoinDate: b.JoinDate,
		Nik: b.Nik, Phone: b.Phone, PositionTitle: b.PositionTitle, WorkEmail: b.WorkEmail,
	}))
	if err != nil {
		return nil, err
	}
	return UpdateEmployee200JSONResponse(toEmployee(e, h.svc.Today())), nil
}

// ResignEmployee mencatat resign.
func (h *HRISHandler) ResignEmployee(ctx context.Context, req ResignEmployeeRequestObject) (ResignEmployeeResponseObject, error) {
	b := req.Body
	e, err := h.svc.Resign(ctx, req.Id, b.Version, fromDate(b.ResignDate), b.Reason)
	if err != nil {
		return nil, err
	}
	return ResignEmployee200JSONResponse(toEmployee(e, h.svc.Today())), nil
}

// ListEmployeeOptions mencari karyawan aktif.
func (h *HRISHandler) ListEmployeeOptions(ctx context.Context, req ListEmployeeOptionsRequestObject) (ListEmployeeOptionsResponseObject, error) {
	list, err := h.svc.Options(ctx, str(req.Params.Q))
	if err != nil {
		return nil, err
	}
	today := h.svc.Today()
	out := make([]EmployeeOption, len(list))
	for i, e := range list {
		out[i] = EmployeeOption{Id: e.ID, EmployeeNumber: e.Number, FullName: e.FullName, IsActive: e.ActiveOn(today)}
	}
	return ListEmployeeOptions200JSONResponse(out), nil
}
