// Pemetaan kode error → judul, teks default, dan tombol (Technical Guideline 10.6, Lampiran A).
// Kode baru ditambahkan bersamaan di Lampiran A, apperr, dan OpenAPI.

export type ErrorAction = "close" | "retry" | "reload" | "relogin"

export type ErrorDisplay = "modal" | "inline" | "silent"

export type ErrorMessage = {
  title: string
  message: string
  action: ErrorAction
  display: ErrorDisplay
}

export const ERROR_MESSAGES = {
  VALIDATION_FAILED: { title: "Data tidak valid", message: "Periksa kembali isian yang ditandai.", action: "close", display: "inline" },
  AUTH_INVALID_CREDENTIALS: { title: "Gagal masuk", message: "Email atau password salah.", action: "close", display: "modal" },
  AUTH_ACCOUNT_LOCKED: { title: "Akun dikunci", message: "Akun dikunci sementara karena terlalu banyak percobaan masuk. Coba lagi dalam 15 menit.", action: "close", display: "modal" },
  AUTH_MFA_REQUIRED: { title: "Kode autentikasi diperlukan", message: "Masukkan kode dari aplikasi autentikator.", action: "close", display: "inline" },
  AUTH_MFA_INVALID: { title: "Kode autentikasi salah", message: "Kode autentikasi salah.", action: "close", display: "inline" },
  AUTH_TOKEN_EXPIRED: { title: "Sesi diperbarui", message: "", action: "close", display: "silent" },
  AUTH_SESSION_EXPIRED: { title: "Sesi berakhir", message: "Sesi Anda berakhir. Silakan masuk kembali.", action: "relogin", display: "modal" },
  AUTH_REAUTH_REQUIRED: { title: "Konfirmasi identitas", message: "Konfirmasi identitas Anda untuk melanjutkan.", action: "close", display: "modal" },
  FORBIDDEN: { title: "Akses ditolak", message: "Anda tidak memiliki akses untuk tindakan ini.", action: "close", display: "modal" },
  NOT_FOUND: { title: "Tidak ditemukan", message: "Data tidak ditemukan.", action: "close", display: "modal" },
  VERSION_CONFLICT: { title: "Data sudah berubah", message: "Data sudah diubah oleh pengguna lain. Muat ulang untuk melihat perubahan terbaru.", action: "reload", display: "modal" },
  INVALID_STATE_TRANSITION: { title: "Tindakan tidak dapat dilakukan", message: "Status dokumen tidak memungkinkan tindakan ini.", action: "close", display: "modal" },
  IDEMPOTENCY_KEY_REQUIRED: { title: "Terjadi kesalahan", message: "Terjadi kesalahan. Muat ulang halaman.", action: "reload", display: "modal" },
  IDEMPOTENCY_IN_PROGRESS: { title: "Masih diproses", message: "Permintaan sebelumnya masih diproses. Tunggu sebentar.", action: "close", display: "modal" },
  IDEMPOTENCY_KEY_REUSED: { title: "Terjadi kesalahan", message: "Terjadi kesalahan. Muat ulang halaman.", action: "reload", display: "modal" },
  DUPLICATE_CODE: { title: "Kode sudah dipakai", message: "Kode sudah dipakai.", action: "close", display: "inline" },
  PERIOD_LOCKED: { title: "Periode sudah ditutup", message: "Periode sudah ditutup. Transaksi tidak bisa dibuat atau diubah.", action: "close", display: "modal" },
  STOCK_INSUFFICIENT: { title: "Stok tidak cukup", message: "Stok tidak cukup.", action: "close", display: "modal" },
  BATCH_SHELF_LIFE_TOO_SHORT: { title: "Umur simpan tidak cukup", message: "Sisa umur simpan batch kurang dari batas channel.", action: "close", display: "modal" },
  APPROVAL_REQUIRED: { title: "Membutuhkan persetujuan", message: "Tindakan ini membutuhkan persetujuan. Dokumen sudah diajukan untuk persetujuan.", action: "close", display: "modal" },
  DISCOUNT_LIMIT_EXCEEDED: { title: "Diskon melebihi batas", message: "Diskon melebihi batas role Anda dan membutuhkan persetujuan.", action: "close", display: "modal" },
  CREDIT_LIMIT_EXCEEDED: { title: "Batas kredit terlampaui", message: "Transaksi melebihi batas kredit pelanggan.", action: "close", display: "modal" },
  THREE_WAY_MATCH_FAILED: { title: "Tagihan tidak cocok", message: "Tagihan tidak cocok dengan PO dan penerimaan barang melebihi toleransi.", action: "close", display: "modal" },
  DEPOSIT_INSUFFICIENT: { title: "Deposit tidak cukup", message: "Saldo deposit reseller tidak cukup.", action: "close", display: "modal" },
  OUTSIDE_ATTENDANCE_RADIUS: { title: "Di luar radius", message: "Lokasi Anda di luar radius lokasi kerja.", action: "close", display: "modal" },
  FILE_TOO_LARGE: { title: "File terlalu besar", message: "Ukuran file tidak diizinkan.", action: "close", display: "inline" },
  FILE_TYPE_NOT_ALLOWED: { title: "Tipe file tidak diizinkan", message: "Tipe file tidak diizinkan.", action: "close", display: "inline" },
  IMPORT_ALREADY_PROCESSED: { title: "File sudah diimpor", message: "File ini sudah pernah diimpor.", action: "close", display: "modal" },
  RATE_LIMITED: { title: "Terlalu banyak permintaan", message: "Terlalu banyak permintaan. Coba lagi sebentar lagi.", action: "close", display: "modal" },
  PROVIDER_UNAVAILABLE: { title: "Layanan tidak tersedia", message: "Layanan pihak ketiga sedang tidak tersedia.", action: "retry", display: "modal" },
  BAD_REQUEST: { title: "Permintaan tidak valid", message: "Permintaan tidak dapat dibaca. Muat ulang halaman.", action: "reload", display: "modal" },
  NETWORK_ERROR: { title: "Koneksi terputus", message: "Koneksi terputus. Periksa jaringan Anda lalu coba lagi.", action: "retry", display: "modal" },
  INTERNAL_ERROR: { title: "Terjadi kesalahan", message: "Terjadi kesalahan pada sistem.", action: "retry", display: "modal" },
} as const satisfies Record<string, ErrorMessage>

export type ErrorCode = keyof typeof ERROR_MESSAGES

export function isErrorCode(value: unknown): value is ErrorCode {
  return typeof value === "string" && Object.hasOwn(ERROR_MESSAGES, value)
}

export function errorMessageFor(code: string): ErrorMessage {
  return isErrorCode(code) ? ERROR_MESSAGES[code] : ERROR_MESSAGES.INTERNAL_ERROR
}

export const ACTION_LABELS: Record<ErrorAction, string> = {
  close: "Tutup",
  retry: "Coba lagi",
  reload: "Muat ulang",
  relogin: "Masuk kembali",
}
