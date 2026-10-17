-- +goose Up
-- Retur Penjualan (RJ) dan Nota Kredit (CN) R1.3 (S-314, S-317; SLS-08; Desain Data Model 8; Desain Status
-- Dokumen 5.5). Retur mereferensikan baris Sales Order yang sudah dikirim; barang masuk lokasi Inbound berstatus
-- Karantina (Inbound/QC modul warehouse) lalu ke Gudang Retur; setelah QC selesai jurnal A-13 diposting dan nota
-- kredit (atau refund) dibuat. Retur MT/reseller (pihak mitra) melewati verifikasi Logistik Leader dan pickup
-- Merchandiser. product_id, batch_id, channel_id, party_id, pickup_user_id, file ID merujuk tabel modul lain
-- tanpa FK (kepemilikan tabel per modul, TG 2.3).
CREATE TABLE sales_returns (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    number                 text,
    sales_order_id         uuid          NOT NULL,
    invoice_id             uuid,
    order_type             text          NOT NULL,
    channel_id             uuid          NOT NULL,
    sub_channel_id         uuid          NOT NULL,
    party_type             text          NOT NULL DEFAULT '',
    party_id               uuid,
    party_name             text          NOT NULL DEFAULT '',
    source_location_id     uuid          NOT NULL,
    return_date            date          NOT NULL,
    settlement_method      text          NOT NULL,
    requires_verification  boolean       NOT NULL DEFAULT false,
    note                   text          NOT NULL DEFAULT '',
    send_back_note         text          NOT NULL DEFAULT '',
    verified_by            uuid,
    verified_at            timestamptz,
    verify_note            text          NOT NULL DEFAULT '',
    pickup_required        boolean       NOT NULL DEFAULT false,
    pickup_user_id         uuid,
    pickup_assigned_at     timestamptz,
    picked_up_at           timestamptz,
    pickup_note            text          NOT NULL DEFAULT '',
    pickup_file_ids        uuid[]        NOT NULL DEFAULT '{}',
    received_by            uuid,
    received_at            timestamptz,
    receive_note           text          NOT NULL DEFAULT '',
    documentation_file_ids uuid[]        NOT NULL DEFAULT '{}',
    inbound_id             uuid,
    inbound_number         text          NOT NULL DEFAULT '',
    return_amount          numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount            numeric(18,2) NOT NULL DEFAULT 0,
    status                 text          NOT NULL DEFAULT 'draft',
    cancel_reason          text          NOT NULL DEFAULT '',
    posted_at              timestamptz,
    journal_entry_id       uuid,
    created_at             timestamptz   NOT NULL DEFAULT now(),
    created_by             uuid          NOT NULL,
    updated_at             timestamptz   NOT NULL DEFAULT now(),
    updated_by             uuid          NOT NULL,
    version                integer       NOT NULL DEFAULT 1,
    deleted_at             timestamptz,
    deleted_by             uuid,
    CONSTRAINT pk_sales_returns PRIMARY KEY (id),
    CONSTRAINT uq_sales_returns_number UNIQUE (number),
    CONSTRAINT fk_sales_returns_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT fk_sales_returns_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices (id),
    CONSTRAINT ck_sales_returns_settlement CHECK (settlement_method IN ('credit_note', 'refund', 'marketplace')),
    CONSTRAINT ck_sales_returns_party_type CHECK (party_type IN ('', 'customer', 'partner')),
    CONSTRAINT ck_sales_returns_status CHECK (status IN (
        'draft', 'submitted', 'verified', 'in_progress', 'posted', 'cancelled')),
    CONSTRAINT ck_sales_returns_amounts CHECK (return_amount >= 0 AND cost_amount >= 0),
    CONSTRAINT ck_sales_returns_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL)
);
CREATE INDEX idx_sales_returns_sales_order_id ON sales_returns (sales_order_id);
CREATE INDEX idx_sales_returns_invoice_id ON sales_returns (invoice_id);
CREATE INDEX idx_sales_returns_status ON sales_returns (status, created_at DESC);
CREATE INDEX idx_sales_returns_party ON sales_returns (party_type, party_id);
CREATE INDEX idx_sales_returns_pickup_user_id ON sales_returns (pickup_user_id) WHERE pickup_user_id IS NOT NULL;

CREATE TABLE sales_return_lines (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    sales_return_id     uuid          NOT NULL,
    line_no             integer       NOT NULL,
    sales_order_line_id uuid          NOT NULL,
    product_id          uuid          NOT NULL,
    batch_id            uuid,
    qty                 numeric(18,3) NOT NULL,
    return_reason_id    uuid          NOT NULL,
    unit_price_amount   numeric(18,2) NOT NULL,
    line_amount         numeric(18,2) NOT NULL,
    original_unit_cost  numeric(18,4) NOT NULL,
    cost_amount         numeric(18,2) NOT NULL,
    note                text          NOT NULL DEFAULT '',
    CONSTRAINT pk_sales_return_lines PRIMARY KEY (id),
    CONSTRAINT uq_sales_return_lines_line_no UNIQUE (sales_return_id, line_no),
    CONSTRAINT fk_sales_return_lines_sales_return_id FOREIGN KEY (sales_return_id) REFERENCES sales_returns (id),
    CONSTRAINT fk_sales_return_lines_sales_order_line_id FOREIGN KEY (sales_order_line_id) REFERENCES sales_order_lines (id),
    CONSTRAINT fk_sales_return_lines_return_reason_id FOREIGN KEY (return_reason_id) REFERENCES return_reasons (id),
    CONSTRAINT ck_sales_return_lines_qty CHECK (qty > 0),
    CONSTRAINT ck_sales_return_lines_amounts CHECK (unit_price_amount >= 0 AND line_amount >= 0
        AND original_unit_cost >= 0 AND cost_amount >= 0)
);
CREATE INDEX idx_sales_return_lines_sales_return_id ON sales_return_lines (sales_return_id);
CREATE INDEX idx_sales_return_lines_sales_order_line_id ON sales_return_lines (sales_order_line_id);
CREATE INDEX idx_sales_return_lines_return_reason_id ON sales_return_lines (return_reason_id);

-- Barang fisik per baris retur (bundel dipecah ke komponen sesuai alokasi Sales Order) dengan HPP asal; satu
-- item = satu baris Inbound (source_line_id) sehingga hasil QC tercatat per item.
CREATE TABLE sales_return_items (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    sales_return_id     uuid          NOT NULL,
    sales_return_line_id uuid         NOT NULL,
    allocation_id       uuid          NOT NULL,
    product_id          uuid          NOT NULL,
    batch_id            uuid,
    qty                 numeric(18,3) NOT NULL,
    unit_cost           numeric(18,4) NOT NULL,
    cost_amount         numeric(18,2) NOT NULL,
    passed_qty          numeric(18,3) NOT NULL DEFAULT 0,
    rejected_qty        numeric(18,3) NOT NULL DEFAULT 0,
    qc_done             boolean       NOT NULL DEFAULT false,
    CONSTRAINT pk_sales_return_items PRIMARY KEY (id),
    CONSTRAINT fk_sales_return_items_sales_return_id FOREIGN KEY (sales_return_id) REFERENCES sales_returns (id),
    CONSTRAINT fk_sales_return_items_sales_return_line_id FOREIGN KEY (sales_return_line_id) REFERENCES sales_return_lines (id),
    CONSTRAINT fk_sales_return_items_allocation_id FOREIGN KEY (allocation_id) REFERENCES sales_order_allocations (id),
    CONSTRAINT ck_sales_return_items_qty CHECK (qty > 0 AND passed_qty >= 0 AND rejected_qty >= 0)
);
CREATE INDEX idx_sales_return_items_sales_return_id ON sales_return_items (sales_return_id);
CREATE INDEX idx_sales_return_items_sales_return_line_id ON sales_return_items (sales_return_line_id);
CREATE INDEX idx_sales_return_items_allocation_id ON sales_return_items (allocation_id);

-- Nota Kredit (CN): pengurang piutang dari retur penjualan. `credit_note` menjadi saldo kredit pelanggan/mitra
-- yang dialokasikan ke invoice asal lalu invoice berikutnya; `refund` dibayarkan kembali (approval Head Finance &
-- Accounting, pembayaran keluar oleh F&A dengan re-autentikasi, jurnal A-PB).
CREATE TABLE credit_notes (
    id                      uuid          NOT NULL DEFAULT uuidv7(),
    number                  text          NOT NULL,
    sales_return_id         uuid          NOT NULL,
    invoice_id              uuid,
    party_type              text          NOT NULL,
    party_id                uuid          NOT NULL,
    party_name              text          NOT NULL,
    channel_id              uuid          NOT NULL,
    sub_channel_id          uuid          NOT NULL,
    settlement_method       text          NOT NULL,
    issue_date              date          NOT NULL,
    amount                  numeric(18,2) NOT NULL,
    applied_amount          numeric(18,2) NOT NULL DEFAULT 0,
    refunded_amount         numeric(18,2) NOT NULL DEFAULT 0,
    status                  text          NOT NULL,
    decision_note           text          NOT NULL DEFAULT '',
    refund_date             date,
    refund_account_code     text          NOT NULL DEFAULT '',
    refund_reference        text          NOT NULL DEFAULT '',
    refund_journal_entry_id uuid,
    created_at              timestamptz   NOT NULL DEFAULT now(),
    created_by              uuid          NOT NULL,
    updated_at              timestamptz   NOT NULL DEFAULT now(),
    updated_by              uuid          NOT NULL,
    version                 integer       NOT NULL DEFAULT 1,
    deleted_at              timestamptz,
    deleted_by              uuid,
    CONSTRAINT pk_credit_notes PRIMARY KEY (id),
    CONSTRAINT uq_credit_notes_number UNIQUE (number),
    CONSTRAINT uq_credit_notes_sales_return_id UNIQUE (sales_return_id),
    CONSTRAINT fk_credit_notes_sales_return_id FOREIGN KEY (sales_return_id) REFERENCES sales_returns (id),
    CONSTRAINT fk_credit_notes_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices (id),
    CONSTRAINT ck_credit_notes_party_type CHECK (party_type IN ('customer', 'partner')),
    CONSTRAINT ck_credit_notes_settlement CHECK (settlement_method IN ('credit_note', 'refund')),
    CONSTRAINT ck_credit_notes_status CHECK (status IN ('posted', 'completed', 'pending_approval', 'awaiting_payment', 'paid')),
    CONSTRAINT ck_credit_notes_amounts CHECK (amount > 0 AND applied_amount >= 0 AND refunded_amount >= 0
        AND applied_amount + refunded_amount <= amount)
);
CREATE INDEX idx_credit_notes_invoice_id ON credit_notes (invoice_id);
CREATE INDEX idx_credit_notes_party ON credit_notes (party_type, party_id);
CREATE INDEX idx_credit_notes_status ON credit_notes (status, created_at DESC);

CREATE TABLE credit_note_applications (
    id             uuid          NOT NULL DEFAULT uuidv7(),
    credit_note_id uuid          NOT NULL,
    invoice_id     uuid          NOT NULL,
    amount         numeric(18,2) NOT NULL,
    applied_date   date          NOT NULL,
    created_at     timestamptz   NOT NULL DEFAULT now(),
    created_by     uuid          NOT NULL,
    CONSTRAINT pk_credit_note_applications PRIMARY KEY (id),
    CONSTRAINT fk_credit_note_applications_credit_note_id FOREIGN KEY (credit_note_id) REFERENCES credit_notes (id),
    CONSTRAINT fk_credit_note_applications_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices (id),
    CONSTRAINT ck_credit_note_applications_amount CHECK (amount > 0)
);
CREATE INDEX idx_credit_note_applications_credit_note_id ON credit_note_applications (credit_note_id);
CREATE INDEX idx_credit_note_applications_invoice_id ON credit_note_applications (invoice_id);

-- Refund nota kredit (uang keluar ke pelanggan/mitra) disetujui Head Finance & Accounting (matriks 3.3
-- `sales.credit_note` A = Head F&A; `accounting.payment` keluar A = Head F&A).
INSERT INTO approval_policies (subject_type, level, min_amount, condition, approver_role_code, valid_from, created_by, updated_by)
VALUES ('sales_refund', 1, NULL, '', 'head_finance', '2026-01-01T00:00:00+07:00',
        '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DELETE FROM approval_policies WHERE subject_type = 'sales_refund';
DROP TABLE credit_note_applications;
DROP TABLE credit_notes;
DROP TABLE sales_return_items;
DROP TABLE sales_return_lines;
DROP TABLE sales_returns;
