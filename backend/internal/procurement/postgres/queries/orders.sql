-- name: ListPurchaseOrders :many
SELECT * FROM purchase_orders
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (cardinality(sqlc.arg('order_types')::text[]) = 0 OR order_type = ANY(sqlc.arg('order_types')::text[]))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('division_id')::uuid IS NULL OR division_id = sqlc.narg('division_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR supplier_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR supplier_code ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'number' THEN number END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-number' THEN number END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'businessDate' THEN business_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-businessDate' THEN business_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'expectedDate' THEN expected_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-expectedDate' THEN expected_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'totalAmount' THEN total_amount END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-totalAmount' THEN total_amount END DESC,
  created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPurchaseOrders :one
SELECT count(*) FROM purchase_orders
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (cardinality(sqlc.arg('order_types')::text[]) = 0 OR order_type = ANY(sqlc.arg('order_types')::text[]))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('division_id')::uuid IS NULL OR division_id = sqlc.narg('division_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR supplier_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR supplier_code ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetPurchaseOrder :one
SELECT * FROM purchase_orders WHERE id = $1;

-- name: GetPurchaseOrderForUpdate :one
SELECT * FROM purchase_orders WHERE id = $1 FOR UPDATE;

-- name: ListPurchaseOrderLines :many
SELECT * FROM purchase_order_lines
WHERE purchase_order_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY purchase_order_id, line_no;

-- name: ListPurchaseOrderMaterials :many
SELECT * FROM purchase_order_materials
WHERE purchase_order_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY purchase_order_id, line_no;

-- name: InsertPurchaseOrder :one
INSERT INTO purchase_orders (
    order_type, supplier_id, supplier_code, supplier_name, division_id, business_date, expected_date,
    payment_term_days, note, toll_location_id, material_source_location_id, subtotal_amount, total_amount, status,
    created_by, updated_by)
VALUES (
    sqlc.arg('order_type'), sqlc.arg('supplier_id'), sqlc.arg('supplier_code'), sqlc.arg('supplier_name'),
    sqlc.arg('division_id'), sqlc.arg('business_date'), sqlc.arg('expected_date'), sqlc.arg('payment_term_days'),
    sqlc.arg('note'), sqlc.narg('toll_location_id'), sqlc.narg('material_source_location_id'),
    sqlc.arg('subtotal_amount'), sqlc.arg('total_amount'), 'draft', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdatePurchaseOrder :one
UPDATE purchase_orders
SET number = sqlc.narg('number'), supplier_id = sqlc.arg('supplier_id'), supplier_code = sqlc.arg('supplier_code'),
    supplier_name = sqlc.arg('supplier_name'), division_id = sqlc.arg('division_id'),
    business_date = sqlc.arg('business_date'), expected_date = sqlc.arg('expected_date'),
    payment_term_days = sqlc.arg('payment_term_days'), note = sqlc.arg('note'),
    toll_location_id = sqlc.narg('toll_location_id'), material_source_location_id = sqlc.narg('material_source_location_id'),
    subtotal_amount = sqlc.arg('subtotal_amount'), total_amount = sqlc.arg('total_amount'), status = sqlc.arg('status'),
    close_reason = sqlc.arg('close_reason'), cancel_reason = sqlc.arg('cancel_reason'),
    submitted_at = sqlc.narg('submitted_at'), approved_at = sqlc.narg('approved_at'),
    eta_notified_at = sqlc.narg('eta_notified_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeletePurchaseOrderLines :exec
DELETE FROM purchase_order_lines WHERE purchase_order_id = $1;

-- name: DeletePurchaseOrderMaterials :exec
DELETE FROM purchase_order_materials WHERE purchase_order_id = $1;

-- name: InsertPurchaseOrderLine :exec
INSERT INTO purchase_order_lines (
    purchase_order_id, line_no, product_id, description, ordered_qty, unit_price_amount, line_amount, received_qty,
    billed_qty, purchase_request_line_id, note)
VALUES (
    sqlc.arg('purchase_order_id'), sqlc.arg('line_no'), sqlc.narg('product_id'), sqlc.arg('description'),
    sqlc.arg('ordered_qty'), sqlc.arg('unit_price_amount'), sqlc.arg('line_amount'), sqlc.arg('received_qty'),
    sqlc.arg('billed_qty'), sqlc.narg('purchase_request_line_id'), sqlc.arg('note'));

-- name: SetPurchaseOrderLineQty :exec
UPDATE purchase_order_lines
SET received_qty = sqlc.arg('received_qty'), billed_qty = sqlc.arg('billed_qty')
WHERE id = sqlc.arg('id');

-- name: InsertPurchaseOrderMaterial :exec
INSERT INTO purchase_order_materials (purchase_order_id, line_no, material_product_id, planned_qty, note)
VALUES (sqlc.arg('purchase_order_id'), sqlc.arg('line_no'), sqlc.arg('material_product_id'), sqlc.arg('planned_qty'),
        sqlc.arg('note'));

-- name: OrdersDueForETA :many
-- PO disetujui yang dijadwalkan tiba pada tanggal tertentu dan belum diberi notifikasi ETA.
SELECT id FROM purchase_orders
WHERE deleted_at IS NULL AND status IN ('approved', 'partially_received')
  AND expected_date = sqlc.arg('expected_date') AND eta_notified_at IS NULL
ORDER BY number;

-- name: MarkPurchaseOrderETANotified :execrows
UPDATE purchase_orders SET eta_notified_at = sqlc.arg('at')
WHERE id = sqlc.arg('id') AND eta_notified_at IS NULL;
