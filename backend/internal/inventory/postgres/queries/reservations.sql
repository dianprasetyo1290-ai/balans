-- name: InsertReservation :one
INSERT INTO stock_reservations (
    source_type, source_id, source_line_id, product_id, location_id, batch_id, reserved_qty, expires_at, created_by,
    updated_by)
VALUES (
    sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.narg('source_line_id'), sqlc.arg('product_id'),
    sqlc.arg('location_id'), sqlc.narg('batch_id'), sqlc.arg('qty'), sqlc.narg('expires_at'), sqlc.arg('actor_id'),
    sqlc.arg('actor_id'))
RETURNING id;

-- name: OpenReservations :many
SELECT r.id, r.source_line_id, r.product_id, r.location_id, r.batch_id,
       (r.reserved_qty - r.consumed_qty - r.released_qty)::numeric AS open_qty, r.expires_at,
       b.batch_number, b.production_date, b.expiry_date
FROM stock_reservations r
LEFT JOIN batches b ON b.id = r.batch_id
WHERE r.source_type = sqlc.arg('source_type') AND r.source_id = sqlc.arg('source_id')
  AND r.consumed_qty + r.released_qty < r.reserved_qty
  AND (sqlc.narg('source_line_id')::uuid IS NULL OR r.source_line_id = sqlc.narg('source_line_id')::uuid)
  AND (sqlc.narg('product_id')::uuid IS NULL OR r.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('location_id')::uuid IS NULL OR r.location_id = sqlc.narg('location_id')::uuid)
ORDER BY r.id;

-- name: LockReservations :many
SELECT id, (reserved_qty - consumed_qty - released_qty)::numeric AS open_qty
FROM stock_reservations
WHERE id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY id
FOR UPDATE;

-- name: AddReservationUsage :exec
UPDATE stock_reservations
SET consumed_qty = consumed_qty + sqlc.arg('consumed_qty'), released_qty = released_qty + sqlc.arg('released_qty'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id');

-- name: ExpiredReservationSources :many
SELECT DISTINCT source_type, source_id
FROM stock_reservations
WHERE expires_at IS NOT NULL AND expires_at <= sqlc.arg('now')::timestamptz
  AND consumed_qty + released_qty < reserved_qty
LIMIT sqlc.arg('row_limit');
