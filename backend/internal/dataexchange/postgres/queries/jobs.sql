-- name: InsertImportJob :one
INSERT INTO import_jobs (kind, file_id, file_name, file_sha256, created_by, updated_by)
VALUES (sqlc.arg('kind'), sqlc.arg('file_id'), sqlc.arg('file_name'), sqlc.arg('file_sha256'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetImportJob :one
SELECT * FROM import_jobs WHERE id = $1;

-- name: GetImportJobForUpdate :one
SELECT * FROM import_jobs WHERE id = $1 FOR UPDATE;

-- name: ListImportJobs :many
SELECT * FROM import_jobs
WHERE kind = ANY(sqlc.arg('kinds')::text[])
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountImportJobs :one
SELECT count(*) FROM import_jobs
WHERE kind = ANY(sqlc.arg('kinds')::text[])
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'));

-- name: UpdateImportJob :execrows
UPDATE import_jobs
SET status = sqlc.arg('status'), total_rows = sqlc.arg('total_rows'), processed_rows = sqlc.arg('processed_rows'),
    valid_rows = sqlc.arg('valid_rows'), error_rows = sqlc.arg('error_rows'),
    committed_rows = sqlc.arg('committed_rows'), error_file_id = sqlc.narg('error_file_id'),
    message = sqlc.arg('message'), validated_at = sqlc.narg('validated_at'),
    completed_at = sqlc.narg('completed_at'), updated_at = sqlc.arg('now')::timestamptz,
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: SetImportProgress :exec
UPDATE import_jobs
SET processed_rows = sqlc.arg('processed_rows'), committed_rows = sqlc.arg('committed_rows'),
    updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id');

-- name: DeleteImportRows :exec
DELETE FROM import_job_rows WHERE job_id = $1;

-- name: InsertImportRows :copyfrom
INSERT INTO import_job_rows (job_id, row_number, is_valid, cell_values, errors) VALUES ($1, $2, $3, $4, $5);

-- name: ListImportRows :many
SELECT * FROM import_job_rows
WHERE job_id = sqlc.arg('job_id')
  AND (sqlc.narg('is_valid')::boolean IS NULL OR is_valid = sqlc.narg('is_valid'))
ORDER BY row_number
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountImportRows :one
SELECT count(*) FROM import_job_rows
WHERE job_id = sqlc.arg('job_id')
  AND (sqlc.narg('is_valid')::boolean IS NULL OR is_valid = sqlc.narg('is_valid'));

-- name: ValidImportRowNumbers :many
SELECT row_number FROM import_job_rows WHERE job_id = $1 AND is_valid ORDER BY row_number;

-- name: SetImportRowError :exec
UPDATE import_job_rows SET is_valid = false, errors = sqlc.arg('errors')
WHERE job_id = sqlc.arg('job_id') AND row_number = sqlc.arg('row_number');

-- name: InsertExportJob :one
INSERT INTO export_jobs (kind, created_by, updated_by)
VALUES (sqlc.arg('kind'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetExportJob :one
SELECT * FROM export_jobs WHERE id = $1;

-- name: GetExportJobForUpdate :one
SELECT * FROM export_jobs WHERE id = $1 FOR UPDATE;

-- name: ListExportJobs :many
SELECT * FROM export_jobs
WHERE kind = ANY(sqlc.arg('kinds')::text[])
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountExportJobs :one
SELECT count(*) FROM export_jobs
WHERE kind = ANY(sqlc.arg('kinds')::text[])
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'));

-- name: UpdateExportJob :execrows
UPDATE export_jobs
SET status = sqlc.arg('status'), total_rows = sqlc.arg('total_rows'), file_id = sqlc.narg('file_id'),
    file_name = sqlc.arg('file_name'), message = sqlc.arg('message'), completed_at = sqlc.narg('completed_at'),
    updated_at = sqlc.arg('now')::timestamptz, updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
