-- name: UpsertMockPayment :one
INSERT INTO mock_payments (order_id, gross_amount)
VALUES ($1, $2)
ON CONFLICT (order_id) DO UPDATE SET updated_at = now()
RETURNING order_id, transaction_id, gross_amount, status, created_at, updated_at;

-- name: GetMockPayment :one
SELECT order_id, transaction_id, gross_amount, status, created_at, updated_at
FROM mock_payments
WHERE order_id = $1;

-- name: ListMockPayments :many
SELECT order_id, transaction_id, gross_amount, status, created_at, updated_at
FROM mock_payments
ORDER BY created_at DESC
LIMIT $1;

-- name: SetMockPaymentStatus :one
UPDATE mock_payments
SET status = $2, updated_at = now()
WHERE order_id = $1 AND status = 'pending'
RETURNING order_id, transaction_id, gross_amount, status, created_at, updated_at;
