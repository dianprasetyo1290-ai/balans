package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"balans/internal/notification/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

type notificationStore interface {
	Insert(ctx context.Context, userIDs []uuid.UUID, m domain.Message) error
	List(ctx context.Context, userID uuid.UUID, unreadOnly bool, page paging.Page) (paging.Result[domain.Notification], error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID, at time.Time) error
	MarkAllRead(ctx context.Context, userID uuid.UUID, at time.Time) error
}

// NotificationService mengelola notifikasi in-app (NFR-11). Setiap user hanya melihat notifikasinya sendiri.
type NotificationService struct {
	store notificationStore
	clock clock.Clock
}

// NewNotificationService membuat NotificationService.
func NewNotificationService(store notificationStore, c clock.Clock) *NotificationService {
	return &NotificationService{store: store, clock: c}
}

// Notify menyimpan notifikasi untuk para penerima di transaksi aktif, sehingga notifikasi hanya ada
// jika transaksi bisnis yang memicunya commit.
func (s *NotificationService) Notify(ctx context.Context, userIDs []uuid.UUID, m domain.Message) error {
	if err := m.Validate(); err != nil {
		return err
	}
	recipients := domain.Recipients(userIDs)
	if len(recipients) == 0 {
		return nil
	}
	return s.store.Insert(ctx, recipients, m)
}

func currentUser(ctx context.Context) (uuid.UUID, error) {
	a, ok := actor.From(ctx)
	if !ok || a.UserID == uuid.Nil {
		return uuid.Nil, apperr.New(apperr.CodeAuthSessionExpired)
	}
	return a.UserID, nil
}

// List mengembalikan notifikasi user yang login.
func (s *NotificationService) List(ctx context.Context, unreadOnly bool, page paging.Page) (paging.Result[domain.Notification], error) {
	userID, err := currentUser(ctx)
	if err != nil {
		return paging.Result[domain.Notification]{}, err
	}
	return s.store.List(ctx, userID, unreadOnly, page)
}

// CountUnread menghitung notifikasi belum dibaca untuk badge.
func (s *NotificationService) CountUnread(ctx context.Context) (int64, error) {
	userID, err := currentUser(ctx)
	if err != nil {
		return 0, err
	}
	return s.store.CountUnread(ctx, userID)
}

// MarkRead menandai satu notifikasi sudah dibaca.
func (s *NotificationService) MarkRead(ctx context.Context, id uuid.UUID) error {
	userID, err := currentUser(ctx)
	if err != nil {
		return err
	}
	return s.store.MarkRead(ctx, userID, id, s.clock.Now())
}

// MarkAllRead menandai semua notifikasi user sudah dibaca.
func (s *NotificationService) MarkAllRead(ctx context.Context) error {
	userID, err := currentUser(ctx)
	if err != nil {
		return err
	}
	return s.store.MarkAllRead(ctx, userID, s.clock.Now())
}
