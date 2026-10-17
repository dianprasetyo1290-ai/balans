-- +goose Up
-- Modul procurement R1.2: Purchase Request, Purchase Order (barang/packaging, jasa maklon, reprocess), dan uang
-- muka pembelian (PRC-01 – PRC-03, PRC-06, PRC-08, PRC-09; Desain Data Model 5, Desain Status Dokumen 2.1 – 2.2).
-- division_id merujuk accounting.divisions, product_id ke catalog.products, supplier_id ke party.suppliers, dan
-- location_id ke warehouse.locations tanpa FK lintas modul (Technical Guideline 2.3).

CREATE TABLE purchase_requests (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    -- Nomor PR/TAHUN/BULAN/NOMOR diberikan saat diajukan.
    number                 text,
    division_id            uuid          NOT NULL,
    requested_by           uuid          NOT NULL,
    business_date          date          NOT NULL,
    needed_date            date,
    note                   text          NOT NULL DEFAULT '',
    estimated_total_amount numeric(18,2) NOT NULL DEFAULT 0,
    status                 text          NOT NULL DEFAULT 'draft',
    close_reason           text          NOT NULL DEFAULT '',
    submitted_at           timestamptz,
    decided_at             timestamptz,
    created_at             timestamptz   NOT NULL DEFAULT now(),
    created_by             uuid          NOT NULL,
    updated_at             timestamptz   NOT NULL DEFAULT now(),
    updated_by             uuid          NOT NULL,
    version                integer       NOT NULL DEFAULT 1,
    deleted_at             timestamptz,
    deleted_by             uuid,
    CONSTRAINT pk_purchase_requests PRIMARY KEY (id),
    CONSTRAINT uq_purchase_requests_number UNIQUE (number),
    CONSTRAINT ck_purchase_requests_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'completed', 'closed', 'cancelled')),
    CONSTRAINT ck_purchase_requests_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_purchase_requests_close_reason CHECK (status <> 'closed' OR btrim(close_reason) <> ''),
    CONSTRAINT ck_purchase_requests_estimated_total CHECK (estimated_total_amount >= 0)
);
CREATE INDEX idx_purchase_requests_status ON purchase_requests (status, business_date DESC);
CREATE INDEX idx_purchase_requests_division_id ON purchase_requests (division_id);
CREATE INDEX idx_purchase_requests_requested_by ON purchase_requests (requested_by);

CREATE TABLE purchase_request_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    purchase_request_id    uuid          NOT NULL,
    line_no                integer       NOT NULL,
    product_id             uuid,
    description            text          NOT NULL DEFAULT '',
    qty                    numeric(18,3) NOT NULL,
    estimated_price_amount numeric(18,2) NOT NULL DEFAULT 0,
    -- Qty yang sudah dijadikan PO (PO dibatalkan tidak dihitung; PO ditutup dihitung sebesar qty diterima).
    fulfilled_qty          numeric(18,3) NOT NULL DEFAULT 0,
    note                   text          NOT NULL DEFAULT '',
    CONSTRAINT pk_purchase_request_lines PRIMARY KEY (id),
    CONSTRAINT fk_purchase_request_lines_purchase_request_id FOREIGN KEY (purchase_request_id)
        REFERENCES purchase_requests (id) ON DELETE CASCADE,
    CONSTRAINT uq_purchase_request_lines_line_no UNIQUE (purchase_request_id, line_no),
    CONSTRAINT ck_purchase_request_lines_item CHECK (product_id IS NOT NULL OR btrim(description) <> ''),
    CONSTRAINT ck_purchase_request_lines_qty CHECK (qty > 0),
    CONSTRAINT ck_purchase_request_lines_price CHECK (estimated_price_amount >= 0),
    CONSTRAINT ck_purchase_request_lines_fulfilled CHECK (fulfilled_qty >= 0 AND fulfilled_qty <= qty)
);
CREATE INDEX idx_purchase_request_lines_product_id ON purchase_request_lines (product_id);

CREATE TABLE purchase_orders (
    id                          uuid          NOT NULL DEFAULT uuidv7(),
    -- Nomor PO/TAHUN/BULAN/NOMOR diberikan saat diajukan (termasuk maklon & reprocess).
    number                      text,
    order_type                  text          NOT NULL,
    supplier_id                 uuid          NOT NULL,
    -- Snapshot kode & nama pemasok agar PO lama tetap terbaca bila pemasok dihapus.
    supplier_code               text          NOT NULL,
    supplier_name               text          NOT NULL,
    division_id                 uuid          NOT NULL,
    business_date               date          NOT NULL,
    expected_date               date          NOT NULL,
    payment_term_days           integer       NOT NULL DEFAULT 0,
    note                        text          NOT NULL DEFAULT '',
    -- Maklon/reprocess: lokasi eksternal maklon tujuan pengiriman bahan dan lokasi asal bahan (RM/GM).
    toll_location_id            uuid,
    material_source_location_id uuid,
    subtotal_amount             numeric(18,2) NOT NULL DEFAULT 0,
    -- NON-PKP: tanpa PPN, total = subtotal (D-04).
    total_amount                numeric(18,2) NOT NULL DEFAULT 0,
    status                      text          NOT NULL DEFAULT 'draft',
    close_reason                text          NOT NULL DEFAULT '',
    cancel_reason               text          NOT NULL DEFAULT '',
    submitted_at                timestamptz,
    approved_at                 timestamptz,
    -- Notifikasi ETA H-1 ke Logistik Leader sudah dikirim (job procurement.notify_eta idempotent).
    eta_notified_at             timestamptz,
    created_at                  timestamptz   NOT NULL DEFAULT now(),
    created_by                  uuid          NOT NULL,
    updated_at                  timestamptz   NOT NULL DEFAULT now(),
    updated_by                  uuid          NOT NULL,
    version                     integer       NOT NULL DEFAULT 1,
    deleted_at                  timestamptz,
    deleted_by                  uuid,
    CONSTRAINT pk_purchase_orders PRIMARY KEY (id),
    CONSTRAINT uq_purchase_orders_number UNIQUE (number),
    CONSTRAINT ck_purchase_orders_order_type CHECK (order_type IN (
        'goods', 'packaging', 'toll_manufacturing', 'reprocess', 'other')),
    CONSTRAINT ck_purchase_orders_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'partially_received', 'received', 'completed',
        'closed', 'cancelled')),
    CONSTRAINT ck_purchase_orders_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_purchase_orders_expected_date CHECK (expected_date >= business_date),
    CONSTRAINT ck_purchase_orders_payment_term CHECK (payment_term_days BETWEEN 0 AND 365),
    CONSTRAINT ck_purchase_orders_amounts CHECK (subtotal_amount >= 0 AND total_amount = subtotal_amount),
    CONSTRAINT ck_purchase_orders_close_reason CHECK (status <> 'closed' OR btrim(close_reason) <> ''),
    CONSTRAINT ck_purchase_orders_toll_location CHECK (
        order_type NOT IN ('toll_manufacturing', 'reprocess') OR toll_location_id IS NOT NULL),
    CONSTRAINT ck_purchase_orders_reprocess_source CHECK (
        order_type <> 'reprocess' OR material_source_location_id IS NOT NULL)
);
CREATE INDEX idx_purchase_orders_status ON purchase_orders (status, business_date DESC);
CREATE INDEX idx_purchase_orders_supplier_id ON purchase_orders (supplier_id);
CREATE INDEX idx_purchase_orders_division_id ON purchase_orders (division_id);
CREATE INDEX idx_purchase_orders_expected_date ON purchase_orders (expected_date)
    WHERE status IN ('approved', 'partially_received') AND eta_notified_at IS NULL;

CREATE TABLE purchase_order_lines (
    id                       uuid          NOT NULL DEFAULT uuidv7(),
    purchase_order_id        uuid          NOT NULL,
    line_no                  integer       NOT NULL,
    -- Barang/packaging yang dibeli, atau FG hasil maklon/reprocess.
    product_id               uuid,
    description              text          NOT NULL DEFAULT '',
    ordered_qty              numeric(18,3) NOT NULL,
    -- Maklon/reprocess: harga jasa per unit FG.
    unit_price_amount        numeric(18,2) NOT NULL,
    line_amount              numeric(18,2) NOT NULL,
    received_qty             numeric(18,3) NOT NULL DEFAULT 0,
    billed_qty               numeric(18,3) NOT NULL DEFAULT 0,
    purchase_request_line_id uuid,
    note                     text          NOT NULL DEFAULT '',
    CONSTRAINT pk_purchase_order_lines PRIMARY KEY (id),
    CONSTRAINT fk_purchase_order_lines_purchase_order_id FOREIGN KEY (purchase_order_id)
        REFERENCES purchase_orders (id) ON DELETE CASCADE,
    CONSTRAINT fk_purchase_order_lines_purchase_request_line_id FOREIGN KEY (purchase_request_line_id)
        REFERENCES purchase_request_lines (id),
    CONSTRAINT uq_purchase_order_lines_line_no UNIQUE (purchase_order_id, line_no),
    CONSTRAINT ck_purchase_order_lines_item CHECK (product_id IS NOT NULL OR btrim(description) <> ''),
    CONSTRAINT ck_purchase_order_lines_qty CHECK (ordered_qty > 0),
    CONSTRAINT ck_purchase_order_lines_price CHECK (unit_price_amount >= 0 AND line_amount >= 0),
    -- Toleransi qty penerimaan 0% (BRD 5.2).
    CONSTRAINT ck_purchase_order_lines_received CHECK (received_qty >= 0 AND received_qty <= ordered_qty),
    CONSTRAINT ck_purchase_order_lines_billed CHECK (billed_qty >= 0 AND billed_qty <= received_qty)
);
CREATE INDEX idx_purchase_order_lines_product_id ON purchase_order_lines (product_id);
CREATE INDEX idx_purchase_order_lines_purchase_request_line_id ON purchase_order_lines (purchase_request_line_id);

-- Packaging (PO maklon) atau bahan GM (PO reprocess) yang direncanakan dipakai dan dikirim ke lokasi maklon.
CREATE TABLE purchase_order_materials (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    purchase_order_id   uuid          NOT NULL,
    line_no             integer       NOT NULL,
    material_product_id uuid          NOT NULL,
    planned_qty         numeric(18,3) NOT NULL,
    note                text          NOT NULL DEFAULT '',
    CONSTRAINT pk_purchase_order_materials PRIMARY KEY (id),
    CONSTRAINT fk_purchase_order_materials_purchase_order_id FOREIGN KEY (purchase_order_id)
        REFERENCES purchase_orders (id) ON DELETE CASCADE,
    CONSTRAINT uq_purchase_order_materials_line_no UNIQUE (purchase_order_id, line_no),
    CONSTRAINT uq_purchase_order_materials_product UNIQUE (purchase_order_id, material_product_id),
    CONSTRAINT ck_purchase_order_materials_qty CHECK (planned_qty > 0)
);
CREATE INDEX idx_purchase_order_materials_material_product_id ON purchase_order_materials (material_product_id);

-- Uang muka pembelian per PO (PRC-09, A-19). Nomor BYR diberikan saat diposting; applied_amount bertambah saat
-- tagihan pemasok memotong uang muka (S-208).
CREATE TABLE purchase_advances (
    id                        uuid          NOT NULL DEFAULT uuidv7(),
    number                    text,
    purchase_order_id         uuid          NOT NULL,
    supplier_id               uuid          NOT NULL,
    division_id               uuid          NOT NULL,
    amount                    numeric(18,2) NOT NULL,
    payment_date              date          NOT NULL,
    bank_account_code         text          NOT NULL,
    note                      text          NOT NULL DEFAULT '',
    applied_amount            numeric(18,2) NOT NULL DEFAULT 0,
    status                    text          NOT NULL DEFAULT 'draft',
    journal_entry_id          uuid,
    reversal_journal_entry_id uuid,
    cancel_reason             text          NOT NULL DEFAULT '',
    posted_at                 timestamptz,
    created_at                timestamptz   NOT NULL DEFAULT now(),
    created_by                uuid          NOT NULL,
    updated_at                timestamptz   NOT NULL DEFAULT now(),
    updated_by                uuid          NOT NULL,
    version                   integer       NOT NULL DEFAULT 1,
    deleted_at                timestamptz,
    deleted_by                uuid,
    CONSTRAINT pk_purchase_advances PRIMARY KEY (id),
    CONSTRAINT uq_purchase_advances_number UNIQUE (number),
    CONSTRAINT fk_purchase_advances_purchase_order_id FOREIGN KEY (purchase_order_id) REFERENCES purchase_orders (id),
    CONSTRAINT ck_purchase_advances_status CHECK (status IN ('draft', 'posted', 'cancelled')),
    CONSTRAINT ck_purchase_advances_amount CHECK (amount > 0),
    CONSTRAINT ck_purchase_advances_applied CHECK (applied_amount >= 0 AND applied_amount <= amount),
    CONSTRAINT ck_purchase_advances_posted CHECK (
        status <> 'posted' OR (number IS NOT NULL AND journal_entry_id IS NOT NULL)),
    CONSTRAINT ck_purchase_advances_bank_account CHECK (bank_account_code ~ '^11[0-9]{2}$')
);
CREATE INDEX idx_purchase_advances_purchase_order_id ON purchase_advances (purchase_order_id);
CREATE INDEX idx_purchase_advances_status ON purchase_advances (status, payment_date DESC);

-- +goose Down
DROP TABLE purchase_advances;
DROP TABLE purchase_order_materials;
DROP TABLE purchase_order_lines;
DROP TABLE purchase_orders;
DROP TABLE purchase_request_lines;
DROP TABLE purchase_requests;
