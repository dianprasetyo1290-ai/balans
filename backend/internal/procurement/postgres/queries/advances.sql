-- name: ListPurchaseAdvances :many
SELECT a.*, o.number AS order_number, o.supplier_name
FROM purchase_advances a
JOIN purchase_orders o ON o.id = a.purchase_order_id
WHERE a.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR a.status = sqlc.narg('status')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR a.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR a.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR a.payment_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR a.payment_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR a.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY a.payment_date DESC, a.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPurchaseAdvances :one
SELECT count(*)
FROM purchase_advances a
JOIN purchase_orders o ON o.id = a.purchase_order_id
WHERE a.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR a.status = sqlc.narg('status')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR a.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR a.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR a.payment_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR a.payment_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR a.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetPurchaseAdvance :one
SELECT a.*, o.number AS order_number, o.supplier_name
FROM purchase_advances a
JOIN purchase_orders o ON o.id = a.purchase_order_id
WHERE a.id = $1;

-- name: LockPurchaseAdvance :one
SELECT id FROM purchase_advances WHERE id = $1 FOR UPDATE;

-- name: ListAdvancesByOrder :many
-- Uang muka satu PO, urut tanggal bayar (FIFO pemotongan tagihan).
SELECT a.*, o.number AS order_number, o.supplier_name
FROM purchase_advances a
JOIN purchase_orders o ON o.id = a.purchase_order_id
WHERE a.purchase_order_id = $1 AND a.deleted_at IS NULL
ORDER BY a.payment_date, a.created_at;

-- name: InsertPurchaseAdvance :one
INSERT INTO purchase_advances (
    purchase_order_id, supplier_id, division_id, amount, payment_date, bank_account_code, note, status, created_by,
    updated_by)
VALUES (
    sqlc.arg('purchase_order_id'), sqlc.arg('supplier_id'), sqlc.arg('division_id'), sqlc.arg('amount'),
    sqlc.arg('payment_date'), sqlc.arg('bank_account_code'), sqlc.arg('note'), 'draft', sqlc.arg('actor_id'),
    sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdatePurchaseAdvance :execrows
UPDATE purchase_advances
SET number = sqlc.narg('number'), amount = sqlc.arg('amount'), payment_date = sqlc.arg('payment_date'),
    bank_account_code = sqlc.arg('bank_account_code'), note = sqlc.arg('note'),
    applied_amount = sqlc.arg('applied_amount'), status = sqlc.arg('status'),
    journal_entry_id = sqlc.narg('journal_entry_id'), reversal_journal_entry_id = sqlc.narg('reversal_journal_entry_id'),
    cancel_reason = sqlc.arg('cancel_reason'), posted_at = sqlc.narg('posted_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
