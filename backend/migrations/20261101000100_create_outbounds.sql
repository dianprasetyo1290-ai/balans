-- +goose Up
-- Outbound / QC (WMS-12, D-23, D-24; Desain Data Model 6.1, Desain Status Dokumen 3.4): picking & packing oleh
-- Staf Gudang (prepared_by), QC oleh Fulfillment Leader/Merchandiser yang bukan penyiap, koli untuk label koli.
-- Stok barang yang disiapkan sudah reserved oleh dokumen asal (mis. Sales Order); Outbound tidak memutasi stok
-- sendiri kecuali lewat handler dokumen asal saat QC gagal. Produk, batch, dan user tanpa FK (modul lain).

CREATE TABLE outbounds (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    number           text        NOT NULL,
    -- Dokumen asal: delivery_order, stock_transfer, internal_usage, purchase_return, dll.
    source_type      text        NOT NULL,
    source_id        uuid        NOT NULL,
    source_label     text        NOT NULL DEFAULT '',
    warehouse_id     uuid        NOT NULL,
    location_id      uuid        NOT NULL,
    business_date    date        NOT NULL,
    -- Penerima untuk label koli.
    recipient_name   text        NOT NULL DEFAULT '',
    recipient_phone  text        NOT NULL DEFAULT '',
    recipient_address text       NOT NULL DEFAULT '',
    status           text        NOT NULL DEFAULT 'in_progress',
    note             text        NOT NULL DEFAULT '',
    prepared_by      uuid,
    prepared_by_name text        NOT NULL DEFAULT '',
    packed_at        timestamptz,
    checked_by       uuid,
    checked_by_name  text        NOT NULL DEFAULT '',
    completed_at     timestamptz,
    cancelled_at     timestamptz,
    cancel_reason    text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_outbounds PRIMARY KEY (id),
    CONSTRAINT uq_outbounds_number UNIQUE (number),
    CONSTRAINT uq_outbounds_source UNIQUE (source_type, source_id),
    CONSTRAINT fk_outbounds_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses (id),
    CONSTRAINT fk_outbounds_location FOREIGN KEY (location_id) REFERENCES locations (id),
    CONSTRAINT ck_outbounds_source_type CHECK (source_type ~ '^[a-z][a-z0-9_]*$' AND length(source_type) <= 60),
    CONSTRAINT ck_outbounds_source_label CHECK (length(source_label) <= 120),
    CONSTRAINT ck_outbounds_recipient CHECK (
        length(recipient_name) <= 200 AND length(recipient_phone) <= 40 AND length(recipient_address) <= 1000),
    CONSTRAINT ck_outbounds_status CHECK (status IN ('in_progress', 'completed', 'cancelled')),
    CONSTRAINT ck_outbounds_note CHECK (length(note) <= 500 AND length(cancel_reason) <= 500)
);
CREATE INDEX idx_outbounds_warehouse_id ON outbounds (warehouse_id);
CREATE INDEX idx_outbounds_location_id ON outbounds (location_id);
CREATE INDEX idx_outbounds_status ON outbounds (status, created_at DESC);

CREATE TABLE outbound_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    outbound_id            uuid          NOT NULL,
    line_no                integer       NOT NULL,
    -- Baris dokumen asal (mis. alokasi Sales Order) yang memegang reservasi stok.
    source_line_id         uuid,
    product_id             uuid          NOT NULL,
    batch_id               uuid,
    allocated_qty          numeric(18,3) NOT NULL,
    picked_qty             numeric(18,3) NOT NULL DEFAULT 0,
    passed_qty             numeric(18,3) NOT NULL DEFAULT 0,
    rejected_qty           numeric(18,3) NOT NULL DEFAULT 0,
    -- pending (menunggu picking) → picked → passed / qc_rejected; held = menunggu stok pengganti;
    -- dropped = dikirim sebagian atas keputusan user (qty tidak ikut dikirim).
    status                 text          NOT NULL DEFAULT 'pending',
    replacement_of_line_id uuid,
    reject_reason          text          NOT NULL DEFAULT '',
    -- Nomor retur pembelian otomatis (RB) untuk barang ditolak yang berasal dari maklon/pemasok (S-311).
    return_label           text          NOT NULL DEFAULT '',
    checked_by             uuid,
    checked_by_name        text          NOT NULL DEFAULT '',
    checked_at             timestamptz,
    CONSTRAINT pk_outbound_lines PRIMARY KEY (id),
    CONSTRAINT uq_outbound_lines_line_no UNIQUE (outbound_id, line_no),
    CONSTRAINT fk_outbound_lines_outbound FOREIGN KEY (outbound_id) REFERENCES outbounds (id),
    CONSTRAINT fk_outbound_lines_replacement FOREIGN KEY (replacement_of_line_id) REFERENCES outbound_lines (id),
    CONSTRAINT ck_outbound_lines_qty CHECK (
        allocated_qty > 0 AND picked_qty >= 0 AND passed_qty >= 0 AND rejected_qty >= 0
        AND passed_qty + rejected_qty <= allocated_qty),
    CONSTRAINT ck_outbound_lines_status CHECK (status IN ('pending', 'picked', 'passed', 'qc_rejected', 'held', 'dropped')),
    CONSTRAINT ck_outbound_lines_reason CHECK (length(reject_reason) <= 500 AND length(return_label) <= 60)
);
CREATE INDEX idx_outbound_lines_replacement ON outbound_lines (replacement_of_line_id);

-- Koli dicatat saat packing (S-316); dasar label koli 100 × 150 mm.
CREATE TABLE outbound_packages (
    id             uuid    NOT NULL DEFAULT uuidv7(),
    outbound_id    uuid    NOT NULL,
    package_number integer NOT NULL,
    package_count  integer NOT NULL,
    weight_grams   integer,
    -- [{lineId, productId, batchId, qty}]
    contents       jsonb   NOT NULL DEFAULT '[]',
    CONSTRAINT pk_outbound_packages PRIMARY KEY (id),
    CONSTRAINT uq_outbound_packages_number UNIQUE (outbound_id, package_number),
    CONSTRAINT fk_outbound_packages_outbound FOREIGN KEY (outbound_id) REFERENCES outbounds (id),
    CONSTRAINT ck_outbound_packages_number CHECK (package_number BETWEEN 1 AND package_count AND package_count <= 500),
    CONSTRAINT ck_outbound_packages_weight CHECK (weight_grams IS NULL OR weight_grams > 0)
);

-- +goose Down
DROP TABLE outbound_packages;
DROP TABLE outbound_lines;
DROP TABLE outbounds;
