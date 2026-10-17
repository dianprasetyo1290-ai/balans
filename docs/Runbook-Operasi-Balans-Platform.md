# Runbook Operasi
## Balans Nutrition — Unified Business Platform

| Item | Keterangan |
|---|---|
| Dokumen | Runbook operasi infrastruktur, rilis, backup, dan monitoring |
| Acuan | PRD Fase 0 (F0-TEK-07 s.d. F0-TEK-11); PRD Fase 1 bagian 9 & 11.2, Backlog S-419; Technical Guideline v0.1 bagian 9.4, 13, 14 |
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
├── monitoring.env          # webhook alert & URL heartbeat (contoh: deploy/monitoring/monitoring.env.example, chmod 600)
├── production.deploy       # REGISTRY, POSTGRES_IMAGE_TAG, ENV_FILE, SMOKE_URL, IMAGE_TAG
├── staging.deploy
├── restore-reports.log     # hasil uji restore bulanan (satu baris per uji)
├── host-check.log          # log cron host-check
├── worker-check.log        # log cron worker-check
└── loadtest-results/       # hasil uji beban (bagian 8.3)
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
11. Pasang monitor eksternal dan heartbeat (bagian 7.1), isi `/opt/balans/monitoring.env` dari `deploy/monitoring/monitoring.env.example`, lalu pasang cron host-check & worker-check (bagian 7.3) dan timer uji restore bulanan (bagian 5.2).
12. Pastikan swap VPS aktif minimal 2 GB sebagai cadangan OS; container tidak memakai swap (`memswap_limit` = `mem_limit`, bagian 8.1).

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
| File aplikasi | Versioning bucket + replikasi ke bucket/provider lain (diatur di provider object storage) setelah adapter object storage dibangun. Selama file modul `filestore` (PDF PKS, foto QC, berita acara) masih di volume `filestore-data` container api (`FILESTORE_DIR=/var/lib/balans/files`), service `backup` memasang volume itu read-only dan `backup-files` mengunggah arsip tar harian ke `<WALG_S3_PREFIX>/filestore/daily/` (dan `filestore/monthly/` setiap tanggal 1) dengan kompresi & kredensial WAL-G | 14 arsip harian; arsip tanggal 1 disimpan 12 bulan |
| Konfigurasi | `deploy/` di repository; secret di password manager | — |

Setiap backup harian (database lalu file) mengirim heartbeat ke `BACKUP_HEARTBEAT_URL` (`/start`, sukses, atau `/fail`); sukses hanya jika keduanya berhasil. Monitor heartbeat mengirim alert jika backup gagal atau tidak berjalan lebih dari 26 jam.

Backup manual (mis. sebelum perubahan besar):

```bash
docker compose -p balans-production --env-file /opt/balans/production.deploy \
  -f /opt/balans/deploy/compose.yaml -f /opt/balans/deploy/compose.production.yaml \
  run --rm --no-deps backup backup-now
# arsip file filestore saja:
docker compose ... run --rm --no-deps backup backup-files
```

### 5.2 Uji restore bulanan

`deploy/scripts/restore-test.sh` dijalankan otomatis oleh systemd timer **tanggal 2 setiap bulan pukul 03.00 WIB** (sehari setelah backup bulanan permanen). Pemasangan sekali di VPS:

```bash
sudo cp /opt/balans/deploy/systemd/balans-restore-test.* /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now balans-restore-test.timer
systemctl list-timers balans-restore-test.timer          # jadwal berikutnya
sudo systemctl start balans-restore-test.service         # jalankan sekarang (mis. sebelum go-live)
journalctl -u balans-restore-test --since today          # log uji
```

Jalankan manual (point-in-time atau backup tertentu):

```bash
bash /opt/balans/deploy/scripts/restore-test.sh                                  # backup terbaru + seluruh WAL
bash /opt/balans/deploy/scripts/restore-test.sh "2026-11-01 10:00:00+07"         # point-in-time
RESTORE_BACKUP_NAME=base_0000000100000000000000A1 bash .../restore-test.sh "..."  # target sebelum backup terbaru
```

Image dibaca dari `/opt/balans/production.deploy` (`REGISTRY`, `POSTGRES_IMAGE_TAG`, `IMAGE_TAG`); bisa ditimpa `RESTORE_IMAGE` / `RESTORE_APP_IMAGE`. Variabel lain ada di kepala skrip.

| Langkah | Isi | Gagal jika |
|---|---|---|
| 1. Restore | `wal-g backup-fetch` ke volume sementara, recovery di container `balans-restore-test` (memori 768 MB, 1 CPU, `shared_buffers=128MB`) | Backup tidak ada/rusak, recovery tidak selesai dalam 1 jam |
| 2. Skema | `migrate status` dengan image aplikasi yang sedang dirilis | Versi skema hasil restore < migrasi terbaru di image (kecuali uji point-in-time) |
| 3. Isi data | Jumlah baris tabel kunci (`users`, `audit_logs`, `journal_entries`, `journal_lines`, `products`, `stock_movements`, `purchase_orders`, `file_objects`, …); setiap jurnal posted debit = kredit = total; saldo `stock_balances` = jumlah `stock_movements` per produk × lokasi × batch × status; waktu audit log terakhir (RPO nyata) | Ada jurnal tidak balance atau saldo stok berbeda dari ledger |
| 4. File | Arsip `filestore/daily` terbaru diambil; setiap `file_objects` berstatus terunggah sebelum arsip dibuat harus ada di arsip | Ada file hilang atau arsip tidak bisa diambil |
| 5. Catat | Satu baris di `/opt/balans/restore-reports.log`, heartbeat `RESTORE_HEARTBEAT_URL` (sukses / `/fail` dengan isi baris laporan), alert `ALERT_WEBHOOK_URL` jika gagal; container, volume, dan jaringan sementara dihapus | — |

Contoh baris laporan (gladi lokal 17 Oktober 2026: stack production lokal + object storage S3-compatible, setelah uji beban bagian 8.4):

```
2026-10-17T05:50:59Z restore OK durasi=8s target=latest backup=base_00000001000000000000000F skema=20261027000100/20261027000100 tabel=95 jurnal_tidak_balance=0 stok_selisih=0 file=1/1@20261017T055010Z data_terakhir=2026-10-17T12:48WIB baris=users:18,audit_logs:2025,accounts:86,journal_entries:861,journal_lines:1722,products:5,...,stock_movements:10,stock_balances:7,inbounds:1,file_objects:2,approval_requests:1
```

Jalur gagal juga digladikan dan tercatat `restore GAGAL` + alert + heartbeat `/fail`: recovery ditolak (parameter lebih kecil dari primary), arsip file tidak ada padahal ada file terunggah, dan file terunggah yang hilang dari arsip.

Rekap untuk steering committee (go/no-go butir 4, PRD Fase 1 bagian 11.2) disalin dari log ke tabel berikut setiap bulan:

| Tanggal | Target | Backup | Durasi | Versi skema | Jurnal tidak balance | Selisih stok | File | Hasil | Pelaksana |
|---|---|---|---|---|---|---|---|---|---|
| | | | | | | | | | |

`RESTORE_VERIFY_SQL` dapat diisi query tambahan (hasilnya dicetak, bukan penentu lulus). Restore memakai sumber daya VPS production: jika uji manual dijalankan di jam kerja, hentikan staging lebih dulu (bagian 8.1).

### 5.3 Pemulihan ke VPS baru (target ≤ 2 jam)

Dipakai jika VPS rusak atau hilang (Technical Guideline 14.8).

| # | Langkah | Perkiraan |
|---|---|---|
| 1 | Buat VPS baru di provider; catat IP | 10 menit |
| 2 | Setup VPS (bagian 2 langkah 1–9) dengan env dari password manager | 30 menit |
| 3 | Ubah record DNS `erp` dan `erp-staging` ke IP baru (TTL record disarankan 300 detik) | 5 menit + propagasi |
| 4 | Restore database production dari backup terbaru: jalankan langkah *backup-fetch* pada `restore-test.sh` ke volume `balans-production_postgres-data` (tanpa menghapusnya), lalu start project production. Pulihkan file: `docker run --rm --env-file /opt/balans/production.env -v balans-production_filestore-data:/files <image balans-postgres> bash -c 'n=$(wal-g st ls filestore/daily/ \| awk "\$1==\"obj\"{print \$NF}" \| sort \| tail -1); wal-g st get filestore/daily/$n /tmp/f.tar && tar -C /files -xf /tmp/f.tar && chown -R 65532:65532 /files'` | 15–30 menit |
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

Dibuat di layanan monitor uptime di luar VPS (F0-EXT-03; mis. Better Stack atau UptimeRobot untuk HTTP, healthchecks.io untuk heartbeat). Health endpoint: `/healthz` (proses hidup) dan `/readyz` (DB terhubung dan skema terbaru, `{"status":"ready"}`); keduanya tanpa basic auth di staging.

| Monitor | Target | Interval | Alert | URL di `monitoring.env` / env |
|---|---|---|---|---|
| ERP production | `https://erp.findyourbalans.com/` | 1 menit | down > 2 menit | — |
| API production | `https://erp.findyourbalans.com/readyz` berisi `"status":"ready"` | 1 menit | down > 2 menit | — |
| API staging | `https://erp-staging.findyourbalans.com/readyz` | 5 menit | down > 15 menit | — |
| Heartbeat backup | ping dari service `backup` setiap hari | periode 1 hari, grace 2 jam | tidak ada ping 26 jam atau `/fail` | `BACKUP_HEARTBEAT_URL` (production.env) |
| Heartbeat host | ping dari cron host-check | periode 5 menit, grace 10 menit | tidak ada ping 15 menit | `HOST_HEARTBEAT_URL` |
| Heartbeat worker production | ping dari cron worker-check | periode 5 menit, grace 10 menit | `/fail` atau tidak ada ping 15 menit | `WORKER_HEARTBEAT_URL` |
| Heartbeat worker staging (opsional) | idem untuk staging | periode 5 menit, grace 1 jam | idem | `STAGING_WORKER_HEARTBEAT_URL` |
| Heartbeat uji restore | ping dari `restore-test.sh` | periode 31 hari, grace 3 hari | `/fail` atau tidak ada ping 34 hari | `RESTORE_HEARTBEAT_URL` |
| Sertifikat TLS | kedua host | harian | kedaluwarsa < 14 hari | — |

**Jalur alert:**

| Tingkat | Contoh | Kanal | Penanggung jawab |
|---|---|---|---|
| Kritis | ERP/API production down, heartbeat host hilang, worker production `/fail`, backup `/fail` | Email tim IT + grup chat tim IT (integrasi monitor) + panggilan/WA on-call di jam operasional | Tech lead on-call; tanggapi ≤ 30 menit jam operasional (NFR-06) |
| Tinggi | Uji restore gagal, disk > 80%, container OOM/restart berulang, job dead-letter | Grup chat (`ALERT_WEBHOOK_URL`) + email | Developer piket, hari kerja yang sama |
| Info | Staging down, sertifikat < 14 hari | Grup chat | Developer piket |

Setiap monitor di layanan uptime diarahkan ke dua integrasi: **email** (alamat grup tim IT) dan **webhook chat** yang sama dengan `ALERT_WEBHOOK_URL`. Skrip di VPS mengirim alert langsung ke webhook chat; heartbeat menjadi jalur cadangan bila VPS sendiri yang mati. Uptime bulanan (laporan monitor) dilampirkan di laporan hypercare (NFR-06).

### 7.2 Log dan error tracking

- Log aplikasi: JSON `slog` ke stdout dengan `request_id`, `user_id`, `module`; dirotasi Docker (`10m` × 5 file). Lihat: `docker logs balans-production-api-1 --since 1h`.
- Cari satu request: `docker logs balans-production-api-1 2>&1 | grep <request-id>`. Request ID yang sama tampil di dialog error ERP untuk error 5xx.
- Sentry backend (`SENTRY_DSN` di env) dan ERP (`WEB_ERP_SENTRY_DSN` di GitHub Variables). Header, cookie, query string, body, dan identitas user selain ID dibuang sebelum dikirim.
- Release Sentry: backend `balans-backend@<VERSION>` (tag proses `api`/`worker`), ERP `balans-web-erp@<VERSION>`; `VERSION` = 12 karakter SHA commit yang di-build workflow *deploy-staging*. Production memakai image hasil retag sehingga release-nya sama dengan staging; lingkungan dibedakan field `environment` (`APP_ENV` untuk backend, host untuk ERP). Untuk mencari release sebuah versi: `git rev-parse --short=12 vX.Y.Z`. Versi yang berjalan juga tampil di halaman Tentang Sistem.

### 7.3 Pemeriksaan host dan worker

Cron di VPS (user deploy):

```
*/5 * * * * /opt/balans/deploy/monitoring/host-check.sh >> /opt/balans/host-check.log 2>&1
*/5 * * * * /opt/balans/deploy/monitoring/worker-check.sh production >> /opt/balans/worker-check.log 2>&1
*/5 * * * * /opt/balans/deploy/monitoring/worker-check.sh staging >> /opt/balans/worker-check.log 2>&1
```

- **host-check**: alert ke `ALERT_WEBHOOK_URL` jika disk > 80%, memori tersedia < 10%, container `unhealthy`, container restart berulang, atau container di-OOM-kill karena melewati `mem_limit` (event Docker 5 menit terakhir); lalu ping `HOST_HEARTBEAT_URL`.
- **worker-check**: container worker tidak punya port untuk healthcheck, jadi skrip membaca tabel river di database lingkungan itu — leader river aktif (`river_leader.expires_at > now()`) dan tidak ada job `available` terlambat > 10 menit. Sukses → ping `WORKER_HEARTBEAT_URL`; gagal → alert + ping `/fail`. Job dead-letter baru (state `discarded`) dikirim sebagai alert.

### 7.4 Uji alert (exit criteria Fase 0 butir 7)

| Alert | Cara memicu | Hasil yang diharapkan |
|---|---|---|
| Aplikasi down | `docker stop balans-staging-api-1` selama 3 menit, lalu start | Monitor staging mengirim alert lalu pulih |
| Backup gagal | Panggil `curl $BACKUP_HEARTBEAT_URL/fail` | Alert backup gagal |
| Disk > 80% | Jalankan host-check dengan `DISK_ALERT_PERCENT=1` | Pesan alert di grup tim IT |
| Worker mati | `docker stop balans-staging-worker-1` ± 1 menit, jalankan `worker-check.sh staging`, lalu start | Alert "tidak ada leader river aktif" + heartbeat staging `/fail` |
| OOM container | `docker run --rm -m 32m --name uji-oom alpine sh -c 'tail /dev/zero'`, lalu host-check | Alert "Container kehabisan memori (OOM kill): uji-oom" |
| Uji restore gagal | `RESTORE_BACKUP_NAME=tidak-ada bash restore-test.sh` | Baris `restore GAGAL` di log, alert chat, heartbeat restore `/fail` |
| Sentry backend | Endpoint uji di staging atau error buatan di lokal dengan DSN staging | Event tampil di Sentry tanpa data pribadi |
| Sentry ERP | Panggil `throw` dari konsol browser di staging | Event tampil di Sentry |

Catat tanggal dan hasil setiap uji di tabel exit criteria (dan go/no-go butir 7, PRD Fase 1 bagian 11.2).

### 7.5 Job dead-letter

Job yang gagal 5 kali (`maxJobAttempts`) berstatus `discarded`. Sampai halaman monitoring job di ERP dibangun:

```bash
# Daftar dead-letter 7 hari terakhir
docker exec balans-production-postgres-1 psql -U balans -d balans -c \
  "select id, kind, attempt, finalized_at, errors->-1->>'error' as error from river_job
   where state = 'discarded' and finalized_at > now() - interval '7 days' order by id desc"
# Jalankan ulang satu job setelah penyebabnya diperbaiki (job idempotent, TG 7.3)
docker exec balans-production-postgres-1 psql -U balans -d balans -c \
  "update river_job set state = 'available', scheduled_at = now(), finalized_at = null,
   max_attempts = greatest(max_attempts, attempt + 1) where id = <id> and state = 'discarded'"
```

---

## 8. Kapasitas dan Uji Beban (S-419, NFR-10)

### 8.1 Anggaran sumber daya VPS 4 core / 8 GB

Batas di `deploy/compose.*.yaml` (Technical Guideline 14.1, 14.8). `mem_limit` = `memswap_limit` (container tidak memakai swap; melewati batas = OOM kill lalu restart otomatis dan alert host-check). `cpus` adalah plafon, bukan reservasi. `GOMEMLIMIT` ± 80% batas memori; `GOMAXPROCS` mengikuti plafon CPU secara otomatis (Go ≥ 1.25).

| Project | Container | Memori | CPU | Catatan |
|---|---|---|---|---|
| production | `postgres` | 3 GB | 2,0 | `shared_buffers=2GB`, `effective_cache_size=4GB`, `work_mem=16MB`, `maintenance_work_mem=256MB`, `max_connections=100`, paralel maks. 2 worker, `jit=off`, `random_page_cost=1.1`, `max_wal_size=2GB`, `idle_in_transaction_session_timeout=5min`, log query > 1 detik |
| production | `api` | 512 MB | 1,5 | `GOMEMLIMIT=400MiB` |
| production | `worker` | 512 MB | 1,0 | `GOMEMLIMIT=400MiB`; queue `bulk` 2 worker |
| production | `web` | 128 MB | 0,5 | file static ERP |
| production | `backup` | 256 MB | 0,5 | aktif ± 01.00 WIB |
| production | `migrate` (sementara) | 256 MB | 1,0 | hanya saat deploy |
| edge | `caddy` | 128 MB | 0,5 | |
| staging | `postgres` / `api` / `worker` / `web` / `mailpit` | 512 / 256 / 192 / 64 / 64 MB | 1,0 / 1,0 / 0,5 / 0,25 / 0,25 | total ± 1,1 GB; hentikan bila production butuh: `docker compose -p balans-staging ... stop` |
| — | uji restore (sementara) | 768 MB | 1,0 | bulanan, 03.00 WIB |
| **Total** | | **± 5,7 GB tetap + 1 GB sementara** | | sisa ± 1,3 GB untuk OS, Docker, dan page cache |

Saat `api-storefront`, `web-store`, dan `gotenberg` ditambahkan (+1,8 GB, TG 14.1), staging harus dihentikan di jam sibuk atau VPS dinaikkan kelasnya (bagian 8.5).

### 8.2 Tanda kapasitas mulai habis

- host-check: memori tersedia < 10%, OOM kill, disk > 80%.
- Log Postgres berisi `duration:` (query > 1 detik): `docker logs balans-production-postgres-1 --since 24h 2>&1 | grep duration: | sort -t: -k5 -n | tail`.
- p95 halaman di uji beban ulang mendekati target (bagian 8.4), atau keluhan user lambat di jam sibuk.

### 8.3 Menjalankan uji beban

Skrip k6 `deploy/loadtest/backoffice.js` mensimulasikan user back-office: setiap virtual user login dengan satu akun uji, lalu berulang membuka halaman sesuai permission akunnya — Persetujuan Saya (inbox + jumlah), list & detail Produk, Pelanggan, Pemasok, Mitra, PR/PO, Stok per Lokasi, Kartu Stok, Inbound, Transfer, Stock Count, Jurnal, Bagan Akun, Daftar Harga — dengan jeda baca 1–5 detik; akun ber-permission jurnal (Finance) memposting Jurnal Umum kecil Kas Besar ↔ Bank (simpan draft + posting, ± 15% aksinya). Ambang lulus: list & detail p95 < 2 detik, simpan p95 < 3 detik, login p95 < 3 detik, error < 1% (PRD Fase 1 bagian 9).

Persiapan di staging (sekali):

1. Buat ± 12 akun uji tanpa MFA lewat Pengaturan › Pengguna & Role, mis. `uji.beban01@findyourbalans.com` …, dengan role Finance & Accounting (minimal 2 akun), Purchasing, Staf Gudang, Logistik Leader, Head Logistik, Sales MT, Customer Service, Head Divisi; atur password sama.
2. Isi staging dengan data seukuran production (mock migration, PRD Fase 1 bagian 11.1) agar hasil mewakili.
3. Uji dijalankan **di VPS** ke `http://staging-api:8080` lewat jaringan `balans-edge` (melewati Caddy). Basic auth Caddy staging memakai header `Authorization` yang sama dengan Bearer token API, sehingga uji lewat host publik staging baru bisa setelah basic auth diganti allowlist IP.
4. Production dalam kondisi normal; jangan jalankan uji di jam operasional setelah go-live.

```bash
cd /opt/balans/deploy
USERS=uji.beban01@findyourbalans.com,uji.beban02@findyourbalans.com,... PASSWORD='...' \
  VUS=40 DURATION=15m bash loadtest/run.sh
# uji singkat: DURATION=2m RAMP=20s; tanpa posting jurnal: WRITE=0
```

`run.sh` menjalankan image `grafana/k6:1.3.0`, merekam `docker stats` tiap 5 detik, dan menulis `summary.txt` (ringkasan + status ambang + puncak CPU/memori per container), `summary.json`, `docker-stats.csv`, dan `k6.log` ke `/opt/balans/loadtest-results/<waktu>/`. Exit code ≠ 0 jika ada ambang yang gagal. Jurnal uji berketerangan `UJI BEBAN k6 …` tetap di staging (data staging bukan data resmi).

### 8.4 Template hasil uji beban

| Item | Isi |
|---|---|
| Tanggal / pelaksana | |
| Lingkungan & versi | mis. staging `a1b2c3d4e5f6`, data hasil mock migration 2 |
| Volume data | jumlah produk, jurnal, mutasi stok, PO |
| Skenario | VU, durasi, ramp, jeda baca, WRITE_RATIO |
| Hasil | request total, rps, error %, p95 list, p95 detail, p95 simpan, p95 login, jurnal terposting |
| Puncak container | CPU & memori `postgres`, `api`, `worker` (dari `summary.txt`) |
| OOM / restart selama uji | ya/tidak |
| Kesimpulan | Lulus / tidak lulus NFR-10; tindak lanjut |

**Hasil gladi lokal 17 Oktober 2026** — stack `compose.yaml` + `compose.production.yaml` dengan batas di atas (image lokal `r14test`, `APP_ENV=local` agar `cmd/devseed` bisa membuat akun), k6 di jaringan `balans-edge` ke `production-api:8080`, 14 akun seed:

| Skenario | Request | rps | Error | p95 list | p95 detail | p95 simpan | p95 login | Jurnal | Puncak api | Puncak postgres | Ambang |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 40 VU, 3 menit + ramp 30 dtk, jeda 1–5 dtk | 4.130 | 17,1 | 0,00% | 6 ms | 5 ms | 26 ms | 58 ms | 49 | 9% CPU, 69 MiB | 15% CPU, 153 MiB | semua lulus |
| Stres 200 VU, 2 menit + ramp 5 dtk, jeda 0,2–1 dtk (± 25× beban NFR) | 56.777 | 434,9 | 0,00% | 33 ms | 60 ms | 89 ms | 3.469 ms | 812 | 140% CPU, 304 MiB | 49% CPU, 132 MiB | login gagal |

Tidak ada OOM atau restart. Pada stres, 200 login dalam 5 detik membuat login p95 3,5 detik dan memori api naik ke 304 MiB (argon2id 19 MiB per login bersamaan; plafon CPU api 1,5 core) — beban login serentak sebesar itu tidak realistis untuk 40 user, tetapi menjadi batas atas yang perlu diingat. Data seed sangat kecil, jadi hasil ini membuktikan skrip, batas container, dan jalur posting jurnal; **bukan** pengganti uji di staging dengan data mock migration.

### 8.5 Jalur naik kelas

Mengikuti Technical Guideline 14.8: (1) upgrade VPS (mis. 8 core / 16 GB, naikkan `shared_buffers` ke 25% RAM dan batas container proporsional), (2) pisahkan Postgres ke VPS sendiri, (3) pisahkan website ke VPS sendiri. Ulangi uji beban setelah setiap perubahan.
