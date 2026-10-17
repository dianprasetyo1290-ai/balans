-- name: ListAssetClasses :many
SELECT sqlc.embed(c), a.code AS asset_account_code, acc.code AS accumulated_account_code
FROM fixed_asset_classes c
JOIN accounts a ON a.id = c.asset_account_id
JOIN accounts acc ON acc.id = c.accumulated_account_id
WHERE sqlc.arg('include_inactive')::boolean OR c.is_active
ORDER BY c.name;

-- name: GetAssetClass :one
SELECT sqlc.embed(c), a.code AS asset_account_code, acc.code AS accumulated_account_code
FROM fixed_asset_classes c
JOIN accounts a ON a.id = c.asset_account_id
JOIN accounts acc ON acc.id = c.accumulated_account_id
WHERE c.id = $1;

-- name: InsertAssetClass :one
INSERT INTO fixed_asset_classes (code, name, useful_life_months, asset_account_id, accumulated_account_id, is_active,
                                 created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('useful_life_months'), sqlc.arg('asset_account_id'),
        sqlc.arg('accumulated_account_id'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateAssetClass :execrows
UPDATE fixed_asset_classes
SET name = sqlc.arg('name'), useful_life_months = sqlc.arg('useful_life_months'),
    asset_account_id = sqlc.arg('asset_account_id'), accumulated_account_id = sqlc.arg('accumulated_account_id'),
    is_active = sqlc.arg('is_active'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListFixedAssets :many
SELECT sqlc.embed(f), c.name AS class_name
FROM fixed_assets f
JOIN fixed_asset_classes c ON c.id = f.class_id
WHERE f.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR f.status = sqlc.narg('status'))
  AND (sqlc.narg('class_id')::uuid IS NULL OR f.class_id = sqlc.narg('class_id'))
  AND (sqlc.narg('q')::text IS NULL OR f.code ILIKE '%' || sqlc.narg('q') || '%' OR f.name ILIKE '%' || sqlc.narg('q') || '%'
       OR f.location_label ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY f.code
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountFixedAssets :one
SELECT count(*) FROM fixed_assets f
WHERE f.deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR f.status = sqlc.narg('status'))
  AND (sqlc.narg('class_id')::uuid IS NULL OR f.class_id = sqlc.narg('class_id'))
  AND (sqlc.narg('q')::text IS NULL OR f.code ILIKE '%' || sqlc.narg('q') || '%' OR f.name ILIKE '%' || sqlc.narg('q') || '%'
       OR f.location_label ILIKE '%' || sqlc.narg('q') || '%');

-- name: GetFixedAsset :one
SELECT sqlc.embed(f), c.name AS class_name
FROM fixed_assets f
JOIN fixed_asset_classes c ON c.id = f.class_id
WHERE f.id = $1;

-- name: GetFixedAssetByCode :one
SELECT sqlc.embed(f), c.name AS class_name
FROM fixed_assets f
JOIN fixed_asset_classes c ON c.id = f.class_id
WHERE f.code = $1;

-- name: LockFixedAsset :one
SELECT version FROM fixed_assets WHERE id = $1 FOR UPDATE;

-- name: InsertFixedAsset :one
INSERT INTO fixed_assets (
    code, name, class_id, location_id, location_label, division_id, acquisition_source, acquisition_date,
    acquisition_cost_amount, salvage_amount, useful_life_months, depreciation_start_date, opening_accumulated_amount,
    opening_depreciated_months, accumulated_amount, payment_account_id, status, note, created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('class_id'), sqlc.narg('location_id'), sqlc.arg('location_label'),
    sqlc.arg('division_id'), sqlc.arg('acquisition_source'), sqlc.arg('acquisition_date'),
    sqlc.arg('acquisition_cost_amount'), sqlc.arg('salvage_amount'), sqlc.arg('useful_life_months'),
    sqlc.arg('depreciation_start_date'), sqlc.arg('opening_accumulated_amount'), sqlc.arg('opening_depreciated_months'),
    sqlc.arg('accumulated_amount'), sqlc.narg('payment_account_id'), sqlc.arg('status'), sqlc.arg('note'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateFixedAsset :execrows
UPDATE fixed_assets
SET name = sqlc.arg('name'), class_id = sqlc.arg('class_id'), location_id = sqlc.narg('location_id'),
    location_label = sqlc.arg('location_label'), division_id = sqlc.arg('division_id'),
    acquisition_source = sqlc.arg('acquisition_source'), acquisition_date = sqlc.arg('acquisition_date'),
    acquisition_cost_amount = sqlc.arg('acquisition_cost_amount'), salvage_amount = sqlc.arg('salvage_amount'),
    useful_life_months = sqlc.arg('useful_life_months'), depreciation_start_date = sqlc.arg('depreciation_start_date'),
    opening_accumulated_amount = sqlc.arg('opening_accumulated_amount'),
    opening_depreciated_months = sqlc.arg('opening_depreciated_months'), accumulated_amount = sqlc.arg('accumulated_amount'),
    payment_account_id = sqlc.narg('payment_account_id'), status = sqlc.arg('status'),
    acquisition_journal_id = sqlc.narg('acquisition_journal_id'), disposal_date = sqlc.narg('disposal_date'),
    disposal_proceeds_amount = sqlc.narg('disposal_proceeds_amount'), disposal_account_id = sqlc.narg('disposal_account_id'),
    disposal_reason = sqlc.arg('disposal_reason'), disposal_journal_id = sqlc.narg('disposal_journal_id'),
    note = sqlc.arg('note'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeleteFixedAsset :execrows
UPDATE fixed_assets
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND status = 'draft' AND deleted_at IS NULL;

-- name: DepreciableAssets :many
-- Aset aktif yang mulai disusutkan paling lambat periode ini dan belum punya baris penyusutan periode ini.
-- Dikunci berurutan id (Technical Guideline 7.2).
SELECT sqlc.embed(f), c.name AS class_name
FROM fixed_assets f
JOIN fixed_asset_classes c ON c.id = f.class_id
WHERE f.status = 'active' AND f.deleted_at IS NULL
  AND f.depreciation_start_date <= sqlc.arg('period_start')::date
  AND f.accumulated_amount < f.acquisition_cost_amount - f.salvage_amount
  AND NOT EXISTS (
      SELECT 1 FROM depreciation_lines d WHERE d.fixed_asset_id = f.id AND d.fiscal_period_id = sqlc.arg('fiscal_period_id'))
ORDER BY f.id
FOR UPDATE OF f;

-- name: AssetClassTotals :many
-- Harga perolehan & akumulasi aset aktif per kelas (rekonsiliasi cut-over).
SELECT a.code AS asset_account_code, acc.code AS accumulated_account_code,
       coalesce(sum(f.acquisition_cost_amount), 0)::numeric AS cost_amount,
       coalesce(sum(f.accumulated_amount), 0)::numeric AS accumulated_amount
FROM fixed_assets f
JOIN fixed_asset_classes c ON c.id = f.class_id
JOIN accounts a ON a.id = c.asset_account_id
JOIN accounts acc ON acc.id = c.accumulated_account_id
WHERE f.status = 'active' AND f.deleted_at IS NULL
GROUP BY a.code, acc.code
ORDER BY a.code;

-- name: GetDepreciationRunByPeriod :one
SELECT sqlc.embed(r), p.year, p.month FROM depreciation_runs r JOIN fiscal_periods p ON p.id = r.fiscal_period_id
WHERE r.fiscal_period_id = $1;

-- name: GetDepreciationRun :one
SELECT sqlc.embed(r), p.year, p.month FROM depreciation_runs r JOIN fiscal_periods p ON p.id = r.fiscal_period_id
WHERE r.id = $1;

-- name: ListDepreciationRuns :many
SELECT sqlc.embed(r), p.year, p.month FROM depreciation_runs r JOIN fiscal_periods p ON p.id = r.fiscal_period_id
ORDER BY p.year DESC, p.month DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountDepreciationRuns :one
SELECT count(*) FROM depreciation_runs;

-- name: UpsertDepreciationRun :one
INSERT INTO depreciation_runs (fiscal_period_id, created_by, updated_by)
VALUES (sqlc.arg('fiscal_period_id'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (fiscal_period_id) DO UPDATE
SET run_count = depreciation_runs.run_count + 1, last_run_at = now(), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = depreciation_runs.version + 1
RETURNING id;

-- name: RefreshDepreciationRunTotals :exec
UPDATE depreciation_runs r
SET asset_count = t.cnt, total_amount = t.total
FROM (SELECT count(*)::integer AS cnt, coalesce(sum(amount), 0)::numeric AS total
      FROM depreciation_lines WHERE depreciation_run_id = sqlc.arg('id')) t
WHERE r.id = sqlc.arg('id');

-- name: InsertDepreciationLine :exec
INSERT INTO depreciation_lines (id, depreciation_run_id, fixed_asset_id, fiscal_period_id, amount, accumulated_amount,
                                book_value_amount, journal_entry_id, created_by)
VALUES (sqlc.arg('id'), sqlc.arg('depreciation_run_id'), sqlc.arg('fixed_asset_id'), sqlc.arg('fiscal_period_id'), sqlc.arg('amount'),
        sqlc.arg('accumulated_amount'), sqlc.arg('book_value_amount'), sqlc.arg('journal_entry_id'), sqlc.arg('actor_id'));

-- name: SetAssetAccumulated :exec
UPDATE fixed_assets SET accumulated_amount = sqlc.arg('accumulated_amount'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id');

-- name: ListDepreciationLines :many
SELECT sqlc.embed(d), f.code AS asset_code, f.name AS asset_name, p.year, p.month, coalesce(e.number, '')::text AS journal_number
FROM depreciation_lines d
JOIN fixed_assets f ON f.id = d.fixed_asset_id
JOIN fiscal_periods p ON p.id = d.fiscal_period_id
JOIN journal_entries e ON e.id = d.journal_entry_id
WHERE (sqlc.narg('run_id')::uuid IS NULL OR d.depreciation_run_id = sqlc.narg('run_id'))
  AND (sqlc.narg('asset_id')::uuid IS NULL OR d.fixed_asset_id = sqlc.narg('asset_id'))
ORDER BY p.year DESC, p.month DESC, f.code;

-- name: ActiveAssetsWithoutDepreciation :one
-- Jumlah aset yang seharusnya disusutkan pada periode tetapi belum (langkah 6 checklist).
SELECT count(*) FROM fixed_assets f
WHERE f.status = 'active' AND f.deleted_at IS NULL
  AND f.depreciation_start_date <= sqlc.arg('period_start')::date
  AND f.accumulated_amount < f.acquisition_cost_amount - f.salvage_amount
  AND NOT EXISTS (
      SELECT 1 FROM depreciation_lines d WHERE d.fixed_asset_id = f.id AND d.fiscal_period_id = sqlc.arg('fiscal_period_id'));
