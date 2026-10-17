-- name: ListPurchaseRequests :many
SELECT * FROM purchase_requests
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('division_id')::uuid IS NULL OR division_id = sqlc.narg('division_id')::uuid)
  AND (sqlc.narg('requested_by')::uuid IS NULL OR requested_by = sqlc.narg('requested_by')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR note ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'number' THEN number END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-number' THEN number END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'businessDate' THEN business_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-businessDate' THEN business_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'estimatedTotal' THEN estimated_total_amount END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-estimatedTotal' THEN estimated_total_amount END DESC,
  created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPurchaseRequests :one
SELECT count(*) FROM purchase_requests
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('division_id')::uuid IS NULL OR division_id = sqlc.narg('division_id')::uuid)
  AND (sqlc.narg('requested_by')::uuid IS NULL OR requested_by = sqlc.narg('requested_by')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR note ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetPurchaseRequest :one
SELECT * FROM purchase_requests WHERE id = $1;

-- name: GetPurchaseRequestForUpdate :one
SELECT * FROM purchase_requests WHERE id = $1 FOR UPDATE;

-- name: ListPurchaseRequestLines :many
SELECT * FROM purchase_request_lines
WHERE purchase_request_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY purchase_request_id, line_no;

-- name: InsertPurchaseRequest :one
INSERT INTO purchase_requests (
    division_id, requested_by, business_date, needed_date, note, estimated_total_amount, status, created_by, updated_by)
VALUES (
    sqlc.arg('division_id'), sqlc.arg('requested_by'), sqlc.arg('business_date'), sqlc.narg('needed_date'),
    sqlc.arg('note'), sqlc.arg('estimated_total_amount'), 'draft', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdatePurchaseRequest :one
UPDATE purchase_requests
SET number = sqlc.narg('number'), division_id = sqlc.arg('division_id'), business_date = sqlc.arg('business_date'),
    needed_date = sqlc.narg('needed_date'), note = sqlc.arg('note'),
    estimated_total_amount = sqlc.arg('estimated_total_amount'), status = sqlc.arg('status'),
    close_reason = sqlc.arg('close_reason'), submitted_at = sqlc.narg('submitted_at'),
    decided_at = sqlc.narg('decided_at'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeletePurchaseRequestLines :exec
DELETE FROM purchase_request_lines WHERE purchase_request_id = $1;

-- name: InsertPurchaseRequestLine :exec
INSERT INTO purchase_request_lines (
    purchase_request_id, line_no, product_id, description, qty, estimated_price_amount, fulfilled_qty, note)
VALUES (
    sqlc.arg('purchase_request_id'), sqlc.arg('line_no'), sqlc.narg('product_id'), sqlc.arg('description'),
    sqlc.arg('qty'), sqlc.arg('estimated_price_amount'), sqlc.arg('fulfilled_qty'), sqlc.arg('note'));

-- name: SetPurchaseRequestLineFulfilled :exec
UPDATE purchase_request_lines SET fulfilled_qty = sqlc.arg('fulfilled_qty') WHERE id = sqlc.arg('id');

-- name: PurchaseRequestIDsByLines :many
SELECT DISTINCT purchase_request_id FROM purchase_request_lines WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: PurchaseRequestLineOrders :many
-- Baris PO yang menautkan baris PR beserta status PO, untuk menghitung qty yang sudah dijadikan PO.
SELECT l.purchase_request_line_id, l.ordered_qty, l.received_qty, o.status, o.id AS purchase_order_id, o.number
FROM purchase_order_lines l
JOIN purchase_orders o ON o.id = l.purchase_order_id
JOIN purchase_request_lines r ON r.id = l.purchase_request_line_id
WHERE r.purchase_request_id = $1;
