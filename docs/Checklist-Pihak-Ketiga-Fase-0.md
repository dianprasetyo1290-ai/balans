# Checklist Pihak Ketiga Fase 0
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Kebutuhan dan langkah pihak ketiga Fase 0 (F0-EXT-01 s.d. F0-EXT-05) |
| Acuan | PRD Fase 0 bagian 6.7, 9; Roadmap v0.6 bagian 13; Technical Guideline 13, 14 |
| Pemilik akun | Semua akun dibuat dan dimiliki atas nama Balans (BRD asumsi #11); tim implementasi diberi akses sebagai anggota |

---

## 1. Provider VPS & Object Storage (F0-EXT-01)

**Kriteria pemilihan VPS:**

| Kriteria | Kebutuhan |
|---|---|
| Spesifikasi | Minimal 4 vCPU, 8 GB RAM, SSD ≥ 160 GB (Technical Guideline 14.1) |
| Lokasi | Data center di Indonesia (latensi pengguna & pertimbangan UU PDP) |
| Jaringan | IPv4 publik statis, firewall bisa diatur, bandwidth cukup untuk website Fase 2 |
| Snapshot | Snapshot/backup image VPS tersedia (pelengkap, bukan pengganti backup database) |
| Naik kelas | Upgrade vCPU/RAM tanpa pindah server (Technical Guideline 14.8) |
| Pembayaran | Invoice atas nama badan usaha Balans |

**Kriteria object storage:**

| Kriteria | Kebutuhan |
|---|---|
| Kompatibilitas | API S3-compatible (dipakai WAL-G dan modul `filestore`) |
| Lokasi | Di luar VPS; boleh provider berbeda dari VPS agar kegagalan satu provider tidak menghapus backup |
| Fitur | Versioning bucket, kebijakan lifecycle, access key per bucket |
| Bucket | Minimal dua: `balans-files` (file aplikasi) dan `balans-backup` (backup DB), dengan access key terpisah |

**Yang perlu dikirim ke tim implementasi setelah akun dibuat:** akses konsol (sebagai anggota tim), IP VPS, endpoint & region object storage, access key bucket backup dan bucket file (lewat password manager).

---

## 2. Domain & DNS (F0-EXT-02)

| Record | Tipe | Nilai | Catatan |
|---|---|---|---|
| `erp.findyourbalans.com` | A | IP VPS | TTL 300 detik selama Fase 0 |
| `erp-staging.findyourbalans.com` | A | IP VPS | TTL 300 detik |
| `findyourbalans.com`, `www` | — | Tidak diubah | Tetap di hosting website saat ini sampai launch R2.2 (D-28) |
| SPF, DKIM, DMARC | TXT | Dari provider email transaksional | Disiapkan sebelum UAT Fase 1 |

Balans memberi akses DNS (atau menambahkan record di atas) dan mengonfirmasi siapa pengelola DNS saat ini.

---

## 3. Layanan Pendukung (F0-EXT-03)

| Layanan | Kebutuhan | Kapan |
|---|---|---|
| Sentry | Organisasi atas nama Balans; dua project: `balans-backend`, `balans-web-erp`; DSN dikirim ke tim | Fase 0 |
| Monitor uptime eksternal | Layanan di luar VPS dengan monitor HTTP, heartbeat/cron monitor, dan alert ke email + grup chat | Fase 0 |
| Kanal alert | Grup chat tim IT yang mendukung webhook (untuk `host-check.sh`) | Fase 0 |
| Password manager tim | Vault bersama untuk semua secret (Runbook Operasi bagian 3) | Fase 0 |
| GitHub | Organisasi/repo milik Balans, GitHub Actions, GHCR, Environment `production` dengan reviewer | Fase 0 |
| Email transaksional | Provider asli dipilih; alamat pengirim di `findyourbalans.com` | Sebelum UAT Fase 1 (sampai saat itu memakai mock/Mailpit, D-27) |

---

## 4. Vendor Vending (F0-EXT-04)

Pertanyaan untuk vendor (lead time panjang, Roadmap risiko #4):

| # | Pertanyaan | Dipakai untuk |
|---|---|---|
| 1 | Apakah mesin punya API (status stok per slot, status mesin, transaksi)? Minta dokumentasi API | VND-06, INT-03 |
| 2 | Apakah transaksi Midtrans dari mesin memuat **kode mesin** di `order_id` atau metadata? | VND-03 (U-05) |
| 3 | Apakah transaksi memuat **SKU / nomor slot** yang terjual? | Pemotongan stok otomatis per SKU |
| 4 | Siapa pemilik akun Midtrans yang dipakai mesin saat ini? | F0-EXT-05 |
| 5 | Apakah harga per slot bisa diatur jarak jauh atau hanya di mesin? | Price list vending |
| 6 | Bagaimana mesin melaporkan error/offline? | Monitoring mesin |
| 7 | Berapa jumlah mesin dan lokasinya saat ini serta rencana penambahan? | NFR-10 (≥ 50 mesin) |

Hasil: kontak vendor (nama, email, telepon) dan jawaban tertulis, dibahas di persiapan R2.3.

---

## 5. Akun Midtrans Vending (F0-EXT-05)

| Kondisi | Tindakan |
|---|---|
| Akun Midtrans vending atas nama Balans | Catat merchant ID; akses dashboard untuk Finance & Accounting |
| Akun atas nama vendor | Sepakati rencana pemindahan ke akun Balans sebelum R2.3 (Roadmap risiko #5), karena dana settlement harus masuk langsung ke rekening Balans |
| Belum ada akun | Balans membuat satu akun Midtrans atas nama Balans untuk website, link bayar WA/IG, dan vending (U-17); sandbox dibutuhkan sebelum UAT R2.2 |

---

## 6. Status

| ID | Kebutuhan | PIC Balans | Status | Tanggal |
|---|---|---|---|---|
| F0-EXT-01 | Provider VPS & object storage dipilih, akun dibuat | | Belum | |
| F0-EXT-02 | Akses DNS; record `erp` & `erp-staging` | | Belum | |
| F0-EXT-03 | Sentry, monitor uptime, kanal alert, password manager, GitHub | | Belum | |
| F0-EXT-04 | Kontak vendor vending + jawaban pertanyaan bagian 4 | | Belum | |
| F0-EXT-05 | Pemilik akun Midtrans vending dikonfirmasi | | Belum | |
