-- name: ListRoles :many
SELECT id, code, name, description, requires_mfa, has_division_scope, version, updated_at
FROM roles
WHERE deleted_at IS NULL
ORDER BY name;

-- name: GetRoleByID :one
SELECT id, code, name, description, requires_mfa, has_division_scope, version, updated_at
FROM roles
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListRolesByCodes :many
SELECT id, code, name, description, requires_mfa, has_division_scope, version, updated_at
FROM roles
WHERE code = ANY(sqlc.arg('codes')::text[]) AND deleted_at IS NULL;

-- name: ListRolePermissionPairs :many
SELECT rp.role_id, p.code
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
ORDER BY p.code;

-- name: ListRolePermissionCodes :many
SELECT p.code
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = $1
ORDER BY p.code;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = $1;

-- name: InsertRolePermissions :exec
INSERT INTO role_permissions (role_id, permission_id, created_by)
SELECT sqlc.arg('role_id')::uuid, p.id, sqlc.arg('created_by')::uuid
FROM permissions p
WHERE p.code = ANY(sqlc.arg('codes')::text[]);

-- name: BumpRoleVersion :one
UPDATE roles SET version = version + 1, updated_at = now(), updated_by = $3
WHERE id = $1 AND version = $2
RETURNING version;

-- name: UpsertPermission :one
INSERT INTO permissions (code, module, description, requires_reauth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (code) DO UPDATE
SET module = EXCLUDED.module, description = EXCLUDED.description, requires_reauth = EXCLUDED.requires_reauth
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: GrantPermissionByRoleCode :exec
INSERT INTO role_permissions (role_id, permission_id, created_by)
SELECT r.id, sqlc.arg('permission_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid
FROM roles r
WHERE r.code = sqlc.arg('role_code')::text
ON CONFLICT DO NOTHING;

-- name: ListPermissions :many
SELECT code, module, description, requires_reauth
FROM permissions
ORDER BY code;

-- name: ListUserRoles :many
SELECT ur.user_id, r.id AS role_id, r.code, r.name, r.requires_mfa, r.has_division_scope, ur.scope_division_id
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ANY(sqlc.arg('user_ids')::uuid[])
ORDER BY r.name;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles WHERE user_id = $1;

-- name: InsertUserRole :exec
INSERT INTO user_roles (user_id, role_id, scope_division_id, created_by)
VALUES ($1, $2, $3, $4);

-- name: ListUserPermissionCodes :many
SELECT DISTINCT p.code
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = $1
ORDER BY p.code;

-- name: ListActiveUsersByRoleCode :many
SELECT DISTINCT u.id, u.email, u.full_name
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
WHERE r.code = $1 AND u.is_active AND u.deleted_at IS NULL
  AND (sqlc.narg('division_id')::uuid IS NULL OR ur.scope_division_id IS NULL OR ur.scope_division_id = sqlc.narg('division_id')::uuid);
