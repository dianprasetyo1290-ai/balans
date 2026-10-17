# Balans Platform

Monorepo Unified Business Platform Balans Nutrition: ERP back-office, Finance & Accounting, dan (kelak) website e-commerce. Aturan teknis mengikat ada di [Technical Guideline](docs/Technical-Guideline-Balans-Unified-Business-Platform-v0.1.md); label UI mengikuti [Naming Convention](docs/Naming-Convention-Balans-Menu-Fitur-v1.0.md).

| Folder | Isi |
|---|---|
| `backend/` | Go modular monolith: binary `api`, `worker`, `migrate`, `cutover`, `storefront`, `healthcheck` |
| `web-erp/` | ERP Vite + React + shadcn (preset Balans), TanStack Router/Query |
| `web-store/` | Website e-commerce (dibangun menjelang rilis website) |
| `deploy/` | Docker Compose, Caddy, image Postgres + WAL-G, skrip deploy/backup/monitoring |
| `docs/` | BRD, Roadmap, PRD, Technical Guideline, desain, runbook — dokumen proyek, tidak dipublikasikan di repo ini; minta ke tim proyek |
| `scripts/` | Generate kode, gate coverage, proteksi branch |

## Prasyarat

Go 1.27.2, Node 24.19.0, pnpm 10.34.6, Docker. `golangci-lint` v2.14.0 untuk lint lokal. `sqlc` tidak wajib dipasang: di Windows dijalankan lewat image Docker oleh `scripts/generate.sh`.

## Menjalankan lokal

```bash
# 1. Postgres dan Mailpit (port bisa diganti jika bentrok)
POSTGRES_PORT=5432 SMTP_PORT=1025 MAILPIT_PORT=8025 docker compose -f deploy/compose.local.yaml up -d --wait

# 2. Backend (terminal 1 dan 2)
cd backend
export DATABASE_URL="postgres://balans:balans@localhost:5432/balans?sslmode=disable"
export APP_ENV=local ENABLED_MODULES=identity,audit,notification,approval,filestore,party,hris,sales,logistics
export MIDTRANS_SERVER_KEY=local BITESHIP_WEBHOOK_SECRET=local
export DEV_SKIP_MFA=true  # opsional, hanya local: login tanpa kode autentikator
go run ./cmd/migrate up
go run ./cmd/devseed      # satu akun per role, password sama (default Balans-Lokal-2026)
go run ./cmd/api          # http://localhost:8080
go run ./cmd/worker

# 3. ERP (terminal 3)
cd web-erp
pnpm install
pnpm dev                  # http://localhost:5173, /api diteruskan ke :8080
```

Halaman yang tersedia: **Masuk** (`/login`), **Ringkasan Operasional** (`/`), **Persetujuan Saya** (`/approvals`), **Pengaturan › Pengguna & Role, Hak Akses, Matriks Persetujuan, Riwayat Perubahan**, **Tentang Sistem** (`/system/about`), simulasi pihak ketiga di `/_mock/` (pembayaran, pengiriman, kotak email Mailpit di `/_mock/mail/` — tautan undangan & atur ulang password muncul di sini).

Akun seed lokal: `admin@balans.local`, `purchasing@balans.local`, `staf.gudang@balans.local`, `logistik.leader@balans.local`, `fulfillment.leader@balans.local`, `merchandiser@balans.local`, `head.logistik@balans.local`, `finance@balans.local`, `head.finance@balans.local`, `sales.mt@balans.local`, `sales.ecommerce@balans.local`, `cs@balans.local`, `head.sales@balans.local`, `marketing@balans.local`, `hrga@balans.local`, `head.divisi@balans.local`, `direksi@balans.local`, `dirut@balans.local`. `cmd/devseed` menolak berjalan di luar `APP_ENV=local`. Di local tanpa `JWT_*`/`PII_*` dipakai kunci pengembangan deterministik; staging dan production wajib kunci asli (Runbook 3.1 – 3.2).

Konfigurasi teknis lewat env (lihat `deploy/env.example`). Nilai bisnis DILARANG di env; tempatnya modul `setting`.

## Perintah harian

| Tujuan | Perintah |
|---|---|
| Generate ulang dari kontrak (OpenAPI bundle, oapi-codegen, sqlc, lint config, tipe TS) | `bash scripts/generate.sh` |
| Test backend (butuh Docker; Postgres asli via testcontainers) | `cd backend && go test ./...` |
| Test backend di Docker Desktop Windows | `TESTCONTAINERS_RYUK_DISABLED=true go test -p 1 ./...` |
| Lint backend (termasuk depguard batas modul) | `cd backend && golangci-lint run ./...` |
| Gate coverage 80% (`domain`, `tax`, `pricing`, `money`) | `bash scripts/coverage-gate.sh` |
| Lint, typecheck, test, build ERP | `cd web-erp && pnpm lint && pnpm typecheck && pnpm test && pnpm build` |
| Smoke test E2E | `cd web-erp && E2E_BASE_URL=http://localhost:5173 pnpm test:e2e` |
| Menambah modul baru ke aturan depguard | ubah `backend/tools/lintconfig/main.go`, lalu `scripts/generate.sh` |

Alur perubahan API: ubah `backend/api/openapi.yaml` atau `backend/api/paths/*.yaml` → `scripts/generate.sh` → implementasikan interface hasil generate di `internal/<modul>/http`. CI gagal jika hasil generate belum di-commit.

## Rilis

| Lingkungan | Host | Pemicu |
|---|---|---|
| staging | `erp-staging.findyourbalans.com` (basic auth) | merge ke `main` |
| production | `erp.findyourbalans.com` | tag `vX.Y.Z` + approval di GitHub Environment `production` |

Image dibangun sekali per commit (tag SHA) dan dipakai ulang untuk production lewat retag. Deploy menjalankan `deploy/scripts/deploy.sh`: backup (production) → migrate → ganti container → smoke `/readyz` → rollback otomatis bila gagal. Rollback manual: jalankan workflow *deploy-production* dengan versi sebelumnya. Detail operasi ada di [Runbook Operasi](docs/Runbook-Operasi-Balans-Platform.md).

Fase BRD hanya ditulis di catatan rilis, tidak di nama file, branch, atau tag (Technical Guideline 4.2).
