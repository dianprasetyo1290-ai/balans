-- name: ListPeriods :many
SELECT * FROM fiscal_periods
WHERE (sqlc.narg('year')::integer IS NULL OR year = sqlc.narg('year'))
ORDER BY year DESC, month DESC;

-- name: EnsurePeriod :exec
INSERT INTO fiscal_periods (year, month, created_by, updated_by)
VALUES (sqlc.arg('year'), sqlc.arg('month'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (year, month) DO NOTHING;

-- name: GetPeriodShared :one
-- Dikunci FOR SHARE selama transaksi posting, sehingga tutup buku (FOR UPDATE) menunggu posting selesai.
SELECT * FROM fiscal_periods WHERE year = sqlc.arg('year') AND month = sqlc.arg('month') FOR SHARE;

-- name: GetPeriod :one
SELECT * FROM fiscal_periods WHERE year = sqlc.arg('year') AND month = sqlc.arg('month');

-- name: ClosedPeriodAfterExists :one
SELECT EXISTS (
    SELECT 1 FROM fiscal_periods WHERE status <> 'open' AND (year * 12 + month - 1) > sqlc.arg('period_index')::integer
)::boolean;
