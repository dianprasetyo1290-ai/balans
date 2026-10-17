-- name: StockCountVarianceReport :many
-- Hasil stock count & selisih per baris (RPT-03): dokumen yang sudah dihitung (menunggu persetujuan, ditolak,
-- diposting, selesai) pada rentang tanggal.
SELECT c.id, coalesce(c.number, '')::text AS number, c.count_type, c.location_id, c.business_date, c.status,
       l.product_id, l.batch_id, l.system_qty, l.counted_qty, l.variance_qty, l.variance_amount, l.unit_cost, l.is_extra
FROM stock_counts c
JOIN stock_count_lines l ON l.stock_count_id = c.id
WHERE c.status IN ('pending_approval', 'rejected', 'posted', 'completed')
  AND c.business_date >= sqlc.arg('date_from')::date AND c.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('location_id')::uuid IS NULL OR c.location_id = sqlc.narg('location_id'))
  AND (sqlc.narg('product_id')::uuid IS NULL OR l.product_id = sqlc.narg('product_id'))
  AND (NOT sqlc.arg('variance_only')::boolean OR l.variance_qty <> 0)
ORDER BY c.business_date DESC, c.number, l.line_no
LIMIT 5000;
