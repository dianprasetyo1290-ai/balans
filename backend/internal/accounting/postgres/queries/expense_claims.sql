-- name: ListExpenseClaims :many
SELECT c.*, d.name AS division_name, coalesce(k.name, '')::text AS campaign_name,
       coalesce(j.number, '')::text AS journal_number, coalesce(p.number, '')::text AS payment_number,
       count(*) OVER () AS total_count
FROM expense_claims c
JOIN divisions d ON d.id = c.division_id
LEFT JOIN campaigns k ON k.id = c.campaign_id
LEFT JOIN journal_entries j ON j.id = c.journal_entry_id
LEFT JOIN payments p ON p.id = c.payment_id
WHERE c.deleted_at IS NULL
  AND (sqlc.narg('requester_id')::uuid IS NULL OR c.requester_id = sqlc.narg('requester_id'))
  AND (sqlc.narg('status')::text IS NULL OR c.status = sqlc.narg('status'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR c.division_id = sqlc.narg('division_id'))
  AND (sqlc.narg('date_from')::date IS NULL OR c.claim_date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::date IS NULL OR c.claim_date <= sqlc.narg('date_to'))
  AND (sqlc.narg('q')::text IS NULL OR c.number ILIKE '%' || sqlc.narg('q') || '%'
       OR c.payee_name ILIKE '%' || sqlc.narg('q') || '%' OR c.description ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY c.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: GetExpenseClaim :one
SELECT c.*, d.name AS division_name, coalesce(k.name, '')::text AS campaign_name,
       coalesce(j.number, '')::text AS journal_number, coalesce(p.number, '')::text AS payment_number,
       0::bigint AS total_count
FROM expense_claims c
JOIN divisions d ON d.id = c.division_id
LEFT JOIN campaigns k ON k.id = c.campaign_id
LEFT JOIN journal_entries j ON j.id = c.journal_entry_id
LEFT JOIN payments p ON p.id = c.payment_id
WHERE c.id = $1 AND c.deleted_at IS NULL;

-- name: LockExpenseClaim :one
SELECT version FROM expense_claims WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ExpenseClaimLines :many
SELECT l.*, t.code AS type_code, t.name AS type_name, a.code AS account_code
FROM expense_claim_lines l
JOIN expense_types t ON t.id = l.expense_type_id
JOIN accounts a ON a.id = t.expense_account_id
WHERE l.claim_id = $1
ORDER BY l.line_no;

-- name: InsertExpenseClaim :one
INSERT INTO expense_claims (
    claim_type, requester_id, division_id, campaign_id, payee_type, payee_id, payee_name, payee_tax_id_enc, claim_date,
    description, total_amount, withholding_amount, payable_amount, created_by, updated_by)
VALUES (
    sqlc.arg('claim_type'), sqlc.arg('requester_id'), sqlc.arg('division_id'), sqlc.narg('campaign_id'),
    sqlc.arg('payee_type'), sqlc.narg('payee_id'), sqlc.arg('payee_name'), sqlc.arg('payee_tax_id_enc'),
    sqlc.arg('claim_date'), sqlc.arg('description'), sqlc.arg('total_amount'), sqlc.arg('withholding_amount'),
    sqlc.arg('payable_amount'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: SaveExpenseClaim :execrows
UPDATE expense_claims
SET number = sqlc.narg('number'), claim_type = sqlc.arg('claim_type'), division_id = sqlc.arg('division_id'),
    campaign_id = sqlc.narg('campaign_id'), payee_type = sqlc.arg('payee_type'), payee_id = sqlc.narg('payee_id'),
    payee_name = sqlc.arg('payee_name'), payee_tax_id_enc = sqlc.arg('payee_tax_id_enc'), claim_date = sqlc.arg('claim_date'),
    description = sqlc.arg('description'), total_amount = sqlc.arg('total_amount'),
    withholding_amount = sqlc.arg('withholding_amount'), payable_amount = sqlc.arg('payable_amount'),
    status = sqlc.arg('status'), submitted_at = sqlc.narg('submitted_at'), decided_at = sqlc.narg('decided_at'),
    decision_note = sqlc.arg('decision_note'), verified_at = sqlc.narg('verified_at'), verified_by = sqlc.narg('verified_by'),
    verify_date = sqlc.narg('verify_date'), journal_entry_id = sqlc.narg('journal_entry_id'),
    payment_id = sqlc.narg('payment_id'), paid_at = sqlc.narg('paid_at'), cancel_reason = sqlc.arg('cancel_reason'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeleteExpenseClaimLines :exec
DELETE FROM expense_claim_lines WHERE claim_id = $1;

-- name: InsertExpenseClaimLine :exec
INSERT INTO expense_claim_lines (
    claim_id, line_no, expense_type_id, description, amount, receipt_file_id, withholding_type, object_code,
    withholding_rate, withholding_amount, created_by)
VALUES (
    sqlc.arg('claim_id'), sqlc.arg('line_no'), sqlc.arg('expense_type_id'), sqlc.arg('description'), sqlc.arg('amount'),
    sqlc.narg('receipt_file_id'), sqlc.arg('withholding_type'), sqlc.arg('object_code'), sqlc.arg('withholding_rate'),
    sqlc.arg('withholding_amount'), sqlc.arg('actor_id'));

-- name: UpdateExpenseClaimLineTax :exec
UPDATE expense_claim_lines
SET withholding_type = sqlc.arg('withholding_type'), object_code = sqlc.arg('object_code'),
    withholding_rate = sqlc.arg('withholding_rate'), withholding_amount = sqlc.arg('withholding_amount')
WHERE id = sqlc.arg('id');

-- name: ClaimsAwaitingPayment :many
SELECT c.id, c.number, c.payee_type, c.payee_id, c.payee_name, c.payable_amount, c.verify_date
FROM expense_claims c
WHERE c.status = 'awaiting_payment' AND c.deleted_at IS NULL
  AND (sqlc.narg('claim_id')::uuid IS NULL OR c.id = sqlc.narg('claim_id'))
ORDER BY c.verify_date, c.number
LIMIT 500;
