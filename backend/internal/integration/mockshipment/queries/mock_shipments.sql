-- name: UpsertMockShipment :one
INSERT INTO mock_shipments (reference_id, tracking_number, courier_code, shipping_cost)
VALUES ($1, $2, $3, $4)
ON CONFLICT (reference_id) DO UPDATE SET updated_at = now()
RETURNING reference_id, tracking_number, courier_code, shipping_cost, status, created_at, updated_at;

-- name: GetMockShipment :one
SELECT reference_id, tracking_number, courier_code, shipping_cost, status, created_at, updated_at
FROM mock_shipments
WHERE reference_id = $1;

-- name: ListMockShipments :many
SELECT reference_id, tracking_number, courier_code, shipping_cost, status, created_at, updated_at
FROM mock_shipments
ORDER BY created_at DESC
LIMIT $1;

-- name: SetMockShipmentStatus :one
UPDATE mock_shipments
SET status = $2, updated_at = now()
WHERE reference_id = $1
RETURNING reference_id, tracking_number, courier_code, shipping_cost, status, created_at, updated_at;
