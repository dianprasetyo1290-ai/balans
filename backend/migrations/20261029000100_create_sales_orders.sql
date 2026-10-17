-- +goose Up
-- Penjualan R1.3 (BRD 5.4, 5.7, 5.8, 5.13; Desain Data Model 8): satu model Sales Order untuk Modern Trade beli
-- putus, B2B, WhatsApp/Instagram, marketplace, dan karyawan; Proforma Invoice; Invoice NON-PKP; pembayaran
-- pelanggan dengan alokasi ke banyak invoice (ACC-03, ACC-15); pengiriman per Sales Order untuk HPP (A-06/A-10).
-- channel_id, sub_channel_id, customer_id, partner_id, product_id, location_id, batch_id merujuk tabel modul lain
-- tanpa FK (kepemilikan tabel per modul, TG 2.3).
CREATE TABLE sales_orders (
    id                         uuid          NOT NULL DEFAULT uuidv7(),
    number                     text,
    order_type                 text          NOT NULL,
    channel_id                 uuid          NOT NULL,
    sub_channel_id             uuid          NOT NULL,
    customer_id                uuid,
    partner_id                 uuid,
    party_code                 text          NOT NULL DEFAULT '',
    party_name                 text          NOT NULL DEFAULT '',
    party_phone                text          NOT NULL DEFAULT '',
    external_order_id          text          NOT NULL DEFAULT '',
    client_po_number           text          NOT NULL DEFAULT '',
    business_date              date          NOT NULL,
    source_location_id         uuid          NOT NULL,
    payment_mode               text          NOT NULL DEFAULT 'prepaid',
    payment_term_days          integer       NOT NULL DEFAULT 0,
    ship_to_name               text          NOT NULL DEFAULT '',
    ship_to_phone              text          NOT NULL DEFAULT '',
    ship_to_address            text          NOT NULL DEFAULT '',
    gross_amount               numeric(18,2) NOT NULL DEFAULT 0,
    line_discount_amount       numeric(18,2) NOT NULL DEFAULT 0,
    document_discount_kind     text          NOT NULL DEFAULT 'amount',
    document_discount_value    numeric(18,4) NOT NULL DEFAULT 0,
    document_discount_amount   numeric(18,2) NOT NULL DEFAULT 0,
    shipping_amount            numeric(18,2) NOT NULL DEFAULT 0,
    total_amount               numeric(18,2) NOT NULL DEFAULT 0,
    discount_approval_required boolean       NOT NULL DEFAULT false,
    discount_approved_at       timestamptz,
    status                     text          NOT NULL DEFAULT 'draft',
    note                       text          NOT NULL DEFAULT '',
    cancel_reason              text          NOT NULL DEFAULT '',
    confirmed_at               timestamptz,
    shipped_at                 timestamptz,
    delivered_at               timestamptz,
    payment_proof_file_id      uuid,
    payment_proof_uploaded_at  timestamptz,
    import_row_key             text,
    allocation_round           integer       NOT NULL DEFAULT 0,
    created_at                 timestamptz   NOT NULL DEFAULT now(),
    created_by                 uuid          NOT NULL,
    updated_at                 timestamptz   NOT NULL DEFAULT now(),
    updated_by                 uuid          NOT NULL,
    version                    integer       NOT NULL DEFAULT 1,
    deleted_at                 timestamptz,
    deleted_by                 uuid,
    CONSTRAINT pk_sales_orders PRIMARY KEY (id),
    CONSTRAINT uq_sales_orders_number UNIQUE (number),
    CONSTRAINT uq_sales_orders_import_row_key UNIQUE (import_row_key),
    CONSTRAINT ck_sales_orders_order_type CHECK (order_type IN ('modern_trade', 'b2b', 'social_commerce', 'marketplace', 'employee')),
    CONSTRAINT ck_sales_orders_payment_mode CHECK (payment_mode IN ('prepaid', 'credit')),
    CONSTRAINT ck_sales_orders_payment_term CHECK (payment_term_days BETWEEN 0 AND 365),
    CONSTRAINT ck_sales_orders_discount_kind CHECK (document_discount_kind IN ('percent', 'amount')),
    CONSTRAINT ck_sales_orders_status CHECK (status IN (
        'draft', 'pending_approval', 'awaiting_payment', 'in_progress', 'in_transit', 'completed', 'cancelled', 'expired')),
    CONSTRAINT ck_sales_orders_amounts CHECK (gross_amount >= 0 AND line_discount_amount >= 0
        AND document_discount_amount >= 0 AND shipping_amount >= 0 AND total_amount >= 0),
    CONSTRAINT ck_sales_orders_party CHECK (customer_id IS NULL OR partner_id IS NULL),
    CONSTRAINT ck_sales_orders_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL)
);
-- Nomor order marketplace tidak bisa ganda per marketplace (sub-channel), termasuk order yang dibatalkan (SLS-05).
CREATE UNIQUE INDEX uq_sales_orders_external_order ON sales_orders (sub_channel_id, external_order_id)
    WHERE external_order_id <> '';
CREATE INDEX idx_sales_orders_status ON sales_orders (status, business_date);
CREATE INDEX idx_sales_orders_customer_id ON sales_orders (customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_sales_orders_partner_id ON sales_orders (partner_id) WHERE partner_id IS NOT NULL;
CREATE INDEX idx_sales_orders_order_type ON sales_orders (order_type, created_at DESC);

CREATE TABLE sales_order_lines (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    sales_order_id        uuid          NOT NULL,
    line_no               integer       NOT NULL,
    product_id            uuid          NOT NULL,
    is_bundle             boolean       NOT NULL DEFAULT false,
    qty                   numeric(18,3) NOT NULL,
    unit_price_amount     numeric(18,2) NOT NULL,
    price_rule            text          NOT NULL,
    price_source_id       uuid,
    price_list_id         uuid,
    price_list_version_id uuid,
    price_description     text          NOT NULL DEFAULT '',
    discount_kind         text          NOT NULL DEFAULT 'amount',
    discount_value        numeric(18,4) NOT NULL DEFAULT 0,
    discount_amount       numeric(18,2) NOT NULL DEFAULT 0,
    gross_amount          numeric(18,2) NOT NULL,
    line_total_amount     numeric(18,2) NOT NULL,
    note                  text          NOT NULL DEFAULT '',
    CONSTRAINT pk_sales_order_lines PRIMARY KEY (id),
    CONSTRAINT uq_sales_order_lines_line_no UNIQUE (sales_order_id, line_no),
    CONSTRAINT fk_sales_order_lines_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT ck_sales_order_lines_qty CHECK (qty > 0),
    CONSTRAINT ck_sales_order_lines_amounts CHECK (unit_price_amount >= 0 AND discount_amount >= 0 AND gross_amount >= 0
        AND line_total_amount >= 0),
    CONSTRAINT ck_sales_order_lines_discount_kind CHECK (discount_kind IN ('percent', 'amount')),
    CONSTRAINT ck_sales_order_lines_price_rule CHECK (price_rule IN (
        'special_price', 'promotion', 'tiered_price', 'price_list', 'base_price', 'marketplace_input'))
);

-- Alokasi stok per komponen baris (bundel dipecah, TG 5.7.4); reservasi inventory memakai ID alokasi sebagai
-- source_line_id. Batch hasil FEFO dicatat di sales_order_allocation_batches.
CREATE TABLE sales_order_allocations (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    sales_order_id      uuid          NOT NULL,
    sales_order_line_id uuid          NOT NULL,
    product_id          uuid          NOT NULL,
    location_id         uuid          NOT NULL,
    qty                 numeric(18,3) NOT NULL,
    shipped_qty         numeric(18,3) NOT NULL DEFAULT 0,
    released            boolean       NOT NULL DEFAULT false,
    CONSTRAINT pk_sales_order_allocations PRIMARY KEY (id),
    CONSTRAINT fk_sales_order_allocations_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT fk_sales_order_allocations_sales_order_line_id FOREIGN KEY (sales_order_line_id) REFERENCES sales_order_lines (id),
    CONSTRAINT ck_sales_order_allocations_qty CHECK (qty > 0 AND shipped_qty >= 0 AND shipped_qty <= qty)
);
CREATE INDEX idx_sales_order_allocations_sales_order_id ON sales_order_allocations (sales_order_id);
CREATE INDEX idx_sales_order_allocations_sales_order_line_id ON sales_order_allocations (sales_order_line_id);

CREATE TABLE sales_order_allocation_batches (
    id            uuid          NOT NULL DEFAULT uuidv7(),
    allocation_id uuid          NOT NULL,
    batch_id      uuid,
    qty           numeric(18,3) NOT NULL,
    CONSTRAINT pk_sales_order_allocation_batches PRIMARY KEY (id),
    CONSTRAINT fk_sales_order_allocation_batches_allocation_id FOREIGN KEY (allocation_id) REFERENCES sales_order_allocations (id),
    CONSTRAINT ck_sales_order_allocation_batches_qty CHECK (qty > 0)
);
CREATE INDEX idx_sales_order_allocation_batches_allocation_id ON sales_order_allocation_batches (allocation_id);

-- Proforma Invoice (BRD 5.4): permintaan pembayaran berlaku 14 hari; kedaluwarsa melepas alokasi.
CREATE TABLE proforma_invoices (
    id             uuid          NOT NULL DEFAULT uuidv7(),
    number         text          NOT NULL,
    sales_order_id uuid          NOT NULL,
    amount         numeric(18,2) NOT NULL,
    issue_date     date          NOT NULL,
    valid_until    date          NOT NULL,
    status         text          NOT NULL DEFAULT 'awaiting_payment',
    paid_at        timestamptz,
    invoice_id     uuid,
    cancel_reason  text          NOT NULL DEFAULT '',
    created_at     timestamptz   NOT NULL DEFAULT now(),
    created_by     uuid          NOT NULL,
    updated_at     timestamptz   NOT NULL DEFAULT now(),
    updated_by     uuid          NOT NULL,
    version        integer       NOT NULL DEFAULT 1,
    deleted_at     timestamptz,
    deleted_by     uuid,
    CONSTRAINT pk_proforma_invoices PRIMARY KEY (id),
    CONSTRAINT uq_proforma_invoices_number UNIQUE (number),
    CONSTRAINT fk_proforma_invoices_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT ck_proforma_invoices_status CHECK (status IN ('awaiting_payment', 'paid', 'expired', 'cancelled')),
    CONSTRAINT ck_proforma_invoices_amount CHECK (amount >= 0),
    CONSTRAINT ck_proforma_invoices_validity CHECK (valid_until >= issue_date)
);
CREATE INDEX idx_proforma_invoices_sales_order_id ON proforma_invoices (sales_order_id);
CREATE INDEX idx_proforma_invoices_open ON proforma_invoices (valid_until) WHERE status = 'awaiting_payment';

-- Invoice NON-PKP (SLS-02, D-04): tanpa PPN; piutang per invoice (ACC-03).
CREATE TABLE invoices (
    id              uuid          NOT NULL DEFAULT uuidv7(),
    number          text          NOT NULL,
    invoice_type    text          NOT NULL DEFAULT 'sales',
    sales_order_id  uuid,
    party_type      text          NOT NULL,
    party_id        uuid          NOT NULL,
    party_code      text          NOT NULL DEFAULT '',
    party_name      text          NOT NULL,
    channel_id      uuid          NOT NULL,
    sub_channel_id  uuid          NOT NULL,
    invoice_date    date          NOT NULL,
    due_date        date          NOT NULL,
    gross_amount    numeric(18,2) NOT NULL,
    discount_amount numeric(18,2) NOT NULL DEFAULT 0,
    shipping_amount numeric(18,2) NOT NULL DEFAULT 0,
    total_amount    numeric(18,2) NOT NULL,
    paid_amount     numeric(18,2) NOT NULL DEFAULT 0,
    status          text          NOT NULL,
    cancel_reason   text          NOT NULL DEFAULT '',
    cancelled_at    timestamptz,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    created_by      uuid          NOT NULL,
    updated_at      timestamptz   NOT NULL DEFAULT now(),
    updated_by      uuid          NOT NULL,
    version         integer       NOT NULL DEFAULT 1,
    deleted_at      timestamptz,
    deleted_by      uuid,
    CONSTRAINT pk_invoices PRIMARY KEY (id),
    CONSTRAINT uq_invoices_number UNIQUE (number),
    CONSTRAINT fk_invoices_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id),
    CONSTRAINT ck_invoices_invoice_type CHECK (invoice_type IN ('sales', 'consignment', 'variance')),
    CONSTRAINT ck_invoices_party_type CHECK (party_type IN ('customer', 'partner')),
    CONSTRAINT ck_invoices_status CHECK (status IN ('awaiting_payment', 'partially_paid', 'paid', 'cancelled')),
    CONSTRAINT ck_invoices_amounts CHECK (gross_amount >= 0 AND discount_amount >= 0 AND shipping_amount >= 0
        AND total_amount >= 0 AND paid_amount >= 0 AND paid_amount <= total_amount),
    CONSTRAINT ck_invoices_due_date CHECK (due_date >= invoice_date)
);
CREATE INDEX idx_invoices_sales_order_id ON invoices (sales_order_id);
CREATE INDEX idx_invoices_party ON invoices (party_type, party_id);
CREATE INDEX idx_invoices_open ON invoices (due_date) WHERE status IN ('awaiting_payment', 'partially_paid');

CREATE TABLE invoice_lines (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    invoice_id        uuid          NOT NULL,
    line_no           integer       NOT NULL,
    product_id        uuid,
    description       text          NOT NULL,
    qty               numeric(18,3) NOT NULL,
    unit_price_amount numeric(18,2) NOT NULL,
    discount_amount   numeric(18,2) NOT NULL DEFAULT 0,
    line_total_amount numeric(18,2) NOT NULL,
    CONSTRAINT pk_invoice_lines PRIMARY KEY (id),
    CONSTRAINT uq_invoice_lines_line_no UNIQUE (invoice_id, line_no),
    CONSTRAINT fk_invoice_lines_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices (id)
);

-- Pembayaran pelanggan (ACC-15): satu pembayaran dialokasikan ke satu atau banyak invoice; jurnal A-PB.
CREATE TABLE customer_payments (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    number           text          NOT NULL,
    party_type       text          NOT NULL,
    party_id         uuid          NOT NULL,
    party_name       text          NOT NULL,
    payment_date     date          NOT NULL,
    amount           numeric(18,2) NOT NULL,
    method           text          NOT NULL,
    account_code     text          NOT NULL,
    reference        text          NOT NULL DEFAULT '',
    proof_file_id    uuid,
    note             text          NOT NULL DEFAULT '',
    status           text          NOT NULL DEFAULT 'posted',
    cancel_reason    text          NOT NULL DEFAULT '',
    journal_entry_id uuid,
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    updated_at       timestamptz   NOT NULL DEFAULT now(),
    updated_by       uuid          NOT NULL,
    version          integer       NOT NULL DEFAULT 1,
    deleted_at       timestamptz,
    deleted_by       uuid,
    CONSTRAINT pk_customer_payments PRIMARY KEY (id),
    CONSTRAINT uq_customer_payments_number UNIQUE (number),
    CONSTRAINT ck_customer_payments_party_type CHECK (party_type IN ('customer', 'partner')),
    CONSTRAINT ck_customer_payments_method CHECK (method IN ('bank_transfer', 'cash')),
    CONSTRAINT ck_customer_payments_status CHECK (status IN ('posted', 'cancelled')),
    CONSTRAINT ck_customer_payments_amount CHECK (amount > 0)
);
CREATE INDEX idx_customer_payments_party ON customer_payments (party_type, party_id);
CREATE INDEX idx_customer_payments_account_date ON customer_payments (account_code, payment_date);

CREATE TABLE customer_payment_allocations (
    id         uuid          NOT NULL DEFAULT uuidv7(),
    payment_id uuid          NOT NULL,
    invoice_id uuid          NOT NULL,
    amount     numeric(18,2) NOT NULL,
    CONSTRAINT pk_customer_payment_allocations PRIMARY KEY (id),
    CONSTRAINT uq_customer_payment_allocations_invoice UNIQUE (payment_id, invoice_id),
    CONSTRAINT fk_customer_payment_allocations_payment_id FOREIGN KEY (payment_id) REFERENCES customer_payments (id),
    CONSTRAINT fk_customer_payment_allocations_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices (id),
    CONSTRAINT ck_customer_payment_allocations_amount CHECK (amount > 0)
);
CREATE INDEX idx_customer_payment_allocations_invoice_id ON customer_payment_allocations (invoice_id);

-- Pengiriman per Sales Order (DO ship / serah ke kurir marketplace): stok reserved keluar dan HPP dijurnal
-- (A-06; marketplace A-10). Satu dokumen pengiriman sumber hanya sekali per Sales Order.
CREATE TABLE sales_shipments (
    id             uuid          NOT NULL DEFAULT uuidv7(),
    sales_order_id uuid          NOT NULL,
    source_type    text          NOT NULL,
    source_id      uuid          NOT NULL,
    source_label   text          NOT NULL DEFAULT '',
    business_date  date          NOT NULL,
    cost_amount    numeric(18,2) NOT NULL,
    created_at     timestamptz   NOT NULL DEFAULT now(),
    created_by     uuid          NOT NULL,
    CONSTRAINT pk_sales_shipments PRIMARY KEY (id),
    CONSTRAINT uq_sales_shipments_source UNIQUE (sales_order_id, source_type, source_id),
    CONSTRAINT fk_sales_shipments_sales_order_id FOREIGN KEY (sales_order_id) REFERENCES sales_orders (id)
);

CREATE TABLE sales_shipment_lines (
    id            uuid          NOT NULL DEFAULT uuidv7(),
    shipment_id   uuid          NOT NULL,
    allocation_id uuid          NOT NULL,
    product_id    uuid          NOT NULL,
    qty           numeric(18,3) NOT NULL,
    cost_amount   numeric(18,2) NOT NULL,
    CONSTRAINT pk_sales_shipment_lines PRIMARY KEY (id),
    CONSTRAINT fk_sales_shipment_lines_shipment_id FOREIGN KEY (shipment_id) REFERENCES sales_shipments (id),
    CONSTRAINT fk_sales_shipment_lines_allocation_id FOREIGN KEY (allocation_id) REFERENCES sales_order_allocations (id),
    CONSTRAINT ck_sales_shipment_lines_qty CHECK (qty > 0)
);
CREATE INDEX idx_sales_shipment_lines_shipment_id ON sales_shipment_lines (shipment_id);
CREATE INDEX idx_sales_shipment_lines_allocation_id ON sales_shipment_lines (allocation_id);

-- Ongkos kirim WA/IG ditagih ke pelanggan → Pendapatan Ongkos Kirim (4201) di jurnal A-05.
INSERT INTO posting_rule_accounts (
    posting_rule_code, amount_key, side, account_resolver, account_id, valid_from, created_by, updated_by)
SELECT 'A-05', 'shipping_income', 'credit', 'fixed', a.id, DATE '2026-01-01',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM accounts a WHERE a.code = '4201';

-- Jurnal penjualan wajib channel + sub-channel (Desain Finance & Accounting 3.4); channel Penjualan Karyawan
-- belum punya sub-channel.
INSERT INTO sub_channels (channel_id, code, name, created_by, updated_by)
SELECT c.id, 'karyawan', 'Karyawan', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM channels c WHERE c.code = 'employee'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM sub_channels s USING channels c
WHERE s.channel_id = c.id AND c.code = 'employee' AND s.code = 'karyawan';
DELETE FROM posting_rule_accounts WHERE posting_rule_code = 'A-05' AND amount_key = 'shipping_income';
DROP TABLE sales_shipment_lines;
DROP TABLE sales_shipments;
DROP TABLE customer_payment_allocations;
DROP TABLE customer_payments;
DROP TABLE invoice_lines;
DROP TABLE invoices;
DROP TABLE proforma_invoices;
DROP TABLE sales_order_allocation_batches;
DROP TABLE sales_order_allocations;
DROP TABLE sales_order_lines;
DROP TABLE sales_orders;
