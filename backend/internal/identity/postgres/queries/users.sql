-- name: InsertUser :one
INSERT INTO users (email, full_name, division_id, employee_id, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id;

-- name: GetUserByID :one
SELECT id, email, full_name, password_hash, is_active, token_version, mfa_secret_enc, is_mfa_enabled,
       employee_id, division_id, last_login_at, created_at, updated_at, version
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT id, email, full_name, password_hash, is_active, token_version, mfa_secret_enc, is_mfa_enabled,
       employee_id, division_id, last_login_at, created_at, updated_at, version
FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT id, email, full_name, is_active, is_mfa_enabled, employee_id, division_id, last_login_at,
       created_at, updated_at, version, (password_hash IS NOT NULL)::boolean AS has_password
FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR full_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR email ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'fullName' THEN full_name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-fullName' THEN full_name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'email' THEN email END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-email' THEN email END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'lastLoginAt' THEN last_login_at END ASC NULLS FIRST,
  CASE WHEN sqlc.arg('sort')::text = '-lastLoginAt' THEN last_login_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountUsers :one
SELECT count(*)
FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR full_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR email ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: CountAllUsers :one
SELECT count(*) FROM users;

-- name: UpdateUserProfile :one
UPDATE users
SET full_name = $3, division_id = $4, employee_id = $5, updated_by = $6, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2 AND deleted_at IS NULL
RETURNING version;

-- name: SetUserActive :one
UPDATE users
SET is_active = $3, token_version = token_version + 1, updated_by = $4, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2 AND deleted_at IS NULL
RETURNING version;

-- name: BumpUserTokenVersion :exec
UPDATE users SET token_version = token_version + 1, updated_at = now(), version = version + 1
WHERE id = $1;

-- name: SetUserPassword :exec
UPDATE users
SET password_hash = $2, password_changed_at = $3, token_version = token_version + 1,
    updated_by = $1, updated_at = now(), version = version + 1
WHERE id = $1;

-- name: SetUserMFASecret :exec
UPDATE users SET mfa_secret_enc = $2, is_mfa_enabled = false, updated_at = now()
WHERE id = $1;

-- name: EnableUserMFA :exec
UPDATE users SET is_mfa_enabled = true, updated_at = now(), version = version + 1
WHERE id = $1 AND mfa_secret_enc IS NOT NULL;

-- name: ResetUserMFA :exec
UPDATE users
SET mfa_secret_enc = NULL, is_mfa_enabled = false, token_version = token_version + 1,
    updated_by = $2, updated_at = now(), version = version + 1
WHERE id = $1;

-- name: TouchUserLogin :exec
UPDATE users SET last_login_at = $2 WHERE id = $1;

-- name: GetAuthState :one
SELECT s.revoked_at, s.revoke_reason, s.expires_at, u.id AS user_id, u.email, u.full_name, u.is_active, u.token_version,
       u.deleted_at, u.is_mfa_enabled
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1;
