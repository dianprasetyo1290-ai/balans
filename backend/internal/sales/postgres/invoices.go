package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

// InvoiceFilter adalah kriteria daftar invoice.
type InvoiceFilter struct {
	Statuses  []domain.Status
	PartyType string
	PartyID   *uuid.UUID
	DateFrom  *clock.BusinessDate
	DateTo    *clock.BusinessDate
	// OverdueOn: hanya invoice terbuka yang jatuh tempo sebelum tanggal ini.
	OverdueOn *clock.BusinessDate
	Search    string
	Sort      paging.Sort
}

func statusesOf(ss []domain.Status) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

func toInvoice(r sqlcgen.ListInvoicesRow) domain.Invoice {
	return domain.Invoice{
		ID: r.ID, Number: r.Number, Type: r.InvoiceType, OrderID: uuidPtr(r.SalesOrderID), OrderNumber: r.OrderNumber,
		PartyType: r.PartyType, PartyID: r.PartyID, PartyCode: r.PartyCode, PartyName: r.PartyName, ChannelID: r.ChannelID,
		SubChannelID: r.SubChannelID, InvoiceDate: r.InvoiceDate, DueDate: r.DueDate, Gross: amount(r.GrossAmount),
		Discount: amount(r.DiscountAmount), Shipping: amount(r.ShippingAmount), Total: amount(r.TotalAmount),
		Paid: amount(r.PaidAmount), Status: domain.Status(r.Status), CancelReason: r.CancelReason,
		CancelledAt: r.CancelledAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func plainInvoiceRow(r sqlcgen.Invoice) sqlcgen.ListInvoicesRow {
	return sqlcgen.ListInvoicesRow{
		ID: r.ID, Number: r.Number, InvoiceType: r.InvoiceType, SalesOrderID: r.SalesOrderID, PartyType: r.PartyType,
		PartyID: r.PartyID, PartyCode: r.PartyCode, PartyName: r.PartyName, ChannelID: r.ChannelID,
		SubChannelID: r.SubChannelID, InvoiceDate: r.InvoiceDate, DueDate: r.DueDate, GrossAmount: r.GrossAmount,
		DiscountAmount: r.DiscountAmount, ShippingAmount: r.ShippingAmount, TotalAmount: r.TotalAmount,
		PaidAmount: r.PaidAmount, Status: r.Status, CancelReason: r.CancelReason, CancelledAt: r.CancelledAt,
		CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy, Version: r.Version,
	}
}

// ListInvoices mengembalikan daftar invoice tanpa baris.
func (s *Store) ListInvoices(ctx context.Context, f InvoiceFilter, page paging.Page) (paging.Result[domain.Invoice], error) {
	search, partyType := text(f.Search), text(f.PartyType)
	rows, err := s.q(ctx).ListInvoices(ctx, sqlcgen.ListInvoicesParams{
		Statuses: statusesOf(f.Statuses), PartyType: partyType, PartyID: nullUUID(f.PartyID), DateFrom: pgDate(f.DateFrom),
		DateTo: pgDate(f.DateTo), OverdueOn: pgDate(f.OverdueOn), Search: search, Sort: f.Sort.Key(),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Invoice]{}, fmt.Errorf("list invoices: %w", err)
	}
	total, err := s.q(ctx).CountInvoices(ctx, sqlcgen.CountInvoicesParams{
		Statuses: statusesOf(f.Statuses), PartyType: partyType, PartyID: nullUUID(f.PartyID), DateFrom: pgDate(f.DateFrom),
		DateTo: pgDate(f.DateTo), OverdueOn: pgDate(f.OverdueOn), Search: search,
	})
	if err != nil {
		return paging.Result[domain.Invoice]{}, fmt.Errorf("count invoices: %w", err)
	}
	items := make([]domain.Invoice, len(rows))
	for i, r := range rows {
		items[i] = toInvoice(r)
	}
	return paging.Result[domain.Invoice]{Items: items, Page: page, Total: total}, nil
}

// OpenInvoices mengembalikan invoice dengan sisa piutang yang terbit sampai asOf (umur piutang).
func (s *Store) OpenInvoices(ctx context.Context, asOf clock.BusinessDate, partyType string) ([]domain.Invoice, error) {
	rows, err := s.q(ctx).OpenInvoices(ctx, sqlcgen.OpenInvoicesParams{AsOf: asOf, PartyType: text(partyType)})
	if err != nil {
		return nil, fmt.Errorf("open invoices: %w", err)
	}
	out := make([]domain.Invoice, len(rows))
	for i, r := range rows {
		out[i] = toInvoice(sqlcgen.ListInvoicesRow(r))
	}
	return out, nil
}

func (s *Store) invoiceLines(ctx context.Context, inv *domain.Invoice) error {
	lines, err := s.q(ctx).ListInvoiceLines(ctx, inv.ID)
	if err != nil {
		return fmt.Errorf("list invoice lines: %w", err)
	}
	inv.Lines = make([]domain.InvoiceLine, len(lines))
	for i, l := range lines {
		inv.Lines[i] = domain.InvoiceLine{
			ID: l.ID, LineNo: int(l.LineNo), ProductID: uuidPtr(l.ProductID), Description: l.Description, Qty: l.Qty,
			UnitPrice: amount(l.UnitPriceAmount), Discount: amount(l.DiscountAmount), Total: amount(l.LineTotalAmount),
		}
	}
	return nil
}

// GetInvoice membaca invoice beserta baris.
func (s *Store) GetInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	r, err := s.q(ctx).GetInvoice(ctx, id)
	if err != nil {
		return domain.Invoice{}, notFound(err, "get invoice")
	}
	inv := toInvoice(sqlcgen.ListInvoicesRow(r))
	return inv, s.invoiceLines(ctx, &inv)
}

// LockInvoice mengunci invoice (tanpa baris).
func (s *Store) LockInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	r, err := s.q(ctx).GetInvoiceForUpdate(ctx, id)
	if err != nil {
		return domain.Invoice{}, notFound(err, "lock invoice")
	}
	return toInvoice(plainInvoiceRow(r)), nil
}

// InvoicesByOrder membaca invoice satu pesanan.
func (s *Store) InvoicesByOrder(ctx context.Context, orderID uuid.UUID) ([]domain.Invoice, error) {
	rows, err := s.q(ctx).InvoicesByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("invoices by order: %w", err)
	}
	out := make([]domain.Invoice, len(rows))
	for i, r := range rows {
		out[i] = toInvoice(plainInvoiceRow(r))
	}
	return out, nil
}

// InsertInvoice menyimpan invoice beserta baris.
func (s *Store) InsertInvoice(ctx context.Context, inv domain.Invoice, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertInvoice(ctx, sqlcgen.InsertInvoiceParams{
		Number: inv.Number, InvoiceType: inv.Type, SalesOrderID: nullUUID(inv.OrderID), PartyType: inv.PartyType,
		PartyID: inv.PartyID, PartyCode: inv.PartyCode, PartyName: inv.PartyName, ChannelID: inv.ChannelID,
		SubChannelID: inv.SubChannelID, InvoiceDate: inv.InvoiceDate, DueDate: inv.DueDate,
		GrossAmount: inv.Gross.Decimal(), DiscountAmount: inv.Discount.Decimal(), ShippingAmount: inv.Shipping.Decimal(),
		TotalAmount: inv.Total.Decimal(), PaidAmount: inv.Paid.Decimal(), Status: string(inv.Status), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert invoice: %w", err)
	}
	for _, l := range inv.Lines {
		if err := s.q(ctx).InsertInvoiceLine(ctx, sqlcgen.InsertInvoiceLineParams{
			InvoiceID: id, LineNo: int32(l.LineNo), ProductID: nullUUID(l.ProductID), Description: l.Description, Qty: l.Qty, //nolint:gosec // ≤ 200
			UnitPriceAmount: l.UnitPrice.Decimal(), DiscountAmount: l.Discount.Decimal(), LineTotalAmount: l.Total.Decimal(),
		}); err != nil {
			return uuid.Nil, fmt.Errorf("insert invoice line: %w", err)
		}
	}
	return id, nil
}

// SaveInvoice menyimpan nominal terbayar dan status invoice dengan optimistic locking.
func (s *Store) SaveInvoice(ctx context.Context, inv domain.Invoice, ver int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateInvoicePayment(ctx, sqlcgen.UpdateInvoicePaymentParams{
		PaidAmount: inv.Paid.Decimal(), Status: string(inv.Status), CancelReason: inv.CancelReason,
		CancelledAt: inv.CancelledAt, ActorID: actorID, ID: inv.ID, Version: version(ver),
	})
	if err != nil {
		return fmt.Errorf("update invoice: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// InvoicePayment adalah pembayaran yang dialokasikan ke satu invoice (riwayat bayar di detail invoice).
type InvoicePayment struct {
	PaymentID     uuid.UUID
	PaymentNumber string
	PaymentDate   clock.BusinessDate
	Status        domain.Status
	Amount        money.Amount
}

// PaymentsByInvoice membaca pembayaran yang dialokasikan ke invoice.
func (s *Store) PaymentsByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]InvoicePayment, error) {
	rows, err := s.q(ctx).PaymentAllocationsByInvoice(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("payments by invoice: %w", err)
	}
	out := make([]InvoicePayment, len(rows))
	for i, r := range rows {
		out[i] = InvoicePayment{
			PaymentID: r.PaymentID, PaymentNumber: r.PaymentNumber, PaymentDate: r.PaymentDate,
			Status: domain.Status(r.PaymentStatus), Amount: amount(r.Amount),
		}
	}
	return out, nil
}

// --- Pembayaran pelanggan --------------------------------------------------------------------

// PaymentFilter adalah kriteria daftar pembayaran.
type PaymentFilter struct {
	Status      domain.Status
	PartyType   string
	PartyID     *uuid.UUID
	AccountCode string
	DateFrom    *clock.BusinessDate
	DateTo      *clock.BusinessDate
	Search      string
}

func toPayment(r sqlcgen.CustomerPayment) domain.CustomerPayment {
	return domain.CustomerPayment{
		ID: r.ID, Number: r.Number, PartyType: r.PartyType, PartyID: r.PartyID, PartyName: r.PartyName,
		PaymentDate: r.PaymentDate, Amount: amount(r.Amount), Method: domain.PaymentMethod(r.Method),
		AccountCode: r.AccountCode, Reference: r.Reference, ProofFileID: uuidPtr(r.ProofFileID), Note: r.Note,
		Status: domain.Status(r.Status), CancelReason: r.CancelReason, JournalEntryID: uuidPtr(r.JournalEntryID),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) attachAllocations(ctx context.Context, list []domain.CustomerPayment) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	idx := map[uuid.UUID]int{}
	for i, p := range list {
		ids[i], idx[p.ID] = p.ID, i
	}
	rows, err := s.q(ctx).ListPaymentAllocations(ctx, ids)
	if err != nil {
		return fmt.Errorf("list payment allocations: %w", err)
	}
	for _, r := range rows {
		i := idx[r.PaymentID]
		list[i].Allocations = append(list[i].Allocations, domain.PaymentAllocation{
			InvoiceID: r.InvoiceID, InvoiceNumber: r.InvoiceNumber, Amount: amount(r.Amount),
		})
	}
	return nil
}

// ListPayments mengembalikan daftar pembayaran beserta alokasinya.
func (s *Store) ListPayments(ctx context.Context, f PaymentFilter, page paging.Page) (paging.Result[domain.CustomerPayment], error) {
	p := sqlcgen.ListCustomerPaymentsParams{
		Status: text(string(f.Status)), PartyType: text(f.PartyType), PartyID: nullUUID(f.PartyID),
		AccountCode: text(f.AccountCode), DateFrom: pgDate(f.DateFrom), DateTo: pgDate(f.DateTo), Search: text(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	}
	rows, err := s.q(ctx).ListCustomerPayments(ctx, p)
	if err != nil {
		return paging.Result[domain.CustomerPayment]{}, fmt.Errorf("list payments: %w", err)
	}
	total, err := s.q(ctx).CountCustomerPayments(ctx, sqlcgen.CountCustomerPaymentsParams{
		Status: p.Status, PartyType: p.PartyType, PartyID: p.PartyID, AccountCode: p.AccountCode, DateFrom: p.DateFrom,
		DateTo: p.DateTo, Search: p.Search,
	})
	if err != nil {
		return paging.Result[domain.CustomerPayment]{}, fmt.Errorf("count payments: %w", err)
	}
	items := make([]domain.CustomerPayment, len(rows))
	for i, r := range rows {
		items[i] = toPayment(r)
	}
	if err := s.attachAllocations(ctx, items); err != nil {
		return paging.Result[domain.CustomerPayment]{}, err
	}
	return paging.Result[domain.CustomerPayment]{Items: items, Page: page, Total: total}, nil
}

// GetPayment membaca pembayaran beserta alokasi; forUpdate mengunci baris.
func (s *Store) GetPayment(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.CustomerPayment, error) {
	var (
		r   sqlcgen.CustomerPayment
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetCustomerPaymentForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetCustomerPayment(ctx, id)
	}
	if err != nil {
		return domain.CustomerPayment{}, notFound(err, "get payment")
	}
	list := []domain.CustomerPayment{toPayment(r)}
	if err := s.attachAllocations(ctx, list); err != nil {
		return domain.CustomerPayment{}, err
	}
	return list[0], nil
}

// InsertPayment menyimpan pembayaran beserta alokasi.
func (s *Store) InsertPayment(ctx context.Context, p domain.CustomerPayment, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertCustomerPayment(ctx, sqlcgen.InsertCustomerPaymentParams{
		Number: p.Number, PartyType: p.PartyType, PartyID: p.PartyID, PartyName: p.PartyName, PaymentDate: p.PaymentDate,
		Amount: p.Amount.Decimal(), Method: string(p.Method), AccountCode: p.AccountCode, Reference: p.Reference,
		ProofFileID: nullUUID(p.ProofFileID), Note: p.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert payment: %w", err)
	}
	for _, a := range p.Allocations {
		if err := s.q(ctx).InsertPaymentAllocation(ctx, sqlcgen.InsertPaymentAllocationParams{
			PaymentID: id, InvoiceID: a.InvoiceID, Amount: a.Amount.Decimal(),
		}); err != nil {
			return uuid.Nil, fmt.Errorf("insert payment allocation: %w", err)
		}
	}
	return id, nil
}

// SavePayment menyimpan status dan jurnal pembayaran dengan optimistic locking.
func (s *Store) SavePayment(ctx context.Context, p domain.CustomerPayment, ver int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateCustomerPayment(ctx, sqlcgen.UpdateCustomerPaymentParams{
		Status: string(p.Status), CancelReason: p.CancelReason, JournalEntryID: nullUUID(p.JournalEntryID),
		ActorID: actorID, ID: p.ID, Version: version(ver),
	})
	if err != nil {
		return fmt.Errorf("update payment: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
