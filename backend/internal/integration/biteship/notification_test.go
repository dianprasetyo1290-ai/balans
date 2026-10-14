package biteship_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/integration/biteship"
	"balans/internal/logistics"
)

const secret = "webhook-secret"

func TestParseNotification(t *testing.T) {
	body := []byte(`{"event":"order.status","order_id":"DO-2026-0001","courier_waybill_id":"MOCK-1","status":"delivered"}`)

	n, err := biteship.ParseNotification(secret, biteship.Signature(secret, body), body)

	require.NoError(t, err)
	assert.Equal(t, "DO-2026-0001", n.ReferenceID)
	assert.Equal(t, "MOCK-1", n.TrackingNumber)
	assert.Equal(t, "DO-2026-0001:delivered", n.EventID)
	assert.Equal(t, logistics.ShipmentStatusDelivered, n.Status)
}

func TestParseNotificationRejectsInvalid(t *testing.T) {
	body := []byte(`{"event":"order.status","order_id":"DO-1","status":"delivered"}`)

	_, err := biteship.ParseNotification(secret, "salah", body)
	assert.ErrorIs(t, err, logistics.ErrInvalidSignature)

	_, err = biteship.ParseNotification("", biteship.Signature("", body), body)
	assert.ErrorIs(t, err, logistics.ErrInvalidSignature, "secret kosong selalu ditolak")

	broken := []byte(`{`)
	_, err = biteship.ParseNotification(secret, biteship.Signature(secret, broken), broken)
	assert.Error(t, err)

	missing := []byte(`{"event":"order.status"}`)
	_, err = biteship.ParseNotification(secret, biteship.Signature(secret, missing), missing)
	assert.Error(t, err)
}

func TestMapStatus(t *testing.T) {
	tests := map[string]logistics.ShipmentStatus{
		"confirmed":         logistics.ShipmentStatusConfirmed,
		"allocated":         logistics.ShipmentStatusConfirmed,
		"picking_up":        logistics.ShipmentStatusPickedUp,
		"picked":            logistics.ShipmentStatusPickedUp,
		"dropping_off":      logistics.ShipmentStatusInTransit,
		"on_hold":           logistics.ShipmentStatusInTransit,
		"delivered":         logistics.ShipmentStatusDelivered,
		"return_in_transit": logistics.ShipmentStatusReturned,
		"returned":          logistics.ShipmentStatusReturned,
		"cancelled":         logistics.ShipmentStatusCancelled,
		"rejected":          logistics.ShipmentStatusCancelled,
	}
	for in, want := range tests {
		assert.Equal(t, want, biteship.MapStatus(in), in)
	}
}
