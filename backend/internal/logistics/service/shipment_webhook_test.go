package service_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/logistics/domain"
	"balans/internal/logistics/service"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/logger"
	"balans/internal/platform/webhook"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const secret = "test-secret"

// fakeProvider memverifikasi HMAC atas body mentah, sama seperti adapter Biteship.
type fakeProvider struct {
	status domain.ShipmentStatus
}

type payload struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (f *fakeProvider) Provider() string { return "fake-courier" }
func (f *fakeProvider) Rates(context.Context, domain.RateRequest) ([]domain.Rate, error) {
	return nil, nil
}
func (f *fakeProvider) CreateOrder(context.Context, domain.ShipmentOrder) (domain.Shipment, error) {
	return domain.Shipment{}, nil
}
func (f *fakeProvider) Status(context.Context, string) (domain.ShipmentStatus, error) {
	return f.status, nil
}
func (f *fakeProvider) ParseNotification(_ context.Context, signature string, body []byte) (domain.ShipmentNotification, error) {
	if !hmac.Equal([]byte(sign(body)), []byte(signature)) {
		return domain.ShipmentNotification{}, domain.ErrInvalidSignature
	}
	return f.DecodeNotification(body)
}
func (f *fakeProvider) DecodeNotification(body []byte) (domain.ShipmentNotification, error) {
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return domain.ShipmentNotification{}, err
	}
	return domain.ShipmentNotification{EventID: p.OrderID + ":" + p.Status, ReferenceID: p.OrderID, Status: domain.ShipmentStatus(p.Status)}, nil
}

func newService(t *testing.T, provider *fakeProvider) (*service.ShipmentWebhookService, *webhook.Inbox) {
	t.Helper()
	pool := dbtest.Pool(t)
	jobs, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	inbox := webhook.NewInbox(pool, db.NewTxManager(pool), jobs, clock.NewFixed(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)))
	return service.NewShipmentWebhookService(provider, inbox, logger.New(&bytes.Buffer{}, "error")), inbox
}

func eventIDFor(t *testing.T, provider, eventID string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, dbtest.Pool(t).QueryRow(context.Background(),
		`SELECT id FROM webhook_events WHERE provider = $1 AND event_id = $2`, provider, eventID).Scan(&id))
	return id
}

func TestReceiveThenProcessStoredPayload(t *testing.T) {
	provider := &fakeProvider{status: domain.ShipmentStatusDelivered}
	svc, inbox := newService(t, provider)
	ctx := context.Background()
	ref := "DO-" + uuid.NewString()
	// Spasi dan urutan field sengaja dibuat berbeda dari hasil normalisasi jsonb.
	body := []byte(`{"status": "delivered",   "order_id": "` + ref + `"}`)

	dup, err := svc.Receive(ctx, sign(body), body)
	require.NoError(t, err)
	assert.False(t, dup)

	id := eventIDFor(t, "fake-courier", ref+":delivered")
	require.NoError(t, svc.Process(ctx, id), "payload tersimpan diproses tanpa verifikasi ulang byte mentah")

	stored, err := inbox.Get(ctx, id)
	require.NoError(t, err)
	assert.True(t, stored.Processed)
	require.NoError(t, svc.Process(ctx, id), "memproses ulang event selesai tidak gagal")

	dup, err = svc.Receive(ctx, sign(body), body)
	require.NoError(t, err)
	assert.True(t, dup)
}

func TestReceiveRejectsInvalidSignature(t *testing.T) {
	svc, _ := newService(t, &fakeProvider{})
	body := []byte(`{"order_id":"DO-1","status":"delivered"}`)

	_, err := svc.Receive(context.Background(), "salah", body)

	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}

func TestReceiveRejectsUnreadablePayload(t *testing.T) {
	svc, _ := newService(t, &fakeProvider{})
	body := []byte(`{`)

	_, err := svc.Receive(context.Background(), sign(body), body)

	assert.True(t, apperr.HasCode(err, apperr.CodeBadRequest))
}
