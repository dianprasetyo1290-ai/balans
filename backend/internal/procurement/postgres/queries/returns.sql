-- name: ListPurchaseReturns :many
SELECT r.*, o.number AS order_number
FROM purchase_returns r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('origin')::text IS NULL OR r.origin = sqlc.narg('origin')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR r.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR r.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR r.business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR r.business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.debit_note_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY r.business_date DESC, r.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPurchaseReturns :one
SELECT count(*)
FROM purchase_returns r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('origin')::text IS NULL OR r.origin = sqlc.narg('origin')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR r.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR r.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR r.business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR r.business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.debit_note_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetPurchaseReturn :one
SELECT r.*, o.number AS order_number
FROM purchase_returns r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.id = $1;

-- name: LockPurchaseReturn :one
SELECT id FROM purchase_returns WHERE id = $1 FOR UPDATE;

-- name: ListPurchaseReturnLines :many
SELECT * FROM purchase_return_lines WHERE purchase_return_id = $1 ORDER BY line_no;

-- name: PendingReturnQtyByReceiptLines :many
-- Qty baris GR di retur lain yang belum diposting (mencegah retur ganda atas barang yang sama).
SELECT l.goods_receipt_line_id, sum(l.qty)::numeric AS qty
FROM purchase_return_lines l
JOIN purchase_returns r ON r.id = l.purchase_return_id
WHERE r.status IN ('draft', 'pending_approval', 'approved') AND r.id <> sqlc.arg('except_id')
  AND l.goods_receipt_line_id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY l.goods_receipt_line_id;

-- name: InsertPurchaseReturn :one
INSERT INTO purchase_returns (
    supplier_id, supplier_name, purchase_order_id, division_id, origin, source_type, source_id, source_label,
    business_date, reason, note, status, created_by, updated_by)
VALUES (
    sqlc.arg('supplier_id'), sqlc.arg('supplier_name'), sqlc.arg('purchase_order_id'), sqlc.arg('division_id'),
    sqlc.arg('origin'), sqlc.arg('source_type'), sqlc.narg('source_id'), sqlc.arg('source_label'),
    sqlc.arg('business_date'), sqlc.arg('reason'), sqlc.arg('note'), 'draft', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdatePurchaseReturn :execrows
UPDATE purchase_returns
SET number = sqlc.narg('number'), debit_note_number = sqlc.narg('debit_note_number'),
    business_date = sqlc.arg('business_date'), reason = sqlc.arg('reason'), note = sqlc.arg('note'),
    total_amount = sqlc.arg('total_amount'), unbilled_amount = sqlc.arg('unbilled_amount'),
    billed_amount = sqlc.arg('billed_amount'), status = sqlc.arg('status'),
    journal_entry_id = sqlc.narg('journal_entry_id'), cancel_reason = sqlc.arg('cancel_reason'),
    submitted_at = sqlc.narg('submitted_at'), approved_at = sqlc.narg('approved_at'), posted_at = sqlc.narg('posted_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeletePurchaseReturnLines :exec
DELETE FROM purchase_return_lines WHERE purchase_return_id = $1;

-- name: InsertPurchaseReturnLine :exec
INSERT INTO purchase_return_lines (
    purchase_return_id, line_no, goods_receipt_line_id, product_id, batch_id, location_id, stock_status, qty,
    unit_cost, amount, unbilled_qty, unbilled_amount, inventory_account_code, note)
VALUES (
    sqlc.arg('purchase_return_id'), sqlc.arg('line_no'), sqlc.arg('goods_receipt_line_id'), sqlc.arg('product_id'),
    sqlc.narg('batch_id'), sqlc.arg('location_id'), sqlc.arg('stock_status'), sqlc.arg('qty'), sqlc.arg('unit_cost'),
    sqlc.arg('amount'), sqlc.arg('unbilled_qty'), sqlc.arg('unbilled_amount'), sqlc.arg('inventory_account_code'),
    sqlc.arg('note'));
