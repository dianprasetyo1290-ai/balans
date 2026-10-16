# Status Fase 0
## Balans Nutrition — Unified Business Platform: Discovery & Fondasi

| Item | Keterangan |
|---|---|
| Dokumen | Pelacak kebutuhan dan exit criteria PRD Fase 0 |
| Acuan | PRD Fase 0 v0.1 bagian 6, 9, 10 |
| Diperbarui | 14 Oktober 2026 |

**Status:** **Selesai** = hasil sudah ada dan terverifikasi oleh tim implementasi. **Draf — review** = artefak lengkap, menunggu review/persetujuan pihak yang disebut. **Menunggu Balans** = tidak bisa diselesaikan tanpa keputusan, data, atau akun dari Balans. **Menunggu VPS** = sudah dibangun dan digladikan lokal, tinggal dijalankan setelah VPS dan DNS tersedia.

---

## 1. Persetujuan Baseline (APR)

| ID | Status | Bukti / artefak | Yang dibutuhkan |
|---|---|---|---|
| F0-APR-01 | Menunggu Balans | Bahan Workshop Persetujuan BRD; Lembar Persetujuan Baseline (sheet Usulan & Konfirmasi) | Jadwal workshop per divisi dan key user |
| F0-APR-02 | Menunggu Balans | Lembar Persetujuan (sheet Opsional) | Keputusan Direksi *Diambil?* U-28 s.d. U-32, ACC-23 |
| F0-APR-03 | Menunggu Balans | Lembar Persetujuan (sheet Parameter) | Konfirmasi 9 parameter |
| F0-APR-04 | Menunggu Balans | Langkah menuju BRD v1.0 (Bahan Workshop bagian 4) | Hasil APR-01 s.d. APR-03, tanda tangan bagian 15 |
| F0-APR-05 | Draf — review | Backlog Fase 1 bagian kalibrasi (97 story, 593 SP) | Komposisi tim final dan persetujuan steering committee |

## 2. Desain Solusi (DSN)

| ID | Status | Artefak |
|---|---|---|
| F0-DSN-01 | Draf — review tim & F&A | Desain Data Model Fase 1 (18 modul, kepemilikan tabel tunggal, ERD per modul) |
| F0-DSN-02 | Draf — review key user | Desain Status Dokumen Fase 1 (20 dokumen, label Naming Convention bagian 7) |
| F0-DSN-03 | Draf — review | Desain Role & Approval bagian 1–3 (15 role, format permission, matriks role × resource, MFA & re-auth) |
| F0-DSN-04 | Menunggu Balans | Desain Role & Approval bagian 4: matriks per jabatan siap; nama approver & pengganti diisi Balans, disetujui Direksi |
| F0-DSN-05 | Draf — review F&A & Sales | Desain Penomoran & Dokumen Cetak; butuh logo, info rekening, teks NON-PKP |
| F0-DSN-06 | Draf — review | Backlog Fase 1 (105 requirement → 97 story dengan kriteria penerimaan & estimasi) |
| F0-DSN-07 | Selesai | `backend/api/paths/identity.yaml`, `catalog.yaml`, `warehouse.yaml`; format error & pagination standar; lolos Redocly lint; tipe TS tergenerate |
| F0-DSN-08 | Draf — review | Desain Alur Lintas Modul Fase 1 (4 sequence diagram) |

## 3. Finance & Accounting (FIN)

| ID | Status | Artefak / yang dibutuhkan |
|---|---|---|
| F0-FIN-01 | Menunggu Balans | COA bernomor di Desain Finance & Accounting bagian 1; butuh COA lama (jika ada) dan persetujuan Head F&A |
| F0-FIN-02 | Draf — review Head F&A | Mapping semua rule A-xx Fase 1 ke akun & dimensi; rule fase berikutnya & opsional ditandai |
| F0-FIN-03 | Draf — review | Daftar channel, sub-channel, divisi, aturan kode kampanye |
| F0-FIN-04 | Menunggu Balans | Formulir profil pajak & jenis biaya awal; butuh data badan usaha/NPWP/PPh Final dan konfirmasi konsultan pajak |
| F0-FIN-05 | Menunggu Balans | Daftar 8 pertanyaan review untuk akuntan/konsultan pajak |

## 4. UI/UX (UX)

| ID | Status | Artefak |
|---|---|---|
| F0-UX-01 | Selesai (spesifikasi + kode) | Desain UI/UX ERP bagian 1; preset shadcn Balans terpasang; 11 komponen bersama di `web-erp/src/components/common` dengan test |
| F0-UX-02 | Selesai (kode), review label | Struktur menu Naming Convention bagian 5 di `menu.ts`; menu fase berikutnya & opsional tersembunyi |
| F0-UX-03 | Draf — review key user | Wireframe pola dasar (6) dan 12 layar kunci Fase 1 |
| F0-UX-04 | Draf — review | Pola form HP Stock Count |
| F0-UX-05 | Menunggu Balans | Rencana tree test; butuh perwakilan 5 divisi |

## 5. Fondasi Teknis (TEK)

| ID | Status | Bukti |
|---|---|---|
| F0-TEK-01 | Selesai, proteksi branch menunggu repo GitHub | Monorepo `backend/`, `web-erp/`, `web-store/`, `deploy/`, `docs/`, `.github/workflows/`; template PR Lampiran C; `scripts/protect-main.sh` |
| F0-TEK-02 | Selesai | Binary `api`, `worker`, `migrate` jalan; `cutover`, `storefront` kerangka; package `platform` (`apperr`, `clock`, `config`, `db`+TxManager, `httpx`, `idempotency`, `jobqueue`, `logger`, `money`, `retry`, `webhook`) teruji; oapi-codegen (spec dibundel), sqlc, goose, river; `/healthz`, `/readyz` |
| F0-TEK-03 | Selesai | depguard dari `tools/lintconfig` (batas subpackage, arah tier, larangan modul → adapter/wiring); terbukti menolak pelanggaran |
| F0-TEK-04 | Selesai | Vite dari preset shadcn Balans; TanStack Router file-based & Query; RHF + Zod (pesan Indonesia); Zustand; `openapi-fetch`; layout `_app` dengan sidebar; 403/404; `lib/format`; `error-messages.ts`; `status-labels.ts` |
| F0-TEK-05 | Selesai | `ENABLED_MODULES` mengatur route API, worker/jadwal, dan menu; nama modul divalidasi |
| F0-TEK-06 | Selesai lokal, menunggu VPS | Halaman Tentang Sistem (versi + `/readyz`) lolos E2E desktop & HP; tergladi lewat Caddy edge |
| F0-TEK-07 | Menunggu VPS | Compose staging/production terpisah, Caddy HTTPS + header keamanan + basic auth staging + `noindex`, batas memori TG 14.1, healthcheck, log rotation; tergladi lokal |
| F0-TEK-08 | Menunggu VPS & GitHub | `ci.yml`, `deploy-staging.yml`, `deploy-production.yml`, `build-postgres.yml` (lolos actionlint); retag tanpa build ulang; `deploy.sh` dengan rollback otomatis — rilis, rollback otomatis, dan rollback manual sudah dicoba di gladi lokal |
| F0-TEK-09 | Menunggu VPS & object storage | WAL-G (full harian + WAL kontinu), retensi 14 harian + 12 bulanan, heartbeat; `restore-test.sh` — restore point-in-time terbukti di gladi lokal (±10 detik); runbook pemulihan ≤ 2 jam |
| F0-TEK-10 | Selesai (kode), menunggu akun | `slog` JSON + `request_id`; Sentry backend & ERP dengan scrubbing; `host-check.sh` (disk > 80%, memori, container); daftar monitor & uji alert di Runbook |
| F0-TEK-11 | Selesai | Runbook bagian 3: tempat secret, prosedur buat & rotasi kunci JWT Ed25519 dan AES-256-GCM |
| F0-TEK-12 | Selesai | `go test` + testcontainers Postgres (17 package), Vitest (37 test), Playwright smoke; gate coverage 80% (`scripts/coverage-gate.sh`) |
| F0-TEK-13 | Selesai | README root & web-erp; `deploy/compose.local.yaml` (Postgres, Mailpit); `scripts/generate.sh` |
| F0-TEK-14 | Selesai | Interface `notification.EmailSender`, `logistics.ShipmentProvider`, `sales.PaymentGateway`; mock dipilih via env; Mailpit; `/_mock/payments`, `/_mock/shipments` mengirim webhook bersignature ke endpoint asli; production menolak start jika modul aktif memakai mock |

## 6. Persiapan Migrasi (MIG)

| ID | Status | Artefak |
|---|---|---|
| F0-MIG-01 | Selesai — siap diserahkan | `docs/migrasi/Template-Migrasi-Master-Data.xlsx` (9 sheet) |
| F0-MIG-02 | Selesai — siap diserahkan | `docs/migrasi/Template-Migrasi-Harga.xlsx` (4 sheet) |
| F0-MIG-03 | Selesai — siap diserahkan | `docs/migrasi/Template-Migrasi-Saldo-Awal.xlsx` (6 sheet) |
| F0-MIG-04 | Selesai | Sheet Petunjuk per workbook (kolom, wajib, format, validasi, contoh) + Panduan Migrasi bagian 1 |
| F0-MIG-05 | Menunggu Balans | Urutan & ketergantungan di Panduan Migrasi bagian 2; nama PIC diisi Balans |
| F0-MIG-06 | Draf — review | Rencana cut-over & pengecekan rekonsiliasi (Panduan Migrasi bagian 3–4) |

## 7. Pihak Ketiga (EXT)

| ID | Status | Artefak |
|---|---|---|
| F0-EXT-01 – F0-EXT-05 | Menunggu Balans | Checklist Pihak Ketiga Fase 0 (kriteria provider, record DNS, layanan pendukung, pertanyaan vendor vending, skenario akun Midtrans) |

---

## 8. Exit Criteria (PRD bagian 10)

| # | Kriteria | Status | Sisa |
|---|---|---|---|
| 1 | BRD v1.0 dan desain solusi disetujui | Desain siap direview | Workshop, review, tanda tangan |
| 2 | Pilihan opsional ditentukan | Lembar siap | Keputusan Direksi |
| 3 | Data Fase 1 dari BRD 12.3 diterima/dijadwalkan | Lembar siap (sheet Data Balans) | Penyerahan/penjadwalan data |
| 4 | Template migrasi diserahkan | Template & panduan siap | Serah terima + penetapan PIC |
| 5 | Staging & production siap | Tergladi lokal | VPS, DNS, GitHub; deploy pertama; rollback dicoba di VPS |
| 6 | Backup teruji | Tergladi lokal | Object storage; uji restore di VPS dicatat |
| 7 | Monitoring aktif | Kode & skrip siap | Akun Sentry & monitor uptime; uji alert |
| 8 | Backlog Fase 1 siap | Draf siap | Review & kalibrasi tim |

## 9. Isu Terbuka Tambahan

| # | Isu | Usulan | PIC |
|---|---|---|---|
| 1 | Label status periode akuntansi (*Terbuka*) belum ada di Naming Convention bagian 7 | Tambahkan lewat uji label (F0-UX-05) | UI/UX |
| 2 | Kode dokumen SO, IN, OUT, PU, MUS, BYR belum ada di U-15 | Dikonfirmasi saat review penomoran (F0-DSN-05) | F&A, PM |
