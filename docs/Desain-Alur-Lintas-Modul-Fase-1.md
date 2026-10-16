# Desain Alur Lintas Modul Fase 1
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Sequence diagram alur yang melintasi banyak modul (F0-DSN-08) |
| Acuan | BRD v0.7 bagian 5.1 – 5.5, 5.11, 5.12, 5.15; Technical Guideline 5.2, 5.5, 5.7, 5.8, 7 |
| Status | Draft untuk direview tim implementasi |

Prinsip yang berlaku di semua alur:

- Satu aksi user = satu transaksi DB yang memuat mutasi stok, jurnal, approval, dan audit sekaligus (Technical Guideline 5.2, 7.3). Efek ke luar (email, PDF) di-enqueue sebagai job di transaksi yang sama.
- Modul memanggil modul lain lewat API publik root package; arah dependensi mengikuti Technical Guideline 2.3.
- Lock stok diambil berurutan `(product_id, location_id, batch_id, stock_status)` untuk mencegah deadlock.

---

## 1. Beli → Terima → QC → Stok → Jurnal

```mermaid
sequenceDiagram
    autonumber
    actor P as Purchasing / Head Logistik
    actor L as Logistik Leader
    actor G as Staf Gudang / Merchandiser
    participant PRC as procurement
    participant APR as approval
    participant WH as warehouse
    participant INV as inventory
    participant ACC as accounting

    P->>PRC: Submit PO (total)
    PRC->>APR: Request(purchase_order, nominal, divisi)
    APR-->>PRC: approval_request dibuat (level 1..n)
    Note over APR: Approver menyetujui tiap level
    APR->>PRC: OnApproved → PO approved (transaksi yang sama)
    PRC-->>L: Notifikasi ETA & detail barang (expected_date PO, H-1)
    Note over L: Atur armada & manpower (di luar sistem)
    G->>PRC: Post Penerimaan Barang (cek qty: baik, reject, batch, expired)
    PRC->>INV: Receive(movement +qty, status quarantine, lokasi Inbound, cost layer)
    PRC->>ACC: Post A-01 (Dr Persediaan, Cr Barang Diterima Belum Ditagih)
    PRC->>WH: Buat Inbound (baris quarantine)
    G->>WH: QC per baris (lulus / tolak, foto, packaging oleh tim Gudang GM)
    WH->>INV: Move quarantine → available ke lokasi tujuan (lulus)
    WH-->>PRC: Baris ditolak → reject penerimaan (retur RB atau diserap ke HPP)
    P->>PRC: Post Tagihan Pemasok
    PRC->>PRC: 3-way match (toleransi dari setting)
    alt Melebihi toleransi
        PRC->>APR: Request(supplier_bill_exception)
    end
    PRC->>ACC: Post A-18 (Dr Barang Diterima Belum Ditagih, Cr Utang Usaha, Cr Utang PPh 23)
```

Untuk PO maklon, langkah 6–7 memakai A-03: nilai batch = packaging terpakai (baik + reject, dari lokasi maklon) + jasa maklon; HPP per unit = nilai batch ÷ qty baik.

---

## 2. Sales Order → Alokasi FEFO → Outbound/QC → DO → Invoice → Jurnal

```mermaid
sequenceDiagram
    autonumber
    actor S as Sales / CS
    actor F as Finance & Accounting
    actor L as Logistik Leader
    actor G as Staf Gudang
    actor FL as Fulfillment Leader / Merchandiser
    participant SLS as sales
    participant PRI as pricing
    participant INV as inventory
    participant WH as warehouse
    participant LOG as logistics
    participant ACC as accounting

    S->>SLS: Buat SO (customer, channel, item)
    SLS->>PRI: Resolve harga per baris (business_date)
    PRI-->>SLS: Harga + aturan + versi
    S->>SLS: Confirm
    SLS->>INV: Allocate(komponen bundle dipecah, FEFO) → available → reserved
    SLS->>SLS: Terbitkan Proforma (MT beli putus / WA-IG transfer)
    F->>SLS: Konfirmasi pembayaran
    SLS->>ACC: Post A-05 + pembayaran (invoice langsung Lunas)
    L->>LOG: Release DO, cetak DO/surat jalan & label produk (kode produk mitra bila ada)
    LOG->>WH: Buat Outbound
    G->>WH: Picking & packing per batch, catat koli (label koli)
    FL->>WH: QC Outbound (cek ulang, bukan user yang menyiapkan)
    alt QC gagal
        WH->>INV: Baris ditolak → quarantine, alokasi pengganti FEFO
        WH-->>S: Notifikasi bila stok pengganti tidak cukup
    end
    FL->>WH: Complete Outbound
    FL->>LOG: Ship (kurir internal Merchandiser / kurir eksternal + resi)
    LOG->>INV: Issue reserved → keluar (konsumsi cost layer FIFO per batch)
    LOG->>ACC: Post A-06 (Dr HPP, Cr Persediaan FG — lokasi asal)
    LOG->>LOG: Status Dalam Pengiriman → Diterima
```

Alur ini adalah PO Offline dari klien MT dan reseller: Sales → Finance → Logistik Leader → Staf Gudang → Fulfillment Leader/Merchandiser. Marketplace: SO dibuat dari import; penjualan diakui saat DO `ship` (A-10 dan A-06 di transaksi yang sama).

---

## 3. Rekonsiliasi Konsinyasi → Invoice Otomatis

```mermaid
sequenceDiagram
    autonumber
    actor M as Sales MT
    actor MD as Merchandiser
    actor H as Head Finance & Accounting
    participant SLS as sales
    participant PTY as party
    participant WH as warehouse
    participant INV as inventory
    participant APR as approval
    participant ACC as accounting

    MD->>WH: Stock count lokasi partner yang ditugaskan (form HP, foto, cut-off akhir bulan)
    M->>SLS: Mulai rekonsiliasi (partner, periode)
    SLS->>PTY: Ambil PKS berlaku (harga, termin, ketentuan selisih)
    SLS->>INV: Mutasi lokasi partner periode (stok awal, kiriman, tarikan)
    SLS->>SLS: Terjual sistem = awal + kiriman − tarikan − sisa fisik, banding laporan partner
    M->>SLS: Isi penyebab selisih, unggah BA bertanda tangan
    M->>SLS: Submit
    SLS->>APR: Request(consignment_reconciliation): Sales MT → Head F&A
    H->>APR: Approve
    APR->>SLS: OnApproved
    SLS->>SLS: Buat Invoice konsinyasi (termin PKS) + invoice selisih bila ditagih ke partner
    SLS->>INV: Issue lokasi konsinyasi (terjual + selisih)
    SLS->>ACC: Post A-08 dan A-23
```

---

## 4. Tutup Buku Bulanan

```mermaid
sequenceDiagram
    autonumber
    actor H as Head Finance & Accounting
    participant ACC as accounting
    participant JOB as worker (river)
    participant INV as inventory

    H->>ACC: Buka checklist periode (9 langkah BRD 5.15)
    ACC->>ACC: Langkah 1: cek dokumen operasional belum diposting di periode
    Note over ACC: Langkah 2–5 diselesaikan modul terkait dan ditandai PIC
    ACC->>JOB: Enqueue accounting.depreciate (langkah 6)
    JOB->>ACC: Post A-31 per aset (idempotent per periode)
    ACC->>JOB: Enqueue PPh Final (langkah 7, bila aktif)
    H->>ACC: Review selisih persediaan & Barang Diterima Belum Ditagih (langkah 8)
    H->>ACC: Tutup periode
    ACC->>ACC: pg_advisory_xact_lock(periode), validasi neraca saldo balance
    ACC->>ACC: fiscal_periods.status = closed, audit log
    Note over ACC,INV: Setelah ditutup: transaksi bertanggal di periode itu ditolak PERIOD_LOCKED
```

Buka kembali periode: `request_reopen` → approval Head F&A → Direksi → status `open`, tercatat di audit trail (ACC-14).

---

## 5. Pemakaian untuk Event → Pengembalian Sisa → Jurnal

```mermaid
sequenceDiagram
    autonumber
    actor R as Sales / Marketing
    actor HD as Head Divisi
    actor L as Logistik Leader
    actor G as Staf Gudang
    actor FL as Fulfillment Leader / Merchandiser
    participant INV as inventory
    participant APR as approval
    participant WH as warehouse
    participant ACC as accounting

    R->>INV: Ajukan Pemakaian Internal (purpose event, lokasi Event, tanggal selesai event)
    INV->>APR: Request(internal_usage, nilai HPP, divisi)
    HD->>APR: Approve (HPP > Rp5 jt + Head F&A)
    APR->>INV: OnApproved
    L->>INV: Release (meneruskan & mengetahui) → Outbound dibuat
    G->>WH: Picking & packing FEFO
    FL->>WH: QC Outbound
    R->>WH: Serah terima (cek ulang barang)
    WH->>INV: Move FG → lokasi Event (tanpa jurnal), status Menunggu Penyelesaian
    FL->>FL: Antar ke event
    Note over R,FL: Setelah event selesai
    FL->>WH: Bawa sisa barang ke Inbound
    R->>INV: Settle: catat qty sisa (maks H+1)
    INV->>WH: Buat Inbound (source internal_usage_return)
    L->>WH: Cek ulang sisa barang (Logistik Leader / Staf Gudang)
    WH->>INV: Sisa lulus → lokasi FG, sisa rusak → Gudang Retur
    INV->>INV: Issue qty terpakai dari lokasi Event (FIFO per batch)
    INV->>ACC: Post A-11 (Dr Beban Marketing, Cr Persediaan FG) atas qty terpakai
```

Property dan booth event bukan stok dan tidak dicatat di sistem Fase 1; pengirimannya cukup diketahui Logistik Leader di luar sistem.

---

## 6. Retur dari MT Offline

```mermaid
sequenceDiagram
    autonumber
    actor S as Sales
    actor L as Logistik Leader
    actor FL as Fulfillment Leader
    actor MD as Merchandiser
    actor G as Staf Gudang
    participant SLS as sales
    participant WH as warehouse
    participant ACC as accounting

    S->>SLS: Buat Retur Penjualan (partner, invoice, item, alasan) → submit
    L->>SLS: Verify sesuai SOP retur (atau kembalikan ke draft dengan catatan)
    alt Perlu pickup
        FL->>SLS: Tugaskan Merchandiser
        MD->>SLS: Cek barang di lokasi partner, catat pickup
    end
    S->>SLS: Receive: cek ulang barang, foto & form retur
    SLS->>WH: Buat Inbound (Karantina)
    G->>WH: QC & usulan disposisi (Logistik Leader / Staf Gudang)
    WH-->>SLS: QC selesai
    SLS->>ACC: Post A-13 + nota kredit
```

Retur dari website, WA/IG, dan marketplace tidak melewati verifikasi; dokumen langsung membuat Inbound saat diajukan.

---

## 7. Permintaan Petty Stock

```mermaid
sequenceDiagram
    autonumber
    actor FL as Fulfillment Leader
    actor L as Logistik Leader
    actor G as Staf Gudang
    participant WH as warehouse
    participant INV as inventory

    FL->>WH: Ajukan permintaan transfer FG Non-Ecommerce → Petty Stock
    L->>WH: Release (meneruskan & mengetahui)
    WH->>INV: Reserve stok asal (FEFO)
    G->>WH: Siapkan & serahkan barang (Outbound)
    FL->>WH: Cek ulang & konfirmasi terima (qty diterima)
    WH->>INV: Move FG reserved → Petty Stock available, sisa reserved dilepas
```

Transfer antar lokasi dalam satu gudang tidak membentuk jurnal.
