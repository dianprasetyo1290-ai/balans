-- +goose Up
-- Master jenis biaya (MD-13, ACC-11; Desain Data Model bagian 9, Desain Finance & Accounting 4.3). Dipakai
-- pengajuan biaya/reimbursement (S-404) dan kas kecil (S-405): akun beban, jenis PPh yang dipotong, wajib
-- bukti, wajib kampanye (dimensi K, Desain Finance & Accounting 3.4), dan jenis dokumen approval.
-- Kode unik permanen: tidak dipakai ulang walau jenis biaya dihapus (Technical Guideline 5.13).
CREATE TABLE expense_types (
    id                 uuid        NOT NULL DEFAULT uuidv7(),
    code               text        NOT NULL,
    name               text        NOT NULL,
    expense_account_id uuid        NOT NULL,
    withholding_type   text        NOT NULL DEFAULT 'none',
    requires_receipt   boolean     NOT NULL DEFAULT true,
    requires_campaign  boolean     NOT NULL DEFAULT false,
    approval_subject   text        NOT NULL DEFAULT 'expense_claim',
    description        text        NOT NULL DEFAULT '',
    is_active          boolean     NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    created_by         uuid        NOT NULL,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    updated_by         uuid        NOT NULL,
    version            integer     NOT NULL DEFAULT 1,
    deleted_at         timestamptz,
    deleted_by         uuid,
    CONSTRAINT pk_expense_types PRIMARY KEY (id),
    CONSTRAINT uq_expense_types_code UNIQUE (code),
    CONSTRAINT fk_expense_types_expense_account_id FOREIGN KEY (expense_account_id) REFERENCES accounts (id),
    CONSTRAINT ck_expense_types_code CHECK (code ~ '^[A-Z][A-Z0-9_]{1,29}$'),
    CONSTRAINT ck_expense_types_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_expense_types_withholding_type CHECK (withholding_type IN ('none', 'pph21', 'pph23', 'pph4_2')),
    CONSTRAINT ck_expense_types_approval_subject CHECK (approval_subject IN ('expense_claim', 'petty_cash_replenishment'))
);
CREATE INDEX idx_expense_types_expense_account_id ON expense_types (expense_account_id);

-- Jenis biaya awal Desain Finance & Accounting 4.3. created_by uuid nil = Sistem. PPh "bila vendor badan usaha
-- jasa" (EVENT) disimpan sebagai PPh 23 dan diputuskan saat verifikasi; "ditentukan saat verifikasi" (OTHER)
-- disimpan tanpa potongan. REIMBURSE memakai Beban Umum Lainnya sampai akun dipilih per baris (S-404).
INSERT INTO expense_types (code, name, expense_account_id, withholding_type, requires_receipt, requires_campaign,
                           approval_subject, created_by, updated_by)
SELECT s.code, s.name, a.id, s.withholding_type, true, s.requires_campaign, 'expense_claim',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('ENDORSE_PERSON', 'Fee endorsement perorangan', '6101', 'pph21', true),
    ('ENDORSE_AGENCY', 'Fee endorsement agensi', '6101', 'pph23', true),
    ('EVENT', 'Biaya event', '6102', 'pph23', true),
    ('ADS', 'Iklan digital', '6104', 'none', true),
    ('RENT', 'Sewa bangunan', '6301', 'pph4_2', false),
    ('UTILITY', 'Utilitas', '6302', 'none', false),
    ('OFFICE_SUPPLY', 'Perlengkapan kantor', '6303', 'none', false),
    ('SOFTWARE', 'Langganan software', '6304', 'none', false),
    ('SHIPPING', 'Ongkos kirim penjualan manual', '6108', 'none', false),
    ('BANK_FEE', 'Biaya bank', '7202', 'none', false),
    ('REIMBURSE', 'Reimbursement karyawan', '6306', 'none', false),
    ('OTHER', 'Beban umum lainnya', '6306', 'none', false)
) AS s (code, name, account_code, withholding_type, requires_campaign)
JOIN accounts a ON a.code = s.account_code;

-- +goose Down
DROP TABLE expense_types;
