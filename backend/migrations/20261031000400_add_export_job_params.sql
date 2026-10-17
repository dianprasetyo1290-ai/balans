-- +goose Up
-- Parameter ekspor (mis. periode Neraca Saldo, rentang Buku Besar) untuk ekspor laporan sebagai job (S-410, S-413).
ALTER TABLE export_jobs ADD COLUMN params jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE export_jobs DROP COLUMN params;
