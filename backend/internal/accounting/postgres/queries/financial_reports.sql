-- Laporan dari read model rm_gl_daily (TG 5.17): tidak menjumlah ulang journal_lines saat laporan dibuka.

-- name: ReportMovements :many
-- Mutasi debit − kredit per akun sebelum date_from dan selama date_from..date_to, opsional per dimensi.
SELECT g.account_id,
       coalesce(sum(g.debit_amount - g.credit_amount) FILTER (WHERE g.business_date < sqlc.arg('date_from')::date), 0)::numeric AS before_net,
       coalesce(sum(g.debit_amount - g.credit_amount) FILTER (WHERE g.business_date >= sqlc.arg('date_from')::date), 0)::numeric AS during_net
FROM rm_gl_daily g
WHERE g.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR g.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR g.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR g.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR g.campaign_id = sqlc.narg('campaign_id'))
GROUP BY g.account_id;

-- name: ReportDimensionMovements :many
-- Mutasi akun laba rugi (kelompok 4–7) per nilai satu dimensi; UUID nol = tanpa dimensi.
SELECT g.account_id,
       (CASE sqlc.arg('dimension')::text
            WHEN 'channel' THEN g.channel_id
            WHEN 'sub_channel' THEN g.sub_channel_id
            WHEN 'division' THEN g.division_id
            ELSE g.campaign_id END)::uuid AS dimension_id,
       sum(g.debit_amount - g.credit_amount)::numeric AS net
FROM rm_gl_daily g
JOIN accounts a ON a.id = g.account_id
WHERE a.account_group >= 4
  AND g.business_date >= sqlc.arg('date_from')::date AND g.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR g.channel_id = sqlc.narg('channel_id'))
GROUP BY 1, 2;

-- name: ReportMonthlyAccounts :many
-- Mutasi bulanan akun tertentu (rekap pajak).
SELECT date_trunc('month', g.business_date)::date AS month, a.code,
       sum(g.debit_amount)::numeric AS debit, sum(g.credit_amount)::numeric AS credit
FROM rm_gl_daily g
JOIN accounts a ON a.id = g.account_id
WHERE a.code = ANY(sqlc.arg('codes')::text[])
  AND g.business_date >= sqlc.arg('date_from')::date AND g.business_date <= sqlc.arg('date_to')::date
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportMonthlyGroups :many
-- Mutasi bulanan per kelompok akun laba rugi (tren dashboard).
SELECT date_trunc('month', g.business_date)::date AS month, a.account_group,
       sum(g.debit_amount - g.credit_amount)::numeric AS net
FROM rm_gl_daily g
JOIN accounts a ON a.id = g.account_id
WHERE a.account_group >= 4
  AND g.business_date >= sqlc.arg('date_from')::date AND g.business_date <= sqlc.arg('date_to')::date
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportTaxLines :many
-- Rincian jurnal akun pajak satu periode (bukan penjumlahan; dibatasi jumlah baris).
SELECT e.id AS entry_id, e.business_date, coalesce(e.number, '')::text AS number, e.source_type, e.source_label,
       e.description, a.code AS account_code, a.name AS account_name, l.debit_amount, l.credit_amount
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.status = 'posted' AND a.code = ANY(sqlc.arg('codes')::text[])
  AND e.business_date >= sqlc.arg('date_from')::date AND e.business_date <= sqlc.arg('date_to')::date
ORDER BY e.business_date, e.number, l.line_no
LIMIT sqlc.arg('row_limit');

-- name: UpsertReadModelRefresh :exec
INSERT INTO read_model_refreshes (name, refreshed_at, duration_ms)
VALUES (sqlc.arg('name'), sqlc.arg('refreshed_at'), sqlc.arg('duration_ms'))
ON CONFLICT (name) DO UPDATE SET refreshed_at = EXCLUDED.refreshed_at, duration_ms = EXCLUDED.duration_ms;

-- name: ListReadModelRefreshes :many
SELECT * FROM read_model_refreshes ORDER BY name;

-- name: TryReadModelLock :one
SELECT pg_try_advisory_xact_lock(hashtext('accounting.refresh_read_models'))::boolean AS locked;
