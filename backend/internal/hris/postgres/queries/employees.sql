-- name: InsertEmployee :one
INSERT INTO employees (
    employee_number, full_name, division_id, position_title, join_date, work_email, phone, nik_enc, nik_hash,
    created_by, updated_by)
VALUES (
    sqlc.arg('employee_number'), sqlc.arg('full_name'), sqlc.narg('division_id'), sqlc.arg('position_title'),
    sqlc.arg('join_date'), sqlc.arg('work_email'), sqlc.arg('phone'), sqlc.arg('nik_enc'), sqlc.narg('nik_hash'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetEmployee :one
SELECT * FROM employees WHERE id = $1 AND deleted_at IS NULL;

-- name: GetEmployeeForUpdate :one
SELECT * FROM employees WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListEmployeesByIDs :many
-- Termasuk karyawan terhapus agar nama tetap tampil di data lama.
SELECT * FROM employees WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListEmployees :many
SELECT * FROM employees
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(full_name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR employee_number ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('nik_hash')::bytea IS NULL OR nik_hash = sqlc.narg('nik_hash')::bytea)
  AND (sqlc.narg('status')::text IS NULL OR employment_status = sqlc.narg('status')::text)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'employeeNumber' THEN employee_number END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-employeeNumber' THEN employee_number END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'fullName' THEN full_name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-fullName' THEN full_name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'joinDate' THEN join_date END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-joinDate' THEN join_date END DESC,
  employee_number ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountEmployees :one
SELECT count(*) FROM employees
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(full_name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR employee_number ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('nik_hash')::bytea IS NULL OR nik_hash = sqlc.narg('nik_hash')::bytea)
  AND (sqlc.narg('status')::text IS NULL OR employment_status = sqlc.narg('status')::text);

-- name: UpdateEmployee :execrows
UPDATE employees
SET full_name = sqlc.arg('full_name'), division_id = sqlc.narg('division_id'),
    position_title = sqlc.arg('position_title'), join_date = sqlc.arg('join_date'),
    employment_status = sqlc.arg('employment_status'), resign_date = sqlc.narg('resign_date'),
    resign_reason = sqlc.arg('resign_reason'), work_email = sqlc.arg('work_email'), phone = sqlc.arg('phone'),
    nik_enc = sqlc.arg('nik_enc'), nik_hash = sqlc.narg('nik_hash'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: GetEmployeeIDByNumber :one
SELECT id FROM employees WHERE employee_number = $1 AND deleted_at IS NULL;
