-- name: InsertStockCount :one
INSERT INTO stock_counts (count_type, location_id, schedule_id, business_date, status, assigned_to,
                          assigned_to_name, note, created_by, updated_by)
VALUES (sqlc.arg('count_type'), sqlc.arg('location_id'), sqlc.narg('schedule_id'),
        sqlc.arg('business_date'), sqlc.arg('status'), sqlc.narg('assigned_to'), sqlc.arg('assigned_to_name'),
        sqlc.arg('note'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (schedule_id, location_id, business_date) WHERE schedule_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetStockCount :one
SELECT * FROM stock_counts WHERE id = $1;

-- name: GetStockCountForUpdate :one
SELECT * FROM stock_counts WHERE id = $1 FOR UPDATE;

-- name: UpdateStockCount :one
UPDATE stock_counts
SET number = sqlc.narg('number'), status = sqlc.arg('status'), note = sqlc.arg('note'), photo_file_ids = sqlc.arg('photo_file_ids')::uuid[],
    snapshot_at = sqlc.narg('snapshot_at'), started_by = sqlc.narg('started_by'), started_by_name = sqlc.arg('started_by_name'),
    submitted_by = sqlc.narg('submitted_by'), submitted_by_name = sqlc.arg('submitted_by_name'),
    submitted_at = sqlc.narg('submitted_at'), variance_amount = sqlc.arg('variance_amount'),
    net_variance = sqlc.arg('net_variance'), decided_at = sqlc.narg('decided_at'), decision_note = sqlc.arg('decision_note'),
    posted_at = sqlc.narg('posted_at'), journal_entry_id = sqlc.narg('journal_entry_id'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: InsertStockCountLine :one
INSERT INTO stock_count_lines (stock_count_id, line_no, product_id, batch_id, system_qty, unit_cost, counted_qty,
                               variance_qty, variance_amount, is_extra, photo_file_id, note, counted_by, counted_at)
VALUES (sqlc.arg('stock_count_id'), sqlc.arg('line_no'), sqlc.arg('product_id'), sqlc.narg('batch_id'),
        sqlc.arg('system_qty'), sqlc.arg('unit_cost'), sqlc.narg('counted_qty'), sqlc.arg('variance_qty'),
        sqlc.arg('variance_amount'), sqlc.arg('is_extra'), sqlc.narg('photo_file_id'), sqlc.arg('note'),
        sqlc.narg('counted_by'), sqlc.narg('counted_at'))
RETURNING id;

-- name: UpdateStockCountLine :exec
UPDATE stock_count_lines
SET counted_qty = sqlc.narg('counted_qty'), variance_qty = sqlc.arg('variance_qty'),
    variance_amount = sqlc.arg('variance_amount'), photo_file_id = sqlc.narg('photo_file_id'), note = sqlc.arg('note'),
    counted_by = sqlc.narg('counted_by'), counted_at = sqlc.narg('counted_at')
WHERE id = sqlc.arg('id');

-- name: DeleteStockCountLines :exec
DELETE FROM stock_count_lines WHERE stock_count_id = $1;

-- name: ListStockCountLines :many
SELECT * FROM stock_count_lines WHERE stock_count_id = $1 ORDER BY line_no;

-- name: ListStockCounts :many
SELECT c.*,
       (SELECT count(*) FROM stock_count_lines l WHERE l.stock_count_id = c.id)::bigint AS line_count,
       (SELECT count(*) FROM stock_count_lines l WHERE l.stock_count_id = c.id AND l.counted_qty IS NOT NULL)::bigint AS counted_count
FROM stock_counts c
WHERE (cardinality(sqlc.arg('statuses')::text[]) = 0 OR c.status = ANY (sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('count_type')::text IS NULL OR c.count_type = sqlc.narg('count_type')::text)
  AND (sqlc.narg('location_id')::uuid IS NULL OR c.location_id = sqlc.narg('location_id')::uuid)
  AND (NOT sqlc.arg('scoped')::boolean OR c.location_id = ANY (sqlc.arg('allowed_location_ids')::uuid[]))
  AND (sqlc.narg('search')::text IS NULL OR c.number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY c.business_date DESC, c.created_at DESC, c.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountStockCounts :one
SELECT count(*) FROM stock_counts c
WHERE (cardinality(sqlc.arg('statuses')::text[]) = 0 OR c.status = ANY (sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('count_type')::text IS NULL OR c.count_type = sqlc.narg('count_type')::text)
  AND (sqlc.narg('location_id')::uuid IS NULL OR c.location_id = sqlc.narg('location_id')::uuid)
  AND (NOT sqlc.arg('scoped')::boolean OR c.location_id = ANY (sqlc.arg('allowed_location_ids')::uuid[]))
  AND (sqlc.narg('search')::text IS NULL OR c.number ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: InsertCountSchedule :one
INSERT INTO stock_count_schedules (name, count_type, frequency, day_of_week, day_of_month, month_of_year, warehouse_id,
                                   location_ids, assignee_user_id, assignee_name, next_run_date, is_active,
                                   created_by, updated_by)
VALUES (sqlc.arg('name'), sqlc.arg('count_type'), sqlc.arg('frequency'), sqlc.narg('day_of_week'),
        sqlc.narg('day_of_month'), sqlc.narg('month_of_year'), sqlc.narg('warehouse_id'),
        sqlc.arg('location_ids')::uuid[], sqlc.narg('assignee_user_id'), sqlc.arg('assignee_name'),
        sqlc.arg('next_run_date'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateCountSchedule :one
UPDATE stock_count_schedules
SET name = sqlc.arg('name'), count_type = sqlc.arg('count_type'), frequency = sqlc.arg('frequency'),
    day_of_week = sqlc.narg('day_of_week'), day_of_month = sqlc.narg('day_of_month'),
    month_of_year = sqlc.narg('month_of_year'), warehouse_id = sqlc.narg('warehouse_id'),
    location_ids = sqlc.arg('location_ids')::uuid[], assignee_user_id = sqlc.narg('assignee_user_id'),
    assignee_name = sqlc.arg('assignee_name'), next_run_date = sqlc.arg('next_run_date'),
    last_run_date = sqlc.narg('last_run_date'), is_active = sqlc.arg('is_active'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: GetCountSchedule :one
SELECT * FROM stock_count_schedules WHERE id = $1;

-- name: ListCountSchedules :many
SELECT * FROM stock_count_schedules ORDER BY is_active DESC, next_run_date, name;

-- name: DueCountSchedules :many
SELECT * FROM stock_count_schedules
WHERE is_active AND next_run_date <= sqlc.arg('today')
ORDER BY next_run_date, id
FOR UPDATE SKIP LOCKED;
