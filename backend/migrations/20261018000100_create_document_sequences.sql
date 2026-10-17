-- +goose Up
-- Penomoran dokumen KODE/TAHUN/BULAN/NOMOR (U-15, Technical Guideline 5.12, Desain Penomoran bagian 1).
-- Nomor urut reset setiap tahun; baris diambil dengan UPDATE … RETURNING di dalam transaksi bisnis,
-- sehingga nomor tidak loncat jika transaksi gagal (baris terkunci sampai commit/rollback).
CREATE TABLE document_sequences (
    document_type text        NOT NULL,
    year          integer     NOT NULL,
    last_number   bigint      NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_document_sequences PRIMARY KEY (document_type, year),
    CONSTRAINT ck_document_sequences_document_type CHECK (document_type ~ '^[A-Z]{2,4}$'),
    CONSTRAINT ck_document_sequences_year CHECK (year BETWEEN 2000 AND 9999),
    CONSTRAINT ck_document_sequences_last_number CHECK (last_number >= 0)
);

-- +goose Down
DROP TABLE document_sequences;
