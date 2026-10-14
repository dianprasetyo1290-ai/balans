-- name: InsertIdempotencyKey :execrows
INSERT INTO idempotency_keys (user_id, key, request_method, request_path, request_hash, status, expires_at)
VALUES ($1, $2, $3, $4, $5, 'processing', $6)
ON CONFLICT (user_id, key) DO NOTHING;

-- name: GetIdempotencyKey :one
SELECT user_id, key, request_method, request_path, request_hash, status, response_code, response_body, created_at, expires_at
FROM idempotency_keys
WHERE user_id = $1 AND key = $2;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET status = 'completed', response_code = $3, response_body = $4
WHERE user_id = $1 AND key = $2;

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE user_id = $1 AND key = $2;

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys
WHERE expires_at < $1;
