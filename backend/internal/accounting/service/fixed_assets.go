package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// Permission aset tetap.
const (
	PermFixedAssetRead  = domain.PermFixedAssetRead
	PermFixedAssetWrite = domain.PermFixedAssetWrite
	PermFixedAssetPost  = domain.PermFixedAssetPost
)

func assetSnapshot(a domain.FixedAsset) map[string]any {
	return map[string]any{
		"code": a.Code, "name": a.Name, "classId": a.ClassID, "locationId": a.LocationID, "locationLabel": a.LocationLabel,
		"divisionId": a.DivisionID, "source": a.Source, "acquisitionDate": a.AcquisitionDate.String(), "cost": a.Cost.String(),
		"salvage": a.Salvage.String(), "usefulLife": a.UsefulLife, "openingAccumulated": a.OpeningAccumulated.String(),
		"accumulated": a.Accumulated.String(), "status": a.Status,
	}
}

// checkClassAccounts memastikan akun kelas aset adalah akun aset (1xxx) yang bisa diposting.
func (s *Service) checkClassAccounts(ctx context.Context, c domain.AssetClass) (domain.AssetClass, error) {
	accounts, err := s.Store.AccountsByIDs(ctx, []uuid.UUID{c.AssetAccountID, c.AccumulatedAccountID})
	if err != nil {
		return c, err
	}
	fields := map[string]string{}
	check := func(id uuid.UUID, field string, wantNormal domain.NormalBalance) string {
		acc, ok := accounts[id]
		switch {
		case !ok:
			fields[field] = "Akun tidak ditemukan"
		case acc.Group != domain.GroupAsset:
			fields[field] = "Pilih akun kelompok Aset (1xxx)"
		case acc.AutoPostingError() != "":
			fields[field] = acc.AutoPostingError()
		case acc.NormalBalance != wantNormal:
			fields[field] = "Saldo normal akun tidak sesuai"
		}
		return acc.Code
	}
	c.AssetAccountCode = check(c.AssetAccountID, "assetAccountId", domain.NormalDebit)
	c.AccumulatedAccountCode = check(c.AccumulatedAccountID, "accumulatedAccountId", domain.NormalCredit)
	if len(fields) > 0 {
		return c, apperr.Validation(fields)
	}
	return c, nil
}

// ListAssetClasses mengembalikan kelas aset (pilihan form aset dan halaman Kelas Aset).
func (s *Service) ListAssetClasses(ctx context.Context, includeInactive bool) ([]domain.AssetClass, error) {
	if _, err := actor.Require(ctx, PermFixedAssetRead); err != nil {
		return nil, err
	}
	return s.Store.ListAssetClasses(ctx, includeInactive)
}

// CreateAssetClass menambah kelas aset.
func (s *Service) CreateAssetClass(ctx context.Context, in domain.AssetClassInput) (domain.AssetClass, error) {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return domain.AssetClass{}, err
	}
	c, err := domain.NewAssetClass(in)
	if err != nil {
		return domain.AssetClass{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if c, err = s.checkClassAccounts(ctx, c); err != nil {
			return err
		}
		if c.ID, err = s.Store.InsertAssetClass(ctx, c, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "fixed_asset_class", EntityID: c.ID, EntityLabel: c.Name,
			After: map[string]any{"code": c.Code, "name": c.Name, "usefulLifeMonths": c.UsefulLifeMonths},
		})
	})
	if err != nil {
		return domain.AssetClass{}, err
	}
	return s.Store.GetAssetClass(ctx, c.ID)
}

// UpdateAssetClass mengubah kelas aset. Umur manfaat baru berlaku untuk aset yang dibuat setelahnya.
func (s *Service) UpdateAssetClass(ctx context.Context, id uuid.UUID, version int, in domain.AssetClassInput) (domain.AssetClass, error) {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return domain.AssetClass{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetAssetClass(ctx, id)
		if err != nil {
			return err
		}
		before := map[string]any{"name": c.Name, "usefulLifeMonths": c.UsefulLifeMonths, "isActive": c.IsActive}
		if err := c.Apply(in); err != nil {
			return err
		}
		if c, err = s.checkClassAccounts(ctx, c); err != nil {
			return err
		}
		if err := s.Store.UpdateAssetClass(ctx, c, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "fixed_asset_class", EntityID: c.ID, EntityLabel: c.Name, Before: before,
			After: map[string]any{"name": c.Name, "usefulLifeMonths": c.UsefulLifeMonths, "isActive": c.IsActive},
		})
	})
	if err != nil {
		return domain.AssetClass{}, err
	}
	return s.Store.GetAssetClass(ctx, id)
}

// ListFixedAssets mengembalikan daftar Aset Tetap.
func (s *Service) ListFixedAssets(ctx context.Context, f postgres.FixedAssetFilter, page paging.Page) (paging.Result[domain.FixedAsset], error) {
	if _, err := actor.RequireAny(ctx, PermFixedAssetRead, PermReportRead); err != nil {
		return paging.Result[domain.FixedAsset]{}, err
	}
	return s.Store.ListFixedAssets(ctx, f, page)
}

// FixedAssetDetail adalah aset beserta riwayat penyusutannya.
type FixedAssetDetail struct {
	Asset domain.FixedAsset
	Lines []domain.DepreciationLine
}

// GetFixedAsset membaca detail aset.
func (s *Service) GetFixedAsset(ctx context.Context, id uuid.UUID) (FixedAssetDetail, error) {
	if _, err := actor.RequireAny(ctx, PermFixedAssetRead, PermReportRead); err != nil {
		return FixedAssetDetail{}, err
	}
	a, err := s.Store.GetFixedAsset(ctx, id, false)
	if err != nil {
		return FixedAssetDetail{}, err
	}
	lines, err := s.Store.ListDepreciationLines(ctx, nil, &id)
	if err != nil {
		return FixedAssetDetail{}, err
	}
	return FixedAssetDetail{Asset: a, Lines: lines}, nil
}

// cutoverDate mengembalikan tanggal cut-over saldo awal, atau nil bila belum diisi.
func (s *Service) cutoverDate(ctx context.Context) (*clock.BusinessDate, error) {
	set, ok, err := s.Store.GetOpeningSet(ctx)
	if err != nil || !ok {
		return nil, err
	}
	d := set.CutoverDate
	return &d, nil
}

// checkAssetRefs memeriksa divisi dan akun pembayaran aset.
func (s *Service) checkAssetRefs(ctx context.Context, a domain.FixedAsset) error {
	fields := map[string]string{}
	if err := s.CheckDivision(ctx, a.DivisionID, a.AcquisitionDate, "divisionId"); err != nil {
		for k, v := range apperr.As(err).Fields {
			fields[k] = v
		}
	}
	if a.PaymentAccountID != nil {
		accounts, err := s.Store.AccountsByIDs(ctx, []uuid.UUID{*a.PaymentAccountID})
		if err != nil {
			return err
		}
		acc, ok := accounts[*a.PaymentAccountID]
		switch {
		case !ok:
			fields["paymentAccountId"] = "Akun tidak ditemukan"
		case acc.AutoPostingError() != "":
			fields["paymentAccountId"] = acc.AutoPostingError()
		case !strings.HasPrefix(acc.Code, "11") && acc.Group != domain.GroupLiability:
			fields["paymentAccountId"] = "Pilih akun Kas & Bank (11xx) atau Utang (2xxx)"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func (s *Service) assetClass(ctx context.Context, id uuid.UUID) (domain.AssetClass, error) {
	if id == uuid.Nil {
		return domain.AssetClass{}, nil
	}
	c, err := s.Store.GetAssetClass(ctx, id)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.AssetClass{}, nil
	}
	return c, err
}

// CreateFixedAsset menyimpan aset baru berstatus Draft.
func (s *Service) CreateFixedAsset(ctx context.Context, in domain.FixedAssetInput) (domain.FixedAsset, error) {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return domain.FixedAsset{}, err
	}
	var asset domain.FixedAsset
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		class, err := s.assetClass(ctx, in.ClassID)
		if err != nil {
			return err
		}
		cutover, err := s.cutoverDate(ctx)
		if err != nil {
			return err
		}
		if asset, err = domain.NewFixedAsset(in, class, cutover); err != nil {
			return err
		}
		if err := s.checkAssetRefs(ctx, asset); err != nil {
			return err
		}
		if asset.ID, err = s.Store.InsertFixedAsset(ctx, asset, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "fixed_asset", EntityID: asset.ID, EntityLabel: asset.Code, After: assetSnapshot(asset),
		})
	})
	if err != nil {
		return domain.FixedAsset{}, err
	}
	return s.Store.GetFixedAsset(ctx, asset.ID, false)
}

// UpdateFixedAsset mengubah aset Draft.
func (s *Service) UpdateFixedAsset(ctx context.Context, id uuid.UUID, version int, in domain.FixedAssetInput) (domain.FixedAsset, error) {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return domain.FixedAsset{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		asset, err := s.lockedAsset(ctx, id, version)
		if err != nil {
			return err
		}
		before := assetSnapshot(asset)
		class, err := s.assetClass(ctx, in.ClassID)
		if err != nil {
			return err
		}
		cutover, err := s.cutoverDate(ctx)
		if err != nil {
			return err
		}
		if err := asset.Update(in, class, cutover); err != nil {
			return err
		}
		if err := s.checkAssetRefs(ctx, asset); err != nil {
			return err
		}
		if err := s.Store.UpdateFixedAsset(ctx, asset, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "fixed_asset", EntityID: asset.ID, EntityLabel: asset.Code,
			Before: before, After: assetSnapshot(asset),
		})
	})
	if err != nil {
		return domain.FixedAsset{}, err
	}
	return s.Store.GetFixedAsset(ctx, id, false)
}

func (s *Service) lockedAsset(ctx context.Context, id uuid.UUID, version int) (domain.FixedAsset, error) {
	asset, err := s.Store.GetFixedAsset(ctx, id, true)
	if err != nil {
		return domain.FixedAsset{}, err
	}
	if version != 0 && asset.Version != version {
		return domain.FixedAsset{}, apperr.New(apperr.CodeVersionConflict)
	}
	return asset, nil
}

// DeleteFixedAsset menghapus aset Draft.
func (s *Service) DeleteFixedAsset(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermFixedAssetWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		asset, err := s.lockedAsset(ctx, id, version)
		if err != nil {
			return err
		}
		if asset.Status != domain.AssetDraft || asset.DeletedAt != nil {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya aset Draft yang bisa dihapus.")
		}
		if err := s.Store.DeleteFixedAsset(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "fixed_asset", EntityID: id, EntityLabel: asset.Code, Before: assetSnapshot(asset),
		})
	})
}

// ActivateFixedAsset mengaktifkan aset Draft. Aset pembelian dijurnal A-31 perolehan (Dr akun aset kelas, Kr
// akun pembayaran) pada tanggal perolehan; aset saldo awal tanpa jurnal karena nilainya masuk lewat A-36.
func (s *Service) ActivateFixedAsset(ctx context.Context, id uuid.UUID, version int) (domain.FixedAsset, error) {
	a, err := actor.Require(ctx, PermFixedAssetPost)
	if err != nil {
		return domain.FixedAsset{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		asset, err := s.lockedAsset(ctx, id, version)
		if err != nil {
			return err
		}
		return s.activate(ctx, asset, a.UserID)
	})
	if err != nil {
		return domain.FixedAsset{}, err
	}
	return s.Store.GetFixedAsset(ctx, id, false)
}

func (s *Service) activate(ctx context.Context, asset domain.FixedAsset, by uuid.UUID) error {
	version := asset.Version
	if err := asset.Activate(); err != nil {
		return err
	}
	if asset.Source == domain.AssetPurchase {
		class, err := s.Store.GetAssetClass(ctx, asset.ClassID)
		if err != nil {
			return err
		}
		accounts, err := s.Store.AccountsByIDs(ctx, []uuid.UUID{*asset.PaymentAccountID})
		if err != nil {
			return err
		}
		div := asset.DivisionID
		e, err := s.Post(ctx, domain.PostingRequest{
			RuleCode: domain.RuleFixedAsset, SourceType: domain.SourceFixedAsset, SourceID: asset.ID, SourceLabel: asset.Code,
			BusinessDate: asset.AcquisitionDate, Amounts: map[string]money.Amount{domain.AmountAcquisition: asset.Cost},
			Accounts: map[domain.Resolver]string{
				domain.ResolverFixedAsset: class.AssetAccountCode, domain.ResolverPayment: accounts[*asset.PaymentAccountID].Code,
			},
			Dimensions: domain.Dimensions{DivisionID: &div}, Description: "Perolehan aset tetap " + asset.Code + " " + asset.Name,
		})
		if err != nil {
			return err
		}
		asset.AcquisitionJournal = &e.ID
	}
	if err := s.Store.UpdateFixedAsset(ctx, asset, version, by); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: "fixed_asset", EntityID: asset.ID, EntityLabel: asset.Code,
		Before: map[string]any{"status": domain.AssetDraft}, After: map[string]any{"status": asset.Status}, Note: "Aktifkan aset",
	})
}

// DisposeInput adalah isian Lepas Aset.
type DisposeInput struct {
	Date      clock.BusinessDate
	Proceeds  money.Amount
	AccountID *uuid.UUID
	Reason    string
}

// DisposeFixedAsset melepas aset aktif dengan jurnal A-31 pelepasan: Dr akumulasi + akun penerimaan + rugi
// (7201), Kr akun aset + laba (7101).
func (s *Service) DisposeFixedAsset(ctx context.Context, id uuid.UUID, version int, in DisposeInput) (domain.FixedAsset, error) {
	a, err := actor.Require(ctx, PermFixedAssetPost)
	if err != nil {
		return domain.FixedAsset{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		asset, err := s.lockedAsset(ctx, id, version)
		if err != nil {
			return err
		}
		class, err := s.Store.GetAssetClass(ctx, asset.ClassID)
		if err != nil {
			return err
		}
		d, err := asset.Dispose(in.Date, in.Proceeds, in.AccountID, in.Reason)
		if err != nil {
			return err
		}
		accounts := map[domain.Resolver]string{
			domain.ResolverFixedAsset: class.AssetAccountCode, domain.ResolverAccumulatedDepreciation: class.AccumulatedAccountCode,
		}
		if asset.DisposalAccountID != nil {
			found, err := s.Store.AccountsByIDs(ctx, []uuid.UUID{*asset.DisposalAccountID})
			if err != nil {
				return err
			}
			acc, ok := found[*asset.DisposalAccountID]
			if !ok {
				return apperr.Validation(map[string]string{"disposalAccountId": "Akun tidak ditemukan"})
			}
			accounts[domain.ResolverPayment] = acc.Code
		}
		div := asset.DivisionID
		e, err := s.Post(ctx, domain.PostingRequest{
			RuleCode: domain.RuleFixedAsset, SourceType: domain.SourceFixedAssetDisposal, SourceID: asset.ID, SourceLabel: asset.Code,
			BusinessDate: in.Date, Amounts: d.Amounts(), Accounts: accounts, Dimensions: domain.Dimensions{DivisionID: &div},
			Description: "Pelepasan aset tetap " + asset.Code + ": " + asset.DisposalReason,
		})
		if err != nil {
			return err
		}
		asset.DisposalJournal = &e.ID
		if err := s.Store.UpdateFixedAsset(ctx, asset, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: "fixed_asset", EntityID: asset.ID, EntityLabel: asset.Code,
			Before: map[string]any{"status": domain.AssetActive},
			After:  map[string]any{"status": asset.Status, "proceeds": in.Proceeds.String(), "reason": asset.DisposalReason},
			Note:   "Lepas aset",
		})
	})
	if err != nil {
		return domain.FixedAsset{}, err
	}
	return s.Store.GetFixedAsset(ctx, id, false)
}

// DepreciationRunDetail adalah run beserta barisnya.
type DepreciationRunDetail struct {
	Run   domain.DepreciationRun
	Lines []domain.DepreciationLine
}

// ListDepreciationRuns mengembalikan riwayat penyusutan per periode.
func (s *Service) ListDepreciationRuns(ctx context.Context, page paging.Page) (paging.Result[domain.DepreciationRun], error) {
	if _, err := actor.RequireAny(ctx, PermFixedAssetRead, PermReportRead); err != nil {
		return paging.Result[domain.DepreciationRun]{}, err
	}
	return s.Store.ListDepreciationRuns(ctx, page)
}

// GetDepreciationRun membaca run penyusutan beserta baris per aset.
func (s *Service) GetDepreciationRun(ctx context.Context, id uuid.UUID) (DepreciationRunDetail, error) {
	if _, err := actor.RequireAny(ctx, PermFixedAssetRead, PermReportRead); err != nil {
		return DepreciationRunDetail{}, err
	}
	r, err := s.Store.GetDepreciationRun(ctx, id)
	if err != nil {
		return DepreciationRunDetail{}, err
	}
	lines, err := s.Store.ListDepreciationLines(ctx, &id, nil)
	if err != nil {
		return DepreciationRunDetail{}, err
	}
	return DepreciationRunDetail{Run: r, Lines: lines}, nil
}

// RunDepreciation adalah aksi Jalankan Penyusutan dari halaman Penyusutan (permission post aset tetap).
func (s *Service) RunDepreciation(ctx context.Context, k domain.PeriodKey) (DepreciationRunDetail, error) {
	if _, err := actor.Require(ctx, PermFixedAssetPost); err != nil {
		return DepreciationRunDetail{}, err
	}
	return s.depreciate(ctx, k)
}

// Depreciate menjalankan penyusutan garis lurus periode k sebagai proses sistem (job accounting.depreciate).
// Idempotent: aset yang sudah punya baris penyusutan periode k dilewati (uq_depreciation_lines_asset_period).
func (s *Service) Depreciate(ctx context.Context, k domain.PeriodKey) (DepreciationRunDetail, error) {
	if _, ok := actor.From(ctx); !ok {
		ctx = actor.System(ctx)
	}
	return s.depreciate(ctx, k)
}

func (s *Service) depreciate(ctx context.Context, k domain.PeriodKey) (DepreciationRunDetail, error) {
	if k.Month < time.January || k.Month > time.December || k.Year < 2000 {
		return DepreciationRunDetail{}, apperr.Validation(map[string]string{"month": "Periode tidak valid"})
	}
	by := actor.UserID(ctx)
	var runID uuid.UUID
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.Store.AdvisoryLockPeriod(ctx, postgres.LockDepreciation, k); err != nil {
			return err
		}
		start := clock.NewBusinessDate(k.Year, k.Month, 1)
		p, err := s.OpenPeriod(ctx, start)
		if err != nil {
			return err
		}
		end := p.End()
		assets, err := s.Store.DepreciableAssets(ctx, p.ID, start)
		if err != nil {
			return err
		}
		if runID, err = s.Store.UpsertDepreciationRun(ctx, p.ID, by); err != nil {
			return err
		}
		classes := map[uuid.UUID]domain.AssetClass{}
		posted := 0
		total := money.Zero
		for _, asset := range assets {
			amount := asset.DepreciationFor(k)
			if !amount.IsPositive() {
				continue
			}
			class, ok := classes[asset.ClassID]
			if !ok {
				if class, err = s.Store.GetAssetClass(ctx, asset.ClassID); err != nil {
					return err
				}
				classes[asset.ClassID] = class
			}
			lineID, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("new depreciation line id: %w", err)
			}
			div := asset.DivisionID
			e, err := s.Post(ctx, domain.PostingRequest{
				RuleCode: domain.RuleFixedAsset, SourceType: domain.SourceFixedAssetDepreciation, SourceID: lineID,
				SourceLabel: asset.Code + " " + p.Label(), BusinessDate: end,
				Amounts:     map[string]money.Amount{domain.AmountDepreciation: amount},
				Accounts:    map[domain.Resolver]string{domain.ResolverAccumulatedDepreciation: class.AccumulatedAccountCode},
				Dimensions:  domain.Dimensions{DivisionID: &div},
				Description: fmt.Sprintf("Penyusutan %s %s %s", p.Label(), asset.Code, asset.Name),
			})
			if err != nil {
				return fmt.Errorf("penyusutan %s: %w", asset.Code, err)
			}
			asset.RecordDepreciation(amount)
			if err := s.Store.InsertDepreciationLine(ctx, domain.DepreciationLine{
				ID: lineID, RunID: runID, AssetID: asset.ID, PeriodID: p.ID, Amount: amount, Accumulated: asset.Accumulated,
				BookValue: asset.BookValue(), JournalEntryID: e.ID,
			}, by); err != nil {
				return err
			}
			posted++
			total = total.Add(amount)
		}
		if err := s.Store.RefreshDepreciationRunTotals(ctx, runID); err != nil {
			return err
		}
		pending, err := s.Store.PendingDepreciationCount(ctx, p.ID, start)
		if err != nil {
			return err
		}
		if pending == 0 {
			if err := s.markStepDone(ctx, p, domain.StepDepreciation, "Penyusutan otomatis selesai"); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: "depreciation_run", EntityID: runID, EntityLabel: "Penyusutan " + p.Label(),
			After: map[string]any{"assets": posted, "total": total.String()},
			Note:  fmt.Sprintf("Penyusutan %s: %d aset, %s", p.Label(), posted, total.String()),
		})
	})
	if err != nil {
		return DepreciationRunDetail{}, err
	}
	r, err := s.Store.GetDepreciationRun(ctx, runID)
	if err != nil {
		return DepreciationRunDetail{}, err
	}
	lines, err := s.Store.ListDepreciationLines(ctx, &runID, nil)
	if err != nil {
		return DepreciationRunDetail{}, err
	}
	return DepreciationRunDetail{Run: r, Lines: lines}, nil
}

// DepreciateArgs adalah argumen job accounting.depreciate (Lampiran B). Year/Month 0 = bulan sebelum hari ini
// (WIB), dipakai jadwal tanggal 1 pukul 01.00.
type DepreciateArgs struct {
	Year  int `json:"year,omitempty"`
	Month int `json:"month,omitempty"`
}

// Kind adalah nama job.
func (DepreciateArgs) Kind() string { return "accounting.depreciate" }

// InsertOpts: antrean bulk, maks. 3 percobaan.
func (DepreciateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: 3}
}

// PreviousPeriod adalah periode bulan sebelum tanggal bisnis.
func PreviousPeriod(today clock.BusinessDate) domain.PeriodKey {
	if today.Month() == time.January {
		return domain.PeriodKey{Year: today.Year() - 1, Month: time.December}
	}
	return domain.PeriodKey{Year: today.Year(), Month: today.Month() - 1}
}

// DepreciateWorker menjalankan penyusutan bulanan.
type DepreciateWorker struct {
	river.WorkerDefaults[DepreciateArgs]
	service *Service
}

// NewDepreciateWorker membuat worker.
func NewDepreciateWorker(s *Service) *DepreciateWorker { return &DepreciateWorker{service: s} }

// Work menyusutkan aset periode argumen (atau bulan lalu). Periode yang sudah ditutup dilewati tanpa retry.
func (w *DepreciateWorker) Work(ctx context.Context, job *river.Job[DepreciateArgs]) error {
	k := domain.PeriodKey{Year: job.Args.Year, Month: time.Month(job.Args.Month)}
	if k.Year == 0 || k.Month == 0 {
		k = PreviousPeriod(clock.Today(w.service.Clock))
	}
	r, err := w.service.Depreciate(ctx, k)
	if apperr.HasCode(err, apperr.CodePeriodLocked) {
		if w.service.Log != nil {
			w.service.Log.WarnContext(ctx, "depreciation skipped: period locked", "period", k.Label())
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("depreciate %s: %w", k.Label(), err)
	}
	if w.service.Log != nil {
		w.service.Log.InfoContext(ctx, "depreciation posted", "period", k.Label(), "assets", r.Run.AssetCount, "total", r.Run.Total.String())
	}
	return nil
}
