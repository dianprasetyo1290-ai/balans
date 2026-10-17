-- name: ListReturnReasons :many
SELECT * FROM return_reasons
WHERE deleted_at IS NULL
  AND (sqlc.arg('include_inactive')::boolean OR is_active)
  AND (sqlc.narg('applies_to')::text IS NULL OR applies_to = sqlc.narg('applies_to') OR applies_to = 'both')
  AND (sqlc.narg('q')::text IS NULL OR code ILIKE '%' || sqlc.narg('q') || '%' OR name ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY code;

-- name: GetReturnReason :one
SELECT * FROM return_reasons WHERE id = $1 AND deleted_at IS NULL;

-- name: GetReturnReasonByCode :one
SELECT * FROM return_reasons WHERE code = $1 AND deleted_at IS NULL;

-- name: GetReturnReasonForUpdate :one
SELECT * FROM return_reasons WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertReturnReason :one
INSERT INTO return_reasons (code, name, applies_to, default_disposition, description, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('applies_to'), sqlc.narg('default_disposition'), sqlc.arg('description'),
    sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateReturnReason :one
UPDATE return_reasons
SET name = sqlc.arg('name'), applies_to = sqlc.arg('applies_to'), default_disposition = sqlc.narg('default_disposition'),
    description = sqlc.arg('description'), is_active = sqlc.arg('is_active'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteReturnReason :execrows
UPDATE return_reasons
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;
