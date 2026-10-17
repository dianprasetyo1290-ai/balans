-- name: InsertOutbound :one
INSERT INTO outbounds (number, source_type, source_id, source_label, warehouse_id, location_id, business_date,
                       recipient_name, recipient_phone, recipient_address, status, note, created_by, updated_by)
VALUES (sqlc.arg('number'), sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('source_label'),
        sqlc.arg('warehouse_id'), sqlc.arg('location_id'), sqlc.arg('business_date'), sqlc.arg('recipient_name'),
        sqlc.arg('recipient_phone'), sqlc.arg('recipient_address'), sqlc.arg('status'), sqlc.arg('note'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetOutbound :one
SELECT * FROM outbounds WHERE id = $1;

-- name: GetOutboundForUpdate :one
SELECT * FROM outbounds WHERE id = $1 FOR UPDATE;

-- name: GetOutboundBySource :one
SELECT * FROM outbounds WHERE source_type = sqlc.arg('source_type') AND source_id = sqlc.arg('source_id');

-- name: UpdateOutbound :one
UPDATE outbounds
SET status = sqlc.arg('status'), prepared_by = sqlc.narg('prepared_by'), prepared_by_name = sqlc.arg('prepared_by_name'),
    packed_at = sqlc.narg('packed_at'), checked_by = sqlc.narg('checked_by'), checked_by_name = sqlc.arg('checked_by_name'),
    completed_at = sqlc.narg('completed_at'), cancelled_at = sqlc.narg('cancelled_at'),
    cancel_reason = sqlc.arg('cancel_reason'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: InsertOutboundLine :one
INSERT INTO outbound_lines (outbound_id, line_no, source_line_id, product_id, batch_id, allocated_qty, picked_qty,
                            status, replacement_of_line_id)
VALUES (sqlc.arg('outbound_id'), sqlc.arg('line_no'), sqlc.narg('source_line_id'), sqlc.arg('product_id'),
        sqlc.narg('batch_id'), sqlc.arg('allocated_qty'), sqlc.arg('picked_qty'), sqlc.arg('status'),
        sqlc.narg('replacement_of_line_id'))
RETURNING id;

-- name: UpdateOutboundLine :exec
UPDATE outbound_lines
SET batch_id = sqlc.narg('batch_id'), allocated_qty = sqlc.arg('allocated_qty'), picked_qty = sqlc.arg('picked_qty'),
    passed_qty = sqlc.arg('passed_qty'), rejected_qty = sqlc.arg('rejected_qty'), status = sqlc.arg('status'),
    reject_reason = sqlc.arg('reject_reason'), return_label = sqlc.arg('return_label'),
    checked_by = sqlc.narg('checked_by'), checked_by_name = sqlc.arg('checked_by_name'), checked_at = sqlc.narg('checked_at')
WHERE id = sqlc.arg('id');

-- name: ListOutboundLines :many
SELECT * FROM outbound_lines WHERE outbound_id = $1 ORDER BY line_no;

-- name: DeleteOutboundPackages :exec
DELETE FROM outbound_packages WHERE outbound_id = $1;

-- name: InsertOutboundPackage :exec
INSERT INTO outbound_packages (outbound_id, package_number, package_count, weight_grams, contents)
VALUES (sqlc.arg('outbound_id'), sqlc.arg('package_number'), sqlc.arg('package_count'), sqlc.narg('weight_grams'),
        sqlc.arg('contents'));

-- name: ListOutboundPackages :many
SELECT * FROM outbound_packages WHERE outbound_id = $1 ORDER BY package_number;

-- name: ListOutbounds :many
SELECT o.*,
       (SELECT count(*) FROM outbound_lines l WHERE l.outbound_id = o.id)::bigint AS line_count,
       (SELECT count(*) FROM outbound_lines l WHERE l.outbound_id = o.id AND l.status IN ('pending', 'picked'))::bigint AS pending_count,
       (SELECT count(*) FROM outbound_lines l WHERE l.outbound_id = o.id AND l.status = 'held')::bigint AS held_count
FROM outbounds o
WHERE (sqlc.narg('status')::text IS NULL OR o.status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_type')::text IS NULL OR o.source_type = sqlc.narg('source_type')::text)
  AND (sqlc.narg('search')::text IS NULL
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.source_label ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY o.created_at DESC, o.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountOutbounds :one
SELECT count(*) FROM outbounds o
WHERE (sqlc.narg('status')::text IS NULL OR o.status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_type')::text IS NULL OR o.source_type = sqlc.narg('source_type')::text)
  AND (sqlc.narg('search')::text IS NULL
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.source_label ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%');
