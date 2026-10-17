// Package postgres adalah repository modul logistics (kurir, Delivery Order, pengiriman).
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/logistics/domain"
	"balans/internal/logistics/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// Store menyimpan data logistik.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(db.Conn(ctx, s.pool)) }

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func textOf(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func versionConflict(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeVersionConflict, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func uuids(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// --- Kurir ------------------------------------------------------------------------------------

func toCourier(r sqlcgen.Courier) domain.Courier {
	return domain.Courier{
		ID: r.ID, Code: r.Code, Name: r.Name, IsInternal: r.IsInternal, IsActive: r.IsActive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListCouriers mengembalikan kurir (active nil = semua).
func (s *Store) ListCouriers(ctx context.Context, active *bool) ([]domain.Courier, error) {
	p := pgtype.Bool{}
	if active != nil {
		p = pgtype.Bool{Bool: *active, Valid: true}
	}
	rows, err := s.q(ctx).ListCouriers(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list couriers: %w", err)
	}
	out := make([]domain.Courier, len(rows))
	for i, r := range rows {
		out[i] = toCourier(r)
	}
	return out, nil
}

// GetCourier membaca kurir.
func (s *Store) GetCourier(ctx context.Context, id uuid.UUID) (domain.Courier, error) {
	r, err := s.q(ctx).GetCourier(ctx, id)
	if err != nil {
		return domain.Courier{}, notFound(err, "get courier")
	}
	return toCourier(r), nil
}

// InsertCourier menyimpan kurir baru.
func (s *Store) InsertCourier(ctx context.Context, c domain.Courier, actorID uuid.UUID) (domain.Courier, error) {
	r, err := s.q(ctx).InsertCourier(ctx, sqlcgen.InsertCourierParams{
		Code: c.Code, Name: c.Name, IsInternal: c.IsInternal, IsActive: c.IsActive, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_couriers_code") {
		return domain.Courier{}, &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "Kode kurir sudah dipakai.",
			Fields: map[string]string{"code": "Kode kurir sudah dipakai"}}
	}
	if err != nil {
		return domain.Courier{}, fmt.Errorf("insert courier: %w", err)
	}
	return toCourier(r), nil
}

// UpdateCourier menyimpan perubahan kurir (optimistic lock).
func (s *Store) UpdateCourier(ctx context.Context, c domain.Courier, actorID uuid.UUID) (domain.Courier, error) {
	r, err := s.q(ctx).UpdateCourier(ctx, sqlcgen.UpdateCourierParams{
		ID: c.ID, Version: int32(c.Version), Name: c.Name, IsInternal: c.IsInternal, IsActive: c.IsActive, ActorID: actorID, //nolint:gosec // versi kecil
	})
	if err != nil {
		return domain.Courier{}, versionConflict(err, "update courier")
	}
	return toCourier(r), nil
}

// --- Delivery Order ---------------------------------------------------------------------------

func toDeliveryOrder(r sqlcgen.DeliveryOrder) domain.DeliveryOrder {
	return domain.DeliveryOrder{
		ID: r.ID, Number: textOf(r.Number), Type: domain.DeliveryType(r.DeliveryType), SourceType: r.SourceType,
		SourceID: r.SourceID, SourceLabel: r.SourceLabel, SalesOrderID: uuidPtr(r.SalesOrderID), PartnerID: uuidPtr(r.PartnerID),
		CustomerID: uuidPtr(r.CustomerID), RecipientName: r.RecipientName, RecipientPhone: r.RecipientPhone,
		ShipToAddress: r.ShipToAddress, LocationID: r.LocationID, BusinessDate: r.BusinessDate, Status: domain.Status(r.Status),
		Note: r.Note, OutboundID: uuidPtr(r.OutboundID), OutboundCompletedAt: r.OutboundCompletedAt,
		ReleasedBy: uuidPtr(r.ReleasedBy), ReleasedByName: r.ReleasedByName, ReleasedAt: r.ReleasedAt, ShippedAt: r.ShippedAt,
		DeliveredAt: r.DeliveredAt, CancelledAt: r.CancelledAt, CancelReason: r.CancelReason, CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// InsertDeliveryOrder menyimpan DO beserta barisnya. DO terbuka kedua untuk dokumen asal yang sama ditolak.
func (s *Store) InsertDeliveryOrder(ctx context.Context, d domain.DeliveryOrder, actorID uuid.UUID) (domain.DeliveryOrder, error) {
	q := s.q(ctx)
	r, err := q.InsertDeliveryOrder(ctx, sqlcgen.InsertDeliveryOrderParams{
		DeliveryType: string(d.Type), SourceType: d.SourceType, SourceID: d.SourceID, SourceLabel: d.SourceLabel,
		SalesOrderID: nullUUID(d.SalesOrderID), PartnerID: nullUUID(d.PartnerID), CustomerID: nullUUID(d.CustomerID),
		RecipientName: d.RecipientName, RecipientPhone: d.RecipientPhone, ShipToAddress: d.ShipToAddress,
		LocationID: d.LocationID, BusinessDate: d.BusinessDate, Status: string(d.Status), Note: d.Note, ActorID: actorID,
	})
	if isUniqueViolation(err, "uq_delivery_orders_open_source") {
		return domain.DeliveryOrder{}, apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"Dokumen asal ini masih punya Delivery Order yang belum dikirim.")
	}
	if err != nil {
		return domain.DeliveryOrder{}, fmt.Errorf("insert delivery order: %w", err)
	}
	out := toDeliveryOrder(r)
	for _, l := range d.Lines {
		raw, err := json.Marshal(l.Batches)
		if err != nil {
			return domain.DeliveryOrder{}, err
		}
		if l.Batches == nil {
			raw = []byte("[]")
		}
		id, err := q.InsertDeliveryOrderLine(ctx, sqlcgen.InsertDeliveryOrderLineParams{
			DeliveryOrderID: r.ID, LineNo: int32(l.LineNo), SourceLineID: nullUUID(l.SourceLineID), //nolint:gosec // maks. 200 baris
			SalesOrderLineID: nullUUID(l.SalesOrderLineID), ProductID: l.ProductID, Qty: l.Qty, Batches: raw,
		})
		if err != nil {
			return domain.DeliveryOrder{}, fmt.Errorf("insert delivery order line: %w", err)
		}
		l.ID = id
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// GetDeliveryOrder membaca DO beserta barisnya; forUpdate mengunci header.
func (s *Store) GetDeliveryOrder(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.DeliveryOrder, error) {
	var (
		r   sqlcgen.DeliveryOrder
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetDeliveryOrderForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetDeliveryOrder(ctx, id)
	}
	if err != nil {
		return domain.DeliveryOrder{}, notFound(err, "get delivery order")
	}
	return s.withLines(ctx, toDeliveryOrder(r))
}

func (s *Store) withLines(ctx context.Context, d domain.DeliveryOrder) (domain.DeliveryOrder, error) {
	rows, err := s.q(ctx).ListDeliveryOrderLines(ctx, d.ID)
	if err != nil {
		return domain.DeliveryOrder{}, fmt.Errorf("list delivery order lines: %w", err)
	}
	for _, r := range rows {
		l := domain.Line{
			ID: r.ID, LineNo: int(r.LineNo), SourceLineID: uuidPtr(r.SourceLineID), SalesOrderLineID: uuidPtr(r.SalesOrderLineID),
			ProductID: r.ProductID, Qty: r.Qty, ShippedQty: r.ShippedQty,
		}
		if err := json.Unmarshal(r.Batches, &l.Batches); err != nil {
			return domain.DeliveryOrder{}, fmt.Errorf("decode delivery order batches: %w", err)
		}
		d.Lines = append(d.Lines, l)
	}
	return d, nil
}

// DeliveryOrdersBySource mengembalikan DO milik dokumen asal (urut dibuat).
func (s *Store) DeliveryOrdersBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]domain.DeliveryOrder, error) {
	rows, err := s.q(ctx).ListDeliveryOrdersBySource(ctx, sqlcgen.ListDeliveryOrdersBySourceParams{SourceType: sourceType, SourceID: sourceID})
	if err != nil {
		return nil, fmt.Errorf("list delivery orders by source: %w", err)
	}
	out := make([]domain.DeliveryOrder, 0, len(rows))
	for _, r := range rows {
		d, err := s.withLines(ctx, toDeliveryOrder(r))
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// SaveDeliveryOrder menyimpan header (optimistic lock) dan qty dikirim per baris.
func (s *Store) SaveDeliveryOrder(ctx context.Context, d domain.DeliveryOrder, actorID uuid.UUID) (domain.DeliveryOrder, error) {
	q := s.q(ctx)
	for _, l := range d.Lines {
		if err := q.UpdateDeliveryOrderLineShipped(ctx, sqlcgen.UpdateDeliveryOrderLineShippedParams{ID: l.ID, ShippedQty: l.ShippedQty}); err != nil {
			return domain.DeliveryOrder{}, fmt.Errorf("update delivery order line: %w", err)
		}
	}
	r, err := q.UpdateDeliveryOrder(ctx, sqlcgen.UpdateDeliveryOrderParams{
		ID: d.ID, Version: int32(d.Version), Number: nullText(d.Number), Status: string(d.Status), //nolint:gosec // versi kecil
		OutboundID: nullUUID(d.OutboundID), OutboundCompletedAt: d.OutboundCompletedAt, ReleasedBy: nullUUID(d.ReleasedBy),
		ReleasedByName: d.ReleasedByName, ReleasedAt: d.ReleasedAt, ShippedAt: d.ShippedAt, DeliveredAt: d.DeliveredAt,
		CancelledAt: d.CancelledAt, CancelReason: d.CancelReason, ActorID: actorID,
	})
	if err != nil {
		return domain.DeliveryOrder{}, versionConflict(err, "update delivery order")
	}
	return s.withLines(ctx, toDeliveryOrder(r))
}

// DeliveryOrderFilter adalah kriteria daftar DO.
type DeliveryOrderFilter struct {
	Status   string
	Type     string
	SourceID *uuid.UUID
	Search   string
}

// DeliveryOrderSummary adalah satu baris daftar DO.
type DeliveryOrderSummary struct {
	DeliveryOrder  domain.DeliveryOrder
	LineCount      int
	CourierName    string
	TrackingNumber string
	MarketplaceAWB string
}

// ListDeliveryOrders mengembalikan satu halaman DO terbaru.
func (s *Store) ListDeliveryOrders(ctx context.Context, f DeliveryOrderFilter, page paging.Page) (paging.Result[DeliveryOrderSummary], error) {
	q := s.q(ctx)
	rows, err := q.ListDeliveryOrders(ctx, sqlcgen.ListDeliveryOrdersParams{
		Status: nullText(f.Status), DeliveryType: nullText(f.Type), SourceID: nullUUID(f.SourceID), Search: nullText(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[DeliveryOrderSummary]{}, fmt.Errorf("list delivery orders: %w", err)
	}
	total, err := q.CountDeliveryOrders(ctx, sqlcgen.CountDeliveryOrdersParams{
		Status: nullText(f.Status), DeliveryType: nullText(f.Type), SourceID: nullUUID(f.SourceID), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[DeliveryOrderSummary]{}, fmt.Errorf("count delivery orders: %w", err)
	}
	items := make([]DeliveryOrderSummary, len(rows))
	for i, r := range rows {
		items[i] = DeliveryOrderSummary{
			DeliveryOrder: toDeliveryOrder(sqlcgen.DeliveryOrder{
				ID: r.ID, Number: r.Number, DeliveryType: r.DeliveryType, SourceType: r.SourceType, SourceID: r.SourceID,
				SourceLabel: r.SourceLabel, SalesOrderID: r.SalesOrderID, PartnerID: r.PartnerID, CustomerID: r.CustomerID,
				RecipientName: r.RecipientName, RecipientPhone: r.RecipientPhone, ShipToAddress: r.ShipToAddress,
				LocationID: r.LocationID, BusinessDate: r.BusinessDate, Status: r.Status, Note: r.Note, OutboundID: r.OutboundID,
				OutboundCompletedAt: r.OutboundCompletedAt, ReleasedBy: r.ReleasedBy, ReleasedByName: r.ReleasedByName,
				ReleasedAt: r.ReleasedAt, ShippedAt: r.ShippedAt, DeliveredAt: r.DeliveredAt, CancelledAt: r.CancelledAt,
				CancelReason: r.CancelReason, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt,
				UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			LineCount: int(r.LineCount), CourierName: textOf(r.CourierName), TrackingNumber: textOf(r.TrackingNumber),
			MarketplaceAWB: textOf(r.MarketplaceAwb),
		}
	}
	return paging.Result[DeliveryOrderSummary]{Items: items, Page: page, Total: total}, nil
}

// --- Pengiriman -------------------------------------------------------------------------------

func toShipment(r sqlcgen.Shipment) domain.ShipmentRecord {
	return domain.ShipmentRecord{
		ID: r.ID, DeliveryOrderID: r.DeliveryOrderID, CourierID: r.CourierID, InternalCourierUserID: uuidPtr(r.InternalCourierUserID),
		InternalCourierName: r.InternalCourierName, ServiceCode: r.ServiceCode, TrackingNumber: r.TrackingNumber,
		MarketplaceAWB: r.MarketplaceAwb, ShippingCost: money.FromDecimal(r.ShippingCostAmount), PackageCount: int(r.PackageCount),
		Status: domain.ShipmentStatus(r.Status), ShippedAt: r.ShippedAt, DeliveredAt: r.DeliveredAt,
		ReceivedByName: r.ReceivedByName, DeliveryNote: r.DeliveryNote, ProofFileIDs: r.ProofFileIds,
		ConfirmedBy: uuidPtr(r.ConfirmedBy), ConfirmedByName: r.ConfirmedByName, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

// InsertShipment menyimpan pengiriman DO.
func (s *Store) InsertShipment(ctx context.Context, sh domain.ShipmentRecord, actorID uuid.UUID) (domain.ShipmentRecord, error) {
	r, err := s.q(ctx).InsertShipment(ctx, sqlcgen.InsertShipmentParams{
		DeliveryOrderID: sh.DeliveryOrderID, CourierID: sh.CourierID, InternalCourierUserID: nullUUID(sh.InternalCourierUserID),
		InternalCourierName: sh.InternalCourierName, ServiceCode: sh.ServiceCode, TrackingNumber: sh.TrackingNumber,
		MarketplaceAwb: sh.MarketplaceAWB, ShippingCostAmount: sh.ShippingCost.Decimal(), PackageCount: int32(sh.PackageCount), //nolint:gosec // maks. 500 koli
		Status: string(sh.Status), ShippedAt: sh.ShippedAt, ActorID: actorID,
	})
	if err != nil {
		return domain.ShipmentRecord{}, fmt.Errorf("insert shipment: %w", err)
	}
	return toShipment(r), nil
}

// ShipmentByDeliveryOrder membaca pengiriman DO; ok false bila belum dikirim.
func (s *Store) ShipmentByDeliveryOrder(ctx context.Context, doID uuid.UUID) (domain.ShipmentRecord, bool, error) {
	r, err := s.q(ctx).GetShipmentByDeliveryOrder(ctx, doID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShipmentRecord{}, false, nil
	}
	if err != nil {
		return domain.ShipmentRecord{}, false, fmt.Errorf("get shipment: %w", err)
	}
	return toShipment(r), true, nil
}

// GetShipment membaca pengiriman.
func (s *Store) GetShipment(ctx context.Context, id uuid.UUID) (domain.ShipmentRecord, error) {
	r, err := s.q(ctx).GetShipment(ctx, id)
	if err != nil {
		return domain.ShipmentRecord{}, notFound(err, "get shipment")
	}
	return toShipment(r), nil
}

// SaveShipment menyimpan status/konfirmasi pengiriman (optimistic lock).
func (s *Store) SaveShipment(ctx context.Context, sh domain.ShipmentRecord, actorID uuid.UUID) (domain.ShipmentRecord, error) {
	r, err := s.q(ctx).UpdateShipment(ctx, sqlcgen.UpdateShipmentParams{
		ID: sh.ID, Version: int32(sh.Version), Status: string(sh.Status), DeliveredAt: sh.DeliveredAt, //nolint:gosec // versi kecil
		ReceivedByName: sh.ReceivedByName, DeliveryNote: sh.DeliveryNote, ProofFileIds: uuids(sh.ProofFileIDs),
		ConfirmedBy: nullUUID(sh.ConfirmedBy), ConfirmedByName: sh.ConfirmedByName, ActorID: actorID,
	})
	if err != nil {
		return domain.ShipmentRecord{}, versionConflict(err, "update shipment")
	}
	return toShipment(r), nil
}

// ShipmentEvent adalah riwayat status pengiriman.
type ShipmentEvent struct {
	ID            uuid.UUID
	Status        domain.ShipmentStatus
	OccurredAt    time.Time
	Source        string
	Note          string
	CreatedByName string
}

// InsertShipmentEvent mencatat riwayat status pengiriman.
func (s *Store) InsertShipmentEvent(ctx context.Context, shipmentID uuid.UUID, e ShipmentEvent, actorID uuid.UUID) error {
	if err := s.q(ctx).InsertShipmentEvent(ctx, sqlcgen.InsertShipmentEventParams{
		ShipmentID: shipmentID, Status: string(e.Status), OccurredAt: e.OccurredAt, Source: e.Source, Note: e.Note,
		ActorID: actorID, ActorName: e.CreatedByName,
	}); err != nil {
		return fmt.Errorf("insert shipment event: %w", err)
	}
	return nil
}

// ShipmentEvents mengembalikan riwayat status pengiriman.
func (s *Store) ShipmentEvents(ctx context.Context, shipmentID uuid.UUID) ([]ShipmentEvent, error) {
	rows, err := s.q(ctx).ListShipmentEvents(ctx, shipmentID)
	if err != nil {
		return nil, fmt.Errorf("list shipment events: %w", err)
	}
	out := make([]ShipmentEvent, len(rows))
	for i, r := range rows {
		out[i] = ShipmentEvent{
			ID: r.ID, Status: domain.ShipmentStatus(r.Status), OccurredAt: r.OccurredAt, Source: r.Source, Note: r.Note,
			CreatedByName: r.CreatedByName,
		}
	}
	return out, nil
}

// ShipmentFilter adalah kriteria daftar pengiriman.
type ShipmentFilter struct {
	Status        string
	CourierUserID *uuid.UUID
	Search        string
}

// ShipmentSummary adalah satu baris daftar pengiriman.
type ShipmentSummary struct {
	Shipment            domain.ShipmentRecord
	DeliveryOrderNumber string
	DeliveryType        domain.DeliveryType
	RecipientName       string
	SourceLabel         string
	CourierName         string
}

// ListShipments mengembalikan satu halaman pengiriman terbaru.
func (s *Store) ListShipments(ctx context.Context, f ShipmentFilter, page paging.Page) (paging.Result[ShipmentSummary], error) {
	q := s.q(ctx)
	rows, err := q.ListShipments(ctx, sqlcgen.ListShipmentsParams{
		Status: nullText(f.Status), CourierUserID: nullUUID(f.CourierUserID), Search: nullText(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[ShipmentSummary]{}, fmt.Errorf("list shipments: %w", err)
	}
	total, err := q.CountShipments(ctx, sqlcgen.CountShipmentsParams{
		Status: nullText(f.Status), CourierUserID: nullUUID(f.CourierUserID), Search: nullText(f.Search),
	})
	if err != nil {
		return paging.Result[ShipmentSummary]{}, fmt.Errorf("count shipments: %w", err)
	}
	items := make([]ShipmentSummary, len(rows))
	for i, r := range rows {
		items[i] = ShipmentSummary{
			Shipment: toShipment(sqlcgen.Shipment{
				ID: r.ID, DeliveryOrderID: r.DeliveryOrderID, CourierID: r.CourierID, InternalCourierUserID: r.InternalCourierUserID,
				InternalCourierName: r.InternalCourierName, ServiceCode: r.ServiceCode, TrackingNumber: r.TrackingNumber,
				MarketplaceAwb: r.MarketplaceAwb, ShippingCostAmount: r.ShippingCostAmount, PackageCount: r.PackageCount,
				Status: r.Status, ShippedAt: r.ShippedAt, DeliveredAt: r.DeliveredAt, ReceivedByName: r.ReceivedByName,
				DeliveryNote: r.DeliveryNote, ProofFileIds: r.ProofFileIds, ConfirmedBy: r.ConfirmedBy,
				ConfirmedByName: r.ConfirmedByName, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt,
				UpdatedBy: r.UpdatedBy, Version: r.Version,
			}),
			DeliveryOrderNumber: textOf(r.DeliveryOrderNumber), DeliveryType: domain.DeliveryType(r.DeliveryType),
			RecipientName: r.RecipientName, SourceLabel: r.SourceLabel, CourierName: r.CourierName,
		}
	}
	return paging.Result[ShipmentSummary]{Items: items, Page: page, Total: total}, nil
}
