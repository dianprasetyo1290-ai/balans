-- +goose Up
-- Dipakai adapter mock Midtrans (Technical Guideline 5.10). Tetap kosong di production.
CREATE TABLE mock_payments (
    order_id       text        NOT NULL,
    transaction_id uuid        NOT NULL DEFAULT uuidv7(),
    gross_amount   numeric(18,2) NOT NULL,
    status         text        NOT NULL DEFAULT 'pending',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_mock_payments PRIMARY KEY (order_id),
    CONSTRAINT ck_mock_payments_status CHECK (status IN ('pending', 'settlement', 'deny', 'expire')),
    CONSTRAINT ck_mock_payments_gross_amount_positive CHECK (gross_amount > 0)
);

-- +goose Down
DROP TABLE mock_payments;
