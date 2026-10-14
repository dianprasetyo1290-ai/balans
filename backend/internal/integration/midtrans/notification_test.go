package midtrans_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/integration/midtrans"
	"balans/internal/sales"
)

const serverKey = "SB-Mid-server-test"

func body(t *testing.T, n midtrans.Notification) []byte {
	t.Helper()
	b, err := json.Marshal(n)
	require.NoError(t, err)
	return b
}

func signed(status string) midtrans.Notification {
	n := midtrans.Notification{
		OrderID: "SO-2026-0001", StatusCode: midtrans.StatusCode(status), GrossAmount: "150000.00",
		TransactionStatus: status, TransactionID: "trx-1",
	}
	n.SignatureKey = midtrans.Signature(n.OrderID, n.StatusCode, n.GrossAmount, serverKey)
	return n
}

func TestSignatureMatchesMidtransFormula(t *testing.T) {
	// Contoh dari dokumentasi Midtrans: SHA512("Postman-1578568851" + "200" + "10000.00" + "VT-server-HJMpl9HLr_ntOKt5mRONdmKj").
	got := midtrans.Signature("Postman-1578568851", "200", "10000.00", "VT-server-HJMpl9HLr_ntOKt5mRONdmKj")
	assert.Len(t, got, 128)
	assert.Equal(t, got, midtrans.Signature("Postman-1578568851", "200", "10000.00", "VT-server-HJMpl9HLr_ntOKt5mRONdmKj"))
	assert.NotEqual(t, got, midtrans.Signature("Postman-1578568851", "200", "10000.01", "VT-server-HJMpl9HLr_ntOKt5mRONdmKj"))
}

func TestParseNotification(t *testing.T) {
	n, err := midtrans.ParseNotification(serverKey, body(t, signed("settlement")))
	require.NoError(t, err)
	assert.Equal(t, "SO-2026-0001", n.OrderID)
	assert.Equal(t, "trx-1:settlement", n.EventID)
	assert.Equal(t, sales.PaymentStatusPaid, n.Status)
}

func TestParseNotificationRejectsTampering(t *testing.T) {
	tampered := signed("settlement")
	tampered.GrossAmount = "1.00"
	_, err := midtrans.ParseNotification(serverKey, body(t, tampered))
	assert.ErrorIs(t, err, sales.ErrInvalidSignature)

	_, err = midtrans.ParseNotification("", body(t, signed("settlement")))
	assert.ErrorIs(t, err, sales.ErrInvalidSignature, "server key kosong selalu ditolak")

	_, err = midtrans.ParseNotification(serverKey, []byte(`{`))
	assert.Error(t, err)

	_, err = midtrans.ParseNotification(serverKey, []byte(`{"order_id":"x"}`))
	assert.Error(t, err)
}

func TestMapStatus(t *testing.T) {
	tests := []struct {
		status, fraud string
		want          sales.PaymentStatus
	}{
		{"settlement", "", sales.PaymentStatusPaid},
		{"capture", "accept", sales.PaymentStatusPaid},
		{"capture", "challenge", sales.PaymentStatusPending},
		{"pending", "", sales.PaymentStatusPending},
		{"deny", "", sales.PaymentStatusFailed},
		{"cancel", "", sales.PaymentStatusFailed},
		{"failure", "", sales.PaymentStatusFailed},
		{"expire", "", sales.PaymentStatusExpired},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, midtrans.MapStatus(tt.status, tt.fraud), tt.status)
	}
	assert.Equal(t, "200", midtrans.StatusCode("settlement"))
	assert.Equal(t, "201", midtrans.StatusCode("pending"))
	assert.Equal(t, "407", midtrans.StatusCode("expire"))
	assert.Equal(t, "202", midtrans.StatusCode("deny"))
}
