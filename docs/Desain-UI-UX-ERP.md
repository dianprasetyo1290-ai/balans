# Desain UI/UX ERP
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Design system (F0-UX-01), struktur menu & label (F0-UX-02), wireframe layar utama Fase 1 (F0-UX-03), pola form HP (F0-UX-04), rencana uji label (F0-UX-05) |
| Acuan | Naming Convention v1.0; Technical Guideline 10; BRD v0.7 bagian 5 |
| Status | Draft. Wireframe direview key user divisi terkait sebelum build |

Design system sudah terpasang sebagai kode di `web-erp` (preset shadcn Balans, komponen bersama, halaman Tentang Sistem). Dokumen ini adalah spesifikasinya.

---

## 1. Design System

### 1.1 Fondasi

| Elemen | Nilai | Sumber |
|---|---|---|
| Preset | shadcn `bR0x3d3` → style `base-vega`, primitive Base UI, warna dasar `neutral`, menu `inverted`, aksen `bold` | `web-erp/components.json` |
| Font | Inter Variable untuk teks dan judul | `@fontsource-variable/inter` |
| Radius | `0.625rem` dasar; turunan `sm`–`4xl` dihitung dari dasar | `--radius` |
| Ikon | lucide-react, ukuran 16 px di tombol/menu | |
| Mode | Terang dan gelap (`ThemeProvider`, mengikuti sistem) | |
| Spacing | Skala Tailwind 4 px; konten halaman `p-4` (HP) / `p-6` (desktop); jarak antar kartu `gap-6` | |
| Lebar konten | Form dan detail maks. `max-w-3xl`; list memakai lebar penuh | |

### 1.2 Token Warna

| Token | Terang | Gelap | Pemakaian |
|---|---|---|---|
| `--primary` | `oklch(0.553 0.195 38.4)` (oranye Balans) | `oklch(0.47 0.157 37.3)` | Tombol utama, link aktif, progress |
| `--primary-foreground` | `oklch(0.98 0.016 73.7)` | sama | Teks di atas primary |
| `--background` / `--foreground` | putih / `oklch(0.145 0 0)` | `oklch(0.145 0 0)` / `oklch(0.985 0 0)` | Halaman |
| `--muted-foreground` | `oklch(0.556 0 0)` | `oklch(0.708 0 0)` | Teks sekunder, deskripsi |
| `--destructive` | `oklch(0.577 0.245 27.3)` | `oklch(0.704 0.191 22.2)` | Batalkan, hapus, error |
| `--border` | `oklch(0.922 0 0)` | putih 10% | Garis tabel, kartu |
| `--sidebar-primary` | `oklch(0.646 0.222 41.1)` | `oklch(0.705 0.213 47.6)` | Item menu aktif |

Warna status (`StatusBadge`) memakai nada tetap agar konsisten lintas modul:

| Nada | Status |
|---|---|
| Netral (abu) | Draft, Dibatalkan, Ditutup |
| Info (biru) | Diproses, Dalam Pengiriman |
| Peringatan (amber) | Menunggu Persetujuan, Menunggu Pembayaran, Dibayar Sebagian, Diterima Sebagian, Karantina |
| Berhasil (hijau) | Disetujui, Lunas, Diposting, Selesai, Diterima |
| Bahaya (merah) | Ditolak, Kedaluwarsa, Ditolak QC |

### 1.3 Komponen Bersama

| Komponen | Spesifikasi | File |
|---|---|---|
| `ErrorDialog` | Modal untuk error bisnis/otentikasi/jaringan/server. Judul & teks dari katalog `error-messages.ts`; tombol sesuai jenis (Tutup, Coba lagi, Muat ulang, Masuk kembali); kode laporan (`requestId`) untuk 5xx | `components/common/error-dialog.tsx` |
| `ConfirmDialog` | Konfirmasi aksi berisiko; label tombol = kata kerja + objek (mis. *Batalkan Purchase Order*); varian merah untuk aksi destruktif; tombol konfirmasi menjadi `LoadingButton` | `confirm-dialog.tsx` |
| `LoadingButton` | Langsung disabled saat proses; indikator circular muncul setelah 300 ms, minimal 500 ms; label berganti (*Menyimpan…*) | `loading-button.tsx` |
| `CircularProgress` | Satu-satunya indikator loading; mode indeterminate dan persentase; `role="progressbar"` + `aria-valuenow` | `circular-progress.tsx` |
| `ProgressDialog` | Proses panjang via job: persentase + pesan tahap + tombol *Jalankan di latar belakang* | `progress-dialog.tsx` |
| `DataTable` | Header bisa diurutkan (`aria-sort`), pagination *Sebelumnya/Berikutnya*, jumlah data, indikator refetch kecil di header, tabel bergulir horizontal di HP; state sort/halaman disimpan di URL oleh route | `data-table.tsx` |
| `MoneyInput` | Prefiks `Rp`, rata kanan, pemisah ribuan `1.250.000`, nilai keluar string desimal `"1250000.00"` | `money-input.tsx` |
| `QuantityInput` | Koma desimal, maks. 3 desimal, satuan di kanan | `quantity-input.tsx` |
| `DateInput` | Kalender locale Indonesia; tampilan `14 Okt 2026`; nilai `YYYY-MM-DD` WIB | `date-input.tsx` |
| `StatusBadge` | Label Naming Convention bagian 7 + nada warna di 1.2 | `status-badge.tsx` |
| `FileUpload` | Upload langsung ke object storage (presigned URL), kompresi foto ±1 MB, progress persentase, target sentuh 44 px | `file-upload.tsx` |

### 1.4 Aturan Interaksi

| Situasi | Pola |
|---|---|
| Sukses simpan/aksi | Toast kanan atas, 4 detik |
| Error field | Teks merah di bawah field, fokus ke field error pertama |
| Error bisnis | `ErrorDialog` (mis. *Stok tidak cukup*, *Periode sudah ditutup*) |
| Konflik versi | Modal *Data sudah berubah* + *Muat ulang* |
| Tombol aksi dokumen | Hanya aksi di `allowedActions` yang tampil; urutan: aksi utama (primary) → sekunder (outline) → destruktif (menu ⋯) |
| Kosong | Teks penjelas + tombol aksi utama (mis. *Belum ada Purchase Order. Buat Purchase Order*) |

---

## 2. Struktur Menu & Label (F0-UX-02)

Struktur mengikuti Naming Convention bagian 5 dan sudah dikodekan di `web-erp/src/lib/navigation/menu.ts`. Menu tampil jika modulnya aktif (`ENABLED_MODULES`) **dan** halamannya sudah dibangun; item opsional tampil hanya jika usulannya diambil (D-26).

| Menu | Submenu yang dirilis di Fase 1 | Submenu fase berikutnya (tersembunyi di Fase 1) | Opsional (tersembunyi sampai diambil) |
|---|---|---|---|
| Ringkasan | Ringkasan Operasional, Dashboard Eksekutif (dasar) | — | — |
| Data Master | Produk, Packaging & Material, Bundling, Daftar Harga, Aturan Harga, Pelanggan, Mitra Konsinyasi, Pemasok & Maklon, Gudang & Lokasi, Karyawan, Alasan Retur, Jenis Biaya, Kampanye | Reseller (R2.1) | — |
| Pengadaan | Semua submenu | — | — |
| Gudang | Inbound, Put-away, Transfer Stok, Stok dalam Pengiriman, Outbound, Stock Count, Gudang Retur, Disposisi Retur, Riwayat Mutasi Stok | — | — |
| Persediaan | Stok per Lokasi, Kartu Stok, Batch & Kedaluwarsa, Alokasi Batch (FEFO), HPP per Batch, Pemakaian Internal, Repacking, Pemusnahan | Peringatan Stok Minimum, Peringatan Kedaluwarsa (R2.1) | — |
| Logistik | Delivery Order, Pengiriman, Kurir & Resi, Pelacakan Pengiriman (manual) | Label Pengiriman (R2.2) | — |
| Penjualan | Sales Order, Modern Trade, Konsinyasi, Penjualan Marketplace, Pesanan WhatsApp/Instagram, Penjualan Karyawan, Proforma Invoice, Invoice, Retur Penjualan, Nota Kredit, Rekonsiliasi Konsinyasi, Harga & Diskon | — | — |
| Pipeline Penjualan | — | Semua (R3.2) | — |
| Pelanggan (CRM) | — | Semua (R2.1, R3.2) | — |
| Reseller | — | Semua (R2.1, R2.2, R3.1) | — |
| Mesin Vending | — | Semua (R2.3, R3.3) | Biaya penempatan (U-32) |
| E-commerce | — | Semua (R2.2, R3.1) | — |
| SDM (HRIS) | Data Karyawan (data dasar) | Lainnya (R5.1, R5.2) | — |
| Keuangan & Akuntansi | Ringkasan Keuangan, Chart of Accounts, Jurnal Umum, Buku Besar, Piutang Usaha, Utang Usaha, Kas & Bank, Rekonsiliasi Bank, Kas Kecil, Biaya Operasional, Pajak & Bukti Potong, Aset Tetap, Penyusutan, Tutup Buku, Saldo Awal, Laporan Keuangan | — | Anggaran Biaya (U-13) |
| Laporan | Laporan Stok, Laporan Mutasi Stok, Laporan Stock Count, Laporan HPP, Laporan Kedaluwarsa, Laporan Penjualan, Laporan Konsinyasi, Laporan Retur, Laporan Pemakaian Internal, Laporan Piutang, Laporan Utang, Laporan Laba Rugi per Dimensi, Laporan Pajak | Laporan Pengiriman otomatis, Laporan Pipeline, Laporan HR, Laporan E-commerce | — |
| Integrasi | Import/Export Excel | Midtrans, Biteship (R2.2), Mesin Vending (R2.3), WhatsApp Business (R3.3), Marketplace (Fase 4) | — |
| Pengaturan | Pengguna & Role, Hak Akses, Matriks Persetujuan, Notifikasi, Nomor Dokumen, Template Dokumen, Preferensi Sistem, Tentang Sistem | — | — |

Opsi U-28 (sisa umur simpan), U-29 (kitting), dan U-30 (batas kredit) tidak menambah menu: U-28 dan U-30 berupa field/pengaturan, U-29 menambah submenu *Kitting* di Persediaan jika diambil.

Label status tambahan yang dibutuhkan dan belum ada di Naming Convention bagian 7: **Terbuka** (periode akuntansi). Diusulkan masuk Naming Convention lewat uji label (bagian 5).

---

## 3. Wireframe Layar Utama (F0-UX-03)

Wireframe di bawah adalah wireframe struktur (low-fidelity). Komposisi visual mengikuti design system di bagian 1.

### 3.1 Pola Dasar: Layout dengan Sidebar

```
┌──────────────┬───────────────────────────────────────────────────────────┐
│ [B] Balans ERP│ ☰  Pengadaan › Purchase Order                    [Staging] │
│  Balans Nutr. ├───────────────────────────────────────────────────────────┤
│               │                                                           │
│ RINGKASAN     │   (konten halaman)                                        │
│  Ringkasan Op.│                                                           │
│ PENGADAAN     │                                                           │
│ ▸ Purchase Req│                                                           │
│ ▸ Purchase Ord│                                                           │
│ GUDANG        │                                                           │
│ ...           │                                                           │
│               │                                                           │
│ Versi v1.0.0  │                                                           │
└──────────────┴───────────────────────────────────────────────────────────┘
HP: sidebar menjadi sheet dari kiri lewat tombol ☰; header tetap.
```

### 3.2 Pola List

```
Purchase Order                                        [+ Buat Purchase Order]
┌ Cari nomor / pemasok ┐ [Status ▾] [Pemasok ▾] [Tanggal: 1 Okt – 31 Okt ▾]
┌────────────────┬────────────┬───────────────┬──────────────┬──────────────┐
│ Nomor PO ↑↓    │ Tanggal ↑↓ │ Pemasok       │ Total ↑↓     │ Status       │
├────────────────┼────────────┼───────────────┼──────────────┼──────────────┤
│ PO/2027/01/012 │ 14 Jan 2027│ PT Kemas Jaya │ Rp12.500.000 │ [Menunggu P.]│
│ ...            │            │               │              │              │
└────────────────┴────────────┴───────────────┴──────────────┴──────────────┘
1.240 data · Halaman 1 dari 25                       [Sebelumnya] [Berikutnya]
Filter, sort, halaman tersimpan di URL (?status=pending_approval&page=1&sort=-createdAt).
```

### 3.3 Pola Detail Dokumen

```
PO/2027/01/00012  [Menunggu Persetujuan]                [Setujui] [Tolak] [⋯]
Pemasok: PT Kemas Jaya   Tanggal: 14 Jan 2027   Divisi: Logistik
┌ Baris ─────────────────────────────────────────────────────────────────────┐
│ SKU      Nama            Qty     Harga        Jumlah      Diterima         │
│ PKG-T01  Tube 30 ml   10.000    Rp1.250   Rp12.500.000          0          │
└────────────────────────────────────────────────────────────────────────────┘
┌ Persetujuan ──────────────┐  ┌ Riwayat (audit) ────────────────────────────┐
│ L1 Head F&A   ✓ 14 Jan     │  │ 14 Jan 09.12 Rina mengajukan                │
│ L2 Direksi    ⏳ menunggu   │  │ 14 Jan 10.05 Budi menyetujui (L1)           │
└────────────────────────────┘  └─────────────────────────────────────────────┘
Tombol hanya dari allowedActions. Lampiran & dokumen terkait (GR, tagihan) di tab.
```

### 3.4 Pola Form

```
Buat Purchase Order
Pemasok*        [Cari pemasok…            ▾]   Tanggal*  [14 Jan 2027 📅]
Jenis PO*       (•) Packaging ( ) Jasa Maklon ( ) Reprocess ( ) Lainnya
Divisi*         [Logistik ▾]                    Tgl kirim [21 Jan 2027 📅]
┌ Baris ──────────────────────────────────────────────────────────────┐
│ Produk*            Qty*          Harga*          Jumlah        [🗑]  │
│ [PKG-T01 Tube ▾]   [10.000 pcs]  [Rp 1.250]      Rp12.500.000        │
│ [+ Tambah baris]                                                    │
└─────────────────────────────────────────────────────────────────────┘
                                         Subtotal (preview)  Rp12.500.000
                              [Simpan Draft]  [Ajukan Purchase Order]
Total final dihitung backend; preview memakai big.js. Error field tampil di bawah field.
```

### 3.5 Pola Inbox Persetujuan

```
Persetujuan Saya (3)                         [Semua jenis ▾] [Divisi ▾]
┌──────────────────┬────────────────────┬──────────────┬────────┬──────────────┐
│ Dokumen          │ Diajukan oleh      │ Nilai        │ Level  │ Aksi         │
├──────────────────┼────────────────────┼──────────────┼────────┼──────────────┤
│ PO/2027/01/012   │ Rina · 14 Jan 09.12│ Rp12.500.000 │ 2 dari 2│[Lihat][Setujui]│
│ EXP/2027/01/017  │ Andi · 13 Jan 16.40│ Rp3.200.000  │ 1 dari 1│[Lihat][Setujui]│
└──────────────────┴────────────────────┴──────────────┴────────┴──────────────┘
Tolak wajib catatan. Badge jumlah di menu dan notifikasi in-app + email (NFR-11).
```

### 3.6 Pola Import Excel dengan Preview

```
Impor Data Produk
1 Unduh template  →  2 Unggah file  →  3 Periksa  →  4 Simpan
[Unduh Template Produk.xlsx]
┌ Hasil pemeriksaan ──────────────────────────────────────────────────────┐
│ 1.200 baris · 1.180 valid · 20 error        [Unduh baris error (.xlsx)] │
│ Baris 14  kolom SKU         Kode sudah dipakai                          │
│ Baris 27  kolom Umur simpan Wajib diisi                                 │
└─────────────────────────────────────────────────────────────────────────┘
                                   [Batal]  [Simpan 1.180 baris valid]
Validasi berjalan sebagai job dengan ProgressDialog; file yang sama tidak bisa diimpor dua kali.
```

### 3.7 Layar Kunci Fase 1

| Layar | Isi utama | Pola | Direview key user |
|---|---|---|---|
| Purchase Order | List, form (jenis PO, baris, packaging untuk maklon), detail + approval + uang muka | 3.2–3.4 | Logistik & F&A |
| Penerimaan Barang | Pilih PO → baris: qty baik, qty reject, batch, tgl produksi, tgl kedaluwarsa; untuk maklon: pemakaian packaging baik + reject dari lokasi maklon | 3.4 | Logistik & F&A |
| Inbound dengan QC | Antrian barang Karantina; per baris: checklist (batch, expired, kondisi, segel), qty lulus/tolak, foto, lokasi tujuan | 3.2 + panel QC | Logistik |
| Transfer Stok | Dari → ke, baris + batch (saran FEFO), status Dalam Pengiriman, konfirmasi terima | 3.4 | Logistik |
| Stock Count | Pilih lokasi → lembar hitung (sistem disembunyikan saat hitung buta) → selisih → ajukan | 3.4 + form HP (bagian 4) | Logistik, Sales MT |
| Sales Order | Channel & pelanggan, harga otomatis + aturan yang dipakai (tooltip), diskon (cek batas role), alokasi batch, Proforma/Invoice terkait | 3.3–3.4 | Sales MT, CS |
| Outbound dengan QC | Picking list per lokasi/batch, packing, QC per baris; gagal QC → baris pengganti otomatis, peringatan stok kurang | 3.2 + panel QC | Logistik |
| Rekonsiliasi Konsinyasi | Tabel per SKU (stok awal, kiriman, tarikan, sisa fisik, terjual sistem, laporan partner, selisih, penyebab, ditanggung), unggah BA, total tagihan | 3.3 | Sales MT, F&A |
| Pesanan WhatsApp/Instagram | Cari pelanggan by HP / buat baru, item, ongkir, unggah bukti bayar, kirim invoice (salin teks/link PDF) | 3.4 | CS |
| Invoice & pembayaran | Invoice + status bayar; form pembayaran dengan alokasi ke banyak invoice | 3.3–3.4 | F&A |
| Jurnal Umum | Baris debit/kredit + dimensi, indikator balance, posting; jurnal otomatis read-only + tombol *Balik Jurnal* | 3.4 | F&A |
| Tutup Buku | Checklist 9 langkah dengan status, PIC, tautan ke laporan; tombol *Tutup Periode* (re-auth) | Checklist | Head F&A |

---

## 4. Pola Form HP (F0-UX-04)

Dipakai Stock Count gudang dan lokasi partner (WMS-11) dan, di fase berikutnya, refill vending, absensi, check-in sales.

```
┌───────────────────────────┐
│ ←  Stock Count            │
│ SC/2027/01/00006          │
│ Lokasi: Konsinyasi – Toko │
│ Sinar Jaya (Cut-off 31 Jan)│
├───────────────────────────┤
│ 🔍 Cari / pindai SKU       │
├───────────────────────────┤
│ Meal Shake Coklat Tube    │
│ Batch B2312 · ED 12 Des 27│
│  [ − ]   [  24  ]   [ + ] │  ← target sentuh ≥ 44 px
├───────────────────────────┤
│ Meal Shake Vanila Pouch   │
│ Batch B2311 · ED 30 Nov 27│
│  [ − ]   [   0  ]   [ + ] │
├───────────────────────────┤
│ 📷 Foto rak (wajib)  [ + ] │
├───────────────────────────┤
│ Tersimpan otomatis 10.42  │
│ [   Kirim Hasil Hitung   ]│  ← tombol penuh, sticky di bawah
└───────────────────────────┘
```

| Aturan | Implementasi |
|---|---|
| Target sentuh | Minimal 44 × 44 px untuk tombol, stepper, item list |
| Isi form sementara | Disimpan di `sessionStorage` per dokumen sampai submit sukses; indikator *Tersimpan otomatis* |
| Jaringan lemah | Submit memakai Idempotency-Key yang sama saat *Coba lagi*; error jaringan tidak menghapus isian |
| Foto | Kamera belakang (`capture="environment"`), dikompres ±1 MB sebelum upload, progress persentase |
| Hitung buta | Qty sistem tidak ditampilkan saat menghitung; selisih tampil setelah dikirim |
| Satu tangan | Aksi utama di bawah (sticky); input angka memakai keyboard numerik |

---

## 5. Uji Label (F0-UX-05)

| Langkah | Detail |
|---|---|
| Peserta | Satu perwakilan Finance & Accounting, Logistik, Sales (MT dan CS), Marketing, HRGA |
| Metode | Tree test: peserta diberi 8–10 tugas (mis. *"Catat barang dari maklon yang baru datang"*, *"Cari stok konsinyasi di Toko X"*, *"Ajukan biaya event"*) dan memilih menu di struktur bagian 2 tanpa layar |
| Ukuran | Persentase tugas yang menemukan menu benar pada percobaan pertama; target ≥ 80% per tugas |
| Label yang diuji khusus | Inbound/Outbound, Packaging & Material, Put-away, Stok dalam Pengiriman, Disposisi Retur, Barang Diterima Belum Ditagih, Terbuka (periode) |
| Hasil | Label yang < 80% diganti; perubahan dimasukkan ke Naming Convention dan `menu.ts` |
