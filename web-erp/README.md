# web-erp

ERP back-office Balans: Vite + React + TypeScript, shadcn/ui (preset Balans `base-vega`), TanStack Router (file-based) dan TanStack Query, React Hook Form + Zod, Zustand, client `openapi-fetch` dari tipe hasil generate. Struktur folder dan aturan mengikuti Technical Guideline bagian 10.

| Folder | Isi |
|---|---|
| `src/routes/` | Definisi route dan komposisi halaman saja |
| `src/features/<modul>/` | Query/mutation, komponen, schema Zod per fitur |
| `src/components/common/` | Komponen bersama: `ErrorDialog`, `ConfirmDialog`, `LoadingButton`, `CircularProgress`, `ProgressDialog`, `DataTable`, `MoneyInput`, `QuantityInput`, `DateInput`, `StatusBadge`, `FileUpload` |
| `src/components/ui/` | Komponen hasil generate shadcn (jangan diubah logikanya) |
| `src/lib/api/` | Client API, `schema.gen.ts`, parser error, `error-messages.ts`, `useIdempotentMutation` |
| `src/lib/format/` | Format uang, tanggal WIB, angka, `status-labels.ts` |
| `src/lib/navigation/menu.ts` | Struktur menu Naming Convention bagian 5, difilter modul aktif |

Perintah: `pnpm dev`, `pnpm lint`, `pnpm typecheck`, `pnpm test`, `pnpm test:e2e`, `pnpm generate:api`. Cara menjalankan bersama backend ada di README root.
