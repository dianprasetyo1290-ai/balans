-- name: ListExpenseTypes :many
SELECT e.*, a.code AS account_code, a.name AS account_name
FROM expense_types e
JOIN accounts a ON a.id = e.expense_account_id
WHERE e.deleted_at IS NULL
  AND (sqlc.arg('include_inactive')::boolean OR e.is_active)
  AND (sqlc.narg('q')::text IS NULL OR e.code ILIKE '%' || sqlc.narg('q') || '%' OR e.name ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY e.code;

-- name: GetExpenseType :one
SELECT e.*, a.code AS account_code, a.name AS account_name
FROM expense_types e
JOIN accounts a ON a.id = e.expense_account_id
WHERE e.id = $1 AND e.deleted_at IS NULL;

-- name: GetExpenseTypeByCode :one
SELECT e.*, a.code AS account_code, a.name AS account_name
FROM expense_types e
JOIN accounts a ON a.id = e.expense_account_id
WHERE e.code = $1 AND e.deleted_at IS NULL;

-- name: GetExpenseTypeForUpdate :one
SELECT e.*, a.code AS account_code, a.name AS account_name
FROM expense_types e
JOIN accounts a ON a.id = e.expense_account_id
WHERE e.id = $1 AND e.deleted_at IS NULL
FOR UPDATE OF e;

-- name: InsertExpenseType :one
INSERT INTO expense_types (
    code, name, expense_account_id, withholding_type, requires_receipt, requires_campaign, approval_subject,
    description, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('expense_account_id'), sqlc.arg('withholding_type'),
    sqlc.arg('requires_receipt'), sqlc.arg('requires_campaign'), sqlc.arg('approval_subject'), sqlc.arg('description'),
    sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateExpenseType :execrows
UPDATE expense_types
SET name = sqlc.arg('name'), expense_account_id = sqlc.arg('expense_account_id'),
    withholding_type = sqlc.arg('withholding_type'), requires_receipt = sqlc.arg('requires_receipt'),
    requires_campaign = sqlc.arg('requires_campaign'), approval_subject = sqlc.arg('approval_subject'),
    description = sqlc.arg('description'), is_active = sqlc.arg('is_active'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeleteExpenseType :execrows
UPDATE expense_types
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;
