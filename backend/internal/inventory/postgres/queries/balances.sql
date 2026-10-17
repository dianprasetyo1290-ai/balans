-- name: EnsureBalance :exec
INSERT INTO stock_balances (product_id, location_id, batch_id, stock_status, qty)
VALUES (sqlc.arg('product_id'), sqlc.arg('location_id'), sqlc.narg('batch_id'), sqlc.arg('stock_status'), 0)
ON CONFLICT ON CONSTRAINT uq_stock_balances_key DO NOTHING;

-- name: LockBalance :one
SELECT id, qty FROM stock_balances
WHERE product_id = sqlc.arg('product_id') AND location_id = sqlc.arg('location_id')
  AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid AND stock_status = sqlc.arg('stock_status')
FOR UPDATE;

-- name: SetBalanceQty :exec
UPDATE stock_balances SET qty = sqlc.arg('qty'), updated_at = now() WHERE id = sqlc.arg('id');

-- name: BalanceCandidates :many
SELECT s.batch_id, b.batch_number, b.production_date, b.expiry_date, s.qty
FROM stock_balances s
LEFT JOIN batches b ON b.id = s.batch_id
WHERE s.product_id = sqlc.arg('product_id') AND s.location_id = sqlc.arg('location_id')
  AND s.stock_status = sqlc.arg('stock_status') AND s.qty > 0;

-- name: ListBalances :many
SELECT s.product_id, s.location_id, s.batch_id, s.stock_status, s.qty, s.updated_at,
       b.batch_number, b.production_date, b.expiry_date,
       coalesce((SELECT sum(l.remaining_qty * l.unit_cost) / nullif(sum(l.remaining_qty), 0)
                 FROM cost_layers l
                 WHERE l.product_id = s.product_id AND l.batch_id IS NOT DISTINCT FROM s.batch_id AND l.remaining_qty > 0),
                0)::numeric AS avg_unit_cost
FROM stock_balances s
LEFT JOIN batches b ON b.id = s.batch_id
WHERE s.qty > 0
  AND (sqlc.narg('product_id')::uuid IS NULL OR s.product_id = sqlc.narg('product_id')::uuid)
  AND (cardinality(sqlc.arg('location_ids')::uuid[]) = 0 OR s.location_id = ANY(sqlc.arg('location_ids')::uuid[]))
  AND (sqlc.narg('batch_id')::uuid IS NULL OR s.batch_id = sqlc.narg('batch_id')::uuid)
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR s.stock_status = ANY(sqlc.arg('statuses')::text[]))
ORDER BY s.product_id, s.location_id, b.expiry_date ASC NULLS LAST, b.batch_number, s.stock_status
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: SummarizeBalances :one
SELECT count(*) AS total, coalesce(sum(s.qty), 0)::numeric AS total_qty
FROM stock_balances s
WHERE s.qty > 0
  AND (sqlc.narg('product_id')::uuid IS NULL OR s.product_id = sqlc.narg('product_id')::uuid)
  AND (cardinality(sqlc.arg('location_ids')::uuid[]) = 0 OR s.location_id = ANY(sqlc.arg('location_ids')::uuid[]))
  AND (sqlc.narg('batch_id')::uuid IS NULL OR s.batch_id = sqlc.narg('batch_id')::uuid)
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR s.stock_status = ANY(sqlc.arg('statuses')::text[]));

-- name: StatusQtyByProductLocation :many
SELECT product_id, location_id, sum(qty)::numeric AS qty
FROM stock_balances
WHERE product_id = ANY(sqlc.arg('product_ids')::uuid[]) AND stock_status = sqlc.arg('stock_status') AND qty > 0
  AND (sqlc.narg('location_id')::uuid IS NULL OR location_id = sqlc.narg('location_id')::uuid)
GROUP BY product_id, location_id;
