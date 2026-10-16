package pii_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/pii"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(9))
	require.NoError(t, err)

	enc, err := c.Encrypt("3201010101010001")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(enc, "k1:"))
	assert.NotContains(t, enc, "3201010101010001")

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "3201010101010001", dec)

	again, err := c.Encrypt("3201010101010001")
	require.NoError(t, err)
	assert.NotEqual(t, enc, again, "nonce acak")
}

func TestRotationKeepsOldDataReadable(t *testing.T) {
	old, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(9))
	require.NoError(t, err)
	enc, err := old.Encrypt("rahasia")
	require.NoError(t, err)

	rotated, err := pii.New("k2", map[string][]byte{"k1": key(1), "k2": key(2)}, key(9))
	require.NoError(t, err)
	dec, err := rotated.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "rahasia", dec)

	fresh, err := rotated.Encrypt("baru")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(fresh, "k2:"))

	_, err = old.Decrypt(fresh)
	assert.ErrorIs(t, err, pii.ErrUnknownKey)
}

func TestDecryptRejectsTampering(t *testing.T) {
	c, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(9))
	require.NoError(t, err)
	enc, err := c.Encrypt("nilai")
	require.NoError(t, err)

	_, err = c.Decrypt(enc[:len(enc)-4] + "AAAA")
	assert.Error(t, err)
	_, err = c.Decrypt("tanpa-pemisah")
	assert.Error(t, err)
	_, err = c.Decrypt("k1:" + base64.StdEncoding.EncodeToString([]byte("x")))
	assert.Error(t, err)
}

func TestEmptyValues(t *testing.T) {
	c, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(9))
	require.NoError(t, err)
	enc, err := c.Encrypt("")
	require.NoError(t, err)
	assert.Empty(t, enc)
	dec, err := c.Decrypt("")
	require.NoError(t, err)
	assert.Empty(t, dec)
}

func TestHashIsDeterministicAndKeyed(t *testing.T) {
	a, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(9))
	require.NoError(t, err)
	b, err := pii.New("k1", map[string][]byte{"k1": key(1)}, key(8))
	require.NoError(t, err)

	assert.Equal(t, a.Hash("user@balans.id"), a.Hash("user@balans.id"))
	assert.NotEqual(t, a.Hash("user@balans.id"), b.Hash("user@balans.id"))
}

func TestNewValidatesKeys(t *testing.T) {
	_, err := pii.New("k9", map[string][]byte{"k1": key(1)}, key(9))
	assert.Error(t, err)
	_, err = pii.New("k1", map[string][]byte{"k1": key(1)[:16]}, key(9))
	assert.Error(t, err)
	_, err = pii.New("k1", map[string][]byte{"k1": key(1)}, key(9)[:8])
	assert.Error(t, err)
}

func TestParseKeyRing(t *testing.T) {
	s := "k1:" + base64.StdEncoding.EncodeToString(key(1)) + ", k2:" + base64.StdEncoding.EncodeToString(key(2))
	keys, err := pii.ParseKeyRing(s)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, key(2), keys["k2"])

	_, err = pii.ParseKeyRing("tanpa-kid")
	assert.Error(t, err)
	_, err = pii.ParseKeyRing("k1:bukan-base64!")
	assert.Error(t, err)
}

func TestMask(t *testing.T) {
	assert.Equal(t, "••••••7890", pii.Mask("1234567890"))
	assert.Equal(t, "•••", pii.Mask("123"))
}
