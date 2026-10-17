-- +goose Up
-- Ledger stok modul inventory (INV-01, INV-03, INV-09, WMS-01; Desain Data Model 6.2; Technical Guideline 5.7, 7.2).
-- product_id dan location_id tanpa FK karena tabel produk dan lokasi milik modul catalog dan warehouse
-- (Technical Guideline 2.3); keberadaannya diperiksa service lewat API publik modul pemilik.

CREATE TABLE batches (
    id              uuid        NOT NULL DEFAULT uuidv7(),
    product_id      uuid        NOT NULL,
    batch_number    text        NOT NULL,
    production_date date,
    expiry_date     date,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid        NOT NULL,
    version         integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_batches PRIMARY KEY (id),
    CONSTRAINT uq_batches_product_batch_number UNIQUE (product_id, batch_number),
    CONSTRAINT ck_batches_batch_number CHECK (length(btrim(batch_number)) BETWEEN 1 AND 40 AND batch_number = btrim(batch_number)),
    CONSTRAINT ck_batches_dates CHECK (production_date IS NULL OR expiry_date IS NULL OR expiry_date >= production_date)
);
CREATE INDEX idx_batches_expiry_date ON batches (expiry_date);

-- Satu posting = satu aksi dokumen sumber terhadap stok (mis. post Penerimaan Barang, lulus QC, reservasi SO).
-- Unik per (source_type, source_id, action) sehingga aksi yang sama tidak bisa memutasi stok dua kali.
CREATE TABLE stock_postings (
    id            uuid        NOT NULL DEFAULT uuidv7(),
    source_type   text        NOT NULL,
    source_id     uuid        NOT NULL,
    action        text        NOT NULL,
    source_label  text        NOT NULL DEFAULT '',
    business_date date        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid        NOT NULL,
    CONSTRAINT pk_stock_postings PRIMARY KEY (id),
    CONSTRAINT uq_stock_postings_source UNIQUE (source_type, source_id, action),
    CONSTRAINT ck_stock_postings_source_type CHECK (source_type ~ '^[a-z][a-z0-9_]*$' AND length(source_type) <= 60),
    CONSTRAINT ck_stock_postings_action CHECK (length(btrim(action)) BETWEEN 1 AND 60),
    CONSTRAINT ck_stock_postings_source_label CHECK (length(source_label) <= 120)
);

CREATE TABLE stock_reservations (
    id             uuid          NOT NULL DEFAULT uuidv7(),
    source_type    text          NOT NULL,
    source_id      uuid          NOT NULL,
    source_line_id uuid,
    product_id     uuid          NOT NULL,
    location_id    uuid          NOT NULL,
    batch_id       uuid,
    reserved_qty   numeric(18,3) NOT NULL,
    consumed_qty   numeric(18,3) NOT NULL DEFAULT 0,
    released_qty   numeric(18,3) NOT NULL DEFAULT 0,
    expires_at     timestamptz,
    created_at     timestamptz   NOT NULL DEFAULT now(),
    created_by     uuid          NOT NULL,
    updated_at     timestamptz   NOT NULL DEFAULT now(),
    updated_by     uuid          NOT NULL,
    version        integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_stock_reservations PRIMARY KEY (id),
    CONSTRAINT fk_stock_reservations_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_stock_reservations_qty CHECK (
        reserved_qty > 0 AND consumed_qty >= 0 AND released_qty >= 0 AND consumed_qty + released_qty <= reserved_qty)
);
CREATE INDEX idx_stock_reservations_batch_id ON stock_reservations (batch_id);
CREATE INDEX idx_stock_reservations_source ON stock_reservations (source_type, source_id)
    WHERE consumed_qty + released_qty < reserved_qty;
CREATE INDEX idx_stock_reservations_expires_at ON stock_reservations (expires_at)
    WHERE expires_at IS NOT NULL AND consumed_qty + released_qty < reserved_qty;

-- Ledger append-only (Technical Guideline 5.7.1). qty bertanda: + masuk, − keluar. cost_amount bertanda sama
-- dengan qty; untuk barang keluar berisi HPP FIFO dari cost layer yang dikonsumsi.
CREATE TABLE stock_movements (
    id                    uuid          NOT NULL DEFAULT uuidv7(),
    posting_id            uuid          NOT NULL,
    line_no               integer       NOT NULL,
    source_type           text          NOT NULL,
    source_id             uuid          NOT NULL,
    source_line_id        uuid,
    business_date         date          NOT NULL,
    movement_type         text          NOT NULL,
    product_id            uuid          NOT NULL,
    location_id           uuid          NOT NULL,
    batch_id              uuid,
    stock_status          text          NOT NULL,
    qty                   numeric(18,3) NOT NULL,
    unit_cost             numeric(18,4) NOT NULL,
    cost_amount           numeric(18,2) NOT NULL,
    reservation_id        uuid,
    batch_override_reason text,
    note                  text          NOT NULL DEFAULT '',
    created_at            timestamptz   NOT NULL DEFAULT now(),
    created_by            uuid          NOT NULL,
    CONSTRAINT pk_stock_movements PRIMARY KEY (id),
    CONSTRAINT uq_stock_movements_posting_line UNIQUE (posting_id, line_no),
    CONSTRAINT fk_stock_movements_posting_id FOREIGN KEY (posting_id) REFERENCES stock_postings (id),
    CONSTRAINT fk_stock_movements_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT fk_stock_movements_reservation_id FOREIGN KEY (reservation_id) REFERENCES stock_reservations (id),
    CONSTRAINT ck_stock_movements_status CHECK (stock_status IN ('available', 'quarantine', 'in_transit', 'reserved')),
    CONSTRAINT ck_stock_movements_type CHECK (movement_type IN ('receive', 'issue', 'move_out', 'move_in')),
    CONSTRAINT ck_stock_movements_sign CHECK (
        CASE WHEN movement_type IN ('receive', 'move_in') THEN qty > 0 ELSE qty < 0 END),
    CONSTRAINT ck_stock_movements_cost CHECK (unit_cost >= 0 AND (cost_amount = 0 OR sign(cost_amount) = sign(qty))),
    CONSTRAINT ck_stock_movements_reason CHECK (batch_override_reason IS NULL OR length(btrim(batch_override_reason)) BETWEEN 1 AND 500),
    CONSTRAINT ck_stock_movements_note CHECK (length(note) <= 500)
);
CREATE INDEX idx_stock_movements_posting_id ON stock_movements (posting_id);
CREATE INDEX idx_stock_movements_batch_id ON stock_movements (batch_id);
CREATE INDEX idx_stock_movements_reservation_id ON stock_movements (reservation_id);
CREATE INDEX idx_stock_movements_card ON stock_movements (product_id, business_date, created_at, id);
CREATE INDEX idx_stock_movements_location_id ON stock_movements (location_id, product_id);
CREATE INDEX idx_stock_movements_source ON stock_movements (source_type, source_id);

-- Saldo per product × location × batch × status (read model posisi stok). batch_id null = produk tanpa batch,
-- sehingga kunci memakai unique NULLS NOT DISTINCT dan id surrogate. Baris dikunci FOR UPDATE berurutan
-- (product_id, location_id, batch_id, stock_status) di transaksi movement (Technical Guideline 7.2).
CREATE TABLE stock_balances (
    id           uuid          NOT NULL DEFAULT uuidv7(),
    product_id   uuid          NOT NULL,
    location_id  uuid          NOT NULL,
    batch_id     uuid,
    stock_status text          NOT NULL,
    qty          numeric(18,3) NOT NULL DEFAULT 0,
    updated_at   timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT pk_stock_balances PRIMARY KEY (id),
    CONSTRAINT uq_stock_balances_key UNIQUE NULLS NOT DISTINCT (product_id, location_id, batch_id, stock_status),
    CONSTRAINT fk_stock_balances_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT ck_stock_balances_status CHECK (stock_status IN ('available', 'quarantine', 'in_transit', 'reserved')),
    CONSTRAINT ck_stock_balances_qty_non_negative CHECK (qty >= 0)
);
CREATE INDEX idx_stock_balances_batch_id ON stock_balances (batch_id);
CREATE INDEX idx_stock_balances_location_id ON stock_balances (location_id) WHERE qty > 0;

-- Lapisan biaya FIFO per penerimaan di dalam batch (INV-03). remaining_qty = sisa stok batch (semua lokasi &
-- status) yang belum keluar; dikonsumsi FIFO saat barang keluar.
CREATE TABLE cost_layers (
    id            uuid          NOT NULL DEFAULT uuidv7(),
    product_id    uuid          NOT NULL,
    batch_id      uuid,
    source_type   text          NOT NULL,
    source_id     uuid          NOT NULL,
    source_label  text          NOT NULL DEFAULT '',
    movement_id   uuid          NOT NULL,
    received_qty  numeric(18,3) NOT NULL,
    remaining_qty numeric(18,3) NOT NULL,
    unit_cost     numeric(18,4) NOT NULL,
    business_date date          NOT NULL,
    created_at    timestamptz   NOT NULL DEFAULT now(),
    updated_at    timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT pk_cost_layers PRIMARY KEY (id),
    CONSTRAINT uq_cost_layers_movement_id UNIQUE (movement_id),
    CONSTRAINT fk_cost_layers_batch_id FOREIGN KEY (batch_id) REFERENCES batches (id),
    CONSTRAINT fk_cost_layers_movement_id FOREIGN KEY (movement_id) REFERENCES stock_movements (id),
    CONSTRAINT ck_cost_layers_qty CHECK (received_qty > 0 AND remaining_qty >= 0 AND remaining_qty <= received_qty),
    CONSTRAINT ck_cost_layers_unit_cost CHECK (unit_cost >= 0)
);
CREATE INDEX idx_cost_layers_batch_id ON cost_layers (batch_id);
CREATE INDEX idx_cost_layers_fifo ON cost_layers (product_id, batch_id, business_date, created_at, id) WHERE remaining_qty > 0;

-- Jejak konsumsi HPP per movement keluar (RPT-04), append-only.
CREATE TABLE cost_layer_consumptions (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    cost_layer_id     uuid          NOT NULL,
    stock_movement_id uuid          NOT NULL,
    qty               numeric(18,3) NOT NULL,
    unit_cost         numeric(18,4) NOT NULL,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    CONSTRAINT pk_cost_layer_consumptions PRIMARY KEY (id),
    CONSTRAINT fk_cost_layer_consumptions_cost_layer_id FOREIGN KEY (cost_layer_id) REFERENCES cost_layers (id),
    CONSTRAINT fk_cost_layer_consumptions_stock_movement_id FOREIGN KEY (stock_movement_id) REFERENCES stock_movements (id),
    CONSTRAINT ck_cost_layer_consumptions_qty CHECK (qty > 0 AND unit_cost >= 0)
);
CREATE INDEX idx_cost_layer_consumptions_cost_layer_id ON cost_layer_consumptions (cost_layer_id);
CREATE INDEX idx_cost_layer_consumptions_stock_movement_id ON cost_layer_consumptions (stock_movement_id);

-- Ledger append-only: UPDATE/DELETE ditolak di level DB (Technical Guideline 5.7.1).
-- +goose StatementBegin
CREATE FUNCTION inventory_reject_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_stock_postings_append_only
    BEFORE UPDATE OR DELETE ON stock_postings
    FOR EACH ROW EXECUTE FUNCTION inventory_reject_mutation();
CREATE TRIGGER trg_stock_movements_append_only
    BEFORE UPDATE OR DELETE ON stock_movements
    FOR EACH ROW EXECUTE FUNCTION inventory_reject_mutation();
CREATE TRIGGER trg_cost_layer_consumptions_append_only
    BEFORE UPDATE OR DELETE ON cost_layer_consumptions
    FOR EACH ROW EXECUTE FUNCTION inventory_reject_mutation();

-- +goose Down
DROP TABLE cost_layer_consumptions;
DROP TABLE cost_layers;
DROP TABLE stock_balances;
DROP TABLE stock_movements;
DROP TABLE stock_reservations;
DROP TABLE stock_postings;
DROP TABLE batches;
DROP FUNCTION inventory_reject_mutation();
