-- name: InsertReturnDisposition :one
INSERT INTO return_dispositions (location_id, inbound_id, business_date, note, status, created_by, created_by_name, updated_by)
VALUES (sqlc.arg('location_id'), sqlc.narg('inbound_id'), sqlc.arg('business_date'), sqlc.arg('note'), sqlc.arg('status'),
        sqlc.arg('created_by'), sqlc.arg('created_by_name'), sqlc.arg('created_by'))
RETURNING id, created_at, updated_at, version;

-- name: GetReturnDisposition :one
SELECT * FROM return_dispositions WHERE id = $1;

-- name: LockReturnDisposition :one
SELECT * FROM return_dispositions WHERE id = $1 FOR UPDATE;

-- name: UpdateReturnDisposition :one
UPDATE return_dispositions SET
    number = sqlc.narg('number'), location_id = sqlc.arg('location_id'), inbound_id = sqlc.narg('inbound_id'),
    business_date = sqlc.arg('business_date'), note = sqlc.arg('note'), status = sqlc.arg('status'),
    disposal_value = sqlc.arg('disposal_value'), cost_amount = sqlc.arg('cost_amount'), journal_id = sqlc.narg('journal_id'),
    submitted_by = sqlc.narg('submitted_by'), submitted_by_name = sqlc.arg('submitted_by_name'),
    submitted_at = sqlc.narg('submitted_at'),
    posted_by = sqlc.narg('posted_by'), posted_by_name = sqlc.arg('posted_by_name'), posted_at = sqlc.narg('posted_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING updated_at, version;

-- name: ListReturnDispositions :many
SELECT * FROM return_dispositions
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY business_date DESC, created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountReturnDispositions :one
SELECT count(*) FROM return_dispositions
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: DeleteReturnDispositionLines :exec
DELETE FROM return_disposition_lines WHERE return_disposition_id = $1;

-- name: InsertReturnDispositionLine :one
INSERT INTO return_disposition_lines (
    return_disposition_id, line_no, product_id, batch_id, stock_status, qty, reason_id, disposition, qc_note)
VALUES (
    sqlc.arg('return_disposition_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.narg('batch_id'),
    sqlc.arg('stock_status'), sqlc.arg('qty'), sqlc.narg('reason_id'), sqlc.arg('disposition'), sqlc.arg('qc_note'))
RETURNING id;

-- name: ListReturnDispositionLines :many
SELECT * FROM return_disposition_lines WHERE return_disposition_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY return_disposition_id, line_no;

-- name: UpdateReturnDispositionLine :exec
UPDATE return_disposition_lines SET estimated_cost = sqlc.arg('estimated_cost'), cost_amount = sqlc.arg('cost_amount'),
    target_type = sqlc.arg('target_type'), target_id = sqlc.narg('target_id')
WHERE id = sqlc.arg('id');
