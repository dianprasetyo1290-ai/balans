// Package smtpmail mengirim email lewat SMTP. Dengan EMAIL_PROVIDER=mock, tujuannya Mailpit sehingga
// email tidak pernah keluar dan bisa dilihat di /_mock/mail (Technical Guideline 5.10).
package smtpmail

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"balans/internal/notification"
)

// Sender mengimplementasikan notification.EmailSender.
type Sender struct {
	addr string
	from string
}

var _ notification.EmailSender = (*Sender)(nil)

// New membuat Sender ke host:port tanpa autentikasi (Mailpit).
func New(host string, port int, from string) *Sender {
	return &Sender{addr: net.JoinHostPort(host, strconv.Itoa(port)), from: from}
}

// Send mengirim email teks UTF-8.
func (s *Sender) Send(ctx context.Context, email notification.Email) error {
	from, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("parse sender address: %w", err)
	}
	msg := s.message(from, email)

	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(s.addr, nil, from.Address, []string{email.To}, msg) }()
	select {
	case <-ctx.Done():
		return fmt.Errorf("send email: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return fmt.Errorf("send email: %w", err)
		}
		return nil
	}
}

func (s *Sender) message(from *mail.Address, email notification.Email) []byte {
	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + email.To + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", email.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(email.Text, "\n", "\r\n"))
	return []byte(b.String())
}
