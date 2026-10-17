-- +goose Up
-- Job impor/ekspor Excel modul dataexchange (BRD MD-10, INT-02, Technical Guideline 5.15). File xlsx disimpan
-- di filestore; file_id/error_file_id merujuk file_objects tanpa FK lintas modul.
CREATE TABLE import_jobs (
    id              uuid        NOT NULL DEFAULT uuidv7(),
    -- kind = nama resource permission, mis. catalog.product (izin <kind>.import).
    kind            text        NOT NULL,
    file_id         uuid        NOT NULL,
    file_name       text        NOT NULL,
    file_sha256     bytea       NOT NULL,
    status          text        NOT NULL DEFAULT 'uploaded',
    total_rows      integer     NOT NULL DEFAULT 0,
    processed_rows  integer     NOT NULL DEFAULT 0,
    valid_rows      integer     NOT NULL DEFAULT 0,
    error_rows      integer     NOT NULL DEFAULT 0,
    committed_rows  integer     NOT NULL DEFAULT 0,
    error_file_id   uuid,
    message         text        NOT NULL DEFAULT '',
    validated_at    timestamptz,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid        NOT NULL,
    version         integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_import_jobs PRIMARY KEY (id),
    CONSTRAINT uq_import_jobs_file_id UNIQUE (file_id),
    CONSTRAINT ck_import_jobs_status CHECK (status IN (
        'uploaded', 'validating', 'validated', 'committing', 'completed', 'failed', 'cancelled')),
    CONSTRAINT ck_import_jobs_counts CHECK (
        total_rows >= 0 AND processed_rows >= 0 AND valid_rows >= 0 AND error_rows >= 0 AND committed_rows >= 0)
);
-- File yang sama (SHA-256) tidak bisa diimpor dua kali per jenis data; file dari job gagal/dibatalkan boleh
-- diunggah ulang.
CREATE UNIQUE INDEX uq_import_jobs_kind_sha256 ON import_jobs (kind, file_sha256)
    WHERE status NOT IN ('failed', 'cancelled');
CREATE INDEX idx_import_jobs_kind_created ON import_jobs (kind, created_at DESC);
CREATE INDEX idx_import_jobs_created_by ON import_jobs (created_by);

-- Hasil pemeriksaan per baris untuk preview. Kolom data pribadi disamarkan (Technical Guideline 6.6); isi asli
-- tetap hanya di file.
CREATE TABLE import_job_rows (
    job_id      uuid    NOT NULL,
    row_number  integer NOT NULL,
    is_valid    boolean NOT NULL,
    cell_values jsonb   NOT NULL,
    errors      jsonb   NOT NULL DEFAULT '[]',
    CONSTRAINT pk_import_job_rows PRIMARY KEY (job_id, row_number),
    CONSTRAINT fk_import_job_rows_job FOREIGN KEY (job_id) REFERENCES import_jobs (id) ON DELETE CASCADE
);
CREATE INDEX idx_import_job_rows_validity ON import_job_rows (job_id, is_valid, row_number);

CREATE TABLE export_jobs (
    id            uuid        NOT NULL DEFAULT uuidv7(),
    kind          text        NOT NULL,
    status        text        NOT NULL DEFAULT 'queued',
    total_rows    integer     NOT NULL DEFAULT 0,
    file_id       uuid,
    file_name     text        NOT NULL DEFAULT '',
    message       text        NOT NULL DEFAULT '',
    completed_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid        NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    updated_by    uuid        NOT NULL,
    version       integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_export_jobs PRIMARY KEY (id),
    CONSTRAINT ck_export_jobs_status CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    CONSTRAINT ck_export_jobs_file CHECK ((status = 'completed') = (file_id IS NOT NULL))
);
CREATE INDEX idx_export_jobs_kind_created ON export_jobs (kind, created_at DESC);
CREATE INDEX idx_export_jobs_created_by ON export_jobs (created_by);

-- +goose Down
DROP TABLE export_jobs;
DROP TABLE import_job_rows;
DROP TABLE import_jobs;
