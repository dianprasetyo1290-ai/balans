// Package config membaca konfigurasi teknis dari env. Nilai bisnis DILARANG disimpan di sini (Technical Guideline 5.18).
package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Version dan Commit diisi saat build lewat -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
)

// Environment adalah lingkungan aplikasi (Technical Guideline 14.2).
type Environment string

const (
	EnvLocal      Environment = "local"
	EnvStaging    Environment = "staging"
	EnvProduction Environment = "production"
)

// ProviderMock adalah nilai env untuk adapter mock (Technical Guideline 5.10).
const ProviderMock = "mock"

// Config adalah seluruh konfigurasi teknis proses.
type Config struct {
	Env            Environment
	HTTPAddr       string
	DatabaseURL    string
	LogLevel       string
	EnabledModules []string
	SentryDSN      string
	// PublicBaseURL adalah URL ERP yang dilihat browser, untuk tautan halaman simulasi mock.
	PublicBaseURL string
	// InternalAPIURL adalah alamat api dari dalam jaringan Compose, tujuan webhook dari mock.
	InternalAPIURL string

	EmailProvider    string
	ShipmentProvider string
	PaymentGateway   string

	SMTPHost string
	SMTPPort int
	MailFrom string

	MidtransServerKey     string
	BiteshipWebhookSecret string

	// Kunci JWT Ed25519 (Runbook 3.1): PEM base64 satu baris; JWTVerifyKeys berformat `kid:base64,kid2:base64`.
	JWTSigningKeyID string
	JWTSigningKey   string
	JWTVerifyKeys   string
	// Kunci data pribadi AES-256-GCM dan HMAC (Runbook 3.2).
	PIIEncryptionKeyID string
	PIIEncryptionKeys  string
	PIIHashKey         string
	// InitialAdminEmail mendapat undangan Admin Sistem saat database belum punya user sama sekali.
	InitialAdminEmail string
	// DevSkipMFA mematikan kewajiban MFA untuk pengembangan lokal. Ditolak di luar APP_ENV=local.
	DevSkipMFA bool

	// FilestoreDir adalah folder penyimpanan file modul filestore (disk lokal sampai adapter S3 dibangun).
	FilestoreDir string

	ShutdownTimeout time.Duration
}

// Load membaca env dan memvalidasinya.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	env := func(key, fallback string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return fallback
	}

	smtpPort, err := strconv.Atoi(env("SMTP_PORT", "1025"))
	if err != nil {
		return Config{}, fmt.Errorf("SMTP_PORT: %w", err)
	}
	shutdown, err := time.ParseDuration(env("SHUTDOWN_TIMEOUT", "20s"))
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}

	cfg := Config{
		Env:                   Environment(env("APP_ENV", string(EnvLocal))),
		HTTPAddr:              env("HTTP_ADDR", ":8080"),
		DatabaseURL:           env("DATABASE_URL", ""),
		LogLevel:              env("LOG_LEVEL", "info"),
		EnabledModules:        ParseModules(env("ENABLED_MODULES", "")),
		SentryDSN:             env("SENTRY_DSN", ""),
		PublicBaseURL:         strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:5173"), "/"),
		InternalAPIURL:        strings.TrimRight(env("INTERNAL_API_URL", "http://localhost:8080"), "/"),
		EmailProvider:         env("EMAIL_PROVIDER", ProviderMock),
		ShipmentProvider:      env("SHIPMENT_PROVIDER", ProviderMock),
		PaymentGateway:        env("PAYMENT_GATEWAY", ProviderMock),
		SMTPHost:              env("SMTP_HOST", "localhost"),
		SMTPPort:              smtpPort,
		MailFrom:              env("MAIL_FROM", "Balans ERP <no-reply@findyourbalans.com>"),
		MidtransServerKey:     env("MIDTRANS_SERVER_KEY", ""),
		BiteshipWebhookSecret: env("BITESHIP_WEBHOOK_SECRET", ""),
		JWTSigningKeyID:       env("JWT_SIGNING_KEY_ID", ""),
		JWTSigningKey:         env("JWT_SIGNING_KEY", ""),
		JWTVerifyKeys:         env("JWT_VERIFY_KEYS", ""),
		PIIEncryptionKeyID:    env("PII_ENCRYPTION_KEY_ID", ""),
		PIIEncryptionKeys:     env("PII_ENCRYPTION_KEYS", ""),
		PIIHashKey:            env("PII_HASH_KEY", ""),
		InitialAdminEmail:     env("INITIAL_ADMIN_EMAIL", ""),
		DevSkipMFA:            env("DEV_SKIP_MFA", "") == "true",
		FilestoreDir:          env("FILESTORE_DIR", "./data/files"),
		ShutdownTimeout:       shutdown,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ParseModules membaca daftar modul dipisah koma, tanpa duplikat, huruf kecil.
func ParseModules(s string) []string {
	var modules []string
	for _, m := range strings.Split(s, ",") {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" && !slices.Contains(modules, m) {
			modules = append(modules, m)
		}
	}
	return modules
}

// IsEnabled memeriksa apakah modul aktif (Technical Guideline 14.5).
func (c Config) IsEnabled(module string) bool {
	return slices.Contains(c.EnabledModules, module)
}

// MockActive bernilai true jika minimal satu adapter memakai mock. Route /_mock/* hanya didaftarkan saat ini true.
func (c Config) MockActive() bool {
	return c.EmailProvider == ProviderMock || c.ShipmentProvider == ProviderMock || c.PaymentGateway == ProviderMock
}

// mockUsers memetakan provider ke modul yang memakainya.
func (c Config) mockUsers() map[string]string {
	return map[string]string{
		"notification": c.EmailProvider,
		"logistics":    c.ShipmentProvider,
		"sales":        c.PaymentGateway,
	}
}

// Validate menolak konfigurasi yang tidak aman. Production DILARANG memakai mock untuk modul aktif.
func (c Config) Validate() error {
	var errs []error
	switch c.Env {
	case EnvLocal, EnvStaging, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV %q tidak dikenal", c.Env))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL wajib diisi"))
	}
	if c.Env == EnvProduction {
		for module, provider := range c.mockUsers() {
			if c.IsEnabled(module) && provider == ProviderMock {
				errs = append(errs, fmt.Errorf("modul %s aktif di production tetapi provider masih mock", module))
			}
		}
	}
	if c.DevSkipMFA && c.Env != EnvLocal {
		errs = append(errs, errors.New("DEV_SKIP_MFA hanya boleh dipakai di APP_ENV=local"))
	}
	if c.Env != EnvLocal && c.IsEnabled("identity") {
		if c.JWTSigningKeyID == "" || c.JWTSigningKey == "" || c.JWTVerifyKeys == "" {
			errs = append(errs, errors.New("JWT_SIGNING_KEY_ID, JWT_SIGNING_KEY, dan JWT_VERIFY_KEYS wajib diisi (Runbook 3.1)"))
		}
		if c.PIIEncryptionKeyID == "" || c.PIIEncryptionKeys == "" || c.PIIHashKey == "" {
			errs = append(errs, errors.New("PII_ENCRYPTION_KEY_ID, PII_ENCRYPTION_KEYS, dan PII_HASH_KEY wajib diisi (Runbook 3.2)"))
		}
	}
	if c.PaymentGateway == ProviderMock && c.MidtransServerKey == "" && c.Env != EnvLocal {
		errs = append(errs, errors.New("MIDTRANS_SERVER_KEY wajib diisi agar webhook mock bersignature"))
	}
	return errors.Join(errs...)
}
