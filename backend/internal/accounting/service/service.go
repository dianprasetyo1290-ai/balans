// Package service berisi use case modul accounting: COA, dimensi, periode, posting rule & Poster, Jurnal Umum,
// Balik Jurnal, dan Buku Besar (ACC-01, ACC-02, ACC-08, ACC-13, ACC-17; Technical Guideline 5.8).
package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// Permission modul accounting (Desain Role & Approval bagian 3).
const (
	PermAccountRead      = "accounting.account.read"
	PermAccountWrite     = "accounting.account.write"
	PermPostingRuleRead  = "accounting.posting_rule.read"
	PermDimensionRead    = "accounting.dimension.read"
	PermDimensionWrite   = "accounting.dimension.write"
	PermFiscalPeriodRead = "accounting.fiscal_period.read"
	// PermFiscalPeriodWrite membuat periode berikutnya lebih awal; tutup/buka kembali di R1.4.
	PermFiscalPeriodWrite = "accounting.fiscal_period.write"
	PermJournalRead       = domain.PermJournalRead
	PermJournalWrite      = domain.PermJournalWrite
	PermJournalPost       = domain.PermJournalPost
	PermJournalReverse    = domain.PermJournalReverse
)

// Numberer mengambil nomor dokumen JV dari modul document di transaksi aktif.
type Numberer interface {
	NextJournalNumber(ctx context.Context, businessDate clock.BusinessDate) (string, error)
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	ListAccounts(ctx context.Context, includeInactive bool) ([]domain.Account, error)
	GetAccount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Account, error)
	AccountsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error)
	AccountsByCodes(ctx context.Context, codes []string) (map[string]domain.Account, error)
	AccountAncestors(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	InsertAccount(ctx context.Context, a domain.Account, actorID uuid.UUID) (domain.Account, error)
	UpdateAccount(ctx context.Context, a domain.Account, version int, actorID uuid.UUID) (domain.Account, error)
	DeleteAccount(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error
	AccountUsage(ctx context.Context, id uuid.UUID) (domain.AccountUsage, error)

	ListDimensions(ctx context.Context, kind domain.DimensionKind, f postgres.DimensionFilter) ([]domain.DimensionValue, error)
	GetDimensionForUpdate(ctx context.Context, kind domain.DimensionKind, id uuid.UUID) (domain.DimensionValue, error)
	InsertDimension(ctx context.Context, v domain.DimensionValue, actorID uuid.UUID) (domain.DimensionValue, error)
	UpdateDimension(ctx context.Context, v domain.DimensionValue, version int, actorID uuid.UUID) (domain.DimensionValue, error)
	DeleteDimension(ctx context.Context, kind domain.DimensionKind, id uuid.UUID, version int, actorID uuid.UUID) error
	DimensionUsage(ctx context.Context, kind domain.DimensionKind, id uuid.UUID) (bool, int64, error)
	CheckDimensions(ctx context.Context, d domain.Dimensions, at clock.BusinessDate) (domain.DimensionCheck, error)
	DimensionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]postgres.DimensionName, error)

	ListPeriods(ctx context.Context, year int) ([]domain.Period, error)
	EnsurePeriod(ctx context.Context, k domain.PeriodKey, actorID uuid.UUID) error
	GetPeriod(ctx context.Context, k domain.PeriodKey, shared bool) (domain.Period, error)
	ClosedPeriodAfter(ctx context.Context, k domain.PeriodKey) (bool, error)

	GetPostingRule(ctx context.Context, code string, at clock.BusinessDate) (domain.PostingRule, error)
	ListPostingRules(ctx context.Context, at clock.BusinessDate) ([]domain.PostingRule, map[string][]postgres.RuleAccountView, error)

	InsertEntry(ctx context.Context, e domain.Entry, actorID uuid.UUID) error
	ReplaceDraft(ctx context.Context, e domain.Entry, version int, actorID uuid.UUID) error
	PostEntry(ctx context.Context, e domain.Entry, version int, actorID uuid.UUID) error
	DeleteDraft(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error
	LockEntry(ctx context.Context, id uuid.UUID) (int, error)
	EntryIDBySource(ctx context.Context, sourceType string, sourceID uuid.UUID, ruleCode string) (uuid.UUID, bool, error)
	GetEntry(ctx context.Context, id uuid.UUID) (domain.Entry, error)
	ListEntries(ctx context.Context, f postgres.EntryFilter, page paging.Page) (paging.Result[domain.Entry], error)
	LedgerSummary(ctx context.Context, f postgres.LedgerFilter) (postgres.LedgerSummary, error)
	LedgerLines(ctx context.Context, f postgres.LedgerFilter, page paging.Page) ([]postgres.LedgerLine, error)
}

// Deps adalah dependensi Service.
type Deps struct {
	Store    store
	Tx       txRunner
	Numberer Numberer
	Audit    audit.Recorder
	Clock    clock.Clock
	Log      *slog.Logger
}

// Service adalah use case modul accounting.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }
