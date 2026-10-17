-- +goose Up
-- Saldo awal migrasi per tanggal cut-over (ACC-14, A-36, U-10; Panduan Migrasi Data Fase 1) dan referensi
-- impor jurnal dari Excel (ACC-10, INT-02).

-- Satu set saldo awal untuk satu tanggal cut-over. Diposting sekali sebagai jurnal A-36.
CREATE TABLE opening_balance_sets (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    cutover_date     date        NOT NULL,
    status           text        NOT NULL DEFAULT 'draft',
    journal_entry_id uuid,
    posted_at        timestamptz,
    posted_by        uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_opening_balance_sets PRIMARY KEY (id),
    CONSTRAINT fk_opening_balance_sets_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_opening_balance_sets_status CHECK (status IN ('draft', 'posted')),
    CONSTRAINT ck_opening_balance_sets_posted CHECK ((status = 'posted') = (journal_entry_id IS NOT NULL AND posted_at IS NOT NULL))
);
CREATE UNIQUE INDEX uq_opening_balance_sets_single ON opening_balance_sets ((true));
CREATE INDEX idx_opening_balance_sets_journal_entry_id ON opening_balance_sets (journal_entry_id);

-- Neraca Saldo Awal per akun neraca (satu baris per akun).
CREATE TABLE opening_balance_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    opening_balance_set_id uuid          NOT NULL,
    account_id             uuid          NOT NULL,
    debit_amount           numeric(18,2) NOT NULL DEFAULT 0,
    credit_amount          numeric(18,2) NOT NULL DEFAULT 0,
    note                   text          NOT NULL DEFAULT '',
    created_at             timestamptz   NOT NULL DEFAULT now(),
    created_by             uuid          NOT NULL,
    updated_at             timestamptz   NOT NULL DEFAULT now(),
    updated_by             uuid          NOT NULL,
    version                integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_opening_balance_lines PRIMARY KEY (id),
    CONSTRAINT uq_opening_balance_lines_account UNIQUE (opening_balance_set_id, account_id),
    CONSTRAINT fk_opening_balance_lines_opening_balance_set_id FOREIGN KEY (opening_balance_set_id) REFERENCES opening_balance_sets (id),
    CONSTRAINT fk_opening_balance_lines_account_id FOREIGN KEY (account_id) REFERENCES accounts (id),
    CONSTRAINT ck_opening_balance_lines_amounts CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR (credit_amount > 0 AND debit_amount = 0))
);
CREATE INDEX idx_opening_balance_lines_account_id ON opening_balance_lines (account_id);

-- Piutang/utang terbuka per dokumen lama (U-10). Dibaca modul penjualan/pengadaan lewat API publik accounting
-- agar umur piutang/utang dan pelunasan mencakup saldo awal. party_id merujuk modul party tanpa FK.
CREATE TABLE opening_items (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    opening_balance_set_id uuid          NOT NULL,
    item_type              text          NOT NULL,
    party_type             text          NOT NULL,
    party_id               uuid          NOT NULL,
    party_code             text          NOT NULL,
    document_number        text          NOT NULL,
    document_date          date          NOT NULL,
    due_date               date          NOT NULL,
    amount                 numeric(18,2) NOT NULL,
    open_amount            numeric(18,2) NOT NULL,
    settled_amount         numeric(18,2) NOT NULL DEFAULT 0,
    account_id             uuid          NOT NULL,
    channel_id             uuid,
    note                   text          NOT NULL DEFAULT '',
    created_at             timestamptz   NOT NULL DEFAULT now(),
    created_by             uuid          NOT NULL,
    updated_at             timestamptz   NOT NULL DEFAULT now(),
    updated_by             uuid          NOT NULL,
    version                integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_opening_items PRIMARY KEY (id),
    CONSTRAINT uq_opening_items_document UNIQUE (item_type, party_type, party_id, document_number),
    CONSTRAINT fk_opening_items_opening_balance_set_id FOREIGN KEY (opening_balance_set_id) REFERENCES opening_balance_sets (id),
    CONSTRAINT fk_opening_items_account_id FOREIGN KEY (account_id) REFERENCES accounts (id),
    CONSTRAINT fk_opening_items_channel_id FOREIGN KEY (channel_id) REFERENCES channels (id),
    CONSTRAINT ck_opening_items_item_type CHECK (item_type IN ('receivable', 'payable')),
    CONSTRAINT ck_opening_items_party_type CHECK (party_type IN ('customer', 'partner', 'supplier')),
    CONSTRAINT ck_opening_items_party CHECK ((item_type = 'payable') = (party_type = 'supplier')),
    CONSTRAINT ck_opening_items_document_number CHECK (length(btrim(document_number)) BETWEEN 1 AND 60),
    CONSTRAINT ck_opening_items_amounts CHECK (
        amount > 0 AND open_amount > 0 AND open_amount <= amount AND settled_amount >= 0 AND settled_amount <= open_amount),
    CONSTRAINT ck_opening_items_dates CHECK (due_date >= document_date)
);
CREATE INDEX idx_opening_items_opening_balance_set_id ON opening_items (opening_balance_set_id);
CREATE INDEX idx_opening_items_account_id ON opening_items (account_id);
CREATE INDEX idx_opening_items_channel_id ON opening_items (channel_id);
CREATE INDEX idx_opening_items_party ON opening_items (party_type, party_id);

-- Pelunasan saldo awal oleh dokumen pembayaran modul lain; unik per dokumen sumber (idempotent).
CREATE TABLE opening_item_settlements (
    id              uuid          NOT NULL DEFAULT uuidv7(),
    opening_item_id uuid          NOT NULL,
    source_type     text          NOT NULL,
    source_id       uuid          NOT NULL,
    business_date   date          NOT NULL,
    amount          numeric(18,2) NOT NULL,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    created_by      uuid          NOT NULL,
    CONSTRAINT pk_opening_item_settlements PRIMARY KEY (id),
    CONSTRAINT uq_opening_item_settlements_source UNIQUE (opening_item_id, source_type, source_id),
    CONSTRAINT fk_opening_item_settlements_opening_item_id FOREIGN KEY (opening_item_id) REFERENCES opening_items (id),
    CONSTRAINT ck_opening_item_settlements_amount CHECK (amount <> 0)
);

-- Draft Jurnal Umum hasil impor Excel: baris dengan Nomor Referensi yang sama di satu file menjadi satu jurnal.
CREATE TABLE journal_import_refs (
    import_key       text        NOT NULL,
    reference        text        NOT NULL,
    journal_entry_id uuid        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_journal_import_refs PRIMARY KEY (import_key, reference),
    CONSTRAINT fk_journal_import_refs_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id)
);
CREATE INDEX idx_journal_import_refs_journal_entry_id ON journal_import_refs (journal_entry_id);

-- +goose Down
DROP TABLE journal_import_refs;
DROP TABLE opening_item_settlements;
DROP TABLE opening_items;
DROP TABLE opening_balance_lines;
DROP TABLE opening_balance_sets;
