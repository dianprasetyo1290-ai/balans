-- +goose Up
-- Dokumen gudang R1.2 (WMS-02, 03, 04, 08, 09, 10, 11; Desain Data Model 6.1, Desain Status Dokumen 3):
-- Inbound/QC, Transfer Stok (termasuk Permintaan Petty Stock), Stock Count beserta jadwalnya. Produk, batch,
-- user, dan file tanpa FK karena tabelnya milik modul lain (Technical Guideline 2.3).

CREATE TABLE inbounds (
    id            uuid        NOT NULL DEFAULT uuidv7(),
    number        text        NOT NULL,
    -- Dokumen asal: goods_receipt, sales_return, consignment_pullback, stock_transfer, internal_usage_return, dll.
    source_type   text        NOT NULL,
    source_id     uuid        NOT NULL,
    source_label  text        NOT NULL DEFAULT '',
    warehouse_id  uuid        NOT NULL,
    location_id   uuid        NOT NULL,
    business_date date        NOT NULL,
    -- Perlakuan qty ditolak QC: hold = tetap Karantina di Inbound (reject ke pemasok/maklon), returns = pindah ke
    -- Gudang Retur berstatus Karantina untuk disposisi.
    reject_action text        NOT NULL,
    status        text        NOT NULL DEFAULT 'in_progress',
    note          text        NOT NULL DEFAULT '',
    completed_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid        NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    updated_by    uuid        NOT NULL,
    version       integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_inbounds PRIMARY KEY (id),
    CONSTRAINT uq_inbounds_number UNIQUE (number),
    CONSTRAINT uq_inbounds_source UNIQUE (source_type, source_id),
    CONSTRAINT fk_inbounds_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses (id),
    CONSTRAINT fk_inbounds_location FOREIGN KEY (location_id) REFERENCES locations (id),
    CONSTRAINT ck_inbounds_source_type CHECK (source_type ~ '^[a-z][a-z0-9_]*$' AND length(source_type) <= 60),
    CONSTRAINT ck_inbounds_source_label CHECK (length(source_label) <= 120),
    CONSTRAINT ck_inbounds_reject_action CHECK (reject_action IN ('hold', 'returns')),
    CONSTRAINT ck_inbounds_status CHECK (status IN ('in_progress', 'completed')),
    CONSTRAINT ck_inbounds_note CHECK (length(note) <= 500)
);
CREATE INDEX idx_inbounds_warehouse_id ON inbounds (warehouse_id);
CREATE INDEX idx_inbounds_location_id ON inbounds (location_id);
CREATE INDEX idx_inbounds_status ON inbounds (status, created_at DESC);

CREATE TABLE inbound_lines (
    id                      uuid          NOT NULL DEFAULT uuidv7(),
    inbound_id              uuid          NOT NULL,
    line_no                 integer       NOT NULL,
    source_line_id          uuid,
    product_id              uuid          NOT NULL,
    batch_id                uuid,
    qty                     numeric(18,3) NOT NULL,
    passed_qty              numeric(18,3) NOT NULL DEFAULT 0,
    rejected_qty            numeric(18,3) NOT NULL DEFAULT 0,
    destination_location_id uuid          NOT NULL,
    -- quarantine (belum QC) → posted (ada qty lulus) atau qc_rejected (seluruhnya ditolak).
    status                  text          NOT NULL DEFAULT 'quarantine',
    reject_reason           text          NOT NULL DEFAULT '',
    reject_location_id      uuid,
    checked_by              uuid,
    checked_by_name         text          NOT NULL DEFAULT '',
    checked_at              timestamptz,
    CONSTRAINT pk_inbound_lines PRIMARY KEY (id),
    CONSTRAINT uq_inbound_lines_line_no UNIQUE (inbound_id, line_no),
    CONSTRAINT fk_inbound_lines_inbound FOREIGN KEY (inbound_id) REFERENCES inbounds (id),
    CONSTRAINT fk_inbound_lines_destination FOREIGN KEY (destination_location_id) REFERENCES locations (id),
    CONSTRAINT fk_inbound_lines_reject_location FOREIGN KEY (reject_location_id) REFERENCES locations (id),
    CONSTRAINT ck_inbound_lines_qty CHECK (
        qty > 0 AND passed_qty >= 0 AND rejected_qty >= 0 AND passed_qty + rejected_qty <= qty),
    CONSTRAINT ck_inbound_lines_status CHECK (status IN ('quarantine', 'posted', 'qc_rejected')),
    CONSTRAINT ck_inbound_lines_checked CHECK (
        (status = 'quarantine') = (checked_at IS NULL)
        AND (status = 'quarantine' OR passed_qty + rejected_qty = qty)),
    CONSTRAINT ck_inbound_lines_reject_reason CHECK (length(reject_reason) <= 500)
);
CREATE INDEX idx_inbound_lines_destination ON inbound_lines (destination_location_id);
CREATE INDEX idx_inbound_lines_reject_location ON inbound_lines (reject_location_id);

-- Hasil QC per baris dokumen (TG 5.7.2). document_type 'inbound' sekarang; 'outbound' dipakai QC Outbound.
CREATE TABLE quality_checks (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    document_type    text          NOT NULL,
    document_id      uuid          NOT NULL,
    document_line_id uuid          NOT NULL,
    result           text          NOT NULL,
    qty              numeric(18,3) NOT NULL,
    -- {batch, expiry, condition, seal}: true = sesuai.
    checks           jsonb         NOT NULL DEFAULT '{}',
    note             text          NOT NULL DEFAULT '',
    photo_file_ids   uuid[]        NOT NULL DEFAULT '{}',
    checked_by       uuid          NOT NULL,
    checked_by_name  text          NOT NULL DEFAULT '',
    checked_at       timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT pk_quality_checks PRIMARY KEY (id),
    CONSTRAINT ck_quality_checks_document_type CHECK (document_type IN ('inbound', 'outbound')),
    CONSTRAINT ck_quality_checks_result CHECK (result IN ('passed', 'rejected')),
    CONSTRAINT ck_quality_checks_qty CHECK (qty > 0),
    CONSTRAINT ck_quality_checks_note CHECK (length(note) <= 1100)
);
CREATE INDEX idx_quality_checks_document ON quality_checks (document_type, document_id);

CREATE TABLE stock_transfers (
    id                uuid        NOT NULL DEFAULT uuidv7(),
    -- Nomor TRF diberikan saat diajukan (permintaan) atau diposting/dikirim (transfer langsung).
    number            text,
    -- direct = transfer oleh Logistik; request = Permintaan Petty Stock oleh Fulfillment (WMS-03).
    transfer_type     text        NOT NULL,
    from_location_id  uuid        NOT NULL,
    to_location_id    uuid        NOT NULL,
    -- true = antar gudang fisik / ke-dari lokasi eksternal: stok Dalam Pengiriman sampai diterima (WMS-10).
    requires_shipping boolean     NOT NULL DEFAULT false,
    business_date     date        NOT NULL,
    status            text        NOT NULL DEFAULT 'draft',
    -- Dokumen modul lain yang membuat transfer lewat API (mis. DO konsinyasi), opsional.
    source_type       text        NOT NULL DEFAULT '',
    source_id         uuid,
    note              text        NOT NULL DEFAULT '',
    requested_by      uuid        NOT NULL,
    requested_by_name text        NOT NULL DEFAULT '',
    submitted_at      timestamptz,
    released_by       uuid,
    released_by_name  text        NOT NULL DEFAULT '',
    released_at       timestamptz,
    prepared_by       uuid,
    prepared_by_name  text        NOT NULL DEFAULT '',
    prepared_at       timestamptz,
    shipped_by        uuid,
    shipped_by_name   text        NOT NULL DEFAULT '',
    shipped_at        timestamptz,
    received_by       uuid,
    received_by_name  text        NOT NULL DEFAULT '',
    received_at       timestamptz,
    completed_at      timestamptz,
    cancelled_at      timestamptz,
    cancel_reason     text        NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid        NOT NULL,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    updated_by        uuid        NOT NULL,
    version           integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_stock_transfers PRIMARY KEY (id),
    CONSTRAINT uq_stock_transfers_number UNIQUE (number),
    CONSTRAINT fk_stock_transfers_from FOREIGN KEY (from_location_id) REFERENCES locations (id),
    CONSTRAINT fk_stock_transfers_to FOREIGN KEY (to_location_id) REFERENCES locations (id),
    CONSTRAINT ck_stock_transfers_type CHECK (transfer_type IN ('direct', 'request')),
    CONSTRAINT ck_stock_transfers_locations CHECK (from_location_id <> to_location_id),
    CONSTRAINT ck_stock_transfers_status CHECK (status IN (
        'draft', 'submitted', 'in_progress', 'in_transit', 'partially_received', 'received', 'completed', 'cancelled')),
    CONSTRAINT ck_stock_transfers_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_stock_transfers_source CHECK (source_type = '' OR (source_type ~ '^[a-z][a-z0-9_]*$' AND source_id IS NOT NULL)),
    CONSTRAINT ck_stock_transfers_note CHECK (length(note) <= 500 AND length(cancel_reason) <= 500)
);
CREATE INDEX idx_stock_transfers_from ON stock_transfers (from_location_id);
CREATE INDEX idx_stock_transfers_to ON stock_transfers (to_location_id);
CREATE INDEX idx_stock_transfers_status ON stock_transfers (transfer_type, status, created_at DESC);
CREATE UNIQUE INDEX uq_stock_transfers_source ON stock_transfers (source_type, source_id) WHERE source_id IS NOT NULL;

CREATE TABLE stock_transfer_lines (
    id           uuid          NOT NULL DEFAULT uuidv7(),
    transfer_id  uuid          NOT NULL,
    line_no      integer       NOT NULL,
    product_id   uuid          NOT NULL,
    batch_id     uuid,
    qty          numeric(18,3) NOT NULL,
    received_qty numeric(18,3) NOT NULL DEFAULT 0,
    -- Kekurangan saat transfer diselesaikan; diproses lewat Stock Count lokasi tujuan.
    shortage_qty numeric(18,3) NOT NULL DEFAULT 0,
    cost_amount  numeric(18,2) NOT NULL DEFAULT 0,
    batch_reason text          NOT NULL DEFAULT '',
    note         text          NOT NULL DEFAULT '',
    CONSTRAINT pk_stock_transfer_lines PRIMARY KEY (id),
    CONSTRAINT uq_stock_transfer_lines_line_no UNIQUE (transfer_id, line_no),
    CONSTRAINT fk_stock_transfer_lines_transfer FOREIGN KEY (transfer_id) REFERENCES stock_transfers (id),
    CONSTRAINT ck_stock_transfer_lines_qty CHECK (
        qty > 0 AND received_qty >= 0 AND shortage_qty >= 0 AND received_qty + shortage_qty <= qty),
    CONSTRAINT ck_stock_transfer_lines_text CHECK (length(batch_reason) <= 500 AND length(note) <= 500)
);

CREATE TABLE stock_count_schedules (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    name             text        NOT NULL,
    count_type       text        NOT NULL,
    -- weekly (day_of_week 1 = Senin … 7 = Minggu), monthly (day_of_month, 31 = akhir bulan), yearly (+ month).
    frequency        text        NOT NULL,
    day_of_week      integer,
    day_of_month     integer,
    month_of_year    integer,
    warehouse_id     uuid,
    -- Kosong = semua lokasi aktif sesuai jenis stock count (gudang / konsinyasi / vending).
    location_ids     uuid[]      NOT NULL DEFAULT '{}',
    assignee_user_id uuid,
    assignee_name    text        NOT NULL DEFAULT '',
    next_run_date    date        NOT NULL,
    last_run_date    date,
    is_active        boolean     NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_stock_count_schedules PRIMARY KEY (id),
    CONSTRAINT fk_stock_count_schedules_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses (id),
    CONSTRAINT ck_stock_count_schedules_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT ck_stock_count_schedules_type CHECK (count_type IN ('full', 'cycle', 'consignment', 'vending')),
    CONSTRAINT ck_stock_count_schedules_frequency CHECK (
        (frequency = 'weekly' AND day_of_week BETWEEN 1 AND 7)
        OR (frequency = 'monthly' AND day_of_month BETWEEN 1 AND 31)
        OR (frequency = 'yearly' AND day_of_month BETWEEN 1 AND 31 AND month_of_year BETWEEN 1 AND 12))
);
CREATE INDEX idx_stock_count_schedules_warehouse ON stock_count_schedules (warehouse_id);
CREATE INDEX idx_stock_count_schedules_due ON stock_count_schedules (next_run_date) WHERE is_active;

CREATE TABLE stock_counts (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    -- Nomor SC diberikan saat mulai hitung (snapshot).
    number            text,
    -- full = penuh (akhir tahun), cycle = per lokasi berkala, consignment/vending = lokasi mitra/mesin bulanan
    -- (hasilnya dipakai rekonsiliasi, tanpa penyesuaian sendiri).
    count_type        text          NOT NULL,
    location_id       uuid          NOT NULL,
    schedule_id       uuid,
    business_date     date          NOT NULL,
    status            text          NOT NULL DEFAULT 'draft',
    assigned_to       uuid,
    assigned_to_name  text          NOT NULL DEFAULT '',
    note              text          NOT NULL DEFAULT '',
    photo_file_ids    uuid[]        NOT NULL DEFAULT '{}',
    snapshot_at       timestamptz,
    started_by        uuid,
    started_by_name   text          NOT NULL DEFAULT '',
    submitted_by      uuid,
    submitted_by_name text          NOT NULL DEFAULT '',
    submitted_at      timestamptz,
    -- Jumlah nilai selisih absolut (nominal approval) dan selisih bersih (+ lebih, − kurang) dari HPP snapshot.
    variance_amount   numeric(18,2) NOT NULL DEFAULT 0,
    net_variance      numeric(18,2) NOT NULL DEFAULT 0,
    decided_at        timestamptz,
    decision_note     text          NOT NULL DEFAULT '',
    posted_at         timestamptz,
    journal_entry_id  uuid,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_stock_counts PRIMARY KEY (id),
    CONSTRAINT uq_stock_counts_number UNIQUE (number),
    CONSTRAINT fk_stock_counts_location FOREIGN KEY (location_id) REFERENCES locations (id),
    CONSTRAINT fk_stock_counts_schedule FOREIGN KEY (schedule_id) REFERENCES stock_count_schedules (id),
    CONSTRAINT ck_stock_counts_type CHECK (count_type IN ('full', 'cycle', 'consignment', 'vending')),
    CONSTRAINT ck_stock_counts_status CHECK (status IN (
        'draft', 'in_progress', 'pending_approval', 'rejected', 'posted', 'completed', 'cancelled')),
    CONSTRAINT ck_stock_counts_snapshot CHECK (status IN ('draft', 'cancelled') OR (snapshot_at IS NOT NULL AND number IS NOT NULL)),
    CONSTRAINT ck_stock_counts_note CHECK (length(note) <= 500 AND length(decision_note) <= 1000)
);
CREATE INDEX idx_stock_counts_location ON stock_counts (location_id, status);
CREATE INDEX idx_stock_counts_status ON stock_counts (status, created_at DESC);
-- Satu stock count per jadwal per lokasi per tanggal: job jadwal aman diulang.
CREATE UNIQUE INDEX uq_stock_counts_schedule_run ON stock_counts (schedule_id, location_id, business_date)
    WHERE schedule_id IS NOT NULL;

CREATE TABLE stock_count_lines (
    id              uuid          NOT NULL DEFAULT uuidv7(),
    stock_count_id  uuid          NOT NULL,
    line_no         integer       NOT NULL,
    product_id      uuid          NOT NULL,
    batch_id        uuid,
    -- Snapshot saldo sistem (Tersedia + Dialokasikan + Karantina) saat mulai; disembunyikan saat hitung buta.
    system_qty      numeric(18,3) NOT NULL DEFAULT 0,
    unit_cost       numeric(18,4) NOT NULL DEFAULT 0,
    counted_qty     numeric(18,3),
    variance_qty    numeric(18,3) NOT NULL DEFAULT 0,
    variance_amount numeric(18,2) NOT NULL DEFAULT 0,
    -- true = barang yang ditemukan saat hitung tetapi tidak ada di snapshot.
    is_extra        boolean       NOT NULL DEFAULT false,
    photo_file_id   uuid,
    note            text          NOT NULL DEFAULT '',
    counted_by      uuid,
    counted_at      timestamptz,
    CONSTRAINT pk_stock_count_lines PRIMARY KEY (id),
    CONSTRAINT uq_stock_count_lines_line_no UNIQUE (stock_count_id, line_no),
    CONSTRAINT fk_stock_count_lines_stock_count FOREIGN KEY (stock_count_id) REFERENCES stock_counts (id),
    CONSTRAINT ck_stock_count_lines_qty CHECK (system_qty >= 0 AND (counted_qty IS NULL OR counted_qty >= 0) AND unit_cost >= 0),
    CONSTRAINT ck_stock_count_lines_note CHECK (length(note) <= 500)
);
CREATE UNIQUE INDEX uq_stock_count_lines_product_batch ON stock_count_lines (stock_count_id, product_id, batch_id) NULLS NOT DISTINCT;

-- +goose Down
DROP TABLE stock_count_lines;
DROP TABLE stock_counts;
DROP TABLE stock_count_schedules;
DROP TABLE stock_transfer_lines;
DROP TABLE stock_transfers;
DROP TABLE quality_checks;
DROP TABLE inbound_lines;
DROP TABLE inbounds;
