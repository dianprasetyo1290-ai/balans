-- name: ListSalesOrders :many
SELECT * FROM sales_orders
WHERE deleted_at IS NULL
  AND order_type = ANY(sqlc.arg('order_types')::text[])
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id')::uuid)
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR sub_channel_id = sqlc.narg('sub_channel_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_phone ILIKE '%' || sqlc.narg('search')::text || '%'
       OR external_order_id ILIKE '%' || sqlc.narg('search')::text || '%'
       OR client_po_number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'number' THEN number END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-number' THEN number END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'businessDate' THEN business_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-businessDate' THEN business_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'totalAmount' THEN total_amount END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-totalAmount' THEN total_amount END DESC,
  created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountSalesOrders :one
SELECT count(*) FROM sales_orders
WHERE deleted_at IS NULL
  AND order_type = ANY(sqlc.arg('order_types')::text[])
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id')::uuid)
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR sub_channel_id = sqlc.narg('sub_channel_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR business_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR business_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_phone ILIKE '%' || sqlc.narg('search')::text || '%'
       OR external_order_id ILIKE '%' || sqlc.narg('search')::text || '%'
       OR client_po_number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetSalesOrder :one
SELECT * FROM sales_orders WHERE id = $1;

-- name: GetSalesOrderForUpdate :one
SELECT * FROM sales_orders WHERE id = $1 FOR UPDATE;

-- name: SalesOrderIDByExternal :one
SELECT id FROM sales_orders WHERE sub_channel_id = $1 AND external_order_id = $2;

-- name: SalesOrderIDByImportRowKey :one
SELECT id FROM sales_orders WHERE import_row_key = $1;

-- name: ListSalesOrderLines :many
SELECT * FROM sales_order_lines WHERE sales_order_id = $1 ORDER BY line_no;

-- name: ListSalesOrderAllocations :many
SELECT * FROM sales_order_allocations WHERE sales_order_id = $1 ORDER BY id;

-- name: ListSalesOrderAllocationBatches :many
SELECT b.* FROM sales_order_allocation_batches b
JOIN sales_order_allocations a ON a.id = b.allocation_id
WHERE a.sales_order_id = $1
ORDER BY b.allocation_id, b.id;

-- name: InsertSalesOrder :one
INSERT INTO sales_orders (
    number, order_type, channel_id, sub_channel_id, customer_id, partner_id, party_code, party_name, party_phone,
    external_order_id, client_po_number, business_date, source_location_id, payment_mode, payment_term_days,
    ship_to_name, ship_to_phone, ship_to_address, gross_amount, line_discount_amount, document_discount_kind,
    document_discount_value, document_discount_amount, shipping_amount, total_amount, status, note, import_row_key,
    created_by, updated_by)
VALUES (
    sqlc.narg('number'), sqlc.arg('order_type'), sqlc.arg('channel_id'), sqlc.arg('sub_channel_id'),
    sqlc.narg('customer_id'), sqlc.narg('partner_id'), sqlc.arg('party_code'), sqlc.arg('party_name'),
    sqlc.arg('party_phone'), sqlc.arg('external_order_id'), sqlc.arg('client_po_number'), sqlc.arg('business_date'),
    sqlc.arg('source_location_id'), sqlc.arg('payment_mode'), sqlc.arg('payment_term_days'), sqlc.arg('ship_to_name'),
    sqlc.arg('ship_to_phone'), sqlc.arg('ship_to_address'), sqlc.arg('gross_amount'), sqlc.arg('line_discount_amount'),
    sqlc.arg('document_discount_kind'), sqlc.arg('document_discount_value'), sqlc.arg('document_discount_amount'),
    sqlc.arg('shipping_amount'), sqlc.arg('total_amount'), 'draft', sqlc.arg('note'), sqlc.narg('import_row_key'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateSalesOrder :execrows
UPDATE sales_orders
SET number = sqlc.narg('number'), channel_id = sqlc.arg('channel_id'), sub_channel_id = sqlc.arg('sub_channel_id'),
    customer_id = sqlc.narg('customer_id'), partner_id = sqlc.narg('partner_id'), party_code = sqlc.arg('party_code'),
    party_name = sqlc.arg('party_name'), party_phone = sqlc.arg('party_phone'),
    external_order_id = sqlc.arg('external_order_id'), client_po_number = sqlc.arg('client_po_number'),
    business_date = sqlc.arg('business_date'), source_location_id = sqlc.arg('source_location_id'),
    payment_mode = sqlc.arg('payment_mode'), payment_term_days = sqlc.arg('payment_term_days'),
    ship_to_name = sqlc.arg('ship_to_name'), ship_to_phone = sqlc.arg('ship_to_phone'),
    ship_to_address = sqlc.arg('ship_to_address'), gross_amount = sqlc.arg('gross_amount'),
    line_discount_amount = sqlc.arg('line_discount_amount'), document_discount_kind = sqlc.arg('document_discount_kind'),
    document_discount_value = sqlc.arg('document_discount_value'),
    document_discount_amount = sqlc.arg('document_discount_amount'), shipping_amount = sqlc.arg('shipping_amount'),
    total_amount = sqlc.arg('total_amount'), discount_approval_required = sqlc.arg('discount_approval_required'),
    discount_approved_at = sqlc.narg('discount_approved_at'), status = sqlc.arg('status'), note = sqlc.arg('note'),
    cancel_reason = sqlc.arg('cancel_reason'), confirmed_at = sqlc.narg('confirmed_at'),
    shipped_at = sqlc.narg('shipped_at'), delivered_at = sqlc.narg('delivered_at'),
    payment_proof_file_id = sqlc.narg('payment_proof_file_id'),
    payment_proof_uploaded_at = sqlc.narg('payment_proof_uploaded_at'), allocation_round = sqlc.arg('allocation_round'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeleteSalesOrderLines :exec
DELETE FROM sales_order_lines WHERE sales_order_id = $1;

-- name: InsertSalesOrderLine :one
INSERT INTO sales_order_lines (
    sales_order_id, line_no, product_id, is_bundle, qty, unit_price_amount, price_rule, price_source_id, price_list_id,
    price_list_version_id, price_description, discount_kind, discount_value, discount_amount, gross_amount,
    line_total_amount, note)
VALUES (
    sqlc.arg('sales_order_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.arg('is_bundle'), sqlc.arg('qty'),
    sqlc.arg('unit_price_amount'), sqlc.arg('price_rule'), sqlc.narg('price_source_id'), sqlc.narg('price_list_id'),
    sqlc.narg('price_list_version_id'), sqlc.arg('price_description'), sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'), sqlc.arg('discount_amount'), sqlc.arg('gross_amount'), sqlc.arg('line_total_amount'),
    sqlc.arg('note'))
RETURNING id;

-- name: DeleteReleasedAllocationBatches :exec
DELETE FROM sales_order_allocation_batches b USING sales_order_allocations a
WHERE b.allocation_id = a.id AND a.sales_order_id = $1 AND a.shipped_qty = 0;

-- name: DeleteUnshippedAllocations :exec
DELETE FROM sales_order_allocations WHERE sales_order_id = $1 AND shipped_qty = 0;

-- name: InsertSalesOrderAllocation :exec
INSERT INTO sales_order_allocations (id, sales_order_id, sales_order_line_id, product_id, location_id, qty)
VALUES (sqlc.arg('id'), sqlc.arg('sales_order_id'), sqlc.arg('sales_order_line_id'), sqlc.arg('product_id'),
        sqlc.arg('location_id'), sqlc.arg('qty'));

-- name: InsertSalesOrderAllocationBatch :exec
INSERT INTO sales_order_allocation_batches (allocation_id, batch_id, qty)
VALUES (sqlc.arg('allocation_id'), sqlc.narg('batch_id'), sqlc.arg('qty'));

-- name: SetSalesOrderAllocation :exec
UPDATE sales_order_allocations SET shipped_qty = sqlc.arg('shipped_qty'), released = sqlc.arg('released')
WHERE id = sqlc.arg('id');

-- name: InsertSalesShipment :one
INSERT INTO sales_shipments (sales_order_id, source_type, source_id, source_label, business_date, cost_amount, created_by)
VALUES (sqlc.arg('sales_order_id'), sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('source_label'),
        sqlc.arg('business_date'), sqlc.arg('cost_amount'), sqlc.arg('actor_id'))
RETURNING *;

-- name: InsertSalesShipmentLine :exec
INSERT INTO sales_shipment_lines (shipment_id, allocation_id, product_id, qty, cost_amount)
VALUES (sqlc.arg('shipment_id'), sqlc.arg('allocation_id'), sqlc.arg('product_id'), sqlc.arg('qty'),
        sqlc.arg('cost_amount'));

-- name: GetSalesShipmentBySource :one
SELECT * FROM sales_shipments WHERE sales_order_id = $1 AND source_type = $2 AND source_id = $3;

-- name: ListSalesShipments :many
SELECT * FROM sales_shipments WHERE sales_order_id = $1 ORDER BY created_at;

-- name: ListSalesShipmentLines :many
SELECT l.* FROM sales_shipment_lines l JOIN sales_shipments s ON s.id = l.shipment_id
WHERE s.sales_order_id = $1 ORDER BY s.created_at, l.id;

-- name: ListProformas :many
SELECT p.*, o.number AS order_number, o.party_name, o.order_type FROM proforma_invoices p
JOIN sales_orders o ON o.id = p.sales_order_id
WHERE p.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status')::text)
  AND (sqlc.narg('search')::text IS NULL OR p.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.party_name ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY p.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountProformas :one
SELECT count(*) FROM proforma_invoices p
JOIN sales_orders o ON o.id = p.sales_order_id
WHERE p.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status')::text)
  AND (sqlc.narg('search')::text IS NULL OR p.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.party_name ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetProforma :one
SELECT p.*, o.number AS order_number, o.party_name, o.order_type FROM proforma_invoices p
JOIN sales_orders o ON o.id = p.sales_order_id WHERE p.id = $1;

-- name: GetProformaForUpdate :one
SELECT * FROM proforma_invoices WHERE id = $1 FOR UPDATE;

-- name: ProformasByOrder :many
SELECT * FROM proforma_invoices WHERE sales_order_id = $1 ORDER BY created_at;

-- name: InsertProforma :one
INSERT INTO proforma_invoices (number, sales_order_id, amount, issue_date, valid_until, status, created_by, updated_by)
VALUES (sqlc.arg('number'), sqlc.arg('sales_order_id'), sqlc.arg('amount'), sqlc.arg('issue_date'),
        sqlc.arg('valid_until'), sqlc.arg('status'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateProforma :execrows
UPDATE proforma_invoices
SET status = sqlc.arg('status'), paid_at = sqlc.narg('paid_at'), invoice_id = sqlc.narg('invoice_id'),
    cancel_reason = sqlc.arg('cancel_reason'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ExpiredProformaIDs :many
-- Proforma belum dibayar yang lewat masa berlaku (valid_until < hari ini).
SELECT id FROM proforma_invoices
WHERE status = 'awaiting_payment' AND valid_until < sqlc.arg('today')::date AND deleted_at IS NULL
ORDER BY valid_until, id
LIMIT 200;

-- name: DeleteAllocationBatches :exec
DELETE FROM sales_order_allocation_batches WHERE allocation_id = $1;
