package domain_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/filestore/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

var policy = domain.OwnerPolicy{
	OwnerType: "partner_agreement", WritePermission: "party.partner_agreement.write",
	ReadPermission: "party.partner_agreement.read", ContentTypes: []string{domain.TypePDF, domain.TypePNG}, MaxBytes: 10 << 20,
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func TestNewUpload(t *testing.T) {
	f, err := domain.NewUpload(domain.UploadInput{
		OwnerType: "partner_agreement", OriginalName: `C:\\Users\\rina\\PKS "Alfa".pdf`, ContentType: "Application/PDF; charset=binary",
		SizeBytes: 2048,
	}, policy)
	require.NoError(t, err)
	assert.Equal(t, "PKS Alfa.pdf", f.OriginalName)
	assert.Equal(t, domain.TypePDF, f.ContentType)
	assert.Equal(t, domain.StatusPending, f.Status)

	cases := []struct {
		name string
		in   domain.UploadInput
		want apperr.Code
	}{
		{"tipe salah", domain.UploadInput{OriginalName: "a.exe", ContentType: "application/x-msdownload", SizeBytes: 1}, apperr.CodeFileTypeNotAllowed},
		{"terlalu besar", domain.UploadInput{OriginalName: "a.pdf", ContentType: domain.TypePDF, SizeBytes: 10<<20 + 1}, apperr.CodeFileTooLarge},
		{"kosong", domain.UploadInput{OriginalName: "a.pdf", ContentType: domain.TypePDF}, apperr.CodeValidationFailed},
		{"tanpa nama", domain.UploadInput{OriginalName: "/", ContentType: domain.TypePDF, SizeBytes: 1}, apperr.CodeValidationFailed},
		{"nama panjang", domain.UploadInput{OriginalName: strings.Repeat("a", 201), ContentType: domain.TypePDF, SizeBytes: 1}, apperr.CodeValidationFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewUpload(c.in, policy)
			assert.Equal(t, c.want, code(err))
		})
	}
	_, err = domain.NewUpload(domain.UploadInput{OriginalName: "a.gif", ContentType: "image/gif", SizeBytes: 1}, policy)
	assert.Contains(t, apperr.As(err).Message, "PDF, PNG")
}

func TestFormatSize(t *testing.T) {
	assert.Equal(t, "10 MB", domain.FormatSize(10<<20))
	assert.Equal(t, "1536 KB", domain.FormatSize(1536<<10))
	assert.Equal(t, "500 byte", domain.FormatSize(500))
	assert.Equal(t, "image/png", domain.NormalizeContentType(" IMAGE/PNG "))
	assert.Equal(t, "x;;", domain.NormalizeContentType("X;;"))
}

func TestMarkUploaded(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	newFile := func() domain.File {
		return domain.File{ContentType: domain.TypePDF, SizeBytes: 100, Status: domain.StatusPending}
	}
	f := newFile()
	require.NoError(t, f.MarkUploaded(100, []byte{1}, "application/pdf", now))
	assert.Equal(t, domain.StatusUploaded, f.Status)
	assert.Equal(t, now, *f.UploadedAt)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(f.MarkUploaded(100, nil, "application/pdf", now)))

	f = newFile()
	assert.Equal(t, apperr.CodeFileTooLarge, code(f.MarkUploaded(101, nil, "application/pdf", now)))
	assert.Equal(t, apperr.CodeBadRequest, code(f.MarkUploaded(99, nil, "application/pdf", now)))
	assert.Equal(t, apperr.CodeFileTypeNotAllowed, code(f.MarkUploaded(100, nil, "text/html; charset=utf-8", now)))
	assert.Equal(t, domain.StatusPending, f.Status)
}

func TestAttachToAndCanRead(t *testing.T) {
	uploader, other, owner := uuid.New(), uuid.New(), uuid.New()
	uploaded := func() domain.File {
		return domain.File{OwnerType: "partner_agreement", Status: domain.StatusUploaded, CreatedBy: uploader}
	}

	f := uploaded()
	assert.True(t, f.CanRead(actor.Actor{UserID: uploader}, policy), "pengunggah membaca file yang belum ditautkan")
	assert.False(t, f.CanRead(actor.Actor{UserID: other, Permissions: map[string]struct{}{policy.ReadPermission: {}}}, policy))

	changed, err := f.AttachTo("partner_agreement", owner, uploader)
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = f.AttachTo("partner_agreement", owner, other)
	require.NoError(t, err)
	assert.False(t, changed, "menautkan ulang ke pemilik yang sama tidak mengubah apa pun")

	assert.True(t, f.CanRead(actor.Actor{UserID: other, Permissions: map[string]struct{}{policy.ReadPermission: {}}}, policy))
	assert.False(t, f.CanRead(actor.Actor{UserID: uploader}, policy), "setelah ditautkan butuh permission baca pemilik")

	_, err = f.AttachTo("partner_agreement", uuid.New(), uploader)
	assert.Contains(t, apperr.As(err).Fields["fileId"], "sudah dipakai")

	g := uploaded()
	_, err = g.AttachTo("qc_photo", owner, uploader)
	assert.Contains(t, apperr.As(err).Fields["fileId"], "tidak bisa dipakai")
	_, err = g.AttachTo("partner_agreement", owner, other)
	assert.Contains(t, apperr.As(err).Fields["fileId"], "pengguna lain")
	g.Status = domain.StatusPending
	_, err = g.AttachTo("partner_agreement", owner, uploader)
	assert.Contains(t, apperr.As(err).Fields["fileId"], "belum selesai")
}

func TestSignedURL(t *testing.T) {
	key := []byte("kunci-uji-coba-yang-cukup-panjang-32")
	sign := func(msg string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	id := uuid.New()
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	exp := now.Add(domain.DownloadURLTTL)

	raw := domain.ContentURL(sign, domain.PurposeDownload, id, exp)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/file-content/"+id.String(), u.Path)
	expUnix, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	require.NoError(t, err)
	sig := u.Query().Get("sig")

	require.NoError(t, domain.Verify(sign, domain.PurposeDownload, id, expUnix, sig, now))
	assert.Equal(t, apperr.CodeForbidden, code(domain.Verify(sign, domain.PurposeUpload, id, expUnix, sig, now)), "tanda tangan unduh tidak berlaku untuk upload")
	assert.Equal(t, apperr.CodeForbidden, code(domain.Verify(sign, domain.PurposeDownload, uuid.New(), expUnix, sig, now)))
	assert.Equal(t, apperr.CodeForbidden, code(domain.Verify(sign, domain.PurposeDownload, id, expUnix+60, sig, now)), "masa berlaku diubah")
	assert.Equal(t, apperr.CodeForbidden, code(domain.Verify(sign, domain.PurposeDownload, id, expUnix, "!!", now)))
	err = domain.Verify(sign, domain.PurposeDownload, id, expUnix, sig, exp.Add(time.Second))
	assert.Contains(t, apperr.As(err).Message, "kedaluwarsa")
}
