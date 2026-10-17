package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

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

func pgDate(d *clock.BusinessDate) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func amount(d decimal.Decimal) money.Amount { return money.FromDecimal(d) }

func version(v int) int32 { return int32(v) } //nolint:gosec // versi kecil

// ErrDuplicateExternalOrder: nomor order marketplace sudah tercatat untuk marketplace yang sama.
var ErrDuplicateExternalOrder = errors.New("duplicate external order")

func mapOrderWriteError(err error, what string) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		switch pe.ConstraintName {
		case "uq_sales_orders_external_order":
			return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "Nomor order marketplace sudah tercatat.",
				Fields: map[string]string{"externalOrderId": "Nomor order sudah tercatat untuk marketplace ini"},
				Err:    fmt.Errorf("%w: %w", ErrDuplicateExternalOrder, err)}
		case "uq_sales_orders_import_row_key":
			return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: "Baris impor sudah tersimpan.", Err: err}
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// OrderFilter adalah kriteria daftar pesanan. Types kosong = tidak ada yang tampil (dibatasi permission).
type OrderFilter struct {
	Types        []domain.OrderType
	Status       domain.Status
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	DateFrom     *clock.BusinessDate
	DateTo       *clock.BusinessDate
	Search       string
	Sort         paging.Sort
}

func typesOf(ts []domain.OrderType) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}

func toOrder(r sqlcgen.SalesOrder) domain.Order {
	return domain.Order{
		ID: r.ID, Number: r.Number.String, Type: domain.OrderType(r.OrderType), ChannelID: r.ChannelID,
		SubChannelID: r.SubChannelID, CustomerID: uuidPtr(r.CustomerID), PartnerID: uuidPtr(r.PartnerID),
		PartyCode: r.PartyCode, PartyName: r.PartyName, PartyPhone: r.PartyPhone, ExternalOrderID: r.ExternalOrderID,
		ClientPONumber: r.ClientPoNumber, BusinessDate: r.BusinessDate, SourceLocationID: r.SourceLocationID,
		PaymentMode: domain.PaymentMode(r.PaymentMode), PaymentTermDays: int(r.PaymentTermDays),
		ShipToName: r.ShipToName, ShipToPhone: r.ShipToPhone, ShipToAddress: r.ShipToAddress,
		DocumentDiscount: domain.Discount{Kind: domain.DiscountKind(r.DocumentDiscountKind), Value: r.DocumentDiscountValue},
		Gross:            amount(r.GrossAmount), LineDiscount: amount(r.LineDiscountAmount),
		DocumentDiscountAmount: amount(r.DocumentDiscountAmount), Shipping: amount(r.ShippingAmount),
		Total: amount(r.TotalAmount), DiscountApprovalRequired: r.DiscountApprovalRequired,
		DiscountApprovedAt: r.DiscountApprovedAt, Status: domain.Status(r.Status), Note: r.Note,
		CancelReason: r.CancelReason, ConfirmedAt: r.ConfirmedAt, ShippedAt: r.ShippedAt, DeliveredAt: r.DeliveredAt,
		PaymentProofFileID: uuidPtr(r.PaymentProofFileID), PaymentProofUploadedAt: r.PaymentProofUploadedAt,
		ImportRowKey: r.ImportRowKey.String, AllocationRound: int(r.AllocationRound), CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListOrders mengembalikan daftar pesanan tanpa baris.
func (s *Store) ListOrders(ctx context.Context, f OrderFilter, page paging.Page) (paging.Result[domain.Order], error) {
	status, search := text(string(f.Status)), text(f.Search)
	rows, err := s.q(ctx).ListSalesOrders(ctx, sqlcgen.ListSalesOrdersParams{
		OrderTypes: typesOf(f.Types), Status: status, ChannelID: nullUUID(f.ChannelID), SubChannelID: nullUUID(f.SubChannelID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search, Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Order]{}, fmt.Errorf("list sales orders: %w", err)
	}
	total, err := s.q(ctx).CountSalesOrders(ctx, sqlcgen.CountSalesOrdersParams{
		OrderTypes: typesOf(f.Types), Status: status, ChannelID: nullUUID(f.ChannelID), SubChannelID: nullUUID(f.SubChannelID),
		DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: search,
	})
	if err != nil {
		return paging.Result[domain.Order]{}, fmt.Errorf("count sales orders: %w", err)
	}
	items := make([]domain.Order, len(rows))
	for i, r := range rows {
		items[i] = toOrder(r)
	}
	return paging.Result[domain.Order]{Items: items, Page: page, Total: total}, nil
}

// GetOrder membaca pesanan beserta baris dan alokasi; forUpdate mengunci baris pesanan.
func (s *Store) GetOrder(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Order, error) {
	var (
		r   sqlcgen.SalesOrder
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetSalesOrderForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetSalesOrder(ctx, id)
	}
	if err != nil {
		return domain.Order{}, notFound(err, "get sales order")
	}
	o := toOrder(r)
	lines, err := s.q(ctx).ListSalesOrderLines(ctx, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("list sales order lines: %w", err)
	}
	o.Lines = make([]domain.Line, len(lines))
	for i, l := range lines {
		o.Lines[i] = domain.Line{
			ID: l.ID, LineNo: int(l.LineNo), ProductID: l.ProductID, IsBundle: l.IsBundle, Qty: l.Qty,
			Price: domain.LinePrice{
				UnitPrice: amount(l.UnitPriceAmount), Rule: l.PriceRule, SourceID: uuidPtr(l.PriceSourceID),
				PriceListID: uuidPtr(l.PriceListID), PriceListVersionID: uuidPtr(l.PriceListVersionID),
				Description: l.PriceDescription,
			},
			Discount: domain.Discount{Kind: domain.DiscountKind(l.DiscountKind), Value: l.DiscountValue},
			Gross:    amount(l.GrossAmount), DiscountAmount: amount(l.DiscountAmount), Total: amount(l.LineTotalAmount),
			Note: l.Note,
		}
	}
	allocs, err := s.q(ctx).ListSalesOrderAllocations(ctx, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("list sales order allocations: %w", err)
	}
	batches, err := s.q(ctx).ListSalesOrderAllocationBatches(ctx, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("list allocation batches: %w", err)
	}
	byAlloc := map[uuid.UUID][]domain.AllocationBatch{}
	for _, b := range batches {
		byAlloc[b.AllocationID] = append(byAlloc[b.AllocationID], domain.AllocationBatch{BatchID: uuidPtr(b.BatchID), Qty: b.Qty})
	}
	o.Allocations = make([]domain.Allocation, len(allocs))
	for i, a := range allocs {
		o.Allocations[i] = domain.Allocation{
			ID: a.ID, LineID: a.SalesOrderLineID, ProductID: a.ProductID, LocationID: a.LocationID, Qty: a.Qty,
			ShippedQty: a.ShippedQty, Released: a.Released, Batches: byAlloc[a.ID],
		}
	}
	return o, nil
}

// OrderIDByExternal mencari pesanan marketplace berdasarkan nomor order per sub-channel.
func (s *Store) OrderIDByExternal(ctx context.Context, subChannelID uuid.UUID, externalID string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).SalesOrderIDByExternal(ctx, sqlcgen.SalesOrderIDByExternalParams{SubChannelID: subChannelID, ExternalOrderID: externalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("sales order by external id: %w", err)
	}
	return id, true, nil
}

// OrderIDByImportRow mencari pesanan dari baris impor (idempotensi impor).
func (s *Store) OrderIDByImportRow(ctx context.Context, key string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).SalesOrderIDByImportRowKey(ctx, text(key))
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("sales order by import row: %w", err)
	}
	return id, true, nil
}

func (s *Store) insertLines(ctx context.Context, o *domain.Order) error {
	for i := range o.Lines {
		l := &o.Lines[i]
		id, err := s.q(ctx).InsertSalesOrderLine(ctx, sqlcgen.InsertSalesOrderLineParams{
			SalesOrderID: o.ID, LineNo: int32(l.LineNo), ProductID: l.ProductID, IsBundle: l.IsBundle, Qty: l.Qty, //nolint:gosec // ≤ 200 baris
			UnitPriceAmount: l.Price.UnitPrice.Decimal(), PriceRule: l.Price.Rule, PriceSourceID: nullUUID(l.Price.SourceID),
			PriceListID: nullUUID(l.Price.PriceListID), PriceListVersionID: nullUUID(l.Price.PriceListVersionID),
			PriceDescription: l.Price.Description, DiscountKind: string(l.Discount.Kind), DiscountValue: l.Discount.Value,
			DiscountAmount: l.DiscountAmount.Decimal(), GrossAmount: l.Gross.Decimal(), LineTotalAmount: l.Total.Decimal(),
			Note: l.Note,
		})
		if err != nil {
			return fmt.Errorf("insert sales order line: %w", err)
		}
		l.ID = id
	}
	return nil
}

// InsertOrder menyimpan pesanan baru beserta baris (ID baris diisi).
func (s *Store) InsertOrder(ctx context.Context, o domain.Order, actorID uuid.UUID) (domain.Order, error) {
	var number pgtype.Text
	if o.Number != "" {
		number = text(o.Number)
	}
	r, err := s.q(ctx).InsertSalesOrder(ctx, sqlcgen.InsertSalesOrderParams{
		Number: number, OrderType: string(o.Type), ChannelID: o.ChannelID, SubChannelID: o.SubChannelID,
		CustomerID: nullUUID(o.CustomerID), PartnerID: nullUUID(o.PartnerID), PartyCode: o.PartyCode, PartyName: o.PartyName,
		PartyPhone: o.PartyPhone, ExternalOrderID: o.ExternalOrderID, ClientPoNumber: o.ClientPONumber,
		BusinessDate: o.BusinessDate, SourceLocationID: o.SourceLocationID, PaymentMode: string(o.PaymentMode),
		PaymentTermDays: int32(o.PaymentTermDays), ShipToName: o.ShipToName, ShipToPhone: o.ShipToPhone, //nolint:gosec // ≤ 365
		ShipToAddress: o.ShipToAddress, GrossAmount: o.Gross.Decimal(), LineDiscountAmount: o.LineDiscount.Decimal(),
		DocumentDiscountKind: string(o.DocumentDiscount.Kind), DocumentDiscountValue: o.DocumentDiscount.Value,
		DocumentDiscountAmount: o.DocumentDiscountAmount.Decimal(), ShippingAmount: o.Shipping.Decimal(),
		TotalAmount: o.Total.Decimal(), Note: o.Note, ImportRowKey: text(o.ImportRowKey), ActorID: actorID,
	})
	if err != nil {
		return domain.Order{}, mapOrderWriteError(err, "insert sales order")
	}
	saved := toOrder(r)
	saved.Lines = o.Lines
	if err := s.insertLines(ctx, &saved); err != nil {
		return domain.Order{}, err
	}
	return saved, nil
}

// SaveOrder menyimpan header pesanan dengan optimistic locking; replaceLines menulis ulang baris (draft).
func (s *Store) SaveOrder(ctx context.Context, o *domain.Order, ver int, actorID uuid.UUID, replaceLines bool) error {
	var number pgtype.Text
	if o.Number != "" {
		number = text(o.Number)
	}
	n, err := s.q(ctx).UpdateSalesOrder(ctx, sqlcgen.UpdateSalesOrderParams{
		Number: number, ChannelID: o.ChannelID, SubChannelID: o.SubChannelID, CustomerID: nullUUID(o.CustomerID),
		PartnerID: nullUUID(o.PartnerID), PartyCode: o.PartyCode, PartyName: o.PartyName, PartyPhone: o.PartyPhone,
		ExternalOrderID: o.ExternalOrderID, ClientPoNumber: o.ClientPONumber, BusinessDate: o.BusinessDate,
		SourceLocationID: o.SourceLocationID, PaymentMode: string(o.PaymentMode), PaymentTermDays: int32(o.PaymentTermDays), //nolint:gosec // ≤ 365
		ShipToName: o.ShipToName, ShipToPhone: o.ShipToPhone, ShipToAddress: o.ShipToAddress,
		GrossAmount: o.Gross.Decimal(), LineDiscountAmount: o.LineDiscount.Decimal(),
		DocumentDiscountKind: string(o.DocumentDiscount.Kind), DocumentDiscountValue: o.DocumentDiscount.Value,
		DocumentDiscountAmount: o.DocumentDiscountAmount.Decimal(), ShippingAmount: o.Shipping.Decimal(),
		TotalAmount: o.Total.Decimal(), DiscountApprovalRequired: o.DiscountApprovalRequired,
		DiscountApprovedAt: o.DiscountApprovedAt, Status: string(o.Status), Note: o.Note, CancelReason: o.CancelReason,
		ConfirmedAt: o.ConfirmedAt, ShippedAt: o.ShippedAt, DeliveredAt: o.DeliveredAt,
		PaymentProofFileID: nullUUID(o.PaymentProofFileID), PaymentProofUploadedAt: o.PaymentProofUploadedAt,
		AllocationRound: int32(o.AllocationRound), ActorID: actorID, ID: o.ID, Version: version(ver), //nolint:gosec // kecil
	})
	if err != nil {
		return mapOrderWriteError(err, "update sales order")
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	o.Version = ver + 1
	if !replaceLines {
		return nil
	}
	// Baris hanya ditulis ulang di draft: alokasi lama (sudah dilepas) ikut dihapus.
	if err := s.q(ctx).DeleteReleasedAllocationBatches(ctx, o.ID); err != nil {
		return fmt.Errorf("delete allocation batches: %w", err)
	}
	if err := s.q(ctx).DeleteUnshippedAllocations(ctx, o.ID); err != nil {
		return fmt.Errorf("delete allocations: %w", err)
	}
	o.Allocations = nil
	if err := s.q(ctx).DeleteSalesOrderLines(ctx, o.ID); err != nil {
		return fmt.Errorf("delete sales order lines: %w", err)
	}
	return s.insertLines(ctx, o)
}

// ReplaceAllocations menghapus alokasi yang belum dikirim lalu menyimpan alokasi baru beserta batch FEFO.
func (s *Store) ReplaceAllocations(ctx context.Context, orderID uuid.UUID, allocs []domain.Allocation) error {
	if err := s.q(ctx).DeleteReleasedAllocationBatches(ctx, orderID); err != nil {
		return fmt.Errorf("delete allocation batches: %w", err)
	}
	if err := s.q(ctx).DeleteUnshippedAllocations(ctx, orderID); err != nil {
		return fmt.Errorf("delete allocations: %w", err)
	}
	for _, a := range allocs {
		if err := s.q(ctx).InsertSalesOrderAllocation(ctx, sqlcgen.InsertSalesOrderAllocationParams{
			ID: a.ID, SalesOrderID: orderID, SalesOrderLineID: a.LineID, ProductID: a.ProductID, LocationID: a.LocationID, Qty: a.Qty,
		}); err != nil {
			return fmt.Errorf("insert allocation: %w", err)
		}
		for _, b := range a.Batches {
			if err := s.q(ctx).InsertSalesOrderAllocationBatch(ctx, sqlcgen.InsertSalesOrderAllocationBatchParams{
				AllocationID: a.ID, BatchID: nullUUID(b.BatchID), Qty: b.Qty,
			}); err != nil {
				return fmt.Errorf("insert allocation batch: %w", err)
			}
		}
	}
	return nil
}

// ReplaceAllocationBatches menulis ulang batch satu alokasi (ganti batch karena QC Outbound gagal).
func (s *Store) ReplaceAllocationBatches(ctx context.Context, allocationID uuid.UUID, batches []domain.AllocationBatch) error {
	if err := s.q(ctx).DeleteAllocationBatches(ctx, allocationID); err != nil {
		return fmt.Errorf("delete allocation batches: %w", err)
	}
	for _, b := range batches {
		if err := s.q(ctx).InsertSalesOrderAllocationBatch(ctx, sqlcgen.InsertSalesOrderAllocationBatchParams{
			AllocationID: allocationID, BatchID: nullUUID(b.BatchID), Qty: b.Qty,
		}); err != nil {
			return fmt.Errorf("insert allocation batch: %w", err)
		}
	}
	return nil
}

// SaveAllocationState menyimpan qty dikirim dan tanda dilepas per alokasi.
func (s *Store) SaveAllocationState(ctx context.Context, allocs []domain.Allocation) error {
	for _, a := range allocs {
		if err := s.q(ctx).SetSalesOrderAllocation(ctx, sqlcgen.SetSalesOrderAllocationParams{
			ShippedQty: a.ShippedQty, Released: a.Released, ID: a.ID,
		}); err != nil {
			return fmt.Errorf("set allocation: %w", err)
		}
	}
	return nil
}

// ShipmentBySource membaca pengiriman pesanan dari dokumen sumber tertentu; ok false bila belum ada.
func (s *Store) ShipmentBySource(ctx context.Context, orderID uuid.UUID, sourceType string, sourceID uuid.UUID) (domain.Shipment, bool, error) {
	r, err := s.q(ctx).GetSalesShipmentBySource(ctx, sqlcgen.GetSalesShipmentBySourceParams{
		SalesOrderID: orderID, SourceType: sourceType, SourceID: sourceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Shipment{}, false, nil
	}
	if err != nil {
		return domain.Shipment{}, false, fmt.Errorf("get shipment: %w", err)
	}
	return toShipment(r), true, nil
}

func toShipment(r sqlcgen.SalesShipment) domain.Shipment {
	return domain.Shipment{
		ID: r.ID, OrderID: r.SalesOrderID, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
		BusinessDate: r.BusinessDate, Cost: amount(r.CostAmount), CreatedAt: r.CreatedAt,
	}
}

// InsertShipment menyimpan pengiriman beserta barisnya.
func (s *Store) InsertShipment(ctx context.Context, sh domain.Shipment, actorID uuid.UUID) (domain.Shipment, error) {
	r, err := s.q(ctx).InsertSalesShipment(ctx, sqlcgen.InsertSalesShipmentParams{
		SalesOrderID: sh.OrderID, SourceType: sh.SourceType, SourceID: sh.SourceID, SourceLabel: sh.SourceLabel,
		BusinessDate: sh.BusinessDate, CostAmount: sh.Cost.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return domain.Shipment{}, fmt.Errorf("insert shipment: %w", err)
	}
	out := toShipment(r)
	for _, l := range sh.Lines {
		if err := s.q(ctx).InsertSalesShipmentLine(ctx, sqlcgen.InsertSalesShipmentLineParams{
			ShipmentID: out.ID, AllocationID: l.AllocationID, ProductID: l.ProductID, Qty: l.Qty, CostAmount: l.Cost.Decimal(),
		}); err != nil {
			return domain.Shipment{}, fmt.Errorf("insert shipment line: %w", err)
		}
	}
	out.Lines = sh.Lines
	return out, nil
}

// ShipmentsByOrder membaca pengiriman pesanan beserta baris.
func (s *Store) ShipmentsByOrder(ctx context.Context, orderID uuid.UUID) ([]domain.Shipment, error) {
	rows, err := s.q(ctx).ListSalesShipments(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list shipments: %w", err)
	}
	lines, err := s.q(ctx).ListSalesShipmentLines(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list shipment lines: %w", err)
	}
	out := make([]domain.Shipment, len(rows))
	idx := map[uuid.UUID]int{}
	for i, r := range rows {
		out[i] = toShipment(r)
		idx[r.ID] = i
	}
	for _, l := range lines {
		i := idx[l.ShipmentID]
		out[i].Lines = append(out[i].Lines, domain.ShipmentLine{
			AllocationID: l.AllocationID, ProductID: l.ProductID, Qty: l.Qty, Cost: amount(l.CostAmount),
		})
	}
	return out, nil
}

// --- Proforma Invoice ------------------------------------------------------------------------

// ProformaFilter adalah kriteria daftar PI.
type ProformaFilter struct {
	Status domain.Status
	Search string
}

func toProforma(r sqlcgen.ProformaInvoice) domain.Proforma {
	return domain.Proforma{
		ID: r.ID, Number: r.Number, OrderID: r.SalesOrderID, Amount: amount(r.Amount), IssueDate: r.IssueDate,
		ValidUntil: r.ValidUntil, Status: domain.Status(r.Status), PaidAt: r.PaidAt, InvoiceID: uuidPtr(r.InvoiceID),
		CancelReason: r.CancelReason, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ProformaRow adalah PI beserta ringkasan pesanan untuk daftar.
type ProformaRow struct {
	domain.Proforma
	PartyName string
	OrderType domain.OrderType
}

func proformaRow(r sqlcgen.ListProformasRow) ProformaRow {
	p := toProforma(sqlcgen.ProformaInvoice{
		ID: r.ID, Number: r.Number, SalesOrderID: r.SalesOrderID, Amount: r.Amount, IssueDate: r.IssueDate,
		ValidUntil: r.ValidUntil, Status: r.Status, PaidAt: r.PaidAt, InvoiceID: r.InvoiceID, CancelReason: r.CancelReason,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	})
	p.OrderNumber = r.OrderNumber.String
	return ProformaRow{Proforma: p, PartyName: r.PartyName, OrderType: domain.OrderType(r.OrderType)}
}

// ListProformas mengembalikan daftar PI terbaru lebih dulu.
func (s *Store) ListProformas(ctx context.Context, f ProformaFilter, page paging.Page) (paging.Result[ProformaRow], error) {
	status, search := text(string(f.Status)), text(f.Search)
	rows, err := s.q(ctx).ListProformas(ctx, sqlcgen.ListProformasParams{
		Status: status, Search: search, RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[ProformaRow]{}, fmt.Errorf("list proformas: %w", err)
	}
	total, err := s.q(ctx).CountProformas(ctx, sqlcgen.CountProformasParams{Status: status, Search: search})
	if err != nil {
		return paging.Result[ProformaRow]{}, fmt.Errorf("count proformas: %w", err)
	}
	items := make([]ProformaRow, len(rows))
	for i, r := range rows {
		items[i] = proformaRow(r)
	}
	return paging.Result[ProformaRow]{Items: items, Page: page, Total: total}, nil
}

// GetProforma membaca PI beserta ringkasan pesanan.
func (s *Store) GetProforma(ctx context.Context, id uuid.UUID) (ProformaRow, error) {
	r, err := s.q(ctx).GetProforma(ctx, id)
	if err != nil {
		return ProformaRow{}, notFound(err, "get proforma")
	}
	return proformaRow(sqlcgen.ListProformasRow(r)), nil
}

// LockProforma mengunci PI.
func (s *Store) LockProforma(ctx context.Context, id uuid.UUID) (domain.Proforma, error) {
	r, err := s.q(ctx).GetProformaForUpdate(ctx, id)
	if err != nil {
		return domain.Proforma{}, notFound(err, "lock proforma")
	}
	return toProforma(r), nil
}

// ProformasByOrder membaca PI satu pesanan.
func (s *Store) ProformasByOrder(ctx context.Context, orderID uuid.UUID) ([]domain.Proforma, error) {
	rows, err := s.q(ctx).ProformasByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("proformas by order: %w", err)
	}
	out := make([]domain.Proforma, len(rows))
	for i, r := range rows {
		out[i] = toProforma(r)
	}
	return out, nil
}

// InsertProforma menyimpan PI baru.
func (s *Store) InsertProforma(ctx context.Context, p domain.Proforma, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertProforma(ctx, sqlcgen.InsertProformaParams{
		Number: p.Number, SalesOrderID: p.OrderID, Amount: p.Amount.Decimal(), IssueDate: p.IssueDate,
		ValidUntil: p.ValidUntil, Status: string(p.Status), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert proforma: %w", err)
	}
	return id, nil
}

// SaveProforma menyimpan status PI dengan optimistic locking.
func (s *Store) SaveProforma(ctx context.Context, p domain.Proforma, ver int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateProforma(ctx, sqlcgen.UpdateProformaParams{
		Status: string(p.Status), PaidAt: p.PaidAt, InvoiceID: nullUUID(p.InvoiceID), CancelReason: p.CancelReason,
		ActorID: actorID, ID: p.ID, Version: version(ver),
	})
	if err != nil {
		return fmt.Errorf("update proforma: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// ExpiredProformaIDs mengembalikan PI belum dibayar yang lewat masa berlaku (maks. 200 per panggilan).
func (s *Store) ExpiredProformaIDs(ctx context.Context, today clock.BusinessDate) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).ExpiredProformaIDs(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("expired proformas: %w", err)
	}
	return ids, nil
}
