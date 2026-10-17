// Package postgres adalah repository data karyawan. NIK dienkripsi dan di-hash di sini (Technical Guideline 6.7).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/hris/domain"
	"balans/internal/hris/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// Cipher mengenkripsi NIK dan menghitung hash HMAC untuk pencarian persis (platform/pii).
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	Hash(value string) []byte
}

// Store menyimpan karyawan.
type Store struct {
	pool   *pgxpool.Pool
	cipher Cipher
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool, cipher Cipher) *Store { return &Store{pool: pool, cipher: cipher} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Filter adalah filter daftar karyawan.
type Filter struct {
	Search string
	// NIK dicari persis lewat hash.
	NIK    string
	Status domain.Status
	Sort   string
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func toPgDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func fromPgDate(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	b := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &b
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func (s *Store) toEmployee(r sqlcgen.Employee) (domain.Employee, error) {
	nik, err := s.cipher.Decrypt(r.NikEnc)
	if err != nil {
		return domain.Employee{}, fmt.Errorf("employee %s nik: %w", r.ID, err)
	}
	e := domain.Employee{
		ID: r.ID, Number: r.EmployeeNumber, FullName: r.FullName, PositionTitle: r.PositionTitle,
		Status: domain.Status(r.EmploymentStatus), JoinDate: r.JoinDate, ResignDate: fromPgDate(r.ResignDate),
		ResignReason: r.ResignReason, WorkEmail: r.WorkEmail, Phone: r.Phone, NIK: nik, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
	if r.DivisionID.Valid {
		id := r.DivisionID.UUID
		e.DivisionID = &id
	}
	return e, nil
}

func (s *Store) toEmployees(rows []sqlcgen.Employee) ([]domain.Employee, error) {
	out := make([]domain.Employee, len(rows))
	for i, r := range rows {
		e, err := s.toEmployee(r)
		if err != nil {
			return nil, err
		}
		out[i] = e
	}
	return out, nil
}

// nik mengenkripsi NIK dan menghitung hash-nya; NIK kosong menghasilkan kolom kosong.
func (s *Store) nik(nik string) (string, []byte, error) {
	if nik == "" {
		return "", nil, nil
	}
	enc, err := s.cipher.Encrypt(nik)
	if err != nil {
		return "", nil, fmt.Errorf("encrypt nik: %w", err)
	}
	return enc, s.cipher.Hash(nik), nil
}

func employeeError(err error, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uq_employees_employee_number":
			return apperr.New(apperr.CodeDuplicateCode)
		case "uq_employees_nik_hash":
			return apperr.Validation(map[string]string{"nik": "NIK sudah terdaftar untuk karyawan lain"})
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// Insert menyimpan karyawan baru. Nomor karyawan yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) Insert(ctx context.Context, e domain.Employee, actorID uuid.UUID) (domain.Employee, error) {
	enc, hash, err := s.nik(e.NIK)
	if err != nil {
		return domain.Employee{}, err
	}
	r, err := s.q(ctx).InsertEmployee(ctx, sqlcgen.InsertEmployeeParams{
		EmployeeNumber: e.Number, FullName: e.FullName, DivisionID: nullUUID(e.DivisionID), PositionTitle: e.PositionTitle,
		JoinDate: e.JoinDate, WorkEmail: e.WorkEmail, Phone: e.Phone, NikEnc: enc, NikHash: hash, ActorID: actorID,
	})
	if err != nil {
		return domain.Employee{}, employeeError(err, "insert employee")
	}
	return s.toEmployee(r)
}

// Get membaca karyawan; forUpdate mengunci baris.
func (s *Store) Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Employee, error) {
	var (
		r   sqlcgen.Employee
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetEmployeeForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetEmployee(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Employee{}, apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("get employee: %w", err))
	}
	if err != nil {
		return domain.Employee{}, fmt.Errorf("get employee: %w", err)
	}
	return s.toEmployee(r)
}

// ByIDs membaca beberapa karyawan, termasuk yang terhapus.
func (s *Store) ByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Employee, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListEmployeesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list employees by id: %w", err)
	}
	return s.toEmployees(rows)
}

// List mengembalikan satu halaman karyawan.
func (s *Store) List(ctx context.Context, f Filter, page paging.Page) (paging.Result[domain.Employee], error) {
	q := s.q(ctx)
	var hash []byte
	if f.NIK != "" {
		hash = s.cipher.Hash(f.NIK)
	}
	search, status := text(f.Search), text(string(f.Status))
	rows, err := q.ListEmployees(ctx, sqlcgen.ListEmployeesParams{
		Search: search, NikHash: hash, Status: status, Sort: f.Sort, RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Employee]{}, fmt.Errorf("list employees: %w", err)
	}
	total, err := q.CountEmployees(ctx, sqlcgen.CountEmployeesParams{Search: search, NikHash: hash, Status: status})
	if err != nil {
		return paging.Result[domain.Employee]{}, fmt.Errorf("count employees: %w", err)
	}
	items, err := s.toEmployees(rows)
	if err != nil {
		return paging.Result[domain.Employee]{}, err
	}
	return paging.Result[domain.Employee]{Items: items, Page: page, Total: total}, nil
}

// Update menyimpan perubahan karyawan (termasuk resign) dengan optimistic locking.
func (s *Store) Update(ctx context.Context, e domain.Employee, version int, actorID uuid.UUID) error {
	enc, hash, err := s.nik(e.NIK)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).UpdateEmployee(ctx, sqlcgen.UpdateEmployeeParams{
		ID: e.ID, Version: int32(version), FullName: e.FullName, DivisionID: nullUUID(e.DivisionID), //nolint:gosec // versi kecil
		PositionTitle: e.PositionTitle, JoinDate: e.JoinDate, EmploymentStatus: string(e.Status),
		ResignDate: toPgDate(e.ResignDate), ResignReason: e.ResignReason, WorkEmail: e.WorkEmail, Phone: e.Phone,
		NikEnc: enc, NikHash: hash, ActorID: actorID,
	})
	if err != nil {
		return employeeError(err, "update employee")
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
