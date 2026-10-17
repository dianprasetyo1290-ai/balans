package http

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"balans/internal/filestore/domain"
	"balans/internal/filestore/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/httpx"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/filestore.yaml ../../../api/openapi.bundle.yaml

// ContentPathPrefix adalah prefix URL isi file bertanda tangan. Path ini tidak memakai access token dan
// Idempotency-Key; tanda tangan URL yang menjadi bukti izin.
const ContentPathPrefix = "/api/v1/file-content/"

// FileHandler mengimplementasikan StrictServerInterface hasil generate.
type FileHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*FileHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *FileHandler { return &FileHandler{svc: svc} }

// Register mendaftarkan route metadata (strict) dan route isi file (streaming).
func Register(r chi.Router, h *FileHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
	c := contentHandler{svc: h.svc, rs: rs}
	r.Put(ContentPathPrefix+"{id}", c.put)
	r.Get(ContentPathPrefix+"{id}", c.get)
}

func toFile(f domain.File) FileObject {
	return FileObject{
		Id: f.ID, FileName: f.OriginalName, ContentType: f.ContentType, SizeBytes: f.SizeBytes, OwnerType: f.OwnerType,
		OwnerId: f.OwnerID, Status: FileObjectStatus(f.Status), UploadedAt: f.UploadedAt, CreatedAt: f.CreatedAt,
	}
}

// CreateFileUpload meminta URL upload.
func (h *FileHandler) CreateFileUpload(ctx context.Context, req CreateFileUploadRequestObject) (CreateFileUploadResponseObject, error) {
	b := req.Body
	f, t, err := h.svc.CreateUpload(ctx, domain.UploadInput{
		OwnerType: b.OwnerType, OriginalName: b.FileName, ContentType: b.ContentType, SizeBytes: b.SizeBytes,
	})
	if err != nil {
		return nil, err
	}
	return CreateFileUpload201JSONResponse{
		FileId: f.ID, UploadUrl: t.URL, ExpiresAt: t.ExpiresAt, MaxSizeBytes: h.svc.MaxBytes(f.OwnerType),
	}, nil
}

// GetFile mengembalikan metadata file.
func (h *FileHandler) GetFile(ctx context.Context, req GetFileRequestObject) (GetFileResponseObject, error) {
	f, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetFile200JSONResponse(toFile(f)), nil
}

// GetFileDownloadUrl membuat URL unduhan.
func (h *FileHandler) GetFileDownloadUrl(ctx context.Context, req GetFileDownloadUrlRequestObject) (GetFileDownloadUrlResponseObject, error) {
	t, err := h.svc.DownloadURL(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetFileDownloadUrl200JSONResponse{Url: t.URL, ExpiresAt: t.ExpiresAt}, nil
}

// contentHandler mengalirkan isi file tanpa menahannya di memori.
type contentHandler struct {
	svc *service.Service
	rs  *httpx.Responder
}

func signedParams(r *http.Request) (uuid.UUID, int64, string, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, 0, "", apperr.New(apperr.CodeNotFound)
	}
	exp, err := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
	if err != nil {
		return uuid.Nil, 0, "", apperr.New(apperr.CodeForbidden)
	}
	return id, exp, r.URL.Query().Get("sig"), nil
}

func (c contentHandler) put(w http.ResponseWriter, r *http.Request) {
	id, exp, sig, err := signedParams(r)
	if err != nil {
		c.rs.Error(w, r, err)
		return
	}
	defer r.Body.Close() //nolint:errcheck // body request ditutup server
	if _, err := c.svc.Upload(r.Context(), id, exp, sig, r.Body); err != nil {
		c.rs.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c contentHandler) get(w http.ResponseWriter, r *http.Request) {
	id, exp, sig, err := signedParams(r)
	if err != nil {
		c.rs.Error(w, r, err)
		return
	}
	f, body, err := c.svc.Open(r.Context(), id, exp, sig)
	if err != nil {
		c.rs.Error(w, r, err)
		return
	}
	defer body.Close() //nolint:errcheck // hanya membaca
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": f.OriginalName}))
	modified := f.CreatedAt
	if f.UploadedAt != nil {
		modified = *f.UploadedAt
	}
	http.ServeContent(w, r, f.OriginalName, modified, body)
}
