-- name: LockOpenLayers :many
SELECT id, remaining_qty, unit_cost FROM cost_layers
WHERE product_id = sqlc.arg('product_id') AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
  AND remaining_qty > 0
ORDER BY business_date, created_at, id
FOR UPDATE;

-- name: LatestLayerCost :one
SELECT unit_cost FROM cost_layers
WHERE product_id = sqlc.arg('product_id') AND batch_id IS NOT DISTINCT FROM sqlc.narg('batch_id')::uuid
ORDER BY business_date DESC, created_at DESC, id DESC
LIMIT 1;

-- name: InsertLayer :one
INSERT INTO cost_layers (
    product_id, batch_id, source_type, source_id, source_label, movement_id, received_qty, remaining_qty, unit_cost,
    business_date)
VALUES (
    sqlc.arg('product_id'), sqlc.narg('batch_id'), sqlc.arg('source_type'), sqlc.arg('source_id'),
    sqlc.arg('source_label'), sqlc.arg('movement_id'), sqlc.arg('qty'), sqlc.arg('qty'), sqlc.arg('unit_cost'),
    sqlc.arg('business_date'))
RETURNING id;

-- name: SetLayerRemaining :exec
UPDATE cost_layers SET remaining_qty = sqlc.arg('remaining_qty'), updated_at = now() WHERE id = sqlc.arg('id');

-- name: InsertConsumption :exec
INSERT INTO cost_layer_consumptions (cost_layer_id, stock_movement_id, qty, unit_cost, created_by)
VALUES (sqlc.arg('cost_layer_id'), sqlc.arg('stock_movement_id'), sqlc.arg('qty'), sqlc.arg('unit_cost'), sqlc.arg('actor_id'));

-- name: ListLayers :many
SELECT l.id, l.product_id, l.batch_id, l.source_type, l.source_id, l.source_label, l.received_qty, l.remaining_qty,
       l.unit_cost, l.business_date, l.created_at, b.batch_number, b.expiry_date
FROM cost_layers l
LEFT JOIN batches b ON b.id = l.batch_id
WHERE (sqlc.narg('product_id')::uuid IS NULL OR l.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('batch_id')::uuid IS NULL OR l.batch_id = sqlc.narg('batch_id')::uuid)
  AND (NOT sqlc.arg('open_only')::boolean OR l.remaining_qty > 0)
ORDER BY l.product_id, b.expiry_date ASC NULLS LAST, b.batch_number, l.business_date, l.created_at, l.id
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountLayers :one
SELECT count(*) FROM cost_layers l
WHERE (sqlc.narg('product_id')::uuid IS NULL OR l.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('batch_id')::uuid IS NULL OR l.batch_id = sqlc.narg('batch_id')::uuid)
  AND (NOT sqlc.arg('open_only')::boolean OR l.remaining_qty > 0);

-- name: GetLayer :one
SELECT * FROM cost_layers WHERE id = $1;

-- name: ListLayerConsumptions :many
SELECT c.id, c.qty, c.unit_cost, c.created_at, m.id AS movement_id, m.business_date, m.source_type, m.source_id,
       p.source_label, m.location_id
FROM cost_layer_consumptions c
JOIN stock_movements m ON m.id = c.stock_movement_id
JOIN stock_postings p ON p.id = m.posting_id
WHERE c.cost_layer_id = $1
ORDER BY m.business_date, c.created_at, c.id;
