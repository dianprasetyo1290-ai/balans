# Status Fase 1
## Balans Nutrition — Unified Business Platform: Core Operations & Finance

| Item | Keterangan |
|---|---|
| Dokumen | Pelacak pengerjaan story Backlog Fase 1 terhadap PRD Fase 1 |
| Acuan | PRD Fase 1 v0.1 bagian 6; Backlog Fase 1 |
| Diperbarui | 16 Oktober 2026 |

**Status:** **Selesai** = kode, test, dan kriteria penerimaan backlog terpenuhi di lokal. **Sebagian** = sebagian kriteria terpenuhi; sisanya disebut. **Belum** = belum dikerjakan.

---

## R1.1 — Fondasi & Master Data

| Story | Fitur PRD | Status | Catatan |
|---|---|---|---|
| S-101 Login | F1-101 | Selesai | JWT EdDSA 15 menit + refresh token berotasi di cookie HttpOnly; deteksi pemakaian ulang mencabut keluarga sesi; 5× gagal → kunci 15 menit; logout mencabut sesi; halaman Masuk, Lupa password, Atur password |
| S-102 User & role | F1-101 | Selesai | CRUD pengguna, undangan atur password via email, nonaktif/ganti role menaikkan `token_version` dan memutus sesi (cache 30 detik, langsung di proses yang sama); halaman Pengaturan › Pengguna & Role |
| S-103 Permission per role | F1-102 | Selesai | Katalog permission disemai dari matriks Desain Role & Approval; Admin mengubah lewat Pengaturan › Hak Akses; menu tersembunyi sesuai permission; API menolak 403 |
| S-104 MFA | F1-102 | Selesai | TOTP wajib untuk role ber-MFA dengan QR enrol; re-autentikasi 5 menit untuk aksi ★ dibuka otomatis oleh dialog konfirmasi identitas |
| S-105 Audit trail | F1-103 | Sebagian | Modul audit, diff + masking, append-only di DB, halaman Riwayat Perubahan; login/gagal login/logout/user/role tercatat. Tab Riwayat di dokumen dibangun bersama halaman detail dokumen |
| S-106 Soft delete | F1-103 | Belum | Pola dibangun bersama master data pertama |
| S-107 Responsif | F1-104 | Sebagian | Layout & halaman baru diuji Playwright desktop + HP |
| S-108 Lokalisasi | F1-104 | Selesai (Fase 0) | Formatter Rupiah/WIB, pesan Indonesia |
| S-109 Privasi data | F1-105 | Sebagian | `platform/pii` AES-256-GCM + HMAC dengan rotasi kunci; dipakai secret MFA dan hash email login. Kolom NIK/NPWP/rekening menyusul di master terkait |
| S-110 – S-112 Approval | F1-106 | Belum | Berikutnya |
| S-113 – S-134 | F1-107 – F1-120 | Belum | |

## Pengembangan Lokal

- `cmd/devseed` membuat satu akun per role dengan password sama (`Balans-Lokal-2026`), hanya di `APP_ENV=local`.
- `DEV_SKIP_MFA=true` (hanya `APP_ENV=local`) melewati kode autentikator agar demo lokal tidak memerlukan HP.

## Asumsi yang Diambil

| # | Asumsi | Alasan |
|---|---|---|
| 1 | Label menu audit trail: **Riwayat Perubahan** di grup Pengaturan | Naming Convention belum memuat label untuk NFR-03; dimasukkan ke uji label F0-UX-05 |
| 2 | Kode error baru `AUTH_MFA_ENROLLMENT_REQUIRED` (403) dan `AUTH_PASSWORD_TOKEN_INVALID` (422) | Dibutuhkan alur aktivasi MFA wajib dan tautan password; sudah ditambahkan ke Technical Guideline Lampiran A |
| 3 | Masa berlaku tautan undangan 72 jam, atur ulang password 1 jam | Tidak ditetapkan BRD/TG; nilai umum yang aman |
| 4 | Refresh dari dua tab bersamaan dalam 10 detik tidak dianggap pencurian token | Mencegah user terlempar keluar saat membuka banyak tab |
| 5 | Admin Sistem tidak bisa mencabut role Admin atau hak mengatur role dari dirinya/role Admin | Mencegah sistem terkunci tanpa admin |
