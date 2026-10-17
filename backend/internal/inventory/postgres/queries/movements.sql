-- name: InsertPosting :one
INSERT INTO stock_postings (source_type, source_id, action, source_label, business_date, created_by)
VALUES (sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('action'), sqlc.arg('source_label'),
        sqlc.arg('business_date'), sqlc.arg('actor_id'))
ON CONFLICT ON CONSTRAINT uq_stock_postings_source DO NOTHING
RETURNING id;

-- name: InsertMovement :one
INSERT INTO stock_movements (
    posting_id, line_no, source_type, source_id, source_line_id, business_date, movement_type, product_id,
    location_id, batch_id, stock_status, qty, unit_cost, cost_amount, reservation_id, batch_override_reason, note,
    created_by)
VALUES (
    sqlc.arg('posting_id'), sqlc.arg('line_no'), sqlc.arg('source_type'), sqlc.arg('source_id'),
    sqlc.narg('source_line_id'), sqlc.arg('business_date'), sqlc.arg('movement_type'), sqlc.arg('product_id'),
    sqlc.arg('location_id'), sqlc.narg('batch_id'), sqlc.arg('stock_status'), sqlc.arg('qty'), sqlc.arg('unit_cost'),
    sqlc.arg('cost_amount'), sqlc.narg('reservation_id'), sqlc.narg('batch_override_reason'), sqlc.arg('note'),
    sqlc.arg('actor_id'))
RETURNING id;

-- name: StockCardOpening :one
SELECT coalesce(sum(qty), 0)::numeric AS qty, coalesce(sum(cost_amount), 0)::numeric AS amount
FROM stock_movements
WHERE product_id = sqlc.arg('product_id')
  AND (sqlc.narg('location_id')::uuid IS NULL OR location_id = sqlc.narg('location_id')::uuid)
  AND (sqlc.narg('batch_id')::uuid IS NULL OR batch_id = sqlc.narg('batch_id')::uuid)
  AND (sqlc.narg('stock_status')::text IS NULL OR stock_status = sqlc.narg('stock_status')::text)
  AND business_date < sqlc.arg('date_from')::date;

-- name: StockCardSummary :one
SELECT count(*) AS total,
       coalesce(sum(qty) FILTER (WHERE qty > 0), 0)::numeric AS qty_in,
       coalesce(-sum(qty) FILTER (WHERE qty < 0), 0)::numeric AS qty_out,
       coalesce(sum(qty), 0)::numeric AS qty_net,
       coalesce(sum(cost_amount), 0)::numeric AS amount_net
FROM stock_movements
WHERE product_id = sqlc.arg('product_id')
  AND (sqlc.narg('location_id')::uuid IS NULL OR location_id = sqlc.narg('location_id')::uuid)
  AND (sqlc.narg('batch_id')::uuid IS NULL OR batch_id = sqlc.narg('batch_id')::uuid)
  AND (sqlc.narg('stock_status')::text IS NULL OR stock_status = sqlc.narg('stock_status')::text)
  AND business_date BETWEEN sqlc.arg('date_from')::date AND sqlc.arg('date_to')::date;

-- name: StockCardLines :many
SELECT * FROM (
    SELECT m.id, m.business_date, m.created_at, m.posting_id, m.line_no, m.source_type, m.source_id, p.source_label,
           m.movement_type, m.location_id, m.batch_id, b.batch_number, m.stock_status, m.qty, m.unit_cost,
           m.cost_amount, m.batch_override_reason, m.note,
           (sum(m.qty) OVER w)::numeric AS running_qty,
           (sum(m.cost_amount) OVER w)::numeric AS running_amount
    FROM stock_movements m
    JOIN stock_postings p ON p.id = m.posting_id
    LEFT JOIN batches b ON b.id = m.batch_id
    WHERE m.product_id = sqlc.arg('product_id')
      AND (sqlc.narg('location_id')::uuid IS NULL OR m.location_id = sqlc.narg('location_id')::uuid)
      AND (sqlc.narg('batch_id')::uuid IS NULL OR m.batch_id = sqlc.narg('batch_id')::uuid)
      AND (sqlc.narg('stock_status')::text IS NULL OR m.stock_status = sqlc.narg('stock_status')::text)
      AND m.business_date BETWEEN sqlc.arg('date_from')::date AND sqlc.arg('date_to')::date
    WINDOW w AS (ORDER BY m.business_date, m.created_at, m.posting_id, m.line_no ROWS UNBOUNDED PRECEDING)
) t
ORDER BY t.business_date, t.created_at, t.posting_id, t.line_no
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: StockValuesAsOf :many
-- Nilai persediaan (Σ cost_amount bertanda) per produk & lokasi s.d. tanggal (rekonsiliasi cut-over).
SELECT product_id, location_id, coalesce(sum(qty), 0)::numeric AS qty, coalesce(sum(cost_amount), 0)::numeric AS value_amount
FROM stock_movements
WHERE business_date <= sqlc.arg('as_of')::date
GROUP BY product_id, location_id
HAVING sum(qty) <> 0 OR sum(cost_amount) <> 0;
