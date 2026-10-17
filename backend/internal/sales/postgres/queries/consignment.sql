-- name: InsertConsignmentDelivery :one
INSERT INTO consignment_deliveries (
    number, delivery_type, partner_id, partner_code, partner_name, partner_agreement_id, agreement_number,
    from_location_id, to_location_id, channel_id, sub_channel_id, business_date, status, source_type, source_id,
    note, created_by, updated_by)
VALUES (
    sqlc.narg('number'), sqlc.arg('delivery_type'), sqlc.arg('partner_id'), sqlc.arg('partner_code'),
    sqlc.arg('partner_name'), sqlc.narg('partner_agreement_id'), sqlc.arg('agreement_number'),
    sqlc.arg('from_location_id'), sqlc.arg('to_location_id'), sqlc.arg('channel_id'), sqlc.arg('sub_channel_id'),
    sqlc.arg('business_date'), sqlc.arg('status'), sqlc.arg('source_type'), sqlc.narg('source_id'), sqlc.arg('note'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id, created_at, updated_at, version;

-- name: UpdateConsignmentDelivery :one
UPDATE consignment_deliveries
SET number = sqlc.narg('number'), partner_agreement_id = sqlc.narg('partner_agreement_id'),
    agreement_number = sqlc.arg('agreement_number'), from_location_id = sqlc.arg('from_location_id'),
    to_location_id = sqlc.arg('to_location_id'), channel_id = sqlc.arg('channel_id'),
    sub_channel_id = sqlc.arg('sub_channel_id'), business_date = sqlc.arg('business_date'), status = sqlc.arg('status'),
    source_id = sqlc.narg('source_id'), transfer_id = sqlc.narg('transfer_id'),
    transfer_number = sqlc.arg('transfer_number'), cost_amount = sqlc.arg('cost_amount'), note = sqlc.arg('note'),
    cancel_reason = sqlc.arg('cancel_reason'), shipped_at = sqlc.narg('shipped_at'),
    received_at = sqlc.narg('received_at'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING updated_at, version;

-- name: GetConsignmentDelivery :one
SELECT * FROM consignment_deliveries WHERE id = $1;

-- name: GetConsignmentDeliveryForUpdate :one
SELECT * FROM consignment_deliveries WHERE id = $1 FOR UPDATE;

-- name: ConsignmentDeliveryBySource :one
SELECT * FROM consignment_deliveries WHERE source_type = $1 AND source_id = $2;

-- name: ListConsignmentDeliveries :many
SELECT * FROM consignment_deliveries
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('delivery_type')::text IS NULL OR delivery_type = sqlc.narg('delivery_type')::text)
  AND (sqlc.narg('partner_id')::uuid IS NULL OR partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_code ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY business_date DESC, created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountConsignmentDeliveries :one
SELECT count(*) FROM consignment_deliveries
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('delivery_type')::text IS NULL OR delivery_type = sqlc.narg('delivery_type')::text)
  AND (sqlc.narg('partner_id')::uuid IS NULL OR partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_code ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: ListConsignmentDeliveryLines :many
SELECT * FROM consignment_delivery_lines WHERE delivery_id = $1 ORDER BY line_no;

-- name: DeleteConsignmentDeliveryLines :exec
DELETE FROM consignment_delivery_lines WHERE delivery_id = $1;

-- name: InsertConsignmentDeliveryLine :exec
INSERT INTO consignment_delivery_lines (delivery_id, line_no, product_id, batch_id, qty, cost_amount, note)
VALUES (sqlc.arg('delivery_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.narg('batch_id'), sqlc.arg('qty'),
        sqlc.arg('cost_amount'), sqlc.arg('note'));

-- name: InsertConsignmentReconciliation :one
INSERT INTO consignment_reconciliations (
    partner_id, partner_code, partner_name, partner_address, location_id, channel_id, sub_channel_id, cut_off_date,
    stock_count_id, stock_count_number, status, note, created_by, updated_by)
VALUES (
    sqlc.arg('partner_id'), sqlc.arg('partner_code'), sqlc.arg('partner_name'), sqlc.arg('partner_address'),
    sqlc.arg('location_id'), sqlc.arg('channel_id'), sqlc.arg('sub_channel_id'), sqlc.arg('cut_off_date'),
    sqlc.arg('stock_count_id'), sqlc.arg('stock_count_number'), 'draft', sqlc.arg('note'), sqlc.arg('actor_id'),
    sqlc.arg('actor_id'))
RETURNING id, created_at, updated_at, version;

-- name: UpdateConsignmentReconciliation :one
UPDATE consignment_reconciliations
SET number = sqlc.narg('number'), partner_agreement_id = sqlc.narg('partner_agreement_id'),
    agreement_number = sqlc.arg('agreement_number'), payment_term_days = sqlc.arg('payment_term_days'),
    margin_percent = sqlc.narg('margin_percent'), loss_borne_by = sqlc.arg('loss_borne_by'),
    partner_damage_borne_by = sqlc.arg('partner_damage_borne_by'),
    product_defect_borne_by = sqlc.arg('product_defect_borne_by'),
    near_expiry_borne_by = sqlc.arg('near_expiry_borne_by'), location_id = sqlc.arg('location_id'),
    channel_id = sqlc.arg('channel_id'), sub_channel_id = sqlc.arg('sub_channel_id'),
    period_start = sqlc.narg('period_start'), cut_off_date = sqlc.arg('cut_off_date'),
    stock_count_id = sqlc.arg('stock_count_id'), stock_count_number = sqlc.arg('stock_count_number'),
    partner_report_file_id = sqlc.narg('partner_report_file_id'), ba_file_id = sqlc.narg('ba_file_id'),
    status = sqlc.arg('status'), note = sqlc.arg('note'), sales_amount = sqlc.arg('sales_amount'),
    billed_variance_amount = sqlc.arg('billed_variance_amount'),
    absorbed_variance_cost = sqlc.arg('absorbed_variance_cost'), cost_amount = sqlc.arg('cost_amount'),
    invoice_id = sqlc.narg('invoice_id'), variance_invoice_id = sqlc.narg('variance_invoice_id'),
    started_at = sqlc.narg('started_at'), submitted_at = sqlc.narg('submitted_at'),
    decided_at = sqlc.narg('decided_at'), decision_note = sqlc.arg('decision_note'),
    posted_at = sqlc.narg('posted_at'), cancel_reason = sqlc.arg('cancel_reason'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING updated_at, version;

-- name: GetConsignmentReconciliation :one
SELECT * FROM consignment_reconciliations WHERE id = $1;

-- name: GetConsignmentReconciliationForUpdate :one
SELECT * FROM consignment_reconciliations WHERE id = $1 FOR UPDATE;

-- name: LastPostedCutOff :one
SELECT cut_off_date FROM consignment_reconciliations
WHERE partner_id = $1 AND status = 'posted'
ORDER BY cut_off_date DESC
LIMIT 1;

-- name: ListConsignmentReconciliations :many
SELECT * FROM consignment_reconciliations
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('partner_id')::uuid IS NULL OR partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_code ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY cut_off_date DESC, created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountConsignmentReconciliations :one
SELECT count(*) FROM consignment_reconciliations
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('partner_id')::uuid IS NULL OR partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR partner_code ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: ListConsignmentReconciliationLines :many
SELECT * FROM consignment_reconciliation_lines WHERE reconciliation_id = $1 ORDER BY line_no;

-- name: DeleteConsignmentReconciliationLines :exec
DELETE FROM consignment_reconciliation_lines WHERE reconciliation_id = $1;

-- name: InsertConsignmentReconciliationLine :one
INSERT INTO consignment_reconciliation_lines (
    reconciliation_id, line_no, product_id, opening_qty, delivered_qty, returned_qty, counted_qty, sold_qty,
    reported_qty, variance_qty, variance_cause, variance_borne_by, unit_price_amount, price_rule, price_description,
    sales_amount, variance_amount, cost_amount, variance_cost_amount, note)
VALUES (
    sqlc.arg('reconciliation_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.arg('opening_qty'),
    sqlc.arg('delivered_qty'), sqlc.arg('returned_qty'), sqlc.arg('counted_qty'), sqlc.arg('sold_qty'),
    sqlc.arg('reported_qty'), sqlc.arg('variance_qty'), sqlc.arg('variance_cause'), sqlc.arg('variance_borne_by'),
    sqlc.arg('unit_price_amount'), sqlc.arg('price_rule'), sqlc.arg('price_description'), sqlc.arg('sales_amount'),
    sqlc.arg('variance_amount'), sqlc.arg('cost_amount'), sqlc.arg('variance_cost_amount'), sqlc.arg('note'))
RETURNING id;

-- name: UpdateConsignmentReconciliationLine :exec
UPDATE consignment_reconciliation_lines
SET reported_qty = sqlc.arg('reported_qty'), variance_qty = sqlc.arg('variance_qty'),
    variance_cause = sqlc.arg('variance_cause'), variance_borne_by = sqlc.arg('variance_borne_by'),
    sales_amount = sqlc.arg('sales_amount'), variance_amount = sqlc.arg('variance_amount'),
    cost_amount = sqlc.arg('cost_amount'), variance_cost_amount = sqlc.arg('variance_cost_amount'),
    note = sqlc.arg('note')
WHERE id = sqlc.arg('id');
