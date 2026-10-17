package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
)

// Read model laporan (TG 5.17). rm_gl_daily menyimpan dimensi kosong sebagai UUID nol.

// MovementRow adalah mutasi debit − kredit satu akun sebelum dan selama periode.
type MovementRow struct {
	AccountID uuid.UUID
	Before    money.Amount
	During    money.Amount
}

func dimArg(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

// ReportMovements membaca mutasi per akun dari rm_gl_daily.
func (s *Store) ReportMovements(ctx context.Context, from, to clock.BusinessDate, d domain.Dimensions) ([]MovementRow, error) {
	rows, err := s.q(ctx).ReportMovements(ctx, sqlcgen.ReportMovementsParams{
		DateFrom: from, DateTo: to, ChannelID: dimArg(d.ChannelID), SubChannelID: dimArg(d.SubChannelID),
		DivisionID: dimArg(d.DivisionID), CampaignID: dimArg(d.CampaignID),
	})
	if err != nil {
		return nil, fmt.Errorf("report movements: %w", err)
	}
	out := make([]MovementRow, len(rows))
	for i, r := range rows {
		out[i] = MovementRow{AccountID: r.AccountID, Before: money.FromDecimal(r.BeforeNet), During: money.FromDecimal(r.DuringNet)}
	}
	return out, nil
}

// DimensionMovementRow adalah mutasi akun laba rugi per nilai dimensi (uuid.Nil = tanpa dimensi).
type DimensionMovementRow struct {
	AccountID   uuid.UUID
	DimensionID uuid.UUID
	Net         money.Amount
}

// ReportDimensionMovements membaca mutasi akun laba rugi per nilai dimensi.
func (s *Store) ReportDimensionMovements(ctx context.Context, kind domain.DimensionKind, from, to clock.BusinessDate, channelID *uuid.UUID) ([]DimensionMovementRow, error) {
	rows, err := s.q(ctx).ReportDimensionMovements(ctx, sqlcgen.ReportDimensionMovementsParams{
		Dimension: string(kind), DateFrom: from, DateTo: to, ChannelID: dimArg(channelID),
	})
	if err != nil {
		return nil, fmt.Errorf("report dimension movements: %w", err)
	}
	out := make([]DimensionMovementRow, len(rows))
	for i, r := range rows {
		out[i] = DimensionMovementRow{AccountID: r.AccountID, DimensionID: r.DimensionID, Net: money.FromDecimal(r.Net)}
	}
	return out, nil
}

// ReportMonthlyAccounts membaca mutasi bulanan akun tertentu.
func (s *Store) ReportMonthlyAccounts(ctx context.Context, codes []string, from, to clock.BusinessDate) ([]domain.MonthlyAccount, error) {
	rows, err := s.q(ctx).ReportMonthlyAccounts(ctx, sqlcgen.ReportMonthlyAccountsParams{Codes: codes, DateFrom: from, DateTo: to})
	if err != nil {
		return nil, fmt.Errorf("report monthly accounts: %w", err)
	}
	out := make([]domain.MonthlyAccount, len(rows))
	for i, r := range rows {
		out[i] = domain.MonthlyAccount{Month: r.Month, Code: r.Code, Debit: money.FromDecimal(r.Debit), Credit: money.FromDecimal(r.Credit)}
	}
	return out, nil
}

// MonthlyGroupRow adalah mutasi debit − kredit bulanan satu kelompok akun laba rugi.
type MonthlyGroupRow struct {
	Month clock.BusinessDate
	Group domain.Group
	Net   money.Amount
}

// ReportMonthlyGroups membaca mutasi bulanan per kelompok laba rugi.
func (s *Store) ReportMonthlyGroups(ctx context.Context, from, to clock.BusinessDate) ([]MonthlyGroupRow, error) {
	rows, err := s.q(ctx).ReportMonthlyGroups(ctx, sqlcgen.ReportMonthlyGroupsParams{DateFrom: from, DateTo: to})
	if err != nil {
		return nil, fmt.Errorf("report monthly groups: %w", err)
	}
	out := make([]MonthlyGroupRow, len(rows))
	for i, r := range rows {
		out[i] = MonthlyGroupRow{Month: r.Month, Group: domain.Group(r.AccountGroup), Net: money.FromDecimal(r.Net)}
	}
	return out, nil
}

// TaxLine adalah satu baris jurnal akun pajak.
type TaxLine struct {
	EntryID      uuid.UUID
	BusinessDate clock.BusinessDate
	Number       string
	SourceType   string
	SourceLabel  string
	Description  string
	AccountCode  string
	AccountName  string
	Debit        money.Amount
	Credit       money.Amount
}

// ReportTaxLines membaca rincian jurnal akun pajak (maks. limit baris).
func (s *Store) ReportTaxLines(ctx context.Context, codes []string, from, to clock.BusinessDate, limit int32) ([]TaxLine, error) {
	rows, err := s.q(ctx).ReportTaxLines(ctx, sqlcgen.ReportTaxLinesParams{Codes: codes, DateFrom: from, DateTo: to, RowLimit: limit})
	if err != nil {
		return nil, fmt.Errorf("report tax lines: %w", err)
	}
	out := make([]TaxLine, len(rows))
	for i, r := range rows {
		out[i] = TaxLine{
			EntryID: r.EntryID, BusinessDate: r.BusinessDate, Number: r.Number, SourceType: r.SourceType,
			SourceLabel: r.SourceLabel, Description: r.Description, AccountCode: r.AccountCode, AccountName: r.AccountName,
			Debit: money.FromDecimal(r.DebitAmount), Credit: money.FromDecimal(r.CreditAmount),
		}
	}
	return out, nil
}

// TryReadModelLock mengambil advisory lock transaksi refresh read model; false = refresh lain sedang berjalan.
func (s *Store) TryReadModelLock(ctx context.Context) (bool, error) {
	ok, err := s.q(ctx).TryReadModelLock(ctx)
	if err != nil {
		return false, fmt.Errorf("read model lock: %w", err)
	}
	return ok, nil
}

// RefreshGLReadModel me-refresh rm_gl_daily tanpa mengunci pembaca.
func (s *Store) RefreshGLReadModel(ctx context.Context) error {
	if _, err := db.Conn(ctx, s.pool).Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY rm_gl_daily"); err != nil {
		return fmt.Errorf("refresh rm_gl_daily: %w", err)
	}
	return nil
}

// ReadModelRefresh adalah waktu refresh terakhir satu read model.
type ReadModelRefresh struct {
	Name        string
	RefreshedAt time.Time
	DurationMs  int
}

// SaveReadModelRefresh mencatat waktu refresh.
func (s *Store) SaveReadModelRefresh(ctx context.Context, r ReadModelRefresh) error {
	if err := s.q(ctx).UpsertReadModelRefresh(ctx, sqlcgen.UpsertReadModelRefreshParams{
		Name: r.Name, RefreshedAt: r.RefreshedAt, DurationMs: int32(min(r.DurationMs, 1<<30)), //nolint:gosec // dibatasi
	}); err != nil {
		return fmt.Errorf("save read model refresh: %w", err)
	}
	return nil
}

// ListReadModelRefreshes membaca waktu refresh semua read model.
func (s *Store) ListReadModelRefreshes(ctx context.Context) ([]ReadModelRefresh, error) {
	rows, err := s.q(ctx).ListReadModelRefreshes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list read model refreshes: %w", err)
	}
	out := make([]ReadModelRefresh, len(rows))
	for i, r := range rows {
		out[i] = ReadModelRefresh{Name: r.Name, RefreshedAt: r.RefreshedAt, DurationMs: int(r.DurationMs)}
	}
	return out, nil
}
