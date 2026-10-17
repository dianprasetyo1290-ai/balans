-- name: InsertInbound :one
INSERT INTO inbounds (number, source_type, source_id, source_label, warehouse_id, location_id, business_date,
                      reject_action, status, note, created_by, updated_by)
VALUES (sqlc.arg('number'), sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('source_label'),
        sqlc.arg('warehouse_id'), sqlc.arg('location_id'), sqlc.arg('business_date'), sqlc.arg('reject_action'),
        sqlc.arg('status'), sqlc.arg('note'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: InsertInboundLine :one
INSERT INTO inbound_lines (inbound_id, line_no, source_line_id, product_id, batch_id, qty, destination_location_id)
VALUES (sqlc.arg('inbound_id'), sqlc.arg('line_no'), sqlc.narg('source_line_id'), sqlc.arg('product_id'),
        sqlc.narg('batch_id'), sqlc.arg('qty'), sqlc.arg('destination_location_id'))
RETURNING id;

-- name: GetInbound :one
SELECT * FROM inbounds WHERE id = $1;

-- name: GetInboundForUpdate :one
SELECT * FROM inbounds WHERE id = $1 FOR UPDATE;

-- name: GetInboundBySource :one
SELECT * FROM inbounds WHERE source_type = sqlc.arg('source_type') AND source_id = sqlc.arg('source_id');

-- name: ListInboundLines :many
SELECT * FROM inbound_lines WHERE inbound_id = $1 ORDER BY line_no;

-- name: UpdateInbound :one
UPDATE inbounds
SET status = sqlc.arg('status'), completed_at = sqlc.narg('completed_at'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: UpdateInboundLineQC :exec
UPDATE inbound_lines
SET passed_qty = sqlc.arg('passed_qty'), rejected_qty = sqlc.arg('rejected_qty'),
    destination_location_id = sqlc.arg('destination_location_id'), status = sqlc.arg('status'),
    reject_reason = sqlc.arg('reject_reason'), reject_location_id = sqlc.narg('reject_location_id'),
    checked_by = sqlc.arg('checked_by'), checked_by_name = sqlc.arg('checked_by_name'), checked_at = sqlc.arg('checked_at')
WHERE id = sqlc.arg('id');

-- name: InsertQualityCheck :exec
INSERT INTO quality_checks (document_type, document_id, document_line_id, result, qty, checks, note, photo_file_ids,
                            checked_by, checked_by_name, checked_at)
VALUES (sqlc.arg('document_type'), sqlc.arg('document_id'), sqlc.arg('document_line_id'), sqlc.arg('result'),
        sqlc.arg('qty'), sqlc.arg('checks'), sqlc.arg('note'), sqlc.arg('photo_file_ids')::uuid[],
        sqlc.arg('checked_by'), sqlc.arg('checked_by_name'), sqlc.arg('checked_at'));

-- name: ListQualityChecks :many
SELECT * FROM quality_checks
WHERE document_type = sqlc.arg('document_type') AND document_id = sqlc.arg('document_id')
ORDER BY checked_at, id;

-- name: ListInbounds :many
SELECT i.*,
       (SELECT count(*) FROM inbound_lines l WHERE l.inbound_id = i.id)::bigint AS line_count,
       (SELECT count(*) FROM inbound_lines l WHERE l.inbound_id = i.id AND l.status = 'quarantine')::bigint AS pending_count
FROM inbounds i
WHERE (sqlc.narg('status')::text IS NULL OR i.status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_type')::text IS NULL OR i.source_type = sqlc.narg('source_type')::text)
  AND (sqlc.narg('warehouse_id')::uuid IS NULL OR i.warehouse_id = sqlc.narg('warehouse_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR i.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.source_label ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY i.created_at DESC, i.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountInbounds :one
SELECT count(*) FROM inbounds i
WHERE (sqlc.narg('status')::text IS NULL OR i.status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_type')::text IS NULL OR i.source_type = sqlc.narg('source_type')::text)
  AND (sqlc.narg('warehouse_id')::uuid IS NULL OR i.warehouse_id = sqlc.narg('warehouse_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR i.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.source_label ILIKE '%' || sqlc.narg('search')::text || '%');
