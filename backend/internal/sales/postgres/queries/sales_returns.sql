-- name: ListSalesReturns :many
SELECT sqlc.embed(r), coalesce(o.number, '')::text AS order_number, coalesce(i.number, '')::text AS invoice_number
FROM sales_returns r
JOIN sales_orders o ON o.id = r.sales_order_id
LEFT JOIN invoices i ON i.id = r.invoice_id
WHERE r.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR r.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('sales_order_id')::uuid IS NULL OR r.sales_order_id = sqlc.narg('sales_order_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR r.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('pickup_user_id')::uuid IS NULL OR r.pickup_user_id = sqlc.narg('pickup_user_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY r.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountSalesReturns :one
SELECT count(*) FROM sales_returns r
JOIN sales_orders o ON o.id = r.sales_order_id
WHERE r.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR r.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('sales_order_id')::uuid IS NULL OR r.sales_order_id = sqlc.narg('sales_order_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR r.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('pickup_user_id')::uuid IS NULL OR r.pickup_user_id = sqlc.narg('pickup_user_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR r.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetSalesReturn :one
SELECT sqlc.embed(r), coalesce(o.number, '')::text AS order_number, coalesce(i.number, '')::text AS invoice_number
FROM sales_returns r
JOIN sales_orders o ON o.id = r.sales_order_id
LEFT JOIN invoices i ON i.id = r.invoice_id
WHERE r.id = $1;

-- name: GetSalesReturnForUpdate :one
SELECT sqlc.embed(r), coalesce(o.number, '')::text AS order_number, coalesce(i.number, '')::text AS invoice_number
FROM sales_returns r
JOIN sales_orders o ON o.id = r.sales_order_id
LEFT JOIN invoices i ON i.id = r.invoice_id
WHERE r.id = $1
FOR UPDATE OF r;

-- name: InsertSalesReturn :one
INSERT INTO sales_returns (
    sales_order_id, invoice_id, order_type, channel_id, sub_channel_id, party_type, party_id, party_name,
    source_location_id, return_date, settlement_method, requires_verification, note, return_amount, cost_amount,
    created_by, updated_by)
VALUES (
    sqlc.arg('sales_order_id'), sqlc.narg('invoice_id'), sqlc.arg('order_type'), sqlc.arg('channel_id'),
    sqlc.arg('sub_channel_id'), sqlc.arg('party_type'), sqlc.narg('party_id'), sqlc.arg('party_name'),
    sqlc.arg('source_location_id'), sqlc.arg('return_date'), sqlc.arg('settlement_method'),
    sqlc.arg('requires_verification'), sqlc.arg('note'), sqlc.arg('return_amount'), sqlc.arg('cost_amount'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateSalesReturn :execrows
UPDATE sales_returns SET
    number = sqlc.narg('number'), invoice_id = sqlc.narg('invoice_id'), return_date = sqlc.arg('return_date'),
    settlement_method = sqlc.arg('settlement_method'), note = sqlc.arg('note'),
    send_back_note = sqlc.arg('send_back_note'), verified_by = sqlc.narg('verified_by'),
    verified_at = sqlc.narg('verified_at'), verify_note = sqlc.arg('verify_note'),
    pickup_required = sqlc.arg('pickup_required'), pickup_user_id = sqlc.narg('pickup_user_id'),
    pickup_assigned_at = sqlc.narg('pickup_assigned_at'), picked_up_at = sqlc.narg('picked_up_at'),
    pickup_note = sqlc.arg('pickup_note'), pickup_file_ids = sqlc.arg('pickup_file_ids')::uuid[],
    received_by = sqlc.narg('received_by'), received_at = sqlc.narg('received_at'),
    receive_note = sqlc.arg('receive_note'), documentation_file_ids = sqlc.arg('documentation_file_ids')::uuid[],
    inbound_id = sqlc.narg('inbound_id'), inbound_number = sqlc.arg('inbound_number'),
    return_amount = sqlc.arg('return_amount'), cost_amount = sqlc.arg('cost_amount'), status = sqlc.arg('status'),
    cancel_reason = sqlc.arg('cancel_reason'), posted_at = sqlc.narg('posted_at'),
    journal_entry_id = sqlc.narg('journal_entry_id'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListSalesReturnLines :many
SELECT * FROM sales_return_lines WHERE sales_return_id = $1 ORDER BY line_no;

-- name: ListSalesReturnItems :many
SELECT * FROM sales_return_items WHERE sales_return_id = $1 ORDER BY id;

-- name: DeleteSalesReturnItems :exec
DELETE FROM sales_return_items WHERE sales_return_id = $1;

-- name: DeleteSalesReturnLines :exec
DELETE FROM sales_return_lines WHERE sales_return_id = $1;

-- name: InsertSalesReturnLine :one
INSERT INTO sales_return_lines (
    sales_return_id, line_no, sales_order_line_id, product_id, batch_id, qty, return_reason_id, unit_price_amount,
    line_amount, original_unit_cost, cost_amount, note)
VALUES (
    sqlc.arg('sales_return_id'), sqlc.arg('line_no'), sqlc.arg('sales_order_line_id'), sqlc.arg('product_id'),
    sqlc.narg('batch_id'), sqlc.arg('qty'), sqlc.arg('return_reason_id'), sqlc.arg('unit_price_amount'),
    sqlc.arg('line_amount'), sqlc.arg('original_unit_cost'), sqlc.arg('cost_amount'), sqlc.arg('note'))
RETURNING id;

-- name: UpdateSalesReturnLineQty :exec
UPDATE sales_return_lines SET qty = sqlc.arg('qty'), line_amount = sqlc.arg('line_amount'),
    cost_amount = sqlc.arg('cost_amount')
WHERE id = sqlc.arg('id');

-- name: InsertSalesReturnItem :one
INSERT INTO sales_return_items (
    sales_return_id, sales_return_line_id, allocation_id, product_id, batch_id, qty, unit_cost, cost_amount)
VALUES (
    sqlc.arg('sales_return_id'), sqlc.arg('sales_return_line_id'), sqlc.arg('allocation_id'), sqlc.arg('product_id'),
    sqlc.narg('batch_id'), sqlc.arg('qty'), sqlc.arg('unit_cost'), sqlc.arg('cost_amount'))
RETURNING id;

-- name: UpdateSalesReturnItem :exec
UPDATE sales_return_items SET qty = sqlc.arg('qty'), cost_amount = sqlc.arg('cost_amount'),
    passed_qty = sqlc.arg('passed_qty'), rejected_qty = sqlc.arg('rejected_qty'), qc_done = sqlc.arg('qc_done')
WHERE id = sqlc.arg('id');

-- name: ReturnedQtyByOrderLine :many
-- Qty retur per baris Sales Order dari retur yang belum dibatalkan (selain retur yang sedang diubah).
SELECT l.sales_order_line_id, sum(l.qty)::numeric AS qty
FROM sales_return_lines l JOIN sales_returns r ON r.id = l.sales_return_id
WHERE r.sales_order_id = sqlc.arg('sales_order_id') AND r.status <> 'cancelled' AND r.deleted_at IS NULL
  AND r.id <> sqlc.arg('exclude_id')
GROUP BY l.sales_order_line_id;

-- name: ReturnSourceOrders :many
-- Sales Order yang sudah ada barang terkirim (sumber retur).
SELECT o.id, coalesce(o.number, '')::text AS number, o.order_type, o.party_name, o.business_date, o.status,
       o.total_amount
FROM sales_orders o
WHERE o.deleted_at IS NULL AND o.status IN ('in_progress', 'in_transit', 'completed')
  AND EXISTS (SELECT 1 FROM sales_order_allocations a WHERE a.sales_order_id = o.id AND a.shipped_qty > 0)
  AND (sqlc.narg('search')::text IS NULL OR o.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.external_order_id ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY o.business_date DESC, o.created_at DESC
LIMIT 20;

-- name: ListCreditNotes :many
SELECT sqlc.embed(c), coalesce(r.number, '')::text AS return_number, coalesce(i.number, '')::text AS invoice_number
FROM credit_notes c
JOIN sales_returns r ON r.id = c.sales_return_id
LEFT JOIN invoices i ON i.id = c.invoice_id
WHERE c.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR c.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('party_type')::text IS NULL OR c.party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR c.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('settlement')::text IS NULL OR c.settlement_method = sqlc.narg('settlement')::text)
  AND (sqlc.narg('search')::text IS NULL OR c.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY c.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountCreditNotes :one
SELECT count(*) FROM credit_notes c
JOIN sales_returns r ON r.id = c.sales_return_id
WHERE c.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR c.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('party_type')::text IS NULL OR c.party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR c.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('settlement')::text IS NULL OR c.settlement_method = sqlc.narg('settlement')::text)
  AND (sqlc.narg('search')::text IS NULL OR c.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR r.number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetCreditNote :one
SELECT sqlc.embed(c), coalesce(r.number, '')::text AS return_number, coalesce(i.number, '')::text AS invoice_number
FROM credit_notes c
JOIN sales_returns r ON r.id = c.sales_return_id
LEFT JOIN invoices i ON i.id = c.invoice_id
WHERE c.id = $1;

-- name: GetCreditNoteForUpdate :one
SELECT sqlc.embed(c), coalesce(r.number, '')::text AS return_number, coalesce(i.number, '')::text AS invoice_number
FROM credit_notes c
JOIN sales_returns r ON r.id = c.sales_return_id
LEFT JOIN invoices i ON i.id = c.invoice_id
WHERE c.id = $1
FOR UPDATE OF c;

-- name: CreditNoteIDByReturn :one
SELECT id FROM credit_notes WHERE sales_return_id = $1;

-- name: InsertCreditNote :one
INSERT INTO credit_notes (
    number, sales_return_id, invoice_id, party_type, party_id, party_name, channel_id, sub_channel_id,
    settlement_method, issue_date, amount, status, created_by, updated_by)
VALUES (
    sqlc.arg('number'), sqlc.arg('sales_return_id'), sqlc.narg('invoice_id'), sqlc.arg('party_type'),
    sqlc.arg('party_id'), sqlc.arg('party_name'), sqlc.arg('channel_id'), sqlc.arg('sub_channel_id'),
    sqlc.arg('settlement_method'), sqlc.arg('issue_date'), sqlc.arg('amount'), sqlc.arg('status'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateCreditNote :execrows
UPDATE credit_notes SET
    settlement_method = sqlc.arg('settlement_method'), applied_amount = sqlc.arg('applied_amount'),
    refunded_amount = sqlc.arg('refunded_amount'), status = sqlc.arg('status'),
    decision_note = sqlc.arg('decision_note'), refund_date = sqlc.narg('refund_date'),
    refund_account_code = sqlc.arg('refund_account_code'), refund_reference = sqlc.arg('refund_reference'),
    refund_journal_entry_id = sqlc.narg('refund_journal_entry_id'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: InsertCreditNoteApplication :exec
INSERT INTO credit_note_applications (credit_note_id, invoice_id, amount, applied_date, created_by)
VALUES (sqlc.arg('credit_note_id'), sqlc.arg('invoice_id'), sqlc.arg('amount'), sqlc.arg('applied_date'),
        sqlc.arg('actor_id'));

-- name: ListCreditNoteApplications :many
SELECT a.*, i.number AS invoice_number FROM credit_note_applications a
JOIN invoices i ON i.id = a.invoice_id
WHERE a.credit_note_id = $1
ORDER BY a.created_at;
