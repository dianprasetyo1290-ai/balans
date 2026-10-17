-- +goose Up
-- Metadata file modul filestore (Technical Guideline 5.16, Desain Data Model 3.3). Isi file disimpan di
-- penyimpanan objek (Fase 1: disk lokal FILESTORE_DIR), bukan di database.
CREATE TABLE file_objects (
    id            uuid        NOT NULL DEFAULT uuidv7(),
    -- object_key acak, terpisah dari id yang terlihat di API.
    object_key    uuid        NOT NULL DEFAULT gen_random_uuid(),
    original_name text        NOT NULL,
    content_type  text        NOT NULL,
    size_bytes    bigint      NOT NULL,
    sha256        bytea,
    -- owner_type menentukan permission baca/tulis dan batas file (mis. partner_agreement = PDF PKS).
    owner_type    text        NOT NULL,
    owner_id      uuid,
    status        text        NOT NULL DEFAULT 'pending',
    uploaded_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid        NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    updated_by    uuid        NOT NULL,
    version       integer     NOT NULL DEFAULT 1,
    deleted_at    timestamptz,
    deleted_by    uuid,
    CONSTRAINT pk_file_objects PRIMARY KEY (id),
    CONSTRAINT uq_file_objects_object_key UNIQUE (object_key),
    CONSTRAINT ck_file_objects_status CHECK (status IN ('pending', 'uploaded')),
    CONSTRAINT ck_file_objects_size CHECK (size_bytes > 0),
    CONSTRAINT ck_file_objects_uploaded CHECK (
        (status = 'uploaded') = (uploaded_at IS NOT NULL AND sha256 IS NOT NULL)),
    CONSTRAINT ck_file_objects_owner CHECK (owner_id IS NULL OR status = 'uploaded')
);
CREATE INDEX idx_file_objects_owner ON file_objects (owner_type, owner_id);
CREATE INDEX idx_file_objects_pending ON file_objects (created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE file_objects;
