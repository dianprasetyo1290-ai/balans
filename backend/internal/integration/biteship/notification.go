// Package biteship menerjemahkan model Biteship ke model Balans (anti-corruption layer, Technical Guideline 5.10).
// Verifikasi notifikasi dipakai bersama oleh adapter asli dan mock.
package biteship

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"balans/internal/logistics"
)

// Provider adalah nama provider di webhook_events.
const Provider = "biteship"

// HeaderSignature adalah header signature webhook.
const HeaderSignature = "X-Biteship-Signature"

// Notification adalah payload webhook status order Biteship.
type Notification struct {
	Event            string `json:"event"`
	OrderID          string `json:"order_id"`
	CourierWaybillID string `json:"courier_waybill_id,omitempty"`
	Status           string `json:"status"`
}

// Signature menghitung HMAC-SHA256 body dengan secret webhook.
func Signature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ParseNotification memverifikasi signature dan menerjemahkan notifikasi ke model Balans.
// order_id Biteship berisi reference_id yang dikirim Balans saat membuat order.
func ParseNotification(secret, signature string, body []byte) (logistics.ShipmentNotification, error) {
	if secret == "" || !hmac.Equal([]byte(Signature(secret, body)), []byte(signature)) {
		return logistics.ShipmentNotification{}, logistics.ErrInvalidSignature
	}
	return DecodeNotification(body)
}

// DecodeNotification menerjemahkan payload yang sudah diverifikasi saat diterima. Payload tersimpan
// di jsonb sehingga byte-nya tidak lagi sama dengan body asli dan tidak bisa diverifikasi ulang.
func DecodeNotification(body []byte) (logistics.ShipmentNotification, error) {
	var n Notification
	if err := json.Unmarshal(body, &n); err != nil {
		return logistics.ShipmentNotification{}, fmt.Errorf("decode biteship notification: %w", err)
	}
	if n.OrderID == "" || n.Status == "" {
		return logistics.ShipmentNotification{}, fmt.Errorf("biteship notification missing required field")
	}
	return logistics.ShipmentNotification{
		EventID:        n.OrderID + ":" + n.Status,
		ReferenceID:    n.OrderID,
		TrackingNumber: n.CourierWaybillID,
		Status:         MapStatus(n.Status),
	}, nil
}

// MapStatus menerjemahkan status Biteship ke status pengiriman Balans.
func MapStatus(status string) logistics.ShipmentStatus {
	switch status {
	case "picked", "picking_up":
		return logistics.ShipmentStatusPickedUp
	case "dropping_off", "on_hold":
		return logistics.ShipmentStatusInTransit
	case "delivered":
		return logistics.ShipmentStatusDelivered
	case "returned", "return_in_transit":
		return logistics.ShipmentStatusReturned
	case "cancelled", "rejected", "disposed":
		return logistics.ShipmentStatusCancelled
	default:
		return logistics.ShipmentStatusConfirmed
	}
}
