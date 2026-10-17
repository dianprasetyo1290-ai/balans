-- name: InsertSupplier :one
INSERT INTO suppliers (
    code, name, supplier_type, is_business_entity, npwp_enc, default_withholding, payment_term_days, address, phone,
    email, pic_name, notes, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('supplier_type'), sqlc.arg('is_business_entity'),
    sqlc.arg('npwp_enc'), sqlc.arg('default_withholding'), sqlc.arg('payment_term_days'), sqlc.arg('address'),
    sqlc.arg('phone'), sqlc.arg('email'), sqlc.arg('pic_name'), sqlc.arg('notes'), sqlc.arg('actor_id'),
    sqlc.arg('actor_id'))
RETURNING *;

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE id = $1 AND deleted_at IS NULL;

-- name: GetSupplierForUpdate :one
SELECT * FROM suppliers WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListSuppliers :many
SELECT * FROM suppliers
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('supplier_type')::text IS NULL OR supplier_type = sqlc.narg('supplier_type')::text)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'code' THEN code END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-code' THEN code END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-name' THEN name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-createdAt' THEN created_at END DESC,
  code ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountSuppliers :one
SELECT count(*) FROM suppliers
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('supplier_type')::text IS NULL OR supplier_type = sqlc.narg('supplier_type')::text);

-- name: UpdateSupplier :execrows
UPDATE suppliers
SET name = sqlc.arg('name'), supplier_type = sqlc.arg('supplier_type'),
    is_business_entity = sqlc.arg('is_business_entity'), npwp_enc = sqlc.arg('npwp_enc'),
    default_withholding = sqlc.arg('default_withholding'), payment_term_days = sqlc.arg('payment_term_days'),
    address = sqlc.arg('address'), phone = sqlc.arg('phone'), email = sqlc.arg('email'),
    pic_name = sqlc.arg('pic_name'), notes = sqlc.arg('notes'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeleteSupplier :execrows
UPDATE suppliers
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: ListBankAccounts :many
SELECT * FROM party_bank_accounts
WHERE party_type = sqlc.arg('party_type') AND party_id = ANY(sqlc.arg('party_ids')::uuid[]) AND deleted_at IS NULL
ORDER BY party_id, is_default DESC, created_at, id;

-- name: InsertBankAccount :one
INSERT INTO party_bank_accounts (
    party_type, party_id, bank_name, account_number_enc, account_name, is_default, created_by, updated_by)
VALUES (
    sqlc.arg('party_type'), sqlc.arg('party_id'), sqlc.arg('bank_name'), sqlc.arg('account_number_enc'),
    sqlc.arg('account_name'), sqlc.arg('is_default'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateBankAccount :execrows
UPDATE party_bank_accounts
SET bank_name = sqlc.arg('bank_name'), account_number_enc = sqlc.arg('account_number_enc'),
    account_name = sqlc.arg('account_name'), is_default = sqlc.arg('is_default'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND party_type = sqlc.arg('party_type') AND party_id = sqlc.arg('party_id')
  AND deleted_at IS NULL;

-- name: ClearDefaultBankAccount :exec
UPDATE party_bank_accounts SET is_default = false
WHERE party_type = $1 AND party_id = $2 AND is_default AND deleted_at IS NULL;

-- name: SoftDeleteBankAccounts :exec
UPDATE party_bank_accounts
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    is_default = false, version = version + 1
WHERE party_type = sqlc.arg('party_type') AND party_id = sqlc.arg('party_id')
  AND id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: GetSupplierIDByCode :one
SELECT id FROM suppliers WHERE code = $1 AND deleted_at IS NULL;
