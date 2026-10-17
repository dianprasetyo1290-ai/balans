-- name: ListAccounts :many
SELECT * FROM accounts
WHERE deleted_at IS NULL
  AND (sqlc.arg('include_inactive')::boolean OR is_active)
ORDER BY code;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1 AND deleted_at IS NULL;

-- name: GetAccountForUpdate :one
SELECT * FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: GetAccountsByCodes :many
SELECT * FROM accounts WHERE code = ANY(sqlc.arg('codes')::text[]) AND deleted_at IS NULL;

-- name: GetAccountsByIDs :many
SELECT * FROM accounts WHERE id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: AccountAncestors :many
-- Rantai induk dari akun ke atas (untuk mencegah siklus).
WITH RECURSIVE chain AS (
    SELECT a.id, a.parent_id, 1 AS depth FROM accounts a WHERE a.id = sqlc.arg('id')
    UNION ALL
    SELECT p.id, p.parent_id, c.depth + 1 FROM accounts p JOIN chain c ON p.id = c.parent_id WHERE c.depth < 20
)
SELECT chain.id::uuid FROM chain;

-- name: InsertAccount :one
INSERT INTO accounts (
    code, name, account_group, normal_balance, parent_id, is_postable, allow_manual_posting, is_active,
    description, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('account_group'), sqlc.arg('normal_balance'), sqlc.narg('parent_id'),
    sqlc.arg('is_postable'), sqlc.arg('allow_manual_posting'), sqlc.arg('is_active'), sqlc.arg('description'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateAccount :one
UPDATE accounts
SET name = sqlc.arg('name'), normal_balance = sqlc.arg('normal_balance'), parent_id = sqlc.narg('parent_id'),
    is_postable = sqlc.arg('is_postable'), allow_manual_posting = sqlc.arg('allow_manual_posting'),
    is_active = sqlc.arg('is_active'), description = sqlc.arg('description'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteAccount :execrows
UPDATE accounts
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: AccountUsage :one
SELECT
    (SELECT count(*) FROM journal_lines l WHERE l.account_id = sqlc.arg('id')::uuid)::bigint AS journal_lines,
    (SELECT count(*) FROM accounts c WHERE c.parent_id = sqlc.arg('id') AND c.deleted_at IS NULL)::bigint AS children,
    (SELECT count(*) FROM posting_rule_accounts m WHERE m.account_id = sqlc.arg('id')
        AND (m.valid_to IS NULL OR m.valid_to > CURRENT_DATE))::bigint AS rule_mappings,
    (SELECT count(*) FROM expense_types e WHERE e.expense_account_id = sqlc.arg('id')
        AND e.deleted_at IS NULL)::bigint AS expense_types;
