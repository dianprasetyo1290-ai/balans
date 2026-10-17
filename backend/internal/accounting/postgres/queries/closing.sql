-- name: GetPeriodByID :one
SELECT * FROM fiscal_periods WHERE id = $1;

-- name: GetPeriodByIDForUpdate :one
SELECT * FROM fiscal_periods WHERE id = $1 FOR UPDATE;

-- name: AdvisoryLockPeriod :exec
-- Kunci proses tunggal per periode (tutup buku, penyusutan; Technical Guideline 7.2).
SELECT pg_advisory_xact_lock(hashtext(sqlc.arg('process')::text), sqlc.arg('period_index')::integer);

-- name: SetPeriodStatus :execrows
UPDATE fiscal_periods
SET status = sqlc.arg('status'), closed_at = sqlc.narg('closed_at'), closed_by = sqlc.narg('closed_by'),
    reopened_count = sqlc.arg('reopened_count'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: OpenPeriodBeforeExists :one
-- Periode sebelumnya yang belum ditutup (tutup buku berurutan).
SELECT EXISTS (
    SELECT 1 FROM fiscal_periods WHERE status <> 'closed' AND (year * 12 + month - 1) < sqlc.arg('period_index')::integer
)::boolean;

-- name: EnsureChecklistStep :exec
INSERT INTO period_close_checklists (fiscal_period_id, step_code, status, completed_at, completed_by, created_by, updated_by)
VALUES (sqlc.arg('fiscal_period_id'), sqlc.arg('step_code'), sqlc.arg('status'), sqlc.narg('completed_at'),
        sqlc.narg('completed_by'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (fiscal_period_id, step_code) DO NOTHING;

-- name: ListChecklist :many
SELECT * FROM period_close_checklists WHERE fiscal_period_id = $1;

-- name: GetChecklistStepForUpdate :one
SELECT * FROM period_close_checklists WHERE fiscal_period_id = $1 AND step_code = $2 FOR UPDATE;

-- name: UpdateChecklistStep :execrows
UPDATE period_close_checklists
SET status = sqlc.arg('status'), pic_user_id = sqlc.narg('pic_user_id'), completed_by = sqlc.narg('completed_by'),
    completed_at = sqlc.narg('completed_at'), note = sqlc.arg('note'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: InsertReopenRequest :one
INSERT INTO fiscal_period_reopens (fiscal_period_id, reason, requested_by, created_by, updated_by)
VALUES (sqlc.arg('fiscal_period_id'), sqlc.arg('reason'), sqlc.arg('actor_id'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: GetReopenRequestForUpdate :one
SELECT * FROM fiscal_period_reopens WHERE id = $1 FOR UPDATE;

-- name: DecideReopenRequest :execrows
UPDATE fiscal_period_reopens
SET status = sqlc.arg('status'), decided_by = sqlc.arg('decided_by'), decided_at = sqlc.arg('decided_at'),
    decision_note = sqlc.arg('decision_note'), updated_at = now(), updated_by = sqlc.arg('decided_by'), version = version + 1
WHERE id = sqlc.arg('id') AND status = 'pending_approval';

-- name: ListReopenRequests :many
SELECT * FROM fiscal_period_reopens WHERE fiscal_period_id = $1 ORDER BY requested_at DESC;

-- name: DraftJournalCount :one
-- Jurnal Umum draft bertanggal di rentang (langkah 1 checklist).
SELECT count(*) FROM journal_entries
WHERE status = 'draft' AND deleted_at IS NULL
  AND business_date >= sqlc.arg('date_from')::date AND business_date <= sqlc.arg('date_to')::date;
