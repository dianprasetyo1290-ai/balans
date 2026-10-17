-- name: ListChannels :many
SELECT * FROM channels
WHERE deleted_at IS NULL AND (sqlc.arg('include_inactive')::boolean OR is_active)
ORDER BY name;

-- name: GetChannelForUpdate :one
SELECT * FROM channels WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertChannel :one
INSERT INTO channels (code, name, is_active, created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateChannel :one
UPDATE channels
SET name = sqlc.arg('name'), is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteChannel :execrows
UPDATE channels
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: ChannelUsage :one
SELECT
    EXISTS (SELECT 1 FROM journal_lines l WHERE l.channel_id = sqlc.arg('id')::uuid)::boolean AS used,
    (SELECT count(*) FROM sub_channels s WHERE s.channel_id = sqlc.arg('id') AND s.deleted_at IS NULL)::bigint AS children;

-- name: ListSubChannels :many
SELECT * FROM sub_channels
WHERE deleted_at IS NULL AND (sqlc.arg('include_inactive')::boolean OR is_active)
  AND (sqlc.narg('channel_id')::uuid IS NULL OR channel_id = sqlc.narg('channel_id'))
ORDER BY name;

-- name: GetSubChannelForUpdate :one
SELECT * FROM sub_channels WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertSubChannel :one
INSERT INTO sub_channels (channel_id, code, name, party_type, party_id, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('channel_id'), sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('party_type'), sqlc.narg('party_id'),
    sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateSubChannel :one
UPDATE sub_channels
SET name = sqlc.arg('name'), party_type = sqlc.arg('party_type'), party_id = sqlc.narg('party_id'),
    is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteSubChannel :execrows
UPDATE sub_channels
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SubChannelUsed :one
SELECT EXISTS (SELECT 1 FROM journal_lines l WHERE l.sub_channel_id = sqlc.arg('id')::uuid)::boolean;

-- name: ListDivisions :many
SELECT * FROM divisions
WHERE deleted_at IS NULL AND (sqlc.arg('include_inactive')::boolean OR is_active)
ORDER BY name;

-- name: GetDivisionForUpdate :one
SELECT * FROM divisions WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertDivision :one
INSERT INTO divisions (code, name, is_active, created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateDivision :one
UPDATE divisions
SET name = sqlc.arg('name'), is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDivision :execrows
UPDATE divisions
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: DivisionUsed :one
SELECT EXISTS (SELECT 1 FROM journal_lines l WHERE l.division_id = sqlc.arg('id')::uuid)::boolean;

-- name: ListCampaigns :many
SELECT * FROM campaigns
WHERE deleted_at IS NULL AND (sqlc.arg('include_inactive')::boolean OR is_active)
  AND (sqlc.narg('q')::text IS NULL OR code ILIKE '%' || sqlc.narg('q') || '%' OR name ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY valid_from DESC, code;

-- name: GetCampaignForUpdate :one
SELECT * FROM campaigns WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertCampaign :one
INSERT INTO campaigns (code, name, valid_from, valid_to, budget_amount, description, is_active, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('valid_from'), sqlc.arg('valid_to'), sqlc.narg('budget_amount'),
    sqlc.arg('description'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateCampaign :one
UPDATE campaigns
SET name = sqlc.arg('name'), valid_from = sqlc.arg('valid_from'), valid_to = sqlc.arg('valid_to'),
    budget_amount = sqlc.narg('budget_amount'), description = sqlc.arg('description'), is_active = sqlc.arg('is_active'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCampaign :execrows
UPDATE campaigns
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: CampaignUsed :one
SELECT EXISTS (SELECT 1 FROM journal_lines l WHERE l.campaign_id = sqlc.arg('id')::uuid)::boolean;

-- name: CheckDimensions :one
-- Dimensi ada, aktif, sub-channel milik channel, dan kampanye berlaku pada tanggal transaksi.
SELECT
    (sqlc.narg('channel_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM channels c WHERE c.id = sqlc.narg('channel_id') AND c.is_active AND c.deleted_at IS NULL))::boolean AS channel_ok,
    (sqlc.narg('sub_channel_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM sub_channels s WHERE s.id = sqlc.narg('sub_channel_id') AND s.channel_id = sqlc.narg('channel_id')
            AND s.is_active AND s.deleted_at IS NULL))::boolean AS sub_channel_ok,
    (sqlc.narg('division_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM divisions d WHERE d.id = sqlc.narg('division_id') AND d.is_active AND d.deleted_at IS NULL))::boolean AS division_ok,
    (sqlc.narg('campaign_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM campaigns k WHERE k.id = sqlc.narg('campaign_id') AND k.is_active AND k.deleted_at IS NULL
            AND k.valid_from <= sqlc.arg('business_date')::date AND k.valid_to >= sqlc.arg('business_date')::date))::boolean AS campaign_ok;

-- name: DimensionNames :many
-- Kode & nama dimensi untuk tampilan (termasuk yang sudah dihapus agar jurnal lama tetap terbaca).
SELECT 'channel'::text AS kind, id, code, name FROM channels WHERE id = ANY(sqlc.arg('ids')::uuid[])
UNION ALL
SELECT 'sub_channel', id, code, name FROM sub_channels WHERE id = ANY(sqlc.arg('ids')::uuid[])
UNION ALL
SELECT 'division', id, code, name FROM divisions WHERE id = ANY(sqlc.arg('ids')::uuid[])
UNION ALL
SELECT 'campaign', id, code, name FROM campaigns WHERE id = ANY(sqlc.arg('ids')::uuid[]);
