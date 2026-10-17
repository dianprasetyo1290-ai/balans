-- +goose Up
-- Read model laporan R1.4 (Technical Guideline 5.17, 2.3 butir 2): setiap modul memiliki materialized view atas
-- tabelnya sendiri; semuanya di-refresh job `accounting.refresh_read_models` tiap 15 menit (Lampiran B) lewat
-- refresher yang didaftarkan modul pemilik saat wiring. Laporan tidak menjumlah ulang ledger saat dibuka dan
-- tidak membaca tabel modul lain. Dimensi kosong disimpan sebagai UUID nol agar unique index bisa dipakai
-- REFRESH MATERIALIZED VIEW CONCURRENTLY.

-- accounting: waktu refresh terakhir per read model (tampil di laporan sebagai "Data per …").
CREATE TABLE read_model_refreshes (
    name         text        NOT NULL,
    refreshed_at timestamptz NOT NULL,
    duration_ms  integer     NOT NULL DEFAULT 0,
    CONSTRAINT pk_read_model_refreshes PRIMARY KEY (name),
    CONSTRAINT ck_read_model_refreshes_duration CHECK (duration_ms >= 0)
);

-- accounting: mutasi jurnal diposting per tanggal × akun × dimensi (account period balances, TG 5.17).
-- Dipakai Neraca, Laba Rugi, Arus Kas, laba rugi per dimensi, laporan pajak, dan dashboard eksekutif.
CREATE MATERIALIZED VIEW rm_gl_daily AS
SELECT e.business_date,
       l.account_id,
       coalesce(l.channel_id, '00000000-0000-0000-0000-000000000000'::uuid)     AS channel_id,
       coalesce(l.sub_channel_id, '00000000-0000-0000-0000-000000000000'::uuid) AS sub_channel_id,
       coalesce(l.division_id, '00000000-0000-0000-0000-000000000000'::uuid)    AS division_id,
       coalesce(l.campaign_id, '00000000-0000-0000-0000-000000000000'::uuid)    AS campaign_id,
       sum(l.debit_amount)::numeric(18,2)  AS debit_amount,
       sum(l.credit_amount)::numeric(18,2) AS credit_amount,
       count(*)::integer                   AS line_count
FROM journal_lines l
JOIN journal_entries e ON e.id = l.journal_entry_id
WHERE e.status = 'posted'
GROUP BY e.business_date, l.account_id, l.channel_id, l.sub_channel_id, l.division_id, l.campaign_id;
CREATE UNIQUE INDEX uq_rm_gl_daily_key ON rm_gl_daily (
    business_date, account_id, channel_id, sub_channel_id, division_id, campaign_id);
CREATE INDEX idx_rm_gl_daily_account_date ON rm_gl_daily (account_id, business_date);

-- sales: penjualan & HPP per tanggal pengakuan × jenis pesanan × channel × sub-channel × produk (RPT-05, RPT-15).
-- Pendapatan diakui sesuai jurnal: tanggal invoice (A-05, invoice tidak batal) atau tanggal serah kurir
-- marketplace (A-10); invoice konsinyasi/selisih dari baris invoice. Diskon dokumen dialokasikan ke baris secara
-- proporsional dengan sisa pembulatan di baris terakhir sehingga jumlahnya sama dengan jurnal 4102. HPP dari
-- pengiriman (A-06/A-10) per baris pesanan.
CREATE MATERIALIZED VIEW rm_sales_lines AS
WITH order_lines AS (
    SELECT l.sales_order_id, l.id AS line_id, l.product_id, l.qty, l.gross_amount, l.discount_amount,
           l.line_total_amount,
           o.document_discount_amount,
           sum(l.line_total_amount) OVER w AS cum_total,
           sum(l.line_total_amount) OVER (PARTITION BY l.sales_order_id) AS subtotal
    FROM sales_order_lines l
    JOIN sales_orders o ON o.id = l.sales_order_id
    WINDOW w AS (PARTITION BY l.sales_order_id ORDER BY l.line_no ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
),
allocated AS (
    SELECT ol.*,
           CASE WHEN ol.subtotal = 0 THEN 0::numeric ELSE
               round(ol.document_discount_amount * ol.cum_total / ol.subtotal, 2)
               - round(ol.document_discount_amount * (ol.cum_total - ol.line_total_amount) / ol.subtotal, 2)
           END AS doc_discount
    FROM order_lines ol
),
recognized AS (
    SELECT i.sales_order_id, i.invoice_date AS business_date
    FROM invoices i
    WHERE i.invoice_type = 'sales' AND i.sales_order_id IS NOT NULL AND i.status <> 'cancelled' AND i.deleted_at IS NULL
    UNION ALL
    SELECT s.sales_order_id, min(s.business_date)
    FROM sales_shipments s
    JOIN sales_orders o ON o.id = s.sales_order_id AND o.order_type = 'marketplace'
    GROUP BY s.sales_order_id
),
revenue AS (
    SELECT r.business_date, o.order_type, o.channel_id, o.sub_channel_id, a.product_id,
           a.qty, a.gross_amount, a.discount_amount + a.doc_discount AS discount_amount,
           a.line_total_amount - a.doc_discount AS net_amount, 0::numeric AS cogs_qty, 0::numeric AS cogs_amount
    FROM recognized r
    JOIN sales_orders o ON o.id = r.sales_order_id
    JOIN allocated a ON a.sales_order_id = o.id
    UNION ALL
    SELECT i.invoice_date, i.invoice_type, i.channel_id, i.sub_channel_id,
           coalesce(il.product_id, '00000000-0000-0000-0000-000000000000'::uuid),
           il.qty, round(il.qty * il.unit_price_amount, 2), il.discount_amount, il.line_total_amount, 0, 0
    FROM invoices i
    JOIN invoice_lines il ON il.invoice_id = i.id
    WHERE i.invoice_type IN ('consignment', 'variance') AND i.status <> 'cancelled' AND i.deleted_at IS NULL
),
cogs AS (
    SELECT s.business_date, o.order_type, o.channel_id, o.sub_channel_id, ol.product_id,
           0::numeric AS qty, 0::numeric AS gross_amount, 0::numeric AS discount_amount, 0::numeric AS net_amount,
           sl.qty AS cogs_qty, sl.cost_amount AS cogs_amount
    FROM sales_shipment_lines sl
    JOIN sales_shipments s ON s.id = sl.shipment_id
    JOIN sales_orders o ON o.id = s.sales_order_id
    JOIN sales_order_allocations al ON al.id = sl.allocation_id
    JOIN sales_order_lines ol ON ol.id = al.sales_order_line_id
)
SELECT business_date, order_type, channel_id, sub_channel_id, product_id,
       sum(qty)::numeric(18,3)             AS qty,
       sum(gross_amount)::numeric(18,2)    AS gross_amount,
       sum(discount_amount)::numeric(18,2) AS discount_amount,
       sum(net_amount)::numeric(18,2)      AS net_amount,
       sum(cogs_qty)::numeric(18,3)        AS cogs_qty,
       sum(cogs_amount)::numeric(18,2)     AS cogs_amount
FROM (SELECT * FROM revenue UNION ALL SELECT * FROM cogs) x
GROUP BY business_date, order_type, channel_id, sub_channel_id, product_id;
CREATE UNIQUE INDEX uq_rm_sales_lines_key ON rm_sales_lines (
    business_date, order_type, channel_id, sub_channel_id, product_id);
CREATE INDEX idx_rm_sales_lines_product ON rm_sales_lines (product_id, business_date);

-- inventory: posisi & nilai stok per produk × lokasi × batch (RPT-01, dashboard). Nilai = Σ cost_amount mutasi
-- (FIFO, termasuk pindah lokasi) dikurangi selisih recost per lokasi, sehingga jumlah per jenis lokasi sama
-- dengan saldo akun persediaan 13xx.
CREATE MATERIALIZED VIEW rm_stock_values AS
WITH mv AS (
    SELECT product_id, location_id, coalesce(batch_id, '00000000-0000-0000-0000-000000000000'::uuid) AS batch_id,
           sum(qty) AS qty, sum(cost_amount) AS value_amount,
           min(business_date) FILTER (WHERE movement_type IN ('receive', 'move_in')) AS first_in_date
    FROM stock_movements
    GROUP BY product_id, location_id, batch_id
),
rc AS (
    SELECT m.product_id, r.location_id, coalesce(m.batch_id, '00000000-0000-0000-0000-000000000000'::uuid) AS batch_id,
           sum(r.difference) AS difference
    FROM recost_lines r
    JOIN stock_movements m ON m.id = r.stock_movement_id
    GROUP BY m.product_id, r.location_id, m.batch_id
)
SELECT coalesce(mv.product_id, rc.product_id)   AS product_id,
       coalesce(mv.location_id, rc.location_id) AS location_id,
       coalesce(mv.batch_id, rc.batch_id)       AS batch_id,
       coalesce(mv.qty, 0)::numeric(18,3)       AS qty,
       (coalesce(mv.value_amount, 0) - coalesce(rc.difference, 0))::numeric(18,2) AS value_amount,
       mv.first_in_date
FROM mv
FULL JOIN rc ON rc.product_id = mv.product_id AND rc.location_id = mv.location_id AND rc.batch_id = mv.batch_id;
CREATE UNIQUE INDEX uq_rm_stock_values_key ON rm_stock_values (product_id, location_id, batch_id);
CREATE INDEX idx_rm_stock_values_location ON rm_stock_values (location_id);

-- +goose Down
DROP MATERIALIZED VIEW rm_stock_values;
DROP MATERIALIZED VIEW rm_sales_lines;
DROP MATERIALIZED VIEW rm_gl_daily;
DROP TABLE read_model_refreshes;
