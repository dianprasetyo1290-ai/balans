package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres/sqlcgen"
)

// --- Read model laporan penjualan (TG 5.17) --------------------------------------------------

// RefreshSalesReadModel me-refresh rm_sales_lines tanpa mengunci pembaca (dipanggil job accounting).
func (s *Store) RefreshSalesReadModel(ctx context.Context) error {
	if _, err := db.Conn(ctx, s.pool).Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY rm_sales_lines"); err != nil {
		return fmt.Errorf("refresh rm_sales_lines: %w", err)
	}
	return nil
}

// SalesReportFilter adalah kriteria laporan penjualan & margin.
type SalesReportFilter struct {
	GroupBy      string
	DateFrom     clock.BusinessDate
	DateTo       clock.BusinessDate
	ChannelID    *uuid.UUID
	SubChannelID *uuid.UUID
	ProductID    *uuid.UUID
	OrderType    string
}

// SalesReportRow adalah ringkasan penjualan satu kunci.
type SalesReportRow struct {
	Key      string
	Qty      money.Amount
	Gross    money.Amount
	Discount money.Amount
	Net      money.Amount
	COGSQty  money.Amount
	COGS     money.Amount
}

// SalesReport membaca ringkasan rm_sales_lines per kunci pengelompokan.
func (s *Store) SalesReport(ctx context.Context, f SalesReportFilter) ([]SalesReportRow, error) {
	rows, err := s.q(ctx).SalesReport(ctx, sqlcgen.SalesReportParams{
		GroupBy: f.GroupBy, DateFrom: f.DateFrom, DateTo: f.DateTo, ChannelID: nullUUID(f.ChannelID),
		SubChannelID: nullUUID(f.SubChannelID), ProductID: nullUUID(f.ProductID), OrderType: text(f.OrderType),
	})
	if err != nil {
		return nil, fmt.Errorf("sales report: %w", err)
	}
	out := make([]SalesReportRow, len(rows))
	for i, r := range rows {
		out[i] = SalesReportRow{
			Key: r.GroupKey, Qty: amount(r.Qty), Gross: amount(r.GrossAmount), Discount: amount(r.DiscountAmount),
			Net: amount(r.NetAmount), COGSQty: amount(r.CogsQty), COGS: amount(r.CogsAmount),
		}
	}
	return out, nil
}

// ConsignmentRow adalah invoice konsinyasi/selisih satu mitra.
type ConsignmentRow struct {
	PartyID      uuid.UUID
	PartyCode    string
	PartyName    string
	InvoiceType  string
	InvoiceCount int64
	Gross        money.Amount
	Discount     money.Amount
	Total        money.Amount
	Paid         money.Amount
}

// ConsignmentReport membaca invoice konsinyasi per mitra.
func (s *Store) ConsignmentReport(ctx context.Context, from, to clock.BusinessDate) ([]ConsignmentRow, error) {
	rows, err := s.q(ctx).ConsignmentReport(ctx, sqlcgen.ConsignmentReportParams{DateFrom: from, DateTo: to})
	if err != nil {
		return nil, fmt.Errorf("consignment report: %w", err)
	}
	out := make([]ConsignmentRow, len(rows))
	for i, r := range rows {
		out[i] = ConsignmentRow{
			PartyID: r.PartyID, PartyCode: r.PartyCode, PartyName: r.PartyName, InvoiceType: r.InvoiceType,
			InvoiceCount: r.InvoiceCount, Gross: amount(r.GrossAmount), Discount: amount(r.DiscountAmount),
			Total: amount(r.TotalAmount), Paid: amount(r.PaidAmount),
		}
	}
	return out, nil
}

// --- Settlement marketplace (S-403) ----------------------------------------------------------

// MarketplaceOrderForSettlement mengunci order marketplace berdasarkan nomor order marketplace; ok false bila
// tidak ada.
func (s *Store) MarketplaceOrderForSettlement(ctx context.Context, subChannelID uuid.UUID, externalOrderID string) (domain.MarketplaceOrderRef, bool, error) {
	r, err := s.q(ctx).MarketplaceOrderForSettlement(ctx, sqlcgen.MarketplaceOrderForSettlementParams{
		SubChannelID: subChannelID, ExternalOrderID: externalOrderID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MarketplaceOrderRef{}, false, nil
	}
	if err != nil {
		return domain.MarketplaceOrderRef{}, false, fmt.Errorf("marketplace order for settlement: %w", err)
	}
	return domain.MarketplaceOrderRef{
		ID: r.ID, Number: r.Number, ChannelID: r.ChannelID, SubChannelID: r.SubChannelID, Total: amount(r.TotalAmount),
		Status: domain.Status(r.Status), HandedOver: r.HandedOver, AlreadySettled: amount(r.SettledAmount),
	}, true, nil
}

func toSettlement(r sqlcgen.MarketplaceSettlement) domain.Settlement {
	return domain.Settlement{
		ID: r.ID, ChannelID: r.ChannelID, SubChannelID: r.SubChannelID, MarketplaceCode: r.MarketplaceCode,
		PayoutReference: r.PayoutReference, SettlementDate: r.SettlementDate, BankAccountCode: r.BankAccountCode,
		Status: domain.SettlementStatus(r.Status), LineCount: int(r.LineCount), Settled: amount(r.SettledAmount),
		Fee: amount(r.FeeAmount), Promo: amount(r.PromoAmount), Net: amount(r.NetAmount),
		JournalEntryID: uuidPtr(r.JournalEntryID), PostedAt: r.PostedAt, CancelReason: r.CancelReason,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// SettlementByPayout mengunci settlement per marketplace + referensi pencairan; ok false bila belum ada.
func (s *Store) SettlementByPayout(ctx context.Context, subChannelID uuid.UUID, ref string) (domain.Settlement, bool, error) {
	r, err := s.q(ctx).GetSettlementByPayout(ctx, sqlcgen.GetSettlementByPayoutParams{SubChannelID: subChannelID, PayoutReference: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settlement{}, false, nil
	}
	if err != nil {
		return domain.Settlement{}, false, fmt.Errorf("get settlement by payout: %w", err)
	}
	return toSettlement(r), true, nil
}

// InsertSettlement membuat settlement draft.
func (s *Store) InsertSettlement(ctx context.Context, st domain.Settlement, actorID uuid.UUID) (domain.Settlement, error) {
	r, err := s.q(ctx).InsertSettlement(ctx, sqlcgen.InsertSettlementParams{
		ChannelID: st.ChannelID, SubChannelID: st.SubChannelID, MarketplaceCode: st.MarketplaceCode,
		PayoutReference: st.PayoutReference, SettlementDate: st.SettlementDate, BankAccountCode: st.BankAccountCode,
		ActorID: actorID,
	})
	if err != nil {
		return domain.Settlement{}, fmt.Errorf("insert settlement: %w", err)
	}
	return toSettlement(r), nil
}

// AddSettlementLine menambah order ke settlement dan menghitung ulang total.
func (s *Store) AddSettlementLine(ctx context.Context, settlementID uuid.UUID, l domain.SettlementLine, rowKey string, actorID uuid.UUID) error {
	err := s.q(ctx).InsertSettlementLine(ctx, sqlcgen.InsertSettlementLineParams{
		SettlementID: settlementID, SalesOrderID: l.OrderID, ExternalOrderID: l.ExternalOrderID, OrderNumber: l.OrderNumber,
		ReceivableAmount: l.Receivable.Decimal(), FeeAmount: l.Fee.Decimal(), PromoAmount: l.Promo.Decimal(),
		NetAmount: l.Net.Decimal(), SettledAmount: l.Settled.Decimal(), ImportRowKey: text(rowKey),
	})
	if err != nil {
		if isUnique(err, "uq_marketplace_settlement_lines_order") {
			return apperr.Validation(map[string]string{"order_no": "Order sudah ada di settlement ini"})
		}
		return fmt.Errorf("insert settlement line: %w", err)
	}
	if err := s.q(ctx).RecalcSettlement(ctx, sqlcgen.RecalcSettlementParams{ID: settlementID, ActorID: actorID}); err != nil {
		return fmt.Errorf("recalc settlement: %w", err)
	}
	return nil
}

// GetSettlement membaca settlement (forUpdate mengunci baris).
func (s *Store) GetSettlement(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Settlement, error) {
	var (
		r   sqlcgen.MarketplaceSettlement
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetSettlementForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetSettlement(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settlement{}, apperr.New(apperr.CodeNotFound)
	}
	if err != nil {
		return domain.Settlement{}, fmt.Errorf("get settlement: %w", err)
	}
	return toSettlement(r), nil
}

// SettlementLines membaca baris settlement.
func (s *Store) SettlementLines(ctx context.Context, id uuid.UUID) ([]domain.SettlementLine, error) {
	rows, err := s.q(ctx).ListSettlementLines(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list settlement lines: %w", err)
	}
	out := make([]domain.SettlementLine, len(rows))
	for i, r := range rows {
		out[i] = domain.SettlementLine{
			LineNo: int(r.LineNo), OrderID: r.SalesOrderID, ExternalOrderID: r.ExternalOrderID, OrderNumber: r.OrderNumber,
			Receivable: amount(r.OrderTotal), Fee: amount(r.FeeAmount), Promo: amount(r.PromoAmount), Net: amount(r.NetAmount),
			Settled: amount(r.SettledAmount), PostedSettled: amount(r.PostedSettled),
		}
	}
	return out, nil
}

// SettlementFilter adalah kriteria daftar settlement.
type SettlementFilter struct {
	Status       string
	SubChannelID *uuid.UUID
}

// ListSettlements membaca satu halaman settlement.
func (s *Store) ListSettlements(ctx context.Context, f SettlementFilter, page paging.Page) (paging.Result[domain.Settlement], error) {
	rows, err := s.q(ctx).ListSettlements(ctx, sqlcgen.ListSettlementsParams{
		Status: text(f.Status), SubChannelID: nullUUID(f.SubChannelID), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Settlement]{}, fmt.Errorf("list settlements: %w", err)
	}
	total, err := s.q(ctx).CountSettlements(ctx, sqlcgen.CountSettlementsParams{Status: text(f.Status), SubChannelID: nullUUID(f.SubChannelID)})
	if err != nil {
		return paging.Result[domain.Settlement]{}, fmt.Errorf("count settlements: %w", err)
	}
	out := make([]domain.Settlement, len(rows))
	for i, r := range rows {
		out[i] = toSettlement(r)
	}
	return paging.Result[domain.Settlement]{Items: out, Page: page, Total: total}, nil
}

// SetSettlementStatus memindah settlement draft ke diposting/dibatalkan (optimistic locking).
func (s *Store) SetSettlementStatus(ctx context.Context, st domain.Settlement, journalID *uuid.UUID, postedAt *time.Time, actorID uuid.UUID) error {
	p := sqlcgen.UpdateSettlementStatusParams{
		Status: string(st.Status), JournalEntryID: nullUUID(journalID), PostedAt: postedAt, CancelReason: st.CancelReason,
		ActorID: actorID, ID: st.ID, Version: version(st.Version),
	}
	if postedAt != nil {
		p.PostedBy = uuid.NullUUID{UUID: actorID, Valid: true}
	}
	n, err := s.q(ctx).UpdateSettlementStatus(ctx, p)
	if err != nil {
		return fmt.Errorf("update settlement status: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// MarketplaceReceivableFilter adalah kriteria sisa piutang marketplace per order.
type MarketplaceReceivableFilter struct {
	SubChannelID *uuid.UUID
	OpenOnly     bool
	Search       string
}

// MarketplaceReceivable adalah sisa piutang satu order marketplace.
type MarketplaceReceivable struct {
	OrderID         uuid.UUID
	Number          string
	ExternalOrderID string
	SubChannelID    uuid.UUID
	OrderDate       clock.BusinessDate
	HandedOverDate  clock.BusinessDate
	BuyerName       string
	Receivable      money.Amount
	Settled         money.Amount
	Draft           money.Amount
	Outstanding     money.Amount
}

// MarketplaceReceivableTotals adalah ringkasan filter sisa piutang marketplace.
type MarketplaceReceivableTotals struct {
	Rows        int64
	Receivable  money.Amount
	Settled     money.Amount
	Outstanding money.Amount
}

// ListMarketplaceReceivables membaca sisa piutang marketplace per order.
func (s *Store) ListMarketplaceReceivables(ctx context.Context, f MarketplaceReceivableFilter, page paging.Page) ([]MarketplaceReceivable, MarketplaceReceivableTotals, error) {
	rows, err := s.q(ctx).ListMarketplaceReceivables(ctx, sqlcgen.ListMarketplaceReceivablesParams{
		SubChannelID: nullUUID(f.SubChannelID), OpenOnly: f.OpenOnly, Search: text(f.Search),
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return nil, MarketplaceReceivableTotals{}, fmt.Errorf("list marketplace receivables: %w", err)
	}
	t, err := s.q(ctx).MarketplaceReceivableTotals(ctx, sqlcgen.MarketplaceReceivableTotalsParams{
		SubChannelID: nullUUID(f.SubChannelID), OpenOnly: f.OpenOnly, Search: text(f.Search),
	})
	if err != nil {
		return nil, MarketplaceReceivableTotals{}, fmt.Errorf("marketplace receivable totals: %w", err)
	}
	out := make([]MarketplaceReceivable, len(rows))
	for i, r := range rows {
		out[i] = MarketplaceReceivable{
			OrderID: r.ID, Number: r.Number, ExternalOrderID: r.ExternalOrderID, SubChannelID: r.SubChannelID,
			OrderDate: r.BusinessDate, HandedOverDate: r.HandedOverDate, BuyerName: r.PartyName,
			Receivable: amount(r.TotalAmount), Settled: amount(r.SettledAmount), Draft: amount(r.DraftAmount),
			Outstanding: amount(r.OutstandingAmount),
		}
	}
	return out, MarketplaceReceivableTotals{
		Rows: t.TotalRows, Receivable: amount(t.ReceivableAmount), Settled: amount(t.SettledAmount),
		Outstanding: amount(t.OutstandingAmount),
	}, nil
}

func isUnique(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == constraint
}
