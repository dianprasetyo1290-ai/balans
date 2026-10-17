-- +goose Up
-- Master alasan retur (MD-13; Desain Data Model bagian 8, BRD 5.11). Dipakai baris retur penjualan
-- (sales_return_lines.return_reason_id) dan retur pembelian untuk analisis RPT-09.
-- default_disposition hanya usulan awal di Disposisi Retur (WMS-07); keputusan tetap di QC.
-- Kode unik permanen: tidak dipakai ulang walau alasan dihapus (Technical Guideline 5.13).
CREATE TABLE return_reasons (
    id                  uuid        NOT NULL DEFAULT uuidv7(),
    code                text        NOT NULL,
    name                text        NOT NULL,
    applies_to          text        NOT NULL DEFAULT 'both',
    default_disposition text,
    description         text        NOT NULL DEFAULT '',
    is_active           boolean     NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid        NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid        NOT NULL,
    version             integer     NOT NULL DEFAULT 1,
    deleted_at          timestamptz,
    deleted_by          uuid,
    CONSTRAINT pk_return_reasons PRIMARY KEY (id),
    CONSTRAINT uq_return_reasons_code UNIQUE (code),
    CONSTRAINT ck_return_reasons_code CHECK (code ~ '^[A-Z][A-Z0-9_]{1,29}$'),
    CONSTRAINT ck_return_reasons_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_return_reasons_applies_to CHECK (applies_to IN ('sales_return', 'purchase_return', 'both')),
    CONSTRAINT ck_return_reasons_default_disposition CHECK (
        default_disposition IS NULL OR default_disposition IN ('restock', 'repack', 'reprocess', 'marketing_sample', 'dispose'))
);

-- Alasan awal BRD 5.11 (rusak saat kirim, kedaluwarsa, salah kirim, cacat produksi, customer berubah pikiran)
-- ditambah tarikan konsinyasi, reject QC ke pemasok/maklon, dan lainnya. created_by uuid nil = Sistem.
INSERT INTO return_reasons (code, name, applies_to, default_disposition, created_by, updated_by)
SELECT code, name, applies_to, default_disposition,
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('DAMAGED_SHIPPING', 'Rusak saat pengiriman', 'both', 'repack'),
    ('EXPIRED', 'Kedaluwarsa', 'sales_return', 'dispose'),
    ('WRONG_ITEM', 'Salah kirim produk/varian', 'both', 'restock'),
    ('PRODUCTION_DEFECT', 'Cacat produksi', 'both', 'reprocess'),
    ('CHANGED_MIND', 'Pelanggan berubah pikiran', 'sales_return', 'restock'),
    ('CONSIGNMENT_PULL', 'Tarikan stok konsinyasi', 'sales_return', 'restock'),
    ('QC_REJECT', 'Reject QC (dikembalikan ke pemasok/maklon)', 'purchase_return', NULL),
    ('OTHER', 'Lainnya', 'both', NULL)
) AS s (code, name, applies_to, default_disposition);

-- +goose Down
DROP TABLE return_reasons;
