package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Settlement marketplace tahap 1 (S-403, ACC-06): laporan settlement diimpor, setiap baris dicocokkan dengan order
// marketplace yang sudah diserahkan ke kurir (piutang A-10), lalu diposting sebagai jurnal A-10s.

// SettlementStatus adalah status settlement.
type SettlementStatus string

const (
	SettlementDraft     SettlementStatus = "draft"
	SettlementPosted    SettlementStatus = "posted"
	SettlementCancelled SettlementStatus = "cancelled"
)

// Label adalah label status (Desain Status Dokumen).
func (s SettlementStatus) Label() string {
	switch s {
	case SettlementDraft:
		return "Draft"
	case SettlementPosted:
		return "Diposting"
	case SettlementCancelled:
		return "Dibatalkan"
	}
	return string(s)
}

// Settlement adalah satu pencairan dana marketplace.
type Settlement struct {
	ID              uuid.UUID
	ChannelID       uuid.UUID
	SubChannelID    uuid.UUID
	MarketplaceCode string
	PayoutReference string
	SettlementDate  clock.BusinessDate
	BankAccountCode string
	Status          SettlementStatus
	LineCount       int
	Settled         money.Amount
	Fee             money.Amount
	Promo           money.Amount
	Net             money.Amount
	JournalEntryID  *uuid.UUID
	PostedAt        *time.Time
	CancelReason    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// Label adalah label tampilan, mis. "Settlement shopee PAYOUT-1".
func (s Settlement) Label() string {
	return "Settlement " + s.MarketplaceCode + " " + s.PayoutReference
}

// Aksi settlement.
const (
	SettlementActionPost   Action = "post"
	SettlementActionCancel Action = "cancel"
)

// Actions mengembalikan aksi yang mungkin pada status sekarang.
func (s Settlement) Actions() []Action {
	if s.Status == SettlementDraft && s.LineCount > 0 {
		return []Action{SettlementActionPost, SettlementActionCancel}
	}
	if s.Status == SettlementDraft {
		return []Action{SettlementActionCancel}
	}
	return nil
}

// CanPost menolak posting selain draft berisi baris.
func (s Settlement) CanPost() error {
	if s.Status != SettlementDraft {
		return apperr.InvalidStateTransition("marketplace_settlement", string(s.Status), string(SettlementPosted))
	}
	if s.LineCount == 0 || !s.Settled.IsPositive() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Settlement belum berisi order.")
	}
	return nil
}

// CanCancel menolak pembatalan selain draft.
func (s Settlement) CanCancel(reason string) error {
	if s.Status != SettlementDraft {
		return apperr.InvalidStateTransition("marketplace_settlement", string(s.Status), string(SettlementCancelled))
	}
	if strings.TrimSpace(reason) == "" {
		return apperr.Validation(map[string]string{"reason": "Alasan pembatalan wajib diisi"})
	}
	return nil
}

// SettlementLine adalah satu order dalam settlement.
type SettlementLine struct {
	LineNo          int
	OrderID         uuid.UUID
	ExternalOrderID string
	OrderNumber     string
	Receivable      money.Amount
	Fee             money.Amount
	Promo           money.Amount
	Net             money.Amount
	Settled         money.Amount
	// PostedSettled adalah total settlement diposting untuk order ini (semua settlement).
	PostedSettled money.Amount
}

// Outstanding adalah sisa piutang order setelah settlement diposting.
func (l SettlementLine) Outstanding() money.Amount { return l.Receivable.Sub(l.PostedSettled) }

// SettlementAmounts adalah nilai satu baris laporan settlement: dana cair (net), fee marketplace, dan promo yang
// ditanggung penjual. Settled = net + fee + promo = pengurang piutang marketplace.
type SettlementAmounts struct {
	Net   money.Amount
	Fee   money.Amount
	Promo money.Amount
}

// Settled adalah jumlah pengurang piutang.
func (a SettlementAmounts) Settled() money.Amount { return a.Net.Add(a.Fee).Add(a.Promo) }

// MarketplaceOrderRef adalah order marketplace yang dicocokkan dengan baris settlement.
type MarketplaceOrderRef struct {
	ID           uuid.UUID
	Number       string
	ChannelID    uuid.UUID
	SubChannelID uuid.UUID
	Total        money.Amount
	Status       Status
	HandedOver   bool
	// AlreadySettled adalah settlement draft & diposting yang sudah mencakup order ini.
	AlreadySettled money.Amount
}

// CheckSettlementLine memeriksa satu baris settlement terhadap order: order sudah diserahkan ke kurir (piutang
// terbentuk), nilai tidak negatif dan > 0, dan total settlement tidak melebihi piutang order.
func CheckSettlementLine(o MarketplaceOrderRef, a SettlementAmounts) map[string]string {
	fields := map[string]string{}
	if o.Status == StatusCancelled || !o.HandedOver {
		fields["order_no"] = "Order belum diserahkan ke kurir; piutang marketplace belum terbentuk"
		return fields
	}
	if a.Net.IsNegative() || a.Fee.IsNegative() || a.Promo.IsNegative() {
		fields["net"] = "Nilai tidak boleh negatif"
		return fields
	}
	settled := a.Settled()
	if !settled.IsPositive() {
		fields["net"] = "Dana cair + fee + promo harus lebih dari 0"
		return fields
	}
	if remaining := o.Total.Sub(o.AlreadySettled); settled.Cmp(remaining) > 0 {
		fields["net"] = "Total settlement " + settled.String() + " melebihi sisa piutang order " + remaining.String()
	}
	return fields
}
