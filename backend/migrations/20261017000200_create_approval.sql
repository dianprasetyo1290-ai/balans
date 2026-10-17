-- +goose Up
-- Engine approval generik (BRD 5.17, Technical Guideline 5.5, Desain Data Model 3.2).
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Satu baris = satu level kebijakan untuk satu jenis dokumen. Effective-dated (Technical Guideline 5.14):
-- perubahan menutup baris lama (valid_to) dan membuat baris baru, sehingga dokumen yang sedang berjalan
-- tetap memakai snapshot langkah yang dibuat saat submit.
CREATE TABLE approval_policies (
    id                 uuid          NOT NULL DEFAULT uuidv7(),
    subject_type       text          NOT NULL,
    division_id        uuid,
    level              integer       NOT NULL,
    -- Level berlaku jika nominal > min_amount (null = semua nominal) dan nominal <= max_amount (null = tanpa batas).
    min_amount         numeric(18,2),
    max_amount         numeric(18,2),
    -- condition: syarat tambahan dari dokumen, mis. has_disposal (ada baris pemusnahan).
    condition          text          NOT NULL DEFAULT '',
    approver_role_code text          NOT NULL,
    approver_user_id   uuid,
    substitute_user_id uuid,
    valid_from         timestamptz   NOT NULL,
    valid_to           timestamptz,
    created_at         timestamptz   NOT NULL DEFAULT now(),
    created_by         uuid          NOT NULL,
    updated_at         timestamptz   NOT NULL DEFAULT now(),
    updated_by         uuid          NOT NULL,
    version            integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_approval_policies PRIMARY KEY (id),
    CONSTRAINT ck_approval_policies_level CHECK (level BETWEEN 1 AND 9),
    CONSTRAINT ck_approval_policies_amount_range CHECK (
        min_amount IS NULL OR max_amount IS NULL OR max_amount > min_amount),
    CONSTRAINT ck_approval_policies_amount_non_negative CHECK (
        (min_amount IS NULL OR min_amount >= 0) AND (max_amount IS NULL OR max_amount > 0)),
    CONSTRAINT ck_approval_policies_validity CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT ck_approval_policies_substitute CHECK (
        substitute_user_id IS NULL OR approver_user_id IS NULL OR substitute_user_id <> approver_user_id),
    CONSTRAINT ex_approval_policies_period EXCLUDE USING gist (
        subject_type WITH =,
        (coalesce(division_id, '00000000-0000-0000-0000-000000000000'::uuid)) WITH =,
        level WITH =,
        tstzrange(valid_from, valid_to) WITH &&)
);
CREATE INDEX idx_approval_policies_subject_type ON approval_policies (subject_type, valid_from);
CREATE INDEX idx_approval_policies_approver_user_id ON approval_policies (approver_user_id);
CREATE INDEX idx_approval_policies_substitute_user_id ON approval_policies (substitute_user_id);

CREATE TABLE approval_requests (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    subject_type      text          NOT NULL,
    subject_id        uuid          NOT NULL,
    subject_label     text          NOT NULL,
    subject_link      text          NOT NULL DEFAULT '',
    summary           text          NOT NULL DEFAULT '',
    amount            numeric(18,2) NOT NULL DEFAULT 0,
    division_id       uuid,
    flags             text[]        NOT NULL DEFAULT '{}',
    requested_by      uuid          NOT NULL,
    requested_by_name text          NOT NULL,
    status            text          NOT NULL DEFAULT 'pending_approval',
    current_level     integer       NOT NULL DEFAULT 1,
    total_levels      integer       NOT NULL,
    decided_at        timestamptz,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_approval_requests PRIMARY KEY (id),
    CONSTRAINT ck_approval_requests_status CHECK (status IN ('pending_approval', 'approved', 'rejected', 'cancelled')),
    CONSTRAINT ck_approval_requests_levels CHECK (total_levels >= 1 AND current_level BETWEEN 1 AND total_levels),
    CONSTRAINT ck_approval_requests_decided CHECK ((status = 'pending_approval') = (decided_at IS NULL))
);
-- Satu dokumen hanya punya satu permintaan yang sedang berjalan.
CREATE UNIQUE INDEX uq_approval_requests_active_subject ON approval_requests (subject_type, subject_id)
    WHERE status = 'pending_approval';
CREATE INDEX idx_approval_requests_subject ON approval_requests (subject_type, subject_id, created_at DESC);
CREATE INDEX idx_approval_requests_requested_by ON approval_requests (requested_by, created_at DESC);
CREATE INDEX idx_approval_requests_pending ON approval_requests (created_at) WHERE status = 'pending_approval';

-- Snapshot langkah per level saat submit. Langkah yang aktif = level = approval_requests.current_level.
CREATE TABLE approval_steps (
    id                     uuid        NOT NULL DEFAULT uuidv7(),
    approval_request_id    uuid        NOT NULL,
    level                  integer     NOT NULL,
    policy_id              uuid        NOT NULL,
    approver_role_code     text        NOT NULL,
    approver_user_id       uuid,
    substitute_user_id     uuid,
    -- division_id: scope approver untuk role ber-scope divisi (Head Divisi).
    division_id            uuid,
    -- requester_is_approver: pembuat dokumen memegang jabatan approver level ini, sehingga level naik ke pengganti.
    requester_is_approver  boolean     NOT NULL DEFAULT false,
    decision               text,
    decided_by             uuid,
    decided_by_name        text        NOT NULL DEFAULT '',
    delegated_from_user_id uuid,
    note                   text        NOT NULL DEFAULT '',
    activated_at           timestamptz,
    decided_at             timestamptz,
    last_reminded_at       timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_approval_steps PRIMARY KEY (id),
    CONSTRAINT uq_approval_steps_level UNIQUE (approval_request_id, level),
    CONSTRAINT fk_approval_steps_approval_request_id FOREIGN KEY (approval_request_id) REFERENCES approval_requests (id),
    CONSTRAINT fk_approval_steps_policy_id FOREIGN KEY (policy_id) REFERENCES approval_policies (id),
    CONSTRAINT ck_approval_steps_decision CHECK (decision IS NULL OR decision IN ('approved', 'rejected')),
    CONSTRAINT ck_approval_steps_decided CHECK ((decision IS NULL) = (decided_at IS NULL) AND (decision IS NULL) = (decided_by IS NULL)),
    CONSTRAINT ck_approval_steps_reject_note CHECK (decision IS DISTINCT FROM 'rejected' OR btrim(note) <> '')
);
CREATE INDEX idx_approval_steps_policy_id ON approval_steps (policy_id);
CREATE INDEX idx_approval_steps_approver_role_code ON approval_steps (approver_role_code) WHERE decision IS NULL;
CREATE INDEX idx_approval_steps_approver_user_id ON approval_steps (approver_user_id) WHERE decision IS NULL;
CREATE INDEX idx_approval_steps_substitute_user_id ON approval_steps (substitute_user_id) WHERE decision IS NULL;
CREATE INDEX idx_approval_steps_decided_by ON approval_steps (decided_by, decided_at DESC);

-- Delegasi approver yang cuti. Rentang tanggal WIB inklusif; satu user hanya satu delegasi aktif per tanggal.
CREATE TABLE approval_delegations (
    id           uuid        NOT NULL DEFAULT uuidv7(),
    from_user_id uuid        NOT NULL,
    to_user_id   uuid        NOT NULL,
    valid_from   date        NOT NULL,
    valid_to     date        NOT NULL,
    reason       text        NOT NULL DEFAULT '',
    cancelled_at timestamptz,
    cancelled_by uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid        NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid        NOT NULL,
    version      integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_approval_delegations PRIMARY KEY (id),
    CONSTRAINT ck_approval_delegations_users CHECK (from_user_id <> to_user_id),
    CONSTRAINT ck_approval_delegations_period CHECK (valid_to >= valid_from),
    CONSTRAINT ex_approval_delegations_period EXCLUDE USING gist (
        from_user_id WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (cancelled_at IS NULL)
);
CREATE INDEX idx_approval_delegations_from_user_id ON approval_delegations (from_user_id, valid_from);
CREATE INDEX idx_approval_delegations_to_user_id ON approval_delegations (to_user_id, valid_from);

-- Matriks approval final (Desain Role & Approval bagian 4) dengan batas nominal awal BRD 5.17.
-- Nama approver dan pengganti diisi Balans lewat Pengaturan › Matriks Persetujuan.
INSERT INTO approval_policies (subject_type, level, min_amount, condition, approver_role_code, valid_from, created_by, updated_by)
SELECT p.subject_type, p.level, p.min_amount, p.condition, p.role, '2026-01-01T00:00:00+07:00',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('purchase_request',            1, NULL::numeric,  '',             'division_head'),
    ('purchase_order',              1, NULL,           '',             'head_finance'),
    ('purchase_order',              2, 10000000,       '',             'director'),
    ('purchase_order',              3, 50000000,       '',             'president_director'),
    ('expense_claim',               1, NULL,           '',             'division_head'),
    ('expense_claim',               2, 5000000,        '',             'head_finance'),
    ('expense_claim',               3, 25000000,       '',             'director'),
    ('internal_usage',              1, NULL,           '',             'division_head'),
    ('internal_usage',              2, 5000000,        '',             'head_finance'),
    ('stock_count',                 1, NULL,           '',             'head_logistics'),
    ('stock_count',                 2, 2000000,        '',             'head_finance'),
    ('return_disposition',          1, NULL,           '',             'head_logistics'),
    ('return_disposition',          2, NULL,           'has_disposal', 'head_finance'),
    ('return_disposition',          3, 10000000,       'has_disposal', 'director'),
    ('disposal',                    1, NULL,           '',             'head_logistics'),
    ('disposal',                    2, NULL,           '',             'head_finance'),
    ('disposal',                    3, 10000000,       '',             'director'),
    ('consignment_reconciliation',  1, NULL,           '',             'sales_mt'),
    ('consignment_reconciliation',  2, NULL,           '',             'head_finance'),
    ('price_list_version',          1, NULL,           '',             'head_sales'),
    ('price_list_version',          2, NULL,           '',             'head_finance'),
    ('sales_order_discount',        1, NULL,           '',             'head_sales'),
    ('supplier_bill_exception',     1, NULL,           '',             'head_finance'),
    ('petty_cash_replenishment',    1, NULL,           '',             'head_finance'),
    ('fiscal_period_reopen',        1, NULL,           '',             'head_finance'),
    ('fiscal_period_reopen',        2, NULL,           '',             'director')
) AS p(subject_type, level, min_amount, condition, role);

-- +goose Down
DROP TABLE approval_delegations;
DROP TABLE approval_steps;
DROP TABLE approval_requests;
DROP TABLE approval_policies;
