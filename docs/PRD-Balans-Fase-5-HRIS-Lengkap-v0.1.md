# Product Requirements Document (PRD) — Fase 5
## Balans Nutrition — Unified Business Platform: HRIS Lengkap

| Item | Keterangan |
|---|---|
| Klien | Balans Nutrition |
| Dokumen | Product Requirements Document Fase 5 — HRIS Lengkap |
| Versi | 0.1 — Draft |
| Tanggal | 17 Oktober 2026 |
| Acuan | BRD Balans Unified Business Platform v0.7 (bagian 5.13, 5.14, 5.16, 5.17, 6.12, 7.2, 10, 12 — U-13, U-22, U-23, U-31); Roadmap v0.6 (bagian 4, 10, 11, 13, D-14); PRD Fase 1 v0.1 (F1-119, F1-403); PRD Fase 3 v0.1 (F3-204); Desain Finance & Accounting Fase 1; Desain Role & Approval Fase 1; Technical Guideline v0.1 |
| Status | Draft untuk direview tim implementasi dan Balans |

PRD ini menjabarkan **apa yang harus dihasilkan Fase 5**, perilaku yang diharapkan dari setiap fitur, kriteria penerimaannya, dan syarat go-live tiap rilis. Rincian per user story disusun di Backlog Fase 5 setelah PRD ini disetujui. Sesuai Roadmap (D-18), dokumen ini **tidak memuat target tanggal**. Yang ditetapkan adalah isi, urutan, dan syarat selesai.

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
10. [Dokumen, Ekspor & Laporan](#10-dokumen-ekspor--laporan)
11. [UAT, Migrasi, Paralel Run & Go-live](#11-uat-migrasi-paralel-run--go-live)
12. [Entry, Exit & Definition of Done](#12-entry-exit--definition-of-done)
13. [Risiko & Mitigasi](#13-risiko--mitigasi)
14. [Asumsi & Batasan](#14-asumsi--batasan)
15. [Keputusan & Isu Terbuka](#15-keputusan--isu-terbuka)
16. [Keterlacakan](#16-keterlacakan)
17. [Persetujuan](#17-persetujuan)

---

## 1. Ringkasan

Fase 5 menghadirkan **HRIS lengkap sampai absensi, termasuk payroll** (arahan Balans P-22, U-22) untuk 50+ karyawan dengan kapasitas hingga 100 karyawan (U-23). Fase ini dibangun di atas data karyawan dasar Fase 1 (HR-01) dan COA yang sudah berjalan, sehingga payroll langsung menghasilkan jurnal per divisi (OBJ-09).

Fase 5 dibagi dua rilis (D-14). **Absensi dan cuti bisa dipakai lebih dulu**, sedangkan payroll butuh satu periode paralel run sebelum dipakai resmi.

| Rilis | Fokus | Hasil yang bisa didemokan di staging | Requirement | Must / Should |
|---|---|---|---|---|
| **R5.1** Data Karyawan, Absensi & Cuti | Kehadiran terkelola | Struktur organisasi dan kontrak dengan pengingat; absensi HP dengan geotag dan selfie; jadwal dan shift; cuti, izin, dan lembur dengan approval; self-service karyawan; rekap kehadiran per cut-off | 5 | 3 / 2 |
| **R5.2** Payroll, BPJS & PPh 21 | Gaji terhitung dan terjurnal | Payroll bulanan dari rekap kehadiran; BPJS; PPh 21 TER dan perhitungan akhir tahun; slip gaji; file transfer bank; jurnal payroll per divisi; laporan HR | 5 | 5 / 0 |
| **Fase 5** | | | **10** | **8 / 2** |

Hitungan mengikuti Roadmap bagian 14. HR-12 (self-service) dirilis di R5.1, dan slip gaji di dalamnya tersedia mulai R5.2. RPT-17 dihitung sebagai Must. Usulan opsional U-31 (potong gaji atas pembelian karyawan, A-33) masuk R5.2 hanya jika dipilih di BRD v1.0.

Fase 5 hanya bergantung pada Fase 1 (Roadmap bagian 4). Dengan tambahan satu tim kecil (1 backend, 1 frontend), Fase 5 bisa dikerjakan paralel dengan Fase 3 atau Fase 4 atas keputusan steering committee (Roadmap bagian 10).

---

## 2. Latar Belakang & Masalah

1. **Data karyawan tersebar.** Kontrak, dokumen, rekening, NPWP, dan kepesertaan BPJS disimpan terpisah, dan kontrak yang akan habis baru diketahui mendekati tanggalnya.
2. **Absensi tidak terhubung ke payroll.** Rekap kehadiran, keterlambatan, dan lembur dihitung manual setiap cut-off, sehingga rawan salah dan memakan waktu HRGA.
3. **Karyawan lapangan sulit dipantau kehadirannya.** Sales, Merchandiser, dan Fulfillment bekerja di lokasi mitra dan event, sementara absensi kantor tidak menjangkau mereka.
4. **Cuti dan lembur lewat chat.** Pengajuan dan approval tidak tercatat, dan saldo cuti dihitung manual.
5. **Payroll dihitung di spreadsheet.** Perhitungan BPJS dan PPh 21 metode TER, termasuk perhitungan akhir tahun, bergantung pada rumus manual.
6. **Jurnal payroll dibuat manual.** Beban gaji per divisi belum tercermin otomatis di laba rugi per divisi (OBJ-03, OBJ-08).

---

## 3. Tujuan & Ukuran Keberhasilan

Ukuran keberhasilan diukur selama tiga bulan setelah go-live setiap rilis. Nilai target adalah usulan tim implementasi dan disahkan bersama PRD ini.

| ID | Tujuan | Ukuran keberhasilan | Acuan |
|---|---|---|---|
| G5-01 | Kehadiran tercatat di sistem | 100% karyawan aktif absen lewat sistem; koreksi absensi ≤ 5% dari total catatan absensi per bulan | HR-05 |
| G5-02 | Cuti dan lembur terkendali | 100% cuti, izin, dan lembur diajukan dan disetujui lewat sistem; saldo cuti tidak lagi dihitung manual | HR-06, HR-07 |
| G5-03 | Kontrak tidak terlewat | 100% kontrak PKWT yang habis punya keputusan perpanjangan, pengangkatan, atau berakhir sebelum tanggal habis | HR-04 |
| G5-04 | Payroll akurat | Selisih paralel run setelah koreksi = 0 per karyawan per komponen; tidak ada koreksi payroll karena kesalahan hitung sistem di tiga periode resmi pertama | HR-08 s.d. HR-10, D-14 |
| G5-05 | Payroll tepat waktu | Payroll disetujui Direksi paling lambat H-2 tanggal bayar, dan gaji dibayar sesuai tanggal bayar di setiap periode | BRD 5.16 |
| G5-06 | Jurnal payroll otomatis | 100% payroll final menghasilkan jurnal A-32 per divisi tanpa jurnal manual | HR-11, OBJ-03 |
| G5-07 | Self-service dipakai | ≥ 90% karyawan aktif membuka slip gaji dan mengajukan cuti lewat self-service | HR-12 |

---

## 4. Ruang Lingkup

### 4.1 Dalam Lingkup

| Kode | Modul | Fase 5 mencakup |
|---|---|---|
| HR | HRIS | HR-04 s.d. HR-07, HR-12 (R5.1); HR-08 s.d. HR-11 (R5.2) |
| ACC | Finance & Accounting | Posting rule A-32 (payroll) dan penyetoran PPh 21 serta BPJS lewat A-29; reimbursement lewat pengajuan biaya Fase 1 (ACC-11, U-13) |
| RPT | Laporan | RPT-17 (R5.2) |
| NFR | Non-fungsional | NFR-04 (data gaji hanya untuk HRGA/Payroll dan Direksi), NFR-08 (slip gaji), NFR-10 (≥ 100 karyawan) |

**Usulan opsional** yang masuk Fase 5 hanya jika dipilih di BRD v1.0:

| Usulan | Requirement | Rilis | Isi |
|---|---|---|---|
| U-31 Potong gaji | SLS-11 (potong gaji), A-33 | R5.2 | Pembelian karyawan dibayar lewat potong gaji; piutang karyawan dilunasi di payroll |

### 4.2 Di Luar Lingkup Fase 5

- **Rekrutmen, penilaian kinerja, pelatihan, dan manajemen talenta.** Tidak ada di BRD.
- **Transfer gaji host-to-host ke bank** (BRD 2.2). Sistem menghasilkan file transfer massal untuk diunggah ke internet banking.
- **Pelaporan langsung ke Coretax dan sistem BPJS** (BRD 2.2). Sistem menyiapkan file ekspor sesuai format yang diterima.
- **Pengenalan wajah otomatis** pada selfie absensi. Selfie disimpan sebagai bukti dan diaudit sampling oleh HRGA (bagian 15.1).
- **Mesin absensi sidik jari atau kartu.** Absensi memakai browser HP (BRD 2.2: tanpa aplikasi mobile native).
- **Pengiriman slip gaji lewat WhatsApp** (INT-06 opsional untuk slip). Slip tersedia di self-service.

---

## 5. Pengguna & Role

| Role | Pekerjaan di Fase 5 |
|---|---|
| Karyawan (self-service) | Absen, melihat jadwal dan rekap kehadiran, mengajukan cuti, izin, lembur, koreksi absensi, kasbon, dan reimbursement, melihat saldo cuti dan slip gaji |
| Atasan langsung | Menyetujui cuti, izin, lembur, koreksi absensi, dan tukar shift timnya; melihat kalender dan kehadiran tim |
| HRGA / Payroll | Data karyawan, struktur organisasi, kontrak, jadwal dan shift, tutup rekap kehadiran, komponen gaji, proses payroll, BPJS, PPh 21, slip dan file transfer |
| Head HRGA | Approval cuti > 3 hari, perubahan gaji, kasbon, dan payroll bulanan (level 1) |
| Direksi | Approval payroll bulanan (level 2) dan perubahan gaji; laporan HR |
| Finance & Accounting | Pembayaran gaji dari file transfer, penyetoran PPh 21 dan BPJS, melihat jurnal payroll agregat per divisi (bukan gaji per orang) |
| Admin Sistem | Pembuatan akun self-service untuk karyawan baru, perangkat absensi bersama |

Atasan langsung bukan role tersendiri. Hubungan atasan diambil dari struktur organisasi (F5-101) dan dipakai engine approval Fase 1 (F1-106) sebagai approver dinamis.

**Key user** Fase 5: HRGA/Payroll, Head HRGA, satu atasan dari tiap divisi, Finance & Accounting, dan perwakilan karyawan lapangan (Sales atau Merchandiser).

---

## 6. Kebutuhan Fungsional per Rilis

Setiap fitur di bawah dipecah menjadi user story di Backlog Fase 5. Kriteria penerimaan di tabel adalah perilaku minimal yang diuji di sprint review dan UAT. **Prioritas** mengikuti BRD: M = Must, S = Should.

### 6.1 R5.1 — Data Karyawan, Absensi & Cuti

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F5-101 | Struktur organisasi & jabatan | Divisi (sama dengan dimensi divisi di akuntansi), departemen, jabatan, dan atasan langsung per karyawan; bagan organisasi; lokasi kerja per karyawan; perubahan jabatan, divisi, dan atasan effective-dated dengan riwayat; atasan langsung dipakai sebagai approver dinamis | HR-04 | M |
| F5-102 | Kontrak & dokumen karyawan | Kontrak PKWT/PKWTT dengan tanggal mulai, berakhir, dan masa percobaan; riwayat perpanjangan dan pengangkatan; pengingat 30 hari dan 7 hari sebelum kontrak habis ke HRGA dan atasan; dokumen (KTP, KK, ijazah, kontrak, NPWP) disimpan terenkripsi dan hanya bisa dibuka HRGA; status PTKP, NPWP, nomor BPJS, dan rekening tersimpan dengan field pribadi terenkripsi | HR-04, NFR-04 | M |
| F5-103 | Jadwal kerja & shift | Pola kerja (5 atau 6 hari kerja), shift dengan jam masuk dan pulang serta toleransi terlambat; jadwal per karyawan atau tim; kalender libur nasional dan cuti bersama per tahun; tukar shift diajukan karyawan dan disetujui atasan | HR-05 | M |
| F5-104 | Absensi HP | Clock-in dan clock-out dari browser HP dengan lokasi dan selfie; ditolak jika di luar radius lokasi kerja (nilai awal 100 m), kecuali karyawan berstatus lapangan yang absen di lokasi kunjungan dengan penanda *lapangan*; keterlambatan dan pulang cepat dihitung dari jadwal; mode perangkat bersama di gudang dan kantor (karyawan memilih nama dan memasukkan PIN, selfie tetap wajib); lupa absen → koreksi absensi dengan alasan dan approval atasan | HR-05 | M |
| F5-105 | Cuti & izin | Jenis cuti dan izin sesuai ketentuan ketenagakerjaan dan kebijakan Balans (tahunan, sakit dengan surat dokter, melahirkan, menikah, duka, izin tidak dibayar, dan lainnya); saldo cuti tahunan dihitung otomatis; cuti bersama memotong saldo; approval atasan, ditambah Head HRGA untuk cuti > 3 hari (BRD 5.17); kalender tim; pengajuan tumpang tindih dengan cuti yang sudah disetujui ditolak | HR-06 | M |
| F5-106 | Lembur | Pengajuan sebelum lembur dengan rencana jam dan alasan; approval atasan; jam lembur yang dibayar = jam yang lebih kecil antara rencana dan realisasi dari absensi; upah lembur dihitung sesuai PP 35/2021 dari parameter effective-dated | HR-07 | S |
| F5-107 | Self-service karyawan | Setiap karyawan aktif punya akun ERP dengan role Karyawan yang hanya melihat datanya sendiri; profil, jadwal, rekap absensi, saldo cuti; pengajuan cuti, izin, lembur, koreksi absensi, dan tukar shift dengan status approval; reimbursement diajukan lewat pengajuan biaya Fase 1 (F1-403) dengan jenis biaya reimbursement; akun dinonaktifkan otomatis di hari terakhir kerja | HR-12, U-13 | S |
| F5-108 | Rekap kehadiran per cut-off | Rekap per karyawan per periode cut-off (nilai awal tanggal 21 s.d. 20): hari kerja, hadir, terlambat, pulang cepat, alpa, cuti per jenis, izin tidak dibayar, jam lembur dibayar; HRGA menutup rekap setelah semua koreksi dan pengajuan dalam periode selesai; rekap tertutup tidak bisa diubah dan menjadi input payroll | HR-05, HR-08 | M |

**Deliverable R5.1:** seluruh karyawan aktif absen, mengajukan cuti, izin, dan lembur lewat sistem, dan rekap kehadiran per cut-off tersedia tanpa olah data di luar sistem.

### 6.2 R5.2 — Payroll, BPJS & PPh 21

**Tahapan:** build → paralel run satu periode payroll → payroll resmi di periode berikutnya (D-14).

| ID | Fitur | Kriteria penerimaan utama | Req | Prior. |
|---|---|---|---|---|
| F5-201 | Komponen gaji | Master komponen: gaji pokok, tunjangan tetap, tunjangan tidak tetap, lembur, potongan; sifat tiap komponen (objek PPh 21, dasar BPJS, prorata); nilai komponen per karyawan effective-dated; perubahan gaji diajukan HRGA dan disetujui Head HRGA → Direksi; data gaji hanya bisa dilihat HRGA/Payroll dan Direksi | HR-08, NFR-04 | M |
| F5-202 | Kasbon karyawan | Pengajuan kasbon dari self-service dengan jumlah dan jumlah cicilan; approval atasan → Head HRGA, ditambah Head F&A di atas Rp5 juta; pencairan dicatat ke Piutang Karyawan (1221); cicilan otomatis dipotong di payroll sampai lunas | HR-08 | M |
| F5-203 | Proses payroll bulanan | Periode payroll dibuat dari rekap kehadiran yang sudah ditutup; perhitungan gaji, tunjangan, lembur, potongan absen dan izin tidak dibayar, cicilan kasbon, reimbursement yang dibayar lewat payroll, dan prorata karyawan masuk atau keluar di tengah periode; alur *Draf → Dihitung → Diajukan → Disetujui → Final*; approval Head HRGA → Direksi (BRD 5.17); payroll final terkunci, dan koreksi dilakukan sebagai rapel di periode berikutnya | HR-08 | M |
| F5-204 | BPJS | BPJS Kesehatan dan Ketenagakerjaan (JHT, JKK, JKM, JP) dengan porsi perusahaan dan karyawan; tarif, batas upah, dan kelas risiko JKK sebagai parameter effective-dated; iuran per karyawan per periode; ekspor rekap iuran ke Excel untuk pelaporan BPJS | HR-09 | M |
| F5-205 | PPh 21 | PPh 21 bulanan metode TER (kategori sesuai status PTKP) untuk Januari s.d. November; perhitungan tahunan dengan tarif Pasal 17 di masa pajak terakhir (Desember atau bulan karyawan keluar); tarif dan PTKP sebagai parameter effective-dated; metode gross sebagai default dan tunjangan pajak (gross-up) bisa diaktifkan per karyawan; data bukti potong bulanan dan tahunan diekspor dalam format impor Coretax | HR-10, U-06 | M |
| F5-206 | Slip gaji & pembayaran | Slip gaji PDF per karyawan di self-service, dibuka dengan re-autentikasi; file transfer massal sesuai profil format bank Balans; status pembayaran per karyawan; rekening tujuan wajib atas nama karyawan | HR-08, HR-12, NFR-08 | M |
| F5-207 | Jurnal payroll | Payroll final menghasilkan jurnal A-32 per divisi: debit Beban Gaji & Tunjangan (6201) dan Beban BPJS porsi perusahaan (6202); kredit Utang Gaji (2201), Utang BPJS (2202), Utang PPh 21 (2301), Piutang Karyawan (1221) untuk cicilan kasbon; pembayaran gaji mendebit Utang Gaji; penyetoran PPh 21 dan BPJS lewat A-29; F&A hanya melihat total per divisi | HR-11, ACC-02 | M |
| F5-208 | Paralel run | Payroll periode pertama dihitung di sistem paralel dengan cara lama; hasil cara lama diimpor per karyawan per komponen; laporan perbandingan menunjukkan selisih per karyawan dan komponen; payroll paralel tidak membuat jurnal dan tidak membuat slip untuk karyawan | D-14 | M |
| F5-209 | Laporan HR | Jumlah karyawan per divisi, status kontrak, dan lokasi; karyawan masuk dan keluar (turnover); kehadiran, keterlambatan, dan alpa; cuti dan saldo cuti; lembur per divisi; rekap payroll per divisi dan komponen; rekap BPJS dan PPh 21; kontrak yang akan habis | RPT-17 | M |

**Deliverable R5.2:** payroll resmi berjalan setelah satu periode paralel run, lengkap dengan BPJS, PPh 21, slip gaji, file transfer, dan jurnal per divisi.

---

## 7. Aturan Bisnis Lintas Modul

Aturan Fase 1 tetap berlaku. Fase 5 menambah aturan berikut.

| # | Aturan | Acuan |
|---|---|---|
| 1 | **Data gaji hanya untuk HRGA/Payroll dan Direksi.** Finance & Accounting melihat jurnal agregat per divisi. Karyawan hanya melihat slipnya sendiri | NFR-04 |
| 2 | **Data karyawan Fase 1 tetap satu-satunya sumber status karyawan.** Status aktif atau resign mengatur harga karyawan (F1-119), akun self-service, dan tautan akun website karyawan | HR-01, HR-03 |
| 3 | **Payroll hanya dihitung dari rekap kehadiran yang sudah ditutup.** Pengajuan yang disetujui setelah rekap ditutup masuk periode berikutnya | F5-108 |
| 4 | **Payroll final tidak bisa diubah.** Koreksi dilakukan sebagai rapel di periode berikutnya dan tercatat di audit trail | HR-08, ACC-13 |
| 5 | **Tarif BPJS, PPh 21, PTKP, dan upah lembur adalah parameter effective-dated.** Perubahan regulasi ditangani lewat konfigurasi, bukan pengembangan | Roadmap risiko #9 |
| 6 | **Approval cuti, izin, dan lembur mengikuti atasan langsung** dari struktur organisasi, dengan delegasi saat atasan cuti (F1-106) | BRD 5.17 |
| 7 | **Pembuat tidak menyetujui pengajuannya sendiri.** Pengajuan atasan diteruskan ke atasannya | Desain Role & Approval |
| 8 | **Absensi wajib lokasi dan selfie.** Absensi di luar radius hanya untuk karyawan berstatus lapangan, dan diberi penanda | HR-05 |
| 9 | **Jurnal payroll membawa dimensi divisi** dari divisi karyawan pada akhir periode payroll | HR-11, ACC-17 |
| 10 | **Rekening tujuan gaji atas nama karyawan.** Perubahan rekening butuh re-autentikasi dan dikonfirmasi HRGA | NFR-04 |

**Approval di Fase 5** (dari BRD 5.17 dan tambahan Fase 5; batas dan approver bisa dikonfigurasi):

| Dokumen | Level 1 | Level 2 | Level 3 |
|---|---|---|---|
| Cuti, izin, lembur, koreksi absensi, tukar shift | Atasan langsung | Cuti > 3 hari: + Head HRGA | — |
| Kasbon | Atasan langsung | Head HRGA | > Rp5 jt: + Head Finance & Accounting |
| Perubahan gaji dan komponen tetap | Head HRGA | Direksi | — |
| Payroll bulanan | Head HRGA | Direksi | — |

---

## 8. Alur End-to-End & Skenario UAT

Setiap skenario dinyatakan lulus jika data, perhitungan, jurnal, dan laporan terkait sesuai hasil yang diharapkan, dan key user menandatanganinya.

| ID | Rilis | Skenario | Divisi penguji | Hasil yang diperiksa |
|---|---|---|---|---|
| UAT5-01 | R5.1 | Karyawan baru → kontrak PKWT 1 tahun → akun self-service → pengingat 30 hari sebelum habis → perpanjangan | HRGA | Riwayat kontrak, pengingat, akun aktif |
| UAT5-02 | R5.1 | Absen di kantor dalam radius, di luar radius (ditolak), sebagai karyawan lapangan di lokasi mitra, dan dari perangkat bersama di gudang | HRGA, Sales, Logistik | Lokasi, selfie, penanda lapangan, keterlambatan |
| UAT5-03 | R5.1 | Lupa clock-out → koreksi absensi → approval atasan | Karyawan, atasan | Catatan koreksi dan audit |
| UAT5-04 | R5.1 | Cuti 2 hari dan cuti 5 hari; atasan sedang cuti dan mendelegasikan; pengajuan tumpang tindih | Karyawan, atasan, Head HRGA | Approval berjenjang, delegasi, saldo cuti |
| UAT5-05 | R5.1 | Lembur hari kerja dan hari libur dengan realisasi absensi lebih pendek dari rencana | Karyawan, atasan | Jam dibayar, perhitungan upah lembur |
| UAT5-06 | R5.1 | Tutup rekap kehadiran → pengajuan disetujui setelah tutup | HRGA | Rekap terkunci, pengajuan masuk periode berikutnya |
| UAT5-07 | R5.1 | Karyawan resign → hari terakhir kerja | HRGA | Akun self-service nonaktif, harga karyawan hilang |
| UAT5-08 | R5.2 | Payroll satu periode: karyawan tetap, karyawan masuk di tengah periode, karyawan keluar, lembur, izin tidak dibayar, kasbon, reimbursement | HRGA, Head HRGA, Direksi | Prorata, potongan, approval, payroll final terkunci |
| UAT5-09 | R5.2 | BPJS untuk karyawan dengan gaji di bawah dan di atas batas upah | HRGA | Iuran per program, porsi perusahaan dan karyawan |
| UAT5-10 | R5.2 | PPh 21 TER untuk beberapa status PTKP; perhitungan tahunan untuk karyawan yang keluar dan di bulan Desember; karyawan dengan gross-up | HRGA, F&A | Nilai sama dengan perhitungan referensi; ekspor bukti potong |
| UAT5-11 | R5.2 | Slip gaji di self-service dengan re-autentikasi; file transfer bank; pembayaran | Karyawan, F&A | Slip hanya milik sendiri, file sesuai format bank |
| UAT5-12 | R5.2 | Jurnal payroll per divisi → penyetoran PPh 21 dan BPJS | F&A | Jurnal A-32 dan A-29 balance dengan dimensi divisi |
| UAT5-13 | R5.2 | Paralel run satu periode dan laporan perbandingan | HRGA, F&A | Selisih per karyawan per komponen terjelaskan |
| UAT5-14 | R5.2 | Koreksi payroll final lewat rapel periode berikutnya | HRGA | Payroll lama tidak berubah, rapel tercatat |
| UAT5-15 | Semua | Uji akses: karyawan membuka data rekan, atasan membuka gaji timnya, F&A membuka gaji per orang | Semua | API menolak 403/404, audit akses data gaji |

---

## 9. Kebutuhan Non-Fungsional

| NFR | Kebutuhan Fase 5 | Cara uji |
|---|---|---|
| NFR-01 Platform | Absensi dan self-service nyaman di HP dengan target sentuh ≥ 44 px; absensi selesai ≤ 10 detik di jaringan 4G | Uji di HP Android kelas menengah dan iPhone |
| NFR-02 Keamanan | Re-autentikasi untuk membuka slip gaji, mengubah rekening, dan menyetujui payroll; MFA wajib untuk HRGA/Payroll, Head HRGA, dan Direksi (sudah berlaku sejak Fase 1) | Test API & E2E |
| NFR-03 Audit | Setiap akses dan perubahan data gaji, rekening, dan dokumen karyawan tercatat di audit trail | Test audit |
| NFR-04 Privasi | NIK, NPWP, rekening, nomor BPJS, dan nilai gaji terenkripsi; dokumen karyawan di object storage terenkripsi; foto selfie dihapus otomatis setelah masa retensi; lokasi hanya diambil saat absen | Review kode + test |
| NFR-08 Dokumen | Slip gaji PDF dengan logo Balans | Review layout oleh HRGA |
| NFR-10 Kapasitas | ≥ 100 karyawan tanpa perubahan desain; perhitungan payroll 100 karyawan ≤ 1 menit sebagai job; 100 absensi bersamaan di jam masuk tanpa penurunan kinerja ERP | Uji beban |

---

## 10. Dokumen, Ekspor & Laporan

### 10.1 Dokumen & Ekspor

| Keluaran | Format | Pengguna |
|---|---|---|
| Slip gaji | PDF di self-service | Karyawan |
| File transfer gaji massal | Format bank Balans (profil kolom yang bisa dikonfigurasi) | F&A |
| Rekap iuran BPJS | Excel untuk pelaporan BPJS | HRGA |
| Bukti potong PPh 21 bulanan dan tahunan | File format impor Coretax | HRGA, F&A |
| Surat keterangan kerja dan kontrak | PDF dari template | HRGA |

### 10.2 Laporan

| ID | Laporan | Pengguna utama |
|---|---|---|
| RPT-17 | Laporan HR (F5-209) | HRGA, Direksi |
| RPT-15 (perluasan) | Laba rugi per divisi dengan beban gaji dan BPJS dari payroll | F&A, Manajemen |
| RPT-18 (perluasan) | Rekap PPh 21 pegawai bersama PPh dipotong lainnya | F&A |

---

## 11. UAT, Migrasi, Paralel Run & Go-live

### 11.1 Tahapan

| Rilis | Tahap | Aktivitas | Selesai jika |
|---|---|---|---|
| R5.1 | Build & UAT | Sprint R5.1; UAT5-01 s.d. UAT5-07 | Semua skenario lulus dan ditandatangani |
| | Migrasi | Struktur organisasi, jabatan, atasan, kontrak, dokumen, lokasi kerja, jadwal, dan saldo cuti per tanggal go-live diimpor lewat template | Data lengkap untuk 100% karyawan aktif; saldo cuti disetujui Head HRGA |
| | Training & go-live | Training karyawan per divisi (absen dan pengajuan), atasan (approval), HRGA; go-live di awal periode cut-off | Go/no-go R5.1 terpenuhi |
| R5.2 | Build & UAT | Sprint R5.2; UAT5-08 s.d. UAT5-12, UAT5-14 | Semua skenario lulus |
| | Migrasi | Komponen gaji, status PTKP, NPWP, nomor BPJS, rekening, saldo kasbon, serta penghasilan bruto dan PPh 21 yang sudah dipotong sejak Januari tahun berjalan per karyawan | Data cocok dengan payroll terakhir cara lama |
| | Paralel run | Satu periode payroll dihitung di sistem bersamaan dengan cara lama (UAT5-13) | Semua selisih terjelaskan dan diperbaiki |
| | Payroll resmi | Periode berikutnya memakai sistem; slip, file transfer, dan jurnal dari sistem | Go/no-go R5.2 terpenuhi |

### 11.2 Go / No-Go

**Umum:** tidak ada defect kritis yang terbuka; defect tinggi punya workaround tertulis yang disetujui key user; key user rilis menandatangani UAT; user terkait sudah ditraining; backup dan uji restore terakhir berhasil.

| Rilis | Butir khusus |
|---|---|
| R5.1 | Lokasi kerja dan radius terkonfigurasi; perangkat bersama di gudang siap; semua karyawan aktif punya akun dan sudah mencoba absen |
| R5.2 | Paralel run selesai dengan selisih 0 setelah koreksi; tarif BPJS, PPh 21, dan PTKP tahun berjalan diverifikasi HRGA dan F&A; data penghasilan tahun berjalan per karyawan disetujui Head HRGA |

### 11.3 Rencana Mundur

- **R5.1:** jika absensi HP terganggu lebih dari satu hari kerja, HRGA mencatat kehadiran sementara lewat koreksi absensi massal dari daftar hadir manual.
- **R5.2:** jika payroll resmi pertama tidak bisa difinalkan sebelum H-2 tanggal bayar karena masalah sistem, gaji periode itu dibayar dengan cara lama, dan hasilnya dicatat sebagai jurnal manual oleh F&A. Payroll sistem dilanjutkan di periode berikutnya setelah perbaikan diverifikasi.
- Rollback aplikasi mengikuti Runbook Operasi.

---

## 12. Entry, Exit & Definition of Done

### 12.1 Entry Criteria

| # | Kriteria | Berlaku untuk |
|---|---|---|
| 1 | Fase 1 sudah go-live; data karyawan dasar (HR-01) dan COA berjalan | Fase 5 |
| 2 | PRD Fase 5 disetujui; Backlog Fase 5 terestimasi; keputusan steering committee soal urutan atau opsi paralel | Fase 5 |
| 3 | Data karyawan, kebijakan cuti dan lembur, lokasi kerja untuk radius absensi (Roadmap bagian 13) | R5.1 |
| 4 | Komponen gaji, kepesertaan BPJS, dan data penghasilan tahun berjalan per karyawan | R5.2 |

### 12.2 Exit per Rilis

| Rilis | Exit criteria |
|---|---|
| R5.1 | Semua story R5.1 memenuhi Definition of Done; UAT5-01 s.d. UAT5-07 lulus; satu periode rekap kehadiran ditutup di production |
| R5.2 | Semua story R5.2 memenuhi Definition of Done; UAT5-08 s.d. UAT5-15 lulus; paralel run selesai; payroll resmi pertama final, dibayar, dan terjurnal; F&A memvalidasi jurnal A-32 |

### 12.3 Definition of Done Story

Sama dengan Fase 1 (PRD Fase 1 bagian 12.3), dengan gate coverage 80% juga berlaku untuk package perhitungan payroll, BPJS, dan PPh 21. Setiap aturan perhitungan punya test dengan contoh perhitungan referensi yang disetujui HRGA.

### 12.4 Exit Fase 5

Fase 5 ditutup di steering committee setelah tiga periode payroll resmi berjalan, ukuran keberhasilan G5-04, G5-05, dan G5-06 tercapai, dan daftar defect tidak memuat defect kritis.

---

## 13. Risiko & Mitigasi

| # | Risiko | Dampak | Mitigasi |
|---|---|---|---|
| 1 | Regulasi PPh 21, BPJS, atau upah lembur berubah | Perhitungan salah | Tarif sebagai parameter effective-dated; paralel run; perubahan regulasi ditangani sebagai pemeliharaan (Roadmap risiko #9) |
| 2 | Data gaji bocor | Pelanggaran privasi, konflik internal | Akses terbatas, enkripsi, re-autentikasi, audit akses, MFA untuk role HR |
| 3 | Titip absen atau lokasi palsu | Kehadiran tidak akurat | Selfie wajib, radius, audit sampling selfie oleh HRGA, penanda absensi lapangan |
| 4 | Karyawan tidak punya HP yang memadai | Tidak bisa absen | Mode perangkat bersama di gudang dan kantor |
| 5 | Data penghasilan tahun berjalan tidak lengkap | Perhitungan PPh 21 akhir tahun salah | Migrasi data penghasilan sejak Januari per karyawan dengan persetujuan Head HRGA; paralel run |
| 6 | Atasan terlambat menyetujui | Rekap kehadiran tidak bisa ditutup | Pengingat approval, delegasi, batas tanggal pengajuan sebelum cut-off |
| 7 | Payroll paralel menemukan banyak selisih | Payroll resmi tertunda | Perhitungan referensi disetujui HRGA sejak sprint pertama R5.2; paralel run tambahan satu periode jika selisih belum terjelaskan |
| 8 | Kapasitas VPS saat jam absen pagi | Absensi lambat | Upload selfie terkompresi di perangkat; uji beban 100 absensi bersamaan |

---

## 14. Asumsi & Batasan

1. Fase 5 dikerjakan tim ERP setelah Fase 4, atau paralel dengan Fase 3 atau Fase 4 jika steering committee menyetujui tambahan tim (Roadmap bagian 10).
2. Setiap titik go-live (R5.1 dan R5.2) menaikkan satu versi minor aplikasi (D-29).
3. Jumlah karyawan 50+ dengan kapasitas desain 100 (U-23). Satu badan usaha dan satu NPWP pemotong.
4. Semua karyawan dibayar bulanan. Karyawan harian atau borongan tidak ada di BRD; jika ada, ditambah lewat change request.
5. Cut-off absensi tanggal 20 dan tanggal bayar 25 adalah nilai awal yang bisa diubah (BRD 5.16).
6. Ketentuan cuti, izin, dan lembur mengikuti peraturan ketenagakerjaan yang berlaku dan peraturan perusahaan Balans. Jenis dan kuota cuti dikonfigurasi HRGA.
7. Reimbursement memakai pengajuan biaya Fase 1. Pembayarannya bisa lewat transfer mingguan atau ikut payroll (BRD 5.14).
8. Karyawan yang juga customer tetap memakai harga karyawan Fase 1. Potong gaji untuk pembelian hanya berlaku jika U-31 dipilih.

---

## 15. Keputusan & Isu Terbuka

### 15.1 Keputusan

| # | Topik | Keputusan | Alasan |
|---|---|---|---|
| 1 | Radius absensi | Nilai awal 100 m dari titik lokasi kerja, bisa diatur per lokasi | Cukup untuk ketidakakuratan GPS di dalam gedung tanpa membuka celah absen dari luar area kerja |
| 2 | Verifikasi selfie | Tanpa pengenalan wajah otomatis. Selfie disimpan sebagai bukti, HRGA mengaudit sampel acak setiap minggu, dan foto dihapus otomatis setelah 12 bulan | Pengenalan wajah menambah data biometrik yang berisiko di bawah UU PDP dan biaya, sementara audit sampling cukup untuk 50 – 100 karyawan |
| 3 | Karyawan tanpa HP memadai | Mode perangkat bersama di gudang dan kantor: karyawan memilih nama, memasukkan PIN pribadi, dan selfie tetap wajib | Semua karyawan bisa absen tanpa membeli mesin absensi |
| 4 | Karyawan lapangan | Status *lapangan* per karyawan dari HRGA. Karyawan lapangan boleh absen di luar radius dengan penanda dan lokasi tercatat. Jika check-in kunjungan (F3-204) sudah aktif, absen pertama dan check-in pertama di lokasi yang sama bisa dilakukan sekaligus | Sesuai BRD 5.16; menghindari input ganda bagi Sales dan Merchandiser |
| 5 | Saldo cuti tahunan | 12 hari per tahun setelah 12 bulan bekerja, sesuai ketentuan ketenagakerjaan; cuti bersama memotong saldo; sisa cuti bisa dibawa ke tahun berikutnya maksimal 6 hari dan hangus akhir Maret | Ketentuan minimal undang-undang dengan aturan sisa cuti yang sederhana dan bisa diubah HRGA |
| 6 | Metode PPh 21 | Metode gross sebagai default; tunjangan pajak (gross-up) bisa diaktifkan per karyawan | Praktik paling umum; tetap mendukung karyawan dengan perjanjian gross-up |
| 7 | Data penghasilan tahun berjalan | Payroll resmi bisa dimulai di bulan apa pun. Penghasilan bruto, iuran, dan PPh 21 yang sudah dipotong sejak Januari diimpor per karyawan agar perhitungan akhir tahun benar | Go-live payroll tidak perlu menunggu Januari |
| 8 | Akses Finance & Accounting ke payroll | F&A melihat jurnal dan rekap per divisi, bukan gaji per orang; file transfer berisi nama, rekening, dan nominal hanya untuk pelaksana pembayaran | Memenuhi NFR-04 (data gaji hanya HRGA/Payroll dan Direksi) sambil tetap memungkinkan pembayaran dan pencatatan |
| 9 | Format file transfer gaji | Profil format bank yang dikonfigurasi (seperti profil impor mutasi bank Fase 1), dengan profil awal untuk bank payroll Balans | Format bank berbeda-beda; profil menghindari perubahan kode |
| 10 | Batas kasbon | Kasbon maksimal 1× gaji pokok, dicicil maksimal 6 bulan, dan cicilan per bulan maksimal 30% gaji pokok. Nilai bisa diubah | Melindungi karyawan dan perusahaan dari cicilan yang memberatkan |

### 15.2 Isu Terbuka

Tidak ada isu desain yang terbuka. Data yang dibutuhkan dari Balans tercantum di entry criteria (bagian 12.1).

---

## 16. Keterlacakan

### 16.1 Sumber Kebutuhan ke Fitur

| Sumber | Fitur PRD |
|---|---|
| Divisi 4 HRGA | F5-101, F5-102 |
| Arahan Balans P-22 / U-22 (HRIS lengkap termasuk payroll) | F5-101 s.d. F5-209 |
| P-23 / U-23 (50+ karyawan, kapasitas 100) | NFR-10 Fase 5 |
| #14 sampel karyawan, #15 harga karyawan | Aturan #2 (status karyawan sebagai sumber tunggal) |
| U-13 reimbursement lewat self-service | F5-107 |
| D-14 paralel run payroll | F5-208 |

### 16.2 Requirement BRD ke Rilis

| Modul | R5.1 | R5.2 |
|---|---|---|
| HR | HR-04, 05, 06, 07, 12 | HR-08, 09, 10, 11; HR-12 (slip gaji) |
| ACC | | ACC-02 (A-32), A-29 untuk PPh 21 dan BPJS |
| RPT | | RPT-17 |
| Opsional | | SLS-11 potong gaji, A-33 (U-31) |

### 16.3 Dokumen Pendukung

| Topik | Dokumen |
|---|---|
| Lingkup, urutan, opsi paralel | Roadmap v0.6 bagian 4 dan 10, D-14 |
| Proses HRIS & payroll | BRD v0.7 bagian 5.16 |
| Karyawan dasar, engine approval, pengajuan biaya | PRD Fase 1 (F1-106, F1-119, F1-403) |
| COA dan posting rule A-29, A-32 | Desain Finance & Accounting Fase 1 |
| Role, permission, matriks approval | Desain Role & Approval Fase 1 |
| User story dan estimasi | Backlog Fase 5 |

---

## 17. Persetujuan

Dengan menandatangani bagian ini, pihak berikut menyetujui isi, kebutuhan, ukuran keberhasilan, dan syarat go-live Fase 5 sebagaimana dijabarkan di dokumen ini.

| Nama | Jabatan | Tanda Tangan | Tanggal |
|---|---|---|---|
| | Balans Nutrition — Direksi | | |
| | Balans Nutrition — PIC Utama Proyek | | |
| | Balans Nutrition — Head HRGA | | |
| | Balans Nutrition — Finance & Accounting | | |
| | Tim Implementasi — Project Manager | | |
| | Tim Implementasi — Tech Lead | | |
