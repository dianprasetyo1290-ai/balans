package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

func toEntry(r sqlcgen.JournalEntry, reversalOfNumber string, reversedBy uuid.NullUUID, reversedByNumber string) domain.Entry {
	return domain.Entry{
		ID: r.ID, Number: r.Number.String, BusinessDate: r.BusinessDate, FiscalPeriodID: uuidPtr(r.FiscalPeriodID),
		EntryType: domain.EntryType(r.EntryType), SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel,
		PostingRuleCode: r.PostingRuleCode.String, Description: r.Description, ReversalOfID: uuidPtr(r.ReversalOfID),
		ReversalOfNumber: reversalOfNumber, ReversedByID: uuidPtr(reversedBy), ReversedByNumber: reversedByNumber,
		Status: domain.EntryStatus(r.Status), TotalAmount: money.FromDecimal(r.TotalAmount), PostedAt: r.PostedAt,
		PostedBy: uuidPtr(r.PostedBy), CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func (s *Store) insertLines(ctx context.Context, entryID uuid.UUID, lines []domain.Line, actorID uuid.UUID) error {
	q := s.q(ctx)
	for _, l := range lines {
		p := sqlcgen.InsertLineParams{
			JournalEntryID: entryID, LineNo: int32(l.LineNo), AccountID: l.AccountID, //nolint:gosec // baris ≤ MaxLines
			DebitAmount: l.Debit.Decimal(), CreditAmount: l.Credit.Decimal(),
			ChannelID: nullUUID(l.Dimensions.ChannelID), SubChannelID: nullUUID(l.Dimensions.SubChannelID),
			DivisionID: nullUUID(l.Dimensions.DivisionID), CampaignID: nullUUID(l.Dimensions.CampaignID),
			AmountKey: l.AmountKey, Memo: l.Memo, ActorID: actorID,
		}
		if l.Party != nil {
			p.PartyType, p.PartyID = l.Party.Type, uuid.NullUUID{UUID: l.Party.ID, Valid: true}
		}
		if err := q.InsertLine(ctx, p); err != nil {
			if pe := pgError(err); pe != nil && pe.Code == "23503" {
				return apperr.Validation(map[string]string{fmt.Sprintf("lines.%d", l.LineNo-1): "Akun atau dimensi tidak ditemukan"})
			}
			return mapWriteError(err, "insert journal line")
		}
	}
	return nil
}

// InsertEntry menyimpan jurnal draft beserta barisnya. ID harus sudah diisi.
func (s *Store) InsertEntry(ctx context.Context, e domain.Entry, actorID uuid.UUID) error {
	err := s.q(ctx).InsertEntry(ctx, sqlcgen.InsertEntryParams{
		ID: e.ID, BusinessDate: e.BusinessDate, EntryType: string(e.EntryType), SourceType: e.SourceType, SourceID: e.SourceID,
		SourceLabel: e.SourceLabel, PostingRuleCode: text(e.PostingRuleCode), Description: e.Description,
		ReversalOfID: nullUUID(e.ReversalOfID), TotalAmount: e.TotalAmount.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return mapWriteError(err, "insert journal entry")
	}
	return s.insertLines(ctx, e.ID, e.Lines, actorID)
}

// ReplaceDraft mengganti isi draft: header diperbarui (optimistic locking), baris dihapus lalu ditulis ulang.
func (s *Store) ReplaceDraft(ctx context.Context, e domain.Entry, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateDraftEntry(ctx, sqlcgen.UpdateDraftEntryParams{
		BusinessDate: e.BusinessDate, Description: e.Description, TotalAmount: e.TotalAmount.Decimal(), ActorID: actorID,
		ID: e.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return mapWriteError(err, "update journal entry")
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	if err := s.q(ctx).DeleteDraftLines(ctx, e.ID); err != nil {
		return mapWriteError(err, "delete journal lines")
	}
	return s.insertLines(ctx, e.ID, e.Lines, actorID)
}

// PostEntry mengubah draft menjadi terposting. Trigger DB memeriksa ulang balance.
func (s *Store) PostEntry(ctx context.Context, e domain.Entry, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).PostEntry(ctx, sqlcgen.PostEntryParams{
		Number: e.Number, FiscalPeriodID: *e.FiscalPeriodID, TotalAmount: e.TotalAmount.Decimal(), PostedAt: *e.PostedAt,
		ActorID: actorID, ID: e.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return mapWriteError(err, "post journal entry")
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// DeleteDraft melakukan soft delete draft Jurnal Umum.
func (s *Store) DeleteDraft(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteDraftEntry(ctx, sqlcgen.SoftDeleteDraftEntryParams{ActorID: actorID, ID: id, Version: int32(version)}) //nolint:gosec // versi kecil
	if err != nil {
		return mapWriteError(err, "delete journal entry")
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// LockEntry mengunci jurnal untuk posting/pembalikan dan mengembalikan versinya.
func (s *Store) LockEntry(ctx context.Context, id uuid.UUID) (int, error) {
	v, err := s.q(ctx).LockEntry(ctx, id)
	if err != nil {
		return 0, notFound(err, "lock journal entry")
	}
	return int(v), nil
}

// EntryIDBySource mencari jurnal dokumen sumber per rule.
func (s *Store) EntryIDBySource(ctx context.Context, sourceType string, sourceID uuid.UUID, ruleCode string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetEntryIDBySource(ctx, sqlcgen.GetEntryIDBySourceParams{SourceType: sourceType, SourceID: sourceID, PostingRuleCode: text(ruleCode)})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get journal entry by source: %w", err)
	}
	return id, true, nil
}

func (s *Store) withLines(ctx context.Context, entries []domain.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(entries))
	idx := make(map[uuid.UUID]int, len(entries))
	for i, e := range entries {
		ids[i], idx[e.ID] = e.ID, i
	}
	rows, err := s.q(ctx).ListLinesByEntries(ctx, ids)
	if err != nil {
		return fmt.Errorf("list journal lines: %w", err)
	}
	for _, r := range rows {
		l := domain.Line{
			ID: r.ID, LineNo: int(r.LineNo), AccountID: r.AccountID, AccountCode: r.AccountCode, AccountName: r.AccountName,
			Debit: money.FromDecimal(r.DebitAmount), Credit: money.FromDecimal(r.CreditAmount),
			Dimensions: domain.Dimensions{
				ChannelID: uuidPtr(r.ChannelID), SubChannelID: uuidPtr(r.SubChannelID), DivisionID: uuidPtr(r.DivisionID), CampaignID: uuidPtr(r.CampaignID),
			},
			AmountKey: r.AmountKey, Memo: r.Memo,
		}
		if r.PartyID.Valid {
			l.Party = &domain.Party{Type: r.PartyType, ID: r.PartyID.UUID}
		}
		i := idx[r.JournalEntryID]
		entries[i].Lines = append(entries[i].Lines, l)
	}
	return nil
}

// GetEntry membaca jurnal beserta barisnya.
func (s *Store) GetEntry(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	r, err := s.q(ctx).GetEntry(ctx, id)
	if err != nil {
		return domain.Entry{}, notFound(err, "get journal entry")
	}
	list := []domain.Entry{toEntry(r.JournalEntry, r.ReversalOfNumber, r.ReversedByID, r.ReversedByNumber)}
	if err := s.withLines(ctx, list); err != nil {
		return domain.Entry{}, err
	}
	return list[0], nil
}

// EntryFilter adalah filter daftar jurnal.
type EntryFilter struct {
	Status    domain.EntryStatus
	EntryType domain.EntryType
	DateFrom  *clock.BusinessDate
	DateTo    *clock.BusinessDate
	Query     string
}

// ListEntries mengembalikan daftar jurnal terbaru lebih dulu (tanpa baris).
func (s *Store) ListEntries(ctx context.Context, f EntryFilter, page paging.Page) (paging.Result[domain.Entry], error) {
	q := s.q(ctx)
	status, typ := text(string(f.Status)), text(string(f.EntryType))
	from, to, search := date(f.DateFrom), date(f.DateTo), text(f.Query)
	rows, err := q.ListEntries(ctx, sqlcgen.ListEntriesParams{
		Status: status, EntryType: typ, DateFrom: from, DateTo: to, Q: search, RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Entry]{}, fmt.Errorf("list journal entries: %w", err)
	}
	total, err := q.CountEntries(ctx, sqlcgen.CountEntriesParams{Status: status, EntryType: typ, DateFrom: from, DateTo: to, Q: search})
	if err != nil {
		return paging.Result[domain.Entry]{}, fmt.Errorf("count journal entries: %w", err)
	}
	items := make([]domain.Entry, len(rows))
	for i, r := range rows {
		items[i] = toEntry(r.JournalEntry, r.ReversalOfNumber, r.ReversedByID, r.ReversedByNumber)
	}
	return paging.Result[domain.Entry]{Items: items, Page: page, Total: total}, nil
}

// LedgerFilter adalah filter Buku Besar satu akun.
type LedgerFilter struct {
	AccountID  uuid.UUID
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	Dimensions domain.Dimensions
}

// LedgerLine adalah satu mutasi Buku Besar.
type LedgerLine struct {
	EntryID      uuid.UUID
	Number       string
	BusinessDate clock.BusinessDate
	Description  string
	EntryType    domain.EntryType
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	LineNo       int
	Debit        money.Amount
	Credit       money.Amount
	Memo         string
	Dimensions   domain.Dimensions
	// CumulativeNet adalah kumulatif debit − kredit sejak DateFrom sampai baris ini.
	CumulativeNet money.Amount
}

// LedgerSummary adalah saldo awal dan mutasi periode.
type LedgerSummary struct {
	OpeningDebit  money.Amount
	OpeningCredit money.Amount
	PeriodDebit   money.Amount
	PeriodCredit  money.Amount
	LineCount     int64
}

// LedgerSummary menghitung saldo awal dan total mutasi.
func (s *Store) LedgerSummary(ctx context.Context, f LedgerFilter) (LedgerSummary, error) {
	r, err := s.q(ctx).LedgerSummary(ctx, sqlcgen.LedgerSummaryParams{
		DateFrom: f.DateFrom, AccountID: f.AccountID, DateTo: f.DateTo,
		ChannelID: nullUUID(f.Dimensions.ChannelID), SubChannelID: nullUUID(f.Dimensions.SubChannelID),
		DivisionID: nullUUID(f.Dimensions.DivisionID), CampaignID: nullUUID(f.Dimensions.CampaignID),
	})
	if err != nil {
		return LedgerSummary{}, fmt.Errorf("ledger summary: %w", err)
	}
	return LedgerSummary{
		OpeningDebit: money.FromDecimal(r.OpeningDebit), OpeningCredit: money.FromDecimal(r.OpeningCredit),
		PeriodDebit: money.FromDecimal(r.PeriodDebit), PeriodCredit: money.FromDecimal(r.PeriodCredit), LineCount: r.LineCount,
	}, nil
}

// LedgerLines mengembalikan satu halaman mutasi urut tanggal & nomor.
func (s *Store) LedgerLines(ctx context.Context, f LedgerFilter, page paging.Page) ([]LedgerLine, error) {
	rows, err := s.q(ctx).LedgerLines(ctx, sqlcgen.LedgerLinesParams{
		AccountID: f.AccountID, DateFrom: f.DateFrom, DateTo: f.DateTo,
		ChannelID: nullUUID(f.Dimensions.ChannelID), SubChannelID: nullUUID(f.Dimensions.SubChannelID),
		DivisionID: nullUUID(f.Dimensions.DivisionID), CampaignID: nullUUID(f.Dimensions.CampaignID),
		RowOffset: page.Offset(), RowLimit: page.Limit(),
	})
	if err != nil {
		return nil, fmt.Errorf("ledger lines: %w", err)
	}
	out := make([]LedgerLine, len(rows))
	for i, r := range rows {
		out[i] = LedgerLine{
			EntryID: r.EntryID, Number: textValue(r.Number), BusinessDate: r.BusinessDate, Description: r.Description,
			EntryType: domain.EntryType(r.EntryType), SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel, LineNo: int(r.LineNo),
			Debit: money.FromDecimal(r.DebitAmount), Credit: money.FromDecimal(r.CreditAmount), Memo: r.Memo,
			Dimensions: domain.Dimensions{
				ChannelID: uuidPtr(r.ChannelID), SubChannelID: uuidPtr(r.SubChannelID), DivisionID: uuidPtr(r.DivisionID), CampaignID: uuidPtr(r.CampaignID),
			},
			CumulativeNet: money.FromDecimal(r.CumulativeNet),
		}
	}
	return out, nil
}

func textValue(t pgtype.Text) string { return t.String }
