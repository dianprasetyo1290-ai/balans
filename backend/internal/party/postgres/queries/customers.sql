-- name: InsertCustomer :one
INSERT INTO customers (
    full_name, phone, phone_normalized, email, customer_type, employee_id, marketing_consent_at,
    marketing_consent_version, notes, created_by, updated_by)
VALUES (
    sqlc.arg('full_name'), sqlc.arg('phone'), sqlc.arg('phone_normalized'), sqlc.arg('email'),
    sqlc.arg('customer_type'), sqlc.narg('employee_id'), sqlc.narg('marketing_consent_at'),
    sqlc.arg('marketing_consent_version'), sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetCustomer :one
SELECT * FROM customers WHERE id = $1 AND deleted_at IS NULL;

-- name: GetCustomerForUpdate :one
SELECT * FROM customers WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: GetCustomerByPhone :one
SELECT * FROM customers WHERE phone_normalized = $1 AND deleted_at IS NULL;

-- name: ListCustomers :many
SELECT * FROM customers
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(full_name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR (sqlc.narg('phone_search')::text IS NOT NULL AND phone_normalized LIKE '%' || sqlc.narg('phone_search')::text || '%'))
  AND (sqlc.narg('phone')::text IS NULL OR phone_normalized = sqlc.narg('phone')::text)
  AND (sqlc.narg('customer_type')::text IS NULL OR customer_type = sqlc.narg('customer_type')::text)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'code' THEN code END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-code' THEN code END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'fullName' THEN full_name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-fullName' THEN full_name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountCustomers :one
SELECT count(*) FROM customers
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(full_name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR (sqlc.narg('phone_search')::text IS NOT NULL AND phone_normalized LIKE '%' || sqlc.narg('phone_search')::text || '%'))
  AND (sqlc.narg('phone')::text IS NULL OR phone_normalized = sqlc.narg('phone')::text)
  AND (sqlc.narg('customer_type')::text IS NULL OR customer_type = sqlc.narg('customer_type')::text);

-- name: UpdateCustomer :execrows
UPDATE customers
SET full_name = sqlc.arg('full_name'), phone = sqlc.arg('phone'), phone_normalized = sqlc.arg('phone_normalized'),
    email = sqlc.arg('email'), customer_type = sqlc.arg('customer_type'), employee_id = sqlc.narg('employee_id'),
    marketing_consent_at = sqlc.narg('marketing_consent_at'),
    marketing_consent_version = sqlc.arg('marketing_consent_version'), notes = sqlc.arg('notes'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeleteCustomer :execrows
UPDATE customers
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: ListCustomerAddresses :many
SELECT * FROM customer_addresses
WHERE customer_id = ANY(sqlc.arg('customer_ids')::uuid[]) AND deleted_at IS NULL
ORDER BY customer_id, is_default DESC, created_at, id;

-- name: InsertCustomerAddress :one
INSERT INTO customer_addresses (
    customer_id, label, recipient_name, phone, address_line, district, city, province, postal_code, latitude,
    longitude, is_default, created_by, updated_by)
VALUES (
    sqlc.arg('customer_id'), sqlc.arg('label'), sqlc.arg('recipient_name'), sqlc.arg('phone'),
    sqlc.arg('address_line'), sqlc.arg('district'), sqlc.arg('city'), sqlc.arg('province'), sqlc.arg('postal_code'),
    sqlc.narg('latitude'), sqlc.narg('longitude'), sqlc.arg('is_default'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateCustomerAddress :execrows
UPDATE customer_addresses
SET label = sqlc.arg('label'), recipient_name = sqlc.arg('recipient_name'), phone = sqlc.arg('phone'),
    address_line = sqlc.arg('address_line'), district = sqlc.arg('district'), city = sqlc.arg('city'),
    province = sqlc.arg('province'), postal_code = sqlc.arg('postal_code'), latitude = sqlc.narg('latitude'),
    longitude = sqlc.narg('longitude'), is_default = sqlc.arg('is_default'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND customer_id = sqlc.arg('customer_id') AND deleted_at IS NULL;

-- name: ClearCustomerDefaultAddress :exec
UPDATE customer_addresses SET is_default = false
WHERE customer_id = $1 AND is_default AND deleted_at IS NULL;

-- name: SoftDeleteCustomerAddresses :exec
UPDATE customer_addresses
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    is_default = false, version = version + 1
WHERE customer_id = sqlc.arg('customer_id') AND id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: GetCustomerIDByCode :one
SELECT id FROM customers WHERE code = $1 AND deleted_at IS NULL;
