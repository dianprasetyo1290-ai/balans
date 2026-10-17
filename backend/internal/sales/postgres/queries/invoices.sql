-- name: ListInvoices :many
SELECT i.*, coalesce(o.number, '')::text AS order_number FROM invoices i
LEFT JOIN sales_orders o ON o.id = i.sales_order_id
WHERE i.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR i.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('party_type')::text IS NULL OR i.party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR i.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR i.invoice_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR i.invoice_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('overdue_on')::date IS NULL OR (i.due_date < sqlc.narg('overdue_on')::date
       AND i.status IN ('awaiting_payment', 'partially_paid')))
  AND (sqlc.narg('search')::text IS NULL OR i.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.party_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'number' THEN i.number END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-number' THEN i.number END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'invoiceDate' THEN i.invoice_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-invoiceDate' THEN i.invoice_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'dueDate' THEN i.due_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-dueDate' THEN i.due_date END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'totalAmount' THEN i.total_amount END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-totalAmount' THEN i.total_amount END DESC,
  i.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountInvoices :one
SELECT count(*) FROM invoices i
LEFT JOIN sales_orders o ON o.id = i.sales_order_id
WHERE i.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR i.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('party_type')::text IS NULL OR i.party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR i.party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('date_from')::date IS NULL OR i.invoice_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR i.invoice_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('overdue_on')::date IS NULL OR (i.due_date < sqlc.narg('overdue_on')::date
       AND i.status IN ('awaiting_payment', 'partially_paid')))
  AND (sqlc.narg('search')::text IS NULL OR i.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR i.party_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: OpenInvoices :many
-- Invoice dengan sisa piutang per tanggal (umur piutang).
SELECT i.*, coalesce(o.number, '')::text AS order_number FROM invoices i
LEFT JOIN sales_orders o ON o.id = i.sales_order_id
WHERE i.deleted_at IS NULL AND i.status IN ('awaiting_payment', 'partially_paid')
  AND i.invoice_date <= sqlc.arg('as_of')::date
  AND (sqlc.narg('party_type')::text IS NULL OR i.party_type = sqlc.narg('party_type')::text)
ORDER BY i.party_name, i.due_date;

-- name: GetInvoice :one
SELECT i.*, coalesce(o.number, '')::text AS order_number FROM invoices i
LEFT JOIN sales_orders o ON o.id = i.sales_order_id WHERE i.id = $1;

-- name: GetInvoiceForUpdate :one
SELECT * FROM invoices WHERE id = $1 FOR UPDATE;

-- name: InvoicesByOrder :many
SELECT * FROM invoices WHERE sales_order_id = $1 ORDER BY created_at;

-- name: ListInvoiceLines :many
SELECT * FROM invoice_lines WHERE invoice_id = $1 ORDER BY line_no;

-- name: InsertInvoice :one
INSERT INTO invoices (
    number, invoice_type, sales_order_id, party_type, party_id, party_code, party_name, channel_id, sub_channel_id,
    invoice_date, due_date, gross_amount, discount_amount, shipping_amount, total_amount, paid_amount, status,
    created_by, updated_by)
VALUES (
    sqlc.arg('number'), sqlc.arg('invoice_type'), sqlc.narg('sales_order_id'), sqlc.arg('party_type'),
    sqlc.arg('party_id'), sqlc.arg('party_code'), sqlc.arg('party_name'), sqlc.arg('channel_id'),
    sqlc.arg('sub_channel_id'), sqlc.arg('invoice_date'), sqlc.arg('due_date'), sqlc.arg('gross_amount'),
    sqlc.arg('discount_amount'), sqlc.arg('shipping_amount'), sqlc.arg('total_amount'), sqlc.arg('paid_amount'),
    sqlc.arg('status'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: InsertInvoiceLine :exec
INSERT INTO invoice_lines (invoice_id, line_no, product_id, description, qty, unit_price_amount, discount_amount,
                           line_total_amount)
VALUES (sqlc.arg('invoice_id'), sqlc.arg('line_no'), sqlc.narg('product_id'), sqlc.arg('description'), sqlc.arg('qty'),
        sqlc.arg('unit_price_amount'), sqlc.arg('discount_amount'), sqlc.arg('line_total_amount'));

-- name: UpdateInvoicePayment :execrows
UPDATE invoices
SET paid_amount = sqlc.arg('paid_amount'), status = sqlc.arg('status'), cancel_reason = sqlc.arg('cancel_reason'),
    cancelled_at = sqlc.narg('cancelled_at'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListCustomerPayments :many
SELECT * FROM customer_payments
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('party_type')::text IS NULL OR party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('account_code')::text IS NULL OR account_code = sqlc.narg('account_code')::text)
  AND (sqlc.narg('date_from')::date IS NULL OR payment_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR payment_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR reference ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY payment_date DESC, created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountCustomerPayments :one
SELECT count(*) FROM customer_payments
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('party_type')::text IS NULL OR party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('account_code')::text IS NULL OR account_code = sqlc.narg('account_code')::text)
  AND (sqlc.narg('date_from')::date IS NULL OR payment_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR payment_date <= sqlc.narg('date_to')::date)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR party_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR reference ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetCustomerPayment :one
SELECT * FROM customer_payments WHERE id = $1;

-- name: GetCustomerPaymentForUpdate :one
SELECT * FROM customer_payments WHERE id = $1 FOR UPDATE;

-- name: ListPaymentAllocations :many
SELECT a.*, i.number AS invoice_number FROM customer_payment_allocations a
JOIN invoices i ON i.id = a.invoice_id
WHERE a.payment_id = ANY(sqlc.arg('payment_ids')::uuid[])
ORDER BY a.payment_id, i.number;

-- name: PaymentAllocationsByInvoice :many
SELECT a.*, p.number AS payment_number, p.payment_date, p.status AS payment_status FROM customer_payment_allocations a
JOIN customer_payments p ON p.id = a.payment_id
WHERE a.invoice_id = $1
ORDER BY p.payment_date, p.number;

-- name: InsertCustomerPayment :one
INSERT INTO customer_payments (
    number, party_type, party_id, party_name, payment_date, amount, method, account_code, reference, proof_file_id,
    note, status, created_by, updated_by)
VALUES (
    sqlc.arg('number'), sqlc.arg('party_type'), sqlc.arg('party_id'), sqlc.arg('party_name'), sqlc.arg('payment_date'),
    sqlc.arg('amount'), sqlc.arg('method'), sqlc.arg('account_code'), sqlc.arg('reference'), sqlc.narg('proof_file_id'),
    sqlc.arg('note'), 'posted', sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: InsertPaymentAllocation :exec
INSERT INTO customer_payment_allocations (payment_id, invoice_id, amount)
VALUES (sqlc.arg('payment_id'), sqlc.arg('invoice_id'), sqlc.arg('amount'));

-- name: UpdateCustomerPayment :execrows
UPDATE customer_payments
SET status = sqlc.arg('status'), cancel_reason = sqlc.arg('cancel_reason'),
    journal_entry_id = sqlc.narg('journal_entry_id'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
