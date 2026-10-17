-- +goose Up
-- PPh potong 21/23/4(2), bukti potong, penyetoran (A-29), dan PPh Final UMKM (A-30) — S-406, S-407; BRD ACC-19,
-- ACC-20; Technical Guideline 5.9, 5.14. Kalkulator di package tax; tarif effective-dated di sini.

-- Tarif per jenis pajak dan kode objek pajak (Coretax). brackets = lapisan tarif progresif Pasal 17
-- [{"upTo": "60000000.00", "ratePercent": "5"}, ...] (upTo kosong = tanpa batas); base_percent = DPP % bruto.
CREATE TABLE withholding_tax_rates (
    id                          uuid         NOT NULL DEFAULT uuidv7(),
    tax_type                    text         NOT NULL,
    object_code                 text         NOT NULL,
    name                        text         NOT NULL,
    rate_percent                numeric(9,4) NOT NULL DEFAULT 0,
    brackets                    jsonb        NOT NULL DEFAULT '[]',
    base_percent                numeric(9,4) NOT NULL DEFAULT 100,
    no_tax_id_surcharge_percent numeric(9,4) NOT NULL DEFAULT 0,
    is_default                  boolean      NOT NULL DEFAULT false,
    valid_from                  date         NOT NULL,
    valid_to                    date,
    note                        text         NOT NULL DEFAULT '',
    created_at                  timestamptz  NOT NULL DEFAULT now(),
    created_by                  uuid         NOT NULL,
    updated_at                  timestamptz  NOT NULL DEFAULT now(),
    updated_by                  uuid         NOT NULL,
    version                     integer      NOT NULL DEFAULT 1,
    CONSTRAINT pk_withholding_tax_rates PRIMARY KEY (id),
    CONSTRAINT ck_withholding_tax_rates_type CHECK (tax_type IN ('pph21', 'pph23', 'pph4_2', 'final_umkm')),
    CONSTRAINT ck_withholding_tax_rates_object CHECK (btrim(object_code) <> ''),
    CONSTRAINT ck_withholding_tax_rates_rate CHECK (rate_percent >= 0 AND rate_percent <= 100),
    CONSTRAINT ck_withholding_tax_rates_base CHECK (base_percent > 0 AND base_percent <= 100),
    CONSTRAINT ck_withholding_tax_rates_valid CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT ex_withholding_tax_rates_period EXCLUDE USING gist (
        tax_type WITH =, object_code WITH =, daterange(valid_from, valid_to, '[]') WITH &&)
);

-- Penyetoran pajak per jenis dan masa (A-29: Dr utang PPh / Cr bank). Satu penyetoran per jenis per masa.
CREATE TABLE tax_deposits (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    number           text          NOT NULL,
    tax_type         text          NOT NULL,
    tax_year         integer       NOT NULL,
    tax_month        integer       NOT NULL,
    bank_account_id  uuid          NOT NULL,
    deposit_date     date          NOT NULL,
    amount           numeric(18,2) NOT NULL,
    billing_code     text          NOT NULL DEFAULT '',
    ntpn             text          NOT NULL DEFAULT '',
    journal_entry_id uuid          NOT NULL,
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    CONSTRAINT pk_tax_deposits PRIMARY KEY (id),
    CONSTRAINT uq_tax_deposits_number UNIQUE (number),
    CONSTRAINT uq_tax_deposits_period UNIQUE (tax_type, tax_year, tax_month),
    CONSTRAINT fk_tax_deposits_bank_account_id FOREIGN KEY (bank_account_id) REFERENCES bank_accounts (id),
    CONSTRAINT fk_tax_deposits_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_tax_deposits_type CHECK (tax_type IN ('pph21', 'pph23', 'pph4_2', 'final_umkm')),
    CONSTRAINT ck_tax_deposits_month CHECK (tax_month BETWEEN 1 AND 12),
    CONSTRAINT ck_tax_deposits_amount CHECK (amount > 0)
);
CREATE INDEX idx_tax_deposits_bank_account_id ON tax_deposits (bank_account_id);
CREATE INDEX idx_tax_deposits_journal_entry_id ON tax_deposits (journal_entry_id);

-- Data bukti potong per dokumen sumber (pengajuan biaya, tagihan pemasok) per jenis + objek pajak. NPWP/NIK
-- penerima terenkripsi; slip_number = nomor bukti potong dari Coretax setelah diinput.
CREATE TABLE withholdings (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    source_type         text          NOT NULL,
    source_id           uuid          NOT NULL,
    source_label        text          NOT NULL DEFAULT '',
    business_date       date          NOT NULL,
    tax_year            integer       NOT NULL,
    tax_month           integer       NOT NULL,
    tax_type            text          NOT NULL,
    object_code         text          NOT NULL,
    object_name         text          NOT NULL DEFAULT '',
    rate_id             uuid          NOT NULL,
    taxpayer_type       text          NOT NULL DEFAULT '',
    taxpayer_id         uuid,
    taxpayer_name       text          NOT NULL,
    taxpayer_tax_id_enc text          NOT NULL DEFAULT '',
    has_tax_id          boolean       NOT NULL DEFAULT false,
    gross_amount        numeric(18,2) NOT NULL,
    taxable_base        numeric(18,2) NOT NULL,
    rate_percent        numeric(9,4)  NOT NULL,
    tax_amount          numeric(18,2) NOT NULL,
    status              text          NOT NULL DEFAULT 'recorded',
    deposit_id          uuid,
    slip_number         text          NOT NULL DEFAULT '',
    created_at          timestamptz   NOT NULL DEFAULT now(),
    created_by          uuid          NOT NULL,
    updated_at          timestamptz   NOT NULL DEFAULT now(),
    updated_by          uuid          NOT NULL,
    version             integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_withholdings PRIMARY KEY (id),
    CONSTRAINT fk_withholdings_rate_id FOREIGN KEY (rate_id) REFERENCES withholding_tax_rates (id),
    CONSTRAINT fk_withholdings_deposit_id FOREIGN KEY (deposit_id) REFERENCES tax_deposits (id),
    CONSTRAINT ck_withholdings_type CHECK (tax_type IN ('pph21', 'pph23', 'pph4_2')),
    CONSTRAINT ck_withholdings_month CHECK (tax_month BETWEEN 1 AND 12),
    CONSTRAINT ck_withholdings_amounts CHECK (gross_amount >= 0 AND taxable_base >= 0 AND tax_amount >= 0),
    CONSTRAINT ck_withholdings_status CHECK (status IN ('recorded', 'deposited', 'cancelled')),
    CONSTRAINT ck_withholdings_deposited CHECK ((status = 'deposited') = (deposit_id IS NOT NULL))
);
CREATE INDEX idx_withholdings_period ON withholdings (tax_year, tax_month, tax_type);
CREATE INDEX idx_withholdings_source ON withholdings (source_type, source_id);
CREATE INDEX idx_withholdings_rate_id ON withholdings (rate_id);
CREATE INDEX idx_withholdings_deposit_id ON withholdings (deposit_id);

-- Status PPh Final UMKM (aktif/nonaktif) effective-dated: baris dengan valid_from terakhir ≤ tanggal berlaku.
-- opening_turnover = omzet bruto kumulatif tahun opening_year sebelum cut-over (pemantauan batas Rp4,8 M).
CREATE TABLE final_tax_settings (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    valid_from       date          NOT NULL,
    is_enabled       boolean       NOT NULL,
    opening_year     integer,
    opening_turnover numeric(18,2) NOT NULL DEFAULT 0,
    note             text          NOT NULL DEFAULT '',
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    CONSTRAINT pk_final_tax_settings PRIMARY KEY (id),
    CONSTRAINT uq_final_tax_settings_valid_from UNIQUE (valid_from),
    CONSTRAINT ck_final_tax_settings_opening CHECK (opening_turnover >= 0)
);

-- Perhitungan PPh Final UMKM per masa (A-30: Dr 7203 / Cr 2304). journal_entry_id kosong bila pajak nol.
CREATE TABLE final_tax_runs (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    tax_year             integer       NOT NULL,
    tax_month            integer       NOT NULL,
    gross_revenue_amount numeric(18,2) NOT NULL,
    rate_id              uuid          NOT NULL,
    rate_percent         numeric(9,4)  NOT NULL,
    tax_amount           numeric(18,2) NOT NULL,
    journal_entry_id     uuid,
    deposit_id           uuid,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    created_by           uuid          NOT NULL,
    CONSTRAINT pk_final_tax_runs PRIMARY KEY (id),
    CONSTRAINT uq_final_tax_runs_period UNIQUE (tax_year, tax_month),
    CONSTRAINT fk_final_tax_runs_rate_id FOREIGN KEY (rate_id) REFERENCES withholding_tax_rates (id),
    CONSTRAINT fk_final_tax_runs_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT fk_final_tax_runs_deposit_id FOREIGN KEY (deposit_id) REFERENCES tax_deposits (id),
    CONSTRAINT ck_final_tax_runs_month CHECK (tax_month BETWEEN 1 AND 12),
    CONSTRAINT ck_final_tax_runs_amount CHECK (tax_amount >= 0)
);
CREATE INDEX idx_final_tax_runs_rate_id ON final_tax_runs (rate_id);
CREATE INDEX idx_final_tax_runs_journal_entry_id ON final_tax_runs (journal_entry_id);
CREATE INDEX idx_final_tax_runs_deposit_id ON final_tax_runs (deposit_id);

-- Tarif awal usulan Desain Finance & Accounting 4.2; nilai & kode objek wajib dikonfirmasi konsultan pajak
-- (F0-FIN-04) dan diubah lewat menu Pajak & Bukti Potong (versi baru, bukan update).
INSERT INTO withholding_tax_rates (tax_type, object_code, name, rate_percent, brackets, base_percent,
                                   no_tax_id_surcharge_percent, is_default, valid_from, note, created_by, updated_by)
VALUES
    ('pph23', '24-104-14', 'Jasa lain (maklon, desain, agensi, event organizer)', 2, '[]', 100, 100, true,
     DATE '2026-01-01', 'Usulan awal; konfirmasi konsultan pajak',
     '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('pph21', '21-100-08', 'Bukan pegawai (fee endorser/talent perorangan)', 0,
     '[{"upTo": "60000000.00", "ratePercent": "5"}, {"upTo": "250000000.00", "ratePercent": "15"},
       {"upTo": "500000000.00", "ratePercent": "25"}, {"upTo": "5000000000.00", "ratePercent": "30"},
       {"upTo": "", "ratePercent": "35"}]', 50, 20, true,
     DATE '2026-01-01', 'Tarif Pasal 17 × 50% bruto (PMK 168/2023); konfirmasi konsultan pajak',
     '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('pph4_2', '28-403-01', 'Sewa tanah dan/atau bangunan', 10, '[]', 100, 0, true,
     DATE '2026-01-01', 'Usulan awal; konfirmasi konsultan pajak',
     '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('final_umkm', 'UMKM', 'PPh Final UMKM (PP 55/2022)', 0.5, '[]', 100, 0, true,
     DATE '2026-01-01', 'Berlaku bila PPh Final UMKM diaktifkan',
     '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DROP TABLE final_tax_runs;
DROP TABLE final_tax_settings;
DROP TABLE withholdings;
DROP TABLE tax_deposits;
DROP TABLE withholding_tax_rates;
