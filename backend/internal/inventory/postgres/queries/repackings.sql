-- name: InsertRepacking :one
INSERT INTO repackings (business_date, note, status, return_disposition_id, created_by, created_by_name, updated_by)
VALUES (sqlc.arg('business_date'), sqlc.arg('note'), sqlc.arg('status'), sqlc.narg('return_disposition_id'),
        sqlc.arg('created_by'), sqlc.arg('created_by_name'), sqlc.arg('created_by'))
RETURNING id, created_at, updated_at, version;

-- name: GetRepacking :one
SELECT * FROM repackings WHERE id = $1;

-- name: LockRepacking :one
SELECT * FROM repackings WHERE id = $1 FOR UPDATE;

-- name: UpdateRepacking :one
UPDATE repackings SET
    number = sqlc.narg('number'), business_date = sqlc.arg('business_date'), note = sqlc.arg('note'),
    status = sqlc.arg('status'), cost_amount = sqlc.arg('cost_amount'), journal_id = sqlc.narg('journal_id'),
    started_by = sqlc.narg('started_by'), started_by_name = sqlc.arg('started_by_name'), started_at = sqlc.narg('started_at'),
    posted_by = sqlc.narg('posted_by'), posted_by_name = sqlc.arg('posted_by_name'), posted_at = sqlc.narg('posted_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING updated_at, version;

-- name: ListRepackings :many
SELECT * FROM repackings
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY business_date DESC, created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountRepackings :one
SELECT count(*) FROM repackings
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: DeleteRepackingLines :exec
DELETE FROM repacking_lines WHERE repacking_id = $1;

-- name: InsertRepackingLine :one
INSERT INTO repacking_lines (
    repacking_id, line_no, direction, product_id, location_id, stock_status, batch_id, batch_number, production_date,
    expiry_date, qty, note)
VALUES (
    sqlc.arg('repacking_id'), sqlc.arg('line_no'), sqlc.arg('direction'), sqlc.arg('product_id'), sqlc.arg('location_id'),
    sqlc.arg('stock_status'), sqlc.narg('batch_id'), sqlc.arg('batch_number'), sqlc.narg('production_date'),
    sqlc.narg('expiry_date'), sqlc.arg('qty'), sqlc.arg('note'))
RETURNING id;

-- name: ListRepackingLines :many
SELECT * FROM repacking_lines WHERE repacking_id = ANY(sqlc.arg('ids')::uuid[]) ORDER BY repacking_id, line_no;

-- name: UpdateRepackingLine :exec
UPDATE repacking_lines SET batch_id = sqlc.narg('batch_id'), cost_amount = sqlc.arg('cost_amount') WHERE id = sqlc.arg('id');
