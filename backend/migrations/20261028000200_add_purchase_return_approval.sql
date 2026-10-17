-- +goose Up
-- Retur pembelian manual (bukan asal QC) butuh persetujuan Head Logistik sebelum diposting (Desain Status Dokumen
-- 2.5; matriks 3.2 `procurement.purchase_return` A = Head Logistik). Retur asal QC diposting tanpa approval (D-24).
INSERT INTO approval_policies (subject_type, level, min_amount, condition, approver_role_code, valid_from, created_by, updated_by)
VALUES ('purchase_return', 1, NULL, '', 'head_logistics', '2026-01-01T00:00:00+07:00',
        '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DELETE FROM approval_policies WHERE subject_type = 'purchase_return';
