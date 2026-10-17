package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// GetOpeningSet membaca set saldo awal; ok false bila belum dibuat.
func (s *Store) GetOpeningSet(ctx context.Context) (domain.OpeningSet, bool, error) {
	r, err := s.q(ctx).GetOpeningSet(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OpeningSet{}, false, nil
	}
	if err != nil {
		return domain.OpeningSet{}, false, fmt.Errorf("get opening set: %w", err)
	}
	o := r.OpeningBalanceSet
	return domain.OpeningSet{
		ID: o.ID, CutoverDate: o.CutoverDate, Status: domain.OpeningStatus(o.Status), JournalEntryID: uuidPtr(o.JournalEntryID),
		JournalNumber: r.JournalNumber, PostedAt: o.PostedAt, PostedBy: uuidPtr(o.PostedBy), UpdatedAt: o.UpdatedAt,
		Version: int(o.Version),
	}, true, nil
}

// LockOpeningSet mengunci set saldo awal dan mengembalikan versinya.
func (s *Store) LockOpeningSet(ctx context.Context, id uuid.UUID) (int, error) {
	v, err := s.q(ctx).LockOpeningSet(ctx, id)
	if err != nil {
		return 0, notFound(err, "lock opening set")
	}
	return int(v), nil
}

// InsertOpeningSet membuat set saldo awal (hanya satu per sistem).
func (s *Store) InsertOpeningSet(ctx context.Context, cutover clock.BusinessDate, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertOpeningSet(ctx, sqlcgen.InsertOpeningSetParams{CutoverDate: cutover, ActorID: actorID})
	if pe := pgError(err); pe != nil && pe.Code == "23505" {
		return uuid.Nil, apperr.WithMessage(apperr.CodeVersionConflict, "Saldo awal sudah dibuat pengguna lain. Muat ulang halaman.")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert opening set: %w", err)
	}
	return id, nil
}

// SaveOpeningSet menyimpan tanggal cut-over dan status posting.
func (s *Store) SaveOpeningSet(ctx context.Context, o domain.OpeningSet, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateOpeningSet(ctx, sqlcgen.UpdateOpeningSetParams{
		ID: o.ID, Version: int32(version), CutoverDate: o.CutoverDate, Status: string(o.Status), //nolint:gosec // versi kecil
		JournalEntryID: nullUUID(o.JournalEntryID), PostedAt: o.PostedAt, PostedBy: nullUUID(o.PostedBy), ActorID: actorID,
	})
	if err != nil {
		return fmt.Errorf("update opening set: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

func toOpeningLine(r sqlcgen.OpeningBalanceLine, code, name string) domain.OpeningLine {
	return domain.OpeningLine{
		ID: r.ID, SetID: r.OpeningBalanceSetID, AccountID: r.AccountID, AccountCode: code, AccountName: name,
		Debit: money.FromDecimal(r.DebitAmount), Credit: money.FromDecimal(r.CreditAmount), Note: r.Note,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListOpeningLines mengembalikan Neraca Saldo Awal urut kode akun.
func (s *Store) ListOpeningLines(ctx context.Context, setID uuid.UUID) ([]domain.OpeningLine, error) {
	rows, err := s.q(ctx).ListOpeningLines(ctx, setID)
	if err != nil {
		return nil, fmt.Errorf("list opening lines: %w", err)
	}
	out := make([]domain.OpeningLine, len(rows))
	for i, r := range rows {
		out[i] = toOpeningLine(r.OpeningBalanceLine, r.AccountCode, r.AccountName)
	}
	return out, nil
}

// GetOpeningLine membaca satu baris saldo awal.
func (s *Store) GetOpeningLine(ctx context.Context, id uuid.UUID) (domain.OpeningLine, error) {
	r, err := s.q(ctx).GetOpeningLine(ctx, id)
	if err != nil {
		return domain.OpeningLine{}, notFound(err, "get opening line")
	}
	return toOpeningLine(r.OpeningBalanceLine, r.AccountCode, r.AccountName), nil
}

// UpsertOpeningLine membuat atau mengganti saldo awal satu akun.
func (s *Store) UpsertOpeningLine(ctx context.Context, setID uuid.UUID, l domain.OpeningLine, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).UpsertOpeningLine(ctx, sqlcgen.UpsertOpeningLineParams{
		OpeningBalanceSetID: setID, AccountID: l.AccountID, DebitAmount: l.Debit.Decimal(), CreditAmount: l.Credit.Decimal(),
		Note: l.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert opening line: %w", err)
	}
	return id, nil
}

// DeleteOpeningLine menghapus satu baris saldo awal.
func (s *Store) DeleteOpeningLine(ctx context.Context, setID, id uuid.UUID) error {
	n, err := s.q(ctx).DeleteOpeningLine(ctx, sqlcgen.DeleteOpeningLineParams{ID: id, OpeningBalanceSetID: setID})
	if err != nil {
		return fmt.Errorf("delete opening line: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeNotFound)
	}
	return nil
}

// OpeningLineNets mengembalikan saldo Neraca Saldo Awal per kode akun (debit − kredit).
func (s *Store) OpeningLineNets(ctx context.Context, setID uuid.UUID) (map[string]money.Amount, error) {
	rows, err := s.q(ctx).OpeningLineTotalsByAccount(ctx, setID)
	if err != nil {
		return nil, fmt.Errorf("opening line nets: %w", err)
	}
	out := make(map[string]money.Amount, len(rows))
	for _, r := range rows {
		out[r.Code] = money.FromDecimal(r.NetAmount)
	}
	return out, nil
}

func toOpeningItem(r sqlcgen.OpeningItem, accountCode string) domain.OpeningItem {
	return domain.OpeningItem{
		ID: r.ID, SetID: r.OpeningBalanceSetID, Type: domain.OpeningItemType(r.ItemType), PartyType: r.PartyType, PartyID: r.PartyID,
		PartyCode: r.PartyCode, DocumentNumber: r.DocumentNumber, DocumentDate: r.DocumentDate, DueDate: r.DueDate,
		Amount: money.FromDecimal(r.Amount), OpenAmount: money.FromDecimal(r.OpenAmount), Settled: money.FromDecimal(r.SettledAmount),
		AccountID: r.AccountID, AccountCode: accountCode, ChannelID: uuidPtr(r.ChannelID), Note: r.Note, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

// UpsertOpeningItem menyimpan piutang/utang terbuka; dokumen yang sama (jenis, pihak, nomor) diganti selama
// belum ada pelunasan.
func (s *Store) UpsertOpeningItem(ctx context.Context, setID uuid.UUID, it domain.OpeningItem, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertOpeningItem(ctx, sqlcgen.InsertOpeningItemParams{
		OpeningBalanceSetID: setID, ItemType: string(it.Type), PartyType: it.PartyType, PartyID: it.PartyID, PartyCode: it.PartyCode,
		DocumentNumber: it.DocumentNumber, DocumentDate: it.DocumentDate, DueDate: it.DueDate, Amount: it.Amount.Decimal(),
		OpenAmount: it.OpenAmount.Decimal(), AccountID: it.AccountID, ChannelID: nullUUID(it.ChannelID), Note: it.Note, ActorID: actorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, apperr.Validation(map[string]string{"documentNumber": "Dokumen sudah punya pelunasan dan tidak bisa diganti"})
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert opening item: %w", err)
	}
	return id, nil
}

// OpeningItemFilter adalah filter daftar saldo terbuka.
type OpeningItemFilter struct {
	Type            domain.OpeningItemType
	PartyType       string
	PartyID         *uuid.UUID
	OutstandingOnly bool
	Query           string
}

// OpeningItemPage adalah satu halaman saldo terbuka beserta total sisa seluruh hasil filter.
type OpeningItemPage struct {
	paging.Result[domain.OpeningItem]
	Outstanding money.Amount
}

// ListOpeningItems mengembalikan saldo terbuka urut jatuh tempo.
func (s *Store) ListOpeningItems(ctx context.Context, f OpeningItemFilter, page paging.Page) (OpeningItemPage, error) {
	q := s.q(ctx)
	rows, err := q.ListOpeningItems(ctx, sqlcgen.ListOpeningItemsParams{
		ItemType: text(string(f.Type)), PartyType: text(f.PartyType), PartyID: nullUUID(f.PartyID), OutstandingOnly: f.OutstandingOnly,
		Q: text(f.Query), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return OpeningItemPage{}, fmt.Errorf("list opening items: %w", err)
	}
	total, err := q.CountOpeningItems(ctx, sqlcgen.CountOpeningItemsParams{
		ItemType: text(string(f.Type)), PartyType: text(f.PartyType), PartyID: nullUUID(f.PartyID), OutstandingOnly: f.OutstandingOnly,
		Q: text(f.Query),
	})
	if err != nil {
		return OpeningItemPage{}, fmt.Errorf("count opening items: %w", err)
	}
	items := make([]domain.OpeningItem, len(rows))
	for i, r := range rows {
		items[i] = toOpeningItem(r.OpeningItem, r.AccountCode)
	}
	return OpeningItemPage{
		Result:      paging.Result[domain.OpeningItem]{Items: items, Page: page, Total: total.Count},
		Outstanding: money.FromDecimal(total.OutstandingAmount),
	}, nil
}

// GetOpeningItem mengunci satu saldo terbuka.
func (s *Store) GetOpeningItem(ctx context.Context, id uuid.UUID) (domain.OpeningItem, error) {
	r, err := s.q(ctx).GetOpeningItemForUpdate(ctx, id)
	if err != nil {
		return domain.OpeningItem{}, notFound(err, "get opening item")
	}
	return toOpeningItem(r.OpeningItem, r.AccountCode), nil
}

// SettlementExists memeriksa pelunasan dari dokumen sumber yang sama (idempotensi).
func (s *Store) SettlementExists(ctx context.Context, itemID uuid.UUID, sourceType string, sourceID uuid.UUID) (bool, error) {
	ok, err := s.q(ctx).OpeningItemSettlementExists(ctx, sqlcgen.OpeningItemSettlementExistsParams{
		OpeningItemID: itemID, SourceType: sourceType, SourceID: sourceID,
	})
	if err != nil {
		return false, fmt.Errorf("settlement exists: %w", err)
	}
	return ok, nil
}

// RecordSettlement mencatat pelunasan dan total terlunasi saldo terbuka.
func (s *Store) RecordSettlement(ctx context.Context, it domain.OpeningItem, sourceType string, sourceID uuid.UUID,
	date clock.BusinessDate, amount money.Amount, actorID uuid.UUID,
) error {
	q := s.q(ctx)
	if err := q.InsertOpeningItemSettlement(ctx, sqlcgen.InsertOpeningItemSettlementParams{
		OpeningItemID: it.ID, SourceType: sourceType, SourceID: sourceID, BusinessDate: date, Amount: amount.Decimal(), ActorID: actorID,
	}); err != nil {
		return fmt.Errorf("insert settlement: %w", err)
	}
	if err := q.SetOpeningItemSettled(ctx, sqlcgen.SetOpeningItemSettledParams{ID: it.ID, SettledAmount: it.Settled.Decimal(), ActorID: actorID}); err != nil {
		return fmt.Errorf("set settled: %w", err)
	}
	return nil
}

// OpeningItemTotal adalah total sisa saldo terbuka per cut-over per jenis & akun.
type OpeningItemTotal struct {
	Type        domain.OpeningItemType
	AccountCode string
	Open        money.Amount
}

// OpeningItemTotals menjumlahkan saldo terbuka per jenis & akun (rekonsiliasi).
func (s *Store) OpeningItemTotals(ctx context.Context) ([]OpeningItemTotal, error) {
	rows, err := s.q(ctx).OpeningItemTotals(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening item totals: %w", err)
	}
	out := make([]OpeningItemTotal, len(rows))
	for i, r := range rows {
		out[i] = OpeningItemTotal{Type: domain.OpeningItemType(r.ItemType), AccountCode: r.AccountCode, Open: money.FromDecimal(r.OpenAmount)}
	}
	return out, nil
}

// JournalImportRef mencari draft jurnal hasil impor untuk Nomor Referensi di satu file.
func (s *Store) JournalImportRef(ctx context.Context, importKey, reference string) (uuid.UUID, bool, error) {
	id, err := s.q(ctx).GetJournalImportRef(ctx, sqlcgen.GetJournalImportRefParams{ImportKey: importKey, Reference: reference})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get journal import ref: %w", err)
	}
	return id, true, nil
}

// InsertJournalImportRef menautkan Nomor Referensi impor ke draft jurnal.
func (s *Store) InsertJournalImportRef(ctx context.Context, importKey, reference string, entryID uuid.UUID) error {
	if err := s.q(ctx).InsertJournalImportRef(ctx, sqlcgen.InsertJournalImportRefParams{
		ImportKey: importKey, Reference: reference, JournalEntryID: entryID,
	}); err != nil {
		return fmt.Errorf("insert journal import ref: %w", err)
	}
	return nil
}

// --- Laporan ---------------------------------------------------------------------------------------

// TrialBalanceFilter adalah rentang dan dimensi Neraca Saldo.
type TrialBalanceFilter struct {
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	Dimensions domain.Dimensions
}

// TrialBalance menghitung Neraca Saldo per akun.
func (s *Store) TrialBalance(ctx context.Context, f TrialBalanceFilter) ([]domain.TrialBalanceRow, error) {
	rows, err := s.q(ctx).TrialBalance(ctx, sqlcgen.TrialBalanceParams{
		DateFrom: f.DateFrom, DateTo: f.DateTo, ChannelID: nullUUID(f.Dimensions.ChannelID),
		SubChannelID: nullUUID(f.Dimensions.SubChannelID), DivisionID: nullUUID(f.Dimensions.DivisionID),
		CampaignID: nullUUID(f.Dimensions.CampaignID),
	})
	if err != nil {
		return nil, fmt.Errorf("trial balance: %w", err)
	}
	out := make([]domain.TrialBalanceRow, len(rows))
	for i, r := range rows {
		out[i] = domain.TrialBalanceRow{
			AccountID: r.AccountID, AccountCode: r.AccountCode, AccountName: r.AccountName, Group: domain.Group(r.AccountGroup),
			NormalBalance: domain.NormalBalance(r.NormalBalance), OpeningNet: money.FromDecimal(r.OpeningNet),
			PeriodDebit: money.FromDecimal(r.PeriodDebit), PeriodCredit: money.FromDecimal(r.PeriodCredit),
		}
	}
	return out, nil
}

// AccountNets mengembalikan saldo debit − kredit per kode akun s.d. tanggal dari jurnal terposting.
func (s *Store) AccountNets(ctx context.Context, asOf clock.BusinessDate) (map[string]money.Amount, error) {
	rows, err := s.q(ctx).AccountNetBalances(ctx, asOf)
	if err != nil {
		return nil, fmt.Errorf("account nets: %w", err)
	}
	out := make(map[string]money.Amount, len(rows))
	for _, r := range rows {
		out[r.Code] = money.FromDecimal(r.NetAmount)
	}
	return out, nil
}

// LedgerExportFilter adalah filter ekspor Buku Besar detail.
type LedgerExportFilter struct {
	AccountID  *uuid.UUID
	DateFrom   clock.BusinessDate
	DateTo     clock.BusinessDate
	Dimensions domain.Dimensions
	Limit      int32
}

// LedgerExportLine adalah satu baris ekspor Buku Besar.
type LedgerExportLine struct {
	AccountCode  string
	AccountName  string
	Number       string
	BusinessDate clock.BusinessDate
	Description  string
	SourceType   string
	SourceLabel  string
	Debit        money.Amount
	Credit       money.Amount
	Memo         string
	Dimensions   domain.Dimensions
}

// LedgerExportLines membaca baris Buku Besar untuk ekspor.
func (s *Store) LedgerExportLines(ctx context.Context, f LedgerExportFilter) ([]LedgerExportLine, error) {
	rows, err := s.q(ctx).LedgerExportLines(ctx, sqlcgen.LedgerExportLinesParams{
		DateFrom: f.DateFrom, DateTo: f.DateTo, AccountID: nullUUID(f.AccountID), ChannelID: nullUUID(f.Dimensions.ChannelID),
		SubChannelID: nullUUID(f.Dimensions.SubChannelID), DivisionID: nullUUID(f.Dimensions.DivisionID),
		CampaignID: nullUUID(f.Dimensions.CampaignID), RowLimit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("ledger export lines: %w", err)
	}
	out := make([]LedgerExportLine, len(rows))
	for i, r := range rows {
		out[i] = LedgerExportLine{
			AccountCode: r.AccountCode, AccountName: r.AccountName, Number: textValue(r.Number), BusinessDate: r.BusinessDate,
			Description: r.Description, SourceType: r.SourceType, SourceLabel: r.SourceLabel, Debit: money.FromDecimal(r.DebitAmount),
			Credit: money.FromDecimal(r.CreditAmount), Memo: r.Memo,
			Dimensions: domain.Dimensions{
				ChannelID: uuidPtr(r.ChannelID), SubChannelID: uuidPtr(r.SubChannelID), DivisionID: uuidPtr(r.DivisionID), CampaignID: uuidPtr(r.CampaignID),
			},
		}
	}
	return out, nil
}
