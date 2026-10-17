// Package postgres adalah repository dokumen pengadaan: Purchase Request, Purchase Order, dan uang muka.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres/sqlcgen"
)

// Store menyimpan dokumen pengadaan.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

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

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func pgDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func fromPgDate(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	b := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &b
}

func amount(d decimal.Decimal) money.Amount { return money.FromDecimal(d) }

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

func int32Of(n int) int32 { return int32(n) } //nolint:gosec // versi, nomor baris, dan termin kecil

// --- Purchase Request ---------------------------------------------------------------------------

// RequestFilter adalah kriteria daftar PR.
type RequestFilter struct {
	Status      domain.Status
	DivisionID  *uuid.UUID
	RequestedBy *uuid.UUID
	DateFrom    *clock.BusinessDate
	DateTo      *clock.BusinessDate
	Search      string
	Sort        paging.Sort
}

func toRequest(r sqlcgen.PurchaseRequest) domain.PurchaseRequest {
	return domain.PurchaseRequest{
		ID: r.ID, Number: r.Number.String, DivisionID: r.DivisionID, RequestedBy: r.RequestedBy,
		BusinessDate: r.BusinessDate, NeededDate: fromPgDate(r.NeededDate), Note: r.Note,
		EstimatedTotal: amount(r.EstimatedTotalAmount), Status: domain.Status(r.Status), CloseReason: r.CloseReason,
		SubmittedAt: r.SubmittedAt, DecidedAt: r.DecidedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

func toRequestLine(l sqlcgen.PurchaseRequestLine) domain.RequestLine {
	return domain.RequestLine{
		ID: l.ID, LineNo: int(l.LineNo), ProductID: uuidPtr(l.ProductID), Description: l.Description, Qty: l.Qty,
		EstimatedPrice: amount(l.EstimatedPriceAmount), FulfilledQty: l.FulfilledQty, Note: l.Note,
	}
}

// ListRequests mengembalikan satu halaman PR (tanpa baris).
func (s *Store) ListRequests(ctx context.Context, f RequestFilter, page paging.Page) (paging.Result[domain.PurchaseRequest], error) {
	status := text(string(f.Status))
	search := text(f.Search)
	rows, err := s.q(ctx).ListPurchaseRequests(ctx, sqlcgen.ListPurchaseRequestsParams{
		Status: status, DivisionID: nullUUID(f.DivisionID), RequestedBy: nullUUID(f.RequestedBy),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search, Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.PurchaseRequest]{}, fmt.Errorf("list purchase requests: %w", err)
	}
	total, err := s.q(ctx).CountPurchaseRequests(ctx, sqlcgen.CountPurchaseRequestsParams{
		Status: status, DivisionID: nullUUID(f.DivisionID), RequestedBy: nullUUID(f.RequestedBy),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search,
	})
	if err != nil {
		return paging.Result[domain.PurchaseRequest]{}, fmt.Errorf("count purchase requests: %w", err)
	}
	items := make([]domain.PurchaseRequest, len(rows))
	for i, r := range rows {
		items[i] = toRequest(r)
	}
	return paging.Result[domain.PurchaseRequest]{Items: items, Page: page, Total: total}, nil
}

// GetRequest membaca PR beserta barisnya; forUpdate mengunci baris PR.
func (s *Store) GetRequest(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PurchaseRequest, error) {
	var (
		r   sqlcgen.PurchaseRequest
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPurchaseRequestForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPurchaseRequest(ctx, id)
	}
	if err != nil {
		return domain.PurchaseRequest{}, notFound(err, "get purchase request")
	}
	out := toRequest(r)
	lines, err := s.q(ctx).ListPurchaseRequestLines(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.PurchaseRequest{}, fmt.Errorf("list purchase request lines: %w", err)
	}
	out.Lines = make([]domain.RequestLine, len(lines))
	for i, l := range lines {
		out.Lines[i] = toRequestLine(l)
	}
	return out, nil
}

func (s *Store) insertRequestLines(ctx context.Context, r domain.PurchaseRequest) error {
	for _, l := range r.Lines {
		if err := s.q(ctx).InsertPurchaseRequestLine(ctx, sqlcgen.InsertPurchaseRequestLineParams{
			PurchaseRequestID: r.ID, LineNo: int32Of(l.LineNo), ProductID: nullUUID(l.ProductID), Description: l.Description,
			Qty: l.Qty, EstimatedPriceAmount: l.EstimatedPrice.Decimal(), FulfilledQty: l.FulfilledQty, Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert purchase request line: %w", err)
		}
	}
	return nil
}

// InsertRequest menyimpan draft PR baru beserta barisnya.
func (s *Store) InsertRequest(ctx context.Context, r domain.PurchaseRequest, actorID uuid.UUID) (domain.PurchaseRequest, error) {
	row, err := s.q(ctx).InsertPurchaseRequest(ctx, sqlcgen.InsertPurchaseRequestParams{
		DivisionID: r.DivisionID, RequestedBy: r.RequestedBy, BusinessDate: r.BusinessDate, NeededDate: pgDate(r.NeededDate),
		Note: r.Note, EstimatedTotalAmount: r.EstimatedTotal.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return domain.PurchaseRequest{}, fmt.Errorf("insert purchase request: %w", err)
	}
	r.ID = row.ID
	if err := s.insertRequestLines(ctx, r); err != nil {
		return domain.PurchaseRequest{}, err
	}
	return s.GetRequest(ctx, r.ID, false)
}

// SaveRequest menyimpan header PR dengan optimistic locking. replaceLines mengganti seluruh baris (draft).
func (s *Store) SaveRequest(ctx context.Context, r domain.PurchaseRequest, version int, actorID uuid.UUID, replaceLines bool) error {
	_, err := s.q(ctx).UpdatePurchaseRequest(ctx, sqlcgen.UpdatePurchaseRequestParams{
		Number: text(r.Number), DivisionID: r.DivisionID, BusinessDate: r.BusinessDate, NeededDate: pgDate(r.NeededDate),
		Note: r.Note, EstimatedTotalAmount: r.EstimatedTotal.Decimal(), Status: string(r.Status), CloseReason: r.CloseReason,
		SubmittedAt: r.SubmittedAt, DecidedAt: r.DecidedAt, ActorID: actorID, ID: r.ID, Version: int32Of(version),
	})
	if err != nil {
		return versionConflict(err, "update purchase request")
	}
	if !replaceLines {
		return nil
	}
	if err := s.q(ctx).DeletePurchaseRequestLines(ctx, r.ID); err != nil {
		return fmt.Errorf("delete purchase request lines: %w", err)
	}
	return s.insertRequestLines(ctx, r)
}

// SetRequestLinesFulfilled menyimpan qty yang sudah dijadikan PO per baris.
func (s *Store) SetRequestLinesFulfilled(ctx context.Context, lines []domain.RequestLine) error {
	for _, l := range lines {
		if err := s.q(ctx).SetPurchaseRequestLineFulfilled(ctx, sqlcgen.SetPurchaseRequestLineFulfilledParams{
			FulfilledQty: l.FulfilledQty, ID: l.ID,
		}); err != nil {
			return fmt.Errorf("set purchase request line fulfilled: %w", err)
		}
	}
	return nil
}

// RequestLineOrder adalah baris PO yang menautkan baris PR.
type RequestLineOrder struct {
	RequestLineID uuid.UUID
	OrderID       uuid.UUID
	OrderNumber   string
	OrderStatus   domain.Status
	OrderedQty    decimal.Decimal
	ReceivedQty   decimal.Decimal
}

// RequestLineOrders mengembalikan baris PO yang dibuat dari PR.
func (s *Store) RequestLineOrders(ctx context.Context, requestID uuid.UUID) ([]RequestLineOrder, error) {
	rows, err := s.q(ctx).PurchaseRequestLineOrders(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("purchase request line orders: %w", err)
	}
	out := make([]RequestLineOrder, len(rows))
	for i, r := range rows {
		out[i] = RequestLineOrder{
			RequestLineID: r.PurchaseRequestLineID.UUID, OrderID: r.PurchaseOrderID, OrderNumber: r.Number.String,
			OrderStatus: domain.Status(r.Status), OrderedQty: r.OrderedQty, ReceivedQty: r.ReceivedQty,
		}
	}
	return out, nil
}

// RequestIDsByLines mengembalikan PR pemilik baris.
func (s *Store) RequestIDsByLines(ctx context.Context, lineIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(lineIDs) == 0 {
		return nil, nil
	}
	ids, err := s.q(ctx).PurchaseRequestIDsByLines(ctx, lineIDs)
	if err != nil {
		return nil, fmt.Errorf("purchase request ids by lines: %w", err)
	}
	return ids, nil
}

// --- Purchase Order -----------------------------------------------------------------------------

// OrderFilter adalah kriteria daftar PO.
type OrderFilter struct {
	Status     domain.Status
	Types      []domain.OrderType
	SupplierID *uuid.UUID
	DivisionID *uuid.UUID
	DateFrom   *clock.BusinessDate
	DateTo     *clock.BusinessDate
	Search     string
	Sort       paging.Sort
}

func toOrder(r sqlcgen.PurchaseOrder) domain.PurchaseOrder {
	return domain.PurchaseOrder{
		ID: r.ID, Number: r.Number.String, Type: domain.OrderType(r.OrderType), SupplierID: r.SupplierID,
		SupplierCode: r.SupplierCode, SupplierName: r.SupplierName, DivisionID: r.DivisionID, BusinessDate: r.BusinessDate,
		ExpectedDate: r.ExpectedDate, PaymentTermDays: int(r.PaymentTermDays), Note: r.Note,
		TollLocationID: uuidPtr(r.TollLocationID), MaterialSourceLocationID: uuidPtr(r.MaterialSourceLocationID),
		Subtotal: amount(r.SubtotalAmount), Total: amount(r.TotalAmount), Status: domain.Status(r.Status),
		CloseReason: r.CloseReason, CancelReason: r.CancelReason, SubmittedAt: r.SubmittedAt, ApprovedAt: r.ApprovedAt,
		ETANotifiedAt: r.EtaNotifiedAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

func typesOf(ts []domain.OrderType) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}

// ListOrders mengembalikan satu halaman PO (tanpa baris).
func (s *Store) ListOrders(ctx context.Context, f OrderFilter, page paging.Page) (paging.Result[domain.PurchaseOrder], error) {
	status, search := text(string(f.Status)), text(f.Search)
	rows, err := s.q(ctx).ListPurchaseOrders(ctx, sqlcgen.ListPurchaseOrdersParams{
		Status: status, OrderTypes: typesOf(f.Types), SupplierID: nullUUID(f.SupplierID), DivisionID: nullUUID(f.DivisionID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search, Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.PurchaseOrder]{}, fmt.Errorf("list purchase orders: %w", err)
	}
	total, err := s.q(ctx).CountPurchaseOrders(ctx, sqlcgen.CountPurchaseOrdersParams{
		Status: status, OrderTypes: typesOf(f.Types), SupplierID: nullUUID(f.SupplierID), DivisionID: nullUUID(f.DivisionID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search,
	})
	if err != nil {
		return paging.Result[domain.PurchaseOrder]{}, fmt.Errorf("count purchase orders: %w", err)
	}
	items := make([]domain.PurchaseOrder, len(rows))
	for i, r := range rows {
		items[i] = toOrder(r)
	}
	return paging.Result[domain.PurchaseOrder]{Items: items, Page: page, Total: total}, nil
}

// GetOrder membaca PO beserta baris dan bahan maklon; forUpdate mengunci baris PO.
func (s *Store) GetOrder(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.PurchaseOrder, error) {
	var (
		r   sqlcgen.PurchaseOrder
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetPurchaseOrderForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetPurchaseOrder(ctx, id)
	}
	if err != nil {
		return domain.PurchaseOrder{}, notFound(err, "get purchase order")
	}
	o := toOrder(r)
	lines, err := s.q(ctx).ListPurchaseOrderLines(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("list purchase order lines: %w", err)
	}
	o.Lines = make([]domain.OrderLine, len(lines))
	for i, l := range lines {
		o.Lines[i] = domain.OrderLine{
			ID: l.ID, LineNo: int(l.LineNo), ProductID: uuidPtr(l.ProductID), Description: l.Description,
			OrderedQty: l.OrderedQty, UnitPrice: amount(l.UnitPriceAmount), Amount: amount(l.LineAmount),
			ReceivedQty: l.ReceivedQty, BilledQty: l.BilledQty, PurchaseRequestLineID: uuidPtr(l.PurchaseRequestLineID),
			Note: l.Note,
		}
	}
	materials, err := s.q(ctx).ListPurchaseOrderMaterials(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("list purchase order materials: %w", err)
	}
	o.Materials = make([]domain.OrderMaterial, len(materials))
	for i, m := range materials {
		o.Materials[i] = domain.OrderMaterial{
			ID: m.ID, LineNo: int(m.LineNo), ProductID: m.MaterialProductID, PlannedQty: m.PlannedQty, Note: m.Note,
		}
	}
	return o, nil
}

func (s *Store) insertOrderItems(ctx context.Context, o domain.PurchaseOrder) error {
	for _, l := range o.Lines {
		if err := s.q(ctx).InsertPurchaseOrderLine(ctx, sqlcgen.InsertPurchaseOrderLineParams{
			PurchaseOrderID: o.ID, LineNo: int32Of(l.LineNo), ProductID: nullUUID(l.ProductID), Description: l.Description,
			OrderedQty: l.OrderedQty, UnitPriceAmount: l.UnitPrice.Decimal(), LineAmount: l.Amount.Decimal(),
			ReceivedQty: l.ReceivedQty, BilledQty: l.BilledQty, PurchaseRequestLineID: nullUUID(l.PurchaseRequestLineID),
			Note: l.Note,
		}); err != nil {
			return fmt.Errorf("insert purchase order line: %w", err)
		}
	}
	for _, m := range o.Materials {
		if err := s.q(ctx).InsertPurchaseOrderMaterial(ctx, sqlcgen.InsertPurchaseOrderMaterialParams{
			PurchaseOrderID: o.ID, LineNo: int32Of(m.LineNo), MaterialProductID: m.ProductID, PlannedQty: m.PlannedQty, Note: m.Note,
		}); err != nil {
			return fmt.Errorf("insert purchase order material: %w", err)
		}
	}
	return nil
}

// InsertOrder menyimpan draft PO baru beserta baris dan bahannya.
func (s *Store) InsertOrder(ctx context.Context, o domain.PurchaseOrder, actorID uuid.UUID) (domain.PurchaseOrder, error) {
	row, err := s.q(ctx).InsertPurchaseOrder(ctx, sqlcgen.InsertPurchaseOrderParams{
		OrderType: string(o.Type), SupplierID: o.SupplierID, SupplierCode: o.SupplierCode, SupplierName: o.SupplierName,
		DivisionID: o.DivisionID, BusinessDate: o.BusinessDate, ExpectedDate: o.ExpectedDate,
		PaymentTermDays: int32Of(o.PaymentTermDays), Note: o.Note, TollLocationID: nullUUID(o.TollLocationID),
		MaterialSourceLocationID: nullUUID(o.MaterialSourceLocationID), SubtotalAmount: o.Subtotal.Decimal(),
		TotalAmount: o.Total.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("insert purchase order: %w", err)
	}
	o.ID = row.ID
	if err := s.insertOrderItems(ctx, o); err != nil {
		return domain.PurchaseOrder{}, err
	}
	return s.GetOrder(ctx, o.ID, false)
}

// SaveOrder menyimpan header PO dengan optimistic locking. replaceLines mengganti baris dan bahan (draft).
func (s *Store) SaveOrder(ctx context.Context, o domain.PurchaseOrder, version int, actorID uuid.UUID, replaceLines bool) error {
	_, err := s.q(ctx).UpdatePurchaseOrder(ctx, sqlcgen.UpdatePurchaseOrderParams{
		Number: text(o.Number), SupplierID: o.SupplierID, SupplierCode: o.SupplierCode, SupplierName: o.SupplierName,
		DivisionID: o.DivisionID, BusinessDate: o.BusinessDate, ExpectedDate: o.ExpectedDate,
		PaymentTermDays: int32Of(o.PaymentTermDays), Note: o.Note, TollLocationID: nullUUID(o.TollLocationID),
		MaterialSourceLocationID: nullUUID(o.MaterialSourceLocationID), SubtotalAmount: o.Subtotal.Decimal(),
		TotalAmount: o.Total.Decimal(), Status: string(o.Status), CloseReason: o.CloseReason, CancelReason: o.CancelReason,
		SubmittedAt: o.SubmittedAt, ApprovedAt: o.ApprovedAt, EtaNotifiedAt: o.ETANotifiedAt, ActorID: actorID,
		ID: o.ID, Version: int32Of(version),
	})
	if err != nil {
		return versionConflict(err, "update purchase order")
	}
	if !replaceLines {
		return nil
	}
	if err := s.q(ctx).DeletePurchaseOrderLines(ctx, o.ID); err != nil {
		return fmt.Errorf("delete purchase order lines: %w", err)
	}
	if err := s.q(ctx).DeletePurchaseOrderMaterials(ctx, o.ID); err != nil {
		return fmt.Errorf("delete purchase order materials: %w", err)
	}
	return s.insertOrderItems(ctx, o)
}

// SaveOrderLineQty menyimpan qty diterima dan ditagih per baris.
func (s *Store) SaveOrderLineQty(ctx context.Context, lines []domain.OrderLine) error {
	for _, l := range lines {
		if err := s.q(ctx).SetPurchaseOrderLineQty(ctx, sqlcgen.SetPurchaseOrderLineQtyParams{
			ReceivedQty: l.ReceivedQty, BilledQty: l.BilledQty, ID: l.ID,
		}); err != nil {
			return fmt.Errorf("set purchase order line qty: %w", err)
		}
	}
	return nil
}

// OrdersDueForETA mengembalikan PO disetujui yang tiba pada tanggal tertentu dan belum diberi notifikasi ETA.
func (s *Store) OrdersDueForETA(ctx context.Context, expected clock.BusinessDate) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).OrdersDueForETA(ctx, expected)
	if err != nil {
		return nil, fmt.Errorf("orders due for eta: %w", err)
	}
	return ids, nil
}

// MarkETANotified menandai PO sudah diberi notifikasi ETA; false jika sudah ditandai proses lain.
func (s *Store) MarkETANotified(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	n, err := s.q(ctx).MarkPurchaseOrderETANotified(ctx, sqlcgen.MarkPurchaseOrderETANotifiedParams{At: &at, ID: id})
	if err != nil {
		return false, fmt.Errorf("mark purchase order eta notified: %w", err)
	}
	return n == 1, nil
}

// --- Uang muka ----------------------------------------------------------------------------------

// AdvanceFilter adalah kriteria daftar uang muka.
type AdvanceFilter struct {
	Status          domain.Status
	PurchaseOrderID *uuid.UUID
	SupplierID      *uuid.UUID
	DateFrom        *clock.BusinessDate
	DateTo          *clock.BusinessDate
	Search          string
}

func toAdvance(r sqlcgen.ListPurchaseAdvancesRow) domain.Advance {
	return domain.Advance{
		ID: r.ID, Number: r.Number.String, PurchaseOrderID: r.PurchaseOrderID, OrderNumber: r.OrderNumber.String,
		SupplierID: r.SupplierID, SupplierName: r.SupplierName, DivisionID: r.DivisionID, Amount: amount(r.Amount),
		PaymentDate: r.PaymentDate, BankAccountCode: r.BankAccountCode, Note: r.Note, Applied: amount(r.AppliedAmount),
		Status: domain.Status(r.Status), JournalEntryID: uuidPtr(r.JournalEntryID),
		ReversalJournalEntryID: uuidPtr(r.ReversalJournalEntryID), CancelReason: r.CancelReason, PostedAt: r.PostedAt,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListAdvances mengembalikan satu halaman uang muka.
func (s *Store) ListAdvances(ctx context.Context, f AdvanceFilter, page paging.Page) (paging.Result[domain.Advance], error) {
	status, search := text(string(f.Status)), text(f.Search)
	rows, err := s.q(ctx).ListPurchaseAdvances(ctx, sqlcgen.ListPurchaseAdvancesParams{
		Status: status, PurchaseOrderID: nullUUID(f.PurchaseOrderID), SupplierID: nullUUID(f.SupplierID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search, RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Advance]{}, fmt.Errorf("list purchase advances: %w", err)
	}
	total, err := s.q(ctx).CountPurchaseAdvances(ctx, sqlcgen.CountPurchaseAdvancesParams{
		Status: status, PurchaseOrderID: nullUUID(f.PurchaseOrderID), SupplierID: nullUUID(f.SupplierID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search,
	})
	if err != nil {
		return paging.Result[domain.Advance]{}, fmt.Errorf("count purchase advances: %w", err)
	}
	items := make([]domain.Advance, len(rows))
	for i, r := range rows {
		items[i] = toAdvance(r)
	}
	return paging.Result[domain.Advance]{Items: items, Page: page, Total: total}, nil
}

// GetAdvance membaca uang muka; forUpdate mengunci barisnya lebih dulu.
func (s *Store) GetAdvance(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Advance, error) {
	if forUpdate {
		if _, err := s.q(ctx).LockPurchaseAdvance(ctx, id); err != nil {
			return domain.Advance{}, notFound(err, "lock purchase advance")
		}
	}
	r, err := s.q(ctx).GetPurchaseAdvance(ctx, id)
	if err != nil {
		return domain.Advance{}, notFound(err, "get purchase advance")
	}
	return toAdvance(sqlcgen.ListPurchaseAdvancesRow(r)), nil
}

// AdvancesByOrder mengembalikan semua uang muka satu PO, urut tanggal bayar.
func (s *Store) AdvancesByOrder(ctx context.Context, orderID uuid.UUID) ([]domain.Advance, error) {
	rows, err := s.q(ctx).ListAdvancesByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list advances by order: %w", err)
	}
	out := make([]domain.Advance, len(rows))
	for i, r := range rows {
		out[i] = toAdvance(sqlcgen.ListPurchaseAdvancesRow(r))
	}
	return out, nil
}

// InsertAdvance menyimpan draft uang muka baru.
func (s *Store) InsertAdvance(ctx context.Context, a domain.Advance, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPurchaseAdvance(ctx, sqlcgen.InsertPurchaseAdvanceParams{
		PurchaseOrderID: a.PurchaseOrderID, SupplierID: a.SupplierID, DivisionID: a.DivisionID, Amount: a.Amount.Decimal(),
		PaymentDate: a.PaymentDate, BankAccountCode: a.BankAccountCode, Note: a.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert purchase advance: %w", err)
	}
	return id, nil
}

// SaveAdvance menyimpan uang muka dengan optimistic locking.
func (s *Store) SaveAdvance(ctx context.Context, a domain.Advance, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdatePurchaseAdvance(ctx, sqlcgen.UpdatePurchaseAdvanceParams{
		Number: text(a.Number), Amount: a.Amount.Decimal(), PaymentDate: a.PaymentDate, BankAccountCode: a.BankAccountCode,
		Note: a.Note, AppliedAmount: a.Applied.Decimal(), Status: string(a.Status), JournalEntryID: nullUUID(a.JournalEntryID),
		ReversalJournalEntryID: nullUUID(a.ReversalJournalEntryID), CancelReason: a.CancelReason, PostedAt: a.PostedAt,
		ActorID: actorID, ID: a.ID, Version: int32Of(version),
	})
	if err != nil {
		return fmt.Errorf("update purchase advance: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
