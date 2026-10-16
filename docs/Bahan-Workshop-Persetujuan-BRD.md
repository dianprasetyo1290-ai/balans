# Bahan Workshop Persetujuan BRD
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Agenda workshop per divisi, format notulen, alur menuju BRD v1.0 (F0-APR-01 s.d. F0-APR-05) |
| Lembar keputusan | `docs/Lembar-Persetujuan-Baseline-BRD.xlsx` (Usulan, Konfirmasi, Opsional, Parameter, Data Balans, Ringkasan) |
| Acuan | BRD v0.7 bagian 1.3, 12, 15; PRD Fase 0 bagian 5, 6.1, 9; Roadmap v0.6 bagian 11, 16 |

Prinsip workshop: usulan sudah lengkap, sehingga key user **menyetujui atau mengoreksi**, bukan merancang dari nol (PRD risiko #1). Catatan kebutuhan Balans (11 September 2026) tetap dasar utama; jika usulan bertentangan dengan catatan, catatan yang berlaku.

---

## 1. Urutan Kegiatan

| # | Kegiatan | Peserta | Output |
|---|---|---|---|
| 1 | Kickoff: tujuan Fase 0, penetapan key user per divisi dan PIC utama, jadwal workshop | Direksi, PIC utama, PM | Daftar key user, jadwal workshop |
| 2 | Workshop Logistik & Finance & Accounting | Key user Logistik & F&A, Head F&A | Status usulan & pernyataan bagian 2.1 |
| 3 | Workshop Sales (MT/konsinyasi, e-commerce/CS, reseller & vending) | Key user Sales, Head Sales | Status bagian 2.2, batas diskon per role |
| 4 | Workshop Marketing | Key user Marketing | Status bagian 2.3, daftar kampanye awal |
| 5 | Workshop HRGA | Key user HRGA | Status bagian 2.4 |
| 6 | Sesi Direksi: pilihan opsional, matriks approval & nama approver, parameter | Direksi, Head F&A, Head Sales | Sheet Opsional & Parameter terisi; nama approver |
| 7 | Review akuntan/konsultan pajak (F0-FIN-05) | Akuntan/konsultan pajak Balans, Head F&A | Catatan review BRD bagian 7 & profil pajak |
| 8 | Penyusunan BRD v1.0 | PM/BA | BRD v1.0 dengan semua koreksi |
| 9 | Penandatanganan BRD v1.0 | Direksi, F&A, Sales, HRGA, tim implementasi | BRD bagian 15 ditandatangani |
| 10 | Steering committee: kalibrasi Fase 1 (F0-APR-05) | Direksi, PM | Komposisi tim, kapasitas sprint, urutan backlog R1.1 – R1.4 disetujui |

---

## 2. Agenda per Divisi

Setiap workshop ±2 jam: 15 menit alur to-be (diagram BRD bagian 5), 90 menit pembahasan usulan per baris di lembar keputusan, 15 menit ringkasan & tindak lanjut.

### 2.1 Logistik & Finance & Accounting

| Topik | Bahan | Butir keputusan |
|---|---|---|
| Struktur gudang & lokasi | BRD 5.1 | U-01, U-08, U-27; K-07 |
| Procurement & maklon | BRD 5.2, 5.3 | U-04, U-07, U-24, U-26; K-06, K-08, K-15 |
| Retur & disposisi | BRD 5.11 | U-09 |
| Persediaan & HPP | BRD 6.4 | K-01, K-02, K-03 |
| Biaya operasional & kas kecil | BRD 5.14 | U-13; K-11 |
| Akuntansi, pajak, tutup buku | BRD 5.15, 7; Desain Finance & Accounting | U-06, U-10, U-11, U-14, U-15, U-25; K-04, K-12, K-13, K-14, K-16; COA final |
| Parameter | Lembar *Parameter* | Toleransi 3-way match, kas kecil, target tutup buku, jadwal stock count |

### 2.2 Sales

| Topik | Bahan | Butir keputusan |
|---|---|---|
| MT beli putus & B2B | BRD 5.4 | K-10; parameter masa berlaku Proforma |
| Konsinyasi | BRD 5.5 | U-02; K-05; ketentuan selisih default |
| Vending | BRD 5.6 | U-05, U-12, U-17 |
| Marketplace & WA/IG | BRD 5.7, 5.8 | U-21 |
| Website & reseller | BRD 5.9, 5.10 | U-16, U-18, U-20; K-09 |
| Aturan harga & diskon | BRD 5.18 | U-03; batas diskon manual per role |
| Sales pipeline | BRD 5.19 | U-19 |
| Opsional terkait Sales | Lembar *Opsional* | Masukan untuk U-28, U-30, U-31, U-32 |

### 2.3 Marketing

| Topik | Bahan | Butir keputusan |
|---|---|---|
| Pemakaian FG untuk endorsement/event/sample | BRD 5.12 | Alur permintaan & approval |
| Fee endorsement & kode kampanye | BRD 5.14; Desain Finance & Accounting 3.3 | Format kode kampanye; jenis biaya marketing |
| Konten website | BRD 5.9 | Kebutuhan aset brand (gate R2.2) |

### 2.4 HRGA

| Topik | Bahan | Butir keputusan |
|---|---|---|
| Sample karyawan & diskon karyawan | BRD 5.12, 5.13 | Parameter diskon karyawan; opsi U-31 |
| HRIS lengkap | BRD 5.16 | U-22, U-23 (dikerjakan Fase 5) |
| Data karyawan dasar Fase 1 | Template migrasi sheet Karyawan | PIC pengisian |

---

## 3. Format Notulen Workshop

```
Notulen Workshop BRD — <Divisi>
Tanggal / waktu :
Peserta          : <nama — jabatan>
Fasilitator      :

1. Ringkasan keputusan
   | ID   | Status  | Isi koreksi (jika Koreksi) |
   | U-xx | Setuju  |                            |
   | K-xx | Koreksi | ...                        |

2. Parameter yang dikonfirmasi
   | Parameter | Nilai awal disetujui / diganti dengan |

3. Pertanyaan yang belum bisa diputuskan
   | Pertanyaan | PIC | Tanggal keputusan |

4. Data yang harus diserahkan Balans
   | Data | PIC | Tanggal |

Disetujui oleh key user: <nama, tanda tangan>
```

Hasil notulen langsung dipindahkan ke lembar keputusan. Butir yang belum diputuskan dibawa ke weekly status dan tidak boleh menahan workshop berikutnya.

---

## 4. Menuju BRD v1.0

| Langkah | Detail |
|---|---|
| Konsolidasi | Semua butir *Koreksi* dari lembar keputusan dan catatan review akuntan dimasukkan ke bagian BRD terkait |
| Opsional | Kolom *Diambil?* BRD 12.2 diisi dari sheet Opsional; requirement opsional yang tidak diambil ditandai di luar lingkup |
| Dampak | Jika lebih dari tiga opsi Fase 1 dipilih, backlog dan kapasitas Fase 1 dihitung ulang (Backlog Fase 1, bagian kalibrasi) |
| Gaya dokumen | BRD v1.0 bersih: tanpa riwayat revisi atau penanda perubahan; rujukan sumber (#n, U-xx, ARH) dipertahankan; file v0.7 dihapus setelah v1.0 disetujui |
| Tanda tangan | Bagian 15 BRD: Direksi, Finance & Accounting, Sales, HRGA, tim implementasi |
| Baseline | Setelah ditandatangani, perubahan lingkup diproses lewat change request |
