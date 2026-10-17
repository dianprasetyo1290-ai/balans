package domain

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// MarketplaceReceivableAccount mengembalikan akun Piutang Marketplace (121x) per sub-channel marketplace
// (Desain Finance & Accounting 1); sub-channel lain memakai akun default posting rule A-10 (1211).
func MarketplaceReceivableAccount(subChannelCode string) string {
	switch subChannelCode {
	case "shopee":
		return "1211"
	case "tiktok_tokopedia":
		return "1212"
	case "lazada":
		return "1213"
	case "blibli":
		return "1214"
	}
	return ""
}

// ImportItem adalah satu item pada kolom Item impor penjualan marketplace.
type ImportItem struct {
	SKU       string
	Qty       decimal.Decimal
	UnitPrice *money.Amount
}

// ParseImportItems membaca kolom Item impor marketplace: `SKU:qty:harga; SKU:qty` (harga opsional = harga dari
// resolver). Pemisah item `;`, pemisah bagian `:`.
func ParseImportItems(s string) ([]ImportItem, string) {
	var out []ImportItem
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		if len(fields) < 2 || len(fields) > 3 {
			return nil, fmt.Sprintf("Format item %q salah; gunakan SKU:qty:harga", part)
		}
		item := ImportItem{SKU: strings.ToUpper(strings.TrimSpace(fields[0]))}
		if item.SKU == "" {
			return nil, fmt.Sprintf("SKU kosong pada item %q", part)
		}
		qty, err := decimal.NewFromString(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Sprintf("Qty item %s tidak valid", item.SKU)
		}
		if msg := ValidQty(qty); msg != "" {
			return nil, fmt.Sprintf("Item %s: %s", item.SKU, msg)
		}
		item.Qty = qty
		if len(fields) == 3 && strings.TrimSpace(fields[2]) != "" {
			price, err := money.Parse(strings.TrimSpace(fields[2]))
			if err != nil || price.IsNegative() {
				return nil, fmt.Sprintf("Harga item %s tidak valid", item.SKU)
			}
			item.UnitPrice = &price
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, "Minimal satu item"
	}
	return out, ""
}

// NonPKPNote adalah keterangan NON-PKP di Proforma Invoice dan Invoice (Desain Dokumen Cetak 2.2, D-04).
const NonPKPNote = "Balans bukan Pengusaha Kena Pajak; tidak dipungut PPN."

// ProformaNote adalah keterangan tambahan Proforma Invoice.
const ProformaNote = "Dokumen ini bukan bukti pembayaran."

// ShareText menyusun teks invoice siap tempel ke WhatsApp (SLS-04): nomor, pesanan, item, total, status bayar,
// dan keterangan NON-PKP.
func ShareText(inv Invoice, link string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*Invoice %s*\n", inv.Number)
	fmt.Fprintf(&b, "Kepada: %s\n", inv.PartyName)
	fmt.Fprintf(&b, "Tanggal: %s · Jatuh tempo: %s\n", formatDate(inv.InvoiceDate), formatDate(inv.DueDate))
	if inv.OrderNumber != "" {
		fmt.Fprintf(&b, "Pesanan: %s\n", inv.OrderNumber)
	}
	b.WriteString("\n")
	for _, l := range inv.Lines {
		fmt.Fprintf(&b, "- %s × %s = %s\n", l.Description, l.Qty.String(), l.Total.Rupiah())
	}
	b.WriteString("\n")
	if inv.Discount.IsPositive() {
		fmt.Fprintf(&b, "Diskon: -%s\n", inv.Discount.Rupiah())
	}
	if inv.Shipping.IsPositive() {
		fmt.Fprintf(&b, "Ongkos kirim: %s\n", inv.Shipping.Rupiah())
	}
	fmt.Fprintf(&b, "*Total: %s*\n", inv.Total.Rupiah())
	switch inv.Status {
	case StatusPaid:
		b.WriteString("Status: LUNAS\n")
	case StatusPartiallyPaid:
		fmt.Fprintf(&b, "Status: Dibayar sebagian, sisa %s\n", inv.Outstanding().Rupiah())
	case StatusDraft, StatusPendingApproval, StatusAwaitingPayment, StatusInProgress, StatusInTransit, StatusCompleted,
		StatusCancelled, StatusExpired, StatusPosted:
		fmt.Fprintf(&b, "Status: %s\n", inv.Status.Label())
	}
	if link != "" {
		fmt.Fprintf(&b, "\nLihat invoice: %s\n", link)
	}
	b.WriteString("\n" + NonPKPNote + "\nTerima kasih telah berbelanja di Balans Nutrition.")
	return b.String()
}

var monthNames = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func formatDate(d clock.BusinessDate) string {
	return fmt.Sprintf("%d %s %d", d.Day(), monthNames[d.Month()-1], d.Year())
}
