-- name: InsertEntry :exec
INSERT INTO journal_entries (
    id, business_date, entry_type, source_type, source_id, source_label, posting_rule_code, description,
    reversal_of_id, status, total_amount, created_by, updated_by)
VALUES (
    sqlc.arg('id'), sqlc.arg('business_date'), sqlc.arg('entry_type'), sqlc.arg('source_type'), sqlc.arg('source_id'),
    sqlc.arg('source_label'), sqlc.narg('posting_rule_code'), sqlc.arg('description'), sqlc.narg('reversal_of_id'),
    'draft', sqlc.arg('total_amount'), sqlc.arg('actor_id'), sqlc.arg('actor_id'));

-- name: InsertLine :exec
INSERT INTO journal_lines (
    journal_entry_id, line_no, account_id, debit_amount, credit_amount, channel_id, sub_channel_id, division_id,
    campaign_id, party_type, party_id, amount_key, memo, created_by)
VALUES (
    sqlc.arg('journal_entry_id'), sqlc.arg('line_no'), sqlc.arg('account_id'), sqlc.arg('debit_amount'),
    sqlc.arg('credit_amount'), sqlc.narg('channel_id'), sqlc.narg('sub_channel_id'), sqlc.narg('division_id'),
    sqlc.narg('campaign_id'), sqlc.arg('party_type'), sqlc.narg('party_id'), sqlc.arg('amount_key'), sqlc.arg('memo'),
    sqlc.arg('actor_id'));

-- name: DeleteDraftLines :exec
DELETE FROM journal_lines WHERE journal_entry_id = $1;

-- name: UpdateDraftEntry :execrows
UPDATE journal_entries
SET business_date = sqlc.arg('business_date'), description = sqlc.arg('description'),
    total_amount = sqlc.arg('total_amount'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND status = 'draft' AND deleted_at IS NULL;

-- name: PostEntry :execrows
UPDATE journal_entries
SET status = 'posted', number = sqlc.arg('number')::text, fiscal_period_id = sqlc.arg('fiscal_period_id')::uuid,
    total_amount = sqlc.arg('total_amount'), posted_at = sqlc.arg('posted_at')::timestamptz, posted_by = sqlc.arg('actor_id')::uuid,
    updated_at = sqlc.arg('posted_at'), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND status = 'draft' AND deleted_at IS NULL;

-- name: SoftDeleteDraftEntry :execrows
UPDATE journal_entries
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND status = 'draft' AND deleted_at IS NULL;

-- name: GetEntry :one
SELECT sqlc.embed(e), coalesce(o.number, '')::text AS reversal_of_number,
       r.id AS reversed_by_id, coalesce(r.number, '')::text AS reversed_by_number
FROM journal_entries e
LEFT JOIN journal_entries o ON o.id = e.reversal_of_id
LEFT JOIN journal_entries r ON r.reversal_of_id = e.id AND r.deleted_at IS NULL
WHERE e.id = sqlc.arg('id') AND e.deleted_at IS NULL;

-- name: LockEntry :one
SELECT version FROM journal_entries WHERE id = sqlc.arg('id') AND deleted_at IS NULL FOR UPDATE;

-- name: GetEntryIDBySource :one
SELECT id FROM journal_entries
WHERE source_type = sqlc.arg('source_type') AND source_id = sqlc.arg('source_id')
  AND posting_rule_code IS NOT DISTINCT FROM sqlc.narg('posting_rule_code') AND deleted_at IS NULL;

-- name: ListLinesByEntries :many
SELECT l.*, a.code AS account_code, a.name AS account_name
FROM journal_lines l
JOIN accounts a ON a.id = l.account_id
WHERE l.journal_entry_id = ANY(sqlc.arg('ids')::uuid[])
ORDER BY l.journal_entry_id, l.line_no;

-- name: ListEntries :many
SELECT sqlc.embed(e), coalesce(o.number, '')::text AS reversal_of_number,
       r.id AS reversed_by_id, coalesce(r.number, '')::text AS reversed_by_number
FROM journal_entries e
LEFT JOIN journal_entries o ON o.id = e.reversal_of_id
LEFT JOIN journal_entries r ON r.reversal_of_id = e.id AND r.deleted_at IS NULL
WHERE e.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR e.status = sqlc.narg('status'))
  AND (sqlc.narg('entry_type')::text IS NULL OR e.entry_type = sqlc.narg('entry_type'))
  AND (sqlc.narg('date_from')::date IS NULL OR e.business_date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::date IS NULL OR e.business_date <= sqlc.narg('date_to'))
  AND (sqlc.narg('q')::text IS NULL OR e.number ILIKE '%' || sqlc.narg('q') || '%'
       OR e.description ILIKE '%' || sqlc.narg('q') || '%' OR e.source_label ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY e.business_date DESC, e.number DESC NULLS FIRST, e.created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountEntries :one
SELECT count(*) FROM journal_entries e
WHERE e.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR e.status = sqlc.narg('status'))
  AND (sqlc.narg('entry_type')::text IS NULL OR e.entry_type = sqlc.narg('entry_type'))
  AND (sqlc.narg('date_from')::date IS NULL OR e.business_date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::date IS NULL OR e.business_date <= sqlc.narg('date_to'))
  AND (sqlc.narg('q')::text IS NULL OR e.number ILIKE '%' || sqlc.narg('q') || '%'
       OR e.description ILIKE '%' || sqlc.narg('q') || '%' OR e.source_label ILIKE '%' || sqlc.narg('q') || '%');

-- name: LedgerSummary :one
-- Saldo awal (sebelum date_from) dan mutasi periode satu akun dari jurnal yang sudah diposting.
SELECT
    coalesce(sum(l.debit_amount) FILTER (WHERE e.business_date < sqlc.arg('date_from')::date), 0)::numeric AS opening_debit,
    coalesce(sum(l.credit_amount) FILTER (WHERE e.business_date < sqlc.arg('date_from')::date), 0)::numeric AS opening_credit,
    coalesce(sum(l.debit_amount) FILTER (WHERE e.business_date >= sqlc.arg('date_from')::date), 0)::numeric AS period_debit,
    coalesce(sum(l.credit_amount) FILTER (WHERE e.business_date >= sqlc.arg('date_from')::date), 0)::numeric AS period_credit,
    count(*) FILTER (WHERE e.business_date >= sqlc.arg('date_from')::date) AS line_count
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE l.account_id = sqlc.arg('account_id') AND e.status = 'posted' AND e.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR l.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR l.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR l.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR l.campaign_id = sqlc.narg('campaign_id'));

-- name: LedgerLines :many
-- Mutasi per baris dengan kumulatif debit − kredit sejak date_from (saldo berjalan dihitung di Go).
SELECT e.id AS entry_id, e.number, e.business_date, e.description, e.entry_type, e.source_type, e.source_label,
       l.line_no, l.debit_amount, l.credit_amount, l.memo, l.channel_id, l.sub_channel_id, l.division_id, l.campaign_id,
       sum(l.debit_amount - l.credit_amount) OVER (
           ORDER BY e.business_date, e.number, l.line_no ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)::numeric AS cumulative_net
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE l.account_id = sqlc.arg('account_id') AND e.status = 'posted'
  AND e.business_date >= sqlc.arg('date_from')::date AND e.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR l.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR l.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR l.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR l.campaign_id = sqlc.narg('campaign_id'))
ORDER BY e.business_date, e.number, l.line_no
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');
