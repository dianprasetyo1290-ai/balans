package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func genKey(t *testing.T) (priv, pub string) {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privDER, err := x509.MarshalPKCS8PrivateKey(sk)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(pk)
	require.NoError(t, err)
	enc := func(typ string, der []byte) string {
		return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
	}
	return enc("PRIVATE KEY", privDER), enc("PUBLIC KEY", pubDER)
}

func TestTokenIssueAndParse(t *testing.T) {
	c := clock.NewFixed(time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC))
	priv, pub := genKey(t)
	iss, err := NewTokenIssuer(KeyConfig{SigningKeyID: "k1", SigningKey: priv, VerifyKeys: "k1:" + pub}, c)
	require.NoError(t, err)

	user, sess := uuid.New(), uuid.New()
	tok, exp, err := iss.Issue(user, sess, 3, c.Now())
	require.NoError(t, err)
	assert.Equal(t, c.Now().Add(15*time.Minute), exp)

	claims, err := iss.Parse(tok)
	require.NoError(t, err)
	assert.Equal(t, user.String(), claims.Subject)
	assert.Equal(t, sess, claims.SessionID)
	assert.Equal(t, 3, claims.TokenVersion)
	assert.Equal(t, c.Now().Unix(), claims.ReauthAt)

	c.Advance(16 * time.Minute)
	_, err = iss.Parse(tok)
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthTokenExpired))
}

func TestTokenRotationAndForgery(t *testing.T) {
	c := clock.NewFixed(time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC))
	priv1, pub1 := genKey(t)
	priv2, pub2 := genKey(t)
	old, err := NewTokenIssuer(KeyConfig{SigningKeyID: "k1", SigningKey: priv1, VerifyKeys: "k1:" + pub1}, c)
	require.NoError(t, err)
	tok, _, err := old.Issue(uuid.New(), uuid.New(), 1, time.Time{})
	require.NoError(t, err)

	rotated, err := NewTokenIssuer(KeyConfig{SigningKeyID: "k2", SigningKey: priv2, VerifyKeys: "k1:" + pub1 + ",k2:" + pub2}, c)
	require.NoError(t, err)
	claims, err := rotated.Parse(tok)
	require.NoError(t, err, "token lama tetap sah selama kunci lama ada di daftar verifikasi")
	assert.Zero(t, claims.ReauthAt)

	onlyNew, err := NewTokenIssuer(KeyConfig{SigningKeyID: "k2", SigningKey: priv2, VerifyKeys: "k2:" + pub2}, c)
	require.NoError(t, err)
	_, err = onlyNew.Parse(tok)
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthSessionExpired))

	_, err = rotated.Parse(tok + "x")
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthSessionExpired))
	_, err = rotated.Parse("bukan.jwt.sah")
	assert.Error(t, err)
}

func TestTokenIssuerConfigErrors(t *testing.T) {
	c := clock.System{}
	priv, pub := genKey(t)
	_, err := NewTokenIssuer(KeyConfig{SigningKeyID: "k1", SigningKey: priv, VerifyKeys: "k2:" + pub}, c)
	assert.Error(t, err)
	_, err = NewTokenIssuer(KeyConfig{SigningKeyID: "k1", SigningKey: "bukan-base64!", VerifyKeys: "k1:" + pub}, c)
	assert.Error(t, err)
	_, err = NewTokenIssuer(KeyConfig{SigningKeyID: "k1", SigningKey: priv, VerifyKeys: "tanpa-kid"}, c)
	assert.Error(t, err)
}

func TestDevIssuerIsStableAcrossRestarts(t *testing.T) {
	c := clock.NewFixed(time.Now())
	tok, _, err := NewDevTokenIssuer(c).Issue(uuid.New(), uuid.New(), 1, time.Time{})
	require.NoError(t, err)
	_, err = NewDevTokenIssuer(c).Parse(tok)
	assert.NoError(t, err)
}

func TestOpaqueToken(t *testing.T) {
	a, ha, err := newOpaqueToken()
	require.NoError(t, err)
	b, _, err := newOpaqueToken()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.Equal(t, ha, hashOpaque(a))
	assert.Len(t, a, 43)
}
