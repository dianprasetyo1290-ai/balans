-- name: ListCouriers :many
SELECT * FROM couriers
WHERE deleted_at IS NULL AND (sqlc.narg('active')::boolean IS NULL OR is_active = sqlc.narg('active')::boolean)
ORDER BY is_internal, name;

-- name: GetCourier :one
SELECT * FROM couriers WHERE id = $1;

-- name: InsertCourier :one
INSERT INTO couriers (code, name, is_internal, is_active, created_by, updated_by)
VALUES (sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('is_internal'), sqlc.arg('is_active'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: UpdateCourier :one
UPDATE couriers
SET name = sqlc.arg('name'), is_internal = sqlc.arg('is_internal'), is_active = sqlc.arg('is_active'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: InsertDeliveryOrder :one
INSERT INTO delivery_orders (delivery_type, source_type, source_id, source_label, sales_order_id, partner_id, customer_id,
                             recipient_name, recipient_phone, ship_to_address, location_id, business_date, status, note,
                             created_by, updated_by)
VALUES (sqlc.arg('delivery_type'), sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('source_label'),
        sqlc.narg('sales_order_id'), sqlc.narg('partner_id'), sqlc.narg('customer_id'), sqlc.arg('recipient_name'),
        sqlc.arg('recipient_phone'), sqlc.arg('ship_to_address'), sqlc.arg('location_id'), sqlc.arg('business_date'),
        sqlc.arg('status'), sqlc.arg('note'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: InsertDeliveryOrderLine :one
INSERT INTO delivery_order_lines (delivery_order_id, line_no, source_line_id, sales_order_line_id, product_id, qty, batches)
VALUES (sqlc.arg('delivery_order_id'), sqlc.arg('line_no'), sqlc.narg('source_line_id'), sqlc.narg('sales_order_line_id'),
        sqlc.arg('product_id'), sqlc.arg('qty'), sqlc.arg('batches'))
RETURNING id;

-- name: GetDeliveryOrder :one
SELECT * FROM delivery_orders WHERE id = $1;

-- name: GetDeliveryOrderForUpdate :one
SELECT * FROM delivery_orders WHERE id = $1 FOR UPDATE;

-- name: ListDeliveryOrderLines :many
SELECT * FROM delivery_order_lines WHERE delivery_order_id = $1 ORDER BY line_no;

-- name: ListDeliveryOrdersBySource :many
SELECT * FROM delivery_orders WHERE source_type = sqlc.arg('source_type') AND source_id = sqlc.arg('source_id')
ORDER BY created_at, id;

-- name: UpdateDeliveryOrder :one
UPDATE delivery_orders
SET number = sqlc.narg('number'), status = sqlc.arg('status'), outbound_id = sqlc.narg('outbound_id'),
    outbound_completed_at = sqlc.narg('outbound_completed_at'), released_by = sqlc.narg('released_by'),
    released_by_name = sqlc.arg('released_by_name'), released_at = sqlc.narg('released_at'),
    shipped_at = sqlc.narg('shipped_at'), delivered_at = sqlc.narg('delivered_at'), cancelled_at = sqlc.narg('cancelled_at'),
    cancel_reason = sqlc.arg('cancel_reason'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: UpdateDeliveryOrderLineShipped :exec
UPDATE delivery_order_lines SET shipped_qty = sqlc.arg('shipped_qty') WHERE id = sqlc.arg('id');

-- name: ListDeliveryOrders :many
SELECT d.*, s.tracking_number, s.marketplace_awb, c.name AS courier_name,
       (SELECT count(*) FROM delivery_order_lines l WHERE l.delivery_order_id = d.id)::bigint AS line_count
FROM delivery_orders d
LEFT JOIN shipments s ON s.delivery_order_id = d.id
LEFT JOIN couriers c ON c.id = s.courier_id
WHERE (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('delivery_type')::text IS NULL OR d.delivery_type = sqlc.narg('delivery_type')::text)
  AND (sqlc.narg('source_id')::uuid IS NULL OR d.source_id = sqlc.narg('source_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR d.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.source_label ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.tracking_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.marketplace_awb ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountDeliveryOrders :one
SELECT count(*) FROM delivery_orders d
LEFT JOIN shipments s ON s.delivery_order_id = d.id
WHERE (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('delivery_type')::text IS NULL OR d.delivery_type = sqlc.narg('delivery_type')::text)
  AND (sqlc.narg('source_id')::uuid IS NULL OR d.source_id = sqlc.narg('source_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR d.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.source_label ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.tracking_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.marketplace_awb ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: InsertShipment :one
INSERT INTO shipments (delivery_order_id, courier_id, internal_courier_user_id, internal_courier_name, service_code,
                       tracking_number, marketplace_awb, shipping_cost_amount, package_count, status, shipped_at,
                       created_by, updated_by)
VALUES (sqlc.arg('delivery_order_id'), sqlc.arg('courier_id'), sqlc.narg('internal_courier_user_id'),
        sqlc.arg('internal_courier_name'), sqlc.arg('service_code'), sqlc.arg('tracking_number'), sqlc.arg('marketplace_awb'),
        sqlc.arg('shipping_cost_amount'), sqlc.arg('package_count'), sqlc.arg('status'), sqlc.arg('shipped_at'),
        sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetShipmentByDeliveryOrder :one
SELECT * FROM shipments WHERE delivery_order_id = $1;

-- name: GetShipment :one
SELECT * FROM shipments WHERE id = $1;

-- name: UpdateShipment :one
UPDATE shipments
SET status = sqlc.arg('status'), delivered_at = sqlc.narg('delivered_at'), received_by_name = sqlc.arg('received_by_name'),
    delivery_note = sqlc.arg('delivery_note'), proof_file_ids = sqlc.arg('proof_file_ids')::uuid[],
    confirmed_by = sqlc.narg('confirmed_by'), confirmed_by_name = sqlc.arg('confirmed_by_name'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version')
RETURNING *;

-- name: InsertShipmentEvent :exec
INSERT INTO shipment_events (shipment_id, status, occurred_at, source, note, created_by, created_by_name)
VALUES (sqlc.arg('shipment_id'), sqlc.arg('status'), sqlc.arg('occurred_at'), sqlc.arg('source'), sqlc.arg('note'),
        sqlc.arg('actor_id'), sqlc.arg('actor_name'));

-- name: ListShipmentEvents :many
SELECT * FROM shipment_events WHERE shipment_id = $1 ORDER BY occurred_at, id;

-- name: ListShipments :many
SELECT s.*, d.number AS delivery_order_number, d.recipient_name, d.source_label, d.delivery_type, c.name AS courier_name
FROM shipments s
JOIN delivery_orders d ON d.id = s.delivery_order_id
JOIN couriers c ON c.id = s.courier_id
WHERE (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status')::text)
  AND (sqlc.narg('courier_user_id')::uuid IS NULL OR s.internal_courier_user_id = sqlc.narg('courier_user_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR d.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.tracking_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.marketplace_awb ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY s.shipped_at DESC, s.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountShipments :one
SELECT count(*) FROM shipments s
JOIN delivery_orders d ON d.id = s.delivery_order_id
WHERE (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status')::text)
  AND (sqlc.narg('courier_user_id')::uuid IS NULL OR s.internal_courier_user_id = sqlc.narg('courier_user_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR d.number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR d.recipient_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.tracking_number ILIKE '%' || sqlc.narg('search')::text || '%'
       OR s.marketplace_awb ILIKE '%' || sqlc.narg('search')::text || '%');
