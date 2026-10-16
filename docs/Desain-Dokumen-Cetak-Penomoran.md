# Desain Penomoran & Dokumen Cetak
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Format nomor dokumen dan layout cetak (F0-DSN-05) |
| Acuan | BRD v0.7 U-15, NFR-08, 5.4, 5.5; Technical Guideline 5.12, 3.1 (Gotenberg) |
| Status | Draft. Format nomor dan layout disetujui Finance & Accounting dan Sales Balans |

---

## 1. Format Nomor

Format `KODE/TAHUN/BULAN/NOMOR` dengan nomor urut 5 digit yang **reset setiap tahun** (U-15). Tahun dan bulan diambil dari tanggal bisnis dokumen. Nomor diberikan saat dokumen diajukan atau diposting, sehingga draft yang dibatalkan tidak membuat nomor loncat.

| Kode | Dokumen | Diberikan saat | Contoh |
|---|---|---|---|
| PR | Purchase Request | Submit | `PR/2027/01/00003` |
| PO | Purchase Order (termasuk maklon & reprocess) | Submit | `PO/2027/01/00012` |
| GR | Penerimaan Barang | Posting | `GR/2027/01/00010` |
| DO | Delivery Order | Release | `DO/2027/01/00045` |
| PI | Proforma Invoice | Terbit | `PI/2027/01/00008` |
| INV | Invoice | Posting | `INV/2027/01/00031` |
| CN | Nota Kredit | Posting | `CN/2027/01/00002` |
| DN | Nota Debit | Posting retur pembelian | `DN/2027/01/00001` |
| RJ | Retur Penjualan | Submit | `RJ/2027/01/00004` |
| RB | Retur Pembelian | Submit/posting | `RB/2027/01/00002` |
| TRF | Transfer Stok | Kirim / posting | `TRF/2027/01/00020` |
| SC | Stock Count | Mulai hitung | `SC/2027/01/00006` |
| KSN | Rekonsiliasi Konsinyasi (Berita Acara) | Mulai rekonsiliasi | `KSN/2027/01/00009` |
| EXP | Pengajuan Biaya | Submit | `EXP/2027/01/00017` |
| KK | Kas Kecil | Posting | `KK/2027/01/00040` |
| JV | Jurnal | Posting | `JV/2027/01/00102` |

Kode tambahan yang dibutuhkan sistem, dengan format yang sama:

| Kode | Dokumen | Alasan |
|---|---|---|
| SO | Sales Order | Dokumen pesanan semua channel (BRD 4), termasuk order website, reseller, dan marketplace; channel dibedakan lewat dimensi, bukan kode |
| IN | Inbound | Nomor dokumen gudang untuk QC barang masuk |
| OUT | Outbound | Nomor dokumen picking/packing/QC |
| PU | Pemakaian Internal | Dokumen permintaan barang marketing/HRGA |
| MUS | Pemusnahan (Berita Acara Pemusnahan) | Berita acara diminta auditor/pajak (BRD 5.11) |
| BYR | Pembayaran | Bukti terima/keluar kas |
| DEP | Mutasi Deposit Reseller | Top-up dan pengembalian deposit reseller (mulai R2.1) |
| RFL | Refill Vending | Refill buffer ke mesin oleh partner (mulai R2.3) |
| RKV | Rekonsiliasi Vending (Berita Acara) | Rekonsiliasi bulanan per mesin (mulai R2.3) |

Nomor tagihan pemasok memakai nomor dari pemasok (unik per pemasok), bukan sequence Balans.

---

## 2. Dokumen Cetak (NFR-08)

Semua PDF dirender dari template HTML (`document_templates`, effective-dated) lewat Gotenberg, memuat logo Balans, dan memakai format Indonesia (`Rp1.250.000`, `14 Okt 2026`).

### 2.1 Elemen Umum

| Bagian | Isi |
|---|---|
| Kop | Logo Balans, nama badan usaha, alamat, telepon, email, NPWP (jika ada) |
| Judul | Nama dokumen (label UI) dan nomor |
| Identitas | Tanggal, mitra (pelanggan/pemasok/partner) beserta alamat, referensi (PO klien, SO, PKS) |
| Isi | Tabel baris: SKU, nama produk, batch (jika relevan), qty, satuan, harga, diskon, jumlah |
| Ringkasan | Subtotal, diskon, ongkos kirim, total; terbilang Bahasa Indonesia |
| Kaki | Info rekening pembayaran, termin, keterangan, tanda tangan, nomor halaman, kode verifikasi (QR ke dokumen di ERP) |

### 2.2 Per Dokumen

| Dokumen | Isi khusus | Keterangan NON-PKP | Tanda tangan |
|---|---|---|---|
| Purchase Order | Pemasok, tanggal kirim diharapkan, syarat pembayaran, uang muka; untuk PO maklon: daftar packaging yang dikirim | — | Dibuat oleh, disetujui (nama approver terakhir) |
| Delivery Order | Alamat kirim, kurir/resi atau nama Merchandiser (kurir internal), jumlah koli, batch & tanggal kedaluwarsa per baris, kode produk mitra bila ada, tanpa harga | — | Disiapkan (Staf Gudang), dicek (Fulfillment), pengirim, penerima (nama, tanggal, tanda tangan) |
| Proforma Invoice | Masa berlaku (default 14 hari), instruksi pembayaran, rekening | "Dokumen ini bukan bukti pembayaran" dan "Balans bukan Pengusaha Kena Pajak; tidak dipungut PPN" | Finance & Accounting |
| Invoice | Tanggal jatuh tempo, status lunas, rekening | "Balans bukan Pengusaha Kena Pajak; tidak dipungut PPN" | Finance & Accounting |
| Nota Kredit | Invoice asal, alasan, nilai pengurang piutang | Sama dengan invoice | Finance & Accounting |
| Nota Debit | Tagihan/PO asal, barang dikembalikan, nilai pengurang utang | — | Logistik, Finance & Accounting |
| Berita Acara Rekonsiliasi Konsinyasi | Periode, PKS, tabel per SKU (stok awal, kiriman, tarikan, sisa fisik, terjual sistem, laporan partner, selisih, penyebab, ditanggung oleh), nilai tagihan | — | Partner dan Sales MT (dua pihak), tanggal |
| Berita Acara Pemusnahan | Tanggal, metode, lokasi, saksi, daftar barang & batch, nilai HPP, lampiran foto | — | Pelaksana, saksi, Head Logistik, Finance & Accounting |

### 2.3 Label

Label dicetak ke printer label thermal (ukuran diatur di template, awal 50 × 30 mm untuk label produk dan 100 × 150 mm untuk label koli) atau ke kertas A4 berisi banyak label. Barcode dirender server sebagai SVG (EAN-13 bila barcode berformat EAN, selain itu Code 128).

| Label | Dicetak dari | Isi | Jumlah default |
|---|---|---|---|
| Label Produk | Master produk, Penerimaan Barang, atau DO/Outbound | Nama produk & varian, SKU, barcode utama, batch, tanggal kedaluwarsa | Qty baris (dapat diubah) |
| Label Produk Mitra | DO/Outbound untuk partner yang punya kode produk mitra | Seperti Label Produk, ditambah kode artikel dan barcode milik retailer | Qty baris |
| Label Koli | Outbound (`outbound_packages`) | Logo, nomor DO, koli ke-n dari total, nama & alamat penerima, partner, ringkasan isi, berat | Satu per koli |

Jika partner tidak punya kode produk mitra untuk suatu SKU, sistem mencetak Label Produk biasa dan memberi peringatan.

Template final disetujui setelah logo, info rekening, dan teks keterangan NON-PKP dikonfirmasi Balans. Slip gaji dirancang di Fase 5.

---

## 3. Data yang Dibutuhkan dari Balans

| Data | Untuk |
|---|---|
| Logo resolusi tinggi (SVG/PNG) | Kop semua dokumen |
| Nama badan usaha resmi, alamat, telepon, email, NPWP | Kop |
| Rekening bank untuk pembayaran pelanggan | Proforma, invoice |
| Teks keterangan NON-PKP yang disetujui konsultan pajak | Proforma, invoice, nota kredit |
| Penanda tangan default per dokumen | Kaki dokumen |
| Tipe & ukuran printer label yang dipakai gudang | Template label produk dan label koli |
| Kode artikel/barcode dari tiap retailer MT yang mewajibkan label sendiri | Kode produk mitra (MD-07) |
