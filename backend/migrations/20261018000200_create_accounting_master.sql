-- +goose Up
-- Master akuntansi: Chart of Accounts (ACC-01), dimensi jurnal (ACC-17), dan periode akuntansi (ACC-08).
-- Desain Data Model bagian 9, Desain Finance & Accounting bagian 1 dan 3.

-- Kode akun 4 digit; digit pertama = kelompok (1 Aset … 7 Lain-lain). Kode unik permanen: tidak dipakai
-- ulang walau akun dihapus (Technical Guideline 5.13). Akun induk (judul) tidak bisa diposting.
CREATE TABLE accounts (
    id                   uuid        NOT NULL DEFAULT uuidv7(),
    code                 text        NOT NULL,
    name                 text        NOT NULL,
    account_group        smallint    NOT NULL,
    normal_balance       text        NOT NULL,
    parent_id            uuid,
    is_postable          boolean     NOT NULL,
    allow_manual_posting boolean     NOT NULL DEFAULT false,
    is_active            boolean     NOT NULL DEFAULT true,
    description          text        NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    created_by           uuid        NOT NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    updated_by           uuid        NOT NULL,
    version              integer     NOT NULL DEFAULT 1,
    deleted_at           timestamptz,
    deleted_by           uuid,
    CONSTRAINT pk_accounts PRIMARY KEY (id),
    CONSTRAINT uq_accounts_code UNIQUE (code),
    CONSTRAINT fk_accounts_parent_id FOREIGN KEY (parent_id) REFERENCES accounts (id),
    CONSTRAINT ck_accounts_code CHECK (code ~ '^[1-7][0-9]{3}$'),
    CONSTRAINT ck_accounts_group CHECK (account_group BETWEEN 1 AND 7 AND left(code, 1) = account_group::text),
    CONSTRAINT ck_accounts_normal_balance CHECK (normal_balance IN ('debit', 'credit')),
    CONSTRAINT ck_accounts_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_accounts_manual_postable CHECK (NOT allow_manual_posting OR is_postable),
    CONSTRAINT ck_accounts_parent_not_self CHECK (parent_id IS DISTINCT FROM id)
);
CREATE INDEX idx_accounts_parent_id ON accounts (parent_id);

-- Dimensi jurnal (Technical Guideline 5.8.2). Kode unik permanen per tabel.
CREATE TABLE channels (
    id          uuid        NOT NULL DEFAULT uuidv7(),
    code        text        NOT NULL,
    name        text        NOT NULL,
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid        NOT NULL,
    version     integer     NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    deleted_by  uuid,
    CONSTRAINT pk_channels PRIMARY KEY (id),
    CONSTRAINT uq_channels_code UNIQUE (code),
    CONSTRAINT ck_channels_code CHECK (code ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT ck_channels_name CHECK (btrim(name) <> '')
);

-- party_type/party_id menautkan sub-channel ke partner MT/konsinyasi atau customer korporat (modul party).
CREATE TABLE sub_channels (
    id          uuid        NOT NULL DEFAULT uuidv7(),
    channel_id  uuid        NOT NULL,
    code        text        NOT NULL,
    name        text        NOT NULL,
    party_type  text        NOT NULL DEFAULT '',
    party_id    uuid,
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid        NOT NULL,
    version     integer     NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    deleted_by  uuid,
    CONSTRAINT pk_sub_channels PRIMARY KEY (id),
    CONSTRAINT uq_sub_channels_code UNIQUE (channel_id, code),
    CONSTRAINT fk_sub_channels_channel_id FOREIGN KEY (channel_id) REFERENCES channels (id),
    CONSTRAINT ck_sub_channels_code CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,49}$'),
    CONSTRAINT ck_sub_channels_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_sub_channels_party CHECK ((party_type = '') = (party_id IS NULL))
);
CREATE INDEX idx_sub_channels_party ON sub_channels (party_type, party_id) WHERE party_id IS NOT NULL;

CREATE TABLE divisions (
    id          uuid        NOT NULL DEFAULT uuidv7(),
    code        text        NOT NULL,
    name        text        NOT NULL,
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid        NOT NULL,
    version     integer     NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    deleted_by  uuid,
    CONSTRAINT pk_divisions PRIMARY KEY (id),
    CONSTRAINT uq_divisions_code UNIQUE (code),
    CONSTRAINT ck_divisions_code CHECK (code ~ '^[A-Z][A-Z0-9]{1,9}$'),
    CONSTRAINT ck_divisions_name CHECK (btrim(name) <> '')
);

-- Kode kampanye KMP-YYYYMM-<singkatan> dengan periode berlaku (Desain Finance & Accounting 3.3, MD-13).
CREATE TABLE campaigns (
    id            uuid          NOT NULL DEFAULT uuidv7(),
    code          text          NOT NULL,
    name          text          NOT NULL,
    valid_from    date          NOT NULL,
    valid_to      date          NOT NULL,
    budget_amount numeric(18,2),
    description   text          NOT NULL DEFAULT '',
    is_active     boolean       NOT NULL DEFAULT true,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    created_by    uuid          NOT NULL,
    updated_at    timestamptz   NOT NULL DEFAULT now(),
    updated_by    uuid          NOT NULL,
    version       integer       NOT NULL DEFAULT 1,
    deleted_at    timestamptz,
    deleted_by    uuid,
    CONSTRAINT pk_campaigns PRIMARY KEY (id),
    CONSTRAINT uq_campaigns_code UNIQUE (code),
    CONSTRAINT ck_campaigns_code CHECK (code ~ '^KMP-[0-9]{4}(0[1-9]|1[0-2])-[A-Z0-9]{2,30}$'),
    CONSTRAINT ck_campaigns_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_campaigns_period CHECK (valid_to >= valid_from),
    CONSTRAINT ck_campaigns_budget CHECK (budget_amount IS NULL OR budget_amount >= 0)
);

-- Periode akuntansi bulanan (ACC-08). Dibuat otomatis per bulan; tutup buku & buka kembali di R1.4 (S-411).
CREATE TABLE fiscal_periods (
    id             uuid        NOT NULL DEFAULT uuidv7(),
    year           integer     NOT NULL,
    month          integer     NOT NULL,
    status         text        NOT NULL DEFAULT 'open',
    closed_at      timestamptz,
    closed_by      uuid,
    reopened_count integer     NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    created_by     uuid        NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    updated_by     uuid        NOT NULL,
    version        integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_fiscal_periods PRIMARY KEY (id),
    CONSTRAINT uq_fiscal_periods_year_month UNIQUE (year, month),
    CONSTRAINT ck_fiscal_periods_month CHECK (month BETWEEN 1 AND 12),
    CONSTRAINT ck_fiscal_periods_year CHECK (year BETWEEN 2000 AND 9999),
    CONSTRAINT ck_fiscal_periods_status CHECK (status IN ('open', 'closed')),
    CONSTRAINT ck_fiscal_periods_closed CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);

-- +goose Down
DROP TABLE fiscal_periods;
DROP TABLE campaigns;
DROP TABLE divisions;
DROP TABLE sub_channels;
DROP TABLE channels;
DROP TABLE accounts;
