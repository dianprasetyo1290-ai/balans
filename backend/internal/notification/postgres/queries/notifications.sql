-- name: InsertNotification :exec
INSERT INTO notifications (user_id, kind, title, body, link)
VALUES ($1, $2, $3, $4, $5);

-- name: ListNotifications :many
SELECT id, user_id, kind, title, body, link, read_at, created_at
FROM notifications
WHERE user_id = sqlc.arg('user_id')
  AND (NOT sqlc.arg('unread_only')::boolean OR read_at IS NULL)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountNotifications :one
SELECT count(*)
FROM notifications
WHERE user_id = sqlc.arg('user_id')
  AND (NOT sqlc.arg('unread_only')::boolean OR read_at IS NULL);

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = coalesce(read_at, sqlc.arg('read_at'))
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id');

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = sqlc.arg('read_at')
WHERE user_id = sqlc.arg('user_id') AND read_at IS NULL;
