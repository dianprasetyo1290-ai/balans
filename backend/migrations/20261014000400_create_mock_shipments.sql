-- +goose Up
-- Dipakai adapter mock Biteship (Technical Guideline 5.10). Tetap kosong di production.
CREATE TABLE mock_shipments (
    reference_id    text        NOT NULL,
    tracking_number text        NOT NULL,
    courier_code    text        NOT NULL,
    shipping_cost   numeric(18,2) NOT NULL,
    status          text        NOT NULL DEFAULT 'confirmed',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_mock_shipments PRIMARY KEY (reference_id),
    CONSTRAINT uq_mock_shipments_tracking_number UNIQUE (tracking_number),
    CONSTRAINT ck_mock_shipments_status CHECK (status IN ('confirmed', 'picked', 'dropping_off', 'delivered', 'returned', 'cancelled'))
);

-- +goose Down
DROP TABLE mock_shipments;
