-- +goose Up
-- Master pelanggan, mitra & PKS, kode produk mitra, pemasok & maklon (BRD MD-06, MD-07, MD-08;
-- Desain Data Model 4.3). Kolom *_enc dienkripsi aplikasi dengan AES-256-GCM (Technical Guideline 6.7).
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SEQUENCE customer_code_seq;

CREATE TABLE customers (
    id                        uuid        NOT NULL DEFAULT uuidv7(),
    code                      text        NOT NULL DEFAULT ('PLG-' || lpad(nextval('customer_code_seq')::text, 6, '0')),
    full_name                 text        NOT NULL,
    -- phone apa adanya dari input; phone_normalized berformat +62… dipakai untuk cek duplikat (BRD 5.8).
    phone                     text        NOT NULL,
    phone_normalized          text        NOT NULL,
    email                     text        NOT NULL DEFAULT '',
    customer_type             text        NOT NULL,
    -- employee_id merujuk hris.employees (tanpa FK lintas modul); harga karyawan berlaku selama karyawan aktif.
    employee_id               uuid,
    marketing_consent_at      timestamptz,
    marketing_consent_version text        NOT NULL DEFAULT '',
    notes                     text        NOT NULL DEFAULT '',
    created_at                timestamptz NOT NULL DEFAULT now(),
    created_by                uuid        NOT NULL,
    updated_at                timestamptz NOT NULL DEFAULT now(),
    updated_by                uuid        NOT NULL,
    version                   integer     NOT NULL DEFAULT 1,
    deleted_at                timestamptz,
    deleted_by                uuid,
    CONSTRAINT pk_customers PRIMARY KEY (id),
    CONSTRAINT uq_customers_code UNIQUE (code),
    CONSTRAINT ck_customers_type CHECK (customer_type IN ('retail', 'b2b', 'reseller', 'employee')),
    CONSTRAINT ck_customers_employee CHECK ((customer_type = 'employee') = (employee_id IS NOT NULL)),
    CONSTRAINT ck_customers_phone_normalized CHECK (phone_normalized ~ '^\+62[1-9][0-9]{7,11}$'),
    CONSTRAINT ck_customers_consent CHECK (marketing_consent_at IS NOT NULL OR marketing_consent_version = '')
);
CREATE UNIQUE INDEX uq_customers_phone_normalized ON customers (phone_normalized) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_customers_employee_id ON customers (employee_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_customers_full_name_trgm ON customers USING gin (lower(full_name) gin_trgm_ops);
CREATE INDEX idx_customers_created_at ON customers (created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE customer_addresses (
    id             uuid         NOT NULL DEFAULT uuidv7(),
    customer_id    uuid         NOT NULL,
    label          text         NOT NULL DEFAULT '',
    recipient_name text         NOT NULL,
    phone          text         NOT NULL DEFAULT '',
    address_line   text         NOT NULL,
    district       text         NOT NULL DEFAULT '',
    city           text         NOT NULL,
    province       text         NOT NULL DEFAULT '',
    postal_code    text         NOT NULL DEFAULT '',
    latitude       numeric(9,6),
    longitude      numeric(9,6),
    is_default     boolean      NOT NULL DEFAULT false,
    created_at     timestamptz  NOT NULL DEFAULT now(),
    created_by     uuid         NOT NULL,
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    updated_by     uuid         NOT NULL,
    version        integer      NOT NULL DEFAULT 1,
    deleted_at     timestamptz,
    deleted_by     uuid,
    CONSTRAINT pk_customer_addresses PRIMARY KEY (id),
    CONSTRAINT fk_customer_addresses_customer FOREIGN KEY (customer_id) REFERENCES customers (id),
    CONSTRAINT ck_customer_addresses_latitude CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CONSTRAINT ck_customer_addresses_longitude CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);
CREATE INDEX idx_customer_addresses_customer_id ON customer_addresses (customer_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_customer_addresses_default ON customer_addresses (customer_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE TABLE partners (
    id                   uuid        NOT NULL DEFAULT uuidv7(),
    code                 text        NOT NULL,
    name                 text        NOT NULL,
    partner_type         text        NOT NULL,
    npwp_enc             text        NOT NULL DEFAULT '',
    address              text        NOT NULL DEFAULT '',
    pic_name             text        NOT NULL DEFAULT '',
    pic_phone            text        NOT NULL DEFAULT '',
    sales_user_id        uuid,
    -- Merchandiser yang ditugaskan; membatasi akses stock count partner (MD-07).
    merchandiser_user_id uuid,
    notes                text        NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    created_by           uuid        NOT NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    updated_by           uuid        NOT NULL,
    version              integer     NOT NULL DEFAULT 1,
    deleted_at           timestamptz,
    deleted_by           uuid,
    CONSTRAINT pk_partners PRIMARY KEY (id),
    -- Kode partner tidak dipakai ulang walau dihapus (Technical Guideline 5.13).
    CONSTRAINT uq_partners_code UNIQUE (code),
    CONSTRAINT ck_partners_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$'),
    CONSTRAINT ck_partners_type CHECK (partner_type IN ('modern_trade', 'consignment', 'b2b'))
);
CREATE INDEX idx_partners_sales_user_id ON partners (sales_user_id);
CREATE INDEX idx_partners_merchandiser_user_id ON partners (merchandiser_user_id);
CREATE INDEX idx_partners_name_trgm ON partners USING gin (lower(name) gin_trgm_ops);

-- PKS mitra (effective-dated, Technical Guideline 5.14). Periode inklusif valid_from..valid_to; satu jenis PKS
-- per mitra tidak boleh tumpang tindih. DO konsinyasi diblokir jika tidak ada PKS konsinyasi berlaku (BRD 5.5).
CREATE TABLE partner_agreements (
    id                         uuid         NOT NULL DEFAULT uuidv7(),
    partner_id                 uuid         NOT NULL,
    agreement_number           text         NOT NULL,
    agreement_type             text         NOT NULL,
    valid_from                 date         NOT NULL,
    valid_to                   date         NOT NULL,
    -- file_object_id merujuk filestore.file_objects (PDF PKS), tanpa FK lintas modul.
    file_object_id             uuid,
    -- price_list_id merujuk pricing.price_lists (dibangun modul pricing), tanpa FK lintas modul.
    price_list_id              uuid,
    consignment_margin_percent numeric(9,4),
    payment_term_days          integer      NOT NULL,
    loss_borne_by              text         NOT NULL DEFAULT 'partner',
    partner_damage_borne_by    text         NOT NULL DEFAULT 'partner',
    product_defect_borne_by    text         NOT NULL DEFAULT 'balans',
    near_expiry_borne_by       text         NOT NULL DEFAULT 'balans',
    notes                      text         NOT NULL DEFAULT '',
    created_at                 timestamptz  NOT NULL DEFAULT now(),
    created_by                 uuid         NOT NULL,
    updated_at                 timestamptz  NOT NULL DEFAULT now(),
    updated_by                 uuid         NOT NULL,
    version                    integer      NOT NULL DEFAULT 1,
    deleted_at                 timestamptz,
    deleted_by                 uuid,
    CONSTRAINT pk_partner_agreements PRIMARY KEY (id),
    CONSTRAINT fk_partner_agreements_partner FOREIGN KEY (partner_id) REFERENCES partners (id),
    CONSTRAINT ck_partner_agreements_type CHECK (agreement_type IN ('consignment', 'modern_trade')),
    CONSTRAINT ck_partner_agreements_validity CHECK (valid_to >= valid_from),
    CONSTRAINT ck_partner_agreements_payment_term CHECK (payment_term_days BETWEEN 0 AND 365),
    CONSTRAINT ck_partner_agreements_margin CHECK (
        consignment_margin_percent IS NULL OR consignment_margin_percent BETWEEN 0 AND 100),
    CONSTRAINT ck_partner_agreements_borne_by CHECK (
        loss_borne_by IN ('partner', 'balans') AND partner_damage_borne_by IN ('partner', 'balans')
        AND product_defect_borne_by IN ('partner', 'balans') AND near_expiry_borne_by IN ('partner', 'balans')),
    CONSTRAINT ex_partner_agreements_period EXCLUDE USING gist (
        partner_id WITH =,
        agreement_type WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&) WHERE (deleted_at IS NULL)
);
CREATE INDEX idx_partner_agreements_partner_id ON partner_agreements (partner_id, valid_from DESC);
CREATE INDEX idx_partner_agreements_file_object_id ON partner_agreements (file_object_id);
CREATE INDEX idx_partner_agreements_price_list_id ON partner_agreements (price_list_id);
CREATE UNIQUE INDEX uq_partner_agreements_number ON partner_agreements (partner_id, agreement_number)
    WHERE deleted_at IS NULL;

-- Kode artikel & barcode retailer per SKU per mitra (MD-07). product_id merujuk catalog.products tanpa FK
-- lintas modul.
CREATE TABLE partner_product_codes (
    id           uuid        NOT NULL DEFAULT uuidv7(),
    partner_id   uuid        NOT NULL,
    product_id   uuid        NOT NULL,
    article_code text        NOT NULL,
    barcode      text        NOT NULL DEFAULT '',
    description  text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid        NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid        NOT NULL,
    version      integer     NOT NULL DEFAULT 1,
    deleted_at   timestamptz,
    deleted_by   uuid,
    CONSTRAINT pk_partner_product_codes PRIMARY KEY (id),
    CONSTRAINT fk_partner_product_codes_partner FOREIGN KEY (partner_id) REFERENCES partners (id)
);
CREATE UNIQUE INDEX uq_partner_product_codes_partner_product ON partner_product_codes (partner_id, product_id)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_partner_product_codes_partner_article ON partner_product_codes (partner_id, article_code)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_partner_product_codes_product_id ON partner_product_codes (product_id);

CREATE TABLE suppliers (
    id                  uuid        NOT NULL DEFAULT uuidv7(),
    code                text        NOT NULL,
    name                text        NOT NULL,
    -- toll_manufacturer = maklon.
    supplier_type       text        NOT NULL,
    is_business_entity  boolean     NOT NULL,
    npwp_enc            text        NOT NULL DEFAULT '',
    default_withholding text        NOT NULL DEFAULT 'none',
    payment_term_days   integer     NOT NULL DEFAULT 0,
    address             text        NOT NULL DEFAULT '',
    phone               text        NOT NULL DEFAULT '',
    email               text        NOT NULL DEFAULT '',
    pic_name            text        NOT NULL DEFAULT '',
    notes               text        NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid        NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid        NOT NULL,
    version             integer     NOT NULL DEFAULT 1,
    deleted_at          timestamptz,
    deleted_by          uuid,
    CONSTRAINT pk_suppliers PRIMARY KEY (id),
    CONSTRAINT uq_suppliers_code UNIQUE (code),
    CONSTRAINT ck_suppliers_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$'),
    CONSTRAINT ck_suppliers_type CHECK (supplier_type IN ('supplier', 'toll_manufacturer')),
    CONSTRAINT ck_suppliers_withholding CHECK (default_withholding IN ('none', 'pph23', 'pph21', 'pph4_2')),
    CONSTRAINT ck_suppliers_payment_term CHECK (payment_term_days BETWEEN 0 AND 365)
);
CREATE INDEX idx_suppliers_name_trgm ON suppliers USING gin (lower(name) gin_trgm_ops);

-- Rekening bank pihak (Fase 1: pemasok). Nomor rekening terenkripsi.
CREATE TABLE party_bank_accounts (
    id                 uuid        NOT NULL DEFAULT uuidv7(),
    party_type         text        NOT NULL,
    party_id           uuid        NOT NULL,
    bank_name          text        NOT NULL,
    account_number_enc text        NOT NULL,
    account_name       text        NOT NULL,
    is_default         boolean     NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    created_by         uuid        NOT NULL,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    updated_by         uuid        NOT NULL,
    version            integer     NOT NULL DEFAULT 1,
    deleted_at         timestamptz,
    deleted_by         uuid,
    CONSTRAINT pk_party_bank_accounts PRIMARY KEY (id),
    CONSTRAINT ck_party_bank_accounts_party_type CHECK (party_type IN ('supplier', 'partner'))
);
CREATE INDEX idx_party_bank_accounts_party ON party_bank_accounts (party_type, party_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_party_bank_accounts_default ON party_bank_accounts (party_type, party_id)
    WHERE is_default AND deleted_at IS NULL;

-- +goose Down
DROP TABLE party_bank_accounts;
DROP TABLE suppliers;
DROP TABLE partner_product_codes;
DROP TABLE partner_agreements;
DROP TABLE partners;
DROP TABLE customer_addresses;
DROP TABLE customers;
DROP SEQUENCE customer_code_seq;
