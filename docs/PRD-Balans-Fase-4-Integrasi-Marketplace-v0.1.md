# Product Requirements Document (PRD) — Fase 4
## Balans Nutrition — Unified Business Platform: Integrasi Marketplace

| Item | Keterangan |
|---|---|
| Klien | Balans Nutrition |
| Dokumen | Product Requirements Document Fase 4 — Integrasi Marketplace |
| Versi | 0.1 — Draft |
| Tanggal | 17 Oktober 2026 |
| Acuan | BRD Balans Unified Business Platform v0.7 (bagian 5.7, 5.11, 6.5, 6.6, 6.13, 7.2, 8, 9, 12 — U-21, U-25); Roadmap v0.6 (bagian 4, 9, 13 – 15, D-13); PRD Fase 1 v0.1; PRD Fase 2 v0.1; Desain Finance & Accounting Fase 1; Technical Guideline v0.1 (bagian 5.10, 9.3, job `marketplace.*`) |
| Status | Draft untuk direview tim implementasi dan Balans |

PRD ini menjabarkan **apa yang harus dihasilkan Fase 4**, perilaku yang diharapkan dari setiap fitur, kriteria penerimaannya, dan syarat go-live tiap gelombang. Rincian per user story disusun di Backlog Fase 4 setelah PRD ini disetujui. Sesuai Roadmap (D-18), dokumen ini **tidak memuat target tanggal**. Yang ditetapkan adalah isi, urutan, dan syarat selesai.

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
10. [Integrasi & Laporan](#10-integrasi--laporan)
11. [Persiapan, UAT & Go-live](#11-persiapan-uat--go-live)
12. [Entry, Exit & Definition of Done](#12-entry-exit--definition-of-done)
13. [Risiko & Mitigasi](#13-risiko--mitigasi)
14. [Asumsi & Batasan](#14-asumsi--batasan)
15. [Keputusan & Isu Terbuka](#15-keputusan--isu-terbuka)
16. [Keterlacakan](#16-keterlacakan)
17. [Persetujuan](#17-persetujuan)

---

## 1. Ringkasan

Fase 4 menggantikan input manual dan impor Excel penjualan marketplace dari Fase 1 dengan **integrasi langsung ke tiap marketplace** (arahan Balans P-21, U-21). Dengan integrasi aktif, sistem:

- menarik order,
- mengirim stok tersedia,
- mengambil label/AWB,
- memperbarui status order,
- menarik data settlement untuk rekonsiliasi otomatis.

Proses gudang, pengakuan penjualan, dan jurnal tetap sama dengan Fase 1. Yang berubah hanya cara data masuk.

Integrasi dibagi **dua gelombang** (D-13): marketplace dengan volume terbesar lebih dulu, lalu marketplace lainnya, sehingga pelajaran dari gelombang 1 bisa dipakai di gelombang 2.

| Rilis | Marketplace (usulan) | Hasil yang bisa didemokan | Requirement |
|---|---|---|---|
| **R4.1** Gelombang 1 | Shopee; TikTok Shop & Tokopedia | Order dua marketplace masuk otomatis, stok tersinkron, AWB dicetak dari ERP, status dan retur tersinkron, settlement terekonsiliasi otomatis | INT-04 (gelombang 1), ACC-06 otomatis |
| **R4.2** Gelombang 2 | Lazada; Blibli | Kemampuan yang sama untuk marketplace lainnya; dashboard eksekutif lanjutan | INT-04 (gelombang 2), ACC-06 otomatis, RPT-14 lanjutan |

Fase 4 hanya memuat **satu requirement baru**, yaitu INT-04 (Should). Requirement ini mencakup 4 – 5 integrasi terpisah. ACC-06 dan RPT-14 sudah dihitung di Fase 1 dan diselesaikan bertahap di fase ini (Roadmap bagian 14). Urutan gelombang final ditetapkan dari data penjualan marketplace sejak go-live Fase 1. Jumlah integrasi terpisah dipastikan saat registrasi, karena Tokopedia dan TikTok Shop sudah berada di satu platform seller.

---

## 2. Latar Belakang & Masalah

1. **Order marketplace dicatat dua kali.** Sales E-commerce memproses order di seller center, lalu mencatat atau mengimpornya lagi ke ERP (SLS-05). Saat volume naik, terutama di tanggal kampanye, pencatatan tertinggal dan stok di ERP tidak lagi real-time.
2. **Stok di marketplace diubah manual.** E-commerce Stock dipakai bersama oleh website dan semua marketplace (BRD 5.7). Tanpa sinkronisasi, stok yang tampil di marketplace bisa lebih besar dari stok fisik, sehingga order dibatalkan karena barang habis dan penilaian toko turun.
3. **AWB dan status dikelola di banyak seller center.** Gudang membuka tiap seller center untuk mencetak label, dan status pengiriman di ERP tidak terbarui otomatis.
4. **Rekonsiliasi settlement masih lewat impor.** Rekonsiliasi tahap 1 (F1-402) memakai file laporan settlement yang formatnya berbeda per marketplace dan bisa berubah sewaktu-waktu. Fee, voucher penjual, dan promo baru terlihat setelah file diolah.
5. **Dashboard eksekutif masih versi dasar.** Manajemen belum bisa melihat margin per marketplace setelah fee dan promo secara langsung (RPT-14 dasar).

---

## 3. Tujuan & Ukuran Keberhasilan

Ukuran keberhasilan diukur selama tiga bulan setelah go-live setiap gelombang, per marketplace yang sudah terintegrasi. Nilai target adalah usulan tim implementasi dan disahkan bersama PRD ini.

| ID | Tujuan | Ukuran keberhasilan | Acuan |
|---|---|---|---|
| G4-01 | Order marketplace masuk tanpa input ulang | 100% order dari toko terintegrasi tercatat otomatis; 0 order diinput manual kecuali saat integrasi gangguan; order baru tercatat di ERP ≤ 15 menit setelah dibayar pembeli | INT-04 |
| G4-02 | Overselling hilang | Pembatalan oleh penjual karena stok habis ≤ 0,5% dari order marketplace | BRD 5.7 |
| G4-03 | Stok marketplace sesuai ERP | Perubahan stok tersedia terkirim ke marketplace ≤ 5 menit; sinkron penuh harian tanpa selisih yang tidak terjelaskan | INT-04 |
| G4-04 | Settlement terekonsiliasi otomatis | ≥ 98% order yang dananya cair cocok otomatis tanpa intervensi; saldo Piutang Marketplace per toko di akhir bulan terjelaskan per order | ACC-06 |
| G4-05 | Pengiriman lebih cepat | Order yang diproses sebelum batas jam pickup harian dikirim di hari yang sama; label/AWB dicetak massal dari ERP | LOG-02, LOG-06 |
| G4-06 | Profitabilitas marketplace terlihat | Margin per marketplace dan per toko setelah fee, promo, dan voucher penjual tersedia di dashboard eksekutif tanpa olah data di luar sistem | RPT-14, OBJ-08 |

---

## 4. Ruang Lingkup

### 4.1 Dalam Lingkup

| Kode | Modul | Fase 4 mencakup |
|---|---|---|
| INT | Integrasi | INT-04: integrasi langsung Shopee, TikTok Shop & Tokopedia (R4.1), Lazada, Blibli (R4.2). Kemampuan per marketplace: koneksi toko, tarik order, sinkron stok, label/AWB, status order, retur/pembatalan, data settlement |
| SLS | Sales Management | SLS-05 berpindah dari input manual/impor ke integrasi per toko; impor Excel tetap tersedia sebagai cadangan |
| LOG | Logistics | LOG-02 dan LOG-06: resi dan AWB marketplace diambil otomatis dan dicetak dari ERP |
| ACC | Finance & Accounting | ACC-06 otomatis: rekonsiliasi settlement marketplace dari API; jurnal A-10, A-10s, A-13 tetap sesuai Desain Finance & Accounting |
| RPT | Laporan | RPT-14 lanjutan (R4.2): dashboard eksekutif lanjutan; laporan integrasi dan settlement per toko |

### 4.2 Di Luar Lingkup Fase 4

- **Pembuatan dan pengubahan listing produk, harga, dan foto di marketplace.** Listing tetap dikelola di seller center. Sistem hanya memetakan listing ke SKU Balans.
- **Pengelolaan promo, voucher, iklan, dan chat marketplace.** Semua tetap di seller center. Dampak finansialnya masuk lewat data order dan settlement.
- **Stok di gudang milik marketplace** (layanan fulfillment oleh marketplace). Jika Balans memakainya, gudang tersebut ditambahkan sebagai lokasi eksternal lewat change request.
- **Marketplace di luar lima nama di BRD.** Marketplace baru ditambah lewat change request dengan pola adapter yang sama.
- **Penarikan order historis sebelum tanggal aktivasi integrasi per toko.** Order lama tetap seperti yang sudah dicatat manual atau diimpor.
- HRIS lengkap (Fase 5) dan seluruh butir BRD 2.2.

---

## 5. Pengguna & Role

Fase 4 tidak menambah role. Akses berikut ditambahkan ke role yang sudah ada.

| Role | Pekerjaan baru di Fase 4 |
|---|---|
| Sales E-commerce | Menghubungkan toko, memetakan listing ke SKU, mengatur stok pengaman per channel, memantau order tertahan dan error sinkron |
| Staf Gudang, Fulfillment Leader | Memproses order marketplace di Outbound, mencetak AWB massal, menandai siap kirim |
| Customer Service | Menangani retur dan pembatalan marketplace yang tersinkron |
| Finance & Accounting | Memantau rekonsiliasi settlement otomatis dan menyelesaikan selisih |
| Head Finance & Accounting | Menyetujui penghapusan selisih settlement di atas batas |
| Admin Sistem | Mengelola kredensial aplikasi marketplace, memantau kesehatan integrasi |
| Direksi | Dashboard eksekutif lanjutan |

**Key user** Fase 4: Sales E-commerce, Logistik, Finance & Accounting, ditambah PIC utama proyek.

---

## 6. Kebutuhan Fungsional per Rilis

Fitur di R4.1 dibangun sebagai **kemampuan umum integrasi marketplace** (F4-101 s.d. F4-110) dengan adapter per marketplace. R4.2 menambah adapter Lazada dan Blibli di atas kemampuan yang sama, ditambah dashboard eksekutif lanjutan. Kriteria penerimaan berlaku untuk setiap marketplace yang diaktifkan.

### 6.1 R4.1 — Gelombang 1: Shopee; TikTok Shop & Tokopedia

| ID | Fitur | Kriteria penerimaan utama | Req |
|---|---|---|---|
| F4-101 | Koneksi toko | Satu marketplace bisa punya lebih dari satu toko; toko dihubungkan lewat otorisasi resmi marketplace oleh pemilik akun Balans; token disimpan terenkripsi dan diperbarui otomatis sebelum kedaluwarsa; status koneksi (aktif, perlu otorisasi ulang, gangguan) tampil per toko; peringatan ke Sales E-commerce dan Admin Sistem 7 hari sebelum otorisasi kedaluwarsa | INT-04 |
| F4-102 | Pemetaan listing ke SKU | Setiap listing dan varian marketplace dipetakan ke satu SKU Balans atau satu bundel; saran pemetaan otomatis dari kode SKU penjual yang sama; listing tanpa pemetaan tidak bisa disinkron stoknya, dan ordernya masuk antrian *Perlu Pemetaan* tanpa memotong stok; pemetaan bisa diimpor dan diekspor lewat Excel | INT-04, MD-03 |
| F4-103 | Tarik order | Order masuk lewat webhook marketplace (jika tersedia) dan polling berkala (5 – 15 menit) sebagai pengaman; nomor order eksternal unik per toko, sehingga order tidak tercatat ganda walau datang dari webhook, polling, dan impor; order dibayar → Sales Order marketplace dengan alokasi FEFO dari E-commerce Stock; bundel dipecah ke komponen; harga, voucher penjual, dan subsidi tercatat dari data order, bukan dari resolver (BRD 5.18) | INT-04, SLS-05 |
| F4-104 | Sinkron stok | Stok tersedia per listing = E-commerce Stock − stok yang sudah dialokasikan − stok pengaman channel, dan untuk bundel = min(stok komponen ÷ qty); perubahan stok dikirim dalam ≤ 5 menit dengan penggabungan perubahan beruntun; sinkron penuh harian; stok pengaman per channel dan per SKU bisa diatur Sales E-commerce; sinkron per listing bisa dihentikan sementara | INT-04 |
| F4-105 | Label/AWB & siap kirim | Permintaan pengiriman (pickup atau drop-off) dikirim ke marketplace setelah QC Outbound lulus; AWB/label diambil otomatis dan dicetak massal dari ERP per gelombang pickup; nomor resi tersimpan di DO; batas waktu kirim dari marketplace tampil di antrian Outbound dan order yang mendekati batas ditandai | LOG-02, LOG-06, WMS-12 |
| F4-106 | Sinkron status order | Status marketplace (dibayar, siap kirim, dikirim, diterima, selesai, dibatalkan) dipetakan ke status DO dan Sales Order; status *dikirim* (barang diserahkan ke kurir) memicu pengakuan penjualan dan HPP lewat jurnal A-10 dan A-06 (U-25); pembatalan sebelum diserahkan ke kurir hanya melepas alokasi stok | INT-04, U-25 |
| F4-107 | Retur & pembatalan | Pengajuan retur atau refund dari marketplace tersinkron ke antrian Retur; barang yang kembali masuk Inbound → Gudang Retur (alur Fase 1); jurnal A-13 mengurangi Piutang Marketplace; status sengketa dan keputusan marketplace tercatat; retur yang diputuskan tanpa barang kembali dicatat sebagai potongan di settlement | SLS-08, INT-04 |
| F4-108 | Settlement otomatis | Data settlement per order ditarik dari API: nilai bruto, komisi, biaya layanan, biaya pembayaran, ongkir yang ditanggung penjual, voucher penjual, promo, dan nilai bersih; dicocokkan per order dengan Piutang Marketplace; jurnal dana cair A-10s per batch pencairan; dana cair dicocokkan dengan mutasi bank lewat rekonsiliasi bank (F1-401); selisih per order tampil di antrian F&A dan penghapusan selisih di atas batas butuh approval Head F&A | ACC-06 |
| F4-109 | Monitor integrasi | Dashboard per toko: waktu sinkron terakhir order, stok, dan settlement; jumlah order tertahan, listing tanpa pemetaan, error API, dan webhook gagal; retry otomatis dengan backoff dan retry manual; alert jika sinkron order berhenti > 30 menit di jam operasional | INT-04, NFR-06 |
| F4-110 | Aktivasi per toko & cadangan manual | Integrasi diaktifkan per toko dengan tanggal mulai; order sebelum tanggal itu tetap dari input manual/impor; mode *bayangan* menarik order tanpa memotong stok atau membuat jurnal, untuk dibandingkan dengan ekspor seller center sebelum aktivasi; jika integrasi gangguan, impor Excel (F1-305) tetap bisa dipakai dan dedup nomor order mencegah data ganda saat integrasi pulih | SLS-05, INT-02 |

**Deliverable R4.1:** dua marketplace dengan volume terbesar berjalan otomatis dari order sampai settlement, dan impor manual tidak lagi dipakai untuk toko-tokonya.

### 6.2 R4.2 — Gelombang 2: Lazada; Blibli

| ID | Fitur | Kriteria penerimaan utama | Req |
|---|---|---|---|
| F4-201 | Adapter Lazada | Kemampuan F4-101 s.d. F4-110 berjalan untuk toko Lazada | INT-04 |
| F4-202 | Adapter Blibli | Kemampuan F4-101 s.d. F4-110 berjalan untuk toko Blibli | INT-04 |
| F4-203 | Rekonsiliasi settlement semua marketplace | Seluruh marketplace terekonsiliasi otomatis; ringkasan bulanan per marketplace dan per toko: penjualan bruto, fee, promo, voucher penjual, retur, dana cair, dan saldo Piutang Marketplace | ACC-06 |
| F4-204 | Dashboard eksekutif lanjutan | Penjualan, margin kotor, dan margin setelah fee/promo per channel, marketplace, toko, produk, dan periode; tren dan perbandingan periode; stok cover (hari) per SKU; stok mendekati kedaluwarsa; kas dan umur piutang; drill-down dari angka ringkasan ke laporan dan dokumen; tampilan HP untuk Direksi; data dari materialized view yang di-refresh job dan menampilkan waktu refresh terakhir | RPT-14 |

**Deliverable R4.2:** semua marketplace di BRD terintegrasi langsung, dan dashboard eksekutif lanjutan tersedia.

---

## 7. Aturan Bisnis Lintas Modul

Aturan Fase 1 – 3 tetap berlaku. Fase 4 menambah aturan berikut.

| # | Aturan | Acuan |
|---|---|---|
| 1 | **Satu nomor order eksternal per toko hanya tercatat sekali**, apa pun jalurnya (webhook, polling, impor) | SLS-05, TG 5.10 |
| 2 | **Satu pool E-commerce Stock untuk website dan semua marketplace**, dikurangi stok pengaman per channel | BRD 5.7 |
| 3 | **Harga order marketplace diambil dari data marketplace**, bukan dari resolver harga. Price list marketplace hanya acuan internal | BRD 5.18 |
| 4 | **Penjualan diakui saat barang diserahkan ke kurir.** Pembatalan sebelum itu hanya melepas alokasi stok | U-25, A-10 |
| 5 | **Order dengan listing yang belum dipetakan tidak memotong stok** dan tertahan sampai dipetakan | F4-102 |
| 6 | **Integrasi tidak pernah membuat stok negatif.** Jika stok fisik tidak cukup untuk order yang masuk, order ditahan dengan notifikasi dan diproses sesuai aturan stok kurang di Outbound (F1-308) | PRD Fase 1 aturan #3 |
| 7 | **Payload marketplace diverifikasi sebelum diproses** (signature webhook) dan status penting dikonfirmasi ulang lewat API | TG 9.3 |
| 8 | **Fee, promo, dan voucher penjual dicatat dari data settlement aktual**, bukan dari perkiraan | A-10s |
| 9 | **Impor Excel tetap tersedia sebagai cadangan** untuk setiap toko, dengan format yang sama dengan Fase 1 | BRD 5.7 |

**Approval baru di Fase 4:**

| Dokumen | Level 1 | Level 2 |
|---|---|---|
| Penghapusan selisih settlement per order di atas batas (nilai awal Rp100.000) | Finance & Accounting | Head Finance & Accounting |

---

## 8. Alur End-to-End & Skenario UAT

Skenario dijalankan per marketplace di staging dengan toko uji (sandbox) dari marketplace. Untuk marketplace yang tidak menyediakan sandbox, skenario dijalankan di mode bayangan pada toko production (F4-110).

| ID | Rilis | Skenario | Divisi penguji | Hasil yang diperiksa |
|---|---|---|---|---|
| UAT4-01 | R4.1, R4.2 | Hubungkan toko → otorisasi kedaluwarsa (disimulasikan) → otorisasi ulang | Sales E-commerce | Status koneksi, peringatan, sinkron pulih |
| UAT4-02 | R4.1, R4.2 | Pemetaan listing tunggal, varian, dan bundel; order untuk listing belum dipetakan | Sales E-commerce | Saran pemetaan, order tertahan tanpa potong stok |
| UAT4-03 | R4.1, R4.2 | Order dibayar → masuk ERP → alokasi FEFO → Outbound dengan QC → AWB dicetak massal → diserahkan ke kurir → diterima → selesai | Sales E-commerce, Logistik | Waktu masuk ≤ 15 menit, jurnal A-10 + A-06 saat dikirim, status tersinkron |
| UAT4-04 | R4.1, R4.2 | Order yang sama datang dari webhook, polling, dan impor Excel | Sales E-commerce | Tercatat sekali |
| UAT4-05 | R4.1, R4.2 | Penerimaan barang, penjualan website, dan order marketplace mengubah stok → stok di marketplace diperbarui; stok pengaman channel diubah | Sales E-commerce | Stok terkirim ≤ 5 menit, angka bundel benar |
| UAT4-06 | R4.1, R4.2 | Pembeli membatalkan sebelum kirim; penjual menolak karena stok kurang | Sales E-commerce | Alokasi dilepas, tidak ada jurnal penjualan |
| UAT4-07 | R4.1, R4.2 | Retur setelah diterima → barang kembali → Inbound → Gudang Retur; refund tanpa barang kembali | CS, Logistik, F&A | Jurnal A-13, potongan di settlement |
| UAT4-08 | R4.1, R4.2 | Settlement satu periode termasuk fee, voucher penjual, ongkir ditanggung penjual, dan retur → pencairan → rekonsiliasi bank | F&A | Jurnal A-10s, selisih per order, dana cair cocok bank |
| UAT4-09 | R4.1, R4.2 | Integrasi diputus (disimulasikan) selama kampanye → impor cadangan → integrasi pulih | Sales E-commerce, Tech | Alert ≤ 30 menit, tidak ada order ganda |
| UAT4-10 | R4.1, R4.2 | Mode bayangan satu minggu → bandingkan dengan ekspor seller center → aktivasi | Sales E-commerce | Selisih jumlah dan nilai order 0 |
| UAT4-11 | R4.2 | Dashboard eksekutif lanjutan untuk satu periode, termasuk drill-down sampai dokumen | Direksi, F&A | Angka cocok dengan laporan keuangan dan laba rugi per dimensi |
| UAT4-12 | R4.1, R4.2 | Uji beban kampanye: lonjakan order di satu toko | Tech | Target bagian 9 tercapai, ERP tetap responsif |

---

## 9. Kebutuhan Non-Fungsional

| NFR | Kebutuhan Fase 4 | Cara uji |
|---|---|---|
| NFR-02 Keamanan | Kredensial aplikasi dan token toko terenkripsi dan hanya bisa dikelola Admin Sistem; verifikasi signature webhook; akses ke halaman koneksi toko butuh re-autentikasi | Review kode + test |
| NFR-04 Privasi | Data pembeli marketplace (nama, alamat, HP) hanya dipakai untuk pengiriman dan disimpan sesuai ketentuan tiap marketplace; data yang disamarkan marketplace tidak diupayakan dibuka; masuk CRM hanya sejauh diizinkan (CRM-01) | Review sesuai kebijakan data tiap marketplace |
| NFR-06 Ketersediaan | Monitor sinkron order, stok, dan settlement per toko; alert jika sinkron berhenti > 30 menit di jam operasional | UAT4-09 |
| NFR-09 Skalabilitas | Toko dan marketplace baru ditambah lewat konfigurasi dan adapter tanpa mengubah modul penjualan, gudang, dan akuntansi | Review arsitektur di R4.2 |
| NFR-10 Kapasitas | 5 marketplace (NFR-10 BRD); ≥ 3.000 order marketplace per hari saat kampanye tanpa menurunkan kinerja ERP; job `marketplace.*` di queue `default`/`bulk` tidak menunda queue `critical`; patuh batas laju API tiap marketplace | Uji beban UAT4-12 |

---

## 10. Integrasi & Laporan

### 10.1 Integrasi

| Marketplace | Gelombang | Kemampuan | Catatan |
|---|---|---|---|
| Shopee | R4.1 (usulan) | Order, stok, label/AWB, status, retur, settlement | Aplikasi didaftarkan di platform developer Shopee atas nama Balans |
| TikTok Shop & Tokopedia | R4.1 (usulan) | Sama | Satu platform seller; jumlah integrasi terpisah dipastikan saat registrasi |
| Lazada | R4.2 (usulan) | Sama | Aplikasi didaftarkan di platform developer Lazada |
| Blibli | R4.2 (usulan) | Sama | Akses API seller diajukan ke Blibli |
| Bank (F1-401) | R4.1 | Dana cair marketplace dicocokkan dengan mutasi bank | Impor mutasi Fase 1 |

Setiap adapter dikembangkan dengan **adapter mock** yang memakai jalur webhook dan job yang sama dengan adapter asli (D-27, TG 5.10), sehingga pengembangan tidak menunggu persetujuan akses API.

### 10.2 Laporan

| ID | Laporan | Pengguna utama |
|---|---|---|
| RPT-14 lanjutan | Dashboard eksekutif lanjutan (F4-204) | Direksi, Manajemen |
| RPT-05 (perluasan) | Penjualan per marketplace dan per toko | Sales E-commerce |
| RPT-15 (perluasan) | Laba rugi per sub-channel marketplace setelah fee dan promo | F&A, Manajemen |
| Rekonsiliasi settlement | Per toko per periode: bruto, fee, promo, retur, dana cair, belum cair, selisih | F&A |
| Kesehatan integrasi | Waktu sinkron, error, order tertahan, listing tanpa pemetaan per toko | Sales E-commerce, Admin Sistem |
| Ketepatan kirim | Order per batas waktu kirim marketplace, terlambat, dibatalkan karena stok | Sales E-commerce, Logistik |

---

## 11. Persiapan, UAT & Go-live

### 11.1 Tahapan

| Urutan | Tahap | Aktivitas | Selesai jika |
|---|---|---|---|
| 1 | Persiapan (paralel dengan Fase 3) | Registrasi aplikasi integrasi di tiap marketplace atas nama Balans; pengajuan akses API; penetapan urutan gelombang dari data penjualan; pemetaan awal listing ke SKU lewat Excel | Akses API gelombang 1 disetujui; pemetaan listing gelombang 1 ≥ 95% |
| 2 | Build R4.1 | Kemampuan umum F4-101 s.d. F4-110 dengan adapter mock, lalu adapter gelombang 1 | Story R4.1 memenuhi Definition of Done |
| 3 | UAT R4.1 | UAT4-01 s.d. UAT4-10 dan UAT4-12 per marketplace gelombang 1 | Semua skenario lulus dan ditandatangani |
| 4 | Mode bayangan | Integrasi menarik order tanpa memotong stok, paralel dengan impor manual, minimal satu minggu per toko | Selisih order 0 terhadap ekspor seller center |
| 5 | Aktivasi | Per toko, di awal hari di luar tanggal kampanye; impor manual dihentikan untuk toko tersebut | Go/no-go terpenuhi |
| 6 | Hypercare | Pemantauan harian sampai satu siklus settlement penuh terekonsiliasi | Settlement bulan pertama terekonsiliasi |
| 7 | R4.2 | Ulangi tahap 2 – 6 untuk Lazada dan Blibli, ditambah dashboard eksekutif lanjutan (UAT4-11) | Exit R4.2 |

### 11.2 Go / No-Go per Toko

1. Tidak ada defect kritis yang terbuka; defect tinggi punya workaround tertulis yang disetujui key user.
2. Mode bayangan selesai tanpa selisih order yang tidak terjelaskan.
3. ≥ 99% listing aktif toko sudah dipetakan.
4. Stok pengaman per channel sudah terhitung (bagian 15.1 #1) dan ditinjau Sales E-commerce.
5. Monitor dan alert integrasi aktif dan pernah dipicu uji.
6. Adapter mock nonaktif untuk toko tersebut di production.
7. Aktivasi tidak jatuh dalam 3 hari sebelum tanggal kampanye besar marketplace.

### 11.3 Rencana Mundur

Jika integrasi satu toko bermasalah dan tidak bisa diperbaiki dalam 4 jam, sinkron toko tersebut dinonaktifkan, stok di seller center diatur manual, dan order dicatat lewat impor Excel seperti Fase 1. Dedup nomor order mencegah data ganda saat integrasi diaktifkan kembali. Rollback aplikasi mengikuti Runbook Operasi.

---

## 12. Entry, Exit & Definition of Done

### 12.1 Entry Criteria

| # | Kriteria | Berlaku untuk |
|---|---|---|
| 1 | PRD Fase 4 disetujui; Backlog Fase 4 terestimasi | Fase 4 |
| 2 | Fase 1 sudah go-live; penjualan marketplace manual/impor dan rekonsiliasi settlement tahap 1 berjalan minimal dua periode tutup buku | R4.1 |
| 3 | Akses seller center tiap marketplace dan urutan gelombang ditetapkan (Roadmap bagian 13) | Persiapan |
| 4 | Akses API marketplace gelombang terkait disetujui | Build adapter asli R4.1 dan R4.2 |

Fase 4 hanya bergantung pada Fase 1 (Roadmap bagian 4). Urutan terhadap Fase 3 dan Fase 5 diputuskan steering committee.

### 12.2 Exit per Rilis

| Rilis | Exit criteria |
|---|---|
| R4.1 | Semua story R4.1 memenuhi Definition of Done; UAT gelombang 1 lulus; semua toko gelombang 1 aktif; settlement bulan pertama terekonsiliasi otomatis; F&A memvalidasi jurnal A-10, A-10s, A-13 dari data integrasi |
| R4.2 | Semua story R4.2 memenuhi Definition of Done; UAT gelombang 2 dan UAT4-11 lulus; semua toko gelombang 2 aktif; settlement bulan pertama terekonsiliasi otomatis |

### 12.3 Definition of Done Story

Sama dengan Fase 1 (PRD Fase 1 bagian 12.3). Untuk story adapter ditambah: contract test terhadap contoh payload resmi marketplace, test idempotency untuk setiap jalur masuk data, dan test perilaku saat batas laju API tercapai.

### 12.4 Exit Fase 4

Fase 4 ditutup di steering committee setelah semua marketplace di BRD terintegrasi, ukuran keberhasilan G4-01, G4-02, dan G4-04 tercapai untuk setiap marketplace, dan daftar defect tidak memuat defect kritis.

---

## 13. Risiko & Mitigasi

| # | Risiko | Dampak | Mitigasi |
|---|---|---|---|
| 1 | Persetujuan akses API marketplace lambat atau ditolak | Gelombang tertunda | Registrasi paralel dengan Fase 3; build dengan adapter mock; impor Excel tetap jalan (Roadmap risiko #6) |
| 2 | API marketplace berubah atau versi lama dihentikan | Sinkron gagal | Adapter terpisah per marketplace; contract test; monitor error; pemeliharaan integrasi dianggarkan setelah fase selesai |
| 3 | Batas laju API tercapai saat kampanye | Order dan stok terlambat sinkron | Penggabungan perubahan stok; antrian dengan backoff; sinkron penuh di luar jam sibuk |
| 4 | Pemetaan listing tidak lengkap atau salah | Stok salah potong, oversell | Antrian *Perlu Pemetaan*; saran otomatis; go/no-go ≥ 99% listing terpetakan; laporan listing tanpa pemetaan |
| 5 | Format atau komponen settlement berbeda antar marketplace | Rekonsiliasi tidak cocok | Pemetaan komponen fee per marketplace ke akun; antrian selisih; validasi F&A di siklus settlement pertama |
| 6 | Data pembeli disamarkan marketplace | CRM dan pengiriman terbatas | Pengiriman memakai label dari marketplace; CRM hanya memakai data yang tersedia |
| 7 | Stok fisik berbeda dari stok sistem | Oversell walau sinkron berjalan | Stok pengaman per channel; cycle count E-commerce Stock; scan barcode (R3.3) |
| 8 | Toko Tokopedia dan TikTok Shop berubah struktur platformnya | Jumlah dan bentuk integrasi berubah | Dipastikan saat registrasi; adapter dirancang per platform seller, bukan per nama marketplace |

---

## 14. Asumsi & Batasan

1. Semua akun seller center dan aplikasi integrasi atas nama Balans dan diurus Balans (BRD 11.11).
2. Balans memakai pengiriman yang diatur marketplace (kurir pilihan marketplace dengan AWB dari marketplace). Pengiriman mandiri untuk order marketplace tidak dipakai.
3. Satu toko per marketplace di awal; toko tambahan didukung lewat konfigurasi.
4. Komponen biaya settlement dipetakan ke akun yang sudah ada di COA Fase 1 (bagian 15.1 #2), dengan akun Piutang Marketplace per marketplace (121x).
5. Batas penghapusan selisih settlement per order Rp100.000 adalah nilai awal yang bisa diubah.
6. Polling 5 – 15 menit (job `marketplace.sync_orders`) tetap berjalan walau webhook tersedia, sebagai pengaman jika webhook hilang.
7. Versi aplikasi naik minor di setiap gelombang (PRD Fase 2 asumsi #2). Aktivasi per toko memakai pengaturan, bukan rilis baru.
8. Dashboard eksekutif lanjutan memakai data hingga refresh terakhir (nilai awal setiap jam), bukan real-time.

---

## 15. Keputusan & Isu Terbuka

### 15.1 Keputusan

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| 1 | Stok pengaman per channel | Nilai awal per SKU per marketplace = rata-rata order harian SKU di marketplace tersebut selama 30 hari terakhir × 1 hari, dibulatkan ke atas, minimal 1 unit untuk SKU dengan stok ≤ 10. Dihitung ulang setiap minggu dan bisa ditimpa Sales E-commerce. Rumus yang sama dipakai untuk buffer website (PRD Fase 2 bagian 15.1 #6) | Satu rumus untuk semua channel yang memakai E-commerce Stock; Sales E-commerce tidak perlu mengisi nilai per SKU per toko sebelum aktivasi |
| 2 | Pemetaan komponen biaya settlement ke akun | Komisi, biaya layanan, dan biaya pembayaran → 6105 Beban Fee Marketplace. Promo dan co-funding yang ditanggung penjual serta biaya iklan yang dipotong dari settlement → 6107 Beban Promosi Marketplace. Voucher penjual → 4102 Potongan Penjualan (A-10). Ongkir yang ditanggung penjual → 6108 Beban Pengiriman Penjualan. Refund tanpa barang kembali → 4103 Retur Penjualan. Komponen baru yang belum dikenal masuk antrian F&A untuk dipetakan sekali, lalu berlaku otomatis | Semua akun sudah ada di COA Fase 1; pemetaan per komponen membuat margin per marketplace setelah biaya bisa dihitung langsung |
| 3 | KPI dashboard eksekutif lanjutan | Penjualan bruto dan neto; margin kotor dan margin kontribusi (setelah fee, promo, voucher, dan ongkir) per channel, marketplace, toko, produk, dan periode; nilai stok, stok cover (hari) per SKU, dan stok mendekati kedaluwarsa; saldo kas & bank, umur piutang dan utang; target vs pencapaian (RPT-16). Workshop dengan Direksi sebelum build R4.2 hanya untuk menyepakati tata letak, bukan isi KPI | KPI diturunkan dari tujuan bisnis OBJ-01, OBJ-03, dan OBJ-08 serta data yang sudah tersedia di sistem |

### 15.2 Isu Terbuka

Isu yang berkaitan dengan pihak ketiga diputuskan saat persiapan Fase 4.

| # | Isu | Kapan diputuskan | PIC |
|---|---|---|---|
| 1 | Urutan final gelombang | Dari data penjualan marketplace tiga bulan setelah go-live Fase 1 | Sales E-commerce, PM |
| 2 | Jumlah integrasi terpisah untuk Tokopedia dan TikTok Shop | Saat registrasi aplikasi | Tech Lead |
| 3 | Apakah Balans memakai gudang fulfillment milik marketplace | Sebelum build R4.1; jika ya, ditambah sebagai lokasi eksternal lewat change request | Sales E-commerce, Logistik |

---

## 16. Keterlacakan

### 16.1 Sumber Kebutuhan ke Fitur

| Sumber | Fitur PRD |
|---|---|
| Catatan #6 (marketplace → E-commerce Stock → DO + barcode marketplace) | F4-103 s.d. F4-106 |
| Catatan #23, arahan Balans P-21 / U-21 (integrasi langsung) | F4-101 s.d. F4-110, F4-201, F4-202 |
| U-25 (penjualan diakui saat serah ke kurir) | F4-106 |
| BRD 5.7 (stok pengaman per channel, pemetaan SKU) | F4-102, F4-104 |
| ACC-06 (settlement otomatis) | F4-108, F4-203 |
| RPT-14 lanjutan, OBJ-08 | F4-204 |

### 16.2 Requirement BRD ke Rilis

| Modul | R4.1 | R4.2 |
|---|---|---|
| INT | INT-04 gelombang 1 | INT-04 gelombang 2 |
| ACC | ACC-06 otomatis (gelombang 1) | ACC-06 otomatis (semua marketplace) |
| SLS, LOG | SLS-05, LOG-02, LOG-06 lewat integrasi | Sama |
| RPT | | RPT-14 lanjutan |

### 16.3 Dokumen Pendukung

| Topik | Dokumen |
|---|---|
| Lingkup, gelombang, persiapan | Roadmap v0.6 bagian 9, D-13 |
| Proses marketplace | BRD v0.7 bagian 5.7 |
| Penjualan marketplace manual/impor, settlement tahap 1 | PRD Fase 1 (F1-305, F1-402) |
| Akun Piutang Marketplace, A-10, A-10s | Desain Finance & Accounting Fase 1 |
| Adapter, webhook, job `marketplace.*` | Technical Guideline v0.1 |
| User story dan estimasi | Backlog Fase 4 |

---

## 17. Persetujuan

Dengan menandatangani bagian ini, pihak berikut menyetujui isi, kebutuhan, ukuran keberhasilan, dan syarat go-live Fase 4 sebagaimana dijabarkan di dokumen ini.

| Nama | Jabatan | Tanda Tangan | Tanggal |
|---|---|---|---|
| | Balans Nutrition — Direksi | | |
| | Balans Nutrition — PIC Utama Proyek | | |
| | Balans Nutrition — Sales E-commerce | | |
| | Balans Nutrition — Finance & Accounting | | |
| | Balans Nutrition — Logistik | | |
| | Tim Implementasi — Project Manager | | |
| | Tim Implementasi — Tech Lead | | |
