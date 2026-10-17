-- +goose Up
-- Settlement marketplace tahap 1 lewat impor laporan settlement (S-403, ACC-06, F1-402). Satu settlement = satu
-- pencairan dana (marketplace + referensi pencairan); baris = order marketplace yang dicocokkan. Posting membuat
-- jurnal A-10s (Dr Bank dana cair + Beban Fee 6105 + Beban Promosi 6107, Cr Piutang Marketplace 121x) dan
-- mengurangi sisa piutang marketplace per order. channel_id, sub_channel_id merujuk tabel modul accounting tanpa FK
-- (TG 2.3).
CREATE TABLE marketplace_settlements (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    channel_id        uuid          NOT NULL,
    sub_channel_id    uuid          NOT NULL,
    marketplace_code  text          NOT NULL,
    payout_reference  text          NOT NULL,
    settlement_date   date          NOT NULL,
    bank_account_code text          NOT NULL,
    status            text          NOT NULL DEFAULT 'draft',
    line_count        integer       NOT NULL DEFAULT 0,
    settled_amount    numeric(18,2) NOT NULL DEFAULT 0,
    fee_amount        numeric(18,2) NOT NULL DEFAULT 0,
    promo_amount      numeric(18,2) NOT NULL DEFAULT 0,
    net_amount        numeric(18,2) NOT NULL DEFAULT 0,
    journal_entry_id  uuid,
    posted_at         timestamptz,
    posted_by         uuid,
    cancel_reason     text          NOT NULL DEFAULT '',
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_marketplace_settlements PRIMARY KEY (id),
    CONSTRAINT uq_marketplace_settlements_payout UNIQUE (sub_channel_id, payout_reference),
    CONSTRAINT ck_marketplace_settlements_status CHECK (status IN ('draft', 'posted', 'cancelled')),
    CONSTRAINT ck_marketplace_settlements_reference CHECK (length(btrim(payout_reference)) BETWEEN 1 AND 80),
    CONSTRAINT ck_marketplace_settlements_amounts CHECK (
        fee_amount >= 0 AND promo_amount >= 0 AND net_amount >= 0 AND settled_amount = net_amount + fee_amount + promo_amount),
    CONSTRAINT ck_marketplace_settlements_posted CHECK (
        status <> 'posted' OR (journal_entry_id IS NOT NULL AND posted_at IS NOT NULL))
);
CREATE INDEX idx_marketplace_settlements_status ON marketplace_settlements (status, settlement_date DESC);

CREATE TABLE marketplace_settlement_lines (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    settlement_id     uuid          NOT NULL,
    line_no           integer       NOT NULL,
    sales_order_id    uuid          NOT NULL,
    external_order_id text          NOT NULL,
    order_number      text          NOT NULL DEFAULT '',
    receivable_amount numeric(18,2) NOT NULL,
    fee_amount        numeric(18,2) NOT NULL DEFAULT 0,
    promo_amount      numeric(18,2) NOT NULL DEFAULT 0,
    net_amount        numeric(18,2) NOT NULL DEFAULT 0,
    settled_amount    numeric(18,2) NOT NULL,
    import_row_key    text,
    CONSTRAINT pk_marketplace_settlement_lines PRIMARY KEY (id),
    CONSTRAINT uq_marketplace_settlement_lines_line_no UNIQUE (settlement_id, line_no),
    CONSTRAINT uq_marketplace_settlement_lines_order UNIQUE (settlement_id, sales_order_id),
    CONSTRAINT uq_marketplace_settlement_lines_import_row_key UNIQUE (import_row_key),
    CONSTRAINT fk_marketplace_settlement_lines_settlement_id FOREIGN KEY (settlement_id) REFERENCES marketplace_settlements (id),
    CONSTRAINT fk_marketplace_settlement_lines_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT ck_marketplace_settlement_lines_amounts CHECK (
        fee_amount >= 0 AND promo_amount >= 0 AND net_amount >= 0 AND settled_amount > 0
        AND settled_amount = net_amount + fee_amount + promo_amount)
);
CREATE INDEX idx_marketplace_settlement_lines_sales_order_id ON marketplace_settlement_lines (sales_order_id);

-- +goose Down
DROP TABLE marketplace_settlement_lines;
DROP TABLE marketplace_settlements;
