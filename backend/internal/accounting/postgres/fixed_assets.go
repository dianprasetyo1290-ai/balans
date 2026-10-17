package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func toAssetClass(r sqlcgen.FixedAssetClass, assetCode, accumulatedCode string) domain.AssetClass {
	return domain.AssetClass{
		ID: r.ID, Code: r.Code, Name: r.Name, UsefulLifeMonths: int(r.UsefulLifeMonths), AssetAccountID: r.AssetAccountID,
		AssetAccountCode: assetCode, AccumulatedAccountID: r.AccumulatedAccountID, AccumulatedAccountCode: accumulatedCode,
		IsActive: r.IsActive, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListAssetClasses mengembalikan kelas aset urut nama.
func (s *Store) ListAssetClasses(ctx context.Context, includeInactive bool) ([]domain.AssetClass, error) {
	rows, err := s.q(ctx).ListAssetClasses(ctx, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("list asset classes: %w", err)
	}
	out := make([]domain.AssetClass, len(rows))
	for i, r := range rows {
		out[i] = toAssetClass(r.FixedAssetClass, r.AssetAccountCode, r.AccumulatedAccountCode)
	}
	return out, nil
}

// GetAssetClass membaca kelas aset.
func (s *Store) GetAssetClass(ctx context.Context, id uuid.UUID) (domain.AssetClass, error) {
	r, err := s.q(ctx).GetAssetClass(ctx, id)
	if err != nil {
		return domain.AssetClass{}, notFound(err, "get asset class")
	}
	return toAssetClass(r.FixedAssetClass, r.AssetAccountCode, r.AccumulatedAccountCode), nil
}

// InsertAssetClass menyimpan kelas aset baru.
func (s *Store) InsertAssetClass(ctx context.Context, c domain.AssetClass, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertAssetClass(ctx, sqlcgen.InsertAssetClassParams{
		Code: c.Code, Name: c.Name, UsefulLifeMonths: int32(c.UsefulLifeMonths), AssetAccountID: c.AssetAccountID, //nolint:gosec // dibatasi domain
		AccumulatedAccountID: c.AccumulatedAccountID, IsActive: c.IsActive, ActorID: actorID,
	})
	if pe := pgError(err); pe != nil && pe.Code == "23505" {
		return uuid.Nil, &apperr.Error{Code: apperr.CodeDuplicateCode, Message: apperr.CodeDuplicateCode.DefaultMessage(),
			Fields: map[string]string{"code": "Kode kelas sudah dipakai"}, Err: err}
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert asset class: %w", err)
	}
	return id, nil
}

// UpdateAssetClass mengubah kelas aset dengan optimistic locking.
func (s *Store) UpdateAssetClass(ctx context.Context, c domain.AssetClass, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateAssetClass(ctx, sqlcgen.UpdateAssetClassParams{
		ID: c.ID, Version: int32(version), Name: c.Name, UsefulLifeMonths: int32(c.UsefulLifeMonths), //nolint:gosec // dibatasi domain
		AssetAccountID: c.AssetAccountID, AccumulatedAccountID: c.AccumulatedAccountID, IsActive: c.IsActive, ActorID: actorID,
	})
	if err != nil {
		return fmt.Errorf("update asset class: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

func pgDate(d *clock.BusinessDate) pgtype.Date { return date(d) }

func datePtr(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	v := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &v
}

func toFixedAsset(r sqlcgen.FixedAsset, className string) domain.FixedAsset {
	return domain.FixedAsset{
		ID: r.ID, Code: r.Code, Name: r.Name, ClassID: r.ClassID, ClassName: className, LocationID: uuidPtr(r.LocationID),
		LocationLabel: r.LocationLabel, DivisionID: r.DivisionID, Source: domain.AssetSource(r.AcquisitionSource),
		AcquisitionDate: r.AcquisitionDate, Cost: money.FromDecimal(r.AcquisitionCostAmount), Salvage: money.FromDecimal(r.SalvageAmount),
		UsefulLife: int(r.UsefulLifeMonths), DepreciationStart: r.DepreciationStartDate,
		OpeningAccumulated: money.FromDecimal(r.OpeningAccumulatedAmount), OpeningMonths: int(r.OpeningDepreciatedMonths),
		Accumulated: money.FromDecimal(r.AccumulatedAmount), PaymentAccountID: uuidPtr(r.PaymentAccountID),
		Status: domain.AssetStatus(r.Status), AcquisitionJournal: uuidPtr(r.AcquisitionJournalID), DisposalDate: datePtr(r.DisposalDate),
		DisposalProceeds: amountPtr(r.DisposalProceedsAmount), DisposalAccountID: uuidPtr(r.DisposalAccountID),
		DisposalReason: r.DisposalReason, DisposalJournal: uuidPtr(r.DisposalJournalID), Note: r.Note, DeletedAt: r.DeletedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// FixedAssetFilter adalah filter daftar Aset Tetap.
type FixedAssetFilter struct {
	Status  domain.AssetStatus
	ClassID *uuid.UUID
	Query   string
}

// ListFixedAssets mengembalikan satu halaman aset (belum dihapus) urut kode.
func (s *Store) ListFixedAssets(ctx context.Context, f FixedAssetFilter, page paging.Page) (paging.Result[domain.FixedAsset], error) {
	q := s.q(ctx)
	rows, err := q.ListFixedAssets(ctx, sqlcgen.ListFixedAssetsParams{
		Status: text(string(f.Status)), ClassID: nullUUID(f.ClassID), Q: text(f.Query), RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.FixedAsset]{}, fmt.Errorf("list fixed assets: %w", err)
	}
	total, err := q.CountFixedAssets(ctx, sqlcgen.CountFixedAssetsParams{Status: text(string(f.Status)), ClassID: nullUUID(f.ClassID), Q: text(f.Query)})
	if err != nil {
		return paging.Result[domain.FixedAsset]{}, fmt.Errorf("count fixed assets: %w", err)
	}
	items := make([]domain.FixedAsset, len(rows))
	for i, r := range rows {
		items[i] = toFixedAsset(r.FixedAsset, r.ClassName)
	}
	return paging.Result[domain.FixedAsset]{Items: items, Page: page, Total: total}, nil
}

// GetFixedAsset membaca aset (termasuk yang dihapus); forUpdate mengunci baris.
func (s *Store) GetFixedAsset(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.FixedAsset, error) {
	q := s.q(ctx)
	if forUpdate {
		if _, err := q.LockFixedAsset(ctx, id); err != nil {
			return domain.FixedAsset{}, notFound(err, "lock fixed asset")
		}
	}
	r, err := q.GetFixedAsset(ctx, id)
	if err != nil {
		return domain.FixedAsset{}, notFound(err, "get fixed asset")
	}
	return toFixedAsset(r.FixedAsset, r.ClassName), nil
}

// GetFixedAssetByCode membaca aset berdasarkan kode (impor).
func (s *Store) GetFixedAssetByCode(ctx context.Context, code string) (domain.FixedAsset, error) {
	r, err := s.q(ctx).GetFixedAssetByCode(ctx, code)
	if err != nil {
		return domain.FixedAsset{}, notFound(err, "get fixed asset by code")
	}
	return toFixedAsset(r.FixedAsset, r.ClassName), nil
}

// InsertFixedAsset menyimpan aset baru.
func (s *Store) InsertFixedAsset(ctx context.Context, a domain.FixedAsset, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertFixedAsset(ctx, sqlcgen.InsertFixedAssetParams{
		Code: a.Code, Name: a.Name, ClassID: a.ClassID, LocationID: nullUUID(a.LocationID), LocationLabel: a.LocationLabel,
		DivisionID: a.DivisionID, AcquisitionSource: string(a.Source), AcquisitionDate: a.AcquisitionDate,
		AcquisitionCostAmount: a.Cost.Decimal(), SalvageAmount: a.Salvage.Decimal(), UsefulLifeMonths: int32(a.UsefulLife), //nolint:gosec // dibatasi domain
		DepreciationStartDate: a.DepreciationStart, OpeningAccumulatedAmount: a.OpeningAccumulated.Decimal(),
		OpeningDepreciatedMonths: int32(a.OpeningMonths), AccumulatedAmount: a.Accumulated.Decimal(), //nolint:gosec // dibatasi domain
		PaymentAccountID: nullUUID(a.PaymentAccountID), Status: string(a.Status), Note: a.Note, ActorID: actorID,
	})
	if pe := pgError(err); pe != nil && pe.Code == "23505" && pe.ConstraintName == "uq_fixed_assets_code" {
		return uuid.Nil, &apperr.Error{Code: apperr.CodeDuplicateCode, Message: apperr.CodeDuplicateCode.DefaultMessage(),
			Fields: map[string]string{"code": "Kode aset sudah dipakai (termasuk aset yang sudah dihapus)"}, Err: err}
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert fixed asset: %w", err)
	}
	return id, nil
}

// UpdateFixedAsset menyimpan seluruh kolom aset dengan optimistic locking.
func (s *Store) UpdateFixedAsset(ctx context.Context, a domain.FixedAsset, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateFixedAsset(ctx, sqlcgen.UpdateFixedAssetParams{
		ID: a.ID, Version: int32(version), Name: a.Name, ClassID: a.ClassID, LocationID: nullUUID(a.LocationID), //nolint:gosec // versi kecil
		LocationLabel: a.LocationLabel, DivisionID: a.DivisionID, AcquisitionSource: string(a.Source),
		AcquisitionDate: a.AcquisitionDate, AcquisitionCostAmount: a.Cost.Decimal(), SalvageAmount: a.Salvage.Decimal(),
		UsefulLifeMonths: int32(a.UsefulLife), DepreciationStartDate: a.DepreciationStart, //nolint:gosec // dibatasi domain
		OpeningAccumulatedAmount: a.OpeningAccumulated.Decimal(), OpeningDepreciatedMonths: int32(a.OpeningMonths), //nolint:gosec // dibatasi domain
		AccumulatedAmount: a.Accumulated.Decimal(), PaymentAccountID: nullUUID(a.PaymentAccountID), Status: string(a.Status),
		AcquisitionJournalID: nullUUID(a.AcquisitionJournal), DisposalDate: pgDate(a.DisposalDate),
		DisposalProceedsAmount: nullAmount(a.DisposalProceeds), DisposalAccountID: nullUUID(a.DisposalAccountID),
		DisposalReason: a.DisposalReason, DisposalJournalID: nullUUID(a.DisposalJournal), Note: a.Note, ActorID: actorID,
	})
	if err != nil {
		return fmt.Errorf("update fixed asset: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// DeleteFixedAsset menghapus (soft delete) aset draft.
func (s *Store) DeleteFixedAsset(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteFixedAsset(ctx, sqlcgen.SoftDeleteFixedAssetParams{ID: id, Version: int32(version), ActorID: actorID}) //nolint:gosec // versi kecil
	if err != nil {
		return fmt.Errorf("delete fixed asset: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// DepreciableAssets mengunci aset aktif yang belum disusutkan pada periode (urut id).
func (s *Store) DepreciableAssets(ctx context.Context, periodID uuid.UUID, start clock.BusinessDate) ([]domain.FixedAsset, error) {
	rows, err := s.q(ctx).DepreciableAssets(ctx, sqlcgen.DepreciableAssetsParams{PeriodStart: start, FiscalPeriodID: periodID})
	if err != nil {
		return nil, fmt.Errorf("depreciable assets: %w", err)
	}
	out := make([]domain.FixedAsset, len(rows))
	for i, r := range rows {
		out[i] = toFixedAsset(r.FixedAsset, r.ClassName)
	}
	return out, nil
}

// PendingDepreciationCount menghitung aset yang seharusnya sudah disusutkan pada periode.
func (s *Store) PendingDepreciationCount(ctx context.Context, periodID uuid.UUID, start clock.BusinessDate) (int64, error) {
	n, err := s.q(ctx).ActiveAssetsWithoutDepreciation(ctx, sqlcgen.ActiveAssetsWithoutDepreciationParams{PeriodStart: start, FiscalPeriodID: periodID})
	if err != nil {
		return 0, fmt.Errorf("pending depreciation: %w", err)
	}
	return n, nil
}

// AssetClassTotal adalah total harga perolehan & akumulasi aset aktif per pasangan akun kelas.
type AssetClassTotal struct {
	AssetAccountCode       string
	AccumulatedAccountCode string
	Cost                   money.Amount
	Accumulated            money.Amount
}

// AssetClassTotals menjumlahkan aset aktif per akun kelas (rekonsiliasi cut-over).
func (s *Store) AssetClassTotals(ctx context.Context) ([]AssetClassTotal, error) {
	rows, err := s.q(ctx).AssetClassTotals(ctx)
	if err != nil {
		return nil, fmt.Errorf("asset class totals: %w", err)
	}
	out := make([]AssetClassTotal, len(rows))
	for i, r := range rows {
		out[i] = AssetClassTotal{
			AssetAccountCode: r.AssetAccountCode, AccumulatedAccountCode: r.AccumulatedAccountCode,
			Cost: money.FromDecimal(r.CostAmount), Accumulated: money.FromDecimal(r.AccumulatedAmount),
		}
	}
	return out, nil
}

func toRun(r sqlcgen.DepreciationRun, year, month int32) domain.DepreciationRun {
	return domain.DepreciationRun{
		ID: r.ID, PeriodID: r.FiscalPeriodID, Period: domain.PeriodKey{Year: int(year), Month: time.Month(month)},
		AssetCount: int(r.AssetCount), Total: money.FromDecimal(r.TotalAmount), RunCount: int(r.RunCount),
		LastRunAt: r.LastRunAt, CreatedAt: r.CreatedAt, Version: int(r.Version),
	}
}

// UpsertDepreciationRun membuat ringkasan penyusutan periode atau menambah hitungan jalan ulang.
func (s *Store) UpsertDepreciationRun(ctx context.Context, periodID, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).UpsertDepreciationRun(ctx, sqlcgen.UpsertDepreciationRunParams{FiscalPeriodID: periodID, ActorID: actorID})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert depreciation run: %w", err)
	}
	return id, nil
}

// RefreshDepreciationRunTotals menghitung ulang jumlah aset & total run dari barisnya.
func (s *Store) RefreshDepreciationRunTotals(ctx context.Context, id uuid.UUID) error {
	if err := s.q(ctx).RefreshDepreciationRunTotals(ctx, id); err != nil {
		return fmt.Errorf("refresh depreciation run: %w", err)
	}
	return nil
}

// GetDepreciationRun membaca run.
func (s *Store) GetDepreciationRun(ctx context.Context, id uuid.UUID) (domain.DepreciationRun, error) {
	r, err := s.q(ctx).GetDepreciationRun(ctx, id)
	if err != nil {
		return domain.DepreciationRun{}, notFound(err, "get depreciation run")
	}
	return toRun(r.DepreciationRun, r.Year, r.Month), nil
}

// DepreciationRunByPeriod membaca run periode; ok false bila belum ada.
func (s *Store) DepreciationRunByPeriod(ctx context.Context, periodID uuid.UUID) (domain.DepreciationRun, bool, error) {
	r, err := s.q(ctx).GetDepreciationRunByPeriod(ctx, periodID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DepreciationRun{}, false, nil
	}
	if err != nil {
		return domain.DepreciationRun{}, false, fmt.Errorf("get depreciation run by period: %w", err)
	}
	return toRun(r.DepreciationRun, r.Year, r.Month), true, nil
}

// ListDepreciationRuns mengembalikan run terbaru lebih dulu.
func (s *Store) ListDepreciationRuns(ctx context.Context, page paging.Page) (paging.Result[domain.DepreciationRun], error) {
	q := s.q(ctx)
	rows, err := q.ListDepreciationRuns(ctx, sqlcgen.ListDepreciationRunsParams{RowLimit: page.Limit(), RowOffset: page.Offset()})
	if err != nil {
		return paging.Result[domain.DepreciationRun]{}, fmt.Errorf("list depreciation runs: %w", err)
	}
	total, err := q.CountDepreciationRuns(ctx)
	if err != nil {
		return paging.Result[domain.DepreciationRun]{}, fmt.Errorf("count depreciation runs: %w", err)
	}
	items := make([]domain.DepreciationRun, len(rows))
	for i, r := range rows {
		items[i] = toRun(r.DepreciationRun, r.Year, r.Month)
	}
	return paging.Result[domain.DepreciationRun]{Items: items, Page: page, Total: total}, nil
}

// InsertDepreciationLine mencatat penyusutan satu aset satu periode dan memperbarui akumulasi aset.
func (s *Store) InsertDepreciationLine(ctx context.Context, l domain.DepreciationLine, actorID uuid.UUID) error {
	q := s.q(ctx)
	err := q.InsertDepreciationLine(ctx, sqlcgen.InsertDepreciationLineParams{
		ID: l.ID, DepreciationRunID: l.RunID, FixedAssetID: l.AssetID, FiscalPeriodID: l.PeriodID, Amount: l.Amount.Decimal(),
		AccumulatedAmount: l.Accumulated.Decimal(), BookValueAmount: l.BookValue.Decimal(), JournalEntryID: l.JournalEntryID,
		ActorID: actorID,
	})
	if err != nil {
		return fmt.Errorf("insert depreciation line: %w", err)
	}
	if err := q.SetAssetAccumulated(ctx, sqlcgen.SetAssetAccumulatedParams{ID: l.AssetID, AccumulatedAmount: l.Accumulated.Decimal(), ActorID: actorID}); err != nil {
		return fmt.Errorf("set asset accumulated: %w", err)
	}
	return nil
}

// ListDepreciationLines mengembalikan baris penyusutan satu run atau satu aset.
func (s *Store) ListDepreciationLines(ctx context.Context, runID, assetID *uuid.UUID) ([]domain.DepreciationLine, error) {
	rows, err := s.q(ctx).ListDepreciationLines(ctx, sqlcgen.ListDepreciationLinesParams{RunID: nullUUID(runID), AssetID: nullUUID(assetID)})
	if err != nil {
		return nil, fmt.Errorf("list depreciation lines: %w", err)
	}
	out := make([]domain.DepreciationLine, len(rows))
	for i, r := range rows {
		d := r.DepreciationLine
		out[i] = domain.DepreciationLine{
			ID: d.ID, RunID: d.DepreciationRunID, AssetID: d.FixedAssetID, AssetCode: r.AssetCode, AssetName: r.AssetName,
			PeriodID: d.FiscalPeriodID, Period: domain.PeriodKey{Year: int(r.Year), Month: time.Month(r.Month)},
			Amount: money.FromDecimal(d.Amount), Accumulated: money.FromDecimal(d.AccumulatedAmount),
			BookValue: money.FromDecimal(d.BookValueAmount), JournalEntryID: d.JournalEntryID, JournalNumber: r.JournalNumber,
			CreatedAt: d.CreatedAt,
		}
	}
	return out, nil
}
