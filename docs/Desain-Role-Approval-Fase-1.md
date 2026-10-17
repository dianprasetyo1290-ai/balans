# Desain Role, Permission & Approval Fase 1
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Matriks role & permission (F0-DSN-03) dan matriks approval final (F0-DSN-04) |
| Acuan | BRD v0.7 bagian 3.1, 5.17, 12.3; Technical Guideline 5.5, 9.1, 9.2 |
| Status | Draft. Nama approver dan batas nominal final diisi Balans, lalu disetujui Direksi |

---

## 1. Role Awal

Role Fase 1 diturunkan dari BRD 3.1. Role partner, reseller, dan self-service karyawan dipakai mulai Fase 2/5 dan tidak dikonfigurasi di Fase 1.

| Kode role | Nama (label UI) | MFA wajib | Ringkas |
|---|---|---|---|
| `system_admin` | Admin Sistem | Ya | Pengguna, role, pengaturan, nomor & template dokumen |
| `purchasing` | Purchasing / Logistik | — | Pengadaan, Delivery Order, transfer; lihat persediaan |
| `warehouse_staff` | Staf Gudang | — | Inbound/QC, transfer, picking/packing Outbound, stock count |
| `logistics_leader` | Logistik Leader | — | Dokumen PO & DO, cetak label, verifikasi retur MT, meneruskan permintaan barang, pengaturan armada & manpower |
| `fulfillment_leader` | Fulfillment Leader | — | Permintaan & penerimaan Petty Stock, QC Outbound, pengiriman & pickup, penugasan Merchandiser |
| `merchandiser` | Merchandiser | — | Kurir internal, pickup retur MT & sisa event, cek qty Inbound, QC Outbound, stock count lokasi konsinyasi |
| `head_logistics` | Head Logistik | — | Approval penyesuaian stok & disposisi retur, laporan gudang |
| `finance` | Finance & Accounting | — | Invoice, pembayaran, rekonsiliasi, pajak, aset, jurnal |
| `head_finance` | Head Finance & Accounting | Ya | Approval keuangan, tutup buku, buka kembali periode |
| `sales_mt` | Sales MT | — | MT beli putus, B2B, konsinyasi & rekonsiliasi |
| `sales_ecommerce` | Sales E-commerce | — | Penjualan marketplace (manual/import) |
| `customer_service` | Customer Service | — | Order WA/IG, retur customer |
| `head_sales` | Head Sales | — | Approval harga & diskon |
| `marketing` | Marketing | — | Pemakaian internal marketing, pengajuan biaya, kampanye |
| `hrga` | HRGA / Payroll | Ya | Data karyawan, sample karyawan |
| `division_head` | Head Divisi | — | Approver level 1 untuk PR, pengajuan biaya, pemakaian internal divisinya (scope divisi) |
| `director` | Direksi | Ya | Approval tingkat atas, semua laporan, read-only transaksi |
| `president_director` | Direktur Utama | Ya | Approval PO > Rp50 jt |

Staf Gudang, Logistik Leader, Fulfillment Leader, dan Merchandiser berada di divisi Logistik & FAT; Fulfillment adalah tim di dalam Warehouse. Logistik Leader satu tingkat di bawah Head Logistik: meneruskan dan mengetahui pengeluaran barang, tetapi tidak memberi approval.

Satu user boleh memiliki beberapa role, misalnya jabatan *Acting FAT & Logistik Head* memegang `head_finance` dan `head_logistics` sekaligus; pengganti approver untuk user ini wajib diisi karena pembuat tidak boleh menyetujui dokumennya sendiri. Role yang wajib MFA mengikuti Technical Guideline 9.1 (Admin Sistem, Head Finance & Accounting, Direksi, HRGA/Payroll).

---

## 2. Format Permission

Permission berformat `<modul>.<resource>.<aksi>` (Technical Guideline 9.2). Aksi standar:

| Aksi | Arti |
|---|---|
| `read` | Melihat list & detail |
| `write` | Membuat dan mengubah draft, menghapus draft |
| `submit` | Mengajukan (memberi nomor, memulai approval) |
| `approve` | Memberi keputusan approval (dicek terhadap `approval_policies`, bukan hanya permission) |
| `post` | Memposting / mengeksekusi (mutasi stok, jurnal) |
| `cancel` | Membatalkan / membuat dokumen pembatalan |
| `verify` | Memeriksa kesesuaian data dengan SOP sebelum proses fisik (tanpa approval berjenjang) |
| `import` / `export` | Import/export Excel |

Aksi bertanda ★ butuh **re-autentikasi** (password/TOTP, berlaku 5 menit): `accounting.fiscal_period.reopen`, `accounting.payment.post` (pembayaran keluar), `identity.user_role.write`, `setting.setting.write`.

---

## 3. Matriks Role × Resource

Kode: R = read, W = write, S = submit, A = approve, P = post, C = cancel, V = verify, I = import/export. Approve hanya berlaku jika user juga tercantum di kebijakan approval yang cocok. Hak akses Logistik Leader, Fulfillment Leader, dan Merchandiser ada di 3.5.

### 3.1 Master Data & Pengaturan

| Resource (permission) | Admin | Purch | Gudang | HLog | FA | HFA | SMT | SEC | CS | HSal | Mkt | HRGA | HDiv | Dir |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `catalog.product` | RWI | R | R | R | R | R | R | R | R | R | R | R | R | R |
| `pricing.price_list`, `pricing.promotion`, `pricing.special_price` | R | — | — | — | R | A | RWS | RWS | R | RWSA | R | — | — | R |
| `pricing.discount_limit` | RW | — | — | — | — | A | — | — | — | RWS | — | — | — | R |
| `party.customer` | RWI | R | — | — | R | R | RW | RW | RW | R | R | — | — | R |
| `party.partner`, `party.partner_agreement`, `party.partner_product_code` | RWI | R | — | — | R | R | RW | — | — | RW | — | — | — | R |
| `party.supplier` | RWI | RW | — | R | RW | R | — | — | — | — | — | — | — | R |
| `warehouse.location` | RWI | R | R | RW | R | R | R | R | — | R | — | — | — | R |
| `hris.employee` | R | — | — | — | R | R | — | — | — | — | — | RWI | — | R |
| `accounting.account` | R | — | — | — | RWI | RWAI | — | — | — | — | — | — | — | R |
| `accounting.posting_rule` | R | — | — | — | RW | RWA | — | — | — | — | — | — | — | R |
| `accounting.dimension` (channel, sub-channel, divisi, kampanye) | RW | — | — | — | RW | RW | R | R | — | R | RW | — | — | R |
| `accounting.expense_type`, `sales.return_reason` | RW | — | — | R | RW | RW | — | — | R | — | R | — | — | R |
| `identity.user`, `identity.user_role` ★ | RW | — | — | — | — | — | — | — | — | — | — | — | — | R |
| `approval.policy` | RW | — | — | — | R | R | — | — | — | — | — | — | — | RA |
| `setting.setting` ★, `document.template`, `document.sequence` | RW | — | — | — | R | RW | — | — | — | — | — | — | — | R |

### 3.2 Pengadaan, Gudang, Persediaan

| Resource | Admin | Purch | Gudang | HLog | FA | HFA | SMT | SEC | CS | HSal | Mkt | HRGA | HDiv | Dir |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `procurement.purchase_request` | R | RWS | — | RWS | RWS | R | RWS | RWS | — | RWS | RWS | RWS | RA | RA |
| `procurement.purchase_order` | R | RWSC | R | R | R | RA | — | — | — | — | — | — | — | RA |
| `procurement.goods_receipt` | R | RWPC | RWP | R | R | R | — | — | — | — | — | — | — | R |
| `procurement.supplier_bill` | R | R | — | — | RWPC | RA | — | — | — | — | — | — | — | R |
| `procurement.purchase_advance` | R | RW | — | — | RWP | R | — | — | — | — | — | — | — | R |
| `procurement.purchase_return` | R | RWSP | RWP | RA | R | R | — | — | — | — | — | — | — | R |
| `warehouse.inbound` (QC) | R | R | RWP | RWP | R | R | — | — | — | — | — | — | — | R |
| `warehouse.stock_transfer` | R | RWPC | RWP | RWP | R | R | R | R | — | — | — | — | — | R |
| `warehouse.stock_count` | R | R | RWS | RWSA | R | RA | RWS | — | — | — | — | — | — | R |
| `warehouse.outbound` (picking/packing) | R | R | RWP | RWP | R | — | R | R | R | — | — | — | — | R |
| `warehouse.outbound_qc` (QC sebelum diserahkan) | R | R | R | RWP | R | — | R | R | R | — | — | — | — | R |
| `warehouse.return_disposition` | R | R | RWS | RA | RA | RA | — | — | — | — | — | — | — | RA |
| `inventory.stock` (kartu stok, posisi) | R | R | R | R | R | R | R | R | R | R | R | R | R | R |
| `inventory.internal_usage` | R | R | RP | R | R | RA | RWS | — | — | — | RWS | RWS | RA | R |
| `inventory.repacking`, `inventory.disposal` | R | R | RWP | RWSA | RA | RA | — | — | — | — | — | — | — | RA |

### 3.3 Penjualan, Logistik

| Resource | Admin | Purch | Gudang | HLog | FA | HFA | SMT | SEC | CS | HSal | Mkt | HRGA | HDiv | Dir |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `sales.sales_order` | R | R | R | R | R | R | RWSC | RWSC | RWSC | RA | R | — | — | R |
| `sales.marketplace_order` (import) | R | — | — | — | R | — | — | RWI | — | R | — | — | — | R |
| `sales.proforma_invoice`, `sales.invoice` | R | R | — | — | RWPC | R | RWS | R | RWS | R | — | — | — | R |
| `sales.credit_note`, `sales.sales_return` | R | R | RW | R | RWP | RA | RWS | RWS | RWS | R | — | — | — | R |
| `sales.consignment_reconciliation` | R | R | — | — | RW | RA | RWSA | — | — | R | — | — | — | R |
| `logistics.delivery_order`, `logistics.shipment` | R | RWPC | RWP | R | R | — | RWS | RWS | RWS | — | — | — | — | R |
| `logistics.label` (cetak label produk & koli) | R | P | P | P | — | — | P | P | P | — | — | — | — | — |

### 3.4 Keuangan

| Resource | Admin | Purch | Gudang | HLog | FA | HFA | SMT | SEC | CS | HSal | Mkt | HRGA | HDiv | Dir |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `accounting.journal` | R | — | — | — | RWPC | RWPCA | — | — | — | — | — | — | — | R |
| `accounting.payment` ★ (keluar) / masuk | R | — | — | — | RWP | RA | — | — | R | — | — | — | — | R |
| `accounting.bank_reconciliation`, `accounting.settlement` | R | — | — | — | RWPI | R | — | — | — | — | — | — | — | R |
| `accounting.expense_claim` | R | RWS | RWS | RWS | RWP | RA | RWS | RWS | RWS | RWS | RWS | RWS | RA | RA |
| `accounting.petty_cash` | R | — | — | — | RWP | RA | — | — | — | — | — | — | — | R |
| `accounting.withholding` (PPh), `accounting.final_tax` | R | — | — | — | RWPI | RA | — | — | — | — | — | — | — | R |
| `accounting.fixed_asset` | R | — | — | — | RWP | RA | — | — | — | — | — | — | — | R |
| `accounting.fiscal_period` (tutup buku; buka kembali ★) | R | — | — | — | RW | RWPA | — | — | — | — | — | — | — | A |
| `accounting.report` (laporan keuangan & dimensi) | — | — | — | — | R | R | — | — | — | — | — | — | — | R |
| `dataexchange.import` (transaksi) | R | RI | — | — | RI | RI | RI | RI | — | — | — | RI | — | — |

### 3.5 Tim Gudang: Logistik Leader, Fulfillment Leader, Merchandiser

| Resource | Logistik Leader | Fulfillment Leader | Merchandiser |
|---|---|---|---|
| `catalog.product`, `party.partner_product_code` | R | R | R |
| `procurement.purchase_order` | R | — | — |
| `procurement.goods_receipt` | RWP | R | RW (cek qty) |
| `warehouse.inbound` (QC) | RWP | R | R |
| `warehouse.stock_transfer` | RWPC (release permintaan) | RWS (ajukan & konfirmasi terima Petty Stock) | R |
| `warehouse.stock_count` | RWS | RWS (Petty Stock) | RWS (lokasi konsinyasi yang ditugaskan) |
| `warehouse.outbound` (picking/packing) | RWP | R | R |
| `warehouse.outbound_qc` | R | RWP | RWP |
| `warehouse.return_disposition` | RWS | — | — |
| `inventory.stock` | R | R | R |
| `inventory.internal_usage` | RP (release) | R | R |
| `sales.sales_order` | R | R | — |
| `sales.sales_return` | RV | RW (penugasan pickup) | RW (pickup & cek barang) |
| `logistics.delivery_order`, `logistics.shipment` | RWPC | RWP | RP (antar & konfirmasi diterima) |
| `logistics.label` | P | P | — |
| `dataexchange.import` (transaksi) | RI | — | — |

QC Outbound tidak boleh dilakukan user yang sama dengan yang menyiapkan barang (`outbounds.prepared_by`). Serah terima Pemakaian Internal dikonfirmasi divisi peminta (role pengaju `inventory.internal_usage`).

Scoping data: `division_head` hanya melihat dan menyetujui dokumen divisinya (`user_roles.scope_division_id`). `merchandiser` hanya melihat dan mengisi stock count lokasi konsinyasi partner yang ditugaskan kepadanya (`partners.merchandiser_user_id`). Data gaji tidak ada di Fase 1.

---

## 4. Matriks Approval Final

Batas nominal adalah nilai awal BRD 5.17 dan disimpan di `approval_policies` (dapat diubah tanpa pengembangan). Approver ditetapkan **per jabatan**; kolom nama diisi Balans dan dapat diganti lewat konfigurasi saat struktur organisasi berubah (PRD risiko #7). Setiap approver wajib punya **pengganti** untuk delegasi saat cuti.

| # | Dokumen (`subject_type`) | Level | Kondisi | Jabatan approver | Nama approver | Pengganti |
|---|---|---|---|---|---|---|
| 1 | Purchase Request (`purchase_request`) | 1 | Semua | Head divisi peminta | | |
| 2 | Purchase Order (`purchase_order`) | 1 | Semua nominal | Head Finance & Accounting | | |
| | | 2 | > Rp10.000.000 | Direksi | | |
| | | 3 | > Rp50.000.000 | Direktur Utama | | |
| 3 | Pengajuan biaya / reimbursement (`expense_claim`) | 1 | Semua nominal | Head divisi pengaju | | |
| | | 2 | > Rp5.000.000 | Head Finance & Accounting | | |
| | | 3 | > Rp25.000.000 | Direksi | | |
| 4 | Pemakaian internal FG (`internal_usage`) | 1 | Semua | Head divisi peminta | | |
| | | 2 | Nilai HPP > Rp5.000.000 | Head Finance & Accounting | | |
| 5 | Penyesuaian stok (`stock_count`) | 1 | Ada selisih | Head Logistik | | |
| | | 2 | Nilai selisih absolut > Rp2.000.000 | Head Finance & Accounting | | |
| 6 | Disposisi retur (`return_disposition`) | 1 | Semua | Head Logistik | | |
| | | 2 | Ada baris pemusnahan | Head Finance & Accounting | | |
| | | 3 | Nilai pemusnahan > Rp10.000.000 | Direksi | | |
| 7 | Pemusnahan non-retur (`disposal`) | sama dengan #6 level 1–3 | | | | |
| 8 | Retur barang reject QC Outbound ke maklon | — | Tanpa approval; diproses Logistik (D-24) | — | — | — |
| 9 | Rekonsiliasi konsinyasi & penagihan selisih (`consignment_reconciliation`) | 1 | Semua | Sales MT (PIC partner) | | |
| | | 2 | Semua | Head Finance & Accounting | | |
| 10 | Price list baru / perubahan harga (`price_list_version`) | 1 | Semua | Head Sales | | |
| | | 2 | Semua | Head Finance & Accounting | | |
| 11 | Diskon manual di atas batas role (`sales_order_discount`) | 1 | Diskon > batas role pembuat | Head Sales | | |
| 12 | Tagihan pemasok melebihi toleransi 3-way match (`supplier_bill_exception`) | 1 | Qty ≠ 0% atau harga > 1% | Head Finance & Accounting | | |
| 13 | Pengisian kembali kas kecil (`petty_cash_replenishment`) | 1 | Semua | Head Finance & Accounting | | |
| 14 | Buka kembali periode (`fiscal_period_reopen`) | 1 | Semua | Head Finance & Accounting | | |
| | | 2 | Semua | Direksi | | |

Baris BRD 5.17 yang berlaku di fase berikutnya (tidak dikonfigurasi di Fase 1): penjualan tempo melebihi batas kredit (U-30, jika diambil), perubahan level reseller (Fase 2), cuti/izin/lembur dan payroll bulanan (Fase 5).

**Aturan engine (Technical Guideline 5.5):**

- Kebijakan dipilih berdasarkan `subject_type`, nominal, dan divisi pada saat submit; perubahan kebijakan setelahnya tidak memengaruhi dokumen yang sedang berjalan.
- Level berurutan; level berikutnya baru aktif setelah level sebelumnya menyetujui.
- Pembuat dokumen tidak boleh menyetujui dokumennya sendiri; jika pembuat adalah approver level tersebut, level itu naik ke pengganti.
- Pengingat approval tertunda dikirim setiap 1 jam pada jam kerja (job `approval.remind`, NFR-11).

---

## 5. Data yang Dibutuhkan dari Balans

| Data | Untuk |
|---|---|
| Nama dan email untuk setiap jabatan di tabel bagian 4, beserta pengganti | Konfigurasi `approval_policies` dan `approval_delegations` |
| Daftar user awal per role (nama, email, divisi) | Pembuatan akun R1.1 |
| Merchandiser yang ditugaskan untuk setiap partner MT/konsinyasi | `partners.merchandiser_user_id` (scope stock count) |
| Konfirmasi batas nominal di tabel bagian 4 (nilai awal BRD atau nilai pengganti) | F0-APR-03 |
| Batas diskon manual per role (persen per baris dan per dokumen) | `discount_limits` (MD-12; ditetapkan di workshop Sales) |
