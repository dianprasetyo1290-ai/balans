-- name: InsertTransfer :one
INSERT INTO stock_transfers (transfer_type, from_location_id, to_location_id, requires_shipping, business_date, status,
                             source_type, source_id, note, requested_by, requested_by_name, created_by, updated_by)
VALUES (sqlc.arg('transfer_type'), sqlc.arg('from_location_id'), sqlc.arg('to_location_id'),
        sqlc.arg('requires_shipping'), sqlc.arg('business_date'), sqlc.arg('status'), sqlc.arg('source_type'),
        sqlc.narg('source_id'), sqlc.arg('note'), sqlc.arg('requested_by'), sqlc.arg('requested_by_name'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: InsertTransferLine :one
INSERT INTO stock_transfer_lines (transfer_id, line_no, product_id, batch_id, qty, received_qty, shortage_qty,
                                  cost_amount, batch_reason, note)
VALUES (sqlc.arg('transfer_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.narg('batch_id'), sqlc.arg('qty'),
        sqlc.arg('received_qty'), sqlc.arg('shortage_qty'), sqlc.arg('cost_amount'), sqlc.arg('batch_reason'),
        sqlc.arg('note'))
RETURNING id;

-- name: DeleteTransferLines :exec
DELETE FROM stock_transfer_lines WHERE transfer_id = $1;

-- name: UpdateTransferLine :exec
UPDATE stock_transfer_lines
SET received_qty = sqlc.arg('received_qty'), shortage_qty = sqlc.arg('shortage_qty'), cost_amount = sqlc.arg('cost_amount')
WHERE id = sqlc.arg('id');

-- name: GetTransfer :one
SELECT * FROM stock_transfers WHERE id = $1;

-- name: GetTransferForUpdate :one
SELECT * FROM stock_transfers WHERE id = $1 FOR UPDATE;

-- name: ListTransferLines :many
SELECT * FROM stock_transfer_lines WHERE transfer_id = $1 ORDER BY line_no;

-- name: UpdateTransfer :one
UPDATE stock_transfers
SET number = sqlc.narg('number'), from_location_id = sqlc.arg('from_location_id'), to_location_id = sqlc.arg('to_location_id'),
    requires_shipping = sqlc.arg('requires_shipping'), business_date = sqlc.arg('business_date'), status = sqlc.arg('status'),
    note = sqlc.arg('note'), submitted_at = sqlc.narg('submitted_at'),
    released_by = sqlc.narg('released_by'), released_by_name = sqlc.arg('released_by_name'), released_at = sqlc.narg('released_at'),
    prepared_by = sqlc.narg('prepared_by'), prepared_by_name = sqlc.arg('prepared_by_name'), prepared_at = sqlc.narg('prepared_at'),
    shipped_by = sqlc.narg('shipped_by'), shipped_by_name = sqlc.arg('shipped_by_name'), shipped_at = sqlc.narg('shipped_at'),
    received_by = sqlc.narg('received_by'), received_by_name = sqlc.arg('received_by_name'), received_at = sqlc.narg('received_at'),
    completed_at = sqlc.narg('completed_at'), cancelled_at = sqlc.narg('cancelled_at'), cancel_reason = sqlc.arg('cancel_reason'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: ListTransfers :many
SELECT t.*,
       (SELECT count(*) FROM stock_transfer_lines l WHERE l.transfer_id = t.id)::bigint AS line_count,
       (SELECT coalesce(sum(l.qty), 0) FROM stock_transfer_lines l WHERE l.transfer_id = t.id)::numeric AS total_qty
FROM stock_transfers t
WHERE t.transfer_type = sqlc.arg('transfer_type')
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR t.status = ANY (sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('location_id')::uuid IS NULL
       OR t.from_location_id = sqlc.narg('location_id')::uuid OR t.to_location_id = sqlc.narg('location_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR t.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR t.note ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountTransfers :one
SELECT count(*) FROM stock_transfers t
WHERE t.transfer_type = sqlc.arg('transfer_type')
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR t.status = ANY (sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('location_id')::uuid IS NULL
       OR t.from_location_id = sqlc.narg('location_id')::uuid OR t.to_location_id = sqlc.narg('location_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR t.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR t.note ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: ListInTransitLines :many
SELECT t.id AS transfer_id, t.number, t.from_location_id, t.to_location_id, t.business_date, t.shipped_at, t.status,
       l.id AS line_id, l.product_id, l.batch_id, l.qty, l.received_qty, l.cost_amount
FROM stock_transfers t
JOIN stock_transfer_lines l ON l.transfer_id = t.id
WHERE t.status IN ('in_transit', 'partially_received')
  AND l.qty > l.received_qty + l.shortage_qty
  AND (sqlc.narg('location_id')::uuid IS NULL
       OR t.from_location_id = sqlc.narg('location_id')::uuid OR t.to_location_id = sqlc.narg('location_id')::uuid)
ORDER BY t.shipped_at, t.id, l.line_no
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountInTransitLines :one
SELECT count(*) FROM stock_transfers t
JOIN stock_transfer_lines l ON l.transfer_id = t.id
WHERE t.status IN ('in_transit', 'partially_received')
  AND l.qty > l.received_qty + l.shortage_qty
  AND (sqlc.narg('location_id')::uuid IS NULL
       OR t.from_location_id = sqlc.narg('location_id')::uuid OR t.to_location_id = sqlc.narg('location_id')::uuid);
