-- Laporan penjualan & margin dari read model rm_sales_lines (TG 5.17), konsinyasi, dan settlement marketplace.

-- name: SalesReport :many
-- Ringkasan penjualan & HPP per kunci pengelompokan (channel, sub_channel, product, month, day).
SELECT (CASE sqlc.arg('group_by')::text
            WHEN 'channel' THEN r.channel_id::text
            WHEN 'sub_channel' THEN r.sub_channel_id::text
            WHEN 'product' THEN r.product_id::text
            WHEN 'month' THEN to_char(r.business_date, 'YYYY-MM')
            ELSE to_char(r.business_date, 'YYYY-MM-DD') END)::text AS group_key,
       sum(r.qty)::numeric AS qty, sum(r.gross_amount)::numeric AS gross_amount,
       sum(r.discount_amount)::numeric AS discount_amount, sum(r.net_amount)::numeric AS net_amount,
       sum(r.cogs_qty)::numeric AS cogs_qty, sum(r.cogs_amount)::numeric AS cogs_amount
FROM rm_sales_lines r
WHERE r.business_date >= sqlc.arg('date_from')::date AND r.business_date <= sqlc.arg('date_to')::date
  AND (sqlc.narg('channel_id')::uuid IS NULL OR r.channel_id = sqlc.narg('channel_id'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR r.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (sqlc.narg('product_id')::uuid IS NULL OR r.product_id = sqlc.narg('product_id'))
  AND (sqlc.narg('order_type')::text IS NULL OR r.order_type = sqlc.narg('order_type'))
GROUP BY 1
ORDER BY 1;

-- name: ConsignmentReport :many
-- Invoice konsinyasi & selisih per mitra pada rentang tanggal (RPT-06); kosong bila rekonsiliasi belum ada.
SELECT i.party_id, i.party_code, i.party_name, i.invoice_type,
       count(*)::bigint AS invoice_count,
       sum(i.gross_amount)::numeric AS gross_amount, sum(i.discount_amount)::numeric AS discount_amount,
       sum(i.total_amount)::numeric AS total_amount, sum(i.paid_amount)::numeric AS paid_amount
FROM invoices i
WHERE i.invoice_type IN ('consignment', 'variance') AND i.status <> 'cancelled' AND i.deleted_at IS NULL
  AND i.invoice_date >= sqlc.arg('date_from')::date AND i.invoice_date <= sqlc.arg('date_to')::date
GROUP BY i.party_id, i.party_code, i.party_name, i.invoice_type
ORDER BY i.party_name, i.invoice_type;

-- name: MarketplaceOrderForSettlement :one
-- Order marketplace per nomor order marketplace, dengan nilai piutang (A-10) dan jumlah yang sudah masuk
-- settlement draft/diposting.
SELECT o.id, coalesce(o.number, '')::text AS number, o.channel_id, o.sub_channel_id, o.total_amount, o.status,
       EXISTS (SELECT 1 FROM sales_shipments s WHERE s.sales_order_id = o.id)::boolean AS handed_over,
       coalesce((SELECT sum(l.settled_amount) FROM marketplace_settlement_lines l
                 JOIN marketplace_settlements m ON m.id = l.settlement_id
                 WHERE l.sales_order_id = o.id AND m.status <> 'cancelled'), 0)::numeric AS settled_amount
FROM sales_orders o
WHERE o.order_type = 'marketplace' AND o.sub_channel_id = sqlc.arg('sub_channel_id')
  AND o.external_order_id = sqlc.arg('external_order_id')
FOR UPDATE OF o;

-- name: GetSettlementByPayout :one
SELECT * FROM marketplace_settlements
WHERE sub_channel_id = sqlc.arg('sub_channel_id') AND payout_reference = sqlc.arg('payout_reference')
FOR UPDATE;

-- name: InsertSettlement :one
INSERT INTO marketplace_settlements (
    channel_id, sub_channel_id, marketplace_code, payout_reference, settlement_date, bank_account_code,
    created_by, updated_by)
VALUES (
    sqlc.arg('channel_id'), sqlc.arg('sub_channel_id'), sqlc.arg('marketplace_code'), sqlc.arg('payout_reference'),
    sqlc.arg('settlement_date'), sqlc.arg('bank_account_code'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: InsertSettlementLine :exec
INSERT INTO marketplace_settlement_lines (
    settlement_id, line_no, sales_order_id, external_order_id, order_number, receivable_amount, fee_amount,
    promo_amount, net_amount, settled_amount, import_row_key)
VALUES (
    sqlc.arg('settlement_id'),
    (SELECT coalesce(max(line_no), 0) + 1 FROM marketplace_settlement_lines WHERE settlement_id = sqlc.arg('settlement_id')),
    sqlc.arg('sales_order_id'), sqlc.arg('external_order_id'), sqlc.arg('order_number'), sqlc.arg('receivable_amount'),
    sqlc.arg('fee_amount'), sqlc.arg('promo_amount'), sqlc.arg('net_amount'), sqlc.arg('settled_amount'),
    sqlc.narg('import_row_key'));

-- name: RecalcSettlement :exec
UPDATE marketplace_settlements m
SET line_count = t.line_count, fee_amount = t.fee, promo_amount = t.promo, net_amount = t.net,
    settled_amount = t.fee + t.promo + t.net, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = m.version + 1
FROM (SELECT count(*)::integer AS line_count, coalesce(sum(fee_amount), 0) AS fee,
             coalesce(sum(promo_amount), 0) AS promo, coalesce(sum(net_amount), 0) AS net
      FROM marketplace_settlement_lines WHERE settlement_id = sqlc.arg('id')) t
WHERE m.id = sqlc.arg('id');

-- name: GetSettlement :one
SELECT * FROM marketplace_settlements WHERE id = $1;

-- name: GetSettlementForUpdate :one
SELECT * FROM marketplace_settlements WHERE id = $1 FOR UPDATE;

-- name: ListSettlementLines :many
SELECT l.*, o.total_amount AS order_total,
       coalesce((SELECT sum(x.settled_amount) FROM marketplace_settlement_lines x
                 JOIN marketplace_settlements m ON m.id = x.settlement_id
                 WHERE x.sales_order_id = l.sales_order_id AND m.status = 'posted'), 0)::numeric AS posted_settled
FROM marketplace_settlement_lines l
JOIN sales_orders o ON o.id = l.sales_order_id
WHERE l.settlement_id = $1
ORDER BY l.line_no;

-- name: ListSettlements :many
SELECT * FROM marketplace_settlements
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR sub_channel_id = sqlc.narg('sub_channel_id'))
ORDER BY settlement_date DESC, created_at DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountSettlements :one
SELECT count(*) FROM marketplace_settlements
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR sub_channel_id = sqlc.narg('sub_channel_id'));

-- name: UpdateSettlementStatus :execrows
UPDATE marketplace_settlements
SET status = sqlc.arg('status'), journal_entry_id = sqlc.narg('journal_entry_id'),
    posted_at = sqlc.narg('posted_at'), posted_by = sqlc.narg('posted_by'), cancel_reason = sqlc.arg('cancel_reason'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND status = 'draft';

-- name: ListMarketplaceReceivables :many
-- Sisa piutang marketplace per order yang sudah diserahkan ke kurir (A-10).
SELECT o.id, coalesce(o.number, '')::text AS number, o.external_order_id, o.sub_channel_id, o.business_date,
       o.total_amount, o.party_name,
       (SELECT min(s.business_date) FROM sales_shipments s WHERE s.sales_order_id = o.id)::date AS handed_over_date,
       coalesce(st.posted, 0)::numeric AS settled_amount, coalesce(st.draft, 0)::numeric AS draft_amount,
       (o.total_amount - coalesce(st.posted, 0))::numeric AS outstanding_amount
FROM sales_orders o
LEFT JOIN (
    SELECT l.sales_order_id,
           sum(l.settled_amount) FILTER (WHERE m.status = 'posted') AS posted,
           sum(l.settled_amount) FILTER (WHERE m.status = 'draft') AS draft
    FROM marketplace_settlement_lines l JOIN marketplace_settlements m ON m.id = l.settlement_id
    GROUP BY l.sales_order_id) st ON st.sales_order_id = o.id
WHERE o.order_type = 'marketplace'
  AND EXISTS (SELECT 1 FROM sales_shipments s WHERE s.sales_order_id = o.id)
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR o.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (NOT sqlc.arg('open_only')::boolean OR o.total_amount - coalesce(st.posted, 0) > 0)
  AND (sqlc.narg('search')::text IS NULL OR o.external_order_id ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY o.business_date, o.number
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: MarketplaceReceivableTotals :one
SELECT count(*)::bigint AS total_rows,
       coalesce(sum(o.total_amount), 0)::numeric AS receivable_amount,
       coalesce(sum(st.posted), 0)::numeric AS settled_amount,
       coalesce(sum(o.total_amount - coalesce(st.posted, 0)), 0)::numeric AS outstanding_amount
FROM sales_orders o
LEFT JOIN (
    SELECT l.sales_order_id, sum(l.settled_amount) FILTER (WHERE m.status = 'posted') AS posted
    FROM marketplace_settlement_lines l JOIN marketplace_settlements m ON m.id = l.settlement_id
    GROUP BY l.sales_order_id) st ON st.sales_order_id = o.id
WHERE o.order_type = 'marketplace'
  AND EXISTS (SELECT 1 FROM sales_shipments s WHERE s.sales_order_id = o.id)
  AND (sqlc.narg('sub_channel_id')::uuid IS NULL OR o.sub_channel_id = sqlc.narg('sub_channel_id'))
  AND (NOT sqlc.arg('open_only')::boolean OR o.total_amount - coalesce(st.posted, 0) > 0)
  AND (sqlc.narg('search')::text IS NULL OR o.external_order_id ILIKE '%' || sqlc.narg('search')::text || '%'
       OR o.number ILIKE '%' || sqlc.narg('search')::text || '%');
