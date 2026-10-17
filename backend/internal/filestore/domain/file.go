// Package domain berisi aturan file: batas ukuran dan tipe per jenis pemilik, status upload, penautan ke
// pemilik, hak unduh, dan tanda tangan URL (Technical Guideline 5.16).
package domain

import (
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"mime"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

// Status adalah status upload file.
type Status string

const (
	// StatusPending: URL upload sudah dibuat, isi file belum diterima.
	StatusPending Status = "pending"
	// StatusUploaded: isi file tersimpan dan sudah diverifikasi ukuran serta tipenya.
	StatusUploaded Status = "uploaded"
)

// Masa berlaku URL bertanda tangan (Technical Guideline 5.16).
const (
	UploadURLTTL   = 15 * time.Minute
	DownloadURLTTL = 5 * time.Minute
)

// Tipe file yang umum dipakai pemilik.
const (
	TypePDF  = "application/pdf"
	TypeJPEG = "image/jpeg"
	TypePNG  = "image/png"
	TypeWebP = "image/webp"
	TypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	// TypeCSV dipakai file mutasi bank (S-402); isinya terdeteksi sebagai teks biasa.
	TypeCSV = "text/csv"
)

// sniffedAs adalah hasil http.DetectContentType untuk tipe yang tidak dikenali langsung (xlsx = arsip zip,
// csv = teks biasa).
var sniffedAs = map[string]string{TypeXLSX: "application/zip", TypeCSV: "text/plain"}

// contentMatches memeriksa isi file (hasil deteksi) sesuai tipe yang dideklarasikan.
func contentMatches(declared, sniffed string) bool {
	sniffed = NormalizeContentType(sniffed)
	if want, ok := sniffedAs[declared]; ok {
		return sniffed == want
	}
	return sniffed == declared
}

// MaxNameLength membatasi nama file asli.
const MaxNameLength = 200

// OwnerPolicy didaftarkan modul pemilik file saat wiring: permission yang dibutuhkan untuk mengunggah dan
// mengunduh, serta tipe dan ukuran yang diizinkan.
type OwnerPolicy struct {
	OwnerType       string
	WritePermission string
	ReadPermission  string
	ContentTypes    []string
	MaxBytes        int64
}

// File adalah metadata satu file.
type File struct {
	ID           uuid.UUID
	ObjectKey    uuid.UUID
	OriginalName string
	ContentType  string
	SizeBytes    int64
	SHA256       []byte
	OwnerType    string
	OwnerID      *uuid.UUID
	Status       Status
	UploadedAt   *time.Time
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
	Version      int
}

// UploadInput adalah permintaan upload dari browser.
type UploadInput struct {
	OwnerType    string
	OriginalName string
	ContentType  string
	SizeBytes    int64
}

// NormalizeContentType membuang parameter (mis. `; charset=…`) dan mengecilkan huruf.
func NormalizeContentType(s string) string {
	mt, _, err := mime.ParseMediaType(strings.TrimSpace(s))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return mt
}

// cleanName mengambil nama dasar file tanpa path dan karakter kontrol.
func cleanName(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\\", "/")
	s = path.Base(s)
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, s)
	if s == "." || s == "/" {
		return ""
	}
	return s
}

// FormatSize menampilkan batas ukuran untuk pesan error, mis. `10 MB`.
func FormatSize(n int64) string {
	const mb = 1 << 20
	if n >= mb && n%mb == 0 {
		return fmt.Sprintf("%d MB", n/mb)
	}
	if n >= 1<<10 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d byte", n)
}

// NewUpload memvalidasi permintaan upload terhadap kebijakan pemilik.
func NewUpload(in UploadInput, p OwnerPolicy) (File, error) {
	name := cleanName(in.OriginalName)
	if name == "" {
		return File{}, apperr.Validation(map[string]string{"fileName": "Nama file wajib diisi"})
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return File{}, apperr.Validation(map[string]string{"fileName": "Nama file maksimal 200 karakter"})
	}
	ct := NormalizeContentType(in.ContentType)
	if !slices.Contains(p.ContentTypes, ct) {
		return File{}, apperr.WithMessage(apperr.CodeFileTypeNotAllowed,
			"Tipe file tidak diizinkan. Gunakan "+describeTypes(p.ContentTypes)+".")
	}
	if in.SizeBytes <= 0 {
		return File{}, apperr.Validation(map[string]string{"sizeBytes": "File kosong"})
	}
	if in.SizeBytes > p.MaxBytes {
		return File{}, apperr.WithMessage(apperr.CodeFileTooLarge, "Ukuran file maksimal "+FormatSize(p.MaxBytes)+".")
	}
	return File{OriginalName: name, ContentType: ct, SizeBytes: in.SizeBytes, OwnerType: p.OwnerType, Status: StatusPending}, nil
}

func describeTypes(types []string) string {
	labels := map[string]string{TypePDF: "PDF", TypeJPEG: "JPG", TypePNG: "PNG", TypeWebP: "WebP", TypeXLSX: "Excel (.xlsx)", TypeCSV: "CSV"}
	out := make([]string, 0, len(types))
	for _, t := range types {
		if l, ok := labels[t]; ok {
			out = append(out, l)
		} else {
			out = append(out, t)
		}
	}
	return strings.Join(out, ", ")
}

// MarkUploaded menerima isi file. written adalah jumlah byte yang diterima, sniffed hasil deteksi isi file
// (http.DetectContentType). Ukuran harus sama persis dengan yang dideklarasikan dan isi harus sesuai tipe.
func (f *File) MarkUploaded(written int64, sum []byte, sniffed string, now time.Time) error {
	if f.Status != StatusPending {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "File sudah diunggah.")
	}
	if written > f.SizeBytes {
		return apperr.WithMessage(apperr.CodeFileTooLarge, "Ukuran file melebihi yang dideklarasikan.")
	}
	if written != f.SizeBytes {
		return apperr.WithMessage(apperr.CodeBadRequest, "Ukuran file tidak sesuai. Unggah ulang file.")
	}
	if !contentMatches(f.ContentType, sniffed) {
		return apperr.WithMessage(apperr.CodeFileTypeNotAllowed, "Isi file tidak sesuai dengan tipenya.")
	}
	f.Status, f.SHA256, f.UploadedAt = StatusUploaded, sum, &now
	return nil
}

// AttachTo menautkan file ke pemiliknya di request bisnis. File harus sudah diunggah, jenis pemiliknya cocok,
// dan diunggah oleh actor yang sama; menautkan ulang ke pemilik yang sama tidak mengubah apa pun.
func (f *File) AttachTo(ownerType string, ownerID, actorID uuid.UUID) (changed bool, err error) {
	invalid := func(msg string) error { return apperr.Validation(map[string]string{"fileId": msg}) }
	switch {
	case f.OwnerType != ownerType:
		return false, invalid("File tidak bisa dipakai untuk data ini")
	case f.Status != StatusUploaded:
		return false, invalid("File belum selesai diunggah")
	case f.OwnerID != nil && *f.OwnerID == ownerID:
		return false, nil
	case f.OwnerID != nil:
		return false, invalid("File sudah dipakai data lain")
	case f.CreatedBy != actorID:
		return false, invalid("File diunggah pengguna lain")
	}
	f.OwnerID = &ownerID
	return true, nil
}

// CanRead: file yang sudah ditautkan dibaca pemegang permission baca pemiliknya; file yang belum ditautkan
// hanya oleh pengunggahnya.
func (f File) CanRead(a actor.Actor, p OwnerPolicy) bool {
	if f.OwnerID == nil {
		return f.CreatedBy == a.UserID
	}
	return p.ReadPermission != "" && a.Has(p.ReadPermission)
}

// Purpose membedakan tanda tangan URL upload dan unduh.
type Purpose string

const (
	PurposeUpload   Purpose = "put"
	PurposeDownload Purpose = "get"
)

// Signer menghitung HMAC pesan.
type Signer func(msg string) []byte

func payload(p Purpose, id uuid.UUID, expires int64) string {
	return fmt.Sprintf("filestore:%s:%s:%d", p, id, expires)
}

// Sign membuat tanda tangan URL.
func Sign(sign Signer, p Purpose, id uuid.UUID, expires time.Time) string {
	return base64.RawURLEncoding.EncodeToString(sign(payload(p, id, expires.Unix())))
}

// Verify memeriksa tanda tangan dan masa berlaku URL.
func Verify(sign Signer, p Purpose, id uuid.UUID, expires int64, sig string, now time.Time) error {
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, sign(payload(p, id, expires))) {
		return apperr.New(apperr.CodeForbidden)
	}
	if now.Unix() > expires {
		return apperr.WithMessage(apperr.CodeForbidden, "Tautan file sudah kedaluwarsa. Muat ulang halaman.")
	}
	return nil
}

// ContentURL menyusun URL relatif isi file.
func ContentURL(sign Signer, p Purpose, id uuid.UUID, expires time.Time) string {
	return fmt.Sprintf("/api/v1/file-content/%s?exp=%d&sig=%s", id, expires.Unix(), Sign(sign, p, id, expires))
}
