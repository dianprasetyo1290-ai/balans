-- +goose Up
CREATE TABLE webhook_events (
    id           uuid        NOT NULL DEFAULT uuidv7(),
    provider     text        NOT NULL,
    event_id     text        NOT NULL,
    event_type   text        NOT NULL,
    payload      jsonb       NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    CONSTRAINT pk_webhook_events PRIMARY KEY (id),
    CONSTRAINT uq_webhook_events_provider_event_id UNIQUE (provider, event_id)
);

-- +goose Down
DROP TABLE webhook_events;
