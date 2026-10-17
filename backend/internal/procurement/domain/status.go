// Package domain berisi aturan dokumen pengadaan: Purchase Request, Purchase Order (barang/packaging, jasa
// maklon, reprocess), dan uang muka pembelian (BRD 5.2, 5.3; Desain Status Dokumen 2.1 – 2.2). Tanpa DB, HTTP,
// atau waktu sistem.
package domain

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
)

// Status adalah status dokumen pengadaan (kode Desain Status Dokumen bagian 1).
type Status string

const (
	StatusDraft             Status = "draft"
	StatusPendingApproval   Status = "pending_approval"
	StatusApproved          Status = "approved"
	StatusRejected          Status = "rejected"
	StatusPartiallyReceived Status = "partially_received"
	StatusReceived          Status = "received"
	StatusCompleted         Status = "completed"
	StatusClosed            Status = "closed"
	StatusCancelled         Status = "cancelled"
	StatusPosted            Status = "posted"
)

var statusLabels = map[Status]string{
	StatusDraft: "Draft", StatusPendingApproval: "Menunggu Persetujuan", StatusApproved: "Disetujui",
	StatusRejected: "Ditolak", StatusPartiallyReceived: "Diterima Sebagian", StatusReceived: "Diterima",
	StatusCompleted: "Selesai", StatusClosed: "Ditutup", StatusCancelled: "Dibatalkan", StatusPosted: "Diposting",
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
	ActionSubmit        Action = "submit"
	ActionCancel        Action = "cancel"
	ActionRevise        Action = "revise"
	ActionClose         Action = "close"
	ActionCreateOrder   Action = "create_order"
	ActionPrint         Action = "print"
	ActionCreateAdvance Action = "create_advance"
	ActionPost          Action = "post"
)

// transitions adalah tabel transisi status satu jenis dokumen.
type transitions map[Status][]Status

func (t transitions) allowed(from, to Status) bool { return slices.Contains(t[from], to) }

func invalidTransition(doc string, from, to Status) error {
	return apperr.WithMessage(apperr.CodeInvalidStateTransition,
		fmt.Sprintf("%s berstatus %s tidak bisa diubah menjadi %s.", doc, from.Label(), to.Label()))
}

// QtyScale dan batas kuantitas (numeric(18,3)).
const (
	QtyScale  = 3
	maxQtyInt = 15
)

// validQty memeriksa kuantitas positif dengan maks. 3 desimal.
func validQty(q decimal.Decimal) string {
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

// validAmount memeriksa nominal tidak negatif dengan maks. 2 desimal.
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

// MaxLines membatasi jumlah baris per dokumen.
const MaxLines = 200

// MaxReasonLength membatasi alasan tutup/batal.
const MaxReasonLength = 400

// requireReason memeriksa alasan wajib.
func requireReason(reason, field, what string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", apperr.Validation(map[string]string{field: "Alasan " + what + " wajib diisi"})
	case len([]rune(reason)) > MaxReasonLength:
		return "", apperr.Validation(map[string]string{field: "Alasan maksimal 400 karakter"})
	}
	return reason, nil
}

// LineAmount menghitung jumlah baris = qty × harga, dibulatkan ke 2 desimal (half up).
func LineAmount(qty decimal.Decimal, price money.Amount) money.Amount {
	return price.Mul(qty).Round()
}
