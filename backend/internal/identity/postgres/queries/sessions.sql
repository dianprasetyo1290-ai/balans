-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, audience, family_id, refresh_token_hash, expires_at, idle_expires_at, user_agent, ip, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetSessionByTokenHash :one
SELECT id, user_id, audience, family_id, expires_at, idle_expires_at, revoked_at, replaced_by
FROM sessions
WHERE refresh_token_hash = $1
FOR UPDATE;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = $2, revoke_reason = $3, replaced_by = $4
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionFamily :many
UPDATE sessions SET revoked_at = $2, revoke_reason = $3
WHERE family_id = $1 AND revoked_at IS NULL
RETURNING id;

-- name: RevokeUserSessions :many
UPDATE sessions SET revoked_at = $2, revoke_reason = $3
WHERE user_id = $1 AND revoked_at IS NULL
RETURNING id;

-- name: InsertLoginAttempt :exec
INSERT INTO login_attempts (email_hash, user_id, ip, succeeded, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: RecentFailedLogins :one
SELECT count(*)::int AS failures, coalesce(max(la.created_at), 'epoch'::timestamptz)::timestamptz AS last_failed_at
FROM login_attempts la
WHERE la.email_hash = $1
  AND NOT la.succeeded
  AND la.created_at >= $2
  AND la.created_at > coalesce(
      (SELECT max(s.created_at) FROM login_attempts s WHERE s.email_hash = $1 AND s.succeeded),
      '-infinity'::timestamptz);

-- name: InsertPasswordToken :exec
INSERT INTO password_tokens (user_id, purpose, token_hash, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5);

-- name: GetPasswordTokenByHash :one
SELECT t.id, t.user_id, t.purpose, t.expires_at, t.used_at, u.email, u.full_name, u.is_active
FROM password_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1 AND u.deleted_at IS NULL
FOR UPDATE OF t;

-- name: MarkPasswordTokenUsed :exec
UPDATE password_tokens SET used_at = $2 WHERE id = $1;

-- name: InvalidateUserPasswordTokens :exec
UPDATE password_tokens SET used_at = $2 WHERE user_id = $1 AND used_at IS NULL;
