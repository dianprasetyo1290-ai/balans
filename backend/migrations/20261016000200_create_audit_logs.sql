-- +goose Up
CREATE TABLE audit_logs (
    id          uuid        NOT NULL DEFAULT uuidv7(),
    actor_id    uuid        NOT NULL,
    actor_name  text        NOT NULL,
    action      text        NOT NULL,
    entity_type text        NOT NULL,
    entity_id   uuid        NOT NULL,
    entity_label text       NOT NULL DEFAULT '',
    changes     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    note        text        NOT NULL DEFAULT '',
    request_id  text        NOT NULL DEFAULT '',
    ip          text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_audit_logs PRIMARY KEY (id),
    CONSTRAINT ck_audit_logs_action CHECK (action IN (
        'create', 'update', 'delete', 'restore', 'status_change', 'approve', 'reject', 'submit',
        'post', 'cancel', 'login', 'login_failed', 'logout', 'import', 'export'))
);
CREATE INDEX idx_audit_logs_entity ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor_id ON audit_logs (actor_id, created_at DESC);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at DESC);

-- Audit trail append-only (Technical Guideline 6.6): perubahan dan penghapusan ditolak di level DB.
-- +goose StatementBegin
CREATE FUNCTION audit_logs_reject_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_audit_logs_append_only
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_reject_mutation();

-- +goose Down
DROP TABLE audit_logs;
DROP FUNCTION audit_logs_reject_mutation();
