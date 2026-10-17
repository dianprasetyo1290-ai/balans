package service_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/filestore/domain"
	"balans/internal/filestore/localdisk"
	"balans/internal/filestore/postgres"
	"balans/internal/filestore/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	permWrite = "party.partner_agreement.write"
	permRead  = "party.partner_agreement.read"
)

var pdf = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 300)...)

type harness struct {
	t     *testing.T
	svc   *service.Service
	clock *clock.Fixed
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	disk, err := localdisk.New(t.TempDir())
	require.NoError(t, err)
	fixed := clock.NewFixed(time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC))
	sign := func(msg string) []byte {
		m := hmac.New(sha256.New, []byte("kunci-uji-coba-filestore-32-byte!"))
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	svc := service.New(service.Deps{Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Storage: disk, Clock: fixed, Sign: sign})
	svc.RegisterOwner(domain.OwnerPolicy{
		OwnerType: "partner_agreement", WritePermission: permWrite, ReadPermission: permRead,
		ContentTypes: []string{domain.TypePDF}, MaxBytes: 1 << 20,
	})
	return &harness{t: t, svc: svc, clock: fixed}
}

func as(user uuid.UUID, perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: user, Permissions: set})
}

func code(err error) apperr.Code { return apperr.As(err).Code }

// signed memecah URL bertanda tangan menjadi parameter handler.
func signed(t *testing.T, raw string) (uuid.UUID, int64, string) {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	id, err := uuid.Parse(u.Path[len("/api/v1/file-content/"):])
	require.NoError(t, err)
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	require.NoError(t, err)
	return id, exp, u.Query().Get("sig")
}

func (h *harness) upload(user uuid.UUID, content []byte) domain.File {
	h.t.Helper()
	f, ticket, err := h.svc.CreateUpload(as(user, permWrite), domain.UploadInput{
		OwnerType: "partner_agreement", OriginalName: "PKS Alfa.pdf", ContentType: "application/pdf", SizeBytes: int64(len(content)),
	})
	require.NoError(h.t, err)
	id, exp, sig := signed(h.t, ticket.URL)
	require.Equal(h.t, f.ID, id)
	f, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(content))
	require.NoError(h.t, err)
	return f
}

func TestUploadAttachDownload(t *testing.T) {
	h := setup(t)
	uploader, reader := uuid.New(), uuid.New()
	f := h.upload(uploader, pdf)
	assert.Equal(t, domain.StatusUploaded, f.Status)
	assert.Len(t, f.SHA256, 32)

	// Sebelum ditautkan hanya pengunggah yang boleh mengunduh.
	_, err := h.svc.DownloadURL(as(reader, permRead), f.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.DownloadURL(as(uploader), f.ID)
	require.NoError(t, err)

	owner := uuid.New()
	require.Error(t, func() error { _, err := h.svc.Attach(as(reader), f.ID, "partner_agreement", owner); return err }(),
		"file pengguna lain tidak bisa ditautkan")
	attached, err := h.svc.Attach(as(uploader), f.ID, "partner_agreement", owner)
	require.NoError(t, err)
	assert.Equal(t, owner, *attached.OwnerID)

	names, err := h.svc.Names(context.Background(), []uuid.UUID{f.ID, uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]string{f.ID: "PKS Alfa.pdf"}, names)

	ticket, err := h.svc.DownloadURL(as(reader, permRead), f.ID)
	require.NoError(t, err)
	assert.Equal(t, h.clock.Now().Add(domain.DownloadURLTTL), ticket.ExpiresAt)
	id, exp, sig := signed(t, ticket.URL)
	got, body, err := h.svc.Open(context.Background(), id, exp, sig)
	require.NoError(t, err)
	defer body.Close() //nolint:errcheck // test
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, pdf, content)
	assert.Equal(t, "application/pdf", got.ContentType)

	// URL unduhan kedaluwarsa setelah 5 menit dan tidak bisa dipakai untuk upload.
	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(pdf))
	assert.Equal(t, apperr.CodeForbidden, code(err))
	h.clock.Advance(6 * time.Minute)
	_, _, err = h.svc.Open(context.Background(), id, exp, sig)
	assert.Equal(t, apperr.CodeForbidden, code(err))
}

func TestCreateUploadRules(t *testing.T) {
	h := setup(t)
	user := uuid.New()
	in := domain.UploadInput{OwnerType: "partner_agreement", OriginalName: "a.pdf", ContentType: "application/pdf", SizeBytes: 10}

	_, _, err := h.svc.CreateUpload(context.Background(), in)
	assert.Equal(t, apperr.CodeAuthSessionExpired, code(err))
	_, _, err = h.svc.CreateUpload(as(user), in)
	assert.Equal(t, apperr.CodeForbidden, code(err), "butuh permission tulis pemilik")

	bad := in
	bad.OwnerType = "lain"
	_, _, err = h.svc.CreateUpload(as(user, permWrite), bad)
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
	bad = in
	bad.SizeBytes = 2 << 20
	_, _, err = h.svc.CreateUpload(as(user, permWrite), bad)
	assert.Equal(t, apperr.CodeFileTooLarge, code(err))
	bad = in
	bad.ContentType = "image/png"
	_, _, err = h.svc.CreateUpload(as(user, permWrite), bad)
	assert.Equal(t, apperr.CodeFileTypeNotAllowed, code(err))
	assert.Equal(t, int64(1<<20), h.svc.MaxBytes("partner_agreement"))
}

func TestUploadRejectsMismatchedContent(t *testing.T) {
	h := setup(t)
	user := uuid.New()
	create := func(size int) (uuid.UUID, int64, string) {
		_, ticket, err := h.svc.CreateUpload(as(user, permWrite), domain.UploadInput{
			OwnerType: "partner_agreement", OriginalName: "a.pdf", ContentType: "application/pdf", SizeBytes: int64(size),
		})
		require.NoError(t, err)
		return signed(t, ticket.URL)
	}

	id, exp, sig := create(len(pdf))
	_, err := h.svc.Upload(context.Background(), id, exp, sig+"x", bytes.NewReader(pdf))
	assert.Equal(t, apperr.CodeForbidden, code(err), "tanda tangan salah")

	html := append([]byte("<html><body>"), bytes.Repeat([]byte("x"), len(pdf)-12)...)
	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(html))
	assert.Equal(t, apperr.CodeFileTypeNotAllowed, code(err), "isi bukan PDF")

	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(append(pdf, 'x')))
	assert.Equal(t, apperr.CodeFileTooLarge, code(err))

	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(pdf[:len(pdf)-1]))
	assert.Equal(t, apperr.CodeBadRequest, code(err))

	f, err := h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(pdf))
	require.NoError(t, err, "upload ulang yang benar tetap diterima")
	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(pdf))
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))

	got, err := h.svc.Get(as(user), f.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusUploaded, got.Status)

	_, err = h.svc.Attach(as(user), uuid.New(), "partner_agreement", uuid.New())
	assert.Contains(t, apperr.As(err).Fields["fileId"], "tidak ditemukan")

	h.clock.Advance(domain.UploadURLTTL + time.Second)
	id, exp, sig = create(len(pdf))
	h.clock.Advance(domain.UploadURLTTL + time.Second)
	_, err = h.svc.Upload(context.Background(), id, exp, sig, bytes.NewReader(pdf))
	assert.Equal(t, apperr.CodeForbidden, code(err), "URL upload kedaluwarsa")
}
