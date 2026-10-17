-- +goose Up
-- Dokumen persediaan modul inventory (R1.2): Pemakaian Internal & event (INV-05, HR-02), Repacking (INV-06),
-- Pemusnahan dengan berita acara (INV-12), Disposisi Retur (WMS-07), dan recost backdate (ACC-14, TG 5.8.4).
-- Desain Data Model 6.2; state machine Desain Status Dokumen 3.5, 4.1, 4.2. product_id, location_id,
-- division_id, campaign_id, reason_id, dan file tanpa FK karena tabelnya milik modul lain (TG 2.3).
-- Nama pelaku disimpan bersama ID-nya (snapshot) agar detail dokumen tidak perlu membaca modul identity.

CREATE TABLE internal_usages (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    number                text,
    purpose               text          NOT NULL,
    division_id           uuid          NOT NULL,
    campaign_id           uuid,
    source_location_id    uuid          NOT NULL,
    event_location_id     uuid,
    event_end_date        date,
    settle_due_date       date,
    business_date         date          NOT NULL,
    note                  text          NOT NULL DEFAULT '',
    status                text          NOT NULL DEFAULT 'draft',
    estimated_cost        numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount           numeric(18,2) NOT NULL DEFAULT 0,
    journal_id            uuid,
    return_disposition_id uuid,
    requested_by          uuid          NOT NULL,
    requested_by_name     text          NOT NULL DEFAULT '',
    released_by           uuid,
    released_by_name      text          NOT NULL DEFAULT '',
    released_at           timestamptz,
    handed_over_by        uuid,
    handed_over_by_name   text          NOT NULL DEFAULT '',
    handed_over_at        timestamptz,
    settled_by            uuid,
    settled_by_name       text          NOT NULL DEFAULT '',
    settled_at            timestamptz,
    posted_by             uuid,
    posted_by_name        text          NOT NULL DEFAULT '',
    posted_at             timestamptz,
    reminded_on           date,
    created_at            timestamptz   NOT NULL DEFAULT now(),
    created_by            uuid          NOT NULL,
    updated_at            timestamptz   NOT NULL DEFAULT now(),
    updated_by            uuid          NOT NULL,
    version               integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_internal_usages PRIMARY KEY (id),
    CONSTRAINT uq_internal_usages_number UNIQUE (number),
    CONSTRAINT ck_internal_usages_purpose CHECK (purpose IN ('marketing', 'event', 'employee_sample', 'sales_sampling')),
    CONSTRAINT ck_internal_usages_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'in_progress', 'awaiting_settlement', 'posted', 'cancelled')),
    CONSTRAINT ck_internal_usages_event CHECK (
        purpose <> 'event' OR (event_location_id IS NOT NULL AND event_end_date IS NOT NULL AND settle_due_date IS NOT NULL)),
    CONSTRAINT ck_internal_usages_note CHECK (length(note) <= 500),
    CONSTRAINT ck_internal_usages_amounts CHECK (estimated_cost >= 0 AND cost_amount >= 0)
);
CREATE INDEX idx_internal_usages_status ON internal_usages (status, business_date DESC);
CREATE INDEX idx_internal_usages_settle_due ON internal_usages (settle_due_date)
    WHERE status = 'awaiting_settlement' AND settled_at IS NULL;

CREATE TABLE internal_usage_lines (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    internal_usage_id uuid          NOT NULL,
    line_no           integer       NOT NULL,
    product_id        uuid          NOT NULL,
    qty               numeric(18,3) NOT NULL,
    returned_qty      numeric(18,3) NOT NULL DEFAULT 0,
    consumed_qty      numeric(18,3) NOT NULL DEFAULT 0,
    cost_amount       numeric(18,2) NOT NULL DEFAULT 0,
    note              text          NOT NULL DEFAULT '',
    CONSTRAINT pk_internal_usage_lines PRIMARY KEY (id),
    CONSTRAINT uq_internal_usage_lines_line_no UNIQUE (internal_usage_id, line_no),
    CONSTRAINT fk_internal_usage_lines_internal_usage_id FOREIGN KEY (internal_usage_id) REFERENCES internal_usages (id) ON DELETE CASCADE,
    CONSTRAINT ck_internal_usage_lines_qty CHECK (
        qty > 0 AND returned_qty >= 0 AND consumed_qty >= 0 AND returned_qty + consumed_qty <= qty),
    CONSTRAINT ck_internal_usage_lines_note CHECK (length(note) <= 500)
);

CREATE TABLE repackings (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    number                text,
    business_date         date          NOT NULL,
    note                  text          NOT NULL DEFAULT '',
    status                text          NOT NULL DEFAULT 'draft',
    cost_amount           numeric(18,2) NOT NULL DEFAULT 0,
    journal_id            uuid,
    return_disposition_id uuid,
    started_by            uuid,
    started_by_name       text          NOT NULL DEFAULT '',
    started_at            timestamptz,
    posted_by             uuid,
    posted_by_name        text          NOT NULL DEFAULT '',
    posted_at             timestamptz,
    created_at            timestamptz   NOT NULL DEFAULT now(),
    created_by            uuid          NOT NULL,
    created_by_name       text          NOT NULL DEFAULT '',
    updated_at            timestamptz   NOT NULL DEFAULT now(),
    updated_by            uuid          NOT NULL,
    version               integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_repackings PRIMARY KEY (id),
    CONSTRAINT uq_repackings_number UNIQUE (number),
    CONSTRAINT ck_repackings_status CHECK (status IN ('draft', 'in_progress', 'posted', 'cancelled')),
    CONSTRAINT ck_repackings_note CHECK (length(note) <= 500),
    CONSTRAINT ck_repackings_cost CHECK (cost_amount >= 0)
);
CREATE INDEX idx_repackings_status ON repackings (status, business_date DESC);

-- direction input: barang retur (lokasi Retur) atau packaging baru (lokasi RM). direction output: FG batch baru.
CREATE TABLE repacking_lines (
    id              uuid          NOT NULL DEFAULT uuidv7(),
    repacking_id    uuid          NOT NULL,
    line_no         integer       NOT NULL,
    direction       text          NOT NULL,
    product_id      uuid          NOT NULL,
    location_id     uuid          NOT NULL,
    stock_status    text          NOT NULL DEFAULT 'available',
    batch_id        uuid,
    batch_number    text          NOT NULL DEFAULT '',
    production_date date,
    expiry_date     date,
    qty             numeric(18,3) NOT NULL,
    cost_amount     numeric(18,2) NOT NULL DEFAULT 0,
    note            text          NOT NULL DEFAULT '',
    CONSTRAINT pk_repacking_lines PRIMARY KEY (id),
    CONSTRAINT uq_repacking_lines_line_no UNIQUE (repacking_id, line_no),
    CONSTRAINT fk_repacking_lines_repacking_id FOREIGN KEY (repacking_id) REFERENCES repackings (id) ON DELETE CASCADE,
    CONSTRAINT fk_repacking_lines_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_repacking_lines_direction CHECK (direction IN ('input', 'output')),
    CONSTRAINT ck_repacking_lines_status CHECK (stock_status IN ('available', 'quarantine')),
    CONSTRAINT ck_repacking_lines_qty CHECK (qty > 0 AND cost_amount >= 0),
    CONSTRAINT ck_repacking_lines_note CHECK (length(note) <= 500)
);
CREATE INDEX idx_repacking_lines_batch_id ON repacking_lines (batch_id);

CREATE TABLE disposals (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    number                text,
    reason                text          NOT NULL,
    disposal_date         date          NOT NULL,
    method                text          NOT NULL DEFAULT '',
    witnesses             text[]        NOT NULL DEFAULT '{}',
    photo_file_ids        uuid[]        NOT NULL DEFAULT '{}',
    note                  text          NOT NULL DEFAULT '',
    status                text          NOT NULL DEFAULT 'draft',
    estimated_cost        numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount           numeric(18,2) NOT NULL DEFAULT 0,
    posting_rule_code     text          NOT NULL DEFAULT '',
    journal_id            uuid,
    return_disposition_id uuid,
    submitted_by          uuid,
    submitted_by_name     text          NOT NULL DEFAULT '',
    submitted_at          timestamptz,
    posted_by             uuid,
    posted_by_name        text          NOT NULL DEFAULT '',
    posted_at             timestamptz,
    created_at            timestamptz   NOT NULL DEFAULT now(),
    created_by            uuid          NOT NULL,
    created_by_name       text          NOT NULL DEFAULT '',
    updated_at            timestamptz   NOT NULL DEFAULT now(),
    updated_by            uuid          NOT NULL,
    version               integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_disposals PRIMARY KEY (id),
    CONSTRAINT uq_disposals_number UNIQUE (number),
    CONSTRAINT ck_disposals_reason CHECK (reason IN ('expired', 'damaged', 'return')),
    CONSTRAINT ck_disposals_status CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected', 'posted', 'cancelled')),
    CONSTRAINT ck_disposals_method CHECK (length(method) <= 500),
    CONSTRAINT ck_disposals_note CHECK (length(note) <= 500),
    CONSTRAINT ck_disposals_amounts CHECK (estimated_cost >= 0 AND cost_amount >= 0)
);
CREATE INDEX idx_disposals_status ON disposals (status, disposal_date DESC);

CREATE TABLE disposal_lines (
    id           uuid          NOT NULL DEFAULT uuidv7(),
    disposal_id  uuid          NOT NULL,
    line_no      integer       NOT NULL,
    product_id   uuid          NOT NULL,
    batch_id     uuid,
    location_id  uuid          NOT NULL,
    stock_status text          NOT NULL DEFAULT 'available',
    qty          numeric(18,3) NOT NULL,
    cost_amount  numeric(18,2) NOT NULL DEFAULT 0,
    note         text          NOT NULL DEFAULT '',
    CONSTRAINT pk_disposal_lines PRIMARY KEY (id),
    CONSTRAINT uq_disposal_lines_line_no UNIQUE (disposal_id, line_no),
    CONSTRAINT fk_disposal_lines_disposal_id FOREIGN KEY (disposal_id) REFERENCES disposals (id) ON DELETE CASCADE,
    CONSTRAINT fk_disposal_lines_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_disposal_lines_status CHECK (stock_status IN ('available', 'quarantine')),
    CONSTRAINT ck_disposal_lines_qty CHECK (qty > 0 AND cost_amount >= 0),
    CONSTRAINT ck_disposal_lines_note CHECK (length(note) <= 500)
);
CREATE INDEX idx_disposal_lines_batch_id ON disposal_lines (batch_id);

CREATE TABLE return_dispositions (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    number            text,
    location_id       uuid          NOT NULL,
    inbound_id        uuid,
    business_date     date          NOT NULL,
    note              text          NOT NULL DEFAULT '',
    status            text          NOT NULL DEFAULT 'draft',
    disposal_value    numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount       numeric(18,2) NOT NULL DEFAULT 0,
    journal_id        uuid,
    submitted_by      uuid,
    submitted_by_name text          NOT NULL DEFAULT '',
    submitted_at      timestamptz,
    posted_by         uuid,
    posted_by_name    text          NOT NULL DEFAULT '',
    posted_at         timestamptz,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    created_by_name   text          NOT NULL DEFAULT '',
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_return_dispositions PRIMARY KEY (id),
    CONSTRAINT uq_return_dispositions_number UNIQUE (number),
    CONSTRAINT ck_return_dispositions_status CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected', 'posted', 'cancelled')),
    CONSTRAINT ck_return_dispositions_note CHECK (length(note) <= 500),
    CONSTRAINT ck_return_dispositions_amounts CHECK (disposal_value >= 0 AND cost_amount >= 0)
);
CREATE INDEX idx_return_dispositions_status ON return_dispositions (status, business_date DESC);

-- target_type/target_id: dokumen lanjutan hasil eksekusi (repacking, disposal, internal_usage).
CREATE TABLE return_disposition_lines (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    return_disposition_id uuid          NOT NULL,
    line_no               integer       NOT NULL,
    product_id            uuid          NOT NULL,
    batch_id              uuid,
    stock_status          text          NOT NULL DEFAULT 'available',
    qty                   numeric(18,3) NOT NULL,
    reason_id             uuid,
    disposition           text          NOT NULL,
    qc_note               text          NOT NULL DEFAULT '',
    estimated_cost        numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount           numeric(18,2) NOT NULL DEFAULT 0,
    target_type           text          NOT NULL DEFAULT '',
    target_id             uuid,
    CONSTRAINT pk_return_disposition_lines PRIMARY KEY (id),
    CONSTRAINT uq_return_disposition_lines_line_no UNIQUE (return_disposition_id, line_no),
    CONSTRAINT fk_return_disposition_lines_return_disposition_id FOREIGN KEY (return_disposition_id)
        REFERENCES return_dispositions (id) ON DELETE CASCADE,
    CONSTRAINT fk_return_disposition_lines_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_return_disposition_lines_status CHECK (stock_status IN ('available', 'quarantine')),
    CONSTRAINT ck_return_disposition_lines_disposition CHECK (
        disposition IN ('restock', 'repack', 'reprocess', 'marketing_sample', 'dispose')),
    CONSTRAINT ck_return_disposition_lines_qty CHECK (qty > 0 AND estimated_cost >= 0 AND cost_amount >= 0),
    CONSTRAINT ck_return_disposition_lines_qc_note CHECK (length(qc_note) <= 500)
);
CREATE INDEX idx_return_disposition_lines_batch_id ON return_disposition_lines (batch_id);

-- Recost akibat penerimaan bertanggal mundur (TG 5.8.4). Satu run per (produk, batch) per penerimaan.
CREATE TABLE recost_runs (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    product_id            uuid          NOT NULL,
    batch_id              uuid,
    from_business_date    date          NOT NULL,
    trigger_source_type   text          NOT NULL,
    trigger_source_id     uuid          NOT NULL,
    trigger_source_label  text          NOT NULL DEFAULT '',
    status                text          NOT NULL DEFAULT 'pending',
    adjustment_amount     numeric(18,2) NOT NULL DEFAULT 0,
    adjustment_journal_id uuid,
    error                 text          NOT NULL DEFAULT '',
    created_at            timestamptz   NOT NULL DEFAULT now(),
    created_by            uuid          NOT NULL,
    completed_at          timestamptz,
    CONSTRAINT pk_recost_runs PRIMARY KEY (id),
    CONSTRAINT fk_recost_runs_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_recost_runs_status CHECK (status IN ('pending', 'completed', 'failed'))
);
CREATE INDEX idx_recost_runs_batch_id ON recost_runs (batch_id);
CREATE INDEX idx_recost_runs_product ON recost_runs (product_id, created_at DESC);

-- Selisih HPP per movement keluar hasil recost, append-only. HPP efektif movement = -cost_amount + Σ difference.
CREATE TABLE recost_lines (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    recost_run_id     uuid          NOT NULL,
    stock_movement_id uuid          NOT NULL,
    location_id       uuid          NOT NULL,
    source_type       text          NOT NULL,
    source_id         uuid          NOT NULL,
    previous_cost     numeric(18,2) NOT NULL,
    recomputed_cost   numeric(18,2) NOT NULL,
    difference        numeric(18,2) NOT NULL,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT pk_recost_lines PRIMARY KEY (id),
    CONSTRAINT fk_recost_lines_recost_run_id FOREIGN KEY (recost_run_id) REFERENCES recost_runs (id),
    CONSTRAINT fk_recost_lines_stock_movement_id FOREIGN KEY (stock_movement_id) REFERENCES stock_movements (id),
    CONSTRAINT ck_recost_lines_difference CHECK (difference = recomputed_cost - previous_cost AND difference <> 0)
);
CREATE INDEX idx_recost_lines_recost_run_id ON recost_lines (recost_run_id);
CREATE INDEX idx_recost_lines_stock_movement_id ON recost_lines (stock_movement_id);

CREATE TRIGGER trg_recost_lines_append_only
    BEFORE UPDATE OR DELETE ON recost_lines
    FOR EACH ROW EXECUTE FUNCTION inventory_reject_mutation();

-- Posting rule penyesuaian HPP recost (TG 5.8.4). inventory_adjustment ke akun persediaan sesuai lokasi/jenis
-- produk, cost_adjustment ke akun lawan pemakaian (HPP 5101, beban pemakaian/pemusnahan). Nilai positif =
-- HPP turun (persediaan bertambah); negatif membalik sisi. Dikirim per akun lewat Splits.
INSERT INTO posting_rules (code, name, required_dimensions, release, description, created_by, updated_by)
VALUES ('A-RC', 'Penyesuaian HPP recost (backdate)', ARRAY['division'], 'R1.2',
        'Selisih HPP konsumsi FIFO setelah penerimaan bertanggal mundur; jurnal lama tidak diubah',
        '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');
INSERT INTO posting_rule_accounts (
    posting_rule_code, amount_key, side, account_resolver, account_id, valid_from, created_by, updated_by)
SELECT m.rule, m.amount_key, m.side, m.resolver, a.id, DATE '2026-01-01',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('A-RC', 'inventory_adjustment', 'debit', 'location_inventory_account', '1311'),
    ('A-RC', 'cost_adjustment', 'credit', 'counter_account', '5101')
) AS m (rule, amount_key, side, resolver, account_code)
JOIN accounts a ON a.code = m.account_code;

-- +goose Down
DELETE FROM posting_rule_accounts WHERE posting_rule_code = 'A-RC';
DELETE FROM posting_rules WHERE code = 'A-RC';
DROP TABLE recost_lines;
DROP TABLE recost_runs;
DROP TABLE return_disposition_lines;
DROP TABLE return_dispositions;
DROP TABLE disposal_lines;
DROP TABLE disposals;
DROP TABLE repacking_lines;
DROP TABLE repackings;
DROP TABLE internal_usage_lines;
DROP TABLE internal_usages;
