-- name: InsertRequest :one
INSERT INTO approval_requests (
    subject_type, subject_id, subject_label, subject_link, summary, amount, division_id, flags,
    requested_by, requested_by_name, status, current_level, total_levels, created_at, created_by, updated_at, updated_by)
VALUES (
    sqlc.arg('subject_type'), sqlc.arg('subject_id'), sqlc.arg('subject_label'), sqlc.arg('subject_link'),
    sqlc.arg('summary'), sqlc.arg('amount'), sqlc.narg('division_id'), sqlc.arg('flags'), sqlc.arg('requested_by'),
    sqlc.arg('requested_by_name'), sqlc.arg('status'), sqlc.arg('current_level'), sqlc.arg('total_levels'),
    sqlc.arg('now'), sqlc.arg('requested_by'), sqlc.arg('now'), sqlc.arg('requested_by'))
RETURNING id;

-- name: InsertStep :exec
INSERT INTO approval_steps (
    approval_request_id, level, policy_id, approver_role_code, approver_user_id, substitute_user_id,
    division_id, requester_is_approver, activated_at)
VALUES (
    sqlc.arg('approval_request_id'), sqlc.arg('level'), sqlc.arg('policy_id'), sqlc.arg('approver_role_code'),
    sqlc.narg('approver_user_id'), sqlc.narg('substitute_user_id'), sqlc.narg('division_id'),
    sqlc.arg('requester_is_approver'), sqlc.narg('activated_at'));

-- name: GetRequest :one
SELECT * FROM approval_requests WHERE id = $1;

-- name: GetRequestForUpdate :one
SELECT * FROM approval_requests WHERE id = $1 FOR UPDATE;

-- name: GetPendingRequestBySubjectForUpdate :one
SELECT * FROM approval_requests
WHERE subject_type = $1 AND subject_id = $2 AND status = 'pending_approval'
FOR UPDATE;

-- name: ListRequestsBySubject :many
SELECT * FROM approval_requests
WHERE subject_type = $1 AND subject_id = $2
ORDER BY created_at DESC, id DESC;

-- name: ListRequests :many
SELECT * FROM approval_requests r
WHERE (sqlc.narg('requested_by')::uuid IS NULL OR r.requested_by = sqlc.narg('requested_by'))
  AND (sqlc.narg('decided_by')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM approval_steps s WHERE s.approval_request_id = r.id AND s.decided_by = sqlc.narg('decided_by')))
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status'))
  AND (sqlc.narg('subject_type')::text IS NULL OR r.subject_type = sqlc.narg('subject_type'))
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountRequests :one
SELECT count(*) FROM approval_requests r
WHERE (sqlc.narg('requested_by')::uuid IS NULL OR r.requested_by = sqlc.narg('requested_by'))
  AND (sqlc.narg('decided_by')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM approval_steps s WHERE s.approval_request_id = r.id AND s.decided_by = sqlc.narg('decided_by')))
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status'))
  AND (sqlc.narg('subject_type')::text IS NULL OR r.subject_type = sqlc.narg('subject_type'));

-- ListPendingCandidates menyaring kasar permintaan yang level aktifnya mungkin bisa diputuskan salah satu
-- user atau role; penilaian akhir (scope divisi, pembuat dokumen, pengganti) dilakukan di domain.
-- name: ListPendingCandidates :many
SELECT r.* FROM approval_requests r
JOIN approval_steps s ON s.approval_request_id = r.id AND s.level = r.current_level
WHERE r.status = 'pending_approval'
  AND (s.approver_user_id = ANY(sqlc.arg('user_ids')::uuid[])
       OR s.substitute_user_id = ANY(sqlc.arg('user_ids')::uuid[])
       OR (s.approver_user_id IS NULL AND s.approver_role_code = ANY(sqlc.arg('role_codes')::text[])))
ORDER BY r.created_at, r.id
LIMIT 500;

-- name: ListPendingDueForReminder :many
SELECT r.* FROM approval_requests r
JOIN approval_steps s ON s.approval_request_id = r.id AND s.level = r.current_level
WHERE r.status = 'pending_approval'
  AND s.activated_at <= sqlc.arg('before')::timestamptz
  AND (s.last_reminded_at IS NULL OR s.last_reminded_at <= sqlc.arg('before')::timestamptz)
ORDER BY r.created_at, r.id
LIMIT 500;

-- name: UpdateRequest :execrows
UPDATE approval_requests
SET status = sqlc.arg('status'), current_level = sqlc.arg('current_level'), decided_at = sqlc.narg('decided_at'),
    updated_at = sqlc.arg('now'), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListStepsByRequests :many
SELECT * FROM approval_steps
WHERE approval_request_id = ANY(sqlc.arg('request_ids')::uuid[])
ORDER BY approval_request_id, level;

-- name: UpdateStep :exec
UPDATE approval_steps
SET decision = sqlc.narg('decision'), decided_by = sqlc.narg('decided_by'), decided_by_name = sqlc.arg('decided_by_name'),
    delegated_from_user_id = sqlc.narg('delegated_from_user_id'), note = sqlc.arg('note'),
    activated_at = sqlc.narg('activated_at'), decided_at = sqlc.narg('decided_at'),
    last_reminded_at = sqlc.narg('last_reminded_at')
WHERE id = sqlc.arg('id');
