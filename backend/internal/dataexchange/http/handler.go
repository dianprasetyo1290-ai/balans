package http

import (
	"bytes"
	"context"

	"github.com/go-chi/chi/v5"

	"balans/internal/dataexchange/domain"
	"balans/internal/dataexchange/service"
	"balans/internal/platform/httpx"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/dataexchange.yaml ../../../api/openapi.bundle.yaml

// DataExchangeHandler mengimplementasikan StrictServerInterface hasil generate.
type DataExchangeHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*DataExchangeHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *DataExchangeHandler { return &DataExchangeHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *DataExchangeHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func pagination(p paging.Page, total int64) Pagination {
	return Pagination{Page: p.Number, PageSize: p.Size, TotalItems: total, TotalPages: p.TotalPages(total)}
}

func toImportJob(j domain.ImportJob, kindLabel string) ImportJob {
	acts := j.AllowedActions()
	actions := make([]ImportJobAllowedActions, len(acts))
	for i, a := range acts {
		actions[i] = ImportJobAllowedActions(a)
	}
	return ImportJob{
		Id: j.ID, Kind: j.Kind, KindLabel: kindLabel, FileName: j.FileName, Status: ImportJobStatus(j.Status),
		TotalRows: j.TotalRows, ProcessedRows: j.ProcessedRows, ValidRows: j.ValidRows, ErrorRows: j.ErrorRows,
		CommittedRows: j.CommittedRows, Progress: j.Progress(), Message: j.Message, AllowedActions: actions,
		ValidatedAt: j.ValidatedAt, CompletedAt: j.CompletedAt, CreatedBy: j.CreatedBy, CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
	}
}

func toExportJob(j domain.ExportJob, kindLabel string) ExportJob {
	return ExportJob{
		Id: j.ID, Kind: j.Kind, KindLabel: kindLabel, Status: ExportJobStatus(j.Status), TotalRows: j.TotalRows,
		FileName: j.FileName, Message: j.Message, CompletedAt: j.CompletedAt, CreatedBy: j.CreatedBy,
		CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
	}
}

// ListDataExchangeKinds mengembalikan jenis data yang tersedia untuk user.
func (h *DataExchangeHandler) ListDataExchangeKinds(ctx context.Context, _ ListDataExchangeKindsRequestObject) (ListDataExchangeKindsResponseObject, error) {
	kinds, err := h.svc.Kinds(ctx)
	if err != nil {
		return nil, err
	}
	maxBytes := int64(domain.MaxFileBytes)
	items := make([]DataExchangeKind, len(kinds))
	for i, k := range kinds {
		cols := make([]DataExchangeColumn, len(k.Spec.Columns))
		for j, c := range k.Spec.Columns {
			kind := DataExchangeColumnColumnType(c.Kind)
			if c.Kind == "" {
				kind = Text
			}
			sensitive := c.Sensitive
			cols[j] = DataExchangeColumn{
				Key: c.Key, Header: c.Header, Required: c.Required, ColumnType: kind, Options: c.OptionLabels(),
				Sensitive: &sensitive,
			}
		}
		items[i] = DataExchangeKind{
			Kind: k.Spec.Kind, Label: k.Spec.Label, CanImport: k.CanImport, CanExport: k.CanExport,
			UploadOwnerType: service.UploadOwnerType(k.Spec.Kind), MaxFileBytes: &maxBytes, Columns: cols,
		}
	}
	return ListDataExchangeKinds200JSONResponse{Items: items}, nil
}

// GetDataExchangeTemplate mengirim file template.
func (h *DataExchangeHandler) GetDataExchangeTemplate(ctx context.Context, req GetDataExchangeTemplateRequestObject) (GetDataExchangeTemplateResponseObject, error) {
	var buf bytes.Buffer
	if _, err := h.svc.WriteTemplate(ctx, req.Kind, &buf); err != nil {
		return nil, err
	}
	n := int64(buf.Len())
	return GetDataExchangeTemplate200ApplicationvndOpenxmlformatsOfficedocumentSpreadsheetmlSheetResponse{Body: &buf, ContentLength: n}, nil
}

// ListImportJobs mengembalikan riwayat impor.
func (h *DataExchangeHandler) ListImportJobs(ctx context.Context, req ListImportJobsRequestObject) (ListImportJobsResponseObject, error) {
	page := paging.New(req.Params.Page, req.Params.PageSize)
	res, err := h.svc.ListImports(ctx, req.Params.Kind, page)
	if err != nil {
		return nil, err
	}
	items := make([]ImportJob, len(res.Items))
	for i, j := range res.Items {
		items[i] = toImportJob(j, h.svc.KindLabel(j.Kind))
	}
	return ListImportJobs200JSONResponse{Items: items, Pagination: pagination(res.Page, res.Total)}, nil
}

// CreateImportJob memulai impor.
func (h *DataExchangeHandler) CreateImportJob(ctx context.Context, req CreateImportJobRequestObject) (CreateImportJobResponseObject, error) {
	j, err := h.svc.CreateImport(ctx, req.Body.Kind, req.Body.FileId)
	if err != nil {
		return nil, err
	}
	return CreateImportJob202JSONResponse(toImportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// GetImportJob mengembalikan status job impor.
func (h *DataExchangeHandler) GetImportJob(ctx context.Context, req GetImportJobRequestObject) (GetImportJobResponseObject, error) {
	j, err := h.svc.GetImport(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetImportJob200JSONResponse(toImportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// ListImportJobRows mengembalikan baris preview.
func (h *DataExchangeHandler) ListImportJobRows(ctx context.Context, req ListImportJobRowsRequestObject) (ListImportJobRowsResponseObject, error) {
	page := paging.New(req.Params.Page, req.Params.PageSize)
	res, err := h.svc.ImportRows(ctx, req.Id, req.Params.Valid, page)
	if err != nil {
		return nil, err
	}
	items := make([]ImportJobRow, len(res.Items))
	for i, r := range res.Items {
		errs := make([]ImportJobRowError, len(r.Errors))
		for k, e := range r.Errors {
			errs[k] = ImportJobRowError{Message: e.Message}
			if e.Column != "" {
				col := e.Column
				errs[k].Column = &col
			}
		}
		values := r.Values
		if values == nil {
			values = map[string]string{}
		}
		items[i] = ImportJobRow{RowNumber: r.Number, IsValid: r.Valid(), Values: values, Errors: errs}
	}
	return ListImportJobRows200JSONResponse{Items: items, Pagination: pagination(res.Page, res.Total)}, nil
}

// CommitImportJob menjadwalkan penyimpanan baris valid.
func (h *DataExchangeHandler) CommitImportJob(ctx context.Context, req CommitImportJobRequestObject) (CommitImportJobResponseObject, error) {
	j, err := h.svc.CommitImport(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return CommitImportJob202JSONResponse(toImportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// CancelImportJob membatalkan impor.
func (h *DataExchangeHandler) CancelImportJob(ctx context.Context, req CancelImportJobRequestObject) (CancelImportJobResponseObject, error) {
	j, err := h.svc.CancelImport(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return CancelImportJob200JSONResponse(toImportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// GetImportJobErrorFile mengembalikan URL file baris error.
func (h *DataExchangeHandler) GetImportJobErrorFile(ctx context.Context, req GetImportJobErrorFileRequestObject) (GetImportJobErrorFileResponseObject, error) {
	url, exp, err := h.svc.ImportErrorFileURL(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetImportJobErrorFile200JSONResponse{Url: url, ExpiresAt: exp}, nil
}

// ListExportJobs mengembalikan riwayat ekspor.
func (h *DataExchangeHandler) ListExportJobs(ctx context.Context, req ListExportJobsRequestObject) (ListExportJobsResponseObject, error) {
	page := paging.New(req.Params.Page, req.Params.PageSize)
	res, err := h.svc.ListExports(ctx, req.Params.Kind, page)
	if err != nil {
		return nil, err
	}
	items := make([]ExportJob, len(res.Items))
	for i, j := range res.Items {
		items[i] = toExportJob(j, h.svc.KindLabel(j.Kind))
	}
	return ListExportJobs200JSONResponse{Items: items, Pagination: pagination(res.Page, res.Total)}, nil
}

// CreateExportJob memulai ekspor.
func (h *DataExchangeHandler) CreateExportJob(ctx context.Context, req CreateExportJobRequestObject) (CreateExportJobResponseObject, error) {
	j, err := h.svc.CreateExport(ctx, req.Body.Kind)
	if err != nil {
		return nil, err
	}
	return CreateExportJob202JSONResponse(toExportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// GetExportJob mengembalikan status job ekspor.
func (h *DataExchangeHandler) GetExportJob(ctx context.Context, req GetExportJobRequestObject) (GetExportJobResponseObject, error) {
	j, err := h.svc.GetExport(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetExportJob200JSONResponse(toExportJob(j, h.svc.KindLabel(j.Kind))), nil
}

// GetExportJobDownloadUrl mengembalikan URL file ekspor.
func (h *DataExchangeHandler) GetExportJobDownloadUrl(ctx context.Context, req GetExportJobDownloadUrlRequestObject) (GetExportJobDownloadUrlResponseObject, error) {
	url, exp, err := h.svc.ExportDownloadURL(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetExportJobDownloadUrl200JSONResponse{Url: url, ExpiresAt: exp}, nil
}
