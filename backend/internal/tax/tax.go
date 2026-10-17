// Package tax berisi kalkulator pajak murni Fase 1: PPh 21 bukan pegawai, PPh 23, PPh 4(2), dan PPh Final UMKM
// (BRD ACC-19, ACC-20; Technical Guideline 5.9). Fungsi di sini tanpa DB, tanpa HTTP, dan tanpa time.Now():
// tarif effective-dated dimuat modul accounting dari tabel withholding_tax_rates lalu dikirim ke kalkulator.
//
// Nilai pajak dibulatkan ke bawah ke rupiah penuh (Technical Guideline 5.8.6: PPh mengikuti pembulatan
// regulasi); tarif dan persentase memakai 4 desimal.
package tax

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Type adalah jenis pajak yang dipotong/dihitung sistem. Nilainya sama dengan withholding_type jenis biaya dan
// default_withholding pemasok (tanpa `none`).
type Type string

const (
	PPh21     Type = "pph21"
	PPh23     Type = "pph23"
	PPh42     Type = "pph4_2"
	FinalUMKM Type = "final_umkm"
)

var typeLabels = map[Type]string{
	PPh21: "PPh 21", PPh23: "PPh 23", PPh42: "PPh 4(2)", FinalUMKM: "PPh Final UMKM",
}

// Types mengembalikan seluruh jenis pajak urut tampilan.
func Types() []Type { return []Type{PPh21, PPh23, PPh42, FinalUMKM} }

// Valid memeriksa jenis pajak.
func (t Type) Valid() bool {
	_, ok := typeLabels[t]
	return ok
}

// Label mengembalikan nama jenis pajak untuk UI dan ekspor.
func (t Type) Label() string { return typeLabels[t] }

// IsWithholding: jenis pajak yang dipotong dari pembayaran ke pihak lain (bukti potong), bukan PPh Final atas
// omzet sendiri.
func (t Type) IsWithholding() bool { return t == PPh21 || t == PPh23 || t == PPh42 }

var hundred = decimal.NewFromInt(100)

// Bracket adalah satu lapisan tarif progresif (Pasal 17 UU PPh). UpTo nol berarti lapisan teratas tanpa batas.
type Bracket struct {
	UpTo        money.Amount
	RatePercent decimal.Decimal
}

// Rate adalah satu versi tarif effective-dated untuk jenis pajak dan kode objek pajak.
type Rate struct {
	Type Type
	// ObjectCode adalah kode objek pajak Coretax, mis. 24-104-14.
	ObjectCode string
	Name       string
	// RatePercent adalah tarif tunggal; diabaikan bila Brackets diisi (tarif progresif).
	RatePercent decimal.Decimal
	Brackets    []Bracket
	// BasePercent adalah dasar pengenaan pajak sebagai persen bruto (50 untuk PPh 21 bukan pegawai). Nol = 100.
	BasePercent decimal.Decimal
	// NoTaxIDSurchargePercent adalah kenaikan tarif bagi penerima tanpa NPWP/NIK (100 untuk PPh 23, 20 untuk
	// PPh 21). Nol = tanpa kenaikan.
	NoTaxIDSurchargePercent decimal.Decimal
	ValidFrom               clock.BusinessDate
	// ValidTo inklusif; nil = berlaku sampai ada versi baru.
	ValidTo *clock.BusinessDate
}

// Validate memeriksa isian tarif. Kunci field memakai nama field API.
func (r Rate) Validate() error {
	fields := map[string]string{}
	if !r.Type.Valid() {
		fields["taxType"] = "Jenis pajak tidak dikenal"
	}
	if strings.TrimSpace(r.ObjectCode) == "" {
		fields["objectCode"] = "Kode objek pajak wajib diisi"
	} else if len(r.ObjectCode) > 30 {
		fields["objectCode"] = "Kode objek pajak paling panjang 30 karakter"
	}
	if strings.TrimSpace(r.Name) == "" {
		fields["name"] = "Nama objek pajak wajib diisi"
	}
	if len(r.Brackets) == 0 && !percentInRange(r.RatePercent, false) {
		fields["ratePercent"] = "Tarif harus lebih dari 0% dan paling besar 100%"
	}
	if msg := validateBrackets(r.Brackets); msg != "" {
		fields["brackets"] = msg
	}
	if !r.BasePercent.IsZero() && !percentInRange(r.BasePercent, false) {
		fields["basePercent"] = "Dasar pengenaan harus lebih dari 0% dan paling besar 100%"
	}
	if r.NoTaxIDSurchargePercent.IsNegative() || r.NoTaxIDSurchargePercent.GreaterThan(hundred) {
		fields["noTaxIdSurchargePercent"] = "Kenaikan tarif tanpa NPWP antara 0% dan 100%"
	}
	if r.ValidFrom.IsZero() {
		fields["validFrom"] = "Tanggal berlaku wajib diisi"
	} else if r.ValidTo != nil && r.ValidTo.Before(r.ValidFrom) {
		fields["validTo"] = "Tanggal akhir tidak boleh sebelum tanggal berlaku"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func percentInRange(p decimal.Decimal, allowZero bool) bool {
	if p.IsNegative() || p.GreaterThan(hundred) {
		return false
	}
	return allowZero || p.IsPositive()
}

func validateBrackets(bs []Bracket) string {
	prev := money.Zero
	for i, b := range bs {
		if !percentInRange(b.RatePercent, false) {
			return fmt.Sprintf("Lapisan %d: tarif harus lebih dari 0%% dan paling besar 100%%", i+1)
		}
		last := i == len(bs)-1
		switch {
		case b.UpTo.IsZero() && !last:
			return fmt.Sprintf("Lapisan %d: hanya lapisan terakhir yang boleh tanpa batas", i+1)
		case !b.UpTo.IsZero() && b.UpTo.Cmp(prev) <= 0:
			return fmt.Sprintf("Lapisan %d: batas harus lebih besar dari lapisan sebelumnya", i+1)
		}
		prev = b.UpTo
	}
	return ""
}

// EffectiveOn: tarif berlaku pada tanggal d.
func (r Rate) EffectiveOn(d clock.BusinessDate) bool {
	if d.Before(r.ValidFrom) {
		return false
	}
	return r.ValidTo == nil || !d.After(*r.ValidTo)
}

// SelectRate memilih versi tarif jenis pajak dan kode objek yang berlaku pada tanggal transaksi
// (Technical Guideline 5.14). Bila beberapa versi tumpang tindih, yang ValidFrom-nya paling akhir menang.
// Tidak ada versi berlaku → VALIDATION_FAILED pada field objectCode.
func SelectRate(rates []Rate, t Type, objectCode string, on clock.BusinessDate) (Rate, error) {
	var (
		best  Rate
		found bool
	)
	for _, r := range rates {
		if r.Type != t || r.ObjectCode != objectCode || !r.EffectiveOn(on) {
			continue
		}
		if !found || r.ValidFrom.After(best.ValidFrom) {
			best, found = r, true
		}
	}
	if !found {
		return Rate{}, apperr.Validation(map[string]string{
			"objectCode": fmt.Sprintf("Tarif %s objek %s belum ada untuk tanggal %s", t.Label(), objectCode, on.String()),
		})
	}
	return best, nil
}

// WithholdingInput adalah dasar pemotongan satu pembayaran.
type WithholdingInput struct {
	// Gross adalah penghasilan bruto yang dibayarkan (sebelum dipotong).
	Gross money.Amount
	// HasTaxID: penerima memiliki NPWP/NIK yang valid. Tanpa NPWP dikenai kenaikan tarif.
	HasTaxID bool
}

// Withholding adalah hasil perhitungan potongan satu pembayaran.
type Withholding struct {
	Type       Type
	ObjectCode string
	Gross      money.Amount
	// TaxableBase adalah dasar pengenaan pajak (bruto × BasePercent).
	TaxableBase money.Amount
	// RatePercent adalah tarif efektif terhadap dasar pengenaan (termasuk kenaikan tanpa NPWP), 4 desimal.
	RatePercent decimal.Decimal
	Surcharged  bool
	// Amount adalah pajak yang dipotong, rupiah penuh dibulatkan ke bawah.
	Amount money.Amount
}

// floorRupiah membulatkan ke bawah ke rupiah penuh.
func floorRupiah(a money.Amount) money.Amount { return money.FromDecimal(a.Decimal().Floor()) }

// Withhold menghitung PPh yang dipotong dari pembayaran bruto (PPh 21 bukan pegawai, PPh 23, PPh 4(2)).
// Bruto nol menghasilkan potongan nol; bruto negatif ditolak.
func (r Rate) Withhold(in WithholdingInput) (Withholding, error) {
	if !r.Type.IsWithholding() {
		return Withholding{}, apperr.Validation(map[string]string{"taxType": r.Type.Label() + " bukan pajak yang dipotong"})
	}
	if in.Gross.IsNegative() {
		return Withholding{}, apperr.Validation(map[string]string{"amount": "Dasar pemotongan tidak boleh negatif"})
	}
	out := Withholding{Type: r.Type, ObjectCode: r.ObjectCode, Gross: in.Gross}
	basePct := r.BasePercent
	if basePct.IsZero() {
		basePct = hundred
	}
	out.TaxableBase = in.Gross.Percent(basePct).Round()
	tax := r.applyRate(out.TaxableBase)
	if !in.HasTaxID && r.NoTaxIDSurchargePercent.IsPositive() {
		tax = tax.Add(tax.Percent(r.NoTaxIDSurchargePercent))
		out.Surcharged = true
	}
	out.Amount = floorRupiah(tax)
	if out.TaxableBase.IsPositive() {
		out.RatePercent = tax.Decimal().Div(out.TaxableBase.Decimal()).Mul(hundred).Round(4)
	} else {
		out.RatePercent = r.nominalRate()
		if out.Surcharged {
			out.RatePercent = out.RatePercent.Add(out.RatePercent.Mul(r.NoTaxIDSurchargePercent).Div(hundred)).Round(4)
		}
	}
	return out, nil
}

// nominalRate adalah tarif tunggal atau tarif lapisan pertama untuk tampilan saat dasar pengenaan nol.
func (r Rate) nominalRate() decimal.Decimal {
	if len(r.Brackets) > 0 {
		return r.Brackets[0].RatePercent
	}
	return r.RatePercent
}

// applyRate menerapkan tarif tunggal atau progresif pada dasar pengenaan (tanpa pembulatan).
func (r Rate) applyRate(base money.Amount) money.Amount {
	if len(r.Brackets) == 0 {
		return base.Percent(r.RatePercent)
	}
	total, lower := money.Zero, money.Zero
	for _, b := range r.Brackets {
		if base.Cmp(lower) <= 0 {
			break
		}
		upper := base
		if !b.UpTo.IsZero() && b.UpTo.Cmp(base) < 0 {
			upper = b.UpTo
		}
		total = total.Add(upper.Sub(lower).Percent(b.RatePercent))
		if b.UpTo.IsZero() {
			break
		}
		lower = b.UpTo
	}
	return total
}

// FinalTax menghitung PPh Final UMKM atas omzet bruto satu masa (PP 55/2022): omzet × tarif, dibulatkan ke
// bawah ke rupiah penuh. Omzet nol atau negatif (retur melebihi penjualan) menghasilkan nol.
func FinalTax(turnover money.Amount, r Rate) (money.Amount, error) {
	if r.Type != FinalUMKM {
		return money.Zero, apperr.Validation(map[string]string{"taxType": "Tarif bukan PPh Final UMKM"})
	}
	if !turnover.IsPositive() {
		return money.Zero, nil
	}
	return floorRupiah(turnover.Percent(r.RatePercent)), nil
}

// ThresholdStatus adalah posisi omzet kumulatif setahun terhadap batas peredaran bruto PPh Final UMKM.
type ThresholdStatus string

const (
	ThresholdBelow       ThresholdStatus = "below"
	ThresholdApproaching ThresholdStatus = "approaching"
	ThresholdExceeded    ThresholdStatus = "exceeded"
)

// UMKMAnnualThreshold adalah batas peredaran bruto setahun PPh Final UMKM (PP 55/2022): Rp4,8 miliar.
var UMKMAnnualThreshold = money.FromInt(4_800_000_000)

// UMKMWarningPercent adalah persentase batas yang memicu peringatan "mendekati batas".
// TODO(setting): pindahkan ke settings `final_tax.warning_percent` bila modul setting sudah ada.
var UMKMWarningPercent = decimal.NewFromInt(80)

// Threshold adalah hasil pemantauan batas omzet.
type Threshold struct {
	Cumulative  money.Amount
	Limit       money.Amount
	Remaining   money.Amount
	UsedPercent decimal.Decimal
	Status      ThresholdStatus
}

// CheckThreshold membandingkan omzet kumulatif tahun berjalan dengan batas: `approaching` mulai warnPercent
// dari batas, `exceeded` bila melewati batas (fasilitas PPh Final berakhir tahun berikutnya).
func CheckThreshold(cumulative, limit money.Amount, warnPercent decimal.Decimal) Threshold {
	out := Threshold{Cumulative: cumulative, Limit: limit, Remaining: limit.Sub(cumulative), Status: ThresholdBelow}
	if out.Remaining.IsNegative() {
		out.Remaining = money.Zero
	}
	if limit.IsPositive() {
		out.UsedPercent = cumulative.Decimal().Div(limit.Decimal()).Mul(hundred).Round(2)
	}
	switch {
	case cumulative.Cmp(limit) > 0:
		out.Status = ThresholdExceeded
	case limit.IsPositive() && out.UsedPercent.GreaterThanOrEqual(warnPercent):
		out.Status = ThresholdApproaching
	}
	return out
}

// Period adalah masa pajak (bulan kalender).
type Period struct {
	Year  int
	Month time.Month
}

// PeriodOf mengembalikan masa pajak tanggal bisnis.
func PeriodOf(d clock.BusinessDate) Period { return Period{Year: d.Year(), Month: d.Month()} }

// Validate memeriksa masa pajak.
func (p Period) Validate() error {
	if p.Year < 2000 || p.Year > 9999 || p.Month < time.January || p.Month > time.December {
		return apperr.Validation(map[string]string{"period": "Masa pajak tidak valid"})
	}
	return nil
}

// String mengembalikan masa pajak berformat YYYY-MM.
func (p Period) String() string { return fmt.Sprintf("%04d-%02d", p.Year, int(p.Month)) }

// FirstDay adalah tanggal 1 masa pajak.
func (p Period) FirstDay() clock.BusinessDate { return clock.NewBusinessDate(p.Year, p.Month, 1) }

// LastDay adalah tanggal terakhir masa pajak.
func (p Period) LastDay() clock.BusinessDate {
	return clock.NewBusinessDate(p.Year, p.Month+1, 1).AddDays(-1)
}

// Before: masa p sebelum masa o.
func (p Period) Before(o Period) bool {
	return p.Year < o.Year || (p.Year == o.Year && p.Month < o.Month)
}
