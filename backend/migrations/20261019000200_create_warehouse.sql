-- +goose Up
-- Master gudang & lokasi stok (MD-09, WMS-01; Desain Data Model 6.1). Kode gudang dan kode lokasi unik permanen
-- termasuk baris yang sudah dihapus (Technical Guideline 5.13).

CREATE TABLE warehouses (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    code       text        NOT NULL,
    name       text        NOT NULL,
    address    text        NOT NULL DEFAULT '',
    is_active  boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid        NOT NULL,
    version    integer     NOT NULL DEFAULT 1,
    deleted_at timestamptz,
    deleted_by uuid,
    CONSTRAINT pk_warehouses PRIMARY KEY (id),
    CONSTRAINT uq_warehouses_code UNIQUE (code),
    CONSTRAINT ck_warehouses_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(code) <= 20),
    CONSTRAINT ck_warehouses_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT ck_warehouses_address CHECK (length(address) <= 500),
    CONSTRAINT ck_warehouses_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);

CREATE TABLE locations (
    id                  uuid        NOT NULL DEFAULT uuidv7(),
    code                text        NOT NULL,
    name                text        NOT NULL,
    -- Null untuk lokasi eksternal (maklon, konsinyasi, buffer vending, mesin vending).
    warehouse_id        uuid,
    location_type       text        NOT NULL,
    -- Pihak pemilik lokasi eksternal. Tanpa FK karena tabel pihak milik modul party/vending (Technical Guideline 2.3).
    external_party_type text,
    external_party_id   uuid,
    is_sellable         boolean     NOT NULL DEFAULT false,
    -- Lokasi standar dibuat otomatis bersama gudang (F1-114) dan ikut terhapus/dipulihkan bersama gudang.
    is_standard         boolean     NOT NULL DEFAULT false,
    is_active           boolean     NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid        NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid        NOT NULL,
    version             integer     NOT NULL DEFAULT 1,
    deleted_at          timestamptz,
    deleted_by          uuid,
    CONSTRAINT pk_locations PRIMARY KEY (id),
    CONSTRAINT uq_locations_code UNIQUE (code),
    CONSTRAINT fk_locations_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses (id),
    CONSTRAINT ck_locations_code CHECK (code ~ '^[A-Z0-9][A-Z0-9-]*$' AND length(code) <= 30),
    CONSTRAINT ck_locations_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT ck_locations_type CHECK (location_type IN (
        'inbound', 'finished_goods', 'ecommerce', 'raw_material', 'goods_material', 'returns', 'event',
        'toll_manufacturer', 'consignment', 'vending_buffer', 'vending_machine')),
    CONSTRAINT ck_locations_party_type CHECK (
        external_party_type IS NULL OR external_party_type IN ('supplier', 'partner', 'vending_machine')),
    -- Lokasi internal wajib berada di gudang; lokasi eksternal wajib menunjuk pihak dan tidak berada di gudang.
    CONSTRAINT ck_locations_placement CHECK (
        CASE WHEN location_type IN ('toll_manufacturer', 'consignment', 'vending_buffer', 'vending_machine')
             THEN warehouse_id IS NULL AND external_party_type IS NOT NULL AND external_party_id IS NOT NULL
             ELSE warehouse_id IS NOT NULL AND external_party_type IS NULL AND external_party_id IS NULL
        END),
    CONSTRAINT ck_locations_deleted CHECK ((deleted_at IS NULL) = (deleted_by IS NULL))
);
CREATE INDEX idx_locations_warehouse_id ON locations (warehouse_id);
CREATE INDEX idx_locations_type ON locations (location_type) WHERE deleted_at IS NULL;
-- Satu lokasi eksternal per jenis per pihak (mis. satu lokasi konsinyasi per mitra).
CREATE UNIQUE INDEX uq_locations_external_party ON locations (location_type, external_party_id)
    WHERE external_party_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP TABLE locations;
DROP TABLE warehouses;
