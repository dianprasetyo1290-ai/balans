package domain

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// PeriodPendingApproval: permintaan buka kembali menunggu approval; periode tetap terkunci.
const PeriodPendingApproval PeriodStatus = "pending_approval"

// Permission tutup buku (Desain Role & Approval 3.4): Finance RW (checklist), Head F&A P (tutup) + reopen ★.
const (
	PermFiscalPeriodPost   = "accounting.fiscal_period.post"
	PermFiscalPeriodReopen = "accounting.fiscal_period.reopen"
)

// ApprovalSubjectFiscalPeriodReopen adalah jenis dokumen approval buka kembali periode (Head F&A → Direksi).
// Nilainya sama dengan approval.SubjectFiscalPeriodReopen; accounting tidak mengimpor approval.
const ApprovalSubjectFiscalPeriodReopen = "fiscal_period_reopen"

// CloseStepCode adalah kode langkah checklist tutup buku (BRD 5.15).
type CloseStepCode string

const (
	StepOperationalDocuments        CloseStepCode = "operational_documents"
	StepConsignmentVendingRecon     CloseStepCode = "consignment_vending_reconciliation"
	StepSettlementReconciliation    CloseStepCode = "settlement_reconciliation"
	StepBankPettyCashReconciliation CloseStepCode = "bank_petty_cash_reconciliation"
	StepPayroll                     CloseStepCode = "payroll"
	StepDepreciation                CloseStepCode = "depreciation"
	StepTaxRecap                    CloseStepCode = "tax_recap"
	StepInventoryReview             CloseStepCode = "inventory_review"
	StepFinancialReview             CloseStepCode = "financial_review"
)

// CloseStep adalah definisi satu langkah: urutan, aktivitas, dan PIC default BRD 5.15.
type CloseStep struct {
	Code     CloseStepCode
	Order    int
	Title    string
	PICLabel string
	// Automatic: diselesaikan sistem (penyusutan), user tetap bisa melihat statusnya.
	Automatic bool
}

// CloseSteps adalah 9 langkah tutup buku bulanan berurutan (BRD 5.15).
var CloseSteps = []CloseStep{
	{StepOperationalDocuments, 1, "Semua dokumen operasional bulan berjalan sudah diposting (GR, DO, invoice, retur, pemakaian internal)", "Semua divisi", false},
	{StepConsignmentVendingRecon, 2, "Rekonsiliasi konsinyasi & vending (stock count + Berita Acara)", "Sales MT, Admin Vending", false},
	{StepSettlementReconciliation, 3, "Rekonsiliasi settlement Midtrans & marketplace", "Finance & Accounting", false},
	{StepBankPettyCashReconciliation, 4, "Rekonsiliasi bank & kas kecil", "Finance & Accounting", false},
	{StepPayroll, 5, "Payroll & jurnal payroll", "HRGA, Finance & Accounting", false},
	{StepDepreciation, 6, "Penyusutan aset tetap (otomatis)", "Sistem", true},
	{StepTaxRecap, 7, "Rekap PPh yang dipotong dan PPh Final (bila berlaku)", "Finance & Accounting", false},
	{StepInventoryReview, 8, "Review selisih persediaan, stok kedaluwarsa, dan Barang Diterima Belum Ditagih", "Finance & Accounting, Head Logistik", false},
	{StepFinancialReview, 9, "Review laporan keuangan, lalu tutup periode", "Head Finance & Accounting", false},
}

// StepByCode mencari definisi langkah.
func StepByCode(code CloseStepCode) (CloseStep, bool) {
	for _, s := range CloseSteps {
		if s.Code == code {
			return s, true
		}
	}
	return CloseStep{}, false
}

// CloseStepStatus adalah status langkah (UI: Belum, Selesai, Tidak Berlaku).
type CloseStepStatus string

const (
	StepPending CloseStepStatus = "pending"
	StepDone    CloseStepStatus = "done"
	StepSkipped CloseStepStatus = "skipped"
)

// Finished: langkah tidak lagi menahan tutup periode.
func (s CloseStepStatus) Finished() bool { return s == StepDone || s == StepSkipped }

// CloseCheck adalah hasil pemeriksaan otomatis satu langkah (hook modul lain atau accounting sendiri).
type CloseCheck struct {
	// Source adalah nama pemeriksa, mis. "Jurnal Umum draft".
	Source  string
	OK      bool
	Message string
	// Link adalah path ERP untuk menindaklanjuti, mis. /finance/journals?status=draft.
	Link string
}

// ChecklistItem adalah status satu langkah checklist suatu periode.
type ChecklistItem struct {
	ID          uuid.UUID
	PeriodID    uuid.UUID
	Step        CloseStep
	Status      CloseStepStatus
	PICUserID   *uuid.UUID
	CompletedBy *uuid.UUID
	CompletedAt *time.Time
	Note        string
	Checks      []CloseCheck
	UpdatedAt   time.Time
	Version     int
}

// ChecksPassed: semua pemeriksaan otomatis lolos.
func (c ChecklistItem) ChecksPassed() bool {
	for _, chk := range c.Checks {
		if !chk.OK {
			return false
		}
	}
	return true
}

// ChecklistUpdate adalah perubahan status/PIC/catatan satu langkah.
type ChecklistUpdate struct {
	Status    CloseStepStatus
	PICUserID *uuid.UUID
	Note      string
}

// Apply mengubah langkah. Langkah selesai butuh semua pemeriksaan otomatis lolos; Tidak Berlaku wajib catatan;
// langkah 9 diselesaikan saat Tutup Periode.
func (c *ChecklistItem) Apply(u ChecklistUpdate, by uuid.UUID, at time.Time) error {
	note := strings.TrimSpace(u.Note)
	fields := map[string]string{}
	switch u.Status {
	case StepPending, StepDone, StepSkipped:
	default:
		fields["status"] = "Status langkah tidak dikenal"
	}
	if len(note) > 1000 {
		fields["note"] = "Catatan paling panjang 1000 karakter"
	}
	if u.Status == StepSkipped && note == "" {
		fields["note"] = "Isi alasan langkah tidak berlaku"
	}
	if c.Step.Code == StepFinancialReview && u.Status != StepPending && u.Status != c.Status {
		fields["status"] = "Langkah 9 selesai otomatis saat Tutup Periode"
	}
	if u.Status == StepDone && u.Status != c.Status && !c.ChecksPassed() {
		for _, chk := range c.Checks {
			if !chk.OK {
				fields["status"] = chk.Message
				break
			}
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	if u.Status != c.Status {
		if u.Status == StepPending {
			c.CompletedBy, c.CompletedAt = nil, nil
		} else {
			c.CompletedBy, c.CompletedAt = &by, &at
		}
	}
	c.Status, c.PICUserID, c.Note = u.Status, u.PICUserID, note
	return nil
}

// CloseBlockers mengembalikan alasan periode belum bisa ditutup dari checklist: langkah 1–8 harus Selesai atau
// Tidak Berlaku dan pemeriksaan otomatis lolos.
func CloseBlockers(items []ChecklistItem) []string {
	var out []string
	for _, it := range items {
		if it.Step.Code == StepFinancialReview {
			continue
		}
		if !it.Status.Finished() {
			out = append(out, "Langkah "+strconv.Itoa(it.Step.Order)+" belum selesai: "+it.Step.Title)
			continue
		}
		if it.Status == StepDone {
			for _, chk := range it.Checks {
				if !chk.OK {
					out = append(out, "Langkah "+strconv.Itoa(it.Step.Order)+": "+chk.Message)
				}
			}
		}
	}
	return out
}

// Close menutup periode terbuka (Desain Status Dokumen 6.4).
func (p *Period) Close(at time.Time, by uuid.UUID) error {
	if p.Status != PeriodOpen {
		return apperr.InvalidStateTransition("fiscal_period", string(p.Status), string(PeriodClosed))
	}
	p.Status, p.ClosedAt, p.ClosedBy = PeriodClosed, &at, &by
	return nil
}

// RequestReopen memindahkan periode tertutup ke menunggu persetujuan buka kembali.
func (p *Period) RequestReopen() error {
	if p.Status != PeriodClosed {
		return apperr.InvalidStateTransition("fiscal_period", string(p.Status), string(PeriodPendingApproval))
	}
	p.Status = PeriodPendingApproval
	return nil
}

// ApproveReopen membuka kembali periode setelah approval terakhir.
func (p *Period) ApproveReopen() error {
	if p.Status != PeriodPendingApproval {
		return apperr.InvalidStateTransition("fiscal_period", string(p.Status), string(PeriodOpen))
	}
	p.Status, p.ClosedAt, p.ClosedBy = PeriodOpen, nil, nil
	p.ReopenedCount++
	return nil
}

// RejectReopen mengembalikan periode ke Ditutup.
func (p *Period) RejectReopen() error {
	if p.Status != PeriodPendingApproval {
		return apperr.InvalidStateTransition("fiscal_period", string(p.Status), string(PeriodClosed))
	}
	p.Status = PeriodClosed
	return nil
}

// ValidateReopenReason memeriksa alasan buka kembali.
func ValidateReopenReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1000 {
		return "", apperr.Validation(map[string]string{"reason": "Alasan buka kembali wajib diisi (maks. 1000 karakter)"})
	}
	return reason, nil
}

// ReopenStatus adalah status permintaan buka kembali.
type ReopenStatus string

const (
	ReopenPending  ReopenStatus = "pending_approval"
	ReopenApproved ReopenStatus = "approved"
	ReopenRejected ReopenStatus = "rejected"
)

// ReopenRequest adalah permintaan buka kembali periode (ACC-14).
type ReopenRequest struct {
	ID           uuid.UUID
	PeriodID     uuid.UUID
	Reason       string
	Status       ReopenStatus
	RequestedBy  uuid.UUID
	RequestedAt  time.Time
	DecidedBy    *uuid.UUID
	DecidedAt    *time.Time
	DecisionNote string
}

// PeriodAction adalah aksi Tutup Buku yang boleh dilakukan user.
type PeriodAction string

const (
	PeriodActionClose         PeriodAction = "close"
	PeriodActionRequestReopen PeriodAction = "request_reopen"
	PeriodActionEditChecklist PeriodAction = "edit_checklist"
)

// AllowedActions untuk tombol halaman Tutup Buku.
func (p Period) AllowedActions(has func(string) bool) []PeriodAction {
	out := []PeriodAction{}
	switch p.Status {
	case PeriodOpen:
		if has(PermFiscalPeriodWrite) || has(PermFiscalPeriodPost) {
			out = append(out, PeriodActionEditChecklist)
		}
		if has(PermFiscalPeriodPost) {
			out = append(out, PeriodActionClose)
		}
	case PeriodClosed:
		if has(PermFiscalPeriodReopen) {
			out = append(out, PeriodActionRequestReopen)
		}
	}
	return out
}

// PermFiscalPeriodWrite: Finance mengisi checklist (Desain Role & Approval 3.4).
const PermFiscalPeriodWrite = "accounting.fiscal_period.write"
