# Product Requirements Document (PRD) — Fase 2
## Balans Nutrition — Unified Business Platform: Website & Channel Digital

| Item | Keterangan |
|---|---|
| Klien | Balans Nutrition |
| Dokumen | Product Requirements Document Fase 2 — Website & Channel Digital |
| Versi | 0.1 — Draft |
| Tanggal | 17 Oktober 2026 |
| Acuan | BRD Balans Unified Business Platform v0.7 (bagian 5.6, 5.8 – 5.10, 5.13, 5.17, 5.18, 6.8 – 6.11, 7.2, 8, 10, 12); Roadmap v0.6 (bagian 7, 11, 13 – 15); PRD Fase 1 v0.1; Desain Finance & Accounting Fase 1; Desain Role & Approval Fase 1; Technical Guideline v0.1 (bagian 5.10, 9, 10.11, 14.5); Naming Convention Menu & Fitur v1.0 |
| Status | Draft untuk direview tim implementasi dan Balans |

PRD ini menjabarkan **apa yang harus dihasilkan Fase 2**, perilaku yang diharapkan dari setiap fitur, kriteria penerimaannya, dan syarat go-live tiap rilis. Rincian per user story disusun di Backlog Fase 2 setelah PRD ini disetujui. Sesuai Roadmap (D-18), dokumen ini **tidak memuat target tanggal**. Yang ditetapkan adalah isi, urutan, dan syarat selesai.

---

## Daftar Isi

1. [Ringkasan](#1-ringkasan)
2. [Latar Belakang & Masalah](#2-latar-belakang--masalah)
3. [Tujuan & Ukuran Keberhasilan](#3-tujuan--ukuran-keberhasilan)
4. [Ruang Lingkup](#4-ruang-lingkup)
5. [Pengguna & Role](#5-pengguna--role)
6. [Kebutuhan Fungsional per Rilis](#6-kebutuhan-fungsional-per-rilis)
7. [Aturan Bisnis Lintas Modul](#7-aturan-bisnis-lintas-modul)
8. [Alur End-to-End & Skenario UAT](#8-alur-end-to-end--skenario-uat)
9. [Kebutuhan Non-Fungsional](#9-kebutuhan-non-fungsional)
10. [Integrasi, Dokumen Cetak & Laporan](#10-integrasi-dokumen-cetak--laporan)
11. [UAT, Migrasi, Training & Go-live](#11-uat-migrasi-training--go-live)
12. [Entry, Exit & Definition of Done](#12-entry-exit--definition-of-done)
13. [Risiko & Mitigasi](#13-risiko--mitigasi)
14. [Asumsi & Batasan](#14-asumsi--batasan)
15. [Keputusan & Isu Terbuka](#15-keputusan--isu-terbuka)
16. [Keterlacakan](#16-keterlacakan)
17. [Persetujuan](#17-persetujuan)

---

## 1. Ringkasan

Fase 2 memusatkan penjualan online Balans ke **website e-commerce findyourbalans.com** (Catatan #18), mulai mengarahkan order WA/IG ke website (Catatan #19), dan memasukkan **reseller** serta **vending machine** (Catatan #5) ke sistem yang sama dengan operasional dan akuntansi Fase 1. Seluruh transaksi Fase 2 memakai master data, stok, resolver harga, Outbound, dan posting rule yang sudah berjalan sejak go-live Fase 1.

Berbeda dengan Fase 1 yang dirilis sekaligus, Fase 2 punya **tiga titik go-live**, satu per rilis. Setiap rilis sudah bisa dipakai sendiri, dan R2.3 bergantung pada integrasi Midtrans dari R2.2 (Roadmap bagian 4).

| Rilis | Fokus | Hasil yang bisa didemokan di staging | Requirement | Must / Should | Titik go-live |
|---|---|---|---|---|---|
| **R2.1** CRM & Reseller | Satu database customer, reseller di ERP, peringatan stok | Customer dari semua channel tergabung per nomor HP; reseller diverifikasi, berlevel, order dengan harga level, bayar di muka, deposit, atau tempo; peringatan stok minimum dan kedaluwarsa terkirim | 10 | 4 / 6 | Rilis ke production setelah UAT R2.1 |
| **R2.2** Website Launch | Website e-commerce, Midtrans, Biteship penuh | Customer belanja di website, bayar lewat Midtrans, order masuk Outbound otomatis, resi dan tracking Biteship berjalan, settlement Midtrans terekonsiliasi | 16 | 11 / 5 | Soft launch, lalu launch publik |
| **R2.3** Vending Machine | Mesin, refill partner, transaksi mesin | Partner refill dari HP, transaksi mesin tercatat dari Midtrans, rekonsiliasi Midtrans vs penjualan vs stok mesin per bulan | 8 | 8 / 0 | Pilot mesin, lalu semua mesin |
| **Fase 2** | | | **34** | **23 / 11** | |

Hitungan requirement mengikuti Roadmap bagian 14: requirement yang dikerjakan bertahap dihitung di rilis pertamanya. ACC-06 (Midtrans) sudah dihitung di Fase 1 dan tidak masuk angka di atas, tetapi dikerjakan di R2.2. Usulan opsional U-31 dan U-32 masuk Fase 2 hanya jika dipilih di BRD v1.0 (bagian 4.1).

---

## 2. Latar Belakang & Masalah

1. **Penjualan online bergantung pada chat manual.** Order WA/IG dicatat CS satu per satu, pembayaran dikonfirmasi dari bukti transfer, dan invoice dikirim manual (Catatan #7). Pola ini tidak bisa menampung pertumbuhan penjualan online (OBJ-05).
2. **Website findyourbalans.com belum bisa menerima order.** Calon pembeli dari iklan dan media sosial tidak bisa langsung membeli (Catatan #18).
3. **Data customer, reseller, dan partner terpecah.** Satu customer bisa tercatat ganda di beberapa channel, sehingga riwayat belanja dan segmentasi tidak bisa dipakai untuk kampanye (OBJ-06).
4. **Reseller belum punya aturan yang tercatat di sistem.** Level, harga, minimal order, dan deposit dikelola di luar sistem, padahal stoknya diambil dari FG Non-Ecommerce yang sama dengan MT (Catatan #1, #17).
5. **Penjualan vending belum terlihat per mesin.** Transaksi di mesin dibayar lewat Midtrans, sedangkan refill dilakukan partner. Tanpa pencatatan per mesin, stok mesin, selisih, dan settlement tidak bisa direkonsiliasi (Catatan #5, OBJ-01).
6. **Pengiriman online masih input resi manual.** CS dan gudang menyalin resi dan status kiriman satu per satu (Catatan #7, arahan Balans P-20).
7. **Stok kritis dan barang mendekati kedaluwarsa baru diketahui saat laporan dibuka.** Laporan kedaluwarsa sudah ada di Fase 1 (RPT-19), tetapi belum ada peringatan aktif ke penanggung jawab lokasi.

---

## 3. Tujuan & Ukuran Keberhasilan

Ukuran keberhasilan diukur selama tiga bulan setelah go-live setiap rilis. Nilai target adalah usulan tim implementasi dan disahkan bersama PRD ini.

| ID | Tujuan | Ukuran keberhasilan | Acuan |
|---|---|---|---|
| G2-01 | Order website tercatat otomatis | 100% order website yang lunas menjadi Sales Order dan memotong E-commerce Stock tanpa input ulang; 0 order lunas tanpa Sales Order | OBJ-05, ECM-03 |
| G2-02 | Pembayaran online terkonfirmasi otomatis | 100% pembayaran website dan link bayar Midtrans terkonfirmasi lewat webhook atau cek status, tanpa konfirmasi manual F&A; selisih rekonsiliasi settlement Midtrans tiap bulan 0 atau terjelaskan | ECM-02, ACC-06 |
| G2-03 | Penjualan online pindah ke website | Dalam tiga bulan setelah launch, ≥ 50% nilai penjualan online non-marketplace (website + WA/IG) dibayar lewat checkout website atau link bayar Midtrans | Catatan #19 |
| G2-04 | Resi dan tracking otomatis | ≥ 95% kiriman website, WA/IG, dan reseller mendapat resi dari Biteship tanpa input manual; status *Diterima* terbarui dari webhook | LOG-05, P-20 |
| G2-05 | Satu database customer | 0 customer ganda dengan nomor HP yang sama; 100% reseller aktif terdaftar dan terverifikasi; 100% order reseller tercatat di sistem | OBJ-06, CRM-01, RSL-01 |
| G2-06 | Vending terkendali per mesin | 100% mesin aktif terdaftar dengan slot dan SKU; setiap bulan setiap mesin punya rekonsiliasi Midtrans vs penjualan vs stok, dan selisihnya diproses sesuai PKS | OBJ-01, VND-04 |
| G2-07 | Website andal dan cepat | Uptime website ≥ 99,5%; Core Web Vitals di HP berstatus *baik* (LCP p75 ≤ 2,5 detik, INP p75 ≤ 200 ms, CLS p75 ≤ 0,1) | NFR-06, ECM-15 |
| G2-08 | Stok kritis terdeteksi lebih awal | 100% SKU-lokasi di bawah stok minimum dan batch yang melewati ambang kedaluwarsa muncul di peringatan paling lambat H+1 | INV-07, INV-08 |

---

## 4. Ruang Lingkup

### 4.1 Dalam Lingkup

| Kode | Modul | Fase 2 mencakup |
|---|---|---|
| CRM | CRM | CRM-01, CRM-02, CRM-03 (R2.1): database customer terpadu, riwayat order dan nilai belanja, segmentasi |
| RSL | Reseller | RSL-01 s.d. RSL-04, RSL-07 (R2.1): registrasi & verifikasi, level & harga, order, ketentuan bayar, deposit. RSL-05 (R2.2): portal reseller di website |
| INV | Inventory | INV-07, INV-08 (R2.1): peringatan stok minimum dan mendekati kedaluwarsa |
| ECM | Web E-commerce | ECM-01 s.d. ECM-09, ECM-15 (R2.2): fitur launch website |
| INT | Integrasi | INT-01 Midtrans (R2.2 website & link bayar; R2.3 vending); INT-05 Biteship (R2.2) |
| LOG | Logistics | LOG-05, LOG-06 (R2.2): Biteship penuh, label pengiriman, cetak AWB marketplace |
| WMS | Warehouse | WMS-05 (R2.2): picking & packing order website di dalam Outbound |
| ACC | Finance & Accounting | ACC-06 Midtrans (R2.2); posting rule A-09, A-21, A-22, A-34 dan perluasan A-05, A-13, A-23 |
| VND | Vending Machine | VND-01 s.d. VND-05, VND-07, VND-08 (R2.3) |
| RPT | Laporan | RPT-07 (R2.3); perluasan RPT-05, RPT-11, RPT-15 untuk channel website, reseller, dan vending |
| NFR | Non-fungsional | NFR-01, 02, 04, 06, 08, 09, 10 diperluas untuk website, portal reseller, dan form partner |

**Usulan opsional** yang masuk Fase 2 hanya jika dipilih di BRD v1.0 (Roadmap bagian 11):

| Usulan | Requirement | Rilis | Isi |
|---|---|---|---|
| U-31 Kuota karyawan | SLS-11 (kuota) | R2.2 | Kuota pembelian harga karyawan per bulan, berlaku di CS dan website. Opsi potong gaji menunggu payroll (R5.2) |
| U-32 Biaya penempatan vending | VND-09, A-25 | R2.3 | Sewa tetap, bagi hasil %, atau tanpa biaya per mesin, dihitung otomatis setiap bulan |

### 4.2 Di Luar Lingkup Fase 2

- **Fitur website lanjutan** (ECM-10 s.d. ECM-14, ECM-16; R3.1): subscription, poin loyalitas, ulasan, wishlist & keranjang tertinggal, referral, pengajuan retur online oleh customer. Di Fase 2, retur order website dicatat CS lewat alur Retur Penjualan Fase 1 (F1-311).
- **Evaluasi level reseller otomatis** (RSL-06; R3.1). Di Fase 2, perubahan level diajukan manual oleh Admin Reseller dan disetujui Head Sales.
- **Laporan e-commerce** (RPT-20; R3.1). Penjualan website sudah tersedia di laporan penjualan per channel.
- **WhatsApp Business API** (INT-06; R3.3). Notifikasi customer Fase 2 lewat email dan halaman lacak pesanan.
- **API vendor vending** (VND-06, INT-03; R3.3) dan **scan barcode** (WMS-06; R3.3).
- **Sales pipeline, target, dan interaksi CS** (SLP-01 s.d. SLP-04, CRM-04; R3.2).
- **Integrasi langsung marketplace** (INT-04; Fase 4). Data customer marketplace hanya masuk CRM sejauh tersedia di file impor.
- **Payroll dan potong gaji karyawan** (HR-08 s.d. HR-11, A-33; R5.2).
- Seluruh butir BRD 2.2, termasuk aplikasi mobile native dan multi mata uang.

---

## 5. Pengguna & Role

Role back-office Fase 1 tetap berlaku. Fase 2 menambah role back-office baru dan **tiga jenis pengguna luar** yang aksesnya dibatasi pada datanya sendiri (Technical Guideline 9.2).

| Jenis | Role | Pekerjaan utama di Fase 2 |
|---|---|---|
| Back-office (baru) | Admin Reseller | Verifikasi registrasi reseller, order reseller atas nama reseller, pengajuan perubahan level, mutasi deposit |
| | Admin Vending | Master mesin & slot, pemantauan transaksi mesin, rekonsiliasi vending, penugasan partner per mesin |
| Back-office (perluasan) | Sales E-commerce | Back-office website: produk tampil di website, voucher, pemantauan order website |
| | Customer Service | Link bayar Midtrans untuk order WA/IG, bantuan customer website, pencatatan retur & refund website |
| | Marketing | Konten website (banner, halaman, blog, FAQ), kode kampanye, pengaturan pixel |
| | Staf Gudang, Fulfillment Leader | Picking gelombang dan packing order website, serah ke kurir Biteship |
| | Finance & Accounting | Konfirmasi top-up deposit, rekonsiliasi settlement Midtrans, deposit Biteship |
| | Head Sales | Approval level reseller, batas kredit reseller, voucher website |
| | HRGA | Meninjau akun website karyawan yang tidak tertaut otomatis |
| Pengguna luar | Customer website | Belanja, akun, alamat, riwayat dan lacak pesanan |
| | Reseller (portal) | Order mandiri, deposit, tagihan, dan riwayat order miliknya |
| | Partner (akses terbatas) | Form refill dan stock count untuk mesin di lokasinya |

Akun customer dan reseller dikelola di layanan storefront (`api-storefront`) dan terpisah dari user back-office. Akun partner adalah user ERP dengan role Partner yang hanya melihat mesin dan buffer di lokasinya.

**Key user** Fase 2: Sales E-commerce/CS, Reseller & Vending, Marketing, Logistik & FAT, ditambah PIC utama proyek.

---

## 6. Kebutuhan Fungsional per Rilis

Setiap fitur di bawah dipecah menjadi user story di Backlog Fase 2. Kriteria penerimaan di tabel adalah perilaku minimal yang diuji di sprint review dan UAT. **Prioritas** mengikuti BRD: M = Must, S = Should. Jika kapasitas tidak cukup, requirement S menjadi kandidat pertama untuk digeser lewat keputusan steering committee, tanpa menunda titik go-live rilisnya.

### 6.1 R2.1 — CRM & Reseller

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F2-101 | Database customer terpadu | Satu customer per nomor HP yang dinormalisasi dari semua channel (WA/IG, website, reseller, MT, karyawan); channel asal dan tanggal pertama tercatat; customer ganda bisa digabung oleh CS/Sales dengan seluruh riwayat dipindah dan tercatat di audit trail; customer marketplace hanya dari data yang tersedia di file impor | CRM-01 | M |
| F2-102 | Riwayat order & nilai belanja | Halaman customer menampilkan semua order lintas channel, nilai belanja total, jumlah order, order pertama dan terakhir, rata-rata nilai order; angka cocok dengan invoice | CRM-02 | S |
| F2-103 | Segmentasi customer | Tag manual dan tag otomatis dari aturan (karyawan, reseller per level, repeat buyer ≥ 2 order dalam 90 hari, belum belanja > 90 hari); daftar segmen bisa diekspor ke Excel hanya untuk customer yang menyetujui pemasaran | CRM-03, NFR-04 | S |
| F2-104 | Registrasi & verifikasi reseller | Data reseller: identitas, NIK (terenkripsi) dan foto KTP, data usaha, alamat, rekening; registrasi diinput Admin Reseller di R2.1 dan dari website mulai R2.2; verifikasi → aktif dengan level awal Bronze; tolak wajib alasan; reseller otomatis tertaut ke customer di CRM | RSL-01 | M |
| F2-105 | Level & harga reseller | Level Bronze/Silver/Gold dengan syarat, diskon dari harga website, minimal order, dan metode bayar yang diizinkan (BRD 5.10) bisa diubah tanpa pengembangan; harga order dari resolver memakai price list level reseller; HET per SKU tersimpan dan tampil; perubahan level manual diajukan Admin Reseller dan disetujui Head Sales | RSL-02, MD-04 | M |
| F2-106 | Order reseller | Sales Order channel reseller memotong FG Non-Ecommerce dengan alokasi FEFO; order di bawah minimal order level ditolak; ongkir ditanggung reseller sebagai baris tersendiri; opsi ambil sendiri di gudang; keluar lewat Outbound dengan QC; jurnal A-05 + A-06 dengan dimensi channel `reseller` dan sub-channel level | RSL-03 | M |
| F2-107 | Ketentuan bayar reseller | Bayar di muka (default): transfer dengan bukti bayar dikonfirmasi F&A, Midtrans mulai R2.2; tempo hanya untuk level yang diizinkan (Gold), dengan batas kredit per reseller dan jatuh tempo 14 hari; order yang melewati batas kredit atau punya tagihan lewat jatuh tempo butuh approval Head Sales → Head F&A; piutang reseller masuk umur piutang | RSL-04 | S |
| F2-108 | Deposit reseller | Top-up deposit dikonfirmasi F&A (A-34); pemakaian deposit memotong saldo saat order dibayar (A-05 debit Deposit Reseller); saldo tidak bisa negatif; mutasi saldo per reseller dengan saldo berjalan; pengembalian sisa deposit butuh approval Head F&A | RSL-07 | S |
| F2-109 | Peringatan stok minimum | Stok minimum per SKU per lokasi (termasuk E-commerce Stock, Petty Stock, buffer vending) dihitung otomatis dari pola keluar barang dan bisa ditimpa Logistik (bagian 15.1 #3); job harian dan pengecekan setelah mutasi; notifikasi in-app + email ke penanggung jawab lokasi dalam ringkasan harian; peringatan tidak memblokir transaksi | INV-07 | S |
| F2-110 | Peringatan mendekati kedaluwarsa | Ambang hari sebelum kedaluwarsa per kategori produk (nilai awal 90 hari di gudang Balans, 120 hari di lokasi eksternal); mencakup gudang, lokasi konsinyasi, buffer vending, dan mesin; ringkasan harian ke Logistik Leader, Sales MT, dan Admin Vending sesuai lokasi; tautan ke laporan RPT-19 | INV-08 | S |

**Deliverable R2.1:** satu database customer berjalan; reseller terdaftar, berlevel, dan bisa order dengan tiga metode bayar; peringatan stok aktif. Pada titik go-live R2.1, data reseller dan saldo deposit yang berjalan dimigrasikan (bagian 11).

### 6.2 R2.2 — Website Launch

**Tahapan:** desain UI (paralel dengan UAT Fase 1) → build → UAT & soft launch → launch publik. Desain UI website mengikuti pola layout dan alur belanja olly.com sebagai referensi, dengan logo, foto produk, dan konten milik Balans dari findyourbalans.com. Aset merek dan konten pihak referensi tidak dipakai.

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F2-201 | Katalog & halaman produk | Daftar produk per kategori dan tipe kemasan; pencarian dan filter; halaman produk dengan foto, deskripsi, informasi nilai gizi, varian, dan bundel; status stok dari E-commerce Stock tersedia (bundel = min stok komponen); data produk dari master MD-01, konten website dikelola di ERP | ECM-01 | M |
| F2-202 | Keranjang & checkout | Checkout sebagai tamu atau dengan akun; alamat dengan pin lokasi; harga dari resolver channel `website`; voucher dihitung terakhir (BRD 5.18); stok dipesan saat order dibuat dan dilepas otomatis jika pembayaran tidak selesai dalam 24 jam; stok tidak cukup ditolak dengan pesan yang menyebut produk dan qty tersedia | ECM-01, ECM-03 | M |
| F2-203 | Pembayaran Midtrans | VA, QRIS, e-wallet, kartu; `order_id` memakai nomor dokumen Balans dengan prefix channel (U-17); webhook diverifikasi signature, status dikonfirmasi ulang ke API Midtrans, dan event ganda diabaikan; job cek status untuk transaksi `pending` yang melewati batas waktu; refund penuh atau sebagian lewat API dari alur Retur Penjualan; data kartu tidak pernah disimpan Balans | ECM-02, INT-01 | M |
| F2-204 | Order website masuk sistem | Pembayaran lunas → Sales Order website terkonfirmasi, alokasi FEFO dari E-commerce Stock, masuk antrian Outbound; penjualan diakui saat diserahkan ke kurir (U-25) dengan jurnal A-21 + A-06; ongkir dicatat sebagai Pendapatan Ongkos Kirim; stok tersedia di website = E-commerce Stock − reserved − buffer stok website | ECM-03 | M |
| F2-205 | Ongkir Biteship | Pilihan kurir dan ongkir dari Biteship berdasarkan alamat gudang dan alamat customer; kurir instan hanya tampil jika pin lokasi diisi; berat dan dimensi dari master produk; daftar kurir aktif bisa diatur Sales E-commerce; jika Biteship tidak merespons, checkout menampilkan pesan coba lagi dan tidak menebak ongkir | ECM-04, INT-05 | M |
| F2-206 | Akun customer & voucher | Daftar dengan email atau HP dan password, verifikasi email, reset password; multi alamat; riwayat order; persetujuan pemasaran terpisah dari syarat belanja; penghapusan akun dijalankan sebagai anonimisasi (UU PDP). Voucher: kode, periode, kuota total dan per customer, minimal belanja, nominal atau persen dengan batas maksimal, cakupan SKU/kategori, satu voucher per order; voucher aktif setelah approval Head Sales | ECM-05 | M |
| F2-207 | Harga karyawan & reseller yang login | Akun website tertaut otomatis ke karyawan aktif jika email atau HP terverifikasi cocok dengan data karyawan (selain itu ditinjau HRGA), atau ke reseller terverifikasi; harga dari resolver sesuai price list karyawan atau level reseller; karyawan resign atau reseller nonaktif kehilangan hak harga seketika | ECM-06 | M |
| F2-208 | Portal reseller | Registrasi reseller dari website masuk antrian verifikasi F2-104; order mandiri dengan minimal order level; bayar lewat Midtrans, deposit, atau tempo sesuai level; top-up deposit lewat Midtrans dengan konfirmasi otomatis; daftar tagihan, mutasi deposit, dan riwayat order; reseller hanya melihat datanya sendiri | RSL-05 | S |
| F2-209 | Lacak pesanan & notifikasi | Halaman status order (tamu dengan link + nomor HP); email saat pembayaran diterima, dikirim (resi + link tracking), dan diterima; email memakai provider transaksional Fase 1 dengan template Balans | ECM-08 | M |
| F2-210 | Link dari WA/IG & link bayar | Link halaman produk dengan parameter UTM untuk WA dan IG; CS membuat link bayar Midtrans dari order WA/IG (SLS-04) yang lunas otomatis tanpa konfirmasi F&A; link kedaluwarsa melepas alokasi; jalur bukti transfer tetap tersedia | ECM-07, INT-01 | S / M |
| F2-211 | Biteship penuh | Setelah QC Outbound lulus, order kurir Biteship dibuat untuk order website, WA/IG, dan reseller; request pickup; resi otomatis tersimpan di DO; status dari webhook memperbarui DO dan memicu notifikasi; kiriman gagal atau dikembalikan kurir masuk Inbound sebagai retur; top-up dan pemakaian Deposit Biteship berjurnal A-22 | LOG-05, INT-05 | M |
| F2-212 | Label pengiriman & AWB | Cetak label Biteship 100 × 150 mm dari DO, satuan atau massal per gelombang; unggah dan cetak massal AWB marketplace dari seller center untuk order marketplace manual | LOG-06 | S |
| F2-213 | Picking & packing order website | Order website digabung per gelombang sesuai jadwal pickup; picking list gabungan per lokasi dan batch FEFO; packing per order dengan packing slip; QC Outbound tetap oleh user yang berbeda dari penyiap (Aturan Fase 1 #2) | WMS-05 | S |
| F2-214 | CMS | Banner, halaman statis, blog/artikel, FAQ; draft → terbit dengan jadwal tayang; pratinjau sebelum terbit; field SEO per halaman; dikelola role Marketing | ECM-09 | S |
| F2-215 | SEO & analytics | Halaman dirender server (SSR); meta, Open Graph, structured data produk, `sitemap.xml`, `robots.txt`, URL ramah; redirect 301 dari URL website lama; GA4, Meta Pixel, TikTok Pixel dengan event `view_item`, `add_to_cart`, `begin_checkout`, `purchase` yang dimuat setelah persetujuan cookie | ECM-15 | M |
| F2-216 | Rekonsiliasi settlement Midtrans | Laporan settlement Midtrans diimpor atau ditarik lewat API; dicocokkan per `order_id`; jurnal dana cair (debit Bank + Beban Fee Payment Gateway, kredit Piutang Midtrans); transaksi belum cair dan selisih terlihat per channel (website, link bayar, vending mulai R2.3) | ACC-06 | M |

**Deliverable R2.2:** website live di findyourbalans.com; order website, link bayar WA/IG, dan order reseller dari portal berjalan otomatis sampai resi, tracking, jurnal, dan settlement.

### 6.3 R2.3 — Vending Machine

**Prasyarat:** akun Midtrans vending atas nama Balans (Roadmap risiko #5) dan kepastian dari vendor apakah data transaksi mesin memuat SKU (U-05).

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F2-301 | Master mesin & slot | Mesin: kode unik (dipakai di `order_id` Midtrans), lokasi, partner dan PKS, status aktif, tertaut ke register aset tetap (U-11); slot dengan SKU dan kapasitas per slot; harga jual per mesin dari price list vending; mesin baru ditambah lewat konfigurasi | VND-01, NFR-09 | M |
| F2-302 | Stok mesin & refill | Setiap mesin adalah lokasi stok; refill = transfer buffer → mesin tanpa jurnal (A-24); refill mencatat per slot SKU, batch, sisa sebelum refill, dan qty diisi; sisa sebelum refill dibandingkan dengan stok sistem dan selisihnya diproses di F2-304 | VND-02 | M |
| F2-303 | Form refill & stock count partner | Form HP untuk partner: hanya mesin dan buffer di lokasinya; target sentuh ≥ 44 px, autosave, foto wajib per mesin; stock count mesin dan buffer bisa dilakukan partner atau Merchandiser | VND-07 | M |
| F2-304 | Buffer vending & selisih | Stok buffer vending terpisah dari stok konsinyasi partner; stock count bulanan mesin + buffer bersamaan dengan rekonsiliasi konsinyasi; selisih ditagih ke partner atau ditanggung Balans sesuai PKS (A-23) | VND-08 | M |
| F2-305 | Transaksi mesin via Midtrans | Webhook Midtrans dengan kode mesin di `order_id` mencatat penjualan per mesin (A-09); jika SKU tersedia, stok mesin terpotong per SKU dan HPP tercatat saat transaksi; jika tidak, penjualan tercatat per mesin dan stok serta HPP per SKU dipotong saat refill berikutnya dari selisih stok (U-05); transaksi gagal atau refund tidak memotong stok | VND-03, INT-01 | M |
| F2-306 | Rekonsiliasi vending | Per mesin per periode: transaksi Midtrans vs penjualan tercatat vs pergerakan stok mesin; Berita Acara Rekonsiliasi Vending (PDF); approval Admin Vending → Head F&A; dana cair dicocokkan lewat F2-216 | VND-04 | M |
| F2-307 | Penjualan lewat partner | Pembelian yang dijual sendiri oleh partner memakai stok konsinyasi partner dan alur konsinyasi Fase 1 (F1-303), terpisah dari stok buffer vending | VND-05 | M |
| F2-308 | Laporan vending | Penjualan dan stok per mesin, per slot, per SKU; margin per mesin; mesin tanpa transaksi > 3 hari ditandai sebagai indikasi gangguan | RPT-07 | M |

**Deliverable R2.3:** semua mesin aktif tercatat, partner refill dari HP, transaksi mesin tercatat dari Midtrans, dan rekonsiliasi vending bulan pertama selesai.

---

## 7. Aturan Bisnis Lintas Modul

Aturan Fase 1 (PRD Fase 1 bagian 7) tetap berlaku. Fase 2 menambah aturan berikut.

| # | Aturan | Acuan |
|---|---|---|
| 1 | **Pembayaran online hanya dianggap lunas setelah dikonfirmasi ulang ke Midtrans.** Payload webhook tidak dipercaya mentah-mentah; event ganda diabaikan lewat event ID | TG 9.3, INT-01 |
| 2 | **Stok website dipesan saat order dibuat**, bukan saat lunas, dan dilepas otomatis jika pembayaran kedaluwarsa (24 jam) atau dibatalkan | ECM-03, BRD 5.9 |
| 3 | **Pool stok per channel tetap seperti Fase 1.** Website memakai E-commerce Stock; reseller memakai FG Non-Ecommerce; mesin memakai stok per mesin yang diisi dari buffer vending | Catatan #1, RSL-03, VND-02 |
| 4 | **Buffer stok website.** Sebagian E-commerce Stock ditahan dari website (per SKU, nilai awal dihitung otomatis dan bisa diubah Sales E-commerce, bagian 15.1 #6) karena order marketplace manual memakai pool yang sama | ECM-03, SLS-05 |
| 5 | **Pengakuan penjualan per channel (U-25):** website dan link bayar saat serah ke kurir; reseller saat invoice; vending saat transaksi di mesin | U-25, A-05, A-09, A-21 |
| 6 | **Harga diambil resolver** untuk website, karyawan yang login, dan reseller. Voucher dihitung setelah harga dan diskon manual | BRD 5.18 |
| 7 | **Hak harga melekat pada status saat ini.** Karyawan resign atau reseller nonaktif langsung kehilangan harga khususnya; order yang sudah dibayar tidak berubah | 5.13, RSL-02 |
| 8 | **Pengguna luar hanya melihat datanya sendiri.** Customer melihat order dan alamatnya; reseller melihat order, deposit, dan tagihannya; partner melihat mesin dan buffer di lokasinya. Scoping dijalankan di repository, bukan hanya di UI | NFR-02, TG 9.2 |
| 9 | **Deposit reseller tidak boleh negatif** dan setiap mutasinya tercatat dengan saldo berjalan | RSL-07, A-34 |
| 10 | **Selisih stok mesin dan buffer menjadi tanggung jawab partner** sesuai PKS | U-12, A-23 |
| 11 | **Satu customer per nomor HP** di seluruh channel. Penggabungan customer tercatat di audit trail dan tidak mengubah dokumen lama | CRM-01 |
| 12 | **Pemasaran hanya ke customer yang menyetujui.** Persetujuan pemasaran dicatat terpisah dengan waktu dan sumbernya | NFR-04 (UU PDP) |
| 13 | **Order WA/IG manual tetap tersedia** setelah website live. CS mengarahkan customer ke website atau mengirim link bayar | Catatan #19, BRD 5.8 |

**Approval baru di Fase 2** (melengkapi Desain Role & Approval; batas dan approver bisa dikonfigurasi):

| Dokumen | Level 1 | Level 2 |
|---|---|---|
| Perubahan level reseller | Head Sales | — |
| Batas kredit reseller (penetapan atau perubahan) | Head Sales | Head Finance & Accounting |
| Order reseller melewati batas kredit atau dengan tagihan lewat jatuh tempo | Head Sales | Head Finance & Accounting |
| Pengembalian sisa deposit reseller | Head Finance & Accounting | — |
| Voucher website | Head Sales | — |
| Rekonsiliasi vending & penagihan selisih | Admin Vending | Head Finance & Accounting |

---

## 8. Alur End-to-End & Skenario UAT

Setiap skenario dinyatakan lulus jika dokumen, stok, jurnal, dan laporan terkait sesuai hasil yang diharapkan, dan key user menandatanganinya. Skenario dijalankan di staging (`erp-staging.findyourbalans.com` dan `staging.findyourbalans.com`).

| ID | Rilis | Skenario | Divisi penguji | Hasil yang diperiksa |
|---|---|---|---|---|
| UAT2-01 | R2.1 | Customer yang sama order lewat WA/IG dan MT dengan format nomor HP berbeda → tergabung; gabung manual dua customer ganda | CS, Sales MT | Satu customer, riwayat order lengkap, audit penggabungan |
| UAT2-02 | R2.1 | Registrasi reseller → verifikasi → Bronze → order di bawah minimal (ditolak) → order valid bayar transfer → Outbound → kirim | Reseller | Harga level, potong FG Non-Ecommerce, jurnal A-05 + A-06 dimensi reseller |
| UAT2-03 | R2.1 | Top-up deposit → order bayar deposit → order melebihi saldo (ditolak) → pengembalian sisa deposit dengan approval | Reseller, F&A | Mutasi deposit, jurnal A-34, saldo tidak negatif |
| UAT2-04 | R2.1 | Perubahan level ke Gold dengan approval → penetapan batas kredit → order tempo → order melewati batas kredit dengan approval → pembayaran tagihan | Reseller, Head Sales, F&A | Approval, piutang reseller, umur piutang |
| UAT2-05 | R2.1 | Stok E-commerce Stock di bawah minimum; batch di lokasi konsinyasi dan buffer vending masuk ambang kedaluwarsa | Logistik, Sales MT | Ringkasan peringatan harian ke penanggung jawab yang benar |
| UAT2-06 | R2.2 | Tamu checkout dengan voucher dan kurir instan → bayar VA → webhook → Outbound gelombang → QC → order Biteship → pickup → tracking → diterima | Sales E-commerce, Logistik | Reservasi dan alokasi FEFO, resi otomatis, email notifikasi, jurnal A-21, A-06, A-22 |
| UAT2-07 | R2.2 | Order tidak dibayar 24 jam; webhook ganda; webhook dengan signature salah; webhook tidak pernah datang | Sales E-commerce, Tech | Reservasi dilepas, event ganda diabaikan, signature salah ditolak, job cek status melunasi order |
| UAT2-08 | R2.2 | Karyawan dan reseller login → harga khusus; karyawan dinonaktifkan HRGA → harga umum di sesi berikutnya | HRGA, Reseller | Harga dari resolver, hilangnya hak harga |
| UAT2-09 | R2.2 | Reseller registrasi dari website → verifikasi → order mandiri lewat portal dengan Midtrans dan deposit; reseller lain mencoba membuka order tersebut | Reseller | Portal, top-up otomatis, penolakan akses |
| UAT2-10 | R2.2 | Order WA/IG → link bayar Midtrans → lunas otomatis → Outbound → Biteship | CS | Tidak ada konfirmasi manual F&A, invoice lunas |
| UAT2-11 | R2.2 | Retur order website → Inbound → Gudang Retur → refund sebagian lewat Midtrans | CS, Logistik, F&A | Refund di Midtrans, jurnal A-13, notifikasi customer |
| UAT2-12 | R2.2 | Impor settlement Midtrans satu periode, termasuk transaksi refund dan belum cair | F&A | Jurnal dana cair, Piutang Midtrans per order, selisih 0 atau terjelaskan |
| UAT2-13 | R2.2 | Terbitkan banner dan artikel terjadwal; cek sitemap, meta, redirect URL lama, event pixel setelah persetujuan cookie | Marketing | Konten tayang sesuai jadwal, SEO dan tracking berjalan |
| UAT2-14 | R2.3 | DO ke buffer vending → partner refill dari HP dengan foto → transaksi mesin dengan SKU dan tanpa SKU → refill berikutnya | Vending | Stok mesin, pemotongan saat refill (U-05), jurnal A-09, A-24 |
| UAT2-15 | R2.3 | Stock count bulanan mesin + buffer dengan selisih → rekonsiliasi vending → approval → tagihan selisih ke partner; settlement Midtrans vending | Vending, F&A | Berita Acara Rekonsiliasi Vending, jurnal A-23, settlement |
| UAT2-16 | Semua | Uji akses: customer, reseller, partner, Admin Reseller, Admin Vending membuka data dan aksi di luar haknya | Semua | UI tersembunyi, API menolak 403/404, audit akses |

---

## 9. Kebutuhan Non-Fungsional

Target kinerja adalah usulan tim implementasi untuk VPS 4 core / 8 GB yang sama dengan Fase 1, dan diuji sebelum launch R2.2.

| NFR | Kebutuhan Fase 2 | Cara uji |
|---|---|---|
| NFR-01 Platform | Website mobile-first dan responsif; portal reseller dan form partner nyaman di HP | Playwright di 3 lebar layar; uji di HP Android kelas menengah |
| NFR-02 Keamanan | Akun customer terpisah dari user back-office; refresh token customer di cookie `HttpOnly`; rate limit login, registrasi, reset password, OTP, dan API publik per IP dan per akun; verifikasi signature webhook; scoping data pengguna luar | Test API per jenis pengguna (UAT2-16); uji keamanan dasar OWASP sebelum launch |
| NFR-04 Privasi | Persetujuan pemasaran dan cookie tercatat; NIK reseller terenkripsi; penghapusan akun = anonimisasi; data kartu tidak disimpan (diproses Midtrans); tidak ada data pribadi di log, Sentry, atau event pixel | Review kode + test |
| NFR-06 Ketersediaan | Website ≥ 99,5%; ERP tetap ≥ 99% pada jam operasional; monitor eksternal untuk website, `api-storefront`, dan endpoint webhook | Laporan uptime tiga bulan setelah launch |
| NFR-08 Dokumen | Invoice order website (unduh dari akun), tagihan reseller, bukti top-up deposit, packing slip, label Biteship, Berita Acara Rekonsiliasi Vending, dengan logo dan penomoran U-15 | Review layout oleh F&A & Sales |
| NFR-09 Skalabilitas | Tambah mesin, slot, kurir, level reseller, dan voucher lewat konfigurasi | Demo di UAT |
| NFR-10 Kapasitas | ≥ 50 mesin vending; ≥ 200 pengunjung website bersamaan dan ≥ 20 checkout per menit tanpa menurunkan kinerja ERP; halaman katalog dan produk TTFB p95 ≤ 800 ms dengan cache; checkout p95 ≤ 3 detik; webhook pembayaran diterima ≤ 1 detik dan diproses di queue `critical` | Uji beban sebelum launch R2.2 |
| NFR-11 Notifikasi | Email customer (pembayaran, dikirim, diterima); notifikasi in-app + email untuk approval baru dan peringatan stok | UAT2-05, UAT2-06 |

---

## 10. Integrasi, Dokumen Cetak & Laporan

### 10.1 Integrasi

| Integrasi | Fase 2 | Catatan |
|---|---|---|
| Midtrans (INT-01) | R2.2: pembayaran website, link bayar WA/IG, top-up deposit reseller, refund, settlement. R2.3: transaksi vending | Satu akun atas nama Balans; `order_id` berprefix per channel/mesin (U-17); adapter mock dipakai sampai akun sandbox tersedia (D-27) |
| Biteship (INT-05) | R2.2: cek ongkir, order kurir, pickup, resi, label, tracking webhook | Akun atas nama Balans; paket dipilih sesuai volume (Roadmap D-02) |
| Email transaksional | Notifikasi customer website memakai provider dan domain pengirim Fase 1 | Template terpisah untuk customer |
| Analytics & pixel | GA4, Meta Pixel, TikTok Pixel di website | Dimuat setelah persetujuan cookie; ID akun dari Marketing |
| Excel (INT-02) | Impor data reseller dan saldo deposit untuk migrasi R2.1; impor settlement Midtrans; ekspor segmen customer | Template dihasilkan sistem |
| WhatsApp, API vending, marketplace | Tidak ada integrasi di Fase 2 | R3.3 dan Fase 4 |

### 10.2 Dokumen Cetak & Label

Layout mengikuti Desain Penomoran & Dokumen Cetak dan diperluas dengan: invoice order website, tagihan reseller, bukti top-up deposit, packing slip, label pengiriman Biteship 100 × 150 mm, dan Berita Acara Rekonsiliasi Vending.

### 10.3 Laporan

| ID | Laporan | Pengguna utama |
|---|---|---|
| RPT-05 (perluasan) | Penjualan per channel termasuk website, link bayar, reseller per level, vending per mesin | Sales, Manajemen |
| RPT-07 | Penjualan dan stok per vending machine, slot, SKU; margin per mesin | Admin Vending |
| RPT-11 (perluasan) | Umur piutang reseller tempo; mutasi dan saldo deposit reseller | F&A, Admin Reseller |
| RPT-15 (perluasan) | Laba rugi per channel dengan sub-channel `findyourbalans`, level reseller, dan mesin | F&A, Manajemen |
| Rekonsiliasi Midtrans | Transaksi lunas, cair, belum cair, fee, dan selisih per channel | F&A |
| Order website | Order per status, reservasi kedaluwarsa, pemakaian voucher, kiriman per kurir | Sales E-commerce |
| Customer | Customer baru per channel, repeat buyer, segmen, nilai belanja | Sales, Marketing |

---

## 11. UAT, Migrasi, Training & Go-live

### 11.1 Tahapan per Rilis

| Rilis | Tahap | Aktivitas | Selesai jika |
|---|---|---|---|
| R2.1 | Build & UAT | Sprint R2.1; UAT2-01 s.d. UAT2-05 | Semua skenario lulus dan ditandatangani |
| | Migrasi | Data reseller, level, batas kredit, dan saldo deposit per tanggal go-live diimpor; saldo deposit dicocokkan dengan akun 2401 Deposit Reseller | Saldo deposit per reseller disetujui Head F&A |
| | Training & go-live | Training Admin Reseller, F&A, CS; rilis ke production | Go/no-go R2.1 terpenuhi |
| R2.2 | Desain | Desain UI website, struktur konten, daftar URL lama untuk redirect; panduan storefront (TG 10.11) | Desain disetujui Sales E-commerce dan Marketing |
| | Build & UAT | Sprint R2.2 dengan adapter mock; UAT2-06 s.d. UAT2-13 dengan akun Midtrans dan Biteship sandbox | Semua skenario lulus dan ditandatangani |
| | Soft launch | Website production dengan akun Midtrans dan Biteship production, terbatas untuk karyawan dan reseller undangan selama minimal 2 minggu | ≥ 30 order nyata selesai sampai diterima dan settlement cocok |
| | Launch publik | DNS findyourbalans.com dialihkan ke website baru; redirect 301 aktif; CS mulai mengarahkan WA/IG ke website | Go/no-go R2.2 terpenuhi |
| R2.3 | Build & UAT | Sprint R2.3; UAT2-14, UAT2-15 | Semua skenario lulus |
| | Pilot | 1 – 2 mesin di satu partner selama satu siklus rekonsiliasi bulanan | Rekonsiliasi pilot selesai tanpa selisih yang tidak terjelaskan |
| | Roll-out | Mesin lain ditambahkan bertahap; stock count awal mesin dan buffer per mesin | Semua mesin aktif terdaftar |

### 11.2 Go / No-Go

Setiap rilis diputuskan steering committee jika butir umum dan butir rilisnya terpenuhi.

**Umum:** tidak ada defect kritis yang terbuka; defect tinggi punya workaround tertulis yang disetujui key user; key user rilis menandatangani UAT; user terkait sudah ditraining; backup dan uji restore terakhir berhasil.

| Rilis | Butir khusus |
|---|---|
| R2.1 | Saldo deposit migrasi disetujui Head F&A; approver level dan batas kredit reseller terkonfigurasi |
| R2.2 | Akun Midtrans dan Biteship production atas nama Balans aktif dan adapter mock nonaktif (D-27); uji beban lolos target bagian 9; monitor uptime website aktif; redirect URL lama teruji; kebijakan privasi, syarat & ketentuan, dan kebijakan retur tayang dan disetujui Balans; soft launch selesai |
| R2.3 | Akun Midtrans vending atas nama Balans; semua mesin pilot punya kode mesin, slot, dan PKS berlaku; partner pilot sudah ditraining form refill |

### 11.3 Rencana Mundur

- **R2.2:** jika masalah kritis di website tidak bisa diperbaiki dalam 4 jam, website menampilkan halaman pemeliharaan dan CS kembali menerima order lewat WA/IG dengan link bayar atau bukti transfer. Order yang sudah dibayar tetap diproses dari ERP. DNS tidak dikembalikan ke website lama.
- **R2.3:** jika pencatatan transaksi mesin bermasalah, penjualan mesin dicatat sementara dari laporan Midtrans dan selisih stok saat refill, seperti mekanisme tanpa SKU (U-05).
- Rollback aplikasi ke versi sebelumnya mengikuti Runbook Operasi.

---

## 12. Entry, Exit & Definition of Done

### 12.1 Entry Criteria

| # | Kriteria | Berlaku untuk |
|---|---|---|
| 1 | Fase 1 sudah go-live dan hypercare selesai (tutup buku bulan pertama di sistem) | Build R2.1 dan R2.2 |
| 2 | PRD Fase 2 disetujui; Backlog Fase 2 terestimasi; track website dan track ERP terbentuk (Roadmap asumsi #2) | Fase 2 |
| 3 | Nilai level reseller dikonfirmasi atau memakai nilai awal BRD 5.10 | R2.1 |
| 4 | Aset brand (logo, foto produk, konten) dan akses domain findyourbalans.com tersedia | Desain R2.2 |
| 5 | Akun Midtrans dan Biteship atas nama Balans (sandbox, lalu production) | UAT R2.2 |
| 6 | Akun Midtrans vending atas nama Balans; informasi vendor tentang data transaksi mesin | R2.3 |

Desain UI website boleh dimulai paralel dengan UAT Fase 1 (Roadmap bagian 4), karena tidak mengubah sistem yang sedang diuji.

### 12.2 Exit per Rilis

| Rilis | Exit criteria |
|---|---|
| R2.1 | Semua story R2.1 memenuhi Definition of Done; UAT2-01 s.d. UAT2-05 lulus; go-live R2.1 selesai |
| R2.2 | Semua story R2.2 memenuhi Definition of Done; UAT2-06 s.d. UAT2-13 lulus; F&A memvalidasi jurnal A-21, A-22, A-13 refund, dan settlement; launch publik selesai |
| R2.3 | Semua story R2.3 memenuhi Definition of Done; UAT2-14, UAT2-15 lulus; F&A memvalidasi jurnal A-09 dan A-23; pilot selesai |

### 12.3 Definition of Done Story

Sama dengan Fase 1 (PRD Fase 1 bagian 12.3). Untuk story website ditambah: halaman lolos audit Lighthouse mobile (performance ≥ 80, accessibility ≥ 90) dan tidak ada data pribadi di event analytics.

### 12.4 Exit Fase 2

Fase 2 ditutup di steering committee setelah ketiga rilis live, rekonsiliasi Midtrans dan rekonsiliasi vending bulan pertama selesai, ukuran keberhasilan G2-01, G2-02, dan G2-06 tercapai, dan daftar defect tidak memuat defect kritis.

---

## 13. Risiko & Mitigasi

| # | Risiko | Dampak | Mitigasi |
|---|---|---|---|
| 1 | Website, SSR, dan ERP berbagi 1 VPS 8 GB | Lonjakan trafik kampanye memperlambat ERP atau website down | Batas memori per container (web-store 512 MB), cache halaman katalog, uji beban sebelum launch, monitor uptime; jalur naik kelas: upgrade VPS → pisahkan database → pisahkan website (Roadmap risiko #12) |
| 2 | Stok E-commerce Stock dipakai bersama website dan marketplace manual | Oversell di website | Buffer stok website per SKU (Aturan #4); impor marketplace lebih sering; peringatan stok minimum |
| 3 | Webhook Midtrans atau Biteship hilang atau tertunda | Order lunas tidak diproses; status kiriman tidak terbarui | Job cek status berkala (TG 5.10); dedup event; alert jika antrian `critical` menumpuk |
| 4 | Peringkat SEO turun saat website lama diganti | Trafik organik turun | Daftar URL lama dan redirect 301; sitemap dikirim ke Search Console saat launch |
| 5 | Penyalahgunaan voucher atau harga karyawan | Margin turun | Kuota voucher total dan per customer; voucher butuh approval; akun karyawan hanya tertaut ke data karyawan aktif; opsi kuota karyawan (U-31) |
| 6 | Reseller tempo menunggak | Piutang macet | Tempo hanya untuk Gold; batas kredit dengan approval; order diblokir jika ada tagihan lewat jatuh tempo |
| 7 | Data transaksi vending tidak memuat SKU | Stok mesin baru akurat saat refill | Pemotongan saat refill (U-05); stock count bulanan; API vendor di R3.3 |
| 8 | Akun Midtrans vending masih milik vendor | Dana tidak langsung ke Balans; rekonsiliasi sulit | Gate R2.3: akun dipindah ke atas nama Balans sebelum build R2.3 dimulai |
| 9 | Partner tidak disiplin mengisi form refill | Stok mesin dan buffer tidak akurat | Foto wajib, form sederhana, stock count bulanan, selisih ditanggung partner sesuai PKS |
| 10 | Scope creep ke fitur website lanjutan | Launch R2.2 mundur | Fitur dibagi launch (R2.2) dan lanjutan (R3.1) sesuai D-15; perubahan lewat change request |
| 11 | Customer tetap memilih chat WA/IG | Target G2-03 tidak tercapai | Link produk dan link bayar dari CS; voucher khusus pembelian pertama di website; order WA/IG manual tetap dilayani |

---

## 14. Asumsi & Batasan

1. Fase 2 dikerjakan dua track paralel: **track website** (R2.2: web-store, `api-storefront`, Midtrans, Biteship) dan **track ERP** (R2.1, lalu R2.3). Track website butuh tambahan kapasitas frontend (Roadmap asumsi #2).
2. Setiap titik go-live menaikkan versi minor aplikasi (R2.1, R2.2, dan R2.3 masing-masing satu versi minor). Modul yang belum dirilis tetap tersembunyi dengan feature flag modul (D-22).
3. Sebelum portal reseller live di R2.2, registrasi dan order reseller diinput Admin Reseller di ERP, dan pembayaran di muka memakai transfer dengan bukti bayar.
4. Website memakai satu gudang asal pengiriman (Gudang Utama, U-08). Gudang asal kedua ditambah lewat konfigurasi jika dibutuhkan.
5. Batas waktu pembayaran website 24 jam mengikuti BRD 5.9 dan bisa diubah di pengaturan.
6. Harga karyawan di website memakai price list karyawan Fase 1 (nilai awal diskon 20% dari harga website).
7. Notifikasi customer di Fase 2 lewat email dan halaman lacak pesanan. Notifikasi WhatsApp otomatis menunggu WhatsApp Business API (R3.3).
8. Retur order website di Fase 2 diajukan customer ke CS lalu dicatat lewat alur Retur Penjualan Fase 1. Pengajuan dari akun customer masuk R3.1.
9. Harga jual di mesin vending diatur di mesin oleh Admin Vending sesuai price list vending; sistem mencatat nominal aktual dari Midtrans dan memakai price list sebagai acuan margin.
10. Kebijakan privasi, syarat & ketentuan, dan kebijakan retur website disiapkan dan disahkan Balans; tim implementasi menyediakan halaman dan kerangkanya.
11. Panduan storefront (SEO, caching, analytics, pixel) disusun sebagai dokumen terpisah sebelum build R2.2 (TG 10.11).

---

## 15. Keputusan & Isu Terbuka

### 15.1 Keputusan

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| 1 | Kode dokumen Fase 2 | Order website, reseller, dan link bayar memakai kode **SO** dengan channel sebagai dimensi. Kode baru: **DEP** (mutasi deposit reseller), **RFL** (refill vending), **RKV** (Berita Acara Rekonsiliasi Vending), dengan format U-15 | SO sudah berlaku untuk semua channel (Desain Penomoran); kode per channel akan memecah urutan nomor tanpa manfaat bagi laporan, karena channel sudah tersedia sebagai dimensi |
| 2 | Verifikasi akun website karyawan | Akun website **tertaut otomatis** ke karyawan jika email atau nomor HP yang sudah diverifikasi (OTP/link verifikasi) sama dengan data karyawan aktif. Akun yang tidak cocok masuk antrian tinjauan HRGA. Status resign memutus tautan seketika | Karyawan tidak perlu menunggu approval manual; HRGA hanya menangani pengecualian; data karyawan Fase 1 menjadi satu-satunya sumber status karyawan |
| 3 | Nilai awal stok minimum | Stok minimum per SKU-lokasi dihitung otomatis setiap minggu = rata-rata keluar harian 30 hari terakhir × (lead time pengisian + 7 hari pengaman), dibulatkan ke atas ke kelipatan isi karton. Lead time awal: 2 hari untuk lokasi yang diisi dari gudang sendiri (E-commerce Stock, Petty Stock, buffer vending), dan lead time maklon per pemasok untuk FG Non-Ecommerce. Logistik bisa menimpa nilai per SKU-lokasi | Logistik dan Sales tidak perlu mengisi ratusan kombinasi SKU-lokasi secara manual; nilai mengikuti pola penjualan aktual |
| 4 | Ambang peringatan kedaluwarsa | 90 hari untuk lokasi di gudang Balans; 120 hari untuk lokasi konsinyasi, buffer vending, dan mesin; bisa diubah per kategori produk | Lokasi eksternal butuh waktu tambahan untuk penarikan atau penukaran oleh Merchandiser |
| 5 | Daftar URL website lama | Tim implementasi menarik seluruh URL website lama dari sitemap dan crawler sebelum desain R2.2 selesai. URL produk, kategori, dan artikel dipetakan redirect 301 ke padanannya; URL tanpa padanan diarahkan ke kategori terdekat atau beranda. Marketing meninjau hasil pemetaan | Pengumpulan URL bisa dilakukan tanpa menunggu Balans; peninjauan Marketing memastikan halaman penting tidak hilang |
| 6 | Buffer stok website | Nilai awal buffer per SKU = rata-rata order website harian SKU tersebut selama 30 hari terakhir × 1 hari, dibulatkan ke atas, minimal 1 unit untuk SKU dengan stok ≤ 10. Dihitung ulang setiap minggu dan bisa ditimpa Sales E-commerce | Melindungi website dari oversell akibat jeda impor order marketplace manual, tanpa menahan stok terlalu banyak |
| 7 | Versi aplikasi untuk fase dengan beberapa titik go-live | Setiap titik go-live menaikkan satu versi minor (asumsi #2), dan dicatat di Log Keputusan Roadmap (D-29) | Setiap go-live butuh versi yang bisa ditelusuri dan di-rollback sendiri |

### 15.2 Isu Terbuka

Isu yang berkaitan dengan integrasi pihak ketiga diputuskan saat persiapan integrasi tersebut.

| # | Isu | Kapan diputuskan | PIC |
|---|---|---|---|
| 1 | Kurir Biteship yang diaktifkan dan paket Biteship | Sebelum UAT R2.2 | Sales E-commerce |
| 2 | Kepastian vendor vending soal SKU di data transaksi dan format kode mesin di `order_id` Midtrans | Sebelum build R2.3 | Admin Vending |

---

## 16. Keterlacakan

### 16.1 Catatan Balans ke Fitur

| Catatan | Fitur PRD |
|---|---|
| Divisi 2 (Sales: e-commerce, CS, reseller, vending) | F2-104 s.d. F2-108, F2-201 s.d. F2-216, F2-301 s.d. F2-308 |
| #1 Stok Non-Ecommerce/MT/Reseller | F2-106 |
| #5 Vending di lokasi konsinyasi | F2-301 s.d. F2-308 |
| #7 WA/IG, kurir pilihan | F2-210, F2-211 |
| #8 Data customer | F2-101 s.d. F2-103 |
| #15 Diskon karyawan | F2-207 |
| #16 – #17 Diskon & model harga | F2-105, F2-202, F2-206 |
| #18 Website e-commerce + payment gateway | F2-201 s.d. F2-206, F2-209, F2-214, F2-215 |
| #19 WA/IG diarahkan ke website | F2-210 |

### 16.2 Requirement BRD ke Rilis

| Modul | R2.1 | R2.2 | R2.3 |
|---|---|---|---|
| CRM | CRM-01, 02, 03 | | |
| RSL | RSL-01, 02, 03, 04, 07 | RSL-05 | |
| INV | INV-07, 08 | | |
| ECM | | ECM-01 s.d. ECM-09, ECM-15 | |
| LOG | | LOG-05, 06 | |
| WMS | | WMS-05 | |
| INT | | INT-01 (website & link bayar), INT-05 | INT-01 (vending) |
| ACC | ACC-02 (A-05 reseller, A-34) | ACC-02 (A-21, A-22), ACC-06 Midtrans | ACC-02 (A-09, A-23 vending) |
| VND | | | VND-01 s.d. VND-05, VND-07, VND-08 |
| RPT | | | RPT-07 |
| Opsional | | SLS-11 kuota (U-31) | VND-09 (U-32) |

### 16.3 Dokumen Pendukung

| Topik | Dokumen |
|---|---|
| Lingkup, urutan, gate pihak ketiga | Roadmap v0.6 bagian 7 dan 13 |
| Proses bisnis website, reseller, vending, harga | BRD v0.7 bagian 5.6, 5.8 – 5.10, 5.13, 5.18 |
| COA dan posting rule A-09, A-21, A-22, A-34 | Desain Finance & Accounting Fase 1 |
| Role, permission, matriks approval | Desain Role & Approval Fase 1 |
| Webhook, adapter mock, storefront, feature flag | Technical Guideline v0.1 bagian 5.10, 9, 10.11, 14.5 |
| User story dan estimasi | Backlog Fase 2 |
| Deploy, backup, restore, secret | Runbook Operasi Balans Platform |

---

## 17. Persetujuan

Dengan menandatangani bagian ini, pihak berikut menyetujui isi, kebutuhan, ukuran keberhasilan, dan syarat go-live Fase 2 sebagaimana dijabarkan di dokumen ini.

| Nama | Jabatan | Tanda Tangan | Tanggal |
|---|---|---|---|
| | Balans Nutrition — Direksi | | |
| | Balans Nutrition — PIC Utama Proyek | | |
| | Balans Nutrition — Sales (E-commerce, Reseller & Vending) | | |
| | Balans Nutrition — Marketing | | |
| | Balans Nutrition — Finance & Accounting | | |
| | Balans Nutrition — Logistik | | |
| | Tim Implementasi — Project Manager | | |
| | Tim Implementasi — Tech Lead | | |
