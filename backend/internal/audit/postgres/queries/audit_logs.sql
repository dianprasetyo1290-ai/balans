-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_id, actor_name, action, entity_type, entity_id, entity_label, changes, note, request_id, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListAuditLogs :many
SELECT id, actor_id, actor_name, action, entity_type, entity_id, entity_label, changes, note, request_id, ip, created_at
FROM audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id'))
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountAuditLogs :one
SELECT count(*)
FROM audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id'))
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR created_at < sqlc.narg('created_to'));
