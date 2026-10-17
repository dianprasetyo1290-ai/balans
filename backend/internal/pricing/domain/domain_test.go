package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/pricing/domain"
)

var today = clock.NewBusinessDate(2026, 10, 23)

func day(offset int) clock.BusinessDate { return today.AddDays(offset) }

func dayPtr(offset int) *clock.BusinessDate {
	d := day(offset)
	return &d
}

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr[T any](v T) *T { return &v }

func TestPeriod(t *testing.T) {
	open := domain.Period{ValidFrom: day(0)}
	closed := domain.Period{ValidFrom: day(0), ValidTo: dayPtr(10)}
	assert.True(t, open.Contains(day(100)))
	assert.False(t, open.Contains(day(-1)))
	assert.True(t, closed.Contains(day(10)))
	assert.False(t, closed.Contains(day(11)))

	assert.True(t, open.Overlaps(domain.Period{ValidFrom: day(50), ValidTo: dayPtr(60)}))
	assert.False(t, closed.Overlaps(domain.Period{ValidFrom: day(11)}))
	assert.True(t, closed.Overlaps(domain.Period{ValidFrom: day(-5), ValidTo: dayPtr(0)}))

	assert.Equal(t, domain.PeriodScheduled, closed.StatusOn(day(-1)))
	assert.Equal(t, domain.PeriodActive, closed.StatusOn(day(3)))
	assert.Equal(t, domain.PeriodExpired, closed.StatusOn(day(11)))
	assert.Equal(t, "mulai 2026-10-23", open.Label())
	assert.Equal(t, "2026-10-23 s.d. 2026-11-02", closed.Label())

	assert.True(t, open.CanDelete(today))
	assert.False(t, open.CanDelete(day(1)))

	p, ok := open.Supersede(day(5))
	require.True(t, ok)
	assert.Equal(t, day(4), *p.ValidTo)
	_, ok = closed.Supersede(day(5))
	assert.False(t, ok, "periode bertanggal akhir tidak ditutup otomatis")
	_, ok = open.Supersede(day(0))
	assert.False(t, ok, "periode yang mulai bersamaan tidak ditutup otomatis")
}

func TestPeriodEndAt(t *testing.T) {
	started := domain.Period{ValidFrom: day(-10)}
	tests := []struct {
		name  string
		p     domain.Period
		to    clock.BusinessDate
		field string
	}{
		{"wajib", started, clock.BusinessDate{}, "validTo"},
		{"periode lampau tidak diubah", started, day(-2), "validTo"},
		{"sebelum mulai", domain.Period{ValidFrom: day(5)}, day(2), "validTo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.p.EndAt(tc.to, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}
	p, err := started.EndAt(day(-1), today)
	require.NoError(t, err)
	assert.Equal(t, day(-1), *p.ValidTo)
}

func TestNewPriceList(t *testing.T) {
	channel, partner, ref := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name  string
		in    domain.PriceListInput
		field string
	}{
		{"kode wajib", domain.PriceListInput{Name: "X", Type: domain.TypeChannel, ChannelID: &channel}, "code"},
		{"kode huruf kecil dinormalkan tapi karakter salah", domain.PriceListInput{Code: "a b", Name: "X", Type: domain.TypeChannel, ChannelID: &channel}, "code"},
		{"kode terlalu panjang", domain.PriceListInput{Code: "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", Name: "X", Type: domain.TypeChannel, ChannelID: &channel}, "code"},
		{"jenis tidak dikenal", domain.PriceListInput{Code: "A", Name: "X", Type: "x"}, "priceListType"},
		{"dasar tidak dibuat ulang", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeBase}, "priceListType"},
		{"reseller fase 2", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeReseller, ResellerLevel: "gold"}, "priceListType"},
		{"nama wajib", domain.PriceListInput{Code: "A", Type: domain.TypeEmployee}, "name"},
		{"channel wajib", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeChannel}, "channelId"},
		{"vending butuh channel", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeVending}, "channelId"},
		{"mitra wajib", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypePartner}, "partnerId"},
		{"basis tidak dikenal", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeEmployee, Basis: "x"}, "basis"},
		{"acuan wajib", domain.PriceListInput{Code: "A", Name: "X", Type: domain.TypeEmployee, Basis: domain.BasisPercent}, "referencePriceListId"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewPriceList(tc.in)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}

	p, err := domain.NewPriceList(domain.PriceListInput{
		Code: " mt-01 ", Name: " Modern Trade ", Type: domain.TypeChannel, ChannelID: &channel, PartnerID: &partner, IsActive: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "MT-01", p.Code)
	assert.Equal(t, "Modern Trade", p.Name)
	assert.Nil(t, p.PartnerID, "cakupan channel tidak menyimpan mitra")
	assert.Equal(t, domain.BasisFixed, p.Basis)
	assert.True(t, p.Usable())
	assert.Empty(t, p.DeleteError())

	e, err := domain.NewPriceList(domain.PriceListInput{
		Code: "KARYAWAN", Name: "Karyawan", Type: domain.TypeEmployee, Basis: domain.BasisPercent, ReferenceID: &ref, IsActive: true,
	})
	require.NoError(t, err)
	assert.Equal(t, &ref, e.ReferenceID)
	assert.Equal(t, "Karyawan", domain.TypeEmployee.Label())

	e.ID = ref
	err = e.Update(domain.PriceListInput{Name: "Karyawan", Basis: domain.BasisPercent, ReferenceID: &ref, IsActive: true})
	assert.Contains(t, fields(t, err), "referencePriceListId")
	require.NoError(t, e.Update(domain.PriceListInput{Name: "Karyawan baru", IsActive: false}))
	assert.Equal(t, domain.BasisFixed, e.Basis)
	assert.False(t, e.Usable())

	base := domain.PriceList{Type: domain.TypeBase, Basis: domain.BasisFixed, IsActive: true}
	assert.NotEmpty(t, base.DeleteError())
	assert.Contains(t, fields(t, base.Update(domain.PriceListInput{Name: "Dasar"})), "isActive")
	assert.Contains(t, fields(t, base.Update(domain.PriceListInput{Name: "Dasar", IsActive: true, Basis: domain.BasisPercent, ReferenceID: &ref})), "basis")

	reseller := domain.PriceList{Type: domain.TypeReseller}
	assert.Contains(t, fields(t, reseller.Update(domain.PriceListInput{Name: "R"})), "resellerLevel")

	deleted := p
	require.NoError(t, deleted.Delete(time.Now(), uuid.New()))
	assert.Error(t, deleted.Update(domain.PriceListInput{Name: "X", IsActive: true}))
}

func TestValidateReference(t *testing.T) {
	p := domain.PriceList{}
	assert.NoError(t, p.ValidateReference(domain.PriceList{Basis: domain.BasisFixed}))
	assert.Contains(t, fields(t, p.ValidateReference(domain.PriceList{Basis: domain.BasisPercent})), "referencePriceListId")
	deleted := domain.PriceList{Basis: domain.BasisFixed}
	require.NoError(t, deleted.Delete(time.Now(), uuid.New()))
	assert.Contains(t, fields(t, p.ValidateReference(deleted)), "referencePriceListId")
}

func fixedList() domain.PriceList {
	return domain.PriceList{ID: uuid.New(), Code: "WEB", Name: "Website", Type: domain.TypeChannel, Basis: domain.BasisFixed, IsActive: true}
}

func TestVersionValidation(t *testing.T) {
	list := fixedList()
	base := domain.PriceList{ID: uuid.New(), Code: "HARGA-DASAR", Name: "Dasar", Type: domain.TypeBase, Basis: domain.BasisFixed, IsActive: true}
	percent := domain.PriceList{ID: uuid.New(), Code: "KRY", Name: "Karyawan", Type: domain.TypeEmployee, Basis: domain.BasisPercent, IsActive: true}
	p1, p2 := uuid.New(), uuid.New()
	tests := []struct {
		name  string
		list  domain.PriceList
		in    domain.VersionInput
		field string
	}{
		{"tanggal mulai wajib", list, domain.VersionInput{}, "validFrom"},
		{"tidak mundur", list, domain.VersionInput{ValidFrom: day(-1)}, "validFrom"},
		{"akhir sebelum mulai", list, domain.VersionInput{ValidFrom: day(5), ValidTo: dayPtr(4)}, "validTo"},
		{"produk wajib", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{Price: "1"}}}, "items.0.productId"},
		{"produk ganda", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "1"}, {ProductID: p1, Price: "2"}}}, "items.1.productId"},
		{"harga wajib", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1}}}, "items.0.priceAmount"},
		{"harga negatif", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "-1"}}}, "items.0.priceAmount"},
		{"harga tidak valid", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "abc"}}}, "items.0.priceAmount"},
		{"harga 3 desimal", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "1.005"}}}, "items.0.priceAmount"},
		{"HET hanya di dasar", list, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "1", MaxRetailPrice: "2"}}}, "items.0.maxRetailPriceAmount"},
		{"HET di bawah harga dasar", base, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: p1, Price: "10", MaxRetailPrice: "9"}}}, "items.0.maxRetailPriceAmount"},
		{"persen wajib", percent, domain.VersionInput{ValidFrom: day(0)}, "percent"},
		{"persen nol", percent, domain.VersionInput{ValidFrom: day(0), Percent: "0"}, "percent"},
		{"persen tidak valid", percent, domain.VersionInput{ValidFrom: day(0), Percent: "x"}, "percent"},
		{"persen 5 desimal", percent, domain.VersionInput{ValidFrom: day(0), Percent: "90.00001"}, "percent"},
		{"basis persen tanpa item", percent, domain.VersionInput{ValidFrom: day(0), Percent: "90", Items: []domain.ItemInput{{ProductID: p2, Price: "1"}}}, "items"},
		{"catatan panjang", list, domain.VersionInput{ValidFrom: day(0), Notes: string(make([]byte, 1001))}, "notes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewVersion(tc.list, 1, tc.in, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}

	v, err := domain.NewVersion(base, 2, domain.VersionInput{
		ValidFrom: day(1), Items: []domain.ItemInput{{ProductID: p1, Price: "45000", MaxRetailPrice: "50000"}, {ProductID: p2, Price: "30000"}},
	}, today)
	require.NoError(t, err)
	assert.Equal(t, domain.VersionDraft, v.Status)
	assert.Equal(t, 2, v.ItemCount)
	assert.Equal(t, "50000.00", v.Items[0].MaxRetailPrice.String())
	assert.Equal(t, "HARGA-DASAR v2", v.Label(base))
	assert.Contains(t, v.Summary(base), "2 produk")

	pv, err := domain.NewVersion(percent, 1, domain.VersionInput{ValidFrom: day(0), Percent: "90"}, today)
	require.NoError(t, err)
	assert.Contains(t, pv.Summary(percent), "90% dari harga acuan")

	deleted := fixedList()
	require.NoError(t, deleted.Delete(time.Now(), uuid.New()))
	_, err = domain.NewVersion(deleted, 1, domain.VersionInput{ValidFrom: day(0)}, today)
	assert.Error(t, err)
}

func TestVersionLifecycle(t *testing.T) {
	list := fixedList()
	by := uuid.New()
	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	v, err := domain.NewVersion(list, 1, domain.VersionInput{ValidFrom: day(2)}, today)
	require.NoError(t, err)

	require.NoError(t, v.Update(list, domain.VersionInput{ValidFrom: day(3), Notes: "kosong"}, today))
	assert.Equal(t, day(3), v.ValidFrom)
	assert.Contains(t, fields(t, v.Update(list, domain.VersionInput{}, today)), "validFrom")
	assert.Equal(t, day(3), v.ValidFrom, "update gagal tidak mengubah versi")

	assert.Contains(t, fields(t, v.Submit(list, today, at, by)), "items", "versi tanpa harga tidak bisa diajukan")
	require.NoError(t, v.Update(list, domain.VersionInput{ValidFrom: day(3), Items: []domain.ItemInput{{ProductID: uuid.New(), Price: "1000"}}}, today))
	require.NoError(t, v.Submit(list, today, at, by))
	assert.Equal(t, domain.VersionPendingApproval, v.Status)
	assert.False(t, v.Editable())
	assert.True(t, apperr.HasCode(v.Update(list, domain.VersionInput{ValidFrom: day(3)}, today), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(v.Submit(list, today, at, by), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(v.Cancel(), apperr.CodeInvalidStateTransition))

	require.NoError(t, v.Withdraw())
	assert.Equal(t, domain.VersionDraft, v.Status)
	assert.Nil(t, v.SubmittedAt)
	assert.True(t, apperr.HasCode(v.Withdraw(), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(v.Activate(today, at, by), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(v.Reject(at, by), apperr.CodeInvalidStateTransition))

	require.NoError(t, v.Submit(list, today, at, by))
	// Disetujui setelah tanggal mulai lewat: berlaku mulai hari persetujuan.
	require.NoError(t, v.Activate(day(5), at, by))
	assert.Equal(t, domain.VersionActive, v.Status)
	assert.Equal(t, day(5), v.ValidFrom)

	r, err := domain.NewVersion(list, 2, domain.VersionInput{ValidFrom: day(0), ValidTo: dayPtr(1), Items: []domain.ItemInput{{ProductID: uuid.New(), Price: "1"}}}, today)
	require.NoError(t, err)
	require.NoError(t, r.Submit(list, today, at, by))
	assert.Error(t, r.Activate(day(3), at, by), "tanggal berakhir sudah lewat saat disetujui")
	require.NoError(t, r.Reject(at, by))
	assert.Equal(t, domain.VersionRejected, r.Status)

	c, err := domain.NewVersion(list, 3, domain.VersionInput{ValidFrom: day(0)}, today)
	require.NoError(t, err)
	require.NoError(t, c.Cancel())
	assert.Equal(t, domain.VersionCancelled, c.Status)

	stale, err := domain.NewVersion(list, 4, domain.VersionInput{ValidFrom: day(0), Items: []domain.ItemInput{{ProductID: uuid.New(), Price: "1"}}}, today)
	require.NoError(t, err)
	assert.Contains(t, fields(t, stale.Submit(list, day(1), at, by)), "validFrom", "draft yang tanggal mulainya lewat harus diperbarui")

	percent := domain.PriceList{Basis: domain.BasisPercent, Type: domain.TypeEmployee}
	pv := domain.PriceListVersion{Status: domain.VersionDraft, Period: domain.Period{ValidFrom: day(0)}}
	assert.Contains(t, fields(t, pv.Submit(percent, today, at, by)), "percent")
}

func TestSupersedeActive(t *testing.T) {
	newV := domain.PriceListVersion{ID: uuid.New(), No: 3, Period: domain.Period{ValidFrom: day(10)}}
	openOld := domain.PriceListVersion{ID: uuid.New(), No: 1, Period: domain.Period{ValidFrom: day(-30)}}
	endedOld := domain.PriceListVersion{ID: uuid.New(), No: 2, Period: domain.Period{ValidFrom: day(-60), ValidTo: dayPtr(-31)}}

	closed, err := newV.SupersedeActive([]domain.PriceListVersion{openOld, endedOld, newV})
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.Equal(t, openOld.ID, closed[0].ID)
	assert.Equal(t, day(9), *closed[0].ValidTo)

	future := domain.PriceListVersion{ID: uuid.New(), No: 2, Period: domain.Period{ValidFrom: day(20)}}
	_, err = newV.SupersedeActive([]domain.PriceListVersion{future})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))

	bounded := domain.PriceListVersion{ID: uuid.New(), No: 2, Period: domain.Period{ValidFrom: day(0), ValidTo: dayPtr(15)}}
	_, err = newV.SupersedeActive([]domain.PriceListVersion{bounded})
	assert.Error(t, err, "versi aktif bertanggal akhir yang beririsan tidak ditutup otomatis")
}

func TestPercentOf(t *testing.T) {
	assert.Equal(t, "40950.00", domain.PercentOf(money.FromInt(45500), dec("90")).String())
	assert.Equal(t, "33334.00", domain.PercentOf(money.FromInt(33333), dec("100.002")).String())
	assert.Equal(t, "1.00", domain.PercentOf(money.MustParse("1.10"), dec("50")).String(), "pembulatan setengah ke atas")
}

func TestSpecialPrice(t *testing.T) {
	tests := []struct {
		name  string
		in    domain.SpecialPriceInput
		field string
	}{
		{"jenis pihak", domain.SpecialPriceInput{PartyID: uuid.New(), ProductID: uuid.New(), Price: "1", ValidFrom: today}, "partyType"},
		{"pihak wajib", domain.SpecialPriceInput{PartyType: domain.PartyCustomer, ProductID: uuid.New(), Price: "1", ValidFrom: today}, "partyId"},
		{"produk wajib", domain.SpecialPriceInput{PartyType: domain.PartyPartner, PartyID: uuid.New(), Price: "1", ValidFrom: today}, "productId"},
		{"harga wajib", domain.SpecialPriceInput{PartyType: domain.PartyPartner, PartyID: uuid.New(), ProductID: uuid.New(), ValidFrom: today}, "priceAmount"},
		{"tidak mundur", domain.SpecialPriceInput{PartyType: domain.PartyPartner, PartyID: uuid.New(), ProductID: uuid.New(), Price: "1", ValidFrom: day(-1)}, "validFrom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewSpecialPrice(tc.in, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}
	sp, err := domain.NewSpecialPrice(domain.SpecialPriceInput{
		PartyType: domain.PartyCustomer, PartyID: uuid.New(), ProductID: uuid.New(), Price: "41000", ValidFrom: today, Notes: " VIP ",
	}, today)
	require.NoError(t, err)
	assert.Equal(t, "41000.00", sp.Price.String())
	assert.Equal(t, "VIP", sp.Notes)
}

func promoInput(items ...domain.PromotionItemInput) domain.PromotionInput {
	return domain.PromotionInput{Code: "promo-1111", Name: "11.11", ValidFrom: day(1), ValidTo: day(3), IsActive: true, Items: items}
}

func TestPromotion(t *testing.T) {
	p1 := uuid.New()
	channel := uuid.New()
	tests := []struct {
		name  string
		in    domain.PromotionInput
		field string
	}{
		{"item wajib", promoInput(), "items"},
		{"isi salah satu", promoInput(domain.PromotionItemInput{ProductID: p1}), "items.0.priceAmount"},
		{"bukan keduanya", promoInput(domain.PromotionItemInput{ProductID: p1, Price: "1", DiscountPercent: "1"}), "items.0.priceAmount"},
		{"persen > 100", promoInput(domain.PromotionItemInput{ProductID: p1, DiscountPercent: "101"}), "items.0.discountPercent"},
		{"produk ganda", promoInput(domain.PromotionItemInput{ProductID: p1, Price: "1"}, domain.PromotionItemInput{ProductID: p1, Price: "2"}), "items.1.productId"},
		{"produk wajib", promoInput(domain.PromotionItemInput{Price: "1"}), "items.0.productId"},
		{"tanggal selesai wajib", domain.PromotionInput{Code: "P", Name: "P", ValidFrom: day(1), Items: []domain.PromotionItemInput{{ProductID: p1, Price: "1"}}}, "validTo"},
		{"sub-channel butuh channel", func() domain.PromotionInput {
			in := promoInput(domain.PromotionItemInput{ProductID: p1, Price: "1"})
			in.SubChannelID = &channel
			return in
		}(), "subChannelId"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewPromotion(tc.in, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}

	in := promoInput(domain.PromotionItemInput{ProductID: p1, DiscountPercent: "10"})
	in.ChannelID = &channel
	p, err := domain.NewPromotion(in, today)
	require.NoError(t, err)
	assert.Equal(t, "PROMO-1111", p.Code)
	assert.False(t, p.Started(today))
	assert.True(t, p.AppliesTo(&channel, nil, day(2)))
	assert.False(t, p.AppliesTo(nil, nil, day(2)), "promo per channel tidak berlaku tanpa channel")
	other := uuid.New()
	assert.False(t, p.AppliesTo(&other, nil, day(2)))
	assert.False(t, p.AppliesTo(&channel, nil, day(4)))
	sub := uuid.New()
	withSub := p
	withSub.SubChannelID = &sub
	assert.False(t, withSub.AppliesTo(&channel, nil, day(2)))
	assert.True(t, withSub.AppliesTo(&channel, &sub, day(2)))
	p.IsActive = false
	assert.False(t, p.AppliesTo(&channel, nil, day(2)))
	p.IsActive = true

	// Sebelum berjalan: semua isian boleh diubah.
	upd := promoInput(domain.PromotionItemInput{ProductID: p1, Price: "9000"})
	upd.ValidTo = day(5)
	require.NoError(t, p.Update(upd, today))
	assert.Nil(t, p.ChannelID)
	assert.Equal(t, "9000.00", p.Items[0].Price.String())

	// Setelah berjalan: hanya nama, keterangan, status, tanggal selesai.
	started := day(2)
	run := upd
	run.Name, run.ValidTo, run.IsActive = "11.11 diperpanjang", day(7), false
	require.NoError(t, p.Update(run, started))
	assert.Equal(t, "11.11 diperpanjang", p.Name)
	assert.Equal(t, day(7), p.ValidTo)

	bad := run
	bad.ValidFrom = day(2)
	assert.Contains(t, fields(t, p.Update(bad, started)), "validFrom")
	bad = run
	bad.ChannelID = &channel
	assert.Contains(t, fields(t, p.Update(bad, started)), "channelId")
	bad = run
	bad.Items = []domain.PromotionItemInput{{ProductID: p1, Price: "8000"}}
	assert.Contains(t, fields(t, p.Update(bad, started)), "items")
	bad = run
	bad.Items = nil
	require.NoError(t, p.Update(bad, started), "items kosong = tidak diubah")
	bad = run
	bad.ValidTo = clock.BusinessDate{}
	assert.Contains(t, fields(t, p.Update(bad, started)), "validTo")
	bad = run
	bad.ValidTo = day(0)
	assert.Contains(t, fields(t, p.Update(bad, started)), "validTo")
	bad = run
	bad.ValidTo = day(-5)
	assert.Contains(t, fields(t, p.Update(bad, day(10))), "validTo")
	assert.Contains(t, fields(t, p.Update(domain.PromotionInput{Name: "X", ValidFrom: day(1)}, today)), "validTo")
}

func TestPromotionOffer(t *testing.T) {
	regular := money.FromInt(50000)
	fixed := domain.PromotionItem{Price: ptr(money.FromInt(39000))}
	pct := domain.PromotionItem{DiscountPercent: ptr(dec("15"))}
	assert.False(t, fixed.NeedsRegular())
	assert.True(t, pct.NeedsRegular())
	assert.Equal(t, "39000.00", fixed.Offer(regular).String())
	assert.Equal(t, "42500.00", pct.Offer(regular).String())
	assert.Equal(t, "33333.00", domain.PromotionItem{DiscountPercent: ptr(dec("33.3333"))}.Offer(regular).String())
}

func TestTieredPrice(t *testing.T) {
	tests := []struct {
		name  string
		in    domain.TieredPriceInput
		field string
	}{
		{"produk wajib", domain.TieredPriceInput{MinQty: "10", Price: "1", ValidFrom: today}, "productId"},
		{"qty wajib", domain.TieredPriceInput{ProductID: uuid.New(), Price: "1", ValidFrom: today}, "minQty"},
		{"qty tidak valid", domain.TieredPriceInput{ProductID: uuid.New(), MinQty: "x", Price: "1", ValidFrom: today}, "minQty"},
		{"qty nol", domain.TieredPriceInput{ProductID: uuid.New(), MinQty: "0", Price: "1", ValidFrom: today}, "minQty"},
		{"qty 4 desimal", domain.TieredPriceInput{ProductID: uuid.New(), MinQty: "1.0001", Price: "1", ValidFrom: today}, "minQty"},
		{"harga wajib", domain.TieredPriceInput{ProductID: uuid.New(), MinQty: "10", ValidFrom: today}, "priceAmount"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewTieredPrice(tc.in, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}
	tp, err := domain.NewTieredPrice(domain.TieredPriceInput{ProductID: uuid.New(), MinQty: "12", Price: "40000", ValidFrom: today}, today)
	require.NoError(t, err)
	assert.True(t, tp.MinQty.Equal(dec("12")))
}

func TestDiscountLimitInput(t *testing.T) {
	tests := []struct {
		name  string
		in    domain.DiscountLimitInput
		field string
	}{
		{"role wajib", domain.DiscountLimitInput{MaxLinePercent: "5", MaxDocumentPercent: "5", ValidFrom: today}, "roleCode"},
		{"batas baris wajib", domain.DiscountLimitInput{RoleCode: "sales_mt", MaxDocumentPercent: "5", ValidFrom: today}, "maxLineDiscountPercent"},
		{"batas dokumen > 100", domain.DiscountLimitInput{RoleCode: "sales_mt", MaxLinePercent: "5", MaxDocumentPercent: "101", ValidFrom: today}, "maxDocumentDiscountPercent"},
		{"negatif", domain.DiscountLimitInput{RoleCode: "sales_mt", MaxLinePercent: "-1", MaxDocumentPercent: "5", ValidFrom: today}, "maxLineDiscountPercent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewDiscountLimit(tc.in, today)
			assert.Contains(t, fields(t, err), tc.field)
		})
	}
	l, err := domain.NewDiscountLimit(domain.DiscountLimitInput{RoleCode: "sales_mt", MaxLinePercent: "0", MaxDocumentPercent: "2.5", ValidFrom: today}, today)
	require.NoError(t, err)
	assert.True(t, l.MaxDocumentPercent.Equal(dec("2.5")))
}

func TestDiscountApply(t *testing.T) {
	base := money.FromInt(200000)
	tests := []struct {
		name    string
		d       domain.Discount
		amount  string
		percent string
		wantErr bool
	}{
		{"tanpa diskon", domain.Discount{}, "0.00", "0", false},
		{"persen", domain.Discount{Kind: domain.DiscountPercent, Value: dec("7.5")}, "15000.00", "7.5", false},
		{"nominal", domain.Discount{Kind: domain.DiscountAmount, Value: dec("15000")}, "15000.00", "7.5", false},
		{"nominal pecahan persen", domain.Discount{Kind: domain.DiscountAmount, Value: dec("10000")}, "10000.00", "5", false},
		{"persen > 100", domain.Discount{Kind: domain.DiscountPercent, Value: dec("101")}, "", "", true},
		{"nominal > dasar", domain.Discount{Kind: domain.DiscountAmount, Value: dec("200001")}, "", "", true},
		{"nominal 3 desimal", domain.Discount{Kind: domain.DiscountAmount, Value: dec("1.001")}, "", "", true},
		{"negatif", domain.Discount{Kind: domain.DiscountAmount, Value: dec("-1")}, "", "", true},
		{"jenis tidak dikenal", domain.Discount{Kind: "x", Value: dec("1")}, "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			amount, pct, err := tc.d.Apply(base)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.amount, amount.String())
			assert.True(t, pct.Equal(dec(tc.percent)), pct.String())
		})
	}
}

func TestEffectiveLimit(t *testing.T) {
	limits := []domain.DiscountLimit{
		{RoleCode: "sales_mt", MaxLinePercent: dec("5"), MaxDocumentPercent: dec("2"), Period: domain.Period{ValidFrom: day(-10)}},
		{RoleCode: "head_sales", MaxLinePercent: dec("20"), MaxDocumentPercent: dec("1"), Period: domain.Period{ValidFrom: day(-10)}},
		{RoleCode: "sales_ecommerce", MaxLinePercent: dec("50"), MaxDocumentPercent: dec("50"), Period: domain.Period{ValidFrom: day(1)}},
	}
	l := domain.EffectiveLimit(limits, []string{"sales_mt", "head_sales", "sales_ecommerce"}, today)
	assert.True(t, l.MaxLinePercent.Equal(dec("20")))
	assert.Equal(t, "head_sales", l.LineRoleCode)
	assert.True(t, l.MaxDocumentPercent.Equal(dec("2")))
	assert.Equal(t, "sales_mt", l.DocumentRoleCode)

	none := domain.EffectiveLimit(limits, []string{"customer_service"}, today)
	assert.True(t, none.MaxLinePercent.IsZero())
	assert.Empty(t, none.LineRoleCode)

	zero := domain.EffectiveLimit([]domain.DiscountLimit{{RoleCode: "cs", Period: domain.Period{ValidFrom: day(0)}}}, []string{"cs"}, today)
	assert.Equal(t, "cs", zero.LineRoleCode, "batas 0% tetap tercatat dari role")
}

func TestCheckDiscount(t *testing.T) {
	limit := domain.Limit{MaxLinePercent: dec("5"), MaxDocumentPercent: dec("2")}
	pct := func(v string) domain.Discount { return domain.Discount{Kind: domain.DiscountPercent, Value: dec(v)} }
	amt := func(v string) domain.Discount { return domain.Discount{Kind: domain.DiscountAmount, Value: dec(v)} }
	gross := money.FromInt(100000)
	tests := []struct {
		name     string
		lines    []domain.LineDiscount
		doc      domain.DocumentDiscount
		approval bool
		total    string
	}{
		{"tanpa diskon", []domain.LineDiscount{{Gross: gross}}, domain.DocumentDiscount{Subtotal: gross}, false, "0.00"},
		{"baris tepat di batas", []domain.LineDiscount{{Gross: gross, Discount: pct("5")}}, domain.DocumentDiscount{Subtotal: money.FromInt(95000)}, false, "5000.00"},
		{"baris di atas batas", []domain.LineDiscount{{Gross: gross, Discount: amt("5001")}}, domain.DocumentDiscount{Subtotal: money.FromInt(94999)}, true, "5001.00"},
		{"dokumen di atas batas", []domain.LineDiscount{{Gross: gross}}, domain.DocumentDiscount{Subtotal: gross, Discount: pct("2.5")}, true, "2500.00"},
		{"dokumen nominal di batas", []domain.LineDiscount{{Gross: gross}}, domain.DocumentDiscount{Subtotal: gross, Discount: amt("2000")}, false, "2000.00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := domain.CheckDiscount(limit, tc.lines, tc.doc)
			require.NoError(t, err)
			assert.Equal(t, tc.approval, c.RequiresApproval)
			assert.Equal(t, tc.total, c.TotalDiscountAmount.String())
		})
	}
	_, err := domain.CheckDiscount(limit, []domain.LineDiscount{{Gross: gross, Discount: pct("120")}}, domain.DocumentDiscount{Subtotal: gross, Discount: amt("999999")})
	f := fields(t, err)
	assert.Contains(t, f, "lines.0.discount")
	assert.Contains(t, f, "documentDiscount")
}

func TestPriceRequestAndRules(t *testing.T) {
	channel := uuid.New()
	assert.NoError(t, domain.PriceRequest{ProductID: uuid.New(), Qty: dec("1"), Date: today}.Validate())
	f := fields(t, domain.PriceRequest{SubChannelID: &channel}.Validate())
	for _, k := range []string{"productId", "qty", "date", "subChannelId"} {
		assert.Contains(t, f, k)
	}
	for _, r := range []domain.Rule{domain.RuleSpecialPrice, domain.RulePromotion, domain.RuleTieredPrice, domain.RulePriceList, domain.RuleBasePrice} {
		assert.NotEmpty(t, r.Label())
	}
	assert.True(t, domain.SameID(nil, nil))
	assert.False(t, domain.SameID(&channel, nil))
	assert.True(t, domain.SameID(&channel, ptr(channel)))
	assert.True(t, domain.PartyCustomer.Valid())
	assert.False(t, domain.PartyType("x").Valid())
	err := domain.OverlapError("harga khusus lain", domain.Period{ValidFrom: today})
	assert.Contains(t, fields(t, err)["validFrom"], "mulai 2026-10-23")
}
