-- name: ListGoodsReceipts :many
SELECT r.*, o.number AS order_number, o.supplier_name
FROM goods_receipts r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (cardinality(sqlc.arg('order_types')::text[]) = 0 OR r.order_type = ANY(sqlc.arg('order_types')::text[]))
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR r.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR r.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR r.business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR r.business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.delivery_note_number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY r.business_date DESC, r.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountGoodsReceipts :one
SELECT count(*)
FROM goods_receipts r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (cardinality(sqlc.arg('order_types')::text[]) = 0 OR r.order_type = ANY(sqlc.arg('order_types')::text[]))
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR r.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR r.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR r.business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR r.business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.delivery_note_number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetGoodsReceipt :one
SELECT r.*, o.number AS order_number, o.supplier_name
FROM goods_receipts r
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.id = $1;

-- name: LockGoodsReceipt :one
SELECT id FROM goods_receipts WHERE id = $1 FOR UPDATE;

-- name: ListGoodsReceiptLines :many
SELECT * FROM goods_receipt_lines
WHERE goods_receipt_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY goods_receipt_id, line_no;

-- name: ListGoodsReceiptMaterialUsages :many
SELECT * FROM goods_receipt_material_usages
WHERE goods_receipt_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY goods_receipt_id, line_no;

-- name: ListGoodsReceiptMaterialConsumptions :many
SELECT c.* FROM goods_receipt_material_consumptions c
JOIN goods_receipt_material_usages u ON u.id = c.usage_id
WHERE u.goods_receipt_id = $1
ORDER BY c.usage_id, c.id;

-- name: InsertGoodsReceipt :one
INSERT INTO goods_receipts (
    purchase_order_id, supplier_id, division_id, order_type, business_date, location_id, toll_location_id,
    delivery_note_number, note, status, created_by, updated_by)
VALUES (
    sqlc.arg('purchase_order_id'), sqlc.arg('supplier_id'), sqlc.arg('division_id'), sqlc.arg('order_type'),
    sqlc.arg('business_date'), sqlc.arg('location_id'), sqlc.narg('toll_location_id'), sqlc.arg('delivery_note_number'),
    sqlc.arg('note'), 'draft', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateGoodsReceipt :execrows
UPDATE goods_receipts
SET number = sqlc.narg('number'), business_date = sqlc.arg('business_date'), location_id = sqlc.arg('location_id'),
    toll_location_id = sqlc.narg('toll_location_id'), delivery_note_number = sqlc.arg('delivery_note_number'),
    note = sqlc.arg('note'), inventory_amount = sqlc.arg('inventory_amount'),
    material_amount = sqlc.arg('material_amount'), grni_amount = sqlc.arg('grni_amount'), status = sqlc.arg('status'),
    inbound_id = sqlc.narg('inbound_id'), journal_entry_id = sqlc.narg('journal_entry_id'),
    reversal_journal_entry_id = sqlc.narg('reversal_journal_entry_id'), cancel_reason = sqlc.arg('cancel_reason'),
    posted_at = sqlc.narg('posted_at'), cancelled_at = sqlc.narg('cancelled_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeleteGoodsReceiptLines :exec
DELETE FROM goods_receipt_lines WHERE goods_receipt_id = $1;

-- name: DeleteGoodsReceiptMaterialUsages :exec
DELETE FROM goods_receipt_material_usages WHERE goods_receipt_id = $1;

-- name: InsertGoodsReceiptLine :exec
INSERT INTO goods_receipt_lines (
    goods_receipt_id, line_no, purchase_order_line_id, product_id, good_qty, reject_qty, reject_disposition,
    batch_number, production_date, expiry_date, unit_price_amount, grni_qty, grni_amount, note)
VALUES (
    sqlc.arg('goods_receipt_id'), sqlc.arg('line_no'), sqlc.arg('purchase_order_line_id'), sqlc.arg('product_id'),
    sqlc.arg('good_qty'), sqlc.arg('reject_qty'), sqlc.arg('reject_disposition'), sqlc.arg('batch_number'),
    sqlc.narg('production_date'), sqlc.narg('expiry_date'), sqlc.arg('unit_price_amount'), sqlc.arg('grni_qty'),
    sqlc.arg('grni_amount'), sqlc.arg('note'));

-- name: UpdateGoodsReceiptLine :exec
UPDATE goods_receipt_lines
SET batch_id = sqlc.narg('batch_id'), material_cost_amount = sqlc.arg('material_cost_amount'),
    inventory_amount = sqlc.arg('inventory_amount'), unit_cost = sqlc.arg('unit_cost'),
    inventory_account_code = sqlc.arg('inventory_account_code'), billed_qty = sqlc.arg('billed_qty'),
    billed_grni_amount = sqlc.arg('billed_grni_amount'), returned_qty = sqlc.arg('returned_qty'),
    returned_unbilled_qty = sqlc.arg('returned_unbilled_qty'), returned_grni_amount = sqlc.arg('returned_grni_amount')
WHERE id = sqlc.arg('id');

-- name: InsertGoodsReceiptMaterialUsage :exec
INSERT INTO goods_receipt_material_usages (
    goods_receipt_id, line_no, material_product_id, from_location_id, used_good_qty, used_reject_qty, note)
VALUES (
    sqlc.arg('goods_receipt_id'), sqlc.arg('line_no'), sqlc.arg('material_product_id'), sqlc.arg('from_location_id'),
    sqlc.arg('used_good_qty'), sqlc.arg('used_reject_qty'), sqlc.arg('note'));

-- name: UpdateGoodsReceiptMaterialUsage :exec
UPDATE goods_receipt_material_usages
SET cost_amount = sqlc.arg('cost_amount'), account_code = sqlc.arg('account_code')
WHERE id = sqlc.arg('id');

-- name: InsertGoodsReceiptMaterialConsumption :exec
INSERT INTO goods_receipt_material_consumptions (usage_id, batch_id, qty, unit_cost)
VALUES (sqlc.arg('usage_id'), sqlc.narg('batch_id'), sqlc.arg('qty'), sqlc.arg('unit_cost'));

-- name: ReceiptLinesByIDs :many
-- Baris GR beserta status & nomor GR dan PO-nya (3-way match tagihan, retur).
SELECT l.*, r.number AS receipt_number, r.status AS receipt_status, r.purchase_order_id, r.location_id, r.business_date
FROM goods_receipt_lines l
JOIN goods_receipts r ON r.id = l.goods_receipt_id
WHERE l.id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ReceiptLinesByOrder :many
-- Baris GR diposting satu PO (form tagihan & retur).
SELECT l.*, r.number AS receipt_number, r.status AS receipt_status, r.purchase_order_id, r.location_id, r.business_date
FROM goods_receipt_lines l
JOIN goods_receipts r ON r.id = l.goods_receipt_id
WHERE r.purchase_order_id = $1 AND r.status = 'posted'
ORDER BY r.business_date, r.number, l.line_no;

-- name: ReceiptLineByBatch :many
-- GR diposting terakhir yang menerima batch tertentu (retur QC Outbound yang hanya tahu batch).
SELECT l.id, r.purchase_order_id
FROM goods_receipt_lines l
JOIN goods_receipts r ON r.id = l.goods_receipt_id
WHERE l.product_id = sqlc.arg('product_id') AND l.batch_id = sqlc.arg('batch_id') AND r.status = 'posted'
ORDER BY r.business_date DESC, r.number DESC
LIMIT 1;

-- name: ReceiptIDsByLines :many
SELECT DISTINCT goods_receipt_id FROM goods_receipt_lines WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: OpenReceiptLines :many
-- Laporan Barang Diterima Belum Ditagih: baris GR diposting yang saldo 2102-nya belum nol.
SELECT l.id, l.goods_receipt_id, l.line_no, l.product_id, l.grni_qty, l.grni_amount, l.billed_qty,
       l.billed_grni_amount, l.returned_unbilled_qty, l.returned_grni_amount, l.unit_price_amount,
       r.number AS receipt_number, r.business_date, r.purchase_order_id, r.supplier_id, r.division_id,
       o.number AS order_number, o.supplier_name, o.order_type
FROM goods_receipt_lines l
JOIN goods_receipts r ON r.id = l.goods_receipt_id
JOIN purchase_orders o ON o.id = r.purchase_order_id
WHERE r.status = 'posted'
  AND l.grni_amount - l.billed_grni_amount - l.returned_grni_amount <> 0
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR r.supplier_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('purchase_order_id')::uuid IS NULL OR r.purchase_order_id = sqlc.narg('purchase_order_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.supplier_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY o.supplier_name, o.number, r.business_date, r.number, l.line_no;

-- name: TotalOpenGRNI :one
-- Total saldo Barang Diterima Belum Ditagih seluruh GR (pembanding saldo akun 2102).
SELECT coalesce(sum(l.grni_amount - l.billed_grni_amount - l.returned_grni_amount), 0)::numeric AS total
FROM goods_receipt_lines l
JOIN goods_receipts r ON r.id = l.goods_receipt_id
WHERE r.status = 'posted';
