# Product Requirements Document (PRD) — Fase 1
## Balans Nutrition — Unified Business Platform: Core Operations & Finance

| Item | Keterangan |
|---|---|
| Klien | Balans Nutrition |
| Dokumen | Product Requirements Document Fase 1 — Core Operations & Finance |
| Versi | 0.1 — Draft |
| Tanggal | 16 Oktober 2026 |
| Acuan | BRD Balans Unified Business Platform v0.7; Roadmap v0.6 (bagian 6, 11 – 15); PRD Fase 0 v0.1; Backlog Fase 1; Desain Data Model, Status Dokumen, Role & Approval, Finance & Accounting, Alur Lintas Modul, Penomoran & Dokumen Cetak, UI/UX ERP; Panduan Migrasi Data Fase 1; Technical Guideline v0.1; Naming Convention Menu & Fitur v1.0; Flow Inbound & Outbound Balans |
| Status | Draft untuk direview tim implementasi dan Balans |

PRD ini merinci **apa yang harus dihasilkan Fase 1**, perilaku yang diharapkan dari setiap fitur, kriteria penerimaannya, dan syarat go-live. Rincian per user story ada di Backlog Fase 1; aturan detail ada di dokumen desain Fase 1. Sesuai Roadmap (D-18), dokumen ini **tidak memuat target tanggal**; yang ditetapkan adalah isi, urutan, dan syarat selesai.

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

Fase 1 memindahkan **seluruh alur operasional dan akuntansi Balans yang tidak bergantung pada website** ke satu sistem ERP: Catatan #1 – #4, #6 – #17, dan #20 – #22. Fase ini dibangun dalam empat rilis internal dan **dirilis ke Balans sekaligus** dalam satu go-live di awal periode akuntansi (Roadmap prinsip 4), sebagai versi aplikasi `v1.0.0`.

| Rilis | Fokus | Hasil yang bisa didemokan di staging | Requirement | Story | SP |
|---|---|---|---|---|---|
| **R1.1** Fondasi & Master Data | Akses, approval, master, COA | User login sesuai role; seluruh master data dan price list bisa diinput dan diimpor; COA dan jurnal umum berjalan | 25 | 34 | 179 |
| **R1.2** Procurement, Maklon, Gudang & Inventory | Siklus beli dan stok | Beli → terima → Inbound/QC → simpan → kirim ke maklon → terima FG → stok berjalan dengan HPP FIFO per batch dan jurnal | 33 | 27 | 173 |
| **R1.3** Sales Management & Logistics | Siklus jual | Semua channel penjualan Fase 1 tercatat, Outbound dengan QC, stok terpotong otomatis, piutang terbentuk | 16 | 17 | 116 |
| **R1.4** Finance Close, Pajak & Laporan | Tutup buku & laporan | Kas/bank, biaya, pajak, aset, laporan, dan tutup buku bulanan berjalan; sistem siap UAT end-to-end | 31 | 19 | 125 |
| **Fase 1** | | | **105** | **97** | **593** |

**Status pengerjaan per 16 Oktober 2026** (rincian per story di Status Fase 1):

| Rilis | Fitur selesai | Fitur sebagian | Fitur belum | Story selesai / sebagian |
|---|---|---|---|---|
| R1.1 | 2 (F1-101, F1-102) | 3 (F1-103, F1-104, F1-105) | 15 | 4 selesai (S-101 – S-104) dan S-108 dari Fase 0; 3 sebagian (S-105, S-107, S-109) |
| R1.2 | — | — | 18 | — |
| R1.3 | — | — | 12 | — |
| R1.4 | — | — | 12 | — |

Status **Selesai** berarti kode, test otomatis, dan kriteria penerimaan terpenuhi di lingkungan lokal; penerimaan PO/key user di sprint review dan deploy staging masih mengikuti Definition of Done (bagian 12.3). Pekerjaan berikutnya: engine & inbox approval (F1-106), lalu fondasi posting rule (F1-118) dan master data.

Setelah keempat rilis selesai, Fase 1 masuk **UAT, mock migration, training, cut-over, go-live, dan hypercare** sampai tutup buku bulan pertama selesai (bagian 11).

---

## 2. Latar Belakang & Masalah

1. **Stok tersebar dan tidak real-time.** Stok FG ada di gudang kantor, Petty Stock, E-commerce Stock, lokasi konsinyasi, buffer vending, dan maklon. Tanpa satu ledger stok, Balans tidak tahu posisi stok per lokasi dan per batch (OBJ-01).
2. **HPP tidak memasukkan seluruh biaya.** Reject/waste packaging dan reject dari maklon ditanggung Balans (#10, #11) tetapi belum tercermin di HPP per batch (OBJ-02).
3. **Akuntansi terpisah dari operasional.** Jurnal dibuat manual setelah transaksi terjadi, sehingga tutup buku lambat dan laba rugi per channel tidak tersedia (OBJ-03, OBJ-08).
4. **Konsinyasi sulit diaudit.** Rekonsiliasi bulanan, penagihan barang terjual, dan selisih per PKS dikerjakan di Excel (OBJ-04).
5. **Kerja manual di Excel dan WhatsApp.** Pesanan WA/IG, penjualan marketplace, approval, dan pemakaian internal dicatat terpisah per divisi (OBJ-07).
6. **Gudang belum punya kontrol mutu yang tercatat.** Flow Inbound & Outbound Balans mensyaratkan QC saat barang masuk dan saat barang keluar oleh orang yang berbeda dari penyiap barang (D-23, D-24).

---

## 3. Tujuan & Ukuran Keberhasilan

Ukuran keberhasilan diukur selama hypercare dan dua periode tutup buku pertama setelah go-live. Nilai target adalah usulan tim implementasi dan disahkan bersama PRD ini.

| ID | Tujuan | Ukuran keberhasilan | Acuan |
|---|---|---|---|
| G1-01 | Stok real-time per lokasi dan batch | 100% pergerakan stok FG, packaging, dan GM di semua lokasi tercatat di sistem; akurasi cycle count gudang ≥ 98% dari nilai persediaan yang dihitung pada bulan kedua | OBJ-01 |
| G1-02 | HPP per batch akurat | 100% batch FG dari maklon punya HPP yang memuat jasa maklon, packaging baik, dan reject; nilai persediaan di laporan stok sama dengan saldo akun persediaan (selisih 0) | OBJ-02 |
| G1-03 | Transaksi menghasilkan jurnal otomatis | 100% transaksi operasional Fase 1 menghasilkan jurnal lewat posting rule; jurnal manual hanya untuk penyesuaian akhir bulan | OBJ-03 |
| G1-04 | Tutup buku tepat waktu | Tutup buku bulan pertama dan kedua selesai paling lambat hari kerja ke-10 bulan berikutnya, dengan checklist 9 langkah lengkap | BRD 5.15 |
| G1-05 | Konsinyasi rapi dan bisa diaudit | 100% partner konsinyasi aktif punya Berita Acara Rekonsiliasi dan invoice dari sistem setiap bulan; selisih diproses sesuai PKS | OBJ-04 |
| G1-06 | Profitabilitas per dimensi sejak go-live | Laba rugi per channel, sub-channel, dan divisi tersedia untuk periode pertama tanpa olah data di luar sistem | OBJ-08 |
| G1-07 | Excel lama ditinggalkan | Pencatatan paralel di Excel lama dihentikan setelah tutup buku bulan pertama; ≥ 90% user terdaftar aktif setiap minggu selama hypercare | OBJ-07, Roadmap risiko #11 |
| G1-08 | Approval terkendali | 100% dokumen yang masuk matriks approval disetujui lewat sistem; tidak ada dokumen yang disetujui oleh pembuatnya sendiri | BRD 5.17 |

---

## 4. Ruang Lingkup

### 4.1 Dalam Lingkup

| Kode | Modul | Fase 1 mencakup |
|---|---|---|
| MD | Master Data | MD-01 s.d. MD-13: produk & label barcode, packaging & material, bundel virtual, price list & resolver harga, diskon, pelanggan, mitra & PKS & kode produk mitra, pemasok & maklon, gudang & lokasi, master alasan retur/jenis biaya/kampanye, impor/ekspor Excel |
| PRC | Procurement | PRC-01 s.d. PRC-10: PR, PO barang, PO jasa maklon, PO reprocess, approval berjenjang, penerimaan sebagian, reject/waste, 3-way match, uang muka |
| WMS | Warehouse | WMS-01, 02, 03, 04, 07, 08, 09, 10, 11, 12: multi-gudang, Inbound/QC & Karantina, transfer & Dalam Pengiriman, Petty Stock, stock count terjadwal & form HP, Gudang Retur, lokasi eksternal, Outbound/QC |
| INV | Inventory & Costing | INV-01 s.d. INV-06, INV-09, INV-12: ledger & kartu stok, HPP termasuk reject, FIFO, batch & kedaluwarsa, pemakaian internal & event, repacking, FEFO, pemusnahan |
| LOG | Logistics | LOG-01 s.d. LOG-04: Delivery Order, label produk/mitra/koli, resi marketplace, kurir & resi manual, kurir internal, status pengiriman |
| SLS | Sales Management | SLS-01 s.d. SLS-09: MT beli putus & B2B, Proforma & Invoice NON-PKP, konsinyasi & rekonsiliasi, WA/IG, marketplace manual/impor, harga & diskon otomatis, diskon karyawan, retur penjualan, selisih konsinyasi |
| ACC | Finance & Accounting | ACC-01 s.d. ACC-08, ACC-10 s.d. ACC-22 (ACC-06 tahap 1 via impor) |
| HR | HRIS dasar | HR-01, HR-02, HR-03 |
| INT | Integrasi | INT-02 Excel |
| RPT | Laporan | RPT-01 s.d. RPT-06, RPT-08 s.d. RPT-12, RPT-14 dasar, RPT-15, RPT-18, RPT-19 |
| NFR | Non-fungsional | NFR-01 s.d. NFR-11 |

**Usulan opsional** U-28 (INV-10), U-29 (INV-11, A-28), dan U-30 (SLS-10) masuk Fase 1 hanya jika dipilih di BRD v1.0 (Roadmap bagian 11). Jika dipilih, backlog bertambah +5, +8, dan +8 SP di rilis R1.2, R1.2, dan R1.3.

### 4.2 Di Luar Lingkup Fase 1

- **Website e-commerce, CRM, reseller, dan vending machine** (ECM, CRM, RSL, VND; Fase 2). Fase 1 hanya menyiapkan lokasi buffer vending dan pengiriman stok ke buffer (A-24).
- **Integrasi Midtrans dan Biteship asli** (INT-01, INT-05, LOG-05, LOG-06; R2.2). Link bayar Midtrans untuk WA/IG di SLS-04 memakai adapter mock sampai R2.2; di Fase 1 CS memakai bukti transfer.
- **Integrasi langsung marketplace** (INT-04, ACC-06 otomatis; Fase 4). Fase 1 memakai input manual dan impor Excel.
- **WhatsApp Business API, API vending, scan barcode** (INT-03, INT-06, WMS-06; R3.3). Invoice dikirim ke pelanggan sebagai link PDF atau teks yang disalin CS.
- **Picking & packing order website** (WMS-05; R2.2). Picking & packing untuk Outbound DO tetap masuk Fase 1 lewat WMS-12.
- **Peringatan stok minimum dan mendekati kedaluwarsa** (INV-07, INV-08; R2.1). Laporan kedaluwarsa (RPT-19) sudah tersedia di Fase 1.
- **HRIS lengkap, payroll, PPh 21 karyawan via payroll** (HR-04 s.d. HR-12; Fase 5). PPh 21 bukan pegawai atas tagihan atau biaya tetap masuk ACC-19.
- **Role partner, reseller, dan self-service karyawan.** Akses pihak luar dimulai Fase 2.
- **Anggaran biaya per divisi** (ACC-23; opsional, R3.2).
- Seluruh butir BRD 2.2 (MRP, e-Faktur/PPN, pelaporan langsung ke Coretax, aplikasi mobile native, host-to-host bank, multi mata uang, POS).

---

## 5. Pengguna & Role

Role Fase 1 mengikuti Desain Role & Approval bagian 1. Satu user boleh memegang beberapa role.

| Divisi | Role | Pekerjaan utama di Fase 1 |
|---|---|---|
| Logistik & FAT | Purchasing / Logistik | PR & PO, Delivery Order, transfer stok |
| | Staf Gudang | Penerimaan, Inbound/QC, picking & packing Outbound, stock count, Gudang Retur |
| | Logistik Leader | Release DO, pemakaian internal, dan Petty Stock; cetak label; verifikasi retur MT; notifikasi ETA kedatangan |
| | Fulfillment Leader | Permintaan & penerimaan Petty Stock, QC Outbound, pengiriman, penugasan Merchandiser |
| | Merchandiser | Kurir internal, pickup retur MT & sisa event, QC Outbound, stock count lokasi konsinyasi dari HP |
| | Head Logistik | Approval selisih stok dan disposisi retur |
| | Finance & Accounting | Tagihan, invoice, pembayaran, rekonsiliasi, pajak, aset, jurnal |
| | Head Finance & Accounting | Approval keuangan, tutup buku, buka kembali periode |
| Sales | Sales MT | MT beli putus, B2B, konsinyasi, rekonsiliasi, retur MT |
| | Sales E-commerce | Penjualan marketplace manual dan impor |
| | Customer Service | Pesanan WA/IG, retur pelanggan |
| | Head Sales | Approval price list dan diskon di atas batas |
| Marketing | Marketing | Pemakaian internal & event, pengajuan biaya, kode kampanye |
| HRGA | HRGA / Payroll | Data dasar karyawan, sampel karyawan |
| Lintas divisi | Head Divisi | Approver level 1 PR, pengajuan biaya, pemakaian internal divisinya |
| Manajemen | Direksi, Direktur Utama | Approval tingkat atas, laporan, dashboard |
| Sistem | Admin Sistem | User, role, pengaturan, penomoran, template dokumen |

**Key user** Fase 1: satu per divisi (Logistik & FAT, Sales MT, Sales E-commerce/CS, Marketing, HRGA) ditambah PIC utama proyek. Key user Reseller & Vending tetap diundang ke sprint review karena master dan stok buffer vending dibangun di Fase 1.

---

## 6. Kebutuhan Fungsional per Rilis

Setiap fitur di bawah dipecah menjadi user story di Backlog Fase 1 (kolom *Story*). Kriteria penerimaan di tabel adalah perilaku minimal yang diuji di sprint review dan UAT. **Prioritas** mengikuti BRD: M = Must, S = Should. Requirement S tetap dikerjakan di Fase 1; jika kapasitas tidak cukup, requirement S menjadi kandidat pertama untuk digeser ke rilis setelah go-live lewat keputusan steering committee.

### 6.1 R1.1 — Fondasi & Master Data

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. | Story | Status |
|---|---|---|---|---|---|---|
| F1-101 | Login & sesi | Email + password; access token 15 menit dan refresh token berotasi; 5× gagal mengunci akun 15 menit; nonaktif user atau ganti role memutus sesi < 30 detik | NFR-02 | M | S-101, S-102 | Selesai |
| F1-102 | Role, permission & MFA | Matriks role × permission dari desain; menu dan tombol tanpa izin tersembunyi dan API menolak 403; MFA TOTP wajib untuk Admin Sistem, Head F&A, Direksi, HRGA; aksi sensitif meminta re-autentikasi | NFR-02 | M | S-103, S-104 | Selesai |
| F1-103 | Audit trail & soft delete | Create/update/delete/restore/ubah status/approval/login tercatat dengan diff dan field sensitif di-mask; hapus master = soft delete dan restore tercatat | NFR-03, D-25 | M | S-105, S-106 | Sebagian |
| F1-104 | Responsif & lokalisasi | Layout nyaman di desktop, tablet, HP; `Rp1.250.000`, `14 Okt 2026 09.30 WIB`; seluruh pesan Bahasa Indonesia | NFR-01, NFR-05 | M | S-107, S-108 | Sebagian |
| F1-105 | Privasi data | NIK, NPWP, rekening terenkripsi AES-256-GCM; tidak ada data pribadi di log dan Sentry | NFR-04 | M | S-109 | Sebagian |
| F1-106 | Engine & inbox approval | Kebijakan per jenis dokumen, nominal, dan divisi; level berurutan; pembuat tidak menyetujui dokumennya sendiri; inbox lintas dokumen; tolak wajib catatan; delegasi saat cuti; notifikasi in-app + email dan pengingat tiap jam di jam kerja | 5.17, NFR-11 | M | S-110, S-111, S-112 | Belum |
| F1-107 | Master produk & label barcode | SKU unik permanen; varian, tipe kemasan, satuan, umur simpan wajib; banyak barcode dengan satu utama; isi per karton; cetak label produk 50 × 30 mm | MD-01 | M | S-113 | Belum |
| F1-108 | Packaging & material, bundel virtual | Packaging (RM) dan bahan reprocess (GM); bundel tanpa stok sendiri, stok tersedia = min(stok komponen ÷ qty) | MD-02, MD-03 | M | S-114, S-115 | Belum |
| F1-109 | Price list & resolver harga | Price list per channel/mitra/karyawan/vending, effective-dated dengan riwayat versi; approval Head Sales → Head F&A; resolver memilih harga khusus → promo → bertingkat → channel → dasar dan menyimpan aturan & versi yang dipakai | MD-04, MD-11 | M | S-116, S-117, S-118 | Belum |
| F1-110 | Diskon & batas diskon | Diskon per item dan per transaksi, nominal atau persen; diskon di atas batas role meminta approval Head Sales | MD-05, MD-12 | M / S | S-119 | Belum |
| F1-111 | Pelanggan | HP dinormalisasi dan unik; multi alamat; tipe pelanggan; penanda karyawan | MD-06 | M | S-120 | Belum |
| F1-112 | Mitra, PKS & kode produk mitra | PKS dengan PDF, masa berlaku, harga konsinyasi, termin, ketentuan selisih; PKS kedaluwarsa memblokir DO konsinyasi; kode artikel & barcode retailer per SKU; Merchandiser per mitra | MD-07 | M | S-121, S-134 | Belum |
| F1-113 | Pemasok & maklon | Badan usaha dan PPh default; rekening terenkripsi | MD-08 | M | S-122 | Belum |
| F1-114 | Gudang & lokasi | Gudang baru otomatis membuat lokasi standar (Inbound, FG Non-Ecommerce, E-commerce Stock, Petty Stock, RM, GM, Retur); lokasi eksternal maklon, konsinyasi, buffer vending, mesin per pihak | MD-09 | M | S-123 | Belum |
| F1-115 | Master pendukung | Alasan retur; jenis biaya dengan akun beban, PPh, approver; kode kampanye dengan periode | MD-13 | S | S-124 | Belum |
| F1-116 | Impor/ekspor master Excel | Template diunduh dari sistem; validasi per baris berjalan sebagai job dengan progress; preview valid/error; unduh baris error; file yang sama tidak bisa diimpor dua kali | MD-10, INT-02 | M | S-125 | Belum |
| F1-117 | COA, jurnal umum, buku besar | COA bernomor sesuai desain final; akun induk tidak bisa diposting; jurnal harus balance, di periode terbuka, dengan dimensi wajib; jurnal terposting tidak bisa diubah dan dikoreksi lewat *Balik Jurnal* | ACC-01, ACC-13 | M | S-126, S-127, S-128 | Belum |
| F1-118 | Dimensi & posting rule | Setiap baris jurnal membawa channel, sub-channel, divisi, kampanye (opsional); `Poster` menolak jurnal tidak balance, periode tertutup, atau dimensi wajib kosong; satu sumber satu jurnal per rule | ACC-17, ACC-02 | M | S-129, S-130 | Belum |
| F1-119 | Karyawan dasar & harga karyawan | Data dasar karyawan dengan NIK terenkripsi; pelanggan tertaut karyawan otomatis mendapat harga karyawan; resign menghapus hak itu | HR-01, HR-03 | M | S-131, S-132 | Belum |
| F1-120 | Penomoran & periode | Nomor per kode dokumen per tahun tanpa loncat sesuai U-15; periode akuntansi dibuat otomatis per bulan | U-15, ACC-08 | M | S-133 | Belum |

**Deliverable R1.1:** seluruh master data bisa diinput dan diimpor; struktur gudang, price list, COA, dan matriks approval sudah terkonfigurasi di staging.

### 6.2 R1.2 — Procurement, Maklon, Gudang & Inventory

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. | Story | Status |
|---|---|---|---|---|---|---|
| F1-201 | Purchase Request | Draft → ajukan → approval Head divisi; PR disetujui dijadikan PO sebagian atau seluruh baris | PRC-08 | S | S-201 | Belum |
| F1-202 | PO barang, maklon & reprocess | PO packaging/barang; PO jasa maklon per batch dengan FG hasil dan packaging yang dipakai; PO reprocess dari lokasi GM; approval berjenjang sesuai nominal; cetak PDF; notifikasi ETA ke Logistik Leader H-1 | PRC-01, 02, 03, 06, NFR-08 | M | S-202, S-203, S-204 | Belum |
| F1-203 | Uang muka pembelian | Uang muka per PO (A-19) dan otomatis dipotong saat tagihan | PRC-09 | S | S-205 | Belum |
| F1-204 | Penerimaan barang | Per PO, penerimaan sebagian; qty baik/reject, batch, tanggal produksi & kedaluwarsa wajib; barang masuk lokasi Inbound berstatus Karantina; jurnal A-01 | PRC-04, 05, INV-04 | M | S-206 | Belum |
| F1-205 | Penerimaan FG maklon & HPP batch | Pemakaian packaging baik + reject diambil dari lokasi maklon; HPP batch = (jasa maklon + packaging terpakai) ÷ qty baik; sisa packaging tetap di lokasi maklon; jurnal A-03 | PRC-10, INV-02 | M | S-207 | Belum |
| F1-206 | Tagihan pemasok & 3-way match | PO–GR–tagihan dicocokkan; toleransi qty 0%, harga 1%; melebihi toleransi butuh approval Head F&A dan pembayaran diblokir sampai disetujui; jurnal A-18; laporan Barang Diterima Belum Ditagih cocok dengan saldo akun 2102 | PRC-07, ACC-04, ACC-22 | M | S-208, S-209 | Belum |
| F1-207 | Retur pembelian & nota debit | RB + nota debit; jurnal A-02; utang berkurang | ACC-16 | S | S-210 | Belum |
| F1-208 | Inbound & QC | Antrian Karantina; checklist QC per baris dengan foto; lulus → available di lokasi tujuan; gagal → reject, retur, atau disposisi sesuai asal; stok Karantina tidak bisa dialokasikan | WMS-02, WMS-09 | M | S-211 | Belum |
| F1-209 | Ledger stok & kartu stok | Semua dokumen stok memakai lokasi dan status (available, quarantine, in_transit, reserved); `stock_movements` append-only; saldo tidak pernah negatif termasuk saat transaksi bersamaan; kartu stok per SKU/lokasi/batch | WMS-01, INV-01 | M | S-212, S-213 | Belum |
| F1-210 | HPP FIFO & saran FEFO | Cost layer per penerimaan dan konsumsi FIFO; batch kedaluwarsa terdekat diusulkan saat picking & transfer, pilihan manual wajib alasan | INV-03, INV-09 | M / S | S-214, S-215 | Belum |
| F1-211 | Transfer stok & Petty Stock | Antar lokasi satu gudang langsung; antar gudang, ke maklon, ke mitra berstatus Dalam Pengiriman sampai diterima; permintaan Petty Stock: Fulfillment ajukan → Logistik Leader release → Staf Gudang siapkan → Fulfillment konfirmasi qty diterima | WMS-03, WMS-10 | M / S | S-216, S-226 | Belum |
| F1-212 | Stock count | Penuh dan cycle per lokasi; hitung buta; selisih → approval Head Logistik (> Rp2 jt + Head F&A); penyesuaian + jurnal A-14; jadwal cycle count, konsinyasi & vending bulanan, penuh akhir tahun; form HP target sentuh 44 px, autosave, foto; Merchandiser hanya melihat lokasi mitra yang ditugaskan | WMS-04, WMS-11 | M / S | S-217, S-218 | Belum |
| F1-213 | Lokasi eksternal | Saldo di lokasi konsinyasi, buffer vending, mesin, dan maklon terlihat di posisi stok | WMS-08 | M | S-219 | Belum |
| F1-214 | Gudang Retur & disposisi | QC retur, usulan disposisi per baris, approval sesuai matriks, eksekusi restock/repack/reprocess/sampel/musnah, jurnal A-26 | WMS-07 | M | S-220 | Belum |
| F1-215 | Pemakaian internal & event | Tujuan marketing/sampel karyawan/sampling sales/event dengan kode kampanye dan approval; release Logistik Leader; keluar FEFO; serah terima dikonfirmasi peminta; event: stok pindah ke lokasi Event tanpa jurnal, sisa dicatat maks. H+1 dan kembali lewat Inbound, jurnal A-11/A-12 hanya atas qty terpakai | INV-05, HR-02 | M | S-221, S-227 | Belum |
| F1-216 | Repacking & pemusnahan | Repacking: input barang retur + packaging baru, output FG batch baru dengan nilai HPP retur + packaging; pemusnahan dengan alasan, metode, saksi, foto, approval berjenjang, PDF berita acara, jurnal A-26/A-27 | INV-06, INV-12 | S / M | S-222, S-223 | Belum |
| F1-217 | Backdate & recost | Dokumen biaya bertanggal mundur di periode terbuka memicu job recost SKU terdampak dan jurnal penyesuaian; periode tertutup ditolak | ACC-14, D-25 | M | S-224 | Belum |
| F1-218 | Validasi jurnal pembelian & persediaan | Test otomatis memastikan A-01, 02, 03, 04, 11, 12, 14, 18, 19, 26, 27 balance dengan akun dan dimensi sesuai Desain Finance & Accounting | ACC-02 | M | S-225 | Belum |

**Deliverable R1.2:** siklus beli → terima → Inbound/QC → simpan → kirim packaging ke maklon → terima FG → hitung stok berjalan, lengkap dengan HPP per batch dan jurnal; Finance & Accounting memvalidasi jurnal rilis ini di sprint review.

### 6.3 R1.3 — Sales Management & Logistics

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. | Story | Status |
|---|---|---|---|---|---|---|
| F1-301 | Sales Order MT beli putus & B2B | SO dengan harga otomatis dari resolver; konfirmasi mengalokasikan stok FG Non-Ecommerce secara FEFO; Proforma Invoice terbit | SLS-01, SLS-06 | M | S-301 | Belum |
| F1-302 | Proforma & Invoice NON-PKP | PI berlaku 14 hari (pengaturan); kedaluwarsa melepas alokasi; pembayaran dikonfirmasi → invoice lunas (A-05); PDF tanpa PPN dengan keterangan NON-PKP | SLS-02, NFR-08 | M | S-302 | Belum |
| F1-303 | Konsinyasi | DO konsinyasi hanya jika PKS berlaku (A-07); rekonsiliasi bulanan: cut-off, stock count lokasi mitra, terjual = saldo awal + kirim − retur − saldo akhir, dibandingkan laporan mitra, unggah BA; approval Sales MT → Head F&A; invoice otomatis sesuai termin PKS (A-08); selisih ditagih ke mitra atau dibebankan Balans sesuai PKS (A-23); PDF Berita Acara Rekonsiliasi | SLS-03, SLS-09 | M | S-303, S-304, S-305 | Belum |
| F1-304 | Pesanan WA/IG | Cari pelanggan dari nomor HP atau buat baru; item dan ongkir; unggah bukti bayar; F&A konfirmasi; invoice dikirim sebagai link PDF atau teks siap tempel ke WA | SLS-04 | M | S-306 | Belum |
| F1-305 | Penjualan marketplace | Input manual dan impor Excel per marketplace; nomor order eksternal tidak bisa ganda; potong E-commerce Stock; bundel dipecah ke komponen | SLS-05 | M | S-307 | Belum |
| F1-306 | Penjualan karyawan | Pelanggan berstatus karyawan otomatis memakai price list karyawan | SLS-07 | M | S-308 | Belum |
| F1-307 | Delivery Order & label | DO dari SO atau konsinyasi; release Logistik Leader; PDF tanpa harga dengan batch & ED; label koli 100 × 150 mm per koli; label produk memakai kode produk mitra bila ada; status disiapkan → dikirim → diterima | LOG-01, LOG-04, NFR-08 | M / S | S-309, S-316 | Belum |
| F1-308 | Outbound & QC | Picking list per batch dan packing oleh Staf Gudang; QC oleh Fulfillment Leader/Merchandiser yang bukan penyiap; gagal QC → barang ke Karantina dan stok pengganti FEFO diambil otomatis; stok kurang → baris ditahan + notifikasi; reject dikembalikan ke maklon lewat retur pembelian otomatis tanpa approval (A-02) | WMS-12, ACC-16 | M | S-310, S-311 | Belum |
| F1-309 | Kurir, resi & kurir internal | Kurir dari master; resi manual; AWB marketplace; kurir internal wajib memilih Merchandiser yang mengonfirmasi diterima dengan foto/tanda terima dari HP | LOG-02, LOG-03 | M | S-312 | Belum |
| F1-310 | Pengakuan penjualan & HPP | Marketplace saat serah ke kurir (A-10 + A-06); MT dan WA/IG saat invoice (A-05 + A-06); konsinyasi saat rekonsiliasi (A-08); test per channel | ACC-02, U-25 | M | S-313 | Belum |
| F1-311 | Retur penjualan | Retur masuk Inbound lalu Gudang Retur; alasan dari master; nota kredit atau refund; jurnal A-13; retur MT/reseller: Sales ajukan → Logistik Leader verifikasi → Merchandiser pickup bila perlu → terima dengan cek ulang, foto, form retur | SLS-08 | M | S-314, S-317 | Belum |
| F1-312 | Piutang & pembayaran | Piutang per invoice; pembayaran sebagian; satu pembayaran dialokasikan ke banyak invoice; umur piutang | ACC-03, ACC-15 | M | S-315 | Belum |

**Deliverable R1.3:** semua channel penjualan Fase 1 (MT beli putus, B2B, konsinyasi, WA/IG, marketplace, karyawan) tercatat, stok terpotong otomatis, Outbound berjalan dengan QC, dan piutang terbentuk.

### 6.4 R1.4 — Finance Close, Pajak & Laporan

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. | Story | Status |
|---|---|---|---|---|---|---|
| F1-401 | Kas & bank, rekonsiliasi bank | Rekening bank terhubung ke akun; mutasi per rekening; impor file mutasi CSV/Excel; pencocokan otomatis berdasarkan referensi → nominal + tanggal → manual; usulan dikonfirmasi user | ACC-05 | M | S-401, S-402 | Belum |
| F1-402 | Settlement marketplace (tahap 1) | Impor laporan settlement; dicocokkan per order; jurnal dana cair A-10s (bank + fee + promo); sisa piutang marketplace per order terlihat | ACC-06 | M | S-403 | Belum |
| F1-403 | Pengajuan biaya & kas kecil | Jenis biaya dari master, bukti wajib, approval per nominal, verifikasi F&A + PPh, jurnal A-15; kas kecil dengan plafon & batas per transaksi dari pengaturan, jurnal A-17, pengisian kembali dengan approval (A-16) | ACC-11, ACC-12 | M / S | S-404, S-405 | Belum |
| F1-404 | PPh potong & PPh Final | Tarif effective-dated; PPh 21/23/4(2) dipotong di tagihan pemasok dan pengajuan biaya; rekap bukti potong per masa diekspor untuk Coretax; penyetoran (A-29); PPh Final UMKM bisa diaktifkan, dihitung dari omzet bruto (A-30), dengan pemantauan batas Rp4,8 M | ACC-19, ACC-20 | M / S | S-406, S-407 | Belum |
| F1-405 | Aset tetap | Register per kelas dan lokasi; penyusutan garis lurus bulanan lewat job (A-31), tidak dobel per periode | ACC-21 | S | S-408 | Belum |
| F1-406 | Laporan keuangan & buku besar | Neraca, Laba Rugi, Arus Kas; neraca saldo dan buku besar per periode & dimensi dengan drill-down ke jurnal dan dokumen sumber; ekspor Excel | ACC-07, ACC-18, RPT-12 | M | S-409, S-410 | Belum |
| F1-407 | Tutup buku | Checklist 9 langkah dengan status & PIC; tutup periode dengan re-autentikasi; buka kembali dengan approval Head F&A → Direksi | ACC-08, ACC-14 | S / M | S-411 | Belum |
| F1-408 | Saldo awal | Impor/entri saldo awal per akun (A-36), piutang/utang terbuka, aset; pengecekan rekonsiliasi cut-over | ACC-14 | M | S-412 | Belum |
| F1-409 | Impor/ekspor transaksi | Template per jenis transaksi dengan validasi dan preview seperti impor master | ACC-10, INT-02 | M | S-413 | Belum |
| F1-410 | Laporan operasional | Laporan stok, mutasi, stock count, HPP, kedaluwarsa, penjualan, konsinyasi, pemakaian internal, retur, outstanding PO, umur piutang/utang, pajak (bagian 10.3) | RPT | M | S-414, S-415, S-416 | Belum |
| F1-411 | Laba rugi per dimensi & dashboard | Laba rugi per channel, sub-channel, divisi; margin kotor per produk/kategori; dashboard eksekutif dasar (penjualan, margin, stok, kas) dari materialized view yang di-refresh job | RPT-15, RPT-14 | M | S-417, S-418 | Belum |
| F1-412 | Kesiapan produksi | Monitor uptime & alert aktif; uji restore bulanan tercatat; batas memori container; uji beban ≥ 40 user back-office lolos target bagian 9 | NFR-06, 07, 09, 10 | M | S-419 | Belum |

**Deliverable R1.4:** sistem siap UAT end-to-end, termasuk satu simulasi tutup buku bulanan di staging dengan data UAT.

---

## 7. Aturan Bisnis Lintas Modul

Aturan berikut berlaku di semua rilis. Detailnya ada di dokumen desain yang dirujuk.

| # | Aturan | Acuan |
|---|---|---|
| 1 | **Semua barang masuk lewat Inbound.** Penerimaan dari pemasok, maklon, retur pelanggan, tarikan konsinyasi, dan sisa event masuk lokasi Inbound berstatus Karantina. Stok Karantina tidak bisa dialokasikan, dijual, atau ditransfer sebelum lulus QC | WMS-02, WMS-09, D-07, D-23 |
| 2 | **Semua barang keluar lewat Outbound dengan QC.** Pemeriksa QC Outbound tidak boleh user yang sama dengan penyiap barang. Barang gagal QC diganti otomatis dengan stok FEFO berikutnya | WMS-12, D-24 |
| 3 | **Stok tidak pernah negatif.** Saldo dikunci berurutan per SKU–lokasi–batch; transaksi yang membuat saldo negatif ditolak dengan pesan yang menyebut SKU, lokasi, dan qty tersedia | INV-01, TG 5 |
| 4 | **Batch & kedaluwarsa wajib untuk FG.** Tidak ada saldo FG tanpa batch dan tanggal kedaluwarsa, termasuk saldo migrasi | INV-04, D-01 |
| 5 | **Nilai FIFO, ambil fisik FEFO.** HPP dihitung FIFO per batch; sistem mengusulkan batch kedaluwarsa terdekat | INV-03, INV-09, D-03 |
| 6 | **Reject ditanggung Balans masuk HPP.** Reject packaging saat terima dibebankan (A-04); reject dari maklon menambah HPP batch FG | INV-02, #10, #11 |
| 7 | **Harga diambil resolver, bukan diketik.** Urutan: harga khusus → promo → bertingkat → price list channel → harga dasar. Dokumen menyimpan aturan dan versi price list yang dipakai, sehingga perubahan harga tidak mengubah transaksi lama | MD-11, BRD 5.18 |
| 8 | **Setiap transaksi operasional menghasilkan jurnal lewat posting rule** di transaksi database yang sama dengan dokumen sumbernya. Jurnal terposting tidak bisa diubah; koreksi lewat jurnal balik | ACC-02, ACC-13 |
| 9 | **Pengakuan penjualan per channel:** marketplace saat serah ke kurir; MT, B2B, WA/IG saat invoice; konsinyasi saat rekonsiliasi | U-25 |
| 10 | **Periode terbuka.** Backdate hanya di periode terbuka dan memicu recost; periode tertutup hanya bisa dibuka dengan approval Head F&A → Direksi | ACC-14, D-25 |
| 11 | **Approval mengikuti matriks per jabatan** dengan batas nominal yang bisa diubah tanpa pengembangan; kebijakan yang berlaku dikunci saat dokumen diajukan; pembuat tidak menyetujui dokumennya sendiri | Desain Role & Approval 4 |
| 12 | **NON-PKP.** Semua invoice tanpa PPN dengan keterangan NON-PKP | D-04 |
| 13 | **Konsinyasi tanpa PKS berlaku tidak bisa dikirim.** Stok di lokasi mitra tetap milik Balans sampai terjual | MD-07, D-05 |
| 14 | **Hapus master = soft delete.** Data lama tetap tampil di dokumen lama | NFR-03, D-25 |
| 15 | **Label UI mengikuti Naming Convention v1.0**, termasuk nama status dokumen di Desain Status Dokumen | D-26 |

---

## 8. Alur End-to-End & Skenario UAT

Skenario UAT disusun dari alur di catatan Balans dan Desain Alur Lintas Modul. Setiap skenario dinyatakan lulus jika dokumen, stok, jurnal, dan laporan terkait sesuai hasil yang diharapkan, dan key user menandatanganinya.

| ID | Skenario | Divisi penguji | Hasil yang diperiksa |
|---|---|---|---|
| UAT-01 | PR → PO packaging → approval berjenjang → penerimaan sebagian dengan reject → Inbound/QC → tagihan 3-way match → pembayaran dengan uang muka | Logistik & FAT | Stok RM per batch, Barang Diterima Belum Ditagih, utang, jurnal A-01, A-04, A-18, A-19 |
| UAT-02 | Transfer packaging ke maklon → PO jasa maklon → terima FG dengan reject → QC → FG Non-Ecommerce | Logistik & FAT | Sisa packaging di lokasi maklon, HPP batch termasuk reject, jurnal A-03 |
| UAT-03 | PO Client MT → SO → alokasi FEFO → Proforma → pembayaran → invoice NON-PKP → DO → Outbound dengan QC gagal sebagian → stok pengganti → kirim → diterima | Sales MT, Logistik | Alokasi batch, Karantina, retur ke maklon otomatis, invoice, jurnal A-05, A-06, A-02 |
| UAT-04 | DO konsinyasi → stock count mitra oleh Merchandiser dari HP → rekonsiliasi bulanan dengan selisih → approval → invoice otomatis | Sales MT, F&A | Stok lokasi mitra, Berita Acara Rekonsiliasi, invoice termin PKS, jurnal A-07, A-08, A-23 |
| UAT-05 | Pesanan WA/IG pelanggan baru dan karyawan → bukti bayar → invoice ke WA → Outbound → kurir eksternal dan kurir internal | CS, HRGA | Harga karyawan otomatis, konfirmasi Merchandiser, piutang lunas |
| UAT-06 | Impor penjualan marketplace (termasuk bundel dan order ganda) → serah kurir → impor settlement → rekonsiliasi | Sales E-commerce, F&A | Penolakan order ganda, stok komponen bundel, jurnal A-10 & A-10s, sisa piutang marketplace |
| UAT-07 | Retur MT offline → verifikasi Logistik Leader → pickup Merchandiser → Inbound → Gudang Retur → disposisi (restock, repack, reprocess, musnah) | Sales MT, Logistik | Nota kredit, jurnal A-13 & A-26, berita acara pemusnahan |
| UAT-08 | Pemakaian event → release → serah terima → pengembalian sisa H+1 → jurnal atas qty terpakai; sampel karyawan | Marketing, HRGA | qty kirim = terpakai + sisa, jurnal A-11/A-12 dengan kode kampanye |
| UAT-09 | Permintaan Petty Stock → release → siapkan → konfirmasi terima sebagian | Fulfillment, Logistik | Stok reserved dilepas, mutasi Petty Stock |
| UAT-10 | Cycle count dengan selisih > Rp2 jt → approval dua level → penyesuaian | Logistik, F&A | Jurnal A-14, laporan stock count |
| UAT-11 | Pengajuan biaya dengan PPh 23, kas kecil & pengisian kembali, penyusutan aset, PPh Final | F&A, Marketing | Utang pajak, rekap bukti potong, jurnal A-15 s.d. A-17, A-29 s.d. A-31 |
| UAT-12 | Backdate penerimaan di periode terbuka → recost → tutup buku dengan checklist → percobaan transaksi di periode tertutup → buka kembali dengan approval | F&A, Direksi | Jurnal penyesuaian recost, penolakan periode tertutup, audit trail |
| UAT-13 | Laporan keuangan, laba rugi per channel/sub-channel/divisi, dashboard eksekutif untuk periode UAT | F&A, Direksi | Angka laporan cocok dengan neraca saldo dan dokumen sumber |
| UAT-14 | Uji akses: user tiap role membuka menu dan aksi di luar haknya; Merchandiser membuka mitra yang tidak ditugaskan | Semua | Menu tersembunyi, API menolak 403, audit login |

---

## 9. Kebutuhan Non-Fungsional

Target kinerja adalah usulan tim implementasi untuk kapasitas NFR-10 di VPS 4 core / 8 GB, dan diuji di S-419.

| NFR | Kebutuhan Fase 1 | Cara uji |
|---|---|---|
| NFR-01 Platform | Seluruh layar ERP berjalan di desktop, tablet, HP; form HP (stock count, konfirmasi kurir internal, QC) dengan target sentuh ≥ 44 px dan autosave | Playwright di 3 lebar layar |
| NFR-02 Keamanan | Login, RBAC per permission, password policy, MFA role sensitif, re-autentikasi aksi sensitif, pemutusan sesi < 30 detik | Test API & E2E per role (UAT-14) |
| NFR-03 Audit | Audit trail untuk transaksi dan perubahan master; soft delete | Test audit per jenis aksi |
| NFR-04 Privasi | Enkripsi field pribadi; scrubbing log & Sentry; data pribadi hanya untuk role yang berhak | Review kode + test |
| NFR-05 Lokalisasi | Bahasa Indonesia, IDR, WIB, format tanggal Indonesia; server & DB UTC | Test formatter + UAT |
| NFR-06 Ketersediaan | Uptime ≥ 99% pada jam operasional; monitor eksternal & alert | Laporan uptime selama hypercare |
| NFR-07 Backup | Backup harian + WAL off-site; uji restore bulanan tercatat; pemulihan ke VPS baru ≤ 2 jam | Uji restore sebelum go-live |
| NFR-08 Dokumen | PDF untuk PO, DO, Proforma, Invoice, nota kredit/debit, Berita Acara Rekonsiliasi, berita acara pemusnahan, label, dengan logo dan penomoran U-15 | Review layout oleh F&A & Sales |
| NFR-09 Skalabilitas | Tambah gudang, lokasi, channel, user lewat konfigurasi tanpa pengembangan | Demo di UAT |
| NFR-10 Kapasitas | ≥ 40 user back-office aktif bersamaan; halaman list & detail p95 ≤ 2 detik, simpan dokumen p95 ≤ 3 detik; laporan dan ekspor besar berjalan sebagai job dengan progress | Uji beban S-419 |
| NFR-11 Notifikasi | Notifikasi in-app + email approval dengan pengingat; email memakai provider asli sejak UAT | UAT-01, UAT-04 |

---

## 10. Integrasi, Dokumen Cetak & Laporan

### 10.1 Integrasi

| Integrasi | Fase 1 | Catatan |
|---|---|---|
| Excel (INT-02) | Impor/ekspor master dan transaksi; impor penjualan & settlement marketplace; impor mutasi bank | Template dihasilkan sistem dan sama dengan template migrasi Fase 0 |
| Email transaksional | Provider asli dipilih dan domain pengirim findyourbalans.com (SPF, DKIM, DMARC) siap sebelum UAT | Gate UAT Fase 1 (Roadmap bagian 13) |
| Midtrans, Biteship | Tetap adapter mock; tidak dipakai di production Fase 1 | Modul yang memakai mock tidak diaktifkan di production (D-27) |
| Marketplace, vending, WhatsApp | Tidak ada integrasi; jalur manual/impor | BRD 8, prinsip cadangan manual |

### 10.2 Dokumen Cetak & Label

Layout mengikuti Desain Penomoran & Dokumen Cetak: PO, DO (tanpa harga, dengan batch & ED), Proforma Invoice, Invoice NON-PKP, nota kredit, nota debit, Berita Acara Rekonsiliasi, berita acara pemusnahan, label produk 50 × 30 mm, label produk mitra, dan label koli 100 × 150 mm.

### 10.3 Laporan

Semua laporan bisa difilter per periode dan dimensi yang relevan, dan diekspor ke Excel sebagai job.

| ID | Laporan | Pengguna utama |
|---|---|---|
| RPT-01 | Posisi stok per SKU, lokasi, batch (termasuk konsinyasi, buffer vending, maklon, Dalam Pengiriman) | Logistik, Manajemen |
| RPT-02 | Kartu stok / mutasi | Logistik, F&A |
| RPT-03 | Hasil stock count & selisih | Logistik, F&A |
| RPT-04 | HPP per SKU per batch dengan rincian reject | F&A, Manajemen |
| RPT-05 | Penjualan per channel, SKU, periode | Sales, Manajemen |
| RPT-06 | Rekonsiliasi konsinyasi per mitra per bulan | Sales MT, F&A |
| RPT-08 | Pemakaian internal per kampanye & sampel karyawan | Marketing, HRGA, F&A |
| RPT-09 | Retur, alasan, tindak lanjut | Logistik, Sales |
| RPT-10 | Outstanding PO, penerimaan, Barang Diterima Belum Ditagih | Purchasing, F&A |
| RPT-11 | Umur piutang & utang | F&A |
| RPT-12 | Neraca, Laba Rugi, Arus Kas | F&A, Manajemen |
| RPT-14 | Dashboard eksekutif dasar | Manajemen |
| RPT-15 | Laba rugi per channel, sub-channel, divisi; margin per produk/kategori | F&A, Manajemen |
| RPT-18 | Rekap PPh dipotong, PPh Final, pemantauan omzet terhadap Rp4,8 M | F&A |
| RPT-19 | Stok mendekati kedaluwarsa & umur stok per batch per lokasi | Logistik, Sales |

---

## 11. UAT, Migrasi, Training & Go-live

### 11.1 Tahapan

| Urutan | Tahap | Aktivitas | Selesai jika |
|---|---|---|---|
| 1 | Sprint R1.1 – R1.4 | Build per rilis; demo di sprint review; F&A memvalidasi jurnal tiap rilis | Exit rilis (bagian 12.2) terpenuhi |
| 2 | Stabilisasi | Satu sprint regresi dan perbaikan sebelum UAT | Tidak ada defect kritis/tinggi terbuka dari QA internal |
| 3 | UAT | Skenario UAT-01 s.d. UAT-14 oleh key user di staging; defect dicatat dan diperbaiki | Semua skenario lulus dan ditandatangani |
| 4 | Mock migration 1 & 2 | Impor seluruh template ke staging dengan `cmd/cutover` di awal dan akhir UAT; pengecekan rekonsiliasi otomatis | Semua cek Panduan Migrasi bagian 4 lolos di mock 2 |
| 5 | Training | Paralel dengan UAT; training per role dengan materi dan video panduan; panduan form HP untuk Merchandiser dan Staf Gudang | Semua user aktif sudah ikut training role-nya |
| 6 | Stock count fisik | Hitung fisik semua lokasi (gudang, konsinyasi, buffer vending, maklon) menjelang cut-over | Berita acara stock count per lokasi |
| 7 | Freeze & migrasi final | Tidak ada transaksi di sistem lama setelah tanggal cut-over; impor ke production; saldo awal A-36 | Impor tanpa error dan rekonsiliasi cocok |
| 8 | Go-live | Awal periode akuntansi; rilis `v1.0.0` ke production | Go/no-go terpenuhi |
| 9 | Hypercare | Pendampingan harian, perbaikan cepat, pemantauan ukuran keberhasilan | Tutup buku bulan pertama selesai |

### 11.2 Go / No-Go

Go-live diputuskan steering committee jika seluruh butir berikut terpenuhi:

1. Tidak ada defect kritis yang terbuka dari UAT; defect tinggi yang tersisa punya workaround tertulis yang disetujui key user.
2. Saldo awal stok dan akuntansi lolos pengecekan rekonsiliasi dan disetujui tertulis oleh Head F&A.
3. Semua key user menandatangani UAT dan semua user aktif sudah ditraining.
4. Backup production berjalan dan uji restore terakhir berhasil.
5. Email notifikasi memakai provider asli; proses production tidak memakai adapter mock untuk modul yang aktif.
6. Nama approver dan pengganti untuk semua baris matriks approval sudah terkonfigurasi di production.
7. Monitoring, alert, dan Sentry production aktif dan pernah dipicu uji.

### 11.3 Rencana Mundur

Jika masalah kritis muncul di minggu pertama go-live dan tidak bisa diperbaiki dalam 1 hari kerja, transaksi tetap dicatat di sistem baru dengan prosedur manual sementara yang disepakati key user, bukan kembali ke sistem lama, agar saldo awal tidak terpecah. Rollback aplikasi ke versi sebelumnya mengikuti Runbook Operasi.

---

## 12. Entry, Exit & Definition of Done

### 12.1 Entry Criteria Fase 1

| # | Kriteria | Sumber |
|---|---|---|
| 1 | Exit criteria Fase 0 terpenuhi (PRD Fase 0 bagian 10) | PRD Fase 0 |
| 2 | BRD v1.0 ditandatangani, pilihan opsional U-28 s.d. U-32 ditentukan | F0-APR-02, F0-APR-04 |
| 3 | Backlog R1.1 terestimasi dan komposisi tim disetujui | F0-APR-05 |
| 4 | Staging berjalan di `erp-staging.findyourbalans.com` dengan CI/CD, backup, dan monitoring | F0-TEK-07 s.d. F0-TEK-10 |

Jalur teknis R1.1 yang tidak bergantung pada keputusan Balans (login, role, audit, engine approval, master produk & gudang) boleh dimulai sebelum butir 2 selesai, karena lingkupnya sama di semua opsi.

### 12.2 Exit per Rilis

| Rilis | Exit criteria |
|---|---|
| R1.1 | Semua story R1.1 memenuhi Definition of Done; master data contoh dari template migrasi berhasil diimpor ke staging; matriks approval terkonfigurasi |
| R1.2 | Semua story R1.2 memenuhi Definition of Done; UAT-01, UAT-02, UAT-09, UAT-10 lulus di sprint review; F&A memvalidasi jurnal R1.2 |
| R1.3 | Semua story R1.3 memenuhi Definition of Done; UAT-03 s.d. UAT-07 lulus di sprint review; F&A memvalidasi jurnal R1.3 |
| R1.4 | Semua story R1.4 memenuhi Definition of Done; UAT-11 s.d. UAT-13 lulus di sprint review; uji beban S-419 lolos |

### 12.3 Definition of Done Story

Checklist PR Technical Guideline Lampiran C lolos; CI hijau termasuk gate coverage 80% untuk package `domain`, `tax`, `pricing`, `money`; terdeploy di staging; diterima PO/key user di sprint review; label sesuai Naming Convention; jurnal baru punya test balance dan dimensi.

### 12.4 Exit Fase 1

Fase 1 ditutup di steering committee setelah go-live dan hypercare, jika tutup buku bulan pertama selesai di sistem, ukuran keberhasilan G1-03, G1-04, dan G1-08 tercapai untuk periode pertama, dan daftar defect pasca go-live tidak memuat defect kritis.

---

## 13. Risiko & Mitigasi

| # | Risiko | Dampak | Mitigasi |
|---|---|---|---|
| 1 | Beban Fase 1 besar (105 requirement, 593 SP) dan velocity belum terukur | Go-live mundur | Sprint pertama sebagai sprint pengukuran; requirement S menjadi kandidat geser pertama; opsi Fase 1 dibatasi (Roadmap bagian 11) |
| 2 | Kualitas data migrasi rendah | HPP dan laporan tidak akurat sejak hari pertama | Dua kali mock migration, pengecekan rekonsiliasi otomatis, stock count fisik sebelum cut-over |
| 3 | Disiplin batch & kedaluwarsa di gudang rendah | FEFO dan HPP FIFO tidak akurat | Batch & ED wajib saat penerimaan; Karantina; label barcode produk; training Staf Gudang |
| 4 | Tim gudang kecil sehingga pemisahan penyiap dan pemeriksa QC sulit dijalankan | Outbound tertahan | Pemeriksa bisa Fulfillment Leader atau Merchandiser; Logistik Leader menugaskan pemeriksa harian; penolakan otomatis bila user sama tetap berlaku |
| 5 | Approver menumpuk dokumen | Operasional tertahan | Pengingat per jam, delegasi saat cuti, inbox lintas dokumen, badge jumlah di menu |
| 6 | Perubahan lingkup setelah BRD v1.0 | Fase 1 melebar | Change request dengan hitung ulang SP dan dampak rilis |
| 7 | Mitra konsinyasi tidak mengirim laporan penjualan tepat waktu | Rekonsiliasi dan invoice terlambat | Stock count oleh Merchandiser sebagai dasar perhitungan; laporan mitra hanya pembanding |
| 8 | Format file mutasi bank dan laporan settlement marketplace berbeda per sumber | Impor gagal | Template pemetaan kolom per bank/marketplace; contoh file diminta dari Balans sebelum R1.4 |
| 9 | Adopsi rendah, user kembali ke Excel/WA | Data sistem tidak lengkap | Key user per divisi, training per role, hypercare, freeze sistem lama setelah cut-over |
| 10 | Seluruh aplikasi di 1 VPS | Downtime saat jam operasional | Backup PITR off-site, monitoring eksternal, runbook pemulihan ≤ 2 jam, batas memori per container |

---

## 14. Asumsi & Batasan

1. Fase 1 dimulai setelah exit Fase 0; keputusan di BRD v1.0 menggantikan usulan v0.7 jika berbeda, dan PRD ini diperbarui mengikutinya.
2. Tim inti sesuai Roadmap asumsi #2 (PM/BA, UI/UX, 2 frontend, 2 backend, QA) dengan sprint 2 minggu; jumlah sprint ditetapkan setelah velocity sprint pertama diukur.
3. Empat rilis R1.1 – R1.4 adalah rilis internal ke staging untuk demo dan validasi. Balans memakai sistem di production hanya setelah go-live `v1.0.0`; modul yang belum siap disembunyikan dengan feature flag modul.
4. Target kinerja di bagian 9 dan ukuran keberhasilan di bagian 3 adalah usulan tim implementasi, karena BRD tidak menetapkan angka; disahkan bersama PRD ini.
5. Invoice WA/IG di Fase 1 dikirim sebagai link PDF atau teks yang ditempel CS ke WhatsApp; pengiriman otomatis menunggu WhatsApp Business API (R3.3).
6. Pembayaran WA/IG di Fase 1 memakai transfer dengan bukti bayar yang dikonfirmasi F&A; link bayar Midtrans tersedia mulai R2.2.
7. Penjualan marketplace dicatat per order (manual atau impor), bukan per rekap harian, agar settlement bisa dicocokkan per order.
8. Rekonsiliasi konsinyasi memakai hasil stock count Merchandiser di lokasi mitra sebagai sumber utama; laporan penjualan mitra dipakai sebagai pembanding dan dasar selisih.
9. Pemeriksa QC Outbound bisa Fulfillment Leader atau Merchandiser; sistem hanya mewajibkan pemeriksa berbeda dari penyiap barang.
10. Stok di lokasi buffer vending sudah bisa dikirim di Fase 1 (A-24), tetapi penjualan mesin vending baru dicatat di R2.3. Jika ada mesin yang sudah berjalan sebelum R2.3, penjualannya dicatat sementara lewat penyesuaian berdasarkan stock count bulanan.
11. Data karyawan Fase 1 hanya data dasar untuk harga karyawan, sampel, dan divisi; data payroll tidak dimigrasi di Fase 1.
12. Property dan booth event bukan stok dan tidak dicatat di sistem Fase 1.
13. Transaksi historis tidak dimigrasi; laporan periode sebelum cut-over tetap di sistem lama (U-10).

---

## 15. Keputusan & Isu Terbuka

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| 1 | Kode dokumen di luar U-15 | SO (Sales Order semua channel), IN (Inbound), OUT (Outbound), PU (Pemakaian Internal), MUS (Berita Acara Pemusnahan), dan BYR (Pembayaran) ditetapkan dengan format U-15 `KODE/TAHUN/BULAN/NOMOR` | Sesuai usulan Desain Penomoran & Dokumen Cetak; format sama dengan kode U-15 sehingga tidak ada aturan penomoran kedua |
| 2 | Label status periode akuntansi | `open` berlabel *Terbuka*, `closed` berlabel *Ditutup*, dan keduanya ditambahkan ke Naming Convention bagian 7 sebagai status khusus periode | Sudah dipakai di Desain Status Dokumen; Naming Convention bagian 7 mengizinkan status khusus proses |
| 3 | Format file mutasi bank dan laporan settlement marketplace | Impor memakai **profil pemetaan kolom** per bank dan per marketplace yang dikonfigurasi Admin Sistem (kolom tanggal, referensi, nominal, debit/kredit, nomor order, komponen biaya). Format baru atau berubah cukup ditambah sebagai profil baru tanpa pengembangan. Balans menyerahkan satu contoh file per bank dan per marketplace sebelum sprint R1.4 untuk profil awal | Format tiap sumber berbeda dan bisa berubah (risiko #8); profil konfigurasi menghindari perubahan kode setiap kali format berubah |
| 4 | Printer label gudang | Printer label thermal 203 dpi yang mendukung lebar 100 mm, dengan gulungan 50 × 30 mm (label produk) dan 100 × 150 mm (label koli); template A4 tetap tersedia sebagai cadangan. Kebutuhan awal satu printer di area Outbound dan satu di area Inbound | Ukuran sudah ditetapkan di Desain Penomoran & Dokumen Cetak; 203 dpi cukup untuk barcode EAN-13 dan Code 128 di ukuran tersebut |

Tidak ada isu terbuka yang tersisa untuk Fase 1 selain penyerahan contoh file di butir 3, yang merupakan data milik Balans.

---

## 16. Keterlacakan

### 16.1 Catatan Balans ke Fitur

| Catatan | Fitur PRD |
|---|---|
| Divisi 1 (Logistik & FAT) | F1-202, F1-204, F1-212, F1-302, F1-307 |
| #1 Inbound & struktur gudang | F1-114, F1-208, F1-209, F1-211 |
| #2 – #3 MT beli putus | F1-301, F1-302 |
| #4 Konsinyasi & PKS | F1-112, F1-303 |
| #6 Marketplace | F1-305, F1-309, F1-402 |
| #7 WA/IG | F1-304, F1-309 |
| #8 Data pelanggan | F1-111 |
| #9 – #11 Packaging, maklon, reject | F1-108, F1-202, F1-204, F1-205 |
| #12 Gudang Retur, repack, reprocess | F1-214, F1-216, F1-202 |
| #13 – #14 Pemakaian internal & sampel | F1-215 |
| #15 Diskon karyawan | F1-119, F1-306 |
| #16 – #17 Diskon & model harga | F1-109, F1-110 |
| #20 Excel | F1-116, F1-409 |
| #21 Akuntansi | F1-117, F1-118, F1-206, F1-310, F1-312, F1-401 s.d. F1-408 |
| #22 Bundel | F1-108, F1-305 |
| Flow Inbound & Outbound | F1-107, F1-208, F1-211, F1-212, F1-215, F1-307, F1-308, F1-309, F1-311 |

### 16.2 Requirement BRD ke Rilis

| Modul | R1.1 | R1.2 | R1.3 | R1.4 |
|---|---|---|---|---|
| MD | MD-01 s.d. MD-13 | | | |
| PRC | | PRC-01 s.d. PRC-10 | | |
| WMS | | WMS-01, 02, 03, 04, 07, 08, 09, 10, 11 | WMS-12 | |
| INV | | INV-01 s.d. INV-06, INV-09, INV-12 | | |
| LOG | | | LOG-01 s.d. LOG-04 | |
| SLS | | | SLS-01 s.d. SLS-09 | |
| HR | HR-01, HR-03 | HR-02 | | |
| ACC | ACC-01, 13, 17; ACC-02 fondasi | ACC-02 pembelian & persediaan; ACC-04, 16, 22 | ACC-02 penjualan; ACC-03, 15 | ACC-05, 06 tahap 1, 07, 08, 10, 11, 12, 14, 18, 19, 20, 21 |
| INT | INT-02 master | | | INT-02 transaksi |
| RPT | | | | RPT-01 s.d. 06, 08 s.d. 12, 14 dasar, 15, 18, 19 |
| NFR | NFR-01 s.d. 05, 11 | NFR-08 (PO) | NFR-08 (DO, PI, invoice, BA) | NFR-06, 07, 09, 10 |

### 16.3 Dokumen Pendukung

| Topik | Dokumen |
|---|---|
| User story, estimasi, kalibrasi | Backlog Fase 1 |
| Tabel, kepemilikan modul, integritas | Desain Data Model Fase 1 |
| Status & transisi dokumen | Desain Status Dokumen Fase 1 |
| Role, permission, matriks approval | Desain Role & Approval Fase 1 |
| COA, posting rule A-xx, dimensi, pajak | Desain Finance & Accounting Fase 1 |
| Sequence alur lintas modul | Desain Alur Lintas Modul Fase 1 |
| Penomoran, layout cetak, label | Desain Penomoran & Dokumen Cetak |
| Design system, menu, wireframe, form HP | Desain UI/UX ERP |
| Template, cut-over, rekonsiliasi saldo awal | Panduan Migrasi Data Fase 1 |
| Deploy, backup, restore, secret | Runbook Operasi Balans Platform |

---

## 17. Persetujuan

Dengan menandatangani bagian ini, pihak berikut menyetujui isi, kebutuhan, ukuran keberhasilan, dan syarat go-live Fase 1 sebagaimana dijabarkan di dokumen ini.

| Nama | Jabatan | Tanda Tangan | Tanggal |
|---|---|---|---|
| | Balans Nutrition — Direksi | | |
| | Balans Nutrition — PIC Utama Proyek | | |
| | Balans Nutrition — Finance & Accounting | | |
| | Balans Nutrition — Logistik | | |
| | Balans Nutrition — Sales | | |
| | Tim Implementasi — Project Manager | | |
| | Tim Implementasi — Tech Lead | | |
