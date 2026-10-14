// Package domain berisi entity dan value object modul logistics.
package domain

import (
	"errors"

	"balans/internal/platform/money"
)

// ShipmentStatus adalah status pengiriman menurut Balans, terlepas dari istilah kurir.
type ShipmentStatus string

const (
	ShipmentStatusConfirmed ShipmentStatus = "confirmed"
	ShipmentStatusPickedUp  ShipmentStatus = "picked_up"
	ShipmentStatusInTransit ShipmentStatus = "in_transit"
	ShipmentStatusDelivered ShipmentStatus = "delivered"
	ShipmentStatusReturned  ShipmentStatus = "returned"
	ShipmentStatusCancelled ShipmentStatus = "cancelled"
)

// ErrInvalidSignature dikembalikan adapter jika signature webhook tidak valid.
var ErrInvalidSignature = errors.New("invalid webhook signature")

// RateRequest adalah permintaan cek ongkir.
type RateRequest struct {
	OriginPostalCode      string
	DestinationPostalCode string
	WeightGrams           int
	CourierCodes          []string
}

// Rate adalah satu pilihan layanan kurir.
type Rate struct {
	CourierCode  string
	ServiceCode  string
	ShippingCost money.Amount
	ETA          string
}

// ShipmentOrder adalah permintaan membuat order pengiriman. ReferenceID adalah ID Delivery Order Balans
// sehingga kurir ikut menolak duplikat (Technical Guideline 5.10).
type ShipmentOrder struct {
	ReferenceID string
	CourierCode string
	ServiceCode string
	WeightGrams int
}

// Shipment adalah order pengiriman yang sudah dibuat di kurir.
type Shipment struct {
	ReferenceID    string
	TrackingNumber string
	ShippingCost   money.Amount
	Status         ShipmentStatus
}

// ShipmentNotification adalah webhook status pengiriman yang sudah diverifikasi dan diterjemahkan.
type ShipmentNotification struct {
	EventID        string
	ReferenceID    string
	TrackingNumber string
	Status         ShipmentStatus
}
