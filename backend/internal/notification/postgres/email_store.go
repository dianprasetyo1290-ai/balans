// Package postgres adalah repository modul notification.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/notification/domain"
	"balans/internal/notification/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
)

const statusSent = "sent"

// EmailStore menyimpan email di tabel notification_emails.
type EmailStore struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewEmailStore membuat EmailStore.
func NewEmailStore(pool *pgxpool.Pool, c clock.Clock) *EmailStore {
	return &EmailStore{pool: pool, clock: c}
}

func (s *EmailStore) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Create menyimpan email baru berstatus queued.
func (s *EmailStore) Create(ctx context.Context, email domain.Email) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertNotificationEmail(ctx, sqlcgen.InsertNotificationEmailParams{
		Recipient: email.To,
		Subject:   email.Subject,
		BodyText:  email.Text,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert notification email: %w", err)
	}
	return id, nil
}

// Get membaca email dan status terkirimnya.
func (s *EmailStore) Get(ctx context.Context, id uuid.UUID) (domain.Email, bool, error) {
	row, err := s.q(ctx).GetNotificationEmail(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Email{}, false, apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("notification email %s", id))
	}
	if err != nil {
		return domain.Email{}, false, fmt.Errorf("get notification email: %w", err)
	}
	return domain.Email{ID: row.ID, To: row.Recipient, Subject: row.Subject, Text: row.BodyText}, row.Status == statusSent, nil
}

// MarkSent menandai email terkirim.
func (s *EmailStore) MarkSent(ctx context.Context, id uuid.UUID) error {
	if err := s.q(ctx).MarkNotificationEmailSent(ctx, sqlcgen.MarkNotificationEmailSentParams{
		ID:     id,
		SentAt: new(s.clock.Now()),
	}); err != nil {
		return fmt.Errorf("mark notification email sent: %w", err)
	}
	return nil
}

// MarkAttemptFailed mencatat percobaan yang gagal.
func (s *EmailStore) MarkAttemptFailed(ctx context.Context, id uuid.UUID, reason string) error {
	if err := s.q(ctx).MarkNotificationEmailAttemptFailed(ctx, sqlcgen.MarkNotificationEmailAttemptFailedParams{
		ID:        id,
		LastError: pgtype.Text{String: reason, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark notification email failed: %w", err)
	}
	return nil
}
