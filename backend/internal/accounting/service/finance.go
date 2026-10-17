package service

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/tax"
)

// Permission Kas & Bank, rekonsiliasi, biaya, kas kecil, dan pajak (Desain Role & Approval 3.4).
const (
	PermPaymentRead  = "accounting.payment.read"
	PermPaymentWrite = "accounting.payment.write"
	// PermPaymentPost memposting pembayaran; pembayaran keluar butuh re-autentikasi (★).
	PermPaymentPost = "accounting.payment.post"

	PermBankReconRead   = "accounting.bank_reconciliation.read"
	PermBankReconWrite  = "accounting.bank_reconciliation.write"
	PermBankReconPost   = "accounting.bank_reconciliation.post"
	PermBankReconImport = "accounting.bank_reconciliation.import"

	PermExpenseClaimWrite   = "accounting.expense_claim.write"
	PermExpenseClaimSubmit  = "accounting.expense_claim.submit"
	PermExpenseClaimPost    = "accounting.expense_claim.post"
	PermExpenseClaimApprove = "accounting.expense_claim.approve"

	PermPettyCashRead    = "accounting.petty_cash.read"
	PermPettyCashWrite   = "accounting.petty_cash.write"
	PermPettyCashPost    = "accounting.petty_cash.post"
	PermPettyCashApprove = "accounting.petty_cash.approve"

	PermWithholdingRead   = "accounting.withholding.read"
	PermWithholdingWrite  = "accounting.withholding.write"
	PermWithholdingPost   = "accounting.withholding.post"
	PermWithholdingExport = "accounting.withholding.export"

	PermFinalTaxRead  = "accounting.final_tax.read"
	PermFinalTaxWrite = "accounting.final_tax.write"
	PermFinalTaxPost  = "accounting.final_tax.post"
)

// Jenis entitas audit trail dan jenis pemilik file.
const (
	EntityBankAccount   = "bank_account"
	EntityPayment       = "payment"
	EntityBankStatement = "bank_statement"
	EntityExpenseClaim  = "expense_claim"
	EntityPettyCashFund = "petty_cash_fund"
	EntityPettyCashTxn  = "petty_cash_transaction"
	EntityTaxRate       = "withholding_tax_rate"
	EntityWithholding   = "withholding"
	EntityTaxDeposit    = "tax_deposit"
	EntityFinalTax      = "final_tax"

	FileOwnerBankStatement   = "bank_statement"
	FileOwnerExpenseClaim    = "expense_claim"
	FileOwnerPettyCash       = "petty_cash_transaction"
	FileOwnerPaymentProof    = "payment"
	FileOwnerWithholdingSlip = "withholding_recap"
)

// Jenis nomor dokumen yang diambil lewat DocNumberer (kode modul document).
const (
	DocExpenseClaim = "EXP"
	DocPettyCash    = "KK"
	DocPayment      = "BYR"
)

// ApprovalSubject adalah dokumen yang diajukan ke modul approval (dipetakan adapter ke approval.Subject).
type ApprovalSubject struct {
	Type     string
	ID       uuid.UUID
	Label    string
	Link     string
	Summary  string
	Amount   money.Amount
	Division *uuid.UUID
}

// Approvals mengajukan dokumen ke modul approval di transaksi aktif.
type Approvals interface {
	RequestApproval(ctx context.Context, s ApprovalSubject) error
}

// DocNumberer mengambil nomor dokumen EXP/KK/BYR dari modul document di transaksi aktif.
type DocNumberer interface {
	NextDocNumber(ctx context.Context, docType string, businessDate clock.BusinessDate) (string, error)
}

// FileContent adalah isi file yang diunggah.
type FileContent struct {
	Name        string
	ContentType string
	Data        []byte
}

// Files adalah akses ke modul filestore.
type Files interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
	Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
	Read(ctx context.Context, fileID uuid.UUID, ownerType string) (FileContent, error)
	SaveExport(ctx context.Context, ownerType string, ownerID uuid.UUID, name, contentType string, data []byte) (DownloadLink, error)
}

// DownloadLink adalah URL unduhan bertanda tangan file hasil ekspor.
type DownloadLink struct {
	FileID    uuid.UUID
	URL       string
	ExpiresAt time.Time
}

// Users membaca nama pengguna.
type Users interface {
	UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Notifier mengirim notifikasi in-app.
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// Suppliers memeriksa pemasok penerima pengajuan biaya (opsional).
type Suppliers interface {
	SupplierName(ctx context.Context, id uuid.UUID) (string, bool, error)
}

// FinanceDeps adalah dependensi fitur R1.4. Finance nil = fitur tidak tersedia (test lama).
type FinanceDeps struct {
	Finance    financeStore
	Files      Files
	Approvals  Approvals
	DocNumbers DocNumberer
	Users      Users
	Notifier   Notifier
	Suppliers  Suppliers
	BaseURL    string
}

type financeStore interface {
	ListBankAccounts(ctx context.Context, includeInactive, deleted bool) ([]domain.BankAccount, error)
	GetBankAccount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.BankAccount, error)
	InsertBankAccount(ctx context.Context, b domain.BankAccount, actorID uuid.UUID) (uuid.UUID, error)
	UpdateBankAccount(ctx context.Context, b domain.BankAccount, version int, actorID uuid.UUID) error
	SetBankAccountDeleted(ctx context.Context, id uuid.UUID, version int, at *time.Time, actorID uuid.UUID) error
	BankAccountUsed(ctx context.Context, id uuid.UUID) (bool, error)

	ListPayments(ctx context.Context, f postgres.PaymentFilter, page paging.Page) (paging.Result[domain.Payment], error)
	GetPayment(ctx context.Context, id uuid.UUID, lock bool) (domain.Payment, error)
	InsertPayment(ctx context.Context, p domain.Payment, actorID uuid.UUID) (uuid.UUID, error)
	SavePayment(ctx context.Context, p domain.Payment, version int, actorID uuid.UUID, cancelledAt *time.Time) error

	BankMutations(ctx context.Context, accountID uuid.UUID, from, to clock.BusinessDate, page paging.Page) (money.Amount, int64, []domain.BankMutation, error)
	UnreconciledBankLines(ctx context.Context, accountID uuid.UUID, from, to clock.BusinessDate) ([]domain.MatchCandidate, error)
	BankLineOfEntry(ctx context.Context, entryID, accountID uuid.UUID) (domain.MatchCandidate, error)
	JournalLine(ctx context.Context, id uuid.UUID) (domain.MatchCandidate, uuid.UUID, error)

	InsertStatement(ctx context.Context, sum domain.StatementSummary, rows []domain.StatementRow, actorID uuid.UUID) (uuid.UUID, error)
	ListStatements(ctx context.Context, bankAccountID *uuid.UUID, page paging.Page) (paging.Result[domain.StatementSummary], error)
	GetStatement(ctx context.Context, id uuid.UUID) (domain.StatementSummary, error)
	StatementLines(ctx context.Context, statementID uuid.UUID, lock bool) ([]domain.StatementLine, error)
	GetStatementLine(ctx context.Context, id uuid.UUID, lock bool) (domain.StatementLine, error)
	SaveStatementLine(ctx context.Context, l domain.StatementLine) error

	ListClaims(ctx context.Context, f postgres.ClaimFilter, page paging.Page) (paging.Result[domain.ExpenseClaim], error)
	GetClaim(ctx context.Context, id uuid.UUID, lock bool) (domain.ExpenseClaim, error)
	InsertClaim(ctx context.Context, c domain.ExpenseClaim, actorID uuid.UUID) (uuid.UUID, error)
	SaveClaim(ctx context.Context, c domain.ExpenseClaim, version int, replaceLines bool, actorID uuid.UUID) error
	ClaimsAwaitingPayment(ctx context.Context, claimID *uuid.UUID) ([]postgres.ClaimPayable, error)

	ListFunds(ctx context.Context, includeInactive bool) ([]domain.PettyCashFund, error)
	GetFund(ctx context.Context, id uuid.UUID, lock bool) (domain.PettyCashFund, error)
	InsertFund(ctx context.Context, f domain.PettyCashFund, actorID uuid.UUID) (uuid.UUID, error)
	SaveFund(ctx context.Context, f domain.PettyCashFund, version int, actorID uuid.UUID) error
	ListPettyCash(ctx context.Context, f postgres.PettyCashFilter, page paging.Page) (paging.Result[domain.PettyCashTxn], error)
	GetPettyCash(ctx context.Context, id uuid.UUID, lock bool) (domain.PettyCashTxn, error)
	InsertPettyCash(ctx context.Context, t domain.PettyCashTxn, actorID uuid.UUID) (uuid.UUID, error)
	SavePettyCash(ctx context.Context, t domain.PettyCashTxn, version int, actorID uuid.UUID) error

	ListRates(ctx context.Context, taxType string) ([]domain.RateVersion, error)
	LatestRate(ctx context.Context, t tax.Type, objectCode string) (domain.RateVersion, bool, error)
	EndRate(ctx context.Context, id uuid.UUID, validTo clock.BusinessDate, actorID uuid.UUID) error
	InsertRate(ctx context.Context, r domain.RateVersion, actorID uuid.UUID) (uuid.UUID, error)
	InsertWithholding(ctx context.Context, w domain.WithholdingRecord, actorID uuid.UUID) (uuid.UUID, error)
	ListWithholdings(ctx context.Context, f postgres.WithholdingFilter, page paging.Page) (paging.Result[domain.WithholdingRecord], error)
	WithholdingsBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]domain.WithholdingRecord, error)
	GetWithholdingForUpdate(ctx context.Context, id uuid.UUID) (domain.WithholdingRecord, error)
	SaveWithholding(ctx context.Context, w domain.WithholdingRecord, version int, actorID uuid.UUID) error
	OpenWithholdings(ctx context.Context, t tax.Type, p tax.Period) ([]uuid.UUID, money.Amount, error)
	MarkDeposited(ctx context.Context, ids []uuid.UUID, depositID, actorID uuid.UUID) error
	WithholdingSummary(ctx context.Context, year int) ([]postgres.WithholdingMonth, error)
	InsertDeposit(ctx context.Context, d domain.TaxDeposit, actorID uuid.UUID) (uuid.UUID, time.Time, error)
	DepositExists(ctx context.Context, t tax.Type, p tax.Period) (bool, error)
	ListDeposits(ctx context.Context, year int) ([]domain.TaxDeposit, error)
	ListFinalTaxSettings(ctx context.Context) ([]domain.FinalTaxSetting, error)
	InsertFinalTaxSetting(ctx context.Context, st domain.FinalTaxSetting, actorID uuid.UUID) (uuid.UUID, error)
	MonthlyRevenue(ctx context.Context, year int) (map[time.Month]money.Amount, error)
	ListFinalTaxRuns(ctx context.Context, year int) ([]domain.FinalTaxRun, error)
	InsertFinalTaxRun(ctx context.Context, r domain.FinalTaxRun, actorID uuid.UUID) (uuid.UUID, time.Time, error)
	FinalTaxRunForDeposit(ctx context.Context, p tax.Period) (uuid.UUID, money.Amount, bool, bool, error)
	MarkFinalTaxRunDeposited(ctx context.Context, runID, depositID uuid.UUID) error
}

// --- Kandidat pembayaran untuk rekonsiliasi bank ----------------------------------------------

// PaymentCandidate adalah dokumen terbuka (invoice, tagihan pemasok, pengajuan biaya) yang bisa dilunasi oleh
// satu baris mutasi bank. Disediakan modul pemilik dokumen lewat PaymentCandidateProvider.
type PaymentCandidate struct {
	// Source adalah nama penyedia, mis. "sales.invoice", "procurement.supplier_bill", "accounting.expense_claim".
	Source        string
	DocumentID    uuid.UUID
	DocumentLabel string
	Direction     domain.PaymentDirection
	PartyType     string
	PartyID       *uuid.UUID
	PartyName     string
	// OpenAmount adalah sisa yang belum dibayar (selalu positif).
	OpenAmount   money.Amount
	DocumentDate clock.BusinessDate
	// References adalah teks yang dicari di referensi/keterangan mutasi (nomor dokumen, nomor VA, dll.).
	References []string
	// CounterAccountCode adalah akun lawan pembayaran: piutang (1201/121x) untuk masuk, utang (2101/2103) untuk
	// keluar.
	CounterAccountCode string
}

// CandidateQuery adalah kriteria pencarian kandidat untuk satu file mutasi.
type CandidateQuery struct {
	BankAccountID uuid.UUID
	Direction     domain.PaymentDirection
	DateFrom      clock.BusinessDate
	DateTo        clock.BusinessDate
}

// PaymentCandidateProvider adalah titik sambung modul piutang/utang ke rekonsiliasi bank (S-402). Accounting
// membuat dan memposting pembayaran (A-PB, akun lawan CounterAccountCode) saat usulan dikonfirmasi, lalu
// memanggil SettlePayment di transaksi yang sama agar modul pemilik mencatat pelunasan dokumennya.
type PaymentCandidateProvider interface {
	Source() string
	// PaymentCandidates mengembalikan dokumen terbuka searah (tanpa cek permission).
	PaymentCandidates(ctx context.Context, q CandidateQuery) ([]PaymentCandidate, error)
	// PaymentCandidate membaca ulang satu dokumen saat konfirmasi; ok=false bila sudah tidak terbuka.
	PaymentCandidate(ctx context.Context, documentID uuid.UUID) (PaymentCandidate, bool, error)
	// SettlePayment mencatat pelunasan dokumen oleh pembayaran yang baru diposting.
	SettlePayment(ctx context.Context, c PaymentCandidate, p domain.Payment) error
}

// BookedPayment adalah pembayaran yang sudah dijurnal modul lain (mis. pembayaran pelanggan sales) di akun
// rekening. Dipakai memperkaya kandidat baris jurnal dengan nomor/referensi agar aturan referensi bisa cocok.
type BookedPayment struct {
	JournalEntryID uuid.UUID
	Number         string
	Reference      string
	PartyName      string
}

// BookedPaymentSource mengembalikan pembayaran Diposting modul lain per kode akun Kas & Bank dan rentang tanggal.
type BookedPaymentSource interface {
	BookedPayments(ctx context.Context, accountCode string, from, to clock.BusinessDate) ([]BookedPayment, error)
}

type providerRegistry struct {
	mu     sync.RWMutex
	byID   map[string]PaymentCandidateProvider
	booked []BookedPaymentSource
}

// RegisterBookedPayments mendaftarkan sumber pembayaran tercatat modul lain (saat wiring).
func (s *Service) RegisterBookedPayments(src BookedPaymentSource) {
	s.providers.mu.Lock()
	defer s.providers.mu.Unlock()
	s.providers.booked = append(s.providers.booked, src)
}

func (s *Service) bookedSources() []BookedPaymentSource {
	s.providers.mu.RLock()
	defer s.providers.mu.RUnlock()
	return slices.Clone(s.providers.booked)
}

func newProviderRegistry() *providerRegistry {
	return &providerRegistry{byID: map[string]PaymentCandidateProvider{}}
}

// RegisterPaymentCandidates mendaftarkan penyedia kandidat pembayaran (saat wiring). Registrasi terakhir menang.
func (s *Service) RegisterPaymentCandidates(p PaymentCandidateProvider) {
	s.providers.mu.Lock()
	defer s.providers.mu.Unlock()
	s.providers.byID[p.Source()] = p
}

func (s *Service) provider(source string) (PaymentCandidateProvider, bool) {
	s.providers.mu.RLock()
	defer s.providers.mu.RUnlock()
	p, ok := s.providers.byID[source]
	return p, ok
}

func (s *Service) allProviders() []PaymentCandidateProvider {
	s.providers.mu.RLock()
	defer s.providers.mu.RUnlock()
	out := make([]PaymentCandidateProvider, 0, len(s.providers.byID))
	for _, p := range s.providers.byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source() < out[j].Source() })
	return out
}

// --- Helper -----------------------------------------------------------------------------------

// filterActions menyaring aksi domain dengan permission actor.
func filterActions(ctx context.Context, actions []domain.Action, perms map[domain.Action]string) []string {
	a, _ := actor.From(ctx)
	out := []string{}
	for _, act := range actions {
		if p, ok := perms[act]; ok && (p == "" || a.Has(p)) {
			out = append(out, string(act))
		}
	}
	return out
}

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

func (s *Service) names(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]string {
	if s.Users == nil || len(ids) == 0 {
		return map[uuid.UUID]string{}
	}
	ids = slices.Compact(sortIDs(ids))
	out, err := s.Users.UserNames(ctx, ids)
	if err != nil {
		s.Log.WarnContext(ctx, "user names", "error", err)
		return map[uuid.UUID]string{}
	}
	return out
}

func sortIDs(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return out
}

func (s *Service) fileNames(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]string {
	if s.Files == nil || len(ids) == 0 {
		return map[uuid.UUID]string{}
	}
	out, err := s.Files.Names(ctx, ids)
	if err != nil {
		s.Log.WarnContext(ctx, "file names", "error", err)
		return map[uuid.UUID]string{}
	}
	return out
}

func (s *Service) notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) {
	if s.Notifier == nil || len(userIDs) == 0 {
		return
	}
	if err := s.Notifier.Notify(ctx, userIDs, kind, title, body, link); err != nil {
		s.Log.WarnContext(ctx, "notify", "kind", kind, "error", err)
	}
}

func (s *Service) attach(ctx context.Context, fileID *uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	if fileID == nil || s.Files == nil {
		return nil
	}
	return s.Files.Attach(ctx, *fileID, ownerType, ownerID)
}
