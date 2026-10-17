package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Proses tunggal per periode yang memakai advisory lock (Technical Guideline 7.2).
const (
	LockPeriodClose  = "accounting.period_close"
	LockDepreciation = "accounting.depreciate"
)

// AdvisoryLockPeriod mengambil pg_advisory_xact_lock(proses, periode) sampai transaksi selesai.
func (s *Store) AdvisoryLockPeriod(ctx context.Context, process string, k domain.PeriodKey) error {
	err := s.q(ctx).AdvisoryLockPeriod(ctx, sqlcgen.AdvisoryLockPeriodParams{Process: process, PeriodIndex: int32(k.Index())}) //nolint:gosec // indeks bulan kecil
	if err != nil {
		return fmt.Errorf("advisory lock %s: %w", process, err)
	}
	return nil
}

// GetPeriodByID membaca periode; forUpdate mengunci baris (menunggu posting yang memegang FOR SHARE).
func (s *Store) GetPeriodByID(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Period, error) {
	var (
		r   sqlcgen.FiscalPeriod
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPeriodByIDForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPeriodByID(ctx, id)
	}
	if err != nil {
		return domain.Period{}, notFound(err, "get fiscal period")
	}
	return toPeriod(r), nil
}

// SavePeriodStatus menyimpan status, waktu tutup, dan jumlah buka kembali periode.
func (s *Store) SavePeriodStatus(ctx context.Context, p domain.Period, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SetPeriodStatus(ctx, sqlcgen.SetPeriodStatusParams{
		ID: p.ID, Version: int32(version), Status: string(p.Status), ClosedAt: p.ClosedAt, ClosedBy: nullUUID(p.ClosedBy), //nolint:gosec // versi kecil
		ReopenedCount: int32(p.ReopenedCount), ActorID: actorID, //nolint:gosec // kecil
	})
	if err != nil {
		return fmt.Errorf("set period status: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// OpenPeriodBefore memeriksa periode sebelum k yang belum ditutup.
func (s *Store) OpenPeriodBefore(ctx context.Context, k domain.PeriodKey) (bool, error) {
	ok, err := s.q(ctx).OpenPeriodBeforeExists(ctx, int32(k.Index())) //nolint:gosec // indeks bulan kecil
	if err != nil {
		return false, fmt.Errorf("open period before: %w", err)
	}
	return ok, nil
}

// EnsureChecklist membuat baris 9 langkah periode bila belum ada.
func (s *Store) EnsureChecklist(ctx context.Context, periodID uuid.UUID, actorID uuid.UUID) error {
	q := s.q(ctx)
	for _, st := range domain.CloseSteps {
		if err := q.EnsureChecklistStep(ctx, sqlcgen.EnsureChecklistStepParams{
			FiscalPeriodID: periodID, StepCode: string(st.Code), Status: string(domain.StepPending), ActorID: actorID,
		}); err != nil {
			return fmt.Errorf("ensure checklist step: %w", err)
		}
	}
	return nil
}

func toChecklistItem(r sqlcgen.PeriodCloseChecklist) domain.ChecklistItem {
	step, _ := domain.StepByCode(domain.CloseStepCode(r.StepCode))
	return domain.ChecklistItem{
		ID: r.ID, PeriodID: r.FiscalPeriodID, Step: step, Status: domain.CloseStepStatus(r.Status), PICUserID: uuidPtr(r.PicUserID),
		CompletedBy: uuidPtr(r.CompletedBy), CompletedAt: r.CompletedAt, Note: r.Note, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListChecklist mengembalikan langkah checklist urut langkah.
func (s *Store) ListChecklist(ctx context.Context, periodID uuid.UUID) ([]domain.ChecklistItem, error) {
	rows, err := s.q(ctx).ListChecklist(ctx, periodID)
	if err != nil {
		return nil, fmt.Errorf("list checklist: %w", err)
	}
	byCode := map[domain.CloseStepCode]domain.ChecklistItem{}
	for _, r := range rows {
		it := toChecklistItem(r)
		byCode[it.Step.Code] = it
	}
	out := make([]domain.ChecklistItem, 0, len(domain.CloseSteps))
	for _, st := range domain.CloseSteps {
		if it, ok := byCode[st.Code]; ok {
			out = append(out, it)
		}
	}
	return out, nil
}

// GetChecklistStep mengunci satu langkah.
func (s *Store) GetChecklistStep(ctx context.Context, periodID uuid.UUID, code domain.CloseStepCode) (domain.ChecklistItem, error) {
	r, err := s.q(ctx).GetChecklistStepForUpdate(ctx, sqlcgen.GetChecklistStepForUpdateParams{FiscalPeriodID: periodID, StepCode: string(code)})
	if err != nil {
		return domain.ChecklistItem{}, notFound(err, "get checklist step")
	}
	return toChecklistItem(r), nil
}

// SaveChecklistStep menyimpan perubahan langkah dengan optimistic locking.
func (s *Store) SaveChecklistStep(ctx context.Context, it domain.ChecklistItem, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateChecklistStep(ctx, sqlcgen.UpdateChecklistStepParams{
		ID: it.ID, Version: int32(version), Status: string(it.Status), PicUserID: nullUUID(it.PICUserID), //nolint:gosec // versi kecil
		CompletedBy: nullUUID(it.CompletedBy), CompletedAt: it.CompletedAt, Note: it.Note, ActorID: actorID,
	})
	if err != nil {
		return fmt.Errorf("update checklist step: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// InsertReopenRequest mencatat permintaan buka kembali.
func (s *Store) InsertReopenRequest(ctx context.Context, periodID uuid.UUID, reason string, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertReopenRequest(ctx, sqlcgen.InsertReopenRequestParams{FiscalPeriodID: periodID, Reason: reason, ActorID: actorID})
	if pe := pgError(err); pe != nil && pe.Code == "23505" {
		return uuid.Nil, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Permintaan buka kembali periode ini masih menunggu persetujuan.")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert reopen request: %w", err)
	}
	return id, nil
}

func toReopen(r sqlcgen.FiscalPeriodReopen) domain.ReopenRequest {
	return domain.ReopenRequest{
		ID: r.ID, PeriodID: r.FiscalPeriodID, Reason: r.Reason, Status: domain.ReopenStatus(r.Status), RequestedBy: r.RequestedBy,
		RequestedAt: r.RequestedAt, DecidedBy: uuidPtr(r.DecidedBy), DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
	}
}

// GetReopenRequest mengunci permintaan buka kembali.
func (s *Store) GetReopenRequest(ctx context.Context, id uuid.UUID) (domain.ReopenRequest, error) {
	r, err := s.q(ctx).GetReopenRequestForUpdate(ctx, id)
	if err != nil {
		return domain.ReopenRequest{}, notFound(err, "get reopen request")
	}
	return toReopen(r), nil
}

// DecideReopenRequest menyimpan keputusan approval.
func (s *Store) DecideReopenRequest(ctx context.Context, id uuid.UUID, status domain.ReopenStatus, by uuid.UUID, at time.Time, note string) error {
	n, err := s.q(ctx).DecideReopenRequest(ctx, sqlcgen.DecideReopenRequestParams{
		ID: id, Status: string(status), DecidedBy: nullUUID(&by), DecidedAt: &at, DecisionNote: note,
	})
	if err != nil {
		return fmt.Errorf("decide reopen request: %w", err)
	}
	if n == 0 {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Permintaan buka kembali sudah diputuskan.")
	}
	return nil
}

// ListReopenRequests mengembalikan riwayat permintaan buka kembali periode.
func (s *Store) ListReopenRequests(ctx context.Context, periodID uuid.UUID) ([]domain.ReopenRequest, error) {
	rows, err := s.q(ctx).ListReopenRequests(ctx, periodID)
	if err != nil {
		return nil, fmt.Errorf("list reopen requests: %w", err)
	}
	out := make([]domain.ReopenRequest, len(rows))
	for i, r := range rows {
		out[i] = toReopen(r)
	}
	return out, nil
}

// DraftJournalCount menghitung jurnal draft di rentang tanggal.
func (s *Store) DraftJournalCount(ctx context.Context, from, to clock.BusinessDate) (int64, error) {
	n, err := s.q(ctx).DraftJournalCount(ctx, sqlcgen.DraftJournalCountParams{DateFrom: from, DateTo: to})
	if err != nil {
		return 0, fmt.Errorf("draft journal count: %w", err)
	}
	return n, nil
}
