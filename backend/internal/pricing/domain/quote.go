package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Rule adalah aturan harga yang menghasilkan harga transaksi, disimpan di baris transaksi sebagai `price_rule`
// (Desain Data Model 4.2). Urutan prioritas BRD 5.18.
type Rule string

const (
	RuleSpecialPrice Rule = "special_price"
	RulePromotion    Rule = "promotion"
	RuleTieredPrice  Rule = "tiered_price"
	RulePriceList    Rule = "price_list"
	RuleBasePrice    Rule = "base_price"
)

var ruleLabels = map[Rule]string{
	RuleSpecialPrice: "Harga khusus", RulePromotion: "Promo", RuleTieredPrice: "Harga bertingkat",
	RulePriceList: "Daftar harga", RuleBasePrice: "Harga dasar",
}

// Label adalah nama aturan untuk UI.
func (r Rule) Label() string { return ruleLabels[r] }

// PriceRequest adalah permintaan harga satu baris transaksi. Date adalah business_date transaksi (TG 5.6):
// harga diambil dari versi yang berlaku pada tanggal itu, bukan harga terbaru.
type PriceRequest struct {
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	CustomerID   *uuid.UUID
	PartnerID    *uuid.UUID
	Date         clock.BusinessDate
}

// Validate memeriksa isian permintaan harga.
func (r PriceRequest) Validate() error {
	f := fieldErrors{}
	if r.ProductID == uuid.Nil {
		f.add("productId", "Pilih produk")
	}
	if !r.Qty.IsPositive() {
		f.add("qty", "Qty harus lebih dari 0")
	}
	if r.Date.IsZero() {
		f.add("date", "Tanggal transaksi wajib diisi")
	}
	if r.SubChannelID != nil && r.ChannelID == nil {
		f.add("subChannelId", "Pilih channel untuk sub-channel ini")
	}
	return f.err()
}

// PriceQuote adalah hasil resolusi harga. Rule dan SourceID disimpan di baris transaksi (`price_rule`,
// `price_source_id`) beserta versi daftar harga yang dipakai, untuk audit dan menjelaskan harga ke user.
type PriceQuote struct {
	UnitPrice money.Amount
	Rule      Rule
	// SourceID adalah ID data yang menghasilkan harga: harga khusus, promo, harga bertingkat, atau versi daftar
	// harga (aturan price_list/base_price).
	SourceID           uuid.UUID
	PriceListID        *uuid.UUID
	PriceListVersionID *uuid.UUID
	PromotionID        *uuid.UUID
	SpecialPriceID     *uuid.UUID
	TieredPriceID      *uuid.UUID
	// RegularPrice adalah harga tanpa promo yang menjadi dasar potongan persen promo.
	RegularPrice *money.Amount
	// MaxRetailPrice adalah HET dari Harga Dasar & HET bila harga berasal dari harga dasar.
	MaxRetailPrice *money.Amount
	// Description menjelaskan asal harga, mis. "Promo PROMO-1111 (10%)".
	Description string
}

// ListPrice adalah harga satu produk dari versi aktif satu daftar harga.
type ListPrice struct {
	PriceListID    uuid.UUID
	VersionID      uuid.UUID
	Code           string
	Name           string
	Price          money.Amount
	MaxRetailPrice *money.Amount
}

// PromotionOffer adalah item promo yang berlaku untuk satu produk.
type PromotionOffer struct {
	PromotionID uuid.UUID
	Code        string
	Name        string
	Item        PromotionItem
}
