-- name: ListWarehouses :many
SELECT * FROM warehouses
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR name ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'code' THEN code END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-code' THEN code END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-name' THEN name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-createdAt' THEN created_at END DESC,
  code ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountWarehouses :one
SELECT count(*) FROM warehouses
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR name ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: GetWarehouse :one
SELECT * FROM warehouses WHERE id = $1;

-- name: GetWarehouseForUpdate :one
SELECT * FROM warehouses WHERE id = $1 FOR UPDATE;

-- name: ListWarehousesByIDs :many
SELECT * FROM warehouses WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: InsertWarehouse :one
INSERT INTO warehouses (code, name, address, is_active, created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('address'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateWarehouse :one
UPDATE warehouses
SET name = sqlc.arg('name'), address = sqlc.arg('address'), is_active = sqlc.arg('is_active'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: SetWarehouseDeleted :one
UPDATE warehouses
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING *;
