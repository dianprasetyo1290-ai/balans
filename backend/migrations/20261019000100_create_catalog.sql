-- +goose Up
-- Master produk, packaging, material, dan bundel virtual (MD-01, MD-02, MD-03; Desain Data Model 4.1).
-- Kode bisnis (kode kategori, SKU) unik permanen termasuk baris yang sudah dihapus (Technical Guideline 5.13).

CREATE TABLE product_categories (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    code       text        NOT NULL,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid        NOT NULL,
    version    integer     NOT NULL DEFAULT 1,
    deleted_at timestamptz,
    deleted_by uuid,
    CONSTRAINT pk_product_categories PRIMARY KEY (id),
    CONSTRAINT uq_product_categories_code UNIQUE (code),
    CONSTRAINT ck_product_categories_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(code) <= 20),
    CONSTRAINT ck_product_categories_name CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    CONSTRAINT ck_product_categories_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);

CREATE TABLE products (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    sku              text          NOT NULL,
    name             text          NOT NULL,
    variant          text          NOT NULL DEFAULT '',
    -- finished_good = Produk Jadi, packaging = RM, reprocess_material = GM (Material).
    product_type     text          NOT NULL,
    packaging_type   text,
    uom              text          NOT NULL,
    shelf_life_days  integer,
    has_batch        boolean       NOT NULL,
    -- Bundel virtual tidak punya stok sendiri; dipecah ke komponen sebelum memanggil inventory (TG 5.7.4).
    is_bundle        boolean       NOT NULL DEFAULT false,
    category_id      uuid,
    min_stock_qty    numeric(18,3),
    units_per_carton integer,
    is_active        boolean       NOT NULL DEFAULT true,
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    updated_at       timestamptz   NOT NULL DEFAULT now(),
    updated_by       uuid          NOT NULL,
    version          integer       NOT NULL DEFAULT 1,
    deleted_at       timestamptz,
    deleted_by       uuid,
    CONSTRAINT pk_products PRIMARY KEY (id),
    CONSTRAINT uq_products_sku UNIQUE (sku),
    CONSTRAINT fk_products_category FOREIGN KEY (category_id) REFERENCES product_categories (id),
    CONSTRAINT ck_products_sku CHECK (sku ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(sku) <= 40),
    CONSTRAINT ck_products_name CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    CONSTRAINT ck_products_product_type CHECK (product_type IN ('finished_good', 'packaging', 'reprocess_material')),
    CONSTRAINT ck_products_packaging_type CHECK (packaging_type IS NULL OR packaging_type IN ('tube', 'pouch', 'roll_sachet')),
    CONSTRAINT ck_products_uom CHECK (length(btrim(uom)) BETWEEN 1 AND 20),
    CONSTRAINT ck_products_shelf_life CHECK (shelf_life_days IS NULL OR shelf_life_days > 0),
    -- Umur simpan wajib untuk Produk Jadi yang punya stok (MD-01); Produk Jadi selalu memakai batch (FEFO).
    CONSTRAINT ck_products_finished_good CHECK (
        product_type <> 'finished_good' OR is_bundle OR (shelf_life_days IS NOT NULL AND has_batch)),
    CONSTRAINT ck_products_bundle CHECK (NOT is_bundle OR (product_type = 'finished_good' AND NOT has_batch)),
    CONSTRAINT ck_products_min_stock CHECK (min_stock_qty IS NULL OR min_stock_qty >= 0),
    CONSTRAINT ck_products_units_per_carton CHECK (units_per_carton IS NULL OR units_per_carton > 0),
    CONSTRAINT ck_products_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);
CREATE INDEX idx_products_category_id ON products (category_id);
CREATE INDEX idx_products_type ON products (product_type, is_bundle) WHERE deleted_at IS NULL;

-- Satu SKU boleh punya beberapa barcode; tepat satu barcode utama dicetak di label produk (MD-01).
CREATE TABLE product_barcodes (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    product_id uuid        NOT NULL,
    barcode    text        NOT NULL,
    symbology  text        NOT NULL,
    is_primary boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid        NOT NULL,
    CONSTRAINT pk_product_barcodes PRIMARY KEY (id),
    CONSTRAINT uq_product_barcodes_barcode UNIQUE (barcode),
    CONSTRAINT fk_product_barcodes_product FOREIGN KEY (product_id) REFERENCES products (id),
    CONSTRAINT ck_product_barcodes_symbology CHECK (symbology IN ('ean13', 'code128')),
    CONSTRAINT ck_product_barcodes_barcode CHECK (length(barcode) BETWEEN 1 AND 48)
);
CREATE INDEX idx_product_barcodes_product_id ON product_barcodes (product_id);
CREATE UNIQUE INDEX uq_product_barcodes_primary ON product_barcodes (product_id) WHERE is_primary;

CREATE TABLE product_bundle_components (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    bundle_product_id    uuid          NOT NULL,
    component_product_id uuid          NOT NULL,
    qty                  numeric(18,3) NOT NULL,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    created_by           uuid          NOT NULL,
    CONSTRAINT pk_product_bundle_components PRIMARY KEY (id),
    CONSTRAINT uq_product_bundle_components_component UNIQUE (bundle_product_id, component_product_id),
    CONSTRAINT fk_product_bundle_components_bundle FOREIGN KEY (bundle_product_id) REFERENCES products (id),
    CONSTRAINT fk_product_bundle_components_component FOREIGN KEY (component_product_id) REFERENCES products (id),
    CONSTRAINT ck_product_bundle_components_qty CHECK (qty > 0),
    CONSTRAINT ck_product_bundle_components_self CHECK (bundle_product_id <> component_product_id)
);
CREATE INDEX idx_product_bundle_components_component ON product_bundle_components (component_product_id);

-- +goose Down
DROP TABLE product_bundle_components;
DROP TABLE product_barcodes;
DROP TABLE products;
DROP TABLE product_categories;
