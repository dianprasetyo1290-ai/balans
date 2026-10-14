## Ringkasan

<!-- Apa yang berubah dan kenapa. Rujuk ID requirement BRD (mis. PRC-06, A-01) jika relevan. -->

## Cara menguji

<!-- Langkah verifikasi untuk reviewer. -->

## Checklist (Technical Guideline Lampiran C)

**Umum**
- [ ] Nama file, folder, branch bersih (tanpa fase, versi, sprint, temp)
- [ ] Istilah domain sesuai glosarium
- [ ] Tidak ada secret, kode mati, `console.log`/`fmt.Println`
- [ ] Lint dan test lolos; test ditambahkan untuk logika baru/bug fix

**Backend**
- [ ] Layer dipatuhi: tidak ada aturan bisnis di handler, tidak ada SQL di service
- [ ] Tidak mengakses tabel/subpackage modul lain
- [ ] Uang/qty memakai desimal; pembulatan lewat `money`
- [ ] Endpoint yang mengubah data idempotent; update memakai `version`
- [ ] Lock diambil dengan urutan konsisten; tidak ada panggilan eksternal di transaksi
- [ ] Efek samping eksternal lewat job di dalam transaksi
- [ ] Jurnal lewat posting rule; dimensi lengkap; tidak ada update/delete ledger
- [ ] Permission & scoping data dicek di service
- [ ] Audit trail dicatat
- [ ] Migrasi bersifat expand → contract, FK punya index
- [ ] Kode error baru terdaftar di Lampiran A dan OpenAPI
- [ ] Tidak ada data pribadi di log

**Frontend**
- [ ] Tipe API dari hasil generate, bukan manual
- [ ] Filter/pagination di URL; data server lewat TanStack Query
- [ ] Mutation memakai `useIdempotentMutation`
- [ ] Error field inline, error bisnis/otentikasi di `ErrorDialog`
- [ ] Loading memakai komponen bersama (`LoadingButton`, `CircularProgress`, `ProgressDialog`)
- [ ] Uang dikirim sebagai string desimal; format lewat `lib/format`
- [ ] Tampilan dicek di lebar HP
