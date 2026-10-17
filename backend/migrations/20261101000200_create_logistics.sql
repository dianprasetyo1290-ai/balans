-- +goose Up
-- Logistik R1.3 (LOG-01 – LOG-04; Desain Data Model 6.x logistics, Desain Status Dokumen 5.4): master kurir, Delivery
-- Order dari Sales Order/konsinyasi, pengiriman (resi manual, AWB marketplace, kurir internal Merchandiser), dan
-- riwayat status pengiriman. Produk, batch, pihak, lokasi, user, dan file tanpa FK (milik modul lain).

CREATE TABLE couriers (
    id          uuid        NOT NULL DEFAULT uuidv7(),
    code        text        NOT NULL,
    name        text        NOT NULL,
    -- true = kurir internal (Merchandiser), wajib memilih user pengantar.
    is_internal boolean     NOT NULL DEFAULT false,
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid        NOT NULL,
    version     integer     NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    deleted_by  uuid,
    CONSTRAINT pk_couriers PRIMARY KEY (id),
    CONSTRAINT uq_couriers_code UNIQUE (code),
    CONSTRAINT ck_couriers_code CHECK (code ~ '^[a-z0-9][a-z0-9_-]*$' AND length(code) <= 30),
    CONSTRAINT ck_couriers_name CHECK (length(name) BETWEEN 1 AND 100)
);

INSERT INTO couriers (code, name, is_internal, created_by, updated_by) VALUES
    ('jne', 'JNE', false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('jnt', 'J&T Express', false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('sicepat', 'SiCepat', false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('gosend', 'GoSend', false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('grab', 'GrabExpress', false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('own', 'Kurir Internal (Merchandiser)', true, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

CREATE TABLE delivery_orders (
    id                    uuid        NOT NULL DEFAULT uuidv7(),
    -- Nomor DO diberikan saat release.
    number                text,
    -- sales, marketplace, consignment, vending_buffer, transfer.
    delivery_type         text        NOT NULL,
    -- Dokumen asal: sales_order atau dokumen modul lain yang memakai API DO (mis. konsinyasi).
    source_type           text        NOT NULL,
    source_id             uuid        NOT NULL,
    source_label          text        NOT NULL DEFAULT '',
    sales_order_id        uuid,
    partner_id            uuid,
    customer_id           uuid,
    recipient_name        text        NOT NULL DEFAULT '',
    recipient_phone       text        NOT NULL DEFAULT '',
    ship_to_address       text        NOT NULL DEFAULT '',
    location_id           uuid        NOT NULL,
    business_date         date        NOT NULL,
    status                text        NOT NULL DEFAULT 'draft',
    note                  text        NOT NULL DEFAULT '',
    outbound_id           uuid,
    outbound_completed_at timestamptz,
    released_by           uuid,
    released_by_name      text        NOT NULL DEFAULT '',
    released_at           timestamptz,
    shipped_at            timestamptz,
    delivered_at          timestamptz,
    cancelled_at          timestamptz,
    cancel_reason         text        NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            uuid        NOT NULL,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    updated_by            uuid        NOT NULL,
    version               integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_delivery_orders PRIMARY KEY (id),
    CONSTRAINT uq_delivery_orders_number UNIQUE (number),
    CONSTRAINT ck_delivery_orders_type CHECK (delivery_type IN ('sales', 'marketplace', 'consignment', 'vending_buffer', 'transfer')),
    CONSTRAINT ck_delivery_orders_source_type CHECK (source_type ~ '^[a-z][a-z0-9_]*$' AND length(source_type) <= 60),
    CONSTRAINT ck_delivery_orders_status CHECK (status IN ('draft', 'in_progress', 'in_transit', 'received', 'cancelled')),
    CONSTRAINT ck_delivery_orders_text CHECK (
        length(source_label) <= 120 AND length(recipient_name) <= 200 AND length(recipient_phone) <= 40
        AND length(ship_to_address) <= 1000 AND length(note) <= 500 AND length(cancel_reason) <= 500)
);
-- Satu DO terbuka per dokumen asal; DO berikutnya untuk sisa barang setelah yang lama dikirim/dibatalkan.
CREATE UNIQUE INDEX uq_delivery_orders_open_source ON delivery_orders (source_type, source_id)
    WHERE status IN ('draft', 'in_progress');
CREATE INDEX idx_delivery_orders_source ON delivery_orders (source_type, source_id);
CREATE INDEX idx_delivery_orders_status ON delivery_orders (status, created_at DESC);

CREATE TABLE delivery_order_lines (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    delivery_order_id   uuid          NOT NULL,
    line_no             integer       NOT NULL,
    -- Baris dokumen asal yang memegang reservasi stok (alokasi Sales Order).
    source_line_id      uuid,
    sales_order_line_id uuid,
    product_id          uuid          NOT NULL,
    qty                 numeric(18,3) NOT NULL,
    shipped_qty         numeric(18,3) NOT NULL DEFAULT 0,
    -- Rencana batch saat DO dibuat [{batchId, qty}]; batch final dari Outbound.
    batches             jsonb         NOT NULL DEFAULT '[]',
    CONSTRAINT pk_delivery_order_lines PRIMARY KEY (id),
    CONSTRAINT uq_delivery_order_lines_line_no UNIQUE (delivery_order_id, line_no),
    CONSTRAINT fk_delivery_order_lines_delivery_order FOREIGN KEY (delivery_order_id) REFERENCES delivery_orders (id),
    CONSTRAINT ck_delivery_order_lines_qty CHECK (qty > 0 AND shipped_qty >= 0 AND shipped_qty <= qty)
);

CREATE TABLE shipments (
    id                       uuid          NOT NULL DEFAULT uuidv7(),
    delivery_order_id        uuid          NOT NULL,
    courier_id               uuid          NOT NULL,
    internal_courier_user_id uuid,
    internal_courier_name    text          NOT NULL DEFAULT '',
    service_code             text          NOT NULL DEFAULT '',
    tracking_number          text          NOT NULL DEFAULT '',
    marketplace_awb          text          NOT NULL DEFAULT '',
    shipping_cost_amount     numeric(18,2) NOT NULL DEFAULT 0,
    package_count            integer       NOT NULL DEFAULT 0,
    -- picked_up (diserahkan ke kurir) → in_transit → delivered; returned = kembali ke gudang.
    status                   text          NOT NULL DEFAULT 'picked_up',
    shipped_at               timestamptz   NOT NULL,
    delivered_at             timestamptz,
    received_by_name         text          NOT NULL DEFAULT '',
    delivery_note            text          NOT NULL DEFAULT '',
    proof_file_ids           uuid[]        NOT NULL DEFAULT '{}',
    confirmed_by             uuid,
    confirmed_by_name        text          NOT NULL DEFAULT '',
    created_at               timestamptz   NOT NULL DEFAULT now(),
    created_by               uuid          NOT NULL,
    updated_at               timestamptz   NOT NULL DEFAULT now(),
    updated_by               uuid          NOT NULL,
    version                  integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_shipments PRIMARY KEY (id),
    CONSTRAINT uq_shipments_delivery_order UNIQUE (delivery_order_id),
    CONSTRAINT fk_shipments_delivery_order FOREIGN KEY (delivery_order_id) REFERENCES delivery_orders (id),
    CONSTRAINT fk_shipments_courier FOREIGN KEY (courier_id) REFERENCES couriers (id),
    CONSTRAINT ck_shipments_status CHECK (status IN ('picked_up', 'in_transit', 'delivered', 'returned')),
    CONSTRAINT ck_shipments_cost CHECK (shipping_cost_amount >= 0 AND package_count >= 0),
    CONSTRAINT ck_shipments_text CHECK (
        length(service_code) <= 40 AND length(tracking_number) <= 60 AND length(marketplace_awb) <= 60
        AND length(received_by_name) <= 200 AND length(delivery_note) <= 500)
);
CREATE INDEX idx_shipments_courier_id ON shipments (courier_id);
CREATE INDEX idx_shipments_tracking ON shipments (tracking_number) WHERE tracking_number <> '';
CREATE INDEX idx_shipments_awb ON shipments (marketplace_awb) WHERE marketplace_awb <> '';
CREATE INDEX idx_shipments_internal_courier ON shipments (internal_courier_user_id) WHERE internal_courier_user_id IS NOT NULL;

CREATE TABLE shipment_events (
    id              uuid        NOT NULL DEFAULT uuidv7(),
    shipment_id     uuid        NOT NULL,
    status          text        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    -- manual (dicatat user) atau webhook (kurir, mulai R2.2).
    source          text        NOT NULL DEFAULT 'manual',
    note            text        NOT NULL DEFAULT '',
    created_by      uuid        NOT NULL,
    created_by_name text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_shipment_events PRIMARY KEY (id),
    CONSTRAINT fk_shipment_events_shipment FOREIGN KEY (shipment_id) REFERENCES shipments (id),
    CONSTRAINT ck_shipment_events_status CHECK (status IN ('picked_up', 'in_transit', 'delivered', 'returned')),
    CONSTRAINT ck_shipment_events_source CHECK (source IN ('manual', 'webhook')),
    CONSTRAINT ck_shipment_events_note CHECK (length(note) <= 500)
);
CREATE INDEX idx_shipment_events_shipment ON shipment_events (shipment_id, occurred_at);

-- +goose Down
DROP TABLE shipment_events;
DROP TABLE shipments;
DROP TABLE delivery_order_lines;
DROP TABLE delivery_orders;
DROP TABLE couriers;
