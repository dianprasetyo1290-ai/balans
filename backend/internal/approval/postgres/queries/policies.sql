-- name: ListPolicies :many
SELECT * FROM approval_policies
WHERE (sqlc.narg('subject_type')::text IS NULL OR subject_type = sqlc.narg('subject_type'))
  AND (sqlc.arg('include_ended')::boolean OR valid_to IS NULL OR valid_to > sqlc.arg('at')::timestamptz)
ORDER BY subject_type, division_id NULLS FIRST, level, valid_from DESC;

-- name: ListActivePoliciesBySubjectType :many
SELECT * FROM approval_policies
WHERE subject_type = sqlc.arg('subject_type')
  AND valid_from <= sqlc.arg('at') AND (valid_to IS NULL OR valid_to > sqlc.arg('at')::timestamptz)
ORDER BY level;

-- name: GetPolicyForUpdate :one
SELECT * FROM approval_policies WHERE id = $1 FOR UPDATE;

-- name: InsertPolicy :one
INSERT INTO approval_policies (
    subject_type, division_id, level, min_amount, max_amount, condition, approver_role_code,
    approver_user_id, substitute_user_id, valid_from, created_by, updated_by)
VALUES (
    sqlc.arg('subject_type'), sqlc.narg('division_id'), sqlc.arg('level'), sqlc.narg('min_amount'),
    sqlc.narg('max_amount'), sqlc.arg('condition'), sqlc.arg('approver_role_code'), sqlc.narg('approver_user_id'),
    sqlc.narg('substitute_user_id'), sqlc.arg('valid_from'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: EndPolicy :execrows
UPDATE approval_policies
SET valid_to = sqlc.arg('valid_to'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
