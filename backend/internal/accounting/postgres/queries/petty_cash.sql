-- name: ListPettyCashFunds :many
SELECT f.*, d.name AS division_name
FROM petty_cash_funds f
JOIN divisions d ON d.id = f.division_id
WHERE (sqlc.arg('include_inactive')::boolean OR f.is_active)
ORDER BY f.code;

-- name: GetPettyCashFund :one
SELECT f.*, d.name AS division_name
FROM petty_cash_funds f
JOIN divisions d ON d.id = f.division_id
WHERE f.id = $1;

-- name: LockPettyCashFund :one
SELECT version FROM petty_cash_funds WHERE id = $1 FOR UPDATE;

-- name: InsertPettyCashFund :one
INSERT INTO petty_cash_funds (code, name, holder_user_id, division_id, ceiling_amount, is_active, description,
                              created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('holder_user_id'), sqlc.arg('division_id'),
        sqlc.arg('ceiling_amount'), sqlc.arg('is_active'), sqlc.arg('description'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: SavePettyCashFund :execrows
UPDATE petty_cash_funds
SET name = sqlc.arg('name'), holder_user_id = sqlc.arg('holder_user_id'), division_id = sqlc.arg('division_id'),
    ceiling_amount = sqlc.arg('ceiling_amount'), balance_amount = sqlc.arg('balance_amount'),
    is_active = sqlc.arg('is_active'), description = sqlc.arg('description'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListPettyCashTransactions :many
SELECT t.*, f.code AS fund_code, f.name AS fund_name, d.name AS division_name,
       coalesce(et.code, '')::text AS type_code, coalesce(et.name, '')::text AS type_name,
       coalesce(ea.code, '')::text AS account_code, coalesce(k.name, '')::text AS campaign_name,
       coalesce(b.name, '')::text AS bank_name, coalesce(j.number, '')::text AS journal_number,
       count(*) OVER () AS total_count
FROM petty_cash_transactions t
JOIN petty_cash_funds f ON f.id = t.fund_id
JOIN divisions d ON d.id = t.division_id
LEFT JOIN expense_types et ON et.id = t.expense_type_id
LEFT JOIN accounts ea ON ea.id = et.expense_account_id
LEFT JOIN campaigns k ON k.id = t.campaign_id
LEFT JOIN bank_accounts b ON b.id = t.bank_account_id
LEFT JOIN journal_entries j ON j.id = t.journal_entry_id
WHERE (sqlc.narg('fund_id')::uuid IS NULL OR t.fund_id = sqlc.narg('fund_id'))
  AND (sqlc.narg('transaction_type')::text IS NULL OR t.transaction_type = sqlc.narg('transaction_type'))
  AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
  AND (sqlc.narg('date_from')::date IS NULL OR t.business_date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::date IS NULL OR t.business_date <= sqlc.narg('date_to'))
ORDER BY t.business_date DESC, t.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: GetPettyCashTransaction :one
SELECT t.*, f.code AS fund_code, f.name AS fund_name, d.name AS division_name,
       coalesce(et.code, '')::text AS type_code, coalesce(et.name, '')::text AS type_name,
       coalesce(ea.code, '')::text AS account_code, coalesce(k.name, '')::text AS campaign_name,
       coalesce(b.name, '')::text AS bank_name, coalesce(j.number, '')::text AS journal_number,
       0::bigint AS total_count
FROM petty_cash_transactions t
JOIN petty_cash_funds f ON f.id = t.fund_id
JOIN divisions d ON d.id = t.division_id
LEFT JOIN expense_types et ON et.id = t.expense_type_id
LEFT JOIN accounts ea ON ea.id = et.expense_account_id
LEFT JOIN campaigns k ON k.id = t.campaign_id
LEFT JOIN bank_accounts b ON b.id = t.bank_account_id
LEFT JOIN journal_entries j ON j.id = t.journal_entry_id
WHERE t.id = $1;

-- name: LockPettyCashTransaction :one
SELECT version FROM petty_cash_transactions WHERE id = $1 FOR UPDATE;

-- name: InsertPettyCashTransaction :one
INSERT INTO petty_cash_transactions (
    fund_id, transaction_type, business_date, expense_type_id, division_id, campaign_id, description, amount,
    receipt_file_id, created_by, updated_by)
VALUES (
    sqlc.arg('fund_id'), sqlc.arg('transaction_type'), sqlc.arg('business_date'), sqlc.narg('expense_type_id'),
    sqlc.arg('division_id'), sqlc.narg('campaign_id'), sqlc.arg('description'), sqlc.arg('amount'),
    sqlc.narg('receipt_file_id'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: SavePettyCashTransaction :execrows
UPDATE petty_cash_transactions
SET number = sqlc.narg('number'), business_date = sqlc.arg('business_date'), expense_type_id = sqlc.narg('expense_type_id'),
    division_id = sqlc.arg('division_id'), campaign_id = sqlc.narg('campaign_id'), description = sqlc.arg('description'),
    amount = sqlc.arg('amount'), receipt_file_id = sqlc.narg('receipt_file_id'), bank_account_id = sqlc.narg('bank_account_id'),
    status = sqlc.arg('status'), decision_note = sqlc.arg('decision_note'), journal_entry_id = sqlc.narg('journal_entry_id'),
    posted_at = sqlc.narg('posted_at'), posted_by = sqlc.narg('posted_by'), cancel_reason = sqlc.arg('cancel_reason'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
