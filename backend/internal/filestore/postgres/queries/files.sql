-- name: InsertFile :one
INSERT INTO file_objects (original_name, content_type, size_bytes, owner_type, created_by, updated_by)
VALUES (sqlc.arg('original_name'), sqlc.arg('content_type'), sqlc.arg('size_bytes'), sqlc.arg('owner_type'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetFile :one
SELECT * FROM file_objects WHERE id = $1 AND deleted_at IS NULL;

-- name: GetFileForUpdate :one
SELECT * FROM file_objects WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListFilesByIDs :many
SELECT * FROM file_objects WHERE id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: MarkFileUploaded :execrows
UPDATE file_objects
SET status = 'uploaded', sha256 = sqlc.arg('sha256'), uploaded_at = sqlc.arg('uploaded_at')::timestamptz,
    updated_at = sqlc.arg('uploaded_at')::timestamptz, version = version + 1
WHERE id = sqlc.arg('id') AND status = 'pending' AND version = sqlc.arg('version');

-- name: AttachFile :execrows
UPDATE file_objects
SET owner_id = sqlc.arg('owner_id'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
