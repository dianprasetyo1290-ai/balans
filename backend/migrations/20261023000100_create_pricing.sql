-- +goose Up
-- Daftar harga, versi berlaku, harga khusus, promo, harga bertingkat, dan batas diskon per role
-- (BRD 5.18, MD-04, MD-05, MD-11, MD-12; Desain Data Model 4.2). Semua tabel ⏱ memakai rentang tanggal
-- inklusif valid_from..valid_to (valid_to kosong = tanpa batas) dengan EXCLUDE agar periode tidak tumpang
-- tindih (Technical Guideline 5.14). Kolom *_id ke modul lain (produk, channel, mitra, pelanggan) tanpa FK
-- lintas modul.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Header daftar harga. price_list_type menentukan cakupan: base (Harga Dasar & HET, satu-satunya), channel
-- (per channel / sub-channel), partner (per mitra MT/konsinyasi), reseller (per level, Fase 2), employee
-- (harga karyawan), vending (per channel vending / mesin). basis percent = persentase dari daftar harga acuan
-- (mis. harga karyawan = 90% harga website).
CREATE TABLE price_lists (
    id                      uuid        NOT NULL DEFAULT uuidv7(),
    code                    text        NOT NULL,
    name                    text        NOT NULL,
    price_list_type         text        NOT NULL,
    channel_id              uuid,
    sub_channel_id          uuid,
    partner_id              uuid,
    reseller_level          text        NOT NULL DEFAULT '',
    basis                   text        NOT NULL DEFAULT 'fixed',
    reference_price_list_id uuid,
    description             text        NOT NULL DEFAULT '',
    is_active               boolean     NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL DEFAULT now(),
    created_by              uuid        NOT NULL,
    updated_at              timestamptz NOT NULL DEFAULT now(),
    updated_by              uuid        NOT NULL,
    version                 integer     NOT NULL DEFAULT 1,
    deleted_at              timestamptz,
    deleted_by              uuid,
    CONSTRAINT pk_price_lists PRIMARY KEY (id),
    -- Kode tidak dipakai ulang walau dihapus (Technical Guideline 5.13).
    CONSTRAINT uq_price_lists_code UNIQUE (code),
    CONSTRAINT fk_price_lists_reference FOREIGN KEY (reference_price_list_id) REFERENCES price_lists (id),
    CONSTRAINT ck_price_lists_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(code) <= 30),
    CONSTRAINT ck_price_lists_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT ck_price_lists_type CHECK (
        price_list_type IN ('base', 'channel', 'partner', 'reseller', 'employee', 'vending')),
    CONSTRAINT ck_price_lists_basis CHECK (basis IN ('fixed', 'percent')),
    CONSTRAINT ck_price_lists_basis_reference CHECK ((basis = 'percent') = (reference_price_list_id IS NOT NULL)),
    CONSTRAINT ck_price_lists_reference_not_self CHECK (reference_price_list_id IS DISTINCT FROM id),
    CONSTRAINT ck_price_lists_sub_channel CHECK (sub_channel_id IS NULL OR channel_id IS NOT NULL),
    CONSTRAINT ck_price_lists_scope CHECK (CASE price_list_type
        WHEN 'base' THEN channel_id IS NULL AND partner_id IS NULL AND reseller_level = '' AND basis = 'fixed'
        WHEN 'employee' THEN channel_id IS NULL AND partner_id IS NULL AND reseller_level = ''
        WHEN 'partner' THEN partner_id IS NOT NULL AND channel_id IS NULL AND reseller_level = ''
        WHEN 'reseller' THEN reseller_level <> '' AND channel_id IS NULL AND partner_id IS NULL
        ELSE channel_id IS NOT NULL AND partner_id IS NULL AND reseller_level = ''
    END),
    CONSTRAINT ck_price_lists_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);
-- Satu daftar harga aktif per cakupan; channel dan vending berbagi cakupan channel/sub-channel.
CREATE UNIQUE INDEX uq_price_lists_scope ON price_lists (
    (CASE WHEN price_list_type = 'vending' THEN 'channel' ELSE price_list_type END),
    channel_id, sub_channel_id, partner_id, reseller_level) NULLS NOT DISTINCT
    WHERE deleted_at IS NULL;
CREATE INDEX idx_price_lists_channel_id ON price_lists (channel_id, sub_channel_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_price_lists_partner_id ON price_lists (partner_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_price_lists_reference ON price_lists (reference_price_list_id);

-- Versi daftar harga. Versi baru selalu draft → pending_approval → active (approval price_list_version:
-- Head Sales → Head Finance & Accounting). Versi aktif tidak pernah diubah; transaksi menyimpan harga dan ID
-- versi yang dipakai sehingga versi baru tidak mengubah transaksi lama.
CREATE TABLE price_list_versions (
    id             uuid         NOT NULL DEFAULT uuidv7(),
    price_list_id  uuid         NOT NULL,
    version_no     integer      NOT NULL,
    valid_from     date         NOT NULL,
    valid_to       date,
    status         text         NOT NULL DEFAULT 'draft',
    -- percent dipakai daftar harga basis percent: harga = harga acuan × percent / 100.
    percent        numeric(9,4),
    notes          text         NOT NULL DEFAULT '',
    submitted_at   timestamptz,
    submitted_by   uuid,
    decided_at     timestamptz,
    decided_by     uuid,
    created_at     timestamptz  NOT NULL DEFAULT now(),
    created_by     uuid         NOT NULL,
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    updated_by     uuid         NOT NULL,
    version        integer      NOT NULL DEFAULT 1,
    CONSTRAINT pk_price_list_versions PRIMARY KEY (id),
    CONSTRAINT uq_price_list_versions_no UNIQUE (price_list_id, version_no),
    CONSTRAINT fk_price_list_versions_price_list FOREIGN KEY (price_list_id) REFERENCES price_lists (id),
    CONSTRAINT ck_price_list_versions_status CHECK (
        status IN ('draft', 'pending_approval', 'active', 'rejected', 'cancelled')),
    CONSTRAINT ck_price_list_versions_validity CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT ck_price_list_versions_percent CHECK (percent IS NULL OR (percent > 0 AND percent <= 1000)),
    CONSTRAINT ck_price_list_versions_no CHECK (version_no > 0),
    CONSTRAINT ex_price_list_versions_period EXCLUDE USING gist (
        price_list_id WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (status = 'active')
);
-- Paling banyak satu versi yang sedang disiapkan atau diajukan per daftar harga.
CREATE UNIQUE INDEX uq_price_list_versions_open ON price_list_versions (price_list_id)
    WHERE status IN ('draft', 'pending_approval');
CREATE INDEX idx_price_list_versions_active ON price_list_versions (price_list_id, valid_from DESC)
    WHERE status = 'active';

CREATE TABLE price_list_items (
    id                      uuid          NOT NULL DEFAULT uuidv7(),
    price_list_version_id   uuid          NOT NULL,
    product_id              uuid          NOT NULL,
    price_amount            numeric(18,2) NOT NULL,
    -- HET (harga eceran tertinggi), hanya di daftar Harga Dasar & HET.
    max_retail_price_amount numeric(18,2),
    created_at              timestamptz   NOT NULL DEFAULT now(),
    created_by              uuid          NOT NULL,
    CONSTRAINT pk_price_list_items PRIMARY KEY (id),
    CONSTRAINT uq_price_list_items_product UNIQUE (price_list_version_id, product_id),
    CONSTRAINT fk_price_list_items_version FOREIGN KEY (price_list_version_id) REFERENCES price_list_versions (id),
    CONSTRAINT ck_price_list_items_price CHECK (price_amount >= 0),
    CONSTRAINT ck_price_list_items_max_retail CHECK (
        max_retail_price_amount IS NULL OR max_retail_price_amount >= price_amount)
);
CREATE INDEX idx_price_list_items_product_id ON price_list_items (product_id);

-- Harga khusus per pelanggan atau mitra (prioritas 1).
CREATE TABLE special_prices (
    id           uuid          NOT NULL DEFAULT uuidv7(),
    party_type   text          NOT NULL,
    party_id     uuid          NOT NULL,
    product_id   uuid          NOT NULL,
    price_amount numeric(18,2) NOT NULL,
    valid_from   date          NOT NULL,
    valid_to     date,
    notes        text          NOT NULL DEFAULT '',
    created_at   timestamptz   NOT NULL DEFAULT now(),
    created_by   uuid          NOT NULL,
    updated_at   timestamptz   NOT NULL DEFAULT now(),
    updated_by   uuid          NOT NULL,
    version      integer       NOT NULL DEFAULT 1,
    deleted_at   timestamptz,
    deleted_by   uuid,
    CONSTRAINT pk_special_prices PRIMARY KEY (id),
    CONSTRAINT ck_special_prices_party_type CHECK (party_type IN ('customer', 'partner')),
    CONSTRAINT ck_special_prices_price CHECK (price_amount >= 0),
    CONSTRAINT ck_special_prices_validity CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT ck_special_prices_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL)),
    CONSTRAINT ex_special_prices_period EXCLUDE USING gist (
        party_type WITH =,
        party_id WITH =,
        product_id WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (deleted_at IS NULL)
);
CREATE INDEX idx_special_prices_product_id ON special_prices (product_id);

-- Promo periode per channel (prioritas 2). channel_id kosong = semua channel.
CREATE TABLE promotions (
    id             uuid        NOT NULL DEFAULT uuidv7(),
    code           text        NOT NULL,
    name           text        NOT NULL,
    channel_id     uuid,
    sub_channel_id uuid,
    valid_from     date        NOT NULL,
    valid_to       date        NOT NULL,
    is_active      boolean     NOT NULL DEFAULT true,
    description    text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    created_by     uuid        NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    updated_by     uuid        NOT NULL,
    version        integer     NOT NULL DEFAULT 1,
    deleted_at     timestamptz,
    deleted_by     uuid,
    CONSTRAINT pk_promotions PRIMARY KEY (id),
    CONSTRAINT uq_promotions_code UNIQUE (code),
    CONSTRAINT ck_promotions_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(code) <= 30),
    CONSTRAINT ck_promotions_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT ck_promotions_sub_channel CHECK (sub_channel_id IS NULL OR channel_id IS NOT NULL),
    CONSTRAINT ck_promotions_validity CHECK (valid_to >= valid_from),
    CONSTRAINT ck_promotions_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);
CREATE INDEX idx_promotions_period ON promotions (valid_from, valid_to) WHERE deleted_at IS NULL AND is_active;
CREATE INDEX idx_promotions_channel_id ON promotions (channel_id, sub_channel_id);

-- Harga promo per produk: harga tetap atau potongan persen dari harga normal.
CREATE TABLE promotion_items (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    promotion_id     uuid          NOT NULL,
    product_id       uuid          NOT NULL,
    price_amount     numeric(18,2),
    discount_percent numeric(9,4),
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    CONSTRAINT pk_promotion_items PRIMARY KEY (id),
    CONSTRAINT uq_promotion_items_product UNIQUE (promotion_id, product_id),
    CONSTRAINT fk_promotion_items_promotion FOREIGN KEY (promotion_id) REFERENCES promotions (id),
    CONSTRAINT ck_promotion_items_value CHECK ((price_amount IS NULL) <> (discount_percent IS NULL)),
    CONSTRAINT ck_promotion_items_price CHECK (price_amount IS NULL OR price_amount >= 0),
    CONSTRAINT ck_promotion_items_percent CHECK (
        discount_percent IS NULL OR (discount_percent > 0 AND discount_percent <= 100))
);
CREATE INDEX idx_promotion_items_product_id ON promotion_items (product_id);

-- Harga bertingkat per qty minimum (prioritas 3). price_list_id kosong = berlaku untuk semua daftar harga.
CREATE TABLE tiered_prices (
    id            uuid          NOT NULL DEFAULT uuidv7(),
    price_list_id uuid,
    product_id    uuid          NOT NULL,
    min_qty       numeric(18,3) NOT NULL,
    price_amount  numeric(18,2) NOT NULL,
    valid_from    date          NOT NULL,
    valid_to      date,
    notes         text          NOT NULL DEFAULT '',
    created_at    timestamptz   NOT NULL DEFAULT now(),
    created_by    uuid          NOT NULL,
    updated_at    timestamptz   NOT NULL DEFAULT now(),
    updated_by    uuid          NOT NULL,
    version       integer       NOT NULL DEFAULT 1,
    deleted_at    timestamptz,
    deleted_by    uuid,
    CONSTRAINT pk_tiered_prices PRIMARY KEY (id),
    CONSTRAINT fk_tiered_prices_price_list FOREIGN KEY (price_list_id) REFERENCES price_lists (id),
    CONSTRAINT ck_tiered_prices_min_qty CHECK (min_qty > 0),
    CONSTRAINT ck_tiered_prices_price CHECK (price_amount >= 0),
    CONSTRAINT ck_tiered_prices_validity CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT ck_tiered_prices_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL)),
    CONSTRAINT ex_tiered_prices_period EXCLUDE USING gist (
        (coalesce(price_list_id, '00000000-0000-0000-0000-000000000000'::uuid)) WITH =,
        product_id WITH =,
        min_qty WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (deleted_at IS NULL)
);
CREATE INDEX idx_tiered_prices_product_id ON tiered_prices (product_id);
CREATE INDEX idx_tiered_prices_price_list_id ON tiered_prices (price_list_id);

-- Batas diskon manual per role (MD-12): persen per baris dan per dokumen. Diskon di atas batas meminta
-- approval sales_order_discount (Head Sales).
CREATE TABLE discount_limits (
    id                            uuid         NOT NULL DEFAULT uuidv7(),
    role_code                     text         NOT NULL,
    max_line_discount_percent     numeric(9,4) NOT NULL,
    max_document_discount_percent numeric(9,4) NOT NULL,
    valid_from                    date         NOT NULL,
    valid_to                      date,
    notes                         text         NOT NULL DEFAULT '',
    created_at                    timestamptz  NOT NULL DEFAULT now(),
    created_by                    uuid         NOT NULL,
    updated_at                    timestamptz  NOT NULL DEFAULT now(),
    updated_by                    uuid         NOT NULL,
    version                       integer      NOT NULL DEFAULT 1,
    deleted_at                    timestamptz,
    deleted_by                    uuid,
    CONSTRAINT pk_discount_limits PRIMARY KEY (id),
    CONSTRAINT ck_discount_limits_role_code CHECK (role_code ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT ck_discount_limits_line CHECK (max_line_discount_percent BETWEEN 0 AND 100),
    CONSTRAINT ck_discount_limits_document CHECK (max_document_discount_percent BETWEEN 0 AND 100),
    CONSTRAINT ck_discount_limits_validity CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT ck_discount_limits_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL)),
    CONSTRAINT ex_discount_limits_period EXCLUDE USING gist (
        role_code WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (deleted_at IS NULL)
);

-- Daftar Harga Dasar & HET selalu ada (prioritas 5); harganya diisi lewat versi yang disetujui.
INSERT INTO price_lists (code, name, price_list_type, description, created_by, updated_by) VALUES
    ('HARGA-DASAR', 'Harga Dasar & HET', 'base', 'Acuan semua channel bila tidak ada aturan harga lain.',
     '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DROP TABLE discount_limits;
DROP TABLE tiered_prices;
DROP TABLE promotion_items;
DROP TABLE promotions;
DROP TABLE special_prices;
DROP TABLE price_list_items;
DROP TABLE price_list_versions;
DROP TABLE price_lists;
