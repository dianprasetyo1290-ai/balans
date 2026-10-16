package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"balans/internal/identity/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// AccessTokenTTL adalah umur access token (Technical Guideline 9.1).
const AccessTokenTTL = 15 * time.Minute

// Claims adalah isi access token. Permission tidak disimpan di token.
type Claims struct {
	jwt.RegisteredClaims
	SessionID    uuid.UUID `json:"sid"`
	TokenVersion int       `json:"ver"`
	ReauthAt     int64     `json:"reauth_at,omitempty"`
}

// TokenIssuer menandatangani dan memverifikasi access token EdDSA dengan header kid.
type TokenIssuer struct {
	signKID string
	signKey ed25519.PrivateKey
	verify  map[string]ed25519.PublicKey
	clock   clock.Clock
	parser  *jwt.Parser
}

// KeyConfig berisi kunci dalam format env Runbook 3.1.
type KeyConfig struct {
	SigningKeyID string
	SigningKey   string // PEM PKCS#8 dalam base64 satu baris
	VerifyKeys   string // kid:base64(PEM publik),...
}

// NewTokenIssuer membaca kunci dari konfigurasi.
func NewTokenIssuer(cfg KeyConfig, c clock.Clock) (*TokenIssuer, error) {
	priv, err := parsePrivateKey(cfg.SigningKey)
	if err != nil {
		return nil, err
	}
	verify := map[string]ed25519.PublicKey{}
	for _, part := range strings.Split(cfg.VerifyKeys, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kid, encoded, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("JWT_VERIFY_KEYS: entri %q harus kid:base64", part)
		}
		pub, err := parsePublicKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("JWT_VERIFY_KEYS %s: %w", kid, err)
		}
		verify[kid] = pub
	}
	if _, ok := verify[cfg.SigningKeyID]; !ok {
		return nil, fmt.Errorf("JWT_VERIFY_KEYS harus memuat kunci publik %q", cfg.SigningKeyID)
	}
	return newIssuer(cfg.SigningKeyID, priv, verify, c), nil
}

// NewDevTokenIssuer membuat kunci deterministik untuk APP_ENV=local, agar token tetap sah setelah restart.
// DILARANG dipakai di staging/production (config.Validate mewajibkan kunci di sana).
func NewDevTokenIssuer(c clock.Clock) *TokenIssuer {
	seed := sha256.Sum256([]byte("balans-local-development-jwt"))
	priv := ed25519.NewKeyFromSeed(seed[:])
	return newIssuer("local-dev", priv, map[string]ed25519.PublicKey{"local-dev": priv.Public().(ed25519.PublicKey)}, c)
}

func newIssuer(kid string, priv ed25519.PrivateKey, verify map[string]ed25519.PublicKey, c clock.Clock) *TokenIssuer {
	return &TokenIssuer{
		signKID: kid, signKey: priv, verify: verify, clock: c,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
			jwt.WithAudience(string(domain.AudienceBackoffice)),
			jwt.WithTimeFunc(c.Now),
			jwt.WithExpirationRequired(),
		),
	}
}

func decodePEM(encoded string) (*pem.Block, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("PEM tidak valid")
	}
	return block, nil
}

func parsePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	block, err := decodePEM(encoded)
	if err != nil {
		return nil, fmt.Errorf("JWT_SIGNING_KEY: %w", err)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("JWT_SIGNING_KEY: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("JWT_SIGNING_KEY harus kunci Ed25519")
	}
	return priv, nil
}

func parsePublicKey(encoded string) (ed25519.PublicKey, error) {
	block, err := decodePEM(encoded)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("kunci publik harus Ed25519")
	}
	return pub, nil
}

// Issue membuat access token untuk sesi.
func (t *TokenIssuer) Issue(userID, sessionID uuid.UUID, tokenVersion int, reauthAt time.Time) (string, time.Time, error) {
	now := t.clock.Now()
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{string(domain.AudienceBackoffice)},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.Must(uuid.NewV7()).String(),
		},
		SessionID:    sessionID,
		TokenVersion: tokenVersion,
	}
	if !reauthAt.IsZero() {
		claims.ReauthAt = reauthAt.Unix()
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = t.signKID
	signed, err := tok.SignedString(t.signKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, exp, nil
}

// Parse memverifikasi access token. Token kedaluwarsa → AUTH_TOKEN_EXPIRED (FE refresh otomatis);
// token tidak sah → AUTH_SESSION_EXPIRED.
func (t *TokenIssuer) Parse(raw string) (Claims, error) {
	var claims Claims
	_, err := t.parser.ParseWithClaims(raw, &claims, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		key, ok := t.verify[kid]
		if !ok {
			return nil, fmt.Errorf("unknown kid %q", kid)
		}
		return key, nil
	})
	if errors.Is(err, jwt.ErrTokenExpired) {
		return Claims{}, apperr.Wrap(apperr.CodeAuthTokenExpired, err)
	}
	if err != nil {
		return Claims{}, apperr.Wrap(apperr.CodeAuthSessionExpired, err)
	}
	return claims, nil
}

// newOpaqueToken membuat token acak (refresh token, tautan password) dan hash SHA-256 untuk disimpan.
func newOpaqueToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("random token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashOpaque(token), nil
}

func hashOpaque(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
