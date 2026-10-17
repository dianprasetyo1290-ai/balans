package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/logistics/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

var (
	day = clock.NewBusinessDate(2026, 11, 2)
	now = time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func isTransition(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, apperr.CodeInvalidStateTransition, apperr.As(err).Code)
}

func newDO(t *testing.T, typ domain.DeliveryType) (domain.DeliveryOrder, uuid.UUID) {
	t.Helper()
	alloc, b := uuid.New(), uuid.New()
	do, err := domain.New(domain.Input{
		Type: typ, SourceType: "sales_order", SourceID: uuid.New(), SourceLabel: "SO/2026/11/00001",
		RecipientName: "Toko Sehat", ShipToAddress: "Jl. Melati 5", LocationID: uuid.New(), BusinessDate: day,
		Lines: []domain.LineInput{{SourceLineID: &alloc, ProductID: uuid.New(), Qty: d("10"), Batches: []domain.BatchQty{{BatchID: &b, Qty: "10.000"}}}},
	})
	require.NoError(t, err)
	do.ID = uuid.New()
	return do, alloc
}

func TestDeliveryOrderValidation(t *testing.T) {
	_, err := domain.New(domain.Input{Type: "x", SourceType: "X", RecipientName: strings.Repeat("a", 201),
		Lines: []domain.LineInput{{Qty: d("1"), Batches: []domain.BatchQty{{Qty: "2"}}}}})
	f := apperr.As(err).Fields
	for _, k := range []string{"deliveryType", "source.type", "source.id", "locationId", "businessDate", "recipientName", "lines[0].productId", "lines[0].batches"} {
		assert.Contains(t, f, k)
	}
	_, err = domain.New(domain.Input{Type: domain.DeliverySales, SourceType: "sales_order", SourceID: uuid.New(), LocationID: uuid.New(), BusinessDate: day})
	assert.Contains(t, apperr.As(err).Fields, "lines")
	assert.True(t, domain.DeliveryConsignment.Valid())
	assert.True(t, domain.DeliveryVendingBuffer.Valid())
	assert.True(t, domain.DeliveryTransfer.Valid())
	assert.True(t, domain.BatchQty{Qty: "x"}.Quantity().IsZero())
}

func TestDeliveryOrderLifecycle(t *testing.T) {
	do, alloc := newDO(t, domain.DeliverySales)
	assert.Equal(t, "Draft DO SO/2026/11/00001", do.Label())
	assert.Equal(t, []string{domain.ActionRelease, domain.ActionCancel}, do.Actions())
	isTransition(t, do.Deliver(now))
	isTransition(t, do.ApplyShipment(nil, now))

	leader := uuid.New()
	require.NoError(t, do.Release("DO/2026/11/00001", leader, "Logistik Leader", now))
	assert.Equal(t, "DO/2026/11/00001", do.Label())
	isTransition(t, do.Release("x", leader, "", now))
	assert.Equal(t, []string{domain.ActionCancel}, do.Actions())
	err := do.ApplyShipment(map[uuid.UUID]decimal.Decimal{alloc: d("10")}, now)
	isTransition(t, err)
	do.OutboundDone(now)
	assert.Equal(t, []string{domain.ActionShip, domain.ActionCancel}, do.Actions())
	isTransition(t, do.ApplyShipment(map[uuid.UUID]decimal.Decimal{}, now))
	require.NoError(t, do.ApplyShipment(map[uuid.UUID]decimal.Decimal{alloc: d("8")}, now))
	assert.True(t, do.Lines[0].ShippedQty.Equal(d("8")))
	assert.Equal(t, domain.StatusInTransit, do.Status)
	assert.Equal(t, []string{domain.ActionDeliver}, do.Actions())
	isTransition(t, do.Cancel("batal", now))
	require.NoError(t, do.Deliver(now))
	assert.Empty(t, do.Actions())
	assert.Equal(t, "Diterima", do.Status.Label())

	other, _ := newDO(t, domain.DeliverySales)
	assert.Contains(t, apperr.As(other.Cancel(" ", now)).Fields, "reason")
	require.NoError(t, other.Cancel("Pelanggan batal", now))
	assert.Equal(t, "Dibatalkan", other.Status.Label())
	for _, s := range []domain.Status{domain.StatusDraft, domain.StatusInProgress, domain.StatusInTransit, "x"} {
		assert.NotEmpty(t, s.Label())
	}
}

func TestShipmentRules(t *testing.T) {
	do, _ := newDO(t, domain.DeliveryMarketplace)
	jne := domain.Courier{ID: uuid.New(), Code: "jne", Name: "JNE", IsActive: true}
	_, err := domain.NewShipment(do, jne, domain.ShipInput{ShippingCost: money.FromInt(-1)}, "", 1, now)
	f := apperr.As(err).Fields
	assert.Contains(t, f, "marketplaceAwb")
	assert.Contains(t, f, "shippingCost")
	s, err := domain.NewShipment(do, jne, domain.ShipInput{MarketplaceAWB: " SPX123 ", TrackingNumber: "JNE1"}, "", 2, now)
	require.NoError(t, err)
	assert.Equal(t, "SPX123", s.MarketplaceAWB)
	assert.Equal(t, domain.ShipmentStatusPickedUp, s.Status)

	require.NoError(t, s.Track(domain.ShipmentStatusInTransit))
	assert.Contains(t, apperr.As(s.Track(domain.ShipmentStatusDelivered)).Fields, "status")
	require.NoError(t, s.Confirm(domain.DeliverInput{ReceivedByName: "Budi"}, uuid.New(), "Staf", now))
	isTransition(t, s.Confirm(domain.DeliverInput{}, uuid.New(), "Staf", now))
	isTransition(t, s.Track(domain.ShipmentStatusReturned))

	// Kurir internal: wajib Merchandiser, hanya dia yang mengonfirmasi dengan foto.
	sales, _ := newDO(t, domain.DeliverySales)
	own := domain.Courier{ID: uuid.New(), Code: "own", Name: "Kurir Internal", IsInternal: true, IsActive: true}
	_, err = domain.NewShipment(sales, own, domain.ShipInput{}, "", 1, now)
	assert.Contains(t, apperr.As(err).Fields, "internalCourierUserId")
	_, err = domain.NewShipment(sales, domain.Courier{}, domain.ShipInput{}, "", 1, now)
	assert.Contains(t, apperr.As(err).Fields, "courierId")
	merch := uuid.New()
	s, err = domain.NewShipment(sales, own, domain.ShipInput{InternalCourierUserID: &merch}, "Merchandiser", 1, now)
	require.NoError(t, err)
	err = s.Confirm(domain.DeliverInput{ProofFileIDs: []uuid.UUID{uuid.New()}}, uuid.New(), "Lain", now)
	assert.Equal(t, apperr.CodeForbidden, apperr.As(err).Code)
	assert.Contains(t, apperr.As(s.Confirm(domain.DeliverInput{}, merch, "Merchandiser", now)).Fields, "proofFileIds")
	assert.Contains(t, apperr.As(s.Confirm(domain.DeliverInput{ProofFileIDs: make([]uuid.UUID, 6)}, merch, "M", now)).Fields, "proofFileIds")
	require.NoError(t, s.Confirm(domain.DeliverInput{ProofFileIDs: []uuid.UUID{uuid.New()}, Note: "Diterima satpam"}, merch, "Merchandiser", now))
	assert.Equal(t, domain.ShipmentStatusDelivered, s.Status)
}

func TestCourier(t *testing.T) {
	_, err := domain.NewCourier(domain.CourierInput{Code: "JNE Reg!", Name: ""})
	f := apperr.As(err).Fields
	assert.Contains(t, f, "code")
	assert.Contains(t, f, "name")
	c, err := domain.NewCourier(domain.CourierInput{Code: " Anteraja ", Name: "AnterAja", IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "anteraja", c.Code)
	require.NoError(t, c.Update(domain.CourierInput{Code: "lain", Name: "AnterAja Baru", IsActive: false}))
	assert.Equal(t, "anteraja", c.Code, "kode tidak berubah")
	assert.False(t, c.IsActive)
}
