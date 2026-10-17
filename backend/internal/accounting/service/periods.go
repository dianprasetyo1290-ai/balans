package service

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"balans/internal/accounting/domain"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
)

// ListPeriods mengembalikan periode akuntansi; year 0 = semua tahun.
func (s *Service) ListPeriods(ctx context.Context, year int) ([]domain.Period, error) {
	if _, err := actor.RequireAny(ctx, PermFiscalPeriodRead, PermJournalRead); err != nil {
		return nil, err
	}
	return s.Store.ListPeriods(ctx, year)
}

// ensurePeriod membuat periode bulan k bila belum ada. Periode sebelum periode yang sudah ditutup tidak dibuat
// otomatis (PERIOD_LOCKED), agar backdate ke bulan tanpa periode tidak membuka celah di tahun yang sudah ditutup.
func (s *Service) ensurePeriod(ctx context.Context, k domain.PeriodKey) (created bool, err error) {
	if _, err := s.Store.GetPeriod(ctx, k, false); err == nil {
		return false, nil
	} else if !apperr.HasCode(err, apperr.CodeNotFound) {
		return false, err
	}
	closedAfter, err := s.Store.ClosedPeriodAfter(ctx, k)
	if err != nil {
		return false, err
	}
	if closedAfter {
		return false, domain.PeriodLocked(k)
	}
	if err := s.Store.EnsurePeriod(ctx, k, actor.UserID(ctx)); err != nil {
		return false, err
	}
	return true, nil
}

// OpenPeriod mengembalikan periode terbuka untuk tanggal bisnis di transaksi aktif, membuatnya bila belum ada,
// dan menguncinya FOR SHARE sampai transaksi selesai (Technical Guideline 5.8.4). Periode tertutup → PERIOD_LOCKED.
func (s *Service) OpenPeriod(ctx context.Context, date clock.BusinessDate) (domain.Period, error) {
	k := domain.PeriodOf(date)
	var p domain.Period
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := s.ensurePeriod(ctx, k); err != nil {
			return err
		}
		var err error
		if p, err = s.Store.GetPeriod(ctx, k, true); err != nil {
			return err
		}
		return p.CheckOpen()
	})
	return p, err
}

// CheckPeriodOpen dipakai modul lain sebelum membuat/mengubah transaksi bertanggal bisnis: menolak PERIOD_LOCKED
// bila periode tanggal tersebut sudah ditutup. Periode yang belum ada dianggap terbuka kecuali periode setelahnya
// sudah ditutup.
func (s *Service) CheckPeriodOpen(ctx context.Context, date clock.BusinessDate) error {
	k := domain.PeriodOf(date)
	p, err := s.Store.GetPeriod(ctx, k, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		closedAfter, err := s.Store.ClosedPeriodAfter(ctx, k)
		if err != nil {
			return err
		}
		if closedAfter {
			return domain.PeriodLocked(k)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return p.CheckOpen()
}

// EnsureUpcomingPeriods membuat periode bulan berjalan dan bulan berikutnya (WIB). Dipanggil job harian
// `accounting.ensure_fiscal_periods` dan tombol Buat Periode. Mengembalikan jumlah periode yang dibuat.
func (s *Service) EnsureUpcomingPeriods(ctx context.Context) (int, error) {
	if _, ok := actor.From(ctx); !ok {
		ctx = actor.System(ctx)
	}
	current := domain.PeriodOf(clock.Today(s.Clock))
	created := 0
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, k := range []domain.PeriodKey{current, current.Next()} {
			ok, err := s.ensurePeriod(ctx, k)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			created++
			p, err := s.Store.GetPeriod(ctx, k, false)
			if err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionCreate, EntityType: "fiscal_period", EntityID: p.ID, EntityLabel: p.Label(),
				After: map[string]any{"year": p.Year, "month": int(p.Month), "status": p.Status},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

// CreateUpcomingPeriods adalah aksi Buat Periode dari halaman Periode Akuntansi.
func (s *Service) CreateUpcomingPeriods(ctx context.Context) (int, error) {
	if _, err := actor.Require(ctx, PermFiscalPeriodWrite); err != nil {
		return 0, err
	}
	return s.EnsureUpcomingPeriods(ctx)
}

// EnsurePeriodsArgs adalah argumen job accounting.ensure_fiscal_periods (Technical Guideline Lampiran B).
type EnsurePeriodsArgs struct{}

// Kind adalah nama job.
func (EnsurePeriodsArgs) Kind() string { return "accounting.ensure_fiscal_periods" }

// InsertOpts: maks. 3 percobaan.
func (EnsurePeriodsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 3}
}

// EnsurePeriodsWorker menjalankan job pembuatan periode.
type EnsurePeriodsWorker struct {
	river.WorkerDefaults[EnsurePeriodsArgs]
	service *Service
}

// NewEnsurePeriodsWorker membuat worker.
func NewEnsurePeriodsWorker(s *Service) *EnsurePeriodsWorker { return &EnsurePeriodsWorker{service: s} }

// Work membuat periode bulan berjalan dan berikutnya. Idempotent.
func (w *EnsurePeriodsWorker) Work(ctx context.Context, _ *river.Job[EnsurePeriodsArgs]) error {
	n, err := w.service.EnsureUpcomingPeriods(ctx)
	if err != nil {
		return fmt.Errorf("ensure fiscal periods: %w", err)
	}
	if n > 0 && w.service.Log != nil {
		w.service.Log.InfoContext(ctx, "fiscal periods created", "count", n)
	}
	return nil
}
