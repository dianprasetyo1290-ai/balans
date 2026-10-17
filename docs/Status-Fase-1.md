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
| S-105 Audit trail | F1-103 | Sebagian | Modul audit, diff + masking, append-only di DB, halaman Riwayat Perubahan; login/gagal login/logout/user/role tercatat. Tab Riwayat di dokumen dibangun bersama halaman detail dokumen |
| S-106 Soft delete | F1-103 | Belum | Pola dibangun bersama master data pertama |
| S-107 Responsif | F1-104 | Sebagian | Layout & halaman baru diuji Playwright desktop + HP |
| S-108 Lokalisasi | F1-104 | Selesai (Fase 0) | Formatter Rupiah/WIB, pesan Indonesia |
| S-109 Privasi data | F1-105 | Selesai | `platform/pii` AES-256-GCM + HMAC dengan rotasi kunci; dipakai secret MFA, hash email login, NIK karyawan (+ `nik_hash` untuk cari persis), NPWP mitra/pemasok, dan nomor rekening pemasok. API dan audit trail hanya menampilkan nilai tersamar (4 digit terakhir). Persetujuan pemasaran pelanggan tersimpan dengan waktu dan versi teks; dicatat ulang saat versi berubah dan dihapus saat ditarik. Access log hanya mencatat path (tanpa query). Komponen gaji menyusul payroll (Fase 5) |
| S-110 Notifikasi approver | F1-106 | Selesai | Notifikasi in-app (lonceng di header) + email saat menjadi approver dan saat dokumen diputuskan; job `approval.remind` tiap jam, hanya Senin–Jumat 08.00–17.00 WIB; badge jumlah di menu *Persetujuan Saya* |
| S-111 Inbox persetujuan | F1-106, BRD 5.17 | Selesai | Halaman *Persetujuan Saya*: Menunggu Saya (lintas jenis dokumen, tampilan kartu di HP), Diajukan Saya, Riwayat Keputusan, Delegasi; tolak wajib catatan; delegasi saat cuti maks. 90 hari, tanpa tumpang tindih |
| S-112 Engine approval | F1-106, BRD 5.17 | Selesai | Modul `approval`: kebijakan effective-dated per jenis/nominal/divisi/syarat, matriks bagian 4 disemai; level berurutan; snapshot langkah saat submit; pembuat tidak pernah menyetujui dokumennya sendiri (level naik ke pengganti); handler modul dokumen dipanggil di transaksi yang sama; keputusan tercatat di audit trail dokumen. Pengaturan › *Matriks Persetujuan* untuk tambah/ubah/hentikan level. Modul dokumen pertama yang memakai engine: PR/PO (R1.2) |
| S-120 Pelanggan | F1-111 | Selesai | Modul `party`: Data Master › *Pelanggan*. Kode otomatis `PLG-000001`; HP `08…`/`628…`/`+628…` dinormalisasi ke `+628…` dan unik di antara pelanggan aktif (pesan menyebut pelanggan pemilik nomor); cari nama/kode/HP format apa pun; multi alamat dengan satu alamat utama dan koordinat opsional; tipe Retail/B2B/Reseller/Karyawan; soft delete (nomor HP boleh dipakai lagi). API publik `CustomerByID`, `CustomerByPhone` |
| S-121 Mitra & PKS | F1-112 | Sebagian | Data Master › *Mitra Konsinyasi*: mitra MT/konsinyasi/B2B, kode permanen, NPWP terenkripsi, Sales MT & Merchandiser per mitra (`PartnersForMerchandiser`). PKS effective-dated (`EXCLUDE` periode tumpang tindih per jenis) dengan PDF (modul `filestore`), masa berlaku, margin konsinyasi, termin, dan ketentuan selisih (default BRD U-02). `RequireActiveAgreement`/`ActiveAgreement` siap dipakai DO konsinyasi; pemblokiran berjalan saat modul DO memanggilnya. Harga konsinyasi per SKU lewat `price_list_id` saat modul pricing ada |
| S-122 Pemasok & maklon | F1-113 | Selesai | Data Master › *Pemasok & Maklon*: jenis pemasok/maklon, badan usaha/perorangan, PPh default (tanpa/PPh 23/PPh 21/PPh 4(2)), termin, NPWP dan rekening terenkripsi (nomor lama dipertahankan bila tidak diisi ulang), tersamar di API & audit. API publik `SupplierByID` |
| S-131 Data karyawan dasar | F1-119 | Selesai | Modul `hris`: SDM › *Data Karyawan* (juga Data Master › *Karyawan*). Nomor karyawan permanen, nama, jabatan, divisi (`division_id`), tanggal bergabung, email kantor, HP, NIK terenkripsi + cari NIK persis via HMAC; resign tercatat dengan tanggal & alasan (boleh terjadwal) |
| S-132 Pelanggan ↔ karyawan | F1-119 | Selesai | Pelanggan bertipe Karyawan wajib tertaut ke karyawan aktif (satu karyawan satu pelanggan). `hris.IsActiveEmployee` dan `party.EmployeePriceEligible(customerID, tanggal)` → false mulai tanggal resign, tanpa job. CS memilih karyawan lewat `/employee-options` (tanpa data pribadi). Penerapan harga karyawan di pricing/SO menyusul F1-306 |
| S-134 Kode produk mitra | F1-112 | Sebagian | Data Master › *Kode Produk Mitra*: kode artikel & barcode retailer per SKU per mitra, unik per mitra–produk dan kode artikel unik per mitra; `ProductCodeFor` untuk label/DO. Merchandiser per mitra di S-121. Impor Excel menyusul bersama modul `dataexchange`; nama produk tampil setelah modul `catalog` diwiring (`party.Products`) |
| S-113 – S-134 lainnya | F1-107 – F1-120 | Belum | |

## Pengembangan Lokal

- `cmd/devseed` membuat satu akun per role dengan password sama (`Balans-Lokal-2026`), hanya di `APP_ENV=local`.
- `cmd/devseed` juga mengajukan 5 dokumen contoh (PO, pengajuan biaya, disposisi retur, daftar harga) agar inbox bisa didemokan sebelum modul dokumen ada; handler contoh hanya dipasang di `APP_ENV=local`.
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
| 12 | File (PDF PKS) disimpan di disk lokal `FILESTORE_DIR` (volume `filestore-data` di container api) dengan URL upload/unduh bertanda tangan HMAC (15/5 menit) di `/api/v1/file-content/{id}`; adapter object storage S3 menyusul tanpa mengubah API | Belum ada bucket object storage; pola URL sama dengan presigned URL (TG 5.16). Volume file perlu ikut backup harian (Runbook) |
| 13 | Batas file PKS: PDF, maksimal 10 MB; isi file diperiksa (signature PDF) dan ukuran harus sama dengan yang dideklarasikan | Batas teknis upload; dokumen PKS hasil scan umumnya < 10 MB |
| 14 | Kode pelanggan dibuat otomatis `PLG-` + 6 digit; kode mitra dan pemasok diisi user (huruf besar, angka, minus) dan permanen | Pelanggan dibuat cepat oleh CS; kode mitra/pemasok mengikuti kode yang sudah dipakai tim |
| 15 | Nomor HP yang diterima hanya Indonesia (+62), nomor nasional 8 – 12 digit, termasuk telepon kantor (021…) | BRD 5.8 hanya menyebut nomor WA/HP pelanggan Indonesia |
| 16 | `resign_date` = tanggal efektif berhenti: karyawan aktif sampai sehari sebelumnya; harga karyawan hilang mulai tanggal itu | HR-03 tidak merinci hari terakhir; dipilih agar resign hari ini langsung berlaku |
| 17 | Satu karyawan hanya tertaut ke satu pelanggan aktif; pelanggan bertipe Karyawan wajib tertaut ke karyawan aktif | Mencegah harga karyawan dipakai beberapa akun |
| 18 | Jenis mitra ditambah `b2b`; harga konsinyasi per SKU dirujuk lewat `price_list_id` (modul pricing) dan PKS menyimpan margin mitra (%) | Data model hanya `modern_trade`/`consignment`; tugas memasukkan B2B. Harga per SKU dikelola Daftar Harga |
| 19 | Perubahan ketentuan PKS dicatat sebagai PKS baru setelah PKS lama dipersingkat; ubah PKS hanya untuk koreksi (tercatat di audit trail) | Effective-dated (TG 5.14) tanpa menulis ulang riwayat |
| 20 | Divisi karyawan disimpan sebagai `division_id` tanpa pemilih di UI sampai master divisi (accounting) tersedia | Tabel `divisions` milik modul accounting belum dibangun |
| 21 | Restore data yang dihapus (TG 5.13) belum ada endpoint; soft delete pelanggan/mitra/PKS/kode produk/pemasok sudah tercatat di audit trail | Restore oleh Admin dibangun bersama S-106 |
| 22 | Kontak tambahan mitra (`party_contacts`) belum dibuat; PIC utama disimpan di mitra | Cukup untuk alur MT/konsinyasi R1.1 |
