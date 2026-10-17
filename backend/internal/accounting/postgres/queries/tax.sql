-- name: ListTaxRates :many
SELECT * FROM withholding_tax_rates
WHERE (sqlc.narg('tax_type')::text IS NULL OR tax_type = sqlc.narg('tax_type'))
ORDER BY tax_type, object_code, valid_from DESC;

-- name: GetTaxRateForUpdate :one
SELECT * FROM withholding_tax_rates WHERE id = $1 FOR UPDATE;

-- name: LatestTaxRateForUpdate :one
SELECT * FROM withholding_tax_rates
WHERE tax_type = sqlc.arg('tax_type') AND object_code = sqlc.arg('object_code')
ORDER BY valid_from DESC
LIMIT 1
FOR UPDATE;

-- name: EndTaxRate :execrows
UPDATE withholding_tax_rates
SET valid_to = sqlc.arg('valid_to'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id');

-- name: InsertTaxRate :one
INSERT INTO withholding_tax_rates (
    tax_type, object_code, name, rate_percent, brackets, base_percent, no_tax_id_surcharge_percent, is_default,
    valid_from, valid_to, note, created_by, updated_by)
VALUES (
    sqlc.arg('tax_type'), sqlc.arg('object_code'), sqlc.arg('name'), sqlc.arg('rate_percent'), sqlc.arg('brackets'),
    sqlc.arg('base_percent'), sqlc.arg('no_tax_id_surcharge_percent'), sqlc.arg('is_default'), sqlc.arg('valid_from'),
    sqlc.narg('valid_to'), sqlc.arg('note'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: InsertWithholding :one
INSERT INTO withholdings (
    source_type, source_id, source_label, business_date, tax_year, tax_month, tax_type, object_code, object_name, rate_id,
    taxpayer_type, taxpayer_id, taxpayer_name, taxpayer_tax_id_enc, has_tax_id, gross_amount, taxable_base,
    rate_percent, tax_amount, created_by, updated_by)
VALUES (
    sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('source_label'), sqlc.arg('business_date'),
    sqlc.arg('tax_year'), sqlc.arg('tax_month'), sqlc.arg('tax_type'), sqlc.arg('object_code'), sqlc.arg('object_name'),
    sqlc.arg('rate_id'), sqlc.arg('taxpayer_type'), sqlc.narg('taxpayer_id'), sqlc.arg('taxpayer_name'),
    sqlc.arg('taxpayer_tax_id_enc'), sqlc.arg('has_tax_id'), sqlc.arg('gross_amount'), sqlc.arg('taxable_base'),
    sqlc.arg('rate_percent'), sqlc.arg('tax_amount'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: ListWithholdings :many
SELECT w.*, coalesce(d.number, '')::text AS deposit_number, count(*) OVER () AS total_count
FROM withholdings w
LEFT JOIN tax_deposits d ON d.id = w.deposit_id
WHERE w.tax_year = sqlc.arg('tax_year') AND w.tax_month = sqlc.arg('tax_month')
  AND (sqlc.narg('tax_type')::text IS NULL OR w.tax_type = sqlc.narg('tax_type'))
  AND (sqlc.arg('include_cancelled')::boolean OR w.status <> 'cancelled')
ORDER BY w.tax_type, w.business_date, w.source_label
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: WithholdingsBySource :many
SELECT w.*, coalesce(d.number, '')::text AS deposit_number, 0::bigint AS total_count
FROM withholdings w
LEFT JOIN tax_deposits d ON d.id = w.deposit_id
WHERE w.source_type = $1 AND w.source_id = $2
ORDER BY w.created_at;

-- name: GetWithholdingForUpdate :one
SELECT w.*, ''::text AS deposit_number, 0::bigint AS total_count
FROM withholdings w WHERE w.id = $1 FOR UPDATE;

-- name: SaveWithholding :execrows
UPDATE withholdings
SET status = sqlc.arg('status'), deposit_id = sqlc.narg('deposit_id'), slip_number = sqlc.arg('slip_number'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: LockOpenWithholdings :many
SELECT id, tax_amount FROM withholdings
WHERE tax_type = sqlc.arg('tax_type') AND tax_year = sqlc.arg('tax_year') AND tax_month = sqlc.arg('tax_month')
  AND status = 'recorded'
ORDER BY id
FOR UPDATE;

-- name: MarkWithholdingsDeposited :execrows
UPDATE withholdings
SET status = 'deposited', deposit_id = sqlc.arg('deposit_id'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = ANY(sqlc.arg('ids')::uuid[]) AND status = 'recorded';

-- name: WithholdingSummary :many
SELECT tax_type, tax_year, tax_month,
       count(*) FILTER (WHERE status <> 'cancelled')::bigint AS slip_count,
       coalesce(sum(tax_amount) FILTER (WHERE status = 'recorded'), 0)::numeric AS open_amount,
       coalesce(sum(tax_amount) FILTER (WHERE status = 'deposited'), 0)::numeric AS deposited_amount
FROM withholdings
WHERE tax_year = sqlc.arg('tax_year')
GROUP BY tax_type, tax_year, tax_month
ORDER BY tax_month, tax_type;

-- name: InsertTaxDeposit :one
INSERT INTO tax_deposits (
    id, number, tax_type, tax_year, tax_month, bank_account_id, deposit_date, amount, billing_code, ntpn,
    journal_entry_id, created_by)
VALUES (
    sqlc.arg('id'), sqlc.arg('number'), sqlc.arg('tax_type'), sqlc.arg('tax_year'), sqlc.arg('tax_month'), sqlc.arg('bank_account_id'),
    sqlc.arg('deposit_date'), sqlc.arg('amount'), sqlc.arg('billing_code'), sqlc.arg('ntpn'),
    sqlc.arg('journal_entry_id'), sqlc.arg('actor_id'))
RETURNING id, created_at;

-- name: TaxDepositByPeriod :one
SELECT id FROM tax_deposits
WHERE tax_type = sqlc.arg('tax_type') AND tax_year = sqlc.arg('tax_year') AND tax_month = sqlc.arg('tax_month');

-- name: ListTaxDeposits :many
SELECT d.*, b.name AS bank_name, j.number AS journal_number
FROM tax_deposits d
JOIN bank_accounts b ON b.id = d.bank_account_id
JOIN journal_entries j ON j.id = d.journal_entry_id
WHERE d.tax_year = sqlc.arg('tax_year')
ORDER BY d.tax_month DESC, d.tax_type;

-- name: ListFinalTaxSettings :many
SELECT * FROM final_tax_settings ORDER BY valid_from DESC;

-- name: InsertFinalTaxSetting :one
INSERT INTO final_tax_settings (valid_from, is_enabled, opening_year, opening_turnover, note, created_by)
VALUES (sqlc.arg('valid_from'), sqlc.arg('is_enabled'), sqlc.narg('opening_year'), sqlc.arg('opening_turnover'),
        sqlc.arg('note'), sqlc.arg('actor_id'))
RETURNING id, created_at;

-- name: MonthlyRevenue :many
SELECT extract(month FROM e.business_date)::int AS month,
       coalesce(sum(l.credit_amount - l.debit_amount), 0)::numeric AS revenue
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.status = 'posted' AND a.code LIKE '4%'
  AND e.business_date >= make_date(sqlc.arg('year')::int, 1, 1)
  AND e.business_date < make_date(sqlc.arg('year')::int + 1, 1, 1)
GROUP BY 1
ORDER BY 1;

-- name: ListFinalTaxRuns :many
SELECT r.*, coalesce(j.number, '')::text AS journal_number
FROM final_tax_runs r
LEFT JOIN journal_entries j ON j.id = r.journal_entry_id
WHERE r.tax_year = $1
ORDER BY r.tax_month;

-- name: InsertFinalTaxRun :one
INSERT INTO final_tax_runs (id, tax_year, tax_month, gross_revenue_amount, rate_id, rate_percent, tax_amount,
                            journal_entry_id, created_by)
VALUES (sqlc.arg('id'), sqlc.arg('tax_year'), sqlc.arg('tax_month'), sqlc.arg('gross_revenue_amount'), sqlc.arg('rate_id'),
        sqlc.arg('rate_percent'), sqlc.arg('tax_amount'), sqlc.narg('journal_entry_id'), sqlc.arg('actor_id'))
RETURNING id, created_at;

-- name: LockFinalTaxRunForDeposit :one
SELECT id, tax_amount, deposit_id FROM final_tax_runs
WHERE tax_year = sqlc.arg('tax_year') AND tax_month = sqlc.arg('tax_month')
FOR UPDATE;

-- name: MarkFinalTaxRunDeposited :exec
UPDATE final_tax_runs SET deposit_id = sqlc.arg('deposit_id') WHERE id = sqlc.arg('id');
