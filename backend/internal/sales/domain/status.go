package domain

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
)

// Status adalah status dokumen penjualan (kode Desain Status Dokumen bagian 1).
type Status string

const (
	StatusDraft           Status = "draft"
	StatusPendingApproval Status = "pending_approval"
	StatusAwaitingPayment Status = "awaiting_payment"
	StatusPartiallyPaid   Status = "partially_paid"
	StatusPaid            Status = "paid"
	StatusInProgress      Status = "in_progress"
	StatusInTransit       Status = "in_transit"
	StatusCompleted       Status = "completed"
	StatusCancelled       Status = "cancelled"
	StatusExpired         Status = "expired"
	StatusPosted          Status = "posted"
)

var statusLabels = map[Status]string{
	StatusDraft: "Draft", StatusPendingApproval: "Menunggu Persetujuan", StatusAwaitingPayment: "Menunggu Pembayaran",
	StatusPartiallyPaid: "Dibayar Sebagian", StatusPaid: "Lunas", StatusInProgress: "Diproses",
	StatusInTransit: "Dalam Pengiriman", StatusCompleted: "Selesai", StatusCancelled: "Dibatalkan",
	StatusExpired: "Kedaluwarsa", StatusPosted: "Diposting",
}

// Label mengembalikan label UI status.
func (s Status) Label() string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return string(s)
}

// Action adalah aksi bernama pada dokumen (allowedActions, Technical Guideline 5.4).
type Action string

const (
	ActionEdit          Action = "edit"
	ActionConfirm       Action = "confirm"
	ActionCancel        Action = "cancel"
	ActionUploadProof   Action = "upload_payment_proof"
	ActionConfirmPay    Action = "confirm_payment"
	ActionPrint         Action = "print"
	ActionRecordPayment Action = "record_payment"
	ActionShareWhatsApp Action = "share_whatsapp"
)

type transitions map[Status][]Status

func (t transitions) allowed(from, to Status) bool { return slices.Contains(t[from], to) }

func invalidTransition(doc string, from, to Status) error {
	return apperr.WithMessage(apperr.CodeInvalidStateTransition,
		fmt.Sprintf("%s berstatus %s tidak bisa diubah menjadi %s.", doc, from.Label(), to.Label()))
}

// QtyScale adalah jumlah desimal qty (numeric(18,3)).
const QtyScale = 3

const maxQtyInt = 15

// ValidQty memeriksa qty positif maks. 3 desimal; "" bila valid.
func ValidQty(q decimal.Decimal) string {
	switch {
	case !q.IsPositive():
		return "Qty harus lebih dari nol"
	case !q.Equal(q.Round(QtyScale)):
		return "Qty maksimal 3 angka di belakang koma"
	case len(q.Truncate(0).String()) > maxQtyInt:
		return "Qty terlalu besar"
	}
	return ""
}

func validAmount(a money.Amount, positive bool) string {
	switch {
	case a.IsNegative():
		return "Nominal tidak boleh negatif"
	case positive && !a.IsPositive():
		return "Nominal harus lebih dari nol"
	case !a.Equal(a.Round()):
		return "Nominal maksimal 2 angka di belakang koma"
	}
	return ""
}

// Batas panjang field.
const (
	MaxLines        = 200
	MaxReasonLength = 400
	MaxNoteLength   = 1000
	MaxTextLength   = 200
)

func requireReason(reason, what string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", apperr.Validation(map[string]string{"reason": "Alasan " + what + " wajib diisi"})
	case len([]rune(reason)) > MaxReasonLength:
		return "", apperr.Validation(map[string]string{"reason": "Alasan maksimal 400 karakter"})
	}
	return reason, nil
}

func tooLong(s string, n int) bool { return len([]rune(s)) > n }
