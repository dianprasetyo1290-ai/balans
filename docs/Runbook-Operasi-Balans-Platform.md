# Runbook Operasi
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Runbook operasi infrastruktur, rilis, backup, dan monitoring |
| Acuan | PRD Fase 0 (F0-TEK-07 s.d. F0-TEK-11); Technical Guideline v0.1 bagian 9.4, 13, 14 |
| Pemakai | Tech lead dan developer yang memegang akses VPS |

---

## 1. Topologi

Satu VPS (4 core, 8 GB, lokasi Indonesia) menjalankan tiga project Docker Compose yang terhubung lewat jaringan `balans-edge`:

| Project | File | Isi |
|---|---|---|
| `balans-edge` | `deploy/compose.edge.yaml` | Caddy: HTTPS, header keamanan, basic auth staging, proxy ke container |
| `balans-production` | `deploy/compose.yaml` + `compose.production.yaml` | `postgres` (WAL-G), `api`, `worker`, `web`, `backup` |
| `balans-staging` | `deploy/compose.yaml` + `compose.staging.yaml` | `postgres`, `api`, `worker`, `web`, `mailpit` |

| Host | Tujuan |
|---|---|
| `erp.findyourbalans.com` | production |
| `erp-staging.findyourbalans.com` | staging, dilindungi basic auth dan `X-Robots-Tag: noindex`; `/healthz`, `/readyz`, `/api/v1/webhooks/*` tanpa basic auth |

Struktur di VPS:

```
/opt/balans/
├── deploy/                 # disalin workflow dari repository (rsync)
├── production.env          # secret production (chmod 600)
├── staging.env             # secret staging (chmod 600)
├── edge.env                # ACME email, host, basic auth staging (chmod 600)
├── monitoring.env          # ALERT_WEBHOOK_URL, HOST_HEARTBEAT_URL (chmod 600)
├── production.deploy       # REGISTRY, POSTGRES_IMAGE_TAG, ENV_FILE, SMOKE_URL, IMAGE_TAG
├── staging.deploy
└── restore-reports.log     # hasil uji restore bulanan
```

---

## 2. Setup VPS Pertama Kali

Prasyarat dari Balans (PRD 6.7): akun VPS dan object storage atas nama Balans (F0-EXT-01), record DNS `erp` dan `erp-staging` mengarah ke IP VPS (F0-EXT-02).

1. Pasang Docker Engine + Compose plugin, `rsync`, `curl`. Aktifkan firewall: hanya 22, 80, 443. Login SSH hanya dengan kunci.
2. Buat user deploy (mis. `deploy`) anggota grup `docker`; daftarkan kunci publik GitHub Actions di `~deploy/.ssh/authorized_keys`.
3. `sudo mkdir -p /opt/balans && sudo chown deploy: /opt/balans`.
4. `docker network create balans-edge`.
5. Salin `deploy/` dari repository ke `/opt/balans/deploy`.
6. Buat file env dari `deploy/env.example` dan `deploy/edge.env.example`, isi secret (bagian 3), lalu `chmod 600 /opt/balans/*.env`.
7. Buat `production.deploy` dan `staging.deploy`:

   ```
   REGISTRY=ghcr.io/<org>
   POSTGRES_IMAGE_TAG=<tag dari workflow build-postgres>
   ENV_FILE=/opt/balans/production.env
   SMOKE_URL=https://erp.findyourbalans.com
   ```

8. Login ke GHCR dengan token baca paket: `docker login ghcr.io`.
9. Jalankan edge: `docker compose -p balans-edge --env-file /opt/balans/edge.env -f /opt/balans/deploy/compose.edge.yaml up -d` (variabel `ENV_FILE` di file env edge menunjuk ke file itu sendiri).
10. Jalankan workflow *deploy-staging* (atau `deploy.sh staging <tag>`), lalu rilis tag pertama untuk production.
11. Pasang cron host-check (bagian 7.3) dan monitor eksternal (bagian 7.1).

---

## 3. Pengelolaan Secret (F0-TEK-11)

Secret hanya disimpan di tiga tempat (Technical Guideline 9.4):

| Tempat | Isi |
|---|---|
| GitHub Actions Secrets | `VPS_SSH_KEY`, `VPS_KNOWN_HOSTS`, `VPS_HOST`, `VPS_USER`, `STAGING_BASIC_AUTH` (`user:password` untuk E2E) |
| File env VPS (`chmod 600`, milik user deploy) | Password DB, kredensial object storage, kunci aplikasi, server key provider |
| Password manager tim | Salinan seluruh secret di atas + kredensial akun provider (VPS, object storage, Sentry, monitor uptime, DNS) |

Aturan:

- Secret DILARANG di-commit, termasuk di `env.example`. Repository hanya berisi placeholder.
- Kredensial backup memakai bucket dan access key terpisah dari bucket file aplikasi; key backup hanya punya akses ke bucket backup.
- Setiap orang yang keluar dari tim: rotasi semua secret yang pernah ia akses, cabut kunci SSH-nya.

### 3.1 Kunci JWT Ed25519 (dipakai mulai R1.1)

Access token ditandatangani EdDSA (Ed25519) dengan header `kid` agar kunci bisa dirotasi (Technical Guideline 9.1).

**Membuat kunci:**

```bash
kid="jwt-$(date -u +%Y%m%d)"
openssl genpkey -algorithm ed25519 -out "$kid.pem"
openssl pkey -in "$kid.pem" -pubout -out "$kid.pub.pem"
# Simpan ke env sebagai base64 satu baris
echo "JWT_SIGNING_KEY_ID=$kid"
echo "JWT_SIGNING_KEY=$(base64 -w0 "$kid.pem")"
echo "JWT_VERIFY_KEYS=$kid:$(base64 -w0 "$kid.pub.pem")"
shred -u "$kid.pem" "$kid.pub.pem"
```

Kunci production dan staging berbeda. Kunci privat hanya ada di env `api` dan `api-storefront`.

**Rotasi (tanpa logout massal):**

1. Buat kunci baru (`kid` baru).
2. Tambahkan kunci publik baru ke `JWT_VERIFY_KEYS` (dipisah koma), deploy. Token lama tetap valid karena kunci lama masih ada di daftar verifikasi.
3. Ganti `JWT_SIGNING_KEY_ID` dan `JWT_SIGNING_KEY` ke kunci baru, deploy.
4. Setelah umur maksimum access token (15 menit) lewat, hapus kunci lama dari `JWT_VERIFY_KEYS`, deploy.
5. Jika kunci bocor: langkah 2–4 sekaligus, lalu naikkan `token_version` semua user agar semua sesi dicabut.

### 3.2 Kunci enkripsi data pribadi AES-256-GCM (dipakai mulai R1.1)

NIK, NPWP, rekening, dan komponen gaji dienkripsi di level aplikasi; `key_id` disimpan bersama ciphertext (Technical Guideline 6.7).

**Membuat kunci:**

```bash
kid="pii-$(date -u +%Y%m%d)"
echo "PII_ENCRYPTION_KEY_ID=$kid"
echo "PII_ENCRYPTION_KEYS=$kid:$(openssl rand -base64 32)"
echo "PII_HASH_KEY=$(openssl rand -base64 32)"   # HMAC untuk kolom pencarian persis
```

**Rotasi:**

1. Tambahkan kunci baru ke `PII_ENCRYPTION_KEYS` (format `kid:base64`, dipisah koma) dan ganti `PII_ENCRYPTION_KEY_ID` ke kunci baru. Data baru memakai kunci baru; data lama tetap terbaca dengan kunci lama.
2. Jalankan job re-enkripsi (dibangun bersama modul HRIS) sampai tidak ada ciphertext dengan `key_id` lama.
3. Hapus kunci lama dari `PII_ENCRYPTION_KEYS`.

`PII_HASH_KEY` tidak dirotasi tanpa migrasi ulang kolom hash. Kehilangan kunci enkripsi berarti data terenkripsi tidak bisa dibaca: kunci WAJIB ada di password manager sebelum dipakai di production.

---

## 4. Rilis dan Rollback (F0-TEK-08)

| Langkah | Staging | Production |
|---|---|---|
| Pemicu | Merge ke `main` | Push tag `vX.Y.Z` |
| Image | Build baru, tag 12 karakter SHA | Retag image SHA yang sama dengan staging (tanpa build ulang) |
| Persetujuan | — | Approval manual di GitHub Environment `production` |
| Backup sebelum migrasi | — | `backup-now` |
| Migrasi | `migrate up` sebelum container diganti | sama |
| Verifikasi | Healthcheck container + `/readyz` + E2E Playwright | Healthcheck + `/readyz` |
| Gagal | Rollback otomatis ke tag sebelumnya | sama |

Semua langkah di VPS dijalankan `deploy/scripts/deploy.sh <lingkungan> <tag>`. Jika migrasi gagal, container lama tetap berjalan dan tag tidak berubah.

**Rollback manual production:** jalankan workflow *deploy-production* lewat *Run workflow* dengan input versi sebelumnya, atau di VPS: `bash /opt/balans/deploy/scripts/deploy.sh production v1.0.0`. Skema database tidak di-rollback (pola expand → contract, Technical Guideline 14.4).

**Jika data rusak:** restore point-in-time ke DB baru (bagian 5.3), verifikasi, lalu alihkan.

---

## 5. Backup dan Restore (F0-TEK-09, NFR-07)

### 5.1 Mekanisme

| Data | Mekanisme | Retensi |
|---|---|---|
| Postgres | WAL-G: WAL kontinu (`archive_timeout=60` detik) + full backup harian 01.00 WIB oleh service `backup` | 14 harian; backup tanggal 1 ditandai permanen dan disimpan 12 bulan |
| File aplikasi | Versioning bucket + replikasi ke bucket/provider lain (diatur di provider object storage) | Mengikuti retensi dokumen |
| Konfigurasi | `deploy/` di repository; secret di password manager | — |

Setiap backup harian mengirim heartbeat ke `BACKUP_HEARTBEAT_URL` (`/start`, sukses, atau `/fail`). Monitor heartbeat mengirim alert jika backup gagal atau tidak berjalan lebih dari 26 jam.

Backup manual (mis. sebelum perubahan besar):

```bash
docker compose -p balans-production --env-file /opt/balans/production.deploy \
  -f /opt/balans/deploy/compose.yaml -f /opt/balans/deploy/compose.production.yaml \
  run --rm --no-deps backup backup-now
```

### 5.2 Uji restore bulanan

```bash
export RESTORE_IMAGE=ghcr.io/<org>/balans-postgres:<POSTGRES_IMAGE_TAG>
bash /opt/balans/deploy/scripts/restore-test.sh                              # backup terbaru + seluruh WAL
bash /opt/balans/deploy/scripts/restore-test.sh "2026-11-01 10:00:00+07"     # point-in-time
```

Skrip mengambil backup dari object storage ke container dan volume sementara, menjalankan recovery, memeriksa versi skema dan jumlah tabel, mencatat durasi ke `/opt/balans/restore-reports.log`, lalu menghapus container sementara. `RESTORE_VERIFY_SQL` dapat diisi query pemeriksaan isi data.

Format laporan untuk exit criteria Fase 0 butir 6:

| Tanggal | Target | Durasi | Versi skema | Hasil | Pelaksana |
|---|---|---|---|---|---|
| | | | | | |

Mekanisme ini sudah digladikan di lingkungan lokal yang meniru VPS (registry lokal, object storage S3-compatible, Caddy edge, stack production dan staging): restore point-in-time hanya memuat data sebelum titik target, restore terbaru memuat seluruh data, durasi ±10 detik untuk database kosong. Uji di VPS production tetap wajib dijalankan dan dicatat.

### 5.3 Pemulihan ke VPS baru (target ≤ 2 jam)

Dipakai jika VPS rusak atau hilang (Technical Guideline 14.8).

| # | Langkah | Perkiraan |
|---|---|---|
| 1 | Buat VPS baru di provider; catat IP | 10 menit |
| 2 | Setup VPS (bagian 2 langkah 1–9) dengan env dari password manager | 30 menit |
| 3 | Ubah record DNS `erp` dan `erp-staging` ke IP baru (TTL record disarankan 300 detik) | 5 menit + propagasi |
| 4 | Restore database production dari backup terbaru: jalankan langkah *backup-fetch* pada `restore-test.sh` ke volume `balans-production_postgres-data` (tanpa menghapusnya), lalu start project production | 15–30 menit |
| 5 | Deploy tag production terakhir: `deploy.sh production <versi>` | 5 menit |
| 6 | Verifikasi: `/readyz`, halaman Tentang Sistem, jumlah data kunci, transaksi terakhir yang tercatat | 10 menit |
| 7 | Aktifkan kembali cron host-check dan pastikan heartbeat backup berjalan | 5 menit |

Staging tidak dipulihkan dari backup; cukup deploy ulang dengan database kosong.

---

## 6. Migrasi Database

- Migrasi berjalan otomatis di `deploy.sh` sebelum container diganti.
- `migrate down` hanya untuk lokal; di staging/production perbaikan selalu lewat migrasi baru.
- Status: `docker compose ... run --rm migrate migrate status`.

---

## 7. Monitoring dan Alert (F0-TEK-10)

### 7.1 Monitor eksternal

Dibuat di layanan monitor uptime di luar VPS (F0-EXT-03):

| Monitor | Target | Interval | Alert |
|---|---|---|---|
| ERP production | `https://erp.findyourbalans.com/` | 1 menit | down > 2 menit |
| API production | `https://erp.findyourbalans.com/readyz` berisi `"status":"ready"` | 1 menit | down > 2 menit |
| API staging | `https://erp-staging.findyourbalans.com/readyz` | 5 menit | down > 15 menit |
| Heartbeat backup | `BACKUP_HEARTBEAT_URL` | harian | tidak ada ping 26 jam atau `/fail` |
| Heartbeat host | `HOST_HEARTBEAT_URL` | 5 menit | tidak ada ping 15 menit |
| Sertifikat TLS | kedua host | harian | kedaluwarsa < 14 hari |

Tujuan alert: email dan grup chat tim IT.

### 7.2 Log dan error tracking

- Log aplikasi: JSON `slog` ke stdout dengan `request_id`, `user_id`, `module`; dirotasi Docker (`10m` × 5 file). Lihat: `docker logs balans-production-api-1 --since 1h`.
- Cari satu request: `docker logs balans-production-api-1 2>&1 | grep <request-id>`. Request ID yang sama tampil di dialog error ERP untuk error 5xx.
- Sentry backend (`SENTRY_DSN` di env) dan ERP (`WEB_ERP_SENTRY_DSN` di GitHub Variables). Header, cookie, query string, body, dan identitas user selain ID dibuang sebelum dikirim.

### 7.3 Pemeriksaan host

Cron di VPS (user deploy):

```
*/5 * * * * /opt/balans/deploy/monitoring/host-check.sh >> /opt/balans/host-check.log 2>&1
```

Alert ke `ALERT_WEBHOOK_URL` jika disk > 80%, memori tersedia < 10%, container `unhealthy`, atau container restart berulang.

### 7.4 Uji alert (exit criteria Fase 0 butir 7)

| Alert | Cara memicu | Hasil yang diharapkan |
|---|---|---|
| Aplikasi down | `docker stop balans-staging-api-1` selama 3 menit, lalu start | Monitor staging mengirim alert lalu pulih |
| Backup gagal | Panggil `curl $BACKUP_HEARTBEAT_URL/fail` | Alert backup gagal |
| Disk > 80% | Jalankan host-check dengan `DISK_ALERT_PERCENT=1` | Pesan alert di grup tim IT |
| Sentry backend | Endpoint uji di staging atau error buatan di lokal dengan DSN staging | Event tampil di Sentry tanpa data pribadi |
| Sentry ERP | Panggil `throw` dari konsol browser di staging | Event tampil di Sentry |

Catat tanggal dan hasil setiap uji di tabel exit criteria.
