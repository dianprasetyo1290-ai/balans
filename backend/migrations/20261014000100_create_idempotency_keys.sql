-- +goose Up
CREATE TABLE idempotency_keys (
    user_id        uuid        NOT NULL,
    key            uuid        NOT NULL,
    request_method text        NOT NULL,
    request_path   text        NOT NULL,
    request_hash   bytea       NOT NULL,
    status         text        NOT NULL,
    response_code  integer,
    response_body  jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    CONSTRAINT pk_idempotency_keys PRIMARY KEY (user_id, key),
    CONSTRAINT ck_idempotency_keys_status CHECK (status IN ('processing', 'completed'))
);

CREATE INDEX idx_idempotency_keys_expires_at ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE idempotency_keys;
