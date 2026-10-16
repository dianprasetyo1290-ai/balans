# Backlog Fase 1
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Backlog user story Fase 1 (F0-DSN-06) |
| Acuan | Roadmap v0.6 bagian 6 dan 14; BRD v0.7 bagian 5 – 7, 9, 10; dokumen desain Fase 1 |
| Status | Draft untuk sprint planning; estimasi dikalibrasi ulang setelah sprint pertama |

**Estimasi** memakai story point (SP) skala 1, 2, 3, 5, 8, 13 dari sudut pandang satu pasangan backend + frontend, termasuk test sesuai Technical Guideline 12. Story bernilai 13 dipecah lagi saat sprint planning. **Definition of Done** berlaku untuk semua story: checklist PR Lampiran C lolos, CI hijau, terdeploy di staging, diterima PO/key user di sprint review, label sesuai Naming Convention.

Kriteria penerimaan ditulis ringkas; detail aturan ada di dokumen desain (Data Model, Status Dokumen, Role & Approval, Finance & Accounting, UI/UX).

---

## R1.1 — Fondasi & Master Data (25 requirement)

| ID | User story | Kriteria penerimaan | Req | SP |
|---|---|---|---|---|
| S-101 | Sebagai user, saya masuk dengan email & password agar bisa memakai ERP | Login sukses → access token 15 mnt + refresh cookie; password salah → modal *Gagal masuk*; 5× gagal → akun terkunci 15 mnt; logout mencabut sesi | NFR-02 | 8 |
| S-102 | Sebagai Admin Sistem, saya mengelola user dan role | CRUD user; undangan atur password via email; nonaktif/ganti role menaikkan `token_version` dan memutus sesi < 30 detik | NFR-02 | 5 |
| S-103 | Sebagai Admin, saya mengatur permission per role | Matriks role × permission dari desain; perubahan langsung berlaku; tombol/halaman tanpa izin tersembunyi dan API menolak 403 | NFR-02 | 5 |
| S-104 | Sebagai role sensitif, saya wajib MFA | Admin, Head F&A, Direksi, HRGA wajib TOTP; enrol QR; aksi bertanda re-auth meminta password/TOTP | NFR-02 | 5 |
| S-105 | Sebagai auditor, saya melihat audit trail | Create/update/delete/restore/status/approval/login tercatat dengan diff; field sensitif di-mask; tampil di tab Riwayat dokumen dan halaman audit | NFR-03 | 5 |
| S-106 | Sebagai user, data yang saya hapus tidak hilang dari database | Hapus master = soft delete; tidak tampil di list/dropdown baru; tetap tampil di dokumen lama; restore oleh Admin tercatat audit | NFR-03 | 3 |
| S-107 | Sebagai user HP, layout ERP nyaman di desktop, tablet, HP | Sidebar jadi sheet di HP; tabel bergulir horizontal; uji visual 3 lebar di Playwright | NFR-01 | 3 |
| S-108 | Sebagai user, semua teks, uang, tanggal memakai format Indonesia & WIB | `Rp1.250.000`, `14 Okt 2026 09.30 WIB`; pesan error Bahasa Indonesia; server UTC | NFR-05 | 2 |
| S-109 | Sebagai pelanggan/karyawan, data pribadi saya dilindungi | NIK/NPWP/rekening terenkripsi AES-256-GCM; pencarian NIK via hash; persetujuan pemasaran tersimpan dengan waktu; tidak ada data pribadi di log | NFR-04 | 5 |
| S-110 | Sebagai approver, saya diberi tahu ada dokumen menunggu | Notifikasi in-app + email saat menjadi approver; pengingat tiap 1 jam jam kerja; badge jumlah di menu | NFR-11 | 5 |
| S-111 | Sebagai approver, saya punya inbox persetujuan | List lintas jenis dokumen; setujui/tolak (catatan wajib saat tolak); delegasi saat cuti | NFR-11, 5.17 | 8 |
| S-112 | Engine approval generik dipakai semua dokumen | Kebijakan per jenis/nominal/divisi effective-dated; level berurutan; pembuat tidak menyetujui dokumennya sendiri; handler pemilik dokumen dipanggil di transaksi yang sama | 5.17 | 8 |
| S-113 | Sebagai Logistik, saya mengelola master produk FG dan mencetak label barcode produk | Field MD-01; SKU unik permanen; umur simpan wajib; barcode banyak dengan satu barcode utama; isi per karton; cetak label produk (thermal/A4, jumlah bisa diubah); list dengan filter & cari | MD-01 | 8 |
| S-114 | Sebagai Logistik, saya mengelola packaging & material | Tipe packaging/material; pakai batch opsional; tampil di menu *Packaging & Material* | MD-02 | 2 |
| S-115 | Sebagai Sales, saya membuat bundel virtual | Komponen + qty; bundel tidak punya stok; stok tersedia bundel = min(stok komponen ÷ qty) | MD-03 | 5 |
| S-116 | Sebagai Sales, saya mengelola price list per channel/mitra/level/karyawan/vending | Header + item effective-dated; versi baru tidak mengubah transaksi lama; approval Head Sales → Head F&A | MD-04 | 8 |
| S-117 | Sistem menentukan harga otomatis sesuai prioritas | Resolver 5 aturan berurutan (khusus → promo → bertingkat → channel → dasar); hasil menyimpan aturan & versi; table-driven test tiap aturan | MD-11 | 8 |
| S-118 | Sebagai Sales, saya membuat promo periode dan harga bertingkat | Promo dengan tanggal mulai–selesai per channel; harga bertingkat per qty minimum | MD-04, MD-11 | 5 |
| S-119 | Sebagai Sales, saya memberi diskon per item/transaksi | Nominal atau persen; di atas batas role → SO butuh approval Head Sales | MD-05, MD-12 | 5 |
| S-120 | Sebagai CS, saya mengelola pelanggan tanpa duplikat | Field MD-06; HP dinormalisasi & unik; multi alamat; tipe pelanggan | MD-06 | 5 |
| S-121 | Sebagai Sales MT, saya mengelola mitra & PKS | Field MD-07; unggah PDF PKS; masa berlaku; harga konsinyasi, termin, ketentuan selisih; PKS kedaluwarsa memblokir DO konsinyasi | MD-07 | 8 |
| S-122 | Sebagai Logistik, saya mengelola pemasok & maklon | Field MD-08; badan usaha & PPh default; rekening terenkripsi | MD-08 | 3 |
| S-123 | Sebagai Logistik, saya mengelola gudang & lokasi | Gudang baru membuat lokasi standar (Inbound, FG Non-Ecommerce, Petty Stock, RM, GM, Retur); lokasi eksternal maklon/konsinyasi/buffer/mesin per pihak | MD-09 | 5 |
| S-124 | Sebagai Admin, saya mengelola alasan retur, jenis biaya, kampanye | Master MD-13; jenis biaya punya akun beban, PPh, approver; kampanye punya kode & periode | MD-13 | 3 |
| S-125 | Sebagai user, saya impor/ekspor master data via Excel | Unduh template dari sistem; validasi per baris sebagai job dengan progress; preview valid/error; unduh baris error; file sama tidak bisa diimpor 2× | MD-10, INT-02 | 13 |
| S-126 | Sebagai F&A, saya mengelola COA | Akun bernomor sesuai COA final; induk tidak bisa diposting; flag posting manual; akun terpakai tidak bisa dihapus | ACC-01 | 3 |
| S-127 | Sebagai F&A, saya membuat jurnal umum dan melihat buku besar | Jurnal balance, periode terbuka, dimensi wajib; buku besar per akun dengan saldo berjalan | ACC-01 | 8 |
| S-128 | Jurnal yang sudah diposting tidak bisa diubah | Edit/hapus ditolak; *Balik Jurnal* membuat JV pembalik yang mereferensikan asal | ACC-13 | 3 |
| S-129 | Setiap baris jurnal membawa dimensi | Channel, sub-channel, divisi, kampanye opsional; aturan wajib per posting rule; master dimensi | ACC-17 | 5 |
| S-130 | Fondasi posting rule & Poster | `posting_rules` + `posting_rule_accounts`; `Poster.Post` menolak tidak balance/periode tertutup/dimensi kosong; unik per sumber+rule | ACC-02 | 8 |
| S-131 | Sebagai HRGA, saya mengelola data dasar karyawan | Field HR-01 dasar; NIK terenkripsi; resign tercatat | HR-01 | 3 |
| S-132 | Pelanggan berstatus karyawan otomatis dapat harga karyawan | Link pelanggan ↔ karyawan; resign → harga karyawan hilang otomatis | HR-03 | 3 |
| S-133 | Penomoran dokumen & periode akuntansi dasar | Sequence per kode/tahun tanpa loncat; periode dibuat otomatis per bulan | U-15, ACC-08 | 3 |
| S-134 | Sebagai Sales MT, saya mengelola kode produk mitra dan Merchandiser per partner | Kode artikel & barcode retailer per SKU per partner, unik per partner–SKU; impor Excel; Merchandiser yang ditugaskan per partner | MD-07 | 3 |
| | **Total R1.1** | | | **179** |

## R1.2 — Procurement, Maklon, Gudang & Inventory (33 requirement)

| ID | User story | Kriteria penerimaan | Req | SP |
|---|---|---|---|---|
| S-201 | Sebagai divisi, saya mengajukan Purchase Request | Draft → ajukan → approval Head divisi; PR disetujui bisa dijadikan PO (sebagian/seluruh baris) | PRC-08 | 5 |
| S-202 | Sebagai Purchasing, saya membuat PO packaging/barang | Form PO; ajukan → approval berjenjang sesuai nominal; cetak PO (PDF); notifikasi ETA & detail barang ke Logistik Leader H-1 tanggal kirim diharapkan | PRC-01, PRC-06, NFR-08 | 8 |
| S-203 | Sebagai Purchasing, saya membuat PO jasa maklon per batch | Baris FG hasil + harga jasa; daftar packaging yang akan dipakai | PRC-02 | 5 |
| S-204 | Sebagai Purchasing, saya membuat PO reprocess | Bahan reprocess dari lokasi GM (Gudang GM) dikirim ke maklon | PRC-03 | 3 |
| S-205 | Sebagai F&A, saya mencatat uang muka pembelian | Uang muka per PO (A-19); otomatis dipotong saat tagihan | PRC-09 | 5 |
| S-206 | Sebagai Staf Gudang, saya mencatat penerimaan barang | Per PO, penerimaan sebagian; qty baik/reject, batch, tgl produksi & kedaluwarsa; stok masuk Inbound Karantina; jurnal A-01 | PRC-04, PRC-05, INV-04 | 8 |
| S-207 | Penerimaan FG dari maklon menghitung HPP batch | Pemakaian packaging baik + reject dari lokasi maklon; nilai batch ÷ qty baik; jurnal A-03; test HPP termasuk reject | PRC-10, INV-02 | 8 |
| S-208 | Sebagai F&A, saya memproses tagihan pemasok dengan 3-way match | Cocokkan PO–GR–tagihan; toleransi qty 0% harga 1% dari pengaturan; melebihi → approval Head F&A; jurnal A-18; utang terbentuk | PRC-07, ACC-04, ACC-22 | 8 |
| S-209 | Sebagai F&A, saya melihat saldo Barang Diterima Belum Ditagih | Laporan per PO/GR yang belum ditagih; cocok dengan saldo akun 2102 | ACC-22 | 3 |
| S-210 | Sebagai Purchasing, saya membuat retur pembelian dengan nota debit | RB + DN; asal QC tanpa approval; jurnal A-02; utang berkurang | ACC-16, PRC-05 | 5 |
| S-211 | Sebagai Staf Gudang, saya memproses Inbound & QC | Antrian Karantina; checklist QC per baris + foto; lulus → available ke lokasi tujuan; tolak → reject/retur/disposisi sesuai asal | WMS-02, WMS-09 | 8 |
| S-212 | Struktur multi-gudang dan lokasi berjalan di transaksi | Semua dokumen stok memakai lokasi; status stok available/quarantine/in_transit/reserved | WMS-01 | 3 |
| S-213 | Ledger stok, saldo, dan kartu stok | `stock_movements` append-only; `stock_balances` non-negatif dengan lock berurutan; kartu stok per SKU/lokasi/batch; test concurrency stok tidak minus | INV-01 | 13 |
| S-214 | HPP FIFO per batch | Cost layer per penerimaan; konsumsi FIFO dalam batch yang keluar; jejak konsumsi; test FIFO | INV-03 | 8 |
| S-215 | Saran FEFO saat picking dan transfer | Batch kedaluwarsa terdekat diusulkan; user bisa memilih manual dengan alasan | INV-09 | 5 |
| S-216 | Sebagai Logistik, saya mentransfer stok | Antar lokasi satu gudang langsung; antar gudang/ke maklon/partner berstatus Dalam Pengiriman sampai diterima | WMS-03, WMS-10 | 8 |
| S-217 | Sebagai Logistik, saya melakukan stock count | Penuh/cycle per lokasi; hitung buta; selisih → approval Head Logistik (> Rp2 jt + Head F&A); penyesuaian + jurnal A-14 | WMS-04 | 8 |
| S-218 | Jadwal stock count dan form HP | Jadwal cycle count, konsinyasi/vending bulanan, penuh akhir tahun; form HP 44 px, autosave, foto; Merchandiser hanya melihat & mengisi lokasi konsinyasi partner yang ditugaskan | WMS-11 | 5 |
| S-219 | Stok di lokasi konsinyasi, buffer vending, mesin, maklon | Saldo per lokasi eksternal terlihat di posisi stok; DO konsinyasi (A-07) dan buffer (A-24) memindahkan stok | WMS-08 | 5 |
| S-220 | Sebagai Staf Gudang, saya mengelola Gudang Retur | QC retur, usulan disposisi per baris, approval sesuai matriks, eksekusi (restock, repack, reprocess, sampel, musnah) + jurnal A-26 | WMS-07 | 8 |
| S-221 | Sebagai Marketing/HRGA, saya meminta pemakaian internal FG | Tujuan (marketing/sampel karyawan/sampling sales), kode kampanye, approval; release oleh Logistik Leader; keluar FEFO; serah terima dikonfirmasi peminta; jurnal A-11/A-12 | INV-05, HR-02 | 8 |
| S-222 | Sebagai Logistik, saya melakukan repacking | Input barang retur + packaging baru, output FG batch baru; nilai = HPP retur + packaging | INV-06 | 5 |
| S-223 | Sebagai Logistik, saya memusnahkan barang dengan berita acara | Alasan, metode, saksi, foto; approval berjenjang; jurnal A-26/A-27; PDF berita acara | INV-12 | 5 |
| S-224 | Backdate di periode terbuka memicu recost | Dokumen biaya bertanggal mundur → job recost SKU terdampak → jurnal penyesuaian; periode tertutup ditolak | ACC-14 (dasar), TG 5.8.4 | 8 |
| S-225 | Jurnal otomatis pembelian & persediaan tervalidasi | Test balance A-01, A-02, A-03, A-04, A-11, A-12, A-14, A-18, A-19, A-26, A-27 dengan akun & dimensi benar | ACC-02 | 5 |
| S-226 | Sebagai Fulfillment Leader, saya meminta Petty Stock | Ajukan transfer FG Non-Ecommerce → Petty Stock; release Logistik Leader (stok asal reserved); Staf Gudang siapkan; Fulfillment cek ulang & konfirmasi qty diterima; sisa reserved dilepas | WMS-03 | 5 |
| S-227 | Sebagai Marketing/Sales, saya membawa barang ke event dan mengembalikan sisanya | Tujuan event dengan tanggal selesai; stok pindah ke lokasi Event tanpa jurnal; catat sisa maks H+1 (pengingat job harian); sisa masuk Inbound & dicek; jurnal A-11 hanya atas qty terpakai; test qty kirim = terpakai + sisa | INV-05 | 8 |
| | **Total R1.2** | | | **173** |

## R1.3 — Sales Management & Logistics (16 requirement)

| ID | User story | Kriteria penerimaan | Req | SP |
|---|---|---|---|---|
| S-301 | Sebagai Sales MT, saya mencatat PO klien MT beli putus & B2B | SO dengan harga otomatis; konfirmasi → alokasi FEFO Non-Ecommerce; Proforma terbit | SLS-01, SLS-06 | 8 |
| S-302 | Proforma dan invoice NON-PKP | PI dengan masa berlaku 14 hari (pengaturan); kedaluwarsa melepas alokasi; pembayaran dikonfirmasi → invoice lunas (A-05); PDF dengan keterangan NON-PKP | SLS-02, NFR-08 | 8 |
| S-303 | Sebagai Sales MT, saya mengirim barang konsinyasi | DO konsinyasi hanya jika PKS berlaku; stok pindah ke lokasi partner (A-07) | SLS-03 | 5 |
| S-304 | Sebagai Sales MT, saya merekonsiliasi konsinyasi bulanan | Cut-off, stock count partner, rumus terjual, banding laporan partner, unggah BA; approval Sales MT → Head F&A | SLS-03 | 13 |
| S-305 | Rekonsiliasi menerbitkan invoice otomatis & memproses selisih | Invoice termin PKS (A-08); selisih per penyebab sesuai PKS: ditagih partner atau beban Balans (A-23); PDF BA | SLS-03, SLS-09, NFR-08 | 8 |
| S-306 | Sebagai CS, saya mencatat pesanan WhatsApp/Instagram | Cari pelanggan by HP; item & ongkir; unggah bukti bayar; F&A konfirmasi; kirim invoice (link PDF/teks WA) | SLS-04 | 8 |
| S-307 | Sebagai Sales E-commerce, saya mencatat penjualan marketplace | Input manual dan impor Excel per marketplace; order eksternal tidak ganda; potong E-commerce Stock; bundel dipecah | SLS-05 | 8 |
| S-308 | Penjualan karyawan dengan harga karyawan | Pelanggan karyawan otomatis memakai price list karyawan | SLS-07 | 3 |
| S-309 | Sebagai Logistik Leader, saya membuat Delivery Order | DO dari SO/konsinyasi; release oleh Logistik Leader; cetak PDF tanpa harga dengan batch & ED; status disiapkan–dikirim–diterima | LOG-01, LOG-04, NFR-08 | 5 |
| S-310 | Sebagai Staf Gudang dan Fulfillment, kami memproses Outbound dengan QC | Picking list per batch dan packing oleh Staf Gudang; QC oleh Fulfillment Leader/Merchandiser, ditolak jika user sama dengan penyiap; gagal QC → Karantina + stok pengganti FEFO otomatis; kurang stok → baris ditahan + notifikasi | WMS-12 | 13 |
| S-311 | Barang reject QC Outbound dikembalikan ke maklon | Retur pembelian otomatis dari Karantina tanpa approval; nota debit; jurnal A-02 | WMS-12, ACC-16 | 3 |
| S-312 | Sebagai Logistik, saya mencatat kurir, resi, dan barcode marketplace | Kurir dari master; resi manual; AWB marketplace; tracking status manual; kurir internal wajib memilih Merchandiser, yang mengonfirmasi diterima dengan foto/tanda terima dari HP | LOG-02, LOG-03 | 5 |
| S-313 | Pengakuan penjualan & HPP per channel | Marketplace saat serah kurir (A-10 + A-06); MT saat invoice; konsinyasi saat rekonsiliasi; test per channel | ACC-02 | 5 |
| S-314 | Sebagai CS/Sales, saya memproses retur penjualan | Retur masuk Inbound → Gudang Retur; alasan dari master; nota kredit atau refund; jurnal A-13 | SLS-08 | 8 |
| S-315 | Sebagai F&A, saya mengelola piutang dan pembayaran | Piutang per invoice; pembayaran sebagian; satu pembayaran dialokasikan ke banyak invoice; umur piutang | ACC-03, ACC-15 | 8 |
| S-316 | Sebagai Staf Gudang, saya mencetak label koli dan label produk mitra | Koli dicatat saat packing; label koli per koli (DO, n/total, penerima, isi); label produk memakai kode produk mitra bila ada, peringatan bila tidak ada | LOG-01 | 3 |
| S-317 | Sebagai Logistik Leader, saya memverifikasi retur MT sebelum barang diambil | Retur MT/reseller: ajukan (Sales) → verifikasi atau kembalikan dengan catatan (Logistik Leader) → tugaskan Merchandiser bila pickup → terima: cek ulang, foto & form retur → Inbound | SLS-08 | 5 |
| | **Total R1.3** | | | **116** |

## R1.4 — Finance Close, Pajak & Laporan (31 requirement)

| ID | User story | Kriteria penerimaan | Req | SP |
|---|---|---|---|---|
| S-401 | Sebagai F&A, saya mengelola kas & bank | Rekening bank ↔ akun; pembayaran masuk/keluar; mutasi per rekening | ACC-05 | 5 |
| S-402 | Sebagai F&A, saya merekonsiliasi bank dari file mutasi | Impor CSV/Excel bank; matching referensi → nominal+tanggal → manual; konfirmasi usulan | ACC-05 | 8 |
| S-403 | Sebagai F&A, saya merekonsiliasi settlement marketplace via impor | Impor laporan settlement; cocokkan per order; jurnal dana cair (A-10s): bank + fee + promo | ACC-06 | 8 |
| S-404 | Sebagai divisi, saya mengajukan biaya operasional | Jenis biaya dari master; bukti wajib; approval per nominal; verifikasi F&A + PPh; jurnal A-15; bayar | ACC-11 | 8 |
| S-405 | Sebagai pemegang kas kecil, saya mencatat pengeluaran | Plafon & maks. per transaksi dari pengaturan; bukti wajib; jurnal A-17; pengisian kembali dengan approval (A-16) | ACC-12 | 5 |
| S-406 | Pemotongan PPh 21/23/4(2) dan rekap Coretax | Tarif effective-dated; potongan di tagihan pemasok & pengajuan biaya; utang pajak; rekap bukti potong per masa (ekspor) ; penyetoran (A-29) | ACC-19 | 8 |
| S-407 | PPh Final UMKM atas omzet bulanan | Bisa diaktifkan; hitung dari omzet bruto; jurnal A-30; pemantauan batas Rp4,8 M | ACC-20 | 3 |
| S-408 | Register aset tetap & penyusutan otomatis | Aset per kelas & lokasi; penyusutan garis lurus bulanan via job (A-31), idempotent per periode | ACC-21 | 5 |
| S-409 | Laporan keuangan | Neraca, Laba Rugi, Arus Kas per periode; ekspor Excel | ACC-07, RPT-12 | 8 |
| S-410 | Neraca saldo & buku besar detail | Per periode & dimensi; drill-down ke jurnal & dokumen sumber | ACC-18 | 5 |
| S-411 | Tutup buku dengan checklist | 9 langkah dengan status & PIC; tutup periode (re-auth, advisory lock); buka kembali dengan approval Head F&A → Direksi | ACC-08, ACC-14 | 8 |
| S-412 | Input saldo awal | Impor/entri saldo awal per akun (A-36), piutang/utang terbuka, aset; pengecekan rekonsiliasi cut-over | ACC-14 | 5 |
| S-413 | Impor/ekspor transaksi Excel | Template per jenis transaksi; validasi & preview seperti master | ACC-10, INT-02 | 5 |
| S-414 | Laporan stok, mutasi, stock count, HPP, kedaluwarsa | RPT-01, 02, 03, 04, 19 dari read model; filter lokasi/SKU/batch; ekspor sebagai job | RPT-01–04, RPT-19 | 13 |
| S-415 | Laporan penjualan, konsinyasi, pemakaian internal, retur | RPT-05, 06, 08, 09 per channel/periode/kampanye | RPT-05, 06, 08, 09 | 8 |
| S-416 | Outstanding PO, umur piutang/utang, laporan pajak | RPT-10, 11, 18 | RPT-10, 11, 18 | 5 |
| S-417 | Laba rugi per dimensi & margin per produk | RPT-15 per channel, sub-channel, divisi; margin kotor per produk/kategori | RPT-15 | 8 |
| S-418 | Dashboard eksekutif dasar | Penjualan, margin, stok, kas; materialized view di-refresh job | RPT-14 | 5 |
| S-419 | Kesiapan produksi | Uptime monitor & alert aktif; uji restore bulanan tercatat; batas memori container; uji beban ≥ 40 user back-office | NFR-06, 07, 09, 10 | 5 |
| | **Total R1.4** | | | **125** |

---

## Ringkasan & Kalibrasi Kapasitas (F0-APR-05)

| Rilis | Requirement | Story | SP |
|---|---|---|---|
| R1.1 | 25 | 34 | 179 |
| R1.2 | 33 | 27 | 173 |
| R1.3 | 16 | 17 | 116 |
| R1.4 | 31 | 19 | 125 |
| **Fase 1** | **105** | **97** | **593** |

Opsi Fase 1 yang menambah backlog jika diambil (Roadmap bagian 11): U-28 sisa umur simpan (+5 SP, R1.2), U-29 kitting (+8 SP, R1.2), U-30 batas kredit & termin (+8 SP, R1.3).

**Cara kalibrasi:**

1. Steering committee menetapkan komposisi tim final (asumsi Roadmap: PM/BA, UI/UX, 2 frontend, 2 backend, QA).
2. Sprint pertama Fase 1 dijadikan sprint pengukuran: velocity aktual = SP yang memenuhi Definition of Done.
3. Jumlah sprint per rilis = SP rilis ÷ velocity, ditambah satu sprint stabilisasi sebelum UAT.
4. Jika lebih dari tiga opsi Fase 1 dipilih, ulangi langkah 3 sebelum exit Fase 0.

Urutan pengerjaan di dalam rilis mengikuti ketergantungan: S-101–S-112 dan S-130 lebih dulu (fondasi), lalu master data; di R1.2 ledger stok (S-213, S-214) sebelum dokumen gudang; di R1.3 SO dan alokasi sebelum Outbound dan rekonsiliasi.
