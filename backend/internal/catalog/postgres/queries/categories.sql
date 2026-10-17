-- name: ListCategories :many
SELECT * FROM product_categories
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
ORDER BY code;

-- name: ListCategoriesByIDs :many
SELECT * FROM product_categories WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetCategory :one
SELECT * FROM product_categories WHERE id = $1;

-- name: GetCategoryForUpdate :one
SELECT * FROM product_categories WHERE id = $1 FOR UPDATE;

-- name: InsertCategory :one
INSERT INTO product_categories (code, name, created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateCategory :one
UPDATE product_categories
SET name = sqlc.arg('name'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: SetCategoryDeleted :one
UPDATE product_categories
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING *;
