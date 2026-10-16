// Package pii mengenkripsi data pribadi di level aplikasi dengan AES-256-GCM dan menghitung hash HMAC
// untuk pencarian persis (Technical Guideline 6.7, NFR-04). Ciphertext menyimpan key ID agar kunci bisa dirotasi.
package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownKey dikembalikan jika ciphertext memakai key ID yang tidak ada di key ring.
var ErrUnknownKey = errors.New("pii: unknown encryption key")

// Cipher mengenkripsi dan mendekripsi teks. Format ciphertext: `<keyID>:<base64(nonce||sealed)>`.
type Cipher struct {
	activeID string
	aeads    map[string]cipher.AEAD
	hashKey  []byte
}

// New membuat Cipher. keys berisi pasangan key ID → kunci 32 byte; activeID dipakai untuk enkripsi baru.
func New(activeID string, keys map[string][]byte, hashKey []byte) (*Cipher, error) {
	if _, ok := keys[activeID]; !ok {
		return nil, fmt.Errorf("pii: active key %q not in key ring", activeID)
	}
	if len(hashKey) < 32 {
		return nil, errors.New("pii: hash key must be at least 32 bytes")
	}
	c := &Cipher{activeID: activeID, aeads: make(map[string]cipher.AEAD, len(keys)), hashKey: hashKey}
	for id, key := range keys {
		if len(key) != 32 {
			return nil, fmt.Errorf("pii: key %q must be 32 bytes for AES-256", id)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("pii: key %q: %w", id, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("pii: key %q: %w", id, err)
		}
		c.aeads[id] = aead
	}
	return c, nil
}

// ParseKeyRing membaca format env `kid:base64,kid2:base64` (Runbook 3.2).
func ParseKeyRing(s string) (map[string][]byte, error) {
	keys := map[string][]byte{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, encoded, ok := strings.Cut(part, ":")
		if !ok || id == "" {
			return nil, fmt.Errorf("pii: key ring entry %q must be kid:base64", part)
		}
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("pii: decode key %q: %w", id, err)
		}
		keys[id] = key
	}
	return keys, nil
}

// Encrypt mengenkripsi plaintext dengan kunci aktif. Teks kosong tetap kosong.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead := c.aeads[c.activeID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("pii: nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), []byte(c.activeID))
	return c.activeID + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt membaca ciphertext dari kunci mana pun di key ring.
func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	id, encoded, ok := strings.Cut(ciphertext, ":")
	if !ok {
		return "", errors.New("pii: malformed ciphertext")
	}
	aead, ok := c.aeads[id]
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownKey, id)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("pii: decode ciphertext: %w", err)
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("pii: ciphertext too short")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(id))
	if err != nil {
		return "", fmt.Errorf("pii: open: %w", err)
	}
	return string(plain), nil
}

// Hash menghitung HMAC-SHA256 dari nilai yang sudah dinormalisasi pemanggil, untuk kolom `_hash`.
func (c *Cipher) Hash(value string) []byte {
	m := hmac.New(sha256.New, c.hashKey)
	m.Write([]byte(value))
	return m.Sum(nil)
}

// Mask menampilkan 4 karakter terakhir, mis. untuk rekening di layar dan audit trail.
func Mask(value string) string {
	r := []rune(value)
	if len(r) <= 4 {
		return strings.Repeat("•", len(r))
	}
	return strings.Repeat("•", len(r)-4) + string(r[len(r)-4:])
}
