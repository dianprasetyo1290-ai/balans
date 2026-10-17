package money

import "strings"

// Rupiah memformat nominal untuk teks notifikasi dan email sesuai NFR-05, mis. Rp1.250.000.
// Nominal dibulatkan ke rupiah penuh.
func (a Amount) Rupiah() string {
	digits := a.RoundRupiah().d.Abs().StringFixed(0)
	var b strings.Builder
	if a.RoundRupiah().IsNegative() {
		b.WriteString("-")
	}
	b.WriteString("Rp")
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String()
}
