-- name: HasIssuesSince :one
SELECT EXISTS (
    SELECT 1 FROM stock_movements
    WHERE product_id = sqlc.arg('product_id') AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
      AND movement_type = 'issue' AND business_date >= sqlc.arg('from_date')::date
)::boolean;

-- name: InsertRecostRun :one
INSERT INTO recost_runs (product_id, batch_id, from_business_date, trigger_source_type, trigger_source_id,
                         trigger_source_label, created_by)
VALUES (sqlc.arg('product_id'), sqlc.narg('batch_id'), sqlc.arg('from_business_date'), sqlc.arg('trigger_source_type'),
        sqlc.arg('trigger_source_id'), sqlc.arg('trigger_source_label'), sqlc.arg('actor_id'))
RETURNING id;

-- name: LockRecostRun :one
SELECT * FROM recost_runs WHERE id = $1 FOR UPDATE;

-- name: GetRecostRun :one
SELECT * FROM recost_runs WHERE id = $1;

-- name: FinishRecostRun :exec
UPDATE recost_runs SET status = sqlc.arg('status'), adjustment_amount = sqlc.arg('adjustment_amount'),
    adjustment_journal_id = sqlc.narg('adjustment_journal_id'), error = sqlc.arg('error'), completed_at = now()
WHERE id = sqlc.arg('id');

-- name: ListRecostRuns :many
SELECT * FROM recost_runs
WHERE (sqlc.narg('product_id')::uuid IS NULL OR product_id = sqlc.narg('product_id')::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountRecostRuns :one
SELECT count(*) FROM recost_runs
WHERE (sqlc.narg('product_id')::uuid IS NULL OR product_id = sqlc.narg('product_id')::uuid);

-- name: LockAllLayers :many
SELECT id, business_date, received_qty, unit_cost FROM cost_layers
WHERE product_id = sqlc.arg('product_id') AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
ORDER BY business_date, created_at, id
FOR UPDATE;

-- name: BatchIssues :many
SELECT m.id, m.business_date, m.location_id, m.source_type, m.source_id, (-m.qty)::numeric AS qty,
       ((-m.cost_amount) + coalesce((SELECT sum(r.difference) FROM recost_lines r WHERE r.stock_movement_id = m.id), 0))::numeric AS current_cost
FROM stock_movements m
WHERE m.product_id = sqlc.arg('product_id') AND m.batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
  AND m.movement_type = 'issue'
ORDER BY m.business_date, m.created_at, m.posting_id, m.line_no;

-- name: InsertRecostLine :exec
INSERT INTO recost_lines (recost_run_id, stock_movement_id, location_id, source_type, source_id, previous_cost,
                          recomputed_cost, difference)
VALUES (sqlc.arg('recost_run_id'), sqlc.arg('stock_movement_id'), sqlc.arg('location_id'), sqlc.arg('source_type'),
        sqlc.arg('source_id'), sqlc.arg('previous_cost'), sqlc.arg('recomputed_cost'), sqlc.arg('difference'));

-- name: ListRecostLines :many
SELECT * FROM recost_lines WHERE recost_run_id = $1 ORDER BY created_at, id;

-- name: OpenLayers :many
SELECT id, remaining_qty, unit_cost FROM cost_layers
WHERE product_id = sqlc.arg('product_id') AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
  AND remaining_qty > 0
ORDER BY business_date, created_at, id;

-- name: SourceMovements :many
SELECT m.id, m.source_line_id, m.movement_type, m.product_id, m.location_id, m.batch_id, m.stock_status, m.qty,
       m.cost_amount, b.batch_number, b.expiry_date
FROM stock_movements m
JOIN stock_postings p ON p.id = m.posting_id
LEFT JOIN batches b ON b.id = m.batch_id
WHERE m.source_type = sqlc.arg('source_type') AND m.source_id = sqlc.arg('source_id')
  AND (sqlc.narg('action')::text IS NULL OR p.action = sqlc.narg('action')::text)
ORDER BY p.created_at, m.line_no;
