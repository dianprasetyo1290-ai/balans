-- +goose Up
-- Register aset tetap & penyusutan garis lurus bulanan (ACC-21, U-11, A-31; Desain Data Model bagian 9).

-- Kelas aset menentukan umur manfaat default dan akun A-31: aset (150x) dan akumulasi (159x). Akun beban
-- penyusutan (6305) mengikuti posting_rule_accounts A-31.
CREATE TABLE fixed_asset_classes (
    id                     uuid        NOT NULL DEFAULT uuidv7(),
    code                   text        NOT NULL,
    name                   text        NOT NULL,
    useful_life_months     integer     NOT NULL,
    asset_account_id       uuid        NOT NULL,
    accumulated_account_id uuid        NOT NULL,
    is_active              boolean     NOT NULL DEFAULT true,
    created_at             timestamptz NOT NULL DEFAULT now(),
    created_by             uuid        NOT NULL,
    updated_at             timestamptz NOT NULL DEFAULT now(),
    updated_by             uuid        NOT NULL,
    version                integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_fixed_asset_classes PRIMARY KEY (id),
    CONSTRAINT uq_fixed_asset_classes_code UNIQUE (code),
    CONSTRAINT fk_fixed_asset_classes_asset_account_id FOREIGN KEY (asset_account_id) REFERENCES accounts (id),
    CONSTRAINT fk_fixed_asset_classes_accumulated_account_id FOREIGN KEY (accumulated_account_id) REFERENCES accounts (id),
    CONSTRAINT ck_fixed_asset_classes_code CHECK (code ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT ck_fixed_asset_classes_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_fixed_asset_classes_useful_life CHECK (useful_life_months BETWEEN 1 AND 600)
);
CREATE INDEX idx_fixed_asset_classes_asset_account_id ON fixed_asset_classes (asset_account_id);
CREATE INDEX idx_fixed_asset_classes_accumulated_account_id ON fixed_asset_classes (accumulated_account_id);

-- Umur manfaat awal usulan konsultan; dikonfirmasi akuntan Balans (Desain Finance & Accounting 5 butir 7).
INSERT INTO fixed_asset_classes (code, name, useful_life_months, asset_account_id, accumulated_account_id,
                                 created_by, updated_by)
SELECT c.code, c.name, c.life, a.id, acc.id,
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('vending_machine', 'Mesin Vending', 60, '1501', '1591'),
    ('office_equipment', 'Peralatan Kantor & Komputer', 48, '1502', '1592'),
    ('vehicle', 'Kendaraan', 96, '1503', '1593')
) AS c (code, name, life, asset_code, accumulated_code)
JOIN accounts a ON a.code = c.asset_code
JOIN accounts acc ON acc.code = c.accumulated_code;

-- Aset tetap. acquisition_source `purchase` = dibeli setelah go-live (jurnal A-31 perolehan saat diaktifkan);
-- `opening` = saldo awal migrasi (nilai GL masuk lewat A-36, tanpa jurnal perolehan). location_id merujuk lokasi
-- modul warehouse tanpa FK lintas modul. accumulated_amount = akumulasi penyusutan termasuk saldo awal.
CREATE TABLE fixed_assets (
    id                         uuid          NOT NULL DEFAULT uuidv7(),
    code                       text          NOT NULL,
    name                       text          NOT NULL,
    class_id                   uuid          NOT NULL,
    location_id                uuid,
    location_label             text          NOT NULL DEFAULT '',
    division_id                uuid          NOT NULL,
    acquisition_source         text          NOT NULL,
    acquisition_date           date          NOT NULL,
    acquisition_cost_amount    numeric(18,2) NOT NULL,
    salvage_amount             numeric(18,2) NOT NULL DEFAULT 0,
    useful_life_months         integer       NOT NULL,
    depreciation_start_date    date          NOT NULL,
    opening_accumulated_amount numeric(18,2) NOT NULL DEFAULT 0,
    opening_depreciated_months integer       NOT NULL DEFAULT 0,
    accumulated_amount         numeric(18,2) NOT NULL DEFAULT 0,
    payment_account_id         uuid,
    status                     text          NOT NULL DEFAULT 'draft',
    acquisition_journal_id     uuid,
    disposal_date              date,
    disposal_proceeds_amount   numeric(18,2),
    disposal_account_id        uuid,
    disposal_reason            text          NOT NULL DEFAULT '',
    disposal_journal_id        uuid,
    note                       text          NOT NULL DEFAULT '',
    created_at                 timestamptz   NOT NULL DEFAULT now(),
    created_by                 uuid          NOT NULL,
    updated_at                 timestamptz   NOT NULL DEFAULT now(),
    updated_by                 uuid          NOT NULL,
    version                    integer       NOT NULL DEFAULT 1,
    deleted_at                 timestamptz,
    deleted_by                 uuid,
    CONSTRAINT pk_fixed_assets PRIMARY KEY (id),
    CONSTRAINT uq_fixed_assets_code UNIQUE (code),
    CONSTRAINT fk_fixed_assets_class_id FOREIGN KEY (class_id) REFERENCES fixed_asset_classes (id),
    CONSTRAINT fk_fixed_assets_division_id FOREIGN KEY (division_id) REFERENCES divisions (id),
    CONSTRAINT fk_fixed_assets_payment_account_id FOREIGN KEY (payment_account_id) REFERENCES accounts (id),
    CONSTRAINT fk_fixed_assets_disposal_account_id FOREIGN KEY (disposal_account_id) REFERENCES accounts (id),
    CONSTRAINT fk_fixed_assets_acquisition_journal_id FOREIGN KEY (acquisition_journal_id) REFERENCES journal_entries (id),
    CONSTRAINT fk_fixed_assets_disposal_journal_id FOREIGN KEY (disposal_journal_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_fixed_assets_code CHECK (code ~ '^[A-Z0-9][A-Z0-9._/-]{0,39}$'),
    CONSTRAINT ck_fixed_assets_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_fixed_assets_source CHECK (acquisition_source IN ('purchase', 'opening')),
    CONSTRAINT ck_fixed_assets_status CHECK (status IN ('draft', 'active', 'disposed')),
    CONSTRAINT ck_fixed_assets_amounts CHECK (
        acquisition_cost_amount > 0 AND salvage_amount >= 0 AND salvage_amount < acquisition_cost_amount
        AND opening_accumulated_amount >= 0 AND accumulated_amount >= opening_accumulated_amount
        AND accumulated_amount <= acquisition_cost_amount - salvage_amount),
    CONSTRAINT ck_fixed_assets_useful_life CHECK (useful_life_months BETWEEN 1 AND 600
        AND opening_depreciated_months BETWEEN 0 AND useful_life_months),
    CONSTRAINT ck_fixed_assets_start CHECK (extract(day FROM depreciation_start_date) = 1),
    CONSTRAINT ck_fixed_assets_disposed CHECK ((status = 'disposed') = (disposal_date IS NOT NULL)),
    CONSTRAINT ck_fixed_assets_deleted CHECK (deleted_at IS NULL OR status = 'draft')
);
CREATE INDEX idx_fixed_assets_class_id ON fixed_assets (class_id);
CREATE INDEX idx_fixed_assets_division_id ON fixed_assets (division_id);
CREATE INDEX idx_fixed_assets_payment_account_id ON fixed_assets (payment_account_id);
CREATE INDEX idx_fixed_assets_disposal_account_id ON fixed_assets (disposal_account_id);
CREATE INDEX idx_fixed_assets_acquisition_journal_id ON fixed_assets (acquisition_journal_id);
CREATE INDEX idx_fixed_assets_disposal_journal_id ON fixed_assets (disposal_journal_id);
CREATE INDEX idx_fixed_assets_location_id ON fixed_assets (location_id);
CREATE INDEX idx_fixed_assets_status ON fixed_assets (status) WHERE deleted_at IS NULL;

-- Satu baris ringkasan per periode yang sudah dijalankan penyusutannya (job accounting.depreciate atau manual).
CREATE TABLE depreciation_runs (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    fiscal_period_id uuid          NOT NULL,
    asset_count      integer       NOT NULL DEFAULT 0,
    total_amount     numeric(18,2) NOT NULL DEFAULT 0,
    run_count        integer       NOT NULL DEFAULT 1,
    last_run_at      timestamptz   NOT NULL DEFAULT now(),
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    updated_at       timestamptz   NOT NULL DEFAULT now(),
    updated_by       uuid          NOT NULL,
    version          integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_depreciation_runs PRIMARY KEY (id),
    CONSTRAINT uq_depreciation_runs_fiscal_period_id UNIQUE (fiscal_period_id),
    CONSTRAINT fk_depreciation_runs_fiscal_period_id FOREIGN KEY (fiscal_period_id) REFERENCES fiscal_periods (id),
    CONSTRAINT ck_depreciation_runs_totals CHECK (asset_count >= 0 AND total_amount >= 0 AND run_count >= 1)
);

-- Penyusutan per aset per periode: unik (A-31 tidak dobel per periode, idempotent saat job diulang).
CREATE TABLE depreciation_lines (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    depreciation_run_id  uuid          NOT NULL,
    fixed_asset_id       uuid          NOT NULL,
    fiscal_period_id     uuid          NOT NULL,
    amount               numeric(18,2) NOT NULL,
    accumulated_amount   numeric(18,2) NOT NULL,
    book_value_amount    numeric(18,2) NOT NULL,
    journal_entry_id     uuid          NOT NULL,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    created_by           uuid          NOT NULL,
    CONSTRAINT pk_depreciation_lines PRIMARY KEY (id),
    CONSTRAINT uq_depreciation_lines_asset_period UNIQUE (fixed_asset_id, fiscal_period_id),
    CONSTRAINT fk_depreciation_lines_depreciation_run_id FOREIGN KEY (depreciation_run_id) REFERENCES depreciation_runs (id),
    CONSTRAINT fk_depreciation_lines_fixed_asset_id FOREIGN KEY (fixed_asset_id) REFERENCES fixed_assets (id),
    CONSTRAINT fk_depreciation_lines_fiscal_period_id FOREIGN KEY (fiscal_period_id) REFERENCES fiscal_periods (id),
    CONSTRAINT fk_depreciation_lines_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_depreciation_lines_amounts CHECK (amount > 0 AND accumulated_amount >= amount AND book_value_amount >= 0)
);
CREATE INDEX idx_depreciation_lines_depreciation_run_id ON depreciation_lines (depreciation_run_id);
CREATE INDEX idx_depreciation_lines_fiscal_period_id ON depreciation_lines (fiscal_period_id);
CREATE INDEX idx_depreciation_lines_journal_entry_id ON depreciation_lines (journal_entry_id);

-- Pelepasan aset (A-31): akumulasi + hasil penjualan + rugi = harga perolehan + laba.
INSERT INTO posting_rule_accounts (
    posting_rule_code, amount_key, side, account_resolver, account_id, valid_from, created_by, updated_by)
SELECT m.rule, m.amount_key, m.side, m.resolver, a.id, DATE '2026-01-01',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('A-31', 'disposal_accumulated', 'debit', 'accumulated_depreciation_account', '1592'),
    ('A-31', 'disposal_proceeds', 'debit', 'payment_account', '1111'),
    ('A-31', 'disposal_loss', 'debit', 'fixed', '7201'),
    ('A-31', 'disposal_cost', 'credit', 'fixed_asset_account', '1502'),
    ('A-31', 'disposal_gain', 'credit', 'fixed', '7101')
) AS m (rule, amount_key, side, resolver, account_code)
JOIN accounts a ON a.code = m.account_code;

-- +goose Down
DELETE FROM posting_rule_accounts WHERE posting_rule_code = 'A-31' AND amount_key LIKE 'disposal_%';
DROP TABLE depreciation_lines;
DROP TABLE depreciation_runs;
DROP TABLE fixed_assets;
DROP TABLE fixed_asset_classes;
