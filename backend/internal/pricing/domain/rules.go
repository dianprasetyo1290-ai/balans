package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/softdelete"
)

// PartyType adalah pihak penerima harga khusus.
type PartyType string

const (
	PartyCustomer PartyType = "customer"
	PartyPartner  PartyType = "partner"
)

// Valid memeriksa jenis pihak.
func (t PartyType) Valid() bool { return t == PartyCustomer || t == PartyPartner }

// SpecialPrice adalah harga khusus satu produk untuk satu pelanggan atau mitra (prioritas 1, effective-dated).
type SpecialPrice struct {
	ID        uuid.UUID
	PartyType PartyType
	PartyID   uuid.UUID
	ProductID uuid.UUID
	Price     money.Amount
	Period
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
	softdelete.Mark
}

// SpecialPriceInput adalah isian Tambah Harga Khusus.
type SpecialPriceInput struct {
	PartyType PartyType
	PartyID   uuid.UUID
	ProductID uuid.UUID
	Price     string
	ValidFrom clock.BusinessDate
	ValidTo   *clock.BusinessDate
	Notes     string
}

// NewSpecialPrice membuat harga khusus yang mulai berlaku hari ini atau nanti.
func NewSpecialPrice(in SpecialPriceInput, today clock.BusinessDate) (SpecialPrice, error) {
	f := fieldErrors{}
	s := SpecialPrice{PartyType: in.PartyType, PartyID: in.PartyID, ProductID: in.ProductID}
	if !in.PartyType.Valid() {
		f.add("partyType", "Pilih pelanggan atau mitra")
	}
	if in.PartyID == uuid.Nil {
		f.add("partyId", "Pilih pelanggan atau mitra")
	}
	if in.ProductID == uuid.Nil {
		f.add("productId", "Pilih produk")
	}
	if p := f.amount("priceAmount", in.Price, "Harga khusus", true); p != nil {
		s.Price = *p
	}
	s.Period = Period{ValidFrom: in.ValidFrom, ValidTo: in.ValidTo}
	s.validateNew(f, today)
	s.Notes = f.text("notes", in.Notes, 500, false, "Catatan")
	return s, f.err()
}

// Promotion adalah promo periode per channel (prioritas 2). ChannelID nil = semua channel.
type Promotion struct {
	ID           uuid.UUID
	Code         string
	Name         string
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	ValidFrom    clock.BusinessDate
	ValidTo      clock.BusinessDate
	IsActive     bool
	Description  string
	Items        []PromotionItem
	ItemCount    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
	softdelete.Mark
}

// PromotionItem adalah harga promo satu produk: harga tetap atau potongan persen dari harga normal.
type PromotionItem struct {
	ProductID       uuid.UUID
	Price           *money.Amount
	DiscountPercent *decimal.Decimal
}

// PromotionItemInput adalah isian satu baris promo; isi salah satu dari Price atau DiscountPercent.
type PromotionItemInput struct {
	ProductID       uuid.UUID
	Price           string
	DiscountPercent string
}

// PromotionInput adalah isian Tambah/Ubah Promo. Code hanya dipakai saat membuat.
type PromotionInput struct {
	Code         string
	Name         string
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	ValidFrom    clock.BusinessDate
	ValidTo      clock.BusinessDate
	IsActive     bool
	Description  string
	Items        []PromotionItemInput
}

// Period mengembalikan periode promo.
func (p Promotion) Period() Period {
	to := p.ValidTo
	return Period{ValidFrom: p.ValidFrom, ValidTo: &to}
}

// NewPromotion membuat promo yang mulai hari ini atau nanti.
func NewPromotion(in PromotionInput, today clock.BusinessDate) (Promotion, error) {
	f := fieldErrors{}
	p := Promotion{Code: f.code("code", in.Code, "Kode promo")}
	p.apply(f, in)
	p.applyItems(f, in.Items)
	period := Period{ValidFrom: in.ValidFrom, ValidTo: &in.ValidTo}
	if in.ValidTo.IsZero() {
		f.add("validTo", "Tanggal selesai promo wajib diisi")
		period.ValidTo = nil
	}
	period.validateNew(f, today)
	p.ValidFrom, p.ValidTo = in.ValidFrom, in.ValidTo
	return p, f.err()
}

// Started: promo sudah pernah berlaku (mulai hari ini atau sebelumnya).
func (p Promotion) Started(today clock.BusinessDate) bool { return !today.Before(p.ValidFrom) }

// Update mengubah promo. Promo yang sudah mulai hanya boleh diubah nama, keterangan, status, dan tanggal
// selesai (paling cepat kemarin) agar transaksi yang sudah memakai promo tidak berubah dasarnya.
func (p *Promotion) Update(in PromotionInput, today clock.BusinessDate) error {
	if err := p.EnsureActive(); err != nil {
		return err
	}
	f := fieldErrors{}
	next := *p
	if !p.Started(today) {
		next.apply(f, in)
		next.applyItems(f, in.Items)
		period := Period{ValidFrom: in.ValidFrom, ValidTo: &in.ValidTo}
		if in.ValidTo.IsZero() {
			f.add("validTo", "Tanggal selesai promo wajib diisi")
			period.ValidTo = nil
		}
		period.validateNew(f, today)
		next.ValidFrom, next.ValidTo = in.ValidFrom, in.ValidTo
	} else {
		next.apply(f, PromotionInput{
			Name: in.Name, ChannelID: p.ChannelID, SubChannelID: p.SubChannelID, IsActive: in.IsActive, Description: in.Description,
		})
		if !in.ValidFrom.Equal(p.ValidFrom) {
			f.add("validFrom", "Promo sudah berjalan; tanggal mulai tidak bisa diubah")
		}
		if !SameID(in.ChannelID, p.ChannelID) || !SameID(in.SubChannelID, p.SubChannelID) {
			f.add("channelId", "Promo sudah berjalan; channel tidak bisa diubah")
		}
		if in.Items != nil && !sameItems(in.Items, p.Items) {
			f.add("items", "Promo sudah berjalan; produk dan harga promo tidak bisa diubah. Akhiri promo lalu buat promo baru.")
		}
		switch {
		case in.ValidTo.IsZero():
			f.add("validTo", "Tanggal selesai promo wajib diisi")
		case in.ValidTo.Before(p.ValidFrom):
			f.add("validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai")
		case !in.ValidTo.Equal(p.ValidTo) && in.ValidTo.Before(today.AddDays(-1)):
			f.add("validTo", "Tanggal selesai paling cepat kemarin")
		}
		next.ValidTo = in.ValidTo
	}
	if err := f.err(); err != nil {
		return err
	}
	*p = next
	return nil
}

func (p *Promotion) apply(f fieldErrors, in PromotionInput) {
	p.Name = f.text("name", in.Name, 120, true, "Nama promo")
	p.Description = f.text("description", in.Description, 500, false, "Keterangan")
	p.IsActive = in.IsActive
	if in.SubChannelID != nil && in.ChannelID == nil {
		f.add("subChannelId", "Pilih channel untuk sub-channel ini")
	}
	p.ChannelID, p.SubChannelID = in.ChannelID, in.SubChannelID
}

func (p *Promotion) applyItems(f fieldErrors, items []PromotionItemInput) {
	in := PromotionInput{Items: items}
	if len(in.Items) == 0 {
		f.add("items", "Isi minimal satu produk promo")
	}
	if len(in.Items) > MaxVersionItems {
		f.add("items", fmt.Sprintf("Paling banyak %d produk per promo", MaxVersionItems))
	}
	p.Items = make([]PromotionItem, 0, len(in.Items))
	seen := map[uuid.UUID]int{}
	for i, it := range in.Items {
		prefix := fmt.Sprintf("items.%d.", i)
		if it.ProductID == uuid.Nil {
			f.add(prefix+"productId", "Pilih produk")
		} else if j, dup := seen[it.ProductID]; dup {
			f.add(prefix+"productId", fmt.Sprintf("Produk sama dengan baris %d", j+1))
		}
		seen[it.ProductID] = i
		item := PromotionItem{ProductID: it.ProductID}
		switch {
		case (it.Price == "") == (it.DiscountPercent == ""):
			f.add(prefix+"priceAmount", "Isi harga promo atau potongan persen (salah satu)")
		case it.Price != "":
			item.Price = f.amount(prefix+"priceAmount", it.Price, "Harga promo", true)
		default:
			item.DiscountPercent = f.percent(prefix+"discountPercent", it.DiscountPercent, "Potongan", false, hundred)
		}
		p.Items = append(p.Items, item)
	}
	p.ItemCount = len(p.Items)
}

// SameID membandingkan dua ID opsional.
func SameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameItems(in []PromotionItemInput, cur []PromotionItem) bool {
	if len(in) != len(cur) {
		return false
	}
	byProduct := make(map[uuid.UUID]PromotionItem, len(cur))
	for _, c := range cur {
		byProduct[c.ProductID] = c
	}
	for _, it := range in {
		c, ok := byProduct[it.ProductID]
		if !ok || !sameValue(it.Price, c.Price) || !samePercent(it.DiscountPercent, c.DiscountPercent) {
			return false
		}
	}
	return true
}

func sameValue(s string, a *money.Amount) bool {
	if a == nil || s == "" {
		return (a == nil) == (s == "")
	}
	d, err := decimal.NewFromString(s)
	return err == nil && d.Equal(a.Decimal())
}

func samePercent(s string, p *decimal.Decimal) bool {
	if p == nil || s == "" {
		return (p == nil) == (s == "")
	}
	d, err := decimal.NewFromString(s)
	return err == nil && d.Equal(*p)
}

// AppliesTo memeriksa apakah promo berlaku untuk channel/sub-channel transaksi pada tanggal on.
func (p Promotion) AppliesTo(channelID, subChannelID *uuid.UUID, on clock.BusinessDate) bool {
	if !p.IsActive || p.Deleted() || !p.Period().Contains(on) {
		return false
	}
	if p.ChannelID != nil && (channelID == nil || *p.ChannelID != *channelID) {
		return false
	}
	return p.SubChannelID == nil || (subChannelID != nil && *p.SubChannelID == *subChannelID)
}

// Offer menghitung harga promo satu item dari harga normal (harga tanpa promo).
func (it PromotionItem) Offer(regular money.Amount) money.Amount {
	if it.Price != nil {
		return *it.Price
	}
	return regular.Sub(regular.Percent(*it.DiscountPercent)).RoundRupiah()
}

// NeedsRegular: item potongan persen butuh harga normal.
func (it PromotionItem) NeedsRegular() bool { return it.Price == nil }

// TieredPrice adalah harga bertingkat per qty minimum (prioritas 3). PriceListID nil = semua daftar harga.
type TieredPrice struct {
	ID          uuid.UUID
	PriceListID *uuid.UUID
	ProductID   uuid.UUID
	MinQty      decimal.Decimal
	Price       money.Amount
	Period
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
	softdelete.Mark
}

// TieredPriceInput adalah isian Tambah Harga Bertingkat.
type TieredPriceInput struct {
	PriceListID *uuid.UUID
	ProductID   uuid.UUID
	MinQty      string
	Price       string
	ValidFrom   clock.BusinessDate
	ValidTo     *clock.BusinessDate
	Notes       string
}

// NewTieredPrice membuat harga bertingkat.
func NewTieredPrice(in TieredPriceInput, today clock.BusinessDate) (TieredPrice, error) {
	f := fieldErrors{}
	t := TieredPrice{PriceListID: in.PriceListID, ProductID: in.ProductID}
	if in.ProductID == uuid.Nil {
		f.add("productId", "Pilih produk")
	}
	q, err := decimal.NewFromString(in.MinQty)
	switch {
	case in.MinQty == "":
		f.add("minQty", "Qty minimum wajib diisi")
	case err != nil:
		f.add("minQty", "Qty minimum tidak valid")
	case !q.IsPositive():
		f.add("minQty", "Qty minimum harus lebih dari 0")
	case !q.Equal(q.Round(3)):
		f.add("minQty", "Qty minimum paling banyak 3 angka desimal")
	default:
		t.MinQty = q
	}
	if p := f.amount("priceAmount", in.Price, "Harga", true); p != nil {
		t.Price = *p
	}
	t.Period = Period{ValidFrom: in.ValidFrom, ValidTo: in.ValidTo}
	t.validateNew(f, today)
	t.Notes = f.text("notes", in.Notes, 500, false, "Catatan")
	return t, f.err()
}

// DiscountLimit adalah batas diskon manual satu role (MD-12, effective-dated). Persen 0–100.
type DiscountLimit struct {
	ID                 uuid.UUID
	RoleCode           string
	MaxLinePercent     decimal.Decimal
	MaxDocumentPercent decimal.Decimal
	Period
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
	softdelete.Mark
}

// DiscountLimitInput adalah isian Atur Batas Diskon.
type DiscountLimitInput struct {
	RoleCode           string
	MaxLinePercent     string
	MaxDocumentPercent string
	ValidFrom          clock.BusinessDate
	ValidTo            *clock.BusinessDate
	Notes              string
}

// NewDiscountLimit membuat batas diskon. Role divalidasi service terhadap modul identity.
func NewDiscountLimit(in DiscountLimitInput, today clock.BusinessDate) (DiscountLimit, error) {
	f := fieldErrors{}
	l := DiscountLimit{RoleCode: in.RoleCode}
	if in.RoleCode == "" {
		f.add("roleCode", "Pilih role")
	}
	if p := f.percent("maxLineDiscountPercent", in.MaxLinePercent, "Batas diskon per baris", true, hundred); p != nil {
		l.MaxLinePercent = *p
	}
	if p := f.percent("maxDocumentDiscountPercent", in.MaxDocumentPercent, "Batas diskon per dokumen", true, hundred); p != nil {
		l.MaxDocumentPercent = *p
	}
	l.Period = Period{ValidFrom: in.ValidFrom, ValidTo: in.ValidTo}
	l.validateNew(f, today)
	l.Notes = f.text("notes", in.Notes, 500, false, "Catatan")
	return l, f.err()
}

// OverlapError adalah pesan saat data effective-dated baru beririsan dengan data lain yang tidak bisa ditutup
// otomatis.
func OverlapError(what string, existing Period) error {
	return apperr.Validation(map[string]string{
		"validFrom": fmt.Sprintf("Periode beririsan dengan %s yang berlaku %s; akhiri atau hapus data itu dulu", what, existing.Label()),
	})
}
