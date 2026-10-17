// Package service berisi use case master gudang dan lokasi stok (MD-09, S-123, S-106 soft delete).
package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Permission modul warehouse untuk master (Desain Role & Approval bagian 3.1).
const (
	PermWarehouseRead  = "warehouse.warehouse.read"
	PermWarehouseWrite = "warehouse.warehouse.write"
	PermLocationRead   = "warehouse.location.read"
	PermLocationWrite  = "warehouse.location.write"
)

// Jenis entitas di audit trail.
const (
	EntityWarehouse = "warehouse"
	EntityLocation  = "location"
)

// Aksi yang dikembalikan di allowedActions.
const (
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionRestore = "restore"
)

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	ListWarehouses(ctx context.Context, f postgres.WarehouseFilter, page paging.Page) (paging.Result[domain.Warehouse], error)
	GetWarehouse(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Warehouse, error)
	InsertWarehouse(ctx context.Context, w domain.Warehouse, actorID uuid.UUID) (domain.Warehouse, error)
	UpdateWarehouse(ctx context.Context, w domain.Warehouse, version int, actorID uuid.UUID) (domain.Warehouse, error)
	SetWarehouseDeleted(ctx context.Context, w domain.Warehouse, actorID uuid.UUID) (domain.Warehouse, error)
	DeleteWarehouseLocations(ctx context.Context, warehouseID uuid.UUID, at time.Time, by uuid.UUID) ([]uuid.UUID, error)
	RestoreWarehouseLocations(ctx context.Context, warehouseID uuid.UUID, deletedAt time.Time, actorID uuid.UUID) ([]uuid.UUID, error)

	ListLocations(ctx context.Context, f postgres.LocationFilter, page paging.Page) (paging.Result[domain.Location], error)
	GetLocation(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Location, error)
	LocationsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Location, error)
	InsertLocation(ctx context.Context, l domain.Location, actorID uuid.UUID) (domain.Location, error)
	UpdateLocation(ctx context.Context, l domain.Location, version int, actorID uuid.UUID) (domain.Location, error)
	SetLocationDeleted(ctx context.Context, l domain.Location, actorID uuid.UUID) (domain.Location, error)
}

// Deps adalah dependensi Service.
type Deps struct {
	Store store
	Tx    txRunner
	Audit audit.Recorder
	Clock clock.Clock
}

// Service adalah use case modul warehouse (bagian master).
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

// RecordActions mengembalikan aksi yang boleh dilakukan actor atas data master (allowedActions).
func RecordActions(ctx context.Context, m softdelete.Mark, writePermission string) []string {
	a, ok := actor.From(ctx)
	switch {
	case !ok:
		return []string{}
	case m.Deleted() && softdelete.CanRestore(a, writePermission):
		return []string{ActionRestore}
	case !m.Deleted() && a.Has(writePermission):
		return []string{ActionUpdate, ActionDelete}
	}
	return []string{}
}

// LocationActions seperti RecordActions, tetapi lokasi standar tidak bisa dihapus sendiri.
func LocationActions(ctx context.Context, l domain.Location) []string {
	out := RecordActions(ctx, l.Mark, PermLocationWrite)
	if l.IsStandard {
		filtered := out[:0]
		for _, a := range out {
			if a == ActionUpdate {
				filtered = append(filtered, a)
			}
		}
		return filtered
	}
	return out
}

// now dipotong ke mikrodetik agar sama persis dengan presisi timestamptz Postgres (dipakai pemulihan berantai).
func (s *Service) now() time.Time { return s.Clock.Now().Truncate(time.Microsecond) }

func deletedSnapshot(m softdelete.Mark) map[string]any {
	return map[string]any{"deletedAt": m.DeletedAt}
}

func warehouseSnapshot(w domain.Warehouse) map[string]any {
	return map[string]any{"code": w.Code, "name": w.Name, "address": w.Address, "isActive": w.IsActive}
}

func locationSnapshot(l domain.Location) map[string]any {
	return map[string]any{
		"code": l.Code, "name": l.Name, "locationType": l.Type, "warehouseId": l.WarehouseID,
		"externalPartyType": l.PartyType, "externalPartyId": l.PartyID, "isSellable": l.IsSellable, "isActive": l.IsActive,
	}
}

// --- Gudang -----------------------------------------------------------------------------------

// WarehouseFilter adalah kriteria daftar gudang.
type WarehouseFilter = postgres.WarehouseFilter

// SortFields adalah field sort yang diizinkan di daftar gudang dan lokasi.
var SortFields = []string{"code", "name", "createdAt"}

// ListWarehouses mengembalikan daftar gudang.
func (s *Service) ListWarehouses(ctx context.Context, f WarehouseFilter, page paging.Page) (paging.Result[domain.Warehouse], error) {
	if _, err := actor.Require(ctx, PermWarehouseRead); err != nil {
		return paging.Result[domain.Warehouse]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListWarehouses(ctx, f, page)
}

// GetWarehouse mengembalikan detail gudang termasuk yang terhapus.
func (s *Service) GetWarehouse(ctx context.Context, id uuid.UUID) (domain.Warehouse, error) {
	if _, err := actor.Require(ctx, PermWarehouseRead); err != nil {
		return domain.Warehouse{}, err
	}
	return s.Store.GetWarehouse(ctx, id, false)
}

// CreateWarehouse membuat gudang beserta lokasi standarnya dalam satu transaksi (F1-114).
func (s *Service) CreateWarehouse(ctx context.Context, in domain.WarehouseInput) (domain.Warehouse, []domain.Location, error) {
	a, err := actor.Require(ctx, PermWarehouseWrite)
	if err != nil {
		return domain.Warehouse{}, nil, err
	}
	w, err := domain.NewWarehouse(in)
	if err != nil {
		return domain.Warehouse{}, nil, err
	}
	var locs []domain.Location
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if w, err = s.Store.InsertWarehouse(ctx, w, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityWarehouse, EntityID: w.ID, EntityLabel: w.Code, After: warehouseSnapshot(w),
		}); err != nil {
			return err
		}
		for _, l := range domain.StandardLocations(w) {
			saved, err := s.Store.InsertLocation(ctx, l, a.UserID)
			if apperr.HasCode(err, apperr.CodeDuplicateCode) {
				return apperr.Validation(map[string]string{"code": "Kode lokasi standar " + l.Code + " sudah dipakai. Pilih kode gudang lain"})
			}
			if err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionCreate, EntityType: EntityLocation, EntityID: saved.ID, EntityLabel: saved.Code,
				After: locationSnapshot(saved), Note: "Lokasi standar gudang " + w.Code,
			}); err != nil {
				return err
			}
			locs = append(locs, saved)
		}
		return nil
	})
	return w, locs, err
}

// UpdateWarehouse mengubah nama, alamat, dan status aktif gudang.
func (s *Service) UpdateWarehouse(ctx context.Context, id uuid.UUID, version int, in domain.WarehouseInput) (domain.Warehouse, error) {
	a, err := actor.Require(ctx, PermWarehouseWrite)
	if err != nil {
		return domain.Warehouse{}, err
	}
	var w domain.Warehouse
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if w, err = s.Store.GetWarehouse(ctx, id, true); err != nil {
			return err
		}
		if w.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := warehouseSnapshot(w)
		if err := w.Update(in); err != nil {
			return err
		}
		if w, err = s.Store.UpdateWarehouse(ctx, w, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityWarehouse, EntityID: w.ID, EntityLabel: w.Code,
			Before: before, After: warehouseSnapshot(w),
		})
	})
	return w, err
}

// DeleteWarehouse menghapus gudang beserta lokasinya yang masih aktif (soft delete, waktu hapus sama).
func (s *Service) DeleteWarehouse(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermWarehouseWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		w, err := s.Store.GetWarehouse(ctx, id, true)
		if err != nil {
			return err
		}
		before, now := w.Mark, s.now()
		if err := w.Delete(now, a.UserID); err != nil {
			return err
		}
		if w, err = s.Store.SetWarehouseDeleted(ctx, w, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityWarehouse, EntityID: w.ID, EntityLabel: w.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(w.Mark),
		}); err != nil {
			return err
		}
		ids, err := s.Store.DeleteWarehouseLocations(ctx, w.ID, *w.DeletedAt, a.UserID)
		if err != nil {
			return err
		}
		return s.recordCascade(ctx, ids, audit.ActionDelete, nil, "Ikut terhapus bersama gudang "+w.Code)
	})
}

// RestoreWarehouse memulihkan gudang dan lokasi yang terhapus bersamanya (hanya Admin Sistem).
func (s *Service) RestoreWarehouse(ctx context.Context, id uuid.UUID) (domain.Warehouse, error) {
	a, err := softdelete.RequireRestore(ctx, PermWarehouseWrite)
	if err != nil {
		return domain.Warehouse{}, err
	}
	var w domain.Warehouse
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if w, err = s.Store.GetWarehouse(ctx, id, true); err != nil {
			return err
		}
		before := w.Mark
		if err := w.Restore(); err != nil {
			return err
		}
		if w, err = s.Store.SetWarehouseDeleted(ctx, w, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionRestore, EntityType: EntityWarehouse, EntityID: w.ID, EntityLabel: w.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(w.Mark),
		}); err != nil {
			return err
		}
		ids, err := s.Store.RestoreWarehouseLocations(ctx, w.ID, *before.DeletedAt, a.UserID)
		if err != nil {
			return err
		}
		return s.recordCascade(ctx, ids, audit.ActionRestore, before.DeletedAt, "Ikut dipulihkan bersama gudang "+w.Code)
	})
	return w, err
}

// recordCascade mencatat audit lokasi yang ikut terhapus/dipulihkan bersama gudang; prevDeletedAt adalah
// waktu hapus sebelum perubahan (nil saat menghapus).
func (s *Service) recordCascade(ctx context.Context, ids []uuid.UUID, action audit.Action, prevDeletedAt *time.Time, note string) error {
	locs, err := s.Store.LocationsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, l := range locs {
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityLocation, EntityID: l.ID, EntityLabel: l.Code,
			Before: map[string]any{"deletedAt": prevDeletedAt}, After: deletedSnapshot(l.Mark), Note: note,
		}); err != nil {
			return err
		}
	}
	return nil
}

// --- Lokasi -----------------------------------------------------------------------------------

// LocationFilter adalah kriteria daftar lokasi.
type LocationFilter = postgres.LocationFilter

// ListLocations mengembalikan daftar lokasi.
func (s *Service) ListLocations(ctx context.Context, f LocationFilter, page paging.Page) (paging.Result[domain.Location], error) {
	if _, err := actor.Require(ctx, PermLocationRead); err != nil {
		return paging.Result[domain.Location]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListLocations(ctx, f, page)
}

// GetLocation mengembalikan detail lokasi termasuk yang terhapus.
func (s *Service) GetLocation(ctx context.Context, id uuid.UUID) (domain.Location, error) {
	if _, err := actor.Require(ctx, PermLocationRead); err != nil {
		return domain.Location{}, err
	}
	return s.Store.GetLocation(ctx, id, false)
}

// CreateLocation membuat lokasi tambahan di gudang atau lokasi eksternal per pihak.
func (s *Service) CreateLocation(ctx context.Context, in domain.LocationInput) (domain.Location, error) {
	a, err := actor.Require(ctx, PermLocationWrite)
	if err != nil {
		return domain.Location{}, err
	}
	l, err := domain.NewLocation(in)
	if err != nil {
		return domain.Location{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if l.WarehouseID != nil {
			w, err := s.Store.GetWarehouse(ctx, *l.WarehouseID, false)
			if apperr.HasCode(err, apperr.CodeNotFound) || (err == nil && w.Deleted()) {
				return apperr.Validation(map[string]string{"warehouseId": "Gudang tidak ditemukan atau sudah dihapus"})
			}
			if err != nil {
				return err
			}
		}
		if l, err = s.Store.InsertLocation(ctx, l, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityLocation, EntityID: l.ID, EntityLabel: l.Code, After: locationSnapshot(l),
		})
	})
	return l, err
}

// UpdateLocation mengubah nama dan status aktif lokasi.
func (s *Service) UpdateLocation(ctx context.Context, id uuid.UUID, version int, name string, isActive bool) (domain.Location, error) {
	a, err := actor.Require(ctx, PermLocationWrite)
	if err != nil {
		return domain.Location{}, err
	}
	var l domain.Location
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if l, err = s.Store.GetLocation(ctx, id, true); err != nil {
			return err
		}
		if l.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := locationSnapshot(l)
		if err := l.Update(name, isActive); err != nil {
			return err
		}
		if l, err = s.Store.UpdateLocation(ctx, l, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityLocation, EntityID: l.ID, EntityLabel: l.Code,
			Before: before, After: locationSnapshot(l),
		})
	})
	return l, err
}

// DeleteLocation menghapus lokasi tambahan (soft delete). Lokasi standar ikut gudangnya.
func (s *Service) DeleteLocation(ctx context.Context, id uuid.UUID) error {
	a, err := actor.Require(ctx, PermLocationWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.Store.GetLocation(ctx, id, true)
		if err != nil {
			return err
		}
		if err := l.CanDelete(); err != nil {
			return err
		}
		before := l.Mark
		if err := l.Delete(s.now(), a.UserID); err != nil {
			return err
		}
		if l, err = s.Store.SetLocationDeleted(ctx, l, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: EntityLocation, EntityID: l.ID, EntityLabel: l.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(l.Mark),
		})
	})
}

// RestoreLocation memulihkan lokasi (hanya Admin Sistem). Lokasi di gudang terhapus harus dipulihkan lewat gudangnya.
func (s *Service) RestoreLocation(ctx context.Context, id uuid.UUID) (domain.Location, error) {
	a, err := softdelete.RequireRestore(ctx, PermLocationWrite)
	if err != nil {
		return domain.Location{}, err
	}
	var l domain.Location
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if l, err = s.Store.GetLocation(ctx, id, true); err != nil {
			return err
		}
		if l.WarehouseID != nil {
			w, err := s.Store.GetWarehouse(ctx, *l.WarehouseID, false)
			if err != nil {
				return err
			}
			if w.Deleted() {
				return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Gudang "+w.Code+" sudah dihapus. Pulihkan gudangnya dulu.")
			}
		}
		before := l.Mark
		if err := l.Restore(); err != nil {
			return err
		}
		if l, err = s.Store.SetLocationDeleted(ctx, l, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionRestore, EntityType: EntityLocation, EntityID: l.ID, EntityLabel: l.Code,
			Before: deletedSnapshot(before), After: deletedSnapshot(l.Mark),
		})
	})
	return l, err
}

// --- Lookup untuk modul lain (tanpa cek permission; modul pemanggil memeriksa permission-nya sendiri) ---

// LocationByID membaca lokasi termasuk yang terhapus.
func (s *Service) LocationByID(ctx context.Context, id uuid.UUID) (domain.Location, error) {
	return s.Store.GetLocation(ctx, id, false)
}

// LocationsByIDs membaca banyak lokasi termasuk yang terhapus, dipetakan per ID.
func (s *Service) LocationsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Location, error) {
	list, err := s.Store.LocationsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.Location, len(list))
	for _, l := range list {
		out[l.ID] = l
	}
	return out, nil
}

// WarehouseByID membaca gudang termasuk yang terhapus.
func (s *Service) WarehouseByID(ctx context.Context, id uuid.UUID) (domain.Warehouse, error) {
	return s.Store.GetWarehouse(ctx, id, false)
}

// FindLocations mengembalikan lokasi aktif yang belum dihapus sesuai filter (maks. 200), urut kode.
func (s *Service) FindLocations(ctx context.Context, f LocationFilter) ([]domain.Location, error) {
	active, size := true, paging.MaxSize
	f.IsActive, f.Deleted, f.Sort = &active, false, paging.Sort{Field: "code"}
	res, err := s.Store.ListLocations(ctx, f, paging.New(nil, &size))
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}
