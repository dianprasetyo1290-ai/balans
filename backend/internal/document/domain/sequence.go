// Package domain berisi kode dokumen dan format nomor KODE/TAHUN/BULAN/NOMOR (U-15, Technical Guideline 5.12,
// Desain Dokumen Cetak & Penomoran bagian 1).
package domain

import (
	"fmt"
	"time"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Type adalah kode dokumen, mis. `JV` atau `PO`.
type Type string

// Kode dokumen (Technical Guideline 5.12 dan kode tambahan Desain Penomoran bagian 1).
const (
	TypePurchaseRequest           Type = "PR"
	TypePurchaseOrder             Type = "PO"
	TypeGoodsReceipt              Type = "GR"
	TypeDeliveryOrder             Type = "DO"
	TypeProformaInvoice           Type = "PI"
	TypeInvoice                   Type = "INV"
	TypeCreditNote                Type = "CN"
	TypeDebitNote                 Type = "DN"
	TypeSalesReturn               Type = "RJ"
	TypePurchaseReturn            Type = "RB"
	TypeStockTransfer             Type = "TRF"
	TypeStockCount                Type = "SC"
	TypeConsignmentReconciliation Type = "KSN"
	TypeExpenseClaim              Type = "EXP"
	TypePettyCash                 Type = "KK"
	TypeJournal                   Type = "JV"
	TypeSalesOrder                Type = "SO"
	TypeInbound                   Type = "IN"
	TypeOutbound                  Type = "OUT"
	TypeInternalUsage             Type = "PU"
	TypeDisposal                  Type = "MUS"
	TypePayment                   Type = "BYR"
	TypeResellerDeposit           Type = "DEP"
	TypeVendingRefill             Type = "RFL"
	TypeVendingReconciliation     Type = "RKV"
	TypeRepacking                 Type = "RPK"
	TypeReturnDisposition         Type = "DRT"
)

// NumberDigits adalah panjang nomor urut (5 digit, contoh `JV/2027/01/00102`).
const NumberDigits = 5

// TypeInfo adalah keterangan kode dokumen untuk halaman Pengaturan › Nomor Dokumen.
type TypeInfo struct {
	Type       Type
	Label      string
	AssignedAt string
}

var types = []TypeInfo{
	{TypePurchaseRequest, "Purchase Request", "Submit"},
	{TypePurchaseOrder, "Purchase Order", "Submit"},
	{TypeGoodsReceipt, "Penerimaan Barang", "Posting"},
	{TypeDeliveryOrder, "Delivery Order", "Release"},
	{TypeProformaInvoice, "Proforma Invoice", "Terbit"},
	{TypeInvoice, "Invoice", "Posting"},
	{TypeCreditNote, "Nota Kredit", "Posting"},
	{TypeDebitNote, "Nota Debit", "Posting retur pembelian"},
	{TypeSalesReturn, "Retur Penjualan", "Submit"},
	{TypePurchaseReturn, "Retur Pembelian", "Submit/posting"},
	{TypeStockTransfer, "Transfer Stok", "Kirim/posting"},
	{TypeStockCount, "Stock Count", "Mulai hitung"},
	{TypeConsignmentReconciliation, "Rekonsiliasi Konsinyasi", "Mulai rekonsiliasi"},
	{TypeExpenseClaim, "Pengajuan Biaya", "Submit"},
	{TypePettyCash, "Kas Kecil", "Posting"},
	{TypeJournal, "Jurnal", "Posting"},
	{TypeSalesOrder, "Sales Order", "Submit"},
	{TypeInbound, "Inbound", "Mulai QC"},
	{TypeOutbound, "Outbound", "Mulai picking"},
	{TypeInternalUsage, "Pemakaian Internal", "Submit"},
	{TypeDisposal, "Pemusnahan", "Submit"},
	{TypePayment, "Pembayaran", "Posting"},
	{TypeResellerDeposit, "Mutasi Deposit Reseller", "Posting"},
	{TypeVendingRefill, "Refill Vending", "Posting"},
	{TypeVendingReconciliation, "Rekonsiliasi Vending", "Mulai rekonsiliasi"},
	{TypeRepacking, "Repacking", "Mulai repacking"},
	{TypeReturnDisposition, "Disposisi Retur", "Submit"},
}

// Types mengembalikan seluruh kode dokumen sesuai urutan desain.
func Types() []TypeInfo { return types }

// Lookup mencari keterangan kode dokumen.
func Lookup(t Type) (TypeInfo, bool) {
	for _, info := range types {
		if info.Type == t {
			return info, true
		}
	}
	return TypeInfo{}, false
}

// Validate menolak kode dokumen yang tidak dikenal.
func (t Type) Validate() error {
	if _, ok := Lookup(t); !ok {
		return apperr.Validation(map[string]string{"documentType": fmt.Sprintf("Kode dokumen %q tidak dikenal", string(t))})
	}
	return nil
}

// Format menyusun nomor dokumen. Tahun dan bulan diambil dari tanggal bisnis dokumen.
func Format(t Type, date clock.BusinessDate, number int64) string {
	return fmt.Sprintf("%s/%04d/%02d/%0*d", t, date.Year(), int(date.Month()), NumberDigits, number)
}

// Sequence adalah nomor terakhir satu kode dokumen pada satu tahun.
type Sequence struct {
	Type       Type
	Year       int
	LastNumber int64
	UpdatedAt  *time.Time
}
