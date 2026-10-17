-- Harga khusus ----------------------------------------------------------------------------------

-- name: ListSpecialPrices :many
SELECT * FROM special_prices
WHERE deleted_at IS NULL
  AND (sqlc.narg('party_type')::text IS NULL OR party_type = sqlc.narg('party_type'))
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id'))
  AND (sqlc.narg('product_id')::uuid IS NULL OR product_id = sqlc.narg('product_id'))
  AND (sqlc.arg('include_ended')::boolean OR valid_to IS NULL OR valid_to >= sqlc.arg('today')::date)
ORDER BY party_type, party_id, product_id, valid_from DESC;

-- name: GetSpecialPrice :one
SELECT * FROM special_prices WHERE id = $1;

-- name: GetSpecialPriceForUpdate :one
SELECT * FROM special_prices WHERE id = $1 FOR UPDATE;

-- name: OverlappingSpecialPrices :many
SELECT * FROM special_prices
WHERE deleted_at IS NULL AND party_type = sqlc.arg('party_type') AND party_id = sqlc.arg('party_id')
  AND product_id = sqlc.arg('product_id')
  AND daterange(valid_from, valid_to, '[]') && daterange(sqlc.arg('valid_from')::date, sqlc.narg('valid_to')::date, '[]')
FOR UPDATE;

-- name: InsertSpecialPrice :one
INSERT INTO special_prices (party_type, party_id, product_id, price_amount, valid_from, valid_to, notes, created_by, updated_by)
VALUES (
    sqlc.arg('party_type'), sqlc.arg('party_id'), sqlc.arg('product_id'), sqlc.arg('price_amount'), sqlc.arg('valid_from'),
    sqlc.narg('valid_to'), sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateSpecialPriceEnd :one
UPDATE special_prices
SET valid_to = sqlc.narg('valid_to'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeleteSpecialPrice :execrows
UPDATE special_prices
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SpecialPriceOn :one
SELECT * FROM special_prices
WHERE deleted_at IS NULL AND party_type = sqlc.arg('party_type') AND party_id = sqlc.arg('party_id')
  AND product_id = sqlc.arg('product_id')
  AND valid_from <= sqlc.arg('on_date')::date AND (valid_to IS NULL OR valid_to >= sqlc.arg('on_date')::date);

-- Promo ------------------------------------------------------------------------------------------

-- name: ListPromotions :many
SELECT * FROM promotions
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.arg('include_ended')::boolean OR valid_to >= sqlc.arg('today')::date)
  AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('q')::text IS NULL OR code ILIKE '%' || sqlc.narg('q') || '%' OR name ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY valid_from DESC, code;

-- name: CountPromotionItems :many
SELECT promotion_id, count(*)::int AS item_count FROM promotion_items
WHERE promotion_id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY promotion_id;

-- name: GetPromotion :one
SELECT * FROM promotions WHERE id = $1;

-- name: GetPromotionForUpdate :one
SELECT * FROM promotions WHERE id = $1 FOR UPDATE;

-- name: InsertPromotion :one
INSERT INTO promotions (code, name, channel_id, sub_channel_id, valid_from, valid_to, is_active, description, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.narg('channel_id'), sqlc.narg('sub_channel_id'), sqlc.arg('valid_from'),
    sqlc.arg('valid_to'), sqlc.arg('is_active'), sqlc.arg('description'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdatePromotion :one
UPDATE promotions
SET name = sqlc.arg('name'), channel_id = sqlc.narg('channel_id'), sub_channel_id = sqlc.narg('sub_channel_id'),
    valid_from = sqlc.arg('valid_from'), valid_to = sqlc.arg('valid_to'), is_active = sqlc.arg('is_active'),
    description = sqlc.arg('description'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeletePromotion :execrows
UPDATE promotions
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: ListPromotionItems :many
SELECT * FROM promotion_items WHERE promotion_id = $1 ORDER BY created_at, id;

-- name: DeletePromotionItems :exec
DELETE FROM promotion_items WHERE promotion_id = $1;

-- name: InsertPromotionItems :exec
INSERT INTO promotion_items (promotion_id, product_id, price_amount, discount_percent, created_by)
SELECT sqlc.arg('promotion_id')::uuid, t.product_id, NULLIF(t.price_amount, '')::numeric, NULLIF(t.discount_percent, '')::numeric,
    sqlc.arg('actor_id')::uuid
FROM (
    SELECT unnest(sqlc.arg('product_ids')::uuid[]) AS product_id, unnest(sqlc.arg('prices')::text[]) AS price_amount,
        unnest(sqlc.arg('discount_percents')::text[]) AS discount_percent) AS t;

-- name: PromotionOffersOn :many
-- Item promo aktif untuk produk pada tanggal dan channel transaksi (promo tanpa channel berlaku semua channel).
SELECT p.id AS promotion_id, p.code, p.name, i.price_amount, i.discount_percent
FROM promotions p
JOIN promotion_items i ON i.promotion_id = p.id
WHERE p.deleted_at IS NULL AND p.is_active AND i.product_id = sqlc.arg('product_id')
  AND p.valid_from <= sqlc.arg('on_date')::date AND p.valid_to >= sqlc.arg('on_date')::date
  AND (p.channel_id IS NULL OR p.channel_id = sqlc.narg('channel_id'))
  AND (p.sub_channel_id IS NULL OR p.sub_channel_id = sqlc.narg('sub_channel_id'))
ORDER BY p.valid_from DESC, p.code;

-- Harga bertingkat -------------------------------------------------------------------------------

-- name: ListTieredPrices :many
SELECT * FROM tiered_prices
WHERE deleted_at IS NULL
  AND (sqlc.narg('product_id')::uuid IS NULL OR product_id = sqlc.narg('product_id'))
  AND (sqlc.narg('price_list_id')::uuid IS NULL OR price_list_id = sqlc.narg('price_list_id'))
  AND (sqlc.arg('include_ended')::boolean OR valid_to IS NULL OR valid_to >= sqlc.arg('today')::date)
ORDER BY product_id, price_list_id NULLS FIRST, min_qty, valid_from DESC;

-- name: GetTieredPrice :one
SELECT * FROM tiered_prices WHERE id = $1;

-- name: GetTieredPriceForUpdate :one
SELECT * FROM tiered_prices WHERE id = $1 FOR UPDATE;

-- name: OverlappingTieredPrices :many
SELECT * FROM tiered_prices
WHERE deleted_at IS NULL AND price_list_id IS NOT DISTINCT FROM sqlc.narg('price_list_id')::uuid
  AND product_id = sqlc.arg('product_id') AND min_qty = sqlc.arg('min_qty')
  AND daterange(valid_from, valid_to, '[]') && daterange(sqlc.arg('valid_from')::date, sqlc.narg('valid_to')::date, '[]')
FOR UPDATE;

-- name: InsertTieredPrice :one
INSERT INTO tiered_prices (price_list_id, product_id, min_qty, price_amount, valid_from, valid_to, notes, created_by, updated_by)
VALUES (
    sqlc.narg('price_list_id'), sqlc.arg('product_id'), sqlc.arg('min_qty'), sqlc.arg('price_amount'), sqlc.arg('valid_from'),
    sqlc.narg('valid_to'), sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateTieredPriceEnd :one
UPDATE tiered_prices
SET valid_to = sqlc.narg('valid_to'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeleteTieredPrice :execrows
UPDATE tiered_prices
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: TieredPricesOn :many
-- Harga bertingkat yang berlaku untuk qty dan tanggal: khusus daftar harga kandidat atau berlaku umum.
SELECT * FROM tiered_prices
WHERE deleted_at IS NULL AND product_id = sqlc.arg('product_id') AND min_qty <= sqlc.arg('qty')::numeric
  AND (price_list_id IS NULL OR price_list_id = ANY(sqlc.arg('price_list_ids')::uuid[]))
  AND valid_from <= sqlc.arg('on_date')::date AND (valid_to IS NULL OR valid_to >= sqlc.arg('on_date')::date)
ORDER BY min_qty DESC;

-- Batas diskon -----------------------------------------------------------------------------------

-- name: ListDiscountLimits :many
SELECT * FROM discount_limits
WHERE deleted_at IS NULL
  AND (sqlc.arg('include_ended')::boolean OR valid_to IS NULL OR valid_to >= sqlc.arg('today')::date)
ORDER BY role_code, valid_from DESC;

-- name: GetDiscountLimitForUpdate :one
SELECT * FROM discount_limits WHERE id = $1 FOR UPDATE;

-- name: OverlappingDiscountLimits :many
SELECT * FROM discount_limits
WHERE deleted_at IS NULL AND role_code = sqlc.arg('role_code')
  AND daterange(valid_from, valid_to, '[]') && daterange(sqlc.arg('valid_from')::date, sqlc.narg('valid_to')::date, '[]')
FOR UPDATE;

-- name: InsertDiscountLimit :one
INSERT INTO discount_limits (
    role_code, max_line_discount_percent, max_document_discount_percent, valid_from, valid_to, notes, created_by, updated_by)
VALUES (
    sqlc.arg('role_code'), sqlc.arg('max_line_discount_percent'), sqlc.arg('max_document_discount_percent'),
    sqlc.arg('valid_from'), sqlc.narg('valid_to'), sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateDiscountLimitEnd :one
UPDATE discount_limits
SET valid_to = sqlc.narg('valid_to'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: DeleteDiscountLimit :execrows
UPDATE discount_limits
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: DiscountLimitsOn :many
SELECT * FROM discount_limits
WHERE deleted_at IS NULL AND role_code = ANY(sqlc.arg('role_codes')::text[])
  AND valid_from <= sqlc.arg('on_date')::date AND (valid_to IS NULL OR valid_to >= sqlc.arg('on_date')::date);
