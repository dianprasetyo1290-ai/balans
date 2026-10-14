// Package midtrans menerjemahkan model Midtrans ke model Balans (anti-corruption layer, Technical Guideline 5.10).
// Verifikasi notifikasi dipakai bersama oleh adapter asli dan mock, sehingga jalurnya sudah teruji
// sebelum akun asli tersedia.
package midtrans

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"balans/internal/sales"
)

// Provider adalah nama provider di webhook_events.
const Provider = "midtrans"

// Notification adalah payload HTTP notification Midtrans.
type Notification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	TransactionID     string `json:"transaction_id"`
	FraudStatus       string `json:"fraud_status,omitempty"`
}

// Signature menghitung signature_key: SHA512(order_id + status_code + gross_amount + server_key).
func Signature(orderID, statusCode, grossAmount, serverKey string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(sum[:])
}

// ParseNotification memverifikasi signature dan menerjemahkan notifikasi ke model Balans.
func ParseNotification(serverKey string, body []byte) (sales.PaymentNotification, error) {
	n, err := decode(body)
	if err != nil {
		return sales.PaymentNotification{}, err
	}
	want := Signature(n.OrderID, n.StatusCode, n.GrossAmount, serverKey)
	if serverKey == "" || subtle.ConstantTimeCompare([]byte(want), []byte(n.SignatureKey)) != 1 {
		return sales.PaymentNotification{}, sales.ErrInvalidSignature
	}
	return translate(n), nil
}

// DecodeNotification menerjemahkan payload yang sudah diverifikasi saat diterima.
func DecodeNotification(body []byte) (sales.PaymentNotification, error) {
	n, err := decode(body)
	if err != nil {
		return sales.PaymentNotification{}, err
	}
	return translate(n), nil
}

func decode(body []byte) (Notification, error) {
	var n Notification
	if err := json.Unmarshal(body, &n); err != nil {
		return Notification{}, fmt.Errorf("decode midtrans notification: %w", err)
	}
	if n.OrderID == "" || n.TransactionID == "" || n.TransactionStatus == "" {
		return Notification{}, fmt.Errorf("midtrans notification missing required field")
	}
	return n, nil
}

func translate(n Notification) sales.PaymentNotification {
	return sales.PaymentNotification{
		EventID: n.TransactionID + ":" + n.TransactionStatus,
		OrderID: n.OrderID,
		Status:  MapStatus(n.TransactionStatus, n.FraudStatus),
	}
}

// MapStatus menerjemahkan transaction_status Midtrans ke status pembayaran Balans.
func MapStatus(transactionStatus, fraudStatus string) sales.PaymentStatus {
	switch transactionStatus {
	case "settlement":
		return sales.PaymentStatusPaid
	case "capture":
		if fraudStatus == "challenge" {
			return sales.PaymentStatusPending
		}
		return sales.PaymentStatusPaid
	case "deny", "cancel", "failure":
		return sales.PaymentStatusFailed
	case "expire":
		return sales.PaymentStatusExpired
	default:
		return sales.PaymentStatusPending
	}
}

// StatusCode mengembalikan status_code Midtrans untuk transaction_status, dipakai mock.
func StatusCode(transactionStatus string) string {
	switch transactionStatus {
	case "settlement", "capture":
		return "200"
	case "pending":
		return "201"
	case "expire":
		return "407"
	default:
		return "202"
	}
}
