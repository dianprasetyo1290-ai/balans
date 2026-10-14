// Package mockshipment adalah adapter mock Biteship (Technical Guideline 5.10, D-27). Hanya boleh aktif
// di local dan staging. Perubahan status dikirim sebagai webhook bersignature ke endpoint asli.
package mockshipment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/integration/biteship"
	"balans/internal/integration/mockshipment/sqlcgen"
	"balans/internal/logistics"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
)

// Statuses adalah status Biteship yang bisa disimulasikan, berurutan sesuai alur pengiriman.
var Statuses = []string{"picked", "dropping_off", "delivered", "returned", "cancelled"}

// tariff adalah tabel ongkir statis per kurir: biaya dasar per kg pertama dan per kg berikutnya.
type tariff struct {
	courier, service, eta string
	firstKg, nextKg       int64
}

var tariffs = []tariff{
	{"jne", "reg", "2-3 hari", 10000, 8000},
	{"jne", "yes", "1 hari", 18000, 15000},
	{"sicepat", "reg", "2-3 hari", 9000, 7000},
	{"jnt", "ez", "2-4 hari", 9500, 7500},
	{"anteraja", "reg", "2-3 hari", 9000, 7000},
}

// Shipment adalah order mock untuk halaman simulasi.
type Shipment struct {
	ReferenceID    string
	TrackingNumber string
	CourierCode    string
	ShippingCost   money.Amount
	Status         string
	CreatedAt      time.Time
}

// Config adalah konfigurasi provider mock.
type Config struct {
	WebhookSecret string
	WebhookURL    string
}

// Provider mengimplementasikan logistics.ShipmentProvider.
type Provider struct {
	pool   *pgxpool.Pool
	cfg    Config
	client *http.Client
}

var _ logistics.ShipmentProvider = (*Provider)(nil)

// New membuat provider mock.
func New(pool *pgxpool.Pool, cfg Config) *Provider {
	return &Provider{pool: pool, cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *Provider) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, p.pool))
}

// Provider mengembalikan nama provider yang ditiru.
func (p *Provider) Provider() string { return biteship.Provider }

// Rates menghitung ongkir dari tabel tarif statis.
func (p *Provider) Rates(_ context.Context, req logistics.RateRequest) ([]logistics.Rate, error) {
	if req.WeightGrams <= 0 {
		return nil, apperr.Validation(map[string]string{"weightGrams": "Berat harus lebih dari 0"})
	}
	kg := int64((req.WeightGrams + 999) / 1000)
	var rates []logistics.Rate
	for _, t := range tariffs {
		if len(req.CourierCodes) > 0 && !slices.Contains(req.CourierCodes, t.courier) {
			continue
		}
		rates = append(rates, logistics.Rate{
			CourierCode:  t.courier,
			ServiceCode:  t.service,
			ShippingCost: money.FromInt(t.firstKg + (kg-1)*t.nextKg),
			ETA:          t.eta,
		})
	}
	return rates, nil
}

// CreateOrder membuat order dengan resi MOCK-<reference_id>.
func (p *Provider) CreateOrder(ctx context.Context, order logistics.ShipmentOrder) (logistics.Shipment, error) {
	rates, err := p.Rates(ctx, logistics.RateRequest{WeightGrams: order.WeightGrams, CourierCodes: []string{order.CourierCode}})
	if err != nil {
		return logistics.Shipment{}, err
	}
	idx := slices.IndexFunc(rates, func(r logistics.Rate) bool { return r.ServiceCode == order.ServiceCode })
	if idx < 0 {
		return logistics.Shipment{}, apperr.Validation(map[string]string{"courierCode": "Layanan kurir tidak tersedia"})
	}
	row, err := p.q(ctx).UpsertMockShipment(ctx, sqlcgen.UpsertMockShipmentParams{
		ReferenceID:    order.ReferenceID,
		TrackingNumber: "MOCK-" + order.ReferenceID,
		CourierCode:    order.CourierCode,
		ShippingCost:   rates[idx].ShippingCost.Decimal(),
	})
	if err != nil {
		return logistics.Shipment{}, fmt.Errorf("create mock shipment: %w", err)
	}
	return logistics.Shipment{
		ReferenceID:    row.ReferenceID,
		TrackingNumber: row.TrackingNumber,
		ShippingCost:   money.FromDecimal(row.ShippingCost),
		Status:         biteship.MapStatus(row.Status),
	}, nil
}

// Status mengembalikan status terkini, seperti API tracking Biteship.
func (p *Provider) Status(ctx context.Context, referenceID string) (logistics.ShipmentStatus, error) {
	s, err := p.Get(ctx, referenceID)
	if err != nil {
		return "", err
	}
	return biteship.MapStatus(s.Status), nil
}

// ParseNotification memakai verifikasi yang sama dengan adapter Biteship asli.
func (p *Provider) ParseNotification(_ context.Context, signature string, body []byte) (logistics.ShipmentNotification, error) {
	return biteship.ParseNotification(p.cfg.WebhookSecret, signature, body)
}

// DecodeNotification memakai decoder yang sama dengan adapter Biteship asli.
func (p *Provider) DecodeNotification(body []byte) (logistics.ShipmentNotification, error) {
	return biteship.DecodeNotification(body)
}

// Get membaca satu order mock.
func (p *Provider) Get(ctx context.Context, referenceID string) (Shipment, error) {
	row, err := p.q(ctx).GetMockShipment(ctx, referenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("mock shipment %s", referenceID))
	}
	if err != nil {
		return Shipment{}, fmt.Errorf("get mock shipment: %w", err)
	}
	return toShipment(row), nil
}

// List membaca order mock terbaru.
func (p *Provider) List(ctx context.Context, limit int32) ([]Shipment, error) {
	rows, err := p.q(ctx).ListMockShipments(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list mock shipments: %w", err)
	}
	shipments := make([]Shipment, 0, len(rows))
	for _, r := range rows {
		shipments = append(shipments, toShipment(r))
	}
	return shipments, nil
}

// Simulate mengubah status order lalu mengirim webhook bersignature ke endpoint asli.
func (p *Provider) Simulate(ctx context.Context, referenceID, status string) error {
	if !slices.Contains(Statuses, status) {
		return apperr.Validation(map[string]string{"status": "Status simulasi tidak dikenal"})
	}
	row, err := p.q(ctx).SetMockShipmentStatus(ctx, sqlcgen.SetMockShipmentStatusParams{ReferenceID: referenceID, Status: status})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("mock shipment %s", referenceID))
	}
	if err != nil {
		return fmt.Errorf("set mock shipment status: %w", err)
	}
	body, err := json.Marshal(biteship.Notification{
		Event:            "order.status",
		OrderID:          row.ReferenceID,
		CourierWaybillID: row.TrackingNumber,
		Status:           row.Status,
	})
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	return p.sendWebhook(ctx, body)
}

func (p *Provider) sendWebhook(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(biteship.HeaderSignature, biteship.Signature(p.cfg.WebhookSecret, body))
	resp, err := p.client.Do(req)
	if err != nil {
		return apperr.Wrap(apperr.CodeProviderUnavailable, fmt.Errorf("send mock webhook: %w", err))
	}
	defer resp.Body.Close() //nolint:errcheck // body hanya dibuang
	if resp.StatusCode != http.StatusOK {
		return apperr.Wrap(apperr.CodeProviderUnavailable, fmt.Errorf("mock webhook rejected with status %d", resp.StatusCode))
	}
	return nil
}

func toShipment(r sqlcgen.MockShipment) Shipment {
	return Shipment{
		ReferenceID:    r.ReferenceID,
		TrackingNumber: r.TrackingNumber,
		CourierCode:    r.CourierCode,
		ShippingCost:   money.FromDecimal(r.ShippingCost),
		Status:         r.Status,
		CreatedAt:      r.CreatedAt,
	}
}
