// Package domain berisi aturan identitas: password, user, role, dan katalog permission.
package domain

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"balans/internal/platform/apperr"
)

// MinPasswordLength adalah panjang minimal password (Technical Guideline 9.1).
const MinPasswordLength = 10

// Parameter argon2id mengikuti rekomendasi OWASP (19 MiB, 2 iterasi, 1 thread).
const (
	argonTime    uint32 = 2
	argonMemory  uint32 = 19 * 1024
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// commonPasswords adalah daftar password paling umum (dicek setelah huruf kecil).
var commonPasswords = map[string]struct{}{
	"1234567890": {}, "12345678910": {}, "123456789a": {}, "1q2w3e4r5t": {}, "qwertyuiop": {},
	"password12": {}, "password123": {}, "passw0rd123": {}, "iloveyou123": {}, "admin12345": {},
	"administrator": {}, "welcome123": {}, "qwerty12345": {}, "abcdefghij": {}, "0987654321": {},
	"1111111111": {}, "0000000000": {}, "balans12345": {}, "balans123456": {}, "bismillah123": {},
	"indonesia123": {}, "jakarta123": {}, "password1234": {}, "rahasia123": {}, "katasandi123": {},
	"sayangku123": {}, "superman123": {}, "qwerty123456": {}, "asdfghjkl1": {}, "zxcvbnm123": {},
}

// ValidatePassword memeriksa kebijakan password. email dipakai agar password tidak sama dengan email.
func ValidatePassword(password, email string) error {
	msg := ""
	lower := strings.ToLower(password)
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	switch {
	case utf8.RuneCountInString(password) < MinPasswordLength:
		msg = fmt.Sprintf("Password minimal %d karakter", MinPasswordLength)
	case utf8.RuneCountInString(password) > 128:
		msg = "Password maksimal 128 karakter"
	case isCommon(lower):
		msg = "Password terlalu umum. Gunakan kombinasi lain"
	case local != "" && strings.Contains(lower, local):
		msg = "Password tidak boleh memuat email"
	}
	if msg != "" {
		return apperr.Validation(map[string]string{"password": msg})
	}
	return nil
}

func isCommon(lower string) bool {
	if _, ok := commonPasswords[lower]; ok {
		return true
	}
	// Satu karakter diulang (mis. "aaaaaaaaaa").
	return strings.Count(lower, lower[:1]) == len(lower)
}

// HashPassword membuat hash argon2id berformat PHC.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

var errMalformedHash = errors.New("malformed argon2id hash")

// VerifyPassword membandingkan password dengan hash dalam waktu konstan.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want))) //nolint:gosec // panjang hash 32 byte
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash dipakai saat email tidak terdaftar agar waktu respons login sama (mencegah enumerasi email).
var dummyHash = func() string {
	h, err := HashPassword("dummy-password-for-timing")
	if err != nil {
		panic(err)
	}
	return h
}()

// BurnPasswordCheck menghabiskan waktu yang sama dengan verifikasi sungguhan.
func BurnPasswordCheck(password string) {
	_, _ = VerifyPassword(password, dummyHash)
}

// NormalizeEmail menyeragamkan email untuk disimpan dan dicari.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
