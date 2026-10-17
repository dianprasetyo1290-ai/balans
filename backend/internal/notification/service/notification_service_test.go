package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/notification/domain"
	"balans/internal/notification/postgres"
	"balans/internal/notification/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func TestInAppNotificationsPerUser(t *testing.T) {
	pool := dbtest.Pool(t)
	fixed := clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC))
	svc := service.NewNotificationService(postgres.NewNotificationStore(pool), fixed)
	alice, bob := uuid.New(), uuid.New()
	asAlice := actor.With(context.Background(), actor.Actor{UserID: alice})
	asBob := actor.With(context.Background(), actor.Actor{UserID: bob})

	msg := domain.Message{Kind: domain.KindApprovalPending, Title: "PO menunggu", Link: "/approvals"}
	require.NoError(t, svc.Notify(context.Background(), []uuid.UUID{alice, alice, uuid.Nil}, msg))
	msg.Title = "PO kedua"
	require.NoError(t, svc.Notify(context.Background(), []uuid.UUID{alice, bob}, msg))
	require.NoError(t, svc.Notify(context.Background(), nil, msg), "tanpa penerima tidak menyimpan apa pun")
	assert.Error(t, svc.Notify(context.Background(), []uuid.UUID{alice}, domain.Message{Kind: "x"}))

	n, err := svc.CountUnread(asAlice)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "duplikat penerima diabaikan")

	list, err := svc.List(asAlice, false, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	assert.Equal(t, "PO kedua", list.Items[0].Title, "terbaru lebih dulu")

	err = svc.MarkRead(asBob, list.Items[0].ID)
	assert.Equal(t, apperr.CodeNotFound, apperr.As(err).Code, "notifikasi user lain tidak bisa ditandai")
	require.NoError(t, svc.MarkRead(asAlice, list.Items[0].ID))
	unread, err := svc.List(asAlice, true, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, unread.Items, 1)
	assert.Equal(t, "PO menunggu", unread.Items[0].Title)

	require.NoError(t, svc.MarkAllRead(asAlice))
	n, err = svc.CountUnread(asAlice)
	require.NoError(t, err)
	assert.Zero(t, n)
	n, err = svc.CountUnread(asBob)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	_, err = svc.CountUnread(context.Background())
	assert.Equal(t, apperr.CodeAuthSessionExpired, apperr.As(err).Code)
}
