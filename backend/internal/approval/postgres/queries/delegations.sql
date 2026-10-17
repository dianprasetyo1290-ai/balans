-- name: InsertDelegation :one
INSERT INTO approval_delegations (from_user_id, to_user_id, valid_from, valid_to, reason, created_by, updated_by)
VALUES (sqlc.arg('from_user_id'), sqlc.arg('to_user_id'), sqlc.arg('valid_from'), sqlc.arg('valid_to'),
        sqlc.arg('reason'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: GetDelegationForUpdate :one
SELECT * FROM approval_delegations WHERE id = $1 FOR UPDATE;

-- name: ListDelegations :many
SELECT * FROM approval_delegations
WHERE (sqlc.narg('user_id')::uuid IS NULL OR from_user_id = sqlc.narg('user_id') OR to_user_id = sqlc.narg('user_id'))
  AND (sqlc.arg('include_past')::boolean OR (cancelled_at IS NULL AND valid_to >= sqlc.arg('today')))
ORDER BY valid_from DESC, id DESC
LIMIT 500;

-- name: ListActiveDelegationsTo :many
SELECT * FROM approval_delegations
WHERE to_user_id = sqlc.arg('to_user_id') AND cancelled_at IS NULL
  AND valid_from <= sqlc.arg('today') AND valid_to >= sqlc.arg('today');

-- name: ListActiveDelegationsFrom :many
SELECT * FROM approval_delegations
WHERE from_user_id = ANY(sqlc.arg('from_user_ids')::uuid[]) AND cancelled_at IS NULL
  AND valid_from <= sqlc.arg('today') AND valid_to >= sqlc.arg('today');

-- name: CancelDelegation :execrows
UPDATE approval_delegations
SET cancelled_at = sqlc.arg('now')::timestamptz, cancelled_by = sqlc.arg('actor_id')::uuid,
    updated_at = sqlc.arg('now')::timestamptz, updated_by = sqlc.arg('actor_id')::uuid, version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
