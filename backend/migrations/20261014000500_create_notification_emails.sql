-- +goose Up
CREATE TABLE notification_emails (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    recipient  text        NOT NULL,
    subject    text        NOT NULL,
    body_text  text        NOT NULL,
    status     text        NOT NULL DEFAULT 'queued',
    attempts   integer     NOT NULL DEFAULT 0,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz,
    CONSTRAINT pk_notification_emails PRIMARY KEY (id),
    CONSTRAINT ck_notification_emails_status CHECK (status IN ('queued', 'sent', 'failed'))
);

-- +goose Down
DROP TABLE notification_emails;
