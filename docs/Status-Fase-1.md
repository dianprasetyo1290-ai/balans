# Status Fase 1
## Balans Nutrition — Unified Business Platform: Core Operations & Finance

| Item | Keterangan |
|---|---|
| Dokumen | Pelacak pengerjaan story Backlog Fase 1 terhadap PRD Fase 1 |
| Acuan | PRD Fase 1 v0.1 bagian 6; Backlog Fase 1 |
| Diperbarui | 17 Oktober 2026 |

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
| S-110 Notifikasi approver | F1-106 | Selesai | Notifikasi in-app (lonceng di header) + email saat menjadi approver dan saat dokumen diputuskan; job `approval.remind` tiap jam, hanya Senin–Jumat 08.00–17.00 WIB; badge jumlah di menu *Persetujuan Saya* |
| S-111 Inbox persetujuan | F1-106, BRD 5.17 | Selesai | Halaman *Persetujuan Saya*: Menunggu Saya (lintas jenis dokumen, tampilan kartu di HP), Diajukan Saya, Riwayat Keputusan, Delegasi; tolak wajib catatan; delegasi saat cuti maks. 90 hari, tanpa tumpang tindih |
| S-112 Engine approval | F1-106, BRD 5.17 | Selesai | Modul `approval`: kebijakan effective-dated per jenis/nominal/divisi/syarat, matriks bagian 4 disemai; level berurutan; snapshot langkah saat submit; pembuat tidak pernah menyetujui dokumennya sendiri (level naik ke pengganti); handler modul dokumen dipanggil di transaksi yang sama; keputusan tercatat di audit trail dokumen. Pengaturan › *Matriks Persetujuan* untuk tambah/ubah/hentikan level. Modul dokumen pertama yang memakai engine: PR/PO (R1.2) |
| S-113 – S-134 | F1-107 – F1-120 | Belum | |

## Pengembangan Lokal

- `cmd/devseed` membuat satu akun per role dengan password sama (`Balans-Lokal-2026`), hanya di `APP_ENV=local`.
- `cmd/devseed` juga mengajukan 5 dokumen contoh (PO, pengajuan biaya, disposisi retur, daftar harga) agar inbox bisa didemokan sebelum modul dokumen ada; handler contoh hanya dipasang di `APP_ENV=local`.
- `DEV_SKIP_MFA=true` (hanya `APP_ENV=local`) melewati kode autentikator agar demo lokal tidak memerlukan HP.

## Asumsi yang Diambil

| # | Asumsi | Alasan |
|---|---|---|
| 1 | Label menu audit trail: **Riwayat Perubahan** di grup Pengaturan | Naming Convention belum memuat label untuk NFR-03; dimasukkan ke uji label F0-UX-05 |
| 2 | Kode error baru `AUTH_MFA_ENROLLMENT_REQUIRED` (403) dan `AUTH_PASSWORD_TOKEN_INVALID` (422) | Dibutuhkan alur aktivasi MFA wajib dan tautan password; sudah ditambahkan ke Technical Guideline Lampiran A |
| 3 | Masa berlaku tautan undangan 72 jam, atur ulang password 1 jam | Tidak ditetapkan BRD/TG; nilai umum yang aman |
| 4 | Refresh dari dua tab bersamaan dalam 10 detik tidak dianggap pencurian token | Mencegah user terlempar keluar saat membuka banyak tab |
| 5 | Admin Sistem tidak bisa mencabut role Admin atau hak mengatur role dari dirinya/role Admin | Mencegah sistem terkunci tanpa admin |
| 6 | Jam kerja pengingat approval: Senin–Jumat 08.00–17.00 WIB, belum memperhitungkan hari libur nasional | NFR-11 menyebut "jam kerja" tanpa rincian; dipindah ke modul `setting` saat modul itu dibangun |
| 7 | Menu *Persetujuan Saya* di grup Ringkasan | Naming Convention hanya memuat *Persetujuan Pengadaan* (Pengadaan); inbox lintas dokumen butuh tempat yang terlihat semua approver. Masuk uji label F0-UX-05 |
| 8 | Perubahan Matriks Persetujuan berlaku langsung oleh Admin Sistem (tercatat di audit trail), belum melalui approval Direksi | Matriks role memberi Direksi `approval.policy.approve`, tetapi alur persetujuan perubahan matriks belum didefinisikan; diusulkan dibahas di sprint review |
| 9 | Delegasi maksimal 90 hari per entri dan satu delegasi per user per tanggal | Mencegah delegasi tanpa batas waktu; cuti lebih panjang dicatat sebagai beberapa entri |
| 10 | Level 1 rekonsiliasi konsinyasi memakai role Sales MT; pembatasan ke PIC partner menunggu master mitra (S-121) | Kebijakan bisa menunjuk user khusus per level sebagai solusi sementara |
| 11 | Dokumen tidak bisa diajukan jika ada level tanpa approver aktif selain pembuat (`APPROVAL_POLICY_MISSING`) | Mencegah dokumen tertahan tanpa ada yang bisa menyetujui |
