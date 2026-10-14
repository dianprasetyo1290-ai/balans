-- name: InsertNotificationEmail :one
INSERT INTO notification_emails (recipient, subject, body_text)
VALUES ($1, $2, $3)
RETURNING id;

-- name: GetNotificationEmail :one
SELECT id, recipient, subject, body_text, status, attempts, last_error, created_at, sent_at
FROM notification_emails
WHERE id = $1;

-- name: MarkNotificationEmailSent :exec
UPDATE notification_emails
SET status = 'sent', attempts = attempts + 1, sent_at = $2, last_error = NULL
WHERE id = $1;

-- name: MarkNotificationEmailAttemptFailed :exec
UPDATE notification_emails
SET attempts = attempts + 1, last_error = $2
WHERE id = $1;
