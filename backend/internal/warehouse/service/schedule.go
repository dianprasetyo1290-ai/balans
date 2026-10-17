package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/jobqueue"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

func scheduleSnapshot(sc domain.CountSchedule) map[string]any {
	return map[string]any{
		"name": sc.Name, "countType": sc.CountType, "frequency": sc.Frequency, "dayOfWeek": sc.DayOfWeek,
		"dayOfMonth": sc.DayOfMonth, "monthOfYear": sc.MonthOfYear, "warehouseId": sc.WarehouseID,
		"locationIds": sc.LocationIDs, "assigneeUserId": sc.AssigneeUserID, "nextRunDate": sc.NextRunDate.String(),
		"isActive": sc.IsActive,
	}
}

// ListSchedules mengembalikan semua jadwal stock count.
func (s *Service) ListSchedules(ctx context.Context) ([]domain.CountSchedule, error) {
	if _, err := actor.Require(ctx, PermCountRead); err != nil {
		return nil, err
	}
	if _, err := s.deps(); err != nil {
		return nil, err
	}
	return s.Docs.ListSchedules(ctx)
}

// CreateSchedule membuat jadwal stock count.
func (s *Service) CreateSchedule(ctx context.Context, in domain.ScheduleInput) (domain.CountSchedule, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return domain.CountSchedule{}, err
	}
	if _, err := s.deps(); err != nil {
		return domain.CountSchedule{}, err
	}
	sc, err := domain.NewSchedule(in, clockToday(s))
	if err != nil {
		return sc, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkScheduleScope(ctx, sc); err != nil {
			return err
		}
		if sc, err = s.Docs.InsertSchedule(ctx, sc, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityCountSchedule, EntityID: sc.ID, EntityLabel: sc.Name, After: scheduleSnapshot(sc),
		})
	})
	return sc, err
}

// UpdateSchedule mengubah jadwal stock count.
func (s *Service) UpdateSchedule(ctx context.Context, id uuid.UUID, version int, in domain.ScheduleInput) (domain.CountSchedule, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return domain.CountSchedule{}, err
	}
	if _, err := s.deps(); err != nil {
		return domain.CountSchedule{}, err
	}
	var sc domain.CountSchedule
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if sc, err = s.Docs.GetSchedule(ctx, id); err != nil {
			return err
		}
		if sc.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := scheduleSnapshot(sc)
		if err := sc.Update(in, clockToday(s)); err != nil {
			return err
		}
		if err := s.checkScheduleScope(ctx, sc); err != nil {
			return err
		}
		if sc, err = s.Docs.UpdateSchedule(ctx, sc, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityCountSchedule, EntityID: sc.ID, EntityLabel: sc.Name,
			Before: before, After: scheduleSnapshot(sc),
		})
	})
	return sc, err
}

func (s *Service) checkScheduleScope(ctx context.Context, sc domain.CountSchedule) error {
	if sc.WarehouseID != nil {
		if _, err := s.Store.GetWarehouse(ctx, *sc.WarehouseID, false); err != nil {
			if apperr.HasCode(err, apperr.CodeNotFound) {
				return apperr.Validation(map[string]string{"warehouseId": "Gudang tidak ditemukan"})
			}
			return err
		}
	}
	for i, id := range sc.LocationIDs {
		l, err := s.activeLocation(ctx, id, fmt.Sprintf("locationIds[%d]", i))
		if err != nil {
			return err
		}
		if !containsType(sc.CountType.LocationTypes(), l.Type) {
			return apperr.Validation(map[string]string{fmt.Sprintf("locationIds[%d]", i): "Lokasi " + l.Code + " tidak sesuai jenis stock count"})
		}
	}
	return nil
}

func containsType(list []domain.LocationType, t domain.LocationType) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

// scheduleLocations menentukan lokasi yang dihitung jadwal: daftar eksplisit, atau semua lokasi aktif sesuai
// jenis stock count (gudang: lokasi internal gudang; konsinyasi/vending: lokasi eksternal mitra/mesin).
func (s *Service) scheduleLocations(ctx context.Context, sc domain.CountSchedule) ([]domain.Location, error) {
	if len(sc.LocationIDs) > 0 {
		m, err := s.LocationsByIDs(ctx, sc.LocationIDs)
		if err != nil {
			return nil, err
		}
		var out []domain.Location
		for _, id := range sc.LocationIDs {
			if l, ok := m[id]; ok && l.IsActive && !l.Deleted() {
				out = append(out, l)
			}
		}
		return out, nil
	}
	f := LocationFilter{Types: sc.CountType.LocationTypes(), WarehouseID: sc.WarehouseID}
	if sc.CountType.Adjusts() {
		internal := false
		f.External = &internal
	}
	return s.FindLocations(ctx, f)
}

// RunSchedules membuat stock count draft dari jadwal yang jatuh tempo dan mengirim pengingat ke petugas:
// assignee jadwal, Merchandiser mitra untuk lokasi konsinyasi, selain itu Logistik Leader. Idempotent: satu
// stock count per jadwal × lokasi × tanggal. Mengembalikan jumlah stock count yang dibuat.
func (s *Service) RunSchedules(ctx context.Context) (int, error) {
	d, err := s.deps()
	if err != nil {
		return 0, err
	}
	ctx = actor.System(ctx)
	today := clockToday(s)
	created := 0
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		due, err := s.Docs.DueSchedules(ctx, today)
		if err != nil {
			return err
		}
		for _, sc := range due {
			locs, err := s.scheduleLocations(ctx, sc)
			if err != nil {
				return err
			}
			for _, l := range locs {
				assignee, name := sc.AssigneeUserID, sc.AssigneeName
				if assignee == nil && l.Type == domain.LocationConsignment && l.PartyID != nil && d.Partners != nil {
					if p, err := d.Partners.PartnerByID(ctx, *l.PartyID); err == nil && p.MerchandiserUserID != nil {
						assignee = p.MerchandiserUserID
					}
				}
				id := sc.ID
				c, err := s.createCount(ctx, domain.StockCountInput{
					CountType: sc.CountType, LocationID: l.ID, BusinessDate: today, ScheduleID: &id,
					AssignedTo: assignee, AssignedToName: name, Note: "Jadwal: " + sc.Name,
				}, uuid.Nil)
				if errors.Is(err, postgres.ErrScheduleRunExists) {
					continue
				}
				if err != nil {
					return err
				}
				created++
				to := s.roleUsers(ctx, d, RoleLogisticsLeader)
				if assignee != nil {
					to = []uuid.UUID{*assignee}
				}
				s.notify(ctx, d, to, "warehouse.stock_count_due", "Jadwal stock count "+l.Code,
					sc.Name+": hitung stok lokasi "+l.Name+" hari ini.", countLink(c.ID))
			}
			before := scheduleSnapshot(sc)
			sc.Advance(today)
			if _, err := s.Docs.UpdateSchedule(ctx, sc, uuid.Nil); err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionUpdate, EntityType: EntityCountSchedule, EntityID: sc.ID, EntityLabel: sc.Name,
				Before: before, After: scheduleSnapshot(sc), Note: "Dijalankan job jadwal stock count",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

// RunSchedulesArgs adalah argumen job warehouse.stock_count_schedule (Technical Guideline Lampiran B).
type RunSchedulesArgs struct{}

// Kind adalah nama job.
func (RunSchedulesArgs) Kind() string { return "warehouse.stock_count_schedule" }

// InsertOpts: maks. 3 percobaan.
func (RunSchedulesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 3}
}

// RunSchedulesWorker menjalankan jadwal stock count harian.
type RunSchedulesWorker struct {
	river.WorkerDefaults[RunSchedulesArgs]
	service *Service
}

// NewRunSchedulesWorker membuat worker.
func NewRunSchedulesWorker(s *Service) *RunSchedulesWorker { return &RunSchedulesWorker{service: s} }

// Work membuat stock count jadwal hari ini. Idempotent.
func (w *RunSchedulesWorker) Work(ctx context.Context, _ *river.Job[RunSchedulesArgs]) error {
	if _, err := w.service.deps(); err != nil {
		return nil //nolint:nilerr // dokumen gudang belum aktif (modul inventory mati): tidak ada yang dijadwalkan
	}
	n, err := w.service.RunSchedules(ctx)
	if err != nil {
		return fmt.Errorf("run stock count schedules: %w", err)
	}
	if n > 0 && w.service.Log != nil {
		w.service.Log.InfoContext(ctx, "stock counts scheduled", "count", n)
	}
	return nil
}
