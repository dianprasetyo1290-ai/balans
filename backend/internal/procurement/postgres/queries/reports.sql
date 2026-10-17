-- Laporan pengadaan (RPT-10, RPT-11 umur utang, RPT-04 komposisi HPP batch).

-- name: OutstandingPurchaseOrderLines :many
-- Baris PO disetujui yang belum diterima penuh atau belum ditagih penuh (RPT-10).
SELECT o.id, coalesce(o.number, '')::text AS number, o.order_type, o.supplier_id, o.supplier_code, o.supplier_name,
       o.business_date, o.expected_date, o.status, l.line_no, l.product_id, l.description, l.ordered_qty, l.received_qty,
       l.billed_qty, l.unit_price_amount
FROM purchase_orders o
JOIN purchase_order_lines l ON l.purchase_order_id = o.id
WHERE o.deleted_at IS NULL AND o.status IN ('approved', 'partially_received', 'received')
  AND (l.received_qty < l.ordered_qty OR l.billed_qty < l.received_qty)
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR o.supplier_id = sqlc.narg('supplier_id'))
ORDER BY o.expected_date, o.number, l.line_no
LIMIT 5000;

-- name: PayableAgingReport :many
-- Utang pemasok terbuka per dokumen (RPT-11 umur utang).
SELECT p.id, p.document_type, p.document_number, p.supplier_id, coalesce(o.supplier_code, '')::text AS supplier_code,
       coalesce(o.supplier_name, '')::text AS supplier_name, p.business_date, p.due_date, p.amount, p.open_amount
FROM supplier_payables p
LEFT JOIN purchase_orders o ON o.id = p.purchase_order_id
WHERE p.open_amount <> 0 AND p.business_date <= sqlc.arg('as_of')::date
ORDER BY 6, p.due_date;

-- name: BatchCostReport :many
-- Komposisi HPP per batch dari Penerimaan Barang diposting: qty baik, reject & disposisinya, nilai PO, packaging
-- terpakai, nilai batch, dan HPP per unit (RPT-04).
SELECT g.id AS receipt_id, coalesce(g.number, '')::text AS receipt_number, g.business_date, o.supplier_name,
       o.order_type, l.product_id, l.batch_id, l.batch_number, l.good_qty, l.reject_qty, l.reject_disposition,
       l.unit_price_amount, l.grni_amount, l.material_cost_amount, l.inventory_amount, l.unit_cost
FROM goods_receipts g
JOIN goods_receipt_lines l ON l.goods_receipt_id = g.id
JOIN purchase_order_lines pl ON pl.id = l.purchase_order_line_id
JOIN purchase_orders o ON o.id = pl.purchase_order_id
WHERE g.status = 'posted'
  AND g.business_date >= sqlc.arg('date_from')::date AND g.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('product_id')::uuid IS NULL OR l.product_id = sqlc.narg('product_id'))
ORDER BY g.business_date, g.number, l.line_no
LIMIT 5000;
