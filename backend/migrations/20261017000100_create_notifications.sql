-- +goose Up
-- Notifikasi in-app per user (NFR-11). Email tetap di notification_emails.
CREATE TABLE notifications (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    user_id    uuid        NOT NULL,
    kind       text        NOT NULL,
    title      text        NOT NULL,
    body       text        NOT NULL DEFAULT '',
    link       text        NOT NULL DEFAULT '',
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_notifications PRIMARY KEY (id),
    CONSTRAINT ck_notifications_kind CHECK (kind ~ '^[a-z_]+\.[a-z_]+$')
);
CREATE INDEX idx_notifications_user_id_created_at ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_user_id_unread ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
