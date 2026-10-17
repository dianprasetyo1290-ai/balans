-- +goose Up
-- COA final F0-FIN-01 (Desain Finance & Accounting bagian 1) dan dimensi awal (bagian 3).
-- created_by uuid nil = Sistem. Kolom Manual "—" = akun induk (judul), tidak bisa diposting.
INSERT INTO accounts (code, name, account_group, normal_balance, is_postable, allow_manual_posting, created_by, updated_by)
SELECT code, name, left(code, 1)::smallint, normal_balance, is_postable, allow_manual,
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('1000', 'ASET', 'debit', false, false),
    ('1100', 'Kas & Bank', 'debit', false, false),
    ('1101', 'Kas Besar', 'debit', true, true),
    ('1102', 'Kas Kecil', 'debit', true, false),
    ('1111', 'Bank — rekening operasional 1', 'debit', true, true),
    ('1112', 'Bank — rekening operasional 2', 'debit', true, true),
    ('1200', 'Piutang', 'debit', false, false),
    ('1201', 'Piutang Usaha', 'debit', true, false),
    ('1202', 'Piutang Midtrans', 'debit', true, false),
    ('1211', 'Piutang Marketplace — Shopee', 'debit', true, false),
    ('1212', 'Piutang Marketplace — TikTok Shop & Tokopedia', 'debit', true, false),
    ('1213', 'Piutang Marketplace — Lazada', 'debit', true, false),
    ('1214', 'Piutang Marketplace — Blibli', 'debit', true, false),
    ('1221', 'Piutang Karyawan', 'debit', true, true),
    ('1300', 'Persediaan', 'debit', false, false),
    ('1301', 'Persediaan Packaging', 'debit', true, false),
    ('1302', 'Persediaan Bahan Reprocess', 'debit', true, false),
    ('1311', 'Persediaan FG — Gudang', 'debit', true, false),
    ('1312', 'Persediaan FG — Konsinyasi', 'debit', true, false),
    ('1313', 'Persediaan FG — Vending', 'debit', true, false),
    ('1314', 'Persediaan Retur', 'debit', true, false),
    ('1400', 'Uang Muka & Dibayar di Muka', 'debit', false, false),
    ('1401', 'Uang Muka Pembelian', 'debit', true, false),
    ('1402', 'Deposit Biteship', 'debit', true, false),
    ('1403', 'Biaya Dibayar di Muka', 'debit', true, true),
    ('1500', 'Aset Tetap', 'debit', false, false),
    ('1501', 'Aset Tetap — Mesin Vending', 'debit', true, false),
    ('1502', 'Aset Tetap — Peralatan Kantor & Komputer', 'debit', true, false),
    ('1503', 'Aset Tetap — Kendaraan', 'debit', true, false),
    ('1591', 'Akumulasi Penyusutan — Mesin Vending', 'credit', true, false),
    ('1592', 'Akumulasi Penyusutan — Peralatan Kantor & Komputer', 'credit', true, false),
    ('1593', 'Akumulasi Penyusutan — Kendaraan', 'credit', true, false),
    ('2000', 'LIABILITAS', 'credit', false, false),
    ('2101', 'Utang Usaha', 'credit', true, false),
    ('2102', 'Barang Diterima Belum Ditagih', 'credit', true, false),
    ('2103', 'Utang Biaya', 'credit', true, false),
    ('2201', 'Utang Gaji', 'credit', true, false),
    ('2202', 'Utang BPJS', 'credit', true, false),
    ('2301', 'Utang PPh 21', 'credit', true, false),
    ('2302', 'Utang PPh 23', 'credit', true, false),
    ('2303', 'Utang PPh 4(2)', 'credit', true, false),
    ('2304', 'Utang PPh Final', 'credit', true, false),
    ('2401', 'Deposit Reseller', 'credit', true, false),
    ('3000', 'EKUITAS', 'credit', false, false),
    ('3101', 'Modal', 'credit', true, true),
    ('3201', 'Laba Ditahan', 'credit', true, true),
    ('3202', 'Laba Tahun Berjalan', 'credit', true, false),
    ('3901', 'Saldo Awal Ekuitas', 'credit', true, false),
    ('4000', 'PENDAPATAN', 'credit', false, false),
    ('4101', 'Penjualan', 'credit', true, false),
    ('4102', 'Potongan Penjualan', 'debit', true, false),
    ('4103', 'Retur Penjualan', 'debit', true, false),
    ('4201', 'Pendapatan Ongkos Kirim', 'credit', true, false),
    ('5000', 'HARGA POKOK PENJUALAN', 'debit', false, false),
    ('5101', 'HPP Barang Jadi', 'debit', true, false),
    ('5102', 'HPP — Reject & Waste', 'debit', true, false),
    ('5103', 'Selisih Harga Pembelian', 'debit', true, false),
    ('6000', 'BEBAN OPERASIONAL', 'debit', false, false),
    ('6101', 'Beban Marketing — Endorsement', 'debit', true, true),
    ('6102', 'Beban Marketing — Event', 'debit', true, true),
    ('6103', 'Beban Marketing — Sampel & Promosi', 'debit', true, true),
    ('6104', 'Beban Marketing — Iklan Digital', 'debit', true, true),
    ('6105', 'Beban Fee Marketplace', 'debit', true, false),
    ('6106', 'Beban Fee Payment Gateway', 'debit', true, false),
    ('6107', 'Beban Promosi Marketplace', 'debit', true, false),
    ('6108', 'Beban Pengiriman Penjualan', 'debit', true, true),
    ('6109', 'Beban Biaya Lokasi Vending', 'debit', true, true),
    ('6201', 'Beban Gaji & Tunjangan', 'debit', true, false),
    ('6202', 'Beban BPJS', 'debit', true, false),
    ('6203', 'Beban Karyawan — Sampel Produk', 'debit', true, false),
    ('6301', 'Beban Sewa', 'debit', true, true),
    ('6302', 'Beban Utilitas', 'debit', true, true),
    ('6303', 'Beban Perlengkapan Kantor', 'debit', true, true),
    ('6304', 'Beban Langganan Software', 'debit', true, true),
    ('6305', 'Beban Penyusutan', 'debit', true, false),
    ('6306', 'Beban Umum Lainnya', 'debit', true, true),
    ('6401', 'Beban Selisih Persediaan', 'debit', true, false),
    ('6402', 'Beban Persediaan Kedaluwarsa & Rusak', 'debit', true, false),
    ('6403', 'Beban Pemusnahan Persediaan', 'debit', true, false),
    ('6404', 'Beban Kehilangan Persediaan', 'debit', true, false),
    ('7000', 'PENDAPATAN & BEBAN LAIN-LAIN', 'debit', false, false),
    ('7101', 'Pendapatan Lain-lain', 'credit', true, true),
    ('7201', 'Beban Lain-lain', 'debit', true, true),
    ('7202', 'Beban Bank', 'debit', true, true),
    ('7203', 'Beban PPh Final', 'debit', true, false),
    ('7301', 'Selisih Pembulatan', 'debit', true, false)
) AS s (code, name, normal_balance, is_postable, allow_manual);

-- Induk: judul kelompok-2-digit (mis. 1100) bila ada, selain itu judul kelompok (mis. 2000).
UPDATE accounts c SET parent_id = coalesce(
    (SELECT p.id FROM accounts p WHERE p.code = left(c.code, 2) || '00' AND NOT p.is_postable AND p.code <> c.code),
    (SELECT p.id FROM accounts p WHERE p.code = left(c.code, 1) || '000' AND p.code <> c.code))
WHERE c.code !~ '000$';

INSERT INTO channels (code, name, created_by, updated_by) VALUES
    ('modern_trade', 'Modern Trade', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('consignment', 'Konsinyasi', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('marketplace', 'Marketplace', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('social_commerce', 'WhatsApp / Instagram', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('website', 'Website', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('reseller', 'Reseller', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('vending', 'Vending', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('b2b', 'B2B / Corporate', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('employee', 'Penjualan Karyawan', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('internal', 'Internal (non-penjualan)', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

INSERT INTO sub_channels (channel_id, code, name, created_by, updated_by)
SELECT c.id, s.code, s.name, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('marketplace', 'shopee', 'Shopee'),
    ('marketplace', 'tiktok_tokopedia', 'TikTok Shop & Tokopedia'),
    ('marketplace', 'lazada', 'Lazada'),
    ('marketplace', 'blibli', 'Blibli'),
    ('social_commerce', 'whatsapp', 'WhatsApp'),
    ('social_commerce', 'instagram', 'Instagram'),
    ('website', 'findyourbalans', 'findyourbalans.com')
) AS s (channel_code, code, name)
JOIN channels c ON c.code = s.channel_code;

INSERT INTO divisions (code, name, created_by, updated_by) VALUES
    ('LOG', 'Logistik', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('FAT', 'Finance & Accounting', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('SLS', 'Sales', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('MKT', 'Marketing', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('HRGA', 'HRGA', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('MGT', 'Manajemen / Direksi', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DELETE FROM sub_channels;
DELETE FROM channels;
DELETE FROM divisions;
UPDATE accounts SET parent_id = NULL;
DELETE FROM accounts;
