# Product Requirements Document (PRD) — Fase 3
## Balans Nutrition — Unified Business Platform: E-commerce Lanjutan & Sales Tracking

| Item | Keterangan |
|---|---|
| Klien | Balans Nutrition |
| Dokumen | Product Requirements Document Fase 3 — E-commerce Lanjutan & Sales Tracking |
| Versi | 0.1 — Draft |
| Tanggal | 17 Oktober 2026 |
| Acuan | BRD Balans Unified Business Platform v0.7 (bagian 5.6, 5.9 – 5.11, 5.14, 5.17, 5.19, 6.3, 6.7 – 6.11, 7.2, 8, 9, 12); Roadmap v0.6 (bagian 4, 8, 11, 13 – 15); PRD Fase 1 v0.1; PRD Fase 2 v0.1; Desain Finance & Accounting Fase 1; Desain Role & Approval Fase 1; Technical Guideline v0.1; Naming Convention Menu & Fitur v1.0 |
| Status | Draft untuk direview tim implementasi dan Balans |

PRD ini menjabarkan **apa yang harus dihasilkan Fase 3**, perilaku yang diharapkan dari setiap fitur, kriteria penerimaannya, dan syarat go-live tiap rilis. Rincian per user story disusun di Backlog Fase 3 setelah PRD ini disetujui. Sesuai Roadmap (D-18), dokumen ini **tidak memuat target tanggal**. Yang ditetapkan adalah isi, urutan, dan syarat selesai.

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
11. [UAT, Training & Go-live](#11-uat-training--go-live)
12. [Entry, Exit & Definition of Done](#12-entry-exit--definition-of-done)
13. [Risiko & Mitigasi](#13-risiko--mitigasi)
14. [Asumsi & Batasan](#14-asumsi--batasan)
15. [Keputusan & Isu Terbuka](#15-keputusan--isu-terbuka)
16. [Keterlacakan](#16-keterlacakan)
17. [Persetujuan](#17-persetujuan)

---

## 1. Ringkasan

Fase 3 melengkapi website sesuai arahan **fitur e-commerce lengkap** (U-16), menjalankan **tracking sales** (U-19), dan menambahkan otomasi yang bergantung pada pihak ketiga: WhatsApp Business API, API vendor vending, dan scan barcode di gudang. Fase ini tidak membuka channel baru. Seluruh fiturnya memperkuat channel, data customer, dan proses gudang yang sudah berjalan sejak Fase 1 dan Fase 2.

Ketiga rilis dikerjakan paralel: **R3.1 oleh track website**, **R3.2 dan R3.3 oleh track ERP** (Roadmap bagian 8). Setiap rilis punya titik go-live sendiri dan bergantung pada rilis Fase 2 yang berbeda (Roadmap bagian 4).

| Rilis | Fokus | Hasil yang bisa didemokan di staging | Bergantung pada | Requirement | Must / Should / Could |
|---|---|---|---|---|---|
| **R3.1** E-commerce Lanjutan | Retensi customer website | Subscription dengan pengingat dan link bayar, poin loyalitas, ulasan, wishlist & keranjang tertinggal, referral, retur online, evaluasi level reseller otomatis | R2.2 | 8 | 1 / 3 / 4 |
| **R3.2** Sales Pipeline & Target | Tracking sales | Lead sampai deal untuk partner MT/konsinyasi, reseller, dan B2B; aktivitas dan check-in kunjungan; target vs pencapaian dari invoice; interaksi CS | R2.1 | 7 | 2 / 3 / 2 |
| **R3.3** Otomasi Komunikasi & Gudang | Integrasi pihak ketiga | Invoice, link bayar, dan notifikasi lewat WhatsApp; status stok dan mesin dari API vendor vending; scan barcode di Inbound, transfer, picking, dan stock count | R2.3 | 4 | 0 / 2 / 2 |
| **Fase 3** | | | | **19** | **3 / 8 / 8** |

Hitungan requirement mengikuti Roadmap bagian 14. RPT dihitung sebagai Must. Usulan opsional ACC-23 (anggaran biaya per divisi) masuk R3.2 hanya jika dipilih di BRD v1.0.

---

## 2. Latar Belakang & Masalah

1. **Website sudah menjual, tetapi belum membuat customer kembali.** Produk nutrisi seperti meal shake dikonsumsi rutin, namun setelah launch R2.2 belum ada mekanisme langganan, poin, atau referral untuk mendorong pembelian ulang (U-16).
2. **Kepercayaan calon pembeli belum dibangun di website.** Belum ada ulasan dari pembeli terverifikasi. Keranjang yang ditinggalkan dan produk yang disimpan juga belum ditindaklanjuti.
3. **Retur website masih lewat CS.** Customer harus menghubungi CS untuk mengajukan retur, dan CS mencatatnya ulang di ERP (PRD Fase 2 asumsi #8).
4. **Aktivitas sales tidak terlacak.** Pendekatan ke calon partner MT, reseller, dan B2B dicatat di luar sistem. Manajemen tidak tahu posisi tiap prospek dan pencapaian sales terhadap target (arahan Balans P-19).
5. **Level reseller dievaluasi manual.** Admin Reseller menghitung omzet 3 bulan secara manual untuk mengusulkan naik atau turun level.
6. **Komunikasi ke customer masih disalin manual ke WhatsApp.** Invoice, link bayar, dan notifikasi dikirim CS satu per satu, padahal WhatsApp adalah channel utama customer Balans (Catatan #7).
7. **Gangguan mesin vending baru diketahui saat refill.** Tanpa data dari vendor, mesin yang offline atau slot yang kosong baru terlihat saat partner datang.
8. **Salah ambil barang di gudang masih mungkin terjadi.** Picking dan stock count mengandalkan pembacaan SKU dan batch secara visual (WMS-06).

---

## 3. Tujuan & Ukuran Keberhasilan

Ukuran keberhasilan diukur selama tiga bulan setelah go-live setiap rilis. Nilai target adalah usulan tim implementasi dan disahkan bersama PRD ini.

| ID | Tujuan | Ukuran keberhasilan | Acuan |
|---|---|---|---|
| G3-01 | Pembelian ulang di website naik | Persentase customer website yang belanja ≥ 2 kali dalam 90 hari naik ≥ 20% (relatif) dibanding tiga bulan sebelum R3.1 | ECM-10, ECM-11 |
| G3-02 | Subscription berjalan | ≥ 80% siklus subscription aktif dibayar dari link bayar sebelum tanggal kirim | ECM-10 |
| G3-03 | Keranjang tertinggal dipulihkan | ≥ 5% keranjang tertinggal yang menerima pengingat berakhir dengan order lunas | ECM-13 |
| G3-04 | Retur website mandiri | ≥ 90% retur order website diajukan dari akun customer; seluruhnya selesai (refund atau tolak) ≤ 7 hari kerja setelah barang diterima di Inbound | ECM-16 |
| G3-05 | Aktivitas sales tercatat | 100% sales MT, reseller, dan B2B mencatat lead dan aktivitas di sistem; 100% partner atau customer baru hasil deal dibuat dari pipeline tanpa input ulang | SLP-01, SLP-02 |
| G3-06 | Target vs pencapaian otomatis | Laporan target vs pencapaian per sales dan per channel tersedia setiap bulan tanpa olah data di luar sistem; angka pencapaian cocok dengan invoice | SLP-03, RPT-16 |
| G3-07 | Komunikasi WhatsApp otomatis | ≥ 95% invoice, link bayar, dan notifikasi order yang memenuhi syarat terkirim otomatis lewat WhatsApp; status terkirim tercatat | INT-06 |
| G3-08 | Akurasi gudang naik | Selisih picking (salah SKU atau batch) yang tertangkap di QC Outbound turun ≥ 50% dibanding tiga bulan sebelum R3.3 | WMS-06 |

---

## 4. Ruang Lingkup

### 4.1 Dalam Lingkup

| Kode | Modul | Fase 3 mencakup |
|---|---|---|
| ECM | Web E-commerce | ECM-10 s.d. ECM-14, ECM-16 (R3.1): subscription, poin, ulasan, wishlist & keranjang tertinggal, referral, retur online |
| RSL | Reseller | RSL-06 (R3.1): usulan naik/turun level otomatis |
| SLP | Sales Pipeline | SLP-01 s.d. SLP-04 (R3.2): pipeline, aktivitas, target, check-in kunjungan |
| CRM | CRM | CRM-04 (R3.2): pencatatan interaksi CS |
| INT | Integrasi | INT-06 WhatsApp Business API, INT-03 API vendor vending (R3.3) |
| VND | Vending Machine | VND-06 (R3.3): status stok dan status mesin dari API vendor |
| WMS | Warehouse | WMS-06 (R3.3): scan barcode |
| ACC | Finance & Accounting | Posting rule A-35 penukaran poin; refund retur online memakai A-13 |
| RPT | Laporan | RPT-20 (R3.1); RPT-13, RPT-16 (R3.2) |

**Usulan opsional** yang masuk Fase 3 hanya jika dipilih di BRD v1.0 (Roadmap bagian 11):

| Usulan | Requirement | Rilis | Isi |
|---|---|---|---|
| Anggaran biaya per divisi (bagian dari U-13) | ACC-23 | R3.2 | Anggaran biaya bulanan per divisi dan jenis biaya, realisasi dari jurnal, peringatan saat realisasi mendekati batas |

### 4.2 Di Luar Lingkup Fase 3

- **Integrasi langsung marketplace** dan rekonsiliasi settlement marketplace otomatis (INT-04, ACC-06 otomatis; Fase 4). Persiapan registrasi aplikasi di seller center dimulai paralel dengan Fase 3 (Roadmap bagian 9), tetapi bukan bagian dari PRD ini.
- **HRIS lengkap dan payroll** (HR-04 s.d. HR-12; Fase 5), termasuk pengiriman slip gaji lewat WhatsApp.
- **Pendebitan otomatis untuk subscription.** Subscription memakai pengingat dan link bayar per periode (BRD 5.9).
- **Aplikasi mobile native** untuk sales atau gudang (BRD 2.2). Check-in kunjungan dan scan barcode berjalan di browser HP.
- **Chatbot atau inbox WhatsApp dua arah.** INT-06 hanya mengirim pesan keluar. Percakapan dengan customer tetap di WhatsApp CS.
- **Kampanye broadcast massal lewat WhatsApp.** Fase 3 hanya mengirim pesan transaksional dan pengingat yang dipicu oleh transaksi customer.

---

## 5. Pengguna & Role

Role Fase 1 dan Fase 2 tetap berlaku. Fase 3 tidak menambah jenis pengguna luar, tetapi memperluas akses beberapa role.

| Role | Pekerjaan baru di Fase 3 |
|---|---|
| Customer website | Mengelola subscription, melihat dan menukar poin, menulis ulasan, wishlist, membagikan kode referral, mengajukan retur |
| Reseller (portal) | Melihat progres omzet terhadap syarat level berikutnya |
| Sales E-commerce | Moderasi ulasan, pengaturan program poin dan referral, paket subscription, pemantauan keranjang tertinggal |
| Customer Service | Verifikasi pengajuan retur online, pencatatan interaksi CS, pemantauan status kirim WhatsApp |
| Sales MT, Admin Reseller, Sales B2B | Lead, aktivitas, check-in kunjungan, deal |
| Head Sales | Target per sales dan channel, approval usulan level reseller otomatis, pemantauan pipeline |
| Admin Vending | Pemantauan status mesin dan slot dari API vendor |
| Staf Gudang, Fulfillment Leader, Merchandiser | Scan barcode di Inbound, transfer, picking, QC Outbound, stock count |
| Head Divisi, Head F&A | Anggaran divisi (jika ACC-23 diambil) |

**Sales B2B** belum punya role tersendiri di Fase 1. Pipeline B2B dipegang Sales MT sampai Balans menunjuk PIC B2B, dan role dibentuk lewat konfigurasi tanpa pengembangan (bagian 15.1 #4).

**Key user** Fase 3: Sales E-commerce/CS, Marketing, Sales MT, Reseller & Vending, Logistik, ditambah PIC utama proyek.

---

## 6. Kebutuhan Fungsional per Rilis

Setiap fitur di bawah dipecah menjadi user story di Backlog Fase 3. Kriteria penerimaan di tabel adalah perilaku minimal yang diuji di sprint review dan UAT. **Prioritas** mengikuti BRD: M = Must, S = Should, C = Could. Jika kapasitas tidak cukup, requirement C digeser lebih dulu, lalu S, lewat keputusan steering committee.

### 6.1 R3.1 — E-commerce Lanjutan

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F3-101 | Subscription | Paket subscription per SKU atau bundel dengan interval 2 minggu, 4 minggu, atau bulanan; diskon subscription sebagai price list promo yang dikelola Sales E-commerce; pengingat H-3 lewat email (dan WhatsApp setelah R3.3) berisi link bayar Midtrans; stok dipesan saat link dibayar; siklus yang tidak dibayar sampai tanggal kirim dilewati, dan setelah 2 siklus berturut-turut tidak dibayar subscription dijeda otomatis; customer bisa ubah qty, lewati siklus, jeda, atau berhenti dari akunnya | ECM-10 | S |
| F3-102 | Poin loyalitas | Poin diperoleh dari nilai belanja bersih (setelah diskon, tanpa ongkir) dengan rasio yang bisa diatur; poin baru bisa dipakai setelah order *Diterima* dan masa retur lewat; poin ditukar sebagai potongan di checkout dengan batas persentase per order; masa berlaku poin (nilai awal 12 bulan) dan pengingat sebelum hangus; poin dari order yang diretur ditarik kembali; mutasi poin per customer; jurnal A-35 saat poin ditukar tanpa pencadangan | ECM-11 | S |
| F3-103 | Ulasan & rating | Hanya pembeli yang order SKU tersebut sudah *Diterima* yang bisa memberi rating 1 – 5, teks, dan foto; ulasan tayang setelah dimoderasi Sales E-commerce; tolak wajib alasan; balasan resmi Balans; rata-rata rating dan jumlah ulasan tampil di halaman produk dan structured data | ECM-12 | C |
| F3-104 | Wishlist & keranjang tertinggal | Customer login bisa menyimpan produk ke wishlist; keranjang customer login yang tidak dibayar dianggap tertinggal setelah 4 jam; pengingat dikirim 4 jam dan 24 jam setelahnya hanya ke customer yang menyetujui pemasaran; pengingat berhenti jika customer sudah order; notifikasi produk wishlist yang kembali tersedia | ECM-13 | C |
| F3-105 | Referral | Setiap customer punya kode referral; customer baru yang memakai kode mendapat voucher order pertama; pemberi referral mendapat poin atau voucher setelah order customer baru *Diterima* dan masa retur lewat; referral tidak dihitung jika nomor HP, alamat, atau metode bayar sama dengan pemberi referral; batas reward per bulan | ECM-14 | C |
| F3-106 | Retur & refund online | Customer mengajukan retur dari akunnya paling lambat 7 hari setelah *Diterima*, dengan alasan dari master retur, qty per baris, dan foto; CS memverifikasi (setuju atau tolak dengan alasan); retur disetujui → pengiriman balik lewat Biteship atau diantar customer → Inbound → Gudang Retur (alur Fase 1); refund lewat Midtrans atau transfer setelah QC retur; jurnal A-13; status retur tampil di akun customer dan dikirim lewat email | ECM-16 | S |
| F3-107 | Evaluasi level reseller otomatis | Job tiap 3 bulan menghitung omzet reseller dari invoice bersih (dikurangi nota kredit) 3 bulan terakhir; usulan naik atau turun level dibuat otomatis sesuai syarat level dan dikirim ke Head Sales untuk approval; reseller melihat progres omzet terhadap syarat level berikutnya di portal; level yang turun tidak berlaku untuk order yang sudah dibayar | RSL-06 | C |
| F3-108 | Laporan e-commerce | Penjualan website per periode, produk, dan sumber (UTM); pemakaian voucher dan poin; subscriber aktif, siklus dibayar, dan churn; keranjang tertinggal dan yang dipulihkan; referral dan reward | RPT-20 | M |

**Deliverable R3.1:** fitur website lanjutan live untuk semua customer; level reseller dievaluasi otomatis; laporan e-commerce tersedia.

### 6.2 R3.2 — Sales Pipeline & Target

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F3-201 | Pipeline lead → deal | Lead untuk partner MT/konsinyasi, reseller, dan B2B/corporate dengan pemilik (sales), sumber, perkiraan nilai, dan tahap *Lead → Kualifikasi → Presentasi/Sampling → Negosiasi → Deal* atau *Lost* dengan alasan wajib dari master; tampilan kanban dan daftar; lead ganda dicegah lewat nomor HP, email, atau NPWP; deal membuat master partner atau customer dari data lead tanpa input ulang (partner MT/konsinyasi tetap wajib PKS sebelum DO konsinyasi, F1-112) | SLP-01 | S |
| F3-202 | Aktivitas sales | Kunjungan, telepon, follow-up, catatan, dan sampling per lead atau partner; follow-up terjadwal dengan pengingat in-app; sampling membuat permintaan pemakaian internal dengan tujuan sampling sales dan kode lead (F1-215, U-19); riwayat aktivitas tampil di halaman lead dan partner | SLP-02 | S |
| F3-203 | Target & pencapaian | Target bulanan per sales dan per channel yang ditetapkan Head Sales; pemilik customer atau partner ditetapkan per sales; pencapaian dihitung otomatis dari invoice bersih (dikurangi nota kredit) pada periode invoice, termasuk invoice konsinyasi hasil rekonsiliasi; dashboard pencapaian harian per sales | SLP-03 | S |
| F3-204 | Check-in kunjungan | Check-in dari browser HP menyimpan lokasi GPS, waktu, dan foto; jarak ke alamat partner atau lead dihitung dan ditandai jika > 500 m; lokasi hanya diambil saat check-in, tidak dilacak terus-menerus | SLP-04, NFR-04 | C |
| F3-205 | Interaksi CS | Catatan interaksi per customer: kategori (keluhan, pertanyaan, saran), channel (WA, IG, email, telepon, website), order terkait, status *Terbuka → Ditangani → Selesai*, dan PIC; keluhan terbuka lebih dari 2 hari kerja masuk pengingat; riwayat tampil di halaman customer CRM | CRM-04 | C |
| F3-206 | Laporan pipeline | Jumlah dan nilai lead per tahap, per jenis, per sales; conversion rate per tahap; alasan lost; umur lead per tahap | RPT-13 | M |
| F3-207 | Laporan target vs pencapaian | Target, pencapaian, dan persentase per sales dan per channel per bulan, dengan drill-down ke invoice | RPT-16 | M |

**Deliverable R3.2:** seluruh tim sales mencatat lead dan aktivitas di sistem, dan target vs pencapaian bulan pertama tersedia otomatis.

### 6.3 R3.3 — Otomasi Komunikasi & Gudang

**Prasyarat:** akun WhatsApp Business atas nama Balans lewat penyedia resmi (BSP) dengan nomor terverifikasi dan template pesan yang disetujui Meta; jawaban vendor vending soal ketersediaan API (U-05).

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F3-301 | WhatsApp Business API | Pesan keluar berbasis template untuk: invoice WA/IG dan reseller (link PDF), link bayar Midtrans, pembayaran diterima, pesanan dikirim dengan resi, pesanan diterima, pengingat subscription, pengingat keranjang tertinggal; pesan pemasaran hanya ke customer yang menyetujui; status terkirim, dibaca, dan gagal dari webhook tercatat per pesan; pesan gagal otomatis dikirim lewat email; pengiriman lewat adapter dengan mock seperti Midtrans dan Biteship (D-27) | INT-06 | C |
| F3-302 | API vendor vending | Status mesin (online, offline, error) dan stok per slot ditarik dari API vendor secara berkala atau diterima lewat webhook; mesin offline > 1 jam dan slot kosong memicu notifikasi ke Admin Vending dan partner; stok slot dari vendor ditampilkan berdampingan dengan stok sistem, dan stok sistem tetap mengikuti refill dan transaksi (F2-302, F2-305) | VND-06, INT-03 | S |
| F3-303 | Scan barcode | Scan dengan kamera HP di browser atau scanner USB/Bluetooth (mode keyboard) di Inbound, transfer, picking, QC Outbound, dan stock count; barcode produk dari master (MD-01) dan barcode batch dari label produk; scan SKU atau batch yang tidak sesuai dokumen ditolak dengan pesan dan bunyi; input manual tetap tersedia dengan alasan | WMS-06 | C |

**Deliverable R3.3:** komunikasi transaksional ke customer terkirim otomatis lewat WhatsApp, status mesin vending terpantau (jika vendor menyediakan API), dan proses gudang utama memakai scan barcode.

Jika vendor vending tidak menyediakan API, F3-302 ditutup dengan berita acara keputusan steering committee dan fallback U-05 tetap berlaku. Requirement VND-06 dan INT-03 dicatat sebagai *tidak tersedia dari vendor*, bukan sebagai defect.

---

## 7. Aturan Bisnis Lintas Modul

Aturan Fase 1 dan Fase 2 tetap berlaku. Fase 3 menambah aturan berikut.

| # | Aturan | Acuan |
|---|---|---|
| 1 | **Poin, reward referral, dan pencapaian target hanya dihitung dari transaksi final**, yaitu order *Diterima* yang sudah lewat masa retur, atau invoice bersih setelah nota kredit | ECM-11, ECM-14, SLP-03 |
| 2 | **Poin dicatat di akuntansi saat ditukar**, sebagai potongan penjualan tanpa pencadangan saat poin diberikan | A-35, Desain F&A aturan #6 |
| 3 | **Urutan potongan di checkout:** harga dari resolver → diskon → voucher → poin. Satu order memakai satu voucher | BRD 5.18 |
| 4 | **Subscription tidak mendebit otomatis.** Setiap siklus dibayar lewat link bayar, dan stok baru dipesan saat link dibayar | BRD 5.9 |
| 5 | **Hanya pembeli terverifikasi yang bisa memberi ulasan**, dan ulasan tayang setelah moderasi | ECM-12 |
| 6 | **Retur online mengikuti alur retur Fase 1** (Inbound → Gudang Retur → QC → disposisi). Refund baru diproses setelah QC retur | BRD 5.11, ECM-16 |
| 7 | **Perubahan level reseller selalu lewat approval Head Sales**, baik dari usulan otomatis maupun manual | RSL-06, BRD 5.17 |
| 8 | **Deal dari pipeline tidak melewati kontrol master.** Partner konsinyasi tetap wajib PKS berlaku, dan customer baru tetap dicek ganda lewat nomor HP | SLP-01, MD-07, CRM-01 |
| 9 | **Pemasaran lewat email dan WhatsApp hanya ke customer yang menyetujui.** Pesan transaksional boleh dikirim ke semua customer yang bertransaksi | NFR-04 (UU PDP) |
| 10 | **Lokasi sales hanya diambil saat check-in** dan hanya bisa dilihat sales itu sendiri, Head Sales, dan Direksi | NFR-04 |
| 11 | **Data vendor vending tidak mengubah stok sistem secara langsung.** Stok sistem tetap mengikuti refill dan transaksi; data vendor menjadi pembanding dan pemicu peringatan | VND-06 |
| 12 | **Scan barcode memvalidasi, bukan menggantikan dokumen.** Setiap scan harus cocok dengan baris dokumen; input manual wajib alasan dan tercatat di audit trail | WMS-06 |

**Approval baru di Fase 3** (melengkapi Desain Role & Approval; batas dan approver bisa dikonfigurasi):

| Dokumen | Level 1 | Level 2 |
|---|---|---|
| Usulan perubahan level reseller otomatis | Head Sales | — |
| Target bulanan per sales dan channel | Head Sales | Direksi (target per channel) |
| Program poin dan referral (rasio, batas, masa berlaku) | Head Sales | Head Finance & Accounting |
| Anggaran biaya per divisi (jika ACC-23 diambil) | Head Divisi | Head Finance & Accounting |

---

## 8. Alur End-to-End & Skenario UAT

Setiap skenario dinyatakan lulus jika dokumen, stok, jurnal, dan laporan terkait sesuai hasil yang diharapkan, dan key user menandatanganinya.

| ID | Rilis | Skenario | Divisi penguji | Hasil yang diperiksa |
|---|---|---|---|---|
| UAT3-01 | R3.1 | Customer berlangganan bulanan → pengingat H-3 → bayar link → kirim; siklus berikutnya tidak dibayar dua kali → jeda otomatis; customer melanjutkan lagi | Sales E-commerce | Reservasi saat bayar, status subscription, harga subscription |
| UAT3-02 | R3.1 | Order *Diterima* → poin masuk setelah masa retur → poin ditukar di checkout bersama voucher → order lain diretur → poin ditarik | Sales E-commerce, F&A | Mutasi poin, urutan potongan, jurnal A-35 |
| UAT3-03 | R3.1 | Pembeli terverifikasi dan bukan pembeli mencoba memberi ulasan → moderasi → tayang dengan balasan | Sales E-commerce | Penolakan non-pembeli, rating di halaman produk |
| UAT3-04 | R3.1 | Keranjang tertinggal oleh customer yang setuju dan tidak setuju pemasaran → pengingat → order | Marketing | Pengingat hanya ke customer yang setuju, pengingat berhenti setelah order |
| UAT3-05 | R3.1 | Customer A membagikan kode → customer B order pertama dengan voucher referral → diterima → reward A; uji referral dengan nomor HP yang sama | Marketing, F&A | Reward setelah masa retur, penolakan referral palsu |
| UAT3-06 | R3.1 | Pengajuan retur online dengan foto → CS setuju → kirim balik Biteship → Inbound → QC → refund sebagian Midtrans | CS, Logistik, F&A | Status di akun customer, jurnal A-13, refund |
| UAT3-07 | R3.1 | Job evaluasi level → usulan naik dan turun → approval Head Sales → harga order berikutnya | Reseller, Head Sales | Omzet bersih 3 bulan, harga level baru |
| UAT3-08 | R3.2 | Lead MT → kualifikasi → sampling (pemakaian internal) → negosiasi → deal → partner dan PKS dibuat → DO konsinyasi pertama | Sales MT | Partner dari lead, jurnal sampling dengan kode lead, PKS wajib |
| UAT3-09 | R3.2 | Lead reseller dan B2B → lost dengan alasan; lead ganda dengan nomor HP sama | Sales | Alasan lost wajib, pencegahan duplikat |
| UAT3-10 | R3.2 | Check-in kunjungan di lokasi partner dan > 500 m dari lokasi | Sales MT | Lokasi, foto, penanda jarak |
| UAT3-11 | R3.2 | Target bulanan → invoice MT, reseller, dan konsinyasi → nota kredit retur → laporan target vs pencapaian | Head Sales, F&A | Pencapaian bersih cocok dengan invoice |
| UAT3-12 | R3.2 | Keluhan customer dari WA → ditangani → selesai; keluhan terbuka > 2 hari kerja | CS | Riwayat di CRM, pengingat |
| UAT3-13 | R3.3 | Order WA/IG → invoice dan link bayar lewat WhatsApp → lunas → notifikasi dikirim dan diterima; nomor tidak valid → fallback email | CS | Status per pesan, fallback |
| UAT3-14 | R3.3 | Mesin offline dan slot kosong dari API vendor (atau simulasi mock) | Vending | Notifikasi Admin Vending dan partner, stok sistem tidak berubah |
| UAT3-15 | R3.3 | Picking dengan scan SKU dan batch yang salah, lalu yang benar; stock count dengan scan; input manual dengan alasan | Logistik | Penolakan scan salah, audit input manual |
| UAT3-16 | Semua | Uji akses: customer membuka subscription dan poin customer lain; sales membuka lokasi check-in sales lain | Semua | API menolak 403/404 |

---

## 9. Kebutuhan Non-Fungsional

NFR Fase 1 dan Fase 2 tetap berlaku. Fase 3 menambahkan atau memperketat kebutuhan berikut.

| NFR | Kebutuhan Fase 3 | Cara uji |
|---|---|---|
| NFR-01 Platform | Kanban pipeline nyaman di tablet; check-in, scan barcode, dan form aktivitas nyaman di HP dengan target sentuh ≥ 44 px | Playwright di 3 lebar layar; uji kamera di HP Android dan iPhone |
| NFR-02 Keamanan | Verifikasi signature webhook WhatsApp dan vendor vending; rate limit pengajuan retur, ulasan, dan klaim referral per akun | Test API |
| NFR-04 Privasi | Persetujuan pemasaran dihormati di email dan WhatsApp; lokasi check-in hanya untuk role yang berhak; foto ulasan dan retur tidak memuat metadata lokasi (EXIF dihapus saat upload) | Review kode + test |
| NFR-06 Ketersediaan | Job pengingat subscription, keranjang tertinggal, dan evaluasi level punya monitor job gagal dan alert | Uji alert |
| NFR-10 Kapasitas | Pengiriman WhatsApp lewat job queue dengan batas laju sesuai tier akun WhatsApp; ≥ 5.000 pesan per hari tanpa menunda job `critical` | Uji beban job |
| NFR-11 Notifikasi | Notifikasi in-app untuk follow-up sales, approval level reseller, keluhan terbuka, dan status mesin | UAT3-07, UAT3-12, UAT3-14 |

---

## 10. Integrasi, Dokumen Cetak & Laporan

### 10.1 Integrasi

| Integrasi | Fase 3 | Catatan |
|---|---|---|
| WhatsApp Business API (INT-06) | R3.3: pesan template keluar dan webhook status | Lewat BSP resmi atas nama Balans; biaya per percakapan mengikuti tarif Meta dan BSP |
| API vendor vending (INT-03) | R3.3: status mesin dan stok slot | Bergantung kemampuan vendor (U-05) |
| Midtrans | R3.1: link bayar subscription, refund retur online | Akun dan integrasi R2.2 |
| Biteship | R3.1: pengiriman balik retur online | Akun dan integrasi R2.2 |
| Email transaksional | R3.1: pengingat subscription, keranjang tertinggal, poin hangus, status retur | Provider Fase 1 |

### 10.2 Dokumen Cetak & Label

Fase 3 menambah formulir retur online (PDF untuk ditempel di paket kiriman balik) dan label lokasi rak dengan barcode untuk scan di gudang (R3.3).

### 10.3 Laporan

| ID | Laporan | Pengguna utama |
|---|---|---|
| RPT-13 | Sales pipeline: jumlah dan nilai per tahap, conversion rate, alasan lost | Sales, Manajemen |
| RPT-16 | Target vs pencapaian per sales dan per channel | Sales, Manajemen |
| RPT-20 | Laporan e-commerce: penjualan website, voucher, poin, subscriber aktif, keranjang tertinggal, referral | Sales E-commerce, Marketing |
| RPT-09 (perluasan) | Retur online per alasan, waktu penyelesaian, refund | CS, Logistik |
| Aktivitas sales | Kunjungan, check-in, follow-up per sales | Head Sales |
| Interaksi CS | Keluhan per kategori, waktu penyelesaian | CS, Manajemen |
| Pesan WhatsApp | Terkirim, dibaca, gagal, fallback email per jenis pesan | CS |

---

## 11. UAT, Training & Go-live

### 11.1 Tahapan per Rilis

Fase 3 tidak membutuhkan cut-over atau migrasi saldo. Data yang dimigrasikan hanya data awal yang belum ada di sistem.

| Rilis | Tahap | Aktivitas | Selesai jika |
|---|---|---|---|
| R3.1 | Build & UAT | Sprint R3.1 oleh track website; UAT3-01 s.d. UAT3-07 | Semua skenario lulus dan ditandatangani |
| | Konfigurasi & go-live | Program poin, referral, dan paket subscription disetujui; syarat & ketentuan program tayang; rilis ke production | Go/no-go R3.1 terpenuhi |
| R3.2 | Build & UAT | Sprint R3.2 oleh track ERP; UAT3-08 s.d. UAT3-12 | Semua skenario lulus |
| | Migrasi data | Lead dan prospek aktif dari catatan sales diimpor lewat template; pemilik customer/partner per sales ditetapkan; target bulan pertama diinput | Data diimpor tanpa error |
| | Training & go-live | Training sales per tim; rilis di awal bulan agar target dan pencapaian bulan pertama utuh | Go/no-go R3.2 terpenuhi |
| R3.3 | Persiapan | Akun WhatsApp Business dan template disetujui; label rak dicetak; scanner atau HP gudang diuji | Template berstatus disetujui Meta |
| | Build & UAT | Sprint R3.3; UAT3-13 s.d. UAT3-15 | Semua skenario lulus |
| | Go-live bertahap | WhatsApp: mulai dari notifikasi order, lalu invoice dan link bayar; scan barcode: mulai dari picking, lalu Inbound, transfer, dan stock count | Semua jenis pesan dan proses gudang aktif |

### 11.2 Go / No-Go

**Umum:** tidak ada defect kritis yang terbuka; defect tinggi punya workaround tertulis yang disetujui key user; key user rilis menandatangani UAT; user terkait sudah ditraining; backup dan uji restore terakhir berhasil.

| Rilis | Butir khusus |
|---|---|
| R3.1 | Syarat & ketentuan poin, referral, dan subscription disetujui Balans dan tayang; F&A memvalidasi jurnal A-35 dan refund retur online |
| R3.2 | Pemilik customer/partner per sales lengkap; target bulan pertama disetujui |
| R3.3 | Nomor WhatsApp Business terverifikasi dan template disetujui; adapter mock WhatsApp nonaktif di production; fallback email teruji |

### 11.3 Rencana Mundur

- **R3.1:** setiap fitur lanjutan punya pengaturan aktif/nonaktif. Jika bermasalah, fitur dinonaktifkan tanpa mengganggu checkout. Poin dan subscription yang sudah tercatat tetap tersimpan.
- **R3.3:** jika pengiriman WhatsApp gagal, semua pesan otomatis lewat email dan CS kembali mengirim manual. Jika scan barcode mengganggu operasional gudang, proses kembali ke input manual dengan alasan *scan tidak tersedia*.
- Rollback aplikasi ke versi sebelumnya mengikuti Runbook Operasi.

---

## 12. Entry, Exit & Definition of Done

### 12.1 Entry Criteria

| # | Kriteria | Berlaku untuk |
|---|---|---|
| 1 | PRD Fase 3 disetujui; Backlog Fase 3 terestimasi | Fase 3 |
| 2 | R2.2 sudah launch publik dan stabil minimal satu siklus settlement Midtrans | R3.1 |
| 3 | R2.1 sudah go-live; daftar sales dan pemilik customer/partner tersedia | R3.2 |
| 4 | R2.3 sudah go-live; akun WhatsApp Business lewat BSP atas nama Balans tersedia; jawaban vendor vending soal API | R3.3 |
| 5 | Keputusan steering committee soal urutan Fase 3 – 5 dan opsi paralel Fase 5 (Roadmap bagian 10) | Fase 3 |

### 12.2 Exit per Rilis

| Rilis | Exit criteria |
|---|---|
| R3.1 | Semua story R3.1 memenuhi Definition of Done; UAT3-01 s.d. UAT3-07 lulus; go-live R3.1 selesai |
| R3.2 | Semua story R3.2 memenuhi Definition of Done; UAT3-08 s.d. UAT3-12 lulus; laporan target vs pencapaian bulan pertama tersedia |
| R3.3 | Semua story R3.3 memenuhi Definition of Done; UAT3-13 s.d. UAT3-15 lulus; semua jenis pesan WhatsApp dan proses scan aktif |

### 12.3 Definition of Done Story

Sama dengan Fase 1 dan Fase 2 (PRD Fase 1 bagian 12.3, PRD Fase 2 bagian 12.3). Untuk story yang mengirim pesan ke customer, ditambah test bahwa customer tanpa persetujuan pemasaran tidak menerima pesan pemasaran.

### 12.4 Exit Fase 3

Fase 3 ditutup di steering committee setelah ketiga rilis live, ukuran keberhasilan G3-02, G3-05, dan G3-06 tercapai, dan daftar defect tidak memuat defect kritis.

---

## 13. Risiko & Mitigasi

| # | Risiko | Dampak | Mitigasi |
|---|---|---|---|
| 1 | Program poin dan referral disalahgunakan | Margin turun | Reward setelah masa retur; deteksi HP, alamat, dan metode bayar yang sama; batas reward per bulan; program bisa dinonaktifkan |
| 2 | Customer lupa membayar siklus subscription | Subscription tidak memberi pendapatan rutin | Pengingat H-3 lewat email dan WhatsApp; jeda otomatis, bukan batal, agar customer mudah melanjutkan |
| 3 | Approval template atau verifikasi WhatsApp Business lambat | R3.3 tertunda | Pengajuan akun dan template dimulai sejak build R3.3; fallback email |
| 4 | Biaya percakapan WhatsApp membengkak | Biaya operasional naik | Hanya pesan transaksional dan pengingat terpicu; laporan jumlah pesan per jenis; batas pengiriman dapat dikonfigurasi |
| 5 | Vendor vending tidak menyediakan API | VND-06 tidak bisa dipenuhi | Fallback U-05 tetap berlaku; requirement ditutup lewat keputusan steering committee |
| 6 | Sales tidak disiplin mencatat aktivitas | Pipeline dan laporan tidak akurat | Pencapaian otomatis dari invoice; Head Sales meninjau pipeline mingguan; form HP singkat |
| 7 | Kamera HP gudang tidak membaca barcode dengan baik | Scan lambat | Opsi scanner USB/Bluetooth; label barcode dengan ukuran dan kontras standar; input manual dengan alasan |
| 8 | Ulasan negatif atau tidak pantas | Citra brand | Moderasi sebelum tayang; balasan resmi; kebijakan ulasan di syarat & ketentuan |
| 9 | Kapasitas tim terbagi antara Fase 3, persiapan Fase 4, dan kemungkinan Fase 5 paralel | Rilis tertunda | Requirement C digeser lebih dulu; keputusan opsi paralel di steering committee |

---

## 14. Asumsi & Batasan

1. Track website mengerjakan R3.1 setelah launch R2.2; track ERP mengerjakan R3.2 dan R3.3 (Roadmap bagian 8). Setiap titik go-live menaikkan versi minor aplikasi (PRD Fase 2 asumsi #2).
2. Rasio poin, masa berlaku poin, batas penukaran, reward referral, diskon subscription, dan masa retur adalah nilai awal yang bisa diubah tanpa pengembangan. Nilai awalnya ditetapkan di bagian 15.1.
3. Poin tidak berlaku untuk order reseller dan pembelian dengan harga karyawan.
4. Pencapaian target dihitung dari tanggal invoice. Penjualan marketplace dan website tidak dihitung ke target sales perorangan, tetapi masuk target per channel.
5. Akun WhatsApp Business memakai satu nomor resmi Balans yang berbeda dari nomor WhatsApp CS harian, agar percakapan CS tidak terganggu.
6. Check-in kunjungan memakai geolocation browser. Akurasi GPS bergantung pada perangkat, sehingga jarak > 500 m hanya menjadi penanda, bukan penolakan.
7. Scan barcode memakai barcode produk yang sudah ada di master (MD-01) dan label produk Fase 1. Barcode per batch dicetak ulang hanya untuk batch baru setelah R3.3.
8. Data lead dari catatan sales sebelum R3.2 diimpor apa adanya, dan tahapnya diperbarui oleh pemilik lead.

---

## 15. Keputusan & Isu Terbuka

### 15.1 Keputusan

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| 1 | Nilai program poin | 1 poin per Rp100 belanja bersih, 1 poin = Rp1 (setara 1%); penukaran maksimal 30% nilai order; masa berlaku 12 bulan sejak diperoleh; poin juga diperoleh dari order subscription | 1% adalah tingkat yang lazim di e-commerce Indonesia dan cukup terasa bagi customer; batas 30% menjaga margin per order |
| 2 | Nilai referral | Customer baru mendapat voucher Rp25.000 untuk order pertama dengan minimal belanja Rp150.000; pemberi referral mendapat 25.000 poin setelah order customer baru final; maksimal 10 reward per pemberi per bulan | Minimal belanja menjaga order referral tetap bernilai; batas bulanan membatasi penyalahgunaan |
| 3 | Diskon subscription dan masa retur | Diskon subscription 10% dari harga website; masa retur website 7 hari setelah *Diterima* | Diskon cukup untuk mendorong langganan rutin; 7 hari sesuai praktik umum dan menjadi dasar kapan poin dan reward menjadi final |
| 4 | PIC pipeline B2B/corporate | Pipeline B2B dipegang Sales MT. Role *Sales B2B* dibentuk lewat konfigurasi saat Balans menunjuk PIC khusus, tanpa pengembangan | Sales MT sudah menangani B2B di Fase 1 (SLS-01) |
| 5 | Kebijakan ulasan, retur online, dan syarat & ketentuan program | Tim implementasi menyusun draf kebijakan ulasan, kebijakan retur (7 hari, alasan dari master retur, foto wajib), dan syarat & ketentuan poin, referral, dan subscription memakai nilai di butir 1 – 3. Balans meninjau dan mengesahkannya sebagai butir go/no-go R3.1 | Draf siap lebih cepat dan isinya konsisten dengan perilaku sistem; pengesahan hukum tetap di Balans |
| 6 | Perangkat scan di gudang | Kamera HP menjadi perangkat utama untuk semua proses scan. Ditambah 2 scanner Bluetooth 1D/2D (mode keyboard) untuk Staf Gudang di area Inbound dan Outbound, tempat volume scan paling tinggi | Tanpa biaya perangkat untuk sebagian besar user; scanner hanya di titik dengan volume tinggi, tempat kecepatan scan paling berpengaruh |
| 7 | ACC-23 anggaran per divisi | Direkomendasikan diambil di R3.2 karena kecil (Could) dan memakai data jurnal per divisi yang sudah ada. Keputusan akhir tetap mengikuti pilihan opsional Balans di BRD v1.0 | Memberi kendali biaya per divisi tanpa sistem tambahan |

### 15.2 Isu Terbuka

Isu yang berkaitan dengan integrasi pihak ketiga diputuskan saat persiapan R3.3.

| # | Isu | Kapan diputuskan | PIC |
|---|---|---|---|
| 1 | Penyedia WhatsApp Business API dan nomor resmi | Sebelum build R3.3 | PM, CS |
| 2 | Ketersediaan dan bentuk API vendor vending | Sebelum build R3.3 | Admin Vending |

---

## 16. Keterlacakan

### 16.1 Sumber Kebutuhan ke Fitur

| Sumber | Fitur PRD |
|---|---|
| Arahan Balans P-16 / U-16 (fitur e-commerce lengkap) | F3-101 s.d. F3-106, F3-108 |
| Arahan Balans P-19 / U-19 (tracking sales) | F3-201 s.d. F3-204, F3-206, F3-207 |
| Catatan #7 (WA/IG) | F3-301 |
| Catatan #5, U-05 (vending) | F3-302 |
| U-18 (alur reseller) | F3-107 |
| Divisi 2 Customer Service | F3-205, F3-106 |
| Catatan #1, Flow Inbound & Outbound | F3-303 |

### 16.2 Requirement BRD ke Rilis

| Modul | R3.1 | R3.2 | R3.3 |
|---|---|---|---|
| ECM | ECM-10, 11, 12, 13, 14, 16 | | |
| RSL | RSL-06 | | |
| SLP | | SLP-01, 02, 03, 04 | |
| CRM | | CRM-04 | |
| INT | | | INT-03, INT-06 |
| VND | | | VND-06 |
| WMS | | | WMS-06 |
| ACC | ACC-02 (A-35), A-13 retur online | | |
| RPT | RPT-20 | RPT-13, RPT-16 | |
| Opsional | | ACC-23 | |

### 16.3 Dokumen Pendukung

| Topik | Dokumen |
|---|---|
| Lingkup, urutan, ketergantungan | Roadmap v0.6 bagian 4 dan 8 |
| Website, reseller, retur, sales pipeline | BRD v0.7 bagian 5.9 – 5.11, 5.19 |
| Fitur website launch, portal reseller, vending | PRD Fase 2 |
| COA dan posting rule A-13, A-35 | Desain Finance & Accounting Fase 1 |
| Role, permission, matriks approval | Desain Role & Approval Fase 1 |
| Adapter mock, webhook, job queue | Technical Guideline v0.1 bagian 5.10, 9.3 |
| User story dan estimasi | Backlog Fase 3 |

---

## 17. Persetujuan

Dengan menandatangani bagian ini, pihak berikut menyetujui isi, kebutuhan, ukuran keberhasilan, dan syarat go-live Fase 3 sebagaimana dijabarkan di dokumen ini.

| Nama | Jabatan | Tanda Tangan | Tanggal |
|---|---|---|---|
| | Balans Nutrition — Direksi | | |
| | Balans Nutrition — PIC Utama Proyek | | |
| | Balans Nutrition — Sales (MT, E-commerce, Reseller & Vending) | | |
| | Balans Nutrition — Marketing | | |
| | Balans Nutrition — Finance & Accounting | | |
| | Balans Nutrition — Logistik | | |
| | Tim Implementasi — Project Manager | | |
| | Tim Implementasi — Tech Lead | | |
