# Desain Finance & Accounting Fase 1
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | COA final (F0-FIN-01), mapping jurnal otomatis (F0-FIN-02), dimensi (F0-FIN-03), profil pajak (F0-FIN-04) |
| Acuan | BRD v0.7 bagian 5.14, 5.15, 7, U-04, U-06, U-14; Technical Guideline 5.8, 5.9 |
| Status | Draft. COA dan mapping disetujui Head Finance & Accounting; profil pajak dikonfirmasi konsultan pajak Balans |

Jika Balans sudah memiliki COA, kolom *Akun Balans saat ini* di bagian 1 diisi untuk memetakan saldo awal (A-36). Struktur di bawah tetap dipakai sistem karena penjualan tidak dipecah per channel di COA; channel dicatat sebagai dimensi (BRD 7.1).

---

## 1. Chart of Accounts

Format kode: 4 digit, digit pertama = kelompok. Akun induk (judul) tidak bisa diposting. Kolom **Manual**: boleh dipakai di Jurnal Umum manual; akun kontrol (piutang, utang, persediaan, Barang Diterima Belum Ditagih) hanya diisi jurnal otomatis agar sub-ledger tetap cocok dengan buku besar.

| Kode | Nama akun | Kelompok | Saldo normal | Manual | Akun Balans saat ini |
|---|---|---|---|---|---|
| **1000** | **ASET** | Aset | Debit | — | |
| 1100 | Kas & Bank | Aset | Debit | — | |
| 1101 | Kas Besar | Aset | Debit | Ya | |
| 1102 | Kas Kecil | Aset | Debit | Tidak | |
| 1111 | Bank — rekening operasional 1 | Aset | Debit | Ya | |
| 1112 | Bank — rekening operasional 2 | Aset | Debit | Ya | |
| 1200 | Piutang | Aset | Debit | — | |
| 1201 | Piutang Usaha | Aset | Debit | Tidak | |
| 1202 | Piutang Midtrans | Aset | Debit | Tidak | |
| 1211 | Piutang Marketplace — Shopee | Aset | Debit | Tidak | |
| 1212 | Piutang Marketplace — TikTok Shop & Tokopedia | Aset | Debit | Tidak | |
| 1213 | Piutang Marketplace — Lazada | Aset | Debit | Tidak | |
| 1214 | Piutang Marketplace — Blibli | Aset | Debit | Tidak | |
| 1221 | Piutang Karyawan | Aset | Debit | Ya | |
| 1300 | Persediaan | Aset | Debit | — | |
| 1301 | Persediaan Packaging | Aset | Debit | Tidak | |
| 1302 | Persediaan Bahan Reprocess | Aset | Debit | Tidak | |
| 1311 | Persediaan FG — Gudang | Aset | Debit | Tidak | |
| 1312 | Persediaan FG — Konsinyasi | Aset | Debit | Tidak | |
| 1313 | Persediaan FG — Vending | Aset | Debit | Tidak | |
| 1314 | Persediaan Retur | Aset | Debit | Tidak | |
| 1400 | Uang Muka & Dibayar di Muka | Aset | Debit | — | |
| 1401 | Uang Muka Pembelian | Aset | Debit | Tidak | |
| 1402 | Deposit Biteship | Aset | Debit | Tidak | |
| 1403 | Biaya Dibayar di Muka | Aset | Debit | Ya | |
| 1500 | Aset Tetap | Aset | Debit | — | |
| 1501 | Aset Tetap — Mesin Vending | Aset | Debit | Tidak | |
| 1502 | Aset Tetap — Peralatan Kantor & Komputer | Aset | Debit | Tidak | |
| 1503 | Aset Tetap — Kendaraan | Aset | Debit | Tidak | |
| 1591 | Akumulasi Penyusutan — Mesin Vending | Aset (kontra) | Kredit | Tidak | |
| 1592 | Akumulasi Penyusutan — Peralatan Kantor & Komputer | Aset (kontra) | Kredit | Tidak | |
| 1593 | Akumulasi Penyusutan — Kendaraan | Aset (kontra) | Kredit | Tidak | |
| **2000** | **LIABILITAS** | Liabilitas | Kredit | — | |
| 2101 | Utang Usaha | Liabilitas | Kredit | Tidak | |
| 2102 | Barang Diterima Belum Ditagih | Liabilitas | Kredit | Tidak | |
| 2103 | Utang Biaya | Liabilitas | Kredit | Tidak | |
| 2201 | Utang Gaji | Liabilitas | Kredit | Tidak | |
| 2202 | Utang BPJS | Liabilitas | Kredit | Tidak | |
| 2301 | Utang PPh 21 | Liabilitas | Kredit | Tidak | |
| 2302 | Utang PPh 23 | Liabilitas | Kredit | Tidak | |
| 2303 | Utang PPh 4(2) | Liabilitas | Kredit | Tidak | |
| 2304 | Utang PPh Final | Liabilitas | Kredit | Tidak | |
| 2401 | Deposit Reseller | Liabilitas | Kredit | Tidak | |
| **3000** | **EKUITAS** | Ekuitas | Kredit | — | |
| 3101 | Modal | Ekuitas | Kredit | Ya | |
| 3201 | Laba Ditahan | Ekuitas | Kredit | Ya | |
| 3202 | Laba Tahun Berjalan | Ekuitas | Kredit | Tidak (dihitung) | |
| 3901 | Saldo Awal Ekuitas | Ekuitas | Kredit | Tidak (migrasi) | |
| **4000** | **PENDAPATAN** | Pendapatan | Kredit | — | |
| 4101 | Penjualan | Pendapatan | Kredit | Tidak | |
| 4102 | Potongan Penjualan | Pendapatan (kontra) | Debit | Tidak | |
| 4103 | Retur Penjualan | Pendapatan (kontra) | Debit | Tidak | |
| 4201 | Pendapatan Ongkos Kirim | Pendapatan | Kredit | Tidak | |
| **5000** | **HARGA POKOK PENJUALAN** | HPP | Debit | — | |
| 5101 | HPP Barang Jadi | HPP | Debit | Tidak | |
| 5102 | HPP — Reject & Waste | HPP | Debit | Tidak | |
| 5103 | Selisih Harga Pembelian | HPP | Debit | Tidak | |
| **6000** | **BEBAN OPERASIONAL** | Beban | Debit | — | |
| 6101 | Beban Marketing — Endorsement | Beban | Debit | Ya | |
| 6102 | Beban Marketing — Event | Beban | Debit | Ya | |
| 6103 | Beban Marketing — Sampel & Promosi | Beban | Debit | Ya | |
| 6104 | Beban Marketing — Iklan Digital | Beban | Debit | Ya | |
| 6105 | Beban Fee Marketplace | Beban | Debit | Tidak | |
| 6106 | Beban Fee Payment Gateway | Beban | Debit | Tidak | |
| 6107 | Beban Promosi Marketplace | Beban | Debit | Tidak | |
| 6108 | Beban Pengiriman Penjualan | Beban | Debit | Ya | |
| 6109 | Beban Biaya Lokasi Vending | Beban | Debit | Ya | |
| 6201 | Beban Gaji & Tunjangan | Beban | Debit | Tidak | |
| 6202 | Beban BPJS | Beban | Debit | Tidak | |
| 6203 | Beban Karyawan — Sampel Produk | Beban | Debit | Tidak | |
| 6301 | Beban Sewa | Beban | Debit | Ya | |
| 6302 | Beban Utilitas | Beban | Debit | Ya | |
| 6303 | Beban Perlengkapan Kantor | Beban | Debit | Ya | |
| 6304 | Beban Langganan Software | Beban | Debit | Ya | |
| 6305 | Beban Penyusutan | Beban | Debit | Tidak | |
| 6306 | Beban Umum Lainnya | Beban | Debit | Ya | |
| 6401 | Beban Selisih Persediaan | Beban | Debit | Tidak | |
| 6402 | Beban Persediaan Kedaluwarsa & Rusak | Beban | Debit | Tidak | |
| 6403 | Beban Pemusnahan Persediaan | Beban | Debit | Tidak | |
| 6404 | Beban Kehilangan Persediaan | Beban | Debit | Tidak | |
| **7000** | **PENDAPATAN & BEBAN LAIN-LAIN** | Lain-lain | — | — | |
| 7101 | Pendapatan Lain-lain | Lain-lain | Kredit | Ya | |
| 7201 | Beban Lain-lain | Lain-lain | Debit | Ya | |
| 7202 | Beban Bank | Lain-lain | Debit | Ya | |
| 7203 | Beban PPh Final | Lain-lain | Debit | Tidak | |
| 7301 | Selisih Pembulatan | Lain-lain | Debit | Tidak | |

Akun 7301 ditambahkan agar selisih pembulatan tidak hilang (Technical Guideline 5.8.6). Rekening bank tambahan dibuat sebagai 1113, 1114, dst. Akun `Laba Tahun Berjalan` dihitung otomatis dari kelompok 4–7 dan ditutup ke Laba Ditahan saat tutup buku akhir tahun.

---

## 2. Mapping Jurnal Otomatis

Dimensi: **C** = channel, **S** = sub-channel, **D** = divisi, **K** = kode kampanye. Dimensi wajib diisi oleh dokumen sumber; `Poster.Post` menolak jurnal dengan dimensi wajib kosong (Technical Guideline 5.8.1). Akun persediaan FG dipilih dari tipe lokasi (gudang → 1311, konsinyasi → 1312, buffer/mesin vending → 1313, retur → 1314).

### 2.1 Berlaku di Fase 1

| Rule | Transaksi | Debit | Kredit | Dimensi wajib | Rilis |
|---|---|---|---|---|---|
| A-01 | Penerimaan barang/packaging dari supplier | 1301 / 1311 | 2102 | D | R1.2 |
| A-02 | Retur pembelian (nota debit) | 2101 (atau 2102 bila belum ditagih) | 1301 / 1311 | D | R1.2 |
| A-03 | Penerimaan FG dari maklon | 1311 | 1301 (packaging terpakai, baik + reject) atau 1302 (reprocess); 2102 (jasa maklon) | D | R1.2 |
| A-04 | Reject/waste setelah penerimaan | 5102 | 1301 / 1311 | D | R1.2 |
| A-05 | Penjualan MT, WA/IG transfer, B2B | 1201 / 1111 (Kas/Bank); 4102 (diskon) | 4101 (harga sebelum diskon) | C, S | R1.3 |
| A-06 | HPP setiap penjualan | 5101 | 1311 / 1312 / 1313 sesuai lokasi asal | C, S | R1.3 |
| A-07 | Kirim barang konsinyasi | 1312 | 1311 | C, S | R1.3 |
| A-08 | Rekonsiliasi konsinyasi (barang terjual) | 1201; 5101 | 4101; 1312 | C, S | R1.3 |
| A-10 | Penjualan marketplace (serah ke kurir) | 121x; 4102 (voucher penjual); 5101 | 4101; 1311 | C, S | R1.3 |
| A-10s | Settlement marketplace (dana cair) | 1111; 6105; 6107 | 121x | C, S | R1.4 |
| A-11 | Pemakaian FG untuk marketing | 6103 | 1311 | D, K | R1.2 |
| A-12 | Sampel produk karyawan | 6203 | 1311 | D | R1.2 |
| A-13 | Retur penjualan | 4103; 1314 | 1201 / 1101 / 1111 / 121x; 5101 | C, S | R1.3 |
| A-14 | Selisih stock count (kurang; dibalik bila lebih) | 6401 | 1301 / 1311 / 1314 | D | R1.2 |
| A-15 | Biaya operasional | Akun jenis biaya (61xx–63xx) | 1111 / 2103; 2301 / 2302 / 2303 bila dipotong | D, K (opsional) | R1.4 |
| A-16 | Pengisian kas kecil | 1102 | 1111 | D | R1.4 |
| A-17 | Pengeluaran kas kecil | Akun jenis biaya | 1102 | D | R1.4 |
| A-18 | Tagihan supplier/maklon (setelah 3-way match) | 2102 (+ 5103 bila selisih harga) | 2101; 2302 bila PPh 23 dipotong | D | R1.2 |
| A-19 | Uang muka pembelian | 1401 | 1111; saat tagihan: Dr 2101 / Cr 1401 | D | R1.2 |
| A-20 | Transfer packaging ke maklon | — | — | — | Tanpa jurnal |
| A-23 | Selisih konsinyasi | Ditagih: seperti A-08. Ditanggung Balans: 6404 | Ditanggung Balans: 1312 | C, S | R1.3 |
| A-24 | Kirim stok ke buffer vending | 1313 | 1311 | C, S | R1.3 (pengiriman); dipakai penuh R2.3 |
| A-26 | Disposisi retur | Restock/repack: 1311. Reprocess: 1302. Musnah: 6403. Sampel: 6103 | 1314 (+ 1301 bila repacking memakai kemasan baru) | D | R1.2 |
| A-27 | Kedaluwarsa/rusak di gudang | 6402 | 1311 | D | R1.2 |
| A-29 | Penyetoran pajak | 2301 / 2302 / 2303 / 2304 | 1111 | — | R1.4 |
| A-30 | PPh Final UMKM (bila berlaku) | 7203 | 2304 | — | R1.4 |
| A-31 | Aset tetap: pembelian / penyusutan | 150x / 6305 | 1111 atau 2101 / 159x | D | R1.4 |
| A-36 | Saldo awal migrasi | Akun neraca bersaldo debit | Akun neraca bersaldo kredit; selisih ke 3901 | — | Cut-over |
| A-PB | Pembayaran pelanggan / pemasok (alokasi) | 1111 / 2101 | 1201 / 1111 | — | R1.3–R1.4 |
| A-RD | Selisih pembulatan dokumen | 7301 | 7301 | — | Semua |

A-10s, A-PB, dan A-RD adalah rule pendukung yang tidak bernomor di BRD 7.2 (dana cair marketplace, pelunasan, pembulatan). Kodenya dipakai di `posting_rules` agar setiap jurnal otomatis tetap punya rule.

### 2.2 Fase Berikutnya dan Opsional

| Rule | Transaksi | Fase |
|---|---|---|
| A-09 | Penjualan vending via Midtrans | Fase 2 (R2.3) |
| A-21 | Penjualan website & link bayar Midtrans (+ settlement) | Fase 2 (R2.2) |
| A-22 | Biaya kirim via Biteship (top-up deposit & per kiriman) | Fase 2 (R2.2) |
| A-32 | Payroll | Fase 5 (R5.2) |
| A-34 | Deposit reseller | Fase 2 (R2.1) |
| A-35 | Penukaran poin loyalitas | Fase 3 (R3.1) |
| A-25 | Biaya penempatan mesin vending | Opsional U-32 (R2.3 jika diambil) |
| A-28 | Kitting bundel fisik | Opsional U-29 (R1.2 jika diambil) |
| A-33 | Pembelian karyawan potong gaji | Opsional U-31 (R5.2 jika diambil) |

Mapping rule ke akun disimpan di `posting_rule_accounts` dan dapat disesuaikan Head Finance & Accounting tanpa mengubah kode. Setiap rilis yang menghasilkan jurnal divalidasi Finance & Accounting dengan test balance per rule (Technical Guideline 12).

---

## 3. Dimensi

### 3.1 Channel

| Kode | Nama | Contoh sub-channel |
|---|---|---|
| `modern_trade` | Modern Trade (beli putus) | Per partner MT |
| `consignment` | Konsinyasi | Per partner konsinyasi |
| `marketplace` | Marketplace | `shopee`, `tiktok_tokopedia`, `lazada`, `blibli` |
| `social_commerce` | WhatsApp / Instagram | `whatsapp`, `instagram` |
| `website` | Website | `findyourbalans` (mulai R2.2) |
| `reseller` | Reseller | Per level (mulai R2.1) |
| `vending` | Vending | Per mesin (mulai R2.3) |
| `b2b` | B2B / Corporate | Per customer korporat |
| `employee` | Penjualan Karyawan | — |
| `internal` | Internal (non-penjualan) | Untuk jurnal biaya dan persediaan |

### 3.2 Divisi

| Kode | Nama |
|---|---|
| `LOG` | Logistik |
| `FAT` | Finance & Accounting |
| `SLS` | Sales |
| `MKT` | Marketing |
| `HRGA` | HRGA |
| `MGT` | Manajemen / Direksi |

### 3.3 Kode Kampanye

- Format: `KMP-YYYYMM-<singkatan>`, contoh `KMP-202701-RAMADAN`. Dibuat Marketing di master kampanye sebelum dipakai.
- **Wajib** di pemakaian internal marketing (A-11) dan pengajuan biaya endorsement/event/iklan. **Opsional** di penjualan (untuk analisis promo).
- Satu kampanye dapat dilihat lintas biaya produk dan biaya fee di laporan laba rugi per dimensi (RPT-15).

### 3.4 Aturan Pemakaian

- Jurnal penjualan dan HPP wajib C + S.
- Jurnal biaya wajib D; K wajib untuk jenis biaya bertanda kampanye.
- Biaya bersama (gaji, sewa) tidak dialokasikan otomatis ke channel di tahap awal (U-14); dicatat dengan channel `internal`.

---

## 4. Profil Pajak

Diisi Balans dan dikonfirmasi konsultan pajak Balans (F0-FIN-04). Tarif disimpan sebagai data effective-dated, sehingga perubahan regulasi tidak mengubah kode (Technical Guideline 5.9, 5.14).

### 4.1 Identitas Pajak

| Data | Isian Balans | Dampak di sistem |
|---|---|---|
| Bentuk badan usaha (PT / CV / perorangan) | | Batas tahun PPh Final UMKM |
| Tahun terdaftar NPWP | | Sama |
| Status PKP | NON-PKP (BRD asumsi #1) | Invoice tanpa PPN |
| Memakai PPh Final UMKM saat ini (Ya/Tidak) | | ACC-20 aktif/nonaktif |
| Batas tahun pemakaian PPh Final menurut konsultan | | Peringatan berakhirnya fasilitas |
| Omzet bruto kumulatif tahun berjalan per tanggal cut-over | | Pemantauan batas Rp4,8 miliar (RPT-18) |

### 4.2 Objek Pemotongan Awal

| Objek | Jenis PPh | Usulan awal | Dipakai di | Konfirmasi konsultan |
|---|---|---|---|---|
| Jasa maklon dari badan usaha | PPh 23 | Objek PPh 23 jasa | Tagihan pemasok maklon | |
| Jasa lain dari badan usaha (desain, agensi endorsement, event organizer) | PPh 23 | Objek PPh 23 jasa | Pengajuan biaya | |
| Fee endorser / talent perorangan (bukan pegawai) | PPh 21 bukan pegawai | Dihitung dari penghasilan bruto sesuai ketentuan bukan pegawai | Pengajuan biaya | |
| Sewa bangunan / ruang (termasuk lokasi gudang) | PPh 4(2) | Sewa tanah/bangunan | Pengajuan biaya | |
| Omzet bruto bulanan | PPh Final UMKM | Jika berlaku bagi Balans | Tutup buku (A-30) | |
| Penghasilan pegawai | PPh 21 TER | Mulai Fase 5 (payroll) | — | |

Tarif persen tidak ditulis di dokumen ini; nilainya diisi di `withholding_tax_rates` sesuai konfirmasi konsultan, termasuk ketentuan tarif lebih tinggi bagi penerima tanpa NPWP/NIK.

### 4.3 Jenis Biaya Awal

| Kode | Jenis biaya | Akun beban | PPh | Kampanye wajib | Approver mengikuti |
|---|---|---|---|---|---|
| `ENDORSE_PERSON` | Fee endorsement perorangan | 6101 | PPh 21 bukan pegawai | Ya | Pengajuan biaya |
| `ENDORSE_AGENCY` | Fee endorsement agensi | 6101 | PPh 23 | Ya | Pengajuan biaya |
| `EVENT` | Biaya event | 6102 | PPh 23 bila vendor badan usaha jasa | Ya | Pengajuan biaya |
| `ADS` | Iklan digital | 6104 | Tidak | Ya | Pengajuan biaya |
| `RENT` | Sewa bangunan | 6301 | PPh 4(2) | Tidak | Pengajuan biaya |
| `UTILITY` | Utilitas | 6302 | Tidak | Tidak | Pengajuan biaya |
| `OFFICE_SUPPLY` | Perlengkapan kantor | 6303 | Tidak | Tidak | Pengajuan biaya / kas kecil |
| `SOFTWARE` | Langganan software | 6304 | Tidak | Tidak | Pengajuan biaya |
| `SHIPPING` | Ongkos kirim penjualan manual | 6108 | Tidak | Tidak | Pengajuan biaya / kas kecil |
| `BANK_FEE` | Biaya bank | 7202 | Tidak | Tidak | Finance & Accounting |
| `REIMBURSE` | Reimbursement karyawan | Sesuai jenis | Tidak | Opsional | Pengajuan biaya |
| `OTHER` | Beban umum lainnya | 6306 | Ditentukan saat verifikasi | Tidak | Pengajuan biaya |

---

## 5. Review Perlakuan Akuntansi (F0-FIN-05)

Bahan untuk akuntan/konsultan pajak Balans:

| # | Pertanyaan review | BRD |
|---|---|---|
| 1 | Pemakaian akun Barang Diterima Belum Ditagih untuk penerimaan sebelum tagihan | 5.2, A-01, A-18 |
| 2 | Reject maklon diserap ke HPP per unit (nilai batch ÷ qty baik) | 5.3, A-03, A-04 |
| 3 | Titik pengakuan penjualan per channel (serah kurir, rekonsiliasi konsinyasi, transaksi vending, invoice MT) | U-25 |
| 4 | Selisih konsinyasi yang ditanggung Balans sebagai beban kehilangan persediaan | A-23 |
| 5 | Pemakaian internal marketing dan sampel karyawan sebesar HPP | A-11, A-12 |
| 6 | Penukaran poin dicatat sebagai potongan penjualan saat ditukar, tanpa pencadangan | A-35 |
| 7 | Penyusutan garis lurus dan umur manfaat per kelas aset | A-31, U-11 |
| 8 | Objek PPh 23 jasa maklon dan PPh 4(2) sewa | U-06 |

Catatan review dan koreksinya dimasukkan ke BRD v1.0.
