-- name: ListProducts :many
SELECT * FROM products p
WHERE (p.deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR p.sku ILIKE '%' || sqlc.narg('search')::text || '%'
       OR p.name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR p.variant ILIKE '%' || sqlc.narg('search')::text || '%'
       OR EXISTS (SELECT 1 FROM product_barcodes b WHERE b.product_id = p.id AND b.barcode = sqlc.narg('search')::text))
  AND (cardinality(sqlc.arg('product_types')::text[]) = 0 OR p.product_type = ANY(sqlc.arg('product_types')::text[]))
  AND (sqlc.narg('is_bundle')::boolean IS NULL OR p.is_bundle = sqlc.narg('is_bundle')::boolean)
  AND (sqlc.narg('category_id')::uuid IS NULL OR p.category_id = sqlc.narg('category_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR p.is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'sku' THEN p.sku END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-sku' THEN p.sku END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'name' THEN p.name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-name' THEN p.name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN p.created_at END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-createdAt' THEN p.created_at END DESC,
  p.sku ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountProducts :one
SELECT count(*) FROM products p
WHERE (p.deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.narg('search')::text IS NULL
       OR p.sku ILIKE '%' || sqlc.narg('search')::text || '%'
       OR p.name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR p.variant ILIKE '%' || sqlc.narg('search')::text || '%'
       OR EXISTS (SELECT 1 FROM product_barcodes b WHERE b.product_id = p.id AND b.barcode = sqlc.narg('search')::text))
  AND (cardinality(sqlc.arg('product_types')::text[]) = 0 OR p.product_type = ANY(sqlc.arg('product_types')::text[]))
  AND (sqlc.narg('is_bundle')::boolean IS NULL OR p.is_bundle = sqlc.narg('is_bundle')::boolean)
  AND (sqlc.narg('category_id')::uuid IS NULL OR p.category_id = sqlc.narg('category_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR p.is_active = sqlc.narg('is_active')::boolean);

-- name: GetProduct :one
SELECT * FROM products WHERE id = $1;

-- name: GetProductForUpdate :one
SELECT * FROM products WHERE id = $1 FOR UPDATE;

-- name: ListProductsByIDs :many
SELECT * FROM products WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetActiveProductBySKU :one
SELECT * FROM products WHERE sku = $1 AND deleted_at IS NULL;

-- name: GetActiveProductByBarcode :one
SELECT p.* FROM products p
JOIN product_barcodes b ON b.product_id = p.id
WHERE b.barcode = $1 AND p.deleted_at IS NULL;

-- name: InsertProduct :one
INSERT INTO products (
    sku, name, variant, product_type, packaging_type, uom, shelf_life_days, has_batch, is_bundle, category_id,
    min_stock_qty, units_per_carton, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('sku'), sqlc.arg('name'), sqlc.arg('variant'), sqlc.arg('product_type'), sqlc.narg('packaging_type'),
    sqlc.arg('uom'), sqlc.narg('shelf_life_days'), sqlc.arg('has_batch'), sqlc.arg('is_bundle'),
    sqlc.narg('category_id'), sqlc.narg('min_stock_qty'), sqlc.narg('units_per_carton'), sqlc.arg('is_active'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateProduct :one
UPDATE products
SET name = sqlc.arg('name'), variant = sqlc.arg('variant'), packaging_type = sqlc.narg('packaging_type'),
    uom = sqlc.arg('uom'), shelf_life_days = sqlc.narg('shelf_life_days'), has_batch = sqlc.arg('has_batch'),
    category_id = sqlc.narg('category_id'), min_stock_qty = sqlc.narg('min_stock_qty'),
    units_per_carton = sqlc.narg('units_per_carton'), is_active = sqlc.arg('is_active'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: SetProductDeleted :one
UPDATE products
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListBarcodesByProducts :many
SELECT * FROM product_barcodes
WHERE product_id = ANY(sqlc.arg('product_ids')::uuid[])
ORDER BY product_id, is_primary DESC, created_at, id;

-- name: ListBarcodeOwners :many
SELECT barcode, product_id FROM product_barcodes WHERE barcode = ANY(sqlc.arg('barcodes')::text[]);

-- name: DeleteBarcodesByProduct :exec
DELETE FROM product_barcodes WHERE product_id = $1;

-- name: InsertBarcode :exec
INSERT INTO product_barcodes (product_id, barcode, symbology, is_primary, created_by)
VALUES (sqlc.arg('product_id'), sqlc.arg('barcode'), sqlc.arg('symbology'), sqlc.arg('is_primary'), sqlc.arg('actor_id'));

-- name: ListComponentsByBundles :many
SELECT c.bundle_product_id, c.component_product_id, c.qty, p.sku, p.name, p.variant, p.uom,
       (p.deleted_at IS NOT NULL)::boolean AS deleted
FROM product_bundle_components c
JOIN products p ON p.id = c.component_product_id
WHERE c.bundle_product_id = ANY(sqlc.arg('bundle_ids')::uuid[])
ORDER BY c.bundle_product_id, c.created_at, c.id;

-- name: DeleteComponentsByBundle :exec
DELETE FROM product_bundle_components WHERE bundle_product_id = $1;

-- name: InsertComponent :exec
INSERT INTO product_bundle_components (bundle_product_id, component_product_id, qty, created_by)
VALUES (sqlc.arg('bundle_product_id'), sqlc.arg('component_product_id'), sqlc.arg('qty'), sqlc.arg('actor_id'));

-- name: ListActiveBundlesUsing :many
SELECT p.sku FROM product_bundle_components c
JOIN products p ON p.id = c.bundle_product_id
WHERE c.component_product_id = $1 AND p.deleted_at IS NULL
ORDER BY p.sku;
