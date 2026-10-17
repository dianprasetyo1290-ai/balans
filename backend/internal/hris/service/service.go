// Package service berisi use case data dasar karyawan (HR-01) dan status aktif karyawan untuk harga karyawan
// (HR-03).
package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/hris/domain"
	"balans/internal/hris/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

// Permission modul hris.
const (
	PermEmployeeRead  = "hris.employee.read"
	PermEmployeeWrite = "hris.employee.write"
	// PermCustomerWrite (modul party) boleh memilih karyawan saat menautkan pelanggan ke karyawan.
	PermCustomerWrite = "party.customer.write"
)

// EntityEmployee adalah jenis entitas di audit trail.
const EntityEmployee = "employee"

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	Insert(ctx context.Context, e domain.Employee, actorID uuid.UUID) (domain.Employee, error)
	Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Employee, error)
	ByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Employee, error)
	List(ctx context.Context, f postgres.Filter, page paging.Page) (paging.Result[domain.Employee], error)
	Update(ctx context.Context, e domain.Employee, version int, actorID uuid.UUID) error
}

// Deps adalah dependensi Service.
type Deps struct {
	Store store
	Tx    txRunner
	Audit audit.Recorder
	Clock clock.Clock
	Log   *slog.Logger
}

// Service adalah use case modul hris.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

// Today mengembalikan tanggal bisnis hari ini (WIB).
func (s *Service) Today() clock.BusinessDate { return clock.Today(s.Clock) }

// snapshot memuat NIK tersamar; audit juga memask field `nik` (Technical Guideline 6.6).
func snapshot(e domain.Employee) map[string]any {
	var nik any
	if e.NIK != "" {
		nik = pii.Mask(e.NIK)
	}
	m := map[string]any{
		"employeeNumber": e.Number, "fullName": e.FullName, "divisionId": e.DivisionID, "positionTitle": e.PositionTitle,
		"employmentStatus": e.Status, "joinDate": e.JoinDate, "resignReason": e.ResignReason, "workEmail": e.WorkEmail,
		"phone": e.Phone, "nik": nik,
	}
	if e.ResignDate != nil {
		m["resignDate"] = *e.ResignDate
	}
	return m
}

// Query adalah filter daftar karyawan.
type Query struct {
	Search string
	NIK    string
	Status domain.Status
	Sort   paging.Sort
}

// List mengembalikan karyawan. NIK dicari persis lewat hash.
func (s *Service) List(ctx context.Context, q Query, page paging.Page) (paging.Result[domain.Employee], error) {
	if _, err := actor.Require(ctx, PermEmployeeRead); err != nil {
		return paging.Result[domain.Employee]{}, err
	}
	f := postgres.Filter{Search: strings.TrimSpace(q.Search), Status: q.Status, Sort: q.Sort.Key()}
	if q.NIK != "" {
		nik, ok := domain.NormalizeNIK(q.NIK)
		if !ok {
			return paging.Result[domain.Employee]{}, apperr.Validation(map[string]string{"nik": "NIK harus 16 digit"})
		}
		f.NIK = nik
	}
	return s.Store.List(ctx, f, page)
}

// Get mengembalikan detail karyawan.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Employee, error) {
	if _, err := actor.Require(ctx, PermEmployeeRead); err != nil {
		return domain.Employee{}, err
	}
	return s.Store.Get(ctx, id, false)
}

// Create menambah karyawan.
func (s *Service) Create(ctx context.Context, in domain.Input) (domain.Employee, error) {
	a, err := actor.Require(ctx, PermEmployeeWrite)
	if err != nil {
		return domain.Employee{}, err
	}
	e, err := domain.New(in)
	if err != nil {
		return domain.Employee{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if e, err = s.Store.Insert(ctx, e, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityEmployee, EntityID: e.ID, EntityLabel: e.Number + " " + e.FullName,
			After: snapshot(e),
		})
	})
	return e, err
}

// Update mengubah data karyawan; nomor karyawan tetap.
func (s *Service) Update(ctx context.Context, id uuid.UUID, version int, in domain.Input) (domain.Employee, error) {
	a, err := actor.Require(ctx, PermEmployeeWrite)
	if err != nil {
		return domain.Employee{}, err
	}
	var next domain.Employee
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		next = cur
		if err := next.Update(in); err != nil {
			return err
		}
		if err := s.Store.Update(ctx, next, version, a.UserID); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityEmployee, EntityID: next.ID,
			EntityLabel: next.Number + " " + next.FullName, Before: snapshot(cur), After: snapshot(next),
		})
	})
	return next, err
}

// Resign mencatat resign. Mulai tanggal resign, pelanggan yang tertaut tidak lagi mendapat harga karyawan.
func (s *Service) Resign(ctx context.Context, id uuid.UUID, version int, date clock.BusinessDate, reason string) (domain.Employee, error) {
	a, err := actor.Require(ctx, PermEmployeeWrite)
	if err != nil {
		return domain.Employee{}, err
	}
	var next domain.Employee
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		next = cur
		if err := next.Resign(date, reason); err != nil {
			return err
		}
		if err := s.Store.Update(ctx, next, version, a.UserID); err != nil {
			return err
		}
		next.Version++
		next.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityEmployee, EntityID: next.ID,
			EntityLabel: next.Number + " " + next.FullName, Before: snapshot(cur), After: snapshot(next),
			Note: "Resign: " + next.ResignReason,
		})
	})
	return next, err
}

// Options mencari karyawan aktif untuk ditautkan ke pelanggan (tanpa data pribadi).
func (s *Service) Options(ctx context.Context, q string) ([]domain.Employee, error) {
	if _, err := actor.RequireAny(ctx, PermEmployeeRead, PermCustomerWrite); err != nil {
		return nil, err
	}
	res, err := s.Store.List(ctx, postgres.Filter{Search: strings.TrimSpace(q), Status: domain.StatusActive, Sort: "fullName"},
		paging.Page{Number: 1, Size: 20})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// IsActiveEmployee dipakai modul lain (party, pricing) tanpa cek permission. Karyawan yang tidak ada
// dianggap tidak aktif.
func (s *Service) IsActiveEmployee(ctx context.Context, id uuid.UUID, on clock.BusinessDate) (bool, error) {
	e, err := s.Store.Get(ctx, id, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return e.ActiveOn(on), nil
}

// ByIDs dipakai modul lain untuk menampilkan karyawan (termasuk yang terhapus).
func (s *Service) ByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Employee, error) {
	return s.Store.ByIDs(ctx, ids)
}
