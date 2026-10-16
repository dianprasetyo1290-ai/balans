# Naming Convention — Menu & Feature Labeling
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Naming Convention UI Labels |
| Versi | 1.0 — Draft |
| Acuan | BRD Balans Unified Business Platform v0.7 dan Product Roadmap v0.6 |
| Ruang lingkup | Label modul/menu dan fitur yang terlihat oleh pengguna |
| Di luar ruang lingkup | Technical naming: kode, database, API, route, component, event, permission key, dan identifier lainnya |

## 1. Tujuan

Dokumen ini menetapkan standar penamaan **menu dan fitur pada UI** agar konsisten di ERP/back-office, website e-commerce, portal reseller, dan halaman self-service. Dokumen ini hanya mengatur label yang dilihat pengguna; technical naming mengikuti *Technical Guideline v0.1*.

## 2. Prinsip Penamaan

1. **Gunakan Bahasa Indonesia untuk menu operasional internal.** Istilah bisnis yang lazim dan sudah digunakan di Balans boleh tetap dalam bahasa Inggris, misalnya *Purchase Order*, *Stock Count*, *Sales Order*, *Payroll*, dan *General Ledger*.
2. **Gunakan istilah yang berorientasi pada pekerjaan pengguna.** Pengguna harus dapat memahami fungsi menu tanpa membaca dokumentasi teknis.
3. **Pilih satu istilah untuk satu konsep.** Jangan mencampur istilah seperti *Stock Opname* dan *Stock Count* untuk fungsi yang sama.
4. **Gunakan pola kata benda untuk menu dan nama proses/aksi untuk fitur.** Contoh menu: `Persediaan`; fitur: `Transfer Stok`.
5. **Gunakan Title Case secara konsisten.** Kapitalisasi kata utama dipakai pada label menu; hindari kapitalisasi seluruh kata.
6. **Hindari singkatan internal pada label UI.** Kode seperti `PRC`, `WMS`, `INV`, `SLS`, dan `ACC` hanya untuk dokumentasi/technical reference, kecuali memang menjadi istilah yang dipahami pengguna.
7. **Bedakan nama modul dari nama fitur.** Modul adalah kelompok pekerjaan; fitur adalah kemampuan spesifik di dalam modul.
8. **Gunakan label yang ringkas.** Utamakan 1–3 kata. Tambahkan konteks hanya jika diperlukan untuk menghindari ambiguitas.
9. **Gunakan istilah yang sama di seluruh UI.** Label di menu, tombol, breadcrumb, judul halaman, notifikasi, dan dokumentasi pengguna harus konsisten.
10. **Jangan menamai menu berdasarkan jabatan pengguna.** Menu mewakili fungsi bisnis; aksesnya diatur oleh role/permission.

## 3. Aturan Format Label

| Elemen UI | Pola | Contoh |
|---|---|---|
| Modul/menu utama | Kata benda atau nama domain | `Pengadaan`, `Gudang`, `Persediaan` |
| Submenu | Objek atau kelompok pekerjaan | `Purchase Order`, `Penerimaan Barang`, `Daftar Produk` |
| Fitur/halaman | Objek + konteks bila perlu | `Stok per Lokasi`, `Rekonsiliasi Konsinyasi` |
| Tombol aksi | Kata kerja + objek | `Buat Purchase Order`, `Terima Barang`, `Ajukan Cuti` |
| Status | Kata/frasa singkat yang menjelaskan kondisi | `Draft`, `Menunggu Persetujuan`, `Disetujui`, `Ditolak`, `Selesai` |
| Kolom/tabel | Nama data yang spesifik | `Nomor PO`, `Tanggal Penerimaan`, `Jumlah Diterima` |
| Laporan | `Laporan` + objek | `Laporan Mutasi Stok`, `Laporan Penjualan` |
| Pengaturan | `Pengaturan` + konteks | `Pengaturan Harga`, `Pengaturan Persetujuan` |

## 4. Standar Istilah

| Konsep | Label yang direkomendasikan | Hindari variasi |
|---|---|---|
| Purchase Request | `Purchase Request` | PR, Permintaan Pembelian, Purchase Requisition digunakan bergantian |
| Purchase Order | `Purchase Order` | PO sebagai label menu; boleh digunakan di kolom/percakapan jika konteks jelas |
| Goods Receipt | `Penerimaan Barang` | Received Stock, Goods Receiving, Penerimaan Stok |
| Finished Goods | `Produk Jadi` | Finish Goods, Finish Good, FG sebagai label UI |
| Raw Material (RM, bahan mentah) / packaging dalam konteks Balans | `Packaging` | Kemasan, Material Kemasan |
| Goods Material (GM, bahan jadi): barang retur untuk diproses ulang maklon | `Material` | Bahan Reprocess, Raw Material dicampur |
| Gudang GM | `Gudang GM`; lokasinya `RM` dan `GM` | Gudang Material, Gudang Bahan |
| Warehouse Management | `Gudang` | WMS sebagai label menu |
| Inventory | `Persediaan` | Inventory dan Stok digunakan bergantian untuk nama menu utama |
| Stock Count | `Stock Count` | Stock Opname, Stock Count, Penghitungan Stok dicampur |
| Inbound | `Inbound` | Penerimaan & QC, Barang Masuk, Penerimaan Gudang |
| Outbound | `Outbound` | Barang Keluar, Pengeluaran Barang |
| Quality Control | `QC` — langkah di dalam Inbound dan Outbound, bukan menu tersendiri | Quality Control, Pemeriksaan Kualitas, Cek Barang dicampur |
| Picking & Packing | `Picking & Packing` — langkah di dalam Outbound | Proses picking/packing dengan variasi ejaan |
| Delivery Order | `Delivery Order` | DO sebagai label menu |
| Sales Order | `Sales Order` | SO sebagai label menu |
| Sales Pipeline | `Pipeline Penjualan` | Sales Pipeline dan Pipeline Penjualan dicampur |
| CRM | `Pelanggan (CRM)` | Customer Management, CRM, Pelanggan dipakai bergantian |
| Consignment | `Konsinyasi` | Titip jual jika konteks formal dan operasional sudah menggunakan Konsinyasi |
| Reseller | `Reseller` | Agen/distributor jika model bisnisnya berbeda |
| Vending Machine | `Mesin Vending` | VM, VND sebagai label UI |
| E-commerce | `E-commerce` | Ecommerce, E Commerce |
| Human Resource Information System | `SDM (HRIS)` | HR, HRIS, HRGA digunakan bergantian |
| Finance & Accounting | `Keuangan & Akuntansi` | FAT, ACC sebagai label UI |
| Accounts Receivable | `Piutang Usaha` | AR sebagai label menu |
| Accounts Payable | `Utang Usaha` | AP sebagai label menu |
| General Ledger | `Buku Besar` | GL sebagai label menu |
| Cost of Goods Sold | `Harga Pokok Penjualan (HPP)` | COGS dan HPP dicampur pada UI berbahasa Indonesia |
| Expiry Date | `Tanggal Kedaluwarsa` | Expired, Expiry, ED dicampur |
| Batch / Lot | `Batch` | Batch Number, Lot, Nomor Lot digunakan tanpa standar |
| Customer | `Pelanggan` | Customer dan Pelanggan digunakan bergantian dalam UI internal |
| Supplier | `Pemasok` | Vendor dan Supplier dicampur, kecuali membedakan pihak berbeda |
| Partner Consignment | `Mitra Konsinyasi` | Partner, Mitra, Klien digunakan bergantian |
| Refund | `Refund` | Refund dan Pengembalian Dana dicampur |
| Return | `Retur` | Return dan Retur dicampur |
| Stock Movement | `Mutasi Stok` | Pergerakan Stok, Stock Movement, Mutasi Stok dicampur |
| Internal Usage | `Pemakaian Internal` | Penggunaan Internal, Pemakaian Internal dicampur |
| Reconciliation | `Rekonsiliasi` | Reconcile, Pencocokan digunakan bergantian untuk proses yang sama |
| Approval | `Persetujuan` | Approval dan Persetujuan dicampur pada label UI |
| Dashboard | `Ringkasan` untuk halaman ringkasan operasional; `Dashboard` boleh dipertahankan untuk halaman eksekutif | Overview, Dashboard, Ringkasan digunakan untuk halaman yang sama |

Beberapa istilah Inggris dipertahankan karena lazim digunakan dalam operasi bisnis. Pilihan final per istilah harus konsisten; jangan menampilkan dua sinonim untuk fungsi yang sama.

## 5. Struktur Menu yang Direkomendasikan

Struktur berikut adalah usulan label UI berdasarkan domain pada BRD v0.7. Nama menu dapat disesuaikan saat desain UX, tetapi istilah yang dipilih perlu dipakai konsisten.

| Menu Utama | Submenu / Fitur |
|---|---|
| **Ringkasan** | Dashboard Eksekutif; Ringkasan Operasional |
| **Data Master** | Produk; Packaging & Material; Bundling; Daftar Harga; Aturan Harga; Pelanggan; Mitra Konsinyasi; Kode Produk Mitra; Reseller; Pemasok & Maklon; Gudang & Lokasi; Karyawan; Alasan Retur; Jenis Biaya; Kampanye |
| **Pengadaan** | Purchase Request; Purchase Order; Purchase Order Maklon; Purchase Order Reprocess; Persetujuan Pengadaan; Penerimaan Barang; Reject & Waste; Tagihan Pemasok; Uang Muka Pembelian |
| **Gudang** | Inbound; Put-away; Transfer Stok; Permintaan Petty Stock; Stok dalam Pengiriman; Outbound; Stock Count; Gudang Retur; Disposisi Retur; Riwayat Mutasi Stok |
| **Persediaan** | Stok per Lokasi; Kartu Stok; Batch & Kedaluwarsa; Alokasi Batch (FEFO); HPP per Batch; Pemakaian Internal; Repacking; Pemusnahan; Peringatan Stok Minimum; Peringatan Kedaluwarsa |
| **Logistik** | Delivery Order; Pengiriman; Kurir & Resi; Label Produk; Label Koli; Label Pengiriman; Pelacakan Pengiriman |
| **Penjualan** | Sales Order; Modern Trade; Konsinyasi; Penjualan Marketplace; Pesanan WhatsApp/Instagram; Penjualan Karyawan; Proforma Invoice; Invoice; Retur Penjualan; Nota Kredit; Rekonsiliasi Konsinyasi; Harga & Diskon |
| **Pipeline Penjualan** | Lead; Prospek; Peluang Penjualan; Aktivitas Sales; Kunjungan; Target Penjualan; Pencapaian Sales |
| **Pelanggan (CRM)** | Daftar Pelanggan; Riwayat Pesanan; Segmentasi Pelanggan; Interaksi Customer Service |
| **Reseller** | Daftar Reseller; Verifikasi Reseller; Level Reseller; Harga Reseller; Pesanan Reseller; Deposit & Tagihan; Evaluasi Level; Portal Reseller |
| **Mesin Vending** | Daftar Mesin; Lokasi & Slot; Stok Mesin; Buffer Vending; Refill; Stock Count Mesin; Transaksi Mesin; Rekonsiliasi Vending; Laporan per Mesin |
| **E-commerce** | Katalog Produk; Kategori; Pencarian; Keranjang; Checkout; Pesanan; Pembayaran; Voucher; Subscription; Member & Poin; Ulasan & Rating; Wishlist; Referral; Retur & Refund; CMS; SEO & Analytics |
| **SDM (HRIS)** | Data Karyawan; Struktur Organisasi; Jabatan & Kontrak; Absensi; Jadwal & Shift; Cuti & Izin; Lembur; Reimbursement; Payroll; BPJS; PPh 21; Slip Gaji; Self-Service |
| **Keuangan & Akuntansi** | Ringkasan Keuangan; Chart of Accounts; Jurnal Umum; Buku Besar; Piutang Usaha; Utang Usaha; Kas & Bank; Rekonsiliasi Bank; Kas Kecil; Biaya Operasional; Anggaran Biaya; Pajak & Bukti Potong; Aset Tetap; Penyusutan; Tutup Buku; Saldo Awal; Laporan Keuangan |
| **Laporan** | Laporan Stok; Laporan Mutasi Stok; Laporan Stock Count; Laporan HPP; Laporan Kedaluwarsa; Laporan Penjualan; Laporan Konsinyasi; Laporan Retur; Laporan Pemakaian Internal; Laporan Pengiriman; Laporan Piutang; Laporan Utang; Laporan Laba Rugi per Dimensi; Laporan Pajak; Laporan Pipeline; Laporan HR; Laporan E-commerce |
| **Integrasi** | Midtrans; Biteship; Marketplace; Mesin Vending; WhatsApp Business; Import/Export Excel |
| **Pengaturan** | Pengguna & Role; Hak Akses; Matriks Persetujuan; Notifikasi; Nomor Dokumen; Template Dokumen; Preferensi Sistem |

### Ketentuan struktur menu

- `Data Master` berisi data referensi. Transaksi harian tetap berada di modul proses terkait.
- `Persediaan` berfokus pada saldo, valuasi, batch, dan pemakaian; `Gudang` berfokus pada eksekusi fisik.
- `Inbound` mencakup semua barang masuk ke gudang (pemasok, maklon, retur pelanggan, tarikan konsinyasi) beserta QC-nya. `Outbound` mencakup picking, packing, dan QC sebelum barang diserahkan ke pengiriman. QC tidak menjadi menu tersendiri.
- `Penerimaan Barang` di `Pengadaan` adalah pencatatan penerimaan terhadap PO; proses fisik dan QC-nya dijalankan di `Inbound`.
- `Penjualan` menangani transaksi sampai invoice dan retur; `Pipeline Penjualan` menangani akuisisi dan progres peluang.
- `Pelanggan (CRM)` mengelola profil dan histori hubungan pelanggan; daftar customer master tetap dikelola sebagai data referensi bersama.
- Menu `E-commerce` untuk back-office website dibedakan dari website storefront yang dilihat pembeli.
- Menu `Integrasi` hanya untuk konfigurasi, status koneksi, log sinkronisasi, dan penanganan error integrasi.
- Menu yang hanya tersedia untuk role tertentu tetap memakai nama yang sama; akses dikontrol lewat role/permission.
- Menu opsional seperti subscription, poin, ulasan, wishlist, referral, bundling fisik, batas kredit, dan anggaran biaya harus mengikuti keputusan scope BRD sebelum ditampilkan.

## 6. Pola Label Fitur dan Aksi

| Konteks | Pola | Contoh |
|---|---|---|
| Membuat dokumen | `Buat + nama dokumen` | `Buat Purchase Order`, `Buat Sales Order` |
| Mengajukan | `Ajukan + objek` | `Ajukan Purchase Request`, `Ajukan Cuti` |
| Memproses penerimaan | Kata kerja + objek | `Terima Barang`, `Periksa Barang` |
| Memindahkan stok | Kata kerja + objek | `Transfer Stok`, `Kirim ke Maklon` |
| Menyelesaikan proses | Kata kerja + hasil | `Selesaikan Rekonsiliasi`, `Tutup Periode` |
| Persetujuan | `Setujui` / `Tolak` | `Setujui Purchase Order`, `Tolak Permintaan` |
| Koreksi | Kata kerja yang menjelaskan tindakan | `Sesuaikan Stok`, `Batalkan Pesanan` |
| Pencarian | `Cari + objek` | `Cari Produk`, `Cari Pelanggan` |
| Ekspor/impor | `Impor/Ekspor + objek` | `Impor Data Produk`, `Ekspor Laporan Penjualan` |
| Pengaturan | `Pengaturan + konteks` | `Pengaturan Harga`, `Pengaturan Payroll` |

Hindari label aksi generik seperti `Submit`, `Process`, `Manage`, atau `Action` jika label yang lebih spesifik dapat digunakan.

## 7. Status Dokumen

Gunakan status yang konsisten lintas modul jika maknanya sama. Status khusus proses boleh ditambahkan sesuai kebutuhan.

Status khusus periode akuntansi: `Terbuka` dan `Ditutup`.

| Status | Pemakaian |
|---|---|
| Draft | Dokumen masih disusun dan belum diajukan |
| Diajukan | Permintaan atau laporan sudah dikirim dan menunggu diperiksa (tanpa approval berjenjang) |
| Terverifikasi | Data sudah diperiksa kesesuaiannya dengan SOP |
| Menunggu Persetujuan | Dokumen menunggu keputusan approver |
| Disetujui | Dokumen telah disetujui |
| Ditolak | Pengajuan ditolak |
| Diproses | Proses sedang berlangsung |
| Menunggu Pembayaran | Tagihan belum lunas dan pembayaran ditunggu |
| Dibayar Sebagian | Baru sebagian nilai dibayar |
| Lunas | Seluruh nilai telah dibayar |
| Diposting | Dokumen sudah dibukukan dan tidak bisa diubah; koreksi lewat pembatalan |
| Dibatalkan | Dokumen dibatalkan dengan alasan |
| Selesai | Proses telah selesai |
| Menunggu Penyelesaian | Barang sudah dipakai di luar gudang (mis. event); sisa barang belum dicatat |
| Ditutup | Dokumen ditutup sebelum seluruh qty terpenuhi, dengan alasan |
| Kedaluwarsa | Masa berlaku habis |
| Dalam Pengiriman | Barang sedang dikirim / in-transit |
| Diterima Sebagian | Baru sebagian qty yang diterima |
| Diterima | Barang atau dokumen penerimaan telah dikonfirmasi |
| Karantina | Barang ditahan untuk pemeriksaan |
| Ditolak QC | Barang tidak lolos pemeriksaan kualitas |

Status harus mengikuti lifecycle proses yang relevan. Jangan menampilkan seluruh status pada setiap modul jika tidak sesuai.

## 8. Aturan Konsistensi dan Review

Sebelum label dirilis:
- Pastikan satu konsep memiliki satu label kanonis.
- Pastikan nama menu sesuai dengan tugas pengguna dan tidak mencampur beberapa domain yang tidak terkait.
- Pastikan label tombol menyatakan aksi dan objeknya.
- Pastikan istilah pada halaman, breadcrumb, notifikasi, ekspor, dan dokumentasi konsisten.
- Pastikan label tidak menampilkan kode teknis atau singkatan internal yang tidak diperlukan pengguna.
- Pastikan fitur opsional belum ditampilkan sebelum scope disetujui.
- Uji label dengan pengguna dari Finance & Accounting, Logistik, Sales, Marketing, HRGA, partner, dan reseller.
- Perbarui dokumen ini ketika perubahan BRD mengubah istilah atau struktur menu.

## 9. Di Luar Ruang Lingkup

Dokumen ini tidak mengatur:
- Kode modul dan requirement seperti `PRC-01`, `WMS-02`, atau `ACC-03`.
- Nama tabel/kolom database, API endpoint, event, permission key, route, component, file, dan variable.
- Format nomor dokumen bisnis seperti nomor PO, invoice, atau Delivery Order.
- Arsitektur navigasi final, layout halaman, atau desain visual UI.
- Hak akses detail selain prinsip bahwa role tidak mengubah label yang sama.

Technical naming mengikuti *Technical Guideline v0.1*.
