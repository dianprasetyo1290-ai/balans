-- +goose Up
-- Konsinyasi R1.3 (BRD 5.5, SLS-03, SLS-09; Desain Data Model 8; Desain Status Dokumen 5.6):
-- pengiriman titipan ke lokasi konsinyasi mitra (A-07) dan buffer vending (A-24), serta Rekonsiliasi Konsinyasi
-- bulanan (KSN, Berita Acara Rekonsiliasi) yang menerbitkan invoice konsinyasi (A-08) dan memproses selisih (A-23).
-- partner_id, product_id, location_id, batch_id, channel_id, transfer_id, stock_count_id, file id, dan invoice id
-- merujuk tabel modul lain atau tabel sales tanpa FK lintas modul (TG 2.3).

-- Pengiriman konsinyasi / buffer vending. Dokumen ERP dibuat sebagai Draft lalu dikirim (TRF + jurnal); modul
-- logistics (DO) nantinya memanggil API publik sales.ShipConsignment dengan source_type `delivery_order`.
CREATE TABLE consignment_deliveries (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    number               text,
    delivery_type        text          NOT NULL,
    partner_id           uuid          NOT NULL,
    partner_code         text          NOT NULL DEFAULT '',
    partner_name         text          NOT NULL DEFAULT '',
    partner_agreement_id uuid,
    agreement_number     text          NOT NULL DEFAULT '',
    from_location_id     uuid          NOT NULL,
    to_location_id       uuid          NOT NULL,
    channel_id           uuid          NOT NULL,
    sub_channel_id       uuid          NOT NULL,
    business_date        date          NOT NULL,
    status               text          NOT NULL DEFAULT 'draft',
    source_type          text          NOT NULL DEFAULT 'consignment_delivery',
    source_id            uuid,
    transfer_id          uuid,
    transfer_number      text          NOT NULL DEFAULT '',
    cost_amount          numeric(18,2) NOT NULL DEFAULT 0,
    note                 text          NOT NULL DEFAULT '',
    cancel_reason        text          NOT NULL DEFAULT '',
    shipped_at           timestamptz,
    received_at          timestamptz,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    created_by           uuid          NOT NULL,
    updated_at           timestamptz   NOT NULL DEFAULT now(),
    updated_by           uuid          NOT NULL,
    version              integer       NOT NULL DEFAULT 1,
    deleted_at           timestamptz,
    deleted_by           uuid,
    CONSTRAINT pk_consignment_deliveries PRIMARY KEY (id),
    CONSTRAINT uq_consignment_deliveries_number UNIQUE (number),
    CONSTRAINT ck_consignment_deliveries_type CHECK (delivery_type IN ('consignment', 'vending_buffer')),
    CONSTRAINT ck_consignment_deliveries_status CHECK (status IN ('draft', 'in_transit', 'completed', 'cancelled')),
    CONSTRAINT ck_consignment_deliveries_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_consignment_deliveries_cost CHECK (cost_amount >= 0),
    CONSTRAINT ck_consignment_deliveries_locations CHECK (from_location_id <> to_location_id)
);
-- Satu dokumen pengiriman sumber (DO logistics) hanya dicatat sekali.
CREATE UNIQUE INDEX uq_consignment_deliveries_source ON consignment_deliveries (source_type, source_id)
    WHERE source_id IS NOT NULL;
CREATE INDEX idx_consignment_deliveries_partner ON consignment_deliveries (partner_id, business_date DESC);
CREATE INDEX idx_consignment_deliveries_status ON consignment_deliveries (status, created_at DESC);

CREATE TABLE consignment_delivery_lines (
    id          uuid          NOT NULL DEFAULT uuidv7(),
    delivery_id uuid          NOT NULL,
    line_no     integer       NOT NULL,
    product_id  uuid          NOT NULL,
    batch_id    uuid,
    qty         numeric(18,3) NOT NULL,
    cost_amount numeric(18,2) NOT NULL DEFAULT 0,
    note        text          NOT NULL DEFAULT '',
    CONSTRAINT pk_consignment_delivery_lines PRIMARY KEY (id),
    CONSTRAINT uq_consignment_delivery_lines_line_no UNIQUE (delivery_id, line_no),
    CONSTRAINT fk_consignment_delivery_lines_delivery_id FOREIGN KEY (delivery_id) REFERENCES consignment_deliveries (id),
    CONSTRAINT ck_consignment_delivery_lines_qty CHECK (qty > 0),
    CONSTRAINT ck_consignment_delivery_lines_cost CHECK (cost_amount >= 0)
);

-- Rekonsiliasi Konsinyasi (KSN): periode (period_start s.d. cut_off_date), stock count lokasi mitra, PKS berlaku
-- (harga, termin, ketentuan selisih disalin saat mulai). invoice_id = invoice konsinyasi (A-08),
-- variance_invoice_id = invoice selisih yang ditagih ke mitra (A-23).
CREATE TABLE consignment_reconciliations (
    id                        uuid          NOT NULL DEFAULT uuidv7(),
    number                    text,
    partner_id                uuid          NOT NULL,
    partner_code              text          NOT NULL DEFAULT '',
    partner_name              text          NOT NULL DEFAULT '',
    partner_address           text          NOT NULL DEFAULT '',
    partner_agreement_id      uuid,
    agreement_number          text          NOT NULL DEFAULT '',
    payment_term_days         integer       NOT NULL DEFAULT 0,
    margin_percent            numeric(9,4),
    loss_borne_by             text          NOT NULL DEFAULT 'partner',
    partner_damage_borne_by   text          NOT NULL DEFAULT 'partner',
    product_defect_borne_by   text          NOT NULL DEFAULT 'balans',
    near_expiry_borne_by      text          NOT NULL DEFAULT 'balans',
    location_id               uuid          NOT NULL,
    channel_id                uuid          NOT NULL,
    sub_channel_id            uuid          NOT NULL,
    period_start              date,
    cut_off_date              date          NOT NULL,
    stock_count_id            uuid          NOT NULL,
    stock_count_number        text          NOT NULL DEFAULT '',
    partner_report_file_id    uuid,
    ba_file_id                uuid,
    status                    text          NOT NULL DEFAULT 'draft',
    note                      text          NOT NULL DEFAULT '',
    sales_amount              numeric(18,2) NOT NULL DEFAULT 0,
    billed_variance_amount    numeric(18,2) NOT NULL DEFAULT 0,
    absorbed_variance_cost    numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount               numeric(18,2) NOT NULL DEFAULT 0,
    invoice_id                uuid,
    variance_invoice_id       uuid,
    started_at                timestamptz,
    submitted_at              timestamptz,
    decided_at                timestamptz,
    decision_note             text          NOT NULL DEFAULT '',
    posted_at                 timestamptz,
    cancel_reason             text          NOT NULL DEFAULT '',
    created_at                timestamptz   NOT NULL DEFAULT now(),
    created_by                uuid          NOT NULL,
    updated_at                timestamptz   NOT NULL DEFAULT now(),
    updated_by                uuid          NOT NULL,
    version                   integer       NOT NULL DEFAULT 1,
    deleted_at                timestamptz,
    deleted_by                uuid,
    CONSTRAINT pk_consignment_reconciliations PRIMARY KEY (id),
    CONSTRAINT uq_consignment_reconciliations_number UNIQUE (number),
    CONSTRAINT ck_consignment_reconciliations_status CHECK (status IN (
        'draft', 'in_progress', 'pending_approval', 'rejected', 'posted', 'cancelled')),
    CONSTRAINT ck_consignment_reconciliations_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_consignment_reconciliations_borne_by CHECK (
        loss_borne_by IN ('partner', 'balans') AND partner_damage_borne_by IN ('partner', 'balans')
        AND product_defect_borne_by IN ('partner', 'balans') AND near_expiry_borne_by IN ('partner', 'balans')),
    CONSTRAINT ck_consignment_reconciliations_term CHECK (payment_term_days BETWEEN 0 AND 365),
    CONSTRAINT ck_consignment_reconciliations_period CHECK (period_start IS NULL OR period_start <= cut_off_date),
    CONSTRAINT ck_consignment_reconciliations_amounts CHECK (sales_amount >= 0 AND billed_variance_amount >= 0
        AND absorbed_variance_cost >= 0 AND cost_amount >= 0)
);
-- Satu rekonsiliasi terbuka per mitra; satu rekonsiliasi per mitra per cut-off; satu stock count dipakai sekali.
CREATE UNIQUE INDEX uq_consignment_reconciliations_open ON consignment_reconciliations (partner_id)
    WHERE status NOT IN ('posted', 'cancelled');
CREATE UNIQUE INDEX uq_consignment_reconciliations_cut_off ON consignment_reconciliations (partner_id, cut_off_date)
    WHERE status <> 'cancelled';
CREATE UNIQUE INDEX uq_consignment_reconciliations_stock_count ON consignment_reconciliations (stock_count_id)
    WHERE status <> 'cancelled';
CREATE INDEX idx_consignment_reconciliations_status ON consignment_reconciliations (status, cut_off_date DESC);

CREATE TABLE consignment_reconciliation_lines (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    reconciliation_id    uuid          NOT NULL,
    line_no              integer       NOT NULL,
    product_id           uuid          NOT NULL,
    opening_qty          numeric(18,3) NOT NULL DEFAULT 0,
    delivered_qty        numeric(18,3) NOT NULL DEFAULT 0,
    returned_qty         numeric(18,3) NOT NULL DEFAULT 0,
    counted_qty          numeric(18,3) NOT NULL DEFAULT 0,
    sold_qty             numeric(18,3) NOT NULL DEFAULT 0,
    reported_qty         numeric(18,3) NOT NULL DEFAULT 0,
    variance_qty         numeric(18,3) NOT NULL DEFAULT 0,
    variance_cause       text          NOT NULL DEFAULT '',
    variance_borne_by    text          NOT NULL DEFAULT '',
    unit_price_amount    numeric(18,2) NOT NULL DEFAULT 0,
    price_rule           text          NOT NULL DEFAULT '',
    price_description    text          NOT NULL DEFAULT '',
    sales_amount         numeric(18,2) NOT NULL DEFAULT 0,
    variance_amount      numeric(18,2) NOT NULL DEFAULT 0,
    cost_amount          numeric(18,2) NOT NULL DEFAULT 0,
    variance_cost_amount numeric(18,2) NOT NULL DEFAULT 0,
    note                 text          NOT NULL DEFAULT '',
    CONSTRAINT pk_consignment_reconciliation_lines PRIMARY KEY (id),
    CONSTRAINT uq_consignment_reconciliation_lines_line_no UNIQUE (reconciliation_id, line_no),
    CONSTRAINT fk_consignment_reconciliation_lines_reconciliation_id FOREIGN KEY (reconciliation_id)
        REFERENCES consignment_reconciliations (id),
    CONSTRAINT ck_consignment_reconciliation_lines_cause CHECK (variance_cause IN (
        '', 'lost', 'partner_damage', 'product_defect', 'near_expiry')),
    CONSTRAINT ck_consignment_reconciliation_lines_borne_by CHECK (variance_borne_by IN ('', 'partner', 'balans')),
    CONSTRAINT ck_consignment_reconciliation_lines_qty CHECK (opening_qty >= 0 AND delivered_qty >= 0
        AND returned_qty >= 0 AND counted_qty >= 0 AND reported_qty >= 0),
    CONSTRAINT ck_consignment_reconciliation_lines_amounts CHECK (unit_price_amount >= 0 AND sales_amount >= 0
        AND variance_amount >= 0 AND cost_amount >= 0 AND variance_cost_amount >= 0)
);

-- +goose Down
DROP TABLE consignment_reconciliation_lines;
DROP TABLE consignment_reconciliations;
DROP TABLE consignment_delivery_lines;
DROP TABLE consignment_deliveries;
