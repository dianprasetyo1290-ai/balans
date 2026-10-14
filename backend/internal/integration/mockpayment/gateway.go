// Package mockpayment adalah adapter mock Midtrans (Technical Guideline 5.10, D-27). Hanya boleh aktif
// di local dan staging. Hasil simulasi dikirim sebagai webhook bersignature ke endpoint asli.
package mockpayment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/integration/midtrans"
	"balans/internal/integration/mockpayment/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/sales"
)

// Outcome adalah hasil yang dipilih di halaman simulasi.
type Outcome string

const (
	OutcomeSuccess Outcome = "settlement"
	OutcomeFailed  Outcome = "deny"
	OutcomeExpired Outcome = "expire"
)

// Payment adalah transaksi mock untuk halaman simulasi.
type Payment struct {
	OrderID       string
	TransactionID string
	GrossAmount   money.Amount
	Status        string
	CreatedAt     time.Time
}

// Config adalah konfigurasi gateway mock.
type Config struct {
	ServerKey     string
	PublicBaseURL string
	WebhookURL    string
}

// Gateway mengimplementasikan sales.PaymentGateway.
type Gateway struct {
	pool   *pgxpool.Pool
	cfg    Config
	client *http.Client
}

var _ sales.PaymentGateway = (*Gateway)(nil)

// New membuat gateway mock.
func New(pool *pgxpool.Pool, cfg Config) *Gateway {
	return &Gateway{pool: pool, cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (g *Gateway) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, g.pool))
}

// Provider mengembalikan nama provider yang ditiru.
func (g *Gateway) Provider() string { return midtrans.Provider }

// CreateTransaction membuat transaksi dan link bayar ke halaman simulasi.
func (g *Gateway) CreateTransaction(ctx context.Context, req sales.PaymentRequest) (sales.PaymentTransaction, error) {
	if !req.GrossAmount.IsPositive() {
		return sales.PaymentTransaction{}, apperr.Validation(map[string]string{"grossAmount": "Nominal harus lebih dari 0"})
	}
	row, err := g.q(ctx).UpsertMockPayment(ctx, sqlcgen.UpsertMockPaymentParams{
		OrderID:     req.OrderID,
		GrossAmount: req.GrossAmount.Round().Decimal(),
	})
	if err != nil {
		return sales.PaymentTransaction{}, fmt.Errorf("create mock payment: %w", err)
	}
	return sales.PaymentTransaction{
		TransactionID: row.TransactionID.String(),
		RedirectURL:   g.cfg.PublicBaseURL + "/_mock/payments/" + url.PathEscape(req.OrderID),
	}, nil
}

// Status mengembalikan status terkini, seperti API cek status Midtrans.
func (g *Gateway) Status(ctx context.Context, orderID string) (sales.PaymentStatus, error) {
	row, err := g.q(ctx).GetMockPayment(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("mock payment %s", orderID))
	}
	if err != nil {
		return "", fmt.Errorf("get mock payment: %w", err)
	}
	return midtrans.MapStatus(row.Status, ""), nil
}

// ParseNotification memakai verifikasi yang sama dengan adapter Midtrans asli.
func (g *Gateway) ParseNotification(_ context.Context, body []byte) (sales.PaymentNotification, error) {
	return midtrans.ParseNotification(g.cfg.ServerKey, body)
}

// DecodeNotification memakai decoder yang sama dengan adapter Midtrans asli.
func (g *Gateway) DecodeNotification(body []byte) (sales.PaymentNotification, error) {
	return midtrans.DecodeNotification(body)
}

// Get membaca satu transaksi mock.
func (g *Gateway) Get(ctx context.Context, orderID string) (Payment, error) {
	row, err := g.q(ctx).GetMockPayment(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("mock payment %s", orderID))
	}
	if err != nil {
		return Payment{}, fmt.Errorf("get mock payment: %w", err)
	}
	return toPayment(row), nil
}

// List membaca transaksi mock terbaru.
func (g *Gateway) List(ctx context.Context, limit int32) ([]Payment, error) {
	rows, err := g.q(ctx).ListMockPayments(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list mock payments: %w", err)
	}
	payments := make([]Payment, 0, len(rows))
	for _, r := range rows {
		payments = append(payments, toPayment(r))
	}
	return payments, nil
}

// Simulate mengubah status transaksi pending lalu mengirim webhook bersignature ke endpoint asli.
func (g *Gateway) Simulate(ctx context.Context, orderID string, outcome Outcome) error {
	switch outcome {
	case OutcomeSuccess, OutcomeFailed, OutcomeExpired:
	default:
		return apperr.Validation(map[string]string{"outcome": "Hasil simulasi tidak dikenal"})
	}
	row, err := g.q(ctx).SetMockPaymentStatus(ctx, sqlcgen.SetMockPaymentStatusParams{OrderID: orderID, Status: string(outcome)})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Transaksi tidak ditemukan atau sudah tidak berstatus pending.")
	}
	if err != nil {
		return fmt.Errorf("set mock payment status: %w", err)
	}
	return g.sendWebhook(ctx, row)
}

func (g *Gateway) sendWebhook(ctx context.Context, row sqlcgen.MockPayment) error {
	gross := money.FromDecimal(row.GrossAmount).String()
	statusCode := midtrans.StatusCode(row.Status)
	body, err := json.Marshal(midtrans.Notification{
		OrderID:           row.OrderID,
		StatusCode:        statusCode,
		GrossAmount:       gross,
		SignatureKey:      midtrans.Signature(row.OrderID, statusCode, gross, g.cfg.ServerKey),
		TransactionStatus: row.Status,
		TransactionID:     row.TransactionID.String(),
	})
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return apperr.Wrap(apperr.CodeProviderUnavailable, fmt.Errorf("send mock webhook: %w", err))
	}
	defer resp.Body.Close() //nolint:errcheck // body hanya dibuang
	if resp.StatusCode != http.StatusOK {
		return apperr.Wrap(apperr.CodeProviderUnavailable, fmt.Errorf("mock webhook rejected with status %d", resp.StatusCode))
	}
	return nil
}

func toPayment(r sqlcgen.MockPayment) Payment {
	return Payment{
		OrderID:       r.OrderID,
		TransactionID: r.TransactionID.String(),
		GrossAmount:   money.FromDecimal(r.GrossAmount),
		Status:        r.Status,
		CreatedAt:     r.CreatedAt,
	}
}
