-- name: TrialBalance :many
-- Neraca Saldo (ACC-18): per akun yang punya mutasi s.d. date_to, saldo awal (sebelum date_from, debit − kredit)
-- dan mutasi rentang dari jurnal yang sudah diposting, dengan filter dimensi opsional.
SELECT a.id AS account_id, a.code AS account_code, a.name AS account_name, a.account_group, a.normal_balance,
       coalesce(sum(l.debit_amount - l.credit_amount) FILTER (WHERE e.business_date < sqlc.arg('date_from')::date), 0)::numeric AS opening_net,
       coalesce(sum(l.debit_amount) FILTER (WHERE e.business_date >= sqlc.arg('date_from')::date), 0)::numeric AS period_debit,
       coalesce(sum(l.credit_amount) FILTER (WHERE e.business_date >= sqlc.arg('date_from')::date), 0)::numeric AS period_credit
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.status = 'posted' AND e.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR l.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR l.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR l.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR l.campaign_id = sqlc.narg('campaign_id'))
GROUP BY a.id, a.code, a.name, a.account_group, a.normal_balance
ORDER BY a.code;

-- name: AccountNetBalances :many
-- Saldo debit − kredit per kode akun s.d. tanggal (rekonsiliasi cut-over).
SELECT a.code, coalesce(sum(l.debit_amount - l.credit_amount), 0)::numeric AS net_amount
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.status = 'posted' AND e.business_date <= sqlc.arg('as_of')::date
GROUP BY a.code;

-- name: LedgerExportLines :many
-- Buku Besar detail semua akun (atau satu akun) untuk ekspor Excel, urut akun lalu tanggal.
SELECT a.code AS account_code, a.name AS account_name, e.number, e.business_date, e.description, e.source_type,
       e.source_label, l.debit_amount, l.credit_amount, l.memo, l.channel_id, l.sub_channel_id, l.division_id, l.campaign_id
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.status = 'posted'
  AND e.business_date >= sqlc.arg('date_from')::date AND e.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('account_id')::uuid IS NULL OR l.account_id = sqlc.narg('account_id'))
  AND (sqlc.narg('channel_id')::uuid IS NULL OR l.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR l.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR l.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR l.campaign_id = sqlc.narg('campaign_id'))
ORDER BY a.code, e.business_date, e.number, l.line_no
LIMIT sqlc.arg('row_limit');
