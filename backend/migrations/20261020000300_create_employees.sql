-- +goose Up
-- Data dasar karyawan Fase 1 (BRD HR-01, HR-03; Desain Data Model 4.4). NIK dienkripsi AES-256-GCM dan dicari
-- persis lewat nik_hash (HMAC) (Technical Guideline 6.7).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE employees (
    id                uuid        NOT NULL DEFAULT uuidv7(),
    employee_number   text        NOT NULL,
    full_name         text        NOT NULL,
    -- division_id merujuk accounting.divisions (tanpa FK lintas modul).
    division_id       uuid,
    position_title    text        NOT NULL,
    employment_status text        NOT NULL DEFAULT 'active',
    join_date         date        NOT NULL,
    -- resign_date = tanggal efektif berhenti; mulai tanggal ini karyawan tidak lagi aktif.
    resign_date       date,
    resign_reason     text        NOT NULL DEFAULT '',
    work_email        text        NOT NULL DEFAULT '',
    phone             text        NOT NULL DEFAULT '',
    nik_enc           text        NOT NULL DEFAULT '',
    nik_hash          bytea,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid        NOT NULL,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    updated_by        uuid        NOT NULL,
    version           integer     NOT NULL DEFAULT 1,
    deleted_at        timestamptz,
    deleted_by        uuid,
    CONSTRAINT pk_employees PRIMARY KEY (id),
    -- Nomor karyawan permanen, tidak dipakai ulang walau dihapus (Technical Guideline 5.13).
    CONSTRAINT uq_employees_employee_number UNIQUE (employee_number),
    CONSTRAINT ck_employees_status CHECK (employment_status IN ('active', 'resigned')),
    CONSTRAINT ck_employees_resign CHECK ((employment_status = 'resigned') = (resign_date IS NOT NULL)),
    CONSTRAINT ck_employees_resign_after_join CHECK (resign_date IS NULL OR resign_date >= join_date),
    CONSTRAINT ck_employees_nik CHECK ((nik_enc = '') = (nik_hash IS NULL))
);
CREATE UNIQUE INDEX uq_employees_nik_hash ON employees (nik_hash) WHERE deleted_at IS NULL AND nik_hash IS NOT NULL;
CREATE INDEX idx_employees_division_id ON employees (division_id);
CREATE INDEX idx_employees_full_name_trgm ON employees USING gin (lower(full_name) gin_trgm_ops);

-- +goose Down
DROP TABLE employees;
