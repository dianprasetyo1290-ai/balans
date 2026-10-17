package accounting

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/service"
)

// --- API publik Kas & Bank, rekonsiliasi, biaya, kas kecil, dan pajak (R1.4) -----------------

type (
	// BankAccount adalah rekening kas/bank Balans (terhubung ke akun 11xx).
	BankAccount = domain.BankAccount
	// Payment adalah pembayaran masuk/keluar (BYR, jurnal A-PB).
	Payment = domain.Payment
	// PaymentDirection adalah arah pembayaran.
	PaymentDirection = domain.PaymentDirection
	// PaymentRequest adalah pembayaran dokumen modul lain yang dicatat lewat RecordPayment.
	PaymentRequest = service.PaymentRequest

	// PaymentCandidate adalah dokumen terbuka yang bisa dilunasi baris mutasi bank.
	PaymentCandidate = service.PaymentCandidate
	// CandidateQuery adalah kriteria pencarian kandidat pembayaran.
	CandidateQuery = service.CandidateQuery
	// PaymentCandidateProvider adalah titik sambung piutang/utang ke rekonsiliasi bank (S-402).
	PaymentCandidateProvider = service.PaymentCandidateProvider

	// BookedPayment dan BookedPaymentSource memperkaya kandidat rekonsiliasi dengan pembayaran modul lain.
	BookedPayment       = service.BookedPayment
	BookedPaymentSource = service.BookedPaymentSource

	// WithholdingRequest adalah permintaan pemotongan PPh satu dokumen sumber.
	WithholdingRequest = service.WithholdingRequest
	// WithholdingResult adalah data bukti potong dan akun utang PPh hasil pemotongan.
	WithholdingResult = service.WithholdingResult
	// WithholdingSubject adalah dokumen sumber dan penerima penghasilan yang dipotong.
	WithholdingSubject = domain.WithholdingSubject
	// WithholdingRecord adalah data bukti potong.
	WithholdingRecord = domain.WithholdingRecord

	// Approvals, DocNumberer, Files, FileContent, DownloadLink, Users, Notifier, Suppliers diisi adapter saat
	// wiring (ConnectFinance); ApprovalSubject dideklarasikan di close.go.
	Approvals    = service.Approvals
	DocNumberer  = service.DocNumberer
	Files        = service.Files
	FileContent  = service.FileContent
	DownloadLink = service.DownloadLink
	Users        = service.Users
	Notifier     = service.Notifier
	Suppliers    = service.Suppliers
)

// Arah pembayaran, sumber kandidat bawaan, jenis pemilik file, dan permission yang dipakai wiring.
const (
	PaymentIncoming = domain.PaymentIncoming
	PaymentOutgoing = domain.PaymentOutgoing

	SourceClaimCandidates = service.SourceClaimCandidates

	FileOwnerBankStatement   = service.FileOwnerBankStatement
	FileOwnerExpenseClaim    = service.FileOwnerExpenseClaim
	FileOwnerPettyCash       = service.FileOwnerPettyCash
	FileOwnerPaymentProof    = service.FileOwnerPaymentProof
	FileOwnerWithholdingSlip = service.FileOwnerWithholdingSlip

	PermPaymentRead        = service.PermPaymentRead
	PermPaymentWrite       = service.PermPaymentWrite
	PermBankReconRead      = service.PermBankReconRead
	PermBankReconImport    = service.PermBankReconImport
	PermExpenseClaimRead   = service.PermExpenseClaimRead
	PermExpenseClaimWrite  = service.PermExpenseClaimWrite
	PermPettyCashRead      = service.PermPettyCashRead
	PermPettyCashWrite     = service.PermPettyCashWrite
	PermWithholdingRead    = service.PermWithholdingRead
	PermWithholdingExport  = service.PermWithholdingExport
	ContentTypeBankCSV     = service.ContentTypeCSV
	ContentTypeBankXLSX    = service.ContentTypeXLSX
	ExpensePayableAccount  = domain.ExpensePayableCode
	TradePayableAccount    = "2101"
	TradeReceivableAccount = "1201"
)

// FinanceConnections adalah dependensi R1.4 dari modul lain yang disusun setelah accounting.
type FinanceConnections struct {
	Files      Files
	Approvals  Approvals
	DocNumbers DocNumberer
	Users      Users
	Notifier   Notifier
	Suppliers  Suppliers
	BaseURL    string
}

// ConnectFinance memasang dependensi Kas & Bank, rekonsiliasi, biaya, kas kecil, dan pajak (saat wiring).
func (m *Module) ConnectFinance(c FinanceConnections) {
	d := &m.svc.FinanceDeps
	d.Files, d.Approvals, d.DocNumbers, d.Users, d.Notifier, d.Suppliers, d.BaseURL =
		c.Files, c.Approvals, c.DocNumbers, c.Users, c.Notifier, c.Suppliers, c.BaseURL
}

// RegisterPaymentCandidates mendaftarkan penyedia kandidat pembayaran untuk rekonsiliasi bank, mis. invoice
// terbuka (sales) atau tagihan pemasok (procurement). Registrasi terakhir per Source menang.
func (m *Module) RegisterPaymentCandidates(p PaymentCandidateProvider) {
	m.svc.RegisterPaymentCandidates(p)
}

// RegisterBookedPayments mendaftarkan sumber pembayaran tercatat modul lain (mis. pembayaran pelanggan sales) agar
// nomor/referensinya ikut dicocokkan dengan mutasi bank.
func (m *Module) RegisterBookedPayments(src BookedPaymentSource) { m.svc.RegisterBookedPayments(src) }

// RecordPayment membuat dan memposting pembayaran dokumen modul lain (nomor BYR, jurnal A-PB) di transaksi aktif.
// Pembayaran keluar butuh re-autentikasi `accounting.payment.post` (★).
func (m *Module) RecordPayment(ctx context.Context, req PaymentRequest) (Payment, error) {
	return m.svc.RecordPayment(ctx, req)
}

// BankAccounts mengembalikan rekening kas/bank aktif untuk pilihan rekening dokumen modul lain (tanpa cek
// permission).
func (m *Module) BankAccounts(ctx context.Context) ([]BankAccount, error) {
	return m.svc.Finance.ListBankAccounts(ctx, false, false)
}

// QuoteWithholding menghitung PPh potong (PPh 21/23/4(2)) dengan tarif effective-dated pada tanggal dokumen
// tanpa menyimpan, mis. pratinjau tagihan pemasok.
func (m *Module) QuoteWithholding(ctx context.Context, req WithholdingRequest) (WithholdingResult, error) {
	return m.svc.QuoteWithholding(ctx, req)
}

// Withhold menghitung dan mencatat data bukti potong di transaksi aktif. Dokumen sumber memposting jurnalnya
// sendiri, mis. tagihan pemasok A-18 `withholding_value` ke WithholdingResult.AccountCode (2302 PPh 23).
func (m *Module) Withhold(ctx context.Context, req WithholdingRequest) (WithholdingResult, error) {
	return m.svc.Withhold(ctx, req)
}

// CancelWithholdings membatalkan data bukti potong dokumen sumber yang dibatalkan (ditolak bila sudah disetor).
func (m *Module) CancelWithholdings(ctx context.Context, sourceType string, sourceID uuid.UUID) error {
	return m.svc.CancelWithholdings(ctx, sourceType, sourceID)
}

// OnExpenseClaimDecided dipanggil handler approval `expense_claim`.
func (m *Module) OnExpenseClaimDecided(ctx context.Context, id uuid.UUID, approved bool, note string) error {
	return m.svc.OnExpenseClaimDecided(ctx, id, approved, note)
}

// OnPettyCashDecided dipanggil handler approval `petty_cash_replenishment`.
func (m *Module) OnPettyCashDecided(ctx context.Context, id uuid.UUID, approved bool, note string) error {
	return m.svc.OnPettyCashDecided(ctx, id, approved, note)
}

// AuditEntities memetakan entitas audit R1.4 ke permission baca untuk tab Riwayat.
func AuditEntities() map[string]string {
	return map[string]string{
		service.EntityBankAccount: service.PermPaymentRead, service.EntityPayment: service.PermPaymentRead,
		service.EntityBankStatement: service.PermBankReconRead, service.EntityExpenseClaim: service.PermExpenseClaimRead,
		service.EntityPettyCashFund: service.PermPettyCashRead, service.EntityPettyCashTxn: service.PermPettyCashRead,
		service.EntityTaxRate: service.PermWithholdingRead, service.EntityWithholding: service.PermWithholdingRead,
		service.EntityTaxDeposit: service.PermWithholdingRead, service.EntityFinalTax: service.PermFinalTaxRead,
	}
}
