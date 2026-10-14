package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/logger"
	"balans/internal/platform/webhook"
	"balans/internal/sales/domain"
	"balans/internal/sales/service"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type notification struct {
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
	Signature string `json:"signature"`
}

type fakeGateway struct {
	confirmed domain.PaymentStatus
	statusErr error
	checked   []string
}

func (f *fakeGateway) Provider() string { return "fake-pay" }
func (f *fakeGateway) CreateTransaction(context.Context, domain.PaymentRequest) (domain.PaymentTransaction, error) {
	return domain.PaymentTransaction{}, nil
}
func (f *fakeGateway) Status(_ context.Context, orderID string) (domain.PaymentStatus, error) {
	f.checked = append(f.checked, orderID)
	return f.confirmed, f.statusErr
}
func (f *fakeGateway) ParseNotification(_ context.Context, body []byte) (domain.PaymentNotification, error) {
	n, err := f.DecodeNotification(body)
	if err != nil {
		return n, err
	}
	var raw notification
	_ = json.Unmarshal(body, &raw)
	if raw.Signature != "valid" {
		return domain.PaymentNotification{}, domain.ErrInvalidSignature
	}
	return n, nil
}
func (f *fakeGateway) DecodeNotification(body []byte) (domain.PaymentNotification, error) {
	var raw notification
	if err := json.Unmarshal(body, &raw); err != nil {
		return domain.PaymentNotification{}, err
	}
	return domain.PaymentNotification{EventID: raw.OrderID + ":" + raw.Status, OrderID: raw.OrderID, Status: domain.PaymentStatus(raw.Status)}, nil
}

func newService(t *testing.T, gw *fakeGateway) *service.PaymentWebhookService {
	t.Helper()
	pool := dbtest.Pool(t)
	jobs, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	inbox := webhook.NewInbox(pool, db.NewTxManager(pool), jobs, clock.NewFixed(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)))
	return service.NewPaymentWebhookService(gw, inbox, logger.New(&bytes.Buffer{}, "error"))
}

func storedEventID(t *testing.T, eventID string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, dbtest.Pool(t).QueryRow(context.Background(),
		`SELECT id FROM webhook_events WHERE provider = 'fake-pay' AND event_id = $1`, eventID).Scan(&id))
	return id
}

func TestPaymentWebhookConfirmsStatusBeforeTrusting(t *testing.T) {
	gw := &fakeGateway{confirmed: domain.PaymentStatusPaid}
	svc := newService(t, gw)
	ctx := context.Background()
	order := "SO-" + uuid.NewString()
	body := []byte(`{"order_id":"` + order + `","status":"paid","signature":"valid"}`)

	dup, err := svc.Receive(ctx, body)
	require.NoError(t, err)
	assert.False(t, dup)

	id := storedEventID(t, order+":paid")
	require.NoError(t, svc.Process(ctx, id))
	assert.Equal(t, []string{order}, gw.checked, "status dikonfirmasi ulang ke provider")

	require.NoError(t, svc.Process(ctx, id))
	assert.Len(t, gw.checked, 1, "event yang sudah diproses tidak dikonfirmasi ulang")
}

func TestPaymentWebhookProcessRetriesWhenProviderDown(t *testing.T) {
	gw := &fakeGateway{statusErr: errors.New("timeout")}
	svc := newService(t, gw)
	ctx := context.Background()
	order := "SO-" + uuid.NewString()

	_, err := svc.Receive(ctx, []byte(`{"order_id":"`+order+`","status":"paid","signature":"valid"}`))
	require.NoError(t, err)

	err = svc.Process(ctx, storedEventID(t, order+":paid"))
	assert.Error(t, err, "error dikembalikan agar job di-retry river")
}

func TestPaymentWebhookRejectsInvalidSignature(t *testing.T) {
	svc := newService(t, &fakeGateway{})

	_, err := svc.Receive(context.Background(), []byte(`{"order_id":"x","status":"paid","signature":"palsu"}`))

	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}
