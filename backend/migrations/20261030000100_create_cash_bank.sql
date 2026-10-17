-- +goose Up
-- Kas & Bank dan rekonsiliasi bank (S-401, S-402; BRD ACC-05; Desain Data Model bagian accounting;
-- Technical Guideline 5.8.5).

-- Rekening kas/bank Balans yang terhubung ke satu akun Kas & Bank (11xx) di COA. Nomor rekening terenkripsi
-- (pola party_bank_accounts); account_number_last4 untuk tampilan tersamar. Kas Kecil (1102) dikelola lewat
-- dana kas kecil, bukan rekening ini.
CREATE TABLE bank_accounts (
    id                   uuid        NOT NULL DEFAULT uuidv7(),
    code                 text        NOT NULL,
    name                 text        NOT NULL,
    kind                 text        NOT NULL DEFAULT 'bank',
    bank_name            text        NOT NULL DEFAULT '',
    account_number_enc   text        NOT NULL DEFAULT '',
    account_number_last4 text        NOT NULL DEFAULT '',
    account_holder       text        NOT NULL DEFAULT '',
    account_id           uuid        NOT NULL,
    is_active            boolean     NOT NULL DEFAULT true,
    description          text        NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    created_by           uuid        NOT NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    updated_by           uuid        NOT NULL,
    version              integer     NOT NULL DEFAULT 1,
    deleted_at           timestamptz,
    deleted_by           uuid,
    CONSTRAINT pk_bank_accounts PRIMARY KEY (id),
    CONSTRAINT uq_bank_accounts_code UNIQUE (code),
    CONSTRAINT fk_bank_accounts_account_id FOREIGN KEY (account_id) REFERENCES accounts (id),
    CONSTRAINT ck_bank_accounts_code CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{1,29}$'),
    CONSTRAINT ck_bank_accounts_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_bank_accounts_kind CHECK (kind IN ('bank', 'cash')),
    CONSTRAINT ck_bank_accounts_bank CHECK (kind = 'cash' OR (bank_name <> '' AND account_number_enc <> ''))
);
-- Satu akun GL hanya untuk satu rekening aktif (belum dihapus) agar mutasi rekening = Buku Besar akun.
CREATE UNIQUE INDEX uq_bank_accounts_account_id ON bank_accounts (account_id) WHERE deleted_at IS NULL;

-- Pembayaran masuk/keluar (BYR). Jurnal A-PB: masuk Dr akun rekening / Cr akun lawan; keluar Dr akun lawan /
-- Cr akun rekening. source_* menautkan dokumen yang dibayar (pengajuan biaya, tagihan pemasok, invoice).
CREATE TABLE payments (
    id                 uuid          NOT NULL DEFAULT uuidv7(),
    number             text,
    direction          text          NOT NULL,
    bank_account_id    uuid          NOT NULL,
    payment_date       date          NOT NULL,
    amount             numeric(18,2) NOT NULL,
    counter_account_id uuid          NOT NULL,
    party_type         text          NOT NULL DEFAULT '',
    party_id           uuid,
    party_name         text          NOT NULL DEFAULT '',
    reference          text          NOT NULL DEFAULT '',
    description        text          NOT NULL DEFAULT '',
    proof_file_id      uuid,
    source_type        text          NOT NULL DEFAULT '',
    source_id          uuid,
    source_label       text          NOT NULL DEFAULT '',
    status             text          NOT NULL DEFAULT 'draft',
    journal_entry_id   uuid,
    posted_at          timestamptz,
    posted_by          uuid,
    cancelled_at       timestamptz,
    cancelled_by       uuid,
    cancel_reason      text          NOT NULL DEFAULT '',
    created_at         timestamptz   NOT NULL DEFAULT now(),
    created_by         uuid          NOT NULL,
    updated_at         timestamptz   NOT NULL DEFAULT now(),
    updated_by         uuid          NOT NULL,
    version            integer       NOT NULL DEFAULT 1,
    deleted_at         timestamptz,
    deleted_by         uuid,
    CONSTRAINT pk_payments PRIMARY KEY (id),
    CONSTRAINT uq_payments_number UNIQUE (number),
    CONSTRAINT fk_payments_bank_account_id FOREIGN KEY (bank_account_id) REFERENCES bank_accounts (id),
    CONSTRAINT fk_payments_counter_account_id FOREIGN KEY (counter_account_id) REFERENCES accounts (id),
    CONSTRAINT fk_payments_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_payments_direction CHECK (direction IN ('incoming', 'outgoing')),
    CONSTRAINT ck_payments_amount CHECK (amount > 0),
    CONSTRAINT ck_payments_status CHECK (status IN ('draft', 'posted', 'cancelled')),
    CONSTRAINT ck_payments_party CHECK ((party_type = '') = (party_id IS NULL)),
    CONSTRAINT ck_payments_posted CHECK (status = 'draft' OR (number IS NOT NULL AND journal_entry_id IS NOT NULL)
        OR (status = 'cancelled' AND number IS NULL)),
    CONSTRAINT ck_payments_deleted CHECK (deleted_at IS NULL OR status = 'draft')
);
CREATE INDEX idx_payments_bank_account_id ON payments (bank_account_id, payment_date DESC);
CREATE INDEX idx_payments_counter_account_id ON payments (counter_account_id);
CREATE INDEX idx_payments_journal_entry_id ON payments (journal_entry_id);
CREATE INDEX idx_payments_source ON payments (source_type, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_payments_status ON payments (status, payment_date DESC) WHERE deleted_at IS NULL;

-- File mutasi rekening yang diimpor (CSV/Excel). Satu file hanya sekali diimpor.
CREATE TABLE bank_statements (
    id              uuid          NOT NULL DEFAULT uuidv7(),
    bank_account_id uuid          NOT NULL,
    file_id         uuid          NOT NULL,
    file_name       text          NOT NULL DEFAULT '',
    period_from     date          NOT NULL,
    period_to       date          NOT NULL,
    line_count      integer       NOT NULL DEFAULT 0,
    total_in        numeric(18,2) NOT NULL DEFAULT 0,
    total_out       numeric(18,2) NOT NULL DEFAULT 0,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    created_by      uuid          NOT NULL,
    updated_at      timestamptz   NOT NULL DEFAULT now(),
    updated_by      uuid          NOT NULL,
    version         integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_bank_statements PRIMARY KEY (id),
    CONSTRAINT uq_bank_statements_file_id UNIQUE (file_id),
    CONSTRAINT fk_bank_statements_bank_account_id FOREIGN KEY (bank_account_id) REFERENCES bank_accounts (id),
    CONSTRAINT ck_bank_statements_period CHECK (period_to >= period_from)
);
CREATE INDEX idx_bank_statements_bank_account_id ON bank_statements (bank_account_id, period_to DESC);

-- Baris mutasi bank. amount bertanda: positif = dana masuk (kredit di rekening koran), negatif = keluar.
-- Usulan dan hasil pencocokan disimpan di baris (1 baris mutasi ↔ 1 baris jurnal akun rekening):
--   match_kind `journal_line` = transaksi yang sudah dijurnal, `document` = dokumen terbuka dari penyedia kandidat
--   (piutang/utang/pengajuan biaya) yang dibayar saat konfirmasi, `new_entry` = jurnal baru dari baris tak cocok.
CREATE TABLE bank_statement_lines (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    statement_id          uuid          NOT NULL,
    line_no               integer       NOT NULL,
    transaction_date      date          NOT NULL,
    description           text          NOT NULL DEFAULT '',
    reference             text          NOT NULL DEFAULT '',
    amount                numeric(18,2) NOT NULL,
    balance               numeric(18,2),
    status                text          NOT NULL DEFAULT 'unmatched',
    match_kind            text          NOT NULL DEFAULT '',
    match_rule            text          NOT NULL DEFAULT '',
    journal_line_id       uuid,
    journal_entry_id      uuid,
    candidate_source      text          NOT NULL DEFAULT '',
    candidate_document_id uuid,
    candidate_label       text          NOT NULL DEFAULT '',
    payment_id            uuid,
    matched_at            timestamptz,
    matched_by            uuid,
    updated_at            timestamptz   NOT NULL DEFAULT now(),
    version               integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_bank_statement_lines PRIMARY KEY (id),
    CONSTRAINT uq_bank_statement_lines_line_no UNIQUE (statement_id, line_no),
    CONSTRAINT fk_bank_statement_lines_statement_id FOREIGN KEY (statement_id) REFERENCES bank_statements (id),
    CONSTRAINT fk_bank_statement_lines_journal_line_id FOREIGN KEY (journal_line_id) REFERENCES journal_lines (id),
    CONSTRAINT fk_bank_statement_lines_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT fk_bank_statement_lines_payment_id FOREIGN KEY (payment_id) REFERENCES payments (id),
    CONSTRAINT ck_bank_statement_lines_amount CHECK (amount <> 0),
    CONSTRAINT ck_bank_statement_lines_status CHECK (status IN ('unmatched', 'suggested', 'matched')),
    CONSTRAINT ck_bank_statement_lines_kind CHECK (match_kind IN ('', 'journal_line', 'document', 'new_entry')),
    CONSTRAINT ck_bank_statement_lines_rule CHECK (match_rule IN ('', 'reference', 'amount_date', 'manual')),
    CONSTRAINT ck_bank_statement_lines_matched CHECK (status <> 'matched' OR journal_line_id IS NOT NULL),
    CONSTRAINT ck_bank_statement_lines_unmatched CHECK (status <> 'unmatched' OR match_kind = '')
);
CREATE INDEX idx_bank_statement_lines_journal_line_id ON bank_statement_lines (journal_line_id);
CREATE INDEX idx_bank_statement_lines_journal_entry_id ON bank_statement_lines (journal_entry_id);
CREATE INDEX idx_bank_statement_lines_payment_id ON bank_statement_lines (payment_id);
-- Satu baris jurnal hanya bisa direkonsiliasi dengan satu baris mutasi.
CREATE UNIQUE INDEX uq_bank_statement_lines_reconciled ON bank_statement_lines (journal_line_id) WHERE status = 'matched';

-- Kas Besar (1101) disemai sebagai rekening kas. Rekening bank dibuat Finance & Accounting karena nomor
-- rekening wajib terenkripsi.
INSERT INTO bank_accounts (code, name, kind, account_id, created_by, updated_by)
SELECT 'KAS-BESAR', a.name, 'cash', a.id, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM accounts a WHERE a.code = '1101';

-- +goose Down
DROP TABLE bank_statement_lines;
DROP TABLE bank_statements;
DROP TABLE payments;
DROP TABLE bank_accounts;
