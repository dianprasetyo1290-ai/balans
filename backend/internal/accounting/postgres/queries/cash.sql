-- name: ListBankAccounts :many
SELECT b.*, a.code AS gl_code, a.name AS gl_name
FROM bank_accounts b
JOIN accounts a ON a.id = b.account_id
WHERE (b.deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.arg('include_inactive')::boolean OR b.is_active)
ORDER BY b.code;

-- name: GetBankAccount :one
SELECT b.*, a.code AS gl_code, a.name AS gl_name
FROM bank_accounts b
JOIN accounts a ON a.id = b.account_id
WHERE b.id = $1;

-- name: GetBankAccountForUpdate :one
SELECT b.*, a.code AS gl_code, a.name AS gl_name
FROM bank_accounts b
JOIN accounts a ON a.id = b.account_id
WHERE b.id = $1
FOR UPDATE OF b;

-- name: InsertBankAccount :one
INSERT INTO bank_accounts (
    code, name, kind, bank_name, account_number_enc, account_number_last4, account_holder, account_id, is_active,
    description, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('kind'), sqlc.arg('bank_name'), sqlc.arg('account_number_enc'),
    sqlc.arg('account_number_last4'), sqlc.arg('account_holder'), sqlc.arg('account_id'), sqlc.arg('is_active'),
    sqlc.arg('description'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateBankAccount :execrows
UPDATE bank_accounts
SET name = sqlc.arg('name'), kind = sqlc.arg('kind'), bank_name = sqlc.arg('bank_name'),
    account_number_enc = sqlc.arg('account_number_enc'), account_number_last4 = sqlc.arg('account_number_last4'),
    account_holder = sqlc.arg('account_holder'), account_id = sqlc.arg('account_id'), is_active = sqlc.arg('is_active'),
    description = sqlc.arg('description'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: SetBankAccountDeleted :execrows
UPDATE bank_accounts
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: BankAccountUsage :one
SELECT
    (SELECT count(*) FROM payments p WHERE p.bank_account_id = $1 AND p.status <> 'cancelled')::bigint AS payments,
    (SELECT count(*) FROM bank_statements s WHERE s.bank_account_id = $1)::bigint AS statements;

-- name: ListPayments :many
SELECT p.*, b.code AS bank_code, b.name AS bank_name, ba.code AS bank_gl_code, c.code AS counter_code,
       c.name AS counter_name, j.number AS journal_number,
       EXISTS (SELECT 1 FROM bank_statement_lines l
               JOIN journal_lines jl ON jl.id = l.journal_line_id
               WHERE l.status = 'matched' AND jl.journal_entry_id = p.journal_entry_id) AS reconciled,
       count(*) OVER () AS total_count
FROM payments p
JOIN bank_accounts b ON b.id = p.bank_account_id
JOIN accounts ba ON ba.id = b.account_id
JOIN accounts c ON c.id = p.counter_account_id
LEFT JOIN journal_entries j ON j.id = p.journal_entry_id
WHERE p.deleted_at IS NULL
  AND (sqlc.narg('bank_account_id')::uuid IS NULL OR p.bank_account_id = sqlc.narg('bank_account_id'))
  AND (sqlc.narg('direction')::text IS NULL OR p.direction = sqlc.narg('direction'))
  AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status'))
  AND (sqlc.narg('date_from')::date IS NULL OR p.payment_date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::date IS NULL OR p.payment_date <= sqlc.narg('date_to'))
  AND (sqlc.narg('q')::text IS NULL OR p.number ILIKE '%' || sqlc.narg('q') || '%'
       OR p.party_name ILIKE '%' || sqlc.narg('q') || '%' OR p.reference ILIKE '%' || sqlc.narg('q') || '%'
       OR p.description ILIKE '%' || sqlc.narg('q') || '%' OR p.source_label ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY p.payment_date DESC, p.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: GetPayment :one
SELECT p.*, b.code AS bank_code, b.name AS bank_name, ba.code AS bank_gl_code, c.code AS counter_code,
       c.name AS counter_name, j.number AS journal_number,
       EXISTS (SELECT 1 FROM bank_statement_lines l
               JOIN journal_lines jl ON jl.id = l.journal_line_id
               WHERE l.status = 'matched' AND jl.journal_entry_id = p.journal_entry_id) AS reconciled,
       0::bigint AS total_count
FROM payments p
JOIN bank_accounts b ON b.id = p.bank_account_id
JOIN accounts ba ON ba.id = b.account_id
JOIN accounts c ON c.id = p.counter_account_id
LEFT JOIN journal_entries j ON j.id = p.journal_entry_id
WHERE p.id = $1 AND p.deleted_at IS NULL;

-- name: LockPayment :one
SELECT version FROM payments WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertPayment :one
INSERT INTO payments (
    direction, bank_account_id, payment_date, amount, counter_account_id, party_type, party_id, party_name,
    reference, description, proof_file_id, source_type, source_id, source_label, created_by, updated_by)
VALUES (
    sqlc.arg('direction'), sqlc.arg('bank_account_id'), sqlc.arg('payment_date'), sqlc.arg('amount'),
    sqlc.arg('counter_account_id'), sqlc.arg('party_type'), sqlc.narg('party_id'), sqlc.arg('party_name'),
    sqlc.arg('reference'), sqlc.arg('description'), sqlc.narg('proof_file_id'), sqlc.arg('source_type'),
    sqlc.narg('source_id'), sqlc.arg('source_label'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: SavePayment :execrows
UPDATE payments
SET number = sqlc.narg('number'), direction = sqlc.arg('direction'), bank_account_id = sqlc.arg('bank_account_id'),
    payment_date = sqlc.arg('payment_date'), amount = sqlc.arg('amount'), counter_account_id = sqlc.arg('counter_account_id'),
    party_type = sqlc.arg('party_type'), party_id = sqlc.narg('party_id'), party_name = sqlc.arg('party_name'),
    reference = sqlc.arg('reference'), description = sqlc.arg('description'), proof_file_id = sqlc.narg('proof_file_id'),
    status = sqlc.arg('status'), journal_entry_id = sqlc.narg('journal_entry_id'), posted_at = sqlc.narg('posted_at'),
    posted_by = sqlc.narg('posted_by'), cancelled_at = sqlc.narg('cancelled_at'), cancelled_by = sqlc.narg('cancelled_by'),
    cancel_reason = sqlc.arg('cancel_reason'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: BankMutationOpening :one
SELECT coalesce(sum(l.debit_amount - l.credit_amount), 0)::numeric AS opening
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE e.status = 'posted' AND l.account_id = sqlc.arg('account_id') AND e.business_date < sqlc.arg('date_from');

-- name: BankMutationCount :one
SELECT count(*)::bigint
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE e.status = 'posted' AND l.account_id = sqlc.arg('account_id')
  AND e.business_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to');

-- name: BankMutations :many
SELECT l.id AS line_id, e.id AS entry_id, e.number, e.business_date, e.description, e.source_type, e.source_label,
       l.memo, (l.debit_amount - l.credit_amount)::numeric AS amount,
       sum(l.debit_amount - l.credit_amount) OVER (ORDER BY e.business_date, e.number, l.line_no)::numeric AS cumulative,
       EXISTS (SELECT 1 FROM bank_statement_lines s WHERE s.journal_line_id = l.id AND s.status = 'matched') AS reconciled
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE e.status = 'posted' AND l.account_id = sqlc.arg('account_id')
  AND e.business_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
ORDER BY e.business_date, e.number, l.line_no
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: UnreconciledBankLines :many
SELECT l.id AS line_id, e.id AS entry_id, e.number, e.business_date, e.description, e.source_type, e.source_label,
       (l.debit_amount - l.credit_amount)::numeric AS amount,
       coalesce(p.reference, '')::text AS payment_reference, coalesce(p.number, '')::text AS payment_number,
       coalesce(p.party_name, '')::text AS party_name
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
LEFT JOIN payments p ON p.journal_entry_id = e.id
WHERE e.status = 'posted' AND l.account_id = sqlc.arg('account_id')
  AND e.business_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
  AND NOT EXISTS (SELECT 1 FROM bank_statement_lines s WHERE s.journal_line_id = l.id AND s.status = 'matched')
ORDER BY e.business_date, e.number, l.line_no
LIMIT 2000;

-- name: JournalLineOnAccount :one
SELECT l.id AS line_id, e.id AS entry_id, e.number, e.business_date, (l.debit_amount - l.credit_amount)::numeric AS amount
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE e.status = 'posted' AND e.id = sqlc.arg('entry_id') AND l.account_id = sqlc.arg('account_id')
ORDER BY l.line_no
LIMIT 1;

-- name: GetJournalLine :one
SELECT l.id AS line_id, e.id AS entry_id, e.number, e.business_date, l.account_id,
       (l.debit_amount - l.credit_amount)::numeric AS amount
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE l.id = $1 AND e.status = 'posted';
