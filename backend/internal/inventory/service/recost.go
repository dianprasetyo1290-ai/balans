package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/catalog"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// Status recost run.
const (
	RecostPending   = "pending"
	RecostCompleted = "completed"
	RecostFailed    = "failed"
)

// RecostArgs adalah argumen job inventory.recost (TG Lampiran B: queue bulk, 5 percobaan).
type RecostArgs struct {
	RunID uuid.UUID `json:"runId"`
}

// Kind adalah nama job.
func (RecostArgs) Kind() string { return "inventory.recost" }

// InsertOpts: queue bulk, maks. 5 percobaan.
func (RecostArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueBulk, MaxAttempts: 5}
}

// RecostWorker menjalankan recost satu run.
type RecostWorker struct {
	river.WorkerDefaults[RecostArgs]
	service *Service
}

// NewRecostWorker membuat worker.
func NewRecostWorker(s *Service) *RecostWorker { return &RecostWorker{service: s} }

// Work menjalankan recost. Idempotent: run yang sudah selesai dilewati.
func (w *RecostWorker) Work(ctx context.Context, job *river.Job[RecostArgs]) error {
	if err := w.service.Recost(ctx, job.Args.RunID); err != nil {
		return fmt.Errorf("recost %s: %w", job.Args.RunID, err)
	}
	return nil
}

// enqueueRecosts mencatat run recost untuk penerimaan bertanggal mundur yang batch-nya sudah punya barang keluar
// pada/sesudah tanggal itu (S-224, TG 5.8.4). Dipanggil di transaksi posting; periode tertutup sudah ditolak.
func (s *Service) enqueueRecosts(ctx context.Context, src domain.Source, res PostingResult, actorID uuid.UUID) error {
	if !src.BusinessDate.Before(s.today()) {
		return nil
	}
	seen := map[string]bool{}
	for _, m := range res.Movements {
		if m.Type != domain.MovementReceive {
			continue
		}
		key := layerGroup{productID: m.ProductID, batchID: m.BatchID}.key()
		if seen[key] {
			continue
		}
		seen[key] = true
		has, err := s.Store.HasIssuesSince(ctx, m.ProductID, m.BatchID, src.BusinessDate)
		if err != nil {
			return err
		}
		if !has {
			continue
		}
		id, err := s.Store.InsertRecostRun(ctx, postgres.RecostRun{
			ProductID: m.ProductID, BatchID: m.BatchID, FromBusinessDate: src.BusinessDate, TriggerSourceType: src.Type,
			TriggerSourceID: src.ID, TriggerSourceLabel: src.Label,
		}, actorID)
		if err != nil {
			return err
		}
		if s.Jobs != nil {
			if err := s.Jobs.Enqueue(ctx, RecostArgs{RunID: id}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// Recost menghitung ulang konsumsi FIFO satu batch (domain.Replay), menyimpan sisa cost layer baru, mencatat
// selisih HPP per movement keluar (recost_lines), dan memposting jurnal penyesuaian A-RC atas selisihnya saja
// dengan tanggal bisnis dokumen penyebab. Jurnal dan movement lama tidak diubah.
func (s *Service) Recost(ctx context.Context, runID uuid.UUID) error {
	ctx = actor.System(ctx)
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		run, err := s.Store.GetRecostRun(ctx, runID, true)
		if err != nil {
			return err
		}
		if run.Status != RecostPending {
			return nil
		}
		layers, err := s.Store.LockAllLayers(ctx, run.ProductID, run.BatchID)
		if err != nil {
			return err
		}
		issues, err := s.Store.BatchIssues(ctx, run.ProductID, run.BatchID)
		if err != nil {
			return err
		}
		replay := make([]domain.ReplayIssue, len(issues))
		byID := map[uuid.UUID]postgres.BatchIssue{}
		for i, is := range issues {
			replay[i] = is.ReplayIssue
			byID[is.MovementID] = is
		}
		res := domain.Replay(layers, replay)
		if res.Short {
			return s.Store.FinishRecostRun(ctx, run.ID, RecostFailed, decimal.Zero, nil, "Cost layer batch lebih kecil dari total barang keluar")
		}
		for _, l := range layers {
			if err := s.Store.SetLayerRemaining(ctx, l.ID, res.Remaining[l.ID]); err != nil {
				return err
			}
		}
		diffs := res.Diffs(replay)
		total := decimal.Zero
		for _, d := range diffs {
			is := byID[d.MovementID]
			if err := s.Store.InsertRecostLine(ctx, postgres.RecostLine{
				RunID: run.ID, MovementID: d.MovementID, LocationID: is.LocationID, SourceType: is.SourceType,
				SourceID: is.SourceID, Previous: d.Previous, Recomputed: d.Recomputed,
			}); err != nil {
				return err
			}
			total = total.Add(d.Difference())
		}
		journalID, err := s.journalRecost(ctx, run, diffs, byID)
		if err != nil {
			return err
		}
		return s.Store.FinishRecostRun(ctx, run.ID, RecostCompleted, total, journalID, "")
	})
}

// journalRecost mengelompokkan selisih per akun persediaan (tipe lokasi/jenis produk) dan akun lawan (jenis
// dokumen pemakai), lalu memposting A-RC lewat Splits. Tanpa modul accounting jurnal dilewati.
func (s *Service) journalRecost(ctx context.Context, run postgres.RecostRun, diffs []domain.RecostDiff, byID map[uuid.UUID]postgres.BatchIssue) (*uuid.UUID, error) {
	if len(diffs) == 0 || s.Poster == nil {
		return nil, nil
	}
	var lids []uuid.UUID
	for _, d := range diffs {
		lids = append(lids, byID[d.MovementID].LocationID)
	}
	locs, err := s.locationsOf(ctx, lids...)
	if err != nil {
		return nil, err
	}
	prods, err := s.productsOf(ctx, run.ProductID)
	if err != nil {
		return nil, err
	}
	inv, counter := map[string]decimal.Decimal{}, map[string]decimal.Decimal{}
	for _, d := range diffs {
		is := byID[d.MovementID]
		adj := d.Difference().Neg() // positif = HPP turun → persediaan bertambah, beban berkurang
		invAcc := inventoryAccount(prods[run.ProductID], locs[is.LocationID])
		ctrAcc, err := s.counterAccount(ctx, is)
		if err != nil {
			return nil, err
		}
		inv[invAcc] = inv[invAcc].Add(adj)
		counter[ctrAcc] = counter[ctrAcc].Add(adj)
	}
	splits := append(splitsOf("inventory_adjustment", inv), splitsOf("cost_adjustment", counter)...)
	if len(splits) == 0 {
		return nil, nil
	}
	div, err := s.divisionByCode(ctx, DivisionLogistics)
	if err != nil {
		return nil, err
	}
	return s.postJournal(ctx, accounting.PostingRequest{
		RuleCode: "A-RC", SourceType: SourceRecost, SourceID: run.ID, SourceLabel: run.TriggerSourceLabel,
		BusinessDate: run.FromBusinessDate, Splits: splits, Dimensions: accounting.Dimensions{DivisionID: &div},
		Description: fmt.Sprintf("Recost HPP %s akibat backdate %s", prods[run.ProductID].SKU, run.TriggerSourceLabel),
	})
}

func splitsOf(key string, byAccount map[string]decimal.Decimal) []accounting.PostingSplit {
	accounts := make([]string, 0, len(byAccount))
	for a, v := range byAccount {
		if !v.IsZero() {
			accounts = append(accounts, a)
		}
	}
	slices.Sort(accounts)
	out := make([]accounting.PostingSplit, len(accounts))
	for i, a := range accounts {
		out[i] = accounting.PostingSplit{AmountKey: key, AccountCode: a, Amount: money.FromDecimal(byAccount[a]).Round()}
	}
	return out
}

// inventoryAccount memilih akun persediaan (Desain Finance & Accounting bagian 2): packaging 1301, GM 1302, FG
// sesuai tipe lokasi (gudang 1311, konsinyasi 1312, vending 1313, retur 1314).
func inventoryAccount(p catalog.Product, l warehouse.Location) string {
	switch p.Type {
	case catalog.TypePackaging:
		return "1301"
	case catalog.TypeReprocessMaterial:
		return "1302"
	}
	switch l.Type {
	case warehouse.LocationReturns:
		return "1314"
	case warehouse.LocationConsignment:
		return "1312"
	case warehouse.LocationVendingBuffer, warehouse.LocationVendingMachine:
		return "1313"
	case warehouse.LocationGoodsMaterial:
		return "1302"
	}
	return "1311"
}

// counterAccount memilih akun lawan selisih HPP sesuai dokumen pemakai: pemakaian internal 6103/6203, pemusnahan
// 6403/6402, repacking 1311 (HPP bahan menjadi nilai FG hasil), selain itu HPP penjualan 5101.
func (s *Service) counterAccount(ctx context.Context, is postgres.BatchIssue) (string, error) {
	switch is.SourceType {
	case SourceInternalUsage:
		u, err := s.Store.GetUsage(ctx, is.SourceID, false)
		if err != nil {
			return "", err
		}
		if u.Purpose == domain.PurposeEmployeeSample {
			return "6203", nil
		}
		return "6103", nil
	case SourceDisposal:
		d, err := s.Store.GetDisposal(ctx, is.SourceID, false)
		if err != nil {
			return "", err
		}
		if d.Reason == domain.DisposalReturn {
			return "6403", nil
		}
		return "6402", nil
	case SourceRepacking:
		return "1311", nil
	}
	return "5101", nil
}

// RecostPage adalah riwayat recost.
type RecostPage struct {
	paging.Result[postgres.RecostRun]
	Refs Refs
}

// ListRecostRuns mengembalikan riwayat recost (permission baca stok).
func (s *Service) ListRecostRuns(ctx context.Context, productID *uuid.UUID, page paging.Page) (RecostPage, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return RecostPage{}, err
	}
	res, err := s.Store.ListRecostRuns(ctx, productID, page)
	if err != nil {
		return RecostPage{}, err
	}
	pids := make([]uuid.UUID, len(res.Items))
	for i, r := range res.Items {
		pids[i] = r.ProductID
	}
	refs, err := s.refs(ctx, pids, nil)
	if err != nil {
		return RecostPage{}, err
	}
	return RecostPage{Result: res, Refs: refs}, nil
}

// RecostLines membaca selisih HPP per movement satu run (dipakai test dan detail).
func (s *Service) RecostLines(ctx context.Context, runID uuid.UUID) ([]postgres.RecostLine, error) {
	return s.Store.RecostLines(ctx, runID)
}

// RecostRun membaca satu run.
func (s *Service) RecostRun(ctx context.Context, runID uuid.UUID) (postgres.RecostRun, error) {
	return s.Store.GetRecostRun(ctx, runID, false)
}
