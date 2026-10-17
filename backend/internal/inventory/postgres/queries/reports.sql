-- Laporan persediaan (RPT-01, 02, 04, 08, 09, 19) dari read model rm_stock_values dan tabel modul inventory.

-- name: StockPositionReport :many
-- Posisi & nilai stok per produk × lokasi × batch (RPT-01). UUID nol = produk tanpa batch.
SELECT v.product_id, v.location_id, v.batch_id, v.qty, v.value_amount,
       coalesce(v.first_in_date, DATE '1900-01-01')::date AS first_in_date,
       coalesce(b.batch_number, '')::text AS batch_number, b.expiry_date
FROM rm_stock_values v
LEFT JOIN batches b ON b.id = v.batch_id
WHERE (v.qty <> 0 OR v.value_amount <> 0)
  AND (cardinality(sqlc.arg('location_ids')::uuid[]) = 0 OR v.location_id = ANY(sqlc.arg('location_ids')::uuid[]))
  AND (sqlc.narg('product_id')::uuid IS NULL OR v.product_id = sqlc.narg('product_id'))
  AND (sqlc.narg('batch_id')::uuid IS NULL OR v.batch_id = sqlc.narg('batch_id'))
ORDER BY v.product_id, v.location_id, b.expiry_date NULLS LAST, b.batch_number;

-- name: StockMovementSummary :many
-- Mutasi per produk × lokasi pada periode (RPT-02): saldo akhir = saldo kini (read model) − mutasi setelah
-- date_to; saldo awal = saldo akhir − mutasi periode. Hanya mutasi sejak date_from yang dibaca.
WITH cur AS (
    SELECT product_id, location_id, sum(qty) AS qty, sum(value_amount) AS value_amount
    FROM rm_stock_values GROUP BY product_id, location_id
),
mv AS (
    SELECT product_id, location_id,
           coalesce(sum(qty) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND movement_type = 'receive'), 0) AS receive_qty,
           coalesce(sum(qty) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND movement_type = 'issue'), 0) AS issue_qty,
           coalesce(sum(qty) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND movement_type = 'move_in'), 0) AS move_in_qty,
           coalesce(sum(qty) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND movement_type = 'move_out'), 0) AS move_out_qty,
           coalesce(sum(cost_amount) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND qty > 0), 0) AS in_value,
           coalesce(sum(cost_amount) FILTER (WHERE business_date <= sqlc.arg('date_to')::date AND qty < 0), 0) AS out_value,
           coalesce(sum(qty) FILTER (WHERE business_date > sqlc.arg('date_to')::date), 0) AS after_qty,
           coalesce(sum(cost_amount) FILTER (WHERE business_date > sqlc.arg('date_to')::date), 0) AS after_value
    FROM stock_movements
    WHERE business_date >= sqlc.arg('date_from')::date
      AND (cardinality(sqlc.arg('location_ids')::uuid[]) = 0 OR location_id = ANY(sqlc.arg('location_ids')::uuid[]))
      AND (sqlc.narg('product_id')::uuid IS NULL OR product_id = sqlc.narg('product_id'))
    GROUP BY product_id, location_id
)
SELECT coalesce(c.product_id, m.product_id)::uuid AS product_id, coalesce(c.location_id, m.location_id)::uuid AS location_id,
       coalesce(m.receive_qty, 0)::numeric AS receive_qty, coalesce(m.issue_qty, 0)::numeric AS issue_qty,
       coalesce(m.move_in_qty, 0)::numeric AS move_in_qty, coalesce(m.move_out_qty, 0)::numeric AS move_out_qty,
       coalesce(m.in_value, 0)::numeric AS in_value, coalesce(m.out_value, 0)::numeric AS out_value,
       (coalesce(c.qty, 0) - coalesce(m.after_qty, 0))::numeric AS closing_qty,
       (coalesce(c.value_amount, 0) - coalesce(m.after_value, 0))::numeric AS closing_value
FROM cur c
FULL JOIN mv m ON m.product_id = c.product_id AND m.location_id = c.location_id
WHERE (cardinality(sqlc.arg('location_ids')::uuid[]) = 0 OR coalesce(c.location_id, m.location_id) = ANY(sqlc.arg('location_ids')::uuid[]))
  AND (sqlc.narg('product_id')::uuid IS NULL OR coalesce(c.product_id, m.product_id) = sqlc.narg('product_id'))
  AND (m.product_id IS NOT NULL OR c.qty <> 0)
ORDER BY 1, 2;

-- name: InternalUsageReport :many
-- Pemakaian internal diposting per dokumen × produk (RPT-08).
SELECT u.id, coalesce(u.number, '')::text AS number, u.business_date, u.purpose, u.division_id, u.campaign_id,
       l.product_id, l.qty, l.returned_qty, l.consumed_qty, l.cost_amount
FROM internal_usages u
JOIN internal_usage_lines l ON l.internal_usage_id = u.id
WHERE u.status = 'posted'
  AND u.business_date >= sqlc.arg('date_from')::date AND u.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('purpose')::text IS NULL OR u.purpose = sqlc.narg('purpose'))
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR u.campaign_id = sqlc.narg('campaign_id'))
  AND (sqlc.narg('division_id')::uuid IS NULL OR u.division_id = sqlc.narg('division_id'))
ORDER BY u.business_date, u.number, l.line_no;

-- name: ReturnDispositionReport :many
-- Barang retur per alasan & tindak lanjut (disposisi) dari Disposisi Retur diposting (RPT-09).
SELECT d.id, coalesce(d.number, '')::text AS number, d.business_date, d.location_id, l.product_id, l.reason_id,
       l.disposition, l.qty, l.cost_amount, l.target_type, l.qc_note
FROM return_dispositions d
JOIN return_disposition_lines l ON l.return_disposition_id = d.id
WHERE d.status = 'posted'
  AND d.business_date >= sqlc.arg('date_from')::date AND d.business_date <= sqlc.arg('date_to')::date
ORDER BY d.business_date, d.number, l.line_no;
