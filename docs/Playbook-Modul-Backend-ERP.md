# Playbook Membangun Modul (Backend Go + ERP)

Panduan ringkas untuk menambah modul/fitur Fase 1 dengan pola yang sama dengan modul `identity`, `audit`, dan `approval`. Aturan mengikat tetap [Technical Guideline](Technical-Guideline-Balans-Unified-Business-Platform-v0.1.md); label UI mengikuti [Naming Convention](Naming-Convention-Balans-Menu-Fitur-v1.0.md). Contoh terlengkap: `backend/internal/approval` dan `web-erp/src/features/approval`.

## 1. Struktur modul backend

```
backend/internal/<modul>/
├── <modul>.go          # API publik: Name, Deps, Module, New, RegisterRoutes, RegisterWorkers, PeriodicJobs + fungsi yang dipakai modul lain
├── domain/             # entity, value object, tabel transisi status, validasi (apperr.Validation), TANPA DB/HTTP/time.Now
├── service/            # use case: cek permission (actor.Require), Tx.WithTx, audit.Record, panggil modul lain lewat interface
├── postgres/           # store + queries/*.sql (sqlc) + sqlcgen/ (generate)
└── http/               # handler mengimplementasikan StrictServerInterface (api.gen.go hasil oapi-codegen)
```

- Modul lain HANYA dipakai lewat root package (`balans/internal/<modul>`), sebaiknya lewat interface yang didefinisikan di `service` modul pemakai dan diisi adapter di `internal/app`. Arah dependensi: lihat `backend/tools/lintconfig/main.go` (tier). Dilarang SQL ke tabel modul lain.
- Wiring di `backend/internal/app/app.go` (`buildModules`), aktif jika `cfg.IsEnabled(<modul>.Name)`. Tambahkan modul ke `ENABLED_MODULES` di `deploy/env.example` dan README.
- Permission sudah ada di matriks `backend/internal/identity/domain/permissions.go`; tambah baris/aksi di sana jika resource baru (dan label di `resourceLabels`).

## 2. Database

- Migrasi goose di `backend/migrations/<YYYYMMDDHHMMSS>_<nama>.sql` dengan `-- +goose Up/Down`. ID `uuid DEFAULT uuidv7()`, uang `numeric(18,2)`, qty `numeric(18,3)`, HPP `numeric(18,4)`, persen `numeric(9,4)`, tanggal bisnis `date`.
- Kolom standar master/dokumen: `created_at, created_by, updated_at, updated_by, version, deleted_at, deleted_by` (TG 6.2). Constraint bernama `pk_/uq_/fk_/ck_/ex_<tabel>_...`; FK wajib punya index.
- Soft delete: list/dropdown memfilter `deleted_at IS NULL`; kode bisnis unik permanen (unique tanpa filter deleted); restore oleh Admin + audit. Pola siap pakai (contoh: `backend/internal/catalog`, `backend/internal/warehouse`):
  - Entity menyematkan `softdelete.Mark` (`balans/internal/platform/softdelete`): `Delete(at, by)`, `Restore()`, `EnsureActive()` (tolak ubah data terhapus).
  - Query list `WHERE (deleted_at IS NOT NULL) = sqlc.arg('deleted')::boolean` → param API `deleted=true` menampilkan daftar *Dihapus*; query get-by-ID **tidak** memfilter agar dokumen lama tetap bisa menampilkan datanya. Satu query `Set<X>Deleted` (`deleted_at = sqlc.narg(...)`) dipakai untuk hapus dan pulihkan.
  - Service: hapus `actor.Require(ctx, PermWrite)`; pulihkan `softdelete.RequireRestore(ctx, PermWrite)` (role Admin Sistem + permission tulis); audit `ActionDelete`/`ActionRestore` dengan `Before/After: {"deletedAt": ...}`. `allowedActions` berisi `restore` jika `softdelete.CanRestore`.
  - Endpoint: `DELETE /x/{id}` (204) dan `POST /x/{id}/restore`. Dropdown modul lain hanya mengambil data aktif dan belum dihapus.
- Daftarkan permission baca entitas untuk tab Riwayat: fungsi `AuditEntities()` di root package modul, lalu `registerAuditEntities(a.audit, <modul>.AuditEntities())` di `app.go`.
- Effective-dated (harga, PKS, tarif): `valid_from`/`valid_to` + `EXCLUDE USING gist` (extension `btree_gist` sudah dibuat).
- Query sqlc di `postgres/queries/*.sql`; daftarkan paket di `backend/sqlc.yaml` (pola `<<: *pkg`). Override tipe sudah ada: uuid, timestamptz (`time.Time`/`*time.Time`), numeric (`decimal.Decimal`/`decimal.NullDecimal`), date (`clock.BusinessDate`). Gunakan cast eksplisit `sqlc.arg('x')::timestamptz` bila tipe ambigu.
- Generate: `bash scripts/generate.sh` (bundle OpenAPI, oapi-codegen, sqlc via Docker `sqlc/sqlc:1.30.0`, lint config, tipe TS). Setelah itu jalankan `cd web-erp && npx prettier --write src/lib/api/schema.gen.ts`.

## 3. API (spec-first)

- Path di `backend/api/paths/<modul>.yaml` (+ `components.schemas` di file yang sama), didaftarkan di `backend/api/openapi.yaml` (paths + tag). Codegen config `backend/api/codegen/<modul>.yaml` (salin dari `approval.yaml`, ganti tag & output).
- Komponen bersama: `../components/common.yaml#/Money` (string desimal), `#/BusinessDate`, `#/Pagination`, `#/VersionInput`, parameter `Page, PageSize, Sort, Search, IdempotencyKey, Id`, response `Problem`.
- Semua POST/PUT/DELETE memakai header `Idempotency-Key`. Update memakai `version` (optimistic locking → `VERSION_CONFLICT`). Detail dokumen mengembalikan `allowedActions`.
- Error lewat `apperr` (kode di `backend/internal/platform/apperr/code.go`). Kode baru wajib ditambah sekaligus di `code.go`, `api/components/problem.yaml`, `web-erp/src/lib/api/error-messages.ts`, `web-erp/src/lib/api/problem.test.ts`, dan Lampiran A Technical Guideline.

## 4. Pola service

```go
func (s *Service) Create(ctx context.Context, in domain.XInput) (domain.X, error) {
	a, err := actor.Require(ctx, PermXWrite)
	if err != nil { return domain.X{}, err }
	x, err := domain.NewX(in)              // validasi di domain
	if err != nil { return domain.X{}, err }
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if x.ID, err = s.Store.Insert(ctx, x, a.UserID); err != nil { return err }
		return s.Audit.Record(ctx, audit.Entry{Action: audit.ActionCreate, EntityType: "x", EntityID: x.ID, EntityLabel: x.Code, After: snapshot(x)})
	})
	return x, err
}
```

- Approval dokumen: `approval.Module.Request(ctx, approval.Subject{...})` di dalam transaksi submit; daftarkan handler `RegisterHandler(subjectType, readPermission, approval.Handler{OnApproved, OnRejected})` saat wiring (handler dipanggil di transaksi keputusan). Hapus jenis itu dari `registerDevApprovalHandlers` bila modul dokumen sudah mendaftar sendiri (otomatis ditimpa karena registrasi terakhir menang — daftarkan SETELAH approval dibuat).
- Notifikasi: `notification.Module.Notify(ctx, userIDs, kind, title, body, link)` + `QueueEmail`.
- Job: river worker + `PeriodicJobs`, argumen hanya ID, idempotent (TG 7.3, Lampiran B).
- Waktu selalu dari `clock.Clock`; tanggal bisnis `clock.Today(clock)`.

## 5. Test

- Domain: table-driven, coverage ≥ 80% (`bash scripts/coverage-gate.sh`).
- Service: test integrasi Postgres asli via `dbtest.Pool(t)` (+ `TestMain` → `dbtest.Main`), contoh `backend/internal/approval/service/service_test.go`. Docker harus jalan (`dockerd &` jika belum). Jalankan `BALANS_REQUIRE_DB=1 go test ./...`.
- Lint: `golangci-lint run ./...` (v2, dibangun dengan Go 1.27: `GOTOOLCHAIN=go1.27.2 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`).

## 6. ERP (web-erp)

- Fitur di `src/features/<modul>/{api,components}`; route di `src/routes/_app/...` (TanStack file route, `validateSearch` zod untuk filter/halaman di URL). `routeTree.gen.ts` dibangkitkan `pnpm exec vite build`.
- API client: `api.GET/POST` dari `@/lib/api/client` + `unwrap`, `queryOptions`, mutasi dengan `useIdempotentMutation` (header `Idempotency-Key`). Error: `parseProblem` → field error ke form, selain itu `showErrorDialog`.
- Komponen: `DataTable`, `StatusBadge`, `ConfirmDialog`, `LoadingButton`, `MoneyInput`, `DateInput`, `QuantityInput`, `Textarea`, `UserPicker` (approval), `PageTabs` (tab di URL), `RecordStatusBadge` (Aktif/Nonaktif/Dihapus), `Barcode` (SVG EAN-13/Code 128), `ProductPicker` (catalog), dan `EntityHistory` (`src/features/audit/components/entity-history.tsx`) untuk tab **Riwayat** di halaman detail. Dialog form: render isi form hanya saat terbuka (`{open ? <Form/> : null}`) dan inisialisasi state di `useState`, JANGAN setState di `useEffect` (lint `react-hooks/set-state-in-effect`).
- Menu: `src/lib/navigation/menu.ts` — isi `to`, `permission`, dan `module` pada item yang sudah dibangun; label persis Naming Convention.
- Format: `formatRupiah`, `formatDate`, `formatDateTime`; uang dikirim sebagai string desimal.
- HP: tabel bergulir horizontal; aksi utama di HP minimal 44 px; uji 3 lebar.
- Cek: `pnpm lint && pnpm typecheck && pnpm test && pnpm build`; format file baru dengan `npx prettier --write <file>`.

## 7. Selesai berarti

Kode + migrasi + OpenAPI + handler + test (domain & integrasi) + halaman ERP + menu + baris di `docs/Status-Fase-1.md` (status story, asumsi yang diambil). Commit per fitur dengan pesan Bahasa Indonesia gaya `feat: ...`.
