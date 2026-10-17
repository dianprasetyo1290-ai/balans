-- name: InsertInternalUsage :one
INSERT INTO internal_usages (
    purpose, division_id, campaign_id, source_location_id, event_location_id, event_end_date, settle_due_date,
    business_date, note, status, return_disposition_id, requested_by, requested_by_name, created_by, updated_by)
VALUES (
    sqlc.arg('purpose'), sqlc.arg('division_id'), sqlc.narg('campaign_id'), sqlc.arg('source_location_id'),
    sqlc.narg('event_location_id'), sqlc.narg('event_end_date'), sqlc.narg('settle_due_date'), sqlc.arg('business_date'),
    sqlc.arg('note'), sqlc.arg('status'), sqlc.narg('return_disposition_id'), sqlc.arg('requested_by'),
    sqlc.arg('requested_by_name'), sqlc.arg('requested_by'), sqlc.arg('requested_by'))
RETURNING id, created_at, updated_at, version;

-- name: GetInternalUsage :one
SELECT * FROM internal_usages WHERE id = $1;

-- name: LockInternalUsage :one
SELECT * FROM internal_usages WHERE id = $1 FOR UPDATE;

-- name: UpdateInternalUsage :one
UPDATE internal_usages SET
    number = sqlc.narg('number'), purpose = sqlc.arg('purpose'), division_id = sqlc.arg('division_id'),
    campaign_id = sqlc.narg('campaign_id'), source_location_id = sqlc.arg('source_location_id'),
    event_location_id = sqlc.narg('event_location_id'), event_end_date = sqlc.narg('event_end_date'),
    settle_due_date = sqlc.narg('settle_due_date'), business_date = sqlc.arg('business_date'), note = sqlc.arg('note'),
    status = sqlc.arg('status'), estimated_cost = sqlc.arg('estimated_cost'), cost_amount = sqlc.arg('cost_amount'),
    journal_id = sqlc.narg('journal_id'),
    released_by = sqlc.narg('released_by'), released_by_name = sqlc.arg('released_by_name'), released_at = sqlc.narg('released_at'),
    handed_over_by = sqlc.narg('handed_over_by'), handed_over_by_name = sqlc.arg('handed_over_by_name'),
    handed_over_at = sqlc.narg('handed_over_at'),
    settled_by = sqlc.narg('settled_by'), settled_by_name = sqlc.arg('settled_by_name'), settled_at = sqlc.narg('settled_at'),
    posted_by = sqlc.narg('posted_by'), posted_by_name = sqlc.arg('posted_by_name'), posted_at = sqlc.narg('posted_at'),
    reminded_on = sqlc.narg('reminded_on'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING updated_at, version;

-- name: ListInternalUsages :many
SELECT * FROM internal_usages
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('purpose')::text IS NULL OR purpose = sqlc.narg('purpose')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY business_date DESC, created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountInternalUsages :one
SELECT count(*) FROM internal_usages
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('purpose')::text IS NULL OR purpose = sqlc.narg('purpose')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: SettlementDueUsages :many
SELECT id FROM internal_usages
WHERE status = 'awaiting_settlement' AND settled_at IS NULL AND settle_due_date <= sqlc.arg('today')::date
  AND (reminded_on IS NULL OR reminded_on < sqlc.arg('today')::date)
ORDER BY settle_due_date, id
LIMIT 200;

-- name: DeleteInternalUsageLines :exec
DELETE FROM internal_usage_lines WHERE internal_usage_id = $1;

-- name: InsertInternalUsageLine :one
INSERT INTO internal_usage_lines (internal_usage_id, line_no, product_id, qty, note)
VALUES (sqlc.arg('internal_usage_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.arg('qty'), sqlc.arg('note'))
RETURNING id;

-- name: ListInternalUsageLines :many
SELECT * FROM internal_usage_lines WHERE internal_usage_id = ANY(sqlc.arg('ids')::uuid[]) ORDER BY internal_usage_id, line_no;

-- name: UpdateInternalUsageLine :exec
UPDATE internal_usage_lines SET returned_qty = sqlc.arg('returned_qty'), consumed_qty = sqlc.arg('consumed_qty'),
    cost_amount = sqlc.arg('cost_amount')
WHERE id = sqlc.arg('id');
