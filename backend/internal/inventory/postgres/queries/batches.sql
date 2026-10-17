-- name: InsertBatch :one
INSERT INTO batches (product_id, batch_number, production_date, expiry_date, created_by, updated_by)
VALUES (sqlc.arg('product_id'), sqlc.arg('batch_number'), sqlc.narg('production_date'), sqlc.narg('expiry_date'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (product_id, batch_number) DO NOTHING
RETURNING *;

-- name: GetBatchByNumber :one
SELECT * FROM batches WHERE product_id = $1 AND batch_number = $2;

-- name: GetBatch :one
SELECT * FROM batches WHERE id = $1;

-- name: ListBatchesByIDs :many
SELECT * FROM batches WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListBatchSummaries :many
SELECT b.id, b.product_id, b.batch_number, b.production_date, b.expiry_date, b.created_at,
       coalesce(sum(s.qty) FILTER (WHERE s.stock_status = 'available'), 0)::numeric AS qty_available,
       coalesce(sum(s.qty) FILTER (WHERE s.stock_status = 'quarantine'), 0)::numeric AS qty_quarantine,
       coalesce(sum(s.qty) FILTER (WHERE s.stock_status = 'in_transit'), 0)::numeric AS qty_in_transit,
       coalesce(sum(s.qty) FILTER (WHERE s.stock_status = 'reserved'), 0)::numeric AS qty_reserved,
       coalesce(sum(s.qty), 0)::numeric AS qty_total
FROM batches b
LEFT JOIN stock_balances s ON s.batch_id = b.id
WHERE (sqlc.narg('product_id')::uuid IS NULL OR b.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR b.batch_number ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('expiry_from')::date IS NULL OR b.expiry_date >= sqlc.narg('expiry_from')::date)
  AND (sqlc.narg('expiry_to')::date IS NULL OR b.expiry_date <= sqlc.narg('expiry_to')::date)
  AND (NOT sqlc.arg('expiry_only')::boolean OR b.expiry_date IS NOT NULL)
  AND (NOT sqlc.arg('no_expiry_only')::boolean OR b.expiry_date IS NULL)
GROUP BY b.id
HAVING NOT sqlc.arg('with_stock')::boolean OR coalesce(sum(s.qty), 0) > 0
ORDER BY b.expiry_date ASC NULLS LAST, b.production_date ASC NULLS LAST, b.batch_number ASC, b.id ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountBatchSummaries :one
SELECT count(*) FROM (
    SELECT b.id
    FROM batches b
    LEFT JOIN stock_balances s ON s.batch_id = b.id
    WHERE (sqlc.narg('product_id')::uuid IS NULL OR b.product_id = sqlc.narg('product_id')::uuid)
      AND (sqlc.narg('search')::text IS NULL OR b.batch_number ILIKE '%' || sqlc.narg('search')::text || '%')
      AND (sqlc.narg('expiry_from')::date IS NULL OR b.expiry_date >= sqlc.narg('expiry_from')::date)
      AND (sqlc.narg('expiry_to')::date IS NULL OR b.expiry_date <= sqlc.narg('expiry_to')::date)
      AND (NOT sqlc.arg('expiry_only')::boolean OR b.expiry_date IS NOT NULL)
      AND (NOT sqlc.arg('no_expiry_only')::boolean OR b.expiry_date IS NULL)
    GROUP BY b.id
    HAVING NOT sqlc.arg('with_stock')::boolean OR coalesce(sum(s.qty), 0) > 0
) t;
