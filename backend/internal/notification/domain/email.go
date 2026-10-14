// Package domain berisi entity notifikasi.
package domain

import (
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// Email adalah email transaksional yang akan dikirim.
type Email struct {
	ID      uuid.UUID
	To      string
	Subject string
	Text    string
}

// NewEmail memvalidasi isi email sebelum disimpan.
func NewEmail(to, subject, text string) (Email, error) {
	e := Email{To: strings.TrimSpace(to), Subject: strings.TrimSpace(subject), Text: text}
	fields := map[string]string{}
	if _, err := mail.ParseAddress(e.To); err != nil || !strings.Contains(e.To, "@") {
		fields["to"] = "Alamat email tidak valid"
	}
	if e.Subject == "" {
		fields["subject"] = "Subjek wajib diisi"
	}
	if strings.TrimSpace(e.Text) == "" {
		fields["text"] = "Isi email wajib diisi"
	}
	if len(fields) > 0 {
		return Email{}, apperr.Validation(fields)
	}
	return e, nil
}
