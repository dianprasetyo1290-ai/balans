-- name: LocationFlows :many
-- Mutasi stok satu lokasi per produk untuk rekonsiliasi (konsinyasi): saldo awal sebelum date_from, masuk dan keluar
-- dalam periode. Movement dijumlahkan per posting × produk agar pindah status di lokasi yang sama (mis. Dalam
-- Pengiriman → Tersedia saat diterima, Tersedia → Reserved) tidak dihitung sebagai masuk/keluar. Keluar dari
-- source_type yang dikecualikan (dokumen rekonsiliasi itu sendiri) dihitung terpisah.
WITH m AS (
    SELECT posting_id, product_id, source_type, business_date, sum(qty) AS qty
    FROM stock_movements
    WHERE location_id = sqlc.arg('location_id')
      AND stock_status = ANY(sqlc.arg('statuses')::text[])
      AND business_date <= sqlc.arg('date_to')::date
    GROUP BY posting_id, product_id, source_type, business_date
)
SELECT product_id,
       coalesce(sum(qty) FILTER (WHERE business_date < sqlc.arg('date_from')::date), 0)::numeric AS opening_qty,
       coalesce(sum(qty) FILTER (WHERE business_date >= sqlc.arg('date_from')::date AND qty > 0), 0)::numeric AS in_qty,
       coalesce(-sum(qty) FILTER (WHERE business_date >= sqlc.arg('date_from')::date AND qty < 0
                                  AND NOT (source_type = ANY(sqlc.arg('exclude_sources')::text[]))), 0)::numeric AS out_qty,
       coalesce(-sum(qty) FILTER (WHERE business_date >= sqlc.arg('date_from')::date AND qty < 0
                                  AND source_type = ANY(sqlc.arg('exclude_sources')::text[])), 0)::numeric AS excluded_out_qty
FROM m
GROUP BY product_id
ORDER BY product_id;
