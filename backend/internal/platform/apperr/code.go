package apperr

import "net/http"

// Code adalah kontrak stabil untuk FE (Technical Guideline Lampiran A).
// Kode baru ditambahkan bersamaan di Lampiran A, OpenAPI, dan web-erp/src/lib/api/error-messages.ts.
type Code string

//nolint:gosec // G101: nama kode error, bukan kredensial
const (
	CodeValidationFailed        Code = "VALIDATION_FAILED"
	CodeAuthInvalidCredentials  Code = "AUTH_INVALID_CREDENTIALS"
	CodeAuthAccountLocked       Code = "AUTH_ACCOUNT_LOCKED"
	CodeAuthMFARequired         Code = "AUTH_MFA_REQUIRED"
	CodeAuthMFAInvalid          Code = "AUTH_MFA_INVALID"
	CodeAuthTokenExpired        Code = "AUTH_TOKEN_EXPIRED"
	CodeAuthSessionExpired      Code = "AUTH_SESSION_EXPIRED"
	CodeAuthReauthRequired      Code = "AUTH_REAUTH_REQUIRED"
	CodeAuthMFAEnrollment       Code = "AUTH_MFA_ENROLLMENT_REQUIRED"
	CodeAuthPasswordToken       Code = "AUTH_PASSWORD_TOKEN_INVALID"
	CodeForbidden               Code = "FORBIDDEN"
	CodeNotFound                Code = "NOT_FOUND"
	CodeVersionConflict         Code = "VERSION_CONFLICT"
	CodeInvalidStateTransition  Code = "INVALID_STATE_TRANSITION"
	CodeIdempotencyKeyRequired  Code = "IDEMPOTENCY_KEY_REQUIRED"
	CodeIdempotencyInProgress   Code = "IDEMPOTENCY_IN_PROGRESS"
	CodeIdempotencyKeyReused    Code = "IDEMPOTENCY_KEY_REUSED"
	CodeDuplicateCode           Code = "DUPLICATE_CODE"
	CodePeriodLocked            Code = "PERIOD_LOCKED"
	CodeStockInsufficient       Code = "STOCK_INSUFFICIENT"
	CodeBatchShelfLifeTooShort  Code = "BATCH_SHELF_LIFE_TOO_SHORT"
	CodeApprovalRequired        Code = "APPROVAL_REQUIRED"
	CodeDiscountLimitExceeded   Code = "DISCOUNT_LIMIT_EXCEEDED"
	CodeCreditLimitExceeded     Code = "CREDIT_LIMIT_EXCEEDED"
	CodeThreeWayMatchFailed     Code = "THREE_WAY_MATCH_FAILED"
	CodeDepositInsufficient     Code = "DEPOSIT_INSUFFICIENT"
	CodeOutsideAttendanceRadius Code = "OUTSIDE_ATTENDANCE_RADIUS"
	CodeFileTooLarge            Code = "FILE_TOO_LARGE"
	CodeFileTypeNotAllowed      Code = "FILE_TYPE_NOT_ALLOWED"
	CodeImportAlreadyProcessed  Code = "IMPORT_ALREADY_PROCESSED"
	CodeRateLimited             Code = "RATE_LIMITED"
	CodeProviderUnavailable     Code = "PROVIDER_UNAVAILABLE"
	CodeBadRequest              Code = "BAD_REQUEST"
	CodeInternal                Code = "INTERNAL_ERROR"
)

type codeSpec struct {
	status  int
	title   string
	message string
}

var codeSpecs = map[Code]codeSpec{
	CodeValidationFailed:        {http.StatusUnprocessableEntity, "Data tidak valid", "Periksa kembali isian yang ditandai."},
	CodeAuthInvalidCredentials:  {http.StatusUnauthorized, "Gagal masuk", "Email atau password salah."},
	CodeAuthAccountLocked:       {http.StatusLocked, "Akun dikunci", "Akun dikunci sementara karena terlalu banyak percobaan masuk. Coba lagi dalam 15 menit."},
	CodeAuthMFARequired:         {http.StatusUnauthorized, "Kode autentikasi diperlukan", "Masukkan kode dari aplikasi autentikator."},
	CodeAuthMFAInvalid:          {http.StatusUnauthorized, "Kode autentikasi salah", "Kode autentikasi salah."},
	CodeAuthTokenExpired:        {http.StatusUnauthorized, "Token kedaluwarsa", "Token akses kedaluwarsa."},
	CodeAuthSessionExpired:      {http.StatusUnauthorized, "Sesi berakhir", "Sesi Anda berakhir. Silakan masuk kembali."},
	CodeAuthReauthRequired:      {http.StatusUnauthorized, "Konfirmasi identitas", "Konfirmasi identitas Anda untuk melanjutkan."},
	CodeAuthMFAEnrollment:       {http.StatusForbidden, "Aktifkan autentikasi dua langkah", "Role Anda mewajibkan autentikasi dua langkah. Aktifkan terlebih dahulu untuk melanjutkan."},
	CodeAuthPasswordToken:       {http.StatusUnprocessableEntity, "Tautan tidak berlaku", "Tautan sudah dipakai atau kedaluwarsa. Minta tautan baru."},
	CodeForbidden:               {http.StatusForbidden, "Akses ditolak", "Anda tidak memiliki akses untuk tindakan ini."},
	CodeNotFound:                {http.StatusNotFound, "Tidak ditemukan", "Data tidak ditemukan."},
	CodeVersionConflict:         {http.StatusConflict, "Data sudah berubah", "Data sudah diubah oleh pengguna lain. Muat ulang untuk melihat perubahan terbaru."},
	CodeInvalidStateTransition:  {http.StatusConflict, "Tindakan tidak dapat dilakukan", "Status dokumen tidak memungkinkan tindakan ini."},
	CodeIdempotencyKeyRequired:  {http.StatusBadRequest, "Terjadi kesalahan", "Terjadi kesalahan. Muat ulang halaman."},
	CodeIdempotencyInProgress:   {http.StatusConflict, "Masih diproses", "Permintaan sebelumnya masih diproses. Tunggu sebentar."},
	CodeIdempotencyKeyReused:    {http.StatusUnprocessableEntity, "Terjadi kesalahan", "Terjadi kesalahan. Muat ulang halaman."},
	CodeDuplicateCode:           {http.StatusUnprocessableEntity, "Kode sudah dipakai", "Kode sudah dipakai."},
	CodePeriodLocked:            {http.StatusUnprocessableEntity, "Periode sudah ditutup", "Periode sudah ditutup. Transaksi tidak bisa dibuat atau diubah."},
	CodeStockInsufficient:       {http.StatusUnprocessableEntity, "Stok tidak cukup", "Stok tidak cukup."},
	CodeBatchShelfLifeTooShort:  {http.StatusUnprocessableEntity, "Umur simpan tidak cukup", "Sisa umur simpan batch kurang dari batas channel."},
	CodeApprovalRequired:        {http.StatusUnprocessableEntity, "Membutuhkan persetujuan", "Tindakan ini membutuhkan persetujuan. Dokumen sudah diajukan untuk persetujuan."},
	CodeDiscountLimitExceeded:   {http.StatusUnprocessableEntity, "Diskon melebihi batas", "Diskon melebihi batas role Anda dan membutuhkan persetujuan."},
	CodeCreditLimitExceeded:     {http.StatusUnprocessableEntity, "Batas kredit terlampaui", "Transaksi melebihi batas kredit pelanggan."},
	CodeThreeWayMatchFailed:     {http.StatusUnprocessableEntity, "Tagihan tidak cocok", "Tagihan tidak cocok dengan PO dan penerimaan barang melebihi toleransi."},
	CodeDepositInsufficient:     {http.StatusUnprocessableEntity, "Deposit tidak cukup", "Saldo deposit reseller tidak cukup."},
	CodeOutsideAttendanceRadius: {http.StatusUnprocessableEntity, "Di luar radius", "Lokasi Anda di luar radius lokasi kerja."},
	CodeFileTooLarge:            {http.StatusUnprocessableEntity, "File terlalu besar", "Ukuran file tidak diizinkan."},
	CodeFileTypeNotAllowed:      {http.StatusUnprocessableEntity, "Tipe file tidak diizinkan", "Tipe file tidak diizinkan."},
	CodeImportAlreadyProcessed:  {http.StatusConflict, "File sudah diimpor", "File ini sudah pernah diimpor."},
	CodeRateLimited:             {http.StatusTooManyRequests, "Terlalu banyak permintaan", "Terlalu banyak permintaan. Coba lagi sebentar lagi."},
	CodeProviderUnavailable:     {http.StatusServiceUnavailable, "Layanan tidak tersedia", "Layanan pihak ketiga sedang tidak tersedia."},
	CodeBadRequest:              {http.StatusBadRequest, "Permintaan tidak valid", "Permintaan tidak dapat dibaca. Muat ulang halaman."},
	CodeInternal:                {http.StatusInternalServerError, "Terjadi kesalahan", "Terjadi kesalahan pada sistem."},
}

// HTTPStatus mengembalikan status HTTP untuk kode (Technical Guideline 8.4).
func (c Code) HTTPStatus() int {
	if spec, ok := codeSpecs[c]; ok {
		return spec.status
	}
	return http.StatusInternalServerError
}

// Title mengembalikan judul singkat Bahasa Indonesia untuk kode.
func (c Code) Title() string {
	if spec, ok := codeSpecs[c]; ok {
		return spec.title
	}
	return codeSpecs[CodeInternal].title
}

// DefaultMessage mengembalikan pesan default Bahasa Indonesia untuk kode.
func (c Code) DefaultMessage() string {
	if spec, ok := codeSpecs[c]; ok {
		return spec.message
	}
	return codeSpecs[CodeInternal].message
}

// Codes mengembalikan seluruh kode yang terdaftar.
func Codes() []Code {
	codes := make([]Code, 0, len(codeSpecs))
	for c := range codeSpecs {
		codes = append(codes, c)
	}
	return codes
}
