-- name: ListPriceLists :many
SELECT * FROM price_lists
WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean
  AND (sqlc.arg('include_inactive')::boolean OR is_active)
  AND (sqlc.narg('price_list_type')::text IS NULL OR price_list_type = sqlc.narg('price_list_type'))
  AND (sqlc.narg('q')::text IS NULL OR code ILIKE '%' || sqlc.narg('q') || '%' OR name ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY (price_list_type <> 'base'), code;

-- name: GetPriceList :one
SELECT * FROM price_lists WHERE id = $1;

-- name: GetPriceListForUpdate :one
SELECT * FROM price_lists WHERE id = $1 FOR UPDATE;

-- name: GetPriceListsByIDs :many
SELECT * FROM price_lists WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetBasePriceList :one
SELECT * FROM price_lists WHERE price_list_type = 'base' AND deleted_at IS NULL;

-- name: InsertPriceList :one
INSERT INTO price_lists (
    code, name, price_list_type, channel_id, sub_channel_id, partner_id, reseller_level, basis,
    reference_price_list_id, description, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('price_list_type'), sqlc.narg('channel_id'), sqlc.narg('sub_channel_id'),
    sqlc.narg('partner_id'), sqlc.arg('reseller_level'), sqlc.arg('basis'), sqlc.narg('reference_price_list_id'),
    sqlc.arg('description'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdatePriceList :one
UPDATE price_lists
SET name = sqlc.arg('name'), channel_id = sqlc.narg('channel_id'), sub_channel_id = sqlc.narg('sub_channel_id'),
    partner_id = sqlc.narg('partner_id'), reseller_level = sqlc.arg('reseller_level'), basis = sqlc.arg('basis'),
    reference_price_list_id = sqlc.narg('reference_price_list_id'), description = sqlc.arg('description'),
    is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: SetPriceListDeleted :one
UPDATE price_lists
SET deleted_at = sqlc.narg('deleted_at'), deleted_by = sqlc.narg('deleted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: CountPriceListReferences :one
SELECT count(*)::int FROM price_lists
WHERE reference_price_list_id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: FindScopePriceLists :many
-- Daftar harga aktif yang mungkin berlaku untuk transaksi: karyawan, mitra, dan channel/sub-channel.
SELECT * FROM price_lists
WHERE deleted_at IS NULL AND is_active AND (
    price_list_type = 'employee'
    OR (price_list_type = 'partner' AND partner_id = sqlc.narg('partner_id'))
    OR (price_list_type IN ('channel', 'vending') AND channel_id = sqlc.narg('channel_id')
        AND (sub_channel_id IS NULL OR sub_channel_id = sqlc.narg('sub_channel_id'))));

-- name: ListVersions :many
SELECT v.*, (SELECT count(*) FROM price_list_items i WHERE i.price_list_version_id = v.id)::int AS item_count
FROM price_list_versions v
WHERE v.price_list_id = $1
ORDER BY v.version_no DESC;

-- name: ListCurrentVersions :many
-- Versi aktif dan versi yang sedang disiapkan/diajukan untuk beberapa daftar harga (ringkasan list).
SELECT v.*, (SELECT count(*) FROM price_list_items i WHERE i.price_list_version_id = v.id)::int AS item_count
FROM price_list_versions v
WHERE v.price_list_id = ANY(sqlc.arg('ids')::uuid[]) AND v.status IN ('active', 'draft', 'pending_approval')
ORDER BY v.price_list_id, v.version_no DESC;

-- name: GetVersion :one
SELECT * FROM price_list_versions WHERE id = $1;

-- name: GetVersionForUpdate :one
SELECT * FROM price_list_versions WHERE id = $1 FOR UPDATE;

-- name: NextVersionNo :one
SELECT (coalesce(max(version_no), 0) + 1)::int FROM price_list_versions WHERE price_list_id = $1;

-- name: ActiveVersionsForUpdate :many
SELECT * FROM price_list_versions WHERE price_list_id = $1 AND status = 'active' ORDER BY valid_from FOR UPDATE;

-- name: InsertVersion :one
INSERT INTO price_list_versions (price_list_id, version_no, valid_from, valid_to, status, percent, notes, created_by, updated_by)
VALUES (
    sqlc.arg('price_list_id'), sqlc.arg('version_no'), sqlc.arg('valid_from'), sqlc.narg('valid_to'), sqlc.arg('status'),
    sqlc.narg('percent'), sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateVersion :one
UPDATE price_list_versions
SET valid_from = sqlc.arg('valid_from'), valid_to = sqlc.narg('valid_to'), status = sqlc.arg('status'),
    percent = sqlc.narg('percent'), notes = sqlc.arg('notes'), submitted_at = sqlc.narg('submitted_at'),
    submitted_by = sqlc.narg('submitted_by'), decided_at = sqlc.narg('decided_at'), decided_by = sqlc.narg('decided_by'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: ListVersionItems :many
SELECT * FROM price_list_items WHERE price_list_version_id = $1 ORDER BY created_at, id;

-- name: DeleteVersionItems :exec
DELETE FROM price_list_items WHERE price_list_version_id = $1;

-- name: InsertVersionItems :exec
INSERT INTO price_list_items (price_list_version_id, product_id, price_amount, max_retail_price_amount, created_by)
SELECT sqlc.arg('version_id')::uuid, t.product_id, t.price_amount::numeric, NULLIF(t.max_retail, '')::numeric,
    sqlc.arg('actor_id')::uuid
FROM (
    SELECT unnest(sqlc.arg('product_ids')::uuid[]) AS product_id, unnest(sqlc.arg('prices')::text[]) AS price_amount,
        unnest(sqlc.arg('max_retail_prices')::text[]) AS max_retail) AS t;

-- name: ListPriceOn :one
-- Harga produk dari versi aktif daftar harga pada tanggal; price_amount kosong bila produk tidak ada di versi.
SELECT v.id AS version_id, v.percent, i.price_amount, i.max_retail_price_amount
FROM price_list_versions v
LEFT JOIN price_list_items i ON i.price_list_version_id = v.id AND i.product_id = sqlc.arg('product_id')
WHERE v.price_list_id = sqlc.arg('price_list_id') AND v.status = 'active'
  AND v.valid_from <= sqlc.arg('on_date')::date AND (v.valid_to IS NULL OR v.valid_to >= sqlc.arg('on_date')::date);
