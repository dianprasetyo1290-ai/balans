-- name: ListLocations :many
SELECT * FROM locations
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR name ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('warehouse_id')::uuid IS NULL OR warehouse_id = sqlc.narg('warehouse_id')::uuid)
  AND (cardinality(sqlc.arg('location_types')::text[]) = 0 OR location_type = ANY(sqlc.arg('location_types')::text[]))
  AND (sqlc.narg('external')::boolean IS NULL OR (warehouse_id IS NULL) = sqlc.narg('external')::boolean)
  AND (sqlc.narg('party_type')::text IS NULL OR external_party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR external_party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'code' THEN code END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-code' THEN code END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-name' THEN name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-createdAt' THEN created_at END DESC,
  is_standard DESC, created_at ASC, id ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountLocations :one
SELECT count(*) FROM locations
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR name ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('warehouse_id')::uuid IS NULL OR warehouse_id = sqlc.narg('warehouse_id')::uuid)
  AND (cardinality(sqlc.arg('location_types')::text[]) = 0 OR location_type = ANY(sqlc.arg('location_types')::text[]))
  AND (sqlc.narg('external')::boolean IS NULL OR (warehouse_id IS NULL) = sqlc.narg('external')::boolean)
  AND (sqlc.narg('party_type')::text IS NULL OR external_party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR external_party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: GetLocation :one
SELECT * FROM locations WHERE id = $1;

-- name: GetLocationForUpdate :one
SELECT * FROM locations WHERE id = $1 FOR UPDATE;

-- name: ListLocationsByIDs :many
SELECT * FROM locations WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: InsertLocation :one
INSERT INTO locations (
    code, name, warehouse_id, location_type, external_party_type, external_party_id, is_sellable, is_standard,
    is_active, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.narg('warehouse_id'), sqlc.arg('location_type'),
    sqlc.narg('external_party_type'), sqlc.narg('external_party_id'), sqlc.arg('is_sellable'), sqlc.arg('is_standard'),
    sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateLocation :one
UPDATE locations
SET name = sqlc.arg('name'), is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: SetLocationDeleted :one
UPDATE locations
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteWarehouseLocations :many
-- Lokasi gudang yang masih aktif ikut terhapus dengan waktu yang sama dengan gudangnya.
UPDATE locations
SET deleted_at = sqlc.arg('deleted_at'), deleted_by = sqlc.arg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('deleted_by'), version = version + 1
WHERE warehouse_id = sqlc.arg('warehouse_id') AND deleted_at IS NULL
RETURNING id;

-- name: RestoreWarehouseLocations :many
-- Memulihkan lokasi yang terhapus bersama gudang (waktu hapus sama); lokasi yang dihapus terpisah tetap terhapus.
UPDATE locations
SET deleted_at = NULL, deleted_by = NULL, updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE warehouse_id = sqlc.arg('warehouse_id') AND deleted_at = sqlc.arg('deleted_at')
RETURNING id;
