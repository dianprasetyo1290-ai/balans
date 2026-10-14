-- name: InsertWebhookEvent :one
INSERT INTO webhook_events (provider, event_id, event_type, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (provider, event_id) DO NOTHING
RETURNING id;

-- name: GetWebhookEvent :one
SELECT id, provider, event_id, event_type, payload, received_at, processed_at
FROM webhook_events
WHERE id = $1;

-- name: MarkWebhookEventProcessed :exec
UPDATE webhook_events
SET processed_at = $2
WHERE id = $1 AND processed_at IS NULL;
