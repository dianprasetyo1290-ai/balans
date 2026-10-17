package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/notification/domain"
	"balans/internal/notification/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// NotificationStore menyimpan notifikasi in-app di tabel notifications.
type NotificationStore struct {
	pool *pgxpool.Pool
}

// NewNotificationStore membuat NotificationStore.
func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

func (s *NotificationStore) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Insert menyimpan satu notifikasi untuk setiap penerima di transaksi aktif.
func (s *NotificationStore) Insert(ctx context.Context, userIDs []uuid.UUID, m domain.Message) error {
	for _, id := range userIDs {
		if err := s.q(ctx).InsertNotification(ctx, sqlcgen.InsertNotificationParams{
			UserID: id, Kind: m.Kind, Title: m.Title, Body: m.Body, Link: m.Link,
		}); err != nil {
			return fmt.Errorf("insert notification: %w", err)
		}
	}
	return nil
}

// List mengembalikan notifikasi user, terbaru lebih dulu.
func (s *NotificationStore) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, page paging.Page) (paging.Result[domain.Notification], error) {
	rows, err := s.q(ctx).ListNotifications(ctx, sqlcgen.ListNotificationsParams{
		UserID: userID, UnreadOnly: unreadOnly, RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Notification]{}, fmt.Errorf("list notifications: %w", err)
	}
	total, err := s.q(ctx).CountNotifications(ctx, sqlcgen.CountNotificationsParams{UserID: userID, UnreadOnly: unreadOnly})
	if err != nil {
		return paging.Result[domain.Notification]{}, fmt.Errorf("count notifications: %w", err)
	}
	items := make([]domain.Notification, len(rows))
	for i, r := range rows {
		items[i] = domain.Notification{
			ID: r.ID, UserID: r.UserID, Kind: r.Kind, Title: r.Title, Body: r.Body, Link: r.Link,
			ReadAt: r.ReadAt, CreatedAt: r.CreatedAt,
		}
	}
	return paging.Result[domain.Notification]{Items: items, Page: page, Total: total}, nil
}

// CountUnread menghitung notifikasi yang belum dibaca.
func (s *NotificationStore) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := s.q(ctx).CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return n, nil
}

// MarkRead menandai satu notifikasi milik user sudah dibaca. Notifikasi user lain dianggap tidak ada.
func (s *NotificationStore) MarkRead(ctx context.Context, userID, id uuid.UUID, at time.Time) error {
	n, err := s.q(ctx).MarkNotificationRead(ctx, sqlcgen.MarkNotificationReadParams{ID: id, UserID: userID, ReadAt: &at})
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeNotFound)
	}
	return nil
}

// MarkAllRead menandai semua notifikasi user sudah dibaca.
func (s *NotificationStore) MarkAllRead(ctx context.Context, userID uuid.UUID, at time.Time) error {
	if err := s.q(ctx).MarkAllNotificationsRead(ctx, sqlcgen.MarkAllNotificationsReadParams{UserID: userID, ReadAt: &at}); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}
