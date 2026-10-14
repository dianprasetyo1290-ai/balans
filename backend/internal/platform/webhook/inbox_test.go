package webhook_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/webhook"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type processArgs struct {
	EventID uuid.UUID `json:"eventId"`
}

func (processArgs) Kind() string { return "test.webhook_process" }

func newInbox(t *testing.T) *webhook.Inbox {
	t.Helper()
	pool := dbtest.Pool(t)
	ins, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	return webhook.NewInbox(pool, db.NewTxManager(pool), ins, clock.NewFixed(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)))
}

func jobsFor(t *testing.T, eventID uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, dbtest.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'test.webhook_process' AND args->>'eventId' = $1`,
		eventID.String()).Scan(&n))
	return n
}

func TestReceiveStoresOnceAndEnqueuesOnce(t *testing.T) {
	inbox := newInbox(t)
	ctx := context.Background()
	event := webhook.Event{Provider: "midtrans", EventID: uuid.NewString(), EventType: "settlement", Payload: []byte(`{"a":1}`)}
	var created []uuid.UUID
	factory := func(id uuid.UUID) (river.JobArgs, *river.InsertOpts) {
		created = append(created, id)
		return processArgs{EventID: id}, nil
	}

	dup1, err := inbox.Receive(ctx, event, factory)
	require.NoError(t, err)
	dup2, err := inbox.Receive(ctx, event, factory)
	require.NoError(t, err)

	assert.False(t, dup1)
	assert.True(t, dup2)
	require.Len(t, created, 1)
	assert.Equal(t, 1, jobsFor(t, created[0]))

	stored, err := inbox.Get(ctx, created[0])
	require.NoError(t, err)
	assert.Equal(t, event.EventID, stored.Event.EventID)
	assert.JSONEq(t, `{"a":1}`, string(stored.Event.Payload))
	assert.False(t, stored.Processed)

	require.NoError(t, inbox.MarkProcessed(ctx, created[0]))
	require.NoError(t, inbox.MarkProcessed(ctx, created[0]))
	stored, err = inbox.Get(ctx, created[0])
	require.NoError(t, err)
	assert.True(t, stored.Processed)
}

func TestSameEventIDFromOtherProviderIsNotDuplicate(t *testing.T) {
	inbox := newInbox(t)
	ctx := context.Background()
	id := uuid.NewString()
	factory := func(eid uuid.UUID) (river.JobArgs, *river.InsertOpts) { return processArgs{EventID: eid}, nil }

	dup1, err := inbox.Receive(ctx, webhook.Event{Provider: "midtrans", EventID: id, EventType: "x", Payload: []byte(`{}`)}, factory)
	require.NoError(t, err)
	dup2, err := inbox.Receive(ctx, webhook.Event{Provider: "biteship", EventID: id, EventType: "x", Payload: []byte(`{}`)}, factory)
	require.NoError(t, err)

	assert.False(t, dup1)
	assert.False(t, dup2)
}

func TestGetUnknownEvent(t *testing.T) {
	_, err := newInbox(t).Get(context.Background(), uuid.New())
	assert.ErrorIs(t, err, webhook.ErrEventNotFound)
}
