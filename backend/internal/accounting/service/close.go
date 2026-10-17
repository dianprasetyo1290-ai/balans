package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// closeStore adalah repository aset tetap, tutup buku, saldo awal, dan laporan (R1.4).
type closeStore interface {
	ListAssetClasses(ctx context.Context, includeInactive bool) ([]domain.AssetClass, error)
	GetAssetClass(ctx context.Context, id uuid.UUID) (domain.AssetClass, error)
	InsertAssetClass(ctx context.Context, c domain.AssetClass, actorID uuid.UUID) (uuid.UUID, error)
	UpdateAssetClass(ctx context.Context, c domain.AssetClass, version int, actorID uuid.UUID) error
	ListFixedAssets(ctx context.Context, f postgres.FixedAssetFilter, page paging.Page) (paging.Result[domain.FixedAsset], error)
	GetFixedAsset(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.FixedAsset, error)
	GetFixedAssetByCode(ctx context.Context, code string) (domain.FixedAsset, error)
	InsertFixedAsset(ctx context.Context, a domain.FixedAsset, actorID uuid.UUID) (uuid.UUID, error)
	UpdateFixedAsset(ctx context.Context, a domain.FixedAsset, version int, actorID uuid.UUID) error
	DeleteFixedAsset(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error
	DepreciableAssets(ctx context.Context, periodID uuid.UUID, start clock.BusinessDate) ([]domain.FixedAsset, error)
	PendingDepreciationCount(ctx context.Context, periodID uuid.UUID, start clock.BusinessDate) (int64, error)
	AssetClassTotals(ctx context.Context) ([]postgres.AssetClassTotal, error)
	UpsertDepreciationRun(ctx context.Context, periodID, actorID uuid.UUID) (uuid.UUID, error)
	RefreshDepreciationRunTotals(ctx context.Context, id uuid.UUID) error
	GetDepreciationRun(ctx context.Context, id uuid.UUID) (domain.DepreciationRun, error)
	DepreciationRunByPeriod(ctx context.Context, periodID uuid.UUID) (domain.DepreciationRun, bool, error)
	ListDepreciationRuns(ctx context.Context, page paging.Page) (paging.Result[domain.DepreciationRun], error)
	InsertDepreciationLine(ctx context.Context, l domain.DepreciationLine, actorID uuid.UUID) error
	ListDepreciationLines(ctx context.Context, runID, assetID *uuid.UUID) ([]domain.DepreciationLine, error)

	AdvisoryLockPeriod(ctx context.Context, process string, k domain.PeriodKey) error
	GetPeriodByID(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Period, error)
	SavePeriodStatus(ctx context.Context, p domain.Period, version int, actorID uuid.UUID) error
	OpenPeriodBefore(ctx context.Context, k domain.PeriodKey) (bool, error)
	EnsureChecklist(ctx context.Context, periodID uuid.UUID, actorID uuid.UUID) error
	ListChecklist(ctx context.Context, periodID uuid.UUID) ([]domain.ChecklistItem, error)
	GetChecklistStep(ctx context.Context, periodID uuid.UUID, code domain.CloseStepCode) (domain.ChecklistItem, error)
	SaveChecklistStep(ctx context.Context, it domain.ChecklistItem, version int, actorID uuid.UUID) error
	InsertReopenRequest(ctx context.Context, periodID uuid.UUID, reason string, actorID uuid.UUID) (uuid.UUID, error)
	GetReopenRequest(ctx context.Context, id uuid.UUID) (domain.ReopenRequest, error)
	DecideReopenRequest(ctx context.Context, id uuid.UUID, status domain.ReopenStatus, by uuid.UUID, at time.Time, note string) error
	ListReopenRequests(ctx context.Context, periodID uuid.UUID) ([]domain.ReopenRequest, error)
	DraftJournalCount(ctx context.Context, from, to clock.BusinessDate) (int64, error)

	GetOpeningSet(ctx context.Context) (domain.OpeningSet, bool, error)
	LockOpeningSet(ctx context.Context, id uuid.UUID) (int, error)
	InsertOpeningSet(ctx context.Context, cutover clock.BusinessDate, actorID uuid.UUID) (uuid.UUID, error)
	SaveOpeningSet(ctx context.Context, o domain.OpeningSet, version int, actorID uuid.UUID) error
	ListOpeningLines(ctx context.Context, setID uuid.UUID) ([]domain.OpeningLine, error)
	GetOpeningLine(ctx context.Context, id uuid.UUID) (domain.OpeningLine, error)
	UpsertOpeningLine(ctx context.Context, setID uuid.UUID, l domain.OpeningLine, actorID uuid.UUID) (uuid.UUID, error)
	DeleteOpeningLine(ctx context.Context, setID, id uuid.UUID) error
	OpeningLineNets(ctx context.Context, setID uuid.UUID) (map[string]money.Amount, error)
	UpsertOpeningItem(ctx context.Context, setID uuid.UUID, it domain.OpeningItem, actorID uuid.UUID) (uuid.UUID, error)
	ListOpeningItems(ctx context.Context, f postgres.OpeningItemFilter, page paging.Page) (postgres.OpeningItemPage, error)
	GetOpeningItem(ctx context.Context, id uuid.UUID) (domain.OpeningItem, error)
	SettlementExists(ctx context.Context, itemID uuid.UUID, sourceType string, sourceID uuid.UUID) (bool, error)
	RecordSettlement(ctx context.Context, it domain.OpeningItem, sourceType string, sourceID uuid.UUID, date clock.BusinessDate, amount money.Amount, actorID uuid.UUID) error
	OpeningItemTotals(ctx context.Context) ([]postgres.OpeningItemTotal, error)
	JournalImportRef(ctx context.Context, importKey, reference string) (uuid.UUID, bool, error)
	InsertJournalImportRef(ctx context.Context, importKey, reference string, entryID uuid.UUID) error

	TrialBalance(ctx context.Context, f postgres.TrialBalanceFilter) ([]domain.TrialBalanceRow, error)
	AccountNets(ctx context.Context, asOf clock.BusinessDate) (map[string]money.Amount, error)
	LedgerExportLines(ctx context.Context, f postgres.LedgerExportFilter) ([]postgres.LedgerExportLine, error)
}

// ApprovalRequester mengajukan dokumen ke modul approval di transaksi aktif.
type ApprovalRequester interface {
	RequestApproval(ctx context.Context, s ApprovalSubject) error
}

// PartyLookup mencari pihak sub-ledger berdasarkan kode (impor piutang/utang terbuka).
type PartyLookup interface {
	// PartyByCode mencari pelanggan atau mitra (receivable) / pemasok (payable). partyType hasil: customer,
	// partner, atau supplier.
	PartyByCode(ctx context.Context, itemType domain.OpeningItemType, code string) (partyType string, id uuid.UUID, ok bool, err error)
}

// StockValuer menghitung nilai persediaan (Σ qty × HPP) per kode akun persediaan s.d. tanggal (rekonsiliasi
// cut-over); diisi adapter inventory + warehouse + catalog.
type StockValuer interface {
	InventoryValueByAccount(ctx context.Context, asOf clock.BusinessDate) (map[string]money.Amount, error)
}

// CloseCheckFunc adalah hook pemeriksaan otomatis langkah checklist dari modul lain untuk rentang periode.
type CloseCheckFunc func(ctx context.Context, from, to clock.BusinessDate) ([]domain.CloseCheck, error)

// UserDirectory membaca nama user (PIC checklist, pengaju buka kembali) dari modul identity.
type UserDirectory interface {
	UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

type extensions struct {
	mu          sync.RWMutex
	users       UserDirectory
	approvals   ApprovalRequester
	parties     PartyLookup
	stock       StockValuer
	closeChecks map[domain.CloseStepCode][]CloseCheckFunc
}

// SetApprovals memasang pengaju approval (buka kembali periode).
func (s *Service) SetApprovals(a ApprovalRequester) {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	s.ext.approvals = a
}

// SetUserDirectory memasang pembaca nama user.
func (s *Service) SetUserDirectory(u UserDirectory) {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	s.ext.users = u
}

// UserNames mengembalikan nama user untuk tampilan; kosong bila direktori belum dipasang.
func (s *Service) UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	s.ext.mu.RLock()
	users := s.ext.users
	s.ext.mu.RUnlock()
	if users == nil || len(ids) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	return users.UserNames(ctx, ids)
}

// SetParties memasang pencari pihak untuk impor piutang/utang terbuka.
func (s *Service) SetParties(p PartyLookup) {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	s.ext.parties = p
}

// SetStockValuer memasang penghitung nilai persediaan untuk rekonsiliasi cut-over.
func (s *Service) SetStockValuer(v StockValuer) {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	s.ext.stock = v
}

// RegisterCloseCheck menambah pemeriksaan otomatis pada langkah checklist (mis. dokumen operasional draft dari
// modul pengadaan/penjualan pada langkah 1).
func (s *Service) RegisterCloseCheck(step domain.CloseStepCode, fn CloseCheckFunc) {
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	if s.ext.closeChecks == nil {
		s.ext.closeChecks = map[domain.CloseStepCode][]CloseCheckFunc{}
	}
	s.ext.closeChecks[step] = append(s.ext.closeChecks[step], fn)
}

func (s *Service) hooks(step domain.CloseStepCode) []CloseCheckFunc {
	s.ext.mu.RLock()
	defer s.ext.mu.RUnlock()
	return append([]CloseCheckFunc(nil), s.ext.closeChecks[step]...)
}

// Checklist adalah halaman Tutup Buku satu periode.
type Checklist struct {
	Period         domain.Period
	Items          []domain.ChecklistItem
	Blockers       []string
	Reopens        []domain.ReopenRequest
	AllowedActions []domain.PeriodAction
}

// runChecks menjalankan pemeriksaan bawaan dan hook modul lain per langkah.
func (s *Service) runChecks(ctx context.Context, p domain.Period, items []domain.ChecklistItem) error {
	from, to := p.Start(), p.End()
	for i := range items {
		it := &items[i]
		it.Checks = nil
		switch it.Step.Code {
		case domain.StepOperationalDocuments:
			n, err := s.Store.DraftJournalCount(ctx, from, to)
			if err != nil {
				return err
			}
			chk := domain.CloseCheck{Source: "Jurnal Umum", OK: n == 0, Message: "Tidak ada Jurnal Umum draft di periode ini",
				Link: fmt.Sprintf("/finance/journals?status=draft&dateFrom=%s&dateTo=%s", from, to)}
			if n > 0 {
				chk.Message = fmt.Sprintf("Masih ada %d Jurnal Umum draft di periode ini", n)
			}
			it.Checks = append(it.Checks, chk)
		case domain.StepDepreciation:
			n, err := s.Store.PendingDepreciationCount(ctx, p.ID, from)
			if err != nil {
				return err
			}
			chk := domain.CloseCheck{Source: "Penyusutan", OK: n == 0, Message: "Penyusutan aset tetap periode ini sudah dijalankan",
				Link: "/finance/depreciation"}
			if n > 0 {
				chk.Message = fmt.Sprintf("%d aset tetap belum disusutkan di periode ini", n)
			}
			it.Checks = append(it.Checks, chk)
		}
		for _, fn := range s.hooks(it.Step.Code) {
			checks, err := fn(ctx, from, to)
			if err != nil {
				return err
			}
			it.Checks = append(it.Checks, checks...)
		}
	}
	return nil
}

// checklistItems mengembalikan 9 langkah; langkah yang belum tersimpan ditampilkan Belum.
func (s *Service) checklistItems(ctx context.Context, p domain.Period) ([]domain.ChecklistItem, error) {
	saved, err := s.Store.ListChecklist(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	byCode := map[domain.CloseStepCode]domain.ChecklistItem{}
	for _, it := range saved {
		byCode[it.Step.Code] = it
	}
	items := make([]domain.ChecklistItem, len(domain.CloseSteps))
	for i, st := range domain.CloseSteps {
		if it, ok := byCode[st.Code]; ok {
			items[i] = it
		} else {
			items[i] = domain.ChecklistItem{PeriodID: p.ID, Step: st, Status: domain.StepPending}
		}
	}
	return items, s.runChecks(ctx, p, items)
}

// CloseChecklist mengembalikan checklist tutup buku periode beserta hasil pemeriksaan otomatis.
func (s *Service) CloseChecklist(ctx context.Context, periodID uuid.UUID) (Checklist, error) {
	a, err := actor.RequireAny(ctx, PermFiscalPeriodRead, domain.PermFiscalPeriodPost)
	if err != nil {
		return Checklist{}, err
	}
	p, err := s.Store.GetPeriodByID(ctx, periodID, false)
	if err != nil {
		return Checklist{}, err
	}
	return s.checklistView(ctx, a, p)
}

func (s *Service) checklistView(ctx context.Context, a actor.Actor, p domain.Period) (Checklist, error) {
	items, err := s.checklistItems(ctx, p)
	if err != nil {
		return Checklist{}, err
	}
	reopens, err := s.Store.ListReopenRequests(ctx, p.ID)
	if err != nil {
		return Checklist{}, err
	}
	c := Checklist{Period: p, Items: items, Reopens: reopens, AllowedActions: p.AllowedActions(a.Has)}
	if p.Status == domain.PeriodOpen {
		c.Blockers = domain.CloseBlockers(items)
	}
	return c, nil
}

// UpdateChecklistStep mengubah status, PIC, atau catatan satu langkah (Finance/Head F&A). Periode harus terbuka.
func (s *Service) UpdateChecklistStep(ctx context.Context, periodID uuid.UUID, code domain.CloseStepCode, version int, u domain.ChecklistUpdate) (Checklist, error) {
	a, err := actor.RequireAny(ctx, domain.PermFiscalPeriodWrite, domain.PermFiscalPeriodPost)
	if err != nil {
		return Checklist{}, err
	}
	if _, ok := domain.StepByCode(code); !ok {
		return Checklist{}, apperr.New(apperr.CodeNotFound)
	}
	var out Checklist
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPeriodByID(ctx, periodID, false)
		if err != nil {
			return err
		}
		if err := p.CheckOpen(); err != nil {
			return err
		}
		if err := s.Store.EnsureChecklist(ctx, p.ID, a.UserID); err != nil {
			return err
		}
		it, err := s.Store.GetChecklistStep(ctx, p.ID, code)
		if err != nil {
			return err
		}
		if version != 0 && it.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := map[string]any{"status": it.Status, "picUserId": it.PICUserID, "note": it.Note}
		items := []domain.ChecklistItem{it}
		if err := s.runChecks(ctx, p, items); err != nil {
			return err
		}
		it = items[0]
		if err := it.Apply(u, a.UserID, s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.SaveChecklistStep(ctx, it, it.Version, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "fiscal_period", EntityID: p.ID, EntityLabel: p.Label(),
			Before: before, After: map[string]any{"status": it.Status, "picUserId": it.PICUserID, "note": it.Note},
			Note: fmt.Sprintf("Checklist tutup buku langkah %d", it.Step.Order),
		}); err != nil {
			return err
		}
		out, err = s.checklistView(ctx, a, p)
		return err
	})
	return out, err
}

// markStepDone menandai langkah otomatis selesai (dipanggil job penyusutan) bila masih Belum.
func (s *Service) markStepDone(ctx context.Context, p domain.Period, code domain.CloseStepCode, note string) error {
	if err := s.Store.EnsureChecklist(ctx, p.ID, actor.UserID(ctx)); err != nil {
		return err
	}
	it, err := s.Store.GetChecklistStep(ctx, p.ID, code)
	if err != nil {
		return err
	}
	if it.Status != domain.StepPending {
		return nil
	}
	now := s.Clock.Now()
	by := actor.UserID(ctx)
	it.Status, it.CompletedAt, it.CompletedBy, it.Note = domain.StepDone, &now, &by, note
	return s.Store.SaveChecklistStep(ctx, it, it.Version, by)
}

// ClosePeriod menutup periode (★ re-autentikasi, Technical Guideline 5.8.4, 7.2): advisory lock per periode,
// baris periode dikunci FOR UPDATE (menunggu posting yang sedang berjalan), periode sebelumnya harus sudah
// ditutup, langkah 1–8 checklist selesai, dan Neraca Saldo periode balance. Langkah 9 ditandai selesai.
func (s *Service) ClosePeriod(ctx context.Context, periodID uuid.UUID, version int) (Checklist, error) {
	a, err := actor.RequireReauth(ctx, s.Clock.Now(), domain.PermFiscalPeriodPost)
	if err != nil {
		return Checklist{}, err
	}
	var out Checklist
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPeriodByID(ctx, periodID, false)
		if err != nil {
			return err
		}
		if err := s.Store.AdvisoryLockPeriod(ctx, postgres.LockPeriodClose, p.Key()); err != nil {
			return err
		}
		if p, err = s.Store.GetPeriodByID(ctx, periodID, true); err != nil {
			return err
		}
		if p.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if p.Status != domain.PeriodOpen {
			return apperr.InvalidStateTransition("fiscal_period", string(p.Status), string(domain.PeriodClosed))
		}
		if open, err := s.Store.OpenPeriodBefore(ctx, p.Key()); err != nil {
			return err
		} else if open {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tutup periode sebelumnya lebih dulu. Tutup buku dilakukan berurutan.")
		}
		items, err := s.checklistItems(ctx, p)
		if err != nil {
			return err
		}
		if blockers := domain.CloseBlockers(items); len(blockers) > 0 {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Periode belum bisa ditutup. "+blockers[0])
		}
		rows, err := s.Store.TrialBalance(ctx, postgres.TrialBalanceFilter{DateFrom: p.Start(), DateTo: p.End()})
		if err != nil {
			return err
		}
		if t := domain.TrialBalanceTotalsOf(rows); !t.Balanced() {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Neraca saldo periode tidak balance (mutasi debit %s, kredit %s).", t.PeriodDebit.String(), t.PeriodCredit.String()))
		}
		if err := s.markStepDone(ctx, p, domain.StepFinancialReview, "Periode ditutup"); err != nil {
			return err
		}
		if err := p.Close(s.Clock.Now(), a.UserID); err != nil {
			return err
		}
		if err := s.Store.SavePeriodStatus(ctx, p, version, a.UserID); err != nil {
			return err
		}
		if p, err = s.Store.GetPeriodByID(ctx, periodID, false); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: "fiscal_period", EntityID: p.ID, EntityLabel: p.Label(),
			Before: map[string]any{"status": domain.PeriodOpen}, After: map[string]any{"status": p.Status}, Note: "Tutup periode",
		}); err != nil {
			return err
		}
		out, err = s.checklistView(ctx, a, p)
		return err
	})
	return out, err
}

// RequestReopen mengajukan buka kembali periode tertutup (★ re-autentikasi, ACC-14): periode menjadi Menunggu
// Persetujuan (tetap terkunci) dan approval `fiscal_period_reopen` Head F&A → Direksi diajukan.
func (s *Service) RequestReopen(ctx context.Context, periodID uuid.UUID, version int, reason string) (Checklist, error) {
	a, err := actor.RequireReauth(ctx, s.Clock.Now(), domain.PermFiscalPeriodReopen)
	if err != nil {
		return Checklist{}, err
	}
	reason, err = domain.ValidateReopenReason(reason)
	if err != nil {
		return Checklist{}, err
	}
	s.ext.mu.RLock()
	approvals := s.ext.approvals
	s.ext.mu.RUnlock()
	if approvals == nil {
		return Checklist{}, apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul persetujuan tidak aktif.")
	}
	var out Checklist
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPeriodByID(ctx, periodID, true)
		if err != nil {
			return err
		}
		if p.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if err := p.RequestReopen(); err != nil {
			return err
		}
		if err := s.Store.SavePeriodStatus(ctx, p, version, a.UserID); err != nil {
			return err
		}
		reqID, err := s.Store.InsertReopenRequest(ctx, p.ID, reason, a.UserID)
		if err != nil {
			return err
		}
		if err := approvals.RequestApproval(ctx, ApprovalSubject{
			Type: domain.ApprovalSubjectFiscalPeriodReopen, ID: reqID, Label: "Buka Kembali Periode " + p.Label(),
			Link: "/finance/period-close?period=" + p.ID.String(), Summary: reason,
		}); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionSubmit, EntityType: "fiscal_period", EntityID: p.ID, EntityLabel: p.Label(),
			Before: map[string]any{"status": domain.PeriodClosed}, After: map[string]any{"status": p.Status}, Note: "Ajukan buka kembali: " + reason,
		}); err != nil {
			return err
		}
		if p, err = s.Store.GetPeriodByID(ctx, periodID, false); err != nil {
			return err
		}
		out, err = s.checklistView(ctx, a, p)
		return err
	})
	return out, err
}

// DecideReopen dijalankan handler approval `fiscal_period_reopen` di transaksi keputusan: disetujui → periode
// Terbuka (jumlah buka kembali +1, langkah 9 kembali Belum); ditolak → Ditutup.
func (s *Service) DecideReopen(ctx context.Context, requestID uuid.UUID, approved bool, decidedBy uuid.UUID, note string) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		req, err := s.Store.GetReopenRequest(ctx, requestID)
		if err != nil {
			return err
		}
		p, err := s.Store.GetPeriodByID(ctx, req.PeriodID, false)
		if err != nil {
			return err
		}
		if err := s.Store.AdvisoryLockPeriod(ctx, postgres.LockPeriodClose, p.Key()); err != nil {
			return err
		}
		if p, err = s.Store.GetPeriodByID(ctx, req.PeriodID, true); err != nil {
			return err
		}
		version := p.Version
		status, action, msg := domain.ReopenRejected, audit.ActionReject, "Buka kembali ditolak"
		if approved {
			status, action, msg = domain.ReopenApproved, audit.ActionApprove, "Periode dibuka kembali"
			err = p.ApproveReopen()
		} else {
			err = p.RejectReopen()
		}
		if err != nil {
			return err
		}
		if err := s.Store.SavePeriodStatus(ctx, p, version, decidedBy); err != nil {
			return err
		}
		if err := s.Store.DecideReopenRequest(ctx, requestID, status, decidedBy, s.Clock.Now(), note); err != nil {
			return err
		}
		if approved {
			it, err := s.Store.GetChecklistStep(ctx, p.ID, domain.StepFinancialReview)
			if err == nil && it.Status != domain.StepPending {
				it.Status, it.CompletedAt, it.CompletedBy = domain.StepPending, nil, nil
				if err := s.Store.SaveChecklistStep(ctx, it, it.Version, decidedBy); err != nil {
					return err
				}
			} else if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: "fiscal_period", EntityID: p.ID, EntityLabel: p.Label(),
			Before: map[string]any{"status": domain.PeriodPendingApproval}, After: map[string]any{"status": p.Status}, Note: msg,
		})
	})
}
