// Package postgres adalah repository master gudang dan lokasi.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres/sqlcgen"
)

// Store menyimpan gudang dan lokasi.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func optBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func mark(at *time.Time, by uuid.NullUUID) softdelete.Mark {
	return softdelete.Mark{DeletedAt: at, DeletedBy: uuidPtr(by)}
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func versionConflict(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeVersionConflict, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func duplicateCode(what string) error {
	return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "Kode " + what + " sudah dipakai.",
		Fields: map[string]string{"code": "Kode sudah dipakai, termasuk oleh " + what + " yang sudah dihapus"}}
}

// --- Gudang -----------------------------------------------------------------------------------

func toWarehouse(r sqlcgen.Warehouse) domain.Warehouse {
	return domain.Warehouse{
		ID: r.ID, Code: r.Code, Name: r.Name, Address: r.Address, IsActive: r.IsActive, Mark: mark(r.DeletedAt, r.DeletedBy),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// WarehouseFilter adalah kriteria daftar gudang.
type WarehouseFilter struct {
	Search   string
	IsActive *bool
	Deleted  bool
	Sort     paging.Sort
}

// ListWarehouses mengembalikan satu halaman gudang.
func (s *Store) ListWarehouses(ctx context.Context, f WarehouseFilter, page paging.Page) (paging.Result[domain.Warehouse], error) {
	search := nullText(f.Search)
	rows, err := s.q(ctx).ListWarehouses(ctx, sqlcgen.ListWarehousesParams{
		Deleted: f.Deleted, Search: search, IsActive: optBool(f.IsActive), Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Warehouse]{}, fmt.Errorf("list warehouses: %w", err)
	}
	total, err := s.q(ctx).CountWarehouses(ctx, sqlcgen.CountWarehousesParams{Deleted: f.Deleted, Search: search, IsActive: optBool(f.IsActive)})
	if err != nil {
		return paging.Result[domain.Warehouse]{}, fmt.Errorf("count warehouses: %w", err)
	}
	items := make([]domain.Warehouse, len(rows))
	for i, r := range rows {
		items[i] = toWarehouse(r)
	}
	return paging.Result[domain.Warehouse]{Items: items, Page: page, Total: total}, nil
}

// GetWarehouse membaca gudang termasuk yang terhapus; forUpdate mengunci baris.
func (s *Store) GetWarehouse(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Warehouse, error) {
	var (
		r   sqlcgen.Warehouse
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetWarehouseForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetWarehouse(ctx, id)
	}
	if err != nil {
		return domain.Warehouse{}, notFound(err, "get warehouse")
	}
	return toWarehouse(r), nil
}

// InsertWarehouse menyimpan gudang baru. Kode yang pernah dipakai ditolak DUPLICATE_CODE.
func (s *Store) InsertWarehouse(ctx context.Context, w domain.Warehouse, actorID uuid.UUID) (domain.Warehouse, error) {
	r, err := s.q(ctx).InsertWarehouse(ctx, sqlcgen.InsertWarehouseParams{
		Code: w.Code, Name: w.Name, Address: w.Address, IsActive: w.IsActive, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_warehouses_code") {
		return domain.Warehouse{}, duplicateCode("gudang")
	}
	if err != nil {
		return domain.Warehouse{}, fmt.Errorf("insert warehouse: %w", err)
	}
	return toWarehouse(r), nil
}

// UpdateWarehouse menyimpan perubahan gudang dengan optimistic locking.
func (s *Store) UpdateWarehouse(ctx context.Context, w domain.Warehouse, version int, actorID uuid.UUID) (domain.Warehouse, error) {
	r, err := s.q(ctx).UpdateWarehouse(ctx, sqlcgen.UpdateWarehouseParams{
		ID: w.ID, Name: w.Name, Address: w.Address, IsActive: w.IsActive, ActorID: actorID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Warehouse{}, versionConflict(err, "update warehouse")
	}
	return toWarehouse(r), nil
}

// SetWarehouseDeleted menyimpan penanda hapus gudang.
func (s *Store) SetWarehouseDeleted(ctx context.Context, w domain.Warehouse, actorID uuid.UUID) (domain.Warehouse, error) {
	r, err := s.q(ctx).SetWarehouseDeleted(ctx, sqlcgen.SetWarehouseDeletedParams{
		ID: w.ID, DeletedAt: w.DeletedAt, DeletedBy: nullUUID(w.DeletedBy), ActorID: actorID,
	})
	if err != nil {
		return domain.Warehouse{}, notFound(err, "set warehouse deleted")
	}
	return toWarehouse(r), nil
}

// DeleteWarehouseLocations menandai lokasi gudang yang masih aktif terhapus pada waktu yang sama dengan gudangnya.
func (s *Store) DeleteWarehouseLocations(ctx context.Context, warehouseID uuid.UUID, at time.Time, by uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).DeleteWarehouseLocations(ctx, sqlcgen.DeleteWarehouseLocationsParams{
		WarehouseID: uuid.NullUUID{UUID: warehouseID, Valid: true}, DeletedAt: &at, DeletedBy: uuid.NullUUID{UUID: by, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("delete warehouse locations: %w", err)
	}
	return ids, nil
}

// RestoreWarehouseLocations memulihkan lokasi yang terhapus bersama gudang pada waktu deletedAt.
func (s *Store) RestoreWarehouseLocations(ctx context.Context, warehouseID uuid.UUID, deletedAt time.Time, actorID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).RestoreWarehouseLocations(ctx, sqlcgen.RestoreWarehouseLocationsParams{
		WarehouseID: uuid.NullUUID{UUID: warehouseID, Valid: true}, DeletedAt: &deletedAt, ActorID: actorID,
	})
	if err != nil {
		return nil, fmt.Errorf("restore warehouse locations: %w", err)
	}
	return ids, nil
}

// --- Lokasi -----------------------------------------------------------------------------------

func toLocation(r sqlcgen.Location) domain.Location {
	return domain.Location{
		ID: r.ID, Code: r.Code, Name: r.Name, WarehouseID: uuidPtr(r.WarehouseID), Type: domain.LocationType(r.LocationType),
		PartyType: domain.PartyType(r.ExternalPartyType.String), PartyID: uuidPtr(r.ExternalPartyID), IsSellable: r.IsSellable,
		IsStandard: r.IsStandard, IsActive: r.IsActive, Mark: mark(r.DeletedAt, r.DeletedBy), CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// withWarehouseNames mengisi nama gudang untuk sekumpulan lokasi dalam satu query.
func (s *Store) withWarehouseNames(ctx context.Context, rows []sqlcgen.Location) ([]domain.Location, error) {
	out := make([]domain.Location, len(rows))
	var ids []uuid.UUID
	for i, r := range rows {
		out[i] = toLocation(r)
		if r.WarehouseID.Valid {
			ids = append(ids, r.WarehouseID.UUID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	whs, err := s.q(ctx).ListWarehousesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list warehouses by id: %w", err)
	}
	names := make(map[uuid.UUID]string, len(whs))
	for _, w := range whs {
		names[w.ID] = w.Name
	}
	for i := range out {
		if out[i].WarehouseID != nil {
			out[i].WarehouseName = names[*out[i].WarehouseID]
		}
	}
	return out, nil
}

func (s *Store) oneLocation(ctx context.Context, r sqlcgen.Location) (domain.Location, error) {
	list, err := s.withWarehouseNames(ctx, []sqlcgen.Location{r})
	if err != nil {
		return domain.Location{}, err
	}
	return list[0], nil
}

// LocationFilter adalah kriteria daftar lokasi.
type LocationFilter struct {
	Search      string
	WarehouseID *uuid.UUID
	Types       []domain.LocationType
	External    *bool
	PartyType   domain.PartyType
	PartyID     *uuid.UUID
	IsActive    *bool
	Deleted     bool
	Sort        paging.Sort
}

// ListLocations mengembalikan satu halaman lokasi.
func (s *Store) ListLocations(ctx context.Context, f LocationFilter, page paging.Page) (paging.Result[domain.Location], error) {
	types := make([]string, len(f.Types))
	for i, t := range f.Types {
		types[i] = string(t)
	}
	search := nullText(f.Search)
	rows, err := s.q(ctx).ListLocations(ctx, sqlcgen.ListLocationsParams{
		Deleted: f.Deleted, Search: search, WarehouseID: nullUUID(f.WarehouseID), LocationTypes: types,
		External: optBool(f.External), PartyType: nullText(string(f.PartyType)), PartyID: nullUUID(f.PartyID),
		IsActive: optBool(f.IsActive), Sort: f.Sort.Key(), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Location]{}, fmt.Errorf("list locations: %w", err)
	}
	total, err := s.q(ctx).CountLocations(ctx, sqlcgen.CountLocationsParams{
		Deleted: f.Deleted, Search: search, WarehouseID: nullUUID(f.WarehouseID), LocationTypes: types,
		External: optBool(f.External), PartyType: nullText(string(f.PartyType)), PartyID: nullUUID(f.PartyID),
		IsActive: optBool(f.IsActive),
	})
	if err != nil {
		return paging.Result[domain.Location]{}, fmt.Errorf("count locations: %w", err)
	}
	items, err := s.withWarehouseNames(ctx, rows)
	if err != nil {
		return paging.Result[domain.Location]{}, err
	}
	return paging.Result[domain.Location]{Items: items, Page: page, Total: total}, nil
}

// GetLocation membaca lokasi termasuk yang terhapus; forUpdate mengunci baris.
func (s *Store) GetLocation(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Location, error) {
	var (
		r   sqlcgen.Location
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetLocationForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetLocation(ctx, id)
	}
	if err != nil {
		return domain.Location{}, notFound(err, "get location")
	}
	return s.oneLocation(ctx, r)
}

// LocationsByIDs membaca lokasi termasuk yang terhapus. ID yang tidak ada diabaikan.
func (s *Store) LocationsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Location, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListLocationsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list locations by id: %w", err)
	}
	return s.withWarehouseNames(ctx, rows)
}

// InsertLocation menyimpan lokasi baru. Kode yang pernah dipakai ditolak DUPLICATE_CODE; lokasi eksternal kedua
// untuk jenis dan pihak yang sama ditolak VALIDATION_FAILED.
func (s *Store) InsertLocation(ctx context.Context, l domain.Location, actorID uuid.UUID) (domain.Location, error) {
	r, err := s.q(ctx).InsertLocation(ctx, sqlcgen.InsertLocationParams{
		Code: l.Code, Name: l.Name, WarehouseID: nullUUID(l.WarehouseID), LocationType: string(l.Type),
		ExternalPartyType: nullText(string(l.PartyType)), ExternalPartyID: nullUUID(l.PartyID), IsSellable: l.IsSellable,
		IsStandard: l.IsStandard, IsActive: l.IsActive, ActorID: actorID,
	})
	switch {
	case isUniqueViolation(err, "uq_locations_code"):
		return domain.Location{}, duplicateCode("lokasi")
	case isUniqueViolation(err, "uq_locations_external_party"):
		return domain.Location{}, apperr.Validation(map[string]string{"externalPartyId": "Pihak ini sudah punya lokasi dengan jenis yang sama"})
	case err != nil:
		return domain.Location{}, fmt.Errorf("insert location: %w", err)
	}
	return s.oneLocation(ctx, r)
}

// UpdateLocation menyimpan nama dan status aktif lokasi dengan optimistic locking.
func (s *Store) UpdateLocation(ctx context.Context, l domain.Location, version int, actorID uuid.UUID) (domain.Location, error) {
	r, err := s.q(ctx).UpdateLocation(ctx, sqlcgen.UpdateLocationParams{
		ID: l.ID, Name: l.Name, IsActive: l.IsActive, ActorID: actorID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Location{}, versionConflict(err, "update location")
	}
	return s.oneLocation(ctx, r)
}

// SetLocationDeleted menyimpan penanda hapus lokasi.
func (s *Store) SetLocationDeleted(ctx context.Context, l domain.Location, actorID uuid.UUID) (domain.Location, error) {
	r, err := s.q(ctx).SetLocationDeleted(ctx, sqlcgen.SetLocationDeletedParams{
		ID: l.ID, DeletedAt: l.DeletedAt, DeletedBy: nullUUID(l.DeletedBy), ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_locations_external_party") {
		return domain.Location{}, apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Pihak ini sudah punya lokasi lain dengan jenis yang sama. Hapus lokasi itu dulu sebelum memulihkan.")
	}
	if err != nil {
		return domain.Location{}, notFound(err, "set location deleted")
	}
	return s.oneLocation(ctx, r)
}
