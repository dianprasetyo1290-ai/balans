package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// Jenis notifikasi in-app. Format `<modul>.<peristiwa>`.
const (
	KindApprovalPending  = "approval.pending"
	KindApprovalReminder = "approval.reminder"
	KindApprovalDecided  = "approval.decided"
)

var kindPattern = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)

// Batas panjang teks notifikasi agar tampil rapi di daftar notifikasi.
const (
	maxTitleLength = 200
	maxBodyLength  = 1000
)

// Notification adalah notifikasi in-app untuk satu user (NFR-11).
type Notification struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Kind   string
	Title  string
	Body   string
	// Link adalah path ERP yang dibuka saat notifikasi diklik, mis. /approvals.
	Link      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// Message adalah isi notifikasi yang dikirim ke satu atau beberapa user.
type Message struct {
	Kind  string
	Title string
	Body  string
	Link  string
}

// Validate memeriksa isi notifikasi sebelum disimpan.
func (m Message) Validate() error {
	fields := map[string]string{}
	if !kindPattern.MatchString(m.Kind) {
		fields["kind"] = "Jenis notifikasi tidak valid"
	}
	title := strings.TrimSpace(m.Title)
	switch {
	case title == "":
		fields["title"] = "Judul wajib diisi"
	case utf8.RuneCountInString(title) > maxTitleLength:
		fields["title"] = "Judul terlalu panjang"
	}
	if utf8.RuneCountInString(m.Body) > maxBodyLength {
		fields["body"] = "Isi terlalu panjang"
	}
	if m.Link != "" && !strings.HasPrefix(m.Link, "/") {
		fields["link"] = "Tautan harus berupa path ERP"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Recipients menghapus user kosong dan duplikat dengan urutan tetap.
func Recipients(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
