package domain

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// DiscountKind adalah bentuk diskon manual (MD-05).
type DiscountKind string

const (
	DiscountPercent DiscountKind = "percent"
	DiscountAmount  DiscountKind = "amount"
)

// Discount adalah diskon manual: persen dari dasar, atau nominal rupiah. Nilai nol = tanpa diskon.
type Discount struct {
	Kind  DiscountKind
	Value decimal.Decimal
}

// Apply menghitung nominal dan persen diskon terhadap dasar (bruto baris atau subtotal dokumen). Persen
// dibulatkan 4 desimal, nominal 2 desimal.
func (d Discount) Apply(base money.Amount) (money.Amount, decimal.Decimal, error) {
	if d.Value.IsZero() {
		return money.Zero, decimal.Zero, nil
	}
	if d.Value.IsNegative() {
		return money.Zero, decimal.Zero, fmt.Errorf("diskon tidak boleh negatif")
	}
	switch d.Kind {
	case DiscountPercent:
		if d.Value.GreaterThan(hundred) {
			return money.Zero, decimal.Zero, fmt.Errorf("diskon paling besar 100%%")
		}
		return base.Percent(d.Value).Round(), d.Value.Round(money.RateScale), nil
	case DiscountAmount:
		amount := money.FromDecimal(d.Value)
		if !d.Value.Equal(d.Value.Round(money.AmountScale)) {
			return money.Zero, decimal.Zero, fmt.Errorf("nominal diskon paling banyak 2 angka desimal")
		}
		if amount.Cmp(base) > 0 {
			return money.Zero, decimal.Zero, fmt.Errorf("diskon melebihi nilai sebelum diskon")
		}
		return amount, d.Value.Mul(hundred).Div(base.Decimal()).Round(money.RateScale), nil
	}
	return money.Zero, decimal.Zero, fmt.Errorf("jenis diskon tidak dikenal")
}

// LineDiscount adalah diskon manual satu baris. Gross = qty × harga satuan sebelum diskon.
type LineDiscount struct {
	Gross    money.Amount
	Discount Discount
}

// DocumentDiscount adalah diskon manual per dokumen. Subtotal = total baris setelah diskon baris.
type DocumentDiscount struct {
	Subtotal money.Amount
	Discount Discount
}

// Limit adalah batas diskon efektif user: nilai terbesar dari semua role yang ia pegang. Tanpa batas
// tercatat = 0% (setiap diskon manual butuh approval).
type Limit struct {
	MaxLinePercent     decimal.Decimal
	MaxDocumentPercent decimal.Decimal
	// LineRoleCode dan DocumentRoleCode adalah role yang memberi batas tersebut ("" bila tidak ada).
	LineRoleCode     string
	DocumentRoleCode string
}

// EffectiveLimit menghitung batas diskon untuk role-role user pada tanggal on.
func EffectiveLimit(limits []DiscountLimit, roleCodes []string, on clock.BusinessDate) Limit {
	var out Limit
	for _, l := range limits {
		if l.Deleted() || !l.Contains(on) || !slices.Contains(roleCodes, l.RoleCode) {
			continue
		}
		if l.MaxLinePercent.GreaterThan(out.MaxLinePercent) || out.LineRoleCode == "" && l.MaxLinePercent.Equal(out.MaxLinePercent) {
			out.MaxLinePercent, out.LineRoleCode = l.MaxLinePercent, l.RoleCode
		}
		if l.MaxDocumentPercent.GreaterThan(out.MaxDocumentPercent) || out.DocumentRoleCode == "" && l.MaxDocumentPercent.Equal(out.MaxDocumentPercent) {
			out.MaxDocumentPercent, out.DocumentRoleCode = l.MaxDocumentPercent, l.RoleCode
		}
	}
	return out
}

// DiscountResult adalah hasil pemeriksaan satu diskon.
type DiscountResult struct {
	Amount       money.Amount
	Percent      decimal.Decimal
	ExceedsLimit bool
}

// DiscountCheck adalah hasil pemeriksaan diskon manual sebuah dokumen penjualan terhadap batas role.
type DiscountCheck struct {
	Limit    Limit
	Lines    []DiscountResult
	Document DiscountResult
	// RequiresApproval: ada diskon di atas batas → dokumen butuh approval sales_order_discount (Head Sales).
	RequiresApproval bool
	// TotalDiscountAmount adalah total nominal diskon baris dan dokumen (nominal approval).
	TotalDiscountAmount money.Amount
}

// CheckDiscount memeriksa diskon per baris dan per dokumen terhadap batas. Diskon tidak valid menghasilkan
// VALIDATION_FAILED dengan field `lines.N.discount` / `documentDiscount`.
func CheckDiscount(limit Limit, lines []LineDiscount, doc DocumentDiscount) (DiscountCheck, error) {
	f := fieldErrors{}
	out := DiscountCheck{Limit: limit, Lines: make([]DiscountResult, len(lines))}
	total := money.Zero
	for i, l := range lines {
		amount, pct, err := l.Discount.Apply(l.Gross)
		if err != nil {
			f.add(fmt.Sprintf("lines.%d.discount", i), "Diskon baris "+err.Error())
			continue
		}
		r := DiscountResult{Amount: amount, Percent: pct, ExceedsLimit: pct.GreaterThan(limit.MaxLinePercent)}
		out.Lines[i] = r
		total = total.Add(amount)
		out.RequiresApproval = out.RequiresApproval || r.ExceedsLimit
	}
	amount, pct, err := doc.Discount.Apply(doc.Subtotal)
	if err != nil {
		f.add("documentDiscount", "Diskon dokumen "+err.Error())
	} else {
		out.Document = DiscountResult{Amount: amount, Percent: pct, ExceedsLimit: pct.GreaterThan(limit.MaxDocumentPercent)}
		out.RequiresApproval = out.RequiresApproval || out.Document.ExceedsLimit
		total = total.Add(amount)
	}
	out.TotalDiscountAmount = total
	if err := f.err(); err != nil {
		return DiscountCheck{}, err
	}
	return out, nil
}
