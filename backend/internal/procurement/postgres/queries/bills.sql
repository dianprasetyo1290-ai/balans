-- name: ListSupplierBills :many
SELECT b.*, o.number AS order_number, o.supplier_name
FROM supplier_bills b
JOIN purchase_orders o ON o.id = b.purchase_order_id
WHERE b.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR b.status = sqlc.narg('status')::text)
  AND (sqlc.narg('match_status')::text IS NULL OR b.match_status = sqlc.narg('match_status')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR b.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR b.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR b.bill_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR b.bill_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR b.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY b.bill_date DESC, b.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountSupplierBills :one
SELECT count(*)
FROM supplier_bills b
JOIN purchase_orders o ON o.id = b.purchase_order_id
WHERE b.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR b.status = sqlc.narg('status')::text)
  AND (sqlc.narg('match_status')::text IS NULL OR b.match_status = sqlc.narg('match_status')::text)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR b.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR b.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR b.bill_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR b.bill_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR b.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetSupplierBill :one
SELECT b.*, o.number AS order_number, o.supplier_name
FROM supplier_bills b
JOIN purchase_orders o ON o.id = b.purchase_order_id
WHERE b.id = $1;

-- name: LockSupplierBill :one
SELECT id FROM supplier_bills WHERE id = $1 FOR UPDATE;

-- name: ListSupplierBillLines :many
SELECT * FROM supplier_bill_lines WHERE supplier_bill_id = $1 ORDER BY line_no;

-- name: ListSupplierBillAdvances :many
SELECT ba.purchase_advance_id, ba.amount, a.number AS advance_number
FROM supplier_bill_advances ba
JOIN purchase_advances a ON a.id = ba.purchase_advance_id
WHERE ba.supplier_bill_id = $1
ORDER BY a.payment_date, a.created_at;

-- name: SupplierBillNumberTaken :one
SELECT EXISTS (
    SELECT 1 FROM supplier_bills
    WHERE supplier_id = sqlc.arg('supplier_id') AND upper(number) = upper(sqlc.arg('number')::text)
      AND status <> 'cancelled' AND id <> sqlc.arg('except_id')
) AS taken;

-- name: PendingBillQtyByReceiptLines :many
-- Qty baris GR di tagihan lain yang menunggu persetujuan selisih (belum mengurangi sisa GR).
SELECT l.goods_receipt_line_id, sum(l.qty)::numeric AS qty
FROM supplier_bill_lines l
JOIN supplier_bills b ON b.id = l.supplier_bill_id
WHERE b.status = 'pending_approval' AND b.id <> sqlc.arg('except_id')
  AND l.goods_receipt_line_id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY l.goods_receipt_line_id;

-- name: InsertSupplierBill :one
INSERT INTO supplier_bills (
    number, supplier_id, purchase_order_id, division_id, bill_date, due_date, note, subtotal_amount, grni_amount,
    price_variance_amount, withholding_type, withholding_rate, withholding_amount, advance_applied_amount,
    total_payable_amount, match_status, status, created_by, updated_by)
VALUES (
    sqlc.arg('number'), sqlc.arg('supplier_id'), sqlc.arg('purchase_order_id'), sqlc.arg('division_id'),
    sqlc.arg('bill_date'), sqlc.arg('due_date'), sqlc.arg('note'), sqlc.arg('subtotal_amount'), sqlc.arg('grni_amount'),
    sqlc.arg('price_variance_amount'), sqlc.arg('withholding_type'), sqlc.arg('withholding_rate'),
    sqlc.arg('withholding_amount'), sqlc.arg('advance_applied_amount'), sqlc.arg('total_payable_amount'),
    sqlc.arg('match_status'), 'draft', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateSupplierBill :execrows
UPDATE supplier_bills
SET number = sqlc.arg('number'), bill_date = sqlc.arg('bill_date'), due_date = sqlc.arg('due_date'),
    note = sqlc.arg('note'), subtotal_amount = sqlc.arg('subtotal_amount'), grni_amount = sqlc.arg('grni_amount'),
    price_variance_amount = sqlc.arg('price_variance_amount'), withholding_type = sqlc.arg('withholding_type'),
    withholding_rate = sqlc.arg('withholding_rate'), withholding_amount = sqlc.arg('withholding_amount'),
    advance_applied_amount = sqlc.arg('advance_applied_amount'), total_payable_amount = sqlc.arg('total_payable_amount'),
    paid_amount = sqlc.arg('paid_amount'), match_status = sqlc.arg('match_status'), status = sqlc.arg('status'),
    journal_entry_id = sqlc.narg('journal_entry_id'), advance_journal_entry_id = sqlc.narg('advance_journal_entry_id'),
    cancel_reason = sqlc.arg('cancel_reason'), submitted_at = sqlc.narg('submitted_at'),
    posted_at = sqlc.narg('posted_at'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeleteSupplierBillLines :exec
DELETE FROM supplier_bill_lines WHERE supplier_bill_id = $1;

-- name: InsertSupplierBillLine :exec
INSERT INTO supplier_bill_lines (
    supplier_bill_id, line_no, goods_receipt_line_id, purchase_order_line_id, product_id, qty, receipt_open_qty,
    unit_price_amount, po_unit_price_amount, line_amount, grni_amount, price_variance_amount, exceeds_tolerance, note)
VALUES (
    sqlc.arg('supplier_bill_id'), sqlc.arg('line_no'), sqlc.arg('goods_receipt_line_id'),
    sqlc.arg('purchase_order_line_id'), sqlc.arg('product_id'), sqlc.arg('qty'), sqlc.arg('receipt_open_qty'),
    sqlc.arg('unit_price_amount'), sqlc.arg('po_unit_price_amount'), sqlc.arg('line_amount'), sqlc.arg('grni_amount'),
    sqlc.arg('price_variance_amount'), sqlc.arg('exceeds_tolerance'), sqlc.arg('note'));

-- name: InsertSupplierBillAdvance :exec
INSERT INTO supplier_bill_advances (supplier_bill_id, purchase_advance_id, amount)
VALUES (sqlc.arg('supplier_bill_id'), sqlc.arg('purchase_advance_id'), sqlc.arg('amount'));

-- name: InsertSupplierPayable :exec
INSERT INTO supplier_payables (
    document_type, document_id, document_number, supplier_id, purchase_order_id, division_id, business_date, due_date,
    amount, open_amount, created_by, updated_by)
VALUES (
    sqlc.arg('document_type'), sqlc.arg('document_id'), sqlc.arg('document_number'), sqlc.arg('supplier_id'),
    sqlc.arg('purchase_order_id'), sqlc.arg('division_id'), sqlc.arg('business_date'), sqlc.arg('due_date'),
    sqlc.arg('amount'), sqlc.arg('open_amount'), sqlc.arg('actor_id'), sqlc.arg('actor_id'));

-- name: OpenSupplierPayables :many
SELECT * FROM supplier_payables
WHERE supplier_id = $1 AND open_amount <> 0
ORDER BY due_date, business_date, document_number;

-- name: GetSupplierPayableForUpdate :one
SELECT * FROM supplier_payables WHERE id = $1 FOR UPDATE;

-- name: UpdateSupplierPayableOpen :execrows
UPDATE supplier_payables
SET open_amount = sqlc.arg('open_amount'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
