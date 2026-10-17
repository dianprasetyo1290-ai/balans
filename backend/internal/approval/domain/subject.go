// Package domain berisi aturan engine approval generik (BRD 5.17, Technical Guideline 5.5):
// pemilihan kebijakan, langkah berurutan, kelayakan approver, dan delegasi.
package domain

import (
	"slices"

	"github.com/google/uuid"

	"balans/internal/platform/money"
)

// SubjectType adalah jenis dokumen yang meminta approval (`subject_type`).
type SubjectType string

// Jenis dokumen di matriks approval Fase 1 (Desain Role & Approval bagian 4).
const (
	SubjectPurchaseRequest           SubjectType = "purchase_request"
	SubjectPurchaseOrder             SubjectType = "purchase_order"
	SubjectExpenseClaim              SubjectType = "expense_claim"
	SubjectInternalUsage             SubjectType = "internal_usage"
	SubjectStockCount                SubjectType = "stock_count"
	SubjectReturnDisposition         SubjectType = "return_disposition"
	SubjectDisposal                  SubjectType = "disposal"
	SubjectConsignmentReconciliation SubjectType = "consignment_reconciliation"
	SubjectPriceListVersion          SubjectType = "price_list_version"
	SubjectSalesOrderDiscount        SubjectType = "sales_order_discount"
	SubjectSupplierBillException     SubjectType = "supplier_bill_exception"
	SubjectPurchaseReturn            SubjectType = "purchase_return"
	SubjectPettyCashReplenishment    SubjectType = "petty_cash_replenishment"
	SubjectFiscalPeriodReopen        SubjectType = "fiscal_period_reopen"
)

// ConditionHasDisposal: dokumen memuat baris pemusnahan (disposisi retur level 2 dan 3).
const ConditionHasDisposal = "has_disposal"

// Condition adalah syarat tambahan sebuah level yang dinyatakan dokumen lewat Subject.Flags.
type Condition struct {
	Code  string
	Label string
}

// SubjectTypeInfo adalah keterangan jenis dokumen untuk Pengaturan › Matriks Persetujuan.
type SubjectTypeInfo struct {
	Type  SubjectType
	Label string
	// AmountLabel menjelaskan nominal yang dibandingkan dengan batas kebijakan.
	AmountLabel string
	Conditions  []Condition
}

var subjectTypes = []SubjectTypeInfo{
	{SubjectPurchaseRequest, "Purchase Request", "Total estimasi", nil},
	{SubjectPurchaseOrder, "Purchase Order", "Total PO", nil},
	{SubjectExpenseClaim, "Pengajuan Biaya", "Total pengajuan", nil},
	{SubjectInternalUsage, "Pemakaian Internal", "Nilai HPP", nil},
	{SubjectStockCount, "Penyesuaian Stok", "Nilai selisih absolut", nil},
	{SubjectReturnDisposition, "Disposisi Retur", "Nilai pemusnahan",
		[]Condition{{ConditionHasDisposal, "Ada baris pemusnahan"}}},
	{SubjectDisposal, "Pemusnahan", "Nilai pemusnahan", nil},
	{SubjectConsignmentReconciliation, "Rekonsiliasi Konsinyasi", "Nilai tagihan", nil},
	{SubjectPriceListVersion, "Daftar Harga", "—", nil},
	{SubjectSalesOrderDiscount, "Diskon Sales Order", "Nilai diskon", nil},
	{SubjectSupplierBillException, "Tagihan Pemasok di Luar Toleransi", "Total tagihan", nil},
	{SubjectPurchaseReturn, "Retur Pembelian", "Nilai retur", nil},
	{SubjectPettyCashReplenishment, "Pengisian Kembali Kas Kecil", "Nilai pengisian", nil},
	{SubjectFiscalPeriodReopen, "Buka Kembali Periode", "—", nil},
}

// SubjectTypes mengembalikan jenis dokumen yang dikenal engine, berurutan seperti matriks approval.
func SubjectTypes() []SubjectTypeInfo { return slices.Clone(subjectTypes) }

// LookupSubjectType mengembalikan keterangan jenis dokumen.
func LookupSubjectType(t SubjectType) (SubjectTypeInfo, bool) {
	for _, s := range subjectTypes {
		if s.Type == t {
			return s, true
		}
	}
	return SubjectTypeInfo{}, false
}

// Label mengembalikan label UI jenis dokumen, atau kodenya jika tidak dikenal.
func (t SubjectType) Label() string {
	if s, ok := LookupSubjectType(t); ok {
		return s.Label
	}
	return string(t)
}

// HasCondition memeriksa apakah jenis dokumen mendukung kode syarat.
func (s SubjectTypeInfo) HasCondition(code string) bool {
	return slices.ContainsFunc(s.Conditions, func(c Condition) bool { return c.Code == code })
}

// Subject adalah dokumen yang diajukan oleh modul pemiliknya.
type Subject struct {
	Type SubjectType
	ID   uuid.UUID
	// Label adalah nomor dokumen yang tampil di inbox, mis. PO/2027/01/00012.
	Label string
	// Link adalah path detail dokumen di ERP.
	Link    string
	Summary string
	Amount  money.Amount
	// Division adalah divisi pemilik dokumen; menentukan kebijakan per divisi dan scope Head Divisi.
	Division *uuid.UUID
	// Flags adalah syarat yang terpenuhi dokumen, mis. ConditionHasDisposal.
	Flags []string
}
