-- name: InsertDisposal :one
INSERT INTO disposals (
    reason, disposal_date, method, witnesses, photo_file_ids, note, status, return_disposition_id, created_by,
    created_by_name, updated_by)
VALUES (
    sqlc.arg('reason'), sqlc.arg('disposal_date'), sqlc.arg('method'), sqlc.arg('witnesses'), sqlc.arg('photo_file_ids'),
    sqlc.arg('note'), sqlc.arg('status'), sqlc.narg('return_disposition_id'), sqlc.arg('created_by'),
    sqlc.arg('created_by_name'), sqlc.arg('created_by'))
RETURNING id, created_at, updated_at, version;

-- name: GetDisposal :one
SELECT * FROM disposals WHERE id = $1;

-- name: LockDisposal :one
SELECT * FROM disposals WHERE id = $1 FOR UPDATE;

-- name: UpdateDisposal :one
UPDATE disposals SET
    number = sqlc.narg('number'), reason = sqlc.arg('reason'), disposal_date = sqlc.arg('disposal_date'),
    method = sqlc.arg('method'), witnesses = sqlc.arg('witnesses'), photo_file_ids = sqlc.arg('photo_file_ids'),
    note = sqlc.arg('note'), status = sqlc.arg('status'), estimated_cost = sqlc.arg('estimated_cost'),
    cost_amount = sqlc.arg('cost_amount'), posting_rule_code = sqlc.arg('posting_rule_code'),
    journal_id = sqlc.narg('journal_id'),
    submitted_by = sqlc.narg('submitted_by'), submitted_by_name = sqlc.arg('submitted_by_name'),
    submitted_at = sqlc.narg('submitted_at'),
    posted_by = sqlc.narg('posted_by'), posted_by_name = sqlc.arg('posted_by_name'), posted_at = sqlc.narg('posted_at'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING updated_at, version;

-- name: ListDisposals :many
SELECT * FROM disposals
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('reason')::text IS NULL OR reason = sqlc.narg('reason')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY disposal_date DESC, created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountDisposals :one
SELECT count(*) FROM disposals
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('reason')::text IS NULL OR reason = sqlc.narg('reason')::text)
  AND (sqlc.narg('q')::text IS NULL OR number ILIKE '%' || sqlc.narg('q')::text || '%' OR note ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: DeleteDisposalLines :exec
DELETE FROM disposal_lines WHERE disposal_id = $1;

-- name: InsertDisposalLine :one
INSERT INTO disposal_lines (disposal_id, line_no, product_id, batch_id, location_id, stock_status, qty, note)
VALUES (sqlc.arg('disposal_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.narg('batch_id'),
        sqlc.arg('location_id'), sqlc.arg('stock_status'), sqlc.arg('qty'), sqlc.arg('note'))
RETURNING id;

-- name: ListDisposalLines :many
SELECT * FROM disposal_lines WHERE disposal_id = ANY(sqlc.arg('ids')::uuid[]) ORDER BY disposal_id, line_no;

-- name: UpdateDisposalLine :exec
UPDATE disposal_lines SET cost_amount = sqlc.arg('cost_amount') WHERE id = sqlc.arg('id');
