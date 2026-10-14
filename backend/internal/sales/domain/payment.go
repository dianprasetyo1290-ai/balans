// Package domain berisi entity dan value object modul sales.
package domain

import (
	"errors"

	"balans/internal/platform/money"
)

// PaymentStatus adalah status pembayaran menurut Balans, terlepas dari istilah provider.
type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
	PaymentStatusFailed  PaymentStatus = "failed"
	PaymentStatusExpired PaymentStatus = "expired"
)

// ErrInvalidSignature dikembalikan adapter jika signature webhook tidak valid.
var ErrInvalidSignature = errors.New("invalid webhook signature")

// PaymentRequest adalah permintaan membuat transaksi pembayaran. OrderID adalah ID dokumen Balans
// sehingga provider ikut menolak duplikat (Technical Guideline 5.10).
type PaymentRequest struct {
	OrderID     string
	GrossAmount money.Amount
}

// PaymentTransaction adalah hasil pembuatan transaksi di provider.
type PaymentTransaction struct {
	TransactionID string
	RedirectURL   string
}

// PaymentNotification adalah webhook pembayaran yang sudah diverifikasi dan diterjemahkan.
type PaymentNotification struct {
	EventID string
	OrderID string
	Status  PaymentStatus
}
