// Package app menyusun seluruh modul (dependency injection manual, Technical Guideline 5.1).
package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/filestore"
	"balans/internal/hris"
	"balans/internal/identity"
	"balans/internal/integration/mockconsole"
	"balans/internal/integration/mockpayment"
	"balans/internal/integration/mockshipment"
	"balans/internal/integration/smtpmail"
	"balans/internal/inventory"
	"balans/internal/logistics"
	"balans/internal/notification"
	"balans/internal/party"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/idempotency"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/pii"
	"balans/internal/platform/webhook"
	"balans/internal/sales"
	"balans/internal/system"
	"balans/internal/warehouse"
)

// App berisi dependensi bersama dan modul yang aktif.
type App struct {
	Config    config.Config
	Log       *slog.Logger
	Pool      *pgxpool.Pool
	Clock     clock.Clock
	Tx        *db.TxManager
	Jobs      *jobqueue.Inserter
	Responder *httpx.Responder
	Migrator  *db.Migrator

	idempotency *idempotency.Store
	system      *system.Module
	audit       *audit.Module
	identity    *identity.Module
	approval    *approval.Module
	document    *document.Module
	accounting  *accounting.Module
	catalog     *catalog.Module
	warehouse   *warehouse.Module
	inventory   *inventory.Module
	filestore   *filestore.Module
	party       *party.Module
	hris        *hris.Module
	cipher      *pii.Cipher
	modules     []module
	console     *mockconsole.Console
}

// New membuka koneksi database dan menyusun modul sesuai ENABLED_MODULES.
func New(ctx context.Context, cfg config.Config, log *slog.Logger, report httpx.ErrorReporter) (*App, error) {
	if err := validateModules(cfg.EnabledModules); err != nil {
		return nil, err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{
		Config:      cfg,
		Log:         log,
		Pool:        pool,
		Clock:       clock.System{},
		Tx:          db.NewTxManager(pool),
		Responder:   httpx.NewResponder(log, report),
		idempotency: idempotency.NewStore(pool),
	}
	if a.Jobs, err = jobqueue.NewInserter(pool); err != nil {
		pool.Close()
		return nil, err
	}
	if a.Migrator, err = db.NewMigrator(pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := a.buildModules(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// Close menutup koneksi.
func (a *App) Close() {
	if a.Migrator != nil {
		if err := a.Migrator.Close(); err != nil {
			a.Log.Warn("close migrator", "error", err)
		}
	}
	a.Pool.Close()
}

func (a *App) buildModules() error {
	cfg := a.Config
	a.system = system.New(system.Deps{
		Build: system.Build{
			Version:        config.Version,
			Commit:         config.Commit,
			Environment:    string(cfg.Env),
			EnabledModules: cfg.EnabledModules,
			MockActive:     cfg.MockActive(),
		},
		Pool:      a.Pool,
		Schema:    a.Migrator,
		Clock:     a.Clock,
		Responder: a.Responder,
	})

	inbox := webhook.NewInbox(a.Pool, a.Tx, a.Jobs, a.Clock)
	console := mockconsole.Deps{Clock: a.Clock, Log: a.Log}

	// Audit trail dipakai semua modul, sehingga selalu disusun; route-nya mengikuti ENABLED_MODULES.
	a.audit = audit.New(audit.Deps{Pool: a.Pool, Responder: a.Responder})
	if cfg.IsEnabled(audit.Name) {
		a.modules = append(a.modules, a.audit)
	}

	var mailer *notification.Module
	if cfg.IsEnabled(notification.Name) {
		if cfg.EmailProvider != config.ProviderMock {
			return providerNotAvailable("EMAIL_PROVIDER", cfg.EmailProvider)
		}
		mailer = notification.New(notification.Deps{
			Pool: a.Pool, Tx: a.Tx, Jobs: a.Jobs, Clock: a.Clock, Log: a.Log,
			Sender:    smtpmail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.MailFrom),
			Responder: a.Responder,
		})
		a.modules = append(a.modules, mailer)
		console.Email = mailer
	}
	if cfg.IsEnabled(identity.Name) {
		if mailer == nil {
			return errors.New("modul identity membutuhkan modul notification untuk email undangan")
		}
		cipher, err := a.piiCipher()
		if err != nil {
			return err
		}
		a.identity, err = identity.New(identity.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit,
			Mailer: mailer, Cipher: cipher, BaseURL: cfg.PublicBaseURL, SecureCookie: cfg.Env != config.EnvLocal,
			SkipMFA: cfg.DevSkipMFA && cfg.Env == config.EnvLocal,
			Keys:    identity.KeyConfig{SigningKeyID: cfg.JWTSigningKeyID, SigningKey: cfg.JWTSigningKey, VerifyKeys: cfg.JWTVerifyKeys},
		})
		if err != nil {
			return err
		}
		a.modules = append(a.modules, a.identity)
	}
	if cfg.IsEnabled(approval.Name) {
		if a.identity == nil || mailer == nil {
			return errors.New("modul approval membutuhkan modul identity dan notification")
		}
		a.approval = approval.New(approval.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit,
			Directory: approvalDirectory{identity: a.identity}, Notifier: mailer, BaseURL: cfg.PublicBaseURL,
		})
		a.modules = append(a.modules, a.approval)
		if cfg.Env == config.EnvLocal {
			registerDevApprovalHandlers(a.approval, a.Log)
		}
	}
	if cfg.IsEnabled(document.Name) {
		a.document = document.New(document.Deps{Pool: a.Pool, Clock: a.Clock, Responder: a.Responder})
		a.modules = append(a.modules, a.document)
	}
	if cfg.IsEnabled(accounting.Name) {
		if a.document == nil {
			return errors.New("modul accounting membutuhkan modul document untuk nomor jurnal JV")
		}
		a.accounting = accounting.New(accounting.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit,
			Numberer: journalNumberer{documents: a.document},
		})
		a.modules = append(a.modules, a.accounting)
		registerAccountingAuditEntities(a.audit)
	}
	if cfg.IsEnabled(catalog.Name) {
		a.catalog = catalog.New(catalog.Deps{Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Responder: a.Responder, Audit: a.audit})
		a.modules = append(a.modules, a.catalog)
		registerAuditEntities(a.audit, catalog.AuditEntities())
	}
	if cfg.IsEnabled(warehouse.Name) {
		a.warehouse = warehouse.New(warehouse.Deps{Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Responder: a.Responder, Audit: a.audit})
		a.modules = append(a.modules, a.warehouse)
		registerAuditEntities(a.audit, warehouse.AuditEntities())
	}
	if err := a.buildMasterData(); err != nil {
		return err
	}
	if cfg.IsEnabled(inventory.Name) {
		if a.catalog == nil || a.warehouse == nil {
			return errors.New("modul inventory membutuhkan modul catalog dan warehouse")
		}
		d := inventory.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder,
			Products: a.catalog, Locations: a.warehouse,
		}
		if a.accounting != nil {
			d.Periods = a.accounting
		}
		a.inventory = inventory.New(d)
		a.modules = append(a.modules, a.inventory)
	}
	if cfg.IsEnabled(sales.Name) {
		if cfg.PaymentGateway != config.ProviderMock {
			return providerNotAvailable("PAYMENT_GATEWAY", cfg.PaymentGateway)
		}
		gateway := mockpayment.New(a.Pool, mockpayment.Config{
			ServerKey:     cfg.MidtransServerKey,
			PublicBaseURL: cfg.PublicBaseURL,
			WebhookURL:    cfg.InternalAPIURL + "/api/v1/webhooks/midtrans",
		})
		a.modules = append(a.modules, sales.New(sales.Deps{
			Pool: a.Pool, Tx: a.Tx, Audit: a.audit, Gateway: gateway, Inbox: inbox, Responder: a.Responder, Log: a.Log,
		}))
		console.Payments = gateway
		a.audit.RegisterEntity("return_reason", "sales.return_reason.read")
	}
	if cfg.IsEnabled(logistics.Name) {
		if cfg.ShipmentProvider != config.ProviderMock {
			return providerNotAvailable("SHIPMENT_PROVIDER", cfg.ShipmentProvider)
		}
		provider := mockshipment.New(a.Pool, mockshipment.Config{
			WebhookSecret: cfg.BiteshipWebhookSecret,
			WebhookURL:    cfg.InternalAPIURL + "/api/v1/webhooks/biteship",
		})
		a.modules = append(a.modules, logistics.New(logistics.Deps{Provider: provider, Inbox: inbox, Responder: a.Responder, Log: a.Log}))
		console.Shipments = provider
	}

	if cfg.MockActive() && cfg.Env != config.EnvProduction && (console.Email != nil || console.Payments != nil || console.Shipments != nil) {
		a.console = mockconsole.New(console)
	}
	return nil
}

// Start menjalankan inisialisasi proses api: sinkron katalog permission dan undangan Admin Sistem pertama.
func (a *App) Start(ctx context.Context) error {
	if a.identity == nil {
		return nil
	}
	return a.identity.Start(ctx, a.Config.InitialAdminEmail)
}

// SeedDev mengisi data pengembangan lokal. Ditolak di luar APP_ENV=local.
func (a *App) SeedDev(ctx context.Context, password string) ([]identity.DevUser, error) {
	if a.Config.Env != config.EnvLocal {
		return nil, errors.New("seed pengembangan hanya untuk APP_ENV=local")
	}
	if a.identity == nil {
		return nil, errors.New("seed pengembangan membutuhkan modul identity aktif")
	}
	users, err := a.identity.SeedDevUsers(ctx, password)
	if err != nil {
		return nil, err
	}
	if err := a.seedDevMasters(ctx); err != nil {
		return nil, err
	}
	n, err := a.seedDevApprovals(ctx)
	if err != nil {
		return nil, err
	}
	a.Log.InfoContext(ctx, "dev approvals seeded", "count", n)
	return users, nil
}

// registerAuditEntities memberi pemegang permission baca entitas hak melihat tab Riwayat entitas itu.
func registerAuditEntities(m *audit.Module, entities map[string]string) {
	for entityType, readPermission := range entities {
		m.RegisterEntity(entityType, readPermission)
	}
}

// seedDevMasters mengisi Gudang Utama dan produk contoh untuk demo lokal.
func (a *App) seedDevMasters(ctx context.Context) error {
	if a.warehouse != nil {
		if err := a.warehouse.SeedDev(ctx); err != nil {
			return err
		}
	}
	if a.catalog != nil {
		if err := a.catalog.SeedDev(ctx); err != nil {
			return err
		}
	}
	if a.inventory != nil && a.catalog != nil && a.warehouse != nil {
		return a.seedDevStock(ctx)
	}
	return nil
}

// piiCipher membuat Cipher data pribadi sekali dan memakainya bersama (identity, party, hris, filestore).
func (a *App) piiCipher() (*pii.Cipher, error) {
	if a.cipher != nil {
		return a.cipher, nil
	}
	c, err := newCipher(a.Config)
	if err != nil {
		return nil, err
	}
	a.cipher = c
	return c, nil
}

// buildMasterData menyusun filestore, party, dan hris (R1.1). party (tingkat 0) membaca karyawan dari hris
// (tingkat 1) lewat adapter yang dipasang setelah hris disusun.
func (a *App) buildMasterData() error {
	cfg := a.Config
	if cfg.IsEnabled(filestore.Name) {
		cipher, err := a.piiCipher()
		if err != nil {
			return err
		}
		a.filestore, err = filestore.New(filestore.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Dir: cfg.FilestoreDir,
			// Tanda tangan URL file memakai kunci HMAC PII dengan awalan khusus agar terpisah dari hash data pribadi.
			Sign: func(msg string) []byte { return cipher.Hash("filestore-url:" + msg) },
		})
		if err != nil {
			return err
		}
		a.modules = append(a.modules, a.filestore)
	}
	if cfg.IsEnabled(party.Name) {
		if a.identity == nil || a.filestore == nil {
			return errors.New("modul party membutuhkan modul identity dan filestore")
		}
		cipher, err := a.piiCipher()
		if err != nil {
			return err
		}
		a.party = party.New(party.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit, Cipher: cipher,
			Users: partyUsers{identity: a.identity}, Files: a.filestore,
		})
		a.party.RegisterAuditEntities(a.audit)
		a.filestore.RegisterOwner(filestore.OwnerPolicy{
			OwnerType: party.FileOwnerAgreement, WritePermission: party.PermAgreementWrite,
			ReadPermission: party.PermAgreementRead, ContentTypes: []string{filestore.TypePDF}, MaxBytes: maxAgreementFileBytes,
		})
		a.modules = append(a.modules, a.party)
	}
	if cfg.IsEnabled(hris.Name) {
		cipher, err := a.piiCipher()
		if err != nil {
			return err
		}
		a.hris = hris.New(hris.Deps{
			Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Log: a.Log, Responder: a.Responder, Audit: a.audit, Cipher: cipher,
		})
		a.hris.RegisterAuditEntities(a.audit)
		a.modules = append(a.modules, a.hris)
		if a.party != nil {
			a.party.SetEmployees(partyEmployees{hris: a.hris})
		}
	}
	return nil
}

// maxAgreementFileBytes adalah batas ukuran PDF PKS. Batas teknis upload, bukan nilai bisnis.
const maxAgreementFileBytes = 10 << 20

// newCipher membaca kunci PII. Di APP_ENV=local tanpa kunci dipakai kunci pengembangan deterministik;
// config.Validate mewajibkan kunci asli di staging dan production.
func newCipher(cfg config.Config) (*pii.Cipher, error) {
	if cfg.PIIEncryptionKeys == "" && cfg.Env == config.EnvLocal {
		key := sha256.Sum256([]byte("balans-local-development-pii"))
		hash := sha256.Sum256([]byte("balans-local-development-pii-hash"))
		return pii.New("local-dev", map[string][]byte{"local-dev": key[:]}, hash[:])
	}
	keys, err := pii.ParseKeyRing(cfg.PIIEncryptionKeys)
	if err != nil {
		return nil, fmt.Errorf("PII_ENCRYPTION_KEYS: %w", err)
	}
	hashKey, err := decodeBase64(cfg.PIIHashKey)
	if err != nil {
		return nil, fmt.Errorf("PII_HASH_KEY: %w", err)
	}
	return pii.New(cfg.PIIEncryptionKeyID, keys, hashKey)
}

// errProviderNotAvailable dipakai sampai adapter provider asli dibangun.
var errProviderNotAvailable = errors.New("adapter provider asli belum tersedia")

func providerNotAvailable(env, value string) error {
	return fmt.Errorf("%s=%q: %w; gunakan %q", env, value, errProviderNotAvailable, config.ProviderMock)
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
