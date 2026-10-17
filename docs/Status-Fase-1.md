# Status Fase 1
## Balans Nutrition — Unified Business Platform: Core Operations & Finance

| Item | Keterangan |
|---|---|
| Dokumen | Pelacak pengerjaan story Backlog Fase 1 terhadap PRD Fase 1 |
| Acuan | PRD Fase 1 v0.1 bagian 6; Backlog Fase 1 |
| Diperbarui | 17 Oktober 2026 |

**Status:** **Selesai** = kode, test, dan kriteria penerimaan backlog terpenuhi di lokal. **Sebagian** = sebagian kriteria terpenuhi; sisanya disebut. **Belum** = belum dikerjakan.

---

## R1.1 — Fondasi & Master Data

| Story | Fitur PRD | Status | Catatan |
|---|---|---|---|
| S-101 Login | F1-101 | Selesai | JWT EdDSA 15 menit + refresh token berotasi di cookie HttpOnly; deteksi pemakaian ulang mencabut keluarga sesi; 5× gagal → kunci 15 menit; logout mencabut sesi; halaman Masuk, Lupa password, Atur password |
| S-102 User & role | F1-101 | Selesai | CRUD pengguna, undangan atur password via email, nonaktif/ganti role menaikkan `token_version` dan memutus sesi (cache 30 detik, langsung di proses yang sama); halaman Pengaturan › Pengguna & Role |
| S-103 Permission per role | F1-102 | Selesai | Katalog permission disemai dari matriks Desain Role & Approval; Admin mengubah lewat Pengaturan › Hak Akses; menu tersembunyi sesuai permission; API menolak 403 |
| S-104 MFA | F1-102 | Selesai | TOTP wajib untuk role ber-MFA dengan QR enrol; re-autentikasi 5 menit untuk aksi ★ dibuka otomatis oleh dialog konfirmasi identitas |
| S-105 Audit trail | F1-103 | Selesai | Modul audit, diff + masking, append-only di DB, halaman Riwayat Perubahan; login/gagal login/logout/user/role/master tercatat. Komponen ERP `EntityHistory` (tab **Riwayat**) membaca `GET /api/v1/audit-logs?entityType=&entityId=` dan dipakai di detail produk dan gudang; modul mendaftarkan permission baca entitas lewat `audit.Module.RegisterEntity` sehingga pemegang hak baca data cukup untuk melihat riwayatnya. Halaman detail dokumen berikutnya memakai komponen yang sama |
| S-106 Soft delete | F1-103 | Selesai | Pola `platform/softdelete` (Playbook 2): hapus = `deleted_at/deleted_by`, tidak tampil di list/pencarian/dropdown, tetap terbaca lewat ID untuk dokumen lama, filter *Dihapus* di list, **Pulihkan** hanya Admin Sistem; hapus & pulihkan tercatat di audit trail. Dipakai produk, kategori, gudang (lokasi ikut terhapus/dipulihkan), dan lokasi; master berikutnya memakai pola yang sama |
| S-107 Responsif | F1-104 | Sebagian | Layout & halaman baru diuji Playwright desktop + HP |
| S-108 Lokalisasi | F1-104 | Selesai (Fase 0) | Formatter Rupiah/WIB, pesan Indonesia |
| S-109 Privasi data | F1-105 | Sebagian | `platform/pii` AES-256-GCM + HMAC dengan rotasi kunci; dipakai secret MFA dan hash email login. Kolom NIK/NPWP/rekening menyusul di master terkait |
| S-110 Notifikasi approver | F1-106 | Selesai | Notifikasi in-app (lonceng di header) + email saat menjadi approver dan saat dokumen diputuskan; job `approval.remind` tiap jam, hanya Senin–Jumat 08.00–17.00 WIB; badge jumlah di menu *Persetujuan Saya* |
| S-111 Inbox persetujuan | F1-106, BRD 5.17 | Selesai | Halaman *Persetujuan Saya*: Menunggu Saya (lintas jenis dokumen, tampilan kartu di HP), Diajukan Saya, Riwayat Keputusan, Delegasi; tolak wajib catatan; delegasi saat cuti maks. 90 hari, tanpa tumpang tindih |
| S-112 Engine approval | F1-106, BRD 5.17 | Selesai | Modul `approval`: kebijakan effective-dated per jenis/nominal/divisi/syarat, matriks bagian 4 disemai; level berurutan; snapshot langkah saat submit; pembuat tidak pernah menyetujui dokumennya sendiri (level naik ke pengganti); handler modul dokumen dipanggil di transaksi yang sama; keputusan tercatat di audit trail dokumen. Pengaturan › *Matriks Persetujuan* untuk tambah/ubah/hentikan level. Modul dokumen pertama yang memakai engine: PR/PO (R1.2) |
| S-113 Produk & label barcode | F1-107 | Selesai | Modul `catalog`: field MD-01 (SKU unik permanen termasuk setelah dihapus, nama, varian, tipe kemasan, satuan, umur simpan wajib untuk Produk Jadi, isi per karton, kategori, stok minimum, aktif); banyak barcode dengan tepat satu utama dan unik lintas produk (EAN-13 bila 13 digit valid, selain itu Code 128); list dengan filter jenis/status & cari SKU/nama/varian/barcode; detail dengan tab Riwayat. *Logistik › Label Produk*: label 50 × 30 mm ke printer thermal (satu label per halaman) atau A4 (3 × 8 per halaman), jumlah per produk bisa diubah, batch & tanggal kedaluwarsa opsional; barcode dirender SVG di browser tanpa dependensi baru. Kategori dikelola dari dialog *Kategori* di halaman Produk |
| S-114 Packaging & material | F1-108 | Selesai | Halaman *Data Master › Packaging & Material* untuk Packaging (RM, tipe kemasan wajib) dan Material (GM); lacak per batch opsional; umur simpan opsional |
| S-115 Bundel virtual | F1-108 | Sebagian | Halaman *Data Master › Bundling*: komponen Produk Jadi aktif non-bundel + qty (3 desimal), bundel tanpa stok/batch/umur simpan; komponen bundel aktif tidak bisa dihapus. API Go `catalog.AvailableBundleQty` = min(floor(stok komponen ÷ qty)) dan `catalog.ExplodeBundle` untuk pemecahan ke komponen (TG 5.7.4) dengan test. Tampilan stok tersedia bundel menunggu modul `inventory` (R1.3) |
| S-116 – S-122 | F1-109 – F1-113 | Belum | |
| S-123 Gudang & lokasi | F1-114 | Selesai | Modul `warehouse` (master): gudang baru otomatis membuat 6 lokasi standar `<KODE>-INB/FG/ECM/RM/GM/RET` (Inbound, FG Non-Ecommerce, E-commerce Stock (Petty Stock), Gudang GM – RM, Gudang GM – GM, Gudang Retur) dalam satu transaksi; lokasi tambahan (mis. Event); lokasi eksternal maklon/konsinyasi/buffer vending/mesin per pihak (`external_party_type` + `external_party_id` tanpa FK ke modul party), satu lokasi per jenis per pihak; *Data Master › Gudang & Lokasi* dengan tab Gudang dan Lokasi Eksternal, detail gudang dengan tab Lokasi dan Riwayat. API Go: `Location`, `Locations`, `FindLocations`, `ExternalLocation`, `Warehouse` |
| S-124 – S-125 | F1-115 – F1-116 | Belum | |
| S-126 COA | F1-117 | Selesai | Modul `accounting`: COA final Desain Finance & Accounting bagian 1 disemai (86 akun, induk per kelompok); kode 4 digit unik permanen; akun induk tidak bisa diposting (`is_postable`); flag *Boleh dipakai di Jurnal Umum* (akun kontrol hanya otomatis); akun terpakai jurnal/posting rule atau yang punya sub-akun tidak bisa dihapus; saldo normal akun terpakai dikunci. Halaman Keuangan & Akuntansi › Chart of Accounts (pohon, tambah/ubah/hapus, sub-akun) |
| S-127 Jurnal Umum & Buku Besar | F1-117 | Selesai | Draft Jurnal Umum → Posting Jurnal: balance, periode terbuka (`PERIOD_LOCKED`), dimensi wajib per kelompok akun, nomor `JV/TAHUN/BULAN/NOMOR` saat posting. Buku Besar per akun dengan saldo awal, mutasi, saldo berjalan sesuai saldo normal, filter divisi/channel. Halaman Jurnal Umum (daftar/form/detail) dan Buku Besar |
| S-128 Jurnal immutable & Balik Jurnal | F1-117 | Selesai | Trigger DB menolak UPDATE/DELETE jurnal terposting dan insert/update/delete barisnya, serta memeriksa ulang balance saat posting; `ck_journal_lines_debit_xor_credit`. *Balik Jurnal* membuat JV pembalik (debit/kredit ditukar, dimensi sama) yang mereferensikan jurnal asal; satu jurnal hanya sekali dibalik (`uq_journal_entries_source`). `Reverse` untuk jurnal otomatis saat dokumen sumber dibatalkan |
| S-129 Dimensi jurnal | F1-118 | Selesai | Setiap `journal_lines` membawa channel, sub-channel, divisi, kampanye (opsional); master disemai dari Desain Finance & Accounting bagian 3 (10 channel, 7 sub-channel, 6 divisi); dimensi wajib per posting rule; validasi aktif, sub-channel milik channel, kampanye berlaku pada tanggal transaksi. Halaman Data Master › Kampanye dan Data Master › Channel & Divisi |
| S-131 – S-132 | F1-119 | Belum | |
| S-133 Penomoran & periode | F1-120 | Selesai | Modul `document`: `document_sequences (document_type, year, last_number)`, `Next` di transaksi pemanggil (upsert `UPDATE … RETURNING`, rollback tidak membuat nomor loncat, ditolak di luar transaksi); kode TG 5.12 + kode tambahan Desain Penomoran; Pengaturan › Nomor Dokumen. Periode akuntansi bulanan dibuat job harian `accounting.ensure_fiscal_periods` (00.05 WIB, juga saat worker mulai), tombol Buat Periode, dan otomatis saat posting; status Terbuka/Ditutup; halaman Keuangan & Akuntansi › Periode Akuntansi. Tutup/buka kembali periode di S-411 |
| S-134 | F1-112 | Belum | |

## Pengembangan Lokal

- `cmd/devseed` membuat satu akun per role dengan password sama (`Balans-Lokal-2026`), hanya di `APP_ENV=local`.
- `cmd/devseed` juga mengajukan 5 dokumen contoh (PO, pengajuan biaya, disposisi retur, daftar harga) agar inbox bisa didemokan sebelum modul dokumen ada; handler contoh hanya dipasang di `APP_ENV=local`.
- `cmd/devseed` juga membuat *Gudang Utama* (`UTAMA`) beserta lokasi standarnya dan produk contoh (2 Produk Jadi, 1 Packaging, 1 Material, 1 bundel) bila modul `catalog`/`warehouse` aktif.
- `DEV_SKIP_MFA=true` (hanya `APP_ENV=local`) melewati kode autentikator agar demo lokal tidak memerlukan HP.

## Asumsi yang Diambil

| # | Asumsi | Alasan |
|---|---|---|
| 1 | Label menu audit trail: **Riwayat Perubahan** di grup Pengaturan | Naming Convention belum memuat label untuk NFR-03; dimasukkan ke uji label F0-UX-05 |
| 2 | Kode error baru `AUTH_MFA_ENROLLMENT_REQUIRED` (403) dan `AUTH_PASSWORD_TOKEN_INVALID` (422) | Dibutuhkan alur aktivasi MFA wajib dan tautan password; sudah ditambahkan ke Technical Guideline Lampiran A |
| 3 | Masa berlaku tautan undangan 72 jam, atur ulang password 1 jam | Tidak ditetapkan BRD/TG; nilai umum yang aman |
| 4 | Refresh dari dua tab bersamaan dalam 10 detik tidak dianggap pencurian token | Mencegah user terlempar keluar saat membuka banyak tab |
| 5 | Admin Sistem tidak bisa mencabut role Admin atau hak mengatur role dari dirinya/role Admin | Mencegah sistem terkunci tanpa admin |
| 6 | Jam kerja pengingat approval: Senin–Jumat 08.00–17.00 WIB, belum memperhitungkan hari libur nasional | NFR-11 menyebut "jam kerja" tanpa rincian; dipindah ke modul `setting` saat modul itu dibangun |
| 7 | Menu *Persetujuan Saya* di grup Ringkasan | Naming Convention hanya memuat *Persetujuan Pengadaan* (Pengadaan); inbox lintas dokumen butuh tempat yang terlihat semua approver. Masuk uji label F0-UX-05 |
| 8 | Perubahan Matriks Persetujuan berlaku langsung oleh Admin Sistem (tercatat di audit trail), belum melalui approval Direksi | Matriks role memberi Direksi `approval.policy.approve`, tetapi alur persetujuan perubahan matriks belum didefinisikan; diusulkan dibahas di sprint review |
| 9 | Delegasi maksimal 90 hari per entri dan satu delegasi per user per tanggal | Mencegah delegasi tanpa batas waktu; cuti lebih panjang dicatat sebagai beberapa entri |
| 10 | Level 1 rekonsiliasi konsinyasi memakai role Sales MT; pembatasan ke PIC partner menunggu master mitra (S-121) | Kebijakan bisa menunjuk user khusus per level sebagai solusi sementara |
| 11 | Dokumen tidak bisa diajukan jika ada level tanpa approver aktif selain pembuat (`APPROVAL_POLICY_MISSING`) | Mencegah dokumen tertahan tanpa ada yang bisa menyetujui |
| 12 | Dimensi wajib Jurnal Umum manual per kelompok akun: pendapatan (4xxx) wajib channel + sub-channel; HPP, beban, lain-lain (5xxx–7xxx) wajib divisi; akun neraca tanpa dimensi wajib | Desain Finance & Accounting 3.4 menetapkan aturan untuk jurnal penjualan & biaya; Jurnal Umum tidak punya posting rule |
| 13 | Balance dan dimensi wajib Jurnal Umum diperiksa saat posting, bukan saat simpan draft; akun dan periode diperiksa saat simpan | Draft bisa disusun bertahap |
| 14 | Balik Jurnal dari halaman jurnal hanya untuk Jurnal Umum manual; jurnal otomatis dibalik lewat pembatalan dokumen sumber (`Reverse`); jurnal pembalik tidak bisa dibalik lagi | TG 5.8.3: pembatalan dokumen operasional membalik stok dan jurnal dalam satu transaksi |
| 15 | Semua jurnal (manual, otomatis, pembalik) memakai nomor JV | Desain Penomoran: JV = Jurnal, diberikan saat posting |
| 16 | Periode yang belum ada dibuat otomatis saat posting, kecuali sudah ada periode tertutup setelahnya (`PERIOD_LOCKED`) | Mencegah backdate ke bulan tanpa periode di tahun yang sudah ditutup |
| 17 | Pemetaan posting rule memakai *resolver* akun (mis. `location_inventory_account`, `receivable_account`) yang dipilih dokumen sumber, dengan akun default; nilai negatif membalik sisi (mis. selisih stock count lebih); A-36 memakai `Splits` per akun | Desain Finance & Accounting 2.1 memuat akun alternatif per rule (1301/1311, 1201/1111, 121x) |
| 18 | Label menu baru: *Channel & Divisi* (Data Master) dan *Periode Akuntansi* (Keuangan & Akuntansi) | Naming Convention belum memuat halaman master dimensi dan daftar periode; masuk uji label F0-UX-05 |
| 19 | Sub-channel MT/konsinyasi/B2B per partner dibuat manual dulu; tautan otomatis ke master mitra (`party_type/party_id`) saat modul `party` dibangun | Master mitra (S-121) belum ada |
| 20 | Kampanye dihapus/nonaktif tidak bisa dipakai jurnal baru; periode kampanye inklusif tanggal mulai–selesai | MD-13 |
| 21 | E-commerce Stock dan Petty Stock adalah satu lokasi (`ecommerce`), sehingga lokasi standar gudang ada 6: Inbound, FG Non-Ecommerce, E-commerce Stock (Petty Stock), RM, GM, Retur. Kode lokasi = kode gudang + akhiran (`UTAMA-INB`) | Glosarium BRD menyamakan keduanya dan Data Model 6.1 memetakan `ecommerce` = Petty Stock; PRD F1-114 menyebutnya terpisah |
| 22 | Pemulihan data terhapus membutuhkan role Admin Sistem **dan** permission tulis resource; hapus cukup permission tulis | TG 5.13 "Restore hanya oleh Admin Sistem"; matriks belum punya aksi `restore` |
| 23 | Hak ubah master produk dan gudang mengikuti matriks Desain Role & Approval 3.1 apa adanya: produk hanya Admin Sistem (`catalog.product.write`), gudang/lokasi Admin Sistem dan Head Logistik | Backlog menulis "Sebagai Logistik/Sales"; bila Logistik/Sales perlu mengubah produk atau bundel, Admin memberi hak lewat *Pengaturan › Hak Akses*. Diusulkan dibahas di sprint review |
| 24 | Label produk dirender di browser (SVG + CSS cetak `@page`), bukan PDF server; isi batch & kedaluwarsa diketik manual di halaman label | Desain Dokumen Cetak 2.3 menyebut barcode dirender server; modul `document`/template belum dibangun. Label dari Penerimaan Barang/Outbound mengisi batch otomatis saat modul itu dibangun |
| 25 | Menu *Label Produk* di grup Logistik (Naming Convention 5), tampil jika modul `catalog` aktif dan user memegang `logistics.label.post` atau `logistics.label.read` | Desain UI/UX bagian 2 belum memuat Label Produk di Logistik Fase 1, tetapi S-113 butuh tempat cetak dari master produk |
| 26 | Barcode 13 digit dianggap EAN-13 dan wajib digit periksa benar; selain itu Code 128 (ASCII tercetak, maks. 48 karakter). Satu-satunya barcode otomatis menjadi barcode utama | Desain Dokumen Cetak 2.3; mencegah label EAN salah cetak |
| 27 | Produk Jadi non-bundel selalu dilacak per batch dan wajib umur simpan serta tipe kemasan; Packaging wajib tipe kemasan; batch Packaging & Material opsional; bundel tanpa batch, umur simpan, dan stok minimum | MD-01 – MD-03, INV-04 (FEFO) |
| 28 | Produk yang masih menjadi komponen bundel aktif tidak bisa dihapus; bundel tidak bisa dipulihkan sebelum komponennya dipulihkan. Lokasi standar tidak bisa dihapus sendiri (nonaktifkan atau hapus gudangnya); hapus gudang ikut menghapus lokasinya dan pulihkan gudang memulihkan lokasi yang terhapus bersamanya | Menjaga bundel dan mutasi stok tetap konsisten |
| 29 | Lokasi eksternal menyimpan ID pihak (UUID) yang diketik manual sampai master Mitra/Pemasok (S-121, S-122) menyediakan pemilih; jenis pihak: maklon = pemasok, konsinyasi = mitra, buffer vending = mitra/mesin, mesin = mesin vending | Modul `party` dibangun paralel; tabel pihak bukan milik modul `warehouse` sehingga tanpa FK |
| 30 | Kategori produk dikelola lewat dialog di halaman Produk, bukan menu tersendiri | Naming Convention tidak memuat menu Kategori di Data Master |
| 31 | Audit trail tidak mencatat field yang bernilai kosong di kedua sisi (mis. field null saat create) | Tab Riwayat lebih ringkas; perubahan nyata tetap tercatat |
