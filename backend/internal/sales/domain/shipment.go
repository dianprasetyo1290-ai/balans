package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// ShipmentLine adalah qty satu alokasi yang keluar dari stok beserta HPP FIFO-nya.
type ShipmentLine struct {
	AllocationID uuid.UUID
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	Cost         money.Amount
}

// Shipment adalah pengiriman barang Sales Order oleh dokumen logistik (DO ship atau serah ke kurir marketplace):
// stok reserved keluar dan HPP dijurnal (A-06; marketplace A-10).
type Shipment struct {
	ID           uuid.UUID
	OrderID      uuid.UUID
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	BusinessDate clock.BusinessDate
	Cost         money.Amount
	Lines        []ShipmentLine
	CreatedAt    time.Time
}
